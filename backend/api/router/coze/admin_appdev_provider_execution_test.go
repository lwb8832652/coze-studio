// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/middleware"
	appdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	typeconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

type adminProviderQuarantineControlStub struct {
	calls   int
	actor   appdev.ProviderQuarantineDispositionActor
	request appdev.ProviderQuarantineDispositionRequest
}

func (control *adminProviderQuarantineControlStub) Dispose(
	_ context.Context,
	actor appdev.ProviderQuarantineDispositionActor,
	request appdev.ProviderQuarantineDispositionRequest,
) (*appdev.ProviderQuarantineDispositionProjection, error) {
	control.calls++
	control.actor = actor
	control.request = request
	return &appdev.ProviderQuarantineDispositionProjection{
		Generation: 7, Version: 12, State: domainappdev.ProviderExecutionLaunchAborted,
		Disposition:     domainappdev.ProviderExecutionQuarantineProviderAbsent,
		CleanupRequired: true,
	}, nil
}

func TestAdminProviderQuarantineDispositionRouteRequiresSystemAdminAndPublishesSafeContract(t *testing.T) {
	previousFactory := adminAuthMiddlewareFactory
	adminAuthMiddlewareFactory = func() app.HandlerFunc {
		return middleware.AdminAuthMWWithEmailLoader(func(context.Context) (string, error) {
			return "admin@example.test", nil
		})
	}
	t.Cleanup(func() { adminAuthMiddlewareFactory = previousFactory })

	control := &adminProviderQuarantineControlStub{}
	publication, err := appdev.PublishProviderQuarantineDispositionControl(control)
	require.NoError(t, err)
	t.Cleanup(func() { appdev.UnpublishProviderQuarantineDispositionControl(publication) })

	h := server.Default(server.WithStreamBody(true), server.WithRedirectTrailingSlash(false))
	installAdminProviderExecutionTestSessionMiddleware(h)
	require.NoError(t, RegisterAdminAppDevProviderExecutionRoutes(h))
	path := "/api/admin/app-dev/provider-executions/1001/project-a/7/quarantine-disposition"
	body := `{"expected_version":11,"expected_state":"quarantined","idempotency_key":"operator-disposition-1","acknowledgement":"provider_absent","reason":"provider_absence_confirmed","evidence_hash":"125c9e7f0d7d31dbaac8f21843892d98385c5d00ed408bc0108c4af1bb6d1e92"}`

	unauthenticated := ut.PerformRequest(h.Engine, http.MethodPost, path, &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	require.Zero(t, control.calls)

	member := performAdminSandboxRouteRequestWithServer(h, "member@example.test", http.MethodPost, path, body)
	require.Equal(t, http.StatusForbidden, member.Code)
	require.Zero(t, control.calls)

	admin := performAdminSandboxRouteRequestWithServer(h, "admin@example.test", http.MethodPost, path, body)
	require.Equal(t, http.StatusOK, admin.Code, string(admin.Result().Body()))
	require.Equal(t, 1, control.calls)
	require.True(t, control.actor.SystemAdmin)
	require.Positive(t, control.actor.UserID)
	require.Equal(t, "1001", control.request.SpaceID)
	require.Equal(t, "project-a", control.request.ProjectID)
	require.Equal(t, uint64(7), control.request.Generation)
	require.JSONEq(t, `{"code":0,"msg":"success","data":{"generation":7,"version":12,"state":"aborted","disposition":"provider_absent","cleanup_required":true}}`, string(admin.Result().Body()))
	for _, forbidden := range []string{"operator-disposition-1", "125c9e7f", "provider-a", "checkpoint", "token", "credential"} {
		require.NotContains(t, string(admin.Result().Body()), forbidden)
	}
}

func TestAdminProviderQuarantineDispositionRouteFailsClosedWithoutPublicationAndOnInvalidCAS(t *testing.T) {
	previousFactory := adminAuthMiddlewareFactory
	adminAuthMiddlewareFactory = func() app.HandlerFunc {
		return middleware.AdminAuthMWWithEmailLoader(func(context.Context) (string, error) {
			return "admin@example.test", nil
		})
	}
	t.Cleanup(func() { adminAuthMiddlewareFactory = previousFactory })
	h := server.Default(server.WithStreamBody(true))
	installAdminProviderExecutionTestSessionMiddleware(h)
	require.NoError(t, RegisterAdminAppDevProviderExecutionRoutes(h))
	path := "/api/admin/app-dev/provider-executions/1001/project-a/7/quarantine-disposition"
	body := `{"expected_version":11,"expected_state":"quarantined","idempotency_key":"operator-disposition-1","acknowledgement":"provider_absent","reason":"provider_absence_confirmed","evidence_hash":"125c9e7f0d7d31dbaac8f21843892d98385c5d00ed408bc0108c4af1bb6d1e92"}`
	response := performAdminSandboxRouteRequestWithServer(h, "admin@example.test", http.MethodPost, path, body)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, string(response.Result().Body()), `"error_code":"PROVIDER_EXECUTION_UNAVAILABLE"`)

	publication, err := appdev.PublishProviderQuarantineDispositionControl(&adminProviderQuarantineControlStub{})
	require.NoError(t, err)
	t.Cleanup(func() { appdev.UnpublishProviderQuarantineDispositionControl(publication) })
	invalid := performAdminSandboxRouteRequestWithServer(h, "admin@example.test", http.MethodPost, path, `{"expected_version":0}`)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
}

func installAdminProviderExecutionTestSessionMiddleware(h *server.Hertz) {
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		email := string(c.Request.Header.Peek("X-Test-User-Email"))
		if email != "" {
			ctx = ctxcache.Init(ctx)
			ctxcache.Store(ctx, typeconsts.SessionDataKeyInCtx, &userentity.Session{
				UserID: 42, UserEmail: email,
			})
		}
		c.Next(ctx)
	})
}

func performAdminSandboxRouteRequestWithServer(
	h *server.Hertz,
	email string,
	method string,
	path string,
	body string,
) *ut.ResponseRecorder {
	headers := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
	if email != "" {
		headers = append(headers, ut.Header{Key: "X-Test-User-Email", Value: email})
	}
	return ut.PerformRequest(h.Engine, method, path, &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}, headers...)
}
