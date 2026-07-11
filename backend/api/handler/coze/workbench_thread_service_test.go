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
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	threadapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	appworkbench "github.com/coze-dev/coze-studio/backend/application/workbench"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/internal/testutil"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestListTaskThreadsHandlerReturnsAgentThreads(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads", ListTaskThreads)
	installAgentThreadTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads?space_id=1&page=1&page_size=10", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"msg":"success"`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"title":"任务列表"`)
}

func TestGetTaskThreadHandlerReturnsAgentThread(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id", GetTaskThread)
	installAgentThreadTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"title":"任务列表"`)
}

func TestGetTaskThreadHandlerReturnsForbiddenForDifferentViewer(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id",
		workbenchSessionMiddlewareForTest(3),
		GetTaskThread,
	)
	installAgentThreadTestService(t)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1",
		nil,
	)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, string(w.Result().Body()), "thread access denied")
}

func TestGetTaskThreadHandlerAccessDeniedWithoutAuthenticatedViewer(t *testing.T) {
	h := server.Default()
	h.GET("/api/workbench/task_threads/:thread_id", GetTaskThread)
	installAgentThreadTestService(t)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1",
		nil,
	)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestWorkbenchChatErrorResponseMapsThreadAccessDeniedToForbidden(t *testing.T) {
	h := server.Default()
	h.GET("/workbench-chat-error", func(ctx context.Context, c *app.RequestContext) {
		workbenchChatErrorResponse(ctx, c, appagentthread.ErrThreadAccessDenied)
	})

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/workbench-chat-error", nil)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, string(w.Result().Body()), "thread access denied")
}

func TestGetTaskThreadHandlerReturnsThreadValuesTodos(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id", GetTaskThread)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		AssistantID: "assistant-a",
		Input:       `{"messages":[{"role":"user","content":"请生成计划"}]}`,
		Config:      `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "eino.adk",
		RuntimeType:     "eino_adk",
		RuntimeKey:      "checkpoint-todos",
		EnvelopeVersion: 1,
		ChannelValues: `{
			"todos": [
				{
					"id": "todo-1",
					"title": "收集资料",
					"status": "completed",
					"prompt": "secret prompt",
					"tool_args": {"api_key": "secret"}
				},
				{
					"id": "todo-2",
					"content": "输出报告",
					"done": false,
					"tool_result": "secret result"
				}
			]
		}`,
		ChannelVersions: `{}`,
		PendingSends:    `[]`,
		Metadata:        `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"values"`)
	require.Contains(t, body, `"todos"`)
	require.Contains(t, body, `"id":"todo-1"`)
	require.Contains(t, body, `"title":"收集资料"`)
	require.Contains(t, body, `"status":"completed"`)
	require.Contains(t, body, `"id":"todo-2"`)
	require.Contains(t, body, `"title":"输出报告"`)
	require.Contains(t, body, `"status":"pending"`)
	require.NotContains(t, body, "secret prompt")
	require.NotContains(t, body, "tool_args")
	require.NotContains(t, body, "secret result")
}

func TestListTaskThreadMessagesHandlerReturnsMessages(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/messages", ListTaskThreadMessages)
	installAgentThreadTestService(t)

	_, err := appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		Role:     appagentthread.MessageRoleUser,
		Content:  "请分析客户反馈",
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		Role:     appagentthread.MessageRoleAssistant,
		Content:  "客户反馈集中在响应速度。",
	})
	require.NoError(t, err)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/messages?page=1&page_size=10", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"role":"user"`)
	require.Contains(t, body, `"content":"请分析客户反馈"`)
	require.Contains(t, body, `"role":"assistant"`)
	require.Contains(t, body, `"content":"客户反馈集中在响应速度。"`)
}

func TestAppendTaskThreadMessageHandlerAccessDeniedDoesNotMutate(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/workbench/task_threads/:thread_id/messages",
		workbenchSessionMiddlewareForTest(3),
		AppendTaskThreadMessage,
	)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"role":    "user",
		"content": "must not persist",
	})
	require.NoError(t, err)
	before, err := appagentthread.SVC.ListMessages(
		context.Background(),
		&appagentthread.ListMessagesRequest{ThreadID: 1},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/messages",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	after, err := appagentthread.SVC.ListMessages(
		context.Background(),
		&appagentthread.ListMessagesRequest{ThreadID: 1},
	)
	require.NoError(t, err)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, before.Total, after.Total)
}

func TestGenerateTaskThreadSuggestionsHandlerReturnsDeerFlowShape(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/workbench/task_threads/:thread_id/suggestions",
		GenerateTaskThreadSuggestions,
	)
	installAgentThreadTestService(t)
	appworkbench.InitService(&appworkbench.ServiceComponents{
		ChatModelProvider: func(_ context.Context, modelType int64) (model.BaseChatModel, bool, error) {
			require.Equal(t, int64(100002), modelType)
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
					require.Len(t, in, 2)
					require.Contains(t, in[1].Content, "User: 请生成武汉三日游攻略")
					return schema.AssistantMessage(`["能补充预算表吗？","可以导出成 Markdown 吗？"]`, nil), nil
				},
			}, true, nil
		},
	})

	payload, err := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "请生成武汉三日游攻略"},
			{"role": "assistant", "content": "已经生成路线。"},
		},
		"n":          2,
		"model_type": "100002",
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/suggestions",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"suggestions"`)
	require.Contains(t, body, `"能补充预算表吗？"`)
	require.Contains(t, body, `"可以导出成 Markdown 吗？"`)
	require.NotContains(t, body, `"code"`)
	require.NotContains(t, body, `"data"`)
}

func TestCreateTaskThreadHandlerCreatesThreadRunAndInitialMessage(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads", workbenchSessionMiddlewareForTest(42), CreateTaskThread)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"space_id":        "9",
		"message":         "请生成行动计划",
		"config":          `{"runtime":"eino_adk"}`,
		"metadata":        `{"source":"new_task"}`,
		"stream_mode":     `["messages","updates"]`,
		"idempotency_key": "new-task-1",
	})
	require.NoError(t, err)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"2"`)
	require.Contains(t, body, `"creator_id":"42"`)
	require.Contains(t, body, `"title":"请生成行动计划"`)
	require.Contains(t, body, `"role":"user"`)
	require.Contains(t, body, `"content":"请生成行动计划"`)
	require.Contains(t, body, `"status":"pending"`)
	require.NotContains(t, body, `"runtime":"eino_adk"`)
	require.NotContains(t, body, `"idempotency_key":"new-task-1"`)

	messages, err := appagentthread.SVC.ListMessages(context.Background(), &appagentthread.ListMessagesRequest{
		ThreadID: 2,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), messages.Total)
	require.Equal(t, "请生成行动计划", messages.Messages[0].Content)

	runs, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 2,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), runs.Total)
	require.Equal(t, runs.Runs[0].RunID, messages.Messages[0].RunID)
	require.Equal(t, appagentthread.RunStatusPending, runs.Runs[0].Status)
	require.Equal(t, appagentthread.RunKindTask, runs.Runs[0].RunKind)
	require.Equal(t, `{"runtime":"eino_adk"}`, runs.Runs[0].Config)
	require.Equal(t, "new-task-1", runs.Runs[0].IdempotencyKey)
}

func TestCreateTaskThreadHandlerRejectsUnauthorizedWorkspaceBeforeMutation(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads", CreateTaskThread)
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
		"space_id": "999",
		"message":  "must not persist",
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	after, err := appagentthread.SVC.ListThreads(context.Background(), &appagentthread.ListThreadsRequest{
		SpaceID: 1,
		Page:    1,
	})
	require.NoError(t, err)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, before.Total, after.Total)
}

func TestCreateTaskThreadHandlerMapsWorkspaceDependencyFailureToInternalError(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads", CreateTaskThread)
	installAgentThreadTestService(t)
	appagentthread.SVC.WorkspaceAuthorizer = &recordingWorkbenchWorkspaceAuthorizer{
		err: appagentthread.ErrThreadAuthorizationUnavailable,
	}
	payload, err := json.Marshal(map[string]any{
		"space_id": "1",
		"message":  "dependency failure",
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NotContains(t, string(w.Result().Body()), "thread access denied")
}

func TestExportTaskThreadMemoriesHandlerReturnsSchemaPayload(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/memories/export", ExportTaskThreadMemories)
	installAgentThreadTestService(t)
	_, err := appagentthread.SVC.RememberMemory(context.Background(), &appagentthread.RememberMemoryRequest{
		ThreadID:   1,
		Scope:      appagentthread.MemoryScopeLongTerm,
		Content:    "用户偏好中文摘要",
		Metadata:   `{"origin":"manual"}`,
		Score:      0.8,
		Confidence: 0.9,
		SourceType: "manual",
		SourceID:   "memory-ui-1",
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/memories/export?scope=long_term&limit=50",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"schema":"coze.task_thread_memories.export.v1"`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"content":"用户偏好中文摘要"`)
	require.NotContains(t, body, "tool_args")
	require.NotContains(t, body, "checkpoint")
}

func TestImportTaskThreadMemoriesHandlerCreatesMemoriesAndAuditsActor(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/workbench/task_threads/:thread_id/memories/import",
		workbenchSessionMiddlewareForTest(99),
		ImportTaskThreadMemories,
	)
	installAgentThreadTestService(t)
	payload := []byte(`{
		"memories": [
			{
				"scope": "thread",
				"content": "导入后的记忆",
				"metadata": "{\"origin\":\"file\"}",
				"score": 0.75,
				"confidence": 0.85,
				"source_type": "import",
				"source_id": "file-1"
			}
		]
	}`)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/memories/import",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	respBody := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, respBody, `"code":0`)
	require.Contains(t, respBody, `"imported":1`)
	require.Contains(t, respBody, `"skipped":0`)
	require.Contains(t, respBody, `"content":"导入后的记忆"`)

	audits, err := appagentthread.SVC.ListMemoryAuditEvents(context.Background(), &appagentthread.ListMemoryAuditEventsRequest{
		ThreadID: 1,
	})
	require.NoError(t, err)
	require.Len(t, audits.Events, 1)
	require.Equal(t, int64(99), audits.Events[0].ActorID)
	require.Equal(t, "memory.imported", audits.Events[0].EventType)
	require.Equal(t, int64(1), audits.Events[0].AffectedCount)
}

func TestListTaskThreadMemoriesHandlerPassesSessionViewerID(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.Use(workbenchSessionMiddlewareForTest(2))
	h.GET("/api/workbench/task_threads/:thread_id/memories", ListTaskThreadMemories)
	installAgentThreadTestService(t)
	authorizer := &recordingWorkbenchMemoryAuthorizer{}
	appagentthread.SVC.MemoryAuthorizer = authorizer

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/memories?page=1&page_size=10",
		nil,
	)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, appagentthread.MemoryAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(1), authorizer.req.ThreadID)
	require.Equal(t, int64(2), authorizer.req.ViewerID)
}

func TestListTaskThreadMemoriesHandlerMapsAuthorizationDeniedToForbidden(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/memories", ListTaskThreadMemories)
	installAgentThreadTestService(t)
	appagentthread.SVC.MemoryAuthorizer = &recordingWorkbenchMemoryAuthorizer{
		err: appagentthread.ErrMemoryAccessDenied,
	}

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/memories?page=1&page_size=10",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, body, "memory access denied")
	require.NotContains(t, body, "internal server error")
}

func TestListTaskThreadGuardrailAuditEventsHandlerReturnsSafeMetadata(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/guardrail_audit_events",
		workbenchSessionMiddlewareForTest(2),
		ListTaskThreadGuardrailAuditEvents,
	)
	installAgentThreadTestService(t)
	recorder := appagentthread.NewApplicationGuardrailAuditRecorder(
		appagentthread.ApplicationGuardrailAuditRecorderOptions{
			Repository: appagentthread.SVC.GuardrailAuditRepository,
			IDGen:      &sequentialIDGen{next: 9101},
			NowMillis:  func() int64 { return 4000 },
		},
	)
	err := recorder.RecordGuardrailDecision(
		context.Background(),
		appagentthread.GuardrailRequest{
			SpaceID:    1,
			ThreadID:   1,
			RunID:      2,
			UserID:     2,
			TargetType: appagentthread.GuardrailTargetToolCall,
			TargetID:   "runtime_tool:search_docs",
			Operation:  "invoke",
			Source:     "adk_tool_wrapper",
			FailMode:   appagentthread.GuardrailFailClosed,
			Metadata: map[string]string{
				"prompt": "secret prompt",
			},
		},
		appagentthread.GuardrailDecision{
			Action:     appagentthread.GuardrailActionConfirm,
			Provider:   "scanner",
			ReasonCode: "high_risk",
			Message:    "review /mnt/raw/object sk-secret",
			RuleIDs:    []string{"rule:high_risk"},
			Metadata: map[string]string{
				"tool_args": `{"q":"secret"}`,
				"object":    "s3://bucket/raw",
			},
		},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/guardrail_audit_events?run_id=2&page=1&page_size=20",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"event_id":"9101"`)
	require.Contains(t, body, `"event_type":"guardrail.decision.confirm"`)
	require.Contains(t, body, `"target_type":"tool_call"`)
	require.Contains(t, body, `"target_id":"runtime_tool:search_docs"`)
	require.Contains(t, body, `"action":"confirm"`)
	require.NotContains(t, body, "secret prompt")
	require.NotContains(t, body, "tool_args")
	require.NotContains(t, body, "sk-secret")
	require.NotContains(t, body, "/mnt/raw")
	require.NotContains(t, body, "s3://")
}

func TestListTaskThreadMCPRuntimeAuditEventsHandlerReturnsSafeMetadata(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/mcp_runtime_audit_events",
		workbenchSessionMiddlewareForTest(2),
		ListTaskThreadMCPRuntimeAuditEvents,
	)
	installAgentThreadTestService(t)
	recorder := appagentthread.NewApplicationADKMCPRuntimeAuditRecorder(
		appagentthread.ApplicationADKMCPRuntimeAuditRecorderOptions{
			Repository: appagentthread.SVC.MCPRuntimeAuditRepository,
			IDGen:      &sequentialIDGen{next: 9201},
			NowMillis:  func() int64 { return 6000 },
		},
	)
	err := recorder.RecordADKMCPRuntimeAudit(
		context.Background(),
		appagentthread.ADKMCPRuntimeAuditRecord{
			SpaceID:         1,
			ThreadID:        1,
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

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/mcp_runtime_audit_events?run_id=2&page=1&page_size=20",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"event_id":"9201"`)
	require.Contains(t, body, `"runtime_tool_name":"mcp_100_get_weather"`)
	require.Contains(t, body, `"event_type":"mcp.tool.completed"`)
	require.Contains(t, body, `"elapsed_millis":33`)
	require.Contains(t, body, `"output_bytes":128`)
	require.NotContains(t, body, "secret prompt")
	require.NotContains(t, body, "tool_args")
	require.NotContains(t, body, "sk-")
	require.NotContains(t, body, "/mnt/raw")
	require.NotContains(t, body, "s3://")
}

func TestListTaskThreadMCPRuntimeAuditEventsHandlerPassesSessionViewerID(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/mcp_runtime_audit_events",
		workbenchSessionMiddlewareForTest(2),
		ListTaskThreadMCPRuntimeAuditEvents,
	)
	installAgentThreadTestService(t)
	authorizer := &recordingWorkbenchMCPRuntimeAuditAuthorizer{}
	appagentthread.SVC.MCPRuntimeAuditAuthorizer = authorizer

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/mcp_runtime_audit_events?run_id=2&page=1&page_size=20",
		nil,
	)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, appagentthread.MCPRuntimeAuditAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(1), authorizer.req.ThreadID)
	require.Equal(t, int64(2), authorizer.req.ViewerID)
}

func TestListTaskThreadMCPRuntimeAuditEventsHandlerMapsAuthorizationDeniedToForbidden(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/mcp_runtime_audit_events",
		ListTaskThreadMCPRuntimeAuditEvents,
	)
	installAgentThreadTestService(t)
	appagentthread.SVC.MCPRuntimeAuditAuthorizer = &recordingWorkbenchMCPRuntimeAuditAuthorizer{
		err: appagentthread.ErrMCPRuntimeAuditAccessDenied,
	}

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/mcp_runtime_audit_events?run_id=2&page=1&page_size=20",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, body, "mcp runtime audit access denied")
	require.NotContains(t, body, "internal server error")
}

func TestExportTaskThreadGuardrailAuditEventsHandlerReturnsSchemaPayload(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/guardrail_audit_events/export",
		workbenchSessionMiddlewareForTest(2),
		ExportTaskThreadGuardrailAuditEvents,
	)
	installAgentThreadTestService(t)
	recorder := appagentthread.NewApplicationGuardrailAuditRecorder(
		appagentthread.ApplicationGuardrailAuditRecorderOptions{
			Repository: appagentthread.SVC.GuardrailAuditRepository,
			IDGen:      &sequentialIDGen{next: 9105},
			NowMillis:  func() int64 { return 5000 },
		},
	)
	err := recorder.RecordGuardrailDecision(
		context.Background(),
		appagentthread.GuardrailRequest{
			SpaceID:    1,
			ThreadID:   1,
			RunID:      2,
			UserID:     2,
			TargetType: appagentthread.GuardrailTargetNetwork,
			TargetID:   "web_fetch",
			Operation:  "invoke",
			Source:     "adk_runtime_tool",
			FailMode:   appagentthread.GuardrailFailClosed,
			Metadata: map[string]string{
				"prompt": "secret prompt",
			},
		},
		appagentthread.GuardrailDecision{
			Action:     appagentthread.GuardrailActionDeny,
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

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/guardrail_audit_events/export?run_id=2&page=1&page_size=50",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"schema":"coze.task_thread_guardrail_audit.export.v1"`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"page":1`)
	require.Contains(t, body, `"page_size":50`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"event_id":"9105"`)
	require.Contains(t, body, `"event_type":"guardrail.decision.deny"`)
	require.Contains(t, body, `"target_type":"network"`)
	require.Contains(t, body, `"target_id":"web_fetch"`)
	require.Contains(t, body, `"action":"deny"`)
	require.NotContains(t, body, "secret prompt")
	require.NotContains(t, body, "tool_args")
	require.NotContains(t, body, "provider_raw")
	require.NotContains(t, body, "sk-secret")
	require.NotContains(t, body, "/mnt/raw")
	require.NotContains(t, body, "s3://")
}

func TestListTaskThreadGuardrailAuditEventsHandlerPassesSessionViewerID(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/guardrail_audit_events",
		workbenchSessionMiddlewareForTest(2),
		ListTaskThreadGuardrailAuditEvents,
	)
	installAgentThreadTestService(t)
	authorizer := &recordingWorkbenchGuardrailAuditAuthorizer{}
	appagentthread.SVC.GuardrailAuditAuthorizer = authorizer

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/guardrail_audit_events?run_id=2&page=1&page_size=20",
		nil,
	)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, appagentthread.GuardrailAuditAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(1), authorizer.req.ThreadID)
	require.Equal(t, int64(2), authorizer.req.ViewerID)
}

func TestListTaskThreadGuardrailAuditEventsHandlerMapsAuthorizationDeniedToForbidden(
	t *testing.T,
) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/guardrail_audit_events",
		ListTaskThreadGuardrailAuditEvents,
	)
	installAgentThreadTestService(t)
	appagentthread.SVC.GuardrailAuditAuthorizer = &recordingWorkbenchGuardrailAuditAuthorizer{
		err: appagentthread.ErrGuardrailAuditAccessDenied,
	}

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/guardrail_audit_events?run_id=2&page=1&page_size=20",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, body, "guardrail audit access denied")
	require.NotContains(t, body, "internal server error")
}

func TestListTaskThreadArtifactsHandlerReturnsArtifacts(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/artifacts", ListTaskThreadArtifacts)
	installAgentThreadTestService(t)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	fileName := strings.Repeat("a", 64) + ".txt"
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:       runResp.Run.RunID,
			FileName:    fileName,
			FileKind:    domainentity.AgentFileKindWorkspace,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/2/trunc/" + fileName,
			ObjectURI:   "agent-runtime/1/1/runs/2/tool-results/trunc/" + fileName,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   128,
			Digest:      strings.Repeat("b", 64),
			Metadata:    `{}`,
		},
	)
	require.NoError(t, err)
	_, _, err = appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts?run_id=2&page=1&page_size=10",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"artifact_type":"report"`)
	require.Contains(t, body, `"preview_mode":"text"`)
	require.Contains(t, body, `"virtual_path":""`)
	require.NotContains(t, body, `/mnt/user-data/workspace/.coze/tool-results/`)
	require.NotContains(t, body, "agent-runtime/")
}

func TestListTaskThreadArtifactsHandlerReturnsDeletedArtifactsWhenRequested(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/artifacts", ListTaskThreadArtifacts)
	installAgentThreadTestService(t)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	fileName := strings.Repeat("c", 64) + ".txt"
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:       runResp.Run.RunID,
			FileName:    fileName,
			FileKind:    domainentity.AgentFileKindWorkspace,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/2/trunc/" + fileName,
			ObjectURI:   "agent-runtime/1/1/runs/2/tool-results/trunc/" + fileName,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   128,
			Digest:      strings.Repeat("d", 64),
			Metadata:    `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	_, deleted, err := appagentthread.SVC.ArtifactSVC.DeleteArtifact(
		context.Background(),
		&domainservice.DeleteArtifactRequest{
			ThreadID:   1,
			ArtifactID: artifact.ID,
			DeletedAt:  1300,
		},
	)
	require.NoError(t, err)
	require.True(t, deleted)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts?deleted_only=true&page=1&page_size=10",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"artifact_id":"`+strconv.FormatInt(artifact.ID, 10)+`"`)
	require.Contains(t, body, `"deleted_at":1300`)
	require.NotContains(t, body, "agent-runtime/")
}

func TestListTaskThreadArtifactsHandlerPassesSessionViewerID(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.Use(workbenchSessionMiddlewareForTest(2))
	h.GET("/api/workbench/task_threads/:thread_id/artifacts", ListTaskThreadArtifacts)
	installAgentThreadTestService(t)
	authorizer := &recordingWorkbenchArtifactAuthorizer{}
	appagentthread.SVC.ArtifactAuthorizer = authorizer

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts?page=1&page_size=10",
		nil,
	)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, appagentthread.ArtifactAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(1), authorizer.req.ThreadID)
	require.Equal(t, int64(2), authorizer.req.ViewerID)
}

func TestListTaskThreadArtifactsHandlerMapsAuthorizationDeniedToForbidden(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/artifacts", ListTaskThreadArtifacts)
	installAgentThreadTestService(t)
	appagentthread.SVC.ArtifactAuthorizer = &recordingWorkbenchArtifactAuthorizer{
		err: appagentthread.ErrArtifactAccessDenied,
	}

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts?page=1&page_size=10",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, body, "artifact access denied")
	require.NotContains(t, body, "internal server error")
}

func TestListTaskThreadArtifactScanJobsHandlerReturnsSafeMetadata(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/artifact_scan_jobs",
		ListTaskThreadArtifactScanJobs,
	)
	installAgentThreadTestService(t)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	runIDText := strconv.FormatInt(runResp.Run.RunID, 10)
	fileName := strings.Repeat("c", 64) + ".txt"
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:            runResp.Run.RunID,
			FileName:         fileName,
			OriginalFileName: "secret.txt",
			FileKind:         domainentity.AgentFileKindWorkspace,
			VirtualPath:      "/mnt/user-data/workspace/.coze/tool-results/runs/" + runIDText + "/trunc/" + fileName,
			ObjectURI:        "agent-runtime/1/1/runs/" + runIDText + "/tool-results/trunc/" + fileName,
			ContentType:      "text/plain; charset=utf-8",
			SizeBytes:        128,
			Digest:           strings.Repeat("c", 64),
			Metadata:         `{}`,
		},
	)
	require.NoError(t, err)
	_, _, err = appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	jobs, err := appagentthread.SVC.ArtifactSVC.ClaimArtifactScanJobs(
		context.Background(),
		&domainservice.ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       "scan-worker-a",
			Limit:          1,
			LeaseTTLMillis: 60000,
		},
	)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	_, ok, err := appagentthread.SVC.ArtifactSVC.FailArtifactScanJob(
		context.Background(),
		&domainservice.FailArtifactScanJobRequest{
			JobID:     jobs[0].ID,
			WorkerID:  "scan-worker-a",
			ErrorText: "scanner unavailable",
		},
	)
	require.NoError(t, err)
	require.True(t, ok)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifact_scan_jobs?status=failed&scanner=default&page=1&page_size=10",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"status":"failed"`)
	require.Contains(t, body, `"last_error":"scanner unavailable"`)
	require.Contains(t, body, `"attempt_count":1`)
	require.NotContains(t, body, "agent-runtime/")
	require.NotContains(t, body, "/mnt/user-data")
	require.NotContains(t, body, "secret.txt")
}

func TestRetryTaskThreadArtifactScanJobHandlerRequeuesFailedJob(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/workbench/task_threads/:thread_id/artifact_scan_jobs/:job_id/retry",
		RetryTaskThreadArtifactScanJob,
	)
	installAgentThreadTestService(t)
	runResp, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{
			ThreadID: 1,
			Input:    `{"messages":[]}`,
		},
	)
	require.NoError(t, err)
	runIDText := strconv.FormatInt(runResp.Run.RunID, 10)
	fileName := strings.Repeat("d", 64) + ".txt"
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:            runResp.Run.RunID,
			FileName:         fileName,
			OriginalFileName: "secret.txt",
			FileKind:         domainentity.AgentFileKindWorkspace,
			VirtualPath:      "/mnt/user-data/workspace/.coze/tool-results/runs/" + runIDText + "/trunc/" + fileName,
			ObjectURI:        "agent-runtime/1/1/runs/" + runIDText + "/tool-results/trunc/" + fileName,
			ContentType:      "text/plain; charset=utf-8",
			SizeBytes:        128,
			Digest:           strings.Repeat("d", 64),
			Metadata:         `{}`,
		},
	)
	require.NoError(t, err)
	_, _, err = appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	jobs, err := appagentthread.SVC.ArtifactSVC.ClaimArtifactScanJobs(
		context.Background(),
		&domainservice.ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       "scan-worker-a",
			Limit:          1,
			LeaseTTLMillis: 60000,
		},
	)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	_, ok, err := appagentthread.SVC.ArtifactSVC.FailArtifactScanJob(
		context.Background(),
		&domainservice.FailArtifactScanJobRequest{
			JobID:     jobs[0].ID,
			WorkerID:  "scan-worker-a",
			ErrorText: "scanner unavailable",
		},
	)
	require.NoError(t, err)
	require.True(t, ok)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/artifact_scan_jobs/"+
			strconv.FormatInt(jobs[0].ID, 10)+"/retry",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"retried":true`)
	require.Contains(t, body, `"status":"pending"`)
	require.Contains(t, body, `"last_error":"manual retry requested"`)
	require.NotContains(t, body, "agent-runtime/")
	require.NotContains(t, body, "/mnt/user-data")
	require.NotContains(t, body, "secret.txt")
}

func TestRetryTaskThreadArtifactScanJobHandlerReturnsConflictForNonFailedJob(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/workbench/task_threads/:thread_id/artifact_scan_jobs/:job_id/retry",
		RetryTaskThreadArtifactScanJob,
	)
	installAgentThreadTestService(t)
	runResp, err := appagentthread.SVC.CreateRun(
		context.Background(),
		&appagentthread.CreateRunRequest{
			ThreadID: 1,
			Input:    `{"messages":[]}`,
		},
	)
	require.NoError(t, err)
	runIDText := strconv.FormatInt(runResp.Run.RunID, 10)
	fileName := strings.Repeat("e", 64) + ".txt"
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:       runResp.Run.RunID,
			FileName:    fileName,
			FileKind:    domainentity.AgentFileKindWorkspace,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/" + runIDText + "/trunc/" + fileName,
			ObjectURI:   "agent-runtime/1/1/runs/" + runIDText + "/tool-results/trunc/" + fileName,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   128,
			Digest:      strings.Repeat("e", 64),
			Metadata:    `{}`,
		},
	)
	require.NoError(t, err)
	_, _, err = appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	jobs, err := appagentthread.SVC.ArtifactSVC.ClaimArtifactScanJobs(
		context.Background(),
		&domainservice.ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       "scan-worker-a",
			Limit:          1,
			LeaseTTLMillis: 60000,
		},
	)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/artifact_scan_jobs/"+
			strconv.FormatInt(jobs[0].ID, 10)+"/retry",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, body, "artifact scan job cannot be retried")
	require.NotContains(t, body, "agent-runtime/")
	require.NotContains(t, body, "/mnt/user-data")
}

func TestGetTaskThreadArtifactContentHandlerReturnsBytesWithSafeHeaders(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content",
		GetTaskThreadArtifactContent,
	)
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects: map[string][]byte{},
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	fileName := strings.Repeat("a", 64) + ".txt"
	objectURI := "agent-runtime/1/1/runs/2/tool-results/trunc/" + fileName
	content := []byte("artifact body")
	storage.objects[objectURI] = content
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:       runResp.Run.RunID,
			FileName:    fileName,
			FileKind:    domainentity.AgentFileKindWorkspace,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/2/trunc/" + fileName,
			ObjectURI:   objectURI,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   int64(len(content)),
			Digest:      strings.Repeat("b", 64),
			Metadata:    `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			Title:        "report.txt",
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordArtifactScanResult(
		context.Background(),
		&appagentthread.RecordArtifactScanResultRequest{
			ThreadID:   1,
			ArtifactID: artifact.ID,
			ScanStatus: "clean",
			Scanner:    "test",
			ScannedAt:  1,
		},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/content?mode=preview",
		nil,
	)
	res := w.Result()

	require.Equal(t, http.StatusOK, res.StatusCode())
	require.Equal(t, content, res.Body())
	require.Contains(t, string(res.Header.Peek("Content-Type")), "text/plain")
	contentDisposition := string(res.Header.Peek("Content-Disposition"))
	require.Contains(t, contentDisposition, "inline")
	require.Contains(t, contentDisposition, "filename*=UTF-8''report.txt")
	require.NotContains(t, contentDisposition, "agent-runtime/")
	require.Equal(t, "nosniff", string(res.Header.Peek("X-Content-Type-Options")))
}

func TestGetTaskThreadArtifactSignedURLHandlerReturnsSafeReceipt(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url",
		GetTaskThreadArtifactSignedURL,
	)
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects:   map[string][]byte{},
		signedURL: "https://storage.example.test/signed/report.txt?token=abc",
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	fileName := strings.Repeat("e", 64) + ".txt"
	objectURI := "agent-runtime/1/1/runs/2/tool-results/trunc/" + fileName
	storage.objects[objectURI] = []byte("artifact body")
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:       runResp.Run.RunID,
			FileName:    fileName,
			FileKind:    domainentity.AgentFileKindWorkspace,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/2/trunc/" + fileName,
			ObjectURI:   objectURI,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   int64(len("artifact body")),
			Digest:      strings.Repeat("f", 64),
			Metadata:    `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			Title:        "report.txt",
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordArtifactScanResult(
		context.Background(),
		&appagentthread.RecordArtifactScanResultRequest{
			ThreadID:   1,
			ArtifactID: artifact.ID,
			ScanStatus: "clean",
			Scanner:    "test",
			ScannedAt:  1,
		},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts/"+
			strconv.FormatInt(artifact.ID, 10)+"/signed_url?mode=preview&ttl_seconds=99999",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"url":"https://storage.example.test/signed/report.txt?token=abc"`)
	require.Contains(t, body, `"expires_in_seconds":3600`)
	require.Contains(t, body, `"artifact_id":"`+strconv.FormatInt(artifact.ID, 10)+`"`)
	require.NotContains(t, body, `"object_uri"`)
	require.NotContains(t, body, objectURI)
	require.Equal(t, objectURI, storage.signKey)
	require.Equal(t, int64(3600), storage.signExpire)
}

func TestGetTaskThreadArtifactSignedURLHandlerCreatesDownloadReceipt(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url",
		GetTaskThreadArtifactSignedURL,
	)
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects:   map[string][]byte{},
		signedURL: "https://storage.example.test/signed/page.html?token=abc",
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	fileName := strings.Repeat("a", 64) + ".txt"
	runID := strconv.FormatInt(runResp.Run.RunID, 10)
	objectURI := "agent-runtime/1/1/runs/" + runID + "/tool-results/trunc/" + fileName
	content := []byte("<!doctype html><html></html>")
	storage.objects[objectURI] = content
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:       runResp.Run.RunID,
			FileName:    fileName,
			FileKind:    domainentity.AgentFileKindWorkspace,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/" + runID + "/trunc/" + fileName,
			ObjectURI:   objectURI,
			ContentType: "text/html; charset=utf-8",
			SizeBytes:   int64(len(content)),
			Digest:      strings.Repeat("b", 64),
			Metadata:    `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			Title:        "page.html",
			ArtifactType: "html",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordArtifactScanResult(
		context.Background(),
		&appagentthread.RecordArtifactScanResultRequest{
			ThreadID:   1,
			ArtifactID: artifact.ID,
			ScanStatus: "clean",
			Scanner:    "test",
			ScannedAt:  1,
		},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts/"+
			strconv.FormatInt(artifact.ID, 10)+"/signed_url?mode=download&ttl_seconds=5",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"url":"https://storage.example.test/signed/page.html?token=abc"`)
	require.Contains(t, body, `"expires_in_seconds":60`)
	require.Contains(t, body, `"preview_mode":"download"`)
	require.NotContains(t, body, objectURI)
	require.Equal(t, objectURI, storage.signKey)
	require.Equal(t, int64(60), storage.signExpire)
	require.Equal(
		t,
		"attachment; filename*=UTF-8''page.html",
		storage.signContentDisposition,
	)
	require.Equal(t, "text/html; charset=utf-8", storage.signContentType)
}

func TestGetTaskThreadArtifactContentHandlerUsesSniffedContentType(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content",
		GetTaskThreadArtifactContent,
	)
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects: map[string][]byte{},
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	fileName := strings.Repeat("c", 64) + ".txt"
	runID := strconv.FormatInt(runResp.Run.RunID, 10)
	objectURI := "agent-runtime/1/1/runs/" + runID + "/tool-results/trunc/" + fileName
	content := []byte("<!doctype html><html><body>unsafe</body></html>")
	storage.objects[objectURI] = content
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:       runResp.Run.RunID,
			FileName:    fileName,
			FileKind:    domainentity.AgentFileKindWorkspace,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/" + runID + "/trunc/" + fileName,
			ObjectURI:   objectURI,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   int64(len(content)),
			Digest:      strings.Repeat("d", 64),
			Metadata:    `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			Title:        "report.txt",
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordArtifactScanResult(
		context.Background(),
		&appagentthread.RecordArtifactScanResultRequest{
			ThreadID:   1,
			ArtifactID: artifact.ID,
			ScanStatus: "clean",
			Scanner:    "test",
			ScannedAt:  1,
		},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/content?mode=preview",
		nil,
	)
	res := w.Result()

	require.Equal(t, http.StatusOK, res.StatusCode())
	require.Equal(t, content, res.Body())
	require.Contains(t, string(res.Header.Peek("Content-Type")), "text/html")
	require.Contains(t, string(res.Header.Peek("Content-Disposition")), "attachment")
	require.Equal(t, "nosniff", string(res.Header.Peek("X-Content-Type-Options")))
}

func TestGetTaskThreadArtifactContentHandlerMapsScanBlockedToConflict(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content",
		GetTaskThreadArtifactContent,
	)
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects: map[string][]byte{},
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	runID := strconv.FormatInt(runResp.Run.RunID, 10)
	fileName := strings.Repeat("e", 64) + ".txt"
	objectURI := "agent-runtime/1/1/runs/" + runID + "/tool-results/trunc/" + fileName
	storage.objects[objectURI] = []byte("blocked body")
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:            runResp.Run.RunID,
			FileName:         fileName,
			OriginalFileName: "secret.txt",
			FileKind:         domainentity.AgentFileKindWorkspace,
			VirtualPath:      "/mnt/user-data/workspace/.coze/tool-results/runs/" + runID + "/trunc/" + fileName,
			ObjectURI:        objectURI,
			ContentType:      "text/plain; charset=utf-8",
			SizeBytes:        int64(len("blocked body")),
			Digest:           strings.Repeat("e", 64),
			Metadata:         `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			Title:        "secret.txt",
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordArtifactScanResult(
		context.Background(),
		&appagentthread.RecordArtifactScanResultRequest{
			ThreadID:   1,
			ArtifactID: artifact.ID,
			ScanStatus: "blocked",
			Scanner:    "test",
			ScannedAt:  1,
		},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/content?mode=preview",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, body, `"msg":"artifact content blocked by scan policy"`)
	require.Contains(t, body, `"reason":"scan_blocked"`)
	require.NotContains(t, body, "agent-runtime/")
	require.NotContains(t, body, "/mnt/user-data")
	require.NotContains(t, body, "secret.txt")
	require.NotContains(t, body, "blocked body")
}

func TestReviewTaskThreadArtifactScanHandlerReleasesBlockedArtifact(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/scan_review",
		ReviewTaskThreadArtifactScan,
	)
	h.GET(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content",
		GetTaskThreadArtifactContent,
	)
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects: map[string][]byte{},
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	runID := strconv.FormatInt(runResp.Run.RunID, 10)
	fileName := strings.Repeat("a", 64) + ".txt"
	objectURI := "agent-runtime/1/1/runs/" + runID + "/tool-results/trunc/" + fileName
	content := []byte("released body")
	storage.objects[objectURI] = content
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:            runResp.Run.RunID,
			FileName:         fileName,
			OriginalFileName: "manual-secret.txt",
			FileKind:         domainentity.AgentFileKindWorkspace,
			VirtualPath:      "/mnt/user-data/workspace/.coze/tool-results/runs/" + runID + "/trunc/" + fileName,
			ObjectURI:        objectURI,
			ContentType:      "text/plain; charset=utf-8",
			SizeBytes:        int64(len(content)),
			Digest:           strings.Repeat("a", 64),
			Metadata:         `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			Title:        "manual-secret.txt",
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordArtifactScanResult(
		context.Background(),
		&appagentthread.RecordArtifactScanResultRequest{
			ThreadID:   1,
			ArtifactID: artifact.ID,
			ScanStatus: "blocked",
			Scanner:    "test",
			Reason:     "signature",
			ScannedAt:  1,
		},
	)
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{
		"decision": "release",
		"reason":   "approved by security reviewer",
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/scan_review",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"reviewed":true`)
	require.Contains(t, body, `"decision":"release"`)
	require.Contains(t, body, `"scan_status":"clean"`)
	require.NotContains(t, body, "agent-runtime/")
	require.NotContains(t, body, "/mnt/user-data")
	require.NotContains(t, body, "manual-secret.txt")
	require.NotContains(t, body, "released body")
	require.NotContains(t, body, "approved by security reviewer")

	contentResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/content?mode=preview",
		nil,
	)
	require.Equal(t, http.StatusOK, contentResp.Code)
	require.Equal(t, content, contentResp.Result().Body())

	events, err := appagentthread.SVC.ListRunEvents(context.Background(), &appagentthread.ListRunEventsRequest{
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	var reviewEventPayload string
	for _, event := range events.Events {
		if event.EventType == "artifact.scan.reviewed" {
			reviewEventPayload = event.Payload
			break
		}
	}
	require.NotEmpty(t, reviewEventPayload)
	require.NotContains(t, reviewEventPayload, "agent-runtime/")
	require.NotContains(t, reviewEventPayload, "/mnt/user-data")
	require.NotContains(t, reviewEventPayload, "manual-secret.txt")
	require.NotContains(t, reviewEventPayload, "approved by security reviewer")
}

func TestDeleteTaskThreadArtifactHandlerHidesArtifactFromList(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/artifacts", ListTaskThreadArtifacts)
	h.DELETE(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id",
		DeleteTaskThreadArtifact,
	)
	h.POST(
		"/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/restore",
		RestoreTaskThreadArtifact,
	)
	installAgentThreadTestService(t)
	storage := &recordingWorkbenchArtifactStorage{
		objects: map[string][]byte{},
	}
	appagentthread.SVC.ArtifactObjectStorage = storage
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[]}`,
	})
	require.NoError(t, err)
	fileName := strings.Repeat("e", 64) + ".txt"
	runID := strconv.FormatInt(runResp.Run.RunID, 10)
	objectURI := "agent-runtime/1/1/runs/" + runID + "/tool-results/trunc/" + fileName
	storage.objects[objectURI] = []byte("artifact body")
	fileResp, _, err := appagentthread.SVC.RuntimeFileSVC.RegisterRuntimeFile(
		context.Background(),
		&domainservice.RegisterRuntimeFileRequest{
			RunID:       runResp.Run.RunID,
			FileName:    fileName,
			FileKind:    domainentity.AgentFileKindWorkspace,
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/" + runID + "/trunc/" + fileName,
			ObjectURI:   objectURI,
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   int64(len("artifact body")),
			Digest:      strings.Repeat("f", 64),
			Metadata:    `{}`,
		},
	)
	require.NoError(t, err)
	artifact, _, err := appagentthread.SVC.ArtifactSVC.RegisterArtifact(
		context.Background(),
		&domainservice.RegisterArtifactRequest{
			SpaceID:      1,
			ThreadID:     1,
			RunID:        runResp.Run.RunID,
			FileID:       fileResp.ID,
			Title:        "report.txt",
			ArtifactType: "report",
			Metadata:     `{"source":"test"}`,
		},
	)
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodDelete,
		"/api/workbench/task_threads/1/artifacts/"+strconv.FormatInt(artifact.ID, 10),
		nil,
	)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, string(w.Result().Body()), `"code":0`)

	w = ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts",
		nil,
	)
	body := string(w.Result().Body())
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"total":0`)
	require.NotContains(t, body, "report.txt")

	w = ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/artifacts/"+strconv.FormatInt(artifact.ID, 10)+"/restore",
		nil,
	)
	body = string(w.Result().Body())
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"restored":true`)
	require.Contains(t, body, `"artifact_id":"`+strconv.FormatInt(artifact.ID, 10)+`"`)
	require.NotContains(t, body, "agent-runtime/")
	require.NotContains(t, body, "/mnt/user-data")
	require.NotContains(t, body, "report.txt")

	w = ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/artifacts",
		nil,
	)
	body = string(w.Result().Body())
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, "report.txt")

	events, err := appagentthread.SVC.ListRunEvents(context.Background(), &appagentthread.ListRunEventsRequest{
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	var restoredEventPayload string
	for _, event := range events.Events {
		if event.EventType == "artifact.restored" {
			restoredEventPayload = event.Payload
			break
		}
	}
	require.NotEmpty(t, restoredEventPayload)
	require.NotContains(t, restoredEventPayload, "agent-runtime/")
	require.NotContains(t, restoredEventPayload, "/mnt/user-data")
	require.NotContains(t, restoredEventPayload, "report.txt")
}

func TestAppendTaskThreadMessageHandlerCreatesMessage(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads/:thread_id/messages", AppendTaskThreadMessage)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"role":     "user",
		"content":  "请生成行动计划",
		"metadata": `{"source":"test"}`,
	})
	require.NoError(t, err)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/messages",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"role":"user"`)
	require.Contains(t, body, `"content":"请生成行动计划"`)

	resp, err := appagentthread.SVC.ListMessages(context.Background(), &appagentthread.ListMessagesRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Equal(t, "请生成行动计划", resp.Messages[0].Content)
	require.Equal(t, `{"source":"test"}`, resp.Messages[0].Metadata)
}

func TestCreateTaskThreadRunHandlerCreatesPendingRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads/:thread_id/runs", CreateTaskThreadRun)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{
		"input":            `{"messages":[{"role":"user","content":"请追加行动建议"}]}`,
		"config":           `{"mode":"Auto"}`,
		"metadata":         `{"source":"test"}`,
		"message_content":  "请追加行动建议",
		"message_metadata": `{"source":"test_followup"}`,
		"idempotency_key":  "thread-only-1-msg-1",
	})
	require.NoError(t, err)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/runs",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"status":"pending"`)
	require.Contains(t, body, `"message"`)
	require.Contains(t, body, `"content":"请追加行动建议"`)

	resp, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Equal(t, appagentthread.RunStatusPending, resp.Runs[0].Status)
	require.Contains(t, resp.Runs[0].Input, `"content":"请追加行动建议"`)
	require.Equal(t, `{"mode":"Auto"}`, resp.Runs[0].Config)
	require.Equal(t, "thread-only-1-msg-1", resp.Runs[0].IdempotencyKey)

	messages, err := appagentthread.SVC.ListMessages(context.Background(), &appagentthread.ListMessagesRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), messages.Total)
	require.Equal(t, resp.Runs[0].RunID, messages.Messages[0].RunID)
	require.Equal(t, "请追加行动建议", messages.Messages[0].Content)
	require.Equal(t, `{"source":"test_followup"}`, messages.Messages[0].Metadata)
}

func TestTaskThreadRunToAPIRedactsSubagentInternalPayloads(t *testing.T) {
	subagent := taskThreadRunToAPI(&appagentthread.RunSummary{
		RunID:       2001,
		ThreadID:    1,
		ParentRunID: 2,
		AssistantID: "singleagent:1001",
		RunKind:     appagentthread.RunKindSubagent,
		Status:      appagentthread.RunStatusFailed,
		Command:     `{"internal":"command"}`,
		Input:       `{"schema":"coze.subagent_tool_call.v1","arguments":{"secret":"do not expose"}}`,
		Config:      `{"agent_name":"researcher","prompt":"do not expose"}`,
		Context:     `{"checkpoint":"do not expose"}`,
		Metadata:    `{"subagent":{"name":"researcher"}}`,
		ErrorCode:   "subagent_failed",
	})

	require.NotNil(t, subagent)
	require.Equal(t, "subagent", subagent.RunKind)
	require.Empty(t, subagent.Command)
	require.Empty(t, subagent.Input)
	require.Empty(t, subagent.Config)
	require.Empty(t, subagent.Context)
	require.Equal(t, `{"subagent":{"name":"researcher"}}`, subagent.Metadata)
	require.Equal(t, "subagent_failed", subagent.ErrorCode)

	task := taskThreadRunToAPI(&appagentthread.RunSummary{
		RunID:   2,
		RunKind: appagentthread.RunKindTask,
		Command: `{"visible":"command"}`,
		Input:   `{"messages":[{"role":"user","content":"visible"}]}`,
		Config:  `{"runtime":"eino_adk"}`,
		Context: `{"plan_scope_run_id":2}`,
	})

	require.Empty(t, task.Command)
	require.Empty(t, task.Input)
	require.Empty(t, task.Config)
	require.Empty(t, task.Context)
}

func TestResumeTaskThreadRunHandlerCreatesQueuedResumeRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads/:thread_id/runs/:run_id/resume", ResumeTaskThreadRun)
	installAgentThreadTestService(t)
	sourceRunID := createInterruptedHumanInteractionRun(t)

	payload, err := json.Marshal(map[string]any{
		"interrupt_id":    "interrupt-1",
		"idempotency_key": "resume-api-key",
		"response": map[string]any{
			"schema":         "coze.human_interaction_response.v1",
			"interaction_id": "hi_1",
			"kind":           "clarification",
			"decision":       "answered",
			"answer":         "最近 7 天",
		},
	})
	require.NoError(t, err)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/runs/2/resume",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"status":"queued"`)
	require.NotContains(t, body, `"idempotency_key":"resume-api-key"`)

	resp, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Len(t, resp.Runs, 2)
	require.Equal(t, sourceRunID, resp.Runs[1].RunID)
	require.Equal(t, appagentthread.RunStatusQueued, resp.Runs[0].Status)
	require.Equal(t, "resume-api-key", resp.Runs[0].IdempotencyKey)
	require.Contains(t, resp.Runs[0].Command, `"interrupt-1"`)
	require.Contains(t, resp.Runs[0].Command, `"answer":"最近 7 天"`)
}

func TestResumeTaskThreadRunHandlerRejectsInvalidPayload(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads/:thread_id/runs/:run_id/resume", ResumeTaskThreadRun)
	installAgentThreadTestService(t)

	payload, err := json.Marshal(map[string]any{})
	require.NoError(t, err)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/runs/2/resume",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestResumeAndRetryTaskThreadRunForbiddenBeforeMutation(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST(
		"/api/workbench/task_threads/:thread_id/runs/:run_id/resume",
		workbenchSessionMiddlewareForTest(3),
		ResumeTaskThreadRun,
	)
	h.POST(
		"/api/workbench/task_threads/:thread_id/runs/:run_id/retry",
		workbenchSessionMiddlewareForTest(3),
		RetryTaskThreadSubagentRun,
	)
	installAgentThreadTestService(t)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{}`,
	})
	require.NoError(t, err)
	before, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 1,
		Page:     1,
	})
	require.NoError(t, err)
	resumePayload, err := json.Marshal(map[string]any{
		"interrupt_id": "interrupt-1",
		"response": map[string]any{
			"schema":         "coze.human_interaction_response.v1",
			"interaction_id": "hi_1",
			"kind":           "clarification",
			"decision":       "answered",
			"answer":         "no access",
		},
	})
	require.NoError(t, err)
	retryPayload := []byte(`{"idempotency_key":"must-not-persist"}`)
	runPath := "/api/workbench/task_threads/1/runs/" + strconv.FormatInt(runResp.Run.RunID, 10)

	resume := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		runPath+"/resume",
		&ut.Body{Body: bytes.NewBuffer(resumePayload), Len: len(resumePayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	retry := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		runPath+"/retry",
		&ut.Body{Body: bytes.NewBuffer(retryPayload), Len: len(retryPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	after, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 1,
		Page:     1,
	})
	require.NoError(t, err)

	require.Equal(t, http.StatusForbidden, resume.Code)
	require.Equal(t, http.StatusForbidden, retry.Code)
	require.Equal(t, before.Total, after.Total)
}

func TestCancelTaskThreadRunHandlerTransitionsRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads/:thread_id/runs/:run_id/cancel", CancelTaskThreadRun)
	installAgentThreadTestService(t)

	resp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		AssistantID: "lead-agent",
		Input:       `{"messages":[{"role":"user","content":"取消这次任务"}]}`,
		Config:      `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)
	runID := resp.Run.RunID
	claimed, err := appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	require.Len(t, claimed.Runs, 1)
	require.Equal(t, runID, claimed.Runs[0].RunID)
	require.Equal(t, appagentthread.RunStatusRunning, claimed.Runs[0].Status)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/runs/"+strconv.FormatInt(runID, 10)+"/cancel",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(runID, 10)+`"`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"status":"canceled"`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{
		RunID: runID,
	})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusCanceled, persisted.Run.Status)
}

func TestCancelTaskThreadRunHandlerCancelsPendingRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads/:thread_id/runs/:run_id/cancel", CancelTaskThreadRun)
	installAgentThreadTestService(t)

	resp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		AssistantID: "lead-agent",
		Input:       `{"messages":[{"role":"user","content":"取消排队任务"}]}`,
		Config:      `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)
	runID := resp.Run.RunID
	require.Equal(t, appagentthread.RunStatusPending, resp.Run.Status)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/runs/"+strconv.FormatInt(runID, 10)+"/cancel",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(runID, 10)+`"`)
	require.Contains(t, body, `"status":"canceled"`)

	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{
		RunID: runID,
	})
	require.NoError(t, err)
	require.Equal(t, appagentthread.RunStatusCanceled, persisted.Run.Status)
}

func TestRetryTaskThreadSubagentRunHandlerCreatesQueuedRetryRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.POST("/api/workbench/task_threads/:thread_id/runs/:run_id/retry", RetryTaskThreadSubagentRun)
	installAgentThreadTestService(t)

	parentResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		AssistantID: "lead-agent",
		Status:      appagentthread.RunStatusQueued,
		Input:       `{"messages":[{"role":"user","content":"拆解任务"}]}`,
		Config:      `{"runtime":"eino_adk"}`,
		Context:     `{"plan_scope_run_id":2}`,
	})
	require.NoError(t, err)
	childResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		ParentRunID: parentResp.Run.RunID,
		AssistantID: "singleagent:1001",
		RunKind:     appagentthread.RunKindSubagent,
		Status:      appagentthread.RunStatusRunning,
		Input:       `{"messages":[]}`,
		Metadata:    `{"subagent":{"name":"researcher"}}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.FailRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:        childResp.Run.RunID,
		From:         appagentthread.RunStatusRunning,
		To:           appagentthread.RunStatusFailed,
		ErrorCode:    "subagent_timeout",
		ErrorMessage: "context deadline exceeded",
	}))
	require.NoError(t, err)

	payload, err := json.Marshal(map[string]any{
		"idempotency_key": "retry-api-key",
	})
	require.NoError(t, err)
	w := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/task_threads/1/runs/"+strconv.FormatInt(childResp.Run.RunID, 10)+"/retry",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"parent_run_id":"0"`)
	require.Contains(t, body, `"run_kind":"task"`)
	require.Contains(t, body, `"status":"queued"`)
	require.NotContains(t, body, `"idempotency_key":"retry-api-key"`)
	require.Contains(t, body, `subagent_retry`)

	resp, err := appagentthread.SVC.ListRuns(context.Background(), &appagentthread.ListRunsRequest{
		ThreadID: 1,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), resp.Total)
	require.Equal(t, appagentthread.RunStatusQueued, resp.Runs[0].Status)
	require.Equal(t, "retry-api-key", resp.Runs[0].IdempotencyKey)
	require.Equal(t, appagentthread.RunKindTask, resp.Runs[0].RunKind)
	require.Zero(t, resp.Runs[0].ParentRunID)
	require.Contains(t, resp.Runs[0].Command, `"source_run_id":`+strconv.FormatInt(childResp.Run.RunID, 10))
}

func TestListTaskThreadRunsHandlerReturnsRuns(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/runs", ListTaskThreadRuns)
	installAgentThreadTestService(t)

	_, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"第一轮"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"第二轮"}]}`,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/runs?page=1&page_size=10", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.NotContains(t, body, `"content":"第二轮"`)
	require.NotContains(t, body, `"content":"第一轮"`)
}

func TestListTaskThreadRunsHandlerReturnsChildRunsForParentRun(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/runs", ListTaskThreadRuns)
	installAgentThreadTestService(t)

	parentResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"拆解任务"}]}`,
	})
	require.NoError(t, err)
	childResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		ParentRunID: parentResp.Run.RunID,
		AssistantID: "singleagent:1001",
		RunKind:     appagentthread.RunKindSubagent,
		Status:      appagentthread.RunStatusRunning,
		Input:       `{"messages":[]}`,
		Metadata:    `{"subagent":{"name":"researcher"}}`,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/runs?parent_run_id="+strconv.FormatInt(parentResp.Run.RunID, 10)+"&page=1&page_size=10",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(childResp.Run.RunID, 10)+`"`)
	require.Contains(t, body, `"parent_run_id":"`+strconv.FormatInt(parentResp.Run.RunID, 10)+`"`)
	require.Contains(t, body, `"run_kind":"subagent"`)
	require.Contains(t, body, `"status":"running"`)
	require.NotContains(t, body, `"content\":\"拆解任务"`)
}

func TestListTaskThreadRunEventsHandlerReturnsEvents(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/run_events", ListTaskThreadRunEvents)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"分析执行流程"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "run.started",
		Payload:   `{"status":"running","worker_id":"worker-a"}`,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/run_events?page=1&page_size=10", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":1`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"event_type":"run.started"`)
	require.Contains(t, body, `"payload":"{\"status\":\"running\"}"`)
	require.NotContains(t, body, "worker-a")
}

func TestListTaskThreadRunEventsHandlerRedactsUnsafePayload(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/run_events", ListTaskThreadRunEvents)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"执行工具"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "tool.completed",
		Payload: `{
			"role":"tool",
			"tool_name":"search_web",
			"tool_call_id":"call_123",
			"content":"tool result sk-secret https://private.example/signed",
			"reasoning_content":"hidden chain of thought",
			"tool_calls":[{"id":"call_123","function":{"name":"search_web","arguments":"{\"url\":\"s3://bucket/raw\"}"}}],
			"media":[{"url":"s3://bucket/raw.png"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5}
		}`,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/run_events?page=1&page_size=10", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Events []struct {
				EventType string `json:"event_type"`
				Payload   string `json:"payload"`
			} `json:"events"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Result().Body(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, int64(1), resp.Data.Total)
	require.Len(t, resp.Data.Events, 1)
	require.Equal(t, "tool.completed", resp.Data.Events[0].EventType)

	payload := resp.Data.Events[0].Payload
	require.JSONEq(t, `{"redacted":true,"role":"tool","tool_name":"search_web","tool_call_id":"call_123","result_present":true}`, payload)
	require.NotContains(t, payload, "tool result")
	require.NotContains(t, payload, "sk-secret")
	require.NotContains(t, payload, "private.example")
	require.NotContains(t, payload, "hidden chain of thought")
	require.NotContains(t, payload, "tool_calls")
	require.NotContains(t, payload, "arguments")
	require.NotContains(t, payload, "s3://bucket/raw")
	require.NotContains(t, payload, "prompt_tokens")
	require.NotContains(t, payload, "completion_tokens")
	require.NotContains(t, payload, "media")
}

func TestTaskThreadPublicMappersDoesNotExposeInternalRecords(t *testing.T) {
	const sensitive = "handler-sensitive-sentinel"

	run := taskThreadRunToAPI(&appagentthread.RunSummary{
		RunID:          1,
		ThreadID:       2,
		ParentRunID:    3,
		SpaceID:        4,
		CreatorID:      5,
		AssistantID:    "researcher",
		RunKind:        appagentthread.RunKindSubagent,
		Status:         appagentthread.RunStatusFailed,
		Command:        `{"resume":"` + sensitive + `"}`,
		Input:          `{"message":"` + sensitive + `"}`,
		Config:         `{"api_key":"` + sensitive + `"}`,
		Context:        `{"reasoning":"` + sensitive + `"}`,
		Metadata:       `{"source":"subagent_retry","source_run_id":3,"subagent":{"name":"researcher","prompt":"` + sensitive + `"}}`,
		IdempotencyKey: sensitive,
		WorkerID:       sensitive,
		ErrorCode:      "model_provider_error",
		ErrorMessage:   sensitive,
	})
	require.Empty(t, run.Command)
	require.Empty(t, run.Input)
	require.Empty(t, run.Config)
	require.Empty(t, run.Context)
	require.Empty(t, run.IdempotencyKey)
	require.Empty(t, run.WorkerID)
	require.Equal(t, "model_provider_error", run.ErrorCode)
	require.Equal(t, "Model request failed", run.ErrorMessage)
	require.NotContains(t, mustMarshalJSON(t, run), sensitive)

	message := taskThreadMessageToAPI(&appagentthread.MessageSummary{
		MessageID: 1,
		ThreadID:  2,
		RunID:     3,
		Role:      appagentthread.MessageRoleAssistant,
		Content:   "visible answer",
		Metadata:  `{"source":"human_interaction","interrupt_id":"interrupt-1","provider_body":"` + sensitive + `"}`,
	})
	require.Equal(t, "visible answer", message.Content)
	require.JSONEq(t, `{"source":"human_interaction","interrupt_id":"interrupt-1"}`, message.Metadata)
	require.NotContains(t, mustMarshalJSON(t, message), sensitive)
	require.Nil(t, taskThreadMessageToAPI(&appagentthread.MessageSummary{
		MessageID: 2,
		ThreadID:  2,
		RunID:     3,
		Role:      appagentthread.MessageRoleSystem,
		Content:   sensitive,
	}))

	artifact := taskThreadArtifactToAPI(&appagentthread.ArtifactSummary{
		ArtifactID:   1,
		ThreadID:     2,
		RunID:        3,
		FileID:       4,
		Title:        "report.md",
		ArtifactType: "markdown",
		VirtualPath:  "/mnt/user-data/outputs/report.md",
		ContentType:  "text/markdown",
		Metadata:     `{"scan_status":"clean","object_uri":"` + sensitive + `"}`,
	})
	require.Equal(t, "/mnt/user-data/outputs/report.md", artifact.VirtualPath)
	require.JSONEq(t, `{"scan_status":"clean"}`, artifact.Metadata)
	require.NotContains(t, mustMarshalJSON(t, artifact), sensitive)

	journal := taskThreadRunJournalMessageToAPI(&appagentthread.RunJournalMessage{
		ID:       "message-1",
		ThreadID: 2,
		RunID:    3,
		Type:     appagentthread.RunJournalMessageTypeAI,
		Role:     appagentthread.MessageRoleAssistant,
		Content:  "visible answer",
		ToolCalls: []appagentthread.RunJournalToolCall{
			{ID: "call-1", Name: "web_search", Type: "function", Args: map[string]any{"query": sensitive}},
		},
		AdditionalKwargs: map[string]any{"reasoning_content": sensitive},
		Usage:            map[string]any{"input_tokens": 10, "raw_usage": sensitive},
	})
	require.Equal(t, "visible answer", journal.Content)
	require.JSONEq(t, `{}`, journal.AdditionalKwargs)
	require.JSONEq(t, `{}`, journal.ToolCalls[0].Arguments)
	require.NotContains(t, mustMarshalJSON(t, journal), sensitive)

	unknownEvent := taskThreadRunEventToAPI(&appagentthread.RunEventSummary{
		EventID:   1,
		ThreadID:  2,
		RunID:     3,
		EventType: "provider.experimental",
		Payload:   `{"raw":"` + sensitive + `"}`,
	})
	require.Equal(t, `{}`, unknownEvent.Payload)
	require.NotContains(t, mustMarshalJSON(t, unknownEvent), sensitive)
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func TestTaskThreadRunEventPayloadRedactsAssistantToolCallInternals(t *testing.T) {
	arguments := `{"description":"创建武汉3日游攻略 Markdown 文档","file_path":"/mnt/user-data/workspace/武汉3日游攻略.md","content":"` + strings.Repeat("x", 5000) + `","url":"https://private.example/signed","api_key":"sk-secret"}`
	reasoning := "Let me create the document in the workspace first. " + strings.Repeat("Keep the visible step summary detailed. ", 8)
	payload := publicRunEventPayloadForTest("message.completed", `{
		"role":"assistant",
		"content":"approved visible assistant response",
		"reasoning_content":`+strconv.Quote(reasoning)+`,
		"tool_calls":[{
			"id":"call_write_file",
			"type":"function",
			"function":{
				"name":"write_file",
				"arguments":`+strconv.Quote(arguments)+`
			}
		}],
		"media":[{"url":"s3://bucket/raw.png"}],
		"usage":{"prompt_tokens":10,"completion_tokens":5}
	}`)

	var got struct {
		Redacted         bool   `json:"redacted"`
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content"`
		ToolCalls        []struct {
			ID               string `json:"id"`
			Name             string `json:"name"`
			ArgumentsPresent bool   `json:"arguments_present"`
		} `json:"tool_calls"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &got))
	require.True(t, got.Redacted)
	require.Equal(t, "assistant", got.Role)
	require.Equal(t, "approved visible assistant response", got.Content)
	require.Empty(t, got.ReasoningContent)
	require.Len(t, got.ToolCalls, 1)
	require.Equal(t, "call_write_file", got.ToolCalls[0].ID)
	require.Equal(t, "write_file", got.ToolCalls[0].Name)
	require.True(t, got.ToolCalls[0].ArgumentsPresent)
	require.NotContains(t, payload, "创建武汉3日游攻略")
	require.NotContains(t, payload, strings.Repeat("x", 100))
	require.NotContains(t, payload, "private.example")
	require.NotContains(t, payload, "sk-secret")
	require.NotContains(t, payload, "s3://bucket/raw")
	require.NotContains(t, payload, "prompt_tokens")
	require.NotContains(t, payload, "completion_tokens")
	require.NotContains(t, payload, "media")
}

func TestTaskThreadRunEventPayloadRedactsWebSearchArguments(t *testing.T) {
	arguments := `{"query":"青岛最佳旅游时间","max_results":5,"url":"https://private.example/signed","api_key":"sk-secret"}`
	payload := publicRunEventPayloadForTest("message.completed", `{
		"role":"assistant",
		"content":"approved visible assistant response",
		"tool_calls":[{
			"id":"call_web_search",
			"type":"function",
			"function":{
				"name":"web_search",
				"arguments":`+strconv.Quote(arguments)+`
			}
		}]
	}`)

	require.JSONEq(t, `{
		"redacted":true,
		"role":"assistant",
		"content":"approved visible assistant response",
		"tool_calls":[{
			"id":"call_web_search",
			"name":"web_search",
			"arguments_present":true
		}]
	}`, payload)
	require.NotContains(t, payload, "青岛最佳旅游时间")
	require.NotContains(t, payload, "private.example")
	require.NotContains(t, payload, "sk-secret")
	require.NotContains(t, payload, "max_results")
}

func TestTaskThreadRunEventPayloadRedactsSkillToolCallArguments(t *testing.T) {
	arguments := `{"skill":"skill-creator","url":"https://private.example/signed","api_key":"sk-secret"}`
	payload := publicRunEventPayloadForTest("message.completed", `{
		"role":"assistant",
		"content":"approved visible assistant response",
		"tool_calls":[{
			"id":"call_skill",
			"type":"function",
			"function":{
				"name":"skill",
				"arguments":`+strconv.Quote(arguments)+`
			}
		}]
	}`)

	require.JSONEq(t, `{
		"redacted":true,
		"role":"assistant",
		"content":"approved visible assistant response",
		"tool_calls":[{
			"id":"call_skill",
			"name":"skill",
			"arguments_present":true
		}]
	}`, payload)
	require.NotContains(t, payload, "skill-creator")
	require.NotContains(t, payload, "private.example")
	require.NotContains(t, payload, "sk-secret")
}

func TestListTaskThreadRunEventsHandlerReturnsJournalMessages(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/run_events", ListTaskThreadRunEvents)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input: `{
			"messages":[
				{"role":"user","content":"上一轮"},
				{"role":"assistant","content":"上一轮回答"},
				{"role":"user","content":"青岛最佳旅游时间"}
			]
		}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
		Role:     appagentthread.MessageRoleUser,
		Content:  "青岛最佳旅游时间",
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "message.completed",
		Payload: `{
			"role":"assistant",
			"content":"approved visible assistant response",
			"reasoning_content":"需要查询季节和天气资料。",
			"tool_calls":[{
				"id":"call_web_search",
				"type":"function",
				"function":{
					"name":"web_search",
					"arguments":"{\"query\":\"青岛最佳旅游时间\",\"url\":\"https://private.example/signed\",\"api_key\":\"sk-secret\"}"
				}
			}],
			"usage":{"prompt_tokens":10,"completion_tokens":5}
		}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID:  1,
		RunID:     runResp.Run.RunID,
		EventType: "tool.completed",
		Payload: `{
			"role":"tool",
			"tool_name":"web_search",
			"tool_call_id":"call_web_search",
			"content":"tool result sk-secret https://private.example/signed"
		}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
		Role:     appagentthread.MessageRoleAssistant,
		Content:  "青岛 4-6 月和 9-10 月最适合旅行。",
	})
	require.NoError(t, err)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/run_events?page=1&page_size=10", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			JournalMessages []struct {
				ID               string `json:"id"`
				Type             string `json:"type"`
				Content          string `json:"content"`
				AdditionalKwargs string `json:"additional_kwargs"`
				Usage            string `json:"usage"`
				ToolCallID       string `json:"tool_call_id"`
				ToolCalls        []struct {
					ID        string `json:"id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"tool_calls"`
			} `json:"journal_messages"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Result().Body(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.JournalMessages, 4)
	require.Equal(t, "human", resp.Data.JournalMessages[0].Type)
	require.Equal(t, "青岛最佳旅游时间", resp.Data.JournalMessages[0].Content)
	require.Equal(t, "ai", resp.Data.JournalMessages[1].Type)
	require.Equal(t, "approved visible assistant response", resp.Data.JournalMessages[1].Content)
	require.JSONEq(t, `{}`, resp.Data.JournalMessages[1].AdditionalKwargs)
	require.JSONEq(t, `{}`, resp.Data.JournalMessages[1].Usage)
	require.Len(t, resp.Data.JournalMessages[1].ToolCalls, 1)
	require.Equal(t, "call_web_search", resp.Data.JournalMessages[1].ToolCalls[0].ID)
	require.Equal(t, "web_search", resp.Data.JournalMessages[1].ToolCalls[0].Name)
	require.JSONEq(t, `{}`, resp.Data.JournalMessages[1].ToolCalls[0].Arguments)
	require.Equal(t, "tool", resp.Data.JournalMessages[2].Type)
	require.Equal(t, "call_web_search", resp.Data.JournalMessages[2].ToolCallID)
	require.Empty(t, resp.Data.JournalMessages[2].Content)
	require.Equal(t, "ai", resp.Data.JournalMessages[3].Type)
	require.Equal(t, "青岛 4-6 月和 9-10 月最适合旅行。", resp.Data.JournalMessages[3].Content)

	body := string(w.Result().Body())
	require.Contains(t, body, "approved visible assistant response")
	require.NotContains(t, body, "需要查询季节和天气资料")
	require.NotContains(t, body, "tool result")
	require.NotContains(t, body, "private.example")
	require.NotContains(t, body, "sk-secret")
	require.NotContains(t, body, "prompt_tokens")
	require.NotContains(t, body, "completion_tokens")
}

func TestGetTaskThreadTokenUsageHandlerReturnsRowsAndAggregate(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/token_usage", GetTaskThreadTokenUsage)
	installAgentThreadTestService(t)

	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"统计 token"}]}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordTokenUsage(context.Background(), &appagentthread.RecordTokenUsageRequest{
		RunID:        runResp.Run.RunID,
		Source:       appagentthread.TokenUsageSourceLeadAgent,
		StepName:     "generate_answer",
		ModelName:    "gpt-4.1",
		Provider:     "openai",
		InputTokens:  12,
		OutputTokens: 8,
		TotalTokens:  20,
		RawUsage:     `{"prompt_tokens":12,"completion_tokens":8}`,
		Metadata:     `{"provider_raw":"sk-secret","prompt":"raw prompt","tool_args":"{\"url\":\"s3://bucket/raw\"}"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordTokenUsage(context.Background(), &appagentthread.RecordTokenUsageRequest{
		RunID:        runResp.Run.RunID,
		Source:       appagentthread.TokenUsageSourceTool,
		StepName:     "call_tool",
		InputTokens:  4,
		OutputTokens: 6,
		TotalTokens:  10,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/token_usage?run_id=2&page=1&page_size=10",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"run_id":"2"`)
	require.Contains(t, body, `"source":"lead_agent"`)
	require.Contains(t, body, `"source":"tool"`)
	require.Contains(t, body, `"input_tokens":16`)
	require.Contains(t, body, `"output_tokens":14`)
	require.Contains(t, body, `"total_tokens":30`)
	require.Contains(t, body, `"call_count":2`)
	require.Contains(t, body, `"lead_agent_tokens":20`)
	require.Contains(t, body, `"tool_tokens":10`)
	require.NotContains(t, body, "prompt_tokens")
	require.NotContains(t, body, "completion_tokens")
	require.NotContains(t, body, "provider_raw")
	require.NotContains(t, body, "sk-secret")
	require.NotContains(t, body, "raw prompt")
	require.NotContains(t, body, "tool_args")
	require.NotContains(t, body, "s3://bucket/raw")
}

func TestGetTaskThreadTokenUsageHandlerCanIncludeChildRuns(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads/:thread_id/token_usage", GetTaskThreadTokenUsage)
	installAgentThreadTestService(t)

	parentResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"拆解任务"}]}`,
	})
	require.NoError(t, err)
	childResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		ParentRunID: parentResp.Run.RunID,
		AssistantID: "singleagent:1001",
		RunKind:     appagentthread.RunKindSubagent,
		Input:       `{"messages":[]}`,
		Metadata:    `{"subagent":{"name":"researcher"}}`,
	})
	require.NoError(t, err)
	siblingResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{"messages":[{"role":"user","content":"独立任务"}]}`,
	})
	require.NoError(t, err)

	_, err = appagentthread.SVC.RecordTokenUsage(context.Background(), &appagentthread.RecordTokenUsageRequest{
		RunID:       parentResp.Run.RunID,
		Source:      appagentthread.TokenUsageSourceLeadAgent,
		StepName:    "lead",
		TotalTokens: 20,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordTokenUsage(context.Background(), &appagentthread.RecordTokenUsageRequest{
		RunID:       childResp.Run.RunID,
		Source:      appagentthread.TokenUsageSourceSubagent,
		StepName:    "child",
		TotalTokens: 10,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.RecordTokenUsage(context.Background(), &appagentthread.RecordTokenUsageRequest{
		RunID:       siblingResp.Run.RunID,
		Source:      appagentthread.TokenUsageSourceLeadAgent,
		StepName:    "sibling",
		TotalTokens: 99,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/token_usage?run_id="+strconv.FormatInt(parentResp.Run.RunID, 10)+"&include_child_runs=true&page=1&page_size=10",
		nil,
	)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"total":2`)
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(parentResp.Run.RunID, 10)+`"`)
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(childResp.Run.RunID, 10)+`"`)
	require.NotContains(t, body, `"run_id":"`+strconv.FormatInt(siblingResp.Run.RunID, 10)+`"`)
	require.Contains(t, body, `"total_tokens":30`)
	require.Contains(t, body, `"call_count":2`)
	require.Contains(t, body, `"lead_agent_tokens":20`)
	require.Contains(t, body, `"subagent_tokens":10`)
	require.Contains(t, body, `"run_aggregates"`)
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(parentResp.Run.RunID, 10)+`","aggregate":{"input_tokens":0,"output_tokens":0,"total_tokens":20`)
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(childResp.Run.RunID, 10)+`","aggregate":{"input_tokens":0,"output_tokens":0,"total_tokens":10`)
	require.NotContains(t, body, `"run_id":"`+strconv.FormatInt(siblingResp.Run.RunID, 10)+`","aggregate"`)
}

func TestStreamTaskThreadRunEventsWritesEventsAndDone(t *testing.T) {
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

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamTaskThreadRunEvents(context.Background(), writer, threadapi.StreamTaskThreadRunEventsRequest{
		ThreadID:   1,
		RunID:      runResp.Run.RunID,
		IntervalMs: 10,
		TimeoutMs:  100,
	})
	body := writer.String()

	require.Contains(t, body, "event: run.event")
	require.Contains(t, body, `data: {"event_id":"3","thread_id":"1","run_id":"2","event_type":"step.completed"`)
	require.Contains(t, body, "event: done")
}

func TestTaskThreadRunEventStreamErrorDoesNotExposeInternalDetails(t *testing.T) {
	writer := &recordingTaskThreadRunEventStreamWriter{}
	writeTaskThreadRunEventStreamError(
		context.Background(),
		writer,
		errors.New("provider failed with credential sensitive-runtime-sentinel"),
	)

	body := writer.String()
	require.Contains(t, body, "event: error")
	require.Contains(t, body, `"code":"runtime_stream_error"`)
	require.Contains(t, body, `"message":"Model request failed"`)
	require.NotContains(t, body, "credential")
	require.NotContains(t, body, "sensitive-runtime-sentinel")
}

func TestStreamTaskThreadRunEventsReturnsForbiddenBeforeSSEHeaders(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET(
		"/api/workbench/task_threads/:thread_id/run_events/stream",
		workbenchSessionMiddlewareForTest(3),
		StreamTaskThreadRunEvents,
	)
	installAgentThreadTestService(t)
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{}`,
	})
	require.NoError(t, err)

	w := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/task_threads/1/run_events/stream?run_id="+
			strconv.FormatInt(runResp.Run.RunID, 10)+"&timeout_ms=1",
		nil,
	)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.NotContains(t, w.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, string(w.Result().Body()), "thread access denied")
}

func TestStreamTaskThreadRunEventsReusesAuthorizedRequestScope(t *testing.T) {
	installAgentThreadTestService(t)
	workspaceAuthorizer := &countingWorkbenchWorkspaceAuthorizer{}
	appagentthread.SVC.WorkspaceAuthorizer = workspaceAuthorizer
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: 1,
		Input:    `{}`,
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
	ctx := appagentthread.WithThreadAccessRequest(context.Background(), appagentthread.ThreadAccessRequest{
		ViewerID: 2,
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
	})
	require.NoError(t, appagentthread.SVC.AuthorizeThreadAccess(ctx, appagentthread.ThreadAccessRequest{
		ViewerID: 2,
		ThreadID: 1,
		RunID:    runResp.Run.RunID,
	}))

	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamTaskThreadRunEvents(ctx, writer, threadapi.StreamTaskThreadRunEventsRequest{
		ThreadID:   1,
		RunID:      runResp.Run.RunID,
		IntervalMs: 10,
		TimeoutMs:  100,
	})

	require.Contains(t, writer.String(), "event: done")
	require.Equal(t, 1, workspaceAuthorizer.calls)
}

func TestListTaskThreadsHandlerRejectsInvalidQuery(t *testing.T) {
	h := authenticatedAgentThreadTestServer()
	h.GET("/api/workbench/task_threads", ListTaskThreads)
	installAgentThreadTestService(t)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads?space_id=bad", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func authenticatedAgentThreadTestServer() *server.Hertz {
	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(2))
	return h
}

func installAgentThreadTestService(t *testing.T) {
	t.Helper()
	prevThreadSVC := appagentthread.SVC.ThreadSVC
	prevThreadAuthorizer := appagentthread.SVC.ThreadAuthorizer
	prevWorkspaceAuthorizer := appagentthread.SVC.WorkspaceAuthorizer
	prevRuntimeFileSVC := appagentthread.SVC.RuntimeFileSVC
	prevPlanSVC := appagentthread.SVC.PlanSVC
	prevArtifactSVC := appagentthread.SVC.ArtifactSVC
	prevArtifactObjectStorage := appagentthread.SVC.ArtifactObjectStorage
	prevArtifactAuthorizer := appagentthread.SVC.ArtifactAuthorizer
	prevMemoryAuthorizer := appagentthread.SVC.MemoryAuthorizer
	prevGuardrailAuditRepository := appagentthread.SVC.GuardrailAuditRepository
	prevGuardrailAuditAuthorizer := appagentthread.SVC.GuardrailAuditAuthorizer
	prevMCPRuntimeAuditRepository := appagentthread.SVC.MCPRuntimeAuditRepository
	prevMCPRuntimeAuditAuthorizer := appagentthread.SVC.MCPRuntimeAuditAuthorizer
	prevArtifactScannerStatus := appagentthread.SVC.ArtifactScannerStatus
	prevArtifactReviewClock := appagentthread.SVC.ArtifactReviewClock
	t.Cleanup(func() {
		appagentthread.SVC.ThreadSVC = prevThreadSVC
		appagentthread.SVC.ThreadAuthorizer = prevThreadAuthorizer
		appagentthread.SVC.WorkspaceAuthorizer = prevWorkspaceAuthorizer
		appagentthread.SVC.RuntimeFileSVC = prevRuntimeFileSVC
		appagentthread.SVC.PlanSVC = prevPlanSVC
		appagentthread.SVC.ArtifactSVC = prevArtifactSVC
		appagentthread.SVC.ArtifactObjectStorage = prevArtifactObjectStorage
		appagentthread.SVC.ArtifactAuthorizer = prevArtifactAuthorizer
		appagentthread.SVC.MemoryAuthorizer = prevMemoryAuthorizer
		appagentthread.SVC.GuardrailAuditRepository = prevGuardrailAuditRepository
		appagentthread.SVC.GuardrailAuditAuthorizer = prevGuardrailAuditAuthorizer
		appagentthread.SVC.MCPRuntimeAuditRepository = prevMCPRuntimeAuditRepository
		appagentthread.SVC.MCPRuntimeAuditAuthorizer = prevMCPRuntimeAuditAuthorizer
		appagentthread.SVC.ArtifactScannerStatus = prevArtifactScannerStatus
		appagentthread.SVC.ArtifactReviewClock = prevArtifactReviewClock
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadHandlerTableForTest(db))
	appagentthread.InitService(&appagentthread.ServiceComponents{DB: db, IDGen: &sequentialIDGen{next: 1}})
	appagentthread.SVC.WorkspaceAuthorizer = allowWorkbenchWorkspaceAuthorizer{}
	appagentthread.SVC.ArtifactAuthorizer = nil
	appagentthread.SVC.MemoryAuthorizer = nil
	_, err = appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:      1,
		UserID:       2,
		Title:        "任务列表",
		Source:       appagentthread.ThreadSourceWeb,
		LegacyTaskID: 100,
		Metadata:     `{"message":"hello"}`,
	})
	require.NoError(t, err)
}

type allowWorkbenchWorkspaceAuthorizer struct{}

func (allowWorkbenchWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	context.Context,
	appagentthread.WorkspaceAccessRequest,
) error {
	return nil
}

type recordingWorkbenchWorkspaceAuthorizer struct {
	req appagentthread.WorkspaceAccessRequest
	err error
}

func (a *recordingWorkbenchWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	_ context.Context,
	req appagentthread.WorkspaceAccessRequest,
) error {
	a.req = req
	return a.err
}

type countingWorkbenchWorkspaceAuthorizer struct {
	calls int
}

func (a *countingWorkbenchWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	context.Context,
	appagentthread.WorkspaceAccessRequest,
) error {
	a.calls++
	return nil
}

func workbenchSessionMiddlewareForTest(userID int64) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		ctx = ctxcache.Init(ctx)
		ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &userentity.Session{
			UserID: userID,
		})
		c.Next(ctx)
	}
}

type recordingWorkbenchArtifactAuthorizer struct {
	req appagentthread.ArtifactAccessRequest
	err error
}

func (a *recordingWorkbenchArtifactAuthorizer) AuthorizeArtifactAccess(
	_ context.Context,
	req appagentthread.ArtifactAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingWorkbenchMemoryAuthorizer struct {
	req appagentthread.MemoryAccessRequest
	err error
}

func (a *recordingWorkbenchMemoryAuthorizer) AuthorizeMemoryAccess(
	_ context.Context,
	req appagentthread.MemoryAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingWorkbenchGuardrailAuditAuthorizer struct {
	req appagentthread.GuardrailAuditAccessRequest
	err error
}

func (a *recordingWorkbenchGuardrailAuditAuthorizer) AuthorizeGuardrailAuditAccess(
	_ context.Context,
	req appagentthread.GuardrailAuditAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingWorkbenchMCPRuntimeAuditAuthorizer struct {
	req appagentthread.MCPRuntimeAuditAccessRequest
	err error
}

func (a *recordingWorkbenchMCPRuntimeAuditAuthorizer) AuthorizeMCPRuntimeAuditAccess(
	_ context.Context,
	req appagentthread.MCPRuntimeAuditAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingWorkbenchArtifactStorage struct {
	objects                map[string][]byte
	signedURL              string
	signKey                string
	signExpire             int64
	signContentDisposition string
	signContentType        string
}

func (s *recordingWorkbenchArtifactStorage) GetObject(
	_ context.Context,
	objectKey string,
) ([]byte, error) {
	content := s.objects[objectKey]
	return append([]byte(nil), content...), nil
}

func (s *recordingWorkbenchArtifactStorage) GetObjectUrl(
	_ context.Context,
	objectKey string,
	opts ...storage.GetOptFn,
) (string, error) {
	s.signKey = objectKey
	option := storage.GetOption{}
	for _, opt := range opts {
		opt(&option)
	}
	s.signExpire = option.Expire
	s.signContentDisposition = option.ResponseContentDisposition
	s.signContentType = option.ResponseContentType
	return s.signedURL, nil
}

func createInterruptedHumanInteractionRun(t *testing.T) int64 {
	t.Helper()
	runID, _ := createInterruptedHumanInteractionRunWithCheckpoint(t)

	return runID
}

func fencedRunStatusRequestForTest(
	t *testing.T,
	req *appagentthread.UpdateRunStatusRequest,
) *appagentthread.UpdateRunStatusRequest {
	t.Helper()
	require.NotNil(t, req)
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: req.RunID})
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Run)
	req.LeaseOwner = persisted.Run.LeaseOwner
	req.LeaseToken = persisted.Run.LeaseToken
	req.ExecutionGeneration = persisted.Run.ExecutionGeneration
	return req
}

func createInterruptedHumanInteractionRunWithCheckpoint(t *testing.T) (int64, int64) {
	t.Helper()
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		AssistantID: "assistant-a",
		Input:       `{"messages":[{"role":"user","content":"请分析周报"}]}`,
		Config:      `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.InterruptRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	}))
	require.NoError(t, err)

	envelope := appagentthread.ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         "eino_adk",
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1},
		Interrupts: map[string]appagentthread.ADKInterruptItem{
			"interrupt-1": {
				ID:          "interrupt-1",
				Address:     "lead/tool/ask_user_clarification",
				IsRootCause: true,
				Info: appagentthread.HumanInteractionPrompt{
					Schema:        "coze.human_interaction.v1",
					InteractionID: "hi_1",
					Kind:          appagentthread.HumanInteractionKindClarification,
					Question:      "请选择时间范围",
					Required:      true,
					AllowFreeText: true,
				},
			},
		},
	}
	raw, err := envelope.Marshal()
	require.NoError(t, err)
	checkpointResp, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "eino.adk",
		RuntimeType:     "eino_adk",
		RuntimeKey:      "checkpoint-1",
		EnvelopeVersion: 1,
		ChannelValues:   string(raw),
		ChannelVersions: `{}`,
		PendingSends:    `[]`,
		Metadata:        `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)

	return runResp.Run.RunID, checkpointResp.Checkpoint.CheckpointID
}

func migrateAgentThreadHandlerTableForTest(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE agent_threads (
			id integer PRIMARY KEY,
			space_id integer,
			creator_id integer,
			agent_id integer,
			title text,
			status text,
			source text,
			legacy_task_id integer,
			metadata json,
			created_at integer,
			updated_at integer,
			last_message_at integer
		);
		CREATE TABLE agent_thread_messages (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			role text,
			content text,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_runs (
			id integer PRIMARY KEY,
			thread_id integer,
			parent_run_id integer DEFAULT 0,
			space_id integer,
			creator_id integer,
			assistant_id text,
			run_kind text DEFAULT 'task',
			status text,
			command json,
			input json,
			config json,
			context json,
			metadata json,
			stream_mode json,
			multitask_strategy text,
			on_disconnect text,
			durability text,
			idempotency_key text,
			worker_id text,
			lease_owner text,
			lease_token text,
			lease_expires_at integer,
			heartbeat_at integer,
			cancel_requested_at integer,
			execution_generation integer NOT NULL DEFAULT 0,
			error_code text,
			error_message text,
			started_at integer,
			ended_at integer,
			created_at integer,
			updated_at integer
		);
		CREATE TABLE agent_run_events (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			event_type text,
			payload json,
			created_at integer
		);
		CREATE TABLE agent_checkpoints (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			parent_checkpoint_id integer,
			checkpoint_ns text,
			runtime_type text DEFAULT 'legacy',
			runtime_key text DEFAULT '',
			envelope_version integer DEFAULT 0,
			runtime_deleted_at integer DEFAULT 0,
			channel_values json,
			channel_versions json,
			pending_sends json,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_thread_memories (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			scope text,
			content text,
			metadata json,
			score real,
			confidence real,
			source_type text,
			source_id text,
			correction_of_memory_id integer,
			corrected_at integer,
			expires_at integer,
			created_at integer,
			updated_at integer,
			deleted_at integer DEFAULT 0
		);
		CREATE TABLE agent_memory_audit_events (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			memory_id integer,
			actor_id integer,
			event_type text,
			scope text,
			source_type text,
			source_id text,
			affected_count integer,
			created_at integer
		);
		CREATE TABLE agent_guardrail_audit_events (
			id integer PRIMARY KEY,
			space_id integer,
			thread_id integer,
			run_id integer,
			actor_id integer,
			event_type text,
			target_type text,
			target_id text,
			operation text,
			source text,
			action text,
			fail_mode text,
			provider text,
			reason_code text,
			rule_ids text,
			created_at integer
		);
		CREATE TABLE agent_mcp_runtime_audit_events (
			id integer PRIMARY KEY,
			space_id integer,
			thread_id integer,
			run_id integer,
			server_id integer,
			runtime_tool_name text,
			event_type text,
			error_code text,
			elapsed_ms integer,
			output_bytes integer,
			created_at integer
		);
		CREATE TABLE agent_token_usage (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			source text,
			step_id text,
			step_index integer,
			step_name text,
			model_name text,
			provider text,
			input_tokens integer,
			output_tokens integer,
			total_tokens integer,
			cost_micros integer,
			currency text,
			estimated boolean,
			raw_usage json,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_files (
			id integer PRIMARY KEY,
			space_id integer,
			user_id integer,
			thread_id integer,
			run_id integer,
			file_name text,
			original_file_name text DEFAULT '',
				file_kind text,
				virtual_path text,
				virtual_path_hash text DEFAULT '',
				object_uri text,
				content_type text DEFAULT '',
			size_bytes integer DEFAULT 0,
			digest text DEFAULT '',
			status text DEFAULT 'active',
			metadata json,
			created_at integer,
			updated_at integer,
				UNIQUE (run_id, virtual_path_hash)
			);
		CREATE TABLE agent_artifacts (
			id integer PRIMARY KEY,
			space_id integer,
			user_id integer,
			thread_id integer,
			run_id integer,
			file_id integer UNIQUE,
			title text DEFAULT '',
			artifact_type text,
			virtual_path text,
			object_uri text,
			content_type text DEFAULT '',
			size_bytes integer DEFAULT 0,
			preview_mode text DEFAULT 'download',
			metadata json,
			created_at integer,
			updated_at integer,
			deleted_at integer DEFAULT 0
		);
		CREATE TABLE agent_artifact_scan_jobs (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			user_id integer,
			artifact_id integer,
			file_id integer,
			scanner text,
			idempotency_key text,
			status text,
			worker_id text DEFAULT '',
			attempt_count integer DEFAULT 0,
			last_error text DEFAULT '',
			available_at integer DEFAULT 0,
			lease_expires_at integer DEFAULT 0,
			started_at integer DEFAULT 0,
			ended_at integer DEFAULT 0,
			created_at integer,
			updated_at integer,
			UNIQUE (artifact_id, idempotency_key)
		);
		CREATE TABLE agent_transcript_snapshots (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			kind text,
			digest text,
			idempotency_key text,
			message_count integer,
			messages json,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_memory_flush_jobs (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			user_id integer,
			assistant_id text,
			transcript_snapshot_id integer,
			idempotency_key text,
			status text,
			attempt_count integer,
			worker_id text,
			last_error text,
			available_at integer,
			lease_expires_at integer,
			started_at integer,
			ended_at integer,
			created_at integer,
			updated_at integer
		);
		CREATE TABLE agent_run_plans (
			run_id integer PRIMARY KEY,
			thread_id integer,
			space_id integer,
			user_id integer,
			high_watermark integer,
			revision integer,
			created_at integer,
			updated_at integer
		);
		CREATE TABLE agent_run_plan_items (
			id integer PRIMARY KEY,
			run_id integer,
			task_id integer,
			subject text,
			description text,
			status text,
			active_form text,
			owner text,
			blocks json,
			blocked_by json,
			metadata json,
			active boolean,
			version integer,
			created_at integer,
			updated_at integer
		)
	`).Error
}

type sequentialIDGen struct {
	mu   sync.Mutex
	next int64
}

type recordingTaskThreadRunEventStreamWriter struct {
	buffer bytes.Buffer
}

func publicRunEventPayloadForTest(eventType, payload string) string {
	projected := appagentthread.ProjectPublicRunEvent(&appagentthread.RunEventSummary{
		EventType: eventType,
		Payload:   payload,
	})
	if projected == nil {
		return `{}`
	}
	return projected.Payload
}

func (w *recordingTaskThreadRunEventStreamWriter) WriteEvent(id, eventType string, data []byte) error {
	if id != "" {
		w.buffer.WriteString("id: ")
		w.buffer.WriteString(id)
		w.buffer.WriteByte('\n')
	}
	if eventType != "" {
		w.buffer.WriteString("event: ")
		w.buffer.WriteString(eventType)
		w.buffer.WriteByte('\n')
	}
	if len(data) > 0 {
		w.buffer.WriteString("data: ")
		w.buffer.Write(data)
		w.buffer.WriteByte('\n')
	}
	w.buffer.WriteByte('\n')

	return nil
}

func (w *recordingTaskThreadRunEventStreamWriter) String() string {
	return w.buffer.String()
}

func (g *sequentialIDGen) GenID(ctx context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.next <= 0 {
		g.next = 2
		return 1, nil
	}

	id := g.next
	g.next++
	return id, nil
}

func (g *sequentialIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}

	return ids, nil
}

func TestTaskThreadRunEventStreamStopsForInterruptedRun(t *testing.T) {
	require.True(t, isTaskThreadRunTerminal(appagentthread.RunStatusInterrupted))
}
