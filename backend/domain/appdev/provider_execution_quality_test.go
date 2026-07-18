// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	sandboxcontract "github.com/coze-dev/coze-studio/backend/pkg/sandboxcontract"
)

func TestProviderExecutionQualityHydrateEnforcesCrossFieldState(t *testing.T) {
	now := time.Date(2026, 7, 16, 20, 0, 0, 0, time.UTC)
	base := &ProviderExecution{
		ID: "apx_quality", SpaceID: "1001", ProjectID: "project-a", Generation: 1,
		IdempotencyKey: "quality-idempotency", DesiredState: ProviderExecutionDesiredRun,
		ObservedState: ProviderExecutionObservedPending, ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
		Version: ProviderExecutionInitialVersion, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := HydrateProviderExecution(base); err != nil {
		t.Fatalf("valid pending hydration failed: %v", err)
	}

	running := *base
	running.ObservedState = ProviderExecutionObservedRunning
	if _, err := HydrateProviderExecution(&running); err == nil {
		t.Fatal("running without submission history/provider execution ID was hydrated")
	}

	terminal := *base
	terminal.ObservedState = ProviderExecutionObservedFailed
	if _, err := HydrateProviderExecution(&terminal); err == nil {
		t.Fatal("terminal state without submission history/provider execution ID was hydrated")
	}

	cleanup := *base
	cleanup.ObservedState = ProviderExecutionObservedCleanupComplete
	if _, err := HydrateProviderExecution(&cleanup); err == nil {
		t.Fatal("cleanup_complete with desired=run was hydrated")
	}
}

func TestProviderExecutionQualityUsesSharedEnvelopeLimit(t *testing.T) {
	now := time.Date(2026, 7, 16, 20, 0, 0, 0, time.UTC)
	operationHash, err := HashProviderExecutionOperationID("limit-operation")
	if err != nil {
		t.Fatal(err)
	}
	entity := &ProviderExecution{
		ID: "apx_limit", SpaceID: "1001", ProjectID: "project-a", Generation: 1,
		IdempotencyKey: "limit-idempotency", DesiredState: ProviderExecutionDesiredRun,
		ObservedState: ProviderExecutionObservedPending, ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
		CheckpointEnvelope:      strings.Repeat("x", sandboxcontract.MaxExecutionCheckpointEnvelopeBytes),
		CheckpointWriteRevision: 1, CheckpointLastOperationHash: operationHash,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := ValidateProviderExecutionEntity(entity); err != nil {
		t.Fatalf("boundary envelope rejected: %v", err)
	}
	entity.CheckpointEnvelope += "x"
	if err := ValidateProviderExecutionEntity(entity); err == nil {
		t.Fatal("over-limit envelope accepted")
	}
}
