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

package skill

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	domain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

func TestDeerFlowBuiltinSkillServiceSeedsAllPublicSkills(t *testing.T) {
	catalog := newRecordingBuiltinSkillCatalog()
	service := newDeerFlowBuiltinSkillService(catalog)

	skills, err := service.List(context.Background(), 7656103552997130240, nil, nil)

	require.NoError(t, err)
	require.Len(t, skills, 22)
	require.Equal(t, 22, catalog.importCount)
	require.Equal(t, 69, catalog.totalResources)
	require.Contains(t, catalog.skillNames(), "academic-paper-review")
	require.Contains(t, catalog.skillNames(), "bootstrap")
	require.Contains(t, catalog.skillNames(), "chart-visualization")
	require.Contains(t, catalog.skillNames(), "deep-research")
	require.Contains(t, catalog.skillNames(), "skill-creator")
	require.Contains(t, catalog.skillNames(), "vercel-deploy")
	require.NotContains(t, catalog.skillNames(), "vercel-deploy-claimable")
	require.Contains(t, catalog.resourcePaths["chart-visualization"], "scripts/generate.js")
	require.Contains(t, catalog.resourcePaths["skill-creator"], "eval-viewer/viewer.html")
	require.Contains(t, catalog.resourcePaths["bootstrap"], "templates/SOUL.template.md")

	for _, item := range skills {
		require.Equal(t, int64(7656103552997130240), item.SpaceID)
		require.Equal(t, entity.TypeDeerSkill, item.Type)
		require.True(t, item.Enabled)
		require.NotEmpty(t, item.Description)
		require.JSONEq(t, `{}`, item.InputSchema)
		require.JSONEq(t, `{}`, item.OutputSchema)
	}

	_, err = service.List(context.Background(), 7656103552997130240, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 22, catalog.importCount)

	serviceAfterRestart := newDeerFlowBuiltinSkillService(catalog)
	_, err = serviceAfterRestart.List(context.Background(), 7656103552997130240, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 22, catalog.importCount)
}

func TestDeerFlowBuiltinSkillServiceDoesNotReplaceExistingSkillName(t *testing.T) {
	catalog := newRecordingBuiltinSkillCatalog()
	catalog.skills["deep-research"] = &entity.Skill{
		ID:           99,
		SpaceID:      1,
		Name:         "deep-research",
		Description:  "Workspace override.",
		Type:         entity.TypeCustomSkill,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  `{}`,
		OutputSchema: `{}`,
		Executor:     `{}`,
		Permissions:  `{}`,
	}
	service := newDeerFlowBuiltinSkillService(catalog)

	skills, err := service.List(context.Background(), 1, nil, nil)

	require.NoError(t, err)
	require.Len(t, skills, 22)
	require.Equal(t, 21, catalog.importCount)
	require.Equal(t, entity.TypeCustomSkill, catalog.skills["deep-research"].Type)
	require.Equal(t, "Workspace override.", catalog.skills["deep-research"].Description)
}

type recordingBuiltinSkillCatalog struct {
	domain.SkillService
	nextID         int64
	skills         map[string]*entity.Skill
	versions       map[int64][]*entity.SkillVersion
	resourcePaths  map[string][]string
	importCount    int
	totalResources int
}

func newRecordingBuiltinSkillCatalog() *recordingBuiltinSkillCatalog {
	return &recordingBuiltinSkillCatalog{
		nextID:        1000,
		skills:        map[string]*entity.Skill{},
		versions:      map[int64][]*entity.SkillVersion{},
		resourcePaths: map[string][]string{},
	}
}

func (c *recordingBuiltinSkillCatalog) ImportDeclaration(ctx context.Context, spaceID int64, fileName string, content []byte) (*entity.Skill, error) {
	return c.ImportDeclarationWithDefaultType(ctx, spaceID, fileName, content, entity.TypeDeerSkill)
}

func (c *recordingBuiltinSkillCatalog) ImportDeclarationWithDefaultType(ctx context.Context, spaceID int64, fileName string, content []byte, defaultType entity.Type) (*entity.Skill, error) {
	decl, err := domain.ParseDeclarationWithDefaultType(fileName, content, defaultType)
	if err != nil {
		return nil, err
	}

	c.nextID++
	skillID := c.nextID
	now := time.Now().UnixMilli()
	inputSchema, err := json.Marshal(decl.InputSchema)
	if err != nil {
		return nil, err
	}
	outputSchema, err := json.Marshal(decl.OutputSchema)
	if err != nil {
		return nil, err
	}
	executor, err := json.Marshal(decl.Executor)
	if err != nil {
		return nil, err
	}
	permissions, err := json.Marshal(decl.Permissions)
	if err != nil {
		return nil, err
	}

	skill := &entity.Skill{
		ID:           skillID,
		SpaceID:      spaceID,
		Name:         decl.Name,
		Description:  decl.Description,
		Type:         entity.Type(decl.Type),
		Version:      decl.Version,
		Enabled:      decl.Enabled,
		InputSchema:  string(inputSchema),
		OutputSchema: string(outputSchema),
		Executor:     string(executor),
		Permissions:  string(permissions),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	c.skills[skill.Name] = skill

	c.nextID++
	version := &entity.SkillVersion{
		ID:           c.nextID,
		SkillID:      skillID,
		Version:      skill.Version,
		SkillMD:      decl.SkillMD,
		InputSchema:  skill.InputSchema,
		OutputSchema: skill.OutputSchema,
		Executor:     skill.Executor,
		Permissions:  skill.Permissions,
		CreatedAt:    now,
	}
	c.versions[skillID] = append(c.versions[skillID], version)
	for _, resource := range decl.Resources {
		c.resourcePaths[skill.Name] = append(c.resourcePaths[skill.Name], resource.Path)
		c.totalResources++
	}
	sort.Strings(c.resourcePaths[skill.Name])
	c.importCount++

	return skill, nil
}

func (c *recordingBuiltinSkillCatalog) List(ctx context.Context, spaceID int64, typ *entity.Type, enabled *bool) ([]*entity.Skill, error) {
	result := make([]*entity.Skill, 0, len(c.skills))
	for _, skill := range c.skills {
		if skill.SpaceID != spaceID {
			continue
		}
		if typ != nil && skill.Type != *typ {
			continue
		}
		if enabled != nil && skill.Enabled != *enabled {
			continue
		}
		result = append(result, skill)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (c *recordingBuiltinSkillCatalog) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	return append([]*entity.SkillVersion(nil), c.versions[skillID]...), nil
}

func (c *recordingBuiltinSkillCatalog) skillNames() []string {
	names := make([]string, 0, len(c.skills))
	for name := range c.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
