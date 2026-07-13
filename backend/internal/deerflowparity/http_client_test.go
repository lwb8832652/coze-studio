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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSafeHTTPClientRejectsNonLoopbackByDefault(t *testing.T) {
	t.Parallel()

	_, err := newSafeHTTPClient("https://example.com", ClientOptions{Timeout: time.Second})
	require.ErrorContains(t, err, "loopback")
}

func TestSafeHTTPClientRetainsCookiesAndUsesBoundedJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/login":
			http.SetCookie(writer, &http.Cookie{Name: "session", Value: "private", Path: "/"})
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"ok":true}`))
		case "/me":
			cookie, err := request.Cookie("session")
			require.NoError(t, err)
			require.Equal(t, "private", cookie.Value)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":"safe"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)

	client, err := newSafeHTTPClient(server.URL, ClientOptions{Timeout: time.Second, MaxResponseBytes: 1024})
	require.NoError(t, err)
	require.NoError(t, client.doJSON(context.Background(), http.MethodPost, "login", "/login", map[string]any{"email": "user@example.com"}, nil, nil))

	var response map[string]any
	require.NoError(t, client.doJSON(context.Background(), http.MethodGet, "me", "/me", nil, nil, &response))
	require.Equal(t, "safe", response["id"])
}

func TestSafeHTTPClientErrorsNeverContainResponseBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"password":"SECRET_RESPONSE_BODY"}`))
	}))
	t.Cleanup(server.Close)

	client, err := newSafeHTTPClient(server.URL, ClientOptions{Timeout: time.Second, MaxResponseBytes: 64})
	require.NoError(t, err)
	err = client.doJSON(context.Background(), http.MethodGet, "safe_endpoint", "/", nil, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "safe_endpoint")
	require.Contains(t, err.Error(), "500")
	require.NotContains(t, err.Error(), "SECRET_RESPONSE_BODY")
}

func TestSafeHTTPClientRejectsOversizedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(strings.Repeat("x", 128)))
	}))
	t.Cleanup(server.Close)

	client, err := newSafeHTTPClient(server.URL, ClientOptions{Timeout: time.Second, MaxResponseBytes: 32})
	require.NoError(t, err)
	err = client.doJSON(context.Background(), http.MethodGet, "bounded", "/", nil, nil, nil)
	require.ErrorContains(t, err, "response exceeds")
}

func TestSafeHTTPClientRejectsCrossOriginRedirectBeforeSendingCredentials(t *testing.T) {
	t.Parallel()

	var targetReached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		targetReached.Store(true)
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Location", target.URL+"?token=REDIRECT_SECRET")
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)

	client, err := newSafeHTTPClient(source.URL, ClientOptions{Timeout: time.Second})
	require.NoError(t, err)
	err = client.doForm(
		context.Background(),
		"login",
		"/",
		map[string][]string{"password": {"private-password"}},
		nil,
		nil,
	)
	require.ErrorContains(t, err, "cross-origin redirect")
	require.NotContains(t, err.Error(), "REDIRECT_SECRET")
	require.False(t, targetReached.Load())
}
