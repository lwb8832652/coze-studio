// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const (
	sessionAcquireSchemaV1       = "coze.sandbox.session_acquire.v1"
	sessionProjectionSchemaV1    = "coze.sandbox.session.v1"
	sessionOperationSchemaV1     = "coze.sandbox.session_operation.v1"
	sessionEventSchemaV1         = "coze.sandbox.session_operation_event.v1"
	sessionConfigurationSchemaV1 = "coze.sandbox.session_configuration.v1"
	// A 32 MiB file encoded in JSON base64 remains below this bound. Keeping
	// the inline result below the independent 64 MiB HTTP response limit also
	// leaves room for the authenticated operation projection metadata.
	maxSessionInlineResultBytes = 48 << 20
)

var errSessionProtocol = errors.New("sandbox session protocol is invalid")

// SessionStableIdentity is the server-owned, persistent subset of the signed
// request identity. It contains no path or upstream resource identifier.
type SessionStableIdentity struct {
	DeploymentID string
	ProviderID   int64
	SpaceID      int64
	UserID       int64
	ThreadID     string
	Profile      domainsandbox.SessionProfile
}

func (SessionStableIdentity) String() string {
	return "sandboxrunner.SessionStableIdentity{identity:<redacted>}"
}
func (SessionStableIdentity) GoString() string {
	return "sandboxrunner.SessionStableIdentity{identity:<redacted>}"
}
func (SessionStableIdentity) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandboxrunner.SessionStableIdentity{identity:<redacted>}")
}

type SessionAcquireCommand struct {
	Identity SessionStableIdentity
	Claims   sandboxidentity.SessionRequest
}

type SessionRouteCommand struct {
	SessionID string
	Claims    sandboxidentity.SessionRequest
}

type SessionProjection struct {
	Schema            string                       `json:"schema"`
	SessionID         string                       `json:"session_id"`
	State             domainsandbox.SessionState   `json:"state"`
	RuntimeGeneration uint64                       `json:"runtime_generation"`
	Profile           domainsandbox.SessionProfile `json:"profile"`
}

// SessionOperationKind and SessionOperationState are shared by the private
// wire protocol and the encrypted operation store so the two layers cannot
// drift onto incompatible state machines.
type SessionOperationKind string

const (
	SessionOperationExec     SessionOperationKind = "exec"
	SessionOperationRead     SessionOperationKind = "read"
	SessionOperationWrite    SessionOperationKind = "write"
	SessionOperationAppend   SessionOperationKind = "append"
	SessionOperationList     SessionOperationKind = "list"
	SessionOperationGlob     SessionOperationKind = "glob"
	SessionOperationGrep     SessionOperationKind = "grep"
	SessionOperationReplace  SessionOperationKind = "replace"
	SessionOperationDownload SessionOperationKind = "download"
)

type SessionOperationState string

const (
	SessionOperationAccepted  SessionOperationState = "accepted"
	SessionOperationQueued    SessionOperationState = "queued"
	SessionOperationRunning   SessionOperationState = "running"
	SessionOperationSucceeded SessionOperationState = "succeeded"
	SessionOperationFailed    SessionOperationState = "failed"
	SessionOperationCanceled  SessionOperationState = "canceled"
	SessionOperationTimedOut  SessionOperationState = "timed_out"
	SessionOperationUnknown   SessionOperationState = "unknown"
)

type SessionOperationPayload struct {
	Argv           []string          `json:"argv,omitempty"`
	Command        string            `json:"command,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	CWD            string            `json:"cwd,omitempty"`
	MaxOutputBytes int64             `json:"max_output_bytes,omitempty"`
	Path           string            `json:"path,omitempty"`
	Content        []byte            `json:"content,omitempty"`
	MaxBytes       int64             `json:"max_bytes,omitempty"`
	Limit          int               `json:"limit,omitempty"`
	Pattern        string            `json:"pattern,omitempty"`
	Old            []byte            `json:"old,omitempty"`
	New            []byte            `json:"new,omitempty"`
}

type SessionOperationCommand struct {
	SessionID   string
	OperationID string
	Kind        SessionOperationKind
	Deadline    time.Time
	Payload     SessionOperationPayload
	Claims      sandboxidentity.SessionRequest
}

type SessionOperationRouteCommand struct {
	SessionID    string
	OperationID  string
	Claims       sandboxidentity.SessionRequest
	AfterEventID string
}

type SessionOperationProjection struct {
	Schema       string                `json:"schema"`
	SessionID    string                `json:"session_id"`
	OperationID  string                `json:"operation_id"`
	Kind         SessionOperationKind  `json:"kind"`
	State        SessionOperationState `json:"state"`
	ReasonCode   string                `json:"reason_code,omitempty"`
	ResultDigest string                `json:"result_digest,omitempty"`
	// Result is returned only by the synchronous submit response. Redis and
	// subsequent GET/recovery projections retain only ResultDigest.
	Result json.RawMessage `json:"result,omitempty"`
}

type SessionOperationEvent struct {
	Schema      string                `json:"schema"`
	EventID     string                `json:"event_id"`
	OperationID string                `json:"operation_id"`
	State       SessionOperationState `json:"state"`
	ReasonCode  string                `json:"reason_code,omitempty"`
}

type SessionConfigurationProjection struct {
	Schema   string                               `json:"schema"`
	Version  uint64                               `json:"version"`
	Settings domainsandbox.SessionRuntimeSettings `json:"settings"`
}

type SessionConfigurationCommand struct {
	Version  uint64
	Settings domainsandbox.SessionRuntimeSettings
	Claims   sandboxidentity.SessionRequest
}

type sessionAcquireWire struct {
	Schema       string                `json:"schema"`
	DeploymentID string                `json:"deployment_id"`
	ProviderID   int64                 `json:"provider_id"`
	Scope        sandboxidentity.Scope `json:"scope"`
	SpaceID      int64                 `json:"space_id"`
	UserID       int64                 `json:"user_id"`
	ThreadID     string                `json:"thread_id"`
	RunID        string                `json:"run_id"`
	OperationID  string                `json:"operation_id"`
	Profile      string                `json:"profile"`
}

type parsedSessionAcquire struct {
	Identity SessionStableIdentity
	Claims   sandboxidentity.SessionRequest
}

func parseSessionAcquire(body []byte, deploymentID string) (parsedSessionAcquire, error) {
	var wire sessionAcquireWire
	if len(body) == 0 || len(body) > maxRequestBytes || decodeStrictJSON(body, &wire) != nil ||
		wire.Schema != sessionAcquireSchemaV1 || wire.DeploymentID != deploymentID ||
		!validSessionProtocolIdentifier(deploymentID) || wire.ProviderID <= 0 || wire.SpaceID <= 0 ||
		wire.UserID <= 0 || !validSessionProtocolIdentifier(wire.ThreadID) ||
		!validSessionProtocolIdentifier(wire.RunID) || !validSessionProtocolIdentifier(wire.OperationID) ||
		wire.Profile != string(domainsandbox.SessionProfileCore) || !validSessionScope(wire.Scope) {
		return parsedSessionAcquire{}, errSessionProtocol
	}
	digest := sha256.Sum256(body)
	claims := sandboxidentity.SessionRequest{
		DeploymentID: deploymentID, ProviderID: wire.ProviderID, Scope: wire.Scope, SpaceID: wire.SpaceID, UserID: wire.UserID,
		ThreadID: wire.ThreadID, RunID: wire.RunID, OperationID: wire.OperationID,
		Profile: wire.Profile, RequestDigest: digest[:],
	}
	return parsedSessionAcquire{
		Identity: SessionStableIdentity{DeploymentID: deploymentID, ProviderID: wire.ProviderID,
			SpaceID: wire.SpaceID, UserID: wire.UserID, ThreadID: wire.ThreadID, Profile: domainsandbox.SessionProfile(wire.Profile)},
		Claims: claims,
	}, nil
}

type sessionOperationWire struct {
	Schema      string                  `json:"schema"`
	OperationID string                  `json:"operation_id"`
	Kind        SessionOperationKind    `json:"kind"`
	Deadline    string                  `json:"deadline"`
	Payload     SessionOperationPayload `json:"payload"`
}

func parseSessionOperation(body []byte, now time.Time, allowedEnvNames ...string) (SessionOperationCommand, error) {
	var wire sessionOperationWire
	if now.IsZero() || len(body) == 0 || len(body) > maxRequestBytes || decodeStrictJSON(body, &wire) != nil ||
		wire.Schema != sessionOperationSchemaV1 || !validSessionProtocolIdentifier(wire.OperationID) || !validSessionOperationKind(wire.Kind) {
		return SessionOperationCommand{}, errSessionProtocol
	}
	deadline, err := time.Parse(time.RFC3339Nano, wire.Deadline)
	if err != nil || !deadline.After(now) || deadline.After(now.Add(infrasandbox.MaxSessionDeadlineAhead)) ||
		validateSessionOperationPayload(wire.Kind, wire.OperationID, deadline.UTC(), wire.Payload, now, allowedEnvNames) != nil {
		return SessionOperationCommand{}, errSessionProtocol
	}
	return SessionOperationCommand{OperationID: wire.OperationID, Kind: wire.Kind, Deadline: deadline.UTC(), Payload: cloneSessionOperationPayload(wire.Payload)}, nil
}

func validateSessionOperationPayload(kind SessionOperationKind, operationID string, deadline time.Time, payload SessionOperationPayload, now time.Time, allowedEnvNames []string) error {
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > maxRequestBytes {
		return errSessionProtocol
	}
	switch kind {
	case SessionOperationExec:
		if (len(payload.Argv) == 0) == (payload.Command == "") || payload.CWD == "" || payload.MaxOutputBytes <= 0 || payload.MaxOutputBytes > 4*1024*1024 ||
			payload.Path != "" || payload.Content != nil || payload.MaxBytes != 0 || payload.Limit != 0 || payload.Pattern != "" || payload.Old != nil || payload.New != nil {
			return errSessionProtocol
		}
		if _, err := infrasandbox.NormalizeExecRequest(infrasandbox.ExecRequest{
			OperationID: operationID, Argv: payload.Argv, Command: payload.Command, Env: payload.Env,
			CWD: payload.CWD, Deadline: deadline, MaxOutputBytes: payload.MaxOutputBytes,
		}, now, allowedEnvNames); err != nil {
			return errSessionProtocol
		}
	case SessionOperationRead, SessionOperationDownload:
		if payload.Path == "" || payload.MaxBytes <= 0 || payload.Argv != nil || payload.Command != "" || payload.Env != nil || payload.CWD != "" ||
			payload.Content != nil || payload.MaxOutputBytes != 0 || payload.Limit != 0 || payload.Pattern != "" || payload.Old != nil || payload.New != nil {
			return errSessionProtocol
		}
		if kind == SessionOperationRead {
			if _, err := infrasandbox.NormalizeReadRequest(infrasandbox.ReadRequest{Path: payload.Path, MaxBytes: payload.MaxBytes}); err != nil {
				return errSessionProtocol
			}
		} else if _, err := infrasandbox.NormalizeDownloadRequest(infrasandbox.DownloadRequest{Path: payload.Path, MaxBytes: payload.MaxBytes}); err != nil {
			return errSessionProtocol
		}
	case SessionOperationWrite, SessionOperationAppend:
		if payload.Path == "" || payload.Content == nil || len(payload.Content) > maxRequestBytes || payload.Argv != nil || payload.Command != "" || payload.Env != nil ||
			payload.CWD != "" || payload.MaxOutputBytes != 0 || payload.MaxBytes != 0 || payload.Limit != 0 || payload.Pattern != "" || payload.Old != nil || payload.New != nil {
			return errSessionProtocol
		}
		if _, err := infrasandbox.NormalizeWriteRequest(infrasandbox.WriteRequest{Path: payload.Path, Content: payload.Content, Append: kind == SessionOperationAppend}); err != nil {
			return errSessionProtocol
		}
	case SessionOperationList:
		if payload.Path == "" || payload.Limit <= 0 || payload.Limit > 1000 || sessionPayloadHasNonListFields(payload) {
			return errSessionProtocol
		}
		if _, err := infrasandbox.NormalizeListRequest(infrasandbox.ListRequest{Path: payload.Path, Limit: payload.Limit}); err != nil {
			return errSessionProtocol
		}
	case SessionOperationGlob, SessionOperationGrep:
		if payload.Path == "" || payload.Pattern == "" || payload.Limit <= 0 || payload.Limit > 1000 || sessionPayloadHasNonPatternFields(payload) {
			return errSessionProtocol
		}
		if kind == SessionOperationGlob {
			if _, err := infrasandbox.NormalizeGlobRequest(infrasandbox.GlobRequest{Path: payload.Path, Pattern: payload.Pattern, Limit: payload.Limit}); err != nil {
				return errSessionProtocol
			}
		} else if _, err := infrasandbox.NormalizeGrepRequest(infrasandbox.GrepRequest{Path: payload.Path, Pattern: payload.Pattern, Limit: payload.Limit}); err != nil {
			return errSessionProtocol
		}
	case SessionOperationReplace:
		if payload.Path == "" || len(payload.Old) == 0 || len(payload.Old)+len(payload.New) > maxRequestBytes || payload.Argv != nil || payload.Command != "" ||
			payload.Env != nil || payload.CWD != "" || payload.MaxOutputBytes != 0 || payload.Content != nil || payload.MaxBytes != 0 || payload.Limit != 0 || payload.Pattern != "" {
			return errSessionProtocol
		}
		if _, err := infrasandbox.NormalizeReplaceRequest(infrasandbox.ReplaceRequest{Path: payload.Path, Old: payload.Old, New: payload.New}); err != nil {
			return errSessionProtocol
		}
	default:
		return errSessionProtocol
	}
	return nil
}

func sessionPayloadHasNonListFields(payload SessionOperationPayload) bool {
	return payload.Argv != nil || payload.Command != "" || payload.Env != nil || payload.CWD != "" || payload.MaxOutputBytes != 0 || payload.Content != nil ||
		payload.MaxBytes != 0 || payload.Pattern != "" || payload.Old != nil || payload.New != nil
}

func sessionPayloadHasNonPatternFields(payload SessionOperationPayload) bool {
	return payload.Argv != nil || payload.Command != "" || payload.Env != nil || payload.CWD != "" || payload.MaxOutputBytes != 0 || payload.Content != nil ||
		payload.MaxBytes != 0 || payload.Old != nil || payload.New != nil
}

func cloneSessionOperationPayload(payload SessionOperationPayload) SessionOperationPayload {
	payload.Argv = append([]string(nil), payload.Argv...)
	payload.Content = append([]byte(nil), payload.Content...)
	payload.Old = append([]byte(nil), payload.Old...)
	payload.New = append([]byte(nil), payload.New...)
	if payload.Env != nil {
		cloned := make(map[string]string, len(payload.Env))
		for key, value := range payload.Env {
			cloned[key] = value
		}
		payload.Env = cloned
	}
	return payload
}

type sessionConfigurationWire struct {
	Schema   string          `json:"schema"`
	Version  uint64          `json:"version"`
	Settings json.RawMessage `json:"settings"`
}

func parseSessionConfiguration(body []byte) (SessionConfigurationCommand, error) {
	var wire sessionConfigurationWire
	if len(body) == 0 || len(body) > maxRequestBytes || decodeStrictJSON(body, &wire) != nil ||
		wire.Schema != sessionConfigurationSchemaV1 || wire.Version == 0 || len(wire.Settings) == 0 {
		return SessionConfigurationCommand{}, errSessionProtocol
	}
	settings, err := domainsandbox.DecodeSessionRuntimeSettingsJSON(wire.Settings)
	if err != nil {
		return SessionConfigurationCommand{}, errSessionProtocol
	}
	settings.Version = wire.Version
	return SessionConfigurationCommand{Version: wire.Version, Settings: settings}, nil
}

func claimsMatchAcquire(actual sandboxidentity.SessionRequest, expected parsedSessionAcquire) bool {
	return sameSessionClaims(actual, expected.Claims) && stableIdentityMatchesClaims(expected.Identity, actual)
}

func stableIdentityMatchesClaims(identity SessionStableIdentity, claims sandboxidentity.SessionRequest) bool {
	return identity.DeploymentID == claims.DeploymentID && identity.ProviderID == claims.ProviderID && identity.SpaceID == claims.SpaceID && identity.UserID == claims.UserID &&
		identity.ThreadID == claims.ThreadID && string(identity.Profile) == claims.Profile
}

func sameSessionClaims(left, right sandboxidentity.SessionRequest) bool {
	return left.DeploymentID == right.DeploymentID && left.ProviderID == right.ProviderID && left.Scope == right.Scope && left.SpaceID == right.SpaceID &&
		left.UserID == right.UserID && left.ThreadID == right.ThreadID && left.RunID == right.RunID &&
		left.OperationID == right.OperationID && left.Profile == right.Profile && hmac.Equal(left.RequestDigest, right.RequestDigest)
}

func validSessionClaims(claims sandboxidentity.SessionRequest, digest []byte) bool {
	return validSessionProtocolIdentifier(claims.DeploymentID) && claims.ProviderID > 0 && claims.SpaceID > 0 && claims.UserID > 0 && validSessionScope(claims.Scope) &&
		validSessionProtocolIdentifier(claims.ThreadID) && validSessionProtocolIdentifier(claims.RunID) &&
		validSessionProtocolIdentifier(claims.OperationID) && claims.Profile == string(domainsandbox.SessionProfileCore) &&
		len(digest) == sha256.Size && hmac.Equal(claims.RequestDigest, digest)
}

func validSessionConfigurationClaims(claims sandboxidentity.SessionRequest, digest []byte) bool {
	return validSessionProtocolIdentifier(claims.DeploymentID) && claims.ProviderID == 0 && claims.Scope == "" && claims.SpaceID == 0 && claims.UserID == 0 &&
		claims.ThreadID == "" && claims.RunID == "" && claims.OperationID == "" && claims.Profile == "" &&
		len(digest) == sha256.Size && hmac.Equal(claims.RequestDigest, digest)
}

func validSessionScope(scope sandboxidentity.Scope) bool {
	switch scope {
	case sandboxidentity.ScopeAgent, sandboxidentity.ScopeMCPStdio, sandboxidentity.ScopeAppDev, sandboxidentity.ScopePlugin:
		return true
	default:
		return false
	}
}

func validSessionOperationKind(kind SessionOperationKind) bool {
	switch kind {
	case SessionOperationExec, SessionOperationRead, SessionOperationWrite, SessionOperationAppend,
		SessionOperationList, SessionOperationGlob, SessionOperationGrep, SessionOperationReplace, SessionOperationDownload:
		return true
	default:
		return false
	}
}

func validSessionOperationState(state SessionOperationState) bool {
	switch state {
	case SessionOperationAccepted, SessionOperationQueued, SessionOperationRunning, SessionOperationSucceeded,
		SessionOperationFailed, SessionOperationCanceled, SessionOperationTimedOut, SessionOperationUnknown:
		return true
	default:
		return false
	}
}

func terminalSessionOperationState(state SessionOperationState) bool {
	switch state {
	case SessionOperationSucceeded, SessionOperationFailed, SessionOperationCanceled, SessionOperationTimedOut, SessionOperationUnknown:
		return true
	default:
		return false
	}
}

func validSessionProtocolIdentifier(value string) bool {
	if value == "" || len(value) > domainsandbox.MaxSessionIdentifierBytes || strings.TrimSpace(value) != value || value == "." || value == ".." {
		return false
	}
	for index := range value {
		character := value[index]
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func sessionRequestDigest(body []byte) []byte {
	digest := sha256.Sum256(body)
	return digest[:]
}

func strictEmptyJSONObject(body []byte) bool {
	var value struct{}
	return len(body) > 0 && decodeStrictJSON(body, &value) == nil && bytes.Equal(bytes.TrimSpace(body), []byte("{}"))
}

func canonicalAllowedEnvironmentNames(names []string) ([]string, error) {
	if len(names) > infrasandbox.MaxEnvVars {
		return nil, errSessionProtocol
	}
	unique := make(map[string]string, len(names))
	for _, name := range names {
		if _, duplicate := unique[name]; duplicate {
			return nil, errSessionProtocol
		}
		unique[name] = ""
	}
	canonical, err := infrasandbox.CanonicalEnvironmentNames(unique)
	if err != nil || len(canonical) != len(names) {
		return nil, errSessionProtocol
	}
	return canonical, nil
}
