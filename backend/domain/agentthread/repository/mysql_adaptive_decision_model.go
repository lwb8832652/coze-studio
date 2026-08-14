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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	adaptiveDecisionModelClaimEventType  = "adaptive.decision_model.claim"
	adaptiveDecisionModelResultEventType = "adaptive.decision_model.result"
	adaptiveDecisionModelSchema          = "workbench-adaptive-decision-model.v1"
	adaptiveDecisionModelMaxPayloadBytes = 64 * 1024
)

type adaptiveDecisionModelIdentity struct {
	ThreadID           int64
	ExecutionRunID     int64
	JournalRunID       int64
	AttemptID          string
	Generation         uint64
	OperationKeyDigest string
	RequestFingerprint string
	claimEventKey      string
	resultEventKey     string
}

type adaptiveDecisionModelClaim struct {
	Schema             string `json:"schema"`
	ThreadID           int64  `json:"thread_id"`
	ExecutionRunID     int64  `json:"execution_run_id"`
	JournalRunID       int64  `json:"journal_run_id"`
	AttemptID          string `json:"attempt_id"`
	Generation         uint64 `json:"execution_generation"`
	OperationKeyDigest string `json:"operation_key_digest"`
	RequestFingerprint string `json:"request_fingerprint"`
	ClaimEventID       int64  `json:"claim_event_id"`
	ResultEventID      int64  `json:"result_event_id"`
	ClaimTokenDigest   string `json:"claim_token_digest"`
	ClaimedAt          int64  `json:"claimed_at"`
}

type adaptiveDecisionModelClaimEnvelope struct {
	Claim adaptiveDecisionModelClaim `json:"adaptive_decision_model_claim"`
}

type adaptiveDecisionModelResult struct {
	Schema             string                               `json:"schema"`
	ThreadID           int64                                `json:"thread_id"`
	ExecutionRunID     int64                                `json:"execution_run_id"`
	JournalRunID       int64                                `json:"journal_run_id"`
	AttemptID          string                               `json:"attempt_id"`
	Generation         uint64                               `json:"execution_generation"`
	OperationKeyDigest string                               `json:"operation_key_digest"`
	RequestFingerprint string                               `json:"request_fingerprint"`
	ClaimEventID       int64                                `json:"claim_event_id"`
	ResultEventID      int64                                `json:"result_event_id"`
	ClaimTokenDigest   string                               `json:"claim_token_digest"`
	Status             AdaptiveDecisionModelOperationStatus `json:"status"`
	ResultPayload      json.RawMessage                      `json:"result_payload"`
	ResultDigest       string                               `json:"result_digest"`
	ErrorCode          string                               `json:"error_code"`
	CompletedAt        int64                                `json:"completed_at"`
}

type adaptiveDecisionModelResultEnvelope struct {
	Result adaptiveDecisionModelResult `json:"adaptive_decision_model_result"`
}

type adaptiveDecisionModelCompletion struct {
	status       AdaptiveDecisionModelOperationStatus
	payload      json.RawMessage
	resultDigest string
	errorCode    string
}

func NewAdaptiveDecisionModelOperationRepository(db *gorm.DB) AdaptiveDecisionModelOperationRepository {
	return &threadRepository{db: db}
}

func (r *threadRepository) ReadAdaptiveDecisionModelOperation(
	ctx context.Context,
	req ReadAdaptiveDecisionModelOperationRequest,
) (*AdaptiveDecisionModelOperation, error) {
	identity, err := normalizeAdaptiveDecisionModelIdentity(
		req.ThreadID,
		req.ExecutionRunID,
		req.JournalRunID,
		req.AttemptID,
		req.Generation,
		req.OperationKey,
		req.RequestFingerprint,
	)
	if err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, adaptiveDecisionModelInvalidf("repository database is missing")
	}
	claim, result, err := findAdaptiveDecisionModelEvents(r.db.WithContext(ctx), identity, false)
	if err != nil {
		return nil, err
	}
	return loadAdaptiveDecisionModelOperation(r.db.Dialector.Name(), identity, claim, result)
}

func (r *threadRepository) PrepareAdaptiveDecisionModelOperation(
	ctx context.Context,
	req PrepareAdaptiveDecisionModelOperationRequest,
) (*PrepareAdaptiveDecisionModelOperationResult, error) {
	identity, err := normalizeAdaptiveDecisionModelIdentity(
		req.ThreadID,
		req.ExecutionRunID,
		req.JournalRunID,
		req.AttemptID,
		req.Generation,
		req.OperationKey,
		req.RequestFingerprint,
	)
	if err != nil {
		return nil, err
	}
	if !adaptiveBootstrapExactNonEmpty(req.LeaseOwner, 191) ||
		!adaptiveBootstrapExactNonEmpty(req.LeaseToken, 191) ||
		!adaptiveBootstrapExactNonEmpty(req.ClaimToken, 191) ||
		req.ClaimEventID <= 0 || req.ResultEventID <= 0 || req.ClaimEventID == req.ResultEventID ||
		!validAdaptiveDecisionModelTime(req.Now) {
		return nil, adaptiveDecisionModelInvalidf("prepare authority is invalid")
	}
	if r == nil || r.db == nil {
		return nil, adaptiveDecisionModelInvalidf("repository database is missing")
	}

	var prepared *PrepareAdaptiveDecisionModelOperationResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt, err := lockAdaptiveDecisionModelAuthority(
			tx,
			identity,
			req.LeaseOwner,
			req.LeaseToken,
			req.Now,
		)
		if err != nil {
			return err
		}
		if err := validateAdaptiveDecisionModelFreshAttempt(attempt); err != nil {
			return err
		}
		claimEvent, resultEvent, err := findAdaptiveDecisionModelEvents(tx, identity, true)
		if err != nil {
			return err
		}
		if claimEvent != nil || resultEvent != nil {
			operation, err := loadAdaptiveDecisionModelOperation(
				tx.Dialector.Name(),
				identity,
				claimEvent,
				resultEvent,
			)
			if err != nil {
				return err
			}
			prepared = &PrepareAdaptiveDecisionModelOperationResult{Operation: operation}
			return nil
		}

		claim := adaptiveDecisionModelClaim{
			Schema:   adaptiveDecisionModelSchema,
			ThreadID: identity.ThreadID, ExecutionRunID: identity.ExecutionRunID,
			JournalRunID: identity.JournalRunID, AttemptID: identity.AttemptID,
			Generation: identity.Generation, OperationKeyDigest: identity.OperationKeyDigest,
			RequestFingerprint: identity.RequestFingerprint,
			ClaimEventID:       req.ClaimEventID, ResultEventID: req.ResultEventID,
			ClaimTokenDigest: adaptiveBootstrapDigest(req.ClaimToken), ClaimedAt: req.Now,
		}
		payload, err := json.Marshal(adaptiveDecisionModelClaimEnvelope{Claim: claim})
		if err != nil {
			return adaptiveDecisionModelInvalidf("claim payload is invalid")
		}
		row := newAdaptiveDecisionModelEvent(
			identity,
			req.ClaimEventID,
			adaptiveDecisionModelClaimEventType,
			identity.claimEventKey,
			AdaptiveDecisionModelOperationStatusCalling,
			payload,
			req.Now,
		)
		if err := tx.Create(row).Error; err != nil {
			return adaptiveDecisionModelWriteError(err)
		}
		operation, err := loadAdaptiveDecisionModelOperation(
			tx.Dialector.Name(),
			identity,
			row,
			nil,
		)
		if err != nil {
			return err
		}
		prepared = &PrepareAdaptiveDecisionModelOperationResult{Operation: operation, Owned: true}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

func (r *threadRepository) CompleteAdaptiveDecisionModelOperation(
	ctx context.Context,
	req CompleteAdaptiveDecisionModelOperationRequest,
) (*CompleteAdaptiveDecisionModelOperationResult, error) {
	identity, err := normalizeAdaptiveDecisionModelIdentity(
		req.ThreadID,
		req.ExecutionRunID,
		req.JournalRunID,
		req.AttemptID,
		req.Generation,
		req.OperationKey,
		req.RequestFingerprint,
	)
	if err != nil {
		return nil, err
	}
	if !adaptiveBootstrapExactNonEmpty(req.LeaseOwner, 191) ||
		!adaptiveBootstrapExactNonEmpty(req.LeaseToken, 191) ||
		!adaptiveBootstrapExactNonEmpty(req.ClaimToken, 191) ||
		!validAdaptiveDecisionModelTime(req.Now) {
		return nil, adaptiveDecisionModelInvalidf("completion authority is invalid")
	}
	completion, err := normalizeAdaptiveDecisionModelCompletion(req)
	if err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return nil, adaptiveDecisionModelInvalidf("repository database is missing")
	}

	var completed *CompleteAdaptiveDecisionModelOperationResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		attempt, err := lockAdaptiveDecisionModelAuthority(
			tx,
			identity,
			req.LeaseOwner,
			req.LeaseToken,
			req.Now,
		)
		if err != nil {
			return err
		}
		if err := validateAdaptiveDecisionModelFreshAttempt(attempt); err != nil {
			return err
		}
		claimEvent, resultEvent, err := findAdaptiveDecisionModelEvents(tx, identity, true)
		if err != nil {
			return err
		}
		operation, err := loadAdaptiveDecisionModelOperation(
			tx.Dialector.Name(),
			identity,
			claimEvent,
			resultEvent,
		)
		if err != nil {
			return err
		}
		if operation.ClaimTokenDigest != adaptiveBootstrapDigest(req.ClaimToken) {
			return adaptiveDecisionModelConflictf("claim ownership drift")
		}
		if operation.Status != AdaptiveDecisionModelOperationStatusCalling {
			if !adaptiveDecisionModelCompletionMatches(operation, completion) {
				return adaptiveDecisionModelConflictf("result replay drift")
			}
			completed = &CompleteAdaptiveDecisionModelOperationResult{Operation: operation, Replayed: true}
			return nil
		}
		if req.Now < operation.ClaimedAt {
			return adaptiveDecisionModelInvalidf("completion precedes claim")
		}

		result := adaptiveDecisionModelResult{
			Schema:   adaptiveDecisionModelSchema,
			ThreadID: identity.ThreadID, ExecutionRunID: identity.ExecutionRunID,
			JournalRunID: identity.JournalRunID, AttemptID: identity.AttemptID,
			Generation: identity.Generation, OperationKeyDigest: identity.OperationKeyDigest,
			RequestFingerprint: identity.RequestFingerprint,
			ClaimEventID:       operation.ClaimEventID, ResultEventID: operation.ResultEventID,
			ClaimTokenDigest: operation.ClaimTokenDigest,
			Status:           completion.status, ResultPayload: completion.payload,
			ResultDigest: completion.resultDigest, ErrorCode: completion.errorCode,
			CompletedAt: req.Now,
		}
		payload, err := json.Marshal(adaptiveDecisionModelResultEnvelope{Result: result})
		if err != nil {
			return adaptiveDecisionModelInvalidf("result payload is invalid")
		}
		row := newAdaptiveDecisionModelEvent(
			identity,
			operation.ResultEventID,
			adaptiveDecisionModelResultEventType,
			identity.resultEventKey,
			completion.status,
			payload,
			req.Now,
		)
		if err := tx.Create(row).Error; err != nil {
			return adaptiveDecisionModelWriteError(err)
		}
		loaded, err := loadAdaptiveDecisionModelOperation(
			tx.Dialector.Name(),
			identity,
			claimEvent,
			row,
		)
		if err != nil {
			return err
		}
		completed = &CompleteAdaptiveDecisionModelOperationResult{Operation: loaded}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return completed, nil
}

func normalizeAdaptiveDecisionModelIdentity(
	threadID, executionRunID, journalRunID int64,
	attemptID string,
	generation uint64,
	operationKey, requestFingerprint string,
) (adaptiveDecisionModelIdentity, error) {
	if threadID <= 0 || executionRunID <= 0 || journalRunID <= 0 || generation == 0 ||
		!adaptiveBootstrapExactNonEmpty(attemptID, 64) ||
		!adaptiveBootstrapExactNonEmpty(operationKey, 191) ||
		!validAdaptiveExecutionFingerprint(requestFingerprint) {
		return adaptiveDecisionModelIdentity{}, adaptiveDecisionModelInvalidf("operation identity is invalid")
	}
	operationDigest := adaptiveBootstrapDigest(operationKey)
	return adaptiveDecisionModelIdentity{
		ThreadID: threadID, ExecutionRunID: executionRunID, JournalRunID: journalRunID,
		AttemptID: attemptID, Generation: generation, OperationKeyDigest: operationDigest,
		RequestFingerprint: requestFingerprint,
		claimEventKey:      "adaptive-decision-model-claim-" + operationDigest,
		resultEventKey:     "adaptive-decision-model-result-" + operationDigest,
	}, nil
}

func normalizeAdaptiveDecisionModelCompletion(
	req CompleteAdaptiveDecisionModelOperationRequest,
) (adaptiveDecisionModelCompletion, error) {
	switch req.Status {
	case AdaptiveDecisionModelOperationStatusCompleted:
		if req.ErrorCode != "" || len(req.ResultPayload) == 0 || len(req.ResultPayload) > adaptiveDecisionModelMaxPayloadBytes {
			return adaptiveDecisionModelCompletion{}, adaptiveDecisionModelInvalidf("completed result shape is invalid")
		}
		canonical, err := canonicalAdaptiveExecutionJSON(string(req.ResultPayload))
		if err != nil || !bytes.Equal(canonical, req.ResultPayload) || string(canonical) == "null" {
			return adaptiveDecisionModelCompletion{}, adaptiveDecisionModelInvalidf("result payload is not canonical JSON")
		}
		return adaptiveDecisionModelCompletion{
			status: req.Status, payload: append(json.RawMessage(nil), canonical...),
			resultDigest: adaptiveBootstrapDigestBytes(canonical),
		}, nil
	case AdaptiveDecisionModelOperationStatusFailed:
		if len(req.ResultPayload) != 0 || !validAdaptiveDecisionModelErrorCode(req.ErrorCode) {
			return adaptiveDecisionModelCompletion{}, adaptiveDecisionModelInvalidf("failed result shape is invalid")
		}
		return adaptiveDecisionModelCompletion{status: req.Status, errorCode: req.ErrorCode}, nil
	default:
		return adaptiveDecisionModelCompletion{}, adaptiveDecisionModelInvalidf("result status is invalid")
	}
}

func lockAdaptiveDecisionModelAuthority(
	tx *gorm.DB,
	identity adaptiveDecisionModelIdentity,
	leaseOwner, leaseToken string,
	now int64,
) (*runAttemptPO, error) {
	if tx == nil {
		return nil, adaptiveDecisionModelInvalidf("operation transaction is missing")
	}
	boundary := CommitAdaptiveExecutionBoundaryRequest{
		ThreadID: identity.ThreadID, ExecutionRunID: identity.ExecutionRunID,
		JournalRunID: identity.JournalRunID, AttemptID: identity.AttemptID,
		Generation: identity.Generation, LeaseOwner: leaseOwner, LeaseToken: leaseToken, Now: now,
	}
	if _, err := lockThreadForUpdate(tx, identity.ThreadID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: thread identity drift", ErrAdaptiveExecutionLineageConflict)
		}
		return nil, err
	}
	journalRun, err := lockAdaptiveExecutionJournalRunIdentity(tx, boundary)
	if err != nil {
		return nil, err
	}
	run := journalRun
	if identity.ExecutionRunID != identity.JournalRunID {
		run, err = lockAdaptiveExecutionRunIdentity(tx, identity.ExecutionRunID)
		if err != nil {
			return nil, err
		}
	}
	attempt, err := lockAdaptiveExecutionAttemptIdentity(tx, identity.JournalRunID, identity.AttemptID)
	if err != nil {
		return nil, err
	}
	if err := validateAdaptiveExecutionRunFence(run, boundary); err != nil {
		return nil, err
	}
	if err := validateAdaptiveExecutionAttemptFence(attempt, boundary); err != nil {
		return nil, err
	}
	return attempt, nil
}

func validateAdaptiveDecisionModelFreshAttempt(attempt *runAttemptPO) error {
	if attempt == nil || attempt.SourceAttemptID != nil || attempt.SourceCheckpointID != nil ||
		attempt.RecoveryIdempotencyKey != nil {
		return adaptiveDecisionModelConflictf("operation requires a fresh attempt")
	}
	return nil
}

func findAdaptiveDecisionModelEvents(
	db *gorm.DB,
	identity adaptiveDecisionModelIdentity,
	lock bool,
) (*runEventPO, *runEventPO, error) {
	if db == nil {
		return nil, nil, adaptiveDecisionModelInvalidf("operation read database is missing")
	}
	query := db.Where(
		"journal_run_id = ? AND attempt_id = ? AND (event_type IN ? OR idempotency_key IN ?)",
		identity.JournalRunID,
		identity.AttemptID,
		[]string{adaptiveDecisionModelClaimEventType, adaptiveDecisionModelResultEventType},
		[]string{identity.claimEventKey, identity.resultEventKey},
	)
	if lock && db.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []runEventPO
	if err := query.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	var claim, result *runEventPO
	for index := range rows {
		row := &rows[index]
		if row.IdempotencyKey == nil {
			return nil, nil, adaptiveDecisionModelConflictf("operation event key is missing")
		}
		switch *row.IdempotencyKey {
		case identity.claimEventKey:
			if claim != nil {
				return nil, nil, adaptiveDecisionModelConflictf("claim event is ambiguous")
			}
			claim = row
		case identity.resultEventKey:
			if result != nil {
				return nil, nil, adaptiveDecisionModelConflictf("result event is ambiguous")
			}
			result = row
		default:
			return nil, nil, adaptiveDecisionModelConflictf("another operation already owns the attempt")
		}
	}
	return claim, result, nil
}

func loadAdaptiveDecisionModelOperation(
	dialect string,
	identity adaptiveDecisionModelIdentity,
	claimEvent, resultEvent *runEventPO,
) (*AdaptiveDecisionModelOperation, error) {
	if claimEvent == nil && resultEvent == nil {
		return nil, ErrAdaptiveDecisionModelOperationNotFound
	}
	if claimEvent == nil {
		return nil, adaptiveDecisionModelConflictf("operation result has no claim")
	}
	claim, err := decodeAdaptiveDecisionModelClaim(dialect, claimEvent, identity)
	if err != nil {
		return nil, err
	}
	operation := &AdaptiveDecisionModelOperation{
		Status:   AdaptiveDecisionModelOperationStatusCalling,
		ThreadID: claim.ThreadID, ExecutionRunID: claim.ExecutionRunID,
		JournalRunID: claim.JournalRunID, AttemptID: claim.AttemptID,
		Generation: claim.Generation, OperationKeyDigest: claim.OperationKeyDigest,
		RequestFingerprint: claim.RequestFingerprint,
		ClaimEventID:       claim.ClaimEventID, ResultEventID: claim.ResultEventID,
		ClaimTokenDigest: claim.ClaimTokenDigest, ClaimedAt: claim.ClaimedAt,
	}
	if resultEvent == nil {
		return operation, nil
	}
	result, resultPayload, err := decodeAdaptiveDecisionModelResult(
		dialect,
		resultEvent,
		identity,
		claim,
	)
	if err != nil {
		return nil, err
	}
	operation.Status = result.Status
	operation.ResultPayload = append(json.RawMessage(nil), resultPayload...)
	operation.ResultDigest = result.ResultDigest
	operation.ErrorCode = result.ErrorCode
	operation.CompletedAt = result.CompletedAt
	return operation, nil
}

func decodeAdaptiveDecisionModelClaim(
	dialect string,
	event *runEventPO,
	identity adaptiveDecisionModelIdentity,
) (adaptiveDecisionModelClaim, error) {
	var envelope adaptiveDecisionModelClaimEnvelope
	canonical, err := decodeAdaptiveDecisionModelEnvelope(dialect, event.Payload, &envelope)
	if err != nil {
		return adaptiveDecisionModelClaim{}, adaptiveDecisionModelConflictf("claim payload is invalid")
	}
	claim := envelope.Claim
	if claim.Schema != adaptiveDecisionModelSchema || claim.ThreadID != identity.ThreadID ||
		claim.ExecutionRunID != identity.ExecutionRunID || claim.JournalRunID != identity.JournalRunID ||
		claim.AttemptID != identity.AttemptID || claim.Generation != identity.Generation ||
		claim.OperationKeyDigest != identity.OperationKeyDigest ||
		claim.RequestFingerprint != identity.RequestFingerprint ||
		claim.ClaimEventID <= 0 || claim.ResultEventID <= 0 || claim.ClaimEventID == claim.ResultEventID ||
		!adaptiveBootstrapLowerHex(claim.ClaimTokenDigest) || !validAdaptiveDecisionModelTime(claim.ClaimedAt) {
		return adaptiveDecisionModelClaim{}, adaptiveDecisionModelConflictf("claim authority drift")
	}
	if err := validateAdaptiveDecisionModelEvent(
		event,
		identity,
		claim.ClaimEventID,
		adaptiveDecisionModelClaimEventType,
		identity.claimEventKey,
		AdaptiveDecisionModelOperationStatusCalling,
		claim.ClaimedAt,
		canonical,
	); err != nil {
		return adaptiveDecisionModelClaim{}, err
	}
	return claim, nil
}

func decodeAdaptiveDecisionModelResult(
	dialect string,
	event *runEventPO,
	identity adaptiveDecisionModelIdentity,
	claim adaptiveDecisionModelClaim,
) (adaptiveDecisionModelResult, json.RawMessage, error) {
	var envelope adaptiveDecisionModelResultEnvelope
	canonical, err := decodeAdaptiveDecisionModelEnvelope(dialect, event.Payload, &envelope)
	if err != nil {
		return adaptiveDecisionModelResult{}, nil, adaptiveDecisionModelConflictf("result payload is invalid")
	}
	result := envelope.Result
	if result.Schema != adaptiveDecisionModelSchema || result.ThreadID != identity.ThreadID ||
		result.ExecutionRunID != identity.ExecutionRunID || result.JournalRunID != identity.JournalRunID ||
		result.AttemptID != identity.AttemptID || result.Generation != identity.Generation ||
		result.OperationKeyDigest != identity.OperationKeyDigest ||
		result.RequestFingerprint != identity.RequestFingerprint ||
		result.ClaimEventID != claim.ClaimEventID || result.ResultEventID != claim.ResultEventID ||
		result.ClaimTokenDigest != claim.ClaimTokenDigest || result.CompletedAt < claim.ClaimedAt ||
		!validAdaptiveDecisionModelTime(result.CompletedAt) {
		return adaptiveDecisionModelResult{}, nil, adaptiveDecisionModelConflictf("result authority drift")
	}
	var resultPayload json.RawMessage
	switch result.Status {
	case AdaptiveDecisionModelOperationStatusCompleted:
		if result.ErrorCode != "" || len(result.ResultPayload) == 0 ||
			string(result.ResultPayload) == "null" || len(result.ResultPayload) > adaptiveDecisionModelMaxPayloadBytes {
			return adaptiveDecisionModelResult{}, nil, adaptiveDecisionModelConflictf("completed result shape drift")
		}
		payload, err := canonicalAdaptiveExecutionJSON(string(result.ResultPayload))
		if err != nil || adaptiveBootstrapDigestBytes(payload) != result.ResultDigest {
			return adaptiveDecisionModelResult{}, nil, adaptiveDecisionModelConflictf("completed result digest drift")
		}
		resultPayload = append(json.RawMessage(nil), payload...)
	case AdaptiveDecisionModelOperationStatusFailed:
		if string(result.ResultPayload) != "null" || result.ResultDigest != "" ||
			!validAdaptiveDecisionModelErrorCode(result.ErrorCode) {
			return adaptiveDecisionModelResult{}, nil, adaptiveDecisionModelConflictf("failed result shape drift")
		}
	default:
		return adaptiveDecisionModelResult{}, nil, adaptiveDecisionModelConflictf("result status drift")
	}
	if err := validateAdaptiveDecisionModelEvent(
		event,
		identity,
		claim.ResultEventID,
		adaptiveDecisionModelResultEventType,
		identity.resultEventKey,
		result.Status,
		result.CompletedAt,
		canonical,
	); err != nil {
		return adaptiveDecisionModelResult{}, nil, err
	}
	return result, resultPayload, nil
}

func decodeAdaptiveDecisionModelEnvelope(dialect string, raw []byte, target any) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("payload is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("payload has trailing content")
	}
	canonical, err := json.Marshal(target)
	if err != nil {
		return nil, err
	}
	if dialect != "mysql" && !bytes.Equal(canonical, raw) {
		return nil, errors.New("payload is not canonical")
	}
	return canonical, nil
}

func validateAdaptiveDecisionModelEvent(
	event *runEventPO,
	identity adaptiveDecisionModelIdentity,
	eventID int64,
	eventType, eventKey string,
	status AdaptiveDecisionModelOperationStatus,
	createdAt int64,
	canonicalPayload []byte,
) error {
	expectedOccurredAt := createdAt * int64(1_000_000)
	expectedStatus := string(status)
	expectedVisibility := string(entity.JournalVisibilityInternal)
	if event == nil || event.ID != eventID || event.ThreadID != identity.ThreadID ||
		event.RunID != identity.ExecutionRunID || event.EventType != eventType ||
		event.CreatedAt != createdAt || event.JournalRunID == nil || *event.JournalRunID != identity.JournalRunID ||
		event.AttemptID == nil || *event.AttemptID != identity.AttemptID || event.Sequence != nil ||
		event.IdempotencyKey == nil || *event.IdempotencyKey != eventKey || event.ParentEventID != nil ||
		event.SchemaVersion == nil || *event.SchemaVersion != entity.JournalSchemaVersion ||
		event.Status == nil || *event.Status != expectedStatus || event.OccurredAtUnixNano == nil ||
		*event.OccurredAtUnixNano != expectedOccurredAt || event.Visibility == nil ||
		*event.Visibility != expectedVisibility || event.PayloadVersion == nil ||
		*event.PayloadVersion != entity.JournalPayloadVersion || event.SnapshotID == nil ||
		*event.SnapshotID != adaptiveBootstrapDigestBytes(canonicalPayload) || event.TraceID != nil ||
		event.ActionID != nil || event.Phase != nil || event.Operation != nil || event.Target != nil ||
		event.Milestone != nil || event.JournalEventType != nil || len(event.JournalPayload) != 0 {
		return adaptiveDecisionModelConflictf("operation event authority drift")
	}
	return nil
}

func newAdaptiveDecisionModelEvent(
	identity adaptiveDecisionModelIdentity,
	eventID int64,
	eventType, eventKey string,
	status AdaptiveDecisionModelOperationStatus,
	payload []byte,
	createdAt int64,
) *runEventPO {
	journalRunID := identity.JournalRunID
	attemptID := identity.AttemptID
	schemaVersion := entity.JournalSchemaVersion
	statusValue := string(status)
	occurredAt := createdAt * int64(1_000_000)
	visibility := string(entity.JournalVisibilityInternal)
	payloadVersion := entity.JournalPayloadVersion
	snapshotID := adaptiveBootstrapDigestBytes(payload)
	return &runEventPO{
		ID: eventID, ThreadID: identity.ThreadID, RunID: identity.ExecutionRunID,
		JournalRunID: &journalRunID, AttemptID: &attemptID,
		IdempotencyKey: &eventKey, SchemaVersion: &schemaVersion, Status: &statusValue,
		OccurredAtUnixNano: &occurredAt, Visibility: &visibility,
		PayloadVersion: &payloadVersion, SnapshotID: &snapshotID,
		EventType: eventType, Payload: append([]byte(nil), payload...), CreatedAt: createdAt,
	}
}

func adaptiveDecisionModelCompletionMatches(
	operation *AdaptiveDecisionModelOperation,
	completion adaptiveDecisionModelCompletion,
) bool {
	return operation != nil && operation.Status == completion.status &&
		bytes.Equal(operation.ResultPayload, completion.payload) &&
		operation.ResultDigest == completion.resultDigest && operation.ErrorCode == completion.errorCode
}

func validAdaptiveDecisionModelTime(value int64) bool {
	return value > 0 && value <= math.MaxInt64/int64(1_000_000)
}

func validAdaptiveDecisionModelErrorCode(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') &&
			character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return strings.TrimSpace(value) == value
}

func adaptiveDecisionModelInvalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrAdaptiveDecisionModelOperationInvalid, fmt.Sprintf(format, args...))
}

func adaptiveDecisionModelConflictf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrAdaptiveDecisionModelOperationConflict, fmt.Sprintf(format, args...))
}

func adaptiveDecisionModelWriteError(err error) error {
	if err == nil {
		return nil
	}
	if isAdaptiveExecutionDuplicateError(err) {
		return adaptiveDecisionModelConflictf("durable event collision")
	}
	return err
}
