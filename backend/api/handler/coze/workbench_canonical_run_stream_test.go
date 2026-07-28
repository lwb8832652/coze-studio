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
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const canonicalRunStreamRequestBody = `{
	"assistant_id":"agent",
	"input":{"messages":[{"role":"user","content":"stream this turn"}]},
	"stream_mode":["events"],
	"on_disconnect":"continue"
}`

func TestStreamCanonicalRunCreatesOneRunAndStreamsPersistedEvents(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
	installAgentThreadTestService(t)
	previousWriterFactory := canonicalRunStreamWriterFactory
	writer := &callbackCanonicalRunStreamWriter{}
	canonicalRunStreamWriterFactory = func(*app.RequestContext) canonicalRunStreamWriterHandle {
		return canonicalRunStreamWriterHandle{writer: writer}
	}
	t.Cleanup(func() { canonicalRunStreamWriterFactory = previousWriterFactory })
	appended := false
	writer.onEvent = func(_ string, eventType string, data []byte) {
		if appended || eventType != langGraphRunStreamMetadata {
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

func TestStreamCanonicalRunReplaysIdempotentRunWithoutSecondMessage(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
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
	t.Setenv(canonicalAPIEnabledEnv, "true")
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
	)

	require.Equal(t, http.StatusNotFound, response.Code, response.Result().Body())
	require.Empty(t, writers.writers)
}

func TestStreamCanonicalRunCommandResumeUsesExistingApplicationFlow(t *testing.T) {
	t.Setenv(canonicalAPIEnabledEnv, "true")
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
	t.Setenv(canonicalAPIEnabledEnv, "true")
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

	t.Setenv(canonicalAPIEnabledEnv, "true")
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
	t.Setenv(canonicalAPIEnabledEnv, "true")
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
	t.Setenv(canonicalAPIEnabledEnv, "true")
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
	t.Setenv(canonicalAPIEnabledEnv, "true")
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
	payloads := canonicalRunStreamPayloads(t, body, langGraphRunStreamMessages)
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

type callbackCanonicalRunStreamWriter struct {
	recordingTaskThreadRunEventStreamWriter
	onEvent func(id, eventType string, data []byte)
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
	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(2))
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
