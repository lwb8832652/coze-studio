// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestADKMCPRuntimeStdioLeasedCleanupFailureSchedulesRetryWithoutFinish(t *testing.T) {
	inner := &recordingADKMCPRuntimeStdioWorkdirPreparer{cleanupErr: errors.New("transient")}
	store := &recordingRetryADKMCPWorkdirLeaseStore{
		lease: ADKMCPRuntimeStdioWorkdirLease{LeaseID: 9001, WorkerID: "worker-a"},
	}
	preparer := NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
		ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions{
			Inner: inner, LeaseStore: store,
			RetryDelay: time.Minute,
			NowMillis:  func() int64 { return 1000 },
		},
	)
	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
		context.Background(), mustProjectValidADKMCPRuntimeStdioExecution(t),
	)
	require.NoError(t, err)

	err = preparer.CleanupADKMCPRuntimeStdioWorkdir(context.Background(), prepared)

	require.Error(t, err)
	require.Equal(t, 0, store.finishCalls)
	require.Equal(t, 1, store.retryCalls)
	require.Equal(t, int64(61_000), store.retryAt)
	require.Equal(t, "cleanup retryable", store.errorText)
}

func TestADKMCPRuntimeStdioUnconfirmedTerminationKeepsLeaseAndWorkdir(t *testing.T) {
	inner := &recordingADKMCPRuntimeStdioWorkdirPreparer{}
	store := &recordingRetryADKMCPWorkdirLeaseStore{
		lease: ADKMCPRuntimeStdioWorkdirLease{LeaseID: 9002, WorkerID: "worker-a"},
	}
	preparer := NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
		ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions{
			Inner: inner, LeaseStore: store, RetryDelay: time.Minute,
			NowMillis: func() int64 { return 1000 },
		},
	)
	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
		context.Background(), mustProjectValidADKMCPRuntimeStdioExecution(t),
	)
	require.NoError(t, err)

	err = preparer.RetryADKMCPRuntimeStdioWorkdirCleanup(
		context.Background(), prepared, "private process detail",
	)

	require.NoError(t, err)
	require.Zero(t, inner.cleanupCalls)
	require.Zero(t, store.finishCalls)
	require.Equal(t, 1, store.retryCalls)
	require.Equal(t, "process termination unconfirmed", store.errorText)
}

func TestADKMCPRuntimeStdioWorkdirReaperRetriesTransientFailureAfterBackoff(t *testing.T) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	now := int64(1000)
	repo := newClaimingWorkdirLeaseRepository(&domainentity.MCPRuntimeWorkdirLease{
		ID: 9101, Workdir: filepath.Join(root, "invocation-retry"),
		Status:   domainentity.MCPRuntimeWorkdirLeaseStatusActive,
		WorkerID: "worker-a", LeaseExpiresAt: 500,
	})
	preparer := &transientWorkdirPreparer{failures: 1}
	reaper := NewADKMCPRuntimeStdioWorkdirLeaseReaper(
		ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
			Repository: repo, Root: root, WorkdirPreparer: preparer,
			RetryDelay: time.Minute, ClaimTTL: time.Minute,
			NowMillis: func() int64 { return now },
		},
	)

	first, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, first.Retried)
	require.Zero(t, first.Finished)
	require.Equal(t, domainentity.MCPRuntimeWorkdirLeaseStatusActive, repo.snapshot().Status)
	retryAt := repo.snapshot().LeaseExpiresAt
	require.Greater(t, retryAt, now)

	second, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(context.Background())
	require.NoError(t, err)
	require.Zero(t, second.Listed)
	now = retryAt
	third, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, third.Cleaned)
	require.Equal(t, 1, third.Finished)
	require.Equal(t, int32(2), preparer.calls.Load())
}

func TestADKMCPRuntimeStdioWorkdirReaperConcurrentInstancesClaimOnce(t *testing.T) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	repo := newClaimingWorkdirLeaseRepository(&domainentity.MCPRuntimeWorkdirLease{
		ID: 9201, Workdir: filepath.Join(root, "invocation-shared"),
		Status:   domainentity.MCPRuntimeWorkdirLeaseStatusActive,
		WorkerID: "worker-a", LeaseExpiresAt: 500,
	})
	preparer := &transientWorkdirPreparer{}
	newReaper := func() *ADKMCPRuntimeStdioWorkdirLeaseReaper {
		return NewADKMCPRuntimeStdioWorkdirLeaseReaper(
			ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
				Repository: repo, Root: root, WorkdirPreparer: preparer,
				NowMillis: func() int64 { return 1000 },
			},
		)
	}
	var wg sync.WaitGroup
	for _, reaper := range []*ADKMCPRuntimeStdioWorkdirLeaseReaper{newReaper(), newReaper()} {
		wg.Add(1)
		go func(item *ADKMCPRuntimeStdioWorkdirLeaseReaper) {
			defer wg.Done()
			_, _ = item.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(context.Background())
		}(reaper)
	}
	wg.Wait()
	require.Equal(t, int32(1), preparer.calls.Load())
	require.Equal(t, 1, repo.finishCalls)
}

type recordingRetryADKMCPWorkdirLeaseStore struct {
	lease       ADKMCPRuntimeStdioWorkdirLease
	finishCalls int
	retryCalls  int
	retryAt     int64
	errorText   string
}

func (s *recordingRetryADKMCPWorkdirLeaseStore) CreateADKMCPRuntimeStdioWorkdirLease(
	context.Context,
	ADKMCPRuntimeStdioSandboxExecution,
	ADKMCPRuntimeStdioPreparedWorkdir,
) (ADKMCPRuntimeStdioWorkdirLease, error) {
	return s.lease, nil
}

func (s *recordingRetryADKMCPWorkdirLeaseStore) FinishADKMCPRuntimeStdioWorkdirLease(
	context.Context,
	ADKMCPRuntimeStdioWorkdirLease,
	ADKMCPRuntimeStdioWorkdirLeaseStatus,
	string,
) error {
	s.finishCalls++
	return nil
}

func (s *recordingRetryADKMCPWorkdirLeaseStore) RetryADKMCPRuntimeStdioWorkdirLease(
	_ context.Context,
	_ ADKMCPRuntimeStdioWorkdirLease,
	retryAt int64,
	errorText string,
) error {
	s.retryCalls++
	s.retryAt = retryAt
	s.errorText = errorText
	return nil
}

type transientWorkdirPreparer struct {
	calls    atomic.Int32
	failures int32
}

func (p *transientWorkdirPreparer) PrepareADKMCPRuntimeStdioWorkdir(
	context.Context,
	ADKMCPRuntimeStdioSandboxExecution,
) (ADKMCPRuntimeStdioPreparedWorkdir, error) {
	return ADKMCPRuntimeStdioPreparedWorkdir{}, nil
}

func (p *transientWorkdirPreparer) CleanupADKMCPRuntimeStdioWorkdir(
	context.Context,
	ADKMCPRuntimeStdioPreparedWorkdir,
) error {
	call := p.calls.Add(1)
	if call <= p.failures {
		return errors.New("transient")
	}
	return nil
}

type claimingWorkdirLeaseRepository struct {
	mu          sync.Mutex
	lease       *domainentity.MCPRuntimeWorkdirLease
	finishCalls int
}

func newClaimingWorkdirLeaseRepository(
	lease *domainentity.MCPRuntimeWorkdirLease,
) *claimingWorkdirLeaseRepository {
	return &claimingWorkdirLeaseRepository{lease: cloneMCPWorkdirLease(lease)}
}

func (r *claimingWorkdirLeaseRepository) snapshot() *domainentity.MCPRuntimeWorkdirLease {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneMCPWorkdirLease(r.lease)
}

func (r *claimingWorkdirLeaseRepository) CreateMCPRuntimeWorkdirLease(
	context.Context,
	*domainentity.MCPRuntimeWorkdirLease,
) error {
	return nil
}

func (r *claimingWorkdirLeaseRepository) GetMCPRuntimeWorkdirLease(
	context.Context,
	int64,
) (*domainentity.MCPRuntimeWorkdirLease, error) {
	return r.snapshot(), nil
}

func (r *claimingWorkdirLeaseRepository) ListExpiredMCPRuntimeWorkdirLeases(
	_ context.Context,
	req domainrepo.ListExpiredMCPRuntimeWorkdirLeasesRequest,
) ([]*domainentity.MCPRuntimeWorkdirLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lease == nil || r.lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
		r.lease.LeaseExpiresAt <= 0 || r.lease.LeaseExpiresAt > req.Now {
		return nil, nil
	}
	return []*domainentity.MCPRuntimeWorkdirLease{cloneMCPWorkdirLease(r.lease)}, nil
}

func (r *claimingWorkdirLeaseRepository) ClaimExpiredMCPRuntimeWorkdirLease(
	_ context.Context,
	req domainrepo.ClaimExpiredMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lease == nil || r.lease.ID != req.LeaseID ||
		r.lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
		r.lease.WorkerID != req.ExpectedWorkerID || r.lease.Workdir != req.ExpectedWorkdir ||
		r.lease.LeaseExpiresAt != req.ExpectedLeaseExpiresAt || r.lease.LeaseExpiresAt > req.Now {
		return nil, false, nil
	}
	r.lease.WorkerID = req.ClaimWorkerID
	r.lease.LeaseExpiresAt = req.ClaimExpiresAt
	r.lease.UpdatedAt = req.Now
	return cloneMCPWorkdirLease(r.lease), true, nil
}

func (r *claimingWorkdirLeaseRepository) RetryMCPRuntimeWorkdirLease(
	_ context.Context,
	req domainrepo.RetryMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lease == nil || r.lease.ID != req.LeaseID || r.lease.WorkerID != req.WorkerID ||
		r.lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive {
		return nil, false, nil
	}
	r.lease.LeaseExpiresAt = req.RetryAt
	r.lease.UpdatedAt = req.Now
	r.lease.LastError = req.LastError
	return cloneMCPWorkdirLease(r.lease), true, nil
}

func (r *claimingWorkdirLeaseRepository) FinishMCPRuntimeWorkdirLease(
	_ context.Context,
	req domainrepo.FinishMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lease == nil || r.lease.ID != req.LeaseID || r.lease.WorkerID != req.WorkerID ||
		r.lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive {
		return nil, false, nil
	}
	r.finishCalls++
	r.lease.Status = req.Status
	r.lease.LeaseExpiresAt = 0
	return cloneMCPWorkdirLease(r.lease), true, nil
}
