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
	"fmt"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const journalRetentionActiveAttemptSQL = `NOT EXISTS (
SELECT 1 FROM agent_run_attempts active_attempt
WHERE active_attempt.journal_run_id = agent_journal_snapshots.journal_run_id
  AND active_attempt.status IN ('pending', 'running')
)`

func lockActiveJournalAttemptsForThread(tx *gorm.DB, threadID int64) ([]runAttemptPO, error) {
	query := tx.Where("thread_id = ? AND status IN (?, ?)",
		threadID, entity.RunAttemptStatusPending, entity.RunAttemptStatusRunning).
		Order("id ASC")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempts []runAttemptPO
	if err := query.Find(&attempts).Error; err != nil {
		return nil, err
	}
	return attempts, nil
}

func (r *threadRepository) ClaimJournalSnapshotCleanup(
	ctx context.Context,
	req JournalRetentionClaimRequest,
) ([]JournalSnapshotCleanupClaim, error) {
	if err := validateJournalRetentionClaimRequest(req); err != nil {
		return nil, err
	}
	claims := make([]JournalSnapshotCleanupClaim, 0)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&journalSnapshotPO{}).
			Where(`(
  (cleanup_state = ? AND created_at <= ?)
  OR cleanup_state IN (?, ?)
  OR (cleanup_state = ? AND (cleanup_claim_expires_at IS NULL OR cleanup_claim_expires_at <= ?))
)`,
				entity.JournalSnapshotCleanupStateActive, req.CutoffAt,
				entity.JournalSnapshotCleanupStatePending, entity.JournalSnapshotCleanupStateFailed,
				entity.JournalSnapshotCleanupStateDeleting, req.Now,
			).
			Where(journalRetentionActiveAttemptSQL).
			Order("created_at ASC, snapshot_id ASC").
			Limit(req.BatchSize)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		var rows []journalSnapshotPO
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			token, err := newRunLeaseToken()
			if err != nil {
				return err
			}
			result := tx.Model(&journalSnapshotPO{}).
				Where("snapshot_id = ?", rows[i].SnapshotID).
				Updates(map[string]any{
					"cleanup_state":            entity.JournalSnapshotCleanupStateDeleting,
					"deleted_at":               gorm.Expr("COALESCE(deleted_at, ?)", req.Now),
					"cleanup_claim_token":      token,
					"cleanup_claim_expires_at": req.LeaseUntil,
					"cleanup_attempt_count":    gorm.Expr("cleanup_attempt_count + 1"),
					"cleanup_last_error_code":  nil,
					"content_json":             nil,
					"summary_json":             nil,
					"summary_hash":             nil,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			if err := tx.Where("snapshot_id = ? AND object_key IS NULL", rows[i].SnapshotID).
				Delete(&journalSnapshotFragmentPO{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&journalSnapshotFragmentPO{}).
				Where("snapshot_id = ?", rows[i].SnapshotID).
				Updates(map[string]any{"metadata_json": nil, "inline_content": nil}).Error; err != nil {
				return err
			}
			objectKeys, err := journalSnapshotCleanupObjectKeys(tx, rows[i])
			if err != nil {
				return err
			}
			claims = append(claims, JournalSnapshotCleanupClaim{
				SpaceID: rows[i].SpaceID, SnapshotID: rows[i].SnapshotID,
				ClaimToken: token, ObjectKeys: objectKeys,
			})
		}
		return nil
	})
	return claims, err
}

func (r *threadRepository) CompleteJournalSnapshotCleanup(
	ctx context.Context,
	claim JournalSnapshotCleanupClaim,
) error {
	if err := validateJournalSnapshotCleanupClaim(claim); err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&journalSnapshotPO{}).
			Where("snapshot_id = ?", claim.SnapshotID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		var owned int64
		if err := tx.Model(&journalSnapshotPO{}).
			Where("snapshot_id = ? AND cleanup_state = ? AND cleanup_claim_token = ?",
				claim.SnapshotID, entity.JournalSnapshotCleanupStateDeleting, claim.ClaimToken).
			Count(&owned).Error; err != nil {
			return err
		}
		if owned == 0 {
			return ErrJournalRetentionClaimLost
		}
		if err := tx.Where("snapshot_id = ?", claim.SnapshotID).
			Delete(&journalSnapshotFragmentPO{}).Error; err != nil {
			return err
		}
		result := tx.Where(
			"snapshot_id = ? AND cleanup_state = ? AND cleanup_claim_token = ?",
			claim.SnapshotID, entity.JournalSnapshotCleanupStateDeleting, claim.ClaimToken,
		).Delete(&journalSnapshotPO{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrJournalRetentionClaimLost
		}
		return nil
	})
}

func (r *threadRepository) ReleaseJournalSnapshotCleanup(
	ctx context.Context,
	claim JournalSnapshotCleanupClaim,
	errorCode string,
) error {
	if err := validateJournalSnapshotCleanupClaim(claim); err != nil {
		return err
	}
	errorCode = normalizeJournalRetentionErrorCode(errorCode)
	result := r.db.WithContext(ctx).Model(&journalSnapshotPO{}).
		Where("snapshot_id = ? AND cleanup_state = ? AND cleanup_claim_token = ?",
			claim.SnapshotID, entity.JournalSnapshotCleanupStateDeleting, claim.ClaimToken).
		Updates(map[string]any{
			"cleanup_state":            entity.JournalSnapshotCleanupStateFailed,
			"cleanup_claim_token":      nil,
			"cleanup_claim_expires_at": nil,
			"cleanup_last_error_code":  errorCode,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).Model(&journalSnapshotPO{}).
			Where("snapshot_id = ?", claim.SnapshotID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrJournalRetentionClaimLost
		}
	}
	return nil
}

func (r *threadRepository) ClaimJournalStagingCleanup(
	ctx context.Context,
	req JournalRetentionClaimRequest,
) ([]JournalStagingCleanupClaim, error) {
	if err := validateJournalRetentionClaimRequest(req); err != nil {
		return nil, err
	}
	claims := make([]JournalStagingCleanupClaim, 0)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&journalSnapshotReservationPO{}).
			Where("expires_at <= ?", req.CutoffAt).
			Where("cleanup_claim_token IS NULL OR cleanup_claim_expires_at IS NULL OR cleanup_claim_expires_at <= ?", req.Now).
			Order("expires_at ASC, snapshot_id ASC").Limit(req.BatchSize)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		var rows []journalSnapshotReservationPO
		if err := query.Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			token, err := newRunLeaseToken()
			if err != nil {
				return err
			}
			result := tx.Model(&journalSnapshotReservationPO{}).
				Where("snapshot_id = ?", rows[i].SnapshotID).
				Updates(map[string]any{
					"cleanup_claim_token":      token,
					"cleanup_claim_expires_at": req.LeaseUntil,
					"cleanup_attempt_count":    gorm.Expr("cleanup_attempt_count + 1"),
					"cleanup_last_error_code":  nil,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				claims = append(claims, JournalStagingCleanupClaim{
					SpaceID: rows[i].SpaceID, SnapshotID: rows[i].SnapshotID,
					ClaimToken: token, Prefix: rows[i].StagingPrefix,
				})
			}
		}
		return nil
	})
	return claims, err
}

func (r *threadRepository) CompleteJournalStagingCleanup(
	ctx context.Context,
	claim JournalStagingCleanupClaim,
) error {
	if err := validateJournalStagingCleanupClaim(claim); err != nil {
		return err
	}
	result := r.db.WithContext(ctx).Where(
		"snapshot_id = ? AND cleanup_claim_token = ?", claim.SnapshotID, claim.ClaimToken,
	).Delete(&journalSnapshotReservationPO{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).Model(&journalSnapshotReservationPO{}).
			Where("snapshot_id = ?", claim.SnapshotID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrJournalRetentionClaimLost
		}
	}
	return nil
}

func (r *threadRepository) ReleaseJournalStagingCleanup(
	ctx context.Context,
	claim JournalStagingCleanupClaim,
	errorCode string,
) error {
	if err := validateJournalStagingCleanupClaim(claim); err != nil {
		return err
	}
	result := r.db.WithContext(ctx).Model(&journalSnapshotReservationPO{}).
		Where("snapshot_id = ? AND cleanup_claim_token = ?", claim.SnapshotID, claim.ClaimToken).
		Updates(map[string]any{
			"cleanup_claim_token":      nil,
			"cleanup_claim_expires_at": nil,
			"cleanup_last_error_code":  normalizeJournalRetentionErrorCode(errorCode),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := r.db.WithContext(ctx).Model(&journalSnapshotReservationPO{}).
			Where("snapshot_id = ?", claim.SnapshotID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrJournalRetentionClaimLost
		}
	}
	return nil
}

func (r *threadRepository) DeleteExpiredJournalCheckpoints(
	ctx context.Context,
	req JournalRetentionDeleteRequest,
) (int64, error) {
	if err := validateJournalRetentionDeleteRequest(req); err != nil {
		return 0, err
	}
	var ids []int64
	err := r.db.WithContext(ctx).Model(&checkpointPO{}).
		Where("created_at <= ?", req.CutoffAt).
		Where(`NOT EXISTS (
SELECT 1 FROM agent_runs active_run
WHERE active_run.id = agent_checkpoints.run_id
  AND active_run.status IN ('pending', 'queued', 'running', 'interrupted')
)`).
		Where(`NOT EXISTS (
SELECT 1 FROM agent_run_attempts active_attempt
WHERE (active_attempt.execution_run_id = agent_checkpoints.run_id
    OR active_attempt.journal_run_id = agent_checkpoints.run_id)
  AND active_attempt.status IN ('pending', 'running')
)`).
		Order("created_at ASC, id ASC").Limit(req.BatchSize).Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	result := r.db.WithContext(ctx).Where("id IN ? AND created_at <= ?", ids, req.CutoffAt).
		Delete(&checkpointPO{})
	return result.RowsAffected, result.Error
}

func (r *threadRepository) DeleteExpiredJournalEventsAndLedgers(
	ctx context.Context,
	req JournalRetentionDeleteRequest,
) (JournalExecutionCleanupResult, error) {
	result := JournalExecutionCleanupResult{}
	if err := validateJournalRetentionDeleteRequest(req); err != nil {
		return result, err
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var eventIDs []int64
		if err := tx.Model(&runEventPO{}).
			Where("attempt_id IS NOT NULL AND created_at <= ?", req.CutoffAt).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_run_attempts active_attempt
WHERE active_attempt.journal_run_id = agent_run_events.journal_run_id
  AND active_attempt.status IN ('pending', 'running')
)`).
			Order("created_at ASC, id ASC").Limit(req.BatchSize).Pluck("id", &eventIDs).Error; err != nil {
			return err
		}
		if len(eventIDs) > 0 {
			deleted := tx.Where("id IN ? AND created_at <= ?", eventIDs, req.CutoffAt).Delete(&runEventPO{})
			if deleted.Error != nil {
				return deleted.Error
			}
			result.EventsDeleted = deleted.RowsAffected
		}

		var ledgerIDs []int64
		if err := tx.Model(&sideEffectLedgerPO{}).
			Where("created_at <= ?", req.CutoffAt).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_run_attempts active_attempt
WHERE active_attempt.journal_run_id = agent_side_effect_ledger.journal_run_id
  AND active_attempt.status IN ('pending', 'running')
)`).
			Order("created_at ASC, id ASC").Limit(req.BatchSize).Pluck("id", &ledgerIDs).Error; err != nil {
			return err
		}
		if len(ledgerIDs) > 0 {
			deleted := tx.Where("id IN ? AND created_at <= ?", ledgerIDs, req.CutoffAt).
				Delete(&sideEffectLedgerPO{})
			if deleted.Error != nil {
				return deleted.Error
			}
			result.LedgersDeleted = deleted.RowsAffected
		}
		return nil
	})
	return result, err
}

func (r *threadRepository) DeleteUnreferencedJournalAttempts(
	ctx context.Context,
	req JournalRetentionDeleteRequest,
) (int64, error) {
	if err := validateJournalRetentionDeleteRequest(req); err != nil {
		return 0, err
	}
	var ids []int64
	err := r.db.WithContext(ctx).Model(&runAttemptPO{}).
		Where("status NOT IN (?, ?) AND COALESCE(ended_at, updated_at, created_at) <= ?",
			entity.RunAttemptStatusPending, entity.RunAttemptStatusRunning, req.CutoffAt).
		Where(`NOT EXISTS (
SELECT 1 FROM agent_run_events event
WHERE event.journal_run_id = agent_run_attempts.journal_run_id
  AND event.attempt_id = agent_run_attempts.attempt_id
)`).
		Where(`NOT EXISTS (
SELECT 1 FROM agent_side_effect_ledger ledger
WHERE ledger.journal_run_id = agent_run_attempts.journal_run_id
  AND ledger.attempt_id = agent_run_attempts.attempt_id
)`).
		Where(`NOT EXISTS (
SELECT 1 FROM agent_journal_snapshots snapshot
WHERE snapshot.journal_run_id = agent_run_attempts.journal_run_id
  AND snapshot.attempt_id = agent_run_attempts.attempt_id
)`).
		Order("COALESCE(ended_at, updated_at, created_at) ASC, id ASC").
		Limit(req.BatchSize).Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	result := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&runAttemptPO{})
	return result.RowsAffected, result.Error
}

func (r *threadRepository) CountJournalRetentionBacklog(
	ctx context.Context,
	req JournalRetentionBacklogRequest,
) (int64, error) {
	if req.SnapshotCutoffAt <= 0 || req.ExecutionCutoffAt <= 0 || req.StagingCutoffAt <= 0 {
		return 0, fmt.Errorf("journal retention backlog request is invalid")
	}
	var total int64
	queries := []*gorm.DB{
		r.db.WithContext(ctx).Model(&journalSnapshotPO{}).
			Where("cleanup_state IN (?, ?, ?) OR (cleanup_state = ? AND created_at <= ?)",
				entity.JournalSnapshotCleanupStatePending, entity.JournalSnapshotCleanupStateDeleting,
				entity.JournalSnapshotCleanupStateFailed, entity.JournalSnapshotCleanupStateActive,
				req.SnapshotCutoffAt).
			Where(journalRetentionActiveAttemptSQL),
		r.db.WithContext(ctx).Model(&journalSnapshotReservationPO{}).
			Where("expires_at <= ?", req.StagingCutoffAt),
		r.db.WithContext(ctx).Model(&checkpointPO{}).
			Where("created_at <= ?", req.SnapshotCutoffAt).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_runs active_run
WHERE active_run.id = agent_checkpoints.run_id
  AND active_run.status IN ('pending', 'queued', 'running', 'interrupted')
)`).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_run_attempts active_attempt
WHERE (active_attempt.execution_run_id = agent_checkpoints.run_id
    OR active_attempt.journal_run_id = agent_checkpoints.run_id)
  AND active_attempt.status IN ('pending', 'running')
)`),
		r.db.WithContext(ctx).Model(&runEventPO{}).
			Where("attempt_id IS NOT NULL AND created_at <= ?", req.ExecutionCutoffAt).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_run_attempts active_attempt
WHERE active_attempt.journal_run_id = agent_run_events.journal_run_id
  AND active_attempt.status IN ('pending', 'running')
)`),
		r.db.WithContext(ctx).Model(&sideEffectLedgerPO{}).
			Where("created_at <= ?", req.ExecutionCutoffAt).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_run_attempts active_attempt
WHERE active_attempt.journal_run_id = agent_side_effect_ledger.journal_run_id
  AND active_attempt.status IN ('pending', 'running')
)`),
		r.db.WithContext(ctx).Model(&runAttemptPO{}).
			Where("status NOT IN (?, ?) AND COALESCE(ended_at, updated_at, created_at) <= ?",
				entity.RunAttemptStatusPending, entity.RunAttemptStatusRunning, req.ExecutionCutoffAt).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_run_events event
WHERE event.journal_run_id = agent_run_attempts.journal_run_id
  AND event.attempt_id = agent_run_attempts.attempt_id
)`).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_side_effect_ledger ledger
WHERE ledger.journal_run_id = agent_run_attempts.journal_run_id
  AND ledger.attempt_id = agent_run_attempts.attempt_id
)`).
			Where(`NOT EXISTS (
SELECT 1 FROM agent_journal_snapshots snapshot
WHERE snapshot.journal_run_id = agent_run_attempts.journal_run_id
  AND snapshot.attempt_id = agent_run_attempts.attempt_id
)`),
	}
	for _, query := range queries {
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

func journalSnapshotCleanupObjectKeys(
	tx *gorm.DB,
	snapshot journalSnapshotPO,
) ([]string, error) {
	keys := make([]string, 0, 1)
	if snapshot.ObjectKey != nil && strings.TrimSpace(*snapshot.ObjectKey) != "" {
		keys = append(keys, strings.TrimSpace(*snapshot.ObjectKey))
	}
	var fragmentKeys []string
	if err := tx.Model(&journalSnapshotFragmentPO{}).
		Where("snapshot_id = ? AND object_key IS NOT NULL", snapshot.SnapshotID).
		Pluck("object_key", &fragmentKeys).Error; err != nil {
		return nil, err
	}
	return append(keys, fragmentKeys...), nil
}

func validateJournalRetentionClaimRequest(req JournalRetentionClaimRequest) error {
	if req.CutoffAt <= 0 || req.Now <= 0 || req.LeaseUntil <= req.Now ||
		req.BatchSize <= 0 || req.BatchSize > 1_000 {
		return fmt.Errorf("journal retention claim request is invalid")
	}
	return nil
}

func validateJournalRetentionDeleteRequest(req JournalRetentionDeleteRequest) error {
	if req.CutoffAt <= 0 || req.BatchSize <= 0 || req.BatchSize > 1_000 {
		return fmt.Errorf("journal retention delete request is invalid")
	}
	return nil
}

func validateJournalSnapshotCleanupClaim(claim JournalSnapshotCleanupClaim) error {
	if claim.SpaceID <= 0 || strings.TrimSpace(claim.SnapshotID) == "" ||
		strings.TrimSpace(claim.ClaimToken) == "" {
		return fmt.Errorf("journal snapshot cleanup claim is invalid")
	}
	return nil
}

func validateJournalStagingCleanupClaim(claim JournalStagingCleanupClaim) error {
	if claim.SpaceID <= 0 || strings.TrimSpace(claim.SnapshotID) == "" ||
		strings.TrimSpace(claim.ClaimToken) == "" {
		return fmt.Errorf("journal staging cleanup claim is invalid")
	}
	return nil
}

func normalizeJournalRetentionErrorCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 64 {
		return "cleanup_failed"
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return "cleanup_failed"
		}
	}
	return value
}

var _ JournalRetentionRepository = (*threadRepository)(nil)
