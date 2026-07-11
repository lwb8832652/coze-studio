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

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	langgraphapi "github.com/coze-dev/coze-studio/backend/api/model/agent/langgraph"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

func TestLangGraphRunCreateListAndGetHandlers(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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
	require.NotContains(t, createBody, `"model_name":"gpt-4.1"`)
	require.NotContains(t, createBody, `"enable_skills":["research"]`)
	require.Contains(t, createBody, `"stream_mode":["messages","updates"]`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: 2})
	require.NoError(t, err)
	require.Contains(t, persisted.Run.Config, `"model_name":"gpt-4.1"`)
	require.Contains(t, persisted.Run.Context, `"enable_skills":["research"]`)

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

func TestLangGraphRunGetForbiddenForDifferentViewer(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/threads/:thread_id/runs/:run_id",
		workbenchSessionMiddlewareForTest(3),
		GetLangGraphRun,
	)
	installAgentThreadTestService(t)
	runResp, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{ThreadID: 1, Input: `{}`},
	)
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10),
		nil,
	)

	require.Equal(t, http.StatusForbidden, resp.Code)
	require.Contains(t, string(resp.Result().Body()), "thread access denied")
}

func TestLangGraphRunCreateAccessDeniedDoesNotMutate(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/threads/:thread_id/runs",
		workbenchSessionMiddlewareForTest(3),
		CreateLangGraphRun,
	)
	installAgentThreadTestService(t)
	payload, err := json.Marshal(map[string]any{
		"input": map[string]any{"messages": []map[string]string{{
			"role": "user", "content": "must not run",
		}}},
	})
	require.NoError(t, err)
	before, err := appagentthread.SVC.ListRuns(
		context.Background(),
		&appagentthread.ListRunsRequest{ThreadID: 1},
	)
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	after, err := appagentthread.SVC.ListRuns(
		context.Background(),
		&appagentthread.ListRunsRequest{ThreadID: 1},
	)
	require.NoError(t, err)

	require.Equal(t, http.StatusForbidden, resp.Code)
	require.Equal(t, before.Total, after.Total)
}

func TestLangGraphRunThreadMismatchForbiddenWithoutCancel(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/threads/:thread_id/runs/:run_id/cancel",
		workbenchSessionMiddlewareForTest(2),
		CancelLangGraphRun,
	)
	installAgentThreadTestService(t)
	threadResp, err := appagentthread.SVC.CreateThread(
		context.Background(),
		&appagentthread.CreateThreadRequest{SpaceID: 1, UserID: 2, Title: "other"},
	)
	require.NoError(t, err)
	runResp, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{ThreadID: threadResp.Thread.ThreadID, Input: `{}`},
	)
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/cancel",
		nil,
	)
	persisted, err := appagentthread.SVC.GetRun(
		context.Background(),
		&appagentthread.GetRunRequest{RunID: runResp.Run.RunID},
	)
	require.NoError(t, err)

	require.Equal(t, http.StatusForbidden, resp.Code)
	require.Equal(t, appagentthread.RunStatusPending, persisted.Run.Status)
}

func TestLangGraphRunCreateAcceptsFlexibleInputAndStreamMode(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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
	require.NotContains(t, body, `"content":"数组输入"`)
	require.Contains(t, body, `"stream_mode":["updates"]`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: 2})
	require.NoError(t, err)
	require.JSONEq(t, `[{"content":"数组输入","role":"user"}]`, persisted.Run.Input)
	require.Equal(t, `["updates"]`, persisted.Run.StreamMode)
}

func TestLangGraphRunCreateAcceptsEmptyObjectInput(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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

func TestLangGraphRunWaitCreatesRunAndReturnsFallbackState(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/threads/:thread_id/runs/wait", WaitLangGraphRun)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input": map[string]any{
			"messages": []map[string]any{{"role": "user", "content": "同步等待"}},
		},
	})
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs/wait?timeout_ms=1&interval_ms=1",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(resp.Result().Body())

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, body, `"status":"pending"`)
	require.Contains(t, body, `"error":""`)
}

func TestLangGraphStatelessRunWaitCreatesRunAndReturnsFallbackState(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/runs/wait", WaitLangGraphStatelessRun)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
		"input": map[string]any{
			"messages": []map[string]any{{"role": "user", "content": "无状态同步等待"}},
		},
		"metadata": map[string]any{
			"space_id": "1",
			"user_id":  "2",
		},
	})
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/runs/wait?timeout_ms=1&interval_ms=1",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(resp.Result().Body())

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, body, `"status":"pending"`)
	require.Contains(t, body, `"error":""`)
}

func TestLangGraphRunCreateCreatesProtectedResumeRunFromReadyCheckpoint(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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
	_, err = appagentthread.SVC.FailRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:        runResp.Run.RunID,
		From:         appagentthread.RunStatusRunning,
		WorkerID:     "source-worker",
		ErrorCode:    "step_error",
		ErrorMessage: "source run failed",
	}))
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
	require.NotContains(t, body, "langgraph_sdk")
	require.Empty(t, apiRun.Command)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: langGraphInt64Value(apiRun.RunID)})
	require.NoError(t, err)
	command := langGraphJSONMap(persisted.Run.Command)
	resumeCommand, ok := command["resume"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10), resumeCommand["checkpoint_id"])
	require.Equal(t, "harness.terminal", resumeCommand["checkpoint_ns"])
	require.Equal(t, "pending_sends", resumeCommand["resume_from"])
	require.Equal(t, "pending_sends_available", resumeCommand["reason"])
	require.Equal(t, "worker_replay_not_enabled", resumeCommand["guard"])

	resumeMetadata, ok := langGraphJSONMap(persisted.Run.Metadata)["checkpoint_resume"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, resumeMetadata["protected_from_worker_claim"])
	require.Equal(t, strconv.FormatInt(checkpointResp.Checkpoint.CheckpointID, 10), resumeMetadata["checkpoint_id"])

	require.Equal(t, appagentthread.RunStatusQueued, persisted.Run.Status)

	claimed, err := appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    10,
	})
	require.NoError(t, err)
	require.Empty(t, claimed.Runs)
	require.NotContains(t, body, `"checkpoint_resume"`)
}

func TestLangGraphRunCreatePreservesResumeTargets(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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
	require.Empty(t, apiRun.Command)
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: langGraphInt64Value(apiRun.RunID)})
	require.NoError(t, err)
	resumeCommand, ok := langGraphJSONMap(persisted.Run.Command)["resume"].(map[string]any)
	require.True(t, ok)
	targets, ok := resumeCommand["targets"].(map[string]any)
	require.True(t, ok)
	target, ok := targets["interrupt-1"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "最近 7 天", target["answer"])
	require.Equal(t, "interrupt", resumeCommand["resume_from"])
}

func TestLangGraphRunCreateRejectsUnknownResumeTarget(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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
	h := authenticatedAgentThreadTestServer()
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
	h := authenticatedAgentThreadTestServer()
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
	_, err = appagentthread.SVC.CompleteRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	}))
	require.NoError(t, err)

	listResp := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs?status=success", nil)
	body := string(listResp.Result().Body())

	require.Equal(t, http.StatusOK, listResp.Code)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"status":"success"`)
}

func TestLangGraphRunGetRejectsCrossThreadRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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

	require.Equal(t, http.StatusForbidden, getResp.Code)
}

func TestLangGraphRunMessagesHandlerReturnsDeerFlowPage(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/threads/:thread_id/runs/:run_id/messages", ListLangGraphRunMessages)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"请搜索青岛最佳旅游时间"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "message.completed",
		Payload:   `{"role":"assistant","content":"我先查询天气和旅游季节。","reasoning_content":"Need travel season evidence","usage":{"input_tokens":11,"output_tokens":7}}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "tool.completed",
		Payload:   `{"tool_name":"web_search","tool_call_id":"call-1","content":"青岛5-10月适合旅游"}`,
	})
	require.NoError(t, err)

	firstPage := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/messages?limit=2",
		nil,
	)
	firstBody := string(firstPage.Result().Body())

	require.Equal(t, http.StatusOK, firstPage.Code)
	require.Contains(t, firstBody, `"has_more":true`)
	require.Contains(t, firstBody, `"seq":1`)
	require.Contains(t, firstBody, `"type":"human"`)
	require.Contains(t, firstBody, `"content":"请搜索青岛最佳旅游时间"`)
	require.Contains(t, firstBody, `"seq":2`)
	require.Contains(t, firstBody, `"type":"ai"`)
	require.Contains(t, firstBody, `"content":"我先查询天气和旅游季节。"`)
	require.NotContains(t, firstBody, "Need travel season evidence")
	require.NotContains(t, firstBody, `"type":"tool"`)

	nextPage := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/messages?after_seq=2",
		nil,
	)
	nextBody := string(nextPage.Result().Body())

	require.Equal(t, http.StatusOK, nextPage.Code)
	require.Contains(t, nextBody, `"has_more":false`)
	require.Contains(t, nextBody, `"seq":3`)
	require.Contains(t, nextBody, `"type":"tool"`)
	require.Contains(t, nextBody, `"name":"web_search"`)
	require.Contains(t, nextBody, `"tool_call_id":"call-1"`)
	require.NotContains(t, nextBody, "Need travel season evidence")
}

func TestLangGraphThreadMessagesHandlerReturnsDeerFlowList(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/threads/:thread_id/messages", ListLangGraphThreadMessages)
	installAgentThreadTestService(t)

	firstRun, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"第一轮问题"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     firstRun.Run.RunID,
		EventType: "message.completed",
		Payload:   `{"role":"assistant","content":"第一轮回复","tool_calls":[{"id":"call-1","name":"web_search","arguments":{"query":"secret raw arg"}}]}`,
	})
	require.NoError(t, err)

	secondRun, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"第二轮问题"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     secondRun.Run.RunID,
		EventType: "message.completed",
		Payload:   `{"role":"assistant","content":"第二轮回复","reasoning_content":"visible bounded reasoning"}`,
	})
	require.NoError(t, err)

	firstPage := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/1/messages?limit=3",
		nil,
	)
	firstBody := string(firstPage.Result().Body())

	require.Equal(t, http.StatusOK, firstPage.Code)
	require.Contains(t, firstBody, `"seq":1`)
	require.Contains(t, firstBody, `"content":"第一轮问题"`)
	require.Contains(t, firstBody, `"seq":2`)
	require.Contains(t, firstBody, `"content":"第一轮回复"`)
	require.Contains(t, firstBody, `"tool_calls":[{"id":"call-1","name":"web_search","type":"function"}]`)
	require.Contains(t, firstBody, `"feedback":null`)
	require.Contains(t, firstBody, `"seq":3`)
	require.Contains(t, firstBody, `"content":"第二轮问题"`)
	require.NotContains(t, firstBody, "secret raw arg")
	require.NotContains(t, firstBody, "第二轮回复")

	nextPage := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/1/messages?after_seq=3",
		nil,
	)
	nextBody := string(nextPage.Result().Body())

	require.Equal(t, http.StatusOK, nextPage.Code)
	require.Contains(t, nextBody, `"seq":4`)
	require.Contains(t, nextBody, `"content":"第二轮回复"`)
	require.NotContains(t, nextBody, "visible bounded reasoning")
	require.NotContains(t, nextBody, "第一轮问题")
}

func TestLangGraphRunEventsHandlerReturnsSafeDeerFlowList(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/threads/:thread_id/runs/:run_id/events", ListLangGraphRunEvents)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"执行工具"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "run.started",
		Payload:   `{"status":"running","worker_id":"worker-a"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "tool.completed",
		Payload: `{
			"role":"tool",
			"tool_name":"web_search",
			"tool_call_id":"call_123",
			"content":"tool result sk-secret https://private.example/signed",
			"usage":{"prompt_tokens":10,"completion_tokens":5}
		}`,
	})
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/events?event_types=tool.completed&limit=10",
		nil,
	)
	body := string(resp.Result().Body())

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, body, `"event_type":"tool.completed"`)
	require.Contains(t, body, `\"redacted\":true`)
	require.Contains(t, body, `\"role\":\"tool\"`)
	require.Contains(t, body, `\"tool_name\":\"web_search\"`)
	require.Contains(t, body, `\"tool_call_id\":\"call_123\"`)
	require.Contains(t, body, `\"result_present\":true`)
	require.NotContains(t, body, `"event_type":"run.started"`)
	require.NotContains(t, body, "tool result")
	require.NotContains(t, body, "sk-secret")
	require.NotContains(t, body, "private.example")
	require.NotContains(t, body, "prompt_tokens")
}

func TestLangGraphStatelessRunMessagesHandlerReturnsDeerFlowPage(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/runs/:run_id/messages", ListLangGraphStatelessRunMessages)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"无状态消息"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "message.completed",
		Payload:   `{"role":"assistant","content":"无状态回复"}`,
	})
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/messages?limit=1",
		nil,
	)
	body := string(resp.Result().Body())

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, body, `"has_more":true`)
	require.Contains(t, body, `"content":"无状态消息"`)
	require.NotContains(t, body, "无状态回复")
}

func TestLangGraphStatelessRunFeedbackHandlerReturnsEmptyListForValidRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/runs/:run_id/feedback", ListLangGraphStatelessRunFeedback)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"反馈"}]}`,
	})
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/feedback",
		nil,
	)

	require.Equal(t, http.StatusOK, resp.Code)
	require.JSONEq(t, `[]`, string(resp.Result().Body()))
}

func TestLangGraphRunCancelHandlerTransitionsPendingRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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
	h := authenticatedAgentThreadTestServer()
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

	require.Equal(t, http.StatusForbidden, cancelResp.Code)

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
	_, err = appagentthread.SVC.CompleteRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	}))
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
	h := authenticatedAgentThreadTestServer()
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

	require.Equal(t, http.StatusForbidden, streamResp.Code)
}

func TestLangGraphRunStreamPostInterruptCancelsRunWhenWaitRequested(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/threads/:thread_id/runs/:run_id/stream", StreamLangGraphRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"停止"}]}`,
	})
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/threads/1/runs/"+strconv.FormatInt(runResp.Run.RunID, 10)+"/stream?action=interrupt&wait=1",
		nil,
	)

	require.Equal(t, http.StatusNoContent, resp.Code)
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusCanceled, persisted.Run.Status)
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
	_, err = appagentthread.SVC.FailRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:        runResp.Run.RunID,
		From:         appagentthread.RunStatusRunning,
		WorkerID:     "source-worker",
		ErrorCode:    "step_error",
		ErrorMessage: "source run failed",
	}))
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
	h := authenticatedAgentThreadTestServer()
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
	h := authenticatedAgentThreadTestServer()
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
	_, err = appagentthread.SVC.CompleteRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	}))
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
	h := authenticatedAgentThreadTestServer()
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
	h := authenticatedAgentThreadTestServer()
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

	require.Equal(t, http.StatusForbidden, postResp.Code)
	require.Equal(t, http.StatusForbidden, getResp.Code)
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
	_, err = appagentthread.SVC.CompleteRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	}))
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
	h := authenticatedAgentThreadTestServer()
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
	h := authenticatedAgentThreadTestServer()
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
	h := authenticatedAgentThreadTestServer()
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
	_, err = appagentthread.SVC.CompleteRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	}))
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
	_, err = appagentthread.SVC.CompleteRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	}))
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
	h := authenticatedAgentThreadTestServer()
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
	require.NotContains(t, body, `"model_name":"gpt-4.1"`)
	require.Contains(t, body, `"stream_mode":["updates"]`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: 3})
	require.NoError(t, err)
	require.Contains(t, persisted.Run.Config, `"model_name":"gpt-4.1"`)

	threadResp, err := appagentthread.SVC.GetThread(context.Background(), &appagentthread.GetThreadRequest{ThreadID: 2})
	require.NoError(t, err)
	require.Equal(t, int64(99), threadResp.Thread.SpaceID)
	require.Equal(t, int64(2), threadResp.Thread.CreatorID)
	require.Equal(t, "一次性分析任务", threadResp.Thread.Title)
	require.Equal(t, appagentthread.ThreadSourceAPI, threadResp.Thread.Source)
	require.Contains(t, threadResp.Thread.Metadata, `"title":"一次性分析任务"`)
}

func TestLangGraphStatelessRunCreateRejectsUnauthorizedWorkspaceBeforeMutation(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/runs", CreateLangGraphStatelessRun)
	installAgentThreadTestService(t)
	appagentthread.SVC.WorkspaceAuthorizer = &recordingWorkbenchWorkspaceAuthorizer{
		err: appagentthread.ErrThreadAccessDenied,
	}
	before, err := appagentthread.SVC.ListThreads(context.Background(), &appagentthread.ListThreadsRequest{
		SpaceID: 1,
		Page:    1,
	})
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{
		"input": map[string]any{
			"messages": []map[string]string{{"role": "user", "content": "must not persist"}},
		},
		"metadata": map[string]any{
			"space_id": "999",
			"user_id":  "888",
		},
	})
	require.NoError(t, err)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/runs",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	after, err := appagentthread.SVC.ListThreads(context.Background(), &appagentthread.ListThreadsRequest{
		SpaceID: 1,
		Page:    1,
	})
	require.NoError(t, err)

	require.Equal(t, http.StatusForbidden, resp.Code)
	require.Equal(t, before.Total, after.Total)
}

func TestLangGraphStatelessRunCreateRejectsMissingInput(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
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

func TestLangGraphPublicProjectionDoesNotExposeInternalRecords(t *testing.T) {
	const sensitive = "langgraph-sensitive-sentinel"

	run := langGraphRunToAPI(&appagentthread.RunSummary{
		RunID:        1,
		ThreadID:     2,
		AssistantID:  "default",
		Status:       appagentthread.RunStatusFailed,
		Input:        `{"messages":["` + sensitive + `"]}`,
		Command:      `{"resume":"` + sensitive + `"}`,
		Config:       `{"api_key":"` + sensitive + `"}`,
		Context:      `{"reasoning":"` + sensitive + `"}`,
		Metadata:     `{"source":"api","credential":"` + sensitive + `"}`,
		ErrorCode:    "model_provider_error",
		ErrorMessage: sensitive,
	})
	require.Equal(t, map[string]any{"source": "api"}, run.Metadata)
	require.Equal(t, map[string]any{}, run.Command)
	require.Equal(t, map[string]any{}, run.Config)
	require.Equal(t, map[string]any{}, run.Context)
	require.Equal(t, map[string]any{}, run.Input)
	require.Equal(t, "Model request failed", run.Error)
	require.NotContains(t, mustMarshalJSON(t, run), sensitive)

	checkpointState := langGraphThreadStateFromCheckpoint(
		&appagentthread.ThreadSummary{ThreadID: 2, Title: "safe title"},
		&appagentthread.CheckpointSummary{
			CheckpointID:    4,
			ThreadID:        2,
			RunID:           1,
			RuntimeType:     string(appagentthread.RuntimeModeEinoADK),
			RuntimeKey:      sensitive,
			ChannelValues:   sensitive,
			ChannelVersions: `{"provider":"` + sensitive + `"}`,
			PendingSends:    sensitive,
			Metadata:        `{"provider":"` + sensitive + `"}`,
		},
	)
	require.NotContains(t, mustMarshalJSON(t, checkpointState), sensitive)

	event := &appagentthread.RunEventSummary{
		EventID:   5,
		ThreadID:  2,
		RunID:     1,
		EventType: "provider.experimental",
		Payload:   `{"raw":"` + sensitive + `"}`,
	}
	require.NotContains(t, mustMarshalJSON(t, langGraphRunGenericEventPayload(event)), sensitive)

	journal := langGraphRunJournalMessageToAPI(&appagentthread.RunJournalMessage{
		ID:       "message-1",
		ThreadID: 2,
		RunID:    1,
		Type:     appagentthread.RunJournalMessageTypeAI,
		Role:     appagentthread.MessageRoleAssistant,
		Content:  "visible response",
		ToolCalls: []appagentthread.RunJournalToolCall{{
			ID:   "call-1",
			Name: "web_search",
			Type: "function",
			Args: map[string]any{"credential": sensitive},
		}},
		AdditionalKwargs: map[string]any{"reasoning_content": sensitive},
		Usage:            map[string]any{"input_tokens": int64(3), "provider_body": sensitive},
	}, 1)
	require.Equal(t, "visible response", journal["content"])
	require.NotContains(t, mustMarshalJSON(t, journal), sensitive)

	stateMessages := langGraphThreadStateMessages([]*appagentthread.MessageSummary{{
		MessageID: 1,
		ThreadID:  2,
		RunID:     1,
		Role:      appagentthread.MessageRoleAssistant,
		Content:   "visible state message",
		Metadata:  `{"source":"runtime","provider_body":"` + sensitive + `"}`,
	}})
	require.NotContains(t, mustMarshalJSON(t, stateMessages), sensitive)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	writeLangGraphRunStreamError(context.Background(), writer, errors.New(sensitive))
	require.NotContains(t, writer.String(), sensitive)
	require.Contains(t, writer.String(), "runtime_failed")
}

func TestLangGraphRunWaitResponseDoesNotExposeProviderError(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	const sensitive = "provider-sensitive-runtime-sentinel"
	_, err = appagentthread.SVC.FailRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:        runResp.Run.RunID,
		From:         appagentthread.RunStatusRunning,
		WorkerID:     "worker-a",
		ErrorCode:    "model_provider_error",
		ErrorMessage: sensitive,
	}))
	require.NoError(t, err)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)
	result, err := langGraphRunWaitResponse(context.Background(), persisted.Run)
	require.NoError(t, err)
	require.Equal(t, "Model request failed", result["error"])
	require.NotContains(t, mustMarshalJSON(t, result), sensitive)
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
