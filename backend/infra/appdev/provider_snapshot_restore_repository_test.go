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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestPersistentStoreProviderSnapshotRestoreJournalFencesSourceAndOperation(t *testing.T) {
	store, db, project, snapshot := newProviderSnapshotRestoreFixture(t, "journal-fence")
	hash, err := domainappdev.HashProviderSnapshotRestoreOperation(project.SpaceID, project.ID, snapshot.ID, "snapshot-restore-parent-001")
	require.NoError(t, err)
	parentHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation(project.SpaceID, project.ID, "snapshot-restore-parent-001")
	require.NoError(t, err)
	reserve := domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash,
		RestartRequired: true, RuntimeGeneration: 7,
	}
	pending, err := store.ReserveProviderSnapshotRestore(context.Background(), reserve)
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderSnapshotRestorePhasePending, pending.Phase)
	require.Equal(t, int64(2), pending.SourceVersion)

	retry, err := store.ReserveProviderSnapshotRestore(context.Background(), reserve)
	require.NoError(t, err)
	require.Equal(t, pending.SourceVersion, retry.SourceVersion)
	require.True(t, pending.OperationHash.Equal(retry.OperationHash))

	otherHash, err := domainappdev.HashProviderSnapshotRestoreOperation(project.SpaceID, project.ID, snapshot.ID, "snapshot-restore-parent-002")
	require.NoError(t, err)
	otherParentHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation(project.SpaceID, project.ID, "snapshot-restore-parent-002")
	require.NoError(t, err)
	conflicting := reserve
	conflicting.OperationHash = otherHash
	conflicting.ParentOperationHash = otherParentHash
	_, err = store.ReserveProviderSnapshotRestore(context.Background(), conflicting)
	require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)

	restored, err := store.ApplyProviderSnapshotRestore(context.Background(), domainappdev.ApplyProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash, ExpectedSourceVersion: pending.SourceVersion,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderSnapshotRestorePhaseRestored, restored.Phase)
	require.Equal(t, int64(3), restored.ResultSourceVersion)

	restoredRetry, err := store.ApplyProviderSnapshotRestore(context.Background(), domainappdev.ApplyProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash, ExpectedSourceVersion: pending.SourceVersion,
	})
	require.NoError(t, err)
	require.Equal(t, restored.ResultSourceVersion, restoredRetry.ResultSourceVersion)
	var record appDevProjectRecord
	require.NoError(t, db.Where("id = ? AND space_id = ?", project.ID, 1001).Take(&record).Error)
	require.Equal(t, int64(3), record.SourceVersion)

	_, err = store.SaveFileContent(context.Background(), project.SpaceID, project.ID, "src/main.ts", "external change")
	require.NoError(t, err)
	_, err = store.ReserveProviderSnapshotRestore(context.Background(), reserve)
	require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)
}

func TestPersistentStoreProviderSnapshotRestoreJournalCompletesOnce(t *testing.T) {
	store, _, project, snapshot := newProviderSnapshotRestoreFixture(t, "journal-complete")
	hash, err := domainappdev.HashProviderSnapshotRestoreOperation(project.SpaceID, project.ID, snapshot.ID, "snapshot-complete-parent")
	require.NoError(t, err)
	parentHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation(project.SpaceID, project.ID, "snapshot-complete-parent")
	require.NoError(t, err)
	reserve := domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash,
		RestartRequired: true, RuntimeGeneration: 4,
	}
	pending, err := store.ReserveProviderSnapshotRestore(context.Background(), reserve)
	require.NoError(t, err)
	restored, err := store.ApplyProviderSnapshotRestore(context.Background(), domainappdev.ApplyProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash, ExpectedSourceVersion: pending.SourceVersion,
	})
	require.NoError(t, err)
	started, err := store.MarkProviderSnapshotRestoreStarted(context.Background(), domainappdev.MarkProviderSnapshotRestoreStartedInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash, ExpectedSourceVersion: restored.ResultSourceVersion, StartedGeneration: 5,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderSnapshotRestorePhaseStarted, started.Phase)
	completed, err := store.CompleteProviderSnapshotRestore(context.Background(), domainappdev.CompleteProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash, ExpectedSourceVersion: restored.ResultSourceVersion, StartedGeneration: 5,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderSnapshotRestorePhaseCompleted, completed.Phase)
	completedRetry, err := store.CompleteProviderSnapshotRestore(context.Background(), domainappdev.CompleteProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash, ExpectedSourceVersion: restored.ResultSourceVersion, StartedGeneration: 5,
	})
	require.NoError(t, err)
	require.Equal(t, completed.UpdatedAt, completedRetry.UpdatedAt)
	formatted := fmt.Sprintf("%v|%+v|%#v", completed, completed, completed)
	require.NotContains(t, formatted, fmt.Sprintf("%x", hash.Bytes()))
	_, err = json.Marshal(completed)
	require.Error(t, err)
}

func TestPersistentStoreProviderSnapshotRestoreWithoutRuntimeCompletesFromRestored(t *testing.T) {
	store, _, project, snapshot := newProviderSnapshotRestoreFixture(t, "journal-no-runtime")
	hash, err := domainappdev.HashProviderSnapshotRestoreOperation(project.SpaceID, project.ID, snapshot.ID, "snapshot-no-runtime-parent")
	require.NoError(t, err)
	parentHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation(project.SpaceID, project.ID, "snapshot-no-runtime-parent")
	require.NoError(t, err)
	reserve := domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash,
	}
	pending, err := store.ReserveProviderSnapshotRestore(context.Background(), reserve)
	require.NoError(t, err)
	restored, err := store.ApplyProviderSnapshotRestore(context.Background(), domainappdev.ApplyProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash, ExpectedSourceVersion: pending.SourceVersion,
	})
	require.NoError(t, err)
	completed, err := store.CompleteProviderSnapshotRestore(context.Background(), domainappdev.CompleteProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: hash, ParentOperationHash: parentHash, ExpectedSourceVersion: restored.ResultSourceVersion,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderSnapshotRestorePhaseCompleted, completed.Phase)
}

func TestPersistentStoreProviderSnapshotRestoreParentIdentityReplacementRules(t *testing.T) {
	t.Run("completed allows a new parent and snapshot", func(t *testing.T) {
		store, _, project, first := newProviderSnapshotRestoreFixture(t, "journal-new-parent")
		completeProviderSnapshotRestoreForTest(t, store, project, first, "snapshot-parent-first")
		second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
		require.NoError(t, err)
		full, parent := providerSnapshotRestoreHashesForTest(t, project, second, "snapshot-parent-second")
		_, err = store.LoadProviderSnapshotRestore(context.Background(), domainappdev.LoadProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: full, ParentOperationHash: parent,
		})
		require.ErrorIs(t, err, domainappdev.ErrNotFound)
		pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: full, ParentOperationHash: parent,
		})
		require.NoError(t, err)
		require.Equal(t, second.ID, pending.SnapshotID)
		require.True(t, parent.Equal(pending.ParentOperationHash))
		require.Equal(t, domainappdev.ProviderSnapshotRestorePhasePending, pending.Phase)
	})

	t.Run("completed rejects the same parent with another snapshot", func(t *testing.T) {
		store, _, project, first := newProviderSnapshotRestoreFixture(t, "journal-same-parent-new-snapshot")
		completeProviderSnapshotRestoreForTest(t, store, project, first, "snapshot-parent-shared")
		second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
		require.NoError(t, err)
		full, parent := providerSnapshotRestoreHashesForTest(t, project, second, "snapshot-parent-shared")
		load := domainappdev.LoadProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: full, ParentOperationHash: parent,
		}
		_, err = store.LoadProviderSnapshotRestore(context.Background(), load)
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)
		_, err = store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: load.SpaceID, ProjectID: load.ProjectID, SnapshotID: load.SnapshotID, OperationHash: load.OperationHash, ParentOperationHash: load.ParentOperationHash,
		})
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)
	})

	t.Run("active journal rejects a different parent", func(t *testing.T) {
		store, _, project, first := newProviderSnapshotRestoreFixture(t, "journal-active-parent")
		firstFull, firstParent := providerSnapshotRestoreHashesForTest(t, project, first, "snapshot-parent-active")
		_, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: first.ID, OperationHash: firstFull, ParentOperationHash: firstParent,
		})
		require.NoError(t, err)
		second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
		require.NoError(t, err)
		secondFull, secondParent := providerSnapshotRestoreHashesForTest(t, project, second, "snapshot-parent-other")
		_, err = store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: secondFull, ParentOperationHash: secondParent,
		})
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)
	})

	t.Run("failed allows a new parent and snapshot", func(t *testing.T) {
		store, db, project, first := newProviderSnapshotRestoreFixture(t, "journal-failed-new-parent")
		failProviderSnapshotRestoreForTest(t, store, db, project, first, "snapshot-parent-failed-first")
		second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
		require.NoError(t, err)
		full, parent := providerSnapshotRestoreHashesForTest(t, project, second, "snapshot-parent-failed-second")
		_, err = store.LoadProviderSnapshotRestore(context.Background(), domainappdev.LoadProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: full, ParentOperationHash: parent,
		})
		require.ErrorIs(t, err, domainappdev.ErrNotFound)
		pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: full, ParentOperationHash: parent,
		})
		require.NoError(t, err)
		require.Equal(t, second.ID, pending.SnapshotID)
		require.True(t, parent.Equal(pending.ParentOperationHash))
		require.Equal(t, domainappdev.ProviderSnapshotRestorePhasePending, pending.Phase)
	})

	t.Run("failed rejects the same parent with another snapshot", func(t *testing.T) {
		store, db, project, first := newProviderSnapshotRestoreFixture(t, "journal-failed-same-parent")
		failProviderSnapshotRestoreForTest(t, store, db, project, first, "snapshot-parent-failed-shared")
		second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
		require.NoError(t, err)
		full, parent := providerSnapshotRestoreHashesForTest(t, project, second, "snapshot-parent-failed-shared")
		load := domainappdev.LoadProviderSnapshotRestoreInput{
			SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: full, ParentOperationHash: parent,
		}
		_, err = store.LoadProviderSnapshotRestore(context.Background(), load)
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)
		_, err = store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: load.SpaceID, ProjectID: load.ProjectID, SnapshotID: load.SnapshotID, OperationHash: load.OperationHash, ParentOperationHash: load.ParentOperationHash,
		})
		require.ErrorIs(t, err, domainappdev.ErrProviderSnapshotRestoreConflict)
	})
}

func TestPersistentStoreProviderSnapshotRestoreConcurrentTerminalReplacementHasSingleWinner(t *testing.T) {
	store, _, project, first := newProviderSnapshotRestoreFixture(t, "journal-concurrent-parent")
	completeProviderSnapshotRestoreForTest(t, store, project, first, "snapshot-parent-completed")
	second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
	require.NoError(t, err)
	third, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "third")
	require.NoError(t, err)
	secondFull, secondParent := providerSnapshotRestoreHashesForTest(t, project, second, "snapshot-parent-second")
	thirdFull, thirdParent := providerSnapshotRestoreHashesForTest(t, project, third, "snapshot-parent-third")
	inputs := []domainappdev.ReserveProviderSnapshotRestoreInput{
		{SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: secondFull, ParentOperationHash: secondParent},
		{SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: third.ID, OperationHash: thirdFull, ParentOperationHash: thirdParent},
	}
	start := make(chan struct{})
	errorsByAttempt := make([]error, len(inputs))
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(len(inputs))
	done.Add(len(inputs))
	for index := range inputs {
		go func(index int) {
			defer done.Done()
			ready.Done()
			<-start
			_, errorsByAttempt[index] = store.ReserveProviderSnapshotRestore(context.Background(), inputs[index])
		}(index)
	}
	ready.Wait()
	close(start)
	done.Wait()
	successes := 0
	conflicts := 0
	for _, reserveErr := range errorsByAttempt {
		if reserveErr == nil {
			successes++
		} else if errors.Is(reserveErr, domainappdev.ErrProviderSnapshotRestoreConflict) {
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
}

func TestPersistentStoreProviderSnapshotRestoreConcurrentFailedReplacementHasSingleWinner(t *testing.T) {
	store, db, project, first := newProviderSnapshotRestoreFixture(t, "journal-concurrent-failed-parent")
	failProviderSnapshotRestoreForTest(t, store, db, project, first, "snapshot-parent-failed")
	second, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "second")
	require.NoError(t, err)
	third, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "third")
	require.NoError(t, err)
	secondFull, secondParent := providerSnapshotRestoreHashesForTest(t, project, second, "snapshot-parent-failed-second")
	thirdFull, thirdParent := providerSnapshotRestoreHashesForTest(t, project, third, "snapshot-parent-failed-third")
	inputs := []domainappdev.ReserveProviderSnapshotRestoreInput{
		{SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: second.ID, OperationHash: secondFull, ParentOperationHash: secondParent},
		{SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: third.ID, OperationHash: thirdFull, ParentOperationHash: thirdParent},
	}
	start := make(chan struct{})
	errorsByAttempt := make([]error, len(inputs))
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(len(inputs))
	done.Add(len(inputs))
	for index := range inputs {
		go func(index int) {
			defer done.Done()
			ready.Done()
			<-start
			_, errorsByAttempt[index] = store.ReserveProviderSnapshotRestore(context.Background(), inputs[index])
		}(index)
	}
	ready.Wait()
	close(start)
	done.Wait()

	winner := -1
	conflicts := 0
	for index, reserveErr := range errorsByAttempt {
		if reserveErr == nil {
			require.Equal(t, -1, winner, "two failed-terminal replacements succeeded")
			winner = index
			continue
		}
		require.ErrorIs(t, reserveErr, domainappdev.ErrProviderSnapshotRestoreConflict)
		conflicts++
	}
	require.NotEqual(t, -1, winner)
	require.Equal(t, 1, conflicts)

	winnerInput := inputs[winner]
	active, err := store.LoadProviderSnapshotRestore(context.Background(), domainappdev.LoadProviderSnapshotRestoreInput{
		SpaceID: winnerInput.SpaceID, ProjectID: winnerInput.ProjectID, SnapshotID: winnerInput.SnapshotID,
		OperationHash: winnerInput.OperationHash, ParentOperationHash: winnerInput.ParentOperationHash,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderSnapshotRestorePhasePending, active.Phase)
	require.Equal(t, winnerInput.SnapshotID, active.SnapshotID)
	require.True(t, winnerInput.ParentOperationHash.Equal(active.ParentOperationHash))
}

func TestProviderExecutionArtifactTimestampSurvivesStopAndCleanupUpdates(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	publishing, ownerHash, operationHash, descriptor := providerExecutionPublishingRecord(t, repository)
	ready, err := repository.CompleteArtifactPublish(context.Background(), domainappdev.CompleteProviderArtifactPublishInput{
		OwnerCAS: providerExecutionCAS(publishing, ownerHash, time.Time{}), ProviderExecutionID: publishing.ProviderExecutionID,
		OperationHash: operationHash, ObjectKey: "appdev/builds/internal/stable-time.zip", Digest: descriptor.Digest, Size: descriptor.Size,
	})
	require.NoError(t, err)
	require.NotNil(t, ready.ArtifactUpdatedAt)
	artifactUpdatedAt := *ready.ArtifactUpdatedAt

	stopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(ready, ownerHash, time.Time{}),
	})
	require.NoError(t, err)
	require.NotNil(t, stopping.ArtifactUpdatedAt)
	require.True(t, stopping.ArtifactUpdatedAt.Equal(artifactUpdatedAt))
	cleanup, err := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(stopping, ownerHash, time.Time{}),
	})
	require.NoError(t, err)
	require.NotNil(t, cleanup.ArtifactUpdatedAt)
	require.True(t, cleanup.ArtifactUpdatedAt.Equal(artifactUpdatedAt))
	loaded, err := repository.LoadLatestReadyArtifact(context.Background(), domainappdev.LoadReadyProviderArtifactInput{SpaceID: "1001", ProjectID: "project-a"})
	require.NoError(t, err)
	require.True(t, loaded.ArtifactUpdatedAt.Equal(artifactUpdatedAt))
	require.True(t, loaded.UpdatedAt.After(artifactUpdatedAt) || loaded.UpdatedAt.Equal(artifactUpdatedAt))
}

func TestProviderSnapshotRestoreModelMigrationAndHCLStayAligned(t *testing.T) {
	model := reflect.TypeOf(appDevProjectRecord{})
	columns := []string{
		"restore_operation_hash", "restore_parent_operation_hash", "restore_snapshot_id", "restore_phase", "restore_runtime_generation",
		"restore_restart_required", "restore_source_version", "restore_result_source_version",
		"restore_started_generation", "restore_safe_error_code", "restore_safe_error_message", "restore_updated_at",
	}
	for _, column := range columns {
		found := false
		for index := 0; index < model.NumField(); index++ {
			if strings.Contains(model.Field(index).Tag.Get("gorm"), "column:"+column) {
				found = true
				break
			}
		}
		require.Truef(t, found, "GORM model missing %s", column)
	}
	for _, file := range []string{
		"../../../docker/atlas/migrations/20260717000100_appdev_snapshot_restore_journal.sql",
		"../../../docker/atlas/opencoze_latest_schema.hcl",
	} {
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, column := range columns {
			require.Containsf(t, string(content), column, "%s missing %s", file, column)
		}
	}
}

func providerSnapshotRestoreHashesForTest(t *testing.T, project *domainappdev.Project, snapshot *domainappdev.ProjectSnapshot, operationID string) (domainappdev.ProviderSnapshotRestoreOperationHash, domainappdev.ProviderSnapshotRestoreParentOperationHash) {
	t.Helper()
	full, err := domainappdev.HashProviderSnapshotRestoreOperation(project.SpaceID, project.ID, snapshot.ID, operationID)
	require.NoError(t, err)
	parent, err := domainappdev.HashProviderSnapshotRestoreParentOperation(project.SpaceID, project.ID, operationID)
	require.NoError(t, err)
	return full, parent
}

func completeProviderSnapshotRestoreForTest(t *testing.T, store *PersistentStore, project *domainappdev.Project, snapshot *domainappdev.ProjectSnapshot, operationID string) *domainappdev.ProviderSnapshotRestoreJournal {
	t.Helper()
	full, parent := providerSnapshotRestoreHashesForTest(t, project, snapshot, operationID)
	pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
	})
	require.NoError(t, err)
	restored, err := store.ApplyProviderSnapshotRestore(context.Background(), domainappdev.ApplyProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent, ExpectedSourceVersion: pending.SourceVersion,
	})
	require.NoError(t, err)
	completed, err := store.CompleteProviderSnapshotRestore(context.Background(), domainappdev.CompleteProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent, ExpectedSourceVersion: restored.ResultSourceVersion,
	})
	require.NoError(t, err)
	return completed
}

func failProviderSnapshotRestoreForTest(t *testing.T, store *PersistentStore, _ *gorm.DB, project *domainappdev.Project, snapshot *domainappdev.ProjectSnapshot, operationID string) {
	t.Helper()
	full, parent := providerSnapshotRestoreHashesForTest(t, project, snapshot, operationID)
	pending, err := store.ReserveProviderSnapshotRestore(context.Background(), domainappdev.ReserveProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
	})
	require.NoError(t, err)
	failed, err := store.FailProviderSnapshotRestore(context.Background(), domainappdev.FailProviderSnapshotRestoreInput{
		SpaceID: project.SpaceID, ProjectID: project.ID, SnapshotID: snapshot.ID, OperationHash: full, ParentOperationHash: parent,
		ExpectedSourceVersion: pending.SourceVersion, SafeErrorCode: "restore_failed", SafeErrorMessage: "safe restore failure",
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderSnapshotRestorePhaseFailed, failed.Phase)
}

func newProviderSnapshotRestoreFixture(t *testing.T, name string) (*PersistentStore, *gorm.DB, *domainappdev.Project, *domainappdev.ProjectSnapshot) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&appDevProjectRecord{}, &appDevSnapshotRecord{}, &providerSnapshotRestoreObjectCleanupRecord{}))
	store := NewPersistentStoreForTest(db, newMemoryObjectStorage(), t.TempDir())
	project := &domainappdev.Project{ID: "project-a", SpaceID: "1001", Name: "Journal", Status: domainappdev.ProjectStatusReady, CreatorID: "42"}
	_, err = store.CreateProject(context.Background(), project, map[string]string{"src/main.ts": "snapshot source"})
	require.NoError(t, err)
	snapshot, err := store.CreateProjectSnapshot(context.Background(), project.SpaceID, project.ID, "snapshot")
	require.NoError(t, err)
	_, err = store.SaveFileContent(context.Background(), project.SpaceID, project.ID, "src/main.ts", "current source")
	require.NoError(t, err)
	return store, db, project, snapshot
}

var _ = errors.Is
