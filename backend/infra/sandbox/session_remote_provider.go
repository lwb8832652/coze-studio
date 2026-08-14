// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/safehttp"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const (
	remoteSessionAcquireSchemaV1     = "coze.sandbox.session_acquire.v1"
	remoteSessionProjectionSchema    = "coze.sandbox.session.v1"
	remoteSessionOperationSchema     = "coze.sandbox.session_operation.v1"
	remoteSessionEventSchema         = "coze.sandbox.session_operation_event.v1"
	remoteSessionConfigurationSchema = "coze.sandbox.session_configuration.v1"
	remoteSessionRuntimeStatusSchema = "coze.sandbox.session_runtime_status.v1"

	maxRemoteSessionResponseBytes = int64(64 << 20)
	maxRemoteSessionEventsBytes   = int64(16 << 10)
	defaultRemoteFileDeadline     = time.Minute
)

// RemoteSessionProviderConfig binds one Session manager to one database
// provider descriptor and one trusted routing scope. ProviderKey and tenant
// identity are deliberately absent: the former never enters v2 claims and the
// latter is supplied only by a normalized SessionKey/SessionRef.
type RemoteSessionProviderConfig struct {
	DeploymentID            string
	ProviderID              int64
	Scope                   sandboxidentity.Scope
	SessionSigner           sandboxidentity.SessionSigner
	AllowedEnvironmentNames []string
}

// RemoteSessionProvider composes the existing one-shot provider with the
// signed private Session protocol. A plain RemoteProvider never satisfies the
// optional SessionRuntimeProvider capability.
type RemoteSessionProvider struct {
	*RemoteProvider
	deploymentID string
	providerID   int64
	scope        sandboxidentity.Scope
	signer       sandboxidentity.SessionSigner
	allowedEnv   []string
	now          func() time.Time
	newID        func() (string, error)
}

// NewRemoteSessionProvider adds signed Session v2 support to an already-safe
// RemoteProvider transport. Failure does not affect the original one-shot
// provider, so missing Session configuration fails closed only for Sessions.
func NewRemoteSessionProvider(remote *RemoteProvider, config RemoteSessionProviderConfig) (*RemoteSessionProvider, error) {
	if remote == nil || remote.endpoint == nil || remote.doer == nil || config.SessionSigner == nil ||
		config.ProviderID <= 0 || !validRemoteSessionIdentifier(config.DeploymentID) || !validRemoteSessionScope(config.Scope) {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	allowed, err := normalizeRemoteSessionAllowedEnvironment(config.AllowedEnvironmentNames)
	if err != nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &RemoteSessionProvider{
		RemoteProvider: remote, deploymentID: config.DeploymentID, providerID: config.ProviderID,
		scope: config.Scope, signer: config.SessionSigner, allowedEnv: allowed,
		now: func() time.Time { return time.Now().UTC() }, newID: newRemoteSessionID,
	}, nil
}

func (provider *RemoteSessionProvider) Acquire(ctx context.Context, input AcquireSessionRequest) (SandboxSession, error) {
	if provider == nil || ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	input, err := NormalizeAcquireSessionRequest(input)
	if err != nil || !provider.matchesKey(input.Key) {
		return nil, domainsandbox.ErrInvalidInput
	}
	runID, operationID, err := provider.newRequestIdentity()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(remoteSessionAcquireRequest{
		Schema: remoteSessionAcquireSchemaV1, DeploymentID: provider.deploymentID,
		ProviderID: provider.providerID, Scope: provider.scope, SpaceID: input.Key.SpaceID,
		UserID: input.Key.UserID, ThreadID: input.Key.ThreadID, RunID: runID,
		OperationID: operationID, Profile: string(input.Key.Profile),
	})
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	projection, err := provider.sessionProjectionRequest(ctx, http.MethodPost, "/v1/sessions:acquire", body,
		input.Key, runID, operationID)
	if err != nil {
		return nil, err
	}
	if projection.State != domainsandbox.SessionStateActive {
		return nil, domainsandbox.ErrUnavailable
	}
	return provider.sessionFromProjection(input.Key, runID, projection)
}

func (provider *RemoteSessionProvider) Get(ctx context.Context, ref domainsandbox.SessionRef) (SandboxSession, error) {
	return provider.sessionLifecycle(ctx, http.MethodGet, "", ref, true, false)
}

func (provider *RemoteSessionProvider) Release(ctx context.Context, ref domainsandbox.SessionRef) error {
	_, err := provider.sessionLifecycle(ctx, http.MethodPost, ":release", ref, false, false)
	return err
}

func (provider *RemoteSessionProvider) Destroy(ctx context.Context, ref domainsandbox.SessionRef) error {
	_, err := provider.sessionLifecycle(ctx, http.MethodPost, ":destroy", ref, false, false)
	return err
}

func (provider *RemoteSessionProvider) Recover(ctx context.Context, ref domainsandbox.SessionRef) (SandboxSession, error) {
	return provider.sessionLifecycle(ctx, http.MethodPost, ":recover", ref, true, true)
}

func (provider *RemoteSessionProvider) SessionSettings(ctx context.Context) (domainsandbox.SessionRuntimeSettings, error) {
	projection, err := provider.sessionConfigurationRequest(ctx, http.MethodGet, nil)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	return projection.Settings, nil
}

func (provider *RemoteSessionProvider) ApplySessionSettings(ctx context.Context, settings domainsandbox.SessionRuntimeSettings) (uint64, error) {
	normalized, err := domainsandbox.NormalizeSessionRuntimeSettings(settings)
	if err != nil || settings.Version == 0 {
		return 0, domainsandbox.ErrInvalidInput
	}
	normalized.Version = settings.Version
	body, err := json.Marshal(remoteSessionConfigurationProjection{Schema: remoteSessionConfigurationSchema, Version: settings.Version, Settings: normalized})
	if err != nil {
		return 0, domainsandbox.ErrInvalidInput
	}
	projection, err := provider.sessionConfigurationRequest(ctx, http.MethodPut, body)
	if err != nil {
		return 0, err
	}
	if projection.Version != settings.Version || !sameRemoteSessionSettings(projection.Settings, normalized) {
		return 0, domainsandbox.ErrProviderUnhealthy
	}
	return projection.Version, nil
}

// SessionRuntimeStatus reads the independent aggregate-only Session status
// route. Runner cannot assert transport security; that fact is filled from
// the exact-origin endpoint policy used for this request.
func (provider *RemoteSessionProvider) SessionRuntimeStatus(ctx context.Context) (SessionRuntimeStatus, error) {
	if provider == nil || ctx == nil || provider.endpoint == nil {
		return SessionRuntimeStatus{}, domainsandbox.ErrInvalidInput
	}
	digest := sha256.Sum256(nil)
	signed, err := provider.signer.SignSession(sandboxidentity.SessionRequest{
		DeploymentID: provider.deploymentID, RequestDigest: digest[:],
	}, http.MethodGet, "/v1/session-runtime-status")
	if err != nil {
		return SessionRuntimeStatus{}, domainsandbox.ErrConfigurationInvalid
	}
	requestCtx := safehttp.WithResponseBodyLimit(ctx, MaxHealthResponseBodyBytes)
	request, err := provider.newRequest(requestCtx, http.MethodGet, "/v1/session-runtime-status", nil)
	if err != nil {
		return SessionRuntimeStatus{}, err
	}
	request.Header.Set(sandboxidentity.SessionContextHeader, signed.Context)
	request.Header.Set(sandboxidentity.SessionContextSignatureHeader, signed.Signature)
	response, err := provider.doer.Do(request)
	if err != nil {
		return SessionRuntimeStatus{}, mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapRemoteSessionHTTPStatus(response.StatusCode); err != nil {
		return SessionRuntimeStatus{}, err
	}
	var projection remoteSessionRuntimeStatusProjection
	if err := decodeStrictJSONResponseContext(request.Context(), response, MaxHealthResponseBodyBytes, &projection); err != nil ||
		!validRemoteSessionRuntimeStatus(projection) {
		return SessionRuntimeStatus{}, domainsandbox.ErrProviderUnhealthy
	}
	return SessionRuntimeStatus{
		Available: projection.Available, AppliedConfigVersion: projection.AppliedConfigVersion,
		RuntimeGeneration: projection.RuntimeGeneration, CoreEnabled: projection.CoreEnabled,
		InteractiveEnabled: projection.InteractiveEnabled, HostShellEnabled: projection.HostShellEnabled,
		HostShellAvailable: projection.HostShellAvailable, RawAIOReady: projection.RawAIOReady,
		GenerationState: projection.GenerationState, QueueDepth: projection.QueueDepth,
		Running: projection.Running, UsedWeight: projection.UsedWeight, TotalWeight: projection.TotalWeight,
		ActiveSessions: projection.ActiveSessions, IdleSessions: projection.IdleSessions,
		ActiveShells: projection.ActiveShells, IdleShells: projection.IdleShells,
		TransportEncrypted: provider.endpoint.TransportEncrypted, ReasonCode: projection.ReasonCode,
	}, nil
}

func (provider *RemoteSessionProvider) sessionConfigurationRequest(ctx context.Context, method string, body []byte) (remoteSessionConfigurationProjection, error) {
	if provider == nil || ctx == nil || (method != http.MethodGet && method != http.MethodPut) || method == http.MethodGet && len(body) != 0 || method == http.MethodPut && len(body) == 0 {
		return remoteSessionConfigurationProjection{}, domainsandbox.ErrInvalidInput
	}
	digest := sha256.Sum256(body)
	signed, err := provider.signer.SignSession(sandboxidentity.SessionRequest{DeploymentID: provider.deploymentID, RequestDigest: digest[:]}, method, "/v1/session-configuration")
	if err != nil {
		return remoteSessionConfigurationProjection{}, domainsandbox.ErrConfigurationInvalid
	}
	requestCtx := safehttp.WithResponseBodyLimit(ctx, MaxHealthResponseBodyBytes)
	request, err := provider.newRequest(requestCtx, method, "/v1/session-configuration", body)
	if err != nil {
		return remoteSessionConfigurationProjection{}, err
	}
	request.Header.Set(sandboxidentity.SessionContextHeader, signed.Context)
	request.Header.Set(sandboxidentity.SessionContextSignatureHeader, signed.Signature)
	response, err := provider.doer.Do(request)
	if err != nil {
		return remoteSessionConfigurationProjection{}, mapProviderErrorWithContext(request.Context(), err)
	}
	defer response.Body.Close()
	if err := mapRemoteSessionHTTPStatus(response.StatusCode); err != nil {
		return remoteSessionConfigurationProjection{}, err
	}
	var projection remoteSessionConfigurationProjection
	if err := decodeStrictJSONResponseContext(request.Context(), response, MaxHealthResponseBodyBytes, &projection); err != nil ||
		projection.Schema != remoteSessionConfigurationSchema || projection.Version == 0 || projection.Settings.Version != 0 {
		return remoteSessionConfigurationProjection{}, domainsandbox.ErrProviderUnhealthy
	}
	settings, err := domainsandbox.NormalizeSessionRuntimeSettings(projection.Settings)
	if err != nil {
		return remoteSessionConfigurationProjection{}, domainsandbox.ErrProviderUnhealthy
	}
	settings.Version = projection.Version
	projection.Settings = settings
	return projection, nil
}

func sameRemoteSessionSettings(left, right domainsandbox.SessionRuntimeSettings) bool {
	left.Version, left.UpdatedBy = 0, 0
	right.Version, right.UpdatedBy = 0, 0
	return left == right
}

func (provider *RemoteSessionProvider) sessionLifecycle(ctx context.Context, method, suffix string, ref domainsandbox.SessionRef, wantProjection, allowNewGeneration bool) (SandboxSession, error) {
	if provider == nil || ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	ref, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || !provider.matchesKey(ref.Key) {
		return nil, domainsandbox.ErrInvalidInput
	}
	runID, operationID, err := provider.newRequestIdentity()
	if err != nil {
		return nil, err
	}
	requestPath := "/v1/sessions/" + ref.SessionID + suffix
	var body []byte
	if method != http.MethodGet {
		body = []byte("{}")
	}
	if wantProjection {
		projection, requestErr := provider.sessionProjectionRequest(ctx, method, requestPath, body, ref.Key, runID, operationID)
		if requestErr != nil {
			return nil, requestErr
		}
		if projection.SessionID != ref.SessionID || (!allowNewGeneration && projection.RuntimeGeneration != ref.RuntimeGeneration) ||
			projection.Profile != ref.Key.Profile || (method == http.MethodGet && projection.State != domainsandbox.SessionStateActive) ||
			(method == http.MethodPost && projection.State != domainsandbox.SessionStateActive) {
			return nil, domainsandbox.ErrUnavailable
		}
		return provider.sessionFromProjection(ref.Key, runID, projection)
	}
	response, requestErr := provider.doSessionRequest(ctx, method, requestPath, body, ref.Key, runID, operationID, maxRemoteSessionEventsBytes)
	if requestErr != nil {
		return nil, requestErr
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent || response.ContentLength > 0 {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	return nil, nil
}

func (provider *RemoteSessionProvider) sessionProjectionRequest(ctx context.Context, method, requestPath string, body []byte, key domainsandbox.SessionKey, runID, operationID string) (remoteSessionProjection, error) {
	response, err := provider.doSessionRequest(ctx, method, requestPath, body, key, runID, operationID, MaxHealthResponseBodyBytes)
	if err != nil {
		return remoteSessionProjection{}, err
	}
	defer response.Body.Close()
	var projection remoteSessionProjection
	if err := decodeStrictJSONResponseContext(ctx, response, MaxHealthResponseBodyBytes, &projection); err != nil ||
		projection.Schema != remoteSessionProjectionSchema || !validRemoteSessionIdentifier(projection.SessionID) ||
		projection.RuntimeGeneration == 0 || projection.Profile != domainsandbox.SessionProfileCore || !validRemoteSessionState(projection.State) {
		return remoteSessionProjection{}, domainsandbox.ErrProviderUnhealthy
	}
	return projection, nil
}

func (provider *RemoteSessionProvider) sessionFromProjection(key domainsandbox.SessionKey, runID string, projection remoteSessionProjection) (*RemoteSession, error) {
	ref, err := domainsandbox.NormalizeSessionRef(domainsandbox.SessionRef{
		SessionID: projection.SessionID, Key: key, RuntimeGeneration: projection.RuntimeGeneration,
	})
	if err != nil {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	return &RemoteSession{provider: provider, ref: ref, runID: runID}, nil
}

func (provider *RemoteSessionProvider) doSessionRequest(ctx context.Context, method, requestPath string, body []byte, key domainsandbox.SessionKey, runID, operationID string, responseLimit int64) (*http.Response, error) {
	return provider.doSessionRequestWithHeaders(ctx, method, requestPath, body, key, runID, operationID, responseLimit, nil)
}

func (provider *RemoteSessionProvider) doSessionRequestWithHeaders(ctx context.Context, method, requestPath string, body []byte, key domainsandbox.SessionKey, runID, operationID string, responseLimit int64, headers map[string]string) (*http.Response, error) {
	if provider == nil || ctx == nil || provider.signer == nil || !provider.matchesKey(key) ||
		!validRemoteSessionIdentifier(runID) || !validRemoteSessionIdentifier(operationID) || responseLimit <= 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	digest := sha256.Sum256(body)
	signed, err := provider.signer.SignSession(sandboxidentity.SessionRequest{
		DeploymentID: provider.deploymentID, ProviderID: provider.providerID, Scope: provider.scope,
		SpaceID: key.SpaceID, UserID: key.UserID, ThreadID: key.ThreadID, RunID: runID,
		OperationID: operationID, Profile: string(key.Profile), RequestDigest: digest[:],
	}, method, requestPath)
	if err != nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	requestCtx := safehttp.WithResponseBodyLimit(ctx, responseLimit)
	request, err := provider.newRequest(requestCtx, method, requestPath, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set(sandboxidentity.SessionContextHeader, signed.Context)
	request.Header.Set(sandboxidentity.SessionContextSignatureHeader, signed.Signature)
	for name, value := range headers {
		if name != "Last-Event-ID" || (value != "" && !validRemoteSessionIdentifier(value)) {
			return nil, domainsandbox.ErrInvalidInput
		}
		if value != "" {
			request.Header.Set(name, value)
		}
	}
	response, err := provider.doer.Do(request)
	if err != nil {
		return nil, mapProviderErrorWithContext(request.Context(), err)
	}
	if err := mapRemoteSessionHTTPStatus(response.StatusCode); err != nil {
		response.Body.Close()
		return nil, err
	}
	return response, nil
}

func (provider *RemoteSessionProvider) matchesKey(key domainsandbox.SessionKey) bool {
	normalized, err := domainsandbox.NormalizeSessionKey(key)
	return err == nil && normalized == key && key.DeploymentID == provider.deploymentID && key.ProviderID == provider.providerID &&
		domainsandbox.ValidatePhase1SessionProfile(key.Profile) == nil
}

func (provider *RemoteSessionProvider) newRequestIdentity() (string, string, error) {
	if provider == nil || provider.newID == nil {
		return "", "", domainsandbox.ErrConfigurationInvalid
	}
	runID, err := provider.newID()
	if err != nil || !validRemoteSessionIdentifier(runID) {
		return "", "", domainsandbox.ErrUnavailable
	}
	operationID, err := provider.newID()
	if err != nil || !validRemoteSessionIdentifier(operationID) {
		return "", "", domainsandbox.ErrUnavailable
	}
	return runID, operationID, nil
}

type RemoteSession struct {
	provider *RemoteSessionProvider
	ref      domainsandbox.SessionRef
	runID    string
}

func (session *RemoteSession) Ref() domainsandbox.SessionRef {
	if session == nil {
		return domainsandbox.SessionRef{}
	}
	return session.ref
}

func (session *RemoteSession) Exec(ctx context.Context, input ExecRequest) (ExecutionStream, error) {
	if session == nil || session.provider == nil || ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	input, err := NormalizeExecRequest(input, session.provider.now(), session.provider.allowedEnv)
	if err != nil {
		return nil, err
	}
	projection, err := session.submit(ctx, input.OperationID, remoteSessionOperationExec, input.Deadline, remoteSessionOperationPayload{
		Argv: input.Argv, Command: input.Command, Env: input.Env, CWD: input.CWD, MaxOutputBytes: input.MaxOutputBytes,
	})
	if err != nil {
		return nil, err
	}
	var result remoteExecutionResult
	if err := projection.decodeSucceededResult(&result); err != nil || result.ExitCode < 0 {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	if int64(len(result.Stdout)+len(result.Stderr)) > input.MaxOutputBytes {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	return newRemoteExecutionStream(result), nil
}

func (session *RemoteSession) Read(ctx context.Context, input ReadRequest) (FileContent, error) {
	input, err := NormalizeReadRequest(input)
	if err != nil {
		return FileContent{}, err
	}
	projection, err := session.submitGenerated(ctx, remoteSessionOperationRead, remoteSessionOperationPayload{Path: input.Path, MaxBytes: input.MaxBytes})
	if err != nil {
		return FileContent{}, err
	}
	var result FileContent
	if err := projection.decodeSucceededResult(&result); err != nil || int64(len(result.Data)) > input.MaxBytes {
		return FileContent{}, domainsandbox.ErrProviderUnhealthy
	}
	return result, nil
}

func (session *RemoteSession) Write(ctx context.Context, input WriteRequest) error {
	input, err := NormalizeWriteRequest(input)
	if err != nil {
		return err
	}
	kind := remoteSessionOperationWrite
	if input.Append {
		kind = remoteSessionOperationAppend
	}
	projection, err := session.submitGenerated(ctx, kind, remoteSessionOperationPayload{Path: input.Path, Content: input.Content})
	if err != nil {
		return err
	}
	var result struct {
		Written bool `json:"written"`
	}
	if err := projection.decodeSucceededResult(&result); err != nil || !result.Written {
		return domainsandbox.ErrProviderUnhealthy
	}
	return nil
}

func (session *RemoteSession) List(ctx context.Context, input ListRequest) ([]FileEntry, error) {
	input, err := NormalizeListRequest(input)
	if err != nil {
		return nil, err
	}
	projection, err := session.submitGenerated(ctx, remoteSessionOperationList, remoteSessionOperationPayload{Path: input.Path, Limit: input.Limit})
	if err != nil {
		return nil, err
	}
	return projection.decodeFileEntries(input.Limit)
}

func (session *RemoteSession) Glob(ctx context.Context, input GlobRequest) ([]FileEntry, error) {
	input, err := NormalizeGlobRequest(input)
	if err != nil {
		return nil, err
	}
	projection, err := session.submitGenerated(ctx, remoteSessionOperationGlob, remoteSessionOperationPayload{Path: input.Path, Pattern: input.Pattern, Limit: input.Limit})
	if err != nil {
		return nil, err
	}
	return projection.decodeFileEntries(input.Limit)
}

func (session *RemoteSession) Grep(ctx context.Context, input GrepRequest) ([]GrepMatch, error) {
	input, err := NormalizeGrepRequest(input)
	if err != nil {
		return nil, err
	}
	projection, err := session.submitGenerated(ctx, remoteSessionOperationGrep, remoteSessionOperationPayload{Path: input.Path, Pattern: input.Pattern, Limit: input.Limit})
	if err != nil {
		return nil, err
	}
	var result []GrepMatch
	if err := projection.decodeSucceededResult(&result); err != nil || len(result) > input.Limit {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	for _, match := range result {
		if _, err := NormalizeGrepRequest(GrepRequest{Path: match.Path, Pattern: input.Pattern, Limit: 1}); err != nil || match.Line < 0 || match.ByteOffset < 0 || !utf8.ValidString(match.Text) {
			return nil, domainsandbox.ErrProviderUnhealthy
		}
	}
	return result, nil
}

func (session *RemoteSession) Replace(ctx context.Context, input ReplaceRequest) error {
	input, err := NormalizeReplaceRequest(input)
	if err != nil {
		return err
	}
	projection, err := session.submitGenerated(ctx, remoteSessionOperationReplace, remoteSessionOperationPayload{Path: input.Path, Old: input.Old, New: input.New})
	if err != nil {
		return err
	}
	var result struct {
		Replaced bool `json:"replaced"`
	}
	if err := projection.decodeSucceededResult(&result); err != nil || !result.Replaced {
		return domainsandbox.ErrProviderUnhealthy
	}
	return nil
}

func (session *RemoteSession) Download(ctx context.Context, input DownloadRequest) (io.ReadCloser, error) {
	input, err := NormalizeDownloadRequest(input)
	if err != nil {
		return nil, err
	}
	projection, err := session.submitGenerated(ctx, remoteSessionOperationDownload, remoteSessionOperationPayload{Path: input.Path, MaxBytes: input.MaxBytes})
	if err != nil {
		return nil, err
	}
	var result []byte
	if err := projection.decodeSucceededResult(&result); err != nil || int64(len(result)) > input.MaxBytes {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	return io.NopCloser(bytes.NewReader(result)), nil
}

// OperationStatus returns the Runner-owned operation projection for an ID
// previously returned or submitted through this Session. It never accepts an
// upstream Shell identifier.
func (session *RemoteSession) OperationStatus(ctx context.Context, operationID string) (SessionOperationStatus, error) {
	return session.operationRoute(ctx, http.MethodGet, operationID, "", "")
}

func (session *RemoteSession) CancelOperation(ctx context.Context, operationID string) (SessionOperationStatus, error) {
	return session.operationRoute(ctx, http.MethodPost, operationID, ":cancel", "")
}

func (session *RemoteSession) OperationEvents(ctx context.Context, operationID, afterEventID string) ([]SessionOperationEvent, error) {
	if session == nil || session.provider == nil || ctx == nil || !validRemoteSessionIdentifier(operationID) ||
		(afterEventID != "" && !validRemoteSessionIdentifier(afterEventID)) {
		return nil, domainsandbox.ErrInvalidInput
	}
	requestPath := session.operationPath(operationID) + "/events"
	response, err := session.provider.doSessionRequestWithHeaders(ctx, http.MethodGet, requestPath, nil, session.ref.Key, session.runID, operationID,
		maxRemoteSessionEventsBytes, map[string]string{"Last-Event-ID": afterEventID})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return decodeRemoteSessionEvents(ctx, response, operationID)
}

func (session *RemoteSession) operationRoute(ctx context.Context, method, operationID, suffix, afterEventID string) (SessionOperationStatus, error) {
	if session == nil || session.provider == nil || ctx == nil || !validRemoteSessionIdentifier(operationID) || afterEventID != "" {
		return SessionOperationStatus{}, domainsandbox.ErrInvalidInput
	}
	body := []byte(nil)
	if method == http.MethodPost {
		body = []byte("{}")
	}
	requestPath := session.operationPath(operationID) + suffix
	response, err := session.provider.doSessionRequest(ctx, method, requestPath, body, session.ref.Key, session.runID, operationID, MaxHealthResponseBodyBytes)
	if err != nil {
		return SessionOperationStatus{}, err
	}
	defer response.Body.Close()
	projection, err := decodeRemoteSessionOperation(ctx, response, session.ref.SessionID, operationID)
	if err != nil {
		return SessionOperationStatus{}, err
	}
	return projection.status(), nil
}

func (session *RemoteSession) submitGenerated(ctx context.Context, kind remoteSessionOperationKind, payload remoteSessionOperationPayload) (remoteSessionOperationProjection, error) {
	if session == nil || session.provider == nil || session.provider.newID == nil {
		return remoteSessionOperationProjection{}, domainsandbox.ErrInvalidInput
	}
	operationID, err := session.provider.newID()
	if err != nil || !validRemoteSessionIdentifier(operationID) {
		return remoteSessionOperationProjection{}, domainsandbox.ErrUnavailable
	}
	return session.submit(ctx, operationID, kind, session.provider.now().Add(defaultRemoteFileDeadline), payload)
}

func (session *RemoteSession) submit(ctx context.Context, operationID string, kind remoteSessionOperationKind, deadline time.Time, payload remoteSessionOperationPayload) (remoteSessionOperationProjection, error) {
	if session == nil || session.provider == nil || ctx == nil || !validRemoteSessionIdentifier(operationID) ||
		deadline.IsZero() || !deadline.After(session.provider.now()) || deadline.After(session.provider.now().Add(MaxSessionDeadlineAhead)) {
		return remoteSessionOperationProjection{}, domainsandbox.ErrInvalidInput
	}
	body, err := json.Marshal(remoteSessionOperationRequest{
		Schema: remoteSessionOperationSchema, OperationID: operationID, Kind: kind,
		Deadline: deadline.UTC().Format(time.RFC3339Nano), Payload: payload,
	})
	if err != nil || len(body) > MaxRequestBodyBytes {
		return remoteSessionOperationProjection{}, domainsandbox.ErrInvalidInput
	}
	response, err := session.provider.doSessionRequest(ctx, http.MethodPost, "/v1/sessions/"+session.ref.SessionID+"/operations", body,
		session.ref.Key, session.runID, operationID, maxRemoteSessionResponseBytes)
	if err != nil {
		return remoteSessionOperationProjection{}, err
	}
	defer response.Body.Close()
	return decodeRemoteSessionOperation(ctx, response, session.ref.SessionID, operationID)
}

func (session *RemoteSession) operationPath(operationID string) string {
	return "/v1/sessions/" + session.ref.SessionID + "/operations/" + operationID
}

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

type SessionOperationStatus struct {
	OperationID  string
	Kind         string
	State        SessionOperationState
	ReasonCode   string
	ResultDigest string
	Result       json.RawMessage
}

func (SessionOperationStatus) String() string {
	return "sandbox.SessionOperationStatus{result:<redacted>}"
}
func (SessionOperationStatus) GoString() string {
	return "sandbox.SessionOperationStatus{result:<redacted>}"
}
func (SessionOperationStatus) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.SessionOperationStatus{result:<redacted>}")
}

type SessionOperationEvent struct {
	EventID     string
	OperationID string
	State       SessionOperationState
	ReasonCode  string
}

type remoteSessionAcquireRequest struct {
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

type remoteSessionProjection struct {
	Schema            string                       `json:"schema"`
	SessionID         string                       `json:"session_id"`
	State             domainsandbox.SessionState   `json:"state"`
	RuntimeGeneration uint64                       `json:"runtime_generation"`
	Profile           domainsandbox.SessionProfile `json:"profile"`
}

type remoteSessionConfigurationProjection struct {
	Schema   string                               `json:"schema"`
	Version  uint64                               `json:"version"`
	Settings domainsandbox.SessionRuntimeSettings `json:"settings"`
}

type remoteSessionRuntimeStatusProjection struct {
	Schema               string `json:"schema"`
	Available            bool   `json:"available"`
	AppliedConfigVersion uint64 `json:"applied_config_version"`
	RuntimeGeneration    uint64 `json:"runtime_generation"`
	CoreEnabled          bool   `json:"core_enabled"`
	InteractiveEnabled   bool   `json:"interactive_enabled"`
	HostShellEnabled     bool   `json:"host_shell_enabled"`
	HostShellAvailable   bool   `json:"host_shell_available"`
	RawAIOReady          bool   `json:"raw_aio_ready"`
	GenerationState      string `json:"generation_state"`
	QueueDepth           int    `json:"queue_depth"`
	Running              int    `json:"running"`
	UsedWeight           int    `json:"used_weight"`
	TotalWeight          int    `json:"total_weight"`
	ActiveSessions       int    `json:"active_sessions"`
	IdleSessions         int    `json:"idle_sessions"`
	ActiveShells         int    `json:"active_shells"`
	IdleShells           int    `json:"idle_shells"`
	ReasonCode           string `json:"reason_code,omitempty"`
}

func validRemoteSessionRuntimeStatus(status remoteSessionRuntimeStatusProjection) bool {
	if status.Schema != remoteSessionRuntimeStatusSchema || status.AppliedConfigVersion == 0 || status.InteractiveEnabled ||
		status.QueueDepth < 0 || status.Running < 0 || status.UsedWeight < 0 || status.TotalWeight != 2 ||
		status.ActiveSessions < 0 || status.IdleSessions < 0 || status.ActiveShells < 0 || status.IdleShells < 0 ||
		status.UsedWeight > status.TotalWeight || status.Running > status.ActiveSessions || status.ActiveShells != status.Running ||
		status.HostShellAvailable && !status.HostShellEnabled || status.Available == (status.ReasonCode != "") ||
		status.ReasonCode != "" && !validRemoteSessionReasonCode(status.ReasonCode) {
		return false
	}
	switch status.GenerationState {
	case "disabled", "unknown":
		return status.RuntimeGeneration == 0 && !status.RawAIOReady
	case "recovering":
		return status.RuntimeGeneration > 0 && !status.RawAIOReady
	case "ready":
		return status.RuntimeGeneration > 0 && status.RawAIOReady
	default:
		return false
	}
}

func validRemoteSessionReasonCode(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character != '_' && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

type remoteSessionOperationKind string

const (
	remoteSessionOperationExec     remoteSessionOperationKind = "exec"
	remoteSessionOperationRead     remoteSessionOperationKind = "read"
	remoteSessionOperationWrite    remoteSessionOperationKind = "write"
	remoteSessionOperationAppend   remoteSessionOperationKind = "append"
	remoteSessionOperationList     remoteSessionOperationKind = "list"
	remoteSessionOperationGlob     remoteSessionOperationKind = "glob"
	remoteSessionOperationGrep     remoteSessionOperationKind = "grep"
	remoteSessionOperationReplace  remoteSessionOperationKind = "replace"
	remoteSessionOperationDownload remoteSessionOperationKind = "download"
)

type remoteSessionOperationPayload struct {
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

type remoteSessionOperationRequest struct {
	Schema      string                        `json:"schema"`
	OperationID string                        `json:"operation_id"`
	Kind        remoteSessionOperationKind    `json:"kind"`
	Deadline    string                        `json:"deadline"`
	Payload     remoteSessionOperationPayload `json:"payload"`
}

type remoteSessionOperationProjection struct {
	Schema       string                     `json:"schema"`
	SessionID    string                     `json:"session_id"`
	OperationID  string                     `json:"operation_id"`
	Kind         remoteSessionOperationKind `json:"kind"`
	State        SessionOperationState      `json:"state"`
	ReasonCode   string                     `json:"reason_code,omitempty"`
	ResultDigest string                     `json:"result_digest,omitempty"`
	Result       json.RawMessage            `json:"result,omitempty"`
}

func (projection remoteSessionOperationProjection) status() SessionOperationStatus {
	return SessionOperationStatus{OperationID: projection.OperationID, Kind: string(projection.Kind), State: projection.State, ReasonCode: projection.ReasonCode, ResultDigest: projection.ResultDigest, Result: append(json.RawMessage(nil), projection.Result...)}
}

func (projection remoteSessionOperationProjection) decodeSucceededResult(target any) error {
	if projection.State != SessionOperationSucceeded || len(projection.Result) == 0 || projection.ResultDigest == "" {
		return mapRemoteSessionOperationState(projection.State)
	}
	digest, err := base64.RawURLEncoding.DecodeString(projection.ResultDigest)
	actual := sha256.Sum256(projection.Result)
	if err != nil || len(digest) != sha256.Size || !bytes.Equal(digest, actual[:]) || scanStrictRemoteJSONValue(projection.Result) != nil ||
		validateCanonicalJSONFields(projection.Result, target) != nil {
		return domainsandbox.ErrProviderUnhealthy
	}
	decoder := json.NewDecoder(bytes.NewReader(projection.Result))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domainsandbox.ErrProviderUnhealthy
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return domainsandbox.ErrProviderUnhealthy
	}
	return nil
}

func (projection remoteSessionOperationProjection) decodeFileEntries(limit int) ([]FileEntry, error) {
	var result []FileEntry
	if err := projection.decodeSucceededResult(&result); err != nil || len(result) > limit {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	for _, entry := range result {
		if _, err := NormalizeListRequest(ListRequest{Path: entry.Path, Limit: 1}); err != nil || entry.Size < 0 || entry.Modified.IsZero() {
			return nil, domainsandbox.ErrProviderUnhealthy
		}
	}
	return result, nil
}

type remoteExecutionResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   []byte `json:"stdout"`
	Stderr   []byte `json:"stderr"`
}

type remoteSessionEventWire struct {
	Schema      string                `json:"schema"`
	EventID     string                `json:"event_id"`
	OperationID string                `json:"operation_id"`
	State       SessionOperationState `json:"state"`
	ReasonCode  string                `json:"reason_code,omitempty"`
}

func decodeRemoteSessionOperation(ctx context.Context, response *http.Response, sessionID, operationID string) (remoteSessionOperationProjection, error) {
	var projection remoteSessionOperationProjection
	if err := decodeStrictJSONResponseContext(ctx, response, maxRemoteSessionResponseBytes, &projection); err != nil ||
		projection.Schema != remoteSessionOperationSchema || projection.SessionID != sessionID || projection.OperationID != operationID ||
		!validRemoteSessionOperationKind(projection.Kind) || !validRemoteSessionOperationState(projection.State) || len(projection.ReasonCode) > 64 ||
		(projection.ResultDigest != "" && !validRemoteSessionDigest(projection.ResultDigest)) ||
		(len(projection.Result) > 0 && (projection.State != SessionOperationSucceeded || projection.ResultDigest == "")) {
		return remoteSessionOperationProjection{}, domainsandbox.ErrProviderUnhealthy
	}
	if len(projection.Result) > 0 {
		digest, err := base64.RawURLEncoding.DecodeString(projection.ResultDigest)
		actual := sha256.Sum256(projection.Result)
		if err != nil || !bytes.Equal(digest, actual[:]) || scanStrictRemoteJSONValue(projection.Result) != nil {
			return remoteSessionOperationProjection{}, domainsandbox.ErrProviderUnhealthy
		}
	}
	return projection, nil
}

func decodeRemoteSessionEvents(ctx context.Context, response *http.Response, operationID string) ([]SessionOperationEvent, error) {
	if response == nil || response.Body == nil || response.ContentLength > maxRemoteSessionEventsBytes {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	mediaType, parameters, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-ndjson" || len(parameters) != 0 {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteSessionEventsBytes+1))
	if err != nil {
		return nil, mapResponseBodyError(ctx, err)
	}
	if int64(len(body)) > maxRemoteSessionEventsBytes || !utf8.Valid(body) {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	lines := bytes.Split(body, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 2 {
		return nil, domainsandbox.ErrProviderUnhealthy
	}
	events := make([]SessionOperationEvent, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 || scanStrictJSONObject(line) != nil {
			return nil, domainsandbox.ErrProviderUnhealthy
		}
		var wire remoteSessionEventWire
		if validateCanonicalJSONFields(line, &wire) != nil {
			return nil, domainsandbox.ErrProviderUnhealthy
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&wire) != nil || wire.Schema != remoteSessionEventSchema || wire.OperationID != operationID ||
			!validRemoteSessionIdentifier(wire.EventID) || !validRemoteSessionEventState(wire.State) || len(wire.ReasonCode) > 64 {
			return nil, domainsandbox.ErrProviderUnhealthy
		}
		events = append(events, SessionOperationEvent{EventID: wire.EventID, OperationID: wire.OperationID, State: wire.State, ReasonCode: wire.ReasonCode})
	}
	return events, nil
}

type remoteExecutionStream struct {
	mu     sync.Mutex
	events []ExecutionEvent
	closed bool
}

func newRemoteExecutionStream(result remoteExecutionResult) *remoteExecutionStream {
	exitCode := result.ExitCode
	events := make([]ExecutionEvent, 0, 3)
	if len(result.Stdout) > 0 {
		events = append(events, ExecutionEvent{Kind: ExecutionEventStdout, Data: append([]byte(nil), result.Stdout...)})
	}
	if len(result.Stderr) > 0 {
		events = append(events, ExecutionEvent{Kind: ExecutionEventStderr, Data: append([]byte(nil), result.Stderr...)})
	}
	events = append(events, ExecutionEvent{Kind: ExecutionEventTerminal, ExitCode: &exitCode})
	return &remoteExecutionStream{events: events}
}

func (stream *remoteExecutionStream) Recv(ctx context.Context) (ExecutionEvent, error) {
	if stream == nil || ctx == nil {
		return ExecutionEvent{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return ExecutionEvent{}, err
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed || len(stream.events) == 0 {
		return ExecutionEvent{}, io.EOF
	}
	event := stream.events[0]
	stream.events = stream.events[1:]
	event.Data = append([]byte(nil), event.Data...)
	return event, nil
}

func (stream *remoteExecutionStream) Close() error {
	if stream == nil {
		return nil
	}
	stream.mu.Lock()
	stream.closed = true
	stream.events = nil
	stream.mu.Unlock()
	return nil
}

func mapRemoteSessionHTTPStatus(status int) error {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return domainsandbox.ErrCredentialInvalid
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity || status == http.StatusRequestEntityTooLarge:
		return domainsandbox.ErrInvalidInput
	case status == http.StatusNotFound:
		return domainsandbox.ErrSessionNotFound
	case status == http.StatusConflict:
		return domainsandbox.ErrVersionConflict
	case status == http.StatusTooManyRequests:
		return domainsandbox.ErrCapacityExhausted
	default:
		return domainsandbox.ErrUnavailable
	}
}

func mapRemoteSessionOperationState(state SessionOperationState) error {
	switch state {
	case SessionOperationCanceled:
		return context.Canceled
	case SessionOperationTimedOut:
		return context.DeadlineExceeded
	case SessionOperationAccepted, SessionOperationQueued, SessionOperationRunning, SessionOperationUnknown:
		return domainsandbox.ErrUnavailable
	case SessionOperationFailed:
		return domainsandbox.ErrExecutionForbidden
	default:
		return domainsandbox.ErrProviderUnhealthy
	}
}

func validRemoteSessionIdentifier(value string) bool {
	if value == "" || len(value) > domainsandbox.MaxSessionIdentifierBytes || !utf8.ValidString(value) {
		return false
	}
	for index := range value {
		character := value[index]
		if index == 0 && !remoteIdentifierAlphaNumeric(character) {
			return false
		}
		if !remoteIdentifierAlphaNumeric(character) && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func remoteIdentifierAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func validRemoteSessionScope(scope sandboxidentity.Scope) bool {
	switch scope {
	case sandboxidentity.ScopeAgent, sandboxidentity.ScopeMCPStdio, sandboxidentity.ScopeAppDev, sandboxidentity.ScopePlugin:
		return true
	default:
		return false
	}
}

func validRemoteSessionState(state domainsandbox.SessionState) bool {
	switch state {
	case domainsandbox.SessionStateActive, domainsandbox.SessionStateRecovering, domainsandbox.SessionStateReleased, domainsandbox.SessionStateDestroyed:
		return true
	default:
		return false
	}
}

func validRemoteSessionOperationKind(kind remoteSessionOperationKind) bool {
	switch kind {
	case remoteSessionOperationExec, remoteSessionOperationRead, remoteSessionOperationWrite, remoteSessionOperationAppend,
		remoteSessionOperationList, remoteSessionOperationGlob, remoteSessionOperationGrep, remoteSessionOperationReplace, remoteSessionOperationDownload:
		return true
	default:
		return false
	}
}

func validRemoteSessionOperationState(state SessionOperationState) bool {
	switch state {
	case SessionOperationAccepted, SessionOperationQueued, SessionOperationRunning, SessionOperationSucceeded, SessionOperationFailed,
		SessionOperationCanceled, SessionOperationTimedOut, SessionOperationUnknown:
		return true
	default:
		return false
	}
}

func validRemoteSessionEventState(state SessionOperationState) bool {
	return state == SessionOperationAccepted || state == SessionOperationSucceeded || state == SessionOperationFailed ||
		state == SessionOperationCanceled || state == SessionOperationTimedOut || state == SessionOperationUnknown
}

func validRemoteSessionDigest(value string) bool {
	digest, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(digest) == sha256.Size
}

func normalizeRemoteSessionAllowedEnvironment(input []string) ([]string, error) {
	if len(input) > MaxEnvVars {
		return nil, domainsandbox.ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(input))
	result := make([]string, len(input))
	for index, name := range input {
		if name == "" {
			return nil, domainsandbox.ErrInvalidInput
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, domainsandbox.ErrInvalidInput
		}
		seen[name] = struct{}{}
		result[index] = name
	}
	return result, nil
}

func newRemoteSessionID() (string, error) {
	raw := make([]byte, 18)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return "remote_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func scanStrictRemoteJSONValue(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || !utf8.Valid(trimmed) {
		return errRemoteJSONInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := scanRemoteJSONValue(decoder, 1); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errRemoteJSONInvalid
	}
	return nil
}

var _ RuntimeProvider = (*RemoteSessionProvider)(nil)
var _ SessionRuntimeProvider = (*RemoteSessionProvider)(nil)
var _ SandboxSession = (*RemoteSession)(nil)

// Keep string values out of diagnostic formatting paths.
func (*RemoteSessionProvider) String() string { return "RemoteSessionProvider{identity:<redacted>}" }
func (*RemoteSession) String() string         { return "RemoteSession{identity:<redacted>}" }

// Guard against accidental physical-path or Shell-id APIs being inferred from
// operation identifiers.
func validRemoteOperationID(value string) bool {
	return validRemoteSessionIdentifier(value) && !strings.Contains(strings.ToLower(value), "shell")
}
