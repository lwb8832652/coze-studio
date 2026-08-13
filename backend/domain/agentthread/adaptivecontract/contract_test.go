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

package adaptivecontract

import (
	"errors"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func contractAdmission(source entity.AdaptiveAdmissionSource) entity.AdaptiveAdmissionSnapshot {
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
		snapshot.SourceRunID = &runID
		snapshot.SourceExecutionGeneration = &generation
	}
	if source == entity.AdaptiveAdmissionSourceLegacyDecoder {
		snapshot.FeatureGateEnabled = false
		snapshot.SourceConfigDigest = strings.Repeat("a", 64)
		snapshot.DecoderVersion = entity.AdaptiveLegacyDecoderVersionV1
	}
	return snapshot
}

func contractDecision(kind entity.ExecutionDecisionKind, shape entity.ExecutionShape) entity.ExecutionDecision {
	decision := entity.ExecutionDecision{
		Schema: entity.ExecutionDecisionSchemaV1, DecisionID: "decision-1", DecisionRevision: 1,
		ExecutionRunID: 10, JournalRunID: 11, AttemptID: "attempt-1", ExecutionGeneration: 1,
		GoalSummary: "deliver goal", Deliverables: []string{"deliverable"},
		AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{{
			CheckID: "check-1", Kind: "assertion", TargetRef: "artifact-1", SafeDescription: "verify output",
		}},
		Decision: kind, ExecutionShape: shape, SafeSummary: "safe summary", CreatedAt: 1,
	}
	if kind == entity.ExecutionDecisionClarification {
		question := "please clarify"
		decision.ClarificationQuestion = &question
	}
	if kind == entity.ExecutionDecisionExecute && shape == entity.ExecutionShapeMultiStep {
		planScopeRunID := int64(12)
		decision.PlanScopeRunID = &planScopeRunID
	}
	return decision
}

func TestValidateAdaptiveAdmissionSnapshotAcceptsC1Sources(t *testing.T) {
	for _, source := range []entity.AdaptiveAdmissionSource{
		entity.AdaptiveAdmissionSourceFresh,
		entity.AdaptiveAdmissionSourceTypedInheritance,
		entity.AdaptiveAdmissionSourceLegacyDecoder,
	} {
		if err := ValidateAdaptiveAdmissionSnapshot(contractAdmission(source)); err != nil {
			t.Fatalf("ValidateAdaptiveAdmissionSnapshot(%q): %v", source, err)
		}
	}
}

func TestValidateAdaptiveAdmissionSnapshotPreservesC1LineageRules(t *testing.T) {
	cases := map[string]entity.AdaptiveAdmissionSnapshot{
		"fresh_with_lineage": func() entity.AdaptiveAdmissionSnapshot {
			v := contractAdmission(entity.AdaptiveAdmissionSourceFresh)
			runID := int64(1)
			v.SourceRunID = &runID
			return v
		}(),
		"typed_without_run": func() entity.AdaptiveAdmissionSnapshot {
			v := contractAdmission(entity.AdaptiveAdmissionSourceTypedInheritance)
			v.SourceRunID = nil
			return v
		}(),
		"legacy_gate_enabled": func() entity.AdaptiveAdmissionSnapshot {
			v := contractAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)
			v.FeatureGateEnabled = true
			return v
		}(),
		"legacy_wrong_decoder": func() entity.AdaptiveAdmissionSnapshot {
			v := contractAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)
			v.DecoderVersion = "other"
			return v
		}(),
		"subagents": func() entity.AdaptiveAdmissionSnapshot {
			v := contractAdmission(entity.AdaptiveAdmissionSourceFresh)
			v.Capabilities.SubagentsAllowed = true
			return v
		}(),
	}
	for name, snapshot := range cases {
		t.Run(name, func(t *testing.T) {
			if !errors.Is(ValidateAdaptiveAdmissionSnapshot(snapshot), ErrAdaptiveAdmissionInvalid) {
				t.Fatalf("expected ErrAdaptiveAdmissionInvalid")
			}
		})
	}
}

func TestValidateExecutionDecisionPreservesC1AttemptIDLimit(t *testing.T) {
	if err := ValidateExecutionDecision(contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)); err != nil {
		t.Fatalf("valid decision: %v", err)
	}
	for _, length := range []int{65, 191} {
		decision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
		decision.AttemptID = strings.Repeat("a", length)
		if err := ValidateExecutionDecision(decision); err != nil {
			t.Fatalf("attempt id with %d bytes rejected: %v", length, err)
		}
	}
	decision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
	decision.AttemptID = strings.Repeat("a", 192)
	if !errors.Is(ValidateExecutionDecision(decision), ErrExecutionDecisionInvalid) {
		t.Fatal("attempt id with 192 bytes was accepted")
	}
}

func TestValidateExecutionDecisionEnforcesSafeTextRules(t *testing.T) {
	cases := map[string]func(*entity.ExecutionDecision){
		"identifier_has_space":  func(v *entity.ExecutionDecision) { v.DecisionID = "decision id" },
		"identifier_has_token":  func(v *entity.ExecutionDecision) { v.AcceptanceChecks[0].CheckID = "ghp_12345678" },
		"goal_has_bearer":       func(v *entity.ExecutionDecision) { v.GoalSummary = "Bearer abcdefgh" },
		"deliverable_has_url":   func(v *entity.ExecutionDecision) { v.Deliverables[0] = "https://example.test" },
		"safe_summary_has_path": func(v *entity.ExecutionDecision) { v.SafeSummary = "/private/task" },
		"question_has_path": func(v *entity.ExecutionDecision) {
			clarification := contractDecision(entity.ExecutionDecisionClarification, entity.ExecutionShapeEmpty)
			question := `C:\\private\\task`
			clarification.ClarificationQuestion = &question
			*v = clarification
		},
		"acceptance_has_secret": func(v *entity.ExecutionDecision) {
			v.AcceptanceChecks[0].SafeDescription = "api_key=secret"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			decision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
			mutate(&decision)
			if !errors.Is(ValidateExecutionDecision(decision), ErrExecutionDecisionInvalid) {
				t.Fatalf("expected ErrExecutionDecisionInvalid")
			}
		})
	}
}

func TestValidateExecutionDecisionAgainstAdmissionPreservesPolicyError(t *testing.T) {
	admission := contractAdmission(entity.AdaptiveAdmissionSourceFresh)
	admission.Capabilities.PlanAllowed = false
	decision := contractDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeMultiStep)
	if !errors.Is(ValidateExecutionDecisionAgainstAdmission(admission, decision), ErrAdaptiveDecisionBlockedPolicy) {
		t.Fatal("expected plan policy to block multi-step decision")
	}
}

func TestValidateAdaptiveBootstrapPairRequiresC2BootstrapConstraints(t *testing.T) {
	admission := contractAdmission(entity.AdaptiveAdmissionSourceFresh)
	decision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
	identity := BootstrapIdentity{
		ExecutionRunID: decision.ExecutionRunID, JournalRunID: decision.JournalRunID,
		AttemptID: decision.AttemptID, ExecutionGeneration: decision.ExecutionGeneration,
	}
	if err := ValidateAdaptiveBootstrapPair(admission, decision, identity); err != nil {
		t.Fatalf("valid bootstrap pair: %v", err)
	}
	for name, mutate := range map[string]func(*BootstrapIdentity){
		"execution_run": func(v *BootstrapIdentity) { v.ExecutionRunID++ },
		"journal_run":   func(v *BootstrapIdentity) { v.JournalRunID++ },
		"attempt":       func(v *BootstrapIdentity) { v.AttemptID = "other" },
		"generation":    func(v *BootstrapIdentity) { v.ExecutionGeneration++ },
		"zero":          func(v *BootstrapIdentity) { v.ExecutionRunID = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := identity
			mutate(&invalid)
			if !errors.Is(ValidateAdaptiveBootstrapPair(admission, decision, invalid), ErrExecutionDecisionInvalid) {
				t.Fatalf("expected ErrExecutionDecisionInvalid")
			}
		})
	}
	for _, revision := range []uint64{0, 2} {
		t.Run("revision", func(t *testing.T) {
			invalid := decision
			invalid.DecisionRevision = revision
			if !errors.Is(ValidateAdaptiveBootstrapPair(admission, invalid, identity), ErrExecutionDecisionInvalid) {
				t.Fatalf("expected revision %d to be rejected", revision)
			}
		})
	}

	multiStep := contractDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeMultiStep)
	matchingScope := multiStep.ExecutionRunID
	multiStep.PlanScopeRunID = &matchingScope
	multiStepIdentity := BootstrapIdentity{
		ExecutionRunID:         multiStep.ExecutionRunID,
		JournalRunID:           multiStep.JournalRunID,
		AttemptID:              multiStep.AttemptID,
		ExecutionGeneration:    multiStep.ExecutionGeneration,
		ExpectedPlanScopeRunID: matchingScope,
	}
	if err := ValidateAdaptiveBootstrapPair(admission, multiStep, multiStepIdentity); err != nil {
		t.Fatalf("valid multi-step bootstrap pair: %v", err)
	}

	differentScope := int64(99)
	multiStepIdentity.ExpectedPlanScopeRunID = differentScope
	multiStep.PlanScopeRunID = &differentScope
	if err := ValidateAdaptiveBootstrapPair(admission, multiStep, multiStepIdentity); err != nil {
		t.Fatalf("valid recovery bootstrap pair with inherited plan scope: %v", err)
	}

	multiStepIdentity.ExpectedPlanScopeRunID = matchingScope
	if !errors.Is(ValidateAdaptiveBootstrapPair(admission, multiStep, multiStepIdentity), ErrExecutionDecisionInvalid) {
		t.Fatal("bootstrap pair accepted plan scope drift from its expected authority")
	}

	longAttempt := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
	longAttempt.AttemptID = strings.Repeat("a", 65)
	if err := ValidateExecutionDecision(longAttempt); err != nil {
		t.Fatalf("generic C1 validator rejected 65-byte attempt id: %v", err)
	}
	longAttemptIdentity := identity
	longAttemptIdentity.AttemptID = longAttempt.AttemptID
	if !errors.Is(ValidateAdaptiveBootstrapPair(admission, longAttempt, longAttemptIdentity), ErrExecutionDecisionInvalid) {
		t.Fatal("bootstrap pair accepted a C2-over-budget attempt id")
	}
}
