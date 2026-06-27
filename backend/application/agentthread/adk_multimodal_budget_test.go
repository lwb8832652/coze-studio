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
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestProjectADKMultimodalHistoryKeepsCurrentTurnAndNewestHistory(
	t *testing.T,
) {
	oldImageURL := "https://example.test/old.png"
	oldFileURL := "https://example.test/old.pdf"
	middleImageURL := "https://example.test/middle.png"
	middleFileURL := "https://example.test/middle.pdf"
	currentImageURL := "https://example.test/current.png"
	currentFileURL := "https://example.test/current.pdf"
	messages := []*schema.Message{
		adkMultimodalBudgetUserMessage(
			"old",
			oldImageURL,
			oldFileURL,
			"old.pdf",
		),
		adkMultimodalBudgetUserMessage(
			"middle",
			middleImageURL,
			middleFileURL,
			"middle.pdf",
		),
		adkMultimodalBudgetUserMessage(
			"current",
			currentImageURL,
			currentFileURL,
			"current.pdf",
		),
	}
	budget := ADKContextBudget{
		MultimodalHistoryTokens: adkImageLowTokens * 2,
		FileHistoryTokens:       adkFileTokens * 2,
	}

	projection, err := projectADKMultimodalHistory(messages, budget)

	require.NoError(t, err)
	require.Len(t, projection.Messages, 3)
	require.Equal(t, 2, projection.KeptMultimodalParts)
	require.Equal(t, 1, projection.OmittedMultimodalParts)
	require.Equal(t, 2, projection.KeptFileParts)
	require.Equal(t, 1, projection.OmittedFileParts)
	require.Equal(
		t,
		adkImageLowTokens*2,
		projection.EstimatedMultimodalTokens,
	)
	require.Equal(
		t,
		adkFileTokens*2,
		projection.EstimatedFileTokens,
	)

	require.Equal(
		t,
		schema.ChatMessagePartTypeText,
		projection.Messages[0].UserInputMultiContent[0].Type,
	)
	require.Contains(
		t,
		projection.Messages[0].UserInputMultiContent[0].Text,
		"historical image",
	)
	require.NotContains(
		t,
		projection.Messages[0].UserInputMultiContent[0].Text,
		oldImageURL,
	)
	require.Equal(
		t,
		schema.ChatMessagePartTypeText,
		projection.Messages[0].UserInputMultiContent[1].Type,
	)
	require.NotContains(
		t,
		projection.Messages[0].UserInputMultiContent[1].Text,
		"old.pdf",
	)

	require.Equal(
		t,
		middleImageURL,
		*projection.Messages[1].UserInputMultiContent[0].Image.URL,
	)
	require.Equal(
		t,
		"middle.pdf",
		projection.Messages[1].UserInputMultiContent[1].File.Name,
	)
	require.Equal(
		t,
		currentImageURL,
		*projection.Messages[2].UserInputMultiContent[0].Image.URL,
	)
	require.Equal(
		t,
		"current.pdf",
		projection.Messages[2].UserInputMultiContent[1].File.Name,
	)

	require.NotSame(t, messages[0], projection.Messages[0])
	require.Equal(
		t,
		oldImageURL,
		*messages[0].UserInputMultiContent[0].Image.URL,
	)
	require.Equal(
		t,
		"old.pdf",
		messages[0].UserInputMultiContent[1].File.Name,
	)

	*projection.Messages[1].UserInputMultiContent[0].Image.URL =
		"https://example.test/provider-mutated.png"
	projection.Messages[1].UserInputMultiContent[1].File.Name =
		"provider-mutated.pdf"
	require.Equal(
		t,
		middleImageURL,
		*messages[1].UserInputMultiContent[0].Image.URL,
	)
	require.Equal(
		t,
		"middle.pdf",
		messages[1].UserInputMultiContent[1].File.Name,
	)
}

func TestProjectADKMultimodalHistoryRejectsOversizedCurrentTurn(t *testing.T) {
	tests := []struct {
		name      string
		message   *schema.Message
		budget    ADKContextBudget
		category  string
		estimate  int
		limit     int
		partCount int
	}{
		{
			name: "image",
			message: func() *schema.Message {
				url := "https://example.test/current.png"
				return &schema.Message{
					Role: schema.User,
					UserInputMultiContent: []schema.MessageInputPart{{
						Type: schema.ChatMessagePartTypeImageURL,
						Image: &schema.MessageInputImage{
							MessagePartCommon: schema.MessagePartCommon{
								URL: &url,
							},
							Detail: schema.ImageURLDetailHigh,
						},
					}},
				}
			}(),
			budget: ADKContextBudget{
				MultimodalHistoryTokens: 1000,
				FileHistoryTokens:       adkFileTokens,
			},
			category:  "multimodal",
			estimate:  adkImageHighTokens,
			limit:     1000,
			partCount: 1,
		},
		{
			name: "files",
			message: func() *schema.Message {
				url := "https://example.test/current.pdf"
				return &schema.Message{
					Role: schema.User,
					UserInputMultiContent: []schema.MessageInputPart{{
						Type: schema.ChatMessagePartTypeFileURL,
						File: &schema.MessageInputFile{
							MessagePartCommon: schema.MessagePartCommon{
								URL: &url,
							},
							Name: "current.pdf",
						},
					}},
				}
			}(),
			budget: ADKContextBudget{
				MultimodalHistoryTokens: adkImageLowTokens,
				FileHistoryTokens:       1000,
			},
			category:  "file",
			estimate:  adkFileTokens,
			limit:     1000,
			partCount: 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			projection, err := projectADKMultimodalHistory(
				[]*schema.Message{testCase.message},
				testCase.budget,
			)

			require.Nil(t, projection)
			var budgetErr *ADKProtectedMultimodalBudgetError
			require.True(t, errors.As(err, &budgetErr))
			require.Equal(t, testCase.category, budgetErr.Category)
			require.Equal(t, testCase.estimate, budgetErr.EstimatedTokens)
			require.Equal(t, testCase.limit, budgetErr.TokenLimit)
			require.Equal(t, testCase.partCount, budgetErr.PartCount)
			require.NotContains(t, err.Error(), "example.test")
			require.NotContains(t, err.Error(), "current.pdf")
		})
	}
}

func TestProjectADKMultimodalHistoryDoesNotLeakOmittedBase64(t *testing.T) {
	base64Data := "private-inline-media-data"
	messages := []*schema.Message{{
		Role: schema.User,
		UserInputMultiContent: []schema.MessageInputPart{{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{
					Base64Data: &base64Data,
					MIMEType:   "image/png",
				},
				Detail: schema.ImageURLDetailLow,
			},
		}},
	}, {
		Role:    schema.User,
		Content: "newest text-only turn",
	}}

	projection, err := projectADKMultimodalHistory(
		messages,
		ADKContextBudget{
			MultimodalHistoryTokens: 1,
			FileHistoryTokens:       adkFileTokens,
		},
	)

	require.NoError(t, err)
	require.Equal(t, 1, projection.OmittedMultimodalParts)
	require.Nil(t, projection.Messages[0].UserInputMultiContent[0].Image)
	require.NotContains(
		t,
		projection.Messages[0].UserInputMultiContent[0].Text,
		base64Data,
	)
	require.Equal(
		t,
		base64Data,
		*messages[0].UserInputMultiContent[0].Image.Base64Data,
	)
}

func TestProjectADKMultimodalHistoryDeepClonesJSONCompatibleMetadata(
	t *testing.T,
) {
	imageURL := "https://example.test/current.png"
	messages := []*schema.Message{{
		Role: schema.User,
		Extra: map[string]any{
			"nested": map[string]any{"status": "source"},
			"items":  []any{map[string]any{"status": "source"}},
		},
		UserInputMultiContent: []schema.MessageInputPart{{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{
					URL: &imageURL,
					Extra: map[string]any{
						"nested": map[string]any{"status": "source"},
					},
				},
				Detail: schema.ImageURLDetailLow,
			},
		}},
	}}

	projection, err := projectADKMultimodalHistory(
		messages,
		ADKContextBudget{
			MultimodalHistoryTokens: adkImageLowTokens,
			FileHistoryTokens:       adkFileTokens,
		},
	)
	require.NoError(t, err)

	projection.Messages[0].Extra["nested"].(map[string]any)["status"] =
		"provider-mutated"
	projection.Messages[0].Extra["items"].([]any)[0].(map[string]any)["status"] =
		"provider-mutated"
	projection.Messages[0].UserInputMultiContent[0].Image.Extra["nested"].(map[string]any)["status"] = "provider-mutated"

	require.Equal(
		t,
		"source",
		messages[0].Extra["nested"].(map[string]any)["status"],
	)
	require.Equal(
		t,
		"source",
		messages[0].Extra["items"].([]any)[0].(map[string]any)["status"],
	)
	require.Equal(
		t,
		"source",
		messages[0].UserInputMultiContent[0].Image.Extra["nested"].(map[string]any)["status"],
	)
}

func TestADKMultimodalBudgetMiddlewareProjectsModelInputWithoutMutatingState(
	t *testing.T,
) {
	oldURL := "https://example.test/old.png"
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL: &oldURL,
					},
					Detail: schema.ImageURLDetailLow,
				},
			}},
		}, schema.UserMessage("current text-only request")},
	}
	events := &recordingRunEventSink{}
	middleware, err := NewADKMultimodalBudgetMiddleware(
		&RunSummary{ThreadID: 10, RunID: 20},
		ADKContextBudget{
			MultimodalHistoryTokens: 1,
			FileHistoryTokens:       adkFileTokens,
		},
		events,
	)
	require.NoError(t, err)

	modelCtx, gotState, err := middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)
	require.NoError(t, err)
	require.Same(t, state, gotState)
	require.Equal(
		t,
		oldURL,
		*state.Messages[0].UserInputMultiContent[0].Image.URL,
	)
	require.Equal(t, []string{"context.multimodal_pruned"}, events.eventTypes())
	require.NotContains(t, events.events[0].Payload, oldURL)

	baseModel := &recordingMultimodalBudgetModel{}
	wrapped, err := middleware.WrapModel(
		context.Background(),
		baseModel,
		&adk.ModelContext{},
	)
	require.NoError(t, err)
	_, err = wrapped.Generate(modelCtx, state.Messages)
	require.NoError(t, err)
	reader, err := wrapped.Stream(modelCtx, state.Messages)
	require.NoError(t, err)
	defer reader.Close()
	_, err = reader.Recv()
	require.NoError(t, err)

	require.Len(t, baseModel.inputs, 2)
	for _, input := range baseModel.inputs {
		require.Equal(
			t,
			schema.ChatMessagePartTypeText,
			input[0].UserInputMultiContent[0].Type,
		)
		require.Nil(t, input[0].UserInputMultiContent[0].Image)
		require.NotContains(
			t,
			input[0].UserInputMultiContent[0].Text,
			oldURL,
		)
	}
	require.Equal(t, []string{"context.multimodal_pruned"}, events.eventTypes())
}

func TestADKMultimodalBudgetMiddlewareRejectsCurrentTurnBeforeProviderCall(
	t *testing.T,
) {
	currentURL := "https://example.test/current.png"
	events := &recordingRunEventSink{}
	middleware, err := NewADKMultimodalBudgetMiddleware(
		&RunSummary{ThreadID: 10, RunID: 20},
		ADKContextBudget{
			MultimodalHistoryTokens: 1000,
			FileHistoryTokens:       adkFileTokens,
		},
		events,
	)
	require.NoError(t, err)
	chatModel := &recordingMultimodalBudgetModel{}
	agent, err := adk.NewChatModelAgent(
		context.Background(),
		&adk.ChatModelAgentConfig{
			Name:        "lead",
			Description: "multimodal budget test",
			Model:       chatModel,
			Handlers:    []adk.ChatModelAgentMiddleware{middleware},
		},
	)
	require.NoError(t, err)

	agentEvents := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL: &currentURL,
					},
					Detail: schema.ImageURLDetailHigh,
				},
			}},
		}},
	})

	require.NotEmpty(t, agentEvents)
	require.Error(t, agentEvents[len(agentEvents)-1].Err)
	require.Empty(t, chatModel.inputs)
	require.Equal(
		t,
		[]string{"context.multimodal_budget_exceeded"},
		events.eventTypes(),
	)
	require.NotContains(t, events.events[0].Payload, currentURL)
}

func TestADKMiddlewareProjectsMainModelButPersistsCompleteTerminalTranscript(
	t *testing.T,
) {
	oldURL := "https://example.test/main-old.png"
	store := &recordingADKTranscriptStore{}
	events := &recordingRunEventSink{}
	chatModel := &recordingSummarizingMultimodalModel{}
	run := &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Config: `{
			"context_budget":{
				"context_window_tokens":10000,
				"summarization_tokens":9000,
				"summarization_messages":100,
				"memory_tokens":100,
				"multimodal_history_tokens":1,
				"file_history_tokens":2048
			}
		}`,
	}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		TranscriptStore: store,
		EventSink:       events,
	})
	bundle, err := assembler.Build(
		context.Background(),
		ADKMiddlewareBuildInput{
			Run:   run,
			Model: chatModel,
			ModelCapabilities: ADKModelCapabilities{
				Vision: true,
			},
		},
	)
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(
		context.Background(),
		&adk.ChatModelAgentConfig{
			Name:        "lead",
			Description: "main multimodal projection",
			Model:       chatModel,
			Handlers:    bundle.Handlers,
		},
	)
	require.NoError(t, err)

	agentEvents := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL: &oldURL,
					},
					Detail: schema.ImageURLDetailLow,
				},
			}},
		}, schema.UserMessage("current text-only request")},
	})

	require.NotEmpty(t, agentEvents)
	require.NoError(t, agentEvents[len(agentEvents)-1].Err)
	require.Len(t, chatModel.normalInputs, 1)
	require.NotContains(
		t,
		multimodalBudgetMessagesJSON(t, chatModel.normalInputs[0]),
		oldURL,
	)
	require.Len(t, store.calls, 1)
	require.Equal(t, TranscriptKindTerminal, store.calls[0].Kind)
	require.Contains(t, store.calls[0].Messages, oldURL)
	require.Contains(t, events.eventTypes(), "context.multimodal_pruned")
}

func TestADKMiddlewareProjectsSummarizationInputBeforeSummaryModelCall(
	t *testing.T,
) {
	oldURL := "https://example.test/summary-old.png"
	store := &recordingADKTranscriptStore{}
	events := &recordingRunEventSink{}
	chatModel := &recordingSummarizingMultimodalModel{}
	run := &RunSummary{
		ThreadID: 10,
		RunID:    20,
		Config: `{
			"context_budget":{
				"context_window_tokens":10000,
				"summarization_tokens":9000,
				"summarization_messages":1,
				"memory_tokens":100,
				"multimodal_history_tokens":1,
				"file_history_tokens":2048
			}
		}`,
	}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		TranscriptStore: store,
		EventSink:       events,
	})
	bundle, err := assembler.Build(
		context.Background(),
		ADKMiddlewareBuildInput{
			Run:   run,
			Model: chatModel,
			ModelCapabilities: ADKModelCapabilities{
				Vision: true,
			},
		},
	)
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(
		context.Background(),
		&adk.ChatModelAgentConfig{
			Name:        "lead",
			Description: "summary multimodal projection",
			Model:       chatModel,
			Handlers:    bundle.Handlers,
		},
	)
	require.NoError(t, err)

	agentEvents := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{{
			Role: schema.User,
			UserInputMultiContent: []schema.MessageInputPart{{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL: &oldURL,
					},
					Detail: schema.ImageURLDetailLow,
				},
			}},
		}, schema.UserMessage("current text-only request")},
	})

	require.NotEmpty(t, agentEvents)
	require.NoError(t, agentEvents[len(agentEvents)-1].Err)
	require.Len(t, chatModel.summaryInputs, 1)
	require.NotContains(
		t,
		multimodalBudgetMessagesJSON(t, chatModel.summaryInputs[0]),
		oldURL,
	)
	require.Len(t, chatModel.normalInputs, 1)
	require.Len(t, store.calls, 2)
	require.Equal(t, TranscriptKindSummaryInput, store.calls[0].Kind)
	require.Contains(t, store.calls[0].Messages, oldURL)
	require.Equal(t, TranscriptKindTerminal, store.calls[1].Kind)

	prunedEvents := 0
	for _, event := range events.events {
		if event.EventType == "context.multimodal_pruned" {
			prunedEvents++
			require.Contains(t, event.Payload, `"phase":"summarization"`)
			require.NotContains(t, event.Payload, oldURL)
		}
	}
	require.Equal(t, 1, prunedEvents)
}

func adkMultimodalBudgetUserMessage(
	content string,
	imageURL string,
	fileURL string,
	fileName string,
) *schema.Message {
	return &schema.Message{
		Role:    schema.User,
		Content: content,
		UserInputMultiContent: []schema.MessageInputPart{{
			Type: schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{
				MessagePartCommon: schema.MessagePartCommon{
					URL: &imageURL,
				},
				Detail: schema.ImageURLDetailLow,
			},
		}, {
			Type: schema.ChatMessagePartTypeFileURL,
			File: &schema.MessageInputFile{
				MessagePartCommon: schema.MessagePartCommon{
					URL: &fileURL,
				},
				Name: fileName,
			},
		}},
	}
}

type recordingMultimodalBudgetModel struct {
	inputs [][]*schema.Message
}

func (m *recordingMultimodalBudgetModel) Generate(
	_ context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.inputs = append(m.inputs, input)
	return schema.AssistantMessage("done", nil), nil
}

func (m *recordingMultimodalBudgetModel) Stream(
	_ context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	m.inputs = append(m.inputs, input)
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	return schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage("done", nil),
	}), nil
}

type recordingSummarizingMultimodalModel struct {
	summaryInputs [][]*schema.Message
	normalInputs  [][]*schema.Message
}

func (m *recordingSummarizingMultimodalModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	if adkUsageKindFromContext(ctx) == string(ADKMiddlewareSummarization) {
		m.summaryInputs = append(m.summaryInputs, input)
		return schema.AssistantMessage("stable summary", nil), nil
	}
	m.normalInputs = append(m.normalInputs, input)
	return schema.AssistantMessage("done", nil), nil
}

func (m *recordingSummarizingMultimodalModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func multimodalBudgetMessagesJSON(
	t *testing.T,
	messages []*schema.Message,
) string {
	t.Helper()
	raw, err := json.Marshal(messages)
	require.NoError(t, err)
	return strings.TrimSpace(string(raw))
}
