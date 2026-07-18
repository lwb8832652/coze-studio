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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type providerArchiveBarrierFixture struct {
	db         *gorm.DB
	objects    *memoryObjectStorage
	storeA     *PersistentStore
	storeB     *PersistentStore
	executions *ProviderExecutionRepository
	project    *domainappdev.Project
	snapshot   *domainappdev.ProjectSnapshot
	running    *domainappdev.ProviderExecution
	owner      domainappdev.ProviderExecutionOwnerHash
}

func newProviderArchiveBarrierFixture(t *testing.T, name string) *providerArchiveBarrierFixture {
	t.Helper()
	dsn := "file:" + name + "?mode=memory&cache=shared&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&appDevProjectRecord{},
		&appDevSnapshotRecord{},
		&appDevRuntimeRecord{},
		&providerSnapshotRestoreObjectCleanupRecord{},
	))
	require.NoError(t, migrateProviderExecutionSQLiteExecutionTestSchema(db))
	objects := newMemoryObjectStorage()
	storeA := NewPersistentStoreForTest(db, objects, t.TempDir())
	storeB := NewPersistentStoreForTest(db.Session(&gorm.Session{NewDB: true}), objects, t.TempDir())
	project, err := storeA.CreateProject(context.Background(), &domainappdev.Project{
		ID: "project-a", SpaceID: "1001", Name: "Archive barrier",
		Status: domainappdev.ProjectStatusReady, CreatorID: "7",
	}, map[string]string{"src/main.ts": "export const value = 1"})
	require.NoError(t, err)
	snapshot, err := storeA.CreateProjectSnapshot(
		context.Background(),
		project.SpaceID,
		project.ID,
		"before archive",
	)
	require.NoError(t, err)
	executions := NewProviderExecutionRepository(db)
	owner := testOwnerHash("archive-barrier-owner")
	now := time.Now().UTC()
	ownerExpiresAt := now.Add(time.Hour)
	record := &providerExecutionRecord{
		ID: "archive-running", SpaceID: 1001, ProjectID: project.ID, Generation: 1,
		IdempotencyKey: "archive-running-start",
		DesiredState:   domainappdev.ProviderExecutionDesiredRun,
		ObservedState:  domainappdev.ProviderExecutionObservedRunning,
		ProviderKey:    "provider-a", ProviderScope: "appdev",
		ProviderExecutionID: "provider-execution-archive",
		SubmissionStartedAt: &now,
		CheckpointEnvelope:  "",
		OwnerIdentityHash:   owner.Bytes(), OwnerEpoch: 1, OwnerExpiresAt: &ownerExpiresAt,
		ArtifactStatus: domainappdev.ProviderExecutionArtifactNone,
		Version:        4, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(record).Error)
	running, err := providerExecutionRecordToDomain(record)
	require.NoError(t, err)
	return &providerArchiveBarrierFixture{
		db: db, objects: objects, storeA: storeA, storeB: storeB,
		executions: executions, project: project, snapshot: snapshot,
		running: running, owner: owner,
	}
}

func (fixture *providerArchiveBarrierFixture) reserveInput() domainappdev.ReserveProjectArchiveInput {
	return domainappdev.ReserveProjectArchiveInput{
		SpaceID: fixture.project.SpaceID, ProjectID: fixture.project.ID,
		ExpectedSourceVersion: fixture.project.SourceVersion,
		RuntimeGeneration:     fixture.running.Generation,
	}
}

func TestProviderArchiveBarrierWinsBeforeRuntimeBuildRestoreAndSourceMutation(t *testing.T) {
	fixture := newProviderArchiveBarrierFixture(t, "provider-archive-barrier-wins")
	ctx := context.Background()

	intent, err := fixture.storeA.ReserveProjectArchive(ctx, fixture.reserveInput())
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProjectArchiveStateArchiving, intent.State)
	require.Equal(t, uint64(1), intent.IntentVersion)
	require.NotEmpty(t, intent.OperationID)

	retry, err := fixture.storeB.ReserveProjectArchive(ctx, fixture.reserveInput())
	require.NoError(t, err)
	require.Equal(t, intent.OperationID, retry.OperationID)
	require.Equal(t, intent.OperationHash, retry.OperationHash)
	require.Equal(t, intent.IntentVersion, retry.IntentVersion)

	_, err = fixture.executions.EnsureStart(ctx, providerExecutionStartInput(
		"archive-blocked-start",
		"archive-blocked-start-operation",
		time.Time{},
	))
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionStateConflict)

	buildInput := providerExecutionReserveBuildInput(t, fixture.running, fixture.owner, "archive-blocked-build")
	_, err = fixture.executions.ReserveBuild(ctx, buildInput)
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionStateConflict)

	restoreOperation, err := domainappdev.HashProviderSnapshotRestoreOperation(
		fixture.project.SpaceID,
		fixture.project.ID,
		fixture.snapshot.ID,
		"archive-blocked-restore",
	)
	require.NoError(t, err)
	restoreParent, err := domainappdev.HashProviderSnapshotRestoreParentOperation(
		fixture.project.SpaceID,
		fixture.project.ID,
		"archive-blocked-restore",
	)
	require.NoError(t, err)
	_, err = fixture.storeB.ReserveProviderSnapshotRestore(ctx, domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: fixture.project.SpaceID, ProjectID: fixture.project.ID,
		SnapshotID: fixture.snapshot.ID, OperationHash: restoreOperation,
		ParentOperationHash: restoreParent, RuntimeGeneration: fixture.running.Generation,
		RestartRequired: true,
	})
	require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)

	_, err = fixture.storeB.SaveFileContent(
		ctx,
		fixture.project.SpaceID,
		fixture.project.ID,
		"src/main.ts",
		"export const value = 2",
	)
	require.ErrorIs(t, err, domainappdev.ErrProjectArchiveConflict)
	authoritative, err := fixture.storeA.GetProject(ctx, fixture.project.SpaceID, fixture.project.ID)
	require.NoError(t, err)
	require.Equal(t, fixture.project.SourceVersion, authoritative.SourceVersion)
}

func TestProviderArchiveBarrierReserveAndCompleteCAS(t *testing.T) {
	fixture := newProviderArchiveBarrierFixture(t, "provider-archive-barrier-cas")
	ctx := context.Background()
	input := fixture.reserveInput()

	const workers = 16
	start := make(chan struct{})
	results := make(chan *domainappdev.ProjectArchiveIntent, workers)
	errs := make(chan error, workers)
	stores := []*PersistentStore{fixture.storeA, fixture.storeB}
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			intent, err := stores[index%len(stores)].ReserveProjectArchive(ctx, input)
			results <- intent
			errs <- err
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errs)

	var operationID string
	for err := range errs {
		require.NoError(t, err)
	}
	for intent := range results {
		require.NotNil(t, intent)
		if operationID == "" {
			operationID = intent.OperationID
		}
		require.Equal(t, operationID, intent.OperationID)
		require.Equal(t, uint64(1), intent.IntentVersion)
	}

	intent, err := fixture.storeA.LoadProjectArchive(ctx, domainappdev.LoadProjectArchiveInput{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID,
	})
	require.NoError(t, err)
	stale := domainappdev.CompleteProjectArchiveInput{Intent: *intent}
	staleIdentity, err := domainappdev.NewProjectArchiveIntentIdentity(
		intent.SpaceID,
		intent.ProjectID,
		intent.SourceVersion,
		intent.RuntimeGeneration,
		intent.IntentVersion+1,
	)
	require.NoError(t, err)
	stale.Intent.IntentVersion = staleIdentity.IntentVersion
	stale.Intent.OperationID = staleIdentity.OperationID
	stale.Intent.OperationHash = staleIdentity.OperationHash
	_, err = fixture.storeB.CompleteProjectArchive(ctx, stale)
	require.ErrorIs(t, err, domainappdev.ErrProjectArchiveConflict)

	_, err = fixture.storeB.CompleteProjectArchive(ctx, domainappdev.CompleteProjectArchiveInput{
		Intent: *intent,
	})
	require.ErrorIs(t, err, domainappdev.ErrProjectArchiveConflict, "active provider execution must be cleaned before archive completion")
}

func TestProviderArchiveBarrierRejectsArchiveWhileSnapshotRestoreIsActive(t *testing.T) {
	fixture := newProviderArchiveBarrierFixture(t, "provider-archive-restore-first")
	ctx := context.Background()
	operation, err := domainappdev.HashProviderSnapshotRestoreOperation(
		fixture.project.SpaceID, fixture.project.ID, fixture.snapshot.ID, "restore-first",
	)
	require.NoError(t, err)
	parent, err := domainappdev.HashProviderSnapshotRestoreParentOperation(
		fixture.project.SpaceID, fixture.project.ID, "restore-first",
	)
	require.NoError(t, err)
	_, err = fixture.storeA.ReserveProviderSnapshotRestore(ctx, domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: fixture.project.SpaceID, ProjectID: fixture.project.ID,
		SnapshotID: fixture.snapshot.ID, OperationHash: operation,
		ParentOperationHash: parent, RuntimeGeneration: fixture.running.Generation,
		RestartRequired: true,
	})
	require.NoError(t, err)

	_, err = fixture.storeB.ReserveProjectArchive(ctx, fixture.reserveInput())
	require.ErrorIs(t, err, domainappdev.ErrProjectArchiveConflict)
}

func TestProviderArchiveOperationHashBindsIdentityAndIntentVersion(t *testing.T) {
	base, err := domainappdev.NewProjectArchiveIntentIdentity("1001", "project-a", 3, 7, 2)
	require.NoError(t, err)
	for _, changed := range []domainappdev.ProjectArchiveIntentIdentity{
		{SpaceID: "1002", ProjectID: "project-a", SourceVersion: 3, RuntimeGeneration: 7, IntentVersion: 2},
		{SpaceID: "1001", ProjectID: "project-b", SourceVersion: 3, RuntimeGeneration: 7, IntentVersion: 2},
		{SpaceID: "1001", ProjectID: "project-a", SourceVersion: 4, RuntimeGeneration: 7, IntentVersion: 2},
		{SpaceID: "1001", ProjectID: "project-a", SourceVersion: 3, RuntimeGeneration: 8, IntentVersion: 2},
		{SpaceID: "1001", ProjectID: "project-a", SourceVersion: 3, RuntimeGeneration: 7, IntentVersion: 3},
	} {
		candidate, candidateErr := domainappdev.NewProjectArchiveIntentIdentity(
			changed.SpaceID,
			changed.ProjectID,
			changed.SourceVersion,
			changed.RuntimeGeneration,
			changed.IntentVersion,
		)
		require.NoError(t, candidateErr)
		require.NotEqual(t, base.OperationHash, candidate.OperationHash)
		require.NotEqual(t, base.OperationID, candidate.OperationID)
	}
	require.False(t, errors.Is(domainappdev.ErrProjectArchiveConflict, domainappdev.ErrProjectArchiveInvalid))
}
