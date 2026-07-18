// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestTask9AAdminProjectionEndpointsAndHealthFilter(t *testing.T) {
	base := &adminSandboxServiceStub{}
	stub := &task9AAdminSandboxServiceStub{adminSandboxServiceStub: base}
	h := newAdminSandboxTestServer(stub)
	handler := newAdminSandboxHandler(stub)
	h.GET("/api/admin/sandboxes/defaults", handler.defaults)
	h.GET("/api/admin/sandboxes/summary", handler.summary)
	h.GET("/api/admin/sandboxes/capabilities", handler.capabilities)

	previousGetenv := adminSandboxGetenv
	adminSandboxGetenv = func(key string) string {
		return map[string]string{
			"SANDBOX_CONTROL_PLANE_ENABLED": "true", "APP_ENV": "debug",
			"APP_DEV_HOST_RUNTIME_ENABLED": "true", "APP_DEV_RUNNER_TOKEN": "secret-must-not-leak",
		}[key]
	}
	t.Cleanup(func() { adminSandboxGetenv = previousGetenv })

	responses := []string{}
	for _, path := range []string{
		"/api/admin/sandboxes?health=unhealthy", "/api/admin/sandboxes/defaults",
		"/api/admin/sandboxes/summary", "/api/admin/sandboxes/capabilities",
	} {
		response := performAdminSandboxRequest(h, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, response.Code, string(response.Result().Body()))
		responses = append(responses, string(response.Result().Body()))
	}
	require.Equal(t, domainsandbox.HealthStatusUnhealthy, stub.listRequest.Health)
	combined := strings.Join(responses, " ")
	require.Contains(t, combined, `"provider_name":"Primary"`)
	require.Contains(t, combined, `"version":3`)
	require.Contains(t, combined, `"unhealthy":2`)
	require.Contains(t, combined, `"local_debug":{"available":true`)
	require.Contains(t, combined, `"message":"bounded health message"`)
	for _, forbidden := range []string{"secret-must-not-leak", "endpoint_secret", "credential_secret", "provider_key", "runner token"} {
		require.NotContains(t, combined, forbidden)
	}
}

func TestTask9AAdminPolicyAcceptsSafeMetadataAndRejectsHostPaths(t *testing.T) {
	stub := &task9AAdminSandboxServiceStub{adminSandboxServiceStub: &adminSandboxServiceStub{}}
	h := newAdminSandboxTestServer(stub)
	safeBody := `{"name":"Primary","type":"remote_http","endpoint":"https://runner.example.test","credential":"credential","scopes":["agent"],"policy":{"timeout_seconds":60,"memory_limit_mb":512,"cpu_limit":1,"max_output_bytes":65536,"max_concurrency":8,"allow_network":false,"network_allowlist":[],"allowed_env_names":["PATH"],"virtual_read_prefixes":["inputs","workspace/src"],"virtual_write_prefixes":["outputs"],"allowed_executables":["node"],"ffi_enabled":true,"node_modules_mode":"approved_directory","node_modules_directory_ref":"node-modules-v1"}}`
	response := performAdminSandboxRequest(h, http.MethodPost, "/api/admin/sandboxes", safeBody)
	require.Equal(t, http.StatusOK, response.Code, string(response.Result().Body()))
	require.Equal(t, []string{"PATH"}, stub.createRequest.Policy.AllowedEnvNames)
	require.Equal(t, domainsandbox.NodeModulesModeApprovedDirectory, stub.createRequest.Policy.NodeModulesMode)

	malicious := strings.Replace(safeBody, `"inputs","workspace/src"`, `"/etc"`, 1)
	response = performAdminSandboxRequest(h, http.MethodPost, "/api/admin/sandboxes", malicious)
	require.Equal(t, http.StatusBadRequest, response.Code, string(response.Result().Body()))
	require.Contains(t, string(response.Result().Body()), `"error_code":"SANDBOX_CONFIGURATION_INVALID"`)
}

func (s *adminSandboxServiceStub) ListDefaults(_ context.Context, actor appsandbox.Actor) (*appsandbox.ListProviderDefaultsResult, error) {
	err := s.record("defaults", actor)
	id := int64(17)
	return &appsandbox.ListProviderDefaultsResult{Items: []appsandbox.ProviderDefaultProjectionDTO{{
		Scope: domainsandbox.ScopeAgent, Configured: true, ProviderID: &id, ProviderName: "Primary", Version: 3, ProviderVersion: 7,
	}}}, err
}

func (s *adminSandboxServiceStub) GetSummary(_ context.Context, actor appsandbox.Actor) (*appsandbox.ProviderSummaryDTO, error) {
	err := s.record("summary", actor)
	return &appsandbox.ProviderSummaryDTO{Total: 12, Enabled: 8, Unhealthy: 2}, err
}

type task9AAdminSandboxServiceStub struct {
	*adminSandboxServiceStub
	listRequest   appsandbox.ListProvidersRequest
	createRequest appsandbox.CreateProviderRequest
}

func (s *task9AAdminSandboxServiceStub) List(ctx context.Context, actor appsandbox.Actor, request appsandbox.ListProvidersRequest) (*appsandbox.ListProvidersResult, error) {
	s.listRequest = request
	result, err := s.adminSandboxServiceStub.List(ctx, actor, request)
	result.Items[0].Health = appsandbox.HealthProjection{Status: domainsandbox.HealthStatusUnhealthy, ReasonCode: "UNHEALTHY", Message: "bounded health message"}
	return result, err
}

func (s *task9AAdminSandboxServiceStub) Create(ctx context.Context, actor appsandbox.Actor, request appsandbox.CreateProviderRequest) (*appsandbox.ProviderDTO, error) {
	s.createRequest = request
	if _, err := domainsandbox.NormalizeRuntimePolicy(request.Policy); err != nil {
		return nil, err
	}
	return s.adminSandboxServiceStub.Create(ctx, actor, request)
}
