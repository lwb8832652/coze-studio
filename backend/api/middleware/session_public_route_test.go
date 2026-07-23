// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

func TestSessionAuthAllowsPublicSiteConfigurationWithoutSession(t *testing.T) {
	h := server.Default()
	h.Use(RequestInspectorMW())
	h.Use(SessionAuthMWWithValidator(func(context.Context, string) (*userentity.Session, error) {
		t.Fatal("public site configuration must not validate a session")
		return nil, nil
	}))
	h.GET("/api/site/config", func(_ context.Context, c *app.RequestContext) {
		c.String(http.StatusOK, "ok")
	})

	response := ut.PerformRequest(h.Engine, http.MethodGet, "/api/site/config", nil)

	require.Equal(t, http.StatusOK, response.Code, string(response.Result().Body()))
	require.Equal(t, "ok", string(response.Result().Body()))
}
