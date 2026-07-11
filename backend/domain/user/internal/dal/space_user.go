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
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/model"
)

func (dao *SpaceDAO) AddSpaceUser(ctx context.Context, spaceUser *model.SpaceUser) error {
	return dao.query.SpaceUser.WithContext(ctx).Create(spaceUser)
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

func (dao *SpaceDAO) RemoveSpaceUser(ctx context.Context, spaceID int64, userID int64) error {
	_, err := dao.query.SpaceUser.WithContext(ctx).Where(
		dao.query.SpaceUser.SpaceID.Eq(spaceID),
		dao.query.SpaceUser.UserID.Eq(userID),
	).Delete()
	return err
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
