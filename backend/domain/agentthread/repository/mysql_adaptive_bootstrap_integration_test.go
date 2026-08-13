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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestOrdinaryLeaseRecoveryMySQLCheckpointAuthorityRace(t *testing.T) {
	assertRejectedWithoutWrites := func(
		t *testing.T,
		db *gorm.DB,
		repo *threadRepository,
		source *entity.Run,
		req CreateRunBundleRequest,
	) {
		t.Helper()
		result, err := repo.CreateRunBundle(context.Background(), req)
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrJournalParentMismatch)

		var storedSource runPO
		require.NoError(t, db.Where("id = ?", source.ID).First(&storedSource).Error)
		require.Equal(t, string(entity.RunStatusRunning), storedSource.Status)
		require.NotNil(t, storedSource.LeaseToken)
		require.Equal(t, source.LeaseToken, *storedSource.LeaseToken)

		var targetRuns, attempts, events int64
		require.NoError(t, db.Model(&runPO{}).Where("id = ?", req.Run.ID).Count(&targetRuns).Error)
		require.NoError(t, db.Model(&runAttemptPO{}).
			Where("execution_run_id IN ?", []int64{source.ID, req.Run.ID}).Count(&attempts).Error)
		require.NoError(t, db.Model(&runEventPO{}).Where("run_id = ?", source.ID).Count(&events).Error)
		require.Zero(t, targetRuns)
		require.Zero(t, attempts)
		require.Zero(t, events)
	}

	for _, tc := range []struct {
		name  string
		drift func(t *testing.T, db *gorm.DB, checkpointID int64)
	}{
		{
			name: "payload_drift",
			drift: func(t *testing.T, db *gorm.DB, checkpointID int64) {
				require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", checkpointID).Update(
					"channel_values",
					datatypes.JSON([]byte(`{"messages":["drifted-after-authority-read"]}`)),
				).Error)
			},
		},
		{
			name: "runtime_authority_drift",
			drift: func(t *testing.T, db *gorm.DB, checkpointID int64) {
				require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", checkpointID).
					Update("runtime_key", "checkpoint-drifted-after-authority-read").Error)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, repoA, repoB := adaptiveExecutionMySQLIntegrationRepositories(t)
			source := seedOrdinaryLeaseRecoverySource(t, db, 10)
			checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
			authority, err := repoA.GetCheckpoint(context.Background(), checkpoint.ID)
			require.NoError(t, err)
			req := ordinaryLeaseRecoveryBundleRequest(source, authority, 20, 100)

			tc.drift(t, repoB.db, checkpoint.ID)

			assertRejectedWithoutWrites(t, db, repoA, source, req)
		})
	}

	t.Run("exact_committed_retry", func(t *testing.T) {
		db, repoA, repoB := adaptiveExecutionMySQLIntegrationRepositories(t)
		source := seedOrdinaryLeaseRecoverySource(t, db, 10)
		checkpoint := seedOrdinaryLeaseRecoveryCheckpoint(t, db, 500, source.ID)
		authority, err := repoA.GetCheckpoint(context.Background(), checkpoint.ID)
		require.NoError(t, err)
		req := ordinaryLeaseRecoveryBundleRequest(source, authority, 20, 100)

		committed, err := repoA.CreateRunBundle(context.Background(), req)
		require.NoError(t, err)
		require.True(t, committed.Created)
		replayed, err := repoB.CreateRunBundle(context.Background(), req)
		require.NoError(t, err)
		require.False(t, replayed.Created)
		require.Equal(t, committed.Run, replayed.Run)
		require.Equal(t, committed.Attempt, replayed.Attempt)

		var targetRuns, sourceAttempts, targetAttempts, events int64
		require.NoError(t, db.Model(&runPO{}).Where("id = ?", req.Run.ID).Count(&targetRuns).Error)
		require.NoError(t, db.Model(&runAttemptPO{}).
			Where("execution_run_id = ?", source.ID).Count(&sourceAttempts).Error)
		require.NoError(t, db.Model(&runAttemptPO{}).
			Where("execution_run_id = ?", req.Run.ID).Count(&targetAttempts).Error)
		require.NoError(t, db.Model(&runEventPO{}).Where("run_id = ?", source.ID).Count(&events).Error)
		require.Equal(t, int64(1), targetRuns)
		require.Zero(t, sourceAttempts)
		require.Equal(t, int64(1), targetAttempts)
		require.Equal(t, int64(1), events)
	})
}

func TestAdaptiveExecutionBootstrapMySQLIntegrationTypedRecoveryRace(t *testing.T) {
	db, repoA, repoB := adaptiveExecutionMySQLIntegrationRepositories(t)
	seedAdaptiveExecutionMySQLState(t, db)
	fixture := seedAdaptiveBootstrapRecoveryMySQLFixture(t, db)

	type outcome struct {
		result *CommitAdaptiveExecutionBootstrapResult
		err    error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	commit := func(repo *threadRepository) {
		<-start
		result, err := repo.CommitAdaptiveExecutionBootstrap(ctx, fixture.TargetRequest)
		outcomes <- outcome{result: result, err: err}
	}
	go commit(repoA)
	go commit(repoB)
	close(start)
	results := []outcome{<-outcomes, <-outcomes}

	committed, replayed := 0, 0
	for _, result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.result)
		require.Equal(t, entity.AdaptiveAdmissionSourceTypedInheritance, result.result.Admission.Source)
		if result.result.Replayed {
			replayed++
		} else {
			committed++
		}
	}
	require.Equal(t, 1, committed)
	require.Equal(t, 1, replayed)
	require.Equal(t, results[0].result.Admission, results[1].result.Admission)
	require.Equal(t, results[0].result.Decision, results[1].result.Decision)
	require.Equal(t, results[0].result.Authority, results[1].result.Authority)

	beforeDrift := assertTypedRecoveryMySQLFacts(t, db, fixture.TargetRequest)
	drifted := fixture.TargetRequest
	drifted.Decision.SafeSummary = "semantically drifted recovery decision"
	_, err := repoB.CommitAdaptiveExecutionBootstrap(context.Background(), drifted)
	require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
	require.Equal(t, beforeDrift, assertTypedRecoveryMySQLFacts(t, db, fixture.TargetRequest))

	durable, err := repoA.ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: fixture.TargetRequest.ThreadID, ExecutionRunID: fixture.TargetRequest.ExecutionRunID,
		JournalRunID: fixture.TargetRequest.JournalRunID, AttemptID: fixture.TargetRequest.AttemptID,
	})
	require.NoError(t, err)
	require.Equal(t, results[0].result.Admission, durable.Admission)
	require.Equal(t, results[0].result.Decision, durable.Decision)
	require.Equal(t, results[0].result.Authority, durable.Authority)
}

func TestAdaptiveExecutionBootstrapMySQLIntegrationLegacyDecoderRecovery(t *testing.T) {
	db, repoA, _ := adaptiveExecutionMySQLIntegrationRepositories(t)
	seedAdaptiveExecutionMySQLState(t, db)
	fixture, sourceConfig := seedAdaptiveBootstrapLegacyRecoveryMySQLFixture(t, db)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(sourceConfig, &decoded))
	require.Equal(t, "pro", decoded["mode"])
	require.Equal(t, "pro", decoded["requested_policy"])
	require.Equal(t, legacyRecoveryConfigDigest(sourceConfig), fixture.TargetRequest.Admission.SourceConfigDigest)

	for _, drift := range []struct {
		name    string
		mutate  func()
		restore func()
	}{
		{
			name: "config",
			mutate: func() {
				require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).
					Update("config", datatypes.JSON([]byte(`{"mode":"ultra","requested_policy":"ultra"}`))).Error)
			},
			restore: func() {
				require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).
					Update("config", datatypes.JSON(sourceConfig)).Error)
			},
		},
		{
			name: "generation",
			mutate: func() {
				require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).
					Update("execution_generation", uint64(4)).Error)
			},
			restore: func() {
				require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).
					Update("execution_generation", uint64(3)).Error)
			},
		},
	} {
		t.Run(drift.name+"_drift_conflict", func(t *testing.T) {
			drift.mutate()
			before := adaptiveBootstrapMySQLFactCounts(t, db, fixture.TargetRequest)
			_, err := repoA.CommitAdaptiveExecutionBootstrap(context.Background(), fixture.TargetRequest)
			require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
			require.Equal(t, before, adaptiveBootstrapMySQLFactCounts(t, db, fixture.TargetRequest))
			drift.restore()
		})
	}

	committed, err := repoA.CommitAdaptiveExecutionBootstrap(context.Background(), fixture.TargetRequest)
	require.NoError(t, err)
	require.False(t, committed.Replayed)
	require.Equal(t, entity.AdaptiveAdmissionSourceLegacyDecoder, committed.Admission.Source)
	require.Equal(t, sourceConfig, readAdaptiveBootstrapSourceConfig(t, db, 20))

	read, err := repoA.ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: fixture.TargetRequest.ThreadID, ExecutionRunID: fixture.TargetRequest.ExecutionRunID,
		JournalRunID: fixture.TargetRequest.JournalRunID, AttemptID: fixture.TargetRequest.AttemptID,
	})
	require.NoError(t, err)
	require.Equal(t, committed.Admission, read.Admission)
	require.Equal(t, committed.Decision, read.Decision)
	require.Equal(t, committed.Authority, read.Authority)

	replayed, err := repoA.CommitAdaptiveExecutionBootstrap(context.Background(), fixture.TargetRequest)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, committed.Admission, replayed.Admission)
	require.Equal(t, committed.Decision, replayed.Decision)
	require.Equal(t, committed.Authority, replayed.Authority)
	assertTypedRecoveryMySQLFacts(t, db, fixture.TargetRequest)
}

func seedAdaptiveBootstrapRecoveryMySQLFixture(
	t *testing.T,
	db *gorm.DB,
) adaptiveBootstrapRecoveryFixture {
	t.Helper()
	repo := &threadRepository{db: db}
	source, err := repo.CommitAdaptiveExecutionBootstrap(
		context.Background(),
		newAdaptiveExecutionBootstrapRequestForTest(),
	)
	require.NoError(t, err)

	terminalID := int64(7901)
	terminal, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
		RunID:   20,
		Status:  entity.RunAttemptStatusCompleted,
		EndedAt: 750,
		Event: &entity.JournalEvent{
			ID: terminalID, ThreadID: 10, RunID: 20,
			JournalRunID: 30, AttemptID: "attempt-1",
			IdempotencyKey: "typed-recovery-source-terminal",
			EventType:      "run.lifecycle", Status: string(entity.RunAttemptStatusCompleted),
			Visibility:         entity.JournalVisibilityUser,
			Payload:            `{"type":"terminal","data":{"status":"completed"}}`,
			OccurredAtUnixNano: 750 * int64(time.Millisecond), CreatedAt: 750,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	require.Equal(t, terminalID, terminal.ID)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).Updates(map[string]any{
		"status": string(entity.RunStatusSucceeded), "ended_at": int64(750), "updated_at": int64(750),
	}).Error)

	sourceCheckpointID := int64(9001)
	require.NoError(t, db.Create(&checkpointPO{
		ID: sourceCheckpointID, ThreadID: 10, RunID: 20,
		CheckpointNS: "eino.adk", RuntimeType: "eino_adk", RuntimeKey: "coze-run-20",
		EnvelopeVersion: 2, ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`),
		PendingSends: []byte(`[]`), Metadata: []byte(`{"runtime":"eino_adk"}`), CreatedAt: 740,
	}).Error)

	leaseOwner, leaseToken, recoveryKey := "worker-2", "lease-2", "recover-2"
	leaseExpiresAt, startedAt := int64(3000), int64(800)
	require.NoError(t, db.Create(&runPO{
		ID: 21, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		AssistantID: "agent", RunKind: string(entity.RunKindTask), Status: string(entity.RunStatusRunning),
		Command: datatypes.JSON([]byte(`{}`)), Input: datatypes.JSON([]byte(`{}`)),
		Config: datatypes.JSON([]byte(`{}`)), Context: datatypes.JSON([]byte(`{}`)),
		Metadata: datatypes.JSON([]byte(`{}`)), StreamMode: datatypes.JSON([]byte(`[]`)),
		ExecutionGeneration: 4, LeaseOwner: &leaseOwner, LeaseToken: &leaseToken,
		LeaseExpiresAt: &leaseExpiresAt, StartedAt: startedAt, CreatedAt: 800, UpdatedAt: 800,
	}).Error)
	active := uint8(1)
	sourceAttemptID := "attempt-1"
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 102, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 21,
		AttemptID: "attempt-2", Ordinal: 2, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &active, NextSequence: 1, LastCommittedSequence: 0,
		SourceAttemptID: &sourceAttemptID, SourceCheckpointID: &sourceCheckpointID,
		RecoveryIdempotencyKey: &recoveryKey, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: string(entity.JournalProjectionStateHealthy), StartedAt: &startedAt,
		CreatedAt: 800, UpdatedAt: 800,
	}).Error)

	target := newAdaptiveExecutionBootstrapRequestForTest()
	target.ExecutionRunID, target.AttemptID, target.Generation = 21, "attempt-2", 4
	target.LeaseOwner, target.LeaseToken = leaseOwner, leaseToken
	target.OperationKey = "adaptive-operation:target"
	target.FactCreatedAt, target.Now = 800, 900
	target.AdmissionEventID, target.DecisionEventID, target.CheckpointID = 7101, 7102, 8101
	sourceRunID, sourceGeneration := int64(20), source.Authority.ExecutionGeneration
	target.Admission = source.Admission
	target.Admission.Source = entity.AdaptiveAdmissionSourceTypedInheritance
	target.Admission.SourceRunID = &sourceRunID
	target.Admission.SourceExecutionGeneration = &sourceGeneration
	target.Admission.SourceConfigDigest, target.Admission.DecoderVersion = "", ""
	planScope := int64(21)
	target.Decision = newAdaptiveBootstrapDecisionForTest(
		t, target.Admission, "decision-target", 21, 30, "attempt-2", 4, planScope, 800,
	)
	return adaptiveBootstrapRecoveryFixture{
		SourceResult: source, TargetRequest: target, SourceAttemptID: sourceAttemptID,
		SourceCheckpoint: sourceCheckpointID, RecoveryKey: recoveryKey,
	}
}

func seedAdaptiveBootstrapLegacyRecoveryMySQLFixture(
	t *testing.T,
	db *gorm.DB,
) (adaptiveBootstrapRecoveryFixture, []byte) {
	t.Helper()
	repo := &threadRepository{db: db}
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).Update(
		"config",
		datatypes.JSON([]byte(`{"mode":"pro","requested_policy":"pro","nested":{"requested_policy":"business-value"}}`)),
	).Error)
	storedConfig := readAdaptiveBootstrapSourceConfig(t, db, 20)

	terminalID := int64(7901)
	terminal, won, err := repo.FinalizeJournalAttempt(context.Background(), FinalizeJournalAttemptRequest{
		RunID: 20, Status: entity.RunAttemptStatusCompleted, EndedAt: 750,
		Event: &entity.JournalEvent{
			ID: terminalID, ThreadID: 10, RunID: 20,
			JournalRunID: 30, AttemptID: "attempt-1",
			IdempotencyKey: "legacy-recovery-source-terminal",
			EventType:      "run.lifecycle", Status: string(entity.RunAttemptStatusCompleted),
			Visibility:         entity.JournalVisibilityUser,
			Payload:            `{"type":"terminal","data":{"status":"completed"}}`,
			OccurredAtUnixNano: 750 * int64(time.Millisecond), CreatedAt: 750,
		},
	})
	require.NoError(t, err)
	require.True(t, won)
	require.Equal(t, terminalID, terminal.ID)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).Updates(map[string]any{
		"status": string(entity.RunStatusSucceeded), "ended_at": int64(750), "updated_at": int64(750),
	}).Error)

	sourceCheckpointID := int64(9001)
	require.NoError(t, db.Create(&checkpointPO{
		ID: sourceCheckpointID, ThreadID: 10, RunID: 20,
		CheckpointNS: "eino.adk", RuntimeType: "eino_adk", RuntimeKey: "coze-run-20",
		EnvelopeVersion: 2, ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`),
		PendingSends: []byte(`[]`), Metadata: []byte(`{"runtime":"eino_adk"}`), CreatedAt: 740,
	}).Error)

	leaseOwner, leaseToken, recoveryKey := "worker-2", "lease-2", "recover-2"
	leaseExpiresAt, startedAt := int64(3000), int64(800)
	require.NoError(t, db.Create(&runPO{
		ID: 21, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		AssistantID: "agent", RunKind: string(entity.RunKindTask), Status: string(entity.RunStatusRunning),
		Command: datatypes.JSON([]byte(`{}`)), Input: datatypes.JSON([]byte(`{}`)),
		Config: datatypes.JSON([]byte(`{}`)), Context: datatypes.JSON([]byte(`{}`)),
		Metadata: datatypes.JSON([]byte(`{}`)), StreamMode: datatypes.JSON([]byte(`[]`)),
		ExecutionGeneration: 4, LeaseOwner: &leaseOwner, LeaseToken: &leaseToken,
		LeaseExpiresAt: &leaseExpiresAt, StartedAt: startedAt, CreatedAt: 800, UpdatedAt: 800,
	}).Error)
	active := uint8(1)
	sourceAttemptID := "attempt-1"
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 102, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 21,
		AttemptID: "attempt-2", Ordinal: 2, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &active, NextSequence: 1, LastCommittedSequence: 0,
		SourceAttemptID: &sourceAttemptID, SourceCheckpointID: &sourceCheckpointID,
		RecoveryIdempotencyKey: &recoveryKey, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: string(entity.JournalProjectionStateHealthy), StartedAt: &startedAt,
		CreatedAt: 800, UpdatedAt: 800,
	}).Error)

	target := newAdaptiveExecutionBootstrapRequestForTest()
	target.ExecutionRunID, target.AttemptID, target.Generation = 21, "attempt-2", 4
	target.LeaseOwner, target.LeaseToken = leaseOwner, leaseToken
	target.OperationKey = "adaptive-operation:legacy-target"
	target.FactCreatedAt, target.Now = 800, 900
	target.AdmissionEventID, target.DecisionEventID, target.CheckpointID = 7101, 7102, 8101
	sourceRunID, sourceGeneration := int64(20), uint64(3)
	target.Admission.Source = entity.AdaptiveAdmissionSourceLegacyDecoder
	target.Admission.SourceRunID = &sourceRunID
	target.Admission.SourceExecutionGeneration = &sourceGeneration
	target.Admission.SourceConfigDigest = legacyRecoveryConfigDigest(storedConfig)
	target.Admission.DecoderVersion = entity.AdaptiveLegacyDecoderVersionV1
	planScope := int64(21)
	target.Decision = newAdaptiveBootstrapDecisionForTest(
		t, target.Admission, "decision-legacy", 21, 30, "attempt-2", 4, planScope, 800,
	)
	return adaptiveBootstrapRecoveryFixture{
		TargetRequest: target, SourceAttemptID: sourceAttemptID,
		SourceCheckpoint: sourceCheckpointID, RecoveryKey: recoveryKey,
	}, storedConfig
}

func readAdaptiveBootstrapSourceConfig(t *testing.T, db *gorm.DB, runID int64) []byte {
	t.Helper()
	var run runPO
	require.NoError(t, db.Where("id = ?", runID).First(&run).Error)
	require.True(t, json.Valid(run.Config))
	return append([]byte(nil), run.Config...)
}

func legacyRecoveryConfigDigest(config []byte) string {
	digest := sha256.Sum256(config)
	return hex.EncodeToString(digest[:])
}

type typedRecoveryMySQLFactCounts struct {
	AdmissionEvents int64
	DecisionEvents  int64
	Checkpoints     int64
	Attempts        int64
}

func assertTypedRecoveryMySQLFacts(
	t *testing.T,
	db *gorm.DB,
	req CommitAdaptiveExecutionBootstrapRequest,
) typedRecoveryMySQLFactCounts {
	t.Helper()
	counts := adaptiveBootstrapMySQLFactCounts(t, db, req)
	require.Equal(t, typedRecoveryMySQLFactCounts{
		AdmissionEvents: 1, DecisionEvents: 1, Checkpoints: 1, Attempts: 1,
	}, counts)
	return counts
}

func adaptiveBootstrapMySQLFactCounts(
	t *testing.T,
	db *gorm.DB,
	req CommitAdaptiveExecutionBootstrapRequest,
) typedRecoveryMySQLFactCounts {
	t.Helper()
	counts := typedRecoveryMySQLFactCounts{}
	identity := "run_id = ? AND journal_run_id = ? AND attempt_id = ?"
	require.NoError(t, db.Model(&runEventPO{}).
		Where(identity+" AND event_type = ?", req.ExecutionRunID, req.JournalRunID,
			req.AttemptID, adaptiveBootstrapAdmissionEventType).
		Count(&counts.AdmissionEvents).Error)
	require.NoError(t, db.Model(&runEventPO{}).
		Where(identity+" AND event_type = ?", req.ExecutionRunID, req.JournalRunID,
			req.AttemptID, adaptiveBootstrapDecisionEventType).
		Count(&counts.DecisionEvents).Error)
	require.NoError(t, db.Model(&checkpointPO{}).
		Where("run_id = ? AND runtime_type = ? AND checkpoint_ns = ?", req.ExecutionRunID,
			adaptiveBootstrapRuntimeType, adaptiveBootstrapCheckpointNS).
		Count(&counts.Checkpoints).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("execution_run_id = ? AND journal_run_id = ? AND attempt_id = ?", req.ExecutionRunID,
			req.JournalRunID, req.AttemptID).
		Count(&counts.Attempts).Error)
	return counts
}
