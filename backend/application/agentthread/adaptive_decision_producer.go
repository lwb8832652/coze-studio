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

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type AdaptiveDecisionRequest struct {
	Admission     entity.AdaptiveAdmissionSnapshot
	SemanticInput AdaptiveDecisionSemanticInput
}

type AdaptiveDecisionSemanticInput struct {
	Messages       []AdaptiveDecisionSemanticMessage
	HasAttachments bool
}

type AdaptiveDecisionSemanticMessage struct {
	Role    string
	Content string
}

type AdaptiveDecisionCandidate struct {
	GoalSummary           string
	Deliverables          []string
	AcceptanceChecks      []entity.AdaptiveAcceptanceCheck
	Decision              entity.ExecutionDecisionKind
	ExecutionShape        entity.ExecutionShape
	ClarificationQuestion *string
	SafeSummary           string
}

type AdaptiveDecisionProducer interface {
	Produce(context.Context, AdaptiveDecisionRequest) (AdaptiveDecisionCandidate, error)
}

// BaselineAdaptiveDecisionProducer adapts the frozen P1M baseline producer to
// the injectable P1D producer seam without widening its input contract.
type BaselineAdaptiveDecisionProducer struct{}

func (BaselineAdaptiveDecisionProducer) Produce(
	ctx context.Context,
	request AdaptiveDecisionRequest,
) (AdaptiveDecisionCandidate, error) {
	if err := ctx.Err(); err != nil {
		return AdaptiveDecisionCandidate{}, err
	}
	if err := ValidateAdaptiveAdmissionSnapshot(request.Admission); err != nil {
		return AdaptiveDecisionCandidate{}, err
	}
	if request.Admission.FeatureGateEnabled {
		return AdaptiveDecisionCandidate{}, ErrAdaptiveProducerUnavailable
	}
	if !request.Admission.Capabilities.PlanAllowed {
		return AdaptiveDecisionCandidate{}, ErrAdaptiveDecisionBlockedPolicy
	}
	return AdaptiveDecisionCandidate{
		GoalSummary: "Execute the submitted task.", Deliverables: []string{},
		AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
		Decision:         entity.ExecutionDecisionExecute, ExecutionShape: entity.ExecutionShapeMultiStep,
		SafeSummary: "Use the baseline multi-step execution path.",
	}, nil
}

// DeterministicAdaptiveDecisionProducer is the conservative MVP gate-on
// producer. It establishes the production seam without inspecting task text,
// request config, or legacy payloads; later model classification can replace
// it behind the same server-owned contract.
type DeterministicAdaptiveDecisionProducer struct{}

func (DeterministicAdaptiveDecisionProducer) Produce(
	ctx context.Context,
	request AdaptiveDecisionRequest,
) (AdaptiveDecisionCandidate, error) {
	if err := ctx.Err(); err != nil {
		return AdaptiveDecisionCandidate{}, err
	}
	if err := ValidateAdaptiveAdmissionSnapshot(request.Admission); err != nil {
		return AdaptiveDecisionCandidate{}, err
	}
	if !request.Admission.FeatureGateEnabled {
		return AdaptiveDecisionCandidate{}, ErrAdaptiveProducerUnavailable
	}
	if !request.Admission.Capabilities.PlanAllowed {
		return AdaptiveDecisionCandidate{}, ErrAdaptiveDecisionBlockedPolicy
	}
	return AdaptiveDecisionCandidate{
		GoalSummary:  "Execute the submitted task with adaptive controls.",
		Deliverables: []string{}, AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
		Decision: entity.ExecutionDecisionExecute, ExecutionShape: entity.ExecutionShapeMultiStep,
		SafeSummary: "Use the conservative adaptive multi-step execution path.",
	}, nil
}
