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
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestGetUserSpaceListPreservesRoleAndMemberCount(t *testing.T) {
	t.Parallel()

	domain := NewUserDomain(context.Background(), &Components{
		IconOSS:  fakeSpaceStorage{},
		UserRepo: &spaceListUserRepo{},
		SpaceRepo: &spaceListSpaceRepo{
			userSpaces: []*model.SpaceUser{
				{SpaceID: 101, UserID: 9, RoleType: 2},
				{SpaceID: 102, UserID: 9, RoleType: 3},
			},
			spaces: []*model.Space{
				{
					ID:          101,
					Name:        "团队空间",
					Description: "协作",
					IconURI:     "space/team.png",
					OwnerID:     7,
				},
				{
					ID:          102,
					Name:        "Personal Space",
					Description: "This is your personal space",
					IconURI:     "space/personal.png",
					OwnerID:     9,
				},
			},
			memberCounts: map[int64]int64{
				101: 5,
				102: 1,
			},
		},
	})

	spaces, err := domain.GetUserSpaceList(context.Background(), 9)

	require.NoError(t, err)
	require.Len(t, spaces, 2)
	require.Equal(t, int32(2), spaces[0].RoleType)
	require.Equal(t, userEntity.SpaceTypeTeam, spaces[0].SpaceType)
	require.Equal(t, int64(5), spaces[0].MemberCount)
	require.Equal(t, "https://cdn.example.test/space/team.png", spaces[0].IconURL)
	require.Equal(t, int32(3), spaces[1].RoleType)
	require.Equal(t, userEntity.SpaceTypePersonal, spaces[1].SpaceType)
	require.Equal(t, int64(1), spaces[1].MemberCount)
}

func TestIsSpaceMemberUsesTargetedRepositoryLookup(t *testing.T) {
	t.Parallel()

	repo := &spaceListSpaceRepo{hasSpaceUser: true}
	domain := NewUserDomain(context.Background(), &Components{
		IconOSS:   fakeSpaceStorage{},
		UserRepo:  &spaceListUserRepo{},
		SpaceRepo: repo,
	})

	member, err := domain.IsSpaceMember(context.Background(), 101, 9)

	require.NoError(t, err)
	require.True(t, member)
	require.Equal(t, int64(101), repo.hasSpaceUserSpaceID)
	require.Equal(t, int64(9), repo.hasSpaceUserUserID)
}

func TestCreateSpaceCreatesTeamSpaceAndOwnerMembership(t *testing.T) {
	t.Parallel()

	spaceRepo := &spaceListSpaceRepo{}
	domain := NewUserDomain(context.Background(), &Components{
		IconOSS:   fakeSpaceStorage{},
		IDGen:     fixedSpaceIDGen{ids: []int64{201}},
		UserRepo:  &spaceListUserRepo{},
		SpaceRepo: spaceRepo,
	})

	space, err := domain.CreateSpace(context.Background(), &CreateSpaceRequest{
		UserID:      9,
		Name:        "  畅享 AI  ",
		Description: "  团队协作空间  ",
		IconURI:     "space/team.png",
		SpaceType:   userEntity.SpaceTypeTeam,
	})

	require.NoError(t, err)
	require.Equal(t, int64(201), space.ID)
	require.Equal(t, "畅享 AI", space.Name)
	require.Equal(t, "团队协作空间", space.Description)
	require.Equal(t, userEntity.SpaceTypeTeam, space.SpaceType)
	require.Equal(t, int64(9), space.OwnerID)
	require.Equal(t, int32(1), space.RoleType)
	require.Equal(t, int64(1), space.MemberCount)
	require.Equal(t, "https://cdn.example.test/space/team.png", space.IconURL)
	require.Equal(t, int64(201), spaceRepo.createdSpace.ID)
	require.Equal(t, int64(9), spaceRepo.createdSpace.OwnerID)
	require.Equal(t, int64(201), spaceRepo.addedSpaceUser.SpaceID)
	require.Equal(t, int64(9), spaceRepo.addedSpaceUser.UserID)
	require.Equal(t, int32(1), spaceRepo.addedSpaceUser.RoleType)
}

func TestAddSpaceMembersSkipsExistingMembers(t *testing.T) {
	t.Parallel()

	spaceRepo := &spaceListSpaceRepo{
		spaceUsers: []*model.SpaceUser{
			{SpaceID: 101, UserID: 10, RoleType: 3},
		},
	}
	domain := NewUserDomain(context.Background(), &Components{
		IconOSS: fakeSpaceStorage{},
		UserRepo: &spaceListUserRepo{
			usersByID: map[int64]*model.User{
				10: {ID: 10, Name: "Member"},
				11: {ID: 11, Name: "Admin"},
			},
		},
		SpaceRepo: spaceRepo,
	})

	err := domain.AddSpaceMembers(context.Background(), []*AddSpaceMemberRequest{
		{SpaceID: 101, UserID: 10, RoleType: 3},
		{SpaceID: 101, UserID: 11, RoleType: 2},
	})

	require.NoError(t, err)
	require.Len(t, spaceRepo.addedSpaceUsers, 1)
	require.Equal(t, int64(101), spaceRepo.addedSpaceUsers[0].SpaceID)
	require.Equal(t, int64(11), spaceRepo.addedSpaceUsers[0].UserID)
	require.Equal(t, int32(2), spaceRepo.addedSpaceUsers[0].RoleType)
}

func TestUpdateSpacePersistsWorkspaceSettings(t *testing.T) {
	t.Parallel()

	spaceRepo := &spaceListSpaceRepo{}
	domain := NewUserDomain(context.Background(), &Components{
		IconOSS:   fakeSpaceStorage{},
		UserRepo:  &spaceListUserRepo{},
		SpaceRepo: spaceRepo,
	})
	allowDevelop := false
	receivePublish := true

	err := domain.UpdateSpace(context.Background(), &UpdateSpaceRequest{
		SpaceID:        101,
		AllowDevelop:   &allowDevelop,
		ReceivePublish: &receivePublish,
	})

	require.NoError(t, err)
	require.Equal(t, int64(101), spaceRepo.updatedSpaceID)
	require.Equal(t, false, spaceRepo.updatedSpace["allow_develop"])
	require.Equal(t, true, spaceRepo.updatedSpace["receive_publish"])
}

type spaceListUserRepo struct {
	usersByID map[int64]*model.User
}

func (r *spaceListUserRepo) GetUsersByEmail(context.Context, string) (*model.User, bool, error) {
	return nil, false, nil
}
func (r *spaceListUserRepo) UpdateSessionKey(context.Context, int64, string) error { return nil }
func (r *spaceListUserRepo) ClearSessionKey(context.Context, int64) error          { return nil }
func (r *spaceListUserRepo) UpdatePassword(context.Context, string, string) error  { return nil }
func (r *spaceListUserRepo) GetUserByID(context.Context, int64) (*model.User, error) {
	return nil, nil
}
func (r *spaceListUserRepo) UpdateAvatar(context.Context, int64, string) error { return nil }
func (r *spaceListUserRepo) CheckUniqueNameExist(context.Context, string) (bool, error) {
	return false, nil
}
func (r *spaceListUserRepo) UpdateProfile(context.Context, int64, map[string]any) error {
	return nil
}
func (r *spaceListUserRepo) CheckEmailExist(context.Context, string) (bool, error) {
	return false, nil
}
func (r *spaceListUserRepo) CreateUser(context.Context, *model.User) error { return nil }
func (r *spaceListUserRepo) GetUserBySessionKey(context.Context, string) (*model.User, bool, error) {
	return nil, false, nil
}
func (r *spaceListUserRepo) GetUsersByIDs(context.Context, []int64) ([]*model.User, error) {
	users := make([]*model.User, 0)
	for _, user := range r.usersByID {
		users = append(users, user)
	}
	return users, nil
}
func (r *spaceListUserRepo) ListUsers(context.Context, string, int, int) ([]*model.User, int64, error) {
	return nil, 0, nil
}

type spaceListSpaceRepo struct {
	userSpaces          []*model.SpaceUser
	spaceUsers          []*model.SpaceUser
	spaces              []*model.Space
	memberCounts        map[int64]int64
	hasSpaceUser        bool
	hasSpaceUserErr     error
	hasSpaceUserSpaceID int64
	hasSpaceUserUserID  int64
	createdSpace        *model.Space
	addedSpaceUser      *model.SpaceUser
	addedSpaceUsers     []*model.SpaceUser
	updatedSpaceID      int64
	updatedSpace        map[string]any
	updatedRole         *model.SpaceUser
	removedSpaceID      int64
	removedUserID       int64
}

func (r *spaceListSpaceRepo) CreateSpace(_ context.Context, space *model.Space) error {
	r.createdSpace = space
	return nil
}
func (r *spaceListSpaceRepo) UpdateSpace(_ context.Context, spaceID int64, updates map[string]any) error {
	r.updatedSpaceID = spaceID
	r.updatedSpace = updates
	return nil
}
func (r *spaceListSpaceRepo) UpdateSpaceOwner(_ context.Context, spaceID int64, ownerID int64) error {
	r.updatedSpaceID = spaceID
	r.updatedSpace = map[string]any{"owner_id": ownerID}
	return nil
}
func (r *spaceListSpaceRepo) DeleteSpace(_ context.Context, spaceID int64) error {
	r.removedSpaceID = spaceID
	return nil
}
func (r *spaceListSpaceRepo) GetSpaceByIDs(_ context.Context, spaceIDs []int64) ([]*model.Space, error) {
	spaceByID := make(map[int64]*model.Space, len(r.spaces))
	for _, space := range r.spaces {
		spaceByID[space.ID] = space
	}

	spaces := make([]*model.Space, 0, len(spaceIDs))
	for _, spaceID := range spaceIDs {
		if space := spaceByID[spaceID]; space != nil {
			spaces = append(spaces, space)
		}
	}

	return spaces, nil
}
func (r *spaceListSpaceRepo) AddSpaceUser(_ context.Context, spaceUser *model.SpaceUser) error {
	r.addedSpaceUser = spaceUser
	r.addedSpaceUsers = append(r.addedSpaceUsers, spaceUser)
	return nil
}
func (r *spaceListSpaceRepo) UpdateSpaceUserRole(_ context.Context, spaceID int64, userID int64, roleType int32) error {
	r.updatedRole = &model.SpaceUser{
		SpaceID:  spaceID,
		UserID:   userID,
		RoleType: roleType,
	}
	return nil
}
func (r *spaceListSpaceRepo) RemoveSpaceUser(_ context.Context, spaceID int64, userID int64) error {
	r.removedSpaceID = spaceID
	r.removedUserID = userID
	return nil
}
func (r *spaceListSpaceRepo) GetSpaceList(context.Context, int64) ([]*model.SpaceUser, error) {
	return r.userSpaces, nil
}
func (r *spaceListSpaceRepo) HasSpaceUser(_ context.Context, spaceID int64, userID int64) (bool, error) {
	r.hasSpaceUserSpaceID = spaceID
	r.hasSpaceUserUserID = userID
	return r.hasSpaceUser, r.hasSpaceUserErr
}
func (r *spaceListSpaceRepo) GetSpaceUsersBySpaceID(context.Context, int64) ([]*model.SpaceUser, error) {
	return r.spaceUsers, nil
}
func (r *spaceListSpaceRepo) CountSpaceUsers(context.Context, []int64) (map[int64]int64, error) {
	return r.memberCounts, nil
}
func (r *spaceListSpaceRepo) ListSpaces(context.Context, string, int, int) ([]*model.Space, int64, error) {
	return r.spaces, int64(len(r.spaces)), nil
}

type fakeSpaceStorage struct{}

func (fakeSpaceStorage) PutObject(context.Context, string, []byte, ...storage.PutOptFn) error {
	return nil
}
func (fakeSpaceStorage) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return nil
}
func (fakeSpaceStorage) GetObject(context.Context, string) ([]byte, error) { return nil, nil }
func (fakeSpaceStorage) DeleteObject(context.Context, string) error        { return nil }
func (fakeSpaceStorage) GetObjectUrl(_ context.Context, objectKey string, _ ...storage.GetOptFn) (string, error) {
	return "https://cdn.example.test/" + objectKey, nil
}
func (fakeSpaceStorage) HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error) {
	return nil, nil
}
func (fakeSpaceStorage) ListAllObjects(context.Context, string, ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	return nil, nil
}
func (fakeSpaceStorage) ListObjectsPaginated(context.Context, *storage.ListObjectsPaginatedInput, ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	return nil, nil
}

type fixedSpaceIDGen struct {
	ids []int64
}

func (g fixedSpaceIDGen) GenID(context.Context) (int64, error) {
	if len(g.ids) == 0 {
		return 0, nil
	}
	return g.ids[0], nil
}

func (g fixedSpaceIDGen) GenMultiIDs(context.Context, int) ([]int64, error) {
	return g.ids, nil
}
