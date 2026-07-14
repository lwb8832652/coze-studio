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

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	"github.com/coze-dev/coze-studio/backend/domain/skill/repository"
)

func TestServiceImportsScriptSkill(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 101}})
	content := []byte(`id: weekly_report
name: Weekly Report
description: build report
type: script
version: 1.0.0
enabled: true
input_schema: {type: object}
output_schema: {type: object}
executor: {language: python, entry: main.py}
permissions: {network: false}
`)

	skill, err := svc.ImportDeclaration(context.Background(), 1, "skill.yaml", content)

	require.NoError(t, err)
	require.Equal(t, int64(101), skill.ID)
	require.Equal(t, entity.TypeScript, skill.Type)
	require.Equal(t, "Weekly Report", skill.Name)
}

func TestServiceImportsSkillMarkdownAndRecordsOriginalSkillMD(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 101}})
	content := []byte(`---
name: weekly-research
description: Research weekly market changes.
allowed-tools:
  - search
---
# Weekly Research

Collect signals and write a short brief.
`)

	skill, err := svc.ImportDeclaration(context.Background(), 1, "SKILL.md", content)

	require.NoError(t, err)
	require.Equal(t, int64(101), skill.ID)
	require.Equal(t, entity.TypeDeerSkill, skill.Type)
	require.Equal(t, "weekly-research", skill.Name)
	require.Equal(t, "Research weekly market changes.", skill.Description)
	require.True(t, skill.Enabled)
	require.JSONEq(t, `{"network":false,"allowed_tools":["search"]}`, skill.Permissions)
	require.Len(t, repo.versions[101], 1)
	require.Equal(t, string(content), repo.versions[101][0].SkillMD)
}

func TestServiceImportsSkillArchiveAndRecordsSkillMD(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 101}})
	content := buildSkillArchive(t, map[string]string{
		"weekly-research/SKILL.md": `---
name: weekly-research
description: Research weekly market changes.
allowed-tools:
  - search
---
# Weekly Research
`,
		"weekly-research/references/prompt.md": "Use concise bullets.",
	})

	skill, err := svc.ImportDeclaration(context.Background(), 1, "weekly-research.skill", content)

	require.NoError(t, err)
	require.Equal(t, entity.TypeDeerSkill, skill.Type)
	require.Equal(t, "weekly-research", skill.Name)
	require.Len(t, repo.versions[101], 1)
	require.Contains(t, repo.versions[101][0].SkillMD, "# Weekly Research")
	require.NotContains(t, repo.versions[101][0].SkillMD, "Use concise bullets")
}

func TestServiceImportsSkillArchiveWithCustomDefaultType(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 101}})
	content := buildSkillArchive(t, map[string]string{
		"weekly-research/SKILL.md": `---
name: weekly-research
description: Research weekly market changes.
---
# Weekly Research
`,
	})

	skill, err := svc.ImportDeclarationWithDefaultType(
		context.Background(),
		1,
		"weekly-research.skill",
		content,
		entity.TypeCustomSkill,
	)

	require.NoError(t, err)
	require.Equal(t, entity.TypeCustomSkill, skill.Type)
	require.Equal(t, "weekly-research", skill.Name)
}

func TestServiceImportCustomSkillArchiveRejectsExistingCustomSkill(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[100] = &entity.Skill{
		ID:           100,
		SpaceID:      1,
		Name:         "weekly-research",
		Description:  "Existing custom skill.",
		Type:         entity.TypeCustomSkill,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  `{}`,
		OutputSchema: `{}`,
		Executor:     `{}`,
		Permissions:  `{}`,
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 101}})
	content := buildSkillArchive(t, map[string]string{
		"weekly-research/SKILL.md": `---
name: weekly-research
description: Research weekly market changes.
---
# Weekly Research
`,
	})

	_, err := svc.ImportDeclarationWithDefaultType(
		context.Background(),
		1,
		"weekly-research.skill",
		content,
		entity.TypeCustomSkill,
	)

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "already exists")
	require.Nil(t, repo.items[101])
	require.Empty(t, repo.versions[101])
}

func TestServiceImportDeclarationDefaultTypeDoesNotOverrideExplicitType(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 101}})
	content := buildSkillArchive(t, map[string]string{
		"weekly-research/SKILL.md": `---
name: weekly-research
description: Research weekly market changes.
type: public_skill
---
# Weekly Research
`,
	})

	skill, err := svc.ImportDeclarationWithDefaultType(
		context.Background(),
		1,
		"weekly-research.skill",
		content,
		entity.TypeCustomSkill,
	)

	require.NoError(t, err)
	require.Equal(t, entity.TypePublicSkill, skill.Type)
}

func TestServiceImportsSkillArchivePersistsResources(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 101}})
	content := buildSkillArchive(t, map[string]string{
		"weekly-research/SKILL.md": `---
name: weekly-research
description: Research weekly market changes.
allowed-tools:
  - search
---
# Weekly Research
`,
		"weekly-research/references/prompt.md": "Use concise bullets.",
		"weekly-research/assets/logo.txt":      "asset",
	})

	skill, err := svc.ImportDeclaration(context.Background(), 1, "weekly-research.skill", content)

	require.NoError(t, err)
	require.Equal(t, int64(101), skill.ID)
	require.Len(t, repo.versions[101], 1)
	versionID := repo.versions[101][0].ID
	resources := repo.resources[versionID]
	require.Len(t, resources, 2)
	require.Equal(t, "assets/logo.txt", resources[0].Path)
	require.Equal(t, []byte("asset"), resources[0].Content)
	require.Equal(t, int64(5), resources[0].Size)
	require.NotEmpty(t, resources[0].SHA256)
	require.Equal(t, int64(101), resources[0].SkillID)
	require.Equal(t, versionID, resources[0].VersionID)
	require.Equal(t, "references/prompt.md", resources[1].Path)
	require.Equal(t, []byte("Use concise bullets."), resources[1].Content)
}

func TestServiceCreateRecordsSkillVersion(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 101}})
	skill := &entity.Skill{
		SpaceID:      1,
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

	created, err := svc.Create(context.Background(), skill)

	require.NoError(t, err)
	require.Equal(t, int64(101), created.ID)
	require.Len(t, repo.versions[101], 1)
	require.Equal(t, "1.0.0", repo.versions[101][0].Version)
	require.Contains(t, repo.versions[101][0].SkillMD, "Weekly Report")
	require.Equal(t, `{"language":"python","entry":"main.py"}`, repo.versions[101][0].Executor)
}

func TestServiceUpdateRecordsSkillVersion(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID:           101,
		SpaceID:      1,
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
	repo.versions[101] = []*entity.SkillVersion{{
		ID:      200,
		SkillID: 101,
		Version: "1.0.0",
		SkillMD: `---
name: Weekly Report
description: build report
type: script
version: 1.0.0
enabled: true
---
Original body.
`,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 201}})

	updated, err := svc.UpdateWithExpectedVersion(context.Background(), &entity.Skill{
		ID:           101,
		SpaceID:      1,
		Name:         "Weekly Report",
		Description:  "build updated report",
		Type:         entity.TypeScript,
		Version:      "1.1.0",
		Enabled:      false,
		InputSchema:  `{"type":"object"}`,
		OutputSchema: `{"type":"object"}`,
		Executor:     `{"language":"python","entry":"main.py"}`,
		Permissions:  `{"network":false}`,
	}, 200)

	require.NoError(t, err)
	require.Equal(t, "1.1.0", updated.Version)
	require.Len(t, repo.versions[101], 2)
	require.Equal(t, int64(201), repo.versions[101][1].ID)
	require.Equal(t, "1.1.0", repo.versions[101][1].Version)
	require.Contains(t, repo.versions[101][1].SkillMD, "build updated report")
}

func TestServiceMetadataUpdatePreservesLatestSkillMDAndResources(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID: 101, SpaceID: 1, Name: "weekly", Description: "before", Type: entity.TypeCustomSkill,
		Version: "1.0.0", Enabled: true, InputSchema: `{}`, OutputSchema: `{}`, Executor: `{}`, Permissions: `{}`,
		IconURI: "skill-icon://ocean", UsageScenarios: "weekly", DevelopmentThreadID: 901,
	}
	originalSkillMD := `---
name: weekly
description: before
type: custom_skill
version: 1.0.0
enabled: true
---
# Full body

Keep this body intact.
`
	repo.versions[101] = []*entity.SkillVersion{{ID: 201, SkillID: 101, Version: "1.0.0", SkillMD: originalSkillMD, InputSchema: `{}`, OutputSchema: `{}`, Executor: `{}`, Permissions: `{}`, CreatedAt: 1}}
	repo.resources[201] = []*entity.SkillResource{{SkillID: 101, VersionID: 201, Path: "references/prompt.md", Content: []byte("keep"), Size: 4, SHA256: sha256Hex([]byte("keep"))}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})

	updated := *repo.items[101]
	updated.Description = "after"
	result, err := svc.UpdateWithExpectedVersion(context.Background(), &updated, 201)

	require.NoError(t, err)
	require.Equal(t, "after", result.Description)
	require.Len(t, repo.versions[101], 2)
	require.Contains(t, repo.versions[101][1].SkillMD, "description: after")
	require.NotContains(t, repo.versions[101][1].SkillMD, "description: before")
	require.Contains(t, repo.versions[101][1].SkillMD, "# Full body\n\nKeep this body intact.")
	require.Len(t, repo.resources[401], 1)
	require.Equal(t, "references/prompt.md", repo.resources[401][0].Path)
	require.Equal(t, []byte("keep"), repo.resources[401][0].Content)
}

func TestServiceResourceWriteRejectsStaleVersion(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{ID: 101, SpaceID: 1, Type: entity.TypeCustomSkill}
	repo.versions[101] = []*entity.SkillVersion{
		{ID: 200, SkillID: 101, SkillMD: validCustomSkillMD("old"), CreatedAt: 1},
		{ID: 201, SkillID: 101, SkillMD: validCustomSkillMD("latest"), CreatedAt: 2},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})

	_, err := svc.UpdateVersionResource(context.Background(), 101, 200, "a.txt", []byte("stale"))

	require.Error(t, err)
	require.True(t, IsConflict(err))
	require.Len(t, repo.versions[101], 2)
}

func TestServiceDirectoryMoveCreatesOneSnapshotAndRejectsOverwrite(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{ID: 101, SpaceID: 1, Type: entity.TypeCustomSkill}
	repo.versions[101] = []*entity.SkillVersion{{ID: 201, SkillID: 101, SkillMD: validCustomSkillMD("weekly"), CreatedAt: 1}}
	repo.resources[201] = []*entity.SkillResource{
		{SkillID: 101, VersionID: 201, Path: "references/a.md", Content: []byte("a"), Size: 1},
		{SkillID: 101, VersionID: 201, Path: "references/b.md", Content: []byte("b"), Size: 1},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})

	version, err := svc.MutateVersionResources(context.Background(), 101, 201, []ResourceMutation{{Operation: ResourceMutationMove, Path: "references", TargetPath: "docs"}})
	require.NoError(t, err)
	require.Equal(t, int64(401), version.ID)
	require.Len(t, repo.versions[101], 2)
	require.Equal(t, []string{"docs/a.md", "docs/b.md"}, []string{repo.resources[401][0].Path, repo.resources[401][1].Path})

	repo.items[102] = &entity.Skill{ID: 102, SpaceID: 1, Type: entity.TypeCustomSkill}
	repo.versions[102] = []*entity.SkillVersion{{ID: 202, SkillID: 102, SkillMD: validCustomSkillMD("collision"), CreatedAt: 1}}
	repo.resources[202] = []*entity.SkillResource{
		{SkillID: 102, VersionID: 202, Path: "references/a.md", Content: []byte("a")},
		{SkillID: 102, VersionID: 202, Path: "docs/a.md", Content: []byte("existing")},
	}
	_, err = svc.MutateVersionResources(context.Background(), 102, 202, []ResourceMutation{{Operation: ResourceMutationMove, Path: "references", TargetPath: "docs"}})
	require.ErrorContains(t, err, "already exists")
	require.Len(t, repo.versions[102], 1)
}

func validCustomSkillMD(name string) string {
	return "---\nname: " + name + "\ndescription: safe\ntype: custom_skill\nversion: 1.0.0\nenabled: true\n---\n# body\n"
}

func TestServiceDeleteHidesSkillAndKeepsVersions(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID:           101,
		SpaceID:      1,
		Name:         "Weekly Report",
		Description:  "build report",
		Type:         entity.TypeCustomSkill,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  `{}`,
		OutputSchema: `{}`,
		Executor:     `{}`,
		Permissions:  `{}`,
	}
	repo.versions[101] = []*entity.SkillVersion{
		{ID: 201, SkillID: 101, Version: "1.0.0", SkillMD: "# Weekly Report"},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 202}})

	deleted, err := svc.Delete(context.Background(), 101)

	require.NoError(t, err)
	require.Equal(t, int64(101), deleted.ID)
	require.Equal(t, int64(101), repo.deletedSkillID)
	_, err = svc.Get(context.Background(), 101)
	require.Error(t, err)
	require.True(t, IsClientError(err))
	versions, err := repo.ListVersions(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "# Weekly Report", versions[0].SkillMD)
}

func TestServiceDeleteBlocksVersionAPIsButKeepsSnapshots(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID:           101,
		SpaceID:      1,
		Name:         "Weekly Report",
		Type:         entity.TypeCustomSkill,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  `{}`,
		OutputSchema: `{}`,
		Executor:     `{}`,
		Permissions:  `{}`,
	}
	repo.versions[101] = []*entity.SkillVersion{
		{ID: 201, SkillID: 101, Version: "1.0.0", SkillMD: "# Weekly Report"},
	}
	repo.resources[201] = []*entity.SkillResource{
		{ID: 301, SkillID: 101, VersionID: 201, Path: "references/prompt.md", Content: []byte("keep")},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 202}})

	_, err := svc.Delete(context.Background(), 101)
	require.NoError(t, err)

	_, err = svc.ListVersions(context.Background(), 101)
	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "not found")

	_, err = svc.ListVersionResources(context.Background(), 101, 201)
	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "not found")
	require.Equal(t, int64(0), repo.listResourcesSkillID)

	versions, err := repo.ListVersions(context.Background(), 101)
	require.NoError(t, err)
	require.Len(t, versions, 1)
	resources, err := repo.ListResources(context.Background(), 101, 201)
	require.NoError(t, err)
	require.Len(t, resources, 1)
}

func TestServiceListVersions(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{ID: 101, SpaceID: 1}
	repo.versions[101] = []*entity.SkillVersion{
		{ID: 201, SkillID: 101, Version: "1.1.0"},
		{ID: 200, SkillID: 101, Version: "1.0.0"},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 202}})

	versions, err := svc.ListVersions(context.Background(), 101)

	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, "1.1.0", versions[0].Version)
}

func TestServiceListVersionResources(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{ID: 101, SpaceID: 1}
	repo.resources[201] = []*entity.SkillResource{
		{
			ID:        301,
			SkillID:   101,
			VersionID: 201,
			Path:      "references/prompt.md",
			Content:   []byte("Use concise bullets."),
			Size:      20,
			SHA256:    "hash-prompt",
			CreatedAt: 1000,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 202}})

	resources, err := svc.ListVersionResources(context.Background(), 101, 201)

	require.NoError(t, err)
	require.Equal(t, int64(101), repo.listResourcesSkillID)
	require.Equal(t, int64(201), repo.listResourcesVersionID)
	require.Len(t, resources, 1)
	require.Equal(t, "references/prompt.md", resources[0].Path)
	require.Equal(t, []byte("Use concise bullets."), resources[0].Content)
}

func TestServiceRollbackVersionRestoresSkillAndCopiesResources(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID:                  101,
		SpaceID:             1,
		Name:                "Weekly Research Current",
		Description:         "Current description.",
		Type:                entity.TypeDeerSkill,
		Version:             "2.0.0",
		Enabled:             false,
		InputSchema:         `{"current":true}`,
		OutputSchema:        `{"current":true}`,
		Executor:            `{"current":true}`,
		Permissions:         `{"current":true}`,
		IconURI:             "skill-icon://ocean",
		UsageScenarios:      "Create a weekly research summary.",
		DevelopmentThreadID: 901,
		CreatedAt:           1000,
		UpdatedAt:           2000,
	}
	repo.versions[101] = []*entity.SkillVersion{
		{
			ID:      201,
			SkillID: 101,
			Version: "1.0.0",
			SkillMD: `---
name: weekly-research
description: Original research skill.
type: deer_skill
version: 1.0.0
enabled: true
---
# Weekly Research
`,
			InputSchema:  `{"type":"object","rollback":true}`,
			OutputSchema: `{"type":"object","rollback":true}`,
			Executor:     `{"mode":"agent"}`,
			Permissions:  `{"network":false,"allowed_tools":["search"]}`,
			CreatedAt:    1000,
		},
	}
	repo.resources[201] = []*entity.SkillResource{
		{
			ID:        301,
			SkillID:   101,
			VersionID: 201,
			Path:      "references/prompt.md",
			Content:   []byte("Use concise bullets."),
			Size:      20,
			SHA256:    "hash-prompt",
			CreatedAt: 1000,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})

	rolledBack, err := svc.RollbackVersionCAS(context.Background(), 101, 201, 201)

	require.NoError(t, err)
	require.Equal(t, "weekly-research", rolledBack.Name)
	require.Equal(t, "Original research skill.", rolledBack.Description)
	require.Equal(t, entity.TypeDeerSkill, rolledBack.Type)
	require.Equal(t, "1.0.0", rolledBack.Version)
	require.True(t, rolledBack.Enabled)
	require.JSONEq(t, `{"type":"object","rollback":true}`, rolledBack.InputSchema)
	require.JSONEq(t, `{"mode":"agent"}`, rolledBack.Executor)
	require.JSONEq(t, `{"network":false,"allowed_tools":["search"]}`, rolledBack.Permissions)
	require.Equal(t, rolledBack, repo.items[101])
	require.Len(t, repo.versions[101], 2)
	require.Equal(t, int64(401), repo.versions[101][1].ID)
	require.Equal(t, "1.0.0", repo.versions[101][1].Version)
	require.Contains(t, repo.versions[101][1].SkillMD, "# Weekly Research")
	require.Len(t, repo.resources[401], 1)
	require.Equal(t, "references/prompt.md", repo.resources[401][0].Path)
	require.Equal(t, []byte("Use concise bullets."), repo.resources[401][0].Content)
	require.Equal(t, int64(401), repo.resources[401][0].VersionID)
}

func TestServiceRollbackVersionReturnsNotFoundForUnknownVersion(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{ID: 101, SpaceID: 1}
	repo.versions[101] = []*entity.SkillVersion{{ID: 201, SkillID: 101, Version: "1.0.0"}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})

	_, err := svc.RollbackVersionCAS(context.Background(), 101, 404, 201)

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "version 404")
}

func TestServiceUpdateVersionResourceCreatesSnapshotWithEditedResource(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID:                  101,
		SpaceID:             1,
		Name:                "Weekly Research Current",
		Description:         "Current description.",
		Type:                entity.TypeDeerSkill,
		Version:             "2.0.0",
		Enabled:             false,
		InputSchema:         `{"current":true}`,
		OutputSchema:        `{"current":true}`,
		Executor:            `{"current":true}`,
		Permissions:         `{"current":true}`,
		IconURI:             "skill-icon://ocean",
		UsageScenarios:      "Create a weekly research summary.",
		DevelopmentThreadID: 901,
		CreatedAt:           1000,
		UpdatedAt:           2000,
	}
	repo.versions[101] = []*entity.SkillVersion{
		{
			ID:      201,
			SkillID: 101,
			Version: "1.0.0",
			SkillMD: `---
name: weekly-research
description: Original research skill.
type: deer_skill
version: 1.0.0
enabled: true
---
# Weekly Research
`,
			InputSchema:  `{"type":"object","rollback":true}`,
			OutputSchema: `{"type":"object","rollback":true}`,
			Executor:     `{"mode":"agent"}`,
			Permissions:  `{"network":false,"allowed_tools":["search"]}`,
			CreatedAt:    1000,
		},
	}
	repo.resources[201] = []*entity.SkillResource{
		{
			ID:        301,
			SkillID:   101,
			VersionID: 201,
			Path:      "references/prompt.md",
			Content:   []byte("Use concise bullets."),
			Size:      20,
			SHA256:    "hash-prompt",
			CreatedAt: 1000,
		},
		{
			ID:        302,
			SkillID:   101,
			VersionID: 201,
			Path:      "assets/logo.txt",
			Content:   []byte("asset"),
			Size:      5,
			SHA256:    "hash-asset",
			CreatedAt: 1000,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})

	version, err := svc.UpdateVersionResource(context.Background(), 101, 201, "references/prompt.md", []byte("Use concise action bullets."))

	require.NoError(t, err)
	require.Equal(t, int64(401), version.ID)
	require.Equal(t, int64(101), version.SkillID)
	require.Equal(t, "1.0.0", version.Version)
	require.Contains(t, version.SkillMD, "# Weekly Research")
	require.Equal(t, "weekly-research", repo.items[101].Name)
	require.Equal(t, "Original research skill.", repo.items[101].Description)
	require.True(t, repo.items[101].Enabled)
	require.Equal(t, "skill-icon://ocean", repo.items[101].IconURI)
	require.Equal(t, "Create a weekly research summary.", repo.items[101].UsageScenarios)
	require.Equal(t, int64(901), repo.items[101].DevelopmentThreadID)
	require.Len(t, repo.versions[101], 2)

	require.Len(t, repo.resources[401], 2)
	require.Equal(t, "assets/logo.txt", repo.resources[401][0].Path)
	require.Equal(t, []byte("asset"), repo.resources[401][0].Content)
	require.Equal(t, int64(401), repo.resources[401][0].VersionID)
	require.Equal(t, "references/prompt.md", repo.resources[401][1].Path)
	require.Equal(t, []byte("Use concise action bullets."), repo.resources[401][1].Content)
	require.Equal(t, int64(len("Use concise action bullets.")), repo.resources[401][1].Size)
	require.Equal(t, sha256Hex([]byte("Use concise action bullets.")), repo.resources[401][1].SHA256)
	require.Equal(t, []byte("Use concise bullets."), repo.resources[201][0].Content)
}

func TestServiceUpdateVersionResourceRejectsSkillMDPath(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{ID: 101, SpaceID: 1}
	repo.versions[101] = []*entity.SkillVersion{{ID: 201, SkillID: 101, Version: "1.0.0", SkillMD: "# Skill"}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})

	_, err := svc.UpdateVersionResource(context.Background(), 101, 201, "SKILL.md", []byte("# Changed"))

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "SKILL.md")
}

func TestServiceUpdateVersionContentCreatesSnapshotAndCopiesResources(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID:           101,
		SpaceID:      1,
		Name:         "Weekly Research Current",
		Description:  "Current description.",
		Type:         entity.TypeDeerSkill,
		Version:      "2.0.0",
		Enabled:      false,
		InputSchema:  `{"current":true}`,
		OutputSchema: `{"current":true}`,
		Executor:     `{"current":true}`,
		Permissions:  `{"current":true}`,
		CreatedAt:    1000,
		UpdatedAt:    2000,
	}
	repo.versions[101] = []*entity.SkillVersion{
		{
			ID:      201,
			SkillID: 101,
			Version: "1.0.0",
			SkillMD: `---
name: weekly-research
description: Original research skill.
type: deer_skill
version: 1.0.0
enabled: true
---
# Weekly Research
`,
			InputSchema:  `{"type":"object","selected":true}`,
			OutputSchema: `{"type":"object","selected":true}`,
			Executor:     `{"mode":"agent"}`,
			Permissions:  `{"network":false,"allowed_tools":["search"]}`,
			CreatedAt:    1000,
		},
	}
	repo.resources[201] = []*entity.SkillResource{
		{
			ID:        301,
			SkillID:   101,
			VersionID: 201,
			Path:      "references/prompt.md",
			Content:   []byte("Use concise bullets."),
			Size:      20,
			SHA256:    "hash-prompt",
			CreatedAt: 1000,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})
	updatedSkillMD := `---
name: custom-weekly-research
description: Updated custom research skill.
type: custom_skill
version: 1.2.0
enabled: false
---
# Custom Weekly Research

Use the selected version resources.
`

	version, err := svc.UpdateVersionContent(context.Background(), 101, 201, updatedSkillMD)

	require.NoError(t, err)
	require.Equal(t, int64(401), version.ID)
	require.Equal(t, int64(101), version.SkillID)
	require.Equal(t, "1.2.0", version.Version)
	require.Equal(t, updatedSkillMD, version.SkillMD)
	require.JSONEq(t, `{"type":"object","selected":true}`, version.InputSchema)
	require.JSONEq(t, `{"mode":"agent"}`, version.Executor)
	require.JSONEq(t, `{"network":false,"allowed_tools":["search"]}`, version.Permissions)

	current := repo.items[101]
	require.Equal(t, "custom-weekly-research", current.Name)
	require.Equal(t, "Updated custom research skill.", current.Description)
	require.Equal(t, entity.TypeCustomSkill, current.Type)
	require.Equal(t, "1.2.0", current.Version)
	require.False(t, current.Enabled)
	require.JSONEq(t, `{"type":"object","selected":true}`, current.InputSchema)
	require.JSONEq(t, `{"mode":"agent"}`, current.Executor)
	require.JSONEq(t, `{"network":false,"allowed_tools":["search"]}`, current.Permissions)

	require.Len(t, repo.versions[101], 2)
	require.Len(t, repo.resources[401], 1)
	require.Equal(t, "references/prompt.md", repo.resources[401][0].Path)
	require.Equal(t, []byte("Use concise bullets."), repo.resources[401][0].Content)
	require.Equal(t, int64(401), repo.resources[401][0].VersionID)
	require.Equal(t, []byte("Use concise bullets."), repo.resources[201][0].Content)
}

func TestServiceTestRunUsesScriptRunnerWithJSONInput(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID:           101,
		SpaceID:      1,
		Name:         "Weekly Report",
		Type:         entity.TypeScript,
		Enabled:      true,
		InputSchema:  `{"type":"object"}`,
		OutputSchema: `{"type":"object"}`,
		Executor:     `{"language":"python","entry":"main.py"}`,
		Permissions:  `{"network":false}`,
	}
	runner := &capturingExecutor{result: map[string]any{"ok": true}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 102}, ScriptRunner: runner})

	output, err := svc.TestRun(context.Background(), 101, `{"topic":"sales"}`)

	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, output)
	require.Equal(t, "sales", runner.input["topic"])
	require.Equal(t, "101", runner.skill.ID)
	require.Equal(t, "python", runner.skill.Executor.Language)
}

func TestServiceTestRunAcceptsPlainTextInput(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{
		ID:           101,
		SpaceID:      1,
		Name:         "Weekly Report",
		Type:         entity.TypeScript,
		Enabled:      true,
		InputSchema:  `{"type":"object"}`,
		OutputSchema: `{"type":"object"}`,
		Executor:     `{"language":"python","entry":"main.py"}`,
		Permissions:  `{"network":false}`,
	}
	runner := &capturingExecutor{result: map[string]any{"ok": true}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 102}, ScriptRunner: runner})

	_, err := svc.TestRun(context.Background(), 101, "summarize this report")

	require.NoError(t, err)
	require.Equal(t, "summarize this report", runner.input["message"])
}

func TestServiceTestRunWorkflowReturnsNotImplementedClientError(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[202] = &entity.Skill{
		ID:           202,
		SpaceID:      1,
		Name:         "Workflow Skill",
		Type:         entity.TypeWorkflow,
		Enabled:      true,
		InputSchema:  `{"type":"object"}`,
		OutputSchema: `{"type":"object"}`,
		Executor:     `{"workflow_id":"wf_1"}`,
		Permissions:  `{"network":false}`,
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 203}, WorkflowRunner: UnsupportedExecutor{}})

	_, err := svc.TestRun(context.Background(), 202, `{}`)

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "not implemented")
}

type memoryRepo struct {
	items                                map[int64]*entity.Skill
	versions                             map[int64][]*entity.SkillVersion
	resources                            map[int64][]*entity.SkillResource
	deletedSkillID                       int64
	listResourcesSkillID                 int64
	listResourcesVersionID               int64
	createWithVersionDevelopmentThreadID int64
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		items:     map[int64]*entity.Skill{},
		versions:  map[int64][]*entity.SkillVersion{},
		resources: map[int64][]*entity.SkillResource{},
	}
}

func (r *memoryRepo) Create(ctx context.Context, skill *entity.Skill) error {
	r.items[skill.ID] = skill
	return nil
}

func (r *memoryRepo) Update(ctx context.Context, skill *entity.Skill) error {
	r.items[skill.ID] = skill
	return nil
}

func (r *memoryRepo) CreateWithVersion(ctx context.Context, skill *entity.Skill, version *entity.SkillVersion, resources []*entity.SkillResource) error {
	r.createWithVersionDevelopmentThreadID = skill.DevelopmentThreadID
	if err := r.Create(ctx, skill); err != nil {
		return err
	}
	if err := r.CreateVersion(ctx, version); err != nil {
		return err
	}
	return r.CreateResources(ctx, resources)
}

func (r *memoryRepo) UpdateWithVersion(ctx context.Context, skill *entity.Skill, version *entity.SkillVersion, resources []*entity.SkillResource) error {
	if err := r.Update(ctx, skill); err != nil {
		return err
	}
	if err := r.CreateVersion(ctx, version); err != nil {
		return err
	}
	return r.CreateResources(ctx, resources)
}

func (r *memoryRepo) UpdateWithVersionCAS(ctx context.Context, skill *entity.Skill, expectedVersionID int64, version *entity.SkillVersion, resources []*entity.SkillResource) error {
	latest, err := r.GetLatestVersion(ctx, skill.ID)
	if err != nil {
		return err
	}
	latestID := int64(0)
	if latest != nil {
		latestID = latest.ID
	}
	if latestID != expectedVersionID {
		return repository.ErrVersionConflict
	}
	return r.UpdateWithVersion(ctx, skill, version, resources)
}

func (r *memoryRepo) Delete(ctx context.Context, id int64) error {
	r.deletedSkillID = id
	if skill := r.items[id]; skill != nil {
		skill.DeletedAt = 1000
	}
	return nil
}

func (r *memoryRepo) Get(ctx context.Context, id int64) (*entity.Skill, error) {
	item := r.items[id]
	if item != nil && item.DeletedAt == 0 {
		return item, nil
	}
	return nil, nil
}

func (r *memoryRepo) List(ctx context.Context, spaceID int64, typ *entity.Type, enabled *bool) ([]*entity.Skill, error) {
	result := make([]*entity.Skill, 0, len(r.items))
	for _, item := range r.items {
		if item.SpaceID == spaceID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *memoryRepo) CreateVersion(ctx context.Context, version *entity.SkillVersion) error {
	r.versions[version.SkillID] = append(r.versions[version.SkillID], version)
	return nil
}

func (r *memoryRepo) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	return append([]*entity.SkillVersion(nil), r.versions[skillID]...), nil
}

func (r *memoryRepo) GetLatestVersion(ctx context.Context, skillID int64) (*entity.SkillVersion, error) {
	versions := r.versions[skillID]
	if len(versions) == 0 {
		return nil, nil
	}
	return versions[len(versions)-1], nil
}

func (r *memoryRepo) CreateResources(ctx context.Context, resources []*entity.SkillResource) error {
	for _, resource := range resources {
		r.resources[resource.VersionID] = append(r.resources[resource.VersionID], resource)
	}
	return nil
}

func (r *memoryRepo) ListResources(ctx context.Context, skillID, versionID int64) ([]*entity.SkillResource, error) {
	r.listResourcesSkillID = skillID
	r.listResourcesVersionID = versionID
	return append([]*entity.SkillResource(nil), r.resources[versionID]...), nil
}

type fixedIDGen struct {
	next int64
}

func (g fixedIDGen) GenID(ctx context.Context) (int64, error) {
	return g.next, nil
}

func (g fixedIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		ids[i] = g.next + int64(i)
	}
	return ids, nil
}

type incrementingIDGen struct {
	next int64
}

func (g *incrementingIDGen) GenID(context.Context) (int64, error) {
	id := g.next
	g.next++
	return id, nil
}

func (g *incrementingIDGen) GenMultiIDs(_ context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		ids[i] = g.next
		g.next++
	}
	return ids, nil
}

type capturingExecutor struct {
	skill  *Declaration
	input  map[string]any
	result map[string]any
}

func (e *capturingExecutor) Run(ctx context.Context, skill *Declaration, input map[string]any) (map[string]any, error) {
	e.skill = skill
	e.input = input
	return e.result, nil
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func TestServiceCreateSerializesSkillMarkdownFrontmatterSafely(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: &incrementingIDGen{next: 201}})
	skill := &entity.Skill{
		ID:           101,
		SpaceID:      1,
		Name:         "Research: \"weekly\"\nreview",
		Description:  "First line: \"quoted\"\nSecond line",
		Type:         entity.TypeCustomSkill,
		Version:      "1.0.0",
		Enabled:      true,
		InputSchema:  `{}`,
		OutputSchema: `{}`,
		Executor:     `{}`,
		Permissions:  `{}`,
	}

	_, err := svc.Create(context.Background(), skill)
	require.NoError(t, err)
	require.Len(t, repo.versions[101], 1)
	decl, err := ParseDeclarationWithDefaultType("SKILL.md", []byte(repo.versions[101][0].SkillMD), entity.TypeCustomSkill)
	require.NoError(t, err)
	require.Equal(t, skill.Name, decl.Name)
	require.Equal(t, skill.Description, decl.Description)
}

func TestServiceMetadataUpdateRewritesManagedYAMLAndPreservesBodyAcrossResourceSave(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{ID: 101, SpaceID: 1, Type: entity.TypeCustomSkill}
	body := "# Keep this body exactly\n\nText: \"quoted\"\n"
	repo.versions[101] = []*entity.SkillVersion{{
		ID:      201,
		SkillID: 101,
		Version: "1.0.0",
		SkillMD: `---
name: old-name
description: old description
type: custom_skill
version: 1.0.0
enabled: true
unknown_config:
  preserve: "value: quoted"
---
` + body,
	}}
	repo.resources[201] = []*entity.SkillResource{{
		ID: 301, SkillID: 101, VersionID: 201, Path: "references/keep.md", Content: []byte("keep"),
	}}
	svc := NewService(&Components{Repo: repo, IDGen: &incrementingIDGen{next: 401}})
	updatedSkill := &entity.Skill{
		ID:             101,
		SpaceID:        1,
		Name:           "new: \"quoted\"\nname",
		Description:    "updated: description\nwith newline",
		Type:           entity.TypeCustomSkill,
		Version:        "2.0.0",
		Enabled:        false,
		InputSchema:    `{"type":"object","managed":true}`,
		OutputSchema:   `{}`,
		Executor:       `{"mode":"agent"}`,
		Permissions:    `{"network":false}`,
		IconURI:        "skill-icon://updated",
		UsageScenarios: "Use: safely\nwith quotes \"here\"",
	}

	_, err := svc.UpdateWithExpectedVersion(context.Background(), updatedSkill, 201)
	require.NoError(t, err)
	require.Len(t, repo.versions[101], 2)
	metadataVersion := repo.versions[101][1]
	frontmatter, actualBody, err := splitSkillMarkdown(metadataVersion.SkillMD)
	require.NoError(t, err)
	require.Equal(t, body, actualBody)
	var metadata map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(frontmatter), &metadata))
	require.Equal(t, updatedSkill.Name, metadata["name"])
	require.Equal(t, updatedSkill.Description, metadata["description"])
	require.Equal(t, "value: quoted", metadata["unknown_config"].(map[string]any)["preserve"])

	resourceVersion, err := svc.UpdateVersionResource(context.Background(), 101, metadataVersion.ID, "references/new.md", []byte("new"))
	require.NoError(t, err)
	require.Equal(t, int64(402), resourceVersion.ID)
	require.Equal(t, metadataVersion.SkillMD, resourceVersion.SkillMD)
}

func TestServiceExpectedVersionCannotBeBypassedWithZero(t *testing.T) {
	repo := newMemoryRepo()
	repo.items[101] = &entity.Skill{ID: 101, SpaceID: 1, Type: entity.TypeCustomSkill}
	repo.versions[101] = []*entity.SkillVersion{{ID: 201, SkillID: 101, SkillMD: validCustomSkillMD("safe")}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 401}})

	_, err := svc.UpdateWithExpectedVersion(context.Background(), repo.items[101], 0)
	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "expected version id is required")
	require.Len(t, repo.versions[101], 1)

	_, err = svc.RollbackVersionCAS(context.Background(), 101, 201, 0)
	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "expected version id is required")
	require.Len(t, repo.versions[101], 1)
}

func TestServiceArtifactImportPersistsDevelopmentThreadInCreateTransaction(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: &incrementingIDGen{next: 101}})
	content := []byte(`---
name: artifact-skill
description: Imported from an authenticated artifact.
type: custom_skill
version: 1.0.0
enabled: true
---
# Artifact Skill
`)

	skill, err := svc.ImportDeclarationWithDevelopmentThread(
		context.Background(), 1, "SKILL.md", content, entity.TypeCustomSkill, 901,
	)
	require.NoError(t, err)
	require.Equal(t, int64(901), skill.DevelopmentThreadID)
	require.Equal(t, int64(901), repo.createWithVersionDevelopmentThreadID)
	require.Equal(t, int64(901), repo.items[skill.ID].DevelopmentThreadID)
	require.Len(t, repo.versions[skill.ID], 1)
}

func TestDeclarationToSkillKeepsManagementMetadata(t *testing.T) {
	skill, err := declarationToSkill(7, 101, &Declaration{
		Name:           "weekly-report",
		Description:    "Create weekly reports",
		Type:           string(entity.TypeCustomSkill),
		Version:        "1.0.0",
		Enabled:        true,
		IconURI:        "skill-icon://ocean",
		UsageScenarios: "Summarize delivery progress",
		InputSchema:    map[string]any{},
		OutputSchema:   map[string]any{},
	})

	require.NoError(t, err)
	require.Equal(t, "skill-icon://ocean", skill.IconURI)
	require.Equal(t, "Summarize delivery progress", skill.UsageScenarios)
}
