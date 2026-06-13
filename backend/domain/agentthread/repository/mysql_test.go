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

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestThreadRepositoryCreateAndGet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&threadPO{}))

	repo := NewThreadRepository(db)
	thread := &entity.Thread{
		ID:            1,
		SpaceID:       10,
		CreatorID:     20,
		AgentID:       30,
		Title:         "生成周报",
		Status:        entity.ThreadStatusIdle,
		Source:        entity.ThreadSourceWeb,
		LegacyTaskID:  40,
		Metadata:      `{"mode":"auto"}`,
		CreatedAt:     1,
		UpdatedAt:     2,
		LastMessageAt: 3,
	}

	require.NoError(t, repo.CreateThread(context.Background(), thread))
	got, err := repo.GetThread(context.Background(), 1)

	require.NoError(t, err)
	require.Equal(t, int64(10), got.SpaceID)
	require.Equal(t, int64(20), got.CreatorID)
	require.Equal(t, int64(30), got.AgentID)
	require.Equal(t, "生成周报", got.Title)
	require.Equal(t, entity.ThreadStatusIdle, got.Status)
	require.Equal(t, entity.ThreadSourceWeb, got.Source)
	require.Equal(t, int64(40), got.LegacyTaskID)
	require.Equal(t, `{"mode":"auto"}`, got.Metadata)
	require.Equal(t, int64(1), got.CreatedAt)
	require.Equal(t, int64(2), got.UpdatedAt)
	require.Equal(t, int64(3), got.LastMessageAt)
}

func TestThreadRepositoryListFiltersAndOrders(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&threadPO{}))

	repo := NewThreadRepository(db)
	status := entity.ThreadStatusRunning
	for _, thread := range []*entity.Thread{
		{ID: 1, SpaceID: 10, CreatorID: 20, Title: "old", Status: status, Source: entity.ThreadSourceWeb, UpdatedAt: 1},
		{ID: 2, SpaceID: 10, CreatorID: 20, Title: "new", Status: status, Source: entity.ThreadSourceWeb, UpdatedAt: 2},
		{ID: 3, SpaceID: 11, CreatorID: 20, Title: "other space", Status: status, Source: entity.ThreadSourceWeb, UpdatedAt: 3},
		{ID: 4, SpaceID: 10, CreatorID: 21, Title: "other user", Status: status, Source: entity.ThreadSourceWeb, UpdatedAt: 4},
		{ID: 5, SpaceID: 10, CreatorID: 20, Title: "idle", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, UpdatedAt: 5},
	} {
		require.NoError(t, repo.CreateThread(context.Background(), thread))
	}

	got, total, err := repo.ListThreads(context.Background(), ListThreadsRequest{
		SpaceID:  10,
		UserID:   20,
		Status:   &status,
		Page:     1,
		PageSize: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, got, 2)
	require.Equal(t, int64(2), got[0].ID)
	require.Equal(t, int64(1), got[1].ID)
}

func TestThreadRepositoryUsesStablePaginationOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&threadPO{}))

	repo := NewThreadRepository(db)
	for _, thread := range []*entity.Thread{
		{ID: 1, SpaceID: 10, CreatorID: 20, Title: "first", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, UpdatedAt: 1},
		{ID: 2, SpaceID: 10, CreatorID: 20, Title: "second", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, UpdatedAt: 1},
		{ID: 3, SpaceID: 10, CreatorID: 20, Title: "third", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, UpdatedAt: 2},
	} {
		require.NoError(t, repo.CreateThread(context.Background(), thread))
	}

	got, total, err := repo.ListThreads(context.Background(), ListThreadsRequest{
		SpaceID:  10,
		Page:     1,
		PageSize: 2,
	})

	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, got, 2)
	require.Equal(t, int64(3), got[0].ID)
	require.Equal(t, int64(2), got[1].ID)
}

func TestThreadRepositoryRejectsInvalidMetadataJSON(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&threadPO{}))

	repo := NewThreadRepository(db)
	err = repo.CreateThread(context.Background(), &entity.Thread{
		ID:        1,
		SpaceID:   10,
		CreatorID: 20,
		Title:     "bad",
		Status:    entity.ThreadStatusIdle,
		Source:    entity.ThreadSourceWeb,
		Metadata:  "{",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "metadata")
}
