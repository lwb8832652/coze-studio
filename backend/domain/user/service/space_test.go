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
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal"
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
					SpaceType:   int32(userEntity.SpaceTypeTeam),
				},
				{
					ID:          102,
					Name:        "Renamed Personal",
					Description: "Private workspace",
					IconURI:     "space/personal.png",
					OwnerID:     9,
					SpaceType:   int32(userEntity.SpaceTypePersonal),
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
	require.Equal(t, int32(userEntity.SpaceTypeTeam), spaceRepo.createdSpace.SpaceType)
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

func TestAddSpaceMembersWithNotificationDerivesRecipientsFromMembershipRows(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, map[int64]*model.User{
		10: {ID: 10, Name: "New Member"},
		11: {ID: 11, Name: "Second Member"},
	})
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
		{SpaceID: 101, UserID: 11, RoleType: 2, CreatedAt: 1001, UpdatedAt: 1001},
		{SpaceID: 202, UserID: 99, RoleType: 2, CreatedAt: 1002, UpdatedAt: 1002},
	})
	events := make([]domainnotification.Event, 0, 2)

	err := domain.AddSpaceMembersWithNotification(
		context.Background(),
		9,
		[]*AddSpaceMemberRequest{{SpaceID: 101, UserID: 10, RoleType: 3}},
		func(_ context.Context, _ *gorm.DB, event domainnotification.Event) error {
			events = append(events, event)
			return nil
		},
	)

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, domainnotification.EventWorkspaceMembershipChanged, events[0].EventType)
	require.Equal(t, "workspace_member.added.target", events[0].AggregateType)
	require.Equal(t, "workspace_member.added.admins", events[1].AggregateType)
	require.Equal(t, "101:10", events[0].AggregateID)
	require.Equal(t, int64(9), events[0].ActorID)
	require.Equal(t, int64(101), events[0].SpaceID)
	require.Equal(t, domainnotification.WorkspaceMemberAdded, events[0].Payload.WorkspaceAction)
	require.Equal(t, domainnotification.WorkspaceMemberAudienceTarget, events[0].Payload.WorkspaceAudience)
	require.Equal(t, []int64{10}, events[0].Payload.ExplicitRecipientIDs)
	require.Equal(t, domainnotification.WorkspaceMemberAudienceAdmins, events[1].Payload.WorkspaceAudience)
	require.Equal(t, []int64{9, 11}, events[1].Payload.ExplicitRecipientIDs)
	require.Contains(t, events[0].EventID, ":added:target:")
	require.Contains(t, events[1].EventID, ":added:admins:")
	require.Greater(t, events[0].AggregateVersion, int64(0))
}

func TestWorkspaceMembershipRecipientsExcludeOtherSpaceManagers(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, map[int64]*model.User{
		10: {ID: 10, Name: "New Member"},
		11: {ID: 11, Name: "Second Member"},
	})
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
		{SpaceID: 202, UserID: 99, RoleType: 2, CreatedAt: 1001, UpdatedAt: 1001},
	})
	events := make([]domainnotification.Event, 0, 2)

	err := domain.AddSpaceMembersWithNotification(
		context.Background(),
		9,
		[]*AddSpaceMemberRequest{{SpaceID: 101, UserID: 10, RoleType: 3}},
		func(_ context.Context, _ *gorm.DB, event domainnotification.Event) error {
			events = append(events, event)
			return nil
		},
	)

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, []int64{10}, events[0].Payload.ExplicitRecipientIDs)
	require.Equal(t, []int64{9}, events[1].Payload.ExplicitRecipientIDs)
}

func TestAddSpaceMembersWithNotificationPersistsOutboxInSameTransaction(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, map[int64]*model.User{
		10: {ID: 10, Name: "New Member"},
		11: {ID: 11, Name: "Second Member"},
	})
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
	})

	err := domain.AddSpaceMembersWithNotification(
		context.Background(),
		9,
		[]*AddSpaceMemberRequest{
			{SpaceID: 101, UserID: 10, RoleType: 3},
			{SpaceID: 101, UserID: 11, RoleType: 3},
		},
		appendMembershipTestOutbox,
	)

	require.NoError(t, err)
	var memberCount int64
	require.NoError(t, db.Model(&model.SpaceUser{}).
		Where("space_id = ? AND user_id IN ?", 101, []int64{10, 11}).
		Count(&memberCount).Error)
	require.Equal(t, int64(2), memberCount)
	var outboxCount int64
	require.NoError(t, db.Model(&membershipOutboxRecord{}).
		Where("space_id = ? AND event_type = ?", 101, string(domainnotification.EventWorkspaceMembershipChanged)).
		Count(&outboxCount).Error)
	require.Equal(t, int64(4), outboxCount)
	for _, userID := range []int64{10, 11} {
		var userOutboxCount int64
		require.NoError(t, db.Model(&membershipOutboxRecord{}).
			Where("event_id LIKE ?", "workspace-member:added:%:101:"+strconv.FormatInt(userID, 10)+":%").
			Count(&userOutboxCount).Error)
		require.Equal(t, int64(2), userOutboxCount)
	}
}

func TestAddSpaceMembersWithNotificationRollsBackMembershipWhenOutboxFails(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, map[int64]*model.User{
		10: {ID: 10, Name: "New Member"},
	})
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
	})
	outboxErr := errors.New("outbox unavailable")

	err := domain.AddSpaceMembersWithNotification(
		context.Background(),
		9,
		[]*AddSpaceMemberRequest{{SpaceID: 101, UserID: 10, RoleType: 3}},
		func(context.Context, *gorm.DB, domainnotification.Event) error {
			return outboxErr
		},
	)

	require.ErrorIs(t, err, outboxErr)
	var count int64
	require.NoError(t, db.Model(&model.SpaceUser{}).
		Where("space_id = ? AND user_id = ?", 101, 10).
		Count(&count).Error)
	require.Zero(t, count)
}

func TestAddSpaceMembersWithNotificationRollsBackEntireBatchWhenSecondOutboxFails(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, map[int64]*model.User{
		10: {ID: 10, Name: "New Member"},
		11: {ID: 11, Name: "Second Member"},
	})
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
	})
	outboxErr := errors.New("second outbox unavailable")
	appendCalls := 0

	err := domain.AddSpaceMembersWithNotification(
		context.Background(),
		9,
		[]*AddSpaceMemberRequest{
			{SpaceID: 101, UserID: 10, RoleType: 3},
			{SpaceID: 101, UserID: 11, RoleType: 3},
		},
		func(ctx context.Context, tx *gorm.DB, event domainnotification.Event) error {
			appendCalls++
			if appendCalls == 2 {
				return outboxErr
			}
			return appendMembershipTestOutbox(ctx, tx, event)
		},
	)

	require.ErrorIs(t, err, outboxErr)
	require.Equal(t, 2, appendCalls)
	var memberCount int64
	require.NoError(t, db.Model(&model.SpaceUser{}).
		Where("space_id = ? AND user_id IN ?", 101, []int64{10, 11}).
		Count(&memberCount).Error)
	require.Zero(t, memberCount)
	var outboxCount int64
	require.NoError(t, db.Model(&membershipOutboxRecord{}).Count(&outboxCount).Error)
	require.Zero(t, outboxCount)
}

func TestPersonalSpaceMembershipUsesDurableTypeMarkerAfterRename(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, map[int64]*model.User{
		10: {ID: 10, Name: "Personal Guest"},
	})
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          102,
		Name:        "My renamed workspace",
		Description: "No longer matches defaults",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypePersonal),
	}, []*model.SpaceUser{
		{SpaceID: 102, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
	})
	appended := false

	err := domain.AddSpaceMembersWithNotification(
		context.Background(),
		9,
		[]*AddSpaceMemberRequest{{SpaceID: 102, UserID: 10, RoleType: 3}},
		func(context.Context, *gorm.DB, domainnotification.Event) error {
			appended = true
			return errors.New("personal space should not append team notification")
		},
	)

	require.NoError(t, err)
	require.False(t, appended)
	var count int64
	require.NoError(t, db.Model(&model.SpaceUser{}).
		Where("space_id = ? AND user_id = ?", 102, 10).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestUnknownHistoricalSpaceDoesNotAppendTeamMembershipNotification(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, map[int64]*model.User{
		10: {ID: 10, Name: "Historical Member"},
	})
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          104,
		Name:        "Historical Workspace",
		Description: "Type was not durably recorded",
		OwnerID:     9,
		SpaceType:   0,
	}, []*model.SpaceUser{
		{SpaceID: 104, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
	})
	appended := false

	err := domain.AddSpaceMembersWithNotification(
		context.Background(),
		9,
		[]*AddSpaceMemberRequest{{SpaceID: 104, UserID: 10, RoleType: 3}},
		func(context.Context, *gorm.DB, domainnotification.Event) error {
			appended = true
			return errors.New("unknown historical space should not append team notification")
		},
	)

	require.NoError(t, err)
	require.False(t, appended)
	var count int64
	require.NoError(t, db.Model(&model.SpaceUser{}).
		Where("space_id = ? AND user_id = ?", 104, 10).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestTeamSpaceWithLegacyPersonalNameStillAppendsMembershipNotification(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, map[int64]*model.User{
		10: {ID: 10, Name: "Team Member"},
	})
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          103,
		Name:        "Personal Space",
		Description: "This is your personal space",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 103, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
	})
	events := make([]domainnotification.Event, 0, 2)

	err := domain.AddSpaceMembersWithNotification(
		context.Background(),
		9,
		[]*AddSpaceMemberRequest{{SpaceID: 103, UserID: 10, RoleType: 3}},
		func(_ context.Context, _ *gorm.DB, event domainnotification.Event) error {
			events = append(events, event)
			return nil
		},
	)

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, domainnotification.EventWorkspaceMembershipChanged, events[0].EventType)
}

func TestUpdateSpaceMemberRoleWithNotificationUsesFreshEpochForReplay(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, nil)
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
		{SpaceID: 101, UserID: 10, RoleType: 3, CreatedAt: 1001, UpdatedAt: 1001},
	})
	events := make([]domainnotification.Event, 0, 4)
	appendOutbox := func(_ context.Context, _ *gorm.DB, event domainnotification.Event) error {
		events = append(events, event)
		return nil
	}

	require.NoError(t, domain.UpdateSpaceMemberRoleWithNotification(context.Background(), 9, 101, 10, 2, appendOutbox))
	require.NoError(t, domain.UpdateSpaceMemberRoleWithNotification(context.Background(), 9, 101, 10, 3, appendOutbox))

	require.Len(t, events, 4)
	require.Equal(t, domainnotification.EventWorkspaceRoleChanged, events[0].EventType)
	require.Equal(t, domainnotification.EventWorkspaceRoleChanged, events[2].EventType)
	require.Equal(t, events[0].AggregateID, events[2].AggregateID)
	require.Greater(t, events[2].AggregateVersion, events[0].AggregateVersion)
	require.Equal(t, []int64{10}, events[2].Payload.ExplicitRecipientIDs)
	require.Equal(t, []int64{9}, events[3].Payload.ExplicitRecipientIDs)
}

func TestUpdateSpaceMemberRoleWithNotificationRollsBackRoleWhenOutboxFails(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, nil)
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
		{SpaceID: 101, UserID: 10, RoleType: 3, CreatedAt: 1001, UpdatedAt: 1001},
	})
	outboxErr := errors.New("outbox unavailable")

	err := domain.UpdateSpaceMemberRoleWithNotification(
		context.Background(),
		9,
		101,
		10,
		2,
		func(context.Context, *gorm.DB, domainnotification.Event) error {
			return outboxErr
		},
	)

	require.ErrorIs(t, err, outboxErr)
	var member model.SpaceUser
	require.NoError(t, db.Where("space_id = ? AND user_id = ?", 101, 10).First(&member).Error)
	require.Equal(t, int32(3), member.RoleType)
}

func TestRemoveSpaceMemberWithNotificationRollsBackRemovalWhenOutboxFails(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, nil)
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
		{SpaceID: 101, UserID: 10, RoleType: 3, CreatedAt: 1001, UpdatedAt: 1001},
	})
	outboxErr := errors.New("outbox unavailable")

	err := domain.RemoveSpaceMemberWithNotification(
		context.Background(),
		9,
		101,
		10,
		func(context.Context, *gorm.DB, domainnotification.Event) error {
			return outboxErr
		},
	)

	require.ErrorIs(t, err, outboxErr)
	var count int64
	require.NoError(t, db.Model(&model.SpaceUser{}).
		Where("space_id = ? AND user_id = ?", 101, 10).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestTransferSpaceWithNotificationAppendsEventsForBothRoleChanges(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, nil)
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
		{SpaceID: 101, UserID: 10, RoleType: 2, CreatedAt: 1001, UpdatedAt: 1001},
	})
	events := make([]domainnotification.Event, 0, 2)

	err := domain.TransferSpaceWithNotification(
		context.Background(),
		9,
		101,
		10,
		func(_ context.Context, _ *gorm.DB, event domainnotification.Event) error {
			events = append(events, event)
			return nil
		},
	)

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "101:10", events[0].AggregateID)
	require.Equal(t, "101:10", events[1].AggregateID)
	require.Equal(t, domainnotification.WorkspaceMemberOwnershipTransferred, events[0].Payload.WorkspaceAction)
	require.Equal(t, domainnotification.WorkspaceMemberAudienceTarget, events[0].Payload.WorkspaceAudience)
	require.Equal(t, []int64{10}, events[0].Payload.ExplicitRecipientIDs)
	require.Equal(t, domainnotification.WorkspaceMemberAudienceAdmins, events[1].Payload.WorkspaceAudience)
	require.Equal(t, []int64{9}, events[1].Payload.ExplicitRecipientIDs)
	var space model.Space
	require.NoError(t, db.First(&space, 101).Error)
	require.Equal(t, int64(10), space.OwnerID)
}

func TestTransferSpaceWithNotificationRollsBackOwnerRolesAndOutboxWhenAppendFails(t *testing.T) {
	t.Parallel()

	db, domain := newMembershipNotificationDomain(t, nil)
	seedMembershipNotificationSpace(t, db, &model.Space{
		ID:          101,
		Name:        "Team Alpha",
		Description: "Team workspace",
		OwnerID:     9,
		SpaceType:   int32(userEntity.SpaceTypeTeam),
	}, []*model.SpaceUser{
		{SpaceID: 101, UserID: 9, RoleType: 1, CreatedAt: 1000, UpdatedAt: 1000},
		{SpaceID: 101, UserID: 10, RoleType: 2, CreatedAt: 1001, UpdatedAt: 1001},
	})
	outboxErr := errors.New("outbox unavailable")

	err := domain.TransferSpaceWithNotification(
		context.Background(),
		9,
		101,
		10,
		func(ctx context.Context, tx *gorm.DB, event domainnotification.Event) error {
			if err := appendMembershipTestOutbox(ctx, tx, event); err != nil {
				return err
			}
			return outboxErr
		},
	)

	require.ErrorIs(t, err, outboxErr)
	var space model.Space
	require.NoError(t, db.First(&space, 101).Error)
	require.Equal(t, int64(9), space.OwnerID)
	var oldOwner model.SpaceUser
	require.NoError(t, db.Where("space_id = ? AND user_id = ?", 101, 9).First(&oldOwner).Error)
	require.Equal(t, int32(1), oldOwner.RoleType)
	var target model.SpaceUser
	require.NoError(t, db.Where("space_id = ? AND user_id = ?", 101, 10).First(&target).Error)
	require.Equal(t, int32(2), target.RoleType)
	var outboxCount int64
	require.NoError(t, db.Model(&membershipOutboxRecord{}).Count(&outboxCount).Error)
	require.Zero(t, outboxCount)
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

type membershipNotificationDomain interface {
	AddSpaceMembersWithNotification(ctx context.Context, actorID int64, members []*AddSpaceMemberRequest, appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error) error
	UpdateSpaceMemberRoleWithNotification(ctx context.Context, actorID int64, spaceID int64, userID int64, roleType int32, appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error) error
	RemoveSpaceMemberWithNotification(ctx context.Context, actorID int64, spaceID int64, userID int64, appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error) error
	TransferSpaceWithNotification(ctx context.Context, actorID int64, spaceID int64, targetUserID int64, appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error) error
}

func newMembershipNotificationDomain(t *testing.T, users map[int64]*model.User) (*gorm.DB, membershipNotificationDomain) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Space{}, &model.SpaceUser{}, &membershipOutboxRecord{}))

	domain := NewUserDomain(context.Background(), &Components{
		IconOSS: fakeSpaceStorage{},
		UserRepo: &spaceListUserRepo{
			usersByID: users,
		},
		SpaceRepo: dal.NewSpaceDAO(db),
	})
	notificationDomain, ok := domain.(membershipNotificationDomain)
	require.True(t, ok)
	return db, notificationDomain
}

func seedMembershipNotificationSpace(t *testing.T, db *gorm.DB, space *model.Space, members []*model.SpaceUser) {
	t.Helper()

	if space.IconURI == "" {
		space.IconURI = "space/team.png"
	}
	if space.CreatorID == 0 {
		space.CreatorID = space.OwnerID
	}
	if space.CreatedAt == 0 {
		space.CreatedAt = 1000
	}
	if space.UpdatedAt == 0 {
		space.UpdatedAt = space.CreatedAt
	}
	require.NoError(t, db.Create(space).Error)
	for _, member := range members {
		require.NoError(t, db.Create(member).Error)
	}
}

type membershipOutboxRecord struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement:true"`
	EventID   string `gorm:"column:event_id;size:128;uniqueIndex"`
	EventType string `gorm:"column:event_type;size:64"`
	SpaceID   int64  `gorm:"column:space_id"`
}

func (membershipOutboxRecord) TableName() string {
	return "membership_test_outbox"
}

func appendMembershipTestOutbox(ctx context.Context, tx *gorm.DB, event domainnotification.Event) error {
	return tx.WithContext(ctx).Create(&membershipOutboxRecord{
		EventID:   event.EventID,
		EventType: string(event.EventType),
		SpaceID:   event.SpaceID,
	}).Error
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
func (r *spaceListSpaceRepo) AddSpaceUserWithNotification(ctx context.Context, _ int64, spaceUser *model.SpaceUser, _ func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	return r.AddSpaceUser(ctx, spaceUser)
}
func (r *spaceListSpaceRepo) AddSpaceUsersWithNotification(ctx context.Context, _ int64, spaceUsers []*model.SpaceUser, _ func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	for _, spaceUser := range spaceUsers {
		if err := r.AddSpaceUser(ctx, spaceUser); err != nil {
			return err
		}
	}
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
func (r *spaceListSpaceRepo) UpdateSpaceUserRoleWithNotification(ctx context.Context, _ int64, spaceID int64, userID int64, roleType int32, _ func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	return r.UpdateSpaceUserRole(ctx, spaceID, userID, roleType)
}
func (r *spaceListSpaceRepo) RemoveSpaceUser(_ context.Context, spaceID int64, userID int64) error {
	r.removedSpaceID = spaceID
	r.removedUserID = userID
	return nil
}
func (r *spaceListSpaceRepo) RemoveSpaceUserWithNotification(ctx context.Context, _ int64, spaceID int64, userID int64, _ func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	return r.RemoveSpaceUser(ctx, spaceID, userID)
}
func (r *spaceListSpaceRepo) TransferSpaceWithNotification(_ context.Context, _ int64, spaceID int64, targetUserID int64, _ func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	r.updatedSpaceID = spaceID
	r.updatedSpace = map[string]any{"owner_id": targetUserID}
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
