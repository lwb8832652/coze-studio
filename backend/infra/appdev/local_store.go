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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

const (
	projectMetaFileName     = "project.json"
	appDevMaxArchiveEntries = 2000
	appDevMaxArchiveDepth   = 20
	appDevMaxArchiveFile    = 10 * 1024 * 1024
	appDevMaxArchiveBytes   = 100 * 1024 * 1024
)

type LocalStore struct {
	root string
	mu   sync.RWMutex
}

func NewLocalStoreFromEnv() *LocalStore {
	root := strings.TrimSpace(os.Getenv("APP_DEV_WORKSPACE_ROOT"))
	if root == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			root = filepath.Join(cacheDir, "coze-studio", "appdev")
		} else {
			root = filepath.Join(os.TempDir(), "coze-studio-appdev")
		}
	}

	return &LocalStore{root: root}
}

func NewLocalStoreForTest(root string) *LocalStore {
	return &LocalStore{root: root}
}

func (s *LocalStore) ListProjects(ctx context.Context, spaceID string, keyword string) ([]*domainappdev.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	spaceDir := s.spaceDir(spaceID)
	entries, err := os.ReadDir(spaceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*domainappdev.Project{}, nil
		}
		return nil, err
	}

	normalizedKeyword := strings.ToLower(strings.TrimSpace(keyword))
	projects := make([]*domainappdev.Project, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		project, err := s.readProject(spaceID, entry.Name())
		if err != nil {
			continue
		}
		if project.Status == domainappdev.ProjectStatusArchived {
			continue
		}
		if normalizedKeyword != "" &&
			!strings.Contains(strings.ToLower(project.Name), normalizedKeyword) &&
			!strings.Contains(strings.ToLower(project.Description), normalizedKeyword) &&
			!strings.Contains(strings.ToLower(project.Prompt), normalizedKeyword) {
			continue
		}
		projects = append(projects, project)
	}

	return projects, nil
}

func (s *LocalStore) CreateProject(ctx context.Context, project *domainappdev.Project, initialFiles map[string]string) (*domainappdev.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	projectDir := s.projectDir(project.SpaceID, project.ID)
	filesDir := s.filesDir(project.SpaceID, project.ID)
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return nil, err
	}

	for filePath, content := range initialFiles {
		if err := s.writeFile(project.SpaceID, project.ID, filePath, content); err != nil {
			return nil, err
		}
	}

	if project.CreatedAt.IsZero() {
		project.CreatedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	if project.SourceUpdatedAt.IsZero() {
		project.SourceUpdatedAt = now
	}
	project.UpdatedAt = now

	payload, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(projectDir, projectMetaFileName), payload, 0o644); err != nil {
		return nil, err
	}

	return project, nil
}

func (s *LocalStore) ImportProjectArchive(ctx context.Context, project *domainappdev.Project, archive []byte) (*domainappdev.Project, error) {
	return s.importProjectArchive(ctx, project, archive, true)
}

func (s *LocalStore) restoreProjectArchive(ctx context.Context, project *domainappdev.Project, archive []byte) (*domainappdev.Project, error) {
	return s.importProjectArchive(ctx, project, archive, false)
}

func (s *LocalStore) importProjectArchive(ctx context.Context, project *domainappdev.Project, archive []byte, stripRoot bool) (*domainappdev.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	if len(reader.File) == 0 {
		return nil, fmt.Errorf("project archive cannot be empty")
	}
	if len(reader.File) > appDevMaxArchiveEntries {
		return nil, fmt.Errorf("project archive cannot exceed %d entries", appDevMaxArchiveEntries)
	}

	projectDir := s.projectDir(project.SpaceID, project.ID)
	if _, err := os.Stat(projectDir); err == nil {
		return nil, fmt.Errorf("project already exists")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	filesDir := s.filesDir(project.SpaceID, project.ID)
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return nil, err
	}
	imported := false
	defer func() {
		if !imported {
			_ = os.RemoveAll(projectDir)
		}
	}()

	rootPrefix := ""
	if stripRoot {
		rootPrefix = archiveRootPrefix(reader.File)
	}
	var totalBytes int64
	written := 0
	for _, zippedFile := range reader.File {
		if zippedFile.FileInfo().IsDir() {
			continue
		}

		normalized, err := domainappdev.NormalizeRelativePath(strings.ReplaceAll(zippedFile.Name, "\\", "/"))
		if err != nil {
			return nil, fmt.Errorf("invalid archive path %q: %w", zippedFile.Name, err)
		}
		if strings.HasPrefix(normalized, "__MACOSX/") || strings.HasSuffix(normalized, ".DS_Store") {
			continue
		}
		if strings.Count(normalized, "/")+1 > appDevMaxArchiveDepth {
			return nil, fmt.Errorf("archive path depth cannot exceed %d: %s", appDevMaxArchiveDepth, normalized)
		}
		normalized = strings.TrimPrefix(normalized, rootPrefix)
		if normalized == "" {
			continue
		}
		if shouldSkipProjectFilePath(normalized) {
			continue
		}
		if zippedFile.UncompressedSize64 > appDevMaxArchiveFile {
			return nil, fmt.Errorf("archive file %s cannot exceed 10MB", normalized)
		}

		fileReader, err := zippedFile.Open()
		if err != nil {
			return nil, err
		}
		payload, err := io.ReadAll(io.LimitReader(fileReader, appDevMaxArchiveFile+1))
		closeErr := fileReader.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(payload) > appDevMaxArchiveFile {
			return nil, fmt.Errorf("archive file %s cannot exceed 10MB", normalized)
		}
		totalBytes += int64(len(payload))
		if totalBytes > appDevMaxArchiveBytes {
			return nil, fmt.Errorf("project archive cannot exceed 100MB")
		}
		if err := s.writeBytesFile(project.SpaceID, project.ID, normalized, payload); err != nil {
			return nil, err
		}
		written++
	}

	if written == 0 {
		return nil, fmt.Errorf("project archive does not contain files")
	}
	if err := s.ensureRunnableProjectFiles(project.SpaceID, project.ID); err != nil {
		return nil, err
	}
	if project.CreatedAt.IsZero() {
		project.CreatedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	if project.SourceUpdatedAt.IsZero() {
		project.SourceUpdatedAt = now
	}
	project.UpdatedAt = now
	if err := s.writeProject(project); err != nil {
		return nil, err
	}

	imported = true
	return project, nil
}

func (s *LocalStore) ensureRunnableProjectFiles(spaceID string, projectID string) error {
	defaults := map[string]string{
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
		"index.html": `<div id="root"></div><script type="module" src="/src/main.tsx"></script>
`,
		"src/main.tsx": `import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
`,
		"src/App.tsx": `export default function App() {
  return (
    <main style={{ minHeight: '100vh', padding: 48, fontFamily: 'Avenir Next, PingFang SC, sans-serif' }}>
      <h1>导入的网页应用</h1>
      <p>项目已导入，可以继续通过 AI 生成和完善页面。</p>
    </main>
  );
}
`,
	}

	for filePath, content := range defaults {
		fullPath := filepath.Join(s.filesDir(spaceID, projectID), filepath.FromSlash(filePath))
		if _, err := os.Stat(fullPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := s.writeBytesFile(spaceID, projectID, filePath, []byte(content)); err != nil {
			return err
		}
	}

	return nil
}

func (s *LocalStore) GetProject(ctx context.Context, spaceID string, projectID string) (*domainappdev.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.readProject(spaceID, projectID)
}

func (s *LocalStore) UpdateProject(ctx context.Context, spaceID string, projectID string, name string, description string) (*domainappdev.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, err := s.readProject(spaceID, projectID)
	if err != nil {
		return nil, err
	}

	project.Name = strings.TrimSpace(name)
	project.Description = strings.TrimSpace(description)
	project.UpdatedAt = time.Now().UTC()
	if err := s.writeProject(project); err != nil {
		return nil, err
	}

	return project, nil
}

func (s *LocalStore) ArchiveProject(ctx context.Context, spaceID string, projectID string) (*domainappdev.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, err := s.readProject(spaceID, projectID)
	if err != nil {
		return nil, err
	}
	project.Status = domainappdev.ProjectStatusArchived
	project.UpdatedAt = time.Now().UTC()
	if err := s.writeProject(project); err != nil {
		return nil, err
	}

	return project, nil
}

func (s *LocalStore) ExportProjectArchive(ctx context.Context, spaceID string, projectID string) ([]byte, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	project, err := s.readProject(spaceID, projectID)
	if err != nil {
		return nil, "", err
	}

	root := s.filesDir(spaceID, projectID)
	buffer := bytes.NewBuffer(nil)
	zipWriter := zip.NewWriter(buffer)
	fileCount := 0
	var totalBytes int64
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if shouldSkipProjectFilePath(relativePath) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := domainappdev.NormalizeRelativePath(relativePath); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fileCount++
		if fileCount > appDevMaxArchiveEntries {
			return fmt.Errorf("project archive cannot exceed %d entries", appDevMaxArchiveEntries)
		}
		if info.Size() > appDevMaxArchiveFile {
			return fmt.Errorf("project file %s cannot exceed 10MB", relativePath)
		}
		totalBytes += info.Size()
		if totalBytes > appDevMaxArchiveBytes {
			return fmt.Errorf("project archive cannot exceed 100MB")
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
		return nil, "", err
	}
	if err := zipWriter.Close(); err != nil {
		return nil, "", err
	}

	return buffer.Bytes(), safeArchiveName(project.Name), nil
}

func (s *LocalStore) ListFiles(ctx context.Context, spaceID string, projectID string) ([]*domainappdev.FileNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	root := s.filesDir(spaceID, projectID)
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return []*domainappdev.FileNode{}, nil
		}
		return nil, err
	}

	return s.readChildren(root, "")
}

func (s *LocalStore) GetFileContent(ctx context.Context, spaceID string, projectID string, filePath string) (*domainappdev.FileContent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized, err := domainappdev.NormalizeRelativePath(filePath)
	if err != nil {
		return nil, err
	}
	if err := rejectManagedProjectPath(normalized); err != nil {
		return nil, err
	}

	fullPath := filepath.Join(s.filesDir(spaceID, projectID), filepath.FromSlash(normalized))
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, domainappdev.ErrInvalidPath
	}
	if info.Size() > 2*1024*1024 {
		return nil, fmt.Errorf("file content cannot exceed 2MB")
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("binary file cannot be opened in editor")
	}

	return &domainappdev.FileContent{
		Path:     normalized,
		Content:  string(content),
		Version:  appDevFileVersion(content),
		Language: languageFromPath(normalized),
	}, nil
}

func (s *LocalStore) SaveFileContent(ctx context.Context, spaceID string, projectID string, filePath string, content string) (*domainappdev.FileContent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writeFile(spaceID, projectID, filePath, content); err != nil {
		return nil, err
	}

	normalized, _ := domainappdev.NormalizeRelativePath(filePath)
	if project, err := s.readProject(spaceID, projectID); err == nil {
		now := time.Now().UTC()
		project.SourceUpdatedAt = now
		project.UpdatedAt = now
		_ = s.writeProject(project)
	}

	return &domainappdev.FileContent{
		Path:     normalized,
		Content:  content,
		Version:  appDevFileVersion([]byte(content)),
		Language: languageFromPath(normalized),
	}, nil
}

func appDevFileVersion(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func (s *LocalStore) SaveUploadedFiles(ctx context.Context, spaceID string, projectID string, files []domainappdev.UploadedFile) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	written := 0
	for _, file := range files {
		normalized, err := domainappdev.NormalizeRelativePath(file.Path)
		if err != nil {
			return written, err
		}
		if err := rejectManagedProjectPath(normalized); err != nil {
			return written, err
		}
		fullPath := filepath.Join(s.filesDir(spaceID, projectID), filepath.FromSlash(normalized))
		if _, err := os.Stat(fullPath); err == nil {
			return written, fmt.Errorf("target path already exists: %s", normalized)
		} else if !os.IsNotExist(err) {
			return written, err
		}
		if err := s.writeBytesFile(spaceID, projectID, file.Path, file.Content); err != nil {
			return written, err
		}
		written++
	}
	if written > 0 {
		s.touchProject(spaceID, projectID)
	}
	return written, nil
}

func (s *LocalStore) DeletePath(ctx context.Context, spaceID string, projectID string, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := domainappdev.NormalizeRelativePath(filePath)
	if err != nil {
		return err
	}
	if err := rejectManagedProjectPath(normalized); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.filesDir(spaceID, projectID), filepath.FromSlash(normalized))); err != nil {
		return err
	}
	s.touchProject(spaceID, projectID)
	return nil
}

func (s *LocalStore) RenamePath(ctx context.Context, spaceID string, projectID string, sourcePath string, targetPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	source, err := domainappdev.NormalizeRelativePath(sourcePath)
	if err != nil {
		return err
	}
	if err := rejectManagedProjectPath(source); err != nil {
		return err
	}
	target, err := domainappdev.NormalizeRelativePath(targetPath)
	if err != nil {
		return err
	}
	if err := rejectManagedProjectPath(target); err != nil {
		return err
	}

	sourceFullPath := filepath.Join(s.filesDir(spaceID, projectID), filepath.FromSlash(source))
	targetFullPath := filepath.Join(s.filesDir(spaceID, projectID), filepath.FromSlash(target))
	if _, err := os.Stat(targetFullPath); err == nil {
		return fmt.Errorf("target path already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetFullPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(sourceFullPath, targetFullPath); err != nil {
		return err
	}
	s.touchProject(spaceID, projectID)
	return nil
}

func (s *LocalStore) CreateProjectSnapshot(ctx context.Context, spaceID string, projectID string, label string) (*domainappdev.ProjectSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.readProject(spaceID, projectID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	snapshot := &domainappdev.ProjectSnapshot{
		ID:        fmt.Sprintf("snapshot_%d", now.UnixNano()),
		Label:     strings.TrimSpace(label),
		CreatedAt: now,
	}
	if snapshot.Label == "" {
		snapshot.Label = "手动快照"
	}
	if len([]rune(snapshot.Label)) > 80 {
		snapshot.Label = string([]rune(snapshot.Label)[:80])
	}

	snapshotDir := s.snapshotDir(spaceID, projectID, snapshot.ID)
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return nil, err
	}
	if err := copyProjectFiles(s.filesDir(spaceID, projectID), filepath.Join(snapshotDir, "files")); err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(snapshotDir, "snapshot.json"), payload, 0o644); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func (s *LocalStore) ListProjectSnapshots(ctx context.Context, spaceID string, projectID string) ([]*domainappdev.ProjectSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	root := s.snapshotsDir(spaceID, projectID)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []*domainappdev.ProjectSnapshot{}, nil
		}
		return nil, err
	}

	snapshots := make([]*domainappdev.ProjectSnapshot, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(root, entry.Name(), "snapshot.json"))
		if err != nil {
			continue
		}
		var snapshot domainappdev.ProjectSnapshot
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			continue
		}
		snapshots = append(snapshots, &snapshot)
	}
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	return snapshots, nil
}

func (s *LocalStore) RestoreProjectSnapshot(ctx context.Context, spaceID string, projectID string, snapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshotID = safeSegment(snapshotID)
	if snapshotID == "_" {
		return fmt.Errorf("snapshot_id cannot be empty")
	}
	sourceDir := filepath.Join(s.snapshotDir(spaceID, projectID, snapshotID), "files")
	if info, err := os.Stat(sourceDir); err != nil {
		if os.IsNotExist(err) {
			return domainappdev.ErrNotFound
		}
		return err
	} else if !info.IsDir() {
		return domainappdev.ErrNotFound
	}

	filesDir := s.filesDir(spaceID, projectID)
	entries, err := os.ReadDir(filesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if shouldSkipProjectFilePath(entry.Name()) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(filesDir, entry.Name())); err != nil {
			return err
		}
	}
	if err := copyProjectFiles(sourceDir, filesDir); err != nil {
		return err
	}
	s.touchProject(spaceID, projectID)
	return nil
}

func (s *LocalStore) ProjectFilesDir(spaceID string, projectID string) (string, error) {
	dir := s.filesDir(spaceID, projectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func (s *LocalStore) UpdateProjectRuntime(ctx context.Context, spaceID string, projectID string, status domainappdev.RuntimeStatus, previewURL string) (*domainappdev.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, err := s.readProject(spaceID, projectID)
	if err != nil {
		return nil, err
	}

	changed := project.RuntimeStatus != status || project.PreviewURL != previewURL
	project.RuntimeStatus = status
	project.PreviewURL = previewURL
	if changed {
		project.UpdatedAt = time.Now().UTC()
	}
	if err := s.writeProject(project); err != nil {
		return nil, err
	}

	return project, nil
}

func (s *LocalStore) UpdateProjectBuild(ctx context.Context, spaceID string, projectID string, status string, publishType string, artifactPath string, message string, buildAt string) (*domainappdev.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	project, err := s.readProject(spaceID, projectID)
	if err != nil {
		return nil, err
	}

	project.LastBuildStatus = strings.TrimSpace(status)
	project.LastBuildType = strings.TrimSpace(publishType)
	project.LastBuildArtifact = strings.TrimSpace(artifactPath)
	project.LastBuildMessage = strings.TrimSpace(message)
	if buildAt != "" {
		if parsed, err := time.Parse(time.RFC3339, buildAt); err == nil {
			project.LastBuildAt = parsed
		}
	}
	project.UpdatedAt = time.Now().UTC()
	if err := s.writeProject(project); err != nil {
		return nil, err
	}

	return project, nil
}

func (s *LocalStore) readProject(spaceID string, projectID string) (*domainappdev.Project, error) {
	payload, err := os.ReadFile(filepath.Join(s.projectDir(spaceID, projectID), projectMetaFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domainappdev.ErrNotFound
		}
		return nil, err
	}

	var project domainappdev.Project
	if err := json.Unmarshal(payload, &project); err != nil {
		return nil, err
	}

	if project.SpaceID != spaceID {
		return nil, domainappdev.ErrNotFound
	}
	if project.SourceUpdatedAt.IsZero() {
		project.SourceUpdatedAt = s.latestSourceUpdatedAt(spaceID, projectID)
	}

	return &project, nil
}

func (s *LocalStore) writeProject(project *domainappdev.Project) error {
	payload, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(s.projectDir(project.SpaceID, project.ID), projectMetaFileName), payload, 0o644)
}

func (s *LocalStore) touchProject(spaceID string, projectID string) {
	project, err := s.readProject(spaceID, projectID)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	project.SourceUpdatedAt = now
	project.UpdatedAt = now
	_ = s.writeProject(project)
}

func (s *LocalStore) latestSourceUpdatedAt(spaceID string, projectID string) time.Time {
	root := s.filesDir(spaceID, projectID)
	var latest time.Time
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil || relativePath == "." {
			return nil
		}
		normalized := filepath.ToSlash(relativePath)
		if shouldSkipProjectFilePath(normalized) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().After(latest) {
			latest = info.ModTime().UTC()
		}
		return nil
	})
	return latest
}

func (s *LocalStore) writeFile(spaceID string, projectID string, filePath string, content string) error {
	return s.writeBytesFile(spaceID, projectID, filePath, []byte(content))
}

func (s *LocalStore) writeBytesFile(spaceID string, projectID string, filePath string, content []byte) error {
	normalized, err := domainappdev.NormalizeRelativePath(filePath)
	if err != nil {
		return err
	}
	if err := rejectManagedProjectPath(normalized); err != nil {
		return err
	}

	fullPath := filepath.Join(s.filesDir(spaceID, projectID), filepath.FromSlash(normalized))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}

	return os.WriteFile(fullPath, content, 0o644)
}

func (s *LocalStore) readChildren(root string, relativeDir string) ([]*domainappdev.FileNode, error) {
	dirPath := filepath.Join(root, filepath.FromSlash(relativeDir))
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	nodes := make([]*domainappdev.FileNode, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		relativePath := entry.Name()
		if relativeDir != "" {
			relativePath = relativeDir + "/" + entry.Name()
		}
		if shouldSkipProjectFilePath(relativePath) {
			continue
		}

		node := &domainappdev.FileNode{
			ID:        relativePath,
			Path:      relativePath,
			Name:      entry.Name(),
			Type:      "file",
			Size:      info.Size(),
			UpdatedAt: info.ModTime(),
		}
		if entry.IsDir() {
			node.Type = "directory"
			node.Size = 0
			children, err := s.readChildren(root, relativePath)
			if err != nil {
				return nil, err
			}
			node.Children = children
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (s *LocalStore) spaceDir(spaceID string) string {
	return filepath.Join(s.root, "spaces", safeSegment(spaceID))
}

func (s *LocalStore) projectDir(spaceID string, projectID string) string {
	return filepath.Join(s.spaceDir(spaceID), safeSegment(projectID))
}

func (s *LocalStore) filesDir(spaceID string, projectID string) string {
	return filepath.Join(s.projectDir(spaceID, projectID), "files")
}

func (s *LocalStore) snapshotsDir(spaceID string, projectID string) string {
	return filepath.Join(s.filesDir(spaceID, projectID), ".coze-appdev", "snapshots")
}

func (s *LocalStore) snapshotDir(spaceID string, projectID string, snapshotID string) string {
	return filepath.Join(s.snapshotsDir(spaceID, projectID), safeSegment(snapshotID))
}

func safeSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "_"
	}

	var builder strings.Builder
	for _, r := range trimmed {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "_"
	}

	return builder.String()
}

func languageFromPath(filePath string) string {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".css", ".less":
		return "css"
	case ".html":
		return "html"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	default:
		return "plaintext"
	}
}

func safeArchiveName(projectName string) string {
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

func shouldSkipProjectFilePath(filePath string) bool {
	normalized := strings.Trim(strings.ReplaceAll(filePath, "\\", "/"), "/")
	if normalized == "" {
		return false
	}

	blockedRoots := []string{
		".coze-appdev",
		".git",
		"node_modules",
	}
	for _, root := range blockedRoots {
		if normalized == root || strings.HasPrefix(normalized, root+"/") {
			return true
		}
	}

	return false
}

func rejectManagedProjectPath(filePath string) error {
	if shouldSkipProjectFilePath(filePath) {
		return fmt.Errorf("path is managed by AppDev and cannot be edited")
	}

	return nil
}

func archiveRootPrefix(files []*zip.File) string {
	root := ""
	fileCount := 0
	for _, zippedFile := range files {
		if zippedFile.FileInfo().IsDir() {
			continue
		}

		normalized, err := domainappdev.NormalizeRelativePath(strings.ReplaceAll(zippedFile.Name, "\\", "/"))
		if err != nil {
			return ""
		}
		if strings.HasPrefix(normalized, "__MACOSX/") || strings.HasSuffix(normalized, ".DS_Store") {
			continue
		}

		parts := strings.Split(normalized, "/")
		if len(parts) < 2 {
			return ""
		}
		if root == "" {
			root = parts[0]
		} else if root != parts[0] {
			return ""
		}
		fileCount++
	}

	if fileCount == 0 || root == "" {
		return ""
	}

	return root + "/"
}

func copyProjectFiles(sourceRoot string, targetRoot string) error {
	return filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceRoot {
			return os.MkdirAll(targetRoot, 0o755)
		}

		relativePath, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if shouldSkipProjectFilePath(relativePath) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		targetPath := filepath.Join(targetRoot, filepath.FromSlash(relativePath))
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, payload, 0o644)
	})
}
