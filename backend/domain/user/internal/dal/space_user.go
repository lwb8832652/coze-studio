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

package dal

import (
	"context"
	"fmt"
	"strconv"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/query"
	"github.com/coze-dev/coze-studio/backend/pkg/safetext"

	"gorm.io/gorm"
)

func (dao *SpaceDAO) AddSpaceUser(ctx context.Context, spaceUser *model.SpaceUser) error {
	return dao.query.SpaceUser.WithContext(ctx).Create(spaceUser)
}

func (dao *SpaceDAO) AddSpaceUserWithNotification(
	ctx context.Context,
	actorID int64,
	spaceUser *model.SpaceUser,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	if spaceUser == nil {
		return gorm.ErrInvalidData
	}
	return dao.AddSpaceUsersWithNotification(ctx, actorID, []*model.SpaceUser{spaceUser}, appendOutbox)
}

func (dao *SpaceDAO) AddSpaceUsersWithNotification(
	ctx context.Context,
	actorID int64,
	spaceUsers []*model.SpaceUser,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	if len(spaceUsers) == 0 {
		return nil
	}
	return dao.query.Transaction(func(tx *query.Query) error {
		spaceID := int64(0)
		for _, spaceUser := range spaceUsers {
			if spaceUser == nil {
				return gorm.ErrInvalidData
			}
			if spaceID == 0 {
				spaceID = spaceUser.SpaceID
			}
			if spaceUser.SpaceID != spaceID {
				return gorm.ErrInvalidData
			}
		}

		space, err := tx.Space.WithContext(ctx).Where(
			tx.Space.ID.Eq(spaceID),
		).First()
		if err != nil {
			return err
		}

		now := time.Now().UnixMilli()
		for _, spaceUser := range spaceUsers {
			if spaceUser.CreatedAt <= 0 {
				spaceUser.CreatedAt = now
			}
			if spaceUser.UpdatedAt <= 0 {
				spaceUser.UpdatedAt = spaceUser.CreatedAt
			}
		}
		if err := tx.SpaceUser.WithContext(ctx).Create(spaceUsers...); err != nil {
			return err
		}

		if !isTeamSpaceForMembershipNotification(space) {
			return nil
		}
		members, err := tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
		).Find()
		if err != nil {
			return err
		}
		for _, spaceUser := range spaceUsers {
			version := spaceUser.ID
			if version <= 0 {
				version = spaceUser.UpdatedAt
			}
			if err := appendWorkspaceMembershipEvents(
				ctx,
				tx,
				appendOutbox,
				domainnotification.EventWorkspaceMembershipChanged,
				domainnotification.WorkspaceMemberAdded,
				"workspace_member.added",
				space,
				spaceUser.UserID,
				actorID,
				version,
				spaceUser.UpdatedAt,
				members,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (dao *SpaceDAO) UpdateSpaceUserRole(ctx context.Context, spaceID int64, userID int64, roleType int32) error {
	_, err := dao.query.SpaceUser.WithContext(ctx).Where(
		dao.query.SpaceUser.SpaceID.Eq(spaceID),
		dao.query.SpaceUser.UserID.Eq(userID),
	).Updates(map[string]interface{}{
		"role_type":  roleType,
		"updated_at": time.Now().UnixMilli(),
	})
	return err
}

func (dao *SpaceDAO) UpdateSpaceUserRoleWithNotification(
	ctx context.Context,
	actorID int64,
	spaceID int64,
	userID int64,
	roleType int32,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	return dao.query.Transaction(func(tx *query.Query) error {
		space, err := tx.Space.WithContext(ctx).Where(
			tx.Space.ID.Eq(spaceID),
		).First()
		if err != nil {
			return err
		}
		member, err := tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
			tx.SpaceUser.UserID.Eq(userID),
		).First()
		if err != nil {
			return err
		}
		if member.RoleType == roleType {
			return nil
		}

		updatedAt := nextMembershipEventEpoch(member.UpdatedAt)
		_, err = tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
			tx.SpaceUser.UserID.Eq(userID),
		).Updates(map[string]interface{}{
			"role_type":  roleType,
			"updated_at": updatedAt,
		})
		if err != nil {
			return err
		}

		if !isTeamSpaceForMembershipNotification(space) {
			return nil
		}
		members, err := tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
		).Find()
		if err != nil {
			return err
		}
		return appendWorkspaceMembershipEvents(
			ctx,
			tx,
			appendOutbox,
			domainnotification.EventWorkspaceRoleChanged,
			domainnotification.WorkspaceMemberRoleChanged,
			"workspace_member.role",
			space,
			userID,
			actorID,
			updatedAt,
			updatedAt,
			members,
		)
	})
}

func (dao *SpaceDAO) RemoveSpaceUser(ctx context.Context, spaceID int64, userID int64) error {
	_, err := dao.query.SpaceUser.WithContext(ctx).Where(
		dao.query.SpaceUser.SpaceID.Eq(spaceID),
		dao.query.SpaceUser.UserID.Eq(userID),
	).Delete()
	return err
}

func (dao *SpaceDAO) RemoveSpaceUserWithNotification(
	ctx context.Context,
	actorID int64,
	spaceID int64,
	userID int64,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	return dao.query.Transaction(func(tx *query.Query) error {
		space, err := tx.Space.WithContext(ctx).Where(
			tx.Space.ID.Eq(spaceID),
		).First()
		if err != nil {
			return err
		}
		member, err := tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
			tx.SpaceUser.UserID.Eq(userID),
		).First()
		if err != nil {
			return err
		}

		removedAt := nextMembershipEventEpoch(member.UpdatedAt)
		_, err = tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
			tx.SpaceUser.UserID.Eq(userID),
		).Delete()
		if err != nil {
			return err
		}

		if !isTeamSpaceForMembershipNotification(space) {
			return nil
		}
		members, err := tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
		).Find()
		if err != nil {
			return err
		}
		return appendWorkspaceMembershipEvents(
			ctx,
			tx,
			appendOutbox,
			domainnotification.EventWorkspaceMembershipChanged,
			domainnotification.WorkspaceMemberRemoved,
			"workspace_member.removed",
			space,
			userID,
			actorID,
			removedAt,
			removedAt,
			members,
		)
	})
}

func (dao *SpaceDAO) TransferSpaceWithNotification(
	ctx context.Context,
	actorID int64,
	spaceID int64,
	targetUserID int64,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
) error {
	return dao.query.Transaction(func(tx *query.Query) error {
		space, err := tx.Space.WithContext(ctx).Where(
			tx.Space.ID.Eq(spaceID),
		).First()
		if err != nil {
			return err
		}
		if space.OwnerID == targetUserID {
			return nil
		}

		members, err := tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
		).Find()
		if err != nil {
			return err
		}
		memberByUserID := make(map[int64]*model.SpaceUser, len(members))
		for _, member := range members {
			memberByUserID[member.UserID] = member
		}
		targetMember := memberByUserID[targetUserID]
		if targetMember == nil {
			return gorm.ErrRecordNotFound
		}

		now := time.Now().UnixMilli()
		oldOwnerID := space.OwnerID
		oldOwnerMember := memberByUserID[oldOwnerID]
		oldOwnerUpdatedAt := int64(0)
		if oldOwnerID > 0 && oldOwnerID != targetUserID && oldOwnerMember != nil {
			oldOwnerUpdatedAt = nextMembershipEventEpochAt(now, oldOwnerMember.UpdatedAt)
			_, err = tx.SpaceUser.WithContext(ctx).Where(
				tx.SpaceUser.SpaceID.Eq(spaceID),
				tx.SpaceUser.UserID.Eq(oldOwnerID),
			).Updates(map[string]interface{}{
				"role_type":  int32(2),
				"updated_at": oldOwnerUpdatedAt,
			})
			if err != nil {
				return err
			}
		}

		targetUpdatedAt := nextMembershipEventEpochAt(now, targetMember.UpdatedAt)
		_, err = tx.Space.WithContext(ctx).Where(
			tx.Space.ID.Eq(spaceID),
		).Updates(map[string]interface{}{
			"owner_id":   targetUserID,
			"updated_at": targetUpdatedAt,
		})
		if err != nil {
			return err
		}
		_, err = tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
			tx.SpaceUser.UserID.Eq(targetUserID),
		).Updates(map[string]interface{}{
			"role_type":  int32(1),
			"updated_at": targetUpdatedAt,
		})
		if err != nil {
			return err
		}

		if !isTeamSpaceForMembershipNotification(space) {
			return nil
		}
		space.OwnerID = targetUserID
		members, err = tx.SpaceUser.WithContext(ctx).Where(
			tx.SpaceUser.SpaceID.Eq(spaceID),
		).Find()
		if err != nil {
			return err
		}

		return appendWorkspaceMembershipEvents(
			ctx,
			tx,
			appendOutbox,
			domainnotification.EventWorkspaceRoleChanged,
			domainnotification.WorkspaceMemberOwnershipTransferred,
			"workspace_member.ownership_transferred",
			space,
			targetUserID,
			actorID,
			targetUpdatedAt,
			targetUpdatedAt,
			members,
		)
	})
}

func (dao *SpaceDAO) HasSpaceUser(ctx context.Context, spaceID int64, userID int64) (bool, error) {
	count, err := dao.query.SpaceUser.WithContext(ctx).Where(
		dao.query.SpaceUser.SpaceID.Eq(spaceID),
		dao.query.SpaceUser.UserID.Eq(userID),
	).Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (dao *SpaceDAO) GetSpaceList(ctx context.Context, userID int64) ([]*model.SpaceUser, error) {
	return dao.query.SpaceUser.WithContext(ctx).Where(
		dao.query.SpaceUser.UserID.Eq(userID),
	).Find()
}

func (dao *SpaceDAO) GetSpaceUsersBySpaceID(ctx context.Context, spaceID int64) ([]*model.SpaceUser, error) {
	return dao.query.SpaceUser.WithContext(ctx).Where(
		dao.query.SpaceUser.SpaceID.Eq(spaceID),
	).Find()
}

func (dao *SpaceDAO) CountSpaceUsers(ctx context.Context, spaceIDs []int64) (map[int64]int64, error) {
	result := make(map[int64]int64, len(spaceIDs))
	if len(spaceIDs) == 0 {
		return result, nil
	}

	spaceUsers, err := dao.query.SpaceUser.WithContext(ctx).Where(
		dao.query.SpaceUser.SpaceID.In(spaceIDs...),
	).Find()
	if err != nil {
		return nil, err
	}

	for _, spaceUser := range spaceUsers {
		result[spaceUser.SpaceID]++
	}

	return result, nil
}

func appendWorkspaceMembershipOutbox(
	ctx context.Context,
	tx *query.Query,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
	event domainnotification.Event,
) error {
	if appendOutbox == nil {
		return domainnotification.ErrStorage
	}
	db := tx.SpaceUser.WithContext(ctx).UnderlyingDB().Session(&gorm.Session{NewDB: true})
	return appendOutbox(ctx, db, event)
}

func workspaceMembershipNotificationEvent(
	eventType domainnotification.EventType,
	action domainnotification.WorkspaceMemberAction,
	audience domainnotification.WorkspaceMemberAudience,
	aggregateType string,
	space *model.Space,
	affectedUserID int64,
	actorID int64,
	aggregateVersion int64,
	occurredAtMilli int64,
	recipients []int64,
) domainnotification.Event {
	if actorID < 0 {
		actorID = 0
	}
	return domainnotification.Event{
		EventID:          fmt.Sprintf("workspace-member:%s:%s:%d:%d:%d", action, audience, space.ID, affectedUserID, aggregateVersion),
		EventType:        eventType,
		AggregateType:    fmt.Sprintf("%s.%s", aggregateType, audience),
		AggregateID:      fmt.Sprintf("%d:%d", space.ID, affectedUserID),
		AggregateVersion: aggregateVersion,
		OccurredAt:       time.UnixMilli(occurredAtMilli),
		ActorID:          actorID,
		SpaceID:          space.ID,
		RecipientPolicy:  domainnotification.RecipientExplicitInternalUsers,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName:  safeWorkspaceDisplayName(space),
			WorkspaceAction:      action,
			WorkspaceAudience:    audience,
			SubjectDisplayName:   "成员 " + strconv.FormatInt(affectedUserID, 10),
			ExplicitRecipientIDs: recipients,
		},
	}
}

func appendWorkspaceMembershipEvents(
	ctx context.Context,
	tx *query.Query,
	appendOutbox func(context.Context, *gorm.DB, domainnotification.Event) error,
	eventType domainnotification.EventType,
	action domainnotification.WorkspaceMemberAction,
	aggregateType string,
	space *model.Space,
	affectedUserID int64,
	actorID int64,
	aggregateVersion int64,
	occurredAtMilli int64,
	members []*model.SpaceUser,
) error {
	targetEvent := workspaceMembershipNotificationEvent(
		eventType,
		action,
		domainnotification.WorkspaceMemberAudienceTarget,
		aggregateType,
		space,
		affectedUserID,
		actorID,
		aggregateVersion,
		occurredAtMilli,
		[]int64{affectedUserID},
	)
	if err := appendWorkspaceMembershipOutbox(
		ctx,
		tx,
		appendOutbox,
		targetEvent,
	); err != nil {
		return err
	}
	adminRecipients, err := workspaceMembershipAdminRecipients(
		space,
		members,
		affectedUserID,
	)
	if err != nil {
		return err
	}
	if len(adminRecipients) == 0 {
		return nil
	}
	adminEvent := workspaceMembershipNotificationEvent(
		eventType,
		action,
		domainnotification.WorkspaceMemberAudienceAdmins,
		aggregateType,
		space,
		affectedUserID,
		actorID,
		aggregateVersion,
		occurredAtMilli,
		adminRecipients,
	)
	return appendWorkspaceMembershipOutbox(ctx, tx, appendOutbox, adminEvent)
}

func workspaceMembershipAdminRecipients(
	space *model.Space,
	members []*model.SpaceUser,
	affectedUserID int64,
) ([]int64, error) {
	ids := make([]int64, 0, len(members)+1)
	if space != nil &&
		space.OwnerID > 0 &&
		space.OwnerID != affectedUserID {
		ids = append(ids, space.OwnerID)
	}
	for _, member := range members {
		if member == nil {
			continue
		}
		if member.UserID != affectedUserID &&
			(member.RoleType == 1 || member.RoleType == 2) {
			ids = append(ids, member.UserID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return domainnotification.NormalizeRecipientIDs(ids)
}

func safeWorkspaceDisplayName(space *model.Space) string {
	if space == nil {
		return "工作空间"
	}
	value, err := safetext.Normalize(space.Name, safetext.Rules{
		MaxRunes: domainnotification.MaxDisplayNameRunes,
	})
	if err != nil || value == "" {
		return "工作空间"
	}
	return value
}

func nextMembershipEventEpoch(previous int64) int64 {
	return nextMembershipEventEpochAt(time.Now().UnixMilli(), previous)
}

func nextMembershipEventEpochAt(now int64, previous int64) int64 {
	if now <= previous {
		return previous + 1
	}
	return now
}

func isTeamSpaceForMembershipNotification(space *model.Space) bool {
	if space == nil {
		return false
	}
	return space.SpaceType == 2
}
