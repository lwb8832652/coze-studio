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
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"io"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestMapADKEventMapsAssistantMessage(t *testing.T) {
	imageURL := "https://example.com/result.png"
	base64Data := "must-not-be-persisted"
	event := &adk.AgentEvent{
		AgentName: "researcher",
		RunPath: []adk.RunStep{
			newADKRunStep(t, "lead"),
			newADKRunStep(t, "researcher"),
		},
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: &schema.Message{
					Role:             schema.Assistant,
					Content:          "hello",
					ReasoningContent: "thinking",
					ToolCalls: []schema.ToolCall{{
						ID:   "call-1",
						Type: "function",
						Function: schema.FunctionCall{
							Name:      "search",
							Arguments: `{"query":"coze"}`,
						},
					}},
					AssistantGenMultiContent: []schema.MessageOutputPart{
						{
							Type: schema.ChatMessagePartTypeReasoning,
							Reasoning: &schema.MessageOutputReasoning{
								Text:      "reasoning part",
								Signature: "signature-1",
							},
						},
						{
							Type: schema.ChatMessagePartTypeImageURL,
							Image: &schema.MessageOutputImage{
								MessagePartCommon: schema.MessagePartCommon{
									URL:        &imageURL,
									Base64Data: &base64Data,
									MIMEType:   "image/png",
								},
							},
						},
					},
					ResponseMeta: &schema.ResponseMeta{
						FinishReason: "stop",
						Usage: &schema.TokenUsage{
							PromptTokens:     10,
							CompletionTokens: 4,
							TotalTokens:      14,
							PromptTokenDetails: schema.PromptTokenDetails{
								CachedTokens: 3,
							},
							CompletionTokensDetails: schema.CompletionTokensDetails{
								ReasoningTokens: 2,
							},
						},
					},
				},
				Role: schema.Assistant,
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)

	require.NoError(t, err)
	require.Equal(t, int64(10), mapped.ThreadID)
	require.Equal(t, int64(20), mapped.RunID)
	require.Equal(t, "message.completed", mapped.EventType)
	require.Equal(t, "hello", mapped.FinalText)
	require.JSONEq(t, `{
		"agent_name":"researcher",
		"run_path":["lead","researcher"],
		"subagent":{
			"name":"researcher",
			"root_name":"lead",
			"parent_name":"lead",
			"step_id":"lead/researcher",
			"run_path":["lead","researcher"],
			"depth":1
		},
		"role":"assistant",
		"content":"hello",
		"reasoning_content":"thinking",
		"reasoning_parts":[{
			"text":"reasoning part",
			"signature":"signature-1"
		}],
		"finish_reason":"stop",
		"tool_calls":[{
			"id":"call-1",
			"type":"function",
			"function":{"name":"search","arguments":"{\"query\":\"coze\"}"}
		}],
		"media":[{
			"type":"image_url",
			"url":"https://example.com/result.png",
			"mime_type":"image/png",
			"has_base64_data":true
		}],
		"usage":{
			"prompt_tokens":10,
			"completion_tokens":4,
			"total_tokens":14,
			"cached_tokens":3,
			"reasoning_tokens":2
		}
	}`, mapped.Payload)
	require.NotContains(t, mapped.Payload, base64Data)
	require.NotNil(t, mapped.Usage)
	require.Equal(t, TokenUsageSourceSubagent, mapped.Usage.Source)
	require.Equal(t, "lead/researcher", mapped.Usage.StepID)
	require.Equal(t, "researcher", mapped.Usage.StepName)
	require.Equal(t, int64(10), mapped.Usage.InputTokens)
	require.Equal(t, int64(4), mapped.Usage.OutputTokens)
	require.Equal(t, int64(14), mapped.Usage.TotalTokens)
	require.JSONEq(t, `{
		"prompt_tokens":10,
		"completion_tokens":4,
		"total_tokens":14,
		"cached_tokens":3,
		"reasoning_tokens":2
	}`, mapped.Usage.RawUsage)
	require.JSONEq(t, `{
		"source":"eino_event",
		"agent_name":"researcher",
		"run_path":["lead","researcher"],
		"step_id":"lead/researcher",
		"step_index":1,
		"subagent":{
			"name":"researcher",
			"root_name":"lead",
			"parent_name":"lead",
			"step_id":"lead/researcher",
			"run_path":["lead","researcher"],
			"depth":1
		}
	}`, mapped.Usage.Metadata)
}

func TestMapADKEventHidesPlanCompletionGuardMessages(t *testing.T) {
	t.Run("assistant tool call", func(t *testing.T) {
		mapped, err := MapADKEvent(context.Background(), 10, 20, &adk.AgentEvent{
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message: &schema.Message{
						Role: schema.Assistant,
						ToolCalls: []schema.ToolCall{{
							ID:   "call-plan-guard",
							Type: "function",
							Function: schema.FunctionCall{
								Name:      adkPlanCompletionGuardToolName,
								Arguments: `{"reminder":"continue"}`,
							},
						}},
					},
					Role: schema.Assistant,
				},
			},
		})

		require.NoError(t, err)
		require.Equal(t, "agent.control", mapped.EventType)
		require.Empty(t, mapped.FinalText)
		require.JSONEq(t, `{
			"hidden":true,
			"schema":"coze.plan_completion_guard.v1"
		}`, mapped.Payload)
	})

	t.Run("tool result", func(t *testing.T) {
		mapped, err := MapADKEvent(context.Background(), 10, 20, &adk.AgentEvent{
			Output: &adk.AgentOutput{
				MessageOutput: &adk.MessageVariant{
					Message: &schema.Message{
						Role:       schema.Tool,
						Content:    "<system_reminder>continue</system_reminder>",
						ToolName:   adkPlanCompletionGuardToolName,
						ToolCallID: "call-plan-guard",
					},
					Role:     schema.Tool,
					ToolName: adkPlanCompletionGuardToolName,
				},
			},
		})

		require.NoError(t, err)
		require.Equal(t, "agent.control", mapped.EventType)
		require.Empty(t, mapped.FinalText)
		require.JSONEq(t, `{
			"hidden":true,
			"schema":"coze.plan_completion_guard.v1"
		}`, mapped.Payload)
	})
}

func TestMapADKEventMapsSafetyFinish(t *testing.T) {
	event := &adk.AgentEvent{
		AgentName: "lead",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: &schema.Message{
					Role:    schema.Assistant,
					Content: "I can't help with that.",
					ResponseMeta: &schema.ResponseMeta{
						FinishReason: "content_filter",
					},
				},
				Role: schema.Assistant,
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)

	require.NoError(t, err)
	require.Equal(t, "model.safety_finish", mapped.EventType)
	require.Equal(t, "I can't help with that.", mapped.FinalText)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"role":"assistant",
		"content":"I can't help with that.",
		"finish_reason":"content_filter",
		"finish_classification":{
			"category":"safety",
			"reason":"content_filter",
			"safety":true,
			"terminal":true
		}
	}`, mapped.Payload)
}

func TestMapADKEventConsumesStreamingMessageOnce(t *testing.T) {
	stream := schema.StreamReaderFromArray([]*schema.Message{
		{Role: schema.Assistant, Content: "hel"},
		{Role: schema.Assistant, Content: "lo"},
	})
	event := &adk.AgentEvent{
		AgentName: "lead",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				IsStreaming:   true,
				MessageStream: stream,
				Role:          schema.Assistant,
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)

	require.NoError(t, err)
	require.Equal(t, "message.completed", mapped.EventType)
	require.Equal(t, "hello", mapped.FinalText)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"role":"assistant",
		"content":"hello"
	}`, mapped.Payload)
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
}

func TestMapADKEventMapsToolResult(t *testing.T) {
	event := &adk.AgentEvent{
		AgentName: "lead",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: &schema.Message{
					Role:       schema.Tool,
					Content:    `{"answer":42}`,
					ToolCallID: "call-1",
				},
				Role:     schema.Tool,
				ToolName: "calculator",
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)

	require.NoError(t, err)
	require.Equal(t, "tool.completed", mapped.EventType)
	require.Empty(t, mapped.FinalText)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"role":"tool",
		"content":"{\"answer\":42}",
		"tool_name":"calculator",
		"tool_call_id":"call-1"
	}`, mapped.Payload)
}

func TestMapADKEventMapsNormalizedToolError(t *testing.T) {
	content := encodeADKToolErrorResult(
		"calculator",
		"call-1",
		errors.New("divide failed"),
	)
	event := &adk.AgentEvent{
		AgentName: "lead",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: &schema.Message{
					Role:       schema.Tool,
					Content:    content,
					ToolCallID: "call-1",
				},
				Role:     schema.Tool,
				ToolName: "calculator",
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)

	require.NoError(t, err)
	require.Equal(t, "tool.failed", mapped.EventType)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"role":"tool",
		"content":"{\"schema\":\"coze.tool_error.v1\",\"status\":\"failed\",\"tool_name\":\"calculator\",\"tool_call_id\":\"call-1\",\"error_message\":\"divide failed\",\"recoverable\":true,\"normalized\":true}",
		"tool_name":"calculator",
		"tool_call_id":"call-1",
		"tool_error":{
			"schema":"coze.tool_error.v1",
			"status":"failed",
			"tool_name":"calculator",
			"tool_call_id":"call-1",
			"error_message":"divide failed",
			"recoverable":true,
			"normalized":true
		}
	}`, mapped.Payload)
}

func TestMapADKEventMapsEnhancedToolErrorTextPart(t *testing.T) {
	content := encodeADKToolErrorResult(
		"inspect_image",
		"call-image",
		errors.New("image inspect failed"),
	)
	event := &adk.AgentEvent{
		AgentName: "lead",
		Output: &adk.AgentOutput{
			MessageOutput: &adk.MessageVariant{
				Message: &schema.Message{
					Role:       schema.Tool,
					ToolCallID: "call-image",
					UserInputMultiContent: []schema.MessageInputPart{{
						Type: schema.ChatMessagePartTypeText,
						Text: content,
					}},
				},
				Role:     schema.Tool,
				ToolName: "inspect_image",
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)

	require.NoError(t, err)
	require.Equal(t, "tool.failed", mapped.EventType)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"role":"tool",
		"content":"{\"schema\":\"coze.tool_error.v1\",\"status\":\"failed\",\"tool_name\":\"inspect_image\",\"tool_call_id\":\"call-image\",\"error_message\":\"image inspect failed\",\"recoverable\":true,\"normalized\":true}",
		"content_parts":[{
			"type":"text",
			"text":"{\"schema\":\"coze.tool_error.v1\",\"status\":\"failed\",\"tool_name\":\"inspect_image\",\"tool_call_id\":\"call-image\",\"error_message\":\"image inspect failed\",\"recoverable\":true,\"normalized\":true}"
		}],
		"tool_name":"inspect_image",
		"tool_call_id":"call-image",
		"tool_error":{
			"schema":"coze.tool_error.v1",
			"status":"failed",
			"tool_name":"inspect_image",
			"tool_call_id":"call-image",
			"error_message":"image inspect failed",
			"recoverable":true,
			"normalized":true
		}
	}`, mapped.Payload)
}

func TestMapADKEventMapsInterrupt(t *testing.T) {
	interrupt := &adk.InterruptCtx{
		ID: "interrupt-1",
		Address: adk.Address{
			{Type: adk.AddressSegmentAgent, ID: "lead"},
			{Type: adk.AddressSegmentTool, ID: "approval", SubID: "call-1"},
		},
		Info:        map[string]any{"question": "approve?"},
		IsRootCause: true,
	}
	event := &adk.AgentEvent{
		AgentName: "lead",
		Action: &adk.AgentAction{
			Interrupted: &adk.InterruptInfo{
				Data:              map[string]any{"reason": "approval_required"},
				InterruptContexts: []*adk.InterruptCtx{interrupt},
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)

	require.NoError(t, err)
	require.Equal(t, "run.interrupted", mapped.EventType)
	require.Equal(t, &ADKInterruptMapping{
		Items: []ADKInterruptItem{{
			ID:          "interrupt-1",
			Address:     "agent:lead;tool:approval:call-1",
			Info:        map[string]any{"question": "approve?"},
			IsRootCause: true,
		}},
	}, mapped.Interrupt)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"data":{"reason":"approval_required"},
		"interrupts":[{
			"id":"interrupt-1",
			"address":"agent:lead;tool:approval:call-1",
			"info":{"question":"approve?"},
			"is_root_cause":true
		}]
	}`, mapped.Payload)
}

func TestMapADKEventMapsHumanInteractionInterrupt(t *testing.T) {
	prompt := HumanInteractionPrompt{
		Schema:        humanInteractionSchema,
		InteractionID: "hi_1",
		Kind:          HumanInteractionKindConfirmation,
		Title:         "确认删除文件",
		Summary:       "删除临时报告文件",
		Required:      true,
		AllowFreeText: true,
		RiskLevel:     HumanInteractionRiskHigh,
		ToolName:      adkConfirmationToolName,
	}
	event := &adk.AgentEvent{
		AgentName: "lead",
		Action: &adk.AgentAction{
			Interrupted: &adk.InterruptInfo{
				InterruptContexts: []*adk.InterruptCtx{{
					ID: "interrupt-1",
					Address: adk.Address{
						{Type: adk.AddressSegmentAgent, ID: "lead"},
						{Type: adk.AddressSegmentTool, ID: "request_human_confirmation", SubID: "call-1"},
					},
					Info:        prompt,
					IsRootCause: true,
				}},
			},
		},
	}

	mapped, err := MapADKEvent(context.Background(), 10, 20, event)

	require.NoError(t, err)
	require.Equal(t, "run.interrupted", mapped.EventType)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"interrupts":[{
			"id":"interrupt-1",
			"address":"agent:lead;tool:request_human_confirmation:call-1",
			"info":{
				"schema":"coze.human_interaction.v1",
				"interaction_id":"hi_1",
				"kind":"confirmation",
				"title":"确认删除文件",
				"summary":"删除临时报告文件",
				"required":true,
				"allow_free_text":true,
				"risk_level":"high",
				"tool_name":"request_human_confirmation"
			},
			"is_root_cause":true
		}],
		"human_interaction":{
			"schema":"coze.human_interaction.v1",
			"interaction_id":"hi_1",
			"kind":"confirmation",
			"title":"确认删除文件",
			"summary":"删除临时报告文件",
			"required":true,
			"allow_free_text":true,
			"risk_level":"high",
			"tool_name":"request_human_confirmation"
		},
		"human_interactions":[{
			"schema":"coze.human_interaction.v1",
			"interaction_id":"hi_1",
			"kind":"confirmation",
			"title":"确认删除文件",
			"summary":"删除临时报告文件",
			"required":true,
			"allow_free_text":true,
			"risk_level":"high",
			"tool_name":"request_human_confirmation"
		}]
	}`, mapped.Payload)
}

func TestMapADKEventMapsSummarizationActions(t *testing.T) {
	tests := []struct {
		name        string
		action      *summarization.CustomizedAction
		eventType   string
		payloadPart string
	}{
		{
			name: "before summarization",
			action: &summarization.CustomizedAction{
				Type: summarization.ActionTypeBeforeSummarize,
				Before: &summarization.BeforeSummarizeAction{
					Messages: []*schema.Message{
						schema.UserMessage("private transcript value"),
					},
				},
			},
			eventType:   "context.summarizing",
			payloadPart: `"message_count":1`,
		},
		{
			name: "summary model call",
			action: &summarization.CustomizedAction{
				Type: summarization.ActionTypeGenerateSummary,
				GenerateSummary: &summarization.GenerateSummaryAction{
					Attempt:       2,
					Phase:         summarization.GenerateSummaryPhaseFailover,
					ModelResponse: schema.AssistantMessage("private summary value", nil),
				},
			},
			eventType:   "context.summary_model_call",
			payloadPart: `"attempt":2`,
		},
		{
			name: "after summarization",
			action: &summarization.CustomizedAction{
				Type: summarization.ActionTypeAfterSummarize,
				After: &summarization.AfterSummarizeAction{
					Messages: []*schema.Message{
						schema.SystemMessage("base"),
						schema.AssistantMessage("private summary value", nil),
					},
				},
			},
			eventType:   "context.summarized",
			payloadPart: `"message_count":2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped, err := MapADKEvent(context.Background(), 10, 20, &adk.AgentEvent{
				AgentName: "lead",
				Action: &adk.AgentAction{
					CustomizedAction: tt.action,
				},
			})

			require.NoError(t, err)
			require.Equal(t, tt.eventType, mapped.EventType)
			require.Contains(t, mapped.Payload, tt.payloadPart)
			require.Contains(t, mapped.Payload, `"digest":`)
			require.NotContains(t, mapped.Payload, "private transcript value")
			require.NotContains(t, mapped.Payload, "private summary value")
		})
	}
}

func TestMapADKEventMapsSemanticLoop(t *testing.T) {
	mapped, err := MapADKEvent(context.Background(), 10, 20, &adk.AgentEvent{
		AgentName: "lead",
		Err: &ADKSemanticLoopError{
			Kind:      ADKSemanticLoopKindToolCalls,
			Signature: "sha256:abc123",
			Count:     3,
			Limit:     2,
		},
	})

	require.NoError(t, err)
	require.Equal(t, "run.semantic_loop_detected", mapped.EventType)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"error":"semantic loop detected: tool_calls repeated 3 times (limit 2)",
		"kind":"tool_calls",
		"signature":"sha256:abc123",
		"count":3,
		"limit":2
	}`, mapped.Payload)
	require.NotContains(t, mapped.Payload, "search_docs")
	require.NotContains(t, mapped.Payload, "same text")
}

func TestMapADKEventMapsUnsupportedProviderCapability(t *testing.T) {
	mapped, err := MapADKEvent(context.Background(), 10, 20, &adk.AgentEvent{
		AgentName: "lead",
		Err: &ADKProviderCapabilityError{
			Capability: ADKProviderCapabilityVision,
			PartType:   string(schema.ChatMessagePartTypeImageURL),
			Count:      2,
		},
	})

	require.NoError(t, err)
	require.Equal(t, "model.unsupported_capability", mapped.EventType)
	require.JSONEq(t, `{
		"agent_name":"lead",
		"error":"provider capability unsupported: vision is required for image_url",
		"capability":"vision",
		"part_type":"image_url",
		"count":2
	}`, mapped.Payload)
	require.NotContains(t, mapped.Payload, "https://example.com/private")
	require.NotContains(t, mapped.Payload, "confidential.pdf")
}

func TestMapADKEventAddsSubagentIdentityToRuntimeErrors(t *testing.T) {
	mapped, err := MapADKEvent(context.Background(), 10, 20, &adk.AgentEvent{
		AgentName: "reviewer",
		RunPath: []adk.RunStep{
			newADKRunStep(t, "lead"),
			newADKRunStep(t, "writer"),
			newADKRunStep(t, "reviewer"),
		},
		Err: errors.New("review failed"),
	})

	require.NoError(t, err)
	require.Equal(t, "run.runtime_error", mapped.EventType)
	require.JSONEq(t, `{
		"agent_name":"reviewer",
		"run_path":["lead","writer","reviewer"],
		"subagent":{
			"name":"reviewer",
			"root_name":"lead",
			"parent_name":"writer",
			"step_id":"lead/writer/reviewer",
			"run_path":["lead","writer","reviewer"],
			"depth":2
		},
		"error":"review failed"
	}`, mapped.Payload)
}

func TestMapADKEventNormalizesRuntimeErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		eventType string
		payload   string
	}{
		{
			name: "retrying",
			err: &adk.WillRetryError{
				ErrStr:       "provider unavailable",
				RetryAttempt: 2,
			},
			eventType: "model.retrying",
			payload: `{
				"agent_name":"lead",
				"error":"provider unavailable",
				"retry_attempt":2
			}`,
		},
		{
			name: "retry exhausted",
			err: &adk.RetryExhaustedError{
				LastErr:      errors.New("provider unavailable"),
				TotalRetries: 3,
			},
			eventType: "model.retry_exhausted",
			payload: `{
				"agent_name":"lead",
				"error":"exceeds max retries: last error: provider unavailable",
				"last_error":"provider unavailable",
				"total_retries":3
			}`,
		},
		{
			name: "canceling",
			err: &adk.CancelError{
				Info: &adk.AgentCancelInfo{
					Mode:      adk.CancelAfterChatModel,
					Escalated: true,
					Timeout:   false,
				},
				InterruptContexts: []*adk.InterruptCtx{{
					ID:          "interrupt-1",
					Address:     adk.Address{{Type: adk.AddressSegmentAgent, ID: "lead"}},
					IsRootCause: true,
				}},
			},
			eventType: "run.canceling",
			payload: `{
				"agent_name":"lead",
				"error":"agent canceled: mode=2, escalated=true",
				"mode":2,
				"escalated":true,
				"timeout":false,
				"interrupts":[{
					"id":"interrupt-1",
					"address":"agent:lead",
					"is_root_cause":true
					}]
				}`,
		},
		{
			name:      "stream canceled",
			err:       &adk.StreamCanceledError{},
			eventType: "run.canceling",
			payload: `{
					"agent_name":"lead",
					"error":"stream canceled"
				}`,
		},
		{
			name:      "runtime error",
			err:       errors.New("model stream failed"),
			eventType: "run.runtime_error",
			payload: `{
				"agent_name":"lead",
				"error":"model stream failed"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapped, err := MapADKEvent(context.Background(), 10, 20, &adk.AgentEvent{
				AgentName: "lead",
				Err:       tt.err,
			})

			require.NoError(t, err)
			require.Equal(t, tt.eventType, mapped.EventType)
			require.JSONEq(t, tt.payload, mapped.Payload)
		})
	}
}

func TestMapADKEventRejectsMissingEvent(t *testing.T) {
	_, err := MapADKEvent(context.Background(), 10, 20, nil)

	require.ErrorContains(t, err, "agent event is required")
}

func TestMapADKEventPersistsCancelEventAfterContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mapped, err := MapADKEvent(ctx, 10, 20, &adk.AgentEvent{
		AgentName: "lead",
		Err: &adk.CancelError{
			Info: &adk.AgentCancelInfo{Mode: adk.CancelImmediate},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "run.canceling", mapped.EventType)
}

func newADKRunStep(t *testing.T, agentName string) adk.RunStep {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(struct {
		AgentName string
	}{AgentName: agentName}))

	var step adk.RunStep
	require.NoError(t, step.GobDecode(buf.Bytes()))

	return step
}
