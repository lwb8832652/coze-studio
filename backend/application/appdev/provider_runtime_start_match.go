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
	"strings"
)

type providerRuntimeStartOperationLedger interface {
	MatchCurrentStartOperation(context.Context, MatchCurrentProviderExecutionStartRequest) (*ProviderExecutionMetadata, bool, error)
}

// MatchCurrentStartOperation performs an internal tenant-scoped postcondition
// check. The stable operation identifier is never copied into the projection.
func (orchestrator *ProviderRuntimeOrchestrator) MatchCurrentStartOperation(ctx context.Context, input ProviderRuntimeStartInput) (*ProviderRuntimeProjection, bool, error) {
	if orchestrator == nil || ctx == nil || ctx.Err() != nil || strings.TrimSpace(input.SpaceID) == "" ||
		strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.OperationID) == "" {
		return nil, false, ErrProviderRuntimeInvalid
	}
	ledger, ok := orchestrator.ledger.(providerRuntimeStartOperationLedger)
	if !ok {
		return nil, false, ErrProviderRuntimeUnavailable
	}
	metadata, matched, err := ledger.MatchCurrentStartOperation(ctx, MatchCurrentProviderExecutionStartRequest{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: input.OperationID,
	})
	if err != nil {
		return nil, false, err
	}
	if metadata == nil {
		return &ProviderRuntimeProjection{State: ProviderRuntimeStateStopped, CanStart: true}, false, nil
	}
	projection := providerRuntimeProjection(*metadata, "")
	return &projection, matched, nil
}
