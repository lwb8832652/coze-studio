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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectRunJournalMessagesFollowsDeerFlowMessageContract(t *testing.T) {
	run := &RunSummary{
		RunID:    42,
		ThreadID: 7,
		Input: `{
			"messages":[
				{"role":"user","content":"之前的问题"},
				{"role":"assistant","content":"之前的回答"},
				{"role":"user","content":"青岛最佳旅游时间"}
			]
		}`,
		CreatedAt: 1000,
	}
	persistedMessages := []*MessageSummary{
		{
			MessageID: 11,
			ThreadID:  7,
			RunID:     42,
			Role:      MessageRoleUser,
			Content:   "青岛最佳旅游时间",
			CreatedAt: 1001,
		},
		{
			MessageID: 12,
			ThreadID:  7,
			RunID:     42,
			Role:      MessageRoleAssistant,
			Content:   "青岛 4-6 月和 9-10 月最适合旅行。",
			CreatedAt: 1040,
		},
	}
	events := []*RunEventSummary{
		{
			EventID:   21,
			ThreadID:  7,
			RunID:     42,
			EventType: "message.completed",
			Payload: `{
				"role":"assistant",
				"reasoning_content":"需要先查询季节和天气资料。",
				"tool_calls":[{
					"id":"call_web_search",
					"type":"function",
					"function":{
						"name":"web_search",
						"arguments":"{\"query\":\"青岛最佳旅游时间\"}"
					}
				}],
				"usage":{"input_tokens":101,"output_tokens":20}
			}`,
			CreatedAt: 1010,
		},
		{
			EventID:   22,
			ThreadID:  7,
			RunID:     42,
			EventType: "tool.completed",
			Payload: `{
				"role":"tool",
				"tool_name":"web_search",
				"tool_call_id":"call_web_search",
				"content":"青岛春末夏初和秋季天气舒适。"
			}`,
			CreatedAt: 1020,
		},
		{
			EventID:   23,
			ThreadID:  7,
			RunID:     42,
			EventType: "message.completed",
			Payload: `{
				"role":"assistant",
				"content":"青岛 4-6 月和 9-10 月最适合旅行。"
			}`,
			CreatedAt: 1030,
		},
	}

	got := ProjectRunJournalMessages(run, persistedMessages, events)
	require.Len(t, got, 4)

	require.Equal(t, RunJournalMessageTypeHuman, got[0].Type)
	require.Equal(t, "11", got[0].ID)
	require.Equal(t, "青岛最佳旅游时间", got[0].Content)
	require.Equal(t, int64(42), got[0].RunID)

	require.Equal(t, RunJournalMessageTypeAI, got[1].Type)
	require.Equal(t, "event-21", got[1].ID)
	require.Equal(t, "需要先查询季节和天气资料。", got[1].AdditionalKwargs["reasoning_content"])
	require.Len(t, got[1].ToolCalls, 1)
	require.Equal(t, "call_web_search", got[1].ToolCalls[0].ID)
	require.Equal(t, "web_search", got[1].ToolCalls[0].Name)
	require.Equal(t, map[string]any{"query": "青岛最佳旅游时间"}, got[1].ToolCalls[0].Args)
	require.Equal(t, map[string]any{"input_tokens": float64(101), "output_tokens": float64(20)}, got[1].Usage)

	require.Equal(t, RunJournalMessageTypeTool, got[2].Type)
	require.Equal(t, "event-22", got[2].ID)
	require.Equal(t, "web_search", got[2].Name)
	require.Equal(t, "call_web_search", got[2].ToolCallID)
	require.Equal(t, "青岛春末夏初和秋季天气舒适。", got[2].Content)

	require.Equal(t, RunJournalMessageTypeAI, got[3].Type)
	require.Equal(t, "12", got[3].ID)
	require.Equal(t, "青岛 4-6 月和 9-10 月最适合旅行。", got[3].Content)
	require.Empty(t, got[3].ToolCalls)
}

func TestProjectRunJournalMessagesFallsBackToLatestRunInputUser(t *testing.T) {
	run := &RunSummary{
		RunID:    42,
		ThreadID: 7,
		Input: `{
			"messages":[
				{"role":"user","content":"第一轮"},
				{"role":"assistant","content":"第一轮回答"},
				{"role":"human","content":"追问内容"}
			]
		}`,
		CreatedAt: 1000,
	}

	got := ProjectRunJournalMessages(run, nil, nil)
	require.Len(t, got, 1)
	require.Equal(t, RunJournalMessageTypeHuman, got[0].Type)
	require.Equal(t, "run-42-input-human", got[0].ID)
	require.Equal(t, "追问内容", got[0].Content)
}
