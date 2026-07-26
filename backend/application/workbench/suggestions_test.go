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

package workbench

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/internal/testutil"
)

func TestGenerateSuggestionsUsesRecentMessagesAndParsesModelJSON(t *testing.T) {
	ctx := context.Background()
	var gotMessages []*schema.Message
	app := &ApplicationService{
		chatModelProvider: func(_ context.Context, modelType int64) (model.BaseChatModel, bool, error) {
			require.Zero(t, modelType)
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
					gotMessages = in
					return schema.AssistantMessage("<think>hidden</think>\n```json\n[\"还能如何优化路线？\", \"预算怎么控制？\", \"还能如何优化路线？\"]\n```", nil), nil
				},
			}, true, nil
		},
	}

	resp, err := app.GenerateSuggestions(ctx, &GenerateSuggestionsRequest{
		Messages: []SuggestionMessage{
			{Role: "user", Content: "帮我制定武汉三日游攻略"},
			{Role: "assistant", Content: "已经整理了路线和预算。"},
		},
		N: 2,
	})

	require.NoError(t, err)
	require.Equal(t, []string{"还能如何优化路线？", "预算怎么控制？"}, resp.Suggestions)
	require.Len(t, gotMessages, 2)
	require.Equal(t, schema.System, gotMessages[0].Role)
	require.Contains(t, gotMessages[0].Content, "Output MUST be a JSON array")
	require.Equal(t, schema.User, gotMessages[1].Role)
	require.Contains(t, gotMessages[1].Content, "User: 帮我制定武汉三日游攻略")
	require.Contains(t, gotMessages[1].Content, "Assistant: 已经整理了路线和预算。")
}

func TestGenerateSuggestionsReportsModelDisabledError(t *testing.T) {
	ctx := context.Background()
	app := &ApplicationService{
		chatModelProvider: func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return nil, false, nil
		},
	}

	resp, err := app.GenerateSuggestions(ctx, &GenerateSuggestionsRequest{
		Messages: []SuggestionMessage{{Role: "user", Content: "hello"}},
	})

	require.Error(t, err)
	require.Nil(t, resp)
}
