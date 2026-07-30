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

func TestRegisterCustomRoutesPublishesPublicHealthz(t *testing.T) {
	const revision = "0123456789abcdef0123456789abcdef01234567"
	t.Setenv("APP_REVISION", revision)

	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/healthz",
		nil,
	)

	require.Equal(t, http.StatusOK, resp.Code)
	require.JSONEq(t, `{
		"status": "ok",
		"revision": "0123456789abcdef0123456789abcdef01234567"
	}`, resp.Body.String())
}
