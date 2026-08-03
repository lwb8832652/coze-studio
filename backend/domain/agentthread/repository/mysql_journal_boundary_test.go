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
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestJournalBoundaryMigrationAndModelAreFrozen(t *testing.T) {
	migration, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "docker", "atlas", "migrations",
		"20260730000400_agent_side_effect_ledger.sql",
	))
	require.NoError(t, err)
	sql := string(migration)
	for _, fragment := range []string{
		"`journal_run_id` bigint NOT NULL",
		"`attempt_id` varchar(64) NOT NULL",
		"`idempotency_key` varchar(191) NOT NULL",
		"`action_kind` varchar(128) NOT NULL",
		"`replay_policy` varchar(32) NOT NULL",
		"`request_hash` char(64) NOT NULL",
		"`external_reference_digest` char(64) DEFAULT NULL",
		"`result_snapshot_id` varchar(64) DEFAULT NULL",
		"`resolution_action` varchar(32) DEFAULT NULL",
		"`resolution_idempotency_key` varchar(191) DEFAULT NULL",
		"`resolved_at` bigint DEFAULT NULL",
		"`version` bigint unsigned NOT NULL DEFAULT 1",
		"uk_agent_side_effect_ledger_identity",
		"uk_agent_side_effect_ledger_resolution",
		"read_only", "idempotent_write", "non_replayable",
		"prepared", "executing", "succeeded", "failed", "unknown", "compensated",
		"mark_succeeded", "skip", "retry",
	} {
		require.Contains(t, sql, fragment)
	}

	db := newJournalBoundaryTestDB(t)
	for _, column := range []string{
		"journal_run_id", "attempt_id", "idempotency_key", "action_kind",
		"replay_policy", "request_hash", "request_summary", "external_reference_digest",
		"result_snapshot_id", "result_event_id", "checkpoint_id", "compensation_kind",
		"resolution_action", "resolution_idempotency_key", "resolved_at",
		"version", "prepared_at", "executing_at", "succeeded_at", "failed_at",
		"unknown_at", "compensated_at",
	} {
		require.Truef(t, db.Migrator().HasColumn(&sideEffectLedgerPO{}, column), "missing ledger column %s", column)
	}
	require.True(t, db.Migrator().HasIndex(
		&sideEffectLedgerPO{}, "uk_agent_side_effect_ledger_identity",
	))
}

func TestJournalBoundaryPreparedCommitsBeforeExecutionAndReplays(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalBoundaryAttempt(t, db)

	prepared, created, err := repo.PrepareSideEffect(context.Background(), PrepareSideEffectRequest{
		Ledger: boundaryLedger(entity.SideEffectLedgerStatusPrepared),
		AuditEvent: boundaryAuditEvent(
			1001, "side-effect:tool-1:prepared", "prepared",
		),
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, entity.SideEffectLedgerStatusPrepared, prepared.Status)
	require.Equal(t, uint64(1), prepared.Version)

	replayed, created, err := repo.PrepareSideEffect(context.Background(), PrepareSideEffectRequest{
		Ledger: boundaryLedger(entity.SideEffectLedgerStatusPrepared),
		AuditEvent: boundaryAuditEvent(
			1002, "side-effect:tool-1:prepared-replay", "prepared",
		),
	})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, prepared.ID, replayed.ID)

	var ledgers, audits int64
	require.NoError(t, db.Model(&sideEffectLedgerPO{}).Count(&ledgers).Error)
	require.NoError(t, db.Model(&runEventPO{}).Count(&audits).Error)
	require.Equal(t, int64(1), ledgers)
	require.Equal(t, int64(1), audits)
}

func TestJournalBoundaryCommitsTerminalEventCheckpointAndOffsetOnce(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalBoundaryAttempt(t, db)
	seedExecutingBoundaryLedger(t, repo)

	result, err := repo.CommitExecutionBoundary(
		context.Background(),
		CommitExecutionBoundaryRequest{
			JournalRunID:            10,
			AttemptID:               "att_10",
			LedgerID:                500,
			ExpectedVersion:         2,
			Status:                  entity.SideEffectLedgerStatusSucceeded,
			ExternalReferenceDigest: boundaryDigest("external-ref"),
			ResultSnapshotID:        "snapshot-1",
			ResultEvent:             boundaryResultEvent(1003, "succeeded"),
			AuditEvent: boundaryAuditEvent(
				1004, "side-effect:tool-1:succeeded", "succeeded",
			),
			CheckpointFactory: func(
				lastCommittedSequence uint64,
				ledgers []*entity.SideEffectLedger,
			) (*entity.Checkpoint, error) {
				require.Len(t, ledgers, 1)
				require.Equal(t, entity.SideEffectLedgerStatusSucceeded, ledgers[0].Status)
				require.Equal(t, uint64(3), ledgers[0].Version)
				return boundaryCheckpoint(lastCommittedSequence), nil
			},
		},
	)
	require.NoError(t, err)
	require.False(t, result.Replayed)
	require.Equal(t, uint64(1), result.LastCommittedSequence)
	require.Equal(t, entity.SideEffectLedgerStatusSucceeded, result.Ledger.Status)
	require.Equal(t, uint64(3), result.Ledger.Version)
	require.Equal(t, uint64(1), result.Event.Sequence)
	require.Equal(t, int64(900), result.Checkpoint.ID)

	var attempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, uint64(2), attempt.NextSequence)
	require.Equal(t, uint64(1), attempt.LastCommittedSequence)

	replayed, err := repo.CommitExecutionBoundary(
		context.Background(),
		CommitExecutionBoundaryRequest{
			JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
			ExpectedVersion: 2, Status: entity.SideEffectLedgerStatusSucceeded,
			ExternalReferenceDigest: boundaryDigest("external-ref"),
			ResultSnapshotID:        "snapshot-1",
			ResultEvent:             boundaryResultEvent(1003, "succeeded"),
			AuditEvent: boundaryAuditEvent(
				1004, "side-effect:tool-1:succeeded", "succeeded",
			),
			CheckpointFactory: func(
				lastCommittedSequence uint64,
				_ []*entity.SideEffectLedger,
			) (*entity.Checkpoint, error) {
				return boundaryCheckpoint(lastCommittedSequence), nil
			},
		},
	)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, uint64(1), replayed.LastCommittedSequence)

	var events, checkpoints int64
	require.NoError(t, db.Model(&runEventPO{}).Count(&events).Error)
	require.NoError(t, db.Model(&checkpointPO{}).Count(&checkpoints).Error)
	require.Equal(t, int64(4), events)
	require.Equal(t, int64(1), checkpoints)
}

func TestJournalBoundaryCheckpointFailureRollsBackSucceededState(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalBoundaryAttempt(t, db)
	seedExecutingBoundaryLedger(t, repo)

	expected := errors.New("forced checkpoint failure")
	_, err := repo.CommitExecutionBoundary(
		context.Background(),
		CommitExecutionBoundaryRequest{
			JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
			ExpectedVersion: 2, Status: entity.SideEffectLedgerStatusSucceeded,
			ResultEvent: boundaryResultEvent(1003, "succeeded"),
			AuditEvent: boundaryAuditEvent(
				1004, "side-effect:tool-1:succeeded", "succeeded",
			),
			CheckpointFactory: func(uint64, []*entity.SideEffectLedger) (*entity.Checkpoint, error) {
				return nil, expected
			},
		},
	)
	require.ErrorIs(t, err, expected)

	stored, err := repo.GetSideEffectLedger(context.Background(), 10, "att_10", "tool-1")
	require.NoError(t, err)
	require.Equal(t, entity.SideEffectLedgerStatusExecuting, stored.Status)
	require.Equal(t, uint64(2), stored.Version)

	var attempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, uint64(1), attempt.NextSequence)
	require.Zero(t, attempt.LastCommittedSequence)

	var resultEvents, checkpoints int64
	require.NoError(t, db.Model(&runEventPO{}).
		Where("id IN ?", []int64{1003, 1004}).Count(&resultEvents).Error)
	require.NoError(t, db.Model(&checkpointPO{}).Count(&checkpoints).Error)
	require.Zero(t, resultEvents)
	require.Zero(t, checkpoints)
}

func TestJournalBoundaryExecutingTimeoutBecomesUnknown(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalBoundaryAttempt(t, db)
	seedExecutingBoundaryLedger(t, repo)

	unknown, changed, err := repo.TransitionSideEffect(
		context.Background(),
		TransitionSideEffectRequest{
			JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
			ExpectedVersion: 2,
			FromStatus:      entity.SideEffectLedgerStatusExecuting,
			ToStatus:        entity.SideEffectLedgerStatusUnknown,
			OccurredAt:      1300,
			AuditEvent: boundaryAuditEvent(
				1003, "side-effect:tool-1:unknown", "unknown",
			),
		},
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, entity.SideEffectLedgerStatusUnknown, unknown.Status)
	require.Equal(t, uint64(3), unknown.Version)
}

func TestJournalBoundaryUnknownResolutionIsAuditedAndIdempotent(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalBoundaryAttempt(t, db)
	seedTerminalBoundaryLedger(t, db, entity.SideEffectLedgerStatusUnknown)

	resolved, changed, err := repo.ResolveUnknownSideEffect(
		context.Background(),
		ResolveUnknownSideEffectRequest{
			JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
			ExpectedVersion: 3,
			Action:          entity.SideEffectResolutionActionSkip,
			IdempotencyKey:  "resolve-unknown-1",
			OccurredAt:      1400,
			AuditEvent: boundaryAuditEvent(
				1005, "side-effect:tool-1:resolution:resolve-unknown-1", "resolved",
			),
		},
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, entity.SideEffectLedgerStatusUnknown, resolved.Status)
	require.Equal(t, entity.SideEffectResolutionActionSkip, resolved.ResolutionAction)
	require.Equal(t, "resolve-unknown-1", resolved.ResolutionIdempotencyKey)
	require.Equal(t, uint64(4), resolved.Version)
	require.NotNil(t, resolved.ResolvedAt)

	replayed, changed, err := repo.ResolveUnknownSideEffect(
		context.Background(),
		ResolveUnknownSideEffectRequest{
			JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
			ExpectedVersion: 3,
			Action:          entity.SideEffectResolutionActionSkip,
			IdempotencyKey:  "resolve-unknown-1",
			OccurredAt:      1500,
			AuditEvent: boundaryAuditEvent(
				1006, "side-effect:tool-1:resolution:resolve-unknown-1", "resolved",
			),
		},
	)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, resolved.Version, replayed.Version)

	_, _, err = repo.ResolveUnknownSideEffect(
		context.Background(),
		ResolveUnknownSideEffectRequest{
			JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
			ExpectedVersion: 4,
			Action:          entity.SideEffectResolutionActionRetry,
			IdempotencyKey:  "resolve-unknown-2",
			OccurredAt:      1600,
			AuditEvent: boundaryAuditEvent(
				1007, "side-effect:tool-1:resolution:resolve-unknown-2", "resolved",
			),
		},
	)
	require.ErrorIs(t, err, ErrSideEffectLedgerConflict)

	var auditCount int64
	require.NoError(t, db.Model(&runEventPO{}).
		Where("idempotency_key = ?", "side-effect:tool-1:resolution:resolve-unknown-1").
		Count(&auditCount).Error)
	require.Equal(t, int64(1), auditCount)
}

func TestJournalBoundaryCompensationRequiresRegisteredSuccess(t *testing.T) {
	tests := []struct {
		name                   string
		compensationRegistered bool
		compensationSucceeded  bool
	}{
		{name: "not registered", compensationSucceeded: true},
		{name: "failed", compensationRegistered: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newJournalBoundaryTestDB(t)
			repo := NewThreadRepository(db)
			seedJournalBoundaryAttempt(t, db)
			seedTerminalBoundaryLedger(t, db, entity.SideEffectLedgerStatusSucceeded)

			_, _, err := repo.TransitionSideEffect(
				context.Background(),
				TransitionSideEffectRequest{
					JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
					ExpectedVersion:        3,
					FromStatus:             entity.SideEffectLedgerStatusSucceeded,
					ToStatus:               entity.SideEffectLedgerStatusCompensated,
					OccurredAt:             1400,
					CompensationRegistered: test.compensationRegistered,
					CompensationSucceeded:  test.compensationSucceeded,
					AuditEvent: boundaryAuditEvent(
						1005, "side-effect:tool-1:compensated", "compensated",
					),
				},
			)
			require.Error(t, err)
			stored, loadErr := repo.GetSideEffectLedger(
				context.Background(), 10, "att_10", "tool-1",
			)
			require.NoError(t, loadErr)
			require.Equal(t, entity.SideEffectLedgerStatusSucceeded, stored.Status)
			require.Nil(t, stored.CompensatedAt)
		})
	}
}

func TestJournalBoundaryRegisteredCompensationSucceedsAndReplays(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalBoundaryAttempt(t, db)
	seedTerminalBoundaryLedger(t, db, entity.SideEffectLedgerStatusSucceeded)

	request := TransitionSideEffectRequest{
		JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
		ExpectedVersion:        3,
		FromStatus:             entity.SideEffectLedgerStatusSucceeded,
		ToStatus:               entity.SideEffectLedgerStatusCompensated,
		OccurredAt:             1400,
		CompensationRegistered: true,
		CompensationSucceeded:  true,
		AuditEvent: boundaryAuditEvent(
			1005, "side-effect:tool-1:compensated", "compensated",
		),
	}
	compensated, changed, err := repo.TransitionSideEffect(context.Background(), request)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, entity.SideEffectLedgerStatusCompensated, compensated.Status)
	require.Equal(t, uint64(4), compensated.Version)
	require.NotNil(t, compensated.CompensatedAt)

	replayed, changed, err := repo.TransitionSideEffect(context.Background(), request)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, compensated.Version, replayed.Version)

	var auditCount int64
	require.NoError(t, db.Model(&runEventPO{}).
		Where("idempotency_key = ?", "side-effect:tool-1:compensated").
		Count(&auditCount).Error)
	require.Equal(t, int64(1), auditCount)
}

func TestJournalRecoveryRunBundleCommitsRunAndAttemptAndReplays(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRecoverySource(t, db)

	created, err := repo.CreateRunBundle(
		context.Background(), recoveryRunBundle(20, 200, "recover-1"),
	)
	require.NoError(t, err)
	require.True(t, created.Created)
	require.Equal(t, int64(20), created.Run.ID)
	require.Equal(t, int64(10), created.Attempt.JournalRunID)
	require.Equal(t, int64(20), created.Attempt.ExecutionRunID)
	require.Equal(t, uint32(2), created.Attempt.Ordinal)
	require.Equal(t, "1.1", created.Attempt.EnrollmentVersion)
	require.Equal(t, "recover-1", *created.Attempt.RecoveryIdempotencyKey)

	replayed, err := repo.CreateRunBundle(
		context.Background(), recoveryRunBundle(21, 201, "recover-1"),
	)
	require.NoError(t, err)
	require.False(t, replayed.Created)
	require.Equal(t, created.Run.ID, replayed.Run.ID)
	require.Equal(t, created.Attempt.ID, replayed.Attempt.ID)

	var runCount, attemptCount int64
	require.NoError(t, db.Model(&runPO{}).Count(&runCount).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Count(&attemptCount).Error)
	require.Equal(t, int64(2), runCount)
	require.Equal(t, int64(2), attemptCount)
}

func TestJournalRecoveryRunBundleRejectsDifferentKeyWithoutOrphans(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRecoverySource(t, db)

	_, err := repo.CreateRunBundle(
		context.Background(), recoveryRunBundle(20, 200, "recover-1"),
	)
	require.NoError(t, err)

	_, err = repo.CreateRunBundle(
		context.Background(), recoveryRunBundle(21, 201, "recover-2"),
	)
	require.ErrorIs(t, err, ErrActiveJournalAttemptExists)

	var runCount, attemptCount int64
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 21).Count(&runCount).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 201).Count(&attemptCount).Error)
	require.Zero(t, runCount)
	require.Zero(t, attemptCount)
}

func TestJournalRecoveryRunBundleConcurrentKeysAdmitOneAttempt(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRecoverySource(t, db)

	start := make(chan struct{})
	type result struct {
		bundle *CreateRunBundleResult
		err    error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			bundle, err := repo.CreateRunBundle(
				context.Background(),
				recoveryRunBundle(
					int64(20+index), int64(200+index), fmt.Sprintf("recover-concurrent-%d", index),
				),
			)
			results <- result{bundle: bundle, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	created := 0
	for result := range results {
		if result.err == nil {
			require.NotNil(t, result.bundle)
			require.True(t, result.bundle.Created)
			created++
			continue
		}
		require.ErrorIs(t, result.err, ErrActiveJournalAttemptExists)
	}
	require.Equal(t, 1, created)

	var activeAttempts, recoveryRuns int64
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("journal_run_id = ? AND active_slot = ?", 10, 1).
		Count(&activeAttempts).Error)
	require.NoError(t, db.Model(&runPO{}).
		Where("id IN ?", []int64{20, 21}).Count(&recoveryRuns).Error)
	require.Equal(t, int64(1), activeAttempts)
	require.Equal(t, int64(1), recoveryRuns)
}

func TestJournalRecoveryRunBundleRollsBackRunWhenAttemptInsertFails(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRecoverySource(t, db)

	request := recoveryRunBundle(20, 100, "recover-attempt-conflict")
	_, err := repo.CreateRunBundle(context.Background(), request)
	require.Error(t, err)

	var runCount, recoveryCount int64
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 20).Count(&runCount).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("journal_run_id = ? AND recovery_idempotency_key = ?", 10, "recover-attempt-conflict").
		Count(&recoveryCount).Error)
	require.Zero(t, runCount)
	require.Zero(t, recoveryCount)
}

func TestJournalLeaseRecoveryBundleAtomicallyTerminatesExpiredSource(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalLeaseRecoverySource(t, db)

	request := journalLeaseRecoveryRunBundle(20, 200, "lease-recover-1")
	created, err := repo.CreateRunBundle(context.Background(), request)
	require.NoError(t, err)
	require.True(t, created.Created)
	require.Equal(t, int64(20), created.Run.ID)
	require.Equal(t, uint32(2), created.Attempt.Ordinal)

	var source runPO
	require.NoError(t, db.Where("id = ?", 10).First(&source).Error)
	require.Equal(t, string(entity.RunStatusFailed), source.Status)
	require.Equal(t, runRecoveredErrorCodeForRepositoryTest, source.ErrorCode)
	require.Nil(t, source.LeaseOwner)
	require.Nil(t, source.LeaseToken)

	var sourceAttempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&sourceAttempt).Error)
	require.Equal(t, string(entity.RunAttemptStatusFailed), sourceAttempt.Status)
	require.Nil(t, sourceAttempt.ActiveSlot)
	require.NotNil(t, sourceAttempt.TerminalEventID)
	require.Equal(t, int64(1008), *sourceAttempt.TerminalEventID)

	var terminal runEventPO
	require.NoError(t, db.Where("id = ?", 1008).First(&terminal).Error)
	require.Equal(t, "run.failed", terminal.EventType)
	require.NotNil(t, terminal.JournalRunID)
	require.Equal(t, int64(10), *terminal.JournalRunID)
	require.NotNil(t, terminal.AttemptID)
	require.Equal(t, "att_100", *terminal.AttemptID)
	require.NotNil(t, terminal.Sequence)
	require.Equal(t, uint64(1), *terminal.Sequence)

	var activeCount int64
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("journal_run_id = ? AND active_slot = ?", 10, 1).
		Count(&activeCount).Error)
	require.Equal(t, int64(1), activeCount)
}

func TestJournalLeaseRecoveryBundleRollsBackSourceWhenNewAttemptFails(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalLeaseRecoverySource(t, db)

	request := journalLeaseRecoveryRunBundle(20, 100, "lease-recover-conflict")
	_, err := repo.CreateRunBundle(context.Background(), request)
	require.Error(t, err)

	var source runPO
	require.NoError(t, db.Where("id = ?", 10).First(&source).Error)
	require.Equal(t, string(entity.RunStatusRunning), source.Status)
	require.NotNil(t, source.LeaseOwner)
	require.Equal(t, "worker-a", *source.LeaseOwner)
	require.NotNil(t, source.LeaseToken)
	require.Equal(t, "lease-10", *source.LeaseToken)

	var sourceAttempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&sourceAttempt).Error)
	require.Equal(t, string(entity.RunAttemptStatusRunning), sourceAttempt.Status)
	require.NotNil(t, sourceAttempt.ActiveSlot)
	require.Nil(t, sourceAttempt.TerminalEventID)

	var runCount, eventCount int64
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 20).Count(&runCount).Error)
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 1008).Count(&eventCount).Error)
	require.Zero(t, runCount)
	require.Zero(t, eventCount)
}

func TestJournalLeaseRecoveryBundleRollsBackSourceWhenNewRunFails(t *testing.T) {
	db := newJournalBoundaryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalLeaseRecoverySource(t, db)
	conflicting := newRepositoryTestRun(20, 1, entity.RunStatusFailed, 20)
	conflicting.SpaceID = 10
	conflicting.CreatorID = 20
	conflicting.RunKind = entity.RunKindTask
	conflicting.IdempotencyKey = "another-request"
	conflictingPO, err := runToPO(conflicting)
	require.NoError(t, err)
	require.NoError(t, db.Create(conflictingPO).Error)

	request := journalLeaseRecoveryRunBundle(20, 200, "lease-recover-run-conflict")
	_, err = repo.CreateRunBundle(context.Background(), request)
	require.Error(t, err)

	var source runPO
	require.NoError(t, db.Where("id = ?", 10).First(&source).Error)
	require.Equal(t, string(entity.RunStatusRunning), source.Status)
	require.NotNil(t, source.LeaseToken)
	require.Equal(t, "lease-10", *source.LeaseToken)

	var sourceAttempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&sourceAttempt).Error)
	require.Equal(t, string(entity.RunAttemptStatusRunning), sourceAttempt.Status)
	require.NotNil(t, sourceAttempt.ActiveSlot)
	require.Nil(t, sourceAttempt.TerminalEventID)

	var attemptCount, eventCount int64
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 200).Count(&attemptCount).Error)
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 1008).Count(&eventCount).Error)
	require.Zero(t, attemptCount)
	require.Zero(t, eventCount)
}

func newJournalBoundaryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newJournalRepositoryTestDB(t)
	require.NoError(t, db.AutoMigrate(&sideEffectLedgerPO{}, &checkpointPO{}))
	return db
}

func seedJournalBoundaryAttempt(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedJournalRunWithStatus(t, db, 10, 1, entity.RunStatusRunning)
	startedAt := int64(1000)
	activeSlot := uint8(1)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 100, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att_10", Ordinal: 1, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &activeSlot, NextSequence: 1, LastCommittedSequence: 0,
		EnrollmentVersion: entity.JournalSchemaVersion,
		SnapshotsEnabled:  true,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		CreatedAt:         1000, UpdatedAt: 1000, StartedAt: &startedAt,
	}).Error)
}

func boundaryLedger(status entity.SideEffectLedgerStatus) *entity.SideEffectLedger {
	return &entity.SideEffectLedger{
		ID: 500, ThreadID: 1, JournalRunID: 10, AttemptID: "att_10",
		IdempotencyKey: "tool-1", ActionKind: "write_file",
		ReplayPolicy:     entity.SideEffectReplayPolicyIdempotentWrite,
		Status:           status,
		RequestHash:      boundaryDigest("request"),
		RequestSummary:   `{"keys":["file_path"]}`,
		CompensationKind: "delete_output_file",
		Version:          1, PreparedAt: 1100, CreatedAt: 1100, UpdatedAt: 1100,
	}
}

func seedExecutingBoundaryLedger(t *testing.T, repo Repository) {
	t.Helper()
	_, _, err := repo.PrepareSideEffect(context.Background(), PrepareSideEffectRequest{
		Ledger: boundaryLedger(entity.SideEffectLedgerStatusPrepared),
		AuditEvent: boundaryAuditEvent(
			1001, "side-effect:tool-1:prepared", "prepared",
		),
	})
	require.NoError(t, err)
	_, changed, err := repo.TransitionSideEffect(
		context.Background(),
		TransitionSideEffectRequest{
			JournalRunID: 10, AttemptID: "att_10", LedgerID: 500,
			ExpectedVersion: 1,
			FromStatus:      entity.SideEffectLedgerStatusPrepared,
			ToStatus:        entity.SideEffectLedgerStatusExecuting,
			OccurredAt:      1200,
			AuditEvent: boundaryAuditEvent(
				1002, "side-effect:tool-1:executing", "executing",
			),
		},
	)
	require.NoError(t, err)
	require.True(t, changed)
}

func seedTerminalBoundaryLedger(
	t *testing.T,
	db *gorm.DB,
	status entity.SideEffectLedgerStatus,
) {
	t.Helper()
	po := sideEffectLedgerToPO(boundaryLedger(status))
	po.Status = string(status)
	po.Version = 3
	when := int64(1300)
	executingAt := int64(1200)
	po.ExecutingAt = &executingAt
	switch status {
	case entity.SideEffectLedgerStatusSucceeded:
		po.SucceededAt = &when
	case entity.SideEffectLedgerStatusFailed:
		po.FailedAt = &when
	case entity.SideEffectLedgerStatusUnknown:
		po.UnknownAt = &when
	}
	require.NoError(t, db.Create(po).Error)
}

func seedJournalRecoverySource(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedJournalRunWithStatus(t, db, 10, 1, entity.RunStatusSucceeded)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusCompleted, 1)
	require.NoError(t, db.Create(&checkpointPO{
		ID: 700, ThreadID: 1, RunID: 10, CheckpointNS: "eino.adk",
		RuntimeType: "eino_adk", RuntimeKey: "run-10", EnvelopeVersion: 3,
		ChannelValues:   []byte(`{"schema_version":"coze.adk.checkpoint.v3"}`),
		ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`), Metadata: []byte(`{}`),
		CreatedAt: 2,
	}).Error)
}

const runRecoveredErrorCodeForRepositoryTest = "run_recovered"

func seedJournalLeaseRecoverySource(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedJournalRunWithStatus(t, db, 10, 1, entity.RunStatusRunning)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 10).Updates(map[string]any{
		"worker_id":            "worker-a",
		"lease_owner":          "worker-a",
		"lease_token":          "lease-10",
		"lease_expires_at":     int64(1500),
		"heartbeat_at":         int64(1400),
		"execution_generation": uint64(3),
	}).Error)
	require.NoError(t, db.Create(&checkpointPO{
		ID: 700, ThreadID: 1, RunID: 10, CheckpointNS: "eino.adk",
		RuntimeType: "eino_adk", RuntimeKey: "run-10", EnvelopeVersion: 3,
		ChannelValues:   []byte(`{"schema_version":"coze.adk.checkpoint.v3"}`),
		ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`), Metadata: []byte(`{}`),
		CreatedAt: 1450,
	}).Error)
}

func journalLeaseRecoveryRunBundle(
	runID, attemptID int64,
	recoveryKey string,
) CreateRunBundleRequest {
	request := recoveryRunBundle(runID, attemptID, recoveryKey)
	request.SkipTopLevelAdmission = false
	sourceAttemptID := "att_100"
	request.Attempt.SourceAttemptID = &sourceAttemptID
	request.RecoverySourceLease = &ReconcileExpiredRunLeaseRequest{
		RunID: 10, LeaseOwner: "worker-a", LeaseToken: "lease-10",
		ExecutionGeneration: 3, ToStatus: entity.RunStatusFailed, Now: 2000,
		ErrorCode:    runRecoveredErrorCodeForRepositoryTest,
		ErrorMessage: "execution recovered from a durable checkpoint",
		Event: &entity.RunEvent{
			ID: 1008, ThreadID: 1, RunID: 10, EventType: "run.failed",
			Payload: `{"status":"failed","error_code":"run_recovered"}`, CreatedAt: 2000,
		},
		JournalEvent: &entity.JournalEvent{
			ID: 1008, ThreadID: 1, RunID: 10,
			IdempotencyKey: "journal:run:10:terminal:failed",
			SchemaVersion:  entity.JournalSchemaVersion,
			Status:         string(entity.RunAttemptStatusFailed),
			Visibility:     entity.JournalVisibilityUser,
			PayloadVersion: entity.JournalPayloadVersion,
			EventType:      "run.lifecycle",
			Payload:        `{"type":"terminal","data":{"status":"failed"}}`,
			CreatedAt:      2000,
		},
	}
	return request
}

func recoveryRunBundle(runID, attemptID int64, recoveryKey string) CreateRunBundleRequest {
	run := newRepositoryTestRun(runID, 1, entity.RunStatusQueued, runID)
	run.SpaceID = 10
	run.CreatorID = 20
	run.RunKind = entity.RunKindTask
	run.IdempotencyKey = recoveryKey
	sourceCheckpointID := int64(700)
	sourceAttemptID := "att_100"
	return CreateRunBundleRequest{
		Run:                   run,
		SkipTopLevelAdmission: true,
		Attempt: &entity.RunAttempt{
			ID: attemptID, ThreadID: 1, JournalRunID: 10, ExecutionRunID: runID,
			AttemptID:              fmt.Sprintf("att_%d", attemptID),
			Status:                 entity.RunAttemptStatusPending,
			SourceCheckpointID:     &sourceCheckpointID,
			SourceAttemptID:        &sourceAttemptID,
			RecoveryIdempotencyKey: &recoveryKey,
			ProjectionState:        entity.JournalProjectionStateHealthy,
		},
	}
}

func boundaryAuditEvent(id int64, idempotencyKey, phase string) *entity.JournalEvent {
	return &entity.JournalEvent{
		ID: id, ThreadID: 1, RunID: 10, JournalRunID: 10, AttemptID: "att_10",
		IdempotencyKey: idempotencyKey, SchemaVersion: entity.JournalSchemaVersion,
		Status: phase, Visibility: entity.JournalVisibilityInternal,
		PayloadVersion: entity.JournalPayloadVersion,
		ActionID:       "tool-1", Phase: "ledger." + phase,
		Operation: "write_file", Target: "output file", Milestone: "execution",
		EventType: "side_effect.audit",
		Payload:   fmt.Sprintf(`{"type":"generic","data":{"status":%q}}`, phase),
		CreatedAt: 1200,
	}
}

func boundaryResultEvent(id int64, status string) *entity.JournalEvent {
	return &entity.JournalEvent{
		ID: id, ThreadID: 1, RunID: 10, JournalRunID: 10, AttemptID: "att_10",
		IdempotencyKey: "side-effect:tool-1:result", SchemaVersion: entity.JournalSchemaVersion,
		Status: status, Visibility: entity.JournalVisibilityUser,
		PayloadVersion: entity.JournalPayloadVersion,
		ActionID:       "tool-1", Phase: "terminal",
		Operation: "write_file", Target: "output file", Milestone: "execution",
		EventType: "action.terminal",
		Payload:   fmt.Sprintf(`{"type":"generic","data":{"status":%q}}`, status),
		CreatedAt: 1300,
	}
}

func boundaryCheckpoint(lastCommittedSequence uint64) *entity.Checkpoint {
	return &entity.Checkpoint{
		ID: 900, ThreadID: 1, RunID: 10, CheckpointNS: "eino.adk",
		RuntimeType: "eino_adk", RuntimeKey: "run-10", EnvelopeVersion: 3,
		ChannelValues: fmt.Sprintf(
			`{"schema_version":"coze.adk.checkpoint.v3","attempt_id":"att_10","last_committed_sequence":%d,"runtime_state":{},"side_effect_ledger":[]}`,
			lastCommittedSequence,
		),
		ChannelVersions: `{}`, PendingSends: `[]`, Metadata: `{}`, CreatedAt: 1300,
	}
}

func boundaryDigest(value string) string {
	return fmt.Sprintf("%064x", value)
}
