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
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestCanonicalRunRequestDefaultsAndAllowlist(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)

	tests := []struct {
		name        string
		extraFields string
		wantModes   []string
	}{
		{name: "defaults", wantModes: []string{"values"}},
		{name: "single mode", extraFields: `,"stream_mode":"updates"`, wantModes: []string{"updates"}},
		{name: "mode array", extraFields: `,"stream_mode":["messages-tuple","custom","events"]`, wantModes: []string{"messages-tuple", "custom", "events"}},
		{name: "fixed sdk compatibility", extraFields: `,
			"stream_subgraphs":false,
			"stream_resumable":false,
			"if_not_exists":"reject",
			"webhook":null,
			"on_completion":null,
			"after_seconds":null,
			"feedback_keys":null,
			"interrupt_before":null,
			"interrupt_after":null,
			"checkpoint":null,
			"checkpoint_id":null,
			"langsmith_tracer":null`, wantModes: []string{"values"}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			thread := createCanonicalTestThread(t, 1001, "run validation", `{}`)
			h := canonicalRunTestServer()
			body := fmt.Sprintf(`{
				"assistant_id":"agent",
				"input":{"messages":[{"role":"user","content":"continue"}]}
				%s
			}`, test.extraFields)

			response := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
				body,
			)

			require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
			var created canonicalRun
			require.NoError(t, json.Unmarshal(response.Result().Body(), &created))
			require.Equal(t, test.wantModes, created.Coze.StreamModes)
			require.Equal(t, "reject", created.MultitaskStrategy)
			require.Equal(t, "cancel", created.Coze.OnDisconnect)
			require.Equal(t, "async", created.Coze.Durability)
		})
	}
}

func TestCanonicalRunRequestRejectsUnsupportedFieldsWithoutSideEffects(t *testing.T) {
	tests := map[string]string{
		"unknown field":             `,"future_field":true`,
		"unknown stream mode":       `,"stream_mode":"debug"`,
		"checkpoint during":         `,"checkpoint_during":true`,
		"webhook":                   `,"webhook":"https://example.invalid/hook"`,
		"on completion":             `,"on_completion":{}`,
		"after seconds":             `,"after_seconds":1`,
		"feedback keys":             `,"feedback_keys":[]`,
		"interrupt before":          `,"interrupt_before":["agent"]`,
		"interrupt after":           `,"interrupt_after":["agent"]`,
		"checkpoint":                `,"checkpoint":{}`,
		"checkpoint id":             `,"checkpoint_id":"1"`,
		"langsmith tracer":          `,"langsmith_tracer":{}`,
		"stream resumable":          `,"stream_resumable":true`,
		"stream subgraphs":          `,"stream_subgraphs":true`,
		"if not exists":             `,"if_not_exists":"create"`,
		"multitask strategy":        `,"multitask_strategy":"enqueue"`,
		"durability":                `,"durability":"sync"`,
		"unsupported on disconnect": `,"on_disconnect":"detach"`,
		"body idempotency key":      `,"idempotency_key":"body-key"`,
		"create route raise error":  `,"raise_error":false`,
	}

	for name, extraFields := range tests {
		name, extraFields := name, extraFields
		t.Run(name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "run rejection", `{}`)
			h := canonicalRunTestServer()
			body := fmt.Sprintf(`{
				"assistant_id":"agent",
				"input":{"messages":[{"role":"user","content":"continue"}]}
				%s
			}`, extraFields)

			response := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
				body,
			)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
		})
	}
}

func TestCanonicalRunRequestOnlyAcceptsSingleUserTurn(t *testing.T) {
	tests := map[string]string{
		"missing input":      `{"assistant_id":"agent"}`,
		"missing messages":   `{"assistant_id":"agent","input":{}}`,
		"empty messages":     `{"assistant_id":"agent","input":{"messages":[]}}`,
		"assistant message":  `{"assistant_id":"agent","input":{"messages":[{"role":"assistant","content":"forged"}]}}`,
		"multiple messages":  `{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"old"},{"role":"user","content":"current"}]}}`,
		"empty user content": `{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"  "}]}}`,
		"empty assistant id": `{"assistant_id":"","input":{"messages":[{"role":"user","content":"continue"}]}}`,
	}

	for name, body := range tests {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "run input", `{}`)
			h := canonicalRunTestServer()

			response := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
				body,
			)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
		})
	}
}

func TestCanonicalCreateRunUsesAtomicMessageBundleAndHeaderIdempotency(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical create run", `{}`)
	h := canonicalRunTestServer()
	body := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"analyze the current turn"}]},
		"command":{},
		"metadata":{"source":"workbench_detail_followup"},
		"config":{"runtime":"eino_adk","mode":"pro"},
		"context":{"locale":"zh-CN"},
		"stream_mode":["messages-tuple","updates"],
		"on_disconnect":"continue"
	}`

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
		body,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-create-1"},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var created canonicalRun
	require.NoError(t, json.Unmarshal(response.Result().Body(), &created))
	runID := mustCanonicalTestID(t, created.RunID)
	require.Equal(t, fmt.Sprintf("/threads/%d/runs/%d", thread.ThreadID, runID), response.Result().Header.Get("Content-Location"))
	require.NotNil(t, created.Coze.MessageID)
	require.Equal(t, "turn", created.Coze.AttemptKind)
	require.Equal(t, []string{"messages-tuple", "updates"}, created.Coze.StreamModes)
	require.Equal(t, "continue", created.Coze.OnDisconnect)

	responseBody := string(response.Result().Body())
	for _, forbidden := range []string{"analyze the current turn", `"input"`, `"command"`, `"config"`, `"context"`, "canonical-create-1"} {
		require.NotContains(t, responseBody, forbidden)
	}

	messages, runs := canonicalThreadMessagesAndRuns(t, thread.ThreadID)
	require.Len(t, messages, 1)
	require.Len(t, runs, 1)
	require.Equal(t, runID, messages[0].RunID)
	require.Equal(t, "analyze the current turn", messages[0].Content)
	require.Equal(t, canonicalScopedIdempotencyKey(2, "canonical-create-1"), runs[0].IdempotencyKey)
	require.Contains(t, runs[0].Input, "analyze the current turn")
	require.Contains(t, runs[0].Metadata, `"_message":{"message_id":`)
	require.Contains(t, runs[0].Metadata, `"_idempotency":`)
	require.Equal(t, created.RunID, fmt.Sprint(runs[0].RunID))
	require.Equal(t, *created.Coze.MessageID, fmt.Sprint(messages[0].MessageID))

	read := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d", thread.ThreadID, runID),
		nil,
	)
	require.Equal(t, http.StatusOK, read.Code, read.Result().Body())
	var persisted canonicalRun
	require.NoError(t, json.Unmarshal(read.Result().Body(), &persisted))
	require.Equal(t, created.Coze.MessageID, persisted.Coze.MessageID)
	require.NotContains(t, string(read.Result().Body()), "_message")
	require.NotContains(t, string(read.Result().Body()), "_idempotency")
}

func TestCanonicalCreateRunRejectsIdempotencyKeyOwnedByAnotherThread(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	firstThread := createCanonicalTestThread(t, 1001, "canonical first idempotency owner", `{}`)
	secondThread := createCanonicalTestThread(t, 1001, "canonical second idempotency owner", `{}`)
	h := canonicalRunTestServer()
	body := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"idempotent request"}]}
	}`
	header := ut.Header{Key: "Idempotency-Key", Value: "canonical-cross-thread-1"}

	first := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", firstThread.ThreadID),
		body,
		header,
	)
	require.Equal(t, http.StatusOK, first.Code, first.Result().Body())

	conflict := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", secondThread.ThreadID),
		body,
		header,
	)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Result().Body())
	var public canonicalError
	require.NoError(t, json.Unmarshal(conflict.Result().Body(), &public))
	require.Equal(t, "idempotency_conflict", public.Code)
	require.Empty(t, canonicalRunsForThread(t, secondThread.ThreadID))
}

func TestCanonicalCreateRunScopesIdempotencyBySessionPrincipal(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	firstThread := createCanonicalTestThreadForUser(t, 1001, 2, "first principal", `{}`)
	secondThread := createCanonicalTestThreadForUser(t, 1001, 3, "second principal", `{}`)
	firstServer := canonicalRunTestServerForUser(2)
	secondServer := canonicalRunTestServerForUser(3)
	body := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"principal-scoped request"}]}
	}`
	header := ut.Header{Key: "Idempotency-Key", Value: "shared-client-key"}

	first := performCanonicalRunJSONRequest(
		t, firstServer, http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", firstThread.ThreadID), body, header,
	)
	second := performCanonicalRunJSONRequest(
		t, secondServer, http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", secondThread.ThreadID), body, header,
	)

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, http.StatusOK, second.Code)
	require.Len(t, canonicalRunsForThread(t, firstThread.ThreadID), 1)
	require.Len(t, canonicalRunsForThread(t, secondThread.ThreadID), 1)
}

func TestCanonicalListRunsUsesExactPaginationAndRejectsSelect(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical list runs", `{}`)
	h := canonicalRunTestServer()

	for index := 0; index < 3; index++ {
		run := createCanonicalRunFixture(t, thread.ThreadID, fmt.Sprintf("turn %d", index+1))
		if index < 2 {
			cancelCanonicalRunFixture(t, run)
		}
	}

	path := fmt.Sprintf("/api/workbench/threads/%d/runs?limit=1&offset=1", thread.ThreadID)
	response := ut.PerformRequest(h.Engine, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var runs []*canonicalRun
	require.NoError(t, json.Unmarshal(response.Result().Body(), &runs))
	require.Len(t, runs, 1)
	require.Equal(t, "3", response.Result().Header.Get("X-Pagination-Total"))
	require.Equal(t, "2", response.Result().Header.Get("X-Pagination-Next"))

	pending := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs?status=pending", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusOK, pending.Code, pending.Result().Body())
	var pendingRuns []*canonicalRun
	require.NoError(t, json.Unmarshal(pending.Result().Body(), &pendingRuns))
	require.Len(t, pendingRuns, 1)
	require.Equal(t, "pending", pendingRuns[0].Status)

	selected := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs?select=run_id", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, selected.Code, selected.Result().Body())
}

func TestCanonicalGetRunChecksPathOwnershipAndReturnsMinimalProjection(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	firstThread := createCanonicalTestThread(t, 1001, "first", `{}`)
	secondThread := createCanonicalTestThread(t, 1001, "second", `{}`)
	run := createCanonicalRunFixture(t, secondThread.ThreadID, "private second-thread turn")
	h := canonicalRunTestServer()

	mismatch := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d", firstThread.ThreadID, run.RunID),
		nil,
	)
	require.Equal(t, http.StatusNotFound, mismatch.Code, mismatch.Result().Body())
	var publicError canonicalError
	require.NoError(t, json.Unmarshal(mismatch.Result().Body(), &publicError))
	require.Equal(t, "resource_not_found", publicError.Code)

	response := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d", secondThread.ThreadID, run.RunID),
		nil,
	)
	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var projected canonicalRun
	require.NoError(t, json.Unmarshal(response.Result().Body(), &projected))
	require.Equal(t, fmt.Sprint(secondThread.ThreadID), projected.ThreadID)
	require.Equal(t, fmt.Sprint(run.RunID), projected.RunID)
	for _, forbidden := range []string{"private second-thread turn", `"input"`, `"command"`, `"config"`, `"context"`} {
		require.NotContains(t, string(response.Result().Body()), forbidden)
	}
}

func TestCanonicalJoinReturnsRawPublicValuesWithoutSSE(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical join", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "join this run")
	completeCanonicalRunWithPublicState(t, run, `{"custom":{"answer":42}}`)
	h := canonicalRunTestServer()

	for _, query := range []string{"", "?cancel_on_disconnect=false", "?cancel_on_disconnect=0"} {
		response := ut.PerformRequest(
			h.Engine,
			http.MethodGet,
			fmt.Sprintf("/api/workbench/threads/%d/runs/%d/join%s", thread.ThreadID, run.RunID, query),
			nil,
		)

		require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
		require.Equal(t, "application/json; charset=utf-8", response.Result().Header.Get("Content-Type"))
		require.NotContains(t, response.Result().Header.Get("Content-Type"), "text/event-stream")
		require.Equal(t, canonicalRunJoinPath(thread.ThreadID, run.RunID), response.Result().Header.Get("Location"))
		var values map[string]any
		require.NoError(t, json.Unmarshal(response.Result().Body(), &values))
		require.Equal(t, float64(42), values["custom"].(map[string]any)["answer"])
		require.NotContains(t, values, "run_id")
	}

	unsupported := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/join?cancel_on_disconnect=true", thread.ThreadID, run.RunID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, unsupported.Code, unsupported.Result().Body())
}

func TestCanonicalJoinReturnsSafeFailureValues(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical failed join", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "join failed run")
	failCanonicalRunFixture(t, run, "provider_http_500", "secret provider body and stack")
	h := canonicalRunTestServer()

	response := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/join", thread.ThreadID, run.RunID),
		nil,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var values map[string]any
	require.NoError(t, json.Unmarshal(response.Result().Body(), &values))
	publicError := values["__error__"].(map[string]any)
	require.Equal(t, "provider_http_500", publicError["error"])
	require.Equal(t, "Model request failed", publicError["message"])
	require.NotContains(t, string(response.Result().Body()), "secret provider body")
}

func TestCanonicalWaitReusesIdempotentRunAndReturnsRawPublicValues(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical wait", `{}`)
	h := canonicalRunTestServer()
	body := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"wait for this turn"}]},
		"on_disconnect":"continue"
	}`
	created := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
		body,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-wait-1"},
	)
	require.Equal(t, http.StatusOK, created.Code, created.Result().Body())
	var createdRun canonicalRun
	require.NoError(t, json.Unmarshal(created.Result().Body(), &createdRun))
	runs := canonicalRunsForThread(t, thread.ThreadID)
	require.Len(t, runs, 1)
	run := runs[0]
	require.Equal(t, mustCanonicalTestID(t, createdRun.RunID), run.RunID)
	completeCanonicalRunWithPublicState(t, run, `{"custom":{"summary":"done"}}`)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/wait", thread.ThreadID),
		body,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-wait-1"},
	)
	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	require.Equal(t, canonicalRunPath(thread.ThreadID, run.RunID), response.Result().Header.Get("Content-Location"))
	require.Equal(t, canonicalRunJoinPath(thread.ThreadID, run.RunID), response.Result().Header.Get("Location"))
	var values map[string]any
	require.NoError(t, json.Unmarshal(response.Result().Body(), &values))
	require.Equal(t, "done", values["custom"].(map[string]any)["summary"])
	require.NotContains(t, values, "run_id")
	require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 1)
}

func TestCanonicalCancelRunIsIdempotentAndReturnsNoContent(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical cancel", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "cancel this run")
	h := canonicalRunTestServer()
	path := fmt.Sprintf("/api/workbench/threads/%d/runs/%d/cancel", thread.ThreadID, run.RunID)

	response := ut.PerformRequest(h.Engine, http.MethodPost, path+"?wait=0&action=interrupt", nil)
	require.Equal(t, http.StatusNoContent, response.Code, response.Result().Body())
	require.Empty(t, response.Result().Body())
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: run.RunID})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusCanceled, persisted.Run.Status)

	repeated := ut.PerformRequest(h.Engine, http.MethodPost, path+"?wait=1", nil)
	require.Equal(t, http.StatusNoContent, repeated.Code, repeated.Result().Body())
	require.Empty(t, repeated.Result().Body())
}

func TestCanonicalCancelRunRejectsRollbackWithoutSideEffects(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical rollback rejection", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "keep this run")
	h := canonicalRunTestServer()

	response := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/cancel?action=rollback", thread.ThreadID, run.RunID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: run.RunID})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusPending, persisted.Run.Status)
}

func TestCanonicalCancelRunIsIdempotentForTerminalRuns(t *testing.T) {
	tests := []struct {
		name     string
		terminal func(*testing.T, *appagentthread.RunSummary)
	}{
		{name: "succeeded", terminal: func(t *testing.T, run *appagentthread.RunSummary) {
			completeCanonicalRunWithPublicState(t, run, `{"completion":"done"}`)
		}},
		{name: "failed", terminal: func(t *testing.T, run *appagentthread.RunSummary) {
			failCanonicalRunFixture(t, run, "provider_timeout", "safe failure")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "canonical terminal cancel", `{}`)
			run := createCanonicalRunFixture(t, thread.ThreadID, "terminal run")
			test.terminal(t, run)
			h := canonicalRunTestServer()

			response := ut.PerformRequest(
				h.Engine,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs/%d/cancel", thread.ThreadID, run.RunID),
				nil,
			)
			require.Equal(t, http.StatusNoContent, response.Code, response.Result().Body())
			require.Empty(t, response.Result().Body())
		})
	}
}

func TestCanonicalWaitCancellationHonorsEndpointDisconnectMode(t *testing.T) {
	tests := []struct {
		name               string
		cancelOnDisconnect bool
		wantStatus         appagentthread.RunStatus
	}{
		{name: "wait cancel mode", cancelOnDisconnect: true, wantStatus: appagentthread.RunStatusCanceled},
		{name: "join keeps run", cancelOnDisconnect: false, wantStatus: appagentthread.RunStatusPending},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "canonical disconnect", `{}`)
			run := createCanonicalRunFixture(t, thread.ThreadID, "disconnect run")
			accessCtx := appagentthread.WithThreadAccessRequest(
				context.Background(),
				appagentthread.ThreadAccessRequest{
					ViewerID: 2, ThreadID: thread.ThreadID, RunID: run.RunID,
				},
			)
			ctx, cancel := context.WithCancel(accessCtx)
			cancel()

			_, err := waitCanonicalRunTerminal(ctx, thread.ThreadID, run, test.cancelOnDisconnect)
			require.ErrorIs(t, err, context.Canceled)
			persisted, getErr := appagentthread.SVC.GetRun(
				context.Background(),
				&appagentthread.GetRunRequest{RunID: run.RunID},
			)
			require.NoError(t, getErr)
			require.Equal(t, test.wantStatus, persisted.Run.Status)
		})
	}
}

func TestCanonicalResumeRouteUsesHumanInteractionApplicationUseCase(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	sourceRunID := createInterruptedHumanInteractionRun(t)
	h := canonicalRunTestServer()
	payload := `{
		"interrupt_id":"interrupt-1",
		"response":{
			"schema":"coze.human_interaction_response.v1",
			"interaction_id":"hi_1",
			"kind":"clarification",
			"decision":"answered",
			"answer":"最近 7 天"
		}
	}`

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/resume", sourceRunID),
		payload,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-resume-route-1"},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var resumed canonicalRun
	require.NoError(t, json.Unmarshal(response.Result().Body(), &resumed))
	require.Equal(t, "pending", resumed.Status)
	require.Equal(t, "resume", resumed.Coze.AttemptKind)
	require.Equal(t, fmt.Sprint(sourceRunID), *resumed.Coze.SourceRunID)
	require.NotContains(t, string(response.Result().Body()), "最近 7 天")
	assertCanonicalResumePersistence(t, sourceRunID, "canonical-resume-route-1")
}

func TestCanonicalResumeRejectsIdempotencyKeyOwnedByAnotherThread(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	sourceRunID := createInterruptedHumanInteractionRun(t)
	otherThread := createCanonicalTestThread(t, 1, "other resume thread", `{}`)
	conflicting, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: otherThread.ThreadID, AssistantID: "agent", Input: `{"uploaded_files":[]}`,
		MessageContent: "other turn", StreamMode: `["values"]`, MultitaskStrategy: "reject",
		OnDisconnect: "cancel", Durability: "async",
		IdempotencyKey: canonicalScopedIdempotencyKey(2, "canonical-resume-conflict"),
	})
	require.NoError(t, err)
	require.NotNil(t, conflicting)
	require.NotNil(t, conflicting.Run)
	h := canonicalRunTestServer()
	payload := `{
		"interrupt_id":"interrupt-1",
		"response":{
			"schema":"coze.human_interaction_response.v1",
			"interaction_id":"hi_1",
			"kind":"clarification",
			"decision":"answered",
			"answer":"last 7 days"
		}
	}`

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/resume", sourceRunID),
		payload,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-resume-conflict"},
	)

	require.Equal(t, http.StatusConflict, response.Code, response.Result().Body())
	require.Contains(t, string(response.Result().Body()), `"code":"idempotency_conflict"`)
	require.NotContains(t, string(response.Result().Body()), strconv.FormatInt(conflicting.Run.RunID, 10))
	require.Len(t, canonicalRunsForThread(t, 1), 1)
}

func TestCanonicalCreateRunCommandResumeUsesSameApplicationUseCase(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	sourceRunID := createInterruptedHumanInteractionRun(t)
	h := canonicalRunTestServer()
	payload := fmt.Sprintf(`{
		"assistant_id":"agent",
		"command":{"resume":{
			"source_run_id":"%d",
			"interrupt_id":"interrupt-1",
			"response":{
				"schema":"coze.human_interaction_response.v1",
				"interaction_id":"hi_1",
				"kind":"clarification",
				"decision":"answered",
				"answer":"最近 30 天"
			}
		}}
	}`, sourceRunID)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs",
		payload,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-command-resume-1"},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var resumed canonicalRun
	require.NoError(t, json.Unmarshal(response.Result().Body(), &resumed))
	require.Equal(t, "resume", resumed.Coze.AttemptKind)
	require.Equal(t, fmt.Sprint(sourceRunID), *resumed.Coze.SourceRunID)
	require.Equal(t, canonicalRunPath(1, mustCanonicalTestID(t, resumed.RunID)), response.Result().Header.Get("Content-Location"))
	require.NotContains(t, string(response.Result().Body()), "最近 30 天")
	assertCanonicalResumePersistence(t, sourceRunID, "canonical-command-resume-1")
}

func TestCanonicalResumeRoutesShareFingerprintAndRejectTurnReuse(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	sourceRunID := createInterruptedHumanInteractionRun(t)
	h := canonicalRunTestServer()
	header := ut.Header{Key: "Idempotency-Key", Value: "canonical-resume-shared-1"}
	response := canonicalResumeResponse{
		Schema: "coze.human_interaction_response.v1", InteractionID: "hi_1",
		Kind: "clarification", Decision: "answered", Answer: "last 14 days",
	}
	dedicatedBody, err := json.Marshal(canonicalResumeRunRequest{
		InterruptID: "interrupt-1", Response: response,
	})
	require.NoError(t, err)

	dedicated := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/resume", sourceRunID),
		string(dedicatedBody),
		header,
	)
	require.Equal(t, http.StatusOK, dedicated.Code, dedicated.Result().Body())
	var first canonicalRun
	require.NoError(t, json.Unmarshal(dedicated.Result().Body(), &first))

	commandBody, err := json.Marshal(map[string]any{
		"assistant_id": "agent",
		"command": map[string]any{"resume": map[string]any{
			"source_run_id": strconv.FormatInt(sourceRunID, 10),
			"interrupt_id":  "interrupt-1",
			"response":      response,
		}},
	})
	require.NoError(t, err)
	command := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs",
		string(commandBody),
		header,
	)
	require.Equal(t, http.StatusOK, command.Code, command.Result().Body())
	var replayed canonicalRun
	require.NoError(t, json.Unmarshal(command.Result().Body(), &replayed))
	require.Equal(t, first.RunID, replayed.RunID)

	turn := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs",
		`{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"unrelated turn"}]}}`,
		header,
	)
	require.Equal(t, http.StatusConflict, turn.Code, turn.Result().Body())
	require.Contains(t, string(turn.Result().Body()), `"code":"idempotency_conflict"`)
	require.Len(t, canonicalRunsForThread(t, 1), 2)
}

func TestCanonicalResumeMapsClientSemanticErrorsToUnprocessableEntity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*canonicalResumeRunRequest)
	}{
		{name: "schema", mutate: func(req *canonicalResumeRunRequest) {
			req.Response.Schema = "unsupported.schema"
		}},
		{name: "kind", mutate: func(req *canonicalResumeRunRequest) {
			req.Response.Kind = "unknown"
		}},
		{name: "interaction mismatch", mutate: func(req *canonicalResumeRunRequest) {
			req.Response.InteractionID = "other-interaction"
		}},
		{name: "oversized answer", mutate: func(req *canonicalResumeRunRequest) {
			req.Response.Answer = strings.Repeat("x", 9<<10)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			installAgentThreadTestService(t)
			sourceRunID := createInterruptedHumanInteractionRun(t)
			h := canonicalRunTestServer()
			request := canonicalResumeRunRequest{
				InterruptID: "interrupt-1",
				Response: canonicalResumeResponse{
					Schema: "coze.human_interaction_response.v1", InteractionID: "hi_1",
					Kind: "clarification", Decision: "answered", Answer: "last 7 days",
				},
			}
			test.mutate(&request)
			body, err := json.Marshal(request)
			require.NoError(t, err)

			response := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/1/runs/%d/resume", sourceRunID),
				string(body),
			)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			require.Contains(t, string(response.Result().Body()), `"code":"invalid_resume"`)
			require.Len(t, canonicalRunsForThread(t, 1), 1)
		})
	}
}

func TestCanonicalRunEventsUseCursorFilterAndSafeProjection(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical run events", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "event source")
	for _, event := range []struct {
		eventType string
		payload   string
	}{
		{eventType: "step.started", payload: `{"step":"plan"}`},
		{eventType: "tool.completed", payload: `{"tool":"search","provider_body":"secret upstream body"}`},
		{eventType: "tool.completed", payload: `{"tool":"summarize"}`},
	} {
		_, err := appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
			ThreadID:  thread.ThreadID,
			RunID:     run.RunID,
			EventType: event.eventType,
			Payload:   event.payload,
		})
		require.NoError(t, err)
	}
	events, err := appagentthread.SVC.ListRunEvents(context.Background(), &appagentthread.ListRunEventsRequest{
		ThreadID: thread.ThreadID,
		RunID:    run.RunID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Len(t, events.Events, 3)
	h := canonicalRunTestServer()

	response := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf(
			"/api/workbench/threads/%d/runs/%d/events?after_event_id=%d&event_types=tool.completed&limit=1",
			thread.ThreadID,
			run.RunID,
			events.Events[0].EventID,
		),
		nil,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var page struct {
		Data             []*canonicalRunEvent `json:"data"`
		HasMore          bool                 `json:"has_more"`
		NextAfterEventID *string              `json:"next_after_event_id"`
	}
	require.NoError(t, json.Unmarshal(response.Result().Body(), &page))
	require.Len(t, page.Data, 1)
	require.Equal(t, "tool.completed", page.Data[0].EventType)
	require.True(t, page.HasMore)
	require.Equal(t, page.Data[0].EventID, *page.NextAfterEventID)
	require.NotContains(t, string(response.Result().Body()), "secret upstream body")
	require.NotContains(t, string(response.Result().Body()), "provider_body")
}

func TestCanonicalRunMessagesPreserveThreadGlobalSequence(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical run messages", `{}`)
	firstRun := createCanonicalRunFixture(t, thread.ThreadID, "first turn")
	cancelCanonicalRunFixture(t, firstRun)
	secondRun := createCanonicalRunFixture(t, thread.ThreadID, "second turn")
	h := canonicalRunTestServer()

	first := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/messages", thread.ThreadID, firstRun.RunID),
		nil,
	)
	require.Equal(t, http.StatusOK, first.Code, first.Result().Body())
	var firstPage canonicalMessagePage
	require.NoError(t, json.Unmarshal(first.Result().Body(), &firstPage))
	require.Len(t, firstPage.Data, 1)
	require.Equal(t, "1", firstPage.Data[0].Seq)
	require.Equal(t, "first turn", firstPage.Data[0].Content)

	second := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/messages?after_seq=1", thread.ThreadID, secondRun.RunID),
		nil,
	)
	require.Equal(t, http.StatusOK, second.Code, second.Result().Body())
	var secondPage canonicalMessagePage
	require.NoError(t, json.Unmarshal(second.Result().Body(), &secondPage))
	require.Len(t, secondPage.Data, 1)
	require.Equal(t, "2", secondPage.Data[0].Seq)
	require.Equal(t, "second turn", secondPage.Data[0].Content)
}

func TestCanonicalRunMessagesEnforceJournalBudgets(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical bounded journal", `{}`)
	createCanonicalRunFixture(t, thread.ThreadID, "bounded message")

	messages, err := loadCanonicalThreadMessagesWithBudget(
		context.Background(),
		thread.ThreadID,
		2,
		canonicalMaxJournalSourceBytes,
	)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	_, err = loadCanonicalThreadMessagesWithBudget(
		context.Background(),
		thread.ThreadID,
		1,
		canonicalMaxJournalSourceBytes,
	)
	require.ErrorIs(t, err, errCanonicalJournalBudgetExceeded)

	_, err = loadCanonicalThreadMessagesWithBudget(
		context.Background(),
		thread.ThreadID,
		canonicalMaxJournalRecords,
		1,
	)
	require.ErrorIs(t, err, errCanonicalJournalBudgetExceeded)
}

func TestCanonicalRunRejectsSensitiveConfigAndContextBeforePersistence(t *testing.T) {
	tests := map[string]string{
		"nested credential key":   `,"config":{"runtime":"eino_adk","provider":{"api_key":"not-persisted"}}`,
		"nested provider payload": `,"context":{"request":{"provider_body":"not-persisted"}}`,
		"sensitive string value":  `,"context":{"note":"access_token=not-persisted"}`,
		"identity field":          `,"context":{"user_id":"not-persisted"}`,
		"refresh token field":     `,"config":{"provider":{"refresh_token":"not-persisted"}}`,
	}
	for name, extra := range tests {
		name, extra := name, extra
		t.Run(name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "canonical input protection", `{}`)
			h := canonicalRunTestServer()
			body := fmt.Sprintf(`{
				"assistant_id":"agent",
				"input":{"messages":[{"role":"user","content":"normal user turn"}]}
				%s
			}`, extra)

			response := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
				body,
			)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			require.NotContains(t, string(response.Result().Body()), "not-persisted")
			require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
		})
	}
}

func TestCanonicalRunRejectsServerOwnedMetadataBeforePersistence(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
	}{
		{name: "message relation", metadata: `{"appended_message_id":"999"}`},
		{name: "source relation", metadata: `{"source_run_id":"998"}`},
		{name: "attempt kind", metadata: `{"attempt_kind":"resume"}`},
		{name: "human interaction", metadata: `{"human_interaction":"forged"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "canonical protected metadata", `{}`)
			h := canonicalRunTestServer()
			body := fmt.Sprintf(`{
				"assistant_id":"agent",
				"input":{"messages":[{"role":"user","content":"do not persist"}]},
				"metadata":%s
			}`, test.metadata)

			response := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
				body,
			)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
		})
	}
}

func TestCanonicalRunDoesNotTreatUserMessageAsRuntimeConfiguration(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical message semantics", `{}`)
	h := canonicalRunTestServer()
	body := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"请说明为什么 api_key=sk-example 不应写入配置"}]}
	}`

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
		body,
	)
	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 1)
}

func TestCanonicalWaitRaiseErrorCompatibility(t *testing.T) {
	for _, test := range []struct {
		name       string
		raiseError bool
		wantStatus int
	}{
		{name: "values projection", raiseError: false, wantStatus: http.StatusOK},
		{name: "http error projection", raiseError: true, wantStatus: http.StatusInternalServerError},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "canonical failed wait", `{}`)
			h := canonicalRunTestServer()
			createBody := `{
				"assistant_id":"agent",
				"input":{"messages":[{"role":"user","content":"fail this wait"}]}
			}`
			idempotencyKey := "canonical-failed-wait"
			created := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
				createBody,
				ut.Header{Key: "Idempotency-Key", Value: idempotencyKey},
			)
			require.Equal(t, http.StatusOK, created.Code, created.Result().Body())
			runs := canonicalRunsForThread(t, thread.ThreadID)
			require.Len(t, runs, 1)
			failCanonicalRunFixture(t, runs[0], "provider_http_500", "secret provider response")
			waitBody := fmt.Sprintf(`{
				"assistant_id":"agent",
				"input":{"messages":[{"role":"user","content":"fail this wait"}]},
				"raise_error":%t
			}`, test.raiseError)

			response := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs/wait", thread.ThreadID),
				waitBody,
				ut.Header{Key: "Idempotency-Key", Value: idempotencyKey},
			)
			require.Equal(t, test.wantStatus, response.Code, response.Result().Body())
			require.NotContains(t, string(response.Result().Body()), "secret provider response")
			if test.raiseError {
				var public canonicalError
				require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
				require.Equal(t, "provider_http_500", public.Code)
				require.Equal(t, "Model request failed", public.Detail)
			} else {
				var values map[string]any
				require.NoError(t, json.Unmarshal(response.Result().Body(), &values))
				public := values["__error__"].(map[string]any)
				require.Equal(t, "provider_http_500", public["error"])
				require.Equal(t, "Model request failed", public["message"])
			}
			require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 1)
		})
	}
}

func TestCanonicalWaitRejectsNonBooleanRaiseErrorBeforeMutation(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical invalid raise error", `{}`)
	h := canonicalRunTestServer()
	body := `{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"do not create"}]},
		"raise_error":"true"
	}`

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/wait", thread.ThreadID),
		body,
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
	require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
}

func TestCanonicalRunRejectsSensitiveUploadedFileDescriptor(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical unsafe upload descriptor", `{}`)
	h := canonicalRunTestServer()
	body := `{
		"assistant_id":"agent",
		"input":{
			"messages":[{"role":"user","content":"inspect attachment"}],
			"uploaded_files":[{"file_id":"5001","provider_body":"not-persisted"}]
		}
	}`

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
		body,
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
	require.NotContains(t, string(response.Result().Body()), "not-persisted")
	require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
}

func TestCanonicalRunNormalizesSDKUploadedFileIDForApplication(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical upload descriptor", `{}`)
	registered, err := appagentthread.SVC.UploadFileSVC.RegisterUploadFile(
		context.Background(),
		&domainservice.RegisterUploadFileRequest{
			SpaceID: thread.SpaceID, UserID: 2, ThreadID: thread.ThreadID,
			FileName: "report.txt", ContentType: "text/plain", SizeBytes: 12,
			Digest: strings.Repeat("0", 64), Metadata: `{}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, registered)
	h := canonicalRunTestServer()
	body := fmt.Sprintf(`{
		"assistant_id":"agent",
		"input":{
			"messages":[{"role":"user","content":"inspect attachment"}],
			"uploaded_files":[{"file_id":"%d"}]
		}
	}`, registered.ID)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
		body,
	)
	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	runs := canonicalRunsForThread(t, thread.ThreadID)
	require.Len(t, runs, 1)
	require.Contains(t, runs[0].Input, fmt.Sprintf(`"file_id":%d`, registered.ID))
	require.NotContains(t, runs[0].Input, fmt.Sprintf(`"file_id":"%d"`, registered.ID))
	require.Contains(t, runs[0].Input, `"virtual_path":"/mnt/user-data/uploads/report.txt"`)
}

func TestCanonicalRunReplaysIdempotentUploadAfterFileDeletion(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical upload replay", `{}`)
	registered, err := appagentthread.SVC.UploadFileSVC.RegisterUploadFile(
		context.Background(),
		&domainservice.RegisterUploadFileRequest{
			SpaceID: thread.SpaceID, UserID: 2, ThreadID: thread.ThreadID,
			FileName: "replay.txt", ContentType: "text/plain", SizeBytes: 12,
			Digest: strings.Repeat("1", 64), Metadata: `{}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, registered)
	h := canonicalRunTestServer()
	body := fmt.Sprintf(`{
		"assistant_id":"agent",
		"input":{
			"messages":[{"role":"user","content":"inspect replay attachment"}],
			"uploaded_files":[{"file_id":"%d"}]
		}
	}`, registered.ID)
	path := fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID)
	header := ut.Header{Key: "Idempotency-Key", Value: "canonical-upload-replay-1"}

	first := performCanonicalRunJSONRequest(t, h, http.MethodPost, path, body, header)
	require.Equal(t, http.StatusOK, first.Code, first.Result().Body())
	var created canonicalRun
	require.NoError(t, json.Unmarshal(first.Result().Body(), &created))

	_, deleted, err := appagentthread.SVC.UploadFileSVC.DeleteUploadFile(
		context.Background(),
		&domainservice.DeleteUploadFileRequest{
			SpaceID: thread.SpaceID, UserID: 2, ThreadID: thread.ThreadID,
			FileName: "replay.txt",
		},
	)
	require.NoError(t, err)
	require.True(t, deleted)

	replayed := performCanonicalRunJSONRequest(t, h, http.MethodPost, path, body, header)
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Result().Body())
	var got canonicalRun
	require.NoError(t, json.Unmarshal(replayed.Result().Body(), &got))
	require.Equal(t, created.RunID, got.RunID)
	require.Equal(t, created.Coze.MessageID, got.Coze.MessageID)
	require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 1)
}

func TestCanonicalRunRejectsChangedPayloadForIdempotencyKey(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical idempotency payload", `{}`)
	h := canonicalRunTestServer()
	path := fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID)
	header := ut.Header{Key: "Idempotency-Key", Value: "canonical-payload-conflict-1"}

	first := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		path,
		`{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"first payload"}]}}`,
		header,
	)
	require.Equal(t, http.StatusOK, first.Code, first.Result().Body())

	conflict := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		path,
		`{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"changed payload"}]}}`,
		header,
	)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Result().Body())
	require.Contains(t, string(conflict.Result().Body()), `"code":"idempotency_conflict"`)
	require.NotContains(t, string(conflict.Result().Body()), "first payload")
	require.NotContains(t, string(conflict.Result().Body()), "changed payload")
	require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 1)
}

func TestCanonicalRunRejectsUploadOwnedByAnotherThread(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	requestThread := createCanonicalTestThread(t, 1001, "canonical upload request", `{}`)
	ownerThread := createCanonicalTestThread(t, 1001, "canonical upload owner", `{}`)
	registered, err := appagentthread.SVC.UploadFileSVC.RegisterUploadFile(
		context.Background(),
		&domainservice.RegisterUploadFileRequest{
			SpaceID: ownerThread.SpaceID, UserID: 2, ThreadID: ownerThread.ThreadID,
			FileName: "other-thread.txt", ContentType: "text/plain", SizeBytes: 8,
			Digest: strings.Repeat("2", 64), Metadata: `{}`,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, registered)
	h := canonicalRunTestServer()
	body := fmt.Sprintf(`{
		"assistant_id":"agent",
		"input":{
			"messages":[{"role":"user","content":"inspect foreign attachment"}],
			"uploaded_files":[{"file_id":"%d"}]
		}
	}`, registered.ID)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", requestThread.ThreadID),
		body,
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
	require.Empty(t, canonicalRunsForThread(t, requestThread.ThreadID))
}

func TestCanonicalRunRejectsOversizedPayloadsBeforeMutation(t *testing.T) {
	tests := []struct {
		name       string
		body       func() string
		wantStatus int
	}{
		{
			name: "request body",
			body: func() string {
				return fmt.Sprintf(`{"assistant_id":"agent","input":{"messages":[{"role":"user","content":%q}]}}`, strings.Repeat("x", 1<<20))
			},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "persisted config string",
			body: func() string {
				return fmt.Sprintf(`{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"bounded"}]},"config":{"note":%q}}`, strings.Repeat("x", 64<<10))
			},
			wantStatus: http.StatusUnprocessableEntity,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(canonicalAPIEnabledEnv, "true")
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "canonical bounded payload", `{}`)
			h := canonicalRunTestServer()

			response := performCanonicalRunJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
				test.body(),
			)
			require.Equal(t, test.wantStatus, response.Code, response.Result().Body())
			require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
		})
	}
}

func TestCanonicalRunRejectsUnknownAssistantAliasBeforeMutation(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "canonical assistant alias", `{}`)
	h := canonicalRunTestServer()
	body := `{
		"assistant_id":"unconfigured-agent",
		"input":{"messages":[{"role":"user","content":"do not create"}]}
	}`

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
		body,
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
	require.Empty(t, canonicalRunsForThread(t, thread.ThreadID))
}

func TestCanonicalRunHandlersFailClosedWhenApplicationServiceIsUnavailable(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	h := canonicalRunTestServer()
	previous := appagentthread.SVC
	appagentthread.SVC = nil
	t.Cleanup(func() { appagentthread.SVC = previous })

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/workbench/threads/1/runs"},
		{method: http.MethodPost, path: "/api/workbench/threads/1/runs", body: `{}`},
		{method: http.MethodPost, path: "/api/workbench/threads/1/runs/wait", body: `{}`},
		{method: http.MethodGet, path: "/api/workbench/threads/1/runs/2"},
		{method: http.MethodGet, path: "/api/workbench/threads/1/runs/2/join"},
		{method: http.MethodPost, path: "/api/workbench/threads/1/runs/2/cancel"},
		{method: http.MethodPost, path: "/api/workbench/threads/1/runs/2/resume", body: `{}`},
		{method: http.MethodGet, path: "/api/workbench/threads/1/runs/2/events"},
		{method: http.MethodGet, path: "/api/workbench/threads/1/runs/2/messages"},
	}
	for _, test := range tests {
		var response *ut.ResponseRecorder
		if test.body == "" {
			response = ut.PerformRequest(h.Engine, test.method, test.path, nil)
		} else {
			response = performCanonicalRunJSONRequest(t, h, test.method, test.path, test.body)
		}
		require.Equal(t, http.StatusServiceUnavailable, response.Code, test.path)
		var public canonicalError
		require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
		require.Equal(t, "dependency_unavailable", public.Code)
		require.True(t, public.Retryable)
	}
}

func canonicalRunTestServer() *server.Hertz {
	return canonicalRunTestServerForUser(2)
}

func canonicalRunTestServerForUser(userID int64) *server.Hertz {
	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(userID))
	h.GET("/api/workbench/threads/:thread_id/runs", ListCanonicalRuns)
	h.POST("/api/workbench/threads/:thread_id/runs", CreateCanonicalRun)
	h.POST("/api/workbench/threads/:thread_id/runs/wait", WaitCanonicalRun)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id", GetCanonicalRun)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/join", JoinCanonicalRun)
	h.POST("/api/workbench/threads/:thread_id/runs/:run_id/cancel", CancelCanonicalRun)
	h.POST("/api/workbench/threads/:thread_id/runs/:run_id/resume", ResumeCanonicalRun)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/events", ListCanonicalRunEvents)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/messages", ListCanonicalRunMessages)
	return h
}

func performCanonicalRunJSONRequest(
	t *testing.T,
	h *server.Hertz,
	method string,
	path string,
	body string,
	headers ...ut.Header,
) *ut.ResponseRecorder {
	t.Helper()
	requestHeaders := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
	requestHeaders = append(requestHeaders, headers...)
	return ut.PerformRequest(
		h.Engine,
		method,
		path,
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		requestHeaders...,
	)
}

func canonicalRunsForThread(t *testing.T, threadID int64) []*appagentthread.RunSummary {
	t.Helper()
	response, err := appagentthread.SVC.SearchRuns(context.Background(), &appagentthread.SearchRunsRequest{
		ThreadID: threadID,
		Page:     appagentthread.CanonicalPage{Limit: 100},
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	return response.Runs
}

func createCanonicalRunFixture(
	t *testing.T,
	threadID int64,
	message string,
) *appagentthread.RunSummary {
	t.Helper()
	response, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: threadID, AssistantID: "agent", Input: `{"uploaded_files":[]}`,
		MessageContent: message, StreamMode: `["values"]`, MultitaskStrategy: "reject",
		OnDisconnect: "cancel", Durability: "async",
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Run)
	return response.Run
}

func cancelCanonicalRunFixture(t *testing.T, run *appagentthread.RunSummary) {
	t.Helper()
	require.NotNil(t, run)
	response, err := appagentthread.SVC.CancelRun(context.Background(), &appagentthread.UpdateRunStatusRequest{
		RunID: run.RunID,
		From:  run.Status,
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Run)
}

func completeCanonicalRunWithPublicState(
	t *testing.T,
	run *appagentthread.RunSummary,
	values string,
) {
	t.Helper()
	require.NotNil(t, run)
	_, err := appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "canonical-test-worker",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        run.ThreadID,
		RunID:           run.RunID,
		CheckpointNS:    "canonical.public",
		RuntimeType:     "canonical_public_state",
		RuntimeKey:      fmt.Sprintf("canonical-test-state-%d", run.RunID),
		ChannelValues:   values,
		ChannelVersions: `{}`,
		PendingSends:    `[]`,
		Metadata:        `{}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CompleteRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "canonical-test-worker",
	}))
	require.NoError(t, err)
}

func failCanonicalRunFixture(
	t *testing.T,
	run *appagentthread.RunSummary,
	errorCode, errorMessage string,
) {
	t.Helper()
	require.NotNil(t, run)
	_, err := appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "canonical-test-worker",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.FailRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:        run.RunID,
		From:         appagentthread.RunStatusRunning,
		WorkerID:     "canonical-test-worker",
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	}))
	require.NoError(t, err)
}

func assertCanonicalResumePersistence(t *testing.T, sourceRunID int64, idempotencyKey string) {
	t.Helper()
	response, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Len(t, response.Runs, 2)
	resumed := response.Runs[0]
	require.Equal(t, appagentthread.RunStatusQueued, resumed.Status)
	require.Equal(t, canonicalScopedIdempotencyKey(2, idempotencyKey), resumed.IdempotencyKey)
	require.Contains(t, resumed.Command, `"interrupt-1"`)
	require.NotEqual(t, sourceRunID, resumed.RunID)
}
