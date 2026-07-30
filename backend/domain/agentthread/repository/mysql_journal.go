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
	if err := ensureJournalAttemptRunning(tx, attempt, event.CreatedAt); err != nil {
		return nil, err
	}
	parentID, err := resolvePublicJournalParent(tx, attempt, event.ParentEventID)
	if err != nil {
		return nil, err
	}
	event.ParentEventID = parentID

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
