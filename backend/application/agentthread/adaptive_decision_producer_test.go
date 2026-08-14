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
	"reflect"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
)

func TestBaselineAdaptiveDecisionProducerAdaptsGateOffContract(t *testing.T) {
	request := AdaptiveDecisionRequest{Admission: baselineDecisionRequest().Admission}

	candidate, err := (BaselineAdaptiveDecisionProducer{}).Produce(context.Background(), request)

	require.NoError(t, err)
	require.Equal(t, AdaptiveDecisionCandidate{
		GoalSummary: "Execute the submitted task.", Deliverables: []string{},
		AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
		Decision:         entity.ExecutionDecisionExecute, ExecutionShape: entity.ExecutionShapeMultiStep,
		SafeSummary: "Use the baseline multi-step execution path.",
	}, candidate)
}

func TestDeterministicAdaptiveDecisionProducerProducesConservativeGateOnCandidate(t *testing.T) {
	request := AdaptiveDecisionRequest{Admission: baselineDecisionRequest().Admission}
	request.Admission.FeatureGateEnabled = true

	candidate, err := (DeterministicAdaptiveDecisionProducer{}).Produce(context.Background(), request)

	require.NoError(t, err)
	require.Equal(t, entity.ExecutionDecisionExecute, candidate.Decision)
	require.Equal(t, entity.ExecutionShapeMultiStep, candidate.ExecutionShape)
	require.Equal(t, "Execute the submitted task with adaptive controls.", candidate.GoalSummary)
	require.Equal(t, "Use the conservative adaptive multi-step execution path.", candidate.SafeSummary)
	require.Empty(t, candidate.Deliverables)
	require.Empty(t, candidate.AcceptanceChecks)
}

func TestDeterministicAdaptiveDecisionProducerFailsClosedForWrongGateAndPolicy(t *testing.T) {
	gateOff := AdaptiveDecisionRequest{Admission: baselineDecisionRequest().Admission}
	_, err := (DeterministicAdaptiveDecisionProducer{}).Produce(context.Background(), gateOff)
	require.ErrorIs(t, err, ErrAdaptiveProducerUnavailable)

	blocked := AdaptiveDecisionRequest{Admission: baselineDecisionRequest().Admission}
	blocked.Admission.FeatureGateEnabled = true
	blocked.Admission.Capabilities.PlanAllowed = false
	_, err = (DeterministicAdaptiveDecisionProducer{}).Produce(context.Background(), blocked)
	require.ErrorIs(t, err, ErrAdaptiveDecisionBlockedPolicy)
}

func TestAdaptiveDecisionProducerContractCannotObserveDurableAuthority(t *testing.T) {
	require.Equal(t, []string{"Admission", "SemanticInput"}, reflectedFieldNames(reflect.TypeOf(AdaptiveDecisionRequest{})))
	require.Equal(t, []string{"Messages", "HasAttachments"}, reflectedFieldNames(reflect.TypeOf(AdaptiveDecisionSemanticInput{})))
	require.Equal(t, []string{"Role", "Content"}, reflectedFieldNames(reflect.TypeOf(AdaptiveDecisionSemanticMessage{})))
	require.Equal(t, []string{
		"GoalSummary", "Deliverables", "AcceptanceChecks", "Decision", "ExecutionShape",
		"ClarificationQuestion", "SafeSummary",
	}, reflectedFieldNames(reflect.TypeOf(AdaptiveDecisionCandidate{})))
}
