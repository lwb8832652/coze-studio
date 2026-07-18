// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	testMCPStdioProviderKey = "mcp-stdio-provider"
	testMCPStdioSecret      = "projected-provider-secret"
)

func TestADKMCPRuntimeStdioProviderTransportBuildsCanonicalSecretSafeRequest(t *testing.T) {
	harness := newMCPStdioProviderHarness(t)
	audit := &recordingADKMCPRuntimeAuditRecorder{}
	transport := newMCPStdioProviderTransportForTest(t, harness.source, audit, 4096)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		validMCPStdioProviderCall(),
	)

	require.NoError(t, err)
	require.JSONEq(t, `{"content":[{"type":"text","text":"safe result"}],"is_error":false}`, result)
	require.Equal(t, domainsandbox.ScopeMCPStdio, harness.router.scope)
	require.Equal(t, testMCPStdioProviderKey, harness.router.providerKey)
	require.Len(t, harness.selection.requests, 1)
	request := harness.selection.requests[0]
	require.Equal(t, domainsandbox.ScopeMCPStdio, request.Scope)
	require.Equal(t, infrasandbox.WorkloadMCPStdio, request.WorkloadKind)
	require.Equal(t, "mcp/stdio/invoke", request.Entrypoint)
	require.Equal(t, "mcp_stdio:operation_test", request.IdempotencyKey)
	require.Equal(t, testMCPStdioSecret, request.Env["MCP_TOKEN"])
	require.Equal(t, []string{"MCP_TOKEN"}, request.Policy.AllowedEnvNames)
	require.Equal(t, []string{"npx"}, request.Policy.AllowedExecutables)
	require.Equal(t, time.Unix(2_100_000_000, 0).UTC().Add(30*time.Second), request.Deadline)
	require.NotContains(t, string(request.Stdin), testMCPStdioSecret)
	require.NotContains(t, string(request.Stdin), "/tmp")
	require.NotContains(t, string(request.Stdin), "/secret")

	var envelope infrasandbox.MCPStdioInvokeEnvelope
	require.NoError(t, json.Unmarshal(request.Stdin, &envelope))
	require.Equal(t, infrasandbox.MCPStdioInvokeSchemaV1, envelope.Schema)
	require.Equal(t, "operation_test", envelope.OperationID)
	require.Equal(t, "npx", envelope.Command)
	require.Equal(t, []string{"-y", "@example/provider-mcp"}, envelope.Args)
	require.Equal(t, "search-docs", envelope.ToolName)
	require.JSONEq(t, `{"query":"customer docs"}`, string(envelope.Arguments))
	require.Equal(t, "workspace/mcp_stdio/space-30/thread-10/run-20/server-100", envelope.WorkingDir)
	require.Equal(t, []string{"MCP_TOKEN"}, envelope.EnvNames)
	require.False(t, strings.HasPrefix(envelope.WorkingDir, "/"))
	require.Equal(t, 1, harness.selection.releaseCalls)
	require.NoError(t, harness.selection.releaseContextErr)

	require.NotEmpty(t, audit.records)
	for _, record := range audit.records {
		encoded, marshalErr := json.Marshal(record)
		require.NoError(t, marshalErr)
		for _, prohibited := range []string{
			"command", "args", "arguments", "env", "stdout", "stderr", "endpoint",
			"execution_id", "host_path", testMCPStdioSecret, "customer docs",
		} {
			require.NotContains(t, strings.ToLower(string(encoded)), strings.ToLower(prohibited))
		}
	}
}

func TestADKMCPRuntimeStdioProviderTransportDefaultSelectionFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*mcpStdioProviderHarness)
		wantCode  string
	}{
		{
			name: "binding unpublished",
			configure: func(h *mcpStdioProviderHarness) {
				h.source.ok = false
			},
			wantCode: domainsandbox.ErrCodeUnavailable,
		},
		{
			name: "default missing",
			configure: func(h *mcpStdioProviderHarness) {
				h.defaults.err = domainsandbox.ErrDefaultMissing
			},
			wantCode: domainsandbox.ErrCodeDefaultMissing,
		},
		{
			name: "default provider missing",
			configure: func(h *mcpStdioProviderHarness) {
				h.providers.err = domainsandbox.ErrProviderNotFound
			},
			wantCode: domainsandbox.ErrCodeProviderNotFound,
		},
		{
			name: "disabled",
			configure: func(h *mcpStdioProviderHarness) {
				h.router.err = domainsandbox.ErrProviderDisabled
			},
			wantCode: domainsandbox.ErrCodeProviderDisabled,
		},
		{
			name: "unhealthy",
			configure: func(h *mcpStdioProviderHarness) {
				h.router.err = domainsandbox.ErrProviderUnhealthy
			},
			wantCode: domainsandbox.ErrCodeProviderUnhealthy,
		},
		{
			name: "unsupported scope",
			configure: func(h *mcpStdioProviderHarness) {
				h.router.err = domainsandbox.ErrScopeUnsupported
			},
			wantCode: domainsandbox.ErrCodeScopeUnsupported,
		},
		{
			name: "capacity",
			configure: func(h *mcpStdioProviderHarness) {
				h.router.err = domainsandbox.ErrCapacityExhausted
			},
			wantCode: domainsandbox.ErrCodeCapacityExhausted,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newMCPStdioProviderHarness(t)
			test.configure(harness)
			audit := &recordingADKMCPRuntimeAuditRecorder{}
			transport := newMCPStdioProviderTransportForTest(t, harness.source, audit, 4096)

			result, err := transport.InvokeADKMCPRuntimeTransport(
				context.Background(),
				validMCPStdioProviderCall(),
			)

			require.Error(t, err)
			require.Empty(t, result)
			require.Contains(t, err.Error(), "mcp runtime stdio provider transport failed")
			assertMCPStdioProviderErrorRedacted(t, err)
			require.Empty(t, harness.selection.requests)
			require.Equal(t, test.wantCode, lastMCPProviderAuditCode(audit.records))
		})
	}
}

func TestADKMCPRuntimeStdioProviderTransportRequiresProviderAndMCPAllowlists(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ADKMCPRuntimeTransportCall, *mcpStdioProviderHarness)
	}{
		{
			name: "absolute executable",
			mutate: func(call *ADKMCPRuntimeTransportCall, _ *mcpStdioProviderHarness) {
				call.Server.Config = `{"command":"/usr/bin/npx","args":["-y","@example/provider-mcp"]}`
			},
		},
		{
			name: "mcp command allowlist",
			mutate: func(call *ADKMCPRuntimeTransportCall, _ *mcpStdioProviderHarness) {
				call.Server.Config = `{"command":"unknown-mcp","args":[]}`
			},
		},
		{
			name: "provider executable allowlist",
			mutate: func(_ *ADKMCPRuntimeTransportCall, harness *mcpStdioProviderHarness) {
				harness.selection.policy.AllowedExecutables = []string{"uvx"}
			},
		},
		{
			name: "provider env allowlist",
			mutate: func(_ *ADKMCPRuntimeTransportCall, harness *mcpStdioProviderHarness) {
				harness.selection.policy.AllowedEnvNames = []string{"OTHER_TOKEN"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newMCPStdioProviderHarness(t)
			call := validMCPStdioProviderCall()
			test.mutate(&call, harness)
			transport := newMCPStdioProviderTransportForTest(t, harness.source, nil, 4096)

			result, err := transport.InvokeADKMCPRuntimeTransport(context.Background(), call)

			require.Error(t, err)
			require.Empty(t, result)
			assertMCPStdioProviderErrorRedacted(t, err)
			require.Empty(t, harness.selection.requests)
		})
	}
}

func TestADKMCPRuntimeStdioProviderTransportAcceptsOnlyTerminalReviewedResults(t *testing.T) {
	exitZero := 0
	exitSeven := 7
	tests := []struct {
		name       string
		result     infrasandbox.ExecuteResult
		maxResult  int
		wantResult string
		wantErr    bool
		wantCancel int
	}{
		{
			name: "success",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_success", Status: infrasandbox.ExecutionStatusSucceeded,
				ExitCode: &exitZero, Stdout: validMCPStdioProviderResult(false),
			},
			maxResult:  4096,
			wantResult: `{"content":[{"type":"text","text":"safe result"}],"is_error":false}`,
		},
		{
			name: "tool error",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_tool_error", Status: infrasandbox.ExecutionStatusSucceeded,
				ExitCode: &exitZero, Stdout: validMCPStdioProviderResult(true),
			},
			maxResult:  4096,
			wantResult: `{"content":[{"type":"text","text":"safe result"}],"is_error":true}`,
		},
		{
			name: "nonzero",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_failed", Status: infrasandbox.ExecutionStatusFailed,
				ExitCode: &exitSeven, Stdout: testMCPStdioSecret, Stderr: testMCPStdioSecret,
			},
			maxResult: 4096, wantErr: true,
		},
		{
			name: "malformed",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_malformed", Status: infrasandbox.ExecutionStatusSucceeded,
				ExitCode: &exitZero, Stdout: `{"schema":`,
			},
			maxResult: 4096, wantErr: true,
		},
		{
			name: "unknown envelope",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_unknown", Status: infrasandbox.ExecutionStatusSucceeded,
				ExitCode: &exitZero, Stdout: `{"schema":"unknown","content":[],"is_error":false}`,
			},
			maxResult: 4096, wantErr: true,
		},
		{
			name: "oversized",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_oversized", Status: infrasandbox.ExecutionStatusSucceeded,
				ExitCode: &exitZero, Stdout: validMCPStdioProviderResultWithText(strings.Repeat("x", 512)),
			},
			maxResult: 128, wantErr: true,
		},
		{
			name: "accepted",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_accepted", Status: infrasandbox.ExecutionStatusAccepted,
			},
			maxResult: 4096, wantErr: true, wantCancel: 1,
		},
		{
			name: "running",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_running", Status: infrasandbox.ExecutionStatusRunning,
			},
			maxResult: 4096, wantErr: true, wantCancel: 1,
		},
		{
			name: "canceled",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_canceled", Status: infrasandbox.ExecutionStatusCanceled,
			},
			maxResult: 4096, wantErr: true,
		},
		{
			name: "timed out",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_timed_out", Status: infrasandbox.ExecutionStatusTimedOut,
			},
			maxResult: 4096, wantErr: true,
		},
		{
			name: "unknown status",
			result: infrasandbox.ExecuteResult{
				ExecutionID: "execution_unknown_status", Status: "mystery",
			},
			maxResult: 4096, wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newMCPStdioProviderHarness(t)
			harness.selection.results = []infrasandbox.ExecuteResult{test.result}
			transport := newMCPStdioProviderTransportForTest(t, harness.source, nil, test.maxResult)

			result, err := transport.InvokeADKMCPRuntimeTransport(
				context.Background(),
				validMCPStdioProviderCall(),
			)

			if test.wantErr {
				require.Error(t, err)
				require.Empty(t, result)
				assertMCPStdioProviderErrorRedacted(t, err)
			} else {
				require.NoError(t, err)
				require.JSONEq(t, test.wantResult, result)
			}
			require.Equal(t, test.wantCancel, harness.selection.cancelCalls)
			require.Equal(t, 1, harness.selection.releaseCalls)
		})
	}
}

func TestADKMCPRuntimeStdioProviderTransportReconcilesOnlyUncertainCanonicalRequest(t *testing.T) {
	exitZero := 0
	tests := []struct {
		name      string
		firstErr  error
		wantCalls int
	}{
		{name: "provider uncertain", firstErr: domainsandbox.ErrUnavailable, wantCalls: 2},
		{name: "provider timeout", firstErr: context.DeadlineExceeded, wantCalls: 2},
		{name: "caller canceled after submission", firstErr: context.Canceled, wantCalls: 2},
		{name: "definite capacity denial", firstErr: domainsandbox.ErrCapacityExhausted, wantCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newMCPStdioProviderHarness(t)
			harness.selection.errs = []error{test.firstErr, nil}
			harness.selection.results = []infrasandbox.ExecuteResult{
				{},
				{
					ExecutionID: "execution_reconciled", Status: infrasandbox.ExecutionStatusSucceeded,
					ExitCode: &exitZero, Stdout: validMCPStdioProviderResult(false),
				},
			}
			transport := newMCPStdioProviderTransportForTest(t, harness.source, nil, 4096)

			result, err := transport.InvokeADKMCPRuntimeTransport(
				context.Background(),
				validMCPStdioProviderCall(),
			)

			require.Len(t, harness.selection.requests, test.wantCalls)
			if test.wantCalls == 2 {
				require.NoError(t, err)
				require.NotEmpty(t, result)
				require.True(t, reflect.DeepEqual(
					harness.selection.requests[0],
					harness.selection.requests[1],
				))
				require.Equal(
					t,
					harness.selection.requests[0].IdempotencyKey,
					harness.selection.requests[1].IdempotencyKey,
				)
			} else {
				require.Error(t, err)
				require.Empty(t, result)
			}
			require.Equal(t, 1, harness.selection.releaseCalls)
		})
	}
}

func TestADKMCPRuntimeStdioProviderTransportUsesIndependentBoundedCleanupContext(t *testing.T) {
	harness := newMCPStdioProviderHarness(t)
	callerCtx, cancelCaller := context.WithCancel(context.Background())
	harness.selection.executeHook = func(context.Context, infrasandbox.ExecuteRequest) {
		cancelCaller()
	}
	harness.selection.releaseHook = func(ctx context.Context) error {
		harness.selection.releaseContextErr = ctx.Err()
		<-ctx.Done()
		return ctx.Err()
	}
	audit := &recordingADKMCPRuntimeAuditRecorder{}
	transport := newMCPStdioProviderTransportForTest(t, harness.source, audit, 4096)

	started := time.Now()
	result, err := transport.InvokeADKMCPRuntimeTransport(
		callerCtx,
		validMCPStdioProviderCall(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Less(t, time.Since(started), time.Second)
	require.Equal(t, 1, harness.selection.releaseCalls)
	require.NoError(t, harness.selection.releaseContextErr)
	require.Equal(t, "cleanup_timeout", lastMCPProviderAuditCode(audit.records))
}

func TestADKMCPRuntimeStdioProviderTransportCancelCompletesBeforeRelease(t *testing.T) {
	harness := newMCPStdioProviderHarness(t)
	harness.selection.results = []infrasandbox.ExecuteResult{{
		ExecutionID: "execution_running",
		Status:      infrasandbox.ExecutionStatusRunning,
	}}
	transport := newMCPStdioProviderTransportForTest(t, harness.source, nil, 4096)

	var wait sync.WaitGroup
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = transport.InvokeADKMCPRuntimeTransport(
				context.Background(),
				validMCPStdioProviderCall(),
			)
		}()
	}
	wait.Wait()

	harness.selection.mu.Lock()
	defer harness.selection.mu.Unlock()
	require.Equal(t, harness.selection.cancelCalls, harness.selection.releaseCalls)
	pendingCancels := 0
	for _, event := range harness.selection.order {
		switch event {
		case "cancel":
			pendingCancels++
		case "release":
			require.Positive(t, pendingCancels)
			pendingCancels--
		default:
			require.Failf(t, "unexpected lifecycle event", "event=%s", event)
		}
	}
	require.Zero(t, pendingCancels)
}

func TestADKMCPRuntimeStdioProviderTransportUsesLatestDynamicBinding(t *testing.T) {
	first := newMCPStdioProviderHarness(t)
	second := newMCPStdioProviderHarness(t)
	second.providers.provider.ProviderKey = "mcp-stdio-provider-replaced"
	second.router.providerKey = ""
	source := &mutableMCPStdioBindingSource{binding: first.source.binding, ok: true}
	transport := newMCPStdioProviderTransportForTest(t, source, nil, 4096)

	_, err := transport.InvokeADKMCPRuntimeTransport(context.Background(), validMCPStdioProviderCall())
	require.NoError(t, err)
	source.set(second.source.binding, true)
	_, err = transport.InvokeADKMCPRuntimeTransport(context.Background(), validMCPStdioProviderCall())
	require.NoError(t, err)
	source.set(ADKMCPRuntimeSandboxBinding{}, false)
	_, err = transport.InvokeADKMCPRuntimeTransport(context.Background(), validMCPStdioProviderCall())
	require.Error(t, err)

	require.Equal(t, 1, first.selection.releaseCalls)
	require.Equal(t, 1, second.selection.releaseCalls)
	require.Equal(t, "mcp-stdio-provider-replaced", second.router.providerKey)
}

func TestADKMCPRuntimeStdioProviderTransportRealRouterToHTTPFake(t *testing.T) {
	exitZero := 0
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestPath = request.URL.Path
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"entrypoint":"mcp/stdio/invoke"`)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(fmt.Sprintf(
			`{"execution_id":"execution_http","status":"succeeded","exit_code":0,"stdout":%q}`,
			validMCPStdioProviderResult(false),
		)))
	}))
	defer server.Close()

	now := time.Now().UTC()
	provider := validMCPStdioRouterProvider(now)
	runtime := &mcpStdioHTTPRuntimeProvider{endpoint: server.URL, client: server.Client()}
	router, err := appsandbox.NewProviderRouter(
		mcpStdioProviderLookup{provider: provider},
		mcpStdioRuntimeFactory{runtime: runtime},
		&mcpStdioCapacityLimiter{expiresAt: now.Add(time.Minute).UnixMilli()},
		time.Minute,
	)
	require.NoError(t, err)
	defaults := &mcpStdioDefaultRepository{value: &domainsandbox.ProviderDefault{
		Scope: domainsandbox.ScopeMCPStdio, ProviderID: provider.ID, Version: 1,
	}}
	providers := &mcpStdioProviderRepository{provider: provider}
	source := &mutableMCPStdioBindingSource{
		ok: true,
		binding: ADKMCPRuntimeSandboxBinding{
			ProviderDefaults: defaults,
			Providers:        providers,
			Router:           mcpStdioRealRouterAdapter{router: router},
		},
	}
	transport := newMCPStdioProviderTransportForTest(t, source, nil, 4096)
	transport.now = time.Now

	result, err := transport.InvokeADKMCPRuntimeTransport(context.Background(), validMCPStdioProviderCall())

	require.NoError(t, err)
	require.JSONEq(t, `{"content":[{"type":"text","text":"safe result"}],"is_error":false}`, result)
	require.Equal(t, "/v1/executions", requestPath)
	require.Equal(t, 1, runtime.executeCalls)
	require.Equal(t, 1, runtime.closeCalls)
	require.Equal(t, exitZero, *runtime.lastResult.ExitCode)
}

func newMCPStdioProviderTransportForTest(
	t *testing.T,
	source ADKMCPRuntimeSandboxBindingSource,
	audit ADKMCPRuntimeAuditRecorder,
	maxResultBytes int,
) *ADKMCPRuntimeStdioProviderTransport {
	t.Helper()
	policy, err := mcpruntime.NewLogicalStdioPolicy(mcpruntime.LogicalStdioPolicyOptions{
		AllowedCommands:  []string{"npx"},
		NpxPackages:      []string{"@example/provider-mcp"},
		AllowedEnvKeys:   []string{"MCP_TOKEN"},
		MaxArgs:          4,
		MaxArgBytes:      256,
		MaxEnvVars:       1,
		MaxEnvValueBytes: 128,
	})
	require.NoError(t, err)
	transport, err := NewADKMCPRuntimeStdioProviderTransport(
		ADKMCPRuntimeStdioProviderTransportOptions{
			BindingSource:  source,
			Policy:         policy,
			AuditRecorder:  audit,
			MaxConfigBytes: 4096,
			MaxResultBytes: maxResultBytes,
			CancelTimeout:  50 * time.Millisecond,
			CleanupTimeout: 50 * time.Millisecond,
			OperationID: func(context.Context) (string, error) {
				return "operation_test", nil
			},
			Now: func() time.Time {
				return time.Unix(2_100_000_000, 0).UTC()
			},
		},
	)
	require.NoError(t, err)
	return transport
}

func validMCPStdioProviderCall() ADKMCPRuntimeTransportCall {
	call := validADKMCPRuntimeStdioCall()
	call.Arguments = `{"query":"customer docs"}`
	call.Server.Config = `{
		"command":"npx",
		"args":["-y","@example/provider-mcp"],
		"cwd":"/secret/host/workdir",
		"auth_env":{"MCP_TOKEN":"credentials.token"}
	}`
	call.Server.Auth = `{"credentials":{"token":"` + testMCPStdioSecret + `"}}`
	return call
}

func validMCPStdioProviderResult(isError bool) string {
	return validMCPStdioProviderResultWithTextAndError("safe result", isError)
}

func validMCPStdioProviderResultWithText(text string) string {
	return validMCPStdioProviderResultWithTextAndError(text, false)
}

func validMCPStdioProviderResultWithTextAndError(text string, isError bool) string {
	payload, _ := json.Marshal(map[string]any{
		"schema": infrasandbox.MCPStdioResultSchemaV1,
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
		"is_error": isError,
	})
	return string(payload)
}

func validMCPStdioProviderPolicy() domainsandbox.RuntimePolicy {
	return domainsandbox.RuntimePolicy{
		TimeoutSeconds:       30,
		MemoryLimitMB:        128,
		CPULimit:             1,
		MaxOutputBytes:       4096,
		MaxConcurrency:       4,
		AllowedEnvNames:      []string{"MCP_TOKEN"},
		VirtualReadPrefixes:  []string{"workspace"},
		VirtualWritePrefixes: []string{"workspace"},
		AllowedExecutables:   []string{"npx"},
	}
}

type mcpStdioProviderHarness struct {
	defaults  *mcpStdioDefaultRepository
	providers *mcpStdioProviderRepository
	router    *mcpStdioRouterStub
	selection *mcpStdioSelectionStub
	source    *mutableMCPStdioBindingSource
}

func newMCPStdioProviderHarness(t *testing.T) *mcpStdioProviderHarness {
	t.Helper()
	selection := &mcpStdioSelectionStub{
		policy: validMCPStdioProviderPolicy(),
		results: []infrasandbox.ExecuteResult{{
			ExecutionID: "execution_success",
			Status:      infrasandbox.ExecutionStatusSucceeded,
			ExitCode:    intPointerForMCPStdioProvider(0),
			Stdout:      validMCPStdioProviderResult(false),
		}},
	}
	defaults := &mcpStdioDefaultRepository{value: &domainsandbox.ProviderDefault{
		Scope: domainsandbox.ScopeMCPStdio, ProviderID: 501, Version: 1,
	}}
	providers := &mcpStdioProviderRepository{provider: &domainsandbox.Provider{
		ID: 501, ProviderKey: testMCPStdioProviderKey,
	}}
	router := &mcpStdioRouterStub{selection: selection}
	source := &mutableMCPStdioBindingSource{
		ok: true,
		binding: ADKMCPRuntimeSandboxBinding{
			ProviderDefaults: defaults,
			Providers:        providers,
			Router:           router,
		},
	}
	return &mcpStdioProviderHarness{
		defaults: defaults, providers: providers, router: router,
		selection: selection, source: source,
	}
}

type mutableMCPStdioBindingSource struct {
	mu      sync.RWMutex
	binding ADKMCPRuntimeSandboxBinding
	ok      bool
}

func (s *mutableMCPStdioBindingSource) LoadADKMCPRuntimeSandboxBinding() (ADKMCPRuntimeSandboxBinding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.binding, s.ok
}

func (s *mutableMCPStdioBindingSource) set(binding ADKMCPRuntimeSandboxBinding, ok bool) {
	s.mu.Lock()
	s.binding = binding
	s.ok = ok
	s.mu.Unlock()
}

type mcpStdioDefaultRepository struct {
	value *domainsandbox.ProviderDefault
	err   error
}

func (r *mcpStdioDefaultRepository) GetProviderDefault(context.Context, domainsandbox.Scope) (*domainsandbox.ProviderDefault, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.value == nil {
		return nil, domainsandbox.ErrDefaultMissing
	}
	copy := *r.value
	return &copy, nil
}

func (r *mcpStdioDefaultRepository) ListProviderDefaults(context.Context) ([]*domainsandbox.ProviderDefault, error) {
	if r.value == nil {
		return nil, nil
	}
	copy := *r.value
	return []*domainsandbox.ProviderDefault{&copy}, nil
}

func (r *mcpStdioDefaultRepository) SetProviderDefault(context.Context, domainsandbox.SetProviderDefaultInput) (*domainsandbox.ProviderDefault, error) {
	return nil, domainsandbox.ErrExecutionForbidden
}

type mcpStdioProviderRepository struct {
	provider *domainsandbox.Provider
	err      error
}

func (r *mcpStdioProviderRepository) GetProvider(context.Context, int64) (*domainsandbox.Provider, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.provider == nil {
		return nil, domainsandbox.ErrProviderNotFound
	}
	copy := *r.provider
	return &copy, nil
}

type mcpStdioRouterStub struct {
	providerKey string
	scope       domainsandbox.Scope
	selection   ADKMCPRuntimeSandboxSelection
	err         error
}

func (r *mcpStdioRouterStub) ResolveADKMCPRuntimeSandbox(
	_ context.Context,
	providerKey string,
	scope domainsandbox.Scope,
) (ADKMCPRuntimeSandboxSelection, error) {
	r.providerKey = providerKey
	r.scope = scope
	if r.err != nil {
		return nil, r.err
	}
	return r.selection, nil
}

type mcpStdioSelectionStub struct {
	mu                sync.Mutex
	policy            domainsandbox.RuntimePolicy
	results           []infrasandbox.ExecuteResult
	errs              []error
	requests          []infrasandbox.ExecuteRequest
	executeHook       func(context.Context, infrasandbox.ExecuteRequest)
	cancelHook        func(context.Context, string) error
	releaseHook       func(context.Context) error
	cancelCalls       int
	releaseCalls      int
	releaseContextErr error
	order             []string
}

func (s *mcpStdioSelectionStub) Status(context.Context) (infrasandbox.ExecuteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return infrasandbox.ExecuteResult{
		ExecutionID: "execution_running",
		Status:      infrasandbox.ExecutionStatusCanceled,
	}, nil
}

func (s *mcpStdioSelectionStub) ADKMCPRuntimeSandboxPolicy() domainsandbox.RuntimePolicy {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.policy
}

func (s *mcpStdioSelectionStub) Execute(
	ctx context.Context,
	request infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	s.mu.Lock()
	index := len(s.requests)
	s.requests = append(s.requests, request)
	var result infrasandbox.ExecuteResult
	if index < len(s.results) {
		result = s.results[index]
	} else if len(s.results) > 0 {
		result = s.results[len(s.results)-1]
	}
	var err error
	if index < len(s.errs) {
		err = s.errs[index]
	}
	hook := s.executeHook
	s.mu.Unlock()
	if hook != nil {
		hook(ctx, request)
	}
	return result, err
}

func (s *mcpStdioSelectionStub) Cancel(ctx context.Context, executionID string) error {
	s.mu.Lock()
	s.cancelCalls++
	s.order = append(s.order, "cancel")
	hook := s.cancelHook
	s.mu.Unlock()
	if hook != nil {
		return hook(ctx, executionID)
	}
	return nil
}

func (s *mcpStdioSelectionStub) Release(ctx context.Context) error {
	s.mu.Lock()
	s.releaseCalls++
	s.order = append(s.order, "release")
	s.releaseContextErr = ctx.Err()
	hook := s.releaseHook
	s.mu.Unlock()
	if hook != nil {
		return hook(ctx)
	}
	return nil
}

func lastMCPProviderAuditCode(records []ADKMCPRuntimeAuditRecord) string {
	if len(records) == 0 {
		return ""
	}
	return records[len(records)-1].ErrorCode
}

func assertMCPStdioProviderErrorRedacted(t *testing.T, err error) {
	t.Helper()
	require.NotNil(t, err)
	for _, prohibited := range []string{
		testMCPStdioSecret,
		"customer docs",
		"npx",
		"@example/provider-mcp",
		"/secret/host/workdir",
		"execution_",
		"mcp-stdio-provider",
	} {
		require.NotContains(t, err.Error(), prohibited)
	}
}

func intPointerForMCPStdioProvider(value int) *int {
	return &value
}

type mcpStdioProviderLookup struct {
	provider *domainsandbox.Provider
}

func (l mcpStdioProviderLookup) GetProviderByKey(
	ctx context.Context,
	key string,
) (*domainsandbox.Provider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.provider == nil || l.provider.ProviderKey != key {
		return nil, domainsandbox.ErrProviderNotFound
	}
	copy := *l.provider
	return &copy, nil
}

type mcpStdioRuntimeFactory struct {
	runtime infrasandbox.RuntimeProvider
}

func (f mcpStdioRuntimeFactory) ValidateConfig(context.Context, appsandbox.ProviderDescriptor) error {
	return nil
}

func (f mcpStdioRuntimeFactory) Build(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
	return f.runtime, nil
}

type mcpStdioCapacityLimiter struct {
	expiresAt int64
}

func (l *mcpStdioCapacityLimiter) Acquire(context.Context, string, string, string, int, time.Duration) (int64, error) {
	return l.expiresAt, nil
}

func (l *mcpStdioCapacityLimiter) Renew(context.Context, string, string, string, string, time.Duration) (int64, error) {
	return l.expiresAt, nil
}

func (l *mcpStdioCapacityLimiter) Release(context.Context, string, string, string) error {
	return nil
}

type mcpStdioRealRouterAdapter struct {
	router *appsandbox.ProviderRouter
}

func (a mcpStdioRealRouterAdapter) ResolveADKMCPRuntimeSandbox(
	ctx context.Context,
	providerKey string,
	scope domainsandbox.Scope,
) (ADKMCPRuntimeSandboxSelection, error) {
	request, err := appsandbox.NewResolveProviderRequest(providerKey, scope)
	if err != nil {
		return nil, err
	}
	selected, err := a.router.Resolve(ctx, request)
	if err != nil {
		return nil, err
	}
	return mcpStdioRealSelection{selected: selected}, nil
}

type mcpStdioRealSelection struct {
	selected *appsandbox.SelectedProvider
}

func (s mcpStdioRealSelection) ADKMCPRuntimeSandboxPolicy() domainsandbox.RuntimePolicy {
	return s.selected.Policy
}

func (s mcpStdioRealSelection) Execute(ctx context.Context, request infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return s.selected.Execute(ctx, request)
}

func (s mcpStdioRealSelection) Status(ctx context.Context) (infrasandbox.ExecuteResult, error) {
	return s.selected.Status(ctx)
}

func (s mcpStdioRealSelection) Cancel(ctx context.Context, executionID string) error {
	return s.selected.Cancel(ctx, executionID)
}

func (s mcpStdioRealSelection) Release(ctx context.Context) error {
	return s.selected.Release(ctx)
}

type mcpStdioHTTPRuntimeProvider struct {
	endpoint     string
	client       *http.Client
	executeCalls int
	closeCalls   int
	lastResult   infrasandbox.ExecuteResult
}

func (p *mcpStdioHTTPRuntimeProvider) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}

func (p *mcpStdioHTTPRuntimeProvider) Execute(
	ctx context.Context,
	request infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	p.executeCalls++
	body, err := json.Marshal(map[string]any{
		"schema":          infrasandbox.ExecuteSchemaV1,
		"scope":           request.Scope,
		"workload_kind":   request.WorkloadKind,
		"idempotency_key": request.IdempotencyKey,
		"entrypoint":      request.Entrypoint,
		"stdin":           request.Stdin,
	})
	if err != nil {
		return infrasandbox.ExecuteResult{}, err
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.endpoint+"/v1/executions",
		bytes.NewReader(body),
	)
	if err != nil {
		return infrasandbox.ExecuteResult{}, err
	}
	response, err := p.client.Do(httpRequest)
	if err != nil {
		return infrasandbox.ExecuteResult{}, err
	}
	defer response.Body.Close()
	var wire struct {
		ExecutionID string                       `json:"execution_id"`
		Status      infrasandbox.ExecutionStatus `json:"status"`
		ExitCode    *int                         `json:"exit_code"`
		Stdout      string                       `json:"stdout"`
	}
	if err := json.NewDecoder(response.Body).Decode(&wire); err != nil {
		return infrasandbox.ExecuteResult{}, err
	}
	p.lastResult = infrasandbox.ExecuteResult{
		ExecutionID: wire.ExecutionID,
		Status:      wire.Status,
		ExitCode:    wire.ExitCode,
		Stdout:      wire.Stdout,
	}
	return p.lastResult, nil
}

func (p *mcpStdioHTTPRuntimeProvider) Cancel(context.Context, string) error {
	return nil
}

func (p *mcpStdioHTTPRuntimeProvider) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.closeCalls++
	return nil
}

func validMCPStdioRouterProvider(now time.Time) *domainsandbox.Provider {
	return &domainsandbox.Provider{
		ID:          501,
		ProviderKey: testMCPStdioProviderKey,
		Name:        "MCP stdio provider",
		Type:        domainsandbox.ProviderTypeRemoteHTTP,
		Scopes:      []domainsandbox.Scope{domainsandbox.ScopeMCPStdio},
		Policy:      validMCPStdioProviderPolicy(),
		Status:      domainsandbox.ProviderStatusEnabled,
		Health: domainsandbox.HealthSnapshot{
			Status:       domainsandbox.HealthStatusHealthy,
			Capabilities: []domainsandbox.Scope{domainsandbox.ScopeMCPStdio},
			CheckedAt:    now,
		},
	}
}
