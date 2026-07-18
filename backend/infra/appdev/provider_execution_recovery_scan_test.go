// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestProviderExecutionRecoveryProjectScanReturnsDistinctSafeIdentities(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:provider-recovery-scan?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&providerExecutionRecord{}))
	now := time.Now().UTC()
	records := []providerExecutionRecord{
		recoveryScanRecord("execution-1", 1001, "project-a", 1, domainappdev.ProviderExecutionObservedRunning, now),
		recoveryScanRecord("execution-2", 1001, "project-a", 2, domainappdev.ProviderExecutionObservedFailed, now.Add(time.Second)),
		recoveryScanRecord("execution-3", 1002, "project-b", 1, domainappdev.ProviderExecutionObservedCleanupComplete, now),
	}
	require.NoError(t, database.Create(&records).Error)

	projects, next, err := NewProviderExecutionRepository(database).ListProviderExecutionRecoveryProjects(context.Background(), nil, 32)
	require.NoError(t, err)
	require.Nil(t, next)
	require.Equal(t, []ProviderExecutionRecoveryProject{{SpaceID: "1001", ProjectID: "project-a"}}, projects)
	require.NotContains(t, fmt.Sprintf("%+v", projects), "execution-")
}

func TestProviderExecutionRecoveryProjectScanUsesDBOwnerLeaseAndStableCursor(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:provider-recovery-scan-cursor?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&providerExecutionRecord{}))
	now := time.Now().UTC()
	records := []providerExecutionRecord{
		recoveryScanRecord("expired-a", 1001, "project-a", 1, domainappdev.ProviderExecutionObservedRunning, now),
		recoveryScanRecord("healthy-b", 1001, "project-b", 1, domainappdev.ProviderExecutionObservedRunning, now),
		recoveryScanRecord("empty-c", 1001, "project-c", 1, domainappdev.ProviderExecutionObservedRunning, now),
		recoveryScanRecord("expired-d", 1002, "project-d", 1, domainappdev.ProviderExecutionObservedFailed, now),
	}
	expired := now.Add(-time.Hour)
	healthy := now.Add(time.Hour)
	records[0].OwnerIdentityHash, records[0].OwnerEpoch, records[0].OwnerExpiresAt = []byte("01234567890123456789012345678901"), 1, &expired
	records[1].OwnerIdentityHash, records[1].OwnerEpoch, records[1].OwnerExpiresAt = []byte("01234567890123456789012345678901"), 1, &healthy
	require.NoError(t, database.Create(&records).Error)

	repository := NewProviderExecutionRepository(database)
	first, cursor, err := repository.ListProviderExecutionRecoveryProjects(context.Background(), nil, 2)
	require.NoError(t, err)
	require.Equal(t, []ProviderExecutionRecoveryProject{
		{SpaceID: "1001", ProjectID: "project-a"},
		{SpaceID: "1001", ProjectID: "project-c"},
	}, first)
	require.NotNil(t, cursor)
	second, cursor, err := repository.ListProviderExecutionRecoveryProjects(context.Background(), cursor, 2)
	require.NoError(t, err)
	require.Equal(t, []ProviderExecutionRecoveryProject{{SpaceID: "1002", ProjectID: "project-d"}}, second)
	require.Nil(t, cursor)
}

func recoveryScanRecord(id string, spaceID int64, projectID string, generation uint64, state domainappdev.ProviderExecutionObservedState, updatedAt time.Time) providerExecutionRecord {
	return providerExecutionRecord{
		ID: id, SpaceID: spaceID, ProjectID: projectID, Generation: generation,
		IdempotencyKey: id + "-operation", DesiredState: domainappdev.ProviderExecutionDesiredRun,
		ObservedState: state, ProviderKey: appDevWiringTestProviderKeyForInfra,
		ProviderScope: domainsandbox.ScopeAppDev, Version: 1,
		CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
}

const appDevWiringTestProviderKeyForInfra = "018f0d2e-7b73-7e21-9a89-1a2b3c4d5e6f"
