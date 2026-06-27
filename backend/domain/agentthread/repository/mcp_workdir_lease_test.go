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
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestMCPRuntimeWorkdirLeaseRepositoryCreatesAndGetsLease(t *testing.T) {
	repo := newTestMCPRuntimeWorkdirLeaseRepository(t)
	lease := sampleMCPRuntimeWorkdirLease(4001)

	err := repo.CreateMCPRuntimeWorkdirLease(context.Background(), lease)

	require.NoError(t, err)
	got, err := repo.GetMCPRuntimeWorkdirLease(context.Background(), 4001)
	require.NoError(t, err)
	require.Equal(t, lease, got)
}

func TestMCPRuntimeWorkdirLeaseRepositoryFinishesActiveLease(t *testing.T) {
	repo := newTestMCPRuntimeWorkdirLeaseRepository(t)
	require.NoError(t, repo.CreateMCPRuntimeWorkdirLease(
		context.Background(),
		sampleMCPRuntimeWorkdirLease(4001),
	))

	finished, ok, err := repo.FinishMCPRuntimeWorkdirLease(
		context.Background(),
		FinishMCPRuntimeWorkdirLeaseRequest{
			LeaseID:   4001,
			WorkerID:  "worker-a",
			Status:    entity.MCPRuntimeWorkdirLeaseStatusReleased,
			Now:       300,
			LastError: "cleanup failed",
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.MCPRuntimeWorkdirLeaseStatusReleased, finished.Status)
	require.Equal(t, int64(300), finished.ReleasedAt)
	require.Equal(t, int64(300), finished.UpdatedAt)
	require.Equal(t, "cleanup failed", finished.LastError)

	finished, ok, err = repo.FinishMCPRuntimeWorkdirLease(
		context.Background(),
		FinishMCPRuntimeWorkdirLeaseRequest{
			LeaseID:  4001,
			WorkerID: "worker-a",
			Status:   entity.MCPRuntimeWorkdirLeaseStatusFailed,
			Now:      400,
		},
	)

	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, finished)
}

func TestMCPRuntimeWorkdirLeaseRepositoryRequiresMatchingWorkerWhenProvided(
	t *testing.T,
) {
	repo := newTestMCPRuntimeWorkdirLeaseRepository(t)
	require.NoError(t, repo.CreateMCPRuntimeWorkdirLease(
		context.Background(),
		sampleMCPRuntimeWorkdirLease(4001),
	))

	finished, ok, err := repo.FinishMCPRuntimeWorkdirLease(
		context.Background(),
		FinishMCPRuntimeWorkdirLeaseRequest{
			LeaseID:  4001,
			WorkerID: "worker-b",
			Status:   entity.MCPRuntimeWorkdirLeaseStatusReleased,
			Now:      300,
		},
	)

	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, finished)
	got, err := repo.GetMCPRuntimeWorkdirLease(context.Background(), 4001)
	require.NoError(t, err)
	require.Equal(t, entity.MCPRuntimeWorkdirLeaseStatusActive, got.Status)
	require.Equal(t, "worker-a", got.WorkerID)
}

func TestMCPRuntimeWorkdirLeaseRepositoryListsExpiredActiveLeases(
	t *testing.T,
) {
	repo := newTestMCPRuntimeWorkdirLeaseRepository(t)
	for _, lease := range []*entity.MCPRuntimeWorkdirLease{
		sampleMCPRuntimeWorkdirLease(4001),
		sampleMCPRuntimeWorkdirLease(4002),
		sampleMCPRuntimeWorkdirLease(4003),
		sampleMCPRuntimeWorkdirLease(4004),
	} {
		switch lease.ID {
		case 4001:
			lease.LeaseExpiresAt = 100
			lease.CreatedAt = 100
		case 4002:
			lease.LeaseExpiresAt = 120
			lease.CreatedAt = 90
		case 4003:
			lease.LeaseExpiresAt = 400
		case 4004:
			lease.Status = entity.MCPRuntimeWorkdirLeaseStatusReleased
			lease.LeaseExpiresAt = 80
			lease.ReleasedAt = 110
		}
		require.NoError(t, repo.CreateMCPRuntimeWorkdirLease(
			context.Background(),
			lease,
		))
	}

	expired, err := repo.ListExpiredMCPRuntimeWorkdirLeases(
		context.Background(),
		ListExpiredMCPRuntimeWorkdirLeasesRequest{
			Now:   200,
			Limit: 10,
		},
	)

	require.NoError(t, err)
	require.Len(t, expired, 2)
	require.Equal(t, int64(4001), expired[0].ID)
	require.Equal(t, int64(4002), expired[1].ID)
	for _, lease := range expired {
		require.Equal(t, entity.MCPRuntimeWorkdirLeaseStatusActive, lease.Status)
		require.LessOrEqual(t, lease.LeaseExpiresAt, int64(200))
	}
}

func newTestMCPRuntimeWorkdirLeaseRepository(
	t *testing.T,
) MCPRuntimeWorkdirLeaseRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpRuntimeWorkdirLeasePO{}))

	return NewMCPRuntimeWorkdirLeaseRepository(db)
}

func sampleMCPRuntimeWorkdirLease(id int64) *entity.MCPRuntimeWorkdirLease {
	return &entity.MCPRuntimeWorkdirLease{
		ID:              id,
		SpaceID:         30,
		ThreadID:        10,
		RunID:           20,
		ServerID:        100,
		RuntimeToolName: "mcp_100_search_docs",
		Workdir:         "/mnt/coze/mcp/spaces/30/threads/10/runs/20/servers/100/tools/mcp_100_search_docs",
		Status:          entity.MCPRuntimeWorkdirLeaseStatusActive,
		WorkerID:        "worker-a",
		LeaseExpiresAt:  200,
		CreatedAt:       100,
		UpdatedAt:       100,
	}
}
