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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const (
	journalTestPayload         = `{"type":"test","data":{}}`
	journalTerminalTestPayload = `{"type":"terminal","data":{}}`
)

func TestJournalFrozenMigrationAndModels(t *testing.T) {
	attemptMigration, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "docker", "atlas", "migrations",
		"20260730000100_agent_run_attempts.sql",
	))
	require.NoError(t, err)
	eventColumnsMigration, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "docker", "atlas", "migrations",
		"20260730000200_agent_run_events_journal_columns.sql",
	))
	require.NoError(t, err)
	eventIndexesMigration, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "docker", "atlas", "migrations",
		"20260730000210_agent_run_events_journal_indexes.sql",
	))
	require.NoError(t, err)

	attemptSQL := string(attemptMigration)
	for _, fragment := range []string{
		"`journal_run_id` bigint NOT NULL",
		"`execution_run_id` bigint NOT NULL",
		"`attempt_id` varchar(64) NOT NULL",
		"`ordinal` int unsigned NOT NULL",
		"`active_slot` tinyint unsigned DEFAULT NULL",
		"`next_sequence` bigint unsigned NOT NULL DEFAULT 1",
		"`last_committed_sequence` bigint unsigned NOT NULL DEFAULT 0",
		"`source_checkpoint_id` bigint DEFAULT NULL",
		"`source_attempt_id` varchar(64) DEFAULT NULL",
		"`recovery_idempotency_key` varchar(191) DEFAULT NULL",
		"`enrollment_version` varchar(32) NOT NULL",
		"`snapshots_enabled` tinyint unsigned NOT NULL DEFAULT 0",
		"`projection_state` varchar(16) NOT NULL DEFAULT 'healthy'",
		"`projection_degraded_at` bigint DEFAULT NULL",
		"`trace_id` varchar(128) DEFAULT NULL",
		"`started_at` bigint DEFAULT NULL",
		"uk_agent_run_attempts_execution",
		"uk_agent_run_attempts_identity",
		"uk_agent_run_attempts_ordinal",
		"uk_agent_run_attempts_active",
		"uk_agent_run_attempts_recovery_key",
		"pending", "running", "failed", "cancelled", "timed_out",
		"last_committed_sequence", "projection_state", "`active_slot` IS NOT NULL",
		"`last_committed_sequence` < `next_sequence`",
	} {
		require.Contains(t, attemptSQL, fragment)
	}

	eventColumnsSQL := string(eventColumnsMigration)
	for _, fragment := range []string{
		"`journal_run_id` bigint DEFAULT NULL",
		"`attempt_id` varchar(64) DEFAULT NULL",
		"`sequence` bigint unsigned DEFAULT NULL",
		"`idempotency_key` varchar(191) DEFAULT NULL",
		"`parent_event_id` bigint DEFAULT NULL",
		"`schema_version` varchar(16) DEFAULT NULL",
		"`status` varchar(32) DEFAULT NULL",
		"`occurred_at_unix_nano` bigint DEFAULT NULL",
		"`visibility` varchar(16) DEFAULT NULL",
		"`payload_version` varchar(16) DEFAULT NULL",
		"`snapshot_id` varchar(64) DEFAULT NULL",
		"`trace_id` varchar(128) DEFAULT NULL",
		"`action_id` varchar(191) DEFAULT NULL",
		"`phase` varchar(64) DEFAULT NULL",
	} {
		require.Contains(t, eventColumnsSQL, fragment)
	}

	eventIndexesSQL := string(eventIndexesMigration)
	for _, fragment := range []string{
		"(`journal_run_id`, `attempt_id`, `sequence`)",
		"(`journal_run_id`, `attempt_id`, `idempotency_key`)",
		"(`journal_run_id`, `attempt_id`, `action_id`, `phase`)",
	} {
		require.Contains(t, eventIndexesSQL, fragment)
	}
	require.NotContains(t, eventIndexesSQL, "idx_agent_run_events_journal_cursor")

	db := newJournalRepositoryTestDB(t)
	for _, column := range []string{
		"journal_run_id", "execution_run_id", "attempt_id", "ordinal", "active_slot",
		"next_sequence", "last_committed_sequence", "source_checkpoint_id", "source_attempt_id",
		"recovery_idempotency_key", "enrollment_version", "snapshots_enabled", "projection_state",
		"projection_degraded_at", "trace_id", "terminal_event_id", "started_at", "ended_at",
	} {
		require.Truef(t, db.Migrator().HasColumn(&runAttemptPO{}, column), "missing attempt column %s", column)
	}
	for _, index := range []string{
		"uk_agent_run_attempts_execution", "uk_agent_run_attempts_identity",
		"uk_agent_run_attempts_ordinal", "uk_agent_run_attempts_active",
		"uk_agent_run_attempts_recovery_key",
	} {
		require.Truef(t, db.Migrator().HasIndex(&runAttemptPO{}, index), "missing attempt index %s", index)
	}
	for _, column := range []string{
		"journal_run_id", "attempt_id", "sequence", "idempotency_key", "parent_event_id",
		"schema_version", "status", "occurred_at_unix_nano", "visibility", "payload_version",
		"snapshot_id", "trace_id", "action_id", "phase", "operation", "target", "milestone",
	} {
		require.Truef(t, db.Migrator().HasColumn(&runEventPO{}, column), "missing event column %s", column)
	}
	for _, index := range []string{
		"uk_agent_run_events_attempt_sequence", "uk_agent_run_events_attempt_idempotency",
		"uk_agent_run_events_action_phase",
	} {
		require.Truef(t, db.Migrator().HasIndex(&runEventPO{}, index), "missing event index %s", index)
	}
	require.False(t, db.Migrator().HasIndex(&runEventPO{}, "idx_agent_run_events_journal_cursor"))
}

func TestJournalAttemptFrozenFieldsRoundTrip(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	sourceCheckpointID := int64(44)
	sourceAttemptID := "att_source"
	recoveryKey := "recover-1"
	degradedAt := int64(1700)
	traceID := "trace-1"
	terminalEventID := int64(90)
	startedAt := int64(1200)
	endedAt := int64(1800)
	activeSlot := uint8(1)

	po := &runAttemptPO{
		ID: 1, ThreadID: 2, JournalRunID: 3, ExecutionRunID: 4,
		AttemptID: "att_1", Ordinal: 2, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &activeSlot, NextSequence: 8, LastCommittedSequence: 7,
		SourceCheckpointID: &sourceCheckpointID, SourceAttemptID: &sourceAttemptID,
		RecoveryIdempotencyKey: &recoveryKey, EnrollmentVersion: "1.1", SnapshotsEnabled: true,
		ProjectionState: string(entity.JournalProjectionStateDegraded), ProjectionDegradedAt: &degradedAt,
		TraceID: &traceID, TerminalEventID: &terminalEventID,
		CreatedAt: 1000, UpdatedAt: 1800, StartedAt: &startedAt, EndedAt: &endedAt,
	}
	require.NoError(t, db.Create(po).Error)
	var stored runAttemptPO
	require.NoError(t, db.First(&stored, 1).Error)
	attempt := stored.toEntity()
	require.Equal(t, &entity.RunAttempt{
		ID: 1, ThreadID: 2, JournalRunID: 3, ExecutionRunID: 4,
		AttemptID: "att_1", Ordinal: 2, Status: entity.RunAttemptStatusRunning,
		ActiveSlot: &activeSlot, NextSequence: 8, LastCommittedSequence: 7,
		SourceCheckpointID: &sourceCheckpointID, SourceAttemptID: &sourceAttemptID,
		RecoveryIdempotencyKey: &recoveryKey, EnrollmentVersion: "1.1", SnapshotsEnabled: true,
		ProjectionState: entity.JournalProjectionStateDegraded, ProjectionDegradedAt: &degradedAt,
		TraceID: &traceID, TerminalEventID: &terminalEventID,
		CreatedAt: 1000, UpdatedAt: 1800, StartedAt: &startedAt, EndedAt: &endedAt,
	}, attempt)
}

func TestJournalRunBundleFreezesEnrollmentAndReplaySemantics(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalThread(t, db, 1)

	firstRun := newRepositoryTestRun(10, 1, entity.RunStatusPending, 1)
	firstRun.SpaceID = 10
	firstRun.CreatorID = 20
	firstRun.RunKind = entity.RunKindTask
	firstRun.IdempotencyKey = "bundle-journal"
	traceID := "trace-frozen"
	first, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: firstRun,
		Attempt: &entity.RunAttempt{
			ID: 100, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
			AttemptID: "att_100", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, LastCommittedSequence: 0, EnrollmentVersion: "1.1",
			SnapshotsEnabled: true, ProjectionState: entity.JournalProjectionStateHealthy,
			TraceID: &traceID,
		},
	})
	require.NoError(t, err)
	require.True(t, first.Created)
	require.Equal(t, "att_100", first.Attempt.AttemptID)
	require.Equal(t, entity.RunAttemptStatusPending, first.Attempt.Status)
	require.Equal(t, "1.1", first.Attempt.EnrollmentVersion)
	require.True(t, first.Attempt.SnapshotsEnabled)
	require.Equal(t, traceID, *first.Attempt.TraceID)

	replayRun := newRepositoryTestRun(11, 1, entity.RunStatusPending, 2)
	replayRun.SpaceID = 10
	replayRun.CreatorID = 20
	replayRun.RunKind = entity.RunKindTask
	replayRun.IdempotencyKey = "bundle-journal"
	replayed, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: replayRun,
		Attempt: &entity.RunAttempt{
			ID: 101, ThreadID: 1, JournalRunID: 11, ExecutionRunID: 11,
			AttemptID: "att_101", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: "1.1", SnapshotsEnabled: true,
			ProjectionState: entity.JournalProjectionStateHealthy, TraceID: &traceID,
		},
	})
	require.NoError(t, err)
	require.False(t, replayed.Created)
	require.Equal(t, first.Run.ID, replayed.Run.ID)
	require.Equal(t, first.Attempt.ID, replayed.Attempt.ID)
	require.Equal(t, first.Attempt.AttemptID, replayed.Attempt.AttemptID)

	driftTraceID := "trace-drift"
	replayedWithNewTrace, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: replayRun,
		Attempt: &entity.RunAttempt{
			ID: 102, ThreadID: 1, JournalRunID: 11, ExecutionRunID: 11,
			AttemptID: "att_102", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: "1.1", SnapshotsEnabled: true,
			ProjectionState: entity.JournalProjectionStateHealthy, TraceID: &driftTraceID,
		},
	})
	require.NoError(t, err)
	require.False(t, replayedWithNewTrace.Created)
	require.Equal(t, first.Attempt.ID, replayedWithNewTrace.Attempt.ID)
	require.Equal(t, traceID, *replayedWithNewTrace.Attempt.TraceID)

	_, err = repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: replayRun,
		Attempt: &entity.RunAttempt{
			ID: 103, ThreadID: 1, JournalRunID: 11, ExecutionRunID: 11,
			AttemptID: "att_103", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: "1.1", SnapshotsEnabled: false,
			ProjectionState: entity.JournalProjectionStateHealthy, TraceID: &driftTraceID,
		},
	})
	require.ErrorIs(t, err, ErrRunIdempotencyConflict)

	_, err = repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: replayRun,
		Attempt: &entity.RunAttempt{
			ID: 104, ThreadID: 1, JournalRunID: 11, ExecutionRunID: 11,
			AttemptID: "att_104", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: "1.2", SnapshotsEnabled: true,
			ProjectionState: entity.JournalProjectionStateHealthy,
		},
	})
	require.ErrorIs(t, err, ErrRunIdempotencyConflict)
}

func TestJournalRunBundleRejectsUnsupportedEnrollmentVersion(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalThread(t, db, 1)
	run := newRepositoryTestRun(10, 1, entity.RunStatusPending, 1)
	run.SpaceID = 10
	run.CreatorID = 20
	run.RunKind = entity.RunKindTask

	_, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: run,
		Attempt: &entity.RunAttempt{
			ID: 100, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
			AttemptID: "att_100", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: "2.0",
			ProjectionState: entity.JournalProjectionStateHealthy,
		},
	})
	require.Error(t, err)
	var runCount, attemptCount int64
	require.NoError(t, db.Model(&runPO{}).Count(&runCount).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Count(&attemptCount).Error)
	require.Zero(t, runCount)
	require.Zero(t, attemptCount)
}

func TestJournalRunBundleRejectsAddingEnrollmentDuringReplay(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalThread(t, db, 1)
	firstRun := newRepositoryTestRun(10, 1, entity.RunStatusPending, 1)
	firstRun.SpaceID = 10
	firstRun.CreatorID = 20
	firstRun.RunKind = entity.RunKindTask
	firstRun.IdempotencyKey = "bundle-add-enrollment"
	first, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{Run: firstRun})
	require.NoError(t, err)
	require.True(t, first.Created)
	require.Nil(t, first.Attempt)

	replayRun := *firstRun
	replayRun.ID = 11
	_, err = repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: &replayRun,
		Attempt: &entity.RunAttempt{
			ID: 100, ThreadID: 1, JournalRunID: 11, ExecutionRunID: 11,
			AttemptID: "att_100", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
			ProjectionState: entity.JournalProjectionStateHealthy,
		},
	})
	require.ErrorIs(t, err, ErrRunIdempotencyConflict)
}

func TestJournalAppendValidatesEnvelopeAndFreezesDefaults(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusPending, 1)

	_, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, EventType: "run.lifecycle", Payload: journalTestPayload,
	})
	require.Error(t, err)
	_, err = repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1001, ThreadID: 1, RunID: 10, IdempotencyKey: "bad-envelope",
		EventType: "run.lifecycle", Payload: `{}`,
	})
	require.Error(t, err)

	event, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1002, ThreadID: 1, RunID: 10, IdempotencyKey: "defaults",
		EventType: "run.lifecycle", Payload: journalTestPayload,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), event.JournalRunID)
	require.Equal(t, "att_100", event.AttemptID)
	require.Equal(t, uint64(1), event.Sequence)
	require.Equal(t, entity.JournalVisibilityUser, event.Visibility)
	require.Equal(t, "1.1", event.SchemaVersion)
	require.Equal(t, "1.0", event.PayloadVersion)
	require.Positive(t, event.OccurredAtUnixNano)

	attempt, err := repo.GetActiveJournalAttempt(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, entity.RunAttemptStatusRunning, attempt.Status)
	require.NotNil(t, attempt.StartedAt)
	require.Equal(t, uint64(2), attempt.NextSequence)
	// Ordinary Journal persistence is not a recoverable execution boundary.
	// Task 7 advances this field only with the checkpoint/ledger transaction.
	require.Zero(t, attempt.LastCommittedSequence)
}

func TestDisableActiveJournalProjectionIsPermanentAndIdempotent(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)

	disabled, changed, err := repo.DisableActiveJournalProjection(context.Background(), 10, 2_000)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, entity.JournalProjectionStateDisabled, disabled.ProjectionState)
	require.Equal(t, int64(2_000), disabled.UpdatedAt)

	disabledAgain, changed, err := repo.DisableActiveJournalProjection(context.Background(), 10, 3_000)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, entity.JournalProjectionStateDisabled, disabledAgain.ProjectionState)
	require.Equal(t, int64(2_000), disabledAgain.UpdatedAt)

	_, err = repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "after-disabled",
		EventType: "tool.started", Payload: journalTestPayload,
	})
	require.ErrorIs(t, err, ErrJournalProjectionInactive)
}

func TestRunEventJournalProjectionKeepsBaseAndJournalViews(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	projectionRepo, ok := repo.(RunEventProjectionRepository)
	require.True(t, ok)

	base := &entity.RunEvent{
		ID: 1000, ThreadID: 1, RunID: 10, EventType: "tool.completed",
		Payload:   `{"tool_name":"read_file","tool_call_id":"call-1","result":"private output"}`,
		CreatedAt: 1000,
	}
	projected, err := projectionRepo.CreateRunEventWithJournalProjection(
		context.Background(),
		CreateRunEventWithJournalProjectionRequest{
			Event: base,
			Journal: &entity.JournalEvent{
				ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "call-1:terminal",
				ActionID: "action-1", Phase: "terminal", Operation: "read", Target: "文件",
				EventType: "action.terminal", Status: "completed",
				Payload:   `{"type":"document","data":{"action_id":"action-1","operation":"read","target":"文件","display_verb_running":"正在读取","display_verb_completed":"已读取"}}`,
				CreatedAt: 1000,
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, projected)
	require.Equal(t, uint64(1), projected.Sequence)
	require.Equal(t, "action.terminal", projected.EventType)

	baseEvents, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID: 10, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, baseEvents, 1)
	require.Equal(t, "tool.completed", baseEvents[0].EventType)
	require.Contains(t, baseEvents[0].Payload, "private output")

	journalEvents, err := repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, journalEvents.Events, 1)
	require.Equal(t, "action.terminal", journalEvents.Events[0].EventType)
	require.NotContains(t, journalEvents.Events[0].Payload, "private output")
}

func TestCreateRunBundleProjectsResolvedConfirmationToSourceAttemptAtomically(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRunWithStatus(t, db, 10, 1, entity.RunStatusInterrupted)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)

	resumeRun := newRepositoryTestRun(11, 1, entity.RunStatusPending, 2)
	resumeRun.SpaceID = 10
	resumeRun.CreatorID = 20
	resumeRun.RunKind = entity.RunKindTask
	resumeRun.IdempotencyKey = "resume-confirmation-1"
	base := &entity.RunEvent{
		ID: 1000, ThreadID: 1, RunID: 11, EventType: "human.interaction.resolved",
		Payload: `{"interrupt_id":"interrupt-1","resume_run_id":11}`, CreatedAt: 1000,
	}
	journal := &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 11,
		IdempotencyKey: "journal:confirmation:interrupt-1:resolved",
		EventType:      "confirmation.resolved", Status: "completed",
		Visibility: entity.JournalVisibilityUser,
		Payload:    `{"type":"confirmation","data":{"confirmation_id":"interrupt-1","confirmation_type":"confirmation","allowed_action_keys":[]}}`,
		CreatedAt:  1000,
	}

	created, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: resumeRun, Event: base,
		EventJournalSourceRunID: 10,
		EventJournal:            journal,
	})
	require.NoError(t, err)
	require.True(t, created.Created)

	baseEvents, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID: 11, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, baseEvents, 1)
	require.Equal(t, "human.interaction.resolved", baseEvents[0].EventType)

	journalEvents, err := repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, AttemptID: "att_100", Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, journalEvents.Events, 1)
	require.Equal(t, "confirmation.resolved", journalEvents.Events[0].EventType)
	require.Equal(t, int64(11), journalEvents.Events[0].RunID)
	require.Equal(t, uint64(1), journalEvents.Events[0].Sequence)
}

func TestRunEventJournalProjectionDegradesOnceAndKeepsBaseEvents(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	projectionRepo, ok := repo.(RunEventProjectionRepository)
	require.True(t, ok)

	_, err := projectionRepo.CreateRunEventWithJournalProjection(
		context.Background(),
		CreateRunEventWithJournalProjectionRequest{
			Event: &entity.RunEvent{
				ID: 1000, ThreadID: 1, RunID: 10, EventType: "tool.completed", Payload: `{}`, CreatedAt: 1000,
			},
			Journal: &entity.JournalEvent{
				ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "invalid",
				EventType: "action.terminal", Payload: `{}`, CreatedAt: 1000,
			},
		},
	)
	require.NoError(t, err)
	firstAttempt, err := repo.GetActiveJournalAttempt(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, entity.JournalProjectionStateDegraded, firstAttempt.ProjectionState)
	require.NotNil(t, firstAttempt.ProjectionDegradedAt)
	degradedAt := *firstAttempt.ProjectionDegradedAt

	_, err = projectionRepo.CreateRunEventWithJournalProjection(
		context.Background(),
		CreateRunEventWithJournalProjectionRequest{
			Event: &entity.RunEvent{
				ID: 1001, ThreadID: 1, RunID: 10, EventType: "tool.completed", Payload: `{}`, CreatedAt: 2000,
			},
			Journal: &entity.JournalEvent{
				ID: 1001, ThreadID: 1, RunID: 10, IdempotencyKey: "valid-after-degrade",
				ActionID: "action-1", Phase: "terminal", Operation: "read", Target: "文件",
				EventType: "action.terminal", Status: "completed",
				Payload: `{"type":"document","data":{"action_id":"action-1"}}`, CreatedAt: 2000,
			},
		},
	)
	require.NoError(t, err)
	secondAttempt, err := repo.GetActiveJournalAttempt(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, degradedAt, *secondAttempt.ProjectionDegradedAt)
	require.Equal(t, uint64(1), secondAttempt.NextSequence)

	baseEvents, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID: 10, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, baseEvents, 2)
	journalEvents, err := repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, Limit: 10,
	})
	require.NoError(t, err)
	require.Empty(t, journalEvents.Events)
}

func TestJournalSequenceAllocationFailureCanFallbackWithoutPartialProjection(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)

	err := db.Transaction(func(tx *gorm.DB) error {
		var staleAttempt runAttemptPO
		if err := tx.Where("id = ?", 100).First(&staleAttempt).Error; err != nil {
			return err
		}
		if err := tx.Model(&runAttemptPO{}).Where("id = ?", 100).
			Update("next_sequence", 2).Error; err != nil {
			return err
		}
		base := &entity.RunEvent{
			ID: 1000, ThreadID: 1, RunID: 10, EventType: "tool.completed",
			Payload: `{"tool_call_id":"call-1"}`, CreatedAt: 1000,
		}
		_, appendErr := appendJournalEventLockedWithBase(tx, &staleAttempt, &entity.JournalEvent{
			ID: 1000, ThreadID: 1, RunID: 10, JournalRunID: 10, AttemptID: "att_100",
			IdempotencyKey: "call-1:terminal", SchemaVersion: entity.JournalSchemaVersion,
			Status: "completed", Visibility: entity.JournalVisibilityUser,
			PayloadVersion: entity.JournalPayloadVersion,
			ActionID:       "action-1", Phase: "terminal", Operation: "read", Target: "文件",
			EventType: "action.terminal",
			Payload:   `{"type":"document","data":{"action_id":"action-1"}}`, CreatedAt: 1000,
		}, base)
		if !errors.Is(appendErr, ErrJournalSequenceAllocation) {
			return fmt.Errorf("expected sequence allocation failure, got %w", appendErr)
		}
		basePO, err := runEventToPO(base)
		if err != nil {
			return err
		}
		if err := createBaseRunEvent(tx, basePO); err != nil {
			return err
		}
		return markJournalProjectionDegraded(tx, &staleAttempt, 1000)
	})
	require.NoError(t, err)

	var rows []runEventPO
	require.NoError(t, db.Where("run_id = ?", 10).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "tool.completed", rows[0].EventType)
	require.Nil(t, rows[0].JournalEventType)
	require.Empty(t, rows[0].JournalPayload)
	var attempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, string(entity.JournalProjectionStateDegraded), attempt.ProjectionState)
}

func TestRunCancellationFinalizesJournalAttemptWithSameBaseRow(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	base := &entity.RunEvent{
		ID: 1000, ThreadID: 1, RunID: 10, EventType: "run.canceled",
		Payload: `{"status":"canceled"}`, CreatedAt: 1000,
	}
	journal := &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "run-10:cancelled",
		EventType: "run.lifecycle", Status: string(entity.RunAttemptStatusCancelled),
		Visibility: entity.JournalVisibilityUser,
		Payload:    `{"type":"terminal","data":{"status":"cancelled"}}`, CreatedAt: 1000,
	}

	result, err := repo.RequestRunCancellation(context.Background(), RequestRunCancellationRequest{
		RunID: 10, Now: 1000, ErrorCode: "run_canceled", ErrorMessage: "canceled",
		Event: base, JournalEvent: journal,
	})

	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Equal(t, entity.RunStatusCanceled, result.Run.Status)
	attempts, err := repo.ListJournalAttempts(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, attempts, 1)
	require.Equal(t, entity.RunAttemptStatusCancelled, attempts[0].Status)
	require.Nil(t, attempts[0].ActiveSlot)
	require.NotNil(t, attempts[0].TerminalEventID)
	require.Equal(t, int64(1000), *attempts[0].TerminalEventID)
	require.Equal(t, uint64(2), attempts[0].NextSequence)

	baseEvents, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID: 10, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "run.canceled", baseEvents[0].EventType)
	journalEvents, err := repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, journalEvents.Events, 1)
	require.Equal(t, "run.lifecycle", journalEvents.Events[0].EventType)
}

func TestRunCancellationKeepsBaseEventWhenTerminalProjectionIsInvalid(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)

	result, err := repo.RequestRunCancellation(context.Background(), RequestRunCancellationRequest{
		RunID: 10, Now: 1000, ErrorCode: "run_canceled", ErrorMessage: "canceled",
		Event: &entity.RunEvent{
			ID: 1000, ThreadID: 1, RunID: 10, EventType: "run.canceled",
			Payload: `{"status":"canceled"}`, CreatedAt: 1000,
		},
		JournalEvent: &entity.JournalEvent{
			ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "invalid-terminal",
			EventType: "action.terminal", Status: "completed",
			Payload: `{"type":"terminal","data":{"status":"completed"}}`, CreatedAt: 1000,
		},
	})

	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Equal(t, entity.RunStatusCanceled, result.Run.Status)
	attempts, err := repo.ListJournalAttempts(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, attempts, 1)
	require.Equal(t, entity.RunAttemptStatusCancelled, attempts[0].Status)
	require.Equal(t, entity.JournalProjectionStateDegraded, attempts[0].ProjectionState)
	require.NotNil(t, attempts[0].ProjectionDegradedAt)
	require.Equal(t, uint64(1), attempts[0].NextSequence)

	baseEvents, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID: 10, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "run.canceled", baseEvents[0].EventType)
	journalEvents, err := repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, Limit: 10,
	})
	require.NoError(t, err)
	require.Empty(t, journalEvents.Events)
}

func TestJournalAppendRejectsTerminalLifecycleOutsideFinalize(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)

	_, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "terminal-bypass",
		EventType: "run.lifecycle", Status: string(entity.RunAttemptStatusCompleted),
		Payload: journalTerminalTestPayload,
	})
	require.ErrorIs(t, err, ErrJournalInvalidStateTransition)

	var eventCount int64
	require.NoError(t, db.Model(&runEventPO{}).Count(&eventCount).Error)
	require.Zero(t, eventCount)
	var attempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, string(entity.RunAttemptStatusRunning), attempt.Status)
	require.NotNil(t, attempt.ActiveSlot)
	require.Equal(t, uint64(1), attempt.NextSequence)
	require.Nil(t, attempt.TerminalEventID)
}

func TestJournalFinalizeRejectsNonTerminalReplayAndKeepsAttemptActive(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	progress := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "not-terminal",
		ActionID: "action-progress", Phase: "progress", Operation: "read", Target: "report.txt",
		EventType: "action.progress", Payload: journalTestPayload,
	})

	_, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
		RunID: 10, Status: entity.RunAttemptStatusFailed,
		Event: &entity.JournalEvent{
			ID: 2000, ThreadID: 1, RunID: 10, IdempotencyKey: progress.IdempotencyKey,
			EventType: "run.lifecycle", Status: string(entity.RunAttemptStatusFailed),
			Payload: journalTerminalTestPayload,
		},
	})
	require.ErrorIs(t, err, ErrJournalTerminalReplayConflict)
	require.False(t, won)
	attempt, getErr := repo.GetActiveJournalAttempt(context.Background(), 10)
	require.NoError(t, getErr)
	require.Equal(t, entity.RunAttemptStatusRunning, attempt.Status)
	require.Nil(t, attempt.TerminalEventID)
	require.Equal(t, uint64(2), attempt.NextSequence)
}

func TestJournalFinalizeTrueTerminalReplayReturnsOriginal(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	req := FinalizeJournalAttemptRequest{
		RunID: 10, Status: entity.RunAttemptStatusCancelled,
		Event: &entity.JournalEvent{
			ID: 2000, ThreadID: 1, RunID: 10, IdempotencyKey: "terminal-cancelled",
			EventType: "run.lifecycle", Status: string(entity.RunAttemptStatusCancelled),
			Payload: journalTerminalTestPayload,
		},
	}
	first, won, err := repo.FinalizeJournalAttempt(context.Background(), req)
	require.NoError(t, err)
	require.True(t, won)

	req.Event = &entity.JournalEvent{
		ID: 2001, ThreadID: 1, RunID: 10, IdempotencyKey: "terminal-cancelled",
		EventType: "run.lifecycle", Status: string(entity.RunAttemptStatusCancelled),
		Payload: journalTerminalTestPayload,
	}
	replayed, won, err := repo.FinalizeJournalAttempt(context.Background(), req)
	require.NoError(t, err)
	require.False(t, won)
	require.Equal(t, first.ID, replayed.ID)
	require.Equal(t, first.Sequence, replayed.Sequence)

	var attempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, string(entity.RunAttemptStatusCancelled), attempt.Status)
	require.Nil(t, attempt.ActiveSlot)
	require.Zero(t, attempt.LastCommittedSequence)
}

func TestJournalAttemptStateMachinePersistsEveryFrozenTerminalStatus(t *testing.T) {
	statuses := []entity.RunAttemptStatus{
		entity.RunAttemptStatusCompleted,
		entity.RunAttemptStatusFailed,
		entity.RunAttemptStatusCancelled,
		entity.RunAttemptStatusTimedOut,
	}
	for i, status := range statuses {
		status := status
		t.Run(string(status), func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			repo := NewThreadRepository(db)
			seedJournalRun(t, db, 10, 1)
			seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusPending, 1)

			active, err := repo.GetActiveJournalAttempt(context.Background(), 10)
			require.NoError(t, err)
			require.Equal(t, entity.RunAttemptStatusPending, active.Status)
			require.Nil(t, active.StartedAt)

			event, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
				RunID: 10, Status: status, EndedAt: int64(200 + i),
				Event: &entity.JournalEvent{
					ID: int64(300 + i), ThreadID: 1, RunID: 10,
					IdempotencyKey: "terminal-" + string(status), EventType: "run.lifecycle",
					Status: string(status), Payload: journalTerminalTestPayload,
				},
			})
			require.NoError(t, err)
			require.True(t, won)
			require.Equal(t, uint64(1), event.Sequence)

			var stored runAttemptPO
			require.NoError(t, db.Where("id = ?", 100).First(&stored).Error)
			require.Equal(t, string(status), stored.Status)
			require.Nil(t, stored.ActiveSlot)
			require.NotNil(t, stored.StartedAt)
			require.Equal(t, int64(200+i), *stored.EndedAt)
			require.Equal(t, event.ID, *stored.TerminalEventID)
			require.Equal(t, uint64(2), stored.NextSequence)
			require.Zero(t, stored.LastCommittedSequence)
			_, err = repo.GetActiveJournalAttempt(context.Background(), 10)
			require.ErrorIs(t, err, ErrJournalAttemptTerminal)
		})
	}
}

func TestJournalLegacyProjectionAllowsOnlySafeConvertedEvents(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRunWithStatus(t, db, 10, 1, entity.RunStatusSucceeded)
	require.NoError(t, db.Create(&runEventPO{
		ID: 500, ThreadID: 1, RunID: 10, EventType: "run.completed",
		Payload: []byte(`{"raw_secret":"must-not-leak"}`), CreatedAt: 2,
	}).Error)
	require.NoError(t, db.Create(&runEventPO{
		ID: 501, ThreadID: 1, RunID: 10, EventType: "tool.result",
		Payload: []byte(`{"provider_raw":"must-not-leak"}`), CreatedAt: 3,
	}).Error)

	safe, err := repo.GetJournalEvent(context.Background(), 500)
	require.NoError(t, err)
	require.Equal(t, "legacy-10", safe.AttemptID)
	require.Equal(t, uint64(500), safe.Sequence)
	require.Equal(t, int64(10), safe.JournalRunID)
	require.Equal(t, entity.JournalVisibilityUser, safe.Visibility)
	require.NotContains(t, safe.Payload, "must-not-leak")
	require.True(t, strings.Contains(safe.Payload, `"type":"legacy_run_lifecycle"`))

	_, err = repo.GetJournalEvent(context.Background(), 501)
	require.ErrorIs(t, err, ErrJournalUnsafeLegacyEvent)
}

func TestJournalInterruptedLegacyRunIsNotProjectedAsTerminal(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRunWithStatus(t, db, 10, 1, entity.RunStatusInterrupted)
	require.NoError(t, db.Create(&runEventPO{
		ID: 500, ThreadID: 1, RunID: 10, EventType: "run.interrupted",
		Payload: []byte(`{"raw":"must-not-project"}`), CreatedAt: 2,
	}).Error)

	_, err := repo.GetJournalEvent(context.Background(), 500)
	require.ErrorIs(t, err, ErrJournalNotEnrolled)
}

func TestJournalAppendAssignsGaplessSequenceConcurrently(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

	const eventCount = 50
	sequences := make([]uint64, eventCount)
	errCh := make(chan error, eventCount)
	var wg sync.WaitGroup
	var ready sync.WaitGroup
	ready.Add(eventCount)
	start := make(chan struct{})
	for i := 0; i < eventCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready.Done()
			<-start
			event, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
				ID:             int64(1000 + i),
				ThreadID:       1,
				RunID:          10,
				Visibility:     entity.JournalVisibilityPublic,
				IdempotencyKey: fmt.Sprintf("event-%d", i),
				EventType:      "message.delta",
				Payload:        journalTestPayload,
			})
			if err != nil {
				errCh <- err
				return
			}
			sequences[i] = event.Sequence
		}()
	}
	ready.Wait()
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	for i := range sequences {
		require.Equal(t, uint64(i+1), sequences[i])
	}
}

func TestJournalTestDBUsesMultipleConcurrentConnections(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.Greater(t, sqlDB.Stats().MaxOpenConnections, 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first, err := sqlDB.Conn(ctx)
	require.NoError(t, err)
	defer first.Close()
	acquired := make(chan error, 1)
	release := make(chan struct{})
	go func() {
		second, err := sqlDB.Conn(ctx)
		acquired <- err
		if err != nil {
			return
		}
		defer second.Close()
		<-release
	}()
	require.NoError(t, <-acquired)
	require.GreaterOrEqual(t, sqlDB.Stats().InUse, 2)
	close(release)
}

func TestJournalAppendReplaysIdempotencyKeyWithoutConsumingSequence(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

	first, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10,
		Visibility: entity.JournalVisibilityPublic, IdempotencyKey: "same", EventType: "message.delta", Payload: journalTestPayload,
	})
	require.NoError(t, err)
	replayed, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1001, ThreadID: 1, RunID: 10,
		Visibility: entity.JournalVisibilityPublic, IdempotencyKey: "same", EventType: "message.delta",
		Payload: `{"type":"ignored","data":{"ignored":true}}`,
	})
	require.NoError(t, err)
	next, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1002, ThreadID: 1, RunID: 10,
		Visibility: entity.JournalVisibilityPublic, IdempotencyKey: "next", EventType: "message.delta", Payload: journalTestPayload,
	})
	require.NoError(t, err)

	require.Equal(t, first.ID, replayed.ID)
	require.Equal(t, uint64(1), replayed.Sequence)
	require.Equal(t, uint64(2), next.Sequence)
}

func TestJournalInternalEventsDoNotConsumeSequenceAndParentsResolvePublicAncestor(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

	publicParent := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "public-parent", EventType: "message.started", Payload: journalTestPayload,
	})
	internal := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1001, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityInternal,
		IdempotencyKey: "internal", ParentEventID: publicParent.ID, EventType: "model.raw", Payload: journalTestPayload,
	})
	internalChild := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1002, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityInternal,
		IdempotencyKey: "internal-child", ParentEventID: internal.ID, EventType: "tool.raw", Payload: journalTestPayload,
	})
	publicChild := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1003, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "public-child", ParentEventID: internalChild.ID, EventType: "message.completed", Payload: journalTestPayload,
	})
	filteredParent := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1004, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "filtered-parent", ParentEventID: 999999, EventType: "message.completed", Payload: journalTestPayload,
	})

	require.Equal(t, uint64(1), publicParent.Sequence)
	require.Zero(t, internal.Sequence)
	require.Zero(t, internalChild.Sequence)
	require.Equal(t, publicParent.ID, internal.ParentEventID)
	require.Equal(t, publicParent.ID, internalChild.ParentEventID)
	require.Equal(t, uint64(2), publicChild.Sequence)
	require.Equal(t, publicParent.ID, publicChild.ParentEventID)
	require.Equal(t, uint64(3), filteredParent.Sequence)
	require.Zero(t, filteredParent.ParentEventID)
}

func TestJournalParentFromAnotherAttemptIsRejectedWithoutSequenceGap(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
	firstAttemptEvent := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "first", EventType: "message.completed", Payload: journalTestPayload,
	})
	_, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
		RunID: 10, Status: entity.RunAttemptStatusCompleted,
		Event: &entity.JournalEvent{
			ID: 1001, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
			IdempotencyKey: "terminal", EventType: "run.lifecycle",
			Status: string(entity.RunAttemptStatusCompleted), Payload: journalTerminalTestPayload,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusRunning)
	recoveryKey := "recover-parent-test"
	sourceAttemptID := "att_100"
	secondAttempt, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_101", Status: entity.RunAttemptStatusRunning,
		RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
	})
	require.NoError(t, err)

	_, err = repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 2000, ThreadID: 1, RunID: 11, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "bad-parent", ParentEventID: firstAttemptEvent.ID,
		EventType: "message.started", Payload: journalTestPayload,
	})
	require.ErrorIs(t, err, ErrJournalParentMismatch)
	next := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 2001, ThreadID: 1, RunID: 11, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "valid-parent", EventType: "message.started", Payload: journalTestPayload,
	})
	require.Equal(t, secondAttempt.AttemptID, next.AttemptID)
	require.Equal(t, uint64(1), next.Sequence)
}

func TestJournalParentFromAnotherLogicalRunIsRejected(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
	otherRun := newRepositoryTestRun(20, 1, entity.RunStatusRunning, 2)
	otherRun.SpaceID = 10
	otherRun.CreatorID = 20
	otherPO, err := runToPO(otherRun)
	require.NoError(t, err)
	require.NoError(t, db.Create(otherPO).Error)
	journalRunID, attemptID := int64(20), "att_other"
	sequence, visibility := uint64(99), string(entity.JournalVisibilityPublic)
	require.NoError(t, db.Create(&runEventPO{
		ID: 999, ThreadID: 1, RunID: 20, JournalRunID: &journalRunID,
		AttemptID: &attemptID, Sequence: &sequence, Visibility: &visibility,
		EventType: "corrupt.parent", Payload: []byte(journalTestPayload),
	}).Error)

	_, err = repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "cross-run-parent", ParentEventID: 999,
		EventType: "message.started", Payload: journalTestPayload,
	})
	require.ErrorIs(t, err, ErrJournalParentMismatch)
}

func TestJournalCreateAttemptConcurrentCallsLeaveOneActive(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusCompleted, 1)
	seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusRunning)
	seedJournalRecoveryRun(t, db, 12, 1, entity.RunStatusRunning)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			recoveryKey := fmt.Sprintf("recovery-%d", i)
			sourceAttemptID := "att_100"
			_, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
				ID: int64(101 + i), ThreadID: 1, JournalRunID: 10, ExecutionRunID: int64(11 + i),
				AttemptID: fmt.Sprintf("att_%d", 101+i), Status: entity.RunAttemptStatusRunning,
				RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
			})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	created := 0
	for err := range results {
		if err == nil {
			created++
			continue
		}
		require.ErrorIs(t, err, ErrActiveJournalAttemptExists)
	}
	require.Equal(t, 1, created)
	var count int64
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("journal_run_id = ? AND active_slot = ?", 10, 1).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestJournalCreateAttemptPreservesValidationErrorsWithExistingActiveAttempt(t *testing.T) {
	t.Run("thread mismatch", func(t *testing.T) {
		db := newJournalRepositoryTestDB(t)
		repo := NewThreadRepository(db)
		seedJournalRun(t, db, 10, 1)
		seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
		seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusRunning)
		recoveryKey := "thread-mismatch"

		_, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
			ID: 101, ThreadID: 2, JournalRunID: 10, ExecutionRunID: 11,
			AttemptID: "att_101", Status: entity.RunAttemptStatusRunning,
			RecoveryIdempotencyKey: &recoveryKey,
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrActiveJournalAttemptExists)
		require.ErrorContains(t, err, "logical top-level task")
	})

	t.Run("terminal execution", func(t *testing.T) {
		db := newJournalRepositoryTestDB(t)
		repo := NewThreadRepository(db)
		seedJournalRun(t, db, 10, 1)
		seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
		seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusSucceeded)
		recoveryKey := "terminal-new-key"

		_, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
			ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
			AttemptID: "att_101", Status: entity.RunAttemptStatusCompleted,
			RecoveryIdempotencyKey: &recoveryKey,
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrActiveJournalAttemptExists)
		require.ErrorContains(t, err, "pending or running")
	})
}

func TestJournalCreateRunBundleRollsBackRunAttemptAndSequence(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalThread(t, db, 1)
	require.NoError(t, db.Create(&runEventPO{ID: 900, ThreadID: 999, RunID: 999, Payload: []byte(`{}`)}).Error)

	run := newRepositoryTestRun(10, 1, entity.RunStatusQueued, 1)
	run.SpaceID = 10
	run.CreatorID = 20
	result, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run:   run,
		Event: &entity.RunEvent{ID: 900, ThreadID: 1, RunID: 10, EventType: "run.created", Payload: `{}`},
		Attempt: &entity.RunAttempt{
			ID: 100, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
			AttemptID: "att_100", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: "1.1",
			ProjectionState: entity.JournalProjectionStateHealthy,
		},
	})
	require.Error(t, err)
	require.Nil(t, result)

	var runCount, attemptCount int64
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 10).Count(&runCount).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("journal_run_id = ?", 10).Count(&attemptCount).Error)
	require.Zero(t, runCount)
	require.Zero(t, attemptCount)

	validRun := newRepositoryTestRun(10, 1, entity.RunStatusQueued, 2)
	validRun.SpaceID = 10
	validRun.CreatorID = 20
	created, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: validRun,
		Attempt: &entity.RunAttempt{
			ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
			AttemptID: "att_101", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: "1.1",
			ProjectionState: entity.JournalProjectionStateHealthy,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, created.Attempt)
	first := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 901, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "after-rollback", EventType: "run.started", Payload: journalTestPayload,
	})
	require.Equal(t, uint64(1), first.Sequence)
}

func TestJournalCreateRunBundleRejectsEnrollmentWithoutAttemptID(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalThread(t, db, 1)
	run := newRepositoryTestRun(10, 1, entity.RunStatusQueued, 1)
	run.SpaceID = 10
	run.CreatorID = 20

	_, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: run,
		Attempt: &entity.RunAttempt{
			ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
			AttemptID: "att_missing", Ordinal: 1, Status: entity.RunAttemptStatusPending,
			NextSequence: 1, EnrollmentVersion: "1.1",
			ProjectionState: entity.JournalProjectionStateHealthy,
		},
	})
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 10).Count(&count).Error)
	require.Zero(t, count)
}

func TestJournalFinalizeRaceHasOneWinnerAndRejectsLateEvents(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

	statuses := []entity.RunAttemptStatus{
		entity.RunAttemptStatusCompleted,
		entity.RunAttemptStatusCancelled,
		entity.RunAttemptStatusTimedOut,
	}
	start := make(chan struct{})
	type finalizeResult struct {
		won bool
		err error
	}
	results := make(chan finalizeResult, len(statuses))
	var wg sync.WaitGroup
	for i, status := range statuses {
		i, status := i, status
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
				RunID:  10,
				Status: status,
				Event: &entity.JournalEvent{
					ID: int64(2000 + i), ThreadID: 1, RunID: 10,
					Visibility:     entity.JournalVisibilityPublic,
					IdempotencyKey: fmt.Sprintf("terminal-%d", i),
					EventType:      "run.lifecycle", Status: string(status),
					Payload: journalTerminalTestPayload,
				},
			})
			results <- finalizeResult{won: won, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for result := range results {
		require.NoError(t, result.err)
		if result.won {
			winners++
		}
	}
	require.Equal(t, 1, winners)

	_, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 3000, ThreadID: 1, RunID: 10,
		Visibility: entity.JournalVisibilityPublic, IdempotencyKey: "late",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	require.ErrorIs(t, err, ErrJournalAttemptTerminal)
	var count int64
	require.NoError(t, db.Model(&runEventPO{}).
		Where("attempt_id = ? AND sequence IS NOT NULL", "att_100").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestJournalTerminalAttemptReplaysIdempotencyKeyWithoutSequence(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
	original := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "replay-after-terminal", EventType: "message.delta", Payload: journalTestPayload,
	})
	finalizeJournalAttemptForTest(t, repo, 2000)

	replayed, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
		ID: 3000, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "replay-after-terminal", EventType: "different.type",
		Payload: `{"type":"ignored","data":{"ignored":true}}`,
	})
	require.NoError(t, err)
	require.Equal(t, original.ID, replayed.ID)
	require.Equal(t, original.Sequence, replayed.Sequence)
	requireJournalPublicSequenceCount(t, db, 100, 2)
}

func TestJournalTerminalAttemptReplaysActionPhaseWithoutSequence(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
	original := appendJournalEventForTest(t, repo, journalActionEvent(1000, "progress", "progress-original"))
	finalizeJournalAttemptForTest(t, repo, 2000)

	duplicate := journalActionEvent(3000, "progress", "progress-retry")
	replayed, err := repo.AppendJournalEvent(context.Background(), duplicate)
	require.NoError(t, err)
	require.Equal(t, original.ID, replayed.ID)
	require.Equal(t, original.Sequence, replayed.Sequence)
	requireJournalPublicSequenceCount(t, db, 100, 2)
}

func TestJournalTerminalAttemptStillRejectsActionDrift(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
	appendJournalEventForTest(t, repo, journalActionEvent(1000, "started", "started-original"))
	finalizeJournalAttemptForTest(t, repo, 2000)

	drift := journalActionEvent(3000, "progress", "progress-drift")
	drift.Target = "other.txt"
	_, err := repo.AppendJournalEvent(context.Background(), drift)
	require.ErrorIs(t, err, ErrJournalActionDrift)
	requireJournalPublicSequenceCount(t, db, 100, 2)
}

func TestJournalFinalizeRequiresPublicRootEvent(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

	_, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
		RunID: 10, Status: entity.RunAttemptStatusCompleted,
		Event: &entity.JournalEvent{
			ID: 2000, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityInternal,
			IdempotencyKey: "internal-terminal", EventType: "run.lifecycle",
			Status: string(entity.RunAttemptStatusCompleted), Payload: journalTerminalTestPayload,
		},
	})
	require.Error(t, err)
	require.False(t, won)
	_, err = repo.GetActiveJournalAttempt(context.Background(), 10)
	require.NoError(t, err)
}

func TestJournalUnenrolledRunsCannotLazilyCreateAttempts(t *testing.T) {
	statuses := []entity.RunStatus{entity.RunStatusRunning, entity.RunStatusSucceeded}
	for _, status := range statuses {
		status := status
		t.Run(string(status), func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			repo := NewThreadRepository(db)
			seedJournalRunWithStatus(t, db, 10, 1, status)

			recoveryKey := "must-not-enroll"
			_, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
				ID: 100, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
				AttemptID: "att_100", Status: entity.RunAttemptStatusRunning,
				RecoveryIdempotencyKey: &recoveryKey,
			})
			require.ErrorIs(t, err, ErrJournalNotEnrolled)
			_, err = repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
				ID: 1000, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
				IdempotencyKey: "not-enrolled", EventType: "message.delta", Payload: journalTestPayload,
			})
			require.ErrorIs(t, err, ErrJournalNotEnrolled)
			var count int64
			require.NoError(t, db.Model(&runAttemptPO{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestJournalSubagentUsesRootActiveAttemptAndCannotCreateOwnAttempt(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
	child := newRepositoryTestRun(11, 1, entity.RunStatusRunning, 2)
	child.ParentRunID = 10
	child.RunKind = entity.RunKindSubagent
	child.SpaceID = 10
	child.CreatorID = 20
	childPO, err := runToPO(child)
	require.NoError(t, err)
	require.NoError(t, db.Create(childPO).Error)

	event := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 11, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "child-event", EventType: "subagent.progress", Payload: journalTestPayload,
	})
	require.Equal(t, int64(11), event.RunID)
	require.Equal(t, int64(10), event.JournalRunID)
	require.Equal(t, "att_100", event.AttemptID)
	require.Equal(t, uint64(1), event.Sequence)
	var stored runEventPO
	require.NoError(t, db.Where("id = ?", event.ID).First(&stored).Error)
	require.Equal(t, int64(11), stored.RunID)
	require.Equal(t, int64(10), *stored.JournalRunID)
	var storedAttempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&storedAttempt).Error)
	require.Equal(t, int64(10), storedAttempt.ExecutionRunID)
	_, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
		RunID: 11, Status: entity.RunAttemptStatusCompleted,
		Event: &entity.JournalEvent{
			ID: 1001, ThreadID: 1, RunID: 11, Visibility: entity.JournalVisibilityPublic,
			IdempotencyKey: "child-terminal", EventType: "run.lifecycle",
			Status: string(entity.RunAttemptStatusCompleted), Payload: journalTerminalTestPayload,
		},
	})
	require.Error(t, err)
	require.False(t, won)
	_, err = repo.GetActiveJournalAttempt(context.Background(), 10)
	require.NoError(t, err)
	_, won, err = repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
		RunID: 10, Status: entity.RunAttemptStatusCompleted,
		Event: &entity.JournalEvent{
			ID: 1002, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
			IdempotencyKey: "root-terminal", EventType: "run.lifecycle",
			Status: string(entity.RunAttemptStatusCompleted), Payload: journalTerminalTestPayload,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	recoveryKey := "subagent-cannot-own"
	_, err = repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_101", Status: entity.RunAttemptStatusRunning,
		RecoveryIdempotencyKey: &recoveryKey,
	})
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&runAttemptPO{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestJournalActionFieldsStayStableAndDuplicatePhaseReplays(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

	started := appendJournalEventForTest(t, repo, journalActionEvent(1000, "started", "started-key"))
	progress := appendJournalEventForTest(t, repo, journalActionEvent(1001, "progress", "progress-key"))
	terminal := appendJournalEventForTest(t, repo, journalActionEvent(1002, "terminal", "terminal-key"))
	require.Equal(t, []uint64{1, 2, 3}, []uint64{started.Sequence, progress.Sequence, terminal.Sequence})

	duplicate := journalActionEvent(1003, "progress", "duplicate-progress-key")
	duplicate.Payload = `{"type":"ignored","data":{"ignored":true}}`
	replayed := appendJournalEventForTest(t, repo, duplicate)
	require.Equal(t, progress.ID, replayed.ID)
	require.Equal(t, progress.Sequence, replayed.Sequence)

	for field, mutate := range map[string]func(*entity.JournalEvent){
		"operation": func(event *entity.JournalEvent) { event.Operation = "write" },
		"target":    func(event *entity.JournalEvent) { event.Target = "other.txt" },
		"milestone": func(event *entity.JournalEvent) { event.Milestone = "publish" },
	} {
		field, mutate := field, mutate
		t.Run(field, func(t *testing.T) {
			drift := journalActionEvent(2000+int64(len(field)), "started", "drift-"+field)
			mutate(drift)
			_, err := repo.AppendJournalEvent(context.Background(), drift)
			require.ErrorIs(t, err, ErrJournalActionDrift)
		})
	}
	next := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 3000, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "after-drift", EventType: "message.delta", Payload: journalTestPayload,
	})
	require.Equal(t, uint64(4), next.Sequence)
}

func TestJournalTerminalInheritsStartedMilestoneAfterProjectionRecovery(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

	started := appendJournalEventForTest(
		t,
		repo,
		journalActionEvent(1000, "started", "recovery-started"),
	)
	terminal := journalActionEvent(1001, "terminal", "recovery-terminal")
	terminal.Milestone = ""

	stored, err := repo.AppendJournalEvent(context.Background(), terminal)

	require.NoError(t, err)
	require.Equal(t, started.Milestone, stored.Milestone)
}

func TestJournalActionEventRequiresStableIdentity(t *testing.T) {
	tests := map[string]func(*entity.JournalEvent){
		"action_id": func(event *entity.JournalEvent) { event.ActionID = "" },
		"phase":     func(event *entity.JournalEvent) { event.Phase = "" },
		"operation": func(event *entity.JournalEvent) { event.Operation = "" },
		"target":    func(event *entity.JournalEvent) { event.Target = "" },
	}
	for name, remove := range tests {
		name, remove := name, remove
		t.Run(name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			repo := NewThreadRepository(db)
			seedJournalRun(t, db, 10, 1)
			seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
			event := journalActionEvent(2000+int64(len(name)), "started", "missing-"+name)
			remove(event)
			_, err := repo.AppendJournalEvent(context.Background(), event)
			require.Error(t, err)
		})
	}

	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
	valid := journalActionEvent(3000, "started", "valid-action")
	valid.Milestone = ""
	appended := appendJournalEventForTest(t, repo, valid)
	require.Equal(t, uint64(1), appended.Sequence)
}

func TestJournalActionMetadataCannotPoisonAnotherEventFamilyOrPhase(t *testing.T) {
	t.Run("non action event", func(t *testing.T) {
		db := newJournalRepositoryTestDB(t)
		repo := NewThreadRepository(db)
		seedJournalRun(t, db, 10, 1)
		seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

		_, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
			ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "poison-non-action",
			ActionID: "action-1", Phase: "started", Operation: "read", Target: "report.txt",
			Milestone: "draft", EventType: "message.delta", Payload: journalTestPayload,
		})
		require.Error(t, err)
		var count int64
		require.NoError(t, db.Model(&runEventPO{}).Count(&count).Error)
		require.Zero(t, count)
	})

	t.Run("phase kind mismatch", func(t *testing.T) {
		db := newJournalRepositoryTestDB(t)
		repo := NewThreadRepository(db)
		seedJournalRun(t, db, 10, 1)
		seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

		_, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
			ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "poison-phase",
			ActionID: "action-1", Phase: "terminal", Operation: "read", Target: "report.txt",
			EventType: "action.started", Payload: journalTestPayload,
		})
		require.Error(t, err)
	})

	t.Run("progress revision", func(t *testing.T) {
		db := newJournalRepositoryTestDB(t)
		repo := NewThreadRepository(db)
		seedJournalRun(t, db, 10, 1)
		seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

		event := appendJournalEventForTest(t, repo, &entity.JournalEvent{
			ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "progress-revision-1",
			ActionID: "action-1", Phase: "progress:1", Operation: "read", Target: "report.txt",
			EventType: "action.progress", Payload: journalTestPayload,
		})
		require.Equal(t, uint64(1), event.Sequence)
	})
}

func TestJournalRecoveryReplayIgnoresTraceID(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusCompleted, 1)
	seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusRunning)

	recoveryKey := "recover-trace-replay"
	sourceAttemptID := "att_100"
	firstTrace := "trace-first"
	first, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_101", Status: entity.RunAttemptStatusRunning,
		RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
		TraceID: &firstTrace,
	})
	require.NoError(t, err)
	recoveryEvent := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 11, IdempotencyKey: "recovery-event",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	require.Equal(t, int64(11), recoveryEvent.RunID)
	require.Equal(t, int64(10), recoveryEvent.JournalRunID)
	require.Equal(t, first.AttemptID, recoveryEvent.AttemptID)
	require.Equal(t, uint64(1), recoveryEvent.Sequence)

	retryTrace := "trace-retry"
	replayed, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 102, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_102", Status: entity.RunAttemptStatusRunning,
		RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
		TraceID: &retryTrace,
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, replayed.ID)
	require.Equal(t, first.AttemptID, replayed.AttemptID)
	require.Equal(t, firstTrace, *replayed.TraceID)
}

func TestJournalRecoveryReplaySurvivesTerminalExecutionRun(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusCompleted, 1)
	seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusRunning)

	recoveryKey := "recover-after-terminal"
	sourceAttemptID := "att_100"
	first, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_101", Status: entity.RunAttemptStatusRunning,
		RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 11).
		Update("status", entity.RunStatusSucceeded).Error)

	replayed, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 102, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_102", Status: entity.RunAttemptStatusCompleted,
		RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, replayed.ID)
	require.Equal(t, first.AttemptID, replayed.AttemptID)
}

func TestJournalRecoveryReplayRejectsExecutionAndSourceDrift(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusCompleted, 1)
	seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusRunning)
	seedJournalRecoveryRun(t, db, 12, 1, entity.RunStatusRunning)

	recoveryKey := "recover-semantic-drift"
	sourceAttemptID := "att_100"
	sourceCheckpointID := int64(500)
	_, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_101", Status: entity.RunAttemptStatusRunning,
		RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
		SourceCheckpointID: &sourceCheckpointID,
	})
	require.NoError(t, err)

	otherSourceAttemptID := "att_other"
	otherCheckpointID := int64(501)
	tests := map[string]*entity.RunAttempt{
		"execution": {
			ID: 102, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 12,
			AttemptID: "att_102", Status: entity.RunAttemptStatusRunning,
			RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
			SourceCheckpointID: &sourceCheckpointID,
		},
		"source_attempt": {
			ID: 103, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
			AttemptID: "att_103", Status: entity.RunAttemptStatusRunning,
			RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &otherSourceAttemptID,
			SourceCheckpointID: &sourceCheckpointID,
		},
		"source_checkpoint": {
			ID: 104, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
			AttemptID: "att_104", Status: entity.RunAttemptStatusRunning,
			RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
			SourceCheckpointID: &otherCheckpointID,
		},
	}
	for name, attempt := range tests {
		name, attempt := name, attempt
		t.Run(name, func(t *testing.T) {
			_, err := repo.CreateJournalAttempt(context.Background(), attempt)
			require.ErrorIs(t, err, ErrRunIdempotencyConflict)
		})
	}

	var count int64
	require.NoError(t, db.Model(&runAttemptPO{}).Where("journal_run_id = ?", 10).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestJournalLegacyTerminalEventsProjectReadOnlyByEventID(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRunWithStatus(t, db, 10, 1, entity.RunStatusSucceeded)
	require.NoError(t, db.Create(&runEventPO{
		ID: 500, ThreadID: 1, RunID: 10, EventType: "run.completed", Payload: []byte(`{}`), CreatedAt: 2,
	}).Error)

	event, err := repo.GetJournalEvent(context.Background(), 500)
	require.NoError(t, err)
	require.Equal(t, "legacy-10", event.AttemptID)
	require.Equal(t, uint64(500), event.Sequence)
	var count int64
	require.NoError(t, db.Model(&runAttemptPO{}).Count(&count).Error)
	require.Zero(t, count)
	visibility := string(entity.JournalVisibilityPublic)
	require.NoError(t, db.Create(&runEventPO{
		ID: 501, ThreadID: 1, RunID: 10, Visibility: &visibility,
		EventType: "partial.journal", Payload: []byte(`{}`), CreatedAt: 3,
	}).Error)
	_, err = repo.GetJournalEvent(context.Background(), 501)
	require.ErrorIs(t, err, ErrJournalNotEnrolled)

	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 10).Update("status", entity.RunStatusRunning).Error)
	_, err = repo.GetJournalEvent(context.Background(), 500)
	require.ErrorIs(t, err, ErrJournalNotEnrolled)
}

func TestJournalListAttemptsAndPublicEventsUseFrozenOrdering(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusCompleted, 1)
	seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusRunning)
	recoveryKey := "list-recovery"
	second, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_101", Status: entity.RunAttemptStatusRunning,
		RecoveryIdempotencyKey: &recoveryKey,
	})
	require.NoError(t, err)

	first := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 11, IdempotencyKey: "list-first",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1001, ThreadID: 1, RunID: 11, Visibility: entity.JournalVisibilityInternal,
		IdempotencyKey: "list-internal", EventType: "model.raw", Payload: journalTestPayload,
	})
	secondEvent := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1002, ThreadID: 1, RunID: 11, IdempotencyKey: "list-second",
		EventType: "message.delta", Payload: journalTestPayload,
	})

	attempts, err := repo.ListJournalAttempts(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, []string{"att_100", second.AttemptID}, []string{
		attempts[0].AttemptID,
		attempts[1].AttemptID,
	})

	page, err := repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, AttemptID: second.AttemptID, Limit: 1,
	})
	require.NoError(t, err)
	require.False(t, page.Legacy)
	require.True(t, page.HasMore)
	require.Len(t, page.Events, 1)
	require.Equal(t, first.ID, page.Events[0].ID)
	require.Equal(t, uint64(1), page.Events[0].Sequence)

	page, err = repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, AttemptID: second.AttemptID, AfterSequence: 1, Limit: 10,
	})
	require.NoError(t, err)
	require.False(t, page.HasMore)
	require.Len(t, page.Events, 1)
	require.Equal(t, secondEvent.ID, page.Events[0].ID)
	require.Equal(t, uint64(2), page.Events[0].Sequence)
}

func TestJournalBootstrapFreezesLatestSequenceAndValidatesDualCursor(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)

	first := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "bootstrap-first",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	second := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1001, ThreadID: 1, RunID: 10, IdempotencyKey: "bootstrap-second",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1002, ThreadID: 1, RunID: 10, IdempotencyKey: "bootstrap-third",
		EventType: "message.delta", Payload: journalTestPayload,
	})

	bootstrap, err := repo.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "att_100", Limit: 2,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), bootstrap.LatestSequence)
	require.Zero(t, bootstrap.ResolvedAfterSequence)
	require.True(t, bootstrap.HasMore)
	require.Equal(t, []uint64{1, 2}, []uint64{
		bootstrap.Events[0].Sequence, bootstrap.Events[1].Sequence,
	})

	resumed, err := repo.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "att_100", AfterEventID: second.ID,
		AfterSequence: second.Sequence, Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), resumed.LatestSequence)
	require.Equal(t, second.Sequence, resumed.ResolvedAfterSequence)
	require.Len(t, resumed.Events, 1)
	require.Equal(t, uint64(3), resumed.Events[0].Sequence)

	_, err = repo.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "att_100", AfterEventID: first.ID,
		AfterSequence: second.Sequence, Limit: 10,
	})
	require.ErrorIs(t, err, ErrJournalEventGap)

	_, err = repo.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "att_100", AfterEventID: first.ID,
		AfterSequence: 0, AfterSequenceSet: true, Limit: 10,
	})
	require.ErrorIs(t, err, ErrJournalEventGap)

	_, err = repo.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "att_100", AfterEventID: 999999, Limit: 10,
	})
	require.ErrorIs(t, err, ErrJournalCursorExpired)

	_, err = repo.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "missing-attempt", AfterSequence: 1, Limit: 10,
	})
	require.ErrorIs(t, err, ErrJournalCursorExpired)

	seedJournalRecoveryRun(t, db, 20, 1, entity.RunStatusRunning)
	seedJournalAttempt(t, db, 200, 20, entity.RunAttemptStatusActive, 1)
	foreign := appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 2000, ThreadID: 1, RunID: 20, IdempotencyKey: "bootstrap-foreign",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	_, err = repo.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "att_100", AfterEventID: foreign.ID, Limit: 10,
	})
	require.ErrorIs(t, err, ErrJournalCursorExpired)
}

func TestJournalBootstrapRejectsPersistedPublicSequenceGap(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusActive, 1)
	appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1000, ThreadID: 1, RunID: 10, IdempotencyKey: "gap-first",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1001, ThreadID: 1, RunID: 10, IdempotencyKey: "gap-second",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	appendJournalEventForTest(t, repo, &entity.JournalEvent{
		ID: 1002, ThreadID: 1, RunID: 10, IdempotencyKey: "gap-third",
		EventType: "message.delta", Payload: journalTestPayload,
	})
	require.NoError(t, db.Delete(&runEventPO{}, 1001).Error)

	_, err := repo.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "att_100", Limit: 10,
	})
	require.ErrorIs(t, err, ErrJournalEventGap)
}

func TestJournalLegacyListUsesEventIDAndSkipsUnsafeEvents(t *testing.T) {
	db := newJournalRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRunWithStatus(t, db, 10, 1, entity.RunStatusSucceeded)
	for _, event := range []runEventPO{
		{ID: 500, ThreadID: 1, RunID: 10, EventType: "run.started", Payload: []byte(`{}`), CreatedAt: 2},
		{ID: 501, ThreadID: 1, RunID: 10, EventType: "tool.result", Payload: []byte(`{"secret":true}`), CreatedAt: 3},
		{ID: 502, ThreadID: 1, RunID: 10, EventType: "run.completed", Payload: []byte(`{}`), CreatedAt: 4},
	} {
		event := event
		require.NoError(t, db.Create(&event).Error)
	}

	page, err := repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, AttemptID: "legacy-10", Limit: 1,
	})
	require.NoError(t, err)
	require.True(t, page.Legacy)
	require.True(t, page.HasMore)
	require.Len(t, page.Events, 1)
	require.Equal(t, int64(500), page.Events[0].ID)
	require.Equal(t, uint64(500), page.Events[0].Sequence)
	require.NotContains(t, page.Events[0].Payload, "secret")

	page, err = repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, AttemptID: "legacy-10", AfterEventID: 500, Limit: 10,
	})
	require.NoError(t, err)
	require.True(t, page.Legacy)
	require.False(t, page.HasMore)
	require.Len(t, page.Events, 1)
	require.Equal(t, int64(502), page.Events[0].ID)

	_, err = repo.ListJournalEvents(context.Background(), ListJournalEventsRequest{
		RunID: 10, AttemptID: "legacy-10", AfterSequence: 500, Limit: 10,
	})
	require.Error(t, err)
	var attemptCount int64
	require.NoError(t, db.Model(&runAttemptPO{}).Count(&attemptCount).Error)
	require.Zero(t, attemptCount)
}

func newJournalRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf(
		"file:%s?_busy_timeout=10000&_journal_mode=WAL&_synchronous=NORMAL&_txlock=immediate",
		filepath.Join(t.TempDir(), "journal.db"),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(16)
	sqlDB.SetMaxIdleConns(16)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&threadPO{}, &runPO{}, &runAttemptPO{}, &runEventPO{}))
	return db
}

func seedJournalThread(t *testing.T, db *gorm.DB, threadID int64) {
	t.Helper()
	require.NoError(t, db.Create(&threadPO{
		ID: threadID, SpaceID: 10, CreatorID: 20, Title: "journal",
		Status: string(entity.ThreadStatusIdle), Source: string(entity.ThreadSourceWeb),
	}).Error)
}

func seedJournalRun(t *testing.T, db *gorm.DB, runID, threadID int64) {
	t.Helper()
	seedJournalRunWithStatus(t, db, runID, threadID, entity.RunStatusRunning)
}

func seedJournalRunWithStatus(
	t *testing.T,
	db *gorm.DB,
	runID, threadID int64,
	status entity.RunStatus,
) {
	t.Helper()
	seedJournalThread(t, db, threadID)
	run := newRepositoryTestRun(runID, threadID, status, 1)
	run.SpaceID = 10
	run.CreatorID = 20
	po, err := runToPO(run)
	require.NoError(t, err)
	require.NoError(t, db.Create(po).Error)
}

func seedJournalChildRun(
	t *testing.T,
	db *gorm.DB,
	runID, parentRunID, threadID int64,
	status entity.RunStatus,
) {
	t.Helper()
	run := newRepositoryTestRun(runID, threadID, status, runID)
	run.ParentRunID = parentRunID
	run.RunKind = entity.RunKindSubagent
	run.SpaceID = 10
	run.CreatorID = 20
	po, err := runToPO(run)
	require.NoError(t, err)
	require.NoError(t, db.Create(po).Error)
}

func seedJournalRecoveryRun(
	t *testing.T,
	db *gorm.DB,
	runID, threadID int64,
	status entity.RunStatus,
) {
	t.Helper()
	run := newRepositoryTestRun(runID, threadID, status, runID)
	run.RunKind = entity.RunKindTask
	run.SpaceID = 10
	run.CreatorID = 20
	po, err := runToPO(run)
	require.NoError(t, err)
	require.NoError(t, db.Create(po).Error)
}

func appendJournalEventForTest(
	t *testing.T,
	repo Repository,
	event *entity.JournalEvent,
) *entity.JournalEvent {
	t.Helper()
	created, err := repo.AppendJournalEvent(context.Background(), event)
	require.NoError(t, err)
	return created
}

func journalActionEvent(id int64, phase, key string) *entity.JournalEvent {
	return &entity.JournalEvent{
		ID: id, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: key, ActionID: "action-1", Phase: phase,
		Operation: "read", Target: "report.txt", Milestone: "draft",
		EventType: "action." + phase, Payload: journalTestPayload,
	}
}

func finalizeJournalAttemptForTest(t *testing.T, repo Repository, eventID int64) *entity.JournalEvent {
	t.Helper()
	event, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
		RunID: 10, Status: entity.RunAttemptStatusCompleted,
		Event: &entity.JournalEvent{
			ID: eventID, ThreadID: 1, RunID: 10, Visibility: entity.JournalVisibilityPublic,
			IdempotencyKey: fmt.Sprintf("terminal-%d", eventID), EventType: "run.lifecycle",
			Status: string(entity.RunAttemptStatusCompleted), Payload: journalTerminalTestPayload,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	return event
}

func requireJournalPublicSequenceCount(t *testing.T, db *gorm.DB, attemptID, expected int64) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&runEventPO{}).
		Where("attempt_id = ? AND sequence IS NOT NULL", fmt.Sprintf("att_%d", attemptID)).
		Count(&count).Error)
	require.Equal(t, expected, count)
}

func seedJournalAttempt(
	t *testing.T,
	db *gorm.DB,
	attemptID, runID int64,
	status entity.RunAttemptStatus,
	attemptNumber uint32,
) {
	t.Helper()
	activeSlot := uint8(1)
	attempt := &entity.RunAttempt{
		ID: attemptID, ThreadID: 1, JournalRunID: runID, ExecutionRunID: runID,
		AttemptID: fmt.Sprintf("att_%d", attemptID), Ordinal: attemptNumber,
		Status: status, NextSequence: 1, EnrollmentVersion: "1.1",
		ProjectionState: entity.JournalProjectionStateHealthy,
	}
	if status.IsActive() {
		attempt.ActiveSlot = &activeSlot
	}
	if status == entity.RunAttemptStatusRunning {
		startedAt := int64(1)
		attempt.StartedAt = &startedAt
	}
	if status.IsTerminal() {
		startedAt, endedAt, terminalID := int64(1), int64(2), attemptID+9000
		attempt.StartedAt = &startedAt
		attempt.EndedAt = &endedAt
		attempt.TerminalEventID = &terminalID
	}
	po := runAttemptToPO(attempt)
	require.NoError(t, db.Create(po).Error)
}
