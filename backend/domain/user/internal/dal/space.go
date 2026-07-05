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
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/model"
	"github.com/coze-dev/coze-studio/backend/domain/user/internal/dal/query"
)

func NewSpaceDAO(db *gorm.DB) *SpaceDAO {
	return &SpaceDAO{
		query: query.Use(db),
	}
}

type SpaceDAO struct {
	query *query.Query
}

func (dao *SpaceDAO) CreateSpace(ctx context.Context, space *model.Space) error {
	return dao.query.Space.WithContext(ctx).Create(space)
}

func (dao *SpaceDAO) UpdateSpace(ctx context.Context, spaceID int64, updates map[string]any) error {
	if _, ok := updates["updated_at"]; !ok {
		updates["updated_at"] = time.Now().UnixMilli()
	}

	_, err := dao.query.Space.WithContext(ctx).Where(
		dao.query.Space.ID.Eq(spaceID),
	).Updates(updates)
	return err
}

func (dao *SpaceDAO) UpdateSpaceOwner(ctx context.Context, spaceID int64, ownerID int64) error {
	_, err := dao.query.Space.WithContext(ctx).Where(
		dao.query.Space.ID.Eq(spaceID),
	).Updates(map[string]interface{}{
		"owner_id":   ownerID,
		"updated_at": time.Now().UnixMilli(),
	})
	return err
}

func (dao *SpaceDAO) DeleteSpace(ctx context.Context, spaceID int64) error {
	_, err := dao.query.Space.WithContext(ctx).Where(
		dao.query.Space.ID.Eq(spaceID),
	).Delete()
	return err
}

func (dao *SpaceDAO) GetSpaceByIDs(ctx context.Context, spaceIDs []int64) ([]*model.Space, error) {
	return dao.query.Space.WithContext(ctx).Where(
		dao.query.Space.ID.In(spaceIDs...),
	).Find()
}

func (dao *SpaceDAO) ListSpaces(ctx context.Context, keyword string, offset int, limit int) ([]*model.Space, int64, error) {
	q := dao.query.Space.WithContext(ctx)
	if normalized := strings.TrimSpace(keyword); normalized != "" {
		pattern := "%" + normalized + "%"
		q = q.Where(dao.query.Space.Name.Like(pattern)).
			Or(dao.query.Space.Description.Like(pattern))
	}

	return q.Order(dao.query.Space.CreatedAt.Desc()).FindByPage(offset, limit)
}
