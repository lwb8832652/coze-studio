// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	sandboxcontract "github.com/coze-dev/coze-studio/backend/pkg/sandboxcontract"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type providerExecutionControlledDBClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *providerExecutionControlledDBClock) set(now time.Time) {
	c.mu.Lock()
	c.now = now.UTC()
	c.mu.Unlock()
}

func (c *providerExecutionControlledDBClock) current() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *providerExecutionControlledDBClock) nowExpression() clause.Expr {
	return gorm.Expr("?", c.current())
}

func (c *providerExecutionControlledDBClock) addExpression(duration time.Duration) clause.Expr {
	return gorm.Expr("?", c.current().Add(duration))
}

func (c *providerExecutionControlledDBClock) read(context.Context, *gorm.DB) (time.Time, error) {
	return c.current(), nil
}

func TestProviderExecutionQualityDBClockControlsClaimAndRecovery(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	clock := &providerExecutionControlledDBClock{now: time.Date(2026, 7, 16, 21, 0, 0, 0, time.UTC)}
	repository.clock = clock
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("clock-id", "clock-idem", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	firstHash := testOwnerHash("clock-owner")
	claimed, err := repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: firstHash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	clock.set(clock.current().Add(30 * time.Second))
	if _, err := repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: claimed.Version, OwnerHash: testOwnerHash("fast-app-node"), LeaseDuration: time.Minute,
	}); !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		t.Fatalf("claim before DB expiry = %v", err)
	}

	clock.set(clock.current().Add(31 * time.Second))
	taken, err := repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: claimed.Version, OwnerHash: testOwnerHash("slow-app-node"), LeaseDuration: time.Minute,
	})
	if err != nil || taken.OwnerEpoch != claimed.OwnerEpoch+1 {
		t.Fatalf("claim after DB expiry = %#v, %v", taken, err)
	}
}

func TestProviderExecutionQualityCheckpointOperationIsIdempotent(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	clock := &providerExecutionControlledDBClock{now: time.Date(2026, 7, 16, 22, 0, 0, 0, time.UTC)}
	repository.clock = clock
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("op-id", "op-idem", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	ownerHash := testOwnerHash("operation-owner")
	record, err = repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: ownerHash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	operationHash, err := domainappdev.HashProviderExecutionOperationID("checkpoint-operation-1")
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(record, ownerHash, time.Time{})
	input := domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: operationHash, ReservationDuration: 20 * time.Second,
	}
	reserved, err := repository.ReserveCheckpointWrite(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := repository.ReserveCheckpointWrite(context.Background(), input)
	if err != nil || retry.Revision != reserved.Revision || retry.ReservedVersion != reserved.ReservedVersion {
		t.Fatalf("same operation reservation retry = %#v, %v", retry, err)
	}
	otherHash, _ := domainappdev.HashProviderExecutionOperationID("checkpoint-operation-2")
	other := input
	other.OperationHash = otherHash
	if _, err := repository.ReserveCheckpointWrite(context.Background(), other); !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("different operation while reserved = %v", err)
	}
	saveCAS := cas
	saveCAS.ExpectedVersion = reserved.ReservedVersion
	if _, err := repository.SaveCheckpoint(context.Background(), domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: saveCAS, OperationHash: operationHash, CheckpointWriteRevision: reserved.Revision,
		CheckpointEnvelope: strings.Repeat("x", sandboxcontract.MaxExecutionCheckpointEnvelopeBytes+1),
	}); !errors.Is(err, domainappdev.ErrProviderExecutionInvalid) {
		t.Fatalf("over-limit persistence error = %v", err)
	}
	saved, err := repository.SaveCheckpoint(context.Background(), domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: saveCAS, OperationHash: operationHash, CheckpointWriteRevision: reserved.Revision,
		CheckpointEnvelope: strings.Repeat("x", sandboxcontract.MaxExecutionCheckpointEnvelopeBytes),
	})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := repository.ReserveCheckpointWrite(context.Background(), input)
	if err != nil || !committed.AlreadyCommitted || committed.Record == nil || committed.Record.Version != saved.Version {
		t.Fatalf("committed operation retry = %#v, %v", committed, err)
	}
}

type providerExecutionLostResponseRepository struct {
	domainappdev.ProviderExecutionRepository
	mu              sync.Mutex
	failReserveOnce bool
	failSaveOnce    bool
	failCleanupOnce bool
}

func (r *providerExecutionLostResponseRepository) ReserveCheckpointWrite(ctx context.Context, input domainappdev.ReserveProviderCheckpointWriteInput) (*domainappdev.ProviderExecutionCheckpointReservation, error) {
	result, err := r.ProviderExecutionRepository.ReserveCheckpointWrite(ctx, input)
	r.mu.Lock()
	fail := err == nil && r.failReserveOnce
	r.failReserveOnce = false
	r.mu.Unlock()
	if fail {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	return result, err
}

func (r *providerExecutionLostResponseRepository) SaveCheckpoint(ctx context.Context, input domainappdev.SaveProviderCheckpointInput) (*domainappdev.ProviderExecution, error) {
	result, err := r.ProviderExecutionRepository.SaveCheckpoint(ctx, input)
	r.mu.Lock()
	fail := err == nil && r.failSaveOnce
	r.failSaveOnce = false
	r.mu.Unlock()
	if fail {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	return result, err
}

func (r *providerExecutionLostResponseRepository) CompleteCleanup(ctx context.Context, input domainappdev.CompleteProviderExecutionCleanupInput) (*domainappdev.ProviderExecution, error) {
	result, err := r.ProviderExecutionRepository.CompleteCleanup(ctx, input)
	r.mu.Lock()
	fail := err == nil && r.failCleanupOnce
	r.failCleanupOnce = false
	r.mu.Unlock()
	if fail {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	return result, err
}

type providerExecutionLostResponseCodec struct{ sealCalls int }

func (c *providerExecutionLostResponseCodec) Seal(context.Context, applicationsandbox.ExecutionCheckpointBinding, applicationsandbox.ExecutionCheckpoint) (string, error) {
	c.sealCalls++
	return "ecp1:lost-response-envelope", nil
}

func (*providerExecutionLostResponseCodec) Open(context.Context, applicationsandbox.ExecutionCheckpointBinding, string) (applicationsandbox.ExecutionCheckpoint, error) {
	return applicationsandbox.ExecutionCheckpoint{}, nil
}

func TestProviderExecutionQualityLostResponsesAreOperationIdempotentAcrossServiceRestart(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	setProviderExecutionDBTime(t, repository, time.Date(2026, 7, 16, 23, 30, 0, 0, time.UTC))
	faults := &providerExecutionLostResponseRepository{
		ProviderExecutionRepository: repository, failReserveOnce: true, failSaveOnce: true,
	}
	codec := &providerExecutionLostResponseCodec{}
	service, err := applicationappdev.NewProviderExecutionService(
		faults, codec,
		applicationappdev.WithProviderExecutionRandom(bytes.NewReader(bytes.Repeat([]byte{0x51}, 128))),
	)
	if err != nil {
		t.Fatal(err)
	}
	started, err := service.EnsureStart(context.Background(), applicationappdev.EnsureProviderExecutionStartRequest{
		SpaceID: "1001", ProjectID: "project-a", IdempotencyKey: "lost-response-idem",
		ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.ClaimRecovery(context.Background(), applicationappdev.ClaimProviderExecutionRecoveryRequest{
		SpaceID: started.SpaceID, ProjectID: started.ProjectID, Generation: started.Generation, ExpectedVersion: started.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := applicationappdev.SaveProviderExecutionCheckpointRequest{
		Owner: claimed.Owner(), ExpectedVersion: claimed.Metadata.Version, OperationID: "lost-checkpoint-operation",
		ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
	}
	if _, err := service.SaveCheckpoint(context.Background(), request); !errors.Is(err, domainappdev.ErrProviderExecutionUnavailable) || codec.sealCalls != 0 {
		t.Fatalf("lost reserve response = %v, seals=%d", err, codec.sealCalls)
	}
	if _, err := service.SaveCheckpoint(context.Background(), request); !errors.Is(err, domainappdev.ErrProviderExecutionUnavailable) || codec.sealCalls != 1 {
		t.Fatalf("lost save response = %v, seals=%d", err, codec.sealCalls)
	}
	restarted, err := applicationappdev.NewProviderExecutionService(faults, codec)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := restarted.SaveCheckpoint(context.Background(), request)
	if err != nil || codec.sealCalls != 1 || saved.Version != claimed.Metadata.Version+2 {
		t.Fatalf("committed checkpoint retry = %#v, %v seals=%d", saved, err, codec.sealCalls)
	}

	stop, err := restarted.SetDesiredStop(context.Background(), applicationappdev.SetProviderExecutionDesiredStopRequest{
		Owner: claimed.Owner(), ExpectedVersion: saved.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	faults.mu.Lock()
	faults.failCleanupOnce = true
	faults.mu.Unlock()
	cleanup := applicationappdev.CompleteProviderExecutionCleanupRequest{
		Owner: claimed.Owner(), ExpectedVersion: stop.Version, OperationID: "lost-cleanup-operation",
	}
	if _, err := restarted.CompleteCleanup(context.Background(), cleanup); !errors.Is(err, domainappdev.ErrProviderExecutionUnavailable) {
		t.Fatalf("lost cleanup response = %v", err)
	}
	restartedAgain, err := applicationappdev.NewProviderExecutionService(faults, codec)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := restartedAgain.CompleteCleanup(context.Background(), cleanup)
	if err != nil || completed.Version != stop.Version+1 || completed.ObservedState != domainappdev.ProviderExecutionObservedCleanupComplete {
		t.Fatalf("committed cleanup retry = %#v, %v", completed, err)
	}
}

func TestProviderExecutionQualityRetryPolicyIsBoundedTypedAndContextAware(t *testing.T) {
	calls := 0
	waits := 0
	policy := providerExecutionRetryPolicy{
		attempts: 6,
		delay:    func(attempt int) time.Duration { return time.Duration(attempt+1) * time.Millisecond },
		wait: func(ctx context.Context, _ time.Duration) error {
			waits++
			return ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runProviderExecutionRetry(ctx, policy, func() error {
		calls++
		return &mysqldriver.MySQLError{Number: 1213}
	})
	if !errors.Is(err, context.Canceled) || calls != 0 || waits != 0 {
		t.Fatalf("canceled retry = %v calls/waits=%d/%d", err, calls, waits)
	}

	calls = 0
	waits = 0
	policy.wait = func(context.Context, time.Duration) error { waits++; return nil }
	err = runProviderExecutionRetry(context.Background(), policy, func() error {
		calls++
		if calls < 3 {
			return &mysqldriver.MySQLError{Number: 1205}
		}
		return nil
	})
	if err != nil || calls != 3 || waits != 2 {
		t.Fatalf("typed retry = %v calls/waits=%d/%d", err, calls, waits)
	}

	calls = 0
	err = runProviderExecutionRetry(context.Background(), policy, func() error { calls++; return errors.New("not retryable") })
	if err == nil || calls != 1 {
		t.Fatalf("unknown error retry = %v calls=%d", err, calls)
	}
}

func TestProviderExecutionQualityConstantTimeHashesRejectWrongLength(t *testing.T) {
	owner := testOwnerHash("constant-time-owner")
	if !owner.EqualBytes(owner.Bytes()) || owner.EqualBytes(owner.Bytes()[:31]) || owner.EqualBytes(append(owner.Bytes(), 0)) {
		t.Fatal("owner hash length/value comparison contract failed")
	}
	operation := testOperationHash(t, "constant-time-operation")
	if !operation.EqualBytes(operation.Bytes()) || operation.EqualBytes(operation.Bytes()[:31]) || operation.EqualBytes(append(operation.Bytes(), 0)) {
		t.Fatal("operation hash length/value comparison contract failed")
	}
}

func TestProviderExecutionQualityExpiredOrAbortedReservationCanBeReplaced(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	clock := &providerExecutionControlledDBClock{now: time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)}
	repository.clock = clock
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("replace-id", "replace-idem", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	ownerHash := testOwnerHash("replace-owner")
	record, err = repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: ownerHash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(record, ownerHash, time.Time{})
	firstHash := testOperationHash(t, "replace-operation-1")
	first, err := repository.ReserveCheckpointWrite(context.Background(), domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: firstHash, ReservationDuration: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.set(clock.current().Add(11 * time.Second))
	secondHash := testOperationHash(t, "replace-operation-2")
	second, err := repository.ReserveCheckpointWrite(context.Background(), domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: secondHash, ReservationDuration: 10 * time.Second,
	})
	if err != nil || second.Revision != first.Revision+1 || second.ReservedVersion != first.ReservedVersion+1 {
		t.Fatalf("expired reservation replacement = %#v, %v", second, err)
	}
	abortCAS := cas
	abortCAS.ExpectedVersion = second.ReservedVersion
	aborted, err := repository.AbortCheckpointWrite(context.Background(), domainappdev.AbortProviderCheckpointWriteInput{
		OwnerCAS: abortCAS, OperationHash: secondHash, CheckpointWriteRevision: second.Revision,
	})
	if err != nil || aborted.CheckpointWritePending {
		t.Fatalf("abort reservation = %#v, %v", aborted, err)
	}
	thirdHash := testOperationHash(t, "replace-operation-3")
	thirdCAS := providerExecutionCAS(aborted, ownerHash, time.Time{})
	third, err := repository.ReserveCheckpointWrite(context.Background(), domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: thirdCAS, OperationHash: thirdHash, ReservationDuration: 10 * time.Second,
	})
	if err != nil || third.Revision != second.Revision+1 {
		t.Fatalf("aborted reservation replacement = %#v, %v", third, err)
	}
}

func TestProviderExecutionQualityMalformedStoredEnvelopeFailsClosed(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("corrupt-id", "corrupt-idem", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	operationHash := testOperationHash(t, "corrupt-operation")
	if err := db.Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, "project-a", record.Generation).
		Updates(map[string]any{
			"checkpoint_envelope":       strings.Repeat("x", sandboxcontract.MaxExecutionCheckpointEnvelopeBytes+1),
			"checkpoint_write_revision": 1, "checkpoint_last_operation_hash": operationHash.Bytes(),
		}).Error; err != nil {
		t.Fatal(err)
	}
	_, err = repository.ListRecoverable(context.Background(), domainappdev.ListRecoverableProviderExecutionsInput{
		SpaceID: "1001", ProjectID: "project-a", Limit: 10,
	})
	if !errors.Is(err, domainappdev.ErrProviderExecutionUnavailable) {
		t.Fatalf("malformed stored envelope recovery = %v", err)
	}
}

func TestProviderExecutionQualityStateSpecificEntrypoints(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	clock := &providerExecutionControlledDBClock{now: time.Date(2026, 7, 16, 23, 0, 0, 0, time.UTC)}
	repository.clock = clock
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("state-id", "state-idem", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	hash := testOwnerHash("state-owner")
	record, err = repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: hash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(record, hash, time.Time{})
	for _, forbidden := range []domainappdev.ProviderExecutionObservedState{
		domainappdev.ProviderExecutionObservedSubmitting,
		domainappdev.ProviderExecutionObservedRunning,
		domainappdev.ProviderExecutionObservedCleanupComplete,
	} {
		if _, err := repository.AdvanceTerminal(context.Background(), domainappdev.AdvanceProviderExecutionTerminalInput{
			OwnerCAS: cas, OperationHash: testOperationHash(t, "forbidden-terminal-"+string(forbidden)), ObservedState: forbidden,
		}); !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
			t.Fatalf("generic transition to %s = %v", forbidden, err)
		}
	}
	started, err := repository.StartSubmission(context.Background(), providerExecutionStartSubmissionInput(t, cas, "quality-fixture-launch"))
	if err != nil || started.ObservedState != domainappdev.ProviderExecutionObservedPending ||
		started.LaunchState != domainappdev.ProviderExecutionLaunchPrepared {
		t.Fatalf("StartSubmission = %#v, %v", started, err)
	}
	started = markProviderExecutionSubmittedForTest(t, repository, started, hash, "quality-fixture-launch")
	saveCAS := providerExecutionCAS(started, hash, time.Time{})
	running, err := repository.SaveSubmission(context.Background(), providerExecutionSaveSubmissionInput(
		t, saveCAS, "quality-fixture-launch", "provider-execution-quality", time.Minute, "",
	))
	if err != nil || running.ObservedState != domainappdev.ProviderExecutionObservedRunning {
		t.Fatalf("SaveSubmission = %#v, %v", running, err)
	}
}

func TestProviderExecutionQualityMySQLClockExpressionsAreAuthoritative(t *testing.T) {
	clock := providerExecutionMySQLDBClock{}
	if nowSQL := clock.nowExpression().SQL; !strings.Contains(nowSQL, "UTC_TIMESTAMP(6)") {
		t.Fatalf("MySQL DB now expression = %q", nowSQL)
	}
	if expirySQL := clock.addExpression(time.Minute).SQL; !strings.Contains(expirySQL, "UTC_TIMESTAMP(6)") {
		t.Fatalf("MySQL DB expiry expression = %q", expirySQL)
	}
}
