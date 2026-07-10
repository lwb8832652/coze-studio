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
	"testing"

	"github.com/stretchr/testify/require"

	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	userservice "github.com/coze-dev/coze-studio/backend/domain/user/service"
)

func TestListAdminWorkspacesReturnsOwnerAndMemberCount(t *testing.T) {
	t.Parallel()

	app := &ManagementApplicationService{
		UserDomainSVC: &fakeAdminUserDomain{
			usersByID: map[int64]*userentity.User{
				9: {UserID: 9, Name: "Owner", Email: "owner@example.test"},
			},
			spaces: []*userentity.Space{
				{
					ID:          101,
					Name:        "畅享 AI",
					Description: "团队协作空间",
					OwnerID:     9,
					MemberCount: 2,
					CreatedAt:   1717000000,
				},
			},
			total: 1,
		},
	}

	resp, err := app.ListAdminWorkspaces(context.Background(), &ListAdminWorkspacesRequest{
		Keyword: "畅享",
		Page:    1,
		Size:    20,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Workspaces, 1)
	require.Equal(t, int64(101), resp.Workspaces[0].ID)
	require.Equal(t, "畅享 AI", resp.Workspaces[0].Name)
	require.Equal(t, int64(9), resp.Workspaces[0].OwnerUserID)
	require.Equal(t, "Owner", resp.Workspaces[0].OwnerName)
	require.Equal(t, int64(2), resp.Workspaces[0].TotalMemberNum)
}

func TestListAdminUsersReturnsUserBasics(t *testing.T) {
	t.Parallel()

	app := &ManagementApplicationService{
		UserDomainSVC: &fakeAdminUserDomain{
			users: []*userentity.User{
				{
					UserID:     9,
					Name:       "Owner",
					UniqueName: "owner",
					Email:      "owner@example.test",
					CreatedAt:  1717000000,
				},
			},
			total: 1,
		},
	}

	resp, err := app.ListAdminUsers(context.Background(), &ListAdminUsersRequest{
		Keyword: "owner",
		Page:    1,
		Size:    20,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Users, 1)
	require.Equal(t, int64(9), resp.Users[0].UserID)
	require.Equal(t, "Owner", resp.Users[0].Name)
	require.Equal(t, "owner@example.test", resp.Users[0].Email)
	require.Equal(t, "owner", resp.Users[0].UniqueName)
}

func TestCreateAdminUserCreatesUserWithPersonalSpace(t *testing.T) {
	t.Parallel()

	domain := &fakeAdminUserDomain{}
	app := &ManagementApplicationService{
		UserDomainSVC: domain,
	}

	resp, err := app.CreateAdminUser(context.Background(), &CreateAdminUserRequest{
		Email:      "new@example.test",
		Password:   "secret1",
		Name:       "New User",
		UniqueName: "new-user",
		Locale:     "zh-CN",
	})

	require.NoError(t, err)
	require.Equal(t, int64(99), resp.User.UserID)
	require.Equal(t, "new@example.test", domain.createdUser.Email)
	require.Equal(t, "secret1", domain.createdUser.Password)
	require.Equal(t, "New User", domain.createdUser.Name)
	require.Equal(t, "new-user", domain.createdUser.UniqueName)
}

func TestUpdateAdminUserUpdatesProfileFields(t *testing.T) {
	t.Parallel()

	domain := &fakeAdminUserDomain{}
	app := &ManagementApplicationService{
		UserDomainSVC: domain,
	}

	_, err := app.UpdateAdminUser(context.Background(), &UpdateAdminUserRequest{
		UserID:     9,
		Name:       "Owner Edited",
		UniqueName: "owner-edited",
		Locale:     "zh-CN",
	})

	require.NoError(t, err)
	require.Equal(t, int64(9), domain.updatedProfile.UserID)
	require.Equal(t, "Owner Edited", *domain.updatedProfile.Name)
	require.Equal(t, "owner-edited", *domain.updatedProfile.UniqueName)
	require.Equal(t, "zh-CN", *domain.updatedProfile.Locale)
}

func TestResetAdminUserPasswordUsesUserEmail(t *testing.T) {
	t.Parallel()

	domain := &fakeAdminUserDomain{
		usersByID: map[int64]*userentity.User{
			9: {UserID: 9, Email: "owner@example.test"},
		},
	}
	app := &ManagementApplicationService{
		UserDomainSVC: domain,
	}

	_, err := app.ResetAdminUserPassword(context.Background(), &ResetAdminUserPasswordRequest{
		UserID:   9,
		Password: "secret2",
	})

	require.NoError(t, err)
	require.Equal(t, "owner@example.test", domain.resetPasswordEmail)
	require.Equal(t, "secret2", domain.resetPasswordValue)
}

func TestListAdminWorkspaceMembersReturnsMemberBasics(t *testing.T) {
	t.Parallel()

	app := &ManagementApplicationService{
		UserDomainSVC: &fakeAdminUserDomain{
			members: []*userentity.SpaceMember{
				{
					UserID:     9,
					Name:       "Owner",
					UniqueName: "owner",
					Email:      "owner@example.test",
					RoleType:   2,
					JoinedAt:   1717000000,
				},
			},
		},
	}

	resp, err := app.ListAdminWorkspaceMembers(context.Background(), &ListAdminWorkspaceMembersRequest{
		SpaceID: 101,
	})

	require.NoError(t, err)
	require.Len(t, resp.Members, 1)
	require.Equal(t, int64(9), resp.Members[0].UserID)
	require.Equal(t, "Owner", resp.Members[0].Name)
	require.Equal(t, "owner@example.test", resp.Members[0].Email)
	require.Equal(t, int32(2), resp.Members[0].RoleType)
}

func TestListAdminUserSpacesReturnsSpaceRoles(t *testing.T) {
	t.Parallel()

	app := &ManagementApplicationService{
		UserDomainSVC: &fakeAdminUserDomain{
			usersByID: map[int64]*userentity.User{
				9: {UserID: 9, Name: "Owner", Email: "owner@example.test"},
			},
			userSpaces: []*userentity.Space{
				{
					ID:          101,
					Name:        "畅享 AI",
					Description: "团队协作空间",
					OwnerID:     9,
					RoleType:    2,
					MemberCount: 3,
					CreatedAt:   1717000000,
				},
			},
		},
	}

	resp, err := app.ListAdminUserSpaces(context.Background(), &ListAdminUserSpacesRequest{
		UserID: 9,
	})

	require.NoError(t, err)
	require.Len(t, resp.Spaces, 1)
	require.Equal(t, int64(101), resp.Spaces[0].ID)
	require.Equal(t, "畅享 AI", resp.Spaces[0].Name)
	require.Equal(t, int64(9), resp.Spaces[0].OwnerUserID)
	require.Equal(t, "Owner", resp.Spaces[0].OwnerName)
	require.Equal(t, int32(2), resp.Spaces[0].RoleType)
	require.Equal(t, int64(3), resp.Spaces[0].TotalMemberNum)
}

type fakeAdminUserDomain struct {
	users              []*userentity.User
	usersByID          map[int64]*userentity.User
	spaces             []*userentity.Space
	userSpaces         []*userentity.Space
	members            []*userentity.SpaceMember
	total              int64
	createdUser        *userservice.CreateUserRequest
	updatedProfile     *userservice.UpdateProfileRequest
	resetPasswordEmail string
	resetPasswordValue string
}

func (d *fakeAdminUserDomain) ListAllUsers(context.Context, string, int, int) ([]*userentity.User, int64, error) {
	return d.users, d.total, nil
}

func (d *fakeAdminUserDomain) ListAllSpaces(context.Context, string, int, int) ([]*userentity.Space, int64, error) {
	return d.spaces, d.total, nil
}

func (d *fakeAdminUserDomain) MGetUserProfiles(_ context.Context, userIDs []int64) ([]*userentity.User, error) {
	users := make([]*userentity.User, 0, len(userIDs))
	for _, userID := range userIDs {
		if user := d.usersByID[userID]; user != nil {
			users = append(users, user)
		}
	}
	return users, nil
}

func (d *fakeAdminUserDomain) GetSpaceMembers(context.Context, int64) ([]*userentity.SpaceMember, error) {
	return d.members, nil
}

func (d *fakeAdminUserDomain) GetUserSpaceList(context.Context, int64) ([]*userentity.Space, error) {
	return d.userSpaces, nil
}

func (d *fakeAdminUserDomain) Create(_ context.Context, req *userservice.CreateUserRequest) (*userentity.User, error) {
	d.createdUser = req
	return &userentity.User{
		UserID:     99,
		Name:       req.Name,
		UniqueName: req.UniqueName,
		Email:      req.Email,
		Locale:     req.Locale,
	}, nil
}

func (d *fakeAdminUserDomain) UpdateProfile(_ context.Context, req *userservice.UpdateProfileRequest) error {
	d.updatedProfile = req
	return nil
}

func (d *fakeAdminUserDomain) ResetPassword(_ context.Context, email string, password string) error {
	d.resetPasswordEmail = email
	d.resetPasswordValue = password
	return nil
}

func (d *fakeAdminUserDomain) GetUserInfo(_ context.Context, userID int64) (*userentity.User, error) {
	if user := d.usersByID[userID]; user != nil {
		return user, nil
	}
	return &userentity.User{UserID: userID, Email: "fallback@example.test"}, nil
}
