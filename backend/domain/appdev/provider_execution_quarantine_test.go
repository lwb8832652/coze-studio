// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestProviderExecutionQuarantineDispositionHashBindsExecutionIdentity(t *testing.T) {
	const operationID = "quarantine-disposition-operation"
	base, err := HashProviderExecutionQuarantineDispositionOperationID(
		operationID, 42, "1001", "project-a", 7, "provider-a", domainsandbox.ScopeAppDev,
	)
	require.NoError(t, err)

	tests := []struct {
		name       string
		spaceID    string
		projectID  string
		generation uint64
		provider   string
		scope      domainsandbox.Scope
	}{
		{name: "space", spaceID: "1002", projectID: "project-a", generation: 7, provider: "provider-a", scope: domainsandbox.ScopeAppDev},
		{name: "project", spaceID: "1001", projectID: "project-b", generation: 7, provider: "provider-a", scope: domainsandbox.ScopeAppDev},
		{name: "generation", spaceID: "1001", projectID: "project-a", generation: 8, provider: "provider-a", scope: domainsandbox.ScopeAppDev},
		{name: "provider", spaceID: "1001", projectID: "project-a", generation: 7, provider: "provider-b", scope: domainsandbox.ScopeAppDev},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate, hashErr := HashProviderExecutionQuarantineDispositionOperationID(
				operationID, 42, test.spaceID, test.projectID, test.generation, test.provider, test.scope,
			)
			require.NoError(t, hashErr)
			require.False(t, base.Equal(candidate))
		})
	}
}

func TestProviderExecutionDisposedQuarantineHydratesAsAbortedCleanupIntent(t *testing.T) {
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	record := &ProviderExecution{
		ID: "execution-a", SpaceID: "1001", ProjectID: "project-a",
		Generation: 7, IdempotencyKey: "start-operation-a",
		DesiredState: ProviderExecutionDesiredStop, ObservedState: ProviderExecutionObservedPending,
		ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
		LaunchState: ProviderExecutionLaunchAborted, ArtifactStatus: ProviderExecutionArtifactNone,
		SafeErrorCode:    ProviderExecutionQuarantineDisposedCode,
		SafeErrorMessage: ProviderExecutionQuarantineDisposedMessage,
		Version:          12, CreatedAt: now, UpdatedAt: now,
	}

	_, err := HydrateProviderExecution(record)
	require.NoError(t, err)
}
