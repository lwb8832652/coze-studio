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

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestModelMemoryExtractorParsesStructuredFacts(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage(`{
			"facts":[
				{
					"key":"preferred_language",
					"scope":"long_term",
					"content":"用户偏好中文回答",
					"metadata":{"category":"preference"},
					"score":0.75,
					"confidence":0.91
				}
			]
		}`, nil),
	}
	extractor := NewModelMemoryExtractor(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		ModelMemoryExtractorOptions{
			ModelID:     123,
			ModelName:   "memory-model",
			MaxFacts:    4,
			MaxTokens:   intPtr(512),
			Temperature: float32Ptr(0),
		},
	)

	facts, err := extractor.ExtractMemories(context.Background(), MemoryExtractionRequest{
		ThreadID:       10,
		RunID:          20,
		SnapshotID:     501,
		Kind:           TranscriptKindTerminal,
		Digest:         "digest",
		IdempotencyKey: "terminal:digest",
		MessageCount:   2,
		Messages:       `[{"role":"user","content":"请记住我偏好中文回答"}]`,
		Metadata:       `{"runtime":"eino_adk"}`,
	})

	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, "preferred_language", facts[0].Key)
	require.Equal(t, MemoryScopeLongTerm, facts[0].Scope)
	require.Equal(t, "用户偏好中文回答", facts[0].Content)
	require.JSONEq(t, `{"category":"preference"}`, facts[0].Metadata)
	require.Equal(t, 0.75, facts[0].Score)
	require.Equal(t, 0.91, facts[0].Confidence)
	require.Equal(t, int64(123), chatModel.messages[1].Extra["memory_model_id"])
	require.NotNil(t, chatModel.options.Model)
	require.Equal(t, "memory-model", *chatModel.options.Model)
	require.NotContains(t, chatModel.messages[0].Content, "agent-runtime")
}

func TestModelMemoryExtractorRejectsInvalidModelJSON(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("not json", nil),
	}
	extractor := NewModelMemoryExtractor(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		ModelMemoryExtractorOptions{},
	)

	facts, err := extractor.ExtractMemories(context.Background(), MemoryExtractionRequest{
		Messages: `[{"role":"user","content":"secret transcript body"}]`,
	})

	require.ErrorContains(t, err, "decode memory extraction model output")
	require.Nil(t, facts)
}

func TestModelMemoryExtractorRecordsTokenUsageWithoutContentLeak(t *testing.T) {
	collector := &recordingADKUsageCollector{}
	chatModel := &recordingChatModel{
		resp: &schema.Message{
			Role:    schema.Assistant,
			Content: `{"facts":[]}`,
			ResponseMeta: &schema.ResponseMeta{
				Usage: &schema.TokenUsage{
					PromptTokens:     11,
					CompletionTokens: 3,
					TotalTokens:      14,
					PromptTokenDetails: schema.PromptTokenDetails{
						CachedTokens: 2,
					},
					CompletionTokensDetails: schema.CompletionTokensDetails{
						ReasoningTokens: 1,
					},
				},
				FinishReason: "stop",
			},
		},
	}
	extractor := NewModelMemoryExtractor(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		ModelMemoryExtractorOptions{
			ModelName:      "memory-model",
			UsageCollector: collector,
		},
	)

	facts, err := extractor.ExtractMemories(context.Background(), MemoryExtractionRequest{
		ThreadID:       10,
		RunID:          20,
		SpaceID:        30,
		SnapshotID:     501,
		Kind:           TranscriptKindTerminal,
		Digest:         "digest",
		IdempotencyKey: "terminal:digest",
		MessageCount:   1,
		Messages:       `[{"role":"user","content":"secret transcript body"}]`,
		Metadata:       `{"runtime":"eino_adk"}`,
	})

	require.NoError(t, err)
	require.Empty(t, facts)
	require.Len(t, collector.usages, 1)
	usage := collector.usages[0]
	require.Equal(t, TokenUsageSourceMiddleware, usage.Source)
	require.Equal(t, "memory_extract:501", usage.StepID)
	require.Equal(t, "memory_extractor", usage.StepName)
	require.Equal(t, "memory-model", usage.ModelName)
	require.Equal(t, int64(11), usage.InputTokens)
	require.Equal(t, int64(3), usage.OutputTokens)
	require.Equal(t, int64(14), usage.TotalTokens)
	require.JSONEq(t, `{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14,"cached_tokens":2,"reasoning_tokens":1}`, usage.RawUsage)
	require.Contains(t, usage.Metadata, `"usage_kind":"memory_extraction"`)
	require.Contains(t, usage.Metadata, `"snapshot_id":501`)
	require.NotContains(t, usage.Metadata, "secret transcript body")
	require.NotContains(t, usage.Metadata, "facts")
}

func TestModelMemoryExtractorFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentMemoryExtractorEnabledEnv, "")

	extractor := NewModelMemoryExtractorFromEnv(&recordingADKUsageCollector{})

	require.Nil(t, extractor)
}

func TestModelMemoryExtractorFromEnvBuildsConfiguredExtractor(t *testing.T) {
	t.Setenv(agentMemoryExtractorEnabledEnv, "true")
	t.Setenv(agentMemoryExtractorModelIDEnv, "123")
	t.Setenv(agentMemoryExtractorModelNameEnv, "memory-model")
	t.Setenv(agentMemoryExtractorTemperatureEnv, "0.1")
	t.Setenv(agentMemoryExtractorTopPEnv, "0.8")
	t.Setenv(agentMemoryExtractorMaxTokensEnv, "512")
	t.Setenv(agentMemoryExtractorMaxFactsEnv, "7")
	collector := &recordingADKUsageCollector{}

	extractor := NewModelMemoryExtractorFromEnv(collector)

	require.NotNil(t, extractor)
	require.Equal(t, int64(123), extractor.options.ModelID)
	require.Equal(t, "memory-model", extractor.options.ModelName)
	require.NotNil(t, extractor.options.Temperature)
	require.InDelta(t, 0.1, *extractor.options.Temperature, 0.0001)
	require.NotNil(t, extractor.options.TopP)
	require.InDelta(t, 0.8, *extractor.options.TopP, 0.0001)
	require.NotNil(t, extractor.options.MaxTokens)
	require.Equal(t, 512, *extractor.options.MaxTokens)
	require.Equal(t, 7, extractor.options.MaxFacts)
	require.Same(t, collector, extractor.options.UsageCollector)
}

func intPtr(value int) *int {
	return &value
}

func float32Ptr(value float32) *float32 {
	return &value
}
