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
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"

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
)

type langGraphRunStreamWriter interface {
	WriteEvent(id, eventType string, data []byte) error
}

// CreateLangGraphRun .
// @router /api/threads/:thread_id/runs [POST]
func CreateLangGraphRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.CreateRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if !langGraphInputProvided(req.Input) {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	createReq, err := buildLangGraphCreateRunRequest(req)
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	resp, err := appagentthread.SVC.CreateRun(ctx, createReq)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(resp.Run))
}

// CreateLangGraphRunStream .
// @router /api/threads/:thread_id/runs/stream [POST]
func CreateLangGraphRunStream(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.CreateStreamRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if !langGraphInputProvided(req.Input) {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	resp, err := createLangGraphRunFromStreamRequest(ctx, req)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	writer := sse.NewWriter(c)
	setLangGraphRunStreamHeaders(c)
	defer func() {
		if err := writer.Close(); err != nil {
			logs.CtxWarnf(ctx, "close langgraph create run stream failed, err=%v", err)
		}
	}()

	streamCreatedLangGraphRun(ctx, writer, req, resp)
}

// CreateLangGraphStatelessRun .
// @router /api/runs [POST]
func CreateLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessCreateRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if !langGraphInputProvided(req.Input) {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	resp, err := createLangGraphStatelessRun(ctx, req)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
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
	if !langGraphInputProvided(req.Input) {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	resp, err := createLangGraphStatelessRun(ctx, statelessCreateRunRequest(req))
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
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

// ListLangGraphRuns .
// @router /api/threads/:thread_id/runs [GET]
func ListLangGraphRuns(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.ListRunsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	var status *appagentthread.RunStatus
	if strings.TrimSpace(req.Status) != "" {
		mapped := langGraphInternalRunStatus(req.Status)
		status = &mapped
	}
	pageSize := req.Limit
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int32(1)
	if req.Offset > 0 {
		page = req.Offset/pageSize + 1
	}

	resp, err := appagentthread.SVC.ListRuns(ctx, &appagentthread.ListRunsRequest{
		ThreadID: req.ThreadID,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunsToAPI(resp.Runs))
}

// GetLangGraphRun .
// @router /api/threads/:thread_id/runs/:run_id [GET]
func GetLangGraphRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.GetRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: req.RunID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || resp.Run == nil || resp.Run.ThreadID != req.ThreadID {
		invalidParamRequestResponse(c, "run_id does not belong to thread_id")
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(resp.Run))
}

// CancelLangGraphRun .
// @router /api/threads/:thread_id/runs/:run_id/cancel [POST]
func CancelLangGraphRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.CancelRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	current, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: req.RunID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if current == nil || current.Run == nil || current.Run.ThreadID != req.ThreadID {
		invalidParamRequestResponse(c, "run_id does not belong to thread_id")
		return
	}

	resp, err := appagentthread.SVC.CancelRun(ctx, &appagentthread.UpdateRunStatusRequest{
		RunID: req.RunID,
		From:  current.Run.Status,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(resp.Run))
}

// StreamLangGraphRun .
// @router /api/threads/:thread_id/runs/:run_id/stream [GET]
func StreamLangGraphRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StreamRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if req.AfterEventID <= 0 {
		afterEventID, ok := parseLangGraphLastEventID(string(c.Request.Header.Get("Last-Event-ID")))
		if !ok {
			invalidParamRequestResponse(c, "Last-Event-ID is invalid")
			return
		}
		req.AfterEventID = afterEventID
	}

	current, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: req.RunID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if current == nil || current.Run == nil || current.Run.ThreadID != req.ThreadID {
		invalidParamRequestResponse(c, "run_id does not belong to thread_id")
		return
	}

	writer := sse.NewWriter(c)
	setLangGraphRunStreamHeaders(c)
	defer func() {
		if err := writer.Close(); err != nil {
			logs.CtxWarnf(ctx, "close langgraph run stream failed, err=%v", err)
		}
	}()

	streamLangGraphRunEvents(ctx, writer, req, current.Run)
}

// JoinLangGraphRun .
// @router /api/threads/:thread_id/runs/:run_id/join [POST]
func JoinLangGraphRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.JoinRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	run, err := getLangGraphThreadRun(ctx, req.ThreadID, req.RunID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id does not belong to thread_id")
		return
	}

	joined, err := waitLangGraphRunTerminal(ctx, req, run)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(joined))
}

// JoinLangGraphRunStream .
// @router /api/threads/:thread_id/runs/:run_id/join [GET]
func JoinLangGraphRunStream(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.JoinRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if req.AfterEventID <= 0 {
		afterEventID, ok := parseLangGraphLastEventID(string(c.Request.Header.Get("Last-Event-ID")))
		if !ok {
			invalidParamRequestResponse(c, "Last-Event-ID is invalid")
			return
		}
		req.AfterEventID = afterEventID
	}

	run, err := getLangGraphThreadRun(ctx, req.ThreadID, req.RunID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id does not belong to thread_id")
		return
	}

	writer := sse.NewWriter(c)
	setLangGraphRunStreamHeaders(c)
	defer func() {
		if err := writer.Close(); err != nil {
			logs.CtxWarnf(ctx, "close langgraph join stream failed, err=%v", err)
		}
	}()

	joinLangGraphRunStreamEvents(ctx, writer, req, run)
}

// GetLangGraphStatelessRun .
// @router /api/runs/:run_id [GET]
func GetLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(run))
}

// CancelLangGraphStatelessRun .
// @router /api/runs/:run_id/cancel [POST]
func CancelLangGraphStatelessRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.StatelessRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	current, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
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
		workbenchThreadErrorResponse(ctx, c, err)
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
	if req.AfterEventID <= 0 {
		afterEventID, ok := parseLangGraphLastEventID(string(c.Request.Header.Get("Last-Event-ID")))
		if !ok {
			invalidParamRequestResponse(c, "Last-Event-ID is invalid")
			return
		}
		req.AfterEventID = afterEventID
	}

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
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

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if run == nil {
		invalidParamRequestResponse(c, "run_id is invalid")
		return
	}

	joined, err := waitLangGraphStatelessRunTerminal(ctx, req, run)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
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
	if req.AfterEventID <= 0 {
		afterEventID, ok := parseLangGraphLastEventID(string(c.Request.Header.Get("Last-Event-ID")))
		if !ok {
			invalidParamRequestResponse(c, "Last-Event-ID is invalid")
			return
		}
		req.AfterEventID = afterEventID
	}

	run, err := getLangGraphRunSummary(ctx, req.RunID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
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

func createLangGraphRunFromStreamRequest(
	ctx context.Context,
	req langgraphapi.CreateStreamRunRequest,
) (*appagentthread.CreateRunResponse, error) {
	createReq, err := buildLangGraphCreateRunRequest(langgraphapi.CreateRunRequest{
		ThreadID:          req.ThreadID,
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
	})
	if err != nil {
		return nil, err
	}

	return appagentthread.SVC.CreateRun(ctx, createReq)
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
		title = defaultLangGraphThreadTitle
		normalized["title"] = title
	}
	source := appagentthread.ThreadSource(langGraphStringMetadata(normalized, "source"))
	if source == "" {
		source = appagentthread.ThreadSourceAPI
		normalized["source"] = string(source)
	}

	metadataJSON, err := sonic.MarshalString(normalized)
	if err != nil {
		return nil, err
	}

	return appagentthread.SVC.CreateThread(ctx, &appagentthread.CreateThreadRequest{
		SpaceID:  langGraphInt64Metadata(normalized, "space_id"),
		UserID:   langGraphInt64Metadata(normalized, "user_id", "creator_id"),
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
		normalized["title"] = defaultLangGraphThreadTitle
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

func createAndStreamLangGraphRun(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.CreateStreamRunRequest,
) (*appagentthread.CreateRunResponse, error) {
	resp, err := createLangGraphRunFromStreamRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	streamCreatedLangGraphRun(ctx, writer, req, resp)

	return resp, nil
}

func createAndStreamLangGraphStatelessRun(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.StatelessCreateStreamRunRequest,
) (*appagentthread.CreateRunResponse, error) {
	resp, err := createLangGraphStatelessRun(ctx, statelessCreateRunRequest(req))
	if err != nil {
		return nil, err
	}
	streamCreatedLangGraphStatelessRun(ctx, writer, req, resp)

	return resp, nil
}

func streamCreatedLangGraphRun(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.CreateStreamRunRequest,
	resp *appagentthread.CreateRunResponse,
) {
	if resp == nil || resp.Run == nil {
		return
	}

	streamLangGraphRunEvents(ctx, writer, langgraphapi.StreamRunRequest{
		ThreadID:    req.ThreadID,
		RunID:       resp.Run.RunID,
		StreamMode:  langGraphStreamModeParam(req.StreamMode),
		StreamModes: langGraphStreamModeList(req.StreamMode),
		IntervalMs:  req.IntervalMs,
		TimeoutMs:   req.TimeoutMs,
	}, resp.Run)
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

func getLangGraphThreadRun(ctx context.Context, threadID, runID int64) (*appagentthread.RunSummary, error) {
	run, err := getLangGraphRunSummary(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil || run.ThreadID != threadID {
		return nil, nil
	}

	return run, nil
}

func waitLangGraphRunTerminal(
	ctx context.Context,
	req langgraphapi.JoinRunRequest,
	run *appagentthread.RunSummary,
) (*appagentthread.RunSummary, error) {
	if run == nil || isTaskThreadRunTerminal(run.Status) {
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
			current, err := getLangGraphThreadRun(ctx, req.ThreadID, req.RunID)
			if err != nil {
				return nil, err
			}
			if current == nil {
				return run, nil
			}
			run = current
			if isTaskThreadRunTerminal(run.Status) {
				return run, nil
			}
		}
	}
}

func joinLangGraphRunStreamEvents(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.JoinRunRequest,
	run *appagentthread.RunSummary,
) {
	streamLangGraphRunEvents(ctx, writer, langgraphapi.StreamRunRequest{
		ThreadID:     req.ThreadID,
		RunID:        req.RunID,
		AfterEventID: req.AfterEventID,
		IntervalMs:   req.IntervalMs,
		TimeoutMs:    req.TimeoutMs,
	}, run)
}

func waitLangGraphStatelessRunTerminal(
	ctx context.Context,
	req langgraphapi.StatelessJoinRunRequest,
	run *appagentthread.RunSummary,
) (*appagentthread.RunSummary, error) {
	if run == nil || isTaskThreadRunTerminal(run.Status) {
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
			if isTaskThreadRunTerminal(run.Status) {
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

func langGraphRunsToAPI(runs []*appagentthread.RunSummary) []*langgraphapi.Run {
	result := make([]*langgraphapi.Run, 0, len(runs))
	for _, run := range runs {
		result = append(result, langGraphRunToAPI(run))
	}

	return result
}

func langGraphRunToAPI(run *appagentthread.RunSummary) *langgraphapi.Run {
	if run == nil {
		return nil
	}

	return &langgraphapi.Run{
		RunID:             strconv.FormatInt(run.RunID, 10),
		ThreadID:          strconv.FormatInt(run.ThreadID, 10),
		AssistantID:       run.AssistantID,
		Status:            langGraphRunStatus(run.Status),
		CreatedAt:         langGraphTime(run.CreatedAt),
		UpdatedAt:         langGraphTime(run.UpdatedAt),
		Metadata:          langGraphJSONMap(run.Metadata),
		Input:             langGraphJSONValue(run.Input, map[string]any{}),
		Command:           langGraphJSONMap(run.Command),
		Config:            langGraphJSONMap(run.Config),
		Context:           langGraphJSONMap(run.Context),
		StreamMode:        langGraphJSONStringSlice(run.StreamMode),
		MultitaskStrategy: run.MultitaskStrategy,
		OnDisconnect:      run.OnDisconnect,
		Durability:        run.Durability,
		Error:             run.ErrorMessage,
	}
}

func streamLangGraphRunEvents(
	ctx context.Context,
	writer langGraphRunStreamWriter,
	req langgraphapi.StreamRunRequest,
	run *appagentthread.RunSummary,
) {
	afterEventID := req.AfterEventID
	interval := clampRunEventStreamDuration(req.IntervalMs, defaultRunEventStreamIntervalMs, minRunEventStreamIntervalMs, maxRunEventStreamIntervalMs)
	timeout := clampRunEventStreamDuration(req.TimeoutMs, defaultRunEventStreamTimeoutMs, minRunEventStreamTimeoutMs, maxRunEventStreamTimeoutMs)
	streamModes := langGraphRequestedStreamModes(req.StreamMode, req.StreamModes)

	if !writeLangGraphRunStreamMetadata(ctx, writer, run) {
		return
	}

	sendNewEvents := func() bool {
		page := int32(1)
		for {
			resp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
				ThreadID: req.ThreadID,
				RunID:    req.RunID,
				Page:     page,
				PageSize: langGraphRunStreamPageSize,
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

			if int64(page)*int64(langGraphRunStreamPageSize) >= resp.Total || len(resp.Events) < int(langGraphRunStreamPageSize) {
				return true
			}
			page++
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
	if writeErr := writer.WriteEvent("", langGraphRunStreamError, []byte(err.Error())); writeErr != nil {
		logs.CtxWarnf(ctx, "write langgraph run stream error failed, err=%v", writeErr)
	}
}

func writeEndWhenLangGraphRunTerminal(ctx context.Context, writer langGraphRunStreamWriter, runID int64) bool {
	resp, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: runID})
	if err != nil {
		writeLangGraphRunStreamError(ctx, writer, err)
		return true
	}
	if resp == nil || resp.Run == nil || !isTaskThreadRunTerminal(resp.Run.Status) {
		return false
	}

	return writeLangGraphRunStreamEnd(ctx, writer, resp.Run)
}

func langGraphRunStreamEventPayload(
	event *appagentthread.RunEventSummary,
	streamModes map[string]struct{},
) (string, any, bool) {
	if event == nil {
		return "", nil, false
	}
	if len(streamModes) == 0 {
		return langGraphRunStreamEvents, langGraphRunGenericEventPayload(event), true
	}
	if _, ok := streamModes[langGraphRunStreamEvents]; ok {
		return langGraphRunStreamEvents, langGraphRunGenericEventPayload(event), true
	}

	mode := langGraphRunStreamEventMode(event)
	if _, ok := streamModes[mode]; !ok {
		return "", nil, false
	}

	switch mode {
	case langGraphRunStreamUpdates:
		return mode, langGraphRunUpdateEventPayload(event), true
	case langGraphRunStreamMessages:
		return mode, langGraphRunMessageEventPayload(event, false), true
	case "messages-tuple":
		return mode, langGraphRunMessageEventPayload(event, true), true
	case langGraphRunStreamValues:
		return mode, langGraphRunValuesEventPayload(event), true
	default:
		return mode, langGraphRunModeEventPayload(event), true
	}
}

func langGraphRunGenericEventPayload(event *appagentthread.RunEventSummary) map[string]any {
	return map[string]any{
		"event_id":   strconv.FormatInt(event.EventID, 10),
		"thread_id":  strconv.FormatInt(event.ThreadID, 10),
		"run_id":     strconv.FormatInt(event.RunID, 10),
		"event_type": event.EventType,
		"payload":    langGraphRunEventPayload(event.Payload),
		"created_at": langGraphTime(event.CreatedAt),
	}
}

func langGraphRunUpdateEventPayload(event *appagentthread.RunEventSummary) map[string]any {
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
		"metadata": langGraphRunStreamEventMetadata(event, node),
	}
}

func langGraphRunMessageEventPayload(event *appagentthread.RunEventSummary, tuple bool) any {
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
	metadata := langGraphRunStreamEventMetadata(event, node)
	if tuple {
		return []any{chunk, metadata}
	}

	return map[string]any{
		"chunk":    chunk,
		"metadata": metadata,
	}
}

func langGraphRunValuesEventPayload(event *appagentthread.RunEventSummary) any {
	payload := langGraphRunEventPayloadMap(event.Payload)
	if value, ok := payload["values"]; ok {
		return value
	}

	return payload
}

func langGraphRunModeEventPayload(event *appagentthread.RunEventSummary) map[string]any {
	payload := langGraphRunEventPayload(event.Payload)
	return map[string]any{
		"payload":  payload,
		"metadata": langGraphRunStreamEventMetadata(event, ""),
	}
}

func langGraphRunStreamEventMetadata(event *appagentthread.RunEventSummary, node string) map[string]any {
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

func langGraphRunStreamEventMode(event *appagentthread.RunEventSummary) string {
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

func parseLangGraphLastEventID(raw string) (int64, bool) {
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

func langGraphInternalRunStatus(status string) appagentthread.RunStatus {
	switch strings.TrimSpace(status) {
	case "success":
		return appagentthread.RunStatusSucceeded
	case "error":
		return appagentthread.RunStatusFailed
	case "interrupted":
		return appagentthread.RunStatusCanceled
	default:
		return appagentthread.RunStatus(status)
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

func langGraphJSONValue(raw string, defaultValue any) any {
	if strings.TrimSpace(raw) == "" {
		return defaultValue
	}

	var result any
	if err := sonic.UnmarshalString(raw, &result); err != nil || result == nil {
		return defaultValue
	}

	return result
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
