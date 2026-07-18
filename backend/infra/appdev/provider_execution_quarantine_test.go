// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestProviderExecutionQuarantineDispositionIsAtomicAuditedAndIdempotent(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-quarantine-disposition")
	createProviderExecutionQuarantineAuditSQLiteTable(t, fixture)
	ctx := context.Background()
	expectedVersion := makeProviderExecutionQuarantined(t, fixture)
	input := providerExecutionQuarantineDispositionInput(t, fixture, expectedVersion, "operator-disposition-1")

	disposed, err := fixture.executions.DisposeQuarantine(ctx, input)
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchAborted, disposed.Execution.LaunchState)
	require.Equal(t, domainappdev.ProviderExecutionDesiredStop, disposed.Execution.DesiredState)
	require.Equal(t, domainappdev.ProviderExecutionObservedPending, disposed.Execution.ObservedState)
	require.Equal(t, expectedVersion+1, disposed.Execution.Version)
	require.Empty(t, disposed.Execution.ProviderExecutionID)
	require.Empty(t, disposed.Execution.CheckpointEnvelope)
	require.True(t, disposed.Execution.OwnerIdentityHash.IsZero())
	require.Equal(t, domainappdev.ProviderExecutionQuarantineDisposedCode, disposed.Execution.SafeErrorCode)
	require.Equal(t, int64(42), disposed.Audit.ActorID)
	require.Equal(t, expectedVersion, disposed.Audit.ExpectedVersion)
	require.Equal(t, disposed.Execution.OwnerEpoch, disposed.Audit.OwnerEpoch)

	retried, err := fixture.executions.DisposeQuarantine(ctx, input)
	require.NoError(t, err)
	require.Equal(t, disposed.Execution.Version, retried.Execution.Version)
	require.Equal(t, disposed.Audit.ID, retried.Audit.ID)
	var audits int64
	require.NoError(t, fixture.db.Model(&providerExecutionQuarantineAuditRecord{}).Count(&audits).Error)
	require.Equal(t, int64(1), audits)

	different := input
	different.OperationID = "operator-disposition-2"
	_, err = fixture.executions.DisposeQuarantine(ctx, different)
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionConflict)

	cleanupOwner := testOwnerHash("quarantine-cleanup-owner")
	claimed, err := fixture.executions.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: disposed.Execution.SpaceID, ProjectID: disposed.Execution.ProjectID,
		Generation: disposed.Execution.Generation, ExpectedVersion: disposed.Execution.Version,
		OwnerHash: cleanupOwner, LeaseDuration: time.Minute,
	})
	require.NoError(t, err)
	pending, err := fixture.executions.BeginCleanup(ctx, domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(claimed, cleanupOwner, time.Time{}),
	})
	require.NoError(t, err)
	completed, err := fixture.executions.CompleteCleanup(ctx, domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS:    providerExecutionCAS(pending, cleanupOwner, time.Time{}),
		OperationID: "quarantine-cleanup-operation",
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionObservedCleanupComplete, completed.ObservedState)

	archiveInput := fixture.archiveInput()
	archiveInput.RuntimeGeneration = 0
	_, err = fixture.storeB.ReserveProjectArchive(ctx, archiveInput)
	require.NoError(t, err, "disposed launch must no longer hold the archive launch interlock")
}

func TestProviderExecutionQuarantineDispositionRejectsStaleOwnerVersionAndCrossExecutionReplay(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-quarantine-disposition-fences")
	createProviderExecutionQuarantineAuditSQLiteTable(t, fixture)
	expectedVersion := makeProviderExecutionQuarantined(t, fixture)
	input := providerExecutionQuarantineDispositionInput(t, fixture, expectedVersion, "operator-disposition-fence")

	stale := input
	stale.ExpectedVersion--
	_, err := fixture.executions.DisposeQuarantine(context.Background(), stale)
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionVersionConflict)

	activeOwner := testOwnerHash("active-owner")
	require.NoError(t, fixture.db.Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, fixture.project.ID, fixture.execution.Generation).
		Updates(map[string]any{
			"owner_identity_hash": activeOwner.Bytes(),
			"owner_expires_at":    fixture.clock.now.Add(time.Minute),
		}).Error)
	_, err = fixture.executions.DisposeQuarantine(context.Background(), input)
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionOwnerConflict)

	require.NoError(t, fixture.db.Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, fixture.project.ID, fixture.execution.Generation).
		Updates(map[string]any{"owner_identity_hash": nil, "owner_expires_at": nil}).Error)
	other := input
	other.ProjectID = "project-other"
	_, err = fixture.executions.DisposeQuarantine(context.Background(), other)
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionConflict)
}

func createProviderExecutionQuarantineAuditSQLiteTable(
	t *testing.T,
	fixture *providerLaunchInterlockFixture,
) {
	t.Helper()
	require.NoError(t, fixture.db.Exec(`
CREATE TABLE appdev_provider_execution_quarantine_audits (
  id TEXT PRIMARY KEY,
  execution_id TEXT NOT NULL,
  space_id INTEGER NOT NULL,
  project_id TEXT NOT NULL,
  generation INTEGER NOT NULL,
  expected_version INTEGER NOT NULL,
  owner_identity_hash BLOB NOT NULL,
  owner_epoch INTEGER NOT NULL,
  actor_id INTEGER NOT NULL,
  operation_hash BLOB NOT NULL,
  acknowledgement TEXT NOT NULL,
  reason TEXT NOT NULL,
  evidence_hash BLOB NOT NULL,
  created_at datetime NOT NULL,
  UNIQUE (space_id, project_id, generation, operation_hash)
)`).Error)
}

func TestProviderExecutionQuarantineForwardMigrationCorrectsCheckpointWithoutHandle(t *testing.T) {
	migration, err := os.ReadFile("../../../docker/atlas/migrations/20260717000700_appdev_provider_quarantine_disposition.sql")
	require.NoError(t, err)
	sql := string(migration)
	require.Contains(t, sql, "CREATE TABLE `appdev_provider_execution_quarantine_audits`")
	require.Contains(t, sql, "`launch_state` = 'legacy_submitted'")
	require.Contains(t, sql, "`provider_execution_id` = ''")
	require.Contains(t, sql, "`checkpoint_envelope` <> ''")
	require.Contains(t, sql, "`launch_state` = 'quarantined'")
	require.Contains(t, sql, "legacy_launch_quarantined")
	require.NotContains(t, sql, "`launch_state` = 'aborted'")

	historical, err := os.ReadFile("../../../docker/atlas/migrations/20260717000600_appdev_provider_legacy_launch_recovery.sql")
	require.NoError(t, err)
	require.NotContains(t, string(historical), "appdev_provider_execution_quarantine_audits")

	hcl, err := os.ReadFile("../../../docker/atlas/opencoze_latest_schema.hcl")
	require.NoError(t, err)
	audits := hclNamedBlock(t, string(hcl), "table", "appdev_provider_execution_quarantine_audits")
	require.Contains(t, audits, `column "operation_hash"`)
	require.Contains(t, audits, `column "evidence_hash"`)
	require.Contains(t, audits, `index "uk_appdev_provider_quarantine_audit_operation"`)
	require.Regexp(t, `(?m)^\s*schema\s*=\s*schema\.opencoze\s*$`, audits)
}

func TestAppDevCanonicalSchemaReferencesDeclaredSchemas(t *testing.T) {
	hcl, err := os.ReadFile("../../../docker/atlas/opencoze_latest_schema.hcl")
	require.NoError(t, err)
	text := string(hcl)

	declared := make(map[string]struct{})
	for _, match := range regexp.MustCompile(`(?m)^schema\s+"([^"]+)"\s*\{`).FindAllStringSubmatch(text, -1) {
		declared[match[1]] = struct{}{}
	}
	require.NotEmpty(t, declared)

	references := regexp.MustCompile(`\bschema\.([A-Za-z][A-Za-z0-9_]*)\b`).FindAllStringSubmatch(text, -1)
	require.NotEmpty(t, references)
	for _, reference := range references {
		_, ok := declared[reference[1]]
		require.Truef(t, ok, "canonical HCL references undeclared schema %q", reference[1])
	}
}

func makeProviderExecutionQuarantined(t *testing.T, fixture *providerLaunchInterlockFixture) uint64 {
	t.Helper()
	now := fixture.clock.now
	checkpointHash := testOperationHash(t, "legacy-quarantine-checkpoint")
	require.NoError(t, fixture.db.Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, fixture.project.ID, fixture.execution.Generation).
		Updates(map[string]any{
			"desired_state":                   domainappdev.ProviderExecutionDesiredRun,
			"observed_state":                  domainappdev.ProviderExecutionObservedSubmitting,
			"submission_started_at":           now,
			"launch_state":                    domainappdev.ProviderExecutionLaunchQuarantined,
			"launch_operation_hash":           nil,
			"launch_provider_operation_id":    "",
			"launch_request_digest":           nil,
			"launch_expires_at":               nil,
			"checkpoint_envelope":             "ecp1:legacy-quarantine",
			"checkpoint_write_revision":       1,
			"checkpoint_last_operation_hash":  checkpointHash.Bytes(),
			"checkpoint_write_pending":        false,
			"checkpoint_write_operation_hash": nil,
			"checkpoint_write_expires_at":     nil,
			"provider_execution_id":           "",
			"provider_lease_expires_at":       nil,
			"owner_identity_hash":             nil,
			"owner_expires_at":                nil,
			"safe_error_code":                 "legacy_launch_quarantined",
			"safe_error_message":              "provider launch requires operator recovery",
			"updated_at":                      now,
		}).Error)
	var record providerExecutionRecord
	require.NoError(t, fixture.db.
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, fixture.project.ID, fixture.execution.Generation).
		Take(&record).Error)
	_, err := providerExecutionRecordToDomain(&record)
	require.NoError(t, err)
	return record.Version
}

func providerExecutionQuarantineDispositionInput(
	t *testing.T,
	fixture *providerLaunchInterlockFixture,
	expectedVersion uint64,
	operationID string,
) domainappdev.DisposeProviderExecutionQuarantineInput {
	t.Helper()
	evidence := testOperationHash(t, "operator-evidence")
	return domainappdev.DisposeProviderExecutionQuarantineInput{
		SpaceID: fixture.execution.SpaceID, ProjectID: fixture.execution.ProjectID,
		Generation: fixture.execution.Generation, ExpectedVersion: expectedVersion,
		ExpectedState: domainappdev.ProviderExecutionLaunchQuarantined,
		OwnerHash:     testOwnerHash("operator-disposition-owner"), ActorID: 42,
		OperationID:     operationID,
		Acknowledgement: domainappdev.ProviderExecutionQuarantineProviderAbsent,
		Reason:          domainappdev.ProviderExecutionQuarantineReasonProviderAbsent,
		EvidenceHash:    evidence,
	}
}

func TestProviderExecutionQuarantineMigrationFixtureHydratesCheckpointBoundary(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput(
		"legacy-checkpoint-quarantine", "legacy-checkpoint-operation", time.Now().UTC(),
	))
	require.NoError(t, err)
	checkpointHash := testOperationHash(t, "legacy-checkpoint-hash")
	now := time.Now().UTC()
	require.NoError(t, db.Model(&providerExecutionRecord{}).Where("id = ?", record.ID).Updates(map[string]any{
		"launch_state":                   domainappdev.ProviderExecutionLaunchQuarantined,
		"observed_state":                 domainappdev.ProviderExecutionObservedSubmitting,
		"submission_started_at":          now,
		"launch_operation_hash":          nil,
		"launch_provider_operation_id":   "",
		"launch_request_digest":          nil,
		"checkpoint_envelope":            "ecp1:legacy-checkpoint",
		"checkpoint_write_revision":      1,
		"checkpoint_last_operation_hash": checkpointHash.Bytes(),
		"safe_error_code":                "legacy_launch_quarantined",
		"safe_error_message":             "provider launch requires operator recovery",
	}).Error)
	loaded, err := repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-a",
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchQuarantined, loaded.LaunchState)
	require.NotEmpty(t, loaded.CheckpointEnvelope)
	require.True(t, strings.HasPrefix(loaded.SafeErrorCode, "legacy_launch_"))
}
