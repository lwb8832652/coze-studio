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

package appdev

import (
	"context"
	"testing"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

// This compile-time contract keeps runtime orchestration on the tenant-scoped
// current-generation lookup. Recovery scans intentionally remain a separate API.
func TestProviderRuntimeLoadCurrentContract(t *testing.T) {
	t.Helper()
	var _ interface {
		LoadCurrent(context.Context, LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error)
	} = (*ProviderExecutionService)(nil)
}

func (ledger *providerBuildFakeLedger) LoadCurrent(ctx context.Context, request LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error) {
	ledger.events.add("list")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.currentCalls++
	if request.SpaceID != ledger.metadata.SpaceID || request.ProjectID != ledger.metadata.ProjectID {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	current := ledger.metadata
	return &current, nil
}

func (ledger *runtimeStatusLedgerFake) LoadCurrent(_ context.Context, request LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.currentCount++
	if ledger.currentErr != nil {
		return nil, ledger.currentErr
	}
	if request.SpaceID != ledger.metadata.SpaceID || request.ProjectID != ledger.metadata.ProjectID {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	current := ledger.metadata
	return &current, nil
}

func (ledger *runtimeStopLedgerFake) LoadCurrent(_ context.Context, request LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.currentCount++
	if ledger.completed {
		return nil, domainappdev.ErrProviderExecutionNotFound
	}
	if request.SpaceID != ledger.metadata.SpaceID || request.ProjectID != ledger.metadata.ProjectID {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	current := ledger.metadata
	return &current, nil
}
