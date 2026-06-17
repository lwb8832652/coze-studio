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
