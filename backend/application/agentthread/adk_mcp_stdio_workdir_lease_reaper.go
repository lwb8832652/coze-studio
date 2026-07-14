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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

const (
	defaultADKMCPRuntimeStdioWorkdirLeaseReaperBatchSize = int32(10)
	defaultADKMCPRuntimeStdioWorkdirLeaseRetryDelay      = time.Minute
	adkMCPRuntimeStdioWorkdirStaleLeaseError             = "stale lease expired"
	adkMCPRuntimeStdioWorkdirRetryableError              = "cleanup retryable"
)

type ADKMCPRuntimeStdioWorkdirLeaseReaperOptions struct {
	Repository      domainrepo.MCPRuntimeWorkdirLeaseRepository
	Root            string
	WorkdirPreparer ADKMCPRuntimeStdioWorkdirPreparer
	BatchSize       int32
	CleanupTimeout  time.Duration
	RetryDelay      time.Duration
	ClaimTTL        time.Duration
	NowMillis       func() int64
}

type ADKMCPRuntimeStdioWorkdirLeaseReaper struct {
	repository      domainrepo.MCPRuntimeWorkdirLeaseRepository
	root            string
	workdirPreparer ADKMCPRuntimeStdioWorkdirPreparer
	batchSize       int32
	cleanupTimeout  time.Duration
	retryDelay      time.Duration
	claimTTL        time.Duration
	nowMillis       func() int64
}

type ADKMCPRuntimeStdioWorkdirLeaseReaperResult struct {
	Listed   int
	Claimed  int
	Cleaned  int
	Finished int
	Retried  int
	Invalid  int
	Failed   int
}

func NewADKMCPRuntimeStdioWorkdirLeaseReaper(
	options ADKMCPRuntimeStdioWorkdirLeaseReaperOptions,
) *ADKMCPRuntimeStdioWorkdirLeaseReaper {
	root := filepath.Clean(strings.TrimSpace(options.Root))
	preparer := options.WorkdirPreparer
	if preparer == nil {
		filesystemPreparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
			ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{
				Root: root, CleanupTimeout: options.CleanupTimeout,
			},
		)
		preparer = filesystemPreparer
		if filesystemPreparer.Valid() {
			root = filesystemPreparer.Root()
		}
	}
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultADKMCPRuntimeStdioWorkdirLeaseReaperBatchSize
	}
	cleanupTimeout := options.CleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = defaultADKMCPRuntimeStdioWorkdirCleanupTimeout
	}
	retryDelay := options.RetryDelay
	if retryDelay <= 0 {
		retryDelay = defaultADKMCPRuntimeStdioWorkdirLeaseRetryDelay
	}
	claimTTL := options.ClaimTTL
	minimumClaimTTL := cleanupTimeout + defaultADKMCPRuntimeStdioWorkdirCleanupMargin
	if claimTTL < minimumClaimTTL {
		claimTTL = minimumClaimTTL
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}
	return &ADKMCPRuntimeStdioWorkdirLeaseReaper{
		repository: options.Repository, root: root, workdirPreparer: preparer,
		batchSize: batchSize, cleanupTimeout: cleanupTimeout,
		retryDelay: retryDelay, claimTTL: claimTTL, nowMillis: nowMillis,
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
		domainrepo.ListExpiredMCPRuntimeWorkdirLeasesRequest{Now: now, Limit: r.batchSize},
	)
	if err != nil {
		return result, errors.New("mcp runtime stdio workdir lease cleanup failed")
	}
	result.Listed = len(leases)
	for _, lease := range leases {
		r.cleanupOne(lease, &result)
	}
	return result, nil
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) cleanupOne(
	listed *domainentity.MCPRuntimeWorkdirLease,
	result *ADKMCPRuntimeStdioWorkdirLeaseReaperResult,
) {
	now := r.now()
	if !validExpiredADKMCPRuntimeStdioLease(listed, now) {
		result.Invalid++
		result.Failed++
		return
	}
	claimWorkerID, ok := newADKMCPRuntimeStdioWorkdirClaimID()
	if !ok {
		result.Failed++
		return
	}
	claimExpiresAt := now + r.claimTTL.Milliseconds()
	claimCtx, cancelClaim := context.WithTimeout(context.Background(), r.cleanupTimeout)
	claimed, claimedOK, err := r.repository.ClaimExpiredMCPRuntimeWorkdirLease(
		claimCtx,
		domainrepo.ClaimExpiredMCPRuntimeWorkdirLeaseRequest{
			LeaseID:                listed.ID,
			ExpectedWorkerID:       strings.TrimSpace(listed.WorkerID),
			ExpectedWorkdir:        filepath.Clean(strings.TrimSpace(listed.Workdir)),
			ExpectedLeaseExpiresAt: listed.LeaseExpiresAt,
			ClaimWorkerID:          claimWorkerID,
			Now:                    now, ClaimExpiresAt: claimExpiresAt,
		},
	)
	cancelClaim()
	if err != nil {
		result.Failed++
		return
	}
	if !claimedOK || claimed == nil {
		return
	}
	result.Claimed++
	getCtx, cancelGet := context.WithTimeout(context.Background(), r.cleanupTimeout)
	current, err := r.repository.GetMCPRuntimeWorkdirLease(getCtx, listed.ID)
	cancelGet()
	if err != nil || !sameClaimedADKMCPRuntimeStdioLease(
		listed, claimed, current, claimWorkerID, claimExpiresAt,
	) {
		result.Failed++
		return
	}
	prepared, valid := r.preparedWorkdirFromLease(current)
	if !valid {
		result.Invalid++
		r.retryClaim(current, result)
		return
	}
	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), r.cleanupTimeout)
	cleanupErr := r.workdirPreparer.CleanupADKMCPRuntimeStdioWorkdir(cleanupCtx, prepared)
	cancelCleanup()
	if cleanupErr != nil {
		r.retryClaim(current, result)
		return
	}
	result.Cleaned++
	finishCtx, cancelFinish := context.WithTimeout(context.Background(), r.cleanupTimeout)
	_, updated, err := r.repository.FinishMCPRuntimeWorkdirLease(
		finishCtx,
		domainrepo.FinishMCPRuntimeWorkdirLeaseRequest{
			LeaseID: current.ID, WorkerID: claimWorkerID,
			Status: domainentity.MCPRuntimeWorkdirLeaseStatusFailed,
			Now:    r.now(), LastError: adkMCPRuntimeStdioWorkdirStaleLeaseError,
		},
	)
	cancelFinish()
	if err != nil || !updated {
		result.Failed++
		return
	}
	result.Finished++
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) retryClaim(
	lease *domainentity.MCPRuntimeWorkdirLease,
	result *ADKMCPRuntimeStdioWorkdirLeaseReaperResult,
) {
	now := r.now()
	retryCtx, cancelRetry := context.WithTimeout(context.Background(), r.cleanupTimeout)
	_, updated, err := r.repository.RetryMCPRuntimeWorkdirLease(
		retryCtx,
		domainrepo.RetryMCPRuntimeWorkdirLeaseRequest{
			LeaseID: lease.ID, WorkerID: strings.TrimSpace(lease.WorkerID),
			Now: now, RetryAt: now + r.retryDelay.Milliseconds(),
			LastError: adkMCPRuntimeStdioWorkdirRetryableError,
		},
	)
	cancelRetry()
	if err != nil || !updated {
		result.Failed++
		return
	}
	result.Retried++
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) preparedWorkdirFromLease(
	lease *domainentity.MCPRuntimeWorkdirLease,
) (ADKMCPRuntimeStdioPreparedWorkdir, bool) {
	if lease == nil || lease.ID <= 0 ||
		lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
		strings.TrimSpace(lease.WorkerID) == "" {
		return ADKMCPRuntimeStdioPreparedWorkdir{}, false
	}
	workdir := filepath.Clean(strings.TrimSpace(lease.Workdir))
	if !filepath.IsAbs(workdir) || workdir == r.root ||
		!adkMCPRuntimePathWithin(workdir, r.root) ||
		!strings.HasPrefix(filepath.Base(workdir), adkMCPRuntimeStdioInvocationDirPrefix) {
		return ADKMCPRuntimeStdioPreparedWorkdir{}, false
	}
	return ADKMCPRuntimeStdioPreparedWorkdir{
		Root: r.root, WorkingDir: workdir,
		LeaseID: lease.ID, LeaseWorkerID: strings.TrimSpace(lease.WorkerID),
	}, true
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) validConfig() bool {
	if r == nil || r.repository == nil || r.workdirPreparer == nil ||
		!filepath.IsAbs(r.root) || r.batchSize <= 0 || r.cleanupTimeout <= 0 ||
		r.retryDelay <= 0 || r.claimTTL <= r.cleanupTimeout {
		return false
	}
	if preparer, ok := r.workdirPreparer.(*ADKMCPRuntimeStdioFilesystemWorkdirPreparer); ok {
		return preparer.Valid() && preparer.Root() == r.root
	}
	return true
}

func (r *ADKMCPRuntimeStdioWorkdirLeaseReaper) now() int64 {
	if r == nil || r.nowMillis == nil {
		return time.Now().UnixMilli()
	}
	return r.nowMillis()
}

func validExpiredADKMCPRuntimeStdioLease(
	lease *domainentity.MCPRuntimeWorkdirLease,
	now int64,
) bool {
	return lease != nil && lease.ID > 0 &&
		lease.Status == domainentity.MCPRuntimeWorkdirLeaseStatusActive &&
		strings.TrimSpace(lease.WorkerID) != "" &&
		filepath.IsAbs(strings.TrimSpace(lease.Workdir)) &&
		lease.LeaseExpiresAt > 0 && lease.LeaseExpiresAt <= now
}

func sameClaimedADKMCPRuntimeStdioLease(
	listed *domainentity.MCPRuntimeWorkdirLease,
	claimed *domainentity.MCPRuntimeWorkdirLease,
	current *domainentity.MCPRuntimeWorkdirLease,
	claimWorkerID string,
	claimExpiresAt int64,
) bool {
	if listed == nil || claimed == nil || current == nil {
		return false
	}
	expectedPath := filepath.Clean(strings.TrimSpace(listed.Workdir))
	for _, lease := range []*domainentity.MCPRuntimeWorkdirLease{claimed, current} {
		if lease.ID != listed.ID ||
			lease.Status != domainentity.MCPRuntimeWorkdirLeaseStatusActive ||
			strings.TrimSpace(lease.WorkerID) != claimWorkerID ||
			filepath.Clean(strings.TrimSpace(lease.Workdir)) != expectedPath ||
			lease.LeaseExpiresAt != claimExpiresAt {
			return false
		}
	}
	return true
}

func newADKMCPRuntimeStdioWorkdirClaimID() (string, bool) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", false
	}
	return "reaper-" + hex.EncodeToString(nonce), true
}
