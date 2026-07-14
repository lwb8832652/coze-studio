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
	"errors"
	"strings"
	"time"
)

type ADKMCPRuntimeStdioWorkdirLeaseStatus string

const (
	ADKMCPRuntimeStdioWorkdirLeaseStatusReleased ADKMCPRuntimeStdioWorkdirLeaseStatus = "released"
	ADKMCPRuntimeStdioWorkdirLeaseStatusFailed   ADKMCPRuntimeStdioWorkdirLeaseStatus = "failed"
)

type ADKMCPRuntimeStdioWorkdirLease struct {
	LeaseID  int64
	WorkerID string
}

type ADKMCPRuntimeStdioWorkdirLeaseStore interface {
	CreateADKMCPRuntimeStdioWorkdirLease(
		ctx context.Context,
		execution ADKMCPRuntimeStdioSandboxExecution,
		prepared ADKMCPRuntimeStdioPreparedWorkdir,
	) (ADKMCPRuntimeStdioWorkdirLease, error)
	FinishADKMCPRuntimeStdioWorkdirLease(
		ctx context.Context,
		lease ADKMCPRuntimeStdioWorkdirLease,
		status ADKMCPRuntimeStdioWorkdirLeaseStatus,
		errorText string,
	) error
}

type ADKMCPRuntimeStdioRetryableWorkdirLeaseStore interface {
	RetryADKMCPRuntimeStdioWorkdirLease(
		ctx context.Context,
		lease ADKMCPRuntimeStdioWorkdirLease,
		retryAt int64,
		errorText string,
	) error
}

type ADKMCPRuntimeStdioRetryableWorkdirPreparer interface {
	RetryADKMCPRuntimeStdioWorkdirCleanup(
		ctx context.Context,
		prepared ADKMCPRuntimeStdioPreparedWorkdir,
		errorText string,
	) error
}

type ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions struct {
	Inner          ADKMCPRuntimeStdioWorkdirPreparer
	LeaseStore     ADKMCPRuntimeStdioWorkdirLeaseStore
	CleanupTimeout time.Duration
	RetryDelay     time.Duration
	NowMillis      func() int64
}

type ADKMCPRuntimeStdioLeasedWorkdirPreparer struct {
	inner          ADKMCPRuntimeStdioWorkdirPreparer
	leaseStore     ADKMCPRuntimeStdioWorkdirLeaseStore
	cleanupTimeout time.Duration
	retryDelay     time.Duration
	nowMillis      func() int64
}

func NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
	options ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions,
) *ADKMCPRuntimeStdioLeasedWorkdirPreparer {
	cleanupTimeout := options.CleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = defaultADKMCPRuntimeStdioWorkdirCleanupTimeout
	}
	retryDelay := options.RetryDelay
	if retryDelay <= 0 {
		retryDelay = time.Minute
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}
	return &ADKMCPRuntimeStdioLeasedWorkdirPreparer{
		inner:          options.Inner,
		leaseStore:     options.LeaseStore,
		cleanupTimeout: cleanupTimeout,
		retryDelay:     retryDelay,
		nowMillis:      nowMillis,
	}
}

func (p *ADKMCPRuntimeStdioLeasedWorkdirPreparer) PrepareADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (ADKMCPRuntimeStdioPreparedWorkdir, error) {
	if p == nil || p.inner == nil || p.leaseStore == nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir lease prepare failed")
	}

	prepared, err := p.inner.PrepareADKMCPRuntimeStdioWorkdir(ctx, execution)
	if err != nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{}, err
	}
	lease, err := p.leaseStore.CreateADKMCPRuntimeStdioWorkdirLease(
		ctx,
		execution,
		prepared,
	)
	if err != nil || lease.LeaseID <= 0 {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), p.cleanupTimeout)
		defer cancel()
		_ = p.inner.CleanupADKMCPRuntimeStdioWorkdir(cleanupCtx, prepared)
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir lease prepare failed")
	}
	prepared.LeaseID = lease.LeaseID
	prepared.LeaseWorkerID = strings.TrimSpace(lease.WorkerID)
	prepared.leaseCleanupState = &adkMCPRuntimeStdioWorkdirCleanupState{}

	return prepared, nil
}

func (p *ADKMCPRuntimeStdioLeasedWorkdirPreparer) CleanupADKMCPRuntimeStdioWorkdir(
	_ context.Context,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) error {
	if p == nil || p.inner == nil || p.leaseStore == nil ||
		prepared.LeaseID <= 0 {
		return errors.New("mcp runtime stdio workdir lease cleanup failed")
	}

	state := prepared.leaseCleanupState
	if state == nil {
		state = &adkMCPRuntimeStdioWorkdirCleanupState{}
	}
	state.once.Do(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), p.cleanupTimeout)
		cleanupErr := p.inner.CleanupADKMCPRuntimeStdioWorkdir(cleanupCtx, prepared)
		cancelCleanup()
		if cleanupErr != nil {
			retryStore, ok := p.leaseStore.(ADKMCPRuntimeStdioRetryableWorkdirLeaseStore)
			if ok {
				retryCtx, cancelRetry := context.WithTimeout(context.Background(), p.cleanupTimeout)
				_ = retryStore.RetryADKMCPRuntimeStdioWorkdirLease(
					retryCtx,
					ADKMCPRuntimeStdioWorkdirLease{
						LeaseID: prepared.LeaseID, WorkerID: strings.TrimSpace(prepared.LeaseWorkerID),
					},
					p.nowMillis()+p.retryDelay.Milliseconds(),
					"cleanup retryable",
				)
				cancelRetry()
			}
			state.err = errors.New("mcp runtime stdio workdir cleanup failed")
			return
		}
		finishCtx, cancelFinish := context.WithTimeout(context.Background(), p.cleanupTimeout)
		finishErr := p.leaseStore.FinishADKMCPRuntimeStdioWorkdirLease(
			finishCtx,
			ADKMCPRuntimeStdioWorkdirLease{
				LeaseID:  prepared.LeaseID,
				WorkerID: strings.TrimSpace(prepared.LeaseWorkerID),
			},
			ADKMCPRuntimeStdioWorkdirLeaseStatusReleased,
			"",
		)
		cancelFinish()
		if finishErr != nil {
			state.err = errors.New("mcp runtime stdio workdir lease finish failed")
		}
	})

	return state.err
}

func (p *ADKMCPRuntimeStdioLeasedWorkdirPreparer) RetryADKMCPRuntimeStdioWorkdirCleanup(
	_ context.Context,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
	_ string,
) error {
	if p == nil || p.leaseStore == nil || prepared.LeaseID <= 0 {
		return errors.New("mcp runtime stdio workdir cleanup retry failed")
	}
	retryStore, ok := p.leaseStore.(ADKMCPRuntimeStdioRetryableWorkdirLeaseStore)
	if !ok {
		return errors.New("mcp runtime stdio workdir cleanup retry failed")
	}
	retryCtx, cancel := context.WithTimeout(context.Background(), p.cleanupTimeout)
	defer cancel()
	if err := retryStore.RetryADKMCPRuntimeStdioWorkdirLease(
		retryCtx,
		ADKMCPRuntimeStdioWorkdirLease{
			LeaseID: prepared.LeaseID, WorkerID: strings.TrimSpace(prepared.LeaseWorkerID),
		},
		p.nowMillis()+p.retryDelay.Milliseconds(),
		"process termination unconfirmed",
	); err != nil {
		return errors.New("mcp runtime stdio workdir cleanup retry failed")
	}
	return nil
}
