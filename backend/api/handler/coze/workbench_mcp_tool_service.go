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
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	appmcptool "github.com/coze-dev/coze-studio/backend/application/mcptool"
)

// UpsertMCPToolServer .
// @router /api/workbench/mcp_tools [POST]
func UpsertMCPToolServer(ctx context.Context, c *app.RequestContext) {
	var req toolapi.UpsertMCPToolServerRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}

	resp, err := appmcptool.SVC.UpsertServer(ctx, &req)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// ListMCPToolServers .
// @router /api/workbench/mcp_tools [GET]
func ListMCPToolServers(ctx context.Context, c *app.RequestContext) {
	var req toolapi.ListMCPToolServersRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}

	resp, err := appmcptool.SVC.ListServers(ctx, &req)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// ListMCPToolRegistryEntries .
// @router /api/workbench/mcp_tools/registry_entries [GET]
func ListMCPToolRegistryEntries(ctx context.Context, c *app.RequestContext) {
	var req toolapi.ListMCPToolRegistryEntriesRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}

	resp, err := appmcptool.SVC.ListRegistryEntries(ctx, &req)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// GetMCPToolServer .
// @router /api/workbench/mcp_tools/:server_id [GET]
func GetMCPToolServer(ctx context.Context, c *app.RequestContext) {
	var req toolapi.GetMCPToolServerRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}

	resp, err := appmcptool.SVC.GetServer(ctx, &req)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// DeleteMCPToolServer .
// @router /api/workbench/mcp_tools/:server_id [DELETE]
func DeleteMCPToolServer(ctx context.Context, c *app.RequestContext) {
	var req toolapi.GetMCPToolServerRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}

	resp, err := appmcptool.SVC.DeleteServer(ctx, &req)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// TestMCPToolCall .
// @router /api/workbench/mcp_tools/:server_id/test_call [POST]
func TestMCPToolCall(ctx context.Context, c *app.RequestContext) {
	var req toolapi.TestMCPToolCallRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}

	resp, err := appmcptool.SVC.TestCall(ctx, &req)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// DiscoverMCPToolServer refreshes the bounded capability snapshot persisted for
// a server. Credentials and provider payloads are never included in the result.
func DiscoverMCPToolServer(ctx context.Context, c *app.RequestContext) {
	var req toolapi.DiscoverMCPToolServerRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}
	data, err := appmcptool.SVC.Discover(ctx, req.ServerID)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, &toolapi.DiscoverMCPToolServerResponse{Code: 0, Msg: "success", Data: data})
}

// ExportMCPToolServer downloads a portable server definition without auth or
// connection secrets.
func ExportMCPToolServer(ctx context.Context, c *app.RequestContext) {
	var req toolapi.ExportMCPToolServerRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}
	data, err := appmcptool.SVC.SafeExport(ctx, req.ServerID)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="mcp-server-%d.json"`, req.ServerID))
	c.JSON(consts.StatusOK, &toolapi.ExportMCPToolServerResponse{Code: 0, Msg: "success", Data: data})
}

// ListMCPToolAuditEvents returns only bounded management audit metadata.
func ListMCPToolAuditEvents(ctx context.Context, c *app.RequestContext) {
	var req toolapi.ListMCPRuntimeAuditEventsRequest
	if err := c.BindAndValidate(&req); err != nil {
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
		return
	}
	resp, err := appmcptool.SVC.ListAuditEvents(ctx, req.ServerID, req.Cursor, int(req.Limit))
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, resp)
}

func workbenchMCPToolErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	_ = ctx
	switch {
	case errors.Is(err, appmcptool.ErrMCPDisabled):
		workbenchMCPToolJSONError(c, http.StatusServiceUnavailable, "mcp service disabled")
	case errors.Is(err, appmcptool.ErrMCPUnauthenticated):
		workbenchMCPToolJSONError(c, http.StatusUnauthorized, "authentication required")
	case errors.Is(err, appmcptool.ErrMCPForbidden), errors.Is(err, appmcptool.ErrNotFound):
		workbenchMCPToolJSONError(c, http.StatusForbidden, "resource unavailable")
	case errors.Is(err, appmcptool.ErrMCPConflict), errors.Is(err, appmcptool.ErrMCPOfficialServerImmutable):
		workbenchMCPToolJSONError(c, http.StatusConflict, "request conflict")
	case errors.Is(err, appmcptool.ErrRuntimeUnavailable),
		errors.Is(err, appmcptool.ErrCapabilityDiscoveryUnavailable),
		errors.Is(err, appmcptool.ErrManagementAuditUnavailable):
		workbenchMCPToolJSONError(c, http.StatusServiceUnavailable, "mcp runtime unavailable")
	case errors.Is(err, appmcptool.ErrRuntimeCallFailed):
		workbenchMCPToolJSONError(c, http.StatusBadGateway, "mcp runtime call failed")
	case appmcptool.IsClientError(err):
		workbenchMCPToolJSONError(c, http.StatusBadRequest, "invalid request")
	default:
		workbenchMCPToolJSONError(c, http.StatusInternalServerError, "internal server error")
	}
}

func workbenchMCPToolJSONError(c *app.RequestContext, statusCode int, message string) {
	c.JSON(statusCode, map[string]any{
		"code": int64(statusCode),
		"msg":  message,
	})
}
