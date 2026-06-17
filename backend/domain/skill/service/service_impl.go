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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	"gorm.io/gorm"
)

type skillService struct {
	components *Components
}

func NewService(c *Components) SkillService {
	return &skillService{components: c}
}

func (s *skillService) ImportDeclaration(ctx context.Context, spaceID int64, fileName string, content []byte) (*entity.Skill, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if err := s.requireIDGen(); err != nil {
		return nil, err
	}

	decl, err := ParseDeclaration(fileName, content)
	if err != nil {
		return nil, InvalidArgumentErrorf("%v", err)
	}

	id, err := s.components.IDGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	skill, err := declarationToSkill(spaceID, id, decl)
	if err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	skill.CreatedAt = now
	skill.UpdatedAt = now
	if err := s.components.Repo.Create(ctx, skill); err != nil {
		return nil, err
	}
	version, err := s.recordVersionWithSkillMD(ctx, skill, decl.SkillMD)
	if err != nil {
		return nil, err
	}
	if err := s.recordResources(ctx, skill.ID, version.ID, decl.Resources); err != nil {
		return nil, err
	}

	return skill, nil
}

func (s *skillService) Create(ctx context.Context, skill *entity.Skill) (*entity.Skill, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, InvalidArgumentErrorf("skill is required")
	}
	if skill.ID == 0 {
		if err := s.requireIDGen(); err != nil {
			return nil, err
		}
		id, err := s.components.IDGen.GenID(ctx)
		if err != nil {
			return nil, err
		}
		skill.ID = id
	}
	now := time.Now().UnixMilli()
	if skill.CreatedAt == 0 {
		skill.CreatedAt = now
	}
	if skill.UpdatedAt == 0 {
		skill.UpdatedAt = skill.CreatedAt
	}
	if err := s.components.Repo.Create(ctx, skill); err != nil {
		return nil, err
	}
	if _, err := s.recordVersion(ctx, skill); err != nil {
		return nil, err
	}
	return skill, nil
}

func (s *skillService) Update(ctx context.Context, skill *entity.Skill) (*entity.Skill, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if skill == nil {
		return nil, InvalidArgumentErrorf("skill is required")
	}
	skill.UpdatedAt = time.Now().UnixMilli()
	if err := s.components.Repo.Update(ctx, skill); err != nil {
		return nil, err
	}
	if _, err := s.recordVersion(ctx, skill); err != nil {
		return nil, err
	}
	return skill, nil
}

func (s *skillService) Get(ctx context.Context, id int64) (*entity.Skill, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	skill, err := s.components.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, NotFoundErrorf("skill %d not found", id)
		}
		return nil, err
	}
	if skill == nil {
		return nil, NotFoundErrorf("skill %d not found", id)
	}
	return skill, nil
}

func (s *skillService) List(ctx context.Context, spaceID int64, typ *entity.Type, enabled *bool) ([]*entity.Skill, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	return s.components.Repo.List(ctx, spaceID, typ, enabled)
}

func (s *skillService) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if skillID <= 0 {
		return nil, InvalidArgumentErrorf("skill id is required")
	}

	return s.components.Repo.ListVersions(ctx, skillID)
}

func (s *skillService) TestRun(ctx context.Context, id int64, input string) (string, error) {
	skill, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}

	params := map[string]any{}
	if strings.TrimSpace(input) != "" {
		if err := json.Unmarshal([]byte(input), &params); err != nil {
			return "", InvalidArgumentErrorf("invalid test input json: %v", err)
		}
	}

	decl, err := skillToDeclaration(skill)
	if err != nil {
		return "", err
	}

	runner, err := s.runnerForType(skill.Type)
	if err != nil {
		return "", err
	}
	result, err := runner.Run(ctx, decl, params)
	if err != nil {
		return "", err
	}
	output, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (s *skillService) requireRepo() error {
	if s == nil || s.components == nil {
		return fmt.Errorf("skill service components are required")
	}
	if s.components.Repo == nil {
		return fmt.Errorf("skill repository is required")
	}
	return nil
}

func (s *skillService) requireIDGen() error {
	if s == nil || s.components == nil || s.components.IDGen == nil {
		return fmt.Errorf("id generator is required")
	}
	return nil
}

func (s *skillService) recordVersion(ctx context.Context, skill *entity.Skill) (*entity.SkillVersion, error) {
	return s.recordVersionWithSkillMD(ctx, skill, "")
}

func (s *skillService) recordVersionWithSkillMD(ctx context.Context, skill *entity.Skill, skillMD string) (*entity.SkillVersion, error) {
	if skill == nil {
		return nil, InvalidArgumentErrorf("skill is required")
	}
	if err := s.requireIDGen(); err != nil {
		return nil, err
	}
	id, err := s.components.IDGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	versionSkillMD := skillMD
	if strings.TrimSpace(versionSkillMD) == "" {
		versionSkillMD = skillMarkdown(skill)
	}
	version := &entity.SkillVersion{
		ID:           id,
		SkillID:      skill.ID,
		Version:      skill.Version,
		SkillMD:      versionSkillMD,
		InputSchema:  skill.InputSchema,
		OutputSchema: skill.OutputSchema,
		Executor:     skill.Executor,
		Permissions:  skill.Permissions,
		CreatedAt:    now,
	}

	if err := s.components.Repo.CreateVersion(ctx, version); err != nil {
		return nil, err
	}
	return version, nil
}

func (s *skillService) recordResources(ctx context.Context, skillID, versionID int64, resources []ArchiveResource) error {
	if len(resources) == 0 {
		return nil
	}

	items := make([]*entity.SkillResource, 0, len(resources))
	for _, resource := range resources {
		items = append(items, &entity.SkillResource{
			SkillID:   skillID,
			VersionID: versionID,
			Path:      resource.Path,
			Content:   append([]byte(nil), resource.Content...),
			Size:      resource.Size,
			SHA256:    resource.SHA256,
		})
	}

	return s.components.Repo.CreateResources(ctx, items)
}

func (s *skillService) runnerForType(typ entity.Type) (Executor, error) {
	if s == nil || s.components == nil {
		return nil, fmt.Errorf("skill service components are required")
	}
	switch typ {
	case entity.TypeScript:
		if s.components.ScriptRunner == nil {
			return nil, fmt.Errorf("script runner is required")
		}
		return s.components.ScriptRunner, nil
	case entity.TypeWorkflow:
		if s.components.WorkflowRunner == nil {
			return UnsupportedExecutor{}, nil
		}
		return s.components.WorkflowRunner, nil
	case entity.TypeDeerSkill, entity.TypePublicSkill, entity.TypeCustomSkill:
		return nil, InvalidArgumentErrorf("skill type %s does not support direct test run", typ)
	default:
		return nil, InvalidArgumentErrorf("unsupported skill type: %s", typ)
	}
}

func declarationToSkill(spaceID, id int64, decl *Declaration) (*entity.Skill, error) {
	typ, err := declarationTypeToEntity(decl.Type)
	if err != nil {
		return nil, err
	}
	inputSchema, err := marshalString(decl.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal input schema: %w", err)
	}
	outputSchema, err := marshalString(decl.OutputSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal output schema: %w", err)
	}
	executor, err := marshalString(decl.Executor)
	if err != nil {
		return nil, fmt.Errorf("marshal executor: %w", err)
	}
	permissions, err := marshalString(decl.Permissions)
	if err != nil {
		return nil, fmt.Errorf("marshal permissions: %w", err)
	}

	return &entity.Skill{
		ID:           id,
		SpaceID:      spaceID,
		Name:         decl.Name,
		Description:  decl.Description,
		Type:         typ,
		Version:      decl.Version,
		Enabled:      decl.Enabled,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Executor:     executor,
		Permissions:  permissions,
	}, nil
}

func skillMarkdown(skill *entity.Skill) string {
	if skill == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(skill.Name)
	b.WriteString("\n")
	b.WriteString("description: ")
	b.WriteString(skill.Description)
	b.WriteString("\n")
	b.WriteString("type: ")
	b.WriteString(string(skill.Type))
	b.WriteString("\n")
	b.WriteString("version: ")
	b.WriteString(skill.Version)
	b.WriteString("\n")
	b.WriteString("enabled: ")
	b.WriteString(strconv.FormatBool(skill.Enabled))
	b.WriteString("\n")
	b.WriteString("---\n")
	if strings.TrimSpace(skill.Description) != "" {
		b.WriteString("\n")
		b.WriteString(skill.Description)
		b.WriteString("\n")
	}

	return b.String()
}

func skillToDeclaration(skill *entity.Skill) (*Declaration, error) {
	if skill == nil {
		return nil, fmt.Errorf("skill is required")
	}

	decl := &Declaration{
		// Original declaration IDs are not persisted yet; use the stable numeric skill ID for runtime callers.
		ID:          strconv.FormatInt(skill.ID, 10),
		Name:        skill.Name,
		Description: skill.Description,
		Type:        string(skill.Type),
		Version:     skill.Version,
		Enabled:     skill.Enabled,
	}
	if err := unmarshalString("input_schema", skill.InputSchema, &decl.InputSchema); err != nil {
		return nil, err
	}
	if err := unmarshalString("output_schema", skill.OutputSchema, &decl.OutputSchema); err != nil {
		return nil, err
	}
	if err := unmarshalString("executor", skill.Executor, &decl.Executor); err != nil {
		return nil, err
	}
	if err := unmarshalString("permissions", skill.Permissions, &decl.Permissions); err != nil {
		return nil, err
	}

	return decl, nil
}

func declarationTypeToEntity(typ string) (entity.Type, error) {
	switch typ {
	case string(entity.TypeScript):
		return entity.TypeScript, nil
	case string(entity.TypeWorkflow):
		return entity.TypeWorkflow, nil
	case string(entity.TypeDeerSkill):
		return entity.TypeDeerSkill, nil
	case string(entity.TypePublicSkill):
		return entity.TypePublicSkill, nil
	case string(entity.TypeCustomSkill):
		return entity.TypeCustomSkill, nil
	default:
		return "", InvalidArgumentErrorf("unsupported declaration type: %s", typ)
	}
}

func marshalString(v any) (string, error) {
	bs, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func unmarshalString(name, value string, target any) error {
	if strings.TrimSpace(value) == "" {
		value = "{}"
	}
	if err := json.Unmarshal([]byte(value), target); err != nil {
		return InvalidArgumentErrorf("unmarshal %s: %v", name, err)
	}
	return nil
}
