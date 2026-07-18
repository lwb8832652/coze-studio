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

type CreateSnapshotRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	Label         string
}

type RestoreSnapshotRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	SnapshotID    string
}

type SnapshotDTO struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt string `json:"createdAt"`
}

type SnapshotResponse struct {
	Code    int64        `json:"code"`
	Message string       `json:"message"`
	Data    *SnapshotDTO `json:"data"`
}

type SnapshotListData struct {
	Items []*SnapshotDTO `json:"items"`
}

type SnapshotListResponse struct {
	Code    int64            `json:"code"`
	Message string           `json:"message"`
	Data    SnapshotListData `json:"data"`
}

func (s *Service) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (*SnapshotResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}
	snapshot, err := s.store.CreateProjectSnapshot(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID), req.Label)
	if err != nil {
		return nil, err
	}
	return &SnapshotResponse{
		Code:    0,
		Message: "success",
		Data:    toSnapshotDTO(snapshot),
	}, nil
}

func (s *Service) ListSnapshots(ctx context.Context, req *ProjectFileRequest) (*SnapshotListResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}
	snapshots, err := s.store.ListProjectSnapshots(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID))
	if err != nil {
		return nil, err
	}
	items := make([]*SnapshotDTO, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, toSnapshotDTO(snapshot))
	}
	return &SnapshotListResponse{
		Code:    0,
		Message: "success",
		Data:    SnapshotListData{Items: items},
	}, nil
}

// RestoreSnapshot is the legacy debug flow. Provider-enabled restoration uses
// ProviderAPIFacade.RestoreSnapshot so it cannot race the old RuntimeManager.
func (s *Service) RestoreSnapshot(ctx context.Context, req *RestoreSnapshotRequest) (*FileMutationResponse, error) {
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}
	spaceID := strings.TrimSpace(req.SpaceID)
	projectID := strings.TrimSpace(req.ProjectID)
	snapshotID := strings.TrimSpace(req.SnapshotID)
	if snapshotID == "" {
		return nil, fmt.Errorf("snapshot_id cannot be empty")
	}

	runtimeReq, runtimeWasActive, err := s.stopRuntimeForSnapshotRestore(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.store.RestoreProjectSnapshot(ctx, spaceID, projectID, snapshotID); err != nil {
		if runtimeWasActive {
			if recovered, recoveryErr := s.runtime.Start(ctx, runtimeReq); recoveryErr == nil && recovered != nil {
				_, _ = s.store.UpdateProjectRuntime(ctx, spaceID, projectID, recovered.Status, recovered.PreviewURL)
			}
		}
		return nil, err
	}
	if runtimeWasActive {
		freshRuntimeReq, runtimeReqErr := s.runtimeStartRequest(ctx, &RuntimeRequest{
			SpaceID:       spaceID,
			CurrentUserID: req.CurrentUserID,
			ProjectID:     projectID,
		})
		if runtimeReqErr != nil {
			_, _ = s.store.UpdateProjectRuntime(ctx, spaceID, projectID, domainappdev.RuntimeStatusError, "")
			return nil, fmt.Errorf("snapshot restored but runtime restart preparation failed: %w", runtimeReqErr)
		}
		restarted, restartErr := s.runtime.Start(ctx, freshRuntimeReq)
		if restartErr != nil {
			_, _ = s.store.UpdateProjectRuntime(ctx, spaceID, projectID, domainappdev.RuntimeStatusError, "")
			return nil, fmt.Errorf("snapshot restored but runtime restart failed: %w", restartErr)
		}
		if restarted != nil {
			_, _ = s.store.UpdateProjectRuntime(ctx, spaceID, projectID, restarted.Status, restarted.PreviewURL)
		}
	}
	return successFileMutationResponse(), nil
}

func (s *Service) stopRuntimeForSnapshotRestore(ctx context.Context, req *RestoreSnapshotRequest) (*RuntimeManagerRequest, bool, error) {
	if s.runtime == nil {
		return nil, false, nil
	}
	runtimeReq, err := s.runtimeRequest(ctx, &RuntimeRequest{
		SpaceID:       strings.TrimSpace(req.SpaceID),
		CurrentUserID: req.CurrentUserID,
		ProjectID:     strings.TrimSpace(req.ProjectID),
	})
	if err != nil {
		return nil, false, err
	}
	info, err := s.runtime.Status(ctx, runtimeReq)
	if err != nil {
		return nil, false, fmt.Errorf("check runtime before snapshot restore: %w", err)
	}
	if info == nil || !isActiveRuntimeStatus(info.Status) {
		return runtimeReq, false, nil
	}
	if _, err := s.runtime.Stop(ctx, runtimeReq); err != nil {
		return nil, false, fmt.Errorf("stop runtime before snapshot restore: %w", err)
	}
	return runtimeReq, true, nil
}

func isActiveRuntimeStatus(status domainappdev.RuntimeStatus) bool {
	switch status {
	case domainappdev.RuntimeStatusStarting,
		domainappdev.RuntimeStatusRunning,
		domainappdev.RuntimeStatusRestarting:
		return true
	default:
		return false
	}
}

func toSnapshotDTO(snapshot *domainappdev.ProjectSnapshot) *SnapshotDTO {
	if snapshot == nil {
		return nil
	}
	return &SnapshotDTO{
		ID:        snapshot.ID,
		Label:     snapshot.Label,
		CreatedAt: formatTime(snapshot.CreatedAt),
	}
}
