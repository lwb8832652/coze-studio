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
	"database/sql"
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

const adaptiveExecutionCheckpointSchemaVersion = "workbench-adaptive-boundary.v2"

var errAdaptiveExecutionBoundaryTupleMissing = errors.New("adaptive execution boundary tuple is missing")

func adaptiveExecutionRecoveryTransactionOptions(db *gorm.DB) *sql.TxOptions {
	if db == nil || db.Dialector == nil || db.Dialector.Name() == "sqlite" {
		return nil
	}
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
}

func NewAdaptiveExecutionRepository(db *gorm.DB) AdaptiveExecutionRepository {
	return &threadRepository{db: db}
}

type adaptiveExecutionCheckpointMetadata struct {
	SchemaVersion         string                               `json:"schema_version"`
	EventID               int64                                `json:"event_id"`
	EventSequence         uint64                               `json:"event_sequence"`
	JournalRunID          int64                                `json:"journal_run_id"`
	AttemptID             string                               `json:"attempt_id"`
	SourceAttemptID       *string                              `json:"source_attempt_id"`
	SourceCheckpointID    *int64                               `json:"source_checkpoint_id"`
	EventIdempotencyKey   string                               `json:"event_idempotency_key"`
	EventFingerprint      string                               `json:"event_fingerprint"`
	CheckpointFingerprint string                               `json:"checkpoint_fingerprint"`
	PlanScopeRunID        int64                                `json:"plan_scope_run_id"`
	PlanRevision          int64                                `json:"plan_revision"`
	ItemFingerprint       string                               `json:"item_fingerprint"`
	ItemRefs              []adaptiveExecutionCheckpointItemRef `json:"item_refs"`
	ExecutionRunID        int64                                `json:"execution_run_id"`
	ExecutionGeneration   uint64                               `json:"execution_generation"`
}

type adaptiveExecutionCheckpointItemRef struct {
	ID      int64 `json:"id"`
	TaskID  int64 `json:"task_id"`
	Version int64 `json:"version"`
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

type adaptiveExecutionFingerprintEvent struct {
	ID                 int64                                    `json:"id"`
	ThreadID           int64                                    `json:"thread_id"`
	RunID              int64                                    `json:"run_id"`
	JournalRunID       *int64                                   `json:"journal_run_id"`
	AttemptID          *string                                  `json:"attempt_id"`
	Sequence           *uint64                                  `json:"sequence"`
	IdempotencyKey     *string                                  `json:"idempotency_key"`
	ParentEventID      *int64                                   `json:"parent_event_id"`
	SchemaVersion      *string                                  `json:"schema_version"`
	Status             *string                                  `json:"status"`
	OccurredAtUnixNano *int64                                   `json:"occurred_at_unix_nano"`
	Visibility         *string                                  `json:"visibility"`
	PayloadVersion     *string                                  `json:"payload_version"`
	SnapshotID         *string                                  `json:"snapshot_id"`
	TraceID            *string                                  `json:"trace_id"`
	ActionID           *string                                  `json:"action_id"`
	Phase              *string                                  `json:"phase"`
	Operation          *string                                  `json:"operation"`
	Target             *string                                  `json:"target"`
	Milestone          *string                                  `json:"milestone"`
	EventType          string                                   `json:"event_type"`
	JournalEventType   *string                                  `json:"journal_event_type"`
	Payload            json.RawMessage                          `json:"payload"`
	JournalPayload     adaptiveExecutionFingerprintOptionalJSON `json:"journal_payload"`
	CreatedAt          int64                                    `json:"created_at"`
}

type adaptiveExecutionFingerprintOptionalJSON struct {
	SQLNull bool            `json:"sql_null"`
	Value   json.RawMessage `json:"value"`
}

type adaptiveExecutionFingerprintCheckpoint struct {
	ID                 int64                               `json:"id"`
	ThreadID           int64                               `json:"thread_id"`
	RunID              int64                               `json:"run_id"`
	ParentCheckpointID int64                               `json:"parent_checkpoint_id"`
	CheckpointNS       string                              `json:"checkpoint_ns"`
	RuntimeType        string                              `json:"runtime_type"`
	RuntimeKey         string                              `json:"runtime_key"`
	EnvelopeVersion    int32                               `json:"envelope_version"`
	RuntimeDeletedAt   int64                               `json:"runtime_deleted_at"`
	ChannelValues      json.RawMessage                     `json:"channel_values"`
	ChannelVersions    json.RawMessage                     `json:"channel_versions"`
	PendingSends       json.RawMessage                     `json:"pending_sends"`
	UserMetadata       json.RawMessage                     `json:"user_metadata"`
	Authority          adaptiveExecutionCheckpointMetadata `json:"adaptive_execution"`
	CreatedAt          int64                               `json:"created_at"`
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

func adaptiveExecutionEventFingerprint(event *runEventPO) (string, error) {
	if event == nil || event.ID <= 0 || event.ThreadID <= 0 || event.RunID <= 0 ||
		strings.TrimSpace(event.EventType) == "" || event.CreatedAt <= 0 ||
		event.JournalRunID == nil || *event.JournalRunID <= 0 ||
		event.AttemptID == nil || strings.TrimSpace(*event.AttemptID) == "" || len([]byte(*event.AttemptID)) > 64 ||
		event.Sequence == nil || *event.Sequence == 0 ||
		event.IdempotencyKey == nil || strings.TrimSpace(*event.IdempotencyKey) == "" ||
		len([]byte(*event.IdempotencyKey)) > 191 {
		return "", fmt.Errorf("adaptive execution fingerprint event is invalid")
	}
	payload, err := canonicalAdaptiveExecutionJSON(string(event.Payload))
	if err != nil {
		return "", fmt.Errorf("canonicalize event payload: %w", err)
	}
	journalPayload, err := canonicalAdaptiveExecutionOptionalJSON(event.JournalPayload)
	if err != nil {
		return "", fmt.Errorf("canonicalize event journal_payload: %w", err)
	}
	encoded, err := json.Marshal(adaptiveExecutionFingerprintEvent{
		ID: event.ID, ThreadID: event.ThreadID, RunID: event.RunID,
		JournalRunID: event.JournalRunID, AttemptID: event.AttemptID,
		Sequence: event.Sequence, IdempotencyKey: event.IdempotencyKey,
		ParentEventID: event.ParentEventID, SchemaVersion: event.SchemaVersion,
		Status: event.Status, OccurredAtUnixNano: event.OccurredAtUnixNano,
		Visibility: event.Visibility, PayloadVersion: event.PayloadVersion,
		SnapshotID: event.SnapshotID, TraceID: event.TraceID, ActionID: event.ActionID,
		Phase: event.Phase, Operation: event.Operation, Target: event.Target,
		Milestone: event.Milestone, EventType: event.EventType,
		JournalEventType: event.JournalEventType, Payload: payload,
		JournalPayload: journalPayload, CreatedAt: event.CreatedAt,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func canonicalAdaptiveExecutionOptionalJSON(raw []byte) (adaptiveExecutionFingerprintOptionalJSON, error) {
	if len(raw) == 0 {
		return adaptiveExecutionFingerprintOptionalJSON{SQLNull: true}, nil
	}
	value, err := canonicalAdaptiveExecutionJSON(string(raw))
	if err != nil {
		return adaptiveExecutionFingerprintOptionalJSON{}, err
	}
	return adaptiveExecutionFingerprintOptionalJSON{Value: value}, nil
}

func adaptiveExecutionEventAnchorsCheckpoint(event *runEventPO, checkpointFingerprint string) bool {
	return event != nil && event.SnapshotID != nil &&
		validAdaptiveExecutionFingerprint(checkpointFingerprint) &&
		*event.SnapshotID == checkpointFingerprint
}

func adaptiveExecutionCheckpointFingerprint(
	checkpoint *checkpointPO,
	authorityMetadata *adaptiveExecutionCheckpointMetadata,
) (string, error) {
	if checkpoint == nil || authorityMetadata == nil {
		return "", fmt.Errorf("adaptive execution fingerprint checkpoint authority is required")
	}
	channelValues, err := canonicalAdaptiveExecutionJSON(string(checkpoint.ChannelValues))
	if err != nil {
		return "", fmt.Errorf("canonicalize checkpoint channel_values: %w", err)
	}
	channelVersions, err := canonicalAdaptiveExecutionJSON(string(checkpoint.ChannelVersions))
	if err != nil {
		return "", fmt.Errorf("canonicalize checkpoint channel_versions: %w", err)
	}
	pendingSends, err := canonicalAdaptiveExecutionJSON(string(checkpoint.PendingSends))
	if err != nil {
		return "", fmt.Errorf("canonicalize checkpoint pending_sends: %w", err)
	}
	userMetadata, err := adaptiveExecutionCheckpointUserMetadata(checkpoint.Metadata)
	if err != nil {
		return "", fmt.Errorf("read checkpoint user metadata: %w", err)
	}
	metadata, err := canonicalAdaptiveExecutionJSON(userMetadata)
	if err != nil {
		return "", fmt.Errorf("canonicalize checkpoint user metadata: %w", err)
	}
	authority := *authorityMetadata
	authority.EventFingerprint = ""
	authority.CheckpointFingerprint = ""
	encoded, err := json.Marshal(adaptiveExecutionFingerprintCheckpoint{
		ID: checkpoint.ID, ThreadID: checkpoint.ThreadID, RunID: checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID, CheckpointNS: checkpoint.CheckpointNS,
		RuntimeType: checkpoint.RuntimeType, RuntimeKey: checkpoint.RuntimeKey,
		EnvelopeVersion: checkpoint.EnvelopeVersion, RuntimeDeletedAt: checkpoint.RuntimeDeletedAt,
		ChannelValues: channelValues, ChannelVersions: channelVersions, PendingSends: pendingSends,
		UserMetadata: metadata, Authority: authority, CreatedAt: checkpoint.CreatedAt,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func adaptiveExecutionItemRefs(items []*entity.AgentRunPlanItem) ([]adaptiveExecutionCheckpointItemRef, error) {
	if len(items) < 1 || len(items) > 32 {
		return nil, fmt.Errorf("adaptive execution checkpoint item refs are invalid")
	}
	refs := make([]adaptiveExecutionCheckpointItemRef, 0, len(items))
	for _, item := range items {
		if item == nil {
			return nil, fmt.Errorf("adaptive execution checkpoint item ref is required")
		}
		refs = append(refs, adaptiveExecutionCheckpointItemRef{
			ID: item.ID, TaskID: item.TaskID, Version: item.Version,
		})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].TaskID < refs[j].TaskID })
	if err := validateAdaptiveExecutionItemRefs(refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func validateAdaptiveExecutionItemRefs(refs []adaptiveExecutionCheckpointItemRef) error {
	if len(refs) < 1 || len(refs) > 32 {
		return fmt.Errorf("adaptive execution checkpoint item refs are invalid")
	}
	itemIDs := make(map[int64]struct{}, len(refs))
	taskIDs := make(map[int64]struct{}, len(refs))
	var previousTaskID int64
	for index, ref := range refs {
		if ref.ID <= 0 || ref.TaskID <= 0 || ref.Version <= 0 ||
			(index > 0 && ref.TaskID <= previousTaskID) {
			return fmt.Errorf("adaptive execution checkpoint item refs are invalid")
		}
		if _, exists := itemIDs[ref.ID]; exists {
			return fmt.Errorf("adaptive execution checkpoint item refs contain duplicate id")
		}
		if _, exists := taskIDs[ref.TaskID]; exists {
			return fmt.Errorf("adaptive execution checkpoint item refs contain duplicate task id")
		}
		itemIDs[ref.ID] = struct{}{}
		taskIDs[ref.TaskID] = struct{}{}
		previousTaskID = ref.TaskID
	}
	return nil
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
	run, err := lockAdaptiveExecutionRunIdentity(tx, req.ExecutionRunID)
	if err != nil {
		return nil, err
	}
	if err := validateAdaptiveExecutionRunFence(run, req); err != nil {
		return nil, err
	}
	return run, nil
}

func lockAdaptiveExecutionRunIdentity(tx *gorm.DB, runID int64) (*runPO, error) {
	query := tx.Where("id = ?", runID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run runPO
	err := query.First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: run %d not found", ErrRunLeaseLost, runID)
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func lockAdaptiveExecutionJournalRunIdentity(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
) (*runPO, error) {
	run, err := lockAdaptiveExecutionRunIdentity(tx, req.JournalRunID)
	if err != nil {
		return nil, fmt.Errorf("%w: journal run is unavailable: %w", ErrAdaptiveExecutionLineageConflict, err)
	}
	if run.ID != req.JournalRunID || run.ThreadID != req.ThreadID || !isJournalRootRun(run) {
		return nil, fmt.Errorf("%w: journal run identity drift", ErrAdaptiveExecutionLineageConflict)
	}
	return run, nil
}

func validateAdaptiveExecutionRunFence(run *runPO, req CommitAdaptiveExecutionBoundaryRequest) error {
	if run == nil {
		return fmt.Errorf("%w: run %d is missing", ErrRunLeaseLost, req.ExecutionRunID)
	}
	if run.ThreadID != req.ThreadID || entity.RunStatus(run.Status) != entity.RunStatusRunning ||
		run.ExecutionGeneration != req.Generation ||
		run.LeaseOwner == nil || *run.LeaseOwner != req.LeaseOwner ||
		run.LeaseToken == nil || *run.LeaseToken != req.LeaseToken ||
		run.LeaseExpiresAt == nil || *run.LeaseExpiresAt <= req.Now {
		return fmt.Errorf("%w: run %d identity drift", ErrRunLeaseLost, req.ExecutionRunID)
	}
	if run.CancelRequestedAt != nil {
		return fmt.Errorf("%w: run %d has cancellation requested", ErrRunCanceled, req.ExecutionRunID)
	}
	return nil
}

func lockAdaptiveExecutionAttempt(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
) (*runAttemptPO, error) {
	attempt, err := lockAdaptiveExecutionAttemptIdentity(tx, req.JournalRunID, req.AttemptID)
	if err != nil {
		return nil, err
	}
	if err := validateAdaptiveExecutionAttemptFence(attempt, req); err != nil {
		return nil, err
	}
	return attempt, nil
}

func lockAdaptiveExecutionAttemptIdentity(
	tx *gorm.DB,
	journalRunID int64,
	attemptID string,
) (*runAttemptPO, error) {
	query := tx.Where(
		"journal_run_id = ? AND attempt_id = ?", journalRunID, attemptID,
	)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempt runAttemptPO
	err := query.First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf(
			"%w: attempt %s not found", ErrAdaptiveExecutionAttemptConflict, attemptID,
		)
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

func validateAdaptiveExecutionAttemptFence(
	attempt *runAttemptPO,
	req CommitAdaptiveExecutionBoundaryRequest,
) error {
	if attempt == nil || attempt.ThreadID != req.ThreadID || attempt.ExecutionRunID != req.ExecutionRunID ||
		!entity.RunAttemptStatus(attempt.Status).IsActive() || attempt.ActiveSlot == nil {
		return fmt.Errorf(
			"%w: attempt %s identity drift", ErrAdaptiveExecutionAttemptConflict, req.AttemptID,
		)
	}
	return nil
}

func lockAdaptiveExecutionBoundaryEventTuple(
	tx *gorm.DB,
	journalRunID int64,
	attemptID, idempotencyKey string,
) (*runEventPO, error) {
	query := tx.Where(
		"journal_run_id = ? AND attempt_id = ? AND idempotency_key = ?",
		journalRunID, attemptID, idempotencyKey,
	)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var event runEventPO
	err := query.First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errAdaptiveExecutionBoundaryTupleMissing
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
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
		journalRun, err := lockAdaptiveExecutionJournalRunIdentity(tx, req)
		if err != nil {
			return err
		}
		run := journalRun
		if req.ExecutionRunID != journalRun.ID {
			run, err = lockAdaptiveExecutionRunIdentity(tx, req.ExecutionRunID)
			if err != nil {
				return err
			}
		}
		attempt, err := lockAdaptiveExecutionAttemptIdentity(tx, req.JournalRunID, req.AttemptID)
		if err != nil {
			return err
		}
		replayEvent, err := lockAdaptiveExecutionBoundaryEventTuple(
			tx, req.JournalRunID, req.AttemptID, req.IdempotencyKey,
		)
		if err == nil {
			committed, err = loadAdaptiveExecutionBoundaryResult(tx, req, run, attempt, replayEvent, true)
			return err
		}
		if !errors.Is(err, errAdaptiveExecutionBoundaryTupleMissing) {
			return err
		}
		if err := validateAdaptiveExecutionRunFence(run, req); err != nil {
			return err
		}
		if err := validateAdaptiveExecutionAttemptFence(attempt, req); err != nil {
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
		itemRefs, err := adaptiveExecutionItemRefs(nextItems)
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
		checkpointBase, err := checkpointToPO(&checkpointEntity)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}
		metadata := adaptiveExecutionCheckpointMetadata{
			SchemaVersion: adaptiveExecutionCheckpointSchemaVersion,
			EventID:       event.ID, EventSequence: sequence,
			JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
			SourceAttemptID: attempt.SourceAttemptID, SourceCheckpointID: attempt.SourceCheckpointID,
			EventIdempotencyKey: req.IdempotencyKey,
			PlanScopeRunID:      req.PlanMutation.PlanScopeRunID,
			PlanRevision:        req.PlanMutation.NextRevision,
			ItemFingerprint:     fingerprint, ItemRefs: itemRefs,
			ExecutionRunID: run.ID, ExecutionGeneration: run.ExecutionGeneration,
		}
		checkpointFingerprint, err := adaptiveExecutionCheckpointFingerprint(checkpointBase, &metadata)
		if err != nil {
			return fmt.Errorf("%w: checkpoint fingerprint is invalid: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}
		event.SnapshotID = adaptiveExecutionStringPointer(checkpointFingerprint)
		eventFingerprint, err := adaptiveExecutionEventFingerprint(event)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}
		metadata.EventFingerprint = eventFingerprint
		metadata.CheckpointFingerprint = checkpointFingerprint
		checkpointEntity.Metadata, err = mergeAdaptiveExecutionCheckpointMetadata(req.Checkpoint.Metadata, metadata)
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
		committed, err = loadAdaptiveExecutionBoundaryResult(tx, req, run, attempt, event, false)
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

func (r *threadRepository) ReadAdaptiveExecutionRecoverySource(
	ctx context.Context,
	req ReadAdaptiveExecutionRecoverySourceRequest,
) (*CommitAdaptiveExecutionBoundaryResult, error) {
	if err := validateAdaptiveExecutionRecoverySourceRequest(req); err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: repository database is missing", ErrAdaptiveExecutionRecoveryConflict)
	}

	var recovered *CommitAdaptiveExecutionBoundaryResult
	read := func(tx *gorm.DB) error {
		var err error
		recovered, err = readAdaptiveExecutionRecoverySource(tx, req)
		return err
	}
	db := r.db.WithContext(ctx)
	options := adaptiveExecutionRecoveryTransactionOptions(r.db)
	var err error
	if options == nil {
		err = db.Transaction(read)
	} else {
		err = db.Transaction(read, options)
	}
	if err != nil {
		if errors.Is(err, ErrAdaptiveExecutionRecoveryConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: recovery snapshot read failed: %w", ErrAdaptiveExecutionRecoveryConflict, err)
	}
	return recovered, nil
}

func readAdaptiveExecutionRecoverySource(
	tx *gorm.DB,
	req ReadAdaptiveExecutionRecoverySourceRequest,
) (*CommitAdaptiveExecutionBoundaryResult, error) {
	conflict := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrAdaptiveExecutionRecoveryConflict, fmt.Sprintf(format, args...))
	}
	readRow := func(err error, row string) error {
		if err == nil {
			return nil
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return conflict("%s is missing", row)
		}
		return fmt.Errorf("%w: read %s: %w", ErrAdaptiveExecutionRecoveryConflict, row, err)
	}
	if tx == nil {
		return nil, conflict("recovery transaction is missing")
	}

	var targetAttempt runAttemptPO
	if err := readRow(tx.Where(
		"journal_run_id = ? AND attempt_id = ?", req.JournalRunID, req.TargetAttemptID,
	).First(&targetAttempt).Error, "target attempt"); err != nil {
		return nil, err
	}
	if targetAttempt.ID <= 0 || targetAttempt.ThreadID != req.ThreadID ||
		targetAttempt.JournalRunID != req.JournalRunID || targetAttempt.AttemptID != req.TargetAttemptID {
		return nil, conflict("target attempt identity drift")
	}
	if targetAttempt.SourceAttemptID == nil || targetAttempt.SourceCheckpointID == nil ||
		strings.TrimSpace(*targetAttempt.SourceAttemptID) == "" ||
		len([]byte(*targetAttempt.SourceAttemptID)) > 64 || *targetAttempt.SourceCheckpointID <= 0 ||
		*targetAttempt.SourceAttemptID == targetAttempt.AttemptID {
		return nil, conflict("target attempt source is invalid")
	}

	var sourceAttempt runAttemptPO
	if err := readRow(tx.Where(
		"journal_run_id = ? AND attempt_id = ?", targetAttempt.JournalRunID, *targetAttempt.SourceAttemptID,
	).First(&sourceAttempt).Error, "source attempt"); err != nil {
		return nil, err
	}
	if sourceAttempt.ID <= 0 || sourceAttempt.ThreadID != req.ThreadID ||
		sourceAttempt.JournalRunID != req.JournalRunID ||
		sourceAttempt.AttemptID != *targetAttempt.SourceAttemptID || sourceAttempt.ExecutionRunID <= 0 ||
		sourceAttempt.Ordinal == 0 || targetAttempt.Ordinal == 0 ||
		sourceAttempt.Ordinal >= targetAttempt.Ordinal {
		return nil, conflict("source attempt identity drift")
	}
	sourceAttemptHasParent := sourceAttempt.SourceAttemptID != nil
	sourceCheckpointHasParent := sourceAttempt.SourceCheckpointID != nil
	if sourceAttemptHasParent != sourceCheckpointHasParent ||
		(sourceAttemptHasParent && (strings.TrimSpace(*sourceAttempt.SourceAttemptID) == "" ||
			len([]byte(*sourceAttempt.SourceAttemptID)) > 64 || *sourceAttempt.SourceCheckpointID <= 0 ||
			*sourceAttempt.SourceAttemptID == sourceAttempt.AttemptID)) {
		return nil, conflict("source attempt lineage drift")
	}

	var sourceCheckpoint checkpointPO
	if err := readRow(tx.Where("id = ?", *targetAttempt.SourceCheckpointID).
		First(&sourceCheckpoint).Error, "source checkpoint"); err != nil {
		return nil, err
	}
	if sourceCheckpoint.ID != *targetAttempt.SourceCheckpointID ||
		sourceCheckpoint.ThreadID != req.ThreadID || sourceCheckpoint.RunID != sourceAttempt.ExecutionRunID ||
		sourceCheckpoint.RuntimeDeletedAt != 0 {
		return nil, conflict("source checkpoint identity drift")
	}
	metadata, err := decodeAdaptiveExecutionCheckpointMetadata(sourceCheckpoint.Metadata)
	if err != nil {
		return nil, conflict("source checkpoint metadata drift: %v", err)
	}
	checkpointFingerprint, err := adaptiveExecutionCheckpointFingerprint(&sourceCheckpoint, metadata)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %w: source checkpoint fingerprint is invalid: %w",
			ErrAdaptiveExecutionRecoveryConflict, ErrAdaptiveExecutionCheckpointConflict, err,
		)
	}
	if checkpointFingerprint != metadata.CheckpointFingerprint {
		return nil, fmt.Errorf(
			"%w: %w: source checkpoint fingerprint drift",
			ErrAdaptiveExecutionRecoveryConflict, ErrAdaptiveExecutionCheckpointConflict,
		)
	}
	if metadata.JournalRunID != sourceAttempt.JournalRunID ||
		metadata.AttemptID != sourceAttempt.AttemptID ||
		metadata.ExecutionRunID != sourceAttempt.ExecutionRunID ||
		metadata.EventSequence > sourceAttempt.LastCommittedSequence ||
		!adaptiveExecutionStringPointersEqual(metadata.SourceAttemptID, sourceAttempt.SourceAttemptID) ||
		!adaptiveExecutionInt64PointersEqual(metadata.SourceCheckpointID, sourceAttempt.SourceCheckpointID) {
		return nil, conflict("source attempt authority drift")
	}
	if metadata.SourceAttemptID == nil {
		if sourceCheckpoint.ParentCheckpointID != 0 {
			return nil, conflict("initial source checkpoint parent drift")
		}
	} else if *metadata.SourceAttemptID == metadata.AttemptID ||
		*metadata.SourceCheckpointID == sourceCheckpoint.ID ||
		sourceCheckpoint.ParentCheckpointID != *metadata.SourceCheckpointID {
		return nil, conflict("recovery source checkpoint parent drift")
	}

	var sourceEvent runEventPO
	if err := readRow(tx.Where(
		"journal_run_id = ? AND attempt_id = ? AND idempotency_key = ?",
		metadata.JournalRunID, metadata.AttemptID, metadata.EventIdempotencyKey,
	).First(&sourceEvent).Error, "source event"); err != nil {
		return nil, err
	}
	if sourceEvent.ID != metadata.EventID || sourceEvent.ThreadID != req.ThreadID ||
		sourceEvent.RunID != sourceAttempt.ExecutionRunID ||
		sourceEvent.JournalRunID == nil || *sourceEvent.JournalRunID != metadata.JournalRunID ||
		sourceEvent.AttemptID == nil || *sourceEvent.AttemptID != metadata.AttemptID ||
		sourceEvent.Sequence == nil || *sourceEvent.Sequence != metadata.EventSequence ||
		sourceEvent.IdempotencyKey == nil || *sourceEvent.IdempotencyKey != metadata.EventIdempotencyKey {
		return nil, conflict("source event tuple drift")
	}
	if !adaptiveExecutionEventAnchorsCheckpoint(&sourceEvent, metadata.CheckpointFingerprint) {
		return nil, fmt.Errorf(
			"%w: %w: source checkpoint event anchor drift",
			ErrAdaptiveExecutionRecoveryConflict, ErrAdaptiveExecutionCheckpointConflict,
		)
	}
	eventFingerprint, err := adaptiveExecutionEventFingerprint(&sourceEvent)
	if err != nil || eventFingerprint != metadata.EventFingerprint {
		return nil, conflict("source event fingerprint drift")
	}

	var sourceRun runPO
	if err := readRow(tx.Where("id = ?", sourceAttempt.ExecutionRunID).
		First(&sourceRun).Error, "source execution run"); err != nil {
		return nil, err
	}
	if sourceRun.ID != sourceAttempt.ExecutionRunID || sourceRun.ThreadID != req.ThreadID ||
		sourceRun.ExecutionGeneration != metadata.ExecutionGeneration {
		return nil, conflict("source execution run authority drift")
	}

	var scopeRun runPO
	if err := readRow(tx.Where("id = ?", metadata.PlanScopeRunID).
		First(&scopeRun).Error, "plan scope run"); err != nil {
		return nil, err
	}
	if scopeRun.ID != metadata.PlanScopeRunID || scopeRun.ThreadID != req.ThreadID ||
		scopeRun.ThreadID != sourceRun.ThreadID || scopeRun.SpaceID != sourceRun.SpaceID ||
		scopeRun.CreatorID != sourceRun.CreatorID {
		return nil, conflict("plan scope run authority drift")
	}

	var plan agentRunPlanPO
	if err := readRow(tx.Where("run_id = ?", metadata.PlanScopeRunID).
		First(&plan).Error, "plan"); err != nil {
		return nil, err
	}
	if plan.RunID != metadata.PlanScopeRunID || plan.ThreadID != scopeRun.ThreadID ||
		plan.SpaceID != scopeRun.SpaceID || plan.UserID != scopeRun.CreatorID ||
		plan.Revision != metadata.PlanRevision || plan.UpdatedAt != sourceEvent.CreatedAt {
		return nil, conflict("plan authority drift")
	}

	itemIDs := make([]int64, 0, len(metadata.ItemRefs))
	for _, ref := range metadata.ItemRefs {
		itemIDs = append(itemIDs, ref.ID)
	}
	var itemRows []agentRunPlanItemPO
	if err := readRow(tx.Where("id IN ?", itemIDs).Order("task_id ASC").Find(&itemRows).Error,
		"plan items"); err != nil {
		return nil, err
	}
	if len(itemRows) != len(metadata.ItemRefs) {
		return nil, conflict("plan item readback is incomplete")
	}
	items := make([]*entity.AgentRunPlanItem, 0, len(itemRows))
	for index := range itemRows {
		row := &itemRows[index]
		ref := metadata.ItemRefs[index]
		if row.ID != ref.ID || row.RunID != metadata.PlanScopeRunID ||
			row.TaskID != ref.TaskID || row.Version != ref.Version {
			return nil, conflict("plan item ref drift")
		}
		items = append(items, row.toEntity())
	}
	itemFingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	if err != nil || itemFingerprint != metadata.ItemFingerprint {
		return nil, conflict("plan item fingerprint drift")
	}

	return &CommitAdaptiveExecutionBoundaryResult{
		Event: sourceEvent.toEntity(), Checkpoint: sourceCheckpoint.toEntity(), Plan: plan.toEntity(),
		Items: items, LastCommittedSequence: metadata.EventSequence, Replayed: true,
		Authority: AdaptiveExecutionBoundaryAuthority{
			ThreadID: sourceEvent.ThreadID, ExecutionRunID: metadata.ExecutionRunID,
			ExecutionGeneration: metadata.ExecutionGeneration,
			JournalRunID:        metadata.JournalRunID, AttemptID: metadata.AttemptID,
			SourceAttemptID:    adaptiveExecutionCloneStringPointer(metadata.SourceAttemptID),
			SourceCheckpointID: adaptiveExecutionCloneInt64Pointer(metadata.SourceCheckpointID),
			EventID:            metadata.EventID, EventSequence: metadata.EventSequence,
			IdempotencyKey: metadata.EventIdempotencyKey, CheckpointID: sourceCheckpoint.ID,
			PlanScopeRunID: metadata.PlanScopeRunID, PlanRevision: metadata.PlanRevision,
			PlanItemFingerprint: metadata.ItemFingerprint,
		},
	}, nil
}

func loadAdaptiveExecutionBoundaryResult(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
	run *runPO,
	attempt *runAttemptPO,
	event *runEventPO,
	replayed bool,
) (*CommitAdaptiveExecutionBoundaryResult, error) {
	conflict := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrAdaptiveExecutionReplayConflict, fmt.Sprintf(format, args...))
	}
	if tx == nil || run == nil || attempt == nil || event == nil {
		return nil, conflict("boundary authority row is missing")
	}
	if run.ID != req.ExecutionRunID || run.ThreadID != req.ThreadID ||
		attempt.ThreadID != req.ThreadID || attempt.JournalRunID != req.JournalRunID ||
		attempt.ExecutionRunID != req.ExecutionRunID || attempt.AttemptID != req.AttemptID {
		return nil, conflict("run or attempt identity drift")
	}
	if event.ID != req.Event.ID || event.ThreadID != req.ThreadID || event.RunID != req.ExecutionRunID ||
		event.EventType != req.Event.EventType || event.CreatedAt != req.Now ||
		event.JournalRunID == nil || *event.JournalRunID != req.JournalRunID ||
		event.AttemptID == nil || *event.AttemptID != req.AttemptID ||
		event.Sequence == nil || *event.Sequence == 0 ||
		event.IdempotencyKey == nil || *event.IdempotencyKey != req.IdempotencyKey {
		return nil, conflict("event identity drift")
	}
	eventPayloadEqual, err := adaptiveExecutionJSONEqual(string(event.Payload), req.Event.Payload)
	if err != nil || !eventPayloadEqual {
		return nil, conflict("event payload drift")
	}

	var checkpoint checkpointPO
	err = tx.Where("id = ?", req.Checkpoint.ID).First(&checkpoint).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, conflict("checkpoint %d is missing", req.Checkpoint.ID)
	}
	if err != nil {
		return nil, err
	}
	if checkpoint.ID != req.Checkpoint.ID || checkpoint.ThreadID != req.Checkpoint.ThreadID ||
		checkpoint.RunID != req.Checkpoint.RunID ||
		checkpoint.ParentCheckpointID != req.Checkpoint.ParentCheckpointID ||
		checkpoint.CheckpointNS != req.Checkpoint.CheckpointNS ||
		checkpoint.RuntimeType != req.Checkpoint.RuntimeType ||
		checkpoint.RuntimeKey != req.Checkpoint.RuntimeKey ||
		checkpoint.EnvelopeVersion != req.Checkpoint.EnvelopeVersion ||
		checkpoint.RuntimeDeletedAt != req.Checkpoint.RuntimeDeletedAt ||
		checkpoint.CreatedAt != req.Checkpoint.CreatedAt {
		return nil, conflict("checkpoint base drift")
	}
	for _, comparison := range []struct {
		name   string
		stored string
		wanted string
	}{
		{name: "channel_values", stored: string(checkpoint.ChannelValues), wanted: req.Checkpoint.ChannelValues},
		{name: "channel_versions", stored: string(checkpoint.ChannelVersions), wanted: req.Checkpoint.ChannelVersions},
		{name: "pending_sends", stored: string(checkpoint.PendingSends), wanted: req.Checkpoint.PendingSends},
	} {
		equal, compareErr := adaptiveExecutionJSONEqual(comparison.stored, comparison.wanted)
		if compareErr != nil || !equal {
			return nil, conflict("checkpoint %s drift", comparison.name)
		}
	}
	userMetadata, err := adaptiveExecutionCheckpointUserMetadata(checkpoint.Metadata)
	if err != nil {
		return nil, conflict("checkpoint metadata envelope drift")
	}
	metadataEqual, err := adaptiveExecutionJSONEqual(userMetadata, req.Checkpoint.Metadata)
	if err != nil || !metadataEqual {
		return nil, conflict("checkpoint user metadata drift")
	}
	metadata, err := decodeAdaptiveExecutionCheckpointMetadata(checkpoint.Metadata)
	if err != nil {
		return nil, conflict("checkpoint authority metadata drift")
	}
	checkpointFingerprint, err := adaptiveExecutionCheckpointFingerprint(&checkpoint, metadata)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: %w: checkpoint fingerprint is invalid: %w",
			ErrAdaptiveExecutionReplayConflict, ErrAdaptiveExecutionCheckpointConflict, err,
		)
	}
	if checkpointFingerprint != metadata.CheckpointFingerprint {
		return nil, fmt.Errorf(
			"%w: %w: checkpoint fingerprint drift",
			ErrAdaptiveExecutionReplayConflict, ErrAdaptiveExecutionCheckpointConflict,
		)
	}
	if !adaptiveExecutionEventAnchorsCheckpoint(event, metadata.CheckpointFingerprint) {
		return nil, fmt.Errorf(
			"%w: %w: checkpoint event anchor drift",
			ErrAdaptiveExecutionReplayConflict, ErrAdaptiveExecutionCheckpointConflict,
		)
	}
	if metadata.EventID != event.ID || metadata.EventSequence != *event.Sequence ||
		metadata.JournalRunID != req.JournalRunID || metadata.AttemptID != req.AttemptID ||
		metadata.EventIdempotencyKey != req.IdempotencyKey ||
		metadata.ExecutionRunID != req.ExecutionRunID || metadata.ExecutionGeneration != req.Generation ||
		run.ExecutionGeneration != metadata.ExecutionGeneration ||
		metadata.PlanScopeRunID != req.PlanMutation.PlanScopeRunID ||
		metadata.PlanRevision != req.PlanMutation.NextRevision ||
		!adaptiveExecutionStringPointersEqual(metadata.SourceAttemptID, attempt.SourceAttemptID) ||
		!adaptiveExecutionInt64PointersEqual(metadata.SourceCheckpointID, attempt.SourceCheckpointID) {
		return nil, conflict("checkpoint authority drift")
	}
	if metadata.SourceCheckpointID == nil {
		if checkpoint.ParentCheckpointID != 0 {
			return nil, conflict("initial checkpoint parent drift")
		}
	} else if checkpoint.ParentCheckpointID != *metadata.SourceCheckpointID {
		return nil, conflict("recovery checkpoint parent drift")
	}
	eventFingerprint, err := adaptiveExecutionEventFingerprint(event)
	if err != nil || eventFingerprint != metadata.EventFingerprint {
		return nil, conflict("event fingerprint drift")
	}

	var currentAttempt runAttemptPO
	if err := tx.Where("id = ?", attempt.ID).First(&currentAttempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, conflict("attempt readback is missing")
		}
		return nil, err
	}
	if currentAttempt.ThreadID != req.ThreadID || currentAttempt.JournalRunID != req.JournalRunID ||
		currentAttempt.ExecutionRunID != req.ExecutionRunID || currentAttempt.AttemptID != req.AttemptID ||
		metadata.EventSequence > currentAttempt.LastCommittedSequence ||
		!adaptiveExecutionStringPointersEqual(metadata.SourceAttemptID, currentAttempt.SourceAttemptID) ||
		!adaptiveExecutionInt64PointersEqual(metadata.SourceCheckpointID, currentAttempt.SourceCheckpointID) {
		return nil, conflict("attempt authority drift")
	}

	scopeRun := run
	if metadata.PlanScopeRunID != run.ID {
		var loadedScopeRun runPO
		if err := tx.Where("id = ?", metadata.PlanScopeRunID).First(&loadedScopeRun).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, conflict("plan scope run is missing")
			}
			return nil, err
		}
		scopeRun = &loadedScopeRun
	}
	var plan agentRunPlanPO
	if err := tx.Where("run_id = ?", metadata.PlanScopeRunID).First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, conflict("plan is missing")
		}
		return nil, err
	}
	if plan.RunID != metadata.PlanScopeRunID || plan.Revision != metadata.PlanRevision ||
		plan.UpdatedAt != req.Now || plan.ThreadID != scopeRun.ThreadID ||
		plan.SpaceID != scopeRun.SpaceID || plan.UserID != scopeRun.CreatorID ||
		scopeRun.ThreadID != run.ThreadID || scopeRun.SpaceID != run.SpaceID ||
		scopeRun.CreatorID != run.CreatorID {
		return nil, conflict("plan authority drift")
	}

	itemIDs := make([]int64, 0, len(metadata.ItemRefs))
	for _, ref := range metadata.ItemRefs {
		itemIDs = append(itemIDs, ref.ID)
	}
	var itemRows []agentRunPlanItemPO
	if err := tx.Where("id IN ?", itemIDs).Order("task_id ASC").Find(&itemRows).Error; err != nil {
		return nil, err
	}
	if len(itemRows) != len(metadata.ItemRefs) {
		return nil, conflict("plan item readback is incomplete")
	}
	items := make([]*entity.AgentRunPlanItem, 0, len(itemRows))
	for index := range itemRows {
		row := &itemRows[index]
		ref := metadata.ItemRefs[index]
		if row.ID != ref.ID || row.RunID != metadata.PlanScopeRunID ||
			row.TaskID != ref.TaskID || row.Version != ref.Version {
			return nil, conflict("plan item ref drift")
		}
		items = append(items, row.toEntity())
	}
	storedFingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	if err != nil || storedFingerprint != metadata.ItemFingerprint {
		return nil, conflict("plan item fingerprint drift")
	}
	normalizedRequestItems, err := normalizeAdaptiveExecutionReplayItems(req, items)
	if err != nil {
		return nil, conflict("request plan item drift")
	}
	requestFingerprint, err := adaptiveExecutionPlanItemFingerprint(normalizedRequestItems)
	if err != nil || requestFingerprint != metadata.ItemFingerprint {
		return nil, conflict("request plan item fingerprint drift")
	}

	return &CommitAdaptiveExecutionBoundaryResult{
		Event: event.toEntity(), Checkpoint: checkpoint.toEntity(), Plan: plan.toEntity(),
		Items: items, LastCommittedSequence: metadata.EventSequence, Replayed: replayed,
		Authority: AdaptiveExecutionBoundaryAuthority{
			ThreadID: event.ThreadID, ExecutionRunID: metadata.ExecutionRunID,
			ExecutionGeneration: metadata.ExecutionGeneration,
			JournalRunID:        metadata.JournalRunID, AttemptID: metadata.AttemptID,
			SourceAttemptID:    adaptiveExecutionCloneStringPointer(metadata.SourceAttemptID),
			SourceCheckpointID: adaptiveExecutionCloneInt64Pointer(metadata.SourceCheckpointID),
			EventID:            metadata.EventID, EventSequence: metadata.EventSequence,
			IdempotencyKey: metadata.EventIdempotencyKey, CheckpointID: checkpoint.ID,
			PlanScopeRunID: metadata.PlanScopeRunID, PlanRevision: metadata.PlanRevision,
			PlanItemFingerprint: metadata.ItemFingerprint,
		},
	}, nil
}

func adaptiveExecutionJSONEqual(left, right string) (bool, error) {
	canonicalLeft, err := canonicalAdaptiveExecutionJSON(left)
	if err != nil {
		return false, err
	}
	canonicalRight, err := canonicalAdaptiveExecutionJSON(right)
	if err != nil {
		return false, err
	}
	return string(canonicalLeft) == string(canonicalRight), nil
}

func adaptiveExecutionCheckpointUserMetadata(raw []byte) (string, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
		return "", fmt.Errorf("checkpoint metadata envelope is invalid")
	}
	delete(envelope, "adaptive_execution")
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func normalizeAdaptiveExecutionReplayItems(
	req CommitAdaptiveExecutionBoundaryRequest,
	storedItems []*entity.AgentRunPlanItem,
) ([]*entity.AgentRunPlanItem, error) {
	if req.PlanMutation == nil || len(req.PlanMutation.Items) != len(storedItems) {
		return nil, fmt.Errorf("plan item count drift")
	}
	storedByTaskID := make(map[int64]*entity.AgentRunPlanItem, len(storedItems))
	for _, item := range storedItems {
		if item == nil {
			return nil, fmt.Errorf("stored plan item is missing")
		}
		storedByTaskID[item.TaskID] = item
	}
	normalized := make([]*entity.AgentRunPlanItem, 0, len(req.PlanMutation.Items))
	for _, mutation := range req.PlanMutation.Items {
		candidate := mutation.NextItem
		if candidate == nil {
			return nil, fmt.Errorf("request plan item is missing")
		}
		stored, exists := storedByTaskID[candidate.TaskID]
		if !exists || candidate.ID != stored.ID || candidate.RunID != stored.RunID ||
			candidate.Version != stored.Version || mutation.ExpectedVersion+1 != stored.Version {
			return nil, fmt.Errorf("request plan item identity drift")
		}
		next := *candidate
		if mutation.ExpectedVersion == 0 {
			next.CreatedAt = req.Now
		} else {
			next.CreatedAt = stored.CreatedAt
		}
		next.UpdatedAt = req.Now
		normalized = append(normalized, &next)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].TaskID < normalized[j].TaskID })
	return normalized, nil
}

func adaptiveExecutionStringPointersEqual(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func adaptiveExecutionInt64PointersEqual(left, right *int64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
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

	discoveredSourceAttempt, err := discoverAdaptiveExecutionSourceAttempt(
		tx, req.JournalRunID, *attempt.SourceAttemptID,
	)
	if err != nil {
		return 0, err
	}
	sourceRun, err := lockAdaptiveExecutionSourceRun(tx, discoveredSourceAttempt.ExecutionRunID)
	if err != nil {
		return 0, err
	}
	sourceAttempt, err := lockAdaptiveExecutionSourceAttempt(tx, req.JournalRunID, *attempt.SourceAttemptID)
	if err != nil {
		return 0, err
	}
	if discoveredSourceAttempt.ID != sourceAttempt.ID ||
		discoveredSourceAttempt.ThreadID != sourceAttempt.ThreadID ||
		discoveredSourceAttempt.JournalRunID != sourceAttempt.JournalRunID ||
		discoveredSourceAttempt.ExecutionRunID != sourceAttempt.ExecutionRunID ||
		discoveredSourceAttempt.AttemptID != sourceAttempt.AttemptID {
		return 0, fmt.Errorf("%w: source attempt discovery drift", ErrAdaptiveExecutionLineageConflict)
	}
	if sourceAttempt.ThreadID != req.ThreadID || sourceAttempt.JournalRunID != req.JournalRunID ||
		sourceAttempt.AttemptID != *attempt.SourceAttemptID || sourceAttempt.Ordinal == 0 ||
		attempt.Ordinal == 0 || sourceAttempt.Ordinal >= attempt.Ordinal {
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
	checkpointFingerprint, err := adaptiveExecutionCheckpointFingerprint(sourceCheckpoint, metadata)
	if err != nil {
		return 0, fmt.Errorf(
			"%w: %w: source checkpoint fingerprint is invalid: %w",
			ErrAdaptiveExecutionLineageConflict, ErrAdaptiveExecutionCheckpointConflict, err,
		)
	}
	if checkpointFingerprint != metadata.CheckpointFingerprint {
		return 0, fmt.Errorf(
			"%w: %w: source checkpoint fingerprint drift",
			ErrAdaptiveExecutionLineageConflict, ErrAdaptiveExecutionCheckpointConflict,
		)
	}
	if metadata.JournalRunID != sourceAttempt.JournalRunID ||
		metadata.AttemptID != sourceAttempt.AttemptID ||
		metadata.ExecutionRunID != sourceAttempt.ExecutionRunID ||
		metadata.EventSequence > sourceAttempt.LastCommittedSequence ||
		!adaptiveExecutionStringPointersEqual(metadata.SourceAttemptID, sourceAttempt.SourceAttemptID) ||
		!adaptiveExecutionInt64PointersEqual(metadata.SourceCheckpointID, sourceAttempt.SourceCheckpointID) ||
		metadata.PlanScopeRunID != req.PlanMutation.PlanScopeRunID ||
		metadata.PlanRevision != req.PlanMutation.ExpectedRevision {
		return 0, fmt.Errorf("%w: source checkpoint metadata drift", ErrAdaptiveExecutionLineageConflict)
	}
	if metadata.SourceCheckpointID == nil {
		if sourceCheckpoint.ParentCheckpointID != 0 {
			return 0, fmt.Errorf("%w: source checkpoint parent drift", ErrAdaptiveExecutionLineageConflict)
		}
	} else if *metadata.SourceAttemptID == metadata.AttemptID ||
		*metadata.SourceCheckpointID == sourceCheckpoint.ID ||
		sourceCheckpoint.ParentCheckpointID != *metadata.SourceCheckpointID {
		return 0, fmt.Errorf("%w: source checkpoint parent drift", ErrAdaptiveExecutionLineageConflict)
	}
	sourceEvent, err := lockAdaptiveExecutionBoundaryEventTuple(
		tx, metadata.JournalRunID, metadata.AttemptID, metadata.EventIdempotencyKey,
	)
	if err != nil {
		if errors.Is(err, errAdaptiveExecutionBoundaryTupleMissing) {
			return 0, fmt.Errorf("%w: source event not found", ErrAdaptiveExecutionLineageConflict)
		}
		return 0, err
	}
	if sourceEvent.ID != metadata.EventID || sourceEvent.ThreadID != req.ThreadID ||
		sourceEvent.RunID != sourceAttempt.ExecutionRunID ||
		sourceEvent.JournalRunID == nil || *sourceEvent.JournalRunID != sourceAttempt.JournalRunID ||
		sourceEvent.AttemptID == nil || *sourceEvent.AttemptID != sourceAttempt.AttemptID ||
		sourceEvent.Sequence == nil || *sourceEvent.Sequence != metadata.EventSequence ||
		sourceEvent.IdempotencyKey == nil || *sourceEvent.IdempotencyKey != metadata.EventIdempotencyKey {
		return 0, fmt.Errorf("%w: source event identity drift", ErrAdaptiveExecutionLineageConflict)
	}
	if !adaptiveExecutionEventAnchorsCheckpoint(sourceEvent, metadata.CheckpointFingerprint) {
		return 0, fmt.Errorf(
			"%w: %w: source checkpoint event anchor drift",
			ErrAdaptiveExecutionLineageConflict, ErrAdaptiveExecutionCheckpointConflict,
		)
	}
	sourceEventFingerprint, err := adaptiveExecutionEventFingerprint(sourceEvent)
	if err != nil {
		return 0, fmt.Errorf("%w: source event fingerprint is invalid", ErrAdaptiveExecutionLineageConflict)
	}
	if sourceEventFingerprint != metadata.EventFingerprint {
		return 0, fmt.Errorf("%w: source event fingerprint drift", ErrAdaptiveExecutionLineageConflict)
	}
	if sourceRun.ThreadID != req.ThreadID || sourceRun.ExecutionGeneration != metadata.ExecutionGeneration {
		return 0, fmt.Errorf("%w: source execution generation drift", ErrAdaptiveExecutionLineageConflict)
	}
	if err := lockAndValidateAdaptiveExecutionLineagePlan(tx, sourceRun, sourceEvent, metadata); err != nil {
		return 0, err
	}
	return metadata.PlanScopeRunID, nil
}

func lockAndValidateAdaptiveExecutionLineagePlan(
	tx *gorm.DB,
	sourceRun *runPO,
	sourceEvent *runEventPO,
	metadata *adaptiveExecutionCheckpointMetadata,
) error {
	if tx == nil || sourceRun == nil || sourceEvent == nil || metadata == nil {
		return fmt.Errorf("%w: source plan authority is missing", ErrAdaptiveExecutionLineageConflict)
	}
	scopeRun, err := lockAdaptiveExecutionScopeRun(tx, metadata.PlanScopeRunID)
	if err != nil {
		return fmt.Errorf("%w: source plan scope is unavailable: %v", ErrAdaptiveExecutionPlanScopeConflict, err)
	}
	if scopeRun.ID != metadata.PlanScopeRunID || scopeRun.ThreadID != sourceRun.ThreadID ||
		scopeRun.SpaceID != sourceRun.SpaceID || scopeRun.CreatorID != sourceRun.CreatorID {
		return fmt.Errorf("%w: source plan scope authority drift", ErrAdaptiveExecutionPlanScopeConflict)
	}
	plan, err := lockAdaptiveExecutionPlan(tx, metadata.PlanScopeRunID)
	if err != nil {
		return fmt.Errorf("%w: source plan is unavailable: %v", ErrAdaptiveExecutionPlanScopeConflict, err)
	}
	if plan.RunID != metadata.PlanScopeRunID || plan.ThreadID != scopeRun.ThreadID ||
		plan.SpaceID != scopeRun.SpaceID || plan.UserID != scopeRun.CreatorID ||
		plan.Revision != metadata.PlanRevision || plan.UpdatedAt != sourceEvent.CreatedAt {
		return fmt.Errorf("%w: source plan authority drift", ErrAdaptiveExecutionPlanScopeConflict)
	}
	items := make([]*entity.AgentRunPlanItem, 0, len(metadata.ItemRefs))
	for _, ref := range metadata.ItemRefs {
		row, itemErr := lockAdaptiveExecutionPlanItem(tx, ref.ID)
		if itemErr != nil {
			return fmt.Errorf("%w: source plan item %d is unavailable: %v", ErrAdaptiveExecutionLineageConflict, ref.ID, itemErr)
		}
		if row.ID != ref.ID || row.RunID != metadata.PlanScopeRunID ||
			row.TaskID != ref.TaskID || row.Version != ref.Version {
			return fmt.Errorf("%w: source plan item ref drift", ErrAdaptiveExecutionLineageConflict)
		}
		items = append(items, row.toEntity())
	}
	fingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	if err != nil || fingerprint != metadata.ItemFingerprint {
		return fmt.Errorf("%w: source plan item fingerprint drift", ErrAdaptiveExecutionLineageConflict)
	}
	return nil
}

func discoverAdaptiveExecutionSourceAttempt(
	tx *gorm.DB,
	journalRunID int64,
	attemptID string,
) (*runAttemptPO, error) {
	var attempt runAttemptPO
	err := tx.Where("journal_run_id = ? AND attempt_id = ?", journalRunID, attemptID).First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: source attempt not found", ErrAdaptiveExecutionLineageConflict)
	}
	if err != nil {
		return nil, err
	}
	return &attempt, nil
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
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("adaptive execution checkpoint metadata is invalid")
	}
	for _, required := range []string{
		"schema_version", "event_id", "event_sequence", "journal_run_id", "attempt_id",
		"source_attempt_id", "source_checkpoint_id", "event_idempotency_key", "event_fingerprint",
		"checkpoint_fingerprint",
		"plan_scope_run_id", "plan_revision", "item_fingerprint", "item_refs",
		"execution_run_id", "execution_generation",
	} {
		if _, present := fields[required]; !present {
			return nil, fmt.Errorf("adaptive execution checkpoint metadata field %s is missing", required)
		}
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
	sourceAttemptPresent := metadata.SourceAttemptID != nil
	sourceCheckpointPresent := metadata.SourceCheckpointID != nil
	if metadata.SchemaVersion != adaptiveExecutionCheckpointSchemaVersion || metadata.EventID <= 0 ||
		metadata.EventSequence == 0 || metadata.JournalRunID <= 0 ||
		strings.TrimSpace(metadata.AttemptID) == "" || len([]byte(metadata.AttemptID)) > 64 ||
		sourceAttemptPresent != sourceCheckpointPresent ||
		(sourceAttemptPresent && (strings.TrimSpace(*metadata.SourceAttemptID) == "" ||
			len([]byte(*metadata.SourceAttemptID)) > 64 || *metadata.SourceCheckpointID <= 0)) ||
		strings.TrimSpace(metadata.EventIdempotencyKey) == "" || len([]byte(metadata.EventIdempotencyKey)) > 191 ||
		!validAdaptiveExecutionFingerprint(metadata.EventFingerprint) ||
		!validAdaptiveExecutionFingerprint(metadata.CheckpointFingerprint) ||
		metadata.PlanScopeRunID <= 0 || metadata.PlanRevision <= 0 ||
		!validAdaptiveExecutionFingerprint(metadata.ItemFingerprint) ||
		metadata.ExecutionRunID <= 0 || metadata.ExecutionGeneration == 0 {
		return nil, fmt.Errorf("adaptive execution checkpoint metadata fields are invalid")
	}
	if err := validateAdaptiveExecutionItemRefs(metadata.ItemRefs); err != nil {
		return nil, err
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
	plan, err := lockAgentRunPlanForUpdate(tx, runID)
	if errors.Is(err, ErrPlanNotFound) {
		return nil, fmt.Errorf("%w: plan %d not found", ErrAdaptiveExecutionPlanScopeConflict, runID)
	}
	if err != nil {
		return nil, err
	}
	return plan, nil
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

func adaptiveExecutionCloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	return adaptiveExecutionInt64Pointer(*value)
}

func adaptiveExecutionCloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	return adaptiveExecutionStringPointer(*value)
}
