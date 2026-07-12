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

func TestNewXClientUsesJSONLoginAndServerOwnedNumericThread(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/passport/web/email/login/":
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, "user@example.com", body["email"])
			http.SetCookie(writer, &http.Cookie{Name: "session_key", Value: "private", Path: "/"})
			writeTestJSON(writer, map[string]any{"code": 0})
		case "/api/threads":
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			metadata := body["metadata"].(map[string]any)
			require.Equal(t, "7656103552997130240", metadata["space_id"])
			require.NotContains(t, metadata, "user_id")
			writeTestJSON(writer, map[string]any{"thread_id": "7657000000000000000"})
		case "/api/threads/7657000000000000000/runs":
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, "lead_agent", body["assistant_id"])
			writeTestJSON(writer, map[string]any{
				"thread_id": "7657000000000000000",
				"run_id":    "7657000000000000001",
				"status":    "pending",
			})
		case "/api/threads/7657000000000000000/runs/7657000000000000001/stream":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "id: 1\nevent: events\ndata: {\"event_type\":\"run.started\",\"payload\":{}}\n\n")
			_, _ = fmt.Fprint(writer, "event: end\ndata: {\"status\":\"succeeded\"}\n\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	require.NoError(t, client.Login(context.Background(), Credentials{Email: "user@example.com", Password: "password"}))
	threadID, err := client.CreateThread(context.Background(), ThreadOptions{SpaceID: "7656103552997130240"})
	require.NoError(t, err)
	require.Equal(t, "7657000000000000000", threadID)

	stream, err := client.StreamRun(context.Background(), threadID, testRunInput())
	require.NoError(t, err)
	require.Equal(t, "7657000000000000001", stream.RunID)
	require.Equal(t, "success", stream.Terminal)
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

func TestNewXClientStreamRunStartsThenFollowsExistingRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/threads/thread-1/runs":
			require.Equal(t, http.MethodPost, request.Method)
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, "lead_agent", body["assistant_id"])
			writeTestJSON(writer, map[string]any{
				"thread_id": "thread-1",
				"run_id":    "run-1",
				"status":    "pending",
			})
		case "/api/threads/thread-1/runs/run-1/stream":
			require.Equal(t, http.MethodGet, request.Method)
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "id: 1\nevent: events\ndata: {\"event_type\":\"run.started\",\"payload\":{}}\n\n")
			_, _ = fmt.Fprint(writer, "event: end\ndata: {\"status\":\"success\"}\n\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	stream, err := client.StreamRun(context.Background(), "thread-1", testRunInput())
	require.NoError(t, err)
	require.Equal(t, "run-1", stream.RunID)
	require.Equal(t, "success", stream.Terminal)
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
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/threads/thread-1/runs/run-1/stream", request.URL.Path)
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(writer, "event: metadata\ndata: {\"run_id\":\"run-1\",\"thread_id\":\"thread-1\"}\n\n")
		_, _ = fmt.Fprint(writer, "id: 1\nevent: events\ndata: {\"event_type\":\"run.started\",\"payload\":{}}\n\n")
		_, _ = fmt.Fprint(writer, "id: 2\nevent: events\ndata: {\"event_type\":\"assistant.completed\",\"payload\":{}}\n\n")
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	stream, err := client.StreamExistingRun(context.Background(), "thread-1", "run-1", StreamOptions{StopAfterFrames: 1})
	require.NoError(t, err)
	require.Equal(t, "1", stream.LastEventID)
	require.Len(t, stream.Frames, 2)
}

func TestNewXClientStreamExistingRunReconnectsUntilTerminal(t *testing.T) {
	streamCalls := 0
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/threads/thread-1/runs/run-1/stream":
			streamCalls++
			require.Equal(t, "events", request.URL.Query().Get("stream_mode"))
			writer.Header().Set("Content-Type", "text/event-stream")
			if streamCalls == 1 {
				require.Empty(t, request.URL.Query().Get("after_event_id"))
				require.Empty(t, request.Header.Get("Last-Event-ID"))
				_, _ = fmt.Fprint(writer, "id: 1\nevent: events\ndata: {\"event_type\":\"run.started\",\"payload\":{}}\n\n")
				return
			}
			require.Equal(t, "1", request.URL.Query().Get("after_event_id"))
			require.Equal(t, "1", request.Header.Get("Last-Event-ID"))
			_, _ = fmt.Fprint(writer, "id: 2\nevent: events\ndata: {\"event_type\":\"assistant.completed\",\"payload\":{}}\n\n")
			_, _ = fmt.Fprint(writer, "event: end\ndata: {\"status\":\"success\"}\n\n")
		case "/api/threads/thread-1/runs/run-1":
			statusCalls++
			writeTestJSON(writer, map[string]any{
				"thread_id": "thread-1",
				"run_id":    "run-1",
				"status":    "running",
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	stream, err := client.StreamExistingRun(context.Background(), "thread-1", "run-1", StreamOptions{})
	require.NoError(t, err)
	require.Equal(t, "success", stream.Terminal)
	require.True(t, stream.TerminalFrameObserved)
	require.Equal(t, "2", stream.LastEventID)
	require.Len(t, stream.Frames, 3)
	require.Equal(t, 2, streamCalls)
	require.Equal(t, 1, statusCalls)
}

func TestStreamResultFromFramesPreservesFallbackRunIDWhenMetadataOmitsIt(t *testing.T) {
	t.Parallel()

	result, err := streamResultFromFrames("newx", "thread-1", "run-1", []SSEFrame{
		{Event: "metadata", Data: []byte(`{"thread_id":"thread-1"}`)},
		{Event: "end", Data: []byte(`{"status":"completed"}`)},
	})
	require.NoError(t, err)
	require.Equal(t, "run-1", result.RunID)
	require.Equal(t, "success", result.Terminal)
	require.True(t, result.TerminalFrameObserved)
}

func TestNewXClientStreamExistingRunRejectsRESTTerminalWithoutTerminalSSE(t *testing.T) {
	streamCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/threads/thread-1/runs/run-1/stream":
			streamCalls++
			writer.Header().Set("Content-Type", "text/event-stream")
			if streamCalls == 1 {
				_, _ = fmt.Fprint(writer, "id: 1\nevent: events\ndata: {\"event_type\":\"assistant.completed\",\"payload\":{}}\n\n")
			}
		case "/api/threads/thread-1/runs/run-1":
			writeTestJSON(writer, map[string]any{
				"thread_id": "thread-1", "run_id": "run-1", "status": "success",
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	_, err = client.StreamExistingRun(context.Background(), "thread-1", "run-1", StreamOptions{})
	require.ErrorContains(t, err, "omitted terminal frame")
	require.Equal(t, 2, streamCalls)
}

func TestNewXClientFollowUpResumesAnInterruptedRunOnTheSameThread(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/threads/thread-1/runs/run-1":
			writeTestJSON(writer, map[string]any{
				"thread_id": "thread-1",
				"run_id":    "run-1",
				"status":    "interrupted",
			})
		case "/api/workbench/task_threads/thread-1/run_events":
			require.Equal(t, "run-1", request.URL.Query().Get("run_id"))
			writeTestJSON(writer, map[string]any{"code": 0, "data": map[string]any{
				"total": 1,
				"events": []map[string]any{{
					"event_id":   "7",
					"event_type": "run.interrupted",
					"payload": `{
					"interrupts":{"items":[{
						"id":"interrupt-1",
						"info":{"schema":"coze.human_interaction.v1","interaction_id":"hi_1","kind":"clarification"},
						"is_root_cause":true
					}]}
				}`,
				}},
			}})
		case "/api/workbench/task_threads/thread-1/runs/run-1/resume":
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, "interrupt-1", body["interrupt_id"])
			response := body["response"].(map[string]any)
			require.Equal(t, "coze.human_interaction_response.v1", response["schema"])
			require.Equal(t, "hi_1", response["interaction_id"])
			require.Equal(t, "clarification", response["kind"])
			require.Equal(t, "answered", response["decision"])
			require.Equal(t, "方案 A", response["answer"])
			writeTestJSON(writer, map[string]any{
				"code": 0,
				"data": map[string]any{
					"thread_id": "thread-1",
					"run_id":    "run-2",
					"status":    "queued",
				},
			})
		case "/api/threads/thread-1/runs/run-2/stream":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "event: end\ndata: {\"status\":\"success\"}\n\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	followedUp, err := client.FollowUpRun(context.Background(), "thread-1", "run-1", testRunInput(), "方案 A")
	require.NoError(t, err)
	require.Equal(t, "run-2", followedUp.RunID)
	require.Equal(t, "success", followedUp.Terminal)
}

func TestNewXClientFollowUpFindsClarificationAcrossEventPages(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/threads/thread-1/runs/run-1":
			writeTestJSON(writer, map[string]any{
				"thread_id": "thread-1", "run_id": "run-1", "status": "interrupted",
			})
		case "/api/workbench/task_threads/thread-1/run_events":
			pages++
			page := request.URL.Query().Get("page")
			events := []map[string]any{{
				"event_id": page, "event_type": "run.started", "payload": `{}`,
			}}
			if page == "2" {
				events = []map[string]any{{
					"event_id": "201", "event_type": "run.interrupted",
					"payload": `{"interrupts":{"items":[{"id":"interrupt-2","info":{"schema":"coze.human_interaction.v1","interaction_id":"hi_2","kind":"clarification"}}]}}`,
				}}
			}
			writeTestJSON(writer, map[string]any{
				"code": 0,
				"data": map[string]any{"events": events, "total": 201},
			})
		case "/api/workbench/task_threads/thread-1/runs/run-1/resume":
			writeTestJSON(writer, map[string]any{
				"code": 0,
				"data": map[string]any{"thread_id": "thread-1", "run_id": "run-2", "status": "queued"},
			})
		case "/api/threads/thread-1/runs/run-2/stream":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "event: end\ndata: {\"status\":\"success\"}\n\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	result, err := client.FollowUpRun(
		context.Background(), "thread-1", "run-1", testRunInput(), "继续",
	)
	require.NoError(t, err)
	require.Equal(t, "run-2", result.RunID)
	require.Equal(t, 2, pages)
}

func TestNewXClientInterruptedWithoutClarificationFallsBackToOrdinaryFollowUp(t *testing.T) {
	started := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/threads/thread-1/runs/run-1":
			writeTestJSON(writer, map[string]any{
				"thread_id": "thread-1", "run_id": "run-1", "status": "interrupted",
			})
		case "/api/workbench/task_threads/thread-1/run_events":
			writeTestJSON(writer, map[string]any{
				"code": 0,
				"data": map[string]any{"events": []any{}, "total": 0},
			})
		case "/api/threads/thread-1/runs":
			started++
			writeTestJSON(writer, map[string]any{
				"thread_id": "thread-1", "run_id": "run-2", "status": "queued",
			})
		case "/api/threads/thread-1/runs/run-2/stream":
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "event: end\ndata: {\"status\":\"success\"}\n\n")
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewNewXClient(server.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	result, err := client.FollowUpRun(
		context.Background(), "thread-1", "run-1", testRunInput(), "继续",
	)
	require.NoError(t, err)
	require.Equal(t, "run-2", result.RunID)
	require.Equal(t, 1, started)
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
