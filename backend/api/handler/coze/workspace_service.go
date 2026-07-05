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
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"github.com/coze-dev/coze-studio/backend/api/internal/httputil"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appworkspace "github.com/coze-dev/coze-studio/backend/application/workspace"
)

type listWorkspaceMembersRequest struct {
	SpaceID  string `json:"space_id"`
	Keyword  string `json:"keyword"`
	RoleType int32  `json:"role_type"`
}

type searchWorkspaceUsersRequest struct {
	SpaceID string `json:"space_id"`
	Keyword string `json:"keyword"`
	Limit   int    `json:"limit"`
}

type updateWorkspaceRequest struct {
	SpaceID        string  `json:"space_id"`
	Name           *string `json:"name"`
	Description    *string `json:"description"`
	IconURI        *string `json:"icon_uri"`
	AllowDevelop   *bool   `json:"allow_develop"`
	ReceivePublish *bool   `json:"receive_publish"`
}

type workspaceMemberMutationRequest struct {
	UserID   string `json:"user_id"`
	RoleType int32  `json:"role_type"`
}

type addWorkspaceMembersRequest struct {
	SpaceID string                           `json:"space_id"`
	Members []workspaceMemberMutationRequest `json:"members"`
}

type updateWorkspaceMemberRoleRequest struct {
	SpaceID  string `json:"space_id"`
	UserID   string `json:"user_id"`
	RoleType int32  `json:"role_type"`
}

type removeWorkspaceMemberRequest struct {
	SpaceID string `json:"space_id"`
	UserID  string `json:"user_id"`
}

type transferWorkspaceRequest struct {
	SpaceID      string `json:"space_id"`
	TargetUserID string `json:"target_user_id"`
}

type deleteWorkspaceRequest struct {
	SpaceID string `json:"space_id"`
}

func GetWorkspaceDetail(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	spaceID, err := parseWorkspaceSpaceID(c.Query("space_id"))
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}

	resp, err := appworkspace.SVC.GetWorkspaceDetail(ctx, &appworkspace.GetWorkspaceDetailRequest{
		SpaceID:       spaceID,
		CurrentUserID: *currentUserID,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func ListWorkspaceMembers(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	var req listWorkspaceMembersRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	spaceID, err := parseWorkspaceSpaceID(req.SpaceID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}

	resp, err := appworkspace.SVC.ListWorkspaceMembers(ctx, &appworkspace.ListWorkspaceMembersRequest{
		SpaceID:       spaceID,
		CurrentUserID: *currentUserID,
		Keyword:       req.Keyword,
		RoleType:      req.RoleType,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func SearchWorkspaceUsers(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	var req searchWorkspaceUsersRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	spaceID, err := parseWorkspaceSpaceID(req.SpaceID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}

	resp, err := appworkspace.SVC.SearchWorkspaceUsers(ctx, &appworkspace.SearchWorkspaceUsersRequest{
		SpaceID:       spaceID,
		CurrentUserID: *currentUserID,
		Keyword:       req.Keyword,
		Limit:         req.Limit,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func UpdateWorkspace(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	var req updateWorkspaceRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	spaceID, err := parseWorkspaceSpaceID(req.SpaceID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}

	resp, err := appworkspace.SVC.UpdateWorkspace(ctx, &appworkspace.UpdateWorkspaceRequest{
		SpaceID:        spaceID,
		CurrentUserID:  *currentUserID,
		Name:           req.Name,
		Description:    req.Description,
		IconURI:        req.IconURI,
		AllowDevelop:   req.AllowDevelop,
		ReceivePublish: req.ReceivePublish,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func AddWorkspaceMembers(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	var req addWorkspaceMembersRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	spaceID, err := parseWorkspaceSpaceID(req.SpaceID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}

	members := make([]*appworkspace.WorkspaceMemberMutation, 0, len(req.Members))
	for _, member := range req.Members {
		userID, err := parseWorkspaceSpaceID(member.UserID)
		if err != nil {
			invalidParamRequestResponse(c, "invalid user_id")
			return
		}
		members = append(members, &appworkspace.WorkspaceMemberMutation{
			UserID:   userID,
			RoleType: member.RoleType,
		})
	}

	resp, err := appworkspace.SVC.AddWorkspaceMembers(ctx, &appworkspace.AddWorkspaceMembersRequest{
		SpaceID:       spaceID,
		CurrentUserID: *currentUserID,
		Members:       members,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func UpdateWorkspaceMemberRole(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	var req updateWorkspaceMemberRoleRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	spaceID, err := parseWorkspaceSpaceID(req.SpaceID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}
	userID, err := parseWorkspaceSpaceID(req.UserID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid user_id")
		return
	}

	resp, err := appworkspace.SVC.UpdateWorkspaceMemberRole(ctx, &appworkspace.UpdateWorkspaceMemberRoleRequest{
		SpaceID:       spaceID,
		CurrentUserID: *currentUserID,
		UserID:        userID,
		RoleType:      req.RoleType,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func RemoveWorkspaceMember(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	var req removeWorkspaceMemberRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	spaceID, err := parseWorkspaceSpaceID(req.SpaceID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}
	userID, err := parseWorkspaceSpaceID(req.UserID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid user_id")
		return
	}

	resp, err := appworkspace.SVC.RemoveWorkspaceMember(ctx, &appworkspace.RemoveWorkspaceMemberRequest{
		SpaceID:       spaceID,
		CurrentUserID: *currentUserID,
		UserID:        userID,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func TransferWorkspace(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	var req transferWorkspaceRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	spaceID, err := parseWorkspaceSpaceID(req.SpaceID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}
	targetUserID, err := parseWorkspaceSpaceID(req.TargetUserID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid target_user_id")
		return
	}

	resp, err := appworkspace.SVC.TransferWorkspace(ctx, &appworkspace.TransferWorkspaceRequest{
		SpaceID:       spaceID,
		CurrentUserID: *currentUserID,
		TargetUserID:  targetUserID,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func DeleteWorkspace(ctx context.Context, c *app.RequestContext) {
	currentUserID := ctxutil.GetUIDFromCtx(ctx)
	if currentUserID == nil {
		httputil.Unauthorized(c, "missing user session")
		return
	}

	var req deleteWorkspaceRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	spaceID, err := parseWorkspaceSpaceID(req.SpaceID)
	if err != nil {
		invalidParamRequestResponse(c, "invalid space_id")
		return
	}

	resp, err := appworkspace.SVC.DeleteWorkspace(ctx, &appworkspace.DeleteWorkspaceRequest{
		SpaceID:       spaceID,
		CurrentUserID: *currentUserID,
	})
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func parseWorkspaceSpaceID(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}
