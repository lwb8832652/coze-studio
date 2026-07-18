// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestRemoteProviderExecutionControlUsesStrictRecordedWire(t *testing.T) {
	var requests []string
	provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/executions/exec-real":
			return jsonResponse(request, http.StatusOK, `{
				"schema":"coze.sandbox.execute.v1","execution_id":"exec-real","status":"running",
				"exit_code":null,"stdout":"","stderr":"","artifacts":[]
			}`), nil
		case request.Method == http.MethodPost && request.URL.Path == "/v1/executions/exec-real:keep-alive":
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		case request.Method == http.MethodPost && request.URL.Path == "/v1/executions/exec-real:cancel":
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
			return nil, nil
		}
	}))
	result, err := provider.Status(context.Background(), "exec-real")
	if err != nil || result.Status != ExecutionStatusRunning || result.ExecutionID != "exec-real" {
		t.Fatalf("status = %#v, %v", result, err)
	}
	if err := provider.KeepAlive(context.Background(), "exec-real"); err != nil {
		t.Fatalf("keep alive: %v", err)
	}
	if err := provider.Cancel(context.Background(), "exec-real"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if !reflect.DeepEqual(requests, []string{
		"GET /v1/executions/exec-real",
		"POST /v1/executions/exec-real:keep-alive",
		"POST /v1/executions/exec-real:cancel",
	}) {
		t.Fatalf("execution control requests = %#v", requests)
	}
}

func TestRemoteProviderStatusAndKeepAliveRejectMalformedResponses(t *testing.T) {
	for name, test := range map[string]struct {
		statusBody      string
		keepAliveBody   string
		keepAliveHeader http.Header
		checkStatus     bool
	}{
		"status unknown field": {
			checkStatus: true,
			statusBody:  `{"schema":"coze.sandbox.execute.v1","execution_id":"exec-real","status":"running","exit_code":null,"stdout":"","stderr":"","artifacts":[],"secret":"leak"}`,
		},
		"status duplicate field": {
			checkStatus: true,
			statusBody:  `{"schema":"coze.sandbox.execute.v1","execution_id":"exec-real","execution_id":"other","status":"running","exit_code":null,"stdout":"","stderr":"","artifacts":[]}`,
		},
		"keep alive non-empty": {
			keepAliveBody:   `{"secret":"leak"}`,
			keepAliveHeader: http.Header{"Content-Type": []string{"application/json"}},
		},
	} {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				if test.checkStatus {
					return jsonResponse(request, http.StatusOK, test.statusBody), nil
				}
				body := test.keepAliveBody
				return &http.Response{
					StatusCode: http.StatusOK, Header: test.keepAliveHeader,
					Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request,
				}, nil
			}))
			var err error
			if test.checkStatus {
				_, err = provider.Status(context.Background(), "exec-real")
			} else {
				err = provider.KeepAlive(context.Background(), "exec-real")
			}
			if !errors.Is(err, domainsandbox.ErrProviderUnhealthy) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("malformed response error = %v", err)
			}
		})
	}
}

func TestRemoteProviderExecuteWiresCompleteCanonicalPolicyAndArtifacts(t *testing.T) {
	var captured map[string]any
	var capturedMethod, capturedPath string
	provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		capturedMethod, capturedPath = request.Method, request.URL.Path
		if err := json.NewDecoder(request.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return jsonResponse(request, http.StatusOK, `{
			"schema":"coze.sandbox.execute.v1","execution_id":"exec-policy","status":"accepted",
			"exit_code":null,"stdout":"","stderr":"","artifacts":[]
		}`), nil
	}))
	request := validExecuteRequest()
	request.Policy.AllowedEnvNames = []string{"HOME", "PATH"}
	request.Policy.VirtualReadPrefixes = []string{"inputs", "workspace/src"}
	request.Policy.VirtualWritePrefixes = []string{"outputs"}
	request.Policy.AllowedExecutables = []string{"node", "python3"}
	request.Policy.FFIEnabled = true
	request.Policy.NodeModulesMode = domainsandbox.NodeModulesModeApprovedDirectory
	request.Policy.NodeModulesDirectoryRef = "node-modules-v1"
	request.Policy.AllowEnv = []string{"LEGACY_SECRET"}
	request.Policy.AllowRead = []string{"legacy/read"}
	request.Policy.AllowWrite = []string{"legacy/write"}
	request.Policy.AllowRun = []string{"legacy-run"}
	request.Policy.AllowFFI = []string{"legacy-ffi"}
	request.Policy.NodeModulesDir = "legacy-node-secret"
	artifactExpiry := time.Now().Add(30 * time.Second).UTC()
	request.ArtifactReferences = []ArtifactReference{{
		Direction: ArtifactDirectionDownload,
		URL:       "https://artifacts.example.test/v1/grants/grant-1",
		Token:     "opaque-artifact-token",
		Digest:    "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Size:      128,
		MediaType: "application/gzip",
		ExpiresAt: artifactExpiry,
	}}
	if _, err := provider.Execute(context.Background(), request); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if capturedMethod != http.MethodPost || capturedPath != "/v1/executions" {
		t.Fatalf("execute request = %s %s", capturedMethod, capturedPath)
	}
	policy, ok := captured["policy"].(map[string]any)
	if !ok {
		t.Fatalf("policy wire = %#v", captured["policy"])
	}
	for _, key := range []string{
		"timeout_seconds", "memory_limit_mb", "cpu_limit", "max_output_bytes", "max_concurrency",
		"allow_network", "network_allowlist", "allowed_env_names", "virtual_read_prefixes",
		"virtual_write_prefixes", "allowed_executables", "ffi_enabled", "node_modules_mode",
		"node_modules_directory_ref",
	} {
		if _, exists := policy[key]; !exists {
			t.Fatalf("canonical policy key %q missing: %#v", key, policy)
		}
	}
	wantPolicy := map[string]any{
		"timeout_seconds": float64(30), "memory_limit_mb": float64(256), "cpu_limit": float64(1),
		"max_output_bytes": float64(1024 * 1024), "max_concurrency": float64(1),
		"allow_network": true, "network_allowlist": []any{"api.example.test"},
		"allowed_env_names":      []any{"HOME", "PATH"},
		"virtual_read_prefixes":  []any{"inputs", "workspace/src"},
		"virtual_write_prefixes": []any{"outputs"},
		"allowed_executables":    []any{"node", "python3"},
		"ffi_enabled":            true, "node_modules_mode": "approved_directory",
		"node_modules_directory_ref": "node-modules-v1",
	}
	if !reflect.DeepEqual(policy, wantPolicy) {
		t.Fatalf("canonical policy values = %#v, want %#v", policy, wantPolicy)
	}
	wireText, err := json.Marshal(captured)
	if err != nil {
		t.Fatalf("marshal captured wire: %v", err)
	}
	for _, prohibited := range []string{"LEGACY_SECRET", "legacy/read", "legacy/write", "legacy-run", "legacy-ffi", "legacy-node-secret", "allow_env", "allow_read", "allow_write", "allow_run", "allow_ffi", "node_modules_dir\"", "opaque-artifact-token", "synthetic-test-token"} {
		if strings.Contains(string(wireText), prohibited) {
			t.Fatalf("legacy policy leaked to wire: %s", wireText)
		}
	}
	artifacts, ok := captured["artifact_references"].([]any)
	if !ok || len(artifacts) != 1 {
		t.Fatalf("artifact references wire = %#v", captured["artifact_references"])
	}
	artifact := artifacts[0].(map[string]any)
	wantArtifact := map[string]any{
		"direction":  "download",
		"url":        "https://artifacts.example.test/v1/grants/grant-1",
		"digest":     "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"size":       float64(128),
		"media_type": "application/gzip",
		"expires_at": artifactExpiry.Format(time.RFC3339Nano),
	}
	if !reflect.DeepEqual(artifact, wantArtifact) {
		t.Fatalf("artifact wire = %#v", artifact)
	}
}

func TestArtifactReferenceBoundsAndSecretsStayProviderOnly(t *testing.T) {
	valid := ArtifactReference{
		Direction: ArtifactDirectionUpload,
		URL:       "https://artifacts.example.test/v1/grants/grant-2",
		Token:     "opaque-artifact-secret",
		Digest:    "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Size:      1024,
		MediaType: "application/zip",
		ExpiresAt: time.Now().Add(30 * time.Second).UTC(),
	}
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal artifact reference: %v", err)
	}
	if strings.Contains(string(encoded), valid.Token) || strings.Contains(string(encoded), valid.URL) {
		t.Fatalf("provider-only artifact leaked via default JSON: %s", encoded)
	}

	mutations := map[string]func(*ArtifactReference){
		"direction":      func(value *ArtifactReference) { value.Direction = ArtifactDirection("read") },
		"plain http":     func(value *ArtifactReference) { value.URL = "http://artifacts.example.test/grant" },
		"url credential": func(value *ArtifactReference) { value.URL = "https://user:pass@artifacts.example.test/grant" },
		"query token":    func(value *ArtifactReference) { value.URL += "?token=secret" },
		"token control":  func(value *ArtifactReference) { value.Token = "secret\nleak" },
		"digest":         func(value *ArtifactReference) { value.Digest = "sha256:bad" },
		"size":           func(value *ArtifactReference) { value.Size = MaxLogicalFileSizeBytes + 1 },
		"media type":     func(value *ArtifactReference) { value.MediaType = "not-a-media-type" },
		"expired":        func(value *ArtifactReference) { value.ExpiresAt = time.Now().Add(-time.Second) },
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			calls := 0
			provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("unexpected transport")
			}))
			request := validExecuteRequest()
			candidate := valid
			candidate.ExpiresAt = time.Now().Add(30 * time.Second).UTC()
			mutate(&candidate)
			request.ArtifactReferences = []ArtifactReference{candidate}
			_, err := provider.Execute(context.Background(), request)
			if !errors.Is(err, domainsandbox.ErrInvalidInput) || calls != 0 {
				t.Fatalf("invalid artifact error/calls = %v/%d", err, calls)
			}
			if strings.Contains(err.Error(), candidate.Token) || strings.Contains(err.Error(), "artifacts.example.test") {
				t.Fatalf("artifact validation leaked secret: %v", err)
			}
		})
	}
}
