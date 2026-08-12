// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestProviderExecutionRepositoryIdempotencyAndConcurrentGeneration(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	first, err := repository.EnsureStart(ctx, providerExecutionStartInput("id-1", "idem-same", now))
	if err != nil {
		t.Fatal(err)
	}
	retry, err := repository.EnsureStart(ctx, providerExecutionStartInput("id-ignored", "idem-same", now.Add(time.Second)))
	if err != nil || retry.ID != first.ID || retry.Generation != first.Generation {
		t.Fatalf("idempotent retry = %#v, %v; want %#v", retry, err, first)
	}

	const concurrent = 24
	results := make(chan *domainappdev.ProviderExecution, concurrent)
	errorsCh := make(chan error, concurrent)
	for index := range concurrent {
		go func(index int) {
			input := providerExecutionStartInput(fmt.Sprintf("id-%02d", index+10), fmt.Sprintf("idem-%02d", index), now)
			record, err := repository.EnsureStart(ctx, input)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- record
		}(index)
	}
	generations := make([]int, 0, concurrent)
	for range concurrent {
		select {
		case err := <-errorsCh:
			t.Fatalf("concurrent EnsureStart: %v", err)
		case record := <-results:
			generations = append(generations, int(record.Generation))
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent generation allocation timed out")
		}
	}
	sort.Ints(generations)
	for index, generation := range generations {
		if generation != index+2 {
			t.Fatalf("generation[%d] = %d, all = %v", index, generation, generations)
		}
	}
}

func TestProviderExecutionRepositoryClaimCASExpiryAndTenantIsolation(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	setProviderExecutionDBTime(t, repository, now)
	record, err := repository.EnsureStart(ctx, providerExecutionStartInput("claim-id", "claim-idem", now))
	if err != nil {
		t.Fatal(err)
	}

	const claimers = 16
	var successesMu sync.Mutex
	successes := make([]*domainappdev.ProviderExecution, 0, 1)
	errs := make(chan error, claimers)
	for index := range claimers {
		go func(index int) {
			claimed, err := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
				SpaceID: "1001", ProjectID: "project-a", Generation: record.Generation, ExpectedVersion: record.Version,
				OwnerHash: testOwnerHash(fmt.Sprintf("owner-%d", index)), LeaseDuration: time.Minute,
			})
			if err != nil {
				errs <- err
				return
			}
			successesMu.Lock()
			successes = append(successes, claimed)
			successesMu.Unlock()
			errs <- nil
		}(index)
	}
	conflicts := 0
	for range claimers {
		if err := <-errs; err != nil {
			if !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
				t.Fatalf("claim error = %v", err)
			}
			conflicts++
		}
	}
	if len(successes) != 1 || conflicts != claimers-1 || successes[0].OwnerEpoch != 1 {
		t.Fatalf("claim winners/conflicts = %d/%d, winner %#v", len(successes), conflicts, successes)
	}
	winner := successes[0]
	setProviderExecutionDBTime(t, repository, now.Add(2*time.Minute))
	taken, err := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-a", Generation: winner.Generation, ExpectedVersion: winner.Version,
		OwnerHash: testOwnerHash("takeover"), LeaseDuration: time.Minute,
	})
	if err != nil || taken.OwnerEpoch != 2 {
		t.Fatalf("expired takeover = %#v, %v", taken, err)
	}
	if _, err := repository.LoadOwnedRecovery(ctx, domainappdev.ProviderExecutionOwnerCAS{
		SpaceID: "2002", ProjectID: "project-a", Generation: taken.Generation, ExpectedVersion: taken.Version,
		OwnerHash: testOwnerHash("takeover"), OwnerEpoch: taken.OwnerEpoch,
	}); !errors.Is(err, domainappdev.ErrProviderExecutionGenerationConflict) {
		t.Fatalf("cross-tenant recovery error = %v", err)
	}
}

func TestProviderExecutionRepositoryOwnedMutationsAreCASAndMonotonic(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	record, err := repository.EnsureStart(ctx, providerExecutionStartInput("mutate-id", "mutate-idem", now))
	if err != nil {
		t.Fatal(err)
	}
	ownerHash := testOwnerHash("mutation-owner")
	record, err = repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-a", Generation: record.Generation, ExpectedVersion: record.Version,
		OwnerHash: ownerHash, LeaseDuration: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(record, ownerHash, now)
	record, err = repository.StartSubmission(ctx, providerExecutionStartSubmissionInput(t, cas, "repository-fixture-launch"))
	if err != nil {
		t.Fatal(err)
	}
	record = markProviderExecutionSubmittedForTest(
		t, repository, record, ownerHash, "repository-fixture-launch",
	)
	cas = providerExecutionCAS(record, ownerHash, now)
	record, err = repository.SaveSubmission(ctx, providerExecutionSaveSubmissionInput(
		t, cas, "repository-fixture-launch", "provider-execution-1", 30*time.Minute, "",
	))
	if err != nil {
		t.Fatal(err)
	}
	cas = providerExecutionCAS(record, ownerHash, now)
	if _, err := repository.SaveSubmission(ctx, providerExecutionSaveSubmissionInput(
		t, cas, "repository-fixture-launch", "provider-execution-2", 30*time.Minute, "",
	)); !errors.Is(err, domainappdev.ErrProviderExecutionIDConflict) {
		t.Fatalf("provider ID overwrite error = %v", err)
	}
	operationHash := testOperationHash(t, "repository-checkpoint-operation")
	reservation, err := repository.ReserveCheckpointWrite(ctx, domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: operationHash, ReservationDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	cas.ExpectedVersion = reservation.ReservedVersion
	record, err = repository.SaveCheckpoint(ctx, domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: cas, OperationHash: operationHash, LaunchOperationHash: record.LaunchOperationHash,
		CheckpointWriteRevision: reservation.Revision, CheckpointEnvelope: "ecp1:opaque",
	})
	if err != nil {
		t.Fatal(err)
	}
	cas = providerExecutionCAS(record, ownerHash, now)
	record, err = repository.AdvanceTerminal(ctx, domainappdev.AdvanceProviderExecutionTerminalInput{
		OwnerCAS: cas, OperationHash: testOperationHash(t, "repository-terminal-operation"), ObservedState: domainappdev.ProviderExecutionObservedFailed,
		SafeErrorCode: "provider_failed", SafeErrorMessage: "provider execution failed", PreviewRoute: "/preview/project-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	cas = providerExecutionCAS(record, ownerHash, now)
	if _, err := repository.AdvanceTerminal(ctx, domainappdev.AdvanceProviderExecutionTerminalInput{
		OwnerCAS: cas, OperationHash: testOperationHash(t, "repository-invalid-terminal-regression"),
		ObservedState: domainappdev.ProviderExecutionObservedRunning,
	}); !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("terminal regression error = %v", err)
	}
	stale := cas
	stale.ExpectedVersion--
	if _, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: stale}); !errors.Is(err, domainappdev.ErrProviderExecutionVersionConflict) {
		t.Fatalf("stale version error = %v", err)
	}
	stale = cas
	stale.OwnerHash = testOwnerHash("stale-owner")
	if _, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: stale}); !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		t.Fatalf("stale owner error = %v", err)
	}
}

func TestProviderExecutionRepositoryRecoverableListIsSafeAndOwnerMaintenanceIsIdempotent(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	pending, err := repository.EnsureStart(ctx, providerExecutionStartInput("recoverable-id", "recoverable-idem", now))
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := repository.ListRecoverable(ctx, domainappdev.ListRecoverableProviderExecutionsInput{
		SpaceID: "1001", ProjectID: "project-a", Limit: 10,
	})
	if err != nil || len(candidates) != 1 || candidates[0].Generation != pending.Generation {
		t.Fatalf("ListRecoverable() = %#v, %v", candidates, err)
	}
	candidateType := reflect.TypeOf(*candidates[0])
	for _, forbidden := range []string{"CheckpointEnvelope", "OwnerIdentityHash", "ArtifactObjectKey", "ProviderExecutionID", "IdempotencyKey"} {
		if _, exists := candidateType.FieldByName(forbidden); exists {
			t.Fatalf("recoverable candidate exposed %s", forbidden)
		}
	}

	ownerHash := testOwnerHash("maintenance-owner")
	claimed, err := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-a", Generation: pending.Generation, ExpectedVersion: pending.Version,
		OwnerHash: ownerHash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	cas := providerExecutionCAS(claimed, ownerHash, now)
	renewal := domainappdev.RenewProviderExecutionOwnerInput{OwnerCAS: cas, LeaseDuration: 2 * time.Minute}
	firstRenew, err := repository.RenewOwner(ctx, renewal)
	if err != nil {
		t.Fatal(err)
	}
	secondRenew, err := repository.RenewOwner(ctx, renewal)
	if err != nil || secondRenew.Version != firstRenew.Version || secondRenew.OwnerExpiresAt == nil || firstRenew.OwnerExpiresAt == nil {
		t.Fatalf("idempotent renewal = %#v, %v; first %#v", secondRenew, err, firstRenew)
	}
	releaseHash, err := domainappdev.HashProviderExecutionReleaseOperationID("maintenance-release", ownerHash)
	if err != nil {
		t.Fatal(err)
	}
	release := domainappdev.ReleaseProviderExecutionOwnerInput{OwnerCAS: cas, OperationHash: releaseHash}
	firstRelease, err := repository.ReleaseOwner(ctx, release)
	if err != nil {
		t.Fatal(err)
	}
	secondRelease, err := repository.ReleaseOwner(ctx, release)
	if err != nil || secondRelease.Version != firstRelease.Version || !secondRelease.OwnerIdentityHash.IsZero() {
		t.Fatalf("idempotent release = %#v, %v; first %#v", secondRelease, err, firstRelease)
	}
}

func newProviderExecutionSQLiteRepository(t *testing.T, spaceID int64, projectID string) (*ProviderExecutionRepository, *gorm.DB) {
	t.Helper()
	db, sqlDB := openProviderExecutionSQLiteTestDB(t, filepath.Join(t.TempDir(), "provider-executions.db"))
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := migrateProviderExecutionSQLiteTestSchema(db); err != nil {
		t.Fatal(err)
	}
	seedProviderExecutionProject(t, db, spaceID, projectID)
	repository := NewProviderExecutionRepository(db)
	repository.clock = &providerExecutionControlledDBClock{now: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)}
	return repository, db
}

func openProviderExecutionSQLiteTestDB(t *testing.T, path string) (*gorm.DB, *sql.DB) {
	t.Helper()
	dsn := "file:" + filepath.ToSlash(path) + "?_foreign_keys=on&_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(8)
	return db, sqlDB
}

func migrateProviderExecutionSQLiteTestSchema(db *gorm.DB) error {
	if err := db.Exec(`CREATE TABLE appdev_projects (id TEXT NOT NULL PRIMARY KEY, space_id INTEGER NOT NULL)`).Error; err != nil {
		return err
	}
	return migrateProviderExecutionSQLiteExecutionTestSchema(db)
}

func migrateProviderExecutionSQLiteExecutionTestSchema(db *gorm.DB) error {
	statements := []string{
		`CREATE TABLE appdev_provider_executions (
            id TEXT NOT NULL PRIMARY KEY,
            space_id INTEGER NOT NULL,
            project_id TEXT NOT NULL,
			actor_user_id INTEGER NOT NULL DEFAULT 0,
            generation INTEGER NOT NULL,
            idempotency_key BLOB NOT NULL,
            desired_state TEXT NOT NULL,
            observed_state TEXT NOT NULL,
            provider_key TEXT NOT NULL,
            provider_scope TEXT NOT NULL,
            provider_execution_id BLOB NOT NULL,
            submission_started_at DATETIME NULL,
            launch_state TEXT NOT NULL DEFAULT 'none',
            launch_operation_hash BLOB NULL,
            launch_provider_operation_id BLOB NOT NULL DEFAULT '',
            launch_request_digest BLOB NULL,
            launch_expires_at DATETIME NULL,
            checkpoint_envelope TEXT NOT NULL,
            checkpoint_write_revision INTEGER NOT NULL DEFAULT 0,
            checkpoint_write_pending INTEGER NOT NULL DEFAULT 0,
            checkpoint_write_operation_hash BLOB NULL,
            checkpoint_write_expires_at DATETIME NULL,
            checkpoint_last_operation_hash BLOB NULL,
            cleanup_operation_hash BLOB NULL,
            terminal_operation_hash BLOB NULL,
            release_owner_operation_hash BLOB NULL,
            provider_lease_expires_at DATETIME NULL,
            owner_identity_hash BLOB NULL,
            owner_epoch INTEGER NOT NULL DEFAULT 0,
            owner_expires_at DATETIME NULL,
            preview_route TEXT NOT NULL DEFAULT '',
            artifact_object_key TEXT NOT NULL DEFAULT '',
			build_operation_id BLOB NOT NULL DEFAULT '',
			build_operation_hash BLOB NULL,
			artifact_status TEXT NOT NULL DEFAULT 'none',
			artifact_kind TEXT NOT NULL DEFAULT '',
			artifact_digest TEXT NOT NULL DEFAULT '',
			artifact_size INTEGER NOT NULL DEFAULT 0,
			artifact_version INTEGER NOT NULL DEFAULT 0,
			build_started_at DATETIME NULL,
			artifact_updated_at DATETIME NULL,
			artifact_safe_error_code TEXT NOT NULL DEFAULT '',
			artifact_safe_error_message TEXT NOT NULL DEFAULT '',
            safe_error_code TEXT NOT NULL DEFAULT '',
            safe_error_message TEXT NOT NULL DEFAULT '',
            version INTEGER NOT NULL DEFAULT 1,
            created_at DATETIME NOT NULL,
            updated_at DATETIME NOT NULL
        )`,
		`CREATE UNIQUE INDEX uk_appdev_provider_exec_generation ON appdev_provider_executions (space_id, project_id, generation)`,
		`CREATE UNIQUE INDEX uk_appdev_provider_exec_idempotency ON appdev_provider_executions (space_id, project_id, idempotency_key)`,
		`CREATE INDEX idx_appdev_provider_exec_recoverable ON appdev_provider_executions (space_id, project_id, observed_state, desired_state, owner_expires_at, updated_at, id)`,
		`CREATE INDEX idx_appdev_provider_exec_provider_id ON appdev_provider_executions (space_id, project_id, provider_key, provider_execution_id)`,
		`CREATE INDEX idx_appdev_provider_exec_launch ON appdev_provider_executions (space_id, project_id, launch_state, launch_expires_at)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedProviderExecutionProject(t *testing.T, db *gorm.DB, spaceID int64, projectID string) {
	t.Helper()
	if err := db.Exec("INSERT INTO appdev_projects (id, space_id) VALUES (?, ?)", projectID, spaceID).Error; err != nil {
		t.Fatal(err)
	}
}

func providerExecutionStartInput(id, idempotencyKey string, now time.Time) domainappdev.EnsureProviderExecutionStartInput {
	_ = now
	return domainappdev.EnsureProviderExecutionStartInput{
		ID: id, SpaceID: "1001", ProjectID: "project-a", ActorUserID: 42, IdempotencyKey: idempotencyKey,
		ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
	}
}

func testOwnerHash(value string) domainappdev.ProviderExecutionOwnerHash {
	digest := sha256.Sum256([]byte(value))
	return domainappdev.ProviderExecutionOwnerHash(digest)
}

func providerExecutionCAS(record *domainappdev.ProviderExecution, ownerHash domainappdev.ProviderExecutionOwnerHash, now time.Time) domainappdev.ProviderExecutionOwnerCAS {
	_ = now
	return domainappdev.ProviderExecutionOwnerCAS{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: ownerHash, OwnerEpoch: record.OwnerEpoch,
		ProviderKey: record.ProviderKey, ProviderScope: record.ProviderScope,
	}
}

func setProviderExecutionDBTime(t *testing.T, repository *ProviderExecutionRepository, now time.Time) {
	t.Helper()
	clock, ok := repository.clock.(*providerExecutionControlledDBClock)
	if !ok {
		t.Fatal("repository does not use the controlled SQLite DB clock")
	}
	clock.set(now)
}

func testOperationHash(t *testing.T, operationID string) domainappdev.ProviderExecutionOperationHash {
	t.Helper()
	hash, err := domainappdev.HashProviderExecutionOperationID(operationID)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
