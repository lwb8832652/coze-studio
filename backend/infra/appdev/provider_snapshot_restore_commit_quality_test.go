/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package appdev

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

var errSnapshotRestoreCommitUnknownForTest = errors.New("commit result unknown")

func TestPersistentStoreProviderSnapshotRestoreCommitUnknownUsesAuthority(t *testing.T) {
	t.Run("committed transaction is returned as success and object is retained", func(t *testing.T) {
		store, db, objects, project, _, input := newSnapshotRestoreCommitFixture(t, "restore-commit-unknown-success")
		store.snapshotRestoreAttemptID = func() (string, error) { return "commit-visible", nil }
		store.snapshotRestoreTransaction = func(ctx context.Context, database *gorm.DB, callback func(*gorm.DB) error) error {
			if err := database.WithContext(ctx).Transaction(callback); err != nil {
				return err
			}
			return errSnapshotRestoreCommitUnknownForTest
		}

		journal, err := store.ApplyProviderSnapshotRestore(context.Background(), input)
		require.NoError(t, err)
		require.Equal(t, domainappdev.ProviderSnapshotRestorePhaseRestored, journal.Phase)
		key := snapshotRestoreSourceObjectKey(1001, project.ID, journal.ResultSourceVersion, "commit-visible")
		require.True(t, memoryStorageHasObject(objects, key))
		require.Equal(t, key, loadSnapshotRestoreProjectRecord(t, db, project.ID).SourceObjectKey)
		require.Zero(t, countSnapshotRestoreCleanupRows(t, db))
	})

	t.Run("confirmed rollback removes only its own object", func(t *testing.T) {
		store, db, objects, project, _, input := newSnapshotRestoreCommitFixture(t, "restore-confirmed-rollback")
		store.snapshotRestoreAttemptID = func() (string, error) { return "rolled-back", nil }
		store.snapshotRestoreTransaction = rollbackSnapshotRestoreTransaction

		journal, err := store.ApplyProviderSnapshotRestore(context.Background(), input)
		require.Nil(t, journal)
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreUnavailable)
		key := snapshotRestoreSourceObjectKey(1001, project.ID, input.ExpectedSourceVersion+1, "rolled-back")
		require.False(t, memoryStorageHasObject(objects, key))
		require.Equal(t, int64(1), countSnapshotRestoreCleanupRows(t, db))
		require.Equal(t, providerSnapshotCleanupStateDeleted, loadSnapshotCleanupRecord(t, db, key).CleanupState)
		require.Equal(t, domainappdev.ProviderSnapshotRestorePhasePending, domainappdev.ProviderSnapshotRestorePhase(loadSnapshotRestoreProjectRecord(t, db, project.ID).RestorePhase))
	})

	t.Run("authority read failure retains object for deferred cleanup", func(t *testing.T) {
		store, db, objects, project, _, input := newSnapshotRestoreCommitFixture(t, "restore-authority-unavailable")
		store.snapshotRestoreAttemptID = func() (string, error) { return "authority-unavailable", nil }
		store.snapshotRestoreTransaction = rollbackSnapshotRestoreTransaction
		readAuthority := store.snapshotRestoreReadProject
		store.snapshotRestoreReadProject = func(context.Context, int64, string) (*appDevProjectRecord, error) {
			return nil, errors.New("database read unavailable with internal dsn")
		}

		journal, err := store.ApplyProviderSnapshotRestore(context.Background(), input)
		require.Nil(t, journal)
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreUnavailable)
		key := snapshotRestoreSourceObjectKey(1001, project.ID, input.ExpectedSourceVersion+1, "authority-unavailable")
		require.True(t, memoryStorageHasObject(objects, key))
		require.Equal(t, int64(1), countSnapshotRestoreCleanupRows(t, db))

		store.snapshotRestoreReadProject = readAuthority
		require.NoError(t, db.Model(&providerSnapshotRestoreObjectCleanupRecord{}).Where("object_key = ?", key).
			Update("cleanup_after", time.Now().UTC().Add(-time.Minute)).Error)
		cleaned, cleanupErr := store.CleanupDeferredProviderSnapshotRestoreObjects(context.Background(), 10)
		require.NoError(t, cleanupErr)
		require.Equal(t, 1, cleaned)
		require.False(t, memoryStorageHasObject(objects, key))
		require.Equal(t, int64(1), countSnapshotRestoreCleanupRows(t, db))
		require.Equal(t, providerSnapshotCleanupStateDeleted, loadSnapshotCleanupRecord(t, db, key).CleanupState)
	})
}

func TestPersistentStoreProviderSnapshotRestoreRollbackCannotDeleteConcurrentWinner(t *testing.T) {
	storeA, db, objects, project, _, input := newSnapshotRestoreCommitFixture(t, "restore-rollback-winner")
	storeB := NewPersistentStoreForTest(db.Session(&gorm.Session{NewDB: true}), objects, t.TempDir())
	storeA.snapshotRestoreAttemptID = func() (string, error) { return "loser-attempt", nil }
	storeB.snapshotRestoreAttemptID = func() (string, error) { return "winner-attempt", nil }
	rolledBack := make(chan struct{})
	winnerCommitted := make(chan struct{})
	storeA.snapshotRestoreTransaction = func(ctx context.Context, database *gorm.DB, callback func(*gorm.DB) error) error {
		err := rollbackSnapshotRestoreTransaction(ctx, database, callback)
		close(rolledBack)
		<-winnerCommitted
		return err
	}

	var loserErr error
	var waiter sync.WaitGroup
	waiter.Add(1)
	go func() {
		defer waiter.Done()
		_, loserErr = storeA.ApplyProviderSnapshotRestore(context.Background(), input)
	}()
	<-rolledBack
	winner, winnerErr := storeB.ApplyProviderSnapshotRestore(context.Background(), input)
	require.NoError(t, winnerErr)
	close(winnerCommitted)
	waiter.Wait()
	require.Error(t, loserErr)

	winnerKey := snapshotRestoreSourceObjectKey(1001, project.ID, winner.ResultSourceVersion, "winner-attempt")
	loserKey := snapshotRestoreSourceObjectKey(1001, project.ID, winner.ResultSourceVersion, "loser-attempt")
	require.Equal(t, winnerKey, loadSnapshotRestoreProjectRecord(t, db, project.ID).SourceObjectKey)
	require.True(t, memoryStorageHasObject(objects, winnerKey))
	require.False(t, memoryStorageHasObject(objects, loserKey))
}

func TestPersistentStoreFailProviderSnapshotRestoreIsProductionFenced(t *testing.T) {
	t.Run("failed permits a new parent", func(t *testing.T) {
		store, _, project, first := newProviderSnapshotRestoreFixture(t, "failed-production-new-parent")
		failed := failProviderSnapshotRestoreThroughRepository(t, store, project, first, "failed-parent-first")
		require.Equal(t, domainappdev.ProviderSnapshotRestorePhaseFailed, failed.Phase)
		second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
		require.NoError(t, err)
		full, parent := providerSnapshotRestoreHashesForTest(t, project, second, "failed-parent-second")
		pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: full, ParentOperationHash: parent,
		})
		require.NoError(t, err)
		require.Equal(t, domainappdev.ProviderSnapshotRestorePhasePending, pending.Phase)
	})

	t.Run("failed rejects the same parent with another snapshot", func(t *testing.T) {
		store, _, project, first := newProviderSnapshotRestoreFixture(t, "failed-production-same-parent")
		failProviderSnapshotRestoreThroughRepository(t, store, project, first, "failed-parent-shared")
		second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
		require.NoError(t, err)
		full, parent := providerSnapshotRestoreHashesForTest(t, project, second, "failed-parent-shared")
		_, err = store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: full, ParentOperationHash: parent,
		})
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)
	})
}

func TestPersistentStoreProviderSnapshotRestoreConcurrentFailedReplacementUsesIndependentStores(t *testing.T) {
	storeA, db, project, first := newProviderSnapshotRestoreFixture(t, "failed-two-store-race")
	require.NoError(t, db.Exec("PRAGMA busy_timeout = 5000").Error)
	storeB := NewPersistentStoreForTest(db.Session(&gorm.Session{NewDB: true}), storeA.objects, t.TempDir())
	failProviderSnapshotRestoreThroughRepository(t, storeA, project, first, "failed-two-store-initial")
	second, err := storeA.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
	require.NoError(t, err)
	third, err := storeA.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "third")
	require.NoError(t, err)
	secondFull, secondParent := providerSnapshotRestoreHashesForTest(t, project, second, "failed-two-store-second")
	thirdFull, thirdParent := providerSnapshotRestoreHashesForTest(t, project, third, "failed-two-store-third")
	stores := []*PersistentStore{storeA, storeB}
	inputs := []domainappdev.ReserveProviderSnapshotRestoreInput{
		{SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: secondFull, ParentOperationHash: secondParent},
		{SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: third.ID, OperationHash: thirdFull, ParentOperationHash: thirdParent},
	}
	start := make(chan struct{})
	errs := make([]error, 2)
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(2)
	done.Add(2)
	for index := range stores {
		go func(index int) {
			defer done.Done()
			ready.Done()
			<-start
			_, errs[index] = stores[index].ReserveProviderSnapshotRestore(context.Background(), inputs[index])
		}(index)
	}
	ready.Wait()
	close(start)
	done.Wait()
	// SQLite may surface the losing concurrent writer as a transient table lock
	// rather than waiting for the winning transaction. Replaying that exact
	// reservation after both transactions finish must observe the active winner
	// as a conflict; it must never become a second success.
	for index, err := range errs {
		if errors.Is(err, domainappdev.ErrProviderSnapshotRestoreUnavailable) {
			_, errs[index] = stores[index].ReserveProviderSnapshotRestore(context.Background(), inputs[index])
		}
	}
	successes := 0
	conflicts := 0
	for _, err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, domainappdev.ErrProviderSnapshotRestoreConflict) {
			conflicts++
		}
	}
	require.Equal(t, 1, successes, fmt.Sprintf("reserve errors: %v", errs))
	require.Equal(t, 1, conflicts, fmt.Sprintf("reserve errors: %v", errs))
}

func TestProviderSnapshotRestoreSchemaContractsAreScopedAndExact(t *testing.T) {
	historical, err := os.ReadFile("../../../docker/atlas/migrations/20260710000100_appdev_persistence.sql")
	require.NoError(t, err)
	require.NotContains(t, string(historical), "restore_operation_hash")
	require.NotContains(t, string(historical), "appdev_source_object_cleanups")

	migration, err := os.ReadFile("../../../docker/atlas/migrations/20260717000100_appdev_snapshot_restore_journal.sql")
	require.NoError(t, err)
	require.Contains(t, string(migration), "ALTER TABLE `appdev_projects`")
	projectsSQL := string(migration)
	for _, definition := range []string{
		"`restore_operation_hash` binary(32) DEFAULT NULL",
		"`restore_parent_operation_hash` binary(32) DEFAULT NULL",
		"`restore_phase` varchar(24) NOT NULL DEFAULT 'none'",
		"`restore_updated_at` datetime(6) DEFAULT NULL",
	} {
		require.Contains(t, projectsSQL, definition)
	}
	cleanupSQL := mysqlTableBlock(t, string(migration), "appdev_source_object_cleanups")
	require.Contains(t, cleanupSQL, "`object_key` varchar(512) NOT NULL")
	require.Contains(t, cleanupSQL, "`restore_operation_hash` binary(32) NOT NULL")
	require.Contains(t, cleanupSQL, "`cleanup_after` datetime(6) NOT NULL")
	require.Contains(t, cleanupSQL, "PRIMARY KEY (`object_key`)")
	require.Contains(t, cleanupSQL, "KEY `idx_appdev_source_cleanup_due` (`cleanup_after`, `space_id`, `project_id`)")
	require.NotContains(t, cleanupSQL, "`cleanup_state`")

	claimMigration, err := os.ReadFile("../../../docker/atlas/migrations/20260717000200_appdev_source_cleanup_claims.sql")
	require.NoError(t, err)
	claimSQL := string(claimMigration)
	require.Contains(t, claimSQL, "ALTER TABLE `appdev_source_object_cleanups`")
	for _, definition := range []string{
		"`cleanup_state` varchar(16) NOT NULL DEFAULT 'pending'",
		"`claim_token_hash` binary(32) DEFAULT NULL",
		"`claim_expires_at` datetime(6) DEFAULT NULL",
		"`attempt_count` int unsigned NOT NULL DEFAULT 0",
		"`tombstone_expires_at` datetime(6) DEFAULT NULL",
		"`updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)",
		"KEY `idx_appdev_source_cleanup_claim` (`cleanup_state`, `cleanup_after`, `claim_expires_at`, `space_id`, `project_id`)",
		"KEY `idx_appdev_source_cleanup_tombstone` (`cleanup_state`, `tombstone_expires_at`)",
	} {
		require.Contains(t, claimSQL, definition)
	}

	hcl, err := os.ReadFile("../../../docker/atlas/opencoze_latest_schema.hcl")
	require.NoError(t, err)
	projectsHCL := hclNamedBlock(t, string(hcl), "table", "appdev_projects")
	require.Contains(t, hclNamedBlock(t, projectsHCL, "column", "restore_operation_hash"), "null = true")
	require.Contains(t, hclNamedBlock(t, projectsHCL, "column", "restore_operation_hash"), "type = binary(32)")
	require.Contains(t, hclNamedBlock(t, projectsHCL, "column", "restore_phase"), "default = \"none\"")
	cleanupHCL := hclNamedBlock(t, string(hcl), "table", "appdev_source_object_cleanups")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "column", "object_key"), "null = false")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "column", "object_key"), "type = varchar(512)")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "index", "idx_appdev_source_cleanup_due"), "column.cleanup_after")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "column", "cleanup_state"), "default = \"pending\"")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "column", "claim_token_hash"), "type = binary(32)")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "column", "claim_expires_at"), "type = datetime(6)")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "column", "attempt_count"), "unsigned = true")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "column", "tombstone_expires_at"), "type = datetime(6)")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "index", "idx_appdev_source_cleanup_claim"), "column.claim_expires_at")
	require.Contains(t, hclNamedBlock(t, cleanupHCL, "index", "idx_appdev_source_cleanup_tombstone"), "column.tombstone_expires_at")
}

func newSnapshotRestoreCommitFixture(t *testing.T, name string) (*PersistentStore, *gorm.DB, *memoryObjectStorage, *domainappdev.Project, *domainappdev.ProjectSnapshot, domainappdev.ApplyProviderSnapshotRestoreInput) {
	t.Helper()
	store, db, project, snapshot := newProviderSnapshotRestoreFixture(t, name)
	require.NoError(t, db.AutoMigrate(&providerSnapshotRestoreObjectCleanupRecord{}))
	objects, ok := store.objects.(*memoryObjectStorage)
	require.True(t, ok)
	full, parent := providerSnapshotRestoreHashesForTest(t, project, snapshot, "snapshot-commit-operation")
	pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
	})
	require.NoError(t, err)
	return store, db, objects, project, snapshot, domainappdev.ApplyProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full,
		ParentOperationHash: parent, ExpectedSourceVersion: pending.SourceVersion,
	}
}

func rollbackSnapshotRestoreTransaction(ctx context.Context, database *gorm.DB, callback func(*gorm.DB) error) error {
	return database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := callback(tx); err != nil {
			return err
		}
		return errSnapshotRestoreCommitUnknownForTest
	})
}

func failProviderSnapshotRestoreThroughRepository(t *testing.T, store *PersistentStore, project *domainappdev.Project, snapshot *domainappdev.ProjectSnapshot, operationID string) *domainappdev.ProviderSnapshotRestoreJournal {
	t.Helper()
	full, parent := providerSnapshotRestoreHashesForTest(t, project, snapshot, operationID)
	pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
	})
	require.NoError(t, err)
	failed, err := store.FailProviderSnapshotRestore(context.Background(), domainappdev.FailProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
		ExpectedSourceVersion: pending.SourceVersion, SafeErrorCode: "snapshot_invalid", SafeErrorMessage: "snapshot restore cannot continue",
	})
	require.NoError(t, err)
	return failed
}

func loadSnapshotRestoreProjectRecord(t *testing.T, db *gorm.DB, projectID string) *appDevProjectRecord {
	t.Helper()
	var record appDevProjectRecord
	require.NoError(t, db.Where("id = ?", projectID).Take(&record).Error)
	return &record
}

func countSnapshotRestoreCleanupRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&providerSnapshotRestoreObjectCleanupRecord{}).Count(&count).Error)
	return count
}

func memoryStorageHasObject(storage *memoryObjectStorage, key string) bool {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	_, ok := storage.objects[key]
	return ok
}

func mysqlTableBlock(t *testing.T, source, table string) string {
	t.Helper()
	start := strings.Index(source, "CREATE TABLE `"+table+"` (")
	require.NotEqual(t, -1, start, table)
	end := strings.Index(source[start:], ") ENGINE=")
	require.NotEqual(t, -1, end, table)
	return source[start : start+end]
}

func hclNamedBlock(t *testing.T, source, kind, name string) string {
	t.Helper()
	start := strings.Index(source, kind+" \""+name+"\" {")
	require.NotEqual(t, -1, start, kind+" "+name)
	depth := 0
	for index := start; index < len(source); index++ {
		switch source[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[start : index+1]
			}
		}
	}
	t.Fatalf("unterminated %s %s", kind, name)
	return ""
}
