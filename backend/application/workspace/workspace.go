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

package workspace

import (
	"context"
	"strings"

	appnotification "github.com/coze-dev/coze-studio/backend/application/notification"
	appuser "github.com/coze-dev/coze-studio/backend/application/user"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	userservice "github.com/coze-dev/coze-studio/backend/domain/user/service"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/types/errno"

	"gorm.io/gorm"
)

type UserDomain interface {
	GetUserSpaceList(ctx context.Context, userID int64) ([]*userentity.Space, error)
	GetSpaceMembers(ctx context.Context, spaceID int64) ([]*userentity.SpaceMember, error)
	ListAllUsers(ctx context.Context, keyword string, offset int, limit int) ([]*userentity.User, int64, error)
	UpdateSpace(ctx context.Context, req *userservice.UpdateSpaceRequest) error
	TransferSpace(ctx context.Context, spaceID int64, targetUserID int64) error
	DeleteSpace(ctx context.Context, spaceID int64) error
	AddSpaceMembers(ctx context.Context, members []*userservice.AddSpaceMemberRequest) error
	UpdateSpaceMemberRole(ctx context.Context, spaceID int64, userID int64, roleType int32) error
	RemoveSpaceMember(ctx context.Context, spaceID int64, userID int64) error
}

type notificationUserDomain interface {
	AddSpaceMembersWithNotification(ctx context.Context, actorID int64, members []*userservice.AddSpaceMemberRequest, appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error) error
	UpdateSpaceMemberRoleWithNotification(ctx context.Context, actorID int64, spaceID int64, userID int64, roleType int32, appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error) error
	RemoveSpaceMemberWithNotification(ctx context.Context, actorID int64, spaceID int64, userID int64, appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error) error
	TransferSpaceWithNotification(ctx context.Context, actorID int64, spaceID int64, targetUserID int64, appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error) error
}

type ApplicationService struct {
	UserDomainSVC UserDomain
}

var SVC = &ApplicationService{}

type GetWorkspaceDetailRequest struct {
	SpaceID       int64
	CurrentUserID int64
}

type CheckWorkspaceMembershipRequest struct {
	SpaceID       int64
	CurrentUserID int64
}

type CheckWorkspaceAppDevAccessRequest struct {
	SpaceID        int64
	CurrentUserID  int64
	RequireManager bool
}

type ListWorkspaceMembersRequest struct {
	SpaceID       int64
	CurrentUserID int64
	Keyword       string
	RoleType      int32
}

type SearchWorkspaceUsersRequest struct {
	SpaceID       int64
	CurrentUserID int64
	Keyword       string
	Limit         int
}

type UpdateWorkspaceRequest struct {
	SpaceID        int64
	CurrentUserID  int64
	Name           *string
	Description    *string
	IconURI        *string
	AllowDevelop   *bool
	ReceivePublish *bool
}

type WorkspaceMemberMutation struct {
	UserID   int64 `json:"user_id,string"`
	RoleType int32 `json:"role_type"`
}

type AddWorkspaceMembersRequest struct {
	SpaceID       int64
	CurrentUserID int64
	Members       []*WorkspaceMemberMutation
}

type UpdateWorkspaceMemberRoleRequest struct {
	SpaceID       int64
	CurrentUserID int64
	UserID        int64
	RoleType      int32
}

type RemoveWorkspaceMemberRequest struct {
	SpaceID       int64
	CurrentUserID int64
	UserID        int64
}

type TransferWorkspaceRequest struct {
	SpaceID       int64
	CurrentUserID int64
	TargetUserID  int64
}

type DeleteWorkspaceRequest struct {
	SpaceID       int64
	CurrentUserID int64
}

type WorkspaceDetail struct {
	ID              int64  `json:"id,string"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	IconURL         string `json:"icon_url,omitempty"`
	SpaceType       int32  `json:"space_type,omitempty"`
	OwnerUserID     int64  `json:"owner_user_id,string"`
	CurrentUserRole int32  `json:"current_user_role"`
	TotalMemberNum  int64  `json:"total_member_num"`
	AllowDevelop    bool   `json:"allow_develop"`
	ReceivePublish  bool   `json:"receive_publish"`
}

type WorkspaceMember struct {
	UserID     int64  `json:"user_id,string"`
	Name       string `json:"name"`
	UniqueName string `json:"user_unique_name,omitempty"`
	Email      string `json:"email,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	RoleType   int32  `json:"role_type"`
	JoinedAt   int64  `json:"joined_at"`
}

type WorkspaceUserCandidate struct {
	UserID     int64  `json:"user_id,string"`
	Name       string `json:"name"`
	UniqueName string `json:"user_unique_name,omitempty"`
	Email      string `json:"email,omitempty"`
	AvatarURL  string `json:"avatar_url,omitempty"`
	RoleType   int32  `json:"role_type,omitempty"`
}

type WorkspaceDetailResponse struct {
	Data *WorkspaceDetail `json:"data"`
	Code int64            `json:"code"`
	Msg  string           `json:"msg"`
}

type WorkspaceMembersResponse struct {
	Members []*WorkspaceMember `json:"members"`
	Code    int64              `json:"code"`
	Msg     string             `json:"msg"`
}

type WorkspaceUsersResponse struct {
	Users []*WorkspaceUserCandidate `json:"users"`
	Total int64                     `json:"total"`
	Code  int64                     `json:"code"`
	Msg   string                    `json:"msg"`
}

type WorkspaceMutationResponse struct {
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
}

const (
	workspaceRoleOwner  int32 = 1
	workspaceRoleAdmin  int32 = 2
	workspaceRoleMember int32 = 3
)

func (s *ApplicationService) GetWorkspaceDetail(
	ctx context.Context,
	req *GetWorkspaceDetailRequest,
) (*WorkspaceDetailResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace detail request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	space, err := s.requireMembership(ctx, domain, req.SpaceID, req.CurrentUserID)
	if err != nil {
		return nil, err
	}

	members, err := domain.GetSpaceMembers(ctx, req.SpaceID)
	if err != nil {
		return nil, err
	}

	currentRole, ok := findMemberRole(members, req.CurrentUserID)
	if !ok {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "current user is not a workspace member"))
	}

	return &WorkspaceDetailResponse{
		Data: &WorkspaceDetail{
			ID:              space.ID,
			Name:            space.Name,
			Description:     space.Description,
			IconURL:         space.IconURL,
			SpaceType:       int32(space.SpaceType),
			OwnerUserID:     space.OwnerID,
			CurrentUserRole: currentRole,
			TotalMemberNum:  int64(len(members)),
			AllowDevelop:    space.AllowDevelop,
			ReceivePublish:  space.ReceivePublish,
		},
		Code: 0,
		Msg:  "success",
	}, nil
}

func (s *ApplicationService) CheckWorkspaceMembership(
	ctx context.Context,
	req *CheckWorkspaceMembershipRequest,
) error {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace membership request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return err
	}

	_, err = s.requireMembership(ctx, domain, req.SpaceID, req.CurrentUserID)
	return err
}

func (s *ApplicationService) CheckWorkspaceAppDevAccess(
	ctx context.Context,
	req *CheckWorkspaceAppDevAccessRequest,
) error {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace appdev access request"))
	}

	resp, err := s.GetWorkspaceDetail(ctx, &GetWorkspaceDetailRequest{
		SpaceID:       req.SpaceID,
		CurrentUserID: req.CurrentUserID,
	})
	if err != nil {
		return err
	}
	if resp == nil || resp.Data == nil {
		return errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "workspace access is unavailable"))
	}

	return validateAppDevAccess(resp.Data, req.RequireManager)
}

func validateAppDevAccess(detail *WorkspaceDetail, requireManager bool) error {
	if detail == nil || !detail.AllowDevelop {
		return errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "workspace does not allow development"))
	}
	if requireManager && detail.CurrentUserRole != workspaceRoleOwner && detail.CurrentUserRole != workspaceRoleAdmin {
		return errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "workspace owner or admin role is required"))
	}
	return nil
}

func (s *ApplicationService) ListWorkspaceMembers(
	ctx context.Context,
	req *ListWorkspaceMembersRequest,
) (*WorkspaceMembersResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace members request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	if _, err := s.requireMembership(ctx, domain, req.SpaceID, req.CurrentUserID); err != nil {
		return nil, err
	}

	members, err := domain.GetSpaceMembers(ctx, req.SpaceID)
	if err != nil {
		return nil, err
	}

	filtered := make([]*WorkspaceMember, 0, len(members))
	for _, member := range members {
		if req.RoleType > 0 && member.RoleType != req.RoleType {
			continue
		}
		if !memberMatchesKeyword(member, req.Keyword) {
			continue
		}
		filtered = append(filtered, &WorkspaceMember{
			UserID:     member.UserID,
			Name:       member.Name,
			UniqueName: member.UniqueName,
			Email:      member.Email,
			AvatarURL:  member.AvatarURL,
			RoleType:   member.RoleType,
			JoinedAt:   member.JoinedAt,
		})
	}

	return &WorkspaceMembersResponse{
		Members: filtered,
		Code:    0,
		Msg:     "success",
	}, nil
}

func (s *ApplicationService) SearchWorkspaceUsers(
	ctx context.Context,
	req *SearchWorkspaceUsersRequest,
) (*WorkspaceUsersResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace user search request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	members, currentRole, err := s.requireManager(ctx, domain, req.SpaceID, req.CurrentUserID)
	if err != nil {
		return nil, err
	}
	if !canManageWorkspace(currentRole) {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "current user cannot manage workspace members"))
	}

	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	users, total, err := domain.ListAllUsers(ctx, req.Keyword, 0, limit)
	if err != nil {
		return nil, err
	}

	roleByUserID := memberRoleMap(members)
	candidates := make([]*WorkspaceUserCandidate, 0, len(users))
	for _, user := range users {
		candidates = append(candidates, &WorkspaceUserCandidate{
			UserID:     user.UserID,
			Name:       user.Name,
			UniqueName: user.UniqueName,
			Email:      user.Email,
			AvatarURL:  user.IconURL,
			RoleType:   roleByUserID[user.UserID],
		})
	}

	return &WorkspaceUsersResponse{
		Users: candidates,
		Total: total,
		Code:  0,
		Msg:   "success",
	}, nil
}

func (s *ApplicationService) UpdateWorkspace(
	ctx context.Context,
	req *UpdateWorkspaceRequest,
) (*WorkspaceMutationResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace update request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	if _, currentRole, err := s.requireManager(ctx, domain, req.SpaceID, req.CurrentUserID); err != nil {
		return nil, err
	} else if !canManageWorkspace(currentRole) {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "current user cannot update workspace"))
	}

	if err := domain.UpdateSpace(ctx, &userservice.UpdateSpaceRequest{
		SpaceID:        req.SpaceID,
		Name:           req.Name,
		Description:    req.Description,
		IconURI:        req.IconURI,
		AllowDevelop:   req.AllowDevelop,
		ReceivePublish: req.ReceivePublish,
	}); err != nil {
		return nil, err
	}

	return successMutationResponse(), nil
}

func (s *ApplicationService) AddWorkspaceMembers(
	ctx context.Context,
	req *AddWorkspaceMembersRequest,
) (*WorkspaceMutationResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 || len(req.Members) == 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace member add request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	members, currentRole, err := s.requireManager(ctx, domain, req.SpaceID, req.CurrentUserID)
	if err != nil {
		return nil, err
	}
	if !canManageWorkspace(currentRole) {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "current user cannot add workspace members"))
	}

	existingRoles := memberRoleMap(members)
	toAdd := make([]*userservice.AddSpaceMemberRequest, 0, len(req.Members))
	for _, member := range req.Members {
		if member == nil || member.UserID <= 0 || !canAssignRole(member.RoleType) {
			return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace member"))
		}
		if existingRoles[member.UserID] > 0 {
			continue
		}
		toAdd = append(toAdd, &userservice.AddSpaceMemberRequest{
			SpaceID:  req.SpaceID,
			UserID:   member.UserID,
			RoleType: member.RoleType,
		})
		existingRoles[member.UserID] = member.RoleType
	}

	if len(toAdd) > 0 {
		notifier, err := requireNotificationUserDomain(domain)
		if err != nil {
			return nil, err
		}
		if err := notifier.AddSpaceMembersWithNotification(ctx, req.CurrentUserID, toAdd, workspaceNotificationOutbox()); err != nil {
			return nil, err
		}
	}

	return successMutationResponse(), nil
}

func (s *ApplicationService) UpdateWorkspaceMemberRole(
	ctx context.Context,
	req *UpdateWorkspaceMemberRoleRequest,
) (*WorkspaceMutationResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 || req.UserID <= 0 || !canAssignRole(req.RoleType) {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace member role request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	members, currentRole, err := s.requireManager(ctx, domain, req.SpaceID, req.CurrentUserID)
	if err != nil {
		return nil, err
	}
	if !canManageWorkspace(currentRole) {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "current user cannot update workspace member role"))
	}

	targetRole := memberRoleMap(members)[req.UserID]
	if targetRole == 0 {
		return nil, errorx.New(errno.ErrUserResourceNotFound, errorx.KV("type", "workspace member"))
	}
	if targetRole == workspaceRoleOwner {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "workspace owner role cannot be changed"))
	}

	notifier, err := requireNotificationUserDomain(domain)
	if err != nil {
		return nil, err
	}
	if err := notifier.UpdateSpaceMemberRoleWithNotification(ctx, req.CurrentUserID, req.SpaceID, req.UserID, req.RoleType, workspaceNotificationOutbox()); err != nil {
		return nil, err
	}

	return successMutationResponse(), nil
}

func (s *ApplicationService) RemoveWorkspaceMember(
	ctx context.Context,
	req *RemoveWorkspaceMemberRequest,
) (*WorkspaceMutationResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 || req.UserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace member remove request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	members, currentRole, err := s.requireManager(ctx, domain, req.SpaceID, req.CurrentUserID)
	if err != nil {
		return nil, err
	}
	if !canManageWorkspace(currentRole) {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "current user cannot remove workspace members"))
	}

	targetRole := memberRoleMap(members)[req.UserID]
	if targetRole == 0 {
		return nil, errorx.New(errno.ErrUserResourceNotFound, errorx.KV("type", "workspace member"))
	}
	if targetRole == workspaceRoleOwner {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "workspace owner cannot be removed"))
	}

	notifier, err := requireNotificationUserDomain(domain)
	if err != nil {
		return nil, err
	}
	if err := notifier.RemoveSpaceMemberWithNotification(ctx, req.CurrentUserID, req.SpaceID, req.UserID, workspaceNotificationOutbox()); err != nil {
		return nil, err
	}

	return successMutationResponse(), nil
}

func (s *ApplicationService) TransferWorkspace(
	ctx context.Context,
	req *TransferWorkspaceRequest,
) (*WorkspaceMutationResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 || req.TargetUserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace transfer request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	members, currentRole, err := s.requireManager(ctx, domain, req.SpaceID, req.CurrentUserID)
	if err != nil {
		return nil, err
	}
	if currentRole != workspaceRoleOwner {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "only workspace owner can transfer workspace"))
	}
	targetRole := memberRoleMap(members)[req.TargetUserID]
	if targetRole == 0 {
		return nil, errorx.New(errno.ErrUserResourceNotFound, errorx.KV("type", "workspace member"))
	}
	if targetRole == workspaceRoleOwner {
		return successMutationResponse(), nil
	}

	notifier, err := requireNotificationUserDomain(domain)
	if err != nil {
		return nil, err
	}
	if err := notifier.TransferSpaceWithNotification(ctx, req.CurrentUserID, req.SpaceID, req.TargetUserID, workspaceNotificationOutbox()); err != nil {
		return nil, err
	}

	return successMutationResponse(), nil
}

func (s *ApplicationService) DeleteWorkspace(
	ctx context.Context,
	req *DeleteWorkspaceRequest,
) (*WorkspaceMutationResponse, error) {
	if req == nil || req.SpaceID <= 0 || req.CurrentUserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid workspace delete request"))
	}

	domain, err := s.userDomain()
	if err != nil {
		return nil, err
	}

	space, err := s.requireMembership(ctx, domain, req.SpaceID, req.CurrentUserID)
	if err != nil {
		return nil, err
	}
	if space.SpaceType == userentity.SpaceTypePersonal {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "personal workspace cannot be deleted"))
	}

	members, err := domain.GetSpaceMembers(ctx, req.SpaceID)
	if err != nil {
		return nil, err
	}
	currentRole, ok := findMemberRole(members, req.CurrentUserID)
	if !ok || currentRole != workspaceRoleOwner {
		return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "only workspace owner can delete workspace"))
	}

	if err := domain.DeleteSpace(ctx, req.SpaceID); err != nil {
		return nil, err
	}

	return successMutationResponse(), nil
}

func (s *ApplicationService) userDomain() (UserDomain, error) {
	if s.UserDomainSVC != nil {
		return s.UserDomainSVC, nil
	}
	if appuser.UserApplicationSVC != nil && appuser.UserApplicationSVC.DomainSVC != nil {
		if domain, ok := appuser.UserApplicationSVC.DomainSVC.(UserDomain); ok {
			return domain, nil
		}
	}
	return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "workspace service is unavailable"))
}

func (s *ApplicationService) requireMembership(
	ctx context.Context,
	domain UserDomain,
	spaceID int64,
	currentUserID int64,
) (*userentity.Space, error) {
	spaces, err := domain.GetUserSpaceList(ctx, currentUserID)
	if err != nil {
		return nil, err
	}

	for _, space := range spaces {
		if space.ID == spaceID {
			return space, nil
		}
	}

	return nil, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "current user is not a workspace member"))
}

func (s *ApplicationService) requireManager(
	ctx context.Context,
	domain UserDomain,
	spaceID int64,
	currentUserID int64,
) ([]*userentity.SpaceMember, int32, error) {
	if _, err := s.requireMembership(ctx, domain, spaceID, currentUserID); err != nil {
		return nil, 0, err
	}

	members, err := domain.GetSpaceMembers(ctx, spaceID)
	if err != nil {
		return nil, 0, err
	}

	currentRole, ok := findMemberRole(members, currentUserID)
	if !ok {
		return nil, 0, errorx.New(errno.ErrUserPermissionCode, errorx.KV("msg", "current user is not a workspace member"))
	}

	return members, currentRole, nil
}

func findMemberRole(members []*userentity.SpaceMember, userID int64) (int32, bool) {
	for _, member := range members {
		if member.UserID == userID {
			return member.RoleType, true
		}
	}
	return 0, false
}

func memberMatchesKeyword(member *userentity.SpaceMember, keyword string) bool {
	normalized := strings.ToLower(strings.TrimSpace(keyword))
	if normalized == "" {
		return true
	}

	return strings.Contains(strings.ToLower(member.Name), normalized) ||
		strings.Contains(strings.ToLower(member.UniqueName), normalized) ||
		strings.Contains(strings.ToLower(member.Email), normalized)
}

func memberRoleMap(members []*userentity.SpaceMember) map[int64]int32 {
	roles := make(map[int64]int32, len(members))
	for _, member := range members {
		roles[member.UserID] = member.RoleType
	}
	return roles
}

func canManageWorkspace(roleType int32) bool {
	return roleType == workspaceRoleOwner || roleType == workspaceRoleAdmin
}

func canAssignRole(roleType int32) bool {
	return roleType == workspaceRoleAdmin || roleType == workspaceRoleMember
}

func requireNotificationUserDomain(domain UserDomain) (notificationUserDomain, error) {
	notifier, ok := domain.(notificationUserDomain)
	if !ok {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "workspace notification mutation service is unavailable"))
	}
	return notifier, nil
}

func successMutationResponse() *WorkspaceMutationResponse {
	return &WorkspaceMutationResponse{
		Code: 0,
		Msg:  "success",
	}
}

func workspaceNotificationOutbox() func(context.Context, *gorm.DB, domainnotification.Event) error {
	if appnotification.SVC == nil || !appnotification.SVC.IsConfigured() {
		return nil
	}
	return appnotification.SVC.AppendInTransaction
}
