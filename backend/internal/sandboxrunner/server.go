// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

var ErrProtocol = errors.New("sandbox runner request is invalid")
var ErrUnavailable = errors.New("sandbox runner dependency is unavailable")

type ExecuteCommand struct {
	Scope          domainsandbox.Scope
	WorkloadKind   infrasandbox.WorkloadKind
	IdempotencyKey string
	Deadline       time.Time
	Entrypoint     string
	Policy         runtimePolicy
	Identity       sandboxidentity.Request
	RawBody        []byte
}

type ExecutionProjection struct {
	ExecutionID string
	Status      infrasandbox.ExecutionStatus
}

type Scheduler interface {
	Accept(context.Context, ExecuteCommand) (ExecutionProjection, error)
}

// Store provides read-only execution and queue projections. It is deliberately
// separate from scheduling so the HTTP boundary cannot manipulate runtime state
// outside the lifecycle interface.
type Store interface {
	Status(context.Context, string) (infrasandbox.ExecuteResult, error)
	Lookup(context.Context, infrasandbox.ExecutionLookupRequest) (infrasandbox.ExecutionLookupResult, error)
	QueueStatus(context.Context, string) (infrasandbox.QueueStatus, error)
}

// LifecycleManager performs idempotent execution state transitions.
type LifecycleManager interface {
	KeepAlive(context.Context, string) error
	Cancel(context.Context, string) error
}

// ReadinessProbe validates that the Runner can still reach its dedicated
// rootless runtime without creating a tenant execution.
type ReadinessProbe interface {
	Ready(context.Context) error
}

// ArtifactPublisher accepts only short-lived artifact publication capabilities.
// It must never persist or log their bearer fields.
type ArtifactPublisher interface {
	PublishArtifact(context.Context, string, infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error)
	BeginBuild(context.Context, string, string) (infrasandbox.BuildObservation, error)
	BuildStatus(context.Context, string, string) (infrasandbox.BuildObservation, error)
}

// ConfigurationSource returns the non-secret Runner configuration projection
// that the control plane may inspect. Raw endpoints and credentials never cross
// this interface.
type ConfigurationSource interface {
	Configuration(context.Context) (ConfigurationProjection, error)
}

// RuntimeStatusSource provides the authenticated operations projection. Its
// response is deliberately aggregate-only: no tenant, execution, container,
// command, artifact, endpoint, or credential data may cross this boundary.
type RuntimeStatusSource interface {
	RuntimeStatus(context.Context) (RuntimeStatusProjection, error)
}

// ConfigurationApplier validates and atomically activates a complete signed
// scheduler snapshot. It intentionally accepts no partial mutations.
type ConfigurationApplier interface {
	ApplyConfiguration(context.Context, infrasandbox.SchedulerConfiguration) error
}

type ConfigurationProjection struct {
	Schema  string `json:"schema"`
	Version uint64 `json:"version"`
}

const runtimeStatusSchemaV1 = "coze.sandbox.runner_runtime_status.v1"

const (
	memoryReserveAvailable      = "available"
	memoryReserveBelowWatermark = "below_watermark"
	memoryReserveUnknown        = "unknown"
)

type RuntimeStatusProjection struct {
	Schema                      string         `json:"schema"`
	AppliedConfigurationVersion uint64         `json:"applied_configuration_version"`
	Queued                      int            `json:"queued"`
	QueuedByScope               map[string]int `json:"queued_by_scope"`
	Running                     int            `json:"running"`
	UsedWeight                  int            `json:"used_weight"`
	TotalWeight                 int            `json:"total_weight"`
	IdleContainers              int            `json:"idle_containers"`
	ActiveContainers            int            `json:"active_containers"`
	QuarantinedContainers       int            `json:"quarantined_containers"`
	MemoryReserveState          string         `json:"memory_reserve_state"`
}

type acceptSchedulerFunc func(context.Context, ExecuteCommand) (ExecutionProjection, error)

func (f acceptSchedulerFunc) Accept(ctx context.Context, command ExecuteCommand) (ExecutionProjection, error) {
	return f(ctx, command)
}

type Server struct {
	config               Config
	scheduler            Scheduler
	store                Store
	lifecycle            LifecycleManager
	artifacts            ArtifactPublisher
	configuration        ConfigurationSource
	configurationApplier ConfigurationApplier
	runtimeStatus        RuntimeStatusSource
	readiness            ReadinessProbe
	handler              http.Handler
}

type Dependencies struct {
	Scheduler            Scheduler
	Store                Store
	Lifecycle            LifecycleManager
	Artifacts            ArtifactPublisher
	Configuration        ConfigurationSource
	ConfigurationApplier ConfigurationApplier
	RuntimeStatus        RuntimeStatusSource
	Readiness            ReadinessProbe
}

func NewServer(config Config, dependencies Dependencies) (*Server, error) {
	if dependencies.Scheduler == nil || config.AuthToken == "" || config.ContextVerifyKeys.ActiveKeyID == "" {
		return nil, ErrConfiguration
	}
	server := &Server{config: config, scheduler: dependencies.Scheduler, store: dependencies.Store, lifecycle: dependencies.Lifecycle, artifacts: dependencies.Artifacts, configuration: dependencies.Configuration, configurationApplier: dependencies.ConfigurationApplier, runtimeStatus: dependencies.RuntimeStatus, readiness: dependencies.Readiness}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", server.health)
	mux.HandleFunc("GET /v1/metrics", server.metrics)
	mux.HandleFunc("POST /v1/executions", server.execute)
	mux.HandleFunc("GET /v1/executions/", server.control)
	mux.HandleFunc("POST /v1/executions/", server.control)
	mux.HandleFunc("POST /v1/executions:lookup", server.control)
	mux.HandleFunc("GET /v1/configuration", server.control)
	mux.HandleFunc("PUT /v1/configuration", server.control)
	mux.HandleFunc("GET /v1/runtime-status", server.control)
	mux.HandleFunc("/v1/", server.notFound)
	server.handler = mux
	return server, nil
}

func (s *Server) Handler() http.Handler {
	if s == nil {
		return http.NotFoundHandler()
	}
	return s.handler
}

func (s *Server) health(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if s.readiness != nil && s.readiness.Ready(request.Context()) != nil {
		writePublicError(writer, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	// The health contract is an admission boundary. Do not advertise a scope
	// until its reviewed runtime adapter is present in the immutable execution
	// image; claiming MCP/AppDev here would route real work to a guaranteed
	// failure instead of allowing an existing compatible Provider to serve it.
	writeJSON(writer, http.StatusOK, map[string]any{"protocol_version": "v1", "status": domainsandbox.HealthStatusHealthy, "capabilities": []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopePlugin}, "features": []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1, domainsandbox.ProviderFeatureSignedExecutionContext}})
}

func (s *Server) execute(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if !authenticateBearer(request, s.config.AuthToken) {
		writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	body := http.MaxBytesReader(writer, request.Body, maxRequestBytes)
	defer body.Close()
	raw, err := readAll(body)
	if err != nil {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	command, err := parseExecute(raw)
	if err != nil {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if _, supported := adapterForCommand(command); !supported {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	digest := sha256.Sum256(raw)
	identity, err := s.config.ContextVerifyKeys.Verify(request.Header.Get(infrasandbox.SandboxContextHeader), request.Header.Get(infrasandbox.SandboxContextSignatureHeader), sandboxidentity.Scope(command.Scope), digest[:], time.Now().UTC())
	if err != nil {
		writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	command.Identity = identity
	result, err := s.scheduler.Accept(request.Context(), command)
	if err != nil || !validExecutionID(result.ExecutionID) || result.Status != infrasandbox.ExecutionStatusAccepted {
		writePublicError(writer, http.StatusServiceUnavailable, "UNAVAILABLE")
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]any{"schema": infrasandbox.ExecuteSchemaV1, "execution_id": result.ExecutionID, "status": result.Status, "stdout": "", "stderr": "", "artifacts": []any{}, "artifact_descriptors": []any{}, "preview_route": ""})
}

func (s *Server) control(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if !authenticateBearer(request, s.config.AuthToken) {
		writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	if !validControlPath(request.Method, request.URL.Path) {
		writePublicError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if err := s.delegateControl(writer, request); err != nil {
		if errors.Is(err, ErrProtocol) {
			writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
			return
		}
		writePublicError(writer, http.StatusServiceUnavailable, "UNAVAILABLE")
	}
}

func (s *Server) delegateControl(writer http.ResponseWriter, request *http.Request) error {
	path := request.URL.Path
	if request.Method == http.MethodGet {
		switch {
		case path == "/v1/configuration":
			if s.configuration == nil {
				return ErrUnavailable
			}
			configuration, err := s.configuration.Configuration(request.Context())
			if err != nil {
				return err
			}
			writeJSON(writer, http.StatusOK, configuration)
			return nil
		case path == "/v1/runtime-status":
			if s.runtimeStatus == nil {
				return ErrUnavailable
			}
			status, err := s.runtimeStatus.RuntimeStatus(request.Context())
			if err != nil {
				return err
			}
			writeJSON(writer, http.StatusOK, status)
			return nil
		case strings.HasSuffix(path, "/queue-status"):
			if s.store == nil {
				return ErrUnavailable
			}
			status, err := s.store.QueueStatus(request.Context(), executionIDFromControlPath(path))
			if err != nil {
				return err
			}
			writeJSON(writer, http.StatusOK, status)
			return nil
		case strings.Contains(path, "/builds/"):
			if s.artifacts == nil {
				return ErrUnavailable
			}
			parts := strings.Split(path, "/")
			observation, err := s.artifacts.BuildStatus(request.Context(), parts[3], parts[5])
			if err != nil {
				return err
			}
			writeBuildObservation(writer, parts[5], observation)
			return nil
		default:
			if s.store == nil {
				return ErrUnavailable
			}
			result, err := s.store.Status(request.Context(), executionIDFromControlPath(path))
			if err != nil {
				return err
			}
			writeExecutionResult(writer, http.StatusOK, result)
			return nil
		}
	}
	if request.Method == http.MethodPut && path == "/v1/configuration" {
		if s.configurationApplier == nil {
			return ErrUnavailable
		}
		body := http.MaxBytesReader(writer, request.Body, maxRequestBytes)
		defer body.Close()
		raw, err := readAll(body)
		if err != nil {
			return ErrProtocol
		}
		configuration, err := infrasandbox.DecodeSchedulerConfiguration(raw)
		if err != nil {
			return ErrProtocol
		}
		if err := s.configurationApplier.ApplyConfiguration(request.Context(), configuration); err != nil {
			return ErrProtocol
		}
		writer.WriteHeader(http.StatusNoContent)
		return nil
	}

	body := http.MaxBytesReader(writer, request.Body, maxRequestBytes)
	defer body.Close()
	raw, err := readAll(body)
	if err != nil {
		return ErrProtocol
	}
	parsed, err := parseControlRequest(path, raw, time.Now().UTC())
	if err != nil {
		return ErrProtocol
	}
	switch {
	case path == "/v1/executions:lookup":
		// Reconciliation is not advertised by this runner. A future lookup
		// contract must carry signed tenant identity before a caller-controlled
		// operation ID can select an execution.
		return ErrProtocol
	case strings.HasSuffix(path, ":keep-alive"):
		if s.lifecycle == nil {
			return ErrUnavailable
		}
		if err := s.lifecycle.KeepAlive(request.Context(), executionIDFromControlPath(path)); err != nil {
			return err
		}
		writer.WriteHeader(http.StatusNoContent)
		return nil
	case strings.HasSuffix(path, ":cancel"):
		if s.lifecycle == nil {
			return ErrUnavailable
		}
		if err := s.lifecycle.Cancel(request.Context(), executionIDFromControlPath(path)); err != nil {
			return err
		}
		writer.WriteHeader(http.StatusNoContent)
		return nil
	case strings.HasSuffix(path, "/builds"):
		if s.artifacts == nil || parsed.BuildID == "" {
			return ErrUnavailable
		}
		observation, err := s.artifacts.BeginBuild(request.Context(), executionIDFromControlPath(path), parsed.BuildID)
		if err != nil {
			return err
		}
		writeBuildObservation(writer, parsed.BuildID, observation)
		return nil
	case strings.HasSuffix(path, "/artifacts:publish"):
		if s.artifacts == nil || parsed.Artifact == nil {
			return ErrUnavailable
		}
		result, err := s.artifacts.PublishArtifact(request.Context(), executionIDFromControlPath(path), *parsed.Artifact)
		if err != nil {
			return err
		}
		writeJSON(writer, http.StatusOK, map[string]any{"schema": infrasandbox.ArtifactPublishSchemaV1, "accepted": result.Accepted})
		return nil
	}
	return ErrProtocol
}

func executionIDFromControlPath(path string) string {
	value := strings.TrimPrefix(path, "/v1/executions/")
	value = strings.TrimSuffix(value, ":keep-alive")
	value = strings.TrimSuffix(value, ":cancel")
	return strings.Split(value, "/")[0]
}

func writeExecutionResult(writer http.ResponseWriter, status int, result infrasandbox.ExecuteResult) {
	writeJSON(writer, status, map[string]any{
		"schema": infrasandbox.ExecuteSchemaV1, "execution_id": result.ExecutionID, "status": result.Status,
		"exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr,
		"artifacts": result.Artifacts, "artifact_descriptors": result.ArtifactDescriptors, "preview_route": result.PreviewRoute,
	})
}

func writeBuildObservation(writer http.ResponseWriter, operationID string, observation infrasandbox.BuildObservation) {
	value := map[string]any{"schema": infrasandbox.AppDevBuildSchemaV1, "operation_id": operationID, "status": observation.Status}
	if observation.Descriptor != (infrasandbox.ArtifactDescriptor{}) {
		value["descriptor"] = observation.Descriptor
	}
	if observation.SafeErrorCode != "" {
		value["safe_error_code"] = observation.SafeErrorCode
		value["safe_error_message"] = observation.SafeErrorMessage
	}
	writeJSON(writer, http.StatusOK, value)
}

func writeLookupResult(writer http.ResponseWriter, result infrasandbox.ExecutionLookupResult) {
	value := map[string]any{"schema": infrasandbox.ExecutionLookupSchemaV1, "status": result.Status}
	if result.Status == infrasandbox.ExecutionLookupFound {
		value["execution"] = map[string]any{
			"schema": infrasandbox.ExecuteSchemaV1, "execution_id": result.Execution.ExecutionID,
			"status": result.Execution.Status, "exit_code": result.Execution.ExitCode,
			"stdout": result.Execution.Stdout, "stderr": result.Execution.Stderr,
			"artifacts": result.Execution.Artifacts, "artifact_descriptors": result.Execution.ArtifactDescriptors,
			"preview_route": result.Execution.PreviewRoute,
		}
	}
	writeJSON(writer, http.StatusOK, value)
}

func validControlPath(method, path string) bool {
	if method == http.MethodGet && path == "/v1/runtime-status" {
		return true
	}
	if method == http.MethodGet && path == "/v1/configuration" {
		return true
	}
	if method == http.MethodPut && path == "/v1/configuration" {
		return true
	}
	if method == http.MethodPost && path == "/v1/executions:lookup" {
		return true
	}
	if method == http.MethodPost && strings.HasPrefix(path, "/v1/executions/") {
		id := strings.TrimPrefix(path, "/v1/executions/")
		if strings.HasSuffix(id, ":keep-alive") {
			return validExecutionID(strings.TrimSuffix(id, ":keep-alive"))
		}
		if strings.HasSuffix(id, ":cancel") {
			return validExecutionID(strings.TrimSuffix(id, ":cancel"))
		}
	}
	segments := strings.Split(strings.TrimPrefix(path, "/v1/executions/"), "/")
	if !strings.HasPrefix(path, "/v1/executions/") || len(segments) == 0 || !validExecutionID(segments[0]) {
		return false
	}
	if len(segments) == 1 {
		return method == http.MethodGet
	}
	if len(segments) == 2 && segments[1] == "queue-status" {
		return method == http.MethodGet
	}
	if len(segments) == 2 && segments[1] == "builds" {
		return method == http.MethodPost
	}
	if len(segments) == 3 && segments[1] == "builds" && validExecutionID(segments[2]) {
		return method == http.MethodGet
	}
	if len(segments) == 2 && segments[1] == "artifacts:publish" {
		return method == http.MethodPost
	}
	return false
}

func (s *Server) notFound(writer http.ResponseWriter, _ *http.Request) {
	writePublicError(writer, http.StatusNotFound, "NOT_FOUND")
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
func writePublicError(writer http.ResponseWriter, status int, code string) {
	writeJSON(writer, status, map[string]string{"code": code, "message": "sandbox runner request was rejected"})
}

func readAll(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(reader)
	if err != nil || len(data) == 0 || len(data) > maxRequestBytes {
		return nil, ErrProtocol
	}
	return data, nil
}
