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
	"errors"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
)

func validAdmission(source entity.AdaptiveAdmissionSource) entity.AdaptiveAdmissionSnapshot {
	snapshot := entity.AdaptiveAdmissionSnapshot{
		Schema:             entity.AdaptiveAdmissionSchemaV1,
		FeatureGateEnabled: true,
		Source:             source,
		Capabilities: entity.AdaptiveCapabilities{
			PlanAllowed: true, HumanInteractionAllowed: true,
		},
		Limits: entity.AdaptiveLimits{
			MaxToolCalls: 24, MaxReplans: 2, MaxVerificationRepairs: 2,
			MaxConsecutiveNoProgress: 3, MaxActiveDurationSeconds: 1200,
		},
	}
	if source != entity.AdaptiveAdmissionSourceFresh {
		runID, generation := int64(4), uint64(2)
		snapshot.SourceRunID, snapshot.SourceExecutionGeneration = &runID, &generation
	}
	if source == entity.AdaptiveAdmissionSourceLegacyDecoder {
		snapshot.FeatureGateEnabled = false
		snapshot.SourceConfigDigest = strings.Repeat("a", 64)
		snapshot.DecoderVersion = entity.AdaptiveLegacyDecoderVersionV1
	}
	return snapshot
}

func TestValidateAdaptiveAdmissionSnapshotAcceptsThreeSourceForms(t *testing.T) {
	for _, source := range []entity.AdaptiveAdmissionSource{
		entity.AdaptiveAdmissionSourceFresh,
		entity.AdaptiveAdmissionSourceTypedInheritance,
		entity.AdaptiveAdmissionSourceLegacyDecoder,
	} {
		require.NoError(t, ValidateAdaptiveAdmissionSnapshot(validAdmission(source)), source)
	}
}

func TestValidateAdaptiveAdmissionSnapshotRejectsInvalidLineageAndLimits(t *testing.T) {
	cases := []entity.AdaptiveAdmissionSnapshot{
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceFresh)
			v.Schema = "wrong"
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceFresh)
			id := int64(1)
			v.SourceRunID = &id
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceTypedInheritance)
			v.SourceRunID = int64Pointer(0)
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceTypedInheritance)
			v.SourceExecutionGeneration = uint64Pointer(0)
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceTypedInheritance)
			v.SourceConfigDigest = strings.Repeat("a", 64)
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)
			v.FeatureGateEnabled = true
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)
			v.SourceRunID = int64Pointer(0)
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)
			v.SourceExecutionGeneration = uint64Pointer(0)
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)
			v.SourceConfigDigest = strings.Repeat("a", 63)
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)
			v.SourceConfigDigest = strings.Repeat("A", 64)
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)
			v.DecoderVersion = "v2"
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceFresh)
			v.Source = "unknown"
			return v
		}(),
		func() entity.AdaptiveAdmissionSnapshot {
			v := validAdmission(entity.AdaptiveAdmissionSourceFresh)
			v.Capabilities.SubagentsAllowed = true
			return v
		}(),
	}
	for _, mutate := range []func(*entity.AdaptiveLimits){
		func(l *entity.AdaptiveLimits) { l.MaxToolCalls = 0 },
		func(l *entity.AdaptiveLimits) { l.MaxReplans = 0 },
		func(l *entity.AdaptiveLimits) { l.MaxVerificationRepairs = 0 },
		func(l *entity.AdaptiveLimits) { l.MaxConsecutiveNoProgress = 0 },
		func(l *entity.AdaptiveLimits) { l.MaxActiveDurationSeconds = 0 },
		func(l *entity.AdaptiveLimits) { l.MaxToolCalls = 25 },
		func(l *entity.AdaptiveLimits) { l.MaxReplans = 3 },
		func(l *entity.AdaptiveLimits) { l.MaxVerificationRepairs = 3 },
		func(l *entity.AdaptiveLimits) { l.MaxConsecutiveNoProgress = 4 },
		func(l *entity.AdaptiveLimits) { l.MaxActiveDurationSeconds = 1201 },
	} {
		v := validAdmission(entity.AdaptiveAdmissionSourceFresh)
		mutate(&v.Limits)
		cases = append(cases, v)
	}
	for _, snapshot := range cases {
		require.ErrorIs(t, ValidateAdaptiveAdmissionSnapshot(snapshot), ErrAdaptiveAdmissionInvalid)
	}
}

func int64Pointer(value int64) *int64 { return &value }

func uint64Pointer(value uint64) *uint64 { return &value }

func validDecision(kind entity.ExecutionDecisionKind, shape entity.ExecutionShape) entity.ExecutionDecision {
	decision := entity.ExecutionDecision{
		Schema: entity.ExecutionDecisionSchemaV1, DecisionID: "decision-1", DecisionRevision: 1,
		ExecutionRunID: 10, JournalRunID: 11, AttemptID: "attempt-1", ExecutionGeneration: 1,
		GoalSummary: "deliver goal", Deliverables: []string{"deliverable"},
		AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{{CheckID: "check-1", Kind: "assertion", TargetRef: "artifact-1", SafeDescription: "verify output"}},
		Decision:         kind, ExecutionShape: shape, SafeSummary: "safe summary", CreatedAt: 1,
	}
	if kind == entity.ExecutionDecisionClarification {
		question := "please clarify"
		decision.ClarificationQuestion = &question
	}
	if kind == entity.ExecutionDecisionExecute && shape == entity.ExecutionShapeMultiStep {
		planID := int64(12)
		decision.PlanScopeRunID = &planID
	}
	return decision
}

func TestValidateExecutionDecisionAcceptsAllDecisionForms(t *testing.T) {
	cases := map[string]entity.ExecutionDecision{
		"clarification": validDecision(entity.ExecutionDecisionClarification, entity.ExecutionShapeEmpty),
		"direct":        validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty),
		"single_step":   validDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeSingleStep),
		"multi_step":    validDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeMultiStep),
		"empty_slices": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.Deliverables = []string{}
			v.AcceptanceChecks = []entity.AdaptiveAcceptanceCheck{}
			return v
		}(),
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, ValidateExecutionDecision(testCase))
		})
	}
}

func TestValidateExecutionDecisionRejectsXORAndBudgetViolations(t *testing.T) {
	cases := map[string]entity.ExecutionDecision{
		"direct_with_question": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			q := "no"
			v.ClarificationQuestion = &q
			return v
		}(),
		"single_step_with_plan": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeSingleStep)
			id := int64(1)
			v.PlanScopeRunID = &id
			return v
		}(),
		"multi_step_without_plan": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeMultiStep)
			v.PlanScopeRunID = nil
			return v
		}(),
		"multi_step_with_zero_plan": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeMultiStep)
			v.PlanScopeRunID = int64Pointer(0)
			return v
		}(),
		"clarification_with_empty_question": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionClarification, entity.ExecutionShapeEmpty)
			empty := ""
			v.ClarificationQuestion = &empty
			return v
		}(),
		"nil_deliverables": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.Deliverables = nil
			return v
		}(),
		"nil_acceptance_checks": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.AcceptanceChecks = nil
			return v
		}(),
		"invalid_schema": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.Schema = "wrong"
			return v
		}(),
		"decision_id_over_budget": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.DecisionID = strings.Repeat("d", 192)
			return v
		}(),
		"empty_decision_id": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.DecisionID = ""
			return v
		}(),
		"zero_revision": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.DecisionRevision = 0
			return v
		}(),
		"zero_execution_run_id": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.ExecutionRunID = 0
			return v
		}(),
		"zero_journal_run_id": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.JournalRunID = 0
			return v
		}(),
		"empty_attempt_id": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.AttemptID = ""
			return v
		}(),
		"zero_execution_generation": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.ExecutionGeneration = 0
			return v
		}(),
		"empty_goal_summary": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.GoalSummary = ""
			return v
		}(),
		"empty_safe_summary": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.SafeSummary = ""
			return v
		}(),
		"zero_created_at": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.CreatedAt = 0
			return v
		}(),
		"too_many_deliverables": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.Deliverables = make([]string, 17)
			for i := range v.Deliverables {
				v.Deliverables[i] = "d"
			}
			return v
		}(),
		"too_many_acceptance_checks": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.AcceptanceChecks = make([]entity.AdaptiveAcceptanceCheck, 33)
			for i := range v.AcceptanceChecks {
				v.AcceptanceChecks[i] = entity.AdaptiveAcceptanceCheck{CheckID: "id", Kind: "kind", TargetRef: "target", SafeDescription: "safe"}
			}
			return v
		}(),
		"empty_deliverable": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.Deliverables = []string{""}
			return v
		}(),
		"empty_acceptance_check": func() entity.ExecutionDecision {
			v := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			v.AcceptanceChecks = []entity.AdaptiveAcceptanceCheck{{}}
			return v
		}(),
	}
	for name, decision := range cases {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, ValidateExecutionDecision(decision), ErrExecutionDecisionInvalid)
		})
	}
}

func TestValidateExecutionDecisionEnforcesAllStringByteCaps(t *testing.T) {
	mutators := map[string]func(*entity.ExecutionDecision){
		"decision_id": func(v *entity.ExecutionDecision) { v.DecisionID = strings.Repeat("x", 192) },
		"attempt_id":  func(v *entity.ExecutionDecision) { v.AttemptID = strings.Repeat("x", 192) },
		"goal_summary": func(v *entity.ExecutionDecision) {
			v.GoalSummary = strings.Repeat("x", 1025)
		},
		"safe_summary": func(v *entity.ExecutionDecision) {
			v.SafeSummary = strings.Repeat("x", 1025)
		},
		"clarification_question": func(v *entity.ExecutionDecision) {
			clarification := validDecision(entity.ExecutionDecisionClarification, entity.ExecutionShapeEmpty)
			question := strings.Repeat("x", 1025)
			clarification.ClarificationQuestion = &question
			*v = clarification
		},
		"deliverable": func(v *entity.ExecutionDecision) {
			v.Deliverables = []string{strings.Repeat("x", 513)}
		},
		"check_id": func(v *entity.ExecutionDecision) {
			v.AcceptanceChecks[0].CheckID = strings.Repeat("x", 192)
		},
		"check_kind": func(v *entity.ExecutionDecision) {
			v.AcceptanceChecks[0].Kind = strings.Repeat("x", 65)
		},
		"check_target_ref": func(v *entity.ExecutionDecision) {
			v.AcceptanceChecks[0].TargetRef = strings.Repeat("x", 192)
		},
		"check_description": func(v *entity.ExecutionDecision) {
			v.AcceptanceChecks[0].SafeDescription = strings.Repeat("x", 513)
		},
	}
	for name, mutate := range mutators {
		decision := validDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
		t.Run(name, func(t *testing.T) {
			mutate(&decision)
			require.ErrorIs(t, ValidateExecutionDecision(decision), ErrExecutionDecisionInvalid)
		})
	}
}

func TestValidateExecutionDecisionAgainstAdmissionBlocksCapabilitiesWithoutMutation(t *testing.T) {
	decision := validDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeMultiStep)
	admission := validAdmission(entity.AdaptiveAdmissionSourceFresh)
	admission.Capabilities.PlanAllowed = false
	before := decision
	before.Deliverables = append(before.Deliverables[:0:0], decision.Deliverables...)
	before.AcceptanceChecks = append(before.AcceptanceChecks[:0:0], decision.AcceptanceChecks...)
	if decision.PlanScopeRunID != nil {
		planScopeRunID := *decision.PlanScopeRunID
		before.PlanScopeRunID = &planScopeRunID
	}
	if decision.ClarificationQuestion != nil {
		clarificationQuestion := *decision.ClarificationQuestion
		before.ClarificationQuestion = &clarificationQuestion
	}
	err := ValidateExecutionDecisionAgainstAdmission(admission, decision)
	require.ErrorIs(t, err, ErrAdaptiveDecisionBlockedPolicy)
	require.True(t, errors.Is(err, ErrAdaptiveDecisionBlockedPolicy))
	require.Equal(t, before, decision)

	decision = validDecision(entity.ExecutionDecisionClarification, entity.ExecutionShapeEmpty)
	admission = validAdmission(entity.AdaptiveAdmissionSourceFresh)
	admission.Capabilities.HumanInteractionAllowed = false
	require.ErrorIs(t, ValidateExecutionDecisionAgainstAdmission(admission, decision), ErrAdaptiveDecisionBlockedPolicy)
}

func TestValidateExecutionDecisionAgainstAdmissionReturnsDecisionValidationBeforePolicy(t *testing.T) {
	admission := validAdmission(entity.AdaptiveAdmissionSourceFresh)
	admission.Capabilities.PlanAllowed = false
	decision := validDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeMultiStep)
	decision.DecisionID = ""
	require.ErrorIs(t, ValidateExecutionDecisionAgainstAdmission(admission, decision), ErrExecutionDecisionInvalid)
}
