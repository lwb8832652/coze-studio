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
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"

	threadapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
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
	resp, err := appagentthread.SVC.ListRuns(ctx, &appagentthread.ListRunsRequest{
		ThreadID: req.ThreadID,
		Status:   status,
		Page:     req.Page,
		PageSize: req.PageSize,
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
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Source:   appagentthread.TokenUsageSource(req.Source),
		Page:     req.Page,
		PageSize: req.PageSize,
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
			Usage:     taskThreadTokenUsagesToAPI(resp.Usage),
			Total:     resp.Total,
			Aggregate: taskThreadTokenUsageAggregateToAPI(resp.Aggregate),
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

func taskThreadRunToAPI(run *appagentthread.RunSummary) *threadapi.TaskThreadRun {
	if run == nil {
		return nil
	}

	return &threadapi.TaskThreadRun{
		RunID:             run.RunID,
		ThreadID:          run.ThreadID,
		SpaceID:           run.SpaceID,
		CreatorID:         run.CreatorID,
		AssistantID:       run.AssistantID,
		Status:            string(run.Status),
		Command:           run.Command,
		Input:             run.Input,
		Config:            run.Config,
		Context:           run.Context,
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
		Payload:   event.Payload,
		CreatedAt: event.CreatedAt,
	}
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
		RawUsage:     usage.RawUsage,
		Metadata:     usage.Metadata,
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
	internalServerErrorResponse(ctx, c, err)
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
