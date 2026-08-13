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

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
	"github.com/stretchr/testify/require"
)

func TestUpstreamConfigFromEnvironmentAllowsOfficialUnauthenticatedMode(t *testing.T) {
	t.Setenv("NEWX_AIO_BASE_URL", "http://127.0.0.1:8080")
	t.Setenv("NEWX_AIO_JWT", "")

	config, err := upstreamConfigFromEnvironment()
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080", config.BaseURL)
	require.Empty(t, config.BearerJWT)
	require.Equal(t, 30*time.Second, config.HTTPClient.Timeout)
}

func TestUpstreamConfigFromEnvironmentRequiresBaseURL(t *testing.T) {
	t.Setenv("NEWX_AIO_BASE_URL", "")
	t.Setenv("NEWX_AIO_JWT", "ignored")

	_, err := upstreamConfigFromEnvironment()
	require.Error(t, err)
}

func TestObservedAIOVersionComesFromEnvironment(t *testing.T) {
	t.Setenv("NEWX_AIO_VERSION", "observed-latest-version")

	require.Equal(t, "observed-latest-version", observedAIOVersion())
}

func TestSafeProbeFailureReportsOnlyStageAndStableReason(t *testing.T) {
	err := newProbeFailure("file_read", errors.New("upstream-secret-body token=secret"))

	require.Equal(
		t,
		"AIO compatibility probe failed: stage=file_read reason=AIO_PROBE_CONTRACT_FAILED",
		safeProbeFailure(err),
	)
	require.NotContains(t, safeProbeFailure(err), "upstream-secret-body")
	require.NotContains(t, safeProbeFailure(err), "token=secret")
}

func TestCancellationProbeWaitsBeforeKillAndValidatesTerminalStates(t *testing.T) {
	client := &recordingCancellationClient{}

	err := probeCancellation(context.Background(), client)

	require.NoError(t, err)
	require.Equal(t, []string{"exec", "view", "wait", "kill"}, client.calls)
}

func TestCleanupProbeRequiresKilledSessionGoneAndLiveSessionCleanupSuccess(t *testing.T) {
	client, calls := newCleanupTestClient(t, false)

	err := probeCleanup(context.Background(), client)

	require.NoError(t, err)
	require.Equal(t, []string{"/v1/shell/sessions/newx-probe-a", "/v1/shell/sessions/newx-probe-b"}, *calls)
}

func TestCleanupProbeAcceptsKilledSessionCleanupSuccess(t *testing.T) {
	client, calls := newCleanupTestClientWithStatuses(t, http.StatusOK, http.StatusOK)

	err := probeCleanup(context.Background(), client)

	require.NoError(t, err)
	require.Equal(t, []string{"/v1/shell/sessions/newx-probe-a", "/v1/shell/sessions/newx-probe-b"}, *calls)
}

func TestCleanupProbeDoesNotAcceptLiveSessionNotFound(t *testing.T) {
	client, _ := newCleanupTestClient(t, true)

	err := probeCleanup(context.Background(), client)

	require.Equal(t, "AIO compatibility probe failed: stage=shell_session_cleanup reason=AIO_UPSTREAM_NOT_FOUND", safeProbeFailure(err))
}

type recordingCancellationClient struct {
	calls []string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func newCleanupTestClient(t *testing.T, liveSessionNotFound bool) (*aio.UpstreamClient, *[]string) {
	t.Helper()
	bStatus := http.StatusOK
	if liveSessionNotFound {
		bStatus = http.StatusNotFound
	}
	return newCleanupTestClientWithStatuses(t, http.StatusNotFound, bStatus)
}

func newCleanupTestClientWithStatuses(t *testing.T, aStatus, bStatus int) (*aio.UpstreamClient, *[]string) {
	t.Helper()
	calls := []string{}
	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls = append(calls, request.URL.Path)
			status := bStatus
			body := `{"success":true,"data":{}}`
			if strings.HasSuffix(request.URL.Path, "newx-probe-a") {
				status = aStatus
			}
			if status != http.StatusOK {
				body = `{"message":"not found"}`
			}
			return &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    request,
			}, nil
		}),
	}
	client, err := aio.NewUpstreamClient(aio.UpstreamClientConfig{
		BaseURL:    "http://127.0.0.1:8080",
		HTTPClient: httpClient,
	})
	require.NoError(t, err)
	return client, &calls
}

func (client *recordingCancellationClient) Exec(context.Context, *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
	client.calls = append(client.calls, "exec")
	return &sandboxapi.ResponseShellCommandResult{Data: &sandboxapi.ShellCommandResult{Status: sandboxapi.BashCommandStatusRunning}}, nil
}

func (client *recordingCancellationClient) View(context.Context, *sandboxapi.ShellViewRequest) (*sandboxapi.ResponseShellViewResult, error) {
	client.calls = append(client.calls, "view")
	return &sandboxapi.ResponseShellViewResult{Data: &sandboxapi.ShellViewResult{Status: sandboxapi.BashCommandStatusRunning}}, nil
}

func (client *recordingCancellationClient) Wait(context.Context, *sandboxapi.ShellWaitRequest) (*sandboxapi.ResponseShellWaitResult, error) {
	client.calls = append(client.calls, "wait")
	return &sandboxapi.ResponseShellWaitResult{Data: &sandboxapi.ShellWaitResult{Status: sandboxapi.BashCommandStatusRunning}}, nil
}

func (client *recordingCancellationClient) Kill(context.Context, *sandboxapi.ShellKillProcessRequest) (*sandboxapi.ResponseShellKillResult, error) {
	client.calls = append(client.calls, "kill")
	return &sandboxapi.ResponseShellKillResult{Data: &sandboxapi.ShellKillResult{Status: sandboxapi.BashCommandStatusTerminated}}, nil
}
