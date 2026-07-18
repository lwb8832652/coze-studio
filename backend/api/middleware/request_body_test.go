// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

func TestRequestBodyCompatibilityBuffersLegacyRoutesButPreservesSandboxStream(t *testing.T) {
	h := server.Default(server.WithStreamBody(true))
	h.Use(RequestBodyCompatibilityMW(1024))
	h.POST("/api/legacy", func(ctx context.Context, c *app.RequestContext) {
		require.False(t, c.Request.IsBodyStream())
		c.String(http.StatusOK, string(c.Request.Body()))
	})
	h.POST("/api/admin/sandboxes", func(ctx context.Context, c *app.RequestContext) {
		require.True(t, c.Request.IsBodyStream())
		body, err := io.ReadAll(c.Request.BodyStream())
		require.NoError(t, err)
		c.String(http.StatusOK, string(body))
	})

	for _, path := range []string{"/api/legacy", "/api/admin/sandboxes"} {
		response := ut.PerformRequest(h.Engine, http.MethodPost, path, &ut.Body{Body: bytes.NewBufferString("payload"), Len: 7})
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "payload", string(response.Result().Body()))
	}
}
