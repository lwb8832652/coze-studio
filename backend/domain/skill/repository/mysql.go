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
