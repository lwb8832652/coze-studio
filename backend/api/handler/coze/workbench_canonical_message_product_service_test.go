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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	appworkbench "github.com/coze-dev/coze-studio/backend/application/workbench"
	"github.com/coze-dev/coze-studio/backend/internal/testutil"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

func TestAppendCanonicalThreadMessageRejectsOrdinaryUserTurnBeforeMutation(t *testing.T) {
	for _, role := range []string{"user", "human"} {
		role := role
		t.Run(role, func(t *testing.T) {
			installAgentThreadTestService(t)
			thread := createCanonicalTestThread(t, 1001, "message append", `{}`)
			run := createCanonicalRunFixture(t, thread.ThreadID, "initial user turn")
			before := canonicalMessageCount(t, thread.ThreadID)
			h := canonicalMessageTestServer()

			response := performCanonicalMessageJSONRequest(
				t,
				h,
				http.MethodPost,
				fmt.Sprintf("/api/workbench/threads/%d/messages", thread.ThreadID),
				canonicalAppendMessageBody(run.RunID, role, "forged user turn", "internal_compat"),
			)

			require.Equal(t, http.StatusConflict, response.Code, response.Result().Body())
			var public canonicalError
			require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
			require.Equal(t, "atomic_run_submission_required", public.Code)
			require.Equal(t, before, canonicalMessageCount(t, thread.ThreadID))
		})
	}
}

func TestAppendCanonicalThreadMessageRequiresInternalCompatModeAndOwnedRun(t *testing.T) {
	t.Run("requires internal compat mode", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "message append", `{}`)
		run := createCanonicalRunFixture(t, thread.ThreadID, "initial user turn")
		h := canonicalMessageTestServer()

		response := performCanonicalMessageJSONRequest(
			t,
			h,
			http.MethodPost,
			fmt.Sprintf("/api/workbench/threads/%d/messages", thread.ThreadID),
			canonicalAppendMessageBody(run.RunID, "assistant", "compat response", "ordinary"),
		)

		require.Equal(t, http.StatusBadRequest, response.Code, response.Result().Body())
		var public canonicalError
		require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
		require.Equal(t, "invalid_append_mode", public.Code)
	})

	t.Run("requires positive run id", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "message append", `{}`)
		h := canonicalMessageTestServer()

		response := performCanonicalMessageJSONRequest(
			t,
			h,
			http.MethodPost,
			fmt.Sprintf("/api/workbench/threads/%d/messages", thread.ThreadID),
			canonicalAppendMessageBody(0, "assistant", "compat response", "internal_compat"),
		)

		require.Equal(t, http.StatusBadRequest, response.Code, response.Result().Body())
		var public canonicalError
		require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
		require.Equal(t, "invalid_run_id", public.Code)
	})

	t.Run("rejects run from another thread", func(t *testing.T) {
		installAgentThreadTestService(t)
		thread := createCanonicalTestThread(t, 1001, "message append", `{}`)
		otherThread := createCanonicalTestThread(t, 1001, "other message append", `{}`)
		otherRun := createCanonicalRunFixture(t, otherThread.ThreadID, "other user turn")
		before := canonicalMessageCount(t, thread.ThreadID)
		h := canonicalMessageTestServer()

		response := performCanonicalMessageJSONRequest(
			t,
			h,
			http.MethodPost,
			fmt.Sprintf("/api/workbench/threads/%d/messages", thread.ThreadID),
			canonicalAppendMessageBody(otherRun.RunID, "assistant", "cross-thread response", "internal_compat"),
		)

		require.Equal(t, http.StatusNotFound, response.Code, response.Result().Body())
		var public canonicalError
		require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
		require.Equal(t, "resource_not_found", public.Code)
		require.Equal(t, before, canonicalMessageCount(t, thread.ThreadID))
	})
}

func TestCanonicalThreadMessageRequestsRejectBodyRouteIdentifiers(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "message identifiers", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "initial user turn")
	h := canonicalMessageTestServer()

	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "append thread id",
			path: fmt.Sprintf("/api/workbench/threads/%d/messages", thread.ThreadID),
			body: fmt.Sprintf(
				`{"thread_id":"999","run_id":"%d","role":"assistant","content":"reply","append_mode":"internal_compat"}`,
				run.RunID,
			),
		},
		{
			name: "suggestions space id",
			path: fmt.Sprintf("/api/workbench/threads/%d/suggestions", thread.ThreadID),
			body: `{"space_id":"999","n":1}`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := performCanonicalMessageJSONRequest(t, h, http.MethodPost, test.path, test.body)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Result().Body())
			var public canonicalError
			require.NoError(t, json.Unmarshal(response.Result().Body(), &public))
			require.Equal(t, "unsupported_sdk_field", public.Code)
		})
	}
}

func TestAppendCanonicalThreadMessageReturnsPublicMessage(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "message append", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "initial user turn")
	h := canonicalMessageTestServer()

	response := performCanonicalMessageJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/messages", thread.ThreadID),
		canonicalAppendMessageBody(run.RunID, "assistant", "assistant public response", "internal_compat"),
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var message canonicalMessage
	require.NoError(t, json.Unmarshal(response.Result().Body(), &message))
	require.NotEmpty(t, message.MessageID)
	require.Equal(t, fmt.Sprint(thread.ThreadID), message.ThreadID)
	require.Equal(t, fmt.Sprint(run.RunID), message.RunID)
	require.Equal(t, "assistant", message.Role)
	require.Equal(t, "assistant public response", message.Content)
	require.Equal(t, "compat", message.Metadata["source"])

	messages := canonicalPersistedMessages(t, thread.ThreadID)
	require.Equal(t, "assistant public response", messages[len(messages)-1].Content)
	require.Equal(t, `{"source":"compat"}`, messages[len(messages)-1].Metadata)
}

func TestAppendCanonicalThreadMessageAllowsToolWithoutLeakingToolContent(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "message append", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "initial user turn")
	h := canonicalMessageTestServer()

	response := performCanonicalMessageJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/messages", thread.ThreadID),
		canonicalAppendMessageBody(run.RunID, "tool", "tool-private-secret", "internal_compat"),
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	body := string(response.Result().Body())
	require.NotContains(t, body, "tool-private-secret")
	var message canonicalMessage
	require.NoError(t, json.Unmarshal(response.Result().Body(), &message))
	require.NotEmpty(t, message.MessageID)
	require.Equal(t, "tool", message.Role)
	require.Empty(t, message.Content)

	messages := canonicalPersistedMessages(t, thread.ThreadID)
	require.Equal(t, "tool-private-secret", messages[len(messages)-1].Content)
}

func TestGenerateCanonicalThreadSuggestionsUsesPersistedPublicMessages(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "suggestions", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "persisted user question")
	_, err := appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: thread.ThreadID,
		RunID:    run.RunID,
		Role:     appagentthread.MessageRoleTool,
		Content:  "tool-result-secret",
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
		ThreadID: thread.ThreadID,
		RunID:    run.RunID,
		Role:     appagentthread.MessageRoleAssistant,
		Content:  "persisted assistant answer",
	})
	require.NoError(t, err)

	var gotModelType int64
	var gotPrompt string
	installCanonicalSuggestionProviderForTest(t, func(_ context.Context, modelType int64) (model.BaseChatModel, bool, error) {
		gotModelType = modelType
		return &testutil.UTChatModel{
			InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
				require.Len(t, in, 2)
				gotPrompt = in[1].Content
				return schema.AssistantMessage(`["follow up one","follow up two"]`, nil), nil
			},
		}, true, nil
	})
	h := canonicalMessageTestServer()

	response := performCanonicalMessageJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/suggestions", thread.ThreadID),
		`{"n":2,"model_type":"42","model_name":"suggestion-test"}`,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var suggestions map[string][]string
	require.NoError(t, json.Unmarshal(response.Result().Body(), &suggestions))
	require.Equal(t, []string{"follow up one", "follow up two"}, suggestions["suggestions"])
	require.Equal(t, int64(42), gotModelType)
	require.Contains(t, gotPrompt, "User: persisted user question")
	require.Contains(t, gotPrompt, "Assistant: persisted assistant answer")
	require.NotContains(t, gotPrompt, "tool-result-secret")
}

func TestGenerateCanonicalThreadSuggestionsKeepsNewestFortyPublicMessages(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "suggestions", `{}`)
	run := createCanonicalRunFixture(t, thread.ThreadID, "public-00")
	for index := 1; index < 45; index++ {
		_, err := appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
			ThreadID: thread.ThreadID,
			RunID:    run.RunID,
			Role:     appagentthread.MessageRoleAssistant,
			Content:  fmt.Sprintf("public-%02d", index),
		})
		require.NoError(t, err)
	}
	for index := 0; index < 45; index++ {
		_, err := appagentthread.SVC.AppendMessage(context.Background(), &appagentthread.AppendMessageRequest{
			ThreadID: thread.ThreadID,
			RunID:    run.RunID,
			Role:     appagentthread.MessageRoleSystem,
			Content:  fmt.Sprintf("system-noise-%02d", index),
		})
		require.NoError(t, err)
	}

	var gotPrompt string
	installCanonicalSuggestionProviderForTest(t, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return &testutil.UTChatModel{
			InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
				require.Len(t, in, 2)
				gotPrompt = in[1].Content
				return schema.AssistantMessage(`["next"]`, nil), nil
			},
		}, true, nil
	})
	h := canonicalMessageTestServer()

	response := performCanonicalMessageJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/suggestions", thread.ThreadID),
		`{"n":1}`,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	require.Contains(t, gotPrompt, "Assistant: public-05")
	require.Contains(t, gotPrompt, "Assistant: public-44")
	require.NotContains(t, gotPrompt, "User: public-00")
	require.NotContains(t, gotPrompt, "Assistant: public-04")
	require.NotContains(t, gotPrompt, "system-noise-44")
}

func TestGenerateCanonicalThreadSuggestionsReturnsEmptyListWhenProviderFails(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "suggestions", `{}`)
	createCanonicalRunFixture(t, thread.ThreadID, "persisted user question")
	installCanonicalSuggestionProviderForTest(t, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return nil, false, errors.New("provider raw secret")
	})
	h := canonicalMessageTestServer()

	response := performCanonicalMessageJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/suggestions", thread.ThreadID),
		`{"n":3}`,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	var suggestions map[string][]string
	require.NoError(t, json.Unmarshal(response.Result().Body(), &suggestions))
	require.Empty(t, suggestions["suggestions"])
}

func TestGenerateCanonicalThreadSuggestionsNeverLogsMessageContent(t *testing.T) {
	installAgentThreadTestService(t)
	thread := createCanonicalTestThread(t, 1001, "suggestions", `{}`)
	createCanonicalRunFixture(t, thread.ThreadID, "message-content-secret")
	installCanonicalSuggestionProviderForTest(t, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return nil, false, errors.New("provider-error-secret")
	})
	h := canonicalMessageTestServer()

	var output bytes.Buffer
	logs.SetOutput(&output)
	t.Cleanup(func() { logs.SetOutput(os.Stderr) })

	response := performCanonicalMessageJSONRequest(
		t,
		h,
		http.MethodPost,
		fmt.Sprintf("/api/workbench/threads/%d/suggestions", thread.ThreadID),
		`{"n":3}`,
	)

	require.Equal(t, http.StatusOK, response.Code, response.Result().Body())
	actual := output.String()
	require.Contains(t, actual, "event_name=workbench.api.request.completed")
	require.Contains(t, actual, "operation=thread.suggestions.generate")
	require.Contains(t, actual, "suggestion_failure=provider_error")
	require.NotContains(t, actual, "message-content-secret")
	require.NotContains(t, actual, "provider-error-secret")
}

func canonicalMessageTestServer() *server.Hertz {
	h := authenticatedAgentThreadTestServer()
	registerCanonicalMessageRoutes(h)
	return h
}

func registerCanonicalMessageRoutes(h *server.Hertz) {
	h.POST("/api/workbench/threads/:thread_id/messages", AppendCanonicalThreadMessage)
	h.POST("/api/workbench/threads/:thread_id/suggestions", GenerateCanonicalThreadSuggestions)
}

func performCanonicalMessageJSONRequest(
	t *testing.T,
	h *server.Hertz,
	method string,
	path string,
	body string,
	headers ...ut.Header,
) *ut.ResponseRecorder {
	t.Helper()
	requestHeaders := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
	hasSpaceHeader := false
	for _, header := range headers {
		if strings.EqualFold(header.Key, canonicalSpaceIDHeader) {
			hasSpaceHeader = true
			break
		}
	}
	if !hasSpaceHeader {
		requestHeaders = append(requestHeaders, ut.Header{Key: canonicalSpaceIDHeader, Value: "1001"})
	}
	requestHeaders = append(requestHeaders, headers...)
	return ut.PerformRequest(
		h.Engine,
		method,
		path,
		&ut.Body{Body: strings.NewReader(body), Len: len(body)},
		requestHeaders...,
	)
}

func canonicalAppendMessageBody(runID int64, role, content, appendMode string) string {
	return fmt.Sprintf(
		`{"run_id":"%d","role":%q,"content":%q,"metadata":"{\"source\":\"compat\"}","append_mode":%q}`,
		runID,
		role,
		content,
		appendMode,
	)
}

func canonicalMessageCount(t *testing.T, threadID int64) int64 {
	t.Helper()
	return int64(len(canonicalPersistedMessages(t, threadID)))
}

func canonicalPersistedMessages(t *testing.T, threadID int64) []*appagentthread.MessageSummary {
	t.Helper()
	resp, err := appagentthread.SVC.ListMessages(context.Background(), &appagentthread.ListMessagesRequest{
		ThreadID: threadID,
		Page:     1,
		PageSize: 100,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp.Messages
}

func installCanonicalSuggestionProviderForTest(
	t *testing.T,
	provider func(context.Context, int64) (model.BaseChatModel, bool, error),
) {
	t.Helper()
	previous := appworkbench.SVC
	appworkbench.SVC = new(appworkbench.ApplicationService)
	appworkbench.InitService(&appworkbench.ServiceComponents{ChatModelProvider: provider})
	t.Cleanup(func() {
		appworkbench.SVC = previous
	})
}
