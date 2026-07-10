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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

const appDevReleasePath = ".coze-appdev/releases/latest"

type projectBuildLock struct {
	mu   sync.Mutex
	refs int
}

type projectBuildLockRegistry struct {
	mu    sync.Mutex
	locks map[string]*projectBuildLock
}

func newProjectBuildLockRegistry() *projectBuildLockRegistry {
	return &projectBuildLockRegistry{locks: make(map[string]*projectBuildLock)}
}

func (r *projectBuildLockRegistry) lock(spaceID string, projectID string) func() {
	key := strings.TrimSpace(spaceID) + "\x00" + strings.TrimSpace(projectID)

	r.mu.Lock()
	entry := r.locks[key]
	if entry == nil {
		entry = &projectBuildLock{}
		r.locks[key] = entry
	}
	entry.refs++
	r.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		r.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(r.locks, key)
		}
		r.mu.Unlock()
	}
}

var appDevBuildLocks = newProjectBuildLockRegistry()

type BuildProjectRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	PublishType   string
}

type BuildProjectData struct {
	Status       string `json:"status"`
	PublishType  string `json:"publishType"`
	ArtifactPath string `json:"artifactPath"`
	DownloadURL  string `json:"downloadUrl"`
	Message      string `json:"message,omitempty"`
	StartedAt    string `json:"startedAt"`
	FinishedAt   string `json:"finishedAt"`
	DurationMs   int64  `json:"durationMs"`
}

type BuildProjectResponse struct {
	Code    int64            `json:"code"`
	Message string           `json:"message"`
	Data    BuildProjectData `json:"data"`
}

type IsolatedBuildRunner interface {
	Build(ctx context.Context, req *IsolatedBuildRequest) (*IsolatedBuildResult, error)
}

type IsolatedBuildRequest struct {
	SpaceID     string
	ProjectID   string
	PublishType string
	SourceURL   string
}

type IsolatedBuildResult struct {
	Status            string
	ArtifactObjectKey string
	Message           string
	StartedAt         time.Time
	FinishedAt        time.Time
}

type BuildArtifactProvider interface {
	GetBuildArtifact(ctx context.Context, spaceID string, projectID string) ([]byte, error)
}

type BuildArtifactWriter interface {
	SaveBuildArtifact(ctx context.Context, spaceID string, projectID string, content []byte) (string, error)
}

func (s *Service) BuildProject(ctx context.Context, req *BuildProjectRequest) (*BuildProjectResponse, error) {
	if err := validateUserAndSpace(req.CurrentUserID, req.SpaceID); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("project_id cannot be empty")
	}
	publishType := strings.TrimSpace(req.PublishType)
	if publishType == "" {
		publishType = "page"
	}
	releaseBuildLock := appDevBuildLocks.lock(req.SpaceID, projectID)
	defer releaseBuildLock()

	if !IsAppDevHostExecutionEnabled() {
		return s.buildProjectWithRunner(ctx, req, projectID, publishType)
	}

	projectDir, err := s.store.ProjectFilesDir(strings.TrimSpace(req.SpaceID), projectID)
	if err != nil {
		return nil, err
	}

	startedAt := time.Now().UTC()
	if _, err := s.store.UpdateProjectBuild(ctx, strings.TrimSpace(req.SpaceID), projectID, "building", publishType, "", "发布构建中", startedAt.Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("persist appdev build start: %w", err)
	}

	buildCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	artifactFullPath := filepath.Join(projectDir, filepath.FromSlash(appDevReleasePath))
	if err := os.RemoveAll(artifactFullPath); err != nil {
		return nil, s.markBuildFailed(ctx, req, publishType, startedAt, fmt.Errorf("清理旧构建产物失败: %w", err))
	}
	if err := os.MkdirAll(filepath.Dir(artifactFullPath), 0o755); err != nil {
		return nil, s.markBuildFailed(ctx, req, publishType, startedAt, fmt.Errorf("创建构建目录失败: %w", err))
	}
	if err := writeBuildViteConfig(projectDir); err != nil {
		return nil, s.markBuildFailed(ctx, req, publishType, startedAt, err)
	}
	if err := ensureBuildDependencies(buildCtx, projectDir); err != nil {
		return nil, s.markBuildFailed(ctx, req, publishType, startedAt, err)
	}
	if err := runAppDevCommand(buildCtx, projectDir, "./node_modules/.bin/vite", "build", "--config", ".coze-appdev/vite.build.config.mjs", "--outDir", appDevReleasePath, "--emptyOutDir"); err != nil {
		return nil, s.markBuildFailed(ctx, req, publishType, startedAt, err)
	}
	artifactPath := appDevReleasePath
	if writer, ok := s.store.(BuildArtifactWriter); ok {
		artifact, err := zipDirectory(artifactFullPath)
		if err != nil {
			return nil, s.markBuildFailed(ctx, req, publishType, startedAt, err)
		}
		artifactPath, err = writer.SaveBuildArtifact(
			ctx,
			strings.TrimSpace(req.SpaceID),
			projectID,
			artifact,
		)
		if err != nil {
			return nil, s.markBuildFailed(ctx, req, publishType, startedAt, err)
		}
	}

	finishedAt := time.Now().UTC()
	message := "发布构建成功"
	if _, err := s.store.UpdateProjectBuild(ctx, strings.TrimSpace(req.SpaceID), projectID, "success", publishType, artifactPath, message, finishedAt.Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("persist appdev build success: %w", err)
	}

	return &BuildProjectResponse{
		Code:    0,
		Message: "success",
		Data: BuildProjectData{
			Status:       "success",
			PublishType:  publishType,
			ArtifactPath: artifactPath,
			DownloadURL:  fmt.Sprintf("/api/app-dev/spaces/%s/projects/%s/release", req.SpaceID, projectID),
			Message:      message,
			StartedAt:    startedAt.Format(time.RFC3339),
			FinishedAt:   finishedAt.Format(time.RFC3339),
			DurationMs:   finishedAt.Sub(startedAt).Milliseconds(),
		},
	}, nil
}

func (s *Service) buildProjectWithRunner(
	ctx context.Context,
	req *BuildProjectRequest,
	projectID string,
	publishType string,
) (*BuildProjectResponse, error) {
	runner, ok := s.runtime.(IsolatedBuildRunner)
	if !ok {
		return nil, fmt.Errorf("isolated appdev build runner is not configured")
	}
	sourceProvider, ok := s.store.(ProjectSourceURLProvider)
	if !ok {
		return nil, fmt.Errorf("appdev source object provider is not configured")
	}
	sourceURL, err := sourceProvider.ProjectSourceURL(ctx, strings.TrimSpace(req.SpaceID), projectID)
	if err != nil {
		return nil, err
	}
	startedAt := time.Now().UTC()
	if _, err := s.store.UpdateProjectBuild(
		ctx,
		strings.TrimSpace(req.SpaceID),
		projectID,
		"building",
		publishType,
		"",
		"发布构建中",
		startedAt.Format(time.RFC3339),
	); err != nil {
		return nil, fmt.Errorf("persist appdev build start: %w", err)
	}
	result, err := runner.Build(ctx, &IsolatedBuildRequest{
		SpaceID:     strings.TrimSpace(req.SpaceID),
		ProjectID:   projectID,
		PublishType: publishType,
		SourceURL:   sourceURL,
	})
	if err != nil {
		return nil, s.markBuildFailed(ctx, req, publishType, startedAt, err)
	}
	if result == nil || result.Status != "success" || strings.TrimSpace(result.ArtifactObjectKey) == "" {
		return nil, s.markBuildFailed(ctx, req, publishType, startedAt, fmt.Errorf("isolated appdev build did not return an artifact"))
	}
	finishedAt := result.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	if !result.StartedAt.IsZero() {
		startedAt = result.StartedAt
	}
	message := strings.TrimSpace(SanitizeAppDevOutput(result.Message))
	if message == "" {
		message = "发布构建成功"
	}
	_, err = s.store.UpdateProjectBuild(
		ctx,
		strings.TrimSpace(req.SpaceID),
		projectID,
		"success",
		publishType,
		result.ArtifactObjectKey,
		message,
		finishedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("persist appdev build success: %w", err)
	}
	return &BuildProjectResponse{
		Code:    0,
		Message: "success",
		Data: BuildProjectData{
			Status:       "success",
			PublishType:  publishType,
			ArtifactPath: result.ArtifactObjectKey,
			DownloadURL:  fmt.Sprintf("/api/app-dev/spaces/%s/projects/%s/release", req.SpaceID, projectID),
			Message:      message,
			StartedAt:    startedAt.Format(time.RFC3339),
			FinishedAt:   finishedAt.Format(time.RFC3339),
			DurationMs:   finishedAt.Sub(startedAt).Milliseconds(),
		},
	}, nil
}

func (s *Service) ExportBuildArtifact(ctx context.Context, req *ExportProjectRequest) (*ExportProjectData, error) {
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
	if artifactProvider, ok := s.store.(BuildArtifactProvider); ok && project.LastBuildArtifact != "" {
		content, err := artifactProvider.GetBuildArtifact(ctx, strings.TrimSpace(req.SpaceID), projectID)
		if err != nil {
			return nil, err
		}
		fileName := strings.TrimSuffix(safeBuildArchiveName(project.Name), ".zip") + "-release.zip"
		return &ExportProjectData{
			FileName:    fileName,
			ContentType: "application/zip",
			Content:     content,
		}, nil
	}
	projectDir, err := s.store.ProjectFilesDir(strings.TrimSpace(req.SpaceID), projectID)
	if err != nil {
		return nil, err
	}
	releaseDir := filepath.Join(projectDir, filepath.FromSlash(appDevReleasePath))
	info, err := os.Stat(releaseDir)
	if err != nil {
		return nil, fmt.Errorf("请先完成发布构建后再下载产物")
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("发布产物目录异常")
	}

	content, err := zipDirectory(releaseDir)
	if err != nil {
		return nil, err
	}
	fileName := strings.TrimSuffix(safeBuildArchiveName(project.Name), ".zip") + "-release.zip"
	return &ExportProjectData{
		FileName:    fileName,
		ContentType: "application/zip",
		Content:     content,
	}, nil
}

func (s *Service) markBuildFailed(ctx context.Context, req *BuildProjectRequest, publishType string, startedAt time.Time, err error) error {
	message := SanitizeAppDevOutput(err.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	if _, persistErr := s.store.UpdateProjectBuild(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), "error", publishType, "", message, time.Now().UTC().Format(time.RFC3339)); persistErr != nil {
		return errors.Join(err, fmt.Errorf("persist appdev build failure: %w", persistErr))
	}
	return err
}

func safeBuildArchiveName(projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "appdev-project"
	}

	var builder strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteRune('-')
	}
	if builder.Len() == 0 {
		return "appdev-project.zip"
	}

	return builder.String() + ".zip"
}

func ensureBuildDependencies(ctx context.Context, projectDir string) error {
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("项目缺少 package.json，无法发布构建")
		}
		return err
	}
	if err := runAppDevCommand(ctx, projectDir, "npm", "install", "--ignore-scripts", "--package-lock=false", "--no-audit", "--no-fund", "--prefer-offline", "--loglevel=warn"); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(projectDir, "node_modules", ".bin", "vite")); err == nil {
		return nil
	}
	if err := runAppDevCommand(ctx, projectDir, "npm", "install", "--ignore-scripts", "--package-lock=false", "--no-audit", "--no-fund", "--prefer-offline", "--loglevel=warn", "vite@^5.4.14", "react@^18.2.0", "react-dom@^18.2.0", "typescript@^5.8.2"); err != nil {
		return err
	}
	return nil
}

func writeBuildViteConfig(projectDir string) error {
	configDir := filepath.Join(projectDir, ".coze-appdev")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	config := `import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    host: '127.0.0.1'
  }
});
`
	return os.WriteFile(filepath.Join(configDir, "vite.build.config.mjs"), []byte(config), 0o644)
}

func runAppDevCommand(ctx context.Context, projectDir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(), "CI=1")
	output := &limitedOutput{limit: 24 * 1024}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(SanitizeAppDevOutput(output.String()))
		if detail == "" {
			detail = SanitizeAppDevOutput(err.Error())
		}
		return fmt.Errorf("appdev command failed: %s", detail)
	}
	return nil
}

type limitedOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (w *limitedOutput) Write(payload []byte) (int, error) {
	if w.buffer.Len() < w.limit {
		remaining := w.limit - w.buffer.Len()
		if len(payload) > remaining {
			_, _ = w.buffer.Write(payload[:remaining])
		} else {
			_, _ = w.buffer.Write(payload)
		}
	}
	return len(payload), nil
}

func (w *limitedOutput) String() string {
	return w.buffer.String()
}

func zipDirectory(root string) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	zipWriter := zip.NewWriter(buffer)
	fileCount := 0
	var totalBytes int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if _, err := domainappdev.NormalizeRelativePath(relativePath); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fileCount++
		if fileCount > 2000 {
			return fmt.Errorf("build artifact cannot exceed 2000 files")
		}
		if info.Size() > 10*1024*1024 {
			return fmt.Errorf("build artifact file %s cannot exceed 10MB", relativePath)
		}
		totalBytes += info.Size()
		if totalBytes > 100*1024*1024 {
			return fmt.Errorf("build artifact cannot exceed 100MB")
		}
		writer, err := zipWriter.Create(relativePath)
		if err != nil {
			return err
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = writer.Write(payload)
		return err
	})
	if err != nil {
		_ = zipWriter.Close()
		return nil, err
	}
	if err := zipWriter.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
