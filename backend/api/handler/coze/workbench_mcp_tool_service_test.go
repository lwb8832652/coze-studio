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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appmcptool "github.com/coze-dev/coze-studio/backend/application/mcptool"
)

func TestWorkbenchMCPToolHandlersManageServerAndTestCall(t *testing.T) {
	h := server.Default()
	h.POST("/api/workbench/mcp_tools", UpsertMCPToolServer)
	h.GET("/api/workbench/mcp_tools", ListMCPToolServers)
	h.GET("/api/workbench/mcp_tools/:server_id", GetMCPToolServer)
	h.POST("/api/workbench/mcp_tools/:server_id/test_call", TestMCPToolCall)
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

	listed := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools?space_id=1", nil)
	listedBody := string(listed.Result().Body())
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listedBody, `"total":1`)
	require.Contains(t, listedBody, `"server_id":"100"`)
	require.Contains(t, listedBody, `"health_status":"unknown"`)

	got := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools/100", nil)
	gotBody := string(got.Result().Body())
	require.Equal(t, http.StatusOK, got.Code)
	require.Contains(t, gotBody, `"server_type":"stdio"`)

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
	require.Equal(t, "browser-tools", output["server_name"])
	require.Equal(t, "search", output["tool_name"])

	listed = ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools?space_id=1", nil)
	listedBody = string(listed.Result().Body())
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listedBody, `"health_status":"healthy"`)
	require.Contains(t, listedBody, `"health_latency_ms":`)
	require.NotContains(t, listedBody, "coze studio")
}

func TestWorkbenchMCPToolHandlersMaskAuth(t *testing.T) {
	h := server.Default()
	h.POST("/api/workbench/mcp_tools", UpsertMCPToolServer)
	h.GET("/api/workbench/mcp_tools", ListMCPToolServers)
	h.GET("/api/workbench/mcp_tools/:server_id", GetMCPToolServer)
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
	require.JSONEq(t,
		`{"type":"bearer","token":"********","nested":{"api_key":"********"}}`,
		createdResponse.Data.Auth,
	)

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
	require.JSONEq(t,
		`{"type":"bearer","token":"********","nested":{"api_key":"********"}}`,
		listedResponse.Data.Servers[0].Auth,
	)

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
	require.JSONEq(t,
		`{"type":"bearer","token":"********","nested":{"api_key":"********"}}`,
		gotResponse.Data.Auth,
	)
}

func TestWorkbenchMCPToolHandlersDeleteServer(t *testing.T) {
	h := server.Default()
	h.POST("/api/workbench/mcp_tools", UpsertMCPToolServer)
	h.GET("/api/workbench/mcp_tools", ListMCPToolServers)
	h.DELETE("/api/workbench/mcp_tools/:server_id", DeleteMCPToolServer)
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
	require.Contains(t, deletedBody, `********`)

	listed := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools?space_id=1", nil)
	listedBody := string(listed.Result().Body())
	require.Equal(t, http.StatusOK, listed.Code)
	require.Contains(t, listedBody, `"total":0`)
}

func TestWorkbenchMCPToolHandlersListRegistryEntries(t *testing.T) {
	h := server.Default()
	h.POST("/api/workbench/mcp_tools", UpsertMCPToolServer)
	h.GET("/api/workbench/mcp_tools/registry_entries", ListMCPToolRegistryEntries)
	h.POST("/api/workbench/mcp_tools/:server_id/test_call", TestMCPToolCall)
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

func installMCPToolTestService(t *testing.T) {
	t.Helper()
	previous := appmcptool.SVC
	appmcptool.InitService(&appmcptool.Components{
		Catalog: appmcptool.NewInMemoryCatalog(),
		IDGen:   &sequentialIDGen{next: 100},
	})
	t.Cleanup(func() {
		appmcptool.SVC = previous
	})
}
