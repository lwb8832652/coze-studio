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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
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
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 201}})

	updated, err := svc.Update(context.Background(), &entity.Skill{
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
	})

	require.NoError(t, err)
	require.Equal(t, "1.1.0", updated.Version)
	require.Len(t, repo.versions[101], 1)
	require.Equal(t, int64(201), repo.versions[101][0].ID)
	require.Equal(t, "1.1.0", repo.versions[101][0].Version)
	require.Contains(t, repo.versions[101][0].SkillMD, "build updated report")
}

func TestServiceListVersions(t *testing.T) {
	repo := newMemoryRepo()
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
	items    map[int64]*entity.Skill
	versions map[int64][]*entity.SkillVersion
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		items:    map[int64]*entity.Skill{},
		versions: map[int64][]*entity.SkillVersion{},
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

func (r *memoryRepo) Get(ctx context.Context, id int64) (*entity.Skill, error) {
	return r.items[id], nil
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
