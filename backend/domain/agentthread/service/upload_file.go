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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const RuntimeUploadVirtualPathPrefix = "/mnt/user-data/uploads/"

var runtimeUploadObjectURIPattern = regexp.MustCompile(
	`^agent-runtime/([1-9][0-9]*)/([1-9][0-9]*)/uploads/(.+)$`,
)

type UploadFileThreadReader interface {
	GetThread(ctx context.Context, id int64) (*entity.Thread, error)
}

type UploadFileRepository interface {
	CreateRuntimeFile(ctx context.Context, file *entity.AgentFile) error
	GetRuntimeFile(
		ctx context.Context,
		runID int64,
		virtualPath string,
	) (*entity.AgentFile, error)
	ListThreadUploadFiles(
		ctx context.Context,
		req repository.ListThreadUploadFilesRequest,
	) ([]*entity.AgentFile, error)
	MarkThreadUploadFileDeleted(
		ctx context.Context,
		req repository.MarkThreadUploadFileDeletedRequest,
	) (*entity.AgentFile, bool, error)
}

type RegisterUploadFileRequest struct {
	SpaceID          int64
	UserID           int64
	ThreadID         int64
	FileName         string
	OriginalFileName string
	ContentType      string
	SizeBytes        int64
	Digest           string
	Metadata         string
}

type ListUploadFilesRequest struct {
	SpaceID  int64
	UserID   int64
	ThreadID int64
	Limit    int
}

type DeleteUploadFileRequest struct {
	SpaceID   int64
	UserID    int64
	ThreadID  int64
	FileName  string
	DeletedAt int64
}

type UploadFileService interface {
	RegisterUploadFile(
		ctx context.Context,
		req *RegisterUploadFileRequest,
	) (*entity.AgentFile, error)
	ListUploadFiles(
		ctx context.Context,
		req *ListUploadFilesRequest,
	) ([]*entity.AgentFile, error)
	DeleteUploadFile(
		ctx context.Context,
		req *DeleteUploadFileRequest,
	) (*entity.AgentFile, bool, error)
}

type UploadFileComponents struct {
	ThreadReader UploadFileThreadReader
	FileRepo     UploadFileRepository
	IDGen        idgen.IDGenerator
}

type uploadFileService struct {
	threadReader UploadFileThreadReader
	fileRepo     UploadFileRepository
	idGen        idgen.IDGenerator
}

type runtimeUploadPath struct {
	FileName string
}

type runtimeUploadObjectURI struct {
	SpaceID  int64
	ThreadID int64
	FileName string
}

func NewUploadFileService(c *UploadFileComponents) UploadFileService {
	if c == nil {
		return &uploadFileService{}
	}
	return &uploadFileService{
		threadReader: c.ThreadReader,
		fileRepo:     c.FileRepo,
		idGen:        c.IDGen,
	}
}

func UploadVirtualPath(fileName string) string {
	return RuntimeUploadVirtualPathPrefix + fileName
}

func UploadObjectURI(spaceID int64, threadID int64, fileName string) string {
	return fmt.Sprintf(
		"agent-runtime/%d/%d/uploads/%s",
		spaceID,
		threadID,
		fileName,
	)
}

func parseRuntimeUploadVirtualPath(value string) (runtimeUploadPath, error) {
	if value == "" ||
		strings.Contains(value, `\`) ||
		strings.IndexFunc(value, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(value) != value ||
		!strings.HasPrefix(value, RuntimeUploadVirtualPathPrefix) {
		return runtimeUploadPath{}, InvalidArgumentErrorf(
			"runtime upload virtual path is invalid",
		)
	}
	fileName := strings.TrimPrefix(value, RuntimeUploadVirtualPathPrefix)
	normalized, err := NormalizeUploadFileName(fileName)
	if err != nil {
		return runtimeUploadPath{}, err
	}
	return runtimeUploadPath{FileName: normalized}, nil
}

func parseRuntimeUploadObjectURI(value string) (runtimeUploadObjectURI, error) {
	if value == "" ||
		strings.Contains(value, "://") ||
		strings.HasPrefix(value, "/") ||
		strings.Contains(value, `\`) ||
		strings.IndexFunc(value, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(value) != value {
		return runtimeUploadObjectURI{}, InvalidArgumentErrorf(
			"runtime upload object uri is invalid",
		)
	}
	match := runtimeUploadObjectURIPattern.FindStringSubmatch(value)
	if len(match) != 4 {
		return runtimeUploadObjectURI{}, InvalidArgumentErrorf(
			"runtime upload object uri is invalid",
		)
	}
	spaceID, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || spaceID <= 0 {
		return runtimeUploadObjectURI{}, InvalidArgumentErrorf(
			"runtime upload object uri is invalid",
		)
	}
	threadID, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil || threadID <= 0 {
		return runtimeUploadObjectURI{}, InvalidArgumentErrorf(
			"runtime upload object uri is invalid",
		)
	}
	fileName, err := NormalizeUploadFileName(match[3])
	if err != nil {
		return runtimeUploadObjectURI{}, InvalidArgumentErrorf(
			"runtime upload object uri is invalid",
		)
	}

	return runtimeUploadObjectURI{
		SpaceID:  spaceID,
		ThreadID: threadID,
		FileName: fileName,
	}, nil
}

func NormalizeUploadFileName(value string) (string, error) {
	fileName := strings.TrimSpace(value)
	if fileName == "" ||
		fileName == "." ||
		fileName == ".." ||
		strings.Contains(fileName, "/") ||
		strings.Contains(fileName, `\`) ||
		strings.IndexFunc(fileName, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(fileName) != fileName ||
		path.Base(fileName) != fileName ||
		len([]rune(fileName)) > 255 {
		return "", InvalidArgumentErrorf("upload file name is invalid")
	}
	return fileName, nil
}

func validateRuntimeFileDigest(digest string) error {
	decodedDigest, err := hex.DecodeString(digest)
	if err != nil || len(decodedDigest) != 32 || strings.ToLower(digest) != digest {
		return InvalidArgumentErrorf(
			"runtime file digest must be lowercase sha256",
		)
	}
	return nil
}

func (s *uploadFileService) RegisterUploadFile(
	ctx context.Context,
	req *RegisterUploadFileRequest,
) (*entity.AgentFile, error) {
	if s == nil || s.threadReader == nil || s.fileRepo == nil || s.idGen == nil {
		return nil, InvalidArgumentErrorf("upload file service is not configured")
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("register upload file request is required")
	}
	thread, err := s.requireOwnedThread(ctx, req.SpaceID, req.UserID, req.ThreadID)
	if err != nil {
		return nil, err
	}
	fileName, err := NormalizeUploadFileName(req.FileName)
	if err != nil {
		return nil, err
	}
	originalFileName := strings.TrimSpace(req.OriginalFileName)
	if originalFileName == "" {
		originalFileName = fileName
	} else {
		normalizedOriginal, err := NormalizeUploadFileName(originalFileName)
		if err != nil {
			return nil, err
		}
		originalFileName = normalizedOriginal
	}
	if req.SizeBytes <= 0 {
		return nil, InvalidArgumentErrorf("upload file size must be positive")
	}
	digest := strings.TrimSpace(req.Digest)
	if err := validateRuntimeFileDigest(digest); err != nil {
		return nil, err
	}
	metadata := strings.TrimSpace(req.Metadata)
	if metadata == "" {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		return nil, InvalidArgumentErrorf(
			"upload file metadata must be valid json",
		)
	}
	virtualPath := UploadVirtualPath(fileName)
	existing, err := s.fileRepo.GetRuntimeFile(ctx, 0, virtualPath)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, InvalidArgumentErrorf("upload file already exists")
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	file := &entity.AgentFile{
		ID:               id,
		SpaceID:          thread.SpaceID,
		UserID:           thread.CreatorID,
		ThreadID:         thread.ID,
		RunID:            0,
		FileName:         fileName,
		OriginalFileName: originalFileName,
		FileKind:         entity.AgentFileKindUpload,
		VirtualPath:      virtualPath,
		ObjectURI:        UploadObjectURI(thread.SpaceID, thread.ID, fileName),
		ContentType:      strings.TrimSpace(req.ContentType),
		SizeBytes:        req.SizeBytes,
		Digest:           digest,
		Status:           entity.AgentFileStatusActive,
		Metadata:         metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.fileRepo.CreateRuntimeFile(ctx, file); err != nil {
		return nil, err
	}
	return file, nil
}

func (s *uploadFileService) ListUploadFiles(
	ctx context.Context,
	req *ListUploadFilesRequest,
) ([]*entity.AgentFile, error) {
	if s == nil || s.threadReader == nil || s.fileRepo == nil {
		return nil, InvalidArgumentErrorf("upload file service is not configured")
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("list upload files request is required")
	}
	if _, err := s.requireOwnedThread(ctx, req.SpaceID, req.UserID, req.ThreadID); err != nil {
		return nil, err
	}
	return s.fileRepo.ListThreadUploadFiles(
		ctx,
		repository.ListThreadUploadFilesRequest{
			SpaceID:  req.SpaceID,
			UserID:   req.UserID,
			ThreadID: req.ThreadID,
			Limit:    req.Limit,
		},
	)
}

func (s *uploadFileService) DeleteUploadFile(
	ctx context.Context,
	req *DeleteUploadFileRequest,
) (*entity.AgentFile, bool, error) {
	if s == nil || s.threadReader == nil || s.fileRepo == nil {
		return nil, false, InvalidArgumentErrorf("upload file service is not configured")
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf("delete upload file request is required")
	}
	if _, err := s.requireOwnedThread(ctx, req.SpaceID, req.UserID, req.ThreadID); err != nil {
		return nil, false, err
	}
	fileName, err := NormalizeUploadFileName(req.FileName)
	if err != nil {
		return nil, false, err
	}
	deletedAt := req.DeletedAt
	if deletedAt <= 0 {
		deletedAt = time.Now().UnixMilli()
	}
	return s.fileRepo.MarkThreadUploadFileDeleted(
		ctx,
		repository.MarkThreadUploadFileDeletedRequest{
			SpaceID:   req.SpaceID,
			UserID:    req.UserID,
			ThreadID:  req.ThreadID,
			FileName:  fileName,
			DeletedAt: deletedAt,
		},
	)
}

func (s *uploadFileService) requireOwnedThread(
	ctx context.Context,
	spaceID int64,
	userID int64,
	threadID int64,
) (*entity.Thread, error) {
	if spaceID <= 0 || userID <= 0 || threadID <= 0 {
		return nil, InvalidArgumentErrorf("upload file scope is required")
	}
	thread, err := s.threadReader.GetThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil ||
		thread.ID != threadID ||
		thread.SpaceID != spaceID ||
		thread.CreatorID != userID {
		return nil, InvalidArgumentErrorf("upload file thread is invalid")
	}
	return thread, nil
}
