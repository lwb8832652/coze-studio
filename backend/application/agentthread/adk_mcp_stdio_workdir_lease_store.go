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
	"path/filepath"
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const defaultADKMCPRuntimeStdioWorkdirLeaseTTLMillis = int64(300000)

type ApplicationADKMCPRuntimeStdioWorkdirLeaseStoreOptions struct {
	Repository     domainrepo.MCPRuntimeWorkdirLeaseRepository
	IDGen          idgen.IDGenerator
	WorkerID       string
	LeaseTTLMillis int64
	NowMillis      func() int64
}

type ApplicationADKMCPRuntimeStdioWorkdirLeaseStore struct {
	repository     domainrepo.MCPRuntimeWorkdirLeaseRepository
	idGen          idgen.IDGenerator
	workerID       string
	leaseTTLMillis int64
	nowMillis      func() int64
}

func NewApplicationADKMCPRuntimeStdioWorkdirLeaseStore(
	options ApplicationADKMCPRuntimeStdioWorkdirLeaseStoreOptions,
) *ApplicationADKMCPRuntimeStdioWorkdirLeaseStore {
	leaseTTLMillis := options.LeaseTTLMillis
	if leaseTTLMillis <= 0 {
		leaseTTLMillis = defaultADKMCPRuntimeStdioWorkdirLeaseTTLMillis
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &ApplicationADKMCPRuntimeStdioWorkdirLeaseStore{
		repository:     options.Repository,
		idGen:          options.IDGen,
		workerID:       strings.TrimSpace(options.WorkerID),
		leaseTTLMillis: leaseTTLMillis,
		nowMillis:      nowMillis,
	}
}

func (s *ApplicationADKMCPRuntimeStdioWorkdirLeaseStore) CreateADKMCPRuntimeStdioWorkdirLease(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) (ADKMCPRuntimeStdioWorkdirLease, error) {
	if !s.validCreateInput(execution, prepared) {
		return ADKMCPRuntimeStdioWorkdirLease{},
			errors.New("mcp runtime stdio workdir lease create failed")
	}
	leaseID, err := s.idGen.GenID(ctx)
	if err != nil || leaseID <= 0 {
		return ADKMCPRuntimeStdioWorkdirLease{},
			errors.New("mcp runtime stdio workdir lease create failed")
	}
	now := s.now()
	domainLease := &domainentity.MCPRuntimeWorkdirLease{
		ID:              leaseID,
		SpaceID:         execution.Run.SpaceID,
		ThreadID:        execution.Run.ThreadID,
		RunID:           execution.Run.RunID,
		ServerID:        execution.ServerID,
		RuntimeToolName: strings.TrimSpace(execution.Name),
		Workdir:         filepath.Clean(strings.TrimSpace(prepared.WorkingDir)),
		Status:          domainentity.MCPRuntimeWorkdirLeaseStatusActive,
		WorkerID:        s.workerID,
		LeaseExpiresAt:  now + s.leaseTTLMillis,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repository.CreateMCPRuntimeWorkdirLease(ctx, domainLease); err != nil {
		return ADKMCPRuntimeStdioWorkdirLease{},
			errors.New("mcp runtime stdio workdir lease create failed")
	}

	return ADKMCPRuntimeStdioWorkdirLease{
		LeaseID:  leaseID,
		WorkerID: s.workerID,
	}, nil
}

func (s *ApplicationADKMCPRuntimeStdioWorkdirLeaseStore) FinishADKMCPRuntimeStdioWorkdirLease(
	ctx context.Context,
	lease ADKMCPRuntimeStdioWorkdirLease,
	status ADKMCPRuntimeStdioWorkdirLeaseStatus,
	errorText string,
) error {
	if s == nil || s.repository == nil || lease.LeaseID <= 0 {
		return errors.New("mcp runtime stdio workdir lease finish failed")
	}
	domainStatus, ok := domainMCPRuntimeWorkdirLeaseStatus(status)
	if !ok {
		return errors.New("mcp runtime stdio workdir lease finish failed")
	}
	workerID := strings.TrimSpace(lease.WorkerID)
	if workerID == "" {
		workerID = s.workerID
	}
	if workerID == "" {
		return errors.New("mcp runtime stdio workdir lease finish failed")
	}

	_, updated, err := s.repository.FinishMCPRuntimeWorkdirLease(
		ctx,
		domainrepo.FinishMCPRuntimeWorkdirLeaseRequest{
			LeaseID:   lease.LeaseID,
			WorkerID:  workerID,
			Status:    domainStatus,
			Now:       s.now(),
			LastError: strings.TrimSpace(errorText),
		},
	)
	if err != nil || !updated {
		return errors.New("mcp runtime stdio workdir lease finish failed")
	}

	return nil
}

func (s *ApplicationADKMCPRuntimeStdioWorkdirLeaseStore) validCreateInput(
	execution ADKMCPRuntimeStdioSandboxExecution,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) bool {
	return s != nil &&
		s.repository != nil &&
		s.idGen != nil &&
		s.workerID != "" &&
		s.leaseTTLMillis > 0 &&
		execution.Run != nil &&
		execution.Run.RunID > 0 &&
		execution.Run.ThreadID > 0 &&
		execution.Run.SpaceID > 0 &&
		execution.ServerID > 0 &&
		isADKSubagentToolName(strings.TrimSpace(execution.Name)) &&
		strings.TrimSpace(prepared.WorkingDir) != "" &&
		filepath.IsAbs(strings.TrimSpace(prepared.WorkingDir))
}

func (s *ApplicationADKMCPRuntimeStdioWorkdirLeaseStore) now() int64 {
	if s == nil || s.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return s.nowMillis()
}

func domainMCPRuntimeWorkdirLeaseStatus(
	status ADKMCPRuntimeStdioWorkdirLeaseStatus,
) (domainentity.MCPRuntimeWorkdirLeaseStatus, bool) {
	switch status {
	case ADKMCPRuntimeStdioWorkdirLeaseStatusReleased:
		return domainentity.MCPRuntimeWorkdirLeaseStatusReleased, true
	case ADKMCPRuntimeStdioWorkdirLeaseStatusFailed:
		return domainentity.MCPRuntimeWorkdirLeaseStatusFailed, true
	default:
		return "", false
	}
}
