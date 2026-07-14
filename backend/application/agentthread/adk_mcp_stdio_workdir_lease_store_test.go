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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestApplicationADKMCPRuntimeStdioWorkdirLeaseStoreCreatesDurableLease(
	t *testing.T,
) {
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{}
	store := NewApplicationADKMCPRuntimeStdioWorkdirLeaseStore(
		ApplicationADKMCPRuntimeStdioWorkdirLeaseStoreOptions{
			Repository:     repo,
			IDGen:          &mcpWorkdirLeaseSequenceIDGen{next: 7001},
			WorkerID:       "worker-a",
			LeaseTTLMillis: 90000,
			NowMillis:      func() int64 { return 1000 },
		},
	)
	execution := mustProjectValidADKMCPRuntimeStdioExecution(t)
	prepared := ADKMCPRuntimeStdioPreparedWorkdir{
		Root:       "/mnt/coze/mcp",
		WorkingDir: "/mnt/coze/mcp/spaces/30/threads/10/runs/20/servers/100/tools/mcp_100_search_docs",
	}

	lease, err := store.CreateADKMCPRuntimeStdioWorkdirLease(
		context.Background(),
		execution,
		prepared,
	)

	require.NoError(t, err)
	require.Equal(t, int64(7001), lease.LeaseID)
	require.Equal(t, "worker-a", lease.WorkerID)
	require.Equal(t, 1, repo.createCalls)
	require.NotNil(t, repo.createdLease)
	require.Equal(t, int64(7001), repo.createdLease.ID)
	require.Equal(t, int64(30), repo.createdLease.SpaceID)
	require.Equal(t, int64(10), repo.createdLease.ThreadID)
	require.Equal(t, int64(20), repo.createdLease.RunID)
	require.Equal(t, int64(100), repo.createdLease.ServerID)
	require.Equal(t, "mcp_100_search_docs", repo.createdLease.RuntimeToolName)
	require.Equal(t, prepared.WorkingDir, repo.createdLease.Workdir)
	require.Equal(t, domainentity.MCPRuntimeWorkdirLeaseStatusActive, repo.createdLease.Status)
	require.Equal(t, "worker-a", repo.createdLease.WorkerID)
	require.Equal(t, int64(91000), repo.createdLease.LeaseExpiresAt)
	require.Equal(t, int64(1000), repo.createdLease.CreatedAt)
	require.Equal(t, int64(1000), repo.createdLease.UpdatedAt)
}

func TestApplicationADKMCPRuntimeStdioWorkdirLeaseStoreFinishesLease(
	t *testing.T,
) {
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		finishedLease: &domainentity.MCPRuntimeWorkdirLease{
			ID:     7001,
			Status: domainentity.MCPRuntimeWorkdirLeaseStatusReleased,
		},
		finishOK: true,
	}
	store := NewApplicationADKMCPRuntimeStdioWorkdirLeaseStore(
		ApplicationADKMCPRuntimeStdioWorkdirLeaseStoreOptions{
			Repository: repo,
			IDGen:      &mcpWorkdirLeaseSequenceIDGen{next: 7001},
			WorkerID:   "worker-a",
			NowMillis:  func() int64 { return 2000 },
		},
	)

	err := store.FinishADKMCPRuntimeStdioWorkdirLease(
		context.Background(),
		ADKMCPRuntimeStdioWorkdirLease{
			LeaseID:  7001,
			WorkerID: "worker-a",
		},
		ADKMCPRuntimeStdioWorkdirLeaseStatusReleased,
		"",
	)

	require.NoError(t, err)
	require.Equal(t, 1, repo.finishCalls)
	require.Equal(t, int64(7001), repo.finishReq.LeaseID)
	require.Equal(t, "worker-a", repo.finishReq.WorkerID)
	require.Equal(t, domainentity.MCPRuntimeWorkdirLeaseStatusReleased, repo.finishReq.Status)
	require.Equal(t, int64(2000), repo.finishReq.Now)
	require.Empty(t, repo.finishReq.LastError)

	err = store.FinishADKMCPRuntimeStdioWorkdirLease(
		context.Background(),
		ADKMCPRuntimeStdioWorkdirLease{
			LeaseID:  7001,
			WorkerID: "worker-a",
		},
		ADKMCPRuntimeStdioWorkdirLeaseStatusFailed,
		"cleanup failed",
	)

	require.NoError(t, err)
	require.Equal(t, 2, repo.finishCalls)
	require.Equal(t, domainentity.MCPRuntimeWorkdirLeaseStatusFailed, repo.finishReq.Status)
	require.Equal(t, "cleanup failed", repo.finishReq.LastError)
}

func TestApplicationADKMCPRuntimeStdioWorkdirLeaseStoreSanitizesErrors(
	t *testing.T,
) {
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		createErr: fmt.Errorf(
			`insert /mnt/coze/mcp/spaces/30 with stdio-secret-token`,
		),
	}
	store := NewApplicationADKMCPRuntimeStdioWorkdirLeaseStore(
		ApplicationADKMCPRuntimeStdioWorkdirLeaseStoreOptions{
			Repository: repo,
			IDGen:      &mcpWorkdirLeaseSequenceIDGen{next: 7001},
			WorkerID:   "worker-a",
			NowMillis:  func() int64 { return 1000 },
		},
	)

	lease, err := store.CreateADKMCPRuntimeStdioWorkdirLease(
		context.Background(),
		mustProjectValidADKMCPRuntimeStdioExecution(t),
		ADKMCPRuntimeStdioPreparedWorkdir{
			Root:       "/mnt/coze/mcp",
			WorkingDir: "/mnt/coze/mcp/spaces/30/threads/10/runs/20/servers/100/tools/mcp_100_search_docs",
		},
	)

	require.Error(t, err)
	require.Empty(t, lease)
	require.Contains(t, err.Error(), "mcp runtime stdio workdir lease create failed")
	assertADKMCPStdioLeaseStoreErrorDoesNotLeak(t, err.Error())
}

type recordingMCPRuntimeWorkdirLeaseRepository struct {
	createdLease  *domainentity.MCPRuntimeWorkdirLease
	finishedLease *domainentity.MCPRuntimeWorkdirLease
	currentLease  *domainentity.MCPRuntimeWorkdirLease
	expiredLeases []*domainentity.MCPRuntimeWorkdirLease
	finishReq     domainrepo.FinishMCPRuntimeWorkdirLeaseRequest
	listReq       domainrepo.ListExpiredMCPRuntimeWorkdirLeasesRequest
	claimReq      domainrepo.ClaimExpiredMCPRuntimeWorkdirLeaseRequest
	retryReq      domainrepo.RetryMCPRuntimeWorkdirLeaseRequest
	createCalls   int
	finishCalls   int
	listCalls     int
	getCalls      int
	claimCalls    int
	retryCalls    int
	finishOK      bool
	createErr     error
	finishErr     error
	listErr       error
}

func (r *recordingMCPRuntimeWorkdirLeaseRepository) CreateMCPRuntimeWorkdirLease(
	ctx context.Context,
	lease *domainentity.MCPRuntimeWorkdirLease,
) error {
	r.createCalls++
	if lease != nil {
		cloned := *lease
		r.createdLease = &cloned
	}
	return r.createErr
}

func (r *recordingMCPRuntimeWorkdirLeaseRepository) GetMCPRuntimeWorkdirLease(
	ctx context.Context,
	leaseID int64,
) (*domainentity.MCPRuntimeWorkdirLease, error) {
	r.getCalls++
	if r.currentLease != nil && r.currentLease.ID == leaseID {
		cloned := *r.currentLease
		return &cloned, nil
	}
	for _, lease := range r.expiredLeases {
		if lease != nil && lease.ID == leaseID {
			cloned := *lease
			return &cloned, nil
		}
	}
	return nil, nil
}

func (r *recordingMCPRuntimeWorkdirLeaseRepository) ClaimExpiredMCPRuntimeWorkdirLease(
	_ context.Context,
	req domainrepo.ClaimExpiredMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.claimCalls++
	r.claimReq = req
	var lease *domainentity.MCPRuntimeWorkdirLease
	if r.currentLease != nil && r.currentLease.ID == req.LeaseID {
		lease = r.currentLease
	} else {
		for _, item := range r.expiredLeases {
			if item != nil && item.ID == req.LeaseID {
				lease = item
				break
			}
		}
	}
	if lease == nil || lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
		lease.WorkerID != req.ExpectedWorkerID || lease.Workdir != req.ExpectedWorkdir ||
		lease.LeaseExpiresAt != req.ExpectedLeaseExpiresAt || lease.LeaseExpiresAt > req.Now {
		return nil, false, nil
	}
	claimed := *lease
	claimed.WorkerID = req.ClaimWorkerID
	claimed.LeaseExpiresAt = req.ClaimExpiresAt
	claimed.UpdatedAt = req.Now
	r.currentLease = &claimed
	result := claimed
	return &result, true, nil
}

func (r *recordingMCPRuntimeWorkdirLeaseRepository) RetryMCPRuntimeWorkdirLease(
	_ context.Context,
	req domainrepo.RetryMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.retryCalls++
	r.retryReq = req
	if r.currentLease == nil || r.currentLease.ID != req.LeaseID ||
		r.currentLease.WorkerID != req.WorkerID ||
		r.currentLease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive {
		return nil, false, nil
	}
	retried := *r.currentLease
	retried.LeaseExpiresAt = req.RetryAt
	retried.UpdatedAt = req.Now
	retried.LastError = req.LastError
	r.currentLease = &retried
	result := retried
	return &result, true, nil
}

func (r *recordingMCPRuntimeWorkdirLeaseRepository) FinishMCPRuntimeWorkdirLease(
	ctx context.Context,
	req domainrepo.FinishMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.finishCalls++
	r.finishReq = req
	return r.finishedLease, r.finishOK, r.finishErr
}

func (r *recordingMCPRuntimeWorkdirLeaseRepository) ListExpiredMCPRuntimeWorkdirLeases(
	ctx context.Context,
	req domainrepo.ListExpiredMCPRuntimeWorkdirLeasesRequest,
) ([]*domainentity.MCPRuntimeWorkdirLease, error) {
	r.listCalls++
	r.listReq = req
	if r.listErr != nil {
		return nil, r.listErr
	}
	result := make([]*domainentity.MCPRuntimeWorkdirLease, 0, len(r.expiredLeases))
	for _, lease := range r.expiredLeases {
		if lease == nil {
			result = append(result, nil)
			continue
		}
		cloned := *lease
		result = append(result, &cloned)
	}
	return result, nil
}

type mcpWorkdirLeaseSequenceIDGen struct {
	next int64
}

func (g *mcpWorkdirLeaseSequenceIDGen) GenID(context.Context) (int64, error) {
	value := g.next
	g.next++
	return value, nil
}

func (g *mcpWorkdirLeaseSequenceIDGen) GenMultiIDs(
	ctx context.Context,
	counts int,
) ([]int64, error) {
	result := make([]int64, counts)
	for index := range result {
		value, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		result[index] = value
	}
	return result, nil
}

func assertADKMCPStdioLeaseStoreErrorDoesNotLeak(t *testing.T, text string) {
	t.Helper()
	assertADKMCPStdioWorkdirPreparerErrorDoesNotLeak(t, text)
	require.NotContains(t, text, "/mnt/coze/mcp")
	require.NotContains(t, text, "stdio-secret-token")
}
