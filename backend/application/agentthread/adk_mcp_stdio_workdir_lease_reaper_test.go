/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * You may not use this file except in compliance with the License.
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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestADKMCPRuntimeStdioWorkdirLeaseReaperCleansExpiredLeases(
	t *testing.T,
) {
	root := t.TempDir()
	workdir := filepath.Join(root, "spaces", "30", "threads", "10", "runs", "20")
	require.NoError(t, os.MkdirAll(workdir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "scratch.txt"), []byte("secret"), 0o600))
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		expiredLeases: []*domainentity.MCPRuntimeWorkdirLease{
			{
				ID:             7001,
				Workdir:        workdir,
				Status:         domainentity.MCPRuntimeWorkdirLeaseStatusActive,
				WorkerID:       "worker-a",
				LeaseExpiresAt: 1000,
			},
		},
		finishedLease: &domainentity.MCPRuntimeWorkdirLease{
			ID:     7001,
			Status: domainentity.MCPRuntimeWorkdirLeaseStatusFailed,
		},
		finishOK: true,
	}
	reaper := NewADKMCPRuntimeStdioWorkdirLeaseReaper(
		ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
			Repository: repo,
			Root:       root,
			BatchSize:  3,
			NowMillis:  func() int64 { return 2000 },
		},
	)

	result, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(
		context.Background(),
	)

	require.NoError(t, err)
	require.Equal(t, ADKMCPRuntimeStdioWorkdirLeaseReaperResult{
		Listed:   1,
		Cleaned:  1,
		Finished: 1,
	}, result)
	_, statErr := os.Stat(workdir)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Stat(root)
	require.NoError(t, statErr)
	require.Equal(t, 1, repo.listCalls)
	require.Equal(t, int64(2000), repo.listReq.Now)
	require.Equal(t, int32(3), repo.listReq.Limit)
	require.Equal(t, int64(7001), repo.finishReq.LeaseID)
	require.Equal(t, "worker-a", repo.finishReq.WorkerID)
	require.Equal(t, domainentity.MCPRuntimeWorkdirLeaseStatusFailed, repo.finishReq.Status)
	require.Equal(t, int64(2000), repo.finishReq.Now)
	require.Equal(t, "stale lease expired", repo.finishReq.LastError)
}

func TestADKMCPRuntimeStdioWorkdirLeaseReaperRejectsUnsafeLeaseWorkdir(
	t *testing.T,
) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-outside")
	require.NoError(t, os.MkdirAll(outside, 0o700))
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		expiredLeases: []*domainentity.MCPRuntimeWorkdirLease{
			{
				ID:       7001,
				Workdir:  outside,
				Status:   domainentity.MCPRuntimeWorkdirLeaseStatusActive,
				WorkerID: "worker-a",
			},
		},
		finishOK: true,
	}
	reaper := NewADKMCPRuntimeStdioWorkdirLeaseReaper(
		ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
			Repository: repo,
			Root:       root,
			NowMillis:  func() int64 { return 2000 },
		},
	)

	result, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(
		context.Background(),
	)

	require.NoError(t, err)
	require.Equal(t, ADKMCPRuntimeStdioWorkdirLeaseReaperResult{
		Listed:  1,
		Invalid: 1,
		Failed:  1,
	}, result)
	require.Equal(t, 0, repo.finishCalls)
	_, statErr := os.Stat(outside)
	require.NoError(t, statErr)
}

func TestADKMCPRuntimeStdioWorkdirLeaseReaperDoesNotFinishAfterCleanupFailure(
	t *testing.T,
) {
	root := t.TempDir()
	workdir := filepath.Join(root, "spaces", "30", "threads", "10", "runs", "20")
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		expiredLeases: []*domainentity.MCPRuntimeWorkdirLease{
			{
				ID:       7001,
				Workdir:  workdir,
				Status:   domainentity.MCPRuntimeWorkdirLeaseStatusActive,
				WorkerID: "worker-a",
			},
		},
		finishOK: true,
	}
	preparer := &recordingADKMCPRuntimeStdioWorkdirPreparer{
		cleanupErr: errors.New("remove /mnt/coze/mcp with stdio-secret-token"),
	}
	reaper := NewADKMCPRuntimeStdioWorkdirLeaseReaper(
		ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
			Repository:      repo,
			Root:            root,
			WorkdirPreparer: preparer,
			NowMillis:       func() int64 { return 2000 },
		},
	)

	result, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(
		context.Background(),
	)

	require.NoError(t, err)
	require.Equal(t, ADKMCPRuntimeStdioWorkdirLeaseReaperResult{
		Listed: 1,
		Failed: 1,
	}, result)
	require.Equal(t, 1, preparer.cleanupCalls)
	require.Equal(t, 0, repo.finishCalls)
}

func TestADKMCPRuntimeStdioWorkdirLeaseReaperSanitizesListErrors(
	t *testing.T,
) {
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		listErr: errors.New("select /mnt/coze/mcp with stdio-secret-token"),
	}
	reaper := NewADKMCPRuntimeStdioWorkdirLeaseReaper(
		ADKMCPRuntimeStdioWorkdirLeaseReaperOptions{
			Repository: repo,
			Root:       t.TempDir(),
			NowMillis:  func() int64 { return 2000 },
		},
	)

	result, err := reaper.CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(
		context.Background(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio workdir lease cleanup failed")
	require.NotContains(t, err.Error(), "/mnt/coze/mcp")
	require.NotContains(t, err.Error(), "stdio-secret-token")
}
