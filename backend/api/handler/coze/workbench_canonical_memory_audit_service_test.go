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

func TestCanonicalThreadMemoryReadsAndExportsAreSafe(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "memory reads", `{}`)
	first := createCanonicalMemoryFixture(t, thread.ThreadID, "用户偏好中文摘要", "source-a")
	second := createCanonicalMemoryFixture(t, thread.ThreadID, "偏好短句回复", "source-b")
	h := canonicalMemoryAuditTestServer()

	list := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/memories?limit=1&offset=0", thread.ThreadID),
		nil,
	)
	listBody := string(list.Result().Body())
	require.Equal(t, http.StatusOK, list.Code, listBody)
	require.Contains(t, listBody, `"total":2`)
	require.Contains(t, listBody, `"has_more":true`)
	require.Contains(t, listBody, `"next_cursor":"1"`)
	require.Contains(t, listBody, `"memory_id":"`+strconv.FormatInt(second.MemoryID, 10)+`"`)
	require.NotContains(t, listBody, `"memory_id":"`+strconv.FormatInt(first.MemoryID, 10)+`"`)
	require.Contains(t, listBody, `"thread_id":"`+strconv.FormatInt(thread.ThreadID, 10)+`"`)
	require.Contains(t, listBody, `"created_at":"`)
	require.NotContains(t, listBody, `"code"`)
	require.NotContains(t, listBody, "provider_body")
	require.NotContains(t, listBody, "authorization")
	require.NotContains(t, listBody, "secret-token")

	exported := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/memories/export?limit=50", thread.ThreadID),
		nil,
	)
	exportBody := string(exported.Result().Body())
	require.Equal(t, http.StatusOK, exported.Code, exportBody)
	require.Contains(t, exportBody, `"schema":"coze.task_thread_memories.export.v1"`)
	require.Contains(t, exportBody, `"thread_id":"`+strconv.FormatInt(thread.ThreadID, 10)+`"`)
	require.Contains(t, exportBody, `"total":2`)
	require.NotContains(t, exportBody, `"code"`)
	require.NotContains(t, exportBody, "provider_body")
	require.NotContains(t, exportBody, "secret-token")

	queryAlias := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/memories?query=%s", thread.ThreadID, "偏好短句回复"),
		nil,
	)
	queryAliasBody := string(queryAlias.Result().Body())
	require.Equal(t, http.StatusOK, queryAlias.Code, queryAliasBody)
	require.Contains(t, queryAliasBody, `"total":2`)

	audit := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/memories/audit_events?limit=10&offset=0", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusOK, audit.Code, audit.Result().Body())
	require.Contains(t, string(audit.Result().Body()), `"events":[]`)
}

func TestCanonicalThreadAuditReadsAndExportsAreSafe(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "audit reads", `{}`)
	recordCanonicalGuardrailAudit(t, thread.ThreadID, 9101, appagentthread.GuardrailActionDeny)
	recordCanonicalMCPRuntimeAudit(t, thread.ThreadID, 9201)
	h := canonicalMemoryAuditTestServer()

	guardrail := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/guardrail_audit_events?limit=10&offset=0", thread.ThreadID),
		nil,
	)
	guardrailBody := string(guardrail.Result().Body())
	require.Equal(t, http.StatusOK, guardrail.Code, guardrailBody)
	require.Contains(t, guardrailBody, `"total":1`)
	require.Contains(t, guardrailBody, `"event_id":"9101"`)
	require.Contains(t, guardrailBody, `"event_type":"guardrail.decision.deny"`)
	require.Contains(t, guardrailBody, `"target_type":"network"`)
	require.Contains(t, guardrailBody, `"rule_ids":["url_review","external_policy"]`)
	require.NotContains(t, guardrailBody, `"code"`)
	require.NotContains(t, guardrailBody, "secret prompt")
	require.NotContains(t, guardrailBody, "provider_raw")
	require.NotContains(t, guardrailBody, "tool_args")
	require.NotContains(t, guardrailBody, "sk-secret")
	require.NotContains(t, guardrailBody, "s3://")

	guardrailExport := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/guardrail_audit_events/export?limit=50", thread.ThreadID),
		nil,
	)
	guardrailExportBody := string(guardrailExport.Result().Body())
	require.Equal(t, http.StatusOK, guardrailExport.Code, guardrailExportBody)
	require.Contains(t, guardrailExportBody, `"schema":"coze.task_thread_guardrail_audit.export.v1"`)
	require.Contains(t, guardrailExportBody, `"thread_id":"`+strconv.FormatInt(thread.ThreadID, 10)+`"`)
	require.Contains(t, guardrailExportBody, `"total":1`)
	require.NotContains(t, guardrailExportBody, "provider_raw")
	require.NotContains(t, guardrailExportBody, "sk-secret")

	mcp := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/mcp_runtime_audit_events?limit=10&offset=0", thread.ThreadID),
		nil,
	)
	mcpBody := string(mcp.Result().Body())
	require.Equal(t, http.StatusOK, mcp.Code, mcpBody)
	require.Contains(t, mcpBody, `"total":1`)
	require.Contains(t, mcpBody, `"event_id":"9201"`)
	require.Contains(t, mcpBody, `"runtime_tool_name":"mcp_100_get_weather"`)
	require.Contains(t, mcpBody, `"elapsed_millis":33`)
	require.Contains(t, mcpBody, `"output_bytes":128`)
	require.NotContains(t, mcpBody, `"code"`)
	require.NotContains(t, mcpBody, "secret prompt")
	require.NotContains(t, mcpBody, "tool_args")
	require.NotContains(t, mcpBody, "sk-")

	guardrailWrongWorkspace := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/guardrail_audit_events", thread.ThreadID),
		nil,
		ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
	)
	require.Equal(t, http.StatusNotFound, guardrailWrongWorkspace.Code)

	mcpWrongWorkspace := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/mcp_runtime_audit_events", thread.ThreadID),
		nil,
		ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
	)
	require.Equal(t, http.StatusNotFound, mcpWrongWorkspace.Code)
}

func TestCanonicalThreadMemoryMutations(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "memory mutations", `{}`)
	memory := createCanonicalMemoryFixture(t, thread.ThreadID, "待更新记忆", "source-update")
	correction := createCanonicalMemoryFixture(t, thread.ThreadID, "被修正的记忆", "source-correction")
	h := canonicalMemoryAuditTestServer()

	updateBody := fmt.Sprintf(`{"run_id":"2","scope":"run","content":"更新后的记忆","metadata":{"origin":"manual"},"score":0.5,"confidence":0.8,"source_type":"manual","source_id":"source-updated","correction_of_memory_id":"%d"}`, correction.MemoryID)
	updated := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodPut,
		fmt.Sprintf("/api/workbench/threads/%d/memories/%d", thread.ThreadID, memory.MemoryID),
		canonicalMemoryJSONBody(updateBody),
	)
	updatedBody := string(updated.Result().Body())
	require.Equal(t, http.StatusOK, updated.Code, updatedBody)
	require.Contains(t, updatedBody, `"updated":true`)
	require.Contains(t, updatedBody, `"content":"更新后的记忆"`)
	require.Contains(t, updatedBody, `"run_id":"2"`)
	require.Contains(t, updatedBody, `"correction_of_memory_id":"`+strconv.FormatInt(correction.MemoryID, 10)+`"`)
	require.Contains(t, updatedBody, `"memory_id":"`+strconv.FormatInt(memory.MemoryID, 10)+`"`)

	deleted := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodDelete,
		fmt.Sprintf("/api/workbench/threads/%d/memories/%d", thread.ThreadID, memory.MemoryID),
		nil,
	)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Result().Body())
	require.Empty(t, deleted.Result().Body())

	restored := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/memories/%d/restore", thread.ThreadID, memory.MemoryID),
		nil,
	)
	restoredBody := string(restored.Result().Body())
	require.Equal(t, http.StatusOK, restored.Code, restoredBody)
	require.Contains(t, restoredBody, `"restored":true`)

	importPayload := fmt.Sprintf(`{"memories":[{"run_id":"2","scope":"run","content":"导入后的记忆","metadata":{"origin":"file"},"source_type":"import","source_id":"file-1","correction_of_memory_id":"%d"}]}`, correction.MemoryID)
	imported := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/memories/import", thread.ThreadID),
		canonicalMemoryJSONBody(importPayload),
	)
	importedBody := string(imported.Result().Body())
	require.Equal(t, http.StatusOK, imported.Code, importedBody)
	require.Contains(t, importedBody, `"imported":1`)
	require.Contains(t, importedBody, `"skipped":0`)
	require.Contains(t, importedBody, `"run_id":"2"`)

	duplicateImport := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/memories/import", thread.ThreadID),
		canonicalMemoryJSONBody(importPayload),
	)
	duplicateImportBody := string(duplicateImport.Result().Body())
	require.Equal(t, http.StatusOK, duplicateImport.Code, duplicateImportBody)
	require.Contains(t, duplicateImportBody, `"imported":0`)
	require.Contains(t, duplicateImportBody, `"skipped":1`)

	cleared := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/memories/clear", thread.ThreadID),
		canonicalMemoryJSONBody(`{"run_id":"2","scopes":["run"]}`),
	)
	require.Equal(t, http.StatusOK, cleared.Code, cleared.Result().Body())
	require.Contains(t, string(cleared.Result().Body()), `"deleted":2`)
}

func TestCanonicalThreadMemoryErrors(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "memory errors", `{}`)
	memory := createCanonicalMemoryFixture(t, thread.ThreadID, "错误记忆", "source-error")
	h := canonicalMemoryAuditTestServer()

	invalidMetadata := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodPut,
		fmt.Sprintf("/api/workbench/threads/%d/memories/%d", thread.ThreadID, memory.MemoryID),
		canonicalMemoryJSONBody(`{"scope":"thread","content":"bad","metadata":"not-object"}`),
	)
	require.Equal(t, http.StatusUnprocessableEntity, invalidMetadata.Code)
	require.Contains(t, string(invalidMetadata.Result().Body()), `"code":"invalid_request"`)

	missingScope := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodPut,
		fmt.Sprintf("/api/workbench/threads/%d/memories/%d", thread.ThreadID, memory.MemoryID),
		canonicalMemoryJSONBody(`{"content":"bad"}`),
	)
	require.Equal(t, http.StatusUnprocessableEntity, missingScope.Code)
	require.Contains(t, string(missingScope.Result().Body()), `"code":"invalid_request"`)

	invalidScope := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/memories?scope=unknown", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, invalidScope.Code)
	require.Contains(t, string(invalidScope.Result().Body()), `"code":"invalid_request"`)

	invalidBool := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/memories?include_deleted=1", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, invalidBool.Code)
	require.Contains(t, string(invalidBool.Result().Body()), `"code":"invalid_query_parameter"`)

	importLimit := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/memories/import", thread.ThreadID),
		canonicalMemoryJSONBody(canonicalMemoryImportPayload(101)),
	)
	require.Equal(t, http.StatusUnprocessableEntity, importLimit.Code)
	require.Contains(t, string(importLimit.Result().Body()), `"code":"invalid_request"`)

	appagentthread.SVC.GuardrailAuditRepository = nil
	guardrailDependency := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/guardrail_audit_events", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusServiceUnavailable, guardrailDependency.Code)
	require.Contains(t, string(guardrailDependency.Result().Body()), `"code":"dependency_unavailable"`)

	appagentthread.SVC.MCPRuntimeAuditRepository = nil
	mcpDependency := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/mcp_runtime_audit_events", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusServiceUnavailable, mcpDependency.Code)
	require.Contains(t, string(mcpDependency.Result().Body()), `"code":"dependency_unavailable"`)

	wrongWorkspace := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/memories", thread.ThreadID),
		nil,
		ut.Header{Key: canonicalSpaceIDHeader, Value: "2002"},
	)
	require.Equal(t, http.StatusNotFound, wrongWorkspace.Code)
	require.Contains(t, string(wrongWorkspace.Result().Body()), `"code":"resource_not_found"`)

	appagentthread.SVC.MemoryAuthorizer = &recordingWorkbenchMemoryAuthorizer{
		err: appagentthread.ErrMemoryAccessDenied,
	}
	denied := performCanonicalMemoryRequest(
		t,
		h,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/%d/memories", thread.ThreadID),
		nil,
	)
	require.Equal(t, http.StatusNotFound, denied.Code)
	require.Contains(t, string(denied.Result().Body()), `"code":"resource_not_found"`)
}

func canonicalMemoryAuditTestServer() *server.Hertz {
	h := authenticatedAgentThreadTestServer()
	registerCanonicalMemoryAuditRoutes(h)
	return h
}

func registerCanonicalMemoryAuditRoutes(h *server.Hertz) {
	h.GET("/api/workbench/threads/:thread_id/memories", ListCanonicalThreadMemories)
	h.PUT("/api/workbench/threads/:thread_id/memories/:memory_id", UpdateCanonicalThreadMemory)
	h.DELETE("/api/workbench/threads/:thread_id/memories/:memory_id", DeleteCanonicalThreadMemory)
	h.POST("/api/workbench/threads/:thread_id/memories/:memory_id/restore", RestoreCanonicalThreadMemory)
	h.POST("/api/workbench/threads/:thread_id/memories/clear", ClearCanonicalThreadMemories)
	h.GET("/api/workbench/threads/:thread_id/memories/export", ExportCanonicalThreadMemories)
	h.POST("/api/workbench/threads/:thread_id/memories/import", ImportCanonicalThreadMemories)
	h.GET("/api/workbench/threads/:thread_id/memories/audit_events", ListCanonicalThreadMemoryAuditEvents)
	h.GET("/api/workbench/threads/:thread_id/guardrail_audit_events", ListCanonicalThreadGuardrailAuditEvents)
	h.GET("/api/workbench/threads/:thread_id/guardrail_audit_events/export", ExportCanonicalThreadGuardrailAuditEvents)
	h.GET("/api/workbench/threads/:thread_id/mcp_runtime_audit_events", ListCanonicalThreadMCPRuntimeAuditEvents)
}

func performCanonicalMemoryRequest(
	t *testing.T,
	h *server.Hertz,
	method string,
	path string,
	body *ut.Body,
	headers ...ut.Header,
) *ut.ResponseRecorder {
	t.Helper()
	requestHeaders := make([]ut.Header, 0, len(headers)+2)
	if body != nil {
		requestHeaders = append(requestHeaders, ut.Header{Key: "Content-Type", Value: "application/json"})
	}
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

func canonicalMemoryJSONBody(body string) *ut.Body {
	return &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
}

func canonicalMemoryImportPayload(count int) string {
	var builder strings.Builder
	builder.WriteString(`{"memories":[`)
	for i := 0; i < count; i++ {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(fmt.Sprintf(`{"scope":"thread","content":"导入记忆 %d","source_type":"import","source_id":"limit-%d"}`, i, i))
	}
	builder.WriteString(`]}`)
	return builder.String()
}

func createCanonicalMemoryFixture(
	t *testing.T,
	threadID int64,
	content string,
	sourceID string,
) *appagentthread.MemorySummary {
	t.Helper()
	resp, err := appagentthread.SVC.RememberMemory(context.Background(), &appagentthread.RememberMemoryRequest{
		ThreadID:   threadID,
		Scope:      appagentthread.MemoryScopeLongTerm,
		Content:    content,
		Metadata:   `{"origin":"manual","provider_body":"secret-token","nested":{"authorization":"secret-token"}}`,
		Score:      0.8,
		Confidence: 0.9,
		SourceType: "manual",
		SourceID:   sourceID,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Memory)
	return resp.Memory
}

func recordCanonicalGuardrailAudit(
	t *testing.T,
	threadID int64,
	eventID int64,
	action appagentthread.GuardrailAction,
) {
	t.Helper()
	recorder := appagentthread.NewApplicationGuardrailAuditRecorder(
		appagentthread.ApplicationGuardrailAuditRecorderOptions{
			Repository: appagentthread.SVC.GuardrailAuditRepository,
			IDGen:      &sequentialIDGen{next: eventID},
			NowMillis:  func() int64 { return 4000 },
		},
	)
	err := recorder.RecordGuardrailDecision(
		context.Background(),
		appagentthread.GuardrailRequest{
			SpaceID:    1001,
			ThreadID:   threadID,
			RunID:      2,
			UserID:     2,
			TargetType: appagentthread.GuardrailTargetNetwork,
			TargetID:   "web_fetch",
			Operation:  "invoke",
			Source:     "adk_runtime_tool",
			FailMode:   appagentthread.GuardrailFailClosed,
			Metadata:   map[string]string{"prompt": "secret prompt"},
		},
		appagentthread.GuardrailDecision{
			Action:     action,
			Provider:   "http_scanner",
			ReasonCode: "network_review",
			Message:    "deny /mnt/raw/object sk-secret",
			RuleIDs:    []string{"url_review", "external_policy"},
			Metadata: map[string]string{
				"provider_raw": `{"decision":"deny","secret":"sk-secret"}`,
				"tool_args":    `{"url":"s3://bucket/raw"}`,
			},
		},
	)
	require.NoError(t, err)
}

func recordCanonicalMCPRuntimeAudit(t *testing.T, threadID int64, eventID int64) {
	t.Helper()
	recorder := appagentthread.NewApplicationADKMCPRuntimeAuditRecorder(
		appagentthread.ApplicationADKMCPRuntimeAuditRecorderOptions{
			Repository: appagentthread.SVC.MCPRuntimeAuditRepository,
			IDGen:      &sequentialIDGen{next: eventID},
			NowMillis:  func() int64 { return 6000 },
		},
	)
	err := recorder.RecordADKMCPRuntimeAudit(
		context.Background(),
		appagentthread.ADKMCPRuntimeAuditRecord{
			SpaceID:         1001,
			ThreadID:        threadID,
			RunID:           2,
			ServerID:        100,
			RuntimeToolName: "mcp_100_get_weather",
			EventType:       "mcp.tool.completed",
			ErrorCode:       "provider_raw_secret_should_not_include_payload",
			ElapsedMillis:   33,
			OutputBytes:     128,
		},
	)
	require.NoError(t, err)
}
