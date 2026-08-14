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

var ErrAdaptiveProducerUnavailable = errors.New("adaptive decision producer is unavailable")

type BaselineDecisionRequest struct {
	Admission           entity.AdaptiveAdmissionSnapshot
	DecisionID          string
	DecisionRevision    uint64
	ExecutionRunID      int64
	JournalRunID        int64
	AttemptID           string
	ExecutionGeneration uint64
	PlanScopeRunID      int64
	CreatedAt           int64
}

type BaselineDecisionProducer struct{}

func (BaselineDecisionProducer) Produce(request BaselineDecisionRequest) (entity.ExecutionDecision, error) {
	if err := ValidateAdaptiveAdmissionSnapshot(request.Admission); err != nil {
		return entity.ExecutionDecision{}, err
	}
	if request.Admission.FeatureGateEnabled {
		return entity.ExecutionDecision{}, ErrAdaptiveProducerUnavailable
	}
	if !request.Admission.Capabilities.PlanAllowed {
		return entity.ExecutionDecision{}, ErrAdaptiveDecisionBlockedPolicy
	}

	planScopeRunID := request.PlanScopeRunID
	decision := entity.ExecutionDecision{
		Schema:              entity.ExecutionDecisionSchemaV1,
		DecisionID:          request.DecisionID,
		DecisionRevision:    request.DecisionRevision,
		ExecutionRunID:      request.ExecutionRunID,
		JournalRunID:        request.JournalRunID,
		AttemptID:           request.AttemptID,
		ExecutionGeneration: request.ExecutionGeneration,
		PlanScopeRunID:      &planScopeRunID,
		GoalSummary:         "Execute the submitted task.",
		Deliverables:        []string{},
		AcceptanceChecks:    []entity.AdaptiveAcceptanceCheck{},
		Decision:            entity.ExecutionDecisionExecute,
		ExecutionShape:      entity.ExecutionShapeMultiStep,
		SafeSummary:         "Use the baseline multi-step execution path.",
		CreatedAt:           request.CreatedAt,
	}
	if err := ValidateExecutionDecisionAgainstAdmission(request.Admission, decision); err != nil {
		return entity.ExecutionDecision{}, err
	}
	return decision, nil
}
