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
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func (r *threadRepository) createOrdinaryLeaseRecoveryRunBundle(
	ctx context.Context,
	req CreateRunBundleRequest,
) (*CreateRunBundleResult, error) {
	if err := validateOrdinaryLeaseRecoveryBundle(req); err != nil {
		return nil, err
	}
	target := *req.Run
	attempt := *req.Attempt
	ordinary := *req.OrdinaryLeaseRecovery
	checkpointAuthority := *ordinary.SourceCheckpoint
	lease := *ordinary.ExpiredLease
	event := *lease.Event
	normalized := CreateRunBundleRequest{Run: &target, Attempt: &attempt}
	ordinary.SourceCheckpoint = &checkpointAuthority
	ordinary.ExpiredLease = &lease
	normalized.OrdinaryLeaseRecovery = &ordinary

	targetPO, err := runToPO(&target)
	if err != nil {
		return nil, err
	}
	eventPO, err := runEventToPO(&event)
	if err != nil {
		return nil, err
	}

	var result *CreateRunBundleResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockThreadForUpdate(tx, target.ThreadID); err != nil {
			return err
		}
		replayed, found, err := findOrdinaryLeaseRecoveryReplay(tx, normalized)
		if err != nil {
			return err
		}
		if found {
			result = replayed
			return nil
		}

		bareSource := strings.TrimSpace(ordinary.SourceAttemptID) == ""
		rootQuery := tx.Where("id = ?", ordinary.JournalRunID)
		if tx.Dialector.Name() != "sqlite" {
			rootQuery = rootQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var root runPO
		if err := rootQuery.First(&root).Error; err != nil {
			return err
		}
		if root.ThreadID != target.ThreadID || root.SpaceID != target.SpaceID ||
			root.CreatorID != target.CreatorID || root.ParentRunID != 0 ||
			(root.RunKind != "" && root.RunKind != string(entity.RunKindTask)) {
			return ErrJournalParentMismatch
		}
		var source runPO
		if bareSource {
			source = root
		} else {
			sourceQuery := tx.Where("id = ?", ordinary.SourceRunID)
			if tx.Dialector.Name() != "sqlite" {
				sourceQuery = sourceQuery.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := sourceQuery.First(&source).Error; err != nil {
				return err
			}
		}
		if source.ID != ordinary.SourceRunID || source.ThreadID != target.ThreadID ||
			source.SpaceID != target.SpaceID || source.CreatorID != target.CreatorID ||
			source.ParentRunID != 0 ||
			(source.RunKind != "" && source.RunKind != string(entity.RunKindTask)) {
			return ErrJournalParentMismatch
		}
		if bareSource {
			var attemptCount int64
			if err := tx.Model(&runAttemptPO{}).
				Where("journal_run_id = ?", ordinary.JournalRunID).
				Count(&attemptCount).Error; err != nil {
				return err
			}
			if attemptCount != 0 {
				return fmt.Errorf("%w: ordinary bare source already has an attempt", ErrRunIdempotencyConflict)
			}
		} else {
			sourceAttemptQuery := tx.Where(
				"journal_run_id = ? AND attempt_id = ?",
				ordinary.JournalRunID,
				ordinary.SourceAttemptID,
			)
			if tx.Dialector.Name() != "sqlite" {
				sourceAttemptQuery = sourceAttemptQuery.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			var sourceAttempt runAttemptPO
			if err := sourceAttemptQuery.First(&sourceAttempt).Error; err != nil {
				return err
			}
			if sourceAttempt.ThreadID != target.ThreadID || sourceAttempt.ExecutionRunID != source.ID ||
				!entity.RunAttemptStatus(sourceAttempt.Status).IsActive() || sourceAttempt.ActiveSlot == nil ||
				sourceAttempt.ProjectionState != string(entity.JournalProjectionStateDisabled) {
				return fmt.Errorf("%w: ordinary recovery source attempt is not active disabled", ErrRunIdempotencyConflict)
			}
			attempt.Ordinal = sourceAttempt.Ordinal + 1
		}

		var checkpoint checkpointPO
		checkpointQuery := tx.Where("id = ?", ordinary.SourceCheckpointID)
		if tx.Dialector.Name() != "sqlite" {
			checkpointQuery = checkpointQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := checkpointQuery.First(&checkpoint).Error; err != nil {
			return err
		}
		checkpointMatches, err := ordinaryLeaseRecoveryCheckpointMatches(
			&checkpoint,
			ordinary.SourceCheckpoint,
		)
		if err != nil {
			return err
		}
		if !checkpointMatches || checkpoint.ThreadID != target.ThreadID || checkpoint.RunID != source.ID {
			return ErrJournalParentMismatch
		}

		updates := map[string]any{
			"status": string(entity.RunStatusInterrupted), "error_code": lease.ErrorCode,
			"error_message": lease.ErrorMessage, "ended_at": lease.Now, "updated_at": lease.Now,
		}
		clearRunLeaseUpdates(updates)
		updated := tx.Model(&runPO{}).
			Where("id = ? AND status = ?", source.ID, string(entity.RunStatusRunning)).
			Where("lease_owner = ? AND lease_token = ?", lease.LeaseOwner, lease.LeaseToken).
			Where("execution_generation = ?", lease.ExecutionGeneration).
			Where("lease_expires_at IS NOT NULL AND lease_expires_at <= ?", lease.Now).
			Updates(updates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return fmt.Errorf("%w: run %d expired lease cannot be recovered", ErrRunLeaseLost, source.ID)
		}
		if err := createBaseRunEvent(tx, eventPO); err != nil {
			return err
		}
		if !bareSource {
			startedAt := source.StartedAt
			if startedAt <= 0 {
				startedAt = lease.Now
			}
			updatedAttempt := tx.Model(&runAttemptPO{}).
				Where("journal_run_id = ? AND attempt_id = ?", ordinary.JournalRunID, ordinary.SourceAttemptID).
				Where("execution_run_id = ? AND active_slot = ?", source.ID, 1).
				Where("status IN ?", []string{
					string(entity.RunAttemptStatusPending), string(entity.RunAttemptStatusRunning),
				}).
				Where("projection_state = ?", string(entity.JournalProjectionStateDisabled)).
				Updates(map[string]any{
					"status": string(entity.RunAttemptStatusInterrupted), "active_slot": nil,
					"terminal_event_id": event.ID, "started_at": startedAt,
					"ended_at": lease.Now, "updated_at": lease.Now,
				})
			if updatedAttempt.Error != nil {
				return updatedAttempt.Error
			}
			if updatedAttempt.RowsAffected != 1 {
				return fmt.Errorf("%w: ordinary recovery source attempt changed", ErrRunLeaseLost)
			}
		}
		activeRuns, err := lockActiveTopLevelRuns(tx, &target, false)
		if err != nil {
			return err
		}
		if len(activeRuns) > 0 {
			return fmt.Errorf("%w: thread %d", ErrActiveRunExists, target.ThreadID)
		}
		if err := tx.Create(targetPO).Error; err != nil {
			return err
		}
		activeSlot := uint8(1)
		attempt.ActiveSlot = &activeSlot
		attempt.TerminalEventID = nil
		attempt.EndedAt = nil
		attempt.StartedAt = nil
		if err := tx.Create(runAttemptToPO(&attempt)).Error; err != nil {
			return err
		}
		result = &CreateRunBundleResult{Run: &target, Attempt: &attempt, Created: true}
		return nil
	})
	if err != nil {
		replayed, found, replayErr := findOrdinaryLeaseRecoveryReplay(r.db.WithContext(ctx), normalized)
		if replayErr != nil {
			return nil, replayErr
		}
		if found {
			return replayed, nil
		}
		return nil, err
	}
	return result, nil
}

func validateOrdinaryLeaseRecoveryBundle(req CreateRunBundleRequest) error {
	ordinary := req.OrdinaryLeaseRecovery
	if req.Run == nil || req.Attempt == nil || ordinary == nil || ordinary.ExpiredLease == nil ||
		ordinary.SourceCheckpoint == nil {
		return fmt.Errorf("ordinary lease recovery bundle is required")
	}
	key := strings.TrimSpace(ordinary.IdempotencyKey)
	sourceAttemptID := strings.TrimSpace(ordinary.SourceAttemptID)
	if !isTopLevelTaskRun(req.Run) || ordinary.JournalRunID <= 0 || ordinary.SourceRunID <= 0 ||
		(sourceAttemptID == "" && ordinary.SourceRunID != ordinary.JournalRunID) ||
		ordinary.SourceCheckpointID <= 0 || key == "" ||
		strings.TrimSpace(req.Run.IdempotencyKey) != key {
		return fmt.Errorf("ordinary lease recovery identity is invalid")
	}
	checkpoint := ordinary.SourceCheckpoint
	if checkpoint.ID != ordinary.SourceCheckpointID || checkpoint.ThreadID != req.Run.ThreadID ||
		checkpoint.RunID != ordinary.SourceRunID || checkpoint.RuntimeDeletedAt != 0 ||
		checkpoint.CheckpointNS != "eino.adk" || checkpoint.RuntimeType != "eino_adk" ||
		strings.TrimSpace(checkpoint.RuntimeKey) == "" || checkpoint.EnvelopeVersion <= 0 {
		return fmt.Errorf("ordinary lease recovery checkpoint authority is invalid")
	}
	if _, err := checkpointToPO(checkpoint); err != nil {
		return fmt.Errorf("ordinary lease recovery checkpoint authority is invalid: %w", err)
	}
	expectedSourceAttempt := req.Attempt.SourceAttemptID == nil && sourceAttemptID == "" ||
		req.Attempt.SourceAttemptID != nil && strings.TrimSpace(*req.Attempt.SourceAttemptID) == sourceAttemptID
	if req.Attempt.ThreadID != req.Run.ThreadID || req.Attempt.ExecutionRunID != req.Run.ID ||
		req.Attempt.JournalRunID != ordinary.JournalRunID || req.Attempt.Ordinal != 1 ||
		req.Attempt.SourceCheckpointID == nil || *req.Attempt.SourceCheckpointID != ordinary.SourceCheckpointID ||
		!expectedSourceAttempt || req.Attempt.RecoveryIdempotencyKey == nil ||
		strings.TrimSpace(*req.Attempt.RecoveryIdempotencyKey) != key ||
		req.Attempt.EnrollmentVersion != entity.JournalSchemaVersion ||
		req.Attempt.SnapshotsEnabled || req.Attempt.ProjectionState != entity.JournalProjectionStateDisabled ||
		req.Attempt.ProjectionDegradedAt != nil {
		return fmt.Errorf("ordinary lease recovery attempt is invalid")
	}
	lease := ordinary.ExpiredLease
	if lease.RunID != ordinary.SourceRunID || lease.ToStatus != entity.RunStatusInterrupted ||
		strings.TrimSpace(lease.LeaseOwner) == "" || strings.TrimSpace(lease.LeaseToken) == "" ||
		lease.ExecutionGeneration == 0 || lease.Now <= 0 || lease.Event == nil ||
		lease.Event.RunID != lease.RunID || lease.Event.ThreadID != req.Run.ThreadID ||
		lease.Event.EventType != "run.interrupted" || lease.JournalEvent != nil {
		return fmt.Errorf("ordinary lease recovery source fence is invalid")
	}
	return nil
}

func ordinaryLeaseRecoveryCheckpointMatches(
	stored *checkpointPO,
	expected *entity.Checkpoint,
) (bool, error) {
	if stored == nil || expected == nil {
		return false, nil
	}
	expectedPO, err := checkpointToPO(expected)
	if err != nil {
		return false, err
	}
	if stored.ID != expectedPO.ID || stored.ThreadID != expectedPO.ThreadID ||
		stored.RunID != expectedPO.RunID || stored.ParentCheckpointID != expectedPO.ParentCheckpointID ||
		stored.CheckpointNS != expectedPO.CheckpointNS || stored.RuntimeType != expectedPO.RuntimeType ||
		stored.RuntimeKey != expectedPO.RuntimeKey || stored.EnvelopeVersion != expectedPO.EnvelopeVersion ||
		stored.RuntimeDeletedAt != expectedPO.RuntimeDeletedAt || stored.CreatedAt != expectedPO.CreatedAt {
		return false, nil
	}
	comparisons := []struct {
		stored string
		wanted string
	}{
		{jsonToString(stored.ChannelValues), jsonToString(expectedPO.ChannelValues)},
		{jsonToString(stored.ChannelVersions), jsonToString(expectedPO.ChannelVersions)},
		{jsonToString(stored.PendingSends), jsonToString(expectedPO.PendingSends)},
		{jsonToString(stored.Metadata), jsonToString(expectedPO.Metadata)},
	}
	for _, comparison := range comparisons {
		equal, compareErr := adaptiveExecutionJSONEqual(comparison.stored, comparison.wanted)
		if compareErr != nil || !equal {
			return false, compareErr
		}
	}
	return true, nil
}

func findOrdinaryLeaseRecoveryReplay(
	db *gorm.DB,
	req CreateRunBundleRequest,
) (*CreateRunBundleResult, bool, error) {
	key := strings.TrimSpace(req.OrdinaryLeaseRecovery.IdempotencyKey)
	var target runPO
	err := db.Where("space_id = ? AND idempotency_key = ?", req.Run.SpaceID, key).First(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	conflict := func(reason string) (*CreateRunBundleResult, bool, error) {
		return nil, false, fmt.Errorf("%w: %s", ErrRunIdempotencyConflict, reason)
	}
	if equal, compareErr := ordinaryLeaseRecoveryRunReplayEqual(&target, req.Run); compareErr != nil {
		return nil, false, compareErr
	} else if !equal {
		return conflict("ordinary recovery target identity drift")
	}
	var attempt runAttemptPO
	err = db.Where("execution_run_id = ?", target.ID).First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return conflict("ordinary recovery target is missing attempt")
	}
	if err != nil {
		return nil, false, err
	}
	sourceAttemptID := strings.TrimSpace(req.OrdinaryLeaseRecovery.SourceAttemptID)
	expectedOrdinal := uint32(1)
	if sourceAttemptID != "" {
		var sourceAttempt runAttemptPO
		if err := db.Where(
			"journal_run_id = ? AND attempt_id = ?",
			req.OrdinaryLeaseRecovery.JournalRunID,
			sourceAttemptID,
		).First(&sourceAttempt).Error; err != nil {
			return nil, false, err
		}
		expectedOrdinal = sourceAttempt.Ordinal + 1
		if sourceAttempt.ExecutionRunID != req.OrdinaryLeaseRecovery.SourceRunID ||
			entity.RunAttemptStatus(sourceAttempt.Status) != entity.RunAttemptStatusInterrupted ||
			sourceAttempt.ActiveSlot != nil || sourceAttempt.TerminalEventID == nil ||
			sourceAttempt.ProjectionState != string(entity.JournalProjectionStateDisabled) {
			return conflict("ordinary recovery source attempt is not committed")
		}
	}
	expectedSourceAttempt := attempt.SourceAttemptID == nil && sourceAttemptID == "" ||
		attempt.SourceAttemptID != nil && strings.TrimSpace(*attempt.SourceAttemptID) == sourceAttemptID
	if attempt.JournalRunID != req.OrdinaryLeaseRecovery.JournalRunID || attempt.Ordinal != expectedOrdinal ||
		attempt.SourceCheckpointID == nil || *attempt.SourceCheckpointID != req.OrdinaryLeaseRecovery.SourceCheckpointID ||
		!expectedSourceAttempt || strings.TrimSpace(stringFromPtr(attempt.RecoveryIdempotencyKey)) != key ||
		attempt.EnrollmentVersion != entity.JournalSchemaVersion || attempt.SnapshotsEnabled ||
		attempt.ProjectionState != string(entity.JournalProjectionStateDisabled) {
		return conflict("ordinary recovery target attempt drift")
	}
	var source runPO
	if err := db.Where("id = ?", req.OrdinaryLeaseRecovery.SourceRunID).First(&source).Error; err != nil {
		return nil, false, err
	}
	if entity.RunStatus(source.Status) != entity.RunStatusInterrupted || source.LeaseToken != nil {
		return conflict("ordinary recovery source is not committed")
	}
	var terminalEvents []runEventPO
	if err := db.Where(
		"thread_id = ? AND run_id = ? AND event_type = ? AND created_at = ?",
		req.Run.ThreadID,
		source.ID,
		"run.interrupted",
		req.OrdinaryLeaseRecovery.ExpiredLease.Event.CreatedAt,
	).Find(&terminalEvents).Error; err != nil {
		return nil, false, err
	}
	if len(terminalEvents) != 1 || !journalColumnsEmpty(&terminalEvents[0]) {
		return conflict("ordinary recovery source event is missing")
	}
	payloadEqual, compareErr := adaptiveExecutionJSONEqual(
		jsonToString(terminalEvents[0].Payload),
		req.OrdinaryLeaseRecovery.ExpiredLease.Event.Payload,
	)
	if compareErr != nil {
		return nil, false, compareErr
	}
	if !payloadEqual {
		return conflict("ordinary recovery source event drift")
	}
	if sourceAttemptID != "" {
		var sourceAttempt runAttemptPO
		if err := db.Where(
			"journal_run_id = ? AND attempt_id = ?",
			req.OrdinaryLeaseRecovery.JournalRunID,
			sourceAttemptID,
		).First(&sourceAttempt).Error; err != nil {
			return nil, false, err
		}
		if sourceAttempt.TerminalEventID == nil || *sourceAttempt.TerminalEventID != terminalEvents[0].ID {
			return conflict("ordinary recovery source terminal pointer drift")
		}
	}
	return &CreateRunBundleResult{Run: target.toEntity(), Attempt: attempt.toEntity(), Created: false}, true, nil
}

func ordinaryLeaseRecoveryRunReplayEqual(stored *runPO, expected *entity.Run) (bool, error) {
	if stored == nil || expected == nil || stored.ThreadID != expected.ThreadID ||
		stored.ParentRunID != expected.ParentRunID || stored.SpaceID != expected.SpaceID ||
		stored.CreatorID != expected.CreatorID || stored.AssistantID != expected.AssistantID ||
		stored.RunKind != string(entity.DefaultRunKind(expected.RunKind, expected.ParentRunID)) ||
		stored.MultitaskStrategy != expected.MultitaskStrategy ||
		stored.OnDisconnect != expected.OnDisconnect || stored.Durability != expected.Durability ||
		stringFromPtr(stored.IdempotencyKey) != expected.IdempotencyKey {
		return false, nil
	}
	comparisons := []struct {
		stored string
		wanted string
	}{
		{jsonToString(stored.Command), expected.Command},
		{jsonToString(stored.Input), expected.Input},
		{jsonToString(stored.Config), expected.Config},
		{jsonToString(stored.Context), expected.Context},
		{jsonToString(stored.Metadata), expected.Metadata},
		{jsonToString(stored.StreamMode), expected.StreamMode},
	}
	for _, comparison := range comparisons {
		equal, err := adaptiveExecutionJSONEqual(comparison.stored, comparison.wanted)
		if err != nil || !equal {
			return false, err
		}
	}
	return true, nil
}
