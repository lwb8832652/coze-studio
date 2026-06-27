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
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/coze-dev/coze-studio/backend/api/model/app/bot_common"
	crossagent "github.com/coze-dev/coze-studio/backend/crossdomain/agent/model"
	saEntity "github.com/coze-dev/coze-studio/backend/domain/agent/singleagent/entity"
	"github.com/stretchr/testify/require"
)

func TestADKSingleAgentSubagentAgentFactoryBuildsRunnableChildAgent(
	t *testing.T,
) {
	modelID := int64(3001)
	temperature := 0.25
	maxTokens := int32(768)
	topP := 0.7
	prompt := "You are a focused research subagent."
	source := &recordingSingleAgentDefinitionService{
		versions: map[string]*saEntity.SingleAgent{
			"1001:v3": {
				SingleAgent: &crossagent.SingleAgent{
					AgentID: 1001,
					Name:    "Research Display Name",
					Desc:    "Research public information.",
					Version: "v3",
					ModelInfo: &bot_common.ModelInfo{
						ModelId:     &modelID,
						Temperature: &temperature,
						MaxTokens:   &maxTokens,
						TopP:        &topP,
					},
					Prompt: &bot_common.PromptInfo{Prompt: &prompt},
				},
			},
		},
	}
	childModel := &recordingChatModel{
		resp: schema.AssistantMessage("child complete", nil),
	}
	var gotModelID int64
	appFactory := NewApplicationADKAgentFactory(
		func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			gotModelID = modelID
			return childModel, true, nil
		},
		nil,
		nil,
	)
	factory := NewADKSingleAgentSubagentAgentFactory(source, appFactory)
	provider := NewADKSubagentToolProvider(
		nil,
		ADKSubagentDefinitionProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentDefinition, error) {
			return []ADKSubagentDefinition{{
				Name:        "researcher",
				Description: "Research public information.",
				AgentID:     1001,
				Version:     "v3",
			}}, nil
		}),
		factory,
	)

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{
			RunID:     20,
			ThreadID:  10,
			SpaceID:   7,
			CreatorID: 9,
		},
	)
	require.NoError(t, err)
	researcher := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"researcher",
	)

	result, err := researcher.InvokableRun(
		context.Background(),
		`{"request":"find context"}`,
	)

	require.NoError(t, err)
	require.Contains(t, result, "child complete")
	require.Equal(t, int64(3001), gotModelID)
	require.Len(t, childModel.messages, 2)
	require.Equal(t, schema.System, childModel.messages[0].Role)
	require.Equal(t, prompt, childModel.messages[0].Content)
	require.Equal(t, schema.User, childModel.messages[1].Role)
	require.NotNil(t, childModel.options.Temperature)
	require.InDelta(t, float32(0.25), *childModel.options.Temperature, 0.0001)
	require.NotNil(t, childModel.options.MaxTokens)
	require.Equal(t, 768, *childModel.options.MaxTokens)
	require.NotNil(t, childModel.options.TopP)
	require.InDelta(t, float32(0.7), *childModel.options.TopP, 0.0001)
}

func TestADKSingleAgentSubagentRunSummaryMapsStableSnapshotFields(
	t *testing.T,
) {
	modelID := int64(3002)
	temperature := 0.35
	maxTokens := int32(512)
	topP := 0.8
	prompt := "Summarize only verified facts."
	child, err := buildADKSingleAgentSubagentRunSummary(
		&RunSummary{
			RunID:       20,
			ThreadID:    10,
			SpaceID:     7,
			CreatorID:   9,
			AssistantID: "lead",
			Config:      `{"model_id":9999,"web_tools":{"enabled":true}}`,
		},
		ADKSubagentDefinition{
			Name:                "writer",
			Description:         "Write concise summaries.",
			AgentID:             1002,
			Version:             "v4",
			AllowedTools:        []string{"read_file"},
			AllowedDynamicTools: []string{"search_docs"},
		},
		&saEntity.SingleAgent{
			SingleAgent: &crossagent.SingleAgent{
				AgentID: 1002,
				ModelInfo: &bot_common.ModelInfo{
					ModelId:     &modelID,
					Temperature: &temperature,
					MaxTokens:   &maxTokens,
					TopP:        &topP,
				},
				Prompt: &bot_common.PromptInfo{Prompt: &prompt},
			},
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(20), child.RunID)
	require.Equal(t, int64(10), child.ThreadID)
	require.Equal(t, int64(7), child.SpaceID)
	require.Equal(t, int64(9), child.CreatorID)
	require.Equal(t, "singleagent:1002", child.AssistantID)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(child.Config), &payload))
	require.Equal(t, float64(3002), payload["model_id"])
	require.Equal(t, "writer", payload["agent_name"])
	require.Equal(t, "Write concise summaries.", payload["agent_description"])
	require.Equal(t, prompt, payload["system_prompt"])
	require.Equal(t, 0.35, payload["temperature"])
	require.Equal(t, float64(512), payload["max_tokens"])
	require.Equal(t, 0.8, payload["top_p"])
	require.NotContains(t, payload, "web_tools")
	require.Equal(t, map[string]any{
		"allowed_tools":         []any{"read_file"},
		"allowed_dynamic_tools": []any{"search_docs"},
	}, payload["tool_policy"])

	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(child.Metadata), &metadata))
	require.Equal(t, "single_agent_subagent", metadata["source"])
	require.Equal(t, float64(1002), metadata["agent_id"])
	require.Equal(t, "v4", metadata["version"])
	require.Equal(t, false, metadata["is_draft"])
	require.Equal(t, map[string]any{
		"name":        "writer",
		"root_name":   "lead",
		"parent_name": "lead",
		"step_id":     "lead/writer",
		"run_path":    []any{"lead", "writer"},
		"depth":       float64(1),
	}, metadata["subagent"])
}

func TestADKSingleAgentSubagentRunSummaryDefaultsToNoInheritedTools(
	t *testing.T,
) {
	child, err := buildADKSingleAgentSubagentRunSummary(
		&RunSummary{RunID: 20, Config: `{"agent_name":"lead"}`},
		ADKSubagentDefinition{
			Name:        "researcher",
			Description: "Research public information.",
			AgentID:     1001,
		},
		singleAgentDefinitionFixture(
			1001,
			"Researcher",
			"Research public information.",
			"",
		),
	)

	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(child.Config), &payload))
	require.Equal(t, map[string]any{
		"allowed_tools":         []any{},
		"allowed_dynamic_tools": []any{},
	}, payload["tool_policy"])
}

func TestADKSingleAgentSubagentAgentFactoryRejectsMissingSnapshot(t *testing.T) {
	factory := NewADKSingleAgentSubagentAgentFactory(
		&recordingSingleAgentDefinitionService{},
		ADKAgentFactoryFunc(func(
			context.Context,
			*RunSummary,
		) (adk.ResumableAgent, error) {
			return nil, nil
		}),
	)

	agent, err := factory.BuildADKSubagent(
		context.Background(),
		&RunSummary{RunID: 20},
		ADKSubagentDefinition{
			Name:        "researcher",
			Description: "Research public information.",
			AgentID:     1001,
			Version:     "v1",
		},
	)

	require.ErrorContains(t, err, "subagent not found")
	require.Nil(t, agent)
}
