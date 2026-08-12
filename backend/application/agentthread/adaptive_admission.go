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

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

var (
	ErrAdaptiveAdmissionInvalid      = errors.New("adaptive admission snapshot is invalid")
	ErrExecutionDecisionInvalid      = errors.New("execution decision is invalid")
	ErrAdaptiveDecisionBlockedPolicy = errors.New("adaptive execution decision is blocked by policy")
)

func ValidateAdaptiveAdmissionSnapshot(snapshot entity.AdaptiveAdmissionSnapshot) error {
	if snapshot.Schema != entity.AdaptiveAdmissionSchemaV1 || snapshot.Capabilities.SubagentsAllowed ||
		!adaptiveLimitsValid(snapshot.Limits) {
		return ErrAdaptiveAdmissionInvalid
	}
	switch snapshot.Source {
	case entity.AdaptiveAdmissionSourceFresh:
		if snapshot.SourceRunID != nil || snapshot.SourceExecutionGeneration != nil ||
			snapshot.SourceConfigDigest != "" || snapshot.DecoderVersion != "" {
			return ErrAdaptiveAdmissionInvalid
		}
	case entity.AdaptiveAdmissionSourceTypedInheritance:
		if !positiveInt64(snapshot.SourceRunID) || !positiveUint64(snapshot.SourceExecutionGeneration) ||
			snapshot.SourceConfigDigest != "" || snapshot.DecoderVersion != "" {
			return ErrAdaptiveAdmissionInvalid
		}
	case entity.AdaptiveAdmissionSourceLegacyDecoder:
		if snapshot.FeatureGateEnabled || !positiveInt64(snapshot.SourceRunID) ||
			!positiveUint64(snapshot.SourceExecutionGeneration) || !lowerHex(snapshot.SourceConfigDigest, 64) ||
			snapshot.DecoderVersion != entity.AdaptiveLegacyDecoderVersionV1 {
			return ErrAdaptiveAdmissionInvalid
		}
	default:
		return ErrAdaptiveAdmissionInvalid
	}
	return nil
}

func ValidateExecutionDecision(decision entity.ExecutionDecision) error {
	if decision.Schema != entity.ExecutionDecisionSchemaV1 || !nonEmptyBytes(decision.DecisionID, 191) ||
		decision.DecisionRevision == 0 || decision.ExecutionRunID <= 0 || decision.JournalRunID <= 0 ||
		!nonEmptyBytes(decision.AttemptID, 191) || decision.ExecutionGeneration == 0 ||
		!nonEmptyBytes(decision.GoalSummary, 1024) || !nonEmptyBytes(decision.SafeSummary, 1024) ||
		decision.CreatedAt <= 0 || !deliverablesValid(decision.Deliverables) ||
		!acceptanceChecksValid(decision.AcceptanceChecks) || !decisionFormValid(decision) {
		return ErrExecutionDecisionInvalid
	}
	return nil
}

func ValidateExecutionDecisionAgainstAdmission(
	admission entity.AdaptiveAdmissionSnapshot,
	decision entity.ExecutionDecision,
) error {
	if err := ValidateExecutionDecision(decision); err != nil {
		return err
	}
	if err := ValidateAdaptiveAdmissionSnapshot(admission); err != nil {
		return err
	}
	if decision.Decision == entity.ExecutionDecisionExecute &&
		decision.ExecutionShape == entity.ExecutionShapeMultiStep && !admission.Capabilities.PlanAllowed {
		return ErrAdaptiveDecisionBlockedPolicy
	}
	if decision.Decision == entity.ExecutionDecisionClarification &&
		!admission.Capabilities.HumanInteractionAllowed {
		return ErrAdaptiveDecisionBlockedPolicy
	}
	return nil
}

func adaptiveLimitsValid(limits entity.AdaptiveLimits) bool {
	return limits.MaxToolCalls > 0 && limits.MaxToolCalls <= 24 && limits.MaxReplans > 0 &&
		limits.MaxReplans <= 2 && limits.MaxVerificationRepairs > 0 &&
		limits.MaxVerificationRepairs <= 2 && limits.MaxConsecutiveNoProgress > 0 &&
		limits.MaxConsecutiveNoProgress <= 3 && limits.MaxActiveDurationSeconds > 0 &&
		limits.MaxActiveDurationSeconds <= 1200
}

func decisionFormValid(decision entity.ExecutionDecision) bool {
	questionValid := decision.ClarificationQuestion != nil && nonEmptyBytes(*decision.ClarificationQuestion, 1024)
	planValid := positiveInt64(decision.PlanScopeRunID)
	switch decision.Decision {
	case entity.ExecutionDecisionClarification:
		return questionValid && decision.ExecutionShape == entity.ExecutionShapeEmpty && decision.PlanScopeRunID == nil
	case entity.ExecutionDecisionDirect:
		return decision.ClarificationQuestion == nil && decision.ExecutionShape == entity.ExecutionShapeEmpty && decision.PlanScopeRunID == nil
	case entity.ExecutionDecisionExecute:
		return decision.ClarificationQuestion == nil &&
			((decision.ExecutionShape == entity.ExecutionShapeSingleStep && decision.PlanScopeRunID == nil) ||
				(decision.ExecutionShape == entity.ExecutionShapeMultiStep && planValid))
	default:
		return false
	}
}

func deliverablesValid(deliverables []string) bool {
	if deliverables == nil || len(deliverables) > 16 {
		return false
	}
	for _, deliverable := range deliverables {
		if !nonEmptyBytes(deliverable, 512) {
			return false
		}
	}
	return true
}

func acceptanceChecksValid(checks []entity.AdaptiveAcceptanceCheck) bool {
	if checks == nil || len(checks) > 32 {
		return false
	}
	for _, check := range checks {
		if !nonEmptyBytes(check.CheckID, 191) || !nonEmptyBytes(check.Kind, 64) ||
			!nonEmptyBytes(check.TargetRef, 191) || !nonEmptyBytes(check.SafeDescription, 512) {
			return false
		}
	}
	return true
}

func positiveInt64(value *int64) bool { return value != nil && *value > 0 }

func positiveUint64(value *uint64) bool { return value != nil && *value > 0 }

func nonEmptyBytes(value string, maximum int) bool { return value != "" && len(value) <= maximum }

func lowerHex(value string, expectedLength int) bool {
	if len(value) != expectedLength {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}
