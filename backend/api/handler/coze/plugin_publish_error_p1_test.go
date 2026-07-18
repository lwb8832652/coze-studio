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

	pluginservice "github.com/coze-dev/coze-studio/backend/domain/plugin/service"
)

func TestPublishProjectErrorResponseMapsAmbiguousCodePluginPublishTo503(t *testing.T) {
	h := server.Default()
	h.GET("/error", func(ctx context.Context, c *app.RequestContext) {
		publishProjectErrorResponse(
			ctx,
			c,
			fmt.Errorf("wrapped: %w: TOP_SECRET", pluginservice.ErrAPPCodePluginPublishUnavailable),
		)
	})

	response := ut.PerformRequest(h.Engine, http.MethodGet, "/error", nil)
	body := string(response.Result().Body())

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, body, "plugin_publish_unavailable")
	require.NotContains(t, body, "TOP_SECRET")
}
