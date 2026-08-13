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

package aio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	"github.com/stretchr/testify/require"
)

func TestUpstreamSDKDoesNotClaimUnixIdentity(t *testing.T) {
	requestTypes := []reflect.Type{
		reflect.TypeOf(sandboxapi.ShellCreateSessionRequest{}),
		reflect.TypeOf(sandboxapi.ShellExecRequest{}),
		reflect.TypeOf(sandboxapi.FileWriteRequest{}),
	}
	for _, requestType := range requestTypes {
		for _, name := range []string{"UID", "GID", "User", "Username"} {
			_, found := requestType.FieldByName(name)
			require.False(t, found, "%s unexpectedly exposes %s", requestType, name)
		}
	}
}

func TestUpstreamSDKRequestFieldsRemainV005(t *testing.T) {
	tests := []struct {
		request any
		fields  []string
	}{
		{sandboxapi.ShellCreateSessionRequest{}, []string{"id", "exec_dir", "no_change_timeout", "preserve_symlinks"}},
		{sandboxapi.ShellExecRequest{}, []string{"id", "exec_dir", "command", "async_mode", "timeout", "strict", "no_change_timeout", "hard_timeout", "preserve_symlinks", "truncate"}},
		{sandboxapi.ShellViewRequest{}, []string{"id"}},
		{sandboxapi.ShellWaitRequest{}, []string{"id", "seconds", "max_wait_seconds"}},
		{sandboxapi.ShellKillProcessRequest{}, []string{"id"}},
		{sandboxapi.FileReadRequest{}, []string{"file", "start_line", "end_line", "sudo"}},
		{sandboxapi.FileWriteRequest{}, []string{"file", "content", "encoding", "append", "leading_newline", "trailing_newline", "sudo"}},
		{sandboxapi.FileListRequest{}, []string{"path", "recursive", "show_hidden", "file_types", "max_depth", "include_size", "include_permissions", "sort_by", "sort_desc"}},
		{sandboxapi.FileGlobRequest{}, []string{"path", "pattern", "exclude", "include_hidden", "files_only", "include_metadata", "max_results", "sort_by", "sort_desc"}},
		{sandboxapi.FileGrepRequest{}, []string{"path", "pattern", "include", "exclude", "case_insensitive", "fixed_strings", "context_before", "context_after", "max_results", "max_file_size", "multiline", "offset", "type", "recursive"}},
		{sandboxapi.FileReplaceRequest{}, []string{"file", "old_str", "new_str", "sudo"}},
		{sandboxapi.FileDownloadFileRequest{}, []string{"-"}},
	}
	for _, test := range tests {
		t.Run(reflect.TypeOf(test.request).Name(), func(t *testing.T) {
			typeOf := reflect.TypeOf(test.request)
			actual := make([]string, 0, typeOf.NumField())
			for index := 0; index < typeOf.NumField(); index++ {
				field := typeOf.Field(index)
				if !field.IsExported() {
					continue
				}
				actual = append(actual, strings.Split(field.Tag.Get("json"), ",")[0])
			}
			require.Equal(t, test.fields, actual)
		})
	}
}

func TestUpstreamClientRejectsUnboundedOrNonOriginConfiguration(t *testing.T) {
	valid := UpstreamClientConfig{
		BaseURL:    "http://127.0.0.1:8080",
		BearerJWT:  "temporary-jwt",
		HTTPClient: &http.Client{Timeout: time.Second},
	}
	mutations := []func(*UpstreamClientConfig){
		func(config *UpstreamClientConfig) { config.BaseURL = "" },
		func(config *UpstreamClientConfig) { config.BaseURL = "ftp://127.0.0.1:8080" },
		func(config *UpstreamClientConfig) { config.BaseURL += "/v1" },
		func(config *UpstreamClientConfig) { config.BaseURL += "?jwt=secret" },
		func(config *UpstreamClientConfig) { config.BaseURL = "http://user:secret@127.0.0.1:8080" },
		func(config *UpstreamClientConfig) { config.HTTPClient = nil },
		func(config *UpstreamClientConfig) { config.HTTPClient = &http.Client{} },
		func(config *UpstreamClientConfig) {
			config.HTTPClient = &http.Client{Timeout: maxUpstreamHTTPTimeout + time.Second}
		},
	}
	for index, mutate := range mutations {
		t.Run(fmt.Sprintf("invalid-%d", index), func(t *testing.T) {
			config := valid
			mutate(&config)
			client, err := NewUpstreamClient(config)
			require.ErrorIs(t, err, ErrInvalidUpstreamConfig)
			require.Nil(t, client)
		})
	}
}

func TestUpstreamClientOmitsAuthorizationWithoutBearerJWT(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"data":{}}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewUpstreamClient(UpstreamClientConfig{
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})
	require.NoError(t, err)

	_, err = client.View(context.Background(), &sandboxapi.ShellViewRequest{Id: "newx-probe"})
	require.NoError(t, err)
	require.Empty(t, authorization)
}

func TestUpstreamClientUsesConfiguredOriginBearerAndV005Routes(t *testing.T) {
	type observedRequest struct {
		method        string
		path          string
		authorization string
		query         string
		body          map[string]any
	}
	var (
		mu       sync.Mutex
		observed []observedRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body := map[string]any{}
		if request.Body != nil && request.ContentLength != 0 {
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		}
		mu.Lock()
		observed = append(observed, observedRequest{
			method:        request.Method,
			path:          request.URL.Path,
			authorization: request.Header.Get("Authorization"),
			query:         request.URL.RawQuery,
			body:          body,
		})
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"data":{}}`))
	}))
	t.Cleanup(server.Close)

	client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	falseValue := false
	one := 1
	two := 2
	utf8 := sandboxapi.FileContentEncodingUtf8
	calls := []struct {
		method string
		path   string
		body   map[string]any
		call   func() error
	}{
		{http.MethodPost, "/v1/shell/sessions/create", map[string]any{"id": "newx-probe", "exec_dir": "/tmp/probe"}, func() error {
			_, err := client.Create(ctx, &sandboxapi.ShellCreateSessionRequest{Id: ptr("newx-probe"), ExecDir: ptr("/tmp/probe")})
			return err
		}},
		{http.MethodPost, "/v1/shell/exec", map[string]any{"id": "newx-probe", "command": "pwd", "async_mode": false}, func() error {
			_, err := client.Exec(ctx, &sandboxapi.ShellExecRequest{Id: ptr("newx-probe"), Command: "pwd", AsyncMode: &falseValue})
			return err
		}},
		{http.MethodPost, "/v1/shell/view", map[string]any{"id": "newx-probe"}, func() error {
			_, err := client.View(ctx, &sandboxapi.ShellViewRequest{Id: "newx-probe"})
			return err
		}},
		{http.MethodPost, "/v1/shell/wait", map[string]any{"id": "newx-probe", "seconds": float64(1)}, func() error {
			_, err := client.Wait(ctx, &sandboxapi.ShellWaitRequest{Id: "newx-probe", Seconds: &one})
			return err
		}},
		{http.MethodPost, "/v1/shell/kill", map[string]any{"id": "newx-probe"}, func() error {
			_, err := client.Kill(ctx, &sandboxapi.ShellKillProcessRequest{Id: "newx-probe"})
			return err
		}},
		{http.MethodDelete, "/v1/shell/sessions/newx-probe", map[string]any{}, func() error { return client.Cleanup(ctx, "newx-probe") }},
		{http.MethodPost, "/v1/file/read", map[string]any{"file": "/tmp/probe.txt", "start_line": float64(1)}, func() error {
			_, err := client.Read(ctx, &sandboxapi.FileReadRequest{File: "/tmp/probe.txt", StartLine: &one})
			return err
		}},
		{http.MethodPost, "/v1/file/write", map[string]any{"file": "/tmp/probe.txt", "content": "marker", "encoding": "utf-8"}, func() error {
			_, err := client.Write(ctx, &sandboxapi.FileWriteRequest{File: "/tmp/probe.txt", Content: "marker", Encoding: &utf8})
			return err
		}},
		{http.MethodPost, "/v1/file/list", map[string]any{"path": "/tmp", "max_depth": float64(2)}, func() error {
			_, err := client.List(ctx, &sandboxapi.FileListRequest{Path: "/tmp", MaxDepth: &two})
			return err
		}},
		{http.MethodPost, "/v1/file/glob", map[string]any{"path": "/tmp", "pattern": "*.txt"}, func() error {
			_, err := client.Glob(ctx, &sandboxapi.FileGlobRequest{Path: "/tmp", Pattern: "*.txt"})
			return err
		}},
		{http.MethodPost, "/v1/file/grep", map[string]any{"path": "/tmp", "pattern": "marker"}, func() error {
			_, err := client.Grep(ctx, &sandboxapi.FileGrepRequest{Path: "/tmp", Pattern: "marker"})
			return err
		}},
		{http.MethodPost, "/v1/file/replace", map[string]any{"file": "/tmp/probe.txt", "old_str": "marker", "new_str": "replaced"}, func() error {
			_, err := client.Replace(ctx, &sandboxapi.FileReplaceRequest{File: "/tmp/probe.txt", OldStr: "marker", NewStr: "replaced"})
			return err
		}},
		{http.MethodGet, "/v1/file/download", map[string]any{}, func() error {
			_, err := client.Download(ctx, "/tmp/probe.txt")
			return err
		}},
	}
	for _, call := range calls {
		require.NoError(t, call.call())
	}
	require.Len(t, observed, len(calls))
	for index, expected := range calls {
		require.Equal(t, expected.method, observed[index].method)
		require.Equal(t, expected.path, observed[index].path)
		require.Equal(t, "Bearer temporary-jwt", observed[index].authorization)
		require.NotContains(t, observed[index].path, "temporary-jwt")
		require.NotContains(t, observed[index].query, "temporary-jwt")
		require.Equal(t, expected.body, observed[index].body)
		if expected.path == "/v1/file/download" {
			require.Equal(t, "path=%2Ftmp%2Fprobe.txt", observed[index].query)
		}
	}
}

func TestUpstreamClientPropagatesContextDeadline(t *testing.T) {
	transport := &deadlineTransport{}
	client := newTestUpstreamClient(t, "http://127.0.0.1:8080", &http.Client{Timeout: time.Second, Transport: transport})
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	t.Cleanup(cancel)
	_, err := client.Exec(ctx, &sandboxapi.ShellExecRequest{Command: "pwd"})
	require.NoError(t, err)
	require.True(t, transport.sawDeadline)
}

func TestUpstreamClientDoesNotRetryAndSanitizesUpstreamErrors(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte("upstream-secret-body"))
	}))
	t.Cleanup(server.Close)
	client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 2 * time.Second})

	_, err := client.Write(context.Background(), &sandboxapi.FileWriteRequest{File: "/tmp/probe", Content: "value"})
	require.Equal(t, int32(1), attempts.Load())
	require.Equal(t, ReasonUpstreamServerError, ReasonCode(err))
	require.NotContains(t, err.Error(), "upstream-secret-body")
	require.NotContains(t, fmt.Sprintf("%+v", err), "upstream-secret-body")
}

func TestUpstreamClientDoesNotRetryTimeout(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		time.Sleep(75 * time.Millisecond)
		writer.WriteHeader(http.StatusGatewayTimeout)
	}))
	t.Cleanup(server.Close)
	client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 25 * time.Millisecond})

	_, err := client.Exec(context.Background(), &sandboxapi.ShellExecRequest{Command: "sleep 30"})
	require.Equal(t, int32(1), attempts.Load())
	require.Equal(t, ReasonUpstreamTimeout, ReasonCode(err))
}

func TestUpstreamClientRawHealthRequiresExactPingContract(t *testing.T) {
	var (
		method        string
		path          string
		authorization string
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		method = request.Method
		path = request.URL.Path
		authorization = request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = writer.Write([]byte("pong"))
	}))
	t.Cleanup(server.Close)

	client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 2 * time.Second})
	require.NoError(t, client.Health(context.Background()))
	require.Equal(t, http.MethodGet, method)
	require.Equal(t, "/v1/ping", path)
	require.Equal(t, "Bearer temporary-jwt", authorization)
}

func TestUpstreamClientRawHealthRejectsMalformedResponse(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		contentType string
		body        string
	}{
		{name: "wrong status", statusCode: http.StatusCreated, contentType: "text/plain; charset=utf-8", body: "pong"},
		{name: "wrong media type", statusCode: http.StatusOK, contentType: "application/json", body: `{"status":"ok"}`},
		{name: "wrong body", statusCode: http.StatusOK, contentType: "text/plain", body: "ok"},
		{name: "trailing newline", statusCode: http.StatusOK, contentType: "text/plain; charset=utf-8", body: "pong\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				writer.WriteHeader(test.statusCode)
				_, _ = writer.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)

			client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 2 * time.Second})
			err := client.Health(context.Background())
			require.Equal(t, ReasonUpstreamMalformedResponse, ReasonCode(err))
			require.NotContains(t, err.Error(), test.body)
		})
	}
}

func TestUpstreamClientListSessionsRequiresSuccessfulCompleteShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, http.MethodGet, request.Method)
		require.Equal(t, "/v1/shell/sessions", request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"data":{"sessions":{"newx-generation-0123456789abcdef0123456789abcdef":{"working_dir":"/mnt/user-data","created_at":"2026-08-13T00:00:00Z","last_used_at":"2026-08-13T00:00:00Z","age_seconds":1,"status":"idle"}}}}`))
	}))
	t.Cleanup(server.Close)

	client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 2 * time.Second})
	response, err := client.ListSessions(context.Background())
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, response.Data)
	require.Contains(t, response.Data.Sessions, "newx-generation-0123456789abcdef0123456789abcdef")
}

func TestUpstreamClientListSessionsMalformedShapeIsUnknownNotMissing(t *testing.T) {
	for _, body := range []string{
		`{"success":true}`,
		`{"success":true,"data":{}}`,
		`{"success":false,"data":{"sessions":{}}}`,
		`{"success":true,"data":{"sessions":{"candidate":null}}}`,
	} {
		body := body
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(body))
			}))
			t.Cleanup(server.Close)

			client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 2 * time.Second})
			response, err := client.ListSessions(context.Background())
			require.Nil(t, response)
			require.Equal(t, ReasonUpstreamMalformedResponse, ReasonCode(err))
			require.NotEqual(t, ReasonUpstreamNotFound, ReasonCode(err))
			require.NotContains(t, err.Error(), body)
		})
	}
}

func TestUpstreamClientOnlyMapsStableNotFoundToMissing(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		reasonCode string
	}{
		{name: "not found", statusCode: http.StatusNotFound, body: `{"detail":"secret missing id"}`, reasonCode: ReasonUpstreamNotFound},
		{name: "server error", statusCode: http.StatusInternalServerError, body: "secret failure", reasonCode: ReasonUpstreamServerError},
		{name: "decode error", statusCode: http.StatusOK, body: "not-json", reasonCode: ReasonUpstreamUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.statusCode)
				_, _ = writer.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)

			client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 2 * time.Second})
			response, err := client.ListSessions(context.Background())
			require.Nil(t, response)
			require.Equal(t, test.reasonCode, ReasonCode(err))
			require.NotContains(t, err.Error(), test.body)
			if test.statusCode != http.StatusNotFound {
				require.NotEqual(t, ReasonUpstreamNotFound, ReasonCode(err))
			}
		})
	}
}

func TestUpstreamClientShellLifecycleOnlyStableNotFoundIsMissing(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		method     string
		statusCode int
		call       func(context.Context, *UpstreamClient) error
		reasonCode string
	}{
		{
			name: "create server error", path: "/v1/shell/sessions/create", method: http.MethodPost,
			statusCode: http.StatusInternalServerError, reasonCode: ReasonUpstreamServerError,
			call: func(ctx context.Context, client *UpstreamClient) error {
				id, execDir := "newx-generation-0123456789abcdef0123456789abcdef", "/mnt/user-data"
				_, err := client.Create(ctx, &sandboxapi.ShellCreateSessionRequest{Id: &id, ExecDir: &execDir})
				return err
			},
		},
		{
			name: "view not found", path: "/v1/shell/view", method: http.MethodPost,
			statusCode: http.StatusNotFound, reasonCode: ReasonUpstreamNotFound,
			call: func(ctx context.Context, client *UpstreamClient) error {
				_, err := client.View(ctx, &sandboxapi.ShellViewRequest{Id: "newx-generation-0123456789abcdef0123456789abcdef"})
				return err
			},
		},
		{
			name: "view server error", path: "/v1/shell/view", method: http.MethodPost,
			statusCode: http.StatusBadGateway, reasonCode: ReasonUpstreamServerError,
			call: func(ctx context.Context, client *UpstreamClient) error {
				_, err := client.View(ctx, &sandboxapi.ShellViewRequest{Id: "newx-generation-0123456789abcdef0123456789abcdef"})
				return err
			},
		},
		{
			name: "cleanup not found", path: "/v1/shell/sessions/newx-generation-0123456789abcdef0123456789abcdef", method: http.MethodDelete,
			statusCode: http.StatusNotFound, reasonCode: ReasonUpstreamNotFound,
			call: func(ctx context.Context, client *UpstreamClient) error {
				return client.Cleanup(ctx, "newx-generation-0123456789abcdef0123456789abcdef")
			},
		},
		{
			name: "cleanup server error", path: "/v1/shell/sessions/newx-generation-0123456789abcdef0123456789abcdef", method: http.MethodDelete,
			statusCode: http.StatusServiceUnavailable, reasonCode: ReasonUpstreamServerError,
			call: func(ctx context.Context, client *UpstreamClient) error {
				return client.Cleanup(ctx, "newx-generation-0123456789abcdef0123456789abcdef")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				require.Equal(t, test.method, request.Method)
				require.Equal(t, test.path, request.URL.Path)
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(test.statusCode)
				_, _ = writer.Write([]byte(`{"detail":"secret upstream lifecycle body"}`))
			}))
			t.Cleanup(server.Close)

			client := newTestUpstreamClient(t, server.URL, &http.Client{Timeout: 2 * time.Second})
			err := test.call(context.Background(), client)
			require.Equal(t, test.reasonCode, ReasonCode(err))
			require.NotContains(t, err.Error(), "secret")
			if test.statusCode != http.StatusNotFound {
				require.NotEqual(t, ReasonUpstreamNotFound, ReasonCode(err))
			}
		})
	}
}

func TestUpstreamClientHealthTimeoutAndTransportErrorsAreUnknown(t *testing.T) {
	tests := []struct {
		name       string
		transport  http.RoundTripper
		reasonCode string
	}{
		{
			name: "deadline", reasonCode: ReasonUpstreamTimeout,
			transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				<-request.Context().Done()
				return nil, request.Context().Err()
			}),
		},
		{
			name: "transport", reasonCode: ReasonUpstreamUnavailable,
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("secret transport failure")
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestUpstreamClient(t, "http://127.0.0.1:8080", &http.Client{Timeout: 25 * time.Millisecond, Transport: test.transport})
			err := client.Health(context.Background())
			require.Equal(t, test.reasonCode, ReasonCode(err))
			require.NotEqual(t, ReasonUpstreamNotFound, ReasonCode(err))
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestUpstreamClientDisablesRedirectsAndConfiguredProxy(t *testing.T) {
	t.Run("redirect", func(t *testing.T) {
		var redirected atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			redirected.Add(1)
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte("pong"))
		}))
		t.Cleanup(target.Close)
		origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, target.URL+"/v1/ping", http.StatusFound)
		}))
		t.Cleanup(origin.Close)

		client := newTestUpstreamClient(t, origin.URL, &http.Client{Timeout: 2 * time.Second})
		err := client.Health(context.Background())
		require.Equal(t, ReasonUpstreamRejected, ReasonCode(err))
		require.Zero(t, redirected.Load())
	})

	t.Run("proxy", func(t *testing.T) {
		var proxied atomic.Int32
		proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			proxied.Add(1)
			writer.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(proxy.Close)
		proxyURL, err := url.Parse(proxy.URL)
		require.NoError(t, err)
		origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = writer.Write([]byte("pong"))
		}))
		t.Cleanup(origin.Close)

		client := newTestUpstreamClient(t, origin.URL, &http.Client{
			Timeout:   2 * time.Second,
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		})
		require.NoError(t, client.Health(context.Background()))
		require.Zero(t, proxied.Load())
	})
}

func TestUpstreamClientRejectsOversizedResponseWithoutLeakingBody(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"text/plain"}},
			Body:          io.NopCloser(strings.NewReader("secret oversized body")),
			ContentLength: maxUpstreamResponseBytes + 1,
			Request:       request,
		}, nil
	})
	client := newTestUpstreamClient(t, "http://127.0.0.1:8080", &http.Client{Timeout: time.Second, Transport: transport})
	err := client.Health(context.Background())
	require.Equal(t, ReasonUpstreamResponseTooLarge, ReasonCode(err))
	require.NotContains(t, err.Error(), "secret oversized body")
}

func newTestUpstreamClient(t *testing.T, baseURL string, httpClient *http.Client) *UpstreamClient {
	t.Helper()
	client, err := NewUpstreamClient(UpstreamClientConfig{BaseURL: baseURL, BearerJWT: "temporary-jwt", HTTPClient: httpClient})
	require.NoError(t, err)
	return client
}

func ptr[T any](value T) *T { return &value }

type deadlineTransport struct{ sawDeadline bool }

func (transport *deadlineTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	_, transport.sawDeadline = request.Context().Deadline()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":{}}`)),
		Request:    request,
	}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
