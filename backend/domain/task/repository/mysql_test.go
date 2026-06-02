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

	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
)

func TestTaskRepositoryCreateAndGet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&taskPO{}, &taskAttemptPO{}, &taskEventPO{}))

	repo := NewTaskRepository(db, fixedIDGen{})
	task := &entity.Task{ID: 1, SpaceID: 10, CreatorID: 20, Title: "report", Status: entity.StatusCreated}

	require.NoError(t, repo.Create(context.Background(), task))
	got, err := repo.Get(context.Background(), 1)

	require.NoError(t, err)
	require.Equal(t, entity.StatusCreated, got.Status)
	require.Equal(t, "report", got.Title)
}

func TestTaskRepositoryListUpdateStatusAndEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&taskPO{}, &taskAttemptPO{}, &taskEventPO{}))

	repo := NewTaskRepository(db, fixedIDGen{})
	require.NoError(t, repo.Create(context.Background(), &entity.Task{
		ID:        1,
		SpaceID:   10,
		CreatorID: 20,
		Title:     "first",
		Status:    entity.StatusCreated,
		UpdatedAt: 1,
	}))
	require.NoError(t, repo.Create(context.Background(), &entity.Task{
		ID:        2,
		SpaceID:   10,
		CreatorID: 20,
		Title:     "second",
		Status:    entity.StatusQueued,
		UpdatedAt: 2,
	}))

	got, total, err := repo.List(context.Background(), 10, nil, 1, 1)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].ID)

	require.NoError(t, repo.UpdateStatus(context.Background(), 2, entity.StatusQueued, entity.StatusRunning, 50, `{"ok":true}`, ""))
	task, err := repo.Get(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, entity.StatusRunning, task.Status)
	require.Equal(t, int32(50), task.Progress)
	require.Equal(t, `{"ok":true}`, task.Result)

	require.Error(t, repo.UpdateStatus(context.Background(), 2, entity.StatusQueued, entity.StatusSucceeded, 100, `{"done":true}`, ""))

	require.NoError(t, repo.CreateEvent(context.Background(), &entity.Event{
		ID:        10,
		TaskID:    2,
		EventType: "progress",
		Payload:   `{"progress":50}`,
		CreatedAt: 3,
	}))
	events, err := repo.ListEvents(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "progress", events[0].EventType)
	require.Equal(t, `{"progress":50}`, events[0].Payload)
}

func TestTaskRepositoryRejectsInvalidJSON(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&taskPO{}, &taskAttemptPO{}, &taskEventPO{}))

	repo := NewTaskRepository(db, fixedIDGen{})
	err = repo.Create(context.Background(), &entity.Task{
		ID:        1,
		SpaceID:   10,
		CreatorID: 20,
		Title:     "bad json",
		Status:    entity.StatusCreated,
		Input:     "{",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "input")
}

func TestTaskRepositoryUsesStablePaginationOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&taskPO{}, &taskAttemptPO{}, &taskEventPO{}))

	repo := NewTaskRepository(db, fixedIDGen{})
	for _, task := range []*entity.Task{
		{ID: 1, SpaceID: 10, CreatorID: 20, Title: "first", Status: entity.StatusCreated, UpdatedAt: 1},
		{ID: 2, SpaceID: 10, CreatorID: 20, Title: "second", Status: entity.StatusCreated, UpdatedAt: 1},
	} {
		require.NoError(t, repo.Create(context.Background(), task))
	}

	got, total, err := repo.List(context.Background(), 10, nil, 1, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, got, 2)
	require.Equal(t, int64(2), got[0].ID)
	require.Equal(t, int64(1), got[1].ID)
}

func TestTaskRepositoryListQueuedOrdersOldestFirst(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&taskPO{}, &taskAttemptPO{}, &taskEventPO{}))

	repo := NewTaskRepository(db, fixedIDGen{})
	for _, task := range []*entity.Task{
		{ID: 1, SpaceID: 10, CreatorID: 20, Title: "created", Status: entity.StatusCreated, UpdatedAt: 1},
		{ID: 2, SpaceID: 10, CreatorID: 20, Title: "new queued", Status: entity.StatusQueued, UpdatedAt: 3},
		{ID: 3, SpaceID: 10, CreatorID: 20, Title: "old queued", Status: entity.StatusQueued, UpdatedAt: 2},
	} {
		require.NoError(t, repo.Create(context.Background(), task))
	}

	got, err := repo.ListQueued(context.Background(), 1)

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(3), got[0].ID)
}

type fixedIDGen struct{}

func (fixedIDGen) GenID(ctx context.Context) (int64, error) { return 1, nil }

func (fixedIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	return ids, nil
}
