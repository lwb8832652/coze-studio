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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestJournalSnapshotMigrationFreezesTenantAndAuditBoundaries(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "..", "docker", "atlas", "migrations",
		"20260730000300_agent_journal_snapshots.sql",
	))
	require.NoError(t, err)
	sql := string(raw)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS `agent_journal_snapshots`",
		"CREATE TABLE IF NOT EXISTS `agent_journal_snapshot_fragments`",
		"CREATE TABLE IF NOT EXISTS `agent_journal_snapshot_access_audits`",
		"CREATE TABLE IF NOT EXISTS `agent_journal_snapshot_reservations`",
		"KEY `idx_agent_journal_snapshots_scope` (`space_id`, `thread_id`, `run_id`)",
		"UNIQUE KEY `uk_agent_journal_snapshots_action_revision`",
		"(`journal_run_id`, `attempt_id`, `action_id`, `revision`)",
		"UNIQUE KEY `uk_agent_journal_snapshot_fragment_index` (`snapshot_id`, `fragment_index`)",
		"UNIQUE KEY `uk_agent_journal_snapshot_audit_idempotency`",
		"(`space_id`, `snapshot_id`, `action`, `actor_id`, `idempotency_key`)",
		"`permission_result` varchar(32) NOT NULL",
		"`object_key` varchar(1024) DEFAULT NULL",
		"`summary_json` mediumblob",
		"`summary_hash` char(64) DEFAULT NULL",
		"`fragment_count` int unsigned NOT NULL DEFAULT 0",
		"`action_id` varchar(191) NOT NULL",
		"`source_revision` char(64) DEFAULT NULL",
		"`target_hash` char(64) NOT NULL",
		"`kind` varchar(32) NOT NULL",
		"`metadata_json` mediumblob",
		"`mime_type` varchar(191) DEFAULT NULL",
		"`compression` varchar(32) NOT NULL DEFAULT 'identity'",
		"`expires_at` bigint NOT NULL",
		"`cleanup_state` varchar(32) NOT NULL DEFAULT 'active'",
		"`deleted_at` bigint DEFAULT NULL",
		"CONSTRAINT `chk_agent_journal_snapshots_payload_storage`",
		"CONSTRAINT `chk_agent_journal_snapshots_content_json`",
		"CONSTRAINT `chk_agent_journal_snapshots_summary_json`",
		"CONSTRAINT `chk_agent_journal_snapshots_retention`",
		"AND `summary_json` IS NOT NULL AND `summary_hash` IS NOT NULL)",
		"CONSTRAINT `chk_agent_journal_snapshot_fragment_kind`",
		"CONSTRAINT `chk_agent_journal_snapshot_fragment_metadata`",
		"DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
	} {
		require.Contains(t, sql, fragment)
	}
	require.NotContains(t, sql, "content` text", "audit rows must not contain content")
	require.NotContains(t, sql, "UNIQUE KEY `uk_agent_journal_snapshots_snapshot_id`")
	require.NotContains(t, sql, "utf8mb4_unicode_ci")
}

func TestJournalSnapshotReservationProtectsStagingAndExpiresBeforeCommit(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).
		Update("snapshots_enabled", true).Error)

	now := time.Now().UnixMilli()
	reservation := &entity.JournalSnapshotReservation{
		SnapshotID: "snap-reserved", ReservationToken: "reservation-token-1",
		SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att_100",
		ActionID: "action-read", Revision: 1, EventID: 520,
		IdempotencyKey: "action-read:ready", ContentHash: "hash-1",
		ACLDomain:     "space:10/thread:1",
		StagingPrefix: "journal-snapshots/staging/10/19700101/acl/hash/snap-reserved/reservation-token-1/",
		ExpiresAt:     now + 60_000, CreatedAt: now,
	}
	reserved, err := repo.ReserveJournalSnapshot(
		context.Background(),
		ReserveJournalSnapshotRequest{Reservation: reservation, Now: now},
	)
	require.NoError(t, err)
	require.False(t, reserved.Replayed)
	require.Equal(t, int64(10), reserved.Reservation.JournalRunID)

	protected, err := repo.IsJournalSnapshotObjectProtected(
		context.Background(),
		10,
		reservation.StagingPrefix+"0000-content",
		now,
	)
	require.NoError(t, err)
	require.True(t, protected)

	require.NoError(t, db.Model(&journalSnapshotReservationPO{}).
		Where("snapshot_id = ?", "snap-reserved").
		Update("expires_at", now-1).Error)
	req := repositorySnapshotCreateRequest("snap-reserved", 520, "original")
	setRepositoryFragmentedSnapshot(req.Snapshot)
	req.Snapshot.ObjectKey = reservation.StagingPrefix + "manifest-content"
	req.Fragments = []*entity.JournalSnapshotFragment{{
		FragmentID: "fragment-reserved-1", SnapshotID: "snap-reserved",
		FragmentIndex: 0, Kind: entity.JournalSnapshotFragmentKindDocumentBlock,
		ObjectKey: reservation.StagingPrefix + "0000-content",
	}}
	req.ReservationToken = "reservation-token-1"
	_, _, _, err = repo.CreateJournalSnapshot(context.Background(), req)
	require.ErrorIs(t, err, ErrJournalSnapshotReservationExpired)

	reserved, err = repo.ReserveJournalSnapshot(
		context.Background(),
		ReserveJournalSnapshotRequest{
			Reservation: &entity.JournalSnapshotReservation{
				SnapshotID: "snap-reserved", ReservationToken: "reservation-token-2",
				SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att_100",
				ActionID: "action-read", Revision: 1, EventID: 521,
				IdempotencyKey: "action-read:ready", ContentHash: "hash-1",
				ACLDomain:     "space:10/thread:1",
				StagingPrefix: "journal-snapshots/staging/10/19700101/acl/hash/snap-reserved/reservation-token-2/",
				ExpiresAt:     now + 120_000, CreatedAt: now + 1,
			},
			Now: now,
		},
	)
	require.NoError(t, err)
	require.Equal(t, "reservation-token-2", reserved.Reservation.ReservationToken)

	req = repositorySnapshotCreateRequest("snap-reserved", 521, "original")
	req.Snapshot.ActionID = "action-read"
	req.Event.ActionID = "action-read"
	req.Event.IdempotencyKey = "action-read:ready"
	setRepositoryFragmentedSnapshot(req.Snapshot)
	req.Snapshot.ObjectKey = "journal-snapshots/staging/10/19700101/acl/hash/snap-reserved/reservation-token-2/manifest-content"
	req.Fragments = []*entity.JournalSnapshotFragment{{
		FragmentID: "fragment-reserved-2", SnapshotID: "snap-reserved",
		FragmentIndex: 0, Kind: entity.JournalSnapshotFragmentKindDocumentBlock,
		ObjectKey: "journal-snapshots/staging/10/19700101/acl/hash/snap-reserved/reservation-token-2/0000-content",
	}}
	req.ReservationToken = "reservation-token-2"
	created, _, replayed, err := repo.CreateJournalSnapshot(context.Background(), req)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "snap-reserved", created.SnapshotID)

	protected, err = repo.IsJournalSnapshotObjectProtected(
		context.Background(),
		10,
		reservation.StagingPrefix+"0000-content",
		now,
	)
	require.NoError(t, err)
	require.False(t, protected, "the expired reservation must not protect its old generation")
}

func TestDeleteExpiredJournalSnapshotReservationsIsTenantScopedAndBounded(t *testing.T) {
	t.Parallel()

	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	reservations := []*journalSnapshotReservationPO{
		{SnapshotID: "expired-10-a", ReservationToken: "token-10-a", SpaceID: 10,
			JournalRunID: 10, AttemptID: "attempt-a", ActionID: "action-a", Revision: 1,
			ExpiresAt: 90, CreatedAt: 1},
		{SnapshotID: "expired-10-b", ReservationToken: "token-10-b", SpaceID: 10,
			JournalRunID: 10, AttemptID: "attempt-b", ActionID: "action-b", Revision: 1,
			ExpiresAt: 95, CreatedAt: 2},
		{SnapshotID: "active-10", ReservationToken: "token-10-active", SpaceID: 10,
			JournalRunID: 10, AttemptID: "attempt-c", ActionID: "action-c", Revision: 1,
			ExpiresAt: 101, CreatedAt: 3},
		{SnapshotID: "expired-11", ReservationToken: "token-11", SpaceID: 11,
			JournalRunID: 11, AttemptID: "attempt-d", ActionID: "action-d", Revision: 1,
			ExpiresAt: 80, CreatedAt: 4},
	}
	require.NoError(t, db.Create(&reservations).Error)

	deleted, err := repo.DeleteExpiredJournalSnapshotReservations(
		context.Background(), 10, 100, 1,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	deleted, err = repo.DeleteExpiredJournalSnapshotReservations(
		context.Background(), 10, 100, 10,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	var remaining []journalSnapshotReservationPO
	require.NoError(t, db.Order("snapshot_id").Find(&remaining).Error)
	require.Equal(t, []string{"active-10", "expired-11"}, []string{
		remaining[0].SnapshotID,
		remaining[1].SnapshotID,
	})
}

func TestJournalSnapshotCreateIsAtomicImmutableAndScoped(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).
		Update("snapshots_enabled", true).Error)

	req := repositorySnapshotCreateRequest("snap-1", 501, "original")
	created, event, replayed, err := repo.CreateJournalSnapshot(context.Background(), req)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "snap-1", created.SnapshotID)
	require.Equal(t, uint32(0), created.FragmentCount)
	require.Equal(t, entity.JournalSnapshotCompressionIdentity, created.Compression)
	require.Equal(t, entity.JournalSnapshotCleanupStateActive, created.CleanupState)
	require.Equal(t, uint64(1), event.Sequence)
	require.Equal(t, "snap-1", event.SnapshotID)

	changed := repositorySnapshotCreateRequest("snap-1", 502, "changed")
	replayedSnapshot, replayedEvent, replayed, err := repo.CreateJournalSnapshot(
		context.Background(), changed,
	)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, created.ContentJSON, replayedSnapshot.ContentJSON)
	require.Equal(t, event.ID, replayedEvent.ID)

	conflict := repositorySnapshotCreateRequest("snap-1", 503, "original")
	conflict.Snapshot.ACLDomain = "space:10/thread:other"
	_, _, _, err = repo.CreateJournalSnapshot(context.Background(), conflict)
	require.ErrorIs(t, err, ErrJournalSnapshotConflict)

	loaded, err := repo.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, SnapshotID: "snap-1",
	})
	require.NoError(t, err)
	require.Equal(t, "original", loaded.ContentJSON)
	_, err = repo.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 11, ThreadID: 1, RunID: 10, SnapshotID: "snap-1",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotNotFound)

	var snapshotCount, eventCount int64
	require.NoError(t, db.Model(&journalSnapshotPO{}).Count(&snapshotCount).Error)
	require.NoError(t, db.Model(&runEventPO{}).Where("snapshot_id = ?", "snap-1").
		Count(&eventCount).Error)
	require.Equal(t, int64(1), snapshotCount)
	require.Equal(t, int64(1), eventCount)
}

func TestJournalSnapshotCreateRejectsMismatchedSummaryHash(t *testing.T) {
	req := repositorySnapshotCreateRequest("snap-summary-hash", 521, "metadata")
	setRepositoryFragmentedSnapshot(req.Snapshot)
	req.Snapshot.ObjectKey = "journal-snapshots/staging/10/summary/manifest-content"
	req.Snapshot.SummaryHash = strings.Repeat("f", 64)
	req.Fragments = []*entity.JournalSnapshotFragment{{
		FragmentID: "fragment-summary", SnapshotID: req.Snapshot.SnapshotID,
		FragmentIndex: 0, Kind: entity.JournalSnapshotFragmentKindDocumentBlock,
		InlineContent: "content",
	}}

	_, _, _, err := normalizeJournalSnapshotCreateRequest(req)
	require.ErrorContains(t, err, "summary hash")
}

func TestJournalSnapshotReservationReplaysAfterAttemptBecomesTerminal(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).
		Update("snapshots_enabled", true).Error)

	req := repositorySnapshotCreateRequest("snap-terminal-replay", 520, "original")
	created, event, _, err := repo.CreateJournalSnapshot(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).
		Updates(map[string]any{
			"snapshots_enabled": false,
			"projection_state":  entity.JournalProjectionStateDegraded,
			"status":            entity.RunAttemptStatusCompleted,
		}).Error)

	now := time.Now().UnixMilli()
	replayed, err := repo.ReserveJournalSnapshot(
		context.Background(),
		ReserveJournalSnapshotRequest{
			Reservation: &entity.JournalSnapshotReservation{
				SnapshotID: created.SnapshotID, ReservationToken: "terminal-retry-token",
				SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: created.AttemptID,
				ActionID: created.ActionID, Revision: created.Revision,
				EventID: event.ID, IdempotencyKey: event.IdempotencyKey,
				ContentHash: created.ContentHash, ACLDomain: created.ACLDomain,
				StagingPrefix: "journal-snapshots/staging/10/replay/",
				ExpiresAt:     now + 60_000, CreatedAt: now,
			},
			Now: now,
		},
	)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, created.SnapshotID, replayed.Snapshot.SnapshotID)
	require.Equal(t, event.ID, replayed.Event.ID)
}

func TestJournalSnapshotCreateRollsBackSnapshotAndEventTogether(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).
		Update("snapshots_enabled", true).Error)

	req := repositorySnapshotCreateRequest("snap-rollback", 503, "metadata")
	setRepositoryFragmentedSnapshot(req.Snapshot)
	req.Snapshot.ObjectKey = "journal-snapshots/staging/10/20260730/acl/hash/snap-rollback/token/manifest-content"
	req.Fragments = []*entity.JournalSnapshotFragment{
		{FragmentID: "frag-a", SnapshotID: "snap-rollback", FragmentIndex: 0, Kind: entity.JournalSnapshotFragmentKindDocumentBlock},
		{FragmentID: "frag-b", SnapshotID: "snap-rollback", FragmentIndex: 0, Kind: entity.JournalSnapshotFragmentKindDocumentBlock},
	}
	_, _, _, err := repo.CreateJournalSnapshot(context.Background(), req)
	require.Error(t, err)

	var snapshotCount, eventCount, fragmentCount int64
	require.NoError(t, db.Model(&journalSnapshotPO{}).Count(&snapshotCount).Error)
	require.NoError(t, db.Model(&journalSnapshotFragmentPO{}).Count(&fragmentCount).Error)
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 503).Count(&eventCount).Error)
	require.Zero(t, snapshotCount)
	require.Zero(t, fragmentCount)
	require.Zero(t, eventCount)
}

func TestJournalEventCannotReferenceUncommittedSnapshot(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	event := repositorySnapshotCreateRequest("missing-snapshot", 507, "metadata").Event
	_, err := repo.AppendJournalEvent(context.Background(), event)
	require.ErrorIs(t, err, ErrJournalSnapshotNotFound)

	var eventCount int64
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 507).Count(&eventCount).Error)
	require.Zero(t, eventCount)
}

func TestJournalSnapshotRecoveryReadsByLogicalRun(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusCompleted, 1)
	seedJournalRecoveryRun(t, db, 11, 1, entity.RunStatusRunning)
	recoveryKey := "recover-snapshot"
	sourceAttemptID := "att_100"
	_, err := repo.CreateJournalAttempt(context.Background(), &entity.RunAttempt{
		ID: 101, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 11,
		AttemptID: "att_101", Status: entity.RunAttemptStatusRunning,
		RecoveryIdempotencyKey: &recoveryKey, SourceAttemptID: &sourceAttemptID,
		SnapshotsEnabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).
		Update("snapshots_enabled", true).Error)

	req := repositorySnapshotCreateRequest("snap-recovery", 509, "recovery")
	req.Snapshot.RunID = 11
	req.Snapshot.AttemptID = "att_101"
	req.Event.RunID = 11
	req.Event.AttemptID = "att_101"
	created, _, _, err := repo.CreateJournalSnapshot(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, int64(11), created.RunID)
	require.Equal(t, int64(10), created.JournalRunID)

	loaded, err := repo.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, SnapshotID: "snap-recovery",
	})
	require.NoError(t, err)
	require.Equal(t, int64(11), loaded.RunID)
	require.Equal(t, int64(10), loaded.JournalRunID)
	_, err = repo.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 11, SnapshotID: "snap-recovery",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotNotFound)
}

func TestJournalSnapshotAndDirectAppendRejectInactiveProjection(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).
		Updates(map[string]any{
			"snapshots_enabled": true,
			"projection_state":  entity.JournalProjectionStateDegraded,
		}).Error)

	_, _, _, err := repo.CreateJournalSnapshot(
		context.Background(),
		repositorySnapshotCreateRequest("snap-degraded", 510, "metadata"),
	)
	require.ErrorIs(t, err, ErrJournalProjectionInactive)

	event := repositorySnapshotCreateRequest("unused", 511, "metadata").Event
	event.SnapshotID = ""
	_, err = repo.AppendJournalEvent(context.Background(), event)
	require.ErrorIs(t, err, ErrJournalProjectionInactive)

	var eventCount, snapshotCount int64
	require.NoError(t, db.Model(&runEventPO{}).
		Where("id IN ?", []int64{510, 511}).Count(&eventCount).Error)
	require.NoError(t, db.Model(&journalSnapshotPO{}).Count(&snapshotCount).Error)
	require.Zero(t, eventCount)
	require.Zero(t, snapshotCount)
}

func TestJournalSnapshotFragmentsPageAndCommittedObjectLookupAreTenantScoped(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).
		Update("snapshots_enabled", true).Error)
	req := repositorySnapshotCreateRequest("snap-fragments", 504, "metadata")
	now := time.Now().UnixMilli()
	stagingPrefix := "journal-snapshots/staging/10/20260730/acl/hash/snap-fragments/reservation-token/"
	_, err := repo.ReserveJournalSnapshot(context.Background(), ReserveJournalSnapshotRequest{
		Reservation: &entity.JournalSnapshotReservation{
			SnapshotID: "snap-fragments", ReservationToken: "reservation-token",
			SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att_100",
			ActionID: req.Snapshot.ActionID, Revision: req.Snapshot.Revision,
			EventID: req.Snapshot.EventID, IdempotencyKey: req.Event.IdempotencyKey,
			ContentHash: req.Snapshot.ContentHash, ACLDomain: req.Snapshot.ACLDomain,
			StagingPrefix: stagingPrefix, ExpiresAt: now + 60_000, CreatedAt: now,
		},
		Now: now,
	})
	require.NoError(t, err)
	req.ReservationToken = "reservation-token"
	setRepositoryFragmentedSnapshot(req.Snapshot)
	req.Snapshot.ObjectKey = stagingPrefix + "manifest-content"
	const metadataJSON = `{ "stream": "stdout", "block_id": "block-0" }`
	req.Fragments = []*entity.JournalSnapshotFragment{
		{FragmentID: "frag-0", SnapshotID: "snap-fragments", FragmentIndex: 0, Kind: entity.JournalSnapshotFragmentKindDocumentBlock, MetadataJSON: metadataJSON, MIMEType: "text/markdown", InlineContent: "one"},
		{FragmentID: "frag-1", SnapshotID: "snap-fragments", FragmentIndex: 1, Kind: entity.JournalSnapshotFragmentKindDocumentBlock, ObjectKey: stagingPrefix + "0001-content"},
		{FragmentID: "frag-2", SnapshotID: "snap-fragments", FragmentIndex: 2, Kind: entity.JournalSnapshotFragmentKindDocumentBlock, InlineContent: "three"},
	}
	_, _, _, err = repo.CreateJournalSnapshot(context.Background(), req)
	require.NoError(t, err)

	page, err := repo.ListJournalSnapshotFragments(context.Background(), ListJournalSnapshotFragmentsRequest{
		SpaceID: 10, SnapshotID: "snap-fragments", AfterIndex: -1, Limit: 2,
	})
	require.NoError(t, err)
	require.Len(t, page.Fragments, 2)
	require.True(t, page.HasMore)
	require.Equal(t, int32(1), page.NextIndex)
	require.Equal(t, entity.JournalSnapshotFragmentKindDocumentBlock, page.Fragments[0].Kind)
	require.Equal(t, metadataJSON, page.Fragments[0].MetadataJSON)
	require.Equal(t, "text/markdown", page.Fragments[0].MIMEType)
	stored, err := repo.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, SnapshotID: "snap-fragments",
	})
	require.NoError(t, err)
	require.Equal(t, uint32(3), stored.FragmentCount)
	require.Empty(t, stored.ContentJSON)
	require.Equal(t, stagingPrefix+"manifest-content", stored.ObjectKey)
	require.Equal(t, req.Snapshot.SummaryJSON, stored.SummaryJSON)
	require.Equal(t, req.Snapshot.SummaryHash, stored.SummaryHash)

	committed, err := repo.IsJournalSnapshotObjectProtected(
		context.Background(), 10, stagingPrefix+"0001-content", time.Now().UnixMilli(),
	)
	require.NoError(t, err)
	require.True(t, committed)
	committed, err = repo.IsJournalSnapshotObjectProtected(
		context.Background(), 11, stagingPrefix+"0001-content", time.Now().UnixMilli(),
	)
	require.NoError(t, err)
	require.False(t, committed)
}

func TestJournalSnapshotAccessAuditIsContentFreeAndIdempotent(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	audit := &entity.JournalSnapshotAccessAudit{
		SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att_100",
		SnapshotID: "snap-1", ContentType: entity.JournalSnapshotContentTypeDocument,
		Action: entity.JournalSnapshotActionOpenOriginal, ActorID: 20,
		PermissionResult: entity.JournalSnapshotPermissionAllowed,
		IdempotencyKey:   "action-1", TargetHash: strings.Repeat("a", 64),
		TraceID: "trace-1", CreatedAt: 123,
	}
	created, replayed, err := repo.RecordJournalSnapshotAccess(context.Background(), audit)
	require.NoError(t, err)
	require.False(t, replayed)
	replayedAudit, replayed, err := repo.RecordJournalSnapshotAccess(context.Background(), audit)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, created.ID, replayedAudit.ID)

	otherActor := *audit
	otherActor.ActorID = 21
	otherActorAudit, replayed, err := repo.RecordJournalSnapshotAccess(
		context.Background(),
		&otherActor,
	)
	require.NoError(t, err)
	require.False(t, replayed)
	require.NotEqual(t, created.ID, otherActorAudit.ID)

	var rows []journalSnapshotAccessAuditPO
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 2)
	require.NotContains(t, rows[0].IdempotencyKey, "content")
	require.Empty(t, rows[0].ObjectKey)
}

func TestJournalSnapshotReadsRejectDeletedOrCleaningRows(t *testing.T) {
	db := newJournalSnapshotRepositoryTestDB(t)
	repo := NewThreadRepository(db)
	seedJournalRun(t, db, 10, 1)
	seedJournalAttempt(t, db, 100, 10, entity.RunAttemptStatusRunning, 1)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).
		Update("snapshots_enabled", true).Error)

	_, _, _, err := repo.CreateJournalSnapshot(
		context.Background(),
		repositorySnapshotCreateRequest("snap-deleted", 508, "metadata"),
	)
	require.NoError(t, err)
	require.NoError(t, db.Model(&journalSnapshotPO{}).
		Where("snapshot_id = ?", "snap-deleted").
		Updates(map[string]any{
			"cleanup_state": entity.JournalSnapshotCleanupStatePending,
			"deleted_at":    int64(1_000),
		}).Error)

	_, err = repo.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, SnapshotID: "snap-deleted",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotNotFound)
	_, err = repo.ListJournalSnapshotFragments(
		context.Background(),
		ListJournalSnapshotFragmentsRequest{
			SpaceID: 10, SnapshotID: "snap-deleted", AfterIndex: -1, Limit: 10,
		},
	)
	require.ErrorIs(t, err, ErrJournalSnapshotNotFound)
}

func repositorySnapshotCreateRequest(
	snapshotID string,
	eventID int64,
	content string,
) CreateJournalSnapshotRequest {
	return CreateJournalSnapshotRequest{
		Snapshot: &entity.JournalContentSnapshot{
			SnapshotID: snapshotID, SpaceID: 10, ThreadID: 1, RunID: 10,
			JournalRunID: 10, AttemptID: "att_100", EventID: eventID,
			ActionID:    "action-" + snapshotID,
			Revision:    1,
			ContentType: entity.JournalSnapshotContentTypeDocument,
			Status:      entity.JournalContentStatusReady,
			Visibility:  entity.JournalVisibilityUser, MIMEType: "text/markdown",
			Encoding: "utf-8", ContentJSON: content, ContentHash: "hash-1",
			ACLDomain: "space:10/thread:1",
			ExpiresAt: 100 + entity.JournalSnapshotRetentionMillis, CreatedAt: 100,
		},
		Event: &entity.JournalEvent{
			ID: eventID, ThreadID: 1, RunID: 10, JournalRunID: 10,
			AttemptID: "att_100", IdempotencyKey: "snapshot:" + snapshotID,
			SchemaVersion: entity.JournalSchemaVersion, Status: "completed",
			OccurredAtUnixNano: 100_000_000, Visibility: entity.JournalVisibilityUser,
			PayloadVersion: entity.JournalPayloadVersion, SnapshotID: snapshotID,
			ActionID: "action-" + snapshotID, Phase: "terminal",
			Operation: "read", Target: "document",
			EventType: "action.terminal", Payload: `{"type":"document","data":{}}`,
			CreatedAt: 100,
		},
	}
}

func setRepositoryFragmentedSnapshot(snapshot *entity.JournalContentSnapshot) {
	const summary = `{"document":{"title":"Review"}}`
	digest := sha256.Sum256([]byte(summary))
	snapshot.IsFragmented = true
	snapshot.ContentJSON = ""
	snapshot.SummaryJSON = summary
	snapshot.SummaryHash = hex.EncodeToString(digest[:])
}

func newJournalSnapshotRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newJournalRepositoryTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&journalSnapshotPO{},
		&journalSnapshotReservationPO{},
		&journalSnapshotFragmentPO{},
		&journalSnapshotAccessAuditPO{},
	))
	return db
}
