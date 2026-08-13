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

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	projectconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestCanonicalThreadResourceHandlersFailClosedWithoutApplicationService(t *testing.T) {
	previous := appagentthread.SVC
	appagentthread.SVC = nil
	t.Cleanup(func() {
		appagentthread.SVC = previous
	})

	h := canonicalAgentThreadTestServer()
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
	h := canonicalAgentThreadTestServer()
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

func TestCreateCanonicalThreadResponseIncludesServerReviewedCanEdit(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)

	response := performCanonicalThreadJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads",
		`{"metadata":{"title":"editable"}}`,
	)

	require.Equal(t, http.StatusOK, response.Code)
	requireCanonicalThreadCanEditJSON(t, string(response.Result().Body()), true)
	var projected canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &projected))
	require.Equal(t, "editable", projected.Metadata["title"])
}

func TestCreateCanonicalThreadRejectsForgedCanEditMetadata(t *testing.T) {
	for _, forged := range []bool{false, true} {
		forged := forged
		t.Run(strconv.FormatBool(forged), func(t *testing.T) {
			h := authenticatedAgentThreadTestServer()
			h.POST("/api/workbench/threads", CreateCanonicalThread)
			installAgentThreadTestService(t)

			before := canonicalThreadCount(t, 1001, 2)
			response := performCanonicalThreadJSONRequest(
				t,
				h,
				http.MethodPost,
				"/api/workbench/threads",
				fmt.Sprintf(`{"metadata":{"can_edit":%t,"title":"forged"}}`, forged),
			)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code)
			require.Equal(t, before, canonicalThreadCount(t, 1001, 2))
		})
	}
}

func TestCreateCanonicalThreadCreatesInitialSubmissionAtomically(t *testing.T) {
	h := canonicalAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)
	authorizer := &canonicalRecordingWorkspaceAuthorizer{}
	appagentthread.SVC.WorkspaceAuthorizer = authorizer

	body := `{
		"metadata":{"title":"新建任务"},
		"coze":{"initial_run":{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]},
			"config":{"runtime":"eino_adk"},
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
			h := canonicalAgentThreadTestServer()
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

func TestCreateCanonicalThreadRejectsExecutionControlsBeforeMutation(t *testing.T) {
	tests := map[string]string{
		"immediate nested mixed case": `{
			"metadata":{},
			"coze":{"initial_run":{
				"assistant_id":"agent",
				"input":{"messages":[{"role":"user","content":"do not persist"}]},
				"CoNfIg":{"CoNfIgUrAbLe":{"ReQuEsTeD_PoLiCy":"fast"}}
			}}
		}`,
		"deferred": `{
			"metadata":{},
			"coze":{"deferred_initial_run":{
				"assistant_id":"agent",
				"input":{"messages":[{"role":"user","content":"do not persist"}]},
				"context":{"reasoning_effort":"high"}
			}}
		}`,
	}

	for name, body := range tests {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			h := canonicalAgentThreadTestServer()
			h.POST("/api/workbench/threads", CreateCanonicalThread)
			installAgentThreadTestService(t)

			response := performCanonicalThreadJSONRequest(
				t, h, http.MethodPost, "/api/workbench/threads", body,
			)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			var public canonicalError
			require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
			require.Equal(t, "unsupported_execution_control", public.Code)
			require.Zero(t, canonicalThreadCount(t, 1001, 2))
		})
	}
}

func TestCreateCanonicalThreadRejectsOversizedInitialSubmissionBeforeMutation(t *testing.T) {
	h := canonicalAgentThreadTestServer()
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
	h := canonicalAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)

	const idempotencyKey = "canonical-initial-thread-1"
	firstBody := `{
		"metadata":{"title":"首提任务"},
		"coze":{"initial_run":{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"first payload"}]},
			"config":{"runtime":"eino_adk"},
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
	h := canonicalAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)
	authorizer := &canonicalRecordingWorkspaceAuthorizer{}
	appagentthread.SVC.WorkspaceAuthorizer = authorizer

	body := `{
		"metadata":{},
		"coze":{"deferred_initial_run":{
			"assistant_id":"agent",
			"input":{"messages":[{"role":"user","content":"分析附件中的销售数据"}]},
			"config":{"runtime":"eino_adk"},
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

func TestCreateCanonicalThreadAcceptsAtomicTypedV2(t *testing.T) {
	h := canonicalAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)

	submission := canonicalTypedInitialThreadSubmissionV2("hello")
	body := `{"metadata":{},"initial_submission_v2":` + submission + `}`
	response := performCanonicalThreadJSONRequest(t, h, http.MethodPost, "/api/workbench/threads", body)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var created canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &created))
	require.NotNil(t, created.Coze.InitialSubmission)
	initialJSON, err := json.Marshal(created.Coze.InitialSubmission)
	require.NoError(t, err)
	require.Contains(t, string(initialJSON), `"content":"hello"`)
	messages, runs := canonicalThreadMessagesAndRuns(t, mustCanonicalTestID(t, created.ThreadID))
	require.Len(t, messages, 1)
	require.Len(t, runs, 1)
	require.Equal(t, canonicalPublicAssistantID, runs[0].AssistantID)
	require.JSONEq(t, `{"source":"workbench_new_task"}`, messages[0].Metadata)
	typed, public := decodeCanonicalTypedInitialSubmissionV2([]byte(submission), "initial_submission_v2")
	require.Nil(t, public)
	mapped, public := mapCanonicalTypedInitialV2(typed, false)
	require.Nil(t, public)
	require.JSONEq(t, mapped.Config, runs[0].Config)
}

func TestCreateCanonicalThreadAcceptsDeferredTypedV2(t *testing.T) {
	h := canonicalAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)

	body := canonicalTypedInitialThreadRequestV2(
		"deferred_initial_submission_v2",
		"分析附件中的销售数据",
	)
	response := performCanonicalThreadJSONRequest(t, h, http.MethodPost, "/api/workbench/threads", body)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var created canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &created))
	require.Equal(t, "分析附件中的销售数据", created.Metadata["title"])
	require.Nil(t, created.Coze.InitialSubmission)
	messages, runs := canonicalThreadMessagesAndRuns(t, mustCanonicalTestID(t, created.ThreadID))
	require.Empty(t, messages)
	require.Empty(t, runs)
}

func TestCreateCanonicalThreadTypedV2RejectsAllVersionMixesWithoutMutation(t *testing.T) {
	initial := canonicalTypedInitialThreadSubmissionV2("typed initial")
	deferred := canonicalTypedInitialThreadSubmissionV2("typed deferred")
	tests := []struct {
		name string
		body string
		path string
	}{
		{
			name: "both typed variants",
			body: `{"metadata":{},"initial_submission_v2":` + initial +
				`,"deferred_initial_submission_v2":` + deferred + `}`,
			path: "initial_submission_v2",
		},
		{
			name: "typed initial and legacy initial",
			body: `{"metadata":{},"initial_submission_v2":` + initial +
				`,"coze":{"initial_run":{}}}`,
			path: "initial_submission_v2",
		},
		{
			name: "typed initial and legacy deferred",
			body: `{"metadata":{},"initial_submission_v2":` + initial +
				`,"coze":{"deferred_initial_run":{}}}`,
			path: "initial_submission_v2",
		},
		{
			name: "typed deferred and legacy initial",
			body: `{"metadata":{},"deferred_initial_submission_v2":` + deferred +
				`,"coze":{"initial_run":{}}}`,
			path: "deferred_initial_submission_v2",
		},
		{
			name: "typed deferred and legacy deferred",
			body: `{"metadata":{},"deferred_initial_submission_v2":` + deferred +
				`,"coze":{"deferred_initial_run":{}}}`,
			path: "deferred_initial_submission_v2",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := canonicalAgentThreadTestServer()
			h.POST("/api/workbench/threads", CreateCanonicalThread)
			installAgentThreadTestService(t)

			_, _, public := canonicalCreateThreadTypedSubmissionV2([]byte(test.body))
			require.NotNil(t, public)
			require.Equal(t, "mixed_submission_versions", public.errorClass)
			require.Contains(t, public.Detail, test.path)

			response := performCanonicalThreadJSONRequest(
				t, h, http.MethodPost, "/api/workbench/threads", test.body,
			)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			var responseError canonicalError
			require.NoError(t, json.Unmarshal(response.Result().Body(), &responseError))
			require.Equal(t, "invalid_request", responseError.Code)
			require.Contains(t, responseError.Detail, test.path)
			require.Zero(t, canonicalThreadCount(t, 1001, 2))
		})
	}
}

func TestCreateCanonicalThreadTypedV2RejectsClosedShapeViolationsWithoutMutation(t *testing.T) {
	unknown := canonicalTypedV2Replace(
		canonicalTypedInitialThreadSubmissionV2("secret-message-not-in-error"),
		`"config":{`,
		`"config":{"foo":"secret-config-value",`,
	)
	tests := []struct {
		name, body, code, path string
	}{
		{
			name: "explicit null",
			body: `{"metadata":{},"initial_submission_v2":null}`,
			code: "invalid_request",
			path: "initial_submission_v2",
		},
		{
			name: "unknown nested field",
			body: `{"metadata":{},"initial_submission_v2":` + unknown + `}`,
			code: "unsupported_sdk_field",
			path: "initial_submission_v2.config.foo",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := canonicalAgentThreadTestServer()
			h.POST("/api/workbench/threads", CreateCanonicalThread)
			installAgentThreadTestService(t)

			response := performCanonicalThreadJSONRequest(
				t, h, http.MethodPost, "/api/workbench/threads", test.body,
			)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			var public canonicalError
			require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
			require.Equal(t, test.code, public.Code)
			require.Contains(t, public.Detail, test.path)
			require.NotContains(t, public.Detail, "secret-")
			require.False(t, public.Retryable)
			require.Zero(t, canonicalThreadCount(t, 1001, 2))
		})
	}
}

func TestCreateCanonicalThreadTypedV2RejectsCaseVariantRootWithoutMutation(t *testing.T) {
	legacy := `{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"legacy must not run"}]}}`
	tests := []string{
		`{"metadata":{},"Initial_Submission_V2":` + canonicalTypedInitialThreadSubmissionV2("case variant") + `}`,
		`{"metadata":{},"DEFERRED_INITIAL_SUBMISSION_V2":` + canonicalTypedInitialThreadSubmissionV2("case variant") + `,"coze":{"initial_run":` + legacy + `}}`,
	}
	for _, body := range tests {
		h := canonicalAgentThreadTestServer()
		h.POST("/api/workbench/threads", CreateCanonicalThread)
		installAgentThreadTestService(t)
		response := performCanonicalThreadJSONRequest(t, h, http.MethodPost, "/api/workbench/threads", body)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
		var public canonicalError
		require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
		require.Equal(t, "unsupported_sdk_field", public.Code)
		require.Zero(t, canonicalThreadCount(t, 1001, 2))
	}
}

func TestCreateCanonicalThreadTypedV2RetiredControlWinsBeforeStrictAndMixing(t *testing.T) {
	deferred := canonicalTypedV2Replace(
		canonicalTypedInitialThreadSubmissionV2("do not persist deferred"),
		`"config":{`,
		`"config":{"configurable":{"context":{"reasoning_effort":"high"}},`,
	)
	initial := canonicalTypedV2Replace(
		canonicalTypedInitialThreadSubmissionV2("do not persist mixed"),
		`"config":{`,
		`"config":{"mode":"legacy",`,
	)
	tests := []struct{ name, body, path string }{
		{
			name: "deferred nested before strict unknown",
			body: `{"metadata":{},"deferred_initial_submission_v2":` + deferred + `}`,
			path: "deferred_initial_submission_v2.config.configurable.context.reasoning_effort",
		},
		{
			name: "retired before version mix",
			body: `{"metadata":{},"initial_submission_v2":` + initial +
				`,"coze":{"initial_run":{}}}`,
			path: "initial_submission_v2.config.mode",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := canonicalAgentThreadTestServer()
			h.POST("/api/workbench/threads", CreateCanonicalThread)
			installAgentThreadTestService(t)

			response := performCanonicalThreadJSONRequest(
				t, h, http.MethodPost, "/api/workbench/threads", test.body,
			)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			var public canonicalError
			require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
			require.Equal(t, "unsupported_execution_control", public.Code)
			require.Equal(t, "Unsupported execution control: "+test.path, public.Detail)
			require.Zero(t, canonicalThreadCount(t, 1001, 2))
		})
	}
}

func TestCreateCanonicalThreadTypedV2IdempotencyReplayAndConflict(t *testing.T) {
	h := canonicalAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)

	firstBody := canonicalTypedInitialThreadRequestV2("initial_submission_v2", "typed idempotent payload")
	header := ut.Header{Key: "Idempotency-Key", Value: "canonical-typed-initial-thread-1"}
	first := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads", firstBody, header,
	)
	require.Equal(t, http.StatusOK, first.Code, first.Result().Body())
	var firstThread canonicalThread
	require.NoError(t, json.Unmarshal(first.Result().Body(), &firstThread))

	replay := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads", firstBody, header,
	)
	require.Equal(t, http.StatusOK, replay.Code, replay.Result().Body())
	var replayedThread canonicalThread
	require.NoError(t, json.Unmarshal(replay.Result().Body(), &replayedThread))
	require.Equal(t, firstThread.ThreadID, replayedThread.ThreadID)

	changedBody := canonicalTypedInitialThreadRequestV2("initial_submission_v2", "changed typed payload")
	conflict := performCanonicalThreadJSONRequest(
		t, h, http.MethodPost, "/api/workbench/threads", changedBody, header,
	)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Result().Body())
	var public canonicalError
	require.NoError(t, json.Unmarshal(conflict.Result().Body(), &public))
	require.Equal(t, "idempotency_conflict", public.Code)
	require.Equal(t, 1, canonicalThreadCount(t, 1001, 2))
	messages, runs := canonicalThreadMessagesAndRuns(t, mustCanonicalTestID(t, firstThread.ThreadID))
	require.Len(t, messages, 1)
	require.Len(t, runs, 1)
	require.Equal(t, "typed idempotent payload", messages[0].Content)
	require.Contains(t, runs[0].Metadata, `"_idempotency"`)
}

func TestCreateCanonicalThreadTypedV2AuthorizesBeforeHostileBody(t *testing.T) {
	h := canonicalAgentThreadTestServer()
	h.POST("/api/workbench/threads", CreateCanonicalThread)
	installAgentThreadTestService(t)
	authorizer := &canonicalRecordingWorkspaceAuthorizer{err: appagentthread.ErrThreadAccessDenied}
	appagentthread.SVC.WorkspaceAuthorizer = authorizer

	response := performCanonicalThreadJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads",
		`{"initial_submission_v2":{"config":{"mode":"hostile-secret"}}}`,
	)
	require.Equal(t, http.StatusNotFound, response.Code, response.Result().Body())
	var public canonicalError
	require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
	require.Equal(t, "workspace_not_found", public.Code)
	require.Equal(t, 1, authorizer.calls)
	authorizer.err = nil
	require.Zero(t, canonicalThreadCount(t, 1001, 2))
}

func canonicalTypedInitialThreadSubmissionV2(message string) string {
	return canonicalTypedV2Replace(
		canonicalSemanticRunV2(canonicalStrictInitialV2),
		`"message":"hello"`,
		`"message":`+strconv.Quote(message),
	)
}

func canonicalTypedInitialThreadRequestV2(field, message string) string {
	return fmt.Sprintf(
		`{"metadata":{},%q:%s}`,
		field,
		canonicalTypedInitialThreadSubmissionV2(message),
	)
}

func TestCreateCanonicalThreadRejectsUnsupportedShapesWithoutSideEffects(t *testing.T) {
	initial := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]},
		"config":{"runtime":"eino_adk"},
		"metadata":{"source":"workbench_home"}
	}`
	deferred := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"分析附件中的销售数据"}]},
		"config":{"runtime":"eino_adk"},
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
			h := canonicalAgentThreadTestServer()
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
	h := canonicalAgentThreadTestServer()
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

func TestSearchCanonicalThreadsResponseIncludesServerReviewedCanEdit(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/threads/search", SearchCanonicalThreads)
	installAgentThreadTestService(t)
	createCanonicalTestThread(t, 1001, "editable", `{"can_edit":true,"team":"alpha"}`)

	response := performCanonicalThreadJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/search",
		`{"limit":10}`,
	)

	require.Equal(t, http.StatusOK, response.Code)
	var threads []json.RawMessage
	require.NoError(t, json.Unmarshal(response.Result().Body(), &threads))
	require.Len(t, threads, 1)
	requireCanonicalThreadCanEditJSON(t, string(threads[0]), true)
	var projected canonicalThread
	require.NoError(t, json.Unmarshal(threads[0], &projected))
	require.Equal(t, "alpha", projected.Metadata["team"])
}

func TestSearchCanonicalThreadsSupportsIDsSortAndPaginationHeaders(t *testing.T) {
	h := canonicalAgentThreadTestServer()
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
	h := canonicalAgentThreadTestServer()
	h.POST("/api/workbench/threads/search", SearchCanonicalThreads)
	installAgentThreadTestService(t)

	idle := createCanonicalTestThread(t, 1001, "idle", `{}`)
	busy, err := appagentthread.SVC.CreateTaskThread(context.Background(), &appagentthread.CreateTaskThreadRequest{
		SpaceID: 1001,
		UserID:  2,
		Message: "busy",
		Config:  `{"runtime":"eino_adk"}`,
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
			h := canonicalAgentThreadTestServer()
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
	h := canonicalAgentThreadTestServer()
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

func TestGetCanonicalThreadRejectsThreadOutsideDeclaredSpace(t *testing.T) {
	h := canonicalAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id", GetCanonicalThread)
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "space scoped", `{}`)
	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10)

	response := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		path,
		nil,
		ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
	)

	require.Equal(t, http.StatusNotFound, response.Code)
	var public canonicalError
	require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
	require.Equal(t, "resource_not_found", public.Code)
}

func TestGetCanonicalThreadResponseIncludesServerReviewedCanEdit(t *testing.T) {
	h := canonicalAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id", GetCanonicalThread)
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "editable", `{"can_edit":false,"team":"alpha"}`)
	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10)

	response := ut.PerformRequest(h.Engine, http.MethodGet, path, nil)

	require.Equal(t, http.StatusOK, response.Code)
	requireCanonicalThreadCanEditJSON(t, string(response.Result().Body()), true)
	var projected canonicalThread
	require.NoError(t, json.Unmarshal(response.Result().Body(), &projected))
	require.Equal(t, "alpha", projected.Metadata["team"])
}

func TestPatchCanonicalThreadUpdatesSafeMetadataAndSupportsMinimalResponse(t *testing.T) {
	h := canonicalAgentThreadTestServer()
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

func TestPatchCanonicalThreadRejectsForgedCanEditMetadata(t *testing.T) {
	for _, forged := range []bool{false, true} {
		forged := forged
		t.Run(strconv.FormatBool(forged), func(t *testing.T) {
			h := authenticatedAgentThreadTestServer()
			h.PATCH("/api/workbench/threads/:thread_id", PatchCanonicalThread)
			h.GET("/api/workbench/threads/:thread_id", GetCanonicalThread)
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "before", `{"team":"alpha"}`)
			path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10)

			response := performCanonicalThreadJSONRequest(
				t,
				h,
				http.MethodPatch,
				path,
				fmt.Sprintf(`{"metadata":{"can_edit":%t,"team":"forged"}}`, forged),
			)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code)

			read := ut.PerformRequest(h.Engine, http.MethodGet, path, nil)
			require.Equal(t, http.StatusOK, read.Code)
			requireCanonicalThreadCanEditJSON(t, string(read.Result().Body()), true)
			var projected canonicalThread
			require.NoError(t, json.Unmarshal(read.Result().Body(), &projected))
			require.Equal(t, "alpha", projected.Metadata["team"])
		})
	}
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
			h := canonicalAgentThreadTestServer()
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

type canonicalDeleteThreadIfIdleSpy struct {
	domainservice.ThreadService
	deleteCalls int
}

func (s *canonicalDeleteThreadIfIdleSpy) DeleteThreadIfIdle(
	context.Context,
	*domainservice.DeleteThreadIfIdleRequest,
) (bool, error) {
	s.deleteCalls++
	return true, nil
}

func TestDeleteCanonicalThreadIsTemporarilyDisabledAfterWorkspaceAuthorizationWithoutMutation(t *testing.T) {
	tests := []struct {
		name     string
		existing bool
	}{
		{name: "existing idle thread", existing: true},
		{name: "valid nonexistent positive thread ID"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			h := canonicalAgentThreadTestServer()
			h.Use(func(ctx context.Context, c *app.RequestContext) {
				ctx = context.WithValue(ctx, projectconsts.CtxLogIDKey, "trace-delete")
				c.Next(ctx)
			})
			h.DELETE("/api/workbench/threads/:thread_id", DeleteCanonicalThread)
			installAgentThreadTestService(t)

			authorizer := &canonicalRecordingWorkspaceAuthorizer{}
			appagentthread.SVC.WorkspaceAuthorizer = authorizer
			realThreadSVC := appagentthread.SVC.ThreadSVC
			spy := &canonicalDeleteThreadIfIdleSpy{ThreadService: realThreadSVC}
			appagentthread.SVC.ThreadSVC = spy

			threadID := int64(999)
			if test.existing {
				threadID = createCanonicalTestThread(t, 1001, "idle", `{}`).ThreadID
			}

			path := "/api/workbench/threads/" + strconv.FormatInt(threadID, 10)
			response := ut.PerformRequest(h.Engine, http.MethodDelete, path, nil)
			if response.Code != http.StatusServiceUnavailable {
				t.Errorf("expected exact HTTP status %d, got %d", http.StatusServiceUnavailable, response.Code)
			}
			if spy.deleteCalls != 0 {
				t.Errorf("expected DeleteThreadIfIdle calls 0, got %d", spy.deleteCalls)
			}
			if response.Code != http.StatusServiceUnavailable || spy.deleteCalls != 0 {
				return
			}

			body := response.Result().Body()
			var public canonicalError
			require.NoError(t, json.Unmarshal(body, &public))
			require.Equal(t, "thread_delete_temporarily_disabled", public.Code)
			require.Equal(t, "Thread deletion is temporarily unavailable", public.Detail)
			require.False(t, public.Retryable)
			require.Equal(t, "trace-delete", public.TraceID)
			require.NotContains(t, string(body), "error_code")
			require.Empty(t, response.Result().Header.Get("Retry-After"))
			require.Equal(t, 1, authorizer.calls)
			require.Equal(t, appagentthread.WorkspaceAccessRequest{
				ViewerID: 2,
				SpaceID:  1001,
			}, authorizer.req)

			if test.existing {
				stored, err := realThreadSVC.GetThread(context.Background(), threadID)
				require.NoError(t, err)
				require.NotNil(t, stored)
				require.Equal(t, threadID, stored.ID)
			}
		})
	}
}

func TestCanonicalThreadStateUpdatesOnlyPublicCustomAndPreservesEinoBytes(t *testing.T) {
	h := canonicalAgentThreadTestServer()
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
	h := canonicalAgentThreadTestServer()
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
	h := canonicalAgentThreadTestServer()
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
	h := canonicalAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/messages", ListCanonicalThreadMessages)
	installAgentThreadTestService(t)

	created, err := appagentthread.SVC.CreateTaskThread(context.Background(), &appagentthread.CreateTaskThreadRequest{
		SpaceID: 1001,
		UserID:  2,
		Message: "message-000",
		Config:  `{"runtime":"eino_adk"}`,
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
	require.Equal(t, "126", firstResponse.Result().Header.Get("X-Pagination-Total"))
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
	require.Equal(t, "126", secondResponse.Result().Header.Get("X-Pagination-Total"))
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
	require.Equal(t, "126", beforeResponse.Result().Header.Get("X-Pagination-Total"))
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

func TestListCanonicalThreadMessagesProjectsLegacyJournalIDsAsDecimalStrings(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/messages", ListCanonicalThreadMessages)
	installAgentThreadTestService(t)

	thread := createCanonicalTestThread(t, 1001, "legacy journal", `{}`)
	runResponse, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{
			ThreadID: thread.ThreadID,
			Input:    `{"messages":[{"role":"user","content":"legacy input"}]}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, runResponse)
	require.NotNil(t, runResponse.Run)
	eventResponse, err := appagentthread.SVC.AppendRunEvent(
		context.Background(),
		&appagentthread.AppendRunEventRequest{
			ThreadID:  thread.ThreadID,
			RunID:     runResponse.Run.RunID,
			EventType: "message.completed",
			Payload:   `{"role":"assistant","content":"legacy output"}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, eventResponse)
	require.NotNil(t, eventResponse.Event)

	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10) + "/messages"
	response := performCanonicalThreadJSONRequest(t, h, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, response.Code)
	var page canonicalMessagePage
	require.NoError(t, json.Unmarshal(response.Result().Body(), &page))
	require.Len(t, page.Data, 2)
	require.Equal(t, strconv.FormatInt(runResponse.Run.RunID, 10), page.Data[0].MessageID)
	require.Equal(t, strconv.FormatInt(eventResponse.Event.EventID, 10), page.Data[1].MessageID)
	for _, message := range page.Data {
		messageID, parseErr := strconv.ParseInt(message.MessageID, 10, 64)
		require.NoError(t, parseErr)
		require.Positive(t, messageID)
	}
	require.NotContains(t, string(response.Result().Body()), `"message_id":"run-`)
	require.NotContains(t, string(response.Result().Body()), `"message_id":"event-`)
}

func TestListCanonicalThreadMessagesPrefersPersistedAssistantOverVisibleEventDuplicate(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/messages", ListCanonicalThreadMessages)
	installAgentThreadTestService(t)

	thread := createCanonicalTestThread(t, 1001, "deduplicated journal", `{}`)
	runResponse, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{
			ThreadID: thread.ThreadID,
			Input:    `{"messages":[{"role":"user","content":"deduplicate input"}]}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, runResponse)
	require.NotNil(t, runResponse.Run)
	_, err = appagentthread.SVC.AppendRunEvent(
		context.Background(),
		&appagentthread.AppendRunEventRequest{
			ThreadID:  thread.ThreadID,
			RunID:     runResponse.Run.RunID,
			EventType: "message.completed",
			Payload:   `{"role":"assistant","content":"deduplicated output","tool_calls":[{"id":"call-1","name":"catalog","arguments":{}}]}`,
		},
	)
	require.NoError(t, err)
	persisted, err := appagentthread.SVC.AppendMessage(
		context.Background(),
		&appagentthread.AppendMessageRequest{
			ThreadID: thread.ThreadID,
			RunID:    runResponse.Run.RunID,
			Role:     appagentthread.MessageRoleAssistant,
			Content:  "deduplicated output",
		},
	)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Message)

	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10) + "/messages"
	response := performCanonicalThreadJSONRequest(t, h, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, response.Code)
	var page canonicalMessagePage
	require.NoError(t, json.Unmarshal(response.Result().Body(), &page))
	require.Len(t, page.Data, 2)
	require.Equal(t, appagentthread.MessageRoleUser, appagentthread.MessageRole(page.Data[0].Role))
	require.Equal(t, appagentthread.MessageRoleAssistant, appagentthread.MessageRole(page.Data[1].Role))
	require.Equal(t, "deduplicated output", page.Data[1].Content)
	require.Equal(t, strconv.FormatInt(persisted.Message.MessageID, 10), page.Data[1].MessageID)
	require.Equal(t, 1, strings.Count(string(response.Result().Body()), "deduplicated output"))
}

func TestListCanonicalThreadMessagesPrefersPersistedAssistantOverIntermediateEvent(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/messages", ListCanonicalThreadMessages)
	installAgentThreadTestService(t)

	thread := createCanonicalTestThread(t, 1001, "single public reply", `{}`)
	runResponse, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{
			ThreadID: thread.ThreadID,
			Input:    `{"messages":[{"role":"user","content":"create a report"}]}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, runResponse)
	require.NotNil(t, runResponse.Run)
	_, err = appagentthread.SVC.AppendRunEvent(
		context.Background(),
		&appagentthread.AppendRunEventRequest{
			ThreadID:  thread.ThreadID,
			RunID:     runResponse.Run.RunID,
			EventType: "message.completed",
			Payload:   `{"role":"assistant","content":"I will write the report now.","tool_calls":[{"id":"call-1","name":"write_file","arguments":{}}]}`,
		},
	)
	require.NoError(t, err)
	persisted, err := appagentthread.SVC.AppendMessage(
		context.Background(),
		&appagentthread.AppendMessageRequest{
			ThreadID: thread.ThreadID,
			RunID:    runResponse.Run.RunID,
			Role:     appagentthread.MessageRoleAssistant,
			Content:  "The report is ready.",
		},
	)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Message)

	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10) + "/messages"
	response := performCanonicalThreadJSONRequest(t, h, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, response.Code)
	var page canonicalMessagePage
	require.NoError(t, json.Unmarshal(response.Result().Body(), &page))
	require.Len(t, page.Data, 2)
	require.Equal(t, appagentthread.MessageRoleUser, appagentthread.MessageRole(page.Data[0].Role))
	require.Equal(t, appagentthread.MessageRoleAssistant, appagentthread.MessageRole(page.Data[1].Role))
	require.Equal(t, "The report is ready.", page.Data[1].Content)
	require.Equal(t, strconv.FormatInt(persisted.Message.MessageID, 10), page.Data[1].MessageID)
	require.NotContains(t, string(response.Result().Body()), "I will write the report now.")
}

func TestListCanonicalThreadMessagesKeepsLatestPersistedAssistantPerRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/messages", ListCanonicalThreadMessages)
	installAgentThreadTestService(t)

	thread := createCanonicalTestThread(t, 1001, "latest durable reply", `{}`)
	runResponse, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{
			ThreadID: thread.ThreadID,
			Input:    `{"messages":[{"role":"user","content":"create a report"}]}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, runResponse)
	require.NotNil(t, runResponse.Run)
	for _, content := range []string{"Obsolete durable reply.", "Authoritative durable reply."} {
		_, err = appagentthread.SVC.AppendMessage(
			context.Background(),
			&appagentthread.AppendMessageRequest{
				ThreadID: thread.ThreadID,
				RunID:    runResponse.Run.RunID,
				Role:     appagentthread.MessageRoleAssistant,
				Content:  content,
			},
		)
		require.NoError(t, err)
	}

	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10) + "/messages"
	response := performCanonicalThreadJSONRequest(t, h, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, response.Code)
	var page canonicalMessagePage
	require.NoError(t, json.Unmarshal(response.Result().Body(), &page))
	require.Len(t, page.Data, 2)
	require.Equal(t, appagentthread.MessageRoleUser, appagentthread.MessageRole(page.Data[0].Role))
	require.Equal(t, appagentthread.MessageRoleAssistant, appagentthread.MessageRole(page.Data[1].Role))
	require.Equal(t, "Authoritative durable reply.", page.Data[1].Content)
	require.NotContains(t, string(response.Result().Body()), "Obsolete durable reply.")
}

func TestListCanonicalThreadMessagesKeepsLatestEventAssistantWithoutPersistedReply(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/messages", ListCanonicalThreadMessages)
	installAgentThreadTestService(t)

	thread := createCanonicalTestThread(t, 1001, "event reply fallback", `{}`)
	runResponse, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{
			ThreadID: thread.ThreadID,
			Input:    `{"messages":[{"role":"user","content":"need clarification"}]}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, runResponse)
	require.NotNil(t, runResponse.Run)
	for _, content := range []string{"Checking the request.", "Which format should I use?"} {
		_, err = appagentthread.SVC.AppendRunEvent(
			context.Background(),
			&appagentthread.AppendRunEventRequest{
				ThreadID:  thread.ThreadID,
				RunID:     runResponse.Run.RunID,
				EventType: "message.completed",
				Payload:   fmt.Sprintf(`{"role":"assistant","content":%q}`, content),
			},
		)
		require.NoError(t, err)
	}
	_, err = appagentthread.SVC.AppendRunEvent(
		context.Background(),
		&appagentthread.AppendRunEventRequest{
			ThreadID:  thread.ThreadID,
			RunID:     runResponse.Run.RunID,
			EventType: "message.completed",
			Payload:   `{"role":"assistant","content":"\u0001"}`,
		},
	)
	require.NoError(t, err)

	path := "/api/workbench/threads/" + strconv.FormatInt(thread.ThreadID, 10) + "/messages"
	response := performCanonicalThreadJSONRequest(t, h, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, response.Code)
	var page canonicalMessagePage
	require.NoError(t, json.Unmarshal(response.Result().Body(), &page))
	require.Len(t, page.Data, 2)
	require.Equal(t, appagentthread.MessageRoleUser, appagentthread.MessageRole(page.Data[0].Role))
	require.Equal(t, appagentthread.MessageRoleAssistant, appagentthread.MessageRole(page.Data[1].Role))
	require.Equal(t, "Which format should I use?", page.Data[1].Content)
	require.NotContains(t, string(response.Result().Body()), "Checking the request.")
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

func canonicalAgentThreadTestServer() *server.Hertz {
	return canonicalAgentThreadTestServerForUser(2)
}

func canonicalAgentThreadTestServerForUser(userID int64) *server.Hertz {
	return canonicalAgentThreadTestServerForUserAndSpace(userID, 1001)
}

func canonicalAgentThreadTestServerForUserAndSpace(userID, spaceID int64) *server.Hertz {
	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(userID))
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		if len(c.GetHeader(canonicalSpaceIDHeader)) == 0 {
			c.Request.Header.Set(canonicalSpaceIDHeader, strconv.FormatInt(spaceID, 10))
		}
		c.Next(ctx)
	})
	return h
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
		Config:  `{"runtime":"eino_adk"}`,
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
