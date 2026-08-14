// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	typeconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

var (
	_ adminSandboxService = applicationAdminSandboxService{}
	_ adminSandboxService = (*adminSandboxServiceStub)(nil)
)

func TestAdminSandboxHandlersExposeCompleteSafeContract(t *testing.T) {
	stub := &adminSandboxServiceStub{}
	h := newAdminSandboxTestServer(stub)
	policyJSON := adminSandboxTestPolicyJSON(t)
	createBody := fmt.Sprintf(`{"name":"Primary","type":"remote_http","endpoint":"https://runner.example.test/full/private/path","credential":"credential-must-not-leak ciphertext-must-not-leak","scopes":["agent"],"policy":%s}`, policyJSON)
	updateBody := fmt.Sprintf(`{"expected_version":1,"name":"Primary v2","scopes":["agent","mcp_stdio"],"policy":%s,"endpoint":{"mode":"replace","value":"https://runner-v2.example.test/private"},"credential":{"mode":"clear"}}`, policyJSON)
	credentialBody := `{"expected_version":4,"mode":"replace","credential":"rotated-secret-must-not-leak raw-config-must-not-leak token-must-not-leak fence-must-not-leak"}`

	tests := []struct {
		method string
		path   string
		body   string
		call   string
	}{
		{http.MethodGet, "/api/admin/sandboxes?limit=10&scope=agent", "", "list"},
		{http.MethodPost, "/api/admin/sandboxes", createBody, "create"},
		{http.MethodGet, "/api/admin/sandboxes/17", "", "get"},
		{http.MethodPut, "/api/admin/sandboxes/17", updateBody, "update"},
		{http.MethodDelete, "/api/admin/sandboxes/17", `{"expected_version":2}`, "delete"},
		{http.MethodPost, "/api/admin/sandboxes/17/enable", `{"expected_version":2}`, "enable"},
		{http.MethodPost, "/api/admin/sandboxes/17/disable", `{"expected_version":3}`, "disable"},
		{http.MethodPost, "/api/admin/sandboxes/17/credentials", credentialBody, "credentials"},
		{http.MethodPost, "/api/admin/sandboxes/17/credentials", `{"expected_version":5,"mode":"clear"}`, "credentials"},
		{http.MethodPost, "/api/admin/sandboxes/17/defaults", `{"expected_version":6,"default_expected_version":0,"scope":"agent"}`, "default"},
		{http.MethodPost, "/api/admin/sandboxes/17/health", `{"expected_version":7}`, "health"},
		{http.MethodGet, "/api/admin/sandboxes/17/audit-events?limit=20&action=health_check", "", "audit"},
	}

	responseBodies := make([]string, 0, len(tests))
	for _, test := range tests {
		response := performAdminSandboxRequest(h, test.method, test.path, test.body)
		body := string(response.Result().Body())
		responseBodies = append(responseBodies, body)
		require.Equalf(t, http.StatusOK, response.Code, "%s %s: %s", test.method, test.path, body)
		require.Contains(t, body, `"code":0`)
	}

	require.Equal(t, []string{
		"list", "create", "get", "update", "delete", "enable", "disable",
		"credentials", "credentials", "default", "health", "audit",
	}, stub.calls)
	require.Equal(t, int64(42), stub.actor.UserID)
	require.True(t, stub.actor.SystemAdmin)
	require.True(t, reflect.DeepEqual(
		[]domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAppDev},
		stub.actor.AllowedScopes,
	))
	responseText := strings.Join(responseBodies, " ")
	for _, forbidden := range []string{
		"credential-must-not-leak",
		"ciphertext-must-not-leak",
		"rotated-secret-must-not-leak",
		"raw-config-must-not-leak",
		"token-must-not-leak",
		"fence-must-not-leak",
		"https://runner.example.test/full/private/path",
		"https://runner-v2.example.test/private",
	} {
		require.NotContains(t, responseText, forbidden)
	}
	require.Contains(t, responseText, `"endpoint_hint":"runner...test"`)
	require.Contains(t, responseText, `"credential_configured":true`)
}

func TestAdminSandboxHandlersRejectStrictInputMatrix(t *testing.T) {
	policyJSON := adminSandboxTestPolicyJSON(t)
	validCreate := fmt.Sprintf(`{"name":"x","type":"remote_http","endpoint":"https://runner.example.test/","credential":"x","scopes":["agent"],"policy":%s}`, policyJSON)
	tooLargeBody := strings.Repeat("x", maxAdminSandboxBodyBytes+1)

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		status      int
	}{
		{"missing content type", http.MethodPost, "/api/admin/sandboxes", validCreate, "", http.StatusBadRequest},
		{"wrong content type", http.MethodPost, "/api/admin/sandboxes", validCreate, "text/plain", http.StatusBadRequest},
		{"malformed json", http.MethodPost, "/api/admin/sandboxes", "{", "application/json", http.StatusBadRequest},
		{"invalid utf8", http.MethodPost, "/api/admin/sandboxes", "{\"name\":\"" + string([]byte{0xff}) + "\"}", "application/json", http.StatusBadRequest},
		{"trailing json", http.MethodPost, "/api/admin/sandboxes", validCreate + `{}`, "application/json", http.StatusBadRequest},
		{"unknown actor field", http.MethodPost, "/api/admin/sandboxes", fmt.Sprintf(`{"name":"x","type":"remote_http","endpoint":"https://runner.example.test/","credential":"x","scopes":["agent"],"policy":%s,"user_id":9}`, policyJSON), "application/json", http.StatusBadRequest},
		{"case variant field", http.MethodPost, "/api/admin/sandboxes", fmt.Sprintf(`{"Name":"x","type":"remote_http","endpoint":"https://runner.example.test/","credential":"x","scopes":["agent"],"policy":%s}`, policyJSON), "application/json", http.StatusBadRequest},
		{"duplicate field", http.MethodPost, "/api/admin/sandboxes", fmt.Sprintf(`{"name":"x","name":"y","type":"remote_http","endpoint":"https://runner.example.test/","credential":"x","scopes":["agent"],"policy":%s}`, policyJSON), "application/json", http.StatusBadRequest},
		{"client provider key", http.MethodPost, "/api/admin/sandboxes", fmt.Sprintf(`{"name":"x","type":"remote_http","provider_key":"client-owned","endpoint":"https://runner.example.test/","credential":"x","scopes":["agent"],"policy":%s}`, policyJSON), "application/json", http.StatusBadRequest},
		{"enum case variant", http.MethodPost, "/api/admin/sandboxes", fmt.Sprintf(`{"name":"x","type":"REMOTE_HTTP","endpoint":"https://runner.example.test/","credential":"x","scopes":["agent"],"policy":%s}`, policyJSON), "application/json", http.StatusBadRequest},
		{"id zero", http.MethodGet, "/api/admin/sandboxes/0", "", "", http.StatusBadRequest},
		{"id negative", http.MethodGet, "/api/admin/sandboxes/-1", "", "", http.StatusBadRequest},
		{"id overflow", http.MethodGet, "/api/admin/sandboxes/9223372036854775808", "", "", http.StatusBadRequest},
		{"id text", http.MethodGet, "/api/admin/sandboxes/not-an-id", "", "", http.StatusBadRequest},
		{"version zero", http.MethodPost, "/api/admin/sandboxes/17/enable", `{"expected_version":0}`, "application/json", http.StatusBadRequest},
		{"version negative", http.MethodPost, "/api/admin/sandboxes/17/enable", `{"expected_version":-1}`, "application/json", http.StatusBadRequest},
		{"version overflow", http.MethodPost, "/api/admin/sandboxes/17/enable", `{"expected_version":18446744073709551616}`, "application/json", http.StatusBadRequest},
		{"role injection", http.MethodPost, "/api/admin/sandboxes/17/enable", `{"expected_version":1,"role":"admin"}`, "application/json", http.StatusBadRequest},
		{"limit zero", http.MethodGet, "/api/admin/sandboxes?limit=0", "", "", http.StatusBadRequest},
		{"limit negative", http.MethodGet, "/api/admin/sandboxes?limit=-1", "", "", http.StatusBadRequest},
		{"limit over max", http.MethodGet, fmt.Sprintf("/api/admin/sandboxes?limit=%d", domainsandbox.MaxPageLimit+1), "", "", http.StatusBadRequest},
		{"limit overflow", http.MethodGet, "/api/admin/sandboxes?limit=999999999999999999999999999", "", "", http.StatusBadRequest},
		{"offset negative", http.MethodGet, "/api/admin/sandboxes?offset=-1", "", "", http.StatusBadRequest},
		{"offset over max", http.MethodGet, fmt.Sprintf("/api/admin/sandboxes?offset=%d", domainsandbox.MaxPageOffset+1), "", "", http.StatusBadRequest},
		{"offset overflow", http.MethodGet, "/api/admin/sandboxes?offset=999999999999999999999999999", "", "", http.StatusBadRequest},
		{"page unsupported", http.MethodGet, "/api/admin/sandboxes?page=1", "", "", http.StatusBadRequest},
		{"duplicate query", http.MethodGet, "/api/admin/sandboxes?limit=1&limit=2", "", "", http.StatusBadRequest},
		{"case variant query", http.MethodGet, "/api/admin/sandboxes?Limit=1", "", "", http.StatusBadRequest},
		{"unknown query", http.MethodGet, "/api/admin/sandboxes?actor_id=42", "", "", http.StatusBadRequest},
		{"case variant scope", http.MethodGet, "/api/admin/sandboxes?scope=Agent", "", "", http.StatusBadRequest},
		{"empty replacement", http.MethodPost, "/api/admin/sandboxes/17/credentials", `{"expected_version":1,"mode":"replace","credential":""}`, "application/json", http.StatusBadRequest},
		{"invalid clear", http.MethodPost, "/api/admin/sandboxes/17/credentials", `{"expected_version":1,"mode":"clear","credential":"not-empty"}`, "application/json", http.StatusBadRequest},
		{"invalid default scope", http.MethodPost, "/api/admin/sandboxes/17/defaults", `{"expected_version":1,"default_expected_version":0,"scope":"workspace"}`, "application/json", http.StatusBadRequest},
		{"body limit plus one", http.MethodPost, "/api/admin/sandboxes/17/health", tooLargeBody, "application/json", http.StatusRequestEntityTooLarge},
		{"trailing slash", http.MethodGet, "/api/admin/sandboxes/17/", "", "", http.StatusNotFound},
		{"extra path", http.MethodGet, "/api/admin/sandboxes/17/extra", "", "", http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &adminSandboxServiceStub{}
			h := newAdminSandboxTestServer(stub)
			response := performAdminSandboxRequestWithContentType(h, test.method, test.path, test.body, test.contentType)
			require.Equalf(t, test.status, response.Code, "%s %s: %s", test.method, test.path, response.Result().Body())
			if test.status == http.StatusBadRequest {
				require.Contains(t, string(response.Result().Body()), `"error_code":"SANDBOX_CONFIGURATION_INVALID"`)
			} else if test.status == http.StatusRequestEntityTooLarge {
				require.Contains(t, string(response.Result().Body()), `"error_code":"SANDBOX_REQUEST_TOO_LARGE"`)
			}
			require.Empty(t, stub.calls)
		})
	}
}

func TestAdminSandboxSchedulerSettingsRejectStrictPayloads(t *testing.T) {
	h := newAdminSandboxTestServer(&adminSandboxServiceStub{})
	validSettings, err := json.Marshal(domainsandbox.DefaultSchedulerSettings())
	require.NoError(t, err)
	valid := `{"expected_version":1,"settings":` + string(validSettings) + `}`
	for _, body := range []string{
		valid,
		`{"expected_version":1,"settings":` + string(validSettings) + `,"credential":"forbidden"}`,
		`{"expected_version":1,"ExpectedVersion":1,"settings":` + string(validSettings) + `}`,
		`{"expected_version":1,"settings":{"total_weight":2}}`,
	} {
		response := performAdminSandboxRequest(h, http.MethodPut, "/api/admin/sandboxes/scheduler-settings", body)
		if body == valid {
			require.Equal(t, http.StatusOK, response.Code, string(response.Result().Body()))
		} else {
			require.Equal(t, http.StatusBadRequest, response.Code, string(response.Result().Body()))
		}
	}
}

func TestAdminSandboxRuntimeStatusReturnsSafeUnavailableEnvelope(t *testing.T) {
	h := newAdminSandboxTestServer(&adminSandboxServiceStub{})
	response := performAdminSandboxRequest(h, http.MethodGet, "/api/admin/sandboxes/runtime-status", "")
	require.Equal(t, http.StatusOK, response.Code, string(response.Result().Body()))
	require.Contains(t, string(response.Result().Body()), `"available":false`)
}

func TestAdminSandboxSessionSettingsHandlersExposeStrictSafeContract(t *testing.T) {
	stub := &adminSandboxServiceStub{}
	h := newAdminSandboxTestServer(stub)

	get := performAdminSandboxRequest(h, http.MethodGet, "/api/admin/sandboxes/session-settings", "")
	require.Equal(t, http.StatusOK, get.Code, string(get.Result().Body()))
	require.Contains(t, string(get.Result().Body()), `"core_enabled":false`)

	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.CoreEnabled = true
	encoded, err := json.Marshal(map[string]any{"expected_version": uint64(1), "settings": settings})
	require.NoError(t, err)
	put := performAdminSandboxRequest(h, http.MethodPut, "/api/admin/sandboxes/session-settings", string(encoded))
	require.Equal(t, http.StatusOK, put.Code, string(put.Result().Body()))
	require.Contains(t, string(put.Result().Body()), `"applied_version":2`)

	status := performAdminSandboxRequest(h, http.MethodGet, "/api/admin/sandboxes/session-runtime-status", "")
	require.Equal(t, http.StatusOK, status.Code, string(status.Result().Body()))
	body := string(status.Result().Body())
	require.Contains(t, body, `"runtime_generation":7`)
	require.Contains(t, body, `"transport_encrypted":true`)
	require.NotContains(t, body, "sentinel")
	require.NotContains(t, body, "shell_id")
	require.NotContains(t, body, "endpoint")
	require.Equal(t, []string{"session_get", "session_update", "session_runtime_status"}, stub.calls)
}

func TestAdminSandboxSessionSettingsRejectInteractiveWith422(t *testing.T) {
	h := newAdminSandboxTestServer(&adminSandboxServiceStub{err: appsandbox.ErrInteractiveUnsupported})
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.InteractiveEnabled = true
	encoded, err := json.Marshal(map[string]any{"expected_version": uint64(1), "settings": settings})
	require.NoError(t, err)
	response := performAdminSandboxRequest(h, http.MethodPut, "/api/admin/sandboxes/session-settings", string(encoded))
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, string(response.Result().Body()))
	require.Contains(t, string(response.Result().Body()), `"error_code":"SANDBOX_INTERACTIVE_UNSUPPORTED"`)
}

func TestAdminSandboxSessionSettingsUse64KiBStrictJSONAndStableErrors(t *testing.T) {
	for _, body := range []string{
		`{"expected_version":1,"settings":{},"unknown":true}`,
		`{"expected_version":1,"expected_version":2,"settings":{}}`,
	} {
		response := performAdminSandboxRequest(newAdminSandboxTestServer(&adminSandboxServiceStub{}), http.MethodPut, "/api/admin/sandboxes/session-settings", body)
		require.Equal(t, http.StatusBadRequest, response.Code, string(response.Result().Body()))
	}

	stub := &adminSandboxServiceStub{}
	h := newAdminSandboxTestServer(stub)
	reader := &countingAdminSandboxReader{reader: strings.NewReader(strings.Repeat("x", maxAdminSandboxSessionBodyBytes+64))}
	response := ut.PerformRequest(h.Engine, http.MethodPut, "/api/admin/sandboxes/session-settings", &ut.Body{Body: reader, Len: -1}, ut.Header{Key: "content-type", Value: "application/json"})
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code, string(response.Result().Body()))
	require.Equal(t, maxAdminSandboxSessionBodyBytes+1, reader.bytesRead)
	require.Empty(t, stub.calls)

	stub.err = domainsandbox.ErrVersionConflict
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.CoreEnabled = true
	encoded, _ := json.Marshal(map[string]any{"expected_version": uint64(1), "settings": settings})
	response = performAdminSandboxRequest(h, http.MethodPut, "/api/admin/sandboxes/session-settings", string(encoded))
	require.Equal(t, http.StatusConflict, response.Code, string(response.Result().Body()))
}

func TestAdminSandboxHandlersAcceptBodyAtExactLimit(t *testing.T) {
	stub := &adminSandboxServiceStub{}
	h := newAdminSandboxTestServer(stub)
	body := `{"expected_version":1}`
	body += strings.Repeat(" ", maxAdminSandboxBodyBytes-len(body))

	response := performAdminSandboxRequest(h, http.MethodPost, "/api/admin/sandboxes/17/enable", body)

	require.Equal(t, http.StatusOK, response.Code, string(response.Result().Body()))
	require.Equal(t, []string{"enable"}, stub.calls)
}

func TestAdminSandboxHandlersBoundUnknownAndDeclaredBodyStreams(t *testing.T) {
	for _, test := range []struct {
		name      string
		length    int
		wantReads int
	}{
		{name: "declared length rejects before read", length: maxAdminSandboxBodyBytes + 1, wantReads: 0},
		{name: "unknown length reads only limit plus one", length: -1, wantReads: maxAdminSandboxBodyBytes + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &adminSandboxServiceStub{}
			h := newAdminSandboxTestServer(stub)
			reader := &countingAdminSandboxReader{reader: strings.NewReader(strings.Repeat("x", maxAdminSandboxBodyBytes+64))}
			response := ut.PerformRequest(h.Engine, http.MethodPost, "/api/admin/sandboxes/17/health", &ut.Body{
				Body: reader,
				Len:  test.length,
			}, ut.Header{Key: "content-type", Value: "application/json"})

			require.Equal(t, http.StatusRequestEntityTooLarge, response.Code, string(response.Result().Body()))
			require.Equal(t, test.wantReads, reader.bytesRead)
			require.Empty(t, stub.calls)
			require.NotContains(t, string(response.Result().Body()), "xxxxxxxx")
		})
	}
}

func TestAdminSandboxHandlersMapStableErrorsAndRedactInternalDetails(t *testing.T) {
	stub := &adminSandboxServiceStub{}
	h := newAdminSandboxTestServer(stub)
	tests := []struct {
		err       error
		status    int
		errorCode string
	}{
		{appsandbox.ErrPermissionDenied, http.StatusForbidden, "SANDBOX_POLICY_DENIED"},
		{domainsandbox.ErrProviderNotFound, http.StatusNotFound, "SANDBOX_PROVIDER_NOT_FOUND"},
		{domainsandbox.ErrDefaultMissing, http.StatusNotFound, "SANDBOX_DEFAULT_NOT_FOUND"},
		{domainsandbox.ErrVersionConflict, http.StatusConflict, "SANDBOX_VERSION_CONFLICT"},
		{domainsandbox.ErrProviderInUse, http.StatusConflict, "SANDBOX_PROVIDER_IN_USE"},
		{domainsandbox.ErrUnavailable, http.StatusServiceUnavailable, "SANDBOX_UNAVAILABLE"},
	}
	for _, test := range tests {
		stub.err = test.err
		response := performAdminSandboxRequest(h, http.MethodGet, "/api/admin/sandboxes/17", "")
		require.Equal(t, test.status, response.Code)
		require.Contains(t, string(response.Result().Body()), `"error_code":"`+test.errorCode+`"`)
	}

	stub.err = errors.New("mysql password=secret raw-provider-body https://private.example.test")
	response := performAdminSandboxRequest(h, http.MethodGet, "/api/admin/sandboxes/17", "")
	body := string(response.Result().Body())
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Contains(t, body, `"error_code":"SANDBOX_INTERNAL"`)
	require.NotContains(t, body, "password")
	require.NotContains(t, body, "raw-provider-body")
	require.NotContains(t, body, "private.example.test")
}

type adminSandboxServiceStub struct {
	err   error
	calls []string
	actor appsandbox.Actor
}

type countingAdminSandboxReader struct {
	reader    io.Reader
	bytesRead int
}

func (r *countingAdminSandboxReader) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	r.bytesRead += count
	return count, err
}

func (s *adminSandboxServiceStub) record(call string, actor appsandbox.Actor) error {
	s.calls = append(s.calls, call)
	s.actor = actor
	return s.err
}

func (s *adminSandboxServiceStub) List(_ context.Context, actor appsandbox.Actor, _ appsandbox.ListProvidersRequest) (*appsandbox.ListProvidersResult, error) {
	err := s.record("list", actor)
	return &appsandbox.ListProvidersResult{Items: []appsandbox.ProviderDTO{*adminSandboxSafeProvider()}, Total: 1}, err
}

func (s *adminSandboxServiceStub) Create(_ context.Context, actor appsandbox.Actor, _ appsandbox.CreateProviderRequest) (*appsandbox.ProviderDTO, error) {
	err := s.record("create", actor)
	return adminSandboxSafeProvider(), err
}

func (s *adminSandboxServiceStub) Get(_ context.Context, actor appsandbox.Actor, _ int64) (*appsandbox.ProviderDTO, error) {
	err := s.record("get", actor)
	return adminSandboxSafeProvider(), err
}

func (s *adminSandboxServiceStub) Update(_ context.Context, actor appsandbox.Actor, _ appsandbox.UpdateProviderRequest) (*appsandbox.ProviderDTO, error) {
	err := s.record("update", actor)
	return adminSandboxSafeProvider(), err
}

func (s *adminSandboxServiceStub) Delete(_ context.Context, actor appsandbox.Actor, _ appsandbox.DeleteProviderRequest) (*appsandbox.ProviderMutationResult, error) {
	err := s.record("delete", actor)
	return &appsandbox.ProviderMutationResult{ProviderID: 17, Version: 3}, err
}

func (s *adminSandboxServiceStub) SetStatus(_ context.Context, actor appsandbox.Actor, request appsandbox.SetProviderStatusRequest) (*appsandbox.ProviderDTO, error) {
	call := "disable"
	if request.Status == domainsandbox.ProviderStatusEnabled {
		call = "enable"
	}
	err := s.record(call, actor)
	return adminSandboxSafeProvider(), err
}

func (s *adminSandboxServiceStub) ReplaceCredentials(_ context.Context, actor appsandbox.Actor, _ int64, _ uint64, _ appsandbox.SecretMutation) (*appsandbox.ProviderDTO, error) {
	err := s.record("credentials", actor)
	return adminSandboxSafeProvider(), err
}

func (s *adminSandboxServiceStub) SetDefault(_ context.Context, actor appsandbox.Actor, _ appsandbox.SetProviderDefaultRequest) (*appsandbox.ProviderDefaultDTO, error) {
	err := s.record("default", actor)
	return &appsandbox.ProviderDefaultDTO{Scope: domainsandbox.ScopeAgent, ProviderID: 17, Version: 1}, err
}

func (s *adminSandboxServiceStub) HealthCheck(_ context.Context, actor appsandbox.Actor, _ appsandbox.HealthCheckRequest) (*appsandbox.ProviderDTO, error) {
	err := s.record("health", actor)
	return adminSandboxSafeProvider(), err
}

func (s *adminSandboxServiceStub) ListAuditEvents(_ context.Context, actor appsandbox.Actor, _ appsandbox.ListAuditEventsRequest) (*appsandbox.ListAuditEventsResult, error) {
	err := s.record("audit", actor)
	return &appsandbox.ListAuditEventsResult{Items: []appsandbox.AuditEventDTO{{
		ID: 1, ProviderID: 17, ActorUserID: actor.UserID, Action: "health_check", Result: "success",
		RequestID: "request-1", Metadata: map[string]string{"status": "healthy"},
	}}, Total: 1}, err
}

func (s *adminSandboxServiceStub) GetSchedulerSettings(_ context.Context, actor appsandbox.Actor) (*appsandbox.SchedulerSettingsDTO, error) {
	err := s.record("scheduler_get", actor)
	return &appsandbox.SchedulerSettingsDTO{Version: 1, Settings: domainsandbox.DefaultSchedulerSettings()}, err
}

func (s *adminSandboxServiceStub) UpdateSchedulerSettings(_ context.Context, actor appsandbox.Actor, request appsandbox.UpdateSchedulerSettingsRequest) (*appsandbox.SchedulerSettingsUpdateResult, error) {
	err := s.record("scheduler_update", actor)
	return &appsandbox.SchedulerSettingsUpdateResult{Version: request.ExpectedVersion + 1, Settings: request.Settings, Applied: true}, err
}

func (s *adminSandboxServiceStub) GetRuntimeStatus(_ context.Context, actor appsandbox.Actor) (*appsandbox.SchedulerRuntimeStatusDTO, error) {
	err := s.record("runtime_status", actor)
	return &appsandbox.SchedulerRuntimeStatusDTO{ReasonCode: appsandbox.SchedulerReasonProviderUnavailable}, err
}

func (s *adminSandboxServiceStub) GetSessionSettings(_ context.Context, actor appsandbox.Actor) (*appsandbox.SessionSettingsDTO, error) {
	err := s.record("session_get", actor)
	return &appsandbox.SessionSettingsDTO{Version: 1, Settings: domainsandbox.DefaultSessionRuntimeSettings()}, err
}

func (s *adminSandboxServiceStub) UpdateSessionSettings(_ context.Context, actor appsandbox.Actor, request appsandbox.UpdateSessionSettingsRequest) (*appsandbox.SessionSettingsUpdateResult, error) {
	err := s.record("session_update", actor)
	return &appsandbox.SessionSettingsUpdateResult{Version: request.ExpectedVersion + 1, Settings: request.Settings, Applied: true, AppliedVersion: request.ExpectedVersion + 1}, err
}

func (s *adminSandboxServiceStub) GetSessionRuntimeStatus(_ context.Context, actor appsandbox.Actor) (*appsandbox.SessionRuntimeStatusDTO, error) {
	err := s.record("session_runtime_status", actor)
	return &appsandbox.SessionRuntimeStatusDTO{Available: true, DesiredConfigVersion: 2, AppliedConfigVersion: 2, RuntimeGeneration: 7, CoreEnabled: true, RawAIOReady: true, GenerationState: "ready", TotalWeight: 2, TransportEncrypted: true}, err
}

func adminSandboxSafeProvider() *appsandbox.ProviderDTO {
	return &appsandbox.ProviderDTO{
		ID:                    17,
		Name:                  "Primary",
		Type:                  domainsandbox.ProviderTypeRemoteHTTP,
		EndpointHint:          "runner...test",
		CredentialConfigured:  true,
		CredentialFingerprint: "sha256:bounded-fingerprint",
		Active:                true,
		Scopes:                []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 60, MemoryLimitMB: 512, CPULimit: 1, MaxOutputBytes: 64 * 1024, MaxConcurrency: 8,
		},
		Status:  domainsandbox.ProviderStatusEnabled,
		Version: 7,
	}
}

func newAdminSandboxTestServer(service adminSandboxService) *server.Hertz {
	h := server.Default(server.WithRedirectTrailingSlash(false), server.WithStreamBody(true))
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		ctx = ctxcache.Init(ctx)
		ctxcache.Store(ctx, typeconsts.SessionDataKeyInCtx, &userentity.Session{UserID: 42, UserEmail: "admin@example.test"})
		c.Next(ctx)
	})
	handler := newAdminSandboxHandler(service)
	h.GET("/api/admin/sandboxes", handler.list)
	h.POST("/api/admin/sandboxes", handler.create)
	h.GET("/api/admin/sandboxes/:id", handler.get)
	h.PUT("/api/admin/sandboxes/:id", handler.update)
	h.DELETE("/api/admin/sandboxes/:id", handler.delete)
	h.POST("/api/admin/sandboxes/:id/enable", handler.enable)
	h.POST("/api/admin/sandboxes/:id/disable", handler.disable)
	h.POST("/api/admin/sandboxes/:id/credentials", handler.credentials)
	h.POST("/api/admin/sandboxes/:id/defaults", handler.setDefault)
	h.POST("/api/admin/sandboxes/:id/health", handler.health)
	h.GET("/api/admin/sandboxes/:id/audit-events", handler.auditEvents)
	h.GET("/api/admin/sandboxes/scheduler-settings", handler.schedulerSettings)
	h.PUT("/api/admin/sandboxes/scheduler-settings", handler.updateSchedulerSettings)
	h.GET("/api/admin/sandboxes/runtime-status", handler.runtimeStatus)
	h.GET("/api/admin/sandboxes/session-settings", handler.sessionSettings)
	h.PUT("/api/admin/sandboxes/session-settings", handler.updateSessionSettings)
	h.GET("/api/admin/sandboxes/session-runtime-status", handler.sessionRuntimeStatus)
	return h
}

func performAdminSandboxRequest(h *server.Hertz, method, path, body string) *ut.ResponseRecorder {
	contentType := ""
	if body != "" {
		contentType = "application/json"
	}
	return performAdminSandboxRequestWithContentType(h, method, path, body, contentType)
}

func performAdminSandboxRequestWithContentType(h *server.Hertz, method, path, body, contentType string) *ut.ResponseRecorder {
	var requestBody *ut.Body
	if body != "" {
		requestBody = &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
	}
	headers := make([]ut.Header, 0, 1)
	if contentType != "" {
		headers = append(headers, ut.Header{Key: "content-type", Value: contentType})
	}
	return ut.PerformRequest(h.Engine, method, path, requestBody, headers...)
}

func adminSandboxTestPolicyJSON(t *testing.T) string {
	t.Helper()
	value, err := json.Marshal(domainsandbox.RuntimePolicy{
		TimeoutSeconds: 60,
		MemoryLimitMB:  512,
		CPULimit:       1,
		MaxOutputBytes: 64 * 1024,
		MaxConcurrency: 8,
	})
	require.NoError(t, err)
	return string(value)
}
