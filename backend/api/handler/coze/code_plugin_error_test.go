// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appPlugin "github.com/coze-dev/coze-studio/backend/application/plugin"
)

func TestCodePluginErrorResponseContractAndRedaction(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "invalid", err: appPlugin.ErrCodePluginInvalidRequest, statusCode: http.StatusBadRequest, code: "code_plugin_invalid_request"},
		{name: "permission", err: appPlugin.ErrCodePluginPermission, statusCode: http.StatusForbidden, code: "code_plugin_forbidden"},
		{name: "validation", err: appPlugin.ErrCodePluginValidation, statusCode: http.StatusUnprocessableEntity, code: "code_plugin_validation_failed"},
		{name: "conflict", err: appPlugin.ErrCodePluginConflict, statusCode: http.StatusConflict, code: "code_plugin_conflict"},
		{name: "unavailable", err: appPlugin.ErrCodePluginUnavailable, statusCode: http.StatusServiceUnavailable, code: "code_plugin_unavailable"},
		{name: "unknown internal", err: fmt.Errorf("unexpected internal failure"), statusCode: http.StatusInternalServerError, code: "code_plugin_internal"},
		{name: "wrapped invalid", err: fmt.Errorf("%w: internal=TOP_SECRET", appPlugin.ErrCodePluginInvalidRequest), statusCode: http.StatusBadRequest, code: "code_plugin_invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := server.Default()
			h.GET("/error", func(ctx context.Context, c *app.RequestContext) {
				codePluginErrorResponse(ctx, c, fmt.Errorf("%w: password=TOP_SECRET", test.err))
			})
			response := ut.PerformRequest(h.Engine, http.MethodGet, "/error", nil)
			body := string(response.Result().Body())
			require.Equal(t, test.statusCode, response.Code)
			require.Contains(t, body, test.code)
			require.NotContains(t, body, "TOP_SECRET")
			require.NotContains(t, body, "password")
		})
	}
}
