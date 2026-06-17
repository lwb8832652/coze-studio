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

func TestThreadRepositoryCreateAndListRunEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runEventPO{}))

	repo := NewThreadRepository(db)
	for _, event := range []*entity.RunEvent{
		{ID: 2, ThreadID: 10, RunID: 20, EventType: "step.completed", Payload: `{"step":2}`, CreatedAt: 200},
		{ID: 1, ThreadID: 10, RunID: 20, EventType: "run.started", Payload: `{"step":1}`, CreatedAt: 100},
		{ID: 3, ThreadID: 10, RunID: 21, EventType: "other.run", Payload: `{}`, CreatedAt: 50},
		{ID: 4, ThreadID: 11, RunID: 22, EventType: "other.thread", Payload: `{}`, CreatedAt: 60},
	} {
		require.NoError(t, repo.CreateRunEvent(context.Background(), event))
	}

	got, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		RunID:    20,
		Page:     1,
		PageSize: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].ID)
	require.Equal(t, int64(10), got[0].ThreadID)
	require.Equal(t, int64(20), got[0].RunID)
	require.Equal(t, "run.started", got[0].EventType)
	require.Equal(t, `{"step":1}`, got[0].Payload)
	require.Equal(t, int64(100), got[0].CreatedAt)
	require.Equal(t, int64(2), got[1].ID)
	require.Equal(t, "step.completed", got[1].EventType)

	threadEvents, threadTotal, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{
		ThreadID: 10,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), threadTotal)
	require.Len(t, threadEvents, 3)
	require.Equal(t, int64(3), threadEvents[0].ID)
	require.Equal(t, int64(1), threadEvents[1].ID)
	require.Equal(t, int64(2), threadEvents[2].ID)
}

func TestThreadRepositoryCreateListAndGetLatestCheckpoints(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&checkpointPO{}))

	repo := NewThreadRepository(db)
	for _, checkpoint := range []*entity.Checkpoint{
		{
			ID:              1,
			ThreadID:        10,
			RunID:           20,
			CheckpointNS:    "",
			ChannelValues:   `{"messages":["old"]}`,
			ChannelVersions: `{"messages":1}`,
			PendingSends:    `[]`,
			Metadata:        `{"source":"runtime"}`,
			CreatedAt:       100,
		},
		{
			ID:                 2,
			ThreadID:           10,
			RunID:              20,
			ParentCheckpointID: 1,
			CheckpointNS:       "planner",
			ChannelValues:      `{"messages":["new"],"next":["tools"]}`,
			ChannelVersions:    `{"messages":2,"next":1}`,
			PendingSends:       `[{"node":"tools"}]`,
			Metadata:           `{"source":"runtime","step":2}`,
			CreatedAt:          200,
		},
		{
			ID:              3,
			ThreadID:        11,
			RunID:           21,
			ChannelValues:   `{}`,
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{}`,
			CreatedAt:       300,
		},
	} {
		require.NoError(t, repo.CreateCheckpoint(context.Background(), checkpoint))
	}

	got, total, err := repo.ListCheckpoints(context.Background(), ListCheckpointsRequest{
		ThreadID: 10,
		Limit:    10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, got, 2)
	require.Equal(t, int64(2), got[0].ID)
	require.Equal(t, int64(1), got[0].ParentCheckpointID)
	require.Equal(t, "planner", got[0].CheckpointNS)
	require.Equal(t, `{"messages":["new"],"next":["tools"]}`, got[0].ChannelValues)
	require.Equal(t, `{"messages":2,"next":1}`, got[0].ChannelVersions)
	require.Equal(t, `[{"node":"tools"}]`, got[0].PendingSends)
	require.Equal(t, `{"source":"runtime","step":2}`, got[0].Metadata)
	require.Equal(t, int64(1), got[1].ID)

	latest, err := repo.GetLatestCheckpoint(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, int64(2), latest.ID)

	byID, err := repo.GetCheckpoint(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, int64(10), byID.ThreadID)
	require.Equal(t, int64(20), byID.RunID)
	require.Equal(t, int64(1), byID.ParentCheckpointID)
	require.Equal(t, "planner", byID.CheckpointNS)
	require.Equal(t, `{"messages":["new"],"next":["tools"]}`, byID.ChannelValues)
}

func TestThreadRepositoryRejectsInvalidCheckpointJSON(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&checkpointPO{}))

	repo := NewThreadRepository(db)
	err = repo.CreateCheckpoint(context.Background(), &entity.Checkpoint{
		ID:              1,
		ThreadID:        10,
		RunID:           20,
		ChannelValues:   "{",
		ChannelVersions: `{}`,
		PendingSends:    `[]`,
		Metadata:        `{}`,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "channel_values")
}

func TestThreadRepositoryCreateAndListMemories(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&memoryPO{}))

	repo := NewThreadRepository(db)
	for _, memory := range []*entity.Memory{
		{
			ID:        1,
			ThreadID:  10,
			RunID:     0,
			SpaceID:   1,
			Scope:     entity.MemoryScopeThread,
			Content:   "thread baseline",
			Metadata:  `{"source":"profile"}`,
			Score:     0.7,
			CreatedAt: 100,
			UpdatedAt: 100,
		},
		{
			ID:        2,
			ThreadID:  10,
			RunID:     20,
			SpaceID:   1,
			Scope:     entity.MemoryScopeRun,
			Content:   "current run",
			Metadata:  `{}`,
			Score:     0.9,
			CreatedAt: 200,
			UpdatedAt: 200,
		},
		{
			ID:        3,
			ThreadID:  10,
			RunID:     21,
			SpaceID:   1,
			Scope:     entity.MemoryScopeRun,
			Content:   "other run",
			Metadata:  `{}`,
			Score:     1,
			CreatedAt: 300,
			UpdatedAt: 300,
		},
		{
			ID:        4,
			ThreadID:  11,
			RunID:     20,
			SpaceID:   1,
			Scope:     entity.MemoryScopeThread,
			Content:   "other thread",
			Metadata:  `{}`,
			Score:     1,
			CreatedAt: 400,
			UpdatedAt: 400,
		},
		{
			ID:        5,
			ThreadID:  10,
			RunID:     0,
			SpaceID:   1,
			Scope:     entity.MemoryScopeThread,
			Content:   "expired",
			Metadata:  `{}`,
			Score:     1,
			ExpiresAt: 500,
			CreatedAt: 500,
			UpdatedAt: 500,
		},
		{
			ID:        6,
			ThreadID:  10,
			RunID:     0,
			SpaceID:   1,
			Scope:     entity.MemoryScopeLongTerm,
			Content:   "newer same score",
			Metadata:  `{}`,
			Score:     0.7,
			CreatedAt: 600,
			UpdatedAt: 600,
		},
	} {
		require.NoError(t, repo.CreateMemory(context.Background(), memory))
	}

	memories, total, err := repo.ListMemories(context.Background(), ListMemoriesRequest{
		ThreadID: 10,
		RunID:    20,
		Now:      1000,
		Limit:    10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, memories, 3)
	require.Equal(t, int64(2), memories[0].ID)
	require.Equal(t, "current run", memories[0].Content)
	require.Equal(t, `{"source":"profile"}`, memories[2].Metadata)
	require.Equal(t, []int64{2, 6, 1}, []int64{memories[0].ID, memories[1].ID, memories[2].ID})
}

func TestThreadRepositoryCreateAndAggregateTokenUsage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&tokenUsagePO{}))

	repo := NewThreadRepository(db)
	for _, usage := range []*entity.TokenUsage{
		{
			ID:           1,
			ThreadID:     10,
			RunID:        20,
			SpaceID:      1,
			Source:       entity.TokenUsageSourceLeadAgent,
			StepID:       "model-1",
			StepIndex:    0,
			StepName:     "generate_answer",
			ModelName:    "gpt-test",
			Provider:     "openai-compatible",
			InputTokens:  12,
			OutputTokens: 8,
			TotalTokens:  20,
			CostMicros:   250,
			Currency:     "USD",
			RawUsage:     `{"prompt_tokens":12,"completion_tokens":8}`,
			Metadata:     `{"phase":"test"}`,
			CreatedAt:    100,
		},
		{
			ID:           2,
			ThreadID:     10,
			RunID:        20,
			SpaceID:      1,
			Source:       entity.TokenUsageSourceTool,
			StepID:       "tool-1",
			StepIndex:    1,
			StepName:     "search",
			InputTokens:  4,
			OutputTokens: 6,
			TotalTokens:  10,
			CostMicros:   100,
			Estimated:    true,
			RawUsage:     `{"estimated":true}`,
			CreatedAt:    200,
		},
		{
			ID:           3,
			ThreadID:     10,
			RunID:        21,
			SpaceID:      1,
			Source:       entity.TokenUsageSourceMiddleware,
			InputTokens:  3,
			OutputTokens: 2,
			TotalTokens:  5,
			CostMicros:   50,
			CreatedAt:    300,
		},
		{
			ID:           4,
			ThreadID:     11,
			RunID:        22,
			SpaceID:      1,
			Source:       entity.TokenUsageSourceLeadAgent,
			InputTokens:  99,
			OutputTokens: 1,
			TotalTokens:  100,
			CostMicros:   900,
			CreatedAt:    400,
		},
	} {
		require.NoError(t, repo.CreateTokenUsage(context.Background(), usage))
	}

	rows, total, err := repo.ListTokenUsage(context.Background(), ListTokenUsageRequest{
		ThreadID: 10,
		RunID:    20,
		Page:     1,
		PageSize: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	require.Equal(t, int64(1), rows[0].ID)
	require.Equal(t, entity.TokenUsageSourceLeadAgent, rows[0].Source)
	require.Equal(t, `{"prompt_tokens":12,"completion_tokens":8}`, rows[0].RawUsage)
	require.Equal(t, int64(2), rows[1].ID)
	require.True(t, rows[1].Estimated)

	runAggregate, err := repo.AggregateTokenUsage(context.Background(), AggregateTokenUsageRequest{
		RunID: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(16), runAggregate.InputTokens)
	require.Equal(t, int64(14), runAggregate.OutputTokens)
	require.Equal(t, int64(30), runAggregate.TotalTokens)
	require.Equal(t, int64(350), runAggregate.CostMicros)
	require.Equal(t, int64(2), runAggregate.CallCount)
	require.Equal(t, int64(20), runAggregate.LeadAgentTokens)
	require.Equal(t, int64(10), runAggregate.ToolTokens)

	threadAggregate, err := repo.AggregateTokenUsage(context.Background(), AggregateTokenUsageRequest{
		ThreadID: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(35), threadAggregate.TotalTokens)
	require.Equal(t, int64(3), threadAggregate.CallCount)
	require.Equal(t, int64(5), threadAggregate.MiddlewareTokens)
}

func TestThreadRepositoryRejectsInvalidRunEventPayload(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runEventPO{}))

	repo := NewThreadRepository(db)
	err = repo.CreateRunEvent(context.Background(), &entity.RunEvent{
		ID:        1,
		ThreadID:  10,
		RunID:     20,
		EventType: "run.started",
		Payload:   "{",
		CreatedAt: 100,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "payload")
}

func TestThreadRepositoryClaimPendingRunsMarksOldestRunsRunning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	require.NoError(t, repo.CreateRun(context.Background(), newRepositoryTestRun(1, 10, entity.RunStatusPending, 100)))
	require.NoError(t, repo.CreateRun(context.Background(), newRepositoryTestRun(2, 10, entity.RunStatusPending, 101)))
	require.NoError(t, repo.CreateRun(context.Background(), newRepositoryTestRun(3, 10, entity.RunStatusRunning, 99)))

	claimed, err := repo.ClaimPendingRuns(context.Background(), ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(1), claimed[0].ID)
	require.Equal(t, entity.RunStatusRunning, claimed[0].Status)
	require.Equal(t, "worker-a", claimed[0].WorkerID)
	require.NotZero(t, claimed[0].StartedAt)
	got, err := repo.GetRun(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusRunning, got.Status)
	require.Equal(t, "worker-a", got.WorkerID)
	require.NotZero(t, got.StartedAt)
}

func TestThreadRepositoryClaimPendingRunsSkipsQueuedResumeRuns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	queued := newRepositoryTestRun(1, 10, entity.RunStatusQueued, 100)
	queued.Metadata = `{"checkpoint_resume":{"protected_from_worker_claim":true}}`
	require.NoError(t, repo.CreateRun(context.Background(), queued))
	require.NoError(t, repo.CreateRun(context.Background(), newRepositoryTestRun(2, 10, entity.RunStatusPending, 101)))

	claimed, err := repo.ClaimPendingRuns(context.Background(), ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    10,
	})

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(2), claimed[0].ID)

	gotQueued, err := repo.GetRun(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusQueued, gotQueued.Status)
	require.Empty(t, gotQueued.WorkerID)
}

func TestThreadRepositoryUpdateRunStatusUsesExpectedStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	run := newRepositoryTestRun(1, 10, entity.RunStatusRunning, 100)
	run.WorkerID = "worker-a"
	require.NoError(t, repo.CreateRun(context.Background(), run))

	require.NoError(t, repo.UpdateRunStatus(context.Background(), UpdateRunStatusRequest{
		RunID:    1,
		From:     entity.RunStatusRunning,
		To:       entity.RunStatusSucceeded,
		WorkerID: "worker-a",
	}))
	got, err := repo.GetRun(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusSucceeded, got.Status)
	require.NotZero(t, got.EndedAt)

	err = repo.UpdateRunStatus(context.Background(), UpdateRunStatusRequest{
		RunID:    1,
		From:     entity.RunStatusRunning,
		To:       entity.RunStatusFailed,
		WorkerID: "worker-a",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in status")
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
