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
	"path/filepath"
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

const (
	defaultADKMCPRuntimeStdioWorkdirLeaseReaperBatchSize = int32(10)
	adkMCPRuntimeStdioWorkdirStaleLeaseError             = "stale lease expired"
)

type ADKMCPRuntimeStdioWorkdirLeaseReaperOptions struct {
	Repository      domainrepo.MCPRuntimeWorkdirLeaseRepository
	Root            string
	WorkdirPreparer ADKMCPRuntimeStdioWorkdirPreparer
	BatchSize       int32
	NowMillis       func() int64
}

type ADKMCPRuntimeStdioWorkdirLeaseReaper struct {
	repository      domainrepo.MCPRuntimeWorkdirLeaseRepository
	root            string
	workdirPreparer ADKMCPRuntimeStdioWorkdirPreparer
	batchSize       int32
	nowMillis       func() int64
}

type ADKMCPRuntimeStdioWorkdirLeaseReaperResult struct {
	Listed   int
	Cleaned  int
	Finished int
	Invalid  int
	Failed   int
}

func NewADKMCPRuntimeStdioWorkdirLeaseReaper(
	options ADKMCPRuntimeStdioWorkdirLeaseReaperOptions,
) *ADKMCPRuntimeStdioWorkdirLeaseReaper {
	root := filepath.Clean(strings.TrimSpace(options.Root))
	preparer := options.WorkdirPreparer
	if preparer == nil {
		preparer = NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
			ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root},
		)
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultADKMCPRuntimeStdioWorkdirLeaseReaperBatchSize
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &ADKMCPRuntimeStdioWorkdirLeaseReaper{
		repository:      options.Repository,
		root:            root,
		workdirPreparer: preparer,
		batchSize:       batchSize,
		nowMillis:       nowMillis,
	}
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) CleanupExpiredADKMCPRuntimeStdioWorkdirLeases(
	ctx context.Context,
) (ADKMCPRuntimeStdioWorkdirLeaseReaperResult, error) {
	result := ADKMCPRuntimeStdioWorkdirLeaseReaperResult{}
	if !r.validConfig() {
		return result, errors.New("mcp runtime stdio workdir lease cleanup failed")
	}
	now := r.now()
	leases, err := r.repository.ListExpiredMCPRuntimeWorkdirLeases(
		ctx,
		domainrepo.ListExpiredMCPRuntimeWorkdirLeasesRequest{
			Now:   now,
			Limit: r.batchSize,
		},
	)
	if err != nil {
		return result, errors.New("mcp runtime stdio workdir lease cleanup failed")
	}
	result.Listed = len(leases)
	for _, lease := range leases {
		r.cleanupOne(ctx, now, lease, &result)
	}

	return result, nil
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) cleanupOne(
	ctx context.Context,
	now int64,
	lease *domainentity.MCPRuntimeWorkdirLease,
	result *ADKMCPRuntimeStdioWorkdirLeaseReaperResult,
) {
	prepared, ok := r.preparedWorkdirFromLease(lease)
	if !ok {
		result.Invalid++
		result.Failed++
		return
	}
	if err := r.workdirPreparer.CleanupADKMCPRuntimeStdioWorkdir(ctx, prepared); err != nil {
		result.Failed++
		return
	}
	result.Cleaned++

	_, updated, err := r.repository.FinishMCPRuntimeWorkdirLease(
		ctx,
		domainrepo.FinishMCPRuntimeWorkdirLeaseRequest{
			LeaseID:   lease.ID,
			WorkerID:  strings.TrimSpace(lease.WorkerID),
			Status:    domainentity.MCPRuntimeWorkdirLeaseStatusFailed,
			Now:       now,
			LastError: adkMCPRuntimeStdioWorkdirStaleLeaseError,
		},
	)
	if err != nil || !updated {
		result.Failed++
		return
	}
	result.Finished++
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) preparedWorkdirFromLease(
	lease *domainentity.MCPRuntimeWorkdirLease,
) (ADKMCPRuntimeStdioPreparedWorkdir, bool) {
	if lease == nil ||
		lease.ID <= 0 ||
		lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
		strings.TrimSpace(lease.WorkerID) == "" {
		return ADKMCPRuntimeStdioPreparedWorkdir{}, false
	}
	workdir := strings.TrimSpace(lease.Workdir)
	if workdir == "" || !filepath.IsAbs(workdir) {
		return ADKMCPRuntimeStdioPreparedWorkdir{}, false
	}
	cleanWorkdir := filepath.Clean(workdir)
	if cleanWorkdir == r.root || !adkMCPRuntimePathWithin(cleanWorkdir, r.root) {
		return ADKMCPRuntimeStdioPreparedWorkdir{}, false
	}

	return ADKMCPRuntimeStdioPreparedWorkdir{
		Root:          r.root,
		WorkingDir:    cleanWorkdir,
		LeaseID:       lease.ID,
		LeaseWorkerID: strings.TrimSpace(lease.WorkerID),
	}, true
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) validConfig() bool {
	return r != nil &&
		r.repository != nil &&
		r.workdirPreparer != nil &&
		filepath.IsAbs(r.root) &&
		r.batchSize > 0
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) now() int64 {
	if r == nil || r.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return r.nowMillis()
}
