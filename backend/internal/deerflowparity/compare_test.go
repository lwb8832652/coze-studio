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

package deerflowparity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareCaseAligned(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := passingObservation(ProductDeerFlow, testCase)
	candidate := passingObservation(ProductNewX, testCase)

	result := CompareCase(testCase, reference, candidate)
	require.Equal(t, StatusAligned, result.Status)
	require.NotEmpty(t, result.Checks)
	for _, check := range result.Checks {
		require.True(t, check.Passed, check.Name)
	}
}

func TestCompareCaseDifferentWhenRequiredEventIsMissing(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := passingObservation(ProductDeerFlow, testCase)
	candidate := passingObservation(ProductNewX, testCase)
	candidate.EventFamilies = []string{"run.started", "run.completed"}

	result := CompareCase(testCase, reference, candidate)
	require.Equal(t, StatusDifferent, result.Status)
	require.Contains(t, failedCheckNames(result), "required_event:assistant.completed")
}

func TestCompareCaseDifferentWhenReferenceViolatesLockedContract(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := passingObservation(ProductDeerFlow, testCase)
	reference.EventFamilies = []string{"run.started", "run.completed"}
	candidate := passingObservation(ProductNewX, testCase)

	result := CompareCase(testCase, reference, candidate)
	require.Equal(t, StatusDifferent, result.Status)
	require.Contains(t, failedCheckNames(result), "reference:required_event:assistant.completed")
}

func TestCompareCaseDifferentWhenCancellationFenceIsViolated(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.cancel")
	reference := passingObservation(ProductDeerFlow, testCase)
	candidate := passingObservation(ProductNewX, testCase)
	candidate.SuccessAfterCancel = true

	result := CompareCase(testCase, reference, candidate)
	require.Equal(t, StatusDifferent, result.Status)
	require.Contains(t, failedCheckNames(result), "cancel_fence")
}

func TestCompareCaseDifferentWhenTerminalIsNotExactlyOncePerRun(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := passingObservation(ProductDeerFlow, testCase)
	candidate := passingObservation(ProductNewX, testCase)
	candidate.TerminalEvents = 2

	result := CompareCase(testCase, reference, candidate)
	require.Equal(t, StatusDifferent, result.Status)
	require.Contains(t, failedCheckNames(result), "terminal_exactly_once")
}

func TestCompareCaseMarksBoundedExtraDiagnosticsStronger(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := passingObservation(ProductDeerFlow, testCase)
	candidate := passingObservation(ProductNewX, testCase)
	candidate.EventFamilies = append(candidate.EventFamilies, "runtime.diagnostic")

	result := CompareCase(testCase, reference, candidate)
	require.Equal(t, StatusStronger, result.Status)
}

func TestCompareCasePropagatesSafeBlocker(t *testing.T) {
	t.Parallel()

	testCase := mustAcceptanceCase(t, "core.flash.direct")
	reference := passingObservation(ProductDeerFlow, testCase)
	candidate := passingObservation(ProductNewX, testCase)
	candidate.Blocker = "newx_schema_migration_missing"

	result := CompareCase(testCase, reference, candidate)
	require.Equal(t, StatusBlocked, result.Status)
	require.Equal(t, "newx_schema_migration_missing", result.Blocker)
}

func mustAcceptanceCase(t *testing.T, id string) Case {
	t.Helper()
	suite, err := LoadCases()
	require.NoError(t, err)
	for _, testCase := range suite.Cases {
		if testCase.ID == id {
			return testCase
		}
	}
	t.Fatalf("acceptance case %q not found", id)
	return Case{}
}

func passingObservation(product Product, testCase Case) Observation {
	runCount := 1
	if caseHasAction(testCase, ActionFollowUp) {
		runCount = 2
	}
	observation := Observation{
		Schema:                  ObservationSchemaV1,
		Product:                 product,
		CaseID:                  testCase.ID,
		Mode:                    testCase.Mode,
		Capabilities:            capabilityStateFromExpectation(testCase.Expect.Capabilities),
		EventFamilies:           append([]string(nil), testCase.Expect.RequiredEvents...),
		AssistantMessagePresent: testCase.Expect.AssistantMessageRequired,
		AssistantMessageBytes:   12,
		Token:                   TokenObservation{Input: 10, Output: 2, Total: 12},
		Terminal:                testCase.Expect.RequiredTerminal[0],
		ChildRuns:               testCase.Expect.Children.Minimum,
		ClarificationPresent:    testCase.Expect.Clarification.Required,
		FollowUpObserved:        testCase.Expect.Clarification.FollowUpRequired,
		Reconnected:             testCase.Expect.ReconnectDeduplicated,
		RunCount:                runCount,
		TerminalEvents:          runCount,
	}
	if testCase.Expect.Todo.Required {
		observation.Todo = TodoObservation{Total: 1, Completed: 1}
	}
	return observation
}

func capabilityStateFromExpectation(expect CapabilityExpectation) CapabilityState {
	state := CapabilityState{}
	if expect.Thinking != nil {
		state.Thinking = *expect.Thinking
	}
	if expect.Plan != nil {
		state.Plan = *expect.Plan
	}
	if expect.Subagent != nil {
		state.Subagent = *expect.Subagent
	}
	return state
}

func failedCheckNames(result ComparisonResult) []string {
	names := make([]string, 0)
	for _, check := range result.Checks {
		if !check.Passed {
			names = append(names, check.Name)
		}
	}
	return names
}
