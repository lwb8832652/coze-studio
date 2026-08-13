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
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestSessionServiceAcquiresRunsAndProjectsTerminalOperation(t *testing.T) {
	service, _ := newSessionServiceFixture(t)
	claims := fixtureSessionClaims("acquire_01")
	acquired, err := service.Acquire(context.Background(), SessionAcquireCommand{
		Identity: fixtureStableIdentity(), Claims: claims,
	})
	if err != nil || acquired.State != domainsandbox.SessionStateActive || acquired.SessionID == "" {
		t.Fatalf("Acquire() = %#v, %v", acquired, err)
	}

	operationClaims := fixtureSessionClaims("operation_01")
	operation, err := service.SubmitOperation(context.Background(), SessionOperationCommand{
		SessionID: acquired.SessionID, OperationID: "operation_01", Kind: SessionOperationWrite,
		Deadline: time.Now().UTC().Add(time.Minute),
		Payload:  SessionOperationPayload{Path: "/mnt/user-data/workspace/result.txt", Content: []byte("ok")},
		Claims:   operationClaims,
	})
	if err != nil || operation.State != SessionOperationSucceeded || operation.ResultDigest == "" {
		t.Fatalf("SubmitOperation() = %#v, %v", operation, err)
	}
	if string(operation.Result) != `{"written":true}` {
		t.Fatalf("SubmitOperation().Result = %s", operation.Result)
	}
	projected, err := service.GetOperation(context.Background(), SessionOperationRouteCommand{
		SessionID: acquired.SessionID, OperationID: "operation_01", Claims: operationClaims,
	})
	if err != nil || projected.State != SessionOperationSucceeded || projected.ResultDigest != operation.ResultDigest || !jsonEqual(projected.Result, operation.Result) {
		t.Fatalf("GetOperation() = %#v, %v", projected, err)
	}
}

func TestSessionServiceReturnsInlineReadAndExecResultsOnlyToSynchronousSubmit(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	manager.execStdout = []byte("hello")
	tests := []struct {
		name    string
		command SessionOperationCommand
		assert  func(*testing.T, json.RawMessage)
	}{
		{
			name: "read",
			command: SessionOperationCommand{SessionID: manager.ref.SessionID, OperationID: "operation_read", Kind: SessionOperationRead,
				Deadline: time.Now().UTC().Add(time.Minute), Payload: SessionOperationPayload{Path: "/mnt/user-data/workspace/result.txt", MaxBytes: 1024}, Claims: fixtureSessionClaims("operation_read")},
			assert: func(t *testing.T, result json.RawMessage) {
				var content infrasandbox.FileContent
				if err := json.Unmarshal(result, &content); err != nil || string(content.Data) != "ok" {
					t.Fatalf("read result = %s, %v", result, err)
				}
			},
		},
		{
			name: "exec",
			command: SessionOperationCommand{SessionID: manager.ref.SessionID, OperationID: "operation_exec", Kind: SessionOperationExec,
				Deadline: time.Now().UTC().Add(time.Minute), Payload: SessionOperationPayload{Command: "printf hello", CWD: "/mnt/user-data/workspace", MaxOutputBytes: 1024}, Claims: fixtureSessionClaims("operation_exec")},
			assert: func(t *testing.T, result json.RawMessage) {
				var terminal executionTerminalResult
				if err := json.Unmarshal(result, &terminal); err != nil || terminal.ExitCode != 0 || string(terminal.Stdout) != "hello" {
					t.Fatalf("exec result = %s, %v", result, err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection, err := service.SubmitOperation(context.Background(), test.command)
			if err != nil || projection.State != SessionOperationSucceeded || len(projection.Result) == 0 {
				t.Fatalf("SubmitOperation() = %#v, %v", projection, err)
			}
			digest := sha256.Sum256(projection.Result)
			if projection.ResultDigest != base64.RawURLEncoding.EncodeToString(digest[:]) {
				t.Fatalf("result digest = %q for %s", projection.ResultDigest, projection.Result)
			}
			test.assert(t, projection.Result)
			stored, err := service.GetOperation(context.Background(), SessionOperationRouteCommand{
				SessionID: test.command.SessionID, OperationID: test.command.OperationID, Claims: test.command.Claims,
			})
			if err != nil || stored.ResultDigest != projection.ResultDigest || !jsonEqual(stored.Result, projection.Result) {
				t.Fatalf("GetOperation() = %#v, %v", stored, err)
			}
		})
	}
}

func TestSessionServiceRecoversBoundedSuccessfulResultAfterSubmitResponseLoss(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	operationID := "operation_result_recovery"
	command := SessionOperationCommand{
		SessionID: manager.ref.SessionID, OperationID: operationID, Kind: SessionOperationRead,
		Deadline: time.Now().UTC().Add(time.Minute),
		Payload:  SessionOperationPayload{Path: "/mnt/user-data/workspace/result.txt", MaxBytes: 1024},
		Claims:   fixtureSessionClaims(operationID),
	}
	first, err := service.SubmitOperation(context.Background(), command)
	if err != nil || first.State != SessionOperationSucceeded || len(first.Result) == 0 {
		t.Fatalf("SubmitOperation() = %#v, %v", first, err)
	}
	got, err := service.GetOperation(context.Background(), SessionOperationRouteCommand{
		SessionID: command.SessionID, OperationID: operationID, Claims: command.Claims,
	})
	if err != nil || got.State != SessionOperationSucceeded || !jsonEqual(got.Result, first.Result) || got.ResultDigest != first.ResultDigest {
		t.Fatalf("GetOperation() = %#v, %v; want recovered result %s", got, err, first.Result)
	}
	replayed, err := service.SubmitOperation(context.Background(), command)
	if err != nil || !jsonEqual(replayed.Result, first.Result) || replayed.ResultDigest != first.ResultDigest {
		t.Fatalf("replayed SubmitOperation() = %#v, %v", replayed, err)
	}
}

func jsonEqual(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func TestSessionServiceCancelsQueuedRequestWhenItsCallerStopsWaiting(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	service.scheduler = &blockingCoreScheduler{}
	command := SessionOperationCommand{
		SessionID: "550e8400-e29b-41d4-a716-446655440000", OperationID: "operation_02", Kind: SessionOperationExec,
		Deadline: time.Now().UTC().Add(time.Minute), Payload: SessionOperationPayload{Command: "printf ok", CWD: "/mnt/user-data/workspace", MaxOutputBytes: 1024},
		Claims: fixtureSessionClaims("operation_02"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	projection, err := service.SubmitOperation(ctx, command)
	if err != nil || projection.State != SessionOperationCanceled || manager.execCalls != 0 {
		t.Fatalf("SubmitOperation() = %#v, %v; exec calls = %d", projection, err, manager.execCalls)
	}
}

func TestSessionServiceWaitsForQueueCapacityWithoutClientResubmissionAndNeverReplaysRunningOperation(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	scheduler := service.scheduler.(*memoryCoreScheduler)
	scheduler.start = false
	queued := SessionOperationCommand{
		SessionID: manager.ref.SessionID, OperationID: "operation_queued_replay", Kind: SessionOperationWrite,
		Deadline: time.Now().UTC().Add(time.Minute), Payload: SessionOperationPayload{Path: "/mnt/user-data/workspace/result.txt", Content: []byte("ok")},
		Claims: fixtureSessionClaims("operation_queued_replay"),
	}
	done := make(chan SessionOperationProjection, 1)
	errDone := make(chan error, 1)
	go func() {
		projection, err := service.SubmitOperation(context.Background(), queued)
		done <- projection
		errDone <- err
	}()
	deadline := time.After(time.Second)
	for scheduler.tryStarts() == 0 {
		select {
		case <-deadline:
			t.Fatal("queued operation never attempted a fair claim")
		case <-time.After(time.Millisecond):
		}
	}
	scheduler.setStart(true)
	var first SessionOperationProjection
	select {
	case first = <-done:
		if err := <-errDone; err != nil {
			t.Fatalf("queued SubmitOperation() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued operation did not start after capacity became available")
	}
	if first.State != SessionOperationSucceeded || manager.writeCalls != 1 || scheduler.tryStarts() < 2 {
		t.Fatalf("queued SubmitOperation() = %#v; write/try-start=%d/%d", first, manager.writeCalls, scheduler.tryStarts())
	}

	running := queued
	running.OperationID = "operation_running_replay"
	running.Claims = fixtureSessionClaims(running.OperationID)
	digest := sessionOperationCommandDigest(running)
	scheduler.putRunning(running.SessionID, running.OperationID, running.Kind, digest)
	beforeTryStart := scheduler.tryStarts()
	projection, err := service.SubmitOperation(context.Background(), running)
	if err != nil || projection.State != SessionOperationRunning || manager.writeCalls != 1 || scheduler.tryStarts() != beforeTryStart {
		t.Fatalf("running replay SubmitOperation() = %#v, %v; write/try-start=%d/%d", projection, err, manager.writeCalls, scheduler.tryStarts())
	}
}

func TestSessionServiceRunningCancellationPropagatesAcrossRunnerInstancesAndDoesNotBecomeSuccess(t *testing.T) {
	worker, manager := newSessionServiceFixture(t)
	manager.blockExec = true
	scheduler := newMemoryCoreScheduler()
	scheduler.running = make(chan struct{})
	worker.scheduler = scheduler
	canceler, err := NewSessionService(SessionServiceConfig{
		DeploymentID: "runner-a", Manager: manager,
		Repository: &recordingSessionLookup{row: domainsandbox.RuntimeSession{Ref: manager.ref, State: domainsandbox.SessionStateActive, UpstreamShellID: "shell_01", Version: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)}},
		Scheduler:  scheduler, Clock: func() time.Time { return time.Now().UTC() }, AllowedEnvironmentNames: []string{"LANG"},
	})
	if err != nil {
		t.Fatal(err)
	}
	operationID := "operation_cancel"
	command := SessionOperationCommand{SessionID: "550e8400-e29b-41d4-a716-446655440000", OperationID: operationID,
		Kind: SessionOperationExec, Deadline: time.Now().UTC().Add(time.Minute),
		Payload: SessionOperationPayload{Command: "sleep 30", CWD: "/mnt/user-data/workspace", MaxOutputBytes: 1024}, Claims: fixtureSessionClaims(operationID)}
	done := make(chan SessionOperationProjection, 1)
	go func() { projection, _ := worker.SubmitOperation(context.Background(), command); done <- projection }()
	<-scheduler.running
	canceled, err := canceler.CancelOperation(context.Background(), SessionOperationRouteCommand{SessionID: command.SessionID, OperationID: operationID, Claims: command.Claims})
	if err != nil || canceled.State != SessionOperationRunning {
		t.Fatalf("CancelOperation() = %#v, %v", canceled, err)
	}
	select {
	case terminal := <-done:
		if terminal.State != SessionOperationCanceled {
			t.Fatalf("terminal = %#v", terminal)
		}
	case <-time.After(time.Second):
		t.Fatal("running operation was not canceled")
	}
}

func TestSessionServiceRunningWorkerFailsClosedUnknownWhenStoreObservationFails(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	manager.blockExec = true
	scheduler := newMemoryCoreScheduler()
	scheduler.running = make(chan struct{})
	service.scheduler = scheduler
	operationID := "operation_observation_error"
	command := SessionOperationCommand{SessionID: manager.ref.SessionID, OperationID: operationID, Kind: SessionOperationExec,
		Deadline: time.Now().UTC().Add(time.Minute), Payload: SessionOperationPayload{Command: "sleep 30", CWD: "/mnt/user-data/workspace", MaxOutputBytes: 1024}, Claims: fixtureSessionClaims(operationID)}
	done := make(chan SessionOperationProjection, 1)
	go func() { projection, _ := service.SubmitOperation(context.Background(), command); done <- projection }()
	<-scheduler.running
	scheduler.failObservation(errors.New("redis unavailable"))
	select {
	case terminal := <-done:
		if terminal.State != SessionOperationUnknown || terminal.ReasonCode != "SANDBOX_OPERATION_UNKNOWN" || len(terminal.Result) != 0 {
			t.Fatalf("terminal = %#v", terminal)
		}
	case <-time.After(time.Second):
		t.Fatal("running operation was not fenced after observation failure")
	}
}

func TestSessionServiceClassifiesSanitizedAIOInterruptions(t *testing.T) {
	tests := []struct {
		name       string
		reasonCode string
		wantState  SessionOperationState
		wantReason string
	}{
		{name: "timeout", reasonCode: aio.ReasonUpstreamTimeout, wantState: SessionOperationTimedOut, wantReason: "SANDBOX_OPERATION_TIMED_OUT"},
		{name: "cancel", reasonCode: aio.ReasonUpstreamCancelled, wantState: SessionOperationCanceled, wantReason: "SANDBOX_OPERATION_CANCELED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, manager := newSessionServiceFixture(t)
			transportErr := context.DeadlineExceeded
			if test.reasonCode == aio.ReasonUpstreamCancelled {
				transportErr = context.Canceled
			}
			client, clientErr := aio.NewUpstreamClient(aio.UpstreamClientConfig{
				BaseURL: "http://aio.invalid", HTTPClient: &http.Client{Timeout: time.Second, Transport: errorRoundTripper{err: transportErr}},
			})
			if clientErr != nil {
				t.Fatal(clientErr)
			}
			manager.execErr = client.Health(context.Background())
			if aio.ReasonCode(manager.execErr) != test.reasonCode {
				t.Fatalf("fixture reason = %q, want %q", aio.ReasonCode(manager.execErr), test.reasonCode)
			}
			operationID := "operation_" + test.name
			projection, err := service.SubmitOperation(context.Background(), SessionOperationCommand{
				SessionID: manager.ref.SessionID, OperationID: operationID, Kind: SessionOperationExec,
				Deadline: time.Now().UTC().Add(time.Minute),
				Payload:  SessionOperationPayload{Command: "sleep 1", CWD: "/mnt/user-data/workspace", MaxOutputBytes: 1024},
				Claims:   fixtureSessionClaims(operationID),
			})
			if err != nil || projection.State != test.wantState || projection.ReasonCode != test.wantReason {
				t.Fatalf("SubmitOperation() = %#v, %v; want %s/%s", projection, err, test.wantState, test.wantReason)
			}
		})
	}
}

type errorRoundTripper struct{ err error }

func (transport errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, transport.err
}

func TestSessionServiceAllowedEnvironmentNamesRejectDuplicatesAndCanonicalize(t *testing.T) {
	_, manager := newSessionServiceFixture(t)
	base := SessionServiceConfig{
		DeploymentID: "runner-a", Manager: manager,
		Repository: &recordingSessionLookup{row: domainsandbox.RuntimeSession{Ref: manager.ref, State: domainsandbox.SessionStateActive, ExpiresAt: time.Now().UTC().Add(time.Hour)}},
		Scheduler:  newMemoryCoreScheduler(), Clock: func() time.Time { return time.Now().UTC() },
	}
	for name, names := range map[string][]string{
		"duplicate": {"LANG", "LANG"},
		"invalid":   {"NOT-AN-ENV"},
	} {
		t.Run(name, func(t *testing.T) {
			config := base
			config.AllowedEnvironmentNames = names
			if _, err := NewSessionService(config); !errors.Is(err, ErrConfiguration) {
				t.Fatalf("NewSessionService(%v) = %v", names, err)
			}
		})
	}
	base.AllowedEnvironmentNames = []string{"TZ", "LANG"}
	service, err := NewSessionService(base)
	if err != nil || strings.Join(service.allowedEnv, ",") != "LANG,TZ" {
		t.Fatalf("NewSessionService() allowed env = %v, %v", service.allowedEnv, err)
	}
}

func TestSessionServiceRejectsMismatchedClaimsBeforeRepositoryOrManager(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	claims := fixtureSessionClaims("operation_wrong")
	claims.UserID++
	_, err := service.Get(context.Background(), SessionRouteCommand{SessionID: manager.ref.SessionID, Claims: claims})
	if !errors.Is(err, domainsandbox.ErrSessionNotFound) && !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("Get(mismatched identity) = %v", err)
	}
	if manager.execCalls != 0 {
		t.Fatalf("manager calls = %d", manager.execCalls)
	}
}

func TestSessionServiceDoesNotProjectExpiredRuntimeSession(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	service.clock = func() time.Time { return now }
	lookup := service.repository.(*recordingSessionLookup)
	lookup.row.ExpiresAt = now
	_, err := service.Get(context.Background(), SessionRouteCommand{SessionID: manager.ref.SessionID, Claims: fixtureSessionClaims("get_expired")})
	if !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Get(expired) error = %v, want ErrUnavailable", err)
	}
}

func TestSessionServiceOperationEventsResumeTerminalWithoutReplayingAccepted(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	scheduler := service.scheduler.(*memoryCoreScheduler)
	digest := sha256.Sum256([]byte("result"))
	scheduler.records["operation_event"] = SessionOperationRecord{SessionID: manager.ref.SessionID, OperationID: "operation_event", Kind: SessionOperationRead, State: SessionOperationSucceeded, ResultDigest: digest[:]}
	events, err := service.OperationEvents(context.Background(), SessionOperationRouteCommand{SessionID: manager.ref.SessionID, OperationID: "operation_event", Claims: fixtureSessionClaims("operation_event"), AfterEventID: "operation_event_accepted"})
	if err != nil || len(events) != 1 || events[0].State != SessionOperationSucceeded {
		t.Fatalf("OperationEvents(resume) = %#v, %v", events, err)
	}
}

func TestSessionServiceRejectsReplayedOperationWithDifferentPayload(t *testing.T) {
	service, manager := newSessionServiceFixture(t)
	claims := fixtureSessionClaims("operation_replay")
	first := SessionOperationCommand{SessionID: manager.ref.SessionID, OperationID: claims.OperationID, Kind: SessionOperationWrite,
		Deadline: time.Now().UTC().Add(time.Minute), Payload: SessionOperationPayload{Path: "/mnt/user-data/workspace/a", Content: []byte("one")}, Claims: claims}
	if _, err := service.SubmitOperation(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	first.Payload.Content = []byte("two")
	if _, err := service.SubmitOperation(context.Background(), first); !errors.Is(err, ErrSessionOperationConflict) {
		t.Fatalf("SubmitOperation(conflicting replay) = %v", err)
	}
}

// The implementation fixture is intentionally declared through the wished-for
// production constructor. The first run must fail until SessionService closes
// the HTTP/store/dispatcher integration gap.
func newSessionServiceFixture(t *testing.T) (*SessionService, *recordingSessionManager) {
	t.Helper()
	manager := &recordingSessionManager{ref: domainsandbox.SessionRef{
		SessionID: "550e8400-e29b-41d4-a716-446655440000", RuntimeGeneration: 1,
		Key: domainsandbox.SessionKey{DeploymentID: "runner-a", ProviderID: 41, SpaceID: 42, UserID: 43, ThreadID: "thread_01", Profile: domainsandbox.SessionProfileCore},
	}}
	service, err := NewSessionService(SessionServiceConfig{
		DeploymentID: "runner-a", Manager: manager, Repository: &recordingSessionLookup{row: domainsandbox.RuntimeSession{Ref: manager.ref, State: domainsandbox.SessionStateActive, UpstreamShellID: "shell_01", Version: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)}},
		Scheduler: newMemoryCoreScheduler(), Clock: func() time.Time { return time.Now().UTC() }, AllowedEnvironmentNames: []string{"LANG"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, manager
}

func fixtureStableIdentity() SessionStableIdentity {
	return SessionStableIdentity{DeploymentID: "runner-a", ProviderID: 41, SpaceID: 42, UserID: 43, ThreadID: "thread_01", Profile: domainsandbox.SessionProfileCore}
}

func fixtureSessionClaims(operationID string) sandboxidentity.SessionRequest {
	digest := sha256.Sum256([]byte(operationID))
	return sandboxidentity.SessionRequest{DeploymentID: "runner-a", ProviderID: 41, Scope: sandboxidentity.ScopeAgent, SpaceID: 42, UserID: 43, ThreadID: "thread_01", RunID: "run_01", OperationID: operationID, Profile: "core", RequestDigest: digest[:]}
}

type recordingSessionManager struct {
	ref        domainsandbox.SessionRef
	execCalls  int
	writeCalls int
	blockExec  bool
	execStdout []byte
	execErr    error
}

func (m *recordingSessionManager) Acquire(context.Context, infrasandbox.AcquireSessionRequest) (infrasandbox.SandboxSession, error) {
	return m, nil
}
func (m *recordingSessionManager) Get(context.Context, domainsandbox.SessionRef) (infrasandbox.SandboxSession, error) {
	return m, nil
}
func (m *recordingSessionManager) Release(context.Context, domainsandbox.SessionRef) error {
	return nil
}
func (m *recordingSessionManager) Destroy(context.Context, domainsandbox.SessionRef) error {
	return nil
}
func (m *recordingSessionManager) Recover(context.Context, domainsandbox.SessionRef) (infrasandbox.SandboxSession, error) {
	return m, nil
}
func (m *recordingSessionManager) Ref() domainsandbox.SessionRef { return m.ref }
func (m *recordingSessionManager) Exec(context.Context, infrasandbox.ExecRequest) (infrasandbox.ExecutionStream, error) {
	m.execCalls++
	if m.execErr != nil {
		return nil, m.execErr
	}
	return &fixtureExecutionStream{block: m.blockExec, stdout: append([]byte(nil), m.execStdout...)}, nil
}
func (m *recordingSessionManager) Read(context.Context, infrasandbox.ReadRequest) (infrasandbox.FileContent, error) {
	return infrasandbox.FileContent{Data: []byte("ok")}, nil
}
func (m *recordingSessionManager) Write(context.Context, infrasandbox.WriteRequest) error {
	m.writeCalls++
	return nil
}
func (m *recordingSessionManager) List(context.Context, infrasandbox.ListRequest) ([]infrasandbox.FileEntry, error) {
	return []infrasandbox.FileEntry{}, nil
}
func (m *recordingSessionManager) Glob(context.Context, infrasandbox.GlobRequest) ([]infrasandbox.FileEntry, error) {
	return []infrasandbox.FileEntry{}, nil
}
func (m *recordingSessionManager) Grep(context.Context, infrasandbox.GrepRequest) ([]infrasandbox.GrepMatch, error) {
	return []infrasandbox.GrepMatch{}, nil
}
func (m *recordingSessionManager) Replace(context.Context, infrasandbox.ReplaceRequest) error {
	return nil
}
func (m *recordingSessionManager) Download(context.Context, infrasandbox.DownloadRequest) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("ok")), nil
}

type fixtureExecutionStream struct {
	sent, block bool
	stdout      []byte
}

func (s *fixtureExecutionStream) Recv(ctx context.Context) (infrasandbox.ExecutionEvent, error) {
	if s.block {
		<-ctx.Done()
		return infrasandbox.ExecutionEvent{}, ctx.Err()
	}
	if s.sent {
		return infrasandbox.ExecutionEvent{}, io.EOF
	}
	s.sent = true
	if len(s.stdout) > 0 {
		data := append([]byte(nil), s.stdout...)
		s.stdout = nil
		s.sent = false
		return infrasandbox.ExecutionEvent{Kind: infrasandbox.ExecutionEventStdout, Data: data}, nil
	}
	code := 0
	return infrasandbox.ExecutionEvent{Kind: infrasandbox.ExecutionEventTerminal, ExitCode: &code}, nil
}
func (*fixtureExecutionStream) Close() error { return nil }

type recordingSessionLookup struct{ row domainsandbox.RuntimeSession }

func (lookup *recordingSessionLookup) AcquireRuntimeSession(context.Context, domainsandbox.AcquireRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	return lookup.row, nil
}
func (lookup *recordingSessionLookup) GetRuntimeSession(context.Context, domainsandbox.SessionRef) (domainsandbox.RuntimeSession, error) {
	return lookup.row, nil
}
func (lookup *recordingSessionLookup) BindRuntimeSessionCAS(context.Context, domainsandbox.BindRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	return lookup.row, nil
}
func (lookup *recordingSessionLookup) TransitionRuntimeSessionCAS(context.Context, domainsandbox.TransitionRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	return lookup.row, nil
}
func (*recordingSessionLookup) ListRecoverableRuntimeSessions(context.Context, domainsandbox.ListRecoverableRuntimeSessionsInput) ([]domainsandbox.RuntimeSession, error) {
	return nil, nil
}

func (lookup *recordingSessionLookup) GetRuntimeSessionByKey(_ context.Context, sessionID string, key domainsandbox.SessionKey) (domainsandbox.RuntimeSession, error) {
	if lookup.row.Ref.SessionID != sessionID || lookup.row.Ref.Key != key {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrSessionNotFound
	}
	return lookup.row, nil
}

type memoryCoreScheduler struct {
	mu            sync.Mutex
	records       map[string]SessionOperationRecord
	digests       map[string][sha256.Size]byte
	start         bool
	running       chan struct{}
	runOnce       sync.Once
	getErr        error
	tryStartCalls int
}

func newMemoryCoreScheduler() *memoryCoreScheduler {
	return &memoryCoreScheduler{records: map[string]SessionOperationRecord{}, digests: map[string][sha256.Size]byte{}, start: true}
}
func (scheduler *memoryCoreScheduler) Accept(_ context.Context, input SessionOperationInput) (SessionOperationRecord, error) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	var digest [sha256.Size]byte
	copy(digest[:], input.RequestDigest)
	if existing, ok := scheduler.records[input.OperationID]; ok {
		if scheduler.digests[input.OperationID] != digest || existing.Kind != input.Kind {
			return SessionOperationRecord{}, ErrSessionOperationConflict
		}
		return existing, nil
	}
	record := SessionOperationRecord{SessionID: input.SessionID, OperationID: input.OperationID, Kind: input.Kind, State: SessionOperationQueued}
	scheduler.records[input.OperationID] = record
	scheduler.digests[input.OperationID] = digest
	return record, nil
}
func (scheduler *memoryCoreScheduler) TryStart(_ context.Context, _, operationID string) (SessionOperationRecord, bool, error) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	scheduler.tryStartCalls++
	record := scheduler.records[operationID]
	if !scheduler.start {
		return record, false, nil
	}
	record.State = SessionOperationRunning
	scheduler.records[operationID] = record
	if scheduler.running != nil {
		scheduler.runOnce.Do(func() { close(scheduler.running) })
	}
	return record, true, nil
}
func (scheduler *memoryCoreScheduler) Finish(_ context.Context, _, operationID string, completion SessionOperationCompletion) (SessionOperationRecord, error) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	record := scheduler.records[operationID]
	if record.CancelRequested && completion.State == SessionOperationSucceeded {
		completion.State, completion.ReasonCode, completion.ResultDigest = SessionOperationCanceled, "SANDBOX_OPERATION_CANCELED", nil
	}
	record.State, record.ResultDigest, record.Result, record.ReasonCode = completion.State, append([]byte(nil), completion.ResultDigest...), append([]byte(nil), completion.Result...), completion.ReasonCode
	scheduler.records[operationID] = record
	return record, nil
}
func (scheduler *memoryCoreScheduler) CancelQueued(_ context.Context, _, operationID string) (SessionOperationRecord, bool, error) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	record := scheduler.records[operationID]
	record.State = SessionOperationCanceled
	scheduler.records[operationID] = record
	return record, true, nil
}
func (scheduler *memoryCoreScheduler) Get(_ context.Context, _, operationID string) (SessionOperationRecord, error) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.getErr != nil {
		return SessionOperationRecord{}, scheduler.getErr
	}
	record, ok := scheduler.records[operationID]
	if !ok {
		return SessionOperationRecord{}, ErrUnavailable
	}
	return record, nil
}
func (scheduler *memoryCoreScheduler) RequestCancel(_ context.Context, sessionID, operationID string) (SessionOperationRecord, bool, error) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	record, ok := scheduler.records[operationID]
	if !ok || record.SessionID != sessionID {
		return SessionOperationRecord{}, false, ErrUnavailable
	}
	switch record.State {
	case SessionOperationAccepted, SessionOperationQueued:
		record.State = SessionOperationCanceled
		record.ReasonCode = "SANDBOX_OPERATION_CANCELED"
		scheduler.records[operationID] = record
		return record, true, nil
	case SessionOperationRunning:
		record.CancelRequested = true
		scheduler.records[operationID] = record
		return record, false, nil
	default:
		return record, true, nil
	}
}

func (scheduler *memoryCoreScheduler) failObservation(err error) {
	scheduler.mu.Lock()
	scheduler.getErr = err
	scheduler.mu.Unlock()
}

func (scheduler *memoryCoreScheduler) setStart(start bool) {
	scheduler.mu.Lock()
	scheduler.start = start
	scheduler.mu.Unlock()
}

func (scheduler *memoryCoreScheduler) putRunning(sessionID, operationID string, kind SessionOperationKind, digest [sha256.Size]byte) {
	scheduler.mu.Lock()
	scheduler.records[operationID] = SessionOperationRecord{SessionID: sessionID, OperationID: operationID, Kind: kind, State: SessionOperationRunning}
	scheduler.digests[operationID] = digest
	scheduler.mu.Unlock()
}

func (scheduler *memoryCoreScheduler) tryStarts() int {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return scheduler.tryStartCalls
}

type blockingCoreScheduler struct{ memoryCoreScheduler }

func (scheduler *blockingCoreScheduler) Accept(ctx context.Context, input SessionOperationInput) (SessionOperationRecord, error) {
	if scheduler.records == nil {
		scheduler.records = map[string]SessionOperationRecord{}
	}
	if scheduler.digests == nil {
		scheduler.digests = map[string][sha256.Size]byte{}
	}
	return scheduler.memoryCoreScheduler.Accept(ctx, input)
}

func (scheduler *blockingCoreScheduler) TryStart(context.Context, string, string) (SessionOperationRecord, bool, error) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	for _, record := range scheduler.records {
		return record, false, nil
	}
	return SessionOperationRecord{}, false, ErrUnavailable
}
