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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/adaptivecontract"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	adaptiveBootstrapAdmissionEventType = "adaptive.admission"
	adaptiveBootstrapDecisionEventType  = "adaptive.decision"
	adaptiveBootstrapRuntimeType        = "workbench_control"
	adaptiveBootstrapCheckpointNS       = "workbench.adaptive.bootstrap"
	adaptiveBootstrapMetadataSchema     = "workbench-adaptive-bootstrap.v1"
	adaptiveBootstrapKeySchema          = "workbench-adaptive-bootstrap.v1"
	adaptiveBootstrapMetadataMaxBytes   = 64 * 1024
)

type adaptiveExecutionBootstrapNormalizedRequest struct {
	request CommitAdaptiveExecutionBootstrapRequest

	admissionCanonical []byte
	decisionCanonical  []byte
	admissionDigest    string
	decisionDigest     string

	operationDigest   string
	admissionEventKey string
	decisionEventKey  string
	checkpointKey     string
}

// adaptiveBootstrapMetadata is deliberately a closed, server-owned envelope.
// It contains durable authority references only, never a lease or raw operation
// key, and is the single object persisted in the control checkpoint metadata.
type adaptiveBootstrapMetadata struct {
	Schema                    string  `json:"schema"`
	ThreadID                  int64   `json:"thread_id"`
	ExecutionRunID            int64   `json:"execution_run_id"`
	JournalRunID              int64   `json:"journal_run_id"`
	AttemptID                 string  `json:"attempt_id"`
	ExecutionGeneration       uint64  `json:"execution_generation"`
	SourceAttemptID           *string `json:"source_attempt_id"`
	SourceCheckpointID        *int64  `json:"source_checkpoint_id"`
	RecoveryIdempotencyKey    *string `json:"recovery_idempotency_key"`
	AdmissionEventID          int64   `json:"admission_event_id"`
	DecisionEventID           int64   `json:"decision_event_id"`
	AdmissionEventKey         string  `json:"admission_event_key"`
	DecisionEventKey          string  `json:"decision_event_key"`
	AdmissionDigest           string  `json:"admission_digest"`
	DecisionDigest            string  `json:"decision_digest"`
	AdmissionEventFingerprint string  `json:"admission_event_fingerprint"`
	DecisionEventFingerprint  string  `json:"decision_event_fingerprint"`
	OperationKeyDigest        string  `json:"operation_key_digest"`
	CheckpointFingerprint     string  `json:"checkpoint_fingerprint"`
	FactCreatedAt             int64   `json:"fact_created_at"`
}

type adaptiveBootstrapMetadataEnvelope struct {
	AdaptiveBootstrap adaptiveBootstrapMetadata `json:"adaptive_bootstrap"`
}

type adaptiveBootstrapCheckpointFingerprintValue struct {
	ID                 int64                     `json:"id"`
	ThreadID           int64                     `json:"thread_id"`
	RunID              int64                     `json:"run_id"`
	ParentCheckpointID int64                     `json:"parent_checkpoint_id"`
	CheckpointNS       string                    `json:"checkpoint_ns"`
	RuntimeType        string                    `json:"runtime_type"`
	RuntimeKey         string                    `json:"runtime_key"`
	EnvelopeVersion    int32                     `json:"envelope_version"`
	RuntimeDeletedAt   int64                     `json:"runtime_deleted_at"`
	ChannelValues      json.RawMessage           `json:"channel_values"`
	ChannelVersions    json.RawMessage           `json:"channel_versions"`
	PendingSends       json.RawMessage           `json:"pending_sends"`
	Metadata           adaptiveBootstrapMetadata `json:"adaptive_bootstrap"`
	CreatedAt          int64                     `json:"created_at"`
}

// adaptiveBootstrapEventFingerprintValue deliberately omits SnapshotID. The
// checkpoint fingerprint is stored in SnapshotID, so including it would form a
// self-referential hash cycle. Every other immutable event scalar and JSON
// field is included.
type adaptiveBootstrapEventFingerprintValue struct {
	ID                 int64           `json:"id"`
	ThreadID           int64           `json:"thread_id"`
	RunID              int64           `json:"run_id"`
	JournalRunID       *int64          `json:"journal_run_id"`
	AttemptID          *string         `json:"attempt_id"`
	Sequence           *uint64         `json:"sequence"`
	IdempotencyKey     *string         `json:"idempotency_key"`
	ParentEventID      *int64          `json:"parent_event_id"`
	SchemaVersion      *string         `json:"schema_version"`
	Status             *string         `json:"status"`
	OccurredAtUnixNano *int64          `json:"occurred_at_unix_nano"`
	Visibility         *string         `json:"visibility"`
	PayloadVersion     *string         `json:"payload_version"`
	TraceID            *string         `json:"trace_id"`
	ActionID           *string         `json:"action_id"`
	Phase              *string         `json:"phase"`
	Operation          *string         `json:"operation"`
	Target             *string         `json:"target"`
	Milestone          *string         `json:"milestone"`
	EventType          string          `json:"event_type"`
	JournalEventType   *string         `json:"journal_event_type"`
	Payload            json.RawMessage `json:"payload"`
	JournalPayload     json.RawMessage `json:"journal_payload"`
	CreatedAt          int64           `json:"created_at"`
}

func (r *threadRepository) CommitAdaptiveExecutionBootstrap(
	ctx context.Context,
	req CommitAdaptiveExecutionBootstrapRequest,
) (*CommitAdaptiveExecutionBootstrapResult, error) {
	normalized, err := normalizeAdaptiveExecutionBootstrapRequest(req)
	if err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, bootstrapInvalidf("repository database is missing")
	}

	var committed *CommitAdaptiveExecutionBootstrapResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return commitAdaptiveExecutionBootstrap(tx, normalized, &committed)
	})
	if err != nil {
		return nil, err
	}
	return committed, nil
}

func (r *threadRepository) ReadAdaptiveExecutionBootstrap(
	ctx context.Context,
	req ReadAdaptiveExecutionBootstrapRequest,
) (*CommitAdaptiveExecutionBootstrapResult, error) {
	if err := validateReadAdaptiveExecutionBootstrapRequest(req); err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, bootstrapInvalidf("repository database is missing")
	}

	var result *CommitAdaptiveExecutionBootstrapResult
	read := func(tx *gorm.DB) error {
		loaded, err := loadAdaptiveExecutionBootstrapResult(tx, req, nil)
		if err != nil {
			return err
		}
		result = loaded
		return nil
	}
	db := r.db.WithContext(ctx)
	options := adaptiveExecutionRecoveryTransactionOptions(r.db)
	if options == nil {
		err := db.Transaction(read)
		if err != nil {
			return nil, err
		}
	} else {
		err := db.Transaction(read, options)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *threadRepository) ReadAdaptiveExecutionBootstrapByRun(
	ctx context.Context,
	req ReadAdaptiveExecutionBootstrapByRunRequest,
) (*CommitAdaptiveExecutionBootstrapResult, error) {
	if err := validateReadAdaptiveExecutionBootstrapByRunRequest(req); err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, bootstrapInvalidf("repository database is missing")
	}

	var result *CommitAdaptiveExecutionBootstrapResult
	read := func(tx *gorm.DB) error {
		var attempts []runAttemptPO
		if err := tx.Where("execution_run_id = ?", req.ExecutionRunID).
			Order("id ASC").Limit(2).Find(&attempts).Error; err != nil {
			return err
		}
		if len(attempts) == 0 {
			var eventCount int64
			if err := tx.Model(&runEventPO{}).Where(
				"run_id = ? AND event_type IN ? AND sequence IS NULL AND visibility = ?",
				req.ExecutionRunID,
				[]string{adaptiveBootstrapAdmissionEventType, adaptiveBootstrapDecisionEventType},
				string(entity.JournalVisibilityInternal),
			).Count(&eventCount).Error; err != nil {
				return err
			}
			var checkpointCount int64
			if err := tx.Model(&checkpointPO{}).Where(
				"run_id = ? AND runtime_type = ? AND checkpoint_ns = ?",
				req.ExecutionRunID,
				adaptiveBootstrapRuntimeType,
				adaptiveBootstrapCheckpointNS,
			).Count(&checkpointCount).Error; err != nil {
				return err
			}
			if eventCount != 0 || checkpointCount != 0 {
				return bootstrapConflictf("bootstrap authority remains after execution attempt removal")
			}
			return fmt.Errorf("%w: execution attempt is missing", ErrAdaptiveExecutionBootstrapNotFound)
		}
		if len(attempts) != 1 {
			return bootstrapConflictf("execution attempt identity is ambiguous")
		}
		attempt := attempts[0]
		if attempt.ThreadID != req.ThreadID || attempt.ExecutionRunID != req.ExecutionRunID ||
			attempt.JournalRunID <= 0 || !adaptiveBootstrapExactNonEmpty(attempt.AttemptID, 64) {
			return bootstrapConflictf("execution attempt identity drift")
		}
		loaded, err := loadAdaptiveExecutionBootstrapResult(tx, ReadAdaptiveExecutionBootstrapRequest{
			ThreadID:       req.ThreadID,
			ExecutionRunID: req.ExecutionRunID,
			JournalRunID:   attempt.JournalRunID,
			AttemptID:      attempt.AttemptID,
		}, nil)
		if err != nil {
			return err
		}
		result = loaded
		return nil
	}
	db := r.db.WithContext(ctx)
	options := adaptiveExecutionRecoveryTransactionOptions(r.db)
	if options == nil {
		if err := db.Transaction(read); err != nil {
			return nil, err
		}
	} else if err := db.Transaction(read, options); err != nil {
		return nil, err
	}
	return result, nil
}

func normalizeAdaptiveExecutionBootstrapRequest(
	req CommitAdaptiveExecutionBootstrapRequest,
) (*adaptiveExecutionBootstrapNormalizedRequest, error) {
	if req.ThreadID <= 0 || req.ExecutionRunID <= 0 || req.JournalRunID <= 0 ||
		req.Generation == 0 || req.Now <= 0 || req.FactCreatedAt <= 0 ||
		req.FactCreatedAt > math.MaxInt64/int64(1_000_000) ||
		req.AdmissionEventID <= 0 || req.DecisionEventID <= 0 || req.CheckpointID <= 0 ||
		req.AdmissionEventID == req.DecisionEventID ||
		!adaptiveBootstrapExactNonEmpty(req.AttemptID, 64) ||
		!adaptiveBootstrapExactNonEmpty(req.LeaseOwner, 191) ||
		!adaptiveBootstrapExactNonEmpty(req.LeaseToken, 191) ||
		!adaptiveBootstrapExactNonEmpty(req.OperationKey, 191) ||
		req.Decision.CreatedAt != req.FactCreatedAt {
		return nil, bootstrapInvalidf("required bootstrap identity is invalid")
	}
	switch req.Admission.Source {
	case entity.AdaptiveAdmissionSourceFresh, entity.AdaptiveAdmissionSourceTypedInheritance,
		entity.AdaptiveAdmissionSourceLegacyDecoder:
	default:
		return nil, bootstrapInvalidf("bootstrap admission source is unsupported")
	}
	if req.Admission.Source == entity.AdaptiveAdmissionSourceFresh &&
		req.Decision.Decision == entity.ExecutionDecisionExecute &&
		req.Decision.ExecutionShape == entity.ExecutionShapeMultiStep &&
		(req.Decision.PlanScopeRunID == nil || *req.Decision.PlanScopeRunID != req.ExecutionRunID) {
		return nil, bootstrapInvalidf("fresh bootstrap plan scope drift")
	}
	if err := adaptivecontract.ValidateAdaptiveBootstrapPair(
		req.Admission,
		req.Decision,
		adaptivecontract.BootstrapIdentity{
			ExecutionRunID:      req.ExecutionRunID,
			JournalRunID:        req.JournalRunID,
			AttemptID:           req.AttemptID,
			ExecutionGeneration: req.Generation,
			ExpectedPlanScopeRunID: func() int64 {
				if req.Decision.PlanScopeRunID == nil {
					return 0
				}
				return *req.Decision.PlanScopeRunID
			}(),
		},
	); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAdaptiveExecutionBootstrapInvalid, err)
	}
	admissionCanonical, admissionDigest, err := adaptivecontract.EncodeAdaptiveAdmission(req.Admission)
	if err != nil {
		return nil, fmt.Errorf("%w: admission is invalid", ErrAdaptiveExecutionBootstrapInvalid)
	}
	decisionCanonical, decisionDigest, err := adaptivecontract.EncodeExecutionDecision(req.Decision)
	if err != nil {
		return nil, fmt.Errorf("%w: decision is invalid", ErrAdaptiveExecutionBootstrapInvalid)
	}
	operationDigest := adaptiveBootstrapDigest(req.OperationKey)
	checkpointKey := "adaptive-bootstrap-" + adaptiveBootstrapDigest(fmt.Sprintf(
		"%s\n%d\n%s", adaptiveBootstrapKeySchema, req.JournalRunID, req.AttemptID,
	))
	return &adaptiveExecutionBootstrapNormalizedRequest{
		request:            req,
		admissionCanonical: append([]byte(nil), admissionCanonical...),
		decisionCanonical:  append([]byte(nil), decisionCanonical...),
		admissionDigest:    admissionDigest,
		decisionDigest:     decisionDigest,
		operationDigest:    operationDigest,
		admissionEventKey:  adaptiveBootstrapEventKey("admission", operationDigest),
		decisionEventKey:   adaptiveBootstrapEventKey("decision", operationDigest),
		checkpointKey:      checkpointKey,
	}, nil
}

func validateReadAdaptiveExecutionBootstrapRequest(req ReadAdaptiveExecutionBootstrapRequest) error {
	if req.ThreadID <= 0 || req.ExecutionRunID <= 0 || req.JournalRunID <= 0 ||
		!adaptiveBootstrapExactNonEmpty(req.AttemptID, 64) {
		return bootstrapInvalidf("read bootstrap identity is invalid")
	}
	return nil
}

func validateReadAdaptiveExecutionBootstrapByRunRequest(req ReadAdaptiveExecutionBootstrapByRunRequest) error {
	if req.ThreadID <= 0 || req.ExecutionRunID <= 0 {
		return bootstrapInvalidf("read bootstrap by run identity is invalid")
	}
	return nil
}

func commitAdaptiveExecutionBootstrap(
	tx *gorm.DB,
	normalized *adaptiveExecutionBootstrapNormalizedRequest,
	committed **CommitAdaptiveExecutionBootstrapResult,
) error {
	if tx == nil || normalized == nil || committed == nil {
		return bootstrapInvalidf("bootstrap transaction is invalid")
	}
	req := normalized.request
	boundary := CommitAdaptiveExecutionBoundaryRequest{
		ThreadID: req.ThreadID, ExecutionRunID: req.ExecutionRunID, JournalRunID: req.JournalRunID,
		AttemptID: req.AttemptID, Generation: req.Generation, LeaseOwner: req.LeaseOwner,
		LeaseToken: req.LeaseToken, Now: req.Now,
	}
	if _, err := lockThreadForUpdate(tx, req.ThreadID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: thread identity drift", ErrAdaptiveExecutionLineageConflict)
		}
		return err
	}
	journalRun, err := lockAdaptiveExecutionJournalRunIdentity(tx, boundary)
	if err != nil {
		return err
	}
	run := journalRun
	if req.ExecutionRunID != req.JournalRunID {
		run, err = lockAdaptiveExecutionRunIdentity(tx, req.ExecutionRunID)
		if err != nil {
			return err
		}
	}
	attempt, err := lockAdaptiveExecutionAttemptIdentity(tx, req.JournalRunID, req.AttemptID)
	if err != nil {
		return err
	}

	_, admissionFound, err := lockAdaptiveBootstrapEventTuple(tx, req.JournalRunID, req.AttemptID, normalized.admissionEventKey)
	if err != nil {
		return err
	}
	_, decisionFound, err := lockAdaptiveBootstrapEventTuple(tx, req.JournalRunID, req.AttemptID, normalized.decisionEventKey)
	if err != nil {
		return err
	}
	checkpoints, err := lockAdaptiveBootstrapCheckpoints(tx, req.ThreadID, req.ExecutionRunID, normalized.checkpointKey)
	if err != nil {
		return err
	}
	if len(checkpoints) > 1 {
		return bootstrapConflictf("control checkpoint tuple is ambiguous")
	}
	checkpointFound := len(checkpoints) == 1

	if admissionFound && decisionFound && checkpointFound {
		loaded, err := loadAdaptiveExecutionBootstrapResult(tx, ReadAdaptiveExecutionBootstrapRequest{
			ThreadID: req.ThreadID, ExecutionRunID: req.ExecutionRunID, JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
		}, normalized)
		if err != nil {
			return err
		}
		loaded.Replayed = true
		*committed = loaded
		return nil
	}
	if admissionFound || decisionFound || checkpointFound {
		return bootstrapConflictf("bootstrap fact is partial")
	}

	reservedEvents, err := lockAdaptiveBootstrapFactEvents(tx, req.JournalRunID, req.AttemptID)
	if err != nil {
		return err
	}
	if len(reservedEvents) != 0 {
		return bootstrapConflictf("another bootstrap fact already exists for attempt")
	}
	if err := validateAdaptiveExecutionRunFence(run, boundary); err != nil {
		return err
	}
	if err := validateAdaptiveExecutionAttemptFence(attempt, boundary); err != nil {
		return err
	}
	switch req.Admission.Source {
	case entity.AdaptiveAdmissionSourceFresh:
		if attempt.SourceAttemptID != nil || attempt.SourceCheckpointID != nil || attempt.RecoveryIdempotencyKey != nil {
			return bootstrapConflictf("fresh bootstrap attempt lineage is not empty")
		}
	case entity.AdaptiveAdmissionSourceTypedInheritance:
		if err := lockAndValidateAdaptiveBootstrapTypedSource(tx, normalized, attempt); err != nil {
			return err
		}
	case entity.AdaptiveAdmissionSourceLegacyDecoder:
		if err := lockAndValidateAdaptiveBootstrapLegacySource(tx, normalized, attempt); err != nil {
			return err
		}
	}

	checkpoint, admissionEvent, decisionEvent, err := newAdaptiveExecutionBootstrapRows(normalized, attempt)
	if err != nil {
		return err
	}
	if err := createAdaptiveBootstrapReservedRunEvent(tx, admissionEvent); err != nil {
		return bootstrapWriteError(err)
	}
	if err := createAdaptiveBootstrapReservedRunEvent(tx, decisionEvent); err != nil {
		return bootstrapWriteError(err)
	}
	if err := tx.Create(checkpoint).Error; err != nil {
		return bootstrapWriteError(err)
	}
	loaded, err := loadAdaptiveExecutionBootstrapResult(tx, ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: req.ThreadID, ExecutionRunID: req.ExecutionRunID, JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
	}, normalized)
	if err != nil {
		return err
	}
	*committed = loaded
	return nil
}

func lockAdaptiveBootstrapEventTuple(
	tx *gorm.DB,
	journalRunID int64,
	attemptID, key string,
) (*runEventPO, bool, error) {
	query := tx.Where("journal_run_id = ? AND attempt_id = ? AND idempotency_key = ?", journalRunID, attemptID, key)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var event runEventPO
	err := query.First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &event, true, nil
}

// adaptiveBootstrapFactEventQuery selects only the private, unsequenced facts
// owned by bootstrap. Legacy plan-boundary adaptive.decision events are public
// journal events with a sequence and must not make bootstrap authority appear
// incomplete; any additional unsequenced internal fact remains a conflict.
func adaptiveBootstrapFactEventQuery(tx *gorm.DB, journalRunID int64, attemptID string) *gorm.DB {
	return tx.Where(
		"journal_run_id = ? AND attempt_id = ? AND event_type IN ? AND sequence IS NULL AND visibility = ?",
		journalRunID,
		attemptID,
		[]string{adaptiveBootstrapAdmissionEventType, adaptiveBootstrapDecisionEventType},
		string(entity.JournalVisibilityInternal),
	)
}

func lockAdaptiveBootstrapFactEvents(
	tx *gorm.DB,
	journalRunID int64,
	attemptID string,
) ([]runEventPO, error) {
	if tx == nil {
		return nil, bootstrapInvalidf("bootstrap event lock transaction is missing")
	}
	query := adaptiveBootstrapFactEventQuery(tx, journalRunID, attemptID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []runEventPO
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func lockAdaptiveBootstrapCheckpoints(
	tx *gorm.DB,
	threadID, executionRunID int64,
	runtimeKey string,
) ([]checkpointPO, error) {
	query := tx.Where(
		"thread_id = ? AND run_id = ? AND runtime_type = ? AND runtime_key = ?",
		threadID, executionRunID, adaptiveBootstrapRuntimeType, runtimeKey,
	)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []checkpointPO
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func lockAndValidateAdaptiveBootstrapTypedSource(
	tx *gorm.DB,
	normalized *adaptiveExecutionBootstrapNormalizedRequest,
	target *runAttemptPO,
) error {
	if tx == nil || normalized == nil || target == nil {
		return bootstrapInvalidf("typed bootstrap validation is missing")
	}
	req := normalized.request
	if target.SourceAttemptID == nil || target.SourceCheckpointID == nil || target.RecoveryIdempotencyKey == nil {
		return bootstrapConflictf("typed bootstrap lineage is incomplete")
	}
	discovered, err := discoverAdaptiveExecutionSourceAttempt(tx, req.JournalRunID, *target.SourceAttemptID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("source attempt discovery", err)
	}
	sourceRun, err := lockAdaptiveExecutionSourceRun(tx, discovered.ExecutionRunID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("source run", err)
	}
	sourceAttempt, err := lockAdaptiveExecutionSourceAttempt(tx, req.JournalRunID, *target.SourceAttemptID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("source attempt lock", err)
	}
	if sourceAttempt.ID != discovered.ID || sourceAttempt.ThreadID != discovered.ThreadID ||
		sourceAttempt.JournalRunID != discovered.JournalRunID ||
		sourceAttempt.ExecutionRunID != discovered.ExecutionRunID ||
		sourceAttempt.AttemptID != discovered.AttemptID || sourceAttempt.ThreadID != req.ThreadID ||
		sourceAttempt.JournalRunID != req.JournalRunID || sourceAttempt.Ordinal == 0 ||
		target.Ordinal == 0 || sourceAttempt.Ordinal >= target.Ordinal ||
		sourceAttempt.ExecutionRunID == req.ExecutionRunID {
		return bootstrapConflictf("typed bootstrap source attempt drift")
	}
	if sourceRun.ID != sourceAttempt.ExecutionRunID || sourceRun.ThreadID != req.ThreadID {
		return bootstrapConflictf("typed bootstrap source run drift")
	}
	sourceCheckpoint, err := lockAdaptiveExecutionSourceCheckpoint(tx, *target.SourceCheckpointID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("source checkpoint", err)
	}
	if sourceCheckpoint.ThreadID != req.ThreadID || sourceCheckpoint.RunID != sourceAttempt.ExecutionRunID ||
		sourceCheckpoint.CheckpointNS != "eino.adk" || sourceCheckpoint.RuntimeType != "eino_adk" ||
		sourceCheckpoint.RuntimeDeletedAt != 0 {
		return bootstrapConflictf("typed bootstrap source checkpoint drift")
	}
	source, err := loadAdaptiveExecutionBootstrapResult(tx, ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: req.ThreadID, ExecutionRunID: sourceAttempt.ExecutionRunID,
		JournalRunID: req.JournalRunID, AttemptID: sourceAttempt.AttemptID,
	}, nil)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("source facts", err)
	}
	if source == nil || source.Authority.ExecutionGeneration != sourceRun.ExecutionGeneration ||
		source.Decision.PlanScopeRunID == nil || req.Decision.PlanScopeRunID == nil ||
		*req.Decision.PlanScopeRunID != *source.Decision.PlanScopeRunID ||
		(source.Admission.Source != entity.AdaptiveAdmissionSourceFresh &&
			source.Admission.Source != entity.AdaptiveAdmissionSourceTypedInheritance &&
			source.Admission.Source != entity.AdaptiveAdmissionSourceLegacyDecoder) ||
		req.Admission.SourceRunID == nil || *req.Admission.SourceRunID != sourceAttempt.ExecutionRunID ||
		req.Admission.SourceExecutionGeneration == nil ||
		*req.Admission.SourceExecutionGeneration != source.Authority.ExecutionGeneration ||
		req.Admission.FeatureGateEnabled != source.Admission.FeatureGateEnabled ||
		req.Admission.Capabilities != source.Admission.Capabilities || req.Admission.Limits != source.Admission.Limits {
		return bootstrapConflictf("typed bootstrap inherited policy drift")
	}
	return nil
}

func lockAndValidateAdaptiveBootstrapLegacySource(
	tx *gorm.DB,
	normalized *adaptiveExecutionBootstrapNormalizedRequest,
	target *runAttemptPO,
) error {
	if tx == nil || normalized == nil || target == nil {
		return bootstrapInvalidf("legacy bootstrap validation is missing")
	}
	req := normalized.request
	if target.SourceAttemptID == nil {
		return lockAndValidateAdaptiveBootstrapBareLegacySource(tx, normalized, target)
	}
	if target.SourceCheckpointID == nil || target.RecoveryIdempotencyKey == nil {
		return bootstrapConflictf("legacy bootstrap lineage is incomplete")
	}
	discovered, err := discoverAdaptiveExecutionSourceAttempt(tx, req.JournalRunID, *target.SourceAttemptID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("legacy source attempt discovery", err)
	}
	sourceRun, err := lockAdaptiveExecutionSourceRun(tx, discovered.ExecutionRunID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("legacy source run", err)
	}
	sourceAttempt, err := lockAdaptiveExecutionSourceAttempt(tx, req.JournalRunID, *target.SourceAttemptID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("legacy source attempt lock", err)
	}
	if sourceAttempt.ID != discovered.ID || sourceAttempt.ThreadID != discovered.ThreadID ||
		sourceAttempt.JournalRunID != discovered.JournalRunID || sourceAttempt.ExecutionRunID != discovered.ExecutionRunID ||
		sourceAttempt.AttemptID != discovered.AttemptID || sourceAttempt.ThreadID != req.ThreadID ||
		sourceAttempt.JournalRunID != req.JournalRunID || sourceAttempt.Ordinal == 0 || target.Ordinal == 0 ||
		sourceAttempt.Ordinal >= target.Ordinal || sourceAttempt.ExecutionRunID == req.ExecutionRunID ||
		sourceRun.ID != sourceAttempt.ExecutionRunID || sourceRun.ThreadID != req.ThreadID || !isJournalRootRun(sourceRun) ||
		req.Admission.SourceRunID == nil || *req.Admission.SourceRunID != sourceRun.ID ||
		req.Admission.SourceExecutionGeneration == nil ||
		*req.Admission.SourceExecutionGeneration != sourceRun.ExecutionGeneration ||
		adaptiveBootstrapDigestBytes(sourceRun.Config) != req.Admission.SourceConfigDigest {
		return bootstrapConflictf("legacy bootstrap source identity drift")
	}
	if req.Decision.PlanScopeRunID == nil || *req.Decision.PlanScopeRunID != sourceRun.ID {
		return bootstrapConflictf("legacy bootstrap plan scope drift")
	}
	sourceCheckpoint, err := lockAdaptiveExecutionSourceCheckpoint(tx, *target.SourceCheckpointID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("legacy source checkpoint", err)
	}
	if sourceCheckpoint.ThreadID != req.ThreadID || sourceCheckpoint.RunID != sourceAttempt.ExecutionRunID ||
		sourceCheckpoint.CheckpointNS != "eino.adk" || sourceCheckpoint.RuntimeType != "eino_adk" ||
		sourceCheckpoint.RuntimeDeletedAt != 0 {
		return bootstrapConflictf("legacy bootstrap source checkpoint drift")
	}
	_, err = loadAdaptiveExecutionBootstrapResult(tx, ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: req.ThreadID, ExecutionRunID: sourceAttempt.ExecutionRunID,
		JournalRunID: req.JournalRunID, AttemptID: sourceAttempt.AttemptID,
	}, nil)
	if errors.Is(err, ErrAdaptiveExecutionBootstrapNotFound) {
		return nil
	}
	if err == nil || errors.Is(err, ErrAdaptiveExecutionBootstrapInvalid) ||
		errors.Is(err, ErrAdaptiveExecutionBootstrapConflict) {
		return bootstrapConflictf("legacy bootstrap source already has bootstrap authority")
	}
	return fmt.Errorf("legacy bootstrap source facts: %w", err)
}

func lockAndValidateAdaptiveBootstrapBareLegacySource(
	tx *gorm.DB,
	normalized *adaptiveExecutionBootstrapNormalizedRequest,
	target *runAttemptPO,
) error {
	if tx == nil || normalized == nil || target == nil {
		return bootstrapInvalidf("bare legacy bootstrap validation is missing")
	}
	req := normalized.request
	if target.SourceCheckpointID == nil || target.RecoveryIdempotencyKey == nil ||
		target.Ordinal != 1 || target.JournalRunID != req.JournalRunID ||
		target.ProjectionState != string(entity.JournalProjectionStateDisabled) ||
		!adaptiveBootstrapExactNonEmpty(*target.RecoveryIdempotencyKey, 191) ||
		req.Admission.SourceRunID == nil || *req.Admission.SourceRunID != req.JournalRunID ||
		req.Decision.PlanScopeRunID == nil || *req.Decision.PlanScopeRunID != req.JournalRunID {
		return bootstrapConflictf("bare legacy bootstrap lineage is invalid")
	}
	sourceRun, err := lockAdaptiveExecutionSourceRun(tx, req.JournalRunID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("bare legacy source run", err)
	}
	if sourceRun.ID == req.ExecutionRunID || sourceRun.ThreadID != req.ThreadID || !isJournalRootRun(sourceRun) ||
		req.Admission.SourceExecutionGeneration == nil ||
		*req.Admission.SourceExecutionGeneration != sourceRun.ExecutionGeneration ||
		adaptiveBootstrapDigestBytes(sourceRun.Config) != req.Admission.SourceConfigDigest {
		return bootstrapConflictf("bare legacy bootstrap source identity drift")
	}
	var sourceAttemptCount int64
	if err := tx.Model(&runAttemptPO{}).
		Where("execution_run_id = ?", sourceRun.ID).
		Count(&sourceAttemptCount).Error; err != nil {
		return err
	}
	if sourceAttemptCount != 0 {
		return bootstrapConflictf("bare legacy bootstrap source has an attempt")
	}
	sourceCheckpoint, err := lockAdaptiveExecutionSourceCheckpoint(tx, *target.SourceCheckpointID)
	if err != nil {
		return adaptiveBootstrapTypedSourceError("bare legacy source checkpoint", err)
	}
	if sourceCheckpoint.ThreadID != req.ThreadID || sourceCheckpoint.RunID != sourceRun.ID ||
		sourceCheckpoint.CheckpointNS != "eino.adk" || sourceCheckpoint.RuntimeType != "eino_adk" ||
		sourceCheckpoint.RuntimeDeletedAt != 0 {
		return bootstrapConflictf("bare legacy bootstrap source checkpoint drift")
	}
	var sourceFactCount int64
	if err := tx.Model(&runEventPO{}).Where(
		"run_id = ? AND event_type IN ? AND sequence IS NULL AND visibility = ?",
		sourceRun.ID,
		[]string{adaptiveBootstrapAdmissionEventType, adaptiveBootstrapDecisionEventType},
		string(entity.JournalVisibilityInternal),
	).Count(&sourceFactCount).Error; err != nil {
		return err
	}
	var sourceControlCheckpointCount int64
	if err := tx.Model(&checkpointPO{}).Where(
		"thread_id = ? AND run_id = ? AND runtime_type = ? AND checkpoint_ns = ?",
		req.ThreadID, sourceRun.ID, adaptiveBootstrapRuntimeType, adaptiveBootstrapCheckpointNS,
	).Count(&sourceControlCheckpointCount).Error; err != nil {
		return err
	}
	if sourceFactCount != 0 || sourceControlCheckpointCount != 0 {
		return bootstrapConflictf("bare legacy bootstrap source already has bootstrap authority")
	}
	return nil
}

func adaptiveBootstrapTypedSourceError(scope string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrAdaptiveExecutionLineageConflict) ||
		errors.Is(err, ErrAdaptiveExecutionBootstrapInvalid) ||
		errors.Is(err, ErrAdaptiveExecutionBootstrapNotFound) ||
		errors.Is(err, ErrAdaptiveExecutionBootstrapConflict) {
		return bootstrapConflictf("typed bootstrap %s is invalid: %v", scope, err)
	}
	return fmt.Errorf("typed bootstrap %s: %w", scope, err)
}

func validateAdaptiveBootstrapStoredLineage(
	attempt *runAttemptPO,
	metadata adaptiveBootstrapMetadata,
	admission entity.AdaptiveAdmissionSnapshot,
) error {
	if attempt == nil {
		return bootstrapConflictf("bootstrap attempt is missing")
	}
	sourceAttemptPresent := attempt.SourceAttemptID != nil
	sourceCheckpointPresent := attempt.SourceCheckpointID != nil
	recoveryKeyPresent := attempt.RecoveryIdempotencyKey != nil
	if !sourceAttemptPresent && !sourceCheckpointPresent && !recoveryKeyPresent {
		if admission.Source != entity.AdaptiveAdmissionSourceFresh || metadata.SourceAttemptID != nil ||
			metadata.SourceCheckpointID != nil || metadata.RecoveryIdempotencyKey != nil {
			return bootstrapConflictf("fresh bootstrap lineage drift")
		}
		return nil
	}
	if !sourceAttemptPresent && sourceCheckpointPresent && recoveryKeyPresent {
		if admission.Source != entity.AdaptiveAdmissionSourceLegacyDecoder || attempt.Ordinal != 1 ||
			attempt.ProjectionState != string(entity.JournalProjectionStateDisabled) ||
			admission.SourceRunID == nil || *admission.SourceRunID != attempt.JournalRunID ||
			metadata.SourceAttemptID != nil ||
			!adaptiveExecutionInt64PointersEqual(metadata.SourceCheckpointID, attempt.SourceCheckpointID) ||
			!adaptiveExecutionStringPointersEqual(metadata.RecoveryIdempotencyKey, attempt.RecoveryIdempotencyKey) {
			return bootstrapConflictf("bare legacy bootstrap lineage drift")
		}
		return nil
	}
	if sourceAttemptPresent != sourceCheckpointPresent || sourceAttemptPresent != recoveryKeyPresent {
		return bootstrapConflictf("bootstrap attempt lineage is partial")
	}
	if (admission.Source != entity.AdaptiveAdmissionSourceTypedInheritance &&
		admission.Source != entity.AdaptiveAdmissionSourceLegacyDecoder) ||
		!adaptiveExecutionStringPointersEqual(metadata.SourceAttemptID, attempt.SourceAttemptID) ||
		!adaptiveExecutionInt64PointersEqual(metadata.SourceCheckpointID, attempt.SourceCheckpointID) ||
		!adaptiveExecutionStringPointersEqual(metadata.RecoveryIdempotencyKey, attempt.RecoveryIdempotencyKey) {
		return bootstrapConflictf("typed bootstrap lineage drift")
	}
	return nil
}

func newAdaptiveExecutionBootstrapRows(
	normalized *adaptiveExecutionBootstrapNormalizedRequest,
	attempt *runAttemptPO,
) (*checkpointPO, *runEventPO, *runEventPO, error) {
	if normalized == nil || attempt == nil {
		return nil, nil, nil, bootstrapInvalidf("normalized bootstrap is missing")
	}
	req := normalized.request
	metadata := adaptiveBootstrapMetadata{
		Schema:   adaptiveBootstrapMetadataSchema,
		ThreadID: req.ThreadID, ExecutionRunID: req.ExecutionRunID, JournalRunID: req.JournalRunID,
		AttemptID: req.AttemptID, ExecutionGeneration: req.Generation,
		AdmissionEventID: req.AdmissionEventID, DecisionEventID: req.DecisionEventID,
		AdmissionEventKey: normalized.admissionEventKey, DecisionEventKey: normalized.decisionEventKey,
		AdmissionDigest: normalized.admissionDigest, DecisionDigest: normalized.decisionDigest,
		OperationKeyDigest: normalized.operationDigest, FactCreatedAt: req.FactCreatedAt,
		SourceAttemptID:        adaptiveExecutionCloneStringPointer(attempt.SourceAttemptID),
		SourceCheckpointID:     adaptiveExecutionCloneInt64Pointer(attempt.SourceCheckpointID),
		RecoveryIdempotencyKey: adaptiveExecutionCloneStringPointer(attempt.RecoveryIdempotencyKey),
	}
	checkpoint := &checkpointPO{
		ID: req.CheckpointID, ThreadID: req.ThreadID, RunID: req.ExecutionRunID,
		ParentCheckpointID: 0, CheckpointNS: adaptiveBootstrapCheckpointNS,
		RuntimeType: adaptiveBootstrapRuntimeType, RuntimeKey: normalized.checkpointKey,
		EnvelopeVersion: 1, RuntimeDeletedAt: 0,
		ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`),
		CreatedAt: req.FactCreatedAt,
	}
	admission := newAdaptiveBootstrapEventPO(
		normalized, req.AdmissionEventID, adaptiveBootstrapAdmissionEventType,
		normalized.admissionEventKey, normalized.admissionCanonical, "",
	)
	decision := newAdaptiveBootstrapEventPO(
		normalized, req.DecisionEventID, adaptiveBootstrapDecisionEventType,
		normalized.decisionEventKey, normalized.decisionCanonical, "",
	)
	var err error
	metadata.AdmissionEventFingerprint, err = adaptiveBootstrapEventFingerprint(admission)
	if err != nil {
		return nil, nil, nil, bootstrapInvalidf("admission event fingerprint is invalid")
	}
	metadata.DecisionEventFingerprint, err = adaptiveBootstrapEventFingerprint(decision)
	if err != nil {
		return nil, nil, nil, bootstrapInvalidf("decision event fingerprint is invalid")
	}
	fingerprint, err := adaptiveBootstrapCheckpointFingerprint(checkpoint, metadata)
	if err != nil {
		return nil, nil, nil, bootstrapInvalidf("checkpoint fingerprint is invalid")
	}
	metadata.CheckpointFingerprint = fingerprint
	metadataRaw, err := json.Marshal(adaptiveBootstrapMetadataEnvelope{AdaptiveBootstrap: metadata})
	if err != nil || len(metadataRaw) > adaptiveBootstrapMetadataMaxBytes {
		return nil, nil, nil, bootstrapInvalidf("checkpoint metadata is invalid")
	}
	checkpoint.Metadata = metadataRaw
	admission.SnapshotID = adaptiveExecutionStringPointer(fingerprint)
	decision.SnapshotID = adaptiveExecutionStringPointer(fingerprint)
	return checkpoint, admission, decision, nil
}

func newAdaptiveBootstrapEventPO(
	normalized *adaptiveExecutionBootstrapNormalizedRequest,
	eventID int64,
	eventType, key string,
	payload []byte,
	checkpointFingerprint string,
) *runEventPO {
	req := normalized.request
	journalRunID := req.JournalRunID
	attemptID := req.AttemptID
	occurredAt := req.FactCreatedAt * int64(1_000_000)
	schemaVersion := entity.JournalSchemaVersion
	visibility := string(entity.JournalVisibilityInternal)
	payloadVersion := entity.JournalPayloadVersion
	return &runEventPO{
		ID: eventID, ThreadID: req.ThreadID, RunID: req.ExecutionRunID,
		JournalRunID: &journalRunID, AttemptID: &attemptID, IdempotencyKey: adaptiveExecutionStringPointer(key),
		SchemaVersion: &schemaVersion, OccurredAtUnixNano: &occurredAt, Visibility: &visibility,
		PayloadVersion: &payloadVersion, SnapshotID: adaptiveExecutionStringPointer(checkpointFingerprint),
		EventType: eventType, Payload: append([]byte(nil), payload...), CreatedAt: req.FactCreatedAt,
	}
}

func loadAdaptiveExecutionBootstrapResult(
	tx *gorm.DB,
	req ReadAdaptiveExecutionBootstrapRequest,
	expected *adaptiveExecutionBootstrapNormalizedRequest,
) (*CommitAdaptiveExecutionBootstrapResult, error) {
	if tx == nil {
		return nil, bootstrapInvalidf("bootstrap read transaction is missing")
	}
	var attempt runAttemptPO
	err := tx.Where("journal_run_id = ? AND attempt_id = ?", req.JournalRunID, req.AttemptID).First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var eventCount int64
		if err := adaptiveBootstrapFactEventQuery(
			tx.Model(&runEventPO{}), req.JournalRunID, req.AttemptID,
		).Count(&eventCount).Error; err != nil {
			return nil, err
		}
		var checkpointCount int64
		if err := tx.Model(&checkpointPO{}).Where(
			"thread_id = ? AND run_id = ? AND runtime_type = ? AND runtime_key = ?",
			req.ThreadID,
			req.ExecutionRunID,
			adaptiveBootstrapRuntimeType,
			adaptiveBootstrapCheckpointKey(req.JournalRunID, req.AttemptID),
		).Count(&checkpointCount).Error; err != nil {
			return nil, err
		}
		if eventCount != 0 || checkpointCount != 0 {
			return nil, bootstrapConflictf("bootstrap authority remains after attempt removal")
		}
		return nil, fmt.Errorf("%w: attempt is missing", ErrAdaptiveExecutionBootstrapNotFound)
	}
	if err != nil {
		return nil, err
	}
	if attempt.ThreadID != req.ThreadID || attempt.ExecutionRunID != req.ExecutionRunID || attempt.JournalRunID != req.JournalRunID || attempt.AttemptID != req.AttemptID {
		return nil, bootstrapConflictf("attempt identity drift")
	}
	var reservedEvents []runEventPO
	if err := adaptiveBootstrapFactEventQuery(tx, req.JournalRunID, req.AttemptID).
		Order("id ASC").Find(&reservedEvents).Error; err != nil {
		return nil, err
	}
	checkpointKey := adaptiveBootstrapCheckpointKey(req.JournalRunID, req.AttemptID)
	var checkpoints []checkpointPO
	if err := tx.Where(
		"thread_id = ? AND run_id = ? AND runtime_type = ? AND runtime_key = ?",
		req.ThreadID, req.ExecutionRunID, adaptiveBootstrapRuntimeType, checkpointKey,
	).Order("id ASC").Find(&checkpoints).Error; err != nil {
		return nil, err
	}
	if len(checkpoints) == 0 {
		if len(reservedEvents) != 0 {
			return nil, bootstrapConflictf("bootstrap fact is partial")
		}
		return nil, fmt.Errorf("%w: control checkpoint is missing", ErrAdaptiveExecutionBootstrapNotFound)
	}
	if len(checkpoints) != 1 {
		return nil, bootstrapConflictf("control checkpoint tuple is ambiguous")
	}
	checkpoint := &checkpoints[0]
	dialect := tx.Dialector.Name()
	metadata, err := decodeAdaptiveBootstrapStoredMetadata(dialect, checkpoint.Metadata)
	if err != nil {
		return nil, bootstrapConflictf("control checkpoint metadata drift")
	}
	if metadata.Schema != adaptiveBootstrapMetadataSchema || metadata.ThreadID != req.ThreadID ||
		metadata.ExecutionRunID != req.ExecutionRunID || metadata.JournalRunID != req.JournalRunID ||
		metadata.AttemptID != req.AttemptID || metadata.ExecutionGeneration == 0 ||
		!adaptiveBootstrapLowerHex(metadata.AdmissionDigest) || !adaptiveBootstrapLowerHex(metadata.DecisionDigest) ||
		!adaptiveBootstrapLowerHex(metadata.AdmissionEventFingerprint) || !adaptiveBootstrapLowerHex(metadata.DecisionEventFingerprint) ||
		!adaptiveBootstrapLowerHex(metadata.OperationKeyDigest) || !adaptiveBootstrapLowerHex(metadata.CheckpointFingerprint) ||
		metadata.AdmissionEventID <= 0 || metadata.DecisionEventID <= 0 || metadata.AdmissionEventID == metadata.DecisionEventID ||
		!adaptiveBootstrapExactNonEmpty(metadata.AdmissionEventKey, 191) ||
		!adaptiveBootstrapExactNonEmpty(metadata.DecisionEventKey, 191) || metadata.FactCreatedAt <= 0 ||
		metadata.FactCreatedAt > math.MaxInt64/int64(1_000_000) {
		return nil, bootstrapConflictf("control checkpoint authority drift")
	}
	if metadata.AdmissionEventKey != adaptiveBootstrapEventKey("admission", metadata.OperationKeyDigest) ||
		metadata.DecisionEventKey != adaptiveBootstrapEventKey("decision", metadata.OperationKeyDigest) {
		return nil, bootstrapConflictf("control checkpoint event key drift")
	}
	if checkpoint.ID <= 0 || checkpoint.ThreadID != req.ThreadID || checkpoint.RunID != req.ExecutionRunID ||
		checkpoint.ParentCheckpointID != 0 || checkpoint.CheckpointNS != adaptiveBootstrapCheckpointNS ||
		checkpoint.RuntimeType != adaptiveBootstrapRuntimeType || checkpoint.RuntimeKey != checkpointKey ||
		checkpoint.EnvelopeVersion != 1 || checkpoint.RuntimeDeletedAt != 0 || checkpoint.CreatedAt != metadata.FactCreatedAt ||
		string(checkpoint.ChannelValues) != "{}" || string(checkpoint.ChannelVersions) != "{}" || string(checkpoint.PendingSends) != "[]" {
		return nil, bootstrapConflictf("control checkpoint identity drift")
	}
	fingerprint, err := adaptiveBootstrapCheckpointFingerprint(checkpoint, metadata)
	if err != nil || fingerprint != metadata.CheckpointFingerprint {
		return nil, bootstrapConflictf("control checkpoint fingerprint drift")
	}

	if len(reservedEvents) != 2 {
		return nil, bootstrapConflictf("bootstrap events are incomplete")
	}
	byID := make(map[int64]*runEventPO, len(reservedEvents))
	for index := range reservedEvents {
		byID[reservedEvents[index].ID] = &reservedEvents[index]
	}
	admissionEvent := byID[metadata.AdmissionEventID]
	decisionEvent := byID[metadata.DecisionEventID]
	if err := validateAdaptiveBootstrapStoredEvent(
		dialect,
		admissionEvent, req, metadata, adaptiveBootstrapAdmissionEventType, metadata.AdmissionEventKey, metadata.AdmissionDigest,
		metadata.AdmissionEventFingerprint,
	); err != nil {
		return nil, err
	}
	if err := validateAdaptiveBootstrapStoredEvent(
		dialect,
		decisionEvent, req, metadata, adaptiveBootstrapDecisionEventType, metadata.DecisionEventKey, metadata.DecisionDigest,
		metadata.DecisionEventFingerprint,
	); err != nil {
		return nil, err
	}
	admission, admissionCanonical, admissionDigest, err := adaptivecontract.DecodeAdaptiveAdmission(admissionEvent.Payload)
	if err != nil || admissionDigest != metadata.AdmissionDigest ||
		!adaptiveBootstrapStoredPayloadMatches(dialect, admissionCanonical, admissionEvent.Payload) {
		return nil, bootstrapConflictf("admission payload drift")
	}
	if err := validateAdaptiveBootstrapStoredLineage(&attempt, metadata, admission); err != nil {
		return nil, err
	}
	decision, decisionCanonical, decisionDigest, err := adaptivecontract.DecodeExecutionDecision(decisionEvent.Payload)
	if err != nil || decisionDigest != metadata.DecisionDigest ||
		!adaptiveBootstrapStoredPayloadMatches(dialect, decisionCanonical, decisionEvent.Payload) {
		return nil, bootstrapConflictf("decision payload drift")
	}
	if admission.Source == entity.AdaptiveAdmissionSourceFresh && decision.PlanScopeRunID != nil &&
		*decision.PlanScopeRunID != req.ExecutionRunID {
		return nil, bootstrapConflictf("fresh bootstrap plan scope drift")
	}
	if err := adaptivecontract.ValidateAdaptiveBootstrapPair(admission, decision, adaptivecontract.BootstrapIdentity{
		ExecutionRunID: req.ExecutionRunID, JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
		ExecutionGeneration: metadata.ExecutionGeneration,
		ExpectedPlanScopeRunID: func() int64 {
			if decision.PlanScopeRunID == nil {
				return 0
			}
			return *decision.PlanScopeRunID
		}(),
	}); err != nil {
		return nil, fmt.Errorf("%w: bootstrap pair drift", ErrAdaptiveExecutionBootstrapConflict)
	}
	if expected != nil {
		if expected.admissionDigest != metadata.AdmissionDigest || expected.decisionDigest != metadata.DecisionDigest ||
			expected.operationDigest != metadata.OperationKeyDigest || expected.admissionEventKey != metadata.AdmissionEventKey ||
			expected.decisionEventKey != metadata.DecisionEventKey || expected.checkpointKey != checkpoint.RuntimeKey ||
			string(expected.admissionCanonical) != string(admissionCanonical) ||
			string(expected.decisionCanonical) != string(decisionCanonical) {
			return nil, bootstrapConflictf("bootstrap replay payload drift")
		}
	}
	return &CommitAdaptiveExecutionBootstrapResult{
		Admission: admission, Decision: decision,
		AdmissionEvent: admissionEvent.toEntity(), DecisionEvent: decisionEvent.toEntity(), Checkpoint: checkpoint.toEntity(),
		Authority: AdaptiveExecutionBootstrapAuthority{
			ThreadID: metadata.ThreadID, ExecutionRunID: metadata.ExecutionRunID, JournalRunID: metadata.JournalRunID,
			AttemptID: metadata.AttemptID, ExecutionGeneration: metadata.ExecutionGeneration,
			AdmissionEventID: metadata.AdmissionEventID, DecisionEventID: metadata.DecisionEventID, CheckpointID: checkpoint.ID,
			AdmissionEventKey: metadata.AdmissionEventKey, DecisionEventKey: metadata.DecisionEventKey,
			AdmissionDigest: metadata.AdmissionDigest, DecisionDigest: metadata.DecisionDigest,
			AdmissionEventFingerprint: metadata.AdmissionEventFingerprint,
			DecisionEventFingerprint:  metadata.DecisionEventFingerprint,
			OperationKeyDigest:        metadata.OperationKeyDigest, CheckpointFingerprint: metadata.CheckpointFingerprint,
			FactCreatedAt: metadata.FactCreatedAt,
		},
	}, nil
}

func validateAdaptiveBootstrapEvent(
	event *runEventPO,
	req ReadAdaptiveExecutionBootstrapRequest,
	metadata adaptiveBootstrapMetadata,
	eventType, eventKey, digest, fingerprint string,
) error {
	if metadata.FactCreatedAt <= 0 || metadata.FactCreatedAt > math.MaxInt64/int64(1_000_000) {
		return bootstrapConflictf("bootstrap event timestamp overflows nanoseconds")
	}
	expectedOccurredAt := metadata.FactCreatedAt * int64(1_000_000)
	if event == nil || event.ThreadID != req.ThreadID || event.RunID != req.ExecutionRunID ||
		event.EventType != eventType || event.CreatedAt != metadata.FactCreatedAt || event.JournalRunID == nil ||
		*event.JournalRunID != req.JournalRunID || event.AttemptID == nil || *event.AttemptID != req.AttemptID ||
		event.Sequence != nil || event.IdempotencyKey == nil || *event.IdempotencyKey != eventKey ||
		event.SchemaVersion == nil || *event.SchemaVersion != entity.JournalSchemaVersion ||
		event.OccurredAtUnixNano == nil || *event.OccurredAtUnixNano != expectedOccurredAt ||
		event.Visibility == nil || *event.Visibility != string(entity.JournalVisibilityInternal) ||
		event.PayloadVersion == nil || *event.PayloadVersion != entity.JournalPayloadVersion ||
		event.SnapshotID == nil || *event.SnapshotID != metadata.CheckpointFingerprint ||
		event.ParentEventID != nil || event.Status != nil || event.TraceID != nil || event.ActionID != nil ||
		event.Phase != nil || event.Operation != nil || event.Target != nil || event.Milestone != nil ||
		event.JournalEventType != nil || len(event.JournalPayload) != 0 || !adaptiveBootstrapLowerHex(digest) ||
		!adaptiveBootstrapLowerHex(fingerprint) {
		return bootstrapConflictf("bootstrap event authority drift")
	}
	actualFingerprint, err := adaptiveBootstrapEventFingerprint(event)
	if err != nil || actualFingerprint != fingerprint {
		return bootstrapConflictf("bootstrap event fingerprint drift")
	}
	return nil
}

func validateAdaptiveBootstrapStoredEvent(
	dialect string,
	event *runEventPO,
	req ReadAdaptiveExecutionBootstrapRequest,
	metadata adaptiveBootstrapMetadata,
	eventType, eventKey, digest, fingerprint string,
) error {
	if dialect != "mysql" {
		return validateAdaptiveBootstrapEvent(event, req, metadata, eventType, eventKey, digest, fingerprint)
	}
	if event == nil {
		return validateAdaptiveBootstrapEvent(event, req, metadata, eventType, eventKey, digest, fingerprint)
	}
	canonical, canonicalDigest, err := adaptiveBootstrapCanonicalEventPayload(eventType, event.Payload)
	if err != nil || canonicalDigest != digest {
		return bootstrapConflictf("bootstrap event payload drift")
	}
	stored := *event
	stored.Payload = append([]byte(nil), canonical...)
	return validateAdaptiveBootstrapEvent(&stored, req, metadata, eventType, eventKey, digest, fingerprint)
}

func adaptiveBootstrapCanonicalEventPayload(eventType string, raw []byte) ([]byte, string, error) {
	switch eventType {
	case adaptiveBootstrapAdmissionEventType:
		_, canonical, digest, err := adaptivecontract.DecodeAdaptiveAdmission(raw)
		return canonical, digest, err
	case adaptiveBootstrapDecisionEventType:
		_, canonical, digest, err := adaptivecontract.DecodeExecutionDecision(raw)
		return canonical, digest, err
	default:
		return nil, "", errors.New("bootstrap event type is unsupported")
	}
}

func adaptiveBootstrapStoredPayloadMatches(dialect string, canonical, stored []byte) bool {
	return dialect == "mysql" || string(canonical) == string(stored)
}

func decodeAdaptiveBootstrapMetadata(raw []byte) (adaptiveBootstrapMetadata, error) {
	return decodeAdaptiveBootstrapMetadataWithCanonicality(raw, true)
}

func decodeAdaptiveBootstrapStoredMetadata(dialect string, raw []byte) (adaptiveBootstrapMetadata, error) {
	return decodeAdaptiveBootstrapMetadataWithCanonicality(raw, dialect != "mysql")
}

func decodeAdaptiveBootstrapMetadataWithCanonicality(
	raw []byte,
	requireCanonical bool,
) (adaptiveBootstrapMetadata, error) {
	if len(raw) == 0 || len(raw) > adaptiveBootstrapMetadataMaxBytes {
		return adaptiveBootstrapMetadata{}, errors.New("metadata size is invalid")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || len(root) != 1 || root["adaptive_bootstrap"] == nil {
		return adaptiveBootstrapMetadata{}, errors.New("metadata root is invalid")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(root["adaptive_bootstrap"], &fields); err != nil {
		return adaptiveBootstrapMetadata{}, err
	}
	if len(fields) != 20 {
		return adaptiveBootstrapMetadata{}, errors.New("metadata field count is invalid")
	}
	for _, field := range []string{
		"schema", "thread_id", "execution_run_id", "journal_run_id", "attempt_id", "execution_generation",
		"source_attempt_id", "source_checkpoint_id", "recovery_idempotency_key", "admission_event_id", "decision_event_id",
		"admission_event_key", "decision_event_key", "admission_digest", "decision_digest",
		"admission_event_fingerprint", "decision_event_fingerprint", "operation_key_digest",
		"checkpoint_fingerprint", "fact_created_at",
	} {
		if _, exists := fields[field]; !exists {
			return adaptiveBootstrapMetadata{}, errors.New("metadata field is missing")
		}
	}
	var envelope adaptiveBootstrapMetadataEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return adaptiveBootstrapMetadata{}, err
	}
	if requireCanonical {
		canonical, err := json.Marshal(envelope)
		if err != nil || string(canonical) != string(raw) {
			return adaptiveBootstrapMetadata{}, errors.New("metadata is not canonical")
		}
	}
	return envelope.AdaptiveBootstrap, nil
}

func adaptiveBootstrapCheckpointFingerprint(checkpoint *checkpointPO, metadata adaptiveBootstrapMetadata) (string, error) {
	if checkpoint == nil {
		return "", errors.New("checkpoint is missing")
	}
	metadata.CheckpointFingerprint = ""
	encoded, err := json.Marshal(adaptiveBootstrapCheckpointFingerprintValue{
		ID: checkpoint.ID, ThreadID: checkpoint.ThreadID, RunID: checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID, CheckpointNS: checkpoint.CheckpointNS,
		RuntimeType: checkpoint.RuntimeType, RuntimeKey: checkpoint.RuntimeKey,
		EnvelopeVersion: checkpoint.EnvelopeVersion, RuntimeDeletedAt: checkpoint.RuntimeDeletedAt,
		ChannelValues:   append(json.RawMessage(nil), checkpoint.ChannelValues...),
		ChannelVersions: append(json.RawMessage(nil), checkpoint.ChannelVersions...),
		PendingSends:    append(json.RawMessage(nil), checkpoint.PendingSends...),
		Metadata:        metadata, CreatedAt: checkpoint.CreatedAt,
	})
	if err != nil {
		return "", err
	}
	return adaptiveBootstrapDigestBytes(encoded), nil
}

func adaptiveBootstrapEventFingerprint(event *runEventPO) (string, error) {
	if event == nil {
		return "", errors.New("event is missing")
	}
	encoded, err := json.Marshal(adaptiveBootstrapEventFingerprintValue{
		ID: event.ID, ThreadID: event.ThreadID, RunID: event.RunID,
		JournalRunID:       adaptiveExecutionCloneInt64Pointer(event.JournalRunID),
		AttemptID:          adaptiveExecutionCloneStringPointer(event.AttemptID),
		Sequence:           event.Sequence,
		IdempotencyKey:     adaptiveExecutionCloneStringPointer(event.IdempotencyKey),
		ParentEventID:      adaptiveExecutionCloneInt64Pointer(event.ParentEventID),
		SchemaVersion:      adaptiveExecutionCloneStringPointer(event.SchemaVersion),
		Status:             adaptiveExecutionCloneStringPointer(event.Status),
		OccurredAtUnixNano: adaptiveExecutionCloneInt64Pointer(event.OccurredAtUnixNano),
		Visibility:         adaptiveExecutionCloneStringPointer(event.Visibility),
		PayloadVersion:     adaptiveExecutionCloneStringPointer(event.PayloadVersion),
		TraceID:            adaptiveExecutionCloneStringPointer(event.TraceID),
		ActionID:           adaptiveExecutionCloneStringPointer(event.ActionID),
		Phase:              adaptiveExecutionCloneStringPointer(event.Phase),
		Operation:          adaptiveExecutionCloneStringPointer(event.Operation),
		Target:             adaptiveExecutionCloneStringPointer(event.Target),
		Milestone:          adaptiveExecutionCloneStringPointer(event.Milestone),
		EventType:          event.EventType,
		JournalEventType:   adaptiveExecutionCloneStringPointer(event.JournalEventType),
		Payload:            append(json.RawMessage(nil), event.Payload...),
		JournalPayload:     append(json.RawMessage(nil), event.JournalPayload...),
		CreatedAt:          event.CreatedAt,
	})
	if err != nil {
		return "", err
	}
	return adaptiveBootstrapDigestBytes(encoded), nil
}

func adaptiveBootstrapCheckpointKey(journalRunID int64, attemptID string) string {
	return "adaptive-bootstrap-" + adaptiveBootstrapDigest(fmt.Sprintf(
		"%s\n%d\n%s", adaptiveBootstrapKeySchema, journalRunID, attemptID,
	))
}

func adaptiveBootstrapEventKey(kind, operationDigest string) string {
	return "adaptive-bootstrap-" + kind + "-" + operationDigest
}

func adaptiveBootstrapDigest(value string) string {
	return adaptiveBootstrapDigestBytes([]byte(value))
}

func adaptiveBootstrapDigestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func adaptiveBootstrapExactNonEmpty(value string, maximum int) bool {
	return value != "" && strings.TrimSpace(value) == value && len([]byte(value)) <= maximum
}

func adaptiveBootstrapLowerHex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func bootstrapInvalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrAdaptiveExecutionBootstrapInvalid, fmt.Sprintf(format, args...))
}

func bootstrapConflictf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrAdaptiveExecutionBootstrapConflict, fmt.Sprintf(format, args...))
}

func bootstrapWriteError(err error) error {
	if err == nil {
		return nil
	}
	if isAdaptiveExecutionDuplicateError(err) || isAdaptiveExecutionCheckpointPrimaryKeyConflict(err) {
		return fmt.Errorf("%w: durable candidate collision", ErrAdaptiveExecutionBootstrapConflict)
	}
	return err
}
