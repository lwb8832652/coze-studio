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
	"fmt"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKSubagentLimitDropsOnlyExcessSubagentCalls(t *testing.T) {
	events := &recordingRunEventSink{}
	middleware := NewADKSubagentLimitMiddleware(
		&RunSummary{RunID: 20, ThreadID: 10},
		[]string{"researcher", "writer"},
		2,
		events,
	)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.UserMessage("compare sources"),
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				adkSubagentLimitTestCall("call-search", "search_docs", `{"query":"safe"}`),
				adkSubagentLimitTestCall("call-research", "researcher", `{"request":"secret-one"}`),
				adkSubagentLimitTestCall("call-calc", "calculator", `{"value":1}`),
				adkSubagentLimitTestCall("call-write", "writer", `{"request":"secret-two"}`),
				adkSubagentLimitTestCall("call-research-2", "researcher", `{"request":"secret-three"}`),
				adkSubagentLimitTestCall("call-fetch", "fetch_page", `{"url":"https://example.com"}`),
			},
		},
	}}
	state.Messages[1].ToolCalls[0].Extra = map[string]any{
		"trace": map[string]any{"status": "original"},
	}

	_, got, err := middleware.AfterModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.NotSame(t, state, got)
	require.Equal(t, []string{
		"search_docs",
		"researcher",
		"calculator",
		"writer",
		"fetch_page",
	}, adkSubagentLimitToolCallNames(got.Messages[1].ToolCalls))
	require.Len(t, state.Messages[1].ToolCalls, 6)
	require.Equal(t, "researcher", state.Messages[1].ToolCalls[4].Function.Name)
	require.JSONEq(t, `{"request":"secret-three"}`, state.Messages[1].ToolCalls[4].Function.Arguments)
	got.Messages[1].ToolCalls[0].Extra["trace"].(map[string]any)["status"] = "changed"
	require.Equal(
		t,
		"original",
		state.Messages[1].ToolCalls[0].Extra["trace"].(map[string]any)["status"],
	)
	require.Len(t, events.events, 1)
	require.Equal(t, "subagent.tool_calls_truncated", events.events[0].EventType)
	require.Equal(t, int64(10), events.events[0].ThreadID)
	require.Equal(t, int64(20), events.events[0].RunID)
	require.JSONEq(t, `{
		"schema":"coze.subagent_tool_limit.v1",
		"limit":2,
		"requested_subagent_calls":3,
		"retained_subagent_calls":2,
		"dropped_subagent_calls":1,
		"dropped_tool_names":["researcher"]
	}`, events.events[0].Payload)
	require.NotContains(t, events.events[0].Payload, "secret")
}

func TestADKSubagentLimitLeavesWithinLimitStateUnchanged(t *testing.T) {
	events := &recordingRunEventSink{}
	middleware := NewADKSubagentLimitMiddleware(
		&RunSummary{RunID: 20, ThreadID: 10},
		[]string{"researcher"},
		2,
		events,
	)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{
			adkSubagentLimitTestCall("call-search", "search_docs", `{}`),
			adkSubagentLimitTestCall("call-research", "researcher", `{}`),
		},
	}}}

	_, got, err := middleware.AfterModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Same(t, state, got)
	require.Empty(t, events.events)
}

func TestADKSubagentLimitBoundsDroppedToolNamesInEvent(t *testing.T) {
	events := &recordingRunEventSink{}
	toolNames := make([]string, 0, 20)
	toolCalls := make([]schema.ToolCall, 0, 20)
	for index := 0; index < 20; index++ {
		name := fmt.Sprintf("worker_%02d", index)
		toolNames = append(toolNames, name)
		toolCalls = append(
			toolCalls,
			adkSubagentLimitTestCall(fmt.Sprintf("call-%02d", index), name, `{}`),
		)
	}
	middleware := NewADKSubagentLimitMiddleware(
		&RunSummary{RunID: 20, ThreadID: 10},
		toolNames,
		2,
		events,
	)

	_, _, err := middleware.AfterModelRewriteState(
		context.Background(),
		&adk.ChatModelAgentState{Messages: []*schema.Message{{
			Role:      schema.Assistant,
			ToolCalls: toolCalls,
		}}},
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Len(t, events.events, 1)
	var payload struct {
		DroppedCount int      `json:"dropped_subagent_calls"`
		DroppedNames []string `json:"dropped_tool_names"`
	}
	require.NoError(t, json.Unmarshal([]byte(events.events[0].Payload), &payload))
	require.Equal(t, 18, payload.DroppedCount)
	require.Len(t, payload.DroppedNames, 16)
	require.Equal(t, "worker_02", payload.DroppedNames[0])
	require.Equal(t, "worker_17", payload.DroppedNames[15])
}

func TestADKMiddlewareIncludesSubagentLimitOnlyForEnabledKnownTools(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
		bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
			Run:               &RunSummary{RunID: 20, Config: `{"mode":"ultra","max_concurrent_subagents":2}`},
			Model:             &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
			SubagentToolNames: []string{"researcher"},
		})

		require.NoError(t, err)
		require.Contains(t, bundle.HandlerNames, ADKMiddlewareSubagentLimit)
	})

	t.Run("no known tools", func(t *testing.T) {
		assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
		bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
			Run:   &RunSummary{RunID: 20, Config: `{"mode":"ultra"}`},
			Model: &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
		})

		require.NoError(t, err)
		require.NotContains(t, bundle.HandlerNames, ADKMiddlewareSubagentLimit)
	})

	t.Run("capability disabled", func(t *testing.T) {
		assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{})
		bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
			Run:               &RunSummary{RunID: 20, Config: `{"mode":"pro"}`},
			Model:             &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
			SubagentToolNames: []string{"researcher"},
		})

		require.NoError(t, err)
		require.NotContains(t, bundle.HandlerNames, ADKMiddlewareSubagentLimit)
	})
}

func adkSubagentLimitTestCall(id string, name string, arguments string) schema.ToolCall {
	return schema.ToolCall{
		ID:   id,
		Type: "function",
		Function: schema.FunctionCall{
			Name:      name,
			Arguments: arguments,
		},
	}
}

func adkSubagentLimitToolCallNames(calls []schema.ToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Function.Name)
	}
	return names
}
