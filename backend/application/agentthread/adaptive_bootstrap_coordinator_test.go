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
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveBootstrapCoordinatorCommitsFreshRootADKRun(t *testing.T) {
	attempt := freshAdaptiveBootstrapAttemptForTest()
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: attempt}
	repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	run := freshAdaptiveBootstrapRunForTest()
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader,
		Repository:    repo,
		IDGen:         ids,
		Now:           func() int64 { return 999 },
	})

	facts, err := coordinator.Bootstrap(context.Background(), run)

	require.NoError(t, err)
	require.NotNil(t, facts)
	require.Equal(t, 1, reader.calls)
	require.Len(t, repo.readRequests, 1)
	require.Equal(t, repository.ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	}, repo.readRequests[0])
	require.Equal(t, []int{3}, ids.counts)
	require.Len(t, repo.commitRequests, 1)
	req := repo.commitRequests[0]
	require.Equal(t, int64(10), req.ThreadID)
	require.Equal(t, int64(20), req.ExecutionRunID)
	require.Equal(t, int64(30), req.JournalRunID)
	require.Equal(t, "attempt-1", req.AttemptID)
	require.Equal(t, "worker-1", req.LeaseOwner)
	require.Equal(t, "lease-1", req.LeaseToken)
	require.Equal(t, uint64(4), req.Generation)
	require.Equal(t, int64(999), req.Now)
	require.Equal(t, int64(700), req.FactCreatedAt)
	require.Equal(t, []int64{101, 102, 103}, []int64{
		req.AdmissionEventID, req.DecisionEventID, req.CheckpointID,
	})
	require.Equal(t, adaptiveBootstrapStableKeyForTest("operation", run, attempt), req.OperationKey)
	require.Equal(t, adaptiveBootstrapStableKeyForTest("decision", run, attempt), req.Decision.DecisionID)
	require.Equal(t, entity.AdaptiveAdmissionSnapshot{
		Schema:             entity.AdaptiveAdmissionSchemaV1,
		FeatureGateEnabled: false,
		Source:             entity.AdaptiveAdmissionSourceFresh,
		Capabilities: entity.AdaptiveCapabilities{
			PlanAllowed: true, HumanInteractionAllowed: true,
		},
		Limits: entity.AdaptiveLimits{
			MaxToolCalls: 24, MaxReplans: 2, MaxVerificationRepairs: 2,
			MaxConsecutiveNoProgress: 3, MaxActiveDurationSeconds: 1200,
		},
	}, req.Admission)
	require.Equal(t, entity.ExecutionDecisionExecute, req.Decision.Decision)
	require.Equal(t, entity.ExecutionShapeMultiStep, req.Decision.ExecutionShape)
	require.Equal(t, uint64(1), req.Decision.DecisionRevision)
	require.Equal(t, int64(20), *req.Decision.PlanScopeRunID)
	require.Equal(t, int64(700), req.Decision.CreatedAt)
	require.Equal(t, req.Admission, facts.Admission)
	require.Equal(t, req.Decision, facts.Decision)

	otherRepo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
	otherCoordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader,
		Repository:    otherRepo,
		IDGen:         &adaptiveBootstrapIDGeneratorStub{ids: []int64{201, 202, 203}},
		Now:           func() int64 { return 123456 },
	})
	otherRun := *run
	otherRun.LeaseOwner, otherRun.LeaseToken = "another-worker", "another-lease"
	otherFacts, err := otherCoordinator.Bootstrap(context.Background(), &otherRun)
	require.NoError(t, err)
	require.NotNil(t, otherFacts)
	require.Equal(t, req.OperationKey, otherRepo.commitRequests[0].OperationKey)
	require.Equal(t, req.Decision.DecisionID, otherRepo.commitRequests[0].Decision.DecisionID)
}

func TestAdaptiveBootstrapCoordinatorUsesGateOnEligibilityAndProducerBeforeIDs(t *testing.T) {
	attempt := freshAdaptiveBootstrapAttemptForTest()
	run := freshAdaptiveBootstrapRunForTest()
	run.SpaceID = 77
	run.Input = `{"messages":[{"role":"assistant","content":"previous result"},{"role":"user","content":"continue safely"}]}`
	resolver := &adaptiveEligibilityResolverStub{admission: baselineAdaptiveAdmission()}
	resolver.admission.FeatureGateEnabled = true
	producer := &adaptiveDecisionProducerStub{candidate: adaptiveDecisionCandidateForTest(true)}
	repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: readerForAdaptiveAttempt(attempt), Repository: repo, IDGen: ids,
		EligibilityResolver: resolver, AdaptiveProducer: producer, Now: func() int64 { return 999 },
	})

	facts, err := coordinator.Bootstrap(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, []AdaptiveEligibilityRequest{{SpaceID: 77}}, resolver.requests)
	require.Len(t, producer.requests, 1)
	require.True(t, producer.requests[0].Admission.FeatureGateEnabled)
	require.Equal(t, AdaptiveDecisionSemanticInput{Messages: []AdaptiveDecisionSemanticMessage{
		{Role: "assistant", Content: "previous result"},
		{Role: "user", Content: "continue safely"},
	}}, producer.requests[0].SemanticInput)
	require.Len(t, producer.contexts, 1)
	invocation, ok := adaptiveDecisionModelInvocationFromContext(producer.contexts[0])
	require.True(t, ok)
	require.Equal(t, run, invocation.run)
	require.Equal(t, attempt, invocation.attempt)
	require.Equal(t, adaptiveBootstrapStableKeyForTest("operation", run, attempt), invocation.operationKey)
	require.Equal(t, []int{3}, ids.counts)
	require.Len(t, repo.commitRequests, 1)
	require.True(t, repo.commitRequests[0].Admission.FeatureGateEnabled)
	require.True(t, facts.Admission.FeatureGateEnabled)
	require.Equal(t, run.RunID, facts.Decision.ExecutionRunID)
	require.Equal(t, uint64(1), facts.Decision.DecisionRevision)
	require.Equal(t, attempt.CreatedAt, facts.Decision.CreatedAt)
}

func TestAdaptiveBootstrapCoordinatorMaterializesServerAuthorityAfterCandidate(t *testing.T) {
	attempt := freshAdaptiveBootstrapAttemptForTest()
	run := freshAdaptiveBootstrapRunForTest()
	run.SpaceID = 77
	producer := &adaptiveDecisionProducerStub{candidate: adaptiveDecisionCandidateForTest(true)}
	repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: readerForAdaptiveAttempt(attempt), Repository: repo,
		IDGen:               &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}},
		EligibilityResolver: gateOnEligibilityResolverForTest(), AdaptiveProducer: producer,
		Now: func() int64 { return 999 },
	})

	_, err := coordinator.Bootstrap(context.Background(), run)

	require.NoError(t, err)
	require.Len(t, repo.commitRequests, 1)
	decision := repo.commitRequests[0].Decision
	require.Equal(t, entity.ExecutionDecisionSchemaV1, decision.Schema)
	require.Equal(t, adaptiveBootstrapStableKeyForTest("decision", run, attempt), decision.DecisionID)
	require.Equal(t, uint64(1), decision.DecisionRevision)
	require.Equal(t, run.RunID, decision.ExecutionRunID)
	require.Equal(t, attempt.JournalRunID, decision.JournalRunID)
	require.Equal(t, attempt.AttemptID, decision.AttemptID)
	require.Equal(t, run.ExecutionGeneration, decision.ExecutionGeneration)
	require.Equal(t, run.RunID, *decision.PlanScopeRunID)
	require.Equal(t, attempt.CreatedAt, decision.CreatedAt)
}

func TestAdaptiveBootstrapCoordinatorMaterializesCandidateForms(t *testing.T) {
	question := "Which result should be produced?"
	for _, test := range []struct {
		name          string
		candidate     AdaptiveDecisionCandidate
		wantPlanScope bool
	}{
		{name: "clarification", candidate: AdaptiveDecisionCandidate{
			GoalSummary: "Clarify the requested outcome.", Deliverables: []string{},
			AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
			Decision:         entity.ExecutionDecisionClarification, ExecutionShape: entity.ExecutionShapeEmpty,
			ClarificationQuestion: &question, SafeSummary: "Ask one safe clarification question.",
		}},
		{name: "direct", candidate: AdaptiveDecisionCandidate{
			GoalSummary: "Answer the request directly.", Deliverables: []string{},
			AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
			Decision:         entity.ExecutionDecisionDirect, ExecutionShape: entity.ExecutionShapeEmpty,
			SafeSummary: "Use the direct execution path.",
		}},
		{name: "single step", candidate: AdaptiveDecisionCandidate{
			GoalSummary: "Execute one bounded step.", Deliverables: []string{},
			AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
			Decision:         entity.ExecutionDecisionExecute, ExecutionShape: entity.ExecutionShapeSingleStep,
			SafeSummary: "Use the single-step execution path.",
		}},
		{name: "multi step", wantPlanScope: true, candidate: AdaptiveDecisionCandidate{
			GoalSummary: "Execute bounded steps.", Deliverables: []string{},
			AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
			Decision:         entity.ExecutionDecisionExecute, ExecutionShape: entity.ExecutionShapeMultiStep,
			SafeSummary: "Use the multi-step execution path.",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			coordinator := &adaptiveBootstrapCoordinator{
				adaptiveProducer: &adaptiveDecisionProducerStub{candidate: test.candidate},
			}
			admission := baselineAdaptiveAdmission()
			admission.FeatureGateEnabled = true
			decision, err := coordinator.produceDecision(context.Background(), admission, adaptiveDecisionAuthority{
				DecisionID: "decision-1", ExecutionRunID: 20, JournalRunID: 30,
				AttemptID: "attempt-1", ExecutionGeneration: 4, PlanScopeRunID: 20, CreatedAt: 700,
			})

			require.NoError(t, err)
			if test.wantPlanScope {
				require.Equal(t, int64(20), *decision.PlanScopeRunID)
			} else {
				require.Nil(t, decision.PlanScopeRunID)
			}
			require.Equal(t, test.candidate.ClarificationQuestion, decision.ClarificationQuestion)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorRejectsInvalidCandidateForm(t *testing.T) {
	question := "Should not be present."
	for _, candidate := range []AdaptiveDecisionCandidate{
		{
			GoalSummary: "Invalid direct candidate.", Deliverables: []string{},
			AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
			Decision:         entity.ExecutionDecisionDirect, ExecutionShape: entity.ExecutionShapeSingleStep,
			SafeSummary: "Reject this invalid candidate.",
		},
		{
			GoalSummary: "Invalid execute candidate.", Deliverables: []string{},
			AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
			Decision:         entity.ExecutionDecisionExecute, ExecutionShape: entity.ExecutionShapeMultiStep,
			ClarificationQuestion: &question, SafeSummary: "Reject this invalid candidate.",
		},
	} {
		coordinator := &adaptiveBootstrapCoordinator{
			adaptiveProducer: &adaptiveDecisionProducerStub{candidate: candidate},
		}
		admission := baselineAdaptiveAdmission()
		admission.FeatureGateEnabled = true

		_, err := coordinator.produceDecision(context.Background(), admission, adaptiveDecisionAuthority{
			DecisionID: "decision-1", ExecutionRunID: 20, JournalRunID: 30,
			AttemptID: "attempt-1", ExecutionGeneration: 4, PlanScopeRunID: 20, CreatedAt: 700,
		})

		require.ErrorIs(t, err, ErrExecutionDecisionInvalid)
	}
}

func TestAdaptiveBootstrapCoordinatorIsolatesAdmissionLineageFromProducer(t *testing.T) {
	sourceRunID := int64(20)
	sourceGeneration := uint64(3)
	admission := baselineAdaptiveAdmission()
	admission.FeatureGateEnabled = true
	admission.Source = entity.AdaptiveAdmissionSourceTypedInheritance
	admission.SourceRunID = &sourceRunID
	admission.SourceExecutionGeneration = &sourceGeneration
	producer := &adaptiveDecisionProducerStub{
		candidate: adaptiveDecisionCandidateForTest(true),
		mutateAdmission: func(candidate entity.AdaptiveAdmissionSnapshot) {
			*candidate.SourceRunID = 999
			*candidate.SourceExecutionGeneration = 999
		},
	}
	coordinator := &adaptiveBootstrapCoordinator{adaptiveProducer: producer}

	_, err := coordinator.produceDecision(context.Background(), admission, adaptiveDecisionAuthority{
		DecisionID: "decision-1", ExecutionRunID: 21, JournalRunID: 20,
		AttemptID: "attempt-2", ExecutionGeneration: 4, PlanScopeRunID: 20, CreatedAt: 700,
	})

	require.NoError(t, err)
	require.Equal(t, int64(20), sourceRunID)
	require.Equal(t, uint64(3), sourceGeneration)
}

func TestAdaptiveBootstrapCoordinatorFailsClosedBeforeIDsForEligibilityOrProducer(t *testing.T) {
	for _, test := range []struct {
		name     string
		resolver AdaptiveEligibilityResolver
		producer AdaptiveDecisionProducer
	}{
		{name: "eligibility error", resolver: &adaptiveEligibilityResolverStub{err: errors.New("eligibility failed")}},
		{name: "missing producer", resolver: gateOnEligibilityResolverForTest()},
		{name: "producer error", resolver: gateOnEligibilityResolverForTest(), producer: &adaptiveDecisionProducerStub{err: errors.New("produce failed")}},
		{name: "invalid candidate", resolver: gateOnEligibilityResolverForTest(), producer: &adaptiveDecisionProducerStub{candidate: AdaptiveDecisionCandidate{Decision: entity.ExecutionDecisionExecute, ExecutionShape: entity.ExecutionShapeEmpty}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: readerForAdaptiveAttempt(freshAdaptiveBootstrapAttemptForTest()),
				Repository:    repo, IDGen: ids, EligibilityResolver: test.resolver,
				AdaptiveProducer: test.producer, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())

			require.Error(t, err)
			require.Nil(t, facts)
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorGateOnProjectionFailureStopsBeforeProducerAndIDs(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	run.Input = `{"messages":[{"role":"user","content":"go"}],"unknown":true}`
	producer := &adaptiveDecisionProducerStub{candidate: adaptiveDecisionCandidateForTest(true)}
	repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: readerForAdaptiveAttempt(freshAdaptiveBootstrapAttemptForTest()),
		Repository:    repo, IDGen: ids, EligibilityResolver: gateOnEligibilityResolverForTest(),
		AdaptiveProducer: producer, Now: func() int64 { return 999 },
	})

	facts, err := coordinator.Bootstrap(context.Background(), run)

	require.ErrorContains(t, err, "project adaptive decision semantic input")
	require.Nil(t, facts)
	require.Empty(t, producer.requests)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorTypedResumeInheritsGateOnWithoutRecomputingEligibility(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	source := adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSourceFresh)
	source.Admission.FeatureGateEnabled = true
	source.Decision = adaptiveDecisionForBootstrapTest(source.Admission, source.Decision)
	resolver := &adaptiveEligibilityResolverStub{err: errors.New("must not be called")}
	producer := &adaptiveDecisionProducerStub{candidate: adaptiveDecisionCandidateForTest(true)}
	repo := &adaptiveBootstrapRepositoryStub{
		readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{nil, source},
		readErrs:    []error{repository.ErrAdaptiveExecutionBootstrapNotFound, nil},
	}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: readerForAdaptiveAttempt(attempt), Repository: repo, IDGen: ids,
		EligibilityResolver: resolver, AdaptiveProducer: producer, Now: func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)

	require.NoError(t, err)
	require.Empty(t, resolver.requests)
	require.Empty(t, producer.requests)
	require.True(t, repo.commitRequests[0].Admission.FeatureGateEnabled)
	require.True(t, facts.Admission.FeatureGateEnabled)
	require.Equal(t, source.Decision.GoalSummary, facts.Decision.GoalSummary)
	require.Equal(t, source.Decision.Decision, facts.Decision.Decision)
	require.Equal(t, source.Decision.ExecutionShape, facts.Decision.ExecutionShape)
}

func TestAdaptiveBootstrapCoordinatorTypedResumeCopiesDecisionWithoutProducer(t *testing.T) {
	question := "Which safe result should be produced?"
	for _, test := range []struct {
		name   string
		mutate func(*entity.ExecutionDecision)
	}{
		{name: "direct", mutate: func(decision *entity.ExecutionDecision) {
			decision.Decision = entity.ExecutionDecisionDirect
			decision.ExecutionShape = entity.ExecutionShapeEmpty
			decision.PlanScopeRunID = nil
		}},
		{name: "single step", mutate: func(decision *entity.ExecutionDecision) {
			decision.ExecutionShape = entity.ExecutionShapeSingleStep
			decision.PlanScopeRunID = nil
		}},
		{name: "multi step", mutate: func(*entity.ExecutionDecision) {}},
		{name: "clarification", mutate: func(decision *entity.ExecutionDecision) {
			decision.Decision = entity.ExecutionDecisionClarification
			decision.ExecutionShape = entity.ExecutionShapeEmpty
			decision.PlanScopeRunID = nil
			decision.ClarificationQuestion = &question
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			source := adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSourceFresh)
			source.Admission.FeatureGateEnabled = true
			source.Decision.GoalSummary = "Preserve the source decision."
			source.Decision.Deliverables = []string{"source-deliverable"}
			source.Decision.AcceptanceChecks = []entity.AdaptiveAcceptanceCheck{{
				CheckID: "source-check", Kind: "assertion", TargetRef: "source-target",
				SafeDescription: "Preserve this source acceptance check.",
			}}
			source.Decision.SafeSummary = "Reuse the already persisted source decision."
			test.mutate(&source.Decision)
			producer := &adaptiveDecisionProducerStub{err: errors.New("must not be called")}
			repo := &adaptiveBootstrapRepositoryStub{
				readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{nil, source},
				readErrs:    []error{repository.ErrAdaptiveExecutionBootstrapNotFound, nil},
			}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: readerForAdaptiveAttempt(attempt), Repository: repo, IDGen: ids,
				AdaptiveProducer: producer, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			require.NoError(t, err)
			require.Empty(t, producer.requests)
			require.Len(t, repo.commitRequests, 1)
			target := repo.commitRequests[0].Decision
			require.Equal(t, source.Decision.GoalSummary, target.GoalSummary)
			require.Equal(t, source.Decision.Deliverables, target.Deliverables)
			require.Equal(t, source.Decision.AcceptanceChecks, target.AcceptanceChecks)
			require.Equal(t, source.Decision.Decision, target.Decision)
			require.Equal(t, source.Decision.ExecutionShape, target.ExecutionShape)
			require.Equal(t, source.Decision.ClarificationQuestion, target.ClarificationQuestion)
			require.Equal(t, source.Decision.SafeSummary, target.SafeSummary)
			require.Equal(t, run.RunID, target.ExecutionRunID)
			require.Equal(t, attempt.AttemptID, target.AttemptID)
			require.Equal(t, run.ExecutionGeneration, target.ExecutionGeneration)
			require.Equal(t, attempt.CreatedAt, target.CreatedAt)
			if source.Decision.PlanScopeRunID == nil {
				require.Nil(t, target.PlanScopeRunID)
			} else {
				require.Equal(t, *source.Decision.PlanScopeRunID, requireInt64PointerForAdaptiveBootstrapTest(t, target.PlanScopeRunID))
			}
			require.Equal(t, target, facts.Decision)
		})
	}
}

func TestTypedAdaptiveAdmissionFromSourceDeepClonesDecisionCandidate(t *testing.T) {
	source := adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSourceFresh)
	question := "Which safe result should be produced?"
	source.Decision.Deliverables = []string{"source-deliverable"}
	source.Decision.AcceptanceChecks = []entity.AdaptiveAcceptanceCheck{{
		CheckID: "source-check", Kind: "assertion", TargetRef: "source-target",
		SafeDescription: "Preserve this source acceptance check.",
	}}
	source.Decision.Decision = entity.ExecutionDecisionClarification
	source.Decision.ExecutionShape = entity.ExecutionShapeEmpty
	source.Decision.PlanScopeRunID = nil
	source.Decision.ClarificationQuestion = &question

	_, candidate, planScopeRunID, err := typedAdaptiveAdmissionFromSource(
		source.Authority.ThreadID,
		source.Authority.JournalRunID,
		source.Authority.ExecutionRunID,
		source.Authority.AttemptID,
		source,
	)

	require.NoError(t, err)
	require.Zero(t, planScopeRunID)
	candidate.Deliverables[0] = "mutated-deliverable"
	candidate.AcceptanceChecks[0].CheckID = "mutated-check"
	*candidate.ClarificationQuestion = "mutated-question"
	require.Equal(t, "source-deliverable", source.Decision.Deliverables[0])
	require.Equal(t, "source-check", source.Decision.AcceptanceChecks[0].CheckID)
	require.Equal(t, question, *source.Decision.ClarificationQuestion)
}

func TestAdaptiveBootstrapCoordinatorReplaysBeforeAllocatingIDs(t *testing.T) {
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: freshAdaptiveBootstrapAttemptForTest()}
	run := freshAdaptiveBootstrapRunForTest()
	attempt := freshAdaptiveBootstrapAttemptForTest()
	repo := &adaptiveBootstrapRepositoryStub{readResult: adaptiveBootstrapResultForTest(t, run, attempt)}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader, Repository: repo, IDGen: ids, Now: func() int64 { return 999 },
	})

	facts, err := coordinator.Bootstrap(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, repo.readResult.Admission, facts.Admission)
	require.Equal(t, repo.readResult.Decision, facts.Decision)
	require.Equal(t, 1, reader.calls)
	require.Len(t, repo.readRequests, 1)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorGateOnReplaySkipsProjectionAndProducer(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	run.Input = `{"messages":null}`
	attempt := freshAdaptiveBootstrapAttemptForTest()
	replay := adaptiveBootstrapResultForTest(t, run, attempt)
	replay.Admission.FeatureGateEnabled = true
	replay.Decision = adaptiveDecisionForBootstrapTest(replay.Admission, replay.Decision)
	resolver := &adaptiveEligibilityResolverStub{err: errors.New("must not be called")}
	producer := &adaptiveDecisionProducerStub{err: errors.New("must not be called")}
	repo := &adaptiveBootstrapRepositoryStub{readResult: replay}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: readerForAdaptiveAttempt(attempt), Repository: repo, IDGen: ids,
		EligibilityResolver: resolver, AdaptiveProducer: producer, Now: func() int64 { return 999 },
	})

	facts, err := coordinator.Bootstrap(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, replay.Decision, facts.Decision)
	require.Empty(t, resolver.requests)
	require.Empty(t, producer.requests)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorResumeReplaysTargetBeforeSourceAndIDs(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	target := adaptiveBootstrapTypedResultForTest(t, run, input, attempt, 3)
	target.Admission.FeatureGateEnabled = true
	target.Decision = adaptiveDecisionForBootstrapTest(target.Admission, target.Decision)
	producer := &adaptiveDecisionProducerStub{err: errors.New("must not be called")}
	repo := &adaptiveBootstrapRepositoryStub{readResult: target}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader:    &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository:       repo,
		IDGen:            ids,
		AdaptiveProducer: producer,
		Now:              func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)

	require.NoError(t, err)
	require.Equal(t, target.Admission, facts.Admission)
	require.Equal(t, target.Decision, facts.Decision)
	require.Equal(t, []repository.ReadAdaptiveExecutionBootstrapRequest{{
		ThreadID: 10, ExecutionRunID: 21, JournalRunID: 30, AttemptID: "attempt-2",
	}}, repo.readRequests)
	require.Equal(t, []int64{run.RunID}, coordinator.(*adaptiveBootstrapCoordinator).attemptReader.(*adaptiveBootstrapAttemptReaderStub).runIDs)
	require.Empty(t, producer.requests)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorResumeRejectsTargetReplayDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*repository.CommitAdaptiveExecutionBootstrapResult)
	}{
		{name: "created at", mutate: func(target *repository.CommitAdaptiveExecutionBootstrapResult) {
			target.Decision.CreatedAt++
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			target := adaptiveBootstrapTypedResultForTest(t, run, input, attempt, 3)
			test.mutate(target)
			repo := &adaptiveBootstrapRepositoryStub{readResult: target}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
				Repository:    repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			require.Error(t, err)
			require.Nil(t, facts)
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorResumeInheritsTypedSource(t *testing.T) {
	for _, sourceKind := range []entity.AdaptiveAdmissionSource{
		entity.AdaptiveAdmissionSourceFresh,
		entity.AdaptiveAdmissionSourceTypedInheritance,
		entity.AdaptiveAdmissionSourceLegacyDecoder,
	} {
		t.Run(string(sourceKind), func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			source := adaptiveBootstrapSourceResultForTest(t, sourceKind)
			target := adaptiveBootstrapTypedResultForTest(t, run, input, attempt, source.Authority.ExecutionGeneration)
			repo := &adaptiveBootstrapRepositoryStub{
				readResults:  []*repository.CommitAdaptiveExecutionBootstrapResult{nil, source},
				readErrs:     []error{repository.ErrAdaptiveExecutionBootstrapNotFound, nil},
				commitResult: target,
			}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
				Repository:    repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			require.NoError(t, err)
			require.Len(t, repo.readRequests, 2)
			require.Equal(t, int64(21), repo.readRequests[0].ExecutionRunID)
			require.Equal(t, int64(20), repo.readRequests[1].ExecutionRunID)
			require.Equal(t, "attempt-1", repo.readRequests[1].AttemptID)
			require.Equal(t, []int{3}, ids.counts)
			require.Len(t, repo.commitRequests, 1)
			req := repo.commitRequests[0]
			require.Equal(t, entity.AdaptiveAdmissionSourceTypedInheritance, req.Admission.Source)
			require.Equal(t, input.SourceRunID, *req.Admission.SourceRunID)
			require.Equal(t, source.Authority.ExecutionGeneration, *req.Admission.SourceExecutionGeneration)
			require.Equal(t, source.Admission.Capabilities, req.Admission.Capabilities)
			require.Equal(t, source.Admission.Limits, req.Admission.Limits)
			require.Equal(t, run.RunID, req.Decision.ExecutionRunID)
			require.Equal(t, attempt.JournalRunID, req.Decision.JournalRunID)
			require.Equal(t, attempt.AttemptID, req.Decision.AttemptID)
			require.Equal(t, run.ExecutionGeneration, req.Decision.ExecutionGeneration)
			require.Equal(t, uint64(1), req.Decision.DecisionRevision)
			require.Equal(t, *source.Decision.PlanScopeRunID, *req.Decision.PlanScopeRunID)
			require.Equal(t, attempt.CreatedAt, req.Decision.CreatedAt)
			require.Equal(t, req.Admission, facts.Admission)
			require.Equal(t, req.Decision, facts.Decision)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorResumeCarriesTypedMultiHopPlanScope(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	source := adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSourceTypedInheritance)
	inheritedScope := int64(19)
	source.Decision.PlanScopeRunID = &inheritedScope
	target := adaptiveBootstrapTypedResultForTest(t, run, input, attempt, source.Authority.ExecutionGeneration)
	target.Decision.PlanScopeRunID = &inheritedScope
	repo := &adaptiveBootstrapRepositoryStub{
		readResults:  []*repository.CommitAdaptiveExecutionBootstrapResult{nil, source},
		readErrs:     []error{repository.ErrAdaptiveExecutionBootstrapNotFound, nil},
		commitResult: target,
	}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository:    repo,
		IDGen:         &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}},
		Now:           func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)

	require.NoError(t, err)
	require.Len(t, repo.commitRequests, 1)
	require.Equal(t, inheritedScope, *repo.commitRequests[0].Decision.PlanScopeRunID)
	require.Equal(t, inheritedScope, *facts.Decision.PlanScopeRunID)
}

func TestAdaptiveBootstrapCoordinatorResumeFallsBackToLegacyOnlyAfterSourceDurableMiss(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	source := &entity.Run{
		ID: input.SourceRunID, ThreadID: run.ThreadID, ExecutionGeneration: 3,
		Config: `{"runtime":"eino_adk","requested_policy":"pro"}`,
	}
	originalConfig := source.Config
	runReader := &adaptiveBootstrapSourceRunReaderStub{run: source}
	repo := &adaptiveBootstrapRepositoryStub{
		readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{nil, nil},
		readErrs: []error{
			repository.ErrAdaptiveExecutionBootstrapNotFound,
			repository.ErrAdaptiveExecutionBootstrapNotFound,
		},
	}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader:   &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository:      repo,
		SourceRunReader: runReader,
		IDGen:           &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}},
		Now:             func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)

	require.NoError(t, err)
	require.Equal(t, []int64{input.SourceRunID}, runReader.runIDs)
	require.Len(t, repo.commitRequests, 1)
	req := repo.commitRequests[0]
	require.Equal(t, entity.AdaptiveAdmissionSourceLegacyDecoder, req.Admission.Source)
	require.Equal(t, input.SourceRunID, requireInt64PointerForAdaptiveBootstrapTest(t, req.Admission.SourceRunID))
	require.Equal(t, source.ExecutionGeneration, requireUint64PointerForAdaptiveBootstrapTest(t, req.Admission.SourceExecutionGeneration))
	require.Equal(t, entity.AdaptiveLegacyDecoderVersionV1, req.Admission.DecoderVersion)
	require.Len(t, req.Admission.SourceConfigDigest, 64)
	require.False(t, req.Admission.FeatureGateEnabled)
	require.Equal(t, run.RunID, req.Decision.ExecutionRunID)
	require.Equal(t, run.ExecutionGeneration, req.Decision.ExecutionGeneration)
	require.Equal(t, entity.AdaptiveAdmissionSourceLegacyDecoder, facts.Admission.Source)
	require.Equal(t, req.Admission, facts.Admission)
	require.Equal(t, req.Decision, facts.Decision)
	require.Equal(t, originalConfig, source.Config)
}

func TestAdaptiveBootstrapCoordinatorResumeBareSourceSkipsDurableSourceRead(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	attempt.JournalRunID = input.SourceRunID
	attempt.Ordinal = 1
	attempt.SourceAttemptID = nil
	attempt.ProjectionState = entity.JournalProjectionStateDisabled
	source := &entity.Run{
		ID: input.SourceRunID, ThreadID: run.ThreadID, RunKind: entity.RunKindTask,
		ExecutionGeneration: 3, Config: `{"runtime":"eino_adk","requested_policy":"pro"}`,
	}
	runReader := &adaptiveBootstrapSourceRunReaderStub{run: source}
	repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader:   &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository:      repo,
		SourceRunReader: runReader,
		IDGen:           &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}},
		Now:             func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)

	require.NoError(t, err)
	require.NotNil(t, facts)
	require.Equal(t, []repository.ReadAdaptiveExecutionBootstrapRequest{{
		ThreadID: run.ThreadID, ExecutionRunID: run.RunID,
		JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
	}}, repo.readRequests)
	require.Equal(t, []int64{input.SourceRunID}, runReader.runIDs)
	require.Len(t, repo.commitRequests, 1)
	require.Equal(t, entity.AdaptiveAdmissionSourceLegacyDecoder, repo.commitRequests[0].Admission.Source)
	require.Equal(t, input.SourceRunID, requireInt64PointerForAdaptiveBootstrapTest(t, repo.commitRequests[0].Decision.PlanScopeRunID))
}

func TestAdaptiveBootstrapCoordinatorResumeRejectsInvalidBareSourceBeforeReads(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*entity.RunAttempt)
	}{
		{name: "healthy projection", mutate: func(attempt *entity.RunAttempt) {
			attempt.ProjectionState = entity.JournalProjectionStateHealthy
		}},
		{name: "missing checkpoint", mutate: func(attempt *entity.RunAttempt) {
			attempt.SourceCheckpointID = nil
		}},
		{name: "missing recovery key", mutate: func(attempt *entity.RunAttempt) {
			attempt.RecoveryIdempotencyKey = nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			attempt.JournalRunID = input.SourceRunID
			attempt.Ordinal = 1
			attempt.SourceAttemptID = nil
			attempt.ProjectionState = entity.JournalProjectionStateDisabled
			test.mutate(attempt)
			runReader := &adaptiveBootstrapSourceRunReaderStub{run: &entity.Run{ID: input.SourceRunID}}
			repo := &adaptiveBootstrapRepositoryStub{}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader:   &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
				Repository:      repo,
				SourceRunReader: runReader,
				IDGen:           ids,
				Now:             func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			require.Error(t, err)
			require.Nil(t, facts)
			require.Empty(t, repo.readRequests)
			require.Empty(t, runReader.runIDs)
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorResumeReplaysLegacyTargetWithoutReadingSource(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	target := adaptiveBootstrapLegacyTargetResultForTest(t, run, input, attempt)
	runReader := &adaptiveBootstrapSourceRunReaderStub{run: &entity.Run{ID: input.SourceRunID}}
	repo := &adaptiveBootstrapRepositoryStub{readResult: target}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader:   &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository:      repo,
		SourceRunReader: runReader,
		IDGen:           &adaptiveBootstrapIDGeneratorStub{},
		Now:             func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)

	require.NoError(t, err)
	require.Equal(t, target.Admission, facts.Admission)
	require.Equal(t, target.Decision, facts.Decision)
	require.Empty(t, runReader.runIDs)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorResumeReplaysBareLegacyTargetWithoutReadingSource(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	attempt.JournalRunID = input.SourceRunID
	attempt.Ordinal = 1
	attempt.SourceAttemptID = nil
	attempt.ProjectionState = entity.JournalProjectionStateDisabled
	target := adaptiveBootstrapLegacyTargetResultForTest(t, run, input, attempt)
	runReader := &adaptiveBootstrapSourceRunReaderStub{run: &entity.Run{ID: input.SourceRunID}}
	repo := &adaptiveBootstrapRepositoryStub{readResult: target}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader:   &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository:      repo,
		SourceRunReader: runReader,
		IDGen:           ids,
		Now:             func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)

	require.NoError(t, err)
	require.Equal(t, target.Admission, facts.Admission)
	require.Equal(t, target.Decision, facts.Decision)
	require.Len(t, repo.readRequests, 1)
	require.Equal(t, run.RunID, repo.readRequests[0].ExecutionRunID)
	require.Empty(t, runReader.runIDs)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorResumeDoesNotReadLegacyRunOnSourceDurableFailure(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	sentinel := errors.New("source durable read failed")
	runReader := &adaptiveBootstrapSourceRunReaderStub{run: &entity.Run{ID: input.SourceRunID}}
	repo := &adaptiveBootstrapRepositoryStub{
		readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{nil, nil},
		readErrs: []error{
			repository.ErrAdaptiveExecutionBootstrapNotFound,
			sentinel,
		},
	}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader:   &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository:      repo,
		SourceRunReader: runReader,
		IDGen:           &adaptiveBootstrapIDGeneratorStub{},
		Now:             func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)

	require.ErrorIs(t, err, sentinel)
	require.Nil(t, facts)
	require.Empty(t, runReader.runIDs)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapHumanResumeConsumesTypedLineageBeforeADKBuild(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	source := adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSourceFresh)
	order := make([]string, 0, 6)
	repo := &adaptiveBootstrapRepositoryStub{
		readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{nil, source},
		readErrs: []error{
			repository.ErrAdaptiveExecutionBootstrapNotFound,
			nil,
		},
		order: &order,
	}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt, order: &order},
		Repository:    repo,
		IDGen:         &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}},
		Now:           func() int64 { return 999 },
	})
	stopAfterBuild := errors.New("stop after typed lineage reaches adk build")
	executor := NewADKExecutor(
		ADKAgentFactoryFunc(func(ctx context.Context, _ *RunSummary) (adk.ResumableAgent, error) {
			order = append(order, "build-agent")
			facts, ok := adaptiveBootstrapFactsFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, entity.AdaptiveAdmissionSourceTypedInheritance, facts.Admission.Source)
			require.Equal(t, input.SourceRunID, requireInt64PointerForAdaptiveBootstrapTest(t, facts.Admission.SourceRunID))
			require.Equal(t, source.Authority.ExecutionGeneration, requireUint64PointerForAdaptiveBootstrapTest(t, facts.Admission.SourceExecutionGeneration))
			require.Equal(t, run.RunID, facts.Decision.ExecutionRunID)
			require.Equal(t, attempt.AttemptID, facts.Decision.AttemptID)
			return nil, stopAfterBuild
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			order = append(order, "build-store")
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
		WithADKAdaptiveBootstrapCoordinator(coordinator),
	)
	input.Runtime = RuntimeModeEinoADK
	input.RuntimeKey = "coze-run-21"
	input.ADKCheckpoint = &ADKCheckpointEnvelope{RuntimeKey: input.RuntimeKey}

	result, err := executor.Resume(context.Background(), run, input)

	require.Nil(t, result)
	require.ErrorIs(t, err, stopAfterBuild)
	require.Equal(t, []string{
		"read-target-attempt",
		"read-bootstrap-21",
		"read-bootstrap-20",
		"commit-bootstrap-21",
		"build-store",
		"build-agent",
	}, order)
	require.Len(t, repo.commitRequests, 1)
	require.Equal(t, attempt.AttemptID, repo.commitRequests[0].AttemptID)
	require.Equal(t, input.SourceRunID, *repo.commitRequests[0].Admission.SourceRunID)
}

func TestAdaptiveBootstrapCoordinatorResumeFailsClosed(t *testing.T) {
	sentinel := errors.New("source read failed")
	for _, test := range []struct {
		name       string
		mutate     func(*RunSummary, *HarnessResumeInput, *entity.RunAttempt)
		attemptErr error
		readResult *repository.CommitAdaptiveExecutionBootstrapResult
		readErr    error
		wantNoop   bool
	}{
		{name: "not enrolled", attemptErr: repository.ErrJournalNotEnrolled, wantNoop: true},
		{name: "partial lineage", mutate: func(_ *RunSummary, _ *HarnessResumeInput, attempt *entity.RunAttempt) {
			attempt.RecoveryIdempotencyKey = nil
		}},
		{name: "inactive slot", mutate: func(_ *RunSummary, _ *HarnessResumeInput, attempt *entity.RunAttempt) {
			value := uint8(2)
			attempt.ActiveSlot = &value
		}},
		{name: "checkpoint drift", mutate: func(_ *RunSummary, input *HarnessResumeInput, _ *entity.RunAttempt) {
			input.CheckpointID++
		}},
		{name: "source read error", readErr: sentinel},
		{name: "source not found", readErr: repository.ErrAdaptiveExecutionBootstrapNotFound},
		{name: "nil source"},
		{name: "legacy source", readResult: adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSource("legacy"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			if test.mutate != nil {
				test.mutate(run, input, attempt)
			}
			reader := &adaptiveBootstrapAttemptReaderStub{attempt: attempt, err: test.attemptErr}
			repo := &adaptiveBootstrapRepositoryStub{
				readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{nil, test.readResult},
				readErrs:    []error{repository.ErrAdaptiveExecutionBootstrapNotFound, test.readErr},
			}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: reader, Repository: repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			if test.wantNoop {
				require.NoError(t, err)
				require.Nil(t, facts)
				require.Empty(t, repo.readRequests)
			} else {
				require.Error(t, err)
			}
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorResumeRejectsTargetReadFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		result *repository.CommitAdaptiveExecutionBootstrapResult
		err    error
	}{
		{name: "nil target"},
		{name: "target conflict", err: repository.ErrAdaptiveExecutionBootstrapConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			repo := &adaptiveBootstrapRepositoryStub{readResult: test.result, readErr: test.err}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
				Repository:    repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			require.Error(t, err)
			require.Nil(t, facts)
			require.Len(t, repo.readRequests, 1)
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorResumeRejectsSourceAuthorityDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*repository.CommitAdaptiveExecutionBootstrapResult)
	}{
		{name: "thread", mutate: func(source *repository.CommitAdaptiveExecutionBootstrapResult) {
			source.Authority.ThreadID++
		}},
		{name: "journal", mutate: func(source *repository.CommitAdaptiveExecutionBootstrapResult) {
			source.Authority.JournalRunID++
			source.Decision.JournalRunID++
		}},
		{name: "attempt", mutate: func(source *repository.CommitAdaptiveExecutionBootstrapResult) {
			source.Authority.AttemptID = "wrong-attempt"
			source.Decision.AttemptID = "wrong-attempt"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			source := adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSourceFresh)
			test.mutate(source)
			repo := &adaptiveBootstrapRepositoryStub{
				readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{nil, source},
				readErrs:    []error{repository.ErrAdaptiveExecutionBootstrapNotFound, nil},
			}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
				Repository:    repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			require.Error(t, err)
			require.Nil(t, facts)
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorResumeRejectsSourcePairDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*repository.CommitAdaptiveExecutionBootstrapResult)
	}{
		{name: "revision", mutate: func(source *repository.CommitAdaptiveExecutionBootstrapResult) {
			source.Decision.DecisionRevision = 2
		}},
		{name: "fresh plan scope drift", mutate: func(source *repository.CommitAdaptiveExecutionBootstrapResult) {
			value := int64(999)
			source.Decision.PlanScopeRunID = &value
		}},
		{name: "multi step without plan scope", mutate: func(source *repository.CommitAdaptiveExecutionBootstrapResult) {
			source.Decision.PlanScopeRunID = nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			source := adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSourceFresh)
			test.mutate(source)
			repo := &adaptiveBootstrapRepositoryStub{
				readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{nil, source},
				readErrs:    []error{repository.ErrAdaptiveExecutionBootstrapNotFound, nil},
			}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
				Repository:    repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			require.Error(t, err)
			require.Nil(t, facts)
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func TestAdaptiveBootstrapCoordinatorResumeRejectsCommitFailure(t *testing.T) {
	commitErr := errors.New("commit failed")
	for _, test := range []struct {
		name                  string
		commitErr             error
		returnNilCommitResult bool
	}{
		{name: "error", commitErr: commitErr},
		{name: "nil result", returnNilCommitResult: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
			source := adaptiveBootstrapSourceResultForTest(t, entity.AdaptiveAdmissionSourceFresh)
			repo := &adaptiveBootstrapRepositoryStub{
				readResults:           []*repository.CommitAdaptiveExecutionBootstrapResult{nil, source},
				readErrs:              []error{repository.ErrAdaptiveExecutionBootstrapNotFound, nil},
				commitErr:             test.commitErr,
				returnNilCommitResult: test.returnNilCommitResult,
			}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
				Repository:    repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			facts, err := coordinator.BootstrapResume(context.Background(), run, input)

			require.Error(t, err)
			require.Nil(t, facts)
			require.Equal(t, []int{3}, ids.counts)
			require.Len(t, repo.commitRequests, 1)
		})
	}
}

func adaptiveBootstrapRecoveryResumeForTest() (*RunSummary, *HarnessResumeInput, *entity.RunAttempt) {
	sourceAttemptID := "attempt-1"
	sourceCheckpointID := int64(9001)
	recoveryKey := "recover-2"
	active := uint8(1)
	return &RunSummary{
			ThreadID: 10, RunID: 21, RunKind: RunKindTask, Status: RunStatusRunning,
			LeaseOwner: "worker-2", LeaseToken: "lease-2", ExecutionGeneration: 4,
		}, &HarnessResumeInput{
			ThreadID: 10, RunID: 21, SourceRunID: 20, CheckpointID: 9001,
		}, &entity.RunAttempt{
			ThreadID: 10, JournalRunID: 30, ExecutionRunID: 21, AttemptID: "attempt-2",
			Status: entity.RunAttemptStatusRunning, ActiveSlot: &active, CreatedAt: 800,
			SourceAttemptID: &sourceAttemptID, SourceCheckpointID: &sourceCheckpointID,
			RecoveryIdempotencyKey: &recoveryKey,
		}
}

func adaptiveBootstrapTypedResultForTest(
	t *testing.T,
	run *RunSummary,
	input *HarnessResumeInput,
	attempt *entity.RunAttempt,
	sourceGeneration uint64,
) *repository.CommitAdaptiveExecutionBootstrapResult {
	t.Helper()
	sourceRunID := input.SourceRunID
	admission := baselineAdaptiveAdmission()
	admission.Source = entity.AdaptiveAdmissionSourceTypedInheritance
	admission.SourceRunID = &sourceRunID
	admission.SourceExecutionGeneration = &sourceGeneration
	decision, err := (BaselineDecisionProducer{}).Produce(BaselineDecisionRequest{
		Admission: admission, DecisionID: adaptiveBootstrapStableKeyForTest("decision", run, attempt),
		DecisionRevision: 1, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
		AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		PlanScopeRunID: input.SourceRunID, CreatedAt: attempt.CreatedAt,
	})
	require.NoError(t, err)
	return &repository.CommitAdaptiveExecutionBootstrapResult{
		Admission: admission,
		Decision:  decision,
		Authority: repository.AdaptiveExecutionBootstrapAuthority{
			ThreadID: run.ThreadID, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
			AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		},
	}
}

func adaptiveBootstrapLegacyTargetResultForTest(
	t *testing.T,
	run *RunSummary,
	input *HarnessResumeInput,
	attempt *entity.RunAttempt,
) *repository.CommitAdaptiveExecutionBootstrapResult {
	t.Helper()
	admission, err := NewLegacyAdaptiveAdmissionDecoder().Decode(&entity.Run{
		ID: input.SourceRunID, ThreadID: run.ThreadID, ExecutionGeneration: 3,
		Config: `{"requested_policy":"pro"}`,
	})
	require.NoError(t, err)
	decision, err := (BaselineDecisionProducer{}).Produce(BaselineDecisionRequest{
		Admission: admission, DecisionID: adaptiveBootstrapStableKeyForTest("decision", run, attempt),
		DecisionRevision: 1, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
		AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		PlanScopeRunID: input.SourceRunID, CreatedAt: attempt.CreatedAt,
	})
	require.NoError(t, err)
	return &repository.CommitAdaptiveExecutionBootstrapResult{
		Admission: admission,
		Decision:  decision,
		Authority: repository.AdaptiveExecutionBootstrapAuthority{
			ThreadID: run.ThreadID, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
			AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		},
	}
}

func adaptiveBootstrapSourceResultForTest(
	t *testing.T,
	source entity.AdaptiveAdmissionSource,
) *repository.CommitAdaptiveExecutionBootstrapResult {
	t.Helper()
	run := freshAdaptiveBootstrapRunForTest()
	run.ExecutionGeneration = 3
	attempt := freshAdaptiveBootstrapAttemptForTest()
	admission := baselineAdaptiveAdmission()
	admission.Source = source
	if source == entity.AdaptiveAdmissionSourceTypedInheritance || source == entity.AdaptiveAdmissionSourceLegacyDecoder {
		sourceRunID, sourceGeneration := int64(19), uint64(2)
		admission.SourceRunID = &sourceRunID
		admission.SourceExecutionGeneration = &sourceGeneration
	}
	if source == entity.AdaptiveAdmissionSourceLegacyDecoder {
		admission.SourceConfigDigest = strings.Repeat("a", 64)
		admission.DecoderVersion = entity.AdaptiveLegacyDecoderVersionV1
	}
	decision, err := (BaselineDecisionProducer{}).Produce(BaselineDecisionRequest{
		Admission: admission, DecisionID: adaptiveBootstrapStableKeyForTest("decision", run, attempt),
		DecisionRevision: 1, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
		AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		PlanScopeRunID: run.RunID, CreatedAt: attempt.CreatedAt,
	})
	if source == entity.AdaptiveAdmissionSource("legacy") {
		require.Error(t, err)
		decision = entity.ExecutionDecision{}
	} else {
		require.NoError(t, err)
	}
	return &repository.CommitAdaptiveExecutionBootstrapResult{
		Admission: admission,
		Decision:  decision,
		Authority: repository.AdaptiveExecutionBootstrapAuthority{
			ThreadID: run.ThreadID, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
			AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		},
	}
}

func TestAdaptiveBootstrapCoordinatorRejectsReplayFromAnotherGeneration(t *testing.T) {
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: freshAdaptiveBootstrapAttemptForTest()}
	repo := &adaptiveBootstrapRepositoryStub{readResult: &repository.CommitAdaptiveExecutionBootstrapResult{
		Authority: repository.AdaptiveExecutionBootstrapAuthority{ExecutionGeneration: 3},
	}}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader,
		Repository:    repo,
		IDGen:         ids,
		Now:           func() int64 { return 999 },
	})

	_, err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())

	require.ErrorContains(t, err, "generation does not match")
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorSkipsNotEnrolledRun(t *testing.T) {
	reader := &adaptiveBootstrapAttemptReaderStub{err: repository.ErrJournalNotEnrolled}
	repo := &adaptiveBootstrapRepositoryStub{}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader, Repository: repo, IDGen: ids, Now: func() int64 { return 999 },
	})

	run := freshAdaptiveBootstrapRunForTest()
	run.ExecutionGeneration = 0
	run.LeaseOwner = ""
	run.LeaseToken = ""
	facts, err := coordinator.Bootstrap(context.Background(), run)

	require.NoError(t, err)
	require.Nil(t, facts)
	require.Equal(t, 1, reader.calls)
	require.Empty(t, repo.readRequests)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorRejectsUnsafeFreshAttemptBeforeWriting(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*RunSummary, *entity.RunAttempt)
	}{
		{
			name: "source attempt lineage",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) {
				value := "source-attempt"
				attempt.SourceAttemptID = &value
			},
		},
		{
			name: "source checkpoint lineage",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) {
				value := int64(91)
				attempt.SourceCheckpointID = &value
			},
		},
		{
			name: "recovery lineage",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) {
				value := "recover-1"
				attempt.RecoveryIdempotencyKey = &value
			},
		},
		{
			name:   "thread ownership",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) { attempt.ThreadID++ },
		},
		{
			name:   "execution ownership",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) { attempt.ExecutionRunID++ },
		},
		{
			name: "inactive attempt",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) {
				attempt.Status = entity.RunAttemptStatusCompleted
			},
		},
		{
			name:   "missing lease",
			mutate: func(run *RunSummary, _ *entity.RunAttempt) { run.LeaseToken = "" },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := freshAdaptiveBootstrapRunForTest()
			attempt := freshAdaptiveBootstrapAttemptForTest()
			test.mutate(run, attempt)
			reader := &adaptiveBootstrapAttemptReaderStub{attempt: attempt}
			repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: reader, Repository: repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			_, err := coordinator.Bootstrap(context.Background(), run)

			require.Error(t, err)
			require.Empty(t, repo.readRequests)
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func freshAdaptiveBootstrapRunForTest() *RunSummary {
	return &RunSummary{
		ThreadID: 10, RunID: 20, RunKind: RunKindTask, Status: RunStatusRunning,
		LeaseOwner: "worker-1", LeaseToken: "lease-1", ExecutionGeneration: 4,
		Input: `{"messages":[{"role":"user","content":"execute the submitted task"}]}`,
	}
}

func freshAdaptiveBootstrapAttemptForTest() *entity.RunAttempt {
	active := uint8(1)
	return &entity.RunAttempt{
		ThreadID: 10, JournalRunID: 30, ExecutionRunID: 20, AttemptID: "attempt-1",
		Status: entity.RunAttemptStatusRunning, ActiveSlot: &active, CreatedAt: 700,
	}
}

func adaptiveBootstrapStableKeyForTest(
	kind string,
	run *RunSummary,
	attempt *entity.RunAttempt,
) string {
	input := strings.Join([]string{
		"workbench-adaptive-bootstrap.v1", kind,
		strconv.FormatInt(run.ThreadID, 10), strconv.FormatInt(run.RunID, 10),
		strconv.FormatInt(attempt.JournalRunID, 10), attempt.AttemptID,
		strconv.FormatUint(run.ExecutionGeneration, 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(input))
	return "adaptive-" + kind + ":" + hex.EncodeToString(digest[:])
}

func readerForAdaptiveAttempt(attempt *entity.RunAttempt) AdaptiveBootstrapAttemptReader {
	return &adaptiveBootstrapAttemptReaderStub{attempt: attempt}
}

type adaptiveEligibilityResolverStub struct {
	admission entity.AdaptiveAdmissionSnapshot
	err       error
	requests  []AdaptiveEligibilityRequest
}

func (s *adaptiveEligibilityResolverStub) Resolve(
	_ context.Context,
	request AdaptiveEligibilityRequest,
) (entity.AdaptiveAdmissionSnapshot, error) {
	s.requests = append(s.requests, request)
	return s.admission, s.err
}

func gateOnEligibilityResolverForTest() AdaptiveEligibilityResolver {
	admission := baselineAdaptiveAdmission()
	admission.FeatureGateEnabled = true
	return &adaptiveEligibilityResolverStub{admission: admission}
}

type adaptiveDecisionProducerStub struct {
	candidate       AdaptiveDecisionCandidate
	err             error
	requests        []AdaptiveDecisionRequest
	contexts        []context.Context
	mutateAdmission func(entity.AdaptiveAdmissionSnapshot)
}

func (s *adaptiveDecisionProducerStub) Produce(
	ctx context.Context,
	request AdaptiveDecisionRequest,
) (AdaptiveDecisionCandidate, error) {
	s.contexts = append(s.contexts, ctx)
	s.requests = append(s.requests, request)
	if s.mutateAdmission != nil {
		s.mutateAdmission(request.Admission)
	}
	return s.candidate, s.err
}

func adaptiveDecisionCandidateForTest(gateOn bool) AdaptiveDecisionCandidate {
	admission := baselineAdaptiveAdmission()
	admission.FeatureGateEnabled = gateOn
	request := AdaptiveDecisionRequest{Admission: admission}
	producer := AdaptiveDecisionProducer(BaselineAdaptiveDecisionProducer{})
	if gateOn {
		producer = DeterministicAdaptiveDecisionProducer{}
	}
	candidate, err := producer.Produce(context.Background(), request)
	if err != nil {
		panic(err)
	}
	return candidate
}

func adaptiveDecisionForBootstrapTest(
	admission entity.AdaptiveAdmissionSnapshot,
	authority entity.ExecutionDecision,
) entity.ExecutionDecision {
	candidate, err := (DeterministicAdaptiveDecisionProducer{}).Produce(
		context.Background(), AdaptiveDecisionRequest{Admission: admission},
	)
	if err != nil {
		panic(err)
	}
	return entity.ExecutionDecision{
		Schema:     entity.ExecutionDecisionSchemaV1,
		DecisionID: authority.DecisionID, DecisionRevision: authority.DecisionRevision,
		ExecutionRunID: authority.ExecutionRunID, JournalRunID: authority.JournalRunID,
		AttemptID: authority.AttemptID, ExecutionGeneration: authority.ExecutionGeneration,
		PlanScopeRunID: authority.PlanScopeRunID,
		GoalSummary:    candidate.GoalSummary, Deliverables: candidate.Deliverables,
		AcceptanceChecks: candidate.AcceptanceChecks, Decision: candidate.Decision,
		ExecutionShape: candidate.ExecutionShape, ClarificationQuestion: candidate.ClarificationQuestion,
		SafeSummary: candidate.SafeSummary, CreatedAt: authority.CreatedAt,
	}
}

type adaptiveBootstrapAttemptReaderStub struct {
	attempt *entity.RunAttempt
	err     error
	calls   int
	runIDs  []int64
	order   *[]string
}

type adaptiveBootstrapSourceRunReaderStub struct {
	run    *entity.Run
	err    error
	runIDs []int64
}

func (s *adaptiveBootstrapSourceRunReaderStub) GetRun(
	_ context.Context,
	req *domainservice.GetRunRequest,
) (*entity.Run, error) {
	if req != nil {
		s.runIDs = append(s.runIDs, req.RunID)
	}
	return s.run, s.err
}

func (s *adaptiveBootstrapAttemptReaderStub) GetActiveJournalAttempt(
	_ context.Context,
	runID int64,
) (*entity.RunAttempt, error) {
	s.calls++
	s.runIDs = append(s.runIDs, runID)
	if s.order != nil {
		*s.order = append(*s.order, "read-target-attempt")
	}
	return s.attempt, s.err
}

type adaptiveBootstrapRepositoryStub struct {
	readResult            *repository.CommitAdaptiveExecutionBootstrapResult
	readErr               error
	readResults           []*repository.CommitAdaptiveExecutionBootstrapResult
	readErrs              []error
	commitResult          *repository.CommitAdaptiveExecutionBootstrapResult
	commitErr             error
	returnNilCommitResult bool
	readRequests          []repository.ReadAdaptiveExecutionBootstrapRequest
	commitRequests        []repository.CommitAdaptiveExecutionBootstrapRequest
	order                 *[]string
}

func (s *adaptiveBootstrapRepositoryStub) ReadAdaptiveExecutionBootstrap(
	_ context.Context,
	req repository.ReadAdaptiveExecutionBootstrapRequest,
) (*repository.CommitAdaptiveExecutionBootstrapResult, error) {
	s.readRequests = append(s.readRequests, req)
	if s.order != nil {
		*s.order = append(*s.order, "read-bootstrap-"+strconv.FormatInt(req.ExecutionRunID, 10))
	}
	index := len(s.readRequests) - 1
	if s.readResults != nil || s.readErrs != nil {
		var result *repository.CommitAdaptiveExecutionBootstrapResult
		var err error
		if index < len(s.readResults) {
			result = s.readResults[index]
		}
		if index < len(s.readErrs) {
			err = s.readErrs[index]
		}
		if index >= len(s.readResults) && index >= len(s.readErrs) {
			return nil, fmt.Errorf("unexpected adaptive bootstrap read %d", index+1)
		}
		return result, err
	}
	return s.readResult, s.readErr
}

func (s *adaptiveBootstrapRepositoryStub) CommitAdaptiveExecutionBootstrap(
	_ context.Context,
	req repository.CommitAdaptiveExecutionBootstrapRequest,
) (*repository.CommitAdaptiveExecutionBootstrapResult, error) {
	s.commitRequests = append(s.commitRequests, req)
	if s.order != nil {
		*s.order = append(*s.order, "commit-bootstrap-"+strconv.FormatInt(req.ExecutionRunID, 10))
	}
	if s.commitResult == nil && s.commitErr == nil && !s.returnNilCommitResult {
		s.commitResult = &repository.CommitAdaptiveExecutionBootstrapResult{
			Admission: req.Admission,
			Decision:  req.Decision,
			Authority: repository.AdaptiveExecutionBootstrapAuthority{
				ThreadID: req.ThreadID, ExecutionRunID: req.ExecutionRunID,
				JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
				ExecutionGeneration: req.Generation,
			},
		}
	}
	return s.commitResult, s.commitErr
}

func requireInt64PointerForAdaptiveBootstrapTest(t *testing.T, value *int64) int64 {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireUint64PointerForAdaptiveBootstrapTest(t *testing.T, value *uint64) uint64 {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

type adaptiveBootstrapIDGeneratorStub struct {
	ids    []int64
	err    error
	counts []int
}

func (s *adaptiveBootstrapIDGeneratorStub) GenMultiIDs(
	_ context.Context,
	counts int,
) ([]int64, error) {
	s.counts = append(s.counts, counts)
	return append([]int64(nil), s.ids...), s.err
}

func TestAdaptiveBootstrapCoordinatorSurfacesPreReadFailure(t *testing.T) {
	readErr := errors.New("read failed")
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: freshAdaptiveBootstrapAttemptForTest()}
	repo := &adaptiveBootstrapRepositoryStub{readErr: readErr}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader, Repository: repo, IDGen: &adaptiveBootstrapIDGeneratorStub{}, Now: func() int64 { return 999 },
	})

	_, err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())

	require.ErrorIs(t, err, readErr)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorRejectsMissingCommitResult(t *testing.T) {
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: freshAdaptiveBootstrapAttemptForTest()}
	repo := &adaptiveBootstrapRepositoryStub{
		readErr:               repository.ErrAdaptiveExecutionBootstrapNotFound,
		returnNilCommitResult: true,
	}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader,
		Repository:    repo,
		IDGen:         &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}},
		Now:           func() int64 { return 999 },
	})

	_, err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())

	require.ErrorContains(t, err, "commit result is required")
	require.Len(t, repo.commitRequests, 1)
}

func TestAdaptiveBootstrapCoordinatorFuncRejectsNilFunction(t *testing.T) {
	var coordinator AdaptiveBootstrapCoordinator = AdaptiveBootstrapCoordinatorFunc(nil)

	require.NotPanics(t, func() {
		_, err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())
		require.ErrorContains(t, err, "coordinator function is required")
	})
}

func TestAdaptiveBootstrapCoordinatorRejectsInvalidDurableFacts(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	attempt := freshAdaptiveBootstrapAttemptForTest()
	result := adaptiveBootstrapResultForTest(t, run, attempt)
	result.Decision.ExecutionRunID++
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository:    &adaptiveBootstrapRepositoryStub{readResult: result},
		IDGen:         &adaptiveBootstrapIDGeneratorStub{},
		Now:           func() int64 { return 999 },
	})

	facts, err := coordinator.Bootstrap(context.Background(), run)

	require.Nil(t, facts)
	require.ErrorContains(t, err, "durable facts")
}

func adaptiveBootstrapResultForTest(
	t *testing.T,
	run *RunSummary,
	attempt *entity.RunAttempt,
) *repository.CommitAdaptiveExecutionBootstrapResult {
	t.Helper()
	admission := baselineAdaptiveAdmission()
	decision, err := (BaselineDecisionProducer{}).Produce(BaselineDecisionRequest{
		Admission: admission, DecisionID: adaptiveBootstrapStableKeyForTest("decision", run, attempt),
		DecisionRevision: 1, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
		AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		PlanScopeRunID: run.RunID, CreatedAt: attempt.CreatedAt,
	})
	require.NoError(t, err)
	return &repository.CommitAdaptiveExecutionBootstrapResult{
		Admission: admission,
		Decision:  decision,
		Authority: repository.AdaptiveExecutionBootstrapAuthority{
			ThreadID: run.ThreadID, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
			AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		},
	}
}

func adaptiveBootstrapFactsForRunTest(t *testing.T, run *RunSummary) *AdaptiveBootstrapFacts {
	t.Helper()
	result := adaptiveBootstrapResultForTest(t, run, freshAdaptiveBootstrapAttemptForTest())
	return &AdaptiveBootstrapFacts{Admission: result.Admission, Decision: result.Decision}
}

func ExampleAdaptiveBootstrapCoordinator_stableIdentity() {
	run := freshAdaptiveBootstrapRunForTest()
	attempt := freshAdaptiveBootstrapAttemptForTest()
	fmt.Println(adaptiveBootstrapStableKeyForTest("operation", run, attempt) == adaptiveBootstrapStableKeyForTest("operation", run, attempt))
	// Output: true
}
