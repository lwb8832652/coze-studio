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
	"errors"
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
	require.JSONEq(t, `[{"content":"数组输入","role":"user"}]`, persisted.Run.Input)
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

func TestLangGraphRunCreateCreatesProtectedResumeRunFromReadyCheckpoint(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs", CreateLangGraphRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"resume guard"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "source-worker",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.FailRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID:        runResp.Run.RunID,
		From:         appagentthread.RunStatusRunning,
		WorkerID:     "source-worker",
		ErrorCode:    "step_error",
		ErrorMessage: "source run failed",
	})
	require.NoError(t, err)
	checkpointResp, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "harness.terminal",
		ChannelValues:   `{"messages":[{"role":"assistant","content":"partial"}]}`,
		ChannelVersions: `{"messages":1}`,
		PendingSends:    `[{"node":"generate_answer","step_id":"step-1"}]`,
		Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed","error_type":"step_error"}`,
	})
	require.NoError(t, err)
	createPayload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input": map[string]any{
			"messages": []map[string]any{{"role": "user", "content": "resume"}},
		},
		"metadata": map[string]any{
			"client": "langgraph_sdk",
		},
		"command": map[string]any{
			"resume": map[string]any{
				"checkpoint_id": strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10),
			},
		},
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

	var apiRun langgraphapi.Run
	require.NoError(t, json.Unmarshal(createResp.Result().Body(), &apiRun))
	require.Equal(t, "queued", apiRun.Status)
	require.Equal(t, "langgraph_sdk", apiRun.Metadata["client"])

	resumeCommand, ok := apiRun.Command["resume"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10), resumeCommand["checkpoint_id"])
	require.Equal(t, "harness.terminal", resumeCommand["checkpoint_ns"])
	require.Equal(t, "pending_sends", resumeCommand["resume_from"])
	require.Equal(t, "pending_sends_available", resumeCommand["reason"])
	require.Equal(t, "worker_replay_not_enabled", resumeCommand["guard"])

	resumeMetadata, ok := apiRun.Metadata["checkpoint_resume"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, resumeMetadata["protected_from_worker_claim"])
	require.Equal(t, strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10), resumeMetadata["checkpoint_id"])

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: langGraphInt64Value(apiRun.RunID)})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusQueued, persisted.Run.Status)

	claimed, err := appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    10,
	})
	require.NoError(t, err)
	require.Empty(t, claimed.Runs)
	require.Contains(t, body, `"checkpoint_resume"`)
}

func TestLangGraphRunCreatePreservesResumeTargets(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs", CreateLangGraphRun)
	installAgentThreadTestService(t)
	_, checkpointID := createInterruptedHumanInteractionRunWithCheckpoint(t)

	createPayload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input":        map[string]any{},
		"command": map[string]any{
			"resume": map[string]any{
				"checkpoint_id": strconv.FormatInt(checkpointID, 10),
				"targets": map[string]any{
					"interrupt-1": map[string]any{
						"schema":         "coze.human_interaction_response.v1",
						"interaction_id": "hi_1",
						"kind":           "clarification",
						"decision":       "answered",
						"answer":         "最近 7 天",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	createResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)

	require.Equal(t, http.StatusOK, createResp.Code)
	var apiRun langgraphapi.Run
	require.NoError(t, json.Unmarshal(createResp.Result().Body(), &apiRun))
	resumeCommand, ok := apiRun.Command["resume"].(map[string]any)
	require.True(t, ok)
	targets, ok := resumeCommand["targets"].(map[string]any)
	require.True(t, ok)
	target, ok := targets["interrupt-1"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "最近 7 天", target["answer"])
	require.Equal(t, "interrupt", resumeCommand["resume_from"])
}

func TestLangGraphRunCreateRejectsUnknownResumeTarget(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs", CreateLangGraphRun)
	installAgentThreadTestService(t)
	_, checkpointID := createInterruptedHumanInteractionRunWithCheckpoint(t)

	createPayload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input":        map[string]any{},
		"command": map[string]any{
			"resume": map[string]any{
				"checkpoint_id": strconv.FormatInt(checkpointID, 10),
				"targets": map[string]any{
					"missing": map[string]any{
						"schema":         "coze.human_interaction_response.v1",
						"interaction_id": "hi_1",
						"kind":           "clarification",
						"decision":       "answered",
						"answer":         "最近 7 天",
					},
				},
			},
		},
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

	require.Equal(t, http.StatusBadRequest, createResp.Code)
	require.Contains(t, body, "resume target missing is not present in checkpoint interrupts")
}

func TestLangGraphRunCreateRejectsNotResumableCheckpoint(t *testing.T) {
	h := server.Default()
	h.POST("/api/threads/:thread_id/runs", CreateLangGraphRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"done guard"}]}`,
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
	createPayload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input":        map[string]any{},
		"config": map[string]any{
			"configurable": map[string]any{
				"checkpoint_id": strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10),
			},
		},
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

	require.Equal(t, http.StatusBadRequest, createResp.Code)
	require.Contains(t, body, "checkpoint is not resumable")
	require.Contains(t, body, "checkpoint_already_succeeded")
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

func TestLangGraphRunStreamReconnectReplaysEventsExactlyOnceInIDOrder(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"断线续传"}]}`,
	})
	require.NoError(t, err)

	expectedIDs := make([]int64, 0, 3)
	for _, eventType := range []string{"step.started", "llm.token", "step.completed"} {
		eventResp, appendErr := appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
			ThreadID:  1,
			RunID:     runResp.Run.RunID,
			EventType: eventType,
			Payload:   `{"step_name":"planner"}`,
		})
		require.NoError(t, appendErr)
		expectedIDs = append(expectedIDs, eventResp.Event.EventID)
	}

	firstWriter := &cursorRecordingLangGraphRunStreamWriter{failAfterEvents: 1}
	streamLangGraphRunEvents(context.Background(), firstWriter, langgraphapi.StreamRunRequest{
		ThreadID:   1,
		RunID:      runResp.Run.RunID,
		IntervalMs: 1,
		TimeoutMs:  1,
	}, runResp.Run)
	require.Len(t, firstWriter.eventIDs, 1)

	secondWriter := &cursorRecordingLangGraphRunStreamWriter{}
	streamLangGraphRunEvents(context.Background(), secondWriter, langgraphapi.StreamRunRequest{
		ThreadID:     1,
		RunID:        runResp.Run.RunID,
		AfterEventID: firstWriter.eventIDs[0],
		IntervalMs:   1,
		TimeoutMs:    1,
	}, runResp.Run)

	replayedIDs := append(append([]int64{}, firstWriter.eventIDs...), secondWriter.eventIDs...)
	require.Equal(t, expectedIDs, replayedIDs)
	require.Len(t, secondWriter.eventIDs, 2)
}

func TestLangGraphRunStreamRespectsUpdatesMode(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"updates mode"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "node.update",
		Payload:   `{"node":"agent","delta":{"messages":[{"role":"assistant","content":"收到"}]}}`,
	})
	require.NoError(t, err)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamLangGraphRunEvents(context.Background(), writer, langgraphapi.StreamRunRequest{
		ThreadID:   1,
		RunID:      runResp.Run.RunID,
		StreamMode: "updates",
		IntervalMs: 1,
		TimeoutMs:  1,
	}, runResp.Run)
	body := writer.String()

	require.Contains(t, body, "event: updates")
	require.NotContains(t, body, "event: events")
	require.Contains(t, body, `"agent":{"messages":[{`)
	require.Contains(t, body, `"content":"收到"`)
	require.Contains(t, body, `"role":"assistant"`)
}

func TestLangGraphRunStreamRespectsMessagesMode(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"messages mode"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "llm.token",
		Payload:   `{"node":"agent","chunk":{"content":"你"}}`,
	})
	require.NoError(t, err)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamLangGraphRunEvents(context.Background(), writer, langgraphapi.StreamRunRequest{
		ThreadID:   1,
		RunID:      runResp.Run.RunID,
		StreamMode: "messages",
		IntervalMs: 1,
		TimeoutMs:  1,
	}, runResp.Run)
	body := writer.String()

	require.Contains(t, body, "event: messages")
	require.NotContains(t, body, "event: events")
	require.Contains(t, body, `"chunk":{"content":"你"}`)
	require.Contains(t, body, `"node":"agent"`)
}

func TestLangGraphRunCreateStreamUsesRequestedStreamMode(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:   1,
		Input:      `{"messages":[{"role":"user","content":"create stream mode"}]}`,
		StreamMode: `["updates"]`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "node.update",
		Payload:   `{"node":"agent","delta":{"status":"planning"}}`,
	})
	require.NoError(t, err)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamCreatedLangGraphRun(context.Background(), writer, langgraphapi.CreateStreamRunRequest{
		ThreadID:   1,
		StreamMode: []string{"updates"},
		IntervalMs: 1,
		TimeoutMs:  1,
	}, runResp)
	body := writer.String()

	require.Contains(t, body, "event: updates")
	require.NotContains(t, body, "event: events")
	require.Contains(t, body, `"agent":{"status":"planning"}`)
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

func TestLangGraphRunCreateStreamCreatesProtectedResumeRunFromReadyCheckpoint(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"resume stream guard"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "source-worker",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.FailRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID:        runResp.Run.RunID,
		From:         appagentthread.RunStatusRunning,
		WorkerID:     "source-worker",
		ErrorCode:    "step_error",
		ErrorMessage: "source run failed",
	})
	require.NoError(t, err)
	checkpointResp, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "harness.terminal",
		ChannelValues:   `{"messages":[{"role":"assistant","content":"partial"}]}`,
		ChannelVersions: `{"messages":1}`,
		PendingSends:    `[{"node":"generate_answer","step_id":"step-1"}]`,
		Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed","error_type":"step_error"}`,
	})
	require.NoError(t, err)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	resp, err := createAndStreamLangGraphRun(context.Background(), writer, langgraphapi.CreateStreamRunRequest{
		ThreadID:    1,
		AssistantID: "default",
		Input:       map[string]any{},
		Command: map[string]any{
			"resume": map[string]any{
				"checkpoint_id": strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10),
			},
		},
		IntervalMs: 1,
		TimeoutMs:  1,
	})

	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusQueued, resp.Run.Status)
	require.Contains(t, writer.String(), `"status":"queued"`)
	require.Contains(t, resp.Run.Metadata, `"protected_from_worker_claim":true`)
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

func TestLangGraphInterruptedStatusMapsToResumableInternalState(t *testing.T) {
	require.Equal(
		t,
		appagentthread.RunStatusInterrupted,
		langGraphInternalRunStatus("interrupted"),
	)
	require.Equal(t, "interrupted", langGraphRunStatus(appagentthread.RunStatusInterrupted))
	require.Equal(t, "interrupted", langGraphRunStatus(appagentthread.RunStatusCanceled))
}

type cursorRecordingLangGraphRunStreamWriter struct {
	eventIDs        []int64
	failAfterEvents int
}

func (w *cursorRecordingLangGraphRunStreamWriter) WriteEvent(id, _ string, _ []byte) error {
	if id == "" {
		return nil
	}
	if w.failAfterEvents > 0 && len(w.eventIDs) >= w.failAfterEvents {
		return errors.New("stream disconnected")
	}
	eventID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return err
	}
	w.eventIDs = append(w.eventIDs, eventID)
	return nil
}
