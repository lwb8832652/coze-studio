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

package agentthread

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	skillentity "github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	skilldomain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

func TestRuntimeSkillProviderLoadsSelectedEnabledPromptSkills(t *testing.T) {
	catalog := &recordingRuntimeSkillCatalog{
		skills: []*skillentity.Skill{
			{
				ID:          101,
				SpaceID:     7,
				Name:        "weekly-research",
				Description: "Research weekly changes.",
				Type:        skillentity.TypeDeerSkill,
				Version:     "1.2.0",
				Enabled:     true,
			},
			{
				ID:      102,
				SpaceID: 7,
				Name:    "legacy-script",
				Type:    skillentity.TypeScript,
				Enabled: true,
			},
			{
				ID:      103,
				SpaceID: 7,
				Name:    "disabled-custom",
				Type:    skillentity.TypeCustomSkill,
				Enabled: false,
			},
		},
		versions: map[int64][]*skillentity.SkillVersion{
			101: {
				{
					ID:      201,
					SkillID: 101,
					Version: "1.2.0",
					SkillMD: `---
name: weekly-research
description: Research weekly changes.
type: deer_skill
version: 1.2.0
enabled: true
context: fork
agent: research-agent
model: reasoning-model
---
# Weekly Research

Collect sources and produce a concise brief.
`,
				},
			},
		},
	}
	provider := NewRuntimeSkillProvider(catalog)

	skills, err := provider.Load(context.Background(), &RunSummary{
		SpaceID: 7,
		Config:  `{"enable_skills":["weekly-research","legacy-preset"]}`,
	})

	require.NoError(t, err)
	require.NotNil(t, catalog.enabled)
	require.True(t, *catalog.enabled)
	require.Equal(t, int64(7), catalog.spaceID)
	require.Len(t, skills, 1)
	require.Equal(t, int64(101), skills[0].ID)
	require.Equal(t, "weekly-research", skills[0].Name)
	require.Equal(t, "deer_skill", skills[0].Type)
	require.Equal(t, "1.2.0", skills[0].Version)
	require.Equal(t, "fork", skills[0].Context)
	require.Equal(t, "research-agent", skills[0].Agent)
	require.Equal(t, "reasoning-model", skills[0].Model)
	require.Contains(t, skills[0].Body, "Collect sources")
	require.NotContains(t, skills[0].Body, "name: weekly-research")
	require.Equal(t, []int64{101}, catalog.listVersionSkillIDs)
}

type recordingRuntimeSkillCatalog struct {
	skilldomain.SkillService
	skills              []*skillentity.Skill
	versions            map[int64][]*skillentity.SkillVersion
	spaceID             int64
	enabled             *bool
	listVersionSkillIDs []int64
}

func (c *recordingRuntimeSkillCatalog) List(ctx context.Context, spaceID int64, typ *skillentity.Type, enabled *bool) ([]*skillentity.Skill, error) {
	c.spaceID = spaceID
	c.enabled = enabled
	return c.skills, nil
}

func (c *recordingRuntimeSkillCatalog) ListVersions(ctx context.Context, skillID int64) ([]*skillentity.SkillVersion, error) {
	c.listVersionSkillIDs = append(c.listVersionSkillIDs, skillID)
	return c.versions[skillID], nil
}
