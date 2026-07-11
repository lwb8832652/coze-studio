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

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestModelExecutorGeneratesAssistantMessageFromRunInput(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage(" 执行完成 ", nil),
	}
	var gotModelID int64
	executor := NewModelExecutor(func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
		gotModelID = modelID

		return chatModel, true, nil
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		RunID:       200,
		ThreadID:    10,
		AssistantID: "assistant-a",
		Input:       `{"messages":[{"role":"user","content":"分析客户反馈"},{"role":"assistant","content":"已有结论"},{"role":"user","content":"补充行动建议"}]}`,
		Config:      `{"model_id":100002,"model_name":"doubao-pro","temperature":0.2,"max_tokens":512,"top_p":0.8,"system_prompt":"你是任务执行助手"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(100002), gotModelID)
	require.NotNil(t, result)
	require.Equal(t, "执行完成", result.Message)
	require.Len(t, chatModel.messages, 4)
	require.Equal(t, schema.System, chatModel.messages[0].Role)
	require.Equal(t, "你是任务执行助手", chatModel.messages[0].Content)
	require.Equal(t, schema.User, chatModel.messages[1].Role)
	require.Equal(t, "分析客户反馈", chatModel.messages[1].Content)
	require.Equal(t, schema.Assistant, chatModel.messages[2].Role)
	require.Equal(t, "已有结论", chatModel.messages[2].Content)
	require.Equal(t, schema.User, chatModel.messages[3].Role)
	require.Equal(t, "补充行动建议", chatModel.messages[3].Content)
	require.NotNil(t, chatModel.options.Model)
	require.Equal(t, "doubao-pro", *chatModel.options.Model)
	require.NotNil(t, chatModel.options.Temperature)
	require.InDelta(t, float32(0.2), *chatModel.options.Temperature, 0.0001)
	require.NotNil(t, chatModel.options.MaxTokens)
	require.Equal(t, 512, *chatModel.options.MaxTokens)
	require.NotNil(t, chatModel.options.TopP)
	require.InDelta(t, float32(0.8), *chatModel.options.TopP, 0.0001)

	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Metadata), &metadata))
	require.Equal(t, "model_executor", metadata["source"])
	require.Equal(t, float64(100002), metadata["model_id"])
	require.Equal(t, "doubao-pro", metadata["model_name"])
}

func TestModelExecutorAcceptsSimpleMessageInput(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("周报已生成", nil),
	}
	var gotModelID int64
	executor := NewModelExecutor(func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
		gotModelID = modelID

		return chatModel, true, nil
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		Input:  `{"message":"请生成周报"}`,
		Config: `{"modelType":200003}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(200003), gotModelID)
	require.Equal(t, "周报已生成", result.Message)
	require.Len(t, chatModel.messages, 1)
	require.Equal(t, schema.User, chatModel.messages[0].Role)
	require.Equal(t, "请生成周报", chatModel.messages[0].Content)
}

func TestParseModelExecutorMessagesPreservesCompleteMultimodalMessages(
	t *testing.T,
) {
	messages, err := parseModelExecutorMessages(`{
		"messages":[
			{
				"role":"user",
				"content":"",
				"user_input_multi_content":[
					{
						"type":"image_url",
						"image":{
							"base64data":"aW1hZ2UtYnl0ZXM=",
							"mime_type":"image/png",
							"detail":"high"
						},
						"extra":{"asset_id":"image-1"}
					},
					{
						"type":"file_url",
						"file":{
							"url":"https://example.test/report.pdf",
							"mime_type":"application/pdf",
							"name":"report.pdf"
						}
					}
				],
				"multi_content":[
					{
						"type":"audio_url",
						"audio_url":{
							"uri":"audio-legacy-1",
							"mime_type":"audio/wav"
						}
					}
				]
			},
			{
				"role":"assistant",
				"content":"",
				"reasoning_content":"inspect attachments",
				"tool_calls":[{
					"id":"call-1",
					"type":"function",
					"function":{
						"name":"inspect_file",
						"arguments":"{\"name\":\"report.pdf\"}"
					}
				}],
				"assistant_output_multi_content":[{
					"type":"reasoning",
					"reasoning":{"text":"inspect attachments","signature":"signed"}
				}]
			},
			{
				"role":"tool",
				"content":"pdf is valid",
				"tool_call_id":"call-1",
				"tool_name":"inspect_file"
			}
		]
	}`, "")

	require.NoError(t, err)
	require.Len(t, messages, 3)
	require.Equal(t, schema.User, messages[0].Role)
	require.Empty(t, messages[0].Content)
	require.Len(t, messages[0].UserInputMultiContent, 2)
	require.Equal(
		t,
		"aW1hZ2UtYnl0ZXM=",
		*messages[0].UserInputMultiContent[0].Image.Base64Data,
	)
	require.Equal(
		t,
		schema.ImageURLDetailHigh,
		messages[0].UserInputMultiContent[0].Image.Detail,
	)
	require.Equal(
		t,
		"image-1",
		messages[0].UserInputMultiContent[0].Extra["asset_id"],
	)
	require.Equal(
		t,
		"report.pdf",
		messages[0].UserInputMultiContent[1].File.Name,
	)
	require.Len(t, messages[0].MultiContent, 1)
	require.Equal(
		t,
		"audio-legacy-1",
		messages[0].MultiContent[0].AudioURL.URI,
	)
	require.Equal(t, schema.Assistant, messages[1].Role)
	require.Equal(t, "inspect attachments", messages[1].ReasoningContent)
	require.Equal(t, "inspect_file", messages[1].ToolCalls[0].Function.Name)
	require.Equal(
		t,
		"signed",
		messages[1].AssistantGenMultiContent[0].Reasoning.Signature,
	)
	require.Equal(t, schema.Tool, messages[2].Role)
	require.Equal(t, "call-1", messages[2].ToolCallID)
	require.Equal(t, "inspect_file", messages[2].ToolName)
}

func TestParseModelExecutorMessagesDoesNotForwardAuthoritativeRunMarker(t *testing.T) {
	messages, err := parseModelExecutorMessages(`{
		"messages":[{"_run_id":100,"role":"user","content":"继续分析"}]
	}`, "")

	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, schema.User, messages[0].Role)
	require.Equal(t, "继续分析", messages[0].Content)
	encoded, err := json.Marshal(messages[0])
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "_run_id")
}

func TestModelExecutorRejectsEmptyRunInputMessages(t *testing.T) {
	called := false
	executor := NewModelExecutor(func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
		called = true

		return &recordingChatModel{}, true, nil
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		Input: `{"messages":[{"role":"user","content":"   "}]}`,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "run input messages are required")
	require.Nil(t, result)
	require.False(t, called)
}

func TestModelExecutorPropagatesModelError(t *testing.T) {
	executor := NewModelExecutor(func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
		return &recordingChatModel{err: fmt.Errorf("model failed")}, true, nil
	})

	result, err := executor.Execute(context.Background(), &RunSummary{
		Input: `{"messages":[{"role":"user","content":"hello"}]}`,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "model failed")
	require.Nil(t, result)
}

type recordingChatModel struct {
	messages []*schema.Message
	options  *model.Options
	resp     *schema.Message
	err      error
	calls    int
}

func (m *recordingChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.calls++
	m.messages = append([]*schema.Message(nil), input...)
	m.options = model.GetCommonOptions(nil, opts...)
	if m.err != nil {
		return nil, m.err
	}

	return m.resp, nil
}

func (m *recordingChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, fmt.Errorf("stream is not implemented")
}
