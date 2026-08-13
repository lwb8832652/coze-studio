// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestSessionDispatcherAcquireBindsOneOpaqueShellInFencedOrder(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.unboundRow()
	fixture.repository.acquireResult = row
	fixture.repository.getResult = row
	fixture.repository.bindResult = boundSessionRow(row, "shell-candidate")

	session, err := fixture.dispatcher.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{Key: fixture.key})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if got := session.Ref(); got != fixture.repository.bindResult.Ref {
		t.Fatalf("Acquire() ref = %#v, want %#v", got, fixture.repository.bindResult.Ref)
	}
	wantOrder := []string{"lease", "generation", "acquire-row", "get-row", "owned", "adapter:shell-candidate", "prepare", "create", "owned", "bind", "release-lease"}
	if got := fixture.events.snapshot(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("Acquire() order = %#v, want %#v", got, wantOrder)
	}
	if fixture.repository.bindInput.UpstreamShellID != "shell-candidate" ||
		fixture.factory.controlShellID != fixture.generation.SentinelID {
		t.Fatalf("candidate/control shell = %q/%q", fixture.repository.bindInput.UpstreamShellID, fixture.factory.controlShellID)
	}
}

func TestSessionDispatcherAcquireCASLoserCleansOnlyCandidateAndAdoptsWinner(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.unboundRow()
	winner := boundSessionRow(row, "shell-winner")
	fixture.repository.acquireResult = row
	fixture.repository.getResults = []domainsandbox.RuntimeSession{row, winner}
	fixture.repository.bindErr = domainsandbox.ErrVersionConflict

	session, err := fixture.dispatcher.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{Key: fixture.key})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if session.Ref() != winner.Ref {
		t.Fatalf("Acquire() ref = %#v, want winner %#v", session.Ref(), winner.Ref)
	}
	if got := fixture.factory.cleanupShells; !reflect.DeepEqual(got, []string{"shell-candidate"}) {
		t.Fatalf("cleanup shells = %#v, want candidate only", got)
	}
	if got := fixture.factory.createdShells; !reflect.DeepEqual(got, []string{"shell-candidate"}) {
		t.Fatalf("created shells = %#v", got)
	}
}

func TestSessionDispatcherAcquireAdoptsAlreadyBoundIdempotentRow(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-existing")
	fixture.repository.acquireResult = row
	fixture.repository.getResult = row

	session, err := fixture.dispatcher.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{Key: fixture.key})
	if err != nil || session.Ref() != row.Ref {
		t.Fatalf("Acquire() = %#v, %v", session, err)
	}
	if fixture.factory.calls != 0 || fixture.repository.bindInput.UpstreamShellID != "" {
		t.Fatalf("idempotent acquire called upstream/bind = %d/%#v", fixture.factory.calls, fixture.repository.bindInput)
	}
}

func TestSessionDispatcherAcquireRejectsAndCleansExpiredBoundRow(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-expired")
	row.ExpiresAt = fixture.now
	fixture.repository.acquireResult = row
	fixture.repository.getResult = row
	fenced := row
	fenced.State = domainsandbox.SessionStateRecovering
	fenced.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fenced.Version++
	released := fenced
	released.State = domainsandbox.SessionStateReleased
	released.UpstreamShellID = ""
	released.RecoveryReason = ""
	released.Version++
	fixture.repository.transitionResults = []domainsandbox.RuntimeSession{fenced, released}

	if _, err := fixture.dispatcher.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{Key: fixture.key}); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Acquire(expired) error = %v, want ErrUnavailable", err)
	}
	if got := fixture.factory.cleanupShells; !reflect.DeepEqual(got, []string{"shell-expired"}) {
		t.Fatalf("expired acquire cleanup shells = %#v", got)
	}
}

func TestSessionDispatcherAcquireRejectsRepositoryIdentityMismatchBeforeUpstream(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.unboundRow()
	fixture.repository.acquireResult = row
	row.Ref.Key.UserID++
	fixture.repository.getResult = row

	if _, err := fixture.dispatcher.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{Key: fixture.key}); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Acquire() error = %v, want ErrUnavailable", err)
	}
	if fixture.factory.calls != 0 {
		t.Fatalf("adapter factory calls = %d, want 0", fixture.factory.calls)
	}
}

func TestSessionDispatcherOldGenerationMarksRecoveringWithoutCallingUpstream(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-old")
	row.Ref.RuntimeGeneration--
	fixture.repository.getResult = row
	fixture.repository.transitionResult = recoveringSessionRow(row)

	if _, err := fixture.dispatcher.Get(context.Background(), row.Ref); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Get() error = %v, want ErrUnavailable", err)
	}
	if fixture.repository.transitionInput.Action != domainsandbox.SessionActionMarkRecovering || fixture.factory.calls != 0 {
		t.Fatalf("transition/factory = %#v/%d", fixture.repository.transitionInput, fixture.factory.calls)
	}
}

func TestSessionDispatcherFileCompletionRequiresLeaseOwnership(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-bound")
	fixture.repository.getResult = row
	leaseLost := errors.New("lease lost")
	session, err := fixture.dispatcher.Get(context.Background(), row.Ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	fixture.lease.ownedErr = leaseLost
	fixture.lease.ownedErrs = []error{nil, nil, leaseLost}
	fixture.lease.ownedCalls = 0
	releasesBefore := fixture.lease.releaseCalls
	if err := session.Write(context.Background(), infrasandbox.WriteRequest{Path: "/mnt/user-data/workspace/a.txt", Content: []byte("body")}); !errors.Is(err, leaseLost) {
		t.Fatalf("Write() error = %v, want lease lost", err)
	}
	if fixture.factory.writeCalls != 1 || fixture.lease.releaseCalls != releasesBefore+1 {
		t.Fatalf("write/release calls = %d/%d (before %d)", fixture.factory.writeCalls, fixture.lease.releaseCalls, releasesBefore)
	}
	if fixture.repository.transitionInput.Action != domainsandbox.SessionActionBeginOperation ||
		fixture.repository.transitionInput.UpstreamShellID != row.UpstreamShellID {
		t.Fatalf("persisted fence = %#v, want begin operation retaining shell", fixture.repository.transitionInput)
	}
}

func TestSessionDispatcherSuccessfulOperationPersistsFenceThenTouchesIdleTTL(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-bound")
	fixture.repository.getResult = row
	fenced := row
	fenced.State = domainsandbox.SessionStateRecovering
	fenced.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fenced.Version++
	completed := fenced
	completed.State = domainsandbox.SessionStateActive
	completed.RecoveryReason = ""
	completed.Version++
	completed.LastActivityAt = fixture.now
	completed.ExpiresAt = fixture.now.Add(time.Hour)
	fixture.repository.transitionResults = []domainsandbox.RuntimeSession{fenced, completed}
	session, err := fixture.dispatcher.Get(context.Background(), row.Ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	fixture.events.reset()
	if err := session.Write(context.Background(), infrasandbox.WriteRequest{Path: "/mnt/user-data/workspace/a.txt", Content: []byte("body")}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	wantOrder := []string{"lease", "generation", "get-row", "owned", "transition", "owned", "adapter:shell-bound", "write", "owned", "transition", "release-lease"}
	if got := fixture.events.snapshot(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("Write() order = %#v, want %#v", got, wantOrder)
	}
	if len(fixture.repository.transitionInputs) != 2 ||
		fixture.repository.transitionInputs[0].Action != domainsandbox.SessionActionBeginOperation ||
		fixture.repository.transitionInputs[1].Action != domainsandbox.SessionActionCompleteOperation ||
		fixture.repository.transitionInputs[1].ExpiresAt != fixture.now.Add(time.Hour) {
		t.Fatalf("operation transitions = %#v", fixture.repository.transitionInputs)
	}
}

func TestSessionDispatcherSuccessfulTouchUsesDatabaseMillisecondPrecision(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	now := fixture.now.Add(123456789 * time.Nanosecond)
	fixture.dispatcher.clock = func() time.Time { return now }
	row := fixture.boundRow("shell-bound")
	fixture.repository.getResult = row
	fenced := row
	fenced.State = domainsandbox.SessionStateRecovering
	fenced.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fenced.Version++
	persistedNow := now.Truncate(time.Millisecond)
	completed := fenced
	completed.State = domainsandbox.SessionStateActive
	completed.RecoveryReason = ""
	completed.Version++
	completed.LastActivityAt = persistedNow
	completed.ExpiresAt = persistedNow.Add(time.Hour)
	fixture.repository.transitionResults = []domainsandbox.RuntimeSession{fenced, completed}
	session := &dispatcherSession{dispatcher: fixture.dispatcher, ref: row.Ref, controlShellID: fixture.generation.SentinelID}

	if err := session.Write(context.Background(), infrasandbox.WriteRequest{Path: "/mnt/user-data/workspace/a.txt", Content: []byte("body")}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := fixture.repository.transitionInputs[1].Now; got != persistedNow {
		t.Fatalf("touch time = %s, want DATETIME(3) precision %s", got, persistedNow)
	}
}

func TestSessionDispatcherExpiredSessionCleansBeforeClearingPersistentBinding(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-expired")
	row.ExpiresAt = fixture.now
	fixture.repository.getResult = row
	fenced := row
	fenced.State = domainsandbox.SessionStateRecovering
	fenced.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fenced.Version++
	released := fenced
	released.State = domainsandbox.SessionStateReleased
	released.UpstreamShellID = ""
	released.RecoveryReason = ""
	released.Version++
	fixture.repository.transitionResults = []domainsandbox.RuntimeSession{fenced, released}

	if _, err := fixture.dispatcher.Get(context.Background(), row.Ref); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Get(expired) error = %v, want ErrUnavailable", err)
	}
	wantOrder := []string{"lease", "generation", "get-row", "owned", "transition", "cleanup:shell-expired", "owned", "transition", "release-lease"}
	if got := fixture.events.snapshot(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("expired cleanup order = %#v, want %#v", got, wantOrder)
	}
}

func TestSessionDispatcherExecCancelTargetsOnlyRowsOpaqueShell(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-row")
	fixture.repository.getResult = row
	fixture.factory.blockExec = true
	session, err := fixture.dispatcher.Get(context.Background(), row.Ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = session.Exec(ctx, infrasandbox.ExecRequest{
		OperationID: "operation-a", Command: "sleep 30", CWD: "/mnt/user-data/workspace",
		Deadline: fixture.now.Add(time.Minute), MaxOutputBytes: 1024,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Exec() error = %v, want context canceled", err)
	}
	if fixture.factory.execShellID != "shell-row" || fixture.factory.cleanupCalls != 0 {
		t.Fatalf("exec shell/cleanup = %q/%d", fixture.factory.execShellID, fixture.factory.cleanupCalls)
	}
}

func TestSessionDispatcherFileCancelNeverCleansOrKillsShell(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-row")
	fixture.repository.getResult = row
	fixture.factory.blockRead = true
	session, err := fixture.dispatcher.Get(context.Background(), row.Ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = session.Read(ctx, infrasandbox.ReadRequest{Path: "/mnt/user-data/workspace/a.txt", MaxBytes: 1024})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() error = %v, want context canceled", err)
	}
	if fixture.factory.cleanupCalls != 0 || fixture.factory.execCalls != 0 {
		t.Fatalf("cleanup/exec calls = %d/%d", fixture.factory.cleanupCalls, fixture.factory.execCalls)
	}
}

func TestSessionDispatcherReleaseCleansShellBeforeClearingBinding(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-row")
	fixture.repository.getResult = row
	released := row
	released.State = domainsandbox.SessionStateReleased
	released.UpstreamShellID = ""
	released.Version++
	fenced := row
	fenced.State = domainsandbox.SessionStateRecovering
	fenced.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fenced.Version++
	fixture.repository.transitionResults = []domainsandbox.RuntimeSession{fenced, released}

	if err := fixture.dispatcher.Release(context.Background(), row.Ref); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	want := []string{"lease", "generation", "get-row", "owned", "transition", "owned", "cleanup:shell-row", "owned", "transition", "release-lease"}
	if got := fixture.events.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Release() order = %#v, want %#v", got, want)
	}
}

func TestSessionDispatcherReleaseCleanupFailureKeepsOpaqueShellForSafeRetry(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-row")
	fixture.repository.getResult = row
	fenced := row
	fenced.State = domainsandbox.SessionStateRecovering
	fenced.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fenced.Version++
	fixture.repository.transitionResult = fenced
	cleanupErr := errors.New("cleanup unavailable")
	fixture.factory.cleanupErrs = []error{cleanupErr, nil}

	if err := fixture.dispatcher.Release(context.Background(), row.Ref); !errors.Is(err, cleanupErr) {
		t.Fatalf("Release(first) error = %v, want cleanup error", err)
	}
	if len(fixture.repository.transitionInputs) != 1 || fixture.repository.transitionInputs[0].Action != domainsandbox.SessionActionBeginOperation {
		t.Fatalf("first release transitions = %#v, want only persistent fence", fixture.repository.transitionInputs)
	}

	fixture.repository.getResult = fenced
	released := fenced
	released.State = domainsandbox.SessionStateReleased
	released.UpstreamShellID = ""
	released.RecoveryReason = ""
	released.Version++
	fixture.repository.transitionResult = released
	if err := fixture.dispatcher.Release(context.Background(), row.Ref); err != nil {
		t.Fatalf("Release(retry) error = %v", err)
	}
	if got := fixture.factory.cleanupShells; !reflect.DeepEqual(got, []string{"shell-row", "shell-row"}) {
		t.Fatalf("cleanup retry shells = %#v", got)
	}
}

func TestSessionDispatcherSecondOwnerCannotDispatchPersistedOperationFence(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-busy")
	row.State = domainsandbox.SessionStateRecovering
	row.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fixture.repository.getResult = row
	session := &dispatcherSession{dispatcher: fixture.dispatcher, ref: row.Ref, controlShellID: fixture.generation.SentinelID}

	if err := session.Write(context.Background(), infrasandbox.WriteRequest{Path: "/mnt/user-data/workspace/a.txt", Content: []byte("body")}); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("second owner Write() error = %v, want ErrUnavailable", err)
	}
	if fixture.factory.calls != 0 || fixture.factory.writeCalls != 0 {
		t.Fatalf("second owner reached adapter = factory %d/write %d", fixture.factory.calls, fixture.factory.writeCalls)
	}
}

func TestSessionDispatcherRecoverCleansPersistedFencedShellBeforeCreatingReplacement(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-fenced")
	row.State = domainsandbox.SessionStateRecovering
	row.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fixture.repository.getResult = row
	recovered := row
	recovered.State = domainsandbox.SessionStateActive
	recovered.UpstreamShellID = "shell-candidate"
	recovered.RecoveryReason = ""
	recovered.Version++
	fixture.repository.transitionResult = recovered

	if _, err := fixture.dispatcher.Recover(context.Background(), row.Ref); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	want := []string{"lease", "generation", "get-row", "owned", "cleanup:shell-fenced", "owned", "owned", "adapter:shell-candidate", "prepare", "create", "owned", "transition", "release-lease"}
	if got := fixture.events.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Recover() order = %#v, want %#v", got, want)
	}
}

func TestSessionDispatcherRecoverRejectsReplacementShellIDCollision(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-candidate")
	row.State = domainsandbox.SessionStateRecovering
	row.RecoveryReason = domainsandbox.SessionOperationFenceReason
	fixture.repository.getResult = row

	if _, err := fixture.dispatcher.Recover(context.Background(), row.Ref); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Recover() error = %v, want ErrUnavailable", err)
	}
	if fixture.factory.calls != 0 || fixture.repository.transitionInput.Action != "" {
		t.Fatalf("colliding replacement reached adapter/transition = %d/%q", fixture.factory.calls, fixture.repository.transitionInput.Action)
	}
}

func TestSessionDispatcherReleaseCASFailureDoesNotCleanActiveShell(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-row")
	fixture.repository.getResult = row
	fixture.repository.transitionErr = domainsandbox.ErrVersionConflict

	if err := fixture.dispatcher.Release(context.Background(), row.Ref); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("Release() error = %v, want version conflict", err)
	}
	if fixture.factory.cleanupCalls != 0 {
		t.Fatalf("cleanup calls = %d, want 0", fixture.factory.cleanupCalls)
	}
}

func TestSessionDispatcherLeaseReleaseFailureFencesSuccessfulFileResult(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-row")
	fixture.repository.getResult = row
	session, err := fixture.dispatcher.Get(context.Background(), row.Ref)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	releaseLost := errors.New("compare owner release failed")
	fixture.lease.releaseErr = releaseLost
	if _, err := session.Read(context.Background(), infrasandbox.ReadRequest{Path: "/mnt/user-data/workspace/a.txt", MaxBytes: 1024}); !errors.Is(err, releaseLost) {
		t.Fatalf("Read() error = %v, want release fence", err)
	}
}

func TestSessionDispatcherAcquireLeaseReleaseFailureReturnsNoSession(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-existing")
	fixture.repository.acquireResult = row
	fixture.repository.getResult = row
	leaseLost := errors.New("lease release lost")
	fixture.lease.releaseErr = leaseLost

	session, err := fixture.dispatcher.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{Key: fixture.key})
	if session != nil || !errors.Is(err, leaseLost) {
		t.Fatalf("Acquire() = %#v, %v, want nil/release lost", session, err)
	}
}

func TestSessionDispatcherUncertainCreateMarksRowRecoveringWithoutGuessingCleanup(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.unboundRow()
	fixture.repository.acquireResult = row
	fixture.repository.getResult = row
	fixture.factory.createErr = errors.New("transport outcome unknown")
	fixture.repository.transitionResult = recoveringSessionRow(row)

	if _, err := fixture.dispatcher.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{Key: fixture.key}); err == nil {
		t.Fatal("Acquire() error = nil")
	}
	if fixture.repository.transitionInput.Action != domainsandbox.SessionActionMarkRecovering || fixture.factory.cleanupCalls != 0 {
		t.Fatalf("transition/cleanup = %#v/%d", fixture.repository.transitionInput, fixture.factory.cleanupCalls)
	}
}

func TestSessionDispatcherOldGenerationReleaseDoesNotTargetCurrentAIOShell(t *testing.T) {
	fixture := newSessionDispatcherFixture(t)
	row := fixture.boundRow("shell-old-generation")
	row.Ref.RuntimeGeneration--
	fixture.repository.getResult = row
	released := row
	released.State = domainsandbox.SessionStateReleased
	released.UpstreamShellID = ""
	released.Version++
	fixture.repository.transitionResult = released

	if err := fixture.dispatcher.Release(context.Background(), row.Ref); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if fixture.factory.cleanupCalls != 0 {
		t.Fatalf("old generation cleanup calls = %d, want 0", fixture.factory.cleanupCalls)
	}
}

type sessionDispatcherFixture struct {
	t          *testing.T
	now        time.Time
	key        domainsandbox.SessionKey
	generation domainsandbox.AIOGenerationState
	events     *dispatcherEventLog
	repository *dispatcherRepository
	lease      *dispatcherLeaseHandle
	factory    *dispatcherAdapterFactory
	dispatcher *SessionDispatcher
}

func newSessionDispatcherFixture(t *testing.T) *sessionDispatcherFixture {
	t.Helper()
	events := &dispatcherEventLog{}
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	key := domainsandbox.SessionKey{DeploymentID: "runner-dev-a", ProviderID: 7, SpaceID: 42, UserID: 43, ThreadID: "thread-a", Profile: domainsandbox.SessionProfileCore}
	generation := domainsandbox.AIOGenerationState{DeploymentID: key.DeploymentID, Generation: 2, SentinelID: "newx-generation-0123456789abcdef0123456789abcdef"}
	repository := &dispatcherRepository{events: events}
	lease := &dispatcherLeaseHandle{events: events, context: context.Background()}
	factory := &dispatcherAdapterFactory{events: events}
	dispatcher, err := NewSessionDispatcher(SessionDispatcherConfig{
		Repository:     repository,
		Leases:         &dispatcherLeaseManager{events: events, handle: lease},
		Generation:     dispatcherGenerationSource{events: events, state: generation},
		AdapterFactory: factory,
		IDs:            dispatcherIDSource{sessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942", shellID: "shell-candidate"},
		Clock:          func() time.Time { return now }, SessionTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewSessionDispatcher() error = %v", err)
	}
	return &sessionDispatcherFixture{t: t, now: now, key: key, generation: generation, events: events, repository: repository, lease: lease, factory: factory, dispatcher: dispatcher}
}

func (f *sessionDispatcherFixture) unboundRow() domainsandbox.RuntimeSession {
	return domainsandbox.RuntimeSession{
		Ref:   domainsandbox.SessionRef{SessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942", Key: f.key, RuntimeGeneration: f.generation.Generation},
		State: domainsandbox.SessionStateActive, Version: 1, ExpiresAt: f.now.Add(time.Hour), CreatedAt: f.now, UpdatedAt: f.now,
	}
}

func (f *sessionDispatcherFixture) boundRow(shellID string) domainsandbox.RuntimeSession {
	return boundSessionRow(f.unboundRow(), shellID)
}

func boundSessionRow(row domainsandbox.RuntimeSession, shellID string) domainsandbox.RuntimeSession {
	row.UpstreamShellID = shellID
	row.Version++
	return row
}

func recoveringSessionRow(row domainsandbox.RuntimeSession) domainsandbox.RuntimeSession {
	row.State = domainsandbox.SessionStateRecovering
	row.UpstreamShellID = ""
	row.RecoveryReason = "aio_runtime_restarted"
	row.Version++
	return row
}

type dispatcherEventLog struct {
	mu sync.Mutex
	v  []string
}

func (l *dispatcherEventLog) add(value string) { l.mu.Lock(); l.v = append(l.v, value); l.mu.Unlock() }
func (l *dispatcherEventLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.v...)
}
func (l *dispatcherEventLog) reset() { l.mu.Lock(); l.v = nil; l.mu.Unlock() }

type dispatcherGenerationSource struct {
	events *dispatcherEventLog
	state  domainsandbox.AIOGenerationState
	err    error
}

func (s dispatcherGenerationSource) GetAIOGeneration(context.Context, string) (domainsandbox.AIOGenerationState, error) {
	s.events.add("generation")
	return s.state, s.err
}

type dispatcherIDSource struct{ sessionID, shellID string }

func (s dispatcherIDSource) NewSessionID() (string, error) { return s.sessionID, nil }
func (s dispatcherIDSource) NewShellID() (string, error)   { return s.shellID, nil }

type dispatcherLeaseManager struct {
	events *dispatcherEventLog
	handle *dispatcherLeaseHandle
}

func (m *dispatcherLeaseManager) Acquire(context.Context, string, string) (SessionLeaseHandle, error) {
	m.events.add("lease")
	return m.handle, nil
}

type dispatcherLeaseHandle struct {
	events       *dispatcherEventLog
	context      context.Context
	ownedErr     error
	ownedErrs    []error
	ownedCalls   int
	releaseErr   error
	releaseCalls int
}

func (h *dispatcherLeaseHandle) Context() context.Context    { return h.context }
func (h *dispatcherLeaseHandle) Renew(context.Context) error { return nil }
func (h *dispatcherLeaseHandle) Owned(context.Context) error {
	h.events.add("owned")
	if h.ownedCalls < len(h.ownedErrs) {
		err := h.ownedErrs[h.ownedCalls]
		h.ownedCalls++
		return err
	}
	h.ownedCalls++
	return h.ownedErr
}
func (h *dispatcherLeaseHandle) Release(context.Context) error {
	h.events.add("release-lease")
	h.releaseCalls++
	return h.releaseErr
}

type dispatcherRepository struct {
	events            *dispatcherEventLog
	acquireResult     domainsandbox.RuntimeSession
	acquireErr        error
	getResult         domainsandbox.RuntimeSession
	getResults        []domainsandbox.RuntimeSession
	getCalls          int
	getErr            error
	bindInput         domainsandbox.BindRuntimeSessionInput
	bindResult        domainsandbox.RuntimeSession
	bindErr           error
	transitionInput   domainsandbox.TransitionRuntimeSessionInput
	transitionInputs  []domainsandbox.TransitionRuntimeSessionInput
	transitionResult  domainsandbox.RuntimeSession
	transitionResults []domainsandbox.RuntimeSession
	transitionCalls   int
	transitionErr     error
}

func (r *dispatcherRepository) AcquireRuntimeSession(_ context.Context, _ domainsandbox.AcquireRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	r.events.add("acquire-row")
	return r.acquireResult, r.acquireErr
}
func (r *dispatcherRepository) GetRuntimeSession(_ context.Context, _ domainsandbox.SessionRef) (domainsandbox.RuntimeSession, error) {
	r.events.add("get-row")
	if r.getCalls < len(r.getResults) {
		result := r.getResults[r.getCalls]
		r.getCalls++
		return result, r.getErr
	}
	r.getCalls++
	return r.getResult, r.getErr
}
func (r *dispatcherRepository) GetRuntimeSessionByKey(_ context.Context, sessionID string, key domainsandbox.SessionKey) (domainsandbox.RuntimeSession, error) {
	result := r.getResult
	if result.Ref.SessionID != sessionID || result.Ref.Key != key {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrSessionNotFound
	}
	return result, r.getErr
}
func (r *dispatcherRepository) BindRuntimeSessionCAS(_ context.Context, input domainsandbox.BindRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	r.events.add("bind")
	r.bindInput = input
	return r.bindResult, r.bindErr
}
func (r *dispatcherRepository) TransitionRuntimeSessionCAS(_ context.Context, input domainsandbox.TransitionRuntimeSessionInput) (domainsandbox.RuntimeSession, error) {
	r.events.add("transition")
	r.transitionInput = input
	r.transitionInputs = append(r.transitionInputs, input)
	if r.transitionCalls < len(r.transitionResults) {
		result := r.transitionResults[r.transitionCalls]
		r.transitionCalls++
		return result, r.transitionErr
	}
	r.transitionCalls++
	if r.transitionResult.Ref.SessionID == "" && r.transitionErr == nil {
		result := r.getResult
		result.Version = input.ExpectedVersion + 1
		result.LastActivityAt = input.Now
		result.UpdatedAt = input.Now
		switch input.Action {
		case domainsandbox.SessionActionBeginOperation:
			result.State = domainsandbox.SessionStateRecovering
			result.UpstreamShellID = input.UpstreamShellID
			result.RecoveryReason = input.RecoveryReason
		case domainsandbox.SessionActionCompleteOperation:
			result.State = domainsandbox.SessionStateActive
			result.UpstreamShellID = input.UpstreamShellID
			result.RecoveryReason = ""
			result.ExpiresAt = input.ExpiresAt
		}
		return result, nil
	}
	return r.transitionResult, r.transitionErr
}
func (*dispatcherRepository) ListRecoverableRuntimeSessions(context.Context, domainsandbox.ListRecoverableRuntimeSessionsInput) ([]domainsandbox.RuntimeSession, error) {
	return nil, nil
}

type dispatcherAdapterFactory struct {
	events         *dispatcherEventLog
	calls          int
	lastShellID    string
	controlShellID string
	createdShells  []string
	cleanupShells  []string
	cleanupCalls   int
	cleanupErrs    []error
	execCalls      int
	execShellID    string
	writeCalls     int
	blockExec      bool
	blockRead      bool
	createErr      error
}

func (f *dispatcherAdapterFactory) NewSessionAdapter(ref domainsandbox.SessionRef, shellID, controlShellID string) (SessionRuntimeAdapter, error) {
	f.calls++
	f.lastShellID, f.controlShellID = shellID, controlShellID
	f.events.add("adapter:" + shellID)
	return &dispatcherRuntimeAdapter{factory: f, ref: ref, shellID: shellID}, nil
}
func (f *dispatcherAdapterFactory) CleanupShell(_ context.Context, shellID string) error {
	f.events.add("cleanup:" + shellID)
	f.cleanupCalls++
	f.cleanupShells = append(f.cleanupShells, shellID)
	if index := f.cleanupCalls - 1; index < len(f.cleanupErrs) {
		return f.cleanupErrs[index]
	}
	return nil
}

type dispatcherRuntimeAdapter struct {
	factory *dispatcherAdapterFactory
	ref     domainsandbox.SessionRef
	shellID string
}

func (a *dispatcherRuntimeAdapter) Ref() domainsandbox.SessionRef { return a.ref }
func (a *dispatcherRuntimeAdapter) Prepare(context.Context) error {
	a.factory.events.add("prepare")
	return nil
}
func (a *dispatcherRuntimeAdapter) Create(context.Context) error {
	a.factory.events.add("create")
	a.factory.createdShells = append(a.factory.createdShells, a.shellID)
	return a.factory.createErr
}
func (a *dispatcherRuntimeAdapter) Exec(ctx context.Context, _ infrasandbox.ExecRequest) (infrasandbox.ExecutionStream, error) {
	a.factory.execCalls++
	a.factory.execShellID = a.shellID
	if a.factory.blockExec {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	exit := 0
	return &dispatcherExecutionStream{events: []infrasandbox.ExecutionEvent{{Kind: infrasandbox.ExecutionEventTerminal, ExitCode: &exit}}}, nil
}
func (a *dispatcherRuntimeAdapter) Read(ctx context.Context, _ infrasandbox.ReadRequest) (infrasandbox.FileContent, error) {
	if a.factory.blockRead {
		<-ctx.Done()
		return infrasandbox.FileContent{}, ctx.Err()
	}
	return infrasandbox.FileContent{Data: []byte("ok")}, nil
}
func (a *dispatcherRuntimeAdapter) Write(context.Context, infrasandbox.WriteRequest) error {
	a.factory.events.add("write")
	a.factory.writeCalls++
	return nil
}
func (*dispatcherRuntimeAdapter) List(context.Context, infrasandbox.ListRequest) ([]infrasandbox.FileEntry, error) {
	return nil, nil
}
func (*dispatcherRuntimeAdapter) Glob(context.Context, infrasandbox.GlobRequest) ([]infrasandbox.FileEntry, error) {
	return nil, nil
}
func (*dispatcherRuntimeAdapter) Grep(context.Context, infrasandbox.GrepRequest) ([]infrasandbox.GrepMatch, error) {
	return nil, nil
}
func (*dispatcherRuntimeAdapter) Replace(context.Context, infrasandbox.ReplaceRequest) error {
	return nil
}
func (*dispatcherRuntimeAdapter) Download(context.Context, infrasandbox.DownloadRequest) (io.ReadCloser, error) {
	return io.NopCloser(&emptyReader{}), nil
}

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

type dispatcherExecutionStream struct {
	events []infrasandbox.ExecutionEvent
	index  int
}

func (s *dispatcherExecutionStream) Recv(context.Context) (infrasandbox.ExecutionEvent, error) {
	if s.index >= len(s.events) {
		return infrasandbox.ExecutionEvent{}, io.EOF
	}
	e := s.events[s.index]
	s.index++
	return e, nil
}
func (*dispatcherExecutionStream) Close() error { return nil }
