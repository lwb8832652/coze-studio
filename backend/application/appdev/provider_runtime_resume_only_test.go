// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestProviderRuntimeResumeOnlyAbortsExpiredPreparedLaunchWithoutProviderCall(t *testing.T) {
	_, recovered, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedPending
	ledger.metadata.LaunchState = domainappdev.ProviderExecutionLaunchPrepared
	ledger.recoveryID = ""
	ledger.hasCheckpoint = false

	projection, err := recovered.RecoverProjectResumeOnly(context.Background(), ProviderRuntimeRecoverProjectInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject,
	})
	require.NoError(t, err)
	require.NotNil(t, projection)
	require.Equal(t, ProviderRuntimeStatePending, projection.State)
	require.Zero(t, router.resolveCount)
	require.Zero(t, router.resumeCount)
	require.Zero(t, router.lookupCount)
	require.Empty(t, selection.executeRequests)
	require.Equal(t, 1, ledger.claimCount)
	require.Equal(t, 1, ledger.releaseCount)
	require.Equal(t, domainappdev.ProviderExecutionLaunchPrepared, ledger.abortRequest.ExpectedState)
}

func TestProviderRuntimeResumeOnlySubmittedBeforeSendUsesAuthoritativeLookupNotExecute(t *testing.T) {
	_, recovered, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
	ledger.metadata.LaunchState = domainappdev.ProviderExecutionLaunchSubmitted
	ledger.recoveryID = ""
	ledger.hasCheckpoint = false
	router.lookupResult = ProviderRuntimeLaunchLookupResult{Status: infrasandbox.ExecutionLookupNotFound}

	projection, err := recovered.RecoverProjectResumeOnly(context.Background(), runtimeRecoveryInput())
	require.NoError(t, err)
	require.NotNil(t, projection)
	require.Equal(t, 1, router.lookupCount)
	require.Equal(t, ledger.launchProviderOperationID, router.lookupRequest.OperationID)
	require.Equal(t, ledger.launchRequestDigest, router.lookupRequest.RequestDigest)
	require.Zero(t, router.resolveCount)
	require.Zero(t, router.resumeCount)
	require.Empty(t, selection.executeRequests)
	require.Equal(t, domainappdev.ProviderExecutionLaunchSubmitted, ledger.abortRequest.ExpectedState)
	require.Equal(t, 1, ledger.releaseCount)
}

func TestProviderRuntimeResumeOnlyAcceptedBeforeSaveCompletesDurableSubmissionFromLookup(t *testing.T) {
	_, recovered, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
	ledger.metadata.LaunchState = domainappdev.ProviderExecutionLaunchSubmitted
	ledger.recoveryID = ""
	ledger.hasCheckpoint = false
	router.lookupResult = ProviderRuntimeLaunchLookupResult{
		Status: infrasandbox.ExecutionLookupFound,
		Execution: infrasandbox.ExecuteResult{
			ExecutionID: "provider-execution-reconciled", Status: infrasandbox.ExecutionStatusRunning,
			PreviewRoute: "/preview/reconciled",
		},
		Selection: selection,
	}

	projection, err := recovered.RecoverProjectResumeOnly(context.Background(), runtimeRecoveryInput())
	require.NoError(t, err)
	require.NotNil(t, projection)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, 1, router.lookupCount)
	require.Equal(t, 1, ledger.completeReconciledCount)
	require.Equal(t, "provider-execution-reconciled", ledger.completeReconciledRequest.ProviderExecutionID)
	require.Equal(t, ledger.startOperationID, ledger.completeReconciledRequest.LaunchOperationID)
	require.Zero(t, router.resolveCount)
	require.Empty(t, selection.executeRequests)
	require.Zero(t, ledger.releaseCount)
}

func TestProviderRuntimeResumeOnlyUnknownLookupRetainsOwnerForLeaseBackoff(t *testing.T) {
	_, recovered, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
	ledger.metadata.LaunchState = domainappdev.ProviderExecutionLaunchSubmitted
	ledger.recoveryID = ""
	ledger.hasCheckpoint = false
	ledger.launchRequestDigest = infrasandbox.ExecutionRequestDigest(sha256.Sum256([]byte("unknown launch")))
	router.lookupResult = ProviderRuntimeLaunchLookupResult{Status: infrasandbox.ExecutionLookupUnknown}
	router.lookupErr = errors.New("provider lookup unavailable with secret")

	projection, err := recovered.RecoverProjectResumeOnly(context.Background(), runtimeRecoveryInput())
	require.ErrorIs(t, err, ErrProviderRuntimeUnavailable)
	require.True(t, IsProviderRuntimeRecoveryOwnerRetained(err))
	require.NotNil(t, projection)
	require.True(t, projection.Recovering)
	require.Equal(t, ProviderRuntimeRecoveryOwnershipRetained, projection.RecoveryOwnership())
	require.Equal(t, 1, router.lookupCount)
	require.Zero(t, ledger.abortRequest.OperationID)
	require.Zero(t, ledger.releaseCount)
	require.Empty(t, selection.executeRequests)
	require.NotContains(t, err.Error(), "secret")
}

func TestProviderRuntimeResumeOnlyResumesSubmittingExecutionWithHandleAndCheckpoint(t *testing.T) {
	_, recovered, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
	ledger.metadata.LaunchState = domainappdev.ProviderExecutionLaunchComplete
	ledger.recoveryID = "provider-execution-real"
	ledger.hasCheckpoint = true
	selection.statusResults = []infrasandbox.ExecuteResult{{
		ExecutionID: "provider-execution-real",
		Status:      infrasandbox.ExecutionStatusRunning,
	}}

	projection, err := recovered.RecoverProjectResumeOnly(context.Background(), ProviderRuntimeRecoverProjectInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject,
	})
	require.NoError(t, err)
	require.NotNil(t, projection)
	require.Equal(t, uint64(7), projection.Generation)
	require.Equal(t, 1, router.resumeCount)
	require.Zero(t, router.resolveCount)
	require.Empty(t, selection.executeRequests)
	require.Zero(t, ledger.releaseCount)
}

func TestProviderRuntimeResumeOnlyReleasesClaimAfterRecoveryLoadFailure(t *testing.T) {
	_, recovered, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedRunning
	ledger.recoveryID = "provider-execution-real"
	ledger.hasCheckpoint = true
	ledger.loadErr = errors.New("encrypted recovery unavailable")

	projection, err := recovered.RecoverProjectResumeOnly(context.Background(), runtimeRecoveryInput())
	require.ErrorIs(t, err, ErrProviderRuntimeUnavailable)
	require.NotNil(t, projection)
	require.True(t, projection.Recovering)
	require.Equal(t, 1, ledger.claimCount)
	require.Equal(t, 1, ledger.releaseCount)
	require.Equal(t, ProviderRuntimeRecoveryOwnershipReleased, projection.RecoveryOwnership())
	require.Zero(t, router.resumeCount)
	require.Empty(t, selection.executeRequests)
}

func TestProviderRuntimeResumeOnlyJoinsOwnerReleaseFailure(t *testing.T) {
	_, recovered, ledger, router, _ := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedRunning
	ledger.recoveryID = "provider-execution-real"
	ledger.hasCheckpoint = true
	router.resumeErr = errors.New("remote resume unavailable")
	ledger.releaseErr = errors.New("owner release unavailable")

	projection, err := recovered.RecoverProjectResumeOnly(context.Background(), runtimeRecoveryInput())
	require.NotNil(t, projection)
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider runtime")
	require.NotContains(t, err.Error(), "remote resume unavailable")
	require.NotContains(t, err.Error(), "owner release unavailable")
	require.Equal(t, 1, ledger.releaseCount)
}
