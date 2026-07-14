// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestADKMCPRuntimeStdioWorkdirConcurrentInvocationsUseUniqueDirectories(t *testing.T) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	preparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root},
	)
	execution := validADKMCPRuntimeStdioSandboxExecution(root)
	execution.WorkingDir = filepath.Join(filepath.Dir(root), "persisted-cwd-must-be-ignored")

	prepared := make([]ADKMCPRuntimeStdioPreparedWorkdir, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for index := range prepared {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			prepared[index], errs[index] = preparer.PrepareADKMCPRuntimeStdioWorkdir(
				context.Background(), execution,
			)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.NotEqual(t, prepared[0].WorkingDir, prepared[1].WorkingDir)
	for _, item := range prepared {
		require.True(t, adkMCPRuntimePathWithin(item.WorkingDir, root))
		require.True(t, strings.HasPrefix(filepath.Base(item.WorkingDir), "invocation-"))
		info, err := os.Stat(item.WorkingDir)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
	require.NoError(t, os.WriteFile(filepath.Join(prepared[1].WorkingDir, "active"), []byte("active"), 0o600))
	require.NoError(t, preparer.CleanupADKMCPRuntimeStdioWorkdir(context.Background(), prepared[0]))
	_, err := os.Stat(filepath.Join(prepared[1].WorkingDir, "active"))
	require.NoError(t, err)
	require.NoError(t, preparer.CleanupADKMCPRuntimeStdioWorkdir(context.Background(), prepared[1]))
}

func TestADKMCPRuntimeStdioWorkdirRejectsSymlinkParentRootAndLeaf(t *testing.T) {
	t.Run("projected parent", func(t *testing.T) {
		root := canonicalADKMCPWorkdirTestRoot(t)
		outside := canonicalADKMCPWorkdirTestRoot(t)
		require.NoError(t, os.Mkdir(filepath.Join(root, "spaces"), 0o700))
		require.NoError(t, os.Symlink(outside, filepath.Join(root, "spaces", "30")))
		preparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
			ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root},
		)
		prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
			context.Background(), validADKMCPRuntimeStdioSandboxExecution(root),
		)
		require.Error(t, err)
		require.Empty(t, prepared.WorkingDir)
		entries, readErr := os.ReadDir(outside)
		require.NoError(t, readErr)
		require.Empty(t, entries)
	})

	t.Run("configured root", func(t *testing.T) {
		target := canonicalADKMCPWorkdirTestRoot(t)
		container := canonicalADKMCPWorkdirTestRoot(t)
		rootLink := filepath.Join(container, "root-link")
		require.NoError(t, os.Symlink(target, rootLink))
		preparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
			ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: rootLink},
		)
		prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
			context.Background(), validADKMCPRuntimeStdioSandboxExecution(rootLink),
		)
		require.Error(t, err)
		require.Empty(t, prepared.WorkingDir)
	})

	t.Run("cleanup leaf", func(t *testing.T) {
		root := canonicalADKMCPWorkdirTestRoot(t)
		outside := canonicalADKMCPWorkdirTestRoot(t)
		marker := filepath.Join(outside, "must-survive")
		require.NoError(t, os.WriteFile(marker, []byte("safe"), 0o600))
		leaf := filepath.Join(root, "invocation-symlink")
		require.NoError(t, os.Symlink(outside, leaf))
		preparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
			ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root},
		)
		err := preparer.CleanupADKMCPRuntimeStdioWorkdir(
			context.Background(),
			ADKMCPRuntimeStdioPreparedWorkdir{Root: root, WorkingDir: leaf},
		)
		require.Error(t, err)
		_, statErr := os.Stat(marker)
		require.NoError(t, statErr)
	})
}

func TestADKMCPRuntimeStdioLeasedCleanupIgnoresCanceledContextAndIsConcurrentIdempotent(
	t *testing.T,
) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	store := &synchronizedADKMCPWorkdirLeaseStore{nextID: 8001}
	preparer := NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
		ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions{
			Inner: NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
				ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root},
			),
			LeaseStore:     store,
			CleanupTimeout: time.Second,
		},
	)
	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
		context.Background(), validADKMCPRuntimeStdioSandboxExecution(root),
	)
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	const callers = 16
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for index := 0; index < callers; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- preparer.CleanupADKMCPRuntimeStdioWorkdir(canceled, prepared)
		}()
	}
	wg.Wait()
	close(errs)
	for cleanupErr := range errs {
		require.NoError(t, cleanupErr)
	}
	store.mu.Lock()
	require.Equal(t, 1, store.finishCalls)
	require.NoError(t, store.finishContextErr)
	store.mu.Unlock()
	_, statErr := os.Stat(prepared.WorkingDir)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestADKMCPRuntimeStdioWorkdirRetryOldReaperCannotDeleteNewInvocation(t *testing.T) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	preparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root},
	)
	execution := validADKMCPRuntimeStdioSandboxExecution(root)
	oldPrepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(context.Background(), execution)
	require.NoError(t, err)
	newPrepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(context.Background(), execution)
	require.NoError(t, err)
	require.NotEqual(t, oldPrepared.WorkingDir, newPrepared.WorkingDir)
	require.NoError(t, os.WriteFile(filepath.Join(newPrepared.WorkingDir, "active"), []byte("active"), 0o600))

	oldLease := &domainentity.MCPRuntimeWorkdirLease{
		ID: 8101, Workdir: oldPrepared.WorkingDir,
		Status:   domainentity.MCPRuntimeWorkdirLeaseStatusActive,
		WorkerID: "worker-a", LeaseExpiresAt: 100,
	}
	newLease := &domainentity.MCPRuntimeWorkdirLease{
		ID: 8102, Workdir: newPrepared.WorkingDir,
		Status:   domainentity.MCPRuntimeWorkdirLeaseStatusActive,
		WorkerID: "worker-a", LeaseExpiresAt: 10_000,
	}
	repo := newConfirmingADKMCPWorkdirLeaseRepository(oldLease, newLease)
	repo.expired = []*domainentity.MCPRuntimeWorkdirLease{cloneMCPWorkdirLease(oldLease)}
	reaper := NewADKMCPRuntimeStdioWorkdirLeaseReaper(
		ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
			Repository: repo, Root: root, WorkdirPreparer: preparer,
			NowMillis: func() int64 { return 1000 },
		},
	)
	result, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.Cleaned)
	require.Equal(t, 1, result.Finished)
	_, oldErr := os.Stat(oldPrepared.WorkingDir)
	require.ErrorIs(t, oldErr, os.ErrNotExist)
	_, newErr := os.Stat(filepath.Join(newPrepared.WorkingDir, "active"))
	require.NoError(t, newErr)
	require.Equal(t, 1, repo.getCalls)
	require.NoError(t, preparer.CleanupADKMCPRuntimeStdioWorkdir(context.Background(), newPrepared))
}

func TestADKMCPRuntimeStdioWorkdirReaperSkipsRenewedLeaseAfterListing(t *testing.T) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	preparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root},
	)
	execution := validADKMCPRuntimeStdioSandboxExecution(root)
	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(context.Background(), execution)
	require.NoError(t, err)
	listed := &domainentity.MCPRuntimeWorkdirLease{
		ID: 8201, Workdir: prepared.WorkingDir,
		Status:   domainentity.MCPRuntimeWorkdirLeaseStatusActive,
		WorkerID: "worker-a", LeaseExpiresAt: 100,
	}
	renewed := cloneMCPWorkdirLease(listed)
	renewed.LeaseExpiresAt = 10_000
	renewed.UpdatedAt = 900
	repo := newConfirmingADKMCPWorkdirLeaseRepository(renewed)
	repo.expired = []*domainentity.MCPRuntimeWorkdirLease{listed}
	reaper := NewADKMCPRuntimeStdioWorkdirLeaseReaper(
		ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
			Repository: repo, Root: root, WorkdirPreparer: preparer,
			NowMillis: func() int64 { return 1000 },
		},
	)
	result, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, result.Listed)
	require.Zero(t, result.Cleaned)
	require.Zero(t, result.Finished)
	require.Zero(t, repo.finishCalls)
	_, statErr := os.Stat(prepared.WorkingDir)
	require.NoError(t, statErr)
	require.NoError(t, preparer.CleanupADKMCPRuntimeStdioWorkdir(context.Background(), prepared))
}

func TestApplicationADKMCPRuntimeStdioLeaseTTLIncludesExecutionAndCleanupBudget(t *testing.T) {
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{}
	store := NewApplicationADKMCPRuntimeStdioWorkdirLeaseStore(
		ApplicationADKMCPRuntimeStdioWorkdirLeaseStoreOptions{
			Repository: repo,
			IDGen:      &mcpWorkdirLeaseSequenceIDGen{next: 8301},
			WorkerID:   "worker-a", LeaseTTLMillis: 1,
			MinimumLeaseTTLMillis: 90_000,
			NowMillis:             func() int64 { return 1000 },
		},
	)
	_, err := store.CreateADKMCPRuntimeStdioWorkdirLease(
		context.Background(), mustProjectValidADKMCPRuntimeStdioExecution(t),
		ADKMCPRuntimeStdioPreparedWorkdir{
			Root: "/mnt/coze/mcp", WorkingDir: "/mnt/coze/mcp/invocation-test",
		},
	)
	require.NoError(t, err)
	require.Equal(t, int64(91_000), repo.createdLease.LeaseExpiresAt)
}

func canonicalADKMCPWorkdirTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.Chmod(root, defaultADKMCPRuntimeStdioWorkdirMode))
	return filepath.Clean(root)
}

type synchronizedADKMCPWorkdirLeaseStore struct {
	mu               sync.Mutex
	nextID           int64
	finishCalls      int
	finishContextErr error
}

func (s *synchronizedADKMCPWorkdirLeaseStore) CreateADKMCPRuntimeStdioWorkdirLease(
	_ context.Context,
	_ ADKMCPRuntimeStdioSandboxExecution,
	_ ADKMCPRuntimeStdioPreparedWorkdir,
) (ADKMCPRuntimeStdioWorkdirLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	return ADKMCPRuntimeStdioWorkdirLease{LeaseID: id, WorkerID: "worker-a"}, nil
}

func (s *synchronizedADKMCPWorkdirLeaseStore) FinishADKMCPRuntimeStdioWorkdirLease(
	ctx context.Context,
	_ ADKMCPRuntimeStdioWorkdirLease,
	_ ADKMCPRuntimeStdioWorkdirLeaseStatus,
	_ string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishCalls++
	s.finishContextErr = ctx.Err()
	return nil
}

type confirmingADKMCPWorkdirLeaseRepository struct {
	mu          sync.Mutex
	current     map[int64]*domainentity.MCPRuntimeWorkdirLease
	expired     []*domainentity.MCPRuntimeWorkdirLease
	getCalls    int
	finishCalls int
}

func newConfirmingADKMCPWorkdirLeaseRepository(
	leases ...*domainentity.MCPRuntimeWorkdirLease,
) *confirmingADKMCPWorkdirLeaseRepository {
	current := make(map[int64]*domainentity.MCPRuntimeWorkdirLease, len(leases))
	for _, lease := range leases {
		current[lease.ID] = cloneMCPWorkdirLease(lease)
	}
	return &confirmingADKMCPWorkdirLeaseRepository{current: current}
}

func (r *confirmingADKMCPWorkdirLeaseRepository) CreateMCPRuntimeWorkdirLease(
	context.Context,
	*domainentity.MCPRuntimeWorkdirLease,
) error {
	return nil
}

func (r *confirmingADKMCPWorkdirLeaseRepository) GetMCPRuntimeWorkdirLease(
	_ context.Context,
	leaseID int64,
) (*domainentity.MCPRuntimeWorkdirLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getCalls++
	return cloneMCPWorkdirLease(r.current[leaseID]), nil
}

func (r *confirmingADKMCPWorkdirLeaseRepository) ClaimExpiredMCPRuntimeWorkdirLease(
	_ context.Context,
	req domainrepo.ClaimExpiredMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lease := r.current[req.LeaseID]
	if lease == nil || lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
		lease.WorkerID != req.ExpectedWorkerID || lease.Workdir != req.ExpectedWorkdir ||
		lease.LeaseExpiresAt != req.ExpectedLeaseExpiresAt || lease.LeaseExpiresAt > req.Now {
		return nil, false, nil
	}
	lease.WorkerID = req.ClaimWorkerID
	lease.LeaseExpiresAt = req.ClaimExpiresAt
	lease.UpdatedAt = req.Now
	return cloneMCPWorkdirLease(lease), true, nil
}

func (r *confirmingADKMCPWorkdirLeaseRepository) RetryMCPRuntimeWorkdirLease(
	_ context.Context,
	req domainrepo.RetryMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lease := r.current[req.LeaseID]
	if lease == nil || lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
		lease.WorkerID != req.WorkerID {
		return nil, false, nil
	}
	lease.LeaseExpiresAt = req.RetryAt
	lease.UpdatedAt = req.Now
	lease.LastError = req.LastError
	return cloneMCPWorkdirLease(lease), true, nil
}

func (r *confirmingADKMCPWorkdirLeaseRepository) FinishMCPRuntimeWorkdirLease(
	_ context.Context,
	req domainrepo.FinishMCPRuntimeWorkdirLeaseRequest,
) (*domainentity.MCPRuntimeWorkdirLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishCalls++
	lease := r.current[req.LeaseID]
	if lease == nil || lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
		lease.WorkerID != req.WorkerID {
		return cloneMCPWorkdirLease(lease), false, nil
	}
	lease.Status = req.Status
	lease.UpdatedAt = req.Now
	return cloneMCPWorkdirLease(lease), true, nil
}

func (r *confirmingADKMCPWorkdirLeaseRepository) ListExpiredMCPRuntimeWorkdirLeases(
	context.Context,
	domainrepo.ListExpiredMCPRuntimeWorkdirLeasesRequest,
) ([]*domainentity.MCPRuntimeWorkdirLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*domainentity.MCPRuntimeWorkdirLease, 0, len(r.expired))
	for _, lease := range r.expired {
		result = append(result, cloneMCPWorkdirLease(lease))
	}
	return result, nil
}

func cloneMCPWorkdirLease(lease *domainentity.MCPRuntimeWorkdirLease) *domainentity.MCPRuntimeWorkdirLease {
	if lease == nil {
		return nil
	}
	cloned := *lease
	return &cloned
}
