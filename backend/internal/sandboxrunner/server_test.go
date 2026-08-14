// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestServerHealthIsDiscoverableAndRejectsTrailingPaths(t *testing.T) {
	server := newTestServer(t)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d", response.Code)
	}
	var body struct {
		ProtocolVersion string                          `json:"protocol_version"`
		Status          domainsandbox.HealthStatus      `json:"status"`
		Capabilities    []domainsandbox.Scope           `json:"capabilities"`
		Features        []domainsandbox.ProviderFeature `json:"features"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.ProtocolVersion != "v1" ||
		body.Status != domainsandbox.HealthStatusHealthy || len(body.Capabilities) != 2 ||
		len(body.Features) != 2 || body.Features[0] != domainsandbox.ProviderFeatureQueueStatusV1 || body.Features[1] != domainsandbox.ProviderFeatureSignedExecutionContext {
		t.Fatalf("health body/error = %#v/%v", body, err)
	}

	trailing := httptest.NewRecorder()
	server.Handler().ServeHTTP(trailing, httptest.NewRequest(http.MethodGet, "/v1/health/extra", nil))
	if trailing.Code != http.StatusNotFound {
		t.Fatalf("trailing path status = %d", trailing.Code)
	}
	query := httptest.NewRecorder()
	server.Handler().ServeHTTP(query, httptest.NewRequest(http.MethodGet, "/v1/health?unexpected=true", nil))
	if query.Code != http.StatusBadRequest || strings.Contains(query.Body.String(), "unexpected") {
		t.Fatalf("query status/body = %d/%s", query.Code, query.Body.String())
	}
}

func TestServerHealthFailsClosedWhenRootlessRuntimeIsUnavailable(t *testing.T) {
	dependencies := &recordingDependencies{}
	dependencies.readinessErr = ErrUnavailable
	server := newTestServerWithDependencies(t, dependencies.Dependencies())
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "rootless") {
		t.Fatalf("health status/body = %d/%s", response.Code, response.Body.String())
	}
}

func TestServerMountsSessionRoutesOnlyWhenSessionBackendEnabled(t *testing.T) {
	config := validRuntimeConfig()
	base := Dependencies{Scheduler: acceptSchedulerFunc(func(context.Context, ExecuteCommand) (ExecutionProjection, error) {
		return ExecutionProjection{ExecutionID: "exec-runner-test", Status: infrasandbox.ExecutionStatusAccepted}, nil
	})}
	session := &recordingSessionHandler{status: 299}
	core := &recordingSessionCoreReadiness{}

	disabled, err := NewServer(config, Dependencies{Scheduler: base.Scheduler, Session: session, CoreReadiness: core})
	if err != nil {
		t.Fatalf("NewServer(disabled) error = %v", err)
	}
	response := httptest.NewRecorder()
	disabled.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/sessions:acquire", nil))
	if response.Code != http.StatusNotFound || session.calls != 0 || core.calls != 0 {
		t.Fatalf("disabled status/session/core calls = %d/%d/%d", response.Code, session.calls, core.calls)
	}

	config.SessionBackendEnabled = true
	enabled, err := NewServer(config, Dependencies{Scheduler: base.Scheduler, Session: session, CoreReadiness: core})
	if err != nil {
		t.Fatalf("NewServer(enabled) error = %v", err)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/v1/sessions:acquire", nil),
		httptest.NewRequest(http.MethodGet, "/v1/sessions/session-1", nil),
		httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1:release", nil),
		httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1:destroy", nil),
		httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1:recover", nil),
		httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1/operations", nil),
		httptest.NewRequest(http.MethodGet, "/v1/sessions/session-1/operations/op-1", nil),
		httptest.NewRequest(http.MethodGet, "/v1/sessions/session-1/operations/op-1/events", nil),
		httptest.NewRequest(http.MethodPost, "/v1/sessions/session-1/operations/op-1:cancel", nil),
		httptest.NewRequest(http.MethodGet, "/v1/session-configuration", nil),
		httptest.NewRequest(http.MethodPut, "/v1/session-configuration", nil),
		httptest.NewRequest(http.MethodGet, "/v1/session-runtime-status", nil),
	} {
		response = httptest.NewRecorder()
		enabled.Handler().ServeHTTP(response, request)
		if response.Code != session.status {
			t.Fatalf("%s %s status = %d", request.Method, request.URL.Path, response.Code)
		}
	}
	if session.calls != 12 {
		t.Fatalf("Session handler calls = %d, want 12", session.calls)
	}
}

func TestServerRequiresSessionHandlerAndCoreReadinessWhenEnabled(t *testing.T) {
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	scheduler := acceptSchedulerFunc(func(context.Context, ExecuteCommand) (ExecutionProjection, error) {
		return ExecutionProjection{ExecutionID: "exec-runner-test", Status: infrasandbox.ExecutionStatusAccepted}, nil
	})
	for name, dependencies := range map[string]Dependencies{
		"missing Session handler": {Scheduler: scheduler, CoreReadiness: &recordingSessionCoreReadiness{}},
		"missing Core readiness":  {Scheduler: scheduler, Session: &recordingSessionHandler{status: http.StatusNoContent}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewServer(config, dependencies); !errors.Is(err, ErrConfiguration) {
				t.Fatalf("NewServer() error = %v, want ErrConfiguration", err)
			}
		})
	}
}

func TestServerHealthAdvertisesSessionFeaturesOnlyWhileCoreReady(t *testing.T) {
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	core := &recordingSessionCoreReadiness{}
	server, err := NewServer(config, Dependencies{
		Scheduler: acceptSchedulerFunc(func(context.Context, ExecuteCommand) (ExecutionProjection, error) {
			return ExecutionProjection{ExecutionID: "exec-runner-test", Status: infrasandbox.ExecutionStatusAccepted}, nil
		}),
		Session: &recordingSessionHandler{status: http.StatusNoContent}, CoreReadiness: core,
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	readFeatures := func() []domainsandbox.ProviderFeature {
		t.Helper()
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("health status/body = %d/%s", response.Code, response.Body.String())
		}
		var body struct {
			Features []domainsandbox.ProviderFeature `json:"features"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Features
	}

	ready := readFeatures()
	if len(ready) != 4 || ready[2] != domainsandbox.ProviderFeatureSandboxSessionV1 || ready[3] != domainsandbox.ProviderFeatureSignedSessionContextV2 {
		t.Fatalf("ready features = %#v", ready)
	}
	core.err = ErrUnavailable
	notReady := readFeatures()
	if len(notReady) != 2 || notReady[0] != domainsandbox.ProviderFeatureQueueStatusV1 || notReady[1] != domainsandbox.ProviderFeatureSignedExecutionContext {
		t.Fatalf("not-ready features = %#v", notReady)
	}
}

func TestServerSessionConfigurationRoutesBypassCoreAdmissionGate(t *testing.T) {
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	core := &recordingSessionCoreReadiness{err: ErrUnavailable}
	session := &recordingSessionHandler{status: http.StatusOK}
	server, err := NewServer(config, Dependencies{
		Scheduler: acceptSchedulerFunc(func(context.Context, ExecuteCommand) (ExecutionProjection, error) {
			return ExecutionProjection{ExecutionID: "exec-runner-test", Status: infrasandbox.ExecutionStatusAccepted}, nil
		}),
		Session: session, CoreReadiness: core,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{{http.MethodGet, "/v1/session-configuration"}, {http.MethodPut, "/v1/session-configuration"}, {http.MethodGet, "/v1/session-runtime-status"}} {
		request := httptest.NewRequest(route.method, route.path, nil)
		request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d", route.method, route.path, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions:acquire", nil)
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || session.calls != 3 {
		t.Fatalf("business status/session calls = %d/%d", response.Code, session.calls)
	}
}

func TestServerRejectsQueryParametersWithoutRequestDetail(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/executions?artifact_token=secret-value", strings.NewReader(validExecuteWireBody(t)))
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "secret-value") || strings.Contains(response.Body.String(), "runner-auth-token") {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
}

func TestServerExecuteRequiresBearerSignedContextAndStrictBody(t *testing.T) {
	server := newTestServer(t)
	body := supportedExecuteWireBody(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/executions", strings.NewReader(body))
	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, request)
	if unauthorized.Code != http.StatusUnauthorized || strings.Contains(unauthorized.Body.String(), "runner-auth-token") {
		t.Fatalf("unauthorized status/body = %d/%s", unauthorized.Code, unauthorized.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/executions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	missingContext := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingContext, request)
	if missingContext.Code != http.StatusUnauthorized {
		t.Fatalf("missing context status = %d", missingContext.Code)
	}

	request = signedExecuteRequest(t, body)
	accepted := httptest.NewRecorder()
	server.Handler().ServeHTTP(accepted, request)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("execute status/body = %d/%s", accepted.Code, accepted.Body.String())
	}

	unknown := strings.TrimSuffix(body, "}") + `,"unexpected":true}`
	request = signedExecuteRequest(t, unknown)
	badBody := httptest.NewRecorder()
	server.Handler().ServeHTTP(badBody, request)
	if badBody.Code != http.StatusBadRequest || strings.Contains(badBody.Body.String(), "unexpected") {
		t.Fatalf("strict body status/body = %d/%s", badBody.Code, badBody.Body.String())
	}

	nestedUnknown := strings.Replace(body, `"max_concurrency":2`, `"max_concurrency":2,"unexpected":true`, 1)
	request = signedExecuteRequest(t, nestedUnknown)
	badNestedBody := httptest.NewRecorder()
	server.Handler().ServeHTTP(badNestedBody, request)
	if badNestedBody.Code != http.StatusBadRequest || strings.Contains(badNestedBody.Body.String(), "unexpected") {
		t.Fatalf("strict nested body status/body = %d/%s", badNestedBody.Code, badNestedBody.Body.String())
	}
}

func TestServerExecuteRejectsScopesNotAdvertisedByRuntimeImage(t *testing.T) {
	server := newTestServer(t)
	for _, body := range []string{
		validExecuteWireBody(t),
		strings.Replace(strings.Replace(validExecuteWireBody(t), `"scope":"appdev","workload_kind":"appdev"`, `"scope":"mcp_stdio","workload_kind":"mcp_stdio"`, 1), `"entrypoint":"appdev/runtime"`, `"entrypoint":"mcp/stdio/invoke"`, 1),
	} {
		request := signedExecuteRequest(t, body)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("unsupported scope status/body = %d/%s", response.Code, response.Body.String())
		}
	}
}

func TestServerLookupFailsClosedUntilIdentityBoundReconciliationIsAdvertised(t *testing.T) {
	dependencies := &recordingDependencies{}
	server := newTestServerWithDependencies(t, dependencies.Dependencies())
	for _, body := range []string{
		validLookupWireBody(),
		strings.Replace(validLookupWireBody(), `"scope":"agent","workload_kind":"agent"`, `"scope":"appdev","workload_kind":"appdev"`, 1),
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/executions:lookup", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || dependencies.lookupCalls != 0 {
			t.Fatalf("lookup status/calls = %d/%d", response.Code, dependencies.lookupCalls)
		}
	}
}

func TestParseExecuteRejectsInvalidDeadlineScopeEntrypointAndIdempotencyKey(t *testing.T) {
	valid := validExecuteWireBody(t)
	for name, body := range map[string]string{
		"expired deadline":           strings.Replace(valid, `"deadline":"`+deadlineFromWireBody(t, valid)+`"`, `"deadline":"2000-01-01T00:00:00Z"`, 1),
		"excessive deadline":         strings.Replace(valid, `"deadline":"`+deadlineFromWireBody(t, valid)+`"`, `"deadline":"2999-01-01T00:00:00Z"`, 1),
		"mismatched scope":           strings.Replace(valid, `"scope":"appdev"`, `"scope":"agent"`, 1),
		"invalid idempotency":        strings.Replace(valid, `"idempotency_key":"runner-test-operation"`, `"idempotency_key":"runner test"`, 1),
		"invalid entrypoint":         strings.Replace(valid, `"entrypoint":"appdev/runtime"`, `"entrypoint":"../main.py"`, 1),
		"plugin entrypoint mismatch": strings.Replace(strings.Replace(valid, `"scope":"appdev","workload_kind":"appdev"`, `"scope":"plugin","workload_kind":"plugin"`, 1), `"entrypoint":"appdev/runtime"`, `"entrypoint":"other.py"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseExecute([]byte(body)); err == nil {
				t.Fatal("parseExecute() unexpectedly accepted request")
			}
		})
	}
}

func TestParseExecuteAcceptsOnlyReviewedWorkloadEntrypoints(t *testing.T) {
	valid := validExecuteWireBody(t)
	for name, body := range map[string]string{
		"agent":                 strings.Replace(strings.Replace(valid, `"scope":"appdev","workload_kind":"appdev"`, `"scope":"agent","workload_kind":"agent"`, 1), `"entrypoint":"appdev/runtime"`, `"entrypoint":"agent/code/run"`, 1),
		"mcp stdio":             strings.Replace(strings.Replace(valid, `"scope":"appdev","workload_kind":"appdev"`, `"scope":"mcp_stdio","workload_kind":"mcp_stdio"`, 1), `"entrypoint":"appdev/runtime"`, `"entrypoint":"mcp/stdio/invoke"`, 1),
		"plugin":                strings.Replace(strings.Replace(valid, `"scope":"appdev","workload_kind":"appdev"`, `"scope":"plugin","workload_kind":"plugin"`, 1), `"entrypoint":"appdev/runtime"`, `"entrypoint":"plugin/code/run"`, 1),
		"appdev legacy adapter": valid,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseExecute([]byte(body)); err != nil {
				t.Fatalf("parseExecute() error = %v", err)
			}
		})
	}
	for name, body := range map[string]string{
		"agent shell":              strings.Replace(valid, `"entrypoint":"appdev/runtime"`, `"entrypoint":"sh"`, 1),
		"mcp arbitrary adapter":    strings.Replace(strings.Replace(valid, `"scope":"appdev","workload_kind":"appdev"`, `"scope":"mcp_stdio","workload_kind":"mcp_stdio"`, 1), `"entrypoint":"appdev/runtime"`, `"entrypoint":"mcp/custom/run"`, 1),
		"appdev arbitrary adapter": strings.Replace(valid, `"entrypoint":"appdev/runtime"`, `"entrypoint":"bin/appdev"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseExecute([]byte(body)); err == nil {
				t.Fatal("parseExecute() unexpectedly accepted unreviewed entrypoint")
			}
		})
	}
}

func TestParseExecuteRejectsUnsafeNestedPayload(t *testing.T) {
	valid := validExecuteWireBody(t)
	for name, body := range map[string]string{
		"unsafe environment key": strings.Replace(valid, `"env":{}`, `"env":{"BAD-KEY":"value"}`, 1),
		"unsafe file path":       strings.Replace(valid, `"files":[]`, `"files":[{"id":"file-1","path":"../escape","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","size":1}]`, 1),
		"unsafe artifact url":    strings.Replace(valid, `"artifact_references":[]`, `"artifact_references":[{"direction":"download","url":"http://storage.example/object","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","size":1,"media_type":"text/plain","expires_at":"`+time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)+`"}]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseExecute([]byte(body)); err == nil {
				t.Fatal("parseExecute() unexpectedly accepted unsafe nested payload")
			}
		})
	}
}

func TestServerBuildOperationIDMatchesRemoteProviderContract(t *testing.T) {
	dependencies := &recordingDependencies{}
	server := newTestServerWithDependencies(t, dependencies.Dependencies())
	request := httptest.NewRequest(http.MethodPost, "/v1/executions/exec-runner-test/builds", strings.NewReader(`{"schema":"coze.sandbox.appdev_build.v1","operation_id":"build operation+1"}`))
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || dependencies.beginBuildCalls != 1 {
		t.Fatalf("status/calls = %d/%d", response.Code, dependencies.beginBuildCalls)
	}
}

func TestServerConfigurationEndpointActivatesOnlyVerifiedSnapshots(t *testing.T) {
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewConfigurationStore(signer, schedulerConfigurationSettings(1))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t)
	server.configuration = store
	server.configurationApplier = store

	next := schedulerConfigurationSettings(2)
	envelope, err := signer.Sign(next)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/v1/configuration", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	accepted := httptest.NewRecorder()
	server.Handler().ServeHTTP(accepted, request)
	if accepted.Code != http.StatusNoContent || store.Settings().Version != 2 {
		t.Fatalf("status/version = %d/%d", accepted.Code, store.Settings().Version)
	}

	envelope.Settings.MaxOutstanding = 31
	body, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPut, "/v1/configuration", strings.NewReader(string(body)))
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	rejected := httptest.NewRecorder()
	server.Handler().ServeHTTP(rejected, request)
	if rejected.Code != http.StatusBadRequest || store.Settings().Version != 2 {
		t.Fatalf("status/version = %d/%d", rejected.Code, store.Settings().Version)
	}
}

func TestServerRuntimeStatusIsAuthenticatedAndContainsOnlyAggregateState(t *testing.T) {
	dependencies := &recordingDependencies{}
	server := newTestServerWithDependencies(t, dependencies.Dependencies())

	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/runtime-status", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/runtime-status", nil)
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || dependencies.runtimeStatusCalls != 1 {
		t.Fatalf("status/calls = %d/%d", response.Code, dependencies.runtimeStatusCalls)
	}
	if strings.Contains(response.Body.String(), "exec-") || strings.Contains(response.Body.String(), "space_id") || strings.Contains(response.Body.String(), "container_id") {
		t.Fatalf("runtime status leaked an identifier: %s", response.Body.String())
	}
	var body RuntimeStatusProjection
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Schema != runtimeStatusSchemaV1 || body.AppliedConfigurationVersion != 1 || body.Queued != 2 || body.ActiveContainers != 1 || body.MemoryReserveState != memoryReserveAvailable {
		t.Fatalf("runtime status/error = %#v/%v", body, err)
	}
}

func TestParseControlRequestRejectsUnsafeArtifactCapability(t *testing.T) {
	valid := validArtifactWireBody()
	for name, body := range map[string]string{
		"token whitespace": strings.Replace(valid, `"upload_token":"short-lived-token"`, `"upload_token":"not allowed"`, 1),
		"url query":        strings.Replace(valid, `"https://storage.example/upload"`, `"https://storage.example/upload?grant=secret"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseControlRequest("/v1/executions/exec-runner-test/artifacts:publish", []byte(body), time.Now().UTC()); err == nil {
				t.Fatal("parseControlRequest() unexpectedly accepted unsafe artifact capability")
			}
		})
	}
}

func TestServerControlEndpointsRequireAuthenticationAndRejectTrailingPaths(t *testing.T) {
	server := newTestServer(t)
	for name, test := range map[string]struct {
		method string
		path   string
		body   string
	}{
		"status":        {method: http.MethodGet, path: "/v1/executions/exec-runner-test"},
		"keep alive":    {method: http.MethodPost, path: "/v1/executions/exec-runner-test:keep-alive", body: `{}`},
		"cancel":        {method: http.MethodPost, path: "/v1/executions/exec-runner-test:cancel", body: `{}`},
		"queue":         {method: http.MethodGet, path: "/v1/executions/exec-runner-test/queue-status"},
		"lookup":        {method: http.MethodPost, path: "/v1/executions:lookup", body: validLookupWireBody()},
		"build":         {method: http.MethodPost, path: "/v1/executions/exec-runner-test/builds", body: `{"schema":"coze.sandbox.appdev_build.v1","operation_id":"build-1"}`},
		"build status":  {method: http.MethodGet, path: "/v1/executions/exec-runner-test/builds/build-1"},
		"artifact":      {method: http.MethodPost, path: "/v1/executions/exec-runner-test/artifacts:publish", body: `{"schema":"coze.sandbox.artifact_publish.v1","descriptor":{"kind":"appdev_build_archive","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","size":1},"upload_url":"https://storage.example/upload","upload_token":"short-lived-token","expires_at":"2026-08-12T00:00:00Z"}`},
		"configuration": {method: http.MethodGet, path: "/v1/configuration"},
	} {
		t.Run(name, func(t *testing.T) {
			unauthorized := httptest.NewRecorder()
			server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
			if unauthorized.Code != http.StatusUnauthorized {
				t.Fatalf("unauthorized status = %d", unauthorized.Code)
			}
			authorized := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			authorized.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, authorized)
			if response.Code == http.StatusNotFound || response.Code == http.StatusUnauthorized {
				t.Fatalf("control endpoint status/body = %d/%s", response.Code, response.Body.String())
			}
			trailing := httptest.NewRecorder()
			trailingRequest := httptest.NewRequest(test.method, test.path+"/extra", strings.NewReader(test.body))
			trailingRequest.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
			server.Handler().ServeHTTP(trailing, trailingRequest)
			if trailing.Code != http.StatusNotFound {
				t.Fatalf("trailing status = %d", trailing.Code)
			}
			if test.method == http.MethodPost {
				unknown := httptest.NewRequest(test.method, test.path, strings.NewReader(`{"unexpected":true}`))
				unknown.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
				badRequest := httptest.NewRecorder()
				server.Handler().ServeHTTP(badRequest, unknown)
				if badRequest.Code != http.StatusBadRequest {
					t.Fatalf("unknown body status/body = %d/%s", badRequest.Code, badRequest.Body.String())
				}
			}
		})
	}
}

func TestServerControlEndpointsDelegateOnlyToInjectedDependencies(t *testing.T) {
	dependencies := &recordingDependencies{}
	server := newTestServerWithDependencies(t, dependencies.Dependencies())
	for name, test := range map[string]struct {
		method string
		path   string
		body   string
	}{
		"status":        {http.MethodGet, "/v1/executions/exec-runner-test", ""},
		"keep alive":    {http.MethodPost, "/v1/executions/exec-runner-test:keep-alive", `{}`},
		"cancel":        {http.MethodPost, "/v1/executions/exec-runner-test:cancel", `{}`},
		"queue":         {http.MethodGet, "/v1/executions/exec-runner-test/queue-status", ""},
		"build":         {http.MethodPost, "/v1/executions/exec-runner-test/builds", `{"schema":"coze.sandbox.appdev_build.v1","operation_id":"build-1"}`},
		"build status":  {http.MethodGet, "/v1/executions/exec-runner-test/builds/build-1", ""},
		"artifact":      {http.MethodPost, "/v1/executions/exec-runner-test/artifacts:publish", validArtifactWireBody()},
		"configuration": {http.MethodGet, "/v1/configuration", ""},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code < http.StatusOK || response.Code >= http.StatusMultipleChoices {
				t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
			}
		})
	}
	if dependencies.statusCalls != 1 || dependencies.keepAliveCalls != 1 || dependencies.cancelCalls != 1 ||
		dependencies.queueCalls != 1 || dependencies.lookupCalls != 0 || dependencies.beginBuildCalls != 1 ||
		dependencies.buildStatusCalls != 1 || dependencies.publishArtifactCalls != 1 || dependencies.configurationCalls != 1 {
		t.Fatalf("dependency calls = %#v", dependencies)
	}
}

func TestServerControlResponsesRemainCompatibleWithRemoteProvider(t *testing.T) {
	dependencies := &recordingDependencies{}
	server := newTestServerWithDependencies(t, dependencies.Dependencies())
	for name, test := range map[string]struct {
		method string
		path   string
		body   string
		check  func(t *testing.T, response *httptest.ResponseRecorder)
	}{
		"keep alive empty body": {
			method: http.MethodPost, path: "/v1/executions/exec-runner-test:keep-alive", body: `{}`,
			check: func(t *testing.T, response *httptest.ResponseRecorder) {
				t.Helper()
				if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
					t.Fatalf("keep alive status/body = %d/%q", response.Code, response.Body.String())
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code < http.StatusOK || response.Code >= http.StatusMultipleChoices {
				t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
			}
			test.check(t, response)
		})
	}
}

func newTestServer(t *testing.T) *Server {
	return newTestServerWithDependencies(t, Dependencies{Scheduler: acceptSchedulerFunc(func(context.Context, ExecuteCommand) (ExecutionProjection, error) {
		return ExecutionProjection{ExecutionID: "exec-runner-test", Status: infrasandbox.ExecutionStatusAccepted}, nil
	})})
}

func newTestServerWithDependencies(t *testing.T, dependencies Dependencies) *Server {
	t.Helper()
	values := validRunnerEnvironment()
	values["SANDBOX_RUNNER_LISTEN_ADDR"] = "127.0.0.1:9443"
	values["SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP"] = "true"
	delete(values, "SANDBOX_RUNNER_TLS_CERT_FILE")
	delete(values, "SANDBOX_RUNNER_TLS_KEY_FILE")
	config, err := LoadConfig(mapGetenv(values))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	server, err := NewServer(config, dependencies)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return server
}

type recordingDependencies struct {
	statusCalls, keepAliveCalls, cancelCalls, queueCalls, lookupCalls           int
	beginBuildCalls, buildStatusCalls, publishArtifactCalls, configurationCalls int
	runtimeStatusCalls                                                          int
	readinessErr                                                                error
}

type recordingSessionHandler struct {
	status int
	calls  int
}

func (handler *recordingSessionHandler) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	handler.calls++
	writer.WriteHeader(handler.status)
}

type recordingSessionCoreReadiness struct {
	err   error
	calls int
}

func (readiness *recordingSessionCoreReadiness) CoreReady(context.Context) error {
	readiness.calls++
	return readiness.err
}

func (d *recordingDependencies) Dependencies() Dependencies {
	return Dependencies{
		Scheduler: acceptSchedulerFunc(func(context.Context, ExecuteCommand) (ExecutionProjection, error) {
			return ExecutionProjection{ExecutionID: "exec-runner-test", Status: infrasandbox.ExecutionStatusAccepted}, nil
		}),
		Store: d, Lifecycle: d, Artifacts: d, Configuration: d, RuntimeStatus: d, Readiness: d,
	}
}
func (d *recordingDependencies) Ready(context.Context) error { return d.readinessErr }

func (d *recordingDependencies) Status(context.Context, string) (infrasandbox.ExecuteResult, error) {
	d.statusCalls++
	return infrasandbox.ExecuteResult{ExecutionID: "exec-runner-test", Status: infrasandbox.ExecutionStatusRunning}, nil
}
func (d *recordingDependencies) Lookup(context.Context, infrasandbox.ExecutionLookupRequest) (infrasandbox.ExecutionLookupResult, error) {
	d.lookupCalls++
	return infrasandbox.ExecutionLookupResult{Status: infrasandbox.ExecutionLookupNotFound}, nil
}
func (d *recordingDependencies) QueueStatus(context.Context, string) (infrasandbox.QueueStatus, error) {
	d.queueCalls++
	return infrasandbox.QueueStatus{Schema: infrasandbox.QueueStatusSchemaV1}, nil
}
func (d *recordingDependencies) KeepAlive(context.Context, string) error {
	d.keepAliveCalls++
	return nil
}
func (d *recordingDependencies) Cancel(context.Context, string) error { d.cancelCalls++; return nil }
func (d *recordingDependencies) BeginBuild(context.Context, string, string) (infrasandbox.BuildObservation, error) {
	d.beginBuildCalls++
	return infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusAccepted}, nil
}
func (d *recordingDependencies) BuildStatus(context.Context, string, string) (infrasandbox.BuildObservation, error) {
	d.buildStatusCalls++
	return infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusRunning}, nil
}
func (d *recordingDependencies) PublishArtifact(context.Context, string, infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
	d.publishArtifactCalls++
	return infrasandbox.ArtifactPublishResult{Accepted: true}, nil
}
func (d *recordingDependencies) Configuration(context.Context) (ConfigurationProjection, error) {
	d.configurationCalls++
	return ConfigurationProjection{Schema: "coze.sandbox.runner_configuration.v1", Version: 1}, nil
}

func (d *recordingDependencies) RuntimeStatus(context.Context) (RuntimeStatusProjection, error) {
	d.runtimeStatusCalls++
	return RuntimeStatusProjection{Schema: runtimeStatusSchemaV1, AppliedConfigurationVersion: 1, Queued: 2, QueuedByScope: map[string]int{"agent": 2}, Running: 1, UsedWeight: 2, TotalWeight: 4, IdleContainers: 1, ActiveContainers: 1, MemoryReserveState: memoryReserveAvailable}, nil
}

func validLookupWireBody() string {
	return `{"schema":"coze.sandbox.execution_lookup.v1","scope":"agent","workload_kind":"agent","operation_id":"operation-1","request_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","space_id":"","project_id":"","generation":0}`
}

func validArtifactWireBody() string {
	return `{"schema":"coze.sandbox.artifact_publish.v1","descriptor":{"kind":"appdev_build_archive","digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","size":1},"upload_url":"https://storage.example/upload","upload_token":"short-lived-token","expires_at":"` + time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano) + `"}`
}

func deadlineFromWireBody(t *testing.T, body string) string {
	t.Helper()
	var value struct {
		Deadline string `json:"deadline"`
	}
	if err := json.Unmarshal([]byte(body), &value); err != nil || value.Deadline == "" {
		t.Fatalf("deadline/error = %q/%v", value.Deadline, err)
	}
	return value.Deadline
}

func validExecuteWireBody(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)
	return `{"schema":"coze.sandbox.execute.v1","scope":"appdev","workload_kind":"appdev","idempotency_key":"runner-test-operation","deadline":"` + deadline + `","policy":{"timeout_seconds":30,"memory_limit_mb":128,"cpu_limit":1,"max_output_bytes":4096,"max_concurrency":2,"allow_network":false,"network_allowlist":[],"allowed_env_names":[],"virtual_read_prefixes":[],"virtual_write_prefixes":[],"allowed_executables":[],"ffi_enabled":false,"node_modules_mode":"disabled","node_modules_directory_ref":""},"entrypoint":"appdev/runtime","args":[],"env":{},"stdin":"","files":[],"artifact_references":[]}`
}

func supportedExecuteWireBody(t *testing.T) string {
	t.Helper()
	return strings.Replace(strings.Replace(validExecuteWireBody(t), `"scope":"appdev","workload_kind":"appdev"`, `"scope":"agent","workload_kind":"agent"`, 1), `"entrypoint":"appdev/runtime"`, `"entrypoint":"agent/code/run"`, 1)
}

func signedExecuteRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	keyring, err := sandboxidentity.NewKeyring("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(body))
	var envelope struct {
		Scope domainsandbox.Scope `json:"scope"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatal(err)
	}
	identity := sandboxidentity.Request{Scope: sandboxidentity.Scope(envelope.Scope), SpaceID: 1, UserID: 2, ExecutionID: "exec-runner-test", RequestDigest: digest[:]}
	if envelope.Scope == domainsandbox.ScopeAppDev {
		identity.ProjectID = "project-1"
	} else if envelope.Scope == domainsandbox.ScopeMCPStdio {
		identity.SessionID = "session-1"
	}
	signed, err := keyring.Sign(identity)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/executions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	request.Header.Set(infrasandbox.SandboxContextHeader, signed.Context)
	request.Header.Set(infrasandbox.SandboxContextSignatureHeader, signed.Signature)
	return request
}
