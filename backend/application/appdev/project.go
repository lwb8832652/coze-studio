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

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type Service struct {
	store   domainappdev.Store
	runtime RuntimeManager
	chat    ChatManager
}

var SVC = &Service{}

func InitService(store domainappdev.Store, runtime RuntimeManager, chat ChatManager) *Service {
	initialized := NewServiceWithChat(store, runtime, chat)
	*SVC = *initialized
	return SVC
}

func NewService(store domainappdev.Store, runtime ...RuntimeManager) *Service {
	svc := &Service{store: store}
	if len(runtime) > 0 {
		svc.runtime = runtime[0]
	}
	return svc
}

func NewServiceWithChat(store domainappdev.Store, runtime RuntimeManager, chat ChatManager) *Service {
	return &Service{
		store:   store,
		runtime: runtime,
		chat:    chat,
	}
}

type ListProjectsRequest struct {
	SpaceID       string
	CurrentUserID int64
	Keyword       string
}

type CreateProjectRequest struct {
	SpaceID       string
	CurrentUserID int64
	Name          string
	Prompt        string
}

type GetProjectRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
}

type UpdateProjectRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	Name          string
	Description   string
}

type DuplicateProjectRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	Name          string
}

type ImportProjectRequest struct {
	SpaceID       string
	CurrentUserID int64
	Name          string
	FileName      string
	Archive       []byte
}

type ExportProjectRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
}

type ArchiveProjectRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
}

type ProjectDTO struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	Prompt            string `json:"prompt,omitempty"`
	Status            string `json:"status"`
	RuntimeStatus     string `json:"runtimeStatus,omitempty"`
	PreviewURL        string `json:"previewUrl,omitempty"`
	LastBuildStatus   string `json:"lastBuildStatus,omitempty"`
	LastBuildType     string `json:"lastBuildType,omitempty"`
	LastBuildArtifact string `json:"lastBuildArtifact,omitempty"`
	LastBuildMessage  string `json:"lastBuildMessage,omitempty"`
	LastBuildAt       string `json:"lastBuildAt,omitempty"`
	SourceUpdatedAt   string `json:"sourceUpdatedAt,omitempty"`
	CreatorName       string `json:"creatorName,omitempty"`
	UpdatedAt         string `json:"updatedAt,omitempty"`
	CreatedAt         string `json:"createdAt,omitempty"`
}

type ProjectPageData struct {
	Items []*ProjectDTO `json:"items"`
	Total int           `json:"total"`
}

type ListProjectsResponse struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    ProjectPageData `json:"data"`
}

type ProjectResponse struct {
	Code    int64       `json:"code"`
	Message string      `json:"message"`
	Data    *ProjectDTO `json:"data"`
}

type ExportProjectData struct {
	FileName    string
	ContentType string
	Content     []byte
}

type ActionResponse struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    map[string]bool `json:"data"`
}

func (s *Service) ListProjects(ctx context.Context, req *ListProjectsRequest) (*ListProjectsResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}

	projects, err := s.store.ListProjects(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.Keyword))
	if err != nil {
		return nil, err
	}

	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].UpdatedAt.After(projects[j].UpdatedAt)
	})

	items := make([]*ProjectDTO, 0, len(projects))
	for _, project := range projects {
		if project.Status == domainappdev.ProjectStatusArchived {
			continue
		}
		items = append(items, toProjectDTO(project))
	}

	return &ListProjectsResponse{
		Code:    0,
		Message: "success",
		Data: ProjectPageData{
			Items: items,
			Total: len(items),
		},
	}, nil
}

func (s *Service) CreateProject(ctx context.Context, req *CreateProjectRequest) (*ProjectResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	prompt := strings.TrimSpace(req.Prompt)
	if name == "" {
		return nil, fmt.Errorf("project name cannot be empty")
	}
	if prompt == "" {
		return nil, fmt.Errorf("project prompt cannot be empty")
	}
	if len(name) > 50 {
		return nil, fmt.Errorf("project name cannot exceed 50 characters")
	}
	if len(prompt) > 2000 {
		return nil, fmt.Errorf("project prompt cannot exceed 2000 characters")
	}

	now := time.Now().UTC()
	project := &domainappdev.Project{
		ID:              newProjectID(),
		SpaceID:         strings.TrimSpace(req.SpaceID),
		Name:            name,
		Prompt:          prompt,
		Status:          domainappdev.ProjectStatusReady,
		RuntimeStatus:   domainappdev.RuntimeStatusStopped,
		CreatorID:       strconv.FormatInt(req.CurrentUserID, 10),
		CreatedAt:       now,
		SourceUpdatedAt: now,
		UpdatedAt:       now,
	}

	created, err := s.store.CreateProject(ctx, project, initialProjectFiles(name, prompt))
	if err != nil {
		return nil, err
	}

	return successProjectResponse(created), nil
}

func (s *Service) ImportProject(ctx context.Context, req *ImportProjectRequest) (*ProjectResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}

	if len(req.Archive) == 0 {
		return nil, fmt.Errorf("project archive cannot be empty")
	}
	if len(req.Archive) > 100*1024*1024 {
		return nil, fmt.Errorf("project archive cannot exceed 100MB")
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(req.FileName)), ".zip") {
		return nil, fmt.Errorf("only .zip project archive is supported")
	}
	if _, err := zip.NewReader(bytes.NewReader(req.Archive), int64(len(req.Archive))); err != nil {
		return nil, fmt.Errorf("invalid project archive: %w", err)
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = projectNameFromArchive(req.FileName)
	}
	if name == "" {
		name = "导入的网页应用"
	}
	if len([]rune(name)) > 50 {
		return nil, fmt.Errorf("project name cannot exceed 50 characters")
	}

	now := time.Now().UTC()
	project := &domainappdev.Project{
		ID:              newProjectID(),
		SpaceID:         strings.TrimSpace(req.SpaceID),
		Name:            name,
		Description:     "从 zip 项目包导入",
		Status:          domainappdev.ProjectStatusReady,
		RuntimeStatus:   domainappdev.RuntimeStatusStopped,
		CreatorID:       strconv.FormatInt(req.CurrentUserID, 10),
		CreatedAt:       now,
		SourceUpdatedAt: now,
		UpdatedAt:       now,
	}

	created, err := s.store.ImportProjectArchive(ctx, project, req.Archive)
	if err != nil {
		return nil, err
	}

	return successProjectResponse(created), nil
}

func (s *Service) ExportProject(ctx context.Context, req *ExportProjectRequest) (*ExportProjectData, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}

	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("project_id cannot be empty")
	}

	content, fileName, err := s.store.ExportProjectArchive(ctx, strings.TrimSpace(req.SpaceID), projectID)
	if err != nil {
		return nil, err
	}

	return &ExportProjectData{
		FileName:    fileName,
		ContentType: "application/zip",
		Content:     content,
	}, nil
}

func (s *Service) ArchiveProject(ctx context.Context, req *ArchiveProjectRequest) (*ActionResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}

	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("project_id cannot be empty")
	}
	if s.runtime != nil {
		if _, err := s.StopRuntime(ctx, &RuntimeRequest{
			SpaceID:       strings.TrimSpace(req.SpaceID),
			CurrentUserID: req.CurrentUserID,
			ProjectID:     projectID,
		}); err != nil {
			return nil, fmt.Errorf("stop appdev runtime before archive: %w", err)
		}
	}

	if _, err := s.store.ArchiveProject(ctx, strings.TrimSpace(req.SpaceID), projectID); err != nil {
		return nil, err
	}

	return &ActionResponse{
		Code:    0,
		Message: "success",
		Data:    map[string]bool{"success": true},
	}, nil
}

func (s *Service) GetProject(ctx context.Context, req *GetProjectRequest) (*ProjectResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}

	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("project_id cannot be empty")
	}

	project, err := s.store.GetProject(ctx, strings.TrimSpace(req.SpaceID), projectID)
	if err != nil {
		return nil, err
	}

	return successProjectResponse(project), nil
}

func (s *Service) UpdateProject(ctx context.Context, req *UpdateProjectRequest) (*ProjectResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}

	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("project_id cannot be empty")
	}

	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	if name == "" {
		return nil, fmt.Errorf("project name cannot be empty")
	}
	if len([]rune(name)) > 50 {
		return nil, fmt.Errorf("project name cannot exceed 50 characters")
	}
	if len([]rune(description)) > 200 {
		return nil, fmt.Errorf("project description cannot exceed 200 characters")
	}

	project, err := s.store.UpdateProject(ctx, strings.TrimSpace(req.SpaceID), projectID, name, description)
	if err != nil {
		return nil, err
	}

	return successProjectResponse(project), nil
}

func (s *Service) DuplicateProject(ctx context.Context, req *DuplicateProjectRequest) (*ProjectResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}

	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("project_id cannot be empty")
	}

	sourceProject, err := s.store.GetProject(ctx, strings.TrimSpace(req.SpaceID), projectID)
	if err != nil {
		return nil, err
	}
	if sourceProject.Status == domainappdev.ProjectStatusArchived {
		return nil, fmt.Errorf("archived project cannot be duplicated")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = sourceProject.Name + " 副本"
	}
	if len([]rune(name)) > 50 {
		return nil, fmt.Errorf("project name cannot exceed 50 characters")
	}

	archive, _, err := s.store.ExportProjectArchive(ctx, strings.TrimSpace(req.SpaceID), projectID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	project := &domainappdev.Project{
		ID:              newProjectID(),
		SpaceID:         strings.TrimSpace(req.SpaceID),
		Name:            name,
		Description:     "复制自 " + sourceProject.Name,
		Prompt:          sourceProject.Prompt,
		Status:          domainappdev.ProjectStatusReady,
		RuntimeStatus:   domainappdev.RuntimeStatusStopped,
		CreatorID:       strconv.FormatInt(req.CurrentUserID, 10),
		CreatedAt:       now,
		SourceUpdatedAt: now,
		UpdatedAt:       now,
	}

	created, err := s.store.ImportProjectArchive(ctx, project, archive)
	if err != nil {
		return nil, err
	}

	return successProjectResponse(created), nil
}

func validateUserAndSpace(currentUserID int64, spaceID string) error {
	if currentUserID <= 0 {
		return fmt.Errorf("missing user session")
	}
	if strings.TrimSpace(spaceID) == "" {
		return fmt.Errorf("space_id is required")
	}
	return nil
}

func successProjectResponse(project *domainappdev.Project) *ProjectResponse {
	return &ProjectResponse{
		Code:    0,
		Message: "success",
		Data:    toProjectDTO(project),
	}
}

func toProjectDTO(project *domainappdev.Project) *ProjectDTO {
	if project == nil {
		return nil
	}

	return &ProjectDTO{
		ID:                project.ID,
		Name:              project.Name,
		Description:       project.Description,
		Prompt:            project.Prompt,
		Status:            string(project.Status),
		RuntimeStatus:     string(project.RuntimeStatus),
		PreviewURL:        project.PreviewURL,
		LastBuildStatus:   project.LastBuildStatus,
		LastBuildType:     project.LastBuildType,
		LastBuildArtifact: project.LastBuildArtifact,
		LastBuildMessage:  project.LastBuildMessage,
		LastBuildAt:       formatTime(project.LastBuildAt),
		SourceUpdatedAt:   formatTime(project.SourceUpdatedAt),
		CreatorName:       project.CreatorName,
		UpdatedAt:         formatTime(project.UpdatedAt),
		CreatedAt:         formatTime(project.CreatedAt),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func newProjectID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("appdev_%d", time.Now().UnixNano())
	}
	return "appdev_" + hex.EncodeToString(b[:])
}

func initialProjectFiles(name string, prompt string) map[string]string {
	return map[string]string{
		"README.md": fmt.Sprintf("# %s\n\n%s\n", name, prompt),
		"package.json": `{
  "scripts": {
    "dev": "vite --host 0.0.0.0"
  },
  "dependencies": {
    "@vitejs/plugin-react": "^4.3.4",
    "vite": "^5.4.14",
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "typescript": "^5.8.2"
  },
  "devDependencies": {}
}
`,
		"src/App.tsx": fmt.Sprintf(`import React from 'react';

const initialRequirement = %s;

export default function App() {
  return (
    <main
      style={{
        minHeight: '100vh',
        padding: 48,
        color: '#172033',
        background: 'linear-gradient(135deg, #f8fafc 0%%, #eef2ff 100%%)',
        fontFamily: 'Avenir Next, PingFang SC, sans-serif',
      }}
    >
      <section
        style={{
          maxWidth: 920,
          margin: '0 auto',
          padding: 36,
          borderRadius: 28,
          background: 'rgba(255,255,255,0.88)',
          boxShadow: '0 20px 70px rgba(30, 64, 175, 0.12)',
        }}
      >
        <p style={{ margin: 0, color: '#2563eb', fontWeight: 700 }}>
          Web App Starter
        </p>
        <h1 style={{ margin: '14px 0', fontSize: 42, lineHeight: 1.12 }}>
          %s
        </h1>
        <p style={{ color: '#475569', fontSize: 16, lineHeight: 1.8 }}>
          {initialRequirement}
        </p>
        <p style={{ marginTop: 28, color: '#64748b' }}>
          继续在左侧向 AI 描述修改需求，即可迭代这个网页应用。
        </p>
      </section>
    </main>
  );
}
`, strconv.Quote(prompt), html.EscapeString(name)),
		"src/main.tsx": `import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
`,
		"index.html": `<div id="root"></div><script type="module" src="/src/main.tsx"></script>
`,
	}
}

func projectNameFromArchive(fileName string) string {
	name := strings.TrimSpace(fileName)
	if strings.HasSuffix(strings.ToLower(name), ".zip") {
		name = name[:len(name)-4]
	}
	name = strings.TrimSpace(name)
	runes := []rune(name)
	if len(runes) > 50 {
		return string(runes[:50])
	}
	return name
}
