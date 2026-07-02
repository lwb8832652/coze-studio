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

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

func TestLangGraphThreadCreateGetAndSearchHandlers(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads", CreateLangGraphThread)
	h.GET("/api/threads/:thread_id", GetLangGraphThread)
	h.POST("/api/threads/search", SearchLangGraphThreads)
	installAgentThreadTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"space_id": "7",
			"user_id":  "9",
			"title":    "LangGraph 兼容任务",
			"source":   "api",
		},
	})
	require.NoError(t, err)
	createResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	createBody := string(createResp.Result().Body())

	require.Equal(t, http.StatusOK, createResp.Code)
	require.Contains(t, createBody, `"thread_id":"2"`)
	require.Contains(t, createBody, `"status":"idle"`)
	require.Contains(t, createBody, `"created_at":`)
	require.Contains(t, createBody, `"updated_at":`)
	require.Contains(t, createBody, `"metadata":`)
	require.Contains(t, createBody, `"source":"api"`)
	require.Contains(t, createBody, `"space_id":"7"`)
	require.Contains(t, createBody, `"title":"LangGraph 兼容任务"`)
	require.Contains(t, createBody, `"user_id":"9"`)
	require.Contains(t, createBody, `"values":{"messages":[]}`)

	getResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/2", nil)
	getBody := string(getResp.Result().Body())

	require.Equal(t, http.StatusOK, getResp.Code)
	require.Contains(t, getBody, `"thread_id":"2"`)
	require.Contains(t, getBody, `"source":"api"`)
	require.Contains(t, getBody, `"space_id":"7"`)
	require.Contains(t, getBody, `"title":"LangGraph 兼容任务"`)
	require.Contains(t, getBody, `"user_id":"9"`)
	require.Contains(t, getBody, `"values":{"messages":[]}`)

	searchPayload, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"space_id": "7",
		},
		"limit":  10,
		"offset": 0,
	})
	require.NoError(t, err)
	searchResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/search",
		&ut.Body{Body: bytes.NewBuffer(searchPayload), Len: len(searchPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	searchBody := string(searchResp.Result().Body())

	require.Equal(t, http.StatusOK, searchResp.Code)
	require.Contains(t, searchBody, `"thread_id":"2"`)
	require.NotContains(t, searchBody, `"thread_id":"1"`)
}

func TestLangGraphThreadPatchMergesMetadataAndPreservesIdentity(t *testing.T) {
	h := server.Default()
	h.PATCH("/api/threads/:thread_id", PatchLangGraphThread)
	h.GET("/api/threads/:thread_id", GetLangGraphThread)
	installAgentThreadTestService(t)

	patchPayload, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"title":       "Patched title metadata",
			"custom":      "new",
			"space_id":    "999",
			"user_id":     "888",
			"creator_id":  "777",
			"thread_id":   "666",
			"created_at":  "bad",
			"updated_at":  "bad",
			"status":      "running",
			"source":      "evil",
			"legacy_task": "evil",
		},
	})
	require.NoError(t, err)

	patchResp := ut.PerformRequest(
		h.Engine,
		http.MethodPatch,
		"/api/threads/1",
		&ut.Body{Body: bytes.NewBuffer(patchPayload), Len: len(patchPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	patchBody := string(patchResp.Result().Body())

	require.Equal(t, http.StatusOK, patchResp.Code)
	require.Contains(t, patchBody, `"thread_id":"1"`)
	require.Contains(t, patchBody, `"custom":"new"`)
	require.Contains(t, patchBody, `"title":"Patched title metadata"`)
	require.Contains(t, patchBody, `"space_id":"1"`)
	require.Contains(t, patchBody, `"user_id":"2"`)
	require.Contains(t, patchBody, `"source":"web"`)
	require.Contains(t, patchBody, `"status":"idle"`)
	require.NotContains(t, patchBody, `"thread_id":"666"`)
	require.NotContains(t, patchBody, `"creator_id":"777"`)

	getResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1", nil)
	getBody := string(getResp.Result().Body())
	require.Equal(t, http.StatusOK, getResp.Code)
	require.Contains(t, getBody, `"custom":"new"`)
	require.Contains(t, getBody, `"space_id":"1"`)
	require.Contains(t, getBody, `"user_id":"2"`)
	require.Contains(t, getBody, `"source":"web"`)
}

func TestLangGraphThreadDeleteRemovesThreadData(t *testing.T) {
	h := server.Default()
	h.DELETE("/api/threads/:thread_id", DeleteLangGraphThread)
	h.GET("/api/threads/:thread_id", GetLangGraphThread)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"delete me"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
		Role:     appagentthread.MessageRoleUser,
		Content:  "delete me",
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "step.completed",
		Payload:   `{"step":"done"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		ChannelValues:   `{"messages":[{"role":"user","content":"delete me"}]}`,
		ChannelVersions: `{}`,
		PendingSends:    `[]`,
		Metadata:        `{"source":"test"}`,
	})
	require.NoError(t, err)

	deleteResp := ut.PerformRequest(h.Engine, http.MethodDelete, "/api/threads/1", nil)
	deleteBody := string(deleteResp.Result().Body())

	require.Equal(t, http.StatusOK, deleteResp.Code)
	require.Contains(t, deleteBody, `"success":true`)
	require.Contains(t, deleteBody, `"Deleted local thread data for 1"`)

	getResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1", nil)
	require.NotEqual(t, http.StatusOK, getResp.Code)

	messagesResp, err := appagentthread.SVC.ListMessages(context.Background(), &appagentthread.ListMessagesRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Empty(t, messagesResp.Messages)
	require.Zero(t, messagesResp.Total)

	runsResp, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Empty(t, runsResp.Runs)
	require.Zero(t, runsResp.Total)
}

func TestLangGraphThreadSearchUsesMetadataSpaceID(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/search", SearchLangGraphThreads)
	installAgentThreadTestService(t)

	_, err := appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:  8,
		UserID:   9,
		Title:    "另一个空间任务",
		Source:   appagentthread.ThreadSourceAPI,
		Metadata: `{"space_id":"8","title":"另一个空间任务","source":"api"}`,
	})
	require.NoError(t, err)

	searchPayload, err := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"space_id": "1",
		},
		"limit":  10,
		"offset": 0,
	})
	require.NoError(t, err)
	searchResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/search",
		&ut.Body{Body: bytes.NewBuffer(searchPayload), Len: len(searchPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	searchBody := string(searchResp.Result().Body())

	require.Equal(t, http.StatusOK, searchResp.Code)
	require.Contains(t, searchBody, `"thread_id":"1"`)
	require.Contains(t, searchBody, `"title":"任务列表"`)
	require.NotContains(t, searchBody, `"另一个空间任务"`)
}

func TestLangGraphThreadStateHandlerReturnsMessagesAndConfig(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/state", GetLangGraphThreadState)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"state run"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
		Role:     appagentthread.MessageRoleUser,
		Content:  "请总结任务状态",
		Metadata: `{"source":"test"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
		Role:     appagentthread.MessageRoleAssistant,
		Content:  "当前任务正在整理结果。",
	})
	require.NoError(t, err)

	stateResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/state", nil)
	body := string(stateResp.Result().Body())

	require.Equal(t, http.StatusOK, stateResp.Code)
	require.Contains(t, body, `"messages":[`)
	require.Contains(t, body, `"role":"user"`)
	require.Contains(t, body, `"content":"请总结任务状态"`)
	require.Contains(t, body, `"role":"assistant"`)
	require.Contains(t, body, `"content":"当前任务正在整理结果。"`)
	require.Contains(t, body, `"next":[]`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"checkpoint_id":"thread-1-latest"`)
	require.Contains(t, body, `"source":"web"`)
	require.Contains(t, body, `"checkpoint_source":"thread_snapshot"`)
}

func TestLangGraphThreadStateHandlerPrefersLatestCheckpoint(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/state", GetLangGraphThreadState)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"checkpoint run"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
		Role:     appagentthread.MessageRoleUser,
		Content:  "fallback message should not be used",
	})
	require.NoError(t, err)
	checkpointResp, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:           1,
		RunID:              runResp.Run.RunID,
		ParentCheckpointID: 99,
		CheckpointNS:       "planner",
		ChannelValues:      `{"messages":[{"role":"assistant","content":"checkpoint state"}],"artifacts":{"report_id":"artifact-1"},"memory":{"items":["from-checkpoint"]}}`,
		ChannelVersions:    `{"messages":2,"artifacts":1}`,
		PendingSends:       `[{"node":"tools"}]`,
		Metadata:           `{"source":"checkpoint","runtime":"go"}`,
	})
	require.NoError(t, err)

	stateResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/state", nil)
	body := string(stateResp.Result().Body())

	require.Equal(t, http.StatusOK, stateResp.Code)
	require.Contains(t, body, `"checkpoint_id":"`+strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10)+`"`)
	require.Contains(t, body, `"checkpoint_ns":"planner"`)
	require.Contains(t, body, `"content":"checkpoint state"`)
	require.NotContains(t, body, "fallback message should not be used")
	require.Contains(t, body, `"next":["tools"]`)
	require.Contains(t, body, `"report_id":"artifact-1"`)
	require.Contains(t, body, `"checkpoint_source":"checkpoint"`)
	require.Contains(t, body, `"runtime":"go"`)
	require.Contains(t, body, `"parent_checkpoint_id":"99"`)
}

func TestLangGraphThreadStatePostHandlerMergesValuesAndSyncsTitle(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/state", PostLangGraphThreadState)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"state update run"}]}`,
	})
	require.NoError(t, err)
	base, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "planner",
		ChannelValues:   `{"messages":[{"role":"assistant","content":"base message"}],"title":"旧标题","thread_data":{"city":"武汉"}}`,
		ChannelVersions: `{"messages":1,"title":1,"thread_data":1}`,
		PendingSends:    `[]`,
		Metadata:        `{"source":"checkpoint","step":1}`,
	})
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{
		"checkpoint_id": strconv.FormatInt(base.Checkpoint.CheckpointID, 10),
		"as_node":       "manual_update",
		"values": map[string]any{
			"title":       "青岛旅游计划",
			"thread_data": map[string]any{"city": "青岛", "days": 3},
			"todos":       []map[string]any{{"content": "补充最佳旅行季节"}},
		},
	})
	require.NoError(t, err)

	updateResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/state",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(updateResp.Result().Body())

	require.Equal(t, http.StatusOK, updateResp.Code)
	require.Contains(t, body, `"content":"base message"`)
	require.Contains(t, body, `"title":"青岛旅游计划"`)
	require.Contains(t, body, `"city":"青岛"`)
	require.Contains(t, body, `"days":3`)
	require.Contains(t, body, `"content":"补充最佳旅行季节"`)
	require.Contains(t, body, `"parent_checkpoint_id":"`+strconv.FormatInt(base.Checkpoint.CheckpointID, 10)+`"`)
	require.Contains(t, body, `"source":"update"`)
	require.Contains(t, body, `"writes":{"manual_update"`)
	require.NotContains(t, body, `"checkpoint_id":"`+strconv.FormatInt(base.Checkpoint.CheckpointID, 10)+`"`)

	threadResp, err := appagentthread.SVC.GetThread(context.Background(), &appagentthread.GetThreadRequest{ThreadID: 1})
	require.NoError(t, err)
	require.Equal(t, "青岛旅游计划", threadResp.Thread.Title)
}

func TestLangGraphThreadHistoryHandlerReturnsEventSnapshots(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/history", GetLangGraphThreadHistory)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"history run"}]}`,
	})
	require.NoError(t, err)
	eventResp, err := appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "node.update",
		Payload:   `{"node":"agent","delta":{"status":"planning"}}`,
	})
	require.NoError(t, err)

	historyResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/history?limit=10", nil)
	body := string(historyResp.Result().Body())

	require.Equal(t, http.StatusOK, historyResp.Code)
	require.Contains(t, body, `"checkpoint_id":"event-`+strconv.FormatInt(eventResp.Event.EventID, 10)+`"`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"source":"event_log"`)
	require.Contains(t, body, `"event_type":"node.update"`)
	require.Contains(t, body, `"status":"planning"`)
}

func TestLangGraphThreadHistoryHandlerPrefersCheckpointHistory(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/history", GetLangGraphThreadHistory)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"checkpoint history"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "node.update",
		Payload:   `{"node":"agent","delta":{"status":"event-fallback"}}`,
	})
	require.NoError(t, err)
	first, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "planner",
		ChannelValues:   `{"messages":[{"role":"assistant","content":"older checkpoint"}]}`,
		ChannelVersions: `{"messages":1}`,
		PendingSends:    `[]`,
		Metadata:        `{"source":"checkpoint","step":1}`,
	})
	require.NoError(t, err)
	second, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:           1,
		RunID:              runResp.Run.RunID,
		ParentCheckpointID: first.Checkpoint.CheckpointID,
		CheckpointNS:       "tools",
		ChannelValues:      `{"messages":[{"role":"assistant","content":"newer checkpoint"}],"tool_results":{"search":"ok"}}`,
		ChannelVersions:    `{"messages":2,"tool_results":1}`,
		PendingSends:       `[{"node":"final"}]`,
		Metadata:           `{"source":"checkpoint","step":2}`,
	})
	require.NoError(t, err)

	historyResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/history?limit=10", nil)
	body := string(historyResp.Result().Body())

	require.Equal(t, http.StatusOK, historyResp.Code)
	require.Contains(t, body, `"checkpoint_id":"`+strconv.FormatInt(second.Checkpoint.CheckpointID, 10)+`"`)
	require.Contains(t, body, `"checkpoint_id":"`+strconv.FormatInt(first.Checkpoint.CheckpointID, 10)+`"`)
	require.Contains(t, body, `"checkpoint_ns":"tools"`)
	require.Contains(t, body, `"content":"newer checkpoint"`)
	require.Contains(t, body, `"tool_results":{"search":"ok"}`)
	require.Contains(t, body, `"next":["final"]`)
	require.Contains(t, body, `"parent_checkpoint_id":"`+strconv.FormatInt(first.Checkpoint.CheckpointID, 10)+`"`)
	require.Contains(t, body, `"checkpoint_source":"checkpoint"`)
	require.NotContains(t, body, "event-fallback")
}

func TestLangGraphThreadHistoryPostHandlerAcceptsDeerFlowBodyCursor(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/history", PostLangGraphThreadHistory)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"post history"}]}`,
	})
	require.NoError(t, err)
	first, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "planner",
		ChannelValues:   `{"messages":[{"role":"assistant","content":"older checkpoint"}],"todos":[{"content":"older"}]}`,
		ChannelVersions: `{"messages":1}`,
		PendingSends:    `[]`,
		Metadata:        `{"source":"checkpoint","step":1}`,
	})
	require.NoError(t, err)
	second, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:           1,
		RunID:              runResp.Run.RunID,
		ParentCheckpointID: first.Checkpoint.CheckpointID,
		CheckpointNS:       "tools",
		ChannelValues:      `{"messages":[{"role":"assistant","content":"newer checkpoint"}],"tool_results":{"search":"ok"}}`,
		ChannelVersions:    `{"messages":2,"tool_results":1}`,
		PendingSends:       `[{"node":"final"}]`,
		Metadata:           `{"source":"checkpoint","step":2}`,
	})
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{
		"limit":  1,
		"before": strconv.FormatInt(second.Checkpoint.CheckpointID, 10),
	})
	require.NoError(t, err)

	historyResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/history",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(historyResp.Result().Body())

	require.Equal(t, http.StatusOK, historyResp.Code)
	require.Contains(t, body, `"checkpoint_id":"`+strconv.FormatInt(first.Checkpoint.CheckpointID, 10)+`"`)
	require.Contains(t, body, `"parent_checkpoint_id":null`)
	require.Contains(t, body, `"checkpoint_ns":"planner"`)
	require.Contains(t, body, `"todos":[{"content":"older"}]`)
	require.NotContains(t, body, `"checkpoint_id":"`+strconv.FormatInt(second.Checkpoint.CheckpointID, 10)+`"`)
	require.NotContains(t, body, "newer checkpoint")
}

func TestLangGraphThreadHistoryPostHandlerRedactsADKCheckpointEnvelope(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/history", PostLangGraphThreadHistory)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"adk checkpoint"}]}`,
	})
	require.NoError(t, err)
	envelope := appagentthread.ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(appagentthread.RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "thread-1/run-2",
		MessageType:     "schema.Message",
		Checkpoint:      []byte("raw-secret-checkpoint-bytes"),
		Interrupts: map[string]appagentthread.ADKInterruptItem{
			"interrupt-1": {
				ID:          "interrupt-1",
				Address:     "agent.ask",
				IsRootCause: true,
			},
		},
		RunRevision: 1,
		CreatedAt:   1777252410411,
	}
	rawEnvelope, err := envelope.Marshal()
	require.NoError(t, err)
	_, err = appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "eino.adk",
		RuntimeType:     string(appagentthread.RuntimeModeEinoADK),
		RuntimeKey:      "thread-1/run-2",
		EnvelopeVersion: 1,
		ChannelValues:   string(rawEnvelope),
		ChannelVersions: `{"runtime":1}`,
		PendingSends:    `[]`,
		Metadata:        `{"source":"agent_harness","checkpoint_phase":"interrupt","status":"running"}`,
	})
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{"limit": 10})
	require.NoError(t, err)

	historyResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/history",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(historyResp.Result().Body())

	require.Equal(t, http.StatusOK, historyResp.Code)
	require.Contains(t, body, `"checkpoint_ns":"eino.adk"`)
	require.Contains(t, body, `"next":["interrupt-1"]`)
	require.Contains(t, body, `"runtime":"eino_adk"`)
	require.NotContains(t, body, "checkpoint_bytes")
	require.NotContains(t, body, "raw-secret-checkpoint-bytes")
	require.NotContains(t, body, "runtime_key")
}

func TestLangGraphCheckpointResumeReadinessHandlerReturnsPendingSends(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/checkpoints/:checkpoint_id/resume", GetLangGraphCheckpointResumeReadiness)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"resume me"}]}`,
	})
	require.NoError(t, err)
	checkpointResp, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "harness.terminal",
		ChannelValues:   `{"messages":[{"role":"assistant","content":"partial"}]}`,
		ChannelVersions: `{"messages":1}`,
		PendingSends:    `[{"node":"generate_answer","step_id":"step-1"},{"node":"finalize","step_id":"step-2"}]`,
		Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed","error_type":"step_error"}`,
	})
	require.NoError(t, err)

	resumeResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/1/checkpoints/"+strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10)+"/resume",
		nil,
	)
	body := string(resumeResp.Result().Body())

	require.Equal(t, http.StatusOK, resumeResp.Code)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"checkpoint_id":"`+strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10)+`"`)
	require.Contains(t, body, `"checkpoint_ns":"harness.terminal"`)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"resumable":true`)
	require.Contains(t, body, `"resume_from":"pending_sends"`)
	require.Contains(t, body, `"reason":"pending_sends_available"`)
	require.Contains(t, body, `"status":"failed"`)
	require.Contains(t, body, `"error_type":"step_error"`)
	require.Contains(t, body, `"pending_sends":["generate_answer","finalize"]`)
	require.Contains(t, body, `"checkpoint_source":"checkpoint"`)
}

func TestLangGraphCheckpointResumeReadinessHandlerReturnsNotResumableForSucceededCheckpoint(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/checkpoints/:checkpoint_id/resume", GetLangGraphCheckpointResumeReadiness)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"done"}]}`,
	})
	require.NoError(t, err)
	checkpointResp, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "harness.terminal",
		ChannelValues:   `{"messages":[{"role":"assistant","content":"done"}]}`,
		ChannelVersions: `{"messages":1}`,
		PendingSends:    `[]`,
		Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"succeeded"}`,
	})
	require.NoError(t, err)

	resumeResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/1/checkpoints/"+strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10)+"/resume",
		nil,
	)
	body := string(resumeResp.Result().Body())

	require.Equal(t, http.StatusOK, resumeResp.Code)
	require.Contains(t, body, `"resumable":false`)
	require.Contains(t, body, `"reason":"checkpoint_already_succeeded"`)
	require.Contains(t, body, `"status":"succeeded"`)
	require.Contains(t, body, `"pending_sends":[]`)
}
