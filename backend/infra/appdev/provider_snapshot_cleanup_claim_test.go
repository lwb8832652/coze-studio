// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestProviderSnapshotCleanupClaimWinsBeforeRestoreCommit(t *testing.T) {
	storeA, db, objects, project, _, input := newSnapshotRestoreCommitFixture(t, "cleanup-claim-wins")
	storeB := NewPersistentStoreForTest(db.Session(&gorm.Session{NewDB: true}), objects, t.TempDir())
	storeA.snapshotRestoreAttemptID = func() (string, error) { return "claim-race", nil }
	restoreReady := make(chan struct{})
	continueRestore := make(chan struct{})
	storeA.snapshotRestoreBeforeCommit = func(*providerSnapshotRestoreObjectCleanupRecord) {
		close(restoreReady)
		<-continueRestore
	}
	claimReady := make(chan struct{})
	continueCleanup := make(chan struct{})
	storeB.snapshotCleanupAfterClaim = func(*providerSnapshotRestoreObjectCleanupRecord) {
		close(claimReady)
		<-continueCleanup
	}

	var restoreErr error
	var restoreDone sync.WaitGroup
	restoreDone.Add(1)
	go func() {
		defer restoreDone.Done()
		_, restoreErr = storeA.ApplyProviderSnapshotRestore(context.Background(), input)
	}()
	<-restoreReady
	key := snapshotRestoreSourceObjectKey(1001, project.ID, input.ExpectedSourceVersion+1, "claim-race")
	require.NoError(t, db.Model(&providerSnapshotRestoreObjectCleanupRecord{}).Where("object_key = ?", key).
		Update("cleanup_after", time.Now().UTC().Add(-time.Minute)).Error)
	cleanupResult := make(chan error, 1)
	go func() {
		_, err := storeB.CleanupDeferredProviderSnapshotRestoreObjects(context.Background(), 10)
		cleanupResult <- err
	}()
	<-claimReady
	close(continueRestore)
	restoreDone.Wait()
	require.ErrorIs(t, restoreErr, domainappdev.ErrProviderSnapshotRestoreConflict)
	close(continueCleanup)
	require.NoError(t, <-cleanupResult)

	require.False(t, memoryStorageHasObject(objects, key))
	require.Equal(t, providerSnapshotCleanupStateDeleted, loadSnapshotCleanupRecord(t, db, key).CleanupState)
	require.NotEqual(t, key, loadSnapshotRestoreProjectRecord(t, db, project.ID).SourceObjectKey)
}

func TestProviderSnapshotCleanupClaimLeaseCrashRecovery(t *testing.T) {
	store, db, objects, project, _, input := newSnapshotRestoreCommitFixture(t, "cleanup-claim-crash")
	key := seedProviderSnapshotCleanup(t, store, db, objects, project, input, "crashed-claim")
	now := time.Now().UTC()
	require.NoError(t, db.Model(&providerSnapshotRestoreObjectCleanupRecord{}).Where("object_key = ?", key).
		Updates(map[string]any{
			"cleanup_state":    providerSnapshotCleanupStateClaimed,
			"claim_token_hash": make([]byte, 32),
			"claim_expires_at": now.Add(time.Minute),
		}).Error)

	cleaned, err := store.CleanupDeferredProviderSnapshotRestoreObjects(context.Background(), 10)
	require.NoError(t, err)
	require.Zero(t, cleaned)
	require.True(t, memoryStorageHasObject(objects, key))

	require.NoError(t, db.Model(&providerSnapshotRestoreObjectCleanupRecord{}).Where("object_key = ?", key).
		Updates(map[string]any{"claim_expires_at": now.Add(-time.Minute), "cleanup_after": now.Add(-time.Minute)}).Error)
	cleaned, err = store.CleanupDeferredProviderSnapshotRestoreObjects(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, cleaned)
	require.False(t, memoryStorageHasObject(objects, key))
	require.Equal(t, providerSnapshotCleanupStateDeleted, loadSnapshotCleanupRecord(t, db, key).CleanupState)
}

func TestProviderSnapshotCleanupPoisonRowDoesNotStarveBatch(t *testing.T) {
	store, db, objects, project, _, input := newSnapshotRestoreCommitFixture(t, "cleanup-poison-fairness")
	first := seedProviderSnapshotCleanup(t, store, db, objects, project, input, "poison-first")
	second := seedProviderSnapshotCleanup(t, store, db, objects, project, input, "healthy-second")
	store.snapshotCleanupDeleteObject = func(ctx context.Context, key string) error {
		if key == first {
			return errors.New("storage delete unavailable")
		}
		return objects.DeleteObject(ctx, key)
	}

	cleaned, err := store.CleanupDeferredProviderSnapshotRestoreObjects(context.Background(), 10)
	require.Error(t, err)
	require.Equal(t, 1, cleaned)
	require.True(t, memoryStorageHasObject(objects, first))
	require.False(t, memoryStorageHasObject(objects, second))
	require.Equal(t, providerSnapshotCleanupStatePending, loadSnapshotCleanupRecord(t, db, first).CleanupState)
	require.Equal(t, providerSnapshotCleanupStateDeleted, loadSnapshotCleanupRecord(t, db, second).CleanupState)
}

func seedProviderSnapshotCleanup(
	t *testing.T,
	store *PersistentStore,
	db *gorm.DB,
	objects *memoryObjectStorage,
	project *domainappdev.Project,
	input domainappdev.ApplyProviderSnapshotRestoreInput,
	attempt string,
) string {
	t.Helper()
	key := snapshotRestoreSourceObjectKey(1001, project.ID, input.ExpectedSourceVersion+1, attempt)
	require.NoError(t, objects.PutObject(context.Background(), key, []byte("cleanup payload")))
	now := time.Now().UTC()
	require.NoError(t, db.Create(&providerSnapshotRestoreObjectCleanupRecord{
		ObjectKey: key, SpaceID: 1001, ProjectID: project.ID,
		RestoreOperationHash: input.OperationHash.Bytes(), SourceVersion: input.ExpectedSourceVersion + 1,
		CleanupState: providerSnapshotCleanupStatePending,
		CleanupAfter: providerSnapshotRestoreTime{Time: now.Add(-time.Minute), Valid: true},
		CreatedAt:    providerSnapshotRestoreTime{Time: now.Add(-time.Minute), Valid: true},
		UpdatedAt:    providerSnapshotRestoreTime{Time: now.Add(-time.Minute), Valid: true},
	}).Error)
	return key
}

func loadSnapshotCleanupRecord(t *testing.T, db *gorm.DB, key string) *providerSnapshotRestoreObjectCleanupRecord {
	t.Helper()
	var record providerSnapshotRestoreObjectCleanupRecord
	require.NoError(t, db.Where("object_key = ?", key).Take(&record).Error)
	return &record
}
