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
	"encoding/json"
	"strconv"
	"strings"
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

func TestRuntimeSkillProviderTreatsSlashSkillPrefixAsTurnScopedSelector(t *testing.T) {
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
				ID:          102,
				SpaceID:     7,
				Name:        "frontend-designer",
				Description: "Design frontend pages.",
				Type:        skillentity.TypeDeerSkill,
				Version:     "1.0.0",
				Enabled:     true,
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
---
# Weekly Research

Collect sources and produce a concise brief.
`,
				},
			},
			102: {
				{
					ID:      202,
					SkillID: 102,
					Version: "1.0.0",
					SkillMD: `---
name: frontend-designer
description: Design frontend pages.
type: deer_skill
version: 1.0.0
enabled: true
---
# Frontend Designer

Design polished frontend pages.
`,
				},
			},
		},
	}
	provider := NewRuntimeSkillProvider(catalog)

	skills, err := provider.Load(context.Background(), &RunSummary{
		SpaceID: 7,
		Input:   `{"messages":[{"role":"user","content":"/weekly-research prepare the weekly report"}]}`,
		Config:  `{}`,
	})

	require.NoError(t, err)
	require.Len(t, skills, 1)
	require.Equal(t, "weekly-research", skills[0].Name)
	require.Equal(t, []int64{101}, catalog.listVersionSkillIDs)
}

func TestRuntimeSkillProviderAllowsSlashSkillWhenExplicitSelectorUsesSkillID(t *testing.T) {
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
				ID:          102,
				SpaceID:     7,
				Name:        "frontend-designer",
				Description: "Design frontend pages.",
				Type:        skillentity.TypeDeerSkill,
				Version:     "1.0.0",
				Enabled:     true,
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
		Input:   `{"messages":[{"role":"user","content":"/weekly-research prepare the weekly report"}]}`,
		Config:  `{"enable_skills":["101"]}`,
	})

	require.NoError(t, err)
	require.Len(t, skills, 1)
	require.Equal(t, "weekly-research", skills[0].Name)
	require.Equal(t, []int64{101}, catalog.listVersionSkillIDs)
}

func TestRuntimeSkillProviderLoadsImplicitSkillCatalogWithinBudget(t *testing.T) {
	catalog := &recordingRuntimeSkillCatalog{
		skills: []*skillentity.Skill{
			{
				ID:          101,
				SpaceID:     7,
				Name:        "auto-skill-a",
				Description: "Automatically selected skill A.",
				Type:        skillentity.TypeDeerSkill,
				Version:     "1.0.0",
				Enabled:     true,
			},
			{
				ID:          102,
				SpaceID:     7,
				Name:        "auto-skill-b",
				Description: "Automatically selected skill B.",
				Type:        skillentity.TypeDeerSkill,
				Version:     "1.0.0",
				Enabled:     true,
			},
		},
		versions: map[int64][]*skillentity.SkillVersion{
			101: {
				{
					ID:      201,
					SkillID: 101,
					Version: "1.0.0",
					SkillMD: `---
name: auto-skill-a
description: Automatically selected skill A.
type: deer_skill
version: 1.0.0
enabled: true
---
# Auto Skill A

Use this skill when it matches the task.
`,
				},
			},
			102: {
				{
					ID:      202,
					SkillID: 102,
					Version: "1.0.0",
					SkillMD: `---
name: auto-skill-b
description: Automatically selected skill B.
type: deer_skill
version: 1.0.0
enabled: true
---
# Auto Skill B

Use this skill when it matches the task.
`,
				},
			},
		},
	}
	provider := NewRuntimeSkillProvider(catalog)

	skills, err := provider.Load(context.Background(), &RunSummary{
		SpaceID: 7,
		Config:  `{}`,
	})

	require.NoError(t, err)
	require.Len(t, skills, 2)
	require.Equal(t, []int64{101, 102}, catalog.listVersionSkillIDs)
}

func TestRuntimeSkillProviderRejectsImplicitSkillCatalogBeyondSelectionLimit(t *testing.T) {
	const skillCount = defaultRuntimeSkillLimit + 3
	catalog := &recordingRuntimeSkillCatalog{
		skills:   make([]*skillentity.Skill, 0, skillCount),
		versions: make(map[int64][]*skillentity.SkillVersion, skillCount),
	}
	for i := 0; i < skillCount; i++ {
		id := int64(100 + i)
		name := "auto-skill-" + strconv.Itoa(i)
		catalog.skills = append(catalog.skills, &skillentity.Skill{
			ID:          id,
			SpaceID:     7,
			Name:        name,
			Description: "Automatically selected skill.",
			Type:        skillentity.TypeDeerSkill,
			Version:     "1.0.0",
			Enabled:     true,
		})
		catalog.versions[id] = []*skillentity.SkillVersion{
			{
				ID:      id + 1000,
				SkillID: id,
				Version: "1.0.0",
				SkillMD: `---
name: ` + name + `
description: Automatically selected skill.
type: deer_skill
version: 1.0.0
enabled: true
---
# ` + name + `

Use this skill when it matches the task.
`,
			},
		}
	}
	provider := NewRuntimeSkillProvider(catalog)

	skills, err := provider.Load(context.Background(), &RunSummary{
		SpaceID: 7,
		Config:  `{}`,
	})

	require.Nil(t, skills)
	require.ErrorContains(t, err, "selected skill count")
	require.Empty(t, catalog.listVersionSkillIDs)
}

func TestRuntimeSkillProviderRejectsTooManyExplicitSkillSelectors(t *testing.T) {
	const skillCount = defaultRuntimeSkillLimit + 1
	catalog := &recordingRuntimeSkillCatalog{
		skills:   make([]*skillentity.Skill, 0, skillCount),
		versions: make(map[int64][]*skillentity.SkillVersion, skillCount),
	}
	selectors := make([]string, 0, skillCount)
	for i := 0; i < skillCount; i++ {
		id := int64(100 + i)
		name := "selected-skill-" + strconv.Itoa(i)
		selectors = append(selectors, name)
		catalog.skills = append(catalog.skills, &skillentity.Skill{
			ID:      id,
			SpaceID: 7,
			Name:    name,
			Type:    skillentity.TypeDeerSkill,
			Enabled: true,
		})
	}
	rawSelectors, err := json.Marshal(selectors)
	require.NoError(t, err)
	provider := NewRuntimeSkillProvider(catalog)

	skills, err := provider.Load(context.Background(), &RunSummary{
		SpaceID: 7,
		Config:  `{"enable_skills":` + string(rawSelectors) + `}`,
	})

	require.Nil(t, skills)
	require.ErrorContains(t, err, "selected skill count")
	require.Empty(t, catalog.listVersionSkillIDs)
}

func TestRuntimeSkillProviderRejectsNestedAllowedSkillsBeyondExplicitLimit(t *testing.T) {
	const skillCount = defaultRuntimeSkillLimit + 4
	catalog := &recordingRuntimeSkillCatalog{
		skills:   make([]*skillentity.Skill, 0, skillCount),
		versions: make(map[int64][]*skillentity.SkillVersion, skillCount),
	}
	selectors := make([]string, 0, skillCount)
	for i := 0; i < skillCount; i++ {
		id := int64(100 + i)
		name := "allowed-skill-" + strconv.Itoa(i)
		selectors = append(selectors, name)
		catalog.skills = append(catalog.skills, &skillentity.Skill{
			ID:      id,
			SpaceID: 7,
			Name:    name,
			Type:    skillentity.TypeDeerSkill,
			Enabled: true,
		})
		catalog.versions[id] = []*skillentity.SkillVersion{
			{
				ID:      id + 1000,
				SkillID: id,
				Version: "1.0.0",
				SkillMD: `---
name: ` + name + `
description: Allowed default skill.
type: deer_skill
version: 1.0.0
enabled: true
---
# ` + name + `

Use this skill when it matches the task.
`,
			},
		}
	}
	rawSelectors, err := json.Marshal(selectors)
	require.NoError(t, err)
	provider := NewRuntimeSkillProvider(catalog)

	skills, err := provider.Load(context.Background(), &RunSummary{
		SpaceID: 7,
		Config:  `{"skills":{"enabled":true,"allowed_skills":` + string(rawSelectors) + `}}`,
	})

	require.Nil(t, skills)
	require.ErrorContains(t, err, "selected skill count")
	require.Empty(t, catalog.listVersionSkillIDs)
}

func TestRuntimeSkillProviderRejectsImplicitCatalogBeyondTotalBodyBytes(t *testing.T) {
	body := strings.Repeat("Use this detailed skill instruction. ", 8000)
	catalog := &recordingRuntimeSkillCatalog{
		skills: []*skillentity.Skill{
			{
				ID:      101,
				SpaceID: 7,
				Name:    "large-auto-skill-a",
				Type:    skillentity.TypeDeerSkill,
				Enabled: true,
			},
			{
				ID:      102,
				SpaceID: 7,
				Name:    "large-auto-skill-b",
				Type:    skillentity.TypeDeerSkill,
				Enabled: true,
			},
		},
		versions: map[int64][]*skillentity.SkillVersion{
			101: {
				{
					ID:      201,
					SkillID: 101,
					Version: "1.0.0",
					SkillMD: `---
name: large-auto-skill-a
description: Large automatically selected skill A.
type: deer_skill
version: 1.0.0
enabled: true
---
# Large Skill A

` + body,
				},
			},
			102: {
				{
					ID:      202,
					SkillID: 102,
					Version: "1.0.0",
					SkillMD: `---
name: large-auto-skill-b
description: Large automatically selected skill B.
type: deer_skill
version: 1.0.0
enabled: true
---
# Large Skill B

` + body,
				},
			},
		},
	}
	provider := NewRuntimeSkillProvider(catalog)

	skills, err := provider.Load(context.Background(), &RunSummary{
		SpaceID: 7,
		Config:  `{}`,
	})

	require.Nil(t, skills)
	require.ErrorContains(t, err, "skill catalog content exceeds")
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
