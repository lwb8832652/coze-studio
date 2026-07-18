// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

func TestCodePluginHandlersSanitizeBindErrors(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		path    string
		handler app.HandlerFunc
	}{
		{name: "get draft", path: "/api/plugin_api/get_code_plugin_draft", handler: GetCodePluginDraft},
		{name: "save draft", path: "/api/plugin_api/save_code_plugin_draft", handler: SaveCodePluginDraft},
		{name: "debug", path: "/api/plugin_api/debug_code_plugin", handler: DebugCodePlugin},
		{name: "get version", path: "/api/plugin_api/get_code_plugin_version", handler: GetCodePluginVersion},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			h := server.New()
			h.POST(testCase.path, testCase.handler)
			body := []byte(`{"plugin_id":"TOP_SECRET_BIND_VALUE"`)
			response := ut.PerformRequest(
				h.Engine,
				http.MethodPost,
				testCase.path,
				&ut.Body{Body: bytes.NewReader(body), Len: len(body)},
				ut.Header{Key: "Content-Type", Value: "application/json"},
			)
			responseBody := string(response.Result().Body())
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Equal(t, "invalid code plugin request", responseBody)
			require.NotContains(t, responseBody, "TOP_SECRET_BIND_VALUE")
			require.NotContains(t, responseBody, "unexpected end")
		})
	}
}
