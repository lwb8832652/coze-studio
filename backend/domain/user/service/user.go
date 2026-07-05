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

package service

import (
	"context"

	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

type UpdateProfileRequest struct {
	UserID      int64
	Name        *string
	UniqueName  *string
	Description *string
	Locale      *string
}

type ValidateProfileUpdateRequest struct {
	UniqueName *string
	Email      *string
}

type ValidateProfileUpdateResult int

const (
	ValidateSuccess             ValidateProfileUpdateResult = 0
	UniqueNameExist             ValidateProfileUpdateResult = 2
	UniqueNameTooShortOrTooLong ValidateProfileUpdateResult = 3
	EmailExist                  ValidateProfileUpdateResult = 5
)

type ValidateProfileUpdateResponse struct {
	Code ValidateProfileUpdateResult
	Msg  string
}

type CreateUserRequest struct {
	Email       string
	Password    string
	Name        string
	UniqueName  string
	Description string
	SpaceID     int64
	Locale      string
}

type CreateSpaceRequest struct {
	UserID      int64
	Name        string
	Description string
	IconURI     string
	SpaceType   entity.SpaceType
}

type UpdateSpaceRequest struct {
	SpaceID        int64
	Name           *string
	Description    *string
	IconURI        *string
	AllowDevelop   *bool
	ReceivePublish *bool
}

type AddSpaceMemberRequest struct {
	SpaceID  int64
	UserID   int64
	RoleType int32
}

type CreateUserResponse struct {
	UserID int64
}

type User interface {
	SaasUserProvider
	// Create creates or registers a new user.
	Create(ctx context.Context, req *CreateUserRequest) (user *entity.User, err error)
	Login(ctx context.Context, email, password string) (user *entity.User, err error)
	Logout(ctx context.Context, userID int64) (err error)
	ResetPassword(ctx context.Context, email, password string) (err error)
	GetUserInfo(ctx context.Context, userID int64) (user *entity.User, err error)
	UpdateAvatar(ctx context.Context, userID int64, ext string, imagePayload []byte) (url string, err error)
	UpdateProfile(ctx context.Context, req *UpdateProfileRequest) (err error)
	ValidateProfileUpdate(ctx context.Context, req *ValidateProfileUpdateRequest) (resp *ValidateProfileUpdateResponse, err error)
	GetUserProfiles(ctx context.Context, userID int64) (user *entity.User, err error)
	MGetUserProfiles(ctx context.Context, userIDs []int64) (users []*entity.User, err error)
	ValidateSession(ctx context.Context, sessionKey string) (session *entity.Session, exist bool, err error)
	CreateSpace(ctx context.Context, req *CreateSpaceRequest) (space *entity.Space, err error)
	UpdateSpace(ctx context.Context, req *UpdateSpaceRequest) (err error)
	TransferSpace(ctx context.Context, spaceID int64, targetUserID int64) (err error)
	DeleteSpace(ctx context.Context, spaceID int64) (err error)
	GetUserSpaceList(ctx context.Context, userID int64) (spaces []*entity.Space, err error)
	GetUserSpaceBySpaceID(ctx context.Context, spaceID []int64) (space []*entity.Space, err error)
	ListAllUsers(ctx context.Context, keyword string, offset int, limit int) (users []*entity.User, total int64, err error)
	ListAllSpaces(ctx context.Context, keyword string, offset int, limit int) (spaces []*entity.Space, total int64, err error)
	GetSpaceMembers(ctx context.Context, spaceID int64) (members []*entity.SpaceMember, err error)
	AddSpaceMembers(ctx context.Context, members []*AddSpaceMemberRequest) (err error)
	UpdateSpaceMemberRole(ctx context.Context, spaceID int64, userID int64, roleType int32) (err error)
	RemoveSpaceMember(ctx context.Context, spaceID int64, userID int64) (err error)
}

type SaasUserProvider interface {
	GetSaasUserInfo(ctx context.Context) (user *entity.SaasUserData, err error)
	GetUserBenefit(ctx context.Context) (benefit *entity.UserBenefit, err error)
}
