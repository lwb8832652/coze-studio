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
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKToolDefinitionBudgetMiddlewareCountsVisibleAndDeferredTools(t *testing.T) {
	middleware, err := NewADKToolDefinitionBudgetMiddleware(ADKContextBudget{
		ToolDefinitionTokens: 20,
	})
	require.NoError(t, err)

	t.Run("visible overflow", func(t *testing.T) {
		state := &adk.ChatModelAgentState{ToolInfos: []*schema.ToolInfo{{
			Name: "large_visible_tool",
			Desc: strings.Repeat("visible description ", 30),
		}}}

		_, _, err := middleware.BeforeModelRewriteState(
			context.Background(),
			state,
			&adk.ModelContext{},
		)

		require.ErrorContains(t, err, "tool definitions exceed token budget 20")
	})

	t.Run("deferred overflow", func(t *testing.T) {
		state := &adk.ChatModelAgentState{DeferredToolInfos: []*schema.ToolInfo{{
			Name: "large_deferred_tool",
			Desc: strings.Repeat("deferred description ", 30),
		}}}

		_, _, err := middleware.BeforeModelRewriteState(
			context.Background(),
			state,
			&adk.ModelContext{},
		)

		require.ErrorContains(t, err, "tool definitions exceed token budget 20")
	})

	t.Run("within budget", func(t *testing.T) {
		state := &adk.ChatModelAgentState{ToolInfos: []*schema.ToolInfo{{
			Name: "small",
			Desc: "Small tool.",
		}}}

		_, got, err := middleware.BeforeModelRewriteState(
			context.Background(),
			state,
			&adk.ModelContext{},
		)

		require.NoError(t, err)
		require.Same(t, state, got)
	})
}

func TestADKToolBudgetRunsAfterClientToolSearch(t *testing.T) {
	dynamicTool := mustADKBudgetTool(
		t,
		"large_dynamic_tool",
		strings.Repeat("large deferred description ", 1000),
	)
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("done", nil),
	}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run: &RunSummary{
			RunID: 20,
			Config: `{
				"context_budget":{
					"tool_definition_tokens":1000
				}
			}`,
		},
		Model:        chatModel,
		DynamicTools: []tool.BaseTool{dynamicTool},
	})
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "client tool budget test",
		Model:       chatModel,
		Handlers:    bundle.Handlers,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("finish directly")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.False(t, toolDescriptionsContain(
		chatModel.options.Tools,
		"large deferred description",
	))
}

func TestADKToolBudgetRejectsProviderNativeDeferredCatalogOverflow(t *testing.T) {
	dynamicTool := mustADKBudgetTool(
		t,
		"large_dynamic_tool",
		strings.Repeat("large deferred description ", 1000),
	)
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("done", nil),
	}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run: &RunSummary{
			RunID: 20,
			Config: `{
				"context_budget":{
					"tool_definition_tokens":1000
				}
			}`,
		},
		Model:        chatModel,
		DynamicTools: []tool.BaseTool{dynamicTool},
		ModelCapabilities: ADKModelCapabilities{
			NativeToolSearch: true,
		},
	})
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "native deferred budget test",
		Model:       chatModel,
		Handlers:    bundle.Handlers,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("finish directly")},
	})

	require.NotEmpty(t, events)
	require.ErrorContains(
		t,
		events[len(events)-1].Err,
		"tool definitions exceed token budget 1000",
	)
	require.Zero(t, chatModel.calls)
}

func TestADKToolBudgetRejectsOversizedPromotedToolBeforeNextModelCall(t *testing.T) {
	dynamicTool := mustADKBudgetTool(
		t,
		"large_dynamic_tool",
		strings.Repeat("large promoted description ", 1000),
	)
	chatModel := &toolSearchPromotionChatModel{}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run: &RunSummary{
			RunID: 20,
			Config: `{
				"context_budget":{
					"tool_definition_tokens":1000
				}
			}`,
		},
		Model:        chatModel,
		DynamicTools: []tool.BaseTool{dynamicTool},
	})
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "promoted tool budget test",
		Model:       chatModel,
		Handlers:    bundle.Handlers,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("find the large tool")},
	})

	require.NotEmpty(t, events)
	require.ErrorContains(
		t,
		events[len(events)-1].Err,
		"tool definitions exceed token budget 1000",
	)
	require.Equal(t, 1, chatModel.calls)
}

func mustADKBudgetTool(
	t *testing.T,
	name string,
	description string,
) tool.BaseTool {
	t.Helper()
	result, err := toolutils.InferTool(
		name,
		description,
		func(context.Context, struct{}) (string, error) {
			return "ok", nil
		},
	)
	require.NoError(t, err)
	return result
}

type toolSearchPromotionChatModel struct {
	calls int
}

func (m *toolSearchPromotionChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	m.calls++
	if m.calls > 1 {
		return nil, fmt.Errorf("oversized promoted tool reached the model")
	}
	return schema.AssistantMessage("", []schema.ToolCall{{
		ID:   "call-search",
		Type: "function",
		Function: schema.FunctionCall{
			Name:      "tool_search",
			Arguments: `{"query":"select:large_dynamic_tool"}`,
		},
	}}), nil
}

func (m *toolSearchPromotionChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stream is not implemented")
}
