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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestCanonicalExtensionsKeepLegacyListThreadsPagingAndOrder(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &threadPO{}, &runPO{})
	repo := NewThreadRepository(db)
	for id := int64(1); id <= 23; id++ {
		require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
			ID: id, SpaceID: 10, CreatorID: 20, Title: fmt.Sprintf("thread-%d", id),
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			CreatedAt: 100, UpdatedAt: 100,
		}))
	}
	for _, thread := range []*entity.Thread{
		{ID: 24, SpaceID: 10, CreatorID: 21, Title: "other user", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, UpdatedAt: 200},
		{ID: 25, SpaceID: 11, CreatorID: 20, Title: "other space", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, UpdatedAt: 300},
	} {
		require.NoError(t, repo.CreateThread(context.Background(), thread))
	}

	threads, total, err := repo.ListThreads(context.Background(), ListThreadsRequest{
		SpaceID: 10,
		UserID:  20,
	})

	require.NoError(t, err)
	require.Equal(t, int64(23), total)
	require.Equal(t, canonicalDescendingIDs(23, 4), threadIDs(threads))
}

func TestCanonicalExtensionsKeepLegacyListRunsPagingAndOrder(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &runPO{})
	repo := NewThreadRepository(db)
	for id := int64(1); id <= 23; id++ {
		require.NoError(t, repo.CreateRun(
			context.Background(), newCanonicalRepositoryRun(id, 10, 0, 100),
		))
	}
	require.NoError(t, repo.CreateRun(context.Background(), newCanonicalRepositoryRun(24, 10, 99, 200)))
	require.NoError(t, repo.CreateRun(context.Background(), newCanonicalRepositoryRun(25, 11, 0, 300)))

	runs, total, err := repo.ListRuns(context.Background(), ListRunsRequest{ThreadID: 10})

	require.NoError(t, err)
	require.Equal(t, int64(23), total)
	require.Equal(t, canonicalDescendingIDs(23, 4), runIDs(runs))
}

func TestCanonicalExtensionsKeepLegacyListMessagesPagingAndOrder(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &messagePO{})
	repo := NewThreadRepository(db)
	for id := int64(1); id <= 53; id++ {
		require.NoError(t, repo.CreateMessage(context.Background(), &entity.Message{
			ID: id, ThreadID: 10, Role: entity.MessageRoleUser,
			Content: fmt.Sprintf("message-%d", id), CreatedAt: 100,
		}))
	}
	require.NoError(t, repo.CreateMessage(context.Background(), &entity.Message{
		ID: 54, ThreadID: 11, Role: entity.MessageRoleUser, Content: "other", CreatedAt: 50,
	}))

	messages, total, err := repo.ListMessages(context.Background(), ListMessagesRequest{ThreadID: 10})

	require.NoError(t, err)
	require.Equal(t, int64(53), total)
	require.Equal(t, canonicalAscendingIDs(1, 50), messageIDs(messages))
}

func TestCanonicalExtensionsKeepLegacyListRunEventsPagingAndOrder(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &runEventPO{})
	repo := NewThreadRepository(db)
	for id := int64(1); id <= 103; id++ {
		require.NoError(t, repo.CreateRunEvent(context.Background(), &entity.RunEvent{
			ID: id, ThreadID: 10, RunID: 20,
			EventType: fmt.Sprintf("event-%d", id), Payload: `{}`,
		}))
	}
	require.NoError(t, repo.CreateRunEvent(context.Background(), &entity.RunEvent{
		ID: 104, ThreadID: 11, RunID: 21, EventType: "other", Payload: `{}`,
	}))

	events, total, err := repo.ListRunEvents(context.Background(), ListRunEventsRequest{ThreadID: 10})

	require.NoError(t, err)
	require.Equal(t, int64(103), total)
	require.Equal(t, canonicalAscendingIDs(1, 100), runEventIDs(events))
}

func TestCanonicalExtensionsKeepLegacyListCheckpointsPagingAndOrder(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &checkpointPO{})
	repo := NewThreadRepository(db)
	for id := int64(1); id <= 23; id++ {
		runtimeType := "eino_adk"
		if id%2 == 0 {
			runtimeType = "canonical_public_state"
		}
		require.NoError(t, repo.CreateCheckpoint(context.Background(),
			newCanonicalRepositoryCheckpoint(id, 10, 20, runtimeType, 100)))
	}
	require.NoError(t, repo.CreateCheckpoint(context.Background(),
		newCanonicalRepositoryCheckpoint(24, 11, 21, "eino_adk", 200)))

	checkpoints, total, err := repo.ListCheckpoints(context.Background(), ListCheckpointsRequest{ThreadID: 10})

	require.NoError(t, err)
	require.Equal(t, int64(23), total)
	require.Equal(t, canonicalDescendingIDs(23, 4), checkpointIDs(checkpoints))
}

func TestCanonicalExtensionsKeepLegacyDeleteThreadCascade(t *testing.T) {
	db := canonicalRepositoryTestDB(t,
		&threadPO{},
		&messagePO{},
		&runPO{},
		&runEventPO{},
		&checkpointPO{},
		&memoryPO{},
		&memoryAuditEventPO{},
		&transcriptSnapshotPO{},
		&memoryFlushJobPO{},
		&tokenUsagePO{},
		&agentFilePO{},
		&agentArtifactPO{},
		&agentArtifactScanJobPO{},
		&agentRunPlanPO{},
		&agentRunPlanItemPO{},
	)
	repo := NewThreadRepository(db)
	require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
		ID: 1, SpaceID: 10, CreatorID: 20, Title: "delete",
		Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
	}))
	require.NoError(t, db.Create(&messagePO{ID: 10, ThreadID: 1, Metadata: []byte(`{}`)}).Error)
	require.NoError(t, db.Create(&runPO{ID: 11, ThreadID: 1, Command: []byte(`{}`), Input: []byte(`{}`), Config: []byte(`{}`), Context: []byte(`{}`), Metadata: []byte(`{}`), StreamMode: []byte(`[]`)}).Error)
	require.NoError(t, db.Create(&runEventPO{ID: 12, ThreadID: 1, RunID: 11, Payload: []byte(`{}`)}).Error)
	require.NoError(t, db.Create(&checkpointPO{ID: 13, ThreadID: 1, RunID: 11, ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`), Metadata: []byte(`{}`)}).Error)
	require.NoError(t, db.Create(&memoryPO{ID: 14, ThreadID: 1, Metadata: []byte(`{}`)}).Error)
	require.NoError(t, db.Create(&memoryAuditEventPO{ID: 15, ThreadID: 1}).Error)
	require.NoError(t, db.Create(&transcriptSnapshotPO{ID: 16, ThreadID: 1, RunID: 11, IdempotencyKey: "snapshot", Messages: []byte(`[]`), Metadata: []byte(`{}`)}).Error)
	require.NoError(t, db.Create(&memoryFlushJobPO{ID: 17, ThreadID: 1, RunID: 11, IdempotencyKey: "flush"}).Error)
	require.NoError(t, db.Create(&tokenUsagePO{ID: 18, ThreadID: 1, RawUsage: []byte(`{}`), Metadata: []byte(`{}`)}).Error)
	require.NoError(t, db.Create(&agentFilePO{ID: 19, ThreadID: 1, RunID: 11, VirtualPathHash: "path", Metadata: []byte(`{}`)}).Error)
	require.NoError(t, db.Create(&agentArtifactPO{ID: 20, ThreadID: 1, RunID: 11, FileID: 19, Metadata: []byte(`{}`)}).Error)
	require.NoError(t, db.Create(&agentArtifactScanJobPO{ID: 21, ThreadID: 1, ArtifactID: 20, IdempotencyKey: "scan"}).Error)
	require.NoError(t, db.Create(&agentRunPlanPO{RunID: 11, ThreadID: 1}).Error)
	require.NoError(t, db.Create(&agentRunPlanItemPO{ID: 22, RunID: 11, TaskID: 1, Blocks: []byte(`[]`), BlockedBy: []byte(`[]`), Metadata: []byte(`{}`)}).Error)

	deleted, err := repo.DeleteThread(context.Background(), DeleteThreadRequest{ThreadID: 1})

	require.NoError(t, err)
	require.True(t, deleted)
	for _, model := range []any{
		&threadPO{}, &messagePO{}, &runPO{}, &runEventPO{}, &checkpointPO{},
		&memoryPO{}, &memoryAuditEventPO{}, &transcriptSnapshotPO{}, &memoryFlushJobPO{},
		&tokenUsagePO{}, &agentFilePO{}, &agentArtifactPO{}, &agentArtifactScanJobPO{},
		&agentRunPlanPO{}, &agentRunPlanItemPO{},
	} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestCanonicalSearchThreadsUsesExactOffsetMetadataAndPermissionTotal(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &threadPO{}, &runPO{})
	repo := &threadRepository{db: db}
	ids := make([]int64, 0, 19)
	for id := int64(1); id <= 15; id++ {
		ids = append(ids, id)
		require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
			ID: id, SpaceID: 10, CreatorID: 20, Title: "match",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			Metadata:  `{"scope":"canonical","enabled":true,"ratio":1.5,"nullable":null}`,
			CreatedAt: id, UpdatedAt: id,
		}))
	}
	for _, thread := range []*entity.Thread{
		{ID: 16, SpaceID: 10, CreatorID: 21, Title: "other user", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, Metadata: `{"scope":"canonical","enabled":true,"ratio":1.5,"nullable":null}`},
		{ID: 17, SpaceID: 11, CreatorID: 20, Title: "other space", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, Metadata: `{"scope":"canonical","enabled":true,"ratio":1.5,"nullable":null}`},
		{ID: 18, SpaceID: 10, CreatorID: 20, Title: "other metadata", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, Metadata: `{"scope":"other","enabled":true,"ratio":1.5,"nullable":null}`},
		{ID: 19, SpaceID: 10, CreatorID: 20, Title: "other status", Status: entity.ThreadStatusFailed, Source: entity.ThreadSourceWeb, Metadata: `{"scope":"canonical","enabled":true,"ratio":1.5,"nullable":null}`},
	} {
		ids = append(ids, thread.ID)
		require.NoError(t, repo.CreateThread(context.Background(), thread))
	}
	status := entity.ThreadStatusIdle

	threads, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10,
		UserID:  20,
		IDs:     ids,
		Status:  &status,
		Metadata: map[string]any{
			"scope":    "canonical",
			"enabled":  true,
			"ratio":    1.5,
			"nullable": nil,
		},
		SortBy:    "thread_id",
		SortOrder: "asc",
		Page:      CanonicalPage{Offset: 7, Limit: 3},
	})

	require.NoError(t, err)
	require.Equal(t, int64(15), total)
	require.Equal(t, []int64{8, 9, 10}, threadIDs(threads))
}

func TestCanonicalSearchThreadsMatchesLiteralTopLevelMetadataKeys(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &threadPO{}, &runPO{})
	repo := &threadRepository{db: db}
	for _, thread := range []*entity.Thread{
		{
			ID: 1, SpaceID: 10, CreatorID: 20, Title: "literal keys",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			Metadata: `{"source.kind":"canonical","feature-flag":true,"nullable.key":null}`,
		},
		{
			ID: 2, SpaceID: 10, CreatorID: 20, Title: "nested keys",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			Metadata: `{"source":{"kind":"canonical"},"feature":{"flag":true},"nullable":{"key":null}}`,
		},
	} {
		require.NoError(t, repo.CreateThread(context.Background(), thread))
	}

	tests := []struct {
		name     string
		metadata map[string]any
	}{
		{name: "dot key", metadata: map[string]any{"source.kind": "canonical"}},
		{name: "hyphen key", metadata: map[string]any{"feature-flag": true}},
		{name: "null dot key", metadata: map[string]any{"nullable.key": nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threads, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
				SpaceID: 10, UserID: 20, Metadata: tt.metadata,
				SortBy: "thread_id", SortOrder: "asc", Page: CanonicalPage{Limit: 10},
			})

			require.NoError(t, err)
			require.Equal(t, int64(1), total)
			require.Equal(t, []int64{1}, threadIDs(threads))
		})
	}
}

func TestCanonicalSearchThreadsSQLiteMatchesExactJSONScalarRepresentation(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &threadPO{}, &runPO{})
	repo := &threadRepository{db: db}
	for _, thread := range []*entity.Thread{
		{
			ID: 1, SpaceID: 10, CreatorID: 20, Title: "exact scalars",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			Metadata: `{
				"large":9223372036854775808,
				"precise":0.123456789012345678901234567890,
				"label":"canonical",
				"escaped":"\u0061",
				"enabled":true,
				"nullable":null,
				"ordinary":1.5,
				"representation":1
			}`,
		},
		{
			ID: 2, SpaceID: 10, CreatorID: 20, Title: "adjacent scalars",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			Metadata: `{
				"large":9223372036854775809,
				"precise":0.123456789012345678901234567891,
				"label":"other",
				"enabled":false,
				"ordinary":2.5
			}`,
		},
		{
			ID: 3, SpaceID: 10, CreatorID: 20, Title: "decimal representation",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			Metadata: `{"representation":1.0}`,
		},
	} {
		require.NoError(t, repo.CreateThread(context.Background(), thread))
	}

	tests := []struct {
		name     string
		metadata map[string]any
		want     []int64
	}{
		{name: "large json number", metadata: map[string]any{"large": json.Number("9223372036854775808")}, want: []int64{1}},
		{name: "large uint64", metadata: map[string]any{"large": uint64(1) << 63}, want: []int64{1}},
		{name: "precise decimal", metadata: map[string]any{"precise": json.Number("0.123456789012345678901234567890")}, want: []int64{1}},
		{name: "string", metadata: map[string]any{"label": "canonical"}, want: []int64{1}},
		{name: "escaped string", metadata: map[string]any{"escaped": "a"}, want: []int64{1}},
		{name: "boolean", metadata: map[string]any{"enabled": true}, want: []int64{1}},
		{name: "null", metadata: map[string]any{"nullable": nil}, want: []int64{1}},
		{name: "ordinary number", metadata: map[string]any{"ordinary": 1.5}, want: []int64{1}},
		{name: "integer representation", metadata: map[string]any{"representation": json.Number("1")}, want: []int64{1}},
		{name: "decimal representation", metadata: map[string]any{"representation": json.Number("1.0")}, want: []int64{3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threads, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
				SpaceID: 10, UserID: 20, Metadata: tt.metadata,
				SortBy: "thread_id", SortOrder: "asc", Page: CanonicalPage{Limit: 10},
			})

			require.NoError(t, err)
			require.Equal(t, int64(len(tt.want)), total)
			require.Equal(t, tt.want, threadIDs(threads))
		})
	}
}

func TestCanonicalSearchThreadsUsesWhitelistedSortsAndSameDirectionTieBreaker(t *testing.T) {
	tests := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      []int64
	}{
		{name: "thread id asc", sortBy: "thread_id", sortOrder: "asc", want: []int64{1, 2, 3, 4}},
		{name: "thread id desc", sortBy: "thread_id", sortOrder: "desc", want: []int64{4, 3, 2, 1}},
		{name: "status asc", sortBy: "status", sortOrder: "asc", want: []int64{4, 3, 1, 2}},
		{name: "status desc", sortBy: "status", sortOrder: "desc", want: []int64{2, 1, 3, 4}},
		{name: "created asc", sortBy: "created_at", sortOrder: "asc", want: []int64{1, 2, 4, 3}},
		{name: "created desc", sortBy: "created_at", sortOrder: "desc", want: []int64{3, 4, 2, 1}},
		{name: "updated asc", sortBy: "updated_at", sortOrder: "asc", want: []int64{2, 3, 1, 4}},
		{name: "updated desc", sortBy: "updated_at", sortOrder: "desc", want: []int64{4, 1, 3, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := canonicalRepositoryTestDB(t, &threadPO{}, &runPO{})
			repo := &threadRepository{db: db}
			for _, thread := range []*entity.Thread{
				{ID: 1, SpaceID: 10, CreatorID: 20, Title: "one", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, CreatedAt: 100, UpdatedAt: 200},
				{ID: 2, SpaceID: 10, CreatorID: 20, Title: "two", Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb, CreatedAt: 100, UpdatedAt: 100},
				{ID: 3, SpaceID: 10, CreatorID: 20, Title: "three", Status: entity.ThreadStatusFailed, Source: entity.ThreadSourceWeb, CreatedAt: 200, UpdatedAt: 100},
				{ID: 4, SpaceID: 10, CreatorID: 20, Title: "four", Status: entity.ThreadStatusCompleted, Source: entity.ThreadSourceWeb, CreatedAt: 100, UpdatedAt: 300},
			} {
				require.NoError(t, repo.CreateThread(context.Background(), thread))
			}

			threads, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
				SpaceID: 10, UserID: 20,
				SortBy: tt.sortBy, SortOrder: tt.sortOrder,
				Page: CanonicalPage{Limit: 10},
			})

			require.NoError(t, err)
			require.Equal(t, int64(4), total)
			require.Equal(t, tt.want, threadIDs(threads))
		})
	}
}

func TestCanonicalSearchRunsUsesExactOffsetAndTopLevelFilters(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &runPO{})
	repo := &threadRepository{db: db}
	for id := int64(1); id <= 12; id++ {
		run := newCanonicalRepositoryRun(id, 10, 0, id)
		run.Status = entity.RunStatusSucceeded
		require.NoError(t, repo.CreateRun(context.Background(), run))
	}
	child := newCanonicalRepositoryRun(13, 10, 99, 100)
	child.Status = entity.RunStatusSucceeded
	require.NoError(t, repo.CreateRun(context.Background(), child))
	otherThread := newCanonicalRepositoryRun(14, 11, 0, 100)
	otherThread.Status = entity.RunStatusSucceeded
	require.NoError(t, repo.CreateRun(context.Background(), otherThread))
	failed := newCanonicalRepositoryRun(15, 10, 0, 100)
	failed.Status = entity.RunStatusFailed
	require.NoError(t, repo.CreateRun(context.Background(), failed))
	status := entity.RunStatusSucceeded

	runs, total, err := repo.SearchRuns(context.Background(), SearchRunsRequest{
		ThreadID: 10,
		Status:   &status,
		Page:     CanonicalPage{Offset: 7, Limit: 3},
	})

	require.NoError(t, err)
	require.Equal(t, int64(12), total)
	require.Equal(t, []int64{5, 4, 3}, runIDs(runs))

	parentRunID := int64(99)
	children, childTotal, err := repo.SearchRuns(context.Background(), SearchRunsRequest{
		ThreadID: 10, ParentRunID: &parentRunID,
		Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), childTotal)
	require.Equal(t, []int64{13}, runIDs(children))
}

func TestCanonicalListRunEventsByCursorFiltersTypeAndReportsHasMore(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &runEventPO{})
	repo := &threadRepository{db: db}
	for _, event := range []*entity.RunEvent{
		{ID: 1, ThreadID: 10, RunID: 20, EventType: "message", Payload: `{}`},
		{ID: 2, ThreadID: 10, RunID: 20, EventType: "debug", Payload: `{}`},
		{ID: 3, ThreadID: 10, RunID: 20, EventType: "status", Payload: `{}`},
		{ID: 4, ThreadID: 10, RunID: 20, EventType: "message", Payload: `{}`},
		{ID: 5, ThreadID: 10, RunID: 20, EventType: "status", Payload: `{}`},
		{ID: 6, ThreadID: 10, RunID: 21, EventType: "message", Payload: `{}`},
		{ID: 7, ThreadID: 11, RunID: 20, EventType: "message", Payload: `{}`},
	} {
		require.NoError(t, repo.CreateRunEvent(context.Background(), event))
	}

	events, hasMore, err := repo.ListRunEventsByCursor(context.Background(), ListRunEventsByCursorRequest{
		ThreadID:     10,
		RunID:        20,
		AfterEventID: 2,
		EventTypes:   []string{"message", "status"},
		Limit:        2,
	})

	require.NoError(t, err)
	require.True(t, hasMore)
	require.Equal(t, []int64{3, 4}, runEventIDs(events))
}

func TestCanonicalListCheckpointsBeforeUsesCursorAndReportsHasMore(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &checkpointPO{})
	repo := &threadRepository{db: db}
	runtimeTypes := []string{
		"eino_adk",
		"canonical_public_state",
		"eino_adk",
		"canonical_public_state",
		"eino_adk",
	}
	for index, runtimeType := range runtimeTypes {
		id := int64(index + 1)
		require.NoError(t, repo.CreateCheckpoint(context.Background(),
			newCanonicalRepositoryCheckpoint(id, 10, 20, runtimeType, id)))
	}
	require.NoError(t, repo.CreateCheckpoint(context.Background(),
		newCanonicalRepositoryCheckpoint(6, 11, 21, "eino_adk", 100)))

	checkpoints, hasMore, err := repo.ListCheckpointsBefore(context.Background(), ListCheckpointsBeforeRequest{
		ThreadID:           10,
		BeforeCheckpointID: 5,
		Limit:              2,
	})

	require.NoError(t, err)
	require.True(t, hasMore)
	require.Equal(t, []int64{4, 3}, checkpointIDs(checkpoints))
	require.Equal(t, []string{"canonical_public_state", "eino_adk"}, checkpointRuntimeTypes(checkpoints))

	checkpoints, hasMore, err = repo.ListCheckpointsBefore(context.Background(), ListCheckpointsBeforeRequest{
		ThreadID:           10,
		BeforeCheckpointID: 3,
		Limit:              10,
	})
	require.NoError(t, err)
	require.False(t, hasMore)
	require.Equal(t, []int64{2, 1}, checkpointIDs(checkpoints))
	require.Equal(t, []string{"canonical_public_state", "eino_adk"}, checkpointRuntimeTypes(checkpoints))
}

func TestCanonicalPatchThreadAtomicallyMergesMetadataAndTitle(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &threadPO{}, &runPO{})
	repo := &threadRepository{db: db}
	require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
		ID: 10, SpaceID: 20, CreatorID: 30, Title: "old",
		Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
		Metadata: `{"source":"old","keep":true}`, CreatedAt: 1, UpdatedAt: 2,
	}))
	title := "new"

	patched, err := repo.PatchThread(context.Background(), PatchThreadRequest{
		ThreadID: 10,
		Title:    &title,
		MetadataPatch: map[string]any{
			"custom": "value",
		},
		UpdatedAt: 100,
	})

	require.NoError(t, err)
	require.Equal(t, "new", patched.Title)
	require.Equal(t, int64(100), patched.UpdatedAt)
	require.JSONEq(t, `{"source":"old","keep":true,"custom":"value"}`, patched.Metadata)
	persisted, err := repo.GetThread(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, patched, persisted)
}

func TestCanonicalPatchThreadPreservesExistingLargeIntegerMetadata(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &threadPO{}, &runPO{})
	repo := &threadRepository{db: db}
	require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
		ID: 10, SpaceID: 20, CreatorID: 30, Title: "numbers",
		Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
		Metadata: `{"large":9007199254740993,"keep":true}`,
	}))

	patched, err := repo.PatchThread(context.Background(), PatchThreadRequest{
		ThreadID: 10, MetadataPatch: map[string]any{"new_key": "value"}, UpdatedAt: 100,
	})

	require.NoError(t, err)
	require.Contains(t, patched.Metadata, `"large":9007199254740993`)
	decoder := json.NewDecoder(strings.NewReader(patched.Metadata))
	decoder.UseNumber()
	var metadata map[string]any
	require.NoError(t, decoder.Decode(&metadata))
	require.Equal(t, json.Number("9007199254740993"), metadata["large"])
	require.Equal(t, true, metadata["keep"])
	require.Equal(t, "value", metadata["new_key"])

	persisted, err := repo.GetThread(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, patched.Metadata, persisted.Metadata)
}

func TestCanonicalPatchThreadRejectsCurrentMetadataThatIsNotSingleObject(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
	}{
		{name: "null", metadata: `null`},
		{name: "array", metadata: `[]`},
		{name: "trailing value", metadata: `{} {}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := canonicalRepositoryTestDB(t, &threadPO{}, &runPO{})
			repo := &threadRepository{db: db}
			require.NoError(t, db.Create(&threadPO{
				ID: 10, SpaceID: 20, CreatorID: 30, Title: "invalid metadata",
				Status: string(entity.ThreadStatusIdle), Source: string(entity.ThreadSourceWeb),
				Metadata: []byte(tt.metadata),
			}).Error)

			patched, err := repo.PatchThread(context.Background(), PatchThreadRequest{
				ThreadID: 10, MetadataPatch: map[string]any{"new_key": "value"}, UpdatedAt: 100,
			})

			require.Error(t, err)
			require.Nil(t, patched)
			var persisted threadPO
			require.NoError(t, db.Where("id = ?", 10).First(&persisted).Error)
			require.Equal(t, tt.metadata, string(persisted.Metadata))
		})
	}
}

func TestCanonicalPatchThreadConcurrentMergesDoNotLoseKeys(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open(canonicalSharedRepositoryTestDSN("canonical-thread-patch")),
		&gorm.Config{},
	)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&threadPO{}, &runPO{}))
	repo := &threadRepository{db: db}
	require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
		ID: 10, SpaceID: 20, CreatorID: 30, Title: "thread",
		Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
		Metadata: `{"base":true}`, CreatedAt: 1, UpdatedAt: 2,
	}))

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for index, key := range []string{"first", "second"} {
		wg.Add(1)
		go func(index int, key string) {
			defer wg.Done()
			<-start
			_, patchErr := repo.PatchThread(context.Background(), PatchThreadRequest{
				ThreadID: 10, MetadataPatch: map[string]any{key: true},
				UpdatedAt: int64(100 + index),
			})
			errs <- patchErr
		}(index, key)
	}
	close(start)
	wg.Wait()
	close(errs)
	for patchErr := range errs {
		require.NoError(t, patchErr)
	}

	patched, err := repo.GetThread(context.Background(), 10)
	require.NoError(t, err)
	require.JSONEq(t, `{"base":true,"first":true,"second":true}`, patched.Metadata)
}

func TestCanonicalMySQLThreadMutationsLockThreadFirstForUpdate(t *testing.T) {
	expectedErr := errors.New("stop after thread lock")
	const lockedThreadQuery = "SELECT \\* FROM `agent_threads` WHERE id = \\? ORDER BY `agent_threads`.`id` LIMIT \\? FOR UPDATE"

	tests := []struct {
		name string
		call func(*threadRepository) error
	}{
		{
			name: "patch thread",
			call: func(repo *threadRepository) error {
				title := "updated"
				_, err := repo.PatchThread(context.Background(), PatchThreadRequest{
					ThreadID: 10, Title: &title, UpdatedAt: 100,
				})
				return err
			},
		},
		{
			name: "delete thread if idle",
			call: func(repo *threadRepository) error {
				_, err := repo.DeleteThreadIfIdle(context.Background(), DeleteThreadIfIdleRequest{ThreadID: 10})
				return err
			},
		},
		{
			name: "create run bundle",
			call: func(repo *threadRepository) error {
				run := newCanonicalRepositoryRun(20, 10, 0, 100)
				run.Status = entity.RunStatusPending
				run.MultitaskStrategy = "reject"
				_, err := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{Run: run})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := canonicalMySQLMockRepository(t)
			mock.ExpectBegin()
			mock.ExpectQuery(lockedThreadQuery).
				WithArgs(int64(10), 1).
				WillReturnError(expectedErr)
			mock.ExpectRollback()

			require.ErrorIs(t, tt.call(repo), expectedErr)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCanonicalDeleteThreadIfIdleBlocksOnlyActiveTopLevelTaskRuns(t *testing.T) {
	activeStatuses := []entity.RunStatus{
		entity.RunStatusPending,
		entity.RunStatusQueued,
		entity.RunStatusRunning,
	}
	for index, status := range activeStatuses {
		t.Run(string(status), func(t *testing.T) {
			db := canonicalDeleteRepositoryTestDB(t, ":memory:")
			repo := &threadRepository{db: db}
			threadID := int64(index + 1)
			require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
				ID: threadID, SpaceID: 10, CreatorID: 20, Title: "active",
				Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			}))
			run := newCanonicalRepositoryRun(100+threadID, threadID, 0, 100)
			run.Status = status
			require.NoError(t, repo.CreateRun(context.Background(), run))

			deleted, err := repo.DeleteThreadIfIdle(context.Background(), DeleteThreadIfIdleRequest{
				ThreadID: threadID,
			})

			require.False(t, deleted)
			require.ErrorIs(t, err, ErrActiveRunExists)
			_, err = repo.GetThread(context.Background(), threadID)
			require.NoError(t, err)
			_, err = repo.GetRun(context.Background(), run.ID)
			require.NoError(t, err)
		})
	}

	nonBlocking := []struct {
		name        string
		status      entity.RunStatus
		parentRunID int64
	}{
		{name: "interrupted", status: entity.RunStatusInterrupted},
		{name: "succeeded", status: entity.RunStatusSucceeded},
		{name: "failed", status: entity.RunStatusFailed},
		{name: "canceled", status: entity.RunStatusCanceled},
		{name: "active child", status: entity.RunStatusRunning, parentRunID: 999},
	}
	for index, tt := range nonBlocking {
		t.Run(tt.name, func(t *testing.T) {
			db := canonicalDeleteRepositoryTestDB(t, ":memory:")
			repo := &threadRepository{db: db}
			threadID := int64(index + 10)
			require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
				ID: threadID, SpaceID: 10, CreatorID: 20, Title: "idle",
				Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			}))
			run := newCanonicalRepositoryRun(200+threadID, threadID, tt.parentRunID, 100)
			run.Status = tt.status
			require.NoError(t, repo.CreateRun(context.Background(), run))

			deleted, err := repo.DeleteThreadIfIdle(context.Background(), DeleteThreadIfIdleRequest{
				ThreadID: threadID,
			})

			require.NoError(t, err)
			require.True(t, deleted)
			_, err = repo.GetThread(context.Background(), threadID)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		})
	}
}

func TestCanonicalDeleteThreadIfIdleLinearizesWithCreateRunBundle(t *testing.T) {
	db := canonicalDeleteRepositoryTestDB(
		t,
		canonicalSharedRepositoryTestDSN("canonical-thread-delete"),
	)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	repo := &threadRepository{db: db}
	require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
		ID: 10, SpaceID: 10, CreatorID: 20, Title: "race",
		Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
	}))
	run := newCanonicalRepositoryRun(20, 10, 0, 100)
	run.Status = entity.RunStatusPending
	run.MultitaskStrategy = "reject"
	message := &entity.Message{
		ID: 30, ThreadID: 10, RunID: 20,
		Role: entity.MessageRoleUser, Content: "start", Metadata: `{}`,
	}
	event := &entity.RunEvent{
		ID: 40, ThreadID: 10, RunID: 20,
		EventType: "run.created", Payload: `{}`,
	}

	start := make(chan struct{})
	type createResult struct {
		result *CreateRunBundleResult
		err    error
	}
	createResults := make(chan createResult, 1)
	type deleteResult struct {
		deleted bool
		err     error
	}
	deleteResults := make(chan deleteResult, 1)
	go func() {
		<-start
		result, createErr := repo.CreateRunBundle(context.Background(), CreateRunBundleRequest{
			Run: run, Message: message, Event: event,
		})
		createResults <- createResult{result: result, err: createErr}
	}()
	go func() {
		<-start
		deleted, deleteErr := repo.DeleteThreadIfIdle(context.Background(), DeleteThreadIfIdleRequest{
			ThreadID: 10,
		})
		deleteResults <- deleteResult{deleted: deleted, err: deleteErr}
	}()
	close(start)
	created := <-createResults
	deleted := <-deleteResults

	if created.err == nil {
		require.NotNil(t, created.result)
		require.True(t, created.result.Created)
		require.False(t, deleted.deleted)
		require.ErrorIs(t, deleted.err, ErrActiveRunExists)
		for _, model := range []any{&threadPO{}, &runPO{}, &messagePO{}, &runEventPO{}} {
			var count int64
			require.NoError(t, db.Model(model).Count(&count).Error)
			require.Equal(t, int64(1), count)
		}
		return
	}

	require.ErrorIs(t, created.err, gorm.ErrRecordNotFound)
	require.NoError(t, deleted.err)
	require.True(t, deleted.deleted)
	for _, model := range []any{&threadPO{}, &runPO{}, &messagePO{}, &runEventPO{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestCanonicalPublicStateCheckpointDoesNotModifyEinoResumeCheckpoint(t *testing.T) {
	db := canonicalRepositoryTestDB(t, &checkpointPO{})
	repo := &threadRepository{db: db}
	eino := &entity.Checkpoint{
		ID: 1, ThreadID: 10, RunID: 20,
		CheckpointNS: "eino.adk", RuntimeType: "eino_adk", RuntimeKey: "coze-run-20",
		EnvelopeVersion: 3, ChannelValues: `{"checkpoint_bytes":"opaque-eino"}`,
		ChannelVersions: `{"messages":"v7"}`, PendingSends: `[{"task":"resume"}]`,
		Metadata: `{"runtime":"eino_adk"}`, CreatedAt: 100,
	}
	require.NoError(t, repo.CreateCheckpoint(context.Background(), eino))
	einoBefore := *eino
	public := &entity.Checkpoint{
		ID: 2, ThreadID: 10, RunID: 20, ParentCheckpointID: 1,
		CheckpointNS: "canonical.public", RuntimeType: "canonical_public_state",
		RuntimeKey: "thread:10", EnvelopeVersion: 0,
		ChannelValues: `{"custom":{"visible":true}}`, ChannelVersions: `{}`,
		PendingSends: `[]`, Metadata: `{"source":"canonical_public_state"}`, CreatedAt: 200,
	}

	require.NoError(t, repo.CreateCheckpoint(context.Background(), public))

	persistedEino, err := repo.GetCheckpoint(context.Background(), eino.ID)
	require.NoError(t, err)
	require.Equal(t, &einoBefore, persistedEino)
	resume, err := repo.GetLatestRuntimeCheckpoint(
		context.Background(), 10, 20, "eino_adk", "coze-run-20",
	)
	require.NoError(t, err)
	require.Equal(t, &einoBefore, resume)
	checkpoints, total, err := repo.ListCheckpoints(context.Background(), ListCheckpointsRequest{
		ThreadID: 10, Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []int64{2, 1}, checkpointIDs(checkpoints))
}

func canonicalDeleteRepositoryTestDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&threadPO{},
		&messagePO{},
		&runPO{},
		&runEventPO{},
		&checkpointPO{},
		&memoryPO{},
		&memoryAuditEventPO{},
		&transcriptSnapshotPO{},
		&memoryFlushJobPO{},
		&tokenUsagePO{},
		&agentFilePO{},
		&agentArtifactPO{},
		&agentArtifactScanJobPO{},
		&agentRunPlanPO{},
		&agentRunPlanItemPO{},
	))
	return db
}

var canonicalRepositoryTestDBSequence atomic.Uint64

func canonicalSharedRepositoryTestDSN(name string) string {
	return fmt.Sprintf(
		"file:%s-%d?mode=memory&cache=shared&_busy_timeout=5000",
		name,
		canonicalRepositoryTestDBSequence.Add(1),
	)
}

func canonicalRepositoryTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(models...))
	return db
}

func canonicalMySQLMockRepository(t *testing.T) (*threadRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, sqlDB.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	return &threadRepository{db: db}, mock
}

func newCanonicalRepositoryRun(id, threadID, parentRunID, createdAt int64) *entity.Run {
	return &entity.Run{
		ID: id, ThreadID: threadID, ParentRunID: parentRunID,
		SpaceID: 10, CreatorID: 20, AssistantID: "agent",
		RunKind: entity.DefaultRunKind("", parentRunID), Status: entity.RunStatusSucceeded,
		Command: `{}`, Input: `{}`, Config: `{}`, Context: `{}`, Metadata: `{}`,
		StreamMode: `[]`, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func newCanonicalRepositoryCheckpoint(id, threadID, runID int64, runtimeType string, createdAt int64) *entity.Checkpoint {
	return &entity.Checkpoint{
		ID: id, ThreadID: threadID, RunID: runID, RuntimeType: runtimeType,
		RuntimeKey: "runtime", ChannelValues: `{}`, ChannelVersions: `{}`,
		PendingSends: `[]`, Metadata: `{}`, CreatedAt: createdAt,
	}
}

func threadIDs(threads []*entity.Thread) []int64 {
	ids := make([]int64, 0, len(threads))
	for _, thread := range threads {
		ids = append(ids, thread.ID)
	}
	return ids
}

func runIDs(runs []*entity.Run) []int64 {
	ids := make([]int64, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

func messageIDs(messages []*entity.Message) []int64 {
	ids := make([]int64, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func runEventIDs(events []*entity.RunEvent) []int64 {
	ids := make([]int64, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.ID)
	}
	return ids
}

func checkpointIDs(checkpoints []*entity.Checkpoint) []int64 {
	ids := make([]int64, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		ids = append(ids, checkpoint.ID)
	}
	return ids
}

func checkpointRuntimeTypes(checkpoints []*entity.Checkpoint) []string {
	runtimeTypes := make([]string, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		runtimeTypes = append(runtimeTypes, checkpoint.RuntimeType)
	}
	return runtimeTypes
}

func canonicalAscendingIDs(first, last int64) []int64 {
	ids := make([]int64, 0, last-first+1)
	for id := first; id <= last; id++ {
		ids = append(ids, id)
	}
	return ids
}

func canonicalDescendingIDs(first, last int64) []int64 {
	ids := make([]int64, 0, first-last+1)
	for id := first; id >= last; id-- {
		ids = append(ids, id)
	}
	return ids
}
