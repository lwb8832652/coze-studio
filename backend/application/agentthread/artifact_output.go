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
	"mime"
	"net/http"
	"path"
	"strings"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

const outputFileWrittenSchema = "coze.output_file_written.v1"
const artifactPresentedSchema = "coze.artifact_presented.v1"
const artifactPresentedEvent = "artifact.presented"
const maxOutputFileWriteBytes = defaultADKMaxOffloadBytes

type ArtifactObjectWriter interface {
	PutObject(
		ctx context.Context,
		objectKey string,
		content []byte,
		opts ...storage.PutOptFn,
	) error
}

func (s *ApplicationService) WriteOutputFile(
	ctx context.Context,
	req *WriteOutputFileRequest,
) (*WriteOutputFileResponse, error) {
	if s == nil || s.RuntimeFileSVC == nil {
		return nil, fmt.Errorf("agent runtime file service is not initialized")
	}
	writer, ok := s.ArtifactObjectStorage.(ArtifactObjectWriter)
	if s.ArtifactObjectStorage == nil || !ok {
		return nil, fmt.Errorf("artifact object storage writer is not initialized")
	}
	if req == nil || req.Run == nil {
		return nil, fmt.Errorf("write output file request is required")
	}
	if req.Run.RunID <= 0 || req.Run.ThreadID <= 0 || req.Run.SpaceID <= 0 {
		return nil, fmt.Errorf("write output file run scope is invalid")
	}
	virtualPath, relativePath, err := normalizeOutputVirtualPath(req.FilePath)
	if err != nil {
		return nil, err
	}
	content := []byte(req.Content)
	if len(content) == 0 || len(content) > maxOutputFileWriteBytes {
		return nil, fmt.Errorf("write output file content size is invalid")
	}
	contentType := outputContentType(req.ContentType, virtualPath, content)
	objectKey := outputObjectKey(req.Run, relativePath)
	if err := writer.PutObject(
		ctx,
		objectKey,
		content,
		storage.WithContentType(contentType),
		storage.WithObjectSize(int64(len(content))),
	); err != nil {
		return nil, fmt.Errorf("write output file storage failed")
	}

	digestBytes := sha256.Sum256(content)
	digest := hex.EncodeToString(digestBytes[:])
	metadata := encodeRunEventPayload(ctx, map[string]any{
		"purpose": "agent_output",
		"source":  "write_file",
	})
	file, created, err := s.RuntimeFileSVC.RegisterRuntimeFile(
		ctx,
		&domainservice.RegisterRuntimeFileRequest{
			RunID:            req.Run.RunID,
			FileName:         path.Base(relativePath),
			OriginalFileName: path.Base(relativePath),
			FileKind:         domainentity.AgentFileKindOutput,
			VirtualPath:      virtualPath,
			ObjectURI:        objectKey,
			ContentType:      contentType,
			SizeBytes:        int64(len(content)),
			Digest:           digest,
			Metadata:         metadata,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("register output file failed: %w", err)
	}
	if file == nil {
		return nil, fmt.Errorf("agent runtime file service returned empty file")
	}

	summary := outputFileSummary(file, virtualPath, contentType, int64(len(content)), digest)
	notice := encodeRunEventPayload(ctx, map[string]any{
		"schema":       outputFileWrittenSchema,
		"file_id":      summary.FileID,
		"file_path":    summary.VirtualPath,
		"file_name":    summary.FileName,
		"content_type": summary.ContentType,
		"size_bytes":   summary.SizeBytes,
		"digest":       summary.Digest,
		"next":         "call present_files with file_path to show this file to the user",
	})

	return &WriteOutputFileResponse{
		File:    summary,
		Created: created,
		Notice:  notice,
	}, nil
}

func (s *ApplicationService) PresentOutputFiles(
	ctx context.Context,
	req *PresentOutputFilesRequest,
) (*PresentOutputFilesResponse, error) {
	if s == nil || s.RuntimeFileSVC == nil {
		return nil, fmt.Errorf("agent runtime file service is not initialized")
	}
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil || req.Run == nil {
		return nil, fmt.Errorf("present output files request is required")
	}
	if req.Run.RunID <= 0 || req.Run.ThreadID <= 0 || req.Run.SpaceID <= 0 {
		return nil, fmt.Errorf("present output files run scope is invalid")
	}
	paths, err := normalizeOutputFilePaths(req.FilePaths)
	if err != nil {
		return nil, err
	}

	resp := &PresentOutputFilesResponse{
		Artifacts: make([]*ArtifactSummary, 0, len(paths)),
	}
	for _, virtualPath := range paths {
		file, err := s.RuntimeFileSVC.ResolveRuntimeFile(
			ctx,
			&domainservice.ResolveRuntimeFileRequest{
				SpaceID:     req.Run.SpaceID,
				ThreadID:    req.Run.ThreadID,
				RunID:       req.Run.RunID,
				VirtualPath: virtualPath,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("resolve output file failed: %w", err)
		}
		if err := validatePresentableOutputFile(req.Run, file, virtualPath); err != nil {
			return nil, err
		}
		artifact, _, err := s.ArtifactSVC.RegisterArtifact(
			ctx,
			&domainservice.RegisterArtifactRequest{
				SpaceID:      req.Run.SpaceID,
				ThreadID:     req.Run.ThreadID,
				RunID:        req.Run.RunID,
				FileID:       file.ID,
				Title:        fileTitle(file),
				ArtifactType: outputArtifactType(file),
				Metadata:     `{"source":"present_files"}`,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("register presented artifact failed: %w", err)
		}
		if artifact == nil {
			return nil, fmt.Errorf("agent artifact service returned empty artifact")
		}
		resp.Artifacts = append(resp.Artifacts, DomainArtifactToSummary(artifact))
	}

	if err := s.emitArtifactPresentedEvent(ctx, req.Run, resp.Artifacts); err != nil {
		return nil, err
	}
	resp.Notice = artifactPresentedNotice(ctx, resp.Artifacts)

	return resp, nil
}

func normalizeOutputFilePaths(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("present output files requires at least one file")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		virtualPath, _, err := normalizeOutputVirtualPath(value)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[virtualPath]; ok {
			continue
		}
		seen[virtualPath] = struct{}{}
		result = append(result, virtualPath)
	}
	return result, nil
}

func normalizeOutputVirtualPath(value string) (string, string, error) {
	virtualPath := strings.TrimSpace(value)
	if virtualPath == "" ||
		strings.Contains(virtualPath, `\`) ||
		strings.IndexFunc(virtualPath, func(r rune) bool {
			return r < 0x20 || r == 0x7f
		}) >= 0 ||
		path.Clean(virtualPath) != virtualPath ||
		!strings.HasPrefix(virtualPath, domainservice.RuntimeOutputVirtualPathPrefix) {
		return "", "", fmt.Errorf("output file path must be under /mnt/user-data/outputs")
	}
	relativePath := strings.TrimPrefix(
		virtualPath,
		domainservice.RuntimeOutputVirtualPathPrefix,
	)
	if relativePath == "" ||
		strings.HasPrefix(relativePath, "/") ||
		path.Clean(relativePath) != relativePath ||
		relativePath == "." ||
		relativePath == ".." ||
		strings.HasPrefix(relativePath, "../") ||
		strings.Contains(relativePath, "/../") {
		return "", "", fmt.Errorf("output file path is invalid")
	}
	return virtualPath, relativePath, nil
}

func outputObjectKey(run *RunSummary, relativePath string) string {
	return fmt.Sprintf(
		"agent-runtime/%d/%d/runs/%d/outputs/%s",
		run.SpaceID,
		run.ThreadID,
		run.RunID,
		relativePath,
	)
}

func outputContentType(requested string, virtualPath string, content []byte) string {
	contentType := strings.TrimSpace(requested)
	if contentType != "" {
		return contentType
	}
	if byExt := mime.TypeByExtension(path.Ext(virtualPath)); byExt != "" {
		return byExt
	}
	if len(content) > 0 {
		return http.DetectContentType(content)
	}
	return "application/octet-stream"
}

func outputFileSummary(
	file *domainentity.AgentFile,
	virtualPath string,
	contentType string,
	sizeBytes int64,
	digest string,
) *OutputFileSummary {
	if file == nil {
		return nil
	}
	fileName := strings.TrimSpace(file.FileName)
	if fileName == "" {
		fileName = path.Base(virtualPath)
	}
	if strings.TrimSpace(file.VirtualPath) != "" {
		virtualPath = strings.TrimSpace(file.VirtualPath)
	}
	if strings.TrimSpace(file.ContentType) != "" {
		contentType = strings.TrimSpace(file.ContentType)
	}
	if file.SizeBytes > 0 {
		sizeBytes = file.SizeBytes
	}
	if strings.TrimSpace(file.Digest) != "" {
		digest = strings.TrimSpace(file.Digest)
	}
	return &OutputFileSummary{
		FileID:      file.ID,
		FileName:    fileName,
		VirtualPath: virtualPath,
		ContentType: contentType,
		SizeBytes:   sizeBytes,
		Digest:      digest,
	}
}

func validatePresentableOutputFile(
	run *RunSummary,
	file *domainentity.AgentFile,
	virtualPath string,
) error {
	if file == nil ||
		file.ID <= 0 ||
		file.SpaceID != run.SpaceID ||
		file.ThreadID != run.ThreadID ||
		file.RunID != run.RunID ||
		file.FileKind != domainentity.AgentFileKindOutput ||
		file.Status != domainentity.AgentFileStatusActive ||
		strings.TrimSpace(file.VirtualPath) != virtualPath ||
		strings.TrimSpace(file.ObjectURI) == "" ||
		file.SizeBytes <= 0 {
		return fmt.Errorf("output file is not presentable")
	}
	return nil
}

func fileTitle(file *domainentity.AgentFile) string {
	if file == nil {
		return "artifact"
	}
	if title := strings.TrimSpace(file.OriginalFileName); title != "" {
		return title
	}
	if title := strings.TrimSpace(file.FileName); title != "" {
		return title
	}
	return "artifact"
}

func outputArtifactType(file *domainentity.AgentFile) string {
	if file == nil {
		return "document"
	}
	contentType := strings.ToLower(strings.TrimSpace(file.ContentType))
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.Contains(contentType, "csv"),
		strings.Contains(contentType, "json"),
		strings.Contains(contentType, "spreadsheet"):
		return "data"
	default:
		return "document"
	}
}

func (s *ApplicationService) emitArtifactPresentedEvent(
	ctx context.Context,
	run *RunSummary,
	artifacts []*ArtifactSummary,
) error {
	payload := map[string]any{
		"schema":         artifactPresentedSchema,
		"thread_id":      run.ThreadID,
		"run_id":         run.RunID,
		"artifact_count": len(artifacts),
		"artifacts":      safeArtifactEventItems(artifacts),
	}
	_, err := s.ThreadSVC.AppendRunEvent(
		ctx,
		&domainservice.AppendRunEventRequest{
			ThreadID:  run.ThreadID,
			RunID:     run.RunID,
			EventType: artifactPresentedEvent,
			Payload:   encodeRunEventPayload(ctx, payload),
		},
	)
	return err
}

func safeArtifactEventItems(artifacts []*ArtifactSummary) []map[string]any {
	result := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		result = append(result, map[string]any{
			"artifact_id":   artifact.ArtifactID,
			"file_id":       artifact.FileID,
			"title":         artifact.Title,
			"artifact_type": artifact.ArtifactType,
			"virtual_path":  artifact.VirtualPath,
			"content_type":  artifact.ContentType,
			"size_bytes":    artifact.SizeBytes,
			"preview_mode":  artifact.PreviewMode,
		})
	}
	return result
}

func artifactPresentedNotice(ctx context.Context, artifacts []*ArtifactSummary) string {
	return encodeRunEventPayload(ctx, map[string]any{
		"schema":         artifactPresentedSchema,
		"artifact_count": len(artifacts),
		"artifacts":      safeArtifactEventItems(artifacts),
	})
}
