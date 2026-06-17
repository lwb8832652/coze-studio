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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type skillRepository struct {
	db    *gorm.DB
	idGen idgen.IDGenerator
}

func NewSkillRepository(db *gorm.DB, idGen idgen.IDGenerator) SkillRepository {
	return &skillRepository{
		db:    db,
		idGen: idGen,
	}
}

type skillPO struct {
	ID           int64          `gorm:"column:id;primaryKey"`
	SpaceID      int64          `gorm:"column:space_id;index:idx_skills_space_type;index:idx_skills_space_enabled"`
	Name         string         `gorm:"column:name"`
	Description  string         `gorm:"column:description"`
	Type         string         `gorm:"column:type;index:idx_skills_space_type"`
	Version      string         `gorm:"column:version"`
	Enabled      bool           `gorm:"column:enabled;index:idx_skills_space_enabled"`
	InputSchema  datatypes.JSON `gorm:"column:input_schema;type:json"`
	OutputSchema datatypes.JSON `gorm:"column:output_schema;type:json"`
	Executor     datatypes.JSON `gorm:"column:executor;type:json"`
	Permissions  datatypes.JSON `gorm:"column:permissions;type:json"`
	CreatedAt    int64          `gorm:"column:created_at"`
	UpdatedAt    int64          `gorm:"column:updated_at"`
}

func (skillPO) TableName() string {
	return "skills"
}

type skillVersionPO struct {
	ID           int64          `gorm:"column:id;primaryKey"`
	SkillID      int64          `gorm:"column:skill_id;index:idx_skill_versions_skill_created;uniqueIndex:uk_skill_versions_skill_version"`
	Version      string         `gorm:"column:version;uniqueIndex:uk_skill_versions_skill_version"`
	SkillMD      string         `gorm:"column:skill_md;type:text"`
	InputSchema  datatypes.JSON `gorm:"column:input_schema;type:json"`
	OutputSchema datatypes.JSON `gorm:"column:output_schema;type:json"`
	Executor     datatypes.JSON `gorm:"column:executor;type:json"`
	Permissions  datatypes.JSON `gorm:"column:permissions;type:json"`
	CreatedAt    int64          `gorm:"column:created_at;index:idx_skill_versions_skill_created"`
}

func (skillVersionPO) TableName() string {
	return "skill_versions"
}

type skillResourcePO struct {
	ID        int64  `gorm:"column:id;primaryKey"`
	SkillID   int64  `gorm:"column:skill_id;index:idx_skill_resources_skill_version"`
	VersionID int64  `gorm:"column:version_id;index:idx_skill_resources_skill_version;uniqueIndex:uk_skill_resources_version_path,priority:1"`
	Path      string `gorm:"column:path;size:512;uniqueIndex:uk_skill_resources_version_path,priority:2"`
	Content   []byte `gorm:"column:content;type:longblob"`
	Size      int64  `gorm:"column:size"`
	SHA256    string `gorm:"column:sha256;size:64"`
	CreatedAt int64  `gorm:"column:created_at"`
}

func (skillResourcePO) TableName() string {
	return "skill_resources"
}

func (r *skillRepository) Create(ctx context.Context, skill *entity.Skill) error {
	if skill.ID == 0 {
		id, err := r.idGen.GenID(ctx)
		if err != nil {
			return err
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

	po, err := skillToPO(skill)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *skillRepository) Update(ctx context.Context, skill *entity.Skill) error {
	if skill.UpdatedAt == 0 {
		skill.UpdatedAt = time.Now().UnixMilli()
	}

	inputSchema, err := requiredJSON("input_schema", skill.InputSchema)
	if err != nil {
		return err
	}
	outputSchema, err := requiredJSON("output_schema", skill.OutputSchema)
	if err != nil {
		return err
	}
	executor, err := requiredJSON("executor", skill.Executor)
	if err != nil {
		return err
	}
	permissions, err := requiredJSON("permissions", skill.Permissions)
	if err != nil {
		return err
	}

	updates := map[string]any{
		"name":          skill.Name,
		"description":   skill.Description,
		"type":          string(skill.Type),
		"version":       skill.Version,
		"enabled":       skill.Enabled,
		"input_schema":  inputSchema,
		"output_schema": outputSchema,
		"executor":      executor,
		"permissions":   permissions,
		"updated_at":    skill.UpdatedAt,
	}

	db := r.db.WithContext(ctx).
		Model(&skillPO{}).
		Where("id = ?", skill.ID).
		Updates(updates)
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 {
		return fmt.Errorf("update skill failed: skill %d not found", skill.ID)
	}

	return nil
}

func (r *skillRepository) Get(ctx context.Context, id int64) (*entity.Skill, error) {
	var po skillPO
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&po).Error; err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *skillRepository) List(ctx context.Context, spaceID int64, typ *entity.Type, enabled *bool) ([]*entity.Skill, error) {
	query := r.db.WithContext(ctx).Where("space_id = ?", spaceID)
	if typ != nil {
		query = query.Where("type = ?", string(*typ))
	}
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}

	pos := make([]*skillPO, 0)
	if err := query.Order("updated_at DESC, id DESC").Find(&pos).Error; err != nil {
		return nil, err
	}

	skills := make([]*entity.Skill, 0, len(pos))
	for _, po := range pos {
		skills = append(skills, po.toEntity())
	}

	return skills, nil
}

func (r *skillRepository) CreateVersion(ctx context.Context, version *entity.SkillVersion) error {
	if version.ID == 0 {
		id, err := r.idGen.GenID(ctx)
		if err != nil {
			return err
		}
		version.ID = id
	}
	if version.CreatedAt == 0 {
		version.CreatedAt = time.Now().UnixMilli()
	}

	po, err := skillVersionToPO(version)
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Create(po).Error
}

func (r *skillRepository) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	pos := make([]*skillVersionPO, 0)
	if err := r.db.WithContext(ctx).
		Where("skill_id = ?", skillID).
		Order("created_at DESC, id DESC").
		Find(&pos).Error; err != nil {
		return nil, err
	}

	versions := make([]*entity.SkillVersion, 0, len(pos))
	for _, po := range pos {
		versions = append(versions, po.toEntity())
	}

	return versions, nil
}

func (r *skillRepository) CreateResources(ctx context.Context, resources []*entity.SkillResource) error {
	if len(resources) == 0 {
		return nil
	}

	missingIDs := 0
	for _, resource := range resources {
		if resource != nil && resource.ID == 0 {
			missingIDs++
		}
	}
	var ids []int64
	if missingIDs > 0 {
		generated, err := r.idGen.GenMultiIDs(ctx, missingIDs)
		if err != nil {
			return err
		}
		ids = generated
	}

	now := time.Now().UnixMilli()
	pos := make([]*skillResourcePO, 0, len(resources))
	idIndex := 0
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		if resource.ID == 0 {
			resource.ID = ids[idIndex]
			idIndex++
		}
		if resource.CreatedAt == 0 {
			resource.CreatedAt = now
		}
		pos = append(pos, skillResourceToPO(resource))
	}
	if len(pos) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).CreateInBatches(pos, 100).Error
}

func (r *skillRepository) ListResources(ctx context.Context, skillID, versionID int64) ([]*entity.SkillResource, error) {
	pos := make([]*skillResourcePO, 0)
	if err := r.db.WithContext(ctx).
		Where("skill_id = ? AND version_id = ?", skillID, versionID).
		Order("path ASC, id ASC").
		Find(&pos).Error; err != nil {
		return nil, err
	}

	resources := make([]*entity.SkillResource, 0, len(pos))
	for _, po := range pos {
		resources = append(resources, po.toEntity())
	}

	return resources, nil
}

func skillToPO(skill *entity.Skill) (*skillPO, error) {
	inputSchema, err := requiredJSON("input_schema", skill.InputSchema)
	if err != nil {
		return nil, err
	}
	outputSchema, err := requiredJSON("output_schema", skill.OutputSchema)
	if err != nil {
		return nil, err
	}
	executor, err := requiredJSON("executor", skill.Executor)
	if err != nil {
		return nil, err
	}
	permissions, err := requiredJSON("permissions", skill.Permissions)
	if err != nil {
		return nil, err
	}

	return &skillPO{
		ID:           skill.ID,
		SpaceID:      skill.SpaceID,
		Name:         skill.Name,
		Description:  skill.Description,
		Type:         string(skill.Type),
		Version:      skill.Version,
		Enabled:      skill.Enabled,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Executor:     executor,
		Permissions:  permissions,
		CreatedAt:    skill.CreatedAt,
		UpdatedAt:    skill.UpdatedAt,
	}, nil
}

func (po *skillPO) toEntity() *entity.Skill {
	return &entity.Skill{
		ID:           po.ID,
		SpaceID:      po.SpaceID,
		Name:         po.Name,
		Description:  po.Description,
		Type:         entity.Type(po.Type),
		Version:      po.Version,
		Enabled:      po.Enabled,
		InputSchema:  jsonToString(po.InputSchema),
		OutputSchema: jsonToString(po.OutputSchema),
		Executor:     jsonToString(po.Executor),
		Permissions:  jsonToString(po.Permissions),
		CreatedAt:    po.CreatedAt,
		UpdatedAt:    po.UpdatedAt,
	}
}

func skillVersionToPO(version *entity.SkillVersion) (*skillVersionPO, error) {
	inputSchema, err := requiredJSON("input_schema", version.InputSchema)
	if err != nil {
		return nil, err
	}
	outputSchema, err := requiredJSON("output_schema", version.OutputSchema)
	if err != nil {
		return nil, err
	}
	executor, err := requiredJSON("executor", version.Executor)
	if err != nil {
		return nil, err
	}
	permissions, err := requiredJSON("permissions", version.Permissions)
	if err != nil {
		return nil, err
	}

	return &skillVersionPO{
		ID:           version.ID,
		SkillID:      version.SkillID,
		Version:      version.Version,
		SkillMD:      version.SkillMD,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		Executor:     executor,
		Permissions:  permissions,
		CreatedAt:    version.CreatedAt,
	}, nil
}

func (po *skillVersionPO) toEntity() *entity.SkillVersion {
	return &entity.SkillVersion{
		ID:           po.ID,
		SkillID:      po.SkillID,
		Version:      po.Version,
		SkillMD:      po.SkillMD,
		InputSchema:  jsonToString(po.InputSchema),
		OutputSchema: jsonToString(po.OutputSchema),
		Executor:     jsonToString(po.Executor),
		Permissions:  jsonToString(po.Permissions),
		CreatedAt:    po.CreatedAt,
	}
}

func skillResourceToPO(resource *entity.SkillResource) *skillResourcePO {
	return &skillResourcePO{
		ID:        resource.ID,
		SkillID:   resource.SkillID,
		VersionID: resource.VersionID,
		Path:      resource.Path,
		Content:   append([]byte(nil), resource.Content...),
		Size:      resource.Size,
		SHA256:    resource.SHA256,
		CreatedAt: resource.CreatedAt,
	}
}

func (po *skillResourcePO) toEntity() *entity.SkillResource {
	return &entity.SkillResource{
		ID:        po.ID,
		SkillID:   po.SkillID,
		VersionID: po.VersionID,
		Path:      po.Path,
		Content:   append([]byte(nil), po.Content...),
		Size:      po.Size,
		SHA256:    po.SHA256,
		CreatedAt: po.CreatedAt,
	}
}

func requiredJSON(field, value string) (datatypes.JSON, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = "{}"
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("%s must be valid JSON", field)
	}

	return datatypes.JSON([]byte(trimmed)), nil
}

func jsonToString(value datatypes.JSON) string {
	if len(value) == 0 {
		return ""
	}

	return string(value)
}
