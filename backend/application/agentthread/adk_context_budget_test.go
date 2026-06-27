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
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKContextBudgetDefaults(t *testing.T) {
	budget, err := adkContextBudgetFromRun(&RunSummary{})

	require.NoError(t, err)
	require.Equal(t, 128000, budget.ContextWindowTokens)
	require.Equal(t, 120000, budget.SummarizationTokens)
	require.Equal(t, 200, budget.SummarizationMessages)
	require.Equal(t, 4000, budget.MemoryTokens)
	require.Equal(t, 4000, budget.SkillCatalogTokens)
	require.Equal(t, 16000, budget.SkillContentTokens)
	require.Equal(t, 12000, budget.ToolDefinitionTokens)
	require.Equal(t, 16000, budget.MultimodalHistoryTokens)
	require.Equal(t, 8000, budget.FileHistoryTokens)
}

func TestADKContextBudgetReadsRuntimeOverrides(t *testing.T) {
	budget, err := adkContextBudgetFromRun(&RunSummary{Config: `{
		"context_budget":{
			"context_window_tokens":64000,
			"summarization_tokens":48000,
			"summarization_messages":80,
			"memory_tokens":1200,
			"skill_catalog_tokens":900,
			"skill_content_tokens":8000,
			"tool_definition_tokens":6000,
			"multimodal_history_tokens":12000,
			"file_history_tokens":6000
		}
	}`})

	require.NoError(t, err)
	require.Equal(t, 64000, budget.ContextWindowTokens)
	require.Equal(t, 48000, budget.SummarizationTokens)
	require.Equal(t, 80, budget.SummarizationMessages)
	require.Equal(t, 1200, budget.MemoryTokens)
	require.Equal(t, 900, budget.SkillCatalogTokens)
	require.Equal(t, 8000, budget.SkillContentTokens)
	require.Equal(t, 6000, budget.ToolDefinitionTokens)
	require.Equal(t, 12000, budget.MultimodalHistoryTokens)
	require.Equal(t, 6000, budget.FileHistoryTokens)
}

func TestADKContextBudgetClampsUnspecifiedComponentDefaultsToWindow(t *testing.T) {
	budget, err := adkContextBudgetFromRun(&RunSummary{Config: `{
		"context_budget":{
			"context_window_tokens":10000,
			"summarization_tokens":2000,
			"memory_tokens":32
		}
	}`})

	require.NoError(t, err)
	require.Equal(t, 4000, budget.SkillCatalogTokens)
	require.Equal(t, 9999, budget.SkillContentTokens)
	require.Equal(t, 9999, budget.ToolDefinitionTokens)
	require.Equal(t, 9999, budget.MultimodalHistoryTokens)
	require.Equal(t, 8000, budget.FileHistoryTokens)
}

func TestADKContextBudgetRejectsInvalidRelationships(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		errString string
	}{
		{
			name: "summary threshold exceeds window",
			config: `{
				"context_budget":{
					"context_window_tokens":32000,
					"summarization_tokens":40000
				}
			}`,
			errString: "summarization tokens",
		},
		{
			name: "memory exceeds summary threshold",
			config: `{
				"context_budget":{
					"summarization_tokens":1000,
					"memory_tokens":2000
				}
			}`,
			errString: "memory tokens",
		},
		{
			name: "explicit negative value",
			config: `{
				"context_budget":{
					"memory_tokens":-1
				}
			}`,
			errString: "memory tokens",
		},
		{
			name: "skill catalog exceeds context",
			config: `{
				"context_budget":{
					"context_window_tokens":20000,
					"summarization_tokens":18000,
					"memory_tokens":1000,
					"skill_catalog_tokens":21000
				}
			}`,
			errString: "skill catalog tokens",
		},
		{
			name: "skill content is negative",
			config: `{
				"context_budget":{
					"skill_content_tokens":-1
				}
			}`,
			errString: "skill content tokens",
		},
		{
			name: "tool definitions exceed context",
			config: `{
				"context_budget":{
					"context_window_tokens":20000,
					"summarization_tokens":18000,
					"memory_tokens":1000,
					"tool_definition_tokens":21000
				}
			}`,
			errString: "tool definition tokens",
		},
		{
			name: "multimodal history exceeds context",
			config: `{
				"context_budget":{
					"context_window_tokens":20000,
					"summarization_tokens":18000,
					"memory_tokens":1000,
					"multimodal_history_tokens":20000
				}
			}`,
			errString: "multimodal history tokens",
		},
		{
			name: "file history is negative",
			config: `{
				"context_budget":{
					"file_history_tokens":-1
				}
			}`,
			errString: "file history tokens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adkContextBudgetFromRun(&RunSummary{Config: tt.config})
			require.ErrorContains(t, err, tt.errString)
		})
	}
}

func TestEstimateADKTextTokensUsesConservativeDeterministicFallback(t *testing.T) {
	require.Zero(t, estimateADKTextTokens(""))
	require.Equal(t, 1, estimateADKTextTokens("abcd"))
	require.Equal(t, 2, estimateADKTextTokens("abcde"))
	require.Equal(t, 2, estimateADKTextTokens("你好"))
	require.Equal(t, 3, estimateADKTextTokens("a你好"))
}

func TestEstimateADKMessagesTokensIncludesToolDefinitionsAndCalls(t *testing.T) {
	base := []*schema.Message{
		schema.UserMessage("analyze this task"),
	}
	withToolCall := append(append([]*schema.Message{}, base...), &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "search_docs",
				Arguments: `{"query":"deployment plan"}`,
			},
		}},
	})
	tools := []*schema.ToolInfo{{
		Name: "search_docs",
		Desc: "Search deployment documents.",
	}}
	toolsWithExtra := []*schema.ToolInfo{{
		Name: "search_docs",
		Desc: "Search deployment documents.",
		Extra: map[string]any{
			"provider_schema": strings.Repeat("x", 80),
		},
	}}

	baseTokens := estimateADKMessagesTokens(base, nil)
	callTokens := estimateADKMessagesTokens(withToolCall, nil)
	fullTokens := estimateADKMessagesTokens(withToolCall, tools)
	fullWithExtraTokens := estimateADKMessagesTokens(
		withToolCall,
		toolsWithExtra,
	)

	require.Greater(t, callTokens, baseTokens)
	require.Greater(t, fullTokens, callTokens)
	require.Greater(t, fullWithExtraTokens, fullTokens)
	require.Equal(t, fullTokens, estimateADKMessagesTokens(withToolCall, tools))
}

func TestEstimateADKMessageTokenBreakdownUsesStableMediaUnits(t *testing.T) {
	shortBase64 := "aW1hZ2U="
	longBase64 := strings.Repeat("a", 16000)
	newMessage := func(base64Data string) *schema.Message {
		return &schema.Message{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						Base64Data: &base64Data,
						MIMEType:   "image/png",
					},
					Detail: schema.ImageURLDetailHigh,
				},
			}, {
				Type: schema.ChatMessagePartTypeFileURL,
				File: &schema.MessageInputFile{
					MessagePartCommon: schema.MessagePartCommon{
						Base64Data: &base64Data,
						MIMEType:   "application/pdf",
					},
					Name: "report.pdf",
				},
			}},
		}
	}

	shortEstimate := estimateADKMessageTokenBreakdown([]*schema.Message{
		newMessage(shortBase64),
	})
	longEstimate := estimateADKMessageTokenBreakdown([]*schema.Message{
		newMessage(longBase64),
	})

	require.Equal(t, shortEstimate, longEstimate)
	require.GreaterOrEqual(t, shortEstimate.MultimodalTokens, 1536)
	require.GreaterOrEqual(t, shortEstimate.FileTokens, 2048)
	require.Greater(t, shortEstimate.TextTokens, 0)
	require.Equal(
		t,
		shortEstimate.TextTokens+
			shortEstimate.MultimodalTokens+
			shortEstimate.FileTokens,
		estimateADKMessagesTokens([]*schema.Message{
			newMessage(shortBase64),
		}, nil),
	)

	shortDataURL := "data:image/png;base64,aW1hZ2U="
	longDataURL := "data:image/png;base64," + strings.Repeat("a", 16000)
	newDataURLMessage := func(dataURL string) *schema.Message {
		return &schema.Message{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL:      &dataURL,
						MIMEType: "image/png",
					},
				},
			}},
		}
	}
	require.Equal(
		t,
		estimateADKMessageTokenBreakdown([]*schema.Message{
			newDataURLMessage(shortDataURL),
		}),
		estimateADKMessageTokenBreakdown([]*schema.Message{
			newDataURLMessage(longDataURL),
		}),
	)
}
