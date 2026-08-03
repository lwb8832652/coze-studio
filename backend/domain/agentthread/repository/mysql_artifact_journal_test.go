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
	"reflect"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestArtifactJournalPersistenceDeclaresFrozenFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))

	for _, column := range []string{
		"journal_run_id",
		"source",
		"generation_status",
		"primary_slot",
		"collection_id",
		"collection_order",
		"detected_content_type",
		"scanned_size_bytes",
		"content_hash",
	} {
		require.Truef(
			t,
			db.Migrator().HasColumn(&agentArtifactPO{}, column),
			"agent_artifacts must persist %s",
			column,
		)
	}
}

func TestArtifactJournalPrimaryReplacementIsAtomic(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))
	repo := NewArtifactRepository(db)

	first := artifactJournalFixture(101, 1001, 10, 200, 201)
	first.IsPrimary = true
	_, created, err := repo.UpsertArtifact(context.Background(), first)
	require.NoError(t, err)
	require.True(t, created)

	second := artifactJournalFixture(102, 1002, 10, 201, 201)
	second.IsPrimary = true
	stored, created, err := repo.UpsertArtifact(context.Background(), second)
	require.NoError(t, err)
	require.True(t, created)
	require.True(t, stored.IsPrimary)

	var primaryCount int64
	require.NoError(t, db.Model(&agentArtifactPO{}).
		Where("journal_run_id = ? AND primary_slot = ?", 201, 1).
		Count(&primaryCount).Error)
	require.Equal(t, int64(1), primaryCount)

	old, err := repo.GetArtifact(context.Background(), 10, first.ID)
	require.NoError(t, err)
	require.NotNil(t, old)
	require.False(t, old.IsPrimary)
}

func TestArtifactJournalDerivesLogicalRunFromExecutionAttempt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))
	require.NoError(t, db.Create(&runAttemptPO{
		ID:             301,
		ThreadID:       10,
		JournalRunID:   200,
		ExecutionRunID: 201,
		AttemptID:      "attempt-retry",
		Ordinal:        2,
		Status:         "running",
		CreatedAt:      1000,
		UpdatedAt:      1000,
	}).Error)
	repo := NewArtifactRepository(db)

	artifact := artifactJournalFixture(109, 1009, 10, 201, 201)
	stored, created, err := repo.UpsertArtifact(context.Background(), artifact)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(200), stored.JournalRunID)
}

func TestArtifactJournalLegacyNullableFieldsFallbackToPhysicalRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))
	repo := NewArtifactRepository(db)
	artifact := artifactJournalFixture(115, 1015, 10, 209, 209)
	artifact.Metadata = `{"scan_status":"clean"}`
	_, _, err = repo.UpsertArtifact(context.Background(), artifact)
	require.NoError(t, err)
	require.NoError(t, db.Model(&agentArtifactPO{}).
		Where("id = ?", artifact.ID).
		Updates(map[string]any{
			"journal_run_id":    nil,
			"source":            nil,
			"generation_status": nil,
		}).Error)

	runID := artifact.RunID
	artifacts, total, err := repo.ListArtifacts(context.Background(), ListArtifactsRequest{
		ThreadID: 10,
		RunID:    &runID,
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, artifacts, 1)
	require.Equal(t, runID, artifacts[0].JournalRunID)
	require.Equal(t, entity.AgentArtifactSourceAgentGenerated, artifacts[0].Source)
	require.Equal(t, entity.AgentArtifactGenerationStatusReady, artifacts[0].GenerationStatus)
}

func TestArtifactJournalDeleteClearsPrimaryAndRestoreDoesNotReclaimIt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))
	repo := NewArtifactRepository(db)

	first := artifactJournalFixture(110, 1010, 10, 206, 206)
	first.IsPrimary = true
	_, _, err = repo.UpsertArtifact(context.Background(), first)
	require.NoError(t, err)

	deleted, changed, err := repo.DeleteArtifact(context.Background(), 10, first.ID, 2000)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, deleted.IsPrimary)

	second := artifactJournalFixture(111, 1011, 10, 206, 206)
	second.IsPrimary = true
	_, _, err = repo.UpsertArtifact(context.Background(), second)
	require.NoError(t, err)

	restored, changed, err := repo.RestoreArtifact(context.Background(), 10, first.ID, 3000)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, restored.IsPrimary)

	var primaryCount int64
	require.NoError(t, db.Model(&agentArtifactPO{}).
		Where("journal_run_id = ? AND primary_slot = ?", 206, 1).
		Count(&primaryCount).Error)
	require.Equal(t, int64(1), primaryCount)
}

func TestArtifactJournalCollectionFilterUsesLogicalRunAndStableOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))
	repo := NewArtifactRepository(db)
	collectionID := "collection_8f3f"

	for _, item := range []struct {
		id         int64
		fileID     int64
		threadID   int64
		runID      int64
		journalRun int64
		order      int32
	}{
		{id: 103, fileID: 1003, threadID: 10, runID: 202, journalRun: 201, order: 2},
		{id: 104, fileID: 1004, threadID: 10, runID: 200, journalRun: 201, order: 0},
		{id: 105, fileID: 1005, threadID: 10, runID: 201, journalRun: 201, order: 1},
		{id: 106, fileID: 1006, threadID: 10, runID: 203, journalRun: 202, order: 0},
		{id: 107, fileID: 1007, threadID: 11, runID: 204, journalRun: 203, order: 0},
	} {
		artifact := artifactJournalFixture(item.id, item.fileID, item.threadID, item.runID, item.journalRun)
		artifact.CollectionID = collectionID
		artifact.CollectionOrder = &item.order
		_, _, err := repo.UpsertArtifact(context.Background(), artifact)
		require.NoError(t, err)
	}

	journalRunID := int64(201)
	req := ListArtifactsRequest{
		ThreadID: 10,
		RunID:    &journalRunID,
		Page:     1,
		PageSize: 20,
	}
	setArtifactCollectionFilterForTest(t, &req, collectionID)
	artifacts, total, err := repo.ListArtifacts(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, artifacts, 3)
	require.Equal(t, []int64{104, 105, 103}, []int64{
		artifacts[0].ID,
		artifacts[1].ID,
		artifacts[2].ID,
	})
}

func TestArtifactJournalTrustedScanResultUpdatesPublicMetadataAtomically(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))
	repo := NewArtifactRepository(db)
	artifact := artifactJournalFixture(108, 1008, 10, 205, 205)
	artifact.ContentType = "application/octet-stream"
	artifact.PreviewMode = entity.AgentArtifactPreviewModeDownload
	_, _, err = repo.UpsertArtifact(context.Background(), artifact)
	require.NoError(t, err)

	updater, ok := repo.(artifactTrustedScanResultUpdater)
	require.True(t, ok, "artifact repository must support trusted scan updates")
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	updated, changed, err := updater.UpdateArtifactTrustedScanResult(
		context.Background(),
		10,
		artifact.ID,
		`{"scan_status":"clean"}`,
		"audio/mpeg",
		8,
		hash,
		entity.AgentArtifactPreviewModeAudio,
		entity.AgentArtifactGenerationStatusReady,
		3000,
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, updated)
	require.Equal(t, "audio/mpeg", updated.DetectedContentType)
	require.NotNil(t, updated.ScannedSizeBytes)
	require.Equal(t, int64(8), *updated.ScannedSizeBytes)
	require.Equal(t, hash, updated.ContentHash)
	require.Equal(t, entity.AgentArtifactPreviewModeAudio, updated.PreviewMode)
	require.Equal(t, entity.AgentArtifactGenerationStatusReady, updated.GenerationStatus)
}

func TestArtifactJournalIdempotentUpsertPreservesTrustedScanResult(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))
	repo := NewArtifactRepository(db)
	artifact := artifactJournalFixture(113, 1013, 10, 207, 207)
	artifact.Metadata = `{"scan_status":"pending","scan_revision":"revision-a"}`
	stored, created, err := repo.UpsertArtifact(context.Background(), artifact)
	require.NoError(t, err)
	require.True(t, created)

	hash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	stored, changed, err := repo.UpdateArtifactTrustedScanResult(
		context.Background(),
		10,
		stored.ID,
		`{"scan_status":"clean","scan_scanned_at":2000,"scan_revision":"revision-a"}`,
		"application/pdf",
		64,
		hash,
		entity.AgentArtifactPreviewModePDF,
		entity.AgentArtifactGenerationStatusReady,
		2000,
	)
	require.NoError(t, err)
	require.True(t, changed)

	repeated := artifactJournalFixture(999, artifact.FileID, 10, 207, 207)
	repeated.Title = "updated title"
	repeated.Metadata = `{"scan_status":"pending","scan_revision":"revision-a"}`
	repeated.GenerationStatus = entity.AgentArtifactGenerationStatusProcessing
	repeated.PreviewMode = entity.AgentArtifactPreviewModeText
	repeated.UpdatedAt = 3000
	stored, created, err = repo.UpsertArtifact(context.Background(), repeated)

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, artifact.ID, stored.ID)
	require.Equal(t, "updated title", stored.Title)
	require.Equal(t, entity.AgentArtifactGenerationStatusReady, stored.GenerationStatus)
	require.Equal(t, entity.AgentArtifactPreviewModePDF, stored.PreviewMode)
	require.Equal(t, "application/pdf", stored.DetectedContentType)
	require.NotNil(t, stored.ScannedSizeBytes)
	require.Equal(t, int64(64), *stored.ScannedSizeBytes)
	require.Equal(t, hash, stored.ContentHash)
	require.JSONEq(t, `{"scan_status":"clean","scan_scanned_at":2000,"scan_revision":"revision-a"}`, stored.Metadata)
}

func TestArtifactJournalChangedRevisionClearsTrustedScanResult(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agentArtifactPO{}, &runAttemptPO{}))
	repo := NewArtifactRepository(db)
	artifact := artifactJournalFixture(114, 1014, 10, 208, 208)
	artifact.Metadata = `{"scan_status":"pending","scan_revision":"revision-a"}`
	stored, created, err := repo.UpsertArtifact(context.Background(), artifact)
	require.NoError(t, err)
	require.True(t, created)

	stored, changed, err := repo.UpdateArtifactTrustedScanResult(
		context.Background(),
		10,
		stored.ID,
		`{"scan_status":"clean","scan_scanned_at":2000,"scan_revision":"revision-a"}`,
		"application/pdf",
		64,
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		entity.AgentArtifactPreviewModePDF,
		entity.AgentArtifactGenerationStatusReady,
		2000,
	)
	require.NoError(t, err)
	require.True(t, changed)

	repeated := artifactJournalFixture(999, artifact.FileID, 10, 208, 208)
	repeated.Metadata = `{"scan_status":"pending","scan_revision":"revision-b"}`
	repeated.GenerationStatus = entity.AgentArtifactGenerationStatusProcessing
	repeated.UpdatedAt = 3000
	stored, created, err = repo.UpsertArtifact(context.Background(), repeated)

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, entity.AgentArtifactGenerationStatusProcessing, stored.GenerationStatus)
	require.Equal(t, entity.AgentArtifactPreviewModeText, stored.PreviewMode)
	require.Empty(t, stored.DetectedContentType)
	require.Nil(t, stored.ScannedSizeBytes)
	require.Empty(t, stored.ContentHash)
	require.JSONEq(t, `{"scan_status":"pending","scan_revision":"revision-b"}`, stored.Metadata)
}

type artifactTrustedScanResultUpdater interface {
	UpdateArtifactTrustedScanResult(
		ctx context.Context,
		threadID int64,
		artifactID int64,
		metadata string,
		detectedContentType string,
		scannedSizeBytes int64,
		contentHash string,
		previewMode entity.AgentArtifactPreviewMode,
		generationStatus entity.AgentArtifactGenerationStatus,
		updatedAt int64,
	) (*entity.AgentArtifact, bool, error)
}

func setArtifactCollectionFilterForTest(
	t *testing.T,
	req *ListArtifactsRequest,
	collectionID string,
) {
	t.Helper()
	field := reflect.ValueOf(req).Elem().FieldByName("CollectionID")
	require.True(t, field.IsValid(), "ListArtifactsRequest must expose CollectionID")
	value := collectionID
	field.Set(reflect.ValueOf(&value))
}

func artifactJournalFixture(
	id int64,
	fileID int64,
	threadID int64,
	runID int64,
	journalRunID int64,
) *entity.AgentArtifact {
	return &entity.AgentArtifact{
		ID:               id,
		SpaceID:          1,
		UserID:           2,
		ThreadID:         threadID,
		RunID:            runID,
		JournalRunID:     journalRunID,
		FileID:           fileID,
		Title:            "artifact",
		ArtifactType:     "report",
		VirtualPath:      "/mnt/user-data/outputs/artifact.txt",
		ObjectURI:        "agent-runtime/object",
		ContentType:      "text/plain",
		SizeBytes:        8,
		PreviewMode:      entity.AgentArtifactPreviewModeText,
		Source:           entity.AgentArtifactSourceAgentGenerated,
		GenerationStatus: entity.AgentArtifactGenerationStatusProcessing,
		Metadata:         `{}`,
		CreatedAt:        id,
		UpdatedAt:        id,
	}
}
