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
	"strings"
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

func TestThreadRepositoryUpdateThreadTitle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&threadPO{}))

	repo := NewThreadRepository(db)
	require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
		ID:            1,
		SpaceID:       10,
		CreatorID:     20,
		Title:         "请生成一份《武汉3日游攻略》正式文档",
		Status:        entity.ThreadStatusIdle,
		Source:        entity.ThreadSourceWeb,
		CreatedAt:     1,
		UpdatedAt:     2,
		LastMessageAt: 3,
	}))

	updated, ok, err := repo.UpdateThreadTitle(context.Background(), UpdateThreadTitleRequest{
		ThreadID:  1,
		Title:     "武汉3日游攻略",
		UpdatedAt: 100,
	})

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "武汉3日游攻略", updated.Title)
	require.Equal(t, int64(100), updated.UpdatedAt)
	require.Equal(t, int64(3), updated.LastMessageAt)
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

func TestMemorySearchLikeConditionUsesMySQLSafeEscape(t *testing.T) {
	condition := memorySearchLikeCondition()

	require.Contains(t, condition, "ESCAPE '!'")
	require.NotContains(t, condition, `ESCAPE '\'`)
	require.Equal(t, `100!%!_ready!! \ path`, escapeSQLLike(`100%_ready! \ path`))
}

func TestRuntimeFileRepositoryUpsertsByRunAndVirtualPath(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentFilePO{}))

	repo := NewRuntimeFileRepository(db)
	first := &entity.AgentFile{
		ID:          1,
		SpaceID:     30,
		UserID:      40,
		ThreadID:    10,
		RunID:       20,
		FileName:    "first.txt",
		FileKind:    entity.AgentFileKindWorkspace,
		VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/file.txt",
		ObjectURI:   "agent-runtime/30/10/runs/20/tool-results/trunc/file.txt",
		ContentType: "text/plain; charset=utf-8",
		SizeBytes:   10,
		Digest:      strings.Repeat("a", 64),
		Status:      entity.AgentFileStatusActive,
		Metadata:    `{"version":1}`,
		CreatedAt:   100,
		UpdatedAt:   100,
	}

	stored, created, err := repo.UpsertRuntimeFile(
		context.Background(),
		first,
	)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(1), stored.ID)

	second := *first
	second.ID = 2
	second.FileName = "second.txt"
	second.ObjectURI = "agent-runtime/30/10/runs/20/tool-results/trunc/file-v2.txt"
	second.SizeBytes = 20
	second.Digest = strings.Repeat("b", 64)
	second.Metadata = `{"version":2}`
	second.UpdatedAt = 200

	stored, created, err = repo.UpsertRuntimeFile(
		context.Background(),
		&second,
	)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(1), stored.ID)
	require.Equal(t, "second.txt", stored.FileName)
	require.Equal(t, int64(20), stored.SizeBytes)
	require.Equal(t, strings.Repeat("b", 64), stored.Digest)
	require.Equal(t, `{"version":2}`, stored.Metadata)
	require.Equal(t, int64(100), stored.CreatedAt)
	require.Equal(t, int64(200), stored.UpdatedAt)

	stored, err = repo.GetRuntimeFile(
		context.Background(),
		first.RunID,
		first.VirtualPath,
	)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, int64(1), stored.ID)
	require.Equal(t, second.ObjectURI, stored.ObjectURI)

	stored, err = repo.GetFileByID(context.Background(), first.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, first.VirtualPath, stored.VirtualPath)

	var count int64
	require.NoError(t, db.Model(&agentFilePO{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var storedPO agentFilePO
	require.NoError(t, db.Where("id = ?", first.ID).First(&storedPO).Error)
	require.Equal(t, agentFileVirtualPathHash(first.VirtualPath), storedPO.VirtualPathHash)
}

func TestArtifactRepositoryUpsertsByFileAndListsByThread(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}))

	repo := NewArtifactRepository(db)
	first := &entity.AgentArtifact{
		ID:           100,
		SpaceID:      30,
		UserID:       40,
		ThreadID:     10,
		RunID:        20,
		FileID:       90,
		Title:        "Initial report",
		ArtifactType: "report",
		VirtualPath:  "/mnt/user-data/outputs/report.txt",
		ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
		ContentType:  "text/plain; charset=utf-8",
		SizeBytes:    128,
		PreviewMode:  entity.AgentArtifactPreviewModeText,
		Metadata:     `{"source":"present_files"}`,
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}

	stored, created, err := repo.UpsertArtifact(context.Background(), first)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(100), stored.ID)

	second := *first
	second.ID = 101
	second.Title = "Updated report"
	second.PreviewMode = entity.AgentArtifactPreviewModeDownload
	second.UpdatedAt = 1100
	stored, created, err = repo.UpsertArtifact(context.Background(), &second)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(100), stored.ID)
	require.Equal(t, "Updated report", stored.Title)
	require.Equal(t, entity.AgentArtifactPreviewModeDownload, stored.PreviewMode)
	require.Equal(t, int64(1000), stored.CreatedAt)
	require.Equal(t, int64(1100), stored.UpdatedAt)

	other := *first
	other.ID = 102
	other.FileID = 91
	other.RunID = 21
	other.Title = "Other run report"
	other.CreatedAt = 1200
	other.UpdatedAt = 1200
	_, created, err = repo.UpsertArtifact(context.Background(), &other)
	require.NoError(t, err)
	require.True(t, created)

	artifacts, total, err := repo.ListArtifacts(context.Background(), ListArtifactsRequest{
		ThreadID: 10,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, artifacts, 2)
	require.Equal(t, []int64{102, 100}, []int64{artifacts[0].ID, artifacts[1].ID})

	runID := int64(20)
	artifacts, total, err = repo.ListArtifacts(context.Background(), ListArtifactsRequest{
		ThreadID: 10,
		RunID:    &runID,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, artifacts, 1)
	require.Equal(t, int64(100), artifacts[0].ID)

	got, err := repo.GetArtifact(context.Background(), 10, 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(100), got.ID)
	require.Equal(t, "Updated report", got.Title)

	got, err = repo.GetArtifact(context.Background(), 11, 100)
	require.NoError(t, err)
	require.Nil(t, got)

	deleted, ok, err := repo.DeleteArtifact(context.Background(), 10, 100, 1300)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, deleted)
	require.Equal(t, int64(100), deleted.ID)
	require.Equal(t, int64(1300), deleted.DeletedAt)

	got, err = repo.GetArtifact(context.Background(), 10, 100)
	require.NoError(t, err)
	require.Nil(t, got)

	artifacts, total, err = repo.ListArtifacts(context.Background(), ListArtifactsRequest{
		ThreadID: 10,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, artifacts, 1)
	require.Equal(t, int64(102), artifacts[0].ID)

	deletedArtifacts, deletedTotal, err := repo.ListArtifacts(context.Background(), ListArtifactsRequest{
		ThreadID:    10,
		DeletedOnly: true,
		Page:        1,
		PageSize:    10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), deletedTotal)
	require.Len(t, deletedArtifacts, 1)
	require.Equal(t, int64(100), deletedArtifacts[0].ID)
	require.Equal(t, int64(1300), deletedArtifacts[0].DeletedAt)

	deleted, ok, err = repo.DeleteArtifact(context.Background(), 10, 100, 1400)
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, deleted)

	restored, ok, err := repo.RestoreArtifact(context.Background(), 10, 100, 1500)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, restored)
	require.Equal(t, int64(100), restored.ID)
	require.Equal(t, int64(0), restored.DeletedAt)
	require.Equal(t, int64(1500), restored.UpdatedAt)

	got, err = repo.GetArtifact(context.Background(), 10, 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(100), got.ID)
	require.Equal(t, int64(0), got.DeletedAt)

	artifacts, total, err = repo.ListArtifacts(context.Background(), ListArtifactsRequest{
		ThreadID: 10,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, artifacts, 2)
	require.Equal(t, []int64{102, 100}, []int64{artifacts[0].ID, artifacts[1].ID})

	deletedArtifacts, deletedTotal, err = repo.ListArtifacts(context.Background(), ListArtifactsRequest{
		ThreadID:    10,
		DeletedOnly: true,
		Page:        1,
		PageSize:    10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), deletedTotal)
	require.Empty(t, deletedArtifacts)

	restored, ok, err = repo.RestoreArtifact(context.Background(), 10, 100, 1600)
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, restored)

	var count int64
	require.NoError(t, db.Model(&agentArtifactPO{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestArtifactRepositoryListDeletedCleanupCandidates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentFilePO{}, &agentArtifactPO{}))

	repo := NewArtifactRepository(db)
	files := []*agentFilePO{
		{
			ID:       90,
			SpaceID:  30,
			ThreadID: 10,
			RunID:    20,
			FileKind: string(entity.AgentFileKindWorkspace),
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" +
				strings.Repeat("a", 64) + ".txt",
			ObjectURI: "agent-runtime/30/10/runs/20/tool-results/trunc/" +
				strings.Repeat("a", 64) + ".txt",
			ContentType: "text/plain",
			Status:      string(entity.AgentFileStatusActive),
			CreatedAt:   1000,
			UpdatedAt:   1000,
		},
		{
			ID:       91,
			SpaceID:  30,
			ThreadID: 10,
			RunID:    20,
			FileKind: string(entity.AgentFileKindWorkspace),
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" +
				strings.Repeat("b", 64) + ".txt",
			ObjectURI: "agent-runtime/30/10/runs/20/tool-results/trunc/" +
				strings.Repeat("b", 64) + ".txt",
			ContentType: "text/plain",
			Status:      string(entity.AgentFileStatusActive),
			CreatedAt:   1000,
			UpdatedAt:   1000,
		},
		{
			ID:       92,
			SpaceID:  30,
			ThreadID: 10,
			RunID:    20,
			FileKind: string(entity.AgentFileKindWorkspace),
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" +
				strings.Repeat("c", 64) + ".txt",
			ObjectURI: "agent-runtime/30/10/runs/20/tool-results/trunc/" +
				strings.Repeat("c", 64) + ".txt",
			ContentType: "text/plain",
			Status:      string(entity.AgentFileStatusDeleted),
			CreatedAt:   1000,
			UpdatedAt:   1000,
		},
		{
			ID:       93,
			SpaceID:  30,
			ThreadID: 10,
			RunID:    20,
			FileKind: string(entity.AgentFileKindWorkspace),
			VirtualPath: "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" +
				strings.Repeat("d", 64) + ".txt",
			ObjectURI: "agent-runtime/30/10/runs/20/tool-results/trunc/" +
				strings.Repeat("d", 64) + ".txt",
			ContentType: "text/plain",
			Status:      string(entity.AgentFileStatusActive),
			CreatedAt:   1000,
			UpdatedAt:   1000,
		},
	}
	for _, file := range files {
		file.VirtualPathHash = agentFileVirtualPathHash(file.VirtualPath)
	}
	require.NoError(t, db.Create(&files).Error)
	artifacts := []*agentArtifactPO{
		{
			ID:           100,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			Title:        "active.txt",
			ArtifactType: "report",
			ObjectURI:    files[0].ObjectURI,
			ContentType:  "text/plain",
			PreviewMode:  string(entity.AgentArtifactPreviewModeText),
			DeletedAt:    0,
			CreatedAt:    1000,
			UpdatedAt:    1000,
		},
		{
			ID:           101,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       91,
			Title:        "recent.txt",
			ArtifactType: "report",
			ObjectURI:    files[1].ObjectURI,
			ContentType:  "text/plain",
			PreviewMode:  string(entity.AgentArtifactPreviewModeText),
			DeletedAt:    1900,
			CreatedAt:    1000,
			UpdatedAt:    1900,
		},
		{
			ID:           102,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       92,
			Title:        "already-cleaned.txt",
			ArtifactType: "report",
			ObjectURI:    files[2].ObjectURI,
			ContentType:  "text/plain",
			PreviewMode:  string(entity.AgentArtifactPreviewModeText),
			DeletedAt:    1200,
			CreatedAt:    1000,
			UpdatedAt:    1200,
		},
		{
			ID:           103,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       93,
			Title:        "expired.txt",
			ArtifactType: "report",
			ObjectURI:    files[3].ObjectURI,
			ContentType:  "text/plain",
			PreviewMode:  string(entity.AgentArtifactPreviewModeText),
			DeletedAt:    1100,
			CreatedAt:    1000,
			UpdatedAt:    1100,
		},
	}
	require.NoError(t, db.Create(&artifacts).Error)

	got, err := repo.ListDeletedArtifactCleanupCandidates(
		context.Background(),
		ListDeletedArtifactCleanupCandidatesRequest{
			CutoffDeletedAt: 1500,
			Limit:           10,
		},
	)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(103), got[0].ID)
	require.Equal(t, int64(93), got[0].FileID)

	marked, err := repo.MarkArtifactFileDeleted(
		context.Background(),
		MarkArtifactFileDeletedRequest{
			FileID:    93,
			ObjectURI: files[3].ObjectURI,
			DeletedAt: 2000,
		},
	)
	require.NoError(t, err)
	require.True(t, marked)

	var stored agentFilePO
	require.NoError(t, db.Where("id = ?", int64(93)).First(&stored).Error)
	require.Equal(t, string(entity.AgentFileStatusDeleted), stored.Status)
	require.Equal(t, int64(2000), stored.UpdatedAt)

	marked, err = repo.MarkArtifactFileDeleted(
		context.Background(),
		MarkArtifactFileDeletedRequest{
			FileID:    93,
			ObjectURI: "wrong-object-uri",
			DeletedAt: 2100,
		},
	)
	require.NoError(t, err)
	require.False(t, marked)
}

func TestArtifactRepositoryUpdatesActiveArtifactScanMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}))

	repo := NewArtifactRepository(db)
	artifact := &entity.AgentArtifact{
		ID:           100,
		SpaceID:      30,
		UserID:       40,
		ThreadID:     10,
		RunID:        20,
		FileID:       90,
		Title:        "Initial report",
		ArtifactType: "report",
		VirtualPath:  "/mnt/user-data/outputs/report.txt",
		ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
		ContentType:  "text/plain; charset=utf-8",
		SizeBytes:    128,
		PreviewMode:  entity.AgentArtifactPreviewModeText,
		Metadata:     `{"source":"present_files","scan_status":"pending"}`,
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	_, created, err := repo.UpsertArtifact(context.Background(), artifact)
	require.NoError(t, err)
	require.True(t, created)

	updated, ok, err := repo.UpdateArtifactScanMetadata(
		context.Background(),
		10,
		100,
		`{"source":"present_files","scan_status":"clean","scan_scanned_at":2000}`,
		2000,
	)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, updated)
	require.Equal(t, int64(100), updated.ID)
	require.Equal(t, int64(2000), updated.UpdatedAt)
	require.JSONEq(
		t,
		`{"source":"present_files","scan_status":"clean","scan_scanned_at":2000}`,
		updated.Metadata,
	)

	deleted, ok, err := repo.DeleteArtifact(context.Background(), 10, 100, 2100)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, deleted)

	updated, ok, err = repo.UpdateArtifactScanMetadata(
		context.Background(),
		10,
		100,
		`{"scan_status":"blocked","scan_scanned_at":2200}`,
		2200,
	)
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, updated)
}

func TestPlanRepositoryReservesHighWatermarkWithCompareAndSwap(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentRunPlanPO{}))

	repo := NewPlanRepository(db)
	plan := &entity.AgentRunPlan{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		UserID:    40,
		CreatedAt: 100,
		UpdatedAt: 100,
	}

	stored, err := repo.ReservePlanTaskID(
		context.Background(),
		plan,
		0,
		1,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), stored.HighWatermark)
	require.Equal(t, int64(1), stored.Revision)

	_, err = repo.ReservePlanTaskID(
		context.Background(),
		plan,
		0,
		1,
	)
	require.ErrorIs(t, err, ErrPlanReservationConflict)

	stored, err = repo.ReservePlanTaskID(
		context.Background(),
		plan,
		1,
		2,
	)
	require.NoError(t, err)
	require.Equal(t, int64(2), stored.HighWatermark)
	require.Equal(t, int64(2), stored.Revision)
}

func TestPlanRepositoryUpsertsAndArchivesCompletedItem(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentRunPlanPO{}, &agentRunPlanItemPO{}))

	repo := NewPlanRepository(db)
	plan := &entity.AgentRunPlan{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		UserID:    40,
		CreatedAt: 100,
		UpdatedAt: 100,
	}
	_, err = repo.ReservePlanTaskID(context.Background(), plan, 0, 1)
	require.NoError(t, err)

	item := &entity.AgentRunPlanItem{
		ID:          100,
		RunID:       20,
		TaskID:      1,
		Subject:     "Run tests",
		Description: "Run the focused tests.",
		Status:      entity.AgentRunPlanItemStatusPending,
		Blocks:      `[]`,
		BlockedBy:   `[]`,
		Metadata:    `{"source":"agent"}`,
		Active:      true,
		Version:     1,
		CreatedAt:   110,
		UpdatedAt:   110,
	}
	stored, previous, storedPlan, created, err := repo.UpsertPlanItem(
		context.Background(),
		item,
	)
	require.NoError(t, err)
	require.True(t, created)
	require.Nil(t, previous)
	require.Equal(t, int64(100), stored.ID)
	require.Equal(t, int64(2), storedPlan.Revision)

	completed := *item
	completed.ID = 101
	completed.Status = entity.AgentRunPlanItemStatusCompleted
	completed.UpdatedAt = 120
	stored, previous, storedPlan, created, err = repo.UpsertPlanItem(
		context.Background(),
		&completed,
	)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, entity.AgentRunPlanItemStatusPending, previous.Status)
	require.Equal(t, int64(100), stored.ID)
	require.Equal(t, int64(2), stored.Version)
	require.Equal(t, int64(3), storedPlan.Revision)

	archived, previous, storedPlan, err := repo.ArchivePlanItem(
		context.Background(),
		20,
		1,
		130,
	)
	require.NoError(t, err)
	require.Equal(t, entity.AgentRunPlanItemStatusCompleted, previous.Status)
	require.Equal(t, entity.AgentRunPlanItemStatusCompleted, archived.Status)
	require.False(t, archived.Active)
	require.Equal(t, int64(3), archived.Version)
	require.Equal(t, int64(4), storedPlan.Revision)

	active, err := repo.ListPlanItems(context.Background(), 20, true)
	require.NoError(t, err)
	require.Empty(t, active)

	history, err := repo.ListPlanItems(context.Background(), 20, false)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, entity.AgentRunPlanItemStatusCompleted, history[0].Status)
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
	require.Equal(t, int64(0), got.ParentRunID)
	require.Equal(t, entity.RunKindTask, got.RunKind)
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

func TestThreadRepositoryCreateAndGetSubagentRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	run := newRepositoryTestRun(101, 10, entity.RunStatusRunning, 33)
	run.ParentRunID = 100
	run.RunKind = entity.RunKindSubagent
	run.AssistantID = "singleagent:1001"

	require.NoError(t, repo.CreateRun(context.Background(), run))
	got, err := repo.GetRun(context.Background(), 101)

	require.NoError(t, err)
	require.Equal(t, int64(100), got.ParentRunID)
	require.Equal(t, entity.RunKindSubagent, got.RunKind)
	require.Equal(t, "singleagent:1001", got.AssistantID)
}

func TestThreadRepositoryGetRunByIdempotencyKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	require.NoError(t, repo.CreateRun(context.Background(), &entity.Run{
		ID:             100,
		ThreadID:       10,
		SpaceID:        1,
		CreatorID:      2,
		AssistantID:    "default",
		Status:         entity.RunStatusQueued,
		Command:        `{}`,
		Input:          `{"messages":[]}`,
		Config:         `{}`,
		Context:        `{}`,
		Metadata:       `{}`,
		StreamMode:     `["messages","updates"]`,
		IdempotencyKey: "idem-1",
		CreatedAt:      33,
		UpdatedAt:      44,
	}))
	require.NoError(t, repo.CreateRun(context.Background(), &entity.Run{
		ID:             101,
		ThreadID:       11,
		SpaceID:        2,
		CreatorID:      2,
		AssistantID:    "default",
		Status:         entity.RunStatusQueued,
		Command:        `{}`,
		Input:          `{"messages":[]}`,
		Config:         `{}`,
		Context:        `{}`,
		Metadata:       `{}`,
		StreamMode:     `["messages","updates"]`,
		IdempotencyKey: "idem-1",
		CreatedAt:      55,
		UpdatedAt:      66,
	}))

	got, err := repo.GetRunByIdempotencyKey(context.Background(), 1, "idem-1")

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(100), got.ID)
	require.Equal(t, int64(1), got.SpaceID)

	missing, err := repo.GetRunByIdempotencyKey(context.Background(), 1, "missing")
	require.NoError(t, err)
	require.Nil(t, missing)
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

func TestThreadRepositoryListRunsDefaultsToTopLevelAndCanListSubagents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	parent := newRepositoryTestRun(1, 10, entity.RunStatusRunning, 1)
	child := newRepositoryTestRun(2, 10, entity.RunStatusRunning, 2)
	child.ParentRunID = 1
	child.RunKind = entity.RunKindSubagent
	otherChild := newRepositoryTestRun(3, 10, entity.RunStatusRunning, 3)
	otherChild.ParentRunID = 99
	otherChild.RunKind = entity.RunKindSubagent
	require.NoError(t, repo.CreateRun(context.Background(), parent))
	require.NoError(t, repo.CreateRun(context.Background(), child))
	require.NoError(t, repo.CreateRun(context.Background(), otherChild))

	topLevel, total, err := repo.ListRuns(context.Background(), ListRunsRequest{
		ThreadID: 10,
		Page:     1,
		PageSize: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, topLevel, 1)
	require.Equal(t, int64(1), topLevel[0].ID)

	parentRunID := int64(1)
	children, total, err := repo.ListRuns(context.Background(), ListRunsRequest{
		ThreadID:    10,
		ParentRunID: &parentRunID,
		Page:        1,
		PageSize:    10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, children, 1)
	require.Equal(t, int64(2), children[0].ID)
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
	require.Equal(t, int64(1), threadEvents[0].ID)
	require.Equal(t, int64(2), threadEvents[1].ID)
	require.Equal(t, int64(3), threadEvents[2].ID)
}

func TestThreadRepositoryListRunEventsUsesCursorStableIDOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runEventPO{}))

	repo := NewThreadRepository(db)
	for _, event := range []*entity.RunEvent{
		{ID: 1, ThreadID: 10, RunID: 20, EventType: "run.started", Payload: `{}`, CreatedAt: 200},
		{ID: 2, ThreadID: 10, RunID: 20, EventType: "step.started", Payload: `{}`, CreatedAt: 100},
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
	require.Equal(t, int64(2), got[1].ID)
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

func TestThreadRepositoryRuntimeCheckpointLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&checkpointPO{}))

	repo := NewThreadRepository(db)
	for _, checkpoint := range []*entity.Checkpoint{
		{
			ID:              1,
			ThreadID:        10,
			RunID:           20,
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues:   `{"checkpoint":"old"}`,
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
			CreatedAt:       100,
		},
		{
			ID:              2,
			ThreadID:        10,
			RunID:           20,
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues:   `{"checkpoint":"new"}`,
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
			CreatedAt:       200,
		},
		{
			ID:              3,
			ThreadID:        10,
			RunID:           20,
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-2",
			EnvelopeVersion: 1,
			ChannelValues:   `{"checkpoint":"other"}`,
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
			CreatedAt:       300,
		},
	} {
		require.NoError(t, repo.CreateCheckpoint(context.Background(), checkpoint))
	}

	latest, err := repo.GetLatestRuntimeCheckpoint(
		context.Background(),
		10,
		20,
		"eino_adk",
		"checkpoint-1",
	)
	require.NoError(t, err)
	require.Equal(t, int64(2), latest.ID)
	require.Equal(t, int32(1), latest.EnvelopeVersion)

	require.NoError(t, repo.DeleteRuntimeCheckpoint(
		context.Background(),
		10,
		20,
		"eino_adk",
		"checkpoint-1",
		500,
	))

	latest, err = repo.GetLatestRuntimeCheckpoint(
		context.Background(),
		10,
		20,
		"eino_adk",
		"checkpoint-1",
	)
	require.NoError(t, err)
	require.Nil(t, latest)

	other, err := repo.GetLatestRuntimeCheckpoint(
		context.Background(),
		10,
		20,
		"eino_adk",
		"checkpoint-2",
	)
	require.NoError(t, err)
	require.Equal(t, int64(3), other.ID)

	var deletedRows []checkpointPO
	require.NoError(t, db.Where("runtime_key = ?", "checkpoint-1").Order("id").Find(&deletedRows).Error)
	require.Len(t, deletedRows, 2)
	require.Equal(t, int64(500), deletedRows[0].RuntimeDeletedAt)
	require.Equal(t, int64(500), deletedRows[1].RuntimeDeletedAt)

	require.NoError(t, repo.CreateCheckpoint(context.Background(), &entity.Checkpoint{
		ID:              4,
		ThreadID:        10,
		RunID:           20,
		RuntimeType:     "eino_adk",
		RuntimeKey:      "checkpoint-1",
		EnvelopeVersion: 1,
		ChannelValues:   `{"checkpoint":"resumed"}`,
		ChannelVersions: `{}`,
		PendingSends:    `[]`,
		Metadata:        `{"runtime":"eino_adk"}`,
		CreatedAt:       600,
	}))

	latest, err = repo.GetLatestRuntimeCheckpoint(
		context.Background(),
		10,
		20,
		"eino_adk",
		"checkpoint-1",
	)
	require.NoError(t, err)
	require.Equal(t, int64(4), latest.ID)
	require.Zero(t, latest.RuntimeDeletedAt)
}

func TestThreadRepositoryCreateAndListMemories(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&memoryPO{}, &memoryAuditEventPO{}))

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
			ID:                   6,
			ThreadID:             10,
			RunID:                0,
			SpaceID:              1,
			Scope:                entity.MemoryScopeLongTerm,
			Content:              "newer same score",
			Metadata:             `{}`,
			Score:                0.7,
			Confidence:           0.82,
			SourceType:           "transcript_summary",
			SourceID:             "snapshot-100",
			CorrectionOfMemoryID: 1,
			CorrectedAt:          650,
			CreatedAt:            600,
			UpdatedAt:            600,
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
	require.Equal(t, 0.82, memories[1].Confidence)
	require.Equal(t, "transcript_summary", memories[1].SourceType)
	require.Equal(t, "snapshot-100", memories[1].SourceID)
	require.Equal(t, int64(1), memories[1].CorrectionOfMemoryID)
	require.Equal(t, int64(650), memories[1].CorrectedAt)
	require.Equal(t, `{"source":"profile"}`, memories[2].Metadata)
	require.Equal(t, []int64{2, 6, 1}, []int64{memories[0].ID, memories[1].ID, memories[2].ID})
}

func TestThreadRepositoryCreateOrGetMemoryBySourceIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&memoryPO{}, &memoryAuditEventPO{}))

	repo := NewThreadRepository(db)
	first := &entity.Memory{
		ID:         10,
		ThreadID:   100,
		SpaceID:    1,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "用户偏好中文回答",
		Metadata:   `{}`,
		Score:      0.8,
		Confidence: 0.9,
		SourceType: "transcript_summary",
		SourceID:   "snapshot:501:language",
		CreatedAt:  1000,
		UpdatedAt:  1000,
	}
	got, created, err := repo.CreateOrGetMemoryBySource(context.Background(), first)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(10), got.ID)

	second := &entity.Memory{
		ID:         11,
		ThreadID:   100,
		SpaceID:    1,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "重复事实不应创建",
		Metadata:   `{}`,
		Score:      1,
		Confidence: 1,
		SourceType: "transcript_summary",
		SourceID:   "snapshot:501:language",
		CreatedAt:  2000,
		UpdatedAt:  2000,
	}
	got, created, err = repo.CreateOrGetMemoryBySource(context.Background(), second)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(10), got.ID)
	require.Equal(t, "用户偏好中文回答", got.Content)

	memories, total, err := repo.ListMemories(context.Background(), ListMemoriesRequest{
		ThreadID: 100,
		Now:      3000,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, memories, 1)
}

func TestThreadRepositoryManagesMemoriesWithSearchAndSoftDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&memoryPO{}, &memoryAuditEventPO{}))

	repo := NewThreadRepository(db)
	for _, memory := range []*entity.Memory{
		{
			ID:         1,
			ThreadID:   10,
			SpaceID:    1,
			Scope:      entity.MemoryScopeLongTerm,
			Content:    "用户偏好中文回答",
			Metadata:   `{"source":"manual"}`,
			Score:      0.8,
			Confidence: 0.9,
			SourceType: "manual",
			SourceID:   "memory-1",
			CreatedAt:  100,
			UpdatedAt:  100,
		},
		{
			ID:         2,
			ThreadID:   10,
			RunID:      20,
			SpaceID:    1,
			Scope:      entity.MemoryScopeRun,
			Content:    "本次运行使用中文总结",
			Metadata:   `{}`,
			Score:      0.7,
			Confidence: 0.8,
			CreatedAt:  200,
			UpdatedAt:  200,
		},
		{
			ID:        3,
			ThreadID:  10,
			SpaceID:   1,
			Scope:     entity.MemoryScopeThread,
			Content:   "expired 中文 memory",
			Metadata:  `{}`,
			ExpiresAt: 250,
			CreatedAt: 300,
			UpdatedAt: 300,
		},
		{
			ID:        4,
			ThreadID:  11,
			SpaceID:   1,
			Scope:     entity.MemoryScopeLongTerm,
			Content:   "其他线程中文记忆",
			Metadata:  `{}`,
			CreatedAt: 400,
			UpdatedAt: 400,
		},
	} {
		require.NoError(t, repo.CreateMemory(context.Background(), memory))
	}

	updated, ok, err := repo.UpdateMemory(context.Background(), UpdateMemoryRequest{
		ThreadID:   10,
		MemoryID:   1,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "用户偏好简短中文回答",
		Metadata:   `{"source":"edited"}`,
		Score:      0.88,
		Confidence: 0.95,
		SourceType: "manual",
		SourceID:   "memory-1-edited",
		UpdatedAt:  900,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "用户偏好简短中文回答", updated.Content)
	require.Equal(t, 0.95, updated.Confidence)
	require.Equal(t, "memory-1-edited", updated.SourceID)
	require.Equal(t, int64(900), updated.UpdatedAt)

	deleted, err := repo.DeleteMemory(context.Background(), DeleteMemoryRequest{
		ThreadID:  10,
		MemoryID:  2,
		DeletedAt: 950,
	})
	require.NoError(t, err)
	require.True(t, deleted)

	memories, total, err := repo.ListMemories(context.Background(), ListMemoriesRequest{
		ThreadID: 10,
		Query:    "中文",
		Page:     1,
		PageSize: 10,
		Now:      1000,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, memories, 1)
	require.Equal(t, int64(1), memories[0].ID)
	require.Zero(t, memories[0].DeletedAt)

	cleared, err := repo.ClearMemories(context.Background(), ClearMemoriesRequest{
		ThreadID:  10,
		Scopes:    []entity.MemoryScope{entity.MemoryScopeLongTerm},
		DeletedAt: 1100,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), cleared)

	memories, total, err = repo.ListMemories(context.Background(), ListMemoriesRequest{
		ThreadID:       10,
		Query:          "中文",
		IncludeExpired: true,
		IncludeDeleted: true,
		Page:           1,
		PageSize:       10,
		Now:            1200,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, memories, 3)
	require.Equal(t, int64(1100), memories[0].DeletedAt)
	require.Equal(t, int64(950), memories[1].DeletedAt)
	require.Zero(t, memories[2].DeletedAt)

	restored, ok, err := repo.RestoreMemory(context.Background(), RestoreMemoryRequest{
		ThreadID:   10,
		MemoryID:   2,
		RestoredAt: 1300,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, restored)
	require.Equal(t, int64(0), restored.DeletedAt)
	require.Equal(t, int64(1300), restored.UpdatedAt)

	require.NoError(t, repo.CreateMemoryAuditEvent(context.Background(), &entity.MemoryAuditEvent{
		ID:            5001,
		ThreadID:      10,
		RunID:         20,
		SpaceID:       1,
		MemoryID:      2,
		ActorID:       7,
		EventType:     "memory.restored",
		Scope:         entity.MemoryScopeRun,
		SourceType:    "transcript_summary",
		SourceID:      strings.Repeat("x", 160),
		AffectedCount: 1,
		CreatedAt:     1400,
	}))
	audits, auditTotal, err := repo.ListMemoryAuditEvents(context.Background(), ListMemoryAuditEventsRequest{
		ThreadID: 10,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), auditTotal)
	require.Len(t, audits, 1)
	require.Equal(t, "memory.restored", audits[0].EventType)
	require.Equal(t, int64(2), audits[0].MemoryID)
	require.Equal(t, int64(1), audits[0].AffectedCount)
	require.LessOrEqual(t, len([]byte(audits[0].SourceID)), 128)
}

func TestThreadRepositoryCreateOrGetTranscriptSnapshotIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&transcriptSnapshotPO{}))

	repo := NewThreadRepository(db)
	first := &entity.TranscriptSnapshot{
		ID:             1001,
		ThreadID:       10,
		RunID:          20,
		SpaceID:        30,
		Kind:           entity.TranscriptKindSummaryInput,
		Digest:         strings.Repeat("a", 64),
		IdempotencyKey: "summary_input:" + strings.Repeat("a", 64),
		MessageCount:   3,
		Messages:       `[{"role":"user","content":"hello"},{"role":"assistant","content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"search","arguments":"{}"}}]},{"role":"tool","content":"result","tool_call_id":"call-1","tool_name":"search"}]`,
		Metadata:       `{"runtime":"eino_adk"}`,
		CreatedAt:      100,
	}

	got, created, err := repo.CreateOrGetTranscriptSnapshot(
		context.Background(),
		first,
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, first, got)

	duplicate := *first
	duplicate.ID = 1002
	duplicate.CreatedAt = 200
	got, created, err = repo.CreateOrGetTranscriptSnapshot(
		context.Background(),
		&duplicate,
	)

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(1001), got.ID)
	require.Equal(t, int64(100), got.CreatedAt)
	require.JSONEq(t, first.Messages, got.Messages)
	require.JSONEq(t, first.Metadata, got.Metadata)

	got, err = repo.GetTranscriptSnapshot(context.Background(), first.ID)
	require.NoError(t, err)
	require.Equal(t, first, got)

	_, err = repo.GetTranscriptSnapshot(context.Background(), 9999)
	require.Error(t, err)
}

func TestThreadRepositoryCreateOrGetMemoryFlushJobIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&memoryFlushJobPO{}))

	repo := NewThreadRepository(db)
	first := &entity.MemoryFlushJob{
		ID:                   2001,
		ThreadID:             10,
		RunID:                20,
		SpaceID:              30,
		UserID:               40,
		AssistantID:          "lead",
		TranscriptSnapshotID: 1001,
		IdempotencyKey:       "summary_input:" + strings.Repeat("a", 64),
		Status:               entity.MemoryFlushJobStatusPending,
		AvailableAt:          100,
		CreatedAt:            100,
		UpdatedAt:            100,
	}

	got, created, err := repo.CreateOrGetMemoryFlushJob(
		context.Background(),
		first,
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, first, got)

	duplicate := *first
	duplicate.ID = 2002
	duplicate.CreatedAt = 200
	duplicate.UpdatedAt = 200
	got, created, err = repo.CreateOrGetMemoryFlushJob(
		context.Background(),
		&duplicate,
	)

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(2001), got.ID)
	require.Equal(t, int64(1001), got.TranscriptSnapshotID)
	require.Equal(t, int64(100), got.CreatedAt)
}

func TestThreadRepositoryClaimMemoryFlushJobsMarksDuePendingProcessing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&memoryFlushJobPO{}))

	repo := NewThreadRepository(db)
	jobs := []*entity.MemoryFlushJob{
		{
			ID:             2101,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			AssistantID:    "lead",
			IdempotencyKey: "summary_input:" + strings.Repeat("a", 64),
			Status:         entity.MemoryFlushJobStatusPending,
			AvailableAt:    100,
			CreatedAt:      100,
			UpdatedAt:      100,
		},
		{
			ID:             2102,
			ThreadID:       10,
			RunID:          21,
			SpaceID:        30,
			UserID:         40,
			AssistantID:    "lead",
			IdempotencyKey: "summary_input:" + strings.Repeat("b", 64),
			Status:         entity.MemoryFlushJobStatusPending,
			AvailableAt:    500,
			CreatedAt:      500,
			UpdatedAt:      500,
		},
		{
			ID:             2103,
			ThreadID:       10,
			RunID:          22,
			SpaceID:        30,
			UserID:         40,
			AssistantID:    "lead",
			IdempotencyKey: "summary_input:" + strings.Repeat("c", 64),
			Status:         entity.MemoryFlushJobStatusProcessing,
			AttemptCount:   2,
			WorkerID:       "dead-worker",
			LeaseExpiresAt: 150,
			AvailableAt:    100,
			CreatedAt:      120,
			UpdatedAt:      120,
		},
	}
	for _, job := range jobs {
		_, created, err := repo.CreateOrGetMemoryFlushJob(context.Background(), job)
		require.NoError(t, err)
		require.True(t, created)
	}

	claimed, err := repo.ClaimMemoryFlushJobs(context.Background(), ClaimMemoryFlushJobsRequest{
		WorkerID:       " worker-a ",
		Limit:          2,
		Now:            200,
		LeaseExpiresAt: 900,
	})

	require.NoError(t, err)
	require.Len(t, claimed, 2)
	require.Equal(t, []int64{2101, 2103}, []int64{claimed[0].ID, claimed[1].ID})
	require.Equal(t, entity.MemoryFlushJobStatusProcessing, claimed[0].Status)
	require.Equal(t, "worker-a", claimed[0].WorkerID)
	require.Equal(t, int32(1), claimed[0].AttemptCount)
	require.Equal(t, int64(900), claimed[0].LeaseExpiresAt)
	require.Equal(t, int64(200), claimed[0].StartedAt)
	require.Equal(t, int32(3), claimed[1].AttemptCount)
	require.Equal(t, "worker-a", claimed[1].WorkerID)
}

func TestThreadRepositoryFinishMemoryFlushJobUsesWorkerLease(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&memoryFlushJobPO{}))

	repo := NewThreadRepository(db)
	for _, job := range []*entity.MemoryFlushJob{
		{
			ID:             2201,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			IdempotencyKey: "summary_input:" + strings.Repeat("a", 64),
			Status:         entity.MemoryFlushJobStatusProcessing,
			WorkerID:       "worker-a",
			LeaseExpiresAt: 900,
			AttemptCount:   1,
			AvailableAt:    100,
			CreatedAt:      100,
			UpdatedAt:      200,
			StartedAt:      200,
		},
		{
			ID:             2202,
			ThreadID:       10,
			RunID:          21,
			SpaceID:        30,
			IdempotencyKey: "summary_input:" + strings.Repeat("b", 64),
			Status:         entity.MemoryFlushJobStatusProcessing,
			WorkerID:       "worker-a",
			LeaseExpiresAt: 900,
			AttemptCount:   1,
			AvailableAt:    100,
			CreatedAt:      100,
			UpdatedAt:      200,
			StartedAt:      200,
		},
		{
			ID:             2203,
			ThreadID:       10,
			RunID:          22,
			SpaceID:        30,
			IdempotencyKey: "summary_input:" + strings.Repeat("c", 64),
			Status:         entity.MemoryFlushJobStatusProcessing,
			WorkerID:       "worker-a",
			LeaseExpiresAt: 900,
			AttemptCount:   1,
			AvailableAt:    100,
			CreatedAt:      100,
			UpdatedAt:      200,
			StartedAt:      200,
		},
	} {
		_, created, err := repo.CreateOrGetMemoryFlushJob(context.Background(), job)
		require.NoError(t, err)
		require.True(t, created)
	}

	completed, ok, err := repo.CompleteMemoryFlushJob(context.Background(), CompleteMemoryFlushJobRequest{
		JobID:    2201,
		WorkerID: "worker-a",
		Now:      300,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.MemoryFlushJobStatusSucceeded, completed.Status)
	require.Equal(t, int64(0), completed.LeaseExpiresAt)
	require.Equal(t, int64(300), completed.EndedAt)

	retried, ok, err := repo.RetryMemoryFlushJob(context.Background(), RetryMemoryFlushJobRequest{
		JobID:       2202,
		WorkerID:    "worker-a",
		ErrorText:   strings.Repeat("x", 600),
		AvailableAt: 450,
		Now:         310,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.MemoryFlushJobStatusPending, retried.Status)
	require.Empty(t, retried.WorkerID)
	require.Len(t, []rune(retried.LastError), 512)
	require.Equal(t, int64(450), retried.AvailableAt)
	require.Equal(t, int64(0), retried.LeaseExpiresAt)

	failed, ok, err := repo.FailMemoryFlushJob(context.Background(), FailMemoryFlushJobRequest{
		JobID:     2203,
		WorkerID:  "worker-a",
		ErrorText: "model refused extraction",
		Now:       320,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.MemoryFlushJobStatusFailed, failed.Status)
	require.Equal(t, "model refused extraction", failed.LastError)
	require.Equal(t, int64(320), failed.EndedAt)

	_, ok, err = repo.CompleteMemoryFlushJob(context.Background(), CompleteMemoryFlushJobRequest{
		JobID:    2203,
		WorkerID: "other-worker",
		Now:      330,
	})
	require.NoError(t, err)
	require.False(t, ok)
}

func TestArtifactRepositoryCreateOrGetScanJobIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactScanJobPO{}))

	repo := NewArtifactRepository(db)
	first := &entity.ArtifactScanJob{
		ID:             3001,
		ThreadID:       10,
		RunID:          20,
		SpaceID:        30,
		UserID:         40,
		ArtifactID:     100,
		FileID:         90,
		Scanner:        "default",
		IdempotencyKey: "artifact_scan:100:default",
		Status:         entity.ArtifactScanJobStatusPending,
		AvailableAt:    100,
		CreatedAt:      100,
		UpdatedAt:      100,
	}

	got, created, err := repo.CreateOrGetArtifactScanJob(
		context.Background(),
		first,
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, first, got)

	duplicate := *first
	duplicate.ID = 3002
	duplicate.CreatedAt = 200
	duplicate.UpdatedAt = 200
	got, created, err = repo.CreateOrGetArtifactScanJob(
		context.Background(),
		&duplicate,
	)

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(3001), got.ID)
	require.Equal(t, entity.ArtifactScanJobStatusPending, got.Status)
	require.Equal(t, int64(100), got.CreatedAt)
}

func TestArtifactRepositoryClaimScanJobsMarksDuePendingProcessing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactScanJobPO{}))

	repo := NewArtifactRepository(db)
	for _, job := range []*entity.ArtifactScanJob{
		{
			ID:             3001,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     100,
			FileID:         90,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:100:default",
			Status:         entity.ArtifactScanJobStatusPending,
			AvailableAt:    100,
			CreatedAt:      100,
			UpdatedAt:      100,
		},
		{
			ID:             3002,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     101,
			FileID:         91,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:101:default",
			Status:         entity.ArtifactScanJobStatusPending,
			AvailableAt:    101,
			CreatedAt:      101,
			UpdatedAt:      101,
		},
		{
			ID:             3003,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     102,
			FileID:         92,
			Scanner:        "other",
			IdempotencyKey: "artifact_scan:102:other",
			Status:         entity.ArtifactScanJobStatusPending,
			AvailableAt:    50,
			CreatedAt:      50,
			UpdatedAt:      50,
		},
		{
			ID:             3004,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     103,
			FileID:         93,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:103:default",
			Status:         entity.ArtifactScanJobStatusPending,
			AvailableAt:    300,
			CreatedAt:      300,
			UpdatedAt:      300,
		},
	} {
		_, _, err := repo.CreateOrGetArtifactScanJob(context.Background(), job)
		require.NoError(t, err)
	}

	claimed, err := repo.ClaimArtifactScanJobs(
		context.Background(),
		ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       "worker-a",
			Limit:          1,
			Now:            200,
			LeaseExpiresAt: 500,
		},
	)

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(3001), claimed[0].ID)
	require.Equal(t, entity.ArtifactScanJobStatusProcessing, claimed[0].Status)
	require.Equal(t, "worker-a", claimed[0].WorkerID)
	require.Equal(t, int64(500), claimed[0].LeaseExpiresAt)
	require.Equal(t, int64(200), claimed[0].StartedAt)
	require.Equal(t, int32(1), claimed[0].AttemptCount)
	require.Equal(t, int64(200), claimed[0].UpdatedAt)

	claimed, err = repo.ClaimArtifactScanJobs(
		context.Background(),
		ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       "worker-b",
			Limit:          10,
			Now:            200,
			LeaseExpiresAt: 600,
		},
	)

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(3002), claimed[0].ID)
	require.Equal(t, "worker-b", claimed[0].WorkerID)
}

func TestArtifactRepositoryClaimScanJobsReclaimsExpiredProcessingLease(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactScanJobPO{}))

	repo := NewArtifactRepository(db)
	for _, job := range []*entity.ArtifactScanJob{
		{
			ID:             3001,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     100,
			FileID:         90,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:100:default",
			Status:         entity.ArtifactScanJobStatusProcessing,
			AttemptCount:   2,
			WorkerID:       "dead-worker",
			LeaseExpiresAt: 100,
			AvailableAt:    50,
			StartedAt:      60,
			CreatedAt:      50,
			UpdatedAt:      60,
		},
		{
			ID:             3002,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     101,
			FileID:         91,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:101:default",
			Status:         entity.ArtifactScanJobStatusProcessing,
			AttemptCount:   1,
			WorkerID:       "active-worker",
			LeaseExpiresAt: 300,
			AvailableAt:    51,
			StartedAt:      70,
			CreatedAt:      51,
			UpdatedAt:      70,
		},
	} {
		_, _, err := repo.CreateOrGetArtifactScanJob(context.Background(), job)
		require.NoError(t, err)
	}

	claimed, err := repo.ClaimArtifactScanJobs(
		context.Background(),
		ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       "worker-a",
			Limit:          10,
			Now:            200,
			LeaseExpiresAt: 500,
		},
	)

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(3001), claimed[0].ID)
	require.Equal(t, "worker-a", claimed[0].WorkerID)
	require.Equal(t, int64(500), claimed[0].LeaseExpiresAt)
	require.Equal(t, int64(60), claimed[0].StartedAt)
	require.Equal(t, int32(3), claimed[0].AttemptCount)
}

func TestArtifactRepositoryCompletesClaimedScanJobWithActiveLease(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactScanJobPO{}))

	repo := NewArtifactRepository(db)
	job := &entity.ArtifactScanJob{
		ID:             3001,
		ThreadID:       10,
		RunID:          20,
		SpaceID:        30,
		UserID:         40,
		ArtifactID:     100,
		FileID:         90,
		Scanner:        "default",
		IdempotencyKey: "artifact_scan:100:default",
		Status:         entity.ArtifactScanJobStatusProcessing,
		WorkerID:       "worker-a",
		AttemptCount:   1,
		LeaseExpiresAt: 500,
		AvailableAt:    100,
		StartedAt:      200,
		CreatedAt:      100,
		UpdatedAt:      200,
	}
	_, _, err = repo.CreateOrGetArtifactScanJob(context.Background(), job)
	require.NoError(t, err)

	completed, ok, err := repo.CompleteArtifactScanJob(
		context.Background(),
		CompleteArtifactScanJobRequest{
			JobID:    3001,
			WorkerID: "worker-a",
			Now:      300,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.ArtifactScanJobStatusSucceeded, completed.Status)
	require.Equal(t, "worker-a", completed.WorkerID)
	require.Equal(t, int64(0), completed.LeaseExpiresAt)
	require.Equal(t, int64(300), completed.EndedAt)
	require.Equal(t, int64(300), completed.UpdatedAt)
	require.Empty(t, completed.LastError)

	completed, ok, err = repo.CompleteArtifactScanJob(
		context.Background(),
		CompleteArtifactScanJobRequest{
			JobID:    3001,
			WorkerID: "worker-a",
			Now:      301,
		},
	)

	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, completed)
}

func TestArtifactRepositoryFailsOnlyClaimedScanJobWithActiveLease(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactScanJobPO{}))

	repo := NewArtifactRepository(db)
	for _, job := range []*entity.ArtifactScanJob{
		{
			ID:             3001,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     100,
			FileID:         90,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:100:default",
			Status:         entity.ArtifactScanJobStatusProcessing,
			WorkerID:       "worker-a",
			AttemptCount:   1,
			LeaseExpiresAt: 500,
			AvailableAt:    100,
			StartedAt:      200,
			CreatedAt:      100,
			UpdatedAt:      200,
		},
		{
			ID:             3002,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     101,
			FileID:         91,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:101:default",
			Status:         entity.ArtifactScanJobStatusProcessing,
			WorkerID:       "worker-b",
			AttemptCount:   1,
			LeaseExpiresAt: 100,
			AvailableAt:    100,
			StartedAt:      200,
			CreatedAt:      100,
			UpdatedAt:      200,
		},
	} {
		_, _, err := repo.CreateOrGetArtifactScanJob(context.Background(), job)
		require.NoError(t, err)
	}

	failed, ok, err := repo.FailArtifactScanJob(
		context.Background(),
		FailArtifactScanJobRequest{
			JobID:     3001,
			WorkerID:  "worker-a",
			ErrorText: strings.Repeat("x", 600),
			Now:       300,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.ArtifactScanJobStatusFailed, failed.Status)
	require.Equal(t, "worker-a", failed.WorkerID)
	require.Equal(t, int64(0), failed.LeaseExpiresAt)
	require.Equal(t, int64(300), failed.EndedAt)
	require.Len(t, failed.LastError, 512)

	failed, ok, err = repo.FailArtifactScanJob(
		context.Background(),
		FailArtifactScanJobRequest{
			JobID:     3002,
			WorkerID:  "worker-b",
			ErrorText: "late",
			Now:       300,
		},
	)

	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, failed)
}

func TestArtifactRepositoryRetriesClaimedScanJobWithBackoff(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactScanJobPO{}))

	repo := NewArtifactRepository(db)
	job := &entity.ArtifactScanJob{
		ID:             3001,
		ThreadID:       10,
		RunID:          20,
		SpaceID:        30,
		UserID:         40,
		ArtifactID:     100,
		FileID:         90,
		Scanner:        "default",
		IdempotencyKey: "artifact_scan:100:default",
		Status:         entity.ArtifactScanJobStatusProcessing,
		WorkerID:       "worker-a",
		AttemptCount:   1,
		LeaseExpiresAt: 500,
		AvailableAt:    100,
		StartedAt:      200,
		CreatedAt:      100,
		UpdatedAt:      200,
	}
	_, _, err = repo.CreateOrGetArtifactScanJob(context.Background(), job)
	require.NoError(t, err)

	retried, ok, err := repo.RetryArtifactScanJob(
		context.Background(),
		RetryArtifactScanJobRequest{
			JobID:       3001,
			WorkerID:    "worker-a",
			ErrorText:   strings.Repeat("x", 600),
			AvailableAt: 700,
			Now:         300,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.ArtifactScanJobStatusPending, retried.Status)
	require.Empty(t, retried.WorkerID)
	require.Equal(t, int64(0), retried.LeaseExpiresAt)
	require.Equal(t, int64(0), retried.EndedAt)
	require.Equal(t, int64(700), retried.AvailableAt)
	require.Equal(t, int64(300), retried.UpdatedAt)
	require.Equal(t, int32(1), retried.AttemptCount)
	require.Len(t, retried.LastError, 512)

	claimed, err := repo.ClaimArtifactScanJobs(
		context.Background(),
		ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       "worker-b",
			Limit:          10,
			Now:            600,
			LeaseExpiresAt: 900,
		},
	)
	require.NoError(t, err)
	require.Empty(t, claimed)

	claimed, err = repo.ClaimArtifactScanJobs(
		context.Background(),
		ClaimArtifactScanJobsRequest{
			Scanner:        "default",
			WorkerID:       "worker-b",
			Limit:          10,
			Now:            700,
			LeaseExpiresAt: 1000,
		},
	)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(3001), claimed[0].ID)
	require.Equal(t, int32(2), claimed[0].AttemptCount)
	require.Equal(t, "worker-b", claimed[0].WorkerID)
}

func TestArtifactRepositoryRequeuesFailedScanJobForManualRetry(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactScanJobPO{}))

	repo := NewArtifactRepository(db)
	for _, job := range []*entity.ArtifactScanJob{
		{
			ID:             3001,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     100,
			FileID:         90,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:100:default",
			Status:         entity.ArtifactScanJobStatusFailed,
			WorkerID:       "worker-a",
			AttemptCount:   3,
			LastError:      "scanner unavailable",
			AvailableAt:    100,
			LeaseExpiresAt: 900,
			StartedAt:      200,
			EndedAt:        800,
			CreatedAt:      100,
			UpdatedAt:      800,
		},
		{
			ID:             3002,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     101,
			FileID:         91,
			Scanner:        "default",
			IdempotencyKey: "artifact_scan:101:default",
			Status:         entity.ArtifactScanJobStatusSucceeded,
			CreatedAt:      100,
			UpdatedAt:      800,
		},
	} {
		_, _, err := repo.CreateOrGetArtifactScanJob(context.Background(), job)
		require.NoError(t, err)
	}

	requeued, ok, err := repo.RequeueFailedArtifactScanJob(
		context.Background(),
		RequeueFailedArtifactScanJobRequest{
			JobID:       3001,
			ThreadID:    10,
			ErrorText:   "manual retry requested",
			AvailableAt: 1200,
			Now:         1100,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.ArtifactScanJobStatusPending, requeued.Status)
	require.Empty(t, requeued.WorkerID)
	require.Equal(t, int64(0), requeued.LeaseExpiresAt)
	require.Equal(t, int64(0), requeued.StartedAt)
	require.Equal(t, int64(0), requeued.EndedAt)
	require.Equal(t, int64(1200), requeued.AvailableAt)
	require.Equal(t, int64(1100), requeued.UpdatedAt)
	require.Equal(t, int32(3), requeued.AttemptCount)
	require.Equal(t, "manual retry requested", requeued.LastError)

	requeued, ok, err = repo.RequeueFailedArtifactScanJob(
		context.Background(),
		RequeueFailedArtifactScanJobRequest{JobID: 3002, ThreadID: 10, Now: 1200},
	)
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, requeued)

	requeued, ok, err = repo.RequeueFailedArtifactScanJob(
		context.Background(),
		RequeueFailedArtifactScanJobRequest{JobID: 3001, ThreadID: 11, Now: 1200},
	)
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, requeued)
}

func TestArtifactRepositoryListScanJobsFiltersByThreadStatusAndScanner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactScanJobPO{}))

	repo := NewArtifactRepository(db)
	for _, job := range []*entity.ArtifactScanJob{
		{
			ID:             3001,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     100,
			FileID:         90,
			Scanner:        "clamav",
			IdempotencyKey: "artifact_scan:100:clamav",
			Status:         entity.ArtifactScanJobStatusFailed,
			WorkerID:       "worker-a",
			AttemptCount:   3,
			LastError:      "scanner unavailable",
			EndedAt:        900,
			CreatedAt:      100,
			UpdatedAt:      900,
		},
		{
			ID:             3002,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     101,
			FileID:         91,
			Scanner:        "clamav",
			IdempotencyKey: "artifact_scan:101:clamav",
			Status:         entity.ArtifactScanJobStatusFailed,
			WorkerID:       "worker-b",
			AttemptCount:   1,
			LastError:      "older",
			EndedAt:        800,
			CreatedAt:      101,
			UpdatedAt:      800,
		},
		{
			ID:             3003,
			ThreadID:       10,
			RunID:          21,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     102,
			FileID:         92,
			Scanner:        "clamav",
			IdempotencyKey: "artifact_scan:102:clamav",
			Status:         entity.ArtifactScanJobStatusPending,
			CreatedAt:      102,
			UpdatedAt:      901,
		},
		{
			ID:             3004,
			ThreadID:       11,
			RunID:          20,
			SpaceID:        31,
			UserID:         41,
			ArtifactID:     103,
			FileID:         93,
			Scanner:        "clamav",
			IdempotencyKey: "artifact_scan:103:clamav",
			Status:         entity.ArtifactScanJobStatusFailed,
			CreatedAt:      103,
			UpdatedAt:      902,
		},
		{
			ID:             3005,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     104,
			FileID:         94,
			Scanner:        "other",
			IdempotencyKey: "artifact_scan:104:other",
			Status:         entity.ArtifactScanJobStatusFailed,
			CreatedAt:      104,
			UpdatedAt:      903,
		},
	} {
		_, _, err := repo.CreateOrGetArtifactScanJob(context.Background(), job)
		require.NoError(t, err)
	}

	status := entity.ArtifactScanJobStatusFailed
	runID := int64(20)
	jobs, total, err := repo.ListArtifactScanJobs(
		context.Background(),
		ListArtifactScanJobsRequest{
			ThreadID: 10,
			RunID:    &runID,
			Status:   &status,
			Scanner:  "clamav",
			Page:     1,
			PageSize: 10,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, jobs, 2)
	require.Equal(t, []int64{3001, 3002}, []int64{jobs[0].ID, jobs[1].ID})
	require.Equal(t, "scanner unavailable", jobs[0].LastError)

	artifactID := int64(101)
	jobs, total, err = repo.ListArtifactScanJobs(
		context.Background(),
		ListArtifactScanJobsRequest{
			ThreadID:   10,
			ArtifactID: &artifactID,
			Status:     &status,
			Page:       1,
			PageSize:   10,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, jobs, 1)
	require.Equal(t, int64(3002), jobs[0].ID)
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

	scopedRows, scopedTotal, err := repo.ListTokenUsage(context.Background(), ListTokenUsageRequest{
		ThreadID: 10,
		RunIDs:   []int64{20, 21},
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), scopedTotal)
	require.Len(t, scopedRows, 3)

	scopedAggregate, err := repo.AggregateTokenUsage(context.Background(), AggregateTokenUsageRequest{
		ThreadID: 10,
		RunIDs:   []int64{20, 21},
	})
	require.NoError(t, err)
	require.Equal(t, int64(35), scopedAggregate.TotalTokens)
	require.Equal(t, int64(3), scopedAggregate.CallCount)
	require.Equal(t, int64(20), scopedAggregate.LeadAgentTokens)
	require.Equal(t, int64(10), scopedAggregate.ToolTokens)
	require.Equal(t, int64(5), scopedAggregate.MiddlewareTokens)

	aggregateByRun, err := repo.AggregateTokenUsageByRun(context.Background(), AggregateTokenUsageRequest{
		ThreadID: 10,
		RunIDs:   []int64{20, 21},
	})
	require.NoError(t, err)
	require.Len(t, aggregateByRun, 2)
	require.Equal(t, int64(20), aggregateByRun[0].RunID)
	require.Equal(t, int64(30), aggregateByRun[0].Aggregate.TotalTokens)
	require.Equal(t, int64(20), aggregateByRun[0].Aggregate.LeadAgentTokens)
	require.Equal(t, int64(10), aggregateByRun[0].Aggregate.ToolTokens)
	require.Equal(t, int64(21), aggregateByRun[1].RunID)
	require.Equal(t, int64(5), aggregateByRun[1].Aggregate.TotalTokens)
	require.Equal(t, int64(5), aggregateByRun[1].Aggregate.MiddlewareTokens)
}

func TestThreadRepositoryCreateTokenUsageIsIdempotentByMetadataKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&tokenUsagePO{}))
	require.NoError(t, db.Exec(`
		CREATE UNIQUE INDEX uk_agent_token_usage_idempotency
		ON agent_token_usage(run_id, json_extract(metadata, '$.idempotency_key'))
		WHERE json_extract(metadata, '$.idempotency_key') IS NOT NULL
	`).Error)

	repo := NewThreadRepository(db)
	for _, id := range []int64{1, 2} {
		require.NoError(t, repo.CreateTokenUsage(context.Background(), &entity.TokenUsage{
			ID:        id,
			ThreadID:  10,
			RunID:     20,
			SpaceID:   1,
			Source:    entity.TokenUsageSourceLeadAgent,
			Metadata:  `{"idempotency_key":"same-call"}`,
			CreatedAt: 100 + id,
		}))
	}

	rows, total, err := repo.ListTokenUsage(context.Background(), ListTokenUsageRequest{
		RunID:    20,
		Page:     1,
		PageSize: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, int64(1), rows[0].ID)
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
	child := newRepositoryTestRun(4, 10, entity.RunStatusPending, 50)
	child.ParentRunID = 1
	child.RunKind = entity.RunKindSubagent
	require.NoError(t, repo.CreateRun(context.Background(), child))
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

func TestThreadRepositoryClaimQueuedResumeRunsMarksOldestResumeRunsRunning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	resumeQueued := newRepositoryTestRun(1, 10, entity.RunStatusQueued, 100)
	resumeQueued.Metadata = `{"checkpoint_resume":{"protected_from_worker_claim":true}}`
	plainQueued := newRepositoryTestRun(2, 10, entity.RunStatusQueued, 101)
	plainQueued.Metadata = `{"source":"manual_queue"}`
	pendingResume := newRepositoryTestRun(3, 10, entity.RunStatusPending, 99)
	pendingResume.Metadata = `{"checkpoint_resume":{"protected_from_worker_claim":true}}`
	require.NoError(t, repo.CreateRun(context.Background(), resumeQueued))
	require.NoError(t, repo.CreateRun(context.Background(), plainQueued))
	require.NoError(t, repo.CreateRun(context.Background(), pendingResume))

	claimed, err := repo.ClaimQueuedResumeRuns(context.Background(), ClaimQueuedResumeRunsRequest{
		WorkerID: "resume-worker-a",
		Limit:    10,
	})

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(1), claimed[0].ID)
	require.Equal(t, entity.RunStatusRunning, claimed[0].Status)
	require.Equal(t, "resume-worker-a", claimed[0].WorkerID)
	require.NotZero(t, claimed[0].StartedAt)

	gotPlain, err := repo.GetRun(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusQueued, gotPlain.Status)
	require.Empty(t, gotPlain.WorkerID)

	gotPending, err := repo.GetRun(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusPending, gotPending.Status)
	require.Empty(t, gotPending.WorkerID)
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
