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

package agentthread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

var (
	ErrJournalRecoveryInvalid           = errors.New("journal recovery request is invalid")
	ErrJournalRecoveryConflict          = errors.New("journal recovery conflicts with an active attempt")
	ErrJournalRecoveryConfirmRequired   = errors.New("journal recovery confirmation is required")
	ErrJournalRecoveryCheckpointInvalid = errors.New("journal recovery checkpoint is invalid")
	ErrJournalRecoveryCheckpointUnsafe  = errors.New("journal recovery checkpoint is not replay safe")
	ErrJournalRecoveryDependencyMissing = errors.New("journal recovery dependency is unavailable")
)

type JournalRecoveryAction string

const (
	JournalRecoveryActionResume        JournalRecoveryAction = "resume"
	JournalRecoveryActionMarkSucceeded JournalRecoveryAction = "mark_succeeded"
	JournalRecoveryActionSkip          JournalRecoveryAction = "skip"
	JournalRecoveryActionRetry         JournalRecoveryAction = "retry"
)

func (a JournalRecoveryAction) Valid() bool {
	switch a {
	case JournalRecoveryActionResume,
		JournalRecoveryActionMarkSucceeded,
		JournalRecoveryActionSkip,
		JournalRecoveryActionRetry:
		return true
	default:
		return false
	}
}

func (a JournalRecoveryAction) resolutionAction() domainentity.SideEffectResolutionAction {
	switch a {
	case JournalRecoveryActionMarkSucceeded:
		return domainentity.SideEffectResolutionActionMarkSucceeded
	case JournalRecoveryActionSkip:
		return domainentity.SideEffectResolutionActionSkip
	case JournalRecoveryActionRetry:
		return domainentity.SideEffectResolutionActionRetry
	default:
		return ""
	}
}

type RecoverJournalRequest struct {
	ViewerID        int64
	SpaceID         int64
	ThreadID        int64
	RunID           int64
	SourceAttemptID string
	Action          JournalRecoveryAction
	Confirmed       bool
	IdempotencyKey  string
	TraceID         string
	expiredLease    *journalRecoveryExpiredLease
}

type journalRecoveryExpiredLease struct {
	RunID               int64
	LeaseOwner          string
	LeaseToken          string
	ExecutionGeneration uint64
	Now                 int64
	ErrorCode           string
	ErrorMessage        string
}

type RecoverJournalResult struct {
	Run      *RunSummary
	Attempt  *domainentity.RunAttempt
	Accepted bool
	Created  bool
}

type JournalRecoveryRepository interface {
	GetActiveJournalAttempt(context.Context, int64) (*domainentity.RunAttempt, error)
	ListJournalAttempts(context.Context, int64) ([]*domainentity.RunAttempt, error)
	ListSideEffectLedgers(context.Context, int64, string) ([]*domainentity.SideEffectLedger, error)
	TransitionSideEffect(
		context.Context,
		domainrepo.TransitionSideEffectRequest,
	) (*domainentity.SideEffectLedger, bool, error)
	ResolveUnknownSideEffect(
		context.Context,
		domainrepo.ResolveUnknownSideEffectRequest,
	) (*domainentity.SideEffectLedger, bool, error)
}

func (s *ApplicationService) RecoverJournal(
	ctx context.Context,
	req RecoverJournalRequest,
) (result *RecoverJournalResult, retErr error) {
	defer func() {
		s.recordJournalRecoveryOutcome(ctx, req, result, retErr)
	}()
	req.SourceAttemptID = strings.TrimSpace(req.SourceAttemptID)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.TraceID = strings.TrimSpace(req.TraceID)
	if req.ViewerID <= 0 || req.SpaceID <= 0 || req.ThreadID <= 0 || req.RunID <= 0 ||
		!req.Action.Valid() || req.IdempotencyKey == "" || len(req.IdempotencyKey) > 191 {
		return nil, ErrJournalRecoveryInvalid
	}
	if req.expiredLease != nil {
		req.expiredLease.LeaseOwner = strings.TrimSpace(req.expiredLease.LeaseOwner)
		req.expiredLease.LeaseToken = strings.TrimSpace(req.expiredLease.LeaseToken)
		req.expiredLease.ErrorCode = strings.TrimSpace(req.expiredLease.ErrorCode)
		req.expiredLease.ErrorMessage = strings.TrimSpace(req.expiredLease.ErrorMessage)
		if req.SourceAttemptID == "" || req.expiredLease.RunID <= 0 ||
			req.expiredLease.LeaseOwner == "" || req.expiredLease.LeaseToken == "" ||
			req.expiredLease.ExecutionGeneration == 0 || req.expiredLease.Now <= 0 {
			return nil, ErrJournalRecoveryInvalid
		}
	}
	if s == nil || s.ThreadSVC == nil || s.JournalRecoveryRepository == nil {
		return nil, ErrJournalRecoveryDependencyMissing
	}
	if s.JournalFeatureGate != nil {
		enabled, err := s.JournalFeatureGate.Enabled(ctx, JournalFeatureRecovery, req.SpaceID)
		if err != nil || !enabled {
			return nil, ErrJournalRecoveryDependencyMissing
		}
	}
	if err := s.AuthorizeThreadAccess(ctx, ThreadAccessRequest{
		ViewerID: req.ViewerID, SpaceID: req.SpaceID, ThreadID: req.ThreadID, RunID: req.RunID,
	}); err != nil {
		return nil, err
	}

	root, err := s.ThreadSVC.GetRun(ctx, &domainservice.GetRunRequest{RunID: req.RunID})
	if err != nil {
		return nil, err
	}
	if root == nil || root.ID != req.RunID || root.ThreadID != req.ThreadID ||
		root.SpaceID != req.SpaceID || root.ParentRunID != 0 ||
		(root.RunKind != "" && root.RunKind != domainentity.RunKindTask) {
		return nil, ErrThreadAccessDenied
	}

	attempts, err := s.JournalRecoveryRepository.ListJournalAttempts(ctx, req.RunID)
	if err != nil {
		return nil, err
	}
	if replay, err := s.replayedJournalRecovery(ctx, req, attempts); replay != nil || err != nil {
		return replay, err
	}
	var activeSource *domainentity.RunAttempt
	for _, attempt := range attempts {
		if attempt == nil || !attempt.Status.IsActive() {
			continue
		}
		if req.expiredLease == nil || activeSource != nil ||
			attempt.AttemptID != req.SourceAttemptID ||
			attempt.ExecutionRunID != req.expiredLease.RunID {
			return nil, ErrJournalRecoveryConflict
		}
		activeSource = attempt
	}
	source := activeSource
	if source == nil {
		source = selectJournalRecoverySourceAttempt(attempts, req.SourceAttemptID)
	}
	if source == nil || (!source.Status.IsTerminal() && source != activeSource) ||
		source.ThreadID != req.ThreadID ||
		source.JournalRunID != req.RunID {
		return nil, ErrJournalRecoveryInvalid
	}
	sourceRun := root
	if source.ExecutionRunID != root.ID {
		sourceRun, err = s.ThreadSVC.GetRun(
			ctx, &domainservice.GetRunRequest{RunID: source.ExecutionRunID},
		)
		if err != nil {
			return nil, err
		}
		if sourceRun == nil || sourceRun.ThreadID != root.ThreadID {
			return nil, ErrJournalRecoveryInvalid
		}
	}
	checkpoint, recoveryState, err := s.loadJournalRecoveryCheckpoint(ctx, source)
	if err != nil {
		return nil, err
	}
	ledgers, err := s.JournalRecoveryRepository.ListSideEffectLedgers(
		ctx, req.RunID, source.AttemptID,
	)
	if err != nil {
		return nil, err
	}
	if err := validateJournalRecoveryLedgerSnapshot(recoveryState.SideEffectLedger, ledgers); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrJournalRecoveryCheckpointUnsafe, err)
	}
	recoveryConfig, err := stripSubmittedExecutionControls(sourceRun.Config)
	if err != nil {
		return nil, err
	}
	ledgers, err = s.reconcileJournalRecoveryLedgers(ctx, req, source, ledgers)
	if err != nil {
		return nil, err
	}

	command, metadata, fingerprint, err := journalRecoveryPayloads(
		root, sourceRun, source, checkpoint, req, ledgers,
	)
	if err != nil {
		return nil, err
	}
	recoveryEnrollment := &domainservice.JournalRecoveryEnrollmentOptions{
		JournalRunID: req.RunID, SourceCheckpointID: checkpoint.CheckpointID,
		SourceAttemptID: source.AttemptID, IdempotencyKey: req.IdempotencyKey,
	}
	if req.expiredLease != nil {
		recoveryEnrollment.ExpiredLease = &domainservice.JournalRecoveryExpiredLeaseOptions{
			RunID: req.expiredLease.RunID, LeaseOwner: req.expiredLease.LeaseOwner,
			LeaseToken:          req.expiredLease.LeaseToken,
			ExecutionGeneration: req.expiredLease.ExecutionGeneration,
			Now:                 req.expiredLease.Now,
			ErrorCode:           req.expiredLease.ErrorCode,
			ErrorMessage:        req.expiredLease.ErrorMessage,
		}
	}
	bundle, err := s.ThreadSVC.CreateRunBundle(ctx, &domainservice.CreateRunBundleRequest{
		Run: domainservice.CreateRunRequest{
			ThreadID: req.ThreadID, AssistantID: sourceRun.AssistantID,
			RunKind: domainentity.RunKindTask, Status: domainentity.RunStatusQueued,
			Command: command, Input: `{"messages":[]}`,
			Config: recoveryConfig, Context: sourceRun.Context, Metadata: metadata,
			StreamMode: sourceRun.StreamMode, MultitaskStrategy: "reject",
			OnDisconnect: sourceRun.OnDisconnect, Durability: sourceRun.Durability,
			IdempotencyKey:       req.IdempotencyKey,
			IdempotencyOperation: "journal.recovery", IdempotencyFingerprint: fingerprint,
		},
		EnrollJournal: true,
		JournalEnrollment: &domainservice.JournalEnrollmentOptions{
			TraceID: req.TraceID, Recovery: recoveryEnrollment,
		},
	})
	if err != nil {
		if errors.Is(err, domainrepo.ErrActiveJournalAttemptExists) ||
			errors.Is(err, domainrepo.ErrActiveRunExists) ||
			errors.Is(err, domainrepo.ErrRunIdempotencyConflict) {
			return nil, ErrJournalRecoveryConflict
		}
		return nil, err
	}
	if bundle == nil || bundle.Run == nil || bundle.Attempt == nil {
		return nil, ErrJournalRecoveryDependencyMissing
	}
	return &RecoverJournalResult{
		Run: DomainRunToSummary(bundle.Run), Attempt: bundle.Attempt,
		Accepted: true, Created: bundle.Created,
	}, nil
}

func (s *ApplicationService) recordJournalRecoveryOutcome(
	ctx context.Context,
	req RecoverJournalRequest,
	result *RecoverJournalResult,
	recoveryErr error,
) {
	if s == nil {
		return
	}
	metricResult := "success"
	errorCode := "none"
	attemptID := strings.TrimSpace(req.SourceAttemptID)
	version := domainentity.JournalSchemaVersion
	if recoveryErr != nil {
		metricResult = "failed"
		errorCode = journalRecoveryErrorCode(recoveryErr)
	}
	if result != nil && result.Attempt != nil {
		attemptID = strings.TrimSpace(result.Attempt.AttemptID)
		if result.Attempt.EnrollmentVersion != "" {
			version = result.Attempt.EnrollmentVersion
		}
	}
	if attemptID == "" {
		attemptID = "unknown"
	}
	labels := JournalMetricLabels{
		Version: version, RolloutCohort: "treatment", TaskType: "unknown",
		ClientVersion: "unknown", Result: metricResult, ErrorCode: errorCode,
	}
	if s.JournalMetrics != nil {
		s.JournalMetrics.RecordRecovery(ctx, JournalRecoveryMetricObservation{Labels: labels})
	}
	if s.JournalTelemetry == nil || req.RunID <= 0 || !req.Action.Valid() {
		return
	}
	if err := s.JournalTelemetry.RecordRecoveryResult(ctx, JournalRecoveryTelemetryEvent{
		EventName: "journal_recovery_result",
		RunID:     req.RunID, AttemptID: attemptID, TraceID: strings.TrimSpace(req.TraceID),
		Action: string(req.Action), Result: metricResult, ErrorCode: errorCode,
		Version: version, RolloutCohort: "treatment", TaskType: "unknown",
	}); err != nil {
		logs.CtxWarnf(
			ctx,
			"[journal-telemetry] recovery result emit failed, run_id=%d attempt_id=%s error_code=%s err=%v",
			req.RunID,
			attemptID,
			errorCode,
			err,
		)
	}
}

func journalRecoveryErrorCode(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, ErrJournalRecoveryInvalid):
		return "invalid_request"
	case errors.Is(err, ErrJournalRecoveryConflict):
		return "conflict"
	case errors.Is(err, ErrJournalRecoveryConfirmRequired):
		return "confirmation_required"
	case errors.Is(err, ErrJournalRecoveryCheckpointInvalid):
		return "checkpoint_invalid"
	case errors.Is(err, ErrJournalRecoveryCheckpointUnsafe):
		return "checkpoint_unsafe"
	case errors.Is(err, ErrJournalRecoveryDependencyMissing):
		return "dependency_unavailable"
	case errors.Is(err, ErrThreadAccessDenied):
		return "no_permission"
	default:
		return "internal"
	}
}

func (s *ApplicationService) replayedJournalRecovery(
	ctx context.Context,
	req RecoverJournalRequest,
	attempts []*domainentity.RunAttempt,
) (*RecoverJournalResult, error) {
	existing, err := s.ThreadSVC.GetRunByIdempotencyKey(ctx, req.SpaceID, req.IdempotencyKey)
	if err != nil || existing == nil {
		return nil, err
	}
	if existing.ThreadID != req.ThreadID {
		return nil, ErrJournalRecoveryConflict
	}
	for _, attempt := range attempts {
		if attempt == nil || attempt.ExecutionRunID != existing.ID ||
			attempt.RecoveryIdempotencyKey == nil {
			continue
		}
		if strings.TrimSpace(*attempt.RecoveryIdempotencyKey) != req.IdempotencyKey {
			return nil, ErrJournalRecoveryConflict
		}
		return &RecoverJournalResult{
			Run: DomainRunToSummary(existing), Attempt: attempt,
			Accepted: true, Created: false,
		}, nil
	}
	return nil, ErrJournalRecoveryConflict
}

func selectJournalRecoverySourceAttempt(
	attempts []*domainentity.RunAttempt,
	requestedID string,
) *domainentity.RunAttempt {
	var selected *domainentity.RunAttempt
	for _, attempt := range attempts {
		if attempt == nil || !attempt.Status.IsTerminal() {
			continue
		}
		if requestedID != "" {
			if attempt.AttemptID == requestedID {
				return attempt
			}
			continue
		}
		if selected == nil || attempt.Ordinal > selected.Ordinal ||
			(attempt.Ordinal == selected.Ordinal && attempt.ID > selected.ID) {
			selected = attempt
		}
	}
	return selected
}

func (s *ApplicationService) loadJournalRecoveryCheckpoint(
	ctx context.Context,
	source *domainentity.RunAttempt,
) (*CheckpointSummary, *ADKRecoveryCheckpoint, error) {
	checkpoints, _, err := s.ThreadSVC.ListCheckpoints(ctx, &domainservice.ListCheckpointsRequest{
		ThreadID: source.ThreadID, RunID: source.ExecutionRunID,
		RuntimeType: string(RuntimeModeEinoADK), Limit: 100,
	})
	if err != nil {
		return nil, nil, err
	}
	var latest *domainentity.Checkpoint
	for _, checkpoint := range checkpoints {
		if checkpoint == nil || checkpoint.RuntimeDeletedAt > 0 ||
			strings.TrimSpace(checkpoint.RuntimeType) != string(RuntimeModeEinoADK) {
			continue
		}
		if latest == nil || checkpoint.CreatedAt > latest.CreatedAt ||
			(checkpoint.CreatedAt == latest.CreatedAt && checkpoint.ID > latest.ID) {
			latest = checkpoint
		}
	}
	if latest == nil {
		return nil, nil, ErrJournalRecoveryCheckpointInvalid
	}
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(latest.ChannelValues))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrJournalRecoveryCheckpointInvalid, err)
	}
	recovery, err := DecodeADKRecoveryCheckpoint(envelope)
	if err != nil {
		if errors.Is(err, ErrADKCheckpointRecoveryUnsafe) {
			return nil, nil, ErrJournalRecoveryCheckpointUnsafe
		}
		return nil, nil, fmt.Errorf("%w: %v", ErrJournalRecoveryCheckpointInvalid, err)
	}
	if recovery.AttemptID != source.AttemptID ||
		recovery.LastCommittedSequence != source.LastCommittedSequence {
		return nil, nil, ErrJournalRecoveryCheckpointUnsafe
	}
	return DomainCheckpointToSummary(latest), recovery, nil
}

func validateJournalRecoveryLedgerSnapshot(
	refs []ADKSideEffectLedgerReference,
	ledgers []*domainentity.SideEffectLedger,
) error {
	if len(refs) != len(ledgers) {
		return fmt.Errorf("checkpoint side effect ledger count changed")
	}
	byID := make(map[int64]*domainentity.SideEffectLedger, len(ledgers))
	for _, ledger := range ledgers {
		if ledger == nil {
			return fmt.Errorf("side effect ledger is missing")
		}
		byID[ledger.ID] = ledger
	}
	for _, ref := range refs {
		ledger := byID[ref.LedgerID]
		if ledger == nil || ledger.IdempotencyKey != ref.IdempotencyKey ||
			ledger.ActionKind != ref.ActionKind || string(ledger.ReplayPolicy) != ref.ReplayPolicy ||
			!journalRecoveryLedgerProgressMatches(ref, ledger) {
			return fmt.Errorf("checkpoint side effect ledger reference changed")
		}
	}
	return nil
}

func journalRecoveryLedgerProgressMatches(
	ref ADKSideEffectLedgerReference,
	ledger *domainentity.SideEffectLedger,
) bool {
	if ledger == nil {
		return false
	}
	currentAction := string(ledger.ResolutionAction)
	if string(ledger.Status) == ref.Status && ledger.Version == ref.Version &&
		currentAction == ref.ResolutionAction &&
		ledger.ResolutionIdempotencyKey == ref.ResolutionIdempotencyKey {
		return true
	}

	currentUnknown := ledger.Status == domainentity.SideEffectLedgerStatusUnknown
	refStatus := domainentity.SideEffectLedgerStatus(ref.Status)
	if !currentUnknown || (refStatus != domainentity.SideEffectLedgerStatusExecuting &&
		refStatus != domainentity.SideEffectLedgerStatusUnknown) {
		return false
	}
	if ledger.Version < ref.Version {
		return false
	}
	versionDelta := ledger.Version - ref.Version
	if refStatus == domainentity.SideEffectLedgerStatusExecuting {
		if versionDelta == 1 && currentAction == "" &&
			ledger.ResolutionIdempotencyKey == "" {
			return true
		}
		return versionDelta == 2 &&
			ledger.ResolutionAction.Valid() &&
			strings.TrimSpace(ledger.ResolutionIdempotencyKey) != ""
	}
	if ref.ResolutionAction != "" || ref.ResolutionIdempotencyKey != "" {
		return false
	}
	return versionDelta == 1 && ledger.ResolutionAction.Valid() &&
		strings.TrimSpace(ledger.ResolutionIdempotencyKey) != ""
}

func (s *ApplicationService) reconcileJournalRecoveryLedgers(
	ctx context.Context,
	req RecoverJournalRequest,
	source *domainentity.RunAttempt,
	ledgers []*domainentity.SideEffectLedger,
) ([]*domainentity.SideEffectLedger, error) {
	result := append([]*domainentity.SideEffectLedger(nil), ledgers...)
	for index, ledger := range result {
		if ledger == nil {
			return nil, ErrJournalRecoveryCheckpointUnsafe
		}
		if ledger.Status == domainentity.SideEffectLedgerStatusExecuting {
			audit, err := s.newJournalRecoveryAuditEvent(
				ctx, source, ledger, req, "unknown",
				journalRecoveryAuditKey(req.IdempotencyKey, ledger.ID, "unknown"),
			)
			if err != nil {
				return nil, err
			}
			updated, _, err := s.JournalRecoveryRepository.TransitionSideEffect(
				ctx,
				domainrepo.TransitionSideEffectRequest{
					JournalRunID: req.RunID, AttemptID: source.AttemptID, LedgerID: ledger.ID,
					ExpectedVersion: ledger.Version,
					FromStatus:      domainentity.SideEffectLedgerStatusExecuting,
					ToStatus:        domainentity.SideEffectLedgerStatusUnknown,
					OccurredAt:      time.Now().UnixMilli(), AuditEvent: audit,
				},
			)
			if err != nil {
				return nil, err
			}
			result[index] = updated
			ledger = updated
		}
		if ledger.ReplayPolicy == domainentity.SideEffectReplayPolicyNonReplayable &&
			ledger.Status != domainentity.SideEffectLedgerStatusUnknown &&
			!req.Confirmed {
			return nil, ErrJournalRecoveryConfirmRequired
		}
		if ledger.Status != domainentity.SideEffectLedgerStatusUnknown {
			continue
		}
		resolution := req.Action.resolutionAction()
		resolutionKey := journalRecoveryLedgerKey(req.IdempotencyKey, ledger.ID)
		if ledger.ResolutionAction != "" || ledger.ResolutionIdempotencyKey != "" {
			if ledger.ResolutionAction != resolution ||
				ledger.ResolutionIdempotencyKey != resolutionKey {
				return nil, ErrJournalRecoveryConflict
			}
			continue
		}
		if !req.Confirmed || !resolution.Valid() {
			return nil, ErrJournalRecoveryConfirmRequired
		}
		audit, err := s.newJournalRecoveryAuditEvent(
			ctx, source, ledger, req, "resolved",
			journalRecoveryAuditKey(req.IdempotencyKey, ledger.ID, "resolved"),
		)
		if err != nil {
			return nil, err
		}
		updated, _, err := s.JournalRecoveryRepository.ResolveUnknownSideEffect(
			ctx,
			domainrepo.ResolveUnknownSideEffectRequest{
				JournalRunID: req.RunID, AttemptID: source.AttemptID, LedgerID: ledger.ID,
				ExpectedVersion: ledger.Version, Action: resolution,
				IdempotencyKey: resolutionKey, OccurredAt: time.Now().UnixMilli(), AuditEvent: audit,
			},
		)
		if err != nil {
			return nil, err
		}
		result[index] = updated
	}
	if req.Action != JournalRecoveryActionResume {
		foundResolution := false
		for _, ledger := range result {
			if ledger != nil && ledger.Status == domainentity.SideEffectLedgerStatusUnknown {
				foundResolution = true
				break
			}
		}
		if !foundResolution {
			return nil, ErrJournalRecoveryInvalid
		}
	}
	return result, nil
}

func (s *ApplicationService) newJournalRecoveryAuditEvent(
	ctx context.Context,
	source *domainentity.RunAttempt,
	ledger *domainentity.SideEffectLedger,
	req RecoverJournalRequest,
	status string,
	idempotencyKey string,
) (*domainentity.JournalEvent, error) {
	if s.JournalRecoveryIDGenerator == nil {
		return nil, ErrJournalRecoveryDependencyMissing
	}
	id, err := s.JournalRecoveryIDGenerator.GenID(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"type": "generic",
		"data": map[string]any{
			"status": status, "ledger_id": ledger.ID, "recovery_action": req.Action,
		},
	})
	if err != nil {
		return nil, err
	}
	return &domainentity.JournalEvent{
		ID: id, ThreadID: source.ThreadID, RunID: source.ExecutionRunID,
		JournalRunID: source.JournalRunID, AttemptID: source.AttemptID,
		IdempotencyKey: idempotencyKey,
		SchemaVersion:  domainentity.JournalSchemaVersion, Status: status,
		Visibility:     domainentity.JournalVisibilityInternal,
		PayloadVersion: domainentity.JournalPayloadVersion,
		TraceID:        req.TraceID, EventType: "side_effect.audit", Payload: string(payload),
		CreatedAt: time.Now().UnixMilli(),
	}, nil
}

func journalRecoveryLedgerKey(requestKey string, ledgerID int64) string {
	candidate := fmt.Sprintf("%s:ledger:%d", strings.TrimSpace(requestKey), ledgerID)
	if len(candidate) <= 191 {
		return candidate
	}
	digest := sha256.Sum256([]byte(candidate))
	return "journal-recovery:" + hex.EncodeToString(digest[:])
}

func journalRecoveryAuditKey(requestKey string, ledgerID int64, phase string) string {
	candidate := journalRecoveryLedgerKey(requestKey, ledgerID) + ":" + strings.TrimSpace(phase)
	if len(candidate) <= 191 {
		return candidate
	}
	digest := sha256.Sum256([]byte(candidate))
	return "journal-recovery-audit:" + hex.EncodeToString(digest[:])
}

func journalRecoveryPayloads(
	root, sourceRun *domainentity.Run,
	sourceAttempt *domainentity.RunAttempt,
	checkpoint *CheckpointSummary,
	req RecoverJournalRequest,
	ledgers []*domainentity.SideEffectLedger,
) (string, string, string, error) {
	if root == nil || sourceRun == nil || sourceAttempt == nil || checkpoint == nil {
		return "", "", "", ErrJournalRecoveryInvalid
	}
	resolutions := make([]map[string]any, 0)
	for _, ledger := range ledgers {
		if ledger == nil || ledger.ResolutionAction == "" {
			continue
		}
		resolutions = append(resolutions, map[string]any{
			"ledger_id": ledger.ID, "action": ledger.ResolutionAction,
			"source_idempotency_key":     ledger.IdempotencyKey,
			"action_kind":                ledger.ActionKind,
			"request_hash":               ledger.RequestHash,
			"resolution_idempotency_key": ledger.ResolutionIdempotencyKey,
		})
	}
	resume := map[string]any{
		"checkpoint_id": checkpoint.CheckpointID,
		"checkpoint_ns": checkpoint.CheckpointNS,
		"resume_from":   "pending_sends",
		"journal": map[string]any{
			"journal_run_id": root.ID, "source_attempt_id": sourceAttempt.AttemptID,
			"action": req.Action, "ledger_resolutions": resolutions,
		},
	}
	command, err := json.Marshal(map[string]any{"resume": resume})
	if err != nil {
		return "", "", "", err
	}
	metadata, err := json.Marshal(map[string]any{
		"checkpoint_resume": map[string]any{
			"protected_from_worker_claim": true,
			"checkpoint_id":               checkpoint.CheckpointID, "checkpoint_ns": checkpoint.CheckpointNS,
			"resume_from": "pending_sends", "source_run_id": sourceRun.ID,
		},
		"journal_recovery": map[string]any{
			"schema": "coze.journal.recovery.v1", "journal_run_id": root.ID,
			"source_run_id": sourceRun.ID, "source_attempt_id": sourceAttempt.AttemptID,
			"action": req.Action,
		},
	})
	if err != nil {
		return "", "", "", err
	}
	fingerprintInput := fmt.Sprintf(
		"%d:%d:%s:%d:%s", root.ID, sourceRun.ID, sourceAttempt.AttemptID,
		checkpoint.CheckpointID, req.Action,
	)
	digest := sha256.Sum256([]byte(fingerprintInput))
	return string(command), string(metadata), hex.EncodeToString(digest[:]), nil
}
