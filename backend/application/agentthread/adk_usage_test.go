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
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKUsageAttributesLeadSubagentAndMiddlewareCalls(t *testing.T) {
	collector := &recordingADKUsageCollector{}
	bridge := NewADKUsageBridge(&RunSummary{
		RunID:  20,
		Config: `{"agent_name":"lead"}`,
	}, collector)

	recordADKModelUsage(t, bridge, "lead", "lead-call", 0, "chat", "openai", 10, 4, 2, 1)
	recordADKModelUsage(t, bridge, "researcher", "sub-call", 0, "chat", "openai", 8, 3, 0, 2)
	recordADKModelUsage(t, bridge, "summarization", "summary-call", 0, "summarization", "openai", 6, 2, 0, 0)

	require.NoError(t, bridge.Err())
	require.Len(t, collector.usages, 3)
	require.Equal(t, TokenUsageSourceLeadAgent, collector.usages[0].Source)
	require.Equal(t, TokenUsageSourceSubagent, collector.usages[1].Source)
	require.Equal(t, TokenUsageSourceMiddleware, collector.usages[2].Source)
	require.Contains(t, collector.usages[0].Metadata, `"idempotency_key":`)
	require.Contains(t, collector.usages[1].Metadata, `"agent_name":"researcher"`)
	require.Contains(t, collector.usages[2].Metadata, `"usage_kind":"summarization"`)
}

func TestADKUsagePreservesCachedReasoningAndRetryAttempts(t *testing.T) {
	collector := &recordingADKUsageCollector{}
	bridge := NewADKUsageBridge(&RunSummary{
		RunID:  20,
		Config: `{"agent_name":"lead"}`,
	}, collector)

	recordADKModelUsage(t, bridge, "lead", "call-1", 0, "chat", "openai", 20, 10, 7, 4)
	recordADKModelUsage(t, bridge, "lead", "call-1", 1, "chat", "openai", 21, 11, 8, 5)
	recordADKModelUsage(t, bridge, "lead", "call-1", 1, "chat", "openai", 21, 11, 8, 5)

	require.NoError(t, bridge.Err())
	require.Len(t, collector.usages, 2)
	require.Contains(t, collector.usages[0].RawUsage, `"cached_tokens":7`)
	require.Contains(t, collector.usages[0].RawUsage, `"reasoning_tokens":4`)
	require.Contains(t, collector.usages[1].Metadata, `"retry_attempt":1`)
}

func TestADKUsageMetadataIncludesTraceAndModelSnapshot(t *testing.T) {
	collector := &recordingADKUsageCollector{}
	bridge := NewADKUsageBridge(&RunSummary{
		RunID:    20,
		ThreadID: 10,
		SpaceID:  30,
		Config:   `{"agent_name":"lead"}`,
	}, collector)
	handler := bridge.Handler()

	ctx := handler.OnStart(context.Background(), &callbacks.RunInfo{
		Name:      "lead",
		Component: adk.ComponentOfAgent,
	}, &adk.AgentCallbackInput{})
	config := &model.Config{
		Model:       "gpt-test",
		MaxTokens:   512,
		Temperature: 0.2,
		TopP:        0.9,
		Stop:        []string{"END"},
	}
	ctx = handler.OnStart(ctx, &callbacks.RunInfo{
		Name:      "chat",
		Type:      "openai",
		Component: components.ComponentOfChatModel,
	}, &model.CallbackInput{
		Messages: []*schema.Message{schema.UserMessage("secret prompt")},
		Config:   config,
		Extra: map[string]any{
			"model_call_id": "call-trace",
			"usage_kind":    "chat",
		},
	})
	handler.OnEnd(ctx, &callbacks.RunInfo{
		Name:      "chat",
		Type:      "openai",
		Component: components.ComponentOfChatModel,
	}, &model.CallbackOutput{
		Config: config,
		Message: &schema.Message{
			Role:    schema.Assistant,
			Content: "secret output",
			ResponseMeta: &schema.ResponseMeta{
				FinishReason: "stop",
			},
		},
		TokenUsage: &model.TokenUsage{
			PromptTokens:     10,
			CompletionTokens: 4,
			TotalTokens:      14,
		},
		Extra: map[string]any{
			"model_call_id": "call-trace",
			"usage_kind":    "chat",
		},
	})

	require.NoError(t, bridge.Err())
	require.Len(t, collector.usages, 1)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(collector.usages[0].Metadata), &metadata))
	require.Regexp(t, `^[0-9a-f]{32}$`, metadata["trace_id"])
	require.Regexp(t, `^[0-9a-f]{16}$`, metadata["span_id"])
	require.Regexp(t, `^[0-9a-f]{16}$`, metadata["parent_span_id"])
	require.Equal(t, "call-trace", metadata["model_call_id"])
	require.Equal(t, "gpt-test", metadata["model_name"])
	require.Equal(t, "openai", metadata["provider"])

	modelMetadata, ok := metadata["model_metadata"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "gpt-test", modelMetadata["model_name"])
	require.Equal(t, "openai", modelMetadata["provider"])
	require.Equal(t, "ChatModel", modelMetadata["component"])
	require.Equal(t, "chat", modelMetadata["component_name"])
	require.Equal(t, "stop", modelMetadata["finish_reason"])
	require.Equal(t, float64(512), modelMetadata["max_tokens"])
	require.InDelta(t, 0.2, modelMetadata["temperature"], 0.0001)
	require.InDelta(t, 0.9, modelMetadata["top_p"], 0.0001)
	require.Equal(t, float64(1), modelMetadata["stop_count"])

	encodedMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NotContains(t, string(encodedMetadata), "secret prompt")
	require.NotContains(t, string(encodedMetadata), "secret output")
}

func TestADKUsageDoesNotDoubleCountCallbackAndEventUsage(t *testing.T) {
	collector := &recordingADKUsageCollector{}
	run := &RunSummary{
		RunID:  20,
		Config: `{"agent_name":"lead"}`,
	}
	bridge := NewADKUsageBridge(run, collector)

	recordADKModelUsage(t, bridge, "lead", "call-1", 0, "chat", "openai", 10, 4, 2, 1)
	err := bridge.RecordEvent(context.Background(), AgentTokenUsage{
		Source:       TokenUsageSourceLeadAgent,
		StepName:     "lead",
		InputTokens:  10,
		OutputTokens: 4,
		TotalTokens:  14,
		RawUsage:     `{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"cached_tokens":2,"reasoning_tokens":1}`,
	})

	require.NoError(t, err)
	require.Len(t, collector.usages, 1)
}

func TestADKUsageStreamingRecordsFinalUsageOnce(t *testing.T) {
	collector := &recordingADKUsageCollector{}
	bridge := NewADKUsageBridge(&RunSummary{
		RunID:  20,
		Config: `{"agent_name":"lead"}`,
	}, collector)
	handler := bridge.Handler()

	ctx := handler.OnStart(context.Background(), &callbacks.RunInfo{
		Name:      "lead",
		Component: adk.ComponentOfAgent,
	}, &adk.AgentCallbackInput{})
	ctx = handler.OnStart(ctx, &callbacks.RunInfo{
		Name:      "chat",
		Type:      "openai",
		Component: components.ComponentOfChatModel,
	}, &model.CallbackInput{
		Config: &model.Config{Model: "gpt-test"},
		Extra: map[string]any{
			"model_call_id": "stream-call-1",
			"usage_kind":    "chat",
		},
	})
	stream := schema.StreamReaderFromArray([]callbacks.CallbackOutput{
		&model.CallbackOutput{
			Config: &model.Config{Model: "gpt-test"},
			TokenUsage: &model.TokenUsage{
				PromptTokens:     10,
				CompletionTokens: 1,
				TotalTokens:      11,
			},
		},
		&model.CallbackOutput{
			Config: &model.Config{Model: "gpt-test"},
			TokenUsage: &model.TokenUsage{
				PromptTokens:     10,
				CompletionTokens: 4,
				TotalTokens:      14,
			},
			Extra: map[string]any{
				"model_call_id": "stream-call-1",
				"usage_kind":    "chat",
			},
		},
	})
	handler.OnEndWithStreamOutput(ctx, &callbacks.RunInfo{
		Name:      "chat",
		Type:      "openai",
		Component: components.ComponentOfChatModel,
	}, stream)

	err := bridge.RecordEvent(context.Background(), AgentTokenUsage{
		Source:       TokenUsageSourceLeadAgent,
		StepName:     "lead",
		InputTokens:  10,
		OutputTokens: 4,
		TotalTokens:  14,
		RawUsage:     `{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14,"cached_tokens":0,"reasoning_tokens":0}`,
	})

	require.NoError(t, err)
	require.Len(t, collector.usages, 1)
	require.Equal(t, int64(14), collector.usages[0].TotalTokens)
	require.Equal(t, "stream-call-1", collector.usages[0].StepID)
}

func TestADKUsageContextAttributesSummarizationAsMiddleware(t *testing.T) {
	bridge := NewADKUsageBridge(&RunSummary{
		RunID:  20,
		Config: `{"agent_name":"lead"}`,
	}, &recordingADKUsageCollector{})

	call := bridge.newCall(
		withADKUsageKind(context.Background(), "summarization"),
		&callbacks.RunInfo{
			Name:      "chat",
			Type:      "openai",
			Component: components.ComponentOfChatModel,
		},
		&model.Config{Model: "gpt-test"},
		nil,
	)

	require.Equal(t, "summarization", call.usageKind)
	require.Equal(t, TokenUsageSourceMiddleware, bridge.sourceFor(call))
}

func recordADKModelUsage(
	t *testing.T,
	bridge *ADKUsageBridge,
	agentName string,
	callID string,
	retryAttempt int,
	usageKind string,
	provider string,
	inputTokens int,
	outputTokens int,
	cachedTokens int,
	reasoningTokens int,
) {
	t.Helper()

	handler := bridge.Handler()
	ctx := handler.OnStart(context.Background(), &callbacks.RunInfo{
		Name:      agentName,
		Component: adk.ComponentOfAgent,
	}, &adk.AgentCallbackInput{})
	ctx = handler.OnStart(ctx, &callbacks.RunInfo{
		Name:      usageKind,
		Type:      provider,
		Component: components.ComponentOfChatModel,
	}, &model.CallbackInput{
		Config: &model.Config{Model: "gpt-test"},
		Extra: map[string]any{
			"model_call_id": callID,
			"retry_attempt": retryAttempt,
			"usage_kind":    usageKind,
		},
	})
	handler.OnEnd(ctx, &callbacks.RunInfo{
		Name:      usageKind,
		Type:      provider,
		Component: components.ComponentOfChatModel,
	}, &model.CallbackOutput{
		Config: &model.Config{Model: "gpt-test"},
		TokenUsage: &model.TokenUsage{
			PromptTokens:     inputTokens,
			CompletionTokens: outputTokens,
			TotalTokens:      inputTokens + outputTokens,
			PromptTokenDetails: model.PromptTokenDetails{
				CachedTokens: cachedTokens,
			},
			CompletionTokensDetails: model.CompletionTokensDetails{
				ReasoningTokens: reasoningTokens,
			},
		},
		Extra: map[string]any{
			"model_call_id": callID,
			"retry_attempt": retryAttempt,
			"usage_kind":    usageKind,
		},
	})
}

type recordingADKUsageCollector struct {
	usages []AgentTokenUsage
	err    error
}

func (c *recordingADKUsageCollector) Record(_ context.Context, _ *RunSummary, usage AgentTokenUsage) error {
	c.usages = append(c.usages, usage)
	return c.err
}
