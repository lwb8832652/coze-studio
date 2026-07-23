// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
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
		{http.MethodPost, "/api/admin/billing/maintenance/run"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := ut.PerformRequest(h.Engine, route.method, route.path, nil)
			require.NotEqual(t, http.StatusNotFound, response.Code)
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
