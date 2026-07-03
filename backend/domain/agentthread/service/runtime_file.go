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
	"encoding/json"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const RuntimeOffloadVirtualPathPrefix = "/mnt/user-data/workspace/.coze/tool-results/runs/"
const RuntimeOutputVirtualPathPrefix = "/mnt/user-data/outputs/"

var runtimeOffloadVirtualPathPattern = regexp.MustCompile(
	`^/mnt/user-data/workspace/\.coze/tool-results/runs/([1-9][0-9]*)/(trunc|clear)/([0-9a-f]{64})\.txt$`,
)

var runtimeOffloadObjectURIPattern = regexp.MustCompile(
	`^agent-runtime/([1-9][0-9]*)/([1-9][0-9]*)/runs/([1-9][0-9]*)/tool-results/(trunc|clear)/([0-9a-f]{64})\.txt$`,
)

var runtimeOutputObjectURIPattern = regexp.MustCompile(
	`^agent-runtime/([1-9][0-9]*)/([1-9][0-9]*)/runs/([1-9][0-9]*)/outputs/(.+)$`,
)

type RuntimeFileRunReader interface {
	GetRun(ctx context.Context, id int64) (*entity.Run, error)
}

type RegisterRuntimeFileRequest struct {
	RunID            int64
	FileName         string
	OriginalFileName string
	FileKind         entity.AgentFileKind
	VirtualPath      string
	ObjectURI        string
	ContentType      string
	SizeBytes        int64
	Digest           string
	Metadata         string
}

type ResolveRuntimeFileRequest struct {
	SpaceID     int64
	ThreadID    int64
	RunID       int64
	VirtualPath string
}

type RuntimeFileService interface {
	RegisterRuntimeFile(
		ctx context.Context,
		req *RegisterRuntimeFileRequest,
	) (*entity.AgentFile, bool, error)
	ResolveRuntimeFile(
		ctx context.Context,
		req *ResolveRuntimeFileRequest,
	) (*entity.AgentFile, error)
}

type RuntimeFileComponents struct {
	RunReader RuntimeFileRunReader
	FileRepo  repository.RuntimeFileRepository
	IDGen     idgen.IDGenerator
}

type runtimeFileService struct {
	runReader RuntimeFileRunReader
	fileRepo  repository.RuntimeFileRepository
	idGen     idgen.IDGenerator
}

type runtimeOffloadPath struct {
	RunID    int64
	Phase    string
	FileName string
}

type runtimeOffloadObjectURI struct {
	SpaceID  int64
	ThreadID int64
	RunID    int64
	Phase    string
	FileName string
}

type runtimeOutputPath struct {
	RelativePath string
	FileName     string
}

type runtimeOutputObjectURI struct {
	SpaceID      int64
	ThreadID     int64
	RunID        int64
	RelativePath string
	FileName     string
}

func NewRuntimeFileService(c *RuntimeFileComponents) RuntimeFileService {
	if c == nil {
		return &runtimeFileService{}
	}
	return &runtimeFileService{
		runReader: c.RunReader,
		fileRepo:  c.FileRepo,
		idGen:     c.IDGen,
	}
}

func parseRuntimeOffloadVirtualPath(value string) (runtimeOffloadPath, error) {
	if value == "" ||
		strings.Contains(value, `\`) ||
		strings.IndexFunc(value, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(value) != value {
		return runtimeOffloadPath{}, InvalidArgumentErrorf(
			"runtime offload virtual path is invalid",
		)
	}
	match := runtimeOffloadVirtualPathPattern.FindStringSubmatch(value)
	if len(match) != 4 {
		return runtimeOffloadPath{}, InvalidArgumentErrorf(
			"runtime offload virtual path is invalid",
		)
	}
	runID, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || runID <= 0 {
		return runtimeOffloadPath{}, InvalidArgumentErrorf(
			"runtime offload virtual path run id is invalid",
		)
	}

	return runtimeOffloadPath{
		RunID:    runID,
		Phase:    match[2],
		FileName: match[3] + ".txt",
	}, nil
}

func parseRuntimeOffloadObjectURI(value string) (runtimeOffloadObjectURI, error) {
	if value == "" ||
		strings.Contains(value, "://") ||
		strings.HasPrefix(value, "/") ||
		strings.Contains(value, `\`) ||
		strings.IndexFunc(value, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(value) != value {
		return runtimeOffloadObjectURI{}, InvalidArgumentErrorf(
			"runtime offload object uri is invalid",
		)
	}
	match := runtimeOffloadObjectURIPattern.FindStringSubmatch(value)
	if len(match) != 6 {
		return runtimeOffloadObjectURI{}, InvalidArgumentErrorf(
			"runtime offload object uri is invalid",
		)
	}

	spaceID, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || spaceID <= 0 {
		return runtimeOffloadObjectURI{}, InvalidArgumentErrorf(
			"runtime offload object uri is invalid",
		)
	}
	threadID, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil || threadID <= 0 {
		return runtimeOffloadObjectURI{}, InvalidArgumentErrorf(
			"runtime offload object uri is invalid",
		)
	}
	runID, err := strconv.ParseInt(match[3], 10, 64)
	if err != nil || runID <= 0 {
		return runtimeOffloadObjectURI{}, InvalidArgumentErrorf(
			"runtime offload object uri is invalid",
		)
	}

	return runtimeOffloadObjectURI{
		SpaceID:  spaceID,
		ThreadID: threadID,
		RunID:    runID,
		Phase:    match[4],
		FileName: match[5] + ".txt",
	}, nil
}

func parseRuntimeOutputVirtualPath(value string) (runtimeOutputPath, error) {
	if value == "" ||
		strings.Contains(value, `\`) ||
		strings.IndexFunc(value, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(value) != value ||
		!strings.HasPrefix(value, RuntimeOutputVirtualPathPrefix) {
		return runtimeOutputPath{}, InvalidArgumentErrorf(
			"runtime output virtual path is invalid",
		)
	}
	relativePath := strings.TrimPrefix(value, RuntimeOutputVirtualPathPrefix)
	normalized, err := normalizeRuntimeOutputRelativePath(relativePath)
	if err != nil {
		return runtimeOutputPath{}, err
	}

	return runtimeOutputPath{
		RelativePath: normalized,
		FileName:     path.Base(normalized),
	}, nil
}

func parseRuntimeOutputObjectURI(value string) (runtimeOutputObjectURI, error) {
	if value == "" ||
		strings.Contains(value, "://") ||
		strings.HasPrefix(value, "/") ||
		strings.Contains(value, `\`) ||
		strings.IndexFunc(value, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(value) != value {
		return runtimeOutputObjectURI{}, InvalidArgumentErrorf(
			"runtime output object uri is invalid",
		)
	}
	match := runtimeOutputObjectURIPattern.FindStringSubmatch(value)
	if len(match) != 5 {
		return runtimeOutputObjectURI{}, InvalidArgumentErrorf(
			"runtime output object uri is invalid",
		)
	}
	spaceID, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil || spaceID <= 0 {
		return runtimeOutputObjectURI{}, InvalidArgumentErrorf(
			"runtime output object uri is invalid",
		)
	}
	threadID, err := strconv.ParseInt(match[2], 10, 64)
	if err != nil || threadID <= 0 {
		return runtimeOutputObjectURI{}, InvalidArgumentErrorf(
			"runtime output object uri is invalid",
		)
	}
	runID, err := strconv.ParseInt(match[3], 10, 64)
	if err != nil || runID <= 0 {
		return runtimeOutputObjectURI{}, InvalidArgumentErrorf(
			"runtime output object uri is invalid",
		)
	}
	relativePath, err := normalizeRuntimeOutputRelativePath(match[4])
	if err != nil {
		return runtimeOutputObjectURI{}, InvalidArgumentErrorf(
			"runtime output object uri is invalid",
		)
	}

	return runtimeOutputObjectURI{
		SpaceID:      spaceID,
		ThreadID:     threadID,
		RunID:        runID,
		RelativePath: relativePath,
		FileName:     path.Base(relativePath),
	}, nil
}

func normalizeRuntimeOutputRelativePath(value string) (string, error) {
	relativePath := strings.TrimSpace(value)
	if relativePath == "" ||
		strings.HasPrefix(relativePath, "/") ||
		strings.Contains(relativePath, `\`) ||
		strings.IndexFunc(relativePath, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(relativePath) != relativePath ||
		relativePath == "." ||
		relativePath == ".." ||
		strings.HasPrefix(relativePath, "../") ||
		strings.Contains(relativePath, "/../") {
		return "", InvalidArgumentErrorf(
			"runtime output relative path is invalid",
		)
	}
	if base := path.Base(relativePath); base == "." ||
		base == "/" ||
		base == "" {
		return "", InvalidArgumentErrorf(
			"runtime output relative path is invalid",
		)
	}

	return relativePath, nil
}

func (s *runtimeFileService) RegisterRuntimeFile(
	ctx context.Context,
	req *RegisterRuntimeFileRequest,
) (*entity.AgentFile, bool, error) {
	if s == nil || s.runReader == nil || s.fileRepo == nil || s.idGen == nil {
		return nil, false, InvalidArgumentErrorf(
			"runtime file service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"register runtime file request is required",
		)
	}
	if req.RunID <= 0 {
		return nil, false, InvalidArgumentErrorf("run id is required")
	}
	virtualPath := strings.TrimSpace(req.VirtualPath)
	fileName := strings.TrimSpace(req.FileName)
	objectURI := strings.TrimSpace(req.ObjectURI)

	var objectSpaceID int64
	var objectThreadID int64
	var objectRunID int64
	switch req.FileKind {
	case entity.AgentFileKindWorkspace:
		parsedPath, err := parseRuntimeOffloadVirtualPath(virtualPath)
		if err != nil || parsedPath.RunID != req.RunID {
			return nil, false, InvalidArgumentErrorf(
				"runtime offload virtual path is invalid",
			)
		}
		if fileName == "" ||
			parsedPath.FileName != fileName ||
			path.Base(virtualPath) != fileName {
			return nil, false, InvalidArgumentErrorf(
				"runtime offload file name is invalid",
			)
		}
		parsedObjectURI, err := parseRuntimeOffloadObjectURI(objectURI)
		if err != nil ||
			parsedObjectURI.RunID != parsedPath.RunID ||
			parsedObjectURI.Phase != parsedPath.Phase ||
			parsedObjectURI.FileName != parsedPath.FileName {
			return nil, false, InvalidArgumentErrorf(
				"runtime offload object uri is invalid",
			)
		}
		objectSpaceID = parsedObjectURI.SpaceID
		objectThreadID = parsedObjectURI.ThreadID
		objectRunID = parsedObjectURI.RunID
	case entity.AgentFileKindOutput:
		parsedPath, err := parseRuntimeOutputVirtualPath(virtualPath)
		if err != nil {
			return nil, false, err
		}
		if fileName == "" ||
			parsedPath.FileName != fileName ||
			path.Base(virtualPath) != fileName {
			return nil, false, InvalidArgumentErrorf(
				"runtime output file name is invalid",
			)
		}
		parsedObjectURI, err := parseRuntimeOutputObjectURI(objectURI)
		if err != nil ||
			parsedObjectURI.RunID != req.RunID ||
			parsedObjectURI.RelativePath != parsedPath.RelativePath ||
			parsedObjectURI.FileName != parsedPath.FileName {
			return nil, false, InvalidArgumentErrorf(
				"runtime output object uri is invalid",
			)
		}
		objectSpaceID = parsedObjectURI.SpaceID
		objectThreadID = parsedObjectURI.ThreadID
		objectRunID = parsedObjectURI.RunID
	default:
		return nil, false, InvalidArgumentErrorf(
			"runtime file kind is invalid",
		)
	}
	if req.SizeBytes <= 0 {
		return nil, false, InvalidArgumentErrorf(
			"runtime file size must be positive",
		)
	}
	digest := strings.TrimSpace(req.Digest)
	if err := validateRuntimeFileDigest(digest); err != nil {
		return nil, false, err
	}
	metadata := strings.TrimSpace(req.Metadata)
	if metadata == "" {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		return nil, false, InvalidArgumentErrorf(
			"runtime file metadata must be valid json",
		)
	}

	run, err := s.runReader.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, false, err
	}
	if run == nil || run.ID != req.RunID {
		return nil, false, InvalidArgumentErrorf("runtime file run is invalid")
	}
	if objectSpaceID != run.SpaceID ||
		objectThreadID != run.ThreadID ||
		objectRunID != run.ID {
		return nil, false, InvalidArgumentErrorf(
			"runtime file object uri is invalid",
		)
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UnixMilli()
	return s.fileRepo.UpsertRuntimeFile(ctx, &entity.AgentFile{
		ID:               id,
		SpaceID:          run.SpaceID,
		UserID:           run.CreatorID,
		ThreadID:         run.ThreadID,
		RunID:            run.ID,
		FileName:         fileName,
		OriginalFileName: strings.TrimSpace(req.OriginalFileName),
		FileKind:         req.FileKind,
		VirtualPath:      virtualPath,
		ObjectURI:        objectURI,
		ContentType:      strings.TrimSpace(req.ContentType),
		SizeBytes:        req.SizeBytes,
		Digest:           digest,
		Status:           entity.AgentFileStatusActive,
		Metadata:         metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
}

func (s *runtimeFileService) ResolveRuntimeFile(
	ctx context.Context,
	req *ResolveRuntimeFileRequest,
) (*entity.AgentFile, error) {
	if s == nil || s.fileRepo == nil {
		return nil, InvalidArgumentErrorf(
			"runtime file service is not configured",
		)
	}
	if req == nil {
		return nil, InvalidArgumentErrorf(
			"resolve runtime file request is required",
		)
	}
	if req.SpaceID <= 0 || req.ThreadID <= 0 || req.RunID <= 0 {
		return nil, InvalidArgumentErrorf("runtime file scope is required")
	}
	virtualPath := strings.TrimSpace(req.VirtualPath)
	var expectedKind entity.AgentFileKind
	var expectedFileName string
	var expectedPhase string
	var expectedOutputRelativePath string
	expectedRunID := req.RunID
	if strings.HasPrefix(virtualPath, RuntimeOutputVirtualPathPrefix) {
		parsedPath, err := parseRuntimeOutputVirtualPath(virtualPath)
		if err != nil {
			return nil, err
		}
		expectedKind = entity.AgentFileKindOutput
		expectedFileName = parsedPath.FileName
		expectedOutputRelativePath = parsedPath.RelativePath
	} else if strings.HasPrefix(virtualPath, RuntimeUploadVirtualPathPrefix) {
		parsedPath, err := parseRuntimeUploadVirtualPath(virtualPath)
		if err != nil {
			return nil, err
		}
		expectedKind = entity.AgentFileKindUpload
		expectedFileName = parsedPath.FileName
		expectedRunID = 0
	} else {
		parsedPath, err := parseRuntimeOffloadVirtualPath(virtualPath)
		if err != nil || parsedPath.RunID != req.RunID {
			return nil, InvalidArgumentErrorf(
				"runtime offload virtual path is invalid",
			)
		}
		expectedKind = entity.AgentFileKindWorkspace
		expectedFileName = parsedPath.FileName
		expectedPhase = parsedPath.Phase
	}

	file, err := s.fileRepo.GetRuntimeFile(ctx, expectedRunID, virtualPath)
	if err != nil {
		return nil, err
	}
	if file == nil ||
		file.SpaceID != req.SpaceID ||
		file.ThreadID != req.ThreadID ||
		file.RunID != expectedRunID ||
		file.FileKind != expectedKind ||
		file.Status != entity.AgentFileStatusActive ||
		file.VirtualPath != virtualPath ||
		file.FileName != expectedFileName {
		return nil, InvalidArgumentErrorf(
			"runtime file is invalid",
		)
	}

	switch expectedKind {
	case entity.AgentFileKindWorkspace:
		parsedObjectURI, err := parseRuntimeOffloadObjectURI(
			strings.TrimSpace(file.ObjectURI),
		)
		if err != nil ||
			parsedObjectURI.SpaceID != req.SpaceID ||
			parsedObjectURI.ThreadID != req.ThreadID ||
			parsedObjectURI.RunID != req.RunID ||
			parsedObjectURI.Phase != expectedPhase ||
			parsedObjectURI.FileName != expectedFileName {
			return nil, InvalidArgumentErrorf(
				"runtime offload object uri is invalid",
			)
		}
	case entity.AgentFileKindOutput:
		parsedObjectURI, err := parseRuntimeOutputObjectURI(
			strings.TrimSpace(file.ObjectURI),
		)
		if err != nil ||
			parsedObjectURI.SpaceID != req.SpaceID ||
			parsedObjectURI.ThreadID != req.ThreadID ||
			parsedObjectURI.RunID != req.RunID ||
			parsedObjectURI.RelativePath != expectedOutputRelativePath ||
			parsedObjectURI.FileName != expectedFileName {
			return nil, InvalidArgumentErrorf(
				"runtime output object uri is invalid",
			)
		}
	case entity.AgentFileKindUpload:
		parsedObjectURI, err := parseRuntimeUploadObjectURI(
			strings.TrimSpace(file.ObjectURI),
		)
		if err != nil ||
			parsedObjectURI.SpaceID != req.SpaceID ||
			parsedObjectURI.ThreadID != req.ThreadID ||
			parsedObjectURI.FileName != expectedFileName {
			return nil, InvalidArgumentErrorf(
				"runtime upload object uri is invalid",
			)
		}
	}
	if file.SizeBytes <= 0 {
		return nil, InvalidArgumentErrorf(
			"runtime file size must be positive",
		)
	}
	digest := strings.TrimSpace(file.Digest)
	if err := validateRuntimeFileDigest(digest); err != nil {
		return nil, err
	}
	metadata := strings.TrimSpace(file.Metadata)
	if metadata != "" && !json.Valid([]byte(metadata)) {
		return nil, InvalidArgumentErrorf(
			"runtime offload metadata must be valid json",
		)
	}
	return file, nil
}
