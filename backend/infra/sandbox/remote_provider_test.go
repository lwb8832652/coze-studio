// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestRemoteProviderExecuteUsesV1ProtocolAndReturnsBoundedProjection(t *testing.T) {
	t.Parallel()

	var capturedRequest *http.Request
	var capturedBody map[string]any
	transport := providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		capturedRequest = request.Clone(request.Context())
		if err := json.NewDecoder(request.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode execute request: %v", err)
		}
		return jsonResponse(request, http.StatusOK, `{
			"schema":"coze.sandbox.execute.v1",
			"execution_id":"exec_123",
			"status":"succeeded",
			"exit_code":0,
			"stdout":"ok",
			"stderr":"",
			"artifacts":[{"id":"artifact_1","name":"report.json","media_type":"application/json","size":12}]
		}`), nil
	})
	provider := mustRemoteProvider(t, transport)
	request := validExecuteRequest()
	result, err := provider.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if capturedRequest.Method != http.MethodPost || capturedRequest.URL.String() != "https://sandbox.example.test/v1/executions" {
		t.Fatalf("execute request = %s %s", capturedRequest.Method, capturedRequest.URL)
	}
	if got := capturedRequest.Header.Get("Authorization"); got != "Bearer synthetic-test-token" {
		t.Fatalf("authorization = %q", got)
	}
	if got := capturedRequest.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	if capturedBody["schema"] != ExecuteSchemaV1 || capturedBody["idempotency_key"] != request.IdempotencyKey {
		t.Fatalf("execute envelope = %#v", capturedBody)
	}
	if result.ExecutionID != "exec_123" || result.Status != ExecutionStatusSucceeded || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("execute result = %#v", result)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Name != "report.json" {
		t.Fatalf("artifact projection = %#v", result.Artifacts)
	}
}

func TestRemoteProviderPluginSerializesCanonicalRouteAndReturnsResult(t *testing.T) {
	t.Parallel()

	var captured executeRequestV1
	provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/executions" {
			t.Fatalf("plugin execute request = %s %s", request.Method, request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Fatalf("decode plugin execute request: %v", err)
		}
		return jsonResponse(request, http.StatusOK, `{
			"schema":"coze.sandbox.execute.v1",
			"execution_id":"execution_plugin",
			"status":"succeeded",
			"exit_code":0,
			"stdout":"plugin-result",
			"stderr":"",
			"artifacts":[]
		}`), nil
	}))
	request := validExecuteRequest()
	request.Scope = domainsandbox.ScopePlugin
	request.WorkloadKind = WorkloadPlugin
	request.Entrypoint = PluginCodeRunnerEntrypoint
	request.Files = nil

	result, err := provider.Execute(context.Background(), request)

	if err != nil {
		t.Fatalf("execute plugin: %v", err)
	}
	if captured.Scope != domainsandbox.ScopePlugin ||
		captured.WorkloadKind != WorkloadPlugin ||
		captured.Entrypoint != PluginCodeRunnerEntrypoint {
		t.Fatalf("plugin execute envelope = %#v", captured)
	}
	if result.ExecutionID != "execution_plugin" ||
		result.Status != ExecutionStatusSucceeded ||
		result.Stdout != "plugin-result" ||
		result.ExitCode == nil ||
		*result.ExitCode != 0 {
		t.Fatalf("plugin execute result = %#v", result)
	}
}

func TestRemoteProviderMCPStdioUsesExistingExecutionEndpoint(t *testing.T) {
	t.Parallel()

	invoke, err := MarshalMCPStdioInvokeEnvelope(MCPStdioInvokeEnvelope{
		Schema:        MCPStdioInvokeSchemaV1,
		OperationID:   "operation_123",
		Command:       "npx",
		Args:          []string{"-y", "@example/provider-mcp"},
		ToolName:      "search-docs",
		Arguments:     json.RawMessage(`{"query":"docs"}`),
		WorkingDir:    "workspace/mcp_stdio/run-20",
		EnvNames:      []string{"MCP_TOKEN"},
		ClientVersion: "1",
	})
	if err != nil {
		t.Fatalf("marshal invoke: %v", err)
	}
	var captured map[string]any
	exitCode := 0
	responseBody, err := json.Marshal(executeResponseV1{
		Schema:      ExecuteSchemaV1,
		ExecutionID: "execution_mcp_stdio",
		Status:      ExecutionStatusSucceeded,
		ExitCode:    &exitCode,
		Stdout:      `{"schema":"coze.sandbox.mcp_stdio.result.v1","content":[{"type":"text","text":"ok"}],"is_error":false}`,
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1/executions" {
			t.Fatalf("unexpected MCP stdio endpoint %q", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(request, http.StatusOK, string(responseBody)), nil
	}))
	request := validExecuteRequest()
	request.Scope = domainsandbox.ScopeMCPStdio
	request.WorkloadKind = WorkloadMCPStdio
	request.Entrypoint = "mcp/stdio/invoke"
	request.Stdin = invoke
	request.Files = nil
	request.Env = map[string]string{"MCP_TOKEN": "provider-request-secret"}
	request.Policy.AllowNetwork = false
	request.Policy.NetworkAllowlist = nil
	request.Policy.AllowedEnvNames = []string{"MCP_TOKEN"}
	request.Policy.AllowedExecutables = []string{"npx"}

	result, err := provider.Execute(context.Background(), request)

	if err != nil {
		t.Fatalf("execute MCP stdio: %v", err)
	}
	if captured["scope"] != string(domainsandbox.ScopeMCPStdio) ||
		captured["workload_kind"] != string(WorkloadMCPStdio) ||
		captured["entrypoint"] != "mcp/stdio/invoke" {
		t.Fatalf("MCP stdio envelope = %#v", captured)
	}
	if result.ExecutionID != "execution_mcp_stdio" {
		t.Fatalf("execution result = %#v", result)
	}
}

func TestRemoteProviderExecutionIdentityUsesHeadersWithoutChangingExecuteJSON(t *testing.T) {
	t.Parallel()

	keyring, err := sandboxidentity.NewKeyring("current", map[string][]byte{"current": []byte("test-signing-key-material")}, time.Minute)
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	keyring.Nonce = func() (string, error) { return "nonce_123", nil }
	config := validRemoteProviderConfig()
	config.IdentitySigner = keyring
	request := validExecuteRequest()
	request.Identity = ExecutionIdentity{SpaceID: 11, UserID: 22, ExecutionID: "exec_123"}
	wantBody, _, err := canonicalExecuteRequest(request, false, timeNow())
	if err != nil {
		t.Fatalf("canonical execute request = %v", err)
	}
	digest := sha256.Sum256(wantBody)
	provider, err := newRemoteProviderWithDoer(config, providerDoerFunc(func(httpRequest *http.Request) (*http.Response, error) {
		body, readErr := io.ReadAll(httpRequest.Body)
		if readErr != nil {
			t.Fatalf("read execute body: %v", readErr)
		}
		if gotDigest := sha256.Sum256(body); gotDigest != digest {
			t.Fatalf("execute body changed with identity: got_length=%d got_sha256=%x want_length=%d want_sha256=%x", len(body), gotDigest, len(wantBody), digest)
		}
		contextHeader := httpRequest.Header.Get(SandboxContextHeader)
		signatureHeader := httpRequest.Header.Get(SandboxContextSignatureHeader)
		if contextHeader == "" || signatureHeader == "" {
			t.Fatalf("identity headers missing: context_present=%t signature_present=%t", contextHeader != "", signatureHeader != "")
		}
		if _, verifyErr := keyring.Verify(contextHeader, signatureHeader, sandboxidentity.ScopeAgent, digest[:], time.Now().UTC()); verifyErr != nil {
			t.Fatalf("verify identity headers: %v", verifyErr)
		}
		return jsonResponse(httpRequest, http.StatusOK, `{"schema":"coze.sandbox.execute.v1","execution_id":"exec_123","status":"succeeded","exit_code":0,"stdout":"","stderr":"","artifacts":[]}`), nil
	}))
	if err != nil {
		t.Fatalf("new remote provider = %v", err)
	}
	if _, err := provider.Execute(context.Background(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestRemoteProviderExecutionIdentityRequiresSigner(t *testing.T) {
	t.Parallel()

	calls := 0
	provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected request")
	}))
	request := validExecuteRequest()
	request.Identity = ExecutionIdentity{SpaceID: 11, UserID: 22, ExecutionID: "exec_123"}
	if _, err := provider.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) || calls != 0 {
		t.Fatalf("Execute() error/calls = %v/%d", err, calls)
	}
}

func TestRemoteProviderHealthUsesStrictBoundedProtocol(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body         string
		wantErr      error
		wantFeatures []domainsandbox.ProviderFeature
	}{
		"valid": {
			body: `{"protocol_version":"v1","status":"healthy","capabilities":["agent","mcp_stdio"]}`,
		},
		"queue status feature": {
			body:         `{"protocol_version":"v1","status":"healthy","capabilities":["agent"],"features":["queue_status_v1"]}`,
			wantFeatures: []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1},
		},
		"unknown feature": {
			body:    `{"protocol_version":"v1","status":"healthy","capabilities":["agent"],"features":["unsafe_debug_v1"]}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"duplicate feature": {
			body:    `{"protocol_version":"v1","status":"healthy","capabilities":["agent"],"features":["queue_status_v1","queue_status_v1"]}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"unknown field": {
			body:    `{"protocol_version":"v1","status":"healthy","capabilities":["agent"],"raw":"secret"}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"duplicate field": {
			body:    `{"protocol_version":"v1","status":"healthy","status":"unhealthy","capabilities":["agent"]}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"case variant duplicate": {
			body:    `{"protocol_version":"v1","Protocol_Version":"v1","status":"healthy","capabilities":["agent"]}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"single protocol case variant": {
			body:    `{"Protocol_Version":"v1","status":"healthy","capabilities":["agent"]}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"single status case variant": {
			body:    `{"protocol_version":"v1","Status":"healthy","capabilities":["agent"]}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"invalid utf8": {
			body:    string([]byte{'{', '"', 's', 't', 'a', 't', 'u', 's', '"', ':', '"', 0xff, '"', '}'}),
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"trailing object": {
			body:    `{"protocol_version":"v1","status":"healthy","capabilities":["agent"]}{}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"non-object root": {
			body:    `[{"protocol_version":"v1","status":"healthy","capabilities":["agent"]}]`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"protocol mismatch": {
			body:    `{"protocol_version":"v2","status":"healthy","capabilities":["agent"]}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"unknown capability": {
			body:    `{"protocol_version":"v1","status":"healthy","capabilities":["shell"]}`,
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
		"oversized": {
			body:    strings.Repeat("x", MaxHealthResponseBodyBytes+1),
			wantErr: domainsandbox.ErrProviderUnhealthy,
		},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Path != "/v1/health" {
					t.Fatalf("health request = %s %s", request.Method, request.URL.Path)
				}
				return jsonResponse(request, http.StatusOK, test.body), nil
			}))
			result, err := provider.Health(context.Background())
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("health error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("health: %v", err)
			}
			if result.ProtocolVersion != HealthProtocolV1 || result.Status != domainsandbox.HealthStatusHealthy || len(result.Capabilities) != 2 && test.wantFeatures == nil {
				t.Fatalf("health result = %#v", result)
			}
			if len(result.Features) != len(test.wantFeatures) {
				t.Fatalf("health features = %#v, want %#v", result.Features, test.wantFeatures)
			}
			for index := range test.wantFeatures {
				if result.Features[index] != test.wantFeatures[index] {
					t.Fatalf("health features = %#v, want %#v", result.Features, test.wantFeatures)
				}
			}
		})
	}
}

func TestRemoteProviderExecuteRejectsNestedDuplicateArtifactFields(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"exact duplicate": `{
			"schema":"coze.sandbox.execute.v1","execution_id":"exec_123","status":"succeeded",
			"exit_code":0,"stdout":"ok","stderr":"",
			"artifacts":[{"id":"artifact_1","id":"artifact_2","name":"report.json","media_type":"application/json","size":12}]
		}`,
		"case variant duplicate": `{
			"schema":"coze.sandbox.execute.v1","execution_id":"exec_123","status":"succeeded",
			"exit_code":0,"stdout":"ok","stderr":"",
			"artifacts":[{"id":"artifact_1","ID":"artifact_2","name":"report.json","media_type":"application/json","size":12}]
		}`,
		"single noncanonical field": `{
			"schema":"coze.sandbox.execute.v1","execution_id":"exec_123","status":"succeeded",
			"exit_code":0,"stdout":"ok","stderr":"",
			"artifacts":[{"ID":"artifact_1","name":"report.json","media_type":"application/json","size":12}]
		}`,
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				return jsonResponse(request, http.StatusOK, body), nil
			}))
			_, err := provider.Execute(context.Background(), validExecuteRequest())
			if !errors.Is(err, domainsandbox.ErrProviderUnhealthy) {
				t.Fatalf("nested duplicate error = %v", err)
			}
		})
	}
}

func TestRemoteStrictJSONAllowsCaseDistinctDynamicMapKeys(t *testing.T) {
	t.Parallel()

	type dynamicResponse struct {
		Metadata map[string]bool `json:"metadata"`
	}
	const body = `{"metadata":{"Feature":true,"feature":false}}`
	response := &http.Response{
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	var decoded dynamicResponse
	if err := decodeStrictJSONResponse(response, 1024, &decoded); err != nil {
		t.Fatalf("case-distinct dynamic metadata: %v", err)
	}
	if !decoded.Metadata["Feature"] || decoded.Metadata["feature"] {
		t.Fatalf("dynamic metadata changed: %#v", decoded.Metadata)
	}
}

func TestRemoteProviderExecutionWireLimitAllowsEscapedLogicalBoundary(t *testing.T) {
	t.Parallel()

	stdout := strings.Repeat("<", MaxOutputStreamBytes)
	stderr := strings.Repeat("<", MaxOutputStreamBytes)
	wire, err := json.Marshal(executeResponseV1{
		Schema:      ExecuteSchemaV1,
		ExecutionID: "exec_escaped_boundary",
		Status:      ExecutionStatusSucceeded,
		ExitCode:    intPointer(0),
		Stdout:      stdout,
		Stderr:      stderr,
		Artifacts:   []ArtifactSummary{},
	})
	if err != nil {
		t.Fatalf("marshal escaped response: %v", err)
	}
	if len(wire) <= MaxCollectedOutputBytes {
		t.Fatalf("test response did not exercise wire expansion: %d", len(wire))
	}
	provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(request, http.StatusOK, string(wire)), nil
	}))
	request := validExecuteRequest()
	request.Policy.MaxOutputBytes = MaxCollectedOutputBytes
	result, err := provider.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("escaped logical boundary: %v", err)
	}
	if len(result.Stdout)+len(result.Stderr) != MaxCollectedOutputBytes {
		t.Fatalf("logical output bytes = %d", len(result.Stdout)+len(result.Stderr))
	}
}

func TestRemoteProviderExecutionWireLimitRejectsOversizedEnvelope(t *testing.T) {
	t.Parallel()

	provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		response := jsonResponse(request, http.StatusOK, `{}`)
		response.ContentLength = 32*1024*1024 + 1
		return response, nil
	}))
	_, err := provider.Execute(context.Background(), validExecuteRequest())
	if !errors.Is(err, domainsandbox.ErrProviderUnhealthy) {
		t.Fatalf("oversized wire response error = %v", err)
	}
}

func TestRemoteProviderProductionConstructorAlwaysBuildsSafeHTTP(t *testing.T) {
	t.Parallel()

	provider, err := NewRemoteProvider(validRemoteProviderConfig())
	if err != nil {
		t.Fatalf("new production provider: %v", err)
	}
	client, ok := provider.doer.(*http.Client)
	if !ok {
		t.Fatalf("production doer type = %T", provider.doer)
	}
	if _, ok := client.Transport.(*endpointHTTPTransport); !ok {
		t.Fatalf("production transport type = %T", client.Transport)
	}
}

func TestRemoteProviderProductionConstructorSupportsExactOriginPlainHTTP(t *testing.T) {
	t.Parallel()

	config := validRemoteProviderConfig()
	config.Endpoint = "http://sandbox.example.test/"
	provider, err := NewRemoteProvider(config)
	if err != nil {
		t.Fatalf("NewRemoteProvider(http) error = %v", err)
	}
	client, ok := provider.doer.(*http.Client)
	if !ok {
		t.Fatalf("production doer type = %T", provider.doer)
	}
	transport, ok := client.Transport.(*endpointHTTPTransport)
	if !ok || transport.base.Proxy != nil || provider.endpoint.TransportEncrypted {
		t.Fatalf("http transport = %#v encrypted=%t", client.Transport, provider.endpoint.TransportEncrypted)
	}
	redirect, _ := http.NewRequest(http.MethodGet, "http://sandbox.example.test/v1/health", nil)
	via, _ := http.NewRequest(http.MethodGet, "http://sandbox.example.test/v1/start", nil)
	redirect.Header.Set("Authorization", "Bearer secret")
	if err := client.CheckRedirect(redirect, []*http.Request{via}); !errors.Is(err, domainsandbox.ErrInvalidInput) || redirect.Header.Get("Authorization") != "" {
		t.Fatalf("redirect error/header = %v/%q", err, redirect.Header.Get("Authorization"))
	}
}

func TestRemoteProviderCloseReleasesIdleConnectionsExactlyOnce(t *testing.T) {
	doer := &providerIdleClosingDoer{}
	provider := mustRemoteProvider(t, doer)
	if err := provider.CloseContext(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := provider.CloseContext(context.Background()); err != nil {
		t.Fatalf("close twice: %v", err)
	}
	if doer.closeCalls != 1 {
		t.Fatalf("idle connection close calls = %d", doer.closeCalls)
	}

	closeErr := errors.New("transport endpoint token must-not-leak")
	errorDoer := &providerErrorClosingDoer{closeErr: closeErr}
	provider = mustRemoteProvider(t, errorDoer)
	if err := provider.CloseContext(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("close error = %v", err)
	}
	if err := provider.CloseContext(context.Background()); !errors.Is(err, closeErr) || errorDoer.closeCalls != 2 {
		t.Fatalf("retryable close error/calls = %v/%d", err, errorDoer.closeCalls)
	}
}

func TestRemoteProviderMapsStatusAndTransportErrorsWithoutLeakage(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		status  int
		wantErr error
	}{
		{status: http.StatusUnauthorized, wantErr: domainsandbox.ErrCredentialInvalid},
		{status: http.StatusForbidden, wantErr: domainsandbox.ErrCredentialInvalid},
		{status: http.StatusTooManyRequests, wantErr: domainsandbox.ErrCapacityExhausted},
		{status: http.StatusInternalServerError, wantErr: domainsandbox.ErrProviderUnhealthy},
		{status: http.StatusBadRequest, wantErr: domainsandbox.ErrInvalidInput},
	} {
		provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
			return jsonResponse(request, test.status, `{"secret":"must-not-leak"}`), nil
		}))
		_, err := provider.Health(context.Background())
		if !errors.Is(err, test.wantErr) {
			t.Fatalf("status %d error = %v", test.status, err)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "sandbox.example.test") {
			t.Fatalf("status %d leaked response or endpoint: %v", test.status, err)
		}
	}

	provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network failed with synthetic-test-token")
	}))
	_, err := provider.Execute(context.Background(), validExecuteRequest())
	if !errors.Is(err, domainsandbox.ErrProviderUnhealthy) || strings.Contains(err.Error(), "synthetic-test-token") {
		t.Fatalf("transport error was not safely mapped: %v", err)
	}
}

func TestRemoteProviderCancelValidatesExecutionID(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	callCount := 0
	provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		if request.Method != http.MethodPost || request.URL.Path != "/v1/executions/exec_123:cancel" {
			t.Fatalf("cancel request = %s %s", request.Method, request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
	}))
	if err := provider.Cancel(context.Background(), "exec_123"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := provider.Cancel(context.Background(), "../secret"); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("invalid execution ID error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if callCount != 1 {
		t.Fatalf("cancel transport calls = %d", callCount)
	}
}

func TestProviderContractNormalizesCopiesAndRejectsUnboundedInputs(t *testing.T) {
	t.Parallel()

	request := validExecuteRequest()
	normalized, err := normalizeExecuteRequest(request, time.Now())
	if err != nil {
		t.Fatalf("normalize request: %v", err)
	}
	request.Args[0] = "mutated"
	request.Env["MODE"] = "mutated"
	request.Files[0].Path = "mutated"
	request.Policy.NetworkAllowlist[0] = "mutated"
	if normalized.Args[0] != "--safe" || normalized.Env["MODE"] != "test" || normalized.Files[0].Path != "inputs/request.json" || normalized.Policy.NetworkAllowlist[0] != "api.example.test" {
		t.Fatalf("normalization did not defensively copy: %#v", normalized)
	}

	invalidRequests := []ExecuteRequest{
		withRequestMutation(validExecuteRequest(), func(value *ExecuteRequest) { value.WorkloadKind = WorkloadAppDev }),
		withRequestMutation(validExecuteRequest(), func(value *ExecuteRequest) { value.Entrypoint = "../bin/run" }),
		withRequestMutation(validExecuteRequest(), func(value *ExecuteRequest) { value.Entrypoint = "/host/bin/run" }),
		withRequestMutation(validExecuteRequest(), func(value *ExecuteRequest) { value.IdempotencyKey = "bad key" }),
		withRequestMutation(validExecuteRequest(), func(value *ExecuteRequest) { value.Args = make([]string, MaxArgs+1) }),
		withRequestMutation(validExecuteRequest(), func(value *ExecuteRequest) { value.Stdin = make([]byte, MaxStdinBytes+1) }),
		withRequestMutation(validExecuteRequest(), func(value *ExecuteRequest) { value.Files[0].Digest = "sha256:bad" }),
	}
	for index, invalid := range invalidRequests {
		if _, err := normalizeExecuteRequest(invalid, time.Now()); !errors.Is(err, domainsandbox.ErrInvalidInput) {
			t.Fatalf("invalid request %d error = %v", index, err)
		}
	}
}

func validRemoteProviderConfig() RemoteProviderConfig {
	return RemoteProviderConfig{
		Endpoint:            "https://sandbox.example.test/",
		Credential:          "synthetic-test-token",
		AllowedHosts:        []string{"sandbox.example.test"},
		AllowedPrivateCIDRs: []string{"10.0.0.0/8"},
		Timeout:             2 * time.Second,
	}
}

func validExecuteRequest() ExecuteRequest {
	exitDeadline := time.Now().Add(time.Minute).UTC()
	return ExecuteRequest{
		Scope:          domainsandbox.ScopeAgent,
		WorkloadKind:   WorkloadAgent,
		IdempotencyKey: "idem_123",
		Deadline:       exitDeadline,
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds:   30,
			MemoryLimitMB:    256,
			CPULimit:         1,
			MaxOutputBytes:   1024 * 1024,
			MaxConcurrency:   1,
			AllowNetwork:     true,
			NetworkAllowlist: []string{"api.example.test"},
		},
		Entrypoint: "bin/run",
		Args:       []string{"--safe"},
		Env:        map[string]string{"MODE": "test"},
		Stdin:      []byte("input"),
		Files: []FileReference{{
			ID:     "file_1",
			Path:   "inputs/request.json",
			Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Size:   12,
		}},
	}
}

func withRequestMutation(request ExecuteRequest, mutate func(*ExecuteRequest)) ExecuteRequest {
	mutate(&request)
	return request
}

func mustRemoteProvider(t *testing.T, doer remoteHTTPDoer) *RemoteProvider {
	t.Helper()
	provider, err := newRemoteProviderWithDoer(validRemoteProviderConfig(), doer)
	if err != nil {
		t.Fatalf("new remote provider: %v", err)
	}
	return provider
}

type providerDoerFunc func(*http.Request) (*http.Response, error)

func (f providerDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

type providerIdleClosingDoer struct {
	closeCalls int
}

func (*providerIdleClosingDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func (d *providerIdleClosingDoer) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.closeCalls++
	return nil
}

type providerErrorClosingDoer struct {
	closeCalls int
	closeErr   error
}

func (*providerErrorClosingDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func (d *providerErrorClosingDoer) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.closeCalls++
	return d.closeErr
}

func jsonResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}
}
