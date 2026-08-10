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
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON(t *testing.T) {
	items := []*entity.AgentRunPlanItem{
		{
			ID: 60, RunID: 20, TaskID: 1, Subject: "step one", Description: "first",
			Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "executing", Owner: "agent",
			Blocks: `{"z":2,"a":1}`, BlockedBy: `[{"b":2,"a":1}]`,
			Metadata: `{"nested":{"z":2,"a":1}}`, Active: true, Version: 2,
			CreatedAt: 900, UpdatedAt: 1000,
		},
		{
			ID: 61, RunID: 20, TaskID: 2, Subject: "step two", Description: "second",
			Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
			Blocks: `[]`, BlockedBy: `[1,2]`, Metadata: `{}`, Active: true, Version: 1,
			CreatedAt: 1000, UpdatedAt: 1000,
		},
	}

	fingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	require.NoError(t, err)

	reordered := cloneAdaptiveItemsForTest(items)
	reordered[0].Blocks = `{ "a": 1, "z": 2 }`
	reordered[0].BlockedBy = `[{"a":1,"b":2}]`
	reordered[0].Metadata = `{"nested":{"a":1,"z":2}}`
	reorderedFingerprint, err := adaptiveExecutionPlanItemFingerprint(reordered)
	require.NoError(t, err)
	require.Equal(t, fingerprint, reorderedFingerprint)

	changedValue := cloneAdaptiveItemsForTest(items)
	changedValue[0].Metadata = `{"nested":{"a":1,"z":3}}`
	changedFingerprint, err := adaptiveExecutionPlanItemFingerprint(changedValue)
	require.NoError(t, err)
	require.NotEqual(t, fingerprint, changedFingerprint)

	changedArrayOrder := cloneAdaptiveItemsForTest(items)
	changedArrayOrder[1].BlockedBy = `[2,1]`
	changedFingerprint, err = adaptiveExecutionPlanItemFingerprint(changedArrayOrder)
	require.NoError(t, err)
	require.NotEqual(t, fingerprint, changedFingerprint)

	changedContent := cloneAdaptiveItemsForTest(items)
	changedContent[1].Subject = "different"
	changedFingerprint, err = adaptiveExecutionPlanItemFingerprint(changedContent)
	require.NoError(t, err)
	require.NotEqual(t, fingerprint, changedFingerprint)

	for _, malformed := range []string{`{"a":`, `{} trailing`} {
		invalid := cloneAdaptiveItemsForTest(items)
		invalid[0].Blocks = malformed
		_, err := adaptiveExecutionPlanItemFingerprint(invalid)
		require.Error(t, err)
	}
}

func TestAdaptiveExecutionBoundaryCommitsInitialMutationAtomically(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)

	result, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, uint64(1), result.LastCommittedSequence)
	require.Equal(t, []int64{1, 2}, adaptiveTaskIDsForTest(result.Items))

	expectedItems := []*entity.AgentRunPlanItem{
		{
			ID: 60, RunID: 20, TaskID: 1, Subject: "updated major step", Description: "updated description",
			Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "executing", Owner: "agent",
			Blocks: `{"z":2,"a":1}`, BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
			Active: true, Version: 2, CreatedAt: 900, UpdatedAt: 1000,
		},
		{
			ID: 61, RunID: 20, TaskID: 2, Subject: "new major step", Description: "new description",
			Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
			Blocks: `[]`, BlockedBy: `[1]`, Metadata: `{"substeps_ref":"lazy:2"}`,
			Active: true, Version: 1, CreatedAt: 1000, UpdatedAt: 1000,
		},
	}
	require.Equal(t, expectedItems, result.Items)
	require.Equal(t, &entity.RunEvent{
		ID: 7001, ThreadID: 10, RunID: 20, EventType: "run.boundary",
		Payload: `{"step":1}`, CreatedAt: 1000,
	}, result.Event)
	require.Equal(t, &entity.AgentRunPlan{
		RunID: 20, ThreadID: 10, SpaceID: 10, UserID: 20,
		HighWatermark: 2, Revision: 2, CreatedAt: 800, UpdatedAt: 1000,
	}, result.Plan)

	var event runEventPO
	require.NoError(t, db.Where("id = ?", 7001).First(&event).Error)
	require.Equal(t, int64(30), requireInt64PointerForTest(t, event.JournalRunID))
	require.Equal(t, "attempt-1", requireStringPointerForTest(t, event.AttemptID))
	require.Equal(t, uint64(1), requireUint64PointerForTest(t, event.Sequence))
	require.Equal(t, "boundary-1", requireStringPointerForTest(t, event.IdempotencyKey))

	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", 8001).First(&checkpoint).Error)
	require.Equal(t, result.Checkpoint, checkpoint.toEntity())
	metadata := decodeAdaptiveExecutionMetadataForTest(t, checkpoint.Metadata)
	require.Equal(t, "workbench-adaptive-boundary.v1", metadata.SchemaVersion)
	require.Equal(t, int64(7001), metadata.EventID)
	require.Equal(t, uint64(1), metadata.EventSequence)
	require.Equal(t, int64(30), metadata.JournalRunID)
	require.Equal(t, "attempt-1", metadata.AttemptID)
	require.Equal(t, int64(20), metadata.PlanScopeRunID)
	require.Equal(t, int64(2), metadata.PlanRevision)
	require.Equal(t, "3855b0cfab3915e2fe2a588e3155382777d4e8b87922bde23fdd278f2bcdb7db", metadata.ItemFingerprint)
	require.Equal(t, int64(20), metadata.ExecutionRunID)
	require.Equal(t, uint64(3), metadata.ExecutionGeneration)

	var plan agentRunPlanPO
	require.NoError(t, db.Where("run_id = ?", 20).First(&plan).Error)
	require.Equal(t, result.Plan, plan.toEntity())
	var itemPOs []agentRunPlanItemPO
	require.NoError(t, db.Where("run_id = ?", 20).Order("task_id ASC").Find(&itemPOs).Error)
	require.Len(t, itemPOs, 2)
	storedItems := []*entity.AgentRunPlanItem{itemPOs[0].toEntity(), itemPOs[1].toEntity()}
	require.Equal(t, expectedItems, storedItems)

	var attempt runAttemptPO
	require.NoError(t, db.Where("journal_run_id = ? AND attempt_id = ?", 30, "attempt-1").First(&attempt).Error)
	require.Equal(t, uint64(2), attempt.NextSequence)
	require.Equal(t, uint64(1), attempt.LastCommittedSequence)
	require.Equal(t, int64(1000), attempt.UpdatedAt)
}

func TestAdaptiveExecutionCheckpointMetadataRoundTrip(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	req.Checkpoint.Metadata = `{"runtime_field":{"nested":true},"runtime_number":1.25}`

	result, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	require.JSONEq(t, `{"nested":true}`, extractJSONFieldForTest(t, result.Checkpoint.Metadata, "runtime_field"))
	require.JSONEq(t, `1.25`, extractJSONFieldForTest(t, result.Checkpoint.Metadata, "runtime_number"))
	metadata := decodeAdaptiveExecutionMetadataForTest(t, []byte(result.Checkpoint.Metadata))
	require.Equal(t, int64(7001), metadata.EventID)
	require.Equal(t, uint64(1), metadata.EventSequence)
	require.Equal(t, int64(30), metadata.JournalRunID)
	require.Equal(t, "attempt-1", metadata.AttemptID)
	require.Equal(t, int64(20), metadata.PlanScopeRunID)
	require.Equal(t, int64(2), metadata.PlanRevision)
	require.Equal(t, int64(20), metadata.ExecutionRunID)
	require.Equal(t, uint64(3), metadata.ExecutionGeneration)
}

func TestAdaptiveExecutionBoundaryRecoversCanonicalPlanScopeAcrossMultipleHops(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)

	a, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000))
	require.NoError(t, err)
	require.Equal(t, int64(20), decodeAdaptiveExecutionMetadataForTest(t, []byte(a.Checkpoint.Metadata)).PlanScopeRunID)
	terminalizeAdaptiveAttemptForTest(t, db, 100)

	bRequest := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 21, AttemptRowID: 101, AttemptID: "attempt-2", Generation: 4,
		SourceAttemptID: "attempt-1", SourceCheckpointID: 8001,
		EventID: 7002, CheckpointID: 8002, ExpectedRevision: 2, ExpectedItemVersion: 2, Now: 1100,
	})
	b, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), bRequest)
	require.NoError(t, err)
	bMetadata := decodeAdaptiveExecutionMetadataForTest(t, []byte(b.Checkpoint.Metadata))
	require.Equal(t, int64(20), bMetadata.PlanScopeRunID)
	require.Equal(t, int64(21), bMetadata.ExecutionRunID)
	require.Equal(t, uint64(4), bMetadata.ExecutionGeneration)
	require.Equal(t, int64(3), bMetadata.PlanRevision)
	terminalizeAdaptiveAttemptForTest(t, db, 101)

	cRequest := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 22, AttemptRowID: 102, AttemptID: "attempt-3", Generation: 5,
		SourceAttemptID: "attempt-2", SourceCheckpointID: 8002,
		EventID: 7003, CheckpointID: 8003, ExpectedRevision: 3, ExpectedItemVersion: 3, Now: 1200,
	})
	c, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), cRequest)
	require.NoError(t, err)
	cMetadata := decodeAdaptiveExecutionMetadataForTest(t, []byte(c.Checkpoint.Metadata))
	require.Equal(t, int64(20), cMetadata.PlanScopeRunID)
	require.Equal(t, int64(22), cMetadata.ExecutionRunID)
	require.Equal(t, uint64(5), cMetadata.ExecutionGeneration)
	require.Equal(t, int64(4), cMetadata.PlanRevision)
	require.Equal(t, int64(20), c.Plan.RunID)
}

func TestAdaptiveExecutionBoundaryRejectsNonCanonicalRecoveryLineage(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "source attempt only", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_checkpoint_id", nil).Error)
		}},
		{name: "source checkpoint only", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_attempt_id", nil).Error)
		}},
		{name: "source attempt does not own checkpoint", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			seedAmbientAdaptiveSourceForTest(t, db)
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_attempt_id", "ambient-attempt").Error)
		}},
		{name: "ambient same thread source", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			seedAmbientAdaptiveSourceForTest(t, db)
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Updates(map[string]any{
				"source_attempt_id": "ambient-attempt", "source_checkpoint_id": 8999,
			}).Error)
		}},
		{name: "target parent drift", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			req.Checkpoint.ParentCheckpointID++
		}},
		{name: "self source attempt", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_attempt_id", "attempt-2").Error)
		}},
		{name: "self source checkpoint", mutate: func(t *testing.T, db *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_checkpoint_id", req.Checkpoint.ID).Error)
			req.Checkpoint.ParentCheckpointID = req.Checkpoint.ID
		}},
		{name: "source checkpoint deleted", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("runtime_deleted_at", 1).Error)
		}},
		{name: "metadata missing namespace", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("metadata", []byte(`{"runtime_field":"preserved"}`)).Error)
		}},
		{name: "metadata unknown schema", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["schema_version"] = json.RawMessage(`"unknown"`)
			})
		}},
		{name: "metadata event missing", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["event_id"] = json.RawMessage(`7999`)
			})
		}},
		{name: "metadata sequence beyond committed", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["event_sequence"] = json.RawMessage(`2`)
			})
		}},
		{name: "metadata request plan scope drift", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			req.PlanMutation.PlanScopeRunID = 21
			req.PlanMutation.Items[0].NextItem.RunID = 21
		}},
		{name: "metadata plan revision drift", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["plan_revision"] = json.RawMessage(`1`)
			})
		}},
		{name: "source checkpoint cross thread", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("thread_id", 11).Error)
		}},
		{name: "source attempt cross thread", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Update("thread_id", 11).Error)
		}},
	}

	for _, field := range []string{
		"schema_version", "event_id", "event_sequence", "journal_run_id", "attempt_id",
		"plan_scope_run_id", "plan_revision", "item_fingerprint", "execution_run_id", "execution_generation",
	} {
		field := field
		tests = append(tests, struct {
			name   string
			mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBoundaryRequest)
		}{name: "metadata missing " + field, mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) { delete(metadata, field) })
		}})
	}
	for _, field := range []string{"thread_id", "run_id", "journal_run_id", "attempt_id", "sequence"} {
		field := field
		tests = append(tests, struct {
			name   string
			mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBoundaryRequest)
		}{name: "source event " + field + " drift", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceEventFieldForTest(t, db, field)
		}})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, req := prepareAdaptiveRecoveryTestForTest(t)
			tt.mutate(t, db, &req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionLineageConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRejectsStaleSourceGeneration(t *testing.T) {
	db, req := prepareAdaptiveRecoveryTestForTest(t)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 20).Update("execution_generation", 4).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.ErrorIs(t, err, ErrAdaptiveExecutionLineageConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRejectsInvalidTargetAttemptCursor(t *testing.T) {
	activeZero := uint8(0)
	activeTwo := uint8(2)
	tests := []struct {
		name        string
		updates     map[string]any
		expectedErr error
		directMax   bool
	}{
		{name: "nil active slot", updates: map[string]any{"active_slot": nil}, expectedErr: ErrAdaptiveExecutionAttemptConflict},
		{name: "zero active slot", updates: map[string]any{"active_slot": activeZero}, expectedErr: ErrAdaptiveExecutionAttemptConflict},
		{name: "two active slot", updates: map[string]any{"active_slot": activeTwo}, expectedErr: ErrAdaptiveExecutionAttemptConflict},
		{name: "zero next sequence", updates: map[string]any{"next_sequence": uint64(0)}, expectedErr: ErrAdaptiveExecutionSequenceConflict},
		{name: "committed equals next", updates: map[string]any{"last_committed_sequence": uint64(1)}, expectedErr: ErrAdaptiveExecutionSequenceConflict},
		{name: "committed beyond next", updates: map[string]any{"last_committed_sequence": uint64(2)}, expectedErr: ErrAdaptiveExecutionSequenceConflict},
		{name: "next sequence max", expectedErr: ErrAdaptiveExecutionSequenceConflict, directMax: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			if !tt.directMax {
				require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Updates(tt.updates).Error)
			}
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			if tt.directMax {
				var locked runAttemptPO
				require.NoError(t, db.Where("id = ?", 100).First(&locked).Error)
				locked.NextSequence = math.MaxUint64
				require.ErrorIs(t, validateAdaptiveExecutionTargetAttemptCursor(&locked), tt.expectedErr)
				require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
				return
			}
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
				context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
			)
			require.ErrorIs(t, err, tt.expectedErr)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRejectsInitialCheckpointParent(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	req.Checkpoint.ParentCheckpointID = 7999
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.ErrorIs(t, err, ErrAdaptiveExecutionLineageConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRejectsMalformedMutationJSONWithoutWrites(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "event payload", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.Payload = `{"broken":` }},
		{name: "checkpoint channel values", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ChannelValues = `{"broken":` }},
		{name: "checkpoint channel versions", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ChannelVersions = `{"broken":` }},
		{name: "checkpoint pending sends", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.PendingSends = `[{` }},
		{name: "checkpoint metadata", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.Metadata = `{"broken":` }},
		{name: "plan item blocks", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.Blocks = `[` }},
		{name: "plan item blocked by", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.BlockedBy = `[` }},
		{name: "plan item metadata", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.Metadata = `{` }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
			tt.mutate(&req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionBoundaryInvalid)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRollsBackCheckpointConflict(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	require.NoError(t, db.Create(&checkpointPO{
		ID: 8001, ThreadID: 10, RunID: 20, CheckpointNS: "existing",
		RuntimeType: "eino_adk", RuntimeKey: "existing", EnvelopeVersion: 1,
		ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`),
		Metadata: []byte(`{}`), CreatedAt: 900,
	}).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
		context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
	)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
	var count int64
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 7001).Count(&count).Error)
	require.Zero(t, count)
}

func TestAdaptiveExecutionBoundaryRejectsStalePlanRevision(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	require.NoError(t, db.Model(&agentRunPlanPO{}).Where("run_id = ?", 20).Update("revision", 2).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
		context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
	)
	require.ErrorIs(t, err, ErrAdaptiveExecutionPlanRevisionConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRejectsStalePlanItemVersion(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "second update stale", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&agentRunPlanItemPO{}).Where("id = ?", 60).Update("version", 2).Error)
		}},
		{name: "create tuple exists", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			item := newAdaptivePlanItem(61, 20, 1, 1, req.Now)
			req.PlanMutation.Items = []AdaptivePlanItemMutation{{ExpectedVersion: 0, NextItem: item}}
		}},
		{name: "create id exists", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			item := newAdaptivePlanItem(60, 20, 2, 1, req.Now)
			req.PlanMutation.Items = []AdaptivePlanItemMutation{{ExpectedVersion: 0, NextItem: item}}
		}},
		{name: "task beyond high watermark", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			item := newAdaptivePlanItem(61, 20, 3, 1, req.Now)
			req.PlanMutation.Items = []AdaptivePlanItemMutation{{ExpectedVersion: 0, NextItem: item}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
			tt.mutate(t, db, &req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionPlanItemVersionConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRejectsPlanScopeIdentityDrift(t *testing.T) {
	tests := []struct {
		name   string
		model  any
		where  string
		column string
		value  any
	}{
		{name: "scope run thread", model: &runPO{}, where: "id = ?", column: "thread_id", value: int64(11)},
		{name: "scope run space", model: &runPO{}, where: "id = ?", column: "space_id", value: int64(11)},
		{name: "scope run creator", model: &runPO{}, where: "id = ?", column: "creator_id", value: int64(21)},
		{name: "plan thread", model: &agentRunPlanPO{}, where: "run_id = ?", column: "thread_id", value: int64(11)},
		{name: "plan space", model: &agentRunPlanPO{}, where: "run_id = ?", column: "space_id", value: int64(11)},
		{name: "plan user", model: &agentRunPlanPO{}, where: "run_id = ?", column: "user_id", value: int64(21)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, req := prepareAdaptiveSecondHopRecoveryTestForTest(t)
			require.NoError(t, db.Model(tt.model).Where(tt.where, 20).Update(tt.column, tt.value).Error)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionPlanScopeConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRollsBackAttemptSequenceCASConflict(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	err := db.Transaction(func(tx *gorm.DB) error {
		state := buildAdaptiveExecutionLockedStateForTest(t, tx, req)
		require.NoError(t, tx.Model(&runAttemptPO{}).Where("id = ?", state.attempt.ID).
			Update("next_sequence", state.attempt.NextSequence+1).Error)
		return commitAdaptiveExecutionMutationLocked(tx, req, state)
	})
	require.ErrorIs(t, err, ErrAdaptiveExecutionSequenceConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func prepareAdaptiveSecondHopRecoveryTestForTest(t *testing.T) (*gorm.DB, CommitAdaptiveExecutionBoundaryRequest) {
	t.Helper()
	db, bRequest := prepareAdaptiveRecoveryTestForTest(t)
	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), bRequest)
	require.NoError(t, err)
	terminalizeAdaptiveAttemptForTest(t, db, 101)
	cRequest := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 22, AttemptRowID: 102, AttemptID: "attempt-3", Generation: 5,
		SourceAttemptID: "attempt-2", SourceCheckpointID: 8002,
		EventID: 7003, CheckpointID: 8003, ExpectedRevision: 3, ExpectedItemVersion: 3, Now: 1200,
	})
	return db, cRequest
}

func buildAdaptiveExecutionLockedStateForTest(
	t *testing.T,
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
) *adaptiveExecutionLockedState {
	t.Helper()
	run, err := lockAdaptiveExecutionRun(tx, req)
	require.NoError(t, err)
	attempt, err := lockAdaptiveExecutionAttempt(tx, req)
	require.NoError(t, err)
	_, err = lockAdaptiveExecutionLineage(tx, req, attempt)
	require.NoError(t, err)
	scopeRun, err := lockAdaptiveExecutionScopeRun(tx, req.PlanMutation.PlanScopeRunID)
	require.NoError(t, err)
	plan, err := lockAdaptiveExecutionPlan(tx, req.PlanMutation.PlanScopeRunID)
	require.NoError(t, err)
	require.NoError(t, validateAdaptiveExecutionPlanScope(run, scopeRun, plan, req.PlanMutation))
	items, err := lockAdaptiveExecutionPlanItems(tx, plan, req.PlanMutation, req.Now)
	require.NoError(t, err)
	nextItems := make([]*entity.AgentRunPlanItem, 0, len(items))
	for _, item := range items {
		nextItems = append(nextItems, item.next)
	}
	fingerprint, err := adaptiveExecutionPlanItemFingerprint(nextItems)
	require.NoError(t, err)
	event, err := runEventToPO(req.Event)
	require.NoError(t, err)
	event.JournalRunID = adaptiveExecutionInt64Pointer(req.JournalRunID)
	event.AttemptID = adaptiveExecutionStringPointer(req.AttemptID)
	event.Sequence = adaptiveExecutionUint64Pointer(attempt.NextSequence)
	event.IdempotencyKey = adaptiveExecutionStringPointer(req.IdempotencyKey)
	checkpointEntity := *req.Checkpoint
	checkpointEntity.Metadata, err = mergeAdaptiveExecutionCheckpointMetadata(req.Checkpoint.Metadata, adaptiveExecutionCheckpointMetadata{
		SchemaVersion: adaptiveExecutionCheckpointSchemaVersion,
		EventID:       event.ID, EventSequence: attempt.NextSequence,
		JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
		PlanScopeRunID: req.PlanMutation.PlanScopeRunID, PlanRevision: req.PlanMutation.NextRevision,
		ItemFingerprint: fingerprint, ExecutionRunID: run.ID, ExecutionGeneration: run.ExecutionGeneration,
	})
	require.NoError(t, err)
	checkpoint, err := checkpointToPO(&checkpointEntity)
	require.NoError(t, err)
	return &adaptiveExecutionLockedState{
		run: run, attempt: attempt, plan: plan, event: event, checkpoint: checkpoint,
		items: items, sequence: attempt.NextSequence,
	}
}

type adaptiveRecoveryTargetForTest struct {
	RunID               int64
	AttemptRowID        int64
	AttemptID           string
	Generation          uint64
	SourceAttemptID     string
	SourceCheckpointID  int64
	EventID             int64
	CheckpointID        int64
	ExpectedRevision    int64
	ExpectedItemVersion int64
	Now                 int64
}

func prepareAdaptiveRecoveryTestForTest(t *testing.T) (*gorm.DB, CommitAdaptiveExecutionBoundaryRequest) {
	t.Helper()
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
		context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
	)
	require.NoError(t, err)
	terminalizeAdaptiveAttemptForTest(t, db, 100)
	req := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 21, AttemptRowID: 101, AttemptID: "attempt-2", Generation: 4,
		SourceAttemptID: "attempt-1", SourceCheckpointID: 8001,
		EventID: 7002, CheckpointID: 8002, ExpectedRevision: 2, ExpectedItemVersion: 2, Now: 1100,
	})
	return db, req
}

func seedAdaptiveRecoveryTargetForTest(
	t *testing.T,
	db *gorm.DB,
	target adaptiveRecoveryTargetForTest,
) CommitAdaptiveExecutionBoundaryRequest {
	t.Helper()
	leaseOwner := fmt.Sprintf("worker-%d", target.RunID)
	leaseToken := fmt.Sprintf("lease-%d", target.RunID)
	leaseExpiresAt := target.Now + 1000
	require.NoError(t, db.Create(&runPO{
		ID: target.RunID, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		Status: string(entity.RunStatusRunning), ExecutionGeneration: target.Generation,
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiresAt,
		CreatedAt: target.Now - 100, UpdatedAt: target.Now - 100,
	}).Error)
	activeSlot := uint8(1)
	sourceAttemptID := target.SourceAttemptID
	sourceCheckpointID := target.SourceCheckpointID
	require.NoError(t, db.Create(&runAttemptPO{
		ID: target.AttemptRowID, ThreadID: 10, JournalRunID: 30, ExecutionRunID: target.RunID,
		AttemptID: target.AttemptID, Ordinal: uint32(target.AttemptRowID - 99),
		Status: string(entity.RunAttemptStatusRunning), ActiveSlot: &activeSlot,
		NextSequence: 1, LastCommittedSequence: 0,
		SourceAttemptID: &sourceAttemptID, SourceCheckpointID: &sourceCheckpointID,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		CreatedAt:         target.Now - 100, UpdatedAt: target.Now - 100,
	}).Error)

	req := newValidAdaptiveExecutionMutationRequest()
	req.ExecutionRunID = target.RunID
	req.AttemptID = target.AttemptID
	req.Generation = target.Generation
	req.LeaseOwner = leaseOwner
	req.LeaseToken = leaseToken
	req.Now = target.Now
	req.IdempotencyKey = fmt.Sprintf("boundary-%s", target.AttemptID)
	req.Event = &entity.RunEvent{
		ID: target.EventID, ThreadID: 10, RunID: target.RunID,
		EventType: "run.boundary", Payload: `{"recovery":true}`, CreatedAt: target.Now,
	}
	req.Checkpoint = &entity.Checkpoint{
		ID: target.CheckpointID, ThreadID: 10, RunID: target.RunID,
		ParentCheckpointID: target.SourceCheckpointID,
		CheckpointNS:       "adaptive", RuntimeType: "eino_adk",
		RuntimeKey: fmt.Sprintf("thread:10:run:%d", target.RunID), EnvelopeVersion: 1,
		ChannelValues: `{}`, ChannelVersions: `{}`, PendingSends: `[]`,
		Metadata: `{"runtime_field":"preserved"}`, CreatedAt: target.Now,
	}
	req.PlanMutation = &AdaptivePlanMutation{
		PlanScopeRunID: 20, ExpectedRevision: target.ExpectedRevision, NextRevision: target.ExpectedRevision + 1,
		Items: []AdaptivePlanItemMutation{{
			ExpectedVersion: target.ExpectedItemVersion,
			NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1,
				Subject:     fmt.Sprintf("recovered step %d", target.ExpectedItemVersion+1),
				Description: "recovery update", Status: entity.AgentRunPlanItemStatusInProgress,
				ActiveForm: "executing", Owner: "agent", Blocks: `[]`, BlockedBy: `[]`,
				Metadata: `{"substeps_ref":"lazy:1"}`, Active: true,
				Version: target.ExpectedItemVersion + 1,
			},
		}},
	}
	return req
}

func terminalizeAdaptiveAttemptForTest(t *testing.T, db *gorm.DB, id int64) {
	t.Helper()
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", id).Updates(map[string]any{
		"status": string(entity.RunAttemptStatusCompleted), "active_slot": nil,
	}).Error)
}

func seedAmbientAdaptiveSourceForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&runPO{
		ID: 29, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		Status: string(entity.RunStatusSucceeded), ExecutionGeneration: 1,
		CreatedAt: 900, UpdatedAt: 900,
	}).Error)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 109, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 29,
		AttemptID: "ambient-attempt", Ordinal: 9, Status: string(entity.RunAttemptStatusCompleted),
		NextSequence: 2, LastCommittedSequence: 1,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		CreatedAt:         900, UpdatedAt: 900,
	}).Error)
	require.NoError(t, db.Create(&checkpointPO{
		ID: 8999, ThreadID: 10, RunID: 29, CheckpointNS: "adaptive",
		RuntimeType: "eino_adk", RuntimeKey: "ambient", EnvelopeVersion: 1,
		ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`),
		Metadata: []byte(`{"ambient":true}`), CreatedAt: 900,
	}).Error)
}

func mutateAdaptiveSourceMetadataForTest(
	t *testing.T,
	db *gorm.DB,
	mutate func(map[string]json.RawMessage),
) {
	t.Helper()
	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", 8001).First(&checkpoint).Error)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(checkpoint.Metadata, &envelope))
	var metadata map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope["adaptive_execution"], &metadata))
	mutate(metadata)
	encodedMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)
	envelope["adaptive_execution"] = encodedMetadata
	encodedEnvelope, err := json.Marshal(envelope)
	require.NoError(t, err)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("metadata", encodedEnvelope).Error)
}

func mutateAdaptiveSourceEventFieldForTest(t *testing.T, db *gorm.DB, field string) {
	t.Helper()
	updates := map[string]any{
		"thread_id": int64(11), "run_id": int64(29), "journal_run_id": int64(31),
		"attempt_id": "wrong-attempt", "sequence": uint64(2),
	}
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 7001).Update(field, updates[field]).Error)
}

type adaptiveExecutionDBSnapshotForTest struct {
	Runs        []runPO
	Attempts    []runAttemptPO
	Events      []runEventPO
	Checkpoints []checkpointPO
	Plans       []agentRunPlanPO
	Items       []agentRunPlanItemPO
}

func snapshotAdaptiveExecutionDBForTest(t *testing.T, db *gorm.DB) adaptiveExecutionDBSnapshotForTest {
	t.Helper()
	var result adaptiveExecutionDBSnapshotForTest
	require.NoError(t, db.Order("id ASC").Find(&result.Runs).Error)
	require.NoError(t, db.Order("id ASC").Find(&result.Attempts).Error)
	require.NoError(t, db.Order("id ASC").Find(&result.Events).Error)
	require.NoError(t, db.Order("id ASC").Find(&result.Checkpoints).Error)
	require.NoError(t, db.Order("run_id ASC").Find(&result.Plans).Error)
	require.NoError(t, db.Order("run_id ASC, task_id ASC").Find(&result.Items).Error)
	return result
}

type adaptiveExecutionMetadataForTest struct {
	SchemaVersion       string `json:"schema_version"`
	EventID             int64  `json:"event_id"`
	EventSequence       uint64 `json:"event_sequence"`
	JournalRunID        int64  `json:"journal_run_id"`
	AttemptID           string `json:"attempt_id"`
	PlanScopeRunID      int64  `json:"plan_scope_run_id"`
	PlanRevision        int64  `json:"plan_revision"`
	ItemFingerprint     string `json:"item_fingerprint"`
	ExecutionRunID      int64  `json:"execution_run_id"`
	ExecutionGeneration uint64 `json:"execution_generation"`
}

func newAdaptiveExecutionRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newJournalRepositoryTestDB(t)
	require.NoError(t, db.AutoMigrate(&checkpointPO{}, &agentRunPlanPO{}, &agentRunPlanItemPO{}))
	return db
}

func seedAdaptiveExecutionInitialState(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedJournalThread(t, db, 10)
	leaseOwner := "worker-1"
	leaseToken := "lease-1"
	leaseExpiresAt := int64(2000)
	require.NoError(t, db.Create(&runPO{
		ID: 20, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		Status: string(entity.RunStatusRunning), ExecutionGeneration: 3,
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiresAt,
		CreatedAt: 700, UpdatedAt: 700,
	}).Error)
	activeSlot := uint8(1)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 100, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 20,
		AttemptID: "attempt-1", Ordinal: 1, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &activeSlot, NextSequence: 1, LastCommittedSequence: 0,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		CreatedAt:         700, UpdatedAt: 700,
	}).Error)
	require.NoError(t, db.Create(&agentRunPlanPO{
		RunID: 20, ThreadID: 10, SpaceID: 10, UserID: 20,
		HighWatermark: 2, Revision: 1, CreatedAt: 800, UpdatedAt: 800,
	}).Error)
	item, err := agentRunPlanItemToPO(&entity.AgentRunPlanItem{
		ID: 60, RunID: 20, TaskID: 1, Subject: "major step", Description: "description",
		Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
		Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
		Active: true, Version: 1, CreatedAt: 900, UpdatedAt: 900,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(item).Error)
}

func adaptiveInitialBoundaryRequest(eventID, checkpointID, now int64) CommitAdaptiveExecutionBoundaryRequest {
	req := newValidAdaptiveExecutionMutationRequest()
	req.Event.ID = eventID
	req.Event.CreatedAt = now
	req.Checkpoint.ID = checkpointID
	req.Checkpoint.CreatedAt = now
	req.Now = now
	req.PlanMutation.Items = []AdaptivePlanItemMutation{
		{
			ExpectedVersion: 0,
			NextItem: &entity.AgentRunPlanItem{
				ID: 61, RunID: 20, TaskID: 2, Subject: "new major step", Description: "new description",
				Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
				Blocks: `[]`, BlockedBy: `[1]`, Metadata: `{"substeps_ref":"lazy:2"}`,
				Active: true, Version: 1,
			},
		},
		{
			ExpectedVersion: 1,
			NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1, Subject: "updated major step", Description: "updated description",
				Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "executing", Owner: "agent",
				Blocks: `{"z":2,"a":1}`, BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
				Active: true, Version: 2, CreatedAt: 123, UpdatedAt: 456,
			},
		},
	}
	return req
}

func cloneAdaptiveItemsForTest(items []*entity.AgentRunPlanItem) []*entity.AgentRunPlanItem {
	result := make([]*entity.AgentRunPlanItem, 0, len(items))
	for _, item := range items {
		clone := *item
		result = append(result, &clone)
	}
	return result
}

func adaptiveTaskIDsForTest(items []*entity.AgentRunPlanItem) []int64 {
	result := make([]int64, 0, len(items))
	for _, item := range items {
		result = append(result, item.TaskID)
	}
	return result
}

func decodeAdaptiveExecutionMetadataForTest(t *testing.T, raw []byte) adaptiveExecutionMetadataForTest {
	t.Helper()
	var envelope struct {
		AdaptiveExecution adaptiveExecutionMetadataForTest `json:"adaptive_execution"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	return envelope.AdaptiveExecution
}

func extractJSONFieldForTest(t *testing.T, raw, field string) string {
	t.Helper()
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &object))
	value, exists := object[field]
	require.True(t, exists)
	return string(value)
}

func requireInt64PointerForTest(t *testing.T, value *int64) int64 {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireUint64PointerForTest(t *testing.T, value *uint64) uint64 {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireStringPointerForTest(t *testing.T, value *string) string {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func TestValidateAdaptiveExecutionMutationRequest(t *testing.T) {
	require.NoError(t, validateAdaptiveExecutionMutationRequest(newValidAdaptiveExecutionMutationRequest()))

	tests := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "nil mutation", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation = nil }},
		{name: "plan scope", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.PlanScopeRunID = 0 }},
		{name: "expected revision", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.ExpectedRevision = 0 }},
		{name: "revision continuity", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.NextRevision++ }},
		{name: "zero items", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items = nil }},
		{name: "too many items", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			item := req.PlanMutation.Items[0]
			req.PlanMutation.Items = make([]AdaptivePlanItemMutation, 33)
			for index := range req.PlanMutation.Items {
				next := *item.NextItem
				next.ID += int64(index)
				next.TaskID += int64(index)
				req.PlanMutation.Items[index] = AdaptivePlanItemMutation{ExpectedVersion: item.ExpectedVersion, NextItem: &next}
			}
		}},
		{name: "duplicate item id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			duplicate := *req.PlanMutation.Items[0].NextItem
			duplicate.TaskID++
			req.PlanMutation.Items = append(req.PlanMutation.Items, AdaptivePlanItemMutation{ExpectedVersion: 1, NextItem: &duplicate})
		}},
		{name: "duplicate task id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			duplicate := *req.PlanMutation.Items[0].NextItem
			duplicate.ID++
			req.PlanMutation.Items = append(req.PlanMutation.Items, AdaptivePlanItemMutation{ExpectedVersion: 1, NextItem: &duplicate})
		}},
		{name: "negative expected version", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].ExpectedVersion = -1 }},
		{name: "nil next item", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem = nil }},
		{name: "item id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.ID = 0 }},
		{name: "item run id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.RunID = 0 }},
		{name: "item task id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.TaskID = 0 }},
		{name: "item scope drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.RunID++ }},
		{name: "item version continuity", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.Version++ }},
		{name: "event id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.ID = 0 }},
		{name: "event type", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.EventType = "" }},
		{name: "event timestamp", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.CreatedAt++ }},
		{name: "checkpoint id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ID = 0 }},
		{name: "checkpoint namespace", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.CheckpointNS = "" }},
		{name: "checkpoint timestamp", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.CreatedAt++ }},
		{name: "runtime type", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RuntimeType = "legacy" }},
		{name: "runtime key", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RuntimeKey = "" }},
		{name: "envelope version", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.EnvelopeVersion = 0 }},
		{name: "runtime deleted", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RuntimeDeletedAt = 1 }},
		{name: "metadata non object", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.Metadata = `[]` }},
		{name: "reserved metadata namespace", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.Checkpoint.Metadata = `{"adaptive_execution":{}}`
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newValidAdaptiveExecutionMutationRequest()
			tt.mutate(&req)
			require.ErrorIs(t, validateAdaptiveExecutionMutationRequest(req), ErrAdaptiveExecutionBoundaryInvalid)
		})
	}
}

func newValidAdaptiveExecutionMutationRequest() CommitAdaptiveExecutionBoundaryRequest {
	req := newAdaptiveExecutionBoundaryRequestForFenceTest()
	req.Event.EventType = "run.boundary"
	req.Event.Payload = `{"step":1}`
	req.Event.CreatedAt = req.Now
	req.Checkpoint.CheckpointNS = "adaptive"
	req.Checkpoint.RuntimeType = "eino_adk"
	req.Checkpoint.RuntimeKey = "thread:10:run:20"
	req.Checkpoint.EnvelopeVersion = 1
	req.Checkpoint.ChannelValues = `{}`
	req.Checkpoint.ChannelVersions = `{}`
	req.Checkpoint.PendingSends = `[]`
	req.Checkpoint.Metadata = `{"runtime_field":"preserved"}`
	req.Checkpoint.CreatedAt = req.Now
	req.PlanMutation = &AdaptivePlanMutation{
		PlanScopeRunID:   20,
		ExpectedRevision: 1,
		NextRevision:     2,
		Items: []AdaptivePlanItemMutation{{
			ExpectedVersion: 1,
			NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1,
				Subject: "major step", Description: "execute the step",
				Status:     entity.AgentRunPlanItemStatusInProgress,
				ActiveForm: "executing", Owner: "agent",
				Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
				Active: true, Version: 2, CreatedAt: req.Now - 1, UpdatedAt: req.Now,
			},
		}},
	}
	return req
}

func newAdaptivePlanItem(id, runID, taskID, version, now int64) *entity.AgentRunPlanItem {
	return &entity.AgentRunPlanItem{
		ID: id, RunID: runID, TaskID: taskID,
		Subject: fmt.Sprintf("step-%d", taskID), Description: fmt.Sprintf("description-%d", taskID),
		Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "working", Owner: "agent",
		Blocks: `[]`, BlockedBy: `[]`, Metadata: `{}`, Active: true,
		Version: version, CreatedAt: now, UpdatedAt: now,
	}
}

func TestValidateAdaptiveExecutionBoundaryRequest(t *testing.T) {
	newValidRequest := func() CommitAdaptiveExecutionBoundaryRequest {
		return CommitAdaptiveExecutionBoundaryRequest{
			ThreadID:       10,
			ExecutionRunID: 20,
			JournalRunID:   30,
			AttemptID:      "attempt-1",
			Generation:     3,
			LeaseOwner:     "worker-1",
			LeaseToken:     "lease-1",
			Now:            1000,
			IdempotencyKey: "boundary-1",
			Event: &entity.RunEvent{
				ID: 40, ThreadID: 10, RunID: 20,
				EventType: "run.boundary", Payload: `{}`, CreatedAt: 1000,
			},
			Checkpoint: &entity.Checkpoint{
				ID: 50, ThreadID: 10, RunID: 20,
				CheckpointNS: "adaptive", RuntimeType: "eino_adk", CreatedAt: 1000,
			},
		}
	}

	require.NoError(t, validateAdaptiveExecutionBoundaryRequest(newValidRequest()))

	tests := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "thread id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.ThreadID = 0 }},
		{name: "execution run id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.ExecutionRunID = 0 }},
		{name: "journal run id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.JournalRunID = 0 }},
		{name: "attempt id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.AttemptID = "" }},
		{name: "generation", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Generation = 0 }},
		{name: "lease owner", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.LeaseOwner = "" }},
		{name: "lease token", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.LeaseToken = "" }},
		{name: "now", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Now = 0 }},
		{name: "idempotency key", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.IdempotencyKey = "" }},
		{name: "event", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event = nil }},
		{name: "checkpoint", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint = nil }},
		{name: "event thread drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.ThreadID++ }},
		{name: "event run drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.RunID++ }},
		{name: "checkpoint thread drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ThreadID++ }},
		{name: "checkpoint run drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RunID++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newValidRequest()
			tt.mutate(&req)
			require.ErrorIs(t, validateAdaptiveExecutionBoundaryRequest(req), ErrAdaptiveExecutionBoundaryInvalid)
		})
	}
}

func TestLockAdaptiveExecutionRun(t *testing.T) {
	const now = int64(1000)

	tests := []struct {
		name          string
		mutateRun     func(*runPO)
		mutateRequest func(*CommitAdaptiveExecutionBoundaryRequest)
		expectedErr   error
	}{
		{name: "success"},
		{
			name: "wrong thread id",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.ThreadID++
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "non-running",
			mutateRun: func(run *runPO) {
				run.Status = string(entity.RunStatusPending)
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "stale generation",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.Generation--
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "wrong owner",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.LeaseOwner = "worker-2"
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "wrong token",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.LeaseToken = "lease-2"
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "expiry equals now",
			mutateRun: func(run *runPO) {
				expiresAt := now
				run.LeaseExpiresAt = &expiresAt
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "cancel requested",
			mutateRun: func(run *runPO) {
				cancelRequestedAt := now - 1
				run.CancelRequestedAt = &cancelRequestedAt
			},
			expectedErr: ErrRunCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			seedJournalThread(t, db, 10)
			leaseOwner := "worker-1"
			leaseToken := "lease-1"
			leaseExpiresAt := now + 1
			run := runPO{
				ID: 20, ThreadID: 10, Status: string(entity.RunStatusRunning),
				LeaseOwner: &leaseOwner, LeaseToken: &leaseToken,
				LeaseExpiresAt: &leaseExpiresAt, ExecutionGeneration: 3,
			}
			if tt.mutateRun != nil {
				tt.mutateRun(&run)
			}
			require.NoError(t, db.Create(&run).Error)

			var before runPO
			require.NoError(t, db.Where("id = ?", run.ID).First(&before).Error)
			req := newAdaptiveExecutionBoundaryRequestForFenceTest()
			if tt.mutateRequest != nil {
				tt.mutateRequest(&req)
			}

			var locked *runPO
			err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				locked, err = lockAdaptiveExecutionRun(tx, req)
				return err
			})
			if tt.expectedErr == nil {
				require.NoError(t, err)
				require.Equal(t, before, *locked)
			} else {
				require.ErrorIs(t, err, tt.expectedErr)
			}

			var after runPO
			require.NoError(t, db.Where("id = ?", run.ID).First(&after).Error)
			require.Equal(t, before, after)
		})
	}
}

func TestLockAdaptiveExecutionAttempt(t *testing.T) {
	tests := []struct {
		name          string
		mutateAttempt func(*runAttemptPO)
		mutateRequest func(*CommitAdaptiveExecutionBoundaryRequest)
		expectedErr   error
	}{
		{name: "success"},
		{
			name: "terminal status",
			mutateAttempt: func(attempt *runAttemptPO) {
				attempt.Status = string(entity.RunAttemptStatusCompleted)
			},
			expectedErr: ErrAdaptiveExecutionAttemptConflict,
		},
		{
			name: "null active slot",
			mutateAttempt: func(attempt *runAttemptPO) {
				attempt.ActiveSlot = nil
			},
			expectedErr: ErrAdaptiveExecutionAttemptConflict,
		},
		{
			name: "execution run drift",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.ExecutionRunID++
			},
			expectedErr: ErrAdaptiveExecutionAttemptConflict,
		},
		{
			name: "cross-thread",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.ThreadID++
			},
			expectedErr: ErrAdaptiveExecutionAttemptConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			activeSlot := uint8(1)
			attempt := runAttemptPO{
				ID: 100, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 20,
				AttemptID: "attempt-1", Ordinal: 1,
				Status: string(entity.RunAttemptStatusRunning), ActiveSlot: &activeSlot,
				NextSequence: 7, LastCommittedSequence: 6,
				EnrollmentVersion: entity.JournalSchemaVersion,
				ProjectionState:   string(entity.JournalProjectionStateHealthy),
				CreatedAt:         1000, UpdatedAt: 1000,
			}
			if tt.mutateAttempt != nil {
				tt.mutateAttempt(&attempt)
			}
			require.NoError(t, db.Create(&attempt).Error)

			var before runAttemptPO
			require.NoError(t, db.Where("id = ?", attempt.ID).First(&before).Error)
			req := newAdaptiveExecutionBoundaryRequestForFenceTest()
			if tt.mutateRequest != nil {
				tt.mutateRequest(&req)
			}

			var locked *runAttemptPO
			err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				locked, err = lockAdaptiveExecutionAttempt(tx, req)
				return err
			})
			if tt.expectedErr == nil {
				require.NoError(t, err)
				require.Equal(t, before, *locked)
			} else {
				require.ErrorIs(t, err, tt.expectedErr)
			}

			var after runAttemptPO
			require.NoError(t, db.Where("id = ?", attempt.ID).First(&after).Error)
			require.Equal(t, before, after)
		})
	}
}

func newAdaptiveExecutionBoundaryRequestForFenceTest() CommitAdaptiveExecutionBoundaryRequest {
	return CommitAdaptiveExecutionBoundaryRequest{
		ThreadID:       10,
		ExecutionRunID: 20,
		JournalRunID:   30,
		AttemptID:      "attempt-1",
		Generation:     3,
		LeaseOwner:     "worker-1",
		LeaseToken:     "lease-1",
		Now:            1000,
		IdempotencyKey: "boundary-1",
		Event:          &entity.RunEvent{ID: 40, ThreadID: 10, RunID: 20},
		Checkpoint:     &entity.Checkpoint{ID: 50, ThreadID: 10, RunID: 20},
	}
}
