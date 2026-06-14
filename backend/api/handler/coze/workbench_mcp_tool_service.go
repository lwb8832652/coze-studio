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
		invalidParamRequestResponse(c, err.Error())
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
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appmcptool.SVC.ListServers(ctx, &req)
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
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appmcptool.SVC.GetServer(ctx, &req)
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
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appmcptool.SVC.TestCall(ctx, &req)
	if err != nil {
		workbenchMCPToolErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func workbenchMCPToolErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	if appmcptool.IsClientError(err) {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	internalServerErrorResponse(ctx, c, err)
}
