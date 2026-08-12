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
	"math"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdaptiveExecutionBootstrapCommitsInitialFactsWithoutPlan(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	require.NoError(t, db.Where("run_id = ?", int64(20)).Delete(&agentRunPlanItemPO{}).Error)
	require.NoError(t, db.Where("run_id = ?", int64(20)).Delete(&agentRunPlanPO{}).Error)

	var beforeAttempt runAttemptPO
	require.NoError(t, db.Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").First(&beforeAttempt).Error)

	result, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBootstrap(
		context.Background(),
		newAdaptiveExecutionBootstrapRequestForTest(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Replayed)
	require.Equal(t, int64(7001), result.Authority.AdmissionEventID)
	require.Equal(t, int64(7002), result.Authority.DecisionEventID)
	require.Equal(t, int64(8001), result.Authority.CheckpointID)

	var events []runEventPO
	require.NoError(t, db.Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, []string{adaptiveBootstrapAdmissionEventType, adaptiveBootstrapDecisionEventType}, []string{events[0].EventType, events[1].EventType})
	for _, event := range events {
		require.Equal(t, int64(20), event.RunID)
		require.Equal(t, int64(30), *event.JournalRunID)
		require.Equal(t, "attempt-1", *event.AttemptID)
		require.Nil(t, event.Sequence)
		require.Equal(t, "internal", *event.Visibility)
		require.Nil(t, event.JournalEventType)
		require.Empty(t, event.JournalPayload)
	}

	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", int64(8001)).First(&checkpoint).Error)
	require.Equal(t, adaptiveBootstrapRuntimeType, checkpoint.RuntimeType)
	require.Equal(t, adaptiveBootstrapCheckpointNS, checkpoint.CheckpointNS)
	require.Zero(t, checkpoint.ParentCheckpointID)

	var afterAttempt runAttemptPO
	require.NoError(t, db.Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").First(&afterAttempt).Error)
	require.Equal(t, beforeAttempt.NextSequence, afterAttempt.NextSequence)
	require.Equal(t, beforeAttempt.LastCommittedSequence, afterAttempt.LastCommittedSequence)
	var planCount int64
	require.NoError(t, db.Model(&agentRunPlanPO{}).Where("run_id = ?", int64(20)).Count(&planCount).Error)
	require.Zero(t, planCount)
}

func TestAdaptiveExecutionBootstrapReplaysExactFactWithoutWrites(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	require.NoError(t, db.Where("run_id = ?", int64(20)).Delete(&agentRunPlanItemPO{}).Error)
	require.NoError(t, db.Where("run_id = ?", int64(20)).Delete(&agentRunPlanPO{}).Error)
	repo := NewAdaptiveExecutionRepository(db)
	req := newAdaptiveExecutionBootstrapRequestForTest()

	first, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	req.Now++
	req.AdmissionEventID += 100
	req.DecisionEventID += 100
	req.CheckpointID += 100
	replayed, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBootstrap(context.Background(), req)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, first.Admission, replayed.Admission)
	require.Equal(t, first.Decision, replayed.Decision)
	require.Equal(t, first.Authority, replayed.Authority)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBootstrapRejectsDifferentOperationForExistingAttemptWithoutWrites(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	require.NoError(t, db.Where("run_id = ?", int64(20)).Delete(&agentRunPlanItemPO{}).Error)
	require.NoError(t, db.Where("run_id = ?", int64(20)).Delete(&agentRunPlanPO{}).Error)
	repo := NewAdaptiveExecutionRepository(db)
	req := newAdaptiveExecutionBootstrapRequestForTest()
	_, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), req)
	require.NoError(t, err)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	req.OperationKey = "bootstrap-2"
	_, err = repo.CommitAdaptiveExecutionBootstrap(context.Background(), req)
	require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBootstrapReaderDistinguishesMissingFromPartialFacts(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	read := ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	}

	_, err := repo.ReadAdaptiveExecutionBootstrap(context.Background(), read)
	require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapNotFound)

	committed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), newAdaptiveExecutionBootstrapRequestForTest())
	require.NoError(t, err)
	require.NoError(t, db.Where("id = ?", committed.Authority.CheckpointID).Delete(&checkpointPO{}).Error)

	_, err = repo.ReadAdaptiveExecutionBootstrap(context.Background(), read)
	require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
}

func TestAdaptiveExecutionBootstrapReaderTreatsMissingAttemptWithFactsAsConflict(t *testing.T) {
	read := ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	}

	t.Run("events and checkpoint remain", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		repo := NewAdaptiveExecutionRepository(db)
		_, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), newAdaptiveExecutionBootstrapRequestForTest())
		require.NoError(t, err)
		require.NoError(t, db.Model(&runAttemptPO{}).
			Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").
			Update("journal_run_id", int64(31)).Error)

		_, err = repo.ReadAdaptiveExecutionBootstrap(context.Background(), read)
		require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
	})

	t.Run("only deterministic checkpoint remains", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		repo := NewAdaptiveExecutionRepository(db)
		committed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), newAdaptiveExecutionBootstrapRequestForTest())
		require.NoError(t, err)
		require.NoError(t, db.Model(&runAttemptPO{}).
			Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").
			Update("journal_run_id", int64(31)).Error)
		require.NoError(t, db.Where("id IN ?", []int64{
			committed.Authority.AdmissionEventID,
			committed.Authority.DecisionEventID,
		}).Delete(&runEventPO{}).Error)

		_, err = repo.ReadAdaptiveExecutionBootstrap(context.Background(), read)
		require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
	})

	t.Run("all bootstrap facts removed", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		repo := NewAdaptiveExecutionRepository(db)
		committed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), newAdaptiveExecutionBootstrapRequestForTest())
		require.NoError(t, err)
		require.NoError(t, db.Model(&runAttemptPO{}).
			Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").
			Update("journal_run_id", int64(31)).Error)
		require.NoError(t, db.Where("id IN ?", []int64{
			committed.Authority.AdmissionEventID,
			committed.Authority.DecisionEventID,
		}).Delete(&runEventPO{}).Error)
		require.NoError(t, db.Where("id = ?", committed.Authority.CheckpointID).Delete(&checkpointPO{}).Error)

		_, err = repo.ReadAdaptiveExecutionBootstrap(context.Background(), read)
		require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapNotFound)
	})
}

func TestAdaptiveExecutionBootstrapReplaysBeforeMutableFencesAndReadsDurably(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	req := newAdaptiveExecutionBootstrapRequestForTest()
	first, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).Updates(map[string]any{
		"status":           string(entity.RunStatusSucceeded),
		"lease_expires_at": int64(1),
	}).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").
		Updates(map[string]any{"status": string(entity.RunAttemptStatusCompleted), "active_slot": nil}).Error)
	beforeReplay := snapshotAdaptiveExecutionDBForTest(t, db)

	replayed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), req)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, first.Authority, replayed.Authority)
	require.Equal(t, beforeReplay, snapshotAdaptiveExecutionDBForTest(t, db))

	read, err := repo.ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	})
	require.NoError(t, err)
	require.False(t, read.Replayed)
	require.Equal(t, first.Admission, read.Admission)
	require.Equal(t, first.Decision, read.Decision)
	require.Equal(t, first.Authority, read.Authority)
}

func TestAdaptiveExecutionBootstrapRejectsExtraReservedFactsAndFreshLineage(t *testing.T) {
	t.Run("extra reserved fact", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		repo := NewAdaptiveExecutionRepository(db)
		req := newAdaptiveExecutionBootstrapRequestForTest()
		_, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), req)
		require.NoError(t, err)
		journalRunID := int64(30)
		attemptID := "attempt-1"
		visibility := string(entity.JournalVisibilityInternal)
		require.NoError(t, db.Create(&runEventPO{
			ID: 7003, ThreadID: 10, RunID: 20, JournalRunID: &journalRunID, AttemptID: &attemptID,
			EventType: adaptiveBootstrapAdmissionEventType, Visibility: &visibility, Payload: []byte(`{}`), CreatedAt: 1000,
		}).Error)

		_, err = repo.ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
			ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
		})
		require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
		_, err = repo.CommitAdaptiveExecutionBootstrap(context.Background(), req)
		require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
	})

	for _, lineage := range []struct {
		name    string
		updates map[string]any
	}{
		{name: "source attempt", updates: map[string]any{"source_attempt_id": "source-attempt"}},
		{name: "source checkpoint", updates: map[string]any{"source_checkpoint_id": int64(99)}},
		{name: "recovery key", updates: map[string]any{"recovery_idempotency_key": "recovery-key"}},
	} {
		lineage := lineage
		t.Run(lineage.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			require.NoError(t, db.Model(&runAttemptPO{}).
				Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").
				Updates(lineage.updates).Error)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBootstrap(
				context.Background(),
				newAdaptiveExecutionBootstrapRequestForTest(),
			)
			require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBootstrapIgnoresSequencedPlanBoundaryDecision(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	bootstrapRequest := newAdaptiveExecutionBootstrapRequestForTest()
	committed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), bootstrapRequest)
	require.NoError(t, err)

	boundaryRequest := adaptiveInitialBoundaryRequest(7003, 8002, 1001)
	boundaryRequest.Event.EventType = adaptiveBootstrapDecisionEventType
	boundaryRequest.Event.Payload = `{"schema":"workbench-adaptive-decision.v1","decision":"multi_step"}`
	boundary, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), boundaryRequest)
	require.NoError(t, err)
	require.Equal(t, adaptiveBootstrapDecisionEventType, boundary.Event.EventType)
	require.Equal(t, int64(2), boundary.Plan.Revision)

	var planDecision runEventPO
	require.NoError(t, db.Where("id = ?", boundaryRequest.Event.ID).First(&planDecision).Error)
	require.NotNil(t, planDecision.Sequence)
	require.Equal(t, uint64(1), *planDecision.Sequence)
	require.True(t, planDecision.Visibility == nil || *planDecision.Visibility != string(entity.JournalVisibilityInternal))

	beforeRead := snapshotAdaptiveExecutionDBForTest(t, db)
	read, err := repo.ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	})
	require.NoError(t, err)
	require.Equal(t, committed.Authority, read.Authority)
	require.Equal(t, beforeRead, snapshotAdaptiveExecutionDBForTest(t, db))

	replayed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), bootstrapRequest)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, committed.Authority, replayed.Authority)
	require.Equal(t, beforeRead, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestLockAdaptiveBootstrapFactEventsSelectsOnlyPrivateUnsequencedFacts(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	journalRunID := int64(30)
	attemptID := "attempt-1"
	internal := string(entity.JournalVisibilityInternal)
	user := string(entity.JournalVisibilityUser)
	firstSequence := uint64(1)
	secondSequence := uint64(2)
	for _, event := range []*runEventPO{
		{
			ID: 7001, ThreadID: 10, RunID: 20, JournalRunID: &journalRunID, AttemptID: &attemptID,
			EventType: adaptiveBootstrapAdmissionEventType, Visibility: &internal, Payload: []byte(`{}`), CreatedAt: 1000,
		},
		{
			ID: 7002, ThreadID: 10, RunID: 20, JournalRunID: &journalRunID, AttemptID: &attemptID,
			EventType: adaptiveBootstrapDecisionEventType, Visibility: &internal, Payload: []byte(`{}`), CreatedAt: 1000,
		},
		{
			ID: 7003, ThreadID: 10, RunID: 20, JournalRunID: &journalRunID, AttemptID: &attemptID,
			EventType: adaptiveBootstrapDecisionEventType, Sequence: &firstSequence, Visibility: &user, Payload: []byte(`{}`), CreatedAt: 1001,
		},
		{
			ID: 7004, ThreadID: 10, RunID: 20, JournalRunID: &journalRunID, AttemptID: &attemptID,
			EventType: adaptiveBootstrapAdmissionEventType, Sequence: &secondSequence, Visibility: &internal, Payload: []byte(`{}`), CreatedAt: 1002,
		},
		{
			ID: 7005, ThreadID: 10, RunID: 20, JournalRunID: &journalRunID, AttemptID: &attemptID,
			EventType: adaptiveBootstrapDecisionEventType, Visibility: &user, Payload: []byte(`{}`), CreatedAt: 1003,
		},
	} {
		require.NoError(t, db.Create(event).Error)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		events, err := lockAdaptiveBootstrapFactEvents(tx, journalRunID, attemptID)
		require.NoError(t, err)
		require.Equal(t, []int64{7001, 7002}, adaptiveBootstrapEventIDsForTest(events))
		return nil
	})
	require.NoError(t, err)
}

func TestAdaptiveExecutionBootstrapPersistsEventFingerprintsAndRejectsTimestampOverflow(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	req := newAdaptiveExecutionBootstrapRequestForTest()
	committed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), req)
	require.NoError(t, err)
	require.True(t, adaptiveBootstrapLowerHex(committed.Authority.AdmissionEventFingerprint))
	require.True(t, adaptiveBootstrapLowerHex(committed.Authority.DecisionEventFingerprint))

	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", committed.Authority.CheckpointID).First(&checkpoint).Error)
	metadata, err := decodeAdaptiveBootstrapMetadata(checkpoint.Metadata)
	require.NoError(t, err)
	require.Equal(t, committed.Authority.AdmissionEventFingerprint, metadata.AdmissionEventFingerprint)
	require.Equal(t, committed.Authority.DecisionEventFingerprint, metadata.DecisionEventFingerprint)

	var admission runEventPO
	require.NoError(t, db.Where("id = ?", committed.Authority.AdmissionEventID).First(&admission).Error)
	fingerprint, err := adaptiveBootstrapEventFingerprint(&admission)
	require.NoError(t, err)
	require.Equal(t, metadata.AdmissionEventFingerprint, fingerprint)

	overflow := math.MaxInt64/int64(1_000_000) + 1
	overflowed := admission
	metadata.FactCreatedAt = overflow
	overflowed.CreatedAt = overflow
	occurredAt := overflow * int64(1_000_000)
	overflowed.OccurredAtUnixNano = &occurredAt
	err = validateAdaptiveBootstrapEvent(
		&overflowed,
		ReadAdaptiveExecutionBootstrapRequest{
			ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
		},
		metadata,
		adaptiveBootstrapAdmissionEventType,
		metadata.AdmissionEventKey,
		metadata.AdmissionDigest,
		metadata.AdmissionEventFingerprint,
	)
	require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
}

func TestAdaptiveExecutionBootstrapReaderRejectsEventKeysNotDerivedFromOperationDigest(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	committed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), newAdaptiveExecutionBootstrapRequestForTest())
	require.NoError(t, err)

	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", committed.Authority.CheckpointID).First(&checkpoint).Error)
	metadata, err := decodeAdaptiveBootstrapMetadata(checkpoint.Metadata)
	require.NoError(t, err)

	var admission, decision runEventPO
	require.NoError(t, db.Where("id = ?", committed.Authority.AdmissionEventID).First(&admission).Error)
	require.NoError(t, db.Where("id = ?", committed.Authority.DecisionEventID).First(&decision).Error)
	admissionKey := "other-admission-key"
	decisionKey := "other-decision-key"
	admission.IdempotencyKey = &admissionKey
	decision.IdempotencyKey = &decisionKey
	metadata.AdmissionEventKey = admissionKey
	metadata.DecisionEventKey = decisionKey
	metadata.AdmissionEventFingerprint, err = adaptiveBootstrapEventFingerprint(&admission)
	require.NoError(t, err)
	metadata.DecisionEventFingerprint, err = adaptiveBootstrapEventFingerprint(&decision)
	require.NoError(t, err)
	checkpointFingerprint, err := adaptiveBootstrapCheckpointFingerprint(&checkpoint, metadata)
	require.NoError(t, err)
	metadata.CheckpointFingerprint = checkpointFingerprint
	metadataRaw, err := json.Marshal(adaptiveBootstrapMetadataEnvelope{AdaptiveBootstrap: metadata})
	require.NoError(t, err)
	admission.SnapshotID = adaptiveExecutionStringPointer(checkpointFingerprint)
	decision.SnapshotID = adaptiveExecutionStringPointer(checkpointFingerprint)
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", admission.ID).Updates(map[string]any{
		"idempotency_key": admissionKey,
		"snapshot_id":     checkpointFingerprint,
	}).Error)
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", decision.ID).Updates(map[string]any{
		"idempotency_key": decisionKey,
		"snapshot_id":     checkpointFingerprint,
	}).Error)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", checkpoint.ID).Update("metadata", metadataRaw).Error)

	_, err = repo.ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	})
	require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
}

func TestAdaptiveExecutionBootstrapReaderRejectsNonCanonicalMetadata(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	committed, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), newAdaptiveExecutionBootstrapRequestForTest())
	require.NoError(t, err)

	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", committed.Authority.CheckpointID).First(&checkpoint).Error)
	require.NoError(t, db.Model(&checkpointPO{}).
		Where("id = ?", checkpoint.ID).
		Update("metadata", append([]byte(" \n"), checkpoint.Metadata...)).Error)

	_, err = repo.ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	})
	require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
}

func newAdaptiveExecutionBootstrapRequestForTest() CommitAdaptiveExecutionBootstrapRequest {
	return CommitAdaptiveExecutionBootstrapRequest{
		ThreadID:         10,
		ExecutionRunID:   20,
		JournalRunID:     30,
		AttemptID:        "attempt-1",
		Generation:       3,
		LeaseOwner:       "worker-1",
		LeaseToken:       "lease-1",
		OperationKey:     "bootstrap-1",
		Now:              1000,
		FactCreatedAt:    1000,
		AdmissionEventID: 7001,
		DecisionEventID:  7002,
		CheckpointID:     8001,
		Admission: entity.AdaptiveAdmissionSnapshot{
			Schema:             entity.AdaptiveAdmissionSchemaV1,
			FeatureGateEnabled: false,
			Source:             entity.AdaptiveAdmissionSourceFresh,
			Capabilities: entity.AdaptiveCapabilities{
				PlanAllowed: true, ReadOnlyToolsAllowed: true, SandboxWritesAllowed: false,
				HumanInteractionAllowed: true, SubagentsAllowed: false,
			},
			Limits: entity.AdaptiveLimits{
				MaxToolCalls: 24, MaxReplans: 2, MaxVerificationRepairs: 2,
				MaxConsecutiveNoProgress: 3, MaxActiveDurationSeconds: 1200,
			},
		},
		Decision: entity.ExecutionDecision{
			Schema: entity.ExecutionDecisionSchemaV1, DecisionID: "decision-1", DecisionRevision: 1,
			ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1", ExecutionGeneration: 3,
			GoalSummary: "complete the requested task", Deliverables: []string{"completed task"},
			AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{{
				CheckID: "check-1", Kind: "completion", TargetRef: "task:1", SafeDescription: "task completed",
			}},
			Decision: entity.ExecutionDecisionDirect, ExecutionShape: entity.ExecutionShapeEmpty,
			SafeSummary: "complete directly", CreatedAt: 1000,
		},
	}
}

func adaptiveBootstrapEventIDsForTest(events []runEventPO) []int64 {
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}

func TestAdaptiveExecutionBootstrapCandidateCheckpointCollisionRollsBackFactEvents(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	require.NoError(t, db.Create(&checkpointPO{
		ID:              8001,
		ThreadID:        99,
		RunID:           99,
		CheckpointNS:    "unrelated",
		RuntimeType:     "eino_adk",
		RuntimeKey:      "unrelated",
		EnvelopeVersion: 1,
		ChannelValues:   []byte(`{}`),
		ChannelVersions: []byte(`{}`),
		PendingSends:    []byte(`[]`),
		Metadata:        []byte(`{}`),
		CreatedAt:       999,
	}).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBootstrap(
		context.Background(),
		newAdaptiveExecutionBootstrapRequestForTest(),
	)
	require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))

	var factCount int64
	require.NoError(t, db.Model(&runEventPO{}).Where("id IN ?", []int64{7001, 7002}).Count(&factCount).Error)
	require.Zero(t, factCount)
}

func TestAdaptiveExecutionBootstrapRejectsNonFreshAdmissionWithoutWrites(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBootstrapRequest)
	}{
		{
			name: "typed inheritance",
			mutate: func(req *CommitAdaptiveExecutionBootstrapRequest) {
				sourceRunID, sourceGeneration := int64(9), uint64(2)
				req.Admission.Source = entity.AdaptiveAdmissionSourceTypedInheritance
				req.Admission.SourceRunID = &sourceRunID
				req.Admission.SourceExecutionGeneration = &sourceGeneration
			},
		},
		{
			name: "legacy decoder",
			mutate: func(req *CommitAdaptiveExecutionBootstrapRequest) {
				sourceRunID, sourceGeneration := int64(9), uint64(2)
				req.Admission.Source = entity.AdaptiveAdmissionSourceLegacyDecoder
				req.Admission.FeatureGateEnabled = false
				req.Admission.SourceRunID = &sourceRunID
				req.Admission.SourceExecutionGeneration = &sourceGeneration
				req.Admission.SourceConfigDigest = strings.Repeat("a", 64)
				req.Admission.DecoderVersion = entity.AdaptiveLegacyDecoderVersionV1
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := newAdaptiveExecutionBootstrapRequestForTest()
			test.mutate(&req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)

			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBootstrap(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapInvalid)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBootstrapRejectsCrossIdentityWithoutWrites(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBootstrapRequest)
		expected error
	}{
		{
			name: "cross thread journal root",
			setup: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBootstrapRequest) {
				req.ThreadID = 11
			},
			expected: ErrAdaptiveExecutionLineageConflict,
		},
		{
			name: "attempt bound to another execution run",
			setup: func(t *testing.T, db *gorm.DB, req *CommitAdaptiveExecutionBootstrapRequest) {
				seedAdaptiveBootstrapRunningRunForTest(t, db, 21)
				req.ExecutionRunID = 21
				req.Decision.ExecutionRunID = 21
			},
			expected: ErrAdaptiveExecutionAttemptConflict,
		},
		{
			name: "same attempt id has a different journal identity",
			setup: func(t *testing.T, db *gorm.DB, req *CommitAdaptiveExecutionBootstrapRequest) {
				seedAdaptiveBootstrapRunningRunForTest(t, db, 21)
				require.NoError(t, db.Create(&runPO{
					ID: 31, ThreadID: 10, SpaceID: 10, CreatorID: 20,
					RunKind: string(entity.RunKindTask), Status: string(entity.RunStatusSucceeded),
					CreatedAt: 600, UpdatedAt: 600,
				}).Error)
				activeSlot := uint8(1)
				require.NoError(t, db.Create(&runAttemptPO{
					ID: 101, ThreadID: 10, JournalRunID: 31, ExecutionRunID: 21,
					AttemptID: "attempt-1", Ordinal: 1, Status: string(entity.RunAttemptStatusRunning),
					ActiveSlot: &activeSlot, NextSequence: 1, LastCommittedSequence: 0,
					EnrollmentVersion: entity.JournalSchemaVersion,
					ProjectionState:   string(entity.JournalProjectionStateHealthy),
					CreatedAt:         700, UpdatedAt: 700,
				}).Error)
				req.JournalRunID = 31
				req.Decision.JournalRunID = 31
			},
			expected: ErrAdaptiveExecutionAttemptConflict,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := newAdaptiveExecutionBootstrapRequestForTest()
			test.setup(t, db, &req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)

			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBootstrap(context.Background(), req)
			require.ErrorIs(t, err, test.expected)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBootstrapReturnsDefensiveCopies(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := newAdaptiveExecutionBootstrapRequestForTest()
	planScopeRunID := int64(20)
	req.Decision.Decision = entity.ExecutionDecisionExecute
	req.Decision.ExecutionShape = entity.ExecutionShapeMultiStep
	req.Decision.PlanScopeRunID = &planScopeRunID

	committed, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBootstrap(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, committed.Decision.PlanScopeRunID)

	req.Decision.Deliverables[0] = "mutated request deliverable"
	req.Decision.AcceptanceChecks[0].SafeDescription = "mutated request check"
	*req.Decision.PlanScopeRunID = 99
	require.Equal(t, "completed task", committed.Decision.Deliverables[0])
	require.Equal(t, "task completed", committed.Decision.AcceptanceChecks[0].SafeDescription)
	require.Equal(t, int64(20), *committed.Decision.PlanScopeRunID)

	committed.Decision.Deliverables[0] = "mutated returned deliverable"
	committed.AdmissionEvent.Payload = `{"mutated":true}`
	committed.DecisionEvent.EventType = "mutated.event"
	committed.Checkpoint.Metadata = `{"mutated":true}`

	read, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	})
	require.NoError(t, err)
	require.Equal(t, "completed task", read.Decision.Deliverables[0])
	require.Equal(t, adaptiveBootstrapAdmissionEventType, read.AdmissionEvent.EventType)
	require.Equal(t, adaptiveBootstrapDecisionEventType, read.DecisionEvent.EventType)
	require.Contains(t, read.Checkpoint.Metadata, "adaptive_bootstrap")
}

func seedAdaptiveBootstrapRunningRunForTest(t *testing.T, db *gorm.DB, id int64) {
	t.Helper()
	leaseOwner := "worker-1"
	leaseToken := "lease-1"
	leaseExpiresAt := int64(2000)
	require.NoError(t, db.Create(&runPO{
		ID: id, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		RunKind: string(entity.RunKindTask), Status: string(entity.RunStatusRunning), ExecutionGeneration: 3,
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiresAt,
		CreatedAt: 700, UpdatedAt: 700,
	}).Error)
}
