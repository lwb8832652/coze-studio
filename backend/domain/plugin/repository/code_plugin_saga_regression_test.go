// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
)

func TestPreparePublishedCodeVersionIgnoresNewDraft(t *testing.T) {
	repo := newCodePluginTestRepository(t)
	const pluginID int64 = 9101
	seedCodePluginParents(t, repo, pluginID)

	first, err := repo.SaveDraftCAS(context.Background(), regressionCodeDraft(pluginID, 10, "print('v1')"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(context.Background(), pluginID, first.Revision))

	prepared, err := repo.PrepareDebuggedVersion(context.Background(), pluginID, "1.0.0", 7)
	require.NoError(t, err)
	require.NoError(t, repo.db.Create(&publishedPluginVersionPO{PluginID: pluginID, Version: "1.0.0"}).Error)

	_, err = repo.SaveDraftCAS(context.Background(), regressionCodeDraft(pluginID, 10, "print('v2')"), first.Revision)
	require.NoError(t, err)

	retried, err := repo.PrepareDebuggedVersion(context.Background(), pluginID, "1.0.0", 7)
	require.NoError(t, err)
	require.Equal(t, CodeVersionAlreadyPublished, retried.State)
	require.Equal(t, prepared.Version.SourceBundleRef, retried.Version.SourceBundleRef)
	require.Equal(t, prepared.Version.SourceRevision, retried.Version.SourceRevision)
}

func TestCodeVersionSagaRejectsExistingCrossSpaceRows(t *testing.T) {
	repo := newCodePluginTestRepository(t)
	const pluginID int64 = 9102
	seedCodePluginParentInSpace(t, repo, pluginID, 11)

	draft, err := repo.SaveDraftCAS(context.Background(), regressionCodeDraft(pluginID, 11, "print('safe')"), 0)
	require.NoError(t, err)
	require.NoError(t, repo.MarkDebuggedCAS(context.Background(), pluginID, draft.Revision))

	prepared, err := repo.PrepareDebuggedVersion(context.Background(), pluginID, "1.0.0", 7)
	require.NoError(t, err)
	require.NoError(t, repo.db.Model(&codeVersionPO{}).
		Where("plugin_id = ? AND version = ?", pluginID, "1.0.0").
		Update("space_id", int64(99)).Error)

	_, err = repo.PrepareDebuggedVersion(context.Background(), pluginID, "1.0.0", 7)
	require.ErrorIs(t, err, ErrCodeSpaceMismatch)
	_, err = repo.CompensatePreparedVersion(context.Background(), prepared)
	require.ErrorIs(t, err, ErrCodeSpaceMismatch)
}

func regressionCodeDraft(pluginID, spaceID int64, source string) *entity.CodeDraft {
	return &entity.CodeDraft{
		PluginID:         pluginID,
		SpaceID:          spaceID,
		Runtime:          entity.CodeRuntimePython,
		EntryFile:        "main.py",
		InputSchemaJSON:  entity.DefaultCodeSchemaJSON,
		OutputSchemaJSON: entity.DefaultCodeSchemaJSON,
		Files:            []*entity.CodeFile{{Path: "main.py", Content: []byte(source)}},
	}
}
