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

package admin

import (
	"context"

	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	userservice "github.com/coze-dev/coze-studio/backend/domain/user/service"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/slices"
	"github.com/coze-dev/coze-studio/backend/types/errno"
)

type UserDomain interface {
	ListAllUsers(ctx context.Context, keyword string, offset int, limit int) ([]*userentity.User, int64, error)
	ListAllSpaces(ctx context.Context, keyword string, offset int, limit int) ([]*userentity.Space, int64, error)
	MGetUserProfiles(ctx context.Context, userIDs []int64) ([]*userentity.User, error)
	GetSpaceMembers(ctx context.Context, spaceID int64) ([]*userentity.SpaceMember, error)
	GetUserSpaceList(ctx context.Context, userID int64) ([]*userentity.Space, error)
}

type ManagementApplicationService struct {
	UserDomainSVC UserDomain
}

var ManagementApplicationSVC = &ManagementApplicationService{}

func InitService(userDomain userservice.User) *ManagementApplicationService {
	ManagementApplicationSVC.UserDomainSVC = userDomain
	return ManagementApplicationSVC
}

type ListAdminWorkspacesRequest struct {
	Keyword string
	Page    int32
	Size    int32
}

type ListAdminUsersRequest struct {
	Keyword string
	Page    int32
	Size    int32
}

type ListAdminWorkspaceMembersRequest struct {
	SpaceID int64
}

type ListAdminUserSpacesRequest struct {
	UserID int64
}

type AdminWorkspace struct {
	ID             int64  `json:"id,string"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	OwnerUserID    int64  `json:"owner_user_id,string"`
	OwnerName      string `json:"owner_name,omitempty"`
	TotalMemberNum int64  `json:"total_member_num"`
	CreatedAt      int64  `json:"created_at"`
}

type AdminUser struct {
	UserID     int64  `json:"user_id,string"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	UniqueName string `json:"user_unique_name,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

type AdminWorkspaceMember struct {
	UserID     int64  `json:"user_id,string"`
	Name       string `json:"name"`
	UniqueName string `json:"user_unique_name,omitempty"`
	Email      string `json:"email,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	RoleType   int32  `json:"role_type"`
	JoinedAt   int64  `json:"joined_at"`
}

type AdminUserSpace struct {
	ID             int64  `json:"id,string"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	SpaceType      int32  `json:"space_type,omitempty"`
	OwnerUserID    int64  `json:"owner_user_id,string"`
	OwnerName      string `json:"owner_name,omitempty"`
	RoleType       int32  `json:"role_type"`
	TotalMemberNum int64  `json:"total_member_num"`
	CreatedAt      int64  `json:"created_at"`
}

type ListAdminWorkspacesResponse struct {
	Workspaces []*AdminWorkspace `json:"workspaces"`
	Total      int64             `json:"total"`
	Code       int64             `json:"code"`
	Msg        string            `json:"msg"`
}

type ListAdminUsersResponse struct {
	Users []*AdminUser `json:"users"`
	Total int64        `json:"total"`
	Code  int64        `json:"code"`
	Msg   string       `json:"msg"`
}

type ListAdminWorkspaceMembersResponse struct {
	Members []*AdminWorkspaceMember `json:"members"`
	Code    int64                   `json:"code"`
	Msg     string                  `json:"msg"`
}

type ListAdminUserSpacesResponse struct {
	Spaces []*AdminUserSpace `json:"spaces"`
	Code   int64             `json:"code"`
	Msg    string            `json:"msg"`
}

func (s *ManagementApplicationService) ListAdminWorkspaces(
	ctx context.Context,
	req *ListAdminWorkspacesRequest,
) (*ListAdminWorkspacesResponse, error) {
	page, size := normalizePagination(req.Page, req.Size)
	offset := int((page - 1) * size)
	limit := int(size)

	spaces, total, err := s.UserDomainSVC.ListAllSpaces(ctx, req.Keyword, offset, limit)
	if err != nil {
		return nil, err
	}

	ownerIDs := slices.Unique(slices.Transform(spaces, func(space *userentity.Space) int64 {
		return space.OwnerID
	}))
	owners, err := s.UserDomainSVC.MGetUserProfiles(ctx, ownerIDs)
	if err != nil {
		return nil, err
	}
	ownerByID := slices.ToMap(owners, func(owner *userentity.User) (int64, *userentity.User) {
		return owner.UserID, owner
	})

	workspaces := make([]*AdminWorkspace, 0, len(spaces))
	for _, space := range spaces {
		ownerName := ""
		if owner := ownerByID[space.OwnerID]; owner != nil {
			ownerName = owner.Name
		}
		workspaces = append(workspaces, &AdminWorkspace{
			ID:             space.ID,
			Name:           space.Name,
			Description:    space.Description,
			OwnerUserID:    space.OwnerID,
			OwnerName:      ownerName,
			TotalMemberNum: space.MemberCount,
			CreatedAt:      space.CreatedAt,
		})
	}

	return &ListAdminWorkspacesResponse{
		Workspaces: workspaces,
		Total:      total,
		Code:       0,
		Msg:        "success",
	}, nil
}

func (s *ManagementApplicationService) ListAdminUserSpaces(
	ctx context.Context,
	req *ListAdminUserSpacesRequest,
) (*ListAdminUserSpacesResponse, error) {
	if req == nil || req.UserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid admin user spaces request"))
	}

	spaces, err := s.UserDomainSVC.GetUserSpaceList(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	ownerIDs := slices.Unique(slices.Transform(spaces, func(space *userentity.Space) int64 {
		return space.OwnerID
	}))
	owners, err := s.UserDomainSVC.MGetUserProfiles(ctx, ownerIDs)
	if err != nil {
		return nil, err
	}
	ownerByID := slices.ToMap(owners, func(owner *userentity.User) (int64, *userentity.User) {
		return owner.UserID, owner
	})

	adminSpaces := make([]*AdminUserSpace, 0, len(spaces))
	for _, space := range spaces {
		ownerName := ""
		if owner := ownerByID[space.OwnerID]; owner != nil {
			ownerName = owner.Name
		}
		adminSpaces = append(adminSpaces, &AdminUserSpace{
			ID:             space.ID,
			Name:           space.Name,
			Description:    space.Description,
			SpaceType:      int32(space.SpaceType),
			OwnerUserID:    space.OwnerID,
			OwnerName:      ownerName,
			RoleType:       space.RoleType,
			TotalMemberNum: space.MemberCount,
			CreatedAt:      space.CreatedAt,
		})
	}

	return &ListAdminUserSpacesResponse{
		Spaces: adminSpaces,
		Code:   0,
		Msg:    "success",
	}, nil
}

func (s *ManagementApplicationService) ListAdminWorkspaceMembers(
	ctx context.Context,
	req *ListAdminWorkspaceMembersRequest,
) (*ListAdminWorkspaceMembersResponse, error) {
	if req == nil || req.SpaceID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid admin workspace members request"))
	}

	members, err := s.UserDomainSVC.GetSpaceMembers(ctx, req.SpaceID)
	if err != nil {
		return nil, err
	}

	adminMembers := make([]*AdminWorkspaceMember, 0, len(members))
	for _, member := range members {
		adminMembers = append(adminMembers, &AdminWorkspaceMember{
			UserID:     member.UserID,
			Name:       member.Name,
			UniqueName: member.UniqueName,
			Email:      member.Email,
			AvatarURL:  member.AvatarURL,
			RoleType:   member.RoleType,
			JoinedAt:   member.JoinedAt,
		})
	}

	return &ListAdminWorkspaceMembersResponse{
		Members: adminMembers,
		Code:    0,
		Msg:     "success",
	}, nil
}

func (s *ManagementApplicationService) ListAdminUsers(
	ctx context.Context,
	req *ListAdminUsersRequest,
) (*ListAdminUsersResponse, error) {
	page, size := normalizePagination(req.Page, req.Size)
	offset := int((page - 1) * size)
	limit := int(size)

	users, total, err := s.UserDomainSVC.ListAllUsers(ctx, req.Keyword, offset, limit)
	if err != nil {
		return nil, err
	}

	adminUsers := make([]*AdminUser, 0, len(users))
	for _, user := range users {
		adminUsers = append(adminUsers, &AdminUser{
			UserID:     user.UserID,
			Name:       user.Name,
			Email:      user.Email,
			UniqueName: user.UniqueName,
			AvatarURL:  user.IconURL,
			CreatedAt:  user.CreatedAt,
		})
	}

	return &ListAdminUsersResponse{
		Users: adminUsers,
		Total: total,
		Code:  0,
		Msg:   "success",
	}, nil
}

func normalizePagination(page int32, size int32) (int32, int32) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
