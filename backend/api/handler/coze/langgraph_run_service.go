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
	langGraphRunStreamEvents   = "events"
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
	if len(req.Input) == 0 {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	input, err := langGraphMarshalJSON(req.Input, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	command, err := langGraphMarshalJSON(req.Command, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	metadata, err := langGraphMarshalJSON(req.Metadata, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	config, err := langGraphMarshalJSON(req.Config, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	runContext, err := langGraphMarshalJSON(req.Context, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	streamMode, err := langGraphMarshalJSON(req.StreamMode, `["messages","updates"]`)
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	resp, err := appagentthread.SVC.CreateRun(ctx, &appagentthread.CreateRunRequest{
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
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(resp.Run))
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
		mapped := appagentthread.RunStatus(req.Status)
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
	c.SetContentType("text/event-stream; charset=utf-8")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("X-Accel-Buffering", "no")
	defer func() {
		if err := writer.Close(); err != nil {
			logs.CtxWarnf(ctx, "close langgraph run stream failed, err=%v", err)
		}
	}()

	streamLangGraphRunEvents(ctx, writer, req, current.Run)
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
		Status:            string(run.Status),
		CreatedAt:         langGraphTime(run.CreatedAt),
		UpdatedAt:         langGraphTime(run.UpdatedAt),
		Metadata:          langGraphJSONMap(run.Metadata),
		Input:             langGraphJSONMap(run.Input),
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
				if !writeLangGraphRunStreamEvent(ctx, writer, event) {
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
		"status":      string(run.Status),
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

func writeLangGraphRunStreamEvent(ctx context.Context, writer langGraphRunStreamWriter, event *appagentthread.RunEventSummary) bool {
	if event == nil {
		return true
	}

	payload, err := sonic.Marshal(map[string]any{
		"event_id":   strconv.FormatInt(event.EventID, 10),
		"thread_id":  strconv.FormatInt(event.ThreadID, 10),
		"run_id":     strconv.FormatInt(event.RunID, 10),
		"event_type": event.EventType,
		"payload":    langGraphRunEventPayload(event.Payload),
		"created_at": langGraphTime(event.CreatedAt),
	})
	if err != nil {
		writeLangGraphRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent(strconv.FormatInt(event.EventID, 10), langGraphRunStreamEvents, payload); err != nil {
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
		"status":    string(run.Status),
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
	result := []string{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := sonic.UnmarshalString(raw, &result); err != nil {
		return []string{}
	}

	return result
}
