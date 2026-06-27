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

type ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions struct {
	Inner      ADKMCPRuntimeStdioWorkdirPreparer
	LeaseStore ADKMCPRuntimeStdioWorkdirLeaseStore
}

type ADKMCPRuntimeStdioLeasedWorkdirPreparer struct {
	inner      ADKMCPRuntimeStdioWorkdirPreparer
	leaseStore ADKMCPRuntimeStdioWorkdirLeaseStore
}

func NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
	options ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions,
) *ADKMCPRuntimeStdioLeasedWorkdirPreparer {
	return &ADKMCPRuntimeStdioLeasedWorkdirPreparer{
		inner:      options.Inner,
		leaseStore: options.LeaseStore,
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
		_ = p.inner.CleanupADKMCPRuntimeStdioWorkdir(ctx, prepared)
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir lease prepare failed")
	}
	prepared.LeaseID = lease.LeaseID
	prepared.LeaseWorkerID = strings.TrimSpace(lease.WorkerID)

	return prepared, nil
}

func (p *ADKMCPRuntimeStdioLeasedWorkdirPreparer) CleanupADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) error {
	if p == nil || p.inner == nil || p.leaseStore == nil ||
		prepared.LeaseID <= 0 {
		return errors.New("mcp runtime stdio workdir lease cleanup failed")
	}

	cleanupErr := p.inner.CleanupADKMCPRuntimeStdioWorkdir(ctx, prepared)
	status := ADKMCPRuntimeStdioWorkdirLeaseStatusReleased
	errorText := ""
	if cleanupErr != nil {
		status = ADKMCPRuntimeStdioWorkdirLeaseStatusFailed
		errorText = "cleanup failed"
	}
	finishErr := p.leaseStore.FinishADKMCPRuntimeStdioWorkdirLease(
		ctx,
		ADKMCPRuntimeStdioWorkdirLease{
			LeaseID:  prepared.LeaseID,
			WorkerID: strings.TrimSpace(prepared.LeaseWorkerID),
		},
		status,
		errorText,
	)
	if cleanupErr != nil {
		return errors.New("mcp runtime stdio workdir cleanup failed")
	}
	if finishErr != nil {
		return errors.New("mcp runtime stdio workdir lease finish failed")
	}

	return nil
}
