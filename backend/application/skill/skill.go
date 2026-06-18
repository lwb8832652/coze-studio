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
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

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

func (s *ApplicationService) ListSkillVersions(ctx context.Context, req *skillapi.ListSkillVersionsRequest) (*skillapi.ListSkillVersionsResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, domain.InvalidArgumentErrorf("list skill versions request is required")
	}
	versions, err := s.DomainSVC.ListVersions(ctx, req.SkillID)
	if err != nil {
		return nil, err
	}

	data := &skillapi.ListSkillVersionsData{
		Versions: make([]*skillapi.SkillVersion, 0, len(versions)),
	}
	for _, version := range versions {
		data.Versions = append(data.Versions, skillVersionToAPI(version))
	}

	return &skillapi.ListSkillVersionsResponse{Code: 0, Msg: "success", Data: data}, nil
}

func (s *ApplicationService) ListSkillVersionResources(ctx context.Context, req *skillapi.ListSkillVersionResourcesRequest) (*skillapi.ListSkillVersionResourcesResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, domain.InvalidArgumentErrorf("list skill version resources request is required")
	}
	resources, err := s.DomainSVC.ListVersionResources(ctx, req.SkillID, req.VersionID)
	if err != nil {
		return nil, err
	}

	data := &skillapi.ListSkillVersionResourcesData{
		Resources: make([]*skillapi.SkillResource, 0, len(resources)),
	}
	for _, resource := range resources {
		data.Resources = append(data.Resources, skillResourceToAPI(resource))
	}

	return &skillapi.ListSkillVersionResourcesResponse{Code: 0, Msg: "success", Data: data}, nil
}

func (s *ApplicationService) UpdateSkillVersionResource(ctx context.Context, req *skillapi.UpdateSkillVersionResourceRequest) (*skillapi.SkillVersionResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, domain.InvalidArgumentErrorf("update skill version resource request is required")
	}
	if req.SkillID <= 0 {
		return nil, domain.InvalidArgumentErrorf("skill id is required")
	}
	if req.VersionID <= 0 {
		return nil, domain.InvalidArgumentErrorf("version id is required")
	}
	content, err := base64.StdEncoding.DecodeString(req.ContentBase64)
	if err != nil {
		return nil, domain.InvalidArgumentErrorf("invalid skill resource content_base64: %v", err)
	}
	version, err := s.DomainSVC.UpdateVersionResource(ctx, req.SkillID, req.VersionID, req.Path, content)
	if err != nil {
		return nil, err
	}

	return &skillapi.SkillVersionResponse{Code: 0, Msg: "success", Data: skillVersionToAPI(version)}, nil
}

func (s *ApplicationService) UpdateSkillVersionContent(ctx context.Context, req *skillapi.UpdateSkillVersionContentRequest) (*skillapi.SkillVersionResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, domain.InvalidArgumentErrorf("update skill version content request is required")
	}
	if req.SkillID <= 0 {
		return nil, domain.InvalidArgumentErrorf("skill id is required")
	}
	if req.VersionID <= 0 {
		return nil, domain.InvalidArgumentErrorf("version id is required")
	}
	version, err := s.DomainSVC.UpdateVersionContent(ctx, req.SkillID, req.VersionID, req.SkillMD)
	if err != nil {
		return nil, err
	}

	return &skillapi.SkillVersionResponse{Code: 0, Msg: "success", Data: skillVersionToAPI(version)}, nil
}

func (s *ApplicationService) ExportSkillVersion(ctx context.Context, req *skillapi.ExportSkillVersionRequest) (*skillapi.ExportSkillVersionResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, domain.InvalidArgumentErrorf("export skill version request is required")
	}
	if req.SkillID <= 0 {
		return nil, domain.InvalidArgumentErrorf("skill id is required")
	}
	if req.VersionID <= 0 {
		return nil, domain.InvalidArgumentErrorf("version id is required")
	}

	version, err := s.getSkillVersion(ctx, req.SkillID, req.VersionID)
	if err != nil {
		return nil, err
	}
	resources, err := s.DomainSVC.ListVersionResources(ctx, req.SkillID, req.VersionID)
	if err != nil {
		return nil, err
	}
	archive, err := buildSkillVersionArchive(version, resources)
	if err != nil {
		return nil, err
	}

	return &skillapi.ExportSkillVersionResponse{
		Code: 0,
		Msg:  "success",
		Data: &skillapi.ExportSkillVersionData{
			FileName:      fmt.Sprintf("skill_%d_%d.skill", req.SkillID, req.VersionID),
			ContentBase64: base64.StdEncoding.EncodeToString(archive),
			ContentType:   "application/zip",
		},
	}, nil
}

func (s *ApplicationService) RollbackSkillVersion(ctx context.Context, req *skillapi.RollbackSkillVersionRequest) (*skillapi.SkillResponse, error) {
	if err := s.requireDomainSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, domain.InvalidArgumentErrorf("rollback skill version request is required")
	}
	if req.SkillID <= 0 {
		return nil, domain.InvalidArgumentErrorf("skill id is required")
	}
	if req.VersionID <= 0 {
		return nil, domain.InvalidArgumentErrorf("version id is required")
	}

	skill, err := s.DomainSVC.RollbackVersion(ctx, req.SkillID, req.VersionID)
	if err != nil {
		return nil, err
	}
	return skillResponse(skill)
}

func (s *ApplicationService) getSkillVersion(ctx context.Context, skillID, versionID int64) (*entity.SkillVersion, error) {
	versions, err := s.DomainSVC.ListVersions(ctx, skillID)
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		if version != nil && version.ID == versionID {
			return version, nil
		}
	}
	return nil, domain.NotFoundErrorf("skill %d version %d not found", skillID, versionID)
}

func (s *ApplicationService) requireDomainSVC() error {
	if s == nil || s.DomainSVC == nil {
		return fmt.Errorf("skill service is not initialized")
	}
	return nil
}

func IsClientError(err error) bool {
	return domain.IsClientError(err)
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

func skillVersionToAPI(version *entity.SkillVersion) *skillapi.SkillVersion {
	if version == nil {
		return nil
	}

	return &skillapi.SkillVersion{
		ID:           version.ID,
		SkillID:      version.SkillID,
		Version:      version.Version,
		SkillMD:      version.SkillMD,
		InputSchema:  version.InputSchema,
		OutputSchema: version.OutputSchema,
		Executor:     version.Executor,
		Permissions:  version.Permissions,
		CreatedAt:    version.CreatedAt,
	}
}

func skillResourceToAPI(resource *entity.SkillResource) *skillapi.SkillResource {
	if resource == nil {
		return nil
	}

	return &skillapi.SkillResource{
		ID:            resource.ID,
		SkillID:       resource.SkillID,
		VersionID:     resource.VersionID,
		Path:          resource.Path,
		ContentBase64: base64.StdEncoding.EncodeToString(resource.Content),
		Size:          resource.Size,
		SHA256:        resource.SHA256,
		CreatedAt:     resource.CreatedAt,
	}
}

func apiTypeToEntity(typ skillapi.SkillType) (entity.Type, error) {
	switch typ {
	case skillapi.SkillType_Script:
		return entity.TypeScript, nil
	case skillapi.SkillType_Workflow:
		return entity.TypeWorkflow, nil
	case skillapi.SkillType_DeerSkill:
		return entity.TypeDeerSkill, nil
	case skillapi.SkillType_PublicSkill:
		return entity.TypePublicSkill, nil
	case skillapi.SkillType_CustomSkill:
		return entity.TypeCustomSkill, nil
	default:
		return "", domain.InvalidArgumentErrorf("unsupported skill type: %d", typ)
	}
}

func entityTypeToAPI(typ entity.Type) (skillapi.SkillType, error) {
	switch typ {
	case entity.TypeScript:
		return skillapi.SkillType_Script, nil
	case entity.TypeWorkflow:
		return skillapi.SkillType_Workflow, nil
	case entity.TypeDeerSkill:
		return skillapi.SkillType_DeerSkill, nil
	case entity.TypePublicSkill:
		return skillapi.SkillType_PublicSkill, nil
	case entity.TypeCustomSkill:
		return skillapi.SkillType_CustomSkill, nil
	default:
		return 0, domain.InvalidArgumentErrorf("unsupported skill type: %s", typ)
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

func buildSkillVersionArchive(version *entity.SkillVersion, resources []*entity.SkillResource) ([]byte, error) {
	if version == nil {
		return nil, domain.InvalidArgumentErrorf("skill version is required")
	}
	if strings.TrimSpace(version.SkillMD) == "" {
		return nil, domain.InvalidArgumentErrorf("skill version SKILL.md is required")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeZipFile(zw, "SKILL.md", []byte(version.SkillMD)); err != nil {
		_ = zw.Close()
		return nil, err
	}

	sorted := append([]*entity.SkillResource(nil), resources...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i] == nil {
			return false
		}
		if sorted[j] == nil {
			return true
		}
		return sorted[i].Path < sorted[j].Path
	})

	for _, resource := range sorted {
		if resource == nil {
			continue
		}
		name, err := safeSkillArchiveExportPath(resource.Path)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if strings.EqualFold(name, "SKILL.md") {
			_ = zw.Close()
			return nil, domain.InvalidArgumentErrorf("skill resource path conflicts with SKILL.md")
		}
		if err := writeZipFile(zw, name, resource.Content); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func safeSkillArchiveExportPath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", domain.InvalidArgumentErrorf("skill resource path is required")
	}
	if strings.Contains(trimmed, "\x00") || strings.Contains(trimmed, "\\") || path.IsAbs(trimmed) {
		return "", domain.InvalidArgumentErrorf("unsafe skill resource path: %s", value)
	}

	normalized := path.Clean(strings.TrimPrefix(trimmed, "/"))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || strings.Contains(normalized, "/../") {
		return "", domain.InvalidArgumentErrorf("unsafe skill resource path: %s", value)
	}
	return normalized, nil
}

func writeZipFile(zw *zip.Writer, name string, content []byte) error {
	header := &zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	}
	w, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create archive file %s: %w", name, err)
	}
	if _, err := w.Write(content); err != nil {
		return fmt.Errorf("write archive file %s: %w", name, err)
	}
	return nil
}

func setJSONField(target map[string]any, key string, value string) error {
	var field any
	if err := json.Unmarshal([]byte(value), &field); err != nil {
		return domain.InvalidArgumentErrorf("unmarshal %s: %v", key, err)
	}
	target[key] = field
	return nil
}
