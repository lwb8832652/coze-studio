// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

func RequestBodyCompatibilityMW(maxBodyBytes int) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		request := &c.Request
		if !request.IsBodyStream() || isAdminSandboxStreamRequest(c) {
			c.Next(ctx)
			return
		}
		if maxBodyBytes <= 0 {
			abortRequestBody(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}
		if contentLength := request.Header.ContentLength(); contentLength > maxBodyBytes {
			_ = request.CloseBodyStream()
			abortRequestBody(c, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE", "request body too large")
			return
		}
		body, readErr := io.ReadAll(io.LimitReader(request.BodyStream(), int64(maxBodyBytes)+1))
		closeErr := request.CloseBodyStream()
		if len(body) > maxBodyBytes {
			abortRequestBody(c, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE", "request body too large")
			return
		}
		if readErr != nil || closeErr != nil {
			abortRequestBody(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
			return
		}
		request.SetBody(body)
		c.Next(ctx)
	}
}

func isAdminSandboxStreamRequest(c *app.RequestContext) bool {
	method := string(c.Request.Header.Method())
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodDelete {
		return false
	}
	path := string(c.Request.URI().Path())
	return path == "/api/admin/sandboxes" || strings.HasPrefix(path, "/api/admin/sandboxes/")
}

func abortRequestBody(c *app.RequestContext, status int, code, message string) {
	c.AbortWithStatusJSON(status, map[string]any{"code": status, "error_code": code, "msg": message})
}
