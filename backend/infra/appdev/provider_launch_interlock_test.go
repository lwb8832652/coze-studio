// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type providerLaunchInterlockFixture struct {
	*providerArchiveBarrierFixture

	execution           *domainappdev.ProviderExecution
	ownerHash           domainappdev.ProviderExecutionOwnerHash
	ownerCAS            domainappdev.ProviderExecutionOwnerCAS
	operationID         string
	operationHash       domainappdev.ProviderExecutionOperationHash
	providerOperationID string
	requestDigest       domainappdev.ProviderExecutionLaunchRequestDigest
	clock               *providerExecutionControlledDBClock
}

func newProviderLaunchInterlockFixture(t *testing.T, databaseName string) *providerLaunchInterlockFixture {
	t.Helper()
	archive := newProviderArchiveBarrierFixture(t, databaseName)
	now := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	clock := &providerExecutionControlledDBClock{
		now: now,
	}
	archive.executions.clock = clock
	require.NoError(t, archive.db.
		Where("space_id = ? AND project_id = ?", 1001, archive.project.ID).
		Delete(&providerExecutionRecord{}).Error)

	const operationID = "launch-interlock-operation"
	pendingRecord := &providerExecutionRecord{
		ID: "launch-interlock-execution", SpaceID: 1001, ProjectID: archive.project.ID,
		Generation: 1, IdempotencyKey: operationID,
		DesiredState: domainappdev.ProviderExecutionDesiredRun, ObservedState: domainappdev.ProviderExecutionObservedPending,
		ProviderKey: "provider-a", ProviderScope: "appdev", ProviderExecutionID: "",
		LaunchState: domainappdev.ProviderExecutionLaunchNone, ArtifactStatus: domainappdev.ProviderExecutionArtifactNone,
		Version: domainappdev.ProviderExecutionInitialVersion, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, archive.db.Create(pendingRecord).Error)
	execution, err := providerExecutionRecordToDomain(pendingRecord)
	require.NoError(t, err)
	ownerHash := testOwnerHash("launch-interlock-owner")
	claimed, err := archive.executions.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: execution.SpaceID, ProjectID: execution.ProjectID,
		Generation: execution.Generation, ExpectedVersion: execution.Version,
		OwnerHash: ownerHash, LeaseDuration: time.Minute,
	})
	require.NoError(t, err)
	ownerCAS := providerExecutionCAS(claimed, ownerHash, time.Time{})
	operationHash, err := domainappdev.HashProviderExecutionLaunchOperationID(operationID, ownerCAS)
	require.NoError(t, err)
	requestDigest := domainappdev.ProviderExecutionLaunchRequestDigest(
		sha256.Sum256([]byte("canonical provider launch envelope")),
	)
	return &providerLaunchInterlockFixture{
		providerArchiveBarrierFixture: archive,
		execution:                     claimed, ownerHash: ownerHash, ownerCAS: ownerCAS,
		operationID: operationID, operationHash: operationHash,
		providerOperationID: "appdev-start-provider-operation", requestDigest: requestDigest,
		clock: clock,
	}
}

func (fixture *providerLaunchInterlockFixture) startSubmissionInput() domainappdev.StartProviderSubmissionInput {
	return domainappdev.StartProviderSubmissionInput{
		OwnerCAS: fixture.ownerCAS, OperationHash: fixture.operationHash,
		ProviderOperationID: fixture.providerOperationID, RequestDigest: fixture.requestDigest,
		LeaseDuration: time.Minute,
	}
}

func (fixture *providerLaunchInterlockFixture) markSubmittedInput(record *domainappdev.ProviderExecution) domainappdev.MarkProviderLaunchSubmittedInput {
	return domainappdev.MarkProviderLaunchSubmittedInput{
		OwnerCAS:              providerExecutionCAS(record, fixture.ownerHash, time.Time{}),
		OperationHash:         fixture.operationHash,
		DispatchLeaseDuration: time.Minute,
	}
}

func (fixture *providerLaunchInterlockFixture) archiveInput() domainappdev.ReserveProjectArchiveInput {
	return domainappdev.ReserveProjectArchiveInput{
		SpaceID: fixture.project.SpaceID, ProjectID: fixture.project.ID,
		ExpectedSourceVersion: fixture.project.SourceVersion,
		RuntimeGeneration:     fixture.execution.Generation,
	}
}

func TestProviderLaunchInterlockBlocksArchiveUntilCheckpointIsDurable(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-launch-interlock")
	ctx := context.Background()

	prepared, err := fixture.executions.StartSubmission(ctx, fixture.startSubmissionInput())
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchPrepared, prepared.LaunchState)
	require.Equal(t, domainappdev.ProviderExecutionObservedPending, prepared.ObservedState)
	require.True(t, fixture.operationHash.Equal(prepared.LaunchOperationHash))
	require.Equal(t, fixture.providerOperationID, prepared.LaunchProviderOperationID)
	require.True(t, fixture.requestDigest.Equal(prepared.LaunchRequestDigest))
	require.NotNil(t, prepared.LaunchExpiresAt)

	retry, err := fixture.executions.StartSubmission(ctx, fixture.startSubmissionInput())
	require.NoError(t, err)
	require.Equal(t, prepared.Version, retry.Version)
	require.True(t, fixture.operationHash.Equal(retry.LaunchOperationHash))

	_, err = fixture.storeB.ReserveProjectArchive(ctx, fixture.archiveInput())
	require.ErrorIs(t, err, domainappdev.ErrProjectArchiveConflict)

	submitting, err := fixture.executions.MarkSubmissionSubmitted(ctx, fixture.markSubmittedInput(prepared))
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchSubmitted, submitting.LaunchState)
	require.Equal(t, domainappdev.ProviderExecutionObservedSubmitting, submitting.ObservedState)

	saved, err := fixture.executions.SaveSubmission(ctx, domainappdev.SaveProviderSubmissionInput{
		OwnerCAS:            providerExecutionCAS(submitting, fixture.ownerHash, time.Time{}),
		OperationHash:       fixture.operationHash,
		ProviderExecutionID: "provider-launch-real", ObservedState: domainappdev.ProviderExecutionObservedRunning,
		ProviderLeaseDuration: time.Minute,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchSubmitted, saved.LaunchState)
	_, err = fixture.storeB.ReserveProjectArchive(ctx, fixture.archiveInput())
	require.ErrorIs(t, err, domainappdev.ErrProjectArchiveConflict)

	checkpointOperation := testOperationHash(t, "launch-checkpoint-operation")
	reservation, err := fixture.executions.ReserveCheckpointWrite(ctx, domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS:      providerExecutionCAS(saved, fixture.ownerHash, time.Time{}),
		OperationHash: checkpointOperation, ReservationDuration: time.Minute,
	})
	require.NoError(t, err)
	checkpointCAS := providerExecutionCAS(saved, fixture.ownerHash, time.Time{})
	checkpointCAS.ExpectedVersion = reservation.ReservedVersion
	checkpointed, err := fixture.executions.SaveCheckpoint(ctx, domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: checkpointCAS, OperationHash: checkpointOperation,
		CheckpointWriteRevision: reservation.Revision,
		CheckpointEnvelope:      "ecp1:launch-interlock",
		LaunchOperationHash:     fixture.operationHash,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchComplete, checkpointed.LaunchState)
	require.Nil(t, checkpointed.LaunchExpiresAt)

	intent, err := fixture.storeB.ReserveProjectArchive(ctx, fixture.archiveInput())
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProjectArchiveStateArchiving, intent.State)
}

func TestProviderLaunchInterlockSurvivesLeaseExpiryAndCanBeReclaimed(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-launch-interlock-reclaim")
	ctx := context.Background()
	prepared, err := fixture.executions.StartSubmission(ctx, fixture.startSubmissionInput())
	require.NoError(t, err)
	fixture.clock.set(time.Date(2026, 7, 17, 16, 2, 0, 0, time.UTC))
	require.NoError(t, fixture.db.Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, fixture.project.ID, prepared.Generation).
		Updates(map[string]any{
			"owner_expires_at":  time.Date(2026, 7, 17, 16, 1, 0, 0, time.UTC),
			"launch_expires_at": time.Date(2026, 7, 17, 16, 1, 0, 0, time.UTC),
		}).Error)

	_, err = fixture.storeA.ReserveProjectArchive(ctx, fixture.archiveInput())
	require.ErrorIs(t, err, domainappdev.ErrProjectArchiveConflict)

	replacementHash := testOwnerHash("launch-interlock-replacement-owner")
	reclaimed, err := fixture.executions.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: prepared.SpaceID, ProjectID: prepared.ProjectID,
		Generation: prepared.Generation, ExpectedVersion: prepared.Version,
		OwnerHash: replacementHash, LeaseDuration: time.Minute,
	})
	require.NoError(t, err)
	aborted, err := fixture.executions.AbortLaunch(ctx, domainappdev.AbortProviderLaunchInput{
		OwnerCAS:      providerExecutionCAS(reclaimed, replacementHash, time.Time{}),
		OperationHash: fixture.operationHash, ExpectedState: domainappdev.ProviderExecutionLaunchPrepared,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchAborted, aborted.LaunchState)
	_, err = fixture.storeA.ReserveProjectArchive(ctx, fixture.archiveInput())
	require.NoError(t, err)
}

func TestProviderLaunchArchiveTOCTOUUsesCommittedLaunchIntent(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-launch-interlock-toctou")
	launchCommitted := make(chan struct{})
	allowRemoteExecute := make(chan struct{})
	var hookOnce sync.Once
	fixture.executions.startSubmissionCommittedHook = func() {
		hookOnce.Do(func() { close(launchCommitted) })
		<-allowRemoteExecute
	}

	submissionResult := make(chan *domainappdev.ProviderExecution, 1)
	submissionError := make(chan error, 1)
	go func() {
		record, err := fixture.executions.StartSubmission(context.Background(), fixture.startSubmissionInput())
		submissionResult <- record
		submissionError <- err
	}()
	<-launchCommitted

	_, archiveErr := fixture.storeB.ReserveProjectArchive(context.Background(), fixture.archiveInput())
	require.ErrorIs(t, archiveErr, domainappdev.ErrProjectArchiveConflict)

	close(allowRemoteExecute)
	require.NoError(t, <-submissionError)
	require.NotNil(t, <-submissionResult)
}

func TestProviderLaunchDefiniteFailureAbortIsOwnerAndOperationFenced(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-launch-interlock-abort")
	ctx := context.Background()
	prepared, err := fixture.executions.StartSubmission(ctx, fixture.startSubmissionInput())
	require.NoError(t, err)
	submitting, err := fixture.executions.MarkSubmissionSubmitted(ctx, fixture.markSubmittedInput(prepared))
	require.NoError(t, err)

	wrongHash, err := domainappdev.HashProviderExecutionOperationID("different-launch-operation")
	require.NoError(t, err)
	_, err = fixture.executions.AbortLaunch(ctx, domainappdev.AbortProviderLaunchInput{
		OwnerCAS:      providerExecutionCAS(submitting, fixture.ownerHash, time.Time{}),
		OperationHash: wrongHash,
		ExpectedState: domainappdev.ProviderExecutionLaunchSubmitted,
	})
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionOperationConflict)

	aborted, err := fixture.executions.AbortLaunch(ctx, domainappdev.AbortProviderLaunchInput{
		OwnerCAS:      providerExecutionCAS(submitting, fixture.ownerHash, time.Time{}),
		OperationHash: fixture.operationHash,
		ExpectedState: domainappdev.ProviderExecutionLaunchSubmitted,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchAborted, aborted.LaunchState)
	require.Equal(t, domainappdev.ProviderExecutionObservedPending, aborted.ObservedState)
	require.Nil(t, aborted.SubmissionStartedAt)
	require.Empty(t, aborted.ProviderExecutionID)

	intent, err := fixture.storeB.ReserveProjectArchive(ctx, fixture.archiveInput())
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProjectArchiveStateArchiving, intent.State)
}

func TestProviderLaunchSubmittedCannotUsePreparedCrashAbort(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-launch-interlock-submitted-fence")
	prepared, err := fixture.executions.StartSubmission(context.Background(), fixture.startSubmissionInput())
	require.NoError(t, err)
	submitted, err := fixture.executions.MarkSubmissionSubmitted(context.Background(), fixture.markSubmittedInput(prepared))
	require.NoError(t, err)

	_, err = fixture.executions.AbortLaunch(context.Background(), domainappdev.AbortProviderLaunchInput{
		OwnerCAS:      providerExecutionCAS(submitted, fixture.ownerHash, time.Time{}),
		OperationHash: fixture.operationHash, ExpectedState: domainappdev.ProviderExecutionLaunchPrepared,
	})
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionStateConflict)
}

func TestProviderLaunchActiveDispatchLeaseRejectsDesiredStop(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-launch-interlock-active-dispatch-stop")
	ctx := context.Background()
	prepared, err := fixture.executions.StartSubmission(ctx, fixture.startSubmissionInput())
	require.NoError(t, err)
	submitted, err := fixture.executions.MarkSubmissionSubmitted(ctx, fixture.markSubmittedInput(prepared))
	require.NoError(t, err)
	require.NotNil(t, submitted.LaunchExpiresAt)
	require.True(t, submitted.LaunchExpiresAt.After(fixture.clock.now))

	_, err = fixture.executions.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(submitted, fixture.ownerHash, time.Time{}),
	})
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionStateConflict)

	current, err := fixture.executions.LoadCurrent(ctx, domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: submitted.SpaceID, ProjectID: submitted.ProjectID,
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionDesiredRun, current.DesiredState)
	require.Equal(t, domainappdev.ProviderExecutionLaunchSubmitted, current.LaunchState)
	require.Empty(t, current.ProviderExecutionID)
}

func TestProviderLaunchExpiredDispatchIsClaimedBeforeLookupAndHandleSave(t *testing.T) {
	fixture := newProviderLaunchInterlockFixture(t, "provider-launch-interlock-reconcile-claim")
	ctx := context.Background()
	prepared, err := fixture.executions.StartSubmission(ctx, fixture.startSubmissionInput())
	require.NoError(t, err)
	submitted, err := fixture.executions.MarkSubmissionSubmitted(ctx, fixture.markSubmittedInput(prepared))
	require.NoError(t, err)
	originalEpoch := submitted.OwnerEpoch
	fixture.clock.set(fixture.clock.now.Add(2 * time.Minute))

	claimed, err := fixture.executions.ClaimLaunchReconciliation(
		ctx,
		domainappdev.ClaimProviderLaunchReconciliationInput{
			SpaceID: submitted.SpaceID, ProjectID: submitted.ProjectID,
			Generation: submitted.Generation, ExpectedVersion: submitted.Version,
			OwnerHash: fixture.ownerHash, ExpectedOwnerEpoch: submitted.OwnerEpoch,
			ExpectedState: domainappdev.ProviderExecutionLaunchSubmitted,
			LeaseDuration: time.Minute, SetDesiredStop: true,
		},
	)
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionDesiredStop, claimed.DesiredState)
	require.Greater(t, claimed.OwnerEpoch, originalEpoch)
	require.NotNil(t, claimed.LaunchExpiresAt)
	require.True(t, claimed.LaunchExpiresAt.After(fixture.clock.now))

	_, err = fixture.executions.ClaimLaunchReconciliation(
		ctx,
		domainappdev.ClaimProviderLaunchReconciliationInput{
			SpaceID: claimed.SpaceID, ProjectID: claimed.ProjectID,
			Generation: claimed.Generation, ExpectedVersion: claimed.Version,
			OwnerHash: fixture.ownerHash, ExpectedOwnerEpoch: claimed.OwnerEpoch,
			ExpectedState: domainappdev.ProviderExecutionLaunchSubmitted,
			LeaseDuration: time.Minute, SetDesiredStop: true,
		},
	)
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionStateConflict)

	saved, err := fixture.executions.SaveSubmission(ctx, domainappdev.SaveProviderSubmissionInput{
		OwnerCAS:              providerExecutionCAS(claimed, fixture.ownerHash, time.Time{}),
		OperationHash:         fixture.operationHash,
		ProviderExecutionID:   "provider-launch-reconciled",
		ObservedState:         domainappdev.ProviderExecutionObservedRunning,
		ProviderLeaseDuration: time.Minute,
	})
	require.NoError(t, err)
	require.Equal(t, "provider-launch-reconciled", saved.ProviderExecutionID)
	require.Equal(t, domainappdev.ProviderExecutionDesiredStop, saved.DesiredState)

	checkpointOperation := testOperationHash(t, "provider-launch-reconciled-checkpoint")
	reservation, err := fixture.executions.ReserveCheckpointWrite(
		ctx,
		domainappdev.ReserveProviderCheckpointWriteInput{
			OwnerCAS:      providerExecutionCAS(saved, fixture.ownerHash, time.Time{}),
			OperationHash: checkpointOperation, ReservationDuration: time.Minute,
		},
	)
	require.NoError(t, err)
	checkpointCAS := providerExecutionCAS(saved, fixture.ownerHash, time.Time{})
	checkpointCAS.ExpectedVersion = reservation.ReservedVersion
	completed, err := fixture.executions.SaveCheckpoint(ctx, domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: checkpointCAS, OperationHash: checkpointOperation,
		LaunchOperationHash:     fixture.operationHash,
		CheckpointWriteRevision: reservation.Revision,
		CheckpointEnvelope:      "ecp1:provider-launch-reconciled",
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionLaunchComplete, completed.LaunchState)
	cleanup, err := fixture.executions.BeginCleanup(ctx, domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(completed, fixture.ownerHash, time.Time{}),
	})
	require.NoError(t, err)
	require.Equal(t, domainappdev.ProviderExecutionObservedCleanupPending, cleanup.ObservedState)
}
