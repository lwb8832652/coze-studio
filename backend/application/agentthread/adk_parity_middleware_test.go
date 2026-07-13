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

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKParityMiddlewareObservesSafeMessagesSummaryAndPromotedTools(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)
	require.NoError(t, tracker.ReplaceMessages([]ADKParityMessage{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
	}, nil))
	ctx := withADKParityStateTracker(context.Background(), tracker)
	dynamicTools := []tool.BaseTool{
		&namedTestTool{name: "weather"},
		&namedTestTool{name: "github"},
	}
	middleware, err := NewADKParityStateMiddleware(ctx, dynamicTools)
	require.NoError(t, err)
	weatherInfo, err := dynamicTools[0].Info(ctx)
	require.NoError(t, err)
	summary := schema.AssistantMessage("internal compacted summary", nil)
	summary.Extra = map[string]any{
		"_eino_summarization_content_type": "summary",
		adkHideFromUIExtraKey:              true,
	}
	hidden := schema.UserMessage("internal tool-search reminder")
	hidden.Extra = map[string]any{"__toolsearch_reminder__": true}
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			summary,
			hidden,
			schema.UserMessage("second question"),
			schema.AssistantMessage("visible answer", nil),
			schema.ToolMessage(`{"secret":"must-not-persist"}`, "call-1"),
		},
		ToolInfos: []*schema.ToolInfo{weatherInfo},
	}

	_, got, err := middleware.BeforeModelRewriteState(ctx, state, &adk.ModelContext{})
	require.NoError(t, err)
	require.Same(t, state, got)

	snapshot := tracker.Snapshot()
	require.Equal(t, []ADKParityMessage{
		{RunID: 1, Role: "user", Content: "second question"},
		{RunID: 1, Role: "assistant", Content: "visible answer"},
	}, snapshot.Messages)
	require.NotNil(t, snapshot.Summary)
	require.NotEmpty(t, snapshot.Summary.Digest)
	require.Equal(t, 3, snapshot.Summary.OriginalMessageCount)
	require.Equal(t, 2, snapshot.Summary.ActiveMessageCount)
	require.NotNil(t, snapshot.PromotedTools)
	require.NotEmpty(t, snapshot.PromotedTools.CatalogHash)
	require.Equal(t, []string{"weather"}, snapshot.PromotedTools.Names)
	require.NotContains(t, snapshot.Messages[0].Content, "tool-search")
	require.NotContains(t, snapshot.Messages[1].Content, "secret")
}

func TestADKParityMiddlewareInvalidatesPromotionsWhenCatalogChanges(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)
	ctx := withADKParityStateTracker(context.Background(), tracker)
	first, err := NewADKParityStateMiddleware(ctx, []tool.BaseTool{
		&namedTestTool{name: "weather"},
	})
	require.NoError(t, err)
	weatherInfo, err := (&namedTestTool{name: "weather"}).Info(ctx)
	require.NoError(t, err)
	_, _, err = first.BeforeModelRewriteState(ctx, &adk.ChatModelAgentState{
		Messages:  []*schema.Message{schema.UserMessage("weather")},
		ToolInfos: []*schema.ToolInfo{weatherInfo},
	}, &adk.ModelContext{})
	require.NoError(t, err)
	firstHash := tracker.Snapshot().PromotedTools.CatalogHash

	second, err := NewADKParityStateMiddleware(ctx, []tool.BaseTool{
		&namedTestTool{name: "github"},
	})
	require.NoError(t, err)
	_, _, err = second.BeforeModelRewriteState(ctx, &adk.ChatModelAgentState{
		Messages: []*schema.Message{schema.UserMessage("github")},
	}, &adk.ModelContext{})
	require.NoError(t, err)

	snapshot := tracker.Snapshot()
	require.NotEqual(t, firstHash, snapshot.PromotedTools.CatalogHash)
	require.Empty(t, snapshot.PromotedTools.Names)
}
