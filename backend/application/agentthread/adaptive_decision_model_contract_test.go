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
	"encoding/json"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
)

func TestProjectAdaptiveDecisionSemanticInputProjectsOnlyTypedSemantics(t *testing.T) {
	input, err := ProjectAdaptiveDecisionSemanticInput(`{
		"messages":[
			{"_run_id":41,"role":"user","content":"build it"},
			{"_run_id":41,"role":"assistant","content":"working"},
			{"role":"user","content":"continue"}
		],
		"uploaded_files":[{"file_id":9,"file_name":"secret.txt","virtual_path":"/private/secret.txt"}]
	}`)

	require.NoError(t, err)
	require.Equal(t, AdaptiveDecisionSemanticInput{
		Messages: []AdaptiveDecisionSemanticMessage{
			{Role: "user", Content: "build it"},
			{Role: "assistant", Content: "working"},
			{Role: "user", Content: "continue"},
		},
		HasAttachments: true,
	}, input)
}

func TestProjectAdaptiveDecisionSemanticInputRejectsOpenOrMalformedInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "null root", input: "null"},
		{name: "missing messages", input: `{}`},
		{name: "null messages", input: `{"messages":null}`},
		{name: "empty messages", input: `{"messages":[]}`},
		{name: "null message", input: `{"messages":[null]}`},
		{name: "unknown root field", input: `{"messages":[{"role":"user","content":"go"}],"metadata":{}}`},
		{name: "unknown message field", input: `{"messages":[{"role":"user","content":"go","name":"owner"}]}`},
		{name: "unsupported role", input: `{"messages":[{"role":"system","content":"go"}]}`},
		{name: "non canonical role", input: `{"messages":[{"role":"User","content":"go"}]}`},
		{name: "empty content", input: `{"messages":[{"role":"user","content":""}]}`},
		{name: "last message is assistant", input: `{"messages":[{"role":"user","content":"go"},{"role":"assistant","content":"done"}]}`},
		{name: "null attachments", input: `{"messages":[{"role":"user","content":"go"}],"uploaded_files":null}`},
		{name: "null attachment", input: `{"messages":[{"role":"user","content":"go"}],"uploaded_files":[null]}`},
		{name: "unknown attachment field", input: `{"messages":[{"role":"user","content":"go"}],"uploaded_files":[{"file_name":"x","secret":"leak"}]}`},
		{name: "trailing document", input: `{"messages":[{"role":"user","content":"go"}]} {}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ProjectAdaptiveDecisionSemanticInput(test.input)
			require.Error(t, err)
		})
	}

	t.Run("invalid utf8", func(t *testing.T) {
		_, err := ProjectAdaptiveDecisionSemanticInput(string([]byte{
			'{', '"', 'm', 'e', 's', 's', 'a', 'g', 'e', 's', '"', ':', '[',
			'{', '"', 'r', 'o', 'l', 'e', '"', ':', '"', 'u', 's', 'e', 'r', '"', ',',
			'"', 'c', 'o', 'n', 't', 'e', 'n', 't', '"', ':', '"', 0xff, '"', '}', ']', '}',
		}))
		require.Error(t, err)
	})
}

func TestProjectAdaptiveDecisionSemanticInputRejectsResourceLimitOverflow(t *testing.T) {
	messages := make([]map[string]string, 33)
	for i := range messages {
		messages[i] = map[string]string{"role": "user", "content": "go"}
	}
	raw, err := json.Marshal(map[string]any{"messages": messages})
	require.NoError(t, err)

	_, err = ProjectAdaptiveDecisionSemanticInput(string(raw))
	require.Error(t, err)

	tooLarge := `{"messages":[{"role":"user","content":"` + strings.Repeat("x", 64<<10) + `"}]}`
	_, err = ProjectAdaptiveDecisionSemanticInput(tooLarge)
	require.Error(t, err)
}

func TestProjectAdaptiveDecisionSemanticInputAcceptsClosedInputWithoutAttachments(t *testing.T) {
	input, err := ProjectAdaptiveDecisionSemanticInput(
		`{"messages":[{"role":"assistant","content":"previous"},{"role":"user","content":"next"}],"uploaded_files":[]}`,
	)

	require.NoError(t, err)
	require.False(t, input.HasAttachments)
	require.Len(t, input.Messages, 2)
}

func TestDecodeAdaptiveDecisionCandidateWireAcceptsTypedCandidate(t *testing.T) {
	admission := baselineDecisionRequest().Admission
	admission.FeatureGateEnabled = true

	candidate, err := decodeAdaptiveDecisionCandidateWire([]byte(`{
		"schema":"coze.adaptive_decision_candidate.v1",
		"goal_summary":"Implement the requested change.",
		"deliverables":[],
		"acceptance_checks":[],
		"decision":"execute",
		"execution_shape":"single_step",
		"safe_summary":"Use one bounded implementation step."
	}`), admission)

	require.NoError(t, err)
	require.Equal(t, AdaptiveDecisionCandidate{
		GoalSummary:      "Implement the requested change.",
		Deliverables:     []string{},
		AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
		Decision:         entity.ExecutionDecisionExecute,
		ExecutionShape:   entity.ExecutionShapeSingleStep,
		SafeSummary:      "Use one bounded implementation step.",
	}, candidate)
}

func TestEncodeAdaptiveDecisionCandidateWireUsesRepositoryCanonicalJSON(t *testing.T) {
	raw, err := encodeAdaptiveDecisionCandidateWire(AdaptiveDecisionCandidate{
		GoalSummary:      "Implement the requested change.",
		Deliverables:     []string{},
		AcceptanceChecks: []entity.AdaptiveAcceptanceCheck{},
		Decision:         entity.ExecutionDecisionExecute,
		ExecutionShape:   entity.ExecutionShapeSingleStep,
		SafeSummary:      "Use one bounded implementation step.",
	})

	require.NoError(t, err)
	require.Equal(t, `{"acceptance_checks":[],"decision":"execute","deliverables":[],"execution_shape":"single_step","goal_summary":"Implement the requested change.","safe_summary":"Use one bounded implementation step.","schema":"coze.adaptive_decision_candidate.v1"}`, string(raw))
}

func TestDecodeAdaptiveDecisionCandidateWireRejectsUntrustedForms(t *testing.T) {
	valid := `{
		"schema":"coze.adaptive_decision_candidate.v1",
		"goal_summary":"Implement the requested change.",
		"deliverables":[],
		"acceptance_checks":[],
		"decision":"execute",
		"execution_shape":"single_step",
		"safe_summary":"Use one bounded implementation step."
	}`
	tests := []struct {
		name string
		raw  string
	}{
		{name: "plain missing fields", raw: `{}`},
		{name: "wrong schema", raw: strings.Replace(valid, adaptiveDecisionCandidateWireSchema, "v2", 1)},
		{name: "unknown field", raw: strings.Replace(valid, `"safe_summary":`, `"decision_id":"server-owned","safe_summary":`, 1)},
		{name: "nil deliverables", raw: strings.Replace(valid, `"deliverables":[]`, `"deliverables":null`, 1)},
		{name: "nil acceptance checks", raw: strings.Replace(valid, `"acceptance_checks":[]`, `"acceptance_checks":null`, 1)},
		{name: "unknown decision", raw: strings.Replace(valid, `"decision":"execute"`, `"decision":"maybe"`, 1)},
		{name: "illegal shape", raw: strings.Replace(valid, `"execution_shape":"single_step"`, `"execution_shape":""`, 1)},
		{name: "multiple values", raw: valid + `{}`},
		{name: "overlong goal", raw: strings.Replace(valid, "Implement the requested change.", strings.Repeat("x", 1025), 1)},
		{name: "oversized arguments", raw: strings.Repeat(" ", (64<<10)+1) + valid},
	}

	admission := baselineDecisionRequest().Admission
	admission.FeatureGateEnabled = true
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeAdaptiveDecisionCandidateWire([]byte(test.raw), admission)
			require.Error(t, err)
		})
	}
}

func TestDecodeAdaptiveDecisionCandidateWireRejectsAdmissionPolicyViolation(t *testing.T) {
	admission := baselineDecisionRequest().Admission
	admission.FeatureGateEnabled = true
	admission.Capabilities.PlanAllowed = false

	_, err := decodeAdaptiveDecisionCandidateWire([]byte(`{
		"schema":"coze.adaptive_decision_candidate.v1",
		"goal_summary":"Implement the requested change.",
		"deliverables":[],
		"acceptance_checks":[],
		"decision":"execute",
		"execution_shape":"multi_step",
		"safe_summary":"Use a bounded implementation plan."
	}`), admission)

	require.ErrorIs(t, err, ErrAdaptiveDecisionBlockedPolicy)
}
