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
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRunTypedV2IdempotencyRejectsChangedCompatibilityComponents(t *testing.T) {
	tests := []struct {
		name    string
		changed func(string) string
	}{
		{
			name: "typed semantic field",
			changed: func(submission string) string {
				return canonicalTypedRunRequestV2(
					canonicalTypedV2Replace(submission, `"message":"typed idempotency"`, `"message":"changed semantic"`),
					"",
				)
			},
		},
		{
			name: "safe route option",
			changed: func(submission string) string {
				return canonicalTypedRunRequestV2(submission, `,"on_disconnect":"continue"`)
			},
		},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "typed idempotency conflict", `{}`)
			path := fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID)
			submission := canonicalTypedRunTurnV2("typed idempotency")
			header := ut.Header{Key: "Idempotency-Key", Value: fmt.Sprintf("typed-conflict-%d", index)}

			first := performCanonicalRunJSONRequest(
				t, canonicalRunTestServer(), http.MethodPost, path,
				canonicalTypedRunRequestV2(submission, ""), header,
			)
			require.Equal(t, http.StatusOK, first.Code, first.Result().Body())

			conflict := performCanonicalRunJSONRequest(
				t, canonicalRunTestServer(), http.MethodPost, path, tt.changed(submission), header,
			)
			require.Equal(t, http.StatusConflict, conflict.Code, conflict.Result().Body())
			var public canonicalError
			require.NoError(t, json.Unmarshal(conflict.Result().Body(), &public))
			require.Equal(t, "idempotency_conflict", public.Code)
			require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 1)
		})
	}
}

func TestCanonicalRunTypedV2RetryConflictsWithHistoricalV1Fingerprint(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "typed historical retry collision", `{}`)
	source := createCanonicalRunFixture(t, thread.ThreadID, "historical retry source")
	failCanonicalRunFixture(t, source, "runtime_failed", "failed")
	typedSubmission := canonicalTypedRunRetryV2(source.RunID, "retry same task")
	typed, public := decodeCanonicalTypedRunSubmissionV2([]byte(typedSubmission))
	require.Nil(t, public)
	mapped, public := mapCanonicalTypedRunV2(typed)
	require.Nil(t, public)

	path := fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID)
	header := ut.Header{
		Key:   "Idempotency-Key",
		Value: fmt.Sprintf("%d:%d:%d:task_retry", thread.SpaceID, thread.ThreadID, source.RunID),
	}
	legacyBody := fmt.Sprintf(`{
		"assistant_id":"agent",
		"input":{"messages":[{"role":"user","content":"retry same task"}]},
		"metadata":{"source":"task_retry","legacy_trace":"historical"},
		"config":%s,
		"context":{},
		"coze":{"attempt_kind":"retry","source_run_id":"%d"}
	}`, mapped.Config, source.RunID)
	legacy := performCanonicalRunJSONRequest(
		t, canonicalRunTestServer(), http.MethodPost, path, legacyBody, header,
	)
	require.Equal(t, http.StatusOK, legacy.Code, legacy.Result().Body())
	require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 2)

	conflict := performCanonicalRunJSONRequest(
		t, canonicalRunTestServer(), http.MethodPost, path,
		canonicalTypedRunRequestV2(typedSubmission, ""), header,
	)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Result().Body())
	require.Contains(t, string(conflict.Result().Body()), `"code":"idempotency_conflict"`)
	require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 2)
}

func TestCanonicalRunTypedV2RejectsUnicodeLookalikeWithoutMutation(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "typed unicode legacy alias", `{}`)
	body := fmt.Sprintf(
		`{"assistant_id":"agent","submission_v2":%s,"inpuſ":null}`,
		canonicalTypedRunTurnV2("must not run"),
	)

	response := performCanonicalRunJSONRequest(
		t, canonicalRunTestServer(), http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/runs", thread.ThreadID), body,
	)

	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
	var public canonicalError
	require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
	// The legacy key is not Unicode-fold-equivalent to "input" (long-s folds to
	// ASCII s, not t), so it remains an unsupported field rather than a mix.
	require.Equal(t, "unsupported_sdk_field", public.Code)
	require.Len(t, canonicalRunsForThread(t, thread.ThreadID), 0)
}
