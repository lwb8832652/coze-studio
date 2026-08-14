// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const (
	sessionOperationCancelPollInterval = 100 * time.Millisecond
	sessionQueuedCancelPersistTimeout  = time.Second
)

type SessionLookupRepository interface {
	GetRuntimeSessionByKey(context.Context, string, domainsandbox.SessionKey) (domainsandbox.RuntimeSession, error)
}

type SessionOperationScheduler interface {
	Accept(context.Context, SessionOperationInput) (SessionOperationRecord, error)
	TryStart(context.Context, string, string) (SessionOperationRecord, bool, error)
	Finish(context.Context, string, string, SessionOperationCompletion) (SessionOperationRecord, error)
	Get(context.Context, string, string) (SessionOperationRecord, error)
	RequestCancel(context.Context, string, string) (SessionOperationRecord, bool, error)
}

type SessionSettingsStore interface {
	GetSessionSettings(context.Context) (domainsandbox.SessionRuntimeSettings, error)
}

type SessionSettingsRuntimeApplier interface {
	ApplySessionSettings(domainsandbox.SessionRuntimeSettings) error
	AppliedSessionSettings() domainsandbox.SessionRuntimeSettings
}

type SessionServiceConfig struct {
	DeploymentID            string
	Manager                 infrasandbox.SandboxSessionManager
	Repository              SessionLookupRepository
	Scheduler               SessionOperationScheduler
	Settings                SessionSettingsStore
	SettingsApplier         SessionSettingsRuntimeApplier
	Clock                   func() time.Time
	AllowedEnvironmentNames []string
}

type SessionService struct {
	deploymentID string
	manager      infrasandbox.SandboxSessionManager
	repository   SessionLookupRepository
	scheduler    SessionOperationScheduler
	settings     SessionSettingsStore
	applier      SessionSettingsRuntimeApplier
	clock        func() time.Time
	allowedEnv   []string

	runningMu sync.Mutex
	running   map[string]context.CancelFunc
}

func NewSessionService(config SessionServiceConfig) (*SessionService, error) {
	if !validSessionProtocolIdentifier(config.DeploymentID) || config.Manager == nil || config.Repository == nil ||
		config.Scheduler == nil || config.Clock == nil {
		return nil, ErrConfiguration
	}
	allowedEnv, err := canonicalAllowedEnvironmentNames(config.AllowedEnvironmentNames)
	if err != nil {
		return nil, ErrConfiguration
	}
	return &SessionService{
		deploymentID: config.DeploymentID, manager: config.Manager, repository: config.Repository,
		scheduler: config.Scheduler, settings: config.Settings, applier: config.SettingsApplier,
		clock: config.Clock, allowedEnv: allowedEnv,
		running: make(map[string]context.CancelFunc),
	}, nil
}

func (service *SessionService) ResolveSessionIdentity(ctx context.Context, sessionID string, expected SessionStableIdentity) (SessionStableIdentity, error) {
	if expected.DeploymentID != service.deploymentID {
		return SessionStableIdentity{}, domainsandbox.ErrInvalidInput
	}
	key := sessionKeyFromIdentity(expected)
	row, err := service.loadRow(ctx, sessionID, &key)
	if err != nil {
		return SessionStableIdentity{}, err
	}
	return stableIdentityFromKey(row.Ref.Key), nil
}

func (service *SessionService) Acquire(ctx context.Context, command SessionAcquireCommand) (SessionProjection, error) {
	if service == nil || !stableIdentityMatchesClaims(command.Identity, command.Claims) || command.Identity.DeploymentID != service.deploymentID {
		return SessionProjection{}, domainsandbox.ErrInvalidInput
	}
	session, err := service.manager.Acquire(ctx, infrasandbox.AcquireSessionRequest{Key: sessionKeyFromIdentity(command.Identity)})
	if err != nil {
		return SessionProjection{}, err
	}
	return projectSessionRef(session.Ref(), domainsandbox.SessionStateActive), nil
}

func (service *SessionService) Get(ctx context.Context, command SessionRouteCommand) (SessionProjection, error) {
	row, err := service.loadRowForClaims(ctx, command.SessionID, command.Claims)
	if err != nil {
		return SessionProjection{}, err
	}
	return projectRuntimeSession(row), nil
}

func (service *SessionService) Release(ctx context.Context, command SessionRouteCommand) error {
	row, err := service.loadRowForClaims(ctx, command.SessionID, command.Claims)
	if err != nil {
		return err
	}
	return service.manager.Release(ctx, row.Ref)
}

func (service *SessionService) Destroy(ctx context.Context, command SessionRouteCommand) error {
	row, err := service.loadRowForClaims(ctx, command.SessionID, command.Claims)
	if err != nil {
		return err
	}
	return service.manager.Destroy(ctx, row.Ref)
}

func (service *SessionService) Recover(ctx context.Context, command SessionRouteCommand) (SessionProjection, error) {
	row, err := service.loadRowForClaims(ctx, command.SessionID, command.Claims)
	if err != nil {
		return SessionProjection{}, err
	}
	session, err := service.manager.Recover(ctx, row.Ref)
	if err != nil {
		return SessionProjection{}, err
	}
	return projectSessionRef(session.Ref(), domainsandbox.SessionStateActive), nil
}

func (service *SessionService) SubmitOperation(ctx context.Context, command SessionOperationCommand) (SessionOperationProjection, error) {
	if service == nil || command.SessionID == "" || command.OperationID == "" || command.Claims.OperationID != command.OperationID {
		return SessionOperationProjection{}, domainsandbox.ErrInvalidInput
	}
	row, err := service.loadRowForClaims(ctx, command.SessionID, command.Claims)
	if err != nil {
		return SessionOperationProjection{}, err
	}
	digest := sessionOperationCommandDigest(command)
	record, err := service.scheduler.Accept(ctx, SessionOperationInput{
		SessionID: command.SessionID, OperationID: command.OperationID, UserID: row.Ref.Key.UserID,
		Kind: command.Kind, RequestDigest: digest[:], Deadline: command.Deadline,
	})
	if err != nil {
		return SessionOperationProjection{}, err
	}
	if terminalSessionOperationState(record.State) {
		return projectOperation(record), nil
	}
	// A running record is already owned by a worker. Replays may retry a
	// queued record with the same request digest, but must never claim or
	// execute a running operation again.
	if record.State == SessionOperationRunning {
		return projectOperation(record), nil
	}
	operationCtx, cancel := context.WithDeadline(ctx, command.Deadline)
	record, started, err := service.waitForOperationStart(operationCtx, command.SessionID, command.OperationID)
	if err != nil || !started {
		cancel()
		return projectOperation(record), err
	}
	service.registerRunning(command.SessionID, command.OperationID, cancel)
	defer func() {
		service.unregisterRunning(command.SessionID, command.OperationID)
		cancel()
	}()

	watchCtx, stopWatching := context.WithCancel(operationCtx)
	watchDone := make(chan sessionOperationWatchResult, 1)
	go service.watchRunningOperation(watchCtx, command.SessionID, command.OperationID, cancel, watchDone)
	inlineResult, resultDigest, state, reason := service.executeOperation(operationCtx, row.Ref, command)
	stopWatching()
	watchResult := <-watchDone
	switch watchResult {
	case sessionOperationWatchCanceled:
		state, reason, inlineResult, resultDigest = SessionOperationCanceled, "SANDBOX_OPERATION_CANCELED", nil, nil
	case sessionOperationWatchUnknown:
		state, reason, inlineResult, resultDigest = SessionOperationUnknown, "SANDBOX_OPERATION_UNKNOWN", nil, nil
	default:
		current, currentErr := service.scheduler.Get(context.WithoutCancel(ctx), command.SessionID, command.OperationID)
		if currentErr != nil || current.State != SessionOperationRunning {
			state, reason, inlineResult, resultDigest = SessionOperationUnknown, "SANDBOX_OPERATION_UNKNOWN", nil, nil
		} else if current.CancelRequested {
			state, reason, inlineResult, resultDigest = SessionOperationCanceled, "SANDBOX_OPERATION_CANCELED", nil, nil
		}
	}
	record, err = service.scheduler.Finish(context.WithoutCancel(ctx), command.SessionID, command.OperationID,
		SessionOperationCompletion{State: state, ResultDigest: resultDigest, Result: inlineResult, ReasonCode: reason})
	if err != nil {
		return SessionOperationProjection{}, err
	}
	return projectOperation(record), nil
}

// waitForOperationStart keeps the authenticated submit request attached to its
// bounded request payload while the fair Redis queue is full. Core operation
// bodies are deliberately not persisted, so a queued request must either win a
// claim while its caller is still present or be atomically canceled. This also
// gives capacity release a live waiter without introducing an unsafe replay
// worker after process restart.
func (service *SessionService) waitForOperationStart(ctx context.Context, sessionID, operationID string) (SessionOperationRecord, bool, error) {
	for {
		if ctx.Err() != nil {
			return service.cancelAbandonedQueuedOperation(ctx, sessionID, operationID)
		}
		record, started, err := service.scheduler.TryStart(ctx, sessionID, operationID)
		if err != nil {
			if ctx.Err() != nil {
				return service.cancelAbandonedQueuedOperation(ctx, sessionID, operationID)
			}
			return SessionOperationRecord{}, false, err
		}
		if started || record.State == SessionOperationRunning || terminalSessionOperationState(record.State) {
			return record, started, nil
		}

		timer := time.NewTimer(sessionOperationCancelPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return service.cancelAbandonedQueuedOperation(ctx, sessionID, operationID)
		case <-timer.C:
		}
	}
}

func (service *SessionService) cancelAbandonedQueuedOperation(ctx context.Context, sessionID, operationID string) (SessionOperationRecord, bool, error) {
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sessionQueuedCancelPersistTimeout)
	defer cancel()
	record, _, err := service.scheduler.RequestCancel(cancelCtx, sessionID, operationID)
	if err != nil {
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	return record, false, nil
}

func (service *SessionService) GetOperation(ctx context.Context, command SessionOperationRouteCommand) (SessionOperationProjection, error) {
	if _, err := service.loadRowForClaims(ctx, command.SessionID, command.Claims); err != nil {
		return SessionOperationProjection{}, err
	}
	record, err := service.scheduler.Get(ctx, command.SessionID, command.OperationID)
	if err != nil {
		return SessionOperationProjection{}, err
	}
	return projectOperation(record), nil
}

func (service *SessionService) OperationEvents(ctx context.Context, command SessionOperationRouteCommand) ([]SessionOperationEvent, error) {
	projection, err := service.GetOperation(ctx, command)
	if err != nil {
		return nil, err
	}
	accepted := SessionOperationEvent{Schema: sessionEventSchemaV1, EventID: command.OperationID + "_accepted", OperationID: command.OperationID, State: SessionOperationAccepted}
	terminal := SessionOperationEvent{Schema: sessionEventSchemaV1, EventID: command.OperationID + "_terminal", OperationID: command.OperationID, State: projection.State, ReasonCode: projection.ReasonCode}
	switch {
	case command.AfterEventID == "":
		if terminalSessionOperationState(projection.State) {
			return []SessionOperationEvent{accepted, terminal}, nil
		}
		return []SessionOperationEvent{accepted}, nil
	case command.AfterEventID == accepted.EventID && terminalSessionOperationState(projection.State):
		return []SessionOperationEvent{terminal}, nil
	case command.AfterEventID == terminal.EventID:
		return []SessionOperationEvent{}, nil
	default:
		return nil, domainsandbox.ErrInvalidInput
	}
}

func (service *SessionService) CancelOperation(ctx context.Context, command SessionOperationRouteCommand) (SessionOperationProjection, error) {
	if _, err := service.loadRowForClaims(ctx, command.SessionID, command.Claims); err != nil {
		return SessionOperationProjection{}, err
	}
	record, immediate, err := service.scheduler.RequestCancel(ctx, command.SessionID, command.OperationID)
	if err != nil {
		return SessionOperationProjection{}, err
	}
	if !immediate && record.State == SessionOperationRunning {
		service.cancelRunning(command.SessionID, command.OperationID)
	}
	return projectOperation(record), nil
}

func (service *SessionService) SessionConfiguration(ctx context.Context, claims sandboxidentity.SessionRequest) (SessionConfigurationProjection, error) {
	if service == nil || service.settings == nil || service.applier == nil || !validSessionConfigurationClaims(claims, claims.RequestDigest) || claims.DeploymentID != service.deploymentID {
		return SessionConfigurationProjection{}, ErrUnavailable
	}
	desired, err := service.settings.GetSessionSettings(ctx)
	if err != nil || desired.Version == 0 {
		return SessionConfigurationProjection{}, err
	}
	applied := service.applier.AppliedSessionSettings()
	if _, err := domainsandbox.NormalizeSessionRuntimeSettings(applied); err != nil || applied.Version == 0 || applied.Version > desired.Version {
		return SessionConfigurationProjection{}, ErrUnavailable
	}
	return SessionConfigurationProjection{Schema: sessionConfigurationSchemaV1, Version: applied.Version, Settings: applied}, nil
}

func (service *SessionService) ApplySessionConfiguration(ctx context.Context, command SessionConfigurationCommand) (SessionConfigurationProjection, error) {
	if service == nil || service.settings == nil || service.applier == nil ||
		!validSessionConfigurationClaims(command.Claims, command.Claims.RequestDigest) || command.Claims.DeploymentID != service.deploymentID ||
		command.Version == 0 || command.Settings.Version != command.Version {
		return SessionConfigurationProjection{}, ErrUnavailable
	}
	current := service.applier.AppliedSessionSettings()
	if _, err := domainsandbox.NormalizeSessionRuntimeSettings(current); err != nil || current.Version == 0 {
		return SessionConfigurationProjection{}, ErrUnavailable
	}
	desired, err := service.settings.GetSessionSettings(ctx)
	if err != nil {
		return SessionConfigurationProjection{}, err
	}
	if command.Version != desired.Version || !sameSessionSettingsPayload(command.Settings, desired) {
		return SessionConfigurationProjection{}, domainsandbox.ErrVersionConflict
	}
	if current.Version > desired.Version || current.Version == desired.Version && !sameSessionSettingsPayload(current, desired) {
		return SessionConfigurationProjection{}, domainsandbox.ErrVersionConflict
	}
	if current.Version == desired.Version {
		return SessionConfigurationProjection{Schema: sessionConfigurationSchemaV1, Version: current.Version, Settings: current}, nil
	}
	if err := service.applier.ApplySessionSettings(desired); err != nil {
		return SessionConfigurationProjection{}, ErrUnavailable
	}
	applied := service.applier.AppliedSessionSettings()
	if applied.Version != desired.Version || !sameSessionSettingsPayload(applied, desired) {
		return SessionConfigurationProjection{}, ErrUnavailable
	}
	return SessionConfigurationProjection{Schema: sessionConfigurationSchemaV1, Version: applied.Version, Settings: applied}, nil
}

func sameSessionSettingsPayload(left, right domainsandbox.SessionRuntimeSettings) bool {
	left.Version, left.UpdatedBy = 0, 0
	right.Version, right.UpdatedBy = 0, 0
	return left == right
}

func (service *SessionService) executeOperation(ctx context.Context, ref domainsandbox.SessionRef, command SessionOperationCommand) (json.RawMessage, []byte, SessionOperationState, string) {
	session, err := service.manager.Get(ctx, ref)
	if err != nil {
		return nil, nil, operationStateForError(err), operationReasonForError(err)
	}
	var result any
	switch command.Kind {
	case SessionOperationExec:
		stream, callErr := session.Exec(ctx, infrasandbox.ExecRequest{OperationID: command.OperationID, Argv: command.Payload.Argv, Command: command.Payload.Command, Env: command.Payload.Env, CWD: command.Payload.CWD, Deadline: command.Deadline, MaxOutputBytes: command.Payload.MaxOutputBytes})
		if callErr == nil {
			result, callErr = consumeExecutionStream(ctx, stream, command.Payload.MaxOutputBytes)
		}
		err = callErr
	case SessionOperationRead:
		result, err = session.Read(ctx, infrasandbox.ReadRequest{Path: command.Payload.Path, MaxBytes: command.Payload.MaxBytes})
	case SessionOperationWrite, SessionOperationAppend:
		err = session.Write(ctx, infrasandbox.WriteRequest{Path: command.Payload.Path, Content: command.Payload.Content, Append: command.Kind == SessionOperationAppend})
		result = map[string]bool{"written": err == nil}
	case SessionOperationList:
		result, err = session.List(ctx, infrasandbox.ListRequest{Path: command.Payload.Path, Limit: command.Payload.Limit})
	case SessionOperationGlob:
		result, err = session.Glob(ctx, infrasandbox.GlobRequest{Path: command.Payload.Path, Pattern: command.Payload.Pattern, Limit: command.Payload.Limit})
	case SessionOperationGrep:
		result, err = session.Grep(ctx, infrasandbox.GrepRequest{Path: command.Payload.Path, Pattern: command.Payload.Pattern, Limit: command.Payload.Limit})
	case SessionOperationReplace:
		err = session.Replace(ctx, infrasandbox.ReplaceRequest{Path: command.Payload.Path, Old: command.Payload.Old, New: command.Payload.New})
		result = map[string]bool{"replaced": err == nil}
	case SessionOperationDownload:
		var reader io.ReadCloser
		reader, err = session.Download(ctx, infrasandbox.DownloadRequest{Path: command.Payload.Path, MaxBytes: command.Payload.MaxBytes})
		if err == nil {
			result, err = readBoundedDownload(reader, command.Payload.MaxBytes)
		}
	default:
		err = domainsandbox.ErrInvalidInput
	}
	if err != nil {
		return nil, nil, operationStateForError(err), operationReasonForError(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > maxSessionInlineResultBytes {
		return nil, nil, SessionOperationFailed, "SANDBOX_RESULT_TOO_LARGE"
	}
	digest := sha256.Sum256(encoded)
	return json.RawMessage(encoded), digest[:], SessionOperationSucceeded, ""
}

type sessionOperationWatchResult uint8

const (
	sessionOperationWatchStopped sessionOperationWatchResult = iota
	sessionOperationWatchCanceled
	sessionOperationWatchUnknown
)

func (service *SessionService) watchRunningOperation(ctx context.Context, sessionID, operationID string, cancel context.CancelFunc, done chan<- sessionOperationWatchResult) {
	result := sessionOperationWatchStopped
	defer func() { done <- result }()
	ticker := time.NewTicker(sessionOperationCancelPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			observationCtx, stopObservation := context.WithTimeout(ctx, sessionOperationCancelPollInterval)
			record, err := service.scheduler.Get(observationCtx, sessionID, operationID)
			stopObservation()
			if ctx.Err() != nil {
				return
			}
			if err != nil || record.SessionID != sessionID || record.OperationID != operationID || record.State != SessionOperationRunning {
				result = sessionOperationWatchUnknown
				cancel()
				return
			}
			if record.CancelRequested {
				result = sessionOperationWatchCanceled
				cancel()
				return
			}
		}
	}
}

func (service *SessionService) loadRowForClaims(ctx context.Context, sessionID string, claims sandboxidentity.SessionRequest) (domainsandbox.RuntimeSession, error) {
	if !validSessionClaims(claims, claims.RequestDigest) {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrInvalidInput
	}
	key := domainsandbox.SessionKey{DeploymentID: service.deploymentID, ProviderID: claims.ProviderID, SpaceID: claims.SpaceID, UserID: claims.UserID, ThreadID: claims.ThreadID, Profile: domainsandbox.SessionProfile(claims.Profile)}
	return service.loadRow(ctx, sessionID, &key)
}

func (service *SessionService) loadRow(ctx context.Context, sessionID string, key *domainsandbox.SessionKey) (domainsandbox.RuntimeSession, error) {
	if service == nil || service.repository == nil || !validSessionProtocolIdentifier(sessionID) {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrInvalidInput
	}
	if key == nil {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrInvalidInput
	}
	normalized, err := domainsandbox.NormalizeSessionKey(*key)
	if err != nil || normalized.DeploymentID != service.deploymentID {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrInvalidInput
	}
	row, err := service.repository.GetRuntimeSessionByKey(ctx, sessionID, normalized)
	if err != nil || row.Ref.SessionID != sessionID || row.Ref.Key != normalized {
		if err != nil {
			return domainsandbox.RuntimeSession{}, err
		}
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrSessionNotFound
	}
	if row.ExpiresAt.IsZero() || !row.ExpiresAt.After(service.clock().UTC()) {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrUnavailable
	}
	return row, nil
}

func (service *SessionService) registerRunning(sessionID, operationID string, cancel context.CancelFunc) {
	service.runningMu.Lock()
	defer service.runningMu.Unlock()
	service.running[sessionID+"\x00"+operationID] = cancel
}
func (service *SessionService) unregisterRunning(sessionID, operationID string) {
	service.runningMu.Lock()
	defer service.runningMu.Unlock()
	delete(service.running, sessionID+"\x00"+operationID)
}
func (service *SessionService) cancelRunning(sessionID, operationID string) {
	service.runningMu.Lock()
	cancel := service.running[sessionID+"\x00"+operationID]
	service.runningMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func stableIdentityFromKey(key domainsandbox.SessionKey) SessionStableIdentity {
	return SessionStableIdentity{DeploymentID: key.DeploymentID, ProviderID: key.ProviderID, SpaceID: key.SpaceID, UserID: key.UserID, ThreadID: key.ThreadID, Profile: key.Profile}
}
func sessionKeyFromIdentity(identity SessionStableIdentity) domainsandbox.SessionKey {
	return domainsandbox.SessionKey{DeploymentID: identity.DeploymentID, ProviderID: identity.ProviderID, SpaceID: identity.SpaceID, UserID: identity.UserID, ThreadID: identity.ThreadID, Profile: identity.Profile}
}
func projectRuntimeSession(row domainsandbox.RuntimeSession) SessionProjection {
	return projectSessionRef(row.Ref, row.State)
}
func projectSessionRef(ref domainsandbox.SessionRef, state domainsandbox.SessionState) SessionProjection {
	return SessionProjection{Schema: sessionProjectionSchemaV1, SessionID: ref.SessionID, State: state, RuntimeGeneration: ref.RuntimeGeneration, Profile: ref.Key.Profile}
}
func projectOperation(record SessionOperationRecord) SessionOperationProjection {
	projection := SessionOperationProjection{Schema: sessionOperationSchemaV1, SessionID: record.SessionID, OperationID: record.OperationID, Kind: record.Kind, State: record.State, ReasonCode: record.ReasonCode}
	if len(record.ResultDigest) == sha256.Size {
		projection.ResultDigest = base64.RawURLEncoding.EncodeToString(record.ResultDigest)
	}
	if record.State == SessionOperationSucceeded && len(record.Result) != 0 {
		projection.Result = append(json.RawMessage(nil), record.Result...)
	}
	return projection
}
func sessionOperationCommandDigest(command SessionOperationCommand) [sha256.Size]byte {
	encoded, _ := json.Marshal(struct {
		Kind     SessionOperationKind    `json:"kind"`
		Deadline string                  `json:"deadline"`
		Payload  SessionOperationPayload `json:"payload"`
	}{command.Kind, command.Deadline.UTC().Format(time.RFC3339Nano), command.Payload})
	return sha256.Sum256(encoded)
}
func operationStateForError(err error) SessionOperationState {
	switch aio.ReasonCode(err) {
	case aio.ReasonUpstreamCancelled:
		return SessionOperationCanceled
	case aio.ReasonUpstreamTimeout:
		return SessionOperationTimedOut
	}
	if errors.Is(err, context.Canceled) {
		return SessionOperationCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return SessionOperationTimedOut
	}
	return SessionOperationFailed
}
func operationReasonForError(err error) string {
	switch operationStateForError(err) {
	case SessionOperationCanceled:
		return "SANDBOX_OPERATION_CANCELED"
	case SessionOperationTimedOut:
		return "SANDBOX_OPERATION_TIMED_OUT"
	default:
		return "SANDBOX_OPERATION_FAILED"
	}
}

type executionTerminalResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   []byte `json:"stdout"`
	Stderr   []byte `json:"stderr"`
}

func consumeExecutionStream(ctx context.Context, stream infrasandbox.ExecutionStream, maximum int64) (executionTerminalResult, error) {
	if stream == nil {
		return executionTerminalResult{}, ErrUnavailable
	}
	defer stream.Close()
	var result executionTerminalResult
	for {
		event, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			return executionTerminalResult{}, err
		}
		switch event.Kind {
		case infrasandbox.ExecutionEventStdout:
			result.Stdout = append(result.Stdout, event.Data...)
		case infrasandbox.ExecutionEventStderr:
			result.Stderr = append(result.Stderr, event.Data...)
		case infrasandbox.ExecutionEventTerminal:
			if event.ExitCode == nil {
				return executionTerminalResult{}, ErrUnavailable
			}
			result.ExitCode = *event.ExitCode
			return result, nil
		default:
			return executionTerminalResult{}, ErrUnavailable
		}
		if int64(len(result.Stdout)+len(result.Stderr)) > maximum {
			return executionTerminalResult{}, ErrUnavailable
		}
	}
}
func readBoundedDownload(reader io.ReadCloser, maximum int64) ([]byte, error) {
	if reader == nil {
		return nil, ErrUnavailable
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, ErrUnavailable
	}
	return data, nil
}
