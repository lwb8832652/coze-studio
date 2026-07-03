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

package agentthread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"path"
	"strings"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

const (
	maxTaskThreadUploadFiles      = 10
	maxTaskThreadUploadFileBytes  = 50 << 20
	maxTaskThreadUploadTotalBytes = 100 << 20
)

type TaskThreadUploadFileInput struct {
	FileName    string
	Content     []byte
	ContentType string
}

type UploadTaskThreadFilesRequest struct {
	SpaceID  int64
	UserID   int64
	ThreadID int64
	Files    []TaskThreadUploadFileInput
}

type TaskThreadUploadedFileSummary struct {
	FileID      int64  `json:"file_id,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	VirtualPath string `json:"virtual_path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
}

type UploadTaskThreadFilesResponse struct {
	Files        []*TaskThreadUploadedFileSummary
	SkippedFiles []string
}

type ListTaskThreadUploadFilesRequest struct {
	SpaceID  int64
	UserID   int64
	ThreadID int64
}

type ListTaskThreadUploadFilesResponse struct {
	Files []*TaskThreadUploadedFileSummary
}

type DeleteTaskThreadUploadFileRequest struct {
	SpaceID  int64
	UserID   int64
	ThreadID int64
	FileName string
}

type DeleteTaskThreadUploadFileResponse struct {
	File    *TaskThreadUploadedFileSummary
	Deleted bool
}

func (s *ApplicationService) UploadTaskThreadFiles(
	ctx context.Context,
	req *UploadTaskThreadFilesRequest,
) (*UploadTaskThreadFilesResponse, error) {
	if s == nil || s.UploadFileSVC == nil {
		return nil, fmt.Errorf("agent upload file service is not initialized")
	}
	writer, ok := s.ArtifactObjectStorage.(ArtifactObjectWriter)
	if s.ArtifactObjectStorage == nil || !ok {
		return nil, fmt.Errorf("artifact object storage writer is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("upload task thread files request is required")
	}
	if len(req.Files) == 0 {
		return nil, fmt.Errorf("upload files are required")
	}
	if len(req.Files) > maxTaskThreadUploadFiles {
		return nil, fmt.Errorf("upload file count exceeds limit")
	}

	existing, err := s.UploadFileSVC.ListUploadFiles(
		ctx,
		&domainservice.ListUploadFilesRequest{
			SpaceID:  req.SpaceID,
			UserID:   req.UserID,
			ThreadID: req.ThreadID,
		},
	)
	if err != nil {
		return nil, err
	}
	usedNames := make(map[string]bool, len(existing)+len(req.Files))
	for _, file := range existing {
		if file != nil {
			usedNames[file.FileName] = true
		}
	}

	resp := &UploadTaskThreadFilesResponse{
		Files:        make([]*TaskThreadUploadedFileSummary, 0, len(req.Files)),
		SkippedFiles: make([]string, 0),
	}
	totalBytes := int64(0)
	for _, input := range req.Files {
		fileName, err := domainservice.NormalizeUploadFileName(input.FileName)
		if err != nil {
			resp.SkippedFiles = append(resp.SkippedFiles, strings.TrimSpace(input.FileName))
			continue
		}
		size := int64(len(input.Content))
		if size <= 0 || size > maxTaskThreadUploadFileBytes {
			return nil, fmt.Errorf("upload file size is invalid")
		}
		totalBytes += size
		if totalBytes > maxTaskThreadUploadTotalBytes {
			return nil, fmt.Errorf("upload total size exceeds limit")
		}
		fileName = uniqueUploadFileName(fileName, usedNames)
		usedNames[fileName] = true

		contentType := strings.TrimSpace(input.ContentType)
		if contentType == "" {
			contentType = http.DetectContentType(input.Content)
		}
		objectKey := domainservice.UploadObjectURI(req.SpaceID, req.ThreadID, fileName)
		if err := writer.PutObject(
			ctx,
			objectKey,
			input.Content,
			storage.WithContentType(contentType),
			storage.WithObjectSize(size),
		); err != nil {
			return nil, fmt.Errorf("upload file storage failed")
		}

		digestBytes := sha256.Sum256(input.Content)
		registered, err := s.UploadFileSVC.RegisterUploadFile(
			ctx,
			&domainservice.RegisterUploadFileRequest{
				SpaceID:          req.SpaceID,
				UserID:           req.UserID,
				ThreadID:         req.ThreadID,
				FileName:         fileName,
				OriginalFileName: fileName,
				ContentType:      contentType,
				SizeBytes:        size,
				Digest:           hex.EncodeToString(digestBytes[:]),
				Metadata:         `{"source":"workbench_composer"}`,
			},
		)
		if err != nil {
			return nil, err
		}
		resp.Files = append(resp.Files, taskThreadUploadedFileSummary(registered))
	}
	return resp, nil
}

func (s *ApplicationService) ListTaskThreadUploadFiles(
	ctx context.Context,
	req *ListTaskThreadUploadFilesRequest,
) (*ListTaskThreadUploadFilesResponse, error) {
	if s == nil || s.UploadFileSVC == nil {
		return nil, fmt.Errorf("agent upload file service is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("list task thread upload files request is required")
	}
	files, err := s.UploadFileSVC.ListUploadFiles(
		ctx,
		&domainservice.ListUploadFilesRequest{
			SpaceID:  req.SpaceID,
			UserID:   req.UserID,
			ThreadID: req.ThreadID,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ListTaskThreadUploadFilesResponse{
		Files: make([]*TaskThreadUploadedFileSummary, 0, len(files)),
	}
	for _, file := range files {
		resp.Files = append(resp.Files, taskThreadUploadedFileSummary(file))
	}
	return resp, nil
}

func (s *ApplicationService) DeleteTaskThreadUploadFile(
	ctx context.Context,
	req *DeleteTaskThreadUploadFileRequest,
) (*DeleteTaskThreadUploadFileResponse, error) {
	if s == nil || s.UploadFileSVC == nil {
		return nil, fmt.Errorf("agent upload file service is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("delete task thread upload file request is required")
	}
	file, deleted, err := s.UploadFileSVC.DeleteUploadFile(
		ctx,
		&domainservice.DeleteUploadFileRequest{
			SpaceID:  req.SpaceID,
			UserID:   req.UserID,
			ThreadID: req.ThreadID,
			FileName: req.FileName,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &DeleteTaskThreadUploadFileResponse{Deleted: deleted}
	if file != nil {
		resp.File = taskThreadUploadedFileSummary(file)
	}
	return resp, nil
}

func uniqueUploadFileName(fileName string, used map[string]bool) string {
	if !used[fileName] {
		return fileName
	}
	ext := path.Ext(fileName)
	base := strings.TrimSuffix(fileName, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if !used[candidate] {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d%s", base, len(used)+1, ext)
}

func taskThreadUploadedFileSummary(
	file *domainentity.AgentFile,
) *TaskThreadUploadedFileSummary {
	if file == nil {
		return nil
	}
	return &TaskThreadUploadedFileSummary{
		FileID:      file.ID,
		FileName:    file.FileName,
		VirtualPath: file.VirtualPath,
		ContentType: file.ContentType,
		SizeBytes:   file.SizeBytes,
		CreatedAt:   file.CreatedAt,
	}
}
