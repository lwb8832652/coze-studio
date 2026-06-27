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
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKSemanticLoopConfigDefaultDisabled(t *testing.T) {
	config, err := adkSemanticLoopConfigFromRun(&RunSummary{})

	require.NoError(t, err)
	require.Zero(t, config.MaxRepeatedToolCalls)
	require.Zero(t, config.MaxRepeatedAssistantMessages)
}

func TestADKSemanticLoopConfigParsesRunConfig(t *testing.T) {
	config, err := adkSemanticLoopConfigFromRun(&RunSummary{
		Config: `{
			"semantic_loop":{
				"max_repeated_tool_calls":2,
				"max_repeated_assistant_messages":3
			}
		}`,
	})

	require.NoError(t, err)
	require.Equal(t, 2, config.MaxRepeatedToolCalls)
	require.Equal(t, 3, config.MaxRepeatedAssistantMessages)
}

func TestADKSemanticLoopConfigRejectsOversizedLimit(t *testing.T) {
	_, err := adkSemanticLoopConfigFromRun(&RunSummary{
		Config: `{
			"semantic_loop":{
				"max_repeated_tool_calls":21
			}
		}`,
	})

	require.ErrorContains(t, err, "semantic loop max_repeated_tool_calls must be between 0 and 20")
}

func TestADKSemanticLoopMiddlewareRejectsRepeatedToolCalls(t *testing.T) {
	middleware := NewADKSemanticLoopMiddleware(ADKSemanticLoopConfig{
		MaxRepeatedToolCalls: 1,
	})
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.UserMessage("research deployment"),
		semanticLoopToolCallMessage("call-1", "search_docs", `{"b":2,"a":1}`),
		{
			Role:       schema.Tool,
			ToolCallID: "call-1",
			ToolName:   "search_docs",
			Content:    "first result",
		},
		semanticLoopToolCallMessage("call-2", "search_docs", `{"a":1,"b":2}`),
	}}

	_, got, err := middleware.AfterModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.Same(t, state, got)
	var loopErr *ADKSemanticLoopError
	require.ErrorAs(t, err, &loopErr)
	require.Equal(t, ADKSemanticLoopKindToolCalls, loopErr.Kind)
	require.Equal(t, 2, loopErr.Count)
	require.Equal(t, 1, loopErr.Limit)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, loopErr.Signature)
}

func TestADKSemanticLoopMiddlewareRejectsRepeatedAssistantText(t *testing.T) {
	middleware := NewADKSemanticLoopMiddleware(ADKSemanticLoopConfig{
		MaxRepeatedAssistantMessages: 1,
	})
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.UserMessage("summarize"),
		schema.AssistantMessage("same text", nil),
		schema.AssistantMessage(" same\ntext ", nil),
	}}

	_, got, err := middleware.AfterModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.Same(t, state, got)
	var loopErr *ADKSemanticLoopError
	require.ErrorAs(t, err, &loopErr)
	require.Equal(t, ADKSemanticLoopKindAssistantMessage, loopErr.Kind)
	require.Equal(t, 2, loopErr.Count)
	require.Equal(t, 1, loopErr.Limit)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, loopErr.Signature)
}

func TestADKSemanticLoopMiddlewareStopsAtLatestUserTurn(t *testing.T) {
	middleware := NewADKSemanticLoopMiddleware(ADKSemanticLoopConfig{
		MaxRepeatedToolCalls: 1,
	})
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.UserMessage("first"),
		semanticLoopToolCallMessage("call-1", "search_docs", `{"query":"same"}`),
		{
			Role:       schema.Tool,
			ToolCallID: "call-1",
			ToolName:   "search_docs",
			Content:    "result",
		},
		schema.UserMessage("second"),
		semanticLoopToolCallMessage("call-2", "search_docs", `{"query":"same"}`),
	}}

	_, got, err := middleware.AfterModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Same(t, state, got)
}

func semanticLoopToolCallMessage(
	callID string,
	name string,
	arguments string,
) *schema.Message {
	return schema.AssistantMessage("", []schema.ToolCall{{
		ID:   callID,
		Type: "function",
		Function: schema.FunctionCall{
			Name:      name,
			Arguments: arguments,
		},
	}})
}
