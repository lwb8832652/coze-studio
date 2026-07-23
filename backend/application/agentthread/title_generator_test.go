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
	"errors"
	"strings"
	"testing"

	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	deepseekmodel "github.com/cloudwego/eino-ext/components/model/deepseek"
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
	require.Equal(t, 256, *chatModel.options.MaxTokens)
}

func TestRunTitleModelOptionsDisablesArkThinking(t *testing.T) {
	options := runTitleModelOptions(
		&arkmodel.ChatModel{},
		runTitleGenerationConfig{},
	)

	require.Len(t, options, 1)
}

func TestRunTitleModelOptionsDisablesDeepSeekThinking(t *testing.T) {
	options := runTitleModelOptions(
		&deepseekmodel.ChatModel{},
		runTitleGenerationConfig{},
	)

	require.Len(t, options, 1)
}

func TestBuildRunTitlePromptRemovesKnownResourceMarkers(t *testing.T) {
	prompt := buildRunTitlePrompt(
		RunTitleGenerationInput{
			Run: &RunSummary{
				Config: `{"enable_skills":["web-search"]}`,
			},
			UserMessage:      "请创建技能 @skill-creator，使用 @web-search，并通知 @alice 或 user@example.com",
			AssistantMessage: "已加载 @skill-creator 和 @web-search，稍后通知 @alice",
		},
		runTitleGenerationConfig{MaxWords: defaultRunTitleMaxWords},
	)

	require.NotContains(t, prompt, "@skill-creator")
	require.NotContains(t, prompt, "@web-search")
	require.Contains(t, prompt, "@alice")
	require.Contains(t, prompt, "user@example.com")
	require.Contains(t, prompt, "Do not include tool names, skill names, or @mentions.")
}

func TestModelRunTitleGeneratorCleansResourceMarkersFromGeneratedTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "keeps meaningful title",
			content: "创建 Agent 技能 @skill-creator",
			want:    "创建 Agent 技能",
		},
		{
			name:    "returns empty title when only marker remains",
			content: `"@skill-creator"。`,
			want:    "",
		},
		{
			name:    "removes every independent ascii mention",
			content: "@unknown .NET C# F# @web-search a@b.co word@mention。",
			want:    ".NET C# F# a@b.co word@mention",
		},
		{
			name:    "returns empty title when only untrusted mention remains",
			content: "@web-search。",
			want:    "",
		},
		{
			name:    "preserves leading dot and technical names",
			content: ".NET、C# 与 F#。",
			want:    ".NET、C# 与 F#",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatModel := &recordingChatModel{
				resp: schema.AssistantMessage(tt.content, nil),
			}
			generator := NewModelRunTitleGenerator(
				func(context.Context, int64) (model.BaseChatModel, bool, error) {
					return chatModel, true, nil
				},
			)

			title, err := generator.GenerateTitle(
				context.Background(),
				RunTitleGenerationInput{Run: &RunSummary{}},
			)

			require.NoError(t, err)
			require.Equal(t, tt.want, title)
		})
	}
}

func TestModelRunTitleGeneratorAppliesDefaultPostLimits(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "keeps exactly six words",
			content: "one two three four five six",
			want:    "one two three four five six",
		},
		{
			name:    "truncates after six words",
			content: "one two three four five six seven",
			want:    "one two three four five six",
		},
		{
			name:    "keeps exactly sixty characters",
			content: strings.Repeat("字", defaultRunTitleMaxChars),
			want:    strings.Repeat("字", defaultRunTitleMaxChars),
		},
		{
			name:    "truncates after sixty characters",
			content: strings.Repeat("字", defaultRunTitleMaxChars+1),
			want:    strings.Repeat("字", defaultRunTitleMaxChars),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatModel := &recordingChatModel{
				resp: schema.AssistantMessage(tt.content, nil),
			}
			generator := NewModelRunTitleGenerator(
				func(context.Context, int64) (model.BaseChatModel, bool, error) {
					return chatModel, true, nil
				},
			)

			title, err := generator.GenerateTitle(
				context.Background(),
				RunTitleGenerationInput{Run: &RunSummary{}},
			)

			require.NoError(t, err)
			require.Equal(t, tt.want, title)
		})
	}
}

func TestModelRunTitleGeneratorAppliesCustomPostLimits(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		content string
		want    string
	}{
		{
			name:    "custom word limit",
			config:  `{"title":{"max_words":3,"max_chars":20}}`,
			content: "one two three four",
			want:    "one two three",
		},
		{
			name:    "custom character limit",
			config:  `{"title":{"max_words":6,"max_chars":10}}`,
			content: strings.Repeat("字", 11),
			want:    strings.Repeat("字", 10),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatModel := &recordingChatModel{
				resp: schema.AssistantMessage(tt.content, nil),
			}
			generator := NewModelRunTitleGenerator(
				func(context.Context, int64) (model.BaseChatModel, bool, error) {
					return chatModel, true, nil
				},
			)

			title, err := generator.GenerateTitle(
				context.Background(),
				RunTitleGenerationInput{
					Run: &RunSummary{Config: tt.config},
				},
			)

			require.NoError(t, err)
			require.Equal(t, tt.want, title)
		})
	}
}

func TestModelRunTitleGeneratorPreservesErrorContracts(t *testing.T) {
	t.Run("nil generator", func(t *testing.T) {
		var generator *ModelRunTitleGenerator

		title, err := generator.GenerateTitle(
			context.Background(),
			RunTitleGenerationInput{Run: &RunSummary{}},
		)

		require.Empty(t, title)
		require.EqualError(t, err, "model run title generator is required")
	})

	t.Run("nil run", func(t *testing.T) {
		generator := NewModelRunTitleGenerator(nil)

		title, err := generator.GenerateTitle(
			context.Background(),
			RunTitleGenerationInput{},
		)

		require.Empty(t, title)
		require.EqualError(t, err, "run is required")
	})

	t.Run("provider error", func(t *testing.T) {
		providerErr := errors.New("provider failed")
		generator := NewModelRunTitleGenerator(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return nil, false, providerErr
			},
		)

		title, err := generator.GenerateTitle(
			context.Background(),
			RunTitleGenerationInput{Run: &RunSummary{}},
		)

		require.Empty(t, title)
		require.ErrorIs(t, err, providerErr)
	})

	t.Run("provider not configured", func(t *testing.T) {
		generator := NewModelRunTitleGenerator(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return nil, false, nil
			},
		)

		title, err := generator.GenerateTitle(
			context.Background(),
			RunTitleGenerationInput{Run: &RunSummary{}},
		)

		require.Empty(t, title)
		require.EqualError(t, err, "agent thread title model is not configured")
	})

	t.Run("nil configured model", func(t *testing.T) {
		generator := NewModelRunTitleGenerator(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return nil, true, nil
			},
		)

		title, err := generator.GenerateTitle(
			context.Background(),
			RunTitleGenerationInput{Run: &RunSummary{}},
		)

		require.Empty(t, title)
		require.EqualError(t, err, "agent thread title model is not configured")
	})

	t.Run("model error", func(t *testing.T) {
		modelErr := errors.New("model failed")
		chatModel := &recordingChatModel{err: modelErr}
		generator := NewModelRunTitleGenerator(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return chatModel, true, nil
			},
		)

		title, err := generator.GenerateTitle(
			context.Background(),
			RunTitleGenerationInput{Run: &RunSummary{}},
		)

		require.Empty(t, title)
		require.ErrorIs(t, err, modelErr)
	})

	t.Run("nil model response", func(t *testing.T) {
		chatModel := &recordingChatModel{}
		generator := NewModelRunTitleGenerator(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return chatModel, true, nil
			},
		)

		title, err := generator.GenerateTitle(
			context.Background(),
			RunTitleGenerationInput{Run: &RunSummary{}},
		)

		require.Empty(t, title)
		require.EqualError(t, err, "agent thread title model returned empty response")
	})

	t.Run("empty model content triggers fallback without error", func(t *testing.T) {
		chatModel := &recordingChatModel{
			resp: schema.AssistantMessage("", nil),
		}
		generator := NewModelRunTitleGenerator(
			func(context.Context, int64) (model.BaseChatModel, bool, error) {
				return chatModel, true, nil
			},
		)

		title, err := generator.GenerateTitle(
			context.Background(),
			RunTitleGenerationInput{Run: &RunSummary{}},
		)

		require.NoError(t, err)
		require.Empty(t, title)
	})
}
