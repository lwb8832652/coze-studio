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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const canonicalRunStreamRequestBody = `{
	"assistant_id":"agent",
	"input":{"messages":[{"role":"user","content":"stream this turn"}]},
	"stream_mode":["events"],
	"on_disconnect":"continue"
}`

func TestStreamCanonicalRunCreatesOneRunAndStreamsPersistedEvents(t *testing.T) {
	installAgentThreadTestService(t)
	previousWriterFactory := canonicalRunStreamWriterFactory
	writer := &callbackCanonicalRunStreamWriter{}
	canonicalRunStreamWriterFactory = func(*app.RequestContext) canonicalRunStreamWriterHandle {
		return canonicalRunStreamWriterHandle{writer: writer}
	}
	t.Cleanup(func() { canonicalRunStreamWriterFactory = previousWriterFactory })
	appended := false
	writer.onEvent = func(_ string, eventType string, data []byte) {
		if appended || eventType != canonicalRunStreamEventMetadata {
			return
		}
		var metadata struct {
			RunID string `json:"run_id"`
		}
		require.NoError(t, json.Unmarshal(data, &metadata))
		runID, err := strconv.ParseInt(metadata.RunID, 10, 64)
		require.NoError(t, err)
		persisted := canonicalRunStreamCurrentRun(t, runID)
		appendCanonicalRunStreamEvent(t, persisted, "step.started", `{"step_name":"planner"}`)
		appendCanonicalRunStreamEvent(t, persisted, "step.completed", `{"step_name":"planner"}`)
		appended = true
	}
	h := canonicalRunStreamTestServer(20 * time.Millisecond)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream",
		canonicalRunStreamRequestBody,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	runs := canonicalRunsForThread(t, 1)
	require.Len(t, runs, 1)
	run := runs[0]
	require.Equal(t, int64(1), run.ThreadID)
	require.Equal(t, canonicalRunPath(1, run.RunID), response.Result().Header.Get("Content-Location"))
	require.Equal(t, canonicalRunStreamPath(1, run.RunID), response.Result().Header.Get("Location"))

	events := canonicalRunStreamEventsForTest(t, run)
	require.NotEmpty(t, events)
	body := writer.String()
	require.Equal(t, canonicalRunStreamSummaryIDs(events), canonicalRunStreamEventIDs(t, body))
	require.Equal(t, 1, strings.Count(body, "event: metadata"))
	require.Contains(t, body, `"thread_id":"1"`)
	require.Contains(t, body, `"run_id":"`+strconv.FormatInt(run.RunID, 10)+`"`)
	require.NotContains(t, body, "provider_body")
	require.NotContains(t, body, "tool_arguments")
}

func TestStreamCanonicalRunTopLevelRetryCreatesNoSecondMessage(t *testing.T) {
	installAgentThreadTestService(t)
	source := createCanonicalRunFixture(t, 1, "stream retry source")
	failCanonicalRunFixture(t, source, "runtime_failed", "failed")
	installCanonicalRunStreamRecordingWriters(t)
	h := canonicalRunStreamTestServer(20 * time.Millisecond)
	body := fmt.Sprintf(`{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"stream retry"}]},
		"stream_mode":["events"],
		"on_disconnect":"continue",
		"coze":{"attempt_kind":"retry","source_run_id":"%d"}
	}`, source.RunID)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream",
		body,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-stream-top-level-retry"},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	messages, runs := canonicalThreadMessagesAndRuns(t, 1)
	require.Len(t, messages, 1)
	require.Len(t, runs, 2)
	var retryRun *appagentthread.RunSummary
	for _, run := range runs {
		if run != nil && run.RunID != source.RunID {
			retryRun = run
			break
		}
	}
	require.NotNil(t, retryRun)
	require.Contains(t, retryRun.Input, "stream retry")
	require.Contains(t, retryRun.Metadata, `"attempt_kind":"retry"`)
	require.Contains(t, retryRun.Metadata, `"source_run_id":`+strconv.FormatInt(source.RunID, 10))
	require.NotContains(t, retryRun.Metadata, `"_message"`)
	require.Equal(t, canonicalRunStreamPath(1, retryRun.RunID), response.Result().Header.Get("Location"))
}

func TestStreamCanonicalRunReplaysIdempotentRunWithoutSecondMessage(t *testing.T) {
	installAgentThreadTestService(t)
	writers := installCanonicalRunStreamRecordingWriters(t)
	h := canonicalRunStreamTestServer(20 * time.Millisecond)
	firstResponse := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream",
		canonicalRunStreamRequestBody,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-stream-replay"},
	)
	require.Equal(t, http.StatusOK, firstResponse.Code, firstResponse.Result().Body())

	createdMessages, createdRuns := canonicalThreadMessagesAndRuns(t, 1)
	require.Len(t, createdRuns, 1)
	require.Len(t, createdMessages, 1)
	firstEvent := appendCanonicalRunStreamEvent(t, createdRuns[0], "step.started", `{"step_name":"planner"}`)
	secondEvent := appendCanonicalRunStreamEvent(
		t,
		createdRuns[0],
		"tool.completed",
		`{"tool_name":"search","tool_arguments":"secret","provider_body":"secret"}`,
	)

	secondResponse := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream",
		canonicalRunStreamRequestBody,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-stream-replay"},
	)
	require.Equal(t, http.StatusOK, secondResponse.Code, secondResponse.Result().Body())

	messages, runs := canonicalThreadMessagesAndRuns(t, 1)
	require.Len(t, runs, 1)
	require.Len(t, messages, 1)
	require.Equal(t, appagentthread.MessageRoleUser, messages[0].Role)
	require.Equal(t, createdMessages[0].MessageID, messages[0].MessageID)
	require.Equal(t, createdMessages[0].RunID, messages[0].RunID)
	require.Equal(t, runs[0].RunID, messages[0].RunID)
	require.Equal(t, canonicalRunPath(1, runs[0].RunID), firstResponse.Result().Header.Get("Content-Location"))
	require.Equal(
		t,
		firstResponse.Result().Header.Get("Content-Location"),
		secondResponse.Result().Header.Get("Content-Location"),
	)
	replayed := writers.writer(t, 1).String()
	require.Equal(t, []int64{firstEvent.EventID, secondEvent.EventID}, canonicalRunStreamEventIDs(t, replayed))
	require.NotContains(t, replayed, "tool_arguments")
	require.NotContains(t, replayed, "provider_body")
}

func TestStreamCanonicalRunAuthorizesPathBeforeReadingSubmission(t *testing.T) {
	installAgentThreadTestService(t)
	writers := installCanonicalRunStreamRecordingWriters(t)
	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(999))
	h.POST("/api/workbench/threads/:thread_id/runs/stream", StreamCanonicalRun)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream",
		`{`,
		ut.Header{Key: canonicalSpaceIDHeader, Value: "1"},
	)

	require.Equal(t, http.StatusNotFound, response.Code, response.Result().Body())
	require.Empty(t, writers.writers)
}

func TestStreamCanonicalRunAuthorizesDeclaredSpaceBeforeReadingSubmission(t *testing.T) {
	installAgentThreadTestService(t)
	writers := installCanonicalRunStreamRecordingWriters(t)
	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1001)
	h.POST("/api/workbench/threads/:thread_id/runs/stream", StreamCanonicalRun)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream",
		`{`,
	)

	require.Equal(t, http.StatusNotFound, response.Code, response.Result().Body())
	var public canonicalError
	require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
	require.Equal(t, "resource_not_found", public.Code)
	require.Empty(t, writers.writers)
}

func TestStreamCanonicalRunCommandResumeUsesExistingApplicationFlow(t *testing.T) {
	installAgentThreadTestService(t)
	sourceRunID := createInterruptedHumanInteractionRun(t)
	beforeMessages, beforeRuns := canonicalThreadMessagesAndRuns(t, 1)
	writers := installCanonicalRunStreamRecordingWriters(t)
	h := canonicalRunStreamTestServer(20 * time.Millisecond)
	payload := fmt.Sprintf(`{
		"assistant_id":"agent",
		"command":{"resume":{
			"source_run_id":"%d",
			"interrupt_id":"interrupt-1",
			"response":{
				"schema":"coze.human_interaction_response.v1",
				"interaction_id":"hi_1",
				"kind":"clarification",
				"decision":"answered",
				"answer":"last 30 days"
			}
		}}
	}`, sourceRunID)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream",
		payload,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-stream-resume-1"},
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	afterMessages, afterRuns := canonicalThreadMessagesAndRuns(t, 1)
	require.Len(t, afterRuns, len(beforeRuns)+1)
	require.Len(t, afterMessages, len(beforeMessages)+1)
	resumed := afterRuns[0]
	require.Equal(t, appagentthread.RunStatusQueued, resumed.Status)
	require.Equal(t, appagentthread.MessageRoleUser, afterMessages[0].Role)
	require.Equal(t, resumed.RunID, afterMessages[0].RunID)
	require.Equal(t, canonicalRunPath(1, resumed.RunID), response.Result().Header.Get("Content-Location"))
	require.Contains(t, writers.writer(t, 0).String(), `"run_id":"`+strconv.FormatInt(resumed.RunID, 10)+`"`)
	require.NotContains(t, writers.writer(t, 0).String(), "last 30 days")
	assertCanonicalResumePersistence(t, sourceRunID, "canonical-stream-resume-1")
}

func TestStreamCanonicalRunCommandResumeValidatesMessageProjectionBeforeSSE(t *testing.T) {
	installAgentThreadTestService(t)
	sourceRunID := createInterruptedHumanInteractionRun(t)
	appagentthread.SVC.ThreadSVC = invalidCanonicalResumeMessageThreadService{
		ThreadService: appagentthread.SVC.ThreadSVC,
	}
	writers := installCanonicalRunStreamRecordingWriters(t)
	h := canonicalRunStreamTestServer(20 * time.Millisecond)
	payload := fmt.Sprintf(`{
		"assistant_id":"agent",
		"command":{"resume":{
			"source_run_id":"%d",
			"interrupt_id":"interrupt-1",
			"response":{
				"schema":"coze.human_interaction_response.v1",
				"interaction_id":"hi_1",
				"kind":"clarification",
				"decision":"answered",
				"answer":"last 30 days"
			}
		}}
	}`, sourceRunID)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream",
		payload,
		ut.Header{Key: "Idempotency-Key", Value: "canonical-stream-resume-invalid-message"},
	)

	require.Equal(t, http.StatusInternalServerError, response.Code, response.Result().Body())
	require.Empty(t, writers.writers)
}

func TestReconnectCanonicalRunStreamReplaysAfterEventIDBeforeLiveEvents(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "reconnect replay", "continue")
	first := appendCanonicalRunStreamEvent(t, run, "step.started", `{"step_name":"planner"}`)
	second := appendCanonicalRunStreamEvent(t, run, "step.completed", `{"step_name":"planner"}`)

	var live *appagentthread.RunEventSummary
	writer := &callbackCanonicalRunStreamWriter{}
	writer.onEvent = func(id, _ string, _ []byte) {
		if live != nil || id != strconv.FormatInt(second.EventID, 10) {
			return
		}
		live = appendCanonicalRunStreamEvent(t, run, "step.completed", `{"step_name":"writer"}`)
	}
	streamCanonicalRunEvents(context.Background(), writer, run, canonicalRunEventStreamConfig{
		StreamModes: []string{"events"}, AfterEventID: first.EventID,
		PollInterval: time.Millisecond, Timeout: 20 * time.Millisecond,
	})

	require.NotNil(t, live)
	require.Equal(t, []int64{second.EventID, live.EventID}, canonicalRunStreamStringIDs(t, writer.ids))
	require.Less(t, strings.Index(writer.String(), `"step_name":"planner"`), strings.Index(writer.String(), `"step_name":"writer"`))
	require.Contains(t, writer.String(), `"thread_id":"1"`)
	require.Contains(t, writer.String(), `"run_id":"`+strconv.FormatInt(run.RunID, 10)+`"`)

	h := canonicalRunStreamTestServer(5 * time.Millisecond)
	response := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/999/runs/%d/stream", run.RunID),
		nil,
	)
	require.Equal(t, http.StatusNotFound, response.Code, response.Result().Body())
	require.NotContains(t, string(response.Result().Body()), "event: metadata")
}

func TestReconnectCanonicalRunStreamUsesLastEventIDHeader(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "header cursor", "continue")
	first := appendCanonicalRunStreamEvent(t, run, "step.started", `{"step_name":"one"}`)
	second := appendCanonicalRunStreamEvent(t, run, "step.completed", `{"step_name":"two"}`)
	cancelCanonicalRunFixture(t, run)
	allEvents := canonicalRunStreamEventsForTest(t, run)
	writers := installCanonicalRunStreamRecordingWriters(t)
	h := canonicalRunStreamTestServer(20 * time.Millisecond)

	maxCursor := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/stream?after_event_id=%d", run.RunID, first.EventID),
		nil,
		ut.Header{Key: "Last-Event-ID", Value: strconv.FormatInt(second.EventID, 10)},
	)
	require.Equal(t, http.StatusOK, maxCursor.Code, maxCursor.Result().Body())
	require.Equal(t, canonicalRunPath(1, run.RunID), maxCursor.Result().Header.Get("Content-Location"))
	require.Equal(t, canonicalRunStreamPath(1, run.RunID), maxCursor.Result().Header.Get("Location"))
	require.Equal(
		t,
		canonicalRunStreamIDsAfter(allEvents, second.EventID),
		canonicalRunStreamEventIDs(t, writers.writer(t, 0).String()),
	)

	reverseMaxCursor := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/stream?after_event_id=%d", run.RunID, second.EventID),
		nil,
		ut.Header{Key: "Last-Event-ID", Value: strconv.FormatInt(first.EventID, 10)},
	)
	require.Equal(t, http.StatusOK, reverseMaxCursor.Code, reverseMaxCursor.Result().Body())
	require.Equal(
		t,
		canonicalRunStreamIDsAfter(allEvents, second.EventID),
		canonicalRunStreamEventIDs(t, writers.writer(t, 1).String()),
	)

	headerOnly := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/stream", run.RunID),
		nil,
		ut.Header{Key: "Last-Event-ID", Value: strconv.FormatInt(second.EventID, 10)},
	)
	require.Equal(t, http.StatusOK, headerOnly.Code, headerOnly.Result().Body())
	require.Equal(t, canonicalRunPath(1, run.RunID), headerOnly.Result().Header.Get("Content-Location"))
	require.Equal(t, canonicalRunStreamPath(1, run.RunID), headerOnly.Result().Header.Get("Location"))
	require.Equal(
		t,
		canonicalRunStreamIDsAfter(allEvents, second.EventID),
		canonicalRunStreamEventIDs(t, writers.writer(t, 2).String()),
	)

	invalidHeader := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/stream?after_event_id=%d", run.RunID, first.EventID),
		nil,
		ut.Header{Key: "Last-Event-ID", Value: "invalid"},
	)
	require.Equal(t, http.StatusUnprocessableEntity, invalidHeader.Code)

	invalidCursor := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/stream?after_event_id=-1", run.RunID),
		nil,
		ut.Header{Key: "Last-Event-ID", Value: strconv.FormatInt(second.EventID, 10)},
	)
	require.Equal(t, http.StatusUnprocessableEntity, invalidCursor.Code)

	for _, value := range []string{"0", "1", "false", "true"} {
		response := ut.PerformRequest(
			h.Engine,
			http.MethodGet,
			fmt.Sprintf(
				"/api/workbench/threads/1/runs/%d/stream?cancel_on_disconnect=%s",
				run.RunID,
				value,
			),
			nil,
		)
		require.Equal(t, http.StatusOK, response.Code, "cancel_on_disconnect=%s: %s", value, response.Result().Body())
	}

	for _, value := range []string{"yes", "TRUE", "%20false%20"} {
		invalidCancelMode := ut.PerformRequest(
			h.Engine,
			http.MethodGet,
			fmt.Sprintf(
				"/api/workbench/threads/1/runs/%d/stream?cancel_on_disconnect=%s",
				run.RunID,
				value,
			),
			nil,
		)
		require.Equal(t, http.StatusUnprocessableEntity, invalidCancelMode.Code, "cancel_on_disconnect=%s", value)
	}

	modeOverrideRun := createCanonicalRunFixture(t, 1, "reconnect mode override")
	appendCanonicalRunStreamEvent(t, modeOverrideRun, "step.started", `{"step_name":"override"}`)
	cancelCanonicalRunFixture(t, modeOverrideRun)
	modeOverrideRun = canonicalRunStreamCurrentRun(t, modeOverrideRun.RunID)
	modeOverrideEvents := canonicalRunStreamEventsForTest(t, modeOverrideRun)
	modeOverride := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/stream?stream_mode=events", modeOverrideRun.RunID),
		nil,
	)
	require.Equal(t, http.StatusOK, modeOverride.Code, modeOverride.Result().Body())
	require.Equal(
		t,
		canonicalRunStreamSummaryIDs(modeOverrideEvents),
		canonicalRunStreamEventIDs(t, writers.writer(t, 7).String()),
	)

	invalidStreamMode := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		fmt.Sprintf("/api/workbench/threads/1/runs/%d/stream?stream_mode=debug", modeOverrideRun.RunID),
		nil,
	)
	require.Equal(t, http.StatusUnprocessableEntity, invalidStreamMode.Code)
}

func TestCanonicalRunStreamContinueDoesNotCancelOnDisconnect(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "continue after disconnect", "continue")
	cancelCalls := 0

	streamCanonicalRunEvents(context.Background(), failingTaskThreadRunEventStreamWriter{}, run, canonicalRunEventStreamConfig{
		StreamModes: []string{"events"}, CancelOnDisconnect: false,
		PollInterval: time.Millisecond, Timeout: time.Millisecond,
		CancelRun: func(context.Context, int64) { cancelCalls++ },
	})

	require.Zero(t, cancelCalls)
	persisted := canonicalRunStreamCurrentRun(t, run.RunID)
	require.Equal(t, appagentthread.RunStatusPending, persisted.Status)
}

func TestReconnectCanonicalRunStreamCancelOnDisconnectOverridesPersistedContinue(t *testing.T) {
	installAgentThreadTestService(t)
	previousWriterFactory := canonicalRunStreamWriterFactory
	canonicalRunStreamWriterFactory = func(*app.RequestContext) canonicalRunStreamWriterHandle {
		return canonicalRunStreamWriterHandle{writer: failingTaskThreadRunEventStreamWriter{}}
	}
	t.Cleanup(func() { canonicalRunStreamWriterFactory = previousWriterFactory })
	h := canonicalRunStreamTestServer(20 * time.Millisecond)

	for _, value := range []string{"true", "1"} {
		run := createCanonicalRunStreamFixture(t, 1, "request cancel override "+value, "continue")
		response := ut.PerformRequest(
			h.Engine,
			http.MethodGet,
			fmt.Sprintf(
				"/api/workbench/threads/1/runs/%d/stream?cancel_on_disconnect=%s",
				run.RunID,
				value,
			),
			nil,
		)

		require.Equal(t, http.StatusOK, response.Code, "cancel_on_disconnect=%s: %s", value, response.Result().Body())
		require.Equal(t, appagentthread.RunStatusCanceled, canonicalRunStreamCurrentRun(t, run.RunID).Status)
	}
}

func TestReconnectCanonicalRunStreamFalseCancelEncodingOverridesPersistedCancel(t *testing.T) {
	installAgentThreadTestService(t)
	previousWriterFactory := canonicalRunStreamWriterFactory
	canonicalRunStreamWriterFactory = func(*app.RequestContext) canonicalRunStreamWriterHandle {
		return canonicalRunStreamWriterHandle{writer: failingTaskThreadRunEventStreamWriter{}}
	}
	t.Cleanup(func() { canonicalRunStreamWriterFactory = previousWriterFactory })
	h := canonicalRunStreamTestServer(20 * time.Millisecond)

	for _, value := range []string{"false", "0"} {
		run := createCanonicalRunStreamFixture(t, 1, "request continue override "+value, "cancel")
		response := ut.PerformRequest(
			h.Engine,
			http.MethodGet,
			fmt.Sprintf(
				"/api/workbench/threads/1/runs/%d/stream?cancel_on_disconnect=%s",
				run.RunID,
				value,
			),
			nil,
		)

		require.Equal(t, http.StatusOK, response.Code, "cancel_on_disconnect=%s: %s", value, response.Result().Body())
		require.Equal(t, appagentthread.RunStatusPending, canonicalRunStreamCurrentRun(t, run.RunID).Status)
		cancelCanonicalRunFixture(t, run)
	}
}

func TestCanonicalRunStreamCancelCancelsOnlyAfterConfirmedDisconnect(t *testing.T) {
	installAgentThreadTestService(t)
	cancelCalls := 0
	cancelRun := func(ctx context.Context, runID int64) {
		cancelCalls++
		require.Positive(t, runID)
		cancelRunAfterStreamDisconnect(ctx, runID)
	}

	disconnected := createCanonicalRunStreamFixture(t, 1, "confirmed disconnect", "cancel")
	streamCanonicalRunEvents(context.Background(), failingTaskThreadRunEventStreamWriter{}, disconnected, canonicalRunEventStreamConfig{
		StreamModes: []string{"events"}, CancelOnDisconnect: true,
		PollInterval: time.Millisecond, Timeout: time.Millisecond, CancelRun: cancelRun,
	})
	require.Equal(t, 1, cancelCalls)
	require.Equal(t, appagentthread.RunStatusCanceled, canonicalRunStreamCurrentRun(t, disconnected.RunID).Status)

	timedOut := createCanonicalRunStreamFixture(t, 1, "context timeout", "cancel")
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	streamCanonicalRunEvents(canceledCtx, &recordingTaskThreadRunEventStreamWriter{}, timedOut, canonicalRunEventStreamConfig{
		StreamModes: []string{"events"}, CancelOnDisconnect: true,
		PollInterval: time.Millisecond, Timeout: time.Millisecond, CancelRun: cancelRun,
	})
	require.Equal(t, 1, cancelCalls)
	cancelCanonicalRunFixture(t, timedOut)

	terminal := createCanonicalRunStreamFixture(t, 1, "terminal disconnect", "cancel")
	cancelCanonicalRunFixture(t, terminal)
	terminal = canonicalRunStreamCurrentRun(t, terminal.RunID)
	streamCanonicalRunEvents(context.Background(), failingTaskThreadRunEventStreamWriter{}, terminal, canonicalRunEventStreamConfig{
		StreamModes: []string{"events"}, CancelOnDisconnect: true,
		PollInterval: time.Millisecond, Timeout: time.Millisecond, CancelRun: cancelRun,
	})
	require.Equal(t, 1, cancelCalls)
}

func TestCanonicalRunStreamWritesMetadataEventAndOneTerminalEnd(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "terminal stream", "continue")
	cancelCanonicalRunFixture(t, run)
	run = canonicalRunStreamCurrentRun(t, run.RunID)
	initialEvents := canonicalRunStreamEventsForTest(t, run)
	require.NotEmpty(t, initialEvents)
	lastInitialEventID := initialEvents[len(initialEvents)-1].EventID
	writer := &callbackCanonicalRunStreamWriter{}
	var lateEvent *appagentthread.RunEventSummary
	writer.onEvent = func(id, _ string, _ []byte) {
		if lateEvent != nil || id != strconv.FormatInt(lastInitialEventID, 10) {
			return
		}
		lateEvent = appendCanonicalRunStreamEvent(t, run, "step.completed", `{"step_name":"terminal_flush"}`)
	}

	streamCanonicalRunEvents(context.Background(), writer, run, canonicalRunEventStreamConfig{
		StreamModes: []string{"events"}, PollInterval: time.Millisecond, Timeout: time.Millisecond,
	})

	body := writer.String()
	require.NotNil(t, lateEvent)
	require.Equal(t, 1, strings.Count(body, "event: metadata"))
	require.Equal(t, 1, strings.Count(body, "event: end"))
	require.Less(t, strings.LastIndex(body, "event: events"), strings.Index(body, "event: end"))
	require.Contains(t, body, `"step_name":"terminal_flush"`)
	require.Contains(t, body, `"status":"canceled"`)
}

func TestCanonicalRunStreamMessagesTupleUsesMessagesEvents(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "messages tuple", "continue")
	messageEvent := appendCanonicalRunStreamEvent(
		t,
		run,
		"message.completed",
		`{"role":"assistant","content":"done","finish_reason":"stop","provider_body":"SECRET_MESSAGE_PROVIDER"}`,
	)
	llmEvent := appendCanonicalRunStreamEvent(
		t,
		run,
		"llm.token",
		`{"chunk":{"role":"assistant","content":"!","provider_body":"SECRET_CHUNK_PROVIDER"},"node":"agent"}`,
	)
	cancelCanonicalRunFixture(t, run)
	run = canonicalRunStreamCurrentRun(t, run.RunID)
	writer := &recordingTaskThreadRunEventStreamWriter{}

	streamCanonicalRunEvents(context.Background(), writer, run, canonicalRunEventStreamConfig{
		StreamModes: []string{"messages-tuple"}, PollInterval: time.Millisecond, Timeout: time.Millisecond,
	})

	body := writer.String()
	require.Equal(t, []int64{messageEvent.EventID, llmEvent.EventID}, canonicalRunStreamEventIDs(t, body))
	require.Equal(t, 2, strings.Count(body, "event: messages\n"))
	require.NotContains(t, body, "event: messages-tuple")
	require.NotContains(t, body, "SECRET_MESSAGE_PROVIDER")
	require.NotContains(t, body, "SECRET_CHUNK_PROVIDER")
	payloads := canonicalRunStreamPayloads(t, body, canonicalRunStreamEventMessages)
	require.Len(t, payloads, 2)
	for i, expected := range []struct {
		event *appagentthread.RunEventSummary
		text  string
	}{{event: messageEvent, text: "done"}, {event: llmEvent, text: "!"}} {
		tuple, ok := payloads[i].([]any)
		require.True(t, ok)
		require.Len(t, tuple, 2)
		chunk, ok := tuple[0].(map[string]any)
		require.True(t, ok)
		require.Equal(t, expected.text, chunk["content"])
		require.Equal(t, "assistant", chunk["role"])
		metadata, ok := tuple[1].(map[string]any)
		require.True(t, ok)
		require.Equal(t, strconv.FormatInt(expected.event.EventID, 10), metadata["event_id"])
		require.Equal(t, strconv.FormatInt(run.RunID, 10), metadata["run_id"])
		require.Equal(t, expected.event.EventType, metadata["event_type"])
		require.Equal(t, "agent", metadata["node"])
	}
}

func TestCanonicalRunStreamProjectsErrorsWithoutInternalDetails(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "safe stream error", "continue")
	appendCanonicalRunStreamEvent(t, run, "tool.failed", `{
		"tool_name":"search",
		"status":"failed",
		"error_code":"provider_failed",
		"arguments":{"api_key":"SECRET_TOKEN"},
		"provider_body":"SECRET_PROVIDER_BODY",
		"tool_arguments":"SECRET_TOOL_ARGUMENTS"
	}`)
	cancelCanonicalRunFixture(t, run)
	run = canonicalRunStreamCurrentRun(t, run.RunID)
	writer := &recordingTaskThreadRunEventStreamWriter{}

	streamCanonicalRunEvents(context.Background(), writer, run, canonicalRunEventStreamConfig{
		StreamModes: []string{"events"}, PollInterval: time.Millisecond, Timeout: time.Millisecond,
	})

	body := writer.String()
	require.Contains(t, body, `"event_type":"tool.failed"`)
	require.Contains(t, body, `"tool_name":"search"`)
	require.Contains(t, body, `"error_code":"provider_failed"`)
	require.Contains(t, body, `"redacted":true`)
	for _, secret := range []string{
		"provider_body", "tool_arguments", "SECRET_PROVIDER_BODY", "SECRET_TOOL_ARGUMENTS", "SECRET_TOKEN", "api_key",
	} {
		require.NotContains(t, body, secret)
	}
}

func TestReconnectCanonicalRunStreamJournalV11UsesAttemptSequenceFrames(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "journal reconnect", "continue")
	attempt := canonicalJournalStreamAttempt(run, "attempt-stream", domainentity.RunAttemptStatusCompleted)
	repository := &canonicalJournalStreamRepository{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		Events: []*domainentity.JournalEvent{{
			ID: 501, ThreadID: run.ThreadID, RunID: run.RunID, JournalRunID: run.RunID,
			AttemptID: attempt.AttemptID, Sequence: 1, EventType: "milestone.started",
			Payload:        `{"type":"milestone","data":{"milestone_id":"m1","title":"规划"}}`,
			SchemaVersion:  domainentity.JournalSchemaVersion,
			PayloadVersion: domainentity.JournalPayloadVersion,
			Visibility:     domainentity.JournalVisibilityUser, CreatedAt: run.CreatedAt,
		}},
		LatestSequence: 1,
	}}
	appagentthread.SVC.JournalQueryRepository = repository
	writers := installCanonicalRunStreamRecordingWriters(t)
	h := canonicalRunStreamTestServer(50 * time.Millisecond)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodGet,
		"/api/workbench/threads/1/runs/"+strconv.FormatInt(run.RunID, 10)+
			"/stream?journal_protocol_version=1.1&attempt_id=attempt-stream",
		"",
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	body := writers.writer(t, 0).String()
	metadataPayloads := canonicalRunStreamPayloads(t, body, canonicalRunStreamEventMetadata)
	require.Len(t, metadataPayloads, 1)
	metadata := metadataPayloads[0].(map[string]any)
	require.Equal(t, "attempt-stream", metadata["attempt_id"])
	require.Equal(t, float64(1), metadata["latest_sequence"])
	require.Equal(t, float64(1), metadata["attempt"])
	require.Equal(t, true, metadata["journal_enabled"])
	require.Equal(t, true, metadata["snapshots_enabled"])
	require.Equal(t, "1.1", metadata["journal_protocol_version"])
	require.NotEmpty(t, metadata["submit_at"])
	require.NotEmpty(t, metadata["server_time"])

	eventPayloads := canonicalRunStreamPayloads(t, body, canonicalRunStreamEventEvents)
	require.Len(t, eventPayloads, 1)
	frame := eventPayloads[0].(map[string]any)
	require.Equal(t, "event", frame["kind"])
	event := frame["event"].(map[string]any)
	require.Equal(t, "501", event["event_id"])
	require.Equal(t, float64(1), event["sequence"])
	require.IsType(t, map[string]any{}, event["payload"])
	require.Equal(t, []int64{501}, canonicalRunStreamEventIDs(t, body))
	require.Equal(t, 1, strings.Count(body, "event: end\n"))
	require.NotContains(t, body, "internal_reason")
	require.NotEmpty(t, repository.requests)
	require.Equal(t, "attempt-stream", repository.requests[0].AttemptID)
}

func TestCanonicalJournalStreamAcceptsRecoveryExecutionRunEvents(t *testing.T) {
	run := &appagentthread.RunSummary{ThreadID: 1, RunID: 10, CreatedAt: 1_000}
	attempt := &domainentity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 20,
		AttemptID: "attempt-recovery", Ordinal: 2,
		Status: domainentity.RunAttemptStatusCompleted, NextSequence: 2,
		SnapshotsEnabled: true,
		ProjectionState:  domainentity.JournalProjectionStateHealthy,
		CreatedAt:        1_000, StartedAt: pointerToInt64(1_000),
	}
	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamCanonicalJournalEvents(context.Background(), writer, run, &appagentthread.JournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		Events: []*domainentity.JournalEvent{{
			ID: 501, ThreadID: 1, RunID: 20, JournalRunID: 10,
			AttemptID: attempt.AttemptID, Sequence: 1, EventType: "action.completed",
			Payload:        `{"type":"generic","data":{}}`,
			SchemaVersion:  domainentity.JournalSchemaVersion,
			PayloadVersion: domainentity.JournalPayloadVersion,
			Visibility:     domainentity.JournalVisibilityUser, CreatedAt: 1_000,
		}},
		LatestSequence: 1, JournalEnabled: true, SnapshotsEnabled: true,
	}, canonicalJournalStreamConfig{ViewerID: 2, SpaceID: 1, AttemptID: attempt.AttemptID})

	body := writer.String()
	require.Contains(t, body, "event: events\n")
	require.Contains(t, body, `"run_id":"10"`)
	require.Equal(t, 1, strings.Count(body, "event: end\n"))
	require.NotContains(t, body, "JOURNAL_EVENT_GAP")
}

func TestCanonicalJournalStreamResumesFromResolvedEventIDSequence(t *testing.T) {
	run := &appagentthread.RunSummary{ThreadID: 1, RunID: 10, CreatedAt: 1_000}
	attempt := &domainentity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "attempt-event-cursor", Ordinal: 1,
		Status: domainentity.RunAttemptStatusCompleted, NextSequence: 3,
		ProjectionState: domainentity.JournalProjectionStateHealthy, CreatedAt: 1_000,
	}
	writer := &recordingTaskThreadRunEventStreamWriter{}
	streamCanonicalJournalEvents(context.Background(), writer, run, &appagentthread.JournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		Events: []*domainentity.JournalEvent{{
			ID: 502, ThreadID: 1, RunID: 10, JournalRunID: 10,
			AttemptID: attempt.AttemptID, Sequence: 2, EventType: "action.completed",
			Payload:        `{"type":"generic","data":{}}`,
			SchemaVersion:  domainentity.JournalSchemaVersion,
			PayloadVersion: domainentity.JournalPayloadVersion,
			Visibility:     domainentity.JournalVisibilityUser, CreatedAt: 1_000,
		}},
		LatestSequence: 2, ResolvedAfterSequence: 1,
		JournalEnabled: true,
	}, canonicalJournalStreamConfig{
		ViewerID: 2, SpaceID: 1, AttemptID: attempt.AttemptID, AfterEventID: 501,
	})

	body := writer.String()
	require.Contains(t, body, "id: 502\n")
	require.Equal(t, 1, strings.Count(body, "event: end\n"))
	require.NotContains(t, body, "JOURNAL_EVENT_GAP")
}

func TestStreamCanonicalRunIgnoresJournalProtocolQuery(t *testing.T) {
	installAgentThreadTestService(t)
	repository := &canonicalJournalStreamRepository{err: errors.New("must not be called")}
	appagentthread.SVC.JournalQueryRepository = repository
	writers := installCanonicalRunStreamRecordingWriters(t)
	h := canonicalRunStreamTestServer(20 * time.Millisecond)

	response := performCanonicalRunJSONRequest(
		t,
		h,
		http.MethodPost,
		"/api/workbench/threads/1/runs/stream?journal_protocol_version=1.1",
		canonicalRunStreamRequestBody,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	metadataPayloads := canonicalRunStreamPayloads(
		t, writers.writer(t, 0).String(), canonicalRunStreamEventMetadata,
	)
	require.Len(t, metadataPayloads, 1)
	metadata := metadataPayloads[0].(map[string]any)
	require.Equal(t, float64(1), metadata["attempt"])
	require.NotContains(t, metadata, "attempt_id")
	require.NotContains(t, metadata, "journal_enabled")
	require.Empty(t, repository.requests)
}

func TestCanonicalJournalStreamHeartbeatRenewsAdmissionLease(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "journal heartbeat", "continue")
	attempt := canonicalJournalStreamAttempt(run, "attempt-heartbeat", domainentity.RunAttemptStatusRunning)
	repository := &canonicalJournalStreamRepository{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
	}}
	appagentthread.SVC.JournalQueryRepository = repository
	lease := &recordingCanonicalJournalAdmissionLease{}
	writer := &recordingTaskThreadRunEventStreamWriter{}
	initial, err := appagentthread.SVC.GetJournalBootstrap(
		canonicalThreadAccessContext(context.Background(), run.ThreadID, run.RunID),
		appagentthread.GetJournalBootstrapRequest{
			ViewerID: 2, SpaceID: 1, ThreadID: run.ThreadID, RunID: run.RunID,
			AttemptID: attempt.AttemptID, Limit: 200,
		},
	)
	require.NoError(t, err)

	streamCanonicalJournalEvents(context.Background(), writer, run, initial, canonicalJournalStreamConfig{
		ViewerID: 2, SpaceID: 1, AttemptID: attempt.AttemptID,
		PollInterval: 50 * time.Millisecond, HeartbeatInterval: time.Millisecond,
		Timeout: 6 * time.Millisecond, AdmissionLease: lease,
	})

	body := writer.String()
	require.GreaterOrEqual(t, strings.Count(body, "event: heartbeat\n"), 2)
	require.GreaterOrEqual(t, lease.renewCalls, 2)
	require.GreaterOrEqual(t, len(repository.requests), 3)
}

func TestCanonicalJournalStreamStopsWithoutFurtherFramesAfterAccessRevocation(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "journal revoked", "continue")
	attempt := canonicalJournalStreamAttempt(run, "attempt-revoked", domainentity.RunAttemptStatusRunning)
	repository := &canonicalJournalStreamRepository{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		Events: []*domainentity.JournalEvent{{
			ID: 701, ThreadID: run.ThreadID, RunID: run.RunID, JournalRunID: run.RunID,
			AttemptID: attempt.AttemptID, Sequence: 1, EventType: "action.completed",
			Payload:        `{"type":"generic","data":{"action_id":"a1","operation":"read","target":"PRD","display_verb_running":"正在读取 PRD","display_verb_completed":"已读取 PRD"}}`,
			SchemaVersion:  domainentity.JournalSchemaVersion,
			PayloadVersion: domainentity.JournalPayloadVersion,
			Visibility:     domainentity.JournalVisibilityUser, CreatedAt: run.CreatedAt,
		}},
		LatestSequence: 1,
	}}
	appagentthread.SVC.JournalQueryRepository = repository
	authorizer := &revokedCanonicalJournalThreadAuthorizer{}
	appagentthread.SVC.ThreadAuthorizer = authorizer
	writer := &recordingTaskThreadRunEventStreamWriter{}

	streamCanonicalJournalEvents(context.Background(), writer, run, &appagentthread.JournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		JournalEnabled: true, SnapshotsEnabled: true,
	}, canonicalJournalStreamConfig{
		ViewerID: 2, SpaceID: 1, AttemptID: attempt.AttemptID,
		PollInterval: time.Millisecond, HeartbeatInterval: 50 * time.Millisecond,
		Timeout: 10 * time.Millisecond,
	})

	body := writer.String()
	require.Equal(t, 1, strings.Count(body, "event: metadata\n"))
	require.NotContains(t, body, "event: events\n")
	require.NotContains(t, body, "event: control\n")
	require.NotContains(t, body, "event: heartbeat\n")
	require.GreaterOrEqual(t, authorizer.calls, 1)
	require.Empty(t, repository.requests)
}

func TestCanonicalJournalStreamStopsAfterWorkspaceAccessRevocation(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "journal workspace revoked", "continue")
	attempt := canonicalJournalStreamAttempt(run, "attempt-workspace-revoked", domainentity.RunAttemptStatusRunning)
	repository := &canonicalJournalStreamRepository{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
	}}
	appagentthread.SVC.JournalQueryRepository = repository
	workspaceAuthorizer := &revokedCanonicalJournalWorkspaceAuthorizer{}
	appagentthread.SVC.WorkspaceAuthorizer = workspaceAuthorizer
	writer := &recordingTaskThreadRunEventStreamWriter{}

	streamCanonicalJournalEvents(context.Background(), writer, run, &appagentthread.JournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		JournalEnabled: true, SnapshotsEnabled: true,
	}, canonicalJournalStreamConfig{
		ViewerID: 2, SpaceID: 1, AttemptID: attempt.AttemptID,
		PollInterval: time.Millisecond, HeartbeatInterval: 50 * time.Millisecond,
		Timeout: 10 * time.Millisecond,
	})

	body := writer.String()
	require.Equal(t, 1, strings.Count(body, "event: metadata\n"))
	require.NotContains(t, body, "event: events\n")
	require.NotContains(t, body, "event: control\n")
	require.NotContains(t, body, "event: heartbeat\n")
	require.GreaterOrEqual(t, workspaceAuthorizer.calls, 1)
	require.Empty(t, repository.requests)
}

func TestCanonicalJournalStreamStopsWithoutFurtherFramesAfterSourceRevocation(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "journal source revoked", "continue")
	attempt := canonicalJournalStreamAttempt(run, "attempt-source-revoked", domainentity.RunAttemptStatusRunning)
	repository := &canonicalJournalStreamRepository{err: appagentthread.ErrJournalSnapshotNoPermission}
	appagentthread.SVC.JournalQueryRepository = repository
	writer := &recordingTaskThreadRunEventStreamWriter{}

	streamCanonicalJournalEvents(context.Background(), writer, run, &appagentthread.JournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		JournalEnabled: true, SnapshotsEnabled: true,
	}, canonicalJournalStreamConfig{
		ViewerID: 2, SpaceID: 1, AttemptID: attempt.AttemptID,
		PollInterval: time.Millisecond, HeartbeatInterval: 50 * time.Millisecond,
		Timeout: 10 * time.Millisecond,
	})

	body := writer.String()
	require.Equal(t, 1, strings.Count(body, "event: metadata\n"))
	require.NotContains(t, body, "event: events\n")
	require.NotContains(t, body, "event: control\n")
	require.NotContains(t, body, "event: heartbeat\n")
	require.NotEmpty(t, repository.requests)
}

func TestCanonicalJournalStreamStopsWithProjectionControl(t *testing.T) {
	run := &appagentthread.RunSummary{ThreadID: 1, RunID: 2, CreatedAt: 1_000}
	for _, test := range []struct {
		name        string
		state       domainentity.JournalProjectionState
		controlType string
	}{
		{name: "disabled", state: domainentity.JournalProjectionStateDisabled, controlType: "journal_disabled"},
		{name: "degraded", state: domainentity.JournalProjectionStateDegraded, controlType: "journal_degraded"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := &recordingTaskThreadRunEventStreamWriter{}
			attempt := &domainentity.RunAttempt{
				AttemptID: "attempt-control", ThreadID: 1, JournalRunID: 2,
				Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
				ProjectionState: test.state, CreatedAt: 1_000, NextSequence: 1,
			}
			streamCanonicalJournalEvents(context.Background(), writer, run, &appagentthread.JournalBootstrapResult{
				Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
				JournalEnabled: test.state != domainentity.JournalProjectionStateDisabled,
			}, canonicalJournalStreamConfig{ViewerID: 2, SpaceID: 1, AttemptID: attempt.AttemptID})
			body := writer.String()
			controls := canonicalRunStreamPayloads(t, body, "control")
			require.Len(t, controls, 1)
			control := controls[0].(map[string]any)["control"].(map[string]any)
			require.Equal(t, test.controlType, control["type"])
			require.NotContains(t, body, "internal_reason")
			require.NotContains(t, body, "event: events\n")
		})
	}
}

func TestBoundedCanonicalJournalStreamWriterRejectsSlowConsumer(t *testing.T) {
	underlying := &blockingCanonicalJournalStreamWriter{
		started: make(chan struct{}), release: make(chan struct{}),
	}
	writer := newBoundedCanonicalJournalStreamWriter(underlying, 1)
	require.NoError(t, writer.WriteEvent("1", "events", []byte(`{"one":1}`)))
	<-underlying.started
	require.NoError(t, writer.WriteEvent("2", "events", []byte(`{"two":2}`)))
	require.ErrorIs(
		t,
		writer.WriteEvent("3", "events", []byte(`{"three":3}`)),
		errCanonicalJournalSlowConsumer,
	)
	close(underlying.release)
	require.NoError(t, writer.Close())
}

func TestBoundedCanonicalJournalStreamDropsQueuedEventsAfterSourceRevocation(t *testing.T) {
	installAgentThreadTestService(t)
	run := createCanonicalRunStreamFixture(t, 1, "journal queued source revoked", "continue")
	attempt := canonicalJournalStreamAttempt(run, "attempt-queued-revoked", domainentity.RunAttemptStatusRunning)
	repository := &canonicalJournalStreamRepository{err: appagentthread.ErrJournalSnapshotNoPermission}
	appagentthread.SVC.JournalQueryRepository = repository
	underlying := &blockingCanonicalJournalStreamWriter{
		started: make(chan struct{}), release: make(chan struct{}),
	}
	writer := newBoundedCanonicalJournalStreamWriter(underlying, 4)

	streamCanonicalJournalEvents(context.Background(), writer, run, &appagentthread.JournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
		Events: []*domainentity.JournalEvent{{
			ID: 701, ThreadID: run.ThreadID, RunID: run.RunID, JournalRunID: run.RunID,
			AttemptID: attempt.AttemptID, Sequence: 1, EventType: "action.completed",
			Payload:        `{"type":"generic","data":{}}`,
			SchemaVersion:  domainentity.JournalSchemaVersion,
			PayloadVersion: domainentity.JournalPayloadVersion,
			Visibility:     domainentity.JournalVisibilityUser, CreatedAt: run.CreatedAt,
		}},
		LatestSequence: 1, JournalEnabled: true, SnapshotsEnabled: true,
	}, canonicalJournalStreamConfig{
		ViewerID: 2, SpaceID: 1, AttemptID: attempt.AttemptID,
		PollInterval: time.Millisecond, HeartbeatInterval: 50 * time.Millisecond,
		Timeout: 10 * time.Millisecond,
	})

	close(underlying.release)
	require.NoError(t, writer.Close())
	require.NotContains(t, underlying.EventTypes(), canonicalRunStreamEventEvents)
	require.NotEmpty(t, repository.requests)
}

type callbackCanonicalRunStreamWriter struct {
	recordingTaskThreadRunEventStreamWriter
	onEvent func(id, eventType string, data []byte)
}

type canonicalJournalStreamRepository struct {
	mu       sync.Mutex
	result   *domainrepo.GetJournalBootstrapResult
	err      error
	requests []domainrepo.GetJournalBootstrapRequest
}

func (r *canonicalJournalStreamRepository) GetJournalBootstrap(
	_ context.Context,
	req domainrepo.GetJournalBootstrapRequest,
) (*domainrepo.GetJournalBootstrapResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
	return r.result, r.err
}

func canonicalJournalStreamAttempt(
	run *appagentthread.RunSummary,
	attemptID string,
	status domainentity.RunAttemptStatus,
) *domainentity.RunAttempt {
	return &domainentity.RunAttempt{
		ThreadID: run.ThreadID, JournalRunID: run.RunID, ExecutionRunID: run.RunID,
		AttemptID: attemptID, Ordinal: 1, Status: status, NextSequence: 2,
		SnapshotsEnabled: true, ProjectionState: domainentity.JournalProjectionStateHealthy,
		CreatedAt: run.CreatedAt, StartedAt: pointerToInt64(run.CreatedAt),
	}
}

type recordingCanonicalJournalAdmissionLease struct {
	renewCalls   int
	releaseCalls int
}

type revokedCanonicalJournalThreadAuthorizer struct {
	calls int
}

type revokedCanonicalJournalWorkspaceAuthorizer struct {
	calls int
}

func (a *revokedCanonicalJournalThreadAuthorizer) AuthorizeThreadAccess(
	context.Context,
	appagentthread.ThreadAccessRequest,
) error {
	a.calls++
	return appagentthread.ErrThreadAccessDenied
}

func (a *revokedCanonicalJournalWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	context.Context,
	appagentthread.WorkspaceAccessRequest,
) error {
	a.calls++
	return appagentthread.ErrThreadAccessDenied
}

func (l *recordingCanonicalJournalAdmissionLease) Renew(context.Context) error {
	l.renewCalls++
	return nil
}

func (l *recordingCanonicalJournalAdmissionLease) Release(context.Context) error {
	l.releaseCalls++
	return nil
}

type blockingCanonicalJournalStreamWriter struct {
	once       sync.Once
	started    chan struct{}
	release    chan struct{}
	mu         sync.Mutex
	eventTypes []string
}

func (w *blockingCanonicalJournalStreamWriter) WriteEvent(_ string, eventType string, _ []byte) error {
	w.once.Do(func() { close(w.started) })
	<-w.release
	w.mu.Lock()
	w.eventTypes = append(w.eventTypes, eventType)
	w.mu.Unlock()
	return nil
}

func (w *blockingCanonicalJournalStreamWriter) WriteKeepAlive() error { return nil }

func (w *blockingCanonicalJournalStreamWriter) EventTypes() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.eventTypes...)
}

type canonicalRunStreamWriterRecorder struct {
	writers []*recordingTaskThreadRunEventStreamWriter
}

type invalidCanonicalResumeMessageThreadService struct {
	domainservice.ThreadService
}

func (s invalidCanonicalResumeMessageThreadService) ListMessages(
	ctx context.Context,
	req *domainservice.ListMessagesRequest,
) ([]*domainentity.Message, int64, error) {
	messages, total, err := s.ThreadService.ListMessages(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	invalid := make([]*domainentity.Message, len(messages))
	for i, message := range messages {
		if message == nil {
			continue
		}
		copy := *message
		copy.ID = 0
		invalid[i] = &copy
	}
	return invalid, total, nil
}

func installCanonicalRunStreamRecordingWriters(t *testing.T) *canonicalRunStreamWriterRecorder {
	t.Helper()
	previous := canonicalRunStreamWriterFactory
	recorder := &canonicalRunStreamWriterRecorder{}
	canonicalRunStreamWriterFactory = func(*app.RequestContext) canonicalRunStreamWriterHandle {
		writer := &recordingTaskThreadRunEventStreamWriter{}
		recorder.writers = append(recorder.writers, writer)
		return canonicalRunStreamWriterHandle{writer: writer}
	}
	t.Cleanup(func() { canonicalRunStreamWriterFactory = previous })
	return recorder
}

func (r *canonicalRunStreamWriterRecorder) writer(
	t *testing.T,
	index int,
) *recordingTaskThreadRunEventStreamWriter {
	t.Helper()
	require.NotNil(t, r)
	require.Greater(t, len(r.writers), index)
	return r.writers[index]
}

func (w *callbackCanonicalRunStreamWriter) WriteEvent(id, eventType string, data []byte) error {
	if err := w.recordingTaskThreadRunEventStreamWriter.WriteEvent(id, eventType, data); err != nil {
		return err
	}
	if w.onEvent != nil {
		w.onEvent(id, eventType, data)
	}
	return nil
}

func canonicalRunStreamTestServer(timeout time.Duration) *server.Hertz {
	h := canonicalAgentThreadTestServerForUserAndSpace(2, 1)
	if timeout > 0 {
		h.Use(func(ctx context.Context, c *app.RequestContext) {
			streamCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			c.Next(streamCtx)
		})
	}
	h.POST("/api/workbench/threads/:thread_id/runs/stream", StreamCanonicalRun)
	h.GET("/api/workbench/threads/:thread_id/runs/:run_id/stream", ReconnectCanonicalRunStream)
	return h
}

func createCanonicalRunStreamFixture(
	t *testing.T,
	threadID int64,
	message string,
	onDisconnect string,
) *appagentthread.RunSummary {
	t.Helper()
	response, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID: threadID, AssistantID: canonicalPublicAssistantID,
		Input: `{"uploaded_files":[]}`, MessageContent: message, StreamMode: `["events"]`,
		MultitaskStrategy: "reject", OnDisconnect: onDisconnect, Durability: "async",
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Run)
	return response.Run
}

func appendCanonicalRunStreamEvent(
	t *testing.T,
	run *appagentthread.RunSummary,
	eventType string,
	payload string,
) *appagentthread.RunEventSummary {
	t.Helper()
	require.NotNil(t, run)
	response, err := appagentthread.SVC.AppendRunEvent(context.Background(), &appagentthread.AppendRunEventRequest{
		ThreadID: run.ThreadID, RunID: run.RunID, EventType: eventType, Payload: payload,
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Event)
	return response.Event
}

func canonicalRunStreamEventsForTest(
	t *testing.T,
	run *appagentthread.RunSummary,
) []*appagentthread.RunEventSummary {
	t.Helper()
	response, err := appagentthread.SVC.ListRunEvents(context.Background(), &appagentthread.ListRunEventsRequest{
		ThreadID: run.ThreadID, RunID: run.RunID, Page: 1, PageSize: 200,
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	return response.Events
}

func canonicalRunStreamCurrentRun(t *testing.T, runID int64) *appagentthread.RunSummary {
	t.Helper()
	response, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: runID})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Run)
	return response.Run
}

func canonicalRunStreamEventIDs(t *testing.T, body string) []int64 {
	t.Helper()
	ids := make([]int64, 0)
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "id: ") {
			continue
		}
		id, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "id: ")), 10, 64)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	return ids
}

func canonicalRunStreamPayloads(t *testing.T, body, eventType string) []any {
	t.Helper()
	payloads := make([]any, 0)
	for _, frame := range strings.Split(strings.TrimSpace(body), "\n\n") {
		lines := strings.Split(frame, "\n")
		matched := false
		for _, line := range lines {
			matched = matched || line == "event: "+eventType
		}
		if !matched {
			continue
		}
		for _, line := range lines {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var payload any
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload))
			payloads = append(payloads, payload)
			break
		}
	}
	return payloads
}

func canonicalRunStreamStringIDs(t *testing.T, raw []string) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(raw))
	for _, value := range raw {
		id, err := strconv.ParseInt(value, 10, 64)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	return ids
}

func canonicalRunStreamSummaryIDs(events []*appagentthread.RunEventSummary) []int64 {
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		if event != nil {
			ids = append(ids, event.EventID)
		}
	}
	return ids
}

func canonicalRunStreamIDsAfter(events []*appagentthread.RunEventSummary, afterEventID int64) []int64 {
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		if event != nil && event.EventID > afterEventID {
			ids = append(ids, event.EventID)
		}
	}
	return ids
}
