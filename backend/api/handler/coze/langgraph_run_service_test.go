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

	langgraphapi "github.com/coze-dev/coze-studio/backend/api/model/agent/langgraph"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

func TestLangGraphRunCreateListAndGetHandlers(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs", CreateLangGraphRun)
	h.GET("/api/threads/:thread_id/runs", ListLangGraphRuns)
	h.GET("/api/threads/:thread_id/runs/:run_id", GetLangGraphRun)
	installAgentThreadTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input": map[string]any{
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "帮我分析这个需求",
				},
			},
		},
		"metadata": map[string]any{
			"source": "langgraph_sdk",
		},
		"config": map[string]any{
			"configurable": map[string]any{
				"model_name": "gpt-4.1",
			},
		},
		"context": map[string]any{
			"enable_skills": []string{"research"},
		},
		"stream_mode":        []string{"messages", "updates"},
		"multitask_strategy": "enqueue",
		"on_disconnect":      "continue",
		"durability":         "async",
	})
	require.NoError(t, err)
	createResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	createBody := string(createResp.Result().Body())

	require.Equal(t, http.StatusOK, createResp.Code)
	require.Contains(t, createBody, `"run_id":"2"`)
	require.Contains(t, createBody, `"thread_id":"1"`)
	require.Contains(t, createBody, `"assistant_id":"default"`)
	require.Contains(t, createBody, `"status":"pending"`)
	require.Contains(t, createBody, `"source":"langgraph_sdk"`)
	require.Contains(t, createBody, `"model_name":"gpt-4.1"`)
	require.Contains(t, createBody, `"enable_skills":["research"]`)
	require.Contains(t, createBody, `"stream_mode":["messages","updates"]`)

	listResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs?limit=10&offset=0", nil)
	listBody := string(listResp.Result().Body())

	require.Equal(t, http.StatusOK, listResp.Code)
	require.Contains(t, listBody, `"run_id":"2"`)
	require.Contains(t, listBody, `"thread_id":"1"`)

	getResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2", nil)
	getBody := string(getResp.Result().Body())

	require.Equal(t, http.StatusOK, getResp.Code)
	require.Contains(t, getBody, `"run_id":"2"`)
	require.Contains(t, getBody, `"thread_id":"1"`)
	require.Contains(t, getBody, `"status":"pending"`)
	require.Contains(t, getBody, `"source":"langgraph_sdk"`)
}

func TestLangGraphRunCreateAcceptsFlexibleInputAndStreamMode(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs", CreateLangGraphRun)
	installAgentThreadTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input": []map[string]any{
			{
				"role":    "user",
				"content": "数组输入",
			},
		},
		"stream_mode": "updates",
	})
	require.NoError(t, err)

	createResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(createResp.Result().Body())

	require.Equal(t, http.StatusOK, createResp.Code)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"input":[{"content":"数组输入","role":"user"}]`)
	require.Contains(t, body, `"stream_mode":["updates"]`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: 2})
	require.NoError(t, err)
	require.Equal(t, `[{"content":"数组输入","role":"user"}]`, persisted.Run.Input)
	require.Equal(t, `["updates"]`, persisted.Run.StreamMode)
}

func TestLangGraphRunCreateAcceptsEmptyObjectInput(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs", CreateLangGraphRun)
	installAgentThreadTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input":        map[string]any{},
	})
	require.NoError(t, err)

	createResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(createResp.Result().Body())

	require.Equal(t, http.StatusOK, createResp.Code)
	require.Contains(t, body, `"input":{}`)
}

func TestLangGraphRunListAcceptsCompatibleStatusFilter(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/runs", ListLangGraphRuns)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"状态筛选"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CompleteRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	})
	require.NoError(t, err)

	listResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs?status=success", nil)
	body := string(listResp.Result().Body())

	require.Equal(t, http.StatusOK, listResp.Code)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"status":"success"`)
}

func TestLangGraphRunGetRejectsCrossThreadRun(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/runs/:run_id", GetLangGraphRun)
	installAgentThreadTestService(t)

	threadResp, err := appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:  1,
		UserID:   2,
		Title:    "另一个任务",
		Source:   appagentthread.ThreadSourceAPI,
		Metadata: `{"space_id":"1","title":"另一个任务","source":"api"}`,
	})
	require.NoError(t, err)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"跨线程检查"}]}`,
	})
	require.NoError(t, err)

	getResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/"+strconv.FormatInt(threadResp.Thread.ThreadID, 10)+"/runs/"+strconv.FormatInt(runResp.Run.RunID, 10),
		nil,
	)

	require.Equal(t, http.StatusBadRequest, getResp.Code)
}

func TestLangGraphRunCancelHandlerTransitionsPendingRun(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs/:run_id/cancel", CancelLangGraphRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"取消这次任务"}]}`,
	})
	require.NoError(t, err)

	cancelResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/cancel",
		nil,
	)
	cancelBody := string(cancelResp.Result().Body())

	require.Equal(t, http.StatusOK, cancelResp.Code)
	require.Contains(t, cancelBody, `"run_id":"2"`)
	require.Contains(t, cancelBody, `"thread_id":"1"`)
	require.Contains(t, cancelBody, `"status":"interrupted"`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusCanceled, persisted.Run.Status)
}

func TestLangGraphRunCancelRejectsCrossThreadRun(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs/:run_id/cancel", CancelLangGraphRun)
	installAgentThreadTestService(t)

	threadResp, err := appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:  1,
		UserID:   2,
		Title:    "另一个任务",
		Source:   appagentthread.ThreadSourceAPI,
		Metadata: `{"space_id":"1","title":"另一个任务","source":"api"}`,
	})
	require.NoError(t, err)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"跨线程取消检查"}]}`,
	})
	require.NoError(t, err)

	cancelResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/"+strconv.FormatInt(threadResp.Thread.ThreadID, 10)+"/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/cancel",
		nil,
	)

	require.Equal(t, http.StatusBadRequest, cancelResp.Code)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusPending, persisted.Run.Status)
}

func TestLangGraphRunStreamWritesMetadataEventsAndEnd(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"分析执行流程"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "step.completed",
		Payload:   `{"step_name":"generate_answer","status":"completed"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CompleteRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	})
	require.NoError(t, err)
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamLangGraphRunEvents(context.Background(), writer, langgraphapi.StreamRunRequest{
		ThreadID:   1,
		RunID:      runResp.Run.RunID,
		IntervalMs: 10,
		TimeoutMs:  100,
	}, persisted.Run)
	body := writer.String()

	require.Contains(t, body, "event: metadata")
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, "id: 3")
	require.Contains(t, body, "event: events")
	require.Contains(t, body, `"event_type":"step.completed"`)
	require.Contains(t, body, `"step_name":"generate_answer"`)
	require.Contains(t, body, `"status":"completed"`)
	require.Contains(t, body, "event: end")
	require.Contains(t, body, `"status":"success"`)
}

func TestLangGraphRunStreamSkipsEventsAtOrBeforeCursor(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"游标回放"}]}`,
	})
	require.NoError(t, err)
	first, err := appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "step.started",
		Payload:   `{"step_name":"planner"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "step.completed",
		Payload:   `{"step_name":"planner"}`,
	})
	require.NoError(t, err)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamLangGraphRunEvents(context.Background(), writer, langgraphapi.StreamRunRequest{
		ThreadID:     1,
		RunID:        runResp.Run.RunID,
		AfterEventID: first.Event.EventID,
		IntervalMs:   10,
		TimeoutMs:    1,
	}, runResp.Run)
	body := writer.String()

	require.NotContains(t, body, `"event_type":"step.started"`)
	require.Contains(t, body, `"event_type":"step.completed"`)
}

func TestLangGraphRunStreamRejectsCrossThreadRun(t *testing.T) {
	h := server.Default()
	h.GET("/api/threads/:thread_id/runs/:run_id/stream", StreamLangGraphRun)
	installAgentThreadTestService(t)

	threadResp, err := appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:  1,
		UserID:   2,
		Title:    "另一个任务",
		Source:   appagentthread.ThreadSourceAPI,
		Metadata: `{"space_id":"1","title":"另一个任务","source":"api"}`,
	})
	require.NoError(t, err)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"跨任务流检查"}]}`,
	})
	require.NoError(t, err)

	streamResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/"+strconv.FormatInt(threadResp.Thread.ThreadID, 10)+"/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/stream",
		nil,
	)

	require.Equal(t, http.StatusBadRequest, streamResp.Code)
}

func TestLangGraphRunCreateStreamCreatesRunAndStreamsMetadata(t *testing.T) {
	installAgentThreadTestService(t)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	_, err := createAndStreamLangGraphRun(context.Background(), writer, langgraphapi.CreateStreamRunRequest{
		ThreadID:    1,
		AssistantID: "default",
		Input: map[string]any{
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "创建并流式返回",
				},
			},
		},
		Metadata: map[string]any{
			"source": "langgraph_sdk",
		},
		Config: map[string]any{
			"configurable": map[string]any{
				"model_name": "gpt-4.1",
			},
		},
		StreamMode: []string{"updates"},
		IntervalMs: 1,
		TimeoutMs:  1,
	})
	require.NoError(t, err)
	body := writer.String()

	require.Contains(t, body, "event: metadata")
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"status":"pending"`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: 2})
	require.NoError(t, err)
	require.Equal(t, int64(1), persisted.Run.ThreadID)
	require.Equal(t, appagentthread.RunStatusPending, persisted.Run.Status)
	require.Contains(t, persisted.Run.Metadata, `"source":"langgraph_sdk"`)
	require.Contains(t, persisted.Run.Config, `"model_name":"gpt-4.1"`)
	require.Equal(t, `["updates"]`, persisted.Run.StreamMode)
}

func TestLangGraphRunCreateStreamRejectsMissingInput(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs/stream", CreateLangGraphRunStream)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
	})
	require.NoError(t, err)

	streamResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs/stream?timeout_ms=1",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)

	require.Equal(t, http.StatusBadRequest, streamResp.Code)
}

func TestLangGraphRunJoinHandlerReturnsTerminalRun(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs/:run_id/join", JoinLangGraphRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"等待完成"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CompleteRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	})
	require.NoError(t, err)

	joinResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/join?interval_ms=1&timeout_ms=1",
		nil,
	)
	body := string(joinResp.Result().Body())

	require.Equal(t, http.StatusOK, joinResp.Code)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"status":"success"`)
}

func TestLangGraphRunJoinReturnsCurrentRunOnTimeout(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs/:run_id/join", JoinLangGraphRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"等待超时"}]}`,
	})
	require.NoError(t, err)

	joinResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/join?interval_ms=1&timeout_ms=1",
		nil,
	)
	body := string(joinResp.Result().Body())

	require.Equal(t, http.StatusOK, joinResp.Code)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"status":"pending"`)
}

func TestLangGraphRunJoinRejectsCrossThreadRun(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs/:run_id/join", JoinLangGraphRun)
	h.GET("/api/threads/:thread_id/runs/:run_id/join", JoinLangGraphRunStream)
	installAgentThreadTestService(t)

	threadResp, err := appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:  1,
		UserID:   2,
		Title:    "另一个任务",
		Source:   appagentthread.ThreadSourceAPI,
		Metadata: `{"space_id":"1","title":"另一个任务","source":"api"}`,
	})
	require.NoError(t, err)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"跨任务等待检查"}]}`,
	})
	require.NoError(t, err)

	postResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/"+strconv.FormatInt(threadResp.Thread.ThreadID, 10)+"/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/join?timeout_ms=1",
		nil,
	)
	getResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/"+strconv.FormatInt(threadResp.Thread.ThreadID, 10)+"/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/join?timeout_ms=1",
		nil,
	)

	require.Equal(t, http.StatusBadRequest, postResp.Code)
	require.Equal(t, http.StatusBadRequest, getResp.Code)
}

func TestLangGraphRunJoinStreamWritesEventsAndEnd(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"join stream"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "step.completed",
		Payload:   `{"step_name":"join"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CompleteRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	})
	require.NoError(t, err)
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	joinLangGraphRunStreamEvents(context.Background(), writer, langgraphapi.JoinRunRequest{
		ThreadID:   1,
		RunID:      runResp.Run.RunID,
		IntervalMs: 1,
		TimeoutMs:  1,
	}, persisted.Run)
	body := writer.String()

	require.Contains(t, body, "event: metadata")
	require.Contains(t, body, "event: events")
	require.Contains(t, body, `"event_type":"step.completed"`)
	require.Contains(t, body, "event: end")
	require.Contains(t, body, `"status":"success"`)
}

func TestLangGraphStatelessRunGetHandlerReturnsRun(t *testing.T) {
	h := server.Default()
	h.GET("/api/runs/:run_id", GetLangGraphStatelessRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"stateless get"}]}`,
	})
	require.NoError(t, err)

	getResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/runs/"+strconv.FormatInt(runResp.Run.RunID, 10),
		nil,
	)
	body := string(getResp.Result().Body())

	require.Equal(t, http.StatusOK, getResp.Code)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"status":"pending"`)
}

func TestLangGraphStatelessRunCancelHandlerTransitionsPendingRun(t *testing.T) {
	h := server.Default()
	h.POST("/api/runs/:run_id/cancel", CancelLangGraphStatelessRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"stateless cancel"}]}`,
	})
	require.NoError(t, err)

	cancelResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/cancel",
		nil,
	)
	body := string(cancelResp.Result().Body())

	require.Equal(t, http.StatusOK, cancelResp.Code)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"status":"interrupted"`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusCanceled, persisted.Run.Status)
}

func TestLangGraphStatelessRunJoinHandlerReturnsTerminalRun(t *testing.T) {
	h := server.Default()
	h.POST("/api/runs/:run_id/join", JoinLangGraphStatelessRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"stateless join"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CompleteRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	})
	require.NoError(t, err)

	joinResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/join?interval_ms=1&timeout_ms=1",
		nil,
	)
	body := string(joinResp.Result().Body())

	require.Equal(t, http.StatusOK, joinResp.Code)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"status":"success"`)
}

func TestLangGraphStatelessRunStreamWritesEventsAndEnd(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"stateless stream"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "step.completed",
		Payload:   `{"step_name":"stateless"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CompleteRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	})
	require.NoError(t, err)
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamLangGraphStatelessRunEvents(context.Background(), writer, langgraphapi.StatelessStreamRunRequest{
		RunID:      runResp.Run.RunID,
		IntervalMs: 1,
		TimeoutMs:  1,
	}, persisted.Run)
	body := writer.String()

	require.Contains(t, body, "event: metadata")
	require.Contains(t, body, "event: events")
	require.Contains(t, body, `"event_type":"step.completed"`)
	require.Contains(t, body, "event: end")
	require.Contains(t, body, `"status":"success"`)
}

func TestLangGraphStatelessRunCreateHandlerCreatesBackingThreadAndRun(t *testing.T) {
	h := server.Default()
	h.POST("/api/runs", CreateLangGraphStatelessRun)
	installAgentThreadTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input": map[string]any{
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "一次性任务",
				},
			},
		},
		"metadata": map[string]any{
			"space_id": "99",
			"user_id":  "88",
			"title":    "一次性分析任务",
			"source":   "api",
		},
		"config": map[string]any{
			"configurable": map[string]any{
				"model_name": "gpt-4.1",
			},
		},
		"stream_mode": []string{"updates"},
	})
	require.NoError(t, err)

	createResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/runs",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(createResp.Result().Body())

	require.Equal(t, http.StatusOK, createResp.Code)
	require.Contains(t, body, `"run_id":"3"`)
	require.Contains(t, body, `"thread_id":"2"`)
	require.Contains(t, body, `"status":"pending"`)
	require.Contains(t, body, `"source":"api"`)
	require.Contains(t, body, `"model_name":"gpt-4.1"`)
	require.Contains(t, body, `"stream_mode":["updates"]`)

	threadResp, err := appagentthread.SVC.GetThread(context.Background(), &appagentthread.GetThreadRequest{ThreadID: 2})
	require.NoError(t, err)
	require.Equal(t, int64(99), threadResp.Thread.SpaceID)
	require.Equal(t, int64(88), threadResp.Thread.CreatorID)
	require.Equal(t, "一次性分析任务", threadResp.Thread.Title)
	require.Equal(t, appagentthread.ThreadSourceAPI, threadResp.Thread.Source)
	require.Contains(t, threadResp.Thread.Metadata, `"title":"一次性分析任务"`)
}

func TestLangGraphStatelessRunCreateRejectsMissingInput(t *testing.T) {
	h := server.Default()
	h.POST("/api/runs", CreateLangGraphStatelessRun)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"metadata": map[string]any{
			"title": "缺少 input",
		},
	})
	require.NoError(t, err)

	createResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/runs",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)

	require.Equal(t, http.StatusBadRequest, createResp.Code)
}

func TestLangGraphStatelessRunCreateStreamCreatesRunAndStreamsMetadata(t *testing.T) {
	installAgentThreadTestService(t)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	resp, err := createAndStreamLangGraphStatelessRun(context.Background(), writer, langgraphapi.StatelessCreateStreamRunRequest{
		AssistantID: "default",
		Input: map[string]any{
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "一次性流式任务",
				},
			},
		},
		Metadata: map[string]any{
			"title":  "一次性流式任务",
			"source": "api",
		},
		StreamMode: []string{"updates"},
		IntervalMs: 1,
		TimeoutMs:  1,
	})
	require.NoError(t, err)
	body := writer.String()

	require.Equal(t, int64(3), resp.Run.RunID)
	require.Equal(t, int64(2), resp.Run.ThreadID)
	require.Contains(t, body, "event: metadata")
	require.Contains(t, body, `"run_id":"3"`)
	require.Contains(t, body, `"thread_id":"2"`)
	require.Contains(t, body, `"status":"pending"`)
}
