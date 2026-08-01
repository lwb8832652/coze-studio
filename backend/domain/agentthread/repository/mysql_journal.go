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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const maxJournalRunParentDepth = 64

const (
	defaultJournalListLimit = 100
	maxJournalListLimit     = 200
)

var safeLegacyJournalEventTypes = []string{
	"run.created",
	"run.queued",
	"run.started",
	"run.completed",
	"run.succeeded",
	"run.failed",
	"run.canceled",
	"run.cancelled",
	"run.timed_out",
}

func (r *threadRepository) CreateJournalAttempt(
	ctx context.Context,
	attempt *entity.RunAttempt,
) (*entity.RunAttempt, error) {
	if attempt == nil || attempt.ID <= 0 || attempt.ThreadID <= 0 ||
		attempt.JournalRunID <= 0 || attempt.ExecutionRunID <= 0 ||
		strings.TrimSpace(attempt.AttemptID) == "" {
		return nil, fmt.Errorf("journal attempt identity is required")
	}
	recoveryKey := strings.TrimSpace(stringFromPtr(attempt.RecoveryIdempotencyKey))
	if recoveryKey == "" {
		return nil, fmt.Errorf("recovery journal attempt requires an idempotency key")
	}

	normalized := cloneRunAttempt(attempt)
	normalized.AttemptID = strings.TrimSpace(normalized.AttemptID)
	normalized.RecoveryIdempotencyKey = &recoveryKey
	normalized.NextSequence = 1
	normalized.LastCommittedSequence = 0
	normalized.TerminalEventID = nil
	normalized.EndedAt = nil
	normalized.ProjectionState = entity.JournalProjectionStateHealthy
	normalized.ProjectionDegradedAt = nil

	var created *entity.RunAttempt
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedRoot := tx.Where("id = ?", normalized.JournalRunID)
		if tx.Dialector.Name() != "sqlite" {
			lockedRoot = lockedRoot.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var logicalRun runPO
		if err := lockedRoot.First(&logicalRun).Error; err != nil {
			return err
		}
		if logicalRun.ThreadID != normalized.ThreadID || !isJournalRootRun(&logicalRun) {
			return fmt.Errorf("journal run must be a logical top-level task")
		}

		var executionRun runPO
		if normalized.ExecutionRunID == logicalRun.ID {
			executionRun = logicalRun
		} else {
			lockedExecution := tx.Where("id = ?", normalized.ExecutionRunID)
			if tx.Dialector.Name() != "sqlite" {
				lockedExecution = lockedExecution.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := lockedExecution.First(&executionRun).Error; err != nil {
				return err
			}
		}
		if executionRun.ThreadID != logicalRun.ThreadID || !isJournalRootRun(&executionRun) {
			return fmt.Errorf("journal attempt execution run must be a top-level task in the logical run thread")
		}

		if replay, found, err := findRecoveryAttempt(tx, normalized); err != nil {
			return err
		} else if found {
			created = replay
			return nil
		}
		if !normalized.Status.IsActive() {
			return fmt.Errorf("new journal attempt must be pending or running")
		}

		var attempts []runAttemptPO
		if err := tx.Where("journal_run_id = ?", logicalRun.ID).
			Order("ordinal ASC").Find(&attempts).Error; err != nil {
			return err
		}
		if len(attempts) == 0 {
			return ErrJournalNotEnrolled
		}
		latest := attempts[len(attempts)-1]
		if entity.RunAttemptStatus(latest.Status).IsActive() || latest.ActiveSlot != nil {
			return ErrActiveJournalAttemptExists
		}

		status, err := journalAttemptStatusFromRun(entity.RunStatus(executionRun.Status))
		if err != nil {
			return err
		}
		if normalized.Status != status {
			return fmt.Errorf("journal attempt status does not match execution run")
		}

		nextOrdinal := latest.Ordinal + 1
		if normalized.Ordinal == 0 {
			normalized.Ordinal = nextOrdinal
		}
		if normalized.Ordinal != nextOrdinal {
			return fmt.Errorf("journal attempt ordinal must be %d", nextOrdinal)
		}
		first := attempts[0]
		normalized.EnrollmentVersion = first.EnrollmentVersion
		normalized.SnapshotsEnabled = first.SnapshotsEnabled
		activeSlot := uint8(1)
		normalized.ActiveSlot = &activeSlot
		now := time.Now().UnixMilli()
		if normalized.CreatedAt <= 0 {
			normalized.CreatedAt = now
		}
		if normalized.UpdatedAt <= 0 {
			normalized.UpdatedAt = normalized.CreatedAt
		}
		if normalized.Status == entity.RunAttemptStatusPending {
			normalized.StartedAt = nil
		} else if normalized.StartedAt == nil {
			startedAt := executionRun.StartedAt
			if startedAt <= 0 {
				startedAt = normalized.CreatedAt
			}
			normalized.StartedAt = &startedAt
		}
		if err := tx.Create(runAttemptToPO(normalized)).Error; err != nil {
			return err
		}
		created = cloneRunAttempt(normalized)
		return nil
	})
	if err == nil {
		return created, nil
	}

	if replay, found, replayErr := findRecoveryAttempt(r.db.WithContext(ctx), normalized); replayErr != nil {
		return nil, replayErr
	} else if found {
		return replay, nil
	}
	if isActiveJournalAttemptConstraintError(err) {
		return nil, ErrActiveJournalAttemptExists
	}
	return nil, err
}

func (r *threadRepository) PrepareSideEffect(
	ctx context.Context,
	req PrepareSideEffectRequest,
) (*entity.SideEffectLedger, bool, error) {
	ledger, err := normalizePreparedSideEffect(req.Ledger)
	if err != nil {
		return nil, false, err
	}
	audit, err := normalizeSideEffectAuditEvent(req.AuditEvent, ledger)
	if err != nil {
		return nil, false, err
	}

	var stored *entity.SideEffectLedger
	var created bool
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt, err := lockJournalAttemptByIdentity(tx, ledger.JournalRunID, ledger.AttemptID)
		if err != nil {
			return err
		}
		if replay, found, err := findSideEffectLedgerByIdentity(
			tx, ledger.JournalRunID, ledger.AttemptID, ledger.IdempotencyKey,
		); err != nil {
			return err
		} else if found {
			if err := validateSideEffectReplay(replay, ledger); err != nil {
				return err
			}
			stored = replay
			return nil
		}
		if !entity.RunAttemptStatus(attempt.Status).IsActive() {
			return ErrJournalAttemptTerminal
		}
		if attempt.ThreadID != ledger.ThreadID {
			return ErrJournalParentMismatch
		}
		if err := ensureJournalAttemptRunning(tx, attempt, ledger.PreparedAt); err != nil {
			return err
		}
		if err := tx.Create(sideEffectLedgerToPO(ledger)).Error; err != nil {
			return err
		}
		if _, err := appendSideEffectAuditLocked(tx, attempt, audit); err != nil {
			return err
		}
		stored = cloneSideEffectLedger(ledger)
		created = true
		return nil
	})
	if err == nil {
		return stored, created, nil
	}

	// The unique identity is the WAL idempotency boundary. A concurrent
	// prepare that committed first is a replay only when every immutable field
	// still describes the same external action.
	replay, found, replayErr := findSideEffectLedgerByIdentity(
		r.db.WithContext(ctx), ledger.JournalRunID, ledger.AttemptID, ledger.IdempotencyKey,
	)
	if replayErr != nil {
		return nil, false, replayErr
	}
	if found {
		if replayErr := validateSideEffectReplay(replay, ledger); replayErr != nil {
			return nil, false, replayErr
		}
		return replay, false, nil
	}
	return nil, false, err
}

func (r *threadRepository) TransitionSideEffect(
	ctx context.Context,
	req TransitionSideEffectRequest,
) (*entity.SideEffectLedger, bool, error) {
	if req.JournalRunID <= 0 || strings.TrimSpace(req.AttemptID) == "" ||
		req.LedgerID <= 0 || req.ExpectedVersion == 0 ||
		!req.FromStatus.Valid() || !req.ToStatus.Valid() {
		return nil, false, fmt.Errorf("side effect transition identity is required")
	}
	if !validStandaloneSideEffectTransition(req) {
		return nil, false, ErrSideEffectTransitionInvalid
	}
	if req.OccurredAt <= 0 {
		req.OccurredAt = time.Now().UnixMilli()
	}
	if req.ExternalReferenceDigest != "" && !validSideEffectDigest(req.ExternalReferenceDigest) {
		return nil, false, fmt.Errorf("external reference digest must be a SHA-256 digest")
	}

	var stored *entity.SideEffectLedger
	var changed bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt, err := lockJournalAttemptByIdentity(
			tx, req.JournalRunID, strings.TrimSpace(req.AttemptID),
		)
		if err != nil {
			return err
		}
		ledger, err := lockSideEffectLedger(tx, req.JournalRunID, req.AttemptID, req.LedgerID)
		if err != nil {
			return err
		}
		if entity.SideEffectLedgerStatus(ledger.Status) == req.ToStatus {
			if ledger.Version != req.ExpectedVersion+1 {
				return ErrSideEffectLedgerConflict
			}
			stored = ledger.toEntity()
			return nil
		}
		if entity.SideEffectLedgerStatus(ledger.Status) != req.FromStatus ||
			ledger.Version != req.ExpectedVersion {
			return ErrSideEffectLedgerConflict
		}
		if req.ToStatus == entity.SideEffectLedgerStatusCompensated &&
			strings.TrimSpace(stringFromPtr(ledger.CompensationKind)) == "" {
			return ErrSideEffectTransitionInvalid
		}
		audit, err := normalizeSideEffectAuditEvent(req.AuditEvent, ledger.toEntity())
		if err != nil {
			return err
		}
		updates := sideEffectTransitionUpdates(req)
		updates["version"] = req.ExpectedVersion + 1
		updates["updated_at"] = req.OccurredAt
		result := tx.Model(&sideEffectLedgerPO{}).
			Where("id = ? AND version = ? AND status = ?", ledger.ID, req.ExpectedVersion, req.FromStatus).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSideEffectLedgerConflict
		}
		if _, err := appendSideEffectAuditLocked(tx, attempt, audit); err != nil {
			return err
		}
		applySideEffectTransition(ledger, req)
		stored = ledger.toEntity()
		changed = true
		return nil
	})
	return stored, changed, err
}

func (r *threadRepository) ResolveUnknownSideEffect(
	ctx context.Context,
	req ResolveUnknownSideEffectRequest,
) (*entity.SideEffectLedger, bool, error) {
	key := strings.TrimSpace(req.IdempotencyKey)
	if req.JournalRunID <= 0 || strings.TrimSpace(req.AttemptID) == "" ||
		req.LedgerID <= 0 || req.ExpectedVersion == 0 || !req.Action.Valid() || key == "" {
		return nil, false, fmt.Errorf("unknown side effect resolution identity is required")
	}
	if len(key) > 191 {
		return nil, false, fmt.Errorf("unknown side effect resolution idempotency key is too long")
	}
	if req.OccurredAt <= 0 {
		req.OccurredAt = time.Now().UnixMilli()
	}

	var stored *entity.SideEffectLedger
	var changed bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt, err := lockJournalAttemptByIdentity(
			tx, req.JournalRunID, strings.TrimSpace(req.AttemptID),
		)
		if err != nil {
			return err
		}
		ledger, err := lockSideEffectLedger(tx, req.JournalRunID, req.AttemptID, req.LedgerID)
		if err != nil {
			return err
		}
		if ledger.ResolutionIdempotencyKey != nil {
			if strings.TrimSpace(stringFromPtr(ledger.ResolutionIdempotencyKey)) == key &&
				entity.SideEffectResolutionAction(stringFromPtr(ledger.ResolutionAction)) == req.Action {
				stored = ledger.toEntity()
				return nil
			}
			return ErrSideEffectLedgerConflict
		}
		if entity.SideEffectLedgerStatus(ledger.Status) != entity.SideEffectLedgerStatusUnknown ||
			ledger.Version != req.ExpectedVersion {
			return ErrSideEffectLedgerConflict
		}
		audit, err := normalizeSideEffectAuditEvent(req.AuditEvent, ledger.toEntity())
		if err != nil {
			return err
		}
		updates := map[string]any{
			"resolution_action":          string(req.Action),
			"resolution_idempotency_key": key,
			"resolved_at":                req.OccurredAt,
			"version":                    req.ExpectedVersion + 1,
			"updated_at":                 req.OccurredAt,
		}
		result := tx.Model(&sideEffectLedgerPO{}).
			Where(
				"id = ? AND version = ? AND status = ? AND resolution_idempotency_key IS NULL",
				ledger.ID, req.ExpectedVersion, entity.SideEffectLedgerStatusUnknown,
			).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrSideEffectLedgerConflict
		}
		if _, err := appendSideEffectAuditLocked(tx, attempt, audit); err != nil {
			return err
		}
		action := string(req.Action)
		ledger.ResolutionAction = &action
		ledger.ResolutionIdempotencyKey = &key
		ledger.ResolvedAt = cloneInt64Pointer(&req.OccurredAt)
		ledger.Version = req.ExpectedVersion + 1
		ledger.UpdatedAt = req.OccurredAt
		stored = ledger.toEntity()
		changed = true
		return nil
	})
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return nil, false, ErrSideEffectLedgerConflict
	}
	return stored, changed, err
}

func (r *threadRepository) CommitExecutionBoundary(
	ctx context.Context,
	req CommitExecutionBoundaryRequest,
) (*CommitExecutionBoundaryResult, error) {
	if req.JournalRunID <= 0 || strings.TrimSpace(req.AttemptID) == "" ||
		req.LedgerID <= 0 || req.ExpectedVersion == 0 ||
		req.ResultEvent == nil || req.AuditEvent == nil || req.CheckpointFactory == nil {
		return nil, fmt.Errorf("execution boundary identity, events, and checkpoint factory are required")
	}
	if req.Status != entity.SideEffectLedgerStatusSucceeded &&
		req.Status != entity.SideEffectLedgerStatusFailed &&
		req.Status != entity.SideEffectLedgerStatusUnknown {
		return nil, ErrSideEffectTransitionInvalid
	}
	if req.ExternalReferenceDigest != "" && !validSideEffectDigest(req.ExternalReferenceDigest) {
		return nil, fmt.Errorf("external reference digest must be a SHA-256 digest")
	}

	var committed *CommitExecutionBoundaryResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt, err := lockJournalAttemptByIdentity(tx, req.JournalRunID, req.AttemptID)
		if err != nil {
			return err
		}
		ledger, err := lockSideEffectLedger(tx, req.JournalRunID, req.AttemptID, req.LedgerID)
		if err != nil {
			return err
		}
		if entity.SideEffectLedgerStatus(ledger.Status).IsTerminal() {
			replay, err := loadExecutionBoundaryReplay(tx, ledger, req)
			if err != nil {
				return err
			}
			committed = replay
			return nil
		}
		if entity.SideEffectLedgerStatus(ledger.Status) != entity.SideEffectLedgerStatusExecuting ||
			ledger.Version != req.ExpectedVersion {
			return ErrSideEffectLedgerConflict
		}
		if !entity.RunAttemptStatus(attempt.Status).IsActive() {
			return ErrJournalAttemptTerminal
		}

		resultEvent, err := normalizeJournalEvent(req.ResultEvent)
		if err != nil {
			return err
		}
		if resultEvent.Visibility != entity.JournalVisibilityUser {
			return fmt.Errorf("execution boundary result event must be user visible")
		}
		if err := bindSideEffectEvent(resultEvent, attempt, ledger); err != nil {
			return err
		}
		audit, err := normalizeSideEffectAuditEvent(req.AuditEvent, ledger.toEntity())
		if err != nil {
			return err
		}

		appended, err := appendJournalEventLocked(tx, attempt, resultEvent)
		if err != nil {
			return err
		}
		if appended.Sequence == 0 {
			return ErrJournalSequenceAllocation
		}
		if _, err := appendSideEffectAuditLocked(tx, attempt, audit); err != nil {
			return err
		}

		now := resultEvent.CreatedAt
		if now <= 0 {
			now = time.Now().UnixMilli()
		}
		checkpointLedgers, err := executionBoundaryLedgerSnapshot(
			tx,
			ledger,
			req,
			appended.ID,
			now,
		)
		if err != nil {
			return err
		}
		checkpoint, err := req.CheckpointFactory(appended.Sequence, checkpointLedgers)
		if err != nil {
			return err
		}
		if err := validateExecutionBoundaryCheckpoint(checkpoint, attempt, appended.Sequence); err != nil {
			return err
		}
		checkpointPO, err := checkpointToPO(checkpoint)
		if err != nil {
			return err
		}
		if err := tx.Create(checkpointPO).Error; err != nil {
			return err
		}

		ledgerUpdates := terminalSideEffectUpdates(req, appended.ID, checkpoint.ID, now)
		ledgerUpdate := tx.Model(&sideEffectLedgerPO{}).
			Where(
				"id = ? AND version = ? AND status = ?",
				ledger.ID, req.ExpectedVersion, entity.SideEffectLedgerStatusExecuting,
			).
			Updates(ledgerUpdates)
		if ledgerUpdate.Error != nil {
			return ledgerUpdate.Error
		}
		if ledgerUpdate.RowsAffected != 1 {
			return ErrSideEffectLedgerConflict
		}

		attemptUpdate := tx.Model(&runAttemptPO{}).
			Where(
				"id = ? AND active_slot = ? AND next_sequence = ? AND last_committed_sequence <= ?",
				attempt.ID, 1, appended.Sequence+1, appended.Sequence,
			).
			Updates(map[string]any{
				"last_committed_sequence": appended.Sequence,
				"updated_at":              now,
			})
		if attemptUpdate.Error != nil {
			return attemptUpdate.Error
		}
		if attemptUpdate.RowsAffected != 1 {
			return ErrJournalSequenceAllocation
		}

		applyTerminalSideEffect(ledger, req, appended.ID, checkpoint.ID, now)
		committed = &CommitExecutionBoundaryResult{
			Ledger: ledger.toEntity(), Event: appended, Checkpoint: checkpoint,
			LastCommittedSequence: appended.Sequence,
		}
		return nil
	})
	return committed, err
}

func executionBoundaryLedgerSnapshot(
	tx *gorm.DB,
	current *sideEffectLedgerPO,
	req CommitExecutionBoundaryRequest,
	resultEventID int64,
	occurredAt int64,
) ([]*entity.SideEffectLedger, error) {
	if tx == nil || current == nil {
		return nil, fmt.Errorf("execution boundary ledger snapshot is required")
	}
	var rows []sideEffectLedgerPO
	if err := tx.Where(
		"journal_run_id = ? AND attempt_id = ?",
		current.JournalRunID,
		current.AttemptID,
	).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]*entity.SideEffectLedger, 0, len(rows))
	found := false
	for index := range rows {
		row := &rows[index]
		if row.ID == current.ID {
			predicted := *current
			applyTerminalSideEffect(&predicted, req, resultEventID, 0, occurredAt)
			result = append(result, predicted.toEntity())
			found = true
			continue
		}
		result = append(result, row.toEntity())
	}
	if !found {
		return nil, ErrSideEffectLedgerNotFound
	}
	return result, nil
}

func (r *threadRepository) GetSideEffectLedger(
	ctx context.Context,
	journalRunID int64,
	attemptID, idempotencyKey string,
) (*entity.SideEffectLedger, error) {
	ledger, found, err := findSideEffectLedgerByIdentity(
		r.db.WithContext(ctx), journalRunID, strings.TrimSpace(attemptID),
		strings.TrimSpace(idempotencyKey),
	)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrSideEffectLedgerNotFound
	}
	return ledger, nil
}

func (r *threadRepository) ListSideEffectLedgers(
	ctx context.Context,
	journalRunID int64,
	attemptID string,
) ([]*entity.SideEffectLedger, error) {
	if journalRunID <= 0 || strings.TrimSpace(attemptID) == "" {
		return nil, fmt.Errorf("side effect ledger attempt identity is required")
	}
	var rows []sideEffectLedgerPO
	if err := r.db.WithContext(ctx).Where(
		"journal_run_id = ? AND attempt_id = ?", journalRunID, strings.TrimSpace(attemptID),
	).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]*entity.SideEffectLedger, 0, len(rows))
	for i := range rows {
		result = append(result, rows[i].toEntity())
	}
	return result, nil
}

func isActiveJournalAttemptConstraintError(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062 &&
			strings.Contains(mysqlErr.Message, "uk_agent_run_attempts_active")
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") &&
		strings.Contains(message, "agent_run_attempts.journal_run_id") &&
		strings.Contains(message, "agent_run_attempts.active_slot")
}

func (r *threadRepository) GetActiveJournalAttempt(
	ctx context.Context,
	runID int64,
) (*entity.RunAttempt, error) {
	db := r.db.WithContext(ctx)
	root, executionPath, err := resolveJournalRunPath(db, runID)
	if err != nil {
		return nil, err
	}
	var attempt runAttemptPO
	err = db.Where("execution_run_id IN ? AND active_slot = ?", executionPath, 1).
		Order("ordinal DESC").First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && runID == root.ID {
		err = db.Where("journal_run_id = ? AND active_slot = ?", root.ID, 1).
			Order("ordinal DESC").First(&attempt).Error
	}
	if err == nil {
		if !entity.RunAttemptStatus(attempt.Status).IsActive() {
			return nil, ErrJournalInvalidStateTransition
		}
		return attempt.toEntity(), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var count int64
	countQuery := db.Model(&runAttemptPO{}).Where("execution_run_id IN ?", executionPath)
	if runID == root.ID {
		countQuery = db.Model(&runAttemptPO{}).
			Where("execution_run_id IN ? OR journal_run_id = ?", executionPath, root.ID)
	}
	if err := countQuery.Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrJournalNotEnrolled
	}
	return nil, ErrJournalAttemptTerminal
}

func (r *threadRepository) ListJournalAttempts(
	ctx context.Context,
	runID int64,
) ([]*entity.RunAttempt, error) {
	if runID <= 0 {
		return nil, fmt.Errorf("journal run id is required")
	}
	db := r.db.WithContext(ctx)
	_, journalRunID, err := resolveJournalRunIdentity(db, runID)
	if err != nil {
		return nil, err
	}
	var rows []runAttemptPO
	if err := db.Where("journal_run_id = ?", journalRunID).
		Order("ordinal ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrJournalNotEnrolled
	}
	attempts := make([]*entity.RunAttempt, 0, len(rows))
	for i := range rows {
		attempts = append(attempts, rows[i].toEntity())
	}
	return attempts, nil
}

func (r *threadRepository) AppendJournalEvent(
	ctx context.Context,
	event *entity.JournalEvent,
) (*entity.JournalEvent, error) {
	normalized, err := normalizeJournalEvent(event)
	if err != nil {
		return nil, err
	}
	if normalized.EventType == "run.lifecycle" {
		payloadType, _ := validateJournalPayloadEnvelope(normalized.Payload)
		if entity.RunAttemptStatus(normalized.Status).IsTerminal() || payloadType == "terminal" {
			return nil, fmt.Errorf(
				"%w: terminal run lifecycle events must use FinalizeJournalAttempt",
				ErrJournalInvalidStateTransition,
			)
		}
	}

	var appended *entity.JournalEvent
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		executionRoot, executionPath, err := resolveJournalRunPath(tx, normalized.RunID)
		if err != nil {
			return err
		}
		if normalized.ThreadID != executionRoot.ThreadID {
			return fmt.Errorf("journal event thread does not match logical run")
		}
		attempt, err := lockJournalAttemptForEvent(
			tx,
			executionRoot.ID,
			executionPath,
			normalized.AttemptID,
		)
		if err != nil {
			return err
		}
		if normalized.JournalRunID != 0 && normalized.JournalRunID != attempt.JournalRunID {
			return ErrJournalParentMismatch
		}
		normalized.JournalRunID = attempt.JournalRunID
		normalized.AttemptID = attempt.AttemptID
		appended, err = appendJournalEventLocked(tx, attempt, normalized)
		return err
	})
	return appended, err
}

func (r *threadRepository) CreateRunEventWithJournalProjection(
	ctx context.Context,
	req CreateRunEventWithJournalProjectionRequest,
) (*entity.JournalEvent, error) {
	if req.Event == nil || req.Event.ID <= 0 || req.Event.ThreadID <= 0 || req.Event.RunID <= 0 {
		return nil, fmt.Errorf("run event identity is required")
	}
	base := *req.Event
	base.EventType = strings.TrimSpace(base.EventType)
	if base.EventType == "" {
		return nil, fmt.Errorf("run event type is required")
	}
	if base.CreatedAt <= 0 {
		base.CreatedAt = time.Now().UnixMilli()
	}
	basePO, err := runEventToPO(&base)
	if err != nil {
		return nil, err
	}

	var appended *entity.JournalEvent
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var persistErr error
		appended, persistErr = persistRunEventWithJournalProjectionTx(
			tx,
			&base,
			basePO,
			req.Journal,
			req.ProjectionFailed,
			base.RunID,
		)
		return persistErr
	})
	return appended, err
}

func persistRunEventWithJournalProjectionTx(
	tx *gorm.DB,
	base *entity.RunEvent,
	basePO *runEventPO,
	journal *entity.JournalEvent,
	projectionFailed bool,
	journalSourceRunID int64,
) (*entity.JournalEvent, error) {
	if tx == nil || base == nil || basePO == nil {
		return nil, fmt.Errorf("base run event is required")
	}
	var projected *entity.JournalEvent
	if journal != nil {
		candidate := *journal
		candidate.ID = base.ID
		candidate.ThreadID = base.ThreadID
		candidate.RunID = base.RunID
		if candidate.CreatedAt <= 0 {
			candidate.CreatedAt = base.CreatedAt
		}
		normalized, err := normalizeJournalEvent(&candidate)
		if err != nil {
			projectionFailed = true
		} else if normalized.EventType == "run.lifecycle" &&
			entity.RunAttemptStatus(normalized.Status).IsTerminal() {
			projectionFailed = true
		} else {
			projected = normalized
		}
	}
	if !projectionFailed && projected == nil {
		return nil, createBaseRunEvent(tx, basePO)
	}
	if journalSourceRunID <= 0 {
		journalSourceRunID = base.RunID
	}
	executionRoot, executionPath, err := resolveJournalRunPath(tx, journalSourceRunID)
	if err != nil {
		return nil, err
	}
	if executionRoot.ThreadID != base.ThreadID {
		return nil, fmt.Errorf("journal source run does not belong to event thread")
	}
	attempt, err := lockJournalAttemptForEvent(tx, executionRoot.ID, executionPath, "")
	if errors.Is(err, ErrJournalNotEnrolled) {
		return nil, createBaseRunEvent(tx, basePO)
	}
	if err != nil {
		return nil, err
	}
	attemptStatus := entity.RunAttemptStatus(attempt.Status)
	projectionState := entity.JournalProjectionState(attempt.ProjectionState)
	if !attemptStatus.IsActive() || projectionState != entity.JournalProjectionStateHealthy {
		return nil, createBaseRunEvent(tx, basePO)
	}
	if projectionFailed {
		if err := createBaseRunEvent(tx, basePO); err != nil {
			return nil, err
		}
		return nil, markJournalProjectionDegraded(tx, attempt, base.CreatedAt)
	}

	projected.JournalRunID = attempt.JournalRunID
	projected.AttemptID = attempt.AttemptID
	appended, err := appendJournalEventLockedWithBase(tx, attempt, projected, base)
	if err == nil {
		return appended, nil
	}
	if errors.Is(err, ErrJournalAttemptTerminal) {
		return nil, createBaseRunEvent(tx, basePO)
	}
	if !isJournalProjectionInvariantError(err) {
		return nil, err
	}
	if err := createBaseRunEvent(tx, basePO); err != nil {
		return nil, err
	}
	return nil, markJournalProjectionDegraded(tx, attempt, base.CreatedAt)
}

func (r *threadRepository) FinalizeJournalAttempt(
	ctx context.Context,
	req FinalizeJournalAttemptRequest,
) (*entity.JournalEvent, bool, error) {
	if req.RunID <= 0 || !req.Status.IsTerminal() {
		return nil, false, fmt.Errorf("terminal execution run id and attempt status are required")
	}
	normalized, err := normalizeJournalEvent(req.Event)
	if err != nil {
		return nil, false, err
	}
	if normalized.RunID != req.RunID {
		return nil, false, fmt.Errorf("terminal event does not belong to execution run")
	}
	if err := validateTerminalJournalEvent(normalized, req.Status); err != nil {
		return nil, false, err
	}

	var terminal *entity.JournalEvent
	var won bool
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var executionRun runPO
		if err := tx.Where("id = ?", req.RunID).First(&executionRun).Error; err != nil {
			return err
		}
		if executionRun.ThreadID != normalized.ThreadID {
			return fmt.Errorf("terminal journal event thread does not match execution run")
		}
		attempt, err := lockJournalAttemptForExecution(tx, req.RunID)
		if err != nil {
			return err
		}
		if normalized.JournalRunID != 0 && normalized.JournalRunID != attempt.JournalRunID {
			return ErrJournalParentMismatch
		}
		if normalized.AttemptID != "" && normalized.AttemptID != attempt.AttemptID {
			return ErrJournalParentMismatch
		}
		normalized.JournalRunID = attempt.JournalRunID
		normalized.AttemptID = attempt.AttemptID

		if entity.RunAttemptStatus(attempt.Status).IsTerminal() {
			if attempt.TerminalEventID == nil {
				return ErrJournalInvalidStateTransition
			}
			var committed runEventPO
			if err := tx.Where("id = ?", *attempt.TerminalEventID).First(&committed).Error; err != nil {
				return err
			}
			terminal = journalEventFromPO(&committed)
			return nil
		}
		if !entity.RunAttemptStatus(attempt.Status).IsActive() {
			return ErrJournalInvalidStateTransition
		}

		var replay runEventPO
		err = tx.Where(
			"journal_run_id = ? AND attempt_id = ? AND idempotency_key = ?",
			attempt.JournalRunID, attempt.AttemptID, normalized.IdempotencyKey,
		).First(&replay).Error
		if err == nil {
			return ErrJournalTerminalReplayConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := ensureJournalAttemptRunning(tx, attempt, normalized.CreatedAt); err != nil {
			return err
		}
		parentID, err := resolvePublicJournalParent(tx, attempt, normalized.ParentEventID)
		if err != nil {
			return err
		}
		normalized.ParentEventID = parentID
		sequence := attempt.NextSequence
		po, err := journalEventToPO(normalized, sequence)
		if err != nil {
			return err
		}
		if err := tx.Create(po).Error; err != nil {
			return err
		}
		endedAt := req.EndedAt
		if endedAt <= 0 {
			endedAt = time.Now().UnixMilli()
		}
		result := tx.Model(&runAttemptPO{}).
			Where(
				"id = ? AND status = ? AND active_slot = ? AND next_sequence = ?",
				attempt.ID, entity.RunAttemptStatusRunning, 1, sequence,
			).
			Updates(map[string]any{
				"status":            string(req.Status),
				"active_slot":       nil,
				"next_sequence":     sequence + 1,
				"terminal_event_id": normalized.ID,
				"ended_at":          endedAt,
				"updated_at":        endedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrJournalAttemptTerminal
		}
		terminal = journalEventFromPO(po)
		won = true
		return nil
	})
	return terminal, won, err
}

func (r *threadRepository) GetJournalEvent(
	ctx context.Context,
	eventID int64,
) (*entity.JournalEvent, error) {
	var po runEventPO
	if err := r.db.WithContext(ctx).Where("id = ?", eventID).First(&po).Error; err != nil {
		return nil, err
	}
	if po.JournalRunID != nil && po.AttemptID != nil {
		return journalEventFromPO(&po), nil
	}
	if !journalColumnsEmpty(&po) {
		return nil, ErrJournalNotEnrolled
	}

	var run runPO
	if err := r.db.WithContext(ctx).Where("id = ?", po.RunID).First(&run).Error; err != nil {
		return nil, err
	}
	if !isLegacyTerminalRunStatus(entity.RunStatus(run.Status)) {
		return nil, ErrJournalNotEnrolled
	}
	return projectSafeLegacyJournalEvent(&po)
}

func (r *threadRepository) ListJournalEvents(
	ctx context.Context,
	req ListJournalEventsRequest,
) (*ListJournalEventsResult, error) {
	if req.RunID <= 0 {
		return nil, fmt.Errorf("journal run id is required")
	}
	limit, err := normalizeJournalListLimit(req.Limit)
	if err != nil {
		return nil, err
	}
	db := r.db.WithContext(ctx)
	root, journalRunID, err := resolveJournalRunIdentity(db, req.RunID)
	if err != nil {
		return nil, err
	}

	var attempts []runAttemptPO
	if err := db.Where("journal_run_id = ?", journalRunID).
		Order("ordinal ASC").Find(&attempts).Error; err != nil {
		return nil, err
	}
	if len(attempts) == 0 {
		return listLegacyJournalEvents(db, root, req, limit)
	}
	if req.AfterEventID != 0 {
		return nil, fmt.Errorf("new journal attempts use sequence cursors")
	}
	attemptID := strings.TrimSpace(req.AttemptID)
	if attemptID == "" {
		attemptID = attempts[len(attempts)-1].AttemptID
	}
	var selected *runAttemptPO
	for i := range attempts {
		if attempts[i].AttemptID == attemptID {
			selected = &attempts[i]
			break
		}
	}
	if selected == nil {
		return nil, ErrJournalNotEnrolled
	}

	var rows []runEventPO
	if err := db.Where(
		"journal_run_id = ? AND attempt_id = ? AND visibility = ? AND sequence IS NOT NULL AND sequence > ?",
		journalRunID,
		attemptID,
		entity.JournalVisibilityUser,
		req.AfterSequence,
	).Order("sequence ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	events := make([]*entity.JournalEvent, 0, len(rows))
	for i := range rows {
		events = append(events, journalEventFromPO(&rows[i]))
	}
	return &ListJournalEventsResult{Events: events, HasMore: hasMore}, nil
}

func (r *threadRepository) GetJournalBootstrap(
	ctx context.Context,
	req GetJournalBootstrapRequest,
) (*GetJournalBootstrapResult, error) {
	if req.RunID <= 0 {
		return nil, fmt.Errorf("journal run id is required")
	}
	limit, err := normalizeJournalListLimit(req.Limit)
	if err != nil {
		return nil, err
	}

	var bootstrap *GetJournalBootstrapResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, journalRunID, err := resolveJournalRunIdentity(tx, req.RunID)
		if err != nil {
			return err
		}

		var attemptPOs []runAttemptPO
		if err := tx.Where("journal_run_id = ?", journalRunID).
			Order("ordinal ASC").Find(&attemptPOs).Error; err != nil {
			return err
		}
		if len(attemptPOs) == 0 {
			return ErrJournalNotEnrolled
		}

		attemptID := strings.TrimSpace(req.AttemptID)
		afterSequence := req.AfterSequence
		afterSequenceSet := req.AfterSequenceSet || req.AfterSequence > 0
		if req.AfterEventID > 0 {
			var cursor runEventPO
			if err := tx.Where(
				"id = ? AND journal_run_id = ?",
				req.AfterEventID,
				journalRunID,
			).First(&cursor).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrJournalCursorExpired
				}
				return err
			}
			if cursor.JournalRunID == nil || cursor.AttemptID == nil ||
				cursor.Sequence == nil || cursor.Visibility == nil ||
				*cursor.Visibility != string(entity.JournalVisibilityUser) {
				return ErrJournalCursorExpired
			}
			if (attemptID != "" && attemptID != *cursor.AttemptID) ||
				(afterSequenceSet && req.AfterSequence != *cursor.Sequence) {
				return ErrJournalEventGap
			}
			attemptID = *cursor.AttemptID
			afterSequence = *cursor.Sequence
		}
		if attemptID == "" {
			attemptID = attemptPOs[len(attemptPOs)-1].AttemptID
		}

		var selectedPO *runAttemptPO
		for index := range attemptPOs {
			if attemptPOs[index].AttemptID == attemptID {
				selectedPO = &attemptPOs[index]
				break
			}
		}
		if selectedPO == nil {
			if req.AfterEventID > 0 || afterSequenceSet {
				return ErrJournalCursorExpired
			}
			return ErrJournalNotEnrolled
		}
		latestSequence := selectedPO.NextSequence - 1
		if afterSequence > latestSequence {
			return ErrJournalEventGap
		}

		var rows []runEventPO
		if err := tx.Where(
			"journal_run_id = ? AND attempt_id = ? AND visibility = ? AND sequence IS NOT NULL AND sequence > ? AND sequence <= ?",
			journalRunID,
			attemptID,
			entity.JournalVisibilityUser,
			afterSequence,
			latestSequence,
		).Order("sequence ASC").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		expectedSequence := afterSequence + 1
		events := make([]*entity.JournalEvent, 0, len(rows))
		for index := range rows {
			if rows[index].Sequence == nil || *rows[index].Sequence != expectedSequence {
				return ErrJournalEventGap
			}
			events = append(events, journalEventFromPO(&rows[index]))
			expectedSequence++
		}
		if len(rows) == 0 && afterSequence < latestSequence {
			return ErrJournalEventGap
		}

		attempts := make([]*entity.RunAttempt, 0, len(attemptPOs))
		for index := range attemptPOs {
			attempts = append(attempts, attemptPOs[index].toEntity())
		}
		selected := selectedPO.toEntity()
		nextSequence := afterSequence
		if len(events) > 0 {
			nextSequence = events[len(events)-1].Sequence
		}
		bootstrap = &GetJournalBootstrapResult{
			Attempts: attempts, SelectedAttempt: selected, Events: events,
			LatestSequence: latestSequence, ResolvedAfterSequence: afterSequence,
			HasMore: nextSequence < latestSequence,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return bootstrap, nil
}

func listLegacyJournalEvents(
	db *gorm.DB,
	root *runPO,
	req ListJournalEventsRequest,
	limit int,
) (*ListJournalEventsResult, error) {
	if root == nil || !isLegacyTerminalRunStatus(entity.RunStatus(root.Status)) {
		return nil, ErrJournalNotEnrolled
	}
	if req.AfterSequence != 0 {
		return nil, fmt.Errorf("legacy journal attempts use event id cursors")
	}
	legacyAttemptID := fmt.Sprintf("legacy-%d", root.ID)
	if attemptID := strings.TrimSpace(req.AttemptID); attemptID != "" && attemptID != legacyAttemptID {
		return nil, ErrJournalNotEnrolled
	}

	var rows []runEventPO
	if err := db.Where(
		`run_id = ? AND id > ? AND event_type IN ? AND
		journal_run_id IS NULL AND attempt_id IS NULL AND sequence IS NULL AND
		idempotency_key IS NULL AND parent_event_id IS NULL AND schema_version IS NULL AND
		status IS NULL AND occurred_at_unix_nano IS NULL AND visibility IS NULL AND
		payload_version IS NULL AND snapshot_id IS NULL AND trace_id IS NULL AND
		action_id IS NULL AND phase IS NULL AND operation IS NULL AND target IS NULL AND milestone IS NULL`,
		root.ID,
		req.AfterEventID,
		safeLegacyJournalEventTypes,
	).Order("id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	events := make([]*entity.JournalEvent, 0, len(rows))
	for i := range rows {
		event, err := projectSafeLegacyJournalEvent(&rows[i])
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return &ListJournalEventsResult{Events: events, HasMore: hasMore, Legacy: true}, nil
}

func normalizeJournalListLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultJournalListLimit, nil
	}
	if limit < 0 || limit > maxJournalListLimit {
		return 0, fmt.Errorf("journal list limit must be between 1 and %d", maxJournalListLimit)
	}
	return limit, nil
}

func appendJournalEventLocked(
	tx *gorm.DB,
	attempt *runAttemptPO,
	event *entity.JournalEvent,
) (*entity.JournalEvent, error) {
	return appendJournalEventLockedWithBase(tx, attempt, event, nil)
}

func appendJournalEventLockedWithBase(
	tx *gorm.DB,
	attempt *runAttemptPO,
	event *entity.JournalEvent,
	base *entity.RunEvent,
) (*entity.JournalEvent, error) {
	var existing runEventPO
	err := tx.Where(
		"journal_run_id = ? AND attempt_id = ? AND idempotency_key = ?",
		attempt.JournalRunID, attempt.AttemptID, event.IdempotencyKey,
	).First(&existing).Error
	if err == nil {
		if err := createProjectedBaseReplay(tx, base); err != nil {
			return nil, err
		}
		return journalEventFromPO(&existing), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if event.ActionID != "" {
		var action runEventPO
		err := tx.Where(
			"journal_run_id = ? AND attempt_id = ? AND action_id = ?",
			attempt.JournalRunID, attempt.AttemptID, event.ActionID,
		).Order("id ASC").First(&action).Error
		if err == nil {
			if stringFromPtr(action.Operation) != event.Operation ||
				stringFromPtr(action.Target) != event.Target ||
				stringFromPtr(action.Milestone) != event.Milestone {
				return nil, ErrJournalActionDrift
			}
			var phase runEventPO
			err = tx.Where(
				"journal_run_id = ? AND attempt_id = ? AND action_id = ? AND phase = ?",
				attempt.JournalRunID, attempt.AttemptID, event.ActionID, event.Phase,
			).First(&phase).Error
			if err == nil {
				if err := createProjectedBaseReplay(tx, base); err != nil {
					return nil, err
				}
				return journalEventFromPO(&phase), nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	if !entity.RunAttemptStatus(attempt.Status).IsActive() {
		return nil, ErrJournalAttemptTerminal
	}
	if entity.JournalProjectionState(attempt.ProjectionState) !=
		entity.JournalProjectionStateHealthy {
		return nil, ErrJournalProjectionInactive
	}
	if err := ensureJournalAttemptRunning(tx, attempt, event.CreatedAt); err != nil {
		return nil, err
	}
	parentID, err := resolvePublicJournalParent(tx, attempt, event.ParentEventID)
	if err != nil {
		return nil, err
	}
	event.ParentEventID = parentID
	if event.SnapshotID != "" {
		var snapshotCount int64
		if err := tx.Model(&journalSnapshotPO{}).Where(
			"snapshot_id = ? AND event_id = ? AND thread_id = ? AND run_id = ? AND journal_run_id = ? AND attempt_id = ?",
			event.SnapshotID,
			event.ID,
			event.ThreadID,
			event.RunID,
			attempt.JournalRunID,
			attempt.AttemptID,
		).Count(&snapshotCount).Error; err != nil {
			return nil, err
		}
		if snapshotCount != 1 {
			return nil, ErrJournalSnapshotNotFound
		}
	}

	sequence := uint64(0)
	if event.Visibility == entity.JournalVisibilityUser {
		sequence = attempt.NextSequence
	}
	po, err := journalEventToPOWithBase(event, sequence, base)
	if err != nil {
		return nil, err
	}
	if sequence > 0 {
		result := tx.Model(&runAttemptPO{}).
			Where(
				"id = ? AND status = ? AND active_slot = ? AND next_sequence = ?",
				attempt.ID, entity.RunAttemptStatusRunning, 1, sequence,
			).
			Updates(map[string]any{
				"next_sequence": sequence + 1,
				"updated_at":    event.CreatedAt,
			})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != 1 {
			return nil, ErrJournalSequenceAllocation
		}
		attempt.NextSequence = sequence + 1
	}
	if err := tx.Create(po).Error; err != nil {
		return nil, err
	}
	return journalEventFromPO(po), nil
}

func createProjectedBaseReplay(tx *gorm.DB, base *entity.RunEvent) error {
	if base == nil {
		return nil
	}
	po, err := runEventToPO(base)
	if err != nil {
		return err
	}
	return createBaseRunEvent(tx, po)
}

func markJournalProjectionDegraded(tx *gorm.DB, attempt *runAttemptPO, degradedAt int64) error {
	if attempt == nil || entity.JournalProjectionState(attempt.ProjectionState) != entity.JournalProjectionStateHealthy {
		return nil
	}
	if degradedAt <= 0 {
		degradedAt = time.Now().UnixMilli()
	}
	result := tx.Model(&runAttemptPO{}).
		Where("id = ? AND projection_state = ?", attempt.ID, entity.JournalProjectionStateHealthy).
		Updates(map[string]any{
			"projection_state":       string(entity.JournalProjectionStateDegraded),
			"projection_degraded_at": degradedAt,
			"updated_at":             degradedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		attempt.ProjectionState = string(entity.JournalProjectionStateDegraded)
		attempt.ProjectionDegradedAt = &degradedAt
	}
	return nil
}

func isJournalProjectionInvariantError(err error) bool {
	return errors.Is(err, ErrJournalParentMismatch) ||
		errors.Is(err, ErrJournalActionDrift) ||
		errors.Is(err, ErrJournalInvalidStateTransition) ||
		errors.Is(err, ErrJournalSequenceAllocation)
}

func persistTerminalRunEventWithJournal(
	tx *gorm.DB,
	base *entity.RunEvent,
	basePO *runEventPO,
	journal *entity.JournalEvent,
	status entity.RunAttemptStatus,
	endedAt int64,
) error {
	if base == nil || basePO == nil || !status.IsTerminal() {
		return fmt.Errorf("terminal run event and attempt status are required")
	}
	// Repository callers that do not opt into Journal keep the original terminal
	// persistence path and must not depend on Journal tables being present.
	if journal == nil {
		return createBaseRunEvent(tx, basePO)
	}
	attempt, enrolled, err := lockJournalAttemptForTerminalProjection(tx, base.RunID)
	if err != nil {
		return err
	}
	if !enrolled {
		return createBaseRunEvent(tx, basePO)
	}
	if attempt.ThreadID != base.ThreadID {
		return fmt.Errorf("terminal journal attempt does not belong to run thread")
	}
	if !entity.RunAttemptStatus(attempt.Status).IsActive() {
		return createBaseRunEvent(tx, basePO)
	}
	if endedAt <= 0 {
		endedAt = time.Now().UnixMilli()
	}

	var normalized *entity.JournalEvent
	projectionValid := journal != nil &&
		entity.JournalProjectionState(attempt.ProjectionState) == entity.JournalProjectionStateHealthy
	if projectionValid {
		candidate := *journal
		candidate.ID = base.ID
		candidate.ThreadID = base.ThreadID
		candidate.RunID = base.RunID
		candidate.JournalRunID = attempt.JournalRunID
		candidate.AttemptID = attempt.AttemptID
		if candidate.CreatedAt <= 0 {
			candidate.CreatedAt = base.CreatedAt
		}
		normalized, err = normalizeJournalEvent(&candidate)
		if err == nil {
			err = validateTerminalJournalEvent(normalized, status)
		}
		projectionValid = err == nil
	}

	if err := ensureJournalAttemptRunning(tx, attempt, base.CreatedAt); err != nil {
		return err
	}
	if projectionValid {
		sequence := attempt.NextSequence
		po, err := journalEventToPOWithBase(normalized, sequence, base)
		if err != nil {
			return err
		}
		result := tx.Model(&runAttemptPO{}).
			Where(
				"id = ? AND status = ? AND active_slot = ? AND next_sequence = ?",
				attempt.ID, entity.RunAttemptStatusRunning, 1, sequence,
			).
			Updates(map[string]any{
				"status":            string(status),
				"active_slot":       nil,
				"next_sequence":     sequence + 1,
				"terminal_event_id": base.ID,
				"ended_at":          endedAt,
				"updated_at":        endedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			if err := tx.Create(po).Error; err != nil {
				return err
			}
			return nil
		}
		projectionValid = false
	}

	if err := createBaseRunEvent(tx, basePO); err != nil {
		return err
	}
	if entity.JournalProjectionState(attempt.ProjectionState) == entity.JournalProjectionStateHealthy {
		if err := markJournalProjectionDegraded(tx, attempt, base.CreatedAt); err != nil {
			return err
		}
	}
	result := tx.Model(&runAttemptPO{}).
		Where("id = ? AND status = ? AND active_slot = ?", attempt.ID, entity.RunAttemptStatusRunning, 1).
		Updates(map[string]any{
			"status":            string(status),
			"active_slot":       nil,
			"terminal_event_id": base.ID,
			"ended_at":          endedAt,
			"updated_at":        endedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrJournalInvalidStateTransition
	}
	return nil
}

func lockJournalAttemptForTerminalProjection(
	tx *gorm.DB,
	executionRunID int64,
) (*runAttemptPO, bool, error) {
	query := tx.Where("execution_run_id = ?", executionRunID).Order("ordinal DESC")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempt runAttemptPO
	err := query.First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &attempt, true, nil
}

func ensureJournalAttemptRunning(tx *gorm.DB, attempt *runAttemptPO, startedAt int64) error {
	status := entity.RunAttemptStatus(attempt.Status)
	if status == entity.RunAttemptStatusRunning {
		return nil
	}
	if status != entity.RunAttemptStatusPending {
		return ErrJournalAttemptTerminal
	}
	if startedAt <= 0 {
		startedAt = time.Now().UnixMilli()
	}
	result := tx.Model(&runAttemptPO{}).
		Where("id = ? AND status = ? AND active_slot = ?", attempt.ID, entity.RunAttemptStatusPending, 1).
		Updates(map[string]any{
			"status":     string(entity.RunAttemptStatusRunning),
			"started_at": startedAt,
			"updated_at": startedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrJournalInvalidStateTransition
	}
	attempt.Status = string(entity.RunAttemptStatusRunning)
	attempt.StartedAt = &startedAt
	return nil
}

func resolvePublicJournalParent(tx *gorm.DB, attempt *runAttemptPO, parentEventID int64) (int64, error) {
	seen := make(map[int64]struct{})
	for parentEventID > 0 {
		if _, ok := seen[parentEventID]; ok {
			return 0, fmt.Errorf("journal parent cycle")
		}
		seen[parentEventID] = struct{}{}
		var parent runEventPO
		err := tx.Where("id = ?", parentEventID).First(&parent).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		if parent.JournalRunID == nil || parent.AttemptID == nil ||
			*parent.JournalRunID != attempt.JournalRunID || *parent.AttemptID != attempt.AttemptID {
			return 0, ErrJournalParentMismatch
		}
		if parent.Visibility != nil &&
			*parent.Visibility == string(entity.JournalVisibilityUser) && parent.Sequence != nil {
			return parent.ID, nil
		}
		parentEventID = int64FromPtr(parent.ParentEventID)
	}
	return 0, nil
}

func resolveJournalRunPath(db *gorm.DB, runID int64) (*runPO, []int64, error) {
	if runID <= 0 {
		return nil, nil, fmt.Errorf("run id is required")
	}
	seen := make(map[int64]struct{})
	currentID := runID
	var threadID int64
	path := make([]int64, 0, 4)
	for depth := 0; depth < maxJournalRunParentDepth; depth++ {
		if _, ok := seen[currentID]; ok {
			return nil, nil, fmt.Errorf("run parent cycle")
		}
		seen[currentID] = struct{}{}
		var run runPO
		if err := db.Where("id = ?", currentID).First(&run).Error; err != nil {
			return nil, nil, err
		}
		if threadID == 0 {
			threadID = run.ThreadID
		} else if threadID != run.ThreadID {
			return nil, nil, fmt.Errorf("run parent crosses thread")
		}
		path = append(path, run.ID)
		if run.ParentRunID == 0 {
			return &run, path, nil
		}
		currentID = run.ParentRunID
	}
	return nil, nil, fmt.Errorf("run parent depth exceeds %d", maxJournalRunParentDepth)
}

func resolveJournalRootRun(db *gorm.DB, runID int64) (*runPO, error) {
	root, _, err := resolveJournalRunPath(db, runID)
	return root, err
}

func resolveJournalRunIdentity(db *gorm.DB, runID int64) (*runPO, int64, error) {
	root, executionPath, err := resolveJournalRunPath(db, runID)
	if err != nil {
		return nil, 0, err
	}
	var attempt runAttemptPO
	err = db.Where("execution_run_id IN ?", executionPath).
		Order("ordinal DESC").First(&attempt).Error
	if err == nil {
		return root, attempt.JournalRunID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, err
	}
	return root, root.ID, nil
}

func lockJournalAttemptForEvent(
	tx *gorm.DB,
	executionRootID int64,
	executionPath []int64,
	attemptID string,
) (*runAttemptPO, error) {
	query := tx.Where("execution_run_id IN ?", executionPath).
		Order("ordinal DESC")
	if attemptID != "" {
		query = query.Where("attempt_id = ?", attemptID)
	}
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempt runAttemptPO
	err := query.First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var count int64
		if countErr := tx.Model(&runAttemptPO{}).
			Where(
				"execution_run_id IN ? OR journal_run_id = ?",
				executionPath,
				executionRootID,
			).Count(&count).Error; countErr != nil {
			return nil, countErr
		}
		if count == 0 {
			return nil, ErrJournalNotEnrolled
		}
		return nil, fmt.Errorf("journal event execution run does not belong to attempt")
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

func lockJournalAttemptForExecution(tx *gorm.DB, executionRunID int64) (*runAttemptPO, error) {
	query := tx.Where("execution_run_id = ?", executionRunID).Order("ordinal DESC")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempt runAttemptPO
	err := query.First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("only the current execution run can finalize a journal attempt")
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

func normalizeJournalEvent(event *entity.JournalEvent) (*entity.JournalEvent, error) {
	if event == nil || event.ID <= 0 || event.ThreadID <= 0 || event.RunID <= 0 {
		return nil, fmt.Errorf("journal event id, thread id, and physical run id are required")
	}
	normalized := *event
	normalized.AttemptID = strings.TrimSpace(normalized.AttemptID)
	normalized.IdempotencyKey = strings.TrimSpace(normalized.IdempotencyKey)
	normalized.SchemaVersion = strings.TrimSpace(normalized.SchemaVersion)
	normalized.Status = strings.TrimSpace(normalized.Status)
	normalized.PayloadVersion = strings.TrimSpace(normalized.PayloadVersion)
	normalized.SnapshotID = strings.TrimSpace(normalized.SnapshotID)
	normalized.TraceID = strings.TrimSpace(normalized.TraceID)
	normalized.ActionID = strings.TrimSpace(normalized.ActionID)
	normalized.Phase = strings.TrimSpace(normalized.Phase)
	normalized.Operation = strings.TrimSpace(normalized.Operation)
	normalized.Target = strings.TrimSpace(normalized.Target)
	normalized.Milestone = strings.TrimSpace(normalized.Milestone)
	normalized.EventType = strings.TrimSpace(normalized.EventType)
	if normalized.EventType == "" || normalized.IdempotencyKey == "" {
		return nil, fmt.Errorf("journal event type and idempotency key are required")
	}
	if normalized.Visibility == "" {
		normalized.Visibility = entity.JournalVisibilityUser
	}
	if normalized.Visibility != entity.JournalVisibilityUser &&
		normalized.Visibility != entity.JournalVisibilityInternal {
		return nil, fmt.Errorf("unsupported journal visibility %q", normalized.Visibility)
	}
	if normalized.SchemaVersion == "" {
		normalized.SchemaVersion = entity.JournalSchemaVersion
	}
	if normalized.SchemaVersion != entity.JournalSchemaVersion {
		return nil, fmt.Errorf("unsupported journal schema version %q", normalized.SchemaVersion)
	}
	if normalized.PayloadVersion == "" {
		normalized.PayloadVersion = entity.JournalPayloadVersion
	}
	if normalized.PayloadVersion != entity.JournalPayloadVersion {
		return nil, fmt.Errorf("unsupported journal payload version %q", normalized.PayloadVersion)
	}
	if _, err := validateJournalPayloadEnvelope(normalized.Payload); err != nil {
		return nil, err
	}
	isActionEvent := strings.HasPrefix(normalized.EventType, "action.")
	hasActionMetadata := normalized.ActionID != "" || normalized.Phase != "" ||
		normalized.Operation != "" || normalized.Target != "" || normalized.Milestone != ""
	if !isActionEvent && hasActionMetadata {
		return nil, fmt.Errorf("journal action metadata is only valid on action events")
	}
	if isActionEvent &&
		(normalized.ActionID == "" || normalized.Phase == "" ||
			normalized.Operation == "" || normalized.Target == "") {
		return nil, fmt.Errorf(
			"journal action events require action id, phase, operation, and target",
		)
	}
	if isActionEvent && !journalActionPhaseMatchesEventType(normalized.EventType, normalized.Phase) {
		return nil, fmt.Errorf(
			"journal action phase %q does not match event type %q",
			normalized.Phase,
			normalized.EventType,
		)
	}
	if normalized.OccurredAtUnixNano <= 0 {
		normalized.OccurredAtUnixNano = time.Now().UnixNano()
	}
	if normalized.CreatedAt <= 0 {
		normalized.CreatedAt = normalized.OccurredAtUnixNano / int64(time.Millisecond)
	}
	normalized.Sequence = 0
	return &normalized, nil
}

func journalActionPhaseMatchesEventType(eventType, phase string) bool {
	switch eventType {
	case "action.started":
		return phase == "started"
	case "action.progress":
		return phase == "progress" ||
			(strings.HasPrefix(phase, "progress:") && strings.TrimPrefix(phase, "progress:") != "")
	case "action.terminal":
		return phase == "terminal"
	default:
		return false
	}
}

func validateJournalPayloadEnvelope(payload string) (string, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return "", fmt.Errorf("journal payload must be a JSON object envelope: %w", err)
	}
	rawType, ok := envelope["type"]
	if !ok {
		return "", fmt.Errorf("journal payload envelope type is required")
	}
	var payloadType string
	if err := json.Unmarshal(rawType, &payloadType); err != nil || strings.TrimSpace(payloadType) == "" {
		return "", fmt.Errorf("journal payload envelope type must be a non-empty string")
	}
	data, ok := envelope["data"]
	if !ok || len(data) == 0 || !json.Valid(data) {
		return "", fmt.Errorf("journal payload envelope data is required")
	}
	return strings.TrimSpace(payloadType), nil
}

func validateTerminalJournalEvent(event *entity.JournalEvent, status entity.RunAttemptStatus) error {
	if event.Visibility != entity.JournalVisibilityUser {
		return fmt.Errorf("terminal journal event must be user visible")
	}
	if event.EventType != "run.lifecycle" || event.Status != string(status) {
		return fmt.Errorf("terminal journal event type and status do not match attempt terminal state")
	}
	payloadType, err := validateJournalPayloadEnvelope(event.Payload)
	if err != nil {
		return err
	}
	if payloadType != "terminal" {
		return fmt.Errorf("terminal journal payload type must be terminal")
	}
	if event.ActionID != "" || event.Phase != "" {
		return fmt.Errorf("terminal run lifecycle event cannot replay an action phase")
	}
	return nil
}

func journalEventToPO(event *entity.JournalEvent, sequence uint64) (*runEventPO, error) {
	payload, err := requiredJSON("payload", event.Payload)
	if err != nil {
		return nil, err
	}
	journalRunID := event.JournalRunID
	attemptID := event.AttemptID
	occurredAt := event.OccurredAtUnixNano
	journalEventType := event.EventType
	po := &runEventPO{
		ID: event.ID, ThreadID: event.ThreadID, RunID: event.RunID,
		JournalRunID: &journalRunID, AttemptID: &attemptID,
		IdempotencyKey: stringPtrOrNil(event.IdempotencyKey),
		ParentEventID:  int64PtrOrNil(event.ParentEventID),
		SchemaVersion:  stringPtrOrNil(event.SchemaVersion), Status: stringPtrOrNil(event.Status),
		OccurredAtUnixNano: &occurredAt, Visibility: stringPtrOrNil(string(event.Visibility)),
		PayloadVersion: stringPtrOrNil(event.PayloadVersion), SnapshotID: stringPtrOrNil(event.SnapshotID),
		TraceID: stringPtrOrNil(event.TraceID), ActionID: stringPtrOrNil(event.ActionID),
		Phase: stringPtrOrNil(event.Phase), Operation: stringPtrOrNil(event.Operation),
		Target: stringPtrOrNil(event.Target), Milestone: stringPtrOrNil(event.Milestone),
		EventType: event.EventType, JournalEventType: &journalEventType,
		Payload: payload, JournalPayload: append([]byte(nil), payload...), CreatedAt: event.CreatedAt,
	}
	if sequence > 0 {
		po.Sequence = &sequence
	}
	return po, nil
}

func journalEventToPOWithBase(
	event *entity.JournalEvent,
	sequence uint64,
	base *entity.RunEvent,
) (*runEventPO, error) {
	po, err := journalEventToPO(event, sequence)
	if err != nil || base == nil {
		return po, err
	}
	if base.ID != event.ID || base.ThreadID != event.ThreadID || base.RunID != event.RunID {
		return nil, fmt.Errorf("base run event and journal projection identity do not match")
	}
	basePO, err := runEventToPO(base)
	if err != nil {
		return nil, err
	}
	po.EventType = basePO.EventType
	po.Payload = basePO.Payload
	po.CreatedAt = basePO.CreatedAt
	return po, nil
}

func journalEventFromPO(po *runEventPO) *entity.JournalEvent {
	eventType := po.EventType
	payload := po.Payload
	if po.JournalEventType != nil && strings.TrimSpace(*po.JournalEventType) != "" && len(po.JournalPayload) > 0 {
		eventType = *po.JournalEventType
		payload = po.JournalPayload
	}
	return &entity.JournalEvent{
		ID: po.ID, ThreadID: po.ThreadID, RunID: po.RunID,
		JournalRunID: int64FromPtr(po.JournalRunID), AttemptID: stringFromPtr(po.AttemptID),
		Sequence: uint64FromPtr(po.Sequence), IdempotencyKey: stringFromPtr(po.IdempotencyKey),
		ParentEventID: int64FromPtr(po.ParentEventID), SchemaVersion: stringFromPtr(po.SchemaVersion),
		Status: stringFromPtr(po.Status), OccurredAtUnixNano: int64FromPtr(po.OccurredAtUnixNano),
		Visibility:     entity.JournalVisibility(stringFromPtr(po.Visibility)),
		PayloadVersion: stringFromPtr(po.PayloadVersion), SnapshotID: stringFromPtr(po.SnapshotID),
		TraceID: stringFromPtr(po.TraceID), ActionID: stringFromPtr(po.ActionID),
		Phase: stringFromPtr(po.Phase), Operation: stringFromPtr(po.Operation),
		Target: stringFromPtr(po.Target), Milestone: stringFromPtr(po.Milestone),
		EventType: eventType, Payload: jsonToString(payload), CreatedAt: po.CreatedAt,
	}
}

func runAttemptToPO(attempt *entity.RunAttempt) *runAttemptPO {
	return &runAttemptPO{
		ID: attempt.ID, ThreadID: attempt.ThreadID,
		JournalRunID: attempt.JournalRunID, ExecutionRunID: attempt.ExecutionRunID,
		AttemptID: attempt.AttemptID, Ordinal: attempt.Ordinal, Status: string(attempt.Status),
		ActiveSlot: cloneUint8Pointer(attempt.ActiveSlot), NextSequence: attempt.NextSequence,
		LastCommittedSequence:  attempt.LastCommittedSequence,
		SourceCheckpointID:     cloneInt64Pointer(attempt.SourceCheckpointID),
		SourceAttemptID:        cloneStringPointer(attempt.SourceAttemptID),
		RecoveryIdempotencyKey: cloneStringPointer(attempt.RecoveryIdempotencyKey),
		EnrollmentVersion:      attempt.EnrollmentVersion, SnapshotsEnabled: attempt.SnapshotsEnabled,
		ProjectionState:      string(attempt.ProjectionState),
		ProjectionDegradedAt: cloneInt64Pointer(attempt.ProjectionDegradedAt),
		TraceID:              cloneStringPointer(attempt.TraceID), TerminalEventID: cloneInt64Pointer(attempt.TerminalEventID),
		CreatedAt: attempt.CreatedAt, UpdatedAt: attempt.UpdatedAt,
		StartedAt: cloneInt64Pointer(attempt.StartedAt), EndedAt: cloneInt64Pointer(attempt.EndedAt),
	}
}

func (po *runAttemptPO) toEntity() *entity.RunAttempt {
	return &entity.RunAttempt{
		ID: po.ID, ThreadID: po.ThreadID,
		JournalRunID: po.JournalRunID, ExecutionRunID: po.ExecutionRunID,
		AttemptID: po.AttemptID, Ordinal: po.Ordinal, Status: entity.RunAttemptStatus(po.Status),
		ActiveSlot: cloneUint8Pointer(po.ActiveSlot), NextSequence: po.NextSequence,
		LastCommittedSequence:  po.LastCommittedSequence,
		SourceCheckpointID:     cloneInt64Pointer(po.SourceCheckpointID),
		SourceAttemptID:        cloneStringPointer(po.SourceAttemptID),
		RecoveryIdempotencyKey: cloneStringPointer(po.RecoveryIdempotencyKey),
		EnrollmentVersion:      po.EnrollmentVersion, SnapshotsEnabled: po.SnapshotsEnabled,
		ProjectionState:      entity.JournalProjectionState(po.ProjectionState),
		ProjectionDegradedAt: cloneInt64Pointer(po.ProjectionDegradedAt),
		TraceID:              cloneStringPointer(po.TraceID), TerminalEventID: cloneInt64Pointer(po.TerminalEventID),
		CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt,
		StartedAt: cloneInt64Pointer(po.StartedAt), EndedAt: cloneInt64Pointer(po.EndedAt),
	}
}

func cloneRunAttempt(attempt *entity.RunAttempt) *entity.RunAttempt {
	if attempt == nil {
		return nil
	}
	clone := *attempt
	clone.ActiveSlot = cloneUint8Pointer(attempt.ActiveSlot)
	clone.SourceCheckpointID = cloneInt64Pointer(attempt.SourceCheckpointID)
	clone.SourceAttemptID = cloneStringPointer(attempt.SourceAttemptID)
	clone.RecoveryIdempotencyKey = cloneStringPointer(attempt.RecoveryIdempotencyKey)
	clone.ProjectionDegradedAt = cloneInt64Pointer(attempt.ProjectionDegradedAt)
	clone.TraceID = cloneStringPointer(attempt.TraceID)
	clone.TerminalEventID = cloneInt64Pointer(attempt.TerminalEventID)
	clone.StartedAt = cloneInt64Pointer(attempt.StartedAt)
	clone.EndedAt = cloneInt64Pointer(attempt.EndedAt)
	return &clone
}

func findRecoveryAttempt(
	db *gorm.DB,
	requested *entity.RunAttempt,
) (*entity.RunAttempt, bool, error) {
	key := strings.TrimSpace(stringFromPtr(requested.RecoveryIdempotencyKey))
	if key == "" {
		return nil, false, nil
	}
	var po runAttemptPO
	err := db.Where(
		"journal_run_id = ? AND recovery_idempotency_key = ?",
		requested.JournalRunID, key,
	).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if po.ExecutionRunID != requested.ExecutionRunID ||
		!equalInt64Pointers(po.SourceCheckpointID, requested.SourceCheckpointID) ||
		!equalStringPointers(po.SourceAttemptID, requested.SourceAttemptID) {
		return nil, false, fmt.Errorf("%w: recovery journal attempt semantics changed", ErrRunIdempotencyConflict)
	}
	return po.toEntity(), true, nil
}

func journalAttemptStatusFromRun(status entity.RunStatus) (entity.RunAttemptStatus, error) {
	switch status {
	case entity.RunStatusPending, entity.RunStatusQueued:
		return entity.RunAttemptStatusPending, nil
	case entity.RunStatusRunning:
		return entity.RunAttemptStatusRunning, nil
	default:
		return "", fmt.Errorf("execution run status %q cannot start a journal attempt", status)
	}
}

func journalAttemptStatusForTerminalRun(
	status entity.RunStatus,
	errorCode string,
) entity.RunAttemptStatus {
	switch status {
	case entity.RunStatusSucceeded:
		return entity.RunAttemptStatusCompleted
	case entity.RunStatusCanceled:
		return entity.RunAttemptStatusCancelled
	case entity.RunStatusFailed:
		if entity.IsJournalTimeoutErrorCode(errorCode) {
			return entity.RunAttemptStatusTimedOut
		}
		return entity.RunAttemptStatusFailed
	default:
		return entity.RunAttemptStatusFailed
	}
}

func normalizePreparedSideEffect(
	ledger *entity.SideEffectLedger,
) (*entity.SideEffectLedger, error) {
	if ledger == nil || ledger.ID <= 0 || ledger.ThreadID <= 0 ||
		ledger.JournalRunID <= 0 || strings.TrimSpace(ledger.AttemptID) == "" ||
		strings.TrimSpace(ledger.IdempotencyKey) == "" ||
		strings.TrimSpace(ledger.ActionKind) == "" {
		return nil, fmt.Errorf("prepared side effect identity is required")
	}
	normalized := cloneSideEffectLedger(ledger)
	normalized.AttemptID = strings.TrimSpace(normalized.AttemptID)
	normalized.IdempotencyKey = strings.TrimSpace(normalized.IdempotencyKey)
	normalized.ActionKind = strings.TrimSpace(normalized.ActionKind)
	normalized.RequestHash = strings.ToLower(strings.TrimSpace(normalized.RequestHash))
	normalized.RequestSummary = strings.TrimSpace(normalized.RequestSummary)
	normalized.CompensationKind = strings.TrimSpace(normalized.CompensationKind)
	if !normalized.ReplayPolicy.Valid() {
		return nil, fmt.Errorf("side effect replay policy is invalid")
	}
	if normalized.Status != entity.SideEffectLedgerStatusPrepared {
		return nil, ErrSideEffectTransitionInvalid
	}
	if !validSideEffectDigest(normalized.RequestHash) {
		return nil, fmt.Errorf("side effect request hash must be a SHA-256 digest")
	}
	if normalized.RequestSummary == "" {
		normalized.RequestSummary = `{}`
	}
	if len(normalized.RequestSummary) > 4096 || !json.Valid([]byte(normalized.RequestSummary)) {
		return nil, fmt.Errorf("side effect request summary must be bounded JSON")
	}
	if normalized.ExternalReferenceDigest != "" || normalized.ResultSnapshotID != "" ||
		normalized.ResultEventID != nil || normalized.CheckpointID != nil ||
		normalized.ResolutionAction != "" || normalized.ResolutionIdempotencyKey != "" ||
		normalized.ResolvedAt != nil ||
		normalized.ExecutingAt != nil || normalized.SucceededAt != nil ||
		normalized.FailedAt != nil || normalized.UnknownAt != nil || normalized.CompensatedAt != nil {
		return nil, ErrSideEffectTransitionInvalid
	}
	if normalized.Version == 0 {
		normalized.Version = 1
	}
	if normalized.Version != 1 {
		return nil, fmt.Errorf("prepared side effect version must be one")
	}
	if normalized.PreparedAt <= 0 {
		normalized.PreparedAt = time.Now().UnixMilli()
	}
	if normalized.CreatedAt <= 0 {
		normalized.CreatedAt = normalized.PreparedAt
	}
	if normalized.UpdatedAt <= 0 {
		normalized.UpdatedAt = normalized.PreparedAt
	}
	return normalized, nil
}

func validSideEffectDigest(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func normalizeSideEffectAuditEvent(
	audit *entity.JournalEvent,
	ledger *entity.SideEffectLedger,
) (*entity.JournalEvent, error) {
	if ledger == nil {
		return nil, fmt.Errorf("side effect ledger is required")
	}
	if audit == nil {
		return nil, fmt.Errorf("side effect transition audit is required")
	}
	candidate := *audit
	if candidate.EventType == "side_effect.audit" {
		candidate.ActionID = ""
		candidate.Phase = ""
		candidate.Operation = ""
		candidate.Target = ""
		candidate.Milestone = ""
	}
	normalized, err := normalizeJournalEvent(&candidate)
	if err != nil {
		return nil, err
	}
	if normalized.Visibility != entity.JournalVisibilityInternal {
		return nil, fmt.Errorf("side effect transition audit must be internal")
	}
	if normalized.ThreadID != ledger.ThreadID ||
		(normalized.JournalRunID != 0 && normalized.JournalRunID != ledger.JournalRunID) ||
		(normalized.AttemptID != "" && normalized.AttemptID != ledger.AttemptID) {
		return nil, ErrJournalParentMismatch
	}
	normalized.JournalRunID = ledger.JournalRunID
	normalized.AttemptID = ledger.AttemptID
	return normalized, nil
}

func appendSideEffectAuditLocked(
	tx *gorm.DB,
	attempt *runAttemptPO,
	audit *entity.JournalEvent,
) (*entity.JournalEvent, error) {
	if tx == nil || attempt == nil || audit == nil {
		return nil, fmt.Errorf("side effect transition audit boundary is required")
	}
	if audit.ThreadID != attempt.ThreadID || audit.RunID != attempt.ExecutionRunID ||
		audit.JournalRunID != attempt.JournalRunID || audit.AttemptID != attempt.AttemptID {
		return nil, ErrJournalParentMismatch
	}
	var replay runEventPO
	err := tx.Where(
		"journal_run_id = ? AND attempt_id = ? AND idempotency_key = ?",
		attempt.JournalRunID, attempt.AttemptID, audit.IdempotencyKey,
	).First(&replay).Error
	if err == nil {
		return journalEventFromPO(&replay), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	po, err := journalEventToPO(audit, 0)
	if err != nil {
		return nil, err
	}
	if err := tx.Create(po).Error; err != nil {
		return nil, err
	}
	return journalEventFromPO(po), nil
}

func lockJournalAttemptByIdentity(
	tx *gorm.DB,
	journalRunID int64,
	attemptID string,
) (*runAttemptPO, error) {
	query := tx.Where(
		"journal_run_id = ? AND attempt_id = ?", journalRunID, strings.TrimSpace(attemptID),
	)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempt runAttemptPO
	err := query.First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrJournalNotEnrolled
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

func findSideEffectLedgerByIdentity(
	db *gorm.DB,
	journalRunID int64,
	attemptID, idempotencyKey string,
) (*entity.SideEffectLedger, bool, error) {
	if journalRunID <= 0 || strings.TrimSpace(attemptID) == "" ||
		strings.TrimSpace(idempotencyKey) == "" {
		return nil, false, nil
	}
	var po sideEffectLedgerPO
	err := db.Where(
		"journal_run_id = ? AND attempt_id = ? AND idempotency_key = ?",
		journalRunID, strings.TrimSpace(attemptID), strings.TrimSpace(idempotencyKey),
	).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return po.toEntity(), true, nil
}

func lockSideEffectLedger(
	tx *gorm.DB,
	journalRunID int64,
	attemptID string,
	ledgerID int64,
) (*sideEffectLedgerPO, error) {
	query := tx.Where(
		"id = ? AND journal_run_id = ? AND attempt_id = ?",
		ledgerID, journalRunID, strings.TrimSpace(attemptID),
	)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var ledger sideEffectLedgerPO
	err := query.First(&ledger).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSideEffectLedgerNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ledger, nil
}

func validateSideEffectReplay(
	stored, requested *entity.SideEffectLedger,
) error {
	if stored == nil || requested == nil ||
		stored.ID != requested.ID || stored.ThreadID != requested.ThreadID ||
		stored.JournalRunID != requested.JournalRunID || stored.AttemptID != requested.AttemptID ||
		stored.IdempotencyKey != requested.IdempotencyKey ||
		stored.ActionKind != requested.ActionKind || stored.ReplayPolicy != requested.ReplayPolicy ||
		stored.RequestHash != requested.RequestHash ||
		stored.RequestSummary != requested.RequestSummary ||
		stored.CompensationKind != requested.CompensationKind {
		return ErrSideEffectLedgerConflict
	}
	return nil
}

func validStandaloneSideEffectTransition(req TransitionSideEffectRequest) bool {
	switch {
	case req.FromStatus == entity.SideEffectLedgerStatusPrepared &&
		req.ToStatus == entity.SideEffectLedgerStatusExecuting:
		return true
	case req.FromStatus == entity.SideEffectLedgerStatusExecuting &&
		req.ToStatus == entity.SideEffectLedgerStatusUnknown:
		return true
	case (req.FromStatus == entity.SideEffectLedgerStatusSucceeded ||
		req.FromStatus == entity.SideEffectLedgerStatusUnknown) &&
		req.ToStatus == entity.SideEffectLedgerStatusCompensated:
		return req.CompensationRegistered && req.CompensationSucceeded
	default:
		return false
	}
}

func sideEffectTransitionUpdates(req TransitionSideEffectRequest) map[string]any {
	updates := map[string]any{"status": string(req.ToStatus)}
	switch req.ToStatus {
	case entity.SideEffectLedgerStatusExecuting:
		updates["executing_at"] = req.OccurredAt
	case entity.SideEffectLedgerStatusUnknown:
		updates["unknown_at"] = req.OccurredAt
	case entity.SideEffectLedgerStatusCompensated:
		updates["compensated_at"] = req.OccurredAt
	}
	if req.ExternalReferenceDigest != "" {
		updates["external_reference_digest"] = strings.ToLower(strings.TrimSpace(req.ExternalReferenceDigest))
	}
	if strings.TrimSpace(req.ResultSnapshotID) != "" {
		updates["result_snapshot_id"] = strings.TrimSpace(req.ResultSnapshotID)
	}
	return updates
}

func applySideEffectTransition(ledger *sideEffectLedgerPO, req TransitionSideEffectRequest) {
	ledger.Status = string(req.ToStatus)
	ledger.Version = req.ExpectedVersion + 1
	ledger.UpdatedAt = req.OccurredAt
	switch req.ToStatus {
	case entity.SideEffectLedgerStatusExecuting:
		ledger.ExecutingAt = cloneInt64Pointer(&req.OccurredAt)
	case entity.SideEffectLedgerStatusUnknown:
		ledger.UnknownAt = cloneInt64Pointer(&req.OccurredAt)
	case entity.SideEffectLedgerStatusCompensated:
		ledger.CompensatedAt = cloneInt64Pointer(&req.OccurredAt)
	}
	if req.ExternalReferenceDigest != "" {
		value := strings.ToLower(strings.TrimSpace(req.ExternalReferenceDigest))
		ledger.ExternalReferenceDigest = &value
	}
	if value := strings.TrimSpace(req.ResultSnapshotID); value != "" {
		ledger.ResultSnapshotID = &value
	}
}

func bindSideEffectEvent(
	event *entity.JournalEvent,
	attempt *runAttemptPO,
	ledger *sideEffectLedgerPO,
) error {
	if event.ThreadID != attempt.ThreadID || event.RunID != attempt.ExecutionRunID ||
		(event.JournalRunID != 0 && event.JournalRunID != attempt.JournalRunID) ||
		(event.AttemptID != "" && event.AttemptID != attempt.AttemptID) {
		return ErrJournalParentMismatch
	}
	event.JournalRunID = attempt.JournalRunID
	event.AttemptID = attempt.AttemptID
	if event.ActionID == "" {
		event.ActionID = ledger.IdempotencyKey
	}
	if event.Operation == "" {
		event.Operation = ledger.ActionKind
	}
	return nil
}

func validateExecutionBoundaryCheckpoint(
	checkpoint *entity.Checkpoint,
	attempt *runAttemptPO,
	lastCommittedSequence uint64,
) error {
	if checkpoint == nil || checkpoint.ID <= 0 || checkpoint.ThreadID != attempt.ThreadID ||
		checkpoint.RunID != attempt.ExecutionRunID {
		return fmt.Errorf("execution boundary checkpoint does not belong to the active attempt")
	}
	var envelope struct {
		SchemaVersion         string `json:"schema_version"`
		AttemptID             string `json:"attempt_id"`
		LastCommittedSequence uint64 `json:"last_committed_sequence"`
		RuntimeState          any    `json:"runtime_state"`
		SideEffectLedger      any    `json:"side_effect_ledger"`
	}
	if err := json.Unmarshal([]byte(checkpoint.ChannelValues), &envelope); err != nil {
		return fmt.Errorf("decode execution boundary checkpoint envelope: %w", err)
	}
	if strings.TrimSpace(envelope.SchemaVersion) == "" ||
		envelope.AttemptID != attempt.AttemptID ||
		envelope.LastCommittedSequence != lastCommittedSequence ||
		envelope.RuntimeState == nil || envelope.SideEffectLedger == nil {
		return fmt.Errorf("execution boundary checkpoint envelope is incomplete")
	}
	return nil
}

func terminalSideEffectUpdates(
	req CommitExecutionBoundaryRequest,
	resultEventID, checkpointID, occurredAt int64,
) map[string]any {
	updates := map[string]any{
		"status":          string(req.Status),
		"version":         req.ExpectedVersion + 1,
		"result_event_id": resultEventID,
		"checkpoint_id":   checkpointID,
		"updated_at":      occurredAt,
	}
	switch req.Status {
	case entity.SideEffectLedgerStatusSucceeded:
		updates["succeeded_at"] = occurredAt
	case entity.SideEffectLedgerStatusFailed:
		updates["failed_at"] = occurredAt
	case entity.SideEffectLedgerStatusUnknown:
		updates["unknown_at"] = occurredAt
	}
	if value := strings.ToLower(strings.TrimSpace(req.ExternalReferenceDigest)); value != "" {
		updates["external_reference_digest"] = value
	}
	if value := strings.TrimSpace(req.ResultSnapshotID); value != "" {
		updates["result_snapshot_id"] = value
	}
	return updates
}

func applyTerminalSideEffect(
	ledger *sideEffectLedgerPO,
	req CommitExecutionBoundaryRequest,
	resultEventID, checkpointID, occurredAt int64,
) {
	ledger.Status = string(req.Status)
	ledger.Version = req.ExpectedVersion + 1
	ledger.ResultEventID = cloneInt64Pointer(&resultEventID)
	ledger.CheckpointID = cloneInt64Pointer(&checkpointID)
	ledger.UpdatedAt = occurredAt
	switch req.Status {
	case entity.SideEffectLedgerStatusSucceeded:
		ledger.SucceededAt = cloneInt64Pointer(&occurredAt)
	case entity.SideEffectLedgerStatusFailed:
		ledger.FailedAt = cloneInt64Pointer(&occurredAt)
	case entity.SideEffectLedgerStatusUnknown:
		ledger.UnknownAt = cloneInt64Pointer(&occurredAt)
	}
	if value := strings.ToLower(strings.TrimSpace(req.ExternalReferenceDigest)); value != "" {
		ledger.ExternalReferenceDigest = &value
	}
	if value := strings.TrimSpace(req.ResultSnapshotID); value != "" {
		ledger.ResultSnapshotID = &value
	}
}

func loadExecutionBoundaryReplay(
	tx *gorm.DB,
	ledger *sideEffectLedgerPO,
	req CommitExecutionBoundaryRequest,
) (*CommitExecutionBoundaryResult, error) {
	if entity.SideEffectLedgerStatus(ledger.Status) != req.Status ||
		ledger.Version != req.ExpectedVersion+1 || ledger.ResultEventID == nil || ledger.CheckpointID == nil ||
		stringFromPtr(ledger.ExternalReferenceDigest) != strings.TrimSpace(req.ExternalReferenceDigest) ||
		stringFromPtr(ledger.ResultSnapshotID) != strings.TrimSpace(req.ResultSnapshotID) {
		return nil, ErrSideEffectLedgerConflict
	}
	var event runEventPO
	if err := tx.Where("id = ?", *ledger.ResultEventID).First(&event).Error; err != nil {
		return nil, err
	}
	var checkpoint checkpointPO
	if err := tx.Where("id = ?", *ledger.CheckpointID).First(&checkpoint).Error; err != nil {
		return nil, err
	}
	projected := journalEventFromPO(&event)
	return &CommitExecutionBoundaryResult{
		Ledger: ledger.toEntity(), Event: projected, Checkpoint: checkpoint.toEntity(),
		LastCommittedSequence: projected.Sequence, Replayed: true,
	}, nil
}

func sideEffectLedgerToPO(ledger *entity.SideEffectLedger) *sideEffectLedgerPO {
	if ledger == nil {
		return nil
	}
	return &sideEffectLedgerPO{
		ID: ledger.ID, ThreadID: ledger.ThreadID, JournalRunID: ledger.JournalRunID,
		AttemptID: ledger.AttemptID, IdempotencyKey: ledger.IdempotencyKey,
		ActionKind: ledger.ActionKind, ReplayPolicy: string(ledger.ReplayPolicy),
		Status: string(ledger.Status), RequestHash: ledger.RequestHash,
		RequestSummary:           []byte(ledger.RequestSummary),
		ExternalReferenceDigest:  sideEffectOptionalString(ledger.ExternalReferenceDigest),
		ResultSnapshotID:         sideEffectOptionalString(ledger.ResultSnapshotID),
		ResultEventID:            cloneInt64Pointer(ledger.ResultEventID),
		CheckpointID:             cloneInt64Pointer(ledger.CheckpointID),
		CompensationKind:         sideEffectOptionalString(ledger.CompensationKind),
		ResolutionAction:         sideEffectOptionalString(string(ledger.ResolutionAction)),
		ResolutionIdempotencyKey: sideEffectOptionalString(ledger.ResolutionIdempotencyKey),
		ResolvedAt:               cloneInt64Pointer(ledger.ResolvedAt),
		Version:                  ledger.Version, PreparedAt: ledger.PreparedAt,
		ExecutingAt: cloneInt64Pointer(ledger.ExecutingAt),
		SucceededAt: cloneInt64Pointer(ledger.SucceededAt), FailedAt: cloneInt64Pointer(ledger.FailedAt),
		UnknownAt: cloneInt64Pointer(ledger.UnknownAt), CompensatedAt: cloneInt64Pointer(ledger.CompensatedAt),
		CreatedAt: ledger.CreatedAt, UpdatedAt: ledger.UpdatedAt,
	}
}

func (po *sideEffectLedgerPO) toEntity() *entity.SideEffectLedger {
	if po == nil {
		return nil
	}
	return &entity.SideEffectLedger{
		ID: po.ID, ThreadID: po.ThreadID, JournalRunID: po.JournalRunID,
		AttemptID: po.AttemptID, IdempotencyKey: po.IdempotencyKey,
		ActionKind: po.ActionKind, ReplayPolicy: entity.SideEffectReplayPolicy(po.ReplayPolicy),
		Status: entity.SideEffectLedgerStatus(po.Status), RequestHash: po.RequestHash,
		RequestSummary:           jsonToString(po.RequestSummary),
		ExternalReferenceDigest:  stringFromPtr(po.ExternalReferenceDigest),
		ResultSnapshotID:         stringFromPtr(po.ResultSnapshotID),
		ResultEventID:            cloneInt64Pointer(po.ResultEventID),
		CheckpointID:             cloneInt64Pointer(po.CheckpointID),
		CompensationKind:         stringFromPtr(po.CompensationKind),
		ResolutionAction:         entity.SideEffectResolutionAction(stringFromPtr(po.ResolutionAction)),
		ResolutionIdempotencyKey: stringFromPtr(po.ResolutionIdempotencyKey),
		ResolvedAt:               cloneInt64Pointer(po.ResolvedAt),
		Version:                  po.Version, PreparedAt: po.PreparedAt,
		ExecutingAt: cloneInt64Pointer(po.ExecutingAt),
		SucceededAt: cloneInt64Pointer(po.SucceededAt), FailedAt: cloneInt64Pointer(po.FailedAt),
		UnknownAt: cloneInt64Pointer(po.UnknownAt), CompensatedAt: cloneInt64Pointer(po.CompensatedAt),
		CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt,
	}
}

func cloneSideEffectLedger(ledger *entity.SideEffectLedger) *entity.SideEffectLedger {
	if ledger == nil {
		return nil
	}
	clone := *ledger
	clone.ResultEventID = cloneInt64Pointer(ledger.ResultEventID)
	clone.CheckpointID = cloneInt64Pointer(ledger.CheckpointID)
	clone.ExecutingAt = cloneInt64Pointer(ledger.ExecutingAt)
	clone.SucceededAt = cloneInt64Pointer(ledger.SucceededAt)
	clone.FailedAt = cloneInt64Pointer(ledger.FailedAt)
	clone.UnknownAt = cloneInt64Pointer(ledger.UnknownAt)
	clone.CompensatedAt = cloneInt64Pointer(ledger.CompensatedAt)
	clone.ResolvedAt = cloneInt64Pointer(ledger.ResolvedAt)
	return &clone
}

func sideEffectOptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func isJournalRootRun(run *runPO) bool {
	return run != nil && run.ParentRunID == 0 &&
		(run.RunKind == "" || run.RunKind == string(entity.RunKindTask))
}

func isLegacyTerminalRunStatus(status entity.RunStatus) bool {
	switch status {
	case entity.RunStatusSucceeded, entity.RunStatusFailed,
		entity.RunStatusCanceled:
		return true
	default:
		return false
	}
}

func projectSafeLegacyJournalEvent(po *runEventPO) (*entity.JournalEvent, error) {
	statusByType := map[string]string{
		"run.created":   string(entity.RunAttemptStatusPending),
		"run.queued":    string(entity.RunAttemptStatusPending),
		"run.started":   string(entity.RunAttemptStatusRunning),
		"run.completed": string(entity.RunAttemptStatusCompleted),
		"run.succeeded": string(entity.RunAttemptStatusCompleted),
		"run.failed":    string(entity.RunAttemptStatusFailed),
		"run.canceled":  string(entity.RunAttemptStatusCancelled),
		"run.cancelled": string(entity.RunAttemptStatusCancelled),
		"run.timed_out": string(entity.RunAttemptStatusTimedOut),
	}
	status, ok := statusByType[po.EventType]
	if !ok {
		return nil, ErrJournalUnsafeLegacyEvent
	}
	payload, err := json.Marshal(map[string]any{
		"type": "legacy_run_lifecycle",
		"data": map[string]string{"event_type": po.EventType, "status": status},
	})
	if err != nil {
		return nil, err
	}
	return &entity.JournalEvent{
		ID: po.ID, ThreadID: po.ThreadID, RunID: po.RunID, JournalRunID: po.RunID,
		AttemptID: fmt.Sprintf("legacy-%d", po.RunID), Sequence: uint64(po.ID),
		Status: status, Visibility: entity.JournalVisibilityUser,
		EventType: "run.lifecycle", Payload: string(payload), CreatedAt: po.CreatedAt,
	}, nil
}

func journalColumnsEmpty(po *runEventPO) bool {
	return po != nil && po.JournalRunID == nil && po.AttemptID == nil && po.Sequence == nil &&
		po.IdempotencyKey == nil && po.ParentEventID == nil && po.SchemaVersion == nil &&
		po.Status == nil && po.OccurredAtUnixNano == nil && po.Visibility == nil &&
		po.PayloadVersion == nil && po.SnapshotID == nil && po.TraceID == nil &&
		po.ActionID == nil && po.Phase == nil && po.Operation == nil &&
		po.Target == nil && po.Milestone == nil && po.JournalEventType == nil && len(po.JournalPayload) == 0
}

func uint64FromPtr(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneUint8Pointer(value *uint8) *uint8 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func equalInt64Pointers(left, right *int64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func equalStringPointers(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}
