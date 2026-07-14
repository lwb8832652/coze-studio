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
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	domain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

const deerFlowBuiltinSkillRoot = "builtin_deerflow/public"

//go:embed builtin_deerflow/public/**
var deerFlowBuiltinSkillFS embed.FS

type deerFlowBuiltinSkillService struct {
	delegate domain.SkillService

	mu            sync.Mutex
	ensuredSpaces map[int64]struct{}
}

type deerFlowBuiltinSkillBundle struct {
	name string
	root string
}

func newDeerFlowBuiltinSkillService(delegate domain.SkillService) domain.SkillService {
	if delegate == nil {
		return nil
	}
	return &deerFlowBuiltinSkillService{
		delegate:      delegate,
		ensuredSpaces: map[int64]struct{}{},
	}
}

func (s *deerFlowBuiltinSkillService) ImportDeclaration(ctx context.Context, spaceID int64, fileName string, content []byte) (*entity.Skill, error) {
	return s.delegate.ImportDeclaration(ctx, spaceID, fileName, content)
}

func (s *deerFlowBuiltinSkillService) ImportDeclarationWithDefaultType(ctx context.Context, spaceID int64, fileName string, content []byte, defaultType entity.Type) (*entity.Skill, error) {
	return s.delegate.ImportDeclarationWithDefaultType(ctx, spaceID, fileName, content, defaultType)
}

func (s *deerFlowBuiltinSkillService) ImportDeclarationWithDevelopmentThread(ctx context.Context, spaceID int64, fileName string, content []byte, defaultType entity.Type, developmentThreadID int64) (*entity.Skill, error) {
	return s.delegate.ImportDeclarationWithDevelopmentThread(ctx, spaceID, fileName, content, defaultType, developmentThreadID)
}

func (s *deerFlowBuiltinSkillService) Create(ctx context.Context, skill *entity.Skill) (*entity.Skill, error) {
	return s.delegate.Create(ctx, skill)
}

func (s *deerFlowBuiltinSkillService) Update(ctx context.Context, skill *entity.Skill) (*entity.Skill, error) {
	return s.delegate.Update(ctx, skill)
}

func (s *deerFlowBuiltinSkillService) UpdateWithExpectedVersion(ctx context.Context, skill *entity.Skill, expectedVersionID int64) (*entity.Skill, error) {
	return s.delegate.UpdateWithExpectedVersion(ctx, skill, expectedVersionID)
}

func (s *deerFlowBuiltinSkillService) Delete(ctx context.Context, id int64) (*entity.Skill, error) {
	return s.delegate.Delete(ctx, id)
}

func (s *deerFlowBuiltinSkillService) Get(ctx context.Context, id int64) (*entity.Skill, error) {
	return s.delegate.Get(ctx, id)
}

func (s *deerFlowBuiltinSkillService) List(ctx context.Context, spaceID int64, typ *entity.Type, enabled *bool) ([]*entity.Skill, error) {
	if shouldEnsureDeerFlowBuiltinSkills(spaceID, typ) {
		if err := s.ensureDeerFlowBuiltinSkills(ctx, spaceID); err != nil {
			return nil, err
		}
	}
	return s.delegate.List(ctx, spaceID, typ, enabled)
}

func (s *deerFlowBuiltinSkillService) ListVersions(ctx context.Context, skillID int64) ([]*entity.SkillVersion, error) {
	return s.delegate.ListVersions(ctx, skillID)
}

func (s *deerFlowBuiltinSkillService) ListVersionResources(ctx context.Context, skillID, versionID int64) ([]*entity.SkillResource, error) {
	return s.delegate.ListVersionResources(ctx, skillID, versionID)
}

func (s *deerFlowBuiltinSkillService) UpdateVersionContent(ctx context.Context, skillID, versionID int64, skillMD string) (*entity.SkillVersion, error) {
	return s.delegate.UpdateVersionContent(ctx, skillID, versionID, skillMD)
}

func (s *deerFlowBuiltinSkillService) UpdateVersionResource(ctx context.Context, skillID, versionID int64, resourcePath string, content []byte) (*entity.SkillVersion, error) {
	return s.delegate.UpdateVersionResource(ctx, skillID, versionID, resourcePath, content)
}

func (s *deerFlowBuiltinSkillService) MutateVersionResources(ctx context.Context, skillID, versionID int64, mutations []domain.ResourceMutation) (*entity.SkillVersion, error) {
	return s.delegate.MutateVersionResources(ctx, skillID, versionID, mutations)
}

func (s *deerFlowBuiltinSkillService) RollbackVersion(ctx context.Context, skillID, versionID int64) (*entity.Skill, error) {
	return s.delegate.RollbackVersion(ctx, skillID, versionID)
}

func (s *deerFlowBuiltinSkillService) RollbackVersionCAS(ctx context.Context, skillID, versionID, expectedVersionID int64) (*entity.Skill, error) {
	return s.delegate.RollbackVersionCAS(ctx, skillID, versionID, expectedVersionID)
}

func (s *deerFlowBuiltinSkillService) TestRun(ctx context.Context, id int64, input string) (string, error) {
	return s.delegate.TestRun(ctx, id, input)
}

func shouldEnsureDeerFlowBuiltinSkills(spaceID int64, typ *entity.Type) bool {
	if spaceID <= 0 {
		return false
	}
	if typ == nil {
		return true
	}
	return *typ == entity.TypeDeerSkill
}

func (s *deerFlowBuiltinSkillService) ensureDeerFlowBuiltinSkills(ctx context.Context, spaceID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.ensuredSpaces[spaceID]; ok {
		return nil
	}

	existingSkills, err := s.delegate.List(ctx, spaceID, nil, nil)
	if err != nil {
		return err
	}
	existingNames := make(map[string]struct{}, len(existingSkills))
	for _, skill := range existingSkills {
		if skill == nil {
			continue
		}
		existingNames[skill.Name] = struct{}{}
	}

	bundles, err := deerFlowBuiltinSkillBundles()
	if err != nil {
		return err
	}
	for _, bundle := range bundles {
		if _, exists := existingNames[bundle.name]; exists {
			continue
		}
		archiveBytes, err := deerFlowBuiltinSkillArchive(bundle)
		if err != nil {
			return err
		}
		if _, err := s.delegate.ImportDeclaration(ctx, spaceID, bundle.name+".skill", archiveBytes); err != nil {
			return fmt.Errorf("import DeerFlow builtin skill %s: %w", bundle.name, err)
		}
		existingNames[bundle.name] = struct{}{}
	}

	s.ensuredSpaces[spaceID] = struct{}{}
	return nil
}

func deerFlowBuiltinSkillBundles() ([]deerFlowBuiltinSkillBundle, error) {
	entries, err := fs.ReadDir(deerFlowBuiltinSkillFS, deerFlowBuiltinSkillRoot)
	if err != nil {
		return nil, fmt.Errorf("read DeerFlow builtin skills: %w", err)
	}

	bundles := make([]deerFlowBuiltinSkillBundle, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := path.Join(deerFlowBuiltinSkillRoot, entry.Name())
		skillMD, err := deerFlowBuiltinSkillFS.ReadFile(path.Join(root, "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("read DeerFlow builtin skill %s/SKILL.md: %w", entry.Name(), err)
		}
		decl, err := domain.ParseDeclaration("SKILL.md", skillMD)
		if err != nil {
			return nil, fmt.Errorf("parse DeerFlow builtin skill %s/SKILL.md: %w", entry.Name(), err)
		}
		bundles = append(bundles, deerFlowBuiltinSkillBundle{
			name: decl.Name,
			root: root,
		})
	}

	sort.Slice(bundles, func(i, j int) bool {
		return bundles[i].name < bundles[j].name
	})
	return bundles, nil
}

func deerFlowBuiltinSkillArchive(bundle deerFlowBuiltinSkillBundle) ([]byte, error) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	err := fs.WalkDir(deerFlowBuiltinSkillFS, bundle.root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relativePath := strings.TrimPrefix(filePath, bundle.root+"/")
		if relativePath == filePath || relativePath == "" {
			return fmt.Errorf("invalid DeerFlow builtin skill path: %s", filePath)
		}
		content, err := deerFlowBuiltinSkillFS.ReadFile(filePath)
		if err != nil {
			return err
		}
		header := &zip.FileHeader{
			Name:   path.Join(bundle.name, relativePath),
			Method: zip.Deflate,
		}
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = writer.Write(content)
		return err
	})
	if err != nil {
		_ = zipWriter.Close()
		return nil, fmt.Errorf("package DeerFlow builtin skill %s: %w", bundle.name, err)
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
