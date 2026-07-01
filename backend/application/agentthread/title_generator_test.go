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
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestModelRunTitleGeneratorBuildsDeerFlowPromptAndCleansTitle(t *testing.T) {
	longUserMessage := strings.Repeat("青岛亲子旅行", 120) + "不要出现的尾部"
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage(`<think>hidden</think>"青岛亲子旅行"。`, nil),
	}
	var gotModelID int64
	generator := NewModelRunTitleGenerator(func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
		gotModelID = modelID
		return chatModel, true, nil
	})

	title, err := generator.GenerateTitle(context.Background(), RunTitleGenerationInput{
		Run: &RunSummary{
			Config: `{
				"model_id":100002,
				"model_name":"deepseek-v4-pro",
				"thinking_enabled":true,
				"title":{"model_name":"title-model"}
			}`,
		},
		UserMessage:      longUserMessage,
		AssistantMessage: "<think>思考过程</think>推荐春秋两季，适合亲子慢游。",
	})

	require.NoError(t, err)
	require.Equal(t, "青岛亲子旅行", title)
	require.Equal(t, int64(100002), gotModelID)
	require.Equal(t, 1, chatModel.calls)
	require.Len(t, chatModel.messages, 1)
	prompt := chatModel.messages[0].Content
	require.Contains(t, prompt, "Generate a concise title (max 6 words)")
	require.Contains(t, prompt, "User: "+string([]rune(longUserMessage)[:500]))
	require.NotContains(t, prompt, "不要出现的尾部")
	require.Contains(t, prompt, "Assistant: 推荐春秋两季，适合亲子慢游。")
	require.NotContains(t, prompt, "<think>")
	require.NotNil(t, chatModel.options.Model)
	require.Equal(t, "title-model", *chatModel.options.Model)
	require.NotNil(t, chatModel.options.MaxTokens)
	require.LessOrEqual(t, *chatModel.options.MaxTokens, 96)
}
