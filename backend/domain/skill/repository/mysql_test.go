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

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
)

func TestSkillRepositoryCreateAndGet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skillPO{}))

	repo := NewSkillRepository(db, fixedIDGen{})
	skill := &entity.Skill{
		ID:           1,
		SpaceID:      10,
		Name:         "Weekly Report",
		Description:  "build report",
		Type:         entity.TypeScript,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  `{"type":"object"}`,
		OutputSchema: `{"type":"object"}`,
		Executor:     `{"language":"python","entry":"main.py"}`,
		Permissions:  `{"network":false}`,
	}

	require.NoError(t, repo.Create(context.Background(), skill))
	got, err := repo.Get(context.Background(), 1)

	require.NoError(t, err)
	require.Equal(t, entity.TypeScript, got.Type)
	require.Equal(t, "Weekly Report", got.Name)
	require.Equal(t, `{"type":"object"}`, got.InputSchema)
	require.Equal(t, `{"language":"python","entry":"main.py"}`, got.Executor)
}

func TestSkillRepositoryListAndUpdate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skillPO{}))

	repo := NewSkillRepository(db, fixedIDGen{})
	require.NoError(t, repo.Create(context.Background(), &entity.Skill{
		ID:           1,
		SpaceID:      10,
		Name:         "Script",
		Description:  "script skill",
		Type:         entity.TypeScript,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  `{"type":"object"}`,
		OutputSchema: `{"type":"object"}`,
		Executor:     `{"language":"python","entry":"main.py"}`,
		Permissions:  `{"network":false}`,
		UpdatedAt:    2,
	}))
	require.NoError(t, repo.Create(context.Background(), &entity.Skill{
		ID:           2,
		SpaceID:      10,
		Name:         "Workflow",
		Description:  "workflow skill",
		Type:         entity.TypeWorkflow,
		Version:      "1.0.0",
		Enabled:      false,
		InputSchema:  `{"type":"object"}`,
		OutputSchema: `{"type":"object"}`,
		Executor:     `{"workflow_id":"100"}`,
		Permissions:  `{"network":false}`,
		UpdatedAt:    1,
	}))

	typ := entity.TypeScript
	enabled := true
	items, err := repo.List(context.Background(), 10, &typ, &enabled)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(1), items[0].ID)

	items[0].Name = "Script Updated"
	items[0].Enabled = false
	require.NoError(t, repo.Update(context.Background(), items[0]))

	got, err := repo.Get(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "Script Updated", got.Name)
	require.False(t, got.Enabled)
}

func TestSkillRepositoryDefaultsEmptyJSONFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skillPO{}))

	repo := NewSkillRepository(db, fixedIDGen{})
	require.NoError(t, repo.Create(context.Background(), &entity.Skill{
		ID:      1,
		SpaceID: 10,
		Name:    "Empty JSON",
		Type:    entity.TypeScript,
		Version: "1.0.0",
		Enabled: true,
	}))

	got, err := repo.Get(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, "{}", got.InputSchema)
	require.Equal(t, "{}", got.OutputSchema)
	require.Equal(t, "{}", got.Executor)
	require.Equal(t, "{}", got.Permissions)
}

func TestSkillRepositoryRejectsInvalidJSON(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skillPO{}))

	repo := NewSkillRepository(db, fixedIDGen{})
	err = repo.Create(context.Background(), &entity.Skill{
		ID:          1,
		SpaceID:     10,
		Name:        "Bad JSON",
		Type:        entity.TypeScript,
		Version:     "1.0.0",
		Enabled:     true,
		InputSchema: "{",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "input_schema")
}

func TestSkillRepositoryUpdateMissingSkillReturnsError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skillPO{}))

	repo := NewSkillRepository(db, fixedIDGen{})
	err = repo.Update(context.Background(), &entity.Skill{
		ID:           404,
		SpaceID:      10,
		Name:         "Missing",
		Type:         entity.TypeScript,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  "{}",
		OutputSchema: "{}",
		Executor:     "{}",
		Permissions:  "{}",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestSkillRepositoryListUsesStableOrdering(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skillPO{}))

	repo := NewSkillRepository(db, fixedIDGen{})
	for _, skill := range []*entity.Skill{
		{
			ID:           1,
			SpaceID:      10,
			Name:         "first",
			Type:         entity.TypeScript,
			Version:      "1.0.0",
			Enabled:      true,
			InputSchema:  "{}",
			OutputSchema: "{}",
			Executor:     "{}",
			Permissions:  "{}",
			UpdatedAt:    1,
		},
		{
			ID:           2,
			SpaceID:      10,
			Name:         "second",
			Type:         entity.TypeScript,
			Version:      "1.0.0",
			Enabled:      true,
			InputSchema:  "{}",
			OutputSchema: "{}",
			Executor:     "{}",
			Permissions:  "{}",
			UpdatedAt:    1,
		},
	} {
		require.NoError(t, repo.Create(context.Background(), skill))
	}

	items, err := repo.List(context.Background(), 10, nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, int64(2), items[0].ID)
	require.Equal(t, int64(1), items[1].ID)
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
