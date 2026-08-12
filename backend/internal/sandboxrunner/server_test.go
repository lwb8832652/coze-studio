// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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
		body.Status != domainsandbox.HealthStatusHealthy || len(body.Capabilities) != 4 ||
		len(body.Features) != 1 || body.Features[0] != domainsandbox.ProviderFeatureQueueStatusV1 {
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
	body := validExecuteWireBody(t)
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

func TestParseExecuteRejectsInvalidDeadlineScopeEntrypointAndIdempotencyKey(t *testing.T) {
	valid := validExecuteWireBody(t)
	for name, body := range map[string]string{
		"expired deadline":           strings.Replace(valid, `"deadline":"`+deadlineFromWireBody(t, valid)+`"`, `"deadline":"2000-01-01T00:00:00Z"`, 1),
		"excessive deadline":         strings.Replace(valid, `"deadline":"`+deadlineFromWireBody(t, valid)+`"`, `"deadline":"2999-01-01T00:00:00Z"`, 1),
		"mismatched scope":           strings.Replace(valid, `"scope":"appdev"`, `"scope":"agent"`, 1),
		"invalid idempotency":        strings.Replace(valid, `"idempotency_key":"runner-test-operation"`, `"idempotency_key":"runner test"`, 1),
		"invalid entrypoint":         strings.Replace(valid, `"entrypoint":"main.py"`, `"entrypoint":"../main.py"`, 1),
		"plugin entrypoint mismatch": strings.Replace(strings.Replace(valid, `"scope":"appdev","workload_kind":"appdev"`, `"scope":"plugin","workload_kind":"plugin"`, 1), `"entrypoint":"main.py"`, `"entrypoint":"other.py"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseExecute([]byte(body)); err == nil {
				t.Fatal("parseExecute() unexpectedly accepted request")
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
		"lookup":        {method: http.MethodPost, path: "/v1/executions:lookup", body: `{"schema":"coze.sandbox.execution_lookup.v1","scope":"appdev","workload_kind":"appdev","operation_id":"operation-1","request_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`},
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
		"lookup":        {http.MethodPost, "/v1/executions:lookup", validLookupWireBody()},
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
		dependencies.queueCalls != 1 || dependencies.lookupCalls != 1 || dependencies.beginBuildCalls != 1 ||
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
		"lookup version": {
			method: http.MethodPost, path: "/v1/executions:lookup", body: validLookupWireBody(),
			check: func(t *testing.T, response *httptest.ResponseRecorder) {
				t.Helper()
				if response.Header().Get(infrasandbox.ExecutionLookupProtocolVersionHeader) != infrasandbox.ExecutionLookupProtocolVersionV1 {
					t.Fatalf("lookup version = %q", response.Header().Get(infrasandbox.ExecutionLookupProtocolVersionHeader))
				}
			},
		},
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
}

func (d *recordingDependencies) Dependencies() Dependencies {
	return Dependencies{
		Scheduler: acceptSchedulerFunc(func(context.Context, ExecuteCommand) (ExecutionProjection, error) {
			return ExecutionProjection{ExecutionID: "exec-runner-test", Status: infrasandbox.ExecutionStatusAccepted}, nil
		}),
		Store: d, Lifecycle: d, Artifacts: d, Configuration: d,
	}
}

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

func validLookupWireBody() string {
	return `{"schema":"coze.sandbox.execution_lookup.v1","scope":"appdev","workload_kind":"appdev","operation_id":"operation-1","request_digest":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","space_id":"","project_id":"","generation":0}`
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
	return `{"schema":"coze.sandbox.execute.v1","scope":"appdev","workload_kind":"appdev","idempotency_key":"runner-test-operation","deadline":"` + deadline + `","policy":{"timeout_seconds":30,"memory_limit_mb":128,"cpu_limit":1,"max_output_bytes":4096,"max_concurrency":2,"allow_network":false,"network_allowlist":[],"allowed_env_names":[],"virtual_read_prefixes":[],"virtual_write_prefixes":[],"allowed_executables":[],"ffi_enabled":false,"node_modules_mode":"disabled","node_modules_directory_ref":""},"entrypoint":"main.py","args":[],"env":{},"stdin":"","files":[],"artifact_references":[]}`
}

func signedExecuteRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	keyring, err := sandboxidentity.NewKeyring("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(body))
	signed, err := keyring.Sign(sandboxidentity.Request{Scope: sandboxidentity.ScopeAppDev, SpaceID: 1, UserID: 2, ProjectID: "project-1", ExecutionID: "exec-runner-test", RequestDigest: digest[:]})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/executions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	request.Header.Set(infrasandbox.SandboxContextHeader, signed.Context)
	request.Header.Set(infrasandbox.SandboxContextSignatureHeader, signed.Signature)
	return request
}
