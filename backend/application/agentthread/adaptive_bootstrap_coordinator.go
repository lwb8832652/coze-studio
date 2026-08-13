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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/adaptivecontract"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const adaptiveBootstrapIdentitySchema = "workbench-adaptive-bootstrap.v1"

type AdaptiveBootstrapCoordinator interface {
	Bootstrap(context.Context, *RunSummary) (*AdaptiveBootstrapFacts, error)
	BootstrapResume(context.Context, *RunSummary, *HarnessResumeInput) (*AdaptiveBootstrapFacts, error)
}

type AdaptiveBootstrapCoordinatorFunc func(context.Context, *RunSummary) (*AdaptiveBootstrapFacts, error)

func (f AdaptiveBootstrapCoordinatorFunc) Bootstrap(
	ctx context.Context,
	run *RunSummary,
) (*AdaptiveBootstrapFacts, error) {
	if f == nil {
		return nil, fmt.Errorf("adaptive bootstrap coordinator function is required")
	}
	return f(ctx, run)
}

func (f AdaptiveBootstrapCoordinatorFunc) BootstrapResume(
	context.Context,
	*RunSummary,
	*HarnessResumeInput,
) (*AdaptiveBootstrapFacts, error) {
	return nil, fmt.Errorf("adaptive resume bootstrap coordinator function is required")
}

// AdaptiveBootstrapFacts are server-owned facts loaded from the durable C2
// bootstrap boundary. They are carried only through the current Execute
// context and are never projected into request config or metadata.
type AdaptiveBootstrapFacts struct {
	Admission domainentity.AdaptiveAdmissionSnapshot
	Decision  domainentity.ExecutionDecision
}

type adaptiveBootstrapFactsContextKey struct{}

func withAdaptiveBootstrapFacts(ctx context.Context, facts *AdaptiveBootstrapFacts) context.Context {
	if facts == nil {
		return ctx
	}
	return context.WithValue(ctx, adaptiveBootstrapFactsContextKey{}, cloneAdaptiveBootstrapFacts(facts))
}

func adaptiveBootstrapFactsFromContext(ctx context.Context) (*AdaptiveBootstrapFacts, bool) {
	if ctx == nil {
		return nil, false
	}
	facts, ok := ctx.Value(adaptiveBootstrapFactsContextKey{}).(*AdaptiveBootstrapFacts)
	if !ok || facts == nil {
		return nil, false
	}
	return cloneAdaptiveBootstrapFacts(facts), true
}

func withoutAdaptiveBootstrapFacts(ctx context.Context) context.Context {
	return context.WithValue(ctx, adaptiveBootstrapFactsContextKey{}, (*AdaptiveBootstrapFacts)(nil))
}

type AdaptiveBootstrapAttemptReader interface {
	GetActiveJournalAttempt(context.Context, int64) (*domainentity.RunAttempt, error)
}

type AdaptiveBootstrapRepository interface {
	ReadAdaptiveExecutionBootstrap(
		context.Context,
		domainrepo.ReadAdaptiveExecutionBootstrapRequest,
	) (*domainrepo.CommitAdaptiveExecutionBootstrapResult, error)
	CommitAdaptiveExecutionBootstrap(
		context.Context,
		domainrepo.CommitAdaptiveExecutionBootstrapRequest,
	) (*domainrepo.CommitAdaptiveExecutionBootstrapResult, error)
}

type AdaptiveBootstrapIDGenerator interface {
	GenMultiIDs(context.Context, int) ([]int64, error)
}

type AdaptiveBootstrapSourceRunReader interface {
	GetRun(context.Context, *domainservice.GetRunRequest) (*domainentity.Run, error)
}

type AdaptiveBootstrapCoordinatorOptions struct {
	AttemptReader   AdaptiveBootstrapAttemptReader
	Repository      AdaptiveBootstrapRepository
	SourceRunReader AdaptiveBootstrapSourceRunReader
	IDGen           AdaptiveBootstrapIDGenerator
	Now             func() int64
}

type adaptiveBootstrapCoordinator struct {
	attemptReader   AdaptiveBootstrapAttemptReader
	repository      AdaptiveBootstrapRepository
	sourceRunReader AdaptiveBootstrapSourceRunReader
	idGen           AdaptiveBootstrapIDGenerator
	now             func() int64
}

func NewAdaptiveBootstrapCoordinator(options AdaptiveBootstrapCoordinatorOptions) AdaptiveBootstrapCoordinator {
	return &adaptiveBootstrapCoordinator{
		attemptReader:   options.AttemptReader,
		repository:      options.Repository,
		sourceRunReader: options.SourceRunReader,
		idGen:           options.IDGen,
		now:             options.Now,
	}
}

func (c *adaptiveBootstrapCoordinator) Bootstrap(
	ctx context.Context,
	run *RunSummary,
) (*AdaptiveBootstrapFacts, error) {
	if run == nil {
		return nil, fmt.Errorf("adaptive bootstrap run is required")
	}
	if run.ParentRunID != 0 || (run.RunKind != "" && run.RunKind != RunKindTask) {
		return nil, nil
	}
	if c == nil || c.attemptReader == nil || c.repository == nil || c.idGen == nil || c.now == nil {
		return nil, fmt.Errorf("adaptive bootstrap dependencies are required")
	}
	if run.ThreadID <= 0 || run.RunID <= 0 {
		return nil, fmt.Errorf("adaptive bootstrap run identity is invalid")
	}

	attempt, err := c.attemptReader.GetActiveJournalAttempt(ctx, run.RunID)
	if errors.Is(err, domainrepo.ErrJournalNotEnrolled) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read active journal attempt for adaptive bootstrap: %w", err)
	}
	if run.ExecutionGeneration == 0 || !adaptiveBootstrapIdentityPart(run.LeaseOwner, 191) ||
		!adaptiveBootstrapIdentityPart(run.LeaseToken, 191) {
		return nil, fmt.Errorf("adaptive bootstrap run identity is invalid")
	}
	if !adaptiveBootstrapFreshAttempt(run, attempt) {
		return nil, fmt.Errorf("adaptive bootstrap requires the current fresh journal attempt")
	}

	readRequest := domainrepo.ReadAdaptiveExecutionBootstrapRequest{
		ThreadID:       run.ThreadID,
		ExecutionRunID: run.RunID,
		JournalRunID:   attempt.JournalRunID,
		AttemptID:      attempt.AttemptID,
	}
	result, err := c.repository.ReadAdaptiveExecutionBootstrap(ctx, readRequest)
	if err == nil {
		if result == nil {
			return nil, fmt.Errorf("adaptive bootstrap replay result is required")
		}
		facts, validateErr := adaptiveBootstrapFactsFromDurableResult(run, attempt, result)
		if validateErr != nil {
			return nil, validateErr
		}
		return facts, nil
	}
	if !errors.Is(err, domainrepo.ErrAdaptiveExecutionBootstrapNotFound) {
		return nil, fmt.Errorf("read adaptive execution bootstrap: %w", err)
	}

	operationKey := adaptiveBootstrapStableKey("operation", run, attempt)
	decisionID := adaptiveBootstrapStableKey("decision", run, attempt)
	admission := baselineAdaptiveAdmission()
	decision, err := (BaselineDecisionProducer{}).Produce(BaselineDecisionRequest{
		Admission:           admission,
		DecisionID:          decisionID,
		DecisionRevision:    1,
		ExecutionRunID:      run.RunID,
		JournalRunID:        attempt.JournalRunID,
		AttemptID:           attempt.AttemptID,
		ExecutionGeneration: run.ExecutionGeneration,
		PlanScopeRunID:      run.RunID,
		CreatedAt:           attempt.CreatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("produce baseline adaptive decision: %w", err)
	}
	ids, err := c.idGen.GenMultiIDs(ctx, 3)
	if err != nil {
		return nil, fmt.Errorf("allocate adaptive bootstrap identifiers: %w", err)
	}
	if len(ids) != 3 || ids[0] <= 0 || ids[1] <= 0 || ids[2] <= 0 ||
		ids[0] == ids[1] || ids[0] == ids[2] || ids[1] == ids[2] {
		return nil, fmt.Errorf("adaptive bootstrap identifiers are invalid")
	}
	now := c.now()
	if now <= 0 {
		return nil, fmt.Errorf("adaptive bootstrap clock is invalid")
	}
	committed, err := c.repository.CommitAdaptiveExecutionBootstrap(ctx, domainrepo.CommitAdaptiveExecutionBootstrapRequest{
		ThreadID: run.ThreadID, ExecutionRunID: run.RunID,
		JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
		LeaseOwner: run.LeaseOwner, LeaseToken: run.LeaseToken,
		OperationKey: operationKey, Generation: run.ExecutionGeneration,
		Now: now, FactCreatedAt: attempt.CreatedAt,
		Admission: admission, Decision: decision,
		AdmissionEventID: ids[0], DecisionEventID: ids[1], CheckpointID: ids[2],
	})
	if err != nil {
		return nil, fmt.Errorf("commit adaptive execution bootstrap: %w", err)
	}
	if committed == nil {
		return nil, fmt.Errorf("adaptive bootstrap commit result is required")
	}
	facts, err := adaptiveBootstrapFactsFromDurableResult(run, attempt, committed)
	if err != nil {
		return nil, err
	}
	return facts, nil
}

func (c *adaptiveBootstrapCoordinator) BootstrapResume(
	ctx context.Context,
	run *RunSummary,
	input *HarnessResumeInput,
) (*AdaptiveBootstrapFacts, error) {
	if run == nil || input == nil || c == nil || c.attemptReader == nil ||
		c.repository == nil || c.idGen == nil || c.now == nil {
		return nil, fmt.Errorf("adaptive resume bootstrap dependencies are required")
	}
	attempt, err := c.attemptReader.GetActiveJournalAttempt(ctx, run.RunID)
	if errors.Is(err, domainrepo.ErrJournalNotEnrolled) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read active journal attempt for adaptive resume bootstrap: %w", err)
	}
	if err := validateAdaptiveBootstrapRecoveryResume(run, input, attempt); err != nil {
		return nil, err
	}

	targetRead := domainrepo.ReadAdaptiveExecutionBootstrapRequest{
		ThreadID:       run.ThreadID,
		ExecutionRunID: run.RunID,
		JournalRunID:   attempt.JournalRunID,
		AttemptID:      attempt.AttemptID,
	}
	target, err := c.repository.ReadAdaptiveExecutionBootstrap(ctx, targetRead)
	if err == nil {
		return adaptiveBootstrapRecoveryFactsFromDurableResult(run, input, attempt, target)
	}
	if !errors.Is(err, domainrepo.ErrAdaptiveExecutionBootstrapNotFound) {
		return nil, fmt.Errorf("read target adaptive execution bootstrap: %w", err)
	}

	var admission domainentity.AdaptiveAdmissionSnapshot
	planScopeRunID := input.SourceRunID
	if attempt.SourceAttemptID == nil {
		admission, err = c.legacyAdaptiveAdmissionFromSourceRun(ctx, run, input.SourceRunID)
		if err != nil {
			return nil, err
		}
	} else {
		source, readErr := c.repository.ReadAdaptiveExecutionBootstrap(ctx, domainrepo.ReadAdaptiveExecutionBootstrapRequest{
			ThreadID:       run.ThreadID,
			ExecutionRunID: input.SourceRunID,
			JournalRunID:   attempt.JournalRunID,
			AttemptID:      *attempt.SourceAttemptID,
		})
		switch {
		case readErr == nil:
			admission, planScopeRunID, err = typedAdaptiveAdmissionFromSource(
				run.ThreadID,
				attempt.JournalRunID,
				input.SourceRunID,
				*attempt.SourceAttemptID,
				source,
			)
		case errors.Is(readErr, domainrepo.ErrAdaptiveExecutionBootstrapNotFound):
			admission, err = c.legacyAdaptiveAdmissionFromSourceRun(ctx, run, input.SourceRunID)
		default:
			return nil, fmt.Errorf("read source adaptive execution bootstrap: %w", readErr)
		}
		if err != nil {
			return nil, err
		}
	}
	decision, err := (BaselineDecisionProducer{}).Produce(BaselineDecisionRequest{
		Admission: admission, DecisionID: adaptiveBootstrapStableKey("decision", run, attempt),
		DecisionRevision: 1, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
		AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		PlanScopeRunID: planScopeRunID, CreatedAt: attempt.CreatedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("produce recovery adaptive decision: %w", err)
	}
	ids, err := c.idGen.GenMultiIDs(ctx, 3)
	if err != nil {
		return nil, fmt.Errorf("allocate recovery bootstrap identifiers: %w", err)
	}
	if len(ids) != 3 || ids[0] <= 0 || ids[1] <= 0 || ids[2] <= 0 ||
		ids[0] == ids[1] || ids[0] == ids[2] || ids[1] == ids[2] {
		return nil, fmt.Errorf("adaptive recovery bootstrap identifiers are invalid")
	}
	now := c.now()
	if now <= 0 {
		return nil, fmt.Errorf("adaptive recovery bootstrap clock is invalid")
	}
	committed, err := c.repository.CommitAdaptiveExecutionBootstrap(ctx, domainrepo.CommitAdaptiveExecutionBootstrapRequest{
		ThreadID: run.ThreadID, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
		AttemptID: attempt.AttemptID, LeaseOwner: run.LeaseOwner, LeaseToken: run.LeaseToken,
		OperationKey: adaptiveBootstrapStableKey("operation", run, attempt), Generation: run.ExecutionGeneration,
		Now: now, FactCreatedAt: attempt.CreatedAt, Admission: admission, Decision: decision,
		AdmissionEventID: ids[0], DecisionEventID: ids[1], CheckpointID: ids[2],
	})
	if err != nil {
		return nil, fmt.Errorf("commit recovery adaptive bootstrap: %w", err)
	}
	return adaptiveBootstrapRecoveryFactsFromDurableResult(run, input, attempt, committed)
}

func (c *adaptiveBootstrapCoordinator) legacyAdaptiveAdmissionFromSourceRun(
	ctx context.Context,
	target *RunSummary,
	sourceRunID int64,
) (domainentity.AdaptiveAdmissionSnapshot, error) {
	if c == nil || c.sourceRunReader == nil {
		return domainentity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("legacy adaptive bootstrap source run reader is required")
	}
	source, err := c.sourceRunReader.GetRun(ctx, &domainservice.GetRunRequest{RunID: sourceRunID})
	if err != nil {
		return domainentity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("read legacy adaptive bootstrap source run: %w", err)
	}
	if source == nil || target == nil || source.ID != sourceRunID || source.ThreadID != target.ThreadID {
		return domainentity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("legacy adaptive bootstrap source run identity is invalid")
	}
	admission, err := NewLegacyAdaptiveAdmissionDecoder().Decode(source)
	if err != nil {
		return domainentity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("decode legacy adaptive bootstrap source run: %w", err)
	}
	return admission, nil
}

func validateAdaptiveBootstrapRecoveryResume(
	run *RunSummary,
	input *HarnessResumeInput,
	attempt *domainentity.RunAttempt,
) error {
	if run.ThreadID <= 0 || run.RunID <= 0 || run.ExecutionGeneration == 0 ||
		!adaptiveBootstrapIdentityPart(run.LeaseOwner, 191) || !adaptiveBootstrapIdentityPart(run.LeaseToken, 191) ||
		input.ThreadID != run.ThreadID || input.RunID != run.RunID || input.SourceRunID <= 0 ||
		input.SourceRunID == run.RunID || input.CheckpointID <= 0 || attempt == nil ||
		attempt.ThreadID != run.ThreadID || attempt.ExecutionRunID != run.RunID || attempt.JournalRunID <= 0 ||
		!adaptiveBootstrapIdentityPart(attempt.AttemptID, 64) || !attempt.Status.IsActive() ||
		attempt.ActiveSlot == nil || *attempt.ActiveSlot != 1 || attempt.CreatedAt <= 0 || attempt.SourceCheckpointID == nil ||
		*attempt.SourceCheckpointID != input.CheckpointID || attempt.RecoveryIdempotencyKey == nil ||
		!adaptiveBootstrapIdentityPart(*attempt.RecoveryIdempotencyKey, 191) {
		return fmt.Errorf("adaptive resume bootstrap target identity or lineage is invalid")
	}
	bareSource := attempt.SourceAttemptID == nil &&
		attempt.ProjectionState == domainentity.JournalProjectionStateDisabled && attempt.Ordinal == 1 &&
		attempt.JournalRunID == input.SourceRunID
	attemptedSource := attempt.SourceAttemptID != nil &&
		adaptiveBootstrapIdentityPart(*attempt.SourceAttemptID, 64)
	if !bareSource && !attemptedSource {
		return fmt.Errorf("adaptive resume bootstrap target identity or lineage is invalid")
	}
	return nil
}

func typedAdaptiveAdmissionFromSource(
	expectedThreadID int64,
	expectedJournalRunID int64,
	sourceRunID int64,
	expectedSourceAttemptID string,
	source *domainrepo.CommitAdaptiveExecutionBootstrapResult,
) (domainentity.AdaptiveAdmissionSnapshot, int64, error) {
	if source == nil || expectedThreadID <= 0 || expectedJournalRunID <= 0 || sourceRunID <= 0 ||
		!adaptiveBootstrapIdentityPart(expectedSourceAttemptID, 64) ||
		source.Authority.ThreadID != expectedThreadID ||
		source.Authority.ExecutionRunID != sourceRunID || source.Authority.JournalRunID <= 0 ||
		source.Authority.JournalRunID != expectedJournalRunID ||
		source.Authority.AttemptID != expectedSourceAttemptID ||
		source.Authority.ExecutionGeneration == 0 || source.Admission.FeatureGateEnabled ||
		(source.Admission.Source != domainentity.AdaptiveAdmissionSourceFresh &&
			source.Admission.Source != domainentity.AdaptiveAdmissionSourceTypedInheritance &&
			source.Admission.Source != domainentity.AdaptiveAdmissionSourceLegacyDecoder) {
		return domainentity.AdaptiveAdmissionSnapshot{}, 0, fmt.Errorf("adaptive recovery source facts are invalid")
	}
	if source.Decision.PlanScopeRunID == nil || *source.Decision.PlanScopeRunID <= 0 {
		return domainentity.AdaptiveAdmissionSnapshot{}, 0, fmt.Errorf("adaptive recovery source plan scope is invalid")
	}
	planScopeRunID := *source.Decision.PlanScopeRunID
	if source.Admission.Source == domainentity.AdaptiveAdmissionSourceFresh &&
		planScopeRunID != source.Authority.ExecutionRunID {
		return domainentity.AdaptiveAdmissionSnapshot{}, 0, fmt.Errorf("adaptive recovery source plan scope is invalid")
	}
	if err := adaptivecontract.ValidateAdaptiveBootstrapPair(
		source.Admission,
		source.Decision,
		adaptivecontract.BootstrapIdentity{
			ExecutionRunID:         source.Authority.ExecutionRunID,
			JournalRunID:           source.Authority.JournalRunID,
			AttemptID:              source.Authority.AttemptID,
			ExecutionGeneration:    source.Authority.ExecutionGeneration,
			ExpectedPlanScopeRunID: planScopeRunID,
		},
	); err != nil {
		return domainentity.AdaptiveAdmissionSnapshot{}, 0, fmt.Errorf("validate adaptive recovery source facts: %w", err)
	}
	sourceGeneration := source.Authority.ExecutionGeneration
	return domainentity.AdaptiveAdmissionSnapshot{
		Schema:                    domainentity.AdaptiveAdmissionSchemaV1,
		FeatureGateEnabled:        false,
		Source:                    domainentity.AdaptiveAdmissionSourceTypedInheritance,
		SourceRunID:               &sourceRunID,
		SourceExecutionGeneration: &sourceGeneration,
		Capabilities:              source.Admission.Capabilities,
		Limits:                    source.Admission.Limits,
	}, planScopeRunID, nil
}

func adaptiveBootstrapRecoveryFactsFromDurableResult(
	run *RunSummary,
	input *HarnessResumeInput,
	attempt *domainentity.RunAttempt,
	result *domainrepo.CommitAdaptiveExecutionBootstrapResult,
) (*AdaptiveBootstrapFacts, error) {
	if result == nil || run == nil || input == nil || attempt == nil ||
		(result.Admission.Source != domainentity.AdaptiveAdmissionSourceTypedInheritance &&
			result.Admission.Source != domainentity.AdaptiveAdmissionSourceLegacyDecoder) ||
		result.Admission.FeatureGateEnabled || result.Admission.SourceRunID == nil ||
		*result.Admission.SourceRunID != input.SourceRunID ||
		result.Authority.ThreadID != run.ThreadID || result.Authority.ExecutionRunID != run.RunID ||
		result.Authority.JournalRunID != attempt.JournalRunID || result.Authority.AttemptID != attempt.AttemptID ||
		result.Authority.ExecutionGeneration != run.ExecutionGeneration ||
		result.Decision.ExecutionRunID != run.RunID || result.Decision.JournalRunID != attempt.JournalRunID ||
		result.Decision.AttemptID != attempt.AttemptID ||
		result.Decision.ExecutionGeneration != run.ExecutionGeneration || result.Decision.DecisionRevision != 1 ||
		result.Decision.PlanScopeRunID == nil || *result.Decision.PlanScopeRunID <= 0 ||
		result.Decision.CreatedAt != attempt.CreatedAt {
		return nil, fmt.Errorf("adaptive recovery bootstrap facts do not match the current target")
	}
	if err := adaptivecontract.ValidateAdaptiveBootstrapPair(
		result.Admission,
		result.Decision,
		adaptivecontract.BootstrapIdentity{
			ExecutionRunID:         result.Authority.ExecutionRunID,
			JournalRunID:           result.Authority.JournalRunID,
			AttemptID:              result.Authority.AttemptID,
			ExecutionGeneration:    result.Authority.ExecutionGeneration,
			ExpectedPlanScopeRunID: *result.Decision.PlanScopeRunID,
		},
	); err != nil {
		return nil, fmt.Errorf("validate adaptive recovery bootstrap facts: %w", err)
	}
	return cloneAdaptiveBootstrapFacts(&AdaptiveBootstrapFacts{
		Admission: result.Admission,
		Decision:  result.Decision,
	}), nil
}

func adaptiveBootstrapFactsFromDurableResult(
	run *RunSummary,
	attempt *domainentity.RunAttempt,
	result *domainrepo.CommitAdaptiveExecutionBootstrapResult,
) (*AdaptiveBootstrapFacts, error) {
	if result == nil || run == nil || attempt == nil {
		return nil, fmt.Errorf("adaptive bootstrap durable facts do not match the current run claim")
	}
	if result.Admission.Source != domainentity.AdaptiveAdmissionSourceFresh ||
		result.Admission.FeatureGateEnabled || result.Decision.DecisionRevision != 1 ||
		result.Authority.ExecutionGeneration != run.ExecutionGeneration ||
		result.Decision.ExecutionGeneration != run.ExecutionGeneration {
		return nil, fmt.Errorf("adaptive bootstrap replay generation does not match the current run claim")
	}
	if result.Authority.ThreadID != run.ThreadID || result.Authority.ExecutionRunID != run.RunID ||
		result.Authority.JournalRunID != attempt.JournalRunID || result.Authority.AttemptID != attempt.AttemptID ||
		result.Decision.ExecutionRunID != run.RunID || result.Decision.JournalRunID != attempt.JournalRunID ||
		result.Decision.AttemptID != attempt.AttemptID ||
		(result.Decision.PlanScopeRunID != nil && *result.Decision.PlanScopeRunID != run.RunID) {
		return nil, fmt.Errorf("adaptive bootstrap durable facts do not match the current run claim")
	}
	if err := ValidateExecutionDecisionAgainstAdmission(result.Admission, result.Decision); err != nil {
		return nil, fmt.Errorf("validate adaptive bootstrap durable facts: %w", err)
	}
	return cloneAdaptiveBootstrapFacts(&AdaptiveBootstrapFacts{
		Admission: result.Admission,
		Decision:  result.Decision,
	}), nil
}

func cloneAdaptiveBootstrapFacts(facts *AdaptiveBootstrapFacts) *AdaptiveBootstrapFacts {
	if facts == nil {
		return nil
	}
	clone := *facts
	if facts.Admission.SourceRunID != nil {
		value := *facts.Admission.SourceRunID
		clone.Admission.SourceRunID = &value
	}
	if facts.Admission.SourceExecutionGeneration != nil {
		value := *facts.Admission.SourceExecutionGeneration
		clone.Admission.SourceExecutionGeneration = &value
	}
	if facts.Decision.Deliverables != nil {
		clone.Decision.Deliverables = append([]string{}, facts.Decision.Deliverables...)
	}
	if facts.Decision.AcceptanceChecks != nil {
		clone.Decision.AcceptanceChecks = append(
			[]domainentity.AdaptiveAcceptanceCheck{},
			facts.Decision.AcceptanceChecks...,
		)
	}
	if facts.Decision.PlanScopeRunID != nil {
		value := *facts.Decision.PlanScopeRunID
		clone.Decision.PlanScopeRunID = &value
	}
	if facts.Decision.ClarificationQuestion != nil {
		value := *facts.Decision.ClarificationQuestion
		clone.Decision.ClarificationQuestion = &value
	}
	return &clone
}

func baselineAdaptiveAdmission() domainentity.AdaptiveAdmissionSnapshot {
	return domainentity.AdaptiveAdmissionSnapshot{
		Schema:             domainentity.AdaptiveAdmissionSchemaV1,
		FeatureGateEnabled: false,
		Source:             domainentity.AdaptiveAdmissionSourceFresh,
		Capabilities: domainentity.AdaptiveCapabilities{
			PlanAllowed:             true,
			HumanInteractionAllowed: true,
		},
		Limits: domainentity.AdaptiveLimits{
			MaxToolCalls: 24, MaxReplans: 2, MaxVerificationRepairs: 2,
			MaxConsecutiveNoProgress: 3, MaxActiveDurationSeconds: 1200,
		},
	}
}

func adaptiveBootstrapFreshAttempt(run *RunSummary, attempt *domainentity.RunAttempt) bool {
	return attempt != nil && attempt.ThreadID == run.ThreadID && attempt.ExecutionRunID == run.RunID &&
		attempt.JournalRunID > 0 && adaptiveBootstrapIdentityPart(attempt.AttemptID, 64) &&
		attempt.Status.IsActive() && attempt.ActiveSlot != nil && attempt.CreatedAt > 0 &&
		attempt.SourceAttemptID == nil && attempt.SourceCheckpointID == nil &&
		attempt.RecoveryIdempotencyKey == nil
}

func adaptiveBootstrapIdentityPart(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && strings.TrimSpace(value) == value
}

func adaptiveBootstrapStableKey(kind string, run *RunSummary, attempt *domainentity.RunAttempt) string {
	input := strings.Join([]string{
		adaptiveBootstrapIdentitySchema,
		kind,
		strconv.FormatInt(run.ThreadID, 10),
		strconv.FormatInt(run.RunID, 10),
		strconv.FormatInt(attempt.JournalRunID, 10),
		attempt.AttemptID,
		strconv.FormatUint(run.ExecutionGeneration, 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(input))
	return "adaptive-" + kind + ":" + hex.EncodeToString(digest[:])
}
