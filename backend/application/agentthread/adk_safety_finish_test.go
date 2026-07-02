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
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKSafetyFinishMiddlewareSuppressesUnsafeToolCalls(t *testing.T) {
	events := &recordingRunEventSink{}
	middleware := NewADKSafetyFinishMiddleware(
		&RunSummary{ThreadID: 10, RunID: 20},
		events,
	)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.UserMessage("write a file"),
		{
			Role:    schema.Assistant,
			Content: "I will write it now.",
			ToolCalls: []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "write_file",
					Arguments: `{"path":"/tmp/unsafe.md","content":"filtered secret"}`,
				},
			}},
			ResponseMeta: &schema.ResponseMeta{
				FinishReason: "content_filter",
			},
			Extra: map[string]any{
				"content_filter_results": map[string]any{"violence": "filtered"},
			},
		},
	}}

	_, got, err := middleware.AfterModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Len(t, got.Messages, 2)
	require.Empty(t, got.Messages[1].ToolCalls)
	require.Contains(t, got.Messages[1].Content, "safety-related signal")
	require.Contains(t, got.Messages[1].Content, "finish_reason")
	require.Contains(t, got.Messages[1].Content, "content_filter")
	require.NotContains(t, got.Messages[1].Content, "filtered secret")

	termination, ok := got.Messages[1].Extra["safety_termination"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "openai_compatible_content_filter", termination["detector"])
	require.Equal(t, "finish_reason", termination["reason_field"])
	require.Equal(t, "content_filter", termination["reason_value"])
	require.Equal(t, 1, termination["suppressed_tool_call_count"])
	require.Equal(t, []string{"write_file"}, termination["suppressed_tool_call_names"])
	require.NotContains(t, termination, "arguments")

	require.Len(t, events.events, 1)
	require.Equal(t, "middleware:safety_termination", events.events[0].EventType)
	require.Equal(t, int64(10), events.events[0].ThreadID)
	require.Equal(t, int64(20), events.events[0].RunID)
	require.NotContains(t, events.events[0].Payload, "filtered secret")
	require.NotContains(t, events.events[0].Payload, "/tmp/unsafe.md")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(events.events[0].Payload), &payload))
	require.Equal(t, "coze.safety_termination.v1", payload["schema"])
	require.Equal(t, "openai_compatible_content_filter", payload["detector"])
	require.Equal(t, []any{"write_file"}, payload["suppressed_tool_call_names"])
}

func TestADKSafetyFinishMiddlewareDetectsProviderSafetySignals(t *testing.T) {
	cases := []struct {
		name      string
		message   *schema.Message
		detector  string
		field     string
		value     string
		unchanged bool
	}{
		{
			name: "anthropic refusal",
			message: &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{{
					ID:   "call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "search",
						Arguments: `{"q":"x"}`,
					},
				}},
				Extra: map[string]any{"stop_reason": "refusal"},
			},
			detector: "anthropic_refusal",
			field:    "stop_reason",
			value:    "refusal",
		},
		{
			name: "gemini safety",
			message: &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{{
					ID:   "call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"content":"x"}`,
					},
				}},
				ResponseMeta: &schema.ResponseMeta{
					FinishReason: "SAFETY",
				},
			},
			detector: "gemini_safety",
			field:    "finish_reason",
			value:    "SAFETY",
		},
		{
			name: "normal stop",
			message: &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{{
					ID:   "call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"content":"x"}`,
					},
				}},
				ResponseMeta: &schema.ResponseMeta{
					FinishReason: "tool_calls",
				},
			},
			unchanged: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			middleware := NewADKSafetyFinishMiddleware(
				&RunSummary{ThreadID: 10, RunID: 20},
				nil,
			)
			_, got, err := middleware.AfterModelRewriteState(
				context.Background(),
				&adk.ChatModelAgentState{Messages: []*schema.Message{tc.message}},
				&adk.ModelContext{},
			)

			require.NoError(t, err)
			if tc.unchanged {
				require.Len(t, got.Messages[0].ToolCalls, 1)
				require.Nil(t, got.Messages[0].Extra["safety_termination"])
				return
			}

			require.Empty(t, got.Messages[0].ToolCalls)
			termination, ok := got.Messages[0].Extra["safety_termination"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, tc.detector, termination["detector"])
			require.Equal(t, tc.field, termination["reason_field"])
			require.Equal(t, tc.value, termination["reason_value"])
		})
	}
}

func TestADKSafetyFinishMiddlewarePrecedesSemanticLoop(t *testing.T) {
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareToolErrorNormalization),
		adkMiddlewareIndex(ADKMiddlewareSafetyFinish),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareSafetyFinish),
		adkMiddlewareIndex(ADKMiddlewareSemanticLoop),
	)
}
