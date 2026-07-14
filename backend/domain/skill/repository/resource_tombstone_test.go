/*
 * Copyright 2025 coze-dev Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
)

func TestListResourcesFiltersInternalDeletionTombstones(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skillResourcePO{}))
	require.NoError(t, db.Create([]*skillResourcePO{
		{
			ID: 1, SkillID: 10, VersionID: 20, Path: "references/live.md",
			Content: []byte("visible"), Size: 7, SHA256: "live", CreatedAt: 1,
		},
		{
			ID: 2, SkillID: 10, VersionID: 20, Path: "references/deleted.md",
			Content: []byte(entity.ResourceTombstoneContent), Size: 33, SHA256: "deleted", CreatedAt: 1,
		},
	}).Error)

	repo := NewSkillRepository(db, nil)
	resources, err := repo.ListResources(context.Background(), 10, 20)
	require.NoError(t, err)
	require.Len(t, resources, 1)
	require.Equal(t, "references/live.md", resources[0].Path)
}

func TestSkillResourceToPOPersistsEmptyFilesAsNonNullContent(t *testing.T) {
	po := skillResourceToPO(&entity.SkillResource{Content: []byte{}})
	require.NotNil(t, po.Content)
	require.Empty(t, po.Content)
}
