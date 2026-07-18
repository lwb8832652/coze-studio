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

package appdev_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	infraappdev "github.com/coze-dev/coze-studio/backend/infra/appdev"
)

type archiveLocalStoreForTest struct {
	*infraappdev.LocalStore

	mu      sync.Mutex
	intents map[string]*domainappdev.ProjectArchiveIntent
}

func newArchiveLocalStoreForTest(root string) *archiveLocalStoreForTest {
	return &archiveLocalStoreForTest{
		LocalStore: infraappdev.NewLocalStoreForTest(root),
		intents:    make(map[string]*domainappdev.ProjectArchiveIntent),
	}
}

func (s *archiveLocalStoreForTest) GetProject(
	ctx context.Context,
	spaceID string,
	projectID string,
) (*domainappdev.Project, error) {
	project, err := s.LocalStore.GetProject(ctx, spaceID, projectID)
	if err == nil && project.SourceVersion <= 0 {
		project.SourceVersion = 1
	}
	return project, err
}

func (s *archiveLocalStoreForTest) LoadProjectArchive(
	_ context.Context,
	input domainappdev.LoadProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	intent := s.intents[input.SpaceID+"/"+input.ProjectID]
	if intent == nil {
		return nil, domainappdev.ErrProjectArchiveNotFound
	}
	copy := *intent
	return &copy, nil
}

func (s *archiveLocalStoreForTest) ReserveProjectArchive(
	ctx context.Context,
	input domainappdev.ReserveProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	if err := domainappdev.ValidateReserveProjectArchiveInput(input); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := input.SpaceID + "/" + input.ProjectID
	if existing := s.intents[key]; existing != nil {
		if existing.SourceVersion != input.ExpectedSourceVersion ||
			existing.RuntimeGeneration != input.RuntimeGeneration {
			return nil, domainappdev.ErrProjectArchiveConflict
		}
		copy := *existing
		return &copy, nil
	}
	project, err := s.GetProject(ctx, input.SpaceID, input.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.SourceVersion != input.ExpectedSourceVersion {
		return nil, domainappdev.ErrProjectArchiveConflict
	}
	identity, err := domainappdev.NewProjectArchiveIntentIdentity(
		input.SpaceID,
		input.ProjectID,
		input.ExpectedSourceVersion,
		input.RuntimeGeneration,
		1,
	)
	if err != nil {
		return nil, err
	}
	intent := &domainappdev.ProjectArchiveIntent{
		ProjectArchiveIntentIdentity: identity,
		State:                        domainappdev.ProjectArchiveStateArchiving,
		StartedAt:                    time.Now().UTC(),
	}
	s.intents[key] = intent
	copy := *intent
	return &copy, nil
}

func (s *archiveLocalStoreForTest) CompleteProjectArchive(
	ctx context.Context,
	input domainappdev.CompleteProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	if err := domainappdev.ValidateCompleteProjectArchiveInput(input); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := input.Intent.SpaceID + "/" + input.Intent.ProjectID
	existing := s.intents[key]
	if existing == nil ||
		existing.OperationHash != input.Intent.OperationHash ||
		existing.IntentVersion != input.Intent.IntentVersion {
		return nil, domainappdev.ErrProjectArchiveConflict
	}
	if existing.State != domainappdev.ProjectArchiveStateArchived {
		if _, err := s.LocalStore.ArchiveProject(ctx, input.Intent.SpaceID, input.Intent.ProjectID); err != nil {
			return nil, err
		}
		existing.State = domainappdev.ProjectArchiveStateArchived
		existing.CompletedAt = time.Now().UTC()
	}
	copy := *existing
	return &copy, nil
}

func TestAppDevProjectAndFileFoundation(t *testing.T) {
	ctx := context.Background()
	store := infraappdev.NewLocalStoreForTest(t.TempDir())
	svc := applicationappdev.NewService(store)

	created, err := svc.CreateProject(ctx, &applicationappdev.CreateProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		Name:          "活动页",
		Prompt:        "做一个活动落地页",
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), created.Code)
	require.NotEmpty(t, created.Data.ID)
	require.NotEmpty(t, created.Data.SourceUpdatedAt)
	createdProject, err := store.GetProject(ctx, "1001", created.Data.ID)
	require.NoError(t, err)
	createdSourceUpdatedAt := createdProject.SourceUpdatedAt

	initialApp, err := svc.GetFileContent(ctx, &applicationappdev.FileContentRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		Path:          "src/App.tsx",
	})
	require.NoError(t, err)
	require.Contains(t, initialApp.Data.Content, "import React from 'react';")

	listed, err := svc.ListProjects(ctx, &applicationappdev.ListProjectsRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
	})
	require.NoError(t, err)
	require.Len(t, listed.Data.Items, 1)

	_, err = svc.GetProject(ctx, &applicationappdev.GetProjectRequest{
		SpaceID:       "2002",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
	})
	require.Error(t, err)

	files, err := svc.ListFiles(ctx, &applicationappdev.ProjectFileRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, files.Data.Items)

	_, err = svc.SaveFileContent(ctx, &applicationappdev.SaveFileContentRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		Path:          "../secret.txt",
		Content:       "nope",
	})
	require.ErrorIs(t, err, domainappdev.ErrInvalidPath)

	time.Sleep(time.Millisecond)
	saved, err := svc.SaveFileContent(ctx, &applicationappdev.SaveFileContentRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		Path:          "src/App.tsx",
		Content:       "export default function App() { return null; }",
	})
	require.NoError(t, err)
	require.Equal(t, "src/App.tsx", saved.Data.Path)
	require.Equal(t, "typescript", saved.Data.Language)

	afterSave, err := svc.GetProject(ctx, &applicationappdev.GetProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, afterSave.Data.SourceUpdatedAt)
	afterSaveProject, err := store.GetProject(ctx, "1001", created.Data.ID)
	require.NoError(t, err)
	require.True(t, afterSaveProject.SourceUpdatedAt.After(createdSourceUpdatedAt))

	_, err = store.UpdateProjectBuild(ctx, "1001", created.Data.ID, "success", "PAGE", ".coze-appdev/releases/latest", "ok", time.Now().UTC().Format(time.RFC3339))
	require.NoError(t, err)
	afterBuild, err := svc.GetProject(ctx, &applicationappdev.GetProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
	})
	require.NoError(t, err)
	afterBuildProject, err := store.GetProject(ctx, "1001", created.Data.ID)
	require.NoError(t, err)
	require.Equal(t, afterSaveProject.SourceUpdatedAt, afterBuildProject.SourceUpdatedAt)
	require.Equal(t, "success", afterBuild.Data.LastBuildStatus)

	updated, err := svc.UpdateProject(ctx, &applicationappdev.UpdateProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		Name:          "新版活动页",
		Description:   "已改名",
	})
	require.NoError(t, err)
	require.Equal(t, "新版活动页", updated.Data.Name)
	require.Equal(t, "已改名", updated.Data.Description)

	duplicated, err := svc.DuplicateProject(ctx, &applicationappdev.DuplicateProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		Name:          "活动页副本",
	})
	require.NoError(t, err)
	require.NotEqual(t, created.Data.ID, duplicated.Data.ID)
	require.Equal(t, "活动页副本", duplicated.Data.Name)
}

func TestAppDevProjectImportExportArchiveAndModels(t *testing.T) {
	ctx := context.Background()
	store := newArchiveLocalStoreForTest(t.TempDir())
	svc := applicationappdev.NewService(store)

	imported, err := svc.ImportProject(ctx, &applicationappdev.ImportProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		Name:          "导入应用",
		FileName:      "imported.zip",
		Archive: buildTestZip(t, map[string]string{
			"package.json": `{"scripts":{"dev":"vite"}}`,
			"src/App.tsx":  "export default function App() { return null; }",
			"src/main.tsx": "console.log('ok');",
		}),
	})
	require.NoError(t, err)
	require.Equal(t, "导入应用", imported.Data.Name)

	_, err = svc.ImportProject(ctx, &applicationappdev.ImportProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		FileName:      "bad.zip",
		Archive: buildTestZip(t, map[string]string{
			"../secret.txt": "nope",
		}),
	})
	require.Error(t, err)

	projectFilesDir, err := store.ProjectFilesDir("1001", imported.Data.ID)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(projectFilesDir, "node_modules/pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectFilesDir, "node_modules/pkg/index.js"), []byte("ignored"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(projectFilesDir, ".coze-appdev/releases/latest"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectFilesDir, ".coze-appdev/releases/latest/index.html"), []byte("ignored"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(projectFilesDir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectFilesDir, ".git/config"), []byte("ignored"), 0o644))

	exported, err := svc.ExportProject(ctx, &applicationappdev.ExportProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     imported.Data.ID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, exported.Content)
	require.Equal(t, "application/zip", exported.ContentType)
	require.Equal(t, map[string]bool{
		"index.html":   true,
		"package.json": true,
		"src/App.tsx":  true,
		"src/main.tsx": true,
	}, zipEntryNames(t, exported.Content))

	require.NoError(t, svc.SetProjectRuntimeLifecycleMode(applicationappdev.ProjectRuntimeLifecycleModeProviderRequired))
	publication, publishErr := svc.PublishProviderRuntimeBindings(&applicationappdev.ProviderAPIFacade{}, applicationappdev.ProviderProjectArchiveLifecycleFunc{
		Current: func(context.Context, string, string) (uint64, error) {
			return 0, nil
		},
		Prepare: func(context.Context, applicationappdev.ProviderProjectArchiveInput) error {
			return nil
		},
	})
	require.NoError(t, publishErr)
	defer svc.UnpublishProviderRuntimeBindings(publication)
	_, err = svc.ArchiveProject(ctx, &applicationappdev.ArchiveProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     imported.Data.ID,
	})
	require.NoError(t, err)

	listed, err := svc.ListProjects(ctx, &applicationappdev.ListProjectsRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
	})
	require.NoError(t, err)
	require.Empty(t, listed.Data.Items)

	models, err := svc.ListModels(ctx, &applicationappdev.ListModelsRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		Scenario:      "PageApp",
	})
	require.NoError(t, err)
	require.Empty(t, models.Data.Items)

	_, err = svc.ListModels(ctx, &applicationappdev.ListModelsRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		Scenario:      "Unsupported",
	})
	require.Error(t, err)
}

func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	buffer := bytes.NewBuffer(nil)
	writer := zip.NewWriter(buffer)
	for path, content := range files {
		fileWriter, err := writer.Create(path)
		require.NoError(t, err)
		_, err = fileWriter.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	return buffer.Bytes()
}

func zipEntryNames(t *testing.T, archive []byte) map[string]bool {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	require.NoError(t, err)

	names := make(map[string]bool, len(reader.File))
	for _, file := range reader.File {
		names[file.Name] = true
	}
	return names
}
