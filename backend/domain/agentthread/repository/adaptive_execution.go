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
	"io"
	"math"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

var (
	ErrAdaptiveExecutionBoundaryInvalid         = errors.New("adaptive execution boundary is invalid")
	ErrAdaptiveExecutionVerifiedSuccessInvalid  = errors.New("adaptive execution verified success invalid")
	ErrAdaptiveExecutionVerifiedSuccessConflict = errors.New("adaptive execution verified success conflict")
	ErrAdaptiveExecutionAttemptConflict         = errors.New("adaptive execution attempt conflict")
	ErrAdaptiveExecutionLineageConflict         = errors.New("adaptive execution lineage conflict")
	ErrAdaptiveExecutionPlanScopeConflict       = errors.New("adaptive execution plan scope conflict")
	ErrAdaptiveExecutionCheckpointConflict      = errors.New("adaptive execution checkpoint conflict")
	ErrAdaptiveExecutionPlanRevisionConflict    = errors.New("adaptive execution plan revision conflict")
	ErrAdaptiveExecutionPlanItemVersionConflict = errors.New("adaptive execution plan item version conflict")
	ErrAdaptiveExecutionSequenceConflict        = errors.New("adaptive execution sequence conflict")
	ErrAdaptiveExecutionReplayConflict          = errors.New("adaptive execution replay conflict")
	ErrAdaptiveExecutionBoundaryNotFound        = errors.New("adaptive execution boundary is not found")
	ErrAdaptiveExecutionRecoveryConflict        = errors.New("adaptive execution recovery conflict")
	ErrAdaptiveExecutionBootstrapInvalid        = errors.New("adaptive execution bootstrap is invalid")
	ErrAdaptiveExecutionBootstrapNotFound       = errors.New("adaptive execution bootstrap is not found")
	ErrAdaptiveExecutionBootstrapConflict       = errors.New("adaptive execution bootstrap conflict")
	ErrAdaptiveExecutionReservedFact            = errors.New("adaptive execution reserved fact")
)

const adaptiveVerifiedSuccessMaxPayloadBytes = 64 * 1024

type AdaptivePlanItemMutation struct {
	ExpectedVersion int64
	NextItem        *entity.AgentRunPlanItem
}

type AdaptivePlanMutation struct {
	PlanScopeRunID        int64
	ExpectedRevision      int64
	NextRevision          int64
	ExpectedHighWatermark int64
	NextHighWatermark     int64
	Items                 []AdaptivePlanItemMutation
}

type CommitAdaptiveExecutionBoundaryResult struct {
	Event                 *entity.RunEvent
	Checkpoint            *entity.Checkpoint
	Plan                  *entity.AgentRunPlan
	Items                 []*entity.AgentRunPlanItem
	LastCommittedSequence uint64
	Authority             AdaptiveExecutionBoundaryAuthority
	Replayed              bool
}

type AdaptiveExecutionBoundaryAuthority struct {
	ThreadID            int64
	ExecutionRunID      int64
	ExecutionGeneration uint64
	JournalRunID        int64
	AttemptID           string
	SourceAttemptID     *string
	SourceCheckpointID  *int64
	EventID             int64
	EventSequence       uint64
	IdempotencyKey      string
	CheckpointID        int64
	PlanScopeRunID      int64
	PlanRevision        int64
	PlanHighWatermark   int64
	PlanItemFingerprint string
}

type AdaptiveVerifiedSuccessGate struct {
	Decision                   AdaptiveExecutionBoundaryAuthority
	Evidence                   AdaptiveExecutionBoundaryAuthority
	DecisionID                 string
	DecisionRevision           int64
	VerificationEvent          *entity.RunEvent
	VerificationIdempotencyKey string
}

type adaptiveVerifiedSuccessPayload struct {
	Schema                     string  `json:"schema"`
	VerificationID             string  `json:"verification_id"`
	ExecutionRunID             int64   `json:"execution_run_id"`
	JournalRunID               int64   `json:"journal_run_id"`
	AttemptID                  string  `json:"attempt_id"`
	ExecutionGeneration        uint64  `json:"execution_generation"`
	DecisionID                 string  `json:"decision_id"`
	DecisionRevision           int64   `json:"decision_revision"`
	ExpectedPlanRevision       int64   `json:"expected_plan_revision"`
	ExpectedPlanFingerprint    string  `json:"expected_plan_fingerprint"`
	VerifiedCheckpointID       int64   `json:"verified_checkpoint_id"`
	EvidenceHeadEventID        int64   `json:"evidence_head_event_id"`
	OutboxFingerprint          *string `json:"outbox_fingerprint"`
	FinalizeRequestFingerprint string  `json:"finalize_request_fingerprint"`
	Status                     string  `json:"status"`
	CreatedAt                  int64   `json:"created_at"`

	canonicalFields map[string]any
}

func validateAdaptiveVerifiedSuccessGate(req FinalizeRunSuccessRequest) error {
	gate := req.AdaptiveGate
	if gate == nil {
		return nil
	}
	if req.RunID <= 0 || req.ExecutionGeneration == 0 || req.Now <= 0 {
		return adaptiveVerifiedSuccessInvalidf("outer success identity is invalid")
	}
	if req.OutboxIntent != nil && req.OutboxIntent.AppendWithResult == nil {
		return adaptiveVerifiedSuccessInvalidf("outbox append-with-result callback is required")
	}
	if err := validateAdaptiveVerifiedSuccessAuthority(gate.Decision); err != nil {
		return err
	}
	if err := validateAdaptiveVerifiedSuccessAuthority(gate.Evidence); err != nil {
		return err
	}
	if !sameAdaptiveVerifiedSuccessAuthorityScope(gate.Decision, gate.Evidence) ||
		gate.Decision.ThreadID != gate.Evidence.ThreadID ||
		gate.Decision.ExecutionRunID != req.RunID ||
		gate.Decision.ExecutionGeneration != req.ExecutionGeneration ||
		gate.Decision.EventSequence > gate.Evidence.EventSequence ||
		gate.Evidence.EventSequence > math.MaxUint64-3 {
		return adaptiveVerifiedSuccessInvalidf("decision and evidence authority drift")
	}
	decisionID := strings.TrimSpace(gate.DecisionID)
	verificationKey := strings.TrimSpace(gate.VerificationIdempotencyKey)
	if decisionID == "" || decisionID != gate.DecisionID || len([]byte(decisionID)) > 191 ||
		gate.DecisionRevision <= 0 || verificationKey == "" ||
		verificationKey != gate.VerificationIdempotencyKey || len([]byte(verificationKey)) > 191 {
		return adaptiveVerifiedSuccessInvalidf("decision or verification identity is invalid")
	}

	verification := gate.VerificationEvent
	if verification == nil || verification.ID <= 0 ||
		verification.ThreadID != gate.Evidence.ThreadID || verification.RunID != req.RunID ||
		verification.EventType != "adaptive.verification" || verification.CreatedAt != req.Now {
		return adaptiveVerifiedSuccessInvalidf("verification event is invalid")
	}
	payload, err := decodeAdaptiveVerifiedSuccessPayload(verification.Payload, false)
	if err != nil {
		return err
	}
	if payload.VerificationID != verificationKey ||
		payload.ExecutionRunID != req.RunID ||
		payload.JournalRunID != gate.Evidence.JournalRunID ||
		payload.AttemptID != gate.Evidence.AttemptID ||
		payload.ExecutionGeneration != req.ExecutionGeneration ||
		payload.DecisionID != decisionID || payload.DecisionRevision != gate.DecisionRevision ||
		payload.ExpectedPlanRevision != gate.Evidence.PlanRevision ||
		payload.VerifiedCheckpointID != gate.Evidence.CheckpointID ||
		payload.EvidenceHeadEventID != gate.Evidence.EventID || payload.CreatedAt != req.Now {
		return adaptiveVerifiedSuccessInvalidf("verification payload authority drift")
	}
	budgetFields := make(map[string]any, len(payload.canonicalFields)+2)
	for key, value := range payload.canonicalFields {
		budgetFields[key] = value
	}
	budgetFields["outbox_fingerprint"] = nil
	if req.OutboxIntent != nil {
		budgetFields["outbox_fingerprint"] = strings.Repeat("0", 64)
	}
	budgetFields["finalize_request_fingerprint"] = strings.Repeat("0", 64)
	budgetPayload, err := json.Marshal(budgetFields)
	if err != nil || len(budgetPayload) > adaptiveVerifiedSuccessMaxPayloadBytes {
		return adaptiveVerifiedSuccessInvalidf("verification payload exceeds durable budget")
	}

	completion := req.CompletionEvent
	if completion == nil || completion.ID <= verification.ID ||
		completion.ThreadID != gate.Evidence.ThreadID || completion.RunID != req.RunID ||
		completion.EventType != "run.completed" {
		return adaptiveVerifiedSuccessInvalidf("completion event is invalid")
	}
	if _, err := runEventToPO(completion); err != nil {
		return adaptiveVerifiedSuccessInvalidf("completion event payload is invalid: %v", err)
	}
	if req.TitleEvent != nil {
		title := req.TitleEvent
		if title.ID <= 0 || title.ID >= verification.ID ||
			title.ThreadID != gate.Evidence.ThreadID || title.RunID != req.RunID ||
			title.EventType != "context.thread_title_updated" {
			return adaptiveVerifiedSuccessInvalidf("title event is invalid")
		}
		if _, err := runEventToPO(title); err != nil {
			return adaptiveVerifiedSuccessInvalidf("title event payload is invalid: %v", err)
		}
	}

	journal := req.JournalEvent
	if journal == nil {
		return adaptiveVerifiedSuccessInvalidf("completion journal event is required")
	}
	journalKey := strings.TrimSpace(journal.IdempotencyKey)
	if journalKey == "" || len([]byte(journalKey)) > 191 || journalKey == verificationKey {
		return adaptiveVerifiedSuccessInvalidf("completion journal key is invalid")
	}
	journalCandidate := *journal
	journalCandidate.ID = completion.ID
	journalCandidate.ThreadID = completion.ThreadID
	journalCandidate.RunID = completion.RunID
	journalCandidate.JournalRunID = gate.Evidence.JournalRunID
	journalCandidate.AttemptID = gate.Evidence.AttemptID
	if journalCandidate.CreatedAt <= 0 {
		journalCandidate.CreatedAt = req.Now
	}
	if journalCandidate.OccurredAtUnixNano <= 0 {
		if req.Now > math.MaxInt64/int64(time.Millisecond) {
			return adaptiveVerifiedSuccessInvalidf("completion journal timestamp overflows nanoseconds")
		}
		journalCandidate.OccurredAtUnixNano = req.Now * int64(time.Millisecond)
	}
	normalizedJournal, err := normalizeJournalEvent(&journalCandidate)
	if err != nil {
		return adaptiveVerifiedSuccessInvalidf("completion journal event is invalid: %v", err)
	}
	if err := validateTerminalJournalEvent(normalizedJournal, entity.RunAttemptStatusCompleted); err != nil {
		return adaptiveVerifiedSuccessInvalidf("completion journal event is invalid: %v", err)
	}

	if err := validateAdaptiveVerifiedSuccessTerminalCheckpoint(
		req.TerminalCheckpoint,
		gate.Evidence,
	); err != nil {
		return err
	}
	if req.TerminalCheckpointOnTitleConflict != nil {
		if err := validateAdaptiveVerifiedSuccessTerminalCheckpoint(
			req.TerminalCheckpointOnTitleConflict,
			gate.Evidence,
		); err != nil {
			return err
		}
		if !sameTerminalCheckpointEntityIdentity(
			req.TerminalCheckpoint,
			req.TerminalCheckpointOnTitleConflict,
		) {
			return adaptiveVerifiedSuccessInvalidf("terminal checkpoint fallback identity drift")
		}
	}
	return nil
}

func decodeAdaptiveVerifiedSuccessPayload(raw string, durable bool) (*adaptiveVerifiedSuccessPayload, error) {
	if len([]byte(raw)) > adaptiveVerifiedSuccessMaxPayloadBytes {
		return nil, adaptiveVerifiedSuccessInvalidf("verification payload exceeds %d bytes", adaptiveVerifiedSuccessMaxPayloadBytes)
	}
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, adaptiveVerifiedSuccessInvalidf("verification payload is not a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, adaptiveVerifiedSuccessInvalidf("verification payload has trailing content")
	}
	required := []string{
		"schema", "verification_id", "execution_run_id", "journal_run_id", "attempt_id",
		"execution_generation", "decision_id", "decision_revision", "expected_plan_revision",
		"expected_plan_fingerprint", "verified_checkpoint_id", "evidence_head_event_id",
		"status", "created_at",
	}
	for _, key := range required {
		if _, exists := fields[key]; !exists {
			return nil, adaptiveVerifiedSuccessInvalidf("verification payload field %s is missing", key)
		}
	}
	_, outboxPresent := fields["outbox_fingerprint"]
	_, finalizePresent := fields["finalize_request_fingerprint"]
	if durable {
		if !outboxPresent || !finalizePresent {
			return nil, adaptiveVerifiedSuccessInvalidf("durable verification fingerprints are missing")
		}
	} else if outboxPresent || finalizePresent {
		return nil, adaptiveVerifiedSuccessInvalidf("caller supplied server-owned verification fingerprint")
	}
	canonical, err := json.Marshal(fields)
	if err != nil || len(canonical) > adaptiveVerifiedSuccessMaxPayloadBytes {
		return nil, adaptiveVerifiedSuccessInvalidf("verification payload canonical form is invalid")
	}
	var payload adaptiveVerifiedSuccessPayload
	if err := json.Unmarshal(canonical, &payload); err != nil {
		return nil, adaptiveVerifiedSuccessInvalidf("verification payload fields are invalid: %v", err)
	}
	if payload.Schema != "workbench-adaptive-verification.v1" ||
		strings.TrimSpace(payload.VerificationID) == "" || len([]byte(payload.VerificationID)) > 191 ||
		payload.ExecutionRunID <= 0 || payload.JournalRunID <= 0 ||
		strings.TrimSpace(payload.AttemptID) == "" || len([]byte(payload.AttemptID)) > 64 ||
		payload.ExecutionGeneration == 0 ||
		strings.TrimSpace(payload.DecisionID) == "" || len([]byte(payload.DecisionID)) > 191 ||
		payload.DecisionRevision <= 0 || payload.ExpectedPlanRevision <= 0 ||
		!validAdaptiveExecutionFingerprint(payload.ExpectedPlanFingerprint) ||
		payload.VerifiedCheckpointID <= 0 || payload.EvidenceHeadEventID <= 0 ||
		payload.Status != "passed" || payload.CreatedAt <= 0 {
		return nil, adaptiveVerifiedSuccessInvalidf("verification payload fields are invalid")
	}
	if durable {
		if payload.OutboxFingerprint != nil && !validAdaptiveExecutionFingerprint(*payload.OutboxFingerprint) {
			return nil, adaptiveVerifiedSuccessInvalidf("durable outbox fingerprint is invalid")
		}
		if !validAdaptiveExecutionFingerprint(payload.FinalizeRequestFingerprint) {
			return nil, adaptiveVerifiedSuccessInvalidf("durable finalize request fingerprint is invalid")
		}
	}
	payload.canonicalFields = fields
	return &payload, nil
}

func validateAdaptiveVerifiedSuccessAuthority(authority AdaptiveExecutionBoundaryAuthority) error {
	sourceAttemptPresent := authority.SourceAttemptID != nil
	sourceCheckpointPresent := authority.SourceCheckpointID != nil
	if authority.ThreadID <= 0 || authority.ExecutionRunID <= 0 || authority.ExecutionGeneration == 0 ||
		authority.JournalRunID <= 0 || strings.TrimSpace(authority.AttemptID) == "" ||
		len([]byte(authority.AttemptID)) > 64 || authority.EventID <= 0 || authority.EventSequence == 0 ||
		strings.TrimSpace(authority.IdempotencyKey) == "" || len([]byte(authority.IdempotencyKey)) > 191 ||
		authority.CheckpointID <= 0 || authority.PlanScopeRunID <= 0 || authority.PlanRevision <= 0 ||
		authority.PlanHighWatermark < 0 ||
		!validAdaptiveExecutionFingerprint(authority.PlanItemFingerprint) ||
		sourceAttemptPresent != sourceCheckpointPresent ||
		(sourceAttemptPresent && (strings.TrimSpace(*authority.SourceAttemptID) == "" ||
			len([]byte(*authority.SourceAttemptID)) > 64 || *authority.SourceCheckpointID <= 0)) {
		return adaptiveVerifiedSuccessInvalidf("adaptive boundary authority is invalid")
	}
	return nil
}

func sameAdaptiveVerifiedSuccessAuthorityScope(
	decision AdaptiveExecutionBoundaryAuthority,
	evidence AdaptiveExecutionBoundaryAuthority,
) bool {
	return decision.ExecutionRunID == evidence.ExecutionRunID &&
		decision.ExecutionGeneration == evidence.ExecutionGeneration &&
		decision.JournalRunID == evidence.JournalRunID &&
		decision.AttemptID == evidence.AttemptID &&
		decision.PlanScopeRunID == evidence.PlanScopeRunID &&
		sameAdaptiveVerifiedSuccessStringPointer(decision.SourceAttemptID, evidence.SourceAttemptID) &&
		sameAdaptiveVerifiedSuccessInt64Pointer(decision.SourceCheckpointID, evidence.SourceCheckpointID)
}

func sameAdaptiveVerifiedSuccessStringPointer(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameAdaptiveVerifiedSuccessInt64Pointer(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validateAdaptiveVerifiedSuccessTerminalCheckpoint(
	checkpoint *entity.Checkpoint,
	evidence AdaptiveExecutionBoundaryAuthority,
) error {
	if checkpoint == nil || checkpoint.ID <= 0 || checkpoint.ThreadID != evidence.ThreadID ||
		checkpoint.RunID != evidence.ExecutionRunID || checkpoint.ParentCheckpointID != evidence.CheckpointID ||
		strings.TrimSpace(checkpoint.CheckpointNS) == "" || checkpoint.RuntimeType != "eino_adk" ||
		strings.TrimSpace(checkpoint.RuntimeKey) == "" || checkpoint.EnvelopeVersion <= 0 ||
		checkpoint.RuntimeDeletedAt != 0 {
		return adaptiveVerifiedSuccessInvalidf("terminal checkpoint is invalid")
	}
	if _, err := checkpointToPO(checkpoint); err != nil {
		return adaptiveVerifiedSuccessInvalidf("terminal checkpoint payload is invalid: %v", err)
	}
	return nil
}

func adaptiveVerifiedSuccessInvalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrAdaptiveExecutionVerifiedSuccessInvalid, fmt.Sprintf(format, args...))
}

type AdaptiveExecutionRepository interface {
	CommitAdaptiveExecutionBoundary(
		ctx context.Context,
		req CommitAdaptiveExecutionBoundaryRequest,
	) (*CommitAdaptiveExecutionBoundaryResult, error)
	ReadAdaptiveExecutionBoundary(
		ctx context.Context,
		req ReadAdaptiveExecutionBoundaryRequest,
	) (*CommitAdaptiveExecutionBoundaryResult, error)
	ReadAdaptiveExecutionRecoverySource(
		ctx context.Context,
		req ReadAdaptiveExecutionRecoverySourceRequest,
	) (*CommitAdaptiveExecutionBoundaryResult, error)
	CommitAdaptiveExecutionBootstrap(
		ctx context.Context,
		req CommitAdaptiveExecutionBootstrapRequest,
	) (*CommitAdaptiveExecutionBootstrapResult, error)
	ReadAdaptiveExecutionBootstrap(
		ctx context.Context,
		req ReadAdaptiveExecutionBootstrapRequest,
	) (*CommitAdaptiveExecutionBootstrapResult, error)
}

// CommitAdaptiveExecutionBootstrapRequest is intentionally repository-private
// surface: no handler, service, or runtime entry point receives it in C2.
// Candidate IDs are only used for the first durable write; exact replay is
// located by the derived event and checkpoint identities.
type CommitAdaptiveExecutionBootstrapRequest struct {
	ThreadID       int64
	ExecutionRunID int64
	JournalRunID   int64
	AttemptID      string
	LeaseOwner     string
	LeaseToken     string
	OperationKey   string
	Generation     uint64
	Now            int64
	FactCreatedAt  int64

	Admission entity.AdaptiveAdmissionSnapshot
	Decision  entity.ExecutionDecision

	AdmissionEventID int64
	DecisionEventID  int64
	CheckpointID     int64
}

type ReadAdaptiveExecutionBootstrapRequest struct {
	ThreadID       int64
	ExecutionRunID int64
	JournalRunID   int64
	AttemptID      string
}

// AdaptiveExecutionBootstrapAuthority contains only immutable identifiers and
// digests. It never retains lease material or the raw operation key.
type AdaptiveExecutionBootstrapAuthority struct {
	ThreadID                  int64
	ExecutionRunID            int64
	JournalRunID              int64
	AttemptID                 string
	ExecutionGeneration       uint64
	AdmissionEventID          int64
	DecisionEventID           int64
	CheckpointID              int64
	AdmissionEventKey         string
	DecisionEventKey          string
	AdmissionDigest           string
	DecisionDigest            string
	AdmissionEventFingerprint string
	DecisionEventFingerprint  string
	OperationKeyDigest        string
	CheckpointFingerprint     string
	FactCreatedAt             int64
}

type CommitAdaptiveExecutionBootstrapResult struct {
	Admission      entity.AdaptiveAdmissionSnapshot
	Decision       entity.ExecutionDecision
	AdmissionEvent *entity.RunEvent
	DecisionEvent  *entity.RunEvent
	Checkpoint     *entity.Checkpoint
	Authority      AdaptiveExecutionBootstrapAuthority
	Replayed       bool
}

type CommitAdaptiveExecutionBoundaryRequest struct {
	ThreadID       int64
	ExecutionRunID int64
	JournalRunID   int64
	AttemptID      string
	Generation     uint64
	LeaseOwner     string
	LeaseToken     string
	Now            int64
	IdempotencyKey string
	Event          *entity.RunEvent
	Checkpoint     *entity.Checkpoint
	PlanMutation   *AdaptivePlanMutation
}

type ReadAdaptiveExecutionBoundaryRequest struct {
	ThreadID               int64
	ExecutionRunID         int64
	JournalRunID           int64
	AttemptID              string
	Generation             uint64
	RuntimeKey             string
	IdempotencyKey         string
	ExpectedMutationDigest string
}

type ReadAdaptiveExecutionRecoverySourceRequest struct {
	ThreadID        int64
	JournalRunID    int64
	TargetAttemptID string
}

func validateAdaptiveExecutionBoundaryRequest(req CommitAdaptiveExecutionBoundaryRequest) error {
	if req.ThreadID <= 0 || req.ExecutionRunID <= 0 || req.JournalRunID <= 0 ||
		strings.TrimSpace(req.AttemptID) == "" || len([]byte(req.AttemptID)) > 64 || req.Generation == 0 ||
		strings.TrimSpace(req.LeaseOwner) == "" || strings.TrimSpace(req.LeaseToken) == "" ||
		req.Now <= 0 || strings.TrimSpace(req.IdempotencyKey) == "" || len([]byte(req.IdempotencyKey)) > 191 ||
		req.Event == nil || req.Checkpoint == nil {
		return fmt.Errorf("%w: required identity is missing", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if req.Event.ThreadID != req.ThreadID || req.Event.RunID != req.ExecutionRunID {
		return fmt.Errorf("%w: event identity drift", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if req.Checkpoint.ThreadID != req.ThreadID || req.Checkpoint.RunID != req.ExecutionRunID {
		return fmt.Errorf("%w: checkpoint identity drift", ErrAdaptiveExecutionBoundaryInvalid)
	}
	return nil
}

func validateAdaptiveExecutionRecoverySourceRequest(req ReadAdaptiveExecutionRecoverySourceRequest) error {
	if req.ThreadID <= 0 || req.JournalRunID <= 0 ||
		strings.TrimSpace(req.TargetAttemptID) == "" || len([]byte(req.TargetAttemptID)) > 64 {
		return fmt.Errorf("%w: required recovery identity is missing", ErrAdaptiveExecutionBoundaryInvalid)
	}
	return nil
}

func validateAdaptiveExecutionBoundaryReadRequest(req ReadAdaptiveExecutionBoundaryRequest) error {
	if req.ThreadID <= 0 || req.ExecutionRunID <= 0 || req.JournalRunID <= 0 ||
		strings.TrimSpace(req.AttemptID) == "" || len([]byte(req.AttemptID)) > 64 || req.Generation == 0 ||
		strings.TrimSpace(req.RuntimeKey) == "" || len([]byte(req.RuntimeKey)) > 191 ||
		strings.TrimSpace(req.IdempotencyKey) == "" || len([]byte(req.IdempotencyKey)) > 191 ||
		!validAdaptiveExecutionFingerprint(req.ExpectedMutationDigest) {
		return fmt.Errorf("%w: required boundary read identity is missing", ErrAdaptiveExecutionBoundaryInvalid)
	}
	return nil
}

func validateAdaptiveExecutionMutationRequest(req CommitAdaptiveExecutionBoundaryRequest) error {
	if err := validateAdaptiveExecutionBoundaryRequest(req); err != nil {
		return err
	}
	mutation := req.PlanMutation
	if mutation == nil || mutation.PlanScopeRunID <= 0 || mutation.ExpectedRevision < 0 ||
		mutation.ExpectedRevision == math.MaxInt64 ||
		mutation.NextRevision != mutation.ExpectedRevision+1 ||
		mutation.ExpectedHighWatermark < 0 || mutation.ExpectedHighWatermark == math.MaxInt64 ||
		mutation.NextHighWatermark < mutation.ExpectedHighWatermark ||
		mutation.NextHighWatermark > mutation.ExpectedHighWatermark+1 ||
		len(mutation.Items) < 1 || len(mutation.Items) > 32 {
		return fmt.Errorf("%w: plan mutation is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if req.Event.ID <= 0 || strings.TrimSpace(req.Event.EventType) == "" || req.Event.CreatedAt != req.Now {
		return fmt.Errorf("%w: event is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if req.Event.EventType == adaptiveBootstrapAdmissionEventType {
		return ErrAdaptiveExecutionReservedFact
	}
	if _, err := runEventToPO(req.Event); err != nil {
		return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
	}
	if err := rejectAdaptiveVerifiedSuccessOutsideFinalizer(req.Event); err != nil {
		return err
	}
	if req.Checkpoint.ID <= 0 || strings.TrimSpace(req.Checkpoint.CheckpointNS) == "" ||
		req.Checkpoint.RuntimeType != "eino_adk" || strings.TrimSpace(req.Checkpoint.RuntimeKey) == "" ||
		req.Checkpoint.EnvelopeVersion <= 0 || req.Checkpoint.RuntimeDeletedAt != 0 ||
		req.Checkpoint.CreatedAt != req.Now {
		return fmt.Errorf("%w: checkpoint is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if _, err := checkpointToPO(req.Checkpoint); err != nil {
		return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
	}
	metadata := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(req.Checkpoint.Metadata), &metadata); err != nil || metadata == nil {
		return fmt.Errorf("%w: checkpoint metadata must be a JSON object", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if _, exists := metadata["adaptive_execution"]; exists {
		return fmt.Errorf("%w: checkpoint metadata contains reserved namespace", ErrAdaptiveExecutionBoundaryInvalid)
	}

	itemIDs := make(map[int64]struct{}, len(mutation.Items))
	taskIDs := make(map[int64]struct{}, len(mutation.Items))
	createdItems := 0
	for _, itemMutation := range mutation.Items {
		item := itemMutation.NextItem
		if itemMutation.ExpectedVersion < 0 || itemMutation.ExpectedVersion == math.MaxInt64 || item == nil ||
			item.ID <= 0 || item.RunID <= 0 || item.TaskID <= 0 ||
			item.RunID != mutation.PlanScopeRunID || item.Version != itemMutation.ExpectedVersion+1 ||
			item.TaskID > mutation.NextHighWatermark {
			return fmt.Errorf("%w: plan item mutation is invalid", ErrAdaptiveExecutionBoundaryInvalid)
		}
		if itemMutation.ExpectedVersion == 0 {
			createdItems++
		}
		if _, exists := itemIDs[item.ID]; exists {
			return fmt.Errorf("%w: duplicate plan item id", ErrAdaptiveExecutionBoundaryInvalid)
		}
		if _, exists := taskIDs[item.TaskID]; exists {
			return fmt.Errorf("%w: duplicate plan task id", ErrAdaptiveExecutionBoundaryInvalid)
		}
		itemIDs[item.ID] = struct{}{}
		taskIDs[item.TaskID] = struct{}{}
		if _, err := agentRunPlanItemToPO(item); err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}
	}
	if createdItems > 1 ||
		(mutation.NextHighWatermark == mutation.ExpectedHighWatermark+1 &&
			(createdItems != 1 || !adaptivePlanMutationCreatesTask(mutation, mutation.NextHighWatermark))) ||
		(mutation.NextHighWatermark == mutation.ExpectedHighWatermark &&
			adaptivePlanMutationCreatesTaskAbove(mutation, mutation.ExpectedHighWatermark)) {
		return fmt.Errorf("%w: plan high watermark mutation is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	return nil
}

func adaptivePlanMutationCreatesTask(mutation *AdaptivePlanMutation, taskID int64) bool {
	for _, item := range mutation.Items {
		if item.ExpectedVersion == 0 && item.NextItem != nil && item.NextItem.TaskID == taskID {
			return true
		}
	}
	return false
}

func adaptivePlanMutationCreatesTaskAbove(mutation *AdaptivePlanMutation, highWatermark int64) bool {
	for _, item := range mutation.Items {
		if item.ExpectedVersion == 0 && item.NextItem != nil && item.NextItem.TaskID > highWatermark {
			return true
		}
	}
	return false
}

func rejectAdaptiveVerifiedSuccessOutsideFinalizer(event *entity.RunEvent) error {
	if event == nil || event.EventType != "adaptive.verification" {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(event.Payload)))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return fmt.Errorf("%w: adaptive verification payload is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: adaptive verification payload has trailing content", ErrAdaptiveExecutionBoundaryInvalid)
	}
	statusValue, exists := fields["status"]
	if !exists {
		return nil
	}
	status, ok := statusValue.(string)
	if !ok {
		return fmt.Errorf("%w: adaptive verification status is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if status == "passed" {
		return fmt.Errorf(
			"%w: passed adaptive verification is restricted to the success finalizer",
			ErrAdaptiveExecutionBoundaryInvalid,
		)
	}
	return nil
}
