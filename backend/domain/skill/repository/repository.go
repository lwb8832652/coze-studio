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
	"errors"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
)

var ErrVersionConflict = errors.New("skill latest version conflict")

type SkillRepository interface {
	Create(ctx context.Context, skill *entity.Skill) error
	Update(ctx context.Context, skill *entity.Skill) error
	CreateWithVersion(ctx context.Context, skill *entity.Skill, version *entity.SkillVersion, resources []*entity.SkillResource) error
	UpdateWithVersion(ctx context.Context, skill *entity.Skill, version *entity.SkillVersion, resources []*entity.SkillResource) error
	UpdateWithVersionCAS(ctx context.Context, skill *entity.Skill, expectedVersionID int64, version *entity.SkillVersion, resources []*entity.SkillResource) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*entity.Skill, error)
	List(ctx context.Context, spaceID int64, typ *entity.Type, enabled *bool) ([]*entity.Skill, error)
	CreateVersion(ctx context.Context, version *entity.SkillVersion) error
	ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error)
	GetLatestVersion(ctx context.Context, skillID int64) (*entity.SkillVersion, error)
	CreateResources(ctx context.Context, resources []*entity.SkillResource) error
	ListResources(ctx context.Context, skillID, versionID int64) ([]*entity.SkillResource, error)
}
