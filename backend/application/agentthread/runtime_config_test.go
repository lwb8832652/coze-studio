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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDeerFlowRuntimeConfigProjectsModes(t *testing.T) {
	tests := []struct {
		mode       DeerFlowMode
		thinking   bool
		plan       bool
		subagent   bool
		effort     string
		concurrent int
	}{
		{mode: DeerFlowModePro, thinking: true, plan: true, effort: "medium"},
		{mode: DeerFlowModeUltra, thinking: true, plan: true, subagent: true, effort: "high", concurrent: 3},
	}

	for _, test := range tests {
		t.Run(string(test.mode), func(t *testing.T) {
			config, err := ParseDeerFlowRuntimeConfig(`{"mode":"` + string(test.mode) + `"}`)

			require.NoError(t, err)
			require.Equal(t, test.mode, config.Mode)
			require.True(t, config.ModeExplicit)
			require.Equal(t, test.thinking, config.ThinkingEnabled)
			require.Equal(t, test.plan, config.IsPlanMode)
			require.Equal(t, test.subagent, config.SubagentEnabled)
			require.Equal(t, test.effort, config.ReasoningEffort)
			require.Equal(t, test.concurrent, config.MaxConcurrentSubagents)
			require.Equal(t, test.plan, config.PlanCapabilityEnabled())
			require.Equal(t, test.subagent, config.SubagentCapabilityEnabled())
		})
	}
}

func TestNormalizeNewDeerFlowRunConfigResolvesRequestedPolicies(t *testing.T) {
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}
	tests := []struct {
		name            string
		requestedPolicy string
		wantMode        DeerFlowMode
		wantSubagent    bool
		wantEffort      string
		wantConcurrency int
	}{
		{
			name:            "auto",
			requestedPolicy: "auto",
			wantMode:        DeerFlowModeUltra,
			wantSubagent:    true,
			wantEffort:      "high",
			wantConcurrency: defaultDeerFlowMaxConcurrentSubagents,
		},
		{
			name:            "pro override",
			requestedPolicy: "pro",
			wantMode:        DeerFlowModePro,
			wantEffort:      "medium",
		},
		{
			name:            "ultra override",
			requestedPolicy: "ultra",
			wantMode:        DeerFlowModeUltra,
			wantSubagent:    true,
			wantEffort:      "high",
			wantConcurrency: defaultDeerFlowMaxConcurrentSubagents,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, config, err := normalizeNewDeerFlowRunConfig(
				`{"requested_policy":"`+test.requestedPolicy+`"}`,
				policy,
			)

			require.NoError(t, err)
			require.Equal(t, test.wantMode, config.Mode)
			require.True(t, config.ThinkingEnabled)
			require.True(t, config.IsPlanMode)
			require.Equal(t, test.wantSubagent, config.SubagentEnabled)
			require.Equal(t, test.wantEffort, config.ReasoningEffort)
			require.Equal(t, test.wantConcurrency, config.MaxConcurrentSubagents)

			var payload map[string]any
			require.NoError(t, json.Unmarshal([]byte(normalized), &payload))
			require.Equal(t, test.requestedPolicy, payload["requested_policy"])
			require.Equal(t, string(test.wantMode), payload["mode"])
		})
	}
}

func TestNormalizeNewDeerFlowRunConfigRejectsRemovedRequestedPolicies(t *testing.T) {
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}

	for _, requestedPolicy := range []string{"flash", "thinking"} {
		t.Run(requestedPolicy, func(t *testing.T) {
			_, _, err := normalizeNewDeerFlowRunConfig(
				`{"requested_policy":"`+requestedPolicy+`"}`,
				policy,
			)

			require.ErrorContains(t, err, "unsupported requested_policy")
		})
	}
}

func TestParseDeerFlowRuntimeConfigRejectsRemovedModes(t *testing.T) {
	for _, mode := range []string{"flash", "thinking", "Auto", "Ask", "Agent"} {
		t.Run(mode, func(t *testing.T) {
			_, err := ParseDeerFlowRuntimeConfig(`{"mode":"` + mode + `"}`)

			require.ErrorContains(t, err, "unsupported DeerFlow mode")
		})
	}
}

func TestParseDeerFlowRuntimeConfigAppliesBoundedOverrides(t *testing.T) {
	config, err := ParseDeerFlowRuntimeConfig(`{
		"mode":"ultra",
		"thinking_enabled":false,
		"is_plan_mode":false,
		"subagent_enabled":true,
		"reasoning_effort":"minimal",
		"max_concurrent_subagents":2
	}`)

	require.NoError(t, err)
	require.False(t, config.ThinkingEnabled)
	require.False(t, config.IsPlanMode)
	require.True(t, config.SubagentEnabled)
	require.Equal(t, "minimal", config.ReasoningEffort)
	require.Equal(t, 2, config.MaxConcurrentSubagents)
}

func TestParseDeerFlowRuntimeConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		message string
	}{
		{name: "mode", config: `{"mode":"auto-magic"}`, message: "unsupported DeerFlow mode"},
		{name: "effort", config: `{"mode":"pro","reasoning_effort":"extreme"}`, message: "unsupported reasoning_effort"},
		{name: "concurrency low", config: `{"mode":"ultra","max_concurrent_subagents":1}`, message: "max_concurrent_subagents must be between 2 and 4"},
		{name: "concurrency high", config: `{"mode":"ultra","max_concurrent_subagents":5}`, message: "max_concurrent_subagents must be between 2 and 4"},
		{name: "boolean type", config: `{"mode":"pro","is_plan_mode":"yes"}`, message: "is_plan_mode must be a boolean"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseDeerFlowRuntimeConfig(test.config)
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestParseDeerFlowRuntimeConfigPreservesHistoricalUnmarkedCapabilitySemantics(t *testing.T) {
	config, err := ParseDeerFlowRuntimeConfig(`{"subagents":[{"name":"writer"}]}`)

	require.NoError(t, err)
	require.False(t, config.ModeExplicit)
	require.True(t, config.PlanCapabilityEnabled())
	require.True(t, config.SubagentCapabilityEnabled())
	require.Empty(t, config.ExecutionReasoningRequest())
}

func TestParseDeerFlowRuntimeConfigDoesNotInventHistoricalReasoningEffort(t *testing.T) {
	config, err := ParseDeerFlowRuntimeConfig(`{"thinking_enabled":true}`)

	require.NoError(t, err)
	require.Equal(t, ADKReasoningRequest{ThinkingEnabled: true}, config.ExecutionReasoningRequest())
}

func TestNormalizeNewDeerFlowRunConfigCanonicalizesEinoAndPreservesFeatures(t *testing.T) {
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}

	normalized, config, err := normalizeNewDeerFlowRunConfig(`{
		"requested_policy":"auto",
		"skills":{"enabled":true},
		"thinking_enabled":true
	}`, policy)

	require.NoError(t, err)
	require.Equal(t, RuntimeModeEinoADK, config.Runtime)
	require.Equal(t, DeerFlowModeUltra, config.Mode)
	require.True(t, config.ThinkingEnabled)
	require.JSONEq(t, `{
		"runtime":"eino_adk",
		"requested_policy":"auto",
		"mode":"ultra",
		"thinking_enabled":true,
		"reasoning_effort":"high",
		"is_plan_mode":true,
		"subagent_enabled":true,
		"max_concurrent_subagents":3,
		"skills":{"enabled":true}
	}`, normalized)
}

func TestNormalizeNewDeerFlowRunConfigUsesAutomaticPolicyByDefault(t *testing.T) {
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}

	normalized, config, err := normalizeNewDeerFlowRunConfig(`{}`, policy)

	require.NoError(t, err)
	require.Equal(t, DeerFlowModeUltra, config.Mode)
	require.True(t, config.ThinkingEnabled)
	require.True(t, config.IsPlanMode)
	require.True(t, config.SubagentEnabled)
	require.Equal(t, "high", config.ReasoningEffort)
	require.JSONEq(t, `{
		"runtime":"eino_adk",
		"requested_policy":"auto",
		"mode":"ultra",
		"thinking_enabled":true,
		"reasoning_effort":"high",
		"is_plan_mode":true,
		"subagent_enabled":true,
		"max_concurrent_subagents":3
	}`, normalized)
}

func TestNormalizeNewDeerFlowRunConfigRejectsLegacyRuntime(t *testing.T) {
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}

	_, _, err := normalizeNewDeerFlowRunConfig(`{"runtime":"legacy"}`, policy)

	require.ErrorIs(t, err, ErrInvalidRuntimeConfig)
	require.ErrorContains(t, err, "legacy runtime is not selectable for new runs")
}

func TestNormalizeNewDeerFlowRunConfigMergesDeerFlowContextWhitelist(t *testing.T) {
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}

	normalized, config, err := normalizeNewDeerFlowRunConfig(
		`{"requested_policy":"pro","system_prompt":"keep"}`,
		policy,
		`{
			"requested_policy":"ultra",
			"model_name":"deepseek-v4-pro",
			"max_concurrent_subagents":4,
			"unknown_context_key":"drop"
		}`,
	)

	require.NoError(t, err)
	require.Equal(t, DeerFlowModeUltra, config.Mode)
	require.Equal(t, 4, config.MaxConcurrentSubagents)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(normalized), &payload))
	require.Equal(t, "deepseek-v4-pro", payload["model_name"])
	require.Equal(t, "keep", payload["system_prompt"])
	require.NotContains(t, payload, "unknown_context_key")
}

func TestNormalizeNewDeerFlowRunConfigUsesNestedLangGraphContextPrecedence(t *testing.T) {
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}

	_, config, err := normalizeNewDeerFlowRunConfig(`{
		"configurable":{"requested_policy":"pro"},
		"context":{"requested_policy":"pro"}
	}`, policy, `{"requested_policy":"ultra"}`)

	require.NoError(t, err)
	require.Equal(t, DeerFlowModePro, config.Mode)
}
