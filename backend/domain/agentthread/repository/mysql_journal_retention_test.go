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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestJournalRetentionSnapshotClaimProtectsActiveAttemptAndRetriesByLease(t *testing.T) {
	db := journalRetentionRepositoryTestDB(t)
	repo := &threadRepository{db: db}
	activeSlot := uint8(1)
	terminalEndedAt := int64(8_000)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 1, ThreadID: 1, JournalRunID: 101, ExecutionRunID: 101, AttemptID: "att-terminal",
		Ordinal: 1, Status: string(entity.RunAttemptStatusCompleted), NextSequence: 2,
		LastCommittedSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: string(entity.JournalProjectionStateHealthy), CreatedAt: 1_000,
		UpdatedAt: 8_000, EndedAt: &terminalEndedAt,
	}).Error)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 2, ThreadID: 2, JournalRunID: 202, ExecutionRunID: 202, AttemptID: "att-active",
		Ordinal: 1, Status: string(entity.RunAttemptStatusRunning), ActiveSlot: &activeSlot,
		NextSequence: 2, LastCommittedSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: string(entity.JournalProjectionStateHealthy), CreatedAt: 1_000, UpdatedAt: 8_000,
	}).Error)
	terminalObject := "journal-snapshots/staging/10/snap-terminal/content"
	terminalFragment := "journal-snapshots/staging/10/snap-terminal/fragment"
	activeObject := "journal-snapshots/staging/10/snap-active/content"
	for _, snapshot := range []*journalSnapshotPO{
		journalRetentionSnapshotPO("snap-terminal", 1, 101, "att-terminal", terminalObject),
		journalRetentionSnapshotPO("snap-active", 2, 202, "att-active", activeObject),
	} {
		require.NoError(t, db.Create(snapshot).Error)
	}
	require.NoError(t, db.Create(&journalSnapshotFragmentPO{
		FragmentID: "fragment-terminal", SnapshotID: "snap-terminal", FragmentIndex: 0,
		Kind: string(entity.JournalSnapshotFragmentKindDocumentBlock), ObjectKey: &terminalFragment,
		SizeBytes: 10, ContentHash: strings.Repeat("b", 64), CreatedAt: 1_000,
	}).Error)
	backlog, err := repo.CountJournalRetentionBacklog(context.Background(), JournalRetentionBacklogRequest{
		SnapshotCutoffAt: 9_000, ExecutionCutoffAt: 9_000, StagingCutoffAt: 9_000,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), backlog)

	claims, err := repo.ClaimJournalSnapshotCleanup(context.Background(), JournalRetentionClaimRequest{
		CutoffAt: 9_000, Now: 10_000, LeaseUntil: 11_000, BatchSize: 10,
	})
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, int64(10), claims[0].SpaceID)
	require.Equal(t, "snap-terminal", claims[0].SnapshotID)
	require.NotEmpty(t, claims[0].ClaimToken)
	require.ElementsMatch(t, []string{terminalObject, terminalFragment}, claims[0].ObjectKeys)

	claimsWhileLeased, err := repo.ClaimJournalSnapshotCleanup(context.Background(), JournalRetentionClaimRequest{
		CutoffAt: 9_000, Now: 10_500, LeaseUntil: 11_500, BatchSize: 10,
	})
	require.NoError(t, err)
	require.Empty(t, claimsWhileLeased)

	require.NoError(t, repo.ReleaseJournalSnapshotCleanup(
		context.Background(), claims[0], "object_delete_failed",
	))
	retryClaims, err := repo.ClaimJournalSnapshotCleanup(context.Background(), JournalRetentionClaimRequest{
		CutoffAt: 9_000, Now: 12_000, LeaseUntil: 13_000, BatchSize: 10,
	})
	require.NoError(t, err)
	require.Len(t, retryClaims, 1)
	require.NotEqual(t, claims[0].ClaimToken, retryClaims[0].ClaimToken)
	require.NoError(t, repo.CompleteJournalSnapshotCleanup(context.Background(), retryClaims[0]))

	var terminalCount, activeCount int64
	require.NoError(t, db.Model(&journalSnapshotPO{}).Where("snapshot_id = ?", "snap-terminal").Count(&terminalCount).Error)
	require.NoError(t, db.Model(&journalSnapshotPO{}).Where("snapshot_id = ?", "snap-active").Count(&activeCount).Error)
	require.Zero(t, terminalCount)
	require.Equal(t, int64(1), activeCount)
}

func TestJournalRetentionStagingClaimHonorsGraceAndIsIdempotent(t *testing.T) {
	db := journalRetentionRepositoryTestDB(t)
	repo := &threadRepository{db: db}
	for _, reservation := range []*journalSnapshotReservationPO{
		journalRetentionReservationPO("expired", 8_000),
		journalRetentionReservationPO("inside-grace", 9_500),
	} {
		require.NoError(t, db.Create(reservation).Error)
	}

	claims, err := repo.ClaimJournalStagingCleanup(context.Background(), JournalRetentionClaimRequest{
		CutoffAt: 9_000, Now: 10_000, LeaseUntil: 11_000, BatchSize: 10,
	})
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, "expired", claims[0].SnapshotID)
	require.Equal(t, int64(10), claims[0].SpaceID)
	require.Equal(t, "journal-snapshots/staging/10/expired/", claims[0].Prefix)
	require.NoError(t, repo.CompleteJournalStagingCleanup(context.Background(), claims[0]))
	require.NoError(t, repo.CompleteJournalStagingCleanup(context.Background(), claims[0]))

	var remaining []journalSnapshotReservationPO
	require.NoError(t, db.Order("snapshot_id").Find(&remaining).Error)
	require.Len(t, remaining, 1)
	require.Equal(t, "inside-grace", remaining[0].SnapshotID)
}

func TestJournalRetentionDeletesTerminalMetadataInBatchesAndNeverActiveAttempts(t *testing.T) {
	db := journalRetentionRepositoryTestDB(t)
	repo := &threadRepository{db: db}
	activeSlot := uint8(1)
	endedAt := int64(8_000)
	for _, run := range []*runPO{
		journalRetentionRunPO(101, 1, entity.RunStatusSucceeded),
		journalRetentionRunPO(202, 2, entity.RunStatusRunning),
	} {
		require.NoError(t, db.Create(run).Error)
	}
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 1, ThreadID: 1, JournalRunID: 101, ExecutionRunID: 101, AttemptID: "att-terminal",
		Ordinal: 1, Status: string(entity.RunAttemptStatusCompleted), NextSequence: 2,
		LastCommittedSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: string(entity.JournalProjectionStateHealthy), CreatedAt: 1_000,
		UpdatedAt: 8_000, EndedAt: &endedAt,
	}).Error)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 2, ThreadID: 2, JournalRunID: 202, ExecutionRunID: 202, AttemptID: "att-active",
		Ordinal: 1, Status: string(entity.RunAttemptStatusRunning), ActiveSlot: &activeSlot,
		NextSequence: 2, LastCommittedSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: string(entity.JournalProjectionStateHealthy), CreatedAt: 1_000, UpdatedAt: 8_000,
	}).Error)
	terminalJournalRunID := int64(101)
	activeJournalRunID := int64(202)
	terminalAttemptID := "att-terminal"
	activeAttemptID := "att-active"
	for _, event := range []*runEventPO{
		{ID: 11, ThreadID: 1, RunID: 101, JournalRunID: &terminalJournalRunID, AttemptID: &terminalAttemptID, Sequence: uint64Pointer(1), IdempotencyKey: stringPointer("event-terminal"), EventType: "run.completed", Payload: []byte(`{}`), CreatedAt: 1_000},
		{ID: 22, ThreadID: 2, RunID: 202, JournalRunID: &activeJournalRunID, AttemptID: &activeAttemptID, Sequence: uint64Pointer(1), IdempotencyKey: stringPointer("event-active"), EventType: "run.started", Payload: []byte(`{}`), CreatedAt: 1_000},
	} {
		require.NoError(t, db.Create(event).Error)
	}
	for _, ledger := range []*sideEffectLedgerPO{
		journalRetentionLedgerPO(31, 1, 101, "att-terminal"),
		journalRetentionLedgerPO(32, 2, 202, "att-active"),
	} {
		require.NoError(t, db.Create(ledger).Error)
	}
	for _, checkpoint := range []*checkpointPO{
		journalRetentionCheckpointPO(41, 1, 101),
		journalRetentionCheckpointPO(42, 2, 202),
	} {
		require.NoError(t, db.Create(checkpoint).Error)
	}

	deletedCheckpoints, err := repo.DeleteExpiredJournalCheckpoints(context.Background(), JournalRetentionDeleteRequest{CutoffAt: 9_000, BatchSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), deletedCheckpoints)
	execution, err := repo.DeleteExpiredJournalEventsAndLedgers(context.Background(), JournalRetentionDeleteRequest{CutoffAt: 9_000, BatchSize: 10})
	require.NoError(t, err)
	require.Equal(t, JournalExecutionCleanupResult{EventsDeleted: 1, LedgersDeleted: 1}, execution)
	deletedAttempts, err := repo.DeleteUnreferencedJournalAttempts(context.Background(), JournalRetentionDeleteRequest{CutoffAt: 9_000, BatchSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), deletedAttempts)

	for _, model := range []any{&checkpointPO{}, &runEventPO{}, &sideEffectLedgerPO{}, &runAttemptPO{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Equal(t, int64(1), count)
	}
	deletedCheckpoints, err = repo.DeleteExpiredJournalCheckpoints(context.Background(), JournalRetentionDeleteRequest{CutoffAt: 9_000, BatchSize: 10})
	require.NoError(t, err)
	require.Zero(t, deletedCheckpoints)
}

func journalRetentionRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return canonicalRepositoryTestDB(t,
		&threadPO{}, &runPO{}, &runAttemptPO{}, &runEventPO{}, &checkpointPO{},
		&sideEffectLedgerPO{}, &journalSnapshotPO{}, &journalSnapshotFragmentPO{},
		&journalSnapshotReservationPO{},
	)
}

func journalRetentionSnapshotPO(snapshotID string, threadID, journalRunID int64, attemptID, objectKey string) *journalSnapshotPO {
	return &journalSnapshotPO{
		SnapshotID: snapshotID, SpaceID: 10, ThreadID: threadID, RunID: journalRunID,
		JournalRunID: journalRunID, AttemptID: attemptID, EventID: journalRunID,
		ActionID: "action-" + snapshotID, Revision: 1,
		ContentType: string(entity.JournalSnapshotContentTypeDocument), Status: string(entity.JournalContentStatusReady),
		Visibility: string(entity.JournalVisibilityUser), MIMEType: "text/markdown", Encoding: "utf-8",
		Compression: "identity", ObjectKey: &objectKey, ContentLength: 10,
		ContentHash: strings.Repeat("a", 64), ACLDomain: "space:10/thread:1",
		ExpiresAt: 8_000, CleanupState: string(entity.JournalSnapshotCleanupStateActive), CreatedAt: 1_000,
	}
}

func journalRetentionReservationPO(snapshotID string, expiresAt int64) *journalSnapshotReservationPO {
	return &journalSnapshotReservationPO{
		SnapshotID: snapshotID, ReservationToken: "token-" + snapshotID,
		SpaceID: 10, ThreadID: 1, RunID: 101, JournalRunID: 101, AttemptID: "att-terminal",
		ActionID: "action-" + snapshotID, Revision: 1, EventID: 101,
		IdempotencyKey: "idem-" + snapshotID, ContentHash: strings.Repeat("a", 64),
		ACLDomain: "space:10/thread:1", StagingPrefix: "journal-snapshots/staging/10/" + snapshotID + "/",
		ExpiresAt: expiresAt, CreatedAt: 1_000,
	}
}

func journalRetentionRunPO(id, threadID int64, status entity.RunStatus) *runPO {
	return &runPO{
		ID: id, ThreadID: threadID, SpaceID: 10, CreatorID: 20, AssistantID: "assistant",
		RunKind: string(entity.RunKindTask), Status: string(status), Command: []byte(`{}`),
		Input: []byte(`{}`), Config: []byte(`{}`), Context: []byte(`{}`), Metadata: []byte(`{}`),
		StreamMode: []byte(`[]`), CreatedAt: 1_000, UpdatedAt: 8_000,
	}
}

func journalRetentionLedgerPO(id, threadID, journalRunID int64, attemptID string) *sideEffectLedgerPO {
	return &sideEffectLedgerPO{
		ID: id, ThreadID: threadID, JournalRunID: journalRunID, AttemptID: attemptID,
		IdempotencyKey: "ledger-" + attemptID, ActionKind: "read_file", ReplayPolicy: "read_only",
		Status: "succeeded", RequestHash: strings.Repeat("a", 64), Version: 1,
		PreparedAt: 1_000, CreatedAt: 1_000, UpdatedAt: 1_000,
	}
}

func journalRetentionCheckpointPO(id, threadID, runID int64) *checkpointPO {
	return &checkpointPO{
		ID: id, ThreadID: threadID, RunID: runID, RuntimeType: "eino_adk", RuntimeKey: "runtime",
		ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`),
		Metadata: []byte(`{}`), CreatedAt: 1_000,
	}
}

func uint64Pointer(value uint64) *uint64 { return &value }

func stringPointer(value string) *string { return &value }
