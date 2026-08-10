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
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
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
	})

	t.Run("ConcurrentAdaptiveAndLegacyUpsertAreLinearizable", func(t *testing.T) {
		runAdaptiveAndLegacyMySQLRace(t, adaptiveExecutionLegacyMutationUpsert)
	})

	t.Run("ConcurrentAdaptiveAndLegacyArchiveAreLinearizable", func(t *testing.T) {
		runAdaptiveAndLegacyMySQLRace(t, adaptiveExecutionLegacyMutationArchive)
	})
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
	require.NotContains(t, lower, "deadlock")
	require.NotContains(t, lower, "lock wait timeout")
	require.NotContains(t, lower, "duplicate entry")
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
	dbB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDBA, err := dbA.DB()
	require.NoError(t, err)
	sqlDBB, err := dbB.DB()
	require.NoError(t, err)
	sqlDBA.SetMaxOpenConns(1)
	sqlDBA.SetMaxIdleConns(1)
	sqlDBB.SetMaxOpenConns(1)
	sqlDBB.SetMaxIdleConns(1)

	t.Cleanup(func() {
		require.NoError(t, dropAdaptiveExecutionMySQLTables(dbA))
		require.NoError(t, sqlDBB.Close())
		require.NoError(t, sqlDBA.Close())
	})
	require.NoError(t, resetAdaptiveExecutionMySQLSchema(dbA))

	var connectionIDA, connectionIDB uint64
	require.NoError(t, dbA.Raw("SELECT CONNECTION_ID()").Scan(&connectionIDA).Error)
	require.NoError(t, dbB.Raw("SELECT CONNECTION_ID()").Scan(&connectionIDB).Error)
	require.NotZero(t, connectionIDA)
	require.NotZero(t, connectionIDB)
	require.NotEqual(t, connectionIDA, connectionIDB)

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
	return nil
}

func dropAdaptiveExecutionMySQLTables(db *gorm.DB) error {
	for _, table := range []string{
		"agent_run_plan_items",
		"agent_run_plans",
		"agent_checkpoints",
		"agent_run_events",
		"agent_run_attempts",
		"agent_runs",
		"agent_threads",
	} {
		if err := db.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			return err
		}
	}
	return nil
}
