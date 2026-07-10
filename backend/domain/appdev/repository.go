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

package appdev

import "context"

type Store interface {
	ListProjects(ctx context.Context, spaceID string, keyword string) ([]*Project, error)
	CreateProject(ctx context.Context, project *Project, initialFiles map[string]string) (*Project, error)
	ImportProjectArchive(ctx context.Context, project *Project, archive []byte) (*Project, error)
	GetProject(ctx context.Context, spaceID string, projectID string) (*Project, error)
	UpdateProject(ctx context.Context, spaceID string, projectID string, name string, description string) (*Project, error)
	ArchiveProject(ctx context.Context, spaceID string, projectID string) (*Project, error)
	ExportProjectArchive(ctx context.Context, spaceID string, projectID string) ([]byte, string, error)
	ListFiles(ctx context.Context, spaceID string, projectID string) ([]*FileNode, error)
	GetFileContent(ctx context.Context, spaceID string, projectID string, filePath string) (*FileContent, error)
	SaveFileContent(ctx context.Context, spaceID string, projectID string, filePath string, content string) (*FileContent, error)
	SaveUploadedFiles(ctx context.Context, spaceID string, projectID string, files []UploadedFile) (int, error)
	DeletePath(ctx context.Context, spaceID string, projectID string, filePath string) error
	RenamePath(ctx context.Context, spaceID string, projectID string, sourcePath string, targetPath string) error
	CreateProjectSnapshot(ctx context.Context, spaceID string, projectID string, label string) (*ProjectSnapshot, error)
	ListProjectSnapshots(ctx context.Context, spaceID string, projectID string) ([]*ProjectSnapshot, error)
	RestoreProjectSnapshot(ctx context.Context, spaceID string, projectID string, snapshotID string) error
	ProjectFilesDir(spaceID string, projectID string) (string, error)
	UpdateProjectRuntime(ctx context.Context, spaceID string, projectID string, status RuntimeStatus, previewURL string) (*Project, error)
	UpdateProjectBuild(ctx context.Context, spaceID string, projectID string, status string, publishType string, artifactPath string, message string, buildAt string) (*Project, error)
}
