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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
	"gorm.io/gorm"

	langgraphapi "github.com/coze-dev/coze-studio/backend/api/model/agent/langgraph"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

const (
	langGraphRunStreamMetadata = "metadata"
	langGraphRunStreamValues   = "values"
	langGraphRunStreamUpdates  = "updates"
	langGraphRunStreamMessages = "messages"
	langGraphRunStreamEvents   = "events"
	langGraphRunStreamDebug    = "debug"
	langGraphRunStreamCustom   = "custom"
	langGraphRunStreamEnd      = "end"
	langGraphRunStreamError    = "error"
	langGraphRunStreamPageSize = int32(200)

	retiredLegacyTaskIDMetadataKey = "legacy_task_id"
)

type langGraphRunStreamWriter interface {
	runEventStreamWriter
}

// CreateLangGraphStatelessRun .
// @router /api/runs [POST]
func CreateLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessCreateRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, 0)
	if !langGraphInputProvided(req.Input) {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	resp, err := createLangGraphStatelessRun(ctx, req)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(resp.Run))
}

// CreateLangGraphStatelessRunStream .
// @router /api/runs/stream [POST]
func CreateLangGraphStatelessRunStream(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessCreateStreamRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, 0)
	if !langGraphInputProvided(req.Input) {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	resp, err := createLangGraphStatelessRun(ctx, statelessCreateRunRequest(req))
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}

	writer := sse.NewWriter(c)
	setLangGraphRunStreamHeaders(c)
	defer func() {
		if err := writer.Close(); err != nil {
			logs.CtxWarnf(ctx, "close langgraph stateless create run stream failed, err=%v", err)
		}
	}()

	streamCreatedLangGraphStatelessRun(ctx, writer, req, resp)
}

// WaitLangGraphStatelessRun .
// @router /api/runs/wait [POST]
func WaitLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessCreateStreamRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, 0)
	if !langGraphInputProvided(req.Input) {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	resp, err := createLangGraphStatelessRun(ctx, statelessCreateRunRequest(req))
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || resp.Run == nil {
		internalServerErrorResponse(ctx, c, errors.New("agent thread service returned empty run"))
		return
	}

	joined, err := waitLangGraphStatelessRunTerminal(ctx, langgraphapi.StatelessJoinRunRequest{
		RunID:      resp.Run.RunID,
		IntervalMs: req.IntervalMs,
		TimeoutMs:  req.TimeoutMs,
	}, resp.Run)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	state, err := langGraphRunWaitResponse(ctx, joined)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, state)
}

// GetLangGraphStatelessRun .
// @router /api/runs/:run_id [GET]
func GetLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, req.RunID)

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(run))
}

// ListLangGraphStatelessRunMessages .
// @router /api/runs/:run_id/messages [GET]
func ListLangGraphStatelessRunMessages(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessListRunMessagesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, req.RunID)

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	page, err := buildLangGraphRunMessagesPage(ctx, run, langgraphapi.ListRunMessagesRequest{
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		Limit:     req.Limit,
		BeforeSeq: req.BeforeSeq,
		AfterSeq:  req.AfterSeq,
	})
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, page)
}

// ListLangGraphStatelessRunFeedback .
// @router /api/runs/:run_id/feedback [GET]
func ListLangGraphStatelessRunFeedback(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessRunFeedbackRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, req.RunID)

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	c.JSON(consts.StatusOK, []map[string]any{})
}

// CancelLangGraphStatelessRun .
// @router /api/runs/:run_id/cancel [POST]
func CancelLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, req.RunID)

	current, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	if current == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	resp, err := appagentthread.SVC.CancelRun(ctx, &appagentthread.UpdateRunStatusRequest{
		RunID: req.RunID,
		From:  current.Status,
	})
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(resp.Run))
}

// StreamLangGraphStatelessRun .
// @router /api/runs/:run_id/stream [GET]
func StreamLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessStreamRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, req.RunID)
	if req.AfterEventID <= 0 {
		afterEventID, ok := parseRunEventCursor(string(c.Request.Header.Get("Last-Event-ID")))
		if !ok {
			invalidParamRequestResponse(c, "Last-Event-ID is invalid")
			return
		}
		req.AfterEventID = afterEventID
	}

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	writer := sse.NewWriter(c)
	setLangGraphRunStreamHeaders(c)
	defer func() {
		if err := writer.Close(); err != nil {
			logs.CtxWarnf(ctx, "close langgraph stateless run stream failed, err=%v", err)
		}
	}()

	streamLangGraphStatelessRunEvents(ctx, writer, req, run)
}

// JoinLangGraphStatelessRun .
// @router /api/runs/:run_id/join [POST]
func JoinLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessJoinRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, req.RunID)

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	joined, err := waitLangGraphStatelessRunTerminal(ctx, req, run)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(joined))
}

// JoinLangGraphStatelessRunStream .
// @router /api/runs/:run_id/join [GET]
func JoinLangGraphStatelessRunStream(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessJoinRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, req.RunID)
	if req.AfterEventID <= 0 {
		afterEventID, ok := parseRunEventCursor(string(c.Request.Header.Get("Last-Event-ID")))
		if !ok {
			invalidParamRequestResponse(c, "Last-Event-ID is invalid")
			return
		}
		req.AfterEventID = afterEventID
	}

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		langGraphErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	writer := sse.NewWriter(c)
	setLangGraphRunStreamHeaders(c)
	defer func() {
		if err := writer.Close(); err != nil {
			logs.CtxWarnf(ctx, "close langgraph stateless join stream failed, err=%v", err)
		}
	}()

	joinLangGraphStatelessRunStreamEvents(ctx, writer, req, run)
}

func buildLangGraphCreateRunRequest(req langgraphapi.CreateRunRequest) (*appagentthread.CreateRunRequest, error) {
	input, err := langGraphMarshalJSON(req.Input, "{}")
	if err != nil {
		return nil, err
	}
	command, err := langGraphMarshalJSON(req.Command, "{}")
	if err != nil {
		return nil, err
	}
	metadata, err := langGraphMarshalJSON(req.Metadata, "{}")
	if err != nil {
		return nil, err
	}
	config, err := langGraphMarshalJSON(req.Config, "{}")
	if err != nil {
		return nil, err
	}
	runContext, err := langGraphMarshalJSON(req.Context, "{}")
	if err != nil {
		return nil, err
	}
	streamMode, err := langGraphMarshalStreamMode(req.StreamMode)
	if err != nil {
		return nil, err
	}

	return &appagentthread.CreateRunRequest{
		ThreadID:          req.ThreadID,
		AssistantID:       req.AssistantID,
		Input:             input,
		Command:           command,
		Metadata:          metadata,
		Config:            config,
		Context:           runContext,
		StreamMode:        streamMode,
		MultitaskStrategy: req.MultitaskStrategy,
		OnDisconnect:      req.OnDisconnect,
		Durability:        req.Durability,
	}, nil
}

func createLangGraphStatelessRun(
	ctx context.Context,
	req langgraphapi.StatelessCreateRunRequest,
) (*appagentthread.CreateRunResponse, error) {
	thread, err := createLangGraphStatelessBackingThread(ctx, req.Metadata)
	if err != nil {
		return nil, err
	}
	if thread == nil || thread.Thread == nil {
		return nil, nil
	}

	createReq, err := buildLangGraphCreateRunRequest(langgraphapi.CreateRunRequest{
		ThreadID:          thread.Thread.ThreadID,
		AssistantID:       req.AssistantID,
		Input:             req.Input,
		Command:           req.Command,
		Metadata:          statelessRunMetadata(req.Metadata),
		Config:            req.Config,
		Context:           req.Context,
		StreamMode:        req.StreamMode,
		MultitaskStrategy: req.MultitaskStrategy,
		OnDisconnect:      req.OnDisconnect,
		Durability:        req.Durability,
	})
	if err != nil {
		return nil, err
	}

	return appagentthread.SVC.CreateRun(ctx, createReq)
}

func createLangGraphStatelessBackingThread(
	ctx context.Context,
	metadata map[string]any,
) (*appagentthread.CreateThreadResponse, error) {
	normalized := normalizeLangGraphMetadata(metadata)
	title := langGraphStringMetadata(normalized, "title")
	if title == "" {
		title = defaultWorkbenchThreadTitle
		normalized["title"] = title
	}
	source := appagentthread.ThreadSource(langGraphStringMetadata(normalized, "source"))
	if source == "" {
		source = appagentthread.ThreadSourceAPI
		normalized["source"] = string(source)
	}
	normalized["user_id"] = strconv.FormatInt(workbenchViewerIDFromCtx(ctx), 10)
	delete(normalized, "creator_id")

	metadataJSON, err := sonic.MarshalString(normalized)
	if err != nil {
		return nil, err
	}

	return appagentthread.SVC.CreateThread(ctx, &appagentthread.CreateThreadRequest{
		SpaceID:  langGraphInt64Metadata(normalized, "space_id"),
		UserID:   workbenchViewerIDFromCtx(ctx),
		Title:    title,
		Source:   source,
		Metadata: metadataJSON,
	})
}

func statelessRunMetadata(metadata map[string]any) map[string]any {
	normalized := normalizeLangGraphMetadata(metadata)
	if _, ok := normalized["source"]; !ok {
		normalized["source"] = string(appagentthread.ThreadSourceAPI)
	}
	if _, ok := normalized["title"]; !ok {
		normalized["title"] = defaultWorkbenchThreadTitle
	}

	return normalized
}

func statelessCreateRunRequest(req langgraphapi.StatelessCreateStreamRunRequest) langgraphapi.StatelessCreateRunRequest {
	return langgraphapi.StatelessCreateRunRequest{
		AssistantID:       req.AssistantID,
		Input:             req.Input,
		Command:           req.Command,
		Metadata:          req.Metadata,
		Config:            req.Config,
		Context:           req.Context,
		StreamMode:        req.StreamMode,
		MultitaskStrategy: req.MultitaskStrategy,
		OnDisconnect:      req.OnDisconnect,
		Durability:        req.Durability,
	}
}

func streamCreatedLangGraphStatelessRun(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.StatelessCreateStreamRunRequest,
	resp *appagentthread.CreateRunResponse,
) {
	if resp == nil || resp.Run == nil {
		return
	}

	streamLangGraphStatelessRunEvents(ctx, writer, langgraphapi.StatelessStreamRunRequest{
		RunID:       resp.Run.RunID,
		StreamMode:  langGraphStreamModeParam(req.StreamMode),
		StreamModes: langGraphStreamModeList(req.StreamMode),
		IntervalMs:  req.IntervalMs,
		TimeoutMs:   req.TimeoutMs,
	}, resp.Run)
}

func setLangGraphRunStreamHeaders(c *app.RequestContext) {
	c.SetContentType("text/event-stream; charset=utf-8")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("X-Accel-Buffering", "no")
}

func getLangGraphRunSummary(ctx context.Context, runID int64) (*appagentthread.RunSummary, error) {
	resp, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: runID})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Run == nil {
		return nil, nil
	}

	return resp.Run, nil
}

func langGraphRunWaitResponse(ctx context.Context, run *appagentthread.RunSummary) (map[string]any, error) {
	if run == nil {
		return map[string]any{
			"status": "not_found",
			"error":  "run is not found",
		}, nil
	}

	checkpointResp, err := appagentthread.SVC.GetLatestCheckpoint(ctx, &appagentthread.GetLatestCheckpointRequest{
		ThreadID: run.ThreadID,
	})
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if checkpointResp != nil && checkpointResp.Checkpoint != nil {
		return langGraphPublicCheckpointValues(
			appagentthread.ProjectPublicCheckpoint(checkpointResp.Checkpoint),
		), nil
	}
	errorMessage := ""
	if publicError := appagentthread.ProjectPublicRuntimeError(run.ErrorCode, run.ErrorMessage); publicError != nil {
		errorMessage = publicError.Message
	}

	return map[string]any{
		"status": langGraphRunStatus(run.Status),
		"error":  errorMessage,
	}, nil
}

func buildLangGraphRunMessagesPage(
	ctx context.Context,
	run *appagentthread.RunSummary,
	req langgraphapi.ListRunMessagesRequest,
) (*langgraphapi.RunMessagesPage, error) {
	limit := normalizeLangGraphRunMessagesLimit(req.Limit)
	messagesResp, err := appagentthread.SVC.ListMessages(ctx, &appagentthread.ListMessagesRequest{
		ThreadID: run.ThreadID,
		Page:     1,
		PageSize: 200,
	})
	if err != nil {
		return nil, err
	}
	eventsResp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
		ThreadID: run.ThreadID,
		RunID:    run.RunID,
		Page:     1,
		PageSize: 200,
	})
	if err != nil {
		return nil, err
	}

	var persistedMessages []*appagentthread.MessageSummary
	if messagesResp != nil {
		persistedMessages = messagesResp.Messages
	}
	var events []*appagentthread.RunEventSummary
	if eventsResp != nil {
		events = eventsResp.Events
	}
	journalMessages := appagentthread.ProjectRunJournalMessages(run, persistedMessages, events)
	items := make([]map[string]any, 0, len(journalMessages))
	for index, message := range journalMessages {
		seq := int64(index + 1)
		if req.BeforeSeq > 0 && seq >= req.BeforeSeq {
			continue
		}
		if req.AfterSeq > 0 && seq <= req.AfterSeq {
			continue
		}
		items = append(items, langGraphRunJournalMessageToAPI(message, seq))
	}

	hasMore := int32(len(items)) > limit
	if hasMore {
		items = items[:int(limit)]
	}

	return &langgraphapi.RunMessagesPage{
		Data:    items,
		HasMore: hasMore,
	}, nil
}

func normalizeLangGraphRunMessagesLimit(limit int32) int32 {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}

	return limit
}

func langGraphRunJournalMessageToAPI(message *appagentthread.RunJournalMessage, seq int64) map[string]any {
	projected := appagentthread.ProjectPublicRunJournalMessage(message)
	if projected == nil {
		return map[string]any{"seq": seq}
	}
	item := map[string]any{
		"id":                projected.ID,
		"seq":               seq,
		"thread_id":         strconv.FormatInt(projected.ThreadID, 10),
		"run_id":            strconv.FormatInt(projected.RunID, 10),
		"type":              string(projected.Type),
		"role":              string(projected.Role),
		"content":           projected.Content,
		"name":              projected.Name,
		"tool_call_id":      projected.ToolCallID,
		"tool_calls":        langGraphRunJournalToolCallsToAPI(projected.ToolCalls),
		"additional_kwargs": projected.AdditionalKwargs,
		"reasoning_present": projected.ReasoningPresent,
		"usage":             projected.Usage,
		"created_at":        langGraphTime(projected.CreatedAt),
		"source_event_id":   projected.SourceEventID,
	}

	return item
}

func langGraphRunJournalToolCallsToAPI(toolCalls []appagentthread.PublicRunJournalToolCall) []map[string]any {
	result := make([]map[string]any, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		result = append(result, map[string]any{
			"id":   toolCall.ID,
			"name": toolCall.Name,
			"type": toolCall.Type,
		})
	}

	return result
}

func waitLangGraphStatelessRunTerminal(
	ctx context.Context,
	req langgraphapi.StatelessJoinRunRequest,
	run *appagentthread.RunSummary,
) (*appagentthread.RunSummary, error) {
	if run == nil || isWorkbenchRunTerminal(run.Status) {
		return run, nil
	}

	interval := clampRunEventStreamDuration(req.IntervalMs, defaultRunEventStreamIntervalMs, minRunEventStreamIntervalMs, maxRunEventStreamIntervalMs)
	timeout := clampRunEventStreamDuration(req.TimeoutMs, defaultRunEventStreamTimeoutMs, minRunEventStreamTimeoutMs, maxRunEventStreamTimeoutMs)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-timer.C:
			return run, nil
		case <-ticker.C:
			current, err := getLangGraphRunSummary(ctx, req.RunID)
			if err != nil {
				return nil, err
			}
			if current == nil {
				return run, nil
			}
			run = current
			if isWorkbenchRunTerminal(run.Status) {
				return run, nil
			}
		}
	}
}

func streamLangGraphStatelessRunEvents(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.StatelessStreamRunRequest,
	run *appagentthread.RunSummary,
) {
	if run == nil {
		return
	}

	streamLangGraphRunEvents(ctx, writer, langgraphapi.StreamRunRequest{
		ThreadID:     run.ThreadID,
		RunID:        req.RunID,
		StreamMode:   req.StreamMode,
		StreamModes:  req.StreamModes,
		AfterEventID: req.AfterEventID,
		IntervalMs:   req.IntervalMs,
		TimeoutMs:    req.TimeoutMs,
	}, run)
}

func joinLangGraphStatelessRunStreamEvents(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.StatelessJoinRunRequest,
	run *appagentthread.RunSummary,
) {
	streamLangGraphStatelessRunEvents(ctx, writer, langgraphapi.StatelessStreamRunRequest{
		RunID:        req.RunID,
		AfterEventID: req.AfterEventID,
		IntervalMs:   req.IntervalMs,
		TimeoutMs:    req.TimeoutMs,
	}, run)
}

func langGraphRunToAPI(run *appagentthread.RunSummary) *langgraphapi.Run {
	projected := appagentthread.ProjectPublicRun(run)
	if projected == nil {
		return nil
	}
	errorMessage := ""
	if projected.Error != nil {
		errorMessage = projected.Error.Message
	}

	return &langgraphapi.Run{
		RunID:             strconv.FormatInt(projected.RunID, 10),
		ThreadID:          strconv.FormatInt(projected.ThreadID, 10),
		AssistantID:       projected.AssistantID,
		Status:            langGraphRunStatus(projected.Status),
		CreatedAt:         langGraphTime(projected.CreatedAt),
		UpdatedAt:         langGraphTime(projected.UpdatedAt),
		Metadata:          langGraphJSONMap(projected.Metadata),
		Input:             map[string]any{},
		Command:           map[string]any{},
		Config:            map[string]any{},
		Context:           map[string]any{},
		StreamMode:        langGraphJSONStringSlice(projected.StreamMode),
		MultitaskStrategy: projected.MultitaskStrategy,
		OnDisconnect:      projected.OnDisconnect,
		Durability:        projected.Durability,
		Error:             errorMessage,
	}
}

func streamLangGraphRunEvents(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.StreamRunRequest,
	run *appagentthread.RunSummary,
) {
	trackedWriter := newDisconnectTrackingRunEventStreamWriter(writer)
	writer = trackedWriter
	defer func() {
		if trackedWriter.disconnectedFrom(ctx) {
			cancelRunAfterStreamDisconnect(ctx, req.RunID)
		}
	}()

	afterEventID := req.AfterEventID
	interval := clampRunEventStreamDuration(req.IntervalMs, defaultRunEventStreamIntervalMs, minRunEventStreamIntervalMs, maxRunEventStreamIntervalMs)
	timeout := clampRunEventStreamDuration(req.TimeoutMs, defaultRunEventStreamTimeoutMs, minRunEventStreamTimeoutMs, maxRunEventStreamTimeoutMs)
	streamModes := langGraphRequestedStreamModes(req.StreamMode, req.StreamModes)

	if !writeLangGraphRunStreamMetadata(ctx, writer, run) {
		return
	}

	sendNewEvents := func() bool {
		for {
			cursorBeforePage := afterEventID
			resp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
				ThreadID:     req.ThreadID,
				RunID:        req.RunID,
				AfterEventID: afterEventID,
				Page:         1,
				PageSize:     langGraphRunStreamPageSize,
			})
			if err != nil {
				writeLangGraphRunStreamError(ctx, writer, err)
				return false
			}

			for _, event := range resp.Events {
				if event == nil || event.EventID <= afterEventID {
					continue
				}
				if !writeLangGraphRunStreamEvent(ctx, writer, event, streamModes) {
					return false
				}
				afterEventID = event.EventID
			}

			if len(resp.Events) < int(langGraphRunStreamPageSize) || afterEventID == cursorBeforePage {
				return true
			}
		}
	}

	if !sendNewEvents() {
		return
	}
	if writeEndWhenLangGraphRunTerminal(ctx, writer, req.RunID) {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	keepAliveTicker := time.NewTicker(runEventStreamKeepAliveInterval(timeout))
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := writer.WriteKeepAlive(); err != nil {
				logs.CtxWarnf(ctx, "probe langgraph run stream before timeout failed, err=%v", err)
			}
			return
		case <-keepAliveTicker.C:
			if err := writer.WriteKeepAlive(); err != nil {
				logs.CtxWarnf(ctx, "write langgraph run stream keepalive failed, err=%v", err)
				return
			}
		case <-ticker.C:
			if !sendNewEvents() {
				return
			}
			if writeEndWhenLangGraphRunTerminal(ctx, writer, req.RunID) {
				return
			}
		}
	}
}

func writeLangGraphRunStreamMetadata(ctx context.Context, writer langGraphRunStreamWriter, run *appagentthread.RunSummary) bool {
	if run == nil {
		return true
	}

	payload, err := sonic.Marshal(map[string]any{
		"run_id":      strconv.FormatInt(run.RunID, 10),
		"thread_id":   strconv.FormatInt(run.ThreadID, 10),
		"status":      langGraphRunStatus(run.Status),
		"attempt":     1,
		"server_time": langGraphTime(time.Now().UnixMilli()),
	})
	if err != nil {
		writeLangGraphRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent("", langGraphRunStreamMetadata, payload); err != nil {
		logs.CtxWarnf(ctx, "write langgraph run stream metadata failed, err=%v", err)
		return false
	}

	return true
}

func writeLangGraphRunStreamEvent(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	event *appagentthread.RunEventSummary,
	streamModes map[string]struct{},
) bool {
	if event == nil {
		return true
	}

	eventType, eventPayload, ok := langGraphRunStreamEventPayload(event, streamModes)
	if !ok {
		return true
	}

	payload, err := sonic.Marshal(eventPayload)
	if err != nil {
		writeLangGraphRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent(strconv.FormatInt(event.EventID, 10), eventType, payload); err != nil {
		logs.CtxWarnf(ctx, "write langgraph run stream event failed, err=%v", err)
		return false
	}

	return true
}

func writeLangGraphRunStreamEnd(ctx context.Context, writer langGraphRunStreamWriter, run *appagentthread.RunSummary) bool {
	if run == nil {
		return true
	}

	payload, err := sonic.Marshal(map[string]any{
		"run_id":    strconv.FormatInt(run.RunID, 10),
		"thread_id": strconv.FormatInt(run.ThreadID, 10),
		"status":    langGraphRunStatus(run.Status),
		"reason":    "terminal_run",
	})
	if err != nil {
		writeLangGraphRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent("", langGraphRunStreamEnd, payload); err != nil {
		logs.CtxWarnf(ctx, "write langgraph run stream end failed, err=%v", err)
		return false
	}

	return true
}

func writeLangGraphRunStreamError(ctx context.Context, writer langGraphRunStreamWriter, err error) {
	if err == nil {
		return
	}
	publicError := appagentthread.ProjectPublicRuntimeError("runtime_failed", err.Error())
	payload, marshalErr := sonic.Marshal(publicError)
	if marshalErr != nil {
		payload = []byte(`{"code":"runtime_failed","message":"Agent run failed"}`)
	}
	if writeErr := writer.WriteEvent("", langGraphRunStreamError, payload); writeErr != nil {
		logs.CtxWarnf(ctx, "write langgraph run stream error failed, err=%v", writeErr)
	}
}

func writeEndWhenLangGraphRunTerminal(ctx context.Context, writer langGraphRunStreamWriter, runID int64) bool {
	resp, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: runID})
	if err != nil {
		writeLangGraphRunStreamError(ctx, writer, err)
		return true
	}
	if resp == nil || resp.Run == nil || !isWorkbenchRunTerminal(resp.Run.Status) {
		return false
	}

	return writeLangGraphRunStreamEnd(ctx, writer, resp.Run)
}

func langGraphRunStreamEventPayload(
	event *appagentthread.RunEventSummary,
	streamModes map[string]struct{},
) (string, any, bool) {
	projected := appagentthread.ProjectPublicRunEvent(event)
	if projected == nil {
		return "", nil, false
	}
	if len(streamModes) == 0 {
		return langGraphRunStreamEvents, langGraphPublicRunGenericEventPayload(projected), true
	}
	if _, ok := streamModes[langGraphRunStreamEvents]; ok {
		return langGraphRunStreamEvents, langGraphPublicRunGenericEventPayload(projected), true
	}

	mode := langGraphPublicRunStreamEventMode(projected)
	if _, ok := streamModes[mode]; !ok {
		return "", nil, false
	}

	switch mode {
	case langGraphRunStreamUpdates:
		return mode, langGraphPublicRunUpdateEventPayload(projected), true
	case langGraphRunStreamMessages:
		return mode, langGraphPublicRunMessageEventPayload(projected, false), true
	case "messages-tuple":
		return mode, langGraphPublicRunMessageEventPayload(projected, true), true
	case langGraphRunStreamValues:
		return mode, langGraphPublicRunValuesEventPayload(projected), true
	default:
		return mode, langGraphPublicRunModeEventPayload(projected), true
	}
}

func langGraphRunGenericEventPayload(event *appagentthread.RunEventSummary) map[string]any {
	return langGraphPublicRunGenericEventPayload(appagentthread.ProjectPublicRunEvent(event))
}

func langGraphPublicRunGenericEventPayload(event *appagentthread.PublicRunEvent) map[string]any {
	if event == nil {
		return map[string]any{}
	}
	return map[string]any{
		"event_id":   strconv.FormatInt(event.EventID, 10),
		"thread_id":  strconv.FormatInt(event.ThreadID, 10),
		"run_id":     strconv.FormatInt(event.RunID, 10),
		"event_type": event.EventType,
		"payload":    langGraphRunEventPayload(event.Payload),
		"created_at": langGraphTime(event.CreatedAt),
	}
}

func langGraphPublicRunUpdateEventPayload(event *appagentthread.PublicRunEvent) map[string]any {
	payload := langGraphRunEventPayloadMap(event.Payload)
	node := langGraphStringValue(payload["node"])
	if node == "" {
		node = "agent"
	}

	update := any(payload)
	if value, ok := payload["delta"]; ok {
		update = value
	} else if value, ok := payload["update"]; ok {
		update = value
	}

	return map[string]any{
		node:       update,
		"metadata": langGraphPublicRunStreamEventMetadata(event, node),
	}
}

func langGraphPublicRunMessageEventPayload(event *appagentthread.PublicRunEvent, tuple bool) any {
	payload := langGraphRunEventPayloadMap(event.Payload)
	node := langGraphStringValue(payload["node"])
	if node == "" {
		node = "agent"
	}

	chunk := any(payload)
	if value, ok := payload["chunk"]; ok {
		chunk = value
	} else if value, ok := payload["message"]; ok {
		chunk = value
	}
	metadata := langGraphPublicRunStreamEventMetadata(event, node)
	if tuple {
		return []any{chunk, metadata}
	}

	return map[string]any{
		"chunk":    chunk,
		"metadata": metadata,
	}
}

func langGraphPublicRunValuesEventPayload(event *appagentthread.PublicRunEvent) any {
	payload := langGraphRunEventPayloadMap(event.Payload)
	if value, ok := payload["values"]; ok {
		return value
	}

	return payload
}

func langGraphPublicRunModeEventPayload(event *appagentthread.PublicRunEvent) map[string]any {
	payload := langGraphRunEventPayload(event.Payload)
	return map[string]any{
		"payload":  payload,
		"metadata": langGraphPublicRunStreamEventMetadata(event, ""),
	}
}

func langGraphPublicRunStreamEventMetadata(event *appagentthread.PublicRunEvent, node string) map[string]any {
	metadata := map[string]any{
		"event_id":   strconv.FormatInt(event.EventID, 10),
		"thread_id":  strconv.FormatInt(event.ThreadID, 10),
		"run_id":     strconv.FormatInt(event.RunID, 10),
		"event_type": event.EventType,
		"created_at": langGraphTime(event.CreatedAt),
	}
	if node != "" {
		metadata["node"] = node
	}

	return metadata
}

func langGraphPublicRunStreamEventMode(event *appagentthread.PublicRunEvent) string {
	eventType := strings.ToLower(strings.TrimSpace(event.EventType))
	switch {
	case eventType == langGraphRunStreamValues || strings.HasPrefix(eventType, "state.") || strings.HasPrefix(eventType, "checkpoint."):
		return langGraphRunStreamValues
	case eventType == langGraphRunStreamUpdates || strings.HasPrefix(eventType, "node.") || strings.HasPrefix(eventType, "step."):
		return langGraphRunStreamUpdates
	case eventType == langGraphRunStreamMessages || eventType == "messages-tuple" || strings.HasPrefix(eventType, "message.") || strings.HasPrefix(eventType, "llm."):
		if eventType == "messages-tuple" {
			return "messages-tuple"
		}
		return langGraphRunStreamMessages
	case eventType == langGraphRunStreamDebug || strings.HasPrefix(eventType, "debug."):
		return langGraphRunStreamDebug
	case eventType == langGraphRunStreamCustom || strings.HasPrefix(eventType, "custom."):
		return langGraphRunStreamCustom
	default:
		return langGraphRunStreamEvents
	}
}

func langGraphRunEventPayload(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}

	var payload any
	if err := sonic.UnmarshalString(raw, &payload); err != nil {
		return raw
	}

	return payload
}

func langGraphRunEventPayloadMap(raw string) map[string]any {
	payload := langGraphRunEventPayload(raw)
	if mapped, ok := payload.(map[string]any); ok {
		return mapped
	}

	return map[string]any{"value": payload}
}

func parseRunEventCursor(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}

	return parsed, true
}

func langGraphInputProvided(value any) bool {
	return value != nil
}

func langGraphRunStatus(status appagentthread.RunStatus) string {
	switch status {
	case appagentthread.RunStatusSucceeded:
		return "success"
	case appagentthread.RunStatusFailed:
		return "error"
	case appagentthread.RunStatusCanceled:
		return "interrupted"
	default:
		return string(status)
	}
}

func langGraphMarshalStreamMode(value any) (string, error) {
	modes, ok, err := langGraphStreamModes(value)
	if err != nil {
		return "", err
	}
	if !ok {
		modes = []string{"messages", "updates"}
	}

	return sonic.MarshalString(modes)
}

func langGraphStreamModes(value any) ([]string, bool, error) {
	if value == nil {
		return nil, false, nil
	}

	switch typed := value.(type) {
	case string:
		mode := strings.TrimSpace(typed)
		if mode == "" {
			return nil, false, nil
		}
		return []string{mode}, true, nil
	case []string:
		modes := compactLangGraphStreamModes(typed)
		return modes, len(modes) > 0, nil
	case []any:
		modes := make([]string, 0, len(typed))
		for _, item := range typed {
			mode, ok := item.(string)
			if !ok {
				return nil, false, fmt.Errorf("stream_mode must be a string or string array")
			}
			mode = strings.TrimSpace(mode)
			if mode != "" {
				modes = append(modes, mode)
			}
		}
		return modes, len(modes) > 0, nil
	default:
		return nil, false, fmt.Errorf("stream_mode must be a string or string array")
	}
}

func langGraphStreamModeParam(value any) string {
	modes := langGraphStreamModeList(value)
	if len(modes) == 0 {
		return ""
	}

	return strings.Join(modes, ",")
}

func langGraphStreamModeList(value any) []string {
	modes, ok, err := langGraphStreamModes(value)
	if err != nil || !ok {
		return nil
	}

	return modes
}

func langGraphRequestedStreamModes(raw string, values []string) map[string]struct{} {
	modes := compactLangGraphStreamModes(values)
	if len(modes) == 0 {
		modes = langGraphStreamModeQueryValues(raw)
	}
	if len(modes) == 0 {
		return nil
	}

	result := make(map[string]struct{}, len(modes))
	for _, mode := range modes {
		result[mode] = struct{}{}
	}

	return result
}

func langGraphStreamModeQueryValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var values []string
		if err := sonic.UnmarshalString(raw, &values); err == nil {
			return compactLangGraphStreamModes(values)
		}
	}

	return compactLangGraphStreamModes(strings.Split(raw, ","))
}

func compactLangGraphStreamModes(values []string) []string {
	modes := make([]string, 0, len(values))
	for _, value := range values {
		mode := strings.TrimSpace(value)
		if mode != "" {
			modes = append(modes, mode)
		}
	}

	return modes
}

func langGraphStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func langGraphMarshalJSON(value any, defaultValue string) (string, error) {
	if value == nil {
		return defaultValue, nil
	}

	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return defaultValue, nil
		}
	case []string:
		if len(typed) == 0 {
			return defaultValue, nil
		}
	}

	return sonic.MarshalString(value)
}

func langGraphJSONMap(raw string) map[string]any {
	result := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := sonic.UnmarshalString(raw, &result); err != nil {
		return map[string]any{}
	}

	return result
}

func langGraphJSONStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}

	var result any
	if err := sonic.UnmarshalString(raw, &result); err != nil {
		return []string{}
	}

	modes, ok, err := langGraphStreamModes(result)
	if err != nil || !ok {
		return []string{}
	}

	return modes
}

func langGraphPublicCheckpointValues(checkpoint *appagentthread.PublicCheckpoint) map[string]any {
	defaults := map[string]any{
		"messages":     []map[string]any{},
		"artifacts":    map[string]any{},
		"todos":        []any{},
		"memory":       map[string]any{},
		"tool_results": map[string]any{},
	}
	if checkpoint == nil {
		return defaults
	}
	values := make(map[string]any, len(checkpoint.Values)+len(defaults))
	for key, value := range checkpoint.Values {
		values[key] = value
	}
	for key, value := range defaults {
		if _, ok := values[key]; !ok {
			values[key] = value
		}
	}

	return values
}

func normalizeLangGraphMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return map[string]any{}
	}

	result := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if isRetiredLangGraphMetadataKey(key) {
			continue
		}
		result[key] = value
	}

	return result
}

func isRetiredLangGraphMetadataKey(key string) bool {
	return strings.EqualFold(strings.TrimSpace(key), retiredLegacyTaskIDMetadataKey)
}

func langGraphStringMetadata(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func langGraphInt64Metadata(metadata map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		if parsed := langGraphInt64Value(value); parsed > 0 {
			return parsed
		}
	}

	return 0
}

func langGraphInt64Value(value any) int64 {
	switch typed := value.(type) {
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		parsed, _ := strconv.ParseInt(typed.String(), 10, 64)
		return parsed
	default:
		return 0
	}
}

func langGraphTime(ms int64) string {
	if ms <= 0 {
		return ""
	}

	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}
