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
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	langgraphapi "github.com/coze-dev/coze-studio/backend/api/model/agent/langgraph"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

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

func TestLangGraphStatelessRunStreamWritesMetadataEventsAndEnd(t *testing.T) {
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
	streamLangGraphStatelessRunEvents(context.Background(), writer, langgraphapi.StatelessStreamRunRequest{
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
	require.Contains(t, body, `"event_type":"run.completed"`)
	require.Contains(t, body, `"step_name":"generate_answer"`)
	require.Contains(t, body, `"status":"completed"`)
	require.Contains(t, body, "event: end")
	require.Contains(t, body, `"status":"success"`)
	require.Less(t, strings.Index(body, `"event_type":"run.completed"`), strings.Index(body, "event: end"))
}

func TestLangGraphStatelessRunStreamCancelsPersistedCancelModeOnWriteFailure(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"disconnect"}]}`,
	})
	require.NoError(t, err)

	streamLangGraphStatelessRunEvents(context.Background(), failingTaskThreadRunEventStreamWriter{}, langgraphapi.StatelessStreamRunRequest{
		RunID:      runResp.Run.RunID,
		IntervalMs: 10,
		TimeoutMs:  100,
	}, runResp.Run)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusCanceled, persisted.Run.Status)
	require.Equal(t, "client_disconnected", persisted.Run.ErrorCode)
}

func TestLangGraphStatelessRunStreamSkipsEventsAtOrBeforeCursor(t *testing.T) {
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
	streamLangGraphStatelessRunEvents(context.Background(), writer, langgraphapi.StatelessStreamRunRequest{
		RunID:        runResp.Run.RunID,
		AfterEventID: first.Event.EventID,
		IntervalMs:   10,
		TimeoutMs:    1,
	}, runResp.Run)
	body := writer.String()

	require.NotContains(t, body, `"event_type":"step.started"`)
	require.Contains(t, body, `"event_type":"step.completed"`)
}

func TestLangGraphStatelessRunStreamReconnectReplaysEventsExactlyOnceInIDOrder(t *testing.T) {
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:     1,
		Input:        `{"messages":[{"role":"user","content":"断线续传"}]}`,
		OnDisconnect: "continue",
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
	streamLangGraphStatelessRunEvents(context.Background(), firstWriter, langgraphapi.StatelessStreamRunRequest{
		RunID:      runResp.Run.RunID,
		IntervalMs: 1,
		TimeoutMs:  1,
	}, runResp.Run)
	require.Len(t, firstWriter.eventIDs, 1)

	secondWriter := &cursorRecordingLangGraphRunStreamWriter{}
	streamLangGraphStatelessRunEvents(context.Background(), secondWriter, langgraphapi.StatelessStreamRunRequest{
		RunID:        runResp.Run.RunID,
		AfterEventID: firstWriter.eventIDs[0],
		IntervalMs:   1,
		TimeoutMs:    1,
	}, runResp.Run)

	replayedIDs := append(append([]int64{}, firstWriter.eventIDs...), secondWriter.eventIDs...)
	require.Equal(t, expectedIDs, replayedIDs)
	require.Len(t, secondWriter.eventIDs, 2)
}

func TestLangGraphStatelessRunStreamRespectsUpdatesMode(t *testing.T) {
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
	streamLangGraphStatelessRunEvents(context.Background(), writer, langgraphapi.StatelessStreamRunRequest{
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

func TestLangGraphStatelessRunStreamRespectsMessagesMode(t *testing.T) {
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
	streamLangGraphStatelessRunEvents(context.Background(), writer, langgraphapi.StatelessStreamRunRequest{
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

func TestLangGraphStatelessRunCreateStreamUsesRequestedStreamMode(t *testing.T) {
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
	streamCreatedLangGraphStatelessRun(context.Background(), writer, langgraphapi.StatelessCreateStreamRunRequest{
		StreamMode: []string{"updates"},
		IntervalMs: 1,
		TimeoutMs:  1,
	}, runResp)
	body := writer.String()

	require.Contains(t, body, "event: updates")
	require.NotContains(t, body, "event: events")
	require.Contains(t, body, `"agent":{"status":"planning"}`)
}

func TestLangGraphStatelessRunCreateStreamRejectsMissingInput(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/runs/stream", CreateLangGraphStatelessRunStream)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"assistant_id": "default",
	})
	require.NoError(t, err)

	streamResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/runs/stream?timeout_ms=1",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)

	require.Equal(t, http.StatusBadRequest, streamResp.Code)
}

func TestLangGraphStatelessRunJoinReturnsCurrentRunOnTimeout(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/runs/:run_id/join", JoinLangGraphStatelessRun)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"等待超时"}]}`,
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
	require.Contains(t, body, `"status":"pending"`)
}

func TestLangGraphStatelessRunJoinStreamWritesEventsAndEnd(t *testing.T) {
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
	joinLangGraphStatelessRunStreamEvents(context.Background(), writer, langgraphapi.StatelessJoinRunRequest{
		RunID:      runResp.Run.RunID,
		IntervalMs: 1,
		TimeoutMs:  1,
	}, persisted.Run)
	body := writer.String()

	require.Contains(t, body, "event: metadata")
	require.Contains(t, body, "event: events")
	require.Contains(t, body, `"event_type":"step.completed"`)
	require.Contains(t, body, `"event_type":"run.completed"`)
	require.Contains(t, body, "event: end")
	require.Contains(t, body, `"status":"success"`)
	require.Less(t, strings.Index(body, `"event_type":"run.completed"`), strings.Index(body, "event: end"))
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
	require.Contains(t, body, `"event_type":"run.completed"`)
	require.Contains(t, body, "event: end")
	require.Contains(t, body, `"status":"success"`)
	require.Less(t, strings.Index(body, `"event_type":"run.completed"`), strings.Index(body, "event: end"))
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

	req := langgraphapi.StatelessCreateStreamRunRequest{
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
			"space_id": "1",
			"title":    "一次性流式任务",
			"source":   "api",
		},
		StreamMode: []string{"updates"},
		IntervalMs: 1,
		TimeoutMs:  1,
	}
	runResp, err := createLangGraphStatelessRun(context.Background(), statelessCreateRunRequest(req))
	require.NoError(t, err)
	require.NotNil(t, runResp)
	require.NotNil(t, runResp.Run)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamCreatedLangGraphStatelessRun(context.Background(), writer, req, runResp)
	body := writer.String()

	require.Contains(t, body, "event: metadata")
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(runResp.Run.RunID, 10)+`"`)
	require.Contains(t, body, `"thread_id":"`+strconv.FormatInt(runResp.Run.ThreadID, 10)+`"`)
	require.Contains(t, body, `"status":"pending"`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runResp.Run.RunID})
	require.NoError(t, err)
	require.Equal(t, runResp.Run.ThreadID, persisted.Run.ThreadID)
}

func TestLangGraphStatelessRunStatusMapsCanceledToInterrupted(t *testing.T) {
	require.Equal(t, "interrupted", langGraphRunStatus(appagentthread.RunStatusInterrupted))
	require.Equal(t, "interrupted", langGraphRunStatus(appagentthread.RunStatusCanceled))
}

func TestLangGraphStatelessPublicProjectionDoesNotExposeInternalRecords(t *testing.T) {
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

	checkpointValues := langGraphPublicCheckpointValues(appagentthread.ProjectPublicCheckpoint(
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
	))
	require.NotContains(t, mustMarshalJSON(t, checkpointValues), sensitive)

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
	require.Equal(t, true, journal["reasoning_present"])
	require.NotContains(t, mustMarshalJSON(t, journal), sensitive)

	writer := &recordingTaskThreadRunEventStreamWriter{}
	writeLangGraphRunStreamError(context.Background(), writer, errors.New(sensitive))
	require.NotContains(t, writer.String(), sensitive)
	require.Contains(t, writer.String(), "runtime_failed")
}

func TestLangGraphStatelessRunWaitResponseDoesNotExposeProviderError(t *testing.T) {
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

func (w *cursorRecordingLangGraphRunStreamWriter) WriteKeepAlive() error {
	return nil
}
