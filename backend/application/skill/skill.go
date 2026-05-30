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
	"fmt"
	"strconv"

	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	domain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

var SVC = new(ApplicationService)

type ApplicationService struct {
	DomainSVC domain.SkillService
}

func (s *ApplicationService) ImportSkill(ctx context.Context, req *skillapi.ImportSkillRequest) (*skillapi.SkillResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	skill, err := s.DomainSVC.ImportDeclaration(ctx, req.SpaceID, req.FileName, []byte(req.Content))
	if err != nil {
		return nil, err
	}
	return skillResponse(skill)
}

func (s *ApplicationService) CreateSkill(ctx context.Context, req *skillapi.UpsertSkillRequest) (*skillapi.SkillResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	skill, err := upsertRequestToEntity(req)
	if err != nil {
		return nil, err
	}
	created, err := s.DomainSVC.Create(ctx, skill)
	if err != nil {
		return nil, err
	}
	return skillResponse(created)
}

func (s *ApplicationService) UpdateSkill(ctx context.Context, req *skillapi.UpdateSkillRequest) (*skillapi.SkillResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	skill, err := updateRequestToEntity(req)
	if err != nil {
		return nil, err
	}
	updated, err := s.DomainSVC.Update(ctx, skill)
	if err != nil {
		return nil, err
	}
	return skillResponse(updated)
}

func (s *ApplicationService) ListSkills(ctx context.Context, req *skillapi.ListSkillsRequest) (*skillapi.ListSkillsResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	var typ *entity.Type
	if req.Type != nil {
		t, err := apiTypeToEntity(*req.Type)
		if err != nil {
			return nil, err
		}
		typ = &t
	}
	skills, err := s.DomainSVC.List(ctx, req.SpaceID, typ, req.Enabled)
	if err != nil {
		return nil, err
	}

	data := &skillapi.ListSkillsData{Skills: make([]*skillapi.Skill, 0, len(skills))}
	for _, item := range skills {
		apiSkill, err := entityToAPI(item)
		if err != nil {
			return nil, err
		}
		data.Skills = append(data.Skills, apiSkill)
	}
	return &skillapi.ListSkillsResponse{Code: 0, Msg: "success", Data: data}, nil
}

func (s *ApplicationService) GetSkill(ctx context.Context, req *skillapi.GetSkillRequest) (*skillapi.SkillResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	skill, err := s.DomainSVC.Get(ctx, req.SkillID)
	if err != nil {
		return nil, err
	}
	return skillResponse(skill)
}

func (s *ApplicationService) ExportSkill(ctx context.Context, req *skillapi.GetSkillRequest) (*skillapi.ExportSkillResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	skill, err := s.DomainSVC.Get(ctx, req.SkillID)
	if err != nil {
		return nil, err
	}
	content, err := exportContent(skill)
	if err != nil {
		return nil, err
	}
	return &skillapi.ExportSkillResponse{
		Code: 0,
		Msg:  "success",
		Data: &skillapi.ExportSkillData{
			FileName: fmt.Sprintf("skill_%d.json", skill.ID),
			Content:  content,
		},
	}, nil
}

func (s *ApplicationService) TestRunSkill(ctx context.Context, req *skillapi.TestRunSkillRequest) (*skillapi.TestRunSkillResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	output, err := s.DomainSVC.TestRun(ctx, req.SkillID, req.Input)
	if err != nil {
		return nil, err
	}
	return &skillapi.TestRunSkillResponse{
		Code: 0,
		Msg:  "success",
		Data: &skillapi.TestRunSkillData{Output: &output},
	}, nil
}

func (s *ApplicationService) requireDomainSVC() error {
	if s == nil || s.DomainSVC == nil {
		return fmt.Errorf("skill service is not initialized")
	}
	return nil
}

func upsertRequestToEntity(req *skillapi.UpsertSkillRequest) (*entity.Skill, error) {
	typ, err := apiTypeToEntity(req.Type)
	if err != nil {
		return nil, err
	}
	skill := &entity.Skill{
		SpaceID:      req.SpaceID,
		Name:         req.Name,
		Description:  req.Description,
		Type:         typ,
		Version:      req.Version,
		Enabled:      req.Enabled,
		InputSchema:  req.InputSchema,
		OutputSchema: req.OutputSchema,
		Executor:     req.Executor,
		Permissions:  req.Permissions,
	}
	if req.ID != nil {
		skill.ID = *req.ID
	}
	return skill, nil
}

func updateRequestToEntity(req *skillapi.UpdateSkillRequest) (*entity.Skill, error) {
	typ, err := apiTypeToEntity(req.Type)
	if err != nil {
		return nil, err
	}
	return &entity.Skill{
		ID:           req.ID,
		SpaceID:      req.SpaceID,
		Name:         req.Name,
		Description:  req.Description,
		Type:         typ,
		Version:      req.Version,
		Enabled:      req.Enabled,
		InputSchema:  req.InputSchema,
		OutputSchema: req.OutputSchema,
		Executor:     req.Executor,
		Permissions:  req.Permissions,
	}, nil
}

func skillResponse(skill *entity.Skill) (*skillapi.SkillResponse, error) {
	apiSkill, err := entityToAPI(skill)
	if err != nil {
		return nil, err
	}
	return &skillapi.SkillResponse{Code: 0, Msg: "success", Data: apiSkill}, nil
}

func entityToAPI(skill *entity.Skill) (*skillapi.Skill, error) {
	if skill == nil {
		return nil, fmt.Errorf("skill is required")
	}
	typ, err := entityTypeToAPI(skill.Type)
	if err != nil {
		return nil, err
	}
	return &skillapi.Skill{
		ID:           skill.ID,
		SpaceID:      skill.SpaceID,
		Name:         skill.Name,
		Description:  skill.Description,
		Type:         typ,
		Version:      skill.Version,
		Enabled:      skill.Enabled,
		InputSchema:  skill.InputSchema,
		OutputSchema: skill.OutputSchema,
		Executor:     skill.Executor,
		Permissions:  skill.Permissions,
		CreatedAt:    skill.CreatedAt,
		UpdatedAt:    skill.UpdatedAt,
	}, nil
}

func apiTypeToEntity(typ skillapi.SkillType) (entity.Type, error) {
	switch typ {
	case skillapi.SkillType_Script:
		return entity.TypeScript, nil
	case skillapi.SkillType_Workflow:
		return entity.TypeWorkflow, nil
	default:
		return "", fmt.Errorf("unsupported skill type: %d", typ)
	}
}

func entityTypeToAPI(typ entity.Type) (skillapi.SkillType, error) {
	switch typ {
	case entity.TypeScript:
		return skillapi.SkillType_Script, nil
	case entity.TypeWorkflow:
		return skillapi.SkillType_Workflow, nil
	default:
		return 0, fmt.Errorf("unsupported skill type: %s", typ)
	}
}

func exportContent(skill *entity.Skill) (string, error) {
	decl := map[string]any{
		"id":          strconv.FormatInt(skill.ID, 10),
		"name":        skill.Name,
		"description": skill.Description,
		"type":        string(skill.Type),
		"version":     skill.Version,
		"enabled":     skill.Enabled,
	}
	if err := setJSONField(decl, "input_schema", skill.InputSchema); err != nil {
		return "", err
	}
	if err := setJSONField(decl, "output_schema", skill.OutputSchema); err != nil {
		return "", err
	}
	if err := setJSONField(decl, "executor", skill.Executor); err != nil {
		return "", err
	}
	if err := setJSONField(decl, "permissions", skill.Permissions); err != nil {
		return "", err
	}
	content, err := json.Marshal(decl)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func setJSONField(target map[string]any, key string, value string) error {
	var field any
	if err := json.Unmarshal([]byte(value), &field); err != nil {
		return fmt.Errorf("unmarshal %s: %w", key, err)
	}
	target[key] = field
	return nil
}
