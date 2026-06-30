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
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"

	threadapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

const (
	taskThreadRunEventStreamEvent   = "run.event"
	taskThreadRunEventStreamDone    = "done"
	taskThreadRunEventStreamError   = "error"
	defaultRunEventStreamIntervalMs = int64(1000)
	defaultRunEventStreamTimeoutMs  = int64(30000)
	minRunEventStreamIntervalMs     = int64(10)
	maxRunEventStreamIntervalMs     = int64(5000)
	minRunEventStreamTimeoutMs      = int64(1)
	maxRunEventStreamTimeoutMs      = int64(60000)
	maxSafeRunEventPayloadStringLen = 128
	maxSafeRunEventReasoningLen     = 2048
	maxSafeRunEventArgumentsJSONLen = 1024 * 1024
	maxSafeRunEventToolCalls        = 8
)

var unsafeRunEventDisplayPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|authorization|bearer|credential|secret|password|provider_raw|object[_-]?key|checkpoint|https?://|file://|s3://|oss://|cos://|minio://)`)

type taskThreadRunEventStreamWriter interface {
	WriteEvent(id, eventType string, data []byte) error
}

// ListTaskThreads .
// @router /api/workbench/task_threads [GET]
func ListTaskThreads(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	var status *appagentthread.ThreadStatus
	if req.Status != "" {
		mapped := appagentthread.ThreadStatus(req.Status)
		status = &mapped
	}
	resp, err := appagentthread.SVC.ListThreads(ctx, &appagentthread.ListThreadsRequest{
		SpaceID:  req.SpaceID,
		Status:   status,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadsData{
			Threads: taskThreadsToAPI(resp.Threads),
			Total:   resp.Total,
		},
	})
}

// CreateTaskThread .
// @router /api/workbench/task_threads [POST]
func CreateTaskThread(ctx context.Context, c *app.RequestContext) {
	var req threadapi.CreateTaskThreadRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.CreateTaskThread(ctx, &appagentthread.CreateTaskThreadRequest{
		SpaceID:           req.SpaceID,
		UserID:            workbenchViewerIDFromCtx(ctx),
		Message:           req.Message,
		Title:             req.Title,
		AssistantID:       req.AssistantID,
		Command:           req.Command,
		Config:            req.Config,
		Context:           req.Context,
		Metadata:          req.Metadata,
		StreamMode:        req.StreamMode,
		MultitaskStrategy: req.MultitaskStrategy,
		OnDisconnect:      req.OnDisconnect,
		Durability:        req.Durability,
		IdempotencyKey:    req.IdempotencyKey,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.CreateTaskThreadResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.CreateTaskThreadData{
			Thread:  taskThreadToAPI(resp.Thread),
			Message: taskThreadMessageToAPI(resp.Message),
			Run:     taskThreadRunToAPI(resp.Run),
		},
	})
}

// GetTaskThread .
// @router /api/workbench/task_threads/:thread_id [GET]
func GetTaskThread(ctx context.Context, c *app.RequestContext) {
	var req threadapi.GetTaskThreadRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.GetTaskThreadResponse{
		Code: 0,
		Msg:  "success",
		Data: taskThreadToAPI(resp.Thread),
	})
}

// ListTaskThreadMessages .
// @router /api/workbench/task_threads/:thread_id/messages [GET]
func ListTaskThreadMessages(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadMessagesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ListMessages(ctx, &appagentthread.ListMessagesRequest{
		ThreadID: req.ThreadID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadMessagesResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadMessagesData{
			Messages: taskThreadMessagesToAPI(resp.Messages),
			Total:    resp.Total,
		},
	})
}

// AppendTaskThreadMessage .
// @router /api/workbench/task_threads/:thread_id/messages [POST]
func AppendTaskThreadMessage(ctx context.Context, c *app.RequestContext) {
	var req threadapi.AppendTaskThreadMessageRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.AppendMessage(ctx, &appagentthread.AppendMessageRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Role:     appagentthread.MessageRole(req.Role),
		Content:  req.Content,
		Metadata: req.Metadata,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.AppendTaskThreadMessageResponse{
		Code: 0,
		Msg:  "success",
		Data: taskThreadMessageToAPI(resp.Message),
	})
}

// ListTaskThreadRuns .
// @router /api/workbench/task_threads/:thread_id/runs [GET]
func ListTaskThreadRuns(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadRunsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	var status *appagentthread.RunStatus
	if req.Status != "" {
		mapped := appagentthread.RunStatus(req.Status)
		status = &mapped
	}
	var parentRunID *int64
	if req.ParentRunID > 0 {
		parentRunID = &req.ParentRunID
	}
	resp, err := appagentthread.SVC.ListRuns(ctx, &appagentthread.ListRunsRequest{
		ThreadID:    req.ThreadID,
		ParentRunID: parentRunID,
		Status:      status,
		Page:        req.Page,
		PageSize:    req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadRunsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadRunsData{
			Runs:  taskThreadRunsToAPI(resp.Runs),
			Total: resp.Total,
		},
	})
}

// ListTaskThreadRunEvents .
// @router /api/workbench/task_threads/:thread_id/run_events [GET]
func ListTaskThreadRunEvents(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadRunEventsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadRunEventsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadRunEventsData{
			Events: taskThreadRunEventsToAPI(resp.Events),
			Total:  resp.Total,
		},
	})
}

// GetTaskThreadTokenUsage .
// @router /api/workbench/task_threads/:thread_id/token_usage [GET]
func GetTaskThreadTokenUsage(ctx context.Context, c *app.RequestContext) {
	var req threadapi.GetTaskThreadTokenUsageRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	usageReq := &appagentthread.GetTokenUsageRequest{
		ThreadID:         req.ThreadID,
		RunID:            req.RunID,
		IncludeChildRuns: req.IncludeChildRuns,
		Source:           appagentthread.TokenUsageSource(req.Source),
		Page:             req.Page,
		PageSize:         req.PageSize,
	}

	var resp *appagentthread.GetTokenUsageResponse
	var err error
	if req.RunID > 0 {
		runResp, runErr := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: req.RunID})
		if runErr != nil {
			workbenchThreadErrorResponse(ctx, c, runErr)
			return
		}
		if runResp == nil || runResp.Run == nil || runResp.Run.ThreadID != req.ThreadID {
			invalidParamRequestResponse(c, "run_id does not belong to thread_id")
			return
		}

		resp, err = appagentthread.SVC.GetRunTokenUsage(ctx, usageReq)
	} else {
		resp, err = appagentthread.SVC.GetThreadTokenUsage(ctx, usageReq)
	}
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.GetTaskThreadTokenUsageResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.GetTaskThreadTokenUsageData{
			Usage:         taskThreadTokenUsagesToAPI(resp.Usage),
			Total:         resp.Total,
			Aggregate:     taskThreadTokenUsageAggregateToAPI(resp.Aggregate),
			RunAggregates: taskThreadTokenUsageRunAggregatesToAPI(resp.RunAggregates),
		},
	})
}

// ListTaskThreadMemories .
// @router /api/workbench/task_threads/:thread_id/memories [GET]
func ListTaskThreadMemories(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadMemoriesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		req.Query = string(c.Query("query"))
	}

	resp, err := appagentthread.SVC.ListMemories(ctx, &appagentthread.ListMemoriesRequest{
		ThreadID:       req.ThreadID,
		ViewerID:       workbenchViewerIDFromCtx(ctx),
		RunID:          req.RunID,
		Scopes:         taskThreadMemoryScopes(req.Scope, req.Scopes),
		Query:          req.Query,
		IncludeExpired: req.IncludeExpired,
		IncludeDeleted: req.IncludeDeleted,
		Page:           req.Page,
		PageSize:       req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadMemoriesResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadMemoriesData{
			Memories: taskThreadMemoriesToAPI(resp.Memories),
			Total:    resp.Total,
		},
	})
}

// ExportTaskThreadMemories .
// @router /api/workbench/task_threads/:thread_id/memories/export [GET]
func ExportTaskThreadMemories(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ExportTaskThreadMemoriesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		req.Query = string(c.Query("query"))
	}

	resp, err := appagentthread.SVC.ExportMemories(ctx, &appagentthread.ExportMemoriesRequest{
		ThreadID:       req.ThreadID,
		ViewerID:       workbenchViewerIDFromCtx(ctx),
		RunID:          req.RunID,
		Scopes:         taskThreadMemoryScopes(req.Scope, req.Scopes),
		Query:          req.Query,
		IncludeExpired: req.IncludeExpired,
		IncludeDeleted: req.IncludeDeleted,
		Limit:          req.Limit,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ExportTaskThreadMemoriesResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ExportTaskThreadMemoriesData{
			Schema:     resp.Schema,
			ThreadID:   resp.ThreadID,
			ExportedAt: resp.ExportedAt,
			Total:      resp.Total,
			Memories:   taskThreadMemoriesToAPI(resp.Memories),
		},
	})
}

// ImportTaskThreadMemories .
// @router /api/workbench/task_threads/:thread_id/memories/import [POST]
func ImportTaskThreadMemories(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ImportTaskThreadMemoriesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	items := make([]appagentthread.ImportMemoryItem, 0, len(req.Memories))
	for _, item := range req.Memories {
		if item == nil {
			continue
		}
		items = append(items, appagentthread.ImportMemoryItem{
			RunID:                item.RunID,
			Scope:                appagentthread.MemoryScope(item.Scope),
			Content:              item.Content,
			Metadata:             item.Metadata,
			Score:                item.Score,
			Confidence:           item.Confidence,
			SourceType:           item.SourceType,
			SourceID:             item.SourceID,
			CorrectionOfMemoryID: item.CorrectionOfMemoryID,
			CorrectedAt:          item.CorrectedAt,
			ExpiresAt:            item.ExpiresAt,
		})
	}
	resp, err := appagentthread.SVC.ImportMemories(ctx, &appagentthread.ImportMemoriesRequest{
		ThreadID: req.ThreadID,
		ActorID:  workbenchViewerIDFromCtx(ctx),
		ViewerID: workbenchViewerIDFromCtx(ctx),
		Memories: items,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ImportTaskThreadMemoriesResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ImportTaskThreadMemoriesData{
			Imported: resp.Imported,
			Skipped:  resp.Skipped,
			Memories: taskThreadMemoriesToAPI(resp.Memories),
		},
	})
}

// UpdateTaskThreadMemory .
// @router /api/workbench/task_threads/:thread_id/memories/:memory_id [PUT]
func UpdateTaskThreadMemory(ctx context.Context, c *app.RequestContext) {
	var req threadapi.UpdateTaskThreadMemoryRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.UpdateMemory(ctx, &appagentthread.UpdateMemoryRequest{
		ThreadID:             req.ThreadID,
		MemoryID:             req.MemoryID,
		ActorID:              workbenchViewerIDFromCtx(ctx),
		ViewerID:             workbenchViewerIDFromCtx(ctx),
		RunID:                req.RunID,
		Scope:                appagentthread.MemoryScope(req.Scope),
		Content:              req.Content,
		Metadata:             req.Metadata,
		Score:                req.Score,
		Confidence:           req.Confidence,
		SourceType:           req.SourceType,
		SourceID:             req.SourceID,
		CorrectionOfMemoryID: req.CorrectionOfMemoryID,
		CorrectedAt:          req.CorrectedAt,
		ExpiresAt:            req.ExpiresAt,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || !resp.Updated {
		invalidParamRequestResponse(c, "memory not found")
		return
	}

	c.JSON(consts.StatusOK, &threadapi.UpdateTaskThreadMemoryResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.UpdateTaskThreadMemoryData{
			Memory:  taskThreadMemoryToAPI(resp.Memory),
			Updated: resp.Updated,
		},
	})
}

// DeleteTaskThreadMemory .
// @router /api/workbench/task_threads/:thread_id/memories/:memory_id [DELETE]
func DeleteTaskThreadMemory(ctx context.Context, c *app.RequestContext) {
	var req threadapi.DeleteTaskThreadMemoryRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.DeleteMemory(ctx, &appagentthread.DeleteMemoryRequest{
		ThreadID: req.ThreadID,
		MemoryID: req.MemoryID,
		ActorID:  workbenchViewerIDFromCtx(ctx),
		ViewerID: workbenchViewerIDFromCtx(ctx),
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || !resp.Deleted {
		invalidParamRequestResponse(c, "memory not found")
		return
	}

	c.JSON(consts.StatusOK, &threadapi.DeleteTaskThreadMemoryResponse{
		Code: 0,
		Msg:  "success",
	})
}

// ClearTaskThreadMemories .
// @router /api/workbench/task_threads/:thread_id/memories/clear [POST]
func ClearTaskThreadMemories(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ClearTaskThreadMemoriesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ClearMemories(ctx, &appagentthread.ClearMemoriesRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Scopes:   taskThreadMemoryScopes("", req.Scopes),
		ActorID:  workbenchViewerIDFromCtx(ctx),
		ViewerID: workbenchViewerIDFromCtx(ctx),
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ClearTaskThreadMemoriesResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ClearTaskThreadMemoriesData{
			Deleted: resp.Deleted,
		},
	})
}

// RestoreTaskThreadMemory .
// @router /api/workbench/task_threads/:thread_id/memories/:memory_id/restore [POST]
func RestoreTaskThreadMemory(ctx context.Context, c *app.RequestContext) {
	var req threadapi.RestoreTaskThreadMemoryRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.RestoreMemory(ctx, &appagentthread.RestoreMemoryRequest{
		ThreadID: req.ThreadID,
		MemoryID: req.MemoryID,
		ActorID:  workbenchViewerIDFromCtx(ctx),
		ViewerID: workbenchViewerIDFromCtx(ctx),
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || !resp.Restored {
		invalidParamRequestResponse(c, "memory not found")
		return
	}

	c.JSON(consts.StatusOK, &threadapi.RestoreTaskThreadMemoryResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.RestoreTaskThreadMemoryData{
			Memory:   taskThreadMemoryToAPI(resp.Memory),
			Restored: resp.Restored,
		},
	})
}

// ListTaskThreadMemoryAuditEvents .
// @router /api/workbench/task_threads/:thread_id/memories/audit_events [GET]
func ListTaskThreadMemoryAuditEvents(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadMemoryAuditEventsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ListMemoryAuditEvents(ctx, &appagentthread.ListMemoryAuditEventsRequest{
		ThreadID: req.ThreadID,
		MemoryID: req.MemoryID,
		ViewerID: workbenchViewerIDFromCtx(ctx),
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadMemoryAuditEventsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadMemoryAuditEventsData{
			Events: taskThreadMemoryAuditEventsToAPI(resp.Events),
			Total:  resp.Total,
		},
	})
}

// ListTaskThreadGuardrailAuditEvents .
// @router /api/workbench/task_threads/:thread_id/guardrail_audit_events [GET]
func ListTaskThreadGuardrailAuditEvents(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadGuardrailAuditEventsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ListGuardrailAuditEvents(
		ctx,
		&appagentthread.ListGuardrailAuditEventsRequest{
			ThreadID: req.ThreadID,
			RunID:    req.RunID,
			ViewerID: workbenchViewerIDFromCtx(ctx),
			Page:     req.Page,
			PageSize: req.PageSize,
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadGuardrailAuditEventsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadGuardrailAuditEventsData{
			Events: taskThreadGuardrailAuditEventsToAPI(resp.Events),
			Total:  resp.Total,
		},
	})
}

// ExportTaskThreadGuardrailAuditEvents .
// @router /api/workbench/task_threads/:thread_id/guardrail_audit_events/export [GET]
func ExportTaskThreadGuardrailAuditEvents(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ExportTaskThreadGuardrailAuditEventsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ExportGuardrailAuditEvents(
		ctx,
		&appagentthread.ExportGuardrailAuditEventsRequest{
			ThreadID: req.ThreadID,
			RunID:    req.RunID,
			ViewerID: workbenchViewerIDFromCtx(ctx),
			Page:     req.Page,
			PageSize: req.PageSize,
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ExportTaskThreadGuardrailAuditEventsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ExportTaskThreadGuardrailAuditEventsData{
			Schema:     resp.Schema,
			ThreadID:   resp.ThreadID,
			ExportedAt: resp.ExportedAt,
			Page:       resp.Page,
			PageSize:   resp.PageSize,
			Total:      resp.Total,
			Events:     taskThreadGuardrailAuditEventsToAPI(resp.Events),
		},
	})
}

// ListTaskThreadArtifacts .
// @router /api/workbench/task_threads/:thread_id/artifacts [GET]
func ListTaskThreadArtifacts(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadArtifactsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	var runID *int64
	if req.RunID > 0 {
		runID = &req.RunID
	}
	resp, err := appagentthread.SVC.ListArtifacts(ctx, &appagentthread.ListArtifactsRequest{
		ThreadID:    req.ThreadID,
		RunID:       runID,
		DeletedOnly: req.DeletedOnly,
		SpaceID:     req.SpaceID,
		ViewerID:    workbenchViewerIDFromCtx(ctx),
		Page:        req.Page,
		PageSize:    req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadArtifactsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadArtifactsData{
			Artifacts: taskThreadArtifactsToAPI(resp.Artifacts),
			Total:     resp.Total,
		},
	})
}

// ListTaskThreadArtifactScanJobs .
// @router /api/workbench/task_threads/:thread_id/artifact_scan_jobs [GET]
func ListTaskThreadArtifactScanJobs(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadArtifactScanJobsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	var runID *int64
	if req.RunID > 0 {
		runID = &req.RunID
	}
	var artifactID *int64
	if req.ArtifactID > 0 {
		artifactID = &req.ArtifactID
	}
	resp, err := appagentthread.SVC.ListArtifactScanJobs(
		ctx,
		&appagentthread.ListArtifactScanJobsRequest{
			ThreadID:   req.ThreadID,
			RunID:      runID,
			ArtifactID: artifactID,
			SpaceID:    req.SpaceID,
			Status:     req.Status,
			Scanner:    req.Scanner,
			ViewerID:   workbenchViewerIDFromCtx(ctx),
			Page:       req.Page,
			PageSize:   req.PageSize,
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadArtifactScanJobsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadArtifactScanJobsData{
			Jobs:  taskThreadArtifactScanJobsToAPI(resp.Jobs),
			Total: resp.Total,
		},
	})
}

// RetryTaskThreadArtifactScanJob .
// @router /api/workbench/task_threads/:thread_id/artifact_scan_jobs/:job_id/retry [POST]
func RetryTaskThreadArtifactScanJob(ctx context.Context, c *app.RequestContext) {
	var req threadapi.RetryTaskThreadArtifactScanJobRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.RetryArtifactScanJob(
		ctx,
		&appagentthread.RetryArtifactScanJobRequest{
			ThreadID: req.ThreadID,
			JobID:    req.JobID,
			SpaceID:  req.SpaceID,
			ViewerID: workbenchViewerIDFromCtx(ctx),
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.RetryTaskThreadArtifactScanJobResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.RetryTaskThreadArtifactScanJobData{
			Job:     taskThreadArtifactScanJobToAPI(resp.Job),
			Retried: resp.Retried,
		},
	})
}

// ReviewTaskThreadArtifactScan .
// @router /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/scan_review [POST]
func ReviewTaskThreadArtifactScan(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ReviewTaskThreadArtifactScanRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ReviewArtifactScan(
		ctx,
		&appagentthread.ReviewArtifactScanRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			SpaceID:    req.SpaceID,
			ViewerID:   workbenchViewerIDFromCtx(ctx),
			Decision:   req.Decision,
			Reason:     req.Reason,
		},
	)
	if err != nil {
		if errors.Is(err, appagentthread.ErrArtifactScanReviewDecisionInvalid) {
			invalidParamRequestResponse(c, err.Error())
			return
		}
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || !resp.Reviewed {
		invalidParamRequestResponse(c, "artifact not found")
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ReviewTaskThreadArtifactScanResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ReviewTaskThreadArtifactScanData{
			ArtifactID: resp.ArtifactID,
			Decision:   resp.Decision,
			ScanStatus: resp.ScanStatus,
			Reviewed:   resp.Reviewed,
		},
	})
}

// GetTaskThreadArtifactContent .
// @router /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content [GET]
func GetTaskThreadArtifactContent(ctx context.Context, c *app.RequestContext) {
	var req threadapi.GetTaskThreadArtifactContentRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	mode := appagentthread.ArtifactContentMode(
		strings.ToLower(strings.TrimSpace(req.Mode)),
	)
	if mode != appagentthread.ArtifactContentModeDownload {
		mode = appagentthread.ArtifactContentModePreview
	}
	resp, err := appagentthread.SVC.ReadArtifactContent(
		ctx,
		&appagentthread.ReadArtifactContentRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			Mode:       mode,
			SpaceID:    req.SpaceID,
			ViewerID:   workbenchViewerIDFromCtx(ctx),
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.SetStatusCode(consts.StatusOK)
	c.SetContentType(resp.ContentType)
	c.Response.Header.Set(
		"Content-Disposition",
		taskThreadArtifactContentDisposition(resp.FileName, resp.Attachment),
	)
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	c.Response.SetBodyRaw(resp.Content)
}

// GetTaskThreadArtifactSignedURL .
// @router /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url [GET]
func GetTaskThreadArtifactSignedURL(ctx context.Context, c *app.RequestContext) {
	var req threadapi.GetTaskThreadArtifactSignedURLRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	mode := appagentthread.ArtifactContentMode(
		strings.ToLower(strings.TrimSpace(req.Mode)),
	)
	if mode == "" {
		mode = appagentthread.ArtifactContentModePreview
	}
	resp, err := appagentthread.SVC.CreateArtifactSignedURL(
		ctx,
		&appagentthread.CreateArtifactSignedURLRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			Mode:       mode,
			SpaceID:    req.SpaceID,
			ViewerID:   workbenchViewerIDFromCtx(ctx),
			TTLSeconds: req.TTLSeconds,
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	artifactID := req.ArtifactID
	if resp.Artifact != nil {
		artifactID = resp.Artifact.ArtifactID
	}
	c.JSON(consts.StatusOK, &threadapi.GetTaskThreadArtifactSignedURLResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.GetTaskThreadArtifactSignedURLData{
			ArtifactID:       artifactID,
			URL:              resp.URL,
			ExpiresInSeconds: resp.ExpiresInSeconds,
			ContentType:      resp.ContentType,
			PreviewMode:      string(resp.PreviewMode),
		},
	})
}

// DeleteTaskThreadArtifact .
// @router /api/workbench/task_threads/:thread_id/artifacts/:artifact_id [DELETE]
func DeleteTaskThreadArtifact(ctx context.Context, c *app.RequestContext) {
	var req threadapi.DeleteTaskThreadArtifactRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.DeleteArtifact(
		ctx,
		&appagentthread.DeleteArtifactRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			SpaceID:    req.SpaceID,
			ViewerID:   workbenchViewerIDFromCtx(ctx),
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || !resp.Deleted {
		invalidParamRequestResponse(c, "artifact not found")
		return
	}

	c.JSON(consts.StatusOK, &threadapi.DeleteTaskThreadArtifactResponse{
		Code: 0,
		Msg:  "success",
	})
}

// RestoreTaskThreadArtifact .
// @router /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/restore [POST]
func RestoreTaskThreadArtifact(ctx context.Context, c *app.RequestContext) {
	var req threadapi.RestoreTaskThreadArtifactRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.RestoreArtifact(
		ctx,
		&appagentthread.RestoreArtifactRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			SpaceID:    req.SpaceID,
			ViewerID:   workbenchViewerIDFromCtx(ctx),
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || !resp.Restored {
		invalidParamRequestResponse(c, "artifact not found")
		return
	}
	artifactID := req.ArtifactID
	if resp.Artifact != nil && resp.Artifact.ArtifactID > 0 {
		artifactID = resp.Artifact.ArtifactID
	}

	c.JSON(consts.StatusOK, &threadapi.RestoreTaskThreadArtifactResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.RestoreTaskThreadArtifactData{
			ArtifactID: artifactID,
			Restored:   resp.Restored,
		},
	})
}

// StreamTaskThreadRunEvents .
// @router /api/workbench/task_threads/:thread_id/run_events/stream [GET]
func StreamTaskThreadRunEvents(ctx context.Context, c *app.RequestContext) {
	var req threadapi.StreamTaskThreadRunEventsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	writer := sse.NewWriter(c)
	c.SetContentType("text/event-stream; charset=utf-8")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	defer func() {
		if err := writer.Close(); err != nil {
			logs.CtxWarnf(ctx, "close task thread run event stream failed, err=%v", err)
		}
	}()

	streamTaskThreadRunEvents(ctx, writer, req)
}

// CreateTaskThreadRun .
// @router /api/workbench/task_threads/:thread_id/runs [POST]
func CreateTaskThreadRun(ctx context.Context, c *app.RequestContext) {
	var req threadapi.CreateTaskThreadRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.CreateRun(ctx, &appagentthread.CreateRunRequest{
		ThreadID:          req.ThreadID,
		AssistantID:       req.AssistantID,
		Command:           req.Command,
		Input:             req.Input,
		Config:            req.Config,
		Context:           req.Context,
		Metadata:          req.Metadata,
		StreamMode:        req.StreamMode,
		MultitaskStrategy: req.MultitaskStrategy,
		OnDisconnect:      req.OnDisconnect,
		Durability:        req.Durability,
		IdempotencyKey:    req.IdempotencyKey,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.CreateTaskThreadRunResponse{
		Code: 0,
		Msg:  "success",
		Data: taskThreadRunToAPI(resp.Run),
	})
}

// ResumeTaskThreadRun .
// @router /api/workbench/task_threads/:thread_id/runs/:run_id/resume [POST]
func ResumeTaskThreadRun(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ResumeTaskThreadRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if err := validateResumeTaskThreadRunRequest(req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ResumeHumanInteraction(ctx, &appagentthread.ResumeHumanInteractionRequest{
		ThreadID:       req.ThreadID,
		SourceRunID:    req.RunID,
		InterruptID:    req.InterruptID,
		IdempotencyKey: req.IdempotencyKey,
		Response: appagentthread.HumanInteractionResponse{
			Schema:        req.Response.Schema,
			InteractionID: req.Response.InteractionID,
			Kind:          appagentthread.HumanInteractionKind(req.Response.Kind),
			Decision:      appagentthread.HumanInteractionDecision(req.Response.Decision),
			Answer:        req.Response.Answer,
			ChoiceID:      req.Response.ChoiceID,
			Comment:       req.Response.Comment,
			SubmittedBy:   req.Response.SubmittedBy,
			SubmittedAt:   req.Response.SubmittedAt,
			Source:        req.Response.Source,
		},
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ResumeTaskThreadRunResponse{
		Code: 0,
		Msg:  "success",
		Data: taskThreadRunToAPI(resp.Run),
	})
}

func validateResumeTaskThreadRunRequest(req threadapi.ResumeTaskThreadRunRequest) error {
	if req.ThreadID <= 0 {
		return strconv.ErrSyntax
	}
	if req.RunID <= 0 {
		return strconv.ErrSyntax
	}
	if strings.TrimSpace(req.InterruptID) == "" {
		return strconv.ErrSyntax
	}
	if strings.TrimSpace(req.Response.Schema) == "" ||
		strings.TrimSpace(req.Response.InteractionID) == "" ||
		strings.TrimSpace(req.Response.Kind) == "" ||
		strings.TrimSpace(req.Response.Decision) == "" {
		return strconv.ErrSyntax
	}

	return nil
}

// CancelTaskThreadRun .
// @router /api/workbench/task_threads/:thread_id/runs/:run_id/cancel [POST]
func CancelTaskThreadRun(ctx context.Context, c *app.RequestContext) {
	var req threadapi.CancelTaskThreadRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if err := validateCancelTaskThreadRunRequest(req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	runResp, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: req.RunID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if runResp == nil || runResp.Run == nil || runResp.Run.ThreadID != req.ThreadID {
		invalidParamRequestResponse(c, "run_id does not belong to thread_id")
		return
	}

	resp, err := appagentthread.SVC.CancelRun(ctx, &appagentthread.UpdateRunStatusRequest{
		RunID: req.RunID,
		From:  runResp.Run.Status,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.CancelTaskThreadRunResponse{
		Code: 0,
		Msg:  "success",
		Data: taskThreadRunToAPI(resp.Run),
	})
}

func validateCancelTaskThreadRunRequest(req threadapi.CancelTaskThreadRunRequest) error {
	if req.ThreadID <= 0 {
		return strconv.ErrSyntax
	}
	if req.RunID <= 0 {
		return strconv.ErrSyntax
	}

	return nil
}

// RetryTaskThreadSubagentRun .
// @router /api/workbench/task_threads/:thread_id/runs/:run_id/retry [POST]
func RetryTaskThreadSubagentRun(ctx context.Context, c *app.RequestContext) {
	var req threadapi.RetryTaskThreadSubagentRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if err := validateRetryTaskThreadSubagentRunRequest(req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.RetrySubagentRun(ctx, &appagentthread.RetrySubagentRunRequest{
		ThreadID:       req.ThreadID,
		SourceRunID:    req.RunID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.RetryTaskThreadSubagentRunResponse{
		Code: 0,
		Msg:  "success",
		Data: taskThreadRunToAPI(resp.Run),
	})
}

func validateRetryTaskThreadSubagentRunRequest(req threadapi.RetryTaskThreadSubagentRunRequest) error {
	if req.ThreadID <= 0 {
		return strconv.ErrSyntax
	}
	if req.RunID <= 0 {
		return strconv.ErrSyntax
	}

	return nil
}

func taskThreadsToAPI(threads []*appagentthread.ThreadSummary) []*threadapi.TaskThread {
	result := make([]*threadapi.TaskThread, 0, len(threads))
	for _, item := range threads {
		result = append(result, taskThreadToAPI(item))
	}

	return result
}

func taskThreadMessagesToAPI(messages []*appagentthread.MessageSummary) []*threadapi.TaskThreadMessage {
	result := make([]*threadapi.TaskThreadMessage, 0, len(messages))
	for _, item := range messages {
		result = append(result, taskThreadMessageToAPI(item))
	}

	return result
}

func taskThreadRunsToAPI(runs []*appagentthread.RunSummary) []*threadapi.TaskThreadRun {
	result := make([]*threadapi.TaskThreadRun, 0, len(runs))
	for _, item := range runs {
		result = append(result, taskThreadRunToAPI(item))
	}

	return result
}

func taskThreadRunEventsToAPI(events []*appagentthread.RunEventSummary) []*threadapi.TaskThreadRunEvent {
	result := make([]*threadapi.TaskThreadRunEvent, 0, len(events))
	for _, item := range events {
		result = append(result, taskThreadRunEventToAPI(item))
	}

	return result
}

func taskThreadTokenUsagesToAPI(usages []*appagentthread.TokenUsageSummary) []*threadapi.TaskThreadTokenUsage {
	result := make([]*threadapi.TaskThreadTokenUsage, 0, len(usages))
	for _, item := range usages {
		result = append(result, taskThreadTokenUsageToAPI(item))
	}

	return result
}

func taskThreadTokenUsageRunAggregatesToAPI(aggregates []*appagentthread.RunTokenUsageAggregateSummary) []*threadapi.TaskThreadTokenUsageRunAggregate {
	result := make([]*threadapi.TaskThreadTokenUsageRunAggregate, 0, len(aggregates))
	for _, item := range aggregates {
		if item == nil {
			continue
		}
		result = append(result, &threadapi.TaskThreadTokenUsageRunAggregate{
			RunID:     item.RunID,
			Aggregate: taskThreadTokenUsageAggregateToAPI(item.Aggregate),
		})
	}

	return result
}

func taskThreadArtifactsToAPI(artifacts []*appagentthread.ArtifactSummary) []*threadapi.TaskThreadArtifact {
	result := make([]*threadapi.TaskThreadArtifact, 0, len(artifacts))
	for _, item := range artifacts {
		result = append(result, taskThreadArtifactToAPI(item))
	}

	return result
}

func taskThreadMemoriesToAPI(memories []*appagentthread.MemorySummary) []*threadapi.TaskThreadMemory {
	result := make([]*threadapi.TaskThreadMemory, 0, len(memories))
	for _, item := range memories {
		result = append(result, taskThreadMemoryToAPI(item))
	}

	return result
}

func taskThreadMemoryAuditEventsToAPI(
	events []*appagentthread.MemoryAuditEventSummary,
) []*threadapi.TaskThreadMemoryAuditEvent {
	result := make([]*threadapi.TaskThreadMemoryAuditEvent, 0, len(events))
	for _, item := range events {
		result = append(result, taskThreadMemoryAuditEventToAPI(item))
	}

	return result
}

func taskThreadGuardrailAuditEventsToAPI(
	events []*appagentthread.GuardrailAuditEventSummary,
) []*threadapi.TaskThreadGuardrailAuditEvent {
	result := make([]*threadapi.TaskThreadGuardrailAuditEvent, 0, len(events))
	for _, item := range events {
		result = append(result, taskThreadGuardrailAuditEventToAPI(item))
	}

	return result
}

func taskThreadMemoryScopes(scope string, scopes []string) []appagentthread.MemoryScope {
	values := make([]appagentthread.MemoryScope, 0, len(scopes)+1)
	if strings.TrimSpace(scope) != "" {
		values = append(values, appagentthread.MemoryScope(strings.TrimSpace(scope)))
	}
	for _, raw := range scopes {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		values = append(values, appagentthread.MemoryScope(strings.TrimSpace(raw)))
	}

	return values
}

func taskThreadArtifactScanJobsToAPI(
	jobs []*appagentthread.ArtifactScanJobSummary,
) []*threadapi.TaskThreadArtifactScanJob {
	result := make([]*threadapi.TaskThreadArtifactScanJob, 0, len(jobs))
	for _, item := range jobs {
		if item == nil {
			continue
		}
		result = append(result, taskThreadArtifactScanJobToAPI(item))
	}

	return result
}

func taskThreadArtifactContentDisposition(fileName string, attachment bool) string {
	disposition := "inline"
	if attachment {
		disposition = "attachment"
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "artifact"
	}
	return disposition + "; filename*=UTF-8''" + url.PathEscape(fileName)
}

func taskThreadToAPI(thread *appagentthread.ThreadSummary) *threadapi.TaskThread {
	if thread == nil {
		return nil
	}

	return &threadapi.TaskThread{
		ThreadID:         thread.ThreadID,
		LegacyTaskID:     thread.LegacyTaskID,
		SpaceID:          thread.SpaceID,
		CreatorID:        thread.CreatorID,
		Title:            thread.Title,
		Status:           string(thread.Status),
		Source:           string(thread.Source),
		Progress:         thread.Progress,
		LastUserMessage:  thread.LastUserMessage,
		LastAgentMessage: thread.LastAgentMessage,
		CreatedAt:        thread.CreatedAt,
		UpdatedAt:        thread.UpdatedAt,
	}
}

func taskThreadArtifactToAPI(artifact *appagentthread.ArtifactSummary) *threadapi.TaskThreadArtifact {
	if artifact == nil {
		return nil
	}

	return &threadapi.TaskThreadArtifact{
		ArtifactID:   artifact.ArtifactID,
		ThreadID:     artifact.ThreadID,
		RunID:        artifact.RunID,
		FileID:       artifact.FileID,
		Title:        artifact.Title,
		ArtifactType: artifact.ArtifactType,
		VirtualPath:  artifact.VirtualPath,
		ContentType:  artifact.ContentType,
		SizeBytes:    artifact.SizeBytes,
		PreviewMode:  string(artifact.PreviewMode),
		Metadata:     artifact.Metadata,
		CreatedAt:    artifact.CreatedAt,
		UpdatedAt:    artifact.UpdatedAt,
		DeletedAt:    artifact.DeletedAt,
	}
}

func taskThreadMemoryToAPI(memory *appagentthread.MemorySummary) *threadapi.TaskThreadMemory {
	if memory == nil {
		return nil
	}

	return &threadapi.TaskThreadMemory{
		MemoryID:             memory.MemoryID,
		ThreadID:             memory.ThreadID,
		RunID:                memory.RunID,
		SpaceID:              memory.SpaceID,
		Scope:                string(memory.Scope),
		Content:              memory.Content,
		Metadata:             memory.Metadata,
		Score:                memory.Score,
		Confidence:           memory.Confidence,
		SourceType:           memory.SourceType,
		SourceID:             memory.SourceID,
		CorrectionOfMemoryID: memory.CorrectionOfMemoryID,
		CorrectedAt:          memory.CorrectedAt,
		ExpiresAt:            memory.ExpiresAt,
		CreatedAt:            memory.CreatedAt,
		UpdatedAt:            memory.UpdatedAt,
		DeletedAt:            memory.DeletedAt,
	}
}

func taskThreadMemoryAuditEventToAPI(
	event *appagentthread.MemoryAuditEventSummary,
) *threadapi.TaskThreadMemoryAuditEvent {
	if event == nil {
		return nil
	}

	return &threadapi.TaskThreadMemoryAuditEvent{
		EventID:       event.EventID,
		ThreadID:      event.ThreadID,
		RunID:         event.RunID,
		SpaceID:       event.SpaceID,
		MemoryID:      event.MemoryID,
		ActorID:       event.ActorID,
		EventType:     event.EventType,
		Scope:         string(event.Scope),
		SourceType:    event.SourceType,
		SourceID:      event.SourceID,
		AffectedCount: event.AffectedCount,
		CreatedAt:     event.CreatedAt,
	}
}

func taskThreadGuardrailAuditEventToAPI(
	event *appagentthread.GuardrailAuditEventSummary,
) *threadapi.TaskThreadGuardrailAuditEvent {
	if event == nil {
		return nil
	}

	return &threadapi.TaskThreadGuardrailAuditEvent{
		EventID:    event.EventID,
		ThreadID:   event.ThreadID,
		RunID:      event.RunID,
		SpaceID:    event.SpaceID,
		ActorID:    event.ActorID,
		EventType:  event.EventType,
		TargetType: event.TargetType,
		TargetID:   event.TargetID,
		Operation:  event.Operation,
		Source:     event.Source,
		Action:     event.Action,
		FailMode:   event.FailMode,
		Provider:   event.Provider,
		ReasonCode: event.ReasonCode,
		RuleIDs:    event.RuleIDs,
		CreatedAt:  event.CreatedAt,
	}
}

func taskThreadArtifactScanJobToAPI(
	job *appagentthread.ArtifactScanJobSummary,
) *threadapi.TaskThreadArtifactScanJob {
	if job == nil {
		return nil
	}

	return &threadapi.TaskThreadArtifactScanJob{
		JobID:          job.JobID,
		ThreadID:       job.ThreadID,
		RunID:          job.RunID,
		SpaceID:        job.SpaceID,
		UserID:         job.UserID,
		ArtifactID:     job.ArtifactID,
		FileID:         job.FileID,
		Scanner:        job.Scanner,
		Status:         string(job.Status),
		WorkerID:       job.WorkerID,
		AttemptCount:   job.AttemptCount,
		LastError:      job.LastError,
		AvailableAt:    job.AvailableAt,
		LeaseExpiresAt: job.LeaseExpiresAt,
		StartedAt:      job.StartedAt,
		EndedAt:        job.EndedAt,
		CreatedAt:      job.CreatedAt,
		UpdatedAt:      job.UpdatedAt,
	}
}

func taskThreadRunToAPI(run *appagentthread.RunSummary) *threadapi.TaskThreadRun {
	if run == nil {
		return nil
	}

	command := run.Command
	input := run.Input
	config := run.Config
	runContext := run.Context
	if run.RunKind == appagentthread.RunKindSubagent {
		command = ""
		input = ""
		config = ""
		runContext = ""
	}

	return &threadapi.TaskThreadRun{
		RunID:             run.RunID,
		ThreadID:          run.ThreadID,
		ParentRunID:       run.ParentRunID,
		SpaceID:           run.SpaceID,
		CreatorID:         run.CreatorID,
		AssistantID:       run.AssistantID,
		RunKind:           string(run.RunKind),
		Status:            string(run.Status),
		Command:           command,
		Input:             input,
		Config:            config,
		Context:           runContext,
		Metadata:          run.Metadata,
		StreamMode:        run.StreamMode,
		MultitaskStrategy: run.MultitaskStrategy,
		OnDisconnect:      run.OnDisconnect,
		Durability:        run.Durability,
		IdempotencyKey:    run.IdempotencyKey,
		WorkerID:          run.WorkerID,
		ErrorCode:         run.ErrorCode,
		ErrorMessage:      run.ErrorMessage,
		StartedAt:         run.StartedAt,
		EndedAt:           run.EndedAt,
		CreatedAt:         run.CreatedAt,
		UpdatedAt:         run.UpdatedAt,
	}
}

func taskThreadRunEventToAPI(event *appagentthread.RunEventSummary) *threadapi.TaskThreadRunEvent {
	if event == nil {
		return nil
	}

	return &threadapi.TaskThreadRunEvent{
		EventID:   event.EventID,
		ThreadID:  event.ThreadID,
		RunID:     event.RunID,
		EventType: event.EventType,
		Payload:   taskThreadRunEventPayloadToAPI(event.EventType, event.Payload),
		CreatedAt: event.CreatedAt,
	}
}

func taskThreadRunEventPayloadToAPI(eventType, payload string) string {
	if !isUnsafeTaskThreadRunEventPayload(eventType) {
		return payload
	}

	safePayload := map[string]any{
		"redacted": true,
	}
	var rawPayload map[string]any
	if err := sonic.UnmarshalString(payload, &rawPayload); err == nil {
		copySafeRunEventString(rawPayload, safePayload, "role")
		copySafeRunEventString(rawPayload, safePayload, "tool_name")
		copySafeRunEventString(rawPayload, safePayload, "tool_call_id")
		copySafeRunEventString(rawPayload, safePayload, "finish_reason")
		copySafeRunEventString(rawPayload, safePayload, "status")
		copySafeRunEventBool(rawPayload, safePayload, "arguments_present")
		copySafeRunEventBool(rawPayload, safePayload, "result_present")

		if eventType == "message.completed" {
			copySafeRunEventReasoningString(rawPayload, safePayload, "reasoning_content")
			copySafeRunEventMessageToolCalls(rawPayload, safePayload)
		}

		if eventType == "tool.completed" && safePayload["result_present"] == nil && hasSafeRunEventString(rawPayload, "content") {
			safePayload["result_present"] = true
		}
	}

	encoded, err := sonic.MarshalString(safePayload)
	if err != nil {
		return `{"redacted":true}`
	}

	return encoded
}

func isUnsafeTaskThreadRunEventPayload(eventType string) bool {
	switch eventType {
	case "message.completed",
		"tool.completed",
		"tool.failed",
		"model.safety_finish",
		"agent.output":
		return true
	default:
		return false
	}
}

func copySafeRunEventString(source, target map[string]any, key string) {
	if value, ok := safeRunEventString(source[key]); ok {
		target[key] = value
	}
}

func copySafeRunEventDisplayString(source, target map[string]any, key string) {
	if value, ok := safeRunEventDisplayString(source[key]); ok {
		target[key] = value
	}
}

func copySafeRunEventReasoningString(source, target map[string]any, key string) {
	if value, ok := safeRunEventDisplayStringWithLimit(source[key], maxSafeRunEventReasoningLen); ok {
		target[key] = value
	}
}

func copySafeRunEventBool(source, target map[string]any, key string) {
	if value, ok := source[key].(bool); ok {
		target[key] = value
	}
}

func copySafeRunEventMessageToolCalls(source, target map[string]any) {
	rawToolCalls, ok := source["tool_calls"].([]any)
	if !ok || len(rawToolCalls) == 0 {
		return
	}

	safeToolCalls := make([]map[string]any, 0, min(len(rawToolCalls), maxSafeRunEventToolCalls))
	for _, rawToolCall := range rawToolCalls {
		if len(safeToolCalls) >= maxSafeRunEventToolCalls {
			break
		}

		toolCall, ok := rawToolCall.(map[string]any)
		if !ok {
			continue
		}

		functionCall, _ := toolCall["function"].(map[string]any)
		toolName, ok := safeRunEventToolName(firstRunEventValue(toolCall["name"], functionCall["name"]))
		if !ok {
			continue
		}

		safeFunctionCall := map[string]any{
			"name": toolName,
		}
		if arguments := safeRunEventToolArguments(toolName, toolCall["args"], toolCall["arguments"], functionCall["arguments"]); len(arguments) > 0 {
			if encodedArguments, err := sonic.MarshalString(arguments); err == nil {
				safeFunctionCall["arguments"] = encodedArguments
			}
		}

		safeToolCall := map[string]any{
			"function": safeFunctionCall,
		}
		if id, ok := safeRunEventDisplayString(toolCall["id"]); ok {
			safeToolCall["id"] = id
		}
		if callType, ok := safeRunEventDisplayString(toolCall["type"]); ok {
			safeToolCall["type"] = callType
		}
		safeToolCalls = append(safeToolCalls, safeToolCall)
	}

	if len(safeToolCalls) > 0 {
		target["tool_calls"] = safeToolCalls
	}
}

func safeRunEventToolArguments(toolName string, values ...any) map[string]any {
	for _, value := range values {
		arguments, ok := runEventToolArgumentsObject(value)
		if !ok {
			continue
		}

		safeArguments := map[string]any{}
		if toolName == "skill" {
			if skill, ok := safeRunEventDisplayString(arguments["skill"]); ok {
				safeArguments["skill"] = skill
			}
			if skillName, ok := safeRunEventDisplayString(arguments["skill_name"]); ok {
				safeArguments["skill_name"] = skillName
			}
		}

		if description, ok := safeRunEventDisplayString(arguments["description"]); ok {
			safeArguments["description"] = description
		}

		for _, key := range []string{"path", "file_path", "filepath", "virtual_path", "output_path"} {
			if path, ok := safeRunEventVirtualPath(arguments[key]); ok {
				safeArguments[key] = path
				break
			}
		}
		if _, ok := safeArguments["path"]; !ok {
			if path, ok := safeRunEventFirstVirtualPath(arguments["filepaths"]); ok {
				safeArguments["path"] = path
			}
		}

		if len(safeArguments) > 0 {
			return safeArguments
		}
	}

	return nil
}

func runEventToolArgumentsObject(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}

	if arguments, ok := value.(map[string]any); ok {
		return arguments, true
	}

	text, ok := safeRunEventArgumentsJSONString(value)
	if !ok {
		return nil, false
	}

	var arguments map[string]any
	if err := sonic.UnmarshalString(text, &arguments); err != nil {
		return nil, false
	}

	return arguments, true
}

func safeRunEventArgumentsJSONString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	text = strings.Map(func(item rune) rune {
		if item < 0x20 || item == 0x7f {
			return -1
		}

		return item
	}, text)
	if text == "" {
		return "", false
	}
	if len([]rune(text)) > maxSafeRunEventArgumentsJSONLen {
		return "", false
	}

	return text, true
}

func firstRunEventValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}

	return nil
}

func hasSafeRunEventString(source map[string]any, key string) bool {
	_, ok := safeRunEventString(source[key])

	return ok
}

func safeRunEventString(value any) (string, bool) {
	return safeRunEventStringWithLimit(value, maxSafeRunEventPayloadStringLen)
}

func safeRunEventStringWithLimit(value any, limit int) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	text = strings.Map(func(item rune) rune {
		if item < 0x20 || item == 0x7f {
			return -1
		}

		return item
	}, text)
	if text == "" {
		return "", false
	}
	runes := []rune(text)
	if len(runes) > limit {
		text = string(runes[:limit])
	}

	return text, true
}

func safeRunEventDisplayString(value any) (string, bool) {
	return safeRunEventDisplayStringWithLimit(value, maxSafeRunEventPayloadStringLen)
}

func safeRunEventDisplayStringWithLimit(value any, limit int) (string, bool) {
	text, ok := safeRunEventStringWithLimit(value, limit)
	if !ok {
		return "", false
	}
	if unsafeRunEventDisplayPattern.MatchString(text) {
		return "", false
	}

	return text, true
}

func safeRunEventFirstVirtualPath(value any) (string, bool) {
	values, ok := value.([]any)
	if !ok {
		return "", false
	}
	for _, value := range values {
		if path, ok := safeRunEventVirtualPath(value); ok {
			return path, true
		}
	}

	return "", false
}

func safeRunEventVirtualPath(value any) (string, bool) {
	text, ok := safeRunEventDisplayString(value)
	if !ok {
		return "", false
	}
	if !strings.HasPrefix(text, "/mnt/user-data/workspace/") && !strings.HasPrefix(text, "/mnt/user-data/outputs/") {
		return "", false
	}

	return text, true
}

func safeRunEventToolName(value any) (string, bool) {
	text, ok := safeRunEventDisplayString(value)
	if !ok || len(text) > 64 {
		return "", false
	}
	for index, char := range text {
		if index == 0 {
			if !isSafeRunEventToolNameFirstChar(char) {
				return "", false
			}
			continue
		}
		if !isSafeRunEventToolNameChar(char) {
			return "", false
		}
	}

	return text, true
}

func isSafeRunEventToolNameFirstChar(char rune) bool {
	return char == '_' || (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z')
}

func isSafeRunEventToolNameChar(char rune) bool {
	return isSafeRunEventToolNameFirstChar(char) || (char >= '0' && char <= '9')
}

func taskThreadTokenUsageToAPI(usage *appagentthread.TokenUsageSummary) *threadapi.TaskThreadTokenUsage {
	if usage == nil {
		return nil
	}

	return &threadapi.TaskThreadTokenUsage{
		UsageID:      usage.UsageID,
		ThreadID:     usage.ThreadID,
		RunID:        usage.RunID,
		SpaceID:      usage.SpaceID,
		Source:       string(usage.Source),
		StepID:       usage.StepID,
		StepIndex:    usage.StepIndex,
		StepName:     usage.StepName,
		ModelName:    usage.ModelName,
		Provider:     usage.Provider,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
		CostMicros:   usage.CostMicros,
		Currency:     usage.Currency,
		Estimated:    usage.Estimated,
		RawUsage:     "",
		Metadata:     "",
		CreatedAt:    usage.CreatedAt,
	}
}

func taskThreadTokenUsageAggregateToAPI(aggregate *appagentthread.TokenUsageAggregateSummary) *threadapi.TaskThreadTokenUsageAggregate {
	if aggregate == nil {
		return &threadapi.TaskThreadTokenUsageAggregate{}
	}

	return &threadapi.TaskThreadTokenUsageAggregate{
		InputTokens:      aggregate.InputTokens,
		OutputTokens:     aggregate.OutputTokens,
		TotalTokens:      aggregate.TotalTokens,
		CostMicros:       aggregate.CostMicros,
		CallCount:        aggregate.CallCount,
		LeadAgentTokens:  aggregate.LeadAgentTokens,
		SubagentTokens:   aggregate.SubagentTokens,
		MiddlewareTokens: aggregate.MiddlewareTokens,
		ToolTokens:       aggregate.ToolTokens,
	}
}

func taskThreadMessageToAPI(message *appagentthread.MessageSummary) *threadapi.TaskThreadMessage {
	if message == nil {
		return nil
	}

	return &threadapi.TaskThreadMessage{
		MessageID: message.MessageID,
		ThreadID:  message.ThreadID,
		RunID:     message.RunID,
		Role:      string(message.Role),
		Content:   message.Content,
		Metadata:  message.Metadata,
		CreatedAt: message.CreatedAt,
	}
}

func workbenchThreadErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	if errors.Is(err, appagentthread.ErrArtifactAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "artifact access denied",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrMemoryAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "memory access denied",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrGuardrailAuditAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "guardrail audit access denied",
		})
		return
	}
	var scanBlocked *appagentthread.ArtifactContentBlockedByScanError
	if errors.As(err, &scanBlocked) {
		c.JSON(consts.StatusConflict, map[string]any{
			"code":   consts.StatusConflict,
			"msg":    "artifact content blocked by scan policy",
			"reason": scanBlocked.Reason,
		})
		return
	}
	if errors.Is(err, appagentthread.ErrArtifactScanJobRetryNotAllowed) {
		c.JSON(consts.StatusConflict, map[string]any{
			"code": consts.StatusConflict,
			"msg":  "artifact scan job cannot be retried",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrArtifactSignedURLNotSupported) {
		c.JSON(consts.StatusConflict, map[string]any{
			"code": consts.StatusConflict,
			"msg":  "artifact signed url is not supported",
		})
		return
	}
	internalServerErrorResponse(ctx, c, err)
}

func workbenchViewerIDFromCtx(ctx context.Context) int64 {
	if uid := ctxutil.GetUIDFromCtx(ctx); uid != nil {
		return *uid
	}
	if apiKey := ctxutil.GetApiAuthFromCtx(ctx); apiKey != nil {
		return apiKey.UserID
	}
	return 0
}

func streamTaskThreadRunEvents(ctx context.Context, writer taskThreadRunEventStreamWriter, req threadapi.StreamTaskThreadRunEventsRequest) {
	afterEventID := req.AfterEventID
	interval := clampRunEventStreamDuration(req.IntervalMs, defaultRunEventStreamIntervalMs, minRunEventStreamIntervalMs, maxRunEventStreamIntervalMs)
	timeout := clampRunEventStreamDuration(req.TimeoutMs, defaultRunEventStreamTimeoutMs, minRunEventStreamTimeoutMs, maxRunEventStreamTimeoutMs)

	sendNewEvents := func() bool {
		resp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
			ThreadID: req.ThreadID,
			RunID:    req.RunID,
			Page:     1,
			PageSize: 200,
		})
		if err != nil {
			writeTaskThreadRunEventStreamError(ctx, writer, err)
			return false
		}

		for _, event := range resp.Events {
			if event == nil || event.EventID <= afterEventID {
				continue
			}
			if !writeTaskThreadRunEventStreamEvent(ctx, writer, event) {
				return false
			}
			afterEventID = event.EventID
		}

		return true
	}

	if !sendNewEvents() {
		return
	}
	if req.RunID > 0 && writeDoneWhenRunTerminal(ctx, writer, req.RunID) {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case <-ticker.C:
			if !sendNewEvents() {
				return
			}
			if req.RunID > 0 && writeDoneWhenRunTerminal(ctx, writer, req.RunID) {
				return
			}
		}
	}
}

func writeTaskThreadRunEventStreamEvent(ctx context.Context, writer taskThreadRunEventStreamWriter, event *appagentthread.RunEventSummary) bool {
	payload, err := sonic.Marshal(taskThreadRunEventToAPI(event))
	if err != nil {
		writeTaskThreadRunEventStreamError(ctx, writer, err)
		return false
	}

	if err := writer.WriteEvent(strconv.FormatInt(event.EventID, 10), taskThreadRunEventStreamEvent, payload); err != nil {
		logs.CtxWarnf(ctx, "write task thread run event stream failed, err=%v", err)
		return false
	}

	return true
}

func writeTaskThreadRunEventStreamError(ctx context.Context, writer taskThreadRunEventStreamWriter, err error) {
	if err == nil {
		return
	}
	if writeErr := writer.WriteEvent("", taskThreadRunEventStreamError, []byte(err.Error())); writeErr != nil {
		logs.CtxWarnf(ctx, "write task thread run event stream error failed, err=%v", writeErr)
	}
}

func writeDoneWhenRunTerminal(ctx context.Context, writer taskThreadRunEventStreamWriter, runID int64) bool {
	resp, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: runID})
	if err != nil {
		writeTaskThreadRunEventStreamError(ctx, writer, err)
		return true
	}
	if resp == nil || resp.Run == nil || !isTaskThreadRunTerminal(resp.Run.Status) {
		return false
	}

	data, err := sonic.Marshal(map[string]string{"reason": "terminal_run"})
	if err != nil {
		writeTaskThreadRunEventStreamError(ctx, writer, err)
		return true
	}
	if err := writer.WriteEvent("", taskThreadRunEventStreamDone, data); err != nil {
		logs.CtxWarnf(ctx, "write task thread run event stream done failed, err=%v", err)
	}

	return true
}

func isTaskThreadRunTerminal(status appagentthread.RunStatus) bool {
	return status == appagentthread.RunStatusSucceeded ||
		status == appagentthread.RunStatusFailed ||
		status == appagentthread.RunStatusInterrupted ||
		status == appagentthread.RunStatusCanceled
}

func clampRunEventStreamDuration(valueMs, defaultMs, minMs, maxMs int64) time.Duration {
	if valueMs <= 0 {
		valueMs = defaultMs
	}
	if valueMs < minMs {
		valueMs = minMs
	}
	if valueMs > maxMs {
		valueMs = maxMs
	}

	return time.Duration(valueMs) * time.Millisecond
}
