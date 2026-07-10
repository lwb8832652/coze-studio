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

	appadmin "github.com/coze-dev/coze-studio/backend/application/admin"
)

type listAdminResourcesRequest struct {
	Keyword string `json:"keyword"`
	Page    int32  `json:"page"`
	Size    int32  `json:"size"`
}

type listAdminWorkspaceMembersRequest struct {
	SpaceID int64 `json:"space_id,string"`
}

type listAdminUserSpacesRequest struct {
	UserID int64 `json:"user_id,string"`
}

type createAdminUserRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	Name       string `json:"name"`
	UniqueName string `json:"user_unique_name"`
	Locale     string `json:"locale"`
}

type updateAdminUserRequest struct {
	UserID     int64  `json:"user_id,string"`
	Name       string `json:"name"`
	UniqueName string `json:"user_unique_name"`
	Locale     string `json:"locale"`
}

type resetAdminUserPasswordRequest struct {
	UserID   int64  `json:"user_id,string"`
	Password string `json:"password"`
}

func ListAdminWorkspaces(ctx context.Context, c *app.RequestContext) {
	var req listAdminResourcesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appadmin.ManagementApplicationSVC.ListAdminWorkspaces(ctx, &appadmin.ListAdminWorkspacesRequest{
		Keyword: req.Keyword,
		Page:    req.Page,
		Size:    req.Size,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func CreateAdminUser(ctx context.Context, c *app.RequestContext) {
	var req createAdminUserRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appadmin.ManagementApplicationSVC.CreateAdminUser(ctx, &appadmin.CreateAdminUserRequest{
		Email:      req.Email,
		Password:   req.Password,
		Name:       req.Name,
		UniqueName: req.UniqueName,
		Locale:     req.Locale,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func UpdateAdminUser(ctx context.Context, c *app.RequestContext) {
	var req updateAdminUserRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appadmin.ManagementApplicationSVC.UpdateAdminUser(ctx, &appadmin.UpdateAdminUserRequest{
		UserID:     req.UserID,
		Name:       req.Name,
		UniqueName: req.UniqueName,
		Locale:     req.Locale,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ResetAdminUserPassword(ctx context.Context, c *app.RequestContext) {
	var req resetAdminUserPasswordRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appadmin.ManagementApplicationSVC.ResetAdminUserPassword(ctx, &appadmin.ResetAdminUserPasswordRequest{
		UserID:   req.UserID,
		Password: req.Password,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ListAdminUserSpaces(ctx context.Context, c *app.RequestContext) {
	var req listAdminUserSpacesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appadmin.ManagementApplicationSVC.ListAdminUserSpaces(ctx, &appadmin.ListAdminUserSpacesRequest{
		UserID: req.UserID,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ListAdminWorkspaceMembers(ctx context.Context, c *app.RequestContext) {
	var req listAdminWorkspaceMembersRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appadmin.ManagementApplicationSVC.ListAdminWorkspaceMembers(ctx, &appadmin.ListAdminWorkspaceMembersRequest{
		SpaceID: req.SpaceID,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ListAdminUsers(ctx context.Context, c *app.RequestContext) {
	var req listAdminResourcesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appadmin.ManagementApplicationSVC.ListAdminUsers(ctx, &appadmin.ListAdminUsersRequest{
		Keyword: req.Keyword,
		Page:    req.Page,
		Size:    req.Size,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}
