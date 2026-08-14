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
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
)

func baselineDecisionRequest() BaselineDecisionRequest {
	admission := validAdmission(entity.AdaptiveAdmissionSourceFresh)
	admission.FeatureGateEnabled = false
	return BaselineDecisionRequest{
		Admission: admission, DecisionID: "decision-baseline", DecisionRevision: 7,
		ExecutionRunID: 101, JournalRunID: 202, AttemptID: "attempt-baseline",
		ExecutionGeneration: 3, PlanScopeRunID: 303, CreatedAt: 404,
	}
}

func TestBaselineDecisionProducerProducesFixedMultiStepDecision(t *testing.T) {
	request := baselineDecisionRequest()
	decision, err := (BaselineDecisionProducer{}).Produce(request)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionDecision{
		Schema: entity.ExecutionDecisionSchemaV1, DecisionID: request.DecisionID,
		DecisionRevision: request.DecisionRevision, ExecutionRunID: request.ExecutionRunID,
		JournalRunID: request.JournalRunID, AttemptID: request.AttemptID,
		ExecutionGeneration: request.ExecutionGeneration, PlanScopeRunID: int64Pointer(request.PlanScopeRunID),
		GoalSummary: "Execute the submitted task.", Deliverables: []string{},
		AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{}, Decision: entity.ExecutionDecisionExecute,
		ExecutionShape: entity.ExecutionShapeMultiStep, SafeSummary: "Use the baseline multi-step execution path.",
		CreatedAt: request.CreatedAt,
	}, decision)
	require.NotNil(t, decision.Deliverables)
	require.NotNil(t, decision.AcceptanceChecks)
	require.Nil(t, decision.ClarificationQuestion)
	require.NotNil(t, decision.PlanScopeRunID)
	require.Equal(t, request.PlanScopeRunID, *decision.PlanScopeRunID)
}

func TestBaselineDecisionProducerFailsClosedForGateAndPolicy(t *testing.T) {
	gateOn := baselineDecisionRequest()
	gateOn.Admission.FeatureGateEnabled = true
	_, err := (BaselineDecisionProducer{}).Produce(gateOn)
	require.ErrorIs(t, err, ErrAdaptiveProducerUnavailable)

	planBlocked := baselineDecisionRequest()
	planBlocked.Admission.Capabilities.PlanAllowed = false
	_, err = (BaselineDecisionProducer{}).Produce(planBlocked)
	require.ErrorIs(t, err, ErrAdaptiveDecisionBlockedPolicy)
}

func TestBaselineDecisionProducerUsesExactConflictPrecedence(t *testing.T) {
	invalidAdmissionWithGate := baselineDecisionRequest()
	invalidAdmissionWithGate.Admission.Schema = "invalid"
	invalidAdmissionWithGate.Admission.FeatureGateEnabled = true
	_, err := (BaselineDecisionProducer{}).Produce(invalidAdmissionWithGate)
	require.ErrorIs(t, err, ErrAdaptiveAdmissionInvalid)

	gateOnWithBlockedPlanAndBadDecisionID := baselineDecisionRequest()
	gateOnWithBlockedPlanAndBadDecisionID.Admission.FeatureGateEnabled = true
	gateOnWithBlockedPlanAndBadDecisionID.Admission.Capabilities.PlanAllowed = false
	gateOnWithBlockedPlanAndBadDecisionID.DecisionID = ""
	_, err = (BaselineDecisionProducer{}).Produce(gateOnWithBlockedPlanAndBadDecisionID)
	require.ErrorIs(t, err, ErrAdaptiveProducerUnavailable)

	gateOffWithBlockedPlanAndBadDecisionID := baselineDecisionRequest()
	gateOffWithBlockedPlanAndBadDecisionID.Admission.Capabilities.PlanAllowed = false
	gateOffWithBlockedPlanAndBadDecisionID.DecisionID = ""
	_, err = (BaselineDecisionProducer{}).Produce(gateOffWithBlockedPlanAndBadDecisionID)
	require.ErrorIs(t, err, ErrAdaptiveDecisionBlockedPolicy)
}

func TestBaselineDecisionProducerRejectsInvalidAdmissionAndDecisionIdentifiers(t *testing.T) {
	invalidAdmission := baselineDecisionRequest()
	invalidAdmission.Admission.Schema = "invalid"
	_, err := (BaselineDecisionProducer{}).Produce(invalidAdmission)
	require.ErrorIs(t, err, ErrAdaptiveAdmissionInvalid)

	for name, mutate := range map[string]func(*BaselineDecisionRequest){
		"zero_decision_revision":    func(request *BaselineDecisionRequest) { request.DecisionRevision = 0 },
		"zero_execution_run_id":     func(request *BaselineDecisionRequest) { request.ExecutionRunID = 0 },
		"zero_journal_run_id":       func(request *BaselineDecisionRequest) { request.JournalRunID = 0 },
		"zero_execution_generation": func(request *BaselineDecisionRequest) { request.ExecutionGeneration = 0 },
		"zero_plan_scope_run_id":    func(request *BaselineDecisionRequest) { request.PlanScopeRunID = 0 },
		"zero_created_at":           func(request *BaselineDecisionRequest) { request.CreatedAt = 0 },
		"empty_decision_id":         func(request *BaselineDecisionRequest) { request.DecisionID = "" },
		"empty_attempt_id":          func(request *BaselineDecisionRequest) { request.AttemptID = "" },
		"decision_id_over_cap":      func(request *BaselineDecisionRequest) { request.DecisionID = strings.Repeat("x", 192) },
		"attempt_id_over_cap":       func(request *BaselineDecisionRequest) { request.AttemptID = strings.Repeat("x", 192) },
	} {
		t.Run(name, func(t *testing.T) {
			request := baselineDecisionRequest()
			mutate(&request)
			_, err := (BaselineDecisionProducer{}).Produce(request)
			require.ErrorIs(t, err, ErrExecutionDecisionInvalid)
		})
	}
}

func TestBaselineDecisionProducerIsDeterministicAndDoesNotMutateAdmission(t *testing.T) {
	request := baselineDecisionRequest()
	request.Admission = validAdmission(entity.AdaptiveAdmissionSourceTypedInheritance)
	request.Admission.FeatureGateEnabled = false
	before := request.Admission
	before.SourceRunID = int64Pointer(*request.Admission.SourceRunID)
	sourceExecutionGeneration := *request.Admission.SourceExecutionGeneration
	before.SourceExecutionGeneration = &sourceExecutionGeneration
	first, err := (BaselineDecisionProducer{}).Produce(request)
	require.NoError(t, err)
	second, err := (BaselineDecisionProducer{}).Produce(request)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NotSame(t, first.PlanScopeRunID, second.PlanScopeRunID)
	require.Equal(t, before, request.Admission)

	*first.PlanScopeRunID = 999
	first.Deliverables = append(first.Deliverables, "mutated")
	require.Equal(t, int64(303), *second.PlanScopeRunID)
	require.Empty(t, second.Deliverables)
	require.Equal(t, int64(303), request.PlanScopeRunID)
}

func TestBaselineDecisionProducerHasNarrowContract(t *testing.T) {
	producerType := reflect.TypeOf(BaselineDecisionProducer{})
	require.Equal(t, 0, producerType.NumField())
	requestType := reflect.TypeOf(BaselineDecisionRequest{})
	require.Equal(t, []string{
		"Admission", "DecisionID", "DecisionRevision", "ExecutionRunID", "JournalRunID", "AttemptID",
		"ExecutionGeneration", "PlanScopeRunID", "CreatedAt",
	}, reflectedFieldNames(requestType))

	source, err := os.ReadFile("adaptive_baseline_decision.go")
	require.NoError(t, err)
	for _, forbidden := range []string{
		"context.Context", "context.", "encoding/json", "json.", "Callback", "TaskText", "TaskBody", "TaskContent",
	} {
		require.NotContains(t, string(source), forbidden)
	}
	require.False(t, errors.Is(ErrAdaptiveProducerUnavailable, ErrAdaptiveDecisionBlockedPolicy))
}

func reflectedFieldNames(value reflect.Type) []string {
	names := make([]string, value.NumField())
	for index := 0; index < value.NumField(); index++ {
		names[index] = value.Field(index).Name
	}
	return names
}
