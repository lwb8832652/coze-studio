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
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const comparisonSchemaV1 = "newx.deerflow.agent.comparison.v1"

func CompareCase(testCase Case, reference, candidate Observation) ComparisonResult {
	result := ComparisonResult{
		Schema: comparisonSchemaV1,
		CaseID: testCase.ID,
		Status: StatusAligned,
		Checks: make([]ComparisonCheck, 0, 24),
	}
	if blocker := safeBlocker(reference.Blocker); blocker != "" {
		result.Status = StatusBlocked
		result.Blocker = blocker
		if !strings.HasPrefix(blocker, "deerflow_") {
			result.Blocker = "deerflow_" + blocker
		}
		return result
	}
	if blocker := safeBlocker(candidate.Blocker); blocker != "" {
		result.Status = StatusBlocked
		result.Blocker = blocker
		return result
	}

	result.Checks = append(result.Checks, observationChecks("reference:", testCase, reference, ProductDeerFlow)...)
	result.Checks = append(result.Checks, observationChecks("", testCase, candidate, ProductNewX)...)

	for _, check := range result.Checks {
		if !check.Passed {
			result.Status = StatusDifferent
			return result
		}
	}
	if hasBoundedExtraDiagnostics(reference.EventFamilies, candidate.EventFamilies) {
		result.Status = StatusStronger
	}
	return result
}

func observationChecks(prefix string, testCase Case, observation Observation, expectedProduct Product) []ComparisonCheck {
	checks := make([]ComparisonCheck, 0, 24)
	addCheck := func(name, expected, actual string, passed bool) {
		if strings.TrimSpace(actual) == "" {
			actual = "missing"
		}
		checks = append(checks, ComparisonCheck{
			Name: prefix + name, Expected: expected, Actual: actual, Passed: passed,
		})
	}
	addCheck("schema", ObservationSchemaV1, observation.Schema, observation.Schema == ObservationSchemaV1)
	addCheck("product", string(expectedProduct), string(observation.Product), observation.Product == expectedProduct)
	addCheck("case_id", testCase.ID, observation.CaseID, observation.CaseID == testCase.ID)
	addCheck("mode", string(testCase.Mode), string(observation.Mode), observation.Mode == testCase.Mode)
	compareCapability := func(name string, expected *bool, actual bool) {
		if expected == nil {
			return
		}
		addCheck("capability:"+name, strconv.FormatBool(*expected), strconv.FormatBool(actual), *expected == actual)
	}
	compareCapability("thinking", testCase.Expect.Capabilities.Thinking, observation.Capabilities.Thinking)
	compareCapability("plan", testCase.Expect.Capabilities.Plan, observation.Capabilities.Plan)
	compareCapability("subagent", testCase.Expect.Capabilities.Subagent, observation.Capabilities.Subagent)

	for _, family := range testCase.Expect.RequiredEvents {
		addCheck("required_event:"+family, "present", presence(observation.EventFamilies, family), slices.Contains(observation.EventFamilies, family))
	}
	for _, pair := range testCase.Expect.EventOrder {
		before := firstIndex(observation.EventFamilies, pair[0])
		after := firstIndex(observation.EventFamilies, pair[1])
		addCheck("event_order:"+pair[0]+"<"+pair[1], "ordered", fmt.Sprintf("%d<%d", before, after), before >= 0 && after >= 0 && before < after)
	}
	addCheck("terminal", strings.Join(testCase.Expect.RequiredTerminal, "|"), observation.Terminal, slices.Contains(testCase.Expect.RequiredTerminal, observation.Terminal))
	addCheck(
		"terminal_exactly_once",
		strconv.Itoa(observation.RunCount),
		strconv.Itoa(observation.TerminalEvents),
		observation.RunCount > 0 && observation.TerminalEvents == observation.RunCount,
	)

	if testCase.Expect.AssistantMessageRequired {
		addCheck("assistant_message", "present", strconv.FormatBool(observation.AssistantMessagePresent), observation.AssistantMessagePresent)
	}
	if testCase.Expect.TokenUsageRequired {
		addCheck("token_usage", "positive", strconv.FormatInt(observation.Token.Total, 10), observation.Token.Total > 0)
	}
	if testCase.Expect.Todo.Required {
		addCheck("todo_present", "positive", strconv.Itoa(observation.Todo.Total), observation.Todo.Total > 0)
	}
	if testCase.Expect.Todo.AllCompleted {
		allCompleted := observation.Todo.Total > 0 && observation.Todo.Completed == observation.Todo.Total
		addCheck("todo_completed", "all", fmt.Sprintf("%d/%d", observation.Todo.Completed, observation.Todo.Total), allCompleted)
	}
	if minimum := testCase.Expect.Children.Minimum; minimum > 0 {
		addCheck("child_runs", fmt.Sprintf(">=%d", minimum), strconv.Itoa(observation.ChildRuns), observation.ChildRuns >= minimum)
		addCheck(
			"child_runs_completed",
			fmt.Sprintf(">=%d", minimum),
			strconv.Itoa(observation.CompletedChildRuns),
			observation.CompletedChildRuns >= minimum,
		)
	}
	if testCase.Expect.Clarification.Required {
		addCheck("clarification", "present", strconv.FormatBool(observation.ClarificationPresent), observation.ClarificationPresent)
	}
	if testCase.Expect.Clarification.FollowUpRequired {
		addCheck("follow_up", "observed", strconv.FormatBool(observation.FollowUpObserved), observation.FollowUpObserved)
	}
	if testCase.Expect.NoSuccessAfterCancel {
		addCheck("cancel_fence", "no_success_after_cancel", strconv.FormatBool(observation.SuccessAfterCancel), !observation.SuccessAfterCancel)
	}
	if testCase.Expect.ReconnectDeduplicated {
		passed := observation.Reconnected && observation.DuplicateEvents == 0 &&
			observation.StreamTerminalObserved
		addCheck(
			"reconnect_deduplicated",
			"true/0/terminal=true",
			fmt.Sprintf(
				"%t/%d/terminal=%t",
				observation.Reconnected,
				observation.DuplicateEvents,
				observation.StreamTerminalObserved,
			),
			passed,
		)
	}
	if caseHasAction(testCase, ActionReloadState) {
		addCheck("state_reload", "durable", strconv.FormatBool(observation.StateReloaded), observation.StateReloaded)
		addCheck("checkpoint_history", "positive", strconv.Itoa(observation.HistoryEntries), observation.HistoryEntries > 0)
	}
	return checks
}

func safeBlocker(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !safeEventFamilyPattern.MatchString(value) {
		return ""
	}
	return value
}

func firstIndex(values []string, expected string) int {
	for index, value := range values {
		if value == expected {
			return index
		}
	}
	return -1
}

func presence(values []string, expected string) string {
	if slices.Contains(values, expected) {
		return "present"
	}
	return "missing"
}

func hasBoundedExtraDiagnostics(reference, candidate []string) bool {
	for _, family := range candidate {
		if slices.Contains(reference, family) {
			continue
		}
		if family == "runtime.diagnostic" || strings.HasPrefix(family, "runtime.diagnostic.") {
			return true
		}
	}
	return false
}
