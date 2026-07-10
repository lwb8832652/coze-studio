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
	"context"
	"fmt"
	"strings"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

const (
	maxEditableFileBytes = 2 * 1024 * 1024
	maxUploadFiles       = 100
	maxUploadFileBytes   = 10 * 1024 * 1024
	maxUploadTotalBytes  = 100 * 1024 * 1024
)

type ProjectFileRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
}

type FileContentRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	Path          string
}

type SaveFileContentRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	Path          string
	Content       string
	Version       string
}

type UploadFileItem struct {
	Path    string
	Content []byte
}

type UploadFilesRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	Files         []UploadFileItem
}

type DeletePathRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	Path          string
}

type RenamePathRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	SourcePath    string
	TargetPath    string
}

type FileNodeDTO struct {
	ID        string         `json:"id"`
	Path      string         `json:"path"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Size      int64          `json:"size,omitempty"`
	Children  []*FileNodeDTO `json:"children,omitempty"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
}

type FileListData struct {
	Items []*FileNodeDTO `json:"items"`
}

type FileListResponse struct {
	Code    int64        `json:"code"`
	Message string       `json:"message"`
	Data    FileListData `json:"data"`
}

type FileContentDTO struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Version  string `json:"version"`
	Language string `json:"language"`
}

type FileContentResponse struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    *FileContentDTO `json:"data"`
}

type FileMutationData struct {
	Success bool `json:"success"`
}

type FileUploadData struct {
	Uploaded int `json:"uploaded"`
}

type FileMutationResponse struct {
	Code    int64            `json:"code"`
	Message string           `json:"message"`
	Data    FileMutationData `json:"data"`
}

type FileUploadResponse struct {
	Code    int64          `json:"code"`
	Message string         `json:"message"`
	Data    FileUploadData `json:"data"`
}

func (s *Service) ListFiles(ctx context.Context, req *ProjectFileRequest) (*FileListResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}

	nodes, err := s.store.ListFiles(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID))
	if err != nil {
		return nil, err
	}

	items := make([]*FileNodeDTO, 0, len(nodes))
	for _, node := range nodes {
		items = append(items, toFileNodeDTO(node))
	}

	return &FileListResponse{
		Code:    0,
		Message: "success",
		Data: FileListData{
			Items: items,
		},
	}, nil
}

func (s *Service) GetFileContent(ctx context.Context, req *FileContentRequest) (*FileContentResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}

	filePath, err := domainappdev.NormalizeRelativePath(req.Path)
	if err != nil {
		return nil, err
	}

	content, err := s.store.GetFileContent(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), filePath)
	if err != nil {
		return nil, err
	}

	return successFileContentResponse(content), nil
}

func (s *Service) SaveFileContent(ctx context.Context, req *SaveFileContentRequest) (*FileContentResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}

	filePath, err := domainappdev.NormalizeRelativePath(req.Path)
	if err != nil {
		return nil, err
	}
	if len([]byte(req.Content)) > maxEditableFileBytes {
		return nil, fmt.Errorf("file content cannot exceed 2MB")
	}
	if version := strings.TrimSpace(req.Version); version != "" {
		current, err := s.store.GetFileContent(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), filePath)
		if err != nil {
			return nil, err
		}
		if current.Version != version {
			return nil, fmt.Errorf("cannot save stale file version, please reload before saving")
		}
	}

	content, err := s.store.SaveFileContent(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), filePath, req.Content)
	if err != nil {
		return nil, err
	}

	return successFileContentResponse(content), nil
}

func (s *Service) UploadFiles(ctx context.Context, req *UploadFilesRequest) (*FileUploadResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}
	files, err := validateUploadFiles(req.Files)
	if err != nil {
		return nil, err
	}

	uploaded, err := s.store.SaveUploadedFiles(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), files)
	if err != nil {
		return nil, err
	}

	return &FileUploadResponse{
		Code:    0,
		Message: "success",
		Data: FileUploadData{
			Uploaded: uploaded,
		},
	}, nil
}

func validateUploadFiles(items []UploadFileItem) ([]domainappdev.UploadedFile, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("upload files cannot be empty")
	}
	if len(items) > maxUploadFiles {
		return nil, fmt.Errorf("upload files cannot exceed %d", maxUploadFiles)
	}

	files := make([]domainappdev.UploadedFile, 0, len(items))
	var totalBytes int64
	seen := map[string]struct{}{}
	for _, item := range items {
		filePath, err := domainappdev.NormalizeRelativePath(item.Path)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[filePath]; ok {
			return nil, fmt.Errorf("duplicated upload path: %s", filePath)
		}
		seen[filePath] = struct{}{}
		if len(item.Content) > maxUploadFileBytes {
			return nil, fmt.Errorf("upload file %s cannot exceed 10MB", filePath)
		}
		totalBytes += int64(len(item.Content))
		if totalBytes > maxUploadTotalBytes {
			return nil, fmt.Errorf("upload files cannot exceed 100MB")
		}
		files = append(files, domainappdev.UploadedFile{
			Path:    filePath,
			Content: item.Content,
		})
	}
	return files, nil
}

func (s *Service) DeletePath(ctx context.Context, req *DeletePathRequest) (*FileMutationResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}

	filePath, err := domainappdev.NormalizeRelativePath(req.Path)
	if err != nil {
		return nil, err
	}

	if err := s.store.DeletePath(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), filePath); err != nil {
		return nil, err
	}

	return successFileMutationResponse(), nil
}

func (s *Service) RenamePath(ctx context.Context, req *RenamePathRequest) (*FileMutationResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}

	sourcePath, err := domainappdev.NormalizeRelativePath(req.SourcePath)
	if err != nil {
		return nil, err
	}
	targetPath, err := domainappdev.NormalizeRelativePath(req.TargetPath)
	if err != nil {
		return nil, err
	}

	if err := s.store.RenamePath(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), sourcePath, targetPath); err != nil {
		return nil, err
	}

	return successFileMutationResponse(), nil
}

func validateProjectFileRequest(spaceID string, currentUserID int64, projectID string) error {
	if err := validateUserAndSpace(currentUserID, spaceID); err != nil {
		return err
	}
	if strings.TrimSpace(projectID) == "" {
		return fmt.Errorf("project_id cannot be empty")
	}
	return nil
}

func successFileContentResponse(content *domainappdev.FileContent) *FileContentResponse {
	return &FileContentResponse{
		Code:    0,
		Message: "success",
		Data:    toFileContentDTO(content),
	}
}

func successFileMutationResponse() *FileMutationResponse {
	return &FileMutationResponse{
		Code:    0,
		Message: "success",
		Data: FileMutationData{
			Success: true,
		},
	}
}

func toFileNodeDTO(node *domainappdev.FileNode) *FileNodeDTO {
	if node == nil {
		return nil
	}

	children := make([]*FileNodeDTO, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, toFileNodeDTO(child))
	}

	return &FileNodeDTO{
		ID:        node.ID,
		Path:      node.Path,
		Name:      node.Name,
		Type:      node.Type,
		Size:      node.Size,
		Children:  children,
		UpdatedAt: formatTime(node.UpdatedAt),
	}
}

func toFileContentDTO(content *domainappdev.FileContent) *FileContentDTO {
	if content == nil {
		return nil
	}

	return &FileContentDTO{
		Path:     content.Path,
		Content:  content.Content,
		Version:  content.Version,
		Language: content.Language,
	}
}
