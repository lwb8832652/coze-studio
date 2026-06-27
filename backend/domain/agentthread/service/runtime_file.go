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

var runtimeOffloadVirtualPathPattern = regexp.MustCompile(
	`^/mnt/user-data/workspace/\.coze/tool-results/runs/([1-9][0-9]*)/(trunc|clear)/([0-9a-f]{64})\.txt$`,
)

var runtimeOffloadObjectURIPattern = regexp.MustCompile(
	`^agent-runtime/([1-9][0-9]*)/([1-9][0-9]*)/runs/([1-9][0-9]*)/tool-results/(trunc|clear)/([0-9a-f]{64})\.txt$`,
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
	if req.FileKind != entity.AgentFileKindWorkspace {
		return nil, false, InvalidArgumentErrorf(
			"runtime offload file kind must be workspace",
		)
	}
	virtualPath := strings.TrimSpace(req.VirtualPath)
	parsedPath, err := parseRuntimeOffloadVirtualPath(virtualPath)
	if err != nil || parsedPath.RunID != req.RunID {
		return nil, false, InvalidArgumentErrorf(
			"runtime offload virtual path is invalid",
		)
	}
	fileName := strings.TrimSpace(req.FileName)
	if fileName == "" ||
		parsedPath.FileName != fileName ||
		path.Base(virtualPath) != fileName {
		return nil, false, InvalidArgumentErrorf(
			"runtime offload file name is invalid",
		)
	}
	objectURI := strings.TrimSpace(req.ObjectURI)
	parsedObjectURI, err := parseRuntimeOffloadObjectURI(objectURI)
	if err != nil ||
		parsedObjectURI.RunID != parsedPath.RunID ||
		parsedObjectURI.Phase != parsedPath.Phase ||
		parsedObjectURI.FileName != parsedPath.FileName {
		return nil, false, InvalidArgumentErrorf(
			"runtime offload object uri is invalid",
		)
	}
	if req.SizeBytes <= 0 {
		return nil, false, InvalidArgumentErrorf(
			"runtime offload size must be positive",
		)
	}
	digest := strings.TrimSpace(req.Digest)
	decodedDigest, err := hex.DecodeString(digest)
	if err != nil || len(decodedDigest) != 32 || strings.ToLower(digest) != digest {
		return nil, false, InvalidArgumentErrorf(
			"runtime offload digest must be lowercase sha256",
		)
	}
	metadata := strings.TrimSpace(req.Metadata)
	if metadata == "" {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		return nil, false, InvalidArgumentErrorf(
			"runtime offload metadata must be valid json",
		)
	}

	run, err := s.runReader.GetRun(ctx, req.RunID)
	if err != nil {
		return nil, false, err
	}
	if run == nil || run.ID != req.RunID {
		return nil, false, InvalidArgumentErrorf("runtime offload run is invalid")
	}
	if parsedObjectURI.SpaceID != run.SpaceID ||
		parsedObjectURI.ThreadID != run.ThreadID ||
		parsedObjectURI.RunID != run.ID {
		return nil, false, InvalidArgumentErrorf(
			"runtime offload object uri is invalid",
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
	parsedPath, err := parseRuntimeOffloadVirtualPath(virtualPath)
	if err != nil || parsedPath.RunID != req.RunID {
		return nil, InvalidArgumentErrorf(
			"runtime offload virtual path is invalid",
		)
	}

	file, err := s.fileRepo.GetRuntimeFile(ctx, req.RunID, virtualPath)
	if err != nil {
		return nil, err
	}
	if file == nil ||
		file.SpaceID != req.SpaceID ||
		file.ThreadID != req.ThreadID ||
		file.RunID != req.RunID ||
		file.FileKind != entity.AgentFileKindWorkspace ||
		file.Status != entity.AgentFileStatusActive ||
		file.VirtualPath != virtualPath ||
		file.FileName != parsedPath.FileName {
		return nil, InvalidArgumentErrorf(
			"runtime offload file is invalid",
		)
	}

	parsedObjectURI, err := parseRuntimeOffloadObjectURI(
		strings.TrimSpace(file.ObjectURI),
	)
	if err != nil ||
		parsedObjectURI.SpaceID != req.SpaceID ||
		parsedObjectURI.ThreadID != req.ThreadID ||
		parsedObjectURI.RunID != req.RunID ||
		parsedObjectURI.Phase != parsedPath.Phase ||
		parsedObjectURI.FileName != parsedPath.FileName {
		return nil, InvalidArgumentErrorf(
			"runtime offload object uri is invalid",
		)
	}
	if file.SizeBytes <= 0 {
		return nil, InvalidArgumentErrorf(
			"runtime offload size must be positive",
		)
	}
	digest := strings.TrimSpace(file.Digest)
	decodedDigest, err := hex.DecodeString(digest)
	if err != nil || len(decodedDigest) != 32 || strings.ToLower(digest) != digest {
		return nil, InvalidArgumentErrorf(
			"runtime offload digest must be lowercase sha256",
		)
	}
	metadata := strings.TrimSpace(file.Metadata)
	if metadata != "" && !json.Valid([]byte(metadata)) {
		return nil, InvalidArgumentErrorf(
			"runtime offload metadata must be valid json",
		)
	}
	return file, nil
}
