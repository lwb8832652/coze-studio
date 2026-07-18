// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
)

func TestCodePluginRepositorySaveDraftCAS(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 101)
	ctx := context.Background()

	saved, err := repo.SaveDraftCAS(ctx, codeDraftFixture(101, 10, "return input"), 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), saved.Revision)
	require.Zero(t, saved.LastDebuggedRevision)
	require.Len(t, saved.Files, 1)
	require.Equal(t, entity.DefaultCodeSchemaJSON, saved.InputSchemaJSON)
	require.Equal(t, entity.DefaultCodeSchemaJSON, saved.OutputSchemaJSON)

	require.NoError(t, repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision))
	saved.Files[0].Content = []byte("return changed")
	updated, err := repo.SaveDraftCAS(ctx, saved, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.Revision)
	require.Equal(t, int64(1), updated.LastDebuggedRevision)

	_, err = repo.SaveDraftCAS(ctx, saved, 1)
	require.ErrorIs(t, err, ErrCodeDraftConflict)

	got, exists, err := repo.GetDraft(ctx, saved.PluginID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, int64(2), got.Revision)
	require.Equal(t, []byte("return changed"), got.Files[0].Content)
}

func TestCodePluginRepositoryVersionIsImmutable(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 102)
	ctx := context.Background()

	saved, err := repo.SaveDraftCAS(ctx, codeDraftFixture(102, 10, "return v1"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision))
	require.NoError(t, repo.PublishDebuggedVersion(ctx, saved.PluginID, "1.0.0", 99))
	require.NoError(t, repo.PublishDebuggedVersion(ctx, saved.PluginID, "1.0.0", 99))
	require.NoError(t, repo.db.Create(&publishedPluginVersionPO{PluginID: saved.PluginID, Version: "1.0.0"}).Error)

	saved.Files[0].Content = []byte("return v2")
	updated, err := repo.SaveDraftCAS(ctx, saved, saved.Revision)
	require.NoError(t, err)

	version, exists, err := repo.GetVersion(ctx, saved.PluginID, "1.0.0")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, int64(1), version.SourceRevision)
	require.Equal(t, []byte("return v1"), version.Files[0].Content)
	require.NoError(t, repo.PublishDebuggedVersion(ctx, saved.PluginID, "1.0.0", 99))
	require.NoError(t, repo.MarkDebuggedCAS(ctx, updated.PluginID, updated.Revision))
	require.NoError(t, repo.PublishDebuggedVersion(ctx, saved.PluginID, "1.0.0", 99))
}

func TestCodePluginRepositoryRecoverablePublishSaga(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 115)
	ctx := context.Background()
	draft, err := repo.SaveDraftCAS(ctx, codeDraftFixture(115, 10, "return v1"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))

	prepared, err := repo.PrepareDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99)
	require.NoError(t, err)
	require.True(t, prepared.Created)
	published, err := repo.CompensatePreparedVersion(ctx, prepared)
	require.NoError(t, err)
	require.False(t, published)
	_, exists, err := repo.GetVersion(ctx, draft.PluginID, "v1.0.0")
	require.NoError(t, err)
	require.False(t, exists)

	prepared, err = repo.PrepareDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99)
	require.NoError(t, err)
	require.NoError(t, repo.db.Create(&publishedPluginVersionPO{PluginID: draft.PluginID, Version: "v1.0.0"}).Error)
	published, err = repo.CompensatePreparedVersion(ctx, prepared)
	require.NoError(t, err)
	require.True(t, published)
	_, exists, err = repo.GetVersion(ctx, draft.PluginID, "v1.0.0")
	require.NoError(t, err)
	require.True(t, exists)
}

func TestCodePluginRepositoryReplacesOnlyUnpublishedOrphan(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 116)
	ctx := context.Background()
	draft, err := repo.SaveDraftCAS(ctx, codeDraftFixture(116, 10, "return old"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))
	_, err = repo.PrepareDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99)
	require.NoError(t, err)

	draft.Files[0].Content = []byte("return replacement")
	draft, err = repo.SaveDraftCAS(ctx, draft, draft.Revision)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))
	replacement, err := repo.PrepareDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99)
	require.NoError(t, err)
	require.True(t, replacement.Created)
	require.Equal(t, draft.SourceBundleRef, replacement.Version.SourceBundleRef)

	require.NoError(t, repo.db.Create(&publishedPluginVersionPO{PluginID: draft.PluginID, Version: "v1.0.0"}).Error)
	draft.Files[0].Content = []byte("return forbidden")
	draft, err = repo.SaveDraftCAS(ctx, draft, draft.Revision)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))
	existing, err := repo.PrepareDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99)
	require.NoError(t, err)
	require.Equal(t, CodeVersionAlreadyPublished, existing.State)
	require.Equal(t, replacement.Version.SourceBundleRef, existing.Version.SourceBundleRef)
}

func TestCodePluginRepositoryEnsureRepairsPublishedSnapshot(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 117)
	ctx := context.Background()
	draft, err := repo.SaveDraftCAS(ctx, codeDraftFixture(117, 10, "return v1"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))
	prepared, err := repo.PrepareDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99)
	require.NoError(t, err)
	require.NoError(t, repo.db.Create(&publishedPluginVersionPO{PluginID: draft.PluginID, Version: "v1.0.0"}).Error)
	require.NoError(t, repo.db.Where("plugin_id = ? AND version = ?", draft.PluginID, "v1.0.0").Delete(&codeVersionPO{}).Error)
	require.NoError(t, repo.EnsurePublishedVersion(ctx, prepared))
	version, exists, err := repo.GetVersion(ctx, draft.PluginID, "v1.0.0")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, prepared.Version.SourceBundleRef, version.SourceBundleRef)
}

func TestCodePluginRepositoryNeverRebuildsPublishedVersionFromCurrentDraft(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 119)
	ctx := context.Background()
	draft, err := repo.SaveDraftCAS(ctx, codeDraftFixture(119, 10, "return published"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))
	prepared, err := repo.PrepareDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99)
	require.NoError(t, err)
	require.NoError(t, repo.db.Create(&publishedPluginVersionPO{PluginID: draft.PluginID, Version: "v1.0.0"}).Error)
	require.NoError(t, repo.db.Where("plugin_id = ? AND version = ?", draft.PluginID, "v1.0.0").Delete(&codeVersionPO{}).Error)

	draft.Files[0].Content = []byte("return current-draft")
	draft, err = repo.SaveDraftCAS(ctx, draft, draft.Revision)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))
	_, err = repo.PrepareDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99)
	require.ErrorIs(t, err, ErrCodeVersionExists)
	require.NoError(t, repo.EnsurePublishedVersion(ctx, prepared))
	version, exists, err := repo.GetVersion(ctx, draft.PluginID, "v1.0.0")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, prepared.Version.SourceBundleRef, version.SourceBundleRef)
	require.NotEqual(t, draft.SourceBundleRef, version.SourceBundleRef)
}

func TestCodePluginRepositoryRejectsVersionLongerThanStorageContract(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	_, err := repo.PrepareDebuggedVersion(context.Background(), 1, strings.Repeat("v", entity.MaxCodeVersionLength+1), 99)
	require.ErrorContains(t, err, "64")
}

type codePluginPublishContextKey struct{}

func TestCodePluginRepositoryPublishDebuggedVersionSerializesConcurrentSave(t *testing.T) {
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 112)
	ctx := context.Background()

	saved, err := repo.SaveDraftCAS(ctx, codeDraftFixture(112, 10, "return v1"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision))

	publishReachedParentLock := make(chan struct{})
	allowPublishToLock := make(chan struct{})
	var signalOnce sync.Once
	const callbackName = "test:block_code_plugin_publish_parent_lock"
	require.NoError(t, repo.db.Callback().Query().Before("gorm:query").Register(callbackName, func(db *gorm.DB) {
		if db.Statement.Context.Value(codePluginPublishContextKey{}) != "publish" ||
			db.Statement.Table != "plugin_draft" {
			return
		}
		signalOnce.Do(func() {
			close(publishReachedParentLock)
			<-allowPublishToLock
		})
	}))
	t.Cleanup(func() {
		require.NoError(t, repo.db.Callback().Query().Remove(callbackName))
	})

	publishResult := make(chan error, 1)
	publishCtx := context.WithValue(ctx, codePluginPublishContextKey{}, "publish")
	go func() {
		publishResult <- repo.PublishDebuggedVersion(publishCtx, saved.PluginID, "race-v1", 99)
	}()

	select {
	case <-publishReachedParentLock:
	case <-time.After(2 * time.Second):
		close(allowPublishToLock)
		t.Fatal("publish did not reach the parent lock")
	}

	updatedDraft := codeDraftFixture(saved.PluginID, saved.SpaceID, "return v2")
	updated, err := repo.SaveDraftCAS(ctx, updatedDraft, saved.Revision)
	require.NoError(t, err)
	require.Equal(t, saved.Revision+1, updated.Revision)
	require.NotEqual(t, updated.Revision, updated.LastDebuggedRevision)
	close(allowPublishToLock)

	select {
	case publishErr := <-publishResult:
		require.ErrorIs(t, publishErr, ErrCodeDraftNotDebugged)
	case <-time.After(2 * time.Second):
		t.Fatal("publish did not finish")
	}

	_, exists, err := repo.GetVersion(ctx, saved.PluginID, "race-v1")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestCodePluginRepositoryCopyResetsDebugState(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParentInSpace(t, repo, 103, 10)
	seedCodePluginParentInSpace(t, repo, 203, 20)
	ctx := context.Background()

	source, err := repo.SaveDraftCAS(ctx, codeDraftFixture(103, 10, "return source"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, source.PluginID, source.Revision))
	require.NoError(t, repo.CopyDraft(ctx, source.PluginID, 203, 20))

	copied, exists, err := repo.GetDraft(ctx, 203)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, int64(20), copied.SpaceID)
	require.Equal(t, int64(1), copied.Revision)
	require.Zero(t, copied.LastDebuggedRevision)
	require.Equal(t, source.SourceBundleRef, copied.SourceBundleRef)
	require.Equal(t, source.Files[0].Content, copied.Files[0].Content)
}

func TestCodePluginRepositorySchemaRoundTripCopyVersionAndHash(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParentInSpace(t, repo, 110, 10)
	seedCodePluginParentInSpace(t, repo, 210, 20)
	ctx := context.Background()

	draft := codeDraftFixture(110, 10, "return input")
	draft.InputSchemaJSON = ` { "required": ["query"], "type": "object", "properties": {"query": {"type": "string"}} } `
	draft.OutputSchemaJSON = `{"type":"object","properties":{"result":{"type":"string"}}}`
	saved, err := repo.SaveDraftCAS(ctx, draft, 0)
	require.NoError(t, err)
	require.Equal(t, `{"properties":{"query":{"type":"string"}},"required":["query"],"type":"object"}`, saved.InputSchemaJSON)
	require.Equal(t, `{"properties":{"result":{"type":"string"}},"type":"object"}`, saved.OutputSchemaJSON)

	equivalent := codeDraftFixture(110, 10, "return input")
	equivalent.InputSchemaJSON = `{"properties":{"query":{"type":"string"}},"type":"object","required":["query"]}`
	equivalent.OutputSchemaJSON = `{"properties":{"result":{"type":"string"}},"type":"object"}`
	preparedEquivalent, err := entity.PrepareCodeDraft(equivalent)
	require.NoError(t, err)
	require.Equal(t, saved.SourceBundleRef, preparedEquivalent.SourceBundleRef)

	require.NoError(t, repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision))
	previousHash := saved.SourceBundleRef
	saved.OutputSchemaJSON = `{"type":"object","required":["result"],"properties":{"result":{"type":"string"}}}`
	updated, err := repo.SaveDraftCAS(ctx, saved, saved.Revision)
	require.NoError(t, err)
	require.NotEqual(t, previousHash, updated.SourceBundleRef)
	require.Equal(t, int64(2), updated.Revision)
	require.Equal(t, int64(1), updated.LastDebuggedRevision)
	require.NotEqual(t, updated.Revision, updated.LastDebuggedRevision)

	require.ErrorIs(t, repo.PublishDebuggedVersion(ctx, updated.PluginID, "2.0.0", 99), ErrCodeDraftNotDebugged)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, updated.PluginID, updated.Revision))
	require.NoError(t, repo.PublishDebuggedVersion(ctx, updated.PluginID, "2.0.0", 99))
	version, exists, err := repo.GetVersion(ctx, updated.PluginID, "2.0.0")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, updated.InputSchemaJSON, version.InputSchemaJSON)
	require.Equal(t, updated.OutputSchemaJSON, version.OutputSchemaJSON)
	require.Equal(t, updated.SourceBundleRef, version.SourceBundleRef)

	require.NoError(t, repo.CopyDraft(ctx, updated.PluginID, 210, 20))
	copied, exists, err := repo.GetDraft(ctx, 210)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, updated.InputSchemaJSON, copied.InputSchemaJSON)
	require.Equal(t, updated.OutputSchemaJSON, copied.OutputSchemaJSON)
	require.Equal(t, updated.SourceBundleRef, copied.SourceBundleRef)
	require.Zero(t, copied.LastDebuggedRevision)

	versionOutputSchema := version.OutputSchemaJSON
	updated.OutputSchemaJSON = `{"type":"object","properties":{"changed":{"type":"boolean"}}}`
	_, err = repo.SaveDraftCAS(ctx, updated, updated.Revision)
	require.NoError(t, err)
	version, exists, err = repo.GetVersion(ctx, updated.PluginID, "2.0.0")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, versionOutputSchema, version.OutputSchemaJSON)
}

func TestCodePluginRepositoryRejectsInvalidSchemas(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		input  string
		output string
	}{
		{name: "invalid json", input: "{"},
		{name: "array input", input: "[]"},
		{name: "null output", output: "null"},
		{name: "scalar output", output: `"value"`},
		{
			name:  "oversized input",
			input: `{"description":"` + strings.Repeat("x", entity.MaxCodeSchemaSize) + `"}`,
		},
	}
	for index, testCase := range testCases {
		index, testCase := index, testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			draft := codeDraftFixture(int64(300+index), 10, "return input")
			draft.InputSchemaJSON = testCase.input
			draft.OutputSchemaJSON = testCase.output
			_, err := entity.PrepareCodeDraft(draft)
			require.Error(t, err)
		})
	}
}

func TestCodePluginRepositoryHydratesLegacySchemaDefaults(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 111)
	ctx := context.Background()

	saved, err := repo.SaveDraftCAS(ctx, codeDraftFixture(111, 10, "return input"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.db.Model(&codeDraftPO{}).
		Where("plugin_id = ?", saved.PluginID).
		Update("source_bundle_ref", "legacy-bundle-hash").Error)

	hydrated, exists, err := repo.GetDraft(ctx, saved.PluginID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, entity.DefaultCodeSchemaJSON, hydrated.InputSchemaJSON)
	require.Equal(t, entity.DefaultCodeSchemaJSON, hydrated.OutputSchemaJSON)
	require.NotEqual(t, "legacy-bundle-hash", hydrated.SourceBundleRef)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision))
	require.NoError(t, repo.PublishDebuggedVersion(ctx, saved.PluginID, "legacy-default", 99))
	version, exists, err := repo.GetVersion(ctx, saved.PluginID, "legacy-default")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, hydrated.SourceBundleRef, version.SourceBundleRef)
}

func TestCodePluginRepositoryValidatesFiles(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 104, 105, 106)
	ctx := context.Background()

	invalidPath := codeDraftFixture(104, 10, "return input")
	invalidPath.Files[0].Path = "../handler.py"
	_, err := repo.SaveDraftCAS(ctx, invalidPath, 0)
	require.ErrorContains(t, err, "parent traversal")

	missingEntry := codeDraftFixture(105, 10, "return input")
	missingEntry.EntryFile = "missing.py"
	_, err = repo.SaveDraftCAS(ctx, missingEntry, 0)
	require.ErrorContains(t, err, "does not exist")

	oversized := codeDraftFixture(106, 10, "")
	oversized.Files[0].Content = bytes.Repeat([]byte("x"), entity.MaxCodeBundleSize+1)
	_, err = repo.SaveDraftCAS(ctx, oversized, 0)
	require.ErrorContains(t, err, "exceeds")
}

func TestCodePluginRepositoryDeletePluginData(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 107)
	ctx := context.Background()

	saved, err := repo.SaveDraftCAS(ctx, codeDraftFixture(107, 10, "return input"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision))
	require.NoError(t, repo.PublishDebuggedVersion(ctx, saved.PluginID, "1.0.0", 99))
	require.NoError(t, repo.DeletePluginData(ctx, saved.PluginID))

	_, exists, err := repo.GetDraft(ctx, saved.PluginID)
	require.NoError(t, err)
	require.False(t, exists)
	_, exists, err = repo.GetVersion(ctx, saved.PluginID, "1.0.0")
	require.NoError(t, err)
	require.False(t, exists)
	for _, model := range []any{
		&codeDraftPO{},
		&codeDraftFilePO{},
		&codeVersionPO{},
		&codeVersionFilePO{},
	} {
		var count int64
		require.NoError(t, repo.db.Model(model).Where("plugin_id = ?", saved.PluginID).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestCodePluginRepositoryParentDeleteCascadesAllCodeRows(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 118)
	ctx := context.Background()
	draft, err := repo.SaveDraftCAS(ctx, codeDraftFixture(118, 10, "return input"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))
	require.NoError(t, repo.PublishDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 99))
	require.NoError(t, repo.db.Delete(&codePluginParentFixture{}, draft.PluginID).Error)
	for _, model := range []any{&codeDraftPO{}, &codeDraftFilePO{}, &codeVersionPO{}, &codeVersionFilePO{}} {
		var count int64
		require.NoError(t, repo.db.Model(model).Where("plugin_id = ?", draft.PluginID).Count(&count).Error)
		require.Zero(t, count)
	}
}

func TestCodePluginRepositoryMarkDebuggedRejectsStaleRevision(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 108)
	ctx := context.Background()

	saved, err := repo.SaveDraftCAS(ctx, codeDraftFixture(108, 10, "return input"), 0)
	require.NoError(t, err)
	require.True(t, errors.Is(repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision+1), ErrCodeDraftConflict))
}

func TestCodePluginRepositoryMarkDebuggedIsIdempotent(t *testing.T) {
	t.Parallel()
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 109)
	ctx := context.Background()

	saved, err := repo.SaveDraftCAS(ctx, codeDraftFixture(109, 10, "return input"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision))
	require.NoError(t, repo.MarkDebuggedCAS(ctx, saved.PluginID, saved.Revision))

	got, exists, err := repo.GetDraft(ctx, saved.PluginID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, saved.Revision, got.LastDebuggedRevision)
}

func TestCodePluginPathRejectsNonCanonicalValues(t *testing.T) {
	t.Parallel()
	invalidPaths := []string{
		" handler.py ",
		"C:/handler.py",
		"C:handler.py",
		"dir\\handler.py",
		"handler\x00.py",
		"handler\n.py",
		"dir//handler.py",
		"dir/./handler.py",
		"dir/../handler.py",
		"dir/",
		"/handler.py",
	}
	for _, invalidPath := range invalidPaths {
		invalidPath := invalidPath
		t.Run(invalidPath, func(t *testing.T) {
			t.Parallel()
			draft := codeDraftFixture(200, 10, "return input")
			draft.EntryFile = invalidPath
			draft.Files[0].Path = invalidPath
			_, err := entity.PrepareCodeDraft(draft)
			require.Error(t, err)
		})
	}
}

func TestCodePluginRepositorySchemaMatchesMigration(t *testing.T) {
	t.Parallel()

	draftType := reflect.TypeOf(codeDraftPO{})
	draftSpace, ok := draftType.FieldByName("SpaceID")
	require.True(t, ok)
	require.Equal(t, "column:space_id;index:idx_plugin_code_drafts_space,priority:1", draftSpace.Tag.Get("gorm"))
	draftUpdatedAt, ok := draftType.FieldByName("UpdatedAt")
	require.True(t, ok)
	require.Equal(t, "column:updated_at;index:idx_plugin_code_drafts_space,priority:2", draftUpdatedAt.Tag.Get("gorm"))

	versionType := reflect.TypeOf(codeVersionPO{})
	versionSpace, ok := versionType.FieldByName("SpaceID")
	require.True(t, ok)
	require.Equal(t, "column:space_id;index:idx_plugin_code_versions_space,priority:1", versionSpace.Tag.Get("gorm"))
	versionCreatedAt, ok := versionType.FieldByName("CreatedAt")
	require.True(t, ok)
	require.Equal(t, "column:created_at;index:idx_plugin_code_versions_space,priority:2", versionCreatedAt.Tag.Get("gorm"))
	for _, modelAndFields := range []struct {
		model  reflect.Type
		fields []string
	}{
		{model: draftType, fields: []string{"InputSchemaJSON", "OutputSchemaJSON"}},
		{model: versionType, fields: []string{"InputSchemaJSON", "OutputSchemaJSON"}},
	} {
		for _, fieldName := range modelAndFields.fields {
			field, exists := modelAndFields.model.FieldByName(fieldName)
			require.True(t, exists)
			require.Contains(t, field.Tag.Get("gorm"), "_schema_json")
		}
	}

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	migrationPath := filepath.Join(filepath.Dir(currentFile), "../../../../docker/atlas/migrations/20260718000100_plugin_code_parity.sql")
	migration, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	migrationSQL := string(migration)
	for _, expected := range []string{
		"KEY `idx_plugin_code_drafts_space` (`space_id`, `updated_at`)",
		"KEY `idx_plugin_code_versions_space` (`space_id`, `created_at`)",
		"CONSTRAINT `fk_plugin_code_drafts_plugin_draft` FOREIGN KEY (`plugin_id`) REFERENCES `plugin_draft` (`id`) ON DELETE CASCADE",
		"CONSTRAINT `fk_plugin_code_draft_files_draft` FOREIGN KEY (`plugin_id`) REFERENCES `plugin_code_drafts` (`plugin_id`) ON DELETE CASCADE",
		"CONSTRAINT `fk_plugin_code_versions_draft` FOREIGN KEY (`plugin_id`) REFERENCES `plugin_code_drafts` (`plugin_id`) ON DELETE CASCADE",
		"CONSTRAINT `fk_plugin_code_version_files_version` FOREIGN KEY (`plugin_id`, `version`) REFERENCES `plugin_code_versions` (`plugin_id`, `version`) ON DELETE CASCADE",
		"`input_schema_json` json NOT NULL DEFAULT (JSON_OBJECT('type', 'object', 'properties', JSON_OBJECT()))",
		"`output_schema_json` json NOT NULL DEFAULT (JSON_OBJECT('type', 'object', 'properties', JSON_OBJECT()))",
	} {
		require.Contains(t, migrationSQL, expected)
	}
}

func TestCodePluginRepositoryRetriesOnlyRetryableWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	attempts := 0
	err := retryCodePluginWrite(ctx, func() error {
		attempts++
		if attempts < codePluginWriteMaxAttempts {
			return &mysqldriver.MySQLError{Number: 1213, Message: "deadlock"}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, codePluginWriteMaxAttempts, attempts)

	nonRetryable := errors.New("permanent failure")
	attempts = 0
	err = retryCodePluginWrite(ctx, func() error {
		attempts++
		return nonRetryable
	})
	require.ErrorIs(t, err, nonRetryable)
	require.Equal(t, 1, attempts)

	require.ErrorIs(t,
		mapCodeDraftCreateError(&mysqldriver.MySQLError{Number: 1062, Message: "duplicate"}),
		ErrCodeDraftConflict,
	)
}

type codePluginParentFixture struct {
	ID      int64 `gorm:"column:id;primaryKey"`
	SpaceID int64 `gorm:"column:space_id"`
}

func (*codePluginParentFixture) TableName() string { return "plugin_draft" }

func newCodePluginTestRepository(t *testing.T) *codePluginRepositoryImpl {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	for _, statement := range []string{
		`CREATE TABLE plugin_draft (
				id INTEGER NOT NULL PRIMARY KEY,
				space_id INTEGER NOT NULL
		)`,
		`CREATE TABLE plugin_version (
			plugin_id INTEGER NOT NULL,
			version TEXT NOT NULL,
			PRIMARY KEY (plugin_id, version),
			FOREIGN KEY (plugin_id) REFERENCES plugin_draft (id) ON DELETE CASCADE
		)`,
		`CREATE TABLE plugin_code_drafts (
			plugin_id INTEGER NOT NULL PRIMARY KEY,
			space_id INTEGER NOT NULL,
			runtime TEXT NOT NULL,
			entry_file TEXT NOT NULL,
			source_bundle_ref TEXT NOT NULL,
			input_schema_json TEXT NOT NULL DEFAULT '{"type":"object","properties":{}}',
			output_schema_json TEXT NOT NULL DEFAULT '{"type":"object","properties":{}}',
			revision INTEGER NOT NULL,
			last_debugged_revision INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (plugin_id) REFERENCES plugin_draft (id) ON DELETE CASCADE
		)`,
		`CREATE INDEX idx_plugin_code_drafts_space
			ON plugin_code_drafts (space_id, updated_at)`,
		`CREATE TABLE plugin_code_draft_files (
			plugin_id INTEGER NOT NULL,
			path TEXT NOT NULL,
			content BLOB NOT NULL,
			size INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (plugin_id, path),
			FOREIGN KEY (plugin_id) REFERENCES plugin_code_drafts (plugin_id) ON DELETE CASCADE
		)`,
		`CREATE TABLE plugin_code_versions (
			plugin_id INTEGER NOT NULL,
			version TEXT NOT NULL,
			space_id INTEGER NOT NULL,
			runtime TEXT NOT NULL,
			entry_file TEXT NOT NULL,
			source_bundle_ref TEXT NOT NULL,
			input_schema_json TEXT NOT NULL DEFAULT '{"type":"object","properties":{}}',
			output_schema_json TEXT NOT NULL DEFAULT '{"type":"object","properties":{}}',
			source_revision INTEGER NOT NULL,
			created_by INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			PRIMARY KEY (plugin_id, version),
			FOREIGN KEY (plugin_id) REFERENCES plugin_code_drafts (plugin_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX idx_plugin_code_versions_space
			ON plugin_code_versions (space_id, created_at)`,
		`CREATE TABLE plugin_code_version_files (
			plugin_id INTEGER NOT NULL,
			version TEXT NOT NULL,
			path TEXT NOT NULL,
			content BLOB NOT NULL,
			size INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			PRIMARY KEY (plugin_id, version, path),
			FOREIGN KEY (plugin_id, version)
				REFERENCES plugin_code_versions (plugin_id, version) ON DELETE CASCADE
		)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	return NewCodePluginRepository(db).(*codePluginRepositoryImpl)
}

func seedCodePluginParents(t *testing.T, repo *codePluginRepositoryImpl, pluginIDs ...int64) {
	t.Helper()
	for _, pluginID := range pluginIDs {
		seedCodePluginParentInSpace(t, repo, pluginID, 10)
	}
}

func seedCodePluginParentInSpace(t *testing.T, repo *codePluginRepositoryImpl, pluginID, spaceID int64) {
	t.Helper()
	require.NoError(t, repo.db.Create(&codePluginParentFixture{ID: pluginID, SpaceID: spaceID}).Error)
}

func TestCodePluginRepositoryRejectsCrossSpaceDraftVersionAndCopy(t *testing.T) {
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 301, 302)
	ctx := context.Background()

	_, err := repo.SaveDraftCAS(ctx, codeDraftFixture(301, 999, "return mismatch"), 0)
	require.ErrorIs(t, err, ErrCodeSpaceMismatch)

	source, err := repo.SaveDraftCAS(ctx, codeDraftFixture(301, 10, "return source"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, source.PluginID, source.Revision))
	require.ErrorIs(t, repo.CopyDraft(ctx, 301, 302, 999), ErrCodeSpaceMismatch)

	require.NoError(t, repo.db.Model(&codeDraftPO{}).Where("plugin_id = ?", 301).Update("space_id", 999).Error)
	_, err = repo.PrepareDebuggedVersion(ctx, 301, "v1.0.0", 88)
	require.ErrorIs(t, err, ErrCodeSpaceMismatch)
}

func TestCodePluginRepositoryGetVersionRejectsCrossSpaceSnapshot(t *testing.T) {
	repo := newCodePluginTestRepository(t)
	seedCodePluginParents(t, repo, 303)
	ctx := context.Background()
	draft, err := repo.SaveDraftCAS(ctx, codeDraftFixture(303, 10, "return source"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(ctx, draft.PluginID, draft.Revision))
	require.NoError(t, repo.PublishDebuggedVersion(ctx, draft.PluginID, "v1.0.0", 88))
	require.NoError(t, repo.db.Model(&codeVersionPO{}).
		Where("plugin_id = ? AND version = ?", 303, "v1.0.0").
		Update("space_id", 999).Error)

	_, _, err = repo.GetVersion(ctx, 303, "v1.0.0")
	require.ErrorIs(t, err, ErrCodeSpaceMismatch)
}

func codeDraftFixture(pluginID, spaceID int64, content string) *entity.CodeDraft {
	return &entity.CodeDraft{
		PluginID:  pluginID,
		SpaceID:   spaceID,
		Runtime:   entity.CodeRuntimePython,
		EntryFile: "handler.py",
		Files: []*entity.CodeFile{
			{Path: "handler.py", Content: []byte(content)},
		},
	}
}
