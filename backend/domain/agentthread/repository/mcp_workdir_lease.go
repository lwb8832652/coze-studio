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

package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type MCPRuntimeWorkdirLeaseRepository interface {
	CreateMCPRuntimeWorkdirLease(
		ctx context.Context,
		lease *entity.MCPRuntimeWorkdirLease,
	) error
	GetMCPRuntimeWorkdirLease(
		ctx context.Context,
		leaseID int64,
	) (*entity.MCPRuntimeWorkdirLease, error)
	FinishMCPRuntimeWorkdirLease(
		ctx context.Context,
		req FinishMCPRuntimeWorkdirLeaseRequest,
	) (*entity.MCPRuntimeWorkdirLease, bool, error)
	ClaimExpiredMCPRuntimeWorkdirLease(
		ctx context.Context,
		req ClaimExpiredMCPRuntimeWorkdirLeaseRequest,
	) (*entity.MCPRuntimeWorkdirLease, bool, error)
	RetryMCPRuntimeWorkdirLease(
		ctx context.Context,
		req RetryMCPRuntimeWorkdirLeaseRequest,
	) (*entity.MCPRuntimeWorkdirLease, bool, error)
	ListExpiredMCPRuntimeWorkdirLeases(
		ctx context.Context,
		req ListExpiredMCPRuntimeWorkdirLeasesRequest,
	) ([]*entity.MCPRuntimeWorkdirLease, error)
}

type FinishMCPRuntimeWorkdirLeaseRequest struct {
	LeaseID   int64
	WorkerID  string
	Status    entity.MCPRuntimeWorkdirLeaseStatus
	Now       int64
	LastError string
}

type ClaimExpiredMCPRuntimeWorkdirLeaseRequest struct {
	LeaseID                int64
	ExpectedWorkerID       string
	ExpectedWorkdir        string
	ExpectedLeaseExpiresAt int64
	ClaimWorkerID          string
	Now                    int64
	ClaimExpiresAt         int64
}

type RetryMCPRuntimeWorkdirLeaseRequest struct {
	LeaseID   int64
	WorkerID  string
	Now       int64
	RetryAt   int64
	LastError string
}

type ListExpiredMCPRuntimeWorkdirLeasesRequest struct {
	Now   int64
	Limit int32
}

type mcpRuntimeWorkdirLeasePO struct {
	ID              int64  `gorm:"column:id;primaryKey"`
	SpaceID         int64  `gorm:"column:space_id;index:idx_agent_mcp_stdio_workdir_leases_space_status,priority:1"`
	ThreadID        int64  `gorm:"column:thread_id;index:idx_agent_mcp_stdio_workdir_leases_thread_created,priority:1"`
	RunID           int64  `gorm:"column:run_id;index:idx_agent_mcp_stdio_workdir_leases_run_created,priority:1"`
	ServerID        int64  `gorm:"column:server_id;index:idx_agent_mcp_stdio_workdir_leases_server"`
	RuntimeToolName string `gorm:"column:runtime_tool_name"`
	Workdir         string `gorm:"column:workdir"`
	Status          string `gorm:"column:status;index:idx_agent_mcp_stdio_workdir_leases_space_status,priority:2;index:idx_agent_mcp_stdio_workdir_leases_status_expires,priority:1;index:idx_agent_mcp_stdio_workdir_leases_worker_status,priority:2"`
	WorkerID        string `gorm:"column:worker_id;index:idx_agent_mcp_stdio_workdir_leases_worker_status,priority:1"`
	LeaseExpiresAt  int64  `gorm:"column:lease_expires_at;index:idx_agent_mcp_stdio_workdir_leases_status_expires,priority:2"`
	ReleasedAt      int64  `gorm:"column:released_at"`
	LastError       string `gorm:"column:last_error"`
	CreatedAt       int64  `gorm:"column:created_at;index:idx_agent_mcp_stdio_workdir_leases_thread_created,priority:2;index:idx_agent_mcp_stdio_workdir_leases_run_created,priority:2;index:idx_agent_mcp_stdio_workdir_leases_status_expires,priority:3"`
	UpdatedAt       int64  `gorm:"column:updated_at;index:idx_agent_mcp_stdio_workdir_leases_space_status,priority:3"`
}

func (mcpRuntimeWorkdirLeasePO) TableName() string {
	return "agent_mcp_stdio_workdir_leases"
}

func NewMCPRuntimeWorkdirLeaseRepository(
	db *gorm.DB,
) MCPRuntimeWorkdirLeaseRepository {
	return &threadRepository{db: db}
}

func (r *threadRepository) CreateMCPRuntimeWorkdirLease(
	ctx context.Context,
	lease *entity.MCPRuntimeWorkdirLease,
) error {
	if lease == nil {
		return errors.New("mcp runtime workdir lease is required")
	}

	return r.db.WithContext(ctx).Create(newMCPRuntimeWorkdirLeasePO(lease)).Error
}

func (r *threadRepository) GetMCPRuntimeWorkdirLease(
	ctx context.Context,
	leaseID int64,
) (*entity.MCPRuntimeWorkdirLease, error) {
	var po mcpRuntimeWorkdirLeasePO
	err := r.db.WithContext(ctx).
		Where("id = ?", leaseID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return po.toEntity(), nil
}

func (r *threadRepository) FinishMCPRuntimeWorkdirLease(
	ctx context.Context,
	req FinishMCPRuntimeWorkdirLeaseRequest,
) (*entity.MCPRuntimeWorkdirLease, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	status := req.Status
	if status != entity.MCPRuntimeWorkdirLeaseStatusReleased &&
		status != entity.MCPRuntimeWorkdirLeaseStatusFailed {
		return nil, false, errors.New("mcp runtime workdir lease status is invalid")
	}

	query := r.db.WithContext(ctx).
		Model(&mcpRuntimeWorkdirLeasePO{}).
		Where("id = ?", req.LeaseID).
		Where("status = ?", string(entity.MCPRuntimeWorkdirLeaseStatusActive))
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID != "" {
		query = query.Where("worker_id = ?", workerID)
	}
	updateResult := query.Updates(map[string]any{
		"status":           string(status),
		"lease_expires_at": 0,
		"released_at":      now,
		"last_error":       truncateMCPRuntimeWorkdirLeaseError(req.LastError),
		"updated_at":       now,
	})
	if updateResult.Error != nil {
		return nil, false, updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		return nil, false, nil
	}

	var po mcpRuntimeWorkdirLeasePO
	if err := r.db.WithContext(ctx).
		Where("id = ?", req.LeaseID).
		First(&po).Error; err != nil {
		return nil, false, err
	}

	return po.toEntity(), true, nil
}

func (r *threadRepository) ClaimExpiredMCPRuntimeWorkdirLease(
	ctx context.Context,
	req ClaimExpiredMCPRuntimeWorkdirLeaseRequest,
) (*entity.MCPRuntimeWorkdirLease, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	expectedWorkerID := strings.TrimSpace(req.ExpectedWorkerID)
	expectedWorkdir := strings.TrimSpace(req.ExpectedWorkdir)
	claimWorkerID := strings.TrimSpace(req.ClaimWorkerID)
	if req.LeaseID <= 0 || expectedWorkerID == "" || expectedWorkdir == "" ||
		claimWorkerID == "" || len(claimWorkerID) > 128 ||
		req.ExpectedLeaseExpiresAt <= 0 || req.ExpectedLeaseExpiresAt > now ||
		req.ClaimExpiresAt <= now {
		return nil, false, errors.New("mcp runtime workdir lease claim is invalid")
	}
	updated := r.db.WithContext(ctx).
		Model(&mcpRuntimeWorkdirLeasePO{}).
		Where("id = ?", req.LeaseID).
		Where("status = ?", string(entity.MCPRuntimeWorkdirLeaseStatusActive)).
		Where("worker_id = ?", expectedWorkerID).
		Where("workdir = ?", expectedWorkdir).
		Where("lease_expires_at = ? AND lease_expires_at <= ?", req.ExpectedLeaseExpiresAt, now).
		Updates(map[string]any{
			"worker_id":        claimWorkerID,
			"lease_expires_at": req.ClaimExpiresAt,
			"last_error":       "",
			"updated_at":       now,
		})
	if updated.Error != nil {
		return nil, false, updated.Error
	}
	if updated.RowsAffected == 0 {
		return nil, false, nil
	}
	lease, err := r.GetMCPRuntimeWorkdirLease(ctx, req.LeaseID)
	if err != nil {
		return nil, false, err
	}
	return lease, lease != nil, nil
}

func (r *threadRepository) RetryMCPRuntimeWorkdirLease(
	ctx context.Context,
	req RetryMCPRuntimeWorkdirLeaseRequest,
) (*entity.MCPRuntimeWorkdirLease, bool, error) {
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if req.LeaseID <= 0 || workerID == "" || req.RetryAt <= now {
		return nil, false, errors.New("mcp runtime workdir lease retry is invalid")
	}
	updated := r.db.WithContext(ctx).
		Model(&mcpRuntimeWorkdirLeasePO{}).
		Where("id = ?", req.LeaseID).
		Where("status = ?", string(entity.MCPRuntimeWorkdirLeaseStatusActive)).
		Where("worker_id = ?", workerID).
		Updates(map[string]any{
			"lease_expires_at": req.RetryAt,
			"released_at":      0,
			"last_error":       truncateMCPRuntimeWorkdirLeaseError(req.LastError),
			"updated_at":       now,
		})
	if updated.Error != nil {
		return nil, false, updated.Error
	}
	if updated.RowsAffected == 0 {
		return nil, false, nil
	}
	lease, err := r.GetMCPRuntimeWorkdirLease(ctx, req.LeaseID)
	if err != nil {
		return nil, false, err
	}
	return lease, lease != nil, nil
}

func (r *threadRepository) ListExpiredMCPRuntimeWorkdirLeases(
	ctx context.Context,
	req ListExpiredMCPRuntimeWorkdirLeasesRequest,
) ([]*entity.MCPRuntimeWorkdirLease, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}

	pos := make([]*mcpRuntimeWorkdirLeasePO, 0, limit)
	if err := r.db.WithContext(ctx).
		Where("status = ?", string(entity.MCPRuntimeWorkdirLeaseStatusActive)).
		Where("lease_expires_at > 0 AND lease_expires_at <= ?", now).
		Order("lease_expires_at ASC, created_at ASC, id ASC").
		Limit(int(limit)).
		Find(&pos).Error; err != nil {
		return nil, err
	}

	leases := make([]*entity.MCPRuntimeWorkdirLease, 0, len(pos))
	for _, po := range pos {
		leases = append(leases, po.toEntity())
	}

	return leases, nil
}

func newMCPRuntimeWorkdirLeasePO(
	lease *entity.MCPRuntimeWorkdirLease,
) *mcpRuntimeWorkdirLeasePO {
	return &mcpRuntimeWorkdirLeasePO{
		ID:              lease.ID,
		SpaceID:         lease.SpaceID,
		ThreadID:        lease.ThreadID,
		RunID:           lease.RunID,
		ServerID:        lease.ServerID,
		RuntimeToolName: strings.TrimSpace(lease.RuntimeToolName),
		Workdir:         strings.TrimSpace(lease.Workdir),
		Status:          string(lease.Status),
		WorkerID:        strings.TrimSpace(lease.WorkerID),
		LeaseExpiresAt:  lease.LeaseExpiresAt,
		ReleasedAt:      lease.ReleasedAt,
		LastError:       truncateMCPRuntimeWorkdirLeaseError(lease.LastError),
		CreatedAt:       lease.CreatedAt,
		UpdatedAt:       lease.UpdatedAt,
	}
}

func (po *mcpRuntimeWorkdirLeasePO) toEntity() *entity.MCPRuntimeWorkdirLease {
	if po == nil {
		return nil
	}

	return &entity.MCPRuntimeWorkdirLease{
		ID:              po.ID,
		SpaceID:         po.SpaceID,
		ThreadID:        po.ThreadID,
		RunID:           po.RunID,
		ServerID:        po.ServerID,
		RuntimeToolName: po.RuntimeToolName,
		Workdir:         po.Workdir,
		Status:          entity.MCPRuntimeWorkdirLeaseStatus(po.Status),
		WorkerID:        po.WorkerID,
		LeaseExpiresAt:  po.LeaseExpiresAt,
		ReleasedAt:      po.ReleasedAt,
		LastError:       po.LastError,
		CreatedAt:       po.CreatedAt,
		UpdatedAt:       po.UpdatedAt,
	}
}

func truncateMCPRuntimeWorkdirLeaseError(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) > 512 {
		runes = runes[:512]
	}

	return string(runes)
}
