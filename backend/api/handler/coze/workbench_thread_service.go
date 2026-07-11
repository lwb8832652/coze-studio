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
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"

	threadapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appworkbench "github.com/coze-dev/coze-studio/backend/application/workbench"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

const (
	taskThreadRunEventStreamEvent    = "run.event"
	taskThreadRunEventStreamDone     = "done"
	taskThreadRunEventStreamError    = "error"
	defaultRunEventStreamIntervalMs  = int64(1000)
	defaultRunEventStreamTimeoutMs   = int64(30000)
	minRunEventStreamIntervalMs      = int64(10)
	maxRunEventStreamIntervalMs      = int64(5000)
	minRunEventStreamTimeoutMs       = int64(1)
	maxRunEventStreamTimeoutMs       = int64(60000)
	taskThreadRunEventStreamPageSize = int32(200)
)

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
	ctx = workbenchThreadAccessContext(ctx, 0, 0)

	var status *appagentthread.ThreadStatus
	if req.Status != "" {
		mapped := appagentthread.ThreadStatus(req.Status)
		status = &mapped
	}
	resp, err := appagentthread.SVC.ListThreads(ctx, &appagentthread.ListThreadsRequest{
		SpaceID:  req.SpaceID,
		UserID:   workbenchViewerIDFromCtx(ctx),
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
	ctx = workbenchThreadAccessContext(ctx, 0, 0)

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
		DeferStart:        req.DeferStart,
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

// ListTaskThreadUploadFiles .
// @router /api/workbench/task_threads/:thread_id/uploads [GET]
func ListTaskThreadUploadFiles(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadUploadFilesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ListTaskThreadUploadFiles(
		ctx,
		&appagentthread.ListTaskThreadUploadFilesRequest{
			SpaceID:  req.SpaceID,
			UserID:   workbenchViewerIDFromCtx(ctx),
			ThreadID: req.ThreadID,
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	files := taskThreadUploadFilesToAPI(resp.Files)
	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadUploadFilesResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadUploadFilesData{
			Files: files,
			Count: int64(len(files)),
		},
	})
}

// UploadTaskThreadFiles .
// @router /api/workbench/task_threads/:thread_id/uploads [POST]
func UploadTaskThreadFiles(ctx context.Context, c *app.RequestContext) {
	var req threadapi.UploadTaskThreadFilesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		invalidParamRequestResponse(c, "multipart form is required")
		return
	}
	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 {
		fileHeaders = form.File["file"]
	}
	if len(fileHeaders) == 0 {
		invalidParamRequestResponse(c, "upload files are required")
		return
	}

	files := make([]appagentthread.TaskThreadUploadFileInput, 0, len(fileHeaders))
	for _, header := range fileHeaders {
		if header == nil {
			continue
		}
		opened, err := header.Open()
		if err != nil {
			internalServerErrorResponse(ctx, c, err)
			return
		}
		content, readErr := io.ReadAll(opened)
		closeErr := opened.Close()
		if readErr != nil {
			internalServerErrorResponse(ctx, c, readErr)
			return
		}
		if closeErr != nil {
			internalServerErrorResponse(ctx, c, closeErr)
			return
		}
		files = append(files, appagentthread.TaskThreadUploadFileInput{
			FileName:    header.Filename,
			Content:     content,
			ContentType: header.Header.Get("Content-Type"),
		})
	}
	resp, err := appagentthread.SVC.UploadTaskThreadFiles(
		ctx,
		&appagentthread.UploadTaskThreadFilesRequest{
			SpaceID:  req.SpaceID,
			UserID:   workbenchViewerIDFromCtx(ctx),
			ThreadID: req.ThreadID,
			Files:    files,
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.UploadTaskThreadFilesResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.UploadTaskThreadFilesData{
			Success:      true,
			Files:        taskThreadUploadFilesToAPI(resp.Files),
			Message:      "uploaded",
			SkippedFiles: resp.SkippedFiles,
		},
	})
}

// DeleteTaskThreadUploadFile .
// @router /api/workbench/task_threads/:thread_id/uploads/:filename [DELETE]
func DeleteTaskThreadUploadFile(ctx context.Context, c *app.RequestContext) {
	var req threadapi.DeleteTaskThreadUploadFileRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.DeleteTaskThreadUploadFile(
		ctx,
		&appagentthread.DeleteTaskThreadUploadFileRequest{
			SpaceID:  req.SpaceID,
			UserID:   workbenchViewerIDFromCtx(ctx),
			ThreadID: req.ThreadID,
			FileName: req.FileName,
		},
	)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.DeleteTaskThreadUploadFileResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.DeleteTaskThreadUploadFileData{
			Deleted: resp.Deleted,
			File:    taskThreadUploadFileToAPI(resp.File),
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	resp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	thread := taskThreadToAPI(resp.Thread)
	if thread != nil {
		thread.Values = taskThreadValuesToAPI(ctx, resp.Thread)
	}
	c.JSON(consts.StatusOK, &threadapi.GetTaskThreadResponse{
		Code: 0,
		Msg:  "success",
		Data: thread,
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

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

// GenerateTaskThreadSuggestions .
// @router /api/workbench/task_threads/:thread_id/suggestions [POST]
func GenerateTaskThreadSuggestions(ctx context.Context, c *app.RequestContext) {
	var req threadapi.GenerateTaskThreadSuggestionsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	if _, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID}); err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	messages := make([]appworkbench.SuggestionMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		if message == nil {
			continue
		}
		messages = append(messages, appworkbench.SuggestionMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}

	resp, err := appworkbench.SVC.GenerateSuggestions(ctx, &appworkbench.GenerateSuggestionsRequest{
		Messages:  messages,
		N:         int(req.N),
		ModelName: req.ModelName,
		ModelType: req.ModelType,
	})
	if err != nil {
		logs.CtxWarnf(ctx, "generate task thread suggestions failed: %v", err)
		c.JSON(consts.StatusOK, &threadapi.GenerateTaskThreadSuggestionsResponse{
			Suggestions: []string{},
		})
		return
	}

	if resp == nil {
		c.JSON(consts.StatusOK, &threadapi.GenerateTaskThreadSuggestionsResponse{
			Suggestions: []string{},
		})
		return
	}

	c.JSON(consts.StatusOK, &threadapi.GenerateTaskThreadSuggestionsResponse{
		Suggestions: resp.Suggestions,
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, req.RunID)

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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, req.RunID)

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
	apiEvents := taskThreadRunEventsToAPI(resp.Events)
	journalMessages, err := taskThreadRunJournalMessagesForAPIEvents(ctx, req, apiEvents)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadRunEventsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadRunEventsData{
			Events:          apiEvents,
			Total:           resp.Total,
			JournalMessages: journalMessages,
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, req.RunID)

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

// ListTaskThreadMCPRuntimeAuditEvents .
// @router /api/workbench/task_threads/:thread_id/mcp_runtime_audit_events [GET]
func ListTaskThreadMCPRuntimeAuditEvents(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadMCPRuntimeAuditEventsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ListMCPRuntimeAuditEvents(
		ctx,
		&appagentthread.ListMCPRuntimeAuditEventsRequest{
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

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadMCPRuntimeAuditEventsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadMCPRuntimeAuditEventsData{
			Events: taskThreadMCPRuntimeAuditEventsToAPI(resp.Events),
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
	afterEventID, ok := resolveTaskThreadRunEventCursor(
		req.AfterEventID,
		string(c.Request.Header.Get("Last-Event-ID")),
	)
	if !ok {
		invalidParamRequestResponse(c, "event cursor is invalid")
		return
	}
	req.AfterEventID = afterEventID
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, req.RunID)
	if !authorizeWorkbenchThreadAccess(ctx, c, req.ThreadID, req.RunID) {
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

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
		MessageContent:    req.MessageContent,
		MessageMetadata:   req.MessageMetadata,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.CreateTaskThreadRunResponse{
		Code:    0,
		Msg:     "success",
		Data:    taskThreadRunToAPI(resp.Run),
		Message: taskThreadMessageToAPI(resp.Message),
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, req.RunID)
	if !authorizeWorkbenchThreadAccess(ctx, c, req.ThreadID, req.RunID) {
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, req.RunID)

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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, req.RunID)
	if !authorizeWorkbenchThreadAccess(ctx, c, req.ThreadID, req.RunID) {
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
		if projected := taskThreadMessageToAPI(item); projected != nil {
			result = append(result, projected)
		}
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

func taskThreadRunJournalMessagesForAPIEvents(
	ctx context.Context,
	req threadapi.ListTaskThreadRunEventsRequest,
	events []*threadapi.TaskThreadRunEvent,
) ([]*threadapi.TaskThreadRunJournalMessage, error) {
	if len(events) == 0 {
		return nil, nil
	}

	messagesResp, err := appagentthread.SVC.ListMessages(ctx, &appagentthread.ListMessagesRequest{
		ThreadID: req.ThreadID,
		Page:     1,
		PageSize: 200,
	})
	if err != nil {
		return nil, err
	}

	runs := make([]*appagentthread.RunSummary, 0)
	if req.RunID > 0 {
		runResp, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{
			RunID: req.RunID,
		})
		if err != nil {
			return nil, err
		}
		if runResp != nil && runResp.Run != nil && runResp.Run.ThreadID == req.ThreadID {
			runs = append(runs, runResp.Run)
		}
	} else {
		runsResp, err := appagentthread.SVC.ListRuns(ctx, &appagentthread.ListRunsRequest{
			ThreadID:         req.ThreadID,
			IncludeChildRuns: true,
			Page:             1,
			PageSize:         200,
		})
		if err != nil {
			return nil, err
		}
		if runsResp != nil {
			runs = append(runs, runsResp.Runs...)
		}
	}

	var persistedMessages []*appagentthread.MessageSummary
	if messagesResp != nil {
		persistedMessages = messagesResp.Messages
	}
	journalMessages := appagentthread.ProjectThreadRunJournalMessages(
		runs,
		persistedMessages,
		taskThreadRunEventAPIsToSummaries(events),
	)

	return taskThreadRunJournalMessagesToAPI(journalMessages), nil
}

func taskThreadRunEventAPIsToSummaries(events []*threadapi.TaskThreadRunEvent) []*appagentthread.RunEventSummary {
	result := make([]*appagentthread.RunEventSummary, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		result = append(result, &appagentthread.RunEventSummary{
			EventID:   event.EventID,
			ThreadID:  event.ThreadID,
			RunID:     event.RunID,
			EventType: event.EventType,
			Payload:   event.Payload,
			CreatedAt: event.CreatedAt,
		})
	}
	return result
}

func taskThreadRunJournalMessagesToAPI(
	messages []*appagentthread.RunJournalMessage,
) []*threadapi.TaskThreadRunJournalMessage {
	result := make([]*threadapi.TaskThreadRunJournalMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, taskThreadRunJournalMessageToAPI(message))
	}
	return result
}

func taskThreadRunJournalMessageToAPI(
	message *appagentthread.RunJournalMessage,
) *threadapi.TaskThreadRunJournalMessage {
	projected := appagentthread.ProjectPublicRunJournalMessage(message)
	if projected == nil {
		return nil
	}

	return &threadapi.TaskThreadRunJournalMessage{
		ID:               projected.ID,
		ThreadID:         projected.ThreadID,
		RunID:            projected.RunID,
		Type:             string(projected.Type),
		Role:             string(projected.Role),
		Content:          projected.Content,
		Name:             projected.Name,
		ToolCallID:       projected.ToolCallID,
		ToolCalls:        taskThreadRunJournalToolCallsToAPI(projected.ToolCalls),
		AdditionalKwargs: taskThreadRunJournalJSONObject(projected.AdditionalKwargs),
		Usage:            taskThreadRunJournalJSONObject(projected.Usage),
		CreatedAt:        projected.CreatedAt,
		SourceEventID:    projected.SourceEventID,
	}
}

func taskThreadRunJournalToolCallsToAPI(
	toolCalls []appagentthread.PublicRunJournalToolCall,
) []*threadapi.TaskThreadRunJournalToolCall {
	result := make([]*threadapi.TaskThreadRunJournalToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		result = append(result, &threadapi.TaskThreadRunJournalToolCall{
			ID:        toolCall.ID,
			Name:      toolCall.Name,
			Type:      toolCall.Type,
			Arguments: taskThreadRunJournalJSONObject(toolCall.Arguments),
		})
	}
	return result
}

func taskThreadRunJournalJSONObject(value any) string {
	if value == nil {
		return "{}"
	}
	encoded, err := sonic.MarshalString(value)
	if err != nil || encoded == "null" || encoded == "{}" {
		return "{}"
	}
	return encoded
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

func taskThreadMCPRuntimeAuditEventsToAPI(
	events []*appagentthread.MCPRuntimeAuditEventSummary,
) []*threadapi.TaskThreadMCPRuntimeAuditEvent {
	result := make([]*threadapi.TaskThreadMCPRuntimeAuditEvent, 0, len(events))
	for _, item := range events {
		result = append(result, taskThreadMCPRuntimeAuditEventToAPI(item))
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

func taskThreadValuesToAPI(
	ctx context.Context,
	thread *appagentthread.ThreadSummary,
) *threadapi.TaskThreadValues {
	if thread == nil || thread.ThreadID <= 0 {
		return nil
	}

	todos := taskThreadTodosFromLatestCheckpoint(ctx, thread.ThreadID)
	if len(todos) == 0 {
		todos = taskThreadTodosFromJSON(thread.Metadata)
	}
	if len(todos) == 0 {
		return nil
	}

	return &threadapi.TaskThreadValues{
		Todos: todos,
	}
}

func taskThreadTodosFromLatestCheckpoint(
	ctx context.Context,
	threadID int64,
) []*threadapi.TaskThreadTodo {
	resp, err := appagentthread.SVC.ListCheckpoints(ctx, &appagentthread.ListCheckpointsRequest{
		ThreadID: threadID,
		Limit:    1,
	})
	if err != nil {
		logs.CtxWarnf(ctx, "list task thread checkpoints for values failed, thread_id=%d, err=%v", threadID, err)
		return nil
	}
	if resp == nil || len(resp.Checkpoints) == 0 || resp.Checkpoints[0] == nil {
		return nil
	}

	return taskThreadTodosFromJSON(resp.Checkpoints[0].ChannelValues)
}

func taskThreadTodosFromJSON(raw string) []*threadapi.TaskThreadTodo {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var values map[string]any
	if err := sonic.UnmarshalString(raw, &values); err != nil {
		return nil
	}

	return taskThreadTodosFromValue(taskThreadTodosValue(values))
}

func taskThreadTodosValue(values map[string]any) any {
	if values == nil {
		return nil
	}
	if todos, ok := values["todos"]; ok {
		return todos
	}
	if nested, ok := taskThreadMapValue(values["values"]); ok {
		if todos, exists := nested["todos"]; exists {
			return todos
		}
	}
	if thread, ok := taskThreadMapValue(values["thread"]); ok {
		if nested, exists := taskThreadMapValue(thread["values"]); exists {
			if todos, ok := nested["todos"]; ok {
				return todos
			}
		}
	}

	return nil
}

func taskThreadTodosFromValue(value any) []*threadapi.TaskThreadTodo {
	switch typed := value.(type) {
	case []any:
		return taskThreadTodosFromList(typed)
	case map[string]any:
		if items, ok := typed["items"].([]any); ok {
			return taskThreadTodosFromList(items)
		}
		if items, ok := typed["todos"].([]any); ok {
			return taskThreadTodosFromList(items)
		}
	}

	return nil
}

func taskThreadTodosFromList(items []any) []*threadapi.TaskThreadTodo {
	todos := make([]*threadapi.TaskThreadTodo, 0, len(items))
	for index, item := range items {
		todoMap, ok := taskThreadMapValue(item)
		if !ok {
			continue
		}

		title := taskThreadFirstString(todoMap, "title", "content", "task", "text", "description")
		if title == "" {
			continue
		}
		id := taskThreadFirstString(todoMap, "id", "todo_id", "task_id")
		if id == "" {
			id = "todo-" + strconv.Itoa(index+1)
		}
		status := taskThreadNormalizeTodoStatus(taskThreadFirstString(todoMap, "status", "state"))
		if status == "" {
			if done, ok := taskThreadBoolValue(todoMap["done"]); ok && done {
				status = "completed"
			} else {
				status = "pending"
			}
		}

		todos = append(todos, &threadapi.TaskThreadTodo{
			ID:     id,
			Title:  title,
			Status: status,
		})
	}

	return todos
}

func taskThreadMapValue(value any) (map[string]any, bool) {
	mapped, ok := value.(map[string]any)

	return mapped, ok
}

func taskThreadFirstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(taskThreadStringValue(values[key]))
		if value != "" {
			return value
		}
	}

	return ""
}

func taskThreadStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}

		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

func taskThreadBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "done", "completed", "success", "succeeded":
			return true, true
		case "false", "pending", "todo", "running", "in_progress":
			return false, true
		}
	}

	return false, false
}

func taskThreadNormalizeTodoStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done", "success", "succeeded":
		return "completed"
	case "running", "in_progress", "in-progress", "doing":
		return "running"
	case "failed", "error":
		return "failed"
	case "canceled", "cancelled":
		return "canceled"
	case "pending", "todo", "created", "queued", "":
		return status
	default:
		return "pending"
	}
}

func taskThreadArtifactToAPI(artifact *appagentthread.ArtifactSummary) *threadapi.TaskThreadArtifact {
	projected := appagentthread.ProjectPublicArtifact(artifact)
	if projected == nil {
		return nil
	}

	return &threadapi.TaskThreadArtifact{
		ArtifactID:   projected.ArtifactID,
		ThreadID:     projected.ThreadID,
		RunID:        projected.RunID,
		FileID:       projected.FileID,
		Title:        projected.Title,
		ArtifactType: projected.ArtifactType,
		VirtualPath:  projected.VirtualPath,
		ContentType:  projected.ContentType,
		SizeBytes:    projected.SizeBytes,
		PreviewMode:  string(projected.PreviewMode),
		Metadata:     projected.Metadata,
		CreatedAt:    projected.CreatedAt,
		UpdatedAt:    projected.UpdatedAt,
		DeletedAt:    projected.DeletedAt,
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

func taskThreadMCPRuntimeAuditEventToAPI(
	event *appagentthread.MCPRuntimeAuditEventSummary,
) *threadapi.TaskThreadMCPRuntimeAuditEvent {
	if event == nil {
		return nil
	}

	return &threadapi.TaskThreadMCPRuntimeAuditEvent{
		EventID:         event.EventID,
		SpaceID:         event.SpaceID,
		ThreadID:        event.ThreadID,
		RunID:           event.RunID,
		ServerID:        event.ServerID,
		RuntimeToolName: event.RuntimeToolName,
		EventType:       event.EventType,
		ErrorCode:       event.ErrorCode,
		ElapsedMillis:   event.ElapsedMillis,
		OutputBytes:     event.OutputBytes,
		CreatedAt:       event.CreatedAt,
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
	projected := appagentthread.ProjectPublicRun(run)
	if projected == nil {
		return nil
	}

	errorCode := ""
	errorMessage := ""
	if projected.Error != nil {
		errorCode = projected.Error.Code
		errorMessage = projected.Error.Message
	}

	return &threadapi.TaskThreadRun{
		RunID:             projected.RunID,
		ThreadID:          projected.ThreadID,
		ParentRunID:       projected.ParentRunID,
		SpaceID:           projected.SpaceID,
		CreatorID:         projected.CreatorID,
		AssistantID:       projected.AssistantID,
		RunKind:           string(projected.RunKind),
		Status:            string(projected.Status),
		Metadata:          projected.Metadata,
		StreamMode:        projected.StreamMode,
		MultitaskStrategy: projected.MultitaskStrategy,
		OnDisconnect:      projected.OnDisconnect,
		Durability:        projected.Durability,
		ErrorCode:         errorCode,
		ErrorMessage:      errorMessage,
		StartedAt:         projected.StartedAt,
		EndedAt:           projected.EndedAt,
		CreatedAt:         projected.CreatedAt,
		UpdatedAt:         projected.UpdatedAt,
	}
}

func taskThreadRunEventToAPI(event *appagentthread.RunEventSummary) *threadapi.TaskThreadRunEvent {
	projected := appagentthread.ProjectPublicRunEvent(event)
	if projected == nil {
		return nil
	}

	return &threadapi.TaskThreadRunEvent{
		EventID:   projected.EventID,
		ThreadID:  projected.ThreadID,
		RunID:     projected.RunID,
		EventType: projected.EventType,
		Payload:   projected.Payload,
		CreatedAt: projected.CreatedAt,
	}
}

func taskThreadTokenUsageToAPI(usage *appagentthread.TokenUsageSummary) *threadapi.TaskThreadTokenUsage {
	projected := appagentthread.ProjectPublicTokenUsage(usage)
	if projected == nil {
		return nil
	}

	return &threadapi.TaskThreadTokenUsage{
		UsageID:      projected.UsageID,
		ThreadID:     projected.ThreadID,
		RunID:        projected.RunID,
		SpaceID:      projected.SpaceID,
		Source:       string(projected.Source),
		StepID:       projected.StepID,
		StepIndex:    projected.StepIndex,
		StepName:     projected.StepName,
		ModelName:    projected.ModelName,
		Provider:     projected.Provider,
		InputTokens:  projected.InputTokens,
		OutputTokens: projected.OutputTokens,
		TotalTokens:  projected.TotalTokens,
		CostMicros:   projected.CostMicros,
		Currency:     projected.Currency,
		Estimated:    projected.Estimated,
		RawUsage:     "",
		Metadata:     "",
		CreatedAt:    projected.CreatedAt,
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
	projected := appagentthread.ProjectPublicMessage(message)
	if projected == nil {
		return nil
	}

	return &threadapi.TaskThreadMessage{
		MessageID: projected.MessageID,
		ThreadID:  projected.ThreadID,
		RunID:     projected.RunID,
		Role:      string(projected.Role),
		Content:   projected.Content,
		Metadata:  projected.Metadata,
		CreatedAt: projected.CreatedAt,
	}
}

func taskThreadUploadFilesToAPI(
	files []*appagentthread.TaskThreadUploadedFileSummary,
) []*threadapi.TaskThreadUploadFile {
	apiFiles := make([]*threadapi.TaskThreadUploadFile, 0, len(files))
	for _, file := range files {
		if mapped := taskThreadUploadFileToAPI(file); mapped != nil {
			apiFiles = append(apiFiles, mapped)
		}
	}
	return apiFiles
}

func taskThreadUploadFileToAPI(
	file *appagentthread.TaskThreadUploadedFileSummary,
) *threadapi.TaskThreadUploadFile {
	if file == nil {
		return nil
	}
	return &threadapi.TaskThreadUploadFile{
		FileID:      file.FileID,
		FileName:    file.FileName,
		Path:        file.VirtualPath,
		VirtualPath: file.VirtualPath,
		ContentType: file.ContentType,
		SizeBytes:   file.SizeBytes,
		CreatedAt:   file.CreatedAt,
	}
}

func workbenchThreadErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	if errors.Is(err, appagentthread.ErrActiveRunExists) {
		c.JSON(consts.StatusConflict, map[string]any{
			"code": consts.StatusConflict,
			"msg":  "thread already has an active run",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrUnsupportedMultitaskStrategy) {
		c.JSON(consts.StatusNotImplemented, map[string]any{
			"code": consts.StatusNotImplemented,
			"msg":  "multitask strategy is not supported",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrThreadAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "thread access denied",
		})
		return
	}
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
	if errors.Is(err, appagentthread.ErrMCPRuntimeAuditAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "mcp runtime audit access denied",
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

func workbenchThreadAccessContext(
	ctx context.Context,
	threadID int64,
	runID int64,
) context.Context {
	return appagentthread.WithThreadAccessRequest(ctx, appagentthread.ThreadAccessRequest{
		ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID,
		RunID:    runID,
	})
}

func authorizeWorkbenchThreadAccess(
	ctx context.Context,
	c *app.RequestContext,
	threadID int64,
	runID int64,
) bool {
	err := appagentthread.SVC.AuthorizeThreadAccess(ctx, appagentthread.ThreadAccessRequest{
		ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID,
		RunID:    runID,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return false
	}
	return true
}

func streamTaskThreadRunEvents(ctx context.Context, writer taskThreadRunEventStreamWriter, req threadapi.StreamTaskThreadRunEventsRequest) {
	afterEventID := req.AfterEventID
	interval := clampRunEventStreamDuration(req.IntervalMs, defaultRunEventStreamIntervalMs, minRunEventStreamIntervalMs, maxRunEventStreamIntervalMs)
	timeout := clampRunEventStreamDuration(req.TimeoutMs, defaultRunEventStreamTimeoutMs, minRunEventStreamTimeoutMs, maxRunEventStreamTimeoutMs)

	sendNewEvents := func() bool {
		for {
			cursorBeforePage := afterEventID
			resp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
				ThreadID:     req.ThreadID,
				RunID:        req.RunID,
				AfterEventID: afterEventID,
				Page:         1,
				PageSize:     taskThreadRunEventStreamPageSize,
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

			if len(resp.Events) < int(taskThreadRunEventStreamPageSize) || afterEventID == cursorBeforePage {
				return true
			}
		}
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

func resolveTaskThreadRunEventCursor(queryCursor int64, lastEventID string) (int64, bool) {
	if queryCursor < 0 {
		return 0, false
	}
	if queryCursor > 0 {
		return queryCursor, true
	}
	return parseRunEventCursor(lastEventID)
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
	logs.CtxErrorf(ctx, "task thread run event stream failed, err=%v", err)
	payload, marshalErr := sonic.Marshal(appagentthread.ProjectPublicRuntimeError("runtime_stream_error", err.Error()))
	if marshalErr != nil {
		payload = []byte(`{"code":"runtime_stream_error","message":"Agent run failed"}`)
	}
	if writeErr := writer.WriteEvent("", taskThreadRunEventStreamError, payload); writeErr != nil {
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
