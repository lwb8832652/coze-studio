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

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	"github.com/coze-dev/coze-studio/backend/domain/skill/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type SkillService interface {
	ImportDeclaration(ctx context.Context, spaceID int64, fileName string, content []byte) (*entity.Skill, error)
	ImportDeclarationWithDefaultType(ctx context.Context, spaceID int64, fileName string, content []byte, defaultType entity.Type) (*entity.Skill, error)
	ImportDeclarationWithDevelopmentThread(ctx context.Context, spaceID int64, fileName string, content []byte, defaultType entity.Type, developmentThreadID int64) (*entity.Skill, error)
	Create(ctx context.Context, skill *entity.Skill) (*entity.Skill, error)
	Update(ctx context.Context, skill *entity.Skill) (*entity.Skill, error)
	UpdateWithExpectedVersion(ctx context.Context, skill *entity.Skill, expectedVersionID int64) (*entity.Skill, error)
	Delete(ctx context.Context, id int64) (*entity.Skill, error)
	Get(ctx context.Context, id int64) (*entity.Skill, error)
	List(ctx context.Context, spaceID int64, typ *entity.Type, enabled *bool) ([]*entity.Skill, error)
	ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error)
	ListVersionResources(ctx context.Context, skillID, versionID int64) ([]*entity.SkillResource, error)
	UpdateVersionContent(ctx context.Context, skillID, versionID int64, skillMD string) (*entity.SkillVersion, error)
	UpdateVersionResource(ctx context.Context, skillID, versionID int64, path string, content []byte) (*entity.SkillVersion, error)
	MutateVersionResources(ctx context.Context, skillID, versionID int64, mutations []ResourceMutation) (*entity.SkillVersion, error)
	RollbackVersion(ctx context.Context, skillID, versionID int64) (*entity.Skill, error)
	RollbackVersionCAS(ctx context.Context, skillID, versionID, expectedVersionID int64) (*entity.Skill, error)
	TestRun(ctx context.Context, id int64, input string) (string, error)
}

type ResourceMutationOperation int

const (
	ResourceMutationUpsert ResourceMutationOperation = iota + 1
	ResourceMutationDelete
	ResourceMutationMove
)

type ResourceMutation struct {
	Operation  ResourceMutationOperation
	Path       string
	TargetPath string
	Content    []byte
}

type Components struct {
	Repo           repository.SkillRepository
	IDGen          idgen.IDGenerator
	ScriptRunner   Executor
	WorkflowRunner Executor
}
