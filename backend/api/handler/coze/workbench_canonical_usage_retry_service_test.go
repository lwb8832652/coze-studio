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

func TestCanonicalThreadTokenUsageReadsAreSafe(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "usage reads", `{}`)
	parent := createCanonicalRunForUsageRetry(t, thread.ThreadID, 0, appagentthread.RunKindTask, appagentthread.RunStatusPending)
	completeCanonicalRunForUsageRetry(t, parent.RunID)
	child := createCanonicalRunForUsageRetry(t, thread.ThreadID, parent.RunID, appagentthread.RunKindSubagent, appagentthread.RunStatusRunning)
	sibling := createCanonicalRunForUsageRetry(t, thread.ThreadID, 0, appagentthread.RunKindTask, appagentthread.RunStatusPending)
	completeCanonicalRunForUsageRetry(t, sibling.RunID)
	recordCanonicalTokenUsage(t, parent.RunID, appagentthread.TokenUsageSourceLeadAgent, "lead", 12, 8, 20)
	recordCanonicalTokenUsage(t, child.RunID, appagentthread.TokenUsageSourceSubagent, "child", 4, 6, 10)
	recordCanonicalTokenUsage(t, sibling.RunID, appagentthread.TokenUsageSourceTool, "tool", 40, 59, 99)
	h := canonicalUsageRetryTestServer()

	threadUsage := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/token_usage?limit=1&offset=0", thread.ThreadID),
		nil,
	)
	threadUsageBody := string(threadUsage.Result().Body())
	require.Equal(t, http.StatusOK, threadUsage.Code, threadUsageBody)
	require.Contains(t, threadUsageBody, `"total":3`)
	require.Contains(t, threadUsageBody, `"has_more":true`)
	require.Contains(t, threadUsageBody, `"next_cursor":"1"`)
	require.Contains(t, threadUsageBody, `"thread_id":"`+strconv.FormatInt(thread.ThreadID, 10)+`"`)
	require.Contains(t, threadUsageBody, `"input_tokens":56`)
	require.Contains(t, threadUsageBody, `"output_tokens":73`)
	require.Contains(t, threadUsageBody, `"total_tokens":129`)
	require.Contains(t, threadUsageBody, `"call_count":3`)
	require.Contains(t, threadUsageBody, `"lead_agent_tokens":20`)
	require.Contains(t, threadUsageBody, `"subagent_tokens":10`)
	require.Contains(t, threadUsageBody, `"tool_tokens":99`)
	require.NotContains(t, threadUsageBody, `"code"`)
	require.NotContains(t, threadUsageBody, "raw_usage")
	require.NotContains(t, threadUsageBody, "prompt_tokens")
	require.NotContains(t, threadUsageBody, "completion_tokens")
	require.NotContains(t, threadUsageBody, "provider_raw")
	require.NotContains(t, threadUsageBody, "sk-secret")
	require.NotContains(t, threadUsageBody, "tool_args")
	require.NotContains(t, threadUsageBody, "s3://bucket/raw")

	runUsage := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/token_usage?run_id=%d&include_child_runs=true&limit=10&offset=0", thread.ThreadID, parent.RunID),
		nil,
	)
	runUsageBody := string(runUsage.Result().Body())
	require.Equal(t, http.StatusOK, runUsage.Code, runUsageBody)
	require.Contains(t, runUsageBody, `"total":2`)
	require.Contains(t, runUsageBody, `"run_id":"`+strconv.FormatInt(parent.RunID, 10)+`"`)
	require.Contains(t, runUsageBody, `"run_id":"`+strconv.FormatInt(child.RunID, 10)+`"`)
	require.NotContains(t, runUsageBody, `"run_id":"`+strconv.FormatInt(sibling.RunID, 10)+`"`)
	require.Contains(t, runUsageBody, `"total_tokens":30`)
	require.Contains(t, runUsageBody, `"run_aggregates":[`)
	require.Contains(t, runUsageBody, `"subagent_tokens":10`)

	sourceUsage := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/token_usage?source=tool&limit=10&offset=0", thread.ThreadID),
		nil,
	)
	sourceUsageBody := string(sourceUsage.Result().Body())
	require.Equal(t, http.StatusOK, sourceUsage.Code, sourceUsageBody)
	require.Contains(t, sourceUsageBody, `"total":1`)
	require.Contains(t, sourceUsageBody, `"source":"tool"`)
	require.NotContains(t, sourceUsageBody, `"source":"lead_agent"`)
}

func TestCanonicalThreadTokenUsageErrors(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "usage errors", `{}`)
	other := createCanonicalTestThread(t, 1001, "usage other", `{}`)
	otherRun := createCanonicalRunForUsageRetry(t, other.ThreadID, 0, appagentthread.RunKindTask, appagentthread.RunStatusQueued)
	empty := createCanonicalTestThread(t, 1001, "usage empty", `{}`)
	h := canonicalUsageRetryTestServer()

	wrongRun := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/token_usage?run_id=%d", thread.ThreadID, otherRun.RunID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, wrongRun.Code)
	require.Contains(t, string(wrongRun.Result().Body()), `"code":"invalid_request"`)

	invalidBool := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/token_usage?include_child_runs=1", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, invalidBool.Code)
	require.Contains(t, string(invalidBool.Result().Body()), `"code":"invalid_query_parameter"`)

	wrongWorkspace := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/token_usage", thread.ThreadID),
		nil,
		ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
	)
	require.Equal(t, http.StatusNotFound, wrongWorkspace.Code)

	emptyUsage := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/token_usage", empty.ThreadID),
		nil,
	)
	emptyUsageBody := string(emptyUsage.Result().Body())
	require.Equal(t, http.StatusOK, emptyUsage.Code, emptyUsageBody)
	require.Contains(t, emptyUsageBody, `"usage":[]`)
	require.Contains(t, emptyUsageBody, `"total":0`)
	require.Contains(t, emptyUsageBody, `"aggregate":{"input_tokens":0`)
}

func TestCanonicalSubagentRunRetryContract(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "retry contract", `{}`)
	parent := createCanonicalRunForUsageRetry(t, thread.ThreadID, 0, appagentthread.RunKindTask, appagentthread.RunStatusQueued)
	child := createCanonicalRunForUsageRetry(t, thread.ThreadID, parent.RunID, appagentthread.RunKindSubagent, appagentthread.RunStatusRunning)
	failCanonicalRunForUsageRetry(t, child.RunID)
	h := canonicalUsageRetryTestServer()
	key := "canonical-retry-child"

	retry := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, child.RunID),
		nil,
		ut.Header{Key: "Idempotency-Key", Value: key},
	)
	retryBody := string(retry.Result().Body())
	require.Equal(t, http.StatusOK, retry.Code, retryBody)
	require.Contains(t, retryBody, `"thread_id":"`+strconv.FormatInt(thread.ThreadID, 10)+`"`)
	require.Contains(t, retryBody, `"status":"pending"`)
	require.Contains(t, retryBody, `"run_kind":"task"`)
	require.Contains(t, retryBody, `"source_run_id":"`+strconv.FormatInt(child.RunID, 10)+`"`)
	require.NotContains(t, retryBody, `"code"`)
	require.NotContains(t, retryBody, "idempotency")
	require.NotContains(t, retryBody, key)

	runs := canonicalRunsForThread(t, thread.ThreadID)
	retryRun := requireCanonicalRetryRun(t, runs, child.RunID)
	require.Equal(t, canonicalScopedIdempotencyKey(2, key), retryRun.IdempotencyKey)
	require.Contains(t, retryRun.Command, `"source_run_id":`+strconv.FormatInt(child.RunID, 10))

	replayed := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, child.RunID),
		nil,
		ut.Header{Key: "Idempotency-Key", Value: key},
	)
	replayedBody := string(replayed.Result().Body())
	require.Equal(t, http.StatusOK, replayed.Code, replayedBody)
	require.Contains(t, replayedBody, `"run_id":"`+strconv.FormatInt(retryRun.RunID, 10)+`"`)
	require.Len(t, canonicalRetryRunsForSource(t, thread.ThreadID, child.RunID), 1)
}

func TestCanonicalSubagentRunRetryRejectsExecutionControlsBeforeApplication(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "retry ingress rejection", `{}`)
	parent := createCanonicalRunForUsageRetry(
		t, thread.ThreadID, 0, appagentthread.RunKindTask, appagentthread.RunStatusQueued,
	)
	child := createCanonicalRunForUsageRetry(
		t, thread.ThreadID, parent.RunID, appagentthread.RunKindSubagent, appagentthread.RunStatusRunning,
	)
	failCanonicalRunForUsageRetry(t, child.RunID)

	response := performCanonicalUsageRetryRequest(
		t,
		canonicalUsageRetryTestServer(),
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, child.RunID),
		canonicalUsageRetryJSONBody(`{"subagent_enabled":true}`),
	)

	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
	require.Contains(t, string(response.Result().Body()), `"code":"unsupported_execution_control"`)
	require.Empty(t, canonicalRetryRunsForSource(t, thread.ThreadID, child.RunID))
}

func TestCanonicalSubagentRunRetryPreservesIngressPriorities(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "retry ingress priority", `{}`)
	parent := createCanonicalRunForUsageRetry(
		t, thread.ThreadID, 0, appagentthread.RunKindTask, appagentthread.RunStatusQueued,
	)
	child := createCanonicalRunForUsageRetry(
		t, thread.ThreadID, parent.RunID, appagentthread.RunKindSubagent, appagentthread.RunStatusRunning,
	)
	failCanonicalRunForUsageRetry(t, child.RunID)
	h := canonicalUsageRetryTestServer()

	wrongWorkspace := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, child.RunID),
		canonicalUsageRetryJSONBody(`{"mode":"ultra"}`),
		ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
	)
	require.Equal(t, http.StatusNotFound, wrongWorkspace.Code, wrongWorkspace.Result().Body())
	require.Contains(t, string(wrongWorkspace.Result().Body()), `"code":"resource_not_found"`)

	invalidPath := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/not-a-run/retry", thread.ThreadID),
		canonicalUsageRetryJSONBody(`{"mode":"ultra"}`),
	)
	require.Equal(t, http.StatusBadRequest, invalidPath.Code, invalidPath.Result().Body())
	require.NotContains(t, string(invalidPath.Result().Body()), "unsupported_execution_control")

	oversized := `{"payload":"` + strings.Repeat("x", canonicalMaxRequestBytes) + `"}`
	tooLarge := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, child.RunID),
		canonicalUsageRetryJSONBody(oversized),
	)
	require.Equal(t, http.StatusRequestEntityTooLarge, tooLarge.Code, tooLarge.Result().Body())
	require.Contains(t, string(tooLarge.Result().Body()), `"code":"request_too_large"`)
	require.Empty(t, canonicalRetryRunsForSource(t, thread.ThreadID, child.RunID))
}

func TestCanonicalSubagentRunRetryErrors(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "retry errors", `{}`)
	parent := createCanonicalRunForUsageRetry(t, thread.ThreadID, 0, appagentthread.RunKindTask, appagentthread.RunStatusQueued)
	failedChild := createCanonicalRunForUsageRetry(t, thread.ThreadID, parent.RunID, appagentthread.RunKindSubagent, appagentthread.RunStatusRunning)
	failCanonicalRunForUsageRetry(t, failedChild.RunID)
	runningChild := createCanonicalRunForUsageRetry(t, thread.ThreadID, parent.RunID, appagentthread.RunKindSubagent, appagentthread.RunStatusRunning)
	otherChild := createCanonicalRunForUsageRetry(t, thread.ThreadID, parent.RunID, appagentthread.RunKindSubagent, appagentthread.RunStatusRunning)
	failCanonicalRunForUsageRetry(t, otherChild.RunID)
	h := canonicalUsageRetryTestServer()

	topLevel := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, parent.RunID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, topLevel.Code)
	require.Contains(t, string(topLevel.Result().Body()), `"code":"invalid_request"`)

	running := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, runningChild.RunID),
		nil,
	)
	require.Equal(t, http.StatusConflict, running.Code)
	require.Contains(t, string(running.Result().Body()), `"code":"run_conflict"`)

	wrongWorkspace := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, failedChild.RunID),
		nil,
		ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
	)
	require.Equal(t, http.StatusNotFound, wrongWorkspace.Code)

	otherThread := createCanonicalTestThread(t, 1001, "retry cross thread", `{}`)
	otherParent := createCanonicalRunForUsageRetry(t, otherThread.ThreadID, 0, appagentthread.RunKindTask, appagentthread.RunStatusQueued)
	otherThreadChild := createCanonicalRunForUsageRetry(t, otherThread.ThreadID, otherParent.RunID, appagentthread.RunKindSubagent, appagentthread.RunStatusRunning)
	failCanonicalRunForUsageRetry(t, otherThreadChild.RunID)
	crossThread := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, otherThreadChild.RunID),
		nil,
	)
	require.Equal(t, http.StatusNotFound, crossThread.Code)
	require.Contains(t, string(crossThread.Result().Body()), `"code":"resource_not_found"`)
	require.Empty(t, canonicalRetryRunsForSource(t, otherThread.ThreadID, otherThreadChild.RunID))

	bodyOnlyKey := "canonical-retry-body-only"
	bodyRejected := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, failedChild.RunID),
		canonicalUsageRetryJSONBody(`{"idempotency_key":"`+bodyOnlyKey+`"}`),
	)
	require.Equal(t, http.StatusUnprocessableEntity, bodyRejected.Code)
	require.Contains(t, string(bodyRejected.Result().Body()), `"code":"invalid_request"`)
	require.Empty(t, canonicalRetryRunsForSource(t, thread.ThreadID, failedChild.RunID))

	foreignKey := "canonical-retry-foreign"
	foreignThread := createCanonicalTestThread(t, 1001, "retry foreign idempotency", `{}`)
	createCanonicalRunForUsageRetryWithMetadata(
		t,
		foreignThread.ThreadID,
		0,
		appagentthread.RunKindTask,
		appagentthread.RunStatusPending,
		canonicalScopedIdempotencyKey(2, foreignKey),
		fmt.Sprintf(`{"source_run_id":%d}`, failedChild.RunID),
	)
	foreignConflict := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, failedChild.RunID),
		nil,
		ut.Header{Key: "Idempotency-Key", Value: foreignKey},
	)
	require.Equal(t, http.StatusConflict, foreignConflict.Code)
	require.Contains(t, string(foreignConflict.Result().Body()), `"code":"idempotency_conflict"`)
	require.Empty(t, canonicalRetryRunsForSource(t, thread.ThreadID, failedChild.RunID))

	key := "canonical-retry-conflict"
	first := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, failedChild.RunID),
		nil,
		ut.Header{Key: "Idempotency-Key", Value: key},
	)
	require.Equal(t, http.StatusOK, first.Code, first.Result().Body())
	conflict := performCanonicalUsageRetryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs/%d/retry", thread.ThreadID, otherChild.RunID),
		nil,
		ut.Header{Key: "Idempotency-Key", Value: key},
	)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.Contains(t, string(conflict.Result().Body()), `"code":"idempotency_conflict"`)
	require.Len(t, canonicalRetryRunsForSource(t, thread.ThreadID, failedChild.RunID), 1)
	require.Empty(t, canonicalRetryRunsForSource(t, thread.ThreadID, otherChild.RunID))
}

func canonicalUsageRetryTestServer() *server.Hertz {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/threads/:thread_id/token_usage", GetCanonicalThreadTokenUsage)
	h.POST("/api/workbench/threads/:thread_id/runs/:run_id/retry", RetryCanonicalSubagentRun)
	return h
}

func performCanonicalUsageRetryRequest(
	t *testing.T,
	h *server.Hertz,
	method string,
	path string,
	body *ut.Body,
	headers ...ut.Header,
) *ut.ResponseRecorder {
	t.Helper()
	requestHeaders := make([]ut.Header, 0, len(headers)+1)
	hasSpaceHeader := false
	for _, header := range headers {
		if strings.EqualFold(header.Key, canonicalSpaceIDHeader) {
			hasSpaceHeader = true
			break
		}
	}
	if !hasSpaceHeader {
		requestHeaders = append(requestHeaders, ut.Header{Key: canonicalSpaceIDHeader, Value: "1001"})
	}
	requestHeaders = append(requestHeaders, headers...)
	return ut.PerformRequest(h.Engine, method, path, body, requestHeaders...)
}

func canonicalUsageRetryJSONBody(body string) *ut.Body {
	return &ut.Body{Body: strings.NewReader(body), Len: len(body)}
}

func createCanonicalRunForUsageRetry(
	t *testing.T,
	threadID int64,
	parentRunID int64,
	kind appagentthread.RunKind,
	status appagentthread.RunStatus,
) *appagentthread.RunSummary {
	t.Helper()
	return createCanonicalRunForUsageRetryWithMetadata(t, threadID, parentRunID, kind, status, "", `{"subagent":{"name":"researcher"}}`)
}

func createCanonicalRunForUsageRetryWithIdempotency(
	t *testing.T,
	threadID int64,
	parentRunID int64,
	kind appagentthread.RunKind,
	status appagentthread.RunStatus,
	idempotencyKey string,
) *appagentthread.RunSummary {
	return createCanonicalRunForUsageRetryWithMetadata(
		t,
		threadID,
		parentRunID,
		kind,
		status,
		idempotencyKey,
		`{"subagent":{"name":"researcher"}}`,
	)
}

func createCanonicalRunForUsageRetryWithMetadata(
	t *testing.T,
	threadID int64,
	parentRunID int64,
	kind appagentthread.RunKind,
	status appagentthread.RunStatus,
	idempotencyKey string,
	metadata string,
) *appagentthread.RunSummary {
	t.Helper()
	if kind == appagentthread.RunKindSubagent {
		require.Equal(t, appagentthread.RunStatusRunning, status)
		parentResponse, err := appagentthread.SVC.GetRun(
			context.Background(),
			&appagentthread.GetRunRequest{RunID: parentRunID},
		)
		require.NoError(t, err)
		require.NotNil(t, parentResponse)
		require.NotNil(t, parentResponse.Run)
		return createCanonicalServerOwnedSubagentFixture(t, parentResponse.Run)
	}
	assistantID := "lead-agent"
	resp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:       threadID,
		ParentRunID:    parentRunID,
		AssistantID:    assistantID,
		RunKind:        kind,
		Status:         status,
		Input:          `{"messages":[]}`,
		Config:         `{"runtime":"eino_adk"}`,
		Context:        `{"plan_scope_run_id":2}`,
		Metadata:       metadata,
		IdempotencyKey: idempotencyKey,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Run)
	return resp.Run
}

func createCanonicalServerOwnedSubagentFixture(
	t *testing.T,
	parent *appagentthread.RunSummary,
) *appagentthread.RunSummary {
	t.Helper()
	require.NotNil(t, parent)
	recorder := appagentthread.NewApplicationADKSubagentRunRecorder(appagentthread.SVC)
	child, err := recorder.StartADKSubagentRun(
		context.Background(),
		appagentthread.ADKSubagentRunStartRequest{
			Parent:          parent,
			ArgumentsInJSON: `{}`,
			Definition: appagentthread.ADKSubagentDefinition{
				Name:        "researcher",
				Description: "Research test fixture.",
				AgentID:     1001,
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, child)
	return child
}

func failCanonicalRunForUsageRetry(t *testing.T, runID int64) {
	t.Helper()
	_, err := appagentthread.SVC.FailRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:        runID,
		From:         appagentthread.RunStatusRunning,
		To:           appagentthread.RunStatusFailed,
		ErrorCode:    "subagent_timeout",
		ErrorMessage: "context deadline exceeded sk-secret",
	}))
	require.NoError(t, err)
}

func completeCanonicalRunForUsageRetry(t *testing.T, runID int64) {
	t.Helper()
	_, err := appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "canonical-usage-retry-test-worker",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CompleteRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "canonical-usage-retry-test-worker",
	}))
	require.NoError(t, err)
}

func recordCanonicalTokenUsage(
	t *testing.T,
	runID int64,
	source appagentthread.TokenUsageSource,
	stepName string,
	inputTokens int64,
	outputTokens int64,
	totalTokens int64,
) {
	t.Helper()
	_, err := appagentthread.SVC.RecordTokenUsage(context.Background(), &appagentthread.RecordTokenUsageRequest{
		RunID:        runID,
		Source:       source,
		StepName:     stepName,
		ModelName:    "gpt-4.1",
		Provider:     "openai",
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
		RawUsage:     `{"prompt_tokens":12,"completion_tokens":8}`,
		Metadata:     `{"provider_raw":"sk-secret","tool_args":"{\"url\":\"s3://bucket/raw\"}"}`,
	})
	require.NoError(t, err)
}

func requireCanonicalRetryRun(t *testing.T, runs []*appagentthread.RunSummary, sourceRunID int64) *appagentthread.RunSummary {
	t.Helper()
	matches := canonicalRetryRunsForSourceFromRuns(runs, sourceRunID)
	require.Len(t, matches, 1)
	return matches[0]
}

func canonicalRetryRunsForSource(t *testing.T, threadID int64, sourceRunID int64) []*appagentthread.RunSummary {
	t.Helper()
	return canonicalRetryRunsForSourceFromRuns(canonicalRunsForThread(t, threadID), sourceRunID)
}

func canonicalRetryRunsForSourceFromRuns(runs []*appagentthread.RunSummary, sourceRunID int64) []*appagentthread.RunSummary {
	marker := `"source_run_id":` + strconv.FormatInt(sourceRunID, 10)
	result := make([]*appagentthread.RunSummary, 0)
	for _, run := range runs {
		if run != nil && strings.Contains(run.Command, `"subagent_retry"`) && strings.Contains(run.Command, marker) {
			result = append(result, run)
		}
	}
	return result
}
