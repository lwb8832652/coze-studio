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

package deerflowparity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeerFlowClientUsesLockedGatewayProtocol(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/auth/login/local":
			require.Equal(t, "application/x-www-form-urlencoded", request.Header.Get("Content-Type"))
			require.NoError(t, request.ParseForm())
			require.Equal(t, "user@example.com", request.Form.Get("username"))
			require.Equal(t, "password", request.Form.Get("password"))
			http.SetCookie(writer, &http.Cookie{Name: "access_token", Value: "private", Path: "/"})
			http.SetCookie(writer, &http.Cookie{Name: "csrf_token", Value: "csrf-safe", Path: "/"})
			writeTestJSON(writer, map[string]any{"expires_in": 3600})
		case "/api/threads":
			require.Equal(t, "csrf-safe", request.Header.Get("X-CSRF-Token"))
			writeTestJSON(writer, map[string]any{"thread_id": "df-thread"})
		case "/api/threads/df-thread/runs":
			require.Equal(t, http.MethodPost, request.Method)
			require.Equal(t, "csrf-safe", request.Header.Get("X-CSRF-Token"))
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, "lead_agent", body["assistant_id"])
			require.Equal(t, []any{"values"}, body["stream_mode"])
			require.Equal(t, "cancel", body["on_disconnect"])
			writeTestJSON(writer, map[string]any{
				"run_id": "df-run", "thread_id": "df-thread", "status": "pending",
			})
		case "/api/threads/df-thread/runs/df-run/stream":
			require.Equal(t, http.MethodGet, request.Method)
			require.Empty(t, request.URL.Query().Get("stream_mode"))
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "id: 1\nevent: values\ndata: {\"messages\":[]}\n\n")
			_, _ = fmt.Fprint(writer, "id: 2\nevent: end\ndata: null\n\n")
		case "/api/threads/df-thread/runs/df-run":
			require.Equal(t, http.MethodGet, request.Method)
			writeTestJSON(writer, map[string]any{
				"run_id": "df-run", "thread_id": "df-thread", "status": "success",
			})
		case "/api/threads/df-thread/state":
			writeTestJSON(writer, map[string]any{"values": map[string]any{"todos": []any{}}})
		case "/api/threads/df-thread/history":
			require.Equal(t, http.MethodPost, request.Method)
			require.Equal(t, "csrf-safe", request.Header.Get("X-CSRF-Token"))
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, float64(100), body["limit"])
			writeTestJSON(writer, []any{map[string]any{
				"checkpoint_id": "checkpoint-1", "parent_checkpoint_id": nil,
			}})
		case "/api/threads/df-thread/runs/df-run/messages":
			writeTestJSON(writer, map[string]any{
				"data": []any{map[string]any{
					"seq": 1, "event_type": "llm.ai.response", "category": "message",
					"content": map[string]any{"type": "ai", "content": "safe answer"},
					"metadata": map[string]any{"caller": "lead_agent", "usage": map[string]any{
						"input_tokens": 5, "output_tokens": 2, "total_tokens": 7,
					}},
				}},
				"has_more": false,
			})
		case "/api/threads/df-thread/runs/df-run/events":
			writeTestJSON(writer, []any{
				map[string]any{"seq": 1, "event_type": "run.start", "category": "trace", "metadata": map[string]any{"caller": "lead_agent"}},
				map[string]any{"seq": 2, "event_type": "llm.ai.response", "category": "message", "content": map[string]any{"type": "ai", "content": "safe answer"}, "metadata": map[string]any{"caller": "lead_agent"}},
				map[string]any{"seq": 3, "event_type": "run.end", "category": "outputs", "metadata": map[string]any{"status": "success"}},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewDeerFlowClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, client.Login(context.Background(), Credentials{Email: "user@example.com", Password: "password"}))
	threadID, err := client.CreateThread(context.Background(), ThreadOptions{})
	require.NoError(t, err)
	require.Equal(t, "df-thread", threadID)

	stream, err := client.StreamRun(context.Background(), threadID, testRunInput())
	require.NoError(t, err)
	require.Equal(t, "df-run", stream.RunID)
	require.Equal(t, "success", stream.Terminal)
	require.Len(t, stream.Frames, 2)

	state, err := client.GetThreadState(context.Background(), threadID)
	require.NoError(t, err)
	require.Contains(t, state, "values")
	history, err := client.GetThreadHistory(context.Background(), threadID, 100)
	require.NoError(t, err)
	require.Len(t, history, 1)
	messages, err := client.ListRunMessages(context.Background(), threadID, stream.RunID, PageRequest{Limit: 200})
	require.NoError(t, err)
	require.Len(t, messages.Data, 1)
	events, err := client.ListRunEvents(context.Background(), threadID, stream.RunID, 100)
	require.NoError(t, err)
	require.Len(t, events, 3)
}

const (
	newXTestSpaceID       = "7656103552997130240"
	newXTestOtherSpaceID  = "7656103552997130241"
	newXTestThreadID      = "7657000000000000000"
	newXTestRunID         = "7657000000000000001"
	newXTestFollowUpRunID = "7657000000000000002"
)

type newXStatefulRunBody struct {
	calls atomic.Int32
}

func (body *newXStatefulRunBody) MarshalJSON() ([]byte, error) {
	if body.calls.Add(1) == 1 {
		return []byte(`{"safe":"inspected"}`), nil
	}
	return []byte(`{"userId":"injected-on-second-marshal"}`), nil
}

func newXCanonicalTestServer(t *testing.T, next http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/threads") || strings.Contains(request.URL.Path, "task_threads") {
			t.Errorf("NewX used a non-canonical route: %s", request.URL.Path)
			http.NotFound(writer, request)
			return
		}
		if strings.HasPrefix(request.URL.Path, "/api/workbench/") {
			require.True(t, strings.HasPrefix(request.URL.Path, "/api/workbench/threads"))
			require.Equal(t, newXTestSpaceID, request.Header.Get("X-Coze-Space-ID"))
		}
		if request.URL.Path != "/api/workbench/threads" {
			next(writer, request)
			return
		}

		require.Equal(t, http.MethodPost, request.Method)
		var body map[string]any
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		metadata, ok := body["metadata"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "api", metadata["source"])
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "space_id")
		require.NotContains(t, string(encoded), "user_id")
		require.NotContains(t, string(encoded), "owner_id")
		writeTestJSON(writer, map[string]any{
			"thread_id":  newXTestThreadID,
			"created_at": "2026-07-29T00:00:00Z",
			"updated_at": "2026-07-29T00:00:00Z",
			"metadata": map[string]any{
				"space_id": "7656103552997130999",
				"user_id":  "7656103552997130998",
				"owner_id": "7656103552997130997",
			},
			"status": "idle", "values": map[string]any{}, "interrupts": map[string]any{}, "coze": map[string]any{},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func newCreatedNewXTestClient(t *testing.T, serverURL string) *NewXClient {
	t.Helper()
	client, err := NewNewXClient(serverURL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	threadID, err := client.CreateThread(context.Background(), ThreadOptions{SpaceID: newXTestSpaceID})
	require.NoError(t, err)
	require.Equal(t, newXTestThreadID, threadID)
	return client
}

func newBoundNewXTestClient(t *testing.T, serverURL string) *NewXClient {
	t.Helper()
	client, err := NewNewXClient(serverURL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, client.bindThreadWorkspace(newXTestThreadID, newXTestSpaceID))
	return client
}

func newXCanonicalRun(runID, status string) map[string]any {
	return map[string]any{
		"run_id": runID, "thread_id": newXTestThreadID, "assistant_id": "agent", "status": status,
		"created_at": "2026-07-29T00:00:00Z", "updated_at": "2026-07-29T00:00:00Z",
		"metadata": map[string]any{}, "multitask_strategy": "reject", "coze": map[string]any{},
	}
}

func newXCanonicalThreadState(threadID string) map[string]any {
	return map[string]any{
		"values": map[string]any{"todos": []any{}},
		"next":   []any{},
		"checkpoint": map[string]any{
			"thread_id": threadID, "checkpoint_ns": "", "checkpoint_id": "7657000000000000100", "checkpoint_map": map[string]any{},
		},
		"metadata":   map[string]any{},
		"created_at": "2026-07-29T00:00:00Z",
		"tasks":      []any{},
		"interrupts": []any{},
	}
}

func newXCanonicalMessage(seq string) map[string]any {
	return map[string]any{
		"message_id": "7657000000000000200",
		"thread_id":  newXTestThreadID,
		"run_id":     newXTestRunID,
		"role":       "assistant",
		"content":    "safe answer",
		"metadata":   map[string]any{},
		"created_at": "2026-07-29T00:00:00Z",
		"seq":        seq,
	}
}

func newXTestSSEMetadata(threadID, runID string) string {
	return fmt.Sprintf("event: metadata\ndata: {\"run_id\":%q,\"thread_id\":%q,\"status\":\"running\"}\n\n", runID, threadID)
}

func newXTestSSEEvent(sseID, eventID, threadID, runID, eventType string) string {
	return fmt.Sprintf(
		"id: %s\nevent: events\ndata: {\"event_id\":%q,\"thread_id\":%q,\"run_id\":%q,\"event_type\":%q,\"payload\":{}}\n\n",
		sseID, eventID, threadID, runID, eventType,
	)
}

func newXTestSSEEnd(threadID, runID, status string) string {
	return fmt.Sprintf(
		"event: end\ndata: {\"run_id\":%q,\"thread_id\":%q,\"status\":%q,\"reason\":\"terminal_run\"}\n\n",
		runID, threadID, status,
	)
}

func TestNewXClientThreadWorkspaceBindingIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/workbench/threads", request.URL.Path)
		require.Equal(t, newXTestSpaceID, request.Header.Get("X-Coze-Space-ID"))
		writeTestJSON(writer, map[string]any{"thread_id": newXTestThreadID})
	}))
	t.Cleanup(server.Close)
	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)

	first, err := client.CreateThread(context.Background(), ThreadOptions{SpaceID: newXTestSpaceID})
	require.NoError(t, err)
	second, err := client.CreateThread(context.Background(), ThreadOptions{SpaceID: newXTestSpaceID})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, newXTestThreadID, second)
}

func TestNewXClientThreadWorkspaceBindingConflictRetainsOriginal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/workbench/threads":
			writeTestJSON(writer, map[string]any{"thread_id": newXTestThreadID})
		case "/api/workbench/threads/" + newXTestThreadID + "/state":
			require.Equal(t, newXTestSpaceID, request.Header.Get("X-Coze-Space-ID"))
			writeTestJSON(writer, newXCanonicalThreadState(newXTestThreadID))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)

	threadID, err := client.CreateThread(context.Background(), ThreadOptions{SpaceID: newXTestSpaceID})
	require.NoError(t, err)
	require.Equal(t, newXTestThreadID, threadID)
	conflictedID, err := client.CreateThread(context.Background(), ThreadOptions{SpaceID: newXTestOtherSpaceID})
	require.Empty(t, conflictedID)
	require.ErrorContains(t, err, "workspace binding conflict")
	_, err = client.GetThreadState(context.Background(), newXTestThreadID)
	require.NoError(t, err)
}

func TestNewXClientUsesCanonicalWorkbenchResources(t *testing.T) {
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/passport/web/email/login/":
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, "user@example.com", body["email"])
			http.SetCookie(writer, &http.Cookie{Name: "session_key", Value: "private", Path: "/"})
			writeTestJSON(writer, map[string]any{"code": 0})
		case "/api/workbench/threads/" + newXTestThreadID + "/runs":
			require.Equal(t, http.MethodPost, request.Method)
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, "lead_agent", body["assistant_id"])
			require.Equal(t, []any{"events"}, body["stream_mode"])
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "user_id")
			require.NotContains(t, string(encoded), "owner_id")
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "pending"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID:
			require.Equal(t, http.MethodGet, request.Method)
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "success"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/cancel":
			require.Equal(t, http.MethodPost, request.Method)
			writer.WriteHeader(http.StatusNoContent)
		case "/api/workbench/threads/" + newXTestThreadID + "/state":
			require.Equal(t, http.MethodGet, request.Method)
			writeTestJSON(writer, newXCanonicalThreadState(newXTestThreadID))
		case "/api/workbench/threads/" + newXTestThreadID + "/history":
			require.Equal(t, http.MethodGet, request.Method)
			require.Equal(t, "100", request.URL.Query().Get("limit"))
			writeTestJSON(writer, []any{newXCanonicalThreadState(newXTestThreadID)})
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/messages":
			require.Equal(t, "2", request.URL.Query().Get("limit"))
			require.Equal(t, "10", request.URL.Query().Get("after_seq"))
			writeTestJSON(writer, map[string]any{
				"data":     []any{newXCanonicalMessage("11")},
				"has_more": true, "next_after_seq": "11",
			})
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/events":
			require.Contains(t, []string{"100", "1000"}, request.URL.Query().Get("limit"))
			writer.Header().Set("X-Pagination-Total", "1")
			writeTestJSON(writer, map[string]any{
				"data": []any{map[string]any{
					"event_id": "7657000000000000300", "thread_id": newXTestThreadID,
					"run_id": newXTestRunID, "event_type": "assistant.completed",
					"payload": map[string]any{}, "created_at": "2026-07-29T00:00:00Z",
				}},
				"has_more": false,
			})
		default:
			http.NotFound(writer, request)
		}
	})

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, client.Login(context.Background(), Credentials{Email: "user@example.com", Password: "password"}))
	threadID, err := client.CreateThread(context.Background(), ThreadOptions{SpaceID: newXTestSpaceID})
	require.NoError(t, err)
	require.Equal(t, newXTestThreadID, threadID)

	run, err := client.StartRun(context.Background(), threadID, testRunInput())
	require.NoError(t, err)
	require.Equal(t, RunHandle{ThreadID: newXTestThreadID, RunID: newXTestRunID, Status: "pending"}, run)
	run, err = client.GetRun(context.Background(), threadID, newXTestRunID)
	require.NoError(t, err)
	require.Equal(t, "success", run.Status)
	require.NoError(t, client.CancelRun(context.Background(), threadID, newXTestRunID))

	state, err := client.GetThreadState(context.Background(), threadID)
	require.NoError(t, err)
	require.Contains(t, state, "values")
	history, err := client.GetThreadHistory(context.Background(), threadID, 100)
	require.NoError(t, err)
	require.Len(t, history, 1)
	messages, err := client.ListRunMessages(
		context.Background(), threadID, newXTestRunID, PageRequest{Limit: 2, AfterSeq: 10},
	)
	require.NoError(t, err)
	require.Len(t, messages.Data, 1)
	require.True(t, messages.HasMore)
	events, err := client.ListRunEvents(context.Background(), threadID, newXTestRunID, 100)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "assistant.completed", events[0]["event_type"])
	require.NoError(t, client.WaitForEvent(context.Background(), threadID, newXTestRunID, "assistant.completed"))
}

func TestNewXClientRequiresSessionCookie(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeTestJSON(writer, map[string]any{"code": 0})
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	err = client.Login(context.Background(), Credentials{Email: "user@example.com", Password: "password"})
	require.ErrorContains(t, err, "session")
}

func TestNewXClientUnknownThreadFailsClosedBeforeHTTP(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		http.NotFound(writer, request)
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	tests := []struct {
		name string
		call func() error
	}{
		{name: "start run", call: func() error {
			_, err := client.StartRun(context.Background(), newXTestThreadID, testRunInput())
			return err
		}},
		{name: "stream run", call: func() error {
			_, err := client.StreamRun(context.Background(), newXTestThreadID, testRunInput())
			return err
		}},
		{name: "stream existing run", call: func() error {
			_, err := client.StreamExistingRun(context.Background(), newXTestThreadID, newXTestRunID, StreamOptions{})
			return err
		}},
		{name: "get run", call: func() error {
			_, err := client.GetRun(context.Background(), newXTestThreadID, newXTestRunID)
			return err
		}},
		{name: "cancel run", call: func() error { return client.CancelRun(context.Background(), newXTestThreadID, newXTestRunID) }},
		{name: "follow up", call: func() error {
			_, err := client.FollowUpRun(context.Background(), newXTestThreadID, newXTestRunID, testRunInput(), "continue")
			return err
		}},
		{name: "state", call: func() error { _, err := client.GetThreadState(context.Background(), newXTestThreadID); return err }},
		{name: "history", call: func() error {
			_, err := client.GetThreadHistory(context.Background(), newXTestThreadID, 20)
			return err
		}},
		{name: "messages", call: func() error {
			_, err := client.ListRunMessages(context.Background(), newXTestThreadID, newXTestRunID, PageRequest{})
			return err
		}},
		{name: "events", call: func() error {
			_, err := client.ListRunEvents(context.Background(), newXTestThreadID, newXTestRunID, 20)
			return err
		}},
		{name: "wait", call: func() error {
			return client.WaitForEvent(context.Background(), newXTestThreadID, newXTestRunID, "run.started")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, test.call(), "workspace")
		})
	}
	require.Zero(t, requestCount.Load())
}

func TestNewXClientRejectsSerializedCallerIdentityBeforeRunHTTP(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(*RunInput)
		wantErr      string
		wantRunCalls int32
		wantAnyErr   bool
	}{
		{
			name: "typed nested camel user id",
			mutate: func(input *RunInput) {
				input.Input["nested"] = map[string]string{"userId": "caller-1"}
			},
			wantErr: "caller identity",
		},
		{
			name: "typed nested camel owner id",
			mutate: func(input *RunInput) {
				input.Config["nested"] = map[string]string{"ownerId": "caller-1"}
			},
			wantErr: "caller identity",
		},
		{
			name: "typed nested camel space id",
			mutate: func(input *RunInput) {
				input.Context["nested"] = map[string]string{"spaceId": newXTestSpaceID}
			},
			wantErr: "caller identity",
		},
		{
			name: "typed nested snake case",
			mutate: func(input *RunInput) {
				input.Input["nested"] = map[string]string{"user_id": "caller-1"}
			},
			wantErr: "caller identity",
		},
		{
			name: "typed nested kebab case",
			mutate: func(input *RunInput) {
				input.Input["nested"] = map[string]string{"owner-id": "caller-1"}
			},
			wantErr: "caller identity",
		},
		{
			name: "identity words in text values",
			mutate: func(input *RunInput) {
				input.Input["note"] = "userId, ownerId, and spaceId are field names"
			},
			wantRunCalls: 1,
		},
		{
			name: "invalid JSON serialization",
			mutate: func(input *RunInput) {
				input.Input["invalid"] = make(chan int)
			},
			wantAnyErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var runCalls atomic.Int32
			server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/api/workbench/threads/"+newXTestThreadID+"/runs" {
					http.NotFound(writer, request)
					return
				}
				runCalls.Add(1)
				writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "pending"))
			})
			client := newCreatedNewXTestClient(t, server.URL)
			input := testRunInput()
			test.mutate(&input)

			_, err := client.StartRun(context.Background(), newXTestThreadID, input)
			switch {
			case test.wantErr != "":
				require.ErrorContains(t, err, test.wantErr)
			case test.wantAnyErr:
				require.Error(t, err)
			default:
				require.NoError(t, err)
			}
			require.Equal(t, test.wantRunCalls, runCalls.Load())
		})
	}
}

func TestNewXClientRejectsEmptyAndNullCanonicalJSONBodies(t *testing.T) {
	endpoints := []struct {
		name string
		call func(*NewXClient) error
	}{
		{name: "create thread", call: func(client *NewXClient) error {
			_, err := client.CreateThread(context.Background(), ThreadOptions{SpaceID: newXTestSpaceID})
			return err
		}},
		{name: "start run", call: func(client *NewXClient) error {
			_, err := client.StartRun(context.Background(), newXTestThreadID, testRunInput())
			return err
		}},
		{name: "get run", call: func(client *NewXClient) error {
			_, err := client.GetRun(context.Background(), newXTestThreadID, newXTestRunID)
			return err
		}},
		{name: "thread state", call: func(client *NewXClient) error {
			_, err := client.GetThreadState(context.Background(), newXTestThreadID)
			return err
		}},
		{name: "thread history", call: func(client *NewXClient) error {
			_, err := client.GetThreadHistory(context.Background(), newXTestThreadID, 20)
			return err
		}},
		{name: "run messages", call: func(client *NewXClient) error {
			_, err := client.ListRunMessages(context.Background(), newXTestThreadID, newXTestRunID, PageRequest{})
			return err
		}},
		{name: "run events", call: func(client *NewXClient) error {
			_, err := client.ListRunEvents(context.Background(), newXTestThreadID, newXTestRunID, 20)
			return err
		}},
		{name: "resume run", call: func(client *NewXClient) error {
			_, err := client.resumeHumanInteraction(
				context.Background(), newXTestThreadID, newXTestRunID,
				newXPendingHumanInteraction{
					InterruptID: "interrupt-1", InteractionID: "hi_1", InteractionKind: "clarification",
				},
				"continue",
			)
			return err
		}},
	}
	shapes := []struct {
		name    string
		payload string
		wantErr string
	}{
		{name: "empty", wantErr: "empty JSON body"},
		{name: "null", payload: "null", wantErr: "null JSON body"},
	}

	for _, endpoint := range endpoints {
		for _, shape := range shapes {
			t.Run(endpoint.name+"/"+shape.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					writer.Header().Set("X-Pagination-Total", "0")
					_, _ = fmt.Fprint(writer, shape.payload)
				}))
				t.Cleanup(server.Close)
				client := newBoundNewXTestClient(t, server.URL)
				require.ErrorContains(t, endpoint.call(client), shape.wantErr)
			})
		}
	}
}

func TestNewXClientCancelAcceptsEmptyNoContentResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client := newBoundNewXTestClient(t, server.URL)
	require.NoError(t, client.CancelRun(context.Background(), newXTestThreadID, newXTestRunID))
}

func TestNewXClientRejectsMalformedCanonicalThreadState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing values", mutate: func(state map[string]any) { delete(state, "values") }},
		{name: "values not object", mutate: func(state map[string]any) { state["values"] = []any{} }},
		{name: "next not array", mutate: func(state map[string]any) { state["next"] = map[string]any{} }},
		{name: "next item not string", mutate: func(state map[string]any) { state["next"] = []any{1} }},
		{name: "missing checkpoint", mutate: func(state map[string]any) { delete(state, "checkpoint") }},
		{name: "checkpoint thread mismatch", mutate: func(state map[string]any) {
			state["checkpoint"].(map[string]any)["thread_id"] = "7657000000000000999"
		}},
		{name: "checkpoint id not positive", mutate: func(state map[string]any) {
			state["checkpoint"].(map[string]any)["checkpoint_id"] = "0"
		}},
		{name: "checkpoint namespace not string", mutate: func(state map[string]any) {
			state["checkpoint"].(map[string]any)["checkpoint_ns"] = 1
		}},
		{name: "checkpoint map not object", mutate: func(state map[string]any) {
			state["checkpoint"].(map[string]any)["checkpoint_map"] = []any{}
		}},
		{name: "metadata not object", mutate: func(state map[string]any) { state["metadata"] = []any{} }},
		{name: "created at empty", mutate: func(state map[string]any) { state["created_at"] = "" }},
		{name: "tasks not array", mutate: func(state map[string]any) { state["tasks"] = map[string]any{} }},
		{name: "interrupts not array", mutate: func(state map[string]any) { state["interrupts"] = map[string]any{} }},
		{name: "invalid parent checkpoint", mutate: func(state map[string]any) {
			state["parent_checkpoint"] = map[string]any{
				"thread_id": newXTestThreadID, "checkpoint_ns": "", "checkpoint_id": "0", "checkpoint_map": map[string]any{},
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newXCanonicalThreadState(newXTestThreadID)
			test.mutate(state)
			server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
				require.Equal(t, "/api/workbench/threads/"+newXTestThreadID+"/state", request.URL.Path)
				writeTestJSON(writer, state)
			})
			client := newCreatedNewXTestClient(t, server.URL)
			_, err := client.GetThreadState(context.Background(), newXTestThreadID)
			require.ErrorContains(t, err, "canonical thread state")
		})
	}
}

func TestNewXClientValidatesEveryCanonicalHistoryItem(t *testing.T) {
	invalid := newXCanonicalThreadState(newXTestThreadID)
	delete(invalid, "created_at")
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/workbench/threads/"+newXTestThreadID+"/history", request.URL.Path)
		writeTestJSON(writer, []any{newXCanonicalThreadState(newXTestThreadID), invalid})
	})
	client := newCreatedNewXTestClient(t, server.URL)
	_, err := client.GetThreadHistory(context.Background(), newXTestThreadID, 20)
	require.ErrorContains(t, err, "canonical thread history item 1")
}

func TestNewXClientRejectsMalformedCanonicalMessagePages(t *testing.T) {
	tests := []struct {
		name        string
		page        PageRequest
		mutate      func(map[string]any, map[string]any)
		totalHeader string
		wantErr     bool
	}{
		{name: "missing data", mutate: func(response, _ map[string]any) { delete(response, "data") }, wantErr: true},
		{name: "null data", mutate: func(response, _ map[string]any) { response["data"] = nil }, wantErr: true},
		{name: "missing has more", mutate: func(response, _ map[string]any) { delete(response, "has_more") }, wantErr: true},
		{name: "missing message id", mutate: func(_ map[string]any, message map[string]any) { delete(message, "message_id") }, wantErr: true},
		{name: "missing thread id", mutate: func(_ map[string]any, message map[string]any) { delete(message, "thread_id") }, wantErr: true},
		{name: "missing run id", mutate: func(_ map[string]any, message map[string]any) { delete(message, "run_id") }, wantErr: true},
		{name: "missing role", mutate: func(_ map[string]any, message map[string]any) { delete(message, "role") }, wantErr: true},
		{name: "missing content", mutate: func(_ map[string]any, message map[string]any) { delete(message, "content") }, wantErr: true},
		{name: "missing metadata", mutate: func(_ map[string]any, message map[string]any) { delete(message, "metadata") }, wantErr: true},
		{name: "metadata not object", mutate: func(_ map[string]any, message map[string]any) { message["metadata"] = []any{} }, wantErr: true},
		{name: "missing created at", mutate: func(_ map[string]any, message map[string]any) { delete(message, "created_at") }, wantErr: true},
		{name: "empty created at", mutate: func(_ map[string]any, message map[string]any) { message["created_at"] = "" }, wantErr: true},
		{name: "thread identity mismatch", mutate: func(_ map[string]any, message map[string]any) { message["thread_id"] = "7657000000000000999" }, wantErr: true},
		{name: "run identity mismatch", mutate: func(_ map[string]any, message map[string]any) { message["run_id"] = "7657000000000000999" }, wantErr: true},
		{name: "has more with empty data", mutate: func(response, _ map[string]any) {
			response["data"] = []any{}
			response["has_more"] = true
			response["next_after_seq"] = "11"
		}, wantErr: true},
		{name: "default cursor not last sequence", mutate: func(response, _ map[string]any) {
			response["has_more"] = true
			response["next_after_seq"] = "12"
		}, wantErr: true},
		{name: "after cursor not last sequence", page: PageRequest{AfterSeq: 10}, mutate: func(response, _ map[string]any) {
			response["has_more"] = true
			response["next_after_seq"] = "12"
		}, wantErr: true},
		{name: "before cursor not first sequence", page: PageRequest{BeforeSeq: 20}, mutate: func(response, _ map[string]any) {
			response["has_more"] = true
			response["next_before_seq"] = "12"
		}, wantErr: true},
		{name: "terminal page advertises cursor", mutate: func(response, _ map[string]any) {
			response["next_after_seq"] = "11"
		}, wantErr: true},
		{name: "invalid pagination total", totalHeader: "invalid", wantErr: true},
		{name: "pagination total below data count", totalHeader: "0", wantErr: true},
		{name: "valid current server page without total header"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := newXCanonicalMessage("11")
			response := map[string]any{"data": []any{message}, "has_more": false}
			if test.mutate != nil {
				test.mutate(response, message)
			}
			server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
				require.Equal(t, "/api/workbench/threads/"+newXTestThreadID+"/runs/"+newXTestRunID+"/messages", request.URL.Path)
				if test.totalHeader != "" {
					writer.Header().Set("X-Pagination-Total", test.totalHeader)
				}
				writeTestJSON(writer, response)
			})
			client := newCreatedNewXTestClient(t, server.URL)
			_, err := client.ListRunMessages(context.Background(), newXTestThreadID, newXTestRunID, test.page)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewXClientSendsTheInspectedRunInputTree(t *testing.T) {
	stateful := &newXStatefulRunBody{}
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/workbench/threads/"+newXTestThreadID+"/runs", request.URL.Path)
		var body map[string]any
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		custom := body["input"].(map[string]any)["custom"].(map[string]any)
		require.Equal(t, "inspected", custom["safe"])
		require.NotContains(t, custom, "userId")
		writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "pending"))
	})
	client := newCreatedNewXTestClient(t, server.URL)
	input := testRunInput()
	input.Input["custom"] = stateful
	_, err := client.StartRun(context.Background(), newXTestThreadID, input)
	require.NoError(t, err)
	require.Equal(t, int32(1), stateful.calls.Load())
}

func testRunInput() RunInput {
	return RunInput{
		AssistantID: "lead_agent",
		Input: map[string]any{
			"messages": []any{map[string]any{"role": "user", "content": "safe prompt"}},
		},
		Config: map[string]any{"recursion_limit": 50},
		Context: map[string]any{
			"mode":             "flash",
			"thinking_enabled": false,
			"is_plan_mode":     false,
			"subagent_enabled": false,
		},
		StreamMode: []string{"events"},
	}
}

func writeTestJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func TestSafeHTTPClientFormEncodingDoesNotLeakIntoErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "secret", request.FormValue("password"))
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	client, err := newSafeHTTPClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	err = client.doForm(context.Background(), "login", "/", url.Values{"password": []string{"secret"}}, nil, nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
}

func TestStreamExistingRunStopAfterFramesCountsEventFrames(t *testing.T) {
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/workbench/threads/"+newXTestThreadID+"/runs/"+newXTestRunID+"/stream", request.URL.Path)
		require.Equal(t, "events", request.URL.Query().Get("stream_mode"))
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(writer, "event: metadata\ndata: {\"run_id\":%q,\"thread_id\":%q}\n\n", newXTestRunID, newXTestThreadID)
		_, _ = fmt.Fprintf(writer, "id: 1\nevent: events\ndata: {\"event_id\":\"1\",\"thread_id\":%q,\"run_id\":%q,\"event_type\":\"run.started\",\"payload\":{}}\n\n", newXTestThreadID, newXTestRunID)
		_, _ = fmt.Fprintf(writer, "id: 2\nevent: events\ndata: {\"event_id\":\"2\",\"thread_id\":%q,\"run_id\":%q,\"event_type\":\"assistant.completed\",\"payload\":{}}\n\n", newXTestThreadID, newXTestRunID)
	})

	client := newCreatedNewXTestClient(t, server.URL)
	stream, err := client.StreamExistingRun(
		context.Background(), newXTestThreadID, newXTestRunID, StreamOptions{StopAfterFrames: 1},
	)
	require.NoError(t, err)
	require.Equal(t, "1", stream.LastEventID)
	require.Len(t, stream.Frames, 2)
}

func TestNewXClientStreamRunReconnectsUntilCanonicalTerminal(t *testing.T) {
	streamCalls := 0
	statusCalls := 0
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/workbench/threads/" + newXTestThreadID + "/runs":
			require.Equal(t, http.MethodPost, request.Method)
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "pending"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/stream":
			streamCalls++
			require.Equal(t, "events", request.URL.Query().Get("stream_mode"))
			writer.Header().Set("Content-Type", "text/event-stream")
			if streamCalls == 1 {
				require.Empty(t, request.URL.Query().Get("after_event_id"))
				require.Empty(t, request.Header.Get("Last-Event-ID"))
				_, _ = fmt.Fprintf(writer, "event: metadata\ndata: {\"run_id\":%q,\"thread_id\":%q,\"status\":\"running\"}\n\n", newXTestRunID, newXTestThreadID)
				_, _ = fmt.Fprintf(writer, "id: 1\nevent: events\ndata: {\"event_id\":\"1\",\"thread_id\":%q,\"run_id\":%q,\"event_type\":\"run.started\",\"payload\":{}}\n\n", newXTestThreadID, newXTestRunID)
				return
			}
			require.Equal(t, "1", request.URL.Query().Get("after_event_id"))
			require.Equal(t, "1", request.Header.Get("Last-Event-ID"))
			_, _ = fmt.Fprintf(writer, "event: metadata\ndata: {\"run_id\":%q,\"thread_id\":%q,\"status\":\"running\"}\n\n", newXTestRunID, newXTestThreadID)
			_, _ = fmt.Fprintf(writer, "id: 2\nevent: events\ndata: {\"event_id\":\"2\",\"thread_id\":%q,\"run_id\":%q,\"event_type\":\"assistant.completed\",\"payload\":{}}\n\n", newXTestThreadID, newXTestRunID)
			_, _ = fmt.Fprintf(writer, "event: end\ndata: {\"run_id\":%q,\"thread_id\":%q,\"status\":\"success\",\"reason\":\"terminal_run\"}\n\n", newXTestRunID, newXTestThreadID)
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID:
			statusCalls++
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "running"))
		default:
			http.NotFound(writer, request)
		}
	})

	client := newCreatedNewXTestClient(t, server.URL)
	stream, err := client.StreamRun(context.Background(), newXTestThreadID, testRunInput())
	require.NoError(t, err)
	require.Equal(t, "success", stream.Terminal)
	require.True(t, stream.TerminalFrameObserved)
	require.Equal(t, "2", stream.LastEventID)
	require.Len(t, stream.Frames, 5)
	require.Equal(t, 2, streamCalls)
	require.Equal(t, 1, statusCalls)
}

func TestStreamResultFromFramesPreservesPositiveFallbackRunIDWhenMetadataOmitsIt(t *testing.T) {
	t.Parallel()

	result, err := streamResultFromFrames("newx", newXTestThreadID, newXTestRunID, []SSEFrame{
		{Event: "metadata", Data: []byte(`{"thread_id":"7657000000000000000"}`)},
		{Event: "end", Data: []byte(`{"status":"completed"}`)},
	})
	require.NoError(t, err)
	require.Equal(t, newXTestRunID, result.RunID)
	require.Equal(t, "success", result.Terminal)
	require.True(t, result.TerminalFrameObserved)
}

func TestNewXClientStreamExistingRunRejectsRESTTerminalWithoutTerminalSSE(t *testing.T) {
	streamCalls := 0
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/stream":
			streamCalls++
			writer.Header().Set("Content-Type", "text/event-stream")
			if streamCalls == 1 {
				_, _ = fmt.Fprintf(writer, "event: metadata\ndata: {\"run_id\":%q,\"thread_id\":%q}\n\n", newXTestRunID, newXTestThreadID)
				_, _ = fmt.Fprintf(writer, "id: 1\nevent: events\ndata: {\"event_id\":\"1\",\"thread_id\":%q,\"run_id\":%q,\"event_type\":\"assistant.completed\",\"payload\":{}}\n\n", newXTestThreadID, newXTestRunID)
			} else {
				_, _ = fmt.Fprintf(writer, "event: metadata\ndata: {\"run_id\":%q,\"thread_id\":%q}\n\n", newXTestRunID, newXTestThreadID)
			}
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID:
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "success"))
		default:
			http.NotFound(writer, request)
		}
	})

	client := newCreatedNewXTestClient(t, server.URL)
	_, err := client.StreamExistingRun(context.Background(), newXTestThreadID, newXTestRunID, StreamOptions{})
	require.ErrorContains(t, err, "omitted terminal frame")
	require.Equal(t, 2, streamCalls)
}

func TestNewXClientRejectsInvalidCanonicalSSEFrames(t *testing.T) {
	otherThreadID := "7657000000000000998"
	otherRunID := "7657000000000000999"
	tests := []struct {
		name         string
		afterEventID string
		stream       string
	}{
		{
			name:   "metadata thread mismatch",
			stream: newXTestSSEMetadata(otherThreadID, newXTestRunID),
		},
		{
			name:   "metadata run mismatch",
			stream: newXTestSSEMetadata(newXTestThreadID, otherRunID),
		},
		{
			name: "event payload thread mismatch",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) +
				newXTestSSEEvent("1", "1", otherThreadID, newXTestRunID, "run.started"),
		},
		{
			name: "event payload run mismatch",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) +
				newXTestSSEEvent("1", "1", newXTestThreadID, otherRunID, "run.started"),
		},
		{
			name: "SSE id differs from payload event id",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) +
				newXTestSSEEvent("1", "2", newXTestThreadID, newXTestRunID, "run.started"),
		},
		{
			name: "duplicate event ids within segment",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) +
				newXTestSSEEvent("6", "6", newXTestThreadID, newXTestRunID, "run.started") +
				newXTestSSEEvent("6", "6", newXTestThreadID, newXTestRunID, "assistant.completed"),
		},
		{
			name: "backward event ids within segment",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) +
				newXTestSSEEvent("7", "7", newXTestThreadID, newXTestRunID, "run.started") +
				newXTestSSEEvent("6", "6", newXTestThreadID, newXTestRunID, "assistant.completed"),
		},
		{
			name:         "event id equals reconnect cursor",
			afterEventID: "5",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) +
				newXTestSSEEvent("5", "5", newXTestThreadID, newXTestRunID, "run.started"),
		},
		{
			name:         "event id precedes reconnect cursor",
			afterEventID: "5",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) +
				newXTestSSEEvent("4", "4", newXTestThreadID, newXTestRunID, "run.started"),
		},
		{
			name:   "unknown terminal status",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) + newXTestSSEEnd(newXTestThreadID, newXTestRunID, "mystery"),
		},
		{
			name:   "empty terminal status",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) + newXTestSSEEnd(newXTestThreadID, newXTestRunID, ""),
		},
		{
			name:   "end thread mismatch",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) + newXTestSSEEnd(otherThreadID, newXTestRunID, "success"),
		},
		{
			name:   "end run mismatch",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) + newXTestSSEEnd(newXTestThreadID, otherRunID, "success"),
		},
		{
			name: "unknown frame semantics",
			stream: newXTestSSEMetadata(newXTestThreadID, newXTestRunID) +
				"id: 1\nevent: updates\ndata: {}\n\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
				require.Equal(t, "/api/workbench/threads/"+newXTestThreadID+"/runs/"+newXTestRunID+"/stream", request.URL.Path)
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(writer, test.stream)
			})
			client := newCreatedNewXTestClient(t, server.URL)
			_, err := client.StreamExistingRun(
				context.Background(), newXTestThreadID, newXTestRunID,
				StreamOptions{AfterEventID: test.afterEventID, StopAfterFrames: 100},
			)
			require.Error(t, err)
		})
	}
}

func TestNewXClientFollowUpFindsCanonicalInteractionAcrossEventPagesAndResumes(t *testing.T) {
	pages := 0
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID:
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "interrupted"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/events":
			pages++
			require.Equal(t, "200", request.URL.Query().Get("limit"))
			writer.Header().Set("X-Pagination-Total", "2")
			if pages == 1 {
				require.Empty(t, request.URL.Query().Get("after_event_id"))
				writeTestJSON(writer, map[string]any{
					"data": []any{map[string]any{
						"event_id": "1", "thread_id": newXTestThreadID, "run_id": newXTestRunID,
						"event_type": "run.started", "payload": map[string]any{}, "created_at": "2026-07-29T00:00:00Z",
					}},
					"has_more": true, "next_after_event_id": "1",
				})
				return
			}
			require.Equal(t, "1", request.URL.Query().Get("after_event_id"))
			writeTestJSON(writer, map[string]any{
				"data": []any{map[string]any{
					"event_id": "2", "thread_id": newXTestThreadID, "run_id": newXTestRunID,
					"event_type": "run.interrupted", "created_at": "2026-07-29T00:00:00Z",
					"payload": map[string]any{
						"interrupts": map[string]any{
							"items": []any{map[string]any{
								"id": "interrupt-1",
								"info": map[string]any{
									"schema": "coze.human_interaction.v1", "interaction_id": "hi_1", "kind": "clarification",
								},
								"is_root_cause": true,
							}},
						},
					},
				}},
				"has_more": false,
			})
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/resume":
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, "interrupt-1", body["interrupt_id"])
			response := body["response"].(map[string]any)
			require.Equal(t, "coze.human_interaction_response.v1", response["schema"])
			require.Equal(t, "hi_1", response["interaction_id"])
			require.Equal(t, "clarification", response["kind"])
			require.Equal(t, "answered", response["decision"])
			require.Equal(t, "方案 A", response["answer"])
			writeTestJSON(writer, newXCanonicalRun(newXTestFollowUpRunID, "pending"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestFollowUpRunID + "/stream":
			require.Equal(t, "events", request.URL.Query().Get("stream_mode"))
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(writer, "event: metadata\ndata: {\"run_id\":%q,\"thread_id\":%q}\n\n", newXTestFollowUpRunID, newXTestThreadID)
			_, _ = fmt.Fprintf(writer, "event: end\ndata: {\"run_id\":%q,\"thread_id\":%q,\"status\":\"success\"}\n\n", newXTestFollowUpRunID, newXTestThreadID)
		default:
			http.NotFound(writer, request)
		}
	})

	client := newCreatedNewXTestClient(t, server.URL)
	followedUp, err := client.FollowUpRun(
		context.Background(), newXTestThreadID, newXTestRunID, testRunInput(), "方案 A",
	)
	require.NoError(t, err)
	require.Equal(t, newXTestFollowUpRunID, followedUp.RunID)
	require.Equal(t, "success", followedUp.Terminal)
	require.Equal(t, 2, pages)
}

func TestNewXClientPendingInteractionRejectsBackwardAlternatingCursor(t *testing.T) {
	var pages atomic.Int32
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID:
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "interrupted"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/events":
			page := pages.Add(1)
			writer.Header().Set("X-Pagination-Total", "10")
			switch page {
			case 1:
				require.Empty(t, request.URL.Query().Get("after_event_id"))
				writeTestJSON(writer, map[string]any{
					"data": []any{map[string]any{
						"event_id": "2", "thread_id": newXTestThreadID, "run_id": newXTestRunID,
						"event_type": "run.started", "payload": map[string]any{},
					}},
					"has_more": true, "next_after_event_id": "2",
				})
			case 2:
				require.Equal(t, "2", request.URL.Query().Get("after_event_id"))
				writeTestJSON(writer, map[string]any{
					"data": []any{map[string]any{
						"event_id": "1", "thread_id": newXTestThreadID, "run_id": newXTestRunID,
						"event_type": "run.started", "payload": map[string]any{},
					}},
					"has_more": true, "next_after_event_id": "1",
				})
			default:
				http.Error(writer, "unexpected extra page", http.StatusInternalServerError)
			}
		default:
			http.NotFound(writer, request)
		}
	})

	client := newCreatedNewXTestClient(t, server.URL)
	_, err := client.FollowUpRun(
		context.Background(), newXTestThreadID, newXTestRunID, testRunInput(), "继续",
	)
	require.ErrorContains(t, err, "pagination made no progress")
	require.Equal(t, int32(2), pages.Load())
}

func TestNewXClientPendingInteractionRejectsCursorDifferentFromLastEvent(t *testing.T) {
	var pages atomic.Int32
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID:
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "interrupted"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/events":
			page := pages.Add(1)
			if page > 1 {
				http.Error(writer, "unexpected extra page", http.StatusInternalServerError)
				return
			}
			writer.Header().Set("X-Pagination-Total", "10")
			writeTestJSON(writer, map[string]any{
				"data": []any{map[string]any{
					"event_id": "5", "thread_id": newXTestThreadID, "run_id": newXTestRunID,
					"event_type": "run.started", "payload": map[string]any{},
				}},
				"has_more": true, "next_after_event_id": "6",
			})
		default:
			http.NotFound(writer, request)
		}
	})

	client := newCreatedNewXTestClient(t, server.URL)
	_, err := client.FollowUpRun(
		context.Background(), newXTestThreadID, newXTestRunID, testRunInput(), "继续",
	)
	require.ErrorContains(t, err, "pagination made no progress")
	require.Equal(t, int32(1), pages.Load())
}

func TestNewXClientPendingInteractionRejectsPageInternalEventRegression(t *testing.T) {
	var pages atomic.Int32
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID:
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "interrupted"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/events":
			page := pages.Add(1)
			writer.Header().Set("X-Pagination-Total", "10")
			switch page {
			case 1:
				require.Empty(t, request.URL.Query().Get("after_event_id"))
				writeTestJSON(writer, map[string]any{
					"data": []any{map[string]any{
						"event_id": "5", "thread_id": newXTestThreadID, "run_id": newXTestRunID,
						"event_type": "run.started", "payload": map[string]any{},
					}},
					"has_more": true, "next_after_event_id": "5",
				})
			case 2:
				require.Equal(t, "5", request.URL.Query().Get("after_event_id"))
				writeTestJSON(writer, map[string]any{
					"data": []any{
						map[string]any{
							"event_id": "100", "thread_id": newXTestThreadID, "run_id": newXTestRunID,
							"event_type": "run.running", "payload": map[string]any{},
						},
						map[string]any{
							"event_id": "6", "thread_id": newXTestThreadID, "run_id": newXTestRunID,
							"event_type": "run.interrupted", "payload": map[string]any{},
						},
					},
					"has_more": true, "next_after_event_id": "6",
				})
			default:
				http.Error(writer, "unexpected extra page", http.StatusInternalServerError)
			}
		default:
			http.NotFound(writer, request)
		}
	})

	client := newCreatedNewXTestClient(t, server.URL)
	_, err := client.FollowUpRun(
		context.Background(), newXTestThreadID, newXTestRunID, testRunInput(), "继续",
	)
	require.ErrorContains(t, err, "event ids are not strictly increasing")
	require.Equal(t, int32(2), pages.Load())
}

func TestNewXClientInterruptedWithoutClarificationFallsBackToOrdinaryFollowUp(t *testing.T) {
	started := 0
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID:
			writeTestJSON(writer, newXCanonicalRun(newXTestRunID, "interrupted"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestRunID + "/events":
			writer.Header().Set("X-Pagination-Total", "0")
			writeTestJSON(writer, map[string]any{
				"data": []any{}, "has_more": false,
			})
		case "/api/workbench/threads/" + newXTestThreadID + "/runs":
			started++
			writeTestJSON(writer, newXCanonicalRun(newXTestFollowUpRunID, "pending"))
		case "/api/workbench/threads/" + newXTestThreadID + "/runs/" + newXTestFollowUpRunID + "/stream":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(writer, "event: metadata\ndata: {\"run_id\":%q,\"thread_id\":%q}\n\n", newXTestFollowUpRunID, newXTestThreadID)
			_, _ = fmt.Fprintf(writer, "event: end\ndata: {\"run_id\":%q,\"thread_id\":%q,\"status\":\"success\"}\n\n", newXTestFollowUpRunID, newXTestThreadID)
		default:
			http.NotFound(writer, request)
		}
	})

	client := newCreatedNewXTestClient(t, server.URL)
	result, err := client.FollowUpRun(
		context.Background(), newXTestThreadID, newXTestRunID, testRunInput(), "继续",
	)
	require.NoError(t, err)
	require.Equal(t, newXTestFollowUpRunID, result.RunID)
	require.Equal(t, 1, started)
}

func TestNewXClientRejectsInvalidCanonicalPaginationTotal(t *testing.T) {
	server := newXCanonicalTestServer(t, func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/workbench/threads/"+newXTestThreadID+"/runs/"+newXTestRunID+"/events", request.URL.Path)
		writer.Header().Set("X-Pagination-Total", "not-a-decimal")
		writeTestJSON(writer, map[string]any{"data": []any{}, "has_more": false})
	})

	client := newCreatedNewXTestClient(t, server.URL)
	_, err := client.ListRunEvents(context.Background(), newXTestThreadID, newXTestRunID, 20)
	require.ErrorContains(t, err, "X-Pagination-Total")
}

func TestDeerFlowClientWaitForRunStartedUsesLiveRunStatus(t *testing.T) {
	t.Parallel()

	getCalls := 0
	eventCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/threads/thread-1/runs/run-1":
			getCalls++
			status := "pending"
			if getCalls > 1 {
				status = "running"
			}
			writeTestJSON(writer, map[string]any{
				"thread_id": "thread-1", "run_id": "run-1", "status": status,
			})
		case "/api/threads/thread-1/runs/run-1/events":
			eventCalls++
			writeTestJSON(writer, []any{})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewDeerFlowClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, client.WaitForEvent(ctx, "thread-1", "run-1", "run.started"))
	require.Equal(t, 2, getCalls)
	require.Zero(t, eventCalls)
}

func TestDeerFlowClientCancelWaitsForInterruptToSettle(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/api/threads/thread-1/runs/run-1/cancel", request.URL.Path)
		require.Equal(t, "true", request.URL.Query().Get("wait"))
		require.Equal(t, "interrupt", request.URL.Query().Get("action"))
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client, err := NewDeerFlowClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, client.CancelRun(context.Background(), "thread-1", "run-1"))
}
