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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadCases(t *testing.T) {
	t.Parallel()

	suite, err := LoadCases()
	require.NoError(t, err)
	require.Equal(t, ContractSchemaV1, suite.Schema)
	require.Equal(t, ScopeSemanticCore, suite.Scope)
	require.Equal(t, "5851f8250eb150ca23134c79b11ebc5073ac2789", suite.DeerFlowRevision)
	require.Len(t, suite.Cases, 7)
}

func TestSemanticCoreCoverage(t *testing.T) {
	t.Parallel()

	suite, err := LoadCases()
	require.NoError(t, err)
	require.Equal(t, SemanticCoreCaseIDs(), suite.CaseIDs())

	for _, testCase := range suite.Cases {
		require.NotEmpty(t, testCase.InputPrompt)
		require.NotEmpty(t, testCase.Actions)
		require.Contains(t, []Mode{ModeFlash, ModeThinking, ModePro, ModeUltra}, testCase.Mode)
		require.NotEmpty(t, testCase.Expect.RequiredTerminal)
		require.NotEmpty(t, testCase.Expect.RequiredEvents)
		require.NotEmpty(t, testCase.Expect.EventOrder)
	}
}

func TestDecodeCasesRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"unknown field": `{
			"schema":"newx.deerflow.agent.acceptance.v1",
			"scope":"semantic_core",
			"deerflow_revision":"5851f8250eb150ca23134c79b11ebc5073ac2789",
			"password":"forbidden",
			"cases":[]
		}`,
		"duplicate case": `{
			"schema":"newx.deerflow.agent.acceptance.v1",
			"scope":"semantic_core",
			"deerflow_revision":"5851f8250eb150ca23134c79b11ebc5073ac2789",
			"cases":[
				{"id":"core.flash.direct","mode":"flash","input_prompt":"a","actions":[{"type":"run"}],"expect":{"required_terminal":["success"],"required_events":["run.started"],"event_order":[["run.started","run.completed"]]}},
				{"id":"core.flash.direct","mode":"flash","input_prompt":"b","actions":[{"type":"run"}],"expect":{"required_terminal":["success"],"required_events":["run.started"],"event_order":[["run.started","run.completed"]]}}
			]
		}`,
		"invalid mode": `{
			"schema":"newx.deerflow.agent.acceptance.v1",
			"scope":"semantic_core",
			"deerflow_revision":"5851f8250eb150ca23134c79b11ebc5073ac2789",
			"cases":[
				{"id":"core.flash.direct","mode":"auto","input_prompt":"a","actions":[{"type":"run"}],"expect":{"required_terminal":["success"],"required_events":["run.started"],"event_order":[["run.started","run.completed"]]}}
			]
		}`,
	}

	for name, raw := range tests {
		raw := raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := DecodeCases(strings.NewReader(raw))
			require.Error(t, err)
		})
	}
}
