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

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestOrdinaryLeaseRecoveryCreatesBareSourceTargetAtomically(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	source := seedOrdinaryLeaseRecoverySource(t, db, 10)
	checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
	req := ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100)

	result, err := repo.CreateRunBundle(context.Background(), req)

	require.NoError(t, err)
	require.True(t, result.Created)
	require.NotNil(t, result.Attempt)
	require.Equal(t, source.ID, result.Attempt.JournalRunID)
	require.Equal(t, uint32(1), result.Attempt.Ordinal)
	require.Nil(t, result.Attempt.SourceAttemptID)
	require.Equal(t, entity.JournalProjectionStateDisabled, result.Attempt.ProjectionState)
	require.False(t, result.Attempt.SnapshotsEnabled)
	storedSource, err := repo.GetRun(context.Background(), source.ID)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusInterrupted, storedSource.Status)
	require.Empty(t, storedSource.LeaseToken)
	events, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID: source.ID, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "run.interrupted", events[0].EventType)

	replayed, err := repo.CreateRunBundle(context.Background(), req)
	require.NoError(t, err)
	require.False(t, replayed.Created)
	require.Equal(t, result.Run.ID, replayed.Run.ID)
	require.Equal(t, result.Attempt.ID, replayed.Attempt.ID)
}

func TestOrdinaryLeaseRecoveryAttemptConflictRollsBackSourceAndTarget(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	source := seedOrdinaryLeaseRecoverySource(t, db, 10)
	checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 100, ThreadID: 999, JournalRunID: 999, ExecutionRunID: 999,
		AttemptID: "att_existing", Ordinal: 1, Status: string(entity.RunAttemptStatusPending),
		NextSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: string(entity.JournalProjectionStateHealthy), CreatedAt: 1, UpdatedAt: 1,
	}).Error)

	result, err := repo.CreateRunBundle(
		context.Background(), ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100),
	)

	require.Nil(t, result)
	require.Error(t, err)
	storedSource, err := repo.GetRun(context.Background(), source.ID)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusRunning, storedSource.Status)
	require.Equal(t, "lease-10", storedSource.LeaseToken)
	_, err = repo.GetRun(context.Background(), 20)
	require.Error(t, err)
	events, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID: source.ID, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, events)
}

func TestOrdinaryLeaseRecoveryRejectsSameKeyPartialRunWithoutAttempt(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	source := seedOrdinaryLeaseRecoverySource(t, db, 10)
	checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
	req := ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100)
	targetPO, err := runToPO(req.Run)
	require.NoError(t, err)
	require.NoError(t, db.Create(targetPO).Error)

	result, err := repo.CreateRunBundle(context.Background(), req)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrRunIdempotencyConflict)
	requireOrdinaryLeaseRecoverySourceUnchanged(t, repo, source.ID)
}

func TestOrdinaryLeaseRecoveryReplayIgnoresFreshCandidateEventID(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	source := seedOrdinaryLeaseRecoverySource(t, db, 10)
	checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
	req := ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100)
	created, err := repo.CreateRunBundle(context.Background(), req)
	require.NoError(t, err)
	require.True(t, created.Created)
	req.OrdinaryLeaseRecovery.ExpiredLease.Event.ID = 91

	replayed, err := repo.CreateRunBundle(context.Background(), req)

	require.NoError(t, err)
	require.False(t, replayed.Created)
	require.Equal(t, created.Run.ID, replayed.Run.ID)
}

func TestOrdinaryLeaseRecoveryReplayRejectsCommittedAggregateDrift(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drift func(t *testing.T, db *gorm.DB, req *CreateRunBundleRequest)
	}{
		{
			name: "source terminal payload",
			drift: func(t *testing.T, db *gorm.DB, _ *CreateRunBundleRequest) {
				t.Helper()
				require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 90).
					Update("payload", []byte(`{"status":"wrong"}`)).Error)
			},
		},
		{
			name: "target immutable config",
			drift: func(t *testing.T, _ *gorm.DB, req *CreateRunBundleRequest) {
				t.Helper()
				req.Run.Config = `{"drift":true}`
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			repo := NewThreadRepository(db)
			source := seedOrdinaryLeaseRecoverySource(t, db, 10)
			checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
			req := ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100)
			created, err := repo.CreateRunBundle(context.Background(), req)
			require.NoError(t, err)
			require.True(t, created.Created)
			tc.drift(t, db, &req)

			replayed, err := repo.CreateRunBundle(context.Background(), req)

			require.Nil(t, replayed)
			require.ErrorIs(t, err, ErrRunIdempotencyConflict)
		})
	}
}

func TestOrdinaryLeaseRecoveryActiveRunAdmissionRollsBackSource(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	source := seedOrdinaryLeaseRecoverySource(t, db, 10)
	checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
	other := newRepositoryTestRun(11, source.ThreadID, entity.RunStatusRunning, 2_000)
	other.SpaceID = source.SpaceID
	other.CreatorID = source.CreatorID
	other.RunKind = entity.RunKindTask
	otherPO, err := runToPO(other)
	require.NoError(t, err)
	require.NoError(t, db.Create(otherPO).Error)

	result, err := repo.CreateRunBundle(
		context.Background(), ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100),
	)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrActiveRunExists)
	requireOrdinaryLeaseRecoverySourceUnchanged(t, repo, source.ID)
	_, err = repo.GetRun(context.Background(), 20)
	require.Error(t, err)
}

func TestOrdinaryLeaseRecoveryRejectsNonEinoSourceCheckpointWithoutWrites(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	source := seedOrdinaryLeaseRecoverySource(t, db, 10)
	checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", checkpoint.ID).Updates(map[string]any{
		"checkpoint_ns":    "legacy",
		"runtime_type":     "legacy",
		"runtime_key":      "",
		"envelope_version": 0,
	}).Error)

	result, err := repo.CreateRunBundle(
		context.Background(), ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100),
	)

	require.Nil(t, result)
	require.Error(t, err)
	requireOrdinaryLeaseRecoverySourceUnchanged(t, repo, source.ID)
	_, err = repo.GetRun(context.Background(), 20)
	require.Error(t, err)
}

func TestOrdinaryLeaseRecoveryRejectsSourceCheckpointPayloadDriftWithoutWrites(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	source := seedOrdinaryLeaseRecoverySource(t, db, 10)
	checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
	req := ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", checkpoint.ID).Update(
		"channel_values",
		`{"messages":["drifted-after-validation"]}`,
	).Error)

	result, err := repo.CreateRunBundle(context.Background(), req)

	require.Nil(t, result)
	require.Error(t, err)
	requireOrdinaryLeaseRecoverySourceUnchanged(t, repo, source.ID)
	_, err = repo.GetRun(context.Background(), 20)
	require.Error(t, err)
}

func TestOrdinaryLeaseRecoveryRollsDisabledAttemptForward(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	source := seedOrdinaryLeaseRecoverySource(t, db, 10)
	checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
	root := newRepositoryTestRun(9, source.ThreadID, entity.RunStatusInterrupted, 500)
	root.SpaceID = source.SpaceID
	root.CreatorID = source.CreatorID
	root.RunKind = entity.RunKindTask
	root.EndedAt = 900
	rootPO, err := runToPO(root)
	require.NoError(t, err)
	require.NoError(t, db.Create(rootPO).Error)
	activeSlot := uint8(1)
	startedAt := int64(1_000)
	require.NoError(t, db.Create(runAttemptToPO(&entity.RunAttempt{
		ID: 101, ThreadID: source.ThreadID, JournalRunID: root.ID, ExecutionRunID: source.ID,
		AttemptID: "att_101", Ordinal: 2, Status: entity.RunAttemptStatusRunning,
		ActiveSlot: &activeSlot, NextSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: entity.JournalProjectionStateDisabled, StartedAt: &startedAt,
		CreatedAt: 1_000, UpdatedAt: 1_000,
	})).Error)
	req := ordinaryLeaseRecoveryBundleRequest(source, checkpoint, 20, 100)
	req.OrdinaryLeaseRecovery.JournalRunID = root.ID
	req.OrdinaryLeaseRecovery.SourceAttemptID = "att_101"
	req.Attempt.JournalRunID = root.ID
	req.Attempt.SourceAttemptID = stringPointer("att_101")
	req.Run.IdempotencyKey = "run-recovery:9:10:3"
	req.OrdinaryLeaseRecovery.IdempotencyKey = req.Run.IdempotencyKey
	req.Attempt.RecoveryIdempotencyKey = stringPointer(req.Run.IdempotencyKey)
	lockedRunIDs := make([]string, 0, 2)
	const callbackName = "ordinary_lease_recovery:observe_root_source_lock_order"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(
		callbackName,
		func(tx *gorm.DB) {
			if tx == nil || tx.Statement == nil || tx.Statement.Table != "agent_runs" ||
				!strings.Contains(tx.Statement.SQL.String(), "WHERE id = ?") || len(tx.Statement.Vars) == 0 {
				return
			}
			lockedRunIDs = append(lockedRunIDs, fmt.Sprint(tx.Statement.Vars[0]))
		},
	))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callbackName)) })

	result, err := repo.CreateRunBundle(context.Background(), req)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(lockedRunIDs), 2)
	require.Equal(t, []string{"9", "10"}, lockedRunIDs[:2])
	require.True(t, result.Created)
	require.Equal(t, root.ID, result.Attempt.JournalRunID)
	require.Equal(t, uint32(3), result.Attempt.Ordinal)
	require.Equal(t, "att_101", *result.Attempt.SourceAttemptID)
	var sourceAttempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 101).First(&sourceAttempt).Error)
	require.Equal(t, string(entity.RunAttemptStatusInterrupted), sourceAttempt.Status)
	require.Nil(t, sourceAttempt.ActiveSlot)
	require.NotNil(t, sourceAttempt.TerminalEventID)
	require.Equal(t, int64(90), *sourceAttempt.TerminalEventID)

	replayed, err := repo.CreateRunBundle(context.Background(), req)
	require.NoError(t, err)
	require.False(t, replayed.Created)
	require.Equal(t, uint32(3), replayed.Attempt.Ordinal)
}

func requireOrdinaryLeaseRecoverySourceUnchanged(
	t *testing.T,
	repo Repository,
	sourceRunID int64,
) {
	t.Helper()
	storedSource, err := repo.GetRun(context.Background(), sourceRunID)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusRunning, storedSource.Status)
	require.Equal(t, "lease-10", storedSource.LeaseToken)
	events, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID: sourceRunID, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, events)
}

func seedOrdinaryLeaseRecoverySource(t *testing.T, db *gorm.DB, runID int64) *entity.Run {
	t.Helper()
	seedJournalThread(t, db, 1)
	run := newRepositoryTestRun(runID, 1, entity.RunStatusRunning, 1_000)
	run.SpaceID = 10
	run.CreatorID = 20
	run.RunKind = entity.RunKindTask
	run.LeaseOwner = "worker-a"
	run.LeaseToken = "lease-10"
	run.LeaseExpiresAt = 2_000
	run.ExecutionGeneration = 3
	run.StartedAt = 1_000
	po, err := runToPO(run)
	require.NoError(t, err)
	require.NoError(t, db.Create(po).Error)
	return run
}

func seedOrdinaryLeaseRecoveryCheckpoint(
	t *testing.T,
	db *gorm.DB,
	checkpointID, runID int64,
) *entity.Checkpoint {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&checkpointPO{}))
	checkpoint := &entity.Checkpoint{
		ID: checkpointID, ThreadID: 1, RunID: runID,
		CheckpointNS: "eino.adk", RuntimeType: "eino_adk",
		RuntimeKey: "checkpoint-500", EnvelopeVersion: 1,
		ChannelValues: `{}`, ChannelVersions: `{}`, PendingSends: `[]`, Metadata: `{}`,
		CreatedAt: 1_500,
	}
	po, err := checkpointToPO(checkpoint)
	require.NoError(t, err)
	require.NoError(t, db.Create(po).Error)
	return checkpoint
}

func ordinaryLeaseRecoveryBundleRequest(
	source *entity.Run,
	checkpoint *entity.Checkpoint,
	targetRunID, targetAttemptID int64,
) CreateRunBundleRequest {
	recoveryKey := "run-recovery:10:3"
	target := newRepositoryTestRun(targetRunID, source.ThreadID, entity.RunStatusQueued, 3_000)
	target.SpaceID = source.SpaceID
	target.CreatorID = source.CreatorID
	target.RunKind = entity.RunKindTask
	target.IdempotencyKey = recoveryKey
	checkpointID := checkpoint.ID
	attempt := &entity.RunAttempt{
		ID: targetAttemptID, ThreadID: source.ThreadID,
		JournalRunID: source.ID, ExecutionRunID: target.ID,
		AttemptID: "att_100", Ordinal: 1, Status: entity.RunAttemptStatusPending,
		NextSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:    entity.JournalProjectionStateDisabled,
		SourceCheckpointID: &checkpointID, RecoveryIdempotencyKey: &recoveryKey,
		CreatedAt: 3_000, UpdatedAt: 3_000,
	}
	event := &entity.RunEvent{
		ID: 90, ThreadID: source.ThreadID, RunID: source.ID,
		EventType: "run.interrupted", Payload: `{"status":"interrupted"}`, CreatedAt: 3_000,
	}
	lease := &ReconcileExpiredRunLeaseRequest{
		RunID: source.ID, LeaseOwner: source.LeaseOwner, LeaseToken: source.LeaseToken,
		ExecutionGeneration: source.ExecutionGeneration, ToStatus: entity.RunStatusInterrupted,
		Now: 3_000, ErrorCode: "run_recovered", ErrorMessage: "recovered", Event: event,
	}
	return CreateRunBundleRequest{
		Run: target, Attempt: attempt,
		OrdinaryLeaseRecovery: &OrdinaryLeaseRecoveryRequest{
			JournalRunID: source.ID, SourceRunID: source.ID,
			SourceCheckpointID: checkpoint.ID, SourceCheckpoint: checkpoint,
			IdempotencyKey: recoveryKey,
			ExpiredLease:   lease,
		},
	}
}
