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
