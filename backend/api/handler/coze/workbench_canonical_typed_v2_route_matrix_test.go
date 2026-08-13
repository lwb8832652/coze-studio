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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

func TestCanonicalTypedV2RunRouteMatrix(t *testing.T) {
	t.Run("create retry", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "typed create retry matrix", `{}`)
		source := createCanonicalRunFixture(t, thread.ThreadID, "create retry source")
		failCanonicalRunFixture(t, source, "runtime_failed", "failed")
		prompt := "create typed retry"

		response := performCanonicalRunJSONRequest(
			t,
			canonicalRunTestServer(),
			http.MethodPost,
			fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
			canonicalTypedRunRequestV2(canonicalTypedRunRetryV2(source.RunID, prompt), ""),
			ut.Header{Key: "Idempotency-Key", Value: "typed-v2-matrix-create-retry"},
		)

		require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
		assertCanonicalTypedV2RetryPersistence(t, thread.ThreadID, source.RunID, prompt)
	})

	t.Run("wait turn", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "typed wait turn matrix", `{}`)
		prompt := "wait typed turn"
		body := canonicalTypedRunRequestV2(canonicalTypedRunTurnV2(prompt), "")
		header := ut.Header{Key: "Idempotency-Key", Value: "typed-v2-matrix-wait-turn"}
		h := canonicalRunTestServer()

		created := performCanonicalRunJSONRequest(
			t,
			h,
			http.MethodPost,
			fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID),
			body,
			header,
		)
		require.Equal(t, http.StatusOK, created.Code, created.Result().Body())
		runs := canonicalRunsForThread(t, thread.ThreadID)
		require.Len(t, runs, 1)
		completeCanonicalRunWithPublicState(t, runs[0], `{"custom":{"route":"wait-turn"}}`)

		waited := performCanonicalRunJSONRequest(
			t,
			h,
			http.MethodPost,
			fmt.Sprintf("/api/workbench/threads/%d/runs/wait", thread.ThreadID),
			body,
			header,
		)
		require.Equal(t, http.StatusOK, waited.Code, waited.Result().Body())
		var values map[string]any
		require.NoError(t, json.Unmarshal(waited.Result().Body(), &values))
		require.Equal(t, "wait-turn", values["custom"].(map[string]any)["route"])
		messages, persistedRuns := canonicalThreadMessagesAndRuns(t, thread.ThreadID)
		require.Len(t, persistedRuns, 1)
		require.Len(t, messages, 1)
		require.Equal(t, prompt, messages[0].Content)
	})

	t.Run("stream turn", func(t *testing.T) {
		installAgentThreadTestService(t)
		writers := installCanonicalRunStreamRecordingWriters(t)
		prompt := "stream typed turn"
		body := canonicalTypedRunRequestV2(
			canonicalTypedRunTurnV2(prompt),
			`,"stream_mode":["events"],"on_disconnect":"continue"`,
		)

		response := performCanonicalRunJSONRequest(
			t,
			canonicalRunStreamTestServer(20*time.Millisecond),
			http.MethodPost,
			"/api/workbench/threads/1/runs/stream",
			body,
		)

		require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
		messages, runs := canonicalThreadMessagesAndRuns(t, 1)
		require.Len(t, runs, 1)
		require.Len(t, messages, 1)
		require.Equal(t, prompt, messages[0].Content)
		require.Empty(t, runs[0].IdempotencyKey)
		var metadata map[string]any
		require.NoError(t, json.Unmarshal([]byte(runs[0].Metadata), &metadata))
		_, hasIdempotencyContract := metadata["_idempotency"]
		require.False(t, hasIdempotencyContract)
		assertCanonicalTypedV2StreamStarted(t, writers.writer(t, 0).String(), runs[0].RunID)
	})

	t.Run("stream retry", func(t *testing.T) {
		installAgentThreadTestService(t)
		source := createCanonicalRunFixture(t, 1, "stream retry matrix source")
		failCanonicalRunFixture(t, source, "runtime_failed", "failed")
		writers := installCanonicalRunStreamRecordingWriters(t)
		prompt := "stream typed retry"
		body := canonicalTypedRunRequestV2(
			canonicalTypedRunRetryV2(source.RunID, prompt),
			`,"stream_mode":["events"],"on_disconnect":"continue"`,
		)

		response := performCanonicalRunJSONRequest(
			t,
			canonicalRunStreamTestServer(20*time.Millisecond),
			http.MethodPost,
			"/api/workbench/threads/1/runs/stream",
			body,
			ut.Header{Key: "Idempotency-Key", Value: "typed-v2-matrix-stream-retry"},
		)

		require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
		retryRun := assertCanonicalTypedV2RetryPersistence(t, 1, source.RunID, prompt)
		assertCanonicalTypedV2StreamStarted(t, writers.writer(t, 0).String(), retryRun.RunID)
	})
}

func assertCanonicalTypedV2RetryPersistence(
	t *testing.T,
	threadID int64,
	sourceRunID int64,
	prompt string,
) *appagentthread.RunSummary {
	t.Helper()
	messages, runs := canonicalThreadMessagesAndRuns(t, threadID)
	require.Len(t, messages, 1)
	require.Len(t, runs, 2)
	var retryRun *appagentthread.RunSummary
	for _, run := range runs {
		if run != nil && run.RunID != sourceRunID {
			retryRun = run
			break
		}
	}
	require.NotNil(t, retryRun)
	require.Contains(t, retryRun.Input, prompt)
	require.NotContains(t, retryRun.Metadata, `"_message"`)
	return retryRun
}

func assertCanonicalTypedV2StreamStarted(t *testing.T, stream string, runID int64) {
	t.Helper()
	require.Contains(t, stream, "event: metadata")
	require.Contains(t, stream, `"run_id":"`+strconv.FormatInt(runID, 10)+`"`)
}
