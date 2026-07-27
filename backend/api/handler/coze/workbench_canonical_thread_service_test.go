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
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

func TestCanonicalThreadResourceHandlersFailClosedWithoutApplicationService(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	previous := appagentthread.SVC
	appagentthread.SVC = nil
	t.Cleanup(func() {
		appagentthread.SVC = previous
	})

	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id", GetCanonicalThread)
	h.PATCH("/api/workbench/threads/:thread_id", PatchCanonicalThread)
	h.DELETE("/api/workbench/threads/:thread_id", DeleteCanonicalThread)
	h.GET("/api/workbench/threads/:thread_id/state", GetCanonicalThreadState)
	h.POST("/api/workbench/threads/:thread_id/state", UpdateCanonicalThreadState)
	h.GET("/api/workbench/threads/:thread_id/history", GetCanonicalThreadHistory)
	h.POST("/api/workbench/threads/:thread_id/history", PostCanonicalThreadHistory)
	h.GET("/api/workbench/threads/:thread_id/messages", ListCanonicalThreadMessages)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "get", method: http.MethodGet, path: "/api/workbench/threads/1"},
		{name: "patch", method: http.MethodPatch, path: "/api/workbench/threads/1", body: `{"metadata":{"title":"updated"}}`},
		{name: "delete", method: http.MethodDelete, path: "/api/workbench/threads/1"},
		{name: "get state", method: http.MethodGet, path: "/api/workbench/threads/1/state"},
		{name: "update state", method: http.MethodPost, path: "/api/workbench/threads/1/state", body: `{"values":{"custom":{}}}`},
		{name: "get history", method: http.MethodGet, path: "/api/workbench/threads/1/history"},
		{name: "post history", method: http.MethodPost, path: "/api/workbench/threads/1/history", body: `{}`},
		{name: "messages", method: http.MethodGet, path: "/api/workbench/threads/1/messages"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var requestBody *ut.Body
			if test.body != "" {
				requestBody = &ut.Body{Body: strings.NewReader(test.body), Len: len(test.body)}
			}
			response := ut.PerformRequest(
				h.Engine,
				test.method,
				test.path,
				requestBody,
				ut.Header{Key: "Content-Type", Value: "application/json"},
			)
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			var public canonicalError
			require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
			require.Equal(t, "dependency_unavailable", public.Code)
		})
	}
}

func TestCreateCanonicalThreadCreatesEmptyThreadInAuthorizedSpace(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)
	authorizer := &canonicalRecordingWorkspaceAuthorizer{}
	appagentthread.SVC.WorkspaceAuthorizer = authorizer

	body := `{"metadata":{"title":"新建任务"}}`
	response := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/threads",
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: canonicalSpaceIDHeader, Value: "1001"},
	)

	require.Equal(t, http.StatusOK, response.Code)
	require.GreaterOrEqual(t, authorizer.calls, 1)
	require.Equal(t, appagentthread.WorkspaceAccessRequest{
		ViewerID: 2,
		SpaceID:  1001,
	}, authorizer.req)

	var created canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &created))
	require.NotEmpty(t, created.ThreadID)
	require.Equal(t, "新建任务", created.Metadata["title"])
	require.Nil(t, created.Coze.InitialSubmission)

	threadID := mustCanonicalTestID(t, created.ThreadID)
	messages, err := appagentthread.SVC.ListMessages(context.Background(), &appagentthread.ListMessagesRequest{
		ThreadID: threadID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Empty(t, messages.Messages)
	runs, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: threadID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Empty(t, runs.Runs)
}

func TestCreateCanonicalThreadCreatesInitialSubmissionAtomically(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)
	authorizer := &canonicalRecordingWorkspaceAuthorizer{}
	appagentthread.SVC.WorkspaceAuthorizer = authorizer

	body := `{
		"metadata":{"title":"新建任务"},
		"coze":{"initial_run":{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]},
			"config":{"runtime":"eino_adk","mode":"pro"},
			"metadata":{"source":"workbench_home"}
		}}
	}`
	response := performCanonicalThreadJSONRequest(t, h, http.MethodPost, "/api/workbench/threads", body)

	require.Equal(t, http.StatusOK, response.Code)
	require.GreaterOrEqual(t, authorizer.calls, 1)
	require.Equal(t, appagentthread.WorkspaceAccessRequest{ViewerID: 2, SpaceID: 1001}, authorizer.req)

	var created canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &created))
	require.NotNil(t, created.Coze.InitialSubmission)
	threadID := mustCanonicalTestID(t, created.ThreadID)
	messages, runs := canonicalThreadMessagesAndRuns(t, threadID)
	require.Len(t, messages, 1)
	require.Equal(t, appagentthread.MessageRoleUser, messages[0].Role)
	require.Equal(t, "请生成产品发布方案", messages[0].Content)
	require.Len(t, runs, 1)
	require.Equal(t, "agent", runs[0].AssistantID)
	require.JSONEq(t, `{"source":"workbench_home"}`, runs[0].Metadata)
	require.Contains(t, runs[0].Config, `"runtime":"eino_adk"`)

	initialJSON, err := json.Marshal(created.Coze.InitialSubmission)
	require.NoError(t, err)
	require.Contains(t, string(initialJSON), `"message_id":"`)
	require.Contains(t, string(initialJSON), `"run_id":"`)
}

func TestCreateCanonicalThreadValidatesInitialSubmissionBeforeMutation(t *testing.T) {
	tests := map[string]string{
		"internal assistant selector": `{
			"assistant_id":"default",
			"input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]}
		}`,
		"protected config identity": `{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]},
			"config":{"user_id":"forged"}
		}`,
		"sensitive context credential": `{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]},
			"context":{"api_key":"TOP_SECRET"}
		}`,
		"protected run metadata": `{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]},
			"metadata":{"_idempotency":"forged"}
		}`,
	}

	for name, initialRun := range tests {
		name, initialRun := name, initialRun
		t.Run(name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			h := authenticatedAgentThreadTestServer()
			h.POST("/api/workbench/threads", CreateCanonicalThread)
			installAgentThreadTestService(t)

			body := `{"metadata":{},"coze":{"initial_run":` + initialRun + `}}`
			response := performCanonicalThreadJSONRequest(
				t, h, http.MethodPost, "/api/workbench/threads", body,
			)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code)
			require.Zero(t, canonicalThreadCount(t, 1001, 2))
		})
	}
}

func TestCreateCanonicalThreadRejectsOversizedInitialSubmissionBeforeMutation(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)

	values := make([]string, 65)
	for index := range values {
		values[index] = strings.Repeat("x", 16<<10)
	}
	body, err := json.Marshal(map[string]any{
		"metadata": map[string]any{},
		"coze": map[string]any{
			"initial_run": map[string]any{
				"assistant_id": "agent",
				"input": map[string]any{"messages": []map[string]any{{
					"role": "user", "content": "oversized request",
				}}},
				"config": map[string]any{"payload": values},
			},
		},
	})
	require.NoError(t, err)
	require.Greater(t, len(body), canonicalMaxRequestBytes)

	response := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads", string(body),
	)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	var public canonicalError
	require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
	require.Equal(t, "request_too_large", public.Code)
	require.Zero(t, canonicalThreadCount(t, 1001, 2))
}

func TestCreateCanonicalThreadInitialRunRejectsChangedIdempotentPayload(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)

	const idempotencyKey = "canonical-initial-thread-1"
	firstBody := `{
		"metadata":{"title":"首提任务"},
		"coze":{"initial_run":{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"first payload"}]},
			"config":{"runtime":"eino_adk","mode":"pro"},
			"metadata":{"source":"workbench_home"}
		}}
	}`
	requestHeader := ut.Header{Key: "Idempotency-Key", Value: idempotencyKey}
	first := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads", firstBody, requestHeader,
	)
	require.Equal(t, http.StatusOK, first.Code)
	var firstThread canonicalThread
	require.NoError(t, json.Unmarshal(first.Result().Body(), &firstThread))

	replayed := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads", firstBody, requestHeader,
	)
	require.Equal(t, http.StatusOK, replayed.Code)
	var replayedThread canonicalThread
	require.NoError(t, json.Unmarshal(replayed.Result().Body(), &replayedThread))
	require.Equal(t, firstThread.ThreadID, replayedThread.ThreadID)

	changedBody := strings.Replace(firstBody, "first payload", "changed payload", 1)
	conflict := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads", changedBody, requestHeader,
	)
	require.Equal(t, http.StatusConflict, conflict.Code)
	var public canonicalError
	require.NoError(t, json.Unmarshal(conflict.Result().Body(), &public))
	require.Equal(t, "idempotency_conflict", public.Code)
	require.Equal(t, 1, canonicalThreadCount(t, 1001, 2))

	messages, runs := canonicalThreadMessagesAndRuns(t, mustCanonicalTestID(t, firstThread.ThreadID))
	require.Len(t, messages, 1)
	require.Len(t, runs, 1)
	require.Equal(t, "first payload", messages[0].Content)
	require.Contains(t, runs[0].Metadata, `"_idempotency"`)
}

func TestCreateCanonicalThreadDefersValidatedInitialSubmission(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)
	authorizer := &canonicalRecordingWorkspaceAuthorizer{}
	appagentthread.SVC.WorkspaceAuthorizer = authorizer

	body := `{
		"metadata":{},
		"coze":{"deferred_initial_run":{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"分析附件中的销售数据"}]},
			"config":{"runtime":"eino_adk","mode":"pro"},
			"metadata":{"source":"workbench_home_with_uploads"}
		}}
	}`
	response := performCanonicalThreadJSONRequest(t, h, http.MethodPost, "/api/workbench/threads", body)

	require.Equal(t, http.StatusOK, response.Code)
	require.GreaterOrEqual(t, authorizer.calls, 1)
	require.Equal(t, appagentthread.WorkspaceAccessRequest{ViewerID: 2, SpaceID: 1001}, authorizer.req)

	var created canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &created))
	require.Equal(t, "分析附件中的销售数据", created.Metadata["title"])
	require.Nil(t, created.Coze.InitialSubmission)
	messages, runs := canonicalThreadMessagesAndRuns(t, mustCanonicalTestID(t, created.ThreadID))
	require.Empty(t, messages)
	require.Empty(t, runs)
}

func TestCreateCanonicalThreadRejectsUnsupportedShapesWithoutSideEffects(t *testing.T) {
	initial := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]},
		"config":{"runtime":"eino_adk","mode":"pro"},
		"metadata":{"source":"workbench_home"}
	}`
	deferred := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"分析附件中的销售数据"}]},
		"config":{"runtime":"eino_adk","mode":"pro"},
		"metadata":{"source":"workbench_home_with_uploads"}
	}`
	tests := map[string]string{
		"both initial variants": `{"metadata":{},"coze":{"initial_run":` + initial + `,"deferred_initial_run":` + deferred + `}}`,
		"client thread id":      `{"thread_id":"client-thread","metadata":{}}`,
		"ttl":                   `{"metadata":{},"ttl":"1h"}`,
		"supersteps":            `{"metadata":{},"supersteps":[{}]}`,
		"graph id":              `{"metadata":{"graph_id":"graph-1"}}`,
		"if exists":             `{"metadata":{},"if_exists":"do_nothing"}`,
		"if exists not exact":   `{"metadata":{},"if_exists":" raise "}`,
		"too many metadata keys": fmt.Sprintf(
			`{"metadata":%s}`,
			canonicalTestMetadataJSON(t, 17),
		),
	}

	for name, body := range tests {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			h := authenticatedAgentThreadTestServer()
			h.POST("/api/workbench/threads", CreateCanonicalThread)
			installAgentThreadTestService(t)
			authorizer := &canonicalRecordingWorkspaceAuthorizer{}
			appagentthread.SVC.WorkspaceAuthorizer = authorizer

			response := performCanonicalThreadJSONRequest(t, h, http.MethodPost, "/api/workbench/threads", body)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code)
			require.Zero(t, canonicalThreadCount(t, 1001, 2))
		})
	}
}

func TestSearchCanonicalThreadsUsesAuthorizedSpaceFiltersAndExactOffset(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads/search", SearchCanonicalThreads)
	installAgentThreadTestService(t)
	authorizer := &canonicalRecordingWorkspaceAuthorizer{}
	appagentthread.SVC.WorkspaceAuthorizer = authorizer

	first := createCanonicalTestThread(t, 1001, "first", `{"team":"alpha"}`)
	second := createCanonicalTestThread(t, 1001, "second", `{"team":"alpha"}`)
	createCanonicalTestThread(t, 1001, "other", `{"team":"beta"}`)

	body := `{"metadata":{"team":"alpha"},"status":"idle","limit":1,"offset":1}`
	response := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads/search", body,
	)

	require.Equal(t, http.StatusOK, response.Code)
	require.GreaterOrEqual(t, authorizer.calls, 1)
	require.Equal(t, appagentthread.WorkspaceAccessRequest{ViewerID: 2, SpaceID: 1001}, authorizer.req)
	require.Equal(t, "2", response.Result().Header.Get("X-Pagination-Total"))
	require.Empty(t, response.Result().Header.Get("X-Pagination-Next"))

	var threads []*canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &threads))
	require.Len(t, threads, 1)
	require.Equal(t, strconv.FormatInt(first.ThreadID, 10), threads[0].ThreadID)
	require.NotEqual(t, strconv.FormatInt(second.ThreadID, 10), threads[0].ThreadID)
}

func TestSearchCanonicalThreadsSupportsIDsSortAndPaginationHeaders(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads/search", SearchCanonicalThreads)
	installAgentThreadTestService(t)

	first := createCanonicalTestThread(t, 1001, "first", `{}`)
	second := createCanonicalTestThread(t, 1001, "second", `{}`)
	body := fmt.Sprintf(
		`{"ids":["%d","%d"],"sort_by":"thread_id","sort_order":"asc","limit":1,"offset":0}`,
		first.ThreadID,
		second.ThreadID,
	)
	response := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads/search", body,
	)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "2", response.Result().Header.Get("X-Pagination-Total"))
	require.Equal(t, "1", response.Result().Header.Get("X-Pagination-Next"))
	var threads []*canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &threads))
	require.Len(t, threads, 1)
	require.Equal(t, strconv.FormatInt(first.ThreadID, 10), threads[0].ThreadID)
}

func TestSearchCanonicalThreadsSortsByProjectedSDKStatus(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads/search", SearchCanonicalThreads)
	installAgentThreadTestService(t)

	idle := createCanonicalTestThread(t, 1001, "idle", `{}`)
	busy, err := appagentthread.SVC.CreateTaskThread(context.Background(), &appagentthread.CreateTaskThreadRequest{
		SpaceID: 1001,
		UserID:  2,
		Message: "busy",
		Config:  `{"runtime":"eino_adk","mode":"pro"}`,
	})
	require.NoError(t, err)

	response := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads/search",
		`{"sort_by":"status","sort_order":"asc","limit":10}`,
	)
	require.Equal(t, http.StatusOK, response.Code)
	var threads []*canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &threads))
	require.Len(t, threads, 2)
	require.Equal(t, strconv.FormatInt(busy.Thread.ThreadID, 10), threads[0].ThreadID)
	require.Equal(t, "busy", threads[0].Status)
	require.Equal(t, strconv.FormatInt(idle.ThreadID, 10), threads[1].ThreadID)
	require.Equal(t, "idle", threads[1].Status)
}

func TestSearchCanonicalThreadsRejectsUnsupportedProjectionFields(t *testing.T) {
	tests := map[string]string{
		"values":            `{"values":true}`,
		"select":            `{"select":["thread_id"]}`,
		"extract":           `{"extract":true}`,
		"too many metadata": fmt.Sprintf(`{"metadata":%s}`, canonicalTestMetadataJSON(t, 17)),
	}
	for name, body := range tests {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			h := authenticatedAgentThreadTestServer()
			h.POST("/api/workbench/threads/search", SearchCanonicalThreads)
			installAgentThreadTestService(t)

			response := performCanonicalThreadJSONRequest(
				t, h, http.MethodPost, "/api/workbench/threads/search", body,
			)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code)
		})
	}
}

func TestGetCanonicalThreadRejectsIncludeAndProjectsAuthorizedThread(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id", GetCanonicalThread)
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "read me", `{"team":"alpha"}`)
	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10)

	rejected := ut.PerformRequest(h.Engine, http.MethodGet, path+"?include=values", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rejected.Code)

	response := ut.PerformRequest(h.Engine, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, response.Code)
	var projected canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &projected))
	require.Equal(t, strconv.FormatInt(thread.ThreadID, 10), projected.ThreadID)
	require.Equal(t, "read me", projected.Metadata["title"])
}

func TestPatchCanonicalThreadUpdatesSafeMetadataAndSupportsMinimalResponse(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.PATCH("/api/workbench/threads/:thread_id", PatchCanonicalThread)
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "before", `{"keep":"yes"}`)
	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10)

	response := performCanonicalThreadJSONRequest(
		t, h, http.MethodPatch, path, `{"metadata":{"title":"after","team":"alpha"}}`,
	)
	require.Equal(t, http.StatusOK, response.Code)
	var projected canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &projected))
	require.Equal(t, "after", projected.Metadata["title"])
	require.Equal(t, "alpha", projected.Metadata["team"])
	require.Equal(t, "yes", projected.Metadata["keep"])

	minimalBody := `{"metadata":{"priority":2}}`
	minimal := ut.PerformRequest(
		h.Engine,
		http.MethodPatch,
		path,
		&ut.Body{Body: strings.NewReader(minimalBody), Len: len(minimalBody)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "Prefer", Value: "return=minimal"},
	)
	require.Equal(t, http.StatusNoContent, minimal.Code)
	require.Empty(t, minimal.Result().Body())
}

func TestPatchCanonicalThreadRejectsUnsafeFieldsWithoutMutation(t *testing.T) {
	tests := map[string]string{
		"space":             `{"metadata":{"space_id":"9999"}}`,
		"status":            `{"metadata":{"status":"running"}}`,
		"ttl":               `{"metadata":{"team":"alpha"},"ttl":"1h"}`,
		"too many metadata": fmt.Sprintf(`{"metadata":%s}`, canonicalTestMetadataJSON(t, 17)),
	}
	for name, body := range tests {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			h := authenticatedAgentThreadTestServer()
			h.PATCH("/api/workbench/threads/:thread_id", PatchCanonicalThread)
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "before", `{}`)
			path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10)

			response := performCanonicalThreadJSONRequest(t, h, http.MethodPatch, path, body)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code)

			stored, err := appagentthread.SVC.GetThread(context.Background(), &appagentthread.GetThreadRequest{
				ThreadID: thread.ThreadID,
			})
			require.NoError(t, err)
			require.Equal(t, "before", stored.Thread.Title)
		})
	}
}

func TestDeleteCanonicalThreadDeletesIdleAndRejectsBusyWithoutCanceling(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.DELETE("/api/workbench/threads/:thread_id", DeleteCanonicalThread)
	installAgentThreadTestService(t)

	idle := createCanonicalTestThread(t, 1001, "idle", `{}`)
	idlePath := "/api/workbench/threads/" + strconv.FormatInt(idle.ThreadID, 10)
	idleResponse := ut.PerformRequest(h.Engine, http.MethodDelete, idlePath, nil)
	require.Equal(t, http.StatusNoContent, idleResponse.Code)
	require.Empty(t, idleResponse.Result().Body())

	busy, err := appagentthread.SVC.CreateTaskThread(context.Background(), &appagentthread.CreateTaskThreadRequest{
		SpaceID: 1001,
		UserID:  2,
		Message: "keep running",
		Config:  `{"runtime":"eino_adk","mode":"pro"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, busy.Run)
	busyPath := "/api/workbench/threads/" + strconv.FormatInt(busy.Thread.ThreadID, 10)
	busyResponse := ut.PerformRequest(h.Engine, http.MethodDelete, busyPath, nil)
	require.Equal(t, http.StatusConflict, busyResponse.Code)
	var public canonicalError
	require.NoError(t, json.Unmarshal(busyResponse.Result().Body(), &public))
	require.Equal(t, "thread_busy", public.Code)

	stored, err := appagentthread.SVC.GetThread(context.Background(), &appagentthread.GetThreadRequest{
		ThreadID: busy.Thread.ThreadID,
	})
	require.NoError(t, err)
	require.NotNil(t, stored.Thread)
	run, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: busy.Run.RunID})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusPending, run.Run.Status)
}

func TestCanonicalThreadStateUpdatesOnlyPublicCustomAndPreservesEinoBytes(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/state", GetCanonicalThreadState)
	h.POST("/api/workbench/threads/:thread_id/state", UpdateCanonicalThreadState)
	installAgentThreadTestService(t)

	thread, checkpoint := createCanonicalStateFixture(t)
	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10) + "/state"
	before := sha256.Sum256([]byte(checkpoint.ChannelValues))

	rejected := ut.PerformRequest(h.Engine, http.MethodGet, path+"?subgraphs=true", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rejected.Code)

	body := fmt.Sprintf(
		`{"values":{"custom":{"theme":"dark","count":1}},"as_node":"ui","checkpoint_id":"%d"}`,
		checkpoint.CheckpointID,
	)
	updated := performCanonicalThreadJSONRequest(t, h, http.MethodPost, path, body)
	require.Equal(t, http.StatusOK, updated.Code)
	var result canonicalThreadUpdateStateResult
	require.NoError(t, json.Unmarshal(updated.Result().Body(), &result))
	require.Equal(t, result.Checkpoint, result.Configurable)
	require.NotEqual(t, strconv.FormatInt(checkpoint.CheckpointID, 10), result.Checkpoint.CheckpointID)

	afterCheckpoint, err := appagentthread.SVC.GetCheckpoint(
		context.Background(),
		&appagentthread.GetCheckpointRequest{CheckpointID: checkpoint.CheckpointID},
	)
	require.NoError(t, err)
	after := sha256.Sum256([]byte(afterCheckpoint.Checkpoint.ChannelValues))
	require.Equal(t, before, after)

	stateResponse := ut.PerformRequest(h.Engine, http.MethodGet, path+"?subgraphs=false", nil)
	require.Equal(t, http.StatusOK, stateResponse.Code)
	var state canonicalThreadState
	require.NoError(t, json.Unmarshal(stateResponse.Result().Body(), &state))
	require.Equal(t, result.Checkpoint.CheckpointID, state.Checkpoint.CheckpointID)
	require.Equal(t, "dark", state.Values["custom"].(map[string]any)["theme"])
	require.Empty(t, state.Tasks)
	require.NotContains(t, string(stateResponse.Result().Body()), "checkpoint_bytes")
	require.NotContains(t, string(stateResponse.Result().Body()), "channel_versions")

	unsupported := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, path, `{"values":{"messages":[]}}`,
	)
	require.Equal(t, http.StatusUnprocessableEntity, unsupported.Code)
}

func TestCanonicalThreadStateRejectsSensitiveCustomBeforePersistence(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads/:thread_id/state", UpdateCanonicalThreadState)
	installAgentThreadTestService(t)
	thread, _ := createCanonicalStateFixture(t)
	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10) + "/state"

	response := performCanonicalThreadJSONRequest(
		t,
		h,
		http.MethodPost,
		path,
		`{"values":{"custom":{"api_key":"must-not-persist","nested":{"checkpoint_bytes":"secret"}}}}`,
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)

	history, err := appagentthread.SVC.ListCheckpointsBefore(
		context.Background(),
		&appagentthread.ListCheckpointsBeforeRequest{ThreadID: thread.ThreadID, Limit: 10},
	)
	require.NoError(t, err)
	require.Len(t, history.Checkpoints, 1)
	require.Equal(t, string(appagentthread.RuntimeModeEinoADK), history.Checkpoints[0].RuntimeType)
}

func TestCanonicalThreadHistoryGETAndPOSTReturnSameOrderedSafeStates(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads/:thread_id/state", UpdateCanonicalThreadState)
	h.GET("/api/workbench/threads/:thread_id/history", GetCanonicalThreadHistory)
	h.POST("/api/workbench/threads/:thread_id/history", PostCanonicalThreadHistory)
	installAgentThreadTestService(t)

	thread, checkpoint := createCanonicalStateFixture(t)
	basePath := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10)
	statePath := basePath + "/state"
	firstBody := fmt.Sprintf(
		`{"values":{"custom":{"first":true}},"checkpoint_id":"%d"}`,
		checkpoint.CheckpointID,
	)
	first := performCanonicalThreadJSONRequest(t, h, http.MethodPost, statePath, firstBody)
	require.Equal(t, http.StatusOK, first.Code)
	second := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, statePath, `{"values":{"custom":{"second":true}}}`,
	)
	require.Equal(t, http.StatusOK, second.Code)

	getResponse := ut.PerformRequest(h.Engine, http.MethodGet, basePath+"/history?limit=10", nil)
	postResponse := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, basePath+"/history", `{"limit":10}`,
	)
	require.Equal(t, http.StatusOK, getResponse.Code)
	require.Equal(t, http.StatusOK, postResponse.Code)
	require.JSONEq(t, string(getResponse.Result().Body()), string(postResponse.Result().Body()))

	var history []*canonicalThreadState
	require.NoError(t, json.Unmarshal(getResponse.Result().Body(), &history))
	require.Len(t, history, 3)
	require.Equal(t, true, history[0].Values["custom"].(map[string]any)["first"])
	require.Equal(t, true, history[0].Values["custom"].(map[string]any)["second"])
	require.Greater(
		t,
		mustCanonicalTestID(t, history[0].Checkpoint.CheckpointID),
		mustCanonicalTestID(t, history[1].Checkpoint.CheckpointID),
	)
	require.NotContains(t, string(getResponse.Result().Body()), "opaque-eino-checkpoint-bytes")
}

func TestListCanonicalThreadMessagesLoadsCompleteJournalBeforeApplyingSeqCursor(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/messages", ListCanonicalThreadMessages)
	installAgentThreadTestService(t)

	created, err := appagentthread.SVC.CreateTaskThread(context.Background(), &appagentthread.CreateTaskThreadRequest{
		SpaceID: 1001,
		UserID:  2,
		Message: "message-000",
		Config:  `{"runtime":"eino_adk","mode":"pro"}`,
	})
	require.NoError(t, err)
	for index := 1; index <= 125; index++ {
		_, err := appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
			ThreadID: created.Thread.ThreadID,
			RunID:    created.Run.RunID,
			Role:     appagentthread.MessageRoleUser,
			Content:  fmt.Sprintf("message-%03d", index),
		})
		require.NoError(t, err)
	}
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  created.Thread.ThreadID,
		RunID:     created.Run.RunID,
		EventType: "tool.completed",
		Payload:   `{"tool_name":"secret_tool","result":"TOP_SECRET_TOOL_RESULT"}`,
	})
	require.NoError(t, err)

	path := "/api/workbench/threads/" + strconv.FormatInt(created.Thread.ThreadID, 10) + "/messages"
	firstResponse := ut.PerformRequest(h.Engine, http.MethodGet, path+"?limit=100", nil)
	require.Equal(t, http.StatusOK, firstResponse.Code)
	var firstPage struct {
		Data          []*canonicalMessage `json:"data"`
		HasMore       bool                `json:"has_more"`
		NextBeforeSeq *string             `json:"next_before_seq"`
		NextAfterSeq  *string             `json:"next_after_seq"`
	}
	require.NoError(t, json.Unmarshal(firstResponse.Result().Body(), &firstPage))
	require.Len(t, firstPage.Data, 100)
	require.True(t, firstPage.HasMore)
	require.Nil(t, firstPage.NextBeforeSeq)
	require.NotNil(t, firstPage.NextAfterSeq)
	require.Equal(t, "100", *firstPage.NextAfterSeq)
	require.Equal(t, "1", firstPage.Data[0].Seq)
	require.Equal(t, "100", firstPage.Data[99].Seq)

	secondResponse := ut.PerformRequest(h.Engine, http.MethodGet, path+"?after_seq=100&limit=100", nil)
	require.Equal(t, http.StatusOK, secondResponse.Code)
	var secondPage struct {
		Data         []*canonicalMessage `json:"data"`
		HasMore      bool                `json:"has_more"`
		NextAfterSeq *string             `json:"next_after_seq"`
	}
	require.NoError(t, json.Unmarshal(secondResponse.Result().Body(), &secondPage))
	require.Len(t, secondPage.Data, 26)
	require.False(t, secondPage.HasMore)
	require.Nil(t, secondPage.NextAfterSeq)
	require.Equal(t, "101", secondPage.Data[0].Seq)
	require.Equal(t, "126", secondPage.Data[25].Seq)
	require.NotContains(t, string(secondResponse.Result().Body()), "TOP_SECRET_TOOL_RESULT")

	beforeResponse := ut.PerformRequest(h.Engine, http.MethodGet, path+"?before_seq=101&limit=2", nil)
	require.Equal(t, http.StatusOK, beforeResponse.Code)
	var beforePage struct {
		Data          []*canonicalMessage `json:"data"`
		HasMore       bool                `json:"has_more"`
		NextBeforeSeq *string             `json:"next_before_seq"`
	}
	require.NoError(t, json.Unmarshal(beforeResponse.Result().Body(), &beforePage))
	require.Len(t, beforePage.Data, 2)
	require.Equal(t, "99", beforePage.Data[0].Seq)
	require.Equal(t, "100", beforePage.Data[1].Seq)
	require.True(t, beforePage.HasMore)
	require.NotNil(t, beforePage.NextBeforeSeq)
	require.Equal(t, "99", *beforePage.NextBeforeSeq)
}

func performCanonicalThreadJSONRequest(
	t *testing.T,
	h *server.Hertz,
	method string,
	path string,
	body string,
	headers ...ut.Header,
) *ut.ResponseRecorder {
	t.Helper()
	requestHeaders := []ut.Header{
		{Key: "Content-Type", Value: "application/json"},
		{Key: canonicalSpaceIDHeader, Value: "1001"},
	}
	requestHeaders = append(requestHeaders, headers...)
	return ut.PerformRequest(
		h.Engine,
		method,
		path,
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		requestHeaders...,
	)
}

func canonicalThreadMessagesAndRuns(
	t *testing.T,
	threadID int64,
) ([]*appagentthread.MessageSummary, []*appagentthread.RunSummary) {
	t.Helper()
	messages, err := appagentthread.SVC.ListMessages(context.Background(), &appagentthread.ListMessagesRequest{
		ThreadID: threadID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	runs, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: threadID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	return messages.Messages, runs.Runs
}

func createCanonicalTestThread(
	t *testing.T,
	spaceID int64,
	title string,
	metadata string,
) *appagentthread.ThreadSummary {
	return createCanonicalTestThreadForUser(t, spaceID, 2, title, metadata)
}

func createCanonicalTestThreadForUser(
	t *testing.T,
	spaceID int64,
	userID int64,
	title string,
	metadata string,
) *appagentthread.ThreadSummary {
	t.Helper()
	response, err := appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:  spaceID,
		UserID:   userID,
		Title:    title,
		Source:   appagentthread.ThreadSourceAPI,
		Metadata: metadata,
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Thread)
	return response.Thread
}

func createCanonicalStateFixture(
	t *testing.T,
) (*appagentthread.ThreadSummary, *appagentthread.CheckpointSummary) {
	t.Helper()
	created, err := appagentthread.SVC.CreateTaskThread(context.Background(), &appagentthread.CreateTaskThreadRequest{
		SpaceID: 1001,
		UserID:  2,
		Message: "state fixture",
		Config:  `{"runtime":"eino_adk","mode":"pro"}`,
	})
	require.NoError(t, err)
	checkpoint, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        created.Thread.ThreadID,
		RunID:           created.Run.RunID,
		CheckpointNS:    "eino.runtime",
		RuntimeType:     string(appagentthread.RuntimeModeEinoADK),
		RuntimeKey:      "thread:" + strconv.FormatInt(created.Thread.ThreadID, 10),
		EnvelopeVersion: 1,
		ChannelValues:   `{"opaque":"opaque-eino-checkpoint-bytes","checkpoint_bytes":"secret"}`,
		ChannelVersions: `{"secret":"version"}`,
		PendingSends:    `[]`,
		Metadata:        `{"source":"runtime","checkpoint_bytes":"do-not-project"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, checkpoint.Checkpoint)
	return created.Thread, checkpoint.Checkpoint
}

func canonicalThreadCount(t *testing.T, spaceID, userID int64) int {
	t.Helper()
	response, err := appagentthread.SVC.SearchThreads(context.Background(), &appagentthread.SearchThreadsRequest{
		SpaceID: spaceID,
		UserID:  userID,
		Page:    appagentthread.CanonicalPage{Limit: 100},
	})
	require.NoError(t, err)
	return len(response.Threads)
}

func mustCanonicalTestID(t *testing.T, value string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(value, 10, 64)
	require.NoError(t, err)
	require.Positive(t, id)
	return id
}

func canonicalTestMetadataJSON(t *testing.T, count int) string {
	t.Helper()
	metadata := make(map[string]any, count)
	for index := 0; index < count; index++ {
		metadata[fmt.Sprintf("key_%02d", index)] = index
	}
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	return string(raw)
}
