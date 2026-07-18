// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/middleware"
)

func TestTask9AAdminProjectionRoutesKeepServerSideAdminGuard(t *testing.T) {
	previousFactory := adminAuthMiddlewareFactory
	adminAuthMiddlewareFactory = func() app.HandlerFunc {
		return middleware.AdminAuthMWWithEmailLoader(func(context.Context) (string, error) {
			return "admin@example.test", nil
		})
	}
	t.Cleanup(func() { adminAuthMiddlewareFactory = previousFactory })

	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("APP_ENV", "debug")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	paths := []string{
		"/api/admin/sandboxes/defaults", "/api/admin/sandboxes/summary", "/api/admin/sandboxes/capabilities",
	}
	for _, path := range paths {
		unauthenticated := performAdminSandboxRouteRequest("", http.MethodGet, path, "")
		require.Equal(t, http.StatusUnauthorized, unauthenticated.Code, path)
		member := performAdminSandboxRouteRequest("member@example.test", http.MethodGet, path, "")
		require.Equal(t, http.StatusForbidden, member.Code, path)
	}
	capabilities := performAdminSandboxRouteRequest("admin@example.test", http.MethodGet, paths[2], "")
	require.Equal(t, http.StatusOK, capabilities.Code, string(capabilities.Result().Body()))
	require.Contains(t, string(capabilities.Result().Body()), `"control_plane":{"available":true`)
	for _, path := range paths[:2] {
		unavailable := performAdminSandboxRouteRequest("admin@example.test", http.MethodGet, path, "")
		require.Equal(t, http.StatusServiceUnavailable, unavailable.Code, path)
		require.Contains(t, string(unavailable.Result().Body()), `"error_code":"SANDBOX_UNAVAILABLE"`)
	}
}
