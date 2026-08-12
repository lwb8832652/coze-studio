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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type RuntimeManager interface {
	Start(ctx context.Context, req *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error)
	Status(ctx context.Context, req *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error)
	KeepAlive(ctx context.Context, req *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error)
	Restart(ctx context.Context, req *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error)
	Stop(ctx context.Context, req *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error)
	Logs(ctx context.Context, req *RuntimeManagerRequest) ([]*domainappdev.RuntimeLog, error)
}

type RuntimeManagerRequest struct {
	SpaceID     string
	ActorUserID int64
	ProjectID   string
	ProjectDir  string
	SourceURL   string
	Snapshot    *RuntimeSnapshotReference
}

type RuntimeSnapshotReference struct {
	ID          string
	Path        string
	Digest      string
	Size        int64
	DownloadURL string
}

type SourceSnapshotRuntime interface {
	RequiresSourceSnapshot() bool
}

type ProjectSourceURLProvider interface {
	ProjectSourceURL(ctx context.Context, spaceID string, projectID string) (string, error)
}

type RuntimeRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
}

type RuntimeInfoDTO struct {
	Status          string `json:"status"`
	PreviewURL      string `json:"-"`
	Message         string `json:"message,omitempty"`
	LastKeepAliveAt string `json:"lastKeepAliveAt,omitempty"`
}

type RuntimeInfoResponse struct {
	Code    int64          `json:"code"`
	Message string         `json:"message"`
	Data    RuntimeInfoDTO `json:"data"`
}

type RuntimeLogDTO struct {
	ID        string `json:"id"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type RuntimeLogListData struct {
	Items []*RuntimeLogDTO `json:"items"`
}

type RuntimeLogListResponse struct {
	Code    int64              `json:"code"`
	Message string             `json:"message"`
	Data    RuntimeLogListData `json:"data"`
}

// StartRuntime is the legacy explicit debug path. Provider API callers must use
// ProviderAPIFacade; Task9.5 removes this path from production handlers.
func (s *Service) StartRuntime(ctx context.Context, req *RuntimeRequest) (*RuntimeInfoResponse, error) {
	managerReq, err := s.runtimeStartRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	info, err := s.runtime.Start(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	if err := s.persistRuntimeState(ctx, req, info); err != nil {
		return nil, s.rollbackRuntimeAfterPersistenceFailure(ctx, managerReq, err)
	}
	return successRuntimeInfoResponse(info), nil
}

func (s *Service) GetRuntimeStatus(ctx context.Context, req *RuntimeRequest) (*RuntimeInfoResponse, error) {
	managerReq, err := s.runtimeRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	info, err := s.runtime.Status(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	if err := s.persistRuntimeState(ctx, req, info); err != nil {
		return nil, err
	}
	return successRuntimeInfoResponse(info), nil
}

func (s *Service) KeepAliveRuntime(ctx context.Context, req *RuntimeRequest) (*RuntimeInfoResponse, error) {
	managerReq, err := s.runtimeRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	info, err := s.runtime.KeepAlive(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	if err := s.persistRuntimeState(ctx, req, info); err != nil {
		return nil, err
	}
	return successRuntimeInfoResponse(info), nil
}

func (s *Service) RestartRuntime(ctx context.Context, req *RuntimeRequest) (*RuntimeInfoResponse, error) {
	managerReq, err := s.runtimeStartRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	info, err := s.runtime.Restart(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	if err := s.persistRuntimeState(ctx, req, info); err != nil {
		return nil, s.rollbackRuntimeAfterPersistenceFailure(ctx, managerReq, err)
	}
	return successRuntimeInfoResponse(info), nil
}

func (s *Service) StopRuntime(ctx context.Context, req *RuntimeRequest) (*RuntimeInfoResponse, error) {
	managerReq, err := s.runtimeRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	info, err := s.runtime.Stop(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	if err := s.persistRuntimeState(ctx, req, info); err != nil {
		return nil, err
	}
	return successRuntimeInfoResponse(info), nil
}

func (s *Service) persistRuntimeState(ctx context.Context, req *RuntimeRequest, info *domainappdev.RuntimeInfo) error {
	if _, err := s.store.UpdateProjectRuntime(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), info.Status, info.PreviewURL); err != nil {
		return fmt.Errorf("persist appdev runtime state: %w", err)
	}
	return nil
}

func (s *Service) rollbackRuntimeAfterPersistenceFailure(
	ctx context.Context,
	managerReq *RuntimeManagerRequest,
	persistErr error,
) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if _, rollbackErr := s.runtime.Stop(rollbackCtx, managerReq); rollbackErr != nil {
		return errors.Join(persistErr, fmt.Errorf("rollback appdev runtime: %w", rollbackErr))
	}
	return persistErr
}

func (s *Service) ListRuntimeLogs(ctx context.Context, req *RuntimeRequest) (*RuntimeLogListResponse, error) {
	managerReq, err := s.runtimeRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	logs, err := s.runtime.Logs(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	items := make([]*RuntimeLogDTO, 0, len(logs))
	for _, logEntry := range logs {
		items = append(items, &RuntimeLogDTO{
			ID:        logEntry.ID,
			Level:     logEntry.Level,
			Message:   logEntry.Message,
			Timestamp: formatTime(logEntry.Timestamp),
		})
	}

	return &RuntimeLogListResponse{
		Code:    0,
		Message: "success",
		Data: RuntimeLogListData{
			Items: items,
		},
	}, nil
}

func (s *Service) runtimeRequest(ctx context.Context, req *RuntimeRequest) (*RuntimeManagerRequest, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("appdev runtime manager is not configured")
	}
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}

	if _, err := s.store.GetProject(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID)); err != nil {
		return nil, err
	}

	projectDir, err := s.store.ProjectFilesDir(strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID))
	if err != nil {
		return nil, err
	}

	managerRequest := &RuntimeManagerRequest{
		SpaceID:     strings.TrimSpace(req.SpaceID),
		ActorUserID: req.CurrentUserID,
		ProjectID:   strings.TrimSpace(req.ProjectID),
		ProjectDir:  projectDir,
	}
	if sourceProvider, ok := s.store.(ProjectSourceURLProvider); ok {
		managerRequest.SourceURL, err = sourceProvider.ProjectSourceURL(
			ctx,
			strings.TrimSpace(req.SpaceID),
			strings.TrimSpace(req.ProjectID),
		)
		if err != nil {
			return nil, err
		}
	}
	return managerRequest, nil
}

func (s *Service) runtimeStartRequest(ctx context.Context, req *RuntimeRequest) (*RuntimeManagerRequest, error) {
	managerRequest, err := s.runtimeRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	requirements, ok := s.runtime.(SourceSnapshotRuntime)
	if !ok || !requirements.RequiresSourceSnapshot() {
		return managerRequest, nil
	}
	archive, _, err := s.store.ExportProjectArchive(
		ctx,
		strings.TrimSpace(req.SpaceID),
		strings.TrimSpace(req.ProjectID),
	)
	if err != nil {
		return nil, fmt.Errorf("prepare appdev sandbox snapshot: %w", err)
	}
	if len(archive) == 0 || len(archive) > 100*1024*1024 {
		return nil, fmt.Errorf("appdev sandbox snapshot is invalid")
	}
	sourceProvider, ok := s.store.(ProjectSourceURLProvider)
	if !ok {
		return nil, fmt.Errorf("appdev source object provider is not configured")
	}
	downloadURL, err := sourceProvider.ProjectSourceURL(
		ctx,
		strings.TrimSpace(req.SpaceID),
		strings.TrimSpace(req.ProjectID),
	)
	if err != nil {
		return nil, fmt.Errorf("prepare appdev sandbox snapshot reference: %w", err)
	}
	normalizedURL, err := validateRuntimeSnapshotDownloadURL(downloadURL)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(archive)
	digestHex := hex.EncodeToString(digest[:])
	managerRequest.ProjectDir = ""
	managerRequest.SourceURL = ""
	managerRequest.Snapshot = &RuntimeSnapshotReference{
		ID:          "snapshot-" + digestHex[:32],
		Path:        "source.zip",
		Digest:      "sha256:" + digestHex,
		Size:        int64(len(archive)),
		DownloadURL: normalizedURL,
	}
	return managerRequest, nil
}

func validateRuntimeSnapshotDownloadURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 8*1024 {
		return "", fmt.Errorf("appdev sandbox snapshot URL is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("appdev sandbox snapshot URL must use trusted HTTPS")
	}
	return parsed.String(), nil
}

func successRuntimeInfoResponse(info *domainappdev.RuntimeInfo) *RuntimeInfoResponse {
	return &RuntimeInfoResponse{
		Code:    0,
		Message: "success",
		Data: RuntimeInfoDTO{
			Status:          string(info.Status),
			PreviewURL:      info.PreviewURL,
			Message:         info.Message,
			LastKeepAliveAt: formatTime(info.LastKeepAliveAt),
		},
	}
}
