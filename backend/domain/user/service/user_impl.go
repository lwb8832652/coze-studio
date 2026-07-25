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
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	uploadEntity "github.com/coze-dev/coze-studio/backend/domain/upload/entity"
	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/domain/user/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/conv"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/slices"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/types/consts"
	"github.com/coze-dev/coze-studio/backend/types/errno"
)

type Components struct {
	IconOSS   storage.Storage
	IDGen     idgen.IDGenerator
	UserRepo  repository.UserRepository
	SpaceRepo repository.SpaceRepository
}

func NewUserDomain(ctx context.Context, c *Components) User {
	return &userImpl{
		Components: c,
	}
}

type userImpl struct {
	*Components
}

const (
	defaultPersonalSpaceName        = "Personal Space"
	defaultPersonalSpaceDescription = "This is your personal space"
)

func (u *userImpl) Login(ctx context.Context, email, password string) (user *userEntity.User, err error) {
	userModel, exist, err := u.UserRepo.GetUsersByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if !exist {
		return nil, errorx.New(errno.ErrUserInfoInvalidateCode)
	}

	// Verify the password using the Argon2id algorithm
	valid, err := verifyPassword(password, userModel.Password)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errorx.New(errno.ErrUserInfoInvalidateCode)
	}

	uniqueSessionID, err := u.IDGen.GenID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate session id: %w", err)
	}

	sessionKey, err := generateSessionKey(uniqueSessionID)
	if err != nil {
		return nil, err
	}

	// Update user session key
	err = u.UserRepo.UpdateSessionKey(ctx, userModel.ID, sessionKey)
	if err != nil {
		return nil, err
	}

	userModel.SessionKey = sessionKey

	resURL, err := u.IconOSS.GetObjectUrl(ctx, userModel.IconURI)
	if err != nil {
		return nil, err
	}

	return userPo2Do(userModel, resURL), nil
}

func (u *userImpl) Logout(ctx context.Context, userID int64) (err error) {
	err = u.UserRepo.ClearSessionKey(ctx, userID)
	if err != nil {
		return err
	}

	return nil
}

func (u *userImpl) ResetPassword(ctx context.Context, email, password string) (err error) {
	// Hashing passwords using the Argon2id algorithm
	hashedPassword, err := hashPassword(password)
	if err != nil {
		return err
	}

	err = u.UserRepo.UpdatePassword(ctx, email, hashedPassword)
	if err != nil {
		return err
	}

	return nil
}

func (u *userImpl) GetUserInfo(ctx context.Context, userID int64) (resp *userEntity.User, err error) {
	if userID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode,
			errorx.KVf("msg", "invalid user id : %d", userID))
	}

	userModel, err := u.UserRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	resURL, err := u.IconOSS.GetObjectUrl(ctx, userModel.IconURI)
	if err != nil {
		return nil, err
	}

	return userPo2Do(userModel, resURL), nil
}

func (u *userImpl) UpdateAvatar(ctx context.Context, userID int64, ext string, imagePayload []byte) (url string, err error) {
	avatarKey := "user_avatar/" + strconv.FormatInt(userID, 10) + "." + ext
	err = u.IconOSS.PutObject(ctx, avatarKey, imagePayload)
	if err != nil {
		return "", err
	}

	err = u.UserRepo.UpdateAvatar(ctx, userID, avatarKey)
	if err != nil {
		return "", err
	}

	url, err = u.IconOSS.GetObjectUrl(ctx, avatarKey)
	if err != nil {
		return "", err
	}

	return url, nil
}

func (u *userImpl) ValidateProfileUpdate(ctx context.Context, req *ValidateProfileUpdateRequest) (
	resp *ValidateProfileUpdateResponse, err error,
) {
	if req.UniqueName == nil && req.Email == nil {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "missing parameter"))
	}

	if req.UniqueName != nil {
		uniqueName := ptr.From(req.UniqueName)
		charNum := utf8.RuneCountInString(uniqueName)

		if charNum < 4 || charNum > 20 {
			return &ValidateProfileUpdateResponse{
				Code: UniqueNameTooShortOrTooLong,
				Msg:  "unique name length should be between 4 and 20",
			}, nil
		}

		exist, err := u.UserRepo.CheckUniqueNameExist(ctx, uniqueName)
		if err != nil {
			return nil, err
		}

		if exist {
			return &ValidateProfileUpdateResponse{
				Code: UniqueNameExist,
				Msg:  "unique name existed",
			}, nil
		}
	}

	return &ValidateProfileUpdateResponse{
		Code: ValidateSuccess,
		Msg:  "success",
	}, nil
}

func (u *userImpl) UpdateProfile(ctx context.Context, req *UpdateProfileRequest) error {
	updates := map[string]interface{}{
		"updated_at": time.Now().UnixMilli(),
	}

	if req.UniqueName != nil {
		resp, err := u.ValidateProfileUpdate(ctx, &ValidateProfileUpdateRequest{
			UniqueName: req.UniqueName,
		})
		if err != nil {
			return err
		}

		if resp.Code != ValidateSuccess {
			return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", resp.Msg))
		}

		updates["unique_name"] = ptr.From(req.UniqueName)
	}

	if req.Name != nil {
		updates["name"] = ptr.From(req.Name)
	}

	if req.Description != nil {
		updates["description"] = ptr.From(req.Description)
	}

	if req.Locale != nil {
		updates["locale"] = ptr.From(req.Locale)
	}

	err := u.UserRepo.UpdateProfile(ctx, req.UserID, updates)
	if err != nil {
		return err
	}

	return nil
}

func (u *userImpl) Create(ctx context.Context, req *CreateUserRequest) (user *userEntity.User, err error) {
	exist, err := u.UserRepo.CheckEmailExist(ctx, req.Email)
	if err != nil {
		return nil, err
	}

	if exist {
		return nil, errorx.New(errno.ErrUserEmailAlreadyExistCode, errorx.KV("email", req.Email))
	}

	if req.UniqueName != "" {
		exist, err = u.UserRepo.CheckUniqueNameExist(ctx, req.UniqueName)
		if err != nil {
			return nil, err
		}
		if exist {
			return nil, errorx.New(errno.ErrUserUniqueNameAlreadyExistCode, errorx.KV("name", req.UniqueName))
		}
	}

	// Hashing passwords using the Argon2id algorithm
	hashedPassword, err := hashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	name := req.Name
	if name == "" {
		name = strings.Split(req.Email, "@")[0]
	}

	userID, err := u.IDGen.GenID(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate id error: %w", err)
	}

	now := time.Now().UnixMilli()

	spaceID := req.SpaceID
	if spaceID <= 0 {
		var sid int64
		sid, err = u.IDGen.GenID(ctx)
		if err != nil {
			return nil, fmt.Errorf("gen space_id failed: %w", err)
		}

		err = u.SpaceRepo.CreateSpace(ctx, &model.Space{
			ID:             sid,
			Name:           defaultPersonalSpaceName,
			Description:    defaultPersonalSpaceDescription,
			IconURI:        uploadEntity.EnterpriseIconURI,
			OwnerID:        userID,
			CreatorID:      userID,
			SpaceType:      int32(userEntity.SpaceTypePersonal),
			AllowDevelop:   true,
			ReceivePublish: false,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		if err != nil {
			return nil, fmt.Errorf("create personal space failed: %w", err)
		}

		spaceID = sid
	}

	newUser := &model.User{
		ID:           userID,
		IconURI:      uploadEntity.UserIconURI,
		Name:         name,
		UniqueName:   u.getUniqueNameFormEmail(ctx, req.Email),
		Email:        req.Email,
		Password:     hashedPassword,
		Description:  req.Description,
		UserVerified: false,
		Locale:       req.Locale,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = u.UserRepo.CreateUser(ctx, newUser)
	if err != nil {
		return nil, fmt.Errorf("insert user failed: %w", err)
	}

	err = u.SpaceRepo.AddSpaceUser(ctx, &model.SpaceUser{
		SpaceID:   spaceID,
		UserID:    userID,
		RoleType:  1,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("add space user failed: %w", err)
	}

	iconURL, err := u.IconOSS.GetObjectUrl(ctx, newUser.IconURI)
	if err != nil {
		return nil, fmt.Errorf("get icon url failed: %w", err)
	}

	return userPo2Do(newUser, iconURL), nil
}

func (u *userImpl) CreateSpace(ctx context.Context, req *CreateSpaceRequest) (*userEntity.Space, error) {
	if req == nil {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "missing request"))
	}
	if req.UserID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid user id"))
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "space name cannot be empty"))
	}

	spaceType := req.SpaceType
	if spaceType == 0 {
		spaceType = userEntity.SpaceTypeTeam
	}
	if spaceType != userEntity.SpaceTypePersonal && spaceType != userEntity.SpaceTypeTeam {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid space type"))
	}

	iconURI := strings.TrimSpace(req.IconURI)
	if iconURI == "" {
		iconURI = uploadEntity.EnterpriseIconURI
	}

	spaceID, err := u.IDGen.GenID(ctx)
	if err != nil {
		return nil, fmt.Errorf("gen space id failed: %w", err)
	}

	now := time.Now().UnixMilli()
	spaceModel := &model.Space{
		ID:             spaceID,
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		IconURI:        iconURI,
		OwnerID:        req.UserID,
		CreatorID:      req.UserID,
		SpaceType:      int32(spaceType),
		AllowDevelop:   true,
		ReceivePublish: false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := u.SpaceRepo.CreateSpace(ctx, spaceModel); err != nil {
		return nil, fmt.Errorf("create space failed: %w", err)
	}

	if err := u.SpaceRepo.AddSpaceUser(ctx, &model.SpaceUser{
		SpaceID:   spaceID,
		UserID:    req.UserID,
		RoleType:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return nil, fmt.Errorf("add space owner failed: %w", err)
	}

	iconURL, err := u.IconOSS.GetObjectUrl(ctx, iconURI)
	if err != nil {
		return nil, fmt.Errorf("get space icon url failed: %w", err)
	}

	space := spacePo2Do(spaceModel, iconURL)
	space.SpaceType = spaceType
	space.RoleType = 1
	space.MemberCount = 1
	return space, nil
}

func (u *userImpl) UpdateSpace(ctx context.Context, req *UpdateSpaceRequest) error {
	if req == nil {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "missing request"))
	}
	if req.SpaceID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid space id"))
	}

	updates := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "space name cannot be empty"))
		}
		updates["name"] = name
	}
	if req.Description != nil {
		updates["description"] = strings.TrimSpace(*req.Description)
	}
	if req.IconURI != nil {
		iconURI := strings.TrimSpace(*req.IconURI)
		if iconURI != "" {
			updates["icon_uri"] = iconURI
		}
	}
	if req.AllowDevelop != nil {
		updates["allow_develop"] = *req.AllowDevelop
	}
	if req.ReceivePublish != nil {
		updates["receive_publish"] = *req.ReceivePublish
	}
	if len(updates) == 0 {
		return nil
	}

	return u.SpaceRepo.UpdateSpace(ctx, req.SpaceID, updates)
}

func (u *userImpl) AddSpaceMembers(ctx context.Context, members []*AddSpaceMemberRequest) error {
	return u.addSpaceMembers(ctx, 0, members, nil, false)
}

func (u *userImpl) AddSpaceMembersWithNotification(
	ctx context.Context,
	actorID int64,
	members []*AddSpaceMemberRequest,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	if actorID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid actor id"))
	}
	return u.addSpaceMembers(ctx, actorID, members, appendOutbox, true)
}

func (u *userImpl) addSpaceMembers(
	ctx context.Context,
	actorID int64,
	members []*AddSpaceMemberRequest,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
	withNotification bool,
) error {
	if len(members) == 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "members cannot be empty"))
	}

	spaceID := members[0].SpaceID
	if spaceID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid space id"))
	}

	userIDs := make([]int64, 0, len(members))
	seen := make(map[int64]bool, len(members))
	for _, member := range members {
		if member == nil || member.SpaceID != spaceID || member.UserID <= 0 {
			return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid member"))
		}
		if member.RoleType != 2 && member.RoleType != 3 {
			return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid role type"))
		}
		if seen[member.UserID] {
			continue
		}
		seen[member.UserID] = true
		userIDs = append(userIDs, member.UserID)
	}

	userModels, err := u.UserRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return err
	}
	if len(userModels) != len(userIDs) {
		return errorx.New(errno.ErrUserResourceNotFound, errorx.KV("type", "user"))
	}

	existingMembers, err := u.SpaceRepo.GetSpaceUsersBySpaceID(ctx, spaceID)
	if err != nil {
		return err
	}
	existingByUserID := slices.ToMap(existingMembers, func(member *model.SpaceUser) (int64, bool) {
		return member.UserID, true
	})

	now := time.Now().UnixMilli()
	newSpaceUsers := make([]*model.SpaceUser, 0, len(members))
	for _, member := range members {
		if existingByUserID[member.UserID] {
			continue
		}
		spaceUser := &model.SpaceUser{
			SpaceID:   spaceID,
			UserID:    member.UserID,
			RoleType:  member.RoleType,
			CreatedAt: now,
			UpdatedAt: now,
		}
		newSpaceUsers = append(newSpaceUsers, spaceUser)
		existingByUserID[member.UserID] = true
	}

	if len(newSpaceUsers) == 0 {
		return nil
	}
	if withNotification {
		return u.SpaceRepo.AddSpaceUsersWithNotification(ctx, actorID, newSpaceUsers, appendOutbox)
	}
	for _, spaceUser := range newSpaceUsers {
		if err := u.SpaceRepo.AddSpaceUser(ctx, spaceUser); err != nil {
			return err
		}
	}

	return nil
}

func (u *userImpl) UpdateSpaceMemberRole(ctx context.Context, spaceID int64, userID int64, roleType int32) error {
	if spaceID <= 0 || userID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid member"))
	}
	if roleType != 2 && roleType != 3 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid role type"))
	}

	return u.SpaceRepo.UpdateSpaceUserRole(ctx, spaceID, userID, roleType)
}

func (u *userImpl) UpdateSpaceMemberRoleWithNotification(
	ctx context.Context,
	actorID int64,
	spaceID int64,
	userID int64,
	roleType int32,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	if actorID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid actor id"))
	}
	if spaceID <= 0 || userID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid member"))
	}
	if roleType != 2 && roleType != 3 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid role type"))
	}

	return u.SpaceRepo.UpdateSpaceUserRoleWithNotification(ctx, actorID, spaceID, userID, roleType, appendOutbox)
}

func (u *userImpl) RemoveSpaceMember(ctx context.Context, spaceID int64, userID int64) error {
	if spaceID <= 0 || userID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid member"))
	}

	return u.SpaceRepo.RemoveSpaceUser(ctx, spaceID, userID)
}

func (u *userImpl) RemoveSpaceMemberWithNotification(
	ctx context.Context,
	actorID int64,
	spaceID int64,
	userID int64,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	if actorID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid actor id"))
	}
	if spaceID <= 0 || userID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid member"))
	}

	return u.SpaceRepo.RemoveSpaceUserWithNotification(ctx, actorID, spaceID, userID, appendOutbox)
}

func (u *userImpl) IsSpaceMember(ctx context.Context, spaceID int64, userID int64) (bool, error) {
	if spaceID <= 0 || userID <= 0 {
		return false, errorx.New(
			errno.ErrUserInvalidParamCode,
			errorx.KV("msg", "invalid workspace membership lookup"),
		)
	}
	return u.SpaceRepo.HasSpaceUser(ctx, spaceID, userID)
}

func (u *userImpl) TransferSpace(ctx context.Context, spaceID int64, targetUserID int64) error {
	if spaceID <= 0 || targetUserID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid transfer request"))
	}

	spaces, err := u.SpaceRepo.GetSpaceByIDs(ctx, []int64{spaceID})
	if err != nil {
		return err
	}
	if len(spaces) == 0 {
		return errorx.New(errno.ErrUserResourceNotFound, errorx.KV("type", "space"))
	}

	spaceUsers, err := u.SpaceRepo.GetSpaceUsersBySpaceID(ctx, spaceID)
	if err != nil {
		return err
	}

	targetIsMember := false
	for _, spaceUser := range spaceUsers {
		if spaceUser.UserID == targetUserID {
			targetIsMember = true
			break
		}
	}
	if !targetIsMember {
		return errorx.New(errno.ErrUserResourceNotFound, errorx.KV("type", "space member"))
	}

	oldOwnerID := spaces[0].OwnerID
	if err := u.SpaceRepo.UpdateSpaceOwner(ctx, spaceID, targetUserID); err != nil {
		return err
	}
	if oldOwnerID > 0 && oldOwnerID != targetUserID {
		if err := u.SpaceRepo.UpdateSpaceUserRole(ctx, spaceID, oldOwnerID, 2); err != nil {
			return err
		}
	}
	return u.SpaceRepo.UpdateSpaceUserRole(ctx, spaceID, targetUserID, 1)
}

func (u *userImpl) TransferSpaceWithNotification(
	ctx context.Context,
	actorID int64,
	spaceID int64,
	targetUserID int64,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	if actorID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid actor id"))
	}
	if spaceID <= 0 || targetUserID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid transfer request"))
	}

	return u.SpaceRepo.TransferSpaceWithNotification(ctx, actorID, spaceID, targetUserID, appendOutbox)
}

func (u *userImpl) DeleteSpace(ctx context.Context, spaceID int64) error {
	if spaceID <= 0 {
		return errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "invalid space id"))
	}

	return u.SpaceRepo.DeleteSpace(ctx, spaceID)
}

func (u *userImpl) getUniqueNameFormEmail(ctx context.Context, email string) string {
	arr := strings.Split(email, "@")
	if len(arr) != 2 {
		return email
	}

	username := arr[0]

	exist, err := u.UserRepo.CheckUniqueNameExist(ctx, username)
	if err != nil {
		logs.CtxWarnf(ctx, "check unique name exist failed: %v", err)
		return email
	}

	if exist {
		logs.CtxWarnf(ctx, "unique name %s already exist", username)

		return email
	}

	return username
}

func (u *userImpl) ValidateSession(ctx context.Context, sessionKey string) (
	session *userEntity.Session, exist bool, err error,
) {
	// authentication session key
	sessionModel, err := verifySessionKey(sessionKey)
	if err != nil {
		return nil, false, errorx.New(errno.ErrUserAuthenticationFailed, errorx.KV("reason", "access denied"))
	}

	// Retrieve user information from the database
	userModel, exist, err := u.UserRepo.GetUserBySessionKey(ctx, sessionKey)
	if err != nil {
		return nil, false, err
	}

	if !exist {
		return nil, false, nil
	}

	return &userEntity.Session{
		UserID:    userModel.ID,
		Locale:    userModel.Locale,
		UserEmail: userModel.Email,
		CreatedAt: sessionModel.CreatedAt,
		ExpiresAt: sessionModel.ExpiresAt,
	}, true, nil
}

func (u *userImpl) MGetUserProfiles(ctx context.Context, userIDs []int64) (users []*userEntity.User, err error) {
	userModels, err := u.UserRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	users = make([]*userEntity.User, 0, len(userModels))
	for _, um := range userModels {
		// Get image URL
		resURL, err := u.IconOSS.GetObjectUrl(ctx, um.IconURI)
		if err != nil {
			continue // If getting the image URL fails, skip the user
		}

		users = append(users, userPo2Do(um, resURL))
	}

	return users, nil
}

func (u *userImpl) GetUserProfiles(ctx context.Context, userID int64) (user *userEntity.User, err error) {
	userInfos, err := u.MGetUserProfiles(ctx, []int64{userID})
	if err != nil {
		return nil, err
	}

	if len(userInfos) == 0 {
		return nil, errorx.New(errno.ErrUserResourceNotFound, errorx.KV("type", "user"),
			errorx.KV("id", conv.Int64ToStr(userID)))
	}

	return userInfos[0], nil
}

func (u *userImpl) GetUserSpaceList(ctx context.Context, userID int64) (spaces []*userEntity.Space, err error) {
	userSpaces, err := u.SpaceRepo.GetSpaceList(ctx, userID)
	if err != nil {
		return nil, err
	}
	spaceIDs := slices.Transform(userSpaces, func(us *model.SpaceUser) int64 {
		return us.SpaceID
	})
	roleBySpaceID := slices.ToMap(userSpaces, func(us *model.SpaceUser) (int64, int32) {
		return us.SpaceID, us.RoleType
	})

	spaceModels, err := u.SpaceRepo.GetSpaceByIDs(ctx, spaceIDs)
	if err != nil {
		return nil, err
	}
	memberCounts, err := u.SpaceRepo.CountSpaceUsers(ctx, spaceIDs)
	if err != nil {
		return nil, err
	}
	uris := slices.ToMap(spaceModels, func(sm *model.Space) (string, bool) {
		return sm.IconURI, false
	})

	urls := make(map[string]string, len(uris))
	for uri := range uris {
		url, err := u.IconOSS.GetObjectUrl(ctx, uri)
		if err != nil {
			return nil, err
		}
		urls[uri] = url
	}
	return slices.Transform(spaceModels, func(sm *model.Space) *userEntity.Space {
		space := spacePo2Do(sm, urls[sm.IconURI])
		space.RoleType = roleBySpaceID[sm.ID]
		space.MemberCount = memberCounts[sm.ID]
		return space
	}), nil
}

func (u *userImpl) GetUserSpaceBySpaceID(ctx context.Context, spaceID []int64) (spaces []*userEntity.Space, err error) {
	if len(spaceID) == 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "spaceID cannot be empty"))
	}

	spaceModels, err := u.SpaceRepo.GetSpaceByIDs(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	if len(spaceModels) == 0 {
		return []*userEntity.Space{}, nil
	}

	uris := slices.ToMap(spaceModels, func(sm *model.Space) (string, bool) {
		return sm.IconURI, false
	})

	urls := make(map[string]string, len(uris))
	for uri := range uris {
		if uri != "" {
			url, err := u.IconOSS.GetObjectUrl(ctx, uri)
			if err != nil {
				return nil, err
			}
			urls[uri] = url
		}
	}

	return slices.Transform(spaceModels, func(sm *model.Space) *userEntity.Space {
		return spacePo2Do(sm, urls[sm.IconURI])
	}), nil
}

func (u *userImpl) GetSpaceMembers(ctx context.Context, spaceID int64) ([]*userEntity.SpaceMember, error) {
	if spaceID <= 0 {
		return nil, errorx.New(errno.ErrUserInvalidParamCode, errorx.KV("msg", "spaceID cannot be empty"))
	}

	spaceUsers, err := u.SpaceRepo.GetSpaceUsersBySpaceID(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	if len(spaceUsers) == 0 {
		return []*userEntity.SpaceMember{}, nil
	}

	userIDs := slices.Transform(spaceUsers, func(su *model.SpaceUser) int64 {
		return su.UserID
	})
	userModels, err := u.UserRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	uris := slices.ToMap(userModels, func(um *model.User) (string, bool) {
		return um.IconURI, false
	})
	urls := make(map[string]string, len(uris))
	for uri := range uris {
		if uri == "" {
			continue
		}
		url, err := u.IconOSS.GetObjectUrl(ctx, uri)
		if err != nil {
			return nil, err
		}
		urls[uri] = url
	}

	userByID := slices.ToMap(userModels, func(um *model.User) (int64, *model.User) {
		return um.ID, um
	})
	members := make([]*userEntity.SpaceMember, 0, len(spaceUsers))
	for _, su := range spaceUsers {
		um := userByID[su.UserID]
		if um == nil {
			continue
		}
		members = append(members, &userEntity.SpaceMember{
			UserID:     su.UserID,
			Name:       um.Name,
			UniqueName: um.UniqueName,
			Email:      um.Email,
			AvatarURL:  urls[um.IconURI],
			RoleType:   su.RoleType,
			JoinedAt:   su.CreatedAt,
		})
	}

	return members, nil
}

func (u *userImpl) ListAllUsers(ctx context.Context, keyword string, offset int, limit int) ([]*userEntity.User, int64, error) {
	userModels, total, err := u.UserRepo.ListUsers(ctx, keyword, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	users := make([]*userEntity.User, 0, len(userModels))
	for _, userModel := range userModels {
		iconURL := ""
		if userModel.IconURI != "" {
			url, err := u.IconOSS.GetObjectUrl(ctx, userModel.IconURI)
			if err != nil {
				return nil, 0, err
			}
			iconURL = url
		}
		users = append(users, userPo2Do(userModel, iconURL))
	}

	return users, total, nil
}

func (u *userImpl) ListAllSpaces(ctx context.Context, keyword string, offset int, limit int) ([]*userEntity.Space, int64, error) {
	spaceModels, total, err := u.SpaceRepo.ListSpaces(ctx, keyword, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	spaceIDs := slices.Transform(spaceModels, func(sm *model.Space) int64 {
		return sm.ID
	})
	memberCounts, err := u.SpaceRepo.CountSpaceUsers(ctx, spaceIDs)
	if err != nil {
		return nil, 0, err
	}

	spaces := make([]*userEntity.Space, 0, len(spaceModels))
	for _, spaceModel := range spaceModels {
		iconURL := ""
		if spaceModel.IconURI != "" {
			url, err := u.IconOSS.GetObjectUrl(ctx, spaceModel.IconURI)
			if err != nil {
				return nil, 0, err
			}
			iconURL = url
		}
		space := spacePo2Do(spaceModel, iconURL)
		space.MemberCount = memberCounts[spaceModel.ID]
		spaces = append(spaces, space)
	}

	return spaces, total, nil
}

func spacePo2Do(space *model.Space, iconUrl string) *userEntity.Space {
	return &userEntity.Space{
		ID:             space.ID,
		Name:           space.Name,
		Description:    space.Description,
		IconURL:        iconUrl,
		SpaceType:      spaceTypePo2Do(space),
		OwnerID:        space.OwnerID,
		CreatorID:      space.CreatorID,
		AllowDevelop:   space.AllowDevelop,
		ReceivePublish: space.ReceivePublish,
		CreatedAt:      space.CreatedAt,
		UpdatedAt:      space.UpdatedAt,
	}
}

func spaceTypePo2Do(space *model.Space) userEntity.SpaceType {
	if space == nil {
		return userEntity.SpaceTypeTeam
	}
	switch userEntity.SpaceType(space.SpaceType) {
	case userEntity.SpaceTypePersonal:
		return userEntity.SpaceTypePersonal
	case userEntity.SpaceTypeTeam:
		return userEntity.SpaceTypeTeam
	default:
		return userEntity.SpaceTypeTeam
	}
}

// Argon2id parameter
type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

// Default Argon2id parameters
var defaultArgon2Params = &argon2Params{
	memory:      64 * 1024, // 64MB
	iterations:  3,
	parallelism: 4,
	saltLength:  16,
	keyLength:   32,
}

// Hashing passwords using the Argon2id algorithm
func hashPassword(password string) (string, error) {
	p := defaultArgon2Params

	// Generate random salt values
	salt := make([]byte, p.saltLength)
	_, err := rand.Read(salt)
	if err != nil {
		return "", err
	}

	// Calculate the hash value using the Argon2id algorithm
	hash := argon2.IDKey(
		[]byte(password),
		salt,
		p.iterations,
		p.memory,
		p.parallelism,
		p.keyLength,
	)

	// Encoding to base64 format
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	// Format: $argon2id $v = 19 $m = 65536, t = 3, p = 4 $< salt > $< hash >
	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		p.memory, p.iterations, p.parallelism, b64Salt, b64Hash)

	return encoded, nil
}

// Verify that the passwords match
func verifyPassword(password, encodedHash string) (bool, error) {
	// Parse the encoded hash string
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format")
	}

	var p argon2Params
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism)
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	p.saltLength = uint32(len(salt))

	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	p.keyLength = uint32(len(decodedHash))

	// Calculate the hash value using the same parameters and salt values
	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		p.iterations,
		p.memory,
		p.parallelism,
		p.keyLength,
	)

	// Compare the calculated hash value with the stored hash value
	return subtle.ConstantTimeCompare(decodedHash, computedHash) == 1, nil
}

// Session structure, which contains session information
type Session struct {
	ID        int64     `json:"id"`         // Session unique device identifier
	CreatedAt time.Time `json:"created_at"` // creation time
	ExpiresAt time.Time `json:"expires_at"` // expiration time
}

// The key used for signing (in practice you should read from the configuration or use environment variables)
var hmacSecret = []byte("opencoze-session-hmac-key")

// Generate a secure session key
func generateSessionKey(sessionID int64) (string, error) {
	// Create the default session structure (without the user ID, which will be set in the Login method)
	session := Session{
		ID:        sessionID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(consts.DefaultSessionDuration),
	}

	// Serialize session data
	sessionData, err := json.Marshal(session)
	if err != nil {
		return "", err
	}

	// Calculate HMAC signatures to ensure integrity
	h := hmac.New(sha256.New, hmacSecret)
	h.Write(sessionData)
	signature := h.Sum(nil)

	// Combining session data and signatures
	finalData := append(sessionData, signature...)

	// Base64 encoding final result
	return base64.RawURLEncoding.EncodeToString(finalData), nil
}

// Verify the validity of the session key
func verifySessionKey(sessionKey string) (*Session, error) {
	// Decode session data
	data, err := base64.RawURLEncoding.DecodeString(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("invalid session format: %w", err)
	}

	// Make sure the data is long enough to include at least session data and signatures
	if len(data) < 32 { // Simple inspection should actually be more rigorous
		return nil, fmt.Errorf("session data too short")
	}

	// Separating session data and signatures
	sessionData := data[:len(data)-32] // Assume the signature is 32 bytes
	signature := data[len(data)-32:]

	// verify signature
	h := hmac.New(sha256.New, hmacSecret)
	h.Write(sessionData)
	expectedSignature := h.Sum(nil)

	if !hmac.Equal(signature, expectedSignature) {
		return nil, fmt.Errorf("invalid session signature")
	}

	// Parsing session data
	var session Session
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return nil, fmt.Errorf("invalid session data: %w", err)
	}

	// Check if the session has expired
	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}

	return &session, nil
}

func userPo2Do(model *model.User, iconURL string) *userEntity.User {
	return &userEntity.User{
		UserID:       model.ID,
		Name:         model.Name,
		UniqueName:   model.UniqueName,
		Email:        model.Email,
		Description:  model.Description,
		IconURI:      model.IconURI,
		IconURL:      iconURL,
		UserVerified: model.UserVerified,
		Locale:       model.Locale,
		SessionKey:   model.SessionKey,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}
