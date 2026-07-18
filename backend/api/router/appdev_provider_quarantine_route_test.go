// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

func TestGeneratedRegisterPublishesAdminProviderQuarantineDispositionBehindAdminAuth(t *testing.T) {
	h := server.Default(server.WithStreamBody(true), server.WithRedirectTrailingSlash(false))
	GeneratedRegister(h)

	response := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/app-dev/provider-executions/1001/project-a/7/quarantine-disposition",
		nil,
	)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.NotEqual(t, http.StatusNotFound, response.Code)
}
