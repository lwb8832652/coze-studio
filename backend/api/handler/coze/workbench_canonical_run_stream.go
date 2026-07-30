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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/sse"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type canonicalRunEventStreamConfig struct {
	StreamModes        []string
	AfterEventID       int64
	CancelOnDisconnect bool
	PollInterval       time.Duration
	Timeout            time.Duration
	CancelRun          func(context.Context, int64)
}

type canonicalRunStreamWriterHandle struct {
	writer canonicalRunStreamProtocolWriter
	close  func() error
}

var canonicalRunStreamWriterFactory = func(c *app.RequestContext) canonicalRunStreamWriterHandle {
	writer := sse.NewWriter(c)
	return canonicalRunStreamWriterHandle{writer: writer, close: writer.Close}
}

func (c canonicalRunEventStreamConfig) normalized() canonicalRunEventStreamConfig {
	if c.AfterEventID < 0 {
		c.AfterEventID = 0
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Duration(defaultRunEventStreamIntervalMs) * time.Millisecond
	}
	if c.Timeout <= 0 {
		c.Timeout = time.Duration(defaultRunEventStreamTimeoutMs) * time.Millisecond
	}
	if c.CancelRun == nil {
		c.CancelRun = cancelCanonicalRunAfterStreamDisconnect
	}
	return c
}

// StreamCanonicalRun serves POST /api/workbench/threads/:thread_id/runs/stream.
// It authorizes the authenticated session principal against the path Thread and any referenced
// source Run; workspace identity comes from those resources, not X-Coze-Space-ID. It calls
// ApplicationService.CreateRun or ResumeHumanInteraction and ListRunEvents, and emits canonical SSE events.
func StreamCanonicalRun(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.stream.create", "/api/workbench/threads/:thread_id/runs/stream")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	ctx = canonicalThreadAccessContext(ctx, threadID, 0)
	if err := appagentthread.SVC.AuthorizeThreadAccess(ctx, appagentthread.ThreadAccessRequest{
		ViewerID: workbenchViewerIDFromCtx(ctx),
		SpaceID:  canonicalSpaceIDFromContext(ctx),
		ThreadID: threadID,
	}); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}

	submission, public := parseCanonicalRunSubmission(c, false)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ResponseBodyKind = "event_page"
	requestLog.LocationKind = "run_stream"
	requestLog.StreamModes = strings.Join(submission.Options.StreamModes, ",")
	requestLog.IdempotencyKeyHash = canonicalLogHash(submission.IdempotencyKey)
	submission.IdempotencyKey, public = canonicalPrincipalScopedIdempotencyKey(ctx, submission.IdempotencyKey)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	run, public, err := createCanonicalStreamRun(ctx, threadID, submission, requestLog)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("canonical run stream returned empty run"))
		return
	}
	requestLog.RunID = run.RunID
	ctx = canonicalThreadAccessContext(ctx, threadID, run.RunID)
	c.Header("Content-Location", canonicalRunPath(threadID, run.RunID))
	c.Header("Location", canonicalRunStreamPath(threadID, run.RunID))

	streamWriter := canonicalRunStreamWriterFactory(c)
	if streamWriter.writer == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("canonical run stream writer is unavailable"))
		return
	}
	setCanonicalRunStreamHeaders(c)
	defer func() {
		if streamWriter.close != nil && streamWriter.close() != nil {
			logs.CtxWarnf(ctx, "event_name=workbench.run.stream.close_failed client_contract=%s thread_id=%d run_id=%d", canonicalContractVersion, threadID, run.RunID)
		}
	}()
	streamCanonicalRunEvents(ctx, streamWriter.writer, run, canonicalRunEventStreamConfig{
		StreamModes:        submission.Options.StreamModes,
		CancelOnDisconnect: submission.Options.OnDisconnect == "cancel",
	})
}

func createCanonicalStreamRun(
	ctx context.Context,
	threadID int64,
	submission *canonicalRunSubmission,
	requestLog *canonicalRequestLog,
) (*appagentthread.RunSummary, *canonicalError, error) {
	if submission == nil {
		return nil, nil, fmt.Errorf("canonical run stream submission is required")
	}
	if submission.Resume != nil {
		requestLog.SubmissionKind = "run_resume"
		requestLog.SourceRunID = submission.Resume.SourceRunID
		run, public, err := resumeCanonicalHumanInteraction(ctx, threadID, submission.Resume, submission.IdempotencyKey)
		if public != nil {
			return nil, public, nil
		}
		if err != nil {
			return nil, nil, err
		}
		projected, projectionErr := projectCanonicalRun(run)
		if projectionErr != nil {
			return nil, nil, projectionErr
		}
		if projected == nil || run == nil || run.ThreadID != threadID {
			return nil, nil, fmt.Errorf("canonical resume stream projection returned invalid run")
		}
		accessCtx := canonicalThreadAccessContext(ctx, threadID, run.RunID)
		message, err := getCanonicalRunUserMessage(accessCtx, threadID, run.RunID)
		if err != nil {
			return nil, nil, err
		}
		if message == nil || message.ThreadID != threadID || message.RunID != run.RunID {
			return nil, nil, fmt.Errorf("canonical resume stream projection returned invalid message")
		}
		projectedMessage, err := projectCanonicalMessage(message)
		if err != nil {
			return nil, nil, err
		}
		if projectedMessage == nil {
			return nil, nil, fmt.Errorf("canonical resume stream projection returned empty message")
		}
		return run, nil, nil
	}

	requestLog.SubmissionKind = "run_turn"
	if submission.TopLevelRetry != nil {
		requestLog.SubmissionKind = "run_retry"
		requestLog.SourceRunID = submission.TopLevelRetry.SourceRunID
	}
	response, public, err := createCanonicalRunBundle(ctx, threadID, submission)
	if public != nil {
		return nil, public, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if !canonicalRunCreationResponseValid(response, threadID, submission) {
		return nil, nil, fmt.Errorf("agent thread application returned invalid run")
	}
	projectedRun, err := projectCanonicalRun(response.Run)
	if err != nil {
		return nil, nil, err
	}
	if projectedRun == nil {
		return nil, nil, fmt.Errorf("canonical run stream projection returned empty resource")
	}
	if response.Message != nil {
		projectedMessage, err := projectCanonicalMessage(response.Message)
		if err != nil {
			return nil, nil, err
		}
		if projectedMessage == nil {
			return nil, nil, fmt.Errorf("canonical run stream projection returned empty message")
		}
	}
	return response.Run, nil, nil
}

// ReconnectCanonicalRunStream serves GET /api/workbench/threads/:thread_id/runs/:run_id/stream.
// It authorizes the authenticated session principal against the path Thread and Run; workspace
// identity comes from those server-authorized resources, not X-Coze-Space-ID. It calls
// ApplicationService.GetRun and ListRunEvents, and emits replay-then-live canonical SSE events.
func ReconnectCanonicalRunStream(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.stream.reconnect", "/api/workbench/threads/:thread_id/runs/:run_id/stream")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	requestLog.ResponseBodyKind = "event_page"
	requestLog.LocationKind = "run_stream"

	afterEventID, public := canonicalReconnectRunEventCursor(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	cancelOnDisconnect, public := canonicalReconnectCancelOnDisconnect(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.AfterEventID = afterEventID

	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	projected, err := projectCanonicalRun(run)
	if err != nil || projected == nil {
		if err == nil {
			err = fmt.Errorf("canonical reconnect stream projection returned empty run")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	streamModes, public := canonicalReconnectRunStreamModes(c, run.StreamMode)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.StreamModes = strings.Join(streamModes, ",")
	c.Header("Content-Location", canonicalRunPath(threadID, runID))
	c.Header("Location", canonicalRunStreamPath(threadID, runID))

	streamWriter := canonicalRunStreamWriterFactory(c)
	if streamWriter.writer == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("canonical run stream writer is unavailable"))
		return
	}
	setCanonicalRunStreamHeaders(c)
	defer func() {
		if streamWriter.close != nil && streamWriter.close() != nil {
			logs.CtxWarnf(ctx, "event_name=workbench.run.stream.close_failed client_contract=%s thread_id=%d run_id=%d", canonicalContractVersion, threadID, runID)
		}
	}()
	streamCanonicalRunEvents(ctx, streamWriter.writer, run, canonicalRunEventStreamConfig{
		StreamModes: streamModes, AfterEventID: afterEventID, CancelOnDisconnect: cancelOnDisconnect,
	})
}

func canonicalReconnectRunEventCursor(c *app.RequestContext) (int64, *canonicalError) {
	queryRaw, _ := c.GetQuery("after_event_id")
	queryCursor, ok := parseCanonicalRunEventCursor(queryRaw)
	if !ok {
		return 0, canonicalInvalidRequest("after_event_id must be a non-negative decimal ID", "invalid_event_cursor")
	}
	headerCursor, ok := parseCanonicalRunEventCursor(string(c.GetHeader("Last-Event-ID")))
	if !ok {
		return 0, canonicalInvalidRequest("Last-Event-ID must be a non-negative decimal ID", "invalid_event_cursor")
	}
	if headerCursor > queryCursor {
		return headerCursor, nil
	}
	return queryCursor, nil
}

func canonicalReconnectCancelOnDisconnect(c *app.RequestContext) (bool, *canonicalError) {
	raw, exists := c.GetQuery("cancel_on_disconnect")
	if !exists {
		return false, nil
	}
	switch raw {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, canonicalInvalidRequest("cancel_on_disconnect must be true, false, 1, or 0", "invalid_cancel_on_disconnect")
	}
}

func canonicalReconnectRunStreamModes(
	c *app.RequestContext,
	persisted string,
) ([]string, *canonicalError) {
	rawValues := c.QueryArgs().PeekAll("stream_mode")
	if len(rawValues) == 0 {
		return canonicalRunStreamModes(persisted), nil
	}
	modes := make([]string, 0, len(rawValues))
	for _, raw := range rawValues {
		modes = append(modes, canonicalRunStreamModeQueryValues(string(raw))...)
	}
	return validateCanonicalRunStreamModes(modes)
}

// streamCanonicalRunEvents emits only persisted, publicly projected events. A
// cancellation callback is reached only after an actual writer failure confirms
// the client is gone; context completion and normal stream timeout are not proof.
func streamCanonicalRunEvents(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	run *appagentthread.RunSummary,
	config canonicalRunEventStreamConfig,
) {
	if writer == nil || run == nil || run.RunID <= 0 || run.ThreadID <= 0 {
		return
	}
	config = config.normalized()
	trackedWriter := newDisconnectTrackingRunEventStreamWriter(writer)
	terminalObserved := isWorkbenchRunTerminal(run.Status)
	defer func() {
		if !config.CancelOnDisconnect || !trackedWriter.disconnected || terminalObserved {
			return
		}
		cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runStreamDisconnectCancelTimeout)
		defer cancel()
		current, err := getCanonicalAuthorizedRun(cancelCtx, run.ThreadID, run.RunID)
		if err != nil || current == nil || isWorkbenchRunTerminal(current.Status) {
			if err != nil {
				logCanonicalRunStreamFailure(cancelCtx, "disconnect_state", run, err)
			}
			return
		}
		logs.CtxWarnf(
			cancelCtx,
			"event_name=workbench.run.stream.client_disconnected client_contract=%s thread_id=%d run_id=%d",
			canonicalContractVersion,
			run.ThreadID,
			run.RunID,
		)
		config.CancelRun(cancelCtx, run.RunID)
	}()

	if !writeCanonicalRunStreamMetadata(ctx, trackedWriter, run) {
		return
	}
	afterEventID := config.AfterEventID
	streamModes := canonicalRequestedRunStreamModes("", config.StreamModes)

	sendNewEvents := func() bool {
		for {
			cursorBeforePage := afterEventID
			response, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
				ThreadID: run.ThreadID, RunID: run.RunID, AfterEventID: afterEventID,
				Page: 1, PageSize: canonicalRunStreamPageSize,
			})
			if err != nil {
				logCanonicalRunStreamFailure(ctx, "list_events", run, err)
				writeCanonicalRunStreamError(ctx, trackedWriter, err)
				return false
			}
			if response == nil {
				err = fmt.Errorf("agent thread application returned empty run event page")
				logCanonicalRunStreamFailure(ctx, "list_events", run, err)
				writeCanonicalRunStreamError(ctx, trackedWriter, err)
				return false
			}

			events := append([]*appagentthread.RunEventSummary(nil), response.Events...)
			sort.SliceStable(events, func(i, j int) bool {
				if events[i] == nil {
					return false
				}
				if events[j] == nil {
					return true
				}
				return events[i].EventID < events[j].EventID
			})
			for _, event := range events {
				if event == nil || event.EventID <= afterEventID {
					continue
				}
				if event.ThreadID != run.ThreadID || event.RunID != run.RunID {
					err = fmt.Errorf("agent thread application returned mismatched run event")
					logCanonicalRunStreamFailure(ctx, "project_event", run, err)
					writeCanonicalRunStreamError(ctx, trackedWriter, err)
					return false
				}
				if !writeCanonicalRunStreamEvent(ctx, trackedWriter, event, streamModes) {
					return false
				}
				afterEventID = event.EventID
			}
			if len(response.Events) < int(canonicalRunStreamPageSize) || afterEventID == cursorBeforePage {
				return true
			}
		}
	}

	if !sendNewEvents() {
		return
	}
	terminal, stop := finishCanonicalRunStreamWhenTerminal(
		ctx,
		trackedWriter,
		run.ThreadID,
		run.RunID,
		sendNewEvents,
	)
	terminalObserved = terminalObserved || terminal
	if stop {
		return
	}

	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()
	timer := time.NewTimer(config.Timeout)
	defer timer.Stop()
	keepAliveTicker := time.NewTicker(runEventStreamKeepAliveInterval(config.Timeout))
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			return
		case <-keepAliveTicker.C:
			if err := trackedWriter.WriteKeepAlive(); err != nil {
				logs.CtxWarnf(
					ctx,
					"event_name=workbench.run.stream.keepalive_failed client_contract=%s thread_id=%d run_id=%d",
					canonicalContractVersion,
					run.ThreadID,
					run.RunID,
				)
				return
			}
		case <-ticker.C:
			if !sendNewEvents() {
				return
			}
			terminal, stop = finishCanonicalRunStreamWhenTerminal(
				ctx,
				trackedWriter,
				run.ThreadID,
				run.RunID,
				sendNewEvents,
			)
			terminalObserved = terminalObserved || terminal
			if stop {
				return
			}
		}
	}
}

// writeCanonicalRunStreamEvent keeps source-route behavior unchanged while
// adapting the canonical messages-tuple request mode to the fixed SDK wire form.
func writeCanonicalRunStreamEvent(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	event *appagentthread.RunEventSummary,
	streamModes map[string]struct{},
) bool {
	if _, generic := streamModes[canonicalRunStreamEventEvents]; generic {
		return writeCanonicalRunStreamProtocolEvent(ctx, writer, event, streamModes)
	}
	if _, tuple := streamModes["messages-tuple"]; !tuple {
		return writeCanonicalRunStreamProtocolEvent(ctx, writer, event, streamModes)
	}
	projected := appagentthread.ProjectPublicRunEvent(event)
	if projected == nil {
		return true
	}
	mode := canonicalPublicRunStreamEventMode(projected)
	if mode != canonicalRunStreamEventMessages && mode != "messages-tuple" {
		return writeCanonicalRunStreamProtocolEvent(ctx, writer, event, streamModes)
	}
	payload, err := sonic.Marshal(canonicalPublicRunMessageEventPayload(projected, true))
	if err != nil {
		writeCanonicalRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent(strconv.FormatInt(event.EventID, 10), canonicalRunStreamEventMessages, payload); err != nil {
		logs.CtxWarnf(ctx, "event_name=workbench.run.stream.write_failed client_contract=%s stage=message thread_id=%d run_id=%d", canonicalContractVersion, event.ThreadID, event.RunID)
		return false
	}
	return true
}

// cancelCanonicalRunAfterStreamDisconnect applies the reconnect request's
// explicit cancel choice, independent of the Run's persisted default policy.
func cancelCanonicalRunAfterStreamDisconnect(ctx context.Context, runID int64) {
	if runID <= 0 {
		return
	}
	current, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: runID})
	if err != nil || current == nil || current.Run == nil || isWorkbenchRunTerminal(current.Run.Status) {
		if err != nil {
			logCanonicalRunStreamFailure(ctx, "disconnect_cancel", &appagentthread.RunSummary{RunID: runID}, err)
		}
		return
	}
	if _, err := appagentthread.SVC.CancelRun(ctx, &appagentthread.UpdateRunStatusRequest{
		RunID:        runID,
		From:         current.Run.Status,
		ErrorCode:    "client_disconnected",
		ErrorMessage: "stream client disconnected",
	}); err != nil {
		logCanonicalRunStreamFailure(ctx, "disconnect_cancel", current.Run, err)
	}
}

func finishCanonicalRunStreamWhenTerminal(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	threadID, runID int64,
	flushEvents func() bool,
) (terminal bool, stop bool) {
	current, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		logCanonicalRunStreamFailure(ctx, "get_terminal_run", &appagentthread.RunSummary{ThreadID: threadID, RunID: runID}, err)
		writeCanonicalRunStreamError(ctx, writer, err)
		return false, true
	}
	if current == nil {
		err = fmt.Errorf("canonical run stream lost its authorized run")
		logCanonicalRunStreamFailure(ctx, "get_terminal_run", &appagentthread.RunSummary{ThreadID: threadID, RunID: runID}, err)
		writeCanonicalRunStreamError(ctx, writer, err)
		return false, true
	}
	if !isWorkbenchRunTerminal(current.Status) {
		return false, false
	}
	// Run status and its terminal event commit together. Flushing after the
	// terminal read closes the race where that commit lands after the prior page.
	if flushEvents != nil && !flushEvents() {
		return true, true
	}
	writeCanonicalRunStreamEnd(ctx, writer, current)
	return true, true
}

func logCanonicalRunStreamFailure(
	ctx context.Context,
	stage string,
	run *appagentthread.RunSummary,
	err error,
) {
	errorCode, errorClass := canonicalErrorLogFields(err)
	var threadID, runID int64
	if run != nil {
		threadID, runID = run.ThreadID, run.RunID
	}
	logs.CtxWarnf(
		ctx,
		"event_name=workbench.run.stream.failed client_contract=%s stage=%s thread_id=%d run_id=%d error_code=%s error_class=%s",
		canonicalContractVersion,
		canonicalLogEnum(stage, "list_events", "project_event", "get_terminal_run", "disconnect_state", "disconnect_cancel"),
		threadID,
		runID,
		errorCode,
		errorClass,
	)
}
