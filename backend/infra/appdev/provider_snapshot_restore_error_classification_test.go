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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestPersistentStoreProviderSnapshotRestorePreservesContextAndAvailability(t *testing.T) {
	t.Run("fail preserves cancellation", func(t *testing.T) {
		store, _, project, snapshot := newProviderSnapshotRestoreFixture(t, "restore-fail-canceled")
		full, parent := providerSnapshotRestoreHashesForTest(t, project, snapshot, "restore-fail-canceled-operation")
		pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
		})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = store.FailProviderSnapshotRestore(ctx, domainappdev.FailProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
			ExpectedSourceVersion: pending.SourceVersion, SafeErrorCode: "restore_failed", SafeErrorMessage: "safe failure",
		})
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreInvalid)
	})

	t.Run("advance preserves deadline", func(t *testing.T) {
		store, _, project, snapshot := newProviderSnapshotRestoreFixture(t, "restore-advance-deadline")
		full, parent := providerSnapshotRestoreHashesForTest(t, project, snapshot, "restore-advance-deadline-operation")
		pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
			RestartRequired: true, RuntimeGeneration: 7,
		})
		require.NoError(t, err)
		restored, err := store.ApplyProviderSnapshotRestore(context.Background(), domainappdev.ApplyProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
			ExpectedSourceVersion: pending.SourceVersion,
		})
		require.NoError(t, err)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		_, err = store.MarkProviderSnapshotRestoreStarted(ctx, domainappdev.MarkProviderSnapshotRestoreStartedInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
			ExpectedSourceVersion: restored.ResultSourceVersion, StartedGeneration: 8,
		})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NotErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreInvalid)
	})

	t.Run("database failure is retryable unavailable", func(t *testing.T) {
		store, db, project, snapshot := newProviderSnapshotRestoreFixture(t, "restore-fail-db-unavailable")
		full, parent := providerSnapshotRestoreHashesForTest(t, project, snapshot, "restore-fail-db-operation")
		pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
		})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		_, err = store.FailProviderSnapshotRestore(context.Background(), domainappdev.FailProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
			ExpectedSourceVersion: pending.SourceVersion, SafeErrorCode: "restore_failed", SafeErrorMessage: "safe failure",
		})
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreUnavailable)
		require.NotErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreInvalid)
	})

	t.Run("advance database failure is retryable unavailable", func(t *testing.T) {
		store, db, project, snapshot := newProviderSnapshotRestoreFixture(t, "restore-advance-db-unavailable")
		full, parent := providerSnapshotRestoreHashesForTest(t, project, snapshot, "restore-advance-db-operation")
		pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
			RestartRequired: true, RuntimeGeneration: 7,
		})
		require.NoError(t, err)
		restored, err := store.ApplyProviderSnapshotRestore(context.Background(), domainappdev.ApplyProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
			ExpectedSourceVersion: pending.SourceVersion,
		})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		_, err = store.MarkProviderSnapshotRestoreStarted(context.Background(), domainappdev.MarkProviderSnapshotRestoreStartedInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
			ExpectedSourceVersion: restored.ResultSourceVersion, StartedGeneration: 8,
		})
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreUnavailable)
		require.NotErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreInvalid)
	})
}

func TestProviderAPIFacadeRealRepositoryUnavailableKeepsRestorePending(t *testing.T) {
	store, db, project, snapshot := newProviderSnapshotRestoreFixture(t, "restore-facade-db-unavailable")
	store.snapshotRestoreTransaction = func(context.Context, *gorm.DB, func(*gorm.DB) error) error {
		return errors.New("injected database failure")
	}
	facade := applicationappdev.NewProviderAPIFacade(providerSnapshotStoppedRuntime{}, nil, nil, store)
	_, err := facade.RestoreSnapshot(context.Background(), applicationappdev.ProviderSnapshotRestoreRequest{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID,
		OperationID: "restore-facade-db-operation", ActorID: "42",
	})
	require.ErrorIs(t, err, applicationappdev.ErrProviderControlUnavailable)
	require.NotErrorIs(t, err, applicationappdev.ErrProviderControlInvalid)
	record := loadSnapshotRestoreProjectRecord(t, db, project.ID)
	require.Equal(t, string(domainappdev.ProviderSnapshotRestorePhasePending), record.RestorePhase)
	require.Empty(t, record.RestoreSafeErrorCode)
}

type providerSnapshotStoppedRuntime struct{}

func (providerSnapshotStoppedRuntime) Start(context.Context, applicationappdev.ProviderRuntimeStartInput) (*applicationappdev.ProviderRuntimeProjection, error) {
	return nil, errors.New("unexpected start")
}

func (providerSnapshotStoppedRuntime) Status(context.Context, applicationappdev.ProviderRuntimeStatusInput) (*applicationappdev.ProviderRuntimeProjection, error) {
	return &applicationappdev.ProviderRuntimeProjection{State: applicationappdev.ProviderRuntimeStateStopped, CanStart: true}, nil
}

func (providerSnapshotStoppedRuntime) Stop(context.Context, applicationappdev.ProviderRuntimeStopInput) (*applicationappdev.ProviderRuntimeProjection, error) {
	return nil, errors.New("unexpected stop")
}

func (providerSnapshotStoppedRuntime) MatchCurrentStartOperation(context.Context, applicationappdev.ProviderRuntimeStartInput) (*applicationappdev.ProviderRuntimeProjection, bool, error) {
	return nil, false, nil
}
