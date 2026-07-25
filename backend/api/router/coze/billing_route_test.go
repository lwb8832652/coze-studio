// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/middleware"
)

func TestRegisterIncludesAdminBillingRoutes(t *testing.T) {
	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/billing/overview"},
		{http.MethodGet, "/api/admin/billing/config"},
		{http.MethodPut, "/api/admin/billing/config"},
		{http.MethodGet, "/api/admin/billing/plans"},
		{http.MethodPost, "/api/admin/billing/plans"},
		{http.MethodGet, "/api/admin/billing/credit-packages"},
		{http.MethodPost, "/api/admin/billing/credit-packages"},
		{http.MethodGet, "/api/admin/billing/accounts"},
		{http.MethodGet, "/api/admin/billing/ledger"},
		{http.MethodGet, "/api/admin/billing/orders"},
		{http.MethodGet, "/api/admin/billing/model-prices"},
		{http.MethodPost, "/api/admin/billing/model-prices"},
		{http.MethodGet, "/api/admin/billing/usage-monitoring"},
		{http.MethodPost, "/api/admin/billing/adjustments"},
		{http.MethodGet, "/api/admin/billing/credit-thresholds"},
		{http.MethodPut, "/api/admin/billing/credit-thresholds"},
		{http.MethodPost, "/api/admin/billing/maintenance/run"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := ut.PerformRequest(h.Engine, route.method, route.path, nil)
			require.NotEqual(t, http.StatusNotFound, response.Code)
		})
	}
}

func TestAdminBillingCreditThresholdRoutesRequireSystemAdmin(t *testing.T) {
	previousFactory := adminAuthMiddlewareFactory
	adminAuthMiddlewareFactory = func() app.HandlerFunc {
		return middleware.AdminAuthMWWithEmailLoader(func(context.Context) (string, error) {
			return "admin@example.test", nil
		})
	}
	t.Cleanup(func() { adminAuthMiddlewareFactory = previousFactory })

	h := server.Default(server.WithStreamBody(true))
	installAdminProviderExecutionTestSessionMiddleware(h)
	Register(h)
	RegisterCustomRoutes(h)
	requests := []struct {
		method string
		path   string
		body   string
	}{
		{
			method: http.MethodGet,
			path:   "/api/admin/billing/credit-thresholds?subject_type=user&subject_id=42",
		},
		{
			method: http.MethodPut,
			path:   "/api/admin/billing/credit-thresholds",
			body:   `{"subject_type":"user","subject_id":"42","enabled":false,"expected_version":1}`,
		},
	}
	for _, request := range requests {
		t.Run(request.method, func(t *testing.T) {
			unauthenticated := performAdminSandboxRouteRequestWithServer(
				h,
				"",
				request.method,
				request.path,
				request.body,
			)
			require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

			member := performAdminSandboxRouteRequestWithServer(
				h,
				"member@example.test",
				request.method,
				request.path,
				request.body,
			)
			require.Equal(t, http.StatusForbidden, member.Code)

			admin := performAdminSandboxRouteRequestWithServer(
				h,
				"admin@example.test",
				request.method,
				request.path,
				request.body,
			)
			require.NotEqual(t, http.StatusUnauthorized, admin.Code)
			require.NotEqual(t, http.StatusForbidden, admin.Code)
		})
	}
}

func TestRegisterIncludesUserBillingRoutes(t *testing.T) {
	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/billing/account"},
		{http.MethodGet, "/api/billing/config"},
		{http.MethodGet, "/api/billing/subscription"},
		{http.MethodGet, "/api/billing/plans"},
		{http.MethodGet, "/api/billing/credit-packages"},
		{http.MethodGet, "/api/billing/ledger"},
		{http.MethodGet, "/api/billing/usage"},
		{http.MethodGet, "/api/billing/orders"},
		{http.MethodPost, "/api/billing/orders"},
		{http.MethodPost, "/api/billing/orders/order-1/checkout"},
		{http.MethodPost, "/api/billing/payment/callbacks/test"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := ut.PerformRequest(h.Engine, route.method, route.path, nil)
			require.NotEqual(t, http.StatusNotFound, response.Code)
		})
	}
}
