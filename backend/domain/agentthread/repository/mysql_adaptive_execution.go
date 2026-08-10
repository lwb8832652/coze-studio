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
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
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

type adaptiveVerifiedSuccessNormalizedFinalize struct {
	now                                 int64
	message                             *entity.Message
	messagePO                           *messagePO
	titleEvent                          *entity.RunEvent
	titleEventPO                        *runEventPO
	completionEvent                     *entity.RunEvent
	completionEventPO                   *runEventPO
	terminalCheckpoint                  *entity.Checkpoint
	terminalCheckpointPO                *checkpointPO
	terminalCheckpointOnTitleConflict   *entity.Checkpoint
	terminalCheckpointOnTitleConflictPO *checkpointPO
}

type adaptiveVerifiedSuccessLockedAuthority struct {
	authority  AdaptiveExecutionBoundaryAuthority
	event      *runEventPO
	checkpoint *checkpointPO
	metadata   *adaptiveExecutionCheckpointMetadata
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

type adaptiveVerifiedSuccessFingerprintOptionalJSON struct {
	SQLNull bool            `json:"sql_null"`
	Value   json.RawMessage `json:"value"`
}

type adaptiveVerifiedSuccessFingerprintRun struct {
	ID                  int64                                          `json:"id"`
	ThreadID            int64                                          `json:"thread_id"`
	ParentRunID         int64                                          `json:"parent_run_id"`
	SpaceID             int64                                          `json:"space_id"`
	CreatorID           int64                                          `json:"creator_id"`
	AssistantID         string                                         `json:"assistant_id"`
	RunKind             string                                         `json:"run_kind"`
	Status              string                                         `json:"status"`
	Command             adaptiveVerifiedSuccessFingerprintOptionalJSON `json:"command"`
	Input               adaptiveVerifiedSuccessFingerprintOptionalJSON `json:"input"`
	Config              adaptiveVerifiedSuccessFingerprintOptionalJSON `json:"config"`
	Context             adaptiveVerifiedSuccessFingerprintOptionalJSON `json:"context"`
	Metadata            adaptiveVerifiedSuccessFingerprintOptionalJSON `json:"metadata"`
	StreamMode          adaptiveVerifiedSuccessFingerprintOptionalJSON `json:"stream_mode"`
	MultitaskStrategy   string                                         `json:"multitask_strategy"`
	OnDisconnect        string                                         `json:"on_disconnect"`
	Durability          string                                         `json:"durability"`
	IdempotencyKey      *string                                        `json:"idempotency_key"`
	WorkerID            string                                         `json:"worker_id"`
	LeaseOwner          *string                                        `json:"lease_owner"`
	LeaseToken          *string                                        `json:"lease_token"`
	LeaseExpiresAt      *int64                                         `json:"lease_expires_at"`
	HeartbeatAt         *int64                                         `json:"heartbeat_at"`
	CancelRequestedAt   *int64                                         `json:"cancel_requested_at"`
	ExecutionGeneration uint64                                         `json:"execution_generation"`
	ErrorCode           string                                         `json:"error_code"`
	ErrorMessage        string                                         `json:"error_message"`
	StartedAt           int64                                          `json:"started_at"`
	EndedAt             int64                                          `json:"ended_at"`
	CreatedAt           int64                                          `json:"created_at"`
	UpdatedAt           int64                                          `json:"updated_at"`
}

type adaptiveVerifiedSuccessFingerprintAttempt struct {
	ID                     int64   `json:"id"`
	ThreadID               int64   `json:"thread_id"`
	JournalRunID           int64   `json:"journal_run_id"`
	ExecutionRunID         int64   `json:"execution_run_id"`
	AttemptID              string  `json:"attempt_id"`
	Ordinal                uint32  `json:"ordinal"`
	Status                 string  `json:"status"`
	ActiveSlot             *uint8  `json:"active_slot"`
	NextSequence           uint64  `json:"next_sequence"`
	LastCommittedSequence  uint64  `json:"last_committed_sequence"`
	SourceCheckpointID     *int64  `json:"source_checkpoint_id"`
	SourceAttemptID        *string `json:"source_attempt_id"`
	RecoveryIdempotencyKey *string `json:"recovery_idempotency_key"`
	EnrollmentVersion      string  `json:"enrollment_version"`
	SnapshotsEnabled       bool    `json:"snapshots_enabled"`
	ProjectionState        string  `json:"projection_state"`
	ProjectionDegradedAt   *int64  `json:"projection_degraded_at"`
	TraceID                *string `json:"trace_id"`
	TerminalEventID        *int64  `json:"terminal_event_id"`
	CreatedAt              int64   `json:"created_at"`
	UpdatedAt              int64   `json:"updated_at"`
	StartedAt              *int64  `json:"started_at"`
	EndedAt                *int64  `json:"ended_at"`
}

type adaptiveVerifiedSuccessFingerprintEvent struct {
	ID                 int64                                          `json:"id"`
	ThreadID           int64                                          `json:"thread_id"`
	RunID              int64                                          `json:"run_id"`
	JournalRunID       *int64                                         `json:"journal_run_id"`
	AttemptID          *string                                        `json:"attempt_id"`
	Sequence           *uint64                                        `json:"sequence"`
	IdempotencyKey     *string                                        `json:"idempotency_key"`
	ParentEventID      *int64                                         `json:"parent_event_id"`
	SchemaVersion      *string                                        `json:"schema_version"`
	Status             *string                                        `json:"status"`
	OccurredAtUnixNano *int64                                         `json:"occurred_at_unix_nano"`
	Visibility         *string                                        `json:"visibility"`
	PayloadVersion     *string                                        `json:"payload_version"`
	SnapshotID         *string                                        `json:"snapshot_id"`
	TraceID            *string                                        `json:"trace_id"`
	ActionID           *string                                        `json:"action_id"`
	Phase              *string                                        `json:"phase"`
	Operation          *string                                        `json:"operation"`
	Target             *string                                        `json:"target"`
	Milestone          *string                                        `json:"milestone"`
	EventType          string                                         `json:"event_type"`
	JournalEventType   *string                                        `json:"journal_event_type"`
	Payload            json.RawMessage                                `json:"payload"`
	JournalPayload     adaptiveVerifiedSuccessFingerprintOptionalJSON `json:"journal_payload"`
	CreatedAt          int64                                          `json:"created_at"`
}

type adaptiveVerifiedSuccessFingerprintAuthority struct {
	ThreadID            int64   `json:"thread_id"`
	ExecutionRunID      int64   `json:"execution_run_id"`
	ExecutionGeneration uint64  `json:"execution_generation"`
	JournalRunID        int64   `json:"journal_run_id"`
	AttemptID           string  `json:"attempt_id"`
	SourceAttemptID     *string `json:"source_attempt_id"`
	SourceCheckpointID  *int64  `json:"source_checkpoint_id"`
	EventID             int64   `json:"event_id"`
	EventSequence       uint64  `json:"event_sequence"`
	IdempotencyKey      string  `json:"idempotency_key"`
	CheckpointID        int64   `json:"checkpoint_id"`
	PlanScopeRunID      int64   `json:"plan_scope_run_id"`
	PlanRevision        int64   `json:"plan_revision"`
	PlanItemFingerprint string  `json:"plan_item_fingerprint"`
}

type adaptiveVerifiedSuccessFingerprintMessage struct {
	ID        int64                                          `json:"id"`
	ThreadID  int64                                          `json:"thread_id"`
	RunID     int64                                          `json:"run_id"`
	Role      string                                         `json:"role"`
	Content   string                                         `json:"content"`
	Metadata  adaptiveVerifiedSuccessFingerprintOptionalJSON `json:"metadata"`
	CreatedAt int64                                          `json:"created_at"`
}

type adaptiveVerifiedSuccessFingerprintCheckpoint struct {
	ID                 int64           `json:"id"`
	ThreadID           int64           `json:"thread_id"`
	RunID              int64           `json:"run_id"`
	ParentCheckpointID int64           `json:"parent_checkpoint_id"`
	CheckpointNS       string          `json:"checkpoint_ns"`
	RuntimeType        string          `json:"runtime_type"`
	RuntimeKey         string          `json:"runtime_key"`
	EnvelopeVersion    int32           `json:"envelope_version"`
	RuntimeDeletedAt   int64           `json:"runtime_deleted_at"`
	ChannelValues      json.RawMessage `json:"channel_values"`
	ChannelVersions    json.RawMessage `json:"channel_versions"`
	PendingSends       json.RawMessage `json:"pending_sends"`
	Metadata           json.RawMessage `json:"metadata"`
	CreatedAt          int64           `json:"created_at"`
}

type adaptiveVerifiedSuccessFinalizeFingerprint struct {
	RunID                int64                                         `json:"run_id"`
	ExecutionGeneration  uint64                                        `json:"execution_generation"`
	Now                  int64                                         `json:"now"`
	TerminalRun          adaptiveVerifiedSuccessFingerprintRun         `json:"terminal_run"`
	TerminalAttempt      adaptiveVerifiedSuccessFingerprintAttempt     `json:"terminal_attempt"`
	Decision             adaptiveVerifiedSuccessFingerprintAuthority   `json:"decision"`
	Evidence             adaptiveVerifiedSuccessFingerprintAuthority   `json:"evidence"`
	DecisionID           string                                        `json:"decision_id"`
	DecisionRevision     int64                                         `json:"decision_revision"`
	Verification         adaptiveVerifiedSuccessFingerprintEvent       `json:"verification"`
	Completion           adaptiveVerifiedSuccessFingerprintEvent       `json:"completion"`
	Message              adaptiveVerifiedSuccessFingerprintMessage     `json:"message"`
	TitleEvent           *adaptiveVerifiedSuccessFingerprintEvent      `json:"title_event"`
	ExpectedThreadTitle  string                                        `json:"expected_thread_title"`
	ThreadTitle          string                                        `json:"thread_title"`
	TitleUpdated         bool                                          `json:"title_updated"`
	CommittedThreadTitle string                                        `json:"committed_thread_title"`
	PrimaryCheckpoint    adaptiveVerifiedSuccessFingerprintCheckpoint  `json:"primary_checkpoint"`
	FallbackCheckpoint   *adaptiveVerifiedSuccessFingerprintCheckpoint `json:"fallback_checkpoint"`
	SelectedCheckpoint   adaptiveVerifiedSuccessFingerprintCheckpoint  `json:"selected_checkpoint"`
	OutboxFingerprint    *string                                       `json:"outbox_fingerprint"`
}

type adaptiveVerifiedSuccessOutboxFingerprint struct {
	EventID          string                          `json:"event_id"`
	EventType        string                          `json:"event_type"`
	AggregateType    string                          `json:"aggregate_type"`
	AggregateID      string                          `json:"aggregate_id"`
	AggregateVersion int64                           `json:"aggregate_version"`
	OccurredAt       int64                           `json:"occurred_at"`
	ActorID          int64                           `json:"actor_id"`
	SpaceID          int64                           `json:"space_id"`
	RecipientPolicy  string                          `json:"recipient_policy"`
	PayloadSchema    int32                           `json:"payload_schema"`
	Payload          domainnotification.EventPayload `json:"payload"`
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

func adaptiveVerifiedSuccessConflictf(format string, args ...any) error {
	return fmt.Errorf(
		"%w: %s",
		ErrAdaptiveExecutionVerifiedSuccessConflict,
		fmt.Sprintf(format, args...),
	)
}

func adaptiveVerifiedSuccessConflictCausef(cause error, format string, args ...any) error {
	return fmt.Errorf(
		"%w: %w: %s",
		ErrAdaptiveExecutionVerifiedSuccessConflict,
		cause,
		fmt.Sprintf(format, args...),
	)
}

func lockAdaptiveVerifiedSuccessCheckpoint(
	tx *gorm.DB,
	checkpointID int64,
) (*checkpointPO, error) {
	query := tx.Where("id = ?", checkpointID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var checkpoint checkpointPO
	if err := query.First(&checkpoint).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, adaptiveVerifiedSuccessConflictCausef(
				ErrAdaptiveExecutionCheckpointConflict,
				"checkpoint %d is missing",
				checkpointID,
			)
		}
		return nil, err
	}
	return &checkpoint, nil
}

func lockAdaptiveVerifiedSuccessAuthorities(
	tx *gorm.DB,
	decision AdaptiveExecutionBoundaryAuthority,
	evidence AdaptiveExecutionBoundaryAuthority,
) (*adaptiveVerifiedSuccessLockedAuthority, *adaptiveVerifiedSuccessLockedAuthority, error) {
	locked := []*adaptiveVerifiedSuccessLockedAuthority{
		{authority: decision},
		{authority: evidence},
	}
	checkpointOrder := append([]*adaptiveVerifiedSuccessLockedAuthority(nil), locked...)
	sort.Slice(checkpointOrder, func(left, right int) bool {
		return checkpointOrder[left].authority.CheckpointID < checkpointOrder[right].authority.CheckpointID
	})
	for _, candidate := range checkpointOrder {
		checkpoint, err := lockAdaptiveVerifiedSuccessCheckpoint(tx, candidate.authority.CheckpointID)
		if err != nil {
			return nil, nil, err
		}
		candidate.checkpoint = checkpoint
	}
	eventOrder := append([]*adaptiveVerifiedSuccessLockedAuthority(nil), locked...)
	sort.Slice(eventOrder, func(left, right int) bool {
		return eventOrder[left].authority.EventID < eventOrder[right].authority.EventID
	})
	for _, candidate := range eventOrder {
		event, err := lockAdaptiveExecutionBoundaryEventTuple(
			tx,
			candidate.authority.JournalRunID,
			candidate.authority.AttemptID,
			candidate.authority.IdempotencyKey,
		)
		if errors.Is(err, errAdaptiveExecutionBoundaryTupleMissing) {
			return nil, nil, adaptiveVerifiedSuccessConflictf(
				"authority event %d is missing", candidate.authority.EventID,
			)
		}
		if err != nil {
			return nil, nil, err
		}
		candidate.event = event
	}
	for _, candidate := range locked {
		metadata, err := validateAdaptiveVerifiedSuccessAuthorityRows(candidate)
		if err != nil {
			return nil, nil, err
		}
		candidate.metadata = metadata
	}
	return locked[0], locked[1], nil
}

func validateAdaptiveVerifiedSuccessAuthorityRows(
	locked *adaptiveVerifiedSuccessLockedAuthority,
) (*adaptiveExecutionCheckpointMetadata, error) {
	if locked == nil || locked.event == nil || locked.checkpoint == nil {
		return nil, adaptiveVerifiedSuccessConflictf("authority rows are missing")
	}
	authority := locked.authority
	event := locked.event
	checkpoint := locked.checkpoint
	if event.ID != authority.EventID || event.ThreadID != authority.ThreadID ||
		event.RunID != authority.ExecutionRunID || event.JournalRunID == nil ||
		*event.JournalRunID != authority.JournalRunID || event.AttemptID == nil ||
		*event.AttemptID != authority.AttemptID || event.Sequence == nil ||
		*event.Sequence != authority.EventSequence || event.IdempotencyKey == nil ||
		*event.IdempotencyKey != authority.IdempotencyKey || event.CreatedAt <= 0 {
		return nil, adaptiveVerifiedSuccessConflictf("authority event identity drift")
	}
	if checkpoint.ID != authority.CheckpointID || checkpoint.ThreadID != authority.ThreadID ||
		checkpoint.RunID != authority.ExecutionRunID || checkpoint.RuntimeDeletedAt != 0 {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"authority checkpoint identity drift",
		)
	}
	expectedParentCheckpointID := int64(0)
	if authority.SourceCheckpointID != nil {
		expectedParentCheckpointID = *authority.SourceCheckpointID
	}
	if checkpoint.ParentCheckpointID != expectedParentCheckpointID {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"authority checkpoint parent drift",
		)
	}
	metadata, err := decodeAdaptiveExecutionCheckpointMetadata(checkpoint.Metadata)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"authority checkpoint metadata is invalid",
		)
	}
	if metadata.EventID != authority.EventID ||
		metadata.EventIdempotencyKey != authority.IdempotencyKey {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"authority checkpoint metadata drift",
		)
	}
	if metadata.EventSequence != authority.EventSequence {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionSequenceConflict,
			"authority checkpoint event sequence drift",
		)
	}
	if metadata.JournalRunID != authority.JournalRunID || metadata.AttemptID != authority.AttemptID ||
		metadata.ExecutionRunID != authority.ExecutionRunID ||
		metadata.ExecutionGeneration != authority.ExecutionGeneration ||
		!adaptiveExecutionStringPointersEqual(metadata.SourceAttemptID, authority.SourceAttemptID) ||
		!adaptiveExecutionInt64PointersEqual(metadata.SourceCheckpointID, authority.SourceCheckpointID) {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionAttemptConflict,
			"authority checkpoint attempt drift",
		)
	}
	if metadata.PlanScopeRunID != authority.PlanScopeRunID {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionPlanScopeConflict,
			"authority checkpoint plan scope drift",
		)
	}
	if metadata.PlanRevision != authority.PlanRevision {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionPlanRevisionConflict,
			"authority checkpoint plan revision drift",
		)
	}
	if metadata.ItemFingerprint != authority.PlanItemFingerprint {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionPlanItemVersionConflict,
			"authority checkpoint item fingerprint drift",
		)
	}
	checkpointFingerprint, err := adaptiveExecutionCheckpointFingerprint(checkpoint, metadata)
	if err != nil || checkpointFingerprint != metadata.CheckpointFingerprint ||
		!adaptiveExecutionEventAnchorsCheckpoint(event, metadata.CheckpointFingerprint) {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"authority checkpoint fingerprint drift",
		)
	}
	eventFingerprint, err := adaptiveExecutionEventFingerprint(event)
	if err != nil || eventFingerprint != metadata.EventFingerprint {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"authority event fingerprint drift",
		)
	}
	return metadata, nil
}

func validateAdaptiveVerifiedSuccessDecisionPayload(
	raw []byte,
	decisionID string,
	decisionRevision int64,
) error {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(string(raw))))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return adaptiveVerifiedSuccessConflictf("decision payload is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return adaptiveVerifiedSuccessConflictf("decision payload has trailing content")
	}
	schema, schemaOK := fields["schema"].(string)
	storedID, idOK := fields["decision_id"].(string)
	revisionNumber, revisionOK := fields["decision_revision"].(json.Number)
	storedRevision, revisionErr := revisionNumber.Int64()
	if !schemaOK || !idOK || !revisionOK || revisionErr != nil ||
		schema != "workbench-adaptive-decision.v1" || storedID != decisionID ||
		storedRevision != decisionRevision {
		return adaptiveVerifiedSuccessConflictf("decision payload authority drift")
	}
	return nil
}

func lockLatestAdaptiveVerifiedSuccessDecision(
	tx *gorm.DB,
	evidence AdaptiveExecutionBoundaryAuthority,
) (*runEventPO, error) {
	query := tx.Where(
		"journal_run_id = ? AND attempt_id = ? AND sequence <= ? AND event_type = ?",
		evidence.JournalRunID,
		evidence.AttemptID,
		evidence.EventSequence,
		"adaptive.decision",
	).Order("sequence DESC, id DESC")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var event runEventPO
	if err := query.First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, adaptiveVerifiedSuccessConflictf("latest decision is missing")
		}
		return nil, err
	}
	return &event, nil
}

func lockAdaptiveVerifiedSuccessPlanItems(
	tx *gorm.DB,
	plan *agentRunPlanPO,
	evidence *adaptiveVerifiedSuccessLockedAuthority,
	expectedFingerprint string,
) ([]agentRunPlanItemPO, error) {
	query := tx.Where("run_id = ?", plan.RunID).Order("task_id ASC")
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []agentRunPlanItemPO
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 || evidence == nil || evidence.metadata == nil {
		return nil, adaptiveVerifiedSuccessConflictf("current plan items are missing")
	}
	if plan.HighWatermark < rows[len(rows)-1].TaskID {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionPlanRevisionConflict,
			"plan high watermark is below the current task id",
		)
	}
	byID := make(map[int64]*entity.AgentRunPlanItem, len(rows))
	all := make([]*entity.AgentRunPlanItem, 0, len(rows))
	for index := range rows {
		item := rows[index].toEntity()
		byID[item.ID] = item
		all = append(all, item)
	}
	refs := evidence.metadata.ItemRefs
	subset := make([]*entity.AgentRunPlanItem, 0, len(refs))
	for _, ref := range refs {
		item, exists := byID[ref.ID]
		if !exists || item.RunID != plan.RunID || item.TaskID != ref.TaskID || item.Version != ref.Version {
			return nil, adaptiveVerifiedSuccessConflictCausef(
				ErrAdaptiveExecutionPlanItemVersionConflict,
				"evidence plan item reference drift",
			)
		}
		subset = append(subset, item)
	}
	subsetFingerprint, err := adaptiveExecutionPlanItemFingerprint(subset)
	if err != nil || subsetFingerprint != evidence.metadata.ItemFingerprint {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionPlanItemVersionConflict,
			"evidence plan item fingerprint drift",
		)
	}
	currentFingerprint, err := adaptiveExecutionPlanItemFingerprint(all)
	if err != nil || currentFingerprint != expectedFingerprint {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionPlanItemVersionConflict,
			"current plan item fingerprint drift",
		)
	}
	return rows, nil
}

func adaptiveVerifiedSuccessCanonicalOptionalJSON(
	raw []byte,
) (adaptiveVerifiedSuccessFingerprintOptionalJSON, error) {
	if len(raw) == 0 {
		return adaptiveVerifiedSuccessFingerprintOptionalJSON{SQLNull: true}, nil
	}
	value, err := canonicalAdaptiveExecutionJSON(string(raw))
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintOptionalJSON{}, err
	}
	return adaptiveVerifiedSuccessFingerprintOptionalJSON{Value: value}, nil
}

func adaptiveVerifiedSuccessFingerprintRunPO(
	run *runPO,
) (adaptiveVerifiedSuccessFingerprintRun, error) {
	if run == nil {
		return adaptiveVerifiedSuccessFingerprintRun{}, fmt.Errorf("terminal run is required")
	}
	command, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(run.Command)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintRun{}, err
	}
	input, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(run.Input)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintRun{}, err
	}
	config, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(run.Config)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintRun{}, err
	}
	contextJSON, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(run.Context)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintRun{}, err
	}
	metadata, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(run.Metadata)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintRun{}, err
	}
	streamMode, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(run.StreamMode)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintRun{}, err
	}
	return adaptiveVerifiedSuccessFingerprintRun{
		ID: run.ID, ThreadID: run.ThreadID, ParentRunID: run.ParentRunID,
		SpaceID: run.SpaceID, CreatorID: run.CreatorID, AssistantID: run.AssistantID,
		RunKind: run.RunKind, Status: run.Status,
		Command: command, Input: input, Config: config, Context: contextJSON,
		Metadata: metadata, StreamMode: streamMode,
		MultitaskStrategy: run.MultitaskStrategy, OnDisconnect: run.OnDisconnect,
		Durability: run.Durability, IdempotencyKey: run.IdempotencyKey,
		WorkerID: run.WorkerID, LeaseOwner: run.LeaseOwner, LeaseToken: run.LeaseToken,
		LeaseExpiresAt: run.LeaseExpiresAt, HeartbeatAt: run.HeartbeatAt,
		CancelRequestedAt: run.CancelRequestedAt, ExecutionGeneration: run.ExecutionGeneration,
		ErrorCode: run.ErrorCode, ErrorMessage: run.ErrorMessage, StartedAt: run.StartedAt,
		EndedAt: run.EndedAt, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	}, nil
}

func adaptiveVerifiedSuccessFingerprintAttemptPO(
	attempt *runAttemptPO,
) (adaptiveVerifiedSuccessFingerprintAttempt, error) {
	if attempt == nil {
		return adaptiveVerifiedSuccessFingerprintAttempt{}, fmt.Errorf("terminal attempt is required")
	}
	return adaptiveVerifiedSuccessFingerprintAttempt{
		ID: attempt.ID, ThreadID: attempt.ThreadID, JournalRunID: attempt.JournalRunID,
		ExecutionRunID: attempt.ExecutionRunID, AttemptID: attempt.AttemptID,
		Ordinal: attempt.Ordinal, Status: attempt.Status, ActiveSlot: attempt.ActiveSlot,
		NextSequence: attempt.NextSequence, LastCommittedSequence: attempt.LastCommittedSequence,
		SourceCheckpointID: attempt.SourceCheckpointID, SourceAttemptID: attempt.SourceAttemptID,
		RecoveryIdempotencyKey: attempt.RecoveryIdempotencyKey,
		EnrollmentVersion:      attempt.EnrollmentVersion, SnapshotsEnabled: attempt.SnapshotsEnabled,
		ProjectionState: attempt.ProjectionState, ProjectionDegradedAt: attempt.ProjectionDegradedAt,
		TraceID: attempt.TraceID, TerminalEventID: attempt.TerminalEventID,
		CreatedAt: attempt.CreatedAt, UpdatedAt: attempt.UpdatedAt,
		StartedAt: attempt.StartedAt, EndedAt: attempt.EndedAt,
	}, nil
}

func adaptiveVerifiedSuccessFingerprintEventPO(
	event *runEventPO,
) (adaptiveVerifiedSuccessFingerprintEvent, error) {
	if event == nil {
		return adaptiveVerifiedSuccessFingerprintEvent{}, fmt.Errorf("terminal event is required")
	}
	payload, err := canonicalAdaptiveExecutionJSON(string(event.Payload))
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintEvent{}, err
	}
	journalPayload, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(event.JournalPayload)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintEvent{}, err
	}
	return adaptiveVerifiedSuccessFingerprintEvent{
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
	}, nil
}

func adaptiveVerifiedSuccessFingerprintAuthorityValue(
	authority AdaptiveExecutionBoundaryAuthority,
) adaptiveVerifiedSuccessFingerprintAuthority {
	return adaptiveVerifiedSuccessFingerprintAuthority{
		ThreadID: authority.ThreadID, ExecutionRunID: authority.ExecutionRunID,
		ExecutionGeneration: authority.ExecutionGeneration, JournalRunID: authority.JournalRunID,
		AttemptID: authority.AttemptID, SourceAttemptID: authority.SourceAttemptID,
		SourceCheckpointID: authority.SourceCheckpointID, EventID: authority.EventID,
		EventSequence: authority.EventSequence, IdempotencyKey: authority.IdempotencyKey,
		CheckpointID: authority.CheckpointID, PlanScopeRunID: authority.PlanScopeRunID,
		PlanRevision: authority.PlanRevision, PlanItemFingerprint: authority.PlanItemFingerprint,
	}
}

func adaptiveVerifiedSuccessFingerprintCheckpointValue(
	checkpoint *entity.Checkpoint,
) (adaptiveVerifiedSuccessFingerprintCheckpoint, error) {
	if checkpoint == nil {
		return adaptiveVerifiedSuccessFingerprintCheckpoint{}, fmt.Errorf("terminal checkpoint is required")
	}
	channelValues, err := canonicalAdaptiveExecutionJSON(checkpoint.ChannelValues)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintCheckpoint{}, err
	}
	channelVersions, err := canonicalAdaptiveExecutionJSON(checkpoint.ChannelVersions)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintCheckpoint{}, err
	}
	pendingSends, err := canonicalAdaptiveExecutionJSON(checkpoint.PendingSends)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintCheckpoint{}, err
	}
	metadata, err := canonicalAdaptiveExecutionJSON(checkpoint.Metadata)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintCheckpoint{}, err
	}
	return adaptiveVerifiedSuccessFingerprintCheckpoint{
		ID: checkpoint.ID, ThreadID: checkpoint.ThreadID, RunID: checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID, CheckpointNS: checkpoint.CheckpointNS,
		RuntimeType: checkpoint.RuntimeType, RuntimeKey: checkpoint.RuntimeKey,
		EnvelopeVersion: checkpoint.EnvelopeVersion, RuntimeDeletedAt: checkpoint.RuntimeDeletedAt,
		ChannelValues: channelValues, ChannelVersions: channelVersions,
		PendingSends: pendingSends, Metadata: metadata, CreatedAt: checkpoint.CreatedAt,
	}, nil
}

func adaptiveVerifiedSuccessOutboxFingerprintValue(
	intent *NotificationOutboxIntent,
) (*string, error) {
	if intent == nil {
		return nil, nil
	}
	event, err := domainnotification.CanonicalizeEvent(intent.Event)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(adaptiveVerifiedSuccessOutboxFingerprint{
		EventID: event.EventID, EventType: string(event.EventType),
		AggregateType: event.AggregateType, AggregateID: event.AggregateID,
		AggregateVersion: event.AggregateVersion, OccurredAt: event.OccurredAt.UnixMilli(),
		ActorID: event.ActorID, SpaceID: event.SpaceID,
		RecipientPolicy: string(event.RecipientPolicy), PayloadSchema: event.PayloadSchema,
		Payload: event.Payload,
	})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	fingerprint := fmt.Sprintf("%x", digest[:])
	return &fingerprint, nil
}

func adaptiveVerifiedSuccessPayloadWithFingerprints(
	raw string,
	outboxFingerprint *string,
	finalizeRequestFingerprint string,
) ([]byte, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, fmt.Errorf("verification payload is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("verification payload has trailing content")
	}
	fields["outbox_fingerprint"] = nil
	if outboxFingerprint != nil {
		fields["outbox_fingerprint"] = *outboxFingerprint
	}
	fields["finalize_request_fingerprint"] = finalizeRequestFingerprint
	return json.Marshal(fields)
}

func adaptiveVerifiedSuccessFinalizeFingerprintValue(
	req FinalizeRunSuccessRequest,
	normalized adaptiveVerifiedSuccessNormalizedFinalize,
	terminalRun *runPO,
	terminalAttempt *runAttemptPO,
	verificationPO *runEventPO,
	completionPO *runEventPO,
	selectedCheckpoint *entity.Checkpoint,
	outboxFingerprint *string,
	titleUpdated bool,
	committedThreadTitle string,
) (string, error) {
	runProjection, err := adaptiveVerifiedSuccessFingerprintRunPO(terminalRun)
	if err != nil {
		return "", err
	}
	attemptProjection, err := adaptiveVerifiedSuccessFingerprintAttemptPO(terminalAttempt)
	if err != nil {
		return "", err
	}
	verificationProjection, err := adaptiveVerifiedSuccessFingerprintEventPO(verificationPO)
	if err != nil {
		return "", err
	}
	completionProjection, err := adaptiveVerifiedSuccessFingerprintEventPO(completionPO)
	if err != nil {
		return "", err
	}
	messageMetadata, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(normalized.messagePO.Metadata)
	if err != nil {
		return "", err
	}
	var titleProjection *adaptiveVerifiedSuccessFingerprintEvent
	if normalized.titleEventPO != nil {
		value, eventErr := adaptiveVerifiedSuccessFingerprintEventPO(normalized.titleEventPO)
		if eventErr != nil {
			return "", eventErr
		}
		titleProjection = &value
	}
	primaryCheckpoint, err := adaptiveVerifiedSuccessFingerprintCheckpointValue(
		normalized.terminalCheckpoint,
	)
	if err != nil {
		return "", err
	}
	var fallbackCheckpoint *adaptiveVerifiedSuccessFingerprintCheckpoint
	if normalized.terminalCheckpointOnTitleConflict != nil {
		value, checkpointErr := adaptiveVerifiedSuccessFingerprintCheckpointValue(
			normalized.terminalCheckpointOnTitleConflict,
		)
		if checkpointErr != nil {
			return "", checkpointErr
		}
		fallbackCheckpoint = &value
	}
	selectedProjection, err := adaptiveVerifiedSuccessFingerprintCheckpointValue(selectedCheckpoint)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(adaptiveVerifiedSuccessFinalizeFingerprint{
		RunID: req.RunID, ExecutionGeneration: req.ExecutionGeneration, Now: normalized.now,
		TerminalRun: runProjection, TerminalAttempt: attemptProjection,
		Decision:   adaptiveVerifiedSuccessFingerprintAuthorityValue(req.AdaptiveGate.Decision),
		Evidence:   adaptiveVerifiedSuccessFingerprintAuthorityValue(req.AdaptiveGate.Evidence),
		DecisionID: req.AdaptiveGate.DecisionID, DecisionRevision: req.AdaptiveGate.DecisionRevision,
		Verification: verificationProjection, Completion: completionProjection,
		Message: adaptiveVerifiedSuccessFingerprintMessage{
			ID: normalized.message.ID, ThreadID: normalized.message.ThreadID,
			RunID: normalized.message.RunID, Role: string(normalized.message.Role),
			Content: normalized.message.Content, Metadata: messageMetadata,
			CreatedAt: normalized.message.CreatedAt,
		},
		TitleEvent:          titleProjection,
		ExpectedThreadTitle: strings.TrimSpace(req.ExpectedThreadTitle),
		ThreadTitle:         strings.TrimSpace(req.ThreadTitle),
		TitleUpdated:        titleUpdated, CommittedThreadTitle: committedThreadTitle,
		PrimaryCheckpoint: primaryCheckpoint, FallbackCheckpoint: fallbackCheckpoint,
		SelectedCheckpoint: selectedProjection, OutboxFingerprint: outboxFingerprint,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:]), nil
}

func lockAdaptiveVerifiedSuccessEventByID(tx *gorm.DB, eventID int64) (*runEventPO, error) {
	query := tx.Where("id = ?", eventID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var event runEventPO
	if err := query.First(&event).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

func lockAdaptiveVerifiedSuccessMessageByID(tx *gorm.DB, messageID int64) (*messagePO, error) {
	query := tx.Where("id = ?", messageID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var message messagePO
	if err := query.First(&message).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, adaptiveVerifiedSuccessConflictf("stored message is missing")
		}
		return nil, err
	}
	return &message, nil
}

func lockAdaptiveVerifiedSuccessOptionalEventByID(
	tx *gorm.DB,
	eventID int64,
) (*runEventPO, bool, error) {
	query := tx.Where("id = ?", eventID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var event runEventPO
	if err := query.First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &event, true, nil
}

func adaptiveVerifiedSuccessFingerprintEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func adaptiveVerifiedSuccessFingerprintMessagePO(
	message *messagePO,
) (adaptiveVerifiedSuccessFingerprintMessage, error) {
	if message == nil {
		return adaptiveVerifiedSuccessFingerprintMessage{}, fmt.Errorf("message is required")
	}
	metadata, err := adaptiveVerifiedSuccessCanonicalOptionalJSON(message.Metadata)
	if err != nil {
		return adaptiveVerifiedSuccessFingerprintMessage{}, err
	}
	return adaptiveVerifiedSuccessFingerprintMessage{
		ID: message.ID, ThreadID: message.ThreadID, RunID: message.RunID,
		Role: message.Role, Content: message.Content, Metadata: metadata,
		CreatedAt: message.CreatedAt,
	}, nil
}

func buildAdaptiveVerifiedSuccessCompletionPO(
	req FinalizeRunSuccessRequest,
	normalized adaptiveVerifiedSuccessNormalizedFinalize,
	attempt *runAttemptPO,
	completionSequence uint64,
) (*runEventPO, error) {
	if attempt == nil || normalized.completionEvent == nil || normalized.completionEventPO == nil ||
		req.JournalEvent == nil {
		return nil, adaptiveVerifiedSuccessConflictf("completion authority is incomplete")
	}
	switch entity.JournalProjectionState(attempt.ProjectionState) {
	case entity.JournalProjectionStateHealthy:
		journalCandidate := *req.JournalEvent
		journalCandidate.ID = normalized.completionEvent.ID
		journalCandidate.ThreadID = normalized.completionEvent.ThreadID
		journalCandidate.RunID = normalized.completionEvent.RunID
		journalCandidate.JournalRunID = attempt.JournalRunID
		journalCandidate.AttemptID = attempt.AttemptID
		if journalCandidate.CreatedAt <= 0 {
			journalCandidate.CreatedAt = normalized.completionEvent.CreatedAt
		}
		if journalCandidate.OccurredAtUnixNano <= 0 {
			journalCandidate.OccurredAtUnixNano = normalized.now * int64(1_000_000)
		}
		normalizedJournal, err := normalizeJournalEvent(&journalCandidate)
		if err != nil {
			return nil, err
		}
		if err := validateTerminalJournalEvent(
			normalizedJournal,
			entity.RunAttemptStatusCompleted,
		); err != nil {
			return nil, err
		}
		return journalEventToPOWithBase(
			normalizedJournal,
			completionSequence,
			normalized.completionEvent,
		)
	case entity.JournalProjectionStateDegraded:
		completion := *normalized.completionEventPO
		completion.JournalRunID = adaptiveExecutionInt64Pointer(attempt.JournalRunID)
		completion.AttemptID = adaptiveExecutionStringPointer(attempt.AttemptID)
		completion.Sequence = adaptiveExecutionUint64Pointer(completionSequence)
		completion.IdempotencyKey = adaptiveExecutionStringPointer(
			strings.TrimSpace(req.JournalEvent.IdempotencyKey),
		)
		return &completion, nil
	default:
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionSequenceConflict,
			"attempt projection state is invalid",
		)
	}
}

func adaptiveVerifiedSuccessCallerPayloadMatchesDurable(
	caller *adaptiveVerifiedSuccessPayload,
	durable *adaptiveVerifiedSuccessPayload,
) bool {
	if caller == nil || durable == nil {
		return false
	}
	durableFields := make(map[string]any, len(durable.canonicalFields)-2)
	for key, value := range durable.canonicalFields {
		if key != "outbox_fingerprint" && key != "finalize_request_fingerprint" {
			durableFields[key] = value
		}
	}
	callerJSON, callerErr := json.Marshal(caller.canonicalFields)
	durableJSON, durableErr := json.Marshal(durableFields)
	return callerErr == nil && durableErr == nil && string(callerJSON) == string(durableJSON)
}

func validateAdaptiveVerifiedSuccessCommittedMismatch(
	ctx context.Context,
	tx *gorm.DB,
	req FinalizeRunSuccessRequest,
	normalized adaptiveVerifiedSuccessNormalizedFinalize,
	callerPayload *adaptiveVerifiedSuccessPayload,
	verification *runEventPO,
	executionRun *runPO,
	thread *threadPO,
	attempt *runAttemptPO,
	evidence *adaptiveVerifiedSuccessLockedAuthority,
	outboxFingerprint *string,
) (*FinalizeRunSuccessResult, error) {
	gate := req.AdaptiveGate
	if gate == nil || verification == nil || executionRun == nil || thread == nil ||
		attempt == nil || evidence == nil || evidence.metadata == nil {
		return nil, adaptiveVerifiedSuccessConflictf("committed verification authority is incomplete")
	}
	verificationSequence := evidence.authority.EventSequence + 1
	if verification.ID != gate.VerificationEvent.ID ||
		verification.ThreadID != gate.VerificationEvent.ThreadID ||
		verification.RunID != gate.VerificationEvent.RunID ||
		verification.EventType != gate.VerificationEvent.EventType ||
		verification.CreatedAt != gate.VerificationEvent.CreatedAt ||
		verification.JournalRunID == nil || *verification.JournalRunID != evidence.authority.JournalRunID ||
		verification.AttemptID == nil || *verification.AttemptID != evidence.authority.AttemptID ||
		verification.Sequence == nil || *verification.Sequence != verificationSequence ||
		verification.IdempotencyKey == nil || *verification.IdempotencyKey != gate.VerificationIdempotencyKey ||
		verification.SnapshotID == nil || *verification.SnapshotID != evidence.metadata.CheckpointFingerprint {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionSequenceConflict,
			"stored verification coupling drift",
		)
	}
	durablePayload, err := decodeAdaptiveVerifiedSuccessPayload(string(verification.Payload), true)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictf("stored verification payload is invalid: %v", err)
	}
	if !adaptiveVerifiedSuccessCallerPayloadMatchesDurable(callerPayload, durablePayload) {
		return nil, adaptiveVerifiedSuccessConflictf("stored verification caller payload drift")
	}
	if outboxFingerprint == nil {
		if durablePayload.OutboxFingerprint != nil {
			return nil, adaptiveVerifiedSuccessConflictf("stored outbox_fingerprint presence drift")
		}
	} else if durablePayload.OutboxFingerprint == nil ||
		*durablePayload.OutboxFingerprint != *outboxFingerprint {
		return nil, adaptiveVerifiedSuccessConflictf("stored outbox_fingerprint drift")
	}

	completionSequence := verificationSequence + 1
	if attempt.Status != string(entity.RunAttemptStatusCompleted) || attempt.ActiveSlot != nil ||
		attempt.NextSequence != completionSequence+1 ||
		attempt.LastCommittedSequence != verificationSequence || attempt.TerminalEventID == nil ||
		*attempt.TerminalEventID != req.CompletionEvent.ID || attempt.EndedAt == nil ||
		*attempt.EndedAt != normalized.now || attempt.UpdatedAt != normalized.now {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionAttemptConflict,
			"stored terminal attempt post-image drift",
		)
	}
	if entity.RunStatus(executionRun.Status) != entity.RunStatusSucceeded ||
		executionRun.EndedAt != normalized.now || executionRun.UpdatedAt != normalized.now ||
		executionRun.WorkerID != "" || executionRun.LeaseOwner != nil || executionRun.LeaseToken != nil ||
		executionRun.LeaseExpiresAt != nil || executionRun.HeartbeatAt != nil ||
		executionRun.CancelRequestedAt != nil {
		return nil, adaptiveVerifiedSuccessConflictf("stored terminal run post-image drift")
	}

	message, err := lockAdaptiveVerifiedSuccessMessageByID(tx, normalized.message.ID)
	if err != nil {
		return nil, err
	}
	storedMessageProjection, err := adaptiveVerifiedSuccessFingerprintMessagePO(message)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictf("stored message is invalid: %v", err)
	}
	expectedMessageProjection, err := adaptiveVerifiedSuccessFingerprintMessagePO(normalized.messagePO)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictf("requested message is invalid: %v", err)
	}
	if !adaptiveVerifiedSuccessFingerprintEqual(storedMessageProjection, expectedMessageProjection) {
		return nil, adaptiveVerifiedSuccessConflictf("stored message drift")
	}

	expectedTitle := strings.TrimSpace(req.ExpectedThreadTitle)
	requestedTitle := strings.TrimSpace(req.ThreadTitle)
	titleChangeRequested := requestedTitle != "" && requestedTitle != expectedTitle
	var titleEvent *runEventPO
	titleUpdated := false
	if normalized.titleEventPO != nil {
		var found bool
		titleEvent, found, err = lockAdaptiveVerifiedSuccessOptionalEventByID(
			tx,
			normalized.titleEventPO.ID,
		)
		if err != nil {
			return nil, err
		}
		if found {
			storedTitleProjection, projectionErr := adaptiveVerifiedSuccessFingerprintEventPO(titleEvent)
			if projectionErr != nil {
				return nil, adaptiveVerifiedSuccessConflictf(
					"stored title event is invalid: %v",
					projectionErr,
				)
			}
			expectedTitleProjection, projectionErr := adaptiveVerifiedSuccessFingerprintEventPO(
				normalized.titleEventPO,
			)
			if projectionErr != nil {
				return nil, adaptiveVerifiedSuccessConflictf(
					"requested title event is invalid: %v",
					projectionErr,
				)
			}
			if !adaptiveVerifiedSuccessFingerprintEqual(storedTitleProjection, expectedTitleProjection) {
				return nil, adaptiveVerifiedSuccessConflictf("stored title event drift")
			}
			if !titleChangeRequested || thread.Title != requestedTitle {
				return nil, adaptiveVerifiedSuccessConflictf("stored thread title drift")
			}
			titleUpdated = true
		}
	}

	completion, err := lockAdaptiveVerifiedSuccessEventByID(tx, req.CompletionEvent.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, adaptiveVerifiedSuccessConflictCausef(
				ErrAdaptiveExecutionSequenceConflict,
				"stored completion coupling is missing",
			)
		}
		return nil, err
	}
	if completion.JournalRunID == nil || *completion.JournalRunID != evidence.authority.JournalRunID ||
		completion.AttemptID == nil || *completion.AttemptID != evidence.authority.AttemptID ||
		completion.Sequence == nil || *completion.Sequence != completionSequence ||
		completion.IdempotencyKey == nil ||
		*completion.IdempotencyKey != strings.TrimSpace(req.JournalEvent.IdempotencyKey) ||
		completion.ID != req.CompletionEvent.ID || completion.ThreadID != req.CompletionEvent.ThreadID ||
		completion.RunID != req.CompletionEvent.RunID {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionSequenceConflict,
			"stored completion coupling drift",
		)
	}
	expectedCompletion, err := buildAdaptiveVerifiedSuccessCompletionPO(
		req,
		normalized,
		attempt,
		completionSequence,
	)
	if err != nil {
		return nil, err
	}
	storedCompletionProjection, err := adaptiveVerifiedSuccessFingerprintEventPO(completion)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictf("stored completion is invalid: %v", err)
	}
	expectedCompletionProjection, err := adaptiveVerifiedSuccessFingerprintEventPO(expectedCompletion)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictf("requested completion is invalid: %v", err)
	}
	if !adaptiveVerifiedSuccessFingerprintEqual(storedCompletionProjection, expectedCompletionProjection) {
		return nil, adaptiveVerifiedSuccessConflictf("stored completion drift")
	}

	selectedCheckpoint := normalized.terminalCheckpoint
	if titleChangeRequested && !titleUpdated &&
		normalized.terminalCheckpointOnTitleConflict != nil {
		selectedCheckpoint = normalized.terminalCheckpointOnTitleConflict
	}
	storedCheckpoint, err := lockAdaptiveVerifiedSuccessCheckpoint(tx, selectedCheckpoint.ID)
	if err != nil {
		return nil, err
	}
	storedCheckpointProjection, err := adaptiveVerifiedSuccessFingerprintCheckpointValue(
		storedCheckpoint.toEntity(),
	)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"stored terminal checkpoint is invalid: %v",
			err,
		)
	}
	expectedCheckpointProjection, err := adaptiveVerifiedSuccessFingerprintCheckpointValue(
		selectedCheckpoint,
	)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"requested terminal checkpoint is invalid: %v",
			err,
		)
	}
	if !adaptiveVerifiedSuccessFingerprintEqual(
		storedCheckpointProjection,
		expectedCheckpointProjection,
	) {
		return nil, adaptiveVerifiedSuccessConflictCausef(
			ErrAdaptiveExecutionCheckpointConflict,
			"stored terminal checkpoint drift",
		)
	}

	verificationForFingerprint := *verification
	verificationPayload, err := adaptiveVerifiedSuccessPayloadWithFingerprints(
		gate.VerificationEvent.Payload,
		outboxFingerprint,
		"",
	)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictf("rebuild verification fingerprint payload: %v", err)
	}
	verificationForFingerprint.Payload = verificationPayload
	expectedFinalizeFingerprint, err := adaptiveVerifiedSuccessFinalizeFingerprintValue(
		req,
		normalized,
		executionRun,
		attempt,
		&verificationForFingerprint,
		expectedCompletion,
		selectedCheckpoint,
		outboxFingerprint,
		titleUpdated,
		thread.Title,
	)
	if err != nil {
		return nil, adaptiveVerifiedSuccessConflictf("rebuild finalize_request_fingerprint: %v", err)
	}
	if durablePayload.FinalizeRequestFingerprint != expectedFinalizeFingerprint {
		return nil, adaptiveVerifiedSuccessConflictf("stored finalize_request_fingerprint drift")
	}

	if req.OutboxIntent != nil {
		inserted, appendErr := req.OutboxIntent.AppendWithResult(
			ctx,
			tx,
			req.OutboxIntent.Event,
		)
		if appendErr != nil {
			return nil, adaptiveVerifiedSuccessConflictCausef(
				appendErr,
				"stored outbox identity drift",
			)
		}
		if inserted {
			return nil, adaptiveVerifiedSuccessConflictf("stored outbox row is missing")
		}
	}

	result := &FinalizeRunSuccessResult{
		Run:                executionRun.toEntity(),
		Message:            message.toEntity(),
		CompletionEvent:    completion.toEntity(),
		TerminalCheckpoint: storedCheckpoint.toEntity(),
		TitleUpdated:       titleUpdated,
		VerificationEvent:  verification.toEntity(),
		Replayed:           true,
	}
	if titleUpdated {
		result.TitleEvent = titleEvent.toEntity()
	}
	return result, nil
}

func (r *threadRepository) finalizeAdaptiveVerifiedRunSuccess(
	ctx context.Context,
	req FinalizeRunSuccessRequest,
	normalized adaptiveVerifiedSuccessNormalizedFinalize,
) (*FinalizeRunSuccessResult, error) {
	gate := req.AdaptiveGate
	if r == nil || r.db == nil || gate == nil {
		return nil, adaptiveVerifiedSuccessConflictf("verified success repository state is missing")
	}
	callerPayload, err := decodeAdaptiveVerifiedSuccessPayload(gate.VerificationEvent.Payload, false)
	if err != nil {
		return nil, err
	}
	outboxFingerprint, err := adaptiveVerifiedSuccessOutboxFingerprintValue(req.OutboxIntent)
	if err != nil {
		return nil, adaptiveVerifiedSuccessInvalidf("outbox event is invalid: %v", err)
	}

	var committed *FinalizeRunSuccessResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		journalRun, err := lockAdaptiveExecutionRunIdentity(tx, gate.Evidence.JournalRunID)
		if err != nil {
			if errors.Is(err, ErrRunLeaseLost) {
				return adaptiveVerifiedSuccessConflictCausef(
					err,
					"journal root run %d is missing",
					gate.Evidence.JournalRunID,
				)
			}
			return err
		}
		if journalRun.ID != gate.Evidence.JournalRunID ||
			journalRun.ThreadID != gate.Evidence.ThreadID || !isJournalRootRun(journalRun) {
			return adaptiveVerifiedSuccessConflictf("journal root identity drift")
		}
		executionRun := journalRun
		if req.RunID != journalRun.ID {
			executionRun, err = lockAdaptiveExecutionRunIdentity(tx, req.RunID)
			if err != nil {
				if errors.Is(err, ErrRunLeaseLost) {
					return adaptiveVerifiedSuccessConflictCausef(
						err,
						"execution run %d is missing",
						req.RunID,
					)
				}
				return err
			}
		}
		if executionRun.ID != req.RunID || executionRun.ThreadID != gate.Evidence.ThreadID ||
			executionRun.SpaceID != journalRun.SpaceID || executionRun.CreatorID != journalRun.CreatorID {
			return adaptiveVerifiedSuccessConflictf("execution run identity drift")
		}

		thread, err := lockThreadForUpdate(tx, gate.Evidence.ThreadID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return adaptiveVerifiedSuccessConflictCausef(
					err,
					"thread %d is missing",
					gate.Evidence.ThreadID,
				)
			}
			return err
		}
		if thread.ID != gate.Evidence.ThreadID || thread.SpaceID != executionRun.SpaceID ||
			thread.CreatorID != executionRun.CreatorID {
			return adaptiveVerifiedSuccessConflictf("thread tenant identity drift")
		}

		attempt, err := lockAdaptiveExecutionAttemptIdentity(
			tx,
			gate.Evidence.JournalRunID,
			gate.Evidence.AttemptID,
		)
		if err != nil {
			if errors.Is(err, ErrAdaptiveExecutionAttemptConflict) {
				return adaptiveVerifiedSuccessConflictCausef(
					err,
					"attempt %s is missing",
					gate.Evidence.AttemptID,
				)
			}
			return err
		}
		if attempt.ThreadID != gate.Evidence.ThreadID || attempt.JournalRunID != gate.Evidence.JournalRunID ||
			attempt.ExecutionRunID != req.RunID || attempt.AttemptID != gate.Evidence.AttemptID ||
			!adaptiveExecutionStringPointersEqual(attempt.SourceAttemptID, gate.Evidence.SourceAttemptID) ||
			!adaptiveExecutionInt64PointersEqual(attempt.SourceCheckpointID, gate.Evidence.SourceCheckpointID) {
			return adaptiveVerifiedSuccessConflictCausef(
				ErrAdaptiveExecutionAttemptConflict,
				"attempt identity drift",
			)
		}

		replayEvent, tupleErr := lockAdaptiveExecutionBoundaryEventTuple(
			tx,
			gate.Evidence.JournalRunID,
			gate.Evidence.AttemptID,
			gate.VerificationIdempotencyKey,
		)
		committedReplay := tupleErr == nil
		if tupleErr != nil && !errors.Is(tupleErr, errAdaptiveExecutionBoundaryTupleMissing) {
			return tupleErr
		}

		if !committedReplay {
			if entity.RunStatus(executionRun.Status) == entity.RunStatusCanceled ||
				executionRun.CancelRequestedAt != nil {
				return adaptiveVerifiedSuccessConflictCausef(
					ErrRunCanceled,
					"run %d rejected late verified success",
					req.RunID,
				)
			}
			if entity.RunStatus(executionRun.Status) != entity.RunStatusRunning ||
				executionRun.ExecutionGeneration != req.ExecutionGeneration ||
				executionRun.LeaseOwner == nil || *executionRun.LeaseOwner != strings.TrimSpace(req.LeaseOwner) ||
				executionRun.LeaseToken == nil || *executionRun.LeaseToken != strings.TrimSpace(req.LeaseToken) ||
				executionRun.LeaseExpiresAt == nil || *executionRun.LeaseExpiresAt <= normalized.now {
				return adaptiveVerifiedSuccessConflictCausef(
					ErrRunLeaseLost,
					"run %d lost the verified success fence",
					req.RunID,
				)
			}
			projectionState := entity.JournalProjectionState(attempt.ProjectionState)
			if attempt.Status != string(entity.RunAttemptStatusRunning) || attempt.ActiveSlot == nil ||
				*attempt.ActiveSlot != 1 ||
				(projectionState != entity.JournalProjectionStateHealthy &&
					projectionState != entity.JournalProjectionStateDegraded) ||
				attempt.LastCommittedSequence != gate.Evidence.EventSequence ||
				attempt.NextSequence != gate.Evidence.EventSequence+1 {
				return adaptiveVerifiedSuccessConflictCausef(
					ErrAdaptiveExecutionSequenceConflict,
					"active attempt cursor drift",
				)
			}
		}

		decision, evidence, err := lockAdaptiveVerifiedSuccessAuthorities(
			tx,
			gate.Decision,
			gate.Evidence,
		)
		if err != nil {
			return err
		}
		if err := validateAdaptiveVerifiedSuccessDecisionPayload(
			decision.event.Payload,
			gate.DecisionID,
			gate.DecisionRevision,
		); err != nil {
			return err
		}
		if decision.authority.EventSequence > evidence.authority.EventSequence {
			return adaptiveVerifiedSuccessConflictf("decision sorts after evidence")
		}
		latestDecision, err := lockLatestAdaptiveVerifiedSuccessDecision(tx, evidence.authority)
		if err != nil {
			return err
		}
		if latestDecision.ID != decision.event.ID || latestDecision.Sequence == nil ||
			*latestDecision.Sequence != decision.authority.EventSequence {
			return adaptiveVerifiedSuccessConflictf("decision is not latest through evidence")
		}

		scopeRun := executionRun
		if gate.Evidence.PlanScopeRunID == journalRun.ID {
			scopeRun = journalRun
		} else if gate.Evidence.PlanScopeRunID != executionRun.ID {
			scopeRun, err = lockAdaptiveExecutionScopeRun(tx, gate.Evidence.PlanScopeRunID)
			if err != nil {
				if errors.Is(err, ErrAdaptiveExecutionPlanScopeConflict) {
					return adaptiveVerifiedSuccessConflictCausef(
						err,
						"plan scope run %d is missing",
						gate.Evidence.PlanScopeRunID,
					)
				}
				return err
			}
		}
		plan, err := lockAdaptiveExecutionPlan(tx, gate.Evidence.PlanScopeRunID)
		if err != nil {
			if errors.Is(err, ErrAdaptiveExecutionPlanScopeConflict) {
				return adaptiveVerifiedSuccessConflictCausef(
					err,
					"plan %d is missing",
					gate.Evidence.PlanScopeRunID,
				)
			}
			return err
		}
		if scopeRun.ID != gate.Evidence.PlanScopeRunID || scopeRun.ThreadID != executionRun.ThreadID ||
			scopeRun.SpaceID != executionRun.SpaceID || scopeRun.CreatorID != executionRun.CreatorID ||
			plan.RunID != scopeRun.ID || plan.ThreadID != scopeRun.ThreadID ||
			plan.SpaceID != scopeRun.SpaceID || plan.UserID != scopeRun.CreatorID {
			return adaptiveVerifiedSuccessConflictf("current plan authority drift")
		}
		if plan.Revision != gate.Evidence.PlanRevision || plan.UpdatedAt != evidence.event.CreatedAt {
			return adaptiveVerifiedSuccessConflictCausef(
				ErrAdaptiveExecutionPlanRevisionConflict,
				"current plan authority drift",
			)
		}
		if _, err := lockAdaptiveVerifiedSuccessPlanItems(
			tx,
			plan,
			evidence,
			callerPayload.ExpectedPlanFingerprint,
		); err != nil {
			return err
		}

		if normalized.message.ThreadID != executionRun.ThreadID ||
			normalized.completionEvent.ThreadID != executionRun.ThreadID ||
			normalized.terminalCheckpoint.ThreadID != executionRun.ThreadID ||
			normalized.terminalCheckpoint.ParentCheckpointID != evidence.checkpoint.ID ||
			normalized.terminalCheckpoint.CheckpointNS != evidence.checkpoint.CheckpointNS ||
			normalized.terminalCheckpoint.RuntimeType != evidence.checkpoint.RuntimeType ||
			normalized.terminalCheckpoint.RuntimeKey != evidence.checkpoint.RuntimeKey ||
			normalized.terminalCheckpoint.EnvelopeVersion != evidence.checkpoint.EnvelopeVersion {
			return adaptiveVerifiedSuccessConflictCausef(
				ErrAdaptiveExecutionCheckpointConflict,
				"terminal checkpoint evidence authority drift",
			)
		}
		if committedReplay {
			replayed, replayErr := validateAdaptiveVerifiedSuccessCommittedMismatch(
				ctx,
				tx,
				req,
				normalized,
				callerPayload,
				replayEvent,
				executionRun,
				thread,
				attempt,
				evidence,
				outboxFingerprint,
			)
			if replayErr != nil {
				return replayErr
			}
			committed = replayed
			return nil
		}

		verificationSequence := attempt.NextSequence
		completionSequence := verificationSequence + 1
		verificationEntity := *gate.VerificationEvent
		verificationPayload, err := adaptiveVerifiedSuccessPayloadWithFingerprints(
			verificationEntity.Payload,
			outboxFingerprint,
			"",
		)
		if err != nil {
			return adaptiveVerifiedSuccessConflictf("prepare verification payload: %v", err)
		}
		verificationEntity.Payload = string(verificationPayload)
		verificationPO, err := runEventToPO(&verificationEntity)
		if err != nil {
			return err
		}
		verificationPO.JournalRunID = adaptiveExecutionInt64Pointer(attempt.JournalRunID)
		verificationPO.AttemptID = adaptiveExecutionStringPointer(attempt.AttemptID)
		verificationPO.Sequence = adaptiveExecutionUint64Pointer(verificationSequence)
		verificationPO.IdempotencyKey = adaptiveExecutionStringPointer(gate.VerificationIdempotencyKey)
		verificationPO.SnapshotID = adaptiveExecutionStringPointer(evidence.metadata.CheckpointFingerprint)

		completionPO, err := buildAdaptiveVerifiedSuccessCompletionPO(
			req,
			normalized,
			attempt,
			completionSequence,
		)
		if err != nil {
			return err
		}

		terminalRun := *executionRun
		terminalRun.Status = string(entity.RunStatusSucceeded)
		terminalRun.ErrorCode = ""
		terminalRun.ErrorMessage = ""
		terminalRun.EndedAt = normalized.now
		terminalRun.UpdatedAt = normalized.now
		terminalRun.WorkerID = ""
		terminalRun.LeaseOwner = nil
		terminalRun.LeaseToken = nil
		terminalRun.LeaseExpiresAt = nil
		terminalRun.HeartbeatAt = nil
		terminalRun.CancelRequestedAt = nil

		terminalAttempt := *attempt
		terminalAttempt.Status = string(entity.RunAttemptStatusCompleted)
		terminalAttempt.ActiveSlot = nil
		terminalAttempt.NextSequence = completionSequence + 1
		terminalAttempt.LastCommittedSequence = verificationSequence
		terminalAttempt.TerminalEventID = adaptiveExecutionInt64Pointer(normalized.completionEvent.ID)
		terminalAttempt.EndedAt = adaptiveExecutionInt64Pointer(normalized.now)
		terminalAttempt.UpdatedAt = normalized.now

		runUpdates := map[string]any{
			"status": string(entity.RunStatusSucceeded), "error_code": "", "error_message": "",
			"ended_at": normalized.now, "updated_at": normalized.now,
		}
		clearRunLeaseUpdates(runUpdates)
		runUpdate := activeRunLeaseQuery(
			tx.Model(&runPO{}),
			req.RunID,
			req.LeaseOwner,
			req.LeaseToken,
			req.ExecutionGeneration,
			normalized.now,
		).Where("cancel_requested_at IS NULL").Updates(runUpdates)
		if runUpdate.Error != nil {
			return runUpdate.Error
		}
		if runUpdate.RowsAffected != 1 {
			return adaptiveVerifiedSuccessConflictCausef(ErrRunLeaseLost, "run success CAS failed")
		}
		if err := tx.Create(normalized.messagePO).Error; err != nil {
			return err
		}

		result := &FinalizeRunSuccessResult{}
		expectedTitle := strings.TrimSpace(req.ExpectedThreadTitle)
		threadTitle := strings.TrimSpace(req.ThreadTitle)
		committedThreadTitle := thread.Title
		if threadTitle != "" && threadTitle != expectedTitle {
			titleUpdate := tx.Model(&threadPO{}).
				Where("id = ? AND title = ?", thread.ID, expectedTitle).
				Updates(map[string]any{"title": threadTitle, "updated_at": normalized.now})
			if titleUpdate.Error != nil {
				return titleUpdate.Error
			}
			result.TitleUpdated = titleUpdate.RowsAffected == 1
			if result.TitleUpdated {
				if normalized.titleEvent == nil || normalized.titleEventPO == nil {
					return adaptiveVerifiedSuccessConflictf("title event is required")
				}
				if err := createBaseRunEvent(tx, normalized.titleEventPO); err != nil {
					return err
				}
				result.TitleEvent = normalized.titleEvent
				committedThreadTitle = threadTitle
			}
		}

		selectedCheckpoint := normalized.terminalCheckpoint
		selectedCheckpointPO := normalized.terminalCheckpointPO
		if threadTitle != "" && threadTitle != expectedTitle && !result.TitleUpdated &&
			normalized.terminalCheckpointOnTitleConflict != nil {
			selectedCheckpoint = normalized.terminalCheckpointOnTitleConflict
			selectedCheckpointPO = normalized.terminalCheckpointOnTitleConflictPO
		}
		finalizeFingerprint, err := adaptiveVerifiedSuccessFinalizeFingerprintValue(
			req,
			normalized,
			&terminalRun,
			&terminalAttempt,
			verificationPO,
			completionPO,
			selectedCheckpoint,
			outboxFingerprint,
			result.TitleUpdated,
			committedThreadTitle,
		)
		if err != nil {
			return adaptiveVerifiedSuccessConflictf("finalize fingerprint is invalid: %v", err)
		}
		durablePayload, err := adaptiveVerifiedSuccessPayloadWithFingerprints(
			gate.VerificationEvent.Payload,
			outboxFingerprint,
			finalizeFingerprint,
		)
		if err != nil {
			return err
		}
		budgetPayload, err := adaptiveVerifiedSuccessPayloadWithFingerprints(
			gate.VerificationEvent.Payload,
			outboxFingerprint,
			strings.Repeat("0", sha256.Size*2),
		)
		if err != nil || len(durablePayload) != len(budgetPayload) ||
			len(durablePayload) > adaptiveVerifiedSuccessMaxPayloadBytes {
			return adaptiveVerifiedSuccessConflictf("durable verification payload budget drift")
		}
		verificationPO.Payload = durablePayload
		verificationEntity.Payload = string(durablePayload)

		if err := createBaseRunEvent(tx, verificationPO); err != nil {
			return err
		}
		if err := tx.Create(completionPO).Error; err != nil {
			return err
		}
		attemptUpdate := tx.Model(&runAttemptPO{}).
			Where(
				"id = ? AND status = ? AND active_slot = ? AND next_sequence = ? AND last_committed_sequence = ?",
				attempt.ID,
				entity.RunAttemptStatusRunning,
				1,
				attempt.NextSequence,
				attempt.LastCommittedSequence,
			).
			Updates(map[string]any{
				"status": string(entity.RunAttemptStatusCompleted), "active_slot": nil,
				"next_sequence":           terminalAttempt.NextSequence,
				"last_committed_sequence": terminalAttempt.LastCommittedSequence,
				"terminal_event_id":       terminalAttempt.TerminalEventID,
				"ended_at":                normalized.now, "updated_at": normalized.now,
			})
		if attemptUpdate.Error != nil {
			return attemptUpdate.Error
		}
		if attemptUpdate.RowsAffected != 1 {
			return adaptiveVerifiedSuccessConflictCausef(
				ErrAdaptiveExecutionSequenceConflict,
				"attempt terminal CAS failed",
			)
		}
		if selectedCheckpointPO != nil {
			if err := tx.Create(selectedCheckpointPO).Error; err != nil {
				return err
			}
			result.TerminalCheckpoint = selectedCheckpoint
		}
		if req.OutboxIntent != nil {
			inserted, appendErr := req.OutboxIntent.AppendWithResult(
				ctx,
				tx,
				req.OutboxIntent.Event,
			)
			if appendErr != nil {
				return appendErr
			}
			if !inserted {
				return adaptiveVerifiedSuccessConflictf("outbox row was not inserted")
			}
		}

		result.Run = terminalRun.toEntity()
		result.Message = normalized.message
		result.VerificationEvent = &verificationEntity
		result.CompletionEvent = normalized.completionEvent
		result.Replayed = false
		committed = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return committed, nil
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
