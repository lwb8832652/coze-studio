// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
	typeconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestAdminAuthMiddlewareDistinguishesAuthenticationAndAuthorization(t *testing.T) {
	previous := loadAdminAuthEmails
	loadAdminAuthEmails = func(context.Context) (string, error) {
		return "admin@example.test", nil
	}
	t.Cleanup(func() { loadAdminAuthEmails = previous })

	t.Run("unauthenticated is 401", func(t *testing.T) {
		response := performAdminAuthRequest("")
		require.Equal(t, http.StatusUnauthorized, response.Code)
		require.Contains(t, string(response.Result().Body()), `"error_code":"AUTHENTICATION_REQUIRED"`)
	})

	t.Run("authenticated non-admin is 403", func(t *testing.T) {
		response := performAdminAuthRequest("member@example.test")
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Contains(t, string(response.Result().Body()), `"error_code":"ADMIN_PERMISSION_DENIED"`)
	})

	t.Run("system administrator passes", func(t *testing.T) {
		response := performAdminAuthRequest("admin@example.test")
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "ok", string(response.Result().Body()))
	})
}

func TestAdminAuthMiddlewareRedactsConfigurationFailures(t *testing.T) {
	previous := loadAdminAuthEmails
	loadAdminAuthEmails = func(context.Context) (string, error) {
		return "", errors.New("database password=must-not-leak")
	}
	t.Cleanup(func() { loadAdminAuthEmails = previous })

	response := performAdminAuthRequest("admin@example.test")
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.NotContains(t, string(response.Result().Body()), "must-not-leak")
}

func TestAdminAuthMiddlewareUsesOnlyCanonicalExplicitAdminEmails(t *testing.T) {
	previousSource := loadAdminAuthEmailConfig
	t.Cleanup(func() { loadAdminAuthEmailConfig = previousSource })
	t.Setenv(typeconsts.AllowRegistrationEmail, "registration@example.test")

	for _, test := range []struct {
		name         string
		configured   string
		email        string
		wantStatus   int
		wantSymbolic string
	}{
		{name: "registration whitelist is not admin", configured: "", email: "registration@example.test", wantStatus: http.StatusForbidden, wantSymbolic: "ADMIN_PERMISSION_DENIED"},
		{name: "invalid admin list fails closed", configured: "admin@example.test,bad address <", email: "admin@example.test", wantStatus: http.StatusForbidden, wantSymbolic: "ADMIN_PERMISSION_DENIED"},
		{name: "explicit admin is trimmed lowercased and deduplicated", configured: " ADMIN@EXAMPLE.TEST , admin@example.test ", email: "admin@example.test", wantStatus: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			loadAdminAuthEmailConfig = func(context.Context) (string, error) { return test.configured, nil }
			response := performAdminAuthRequest(test.email)
			require.Equal(t, test.wantStatus, response.Code, string(response.Result().Body()))
			if test.wantSymbolic != "" {
				require.Contains(t, string(response.Result().Body()), `"error_code":"`+test.wantSymbolic+`"`)
			}
		})
	}
}

func TestResolveAdminAuthEmailConfigBootstrapsOnlyBeforePersistence(t *testing.T) {
	getenv := func(key string) string {
		require.Equal(t, systemAdminBootstrapEmailsEnv, key)
		return "bootstrap@example.test"
	}

	require.Equal(
		t,
		"bootstrap@example.test",
		resolveAdminAuthEmailConfig("", kvstore.MissingRevision, getenv),
	)
	require.Equal(
		t,
		"database@example.test",
		resolveAdminAuthEmailConfig("database@example.test", kvstore.MissingRevision, getenv),
	)
	require.Empty(t, resolveAdminAuthEmailConfig("", "persisted-revision", getenv))
	require.Equal(
		t,
		"database@example.test",
		resolveAdminAuthEmailConfig("database@example.test", "persisted-revision", getenv),
	)
}

func TestSessionAndAdminAuthMiddlewareProductionOrder(t *testing.T) {
	for _, test := range []struct {
		name         string
		cookie       string
		session      *userentity.Session
		validateErr  error
		wantStatus   int
		wantSymbolic string
	}{
		{name: "missing cookie", wantStatus: http.StatusUnauthorized, wantSymbolic: "AUTHENTICATION_REQUIRED"},
		{name: "invalid session", cookie: "invalid", validateErr: errors.New("session backend secret"), wantStatus: http.StatusUnauthorized, wantSymbolic: "AUTHENTICATION_REQUIRED"},
		{name: "missing session projection", cookie: "missing", wantStatus: http.StatusUnauthorized, wantSymbolic: "AUTHENTICATION_REQUIRED"},
		{name: "ordinary account", cookie: "member", session: &userentity.Session{UserID: 41, UserEmail: "member@example.test"}, wantStatus: http.StatusForbidden, wantSymbolic: "ADMIN_PERMISSION_DENIED"},
		{name: "administrator", cookie: "admin", session: &userentity.Session{UserID: 42, UserEmail: "admin@example.test"}, wantStatus: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := server.Default()
			h.Use(ContextCacheMW())
			h.Use(RequestInspectorMW())
			h.Use(SessionAuthMWWithValidator(func(context.Context, string) (*userentity.Session, error) {
				return test.session, test.validateErr
			}))
			h.Use(AdminAuthMWWithEmailLoader(func(context.Context) (string, error) {
				return "admin@example.test", nil
			}))
			h.GET("/api/admin/protected", func(ctx context.Context, c *app.RequestContext) {
				c.String(http.StatusOK, "ok")
			})
			headers := make([]ut.Header, 0, 1)
			if test.cookie != "" {
				headers = append(headers, ut.Header{Key: "cookie", Value: userentity.SessionKey + "=" + test.cookie})
			}
			response := ut.PerformRequest(h.Engine, http.MethodGet, "/api/admin/protected", nil, headers...)
			require.Equal(t, test.wantStatus, response.Code, string(response.Result().Body()))
			if test.wantSymbolic != "" {
				body := string(response.Result().Body())
				require.Contains(t, body, `"error_code":"`+test.wantSymbolic+`"`)
				require.NotContains(t, body, "backend secret")
			}
		})
	}
}

func performAdminAuthRequest(email string) *ut.ResponseRecorder {
	h := server.Default()
	if email != "" {
		h.Use(func(ctx context.Context, c *app.RequestContext) {
			ctx = ctxcache.Init(ctx)
			ctxcache.Store(ctx, typeconsts.SessionDataKeyInCtx, &userentity.Session{UserID: 42, UserEmail: email})
			c.Next(ctx)
		})
	}
	h.Use(AdminAuthMW())
	h.GET("/api/admin/protected", func(ctx context.Context, c *app.RequestContext) {
		c.String(http.StatusOK, "ok")
	})
	return ut.PerformRequest(h.Engine, http.MethodGet, "/api/admin/protected", nil)
}
