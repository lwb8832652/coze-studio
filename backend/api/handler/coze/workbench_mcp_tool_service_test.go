/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	appmcptool "github.com/coze-dev/coze-studio/backend/application/mcptool"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	entityconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestWorkbenchMCPToolHandlersManageServerAndTestCall(t *testing.T) {
	h := server.Default()
	registerMCPToolHandlerTestRoutes(h)
	installMCPToolTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"space_id":    "1",
		"name":        "browser-tools",
		"description": "Browser automation tools",
		"server_type": "stdio",
		"enabled":     true,
		"config":      `{"command":"npx","args":["-y","@example/browser"]}`,
		"auth":        `{"type":"none"}`,
		"tools": []map[string]any{
			{
				"name":         "search",
				"description":  "Search the web",
				"input_schema": `{"type":"object","properties":{"query":{"type":"string"}}}`,
			},
		},
	})
	require.NoError(t, err)

	created := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/mcp_tools",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	createdBody := string(created.Result().Body())

	require.Equal(t, http.StatusOK, created.Code)
	require.Contains(t, createdBody, `"code":0`)
	require.Contains(t, createdBody, `"server_id":"100"`)
	require.Contains(t, createdBody, `"name":"browser-tools"`)
	require.Contains(t, createdBody, `"name":"search"`)
	require.Contains(t, createdBody, `"health_status":"healthy"`)

	listed := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools?space_id=1", nil)
	listedBody := string(listed.Result().Body())
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listedBody, `"total":1`)
	require.Contains(t, listedBody, `"server_id":"100"`)
	require.Contains(t, listedBody, `"health_status":"healthy"`)

	got := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools/100", nil)
	gotBody := string(got.Result().Body())
	require.Equal(t, http.StatusOK, got.Code)
	require.Contains(t, gotBody, `"server_type":"stdio"`)
	discovered := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/mcp_tools/100/discover", nil)
	require.Equal(t, http.StatusOK, discovered.Code)
	require.Contains(t, string(discovered.Result().Body()), `"name":"search"`)

	testPayload, err := json.Marshal(map[string]any{
		"tool_name": "search",
		"arguments": `{"query":"coze studio"}`,
	})
	require.NoError(t, err)
	tested := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/mcp_tools/100/test_call",
		&ut.Body{Body: bytes.NewBuffer(testPayload), Len: len(testPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	testedBody := string(tested.Result().Body())

	require.Equal(t, http.StatusOK, tested.Code)
	require.Contains(t, testedBody, `"status":"success"`)

	var testResponse struct {
		Data struct {
			Output string `json:"output"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(tested.Result().Body(), &testResponse))
	var output map[string]any
	require.NoError(t, json.Unmarshal([]byte(testResponse.Data.Output), &output))
	require.Equal(t, "completed", output["result"])
	require.NotContains(t, testedBody, "coze studio")

	listed = ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools?space_id=1", nil)
	listedBody = string(listed.Result().Body())
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listedBody, `"health_status":"healthy"`)
	require.Contains(t, listedBody, `"health_latency_ms":`)
	require.NotContains(t, listedBody, "coze studio")
}

func TestWorkbenchMCPToolHandlersMaskAuth(t *testing.T) {
	h := server.Default()
	registerMCPToolHandlerTestRoutes(h)
	installMCPToolTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"space_id":    "1",
		"name":        "secure-tools",
		"description": "Secure MCP tools",
		"server_type": "streamable_http",
		"enabled":     true,
		"config":      `{"url":"https://mcp.example.test"}`,
		"auth":        `{"type":"bearer","token":"secret-token","nested":{"api_key":"secret-key"}}`,
		"tools": []map[string]any{
			{
				"name":         "search",
				"description":  "Search the web",
				"input_schema": `{"type":"object"}`,
			},
		},
	})
	require.NoError(t, err)

	created := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/mcp_tools",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	createdBody := string(created.Result().Body())

	require.Equal(t, http.StatusOK, created.Code)
	require.NotContains(t, createdBody, "secret-token")
	require.NotContains(t, createdBody, "secret-key")
	var createdResponse struct {
		Data struct {
			Auth string `json:"auth"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Result().Body(), &createdResponse))
	require.JSONEq(t, `{"configured":true}`, createdResponse.Data.Auth)

	listed := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools?space_id=1", nil)
	listedBody := string(listed.Result().Body())
	require.Equal(t, http.StatusOK, listed.Code)
	require.NotContains(t, listedBody, "secret-token")
	require.NotContains(t, listedBody, "secret-key")
	var listedResponse struct {
		Data struct {
			Servers []struct {
				Auth string `json:"auth"`
			} `json:"servers"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listed.Result().Body(), &listedResponse))
	require.Len(t, listedResponse.Data.Servers, 1)
	require.JSONEq(t, `{"configured":true}`, listedResponse.Data.Servers[0].Auth)

	got := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools/100", nil)
	gotBody := string(got.Result().Body())
	require.Equal(t, http.StatusOK, got.Code)
	require.NotContains(t, gotBody, "secret-token")
	require.NotContains(t, gotBody, "secret-key")
	var gotResponse struct {
		Data struct {
			Auth string `json:"auth"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(got.Result().Body(), &gotResponse))
	require.JSONEq(t, `{"configured":true}`, gotResponse.Data.Auth)
}

func TestWorkbenchMCPToolHandlersDeleteServer(t *testing.T) {
	h := server.Default()
	registerMCPToolHandlerTestRoutes(h)
	installMCPToolTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"space_id":    "1",
		"name":        "delete-me",
		"description": "Temporary MCP tools",
		"server_type": "stdio",
		"enabled":     true,
		"config":      `{"command":"npx"}`,
		"auth":        `{"type":"bearer","token":"secret-token"}`,
		"tools": []map[string]any{
			{
				"name":         "search",
				"description":  "Search the web",
				"input_schema": `{"type":"object"}`,
			},
		},
	})
	require.NoError(t, err)
	created := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/mcp_tools",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	require.Equal(t, http.StatusOK, created.Code)

	deleted := ut.PerformRequest(h.Engine, http.MethodDelete, "/api/workbench/mcp_tools/100", nil)
	deletedBody := string(deleted.Result().Body())
	require.Equal(t, http.StatusOK, deleted.Code)
	require.Contains(t, deletedBody, `"server_id":"100"`)
	require.NotContains(t, deletedBody, "secret-token")
	var deletedResponse struct {
		Data struct {
			Auth string `json:"auth"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(deleted.Result().Body(), &deletedResponse))
	require.JSONEq(t, `{"configured":true}`, deletedResponse.Data.Auth)

	listed := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools?space_id=1", nil)
	listedBody := string(listed.Result().Body())
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listedBody, `"total":0`)
}

func TestWorkbenchMCPToolHandlersListRegistryEntries(t *testing.T) {
	h := server.Default()
	registerMCPToolHandlerTestRoutes(h)
	installMCPToolTestService(t)

	createPayload, err := json.Marshal(map[string]any{
		"space_id":    "1",
		"name":        "docs-mcp",
		"description": "Documentation MCP tools",
		"server_type": "stdio",
		"enabled":     true,
		"config":      `{"command":"npx"}`,
		"auth":        `{"type":"bearer","token":"secret-token"}`,
		"tools": []map[string]any{
			{
				"name":         "search-docs",
				"description":  "Search docs",
				"input_schema": `{"type":"object","properties":{"query":{"type":"string"}}}`,
			},
		},
	})
	require.NoError(t, err)
	created := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/mcp_tools",
		&ut.Body{Body: bytes.NewBuffer(createPayload), Len: len(createPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	require.Equal(t, http.StatusOK, created.Code)
	discovered := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/mcp_tools/100/discover", nil)
	require.Equal(t, http.StatusOK, discovered.Code)

	testPayload, err := json.Marshal(map[string]any{
		"tool_name": "search-docs",
		"arguments": `{"query":"coze studio"}`,
	})
	require.NoError(t, err)
	tested := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/mcp_tools/100/test_call",
		&ut.Body{Body: bytes.NewBuffer(testPayload), Len: len(testPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	require.Equal(t, http.StatusOK, tested.Code)

	listed := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools/registry_entries?space_id=1", nil)
	body := string(listed.Result().Body())

	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, body, `"name":"mcp_100_search_docs"`)
	require.Contains(t, body, `"tool_name":"search-docs"`)
	require.Contains(t, body, `"health_status":"healthy"`)
	require.Contains(t, body, `"input_schema"`)
	require.NotContains(t, body, "secret-token")
	require.NotContains(t, body, `"command"`)
	require.NotContains(t, body, "coze studio")
}

func TestWorkbenchMCPToolHandlersDiscoverExportAndListAuditEvents(t *testing.T) {
	h := server.Default()
	registerMCPToolHandlerTestRoutes(h)
	installMCPToolTestService(t)
	payload := []byte(`{
		"space_id":"1",
		"name":"secure-http",
		"server_type":"streamable_http",
		"enabled":true,
		"config":"{\"url\":\"https://mcp.example.test\"}",
		"auth":"{\"type\":\"bearer\",\"token\":\"auth-secret\"}",
		"tools":[]
	}`)
	created := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/mcp_tools",
		&ut.Body{Body: bytes.NewBuffer(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	require.Equal(t, http.StatusOK, created.Code)

	discovered := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/mcp_tools/100/discover", nil)
	require.Equal(t, http.StatusOK, discovered.Code)
	require.Contains(t, string(discovered.Result().Body()), `"name":"search"`)
	require.NotContains(t, string(discovered.Result().Body()), "auth-secret")

	exported := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools/100/export", nil)
	exportBody := string(exported.Result().Body())
	require.Equal(t, http.StatusOK, exported.Code)
	require.Equal(t, `attachment; filename="mcp-server-100.json"`, string(exported.Result().Header.Peek("Content-Disposition")))
	require.Contains(t, exportBody, `"name":"secure-http"`)
	require.NotContains(t, exportBody, "auth-secret")
	require.NotContains(t, exportBody, "config-secret")
	require.NotContains(t, exportBody, `"auth"`)

	testPayload := []byte(`{"tool_name":"search","arguments":"{\"query\":\"argument-secret\"}"}`)
	tested := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workbench/mcp_tools/100/test_call",
		&ut.Body{Body: bytes.NewBuffer(testPayload), Len: len(testPayload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	require.Equal(t, http.StatusOK, tested.Code)

	audited := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools/100/audit_events?limit=1", nil)
	auditBody := string(audited.Result().Body())
	require.Equal(t, http.StatusOK, audited.Code)
	require.Contains(t, auditBody, `"status":"success"`)
	require.Contains(t, auditBody, `"actor_id":"7"`)
	require.Contains(t, auditBody, `"tool_name":"search"`)
	for _, forbidden := range []string{
		"argument-secret", "provider-secret", "auth-secret", "config-secret",
		"arguments", "provider_body", `"auth"`, `"config"`,
	} {
		require.NotContains(t, auditBody, forbidden)
	}
}

func TestWorkbenchMCPToolHandlersBindAndMapErrorsWithoutLeakingInternals(t *testing.T) {
	h := server.Default()
	registerMCPToolHandlerTestRoutes(h)
	installMCPToolTestService(t)

	invalid := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools/not-a-number/audit_events?cursor=private", nil)
	require.Equal(t, http.StatusBadRequest, invalid.Code)
	require.Contains(t, string(invalid.Result().Body()), `"msg":"invalid request"`)
	require.NotContains(t, string(invalid.Result().Body()), "not-a-number")
	require.NotContains(t, string(invalid.Result().Body()), "private")

	tests := []struct {
		name       string
		err        error
		statusCode int
		message    string
	}{
		{name: "disabled", err: appmcptool.ErrMCPDisabled, statusCode: http.StatusServiceUnavailable, message: "mcp service disabled"},
		{name: "unauthenticated", err: appmcptool.ErrMCPUnauthenticated, statusCode: http.StatusUnauthorized, message: "authentication required"},
		{name: "forbidden", err: appmcptool.ErrMCPForbidden, statusCode: http.StatusForbidden, message: "resource unavailable"},
		{name: "not found", err: appmcptool.ErrNotFound, statusCode: http.StatusForbidden, message: "resource unavailable"},
		{name: "conflict", err: appmcptool.ErrMCPConflict, statusCode: http.StatusConflict, message: "request conflict"},
		{name: "validation", err: appmcptool.InvalidArgumentErrorf("private validation detail"), statusCode: http.StatusBadRequest, message: "invalid request"},
		{name: "runtime unavailable", err: appmcptool.ErrRuntimeUnavailable, statusCode: http.StatusServiceUnavailable, message: "mcp runtime unavailable"},
		{name: "discovery unavailable", err: appmcptool.ErrCapabilityDiscoveryUnavailable, statusCode: http.StatusServiceUnavailable, message: "mcp runtime unavailable"},
		{name: "internal", err: errors.New("database password=private-internal"), statusCode: http.StatusInternalServerError, message: "internal server error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverWithError := server.Default()
			serverWithError.GET("/error", func(ctx context.Context, c *app.RequestContext) {
				workbenchMCPToolErrorResponse(ctx, c, test.err)
			})
			response := ut.PerformRequest(serverWithError.Engine, http.MethodGet, "/error", nil)
			body := string(response.Result().Body())
			require.Equal(t, test.statusCode, response.Code)
			require.Contains(t, body, test.message)
			require.NotContains(t, body, "private")
			require.NotContains(t, body, test.err.Error())
		})
	}
}

func installMCPToolTestService(t *testing.T) {
	t.Helper()
	previous := appmcptool.SVC
	appmcptool.InitService(&appmcptool.Components{
		Catalog:              appmcptool.NewInMemoryCatalog(),
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  &mcpToolHandlerRoleReader{},
		RuntimeExecutor:      &mcpToolHandlerRuntimeExecutor{},
		CapabilityDiscoverer: &mcpToolHandlerDiscoverer{},
		AuditRepository:      appmcptool.NewInMemoryManagementAuditRepository(),
	})
	t.Cleanup(func() {
		appmcptool.SVC = previous
	})
}

func registerMCPToolHandlerTestRoutes(h *server.Hertz) {
	h.POST("/api/workbench/mcp_tools", withMCPToolHandlerContext(UpsertMCPToolServer))
	h.GET("/api/workbench/mcp_tools", withMCPToolHandlerContext(ListMCPToolServers))
	h.GET("/api/workbench/mcp_tools/registry_entries", withMCPToolHandlerContext(ListMCPToolRegistryEntries))
	h.GET("/api/workbench/mcp_tools/:server_id", withMCPToolHandlerContext(GetMCPToolServer))
	h.DELETE("/api/workbench/mcp_tools/:server_id", withMCPToolHandlerContext(DeleteMCPToolServer))
	h.POST("/api/workbench/mcp_tools/:server_id/test_call", withMCPToolHandlerContext(TestMCPToolCall))
	h.POST("/api/workbench/mcp_tools/:server_id/discover", withMCPToolHandlerContext(DiscoverMCPToolServer))
	h.GET("/api/workbench/mcp_tools/:server_id/export", withMCPToolHandlerContext(ExportMCPToolServer))
	h.GET("/api/workbench/mcp_tools/:server_id/audit_events", withMCPToolHandlerContext(ListMCPToolAuditEvents))
}

func withMCPToolHandlerContext(handler app.HandlerFunc) app.HandlerFunc {
	return func(_ context.Context, c *app.RequestContext) {
		ctx := ctxcache.Init(context.Background())
		ctxcache.Store(ctx, entityconsts.SessionDataKeyInCtx, &userentity.Session{UserID: 7})
		handler(ctx, c)
	}
}

type mcpToolHandlerRoleReader struct{}

func (*mcpToolHandlerRoleReader) GetUserSpaceList(context.Context, int64) ([]*userentity.Space, error) {
	return []*userentity.Space{{ID: 1, RoleType: 1}}, nil
}

type mcpToolHandlerRuntimeExecutor struct{}

func (*mcpToolHandlerRuntimeExecutor) ExecuteMCPTool(
	_ context.Context,
	_ appmcptool.RuntimeToolCall,
) (*appmcptool.RuntimeToolResult, error) {
	return &appmcptool.RuntimeToolResult{
		Status: "success", Output: `{"provider_body":"provider-secret"}`, LatencyMs: 7,
	}, nil
}

type mcpToolHandlerDiscoverer struct{}

func (*mcpToolHandlerDiscoverer) Discover(
	_ context.Context,
	_ appmcptool.MCPServerConnection,
) (*appmcptool.DiscoveredCapabilities, error) {
	return &appmcptool.DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{
		{Name: "search", Description: "Search the web", InputSchema: `{"type":"object"}`},
		{Name: "search-docs", Description: "Search documentation", InputSchema: `{"type":"object"}`},
	}}, nil
}
