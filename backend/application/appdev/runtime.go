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
	"errors"
	"fmt"
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
	SpaceID    string
	ProjectID  string
	ProjectDir string
	SourceURL  string
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
	PreviewURL      string `json:"previewUrl,omitempty"`
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

func (s *Service) StartRuntime(ctx context.Context, req *RuntimeRequest) (*RuntimeInfoResponse, error) {
	managerReq, err := s.runtimeRequest(ctx, req)
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
	managerReq, err := s.runtimeRequest(ctx, req)
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
		SpaceID:    strings.TrimSpace(req.SpaceID),
		ProjectID:  strings.TrimSpace(req.ProjectID),
		ProjectDir: projectDir,
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
