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

func TestThreadRepositoryCreateAndListMessages(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&threadPO{}, &messagePO{}))

	repo := NewThreadRepository(db)
	for _, message := range []*entity.Message{
		{
			ID:        2,
			ThreadID:  10,
			RunID:     20,
			Role:      entity.MessageRoleAssistant,
			Content:   "第二条",
			Metadata:  `{"source":"agent"}`,
			CreatedAt: 200,
		},
		{
			ID:        1,
			ThreadID:  10,
			RunID:     20,
			Role:      entity.MessageRoleUser,
			Content:   "第一条",
			Metadata:  `{"source":"user"}`,
			CreatedAt: 100,
		},
		{
			ID:        3,
			ThreadID:  11,
			Role:      entity.MessageRoleUser,
			Content:   "其他线程",
			CreatedAt: 50,
		},
	} {
		require.NoError(t, repo.CreateMessage(context.Background(), message))
	}

	got, total, err := repo.ListMessages(context.Background(), ListMessagesRequest{
		ThreadID: 10,
		Page:     1,
		PageSize: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, entity.MessageRoleUser, got[0].Role)
	require.Equal(t, "第一条", got[0].Content)
	require.Equal(t, `{"source":"user"}`, got[0].Metadata)
	require.Equal(t, int64(2), got[1].ID)
	require.Equal(t, entity.MessageRoleAssistant, got[1].Role)
	require.Equal(t, "第二条", got[1].Content)
	require.Equal(t, int64(20), got[1].RunID)
}

func TestThreadRepositoryCreateAndGetRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	run := &entity.Run{
		ID:                100,
		ThreadID:          10,
		SpaceID:           1,
		CreatorID:         2,
		AssistantID:       "default",
		Status:            entity.RunStatusPending,
		Command:           `{"resume":false}`,
		Input:             `{"messages":[{"role":"user","content":"hello"}]}`,
		Config:            `{"mode":"Auto"}`,
		Context:           `{"source":"web"}`,
		Metadata:          `{"trace":"abc"}`,
		StreamMode:        `["messages","updates"]`,
		MultitaskStrategy: "enqueue",
		OnDisconnect:      "continue",
		Durability:        "async",
		IdempotencyKey:    "idem-1",
		WorkerID:          "worker-1",
		ErrorCode:         "tool_failed",
		ErrorMessage:      "tool error",
		StartedAt:         11,
		EndedAt:           22,
		CreatedAt:         33,
		UpdatedAt:         44,
	}

	require.NoError(t, repo.CreateRun(context.Background(), run))
	got, err := repo.GetRun(context.Background(), 100)

	require.NoError(t, err)
	require.Equal(t, int64(10), got.ThreadID)
	require.Equal(t, int64(1), got.SpaceID)
	require.Equal(t, int64(2), got.CreatorID)
	require.Equal(t, "default", got.AssistantID)
	require.Equal(t, entity.RunStatusPending, got.Status)
	require.Equal(t, `{"resume":false}`, got.Command)
	require.Equal(t, `{"messages":[{"role":"user","content":"hello"}]}`, got.Input)
	require.Equal(t, `{"mode":"Auto"}`, got.Config)
	require.Equal(t, `{"source":"web"}`, got.Context)
	require.Equal(t, `{"trace":"abc"}`, got.Metadata)
	require.Equal(t, `["messages","updates"]`, got.StreamMode)
	require.Equal(t, "enqueue", got.MultitaskStrategy)
	require.Equal(t, "continue", got.OnDisconnect)
	require.Equal(t, "async", got.Durability)
	require.Equal(t, "idem-1", got.IdempotencyKey)
	require.Equal(t, "worker-1", got.WorkerID)
	require.Equal(t, "tool_failed", got.ErrorCode)
	require.Equal(t, "tool error", got.ErrorMessage)
	require.Equal(t, int64(11), got.StartedAt)
	require.Equal(t, int64(22), got.EndedAt)
	require.Equal(t, int64(33), got.CreatedAt)
	require.Equal(t, int64(44), got.UpdatedAt)
}

func TestThreadRepositoryListRunsFiltersByThreadAndOrdersNewestFirst(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	status := entity.RunStatusPending
	for _, run := range []*entity.Run{
		newRepositoryTestRun(1, 10, status, 1),
		newRepositoryTestRun(2, 10, status, 2),
		newRepositoryTestRun(3, 11, status, 3),
		newRepositoryTestRun(4, 10, entity.RunStatusRunning, 4),
	} {
		require.NoError(t, repo.CreateRun(context.Background(), run))
	}

	got, total, err := repo.ListRuns(context.Background(), ListRunsRequest{
		ThreadID: 10,
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

func newRepositoryTestRun(id, threadID int64, status entity.RunStatus, createdAt int64) *entity.Run {
	return &entity.Run{
		ID:                id,
		ThreadID:          threadID,
		SpaceID:           1,
		CreatorID:         2,
		AssistantID:       "default",
		Status:            status,
		Command:           `{}`,
		Input:             `{}`,
		Config:            `{}`,
		Context:           `{}`,
		Metadata:          `{}`,
		StreamMode:        `["messages","updates"]`,
		MultitaskStrategy: "enqueue",
		OnDisconnect:      "continue",
		Durability:        "async",
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}
}
