// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/middleware"
	rootapplication "github.com/coze-dev/coze-studio/backend/application"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

func TestAdminSandboxRoutesUseSharedAdminAuthAndExposeCompleteContract(t *testing.T) {
	previousFactory := adminAuthMiddlewareFactory
	adminAuthMiddlewareFactory = func() app.HandlerFunc {
		return middleware.AdminAuthMWWithEmailLoader(func(context.Context) (string, error) {
			return "admin@example.test", nil
		})
	}
	t.Cleanup(func() { adminAuthMiddlewareFactory = previousFactory })

	previousService := rootapplication.SandboxSVC
	rootapplication.SandboxSVC = nil
	t.Cleanup(func() { rootapplication.SandboxSVC = previousService })

	t.Run("unauthenticated receives shared symbolic 401", func(t *testing.T) {
		response := performAdminSandboxRouteRequest("", http.MethodGet, "/api/admin/sandboxes", "")
		require.Equal(t, http.StatusUnauthorized, response.Code)
		require.Contains(t, string(response.Result().Body()), `"error_code":"AUTHENTICATION_REQUIRED"`)
	})

	t.Run("authenticated non-admin receives shared symbolic 403", func(t *testing.T) {
		response := performAdminSandboxRouteRequest("member@example.test", http.MethodGet, "/api/admin/sandboxes", "")
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Contains(t, string(response.Result().Body()), `"error_code":"ADMIN_PERMISSION_DENIED"`)
	})

	t.Run("trailing slash remains protected by shared admin auth", func(t *testing.T) {
		response := performAdminSandboxRouteRequest("", http.MethodGet, "/api/admin/sandboxes/", "")
		require.Equal(t, http.StatusUnauthorized, response.Code)
		require.Contains(t, string(response.Result().Body()), `"error_code":"AUTHENTICATION_REQUIRED"`)

		response = performAdminSandboxRouteRequest("member@example.test", http.MethodGet, "/api/admin/sandboxes/", "")
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Contains(t, string(response.Result().Body()), `"error_code":"ADMIN_PERMISSION_DENIED"`)
	})

	t.Run("administrator receives safe 404 for sandbox trailing slash routes", func(t *testing.T) {
		requests := []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/api/admin/sandboxes/"},
			{http.MethodGet, "/api/admin/sandboxes/17/"},
			{http.MethodPost, "/api/admin/sandboxes/17/enable/"},
			{http.MethodGet, "/api/admin/sandboxes/17/audit-events/"},
		}
		for _, request := range requests {
			response := performAdminSandboxRouteRequest("admin@example.test", request.method, request.path, "")
			body := string(response.Result().Body())
			require.Equalf(t, http.StatusNotFound, response.Code, "%s %s: %s", request.method, request.path, body)
			require.JSONEq(t, `{"code":404,"error_code":"NOT_FOUND","msg":"not found"}`, body)
			require.NotContains(t, body, "secret")
			require.NotContains(t, body, "token")
			require.NotContains(t, body, "endpoint")
		}
	})

	policy := fmt.Sprintf(`{"timeout_seconds":60,"memory_limit_mb":512,"cpu_limit":1,"max_output_bytes":65536,"max_concurrency":8}`)
	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/admin/sandboxes", ""},
		{http.MethodPost, "/api/admin/sandboxes", fmt.Sprintf(`{"name":"Primary","type":"remote_http","endpoint":"https://runner.example.test","credential":"secret","scopes":["agent"],"policy":%s}`, policy)},
		{http.MethodGet, "/api/admin/sandboxes/17", ""},
		{http.MethodPut, "/api/admin/sandboxes/17", fmt.Sprintf(`{"expected_version":1,"name":"Primary","scopes":["agent"],"policy":%s}`, policy)},
		{http.MethodDelete, "/api/admin/sandboxes/17", `{"expected_version":1}`},
		{http.MethodPost, "/api/admin/sandboxes/17/enable", `{"expected_version":1}`},
		{http.MethodPost, "/api/admin/sandboxes/17/disable", `{"expected_version":1}`},
		{http.MethodPost, "/api/admin/sandboxes/17/credentials", `{"expected_version":1,"mode":"clear"}`},
		{http.MethodPost, "/api/admin/sandboxes/17/defaults", `{"expected_version":1,"default_expected_version":0,"scope":"agent"}`},
		{http.MethodPost, "/api/admin/sandboxes/17/health", `{"expected_version":1}`},
		{http.MethodGet, "/api/admin/sandboxes/17/audit-events", ""},
	}

	for _, request := range requests {
		response := performAdminSandboxRouteRequest("admin@example.test", request.method, request.path, request.body)
		require.Equalf(t, http.StatusServiceUnavailable, response.Code, "%s %s: %s", request.method, request.path, response.Result().Body())
		require.Contains(t, string(response.Result().Body()), `"error_code":"SANDBOX_UNAVAILABLE"`)
	}
}

func TestAdminSandboxSchedulerRoutesRequireSystemAdmin(t *testing.T) {
	previousFactory := adminAuthMiddlewareFactory
	adminAuthMiddlewareFactory = func() app.HandlerFunc {
		return middleware.AdminAuthMWWithEmailLoader(func(context.Context) (string, error) {
			return "admin@example.test", nil
		})
	}
	t.Cleanup(func() { adminAuthMiddlewareFactory = previousFactory })

	previousService := rootapplication.SandboxSchedulerSVC
	rootapplication.SandboxSchedulerSVC = nil
	t.Cleanup(func() { rootapplication.SandboxSchedulerSVC = previousService })
	for _, request := range []struct {
		path   string
		status int
	}{
		{"/api/admin/sandboxes/scheduler-settings", http.StatusServiceUnavailable},
		{"/api/admin/sandboxes/runtime-status", http.StatusOK},
	} {
		unauthenticated := performAdminSandboxRouteRequest("", http.MethodGet, request.path, "")
		require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
		member := performAdminSandboxRouteRequest("member@example.test", http.MethodGet, request.path, "")
		require.Equal(t, http.StatusForbidden, member.Code)
		administrator := performAdminSandboxRouteRequest("admin@example.test", http.MethodGet, request.path, "")
		require.Equal(t, request.status, administrator.Code, string(administrator.Result().Body()))
	}
}

func performAdminSandboxRouteRequest(email, method, path, body string) *ut.ResponseRecorder {
	h := server.Default(server.WithStreamBody(true))
	h.Use(middleware.ContextCacheMW())
	h.Use(middleware.RequestInspectorMW())
	h.Use(middleware.SessionAuthMWWithValidator(func(context.Context, string) (*userentity.Session, error) {
		return &userentity.Session{UserID: 42, UserEmail: email}, nil
	}))
	Register(h)
	RegisterCustomRoutes(h)

	var requestBody *ut.Body
	headers := make([]ut.Header, 0, 2)
	if email != "" {
		headers = append(headers, ut.Header{Key: "cookie", Value: userentity.SessionKey + "=route-session"})
	}
	if body != "" {
		requestBody = &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
		headers = append(headers, ut.Header{Key: "content-type", Value: "application/json"})
	}
	return ut.PerformRequest(h.Engine, method, path, requestBody, headers...)
}

var _ = domainsandbox.ScopeAgent
