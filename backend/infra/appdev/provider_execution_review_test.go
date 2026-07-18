// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	mysqldriver "github.com/go-sql-driver/mysql"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestProviderExecutionRepositoryRequiresExactRecoveryProject(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	seedProviderExecutionProject(t, db, 1001, "project-b")
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	if _, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("scope-a", "scope-a", now)); err != nil {
		t.Fatal(err)
	}
	other := providerExecutionStartInput("scope-b", "scope-b", now)
	other.ProjectID = "project-b"
	if _, err := repository.EnsureStart(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	items, err := repository.ListRecoverable(context.Background(), domainappdev.ListRecoverableProviderExecutionsInput{
		SpaceID: "1001", ProjectID: "project-a", Limit: 10,
	})
	if err != nil || len(items) != 1 || items[0].ProjectID != "project-a" {
		t.Fatalf("scoped recovery = %#v, %v", items, err)
	}
	_, err = repository.ListRecoverable(context.Background(), domainappdev.ListRecoverableProviderExecutionsInput{
		SpaceID: "1001", Limit: 10,
	})
	if !errors.Is(err, domainappdev.ErrProviderExecutionInvalid) {
		t.Fatalf("empty project error = %v", err)
	}
}

func TestProviderExecutionRepositoryDesiredStopIsOwnerBoundAndIdempotent(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 14, 0, 0, 0, time.UTC)
	record, err := repository.EnsureStart(ctx, providerExecutionStartInput("stop-id", "stop-idem", now))
	if err != nil {
		t.Fatal(err)
	}
	ownerHash := testOwnerHash("stop-owner")
	record, err = repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: ownerHash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstCAS := providerExecutionCAS(record, ownerHash, now)
	first, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: firstCAS})
	if err != nil || first.DesiredState != domainappdev.ProviderExecutionDesiredStop || first.Version != record.Version+1 {
		t.Fatalf("first stop = %#v, %v", first, err)
	}
	oldVersionRetry, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: firstCAS})
	if err != nil || oldVersionRetry.Version != first.Version {
		t.Fatalf("old-version retry = %#v, %v", oldVersionRetry, err)
	}
	currentCAS := providerExecutionCAS(first, ownerHash, now)
	currentRetry, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: currentCAS})
	if err != nil || currentRetry.Version != first.Version {
		t.Fatalf("current-version retry = %#v, %v", currentRetry, err)
	}

	takeoverHash := testOwnerHash("stop-takeover")
	takeoverNow := now.Add(2 * time.Minute)
	setProviderExecutionDBTime(t, repository, takeoverNow)
	taken, err := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: first.SpaceID, ProjectID: first.ProjectID, Generation: first.Generation,
		ExpectedVersion: first.Version, OwnerHash: takeoverHash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	stale := firstCAS
	if _, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: stale}); !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("stale owner stop error = %v", err)
	}
	takenCAS := providerExecutionCAS(taken, takeoverHash, takeoverNow)
	if retry, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: takenCAS}); err != nil || retry.Version != taken.Version {
		t.Fatalf("new owner stop retry = %#v, %v", retry, err)
	}
}

func TestProviderExecutionRepositoryConcurrentDesiredStopIncrementsOnce(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	now := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("stop-concurrent", "stop-concurrent", now))
	if err != nil {
		t.Fatal(err)
	}
	hash := testOwnerHash("stop-concurrent-owner")
	record, err = repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: hash, LeaseDuration: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(record, hash, now)
	const workers = 12
	results := make(chan *domainappdev.ProviderExecution, workers)
	errs := make(chan error, workers)
	runProviderExecutionBarrier(workers, func(_ int) {
		result, callErr := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: cas})
		results <- result
		errs <- callErr
	})
	for range workers {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if result := <-results; result.Version != record.Version+1 || result.DesiredState != domainappdev.ProviderExecutionDesiredStop {
			t.Fatalf("concurrent stop = %#v", result)
		}
	}
}

func TestProviderExecutionRepositoryCheckpointReservationIsRetryable(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	now := time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC)
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("checkpoint-id", "checkpoint-idem", now))
	if err != nil {
		t.Fatal(err)
	}
	hash := testOwnerHash("checkpoint-owner")
	record, err = repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: hash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(record, hash, now)
	operationHash := testOperationHash(t, "review-checkpoint-operation")
	reservationInput := domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: operationHash, ReservationDuration: time.Minute,
	}
	reservation, err := repository.ReserveCheckpointWrite(context.Background(), reservationInput)
	if err != nil || reservation.Revision != 1 || reservation.ReservedVersion != record.Version+1 {
		t.Fatalf("reservation = %#v, %v", reservation, err)
	}
	retry, err := repository.ReserveCheckpointWrite(context.Background(), reservationInput)
	if err != nil || !reflect.DeepEqual(retry, reservation) {
		t.Fatalf("crash retry reservation = %#v, %v", retry, err)
	}
	saveCAS := cas
	saveCAS.ExpectedVersion = reservation.ReservedVersion
	saved, err := repository.SaveCheckpoint(context.Background(), domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: saveCAS, OperationHash: operationHash, CheckpointWriteRevision: reservation.Revision, CheckpointEnvelope: "ecp1:reserved",
	})
	if err != nil || saved.Version != reservation.ReservedVersion+1 || saved.CheckpointEnvelope != "ecp1:reserved" {
		t.Fatalf("reserved save = %#v, %v", saved, err)
	}
	stale := cas
	stale.OwnerHash = testOwnerHash("stale-checkpoint-owner")
	staleInput := reservationInput
	staleInput.OwnerCAS = stale
	if _, err := repository.ReserveCheckpointWrite(context.Background(), staleInput); !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("stale reservation error = %v", err)
	}
}

func TestProviderExecutionRepositorySQLiteUsesMultipleConnectionsAndBarriers(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB.Stats().MaxOpenConnections <= 1 {
		t.Fatalf("SQLite concurrency pool = %d", sqlDB.Stats().MaxOpenConnections)
	}
	now := time.Date(2026, 7, 16, 17, 0, 0, 0, time.UTC)
	const workers = 12
	results := make(chan *domainappdev.ProviderExecution, workers)
	errs := make(chan error, workers)
	runProviderExecutionBarrier(workers, func(index int) {
		result, callErr := repository.EnsureStart(context.Background(), providerExecutionStartInput(
			fmt.Sprintf("barrier-id-%d", index), "barrier-idem", now,
		))
		results <- result
		errs <- callErr
	})
	var firstID string
	for range workers {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		result := <-results
		if firstID == "" {
			firstID = result.ID
		}
		if result.ID != firstID || result.Generation != 1 {
			t.Fatalf("same-key barrier result = %#v, first=%q", result, firstID)
		}
	}
}

func TestProviderExecutionDatabaseErrorClassificationIsTyped(t *testing.T) {
	duplicates := []error{
		gorm.ErrDuplicatedKey,
		fmt.Errorf("wrapped: %w", &mysqldriver.MySQLError{Number: 1062}),
		sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintUnique},
		sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintPrimaryKey},
	}
	for _, err := range duplicates {
		if !providerExecutionDuplicate(err) {
			t.Fatalf("typed duplicate not recognized: %T", err)
		}
	}
	for _, err := range []error{
		&mysqldriver.MySQLError{Number: 1048},
		sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintNotNull},
		errors.New("UNIQUE constraint failed: must not be string matched"),
	} {
		if providerExecutionDuplicate(err) {
			t.Fatalf("non-duplicate recognized: %T %v", err, err)
		}
	}
	unknown := errors.New("unknown database failure")
	if normalized := normalizeProviderExecutionDatabaseError(unknown); !errors.Is(normalized, domainappdev.ErrProviderExecutionUnavailable) || errors.Is(normalized, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("unknown database error = %v", normalized)
	}
}

func TestProviderExecutionRecordTagsMatchMigration(t *testing.T) {
	typeOfRecord := reflect.TypeOf(providerExecutionRecord{})
	expectedTypes := map[string]string{
		"ID": "type:varchar(64)", "SpaceID": "type:bigint unsigned", "ProjectID": "type:varchar(64)",
		"Generation": "type:bigint unsigned", "IdempotencyKey": "type:varbinary(128)",
		"ProviderExecutionID": "type:varbinary(128)", "CheckpointEnvelope": "type:mediumtext",
		"CheckpointWriteRevision": "type:bigint unsigned", "CheckpointWritePending": "type:tinyint(1)",
		"SubmissionStartedAt": "type:datetime(6)", "CheckpointWriteOperationHash": "type:binary(32)",
		"CheckpointWriteExpiresAt": "type:datetime(6)", "CheckpointLastOperationHash": "type:binary(32)",
		"CleanupOperationHash": "type:binary(32)", "TerminalOperationHash": "type:binary(32)",
		"ReleaseOwnerOperationHash": "type:binary(32)",
		"ProviderLeaseExpiresAt":    "type:datetime(6)", "OwnerIdentityHash": "type:binary(32)",
		"OwnerEpoch": "type:bigint unsigned", "OwnerExpiresAt": "type:datetime(6)",
		"Version": "type:bigint unsigned", "CreatedAt": "type:datetime(6)", "UpdatedAt": "type:datetime(6)",
	}
	for fieldName, expected := range expectedTypes {
		field, ok := typeOfRecord.FieldByName(fieldName)
		if !ok || !strings.Contains(strings.ToLower(field.Tag.Get("gorm")), expected) {
			t.Errorf("%s tag = %q, want %q", fieldName, field.Tag.Get("gorm"), expected)
		}
	}
	for fieldName, expected := range map[string]string{
		"SpaceID":        "index:idx_appdev_provider_exec_recoverable,priority:1",
		"ProjectID":      "index:idx_appdev_provider_exec_recoverable,priority:2",
		"ObservedState":  "index:idx_appdev_provider_exec_recoverable,priority:3",
		"DesiredState":   "index:idx_appdev_provider_exec_recoverable,priority:4",
		"OwnerExpiresAt": "index:idx_appdev_provider_exec_recoverable,priority:5",
		"UpdatedAt":      "index:idx_appdev_provider_exec_recoverable,priority:6",
		"ID":             "index:idx_appdev_provider_exec_recoverable,priority:7",
	} {
		field, _ := typeOfRecord.FieldByName(fieldName)
		if !strings.Contains(field.Tag.Get("gorm"), expected) {
			t.Errorf("%s recoverable tag = %q", fieldName, field.Tag.Get("gorm"))
		}
	}
}

func TestProviderExecutionGenerationAllocationDeclaresScopedRowLock(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	var sawLock, sawScope bool
	if err := db.Callback().Query().Before("gorm:query").Register("appdev:provider_execution_lock_evidence", func(tx *gorm.DB) {
		if tx.Statement.Table != "appdev_projects" {
			return
		}
		_, sawLock = tx.Statement.Clauses["FOR"]
		where, ok := tx.Statement.Clauses["WHERE"].Expression.(clause.Where)
		if !ok {
			return
		}
		for _, expression := range where.Exprs {
			if scoped, ok := expression.(clause.Expr); ok && scoped.SQL == "space_id = ? AND id = ?" {
				sawScope = true
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("lock-id", "lock-idem", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if !sawLock || !sawScope {
		t.Fatalf("generation lock/scope evidence = %t/%t", sawLock, sawScope)
	}
}

func runProviderExecutionBarrier(workers int, operation func(int)) {
	var ready sync.WaitGroup
	ready.Add(workers)
	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(workers)
	for index := range workers {
		go func(index int) {
			defer done.Done()
			ready.Done()
			<-start
			operation(index)
		}(index)
	}
	ready.Wait()
	close(start)
	done.Wait()
}
