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
	"testing"

	"github.com/stretchr/testify/require"

	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	userservice "github.com/coze-dev/coze-studio/backend/domain/user/service"
)

func TestGetWorkspaceDetailReturnsCurrentUserRoleAndMemberCount(t *testing.T) {
	t.Parallel()

	app := &ApplicationService{
		UserDomainSVC: &fakeUserDomain{
			spaces: []*userentity.Space{
				{
					ID:             101,
					Name:           "畅享 AI",
					Description:    "团队协作空间",
					IconURL:        "https://cdn.example.test/space.png",
					OwnerID:        9,
					AllowDevelop:   true,
					ReceivePublish: false,
				},
			},
			members: []*userentity.SpaceMember{
				{
					UserID:    9,
					Name:      "Owner",
					RoleType:  1,
					AvatarURL: "https://cdn.example.test/owner.png",
					JoinedAt:  1717000000,
				},
				{
					UserID:   10,
					Name:     "Member",
					RoleType: 3,
					JoinedAt: 1717000100,
				},
			},
		},
	}

	resp, err := app.GetWorkspaceDetail(context.Background(), &GetWorkspaceDetailRequest{
		SpaceID:       101,
		CurrentUserID: 9,
	})

	require.NoError(t, err)
	require.Equal(t, int64(101), resp.Data.ID)
	require.Equal(t, "畅享 AI", resp.Data.Name)
	require.Equal(t, "团队协作空间", resp.Data.Description)
	require.Equal(t, int64(9), resp.Data.OwnerUserID)
	require.Equal(t, int32(1), resp.Data.CurrentUserRole)
	require.Equal(t, int64(2), resp.Data.TotalMemberNum)
	require.True(t, resp.Data.AllowDevelop)
	require.False(t, resp.Data.ReceivePublish)
}

func TestGetWorkspaceDetailRejectsSpaceOutsideCurrentUserMembership(t *testing.T) {
	t.Parallel()

	app := &ApplicationService{
		UserDomainSVC: &fakeUserDomain{
			spaces: []*userentity.Space{
				{ID: 202, Name: "其他空间"},
			},
			members: []*userentity.SpaceMember{
				{UserID: 9, Name: "Owner", RoleType: 1},
			},
		},
	}

	_, err := app.GetWorkspaceDetail(context.Background(), &GetWorkspaceDetailRequest{
		SpaceID:       101,
		CurrentUserID: 9,
	})

	require.Error(t, err)
}

func TestListWorkspaceMembersFiltersByKeywordAndRole(t *testing.T) {
	t.Parallel()

	app := &ApplicationService{
		UserDomainSVC: &fakeUserDomain{
			spaces: []*userentity.Space{
				{ID: 101, Name: "畅享 AI"},
			},
			members: []*userentity.SpaceMember{
				{
					UserID:     9,
					Name:       "Owner",
					UniqueName: "owner",
					Email:      "owner@example.test",
					RoleType:   1,
					JoinedAt:   1717000000,
				},
				{
					UserID:     10,
					Name:       "Member",
					UniqueName: "member",
					Email:      "member@example.test",
					RoleType:   3,
					JoinedAt:   1717000100,
				},
			},
		},
	}

	resp, err := app.ListWorkspaceMembers(context.Background(), &ListWorkspaceMembersRequest{
		SpaceID:       101,
		CurrentUserID: 9,
		Keyword:       "owner",
		RoleType:      1,
	})

	require.NoError(t, err)
	require.Len(t, resp.Members, 1)
	require.Equal(t, int64(9), resp.Members[0].UserID)
	require.Equal(t, "owner@example.test", resp.Members[0].Email)
	require.Equal(t, int32(1), resp.Members[0].RoleType)
}

func TestSearchWorkspaceUsersMarksExistingMemberRole(t *testing.T) {
	t.Parallel()

	app := &ApplicationService{
		UserDomainSVC: &fakeUserDomain{
			spaces: []*userentity.Space{
				{ID: 101, Name: "畅享 AI"},
			},
			members: []*userentity.SpaceMember{
				{UserID: 9, Name: "Owner", RoleType: 1},
				{UserID: 10, Name: "Member", RoleType: 3},
			},
			users: []*userentity.User{
				{UserID: 10, Name: "Member", Email: "member@example.test"},
				{UserID: 11, Name: "New User", Email: "new@example.test"},
			},
		},
	}

	resp, err := app.SearchWorkspaceUsers(context.Background(), &SearchWorkspaceUsersRequest{
		SpaceID:       101,
		CurrentUserID: 9,
		Keyword:       "user",
	})

	require.NoError(t, err)
	require.Len(t, resp.Users, 2)
	require.Equal(t, int32(3), resp.Users[0].RoleType)
	require.Equal(t, int32(0), resp.Users[1].RoleType)
}

func TestUpdateWorkspacePersistsSpaceSettings(t *testing.T) {
	t.Parallel()

	allowDevelop := false
	receivePublish := true
	domain := &fakeUserDomain{
		spaces: []*userentity.Space{
			{ID: 101, Name: "畅享 AI", SpaceType: userentity.SpaceTypeTeam},
		},
		members: []*userentity.SpaceMember{
			{UserID: 9, Name: "Owner", RoleType: 1},
		},
	}
	app := &ApplicationService{UserDomainSVC: domain}

	resp, err := app.UpdateWorkspace(context.Background(), &UpdateWorkspaceRequest{
		SpaceID:        101,
		CurrentUserID:  9,
		AllowDevelop:   &allowDevelop,
		ReceivePublish: &receivePublish,
	})

	require.NoError(t, err)
	require.Equal(t, int64(0), resp.Code)
	require.NotNil(t, domain.updatedSpace)
	require.NotNil(t, domain.updatedSpace.AllowDevelop)
	require.False(t, *domain.updatedSpace.AllowDevelop)
	require.NotNil(t, domain.updatedSpace.ReceivePublish)
	require.True(t, *domain.updatedSpace.ReceivePublish)
}

func TestAddWorkspaceMembersRequiresManagerAndSkipsExistingMembers(t *testing.T) {
	t.Parallel()

	domain := &fakeUserDomain{
		spaces: []*userentity.Space{
			{ID: 101, Name: "畅享 AI"},
		},
		members: []*userentity.SpaceMember{
			{UserID: 9, Name: "Owner", RoleType: 1},
			{UserID: 10, Name: "Member", RoleType: 3},
		},
	}
	app := &ApplicationService{UserDomainSVC: domain}

	resp, err := app.AddWorkspaceMembers(context.Background(), &AddWorkspaceMembersRequest{
		SpaceID:       101,
		CurrentUserID: 9,
		Members: []*WorkspaceMemberMutation{
			{UserID: 10, RoleType: 3},
			{UserID: 11, RoleType: 2},
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(0), resp.Code)
	require.Len(t, domain.addedMembers, 1)
	require.Equal(t, int64(11), domain.addedMembers[0].UserID)
	require.Equal(t, int32(2), domain.addedMembers[0].RoleType)
}

func TestUpdateWorkspaceMemberRoleRejectsOwnerMutation(t *testing.T) {
	t.Parallel()

	app := &ApplicationService{
		UserDomainSVC: &fakeUserDomain{
			spaces: []*userentity.Space{
				{ID: 101, Name: "畅享 AI"},
			},
			members: []*userentity.SpaceMember{
				{UserID: 9, Name: "Owner", RoleType: 1},
				{UserID: 10, Name: "Admin", RoleType: 2},
			},
		},
	}

	_, err := app.UpdateWorkspaceMemberRole(context.Background(), &UpdateWorkspaceMemberRoleRequest{
		SpaceID:       101,
		CurrentUserID: 9,
		UserID:        9,
		RoleType:      3,
	})

	require.Error(t, err)
}

func TestRemoveWorkspaceMemberRejectsOwnerRemoval(t *testing.T) {
	t.Parallel()

	app := &ApplicationService{
		UserDomainSVC: &fakeUserDomain{
			spaces: []*userentity.Space{
				{ID: 101, Name: "畅享 AI"},
			},
			members: []*userentity.SpaceMember{
				{UserID: 9, Name: "Owner", RoleType: 1},
			},
		},
	}

	_, err := app.RemoveWorkspaceMember(context.Background(), &RemoveWorkspaceMemberRequest{
		SpaceID:       101,
		CurrentUserID: 9,
		UserID:        9,
	})

	require.Error(t, err)
}

func TestTransferWorkspaceRequiresOwnerAndTargetMember(t *testing.T) {
	t.Parallel()

	domain := &fakeUserDomain{
		spaces: []*userentity.Space{
			{ID: 101, Name: "畅享 AI", SpaceType: userentity.SpaceTypeTeam},
		},
		members: []*userentity.SpaceMember{
			{UserID: 9, Name: "Owner", RoleType: 1},
			{UserID: 10, Name: "Admin", RoleType: 2},
		},
	}
	app := &ApplicationService{UserDomainSVC: domain}

	resp, err := app.TransferWorkspace(context.Background(), &TransferWorkspaceRequest{
		SpaceID:       101,
		CurrentUserID: 9,
		TargetUserID:  10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, int64(101), domain.transferredSpaceID)
	require.Equal(t, int64(10), domain.transferredTargetUserID)
}

func TestDeleteWorkspaceRejectsPersonalWorkspace(t *testing.T) {
	t.Parallel()

	app := &ApplicationService{
		UserDomainSVC: &fakeUserDomain{
			spaces: []*userentity.Space{
				{ID: 101, Name: "Personal Space", SpaceType: userentity.SpaceTypePersonal},
			},
			members: []*userentity.SpaceMember{
				{UserID: 9, Name: "Owner", RoleType: 1},
			},
		},
	}

	_, err := app.DeleteWorkspace(context.Background(), &DeleteWorkspaceRequest{
		SpaceID:       101,
		CurrentUserID: 9,
	})

	require.Error(t, err)
}

type fakeUserDomain struct {
	spaces                  []*userentity.Space
	members                 []*userentity.SpaceMember
	users                   []*userentity.User
	updatedSpace            *userservice.UpdateSpaceRequest
	addedMembers            []*userservice.AddSpaceMemberRequest
	roleUpdates             []*userservice.AddSpaceMemberRequest
	removedUsers            []int64
	transferredSpaceID      int64
	transferredTargetUserID int64
	deletedSpaceID          int64
}

func (d *fakeUserDomain) GetUserSpaceList(_ context.Context, _ int64) ([]*userentity.Space, error) {
	return d.spaces, nil
}

func (d *fakeUserDomain) GetSpaceMembers(_ context.Context, _ int64) ([]*userentity.SpaceMember, error) {
	return d.members, nil
}

func (d *fakeUserDomain) ListAllUsers(_ context.Context, _ string, _ int, _ int) ([]*userentity.User, int64, error) {
	return d.users, int64(len(d.users)), nil
}

func (d *fakeUserDomain) UpdateSpace(_ context.Context, req *userservice.UpdateSpaceRequest) error {
	d.updatedSpace = req
	return nil
}

func (d *fakeUserDomain) AddSpaceMembers(_ context.Context, members []*userservice.AddSpaceMemberRequest) error {
	d.addedMembers = append(d.addedMembers, members...)
	return nil
}

func (d *fakeUserDomain) UpdateSpaceMemberRole(_ context.Context, spaceID int64, userID int64, roleType int32) error {
	d.roleUpdates = append(d.roleUpdates, &userservice.AddSpaceMemberRequest{
		SpaceID:  spaceID,
		UserID:   userID,
		RoleType: roleType,
	})
	return nil
}

func (d *fakeUserDomain) RemoveSpaceMember(_ context.Context, _ int64, userID int64) error {
	d.removedUsers = append(d.removedUsers, userID)
	return nil
}

func (d *fakeUserDomain) TransferSpace(_ context.Context, spaceID int64, targetUserID int64) error {
	d.transferredSpaceID = spaceID
	d.transferredTargetUserID = targetUserID
	return nil
}

func (d *fakeUserDomain) DeleteSpace(_ context.Context, spaceID int64) error {
	d.deletedSpaceID = spaceID
	return nil
}
