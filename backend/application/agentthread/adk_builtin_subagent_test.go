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
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"
)

func TestBuiltinSubagentFactoryInheritsModelAndDisablesRecursiveCapabilities(t *testing.T) {
	var child *RunSummary
	factory := newADKBuiltinSubagentAgentFactory(ADKAgentFactoryFunc(func(
		_ context.Context,
		run *RunSummary,
	) (adk.ResumableAgent, error) {
		child = run
		return &recordingSubagentReplayAgent{}, nil
	}))
	parent := &RunSummary{
		RunID:     20,
		ThreadID:  30,
		SpaceID:   40,
		CreatorID: 50,
		Config: `{
			"runtime":"eino_adk",
			"mode":"ultra",
			"model_id":100002,
			"model_name":"deepseek-v4-pro",
			"thinking_enabled":true,
			"is_plan_mode":true,
			"subagent_enabled":true,
			"web_tools":{"enabled":true,"search":{"enabled":true}},
			"mcp_tools":{"enabled":true,"allowed_tools":["mcp_1_search"]},
			"tool_policy":{
				"allowed_tools":["web_search","ask_clarification","present_files"],
				"allowed_dynamic_tools":["mcp_1_search"]
			},
			"subagent_refs":[{"agent_id":999}],
			"private_parent_overlay":"PRIVATE_PARENT_OVERLAY"
		}`,
	}

	agent, err := factory.BuildADKSubagent(
		context.Background(),
		parent,
		builtinADKGeneralPurposeDefinition(),
	)
	require.NoError(t, err)
	require.NotNil(t, agent)
	require.NotNil(t, child)
	require.Equal(t, RunKindSubagent, child.RunKind)
	require.Equal(t, parent.RunID, child.ParentRunID)
	require.Equal(t, parent.ThreadID, child.ThreadID)

	var config map[string]any
	require.NoError(t, json.Unmarshal([]byte(child.Config), &config))
	require.Equal(t, "eino_adk", config["runtime"])
	require.Equal(t, "pro", config["requested_policy"])
	require.Equal(t, "pro", config["mode"])
	require.Equal(t, false, config["thinking_enabled"])
	require.Equal(t, false, config["is_plan_mode"])
	require.Equal(t, false, config["subagent_enabled"])
	require.Equal(t, map[string]any{
		"allowed_tools": []any{
			"web_search", "ask_clarification", "present_files",
		},
		"allowed_dynamic_tools": []any{"mcp_1_search"},
	}, config["tool_policy"])
	require.Equal(t, map[string]any{
		"enabled": true,
		"search":  map[string]any{"enabled": true},
	}, config["web_tools"])
	require.Equal(t, map[string]any{
		"enabled":       true,
		"allowed_tools": []any{"mcp_1_search"},
	}, config["mcp_tools"])
	require.Equal(t, float64(100002), config["model_id"])
	require.Equal(t, "deepseek-v4-pro", config["model_name"])
	require.NotEmpty(t, config["system_prompt"])
	require.NotContains(t, config, "subagent_refs")
	require.NotContains(t, child.Config, "PRIVATE_PARENT_OVERLAY")

	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(child.Metadata), &metadata))
	require.Equal(t, "builtin_subagent", metadata["source"])

	toolSet, err := newADKBuiltinSubagentToolProvider(
		&recordingADKToolSetProvider{set: ADKToolSet{
			StaticTools: []tool.BaseTool{
				&namedTestTool{name: "web_search"},
				&namedTestTool{name: adkClarificationToolName},
				&namedTestTool{name: adkDeerFlowClarificationToolName},
				&namedTestTool{name: adkConfirmationToolName},
				&namedTestTool{name: adkPresentFilesToolName},
			},
			DynamicTools: []tool.BaseTool{
				&namedTestTool{name: "mcp_1_search"},
			},
		}},
	).ResolveToolSet(context.Background(), child)
	require.NoError(t, err)
	require.Equal(t, []string{"web_search"}, adkToolNames(t, context.Background(), toolSet.StaticTools))
	require.Equal(t, []string{"mcp_1_search"}, adkToolNames(t, context.Background(), toolSet.DynamicTools))
}

func TestBuiltinSubagentDefinitionProviderAlwaysReservesGeneralPurposeName(t *testing.T) {
	provider := newADKBuiltinSubagentDefinitionProvider(
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			return []ADKSubagentDefinition{{
				Name:        adkBuiltinGeneralPurposeName,
				Description: "configured collision",
			}}, nil
		}),
	)

	_, err := provider.ResolveADKSubagents(
		context.Background(),
		&RunSummary{Config: `{"mode":"pro"}`},
	)
	require.ErrorContains(t, err, "reserved by a builtin")
}
