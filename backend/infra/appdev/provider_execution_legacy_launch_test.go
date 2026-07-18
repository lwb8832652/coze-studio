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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestProviderExecutionLegacyLaunchRowsHydrateWithoutBecomingRepositoryUnavailable(t *testing.T) {
	tests := []struct {
		name     string
		state    domainappdev.ProviderExecutionLaunchState
		mutate   func(*providerExecutionRecord)
		wantCode string
	}{
		{
			name:  "old pending with handle and checkpoint is complete",
			state: domainappdev.ProviderExecutionLaunchComplete,
			mutate: func(record *providerExecutionRecord) {
				record.ObservedState = domainappdev.ProviderExecutionObservedRunning
				record.ProviderExecutionID = "provider-execution-legacy"
				now := time.Now().UTC()
				record.SubmissionStartedAt = &now
				record.CheckpointEnvelope = "ecp1:legacy-checkpoint"
				record.CheckpointWriteRevision = 1
				record.CheckpointLastOperationHash = testOperationHash(t, "legacy-checkpoint").Bytes()
				record.LaunchOperationHash = testOperationHash(t, "legacy-launch").Bytes()
				record.LaunchProviderOperationID = "appdev_start_legacy"
			},
		},
		{
			name:  "old pending without handle but operation is reconcilable",
			state: domainappdev.ProviderExecutionLaunchLegacySubmitted,
			mutate: func(record *providerExecutionRecord) {
				record.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
				now := time.Now().UTC()
				record.SubmissionStartedAt = &now
				record.LaunchOperationHash = testOperationHash(t, "legacy-launch").Bytes()
				record.LaunchProviderOperationID = "appdev_start_legacy"
			},
		},
		{
			name:  "old pending without identity is quarantined but observable",
			state: domainappdev.ProviderExecutionLaunchQuarantined,
			mutate: func(record *providerExecutionRecord) {
				record.IdempotencyKey = "legacy-quarantine-row"
				record.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
				now := time.Now().UTC()
				record.SubmissionStartedAt = &now
				record.SafeErrorCode = "legacy_launch_quarantined"
				record.SafeErrorMessage = "provider launch requires operator recovery"
			},
			wantCode: "legacy_launch_quarantined",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
			record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput(
				"legacy-row", "legacy-operation", time.Now().UTC(),
			))
			require.NoError(t, err)
			var raw providerExecutionRecord
			require.NoError(t, db.Where("id = ?", record.ID).Take(&raw).Error)
			test.mutate(&raw)
			raw.LaunchState = test.state
			raw.LaunchExpiresAt = nil
			require.NoError(t, db.Save(&raw).Error)

			loaded, err := repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
				SpaceID: "1001", ProjectID: "project-a",
			})
			require.NoError(t, err)
			require.Equal(t, test.state, loaded.LaunchState)
			require.Equal(t, test.wantCode, loaded.SafeErrorCode)
		})
	}
}

func TestProviderExecutionLegacyLaunchForwardMigrationIsExplicitAndFailClosed(t *testing.T) {
	migration, err := os.ReadFile("../../../docker/atlas/migrations/20260717000600_appdev_provider_legacy_launch_recovery.sql")
	require.NoError(t, err)
	sql := string(migration)
	require.Contains(t, sql, "MODIFY COLUMN `launch_state` varchar(24)")
	require.Contains(t, sql, "`launch_state` = 'complete'")
	require.Contains(t, sql, "`launch_state` = 'legacy_submitted'")
	require.Contains(t, sql, "`launch_state` = 'quarantined'")
	require.Contains(t, sql, "SHA2")
	require.Contains(t, sql, "legacy_launch_quarantined")
	require.NotContains(t, sql, "`launch_state` = 'aborted'")

	for _, historical := range []string{
		"../../../docker/atlas/migrations/20260717000400_appdev_provider_launch_interlock.sql",
		"../../../docker/atlas/migrations/20260717000500_appdev_provider_launch_reconciliation.sql",
	} {
		content, readErr := os.ReadFile(historical)
		require.NoError(t, readErr)
		require.False(t, strings.Contains(string(content), "legacy_submitted"))
		require.False(t, strings.Contains(string(content), "quarantined"))
	}

	hcl, err := os.ReadFile("../../../docker/atlas/opencoze_latest_schema.hcl")
	require.NoError(t, err)
	executions := hclNamedBlock(t, string(hcl), "table", "appdev_provider_executions")
	require.Contains(t, hclNamedBlock(t, executions, "column", "launch_state"), "type    = varchar(24)")
}
