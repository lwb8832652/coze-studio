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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestHumanResumeRolloverCommitsExactAggregateAndAdjacentSequences(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedHumanResumeRolloverSource(t, db)
	req := humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604)

	created, err := repo.CreateRunBundle(context.Background(), req)

	require.NoError(t, err)
	require.True(t, created.Created)
	require.Equal(t, int64(60), created.Run.ID)
	require.Equal(t, int64(601), created.Message.ID)
	require.Equal(t, int64(602), created.Event.ID)
	require.Equal(t, int64(604), created.Attempt.ID)

	var source runAttemptPO
	require.NoError(t, db.Where("id = ?", 500).First(&source).Error)
	require.Equal(t, string(entity.RunAttemptStatusInterrupted), source.Status)
	require.Nil(t, source.ActiveSlot)
	require.NotNil(t, source.TerminalEventID)
	require.Equal(t, int64(603), *source.TerminalEventID)
	require.Equal(t, uint64(3), source.NextSequence)
	require.Equal(t, uint64(2), source.LastCommittedSequence)

	var rows []runEventPO
	require.NoError(t, db.Where("attempt_id = ?", "att_500").Order("sequence ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, uint64(1), *rows[0].Sequence)
	require.Equal(t, uint64(2), *rows[1].Sequence)
	require.Equal(t, int64(602), rows[0].ID)
	require.Equal(t, int64(603), rows[1].ID)
	require.Equal(t, int64(602), int64FromPtr(rows[1].ParentEventID))

	var target runAttemptPO
	require.NoError(t, db.Where("id = ?", 604).First(&target).Error)
	require.Equal(t, uint32(2), target.Ordinal)
	require.Equal(t, int64(40), target.JournalRunID)
	require.Equal(t, int64(60), target.ExecutionRunID)
	require.NotNil(t, target.ActiveSlot)
	require.Equal(t, uint8(1), *target.ActiveSlot)
}

func TestHumanResumeRolloverExactReplayAfterLifecycleChanges(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedHumanResumeRolloverSource(t, db)
	first := humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604)
	created, err := repo.CreateRunBundle(context.Background(), first)
	require.NoError(t, err)
	require.True(t, created.Created)

	startedAt := int64(400)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 60).
		Update("status", string(entity.RunStatusRunning)).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 604).
		Updates(map[string]any{
			"status": string(entity.RunAttemptStatusRunning), "started_at": startedAt,
			"updated_at": startedAt,
		}).Error)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 700).
		Update("runtime_deleted_at", int64(401)).Error)

	retry := humanResumeRolloverRequest("human-resume-1", 70, 701, 702, 703, 704)
	setHumanResumeRequestSubmittedAt(t, &retry, 999)
	replayed, err := repo.CreateRunBundle(context.Background(), retry)

	require.NoError(t, err)
	require.False(t, replayed.Created)
	require.Equal(t, int64(60), replayed.Run.ID)
	require.Equal(t, int64(601), replayed.Message.ID)
	require.Equal(t, int64(602), replayed.Event.ID)
	require.Equal(t, int64(604), replayed.Attempt.ID)
	var runCount, messageCount, eventCount, attemptCount int64
	require.NoError(t, db.Model(&runPO{}).Count(&runCount).Error)
	require.NoError(t, db.Model(&messagePO{}).Count(&messageCount).Error)
	require.NoError(t, db.Model(&runEventPO{}).Count(&eventCount).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Count(&attemptCount).Error)
	require.Equal(t, int64(3), runCount)
	require.Equal(t, int64(1), messageCount)
	require.Equal(t, int64(2), eventCount)
	require.Equal(t, int64(2), attemptCount)
}

func TestHumanResumeRolloverStableReplaySurvivesSourceLifecycleAndCheckpointDeletion(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	replayRepo, ok := repo.(HumanResumeRolloverReplayRepository)
	require.True(t, ok)
	seedHumanResumeRolloverSource(t, db)
	request := humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604)
	_, err := repo.CreateRunBundle(context.Background(), request)
	require.NoError(t, err)
	startedAt := int64(400)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 50).
		Update("status", string(entity.RunStatusSucceeded)).Error)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 60).
		Update("status", string(entity.RunStatusRunning)).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 604).
		Updates(map[string]any{
			"status": string(entity.RunAttemptStatusRunning), "started_at": startedAt,
			"updated_at": startedAt,
		}).Error)
	require.NoError(t, db.Where("id = ?", 700).Delete(&checkpointPO{}).Error)

	replayed, err := replayRepo.GetHumanResumeRolloverReplay(
		context.Background(), humanResumeRolloverReplayRequest("human-resume-1"),
	)

	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, int64(60), replayed.Run.ID)
	require.Equal(t, int64(601), replayed.Message.ID)
	require.Equal(t, int64(602), replayed.Event.ID)
	require.Equal(t, int64(604), replayed.Attempt.ID)
	require.Equal(t, int64(500), replayed.SourceAttempt.ID)
	require.Equal(t, int64(603), replayed.TerminalEvent.ID)
	var runCount, messageCount, eventCount, attemptCount int64
	require.NoError(t, db.Model(&runPO{}).Count(&runCount).Error)
	require.NoError(t, db.Model(&messagePO{}).Count(&messageCount).Error)
	require.NoError(t, db.Model(&runEventPO{}).Count(&eventCount).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Count(&attemptCount).Error)
	require.Equal(t, int64(3), runCount)
	require.Equal(t, int64(1), messageCount)
	require.Equal(t, int64(2), eventCount)
	require.Equal(t, int64(2), attemptCount)
}

func TestHumanResumeRolloverRejectsCrossArtifactDriftWithoutWrites(t *testing.T) {
	tests := []struct {
		name  string
		drift func(*testing.T, *CreateRunBundleRequest)
	}{
		{
			name: "command checkpoint differs from attempt lineage",
			drift: func(t *testing.T, req *CreateRunBundleRequest) {
				mutateHumanResumeJSON(t, &req.Run.Command, func(root map[string]any) {
					root["resume"].(map[string]any)["checkpoint_id"] = float64(701)
				})
			},
		},
		{
			name: "metadata source differs from rollover source",
			drift: func(t *testing.T, req *CreateRunBundleRequest) {
				mutateHumanResumeJSON(t, &req.Run.Metadata, func(root map[string]any) {
					root["checkpoint_resume"].(map[string]any)["source_run_id"] = float64(51)
				})
			},
		},
		{
			name: "command interrupt differs from resolved correlation",
			drift: func(t *testing.T, req *CreateRunBundleRequest) {
				mutateHumanResumeJSON(t, &req.Run.Command, func(root map[string]any) {
					targets := root["resume"].(map[string]any)["targets"].(map[string]any)
					targets["interrupt-other"] = targets["interrupt-1"]
					delete(targets, "interrupt-1")
				})
			},
		},
		{
			name: "command response differs from message and resolved facts",
			drift: func(t *testing.T, req *CreateRunBundleRequest) {
				mutateHumanResumeJSON(t, &req.Run.Command, func(root map[string]any) {
					target := root["resume"].(map[string]any)["targets"].(map[string]any)["interrupt-1"].(map[string]any)
					target["decision"] = "rejected"
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			repo := NewThreadRepository(db)
			seedHumanResumeRolloverSource(t, db)
			req := humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604)
			tt.drift(t, &req)

			_, err := repo.CreateRunBundle(context.Background(), req)

			require.ErrorContains(t, err, "human resume")
			assertHumanResumeTargetAbsent(t, db, 60, 601, 602, 603, 604)
			assertHumanResumeSourceStillActive(t, db)
		})
	}
}

func TestHumanResumeRolloverRejectsInvalidTargetAttemptIdentityWithoutWrites(t *testing.T) {
	tests := []struct {
		name  string
		drift func(*entity.RunAttempt)
	}{
		{name: "missing database id", drift: func(attempt *entity.RunAttempt) { attempt.ID = 0 }},
		{name: "missing public attempt id", drift: func(attempt *entity.RunAttempt) { attempt.AttemptID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			repo := NewThreadRepository(db)
			seedHumanResumeRolloverSource(t, db)
			req := humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604)
			tt.drift(req.Attempt)

			_, err := repo.CreateRunBundle(context.Background(), req)

			require.ErrorContains(t, err, "target attempt")
			assertHumanResumeTargetAbsent(t, db, 60, 601, 602, 603, 604)
			var attemptCount int64
			require.NoError(t, db.Model(&runAttemptPO{}).Where("execution_run_id = ?", 60).Count(&attemptCount).Error)
			require.Zero(t, attemptCount)
			assertHumanResumeSourceStillActive(t, db)
		})
	}
}

func TestHumanResumeRolloverRejectsNonCanonicalResolvedJournalKeyWithoutWrites(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedHumanResumeRolloverSource(t, db)
	req := humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604)
	req.EventJournal.IdempotencyKey = "journal:runtime-v1:" + strings.Repeat("0", 64)

	_, err := repo.CreateRunBundle(context.Background(), req)

	require.ErrorContains(t, err, "replay authority")
	assertHumanResumeTargetAbsent(t, db, 60, 601, 602, 603, 604)
	assertHumanResumeSourceStillActive(t, db)
}

func TestHumanResumeRolloverStableReplayRejectsNonCanonicalResolvedJournalKey(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	replayRepo, ok := repo.(HumanResumeRolloverReplayRepository)
	require.True(t, ok)
	seedHumanResumeRolloverSource(t, db)
	_, err := repo.CreateRunBundle(context.Background(),
		humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604))
	require.NoError(t, err)
	replay := humanResumeRolloverReplayRequest("human-resume-1")
	replay.ResolvedJournalKey = "journal:runtime-v1:" + strings.Repeat("0", 64)

	_, err = replayRepo.GetHumanResumeRolloverReplay(context.Background(), replay)

	require.ErrorContains(t, err, "replay authority")
}

func TestHumanResumeRolloverDifferentKeyLeavesNoOrphan(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedHumanResumeRolloverSource(t, db)
	_, err := repo.CreateRunBundle(context.Background(),
		humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604))
	require.NoError(t, err)

	_, err = repo.CreateRunBundle(context.Background(),
		humanResumeRolloverRequest("human-resume-2", 70, 701, 702, 703, 704))

	require.ErrorIs(t, err, ErrHumanResumeRolloverConflict)
	var orphanRuns, orphanMessages, orphanEvents, orphanAttempts int64
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 70).Count(&orphanRuns).Error)
	require.NoError(t, db.Model(&messagePO{}).Where("run_id = ?", 70).Count(&orphanMessages).Error)
	require.NoError(t, db.Model(&runEventPO{}).Where("id IN ?", []int64{702, 703}).Count(&orphanEvents).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 704).Count(&orphanAttempts).Error)
	require.Zero(t, orphanRuns)
	require.Zero(t, orphanMessages)
	require.Zero(t, orphanEvents)
	require.Zero(t, orphanAttempts)
}

func TestHumanResumeRolloverReplayRejectsAggregateDrift(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedHumanResumeRolloverSource(t, db)
	_, err := repo.CreateRunBundle(context.Background(),
		humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604))
	require.NoError(t, err)

	drifted := humanResumeRolloverRequest("human-resume-1", 70, 701, 702, 703, 704)
	drifted.Run.Command = strings.Replace(drifted.Run.Command, `"decision":"approved"`, `"decision":"rejected"`, 1)
	drifted.Run.Metadata = strings.Replace(drifted.Run.Metadata, `"decision":"approved"`, `"decision":"rejected"`, 1)
	drifted.Message.Content = "已拒绝执行"
	drifted.Message.Metadata = strings.Replace(drifted.Message.Metadata, `"decision":"approved"`, `"decision":"rejected"`, 1)
	drifted.Event.Payload = strings.Replace(drifted.Event.Payload, `"decision":"approved"`, `"decision":"rejected"`, 1)
	_, err = repo.CreateRunBundle(context.Background(), drifted)

	require.ErrorIs(t, err, ErrRunIdempotencyConflict)
	var targetCount int64
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 70).Count(&targetCount).Error)
	require.Zero(t, targetCount)
}

func TestHumanResumeRolloverRejectsSourceAndCheckpointDriftWithoutWrites(t *testing.T) {
	tests := []struct {
		name   string
		drift  func(*gorm.DB)
		assert func(*testing.T, error)
	}{
		{
			name: "source run is no longer interrupted",
			drift: func(db *gorm.DB) {
				require.NoError(t, db.Model(&runPO{}).Where("id = ?", 50).
					Update("status", string(entity.RunStatusFailed)).Error)
			},
			assert: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrHumanResumeRolloverConflict) },
		},
		{
			name: "source attempt is not active",
			drift: func(db *gorm.DB) {
				require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 500).
					Update("active_slot", nil).Error)
			},
			assert: func(t *testing.T, err error) { require.ErrorIs(t, err, ErrHumanResumeRolloverConflict) },
		},
		{
			name: "checkpoint was runtime deleted",
			drift: func(db *gorm.DB) {
				require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 700).
					Update("runtime_deleted_at", int64(1)).Error)
			},
			assert: func(t *testing.T, err error) { require.ErrorContains(t, err, "source checkpoint") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			repo := NewThreadRepository(db)
			seedHumanResumeRolloverSource(t, db)
			tt.drift(db)

			_, err := repo.CreateRunBundle(context.Background(),
				humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604))

			tt.assert(t, err)
			assertHumanResumeTargetAbsent(t, db, 60, 601, 602, 603, 604)
		})
	}
}

func TestHumanResumeRolloverRollsBackWhenTargetAttemptInsertFails(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedHumanResumeRolloverSource(t, db)
	req := humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 500)

	_, err := repo.CreateRunBundle(context.Background(), req)

	require.Error(t, err)
	assertHumanResumeTargetAbsent(t, db, 60, 601, 602, 603, 0)
	var source runAttemptPO
	require.NoError(t, db.Where("id = ?", 500).First(&source).Error)
	require.Equal(t, string(entity.RunAttemptStatusRunning), source.Status)
	require.NotNil(t, source.ActiveSlot)
	require.Equal(t, uint8(1), *source.ActiveSlot)
	require.Equal(t, uint64(1), source.NextSequence)
	require.Zero(t, source.LastCommittedSequence)
	require.Nil(t, source.EndedAt)
	require.Nil(t, source.TerminalEventID)
}

func TestHumanResumeRolloverRejectsLateAppendWithoutSequenceMutation(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedHumanResumeRolloverSource(t, db)
	_, err := repo.CreateRunBundle(context.Background(),
		humanResumeRolloverRequest("human-resume-1", 60, 601, 602, 603, 604))
	require.NoError(t, err)

	_, err = repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 900, ThreadID: 10, RunID: 50, JournalRunID: 40, AttemptID: "att_500",
		IdempotencyKey: "journal:late", SchemaVersion: entity.JournalSchemaVersion,
		Status: "running", Visibility: entity.JournalVisibilityUser,
		PayloadVersion: entity.JournalPayloadVersion, EventType: "memory.updated",
		Payload: `{"type":"memory","data":{}}`, CreatedAt: 400,
	})

	require.ErrorIs(t, err, ErrJournalAttemptTerminal)
	var source runAttemptPO
	require.NoError(t, db.Where("id = ?", 500).First(&source).Error)
	require.Equal(t, uint64(3), source.NextSequence)
	require.Equal(t, uint64(2), source.LastCommittedSequence)
	require.Equal(t, int64(603), int64FromPtr(source.TerminalEventID))
}

func assertHumanResumeTargetAbsent(
	t *testing.T,
	db *gorm.DB,
	runID, messageID, resolvedID, terminalID, attemptID int64,
) {
	t.Helper()
	for _, check := range []struct {
		model any
		id    int64
	}{
		{model: &runPO{}, id: runID},
		{model: &messagePO{}, id: messageID},
		{model: &runEventPO{}, id: resolvedID},
		{model: &runEventPO{}, id: terminalID},
		{model: &runAttemptPO{}, id: attemptID},
	} {
		if check.id == 0 {
			continue
		}
		var count int64
		require.NoError(t, db.Model(check.model).Where("id = ?", check.id).Count(&count).Error)
		require.Zero(t, count)
	}
}

func assertHumanResumeSourceStillActive(t *testing.T, db *gorm.DB) {
	t.Helper()
	var source runAttemptPO
	require.NoError(t, db.Where("id = ?", 500).First(&source).Error)
	require.Equal(t, string(entity.RunAttemptStatusRunning), source.Status)
	require.NotNil(t, source.ActiveSlot)
	require.Equal(t, uint8(1), *source.ActiveSlot)
	require.Equal(t, uint64(1), source.NextSequence)
	require.Zero(t, source.LastCommittedSequence)
	require.Nil(t, source.EndedAt)
	require.Nil(t, source.TerminalEventID)
}

func humanResumeRolloverReplayRequest(key string) HumanResumeRolloverReplayRequest {
	return HumanResumeRolloverReplayRequest{
		SpaceID: 10, ThreadID: 10, SourceRunID: 50, IdempotencyKey: key,
		IdempotencyOperation:   "workbench.run.resume.v1",
		IdempotencyFingerprint: strings.Repeat("a", 64),
		ResolvedJournalKey:     humanResumeResolvedJournalKey(50, "interrupt-1"),
		InterruptID:            "interrupt-1",
		Response: HumanResumeRolloverResponse{
			Schema: "coze.human_interaction_response.v1", InteractionID: "interrupt-1",
			Kind: "confirmation", Decision: "approved",
		},
		PersistMessageReference: true,
	}
}

func mutateHumanResumeJSON(t *testing.T, raw *string, mutate func(map[string]any)) {
	t.Helper()
	var root map[string]any
	require.NoError(t, json.Unmarshal([]byte(*raw), &root))
	mutate(root)
	encoded, err := json.Marshal(root)
	require.NoError(t, err)
	*raw = string(encoded)
}

func seedHumanResumeRolloverSource(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&messagePO{}, &checkpointPO{}))
	seedJournalThread(t, db, 10)
	root := newRepositoryTestRun(40, 10, entity.RunStatusInterrupted, 100)
	root.SpaceID, root.CreatorID, root.RunKind = 10, 20, entity.RunKindTask
	rootPO, err := runToPO(root)
	require.NoError(t, err)
	require.NoError(t, db.Create(rootPO).Error)
	source := newRepositoryTestRun(50, 10, entity.RunStatusInterrupted, 200)
	source.SpaceID, source.CreatorID, source.RunKind = 10, 20, entity.RunKindTask
	sourcePO, err := runToPO(source)
	require.NoError(t, err)
	require.NoError(t, db.Create(sourcePO).Error)
	activeSlot, startedAt := uint8(1), int64(210)
	traceID := "trace-human-resume"
	require.NoError(t, db.Create(runAttemptToPO(&entity.RunAttempt{
		ID: 500, ThreadID: 10, JournalRunID: 40, ExecutionRunID: 50,
		AttemptID: "att_500", Ordinal: 1, Status: entity.RunAttemptStatusRunning,
		ActiveSlot: &activeSlot, NextSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
		SnapshotsEnabled: true, ProjectionState: entity.JournalProjectionStateHealthy,
		TraceID: &traceID, StartedAt: &startedAt, CreatedAt: 200, UpdatedAt: 210,
	})).Error)
	require.NoError(t, db.Create(&checkpointPO{
		ID: 700, ThreadID: 10, RunID: 50, CheckpointNS: "eino.adk",
		RuntimeType: "eino_adk", RuntimeKey: "run-50", EnvelopeVersion: 3,
		ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`),
		PendingSends: []byte(`[]`), Metadata: []byte(`{}`), CreatedAt: 220,
	}).Error)
}

func humanResumeRolloverRequest(
	key string,
	runID, messageID, resolvedID, terminalID, attemptID int64,
) CreateRunBundleRequest {
	run := newRepositoryTestRun(runID, 10, entity.RunStatusQueued, 300)
	run.SpaceID, run.CreatorID, run.RunKind = 10, 20, entity.RunKindTask
	run.IdempotencyKey = key
	run.Command = `{"resume":{"checkpoint_id":700,"checkpoint_ns":"eino.adk","resume_from":"interrupt","targets":{"interrupt-1":{"schema":"coze.human_interaction_response.v1","interaction_id":"interrupt-1","kind":"confirmation","decision":"approved","submitted_at":300}}}}`
	run.Metadata = fmt.Sprintf(`{"checkpoint_resume":{"protected_from_worker_claim":true,"checkpoint_id":700,"checkpoint_ns":"eino.adk","resume_from":"interrupt","source_run_id":50},"human_interaction":{"schema":"coze.human_interaction_resolved.v1","thread_id":10,"source_run_id":50,"interrupt_id":"interrupt-1","interaction_id":"interrupt-1","kind":"confirmation","decision":"approved","submitted_at":300},"_idempotency":{"operation":"workbench.run.resume.v1","fingerprint":"%s"},"_message":{"message_id":%d}}`, strings.Repeat("a", 64), messageID)
	sourceCheckpointID := int64(700)
	sourceAttemptID := "att_500"
	return CreateRunBundleRequest{
		Run: run,
		Message: &entity.Message{
			ID: messageID, ThreadID: 10, RunID: runID, Role: entity.MessageRoleUser,
			Content: "已确认执行", Metadata: `{"human_interaction":{"schema":"coze.human_interaction_resolved.v1","source_run_id":50,"interrupt_id":"interrupt-1","interaction_id":"interrupt-1","kind":"confirmation","decision":"approved","submitted_at":300}}`, CreatedAt: 300,
		},
		Event: &entity.RunEvent{
			ID: resolvedID, ThreadID: 10, RunID: runID, EventType: "human.interaction.resolved",
			Payload: fmt.Sprintf(`{"schema":"coze.human_interaction_resolved.v1","thread_id":10,"source_run_id":50,"interrupt_id":"interrupt-1","interaction_id":"interrupt-1","kind":"confirmation","decision":"approved","submitted_at":300,"resume_run_id":%d}`, runID), CreatedAt: 300,
		},
		EventJournalSourceRunID: 50,
		EventJournal: &entity.JournalEvent{
			ID: resolvedID, ThreadID: 10, RunID: runID,
			IdempotencyKey: humanResumeResolvedJournalKey(50, "interrupt-1"), SchemaVersion: entity.JournalSchemaVersion,
			Status: "completed", Visibility: entity.JournalVisibilityUser,
			PayloadVersion: entity.JournalPayloadVersion, EventType: "confirmation.resolved",
			Payload:   `{"type":"confirmation","data":{"confirmation_id":"interrupt-1","confirmation_type":"confirmation","allowed_action_keys":[]}}`,
			CreatedAt: 300,
		},
		Attempt: &entity.RunAttempt{
			ID: attemptID, ThreadID: 10, JournalRunID: 40, ExecutionRunID: runID,
			AttemptID: fmt.Sprintf("att_%d", attemptID), Status: entity.RunAttemptStatusPending,
			SourceCheckpointID: &sourceCheckpointID, SourceAttemptID: &sourceAttemptID,
			RecoveryIdempotencyKey: &key, ProjectionState: entity.JournalProjectionStateHealthy,
		},
		HumanResumeRollover: &HumanResumeRolloverRequest{
			SourceRunID: 50,
			TerminalBase: &entity.RunEvent{
				ID: terminalID, ThreadID: 10, RunID: 50,
				EventType: entity.JournalAttemptInterruptedRunEventType,
				Payload:   fmt.Sprintf(`{"schema":"coze.journal_attempt_interrupted.v1","status":"interrupted","resume_run_id":%d}`, runID),
				CreatedAt: 300,
			},
			TerminalJournal: &entity.JournalEvent{
				ID: terminalID, ThreadID: 10, RunID: 50, ParentEventID: resolvedID,
				IdempotencyKey: "journal:run:50:terminal:interrupted", SchemaVersion: entity.JournalSchemaVersion,
				Status: string(entity.RunAttemptStatusInterrupted), Visibility: entity.JournalVisibilityUser,
				PayloadVersion: entity.JournalPayloadVersion, EventType: "run.lifecycle",
				Payload: `{"type":"terminal","data":{"status":"interrupted"}}`, CreatedAt: 300,
			},
		},
		ValidateIdempotencyReplay: true,
	}
}

func setHumanResumeRequestSubmittedAt(t *testing.T, req *CreateRunBundleRequest, submittedAt int64) {
	t.Helper()
	for _, target := range []*string{
		&req.Run.Command,
		&req.Run.Metadata,
		&req.Message.Metadata,
		&req.Event.Payload,
	} {
		var value any
		require.NoError(t, json.Unmarshal([]byte(*target), &value))
		setHumanResumeSubmittedAt(value, submittedAt)
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		*target = string(encoded)
	}
}

func setHumanResumeSubmittedAt(value any, submittedAt int64) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "submitted_at" {
				typed[key] = submittedAt
				continue
			}
			setHumanResumeSubmittedAt(child, submittedAt)
		}
	case []any:
		for _, child := range typed {
			setHumanResumeSubmittedAt(child, submittedAt)
		}
	}
}
