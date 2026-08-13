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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLegacyPlanItemMutationsLockPlanFirstForUpdate(t *testing.T) {
	expectedErr := errors.New("stop after plan lock")
	const lockedPlanQuery = "SELECT \\* FROM `agent_run_plans` WHERE run_id = \\? ORDER BY `agent_run_plans`.`run_id` LIMIT \\? FOR UPDATE"

	tests := []struct {
		name string
		call func(*threadRepository) error
	}{
		{
			name: "upsert",
			call: func(repo *threadRepository) error {
				_, _, _, _, err := repo.UpsertPlanItem(context.Background(), &entity.AgentRunPlanItem{
					ID: 100, RunID: 20, TaskID: 1,
					Subject: "upsert", Status: entity.AgentRunPlanItemStatusPending,
					Blocks: `[]`, BlockedBy: `[]`, Metadata: `{}`,
					Active: true, Version: 1, CreatedAt: 100, UpdatedAt: 100,
				})
				return err
			},
		},
		{
			name: "archive",
			call: func(repo *threadRepository) error {
				_, _, _, err := repo.ArchivePlanItem(context.Background(), 20, 1, 100)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := canonicalMySQLMockRepository(t)
			mock.ExpectBegin()
			mock.ExpectQuery(lockedPlanQuery).
				WithArgs(int64(20), 1).
				WillReturnError(expectedErr)
			mock.ExpectRollback()

			require.ErrorIs(t, tt.call(repo), expectedErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAdaptiveExecutionBoundaryMySQL(t *testing.T) {
	t.Run("ConcurrentPlanItemMutationHasSingleWinner", func(t *testing.T) {
		runAdaptiveExecutionConcurrentPlanItemMutation(t)
	})

	t.Run("ConcurrentAdaptiveAndLegacyUpsertAreLinearizable", func(t *testing.T) {
		runAdaptiveAndLegacyMySQLRace(t, adaptiveExecutionLegacyMutationUpsert)
	})

	t.Run("ConcurrentAdaptiveAndLegacyArchiveAreLinearizable", func(t *testing.T) {
		runAdaptiveAndLegacyMySQLRace(t, adaptiveExecutionLegacyMutationArchive)
	})
}

func TestAdaptiveExecutionBootstrapMySQLIntegrationRollingPlanBoundary(t *testing.T) {
	db, repoA, repoB := adaptiveExecutionMySQLIntegrationRepositories(t)
	seedAdaptiveExecutionRollingPlanMySQLState(t, db)
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &agentRunPlanPO{}, "run_id = ?", 20))
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &agentRunPlanItemPO{}, "run_id = ?", 20))

	b1Request := adaptiveExecutionRollingPlanMySQLRequest(
		"b1", 7101, 8101, 0, 0, 1_000,
		AdaptivePlanItemMutation{ExpectedVersion: 0, NextItem: &entity.AgentRunPlanItem{
			ID: 60, RunID: 20, TaskID: 1,
			Subject: "first task", Description: "created by B1",
			Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
			Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"boundary":"b1"}`,
			Active: true, Version: 1,
		}},
	)
	b1, err := repoA.CommitAdaptiveExecutionBoundary(context.Background(), b1Request)
	require.NoError(t, err)
	assertAdaptiveExecutionRollingPlanMySQLBoundary(t, b1, 1, 0, 1, 1, 1, []int64{1})
	require.False(t, b1.Replayed)

	b2Request := adaptiveExecutionRollingPlanMySQLRequest(
		"b2", 7102, 8102, b1.Checkpoint.ID, 1, 1_100,
		AdaptivePlanItemMutation{ExpectedVersion: 0, NextItem: &entity.AgentRunPlanItem{
			ID: 61, RunID: 20, TaskID: 2,
			Subject: "second task", Description: "created by B2",
			Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "queued", Owner: "agent",
			Blocks: `[]`, BlockedBy: `[1]`, Metadata: `{"boundary":"b2"}`,
			Active: true, Version: 1,
		}},
	)
	b2, err := repoA.CommitAdaptiveExecutionBoundary(context.Background(), b2Request)
	require.NoError(t, err)
	assertAdaptiveExecutionRollingPlanMySQLBoundary(t, b2, 2, b1.Checkpoint.ID, 2, 2, 1, []int64{2})
	require.False(t, b2.Replayed)

	b3Request := adaptiveExecutionRollingPlanMySQLRequest(
		"b3", 7103, 8103, b2.Checkpoint.ID, 2, 1_200,
		AdaptivePlanItemMutation{ExpectedVersion: 1, NextItem: &entity.AgentRunPlanItem{
			ID: 60, RunID: 20, TaskID: 1,
			Subject: "first task", Description: "updated by B3",
			Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "executing", Owner: "agent",
			Blocks: `[2]`, BlockedBy: `[]`, Metadata: `{"boundary":"b3"}`,
			Active: true, Version: 2,
		}},
	)
	b3, err := repoA.CommitAdaptiveExecutionBoundary(context.Background(), b3Request)
	require.NoError(t, err)
	assertAdaptiveExecutionRollingPlanMySQLBoundary(t, b3, 3, b2.Checkpoint.ID, 3, 2, 2, []int64{1})
	require.False(t, b3.Replayed)
	require.Equal(t, "updated by B3", b3.Items[0].Description)

	stateBeforeReplay := readAdaptiveExecutionRollingPlanMySQLState(t, db)
	require.Equal(t, uint64(4), stateBeforeReplay.attempt.NextSequence)
	require.Equal(t, uint64(3), stateBeforeReplay.attempt.LastCommittedSequence)
	require.Equal(t, int64(3), stateBeforeReplay.plan.Revision)
	require.Equal(t, int64(2), stateBeforeReplay.plan.HighWatermark)
	require.Len(t, stateBeforeReplay.items, 2)
	require.Equal(t, int64(1), stateBeforeReplay.items[0].TaskID)
	require.Equal(t, int64(2), stateBeforeReplay.items[0].Version)
	require.Equal(t, "updated by B3", stateBeforeReplay.items[0].Description)
	require.Equal(t, int64(2), stateBeforeReplay.items[1].TaskID)
	require.Equal(t, int64(1), stateBeforeReplay.items[1].Version)
	require.Len(t, stateBeforeReplay.events, 3)
	require.Len(t, stateBeforeReplay.checkpoints, 3)
	replayedB1, err := repoB.CommitAdaptiveExecutionBoundary(context.Background(), b1Request)
	require.NoError(t, err)
	require.True(t, replayedB1.Replayed)
	assertAdaptiveExecutionRollingPlanMySQLBoundary(t, replayedB1, 1, 0, 1, 1, 1, []int64{1})
	require.Equal(t, stateBeforeReplay, readAdaptiveExecutionRollingPlanMySQLState(t, db))

	staleRequests := map[string]CommitAdaptiveExecutionBoundaryRequest{
		"writer-a": adaptiveExecutionRollingPlanMySQLRequest(
			"stale-a", 7104, 8104, b3.Checkpoint.ID, 3, 1_300,
			AdaptivePlanItemMutation{ExpectedVersion: 2, NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1,
				Subject: "first task", Description: "writer A",
				Status: entity.AgentRunPlanItemStatusCompleted, ActiveForm: "done", Owner: "agent",
				Blocks: `[2]`, BlockedBy: `[]`, Metadata: `{"writer":"a"}`,
				Active: true, Version: 3,
			}},
		),
		"writer-b": adaptiveExecutionRollingPlanMySQLRequest(
			"stale-b", 7105, 8105, b3.Checkpoint.ID, 3, 1_300,
			AdaptivePlanItemMutation{ExpectedVersion: 2, NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1,
				Subject: "first task", Description: "writer B",
				Status: entity.AgentRunPlanItemStatusDeleted, ActiveForm: "failed", Owner: "agent",
				Blocks: `[2]`, BlockedBy: `[]`, Metadata: `{"writer":"b"}`,
				Active: true, Version: 3,
			}},
		),
	}
	raceResults := runAdaptiveExecutionMySQLRace(t, []adaptiveExecutionMySQLRaceCall{
		{name: "writer-a", call: func(ctx context.Context) (any, error) {
			return repoA.CommitAdaptiveExecutionBoundary(ctx, staleRequests["writer-a"])
		}},
		{name: "writer-b", call: func(ctx context.Context) (any, error) {
			return repoB.CommitAdaptiveExecutionBoundary(ctx, staleRequests["writer-b"])
		}},
	})
	winners := 0
	losers := 0
	for _, raceResult := range raceResults {
		requireAdaptiveExecutionMySQLRaceError(t, raceResult.err)
		if raceResult.err == nil {
			winners++
			boundary, ok := raceResult.value.(*CommitAdaptiveExecutionBoundaryResult)
			require.True(t, ok)
			assertAdaptiveExecutionRollingPlanMySQLBoundary(t, boundary, 4, b3.Checkpoint.ID, 4, 2, 3, []int64{1})
			continue
		}
		losers++
		require.True(t,
			errors.Is(raceResult.err, ErrAdaptiveExecutionPlanRevisionConflict) ||
				errors.Is(raceResult.err, ErrAdaptiveExecutionLineageConflict) ||
				errors.Is(raceResult.err, ErrAdaptiveExecutionSequenceConflict),
			"expected a typed stale-writer conflict, got %v", raceResult.err,
		)
		require.Nil(t, raceResult.value)
		require.Zero(t, adaptiveExecutionMySQLRowCount(
			t, db, &runEventPO{}, "id = ?", staleRequests[raceResult.name].Event.ID,
		))
		require.Zero(t, adaptiveExecutionMySQLRowCount(
			t, db, &checkpointPO{}, "id = ?", staleRequests[raceResult.name].Checkpoint.ID,
		))
	}
	require.Equal(t, 1, winners)
	require.Equal(t, 1, losers)
	finalState := readAdaptiveExecutionRollingPlanMySQLState(t, db)
	require.Equal(t, uint64(5), finalState.attempt.NextSequence)
	require.Equal(t, uint64(4), finalState.attempt.LastCommittedSequence)
	require.Equal(t, int64(4), finalState.plan.Revision)
	require.Equal(t, int64(2), finalState.plan.HighWatermark)
	require.Len(t, finalState.items, 2)
	require.Equal(t, int64(3), finalState.items[0].Version)
	require.Equal(t, int64(1), finalState.items[1].Version)
	require.Len(t, finalState.events, 4)
	require.Len(t, finalState.checkpoints, 4)
}

type adaptiveExecutionRollingPlanMySQLState struct {
	attempt     runAttemptPO
	plan        agentRunPlanPO
	items       []agentRunPlanItemPO
	events      []runEventPO
	checkpoints []checkpointPO
}

func seedAdaptiveExecutionRollingPlanMySQLState(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&threadPO{
		ID: 10, SpaceID: 10, CreatorID: 20, Title: "rolling plan",
		Status: string(entity.ThreadStatusRunning), Source: string(entity.ThreadSourceWeb),
		Metadata: datatypes.JSON([]byte(`{}`)), CreatedAt: 600, UpdatedAt: 600,
	}).Error)
	require.NoError(t, db.Create(&runPO{
		ID: 30, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		AssistantID: "agent", RunKind: string(entity.RunKindTask), Status: string(entity.RunStatusSucceeded),
		Command: datatypes.JSON([]byte(`{}`)), Input: datatypes.JSON([]byte(`{}`)),
		Config: datatypes.JSON([]byte(`{}`)), Context: datatypes.JSON([]byte(`{}`)),
		Metadata: datatypes.JSON([]byte(`{}`)), StreamMode: datatypes.JSON([]byte(`[]`)),
		StartedAt: 600, EndedAt: 600, CreatedAt: 600, UpdatedAt: 600,
	}).Error)
	leaseOwner := "worker-1"
	leaseToken := "lease-1"
	leaseExpiresAt := int64(5_000)
	require.NoError(t, db.Create(&runPO{
		ID: 20, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		AssistantID: "agent", RunKind: string(entity.RunKindTask), Status: string(entity.RunStatusRunning),
		Command: datatypes.JSON([]byte(`{}`)), Input: datatypes.JSON([]byte(`{}`)),
		Config: datatypes.JSON([]byte(`{}`)), Context: datatypes.JSON([]byte(`{}`)),
		Metadata: datatypes.JSON([]byte(`{}`)), StreamMode: datatypes.JSON([]byte(`[]`)),
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiresAt,
		ExecutionGeneration: 3, StartedAt: 700, CreatedAt: 700, UpdatedAt: 700,
	}).Error)
	activeSlot := uint8(1)
	startedAt := int64(700)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 100, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 20,
		AttemptID: "attempt-1", Ordinal: 1, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &activeSlot, NextSequence: 1, LastCommittedSequence: 0,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		StartedAt:         &startedAt, CreatedAt: 700, UpdatedAt: 700,
	}).Error)
}

func adaptiveExecutionRollingPlanMySQLRequest(
	name string,
	eventID int64,
	checkpointID int64,
	parentCheckpointID int64,
	expectedRevision int64,
	now int64,
	itemMutation AdaptivePlanItemMutation,
) CommitAdaptiveExecutionBoundaryRequest {
	nextHighWatermark := int64(1)
	if expectedRevision > 0 {
		nextHighWatermark = 2
	}
	return CommitAdaptiveExecutionBoundaryRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30,
		AttemptID: "attempt-1", Generation: 3,
		LeaseOwner: "worker-1", LeaseToken: "lease-1", Now: now,
		IdempotencyKey: "rolling-plan-" + name,
		Event: &entity.RunEvent{
			ID: eventID, ThreadID: 10, RunID: 20,
			EventType: "plan.updated", Payload: `{"boundary":"` + name + `"}`, CreatedAt: now,
		},
		Checkpoint: &entity.Checkpoint{
			ID: checkpointID, ThreadID: 10, RunID: 20,
			ParentCheckpointID: parentCheckpointID,
			CheckpointNS:       "adaptive", RuntimeType: "eino_adk",
			RuntimeKey: "thread:10:run:20:" + name, EnvelopeVersion: 1,
			ChannelValues: `{}`, ChannelVersions: `{}`, PendingSends: `[]`,
			Metadata: `{"boundary":"` + name + `"}`, CreatedAt: now,
		},
		PlanMutation: &AdaptivePlanMutation{
			PlanScopeRunID:   20,
			ExpectedRevision: expectedRevision, NextRevision: expectedRevision + 1,
			ExpectedHighWatermark: min(expectedRevision, int64(2)),
			NextHighWatermark:     nextHighWatermark,
			Items:                 []AdaptivePlanItemMutation{itemMutation},
		},
	}
}

func assertAdaptiveExecutionRollingPlanMySQLBoundary(
	t *testing.T,
	boundary *CommitAdaptiveExecutionBoundaryResult,
	sequence uint64,
	parentCheckpointID int64,
	planRevision int64,
	highWatermark int64,
	itemVersion int64,
	taskIDs []int64,
) {
	t.Helper()
	require.NotNil(t, boundary)
	require.Equal(t, sequence, boundary.Authority.EventSequence)
	require.Equal(t, sequence, boundary.LastCommittedSequence)
	require.Equal(t, parentCheckpointID, boundary.Checkpoint.ParentCheckpointID)
	require.Equal(t, planRevision, boundary.Plan.Revision)
	require.Equal(t, highWatermark, boundary.Plan.HighWatermark)
	require.Equal(t, int64(20), boundary.Authority.PlanScopeRunID)
	require.Equal(t, int64(10), boundary.Authority.ThreadID)
	require.Equal(t, int64(30), boundary.Authority.JournalRunID)
	require.Equal(t, int64(20), boundary.Authority.ExecutionRunID)
	require.Equal(t, "attempt-1", boundary.Authority.AttemptID)
	require.Equal(t, int64(3), boundary.Authority.ExecutionGeneration)
	require.Equal(t, planRevision, boundary.Authority.PlanRevision)
	require.Equal(t, highWatermark, boundary.Authority.PlanHighWatermark)
	require.Nil(t, boundary.Authority.SourceAttemptID)
	require.Nil(t, boundary.Authority.SourceCheckpointID)
	require.Len(t, boundary.Items, len(taskIDs))
	for index, taskID := range taskIDs {
		require.Equal(t, taskID, boundary.Items[index].TaskID)
		require.Equal(t, itemVersion, boundary.Items[index].Version)
	}
	require.True(t, validAdaptiveExecutionFingerprint(boundary.Authority.PlanItemFingerprint))
}

func readAdaptiveExecutionRollingPlanMySQLState(
	t *testing.T,
	db *gorm.DB,
) adaptiveExecutionRollingPlanMySQLState {
	t.Helper()
	var state adaptiveExecutionRollingPlanMySQLState
	require.NoError(t, db.Where("journal_run_id = ? AND attempt_id = ?", 30, "attempt-1").
		First(&state.attempt).Error)
	require.NoError(t, db.Where("run_id = ?", 20).First(&state.plan).Error)
	require.NoError(t, db.Where("run_id = ?", 20).Order("task_id ASC").Find(&state.items).Error)
	require.NoError(t, db.Where("run_id = ?", 20).Order("id ASC").Find(&state.events).Error)
	require.NoError(t, db.Where("run_id = ?", 20).Order("id ASC").Find(&state.checkpoints).Error)
	return state
}

func TestAdaptiveExecutionP0DMySQLFixtureClosure(t *testing.T) {
	t.Run("VerifiedSuccessSmoke", func(t *testing.T) {
		fixture := newAdaptiveExecutionP0DMySQLFixture(t)
		seedAdaptiveExecutionMySQLState(t, fixture.dbA)

		boundaryRequest := adaptiveExecutionMySQLRequest("fixture-decision", 7001, 8001, 1_000)
		boundaryRequest.Event.EventType = "adaptive.decision"
		boundaryRequest.Event.Payload = `{"schema":"workbench-adaptive-decision.v1","decision_id":"decision-1","decision_revision":1}`
		boundary, err := NewAdaptiveExecutionRepository(fixture.dbA).
			CommitAdaptiveExecutionBoundary(context.Background(), boundaryRequest)
		require.NoError(t, err)
		require.NotNil(t, boundary)
		require.Equal(t, uint64(1), boundary.Authority.EventSequence)
		require.Equal(t, int64(10), boundary.Authority.ThreadID)
		require.Equal(t, int64(20), boundary.Authority.ExecutionRunID)
		require.Equal(t, uint64(3), boundary.Authority.ExecutionGeneration)
		require.Equal(t, int64(30), boundary.Authority.JournalRunID)
		require.Equal(t, "attempt-1", boundary.Authority.AttemptID)
		require.Equal(t, int64(20), boundary.Authority.PlanScopeRunID)
		require.Equal(t, int64(2), boundary.Authority.PlanRevision)
		require.True(t, validAdaptiveExecutionFingerprint(boundary.Authority.PlanItemFingerprint))
		var currentItemRows []agentRunPlanItemPO
		require.NoError(t, fixture.dbA.Where("run_id = ?", boundary.Authority.PlanScopeRunID).
			Order("task_id ASC").Find(&currentItemRows).Error)
		currentItems := make([]*entity.AgentRunPlanItem, 0, len(currentItemRows))
		for index := range currentItemRows {
			currentItems = append(currentItems, currentItemRows[index].toEntity())
		}
		currentFingerprint, err := adaptiveExecutionPlanItemFingerprint(currentItems)
		require.NoError(t, err)

		request := newValidAdaptiveVerifiedSuccessRequestForTest()
		request.Now = 1_200
		request.ExpectedThreadTitle = "adaptive mysql"
		request.ThreadTitle = "adaptive fixture complete"
		request.Message.CreatedAt = request.Now
		request.TitleEvent.CreatedAt = request.Now
		request.CompletionEvent.CreatedAt = request.Now
		request.JournalEvent.CreatedAt = request.Now
		request.JournalEvent.OccurredAtUnixNano = request.Now * int64(time.Millisecond)
		request.TerminalCheckpoint = &entity.Checkpoint{
			ID: 8003, ThreadID: 10, RunID: 20, ParentCheckpointID: boundary.Checkpoint.ID,
			CheckpointNS: boundary.Checkpoint.CheckpointNS, RuntimeType: boundary.Checkpoint.RuntimeType,
			RuntimeKey: boundary.Checkpoint.RuntimeKey, EnvelopeVersion: boundary.Checkpoint.EnvelopeVersion,
			ChannelValues: `{"terminal":true}`, ChannelVersions: `{"state":3}`,
			PendingSends: `[]`, Metadata: `{"fixture":"p0d"}`, CreatedAt: request.Now,
		}
		fallback := *request.TerminalCheckpoint
		fallback.ChannelValues = `{"terminal":true,"title":"preserved"}`
		request.TerminalCheckpointOnTitleConflict = &fallback
		request.AdaptiveGate.Decision = boundary.Authority
		request.AdaptiveGate.Evidence = boundary.Authority
		require.Equal(t, request.AdaptiveGate.Decision, request.AdaptiveGate.Evidence)
		request.AdaptiveGate.VerificationEvent.CreatedAt = request.Now
		request.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
			t,
			request.AdaptiveGate.VerificationEvent.Payload,
			func(fields map[string]any) {
				fields["execution_generation"] = boundary.Authority.ExecutionGeneration
				fields["journal_run_id"] = boundary.Authority.JournalRunID
				fields["attempt_id"] = boundary.Authority.AttemptID
				fields["expected_plan_revision"] = boundary.Authority.PlanRevision
				fields["expected_plan_fingerprint"] = currentFingerprint
				fields["verified_checkpoint_id"] = boundary.Authority.CheckpointID
				fields["evidence_head_event_id"] = boundary.Authority.EventID
				fields["created_at"] = request.Now
			},
		)
		outboxIntent := newAdaptiveExecutionP0DOutboxIntent(t, request.Now)
		request.OutboxIntent = outboxIntent
		require.NoError(t, validateAdaptiveVerifiedSuccessGate(request))

		result, err := fixture.repoA.FinalizeRunSuccess(context.Background(), request)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.Replayed)
		require.True(t, result.TitleUpdated)
		require.Equal(t, entity.RunStatusSucceeded, result.Run.Status)
		require.Equal(t, request.Message, result.Message)
		require.Equal(t, request.TitleEvent, result.TitleEvent)
		require.Equal(t, request.CompletionEvent.ID, result.CompletionEvent.ID)
		require.Equal(t, request.TerminalCheckpoint, result.TerminalCheckpoint)
		require.NotNil(t, result.VerificationEvent)

		assertAdaptiveExecutionP0DVerifiedSuccessState(t, fixture.dbA, request, boundary)
		require.NoError(t, fixture.dbB.Transaction(func(tx *gorm.DB) error {
			inserted, appendErr := outboxIntent.AppendWithResult(
				context.Background(), tx, outboxIntent.Event,
			)
			require.NoError(t, appendErr)
			require.False(t, inserted)
			return nil
		}))
	})

	t.Run("LockWaitObserverCapabilities", func(t *testing.T) {
		fixture := newAdaptiveExecutionP0DMySQLFixture(t)
		require.NoError(t, fixture.dbA.Create(&threadPO{
			ID: 10, SpaceID: 10, CreatorID: 20, Title: "lock observer",
			Status: string(entity.ThreadStatusRunning), Source: string(entity.ThreadSourceWeb),
			Metadata: datatypes.JSON([]byte(`{}`)), CreatedAt: 600, UpdatedAt: 600,
		}).Error)
		assertAdaptiveExecutionP0DThreadRowWait(t, fixture)
	})
}

func TestAdaptiveExecutionP0DMySQL(t *testing.T) {
	t.Run("ConcurrentPlanItemMutation", func(t *testing.T) {
		runAdaptiveExecutionConcurrentPlanItemMutation(t)
	})

	t.Run("LeaseTakeover", func(t *testing.T) {
		t.Run("RecoveryAdmissionVsVerifiedFinalizer", func(t *testing.T) {
			for _, gateMode := range []adaptiveExecutionP0DGateMode{
				adaptiveExecutionP0DGateOn,
				adaptiveExecutionP0DGateOff,
			} {
				gateMode := gateMode
				t.Run(string(gateMode), func(t *testing.T) {
					for _, direction := range []adaptiveExecutionP0DFinalizerDirection{
						adaptiveExecutionP0DCompetitorFirst,
						adaptiveExecutionP0DFinalizerFirst,
					} {
						direction := direction
						t.Run(string(direction), func(t *testing.T) {
							runAdaptiveExecutionP0DRecoveryVsFinalizer(t, gateMode, direction)
						})
					}
				})
			}
		})

		t.Run("RecoveryAdmissionVsCancellation", func(t *testing.T) {
			for _, direction := range []adaptiveExecutionP0DRecoveryCancelDirection{
				adaptiveExecutionP0DRecoveryFirst,
				adaptiveExecutionP0DCancellationFirst,
			} {
				direction := direction
				t.Run(string(direction), func(t *testing.T) {
					runAdaptiveExecutionP0DRecoveryVsCancellation(t, direction)
				})
			}
		})

		t.Run("DeleteThreadIfIdleVsVerifiedFinalizer", func(t *testing.T) {
			for _, gateMode := range []adaptiveExecutionP0DGateMode{
				adaptiveExecutionP0DGateOn,
				adaptiveExecutionP0DGateOff,
			} {
				gateMode := gateMode
				t.Run(string(gateMode), func(t *testing.T) {
					for _, direction := range []adaptiveExecutionP0DFinalizerDirection{
						adaptiveExecutionP0DCompetitorFirst,
						adaptiveExecutionP0DFinalizerFirst,
					} {
						direction := direction
						t.Run(string(direction), func(t *testing.T) {
							runAdaptiveExecutionP0DDeleteVsFinalizer(t, gateMode, direction)
						})
					}
				})
			}
		})
	})

	t.Run("CancelVsVerifiedSuccess", func(t *testing.T) {
		for _, direction := range []adaptiveExecutionP0DCancelSuccessDirection{
			adaptiveExecutionP0DCancelWins,
			adaptiveExecutionP0DSuccessWins,
		} {
			direction := direction
			t.Run(string(direction), func(t *testing.T) {
				runAdaptiveExecutionP0DCancelVsVerifiedSuccess(t, direction)
			})
		}
	})

	t.Run("CrashAfterCommitRetry", func(t *testing.T) {
		runAdaptiveExecutionP0DCrashAfterCommitRetry(t)
	})
}

func TestAdaptiveExecutionP0DCrashHelper(t *testing.T) {
	if os.Getenv(adaptiveExecutionP0DCrashHelperEnv) != "1" {
		return
	}
	db, _ := openAdaptiveExecutionP0DExistingMySQL(t)
	request, tracker := adaptiveExecutionP0DCrashFinalizerRequest(t)
	result, err := (&threadRepository{db: db}).FinalizeRunSuccess(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Replayed)
	require.Equal(t, []bool{true}, tracker.snapshot())
	assertAdaptiveExecutionP0DCrashTerminalTupleCounts(t, db, request, 1)
	snapshot := readAdaptiveExecutionP0DDurableSnapshot(t, db)
	assertAdaptiveExecutionP0DCrashDurableSnapshot(t, snapshot, request)
	assertAdaptiveExecutionP0DCrashFirstResultIdentities(t, request, result)
	os.Exit(adaptiveExecutionP0DCrashHelperExitCode)
}

func runAdaptiveExecutionConcurrentPlanItemMutation(t *testing.T) {
	t.Helper()
	db, repoA, repoB := adaptiveExecutionMySQLIntegrationRepositories(t)
	seedAdaptiveExecutionMySQLState(t, db)

	requests := map[string]CommitAdaptiveExecutionBoundaryRequest{
		"writer-a": adaptiveExecutionMySQLRequest("writer-a", 7001, 8001, 1000),
		"writer-b": adaptiveExecutionMySQLRequest("writer-b", 7002, 8002, 1000),
	}
	results := runAdaptiveExecutionMySQLRace(t, []adaptiveExecutionMySQLRaceCall{
		{
			name: "writer-a",
			call: func(ctx context.Context) (any, error) {
				return repoA.CommitAdaptiveExecutionBoundary(ctx, requests["writer-a"])
			},
		},
		{
			name: "writer-b",
			call: func(ctx context.Context) (any, error) {
				return repoB.CommitAdaptiveExecutionBoundary(ctx, requests["writer-b"])
			},
		},
	})

	var winner, loser adaptiveExecutionMySQLRaceResult
	for _, result := range results {
		requireAdaptiveExecutionMySQLRaceError(t, result.err)
		if result.err == nil {
			require.Empty(t, winner.name)
			winner = result
		} else {
			require.Empty(t, loser.name)
			loser = result
		}
	}
	require.NotEmpty(t, winner.name)
	require.NotEmpty(t, loser.name)
	require.ErrorIs(t, loser.err, ErrAdaptiveExecutionPlanRevisionConflict)
	require.Nil(t, loser.value)
	winnerResult, ok := winner.value.(*CommitAdaptiveExecutionBoundaryResult)
	require.True(t, ok)
	require.NotNil(t, winnerResult)
	winnerRequest := requests[winner.name]
	expectedWinnerItem := *winnerRequest.PlanMutation.Items[0].NextItem
	expectedWinnerItem.CreatedAt = 900
	expectedWinnerItem.UpdatedAt = winnerRequest.Now
	expectedWinnerPlan := &entity.AgentRunPlan{
		RunID: 20, ThreadID: 10, SpaceID: 10, UserID: 20,
		HighWatermark: 2, Revision: 2, CreatedAt: 800, UpdatedAt: winnerRequest.Now,
	}
	require.Equal(t, expectedWinnerPlan, winnerResult.Plan)
	require.Len(t, winnerResult.Items, 1)
	assertAdaptiveExecutionMySQLPlanItemEqual(t, &expectedWinnerItem, winnerResult.Items[0])

	state := readAdaptiveExecutionMySQLState(t, db)
	assertAdaptiveExecutionMySQLRunUnchanged(t, state.run)
	require.Equal(t, uint64(2), state.attempt.NextSequence)
	require.Equal(t, uint64(1), state.attempt.LastCommittedSequence)
	require.Equal(t, expectedWinnerPlan, state.plan.toEntity())
	require.Equal(t, state.plan.toEntity(), winnerResult.Plan)
	require.Len(t, state.items, 2)
	assertAdaptiveExecutionMySQLPlanItemEqual(t, &expectedWinnerItem, state.items[0].toEntity())
	assertAdaptiveExecutionMySQLSentinelUnchanged(t, state.items[1])
	assertAdaptiveExecutionMySQLCommittedArtifacts(t, state, requests[winner.name])
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &runEventPO{}, "id = ?", requests[loser.name].Event.ID))
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &checkpointPO{}, "id = ?", requests[loser.name].Checkpoint.ID))
}

type adaptiveExecutionP0DGateMode string

const (
	adaptiveExecutionP0DGateOn  adaptiveExecutionP0DGateMode = "gate_on"
	adaptiveExecutionP0DGateOff adaptiveExecutionP0DGateMode = "gate_off"
)

type adaptiveExecutionP0DFinalizerDirection string

const (
	adaptiveExecutionP0DCompetitorFirst adaptiveExecutionP0DFinalizerDirection = "competitor_first"
	adaptiveExecutionP0DFinalizerFirst  adaptiveExecutionP0DFinalizerDirection = "finalizer_first"
)

type adaptiveExecutionP0DRecoveryCancelDirection string

const (
	adaptiveExecutionP0DRecoveryFirst     adaptiveExecutionP0DRecoveryCancelDirection = "recovery_first"
	adaptiveExecutionP0DCancellationFirst adaptiveExecutionP0DRecoveryCancelDirection = "cancellation_first"
)

type adaptiveExecutionP0DCancelSuccessDirection string

const (
	adaptiveExecutionP0DCancelWins  adaptiveExecutionP0DCancelSuccessDirection = "cancel_wins"
	adaptiveExecutionP0DSuccessWins adaptiveExecutionP0DCancelSuccessDirection = "success_wins"
)

const (
	adaptiveExecutionP0DCrashHelperEnv             = "COZE_AGENTTHREAD_P0D_CRASH_HELPER"
	adaptiveExecutionP0DCrashHelperExitCode        = 86
	adaptiveExecutionP0DCrashSuccessResponseMarker = "P0D_CRASH_SUCCESS_RESPONSE"
)

type adaptiveExecutionP0DLeaseFixture struct {
	mysql               adaptiveExecutionP0DMySQLFixture
	boundaryRequest     CommitAdaptiveExecutionBoundaryRequest
	boundary            *CommitAdaptiveExecutionBoundaryResult
	finalizer           FinalizeRunSuccessRequest
	finalizerOutbox     *adaptiveExecutionP0DOutboxTracker
	recovery            CreateRunBundleRequest
	cancellation        RequestRunCancellationRequest
	cancellationOutbox  *adaptiveExecutionP0DOutboxTracker
	threadBefore        threadPO
	rootBefore          runPO
	runBefore           runPO
	attemptBefore       runAttemptPO
	planBefore          agentRunPlanPO
	itemsBefore         []agentRunPlanItemPO
	boundaryEventBefore runEventPO
	boundaryCPBefore    checkpointPO
	sentinel            *runPO
}

type adaptiveExecutionP0DDurableSnapshot struct {
	Threads     []threadPO
	Runs        []runPO
	Attempts    []runAttemptPO
	Events      []runEventPO
	Messages    []messagePO
	Checkpoints []checkpointPO
	Plans       []agentRunPlanPO
	Items       []agentRunPlanItemPO
	Outbox      []adaptiveExecutionP0DOutboxProbePO
}

func prepareAdaptiveExecutionP0DLeaseFixture(
	t *testing.T,
	gateMode adaptiveExecutionP0DGateMode,
	withDeleteSentinel bool,
) adaptiveExecutionP0DLeaseFixture {
	t.Helper()
	fixture := newAdaptiveExecutionP0DMySQLFixture(t)
	seedAdaptiveExecutionMySQLState(t, fixture.dbA)
	if withDeleteSentinel {
		// Keep the active Execution Run first in DeleteThreadIfIdle's
		// (thread_id, created_at, id) scan. The logical root remains a complete,
		// terminal public authority row; only the fixture's equal creation instant
		// avoids acquiring a filtered root row before the active Run.
		createdAtUpdate := fixture.dbA.Model(&runPO{}).
			Where("id = ?", 20).
			UpdateColumn("created_at", 600)
		require.NoError(t, createdAtUpdate.Error)
		require.Equal(t, int64(1), createdAtUpdate.RowsAffected)
	}

	boundaryRequest := adaptiveExecutionMySQLRequest("p0d-decision", 7001, 8001, 1_000)
	boundaryRequest.Event.EventType = "adaptive.decision"
	boundaryRequest.Event.Payload =
		`{"schema":"workbench-adaptive-decision.v1","decision_id":"decision-1","decision_revision":1}`
	boundary, err := NewAdaptiveExecutionRepository(fixture.dbA).
		CommitAdaptiveExecutionBoundary(context.Background(), boundaryRequest)
	require.NoError(t, err)
	require.NotNil(t, boundary)
	require.Equal(t, int64(10), boundary.Authority.ThreadID)
	require.Equal(t, uint64(1), boundary.Authority.EventSequence)
	require.Equal(t, int64(30), boundary.Authority.JournalRunID)
	require.Equal(t, int64(20), boundary.Authority.ExecutionRunID)
	require.Equal(t, uint64(3), boundary.Authority.ExecutionGeneration)
	require.Equal(t, "attempt-1", boundary.Authority.AttemptID)
	require.Equal(t, int64(7001), boundary.Authority.EventID)
	require.Equal(t, boundaryRequest.IdempotencyKey, boundary.Authority.IdempotencyKey)
	require.Equal(t, int64(8001), boundary.Authority.CheckpointID)
	require.Equal(t, int64(20), boundary.Authority.PlanScopeRunID)
	require.Equal(t, int64(2), boundary.Authority.PlanRevision)
	require.True(t, validAdaptiveExecutionFingerprint(boundary.Authority.PlanItemFingerprint))

	finalizer, finalizerOutbox := adaptiveExecutionP0DFinalizerRequest(
		t,
		fixture.dbA,
		boundary,
		gateMode,
	)
	recovery := adaptiveExecutionP0DRecoveryRequest(t, boundary)
	cancellation, cancellationOutbox := adaptiveExecutionP0DCancellationRequest(t)
	var threadBefore threadPO
	var rootBefore, runBefore runPO
	var attemptBefore runAttemptPO
	var planBefore agentRunPlanPO
	var itemsBefore []agentRunPlanItemPO
	var boundaryEventBefore runEventPO
	var boundaryCPBefore checkpointPO
	require.NoError(t, fixture.dbA.Where("id = ?", 10).First(&threadBefore).Error)
	require.NoError(t, fixture.dbA.Where("id = ?", 30).First(&rootBefore).Error)
	require.NoError(t, fixture.dbA.Where("id = ?", 20).First(&runBefore).Error)
	require.NoError(t, fixture.dbA.Where("id = ?", 100).First(&attemptBefore).Error)
	require.NoError(t, fixture.dbA.Where("run_id = ?", 20).First(&planBefore).Error)
	require.NoError(t, fixture.dbA.Where("run_id = ?", 20).Order("task_id ASC").Find(&itemsBefore).Error)
	require.NoError(t, fixture.dbA.Where("id = ?", boundary.Authority.EventID).First(&boundaryEventBefore).Error)
	require.NoError(t, fixture.dbA.Where("id = ?", boundary.Authority.CheckpointID).First(&boundaryCPBefore).Error)

	var sentinel *runPO
	if withDeleteSentinel {
		stored := seedAdaptiveExecutionP0DDeleteSentinel(t, fixture.dbA)
		sentinel = &stored
		require.Equal(t, []int64{20, 40}, adaptiveExecutionP0DActiveRunIDs(t, fixture.dbA))
	}
	return adaptiveExecutionP0DLeaseFixture{
		mysql: fixture, boundaryRequest: boundaryRequest, boundary: boundary,
		finalizer: finalizer, finalizerOutbox: finalizerOutbox,
		recovery: recovery, cancellation: cancellation,
		cancellationOutbox: cancellationOutbox,
		threadBefore:       threadBefore, rootBefore: rootBefore, runBefore: runBefore,
		attemptBefore: attemptBefore, planBefore: planBefore, itemsBefore: itemsBefore,
		boundaryEventBefore: boundaryEventBefore, boundaryCPBefore: boundaryCPBefore,
		sentinel: sentinel,
	}
}

func adaptiveExecutionP0DFinalizerRequest(
	t *testing.T,
	db *gorm.DB,
	boundary *CommitAdaptiveExecutionBoundaryResult,
	gateMode adaptiveExecutionP0DGateMode,
) (FinalizeRunSuccessRequest, *adaptiveExecutionP0DOutboxTracker) {
	t.Helper()
	require.NotNil(t, db)
	require.NotNil(t, boundary)
	var itemRows []agentRunPlanItemPO
	require.NoError(t, db.Where("run_id = ?", boundary.Authority.PlanScopeRunID).
		Order("task_id ASC").Find(&itemRows).Error)
	items := make([]*entity.AgentRunPlanItem, 0, len(itemRows))
	for index := range itemRows {
		items = append(items, itemRows[index].toEntity())
	}
	return adaptiveExecutionP0DFinalizerRequestWithItems(t, boundary, gateMode, items)
}

func adaptiveExecutionP0DFinalizerRequestWithItems(
	t *testing.T,
	boundary *CommitAdaptiveExecutionBoundaryResult,
	gateMode adaptiveExecutionP0DGateMode,
	items []*entity.AgentRunPlanItem,
) (FinalizeRunSuccessRequest, *adaptiveExecutionP0DOutboxTracker) {
	t.Helper()
	require.NotNil(t, boundary)
	itemFingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	require.NoError(t, err)
	require.True(t, validAdaptiveExecutionFingerprint(boundary.Authority.PlanItemFingerprint))
	require.True(t, validAdaptiveExecutionFingerprint(itemFingerprint))

	request := newValidAdaptiveVerifiedSuccessRequestForTest()
	request.Now = 1_200
	request.ExpectedThreadTitle = "adaptive mysql"
	request.ThreadTitle = "adaptive p0d complete"
	request.Message.CreatedAt = request.Now
	request.TitleEvent.CreatedAt = request.Now
	request.TitleEvent.Payload = `{"thread_title":"adaptive p0d complete"}`
	request.CompletionEvent.CreatedAt = request.Now
	request.JournalEvent.CreatedAt = request.Now
	request.JournalEvent.OccurredAtUnixNano = request.Now * int64(time.Millisecond)
	request.TerminalCheckpoint = &entity.Checkpoint{
		ID: 8003, ThreadID: 10, RunID: 20, ParentCheckpointID: boundary.Checkpoint.ID,
		CheckpointNS: boundary.Checkpoint.CheckpointNS, RuntimeType: boundary.Checkpoint.RuntimeType,
		RuntimeKey: boundary.Checkpoint.RuntimeKey, EnvelopeVersion: boundary.Checkpoint.EnvelopeVersion,
		ChannelValues: `{"terminal":true}`, ChannelVersions: `{"state":3}`,
		PendingSends: `[]`, Metadata: `{"fixture":"p0d-lock"}`, CreatedAt: request.Now,
	}
	fallback := *request.TerminalCheckpoint
	fallback.ChannelValues = `{"terminal":true,"title":"preserved"}`
	request.TerminalCheckpointOnTitleConflict = &fallback
	request.AdaptiveGate.Decision = boundary.Authority
	request.AdaptiveGate.Evidence = boundary.Authority
	require.Equal(t, request.AdaptiveGate.Decision, request.AdaptiveGate.Evidence)
	request.AdaptiveGate.VerificationEvent.CreatedAt = request.Now
	request.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
		t,
		request.AdaptiveGate.VerificationEvent.Payload,
		func(fields map[string]any) {
			fields["execution_generation"] = boundary.Authority.ExecutionGeneration
			fields["journal_run_id"] = boundary.Authority.JournalRunID
			fields["attempt_id"] = boundary.Authority.AttemptID
			fields["expected_plan_revision"] = boundary.Authority.PlanRevision
			fields["expected_plan_fingerprint"] = itemFingerprint
			fields["verified_checkpoint_id"] = boundary.Authority.CheckpointID
			fields["evidence_head_event_id"] = boundary.Authority.EventID
			fields["created_at"] = request.Now
		},
	)
	outboxEvent := adaptiveVerifiedSuccessOutboxEventForTest(request.Now)
	outboxIntent, tracker := newAdaptiveExecutionP0DTrackedOutboxIntent(t, outboxEvent)
	request.OutboxIntent = outboxIntent
	if gateMode == adaptiveExecutionP0DGateOff {
		request.AdaptiveGate = nil
	}
	require.NoError(t, validateAdaptiveVerifiedSuccessGate(request))
	return request, tracker
}

func adaptiveExecutionP0DRecoveryRequest(
	t *testing.T,
	boundary *CommitAdaptiveExecutionBoundaryResult,
) CreateRunBundleRequest {
	t.Helper()
	require.NotNil(t, boundary)
	const recoveryNow = int64(6_000)
	recoveryKey := "p0d-recovery-attempt-2"
	run := newRepositoryTestRun(21, 10, entity.RunStatusQueued, recoveryNow)
	run.SpaceID = 10
	run.CreatorID = 20
	run.AssistantID = "agent"
	run.RunKind = entity.RunKindTask
	run.IdempotencyKey = recoveryKey
	run.MultitaskStrategy = "reject"
	sourceAttemptID := "attempt-1"
	sourceCheckpointID := boundary.Authority.CheckpointID
	request := CreateRunBundleRequest{
		Run: run,
		Attempt: &entity.RunAttempt{
			ID: 101, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 21,
			AttemptID: "attempt-2", Status: entity.RunAttemptStatusPending,
			SourceCheckpointID: &sourceCheckpointID, SourceAttemptID: &sourceAttemptID,
			RecoveryIdempotencyKey: &recoveryKey,
			ProjectionState:        entity.JournalProjectionStateHealthy,
		},
		RecoverySourceLease: &ReconcileExpiredRunLeaseRequest{
			RunID: 20, LeaseOwner: "worker-1", LeaseToken: "lease-1",
			ExecutionGeneration: 3, ToStatus: entity.RunStatusFailed, Now: recoveryNow,
			ErrorCode:    runRecoveredErrorCodeForRepositoryTest,
			ErrorMessage: "execution recovered from durable P0D authority",
			Event: &entity.RunEvent{
				ID: 7201, ThreadID: 10, RunID: 20, EventType: "run.failed",
				Payload: `{"status":"failed","error_code":"run_recovered"}`, CreatedAt: recoveryNow,
			},
			JournalEvent: &entity.JournalEvent{
				ID: 7201, ThreadID: 10, RunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
				IdempotencyKey: "p0d-recovery-source-failed", SchemaVersion: entity.JournalSchemaVersion,
				Status: string(entity.RunAttemptStatusFailed), Visibility: entity.JournalVisibilityUser,
				PayloadVersion: entity.JournalPayloadVersion, EventType: "run.lifecycle",
				Payload:            `{"type":"terminal","data":{"status":"failed"}}`,
				OccurredAtUnixNano: recoveryNow * int64(time.Millisecond), CreatedAt: recoveryNow,
			},
		},
	}
	require.NotNil(t, request.RecoverySourceLease)
	require.Less(t, int64(5_000), request.RecoverySourceLease.Now)
	return request
}

func adaptiveExecutionP0DCancellationRequest(
	t *testing.T,
) (RequestRunCancellationRequest, *adaptiveExecutionP0DOutboxTracker) {
	t.Helper()
	const cancellationNow = int64(1_300)
	outboxEvent := adaptiveVerifiedSuccessOutboxEventForTest(cancellationNow)
	outboxEvent.EventID = "p0d-cancel-outbox-1"
	outboxEvent.EventType = domainnotification.EventTaskCancelled
	outboxEvent.AggregateVersion = 4
	outboxIntent, tracker := newAdaptiveExecutionP0DTrackedOutboxIntent(t, outboxEvent)
	return RequestRunCancellationRequest{
		RunID: 20, Now: cancellationNow,
		ErrorCode: "user_canceled", ErrorMessage: "canceled by P0D race",
		Event: &entity.RunEvent{
			ID: 7301, ThreadID: 10, RunID: 20, EventType: "run.canceled",
			Payload: `{"status":"canceled"}`, CreatedAt: cancellationNow,
		},
		JournalEvent: &entity.JournalEvent{
			ID: 7301, ThreadID: 10, RunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
			IdempotencyKey: "p0d-cancel-attempt-1", SchemaVersion: entity.JournalSchemaVersion,
			Status: string(entity.RunAttemptStatusCancelled), Visibility: entity.JournalVisibilityUser,
			PayloadVersion: entity.JournalPayloadVersion, EventType: "run.lifecycle",
			Payload:            `{"type":"terminal","data":{"status":"cancelled"}}`,
			OccurredAtUnixNano: cancellationNow * int64(time.Millisecond), CreatedAt: cancellationNow,
		},
		OutboxIntent: outboxIntent,
	}, tracker
}

func seedAdaptiveExecutionP0DDeleteSentinel(t *testing.T, db *gorm.DB) runPO {
	t.Helper()
	leaseOwner := "sentinel-worker"
	leaseToken := "sentinel-lease"
	leaseExpiresAt := int64(9_000)
	sentinel := runPO{
		ID: 40, ThreadID: 10, ParentRunID: 0, SpaceID: 10, CreatorID: 20,
		AssistantID: "sentinel-agent", RunKind: string(entity.RunKindTask),
		Status:     string(entity.RunStatusRunning),
		Command:    datatypes.JSON([]byte(`{"sentinel":"command"}`)),
		Input:      datatypes.JSON([]byte(`{"sentinel":"input"}`)),
		Config:     datatypes.JSON([]byte(`{"sentinel":"config"}`)),
		Context:    datatypes.JSON([]byte(`{"sentinel":"context"}`)),
		Metadata:   datatypes.JSON([]byte(`{"sentinel":true}`)),
		StreamMode: datatypes.JSON([]byte(`["messages"]`)),
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiresAt,
		ExecutionGeneration: 9, StartedAt: 750, CreatedAt: 750, UpdatedAt: 750,
	}
	require.NoError(t, db.Create(&sentinel).Error)
	var stored runPO
	require.NoError(t, db.Where("id = ?", sentinel.ID).First(&stored).Error)
	assertAdaptiveExecutionP0DRunPOEqual(t, sentinel, stored)
	return stored
}

func adaptiveExecutionP0DActiveRunIDs(t *testing.T, db *gorm.DB) []int64 {
	t.Helper()
	var runs []runPO
	require.NoError(t, db.Where("thread_id = ?", 10).
		Where("parent_run_id = 0").
		Where("(run_kind = ? OR run_kind = '')", string(entity.RunKindTask)).
		Where("status IN ?", []string{
			string(entity.RunStatusPending),
			string(entity.RunStatusQueued),
			string(entity.RunStatusRunning),
		}).Order("id ASC").Find(&runs).Error)
	ids := make([]int64, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

func runAdaptiveExecutionP0DRecoveryVsFinalizer(
	t *testing.T,
	gateMode adaptiveExecutionP0DGateMode,
	direction adaptiveExecutionP0DFinalizerDirection,
) {
	t.Helper()
	state := prepareAdaptiveExecutionP0DLeaseFixture(t, gateMode, false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	recoveryRepo := &threadRepository{db: state.mysql.dbB}

	var finalizerBarrier *adaptiveExecutionP0DLockBarrier
	var recoveryBarrier *adaptiveExecutionP0DLockBarrier
	var finalizerResult, recoveryResult <-chan adaptiveExecutionMySQLRaceResult
	var wait adaptiveExecutionP0DLockWaitRow
	var finalizerConnectionID, recoveryConnectionID uint64
	switch direction {
	case adaptiveExecutionP0DCompetitorFirst:
		recoveryBarrier = newAdaptiveExecutionP0DLockBarrier(adaptiveExecutionP0DThreadLockMatch)
		finalizerBarrier = newAdaptiveExecutionP0DLockBarrier(nil)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbB, recoveryBarrier)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbA, finalizerBarrier)
		defer recoveryBarrier.releaseParticipant()

		recoveryResult = startAdaptiveExecutionP0DCall(
			ctx,
			"recovery",
			func(callCtx context.Context) (any, error) {
				return recoveryRepo.CreateRunBundle(recoveryBarrier.context(callCtx), state.recovery)
			},
		)
		recoveryConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, recoveryBarrier, state.mysql.connectionIDB,
		)
		waitAdaptiveExecutionP0DLockAcquired(
			t,
			ctx,
			recoveryBarrier,
			recoveryConnectionID,
		)
		finalizerResult = startAdaptiveExecutionP0DCall(
			ctx,
			"finalizer",
			func(callCtx context.Context) (any, error) {
				return state.mysql.repoA.FinalizeRunSuccess(
					finalizerBarrier.context(callCtx),
					state.finalizer,
				)
			},
		)
		finalizerConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, finalizerBarrier, state.mysql.connectionIDA,
		)
		wait = observeAdaptiveExecutionP0DExactRowWait(
			t,
			ctx,
			state.mysql,
			finalizerConnectionID,
			recoveryConnectionID,
			adaptiveExecutionP0DPrimaryWait("agent_threads", 10),
		)
		recoveryBarrier.releaseParticipant()

	case adaptiveExecutionP0DFinalizerFirst:
		finalizerBarrier = newAdaptiveExecutionP0DLockBarrier(
			adaptiveExecutionP0DFinalizerExecutionLockMatch(gateMode),
		)
		recoveryBarrier = newAdaptiveExecutionP0DLockBarrier(nil)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbA, finalizerBarrier)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbB, recoveryBarrier)
		defer finalizerBarrier.releaseParticipant()

		finalizerResult = startAdaptiveExecutionP0DCall(
			ctx,
			"finalizer",
			func(callCtx context.Context) (any, error) {
				return state.mysql.repoA.FinalizeRunSuccess(
					finalizerBarrier.context(callCtx),
					state.finalizer,
				)
			},
		)
		finalizerConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, finalizerBarrier, state.mysql.connectionIDA,
		)
		waitAdaptiveExecutionP0DLockAcquired(
			t,
			ctx,
			finalizerBarrier,
			finalizerConnectionID,
		)
		first, ok := finalizerBarrier.firstStage()
		require.True(t, ok)
		recoveryResult = startAdaptiveExecutionP0DCall(
			ctx,
			"recovery",
			func(callCtx context.Context) (any, error) {
				return recoveryRepo.CreateRunBundle(recoveryBarrier.context(callCtx), state.recovery)
			},
		)
		recoveryConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, recoveryBarrier, state.mysql.connectionIDB,
		)
		expectedWait := adaptiveExecutionP0DPrimaryWait("agent_threads", 10)
		if first.Table != "agent_threads" {
			if gateMode == adaptiveExecutionP0DGateOn {
				expectedWait = adaptiveExecutionP0DPrimaryWait("agent_runs", 30)
			} else {
				expectedWait = adaptiveExecutionP0DPrimaryWait("agent_runs", 20)
			}
		}
		wait = observeAdaptiveExecutionP0DExactRowWait(
			t,
			ctx,
			state.mysql,
			recoveryConnectionID,
			finalizerConnectionID,
			expectedWait,
		)
		finalizerBarrier.releaseParticipant()

	default:
		t.Fatalf("unsupported recovery/finalizer direction %q", direction)
	}

	results := []adaptiveExecutionMySQLRaceResult{
		waitAdaptiveExecutionP0DCall(t, ctx, finalizerResult),
		waitAdaptiveExecutionP0DCall(t, ctx, recoveryResult),
	}
	first, ok := finalizerBarrier.firstStage()
	require.True(t, ok, "fixture failure: finalizer did not record its first acquired lock")
	failAdaptiveExecutionP0DOnOldDeadlock(
		t,
		results,
		wait,
		first,
		adaptiveExecutionP0DOldFinalizerLockExpectation(gateMode),
		true,
	)
	require.Equal(t, "agent_threads", first.Table)
	require.Equal(t, adaptiveExecutionP0DQueryLock, first.Operation)
	require.True(t, adaptiveExecutionP0DStageHasInt64(first, 10))
	assertAdaptiveExecutionP0DRecoveryVsFinalizerPostState(t, state, gateMode, direction, results)
}

func runAdaptiveExecutionP0DRecoveryVsCancellation(
	t *testing.T,
	direction adaptiveExecutionP0DRecoveryCancelDirection,
) {
	t.Helper()
	state := prepareAdaptiveExecutionP0DLeaseFixture(t, adaptiveExecutionP0DGateOn, false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	recoveryRepo := state.mysql.repoA
	cancellationRepo := &threadRepository{db: state.mysql.dbB}

	var recoveryBarrier *adaptiveExecutionP0DLockBarrier
	var cancellationBarrier *adaptiveExecutionP0DLockBarrier
	var recoveryResult, cancellationResult <-chan adaptiveExecutionMySQLRaceResult
	var wait adaptiveExecutionP0DLockWaitRow
	var firstSource adaptiveExecutionP0DLockStage
	var recoveryConnectionID, cancellationConnectionID uint64
	recoveryHeldAttemptBeforeRunWait := true
	switch direction {
	case adaptiveExecutionP0DRecoveryFirst:
		recoveryBarrier = newAdaptiveExecutionP0DLockBarrier(adaptiveExecutionP0DRecoverySourceLockMatch)
		cancellationBarrier = newAdaptiveExecutionP0DLockBarrier(nil)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbA, recoveryBarrier)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbB, cancellationBarrier)
		defer recoveryBarrier.releaseParticipant()
		recoveryResult = startAdaptiveExecutionP0DCall(
			ctx,
			"recovery",
			func(callCtx context.Context) (any, error) {
				return recoveryRepo.CreateRunBundle(recoveryBarrier.context(callCtx), state.recovery)
			},
		)
		recoveryConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, recoveryBarrier, state.mysql.connectionIDA,
		)
		firstSource = waitAdaptiveExecutionP0DLockAcquired(
			t,
			ctx,
			recoveryBarrier,
			recoveryConnectionID,
		)
		cancellationResult = startAdaptiveExecutionP0DCall(
			ctx,
			"cancellation",
			func(callCtx context.Context) (any, error) {
				return cancellationRepo.RequestRunCancellation(
					cancellationBarrier.context(callCtx),
					state.cancellation,
				)
			},
		)
		cancellationConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, cancellationBarrier, state.mysql.connectionIDB,
		)
		expectedWait := adaptiveExecutionP0DPrimaryWait("agent_runs", 20)
		if firstSource.Table == "agent_run_attempts" {
			expectedWait = adaptiveExecutionP0DAttemptWait()
		}
		wait = observeAdaptiveExecutionP0DExactRowWait(
			t,
			ctx,
			state.mysql,
			cancellationConnectionID,
			recoveryConnectionID,
			expectedWait,
		)
		recoveryBarrier.releaseParticipant()

	case adaptiveExecutionP0DCancellationFirst:
		cancellationBarrier = newAdaptiveExecutionP0DLockBarrier(adaptiveExecutionP0DCancellationRunLockMatch)
		recoveryBarrier = newAdaptiveExecutionP0DLockRecorder(adaptiveExecutionP0DRecoverySourceLockMatch)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbA, cancellationBarrier)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbB, recoveryBarrier)
		defer cancellationBarrier.releaseParticipant()
		cancellationResult = startAdaptiveExecutionP0DCall(
			ctx,
			"cancellation",
			func(callCtx context.Context) (any, error) {
				return recoveryRepo.RequestRunCancellation(
					cancellationBarrier.context(callCtx),
					state.cancellation,
				)
			},
		)
		cancellationConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, cancellationBarrier, state.mysql.connectionIDA,
		)
		waitAdaptiveExecutionP0DLockAcquired(
			t,
			ctx,
			cancellationBarrier,
			cancellationConnectionID,
		)
		recoveryResult = startAdaptiveExecutionP0DCall(
			ctx,
			"recovery",
			func(callCtx context.Context) (any, error) {
				return (&threadRepository{db: state.mysql.dbB}).CreateRunBundle(
					recoveryBarrier.context(callCtx),
					state.recovery,
				)
			},
		)
		recoveryConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, recoveryBarrier, state.mysql.connectionIDB,
		)
		wait = observeAdaptiveExecutionP0DExactRowWait(
			t,
			ctx,
			state.mysql,
			recoveryConnectionID,
			cancellationConnectionID,
			adaptiveExecutionP0DPrimaryWait("agent_runs", 20),
		)
		heldStage, held := recoveryBarrier.firstWatchedStage()
		recoveryHeldAttemptBeforeRunWait = held &&
			adaptiveExecutionP0DOldRecoverySourceLockExpectation().matches(heldStage)
		cancellationBarrier.releaseParticipant()

	default:
		t.Fatalf("unsupported recovery/cancellation direction %q", direction)
	}
	results := []adaptiveExecutionMySQLRaceResult{
		waitAdaptiveExecutionP0DCall(t, ctx, recoveryResult),
		waitAdaptiveExecutionP0DCall(t, ctx, cancellationResult),
	}
	if direction == adaptiveExecutionP0DCancellationFirst {
		var ok bool
		firstSource, ok = recoveryBarrier.firstWatchedStage()
		require.True(t, ok, "fixture failure: recovery did not record a source lock")
	}
	failAdaptiveExecutionP0DOnOldDeadlock(
		t,
		results,
		wait,
		firstSource,
		adaptiveExecutionP0DOldRecoverySourceLockExpectation(),
		recoveryHeldAttemptBeforeRunWait,
	)
	require.Equal(t, "agent_runs", firstSource.Table)
	require.Equal(t, adaptiveExecutionP0DQueryLock, firstSource.Operation)
	require.True(t, adaptiveExecutionP0DStageHasInt64(firstSource, 20))
	assertAdaptiveExecutionP0DRecoveryVsCancellationPostState(t, state, direction, results)
}

func runAdaptiveExecutionP0DCancelVsVerifiedSuccess(
	t *testing.T,
	direction adaptiveExecutionP0DCancelSuccessDirection,
) {
	t.Helper()
	state := prepareAdaptiveExecutionP0DLeaseFixture(t, adaptiveExecutionP0DGateOn, false)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var finalizerResult, cancellationResult <-chan adaptiveExecutionMySQLRaceResult
	var winnerBarrier, loserRecorder *adaptiveExecutionP0DLockBarrier
	var winnerConnectionID, loserConnectionID uint64
	switch direction {
	case adaptiveExecutionP0DCancelWins:
		winnerBarrier = newAdaptiveExecutionP0DLockBarrier(adaptiveExecutionP0DCancellationRunLockMatch)
		loserRecorder = newAdaptiveExecutionP0DLockRecorder(nil)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbA, winnerBarrier)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbB, loserRecorder)
		defer winnerBarrier.releaseParticipant()
		cancellationResult = startAdaptiveExecutionP0DCall(
			ctx,
			"cancellation",
			func(callCtx context.Context) (any, error) {
				return state.mysql.repoA.RequestRunCancellation(
					winnerBarrier.context(callCtx),
					state.cancellation,
				)
			},
		)
		winnerConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, winnerBarrier, state.mysql.connectionIDA,
		)
		winnerStage := waitAdaptiveExecutionP0DLockAcquired(
			t, ctx, winnerBarrier, winnerConnectionID,
		)
		require.Equal(t, "agent_runs", winnerStage.Table)
		require.Equal(t, adaptiveExecutionP0DQueryLock, winnerStage.Operation)
		require.True(t, adaptiveExecutionP0DStageHasInt64(winnerStage, 20))
		finalizerResult = startAdaptiveExecutionP0DCall(
			ctx,
			"finalizer",
			func(callCtx context.Context) (any, error) {
				return (&threadRepository{db: state.mysql.dbB}).FinalizeRunSuccess(
					loserRecorder.context(callCtx),
					state.finalizer,
				)
			},
		)
		loserConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, loserRecorder, state.mysql.connectionIDB,
		)

	case adaptiveExecutionP0DSuccessWins:
		winnerBarrier = newAdaptiveExecutionP0DLockBarrier(
			adaptiveExecutionP0DFinalizerExecutionLockMatch(adaptiveExecutionP0DGateOn),
		)
		loserRecorder = newAdaptiveExecutionP0DLockRecorder(nil)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbA, winnerBarrier)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbB, loserRecorder)
		defer winnerBarrier.releaseParticipant()
		finalizerResult = startAdaptiveExecutionP0DCall(
			ctx,
			"finalizer",
			func(callCtx context.Context) (any, error) {
				return state.mysql.repoA.FinalizeRunSuccess(
					winnerBarrier.context(callCtx),
					state.finalizer,
				)
			},
		)
		winnerConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, winnerBarrier, state.mysql.connectionIDA,
		)
		winnerStage := waitAdaptiveExecutionP0DLockAcquired(
			t, ctx, winnerBarrier, winnerConnectionID,
		)
		require.Equal(t, "agent_runs", winnerStage.Table)
		require.Equal(t, adaptiveExecutionP0DQueryLock, winnerStage.Operation)
		require.True(t, adaptiveExecutionP0DStageHasInt64(winnerStage, 20))
		cancellationResult = startAdaptiveExecutionP0DCall(
			ctx,
			"cancellation",
			func(callCtx context.Context) (any, error) {
				return (&threadRepository{db: state.mysql.dbB}).RequestRunCancellation(
					loserRecorder.context(callCtx),
					state.cancellation,
				)
			},
		)
		loserConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, loserRecorder, state.mysql.connectionIDB,
		)

	default:
		t.Fatalf("unsupported cancel/success direction %q", direction)
	}

	observeAdaptiveExecutionP0DExactRowWait(
		t,
		ctx,
		state.mysql,
		loserConnectionID,
		winnerConnectionID,
		adaptiveExecutionP0DPrimaryWait("agent_runs", 20),
	)
	winnerBarrier.releaseParticipant()
	results := []adaptiveExecutionMySQLRaceResult{
		waitAdaptiveExecutionP0DCall(t, ctx, finalizerResult),
		waitAdaptiveExecutionP0DCall(t, ctx, cancellationResult),
	}
	assertAdaptiveExecutionP0DCancelVsVerifiedSuccessPostState(t, state, direction, results)
}

func runAdaptiveExecutionP0DCrashAfterCommitRetry(t *testing.T) {
	t.Helper()
	state := prepareAdaptiveExecutionP0DLeaseFixture(t, adaptiveExecutionP0DGateOn, false)
	reconstructedBoundary := adaptiveExecutionP0DCrashBoundary(t)
	require.Equal(t, state.boundary.Authority, reconstructedBoundary.Authority)
	require.Equal(t, state.boundary.Checkpoint.ID, reconstructedBoundary.Checkpoint.ID)
	request, _ := adaptiveExecutionP0DCrashFinalizerRequest(t)
	assertAdaptiveExecutionP0DCrashArtifactsAbsent(t, state.mysql.dbA, request)

	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		executable,
		"-test.run=^TestAdaptiveExecutionP0DCrashHelper$",
	)
	command.Env = []string{
		canonicalMySQLTestDSNEnv + "=" + os.Getenv(canonicalMySQLTestDSNEnv),
		canonicalMySQLTestDDLGateEnv + "=" + os.Getenv(canonicalMySQLTestDDLGateEnv),
		adaptiveExecutionP0DCrashHelperEnv + "=1",
	}
	output, commandErr := command.CombinedOutput()
	require.Error(t, commandErr)
	require.NoError(t, ctx.Err(), "fixture failure: crash helper did not exit before its deadline")
	var exitErr *exec.ExitError
	require.ErrorAs(t, commandErr, &exitErr, "fixture failure: crash helper did not report an exit status")
	require.Equal(
		t,
		adaptiveExecutionP0DCrashHelperExitCode,
		exitErr.ExitCode(),
		"fixture failure: crash helper output: %s",
		string(output),
	)
	require.NotContains(t, string(output), adaptiveExecutionP0DCrashSuccessResponseMarker)

	assertAdaptiveExecutionP0DCrashTerminalTupleCounts(t, state.mysql.dbA, request, 1)
	committed := readAdaptiveExecutionP0DDurableSnapshot(t, state.mysql.dbA)
	assertAdaptiveExecutionP0DCrashDurableSnapshot(t, committed, request)
	expectedFirst := adaptiveExecutionP0DCrashResultFromSnapshot(t, committed, request, false)

	retryDB, retryConnectionID := openAdaptiveExecutionP0DExistingMySQL(t)
	require.NotEqual(t, state.mysql.connectionIDA, retryConnectionID)
	require.NotEqual(t, state.mysql.connectionIDB, retryConnectionID)
	require.NotEqual(t, state.mysql.observerConnectionID, retryConnectionID)
	retryRequest, retryTracker := adaptiveExecutionP0DCrashFinalizerRequest(t)
	assertAdaptiveExecutionP0DCrashRequestEqual(t, request, retryRequest)
	replayed, retryErr := (&threadRepository{db: retryDB}).FinalizeRunSuccess(
		context.Background(),
		retryRequest,
	)
	require.NoError(t, retryErr)
	require.NotNil(t, replayed)
	require.True(t, replayed.Replayed)
	require.Equal(t, []bool{false}, retryTracker.snapshot())
	assertAdaptiveExecutionP0DCrashTerminalTupleCounts(t, retryDB, retryRequest, 1)
	expectedReplay := *expectedFirst
	expectedReplay.Replayed = true
	require.Equal(t, &expectedReplay, replayed)

	afterRetry := readAdaptiveExecutionP0DDurableSnapshot(t, retryDB)
	require.Equal(t, committed, afterRetry)
	assertAdaptiveExecutionP0DCrashDurableSnapshot(t, afterRetry, retryRequest)
}

func adaptiveExecutionP0DCrashBoundary(t *testing.T) *CommitAdaptiveExecutionBoundaryResult {
	t.Helper()
	boundaryRequest := adaptiveExecutionMySQLRequest("p0d-decision", 7001, 8001, 1_000)
	boundaryRequest.Event.EventType = "adaptive.decision"
	boundaryRequest.Event.Payload =
		`{"schema":"workbench-adaptive-decision.v1","decision_id":"decision-1","decision_revision":1}`
	items := make([]*entity.AgentRunPlanItem, 0, len(boundaryRequest.PlanMutation.Items))
	for _, mutation := range boundaryRequest.PlanMutation.Items {
		require.NotNil(t, mutation.NextItem)
		item := *mutation.NextItem
		if mutation.ExpectedVersion > 0 {
			item.CreatedAt = 900
		} else {
			item.CreatedAt = boundaryRequest.Now
		}
		item.UpdatedAt = boundaryRequest.Now
		items = append(items, &item)
	}
	itemFingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	require.NoError(t, err)
	require.True(t, validAdaptiveExecutionFingerprint(itemFingerprint))
	return &CommitAdaptiveExecutionBoundaryResult{
		Event: boundaryRequest.Event, Checkpoint: boundaryRequest.Checkpoint,
		Plan: &entity.AgentRunPlan{
			RunID: 20, ThreadID: 10, SpaceID: 10, UserID: 20,
			HighWatermark: 2, Revision: 2, CreatedAt: 800, UpdatedAt: boundaryRequest.Now,
		},
		Items: items, LastCommittedSequence: 1,
		Authority: AdaptiveExecutionBoundaryAuthority{
			ThreadID: 10, ExecutionRunID: 20, ExecutionGeneration: 3,
			JournalRunID: 30, AttemptID: "attempt-1",
			EventID: 7001, EventSequence: 1, IdempotencyKey: boundaryRequest.IdempotencyKey,
			CheckpointID: 8001, PlanScopeRunID: 20, PlanRevision: 2,
			PlanItemFingerprint: itemFingerprint,
		},
	}
}

func adaptiveExecutionP0DCrashFinalizerRequest(
	t *testing.T,
) (FinalizeRunSuccessRequest, *adaptiveExecutionP0DOutboxTracker) {
	t.Helper()
	boundary := adaptiveExecutionP0DCrashBoundary(t)
	require.Len(t, boundary.Items, 1)
	mutated := *boundary.Items[0]
	currentItems := []*entity.AgentRunPlanItem{
		&mutated,
		{
			ID: 61, RunID: 20, TaskID: 2,
			Subject: "sentinel major step", Description: "sentinel description",
			Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "queued", Owner: "sentinel",
			Blocks: `[1]`, BlockedBy: `[2]`, Metadata: `{"sentinel":true}`,
			Active: true, Version: 7, CreatedAt: 850, UpdatedAt: 850,
		},
	}
	return adaptiveExecutionP0DFinalizerRequestWithItems(
		t,
		boundary,
		adaptiveExecutionP0DGateOn,
		currentItems,
	)
}

func assertAdaptiveExecutionP0DCrashRequestEqual(
	t *testing.T,
	expected FinalizeRunSuccessRequest,
	actual FinalizeRunSuccessRequest,
) {
	t.Helper()
	require.NotNil(t, expected.OutboxIntent)
	require.NotNil(t, actual.OutboxIntent)
	require.Equal(t, expected.OutboxIntent.Event, actual.OutboxIntent.Event)
	expected.OutboxIntent = nil
	actual.OutboxIntent = nil
	require.Equal(t, expected, actual)
}

func assertAdaptiveExecutionP0DCrashArtifactsAbsent(
	t *testing.T,
	db *gorm.DB,
	request FinalizeRunSuccessRequest,
) {
	t.Helper()
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &messagePO{}, "id = ?", request.Message.ID))
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &messagePO{}, "1 = 1"))
	for _, eventID := range []int64{
		request.TitleEvent.ID,
		request.AdaptiveGate.VerificationEvent.ID,
		request.CompletionEvent.ID,
	} {
		require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &runEventPO{}, "id = ?", eventID))
	}
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &runEventPO{}, "1 = 1"))
	require.Zero(t, adaptiveExecutionMySQLRowCount(
		t,
		db,
		&checkpointPO{},
		"id = ?",
		request.TerminalCheckpoint.ID,
	))
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &checkpointPO{}, "1 = 1"))
	require.Zero(t, adaptiveExecutionMySQLRowCount(
		t,
		db,
		&adaptiveExecutionP0DOutboxProbePO{},
		"idempotency_key = ?",
		request.OutboxIntent.Event.IdempotencyKey(),
	))
	assertAdaptiveExecutionP0DCrashTerminalTupleCounts(t, db, request, 0)
}

func assertAdaptiveExecutionP0DCrashTerminalTupleCounts(
	t *testing.T,
	db *gorm.DB,
	request FinalizeRunSuccessRequest,
	expected int64,
) {
	t.Helper()
	require.NotNil(t, request.Message)
	require.NotNil(t, request.TitleEvent)
	require.NotNil(t, request.CompletionEvent)
	require.NotNil(t, request.JournalEvent)
	require.NotNil(t, request.AdaptiveGate)
	require.NotNil(t, request.AdaptiveGate.VerificationEvent)
	require.NotNil(t, request.TerminalCheckpoint)
	require.NotNil(t, request.OutboxIntent)
	require.Equal(t, expected, adaptiveExecutionMySQLRowCount(
		t,
		db,
		&messagePO{},
		"id = ? AND thread_id = ? AND run_id = ? AND role = ? AND content = ? AND created_at = ?",
		request.Message.ID,
		request.Message.ThreadID,
		request.Message.RunID,
		string(request.Message.Role),
		request.Message.Content,
		request.Message.CreatedAt,
	))
	require.Equal(t, expected, adaptiveExecutionMySQLRowCount(
		t,
		db,
		&runEventPO{},
		"id = ? AND thread_id = ? AND run_id = ? AND journal_run_id IS NULL AND attempt_id IS NULL "+
			"AND sequence IS NULL AND idempotency_key IS NULL AND event_type = ? AND created_at = ?",
		request.TitleEvent.ID,
		request.TitleEvent.ThreadID,
		request.TitleEvent.RunID,
		request.TitleEvent.EventType,
		request.TitleEvent.CreatedAt,
	))
	verificationSequence := request.AdaptiveGate.Evidence.EventSequence + 1
	require.Equal(t, expected, adaptiveExecutionMySQLRowCount(
		t,
		db,
		&runEventPO{},
		"id = ? AND thread_id = ? AND run_id = ? AND journal_run_id = ? AND attempt_id = ? "+
			"AND sequence = ? AND idempotency_key = ? AND event_type = ? AND created_at = ?",
		request.AdaptiveGate.VerificationEvent.ID,
		request.AdaptiveGate.VerificationEvent.ThreadID,
		request.AdaptiveGate.VerificationEvent.RunID,
		request.AdaptiveGate.Evidence.JournalRunID,
		request.AdaptiveGate.Evidence.AttemptID,
		verificationSequence,
		request.AdaptiveGate.VerificationIdempotencyKey,
		request.AdaptiveGate.VerificationEvent.EventType,
		request.AdaptiveGate.VerificationEvent.CreatedAt,
	))
	require.Equal(t, expected, adaptiveExecutionMySQLRowCount(
		t,
		db,
		&runEventPO{},
		"id = ? AND thread_id = ? AND run_id = ? AND journal_run_id = ? AND attempt_id = ? "+
			"AND sequence = ? AND idempotency_key = ? AND event_type = ? AND created_at = ?",
		request.CompletionEvent.ID,
		request.CompletionEvent.ThreadID,
		request.CompletionEvent.RunID,
		request.JournalEvent.JournalRunID,
		request.JournalEvent.AttemptID,
		verificationSequence+1,
		request.JournalEvent.IdempotencyKey,
		request.CompletionEvent.EventType,
		request.CompletionEvent.CreatedAt,
	))
	require.Equal(t, expected, adaptiveExecutionMySQLRowCount(
		t,
		db,
		&checkpointPO{},
		"id = ? AND thread_id = ? AND run_id = ? AND parent_checkpoint_id = ? "+
			"AND checkpoint_ns = ? AND runtime_type = ? AND runtime_key = ? "+
			"AND envelope_version = ? AND runtime_deleted_at = ? AND created_at = ?",
		request.TerminalCheckpoint.ID,
		request.TerminalCheckpoint.ThreadID,
		request.TerminalCheckpoint.RunID,
		request.TerminalCheckpoint.ParentCheckpointID,
		request.TerminalCheckpoint.CheckpointNS,
		request.TerminalCheckpoint.RuntimeType,
		request.TerminalCheckpoint.RuntimeKey,
		request.TerminalCheckpoint.EnvelopeVersion,
		request.TerminalCheckpoint.RuntimeDeletedAt,
		request.TerminalCheckpoint.CreatedAt,
	))
	expectedOutbox, err := adaptiveExecutionP0DOutboxProbeRow(t, request.OutboxIntent.Event)
	require.NoError(t, err)
	require.Equal(t, expected, adaptiveExecutionMySQLRowCount(
		t,
		db,
		&adaptiveExecutionP0DOutboxProbePO{},
		"idempotency_key = ? AND event_id = ? AND fingerprint = ? AND payload = ? AND created_at = ?",
		expectedOutbox.IdempotencyKey,
		expectedOutbox.EventID,
		expectedOutbox.Fingerprint,
		expectedOutbox.Payload,
		expectedOutbox.CreatedAt,
	))
}

func readAdaptiveExecutionP0DDurableSnapshot(
	t *testing.T,
	db *gorm.DB,
) adaptiveExecutionP0DDurableSnapshot {
	t.Helper()
	var snapshot adaptiveExecutionP0DDurableSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Order("id ASC").Find(&snapshot.Threads).Error; err != nil {
			return err
		}
		if err := tx.Order("id ASC").Find(&snapshot.Runs).Error; err != nil {
			return err
		}
		if err := tx.Order("id ASC").Find(&snapshot.Attempts).Error; err != nil {
			return err
		}
		if err := tx.Order("id ASC").Find(&snapshot.Events).Error; err != nil {
			return err
		}
		if err := tx.Order("id ASC").Find(&snapshot.Messages).Error; err != nil {
			return err
		}
		if err := tx.Order("id ASC").Find(&snapshot.Checkpoints).Error; err != nil {
			return err
		}
		if err := tx.Order("run_id ASC").Find(&snapshot.Plans).Error; err != nil {
			return err
		}
		if err := tx.Order("run_id ASC").Order("task_id ASC").Find(&snapshot.Items).Error; err != nil {
			return err
		}
		return tx.Order("idempotency_key ASC").Find(&snapshot.Outbox).Error
	}))
	return snapshot
}

func assertAdaptiveExecutionP0DCrashDurableSnapshot(
	t *testing.T,
	snapshot adaptiveExecutionP0DDurableSnapshot,
	request FinalizeRunSuccessRequest,
) {
	t.Helper()
	require.Len(t, snapshot.Threads, 1)
	require.Equal(t, int64(10), snapshot.Threads[0].ID)
	require.Equal(t, strings.TrimSpace(request.ThreadTitle), snapshot.Threads[0].Title)
	require.Len(t, snapshot.Runs, 2)
	require.Equal(t, []int64{20, 30}, []int64{snapshot.Runs[0].ID, snapshot.Runs[1].ID})
	require.Equal(t, string(entity.RunStatusSucceeded), snapshot.Runs[0].Status)
	require.Equal(t, string(entity.RunStatusSucceeded), snapshot.Runs[1].Status)
	require.Len(t, snapshot.Attempts, 1)
	require.Equal(t, int64(100), snapshot.Attempts[0].ID)
	require.Equal(t, string(entity.RunAttemptStatusCompleted), snapshot.Attempts[0].Status)
	require.Nil(t, snapshot.Attempts[0].ActiveSlot)
	require.Equal(t, request.CompletionEvent.ID, requireInt64PointerForTest(t, snapshot.Attempts[0].TerminalEventID))
	require.Equal(t, uint64(4), snapshot.Attempts[0].NextSequence)
	require.Equal(t, uint64(2), snapshot.Attempts[0].LastCommittedSequence)
	require.Len(t, snapshot.Events, 4)
	require.Equal(t, []int64{7001, 7003, 7004, 7005}, []int64{
		snapshot.Events[0].ID,
		snapshot.Events[1].ID,
		snapshot.Events[2].ID,
		snapshot.Events[3].ID,
	})
	require.Len(t, snapshot.Messages, 1)
	require.Equal(t, request.Message.ID, snapshot.Messages[0].ID)
	require.Len(t, snapshot.Checkpoints, 2)
	require.Equal(t, []int64{8001, 8003}, []int64{
		snapshot.Checkpoints[0].ID,
		snapshot.Checkpoints[1].ID,
	})
	require.Equal(t, request.TerminalCheckpoint.ID, snapshot.Checkpoints[1].ID)
	require.Len(t, snapshot.Plans, 1)
	require.Equal(t, int64(20), snapshot.Plans[0].RunID)
	require.Equal(t, int64(2), snapshot.Plans[0].Revision)
	require.Len(t, snapshot.Items, 2)
	require.Equal(t, []int64{1, 2}, []int64{snapshot.Items[0].TaskID, snapshot.Items[1].TaskID})
	require.Len(t, snapshot.Outbox, 1)
	expectedOutbox, err := adaptiveExecutionP0DOutboxProbeRow(t, request.OutboxIntent.Event)
	require.NoError(t, err)
	require.Equal(t, expectedOutbox, snapshot.Outbox[0])
}

func adaptiveExecutionP0DCrashResultFromSnapshot(
	t *testing.T,
	snapshot adaptiveExecutionP0DDurableSnapshot,
	request FinalizeRunSuccessRequest,
	replayed bool,
) *FinalizeRunSuccessResult {
	t.Helper()
	assertAdaptiveExecutionP0DCrashDurableSnapshot(t, snapshot, request)
	return &FinalizeRunSuccessResult{
		Run:                snapshot.Runs[0].toEntity(),
		Message:            snapshot.Messages[0].toEntity(),
		CompletionEvent:    snapshot.Events[3].toEntity(),
		TerminalCheckpoint: snapshot.Checkpoints[1].toEntity(),
		TitleUpdated:       true,
		TitleEvent:         snapshot.Events[1].toEntity(),
		VerificationEvent:  snapshot.Events[2].toEntity(),
		Replayed:           replayed,
	}
}

func assertAdaptiveExecutionP0DCrashFirstResultIdentities(
	t *testing.T,
	request FinalizeRunSuccessRequest,
	result *FinalizeRunSuccessResult,
) {
	t.Helper()
	require.NotNil(t, result)
	require.False(t, result.Replayed)
	require.True(t, result.TitleUpdated)
	require.NotNil(t, result.Run)
	require.Equal(t, request.RunID, result.Run.ID)
	require.Equal(t, request.Message.ThreadID, result.Run.ThreadID)
	require.Equal(t, entity.RunStatusSucceeded, result.Run.Status)
	require.Empty(t, result.Run.LeaseOwner)
	require.Empty(t, result.Run.LeaseToken)
	require.Zero(t, result.Run.LeaseExpiresAt)

	require.NotNil(t, result.Message)
	expectedMessage := *request.Message
	actualMessage := *result.Message
	expectedMessage.Metadata = ""
	actualMessage.Metadata = ""
	require.Equal(t, expectedMessage, actualMessage)
	require.JSONEq(t, request.Message.Metadata, result.Message.Metadata)

	require.NotNil(t, result.TitleEvent)
	require.Equal(t, request.TitleEvent.ID, result.TitleEvent.ID)
	require.Equal(t, request.TitleEvent.ThreadID, result.TitleEvent.ThreadID)
	require.Equal(t, request.TitleEvent.RunID, result.TitleEvent.RunID)
	require.Equal(t, request.TitleEvent.EventType, result.TitleEvent.EventType)
	require.Equal(t, request.TitleEvent.CreatedAt, result.TitleEvent.CreatedAt)
	require.JSONEq(t, request.TitleEvent.Payload, result.TitleEvent.Payload)

	require.NotNil(t, result.CompletionEvent)
	require.Equal(t, request.CompletionEvent.ID, result.CompletionEvent.ID)
	require.Equal(t, request.CompletionEvent.ThreadID, result.CompletionEvent.ThreadID)
	require.Equal(t, request.CompletionEvent.RunID, result.CompletionEvent.RunID)
	require.Equal(t, request.CompletionEvent.EventType, result.CompletionEvent.EventType)
	require.Equal(t, request.CompletionEvent.CreatedAt, result.CompletionEvent.CreatedAt)
	require.JSONEq(t, request.CompletionEvent.Payload, result.CompletionEvent.Payload)

	require.NotNil(t, result.TerminalCheckpoint)
	expectedCheckpoint := *request.TerminalCheckpoint
	actualCheckpoint := *result.TerminalCheckpoint
	expectedCheckpoint.ChannelValues = ""
	expectedCheckpoint.ChannelVersions = ""
	expectedCheckpoint.PendingSends = ""
	expectedCheckpoint.Metadata = ""
	actualCheckpoint.ChannelValues = ""
	actualCheckpoint.ChannelVersions = ""
	actualCheckpoint.PendingSends = ""
	actualCheckpoint.Metadata = ""
	require.Equal(t, expectedCheckpoint, actualCheckpoint)
	require.JSONEq(t, request.TerminalCheckpoint.ChannelValues, result.TerminalCheckpoint.ChannelValues)
	require.JSONEq(t, request.TerminalCheckpoint.ChannelVersions, result.TerminalCheckpoint.ChannelVersions)
	require.JSONEq(t, request.TerminalCheckpoint.PendingSends, result.TerminalCheckpoint.PendingSends)
	require.JSONEq(t, request.TerminalCheckpoint.Metadata, result.TerminalCheckpoint.Metadata)

	require.NotNil(t, result.VerificationEvent)
	require.Equal(t, request.AdaptiveGate.VerificationEvent.ID, result.VerificationEvent.ID)
	require.Equal(t, request.AdaptiveGate.VerificationEvent.ThreadID, result.VerificationEvent.ThreadID)
	require.Equal(t, request.AdaptiveGate.VerificationEvent.RunID, result.VerificationEvent.RunID)
	require.Equal(t, request.AdaptiveGate.VerificationEvent.EventType, result.VerificationEvent.EventType)
	require.Equal(t, request.AdaptiveGate.VerificationEvent.CreatedAt, result.VerificationEvent.CreatedAt)
	callerPayload, err := decodeAdaptiveVerifiedSuccessPayload(
		request.AdaptiveGate.VerificationEvent.Payload,
		false,
	)
	require.NoError(t, err)
	durablePayload, err := decodeAdaptiveVerifiedSuccessPayload(result.VerificationEvent.Payload, true)
	require.NoError(t, err)
	require.True(t, adaptiveVerifiedSuccessCallerPayloadMatchesDurable(callerPayload, durablePayload))
	require.NotNil(t, durablePayload.OutboxFingerprint)
	require.True(t, validAdaptiveExecutionFingerprint(*durablePayload.OutboxFingerprint))
	require.True(t, validAdaptiveExecutionFingerprint(durablePayload.FinalizeRequestFingerprint))
}

func runAdaptiveExecutionP0DDeleteVsFinalizer(
	t *testing.T,
	gateMode adaptiveExecutionP0DGateMode,
	direction adaptiveExecutionP0DFinalizerDirection,
) {
	t.Helper()
	state := prepareAdaptiveExecutionP0DLeaseFixture(t, gateMode, true)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	deleteRepo := &threadRepository{db: state.mysql.dbB}
	deleteRequest := DeleteThreadIfIdleRequest{ThreadID: 10}

	var finalizerBarrier *adaptiveExecutionP0DLockBarrier
	var deleteBarrier *adaptiveExecutionP0DLockBarrier
	var finalizerResult, deleteResult <-chan adaptiveExecutionMySQLRaceResult
	var wait adaptiveExecutionP0DLockWaitRow
	var finalizerConnectionID, deleteConnectionID uint64
	switch direction {
	case adaptiveExecutionP0DCompetitorFirst:
		deleteBarrier = newAdaptiveExecutionP0DLockBarrier(adaptiveExecutionP0DThreadLockMatch)
		finalizerBarrier = newAdaptiveExecutionP0DLockBarrier(nil)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbB, deleteBarrier)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbA, finalizerBarrier)
		defer deleteBarrier.releaseParticipant()
		deleteResult = startAdaptiveExecutionP0DCall(
			ctx,
			"delete",
			func(callCtx context.Context) (any, error) {
				deleted, err := deleteRepo.DeleteThreadIfIdle(deleteBarrier.context(callCtx), deleteRequest)
				return deleted, err
			},
		)
		deleteConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, deleteBarrier, state.mysql.connectionIDB,
		)
		waitAdaptiveExecutionP0DLockAcquired(t, ctx, deleteBarrier, deleteConnectionID)
		finalizerResult = startAdaptiveExecutionP0DCall(
			ctx,
			"finalizer",
			func(callCtx context.Context) (any, error) {
				return state.mysql.repoA.FinalizeRunSuccess(
					finalizerBarrier.context(callCtx),
					state.finalizer,
				)
			},
		)
		finalizerConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, finalizerBarrier, state.mysql.connectionIDA,
		)
		wait = observeAdaptiveExecutionP0DExactRowWait(
			t,
			ctx,
			state.mysql,
			finalizerConnectionID,
			deleteConnectionID,
			adaptiveExecutionP0DPrimaryWait("agent_threads", 10),
		)
		deleteBarrier.releaseParticipant()

	case adaptiveExecutionP0DFinalizerFirst:
		finalizerBarrier = newAdaptiveExecutionP0DLockBarrier(
			adaptiveExecutionP0DFinalizerExecutionLockMatch(gateMode),
		)
		deleteBarrier = newAdaptiveExecutionP0DLockBarrier(nil)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbA, finalizerBarrier)
		registerAdaptiveExecutionP0DLockBarrier(t, state.mysql.dbB, deleteBarrier)
		defer finalizerBarrier.releaseParticipant()
		finalizerResult = startAdaptiveExecutionP0DCall(
			ctx,
			"finalizer",
			func(callCtx context.Context) (any, error) {
				return state.mysql.repoA.FinalizeRunSuccess(
					finalizerBarrier.context(callCtx),
					state.finalizer,
				)
			},
		)
		finalizerConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, finalizerBarrier, state.mysql.connectionIDA,
		)
		waitAdaptiveExecutionP0DLockAcquired(t, ctx, finalizerBarrier, finalizerConnectionID)
		first, ok := finalizerBarrier.firstStage()
		require.True(t, ok)
		deleteResult = startAdaptiveExecutionP0DCall(
			ctx,
			"delete",
			func(callCtx context.Context) (any, error) {
				deleted, err := deleteRepo.DeleteThreadIfIdle(deleteBarrier.context(callCtx), deleteRequest)
				return deleted, err
			},
		)
		deleteConnectionID = waitAdaptiveExecutionP0DTransactionConnection(
			t, ctx, deleteBarrier, state.mysql.connectionIDB,
		)
		expectedWait := adaptiveExecutionP0DPrimaryWait("agent_threads", 10)
		if first.Table != "agent_threads" {
			expectedWait = adaptiveExecutionP0DPrimaryWait("agent_runs", 20)
		}
		wait = observeAdaptiveExecutionP0DExactRowWait(
			t,
			ctx,
			state.mysql,
			deleteConnectionID,
			finalizerConnectionID,
			expectedWait,
		)
		finalizerBarrier.releaseParticipant()

	default:
		t.Fatalf("unsupported delete/finalizer direction %q", direction)
	}
	results := []adaptiveExecutionMySQLRaceResult{
		waitAdaptiveExecutionP0DCall(t, ctx, finalizerResult),
		waitAdaptiveExecutionP0DCall(t, ctx, deleteResult),
	}
	first, ok := finalizerBarrier.firstStage()
	require.True(t, ok, "fixture failure: finalizer did not record its first acquired lock")
	failAdaptiveExecutionP0DOnOldDeadlock(
		t,
		results,
		wait,
		first,
		adaptiveExecutionP0DOldFinalizerLockExpectation(gateMode),
		true,
	)
	require.Equal(t, "agent_threads", first.Table)
	require.Equal(t, adaptiveExecutionP0DQueryLock, first.Operation)
	require.True(t, adaptiveExecutionP0DStageHasInt64(first, 10))
	assertAdaptiveExecutionP0DDeleteVsFinalizerPostState(t, state, gateMode, results)
}

func assertAdaptiveExecutionP0DRecoveryVsFinalizerPostState(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	gateMode adaptiveExecutionP0DGateMode,
	direction adaptiveExecutionP0DFinalizerDirection,
	results []adaptiveExecutionMySQLRaceResult,
) {
	t.Helper()
	byName := adaptiveExecutionP0DResultsByName(t, results, "finalizer", "recovery")
	if direction == adaptiveExecutionP0DCompetitorFirst {
		require.Nil(t, byName["finalizer"].value)
		if gateMode == adaptiveExecutionP0DGateOn {
			require.ErrorIs(t, byName["finalizer"].err, ErrAdaptiveExecutionVerifiedSuccessConflict)
		}
		require.ErrorIs(t, byName["finalizer"].err, ErrRunLeaseLost)
		assertAdaptiveExecutionP0DRecoveryWinner(t, state, byName["recovery"])
		return
	}

	require.Nil(t, byName["recovery"].value)
	require.ErrorIs(t, byName["recovery"].err, ErrJournalInvalidStateTransition)
	assertAdaptiveExecutionP0DFinalizerWinner(t, state, gateMode, byName["finalizer"])
}

func assertAdaptiveExecutionP0DRecoveryVsCancellationPostState(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	direction adaptiveExecutionP0DRecoveryCancelDirection,
	results []adaptiveExecutionMySQLRaceResult,
) {
	t.Helper()
	byName := adaptiveExecutionP0DResultsByName(t, results, "recovery", "cancellation")
	if direction == adaptiveExecutionP0DRecoveryFirst {
		require.Nil(t, byName["cancellation"].value)
		require.Error(t, byName["cancellation"].err)
		require.Contains(t, byName["cancellation"].err.Error(), "cannot be canceled from status failed")
		assertAdaptiveExecutionP0DRecoveryWinner(t, state, byName["recovery"])
		return
	}

	require.Nil(t, byName["recovery"].value)
	require.ErrorIs(t, byName["recovery"].err, ErrJournalInvalidStateTransition)
	assertAdaptiveExecutionP0DCancellationWinner(t, state, byName["cancellation"])
}

func assertAdaptiveExecutionP0DCancelVsVerifiedSuccessPostState(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	direction adaptiveExecutionP0DCancelSuccessDirection,
	results []adaptiveExecutionMySQLRaceResult,
) {
	t.Helper()
	byName := adaptiveExecutionP0DResultsByName(t, results, "finalizer", "cancellation")
	winners := 0
	for _, result := range results {
		requireAdaptiveExecutionMySQLRaceError(t, result.err)
		if result.err == nil {
			winners++
		}
	}
	require.Equal(t, 1, winners)
	switch direction {
	case adaptiveExecutionP0DCancelWins:
		require.Nil(t, byName["finalizer"].value)
		require.ErrorIs(t, byName["finalizer"].err, ErrAdaptiveExecutionVerifiedSuccessConflict)
		require.ErrorIs(t, byName["finalizer"].err, ErrRunCanceled)
		assertAdaptiveExecutionP0DCancellationWinner(t, state, byName["cancellation"])
	case adaptiveExecutionP0DSuccessWins:
		require.Nil(t, byName["cancellation"].value)
		require.Error(t, byName["cancellation"].err)
		require.Contains(t, byName["cancellation"].err.Error(), "cannot be canceled from status succeeded")
		assertAdaptiveExecutionP0DFinalizerWinner(
			t,
			state,
			adaptiveExecutionP0DGateOn,
			byName["finalizer"],
		)
	default:
		t.Fatalf("unsupported cancel/success direction %q", direction)
	}
}

func assertAdaptiveExecutionP0DDeleteVsFinalizerPostState(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	gateMode adaptiveExecutionP0DGateMode,
	results []adaptiveExecutionMySQLRaceResult,
) {
	t.Helper()
	byName := adaptiveExecutionP0DResultsByName(t, results, "finalizer", "delete")
	require.ErrorIs(t, byName["delete"].err, ErrActiveRunExists)
	deleted, ok := byName["delete"].value.(bool)
	require.True(t, ok)
	require.False(t, deleted)
	assertAdaptiveExecutionP0DFinalizerWinner(t, state, gateMode, byName["finalizer"])
}

func adaptiveExecutionP0DResultsByName(
	t *testing.T,
	results []adaptiveExecutionMySQLRaceResult,
	names ...string,
) map[string]adaptiveExecutionMySQLRaceResult {
	t.Helper()
	require.Len(t, results, len(names))
	byName := make(map[string]adaptiveExecutionMySQLRaceResult, len(results))
	for _, result := range results {
		_, duplicate := byName[result.name]
		require.False(t, duplicate, "duplicate race result %q", result.name)
		byName[result.name] = result
	}
	for _, name := range names {
		_, found := byName[name]
		require.True(t, found, "missing race result %q", name)
	}
	return byName
}

func assertAdaptiveExecutionP0DFinalizerWinner(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	gateMode adaptiveExecutionP0DGateMode,
	race adaptiveExecutionMySQLRaceResult,
) {
	t.Helper()
	require.NoError(t, race.err)
	result, ok := race.value.(*FinalizeRunSuccessResult)
	require.True(t, ok)
	require.NotNil(t, result)
	require.False(t, result.Replayed)
	require.True(t, result.TitleUpdated)
	require.Equal(t, state.finalizer.Message, result.Message)
	require.Equal(t, state.finalizer.TitleEvent, result.TitleEvent)
	require.Equal(t, state.finalizer.CompletionEvent, result.CompletionEvent)
	require.Equal(t, state.finalizer.TerminalCheckpoint, result.TerminalCheckpoint)
	if gateMode == adaptiveExecutionP0DGateOn {
		require.NotNil(t, result.VerificationEvent)
		require.Equal(t, state.finalizer.AdaptiveGate.VerificationEvent.ID, result.VerificationEvent.ID)
		require.Equal(t, state.finalizer.AdaptiveGate.VerificationEvent.EventType, result.VerificationEvent.EventType)
	} else {
		require.Nil(t, result.VerificationEvent)
	}

	db := state.mysql.dbA
	assertAdaptiveExecutionP0DBaseAuthorityUnchanged(t, state)
	expectedThread := state.threadBefore
	expectedThread.Title = strings.TrimSpace(state.finalizer.ThreadTitle)
	expectedThread.UpdatedAt = state.finalizer.Now
	var thread threadPO
	require.NoError(t, db.Where("id = ?", 10).First(&thread).Error)
	require.Equal(t, expectedThread, thread)

	expectedRun := adaptiveExecutionP0DTerminalRun(
		state.runBefore,
		entity.RunStatusSucceeded,
		state.finalizer.Now,
		"",
		"",
		false,
	)
	var run runPO
	require.NoError(t, db.Where("id = ?", 20).First(&run).Error)
	assertAdaptiveExecutionP0DRunPOEqual(t, expectedRun, run)
	require.Equal(t, expectedRun.toEntity(), result.Run)

	expectedAttempt := state.attemptBefore
	expectedAttempt.Status = string(entity.RunAttemptStatusCompleted)
	expectedAttempt.ActiveSlot = nil
	expectedAttempt.TerminalEventID = adaptiveExecutionInt64Pointer(state.finalizer.CompletionEvent.ID)
	expectedAttempt.EndedAt = adaptiveExecutionInt64Pointer(state.finalizer.Now)
	expectedAttempt.UpdatedAt = state.finalizer.Now
	if gateMode == adaptiveExecutionP0DGateOn {
		expectedAttempt.NextSequence = state.attemptBefore.NextSequence + 2
		expectedAttempt.LastCommittedSequence = state.attemptBefore.NextSequence
	} else {
		expectedAttempt.NextSequence = state.attemptBefore.NextSequence + 1
	}
	var attempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, expectedAttempt, attempt)

	var messages []messagePO
	require.NoError(t, db.Order("id ASC").Find(&messages).Error)
	require.Len(t, messages, 1)
	expectedMessage, err := messageToPO(state.finalizer.Message)
	require.NoError(t, err)
	assertAdaptiveExecutionP0DMessagePOEqual(t, *expectedMessage, messages[0])

	var checkpoints []checkpointPO
	require.NoError(t, db.Order("id ASC").Find(&checkpoints).Error)
	require.Len(t, checkpoints, 2)
	require.Equal(t, state.boundaryCPBefore, checkpoints[0])
	expectedCheckpoint, err := checkpointToPO(state.finalizer.TerminalCheckpoint)
	require.NoError(t, err)
	assertAdaptiveExecutionP0DCheckpointPOEqual(t, *expectedCheckpoint, checkpoints[1])

	var events []runEventPO
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	expectedEventCount := 3
	if gateMode == adaptiveExecutionP0DGateOn {
		expectedEventCount = 4
	}
	require.Len(t, events, expectedEventCount)
	require.Equal(t, state.boundaryEventBefore, events[0])
	expectedTitle, err := runEventToPO(state.finalizer.TitleEvent)
	require.NoError(t, err)
	assertAdaptiveExecutionP0DRunEventPOEqual(t, *expectedTitle, events[1])
	if gateMode == adaptiveExecutionP0DGateOn {
		assertAdaptiveExecutionP0DVerificationEvent(t, state, events[2])
		expectedCompletion := adaptiveExecutionP0DGateOnCompletionPO(t, state)
		assertAdaptiveExecutionP0DRunEventPOEqual(t, expectedCompletion, events[3])
	} else {
		expectedCompletion := adaptiveExecutionP0DProjectedTerminalEventPO(
			t,
			state.finalizer.CompletionEvent,
			state.finalizer.JournalEvent,
			state.attemptBefore.NextSequence,
			entity.RunAttemptStatusCompleted,
		)
		assertAdaptiveExecutionP0DRunEventPOEqual(t, expectedCompletion, events[2])
	}

	assertAdaptiveExecutionP0DOutboxRows(t, db, state.finalizer.OutboxIntent.Event)
	require.Equal(t, []bool{true}, state.finalizerOutbox.snapshot())
	require.Empty(t, state.cancellationOutbox.snapshot())
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &runPO{}, "id = ?", 21))
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &runAttemptPO{}, "id = ?", 101))
	if state.sentinel == nil {
		require.Empty(t, adaptiveExecutionP0DActiveRunIDs(t, db))
		require.Equal(t, int64(2), adaptiveExecutionMySQLRowCount(t, db, &runPO{}, "1 = 1"))
	} else {
		var sentinel runPO
		require.NoError(t, db.Where("id = ?", state.sentinel.ID).First(&sentinel).Error)
		assertAdaptiveExecutionP0DRunPOEqual(t, *state.sentinel, sentinel)
		require.Equal(t, []int64{40}, adaptiveExecutionP0DActiveRunIDs(t, db))
		require.Equal(t, int64(3), adaptiveExecutionMySQLRowCount(t, db, &runPO{}, "1 = 1"))
	}
	assertAdaptiveExecutionP0DTableCounts(
		t,
		db,
		adaptiveExecutionMySQLRowCount(t, db, &runPO{}, "1 = 1"),
		1,
		1,
		int64(expectedEventCount),
		2,
		1,
	)
}

func assertAdaptiveExecutionP0DRecoveryWinner(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	race adaptiveExecutionMySQLRaceResult,
) {
	t.Helper()
	require.NoError(t, race.err)
	result, ok := race.value.(*CreateRunBundleResult)
	require.True(t, ok)
	require.NotNil(t, result)
	require.True(t, result.Created)
	require.NotNil(t, result.Run)
	require.NotNil(t, result.Attempt)
	require.Equal(t, int64(21), result.Run.ID)
	require.Equal(t, "attempt-2", result.Attempt.AttemptID)
	require.Empty(t, result.InterruptedRuns)
	require.Empty(t, result.InterruptedEvents)

	db := state.mysql.dbA
	assertAdaptiveExecutionP0DBaseAuthorityUnchanged(t, state)
	var thread threadPO
	require.NoError(t, db.Where("id = ?", 10).First(&thread).Error)
	require.Equal(t, state.threadBefore, thread)
	expectedSourceRun := adaptiveExecutionP0DTerminalRun(
		state.runBefore,
		entity.RunStatusFailed,
		state.recovery.RecoverySourceLease.Now,
		state.recovery.RecoverySourceLease.ErrorCode,
		state.recovery.RecoverySourceLease.ErrorMessage,
		false,
	)
	var sourceRun runPO
	require.NoError(t, db.Where("id = ?", 20).First(&sourceRun).Error)
	assertAdaptiveExecutionP0DRunPOEqual(t, expectedSourceRun, sourceRun)

	expectedSourceAttempt := state.attemptBefore
	expectedSourceAttempt.Status = string(entity.RunAttemptStatusFailed)
	expectedSourceAttempt.ActiveSlot = nil
	expectedSourceAttempt.NextSequence = state.attemptBefore.NextSequence + 1
	expectedSourceAttempt.TerminalEventID = adaptiveExecutionInt64Pointer(
		state.recovery.RecoverySourceLease.Event.ID,
	)
	expectedSourceAttempt.EndedAt = adaptiveExecutionInt64Pointer(state.recovery.RecoverySourceLease.Now)
	expectedSourceAttempt.UpdatedAt = state.recovery.RecoverySourceLease.Now
	var sourceAttempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&sourceAttempt).Error)
	require.Equal(t, expectedSourceAttempt, sourceAttempt)

	expectedRun, err := runToPO(state.recovery.Run)
	require.NoError(t, err)
	var recoveredRun runPO
	require.NoError(t, db.Where("id = ?", 21).First(&recoveredRun).Error)
	assertAdaptiveExecutionP0DRunPOEqual(t, *expectedRun, recoveredRun)
	resultRun, err := runToPO(result.Run)
	require.NoError(t, err)
	assertAdaptiveExecutionP0DRunPOEqual(t, recoveredRun, *resultRun)

	expectedAttemptEntity := *state.recovery.Attempt
	expectedAttemptEntity.Ordinal = state.attemptBefore.Ordinal + 1
	expectedAttemptEntity.EnrollmentVersion = state.attemptBefore.EnrollmentVersion
	expectedAttemptEntity.SnapshotsEnabled = state.attemptBefore.SnapshotsEnabled
	expectedAttemptEntity.ProjectionState = entity.JournalProjectionStateHealthy
	expectedAttemptEntity.ProjectionDegradedAt = nil
	activeSlot := uint8(1)
	expectedAttemptEntity.ActiveSlot = &activeSlot
	expectedAttemptEntity.NextSequence = 1
	expectedAttemptEntity.LastCommittedSequence = 0
	expectedAttemptEntity.TerminalEventID = nil
	expectedAttemptEntity.CreatedAt = state.recovery.Run.CreatedAt
	expectedAttemptEntity.UpdatedAt = state.recovery.Run.UpdatedAt
	expectedAttemptEntity.StartedAt = nil
	expectedAttemptEntity.EndedAt = nil
	expectedAttempt := runAttemptToPO(&expectedAttemptEntity)
	var recoveredAttempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 101).First(&recoveredAttempt).Error)
	require.Equal(t, *expectedAttempt, recoveredAttempt)
	require.Equal(t, recoveredAttempt.toEntity(), result.Attempt)

	var events []runEventPO
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, state.boundaryEventBefore, events[0])
	expectedTerminal := adaptiveExecutionP0DProjectedTerminalEventPO(
		t,
		state.recovery.RecoverySourceLease.Event,
		state.recovery.RecoverySourceLease.JournalEvent,
		state.attemptBefore.NextSequence,
		entity.RunAttemptStatusFailed,
	)
	assertAdaptiveExecutionP0DRunEventPOEqual(t, expectedTerminal, events[1])
	assertAdaptiveExecutionP0DNoFinalizerArtifacts(t, state, []int64{21}, 0)
	require.Empty(t, state.finalizerOutbox.snapshot())
	require.Empty(t, state.cancellationOutbox.snapshot())
	require.Equal(t, []int64{21}, adaptiveExecutionP0DActiveRunIDs(t, db))
	assertAdaptiveExecutionP0DTableCounts(t, db, 3, 2, 0, 2, 1, 0)
}

func assertAdaptiveExecutionP0DCancellationWinner(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	race adaptiveExecutionMySQLRaceResult,
) {
	t.Helper()
	require.NoError(t, race.err)
	result, ok := race.value.(*RequestRunCancellationResult)
	require.True(t, ok)
	require.NotNil(t, result)
	require.True(t, result.Changed)
	require.Equal(t, entity.RunStatusRunning, result.PreviousStatus)

	db := state.mysql.dbA
	assertAdaptiveExecutionP0DBaseAuthorityUnchanged(t, state)
	var thread threadPO
	require.NoError(t, db.Where("id = ?", 10).First(&thread).Error)
	require.Equal(t, state.threadBefore, thread)
	expectedRun := adaptiveExecutionP0DTerminalRun(
		state.runBefore,
		entity.RunStatusCanceled,
		state.cancellation.Now,
		state.cancellation.ErrorCode,
		state.cancellation.ErrorMessage,
		true,
	)
	expectedRun.ExecutionGeneration = state.runBefore.ExecutionGeneration + 1
	var run runPO
	require.NoError(t, db.Where("id = ?", 20).First(&run).Error)
	assertAdaptiveExecutionP0DRunPOEqual(t, expectedRun, run)
	require.Equal(t, run.toEntity(), result.Run)

	expectedAttempt := state.attemptBefore
	expectedAttempt.Status = string(entity.RunAttemptStatusCancelled)
	expectedAttempt.ActiveSlot = nil
	expectedAttempt.NextSequence = state.attemptBefore.NextSequence + 1
	expectedAttempt.TerminalEventID = adaptiveExecutionInt64Pointer(state.cancellation.Event.ID)
	expectedAttempt.EndedAt = adaptiveExecutionInt64Pointer(state.cancellation.Now)
	expectedAttempt.UpdatedAt = state.cancellation.Now
	var attempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, expectedAttempt, attempt)

	var events []runEventPO
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, state.boundaryEventBefore, events[0])
	expectedTerminal := adaptiveExecutionP0DProjectedTerminalEventPO(
		t,
		state.cancellation.Event,
		state.cancellation.JournalEvent,
		state.attemptBefore.NextSequence,
		entity.RunAttemptStatusCancelled,
	)
	assertAdaptiveExecutionP0DRunEventPOEqual(t, expectedTerminal, events[1])
	assertAdaptiveExecutionP0DNoFinalizerArtifacts(t, state, nil, 1)
	assertAdaptiveExecutionP0DOutboxRows(t, db, state.cancellation.OutboxIntent.Event)
	require.Empty(t, state.finalizerOutbox.snapshot())
	require.Equal(t, []bool{true}, state.cancellationOutbox.snapshot())
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &runPO{}, "id = ?", 21))
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &runAttemptPO{}, "id = ?", 101))
	require.Empty(t, adaptiveExecutionP0DActiveRunIDs(t, db))
	assertAdaptiveExecutionP0DTableCounts(t, db, 2, 1, 0, 2, 1, 1)
}

func assertAdaptiveExecutionP0DBaseAuthorityUnchanged(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
) {
	t.Helper()
	db := state.mysql.dbA
	var root runPO
	var plan agentRunPlanPO
	var items []agentRunPlanItemPO
	var boundaryEvent runEventPO
	var boundaryCheckpoint checkpointPO
	require.NoError(t, db.Where("id = ?", 30).First(&root).Error)
	require.NoError(t, db.Where("run_id = ?", 20).First(&plan).Error)
	require.NoError(t, db.Where("run_id = ?", 20).Order("task_id ASC").Find(&items).Error)
	require.NoError(t, db.Where("id = ?", state.boundary.Authority.EventID).First(&boundaryEvent).Error)
	require.NoError(t, db.Where("id = ?", state.boundary.Authority.CheckpointID).First(&boundaryCheckpoint).Error)
	assertAdaptiveExecutionP0DRunPOEqual(t, state.rootBefore, root)
	require.Equal(t, state.planBefore, plan)
	require.Equal(t, state.itemsBefore, items)
	require.Equal(t, state.boundaryEventBefore, boundaryEvent)
	require.Equal(t, state.boundaryCPBefore, boundaryCheckpoint)
}

func assertAdaptiveExecutionP0DNoFinalizerArtifacts(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	expectedActiveRunIDs []int64,
	expectedOutboxRows int64,
) {
	t.Helper()
	db := state.mysql.dbA
	require.Zero(t, adaptiveExecutionMySQLRowCount(t, db, &messagePO{}, "1 = 1"))
	require.Equal(t, int64(2), adaptiveExecutionMySQLRowCount(t, db, &runEventPO{}, "1 = 1"))
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &checkpointPO{}, "1 = 1"))
	require.Equal(t, expectedOutboxRows, adaptiveExecutionMySQLRowCount(
		t, db, &adaptiveExecutionP0DOutboxProbePO{}, "1 = 1",
	))
	activeRunIDs := adaptiveExecutionP0DActiveRunIDs(t, db)
	if len(expectedActiveRunIDs) == 0 {
		require.Empty(t, activeRunIDs)
	} else {
		require.Equal(t, expectedActiveRunIDs, activeRunIDs)
	}
}

func assertAdaptiveExecutionP0DTableCounts(
	t *testing.T,
	db *gorm.DB,
	runs int64,
	attempts int64,
	messages int64,
	events int64,
	checkpoints int64,
	outbox int64,
) {
	t.Helper()
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &threadPO{}, "1 = 1"))
	require.Equal(t, runs, adaptiveExecutionMySQLRowCount(t, db, &runPO{}, "1 = 1"))
	require.Equal(t, attempts, adaptiveExecutionMySQLRowCount(t, db, &runAttemptPO{}, "1 = 1"))
	require.Equal(t, messages, adaptiveExecutionMySQLRowCount(t, db, &messagePO{}, "1 = 1"))
	require.Equal(t, events, adaptiveExecutionMySQLRowCount(t, db, &runEventPO{}, "1 = 1"))
	require.Equal(t, checkpoints, adaptiveExecutionMySQLRowCount(t, db, &checkpointPO{}, "1 = 1"))
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &agentRunPlanPO{}, "1 = 1"))
	require.Equal(t, int64(2), adaptiveExecutionMySQLRowCount(t, db, &agentRunPlanItemPO{}, "1 = 1"))
	require.Equal(t, outbox, adaptiveExecutionMySQLRowCount(t, db, &adaptiveExecutionP0DOutboxProbePO{}, "1 = 1"))
}

func assertAdaptiveExecutionP0DOutboxRows(
	t *testing.T,
	db *gorm.DB,
	event domainnotification.Event,
) {
	t.Helper()
	var rows []adaptiveExecutionP0DOutboxProbePO
	require.NoError(t, db.Order("idempotency_key ASC").Find(&rows).Error)
	require.Len(t, rows, 1)
	expected, err := adaptiveExecutionP0DOutboxProbeRow(t, event)
	require.NoError(t, err)
	require.Equal(t, expected, rows[0])
}

func adaptiveExecutionP0DTerminalRun(
	before runPO,
	status entity.RunStatus,
	now int64,
	errorCode string,
	errorMessage string,
	canceled bool,
) runPO {
	before.Status = string(status)
	before.ErrorCode = strings.TrimSpace(errorCode)
	before.ErrorMessage = strings.TrimSpace(errorMessage)
	before.EndedAt = now
	before.UpdatedAt = now
	before.WorkerID = ""
	before.LeaseOwner = nil
	before.LeaseToken = nil
	before.LeaseExpiresAt = nil
	before.HeartbeatAt = nil
	before.CancelRequestedAt = nil
	if canceled {
		before.CancelRequestedAt = adaptiveExecutionInt64Pointer(now)
	}
	return before
}

func adaptiveExecutionP0DProjectedTerminalEventPO(
	t *testing.T,
	base *entity.RunEvent,
	journal *entity.JournalEvent,
	sequence uint64,
	status entity.RunAttemptStatus,
) runEventPO {
	t.Helper()
	require.NotNil(t, base)
	require.NotNil(t, journal)
	candidate := *journal
	candidate.ID = base.ID
	candidate.ThreadID = base.ThreadID
	candidate.RunID = base.RunID
	if candidate.CreatedAt <= 0 {
		candidate.CreatedAt = base.CreatedAt
	}
	normalized, err := normalizeJournalEvent(&candidate)
	require.NoError(t, err)
	require.NoError(t, validateTerminalJournalEvent(normalized, status))
	po, err := journalEventToPOWithBase(normalized, sequence, base)
	require.NoError(t, err)
	return *po
}

func adaptiveExecutionP0DGateOnCompletionPO(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
) runEventPO {
	t.Helper()
	messagePO, err := messageToPO(state.finalizer.Message)
	require.NoError(t, err)
	completionPO, err := runEventToPO(state.finalizer.CompletionEvent)
	require.NoError(t, err)
	normalized := adaptiveVerifiedSuccessNormalizedFinalize{
		now:     state.finalizer.Now,
		message: state.finalizer.Message, messagePO: messagePO,
		completionEvent: state.finalizer.CompletionEvent, completionEventPO: completionPO,
	}
	po, err := buildAdaptiveVerifiedSuccessCompletionPO(
		state.finalizer,
		normalized,
		&state.attemptBefore,
		state.attemptBefore.NextSequence+1,
	)
	require.NoError(t, err)
	return *po
}

func assertAdaptiveExecutionP0DVerificationEvent(
	t *testing.T,
	state adaptiveExecutionP0DLeaseFixture,
	actual runEventPO,
) {
	t.Helper()
	gate := state.finalizer.AdaptiveGate
	require.NotNil(t, gate)
	expected, err := runEventToPO(gate.VerificationEvent)
	require.NoError(t, err)
	expected.JournalRunID = adaptiveExecutionInt64Pointer(state.attemptBefore.JournalRunID)
	expected.AttemptID = adaptiveExecutionStringPointer(state.attemptBefore.AttemptID)
	expected.Sequence = adaptiveExecutionUint64Pointer(state.attemptBefore.NextSequence)
	expected.IdempotencyKey = adaptiveExecutionStringPointer(gate.VerificationIdempotencyKey)
	metadata := decodeAdaptiveExecutionMetadataForTest(t, state.boundaryCPBefore.Metadata)
	expected.SnapshotID = adaptiveExecutionStringPointer(metadata.CheckpointFingerprint)
	expected.Payload = actual.Payload
	assertAdaptiveExecutionP0DRunEventPOEqual(t, *expected, actual)
	callerPayload, err := decodeAdaptiveVerifiedSuccessPayload(gate.VerificationEvent.Payload, false)
	require.NoError(t, err)
	durablePayload, err := decodeAdaptiveVerifiedSuccessPayload(string(actual.Payload), true)
	require.NoError(t, err)
	require.True(t, adaptiveVerifiedSuccessCallerPayloadMatchesDurable(callerPayload, durablePayload))
	require.NotNil(t, durablePayload.OutboxFingerprint)
	require.True(t, validAdaptiveExecutionFingerprint(*durablePayload.OutboxFingerprint))
	require.True(t, validAdaptiveExecutionFingerprint(durablePayload.FinalizeRequestFingerprint))
}

func assertAdaptiveExecutionP0DRunPOEqual(t *testing.T, expected, actual runPO) {
	t.Helper()
	expectedJSON := []datatypes.JSON{
		expected.Command, expected.Input, expected.Config,
		expected.Context, expected.Metadata, expected.StreamMode,
	}
	actualJSON := []datatypes.JSON{
		actual.Command, actual.Input, actual.Config,
		actual.Context, actual.Metadata, actual.StreamMode,
	}
	expected.Command, expected.Input, expected.Config = nil, nil, nil
	expected.Context, expected.Metadata, expected.StreamMode = nil, nil, nil
	actual.Command, actual.Input, actual.Config = nil, nil, nil
	actual.Context, actual.Metadata, actual.StreamMode = nil, nil, nil
	require.Equal(t, expected, actual)
	for index := range expectedJSON {
		assertAdaptiveExecutionP0DJSONEqual(t, expectedJSON[index], actualJSON[index])
	}
}

func assertAdaptiveExecutionP0DMessagePOEqual(t *testing.T, expected, actual messagePO) {
	t.Helper()
	expectedMetadata, actualMetadata := expected.Metadata, actual.Metadata
	expected.Metadata, actual.Metadata = nil, nil
	require.Equal(t, expected, actual)
	assertAdaptiveExecutionP0DJSONEqual(t, expectedMetadata, actualMetadata)
}

func assertAdaptiveExecutionP0DCheckpointPOEqual(t *testing.T, expected, actual checkpointPO) {
	t.Helper()
	expectedJSON := []datatypes.JSON{
		expected.ChannelValues, expected.ChannelVersions, expected.PendingSends, expected.Metadata,
	}
	actualJSON := []datatypes.JSON{
		actual.ChannelValues, actual.ChannelVersions, actual.PendingSends, actual.Metadata,
	}
	expected.ChannelValues, expected.ChannelVersions, expected.PendingSends, expected.Metadata = nil, nil, nil, nil
	actual.ChannelValues, actual.ChannelVersions, actual.PendingSends, actual.Metadata = nil, nil, nil, nil
	require.Equal(t, expected, actual)
	for index := range expectedJSON {
		assertAdaptiveExecutionP0DJSONEqual(t, expectedJSON[index], actualJSON[index])
	}
}

func assertAdaptiveExecutionP0DRunEventPOEqual(t *testing.T, expected, actual runEventPO) {
	t.Helper()
	expectedPayload, expectedJournalPayload := expected.Payload, expected.JournalPayload
	actualPayload, actualJournalPayload := actual.Payload, actual.JournalPayload
	expected.Payload, expected.JournalPayload = nil, nil
	actual.Payload, actual.JournalPayload = nil, nil
	require.Equal(t, expected, actual)
	assertAdaptiveExecutionP0DJSONEqual(t, expectedPayload, actualPayload)
	assertAdaptiveExecutionP0DJSONEqual(t, expectedJournalPayload, actualJournalPayload)
}

func assertAdaptiveExecutionP0DJSONEqual(t *testing.T, expected, actual []byte) {
	t.Helper()
	if len(expected) == 0 || len(actual) == 0 {
		require.Equal(t, expected, actual)
		return
	}
	require.JSONEq(t, string(expected), string(actual))
}

func startAdaptiveExecutionP0DCall(
	ctx context.Context,
	name string,
	call func(context.Context) (any, error),
) <-chan adaptiveExecutionMySQLRaceResult {
	result := make(chan adaptiveExecutionMySQLRaceResult, 1)
	go func() {
		value, err := call(ctx)
		result <- adaptiveExecutionMySQLRaceResult{name: name, value: value, err: err}
	}()
	return result
}

func waitAdaptiveExecutionP0DCall(
	t *testing.T,
	ctx context.Context,
	result <-chan adaptiveExecutionMySQLRaceResult,
) adaptiveExecutionMySQLRaceResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		t.Fatalf("fixture failure: race participant did not finish: %v", ctx.Err())
		return adaptiveExecutionMySQLRaceResult{}
	}
}

func adaptiveExecutionP0DThreadLockMatch(stage adaptiveExecutionP0DLockStage) bool {
	return stage.Operation == adaptiveExecutionP0DQueryLock && stage.Table == "agent_threads" &&
		adaptiveExecutionP0DStageHasInt64(stage, 10)
}

func adaptiveExecutionP0DFinalizerExecutionLockMatch(
	gateMode adaptiveExecutionP0DGateMode,
) func(adaptiveExecutionP0DLockStage) bool {
	return func(stage adaptiveExecutionP0DLockStage) bool {
		if stage.Table != "agent_runs" || !adaptiveExecutionP0DStageHasInt64(stage, 20) {
			return false
		}
		if gateMode == adaptiveExecutionP0DGateOn {
			return stage.Operation == adaptiveExecutionP0DQueryLock
		}
		return stage.Operation == adaptiveExecutionP0DUpdateLock
	}
}

func adaptiveExecutionP0DRecoverySourceLockMatch(stage adaptiveExecutionP0DLockStage) bool {
	if stage.Operation != adaptiveExecutionP0DQueryLock {
		return false
	}
	if stage.Table == "agent_runs" && adaptiveExecutionP0DStageHasInt64(stage, 20) {
		return true
	}
	return stage.Table == "agent_run_attempts" &&
		adaptiveExecutionP0DStageHasInt64(stage, 30) &&
		adaptiveExecutionP0DStageHasString(stage, "attempt-1")
}

func adaptiveExecutionP0DCancellationRunLockMatch(stage adaptiveExecutionP0DLockStage) bool {
	return stage.Operation == adaptiveExecutionP0DQueryLock && stage.Table == "agent_runs" &&
		adaptiveExecutionP0DStageHasInt64(stage, 20)
}

type adaptiveExecutionP0DOldLockExpectation struct {
	table        string
	operation    adaptiveExecutionP0DLockOperation
	intValues    []int64
	stringValues []string
}

func adaptiveExecutionP0DOldFinalizerLockExpectation(
	gateMode adaptiveExecutionP0DGateMode,
) adaptiveExecutionP0DOldLockExpectation {
	if gateMode == adaptiveExecutionP0DGateOn {
		return adaptiveExecutionP0DOldLockExpectation{
			table: "agent_runs", operation: adaptiveExecutionP0DQueryLock, intValues: []int64{30},
		}
	}
	return adaptiveExecutionP0DOldLockExpectation{
		table: "agent_runs", operation: adaptiveExecutionP0DUpdateLock, intValues: []int64{20},
	}
}

func adaptiveExecutionP0DOldRecoverySourceLockExpectation() adaptiveExecutionP0DOldLockExpectation {
	return adaptiveExecutionP0DOldLockExpectation{
		table: "agent_run_attempts", operation: adaptiveExecutionP0DQueryLock,
		intValues: []int64{30}, stringValues: []string{"attempt-1"},
	}
}

func (expected adaptiveExecutionP0DOldLockExpectation) matches(stage adaptiveExecutionP0DLockStage) bool {
	if stage.Table != expected.table || stage.Operation != expected.operation ||
		!stage.ObservedAfterSQL || stage.ConnectionID == 0 {
		return false
	}
	for _, value := range expected.intValues {
		if !adaptiveExecutionP0DStageHasInt64(stage, value) {
			return false
		}
	}
	for _, value := range expected.stringValues {
		if !adaptiveExecutionP0DStageHasString(stage, value) {
			return false
		}
	}
	return true
}

func (expected adaptiveExecutionP0DOldLockExpectation) requireMatches(
	t *testing.T,
	stage adaptiveExecutionP0DLockStage,
) {
	t.Helper()
	require.Equal(t, expected.table, stage.Table)
	require.Equal(t, expected.operation, stage.Operation)
	require.True(t, stage.ObservedAfterSQL)
	require.NotZero(t, stage.ConnectionID)
	for _, value := range expected.intValues {
		require.True(t, adaptiveExecutionP0DStageHasInt64(stage, value))
	}
	for _, value := range expected.stringValues {
		require.True(t, adaptiveExecutionP0DStageHasString(stage, value))
	}
}

func failAdaptiveExecutionP0DOnOldDeadlock(
	t *testing.T,
	results []adaptiveExecutionMySQLRaceResult,
	wait adaptiveExecutionP0DLockWaitRow,
	first adaptiveExecutionP0DLockStage,
	expectedOldFirst adaptiveExecutionP0DOldLockExpectation,
	oldPrerequisiteObserved bool,
) {
	t.Helper()
	deadlockVictims := make([]string, 0, 1)
	for _, result := range results {
		if result.err == nil {
			continue
		}
		var mysqlErr *mysqldriver.MySQLError
		if errors.As(result.err, &mysqlErr) && mysqlErr.Number == 1213 {
			deadlockVictims = append(deadlockVictims, result.name)
			continue
		}
		requireAdaptiveExecutionMySQLRaceError(t, result.err)
	}
	if len(deadlockVictims) > 0 {
		require.Len(t, deadlockVictims, 1, "old-order RED requires exactly one MySQL 1213 victim")
		require.True(t, oldPrerequisiteObserved, "old-order RED prerequisite was not observed before release")
		expectedOldFirst.requireMatches(t, first)
		t.Fatalf(
			"P0D_OLD_LOCK_CYCLE_OBSERVED mysql=1213 victim=%s first_lock=%s wait=%s/%s/%s",
			deadlockVictims[0],
			first.Name,
			wait.ObjectName,
			wait.IndexName,
			wait.LockData,
		)
	}
}

type adaptiveExecutionP0DMySQLFixture struct {
	dbA                  *gorm.DB
	dbB                  *gorm.DB
	observer             *gorm.DB
	repoA                *threadRepository
	databaseName         string
	connectionIDA        uint64
	connectionIDB        uint64
	observerConnectionID uint64
}

type adaptiveExecutionP0DOutboxProbePO struct {
	IdempotencyKey string `gorm:"column:idempotency_key"`
	EventID        string `gorm:"column:event_id"`
	Fingerprint    string `gorm:"column:fingerprint"`
	Payload        []byte `gorm:"column:payload"`
	CreatedAt      int64  `gorm:"column:created_at"`
}

func (adaptiveExecutionP0DOutboxProbePO) TableName() string {
	return "p0d_adaptive_outbox_probe"
}

func newAdaptiveExecutionP0DMySQLFixture(t *testing.T) adaptiveExecutionP0DMySQLFixture {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(canonicalMySQLTestDSNEnv))
	if dsn == "" || os.Getenv(canonicalMySQLTestDDLGateEnv) != canonicalMySQLTestDDLGate {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("CI")), "true") {
			t.Fatalf(
				"CI requires %s and %s=%s",
				canonicalMySQLTestDSNEnv,
				canonicalMySQLTestDDLGateEnv,
				canonicalMySQLTestDDLGate,
			)
		}
		t.Skip("requires an explicitly gated disposable MySQL database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(config.DBName), "agentthread_disposable")

	openedConnections := make([]*gorm.DB, 0, 3)
	t.Cleanup(func() {
		for index := len(openedConnections) - 1; index >= 0; index-- {
			sqlDB, sqlErr := openedConnections[index].DB()
			require.NoError(t, sqlErr)
			require.NoError(t, sqlDB.Close())
		}
	})
	open := func() *gorm.DB {
		db, openErr := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		require.NoError(t, openErr)
		sqlDB, sqlErr := db.DB()
		require.NoError(t, sqlErr)
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		openedConnections = append(openedConnections, db)
		return db
	}
	dbA := open()
	dbB := open()
	observer := open()

	connectionIDA := assertAdaptiveExecutionP0DMySQLConnection(t, dbA, config.DBName)
	connectionIDB := assertAdaptiveExecutionP0DMySQLConnection(t, dbB, config.DBName)
	observerConnectionID := assertAdaptiveExecutionP0DMySQLConnection(t, observer, config.DBName)
	require.NotEqual(t, connectionIDA, connectionIDB)
	require.NotEqual(t, connectionIDA, observerConnectionID)
	require.NotEqual(t, connectionIDB, observerConnectionID)

	t.Cleanup(func() {
		require.NoError(t, dropAdaptiveExecutionMySQLTables(dbA))
	})
	require.NoError(t, resetAdaptiveExecutionMySQLSchema(dbA))
	return adaptiveExecutionP0DMySQLFixture{
		dbA: dbA, dbB: dbB, observer: observer, repoA: &threadRepository{db: dbA},
		databaseName: config.DBName, connectionIDA: connectionIDA,
		connectionIDB: connectionIDB, observerConnectionID: observerConnectionID,
	}
}

func openAdaptiveExecutionP0DExistingMySQL(t *testing.T) (*gorm.DB, uint64) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(canonicalMySQLTestDSNEnv))
	require.NotEmpty(t, dsn, "fixture failure: crash helper requires the dedicated MySQL DSN")
	require.Equal(
		t,
		canonicalMySQLTestDDLGate,
		os.Getenv(canonicalMySQLTestDDLGateEnv),
		"fixture failure: crash helper requires the disposable DDL gate",
	)
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(config.DBName), "agentthread_disposable")
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return db, assertAdaptiveExecutionP0DMySQLConnection(t, db, config.DBName)
}

func assertAdaptiveExecutionP0DMySQLConnection(t *testing.T, db *gorm.DB, databaseName string) uint64 {
	t.Helper()
	var selectedDatabase string
	require.NoError(t, db.Raw("SELECT DATABASE()").Scan(&selectedDatabase).Error)
	require.Equal(t, databaseName, selectedDatabase)
	var connectionID uint64
	require.NoError(t, db.Raw("SELECT CONNECTION_ID()").Scan(&connectionID).Error)
	require.NotZero(t, connectionID)
	return connectionID
}

func newAdaptiveExecutionP0DOutboxIntent(t *testing.T, now int64) *NotificationOutboxIntent {
	t.Helper()
	return newAdaptiveExecutionP0DOutboxIntentForEvent(
		t,
		adaptiveVerifiedSuccessOutboxEventForTest(now),
	)
}

func newAdaptiveExecutionP0DOutboxIntentForEvent(
	t *testing.T,
	event domainnotification.Event,
) *NotificationOutboxIntent {
	t.Helper()
	expected, err := adaptiveExecutionP0DOutboxProbeRow(t, event)
	require.NoError(t, err)
	appendWithResult := func(_ context.Context, tx *gorm.DB, actual domainnotification.Event) (bool, error) {
		if tx == nil {
			return false, fmt.Errorf("p0d outbox probe requires caller transaction")
		}
		actualRow, rowErr := adaptiveExecutionP0DOutboxProbeRow(t, actual)
		if rowErr != nil {
			return false, rowErr
		}
		if actualRow.IdempotencyKey != expected.IdempotencyKey ||
			actualRow.EventID != expected.EventID ||
			actualRow.Fingerprint != expected.Fingerprint ||
			string(actualRow.Payload) != string(expected.Payload) ||
			actualRow.CreatedAt != expected.CreatedAt {
			return false, fmt.Errorf("p0d outbox probe immutable identity mismatch")
		}
		insert := tx.Exec(
			"INSERT IGNORE INTO p0d_adaptive_outbox_probe "+
				"(idempotency_key, event_id, fingerprint, payload, created_at) VALUES (?, ?, ?, ?, ?)",
			expected.IdempotencyKey, expected.EventID, expected.Fingerprint, expected.Payload, expected.CreatedAt,
		)
		if insert.Error != nil {
			return false, insert.Error
		}
		if insert.RowsAffected != 0 && insert.RowsAffected != 1 {
			return false, fmt.Errorf("p0d outbox probe affected %d rows", insert.RowsAffected)
		}
		var stored adaptiveExecutionP0DOutboxProbePO
		if selectErr := tx.Raw(
			"SELECT idempotency_key, event_id, fingerprint, payload, created_at "+
				"FROM p0d_adaptive_outbox_probe WHERE idempotency_key = ?",
			expected.IdempotencyKey,
		).Scan(&stored).Error; selectErr != nil {
			return false, selectErr
		}
		if stored.IdempotencyKey != expected.IdempotencyKey ||
			stored.EventID != expected.EventID ||
			stored.Fingerprint != expected.Fingerprint ||
			string(stored.Payload) != string(expected.Payload) ||
			stored.CreatedAt != expected.CreatedAt {
			return false, fmt.Errorf("p0d outbox probe durable identity mismatch")
		}
		return insert.RowsAffected == 1, nil
	}
	return &NotificationOutboxIntent{
		Event:            event,
		AppendWithResult: appendWithResult,
		Append: func(ctx context.Context, tx *gorm.DB, actual domainnotification.Event) error {
			inserted, appendErr := appendWithResult(ctx, tx, actual)
			if appendErr != nil {
				return appendErr
			}
			if !inserted {
				return fmt.Errorf("p0d legacy outbox append requires first insert")
			}
			return nil
		},
	}
}

type adaptiveExecutionP0DOutboxTracker struct {
	mu       sync.Mutex
	inserted []bool
}

func newAdaptiveExecutionP0DTrackedOutboxIntent(
	t *testing.T,
	event domainnotification.Event,
) (*NotificationOutboxIntent, *adaptiveExecutionP0DOutboxTracker) {
	t.Helper()
	intent := newAdaptiveExecutionP0DOutboxIntentForEvent(t, event)
	tracker := &adaptiveExecutionP0DOutboxTracker{}
	appendWithResult := intent.AppendWithResult
	record := func(inserted bool) {
		tracker.mu.Lock()
		tracker.inserted = append(tracker.inserted, inserted)
		tracker.mu.Unlock()
	}
	intent.AppendWithResult = func(
		ctx context.Context,
		tx *gorm.DB,
		actual domainnotification.Event,
	) (bool, error) {
		inserted, err := appendWithResult(ctx, tx, actual)
		if err == nil {
			record(inserted)
		}
		return inserted, err
	}
	intent.Append = func(ctx context.Context, tx *gorm.DB, actual domainnotification.Event) error {
		inserted, err := appendWithResult(ctx, tx, actual)
		if err != nil {
			return err
		}
		record(inserted)
		if !inserted {
			return fmt.Errorf("p0d legacy outbox append requires first insert")
		}
		return nil
	}
	return intent, tracker
}

func (tracker *adaptiveExecutionP0DOutboxTracker) snapshot() []bool {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return append([]bool(nil), tracker.inserted...)
}

func adaptiveExecutionP0DOutboxProbeRow(
	t *testing.T,
	event domainnotification.Event,
) (adaptiveExecutionP0DOutboxProbePO, error) {
	t.Helper()
	canonical, err := domainnotification.CanonicalizeEvent(event)
	if err != nil {
		return adaptiveExecutionP0DOutboxProbePO{}, err
	}
	payload, err := json.Marshal(canonical.Payload)
	if err != nil {
		return adaptiveExecutionP0DOutboxProbePO{}, err
	}
	return adaptiveExecutionP0DOutboxProbePO{
		IdempotencyKey: canonical.IdempotencyKey(), EventID: canonical.EventID,
		Fingerprint: adaptiveVerifiedSuccessOutboxFingerprintOracleForTest(t, canonical),
		Payload:     payload, CreatedAt: canonical.OccurredAt.UnixMilli(),
	}, nil
}

func assertAdaptiveExecutionP0DVerifiedSuccessState(
	t *testing.T,
	db *gorm.DB,
	request FinalizeRunSuccessRequest,
	boundary *CommitAdaptiveExecutionBoundaryResult,
) {
	t.Helper()
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &threadPO{}, "1 = 1"))
	require.Equal(t, int64(2), adaptiveExecutionMySQLRowCount(t, db, &runPO{}, "1 = 1"))
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &runAttemptPO{}, "1 = 1"))
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &messagePO{}, "1 = 1"))
	require.Equal(t, int64(4), adaptiveExecutionMySQLRowCount(t, db, &runEventPO{}, "1 = 1"))
	require.Equal(t, int64(2), adaptiveExecutionMySQLRowCount(t, db, &checkpointPO{}, "1 = 1"))
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(t, db, &agentRunPlanPO{}, "1 = 1"))
	require.Equal(t, int64(2), adaptiveExecutionMySQLRowCount(t, db, &agentRunPlanItemPO{}, "1 = 1"))
	require.Equal(t, int64(1), adaptiveExecutionMySQLRowCount(
		t, db, &adaptiveExecutionP0DOutboxProbePO{}, "1 = 1",
	))
	var thread threadPO
	require.NoError(t, db.Where("id = ?", 10).First(&thread).Error)
	require.Equal(t, "adaptive fixture complete", thread.Title)
	require.Equal(t, int64(10), thread.SpaceID)
	require.Equal(t, int64(20), thread.CreatorID)
	require.Equal(t, string(entity.ThreadStatusRunning), thread.Status)
	require.JSONEq(t, `{}`, string(thread.Metadata))

	var root runPO
	require.NoError(t, db.Where("id = ?", 30).First(&root).Error)
	require.Equal(t, int64(10), root.ThreadID)
	require.Equal(t, int64(10), root.SpaceID)
	require.Equal(t, int64(20), root.CreatorID)
	require.Zero(t, root.ParentRunID)
	require.Equal(t, "agent", root.AssistantID)
	require.Equal(t, string(entity.RunKindTask), root.RunKind)
	require.Equal(t, string(entity.RunStatusSucceeded), root.Status)
	require.JSONEq(t, `{}`, string(root.Command))
	require.JSONEq(t, `{}`, string(root.Input))
	require.JSONEq(t, `{}`, string(root.Config))
	require.JSONEq(t, `{}`, string(root.Context))
	require.JSONEq(t, `{}`, string(root.Metadata))
	require.JSONEq(t, `[]`, string(root.StreamMode))
	require.Nil(t, root.LeaseOwner)
	require.Nil(t, root.LeaseToken)
	require.Nil(t, root.LeaseExpiresAt)
	require.Zero(t, root.ExecutionGeneration)
	require.Equal(t, int64(600), root.StartedAt)
	require.Equal(t, int64(600), root.EndedAt)

	var run runPO
	require.NoError(t, db.Where("id = ?", 20).First(&run).Error)
	require.Equal(t, int64(10), run.ThreadID)
	require.Equal(t, int64(10), run.SpaceID)
	require.Equal(t, int64(20), run.CreatorID)
	require.Equal(t, uint64(3), run.ExecutionGeneration)
	require.Equal(t, string(entity.RunStatusSucceeded), run.Status)
	require.Nil(t, run.LeaseOwner)
	require.Nil(t, run.LeaseToken)
	require.Nil(t, run.LeaseExpiresAt)
	require.Equal(t, request.Now, run.EndedAt)

	var attempt runAttemptPO
	require.NoError(t, db.Where("journal_run_id = ? AND attempt_id = ?", 30, "attempt-1").First(&attempt).Error)
	require.Equal(t, string(entity.RunAttemptStatusCompleted), attempt.Status)
	require.Equal(t, int64(20), attempt.ExecutionRunID)
	require.Equal(t, int64(30), attempt.JournalRunID)
	require.Equal(t, "attempt-1", attempt.AttemptID)
	require.Nil(t, attempt.ActiveSlot)
	require.Equal(t, boundary.Authority.EventSequence+3, attempt.NextSequence)
	require.Equal(t, boundary.Authority.EventSequence+1, attempt.LastCommittedSequence)
	require.Equal(t, request.CompletionEvent.ID, requireInt64PointerForTest(t, attempt.TerminalEventID))

	var events []runEventPO
	require.NoError(t, db.Where("run_id = ?", 20).Order("id ASC").Find(&events).Error)
	require.Len(t, events, 4)
	require.Equal(t, []int64{7001, 7003, 7004, 7005}, []int64{
		events[0].ID, events[1].ID, events[2].ID, events[3].ID,
	})
	require.Equal(t, []string{
		"adaptive.decision", "context.thread_title_updated", "adaptive.verification", "run.completed",
	}, []string{events[0].EventType, events[1].EventType, events[2].EventType, events[3].EventType})
	require.Equal(t, uint64(1), requireUint64PointerForTest(t, events[0].Sequence))
	require.Nil(t, events[1].Sequence)
	require.Equal(t, uint64(2), requireUint64PointerForTest(t, events[2].Sequence))
	require.Equal(t, uint64(3), requireUint64PointerForTest(t, events[3].Sequence))
	require.Equal(t, "verification-1", requireStringPointerForTest(t, events[2].IdempotencyKey))
	require.Equal(t, "completion-1", requireStringPointerForTest(t, events[3].IdempotencyKey))

	var messages []messagePO
	require.NoError(t, db.Where("run_id = ?", 20).Order("id ASC").Find(&messages).Error)
	require.Len(t, messages, 1)
	require.Equal(t, request.Message.ID, messages[0].ID)
	require.Equal(t, int64(10), messages[0].ThreadID)
	require.Equal(t, int64(20), messages[0].RunID)
	require.Equal(t, string(entity.MessageRoleAssistant), messages[0].Role)
	require.Equal(t, request.Message.Content, messages[0].Content)
	require.JSONEq(t, request.Message.Metadata, string(messages[0].Metadata))

	var checkpoints []checkpointPO
	require.NoError(t, db.Where("run_id = ?", 20).Order("id ASC").Find(&checkpoints).Error)
	require.Len(t, checkpoints, 2)
	require.Equal(t, []int64{boundary.Checkpoint.ID, request.TerminalCheckpoint.ID}, []int64{
		checkpoints[0].ID, checkpoints[1].ID,
	})
	require.Equal(t, boundary.Checkpoint.ID, checkpoints[1].ParentCheckpointID)

	var plan agentRunPlanPO
	require.NoError(t, db.Where("run_id = ?", 20).First(&plan).Error)
	require.Equal(t, boundary.Authority.PlanRevision, plan.Revision)
	require.Equal(t, int64(2), plan.HighWatermark)
	require.Equal(t, int64(10), plan.ThreadID)
	require.Equal(t, int64(10), plan.SpaceID)
	require.Equal(t, int64(20), plan.UserID)
	var items []agentRunPlanItemPO
	require.NoError(t, db.Where("run_id = ?", 20).Order("task_id ASC").Find(&items).Error)
	require.Len(t, items, 2)
	require.Equal(t, int64(1), items[0].TaskID)
	require.Equal(t, int64(2), items[1].TaskID)

	var outboxRows []adaptiveExecutionP0DOutboxProbePO
	require.NoError(t, db.Order("idempotency_key ASC").Find(&outboxRows).Error)
	require.Len(t, outboxRows, 1)
	expectedOutbox, err := adaptiveExecutionP0DOutboxProbeRow(t, request.OutboxIntent.Event)
	require.NoError(t, err)
	require.Equal(t, expectedOutbox, outboxRows[0])
}

type adaptiveExecutionP0DLockWaitRow struct {
	RequesterConnectionID uint64 `gorm:"column:requester_connection_id"`
	BlockerConnectionID   uint64 `gorm:"column:blocker_connection_id"`
	ObjectSchema          string `gorm:"column:object_schema"`
	ObjectName            string `gorm:"column:object_name"`
	IndexName             string `gorm:"column:index_name"`
	LockData              string `gorm:"column:lock_data"`
}

func assertAdaptiveExecutionP0DThreadRowWait(t *testing.T, fixture adaptiveExecutionP0DMySQLFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	holderAcquired := make(chan struct{})
	waiterIssued := make(chan struct{})
	releaseHolder := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseHolder) }) }
	defer release()
	holderResult := make(chan error, 1)
	waiterResult := make(chan error, 1)

	go func() {
		holderResult <- fixture.dbA.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var connectionID uint64
			if err := tx.Raw("SELECT CONNECTION_ID()").Scan(&connectionID).Error; err != nil {
				return err
			}
			if connectionID != fixture.connectionIDA {
				return fmt.Errorf("holder transaction connection changed")
			}
			var threadID int64
			if err := tx.Raw("SELECT id FROM agent_threads WHERE id = ? FOR UPDATE", 10).
				Row().Scan(&threadID); err != nil {
				return err
			}
			if threadID != 10 {
				return fmt.Errorf("holder locked unexpected thread %d", threadID)
			}
			close(holderAcquired)
			select {
			case <-releaseHolder:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	select {
	case <-holderAcquired:
	case err := <-holderResult:
		require.NoError(t, err)
		t.Fatal("holder exited before acquiring Thread row")
	case <-ctx.Done():
		t.Fatalf("holder did not acquire Thread row: %v", ctx.Err())
	}

	go func() {
		waiterResult <- fixture.dbB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var connectionID uint64
			if err := tx.Raw("SELECT CONNECTION_ID()").Scan(&connectionID).Error; err != nil {
				return err
			}
			if connectionID != fixture.connectionIDB {
				return fmt.Errorf("waiter transaction connection changed")
			}
			close(waiterIssued)
			var threadID int64
			if err := tx.Raw("SELECT id FROM agent_threads WHERE id = ? FOR UPDATE", 10).
				Row().Scan(&threadID); err != nil {
				return err
			}
			if threadID != 10 {
				return fmt.Errorf("waiter locked unexpected thread %d", threadID)
			}
			return nil
		})
	}()

	select {
	case <-waiterIssued:
	case err := <-waiterResult:
		require.NoError(t, err)
		t.Fatal("waiter exited before issuing the locking SQL")
	case <-ctx.Done():
		t.Fatalf("waiter did not issue locking SQL: %v", ctx.Err())
	}

	wait := observeAdaptiveExecutionP0DThreadRowWait(t, ctx, fixture)
	require.Equal(t, fixture.connectionIDB, wait.RequesterConnectionID)
	require.Equal(t, fixture.connectionIDA, wait.BlockerConnectionID)
	require.Equal(t, fixture.databaseName, wait.ObjectSchema)
	require.Equal(t, "agent_threads", wait.ObjectName)
	require.Equal(t, "PRIMARY", wait.IndexName)
	require.Equal(t, "10", wait.LockData)
	release()
	require.NoError(t, <-holderResult)
	require.NoError(t, <-waiterResult)
}

func observeAdaptiveExecutionP0DThreadRowWait(
	t *testing.T,
	ctx context.Context,
	fixture adaptiveExecutionP0DMySQLFixture,
) adaptiveExecutionP0DLockWaitRow {
	t.Helper()
	query := `SELECT
  waiting_thread.PROCESSLIST_ID AS requester_connection_id,
  blocking_thread.PROCESSLIST_ID AS blocker_connection_id,
  requested_lock.OBJECT_SCHEMA AS object_schema,
  requested_lock.OBJECT_NAME AS object_name,
  requested_lock.INDEX_NAME AS index_name,
  requested_lock.LOCK_DATA AS lock_data
FROM performance_schema.data_lock_waits AS waits
JOIN performance_schema.data_locks AS requested_lock
  ON requested_lock.ENGINE_LOCK_ID = waits.REQUESTING_ENGINE_LOCK_ID
JOIN performance_schema.data_locks AS blocking_lock
  ON blocking_lock.ENGINE_LOCK_ID = waits.BLOCKING_ENGINE_LOCK_ID
JOIN performance_schema.threads AS waiting_thread
  ON waiting_thread.THREAD_ID = requested_lock.THREAD_ID
JOIN performance_schema.threads AS blocking_thread
  ON blocking_thread.THREAD_ID = blocking_lock.THREAD_ID
WHERE waiting_thread.PROCESSLIST_ID = ?
  AND blocking_thread.PROCESSLIST_ID = ?
  AND requested_lock.OBJECT_SCHEMA = ?
  AND requested_lock.OBJECT_NAME = 'agent_threads'
  AND requested_lock.INDEX_NAME = 'PRIMARY'
  AND requested_lock.LOCK_DATA = '10'`
	for {
		var rows []adaptiveExecutionP0DLockWaitRow
		err := fixture.observer.WithContext(ctx).Raw(
			query, fixture.connectionIDB, fixture.connectionIDA, fixture.databaseName,
		).Scan(&rows).Error
		if err != nil {
			require.NoError(t, err)
		}
		if len(rows) == 1 {
			return rows[0]
		}
		require.Empty(t, rows)
		select {
		case <-ctx.Done():
			t.Fatalf("fixture failure: Thread row wait was not observable: %v", ctx.Err())
		default:
		}
	}
}

type adaptiveExecutionP0DLockOperation string

const (
	adaptiveExecutionP0DQueryLock  adaptiveExecutionP0DLockOperation = "select_for_update"
	adaptiveExecutionP0DUpdateLock adaptiveExecutionP0DLockOperation = "update"
)

type adaptiveExecutionP0DLockStage struct {
	Name             string
	Table            string
	Operation        adaptiveExecutionP0DLockOperation
	ConnectionID     uint64
	StatementSQL     string
	StatementVars    []any
	ObservedAfterSQL bool
}

type adaptiveExecutionP0DLockAcquisition struct {
	Stage adaptiveExecutionP0DLockStage
	Err   error
}

type adaptiveExecutionP0DConnectionObservation struct {
	ConnectionID uint64
	Err          error
}

type adaptiveExecutionP0DLockBarrierContextKey struct{}

type adaptiveExecutionP0DLockBarrier struct {
	mu             sync.Mutex
	first          *adaptiveExecutionP0DLockStage
	firstWatched   *adaptiveExecutionP0DLockStage
	watch          func(adaptiveExecutionP0DLockStage) bool
	match          func(adaptiveExecutionP0DLockStage) bool
	connection     chan adaptiveExecutionP0DConnectionObservation
	connectionID   uint64
	connectionErr  error
	connectionOnce sync.Once
	once           sync.Once
	release        chan struct{}
	released       sync.Once
	acquired       chan adaptiveExecutionP0DLockAcquisition
}

func newAdaptiveExecutionP0DLockBarrier(
	match func(adaptiveExecutionP0DLockStage) bool,
) *adaptiveExecutionP0DLockBarrier {
	return &adaptiveExecutionP0DLockBarrier{
		watch: match, match: match, release: make(chan struct{}),
		connection: make(chan adaptiveExecutionP0DConnectionObservation, 1),
		acquired:   make(chan adaptiveExecutionP0DLockAcquisition, 1),
	}
}

func newAdaptiveExecutionP0DLockRecorder(
	watch func(adaptiveExecutionP0DLockStage) bool,
) *adaptiveExecutionP0DLockBarrier {
	barrier := newAdaptiveExecutionP0DLockBarrier(nil)
	barrier.watch = watch
	return barrier
}

func (barrier *adaptiveExecutionP0DLockBarrier) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, adaptiveExecutionP0DLockBarrierContextKey{}, barrier)
}

func (barrier *adaptiveExecutionP0DLockBarrier) releaseParticipant() {
	barrier.released.Do(func() { close(barrier.release) })
}

func (barrier *adaptiveExecutionP0DLockBarrier) firstStage() (adaptiveExecutionP0DLockStage, bool) {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	if barrier.first == nil {
		return adaptiveExecutionP0DLockStage{}, false
	}
	return *barrier.first, true
}

func (barrier *adaptiveExecutionP0DLockBarrier) firstWatchedStage() (adaptiveExecutionP0DLockStage, bool) {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	if barrier.firstWatched == nil {
		return adaptiveExecutionP0DLockStage{}, false
	}
	return *barrier.firstWatched, true
}

func (barrier *adaptiveExecutionP0DLockBarrier) recordConnection(tx *gorm.DB) {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil ||
		tx.Statement.Context.Value(adaptiveExecutionP0DLockBarrierContextKey{}) != barrier {
		return
	}
	barrier.connectionOnce.Do(func() {
		var connectionID uint64
		err := tx.Statement.ConnPool.QueryRowContext(
			tx.Statement.Context,
			"SELECT CONNECTION_ID()",
		).Scan(&connectionID)
		barrier.mu.Lock()
		barrier.connectionID = connectionID
		barrier.connectionErr = err
		barrier.mu.Unlock()
		barrier.connection <- adaptiveExecutionP0DConnectionObservation{
			ConnectionID: connectionID,
			Err:          err,
		}
		if err != nil {
			tx.AddError(err)
		}
	})
}

func (barrier *adaptiveExecutionP0DLockBarrier) recordedConnection() (uint64, error, bool) {
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	if barrier.connectionID == 0 && barrier.connectionErr == nil {
		return 0, nil, false
	}
	return barrier.connectionID, barrier.connectionErr, true
}

func (barrier *adaptiveExecutionP0DLockBarrier) observe(
	tx *gorm.DB,
	operation adaptiveExecutionP0DLockOperation,
) {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil ||
		tx.Statement.Context.Value(adaptiveExecutionP0DLockBarrierContextKey{}) != barrier {
		return
	}
	stage, ok := adaptiveExecutionP0DLockStageFromStatement(tx, operation)
	if !ok {
		return
	}
	matched := barrier.match != nil && barrier.match(stage)
	if tx.Error != nil || tx.RowsAffected != 1 {
		if matched {
			barrier.once.Do(func() {
				err := tx.Error
				if err == nil {
					err = fmt.Errorf(
						"fixture failure: matched lock SQL affected %d rows instead of one",
						tx.RowsAffected,
					)
				}
				barrier.acquired <- adaptiveExecutionP0DLockAcquisition{Stage: stage, Err: err}
			})
		}
		return
	}
	connectionID, connectionErr, recorded := barrier.recordedConnection()
	if !recorded {
		tx.AddError(fmt.Errorf("fixture failure: transaction connection was not recorded before lock observation"))
		return
	}
	if connectionErr != nil {
		tx.AddError(connectionErr)
		return
	}
	stage.ConnectionID = connectionID
	barrier.mu.Lock()
	if barrier.first == nil {
		first := stage
		barrier.first = &first
	}
	if barrier.firstWatched == nil && barrier.watch != nil && barrier.watch(stage) {
		firstWatched := stage
		barrier.firstWatched = &firstWatched
	}
	barrier.mu.Unlock()
	if !matched {
		return
	}
	barrier.once.Do(func() {
		barrier.acquired <- adaptiveExecutionP0DLockAcquisition{Stage: stage}
		select {
		case <-barrier.release:
		case <-tx.Statement.Context.Done():
			tx.AddError(tx.Statement.Context.Err())
		}
	})
}

func adaptiveExecutionP0DLockStageFromStatement(
	tx *gorm.DB,
	operation adaptiveExecutionP0DLockOperation,
) (adaptiveExecutionP0DLockStage, bool) {
	statementSQL := tx.Statement.SQL.String()
	upperSQL := strings.ToUpper(statementSQL)
	switch operation {
	case adaptiveExecutionP0DQueryLock:
		if !strings.Contains(upperSQL, "FOR UPDATE") {
			return adaptiveExecutionP0DLockStage{}, false
		}
	case adaptiveExecutionP0DUpdateLock:
		if !strings.HasPrefix(strings.TrimSpace(upperSQL), "UPDATE") {
			return adaptiveExecutionP0DLockStage{}, false
		}
	default:
		return adaptiveExecutionP0DLockStage{}, false
	}
	table := tx.Statement.Table
	name := table + "_" + string(operation)
	return adaptiveExecutionP0DLockStage{
		Name: name, Table: table, Operation: operation,
		StatementSQL: statementSQL, StatementVars: append([]any(nil), tx.Statement.Vars...),
		ObservedAfterSQL: true,
	}, true
}

func registerAdaptiveExecutionP0DLockBarrier(
	t *testing.T,
	db *gorm.DB,
	barrier *adaptiveExecutionP0DLockBarrier,
) {
	t.Helper()
	require.NotNil(t, db)
	require.NotNil(t, barrier)
	queryName := fmt.Sprintf("p0d:lock-barrier:%p:query", barrier)
	updateName := fmt.Sprintf("p0d:lock-barrier:%p:update", barrier)
	queryConnectionName := fmt.Sprintf("p0d:connection-recorder:%p:query", barrier)
	updateConnectionName := fmt.Sprintf("p0d:connection-recorder:%p:update", barrier)
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(
		queryConnectionName,
		func(tx *gorm.DB) { barrier.recordConnection(tx) },
	))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(
		updateConnectionName,
		func(tx *gorm.DB) { barrier.recordConnection(tx) },
	))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(
		queryName,
		func(tx *gorm.DB) { barrier.observe(tx, adaptiveExecutionP0DQueryLock) },
	))
	require.NoError(t, db.Callback().Update().After("gorm:update").Register(
		updateName,
		func(tx *gorm.DB) { barrier.observe(tx, adaptiveExecutionP0DUpdateLock) },
	))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Update().Remove(updateName))
		require.NoError(t, db.Callback().Query().Remove(queryName))
		require.NoError(t, db.Callback().Update().Remove(updateConnectionName))
		require.NoError(t, db.Callback().Query().Remove(queryConnectionName))
	})
}

func waitAdaptiveExecutionP0DTransactionConnection(
	t *testing.T,
	ctx context.Context,
	barrier *adaptiveExecutionP0DLockBarrier,
	expectedConnectionID uint64,
) uint64 {
	t.Helper()
	select {
	case observation := <-barrier.connection:
		require.NoError(t, observation.Err)
		require.NotZero(t, observation.ConnectionID)
		require.Equal(t, expectedConnectionID, observation.ConnectionID)
		return observation.ConnectionID
	case <-ctx.Done():
		t.Fatalf("fixture failure: participant transaction connection was not recorded: %v", ctx.Err())
		return 0
	}
}

func waitAdaptiveExecutionP0DLockAcquired(
	t *testing.T,
	ctx context.Context,
	barrier *adaptiveExecutionP0DLockBarrier,
	expectedConnectionID uint64,
) adaptiveExecutionP0DLockStage {
	t.Helper()
	select {
	case acquisition := <-barrier.acquired:
		require.NoError(t, acquisition.Err)
		require.NotZero(t, acquisition.Stage.ConnectionID)
		require.Equal(t, expectedConnectionID, acquisition.Stage.ConnectionID)
		require.True(t, acquisition.Stage.ObservedAfterSQL)
		return acquisition.Stage
	case <-ctx.Done():
		t.Fatalf("fixture failure: lock barrier was not acquired: %v", ctx.Err())
		return adaptiveExecutionP0DLockStage{}
	}
}

func adaptiveExecutionP0DStageHasInt64(stage adaptiveExecutionP0DLockStage, expected int64) bool {
	for _, value := range stage.StatementVars {
		switch typed := value.(type) {
		case int:
			if int64(typed) == expected {
				return true
			}
		case int64:
			if typed == expected {
				return true
			}
		case uint:
			if uint64(typed) == uint64(expected) {
				return true
			}
		case uint64:
			if typed == uint64(expected) {
				return true
			}
		}
	}
	return false
}

func adaptiveExecutionP0DStageHasString(stage adaptiveExecutionP0DLockStage, expected string) bool {
	for _, value := range stage.StatementVars {
		if typed, ok := value.(string); ok && typed == expected {
			return true
		}
	}
	return false
}

type adaptiveExecutionP0DExpectedWait struct {
	ObjectName      string
	IndexName       string
	AllowedLockData []string
}

func adaptiveExecutionP0DPrimaryWait(table string, id int64) adaptiveExecutionP0DExpectedWait {
	return adaptiveExecutionP0DExpectedWait{
		ObjectName: table, IndexName: "PRIMARY", AllowedLockData: []string{fmt.Sprint(id)},
	}
}

func adaptiveExecutionP0DAttemptWait() adaptiveExecutionP0DExpectedWait {
	return adaptiveExecutionP0DExpectedWait{
		ObjectName: "agent_run_attempts", IndexName: "PRIMARY",
		AllowedLockData: []string{"100"},
	}
}

func observeAdaptiveExecutionP0DExactRowWait(
	t *testing.T,
	ctx context.Context,
	fixture adaptiveExecutionP0DMySQLFixture,
	requesterConnectionID uint64,
	blockerConnectionID uint64,
	expected adaptiveExecutionP0DExpectedWait,
) adaptiveExecutionP0DLockWaitRow {
	t.Helper()
	query := `SELECT
  waiting_thread.PROCESSLIST_ID AS requester_connection_id,
  blocking_thread.PROCESSLIST_ID AS blocker_connection_id,
  requested_lock.OBJECT_SCHEMA AS object_schema,
  requested_lock.OBJECT_NAME AS object_name,
  requested_lock.INDEX_NAME AS index_name,
  requested_lock.LOCK_DATA AS lock_data
FROM performance_schema.data_lock_waits AS waits
JOIN performance_schema.data_locks AS requested_lock
  ON requested_lock.ENGINE_LOCK_ID = waits.REQUESTING_ENGINE_LOCK_ID
JOIN performance_schema.data_locks AS blocking_lock
  ON blocking_lock.ENGINE_LOCK_ID = waits.BLOCKING_ENGINE_LOCK_ID
JOIN performance_schema.threads AS waiting_thread
  ON waiting_thread.THREAD_ID = requested_lock.THREAD_ID
JOIN performance_schema.threads AS blocking_thread
  ON blocking_thread.THREAD_ID = blocking_lock.THREAD_ID
WHERE waiting_thread.PROCESSLIST_ID = ?
  AND blocking_thread.PROCESSLIST_ID = ?
  AND requested_lock.OBJECT_SCHEMA = ?
  AND requested_lock.OBJECT_NAME = ?`
	for {
		var rows []adaptiveExecutionP0DLockWaitRow
		err := fixture.observer.WithContext(ctx).Raw(
			query,
			requesterConnectionID,
			blockerConnectionID,
			fixture.databaseName,
			expected.ObjectName,
		).Scan(&rows).Error
		if err != nil {
			t.Fatalf("fixture failure: observe server row wait: %v", err)
		}
		if len(rows) > 0 {
			require.Len(t, rows, 1, "fixture failure: expected one exact server wait")
			row := rows[0]
			require.Equal(t, requesterConnectionID, row.RequesterConnectionID)
			require.Equal(t, blockerConnectionID, row.BlockerConnectionID)
			require.Equal(t, fixture.databaseName, row.ObjectSchema)
			require.Equal(t, expected.ObjectName, row.ObjectName)
			require.Equal(t, expected.IndexName, row.IndexName)
			require.Contains(t, expected.AllowedLockData, row.LockData)
			return row
		}
		select {
		case <-ctx.Done():
			t.Fatalf(
				"fixture failure: %s row wait was not observable: %v",
				expected.ObjectName,
				ctx.Err(),
			)
		default:
		}
	}
}

type adaptiveExecutionMySQLRaceCall struct {
	name string
	call func(context.Context) (any, error)
}

type adaptiveExecutionMySQLRaceResult struct {
	name  string
	value any
	err   error
}

func runAdaptiveExecutionMySQLRace(
	t *testing.T,
	calls []adaptiveExecutionMySQLRaceCall,
) []adaptiveExecutionMySQLRaceResult {
	t.Helper()
	require.Len(t, calls, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan adaptiveExecutionMySQLRaceResult, 2)
	for _, raceCall := range calls {
		raceCall := raceCall
		go func() {
			ready <- struct{}{}
			select {
			case <-start:
			case <-ctx.Done():
				results <- adaptiveExecutionMySQLRaceResult{name: raceCall.name, err: ctx.Err()}
				return
			}
			value, err := raceCall.call(ctx)
			results <- adaptiveExecutionMySQLRaceResult{name: raceCall.name, value: value, err: err}
		}()
	}
	for range calls {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatalf("race participants did not reach the start barrier: %v", ctx.Err())
		}
	}
	close(start)

	collected := make([]adaptiveExecutionMySQLRaceResult, 0, len(calls))
	for range calls {
		select {
		case result := <-results:
			collected = append(collected, result)
		case <-ctx.Done():
			t.Fatalf("race did not complete before the bounded deadline: %v", ctx.Err())
		}
	}
	return collected
}

func requireAdaptiveExecutionMySQLRaceError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	require.NotErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, context.Canceled)
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) {
		require.NotEqual(t, uint16(1213), mysqlErr.Number, "deadlock is not a legal race result")
		require.NotEqual(t, uint16(1205), mysqlErr.Number, "lock timeout is not a legal race result")
		require.NotEqual(t, uint16(1062), mysqlErr.Number, "raw duplicate is not a legal race result")
	}
	lower := strings.ToLower(err.Error())
	for _, forbidden := range []string{
		"panic",
		"unknown column",
		"doesn't exist",
		"does not exist",
		"no such table",
		"fixture failure",
		"build failed",
		"no tests to run",
		"context deadline",
		"deadline exceeded",
		"context canceled",
		"context cancelled",
		"1213",
		"1205",
		"1062",
		"deadlock",
		"lock wait timeout",
		"duplicate entry",
	} {
		require.NotContains(
			t,
			lower,
			forbidden,
			"race participant error contains forbidden marker %q",
			forbidden,
		)
	}
}

type adaptiveExecutionMySQLState struct {
	run         runPO
	attempt     runAttemptPO
	plan        agentRunPlanPO
	items       []agentRunPlanItemPO
	events      []runEventPO
	checkpoints []checkpointPO
}

func readAdaptiveExecutionMySQLState(t *testing.T, db *gorm.DB) adaptiveExecutionMySQLState {
	t.Helper()
	var state adaptiveExecutionMySQLState
	require.NoError(t, db.Where("id = ?", 20).First(&state.run).Error)
	require.NoError(t, db.Where("journal_run_id = ? AND attempt_id = ?", 30, "attempt-1").First(&state.attempt).Error)
	require.NoError(t, db.Where("run_id = ?", 20).First(&state.plan).Error)
	require.NoError(t, db.Where("run_id = ?", 20).Order("task_id ASC").Find(&state.items).Error)
	require.NoError(t, db.Where("run_id = ?", 20).Order("id ASC").Find(&state.events).Error)
	require.NoError(t, db.Where("run_id = ?", 20).Order("id ASC").Find(&state.checkpoints).Error)
	return state
}

func adaptiveExecutionMySQLRowCount(
	t *testing.T,
	db *gorm.DB,
	model any,
	query string,
	args ...any,
) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(model).Where(query, args...).Count(&count).Error)
	return count
}

func assertAdaptiveExecutionMySQLRunUnchanged(t *testing.T, run runPO) {
	t.Helper()
	require.Equal(t, int64(20), run.ID)
	require.Equal(t, int64(10), run.ThreadID)
	require.Equal(t, int64(10), run.SpaceID)
	require.Equal(t, int64(20), run.CreatorID)
	require.Equal(t, string(entity.RunStatusRunning), run.Status)
	require.Equal(t, uint64(3), run.ExecutionGeneration)
	require.Equal(t, "worker-1", requireStringPointerForTest(t, run.LeaseOwner))
	require.Equal(t, "lease-1", requireStringPointerForTest(t, run.LeaseToken))
}

func assertAdaptiveExecutionMySQLSentinelUnchanged(t *testing.T, item agentRunPlanItemPO) {
	t.Helper()
	assertAdaptiveExecutionMySQLPlanItemEqual(t, &entity.AgentRunPlanItem{
		ID: 61, RunID: 20, TaskID: 2,
		Subject: "sentinel major step", Description: "sentinel description",
		Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "queued", Owner: "sentinel",
		Blocks: `[1]`, BlockedBy: `[2]`, Metadata: `{"sentinel":true}`,
		Active: true, Version: 7, CreatedAt: 850, UpdatedAt: 850,
	}, item.toEntity())
}

func assertAdaptiveExecutionMySQLPlanItemEqual(
	t *testing.T,
	expected *entity.AgentRunPlanItem,
	actual *entity.AgentRunPlanItem,
) {
	t.Helper()
	require.NotNil(t, expected)
	require.NotNil(t, actual)
	expectedScalar := *expected
	actualScalar := *actual
	expectedScalar.Blocks = ""
	expectedScalar.BlockedBy = ""
	expectedScalar.Metadata = ""
	actualScalar.Blocks = ""
	actualScalar.BlockedBy = ""
	actualScalar.Metadata = ""
	require.Equal(t, expectedScalar, actualScalar)
	require.JSONEq(t, expected.Blocks, actual.Blocks)
	require.JSONEq(t, expected.BlockedBy, actual.BlockedBy)
	require.JSONEq(t, expected.Metadata, actual.Metadata)
}

func assertAdaptiveExecutionMySQLCommittedArtifacts(
	t *testing.T,
	state adaptiveExecutionMySQLState,
	req CommitAdaptiveExecutionBoundaryRequest,
) {
	t.Helper()
	require.Len(t, state.events, 1)
	event := state.events[0]
	require.Equal(t, req.Event.ID, event.ID)
	require.Equal(t, req.ThreadID, event.ThreadID)
	require.Equal(t, req.ExecutionRunID, event.RunID)
	require.Equal(t, req.JournalRunID, requireInt64PointerForTest(t, event.JournalRunID))
	require.Equal(t, req.AttemptID, requireStringPointerForTest(t, event.AttemptID))
	require.Equal(t, uint64(1), requireUint64PointerForTest(t, event.Sequence))
	require.Equal(t, req.IdempotencyKey, requireStringPointerForTest(t, event.IdempotencyKey))
	require.Equal(t, req.Event.EventType, event.EventType)
	require.JSONEq(t, req.Event.Payload, string(event.Payload))
	require.Equal(t, req.Now, event.CreatedAt)

	require.Len(t, state.checkpoints, 1)
	checkpoint := state.checkpoints[0]
	require.Equal(t, req.Checkpoint.ID, checkpoint.ID)
	require.Equal(t, req.ThreadID, checkpoint.ThreadID)
	require.Equal(t, req.ExecutionRunID, checkpoint.RunID)
	require.Equal(t, req.Checkpoint.ParentCheckpointID, checkpoint.ParentCheckpointID)
	require.Equal(t, req.Checkpoint.CheckpointNS, checkpoint.CheckpointNS)
	require.Equal(t, req.Checkpoint.RuntimeType, checkpoint.RuntimeType)
	require.Equal(t, req.Checkpoint.RuntimeKey, checkpoint.RuntimeKey)
	require.Equal(t, req.Checkpoint.EnvelopeVersion, checkpoint.EnvelopeVersion)
	require.Zero(t, checkpoint.RuntimeDeletedAt)
	require.JSONEq(t, req.Checkpoint.ChannelValues, string(checkpoint.ChannelValues))
	require.JSONEq(t, req.Checkpoint.ChannelVersions, string(checkpoint.ChannelVersions))
	require.JSONEq(t, req.Checkpoint.PendingSends, string(checkpoint.PendingSends))
	require.Equal(t, req.Now, checkpoint.CreatedAt)
	require.JSONEq(
		t,
		extractJSONFieldForTest(t, req.Checkpoint.Metadata, "writer"),
		extractJSONFieldForTest(t, string(checkpoint.Metadata), "writer"),
	)
	metadata := decodeAdaptiveExecutionMetadataForTest(t, checkpoint.Metadata)
	require.Equal(t, req.Event.ID, metadata.EventID)
	require.Equal(t, uint64(1), metadata.EventSequence)
	require.Equal(t, req.JournalRunID, metadata.JournalRunID)
	require.Equal(t, req.AttemptID, metadata.AttemptID)
	require.Equal(t, req.PlanMutation.PlanScopeRunID, metadata.PlanScopeRunID)
	require.Equal(t, req.PlanMutation.NextRevision, metadata.PlanRevision)
	require.Equal(t, req.ExecutionRunID, metadata.ExecutionRunID)
	require.Equal(t, req.Generation, metadata.ExecutionGeneration)
	require.True(t, validAdaptiveExecutionFingerprint(metadata.ItemFingerprint))
}

type adaptiveExecutionLegacyMutation string

const (
	adaptiveExecutionLegacyMutationUpsert  adaptiveExecutionLegacyMutation = "upsert"
	adaptiveExecutionLegacyMutationArchive adaptiveExecutionLegacyMutation = "archive"
)

type adaptiveExecutionLegacyResult struct {
	stored   *entity.AgentRunPlanItem
	previous *entity.AgentRunPlanItem
	plan     *entity.AgentRunPlan
	created  bool
}

func runAdaptiveAndLegacyMySQLRace(t *testing.T, mutation adaptiveExecutionLegacyMutation) {
	t.Helper()
	db, repoA, repoB := adaptiveExecutionMySQLIntegrationRepositories(t)
	seedAdaptiveExecutionMySQLState(t, db)
	adaptiveRequest := adaptiveExecutionMySQLRequest("adaptive", 7101, 8101, 1000)
	adaptiveRequest.PlanMutation.Items[0].NextItem.Subject = "adaptive post-image"
	adaptiveRequest.PlanMutation.Items[0].NextItem.Description = "adaptive description"
	adaptiveRequest.PlanMutation.Items[0].NextItem.Metadata = `{"writer":"adaptive"}`

	legacyCall := func(ctx context.Context) (any, error) {
		switch mutation {
		case adaptiveExecutionLegacyMutationUpsert:
			stored, previous, plan, created, err := repoB.UpsertPlanItem(ctx, &entity.AgentRunPlanItem{
				ID: 999, RunID: 20, TaskID: 1,
				Subject: "legacy post-image", Description: "legacy description",
				Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "legacy",
				Owner: "legacy", Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"writer":"legacy"}`,
				Active: true, Version: 2, CreatedAt: 1100, UpdatedAt: 1100,
			})
			return &adaptiveExecutionLegacyResult{
				stored: stored, previous: previous, plan: plan, created: created,
			}, err
		case adaptiveExecutionLegacyMutationArchive:
			stored, previous, plan, err := repoB.ArchivePlanItem(ctx, 20, 1, 1100)
			return &adaptiveExecutionLegacyResult{stored: stored, previous: previous, plan: plan}, err
		default:
			return nil, errors.New("unsupported legacy mutation")
		}
	}

	results := runAdaptiveExecutionMySQLRace(t, []adaptiveExecutionMySQLRaceCall{
		{
			name: "adaptive",
			call: func(ctx context.Context) (any, error) {
				return repoA.CommitAdaptiveExecutionBoundary(ctx, adaptiveRequest)
			},
		},
		{name: "legacy", call: legacyCall},
	})
	byName := make(map[string]adaptiveExecutionMySQLRaceResult, len(results))
	for _, result := range results {
		requireAdaptiveExecutionMySQLRaceError(t, result.err)
		byName[result.name] = result
	}
	adaptiveResult := byName["adaptive"]
	legacyRaceResult := byName["legacy"]
	require.NoError(t, legacyRaceResult.err)
	legacyResult, ok := legacyRaceResult.value.(*adaptiveExecutionLegacyResult)
	require.True(t, ok)
	require.NotNil(t, legacyResult)
	require.NotNil(t, legacyResult.stored)
	require.NotNil(t, legacyResult.previous)
	require.NotNil(t, legacyResult.plan)
	if mutation == adaptiveExecutionLegacyMutationUpsert {
		require.False(t, legacyResult.created)
	}

	state := readAdaptiveExecutionMySQLState(t, db)
	assertAdaptiveExecutionMySQLRunUnchanged(t, state.run)
	require.Len(t, state.items, 2)
	finalItem := state.items[0].toEntity()
	assertAdaptiveExecutionMySQLSentinelUnchanged(t, state.items[1])

	switch {
	case errors.Is(adaptiveResult.err, ErrAdaptiveExecutionPlanRevisionConflict):
		require.Nil(t, adaptiveResult.value)
		require.Equal(t, int64(2), state.plan.Revision)
		require.Equal(t, int64(2), finalItem.Version)
		require.Equal(t, int64(1), legacyResult.previous.Version)
		require.Equal(t, int64(2), legacyResult.plan.Revision)
		require.Equal(t, uint64(1), state.attempt.NextSequence)
		require.Zero(t, state.attempt.LastCommittedSequence)
		require.Empty(t, state.events)
		require.Empty(t, state.checkpoints)
	case adaptiveResult.err == nil:
		committed, ok := adaptiveResult.value.(*CommitAdaptiveExecutionBoundaryResult)
		require.True(t, ok)
		require.NotNil(t, committed)
		require.Equal(t, int64(2), committed.Plan.Revision)
		require.Equal(t, int64(3), state.plan.Revision)
		require.Equal(t, int64(3), finalItem.Version)
		assertAdaptiveExecutionMySQLPlanItemEqual(
			t,
			adaptiveRequest.PlanMutation.Items[0].NextItem,
			legacyResult.previous,
		)
		require.Equal(t, int64(3), legacyResult.plan.Revision)
		require.Equal(t, uint64(2), state.attempt.NextSequence)
		require.Equal(t, uint64(1), state.attempt.LastCommittedSequence)
		assertAdaptiveExecutionMySQLCommittedArtifacts(t, state, adaptiveRequest)
	default:
		t.Fatalf("adaptive writer returned an illegal linearization result: %v", adaptiveResult.err)
	}
	assertAdaptiveExecutionMySQLPlanItemEqual(t, finalItem, legacyResult.stored)
	require.Equal(t, state.plan.toEntity(), legacyResult.plan)

	expectedVersion := int64(2)
	if adaptiveResult.err == nil {
		expectedVersion = 3
	}
	var expectedFinal *entity.AgentRunPlanItem
	switch mutation {
	case adaptiveExecutionLegacyMutationUpsert:
		expectedFinal = &entity.AgentRunPlanItem{
			ID: 60, RunID: 20, TaskID: 1,
			Subject: "legacy post-image", Description: "legacy description",
			Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "legacy", Owner: "legacy",
			Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"writer":"legacy"}`,
			Active: true, Version: expectedVersion, CreatedAt: 900, UpdatedAt: 1100,
		}
	case adaptiveExecutionLegacyMutationArchive:
		if adaptiveResult.err == nil {
			clone := *adaptiveRequest.PlanMutation.Items[0].NextItem
			expectedFinal = &clone
		} else {
			expectedFinal = &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1,
				Subject: "seed post-image", Description: "seed description",
				Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
				Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"writer":"seed"}`,
				Active: true, Version: 1, CreatedAt: 900, UpdatedAt: 900,
			}
		}
		expectedFinal.Status = entity.AgentRunPlanItemStatusDeleted
		expectedFinal.Active = false
		expectedFinal.Version = expectedVersion
		expectedFinal.UpdatedAt = 1100
	default:
		t.Fatalf("unsupported legacy mutation %q", mutation)
	}
	assertAdaptiveExecutionMySQLPlanItemEqual(t, expectedFinal, finalItem)
}

func adaptiveExecutionMySQLRequest(
	writer string,
	eventID int64,
	checkpointID int64,
	now int64,
) CommitAdaptiveExecutionBoundaryRequest {
	return CommitAdaptiveExecutionBoundaryRequest{
		ThreadID:       10,
		ExecutionRunID: 20,
		JournalRunID:   30,
		AttemptID:      "attempt-1",
		Generation:     3,
		LeaseOwner:     "worker-1",
		LeaseToken:     "lease-1",
		Now:            now,
		IdempotencyKey: "boundary-" + writer,
		Event: &entity.RunEvent{
			ID: eventID, ThreadID: 10, RunID: 20,
			EventType: "run.boundary", Payload: `{"writer":"` + writer + `"}`,
			CreatedAt: now,
		},
		Checkpoint: &entity.Checkpoint{
			ID: checkpointID, ThreadID: 10, RunID: 20,
			CheckpointNS: "adaptive", RuntimeType: "eino_adk",
			RuntimeKey: "thread:10:run:20:" + writer, EnvelopeVersion: 1,
			ChannelValues: `{}`, ChannelVersions: `{}`, PendingSends: `[]`,
			Metadata: `{"writer":"` + writer + `"}`, CreatedAt: now,
		},
		PlanMutation: &AdaptivePlanMutation{
			PlanScopeRunID: 20, ExpectedRevision: 1, NextRevision: 2,
			Items: []AdaptivePlanItemMutation{{
				ExpectedVersion: 1,
				NextItem: &entity.AgentRunPlanItem{
					ID: 60, RunID: 20, TaskID: 1,
					Subject: writer + " post-image", Description: writer + " description",
					Status:     entity.AgentRunPlanItemStatusInProgress,
					ActiveForm: "executing", Owner: "agent",
					Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"writer":"` + writer + `"}`,
					Active: true, Version: 2, CreatedAt: 900, UpdatedAt: now,
				},
			}},
		},
	}
}

func seedAdaptiveExecutionMySQLState(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&threadPO{
		ID: 10, SpaceID: 10, CreatorID: 20, Title: "adaptive mysql",
		Status: string(entity.ThreadStatusRunning), Source: string(entity.ThreadSourceWeb),
		Metadata: datatypes.JSON([]byte(`{}`)), CreatedAt: 600, UpdatedAt: 600,
	}).Error)
	require.NoError(t, db.Create(&runPO{
		ID: 30, ThreadID: 10, ParentRunID: 0, SpaceID: 10, CreatorID: 20,
		AssistantID: "agent", RunKind: string(entity.RunKindTask),
		Status:  string(entity.RunStatusSucceeded),
		Command: datatypes.JSON([]byte(`{}`)), Input: datatypes.JSON([]byte(`{}`)),
		Config: datatypes.JSON([]byte(`{}`)), Context: datatypes.JSON([]byte(`{}`)),
		Metadata: datatypes.JSON([]byte(`{}`)), StreamMode: datatypes.JSON([]byte(`[]`)),
		StartedAt: 600, EndedAt: 600, CreatedAt: 600, UpdatedAt: 600,
	}).Error)
	leaseOwner := "worker-1"
	leaseToken := "lease-1"
	leaseExpiresAt := int64(5000)
	require.NoError(t, db.Create(&runPO{
		ID: 20, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		AssistantID: "agent", RunKind: string(entity.RunKindTask),
		Status:  string(entity.RunStatusRunning),
		Command: datatypes.JSON([]byte(`{}`)), Input: datatypes.JSON([]byte(`{}`)),
		Config: datatypes.JSON([]byte(`{}`)), Context: datatypes.JSON([]byte(`{}`)),
		Metadata: datatypes.JSON([]byte(`{}`)), StreamMode: datatypes.JSON([]byte(`[]`)),
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiresAt,
		ExecutionGeneration: 3, StartedAt: 700, CreatedAt: 700, UpdatedAt: 700,
	}).Error)
	activeSlot := uint8(1)
	startedAt := int64(700)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 100, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 20,
		AttemptID: "attempt-1", Ordinal: 1, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &activeSlot, NextSequence: 1, LastCommittedSequence: 0,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		StartedAt:         &startedAt, CreatedAt: 700, UpdatedAt: 700,
	}).Error)
	require.NoError(t, db.Create(&agentRunPlanPO{
		RunID: 20, ThreadID: 10, SpaceID: 10, UserID: 20,
		HighWatermark: 2, Revision: 1, CreatedAt: 800, UpdatedAt: 800,
	}).Error)
	item, err := agentRunPlanItemToPO(&entity.AgentRunPlanItem{
		ID: 60, RunID: 20, TaskID: 1,
		Subject: "seed post-image", Description: "seed description",
		Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
		Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"writer":"seed"}`,
		Active: true, Version: 1, CreatedAt: 900, UpdatedAt: 900,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(item).Error)
	sentinel, err := agentRunPlanItemToPO(&entity.AgentRunPlanItem{
		ID: 61, RunID: 20, TaskID: 2,
		Subject: "sentinel major step", Description: "sentinel description",
		Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "queued", Owner: "sentinel",
		Blocks: `[1]`, BlockedBy: `[2]`, Metadata: `{"sentinel":true}`,
		Active: true, Version: 7, CreatedAt: 850, UpdatedAt: 850,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(sentinel).Error)
}

func adaptiveExecutionMySQLIntegrationRepositories(
	t *testing.T,
) (*gorm.DB, *threadRepository, *threadRepository) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(canonicalMySQLTestDSNEnv))
	if dsn == "" || os.Getenv(canonicalMySQLTestDDLGateEnv) != canonicalMySQLTestDDLGate {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("CI")), "true") {
			t.Fatalf(
				"CI requires %s and %s=%s",
				canonicalMySQLTestDSNEnv,
				canonicalMySQLTestDDLGateEnv,
				canonicalMySQLTestDDLGate,
			)
		}
		t.Skip("requires an explicitly gated disposable MySQL database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	if !strings.Contains(strings.ToLower(config.DBName), "agentthread_disposable") {
		t.Fatalf("%s must select a database whose name contains agentthread_disposable", canonicalMySQLTestDSNEnv)
	}

	dbA, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDBA, err := dbA.DB()
	require.NoError(t, err)
	sqlDBA.SetMaxOpenConns(1)
	sqlDBA.SetMaxIdleConns(1)
	t.Cleanup(func() {
		require.NoError(t, sqlDBA.Close())
	})

	dbB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDBB, err := dbB.DB()
	require.NoError(t, err)
	sqlDBB.SetMaxOpenConns(1)
	sqlDBB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		require.NoError(t, sqlDBB.Close())
	})

	var connectionIDA, connectionIDB uint64
	var selectedDatabaseA, selectedDatabaseB string
	require.NoError(t, dbA.Raw("SELECT DATABASE()").Scan(&selectedDatabaseA).Error)
	require.NoError(t, dbB.Raw("SELECT DATABASE()").Scan(&selectedDatabaseB).Error)
	require.Equal(t, config.DBName, selectedDatabaseA)
	require.Equal(t, config.DBName, selectedDatabaseB)
	require.NoError(t, dbA.Raw("SELECT CONNECTION_ID()").Scan(&connectionIDA).Error)
	require.NoError(t, dbB.Raw("SELECT CONNECTION_ID()").Scan(&connectionIDB).Error)
	require.NotZero(t, connectionIDA)
	require.NotZero(t, connectionIDB)
	require.NotEqual(t, connectionIDA, connectionIDB)
	t.Cleanup(func() {
		require.NoError(t, dropAdaptiveExecutionMySQLTables(dbA))
	})
	require.NoError(t, resetAdaptiveExecutionMySQLSchema(dbA))

	return dbA, &threadRepository{db: dbA}, &threadRepository{db: dbB}
}

func resetAdaptiveExecutionMySQLSchema(db *gorm.DB) error {
	if err := dropAdaptiveExecutionMySQLTables(db); err != nil {
		return err
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return gorm.ErrInvalidData
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../../.."))
	migrations := []string{
		"docker/atlas/migrations/20260613000100_agent_threads.sql",
		"docker/atlas/migrations/20260614000100_agent_thread_messages.sql",
		"docker/atlas/migrations/20260614000200_agent_runs.sql",
		"docker/atlas/migrations/20260614000300_agent_run_events.sql",
		"docker/atlas/migrations/20260617000100_agent_checkpoints.sql",
		"docker/atlas/migrations/20260619000100_agent_checkpoint_runtime_keys.sql",
		"docker/atlas/migrations/20260620000400_agent_run_plans.sql",
		"docker/atlas/migrations/20260621000100_agent_run_children.sql",
		"docker/atlas/migrations/20260711000100_agent_run_leases.sql",
		"docker/atlas/migrations/20260730000100_agent_run_attempts.sql",
		"docker/atlas/migrations/20260730000200_agent_run_events_journal_columns.sql",
		"docker/atlas/migrations/20260730000210_agent_run_events_journal_indexes.sql",
		"docker/atlas/migrations/20260730000220_agent_run_events_journal_projection.sql",
	}
	for _, migration := range migrations {
		contents, err := os.ReadFile(filepath.Join(repositoryRoot, migration))
		if err != nil {
			return err
		}
		for _, statement := range strings.Split(string(contents), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if err := db.Exec(statement).Error; err != nil {
				return err
			}
		}
	}
	if err := db.Exec(`CREATE TABLE p0d_adaptive_outbox_probe (
  idempotency_key VARCHAR(191) NOT NULL,
  event_id VARCHAR(128) NOT NULL,
  fingerprint CHAR(64) NOT NULL,
  payload LONGBLOB NOT NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (idempotency_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error; err != nil {
		return err
	}
	return nil
}

func dropAdaptiveExecutionMySQLTables(db *gorm.DB) error {
	for _, table := range []string{
		"p0d_adaptive_outbox_probe",
		"agent_run_attempts",
		"agent_run_plan_items",
		"agent_run_plans",
		"agent_checkpoints",
		"agent_run_events",
		"agent_thread_messages",
		"agent_runs",
		"agent_threads",
	} {
		if err := db.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			return err
		}
	}
	return nil
}
