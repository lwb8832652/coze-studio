// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	MaxRequestBodyBytes        = 2 * 1024 * 1024
	MaxHealthResponseBodyBytes = 64 * 1024
	// MaxExecutionResponseWireBytes leaves bounded room for worst-case JSON
	// escaping of valid collected output plus the response envelope.
	MaxExecutionResponseWireBytes = 32 * 1024 * 1024
	// MaxExecutionResponseBodyBytes is retained for source compatibility.
	// Deprecated: use MaxExecutionResponseWireBytes.
	MaxExecutionResponseBodyBytes        = MaxExecutionResponseWireBytes
	MaxArgs                              = 128
	MaxArgumentBytes                     = 4 * 1024
	MaxEnvVars                           = 128
	MaxEnvKeyBytes                       = 128
	MaxEnvValueBytes                     = 8 * 1024
	MaxStdinBytes                        = 1024 * 1024
	MaxFiles                             = 256
	MaxArtifacts                         = 256
	MaxIdentifierBytes                   = 128
	MaxLogicalPathBytes                  = 1024
	MaxDigestBytes                       = 71
	MaxArtifactNameBytes                 = 255
	MaxMediaTypeBytes                    = 255
	MaxArtifactReferenceURLBytes         = 4096
	MaxArtifactReferenceTokenBytes       = 4096
	MaxOutputStreamBytes                 = 2 * 1024 * 1024
	MaxCollectedOutputBytes              = 4 * 1024 * 1024
	MaxLogicalFileSizeBytes        int64 = 1 << 40
	MaxExecutionDeadlineAhead            = time.Hour
	MaxProviderCredentialBytes           = 64 * 1024

	HealthProtocolV1                    = "v1"
	ExecuteSchemaV1                     = "coze.sandbox.execute.v1"
	ArtifactPublishSchemaV1             = "coze.sandbox.artifact_publish.v1"
	AppDevBuildSchemaV1                 = "coze.sandbox.appdev_build.v1"
	MCPStdioInvokeSchemaV1              = "coze.sandbox.mcp_stdio.invoke.v1"
	MCPStdioResultSchemaV1              = "coze.sandbox.mcp_stdio.result.v1"
	MCPStdioClientVersionV1             = "1"
	ExecutionLookupSchemaV1             = "coze.sandbox.execution_lookup.v1"
	ExecutionLookupLegacySchemaV1       = "coze.sandbox.execution_lookup.legacy.v1"
	MaxArtifactDescriptorBytes    int64 = 100 * 1024 * 1024
)

const maxMCPStdioToolContentItems = 64

type WorkloadKind string

const (
	WorkloadAgent    WorkloadKind = "agent"
	WorkloadMCPStdio WorkloadKind = "mcp_stdio"
	WorkloadAppDev   WorkloadKind = "appdev"
)

type MCPStdioInvokeEnvelope struct {
	Schema        string          `json:"schema"`
	OperationID   string          `json:"operation_id"`
	Command       string          `json:"command"`
	Args          []string        `json:"args"`
	ToolName      string          `json:"tool_name"`
	Arguments     json.RawMessage `json:"arguments"`
	WorkingDir    string          `json:"working_dir"`
	EnvNames      []string        `json:"env_names"`
	ClientVersion string          `json:"client_version"`
}

type MCPStdioToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPStdioResultEnvelope struct {
	Schema  string                `json:"schema"`
	Content []MCPStdioToolContent `json:"content"`
	IsError bool                  `json:"is_error"`
}

type MCPStdioToolResult struct {
	Content []MCPStdioToolContent `json:"content"`
	IsError bool                  `json:"is_error"`
}

func NormalizeMCPStdioInvokeEnvelope(input MCPStdioInvokeEnvelope) (MCPStdioInvokeEnvelope, error) {
	if input.Schema != MCPStdioInvokeSchemaV1 || input.ClientVersion != MCPStdioClientVersionV1 ||
		!validIdentifier(input.OperationID) || !validMCPStdioLogicalExecutable(input.Command) ||
		!validIdentifier(input.ToolName) || !validLogicalPath(input.WorkingDir) ||
		len(input.Args) > MaxArgs || len(input.EnvNames) > MaxEnvVars {
		return MCPStdioInvokeEnvelope{}, domainsandbox.ErrInvalidInput
	}
	args := make([]string, len(input.Args))
	totalArgBytes := 0
	for index, argument := range input.Args {
		if !validBoundedText(argument, MaxArgumentBytes, false) {
			return MCPStdioInvokeEnvelope{}, domainsandbox.ErrInvalidInput
		}
		totalArgBytes += len([]byte(argument))
		if totalArgBytes > MaxArgs*MaxArgumentBytes {
			return MCPStdioInvokeEnvelope{}, domainsandbox.ErrInvalidInput
		}
		args[index] = argument
	}
	rawArguments := bytes.TrimSpace(input.Arguments)
	if len(rawArguments) < 2 || len(rawArguments) > MaxStdinBytes ||
		scanStrictJSONObject(rawArguments) != nil {
		return MCPStdioInvokeEnvelope{}, domainsandbox.ErrInvalidInput
	}
	var argumentFields map[string]json.RawMessage
	if err := json.Unmarshal(rawArguments, &argumentFields); err != nil || argumentFields == nil {
		return MCPStdioInvokeEnvelope{}, domainsandbox.ErrInvalidInput
	}
	canonicalArguments, err := json.Marshal(argumentFields)
	if err != nil || len(canonicalArguments) > MaxStdinBytes {
		return MCPStdioInvokeEnvelope{}, domainsandbox.ErrInvalidInput
	}
	envSet := make(map[string]struct{}, len(input.EnvNames))
	for _, name := range input.EnvNames {
		if !validEnvironmentKey(name) {
			return MCPStdioInvokeEnvelope{}, domainsandbox.ErrInvalidInput
		}
		envSet[name] = struct{}{}
	}
	envNames := make([]string, 0, len(envSet))
	for name := range envSet {
		envNames = append(envNames, name)
	}
	sort.Strings(envNames)
	return MCPStdioInvokeEnvelope{
		Schema:        MCPStdioInvokeSchemaV1,
		OperationID:   input.OperationID,
		Command:       input.Command,
		Args:          args,
		ToolName:      input.ToolName,
		Arguments:     append(json.RawMessage(nil), canonicalArguments...),
		WorkingDir:    input.WorkingDir,
		EnvNames:      envNames,
		ClientVersion: MCPStdioClientVersionV1,
	}, nil
}

func MarshalMCPStdioInvokeEnvelope(input MCPStdioInvokeEnvelope) ([]byte, error) {
	normalized, err := NormalizeMCPStdioInvokeEnvelope(input)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	body, err := json.Marshal(normalized)
	if err != nil || len(body) > MaxStdinBytes {
		return nil, domainsandbox.ErrInvalidInput
	}
	return body, nil
}

func ParseMCPStdioResultEnvelope(body []byte, maxBytes int) (MCPStdioToolResult, error) {
	if maxBytes <= 0 || len(body) == 0 || len(body) > maxBytes || !utf8.Valid(body) {
		return MCPStdioToolResult{}, domainsandbox.ErrInvalidInput
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) < 2 || scanStrictJSONObject(trimmed) != nil {
		return MCPStdioToolResult{}, domainsandbox.ErrInvalidInput
	}
	var wire MCPStdioResultEnvelope
	if validateCanonicalJSONFields(trimmed, &wire) != nil {
		return MCPStdioToolResult{}, domainsandbox.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil || wire.Schema != MCPStdioResultSchemaV1 ||
		len(wire.Content) > maxMCPStdioToolContentItems {
		return MCPStdioToolResult{}, domainsandbox.ErrInvalidInput
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return MCPStdioToolResult{}, domainsandbox.ErrInvalidInput
	}
	content := make([]MCPStdioToolContent, len(wire.Content))
	totalTextBytes := 0
	for index, item := range wire.Content {
		totalTextBytes += len([]byte(item.Text))
		if item.Type != "text" || !validBoundedText(item.Text, maxBytes, true) ||
			totalTextBytes > maxBytes {
			return MCPStdioToolResult{}, domainsandbox.ErrInvalidInput
		}
		content[index] = MCPStdioToolContent{Type: "text", Text: item.Text}
	}
	result := MCPStdioToolResult{Content: content, IsError: wire.IsError}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > maxBytes {
		return MCPStdioToolResult{}, domainsandbox.ErrInvalidInput
	}
	return result, nil
}

func validMCPStdioLogicalExecutable(value string) bool {
	if value == "" || len(value) > MaxIdentifierBytes || !utf8.ValidString(value) ||
		path.IsAbs(value) || strings.ContainsAny(value, `/\:`) {
		return false
	}
	switch strings.ToLower(value) {
	case "sh", "bash", "dash", "zsh", "fish", "ksh", "csh", "tcsh":
		return false
	}
	for index := range value {
		character := value[index]
		if index == 0 && !isIdentifierAlphaNumeric(character) {
			return false
		}
		if !isIdentifierAlphaNumeric(character) && character != '_' && character != '-' &&
			character != '.' && character != '+' {
			return false
		}
	}
	return true
}

type ExecutionStatus string

const (
	ExecutionStatusAccepted  ExecutionStatus = "accepted"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusSucceeded ExecutionStatus = "succeeded"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCanceled  ExecutionStatus = "canceled"
	ExecutionStatusTimedOut  ExecutionStatus = "timed_out"
)

type RuntimeProvider interface {
	Health(ctx context.Context) (HealthResult, error)
	// Execute must return a terminal result unless the implementation also
	// implements AsyncRuntimeProvider. A synchronous implementation must not
	// create a detached execution and return accepted or running.
	Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error)
	Cancel(ctx context.Context, executionID string) error
	// CloseContext must be idempotent and must return when ctx is canceled.
	// Providers that cannot honor cancellation do not satisfy this contract.
	CloseContext(ctx context.Context) error
}

// AsyncRuntimeProvider is an optional capability for providers whose Execute
// call returns accepted or running. Synchronous RuntimeProvider implementations
// remain source-compatible and do not need to implement it.
type AsyncRuntimeProvider interface {
	Status(ctx context.Context, executionID string) (ExecuteResult, error)
	KeepAlive(ctx context.Context, executionID string) error
}

// ExecutionReconciler resolves an ambiguous asynchronous Execute submission by
// replaying the exact canonical request and idempotency key. Implementations
// must use the same endpoint and wire body. The server must consult its
// idempotency record before temporal validation or artifact consumption, so an
// expired original request can recover an already accepted execution without
// consuming the artifact again.
type ExecutionReconciler interface {
	Reconcile(ctx context.Context, request ExecuteRequest) (ExecuteResult, error)
}

// ExecutionLookupProvider is the restart-safe submission reconciliation
// capability. Unlike ExecutionReconciler it never replays Execute or consumes
// an artifact capability: it looks up a provider-side idempotency record by the
// immutable operation identity and canonical request digest.
type ExecutionLookupProvider interface {
	LookupExecution(context.Context, ExecutionLookupRequest) (ExecutionLookupResult, error)
}

type ExecutionRequestDigest [sha256.Size]byte

func (digest ExecutionRequestDigest) IsZero() bool {
	var zero ExecutionRequestDigest
	return subtle.ConstantTimeCompare(digest[:], zero[:]) == 1
}

func (digest ExecutionRequestDigest) Equal(other ExecutionRequestDigest) bool {
	return subtle.ConstantTimeCompare(digest[:], other[:]) == 1
}

func (digest ExecutionRequestDigest) Hex() string {
	return hex.EncodeToString(digest[:])
}

func (digest ExecutionRequestDigest) Bytes() []byte {
	return append([]byte(nil), digest[:]...)
}

func ExecutionRequestDigestFromBytes(value []byte) (ExecutionRequestDigest, error) {
	var digest ExecutionRequestDigest
	if len(value) != sha256.Size {
		return digest, domainsandbox.ErrInvalidInput
	}
	copy(digest[:], value)
	if digest.IsZero() {
		return ExecutionRequestDigest{}, domainsandbox.ErrInvalidInput
	}
	return digest, nil
}

func (ExecutionRequestDigest) String() string   { return "ExecutionRequestDigest{value:<redacted>}" }
func (ExecutionRequestDigest) GoString() string { return "ExecutionRequestDigest{value:<redacted>}" }
func (ExecutionRequestDigest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ExecutionRequestDigest{value:<redacted>}")
}

type ExecutionLookupStatus string

const (
	ExecutionLookupFound    ExecutionLookupStatus = "found"
	ExecutionLookupNotFound ExecutionLookupStatus = "not_found"
	ExecutionLookupUnknown  ExecutionLookupStatus = "unknown"
)

type ExecutionLookupRequest struct {
	Scope         domainsandbox.Scope
	WorkloadKind  WorkloadKind
	OperationID   string
	RequestDigest ExecutionRequestDigest
	Legacy        bool
	SpaceID       string
	ProjectID     string
	Generation    uint64
}

func (ExecutionLookupRequest) String() string {
	return "ExecutionLookupRequest{identity:<redacted>}"
}
func (ExecutionLookupRequest) GoString() string {
	return "ExecutionLookupRequest{identity:<redacted>}"
}
func (ExecutionLookupRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ExecutionLookupRequest{identity:<redacted>}")
}

type ExecutionLookupResult struct {
	Status    ExecutionLookupStatus
	Execution ExecuteResult
}

func NormalizeExecutionLookupRequest(input ExecutionLookupRequest) (ExecutionLookupRequest, error) {
	if !scopeMatchesWorkload(input.Scope, input.WorkloadKind) || !validIdentifier(input.OperationID) {
		return ExecutionLookupRequest{}, domainsandbox.ErrInvalidInput
	}
	if input.Legacy {
		spaceID, err := strconv.ParseUint(input.SpaceID, 10, 63)
		if err != nil || spaceID == 0 || strconv.FormatUint(spaceID, 10) != input.SpaceID ||
			!validIdentifier(input.ProjectID) || input.Generation == 0 ||
			!input.RequestDigest.IsZero() || input.WorkloadKind != WorkloadAppDev {
			return ExecutionLookupRequest{}, domainsandbox.ErrInvalidInput
		}
		return input, nil
	}
	if input.SpaceID != "" || input.ProjectID != "" || input.Generation != 0 ||
		input.RequestDigest.IsZero() {
		return ExecutionLookupRequest{}, domainsandbox.ErrInvalidInput
	}
	return input, nil
}

func NormalizeExecutionLookupResult(input ExecutionLookupResult, maxOutputBytes int64) (ExecutionLookupResult, error) {
	switch input.Status {
	case ExecutionLookupFound:
		execution, err := NormalizeExecuteResult(input.Execution, maxOutputBytes)
		if err != nil {
			return ExecutionLookupResult{}, domainsandbox.ErrInvalidInput
		}
		return ExecutionLookupResult{Status: ExecutionLookupFound, Execution: execution}, nil
	case ExecutionLookupNotFound, ExecutionLookupUnknown:
		if input.Execution.ExecutionID != "" || input.Execution.Status != "" ||
			input.Execution.ExitCode != nil || input.Execution.Stdout != "" ||
			input.Execution.Stderr != "" || len(input.Execution.Artifacts) != 0 ||
			len(input.Execution.ArtifactDescriptors) != 0 || input.Execution.PreviewRoute != "" {
			return ExecutionLookupResult{}, domainsandbox.ErrInvalidInput
		}
		return ExecutionLookupResult{Status: input.Status}, nil
	default:
		return ExecutionLookupResult{}, domainsandbox.ErrInvalidInput
	}
}

// DigestExecuteRequest hashes the exact canonical Execute wire envelope,
// including its immutable deadline and artifact capability. Only the digest is
// durable; plaintext capability fields remain process-local.
func DigestExecuteRequest(input ExecuteRequest) (ExecutionRequestDigest, error) {
	body, _, err := canonicalExecuteRequest(input, false, timeNow())
	if err != nil {
		return ExecutionRequestDigest{}, err
	}
	return ExecutionRequestDigest(sha256.Sum256(body)), nil
}

// ArtifactPublisher is an optional authenticated provider control capability.
// The upload capability is short-lived and must never be persisted or logged.
type ArtifactPublisher interface {
	PublishArtifact(ctx context.Context, executionID string, request ArtifactPublishRequest) (ArtifactPublishResult, error)
}

// AppDevBuildExecutor is an optional authenticated control capability. Build
// observations may only carry a safe descriptor; provider object keys and URIs
// are never accepted by this contract.
type AppDevBuildExecutor interface {
	BeginBuild(ctx context.Context, executionID, stableOperationID string) (BuildObservation, error)
	BuildStatus(ctx context.Context, executionID, stableOperationID string) (BuildObservation, error)
}

type LocalExecutionDelegate interface {
	Health(ctx context.Context) (HealthResult, error)
	Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error)
	Cancel(ctx context.Context, executionID string) error
}

type HealthResult struct {
	ProtocolVersion string
	Status          domainsandbox.HealthStatus
	Capabilities    []domainsandbox.Scope
}

type FileReference struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

type ArtifactDirection string

const (
	ArtifactDirectionDownload ArtifactDirection = "download"
	ArtifactDirectionUpload   ArtifactDirection = "upload"
)

// ArtifactReference is an internal provider transport capability. Its default
// JSON representation is deliberately empty so grant URLs and bearer tokens
// cannot accidentally enter public DTOs, logs, or audit payloads. Remote wire
// serialization is performed explicitly by RemoteProvider.
type ArtifactReference struct {
	Direction ArtifactDirection `json:"-"`
	URL       string            `json:"-"`
	Token     string            `json:"-"`
	Digest    string            `json:"-"`
	Size      int64             `json:"-"`
	MediaType string            `json:"-"`
	ExpiresAt time.Time         `json:"-"`
}

func (r ArtifactReference) redactedString() string {
	return "ArtifactReference{Direction:" + strconv.Quote(string(r.Direction)) +
		" URL:<redacted> Token:<redacted> Digest:" + strconv.Quote(r.Digest) +
		" Size:" + strconv.FormatInt(r.Size, 10) + " MediaType:" + strconv.Quote(r.MediaType) +
		" ExpiresAt:" + strconv.Quote(r.ExpiresAt.UTC().Format(time.RFC3339Nano)) + "}"
}

func (r ArtifactReference) String() string   { return r.redactedString() }
func (r ArtifactReference) GoString() string { return r.redactedString() }

func (r ArtifactReference) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, r.redactedString())
}

type ExecuteRequest struct {
	Scope              domainsandbox.Scope
	WorkloadKind       WorkloadKind
	IdempotencyKey     string
	Deadline           time.Time
	Policy             domainsandbox.RuntimePolicy
	Entrypoint         string
	Args               []string
	Env                map[string]string
	Stdin              []byte
	Files              []FileReference
	ArtifactReferences []ArtifactReference `json:"-"`
}

type ArtifactSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
}

type ArtifactKind string

const ArtifactKindAppDevBuildArchive ArtifactKind = "appdev_build_archive"

// ArtifactDescriptor is safe provider metadata. It deliberately contains no
// object key, URI, provider execution identity, or bearer capability.
type ArtifactDescriptor struct {
	Kind   ArtifactKind `json:"kind"`
	Digest string       `json:"digest"`
	Size   int64        `json:"size"`
}

func (d ArtifactDescriptor) String() string {
	return fmt.Sprintf("ArtifactDescriptor{Kind:%q Digest:%q Size:%d}", d.Kind, d.Digest, d.Size)
}
func (d ArtifactDescriptor) GoString() string { return d.String() }
func (d ArtifactDescriptor) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, d.String())
}

func NormalizeArtifactDescriptor(input ArtifactDescriptor) (ArtifactDescriptor, error) {
	if input.Kind != ArtifactKindAppDevBuildArchive || !validDigest(input.Digest) ||
		strings.Trim(input.Digest[len("sha256:"):], "0") == "" ||
		input.Size <= 0 || input.Size > MaxArtifactDescriptorBytes {
		return ArtifactDescriptor{}, domainsandbox.ErrInvalidInput
	}
	return input, nil
}

type BuildStatus string

const (
	BuildStatusAccepted        BuildStatus = "accepted"
	BuildStatusRunning         BuildStatus = "running"
	BuildStatusDescriptorReady BuildStatus = "descriptor_ready"
	BuildStatusFailed          BuildStatus = "failed"
)

type BuildObservation struct {
	Status           BuildStatus        `json:"status"`
	Descriptor       ArtifactDescriptor `json:"descriptor,omitempty"`
	SafeErrorCode    string             `json:"safe_error_code,omitempty"`
	SafeErrorMessage string             `json:"safe_error_message,omitempty"`
}

const (
	BuildSafeErrorCodeBuildFailed               = "build_failed"
	BuildSafeErrorCodeSourceInvalid             = "source_invalid"
	BuildSafeErrorCodeDependencyFailed          = "dependency_failed"
	BuildSafeErrorCodeProviderUnavailable       = "provider_unavailable"
	BuildSafeErrorCodeProviderCapabilityMissing = "provider_capability_missing"
	BuildSafeErrorCodeProviderContractViolation = "provider_contract_violation"
	BuildSafeErrorCodeBuildTimedOut             = "build_timed_out"
	BuildSafeErrorCodeBuildCanceled             = "build_canceled"
	BuildSafeErrorCodeArtifactInvalid           = "artifact_invalid"

	BuildSafeErrorMessageBuildFailed               = "Build failed. Please try again."
	BuildSafeErrorMessageSourceInvalid             = "The project source could not be built."
	BuildSafeErrorMessageDependencyFailed          = "A project dependency could not be built."
	BuildSafeErrorMessageProviderUnavailable       = "The build service is temporarily unavailable."
	BuildSafeErrorMessageProviderCapabilityMissing = "The build service does not support this operation."
	BuildSafeErrorMessageProviderContractViolation = "The build service returned an invalid result."
	BuildSafeErrorMessageBuildTimedOut             = "The build timed out. Please try again."
	BuildSafeErrorMessageBuildCanceled             = "The build was canceled."
	BuildSafeErrorMessageArtifactInvalid           = "The build artifact could not be verified."
)

// NormalizeBuildSafeError maps an untrusted provider code to a server-owned
// public code and fixed message. Provider-authored messages are never trusted.
func NormalizeBuildSafeError(code string) (string, string) {
	switch code {
	case BuildSafeErrorCodeSourceInvalid:
		return code, BuildSafeErrorMessageSourceInvalid
	case BuildSafeErrorCodeDependencyFailed:
		return code, BuildSafeErrorMessageDependencyFailed
	case BuildSafeErrorCodeProviderUnavailable:
		return code, BuildSafeErrorMessageProviderUnavailable
	case BuildSafeErrorCodeProviderCapabilityMissing:
		return code, BuildSafeErrorMessageProviderCapabilityMissing
	case BuildSafeErrorCodeProviderContractViolation:
		return code, BuildSafeErrorMessageProviderContractViolation
	case BuildSafeErrorCodeBuildTimedOut:
		return code, BuildSafeErrorMessageBuildTimedOut
	case BuildSafeErrorCodeBuildCanceled:
		return code, BuildSafeErrorMessageBuildCanceled
	case BuildSafeErrorCodeArtifactInvalid:
		return code, BuildSafeErrorMessageArtifactInvalid
	case BuildSafeErrorCodeBuildFailed:
		return code, BuildSafeErrorMessageBuildFailed
	default:
		return BuildSafeErrorCodeBuildFailed, BuildSafeErrorMessageBuildFailed
	}
}

func (observation BuildObservation) String() string {
	return fmt.Sprintf("BuildObservation{Status:%q Descriptor:%v SafeError:<redacted:%t>}",
		observation.Status, observation.Descriptor, observation.SafeErrorCode != "" || observation.SafeErrorMessage != "")
}

func (observation BuildObservation) GoString() string { return observation.String() }

func (observation BuildObservation) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, observation.String())
}

func NormalizeBuildObservation(input BuildObservation) (BuildObservation, error) {
	if input.Status == BuildStatusFailed {
		input.SafeErrorCode, input.SafeErrorMessage = NormalizeBuildSafeError(input.SafeErrorCode)
	}
	descriptorPresent := input.Descriptor != (ArtifactDescriptor{})
	safeErrorPresent := input.SafeErrorCode != "" || input.SafeErrorMessage != ""
	switch input.Status {
	case BuildStatusAccepted, BuildStatusRunning:
		if descriptorPresent || safeErrorPresent {
			return BuildObservation{}, domainsandbox.ErrInvalidInput
		}
	case BuildStatusDescriptorReady:
		descriptor, err := NormalizeArtifactDescriptor(input.Descriptor)
		if err != nil || safeErrorPresent {
			return BuildObservation{}, domainsandbox.ErrInvalidInput
		}
		input.Descriptor = descriptor
	case BuildStatusFailed:
		if descriptorPresent || !validBuildSafeError(input.SafeErrorCode, input.SafeErrorMessage) {
			return BuildObservation{}, domainsandbox.ErrInvalidInput
		}
	default:
		return BuildObservation{}, domainsandbox.ErrInvalidInput
	}
	return input, nil
}

func validBuildSafeError(code, message string) bool {
	return code != "" && len(code) <= 64 && utf8.ValidString(code) &&
		!strings.ContainsAny(code, "\x00\r\n\t ") && len(message) <= 255 && utf8.ValidString(message) &&
		!strings.ContainsAny(message, "\x00\r\n\t")
}

func validBuildOperationID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= MaxIdentifierBytes &&
		value != "." && value != ".." && utf8.ValidString(value) && !strings.ContainsAny(value, "/\\?#%\x00\r\n\t")
}

var errArtifactPublishSecret = errors.New("sandbox artifact publish capability cannot be serialized")

type ArtifactPublishRequest struct {
	Descriptor ArtifactDescriptor `json:"-"`
	UploadURL  string             `json:"-"`
	Token      string             `json:"-"`
	ExpiresAt  time.Time          `json:"-"`
}

func (ArtifactPublishRequest) String() string { return "ArtifactPublishRequest{capability:<redacted>}" }
func (ArtifactPublishRequest) GoString() string {
	return "ArtifactPublishRequest{capability:<redacted>}"
}
func (ArtifactPublishRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ArtifactPublishRequest{capability:<redacted>}")
}
func (ArtifactPublishRequest) MarshalJSON() ([]byte, error) { return nil, errArtifactPublishSecret }

type ArtifactPublishResult struct {
	Accepted bool `json:"accepted"`
}

type ExecuteResult struct {
	ExecutionID         string
	Status              ExecutionStatus
	ExitCode            *int
	Stdout              string
	Stderr              string
	Artifacts           []ArtifactSummary
	ArtifactDescriptors []ArtifactDescriptor `json:"-"`
	PreviewRoute        string
}

func normalizeExecuteRequest(input ExecuteRequest, now time.Time) (ExecuteRequest, error) {
	return normalizeExecuteRequestWithTemporalMode(input, now, false)
}

func normalizeExecuteRequestForReconciliation(input ExecuteRequest, now time.Time) (ExecuteRequest, error) {
	return normalizeExecuteRequestWithTemporalMode(input, now, true)
}

func normalizeExecuteRequestWithTemporalMode(input ExecuteRequest, now time.Time, allowExpired bool) (ExecuteRequest, error) {
	if !scopeMatchesWorkloadEntrypoint(input.Scope, input.WorkloadKind, input.Entrypoint) ||
		!validIdentifier(input.IdempotencyKey) ||
		input.Deadline.IsZero() || (!allowExpired && !input.Deadline.After(now)) || input.Deadline.After(now.Add(MaxExecutionDeadlineAhead)) ||
		!validLogicalPath(input.Entrypoint) || len(input.Args) > MaxArgs || len(input.Env) > MaxEnvVars ||
		len(input.Stdin) > MaxStdinBytes || len(input.Files) > MaxFiles || len(input.ArtifactReferences) > MaxArtifacts {
		return ExecuteRequest{}, domainsandbox.ErrInvalidInput
	}
	policy, err := domainsandbox.NormalizeRuntimePolicy(input.Policy)
	if err != nil {
		return ExecuteRequest{}, domainsandbox.ErrInvalidInput
	}
	args := make([]string, len(input.Args))
	for index, argument := range input.Args {
		if !validBoundedText(argument, MaxArgumentBytes, false) {
			return ExecuteRequest{}, domainsandbox.ErrInvalidInput
		}
		args[index] = argument
	}
	environment := make(map[string]string, len(input.Env))
	for key, value := range input.Env {
		if !validEnvironmentKey(key) || !validBoundedText(value, MaxEnvValueBytes, false) {
			return ExecuteRequest{}, domainsandbox.ErrInvalidInput
		}
		environment[key] = value
	}
	files := make([]FileReference, len(input.Files))
	fileIDs := make(map[string]struct{}, len(input.Files))
	filePaths := make(map[string]struct{}, len(input.Files))
	for index, file := range input.Files {
		if !validIdentifier(file.ID) || !validLogicalPath(file.Path) || !validDigest(file.Digest) ||
			file.Size < 0 || file.Size > MaxLogicalFileSizeBytes {
			return ExecuteRequest{}, domainsandbox.ErrInvalidInput
		}
		if _, duplicate := fileIDs[file.ID]; duplicate {
			return ExecuteRequest{}, domainsandbox.ErrInvalidInput
		}
		if _, duplicate := filePaths[file.Path]; duplicate {
			return ExecuteRequest{}, domainsandbox.ErrInvalidInput
		}
		fileIDs[file.ID] = struct{}{}
		filePaths[file.Path] = struct{}{}
		files[index] = file
	}
	artifactReferences := make([]ArtifactReference, len(input.ArtifactReferences))
	for index, reference := range input.ArtifactReferences {
		if !validArtifactDirection(reference.Direction) || !validArtifactReferenceURL(reference.URL) ||
			len(reference.Token) > MaxArtifactReferenceTokenBytes || !validCredential(reference.Token) ||
			!validDigest(reference.Digest) || reference.Size < 0 || reference.Size > MaxLogicalFileSizeBytes ||
			!validMediaType(reference.MediaType) || reference.ExpiresAt.IsZero() ||
			(!allowExpired && !reference.ExpiresAt.After(now)) || reference.ExpiresAt.After(input.Deadline) {
			return ExecuteRequest{}, domainsandbox.ErrInvalidInput
		}
		reference.ExpiresAt = reference.ExpiresAt.UTC()
		artifactReferences[index] = reference
	}
	policy.NetworkAllowlist = append([]string(nil), policy.NetworkAllowlist...)
	policy.AllowedEnvNames = append([]string(nil), policy.AllowedEnvNames...)
	policy.VirtualReadPrefixes = append([]string(nil), policy.VirtualReadPrefixes...)
	policy.VirtualWritePrefixes = append([]string(nil), policy.VirtualWritePrefixes...)
	policy.AllowedExecutables = append([]string(nil), policy.AllowedExecutables...)
	return ExecuteRequest{
		Scope:              input.Scope,
		WorkloadKind:       input.WorkloadKind,
		IdempotencyKey:     input.IdempotencyKey,
		Deadline:           input.Deadline.UTC(),
		Policy:             policy,
		Entrypoint:         input.Entrypoint,
		Args:               args,
		Env:                environment,
		Stdin:              append([]byte(nil), input.Stdin...),
		Files:              files,
		ArtifactReferences: artifactReferences,
	}, nil
}

func normalizeHealthResult(input HealthResult) (HealthResult, error) {
	if input.ProtocolVersion != HealthProtocolV1 {
		return HealthResult{}, domainsandbox.ErrInvalidInput
	}
	switch input.Status {
	case domainsandbox.HealthStatusHealthy, domainsandbox.HealthStatusDegraded, domainsandbox.HealthStatusUnhealthy:
	default:
		return HealthResult{}, domainsandbox.ErrInvalidInput
	}
	capabilities := []domainsandbox.Scope{}
	if len(input.Capabilities) > 0 {
		normalized, err := domainsandbox.NormalizeScopes(input.Capabilities)
		if err != nil {
			return HealthResult{}, domainsandbox.ErrInvalidInput
		}
		capabilities = append(capabilities, normalized...)
	}
	return HealthResult{ProtocolVersion: HealthProtocolV1, Status: input.Status, Capabilities: capabilities}, nil
}

// NormalizeExecuteResult is the single result contract shared by provider
// adapters and the application router. It returns a detached, validated copy.
func NormalizeExecuteResult(input ExecuteResult, maxOutputBytes int64) (ExecuteResult, error) {
	previewRoute, previewErr := NormalizePreviewRoute(input.PreviewRoute)
	if !validIdentifier(input.ExecutionID) || !validExecutionStatus(input.Status) ||
		!validOutput(input.Stdout) || !validOutput(input.Stderr) || len(input.Stdout) > MaxOutputStreamBytes ||
		len(input.Stderr) > MaxOutputStreamBytes || len(input.Artifacts) > MaxArtifacts || len(input.ArtifactDescriptors) > 1 || previewErr != nil {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	if len(input.ArtifactDescriptors) > 0 && input.Status != ExecutionStatusSucceeded {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	descriptors := make([]ArtifactDescriptor, len(input.ArtifactDescriptors))
	for index, descriptor := range input.ArtifactDescriptors {
		normalized, err := NormalizeArtifactDescriptor(descriptor)
		if err != nil {
			return ExecuteResult{}, domainsandbox.ErrInvalidInput
		}
		descriptors[index] = normalized
	}
	totalOutput := int64(len(input.Stdout) + len(input.Stderr))
	if maxOutputBytes <= 0 || totalOutput > maxOutputBytes || totalOutput > MaxCollectedOutputBytes {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	if input.ExitCode != nil && (*input.ExitCode < -1 || *input.ExitCode > 255) {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	switch input.Status {
	case ExecutionStatusAccepted, ExecutionStatusRunning:
		if input.ExitCode != nil {
			return ExecuteResult{}, domainsandbox.ErrInvalidInput
		}
	case ExecutionStatusSucceeded:
		if input.ExitCode == nil || *input.ExitCode != 0 {
			return ExecuteResult{}, domainsandbox.ErrInvalidInput
		}
	case ExecutionStatusFailed:
		if input.ExitCode == nil || *input.ExitCode == 0 {
			return ExecuteResult{}, domainsandbox.ErrInvalidInput
		}
	case ExecutionStatusCanceled, ExecutionStatusTimedOut:
		if input.ExitCode != nil {
			return ExecuteResult{}, domainsandbox.ErrInvalidInput
		}
	}
	artifacts := make([]ArtifactSummary, len(input.Artifacts))
	artifactIDs := make(map[string]struct{}, len(input.Artifacts))
	for index, artifact := range input.Artifacts {
		if !validIdentifier(artifact.ID) || !validArtifactName(artifact.Name) || !validMediaType(artifact.MediaType) ||
			artifact.Size < 0 || artifact.Size > MaxLogicalFileSizeBytes {
			return ExecuteResult{}, domainsandbox.ErrInvalidInput
		}
		if _, duplicate := artifactIDs[artifact.ID]; duplicate {
			return ExecuteResult{}, domainsandbox.ErrInvalidInput
		}
		artifactIDs[artifact.ID] = struct{}{}
		artifacts[index] = artifact
	}
	var exitCode *int
	if input.ExitCode != nil {
		value := *input.ExitCode
		exitCode = &value
	}
	return ExecuteResult{
		ExecutionID:         input.ExecutionID,
		Status:              input.Status,
		ExitCode:            exitCode,
		Stdout:              input.Stdout,
		Stderr:              input.Stderr,
		Artifacts:           artifacts,
		ArtifactDescriptors: descriptors,
		PreviewRoute:        previewRoute,
	}, nil
}

func NormalizePreviewRoute(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 512 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\\x00\r\n\t") ||
		!strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "", domainsandbox.ErrInvalidInput
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", domainsandbox.ErrInvalidInput
	}
	if cleaned := path.Clean(parsed.Path); cleaned != parsed.Path || strings.Contains(cleaned, "..") {
		return "", domainsandbox.ErrInvalidInput
	}
	return value, nil
}

func normalizeArtifactPublishRequest(input ArtifactPublishRequest, now time.Time) (ArtifactPublishRequest, error) {
	descriptor, err := NormalizeArtifactDescriptor(input.Descriptor)
	if err != nil || !validArtifactReferenceURL(input.UploadURL) || !validCredential(input.Token) ||
		input.ExpiresAt.IsZero() || !input.ExpiresAt.After(now) || input.ExpiresAt.After(now.Add(120*time.Second)) {
		return ArtifactPublishRequest{}, domainsandbox.ErrInvalidInput
	}
	return ArtifactPublishRequest{
		Descriptor: descriptor, UploadURL: input.UploadURL, Token: input.Token, ExpiresAt: input.ExpiresAt.UTC(),
	}, nil
}

func normalizeExecuteResult(input ExecuteResult, maxOutputBytes int64) (ExecuteResult, error) {
	return NormalizeExecuteResult(input, maxOutputBytes)
}

func cloneExecuteRequest(input ExecuteRequest) ExecuteRequest {
	cloned := input
	cloned.Policy.NetworkAllowlist = append([]string(nil), input.Policy.NetworkAllowlist...)
	cloned.Args = append([]string(nil), input.Args...)
	cloned.Env = make(map[string]string, len(input.Env))
	for key, value := range input.Env {
		cloned.Env[key] = value
	}
	cloned.Stdin = append([]byte(nil), input.Stdin...)
	cloned.Files = append([]FileReference(nil), input.Files...)
	cloned.ArtifactReferences = append([]ArtifactReference(nil), input.ArtifactReferences...)
	cloned.Policy.AllowedEnvNames = append([]string(nil), input.Policy.AllowedEnvNames...)
	cloned.Policy.VirtualReadPrefixes = append([]string(nil), input.Policy.VirtualReadPrefixes...)
	cloned.Policy.VirtualWritePrefixes = append([]string(nil), input.Policy.VirtualWritePrefixes...)
	cloned.Policy.AllowedExecutables = append([]string(nil), input.Policy.AllowedExecutables...)
	return cloned
}

func validExecutionID(value string) bool { return validIdentifier(value) }

func scopeMatchesWorkload(scope domainsandbox.Scope, kind WorkloadKind) bool {
	return scope == domainsandbox.ScopeAgent && kind == WorkloadAgent ||
		scope == domainsandbox.ScopeMCPStdio && kind == WorkloadMCPStdio ||
		scope == domainsandbox.ScopeAppDev && kind == WorkloadAppDev ||
		scope == domainsandbox.ScopePlugin && kind == WorkloadPlugin
}

func scopeMatchesWorkloadEntrypoint(
	scope domainsandbox.Scope,
	kind WorkloadKind,
	entrypoint string,
) bool {
	if !scopeMatchesWorkload(scope, kind) {
		return false
	}
	return scope != domainsandbox.ScopePlugin || entrypoint == PluginCodeRunnerEntrypoint
}

func validIdentifier(value string) bool {
	if len(value) == 0 || len(value) > MaxIdentifierBytes || !isIdentifierAlphaNumeric(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isIdentifierAlphaNumeric(character) && character != '.' && character != '_' && character != '-' && character != ':' {
			return false
		}
	}
	return true
}

func validLogicalPath(value string) bool {
	if len(value) == 0 || len(value) > MaxLogicalPathBytes || !utf8.ValidString(value) || path.IsAbs(value) ||
		path.Clean(value) != value || value == "." || strings.ContainsAny(value, "\\:\x00") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || !validBoundedText(segment, MaxLogicalPathBytes, false) {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != MaxDigestBytes || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for index := len("sha256:"); index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			if value[index] < 'a' || value[index] > 'f' {
				return false
			}
		}
	}
	return true
}

func validEnvironmentKey(value string) bool {
	if len(value) == 0 || len(value) > MaxEnvKeyBytes || !isASCIIAlpha(value[0]) && value[0] != '_' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if !isASCIIAlpha(value[index]) && (value[index] < '0' || value[index] > '9') && value[index] != '_' {
			return false
		}
	}
	return true
}

func validBoundedText(value string, maximum int, allowLineBreaks bool) bool {
	if len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == 0 || character == 0x7f || character < 0x20 && !(allowLineBreaks && (character == '\n' || character == '\r' || character == '\t')) {
			return false
		}
	}
	return true
}

func validOutput(value string) bool {
	return validBoundedText(value, MaxOutputStreamBytes, true)
}

func validArtifactName(value string) bool {
	return value != "" && len(value) <= MaxArtifactNameBytes && !strings.ContainsAny(value, "/\\") &&
		value != "." && value != ".." && validBoundedText(value, MaxArtifactNameBytes, false)
}

func validMediaType(value string) bool {
	if value == "" || len(value) > MaxMediaTypeBytes || !validBoundedText(value, MaxMediaTypeBytes, false) {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.Contains(mediaType, "/")
}

func validArtifactDirection(direction ArtifactDirection) bool {
	return direction == ArtifactDirectionDownload || direction == ArtifactDirectionUpload
}

func validArtifactReferenceURL(value string) bool {
	if value == "" || len(value) > MaxArtifactReferenceURLBytes || !utf8.ValidString(value) ||
		!validBoundedText(value, MaxArtifactReferenceURLBytes, false) {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		parsed.Fragment == "" && parsed.RawQuery == "" && parsed.Path != ""
}

func validExecutionStatus(status ExecutionStatus) bool {
	switch status {
	case ExecutionStatusAccepted, ExecutionStatusRunning, ExecutionStatusSucceeded, ExecutionStatusFailed,
		ExecutionStatusCanceled, ExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}

func validCredential(value string) bool {
	if len(value) == 0 || len(value) > MaxProviderCredentialBytes {
		return false
	}
	for index := range value {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

type executionSubmissionError struct {
	err       error
	uncertain bool
}

const (
	reconciliationProtocolVersionV1 = "1"
	reconciliationNotRecordedCodeV1 = "execution_not_recorded"
)

type authoritativeReconciliationNotRecordedError struct{}

var authoritativeReconciliationNotRecordedV1 = &authoritativeReconciliationNotRecordedError{}

func (*authoritativeReconciliationNotRecordedError) Error() string {
	return domainsandbox.ErrExecutionForbidden.Error()
}
func (*authoritativeReconciliationNotRecordedError) Unwrap() error {
	return domainsandbox.ErrExecutionForbidden
}

func newReconciliationNotRecordedErrorV1() error {
	return authoritativeReconciliationNotRecordedV1
}

// IsAuthoritativeNotRecorded recognizes only the sealed infra response marker.
// Standard error wrapping is allowed. The bounded traversal deliberately does
// not invoke errors.As or errors.Is, so package-external errors cannot forge
// the marker through custom As/Is methods.
func IsAuthoritativeNotRecorded(err error) bool {
	const (
		maxTraversalDepth = 32
		maxTraversalNodes = 128
	)
	type traversalNode struct {
		err   error
		depth int
	}

	pending := []traversalNode{{err: err}}
	comparableVisited := make(map[error]struct{})
	referenceVisited := make(map[authoritativeTraversalReference]struct{})
	visitedNodes := 0
	for len(pending) > 0 && visitedNodes < maxTraversalNodes {
		node := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if node.err == nil {
			continue
		}
		visitedNodes++
		if marker, ok := node.err.(*authoritativeReconciliationNotRecordedError); ok && marker == authoritativeReconciliationNotRecordedV1 {
			return true
		}
		if authoritativeTraversalAlreadyVisited(node.err, comparableVisited, referenceVisited) || node.depth >= maxTraversalDepth {
			continue
		}
		children := authoritativeTraversalUnwrap(node.err)
		remaining := maxTraversalNodes - visitedNodes
		if len(children) > remaining {
			children = children[:remaining]
		}
		for _, child := range children {
			pending = append(pending, traversalNode{err: child, depth: node.depth + 1})
		}
	}
	return false
}

type authoritativeTraversalReference struct {
	typeOf  reflect.Type
	pointer uintptr
}

func authoritativeTraversalAlreadyVisited(err error, comparable map[error]struct{}, references map[authoritativeTraversalReference]struct{}) bool {
	typeOf := reflect.TypeOf(err)
	if typeOf.Comparable() {
		if _, ok := comparable[err]; ok {
			return true
		}
		comparable[err] = struct{}{}
		return false
	}

	value := reflect.ValueOf(err)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		identity := authoritativeTraversalReference{typeOf: typeOf, pointer: value.Pointer()}
		if _, ok := references[identity]; ok {
			return true
		}
		references[identity] = struct{}{}
	}
	return false
}

func authoritativeTraversalUnwrap(err error) (children []error) {
	defer func() {
		if recover() != nil {
			children = nil
		}
	}()
	if wrapped, ok := err.(interface{ Unwrap() []error }); ok {
		return wrapped.Unwrap()
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return []error{wrapped.Unwrap()}
	}
	return nil
}

func (e *executionSubmissionError) Error() string { return e.err.Error() }
func (e *executionSubmissionError) Unwrap() error { return e.err }
func (e *executionSubmissionError) SubmissionOutcomeUncertain() bool {
	return e.uncertain
}

func executionNotSubmitted(err error) error {
	if err == nil {
		return nil
	}
	return &executionSubmissionError{err: err}
}

func executionOutcomeUnknown(err error) error {
	if err == nil {
		return nil
	}
	return &executionSubmissionError{err: err, uncertain: true}
}

// IsExecutionSubmissionUncertain reports whether an Execute error may have
// occurred after the provider accepted the idempotent submission. Unknown
// errors fail closed as uncertain; explicit input/auth/capacity denials do not.
func IsExecutionSubmissionUncertain(err error) bool {
	if err == nil {
		return false
	}
	var classified interface{ SubmissionOutcomeUncertain() bool }
	if errors.As(err, &classified) {
		return classified.SubmissionOutcomeUncertain()
	}
	for _, definite := range []error{
		domainsandbox.ErrInvalidInput,
		domainsandbox.ErrCredentialInvalid,
		domainsandbox.ErrCapacityExhausted,
		domainsandbox.ErrExecutionForbidden,
	} {
		if errors.Is(err, definite) {
			return false
		}
	}
	return true
}

func mapProviderError(err error) error {
	return mapProviderErrorWithContext(nil, err)
}

func mapProviderErrorWithContext(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
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
	for _, safeError := range []error{
		domainsandbox.ErrInvalidInput,
		domainsandbox.ErrCredentialInvalid,
		domainsandbox.ErrCapacityExhausted,
		domainsandbox.ErrExecutionForbidden,
		domainsandbox.ErrProviderUnhealthy,
	} {
		if errors.Is(err, safeError) {
			return safeError
		}
	}
	return domainsandbox.ErrProviderUnhealthy
}

func isIdentifierAlphaNumeric(character byte) bool {
	return isASCIIAlpha(character) || character >= '0' && character <= '9'
}

func isASCIIAlpha(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}
