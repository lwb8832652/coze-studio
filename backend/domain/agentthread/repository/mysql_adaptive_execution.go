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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const adaptiveExecutionCheckpointSchemaVersion = "workbench-adaptive-boundary.v1"

func NewAdaptiveExecutionRepository(db *gorm.DB) AdaptiveExecutionRepository {
	return &threadRepository{db: db}
}

type adaptiveExecutionCheckpointMetadata struct {
	SchemaVersion       string `json:"schema_version"`
	EventID             int64  `json:"event_id"`
	EventSequence       uint64 `json:"event_sequence"`
	JournalRunID        int64  `json:"journal_run_id"`
	AttemptID           string `json:"attempt_id"`
	PlanScopeRunID      int64  `json:"plan_scope_run_id"`
	PlanRevision        int64  `json:"plan_revision"`
	ItemFingerprint     string `json:"item_fingerprint"`
	ExecutionRunID      int64  `json:"execution_run_id"`
	ExecutionGeneration uint64 `json:"execution_generation"`
}

type adaptiveExecutionLockedItem struct {
	mutation AdaptivePlanItemMutation
	existing *agentRunPlanItemPO
	next     *entity.AgentRunPlanItem
}

type adaptiveExecutionLockedState struct {
	run        *runPO
	attempt    *runAttemptPO
	plan       *agentRunPlanPO
	event      *runEventPO
	checkpoint *checkpointPO
	items      []adaptiveExecutionLockedItem
	sequence   uint64
}

type adaptiveExecutionFingerprintItem struct {
	ID          int64           `json:"id"`
	RunID       int64           `json:"run_id"`
	TaskID      int64           `json:"task_id"`
	Subject     string          `json:"subject"`
	Description string          `json:"description"`
	Status      string          `json:"status"`
	ActiveForm  string          `json:"active_form"`
	Owner       string          `json:"owner"`
	Blocks      json.RawMessage `json:"blocks"`
	BlockedBy   json.RawMessage `json:"blocked_by"`
	Metadata    json.RawMessage `json:"metadata"`
	Active      bool            `json:"active"`
	Version     int64           `json:"version"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
}

func adaptiveExecutionPlanItemFingerprint(items []*entity.AgentRunPlanItem) (string, error) {
	sorted := append([]*entity.AgentRunPlanItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i] == nil {
			return false
		}
		if sorted[j] == nil {
			return true
		}
		return sorted[i].TaskID < sorted[j].TaskID
	})
	fingerprintItems := make([]adaptiveExecutionFingerprintItem, 0, len(sorted))
	for _, item := range sorted {
		if item == nil {
			return "", fmt.Errorf("adaptive execution fingerprint item is required")
		}
		blocks, err := canonicalAdaptiveExecutionJSON(item.Blocks)
		if err != nil {
			return "", fmt.Errorf("canonicalize blocks: %w", err)
		}
		blockedBy, err := canonicalAdaptiveExecutionJSON(item.BlockedBy)
		if err != nil {
			return "", fmt.Errorf("canonicalize blocked_by: %w", err)
		}
		metadata, err := canonicalAdaptiveExecutionJSON(item.Metadata)
		if err != nil {
			return "", fmt.Errorf("canonicalize metadata: %w", err)
		}
		fingerprintItems = append(fingerprintItems, adaptiveExecutionFingerprintItem{
			ID: item.ID, RunID: item.RunID, TaskID: item.TaskID,
			Subject: item.Subject, Description: item.Description, Status: string(item.Status),
			ActiveForm: item.ActiveForm, Owner: item.Owner,
			Blocks: blocks, BlockedBy: blockedBy, Metadata: metadata,
			Active: item.Active, Version: item.Version,
			CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	encoded, err := json.Marshal(fingerprintItems)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func canonicalAdaptiveExecutionJSON(raw string) (json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values are not allowed")
		}
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func lockAdaptiveExecutionRun(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
) (*runPO, error) {
	query := tx.Where("id = ?", req.ExecutionRunID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run runPO
	err := query.First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: run %d not found", ErrRunLeaseLost, req.ExecutionRunID)
	}
	if err != nil {
		return nil, err
	}
	if run.ThreadID != req.ThreadID || entity.RunStatus(run.Status) != entity.RunStatusRunning ||
		run.ExecutionGeneration != req.Generation ||
		run.LeaseOwner == nil || *run.LeaseOwner != req.LeaseOwner ||
		run.LeaseToken == nil || *run.LeaseToken != req.LeaseToken ||
		run.LeaseExpiresAt == nil || *run.LeaseExpiresAt <= req.Now {
		return nil, fmt.Errorf("%w: run %d identity drift", ErrRunLeaseLost, req.ExecutionRunID)
	}
	if run.CancelRequestedAt != nil {
		return nil, fmt.Errorf("%w: run %d has cancellation requested", ErrRunCanceled, req.ExecutionRunID)
	}
	return &run, nil
}

func lockAdaptiveExecutionAttempt(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
) (*runAttemptPO, error) {
	query := tx.Where(
		"journal_run_id = ? AND attempt_id = ?", req.JournalRunID, req.AttemptID,
	)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempt runAttemptPO
	err := query.First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf(
			"%w: attempt %s not found", ErrAdaptiveExecutionAttemptConflict, req.AttemptID,
		)
	}
	if err != nil {
		return nil, err
	}
	if attempt.ThreadID != req.ThreadID || attempt.ExecutionRunID != req.ExecutionRunID ||
		!entity.RunAttemptStatus(attempt.Status).IsActive() || attempt.ActiveSlot == nil {
		return nil, fmt.Errorf(
			"%w: attempt %s identity drift", ErrAdaptiveExecutionAttemptConflict, req.AttemptID,
		)
	}
	return &attempt, nil
}

func (r *threadRepository) CommitAdaptiveExecutionBoundary(
	ctx context.Context,
	req CommitAdaptiveExecutionBoundaryRequest,
) (*CommitAdaptiveExecutionBoundaryResult, error) {
	if err := validateAdaptiveExecutionMutationRequest(req); err != nil {
		return nil, err
	}

	var committed *CommitAdaptiveExecutionBoundaryResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		run, err := lockAdaptiveExecutionRun(tx, req)
		if err != nil {
			return err
		}
		attempt, err := lockAdaptiveExecutionAttempt(tx, req)
		if err != nil {
			return err
		}
		if err := validateAdaptiveExecutionTargetAttemptCursor(attempt); err != nil {
			return err
		}
		planScopeRunID, err := lockAdaptiveExecutionLineage(tx, req, attempt)
		if err != nil {
			return err
		}
		if planScopeRunID != req.PlanMutation.PlanScopeRunID {
			return fmt.Errorf("%w: inherited plan scope drift", ErrAdaptiveExecutionLineageConflict)
		}

		scopeRun, err := lockAdaptiveExecutionScopeRun(tx, req.PlanMutation.PlanScopeRunID)
		if err != nil {
			return err
		}
		plan, err := lockAdaptiveExecutionPlan(tx, req.PlanMutation.PlanScopeRunID)
		if err != nil {
			return err
		}
		if err := validateAdaptiveExecutionPlanScope(run, scopeRun, plan, req.PlanMutation); err != nil {
			return err
		}
		lockedItems, err := lockAdaptiveExecutionPlanItems(tx, plan, req.PlanMutation, req.Now)
		if err != nil {
			return err
		}
		nextItems := make([]*entity.AgentRunPlanItem, 0, len(lockedItems))
		for _, item := range lockedItems {
			nextItems = append(nextItems, item.next)
		}
		fingerprint, err := adaptiveExecutionPlanItemFingerprint(nextItems)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}

		event, err := runEventToPO(req.Event)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}
		sequence := attempt.NextSequence
		event.JournalRunID = adaptiveExecutionInt64Pointer(req.JournalRunID)
		event.AttemptID = adaptiveExecutionStringPointer(req.AttemptID)
		event.Sequence = adaptiveExecutionUint64Pointer(sequence)
		event.IdempotencyKey = adaptiveExecutionStringPointer(req.IdempotencyKey)

		checkpointEntity := *req.Checkpoint
		checkpointEntity.Metadata, err = mergeAdaptiveExecutionCheckpointMetadata(
			req.Checkpoint.Metadata,
			adaptiveExecutionCheckpointMetadata{
				SchemaVersion: adaptiveExecutionCheckpointSchemaVersion,
				EventID:       event.ID, EventSequence: sequence,
				JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
				PlanScopeRunID:  req.PlanMutation.PlanScopeRunID,
				PlanRevision:    req.PlanMutation.NextRevision,
				ItemFingerprint: fingerprint,
				ExecutionRunID:  run.ID, ExecutionGeneration: run.ExecutionGeneration,
			},
		)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}
		checkpoint, err := checkpointToPO(&checkpointEntity)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}

		state := &adaptiveExecutionLockedState{
			run: run, attempt: attempt, plan: plan, event: event, checkpoint: checkpoint,
			items: lockedItems, sequence: sequence,
		}
		if err := commitAdaptiveExecutionMutationLocked(tx, req, state); err != nil {
			return err
		}
		committed, err = readAdaptiveExecutionBoundaryResult(tx, state)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return committed, nil
}

func readAdaptiveExecutionBoundaryResult(
	tx *gorm.DB,
	state *adaptiveExecutionLockedState,
) (*CommitAdaptiveExecutionBoundaryResult, error) {
	var event runEventPO
	if err := tx.Where("id = ?", state.event.ID).First(&event).Error; err != nil {
		return nil, err
	}
	var checkpoint checkpointPO
	if err := tx.Where("id = ?", state.checkpoint.ID).First(&checkpoint).Error; err != nil {
		return nil, err
	}
	var plan agentRunPlanPO
	if err := tx.Where("run_id = ?", state.plan.RunID).First(&plan).Error; err != nil {
		return nil, err
	}
	itemIDs := make([]int64, 0, len(state.items))
	for _, item := range state.items {
		itemIDs = append(itemIDs, item.next.ID)
	}
	var itemRows []agentRunPlanItemPO
	if err := tx.Where("id IN ?", itemIDs).Order("task_id ASC").Find(&itemRows).Error; err != nil {
		return nil, err
	}
	if len(itemRows) != len(itemIDs) {
		return nil, fmt.Errorf("adaptive execution item readback is incomplete")
	}
	items := make([]*entity.AgentRunPlanItem, 0, len(itemRows))
	for index := range itemRows {
		items = append(items, itemRows[index].toEntity())
	}
	var attempt runAttemptPO
	if err := tx.Where("id = ?", state.attempt.ID).First(&attempt).Error; err != nil {
		return nil, err
	}
	if attempt.LastCommittedSequence != state.sequence {
		return nil, fmt.Errorf("%w: attempt sequence readback drift", ErrAdaptiveExecutionSequenceConflict)
	}
	return &CommitAdaptiveExecutionBoundaryResult{
		Event: event.toEntity(), Checkpoint: checkpoint.toEntity(), Plan: plan.toEntity(),
		Items: items, LastCommittedSequence: attempt.LastCommittedSequence,
	}, nil
}

func validateAdaptiveExecutionTargetAttemptCursor(attempt *runAttemptPO) error {
	if attempt == nil || attempt.ActiveSlot == nil || *attempt.ActiveSlot != 1 {
		return fmt.Errorf("%w: target attempt active slot drift", ErrAdaptiveExecutionAttemptConflict)
	}
	if attempt.NextSequence == 0 || attempt.NextSequence == math.MaxUint64 ||
		attempt.LastCommittedSequence >= attempt.NextSequence {
		return fmt.Errorf("%w: target attempt cursor drift", ErrAdaptiveExecutionSequenceConflict)
	}
	return nil
}

func lockAdaptiveExecutionLineage(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
	attempt *runAttemptPO,
) (int64, error) {
	if attempt == nil {
		return 0, fmt.Errorf("%w: target attempt is required", ErrAdaptiveExecutionLineageConflict)
	}
	sourceAttemptPresent := attempt.SourceAttemptID != nil
	sourceCheckpointPresent := attempt.SourceCheckpointID != nil
	if sourceAttemptPresent != sourceCheckpointPresent {
		return 0, fmt.Errorf("%w: partial recovery source", ErrAdaptiveExecutionLineageConflict)
	}
	if !sourceAttemptPresent {
		if req.PlanMutation.PlanScopeRunID != req.ExecutionRunID || req.Checkpoint.ParentCheckpointID != 0 {
			return 0, fmt.Errorf("%w: initial lineage drift", ErrAdaptiveExecutionLineageConflict)
		}
		return req.ExecutionRunID, nil
	}
	if *attempt.SourceAttemptID == attempt.AttemptID || *attempt.SourceCheckpointID == req.Checkpoint.ID ||
		req.Checkpoint.ParentCheckpointID != *attempt.SourceCheckpointID {
		return 0, fmt.Errorf("%w: recovery source self-reference or parent drift", ErrAdaptiveExecutionLineageConflict)
	}

	sourceAttempt, err := lockAdaptiveExecutionSourceAttempt(tx, req.JournalRunID, *attempt.SourceAttemptID)
	if err != nil {
		return 0, err
	}
	if sourceAttempt.ThreadID != req.ThreadID || sourceAttempt.JournalRunID != req.JournalRunID ||
		sourceAttempt.AttemptID != *attempt.SourceAttemptID {
		return 0, fmt.Errorf("%w: source attempt identity drift", ErrAdaptiveExecutionLineageConflict)
	}
	sourceCheckpoint, err := lockAdaptiveExecutionSourceCheckpoint(tx, *attempt.SourceCheckpointID)
	if err != nil {
		return 0, err
	}
	if sourceCheckpoint.ThreadID != req.ThreadID || sourceCheckpoint.RunID != sourceAttempt.ExecutionRunID ||
		sourceCheckpoint.RuntimeDeletedAt != 0 {
		return 0, fmt.Errorf("%w: source checkpoint identity drift", ErrAdaptiveExecutionLineageConflict)
	}
	metadata, err := decodeAdaptiveExecutionCheckpointMetadata(sourceCheckpoint.Metadata)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrAdaptiveExecutionLineageConflict, err)
	}
	if metadata.JournalRunID != sourceAttempt.JournalRunID ||
		metadata.AttemptID != sourceAttempt.AttemptID ||
		metadata.ExecutionRunID != sourceAttempt.ExecutionRunID ||
		metadata.EventSequence > sourceAttempt.LastCommittedSequence ||
		metadata.PlanScopeRunID != req.PlanMutation.PlanScopeRunID ||
		metadata.PlanRevision != req.PlanMutation.ExpectedRevision {
		return 0, fmt.Errorf("%w: source checkpoint metadata drift", ErrAdaptiveExecutionLineageConflict)
	}
	sourceEvent, err := lockAdaptiveExecutionSourceEvent(tx, metadata.EventID)
	if err != nil {
		return 0, err
	}
	if sourceEvent.ThreadID != req.ThreadID || sourceEvent.RunID != sourceAttempt.ExecutionRunID ||
		sourceEvent.JournalRunID == nil || *sourceEvent.JournalRunID != sourceAttempt.JournalRunID ||
		sourceEvent.AttemptID == nil || *sourceEvent.AttemptID != sourceAttempt.AttemptID ||
		sourceEvent.Sequence == nil || *sourceEvent.Sequence != metadata.EventSequence {
		return 0, fmt.Errorf("%w: source event identity drift", ErrAdaptiveExecutionLineageConflict)
	}
	sourceRun, err := lockAdaptiveExecutionSourceRun(tx, sourceAttempt.ExecutionRunID)
	if err != nil {
		return 0, err
	}
	if sourceRun.ThreadID != req.ThreadID || sourceRun.ExecutionGeneration != metadata.ExecutionGeneration {
		return 0, fmt.Errorf("%w: source execution generation drift", ErrAdaptiveExecutionLineageConflict)
	}
	return metadata.PlanScopeRunID, nil
}

func lockAdaptiveExecutionSourceAttempt(
	tx *gorm.DB,
	journalRunID int64,
	attemptID string,
) (*runAttemptPO, error) {
	query := tx.Where("journal_run_id = ? AND attempt_id = ?", journalRunID, attemptID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempt runAttemptPO
	if err := query.First(&attempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: source attempt not found", ErrAdaptiveExecutionLineageConflict)
		}
		return nil, err
	}
	return &attempt, nil
}

func lockAdaptiveExecutionSourceCheckpoint(tx *gorm.DB, checkpointID int64) (*checkpointPO, error) {
	query := tx.Where("id = ?", checkpointID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var checkpoint checkpointPO
	if err := query.First(&checkpoint).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: source checkpoint not found", ErrAdaptiveExecutionLineageConflict)
		}
		return nil, err
	}
	return &checkpoint, nil
}

func decodeAdaptiveExecutionCheckpointMetadata(raw []byte) (*adaptiveExecutionCheckpointMetadata, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
		return nil, fmt.Errorf("checkpoint metadata envelope is invalid")
	}
	encoded, exists := envelope["adaptive_execution"]
	if !exists {
		return nil, fmt.Errorf("adaptive execution checkpoint metadata is missing")
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var metadata adaptiveExecutionCheckpointMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("adaptive execution checkpoint metadata is invalid: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("adaptive execution checkpoint metadata has trailing content")
	}
	if metadata.SchemaVersion != adaptiveExecutionCheckpointSchemaVersion || metadata.EventID <= 0 ||
		metadata.EventSequence == 0 || metadata.JournalRunID <= 0 || strings.TrimSpace(metadata.AttemptID) == "" ||
		metadata.PlanScopeRunID <= 0 || metadata.PlanRevision <= 0 ||
		!validAdaptiveExecutionFingerprint(metadata.ItemFingerprint) ||
		metadata.ExecutionRunID <= 0 || metadata.ExecutionGeneration == 0 {
		return nil, fmt.Errorf("adaptive execution checkpoint metadata fields are invalid")
	}
	return &metadata, nil
}

func validAdaptiveExecutionFingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func lockAdaptiveExecutionSourceEvent(tx *gorm.DB, eventID int64) (*runEventPO, error) {
	query := tx.Where("id = ?", eventID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var event runEventPO
	if err := query.First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: source event not found", ErrAdaptiveExecutionLineageConflict)
		}
		return nil, err
	}
	return &event, nil
}

func lockAdaptiveExecutionSourceRun(tx *gorm.DB, runID int64) (*runPO, error) {
	query := tx.Where("id = ?", runID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run runPO
	if err := query.First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: source run not found", ErrAdaptiveExecutionLineageConflict)
		}
		return nil, err
	}
	return &run, nil
}

func lockAdaptiveExecutionScopeRun(tx *gorm.DB, runID int64) (*runPO, error) {
	query := tx.Where("id = ?", runID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run runPO
	if err := query.First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: plan scope run %d not found", ErrAdaptiveExecutionPlanScopeConflict, runID)
		}
		return nil, err
	}
	return &run, nil
}

func lockAdaptiveExecutionPlan(tx *gorm.DB, runID int64) (*agentRunPlanPO, error) {
	query := tx.Where("run_id = ?", runID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var plan agentRunPlanPO
	if err := query.First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: plan %d not found", ErrAdaptiveExecutionPlanScopeConflict, runID)
		}
		return nil, err
	}
	return &plan, nil
}

func validateAdaptiveExecutionPlanScope(
	targetRun *runPO,
	scopeRun *runPO,
	plan *agentRunPlanPO,
	mutation *AdaptivePlanMutation,
) error {
	if targetRun == nil || scopeRun == nil || plan == nil || mutation == nil ||
		plan.RunID != mutation.PlanScopeRunID || scopeRun.ID != mutation.PlanScopeRunID ||
		plan.ThreadID != scopeRun.ThreadID || scopeRun.ThreadID != targetRun.ThreadID ||
		plan.SpaceID != scopeRun.SpaceID || scopeRun.SpaceID != targetRun.SpaceID ||
		plan.UserID != scopeRun.CreatorID || scopeRun.CreatorID != targetRun.CreatorID {
		return fmt.Errorf("%w: plan scope identity drift", ErrAdaptiveExecutionPlanScopeConflict)
	}
	if plan.Revision != mutation.ExpectedRevision {
		return fmt.Errorf("%w: expected revision %d, got %d", ErrAdaptiveExecutionPlanRevisionConflict, mutation.ExpectedRevision, plan.Revision)
	}
	return nil
}

func lockAdaptiveExecutionPlanItems(
	tx *gorm.DB,
	plan *agentRunPlanPO,
	mutation *AdaptivePlanMutation,
	now int64,
) ([]adaptiveExecutionLockedItem, error) {
	mutations := append([]AdaptivePlanItemMutation(nil), mutation.Items...)
	sort.Slice(mutations, func(i, j int) bool {
		return mutations[i].NextItem.TaskID < mutations[j].NextItem.TaskID
	})
	locked := make([]adaptiveExecutionLockedItem, 0, len(mutations))
	for _, itemMutation := range mutations {
		candidate := itemMutation.NextItem
		if candidate.TaskID > plan.HighWatermark {
			return nil, fmt.Errorf("%w: task %d exceeds high watermark", ErrAdaptiveExecutionPlanItemVersionConflict, candidate.TaskID)
		}
		if itemMutation.ExpectedVersion == 0 {
			if err := ensureAdaptiveExecutionPlanItemAbsent(tx, candidate); err != nil {
				return nil, err
			}
			next := *candidate
			next.CreatedAt = now
			next.UpdatedAt = now
			locked = append(locked, adaptiveExecutionLockedItem{mutation: itemMutation, next: &next})
			continue
		}

		existing, err := lockAdaptiveExecutionPlanItem(tx, candidate.ID)
		if err != nil {
			return nil, err
		}
		if existing.RunID != candidate.RunID || existing.TaskID != candidate.TaskID ||
			existing.Version != itemMutation.ExpectedVersion {
			return nil, fmt.Errorf("%w: plan item %d identity or version drift", ErrAdaptiveExecutionPlanItemVersionConflict, candidate.ID)
		}
		next := *candidate
		next.CreatedAt = existing.CreatedAt
		next.UpdatedAt = now
		locked = append(locked, adaptiveExecutionLockedItem{
			mutation: itemMutation, existing: existing, next: &next,
		})
	}
	return locked, nil
}

func ensureAdaptiveExecutionPlanItemAbsent(tx *gorm.DB, candidate *entity.AgentRunPlanItem) error {
	for _, condition := range []struct {
		query string
		args  []any
	}{
		{query: "id = ?", args: []any{candidate.ID}},
		{query: "run_id = ? AND task_id = ?", args: []any{candidate.RunID, candidate.TaskID}},
	} {
		query := tx.Where(condition.query, condition.args...)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var row agentRunPlanItemPO
		err := query.First(&row).Error
		switch {
		case err == nil:
			return fmt.Errorf("%w: plan item already exists", ErrAdaptiveExecutionPlanItemVersionConflict)
		case errors.Is(err, gorm.ErrRecordNotFound):
			continue
		default:
			return err
		}
	}
	return nil
}

func lockAdaptiveExecutionPlanItem(tx *gorm.DB, id int64) (*agentRunPlanItemPO, error) {
	query := tx.Where("id = ?", id)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row agentRunPlanItemPO
	if err := query.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: plan item %d not found", ErrAdaptiveExecutionPlanItemVersionConflict, id)
		}
		return nil, err
	}
	return &row, nil
}

func mergeAdaptiveExecutionCheckpointMetadata(
	raw string,
	metadata adaptiveExecutionCheckpointMetadata,
) (string, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil || object == nil {
		return "", fmt.Errorf("checkpoint metadata must be a JSON object")
	}
	if _, exists := object["adaptive_execution"]; exists {
		return "", fmt.Errorf("checkpoint metadata contains reserved namespace")
	}
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	object["adaptive_execution"] = encodedMetadata
	encoded, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func commitAdaptiveExecutionMutationLocked(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
	state *adaptiveExecutionLockedState,
) error {
	if tx == nil || state == nil || state.attempt == nil || state.plan == nil ||
		state.event == nil || state.checkpoint == nil || state.sequence == 0 {
		return fmt.Errorf("%w: locked mutation state is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if err := createBaseRunEvent(tx, state.event); err != nil {
		return err
	}
	if err := tx.Create(state.checkpoint).Error; err != nil {
		if isAdaptiveExecutionCheckpointPrimaryKeyConflict(err) {
			return fmt.Errorf("%w: checkpoint %d already exists", ErrAdaptiveExecutionCheckpointConflict, state.checkpoint.ID)
		}
		return err
	}
	for _, item := range state.items {
		po, err := agentRunPlanItemToPO(item.next)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}
		if item.mutation.ExpectedVersion == 0 {
			if err := tx.Create(po).Error; err != nil {
				if isAdaptiveExecutionDuplicateError(err) {
					return fmt.Errorf("%w: plan item create conflict", ErrAdaptiveExecutionPlanItemVersionConflict)
				}
				return err
			}
			continue
		}
		update := tx.Model(&agentRunPlanItemPO{}).
			Where(
				"id = ? AND run_id = ? AND task_id = ? AND version = ?",
				po.ID, po.RunID, po.TaskID, item.mutation.ExpectedVersion,
			).
			Updates(map[string]any{
				"subject": po.Subject, "description": po.Description, "status": po.Status,
				"active_form": po.ActiveForm, "owner": po.Owner,
				"blocks": po.Blocks, "blocked_by": po.BlockedBy, "metadata": po.Metadata,
				"active": po.Active, "version": po.Version, "updated_at": po.UpdatedAt,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("%w: plan item %d CAS failed", ErrAdaptiveExecutionPlanItemVersionConflict, po.ID)
		}
	}

	planUpdate := tx.Model(&agentRunPlanPO{}).
		Where("run_id = ? AND revision = ?", state.plan.RunID, req.PlanMutation.ExpectedRevision).
		Updates(map[string]any{"revision": req.PlanMutation.NextRevision, "updated_at": req.Now})
	if planUpdate.Error != nil {
		return planUpdate.Error
	}
	if planUpdate.RowsAffected != 1 {
		return fmt.Errorf("%w: plan revision CAS failed", ErrAdaptiveExecutionPlanRevisionConflict)
	}

	attemptUpdate := tx.Model(&runAttemptPO{}).
		Where(
			"id = ? AND active_slot = ? AND next_sequence = ? AND last_committed_sequence = ?",
			state.attempt.ID, 1, state.attempt.NextSequence, state.attempt.LastCommittedSequence,
		).
		Updates(map[string]any{
			"next_sequence": state.sequence + 1, "last_committed_sequence": state.sequence,
			"updated_at": req.Now,
		})
	if attemptUpdate.Error != nil {
		return attemptUpdate.Error
	}
	if attemptUpdate.RowsAffected != 1 {
		return fmt.Errorf("%w: attempt sequence CAS failed", ErrAdaptiveExecutionSequenceConflict)
	}
	return nil
}

func isAdaptiveExecutionCheckpointPrimaryKeyConflict(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062 && strings.Contains(strings.ToUpper(mysqlErr.Message), "PRIMARY")
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") &&
		strings.Contains(message, "agent_checkpoints.id")
}

func isAdaptiveExecutionDuplicateError(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

func adaptiveExecutionInt64Pointer(value int64) *int64    { return &value }
func adaptiveExecutionUint64Pointer(value uint64) *uint64 { return &value }
func adaptiveExecutionStringPointer(value string) *string { return &value }
