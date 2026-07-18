// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/safehttp"
)

type RemoteProvider struct {
	endpoint   *endpointPolicy
	credential string
	doer       remoteHTTPDoer
	closeState atomic.Pointer[remoteProviderCloseAttempt]
	closed     atomic.Bool
}

type remoteProviderCloseAttempt struct {
	done chan struct{}
	err  error
}

const (
	remoteReconciliationRequestTimeout   = 30 * time.Second
	ReconciliationProtocolVersionHeader  = "X-Coze-Sandbox-Reconciliation-Version"
	ExecutionLookupProtocolVersionHeader = "X-Coze-Sandbox-Execution-Lookup-Version"
	ExecutionLookupProtocolVersionV1     = "v1"
)

func (p *RemoteProvider) CloseContext(ctx context.Context) error {
	if ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	for {
		if p.closed.Load() {
			return nil
		}
		attempt := &remoteProviderCloseAttempt{done: make(chan struct{})}
		if p.closeState.CompareAndSwap(nil, attempt) {
			err := p.closeDoer(ctx)
			attempt.err = err
			if err == nil {
				p.closed.Store(true)
			}
			close(attempt.done)
			if err != nil {
				p.closeState.CompareAndSwap(attempt, nil)
			}
			return err
		}
		current := p.closeState.Load()
		if current == nil {
			continue
		}
		select {
		case <-current.done:
			return current.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *RemoteProvider) closeDoer(ctx context.Context) error {
	if closer, ok := p.doer.(interface{ CloseContext(context.Context) error }); ok {
		return closer.CloseContext(ctx)
	}
	return nil
}

func (*RemoteProvider) String() string   { return "RemoteProvider{credential:<redacted>}" }
func (*RemoteProvider) GoString() string { return "RemoteProvider{credential:<redacted>}" }

func (*RemoteProvider) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "RemoteProvider{credential:<redacted>}")
}

type remoteHTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

var errRemoteJSONInvalid = errors.New("sandbox provider response JSON is invalid")

const maxRemoteJSONDepth = 32

type healthResponseV1 struct {
	ProtocolVersion string                     `json:"protocol_version"`
	Status          domainsandbox.HealthStatus `json:"status"`
	Capabilities    []domainsandbox.Scope      `json:"capabilities"`
}

type runtimePolicyV1 struct {
	TimeoutSeconds          int                           `json:"timeout_seconds"`
	MemoryLimitMB           int                           `json:"memory_limit_mb"`
	CPULimit                float64                       `json:"cpu_limit"`
	MaxOutputBytes          int64                         `json:"max_output_bytes"`
	MaxConcurrency          int                           `json:"max_concurrency"`
	AllowNetwork            bool                          `json:"allow_network"`
	NetworkAllowlist        []string                      `json:"network_allowlist"`
	AllowedEnvNames         []string                      `json:"allowed_env_names"`
	VirtualReadPrefixes     []string                      `json:"virtual_read_prefixes"`
	VirtualWritePrefixes    []string                      `json:"virtual_write_prefixes"`
	AllowedExecutables      []string                      `json:"allowed_executables"`
	FFIEnabled              bool                          `json:"ffi_enabled"`
	NodeModulesMode         domainsandbox.NodeModulesMode `json:"node_modules_mode"`
	NodeModulesDirectoryRef string                        `json:"node_modules_directory_ref"`
}

type artifactReferenceV1 struct {
	Direction ArtifactDirection `json:"direction"`
	URL       string            `json:"url"`
	Digest    string            `json:"digest"`
	Size      int64             `json:"size"`
	MediaType string            `json:"media_type"`
	ExpiresAt string            `json:"expires_at"`
}

type executeRequestV1 struct {
	Schema             string                `json:"schema"`
	Scope              domainsandbox.Scope   `json:"scope"`
	WorkloadKind       WorkloadKind          `json:"workload_kind"`
	IdempotencyKey     string                `json:"idempotency_key"`
	Deadline           string                `json:"deadline"`
	Policy             runtimePolicyV1       `json:"policy"`
	Entrypoint         string                `json:"entrypoint"`
	Args               []string              `json:"args"`
	Env                map[string]string     `json:"env"`
	Stdin              []byte                `json:"stdin"`
	Files              []FileReference       `json:"files"`
	ArtifactReferences []artifactReferenceV1 `json:"artifact_references"`
}

type executeResponseV1 struct {
	Schema              string               `json:"schema"`
	ExecutionID         string               `json:"execution_id"`
	Status              ExecutionStatus      `json:"status"`
	ExitCode            *int                 `json:"exit_code"`
	Stdout              string               `json:"stdout"`
	Stderr              string               `json:"stderr"`
	Artifacts           []ArtifactSummary    `json:"artifacts"`
	ArtifactDescriptors []ArtifactDescriptor `json:"artifact_descriptors"`
	PreviewRoute        string               `json:"preview_route"`
}

type executionLookupRequestV1 struct {
	Schema        string              `json:"schema"`
	Scope         domainsandbox.Scope `json:"scope"`
	WorkloadKind  WorkloadKind        `json:"workload_kind"`
	OperationID   string              `json:"operation_id"`
	RequestDigest string              `json:"request_digest"`
}

type executionLookupLegacyRequestV1 struct {
	Schema       string              `json:"schema"`
	Scope        domainsandbox.Scope `json:"scope"`
	WorkloadKind WorkloadKind        `json:"workload_kind"`
	SpaceID      string              `json:"space_id"`
	ProjectID    string              `json:"project_id"`
	Generation   uint64              `json:"generation"`
	OperationID  string              `json:"operation_id"`
}

type executionLookupResponseV1 struct {
	Schema    string                `json:"schema"`
	Status    ExecutionLookupStatus `json:"status"`
	Execution *executeResponseV1    `json:"execution"`
}

type artifactPublishRequestV1 struct {
	Schema     string             `json:"schema"`
	Descriptor ArtifactDescriptor `json:"descriptor"`
	UploadURL  string             `json:"upload_url"`
	Token      string             `json:"upload_token"`
	ExpiresAt  string             `json:"expires_at"`
}

type artifactPublishResponseV1 struct {
	Schema   string `json:"schema"`
	Accepted bool   `json:"accepted"`
}

type appDevBuildRequestV1 struct {
	Schema      string `json:"schema"`
	OperationID string `json:"operation_id"`
}

type appDevBuildResponseV1 struct {
	Schema           string              `json:"schema"`
	OperationID      string              `json:"operation_id"`
	Status           BuildStatus         `json:"status"`
	Descriptor       *ArtifactDescriptor `json:"descriptor,omitempty"`
	SafeErrorCode    string              `json:"safe_error_code,omitempty"`
	SafeErrorMessage string              `json:"safe_error_message,omitempty"`
}

func NewRemoteProvider(config RemoteProviderConfig) (*RemoteProvider, error) {
	endpoint, err := validateRemoteProviderConfig(config)
	if err != nil {
		return nil, err
	}
	client, err := safehttp.NewClient(safehttp.ClientOptions{
		Policy:               endpoint.httpPolicy,
		Timeout:              config.Timeout,
		MaxResponseBodyBytes: MaxExecutionResponseWireBytes,
		UnavailableError:     domainsandbox.ErrProviderUnhealthy,
	})
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return &RemoteProvider{endpoint: endpoint, credential: config.Credential, doer: client}, nil
}

func newRemoteProviderWithDoer(config RemoteProviderConfig, doer remoteHTTPDoer) (*RemoteProvider, error) {
	endpoint, err := validateRemoteProviderConfig(config)
	if err != nil {
		return nil, err
	}
	if doer == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return &RemoteProvider{endpoint: endpoint, credential: config.Credential, doer: doer}, nil
}

func validateRemoteProviderConfig(config RemoteProviderConfig) (*endpointPolicy, error) {
	if !validCredential(config.Credential) {
		return nil, domainsandbox.ErrCredentialInvalid
	}
	endpoint, err := newEndpointPolicy(config)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return endpoint, nil
}

func (p *RemoteProvider) Health(ctx context.Context) (HealthResult, error) {
	if p == nil || ctx == nil {
		return HealthResult{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return HealthResult{}, err
	}
	ctx = safehttp.WithResponseBodyLimit(ctx, MaxHealthResponseBodyBytes)
	request, err := p.newRequest(ctx, http.MethodGet, "/v1/health", nil)
	if err != nil {
		return HealthResult{}, err
	}
	response, err := p.doer.Do(request)
	if err != nil {
		return HealthResult{}, mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapHTTPStatus(response.StatusCode); err != nil {
		return HealthResult{}, err
	}
	var wire healthResponseV1
	if err := decodeStrictJSONResponseContext(request.Context(), response, MaxHealthResponseBodyBytes, &wire); err != nil {
		return HealthResult{}, err
	}
	result, err := normalizeHealthResult(HealthResult{
		ProtocolVersion: wire.ProtocolVersion,
		Status:          wire.Status,
		Capabilities:    wire.Capabilities,
	})
	if err != nil {
		return HealthResult{}, domainsandbox.ErrProviderUnhealthy
	}
	return result, nil
}

func (p *RemoteProvider) Execute(ctx context.Context, input ExecuteRequest) (ExecuteResult, error) {
	return p.execute(ctx, input, false)
}

// Reconcile replays the exact canonical Execute request. It changes neither
// endpoint nor wire representation; only local freshness checks are relaxed.
func (p *RemoteProvider) Reconcile(ctx context.Context, input ExecuteRequest) (ExecuteResult, error) {
	result, err := p.execute(ctx, input, true)
	if err != nil && !IsAuthoritativeNotRecorded(err) {
		return ExecuteResult{}, executionOutcomeUnknown(err)
	}
	return result, err
}

func (p *RemoteProvider) LookupExecution(ctx context.Context, input ExecutionLookupRequest) (ExecutionLookupResult, error) {
	if p == nil || ctx == nil {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, err
	}
	normalized, err := NormalizeExecutionLookupRequest(input)
	if err != nil {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrInvalidInput
	}
	var body []byte
	if normalized.Legacy {
		body, err = json.Marshal(executionLookupLegacyRequestV1{
			Schema: ExecutionLookupLegacySchemaV1, Scope: normalized.Scope,
			WorkloadKind: normalized.WorkloadKind, SpaceID: normalized.SpaceID,
			ProjectID: normalized.ProjectID, Generation: normalized.Generation,
			OperationID: normalized.OperationID,
		})
	} else {
		body, err = json.Marshal(executionLookupRequestV1{
			Schema: ExecutionLookupSchemaV1, Scope: normalized.Scope,
			WorkloadKind: normalized.WorkloadKind, OperationID: normalized.OperationID,
			RequestDigest: normalized.RequestDigest.Hex(),
		})
	}
	if err != nil || len(body) > MaxRequestBodyBytes {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrInvalidInput
	}
	requestCtx, cancel := context.WithTimeout(ctx, remoteReconciliationRequestTimeout)
	defer cancel()
	requestCtx = safehttp.WithResponseBodyLimit(requestCtx, MaxExecutionResponseWireBytes)
	request, err := p.newRequest(requestCtx, http.MethodPost, "/v1/executions:lookup", body)
	if err != nil {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, err
	}
	response, err := p.doer.Do(request)
	if err != nil {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapHTTPStatus(response.StatusCode); err != nil {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrUnavailable
	}
	if response.Header.Get(ExecutionLookupProtocolVersionHeader) != ExecutionLookupProtocolVersionV1 {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrUnavailable
	}
	var wire executionLookupResponseV1
	if err := decodeStrictJSONResponseContext(
		request.Context(), response, MaxExecutionResponseWireBytes, &wire,
	); err != nil {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrUnavailable
	}
	if wire.Schema != ExecutionLookupSchemaV1 {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrUnavailable
	}
	result := ExecutionLookupResult{Status: wire.Status}
	if wire.Execution != nil {
		if wire.Execution.Schema != ExecuteSchemaV1 {
			return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrUnavailable
		}
		result.Execution = ExecuteResult{
			ExecutionID: wire.Execution.ExecutionID, Status: wire.Execution.Status,
			ExitCode: wire.Execution.ExitCode, Stdout: wire.Execution.Stdout, Stderr: wire.Execution.Stderr,
			Artifacts: wire.Execution.Artifacts, ArtifactDescriptors: wire.Execution.ArtifactDescriptors,
			PreviewRoute: wire.Execution.PreviewRoute,
		}
	}
	normalizedResult, err := NormalizeExecutionLookupResult(result, MaxCollectedOutputBytes)
	if err != nil {
		return ExecutionLookupResult{Status: ExecutionLookupUnknown}, domainsandbox.ErrUnavailable
	}
	return normalizedResult, nil
}

func (p *RemoteProvider) execute(ctx context.Context, input ExecuteRequest, reconciliation bool) (ExecuteResult, error) {
	if p == nil || ctx == nil {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return ExecuteResult{}, executionNotSubmitted(err)
	}
	body, normalized, err := canonicalExecuteRequest(input, reconciliation, timeNow())
	if err != nil {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	var executionContext context.Context
	var cancel context.CancelFunc
	if reconciliation {
		executionContext, cancel = context.WithTimeout(ctx, remoteReconciliationRequestTimeout)
	} else {
		executionContext, cancel = context.WithDeadline(ctx, normalized.Deadline)
	}
	defer cancel()
	executionContext = safehttp.WithResponseBodyLimit(executionContext, MaxExecutionResponseWireBytes)
	request, err := p.newRequest(executionContext, http.MethodPost, "/v1/executions", body)
	if err != nil {
		return ExecuteResult{}, err
	}
	if err := request.Context().Err(); err != nil {
		return ExecuteResult{}, executionNotSubmitted(err)
	}
	response, err := p.doer.Do(request)
	if err != nil {
		return ExecuteResult{}, executionOutcomeUnknown(mapProviderErrorWithContext(request.Context(), err))
	}
	defer response.Body.Close()
	if err := mapHTTPStatus(response.StatusCode); err != nil {
		if reconciliation {
			if response.StatusCode == http.StatusNotFound &&
				response.Header.Get(ReconciliationProtocolVersionHeader) == reconciliationProtocolVersionV1 {
				var authority struct {
					Code string `json:"code"`
				}
				if decodeErr := decodeStrictJSONResponseContext(
					request.Context(), response, MaxHealthResponseBodyBytes, &authority,
				); decodeErr != nil {
					return ExecuteResult{}, executionOutcomeUnknown(decodeErr)
				}
				if authority.Code == reconciliationNotRecordedCodeV1 {
					return ExecuteResult{}, newReconciliationNotRecordedErrorV1()
				}
			}
			return ExecuteResult{}, executionOutcomeUnknown(err)
		}
		if response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError {
			return ExecuteResult{}, executionNotSubmitted(err)
		}
		return ExecuteResult{}, executionOutcomeUnknown(err)
	}
	result, err := decodeExecutionResponseContext(request.Context(), response, normalized.Policy.MaxOutputBytes)
	if err != nil {
		return ExecuteResult{}, executionOutcomeUnknown(err)
	}
	return result, nil
}

func canonicalExecuteRequest(input ExecuteRequest, reconciliation bool, now time.Time) ([]byte, ExecuteRequest, error) {
	var normalized ExecuteRequest
	var err error
	if reconciliation {
		normalized, err = normalizeExecuteRequestForReconciliation(input, now)
	} else {
		normalized, err = normalizeExecuteRequest(input, now)
	}
	if err != nil {
		return nil, ExecuteRequest{}, domainsandbox.ErrInvalidInput
	}
	body, err := json.Marshal(executeRequestV1{
		Schema:         ExecuteSchemaV1,
		Scope:          normalized.Scope,
		WorkloadKind:   normalized.WorkloadKind,
		IdempotencyKey: normalized.IdempotencyKey,
		Deadline:       normalized.Deadline.Format(time.RFC3339Nano),
		Policy: runtimePolicyV1{
			TimeoutSeconds: normalized.Policy.TimeoutSeconds, MemoryLimitMB: normalized.Policy.MemoryLimitMB,
			CPULimit: normalized.Policy.CPULimit, MaxOutputBytes: normalized.Policy.MaxOutputBytes,
			MaxConcurrency: normalized.Policy.MaxConcurrency, AllowNetwork: normalized.Policy.AllowNetwork,
			NetworkAllowlist:        append([]string(nil), normalized.Policy.NetworkAllowlist...),
			AllowedEnvNames:         append([]string(nil), normalized.Policy.AllowedEnvNames...),
			VirtualReadPrefixes:     append([]string(nil), normalized.Policy.VirtualReadPrefixes...),
			VirtualWritePrefixes:    append([]string(nil), normalized.Policy.VirtualWritePrefixes...),
			AllowedExecutables:      append([]string(nil), normalized.Policy.AllowedExecutables...),
			FFIEnabled:              normalized.Policy.FFIEnabled,
			NodeModulesMode:         normalized.Policy.NodeModulesMode,
			NodeModulesDirectoryRef: normalized.Policy.NodeModulesDirectoryRef,
		},
		Entrypoint:         normalized.Entrypoint,
		Args:               append([]string(nil), normalized.Args...),
		Env:                cloneStringValues(normalized.Env),
		Stdin:              append([]byte(nil), normalized.Stdin...),
		Files:              append([]FileReference(nil), normalized.Files...),
		ArtifactReferences: artifactReferencesToWire(normalized.ArtifactReferences),
	})
	if err != nil || len(body) > MaxRequestBodyBytes {
		return nil, ExecuteRequest{}, domainsandbox.ErrInvalidInput
	}
	return body, normalized, nil
}

func (p *RemoteProvider) Status(ctx context.Context, executionID string) (ExecuteResult, error) {
	if p == nil || ctx == nil || !validExecutionID(executionID) {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return ExecuteResult{}, err
	}
	ctx = safehttp.WithResponseBodyLimit(ctx, MaxExecutionResponseWireBytes)
	request, err := p.newRequest(ctx, http.MethodGet, "/v1/executions/"+executionID, nil)
	if err != nil {
		return ExecuteResult{}, err
	}
	response, err := p.doer.Do(request)
	if err != nil {
		return ExecuteResult{}, mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapHTTPStatus(response.StatusCode); err != nil {
		return ExecuteResult{}, err
	}
	return decodeExecutionResponseContext(request.Context(), response, MaxCollectedOutputBytes)
}

func (p *RemoteProvider) KeepAlive(ctx context.Context, executionID string) error {
	if p == nil || ctx == nil || !validExecutionID(executionID) {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx = safehttp.WithResponseBodyLimit(ctx, MaxHealthResponseBodyBytes)
	request, err := p.newRequest(ctx, http.MethodPost, "/v1/executions/"+executionID+":keep-alive", []byte("{}"))
	if err != nil {
		return err
	}
	response, err := p.doer.Do(request)
	if err != nil {
		return mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapHTTPStatus(response.StatusCode); err != nil {
		return err
	}
	if err := validateCancelResponseContext(request.Context(), response); err != nil {
		return err
	}
	return nil
}

func (p *RemoteProvider) Cancel(ctx context.Context, executionID string) error {
	if p == nil || ctx == nil || !validExecutionID(executionID) {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx = safehttp.WithResponseBodyLimit(ctx, MaxHealthResponseBodyBytes)
	request, err := p.newRequest(ctx, http.MethodPost, "/v1/executions/"+executionID+":cancel", []byte("{}"))
	if err != nil {
		return err
	}
	response, err := p.doer.Do(request)
	if err != nil {
		return mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapHTTPStatus(response.StatusCode); err != nil {
		return err
	}
	if err := validateCancelResponseContext(request.Context(), response); err != nil {
		return err
	}
	return nil
}

func (p *RemoteProvider) BeginBuild(ctx context.Context, executionID, stableOperationID string) (BuildObservation, error) {
	if p == nil || ctx == nil || !validExecutionID(executionID) || !validBuildOperationID(stableOperationID) {
		return BuildObservation{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return BuildObservation{}, err
	}
	body, err := json.Marshal(appDevBuildRequestV1{Schema: AppDevBuildSchemaV1, OperationID: stableOperationID})
	if err != nil || len(body) > MaxRequestBodyBytes {
		return BuildObservation{}, domainsandbox.ErrInvalidInput
	}
	return p.appDevBuildRequest(ctx, http.MethodPost, "/v1/executions/"+executionID+"/builds", stableOperationID, body)
}

func (p *RemoteProvider) BuildStatus(ctx context.Context, executionID, stableOperationID string) (BuildObservation, error) {
	if p == nil || ctx == nil || !validExecutionID(executionID) || !validBuildOperationID(stableOperationID) {
		return BuildObservation{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return BuildObservation{}, err
	}
	return p.appDevBuildRequest(ctx, http.MethodGet, "/v1/executions/"+executionID+"/builds/"+stableOperationID, stableOperationID, nil)
}

func (p *RemoteProvider) appDevBuildRequest(ctx context.Context, method, requestPath, operationID string, body []byte) (BuildObservation, error) {
	requestContext := safehttp.WithResponseBodyLimit(ctx, MaxHealthResponseBodyBytes)
	request, err := p.newRequest(requestContext, method, requestPath, body)
	if err != nil {
		return BuildObservation{}, err
	}
	response, err := p.doer.Do(request)
	if err != nil {
		return BuildObservation{}, mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapHTTPStatus(response.StatusCode); err != nil {
		return BuildObservation{}, err
	}
	var wire appDevBuildResponseV1
	if err := decodeStrictJSONResponseContext(request.Context(), response, MaxHealthResponseBodyBytes, &wire); err != nil {
		return BuildObservation{}, err
	}
	if wire.Schema != AppDevBuildSchemaV1 || wire.OperationID != operationID {
		return BuildObservation{}, domainsandbox.ErrProviderUnhealthy
	}
	observation := BuildObservation{
		Status: wire.Status, SafeErrorCode: wire.SafeErrorCode,
	}
	if wire.Descriptor != nil {
		observation.Descriptor = *wire.Descriptor
	}
	normalized, err := NormalizeBuildObservation(observation)
	if err != nil {
		return BuildObservation{}, domainsandbox.ErrProviderUnhealthy
	}
	return normalized, nil
}

func (p *RemoteProvider) PublishArtifact(
	ctx context.Context,
	executionID string,
	input ArtifactPublishRequest,
) (ArtifactPublishResult, error) {
	if p == nil || ctx == nil || !validExecutionID(executionID) {
		return ArtifactPublishResult{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return ArtifactPublishResult{}, err
	}
	normalized, err := normalizeArtifactPublishRequest(input, timeNow())
	if err != nil {
		return ArtifactPublishResult{}, domainsandbox.ErrInvalidInput
	}
	body, err := json.Marshal(artifactPublishRequestV1{
		Schema: ArtifactPublishSchemaV1, Descriptor: normalized.Descriptor,
		UploadURL: normalized.UploadURL, Token: normalized.Token,
		ExpiresAt: normalized.ExpiresAt.Format(time.RFC3339Nano),
	})
	if err != nil || len(body) > MaxRequestBodyBytes {
		return ArtifactPublishResult{}, domainsandbox.ErrInvalidInput
	}
	requestContext := safehttp.WithResponseBodyLimit(ctx, MaxHealthResponseBodyBytes)
	request, err := p.newRequest(
		requestContext, http.MethodPost, "/v1/executions/"+executionID+"/artifacts:publish", body,
	)
	if err != nil {
		return ArtifactPublishResult{}, err
	}
	response, err := p.doer.Do(request)
	if err != nil {
		return ArtifactPublishResult{}, mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapHTTPStatus(response.StatusCode); err != nil {
		return ArtifactPublishResult{}, err
	}
	var wire artifactPublishResponseV1
	if err := decodeStrictJSONResponseContext(request.Context(), response, MaxHealthResponseBodyBytes, &wire); err != nil {
		return ArtifactPublishResult{}, err
	}
	if wire.Schema != ArtifactPublishSchemaV1 || !wire.Accepted {
		return ArtifactPublishResult{}, domainsandbox.ErrProviderUnhealthy
	}
	return ArtifactPublishResult{Accepted: true}, nil
}

func (p *RemoteProvider) newRequest(ctx context.Context, method, requestPath string, body []byte) (*http.Request, error) {
	if p == nil || p.endpoint == nil || p.doer == nil || ctx == nil || len(body) > MaxRequestBodyBytes {
		return nil, domainsandbox.ErrInvalidInput
	}
	request, err := http.NewRequestWithContext(ctx, method, p.endpoint.urlForPath(requestPath).String(), bytes.NewReader(body))
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	request.Header.Set("Authorization", "Bearer "+p.credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if err := p.endpoint.validateRequest(request); err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return request, nil
}

func mapHTTPStatus(status int) error {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return domainsandbox.ErrCredentialInvalid
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity || status == http.StatusRequestEntityTooLarge:
		return domainsandbox.ErrInvalidInput
	case status == http.StatusNotFound || status == http.StatusConflict:
		return domainsandbox.ErrExecutionForbidden
	case status == http.StatusTooManyRequests:
		return domainsandbox.ErrCapacityExhausted
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		return domainsandbox.ErrExecutionForbidden
	default:
		return domainsandbox.ErrProviderUnhealthy
	}
}

func decodeStrictJSONResponse(response *http.Response, limit int64, target any) error {
	return decodeStrictJSONResponseContext(context.Background(), response, limit, target)
}

func decodeStrictJSONResponseContext(ctx context.Context, response *http.Response, limit int64, target any) error {
	if response == nil || response.Body == nil || limit <= 0 || response.ContentLength > limit {
		return domainsandbox.ErrProviderUnhealthy
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return domainsandbox.ErrProviderUnhealthy
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return mapResponseBodyError(ctx, err)
	}
	if int64(len(data)) > limit || !utf8.Valid(data) {
		return domainsandbox.ErrProviderUnhealthy
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) < 2 {
		return domainsandbox.ErrProviderUnhealthy
	}
	if err := scanStrictJSONObject(trimmed); err != nil {
		return mapResponseBodyError(ctx, err)
	}
	if err := validateCanonicalJSONFields(trimmed, target); err != nil {
		return mapResponseBodyError(ctx, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return mapResponseBodyError(ctx, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return mapResponseBodyError(ctx, err)
	}
	return nil
}

func scanStrictJSONObject(data []byte) error {
	if !utf8.Valid(data) {
		return errRemoteJSONInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	opening, err := decoder.Token()
	delimiter, ok := opening.(json.Delim)
	if err != nil || !ok || delimiter != '{' {
		return errRemoteJSONInvalid
	}
	if err := scanJSONObjectContents(decoder, 1); err != nil {
		return errRemoteJSONInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errRemoteJSONInvalid
	}
	return nil
}

func scanJSONObjectContents(decoder *json.Decoder, depth int) error {
	if depth > maxRemoteJSONDepth {
		return errRemoteJSONInvalid
	}
	keys := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return errRemoteJSONInvalid
		}
		if _, duplicate := keys[key]; duplicate {
			return errRemoteJSONInvalid
		}
		keys[key] = struct{}{}
		if err := scanRemoteJSONValue(decoder, depth+1); err != nil {
			return errRemoteJSONInvalid
		}
	}
	closing, err := decoder.Token()
	delimiter, ok := closing.(json.Delim)
	if err != nil || !ok || delimiter != '}' {
		return errRemoteJSONInvalid
	}
	return nil
}

func scanRemoteJSONValue(decoder *json.Decoder, depth int) error {
	if depth > maxRemoteJSONDepth {
		return errRemoteJSONInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return errRemoteJSONInvalid
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	switch delimiter {
	case '{':
		return scanJSONObjectContents(decoder, depth)
	case '[':
		for decoder.More() {
			if err := scanRemoteJSONValue(decoder, depth+1); err != nil {
				return errRemoteJSONInvalid
			}
		}
		closing, err := decoder.Token()
		closingDelimiter, ok := closing.(json.Delim)
		if err != nil || !ok || closingDelimiter != ']' {
			return errRemoteJSONInvalid
		}
		return nil
	default:
		return errRemoteJSONInvalid
	}
}

func validateCanonicalJSONFields(body []byte, target any) error {
	if target == nil {
		return errRemoteJSONInvalid
	}
	return validateCanonicalJSONValue(json.RawMessage(body), reflect.TypeOf(target))
}

func validateCanonicalJSONValue(raw json.RawMessage, targetType reflect.Type) error {
	for targetType.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		targetType = targetType.Elem()
	}

	switch targetType.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil || object == nil {
			return errRemoteJSONInvalid
		}
		fields := make(map[string]reflect.Type, targetType.NumField())
		for index := 0; index < targetType.NumField(); index++ {
			field := targetType.Field(index)
			if field.PkgPath != "" {
				continue
			}
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
		for name, value := range object {
			fieldType, exists := fields[name]
			if !exists || validateCanonicalJSONValue(value, fieldType) != nil {
				return errRemoteJSONInvalid
			}
		}
	case reflect.Slice, reflect.Array:
		if targetType.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return errRemoteJSONInvalid
		}
		for _, value := range values {
			if validateCanonicalJSONValue(value, targetType.Elem()) != nil {
				return errRemoteJSONInvalid
			}
		}
	case reflect.Map:
		if targetType.Key().Kind() != reflect.String {
			return nil
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return errRemoteJSONInvalid
		}
		for _, value := range values {
			if validateCanonicalJSONValue(value, targetType.Elem()) != nil {
				return errRemoteJSONInvalid
			}
		}
	}
	return nil
}

func validateCancelResponse(response *http.Response) error {
	return validateCancelResponseContext(context.Background(), response)
}

func validateCancelResponseContext(ctx context.Context, response *http.Response) error {
	if response == nil || response.Body == nil || response.ContentLength > MaxHealthResponseBodyBytes {
		return domainsandbox.ErrProviderUnhealthy
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxHealthResponseBodyBytes+1))
	if err != nil {
		return mapResponseBodyError(ctx, err)
	}
	if len(data) > MaxHealthResponseBodyBytes {
		return domainsandbox.ErrProviderUnhealthy
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
	var empty struct{}
	return decodeStrictJSONResponseContext(ctx, response, MaxHealthResponseBodyBytes, &empty)
}

func decodeExecutionResponse(response *http.Response, maxOutputBytes int64) (ExecuteResult, error) {
	return decodeExecutionResponseContext(context.Background(), response, maxOutputBytes)
}

func decodeExecutionResponseContext(ctx context.Context, response *http.Response, maxOutputBytes int64) (ExecuteResult, error) {
	var wire executeResponseV1
	if err := decodeStrictJSONResponseContext(ctx, response, MaxExecutionResponseWireBytes, &wire); err != nil {
		return ExecuteResult{}, err
	}
	if wire.Schema != ExecuteSchemaV1 {
		return ExecuteResult{}, domainsandbox.ErrProviderUnhealthy
	}
	result, err := NormalizeExecuteResult(ExecuteResult{
		ExecutionID: wire.ExecutionID, Status: wire.Status, ExitCode: wire.ExitCode,
		Stdout: wire.Stdout, Stderr: wire.Stderr, Artifacts: wire.Artifacts,
		ArtifactDescriptors: wire.ArtifactDescriptors,
		PreviewRoute:        wire.PreviewRoute,
	}, maxOutputBytes)
	if err != nil {
		return ExecuteResult{}, domainsandbox.ErrProviderUnhealthy
	}
	return result, nil
}

func mapResponseBodyError(ctx context.Context, err error) error {
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return domainsandbox.ErrProviderUnhealthy
}

func artifactReferencesToWire(references []ArtifactReference) []artifactReferenceV1 {
	wire := make([]artifactReferenceV1, len(references))
	for index, reference := range references {
		wire[index] = artifactReferenceV1{
			Direction: reference.Direction, URL: reference.URL,
			Digest: reference.Digest, Size: reference.Size, MediaType: reference.MediaType,
			ExpiresAt: reference.ExpiresAt.Format(time.RFC3339Nano),
		}
	}
	return wire
}

func cloneStringValues(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

var timeNow = time.Now
