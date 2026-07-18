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
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type runtimeRecoveryLedgerFake struct {
	*runtimeStopLedgerFake

	currentOwner              domainappdev.ProviderExecutionOwnerToken
	leaseExpired              bool
	startOperationID          string
	launchProviderOperationID string
	launchRequestDigest       infrasandbox.ExecutionRequestDigest
	releaseCount              int
	releaseErr                error
	completeReconciledCount   int
	completeReconciledRequest CompleteReconciledProviderLaunchRequest
}

func (fake *runtimeRecoveryLedgerFake) ClaimLaunchReconciliation(
	_ context.Context,
	request ClaimProviderExecutionLaunchReconciliationRequest,
) (*ClaimedProviderExecution, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.claimCount++
	if request.ExpectedVersion != fake.metadata.Version ||
		request.ExpectedState != fake.metadata.LaunchState {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	candidate := request.ExistingOwner
	if candidate.IsZero() {
		candidate = request.ProposedOwner
	}
	if candidate.IsZero() {
		return nil, domainappdev.ErrProviderExecutionOwnerConflict
	}
	if fake.metadata.OwnerEpoch == 0 {
		fake.metadata.OwnerEpoch = 1
	} else if candidate != fake.currentOwner {
		if !fake.leaseExpired {
			return nil, domainappdev.ErrProviderExecutionOwnerConflict
		}
		fake.metadata.OwnerEpoch++
	}
	fake.currentOwner = candidate
	fake.leaseExpired = false
	fake.metadata.Version++
	if request.SetDesiredStop {
		fake.metadata.DesiredState = domainappdev.ProviderExecutionDesiredStop
		*fake.events = append(*fake.events, "desired-stop")
	}
	fake.owner = ProviderExecutionOwner{
		spaceID: fake.metadata.SpaceID, projectID: fake.metadata.ProjectID,
		generation: fake.metadata.Generation, providerKey: fake.metadata.ProviderKey,
		providerScope: fake.metadata.ProviderScope, token: candidate, epoch: fake.metadata.OwnerEpoch,
	}
	copy := fake.metadata
	return &ClaimedProviderExecution{Metadata: copy, owner: fake.owner}, nil
}

func (fake *runtimeRecoveryLedgerFake) ListRecoverable(
	_ context.Context,
	request ListRecoverableProviderExecutionsRequest,
) ([]ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if request.SpaceID != fake.metadata.SpaceID || request.ProjectID != fake.metadata.ProjectID {
		return nil, domainappdev.ErrProviderExecutionNotFound
	}
	metadata := fake.metadata
	metadata.HasProviderExecution = fake.recoveryID != ""
	metadata.HasCheckpoint = fake.hasCheckpoint
	return []ProviderExecutionMetadata{metadata}, nil
}

func (fake *runtimeRecoveryLedgerFake) ReleaseOwner(
	_ context.Context,
	request ReleaseProviderExecutionOwnerRequest,
) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.releaseCount++
	if fake.releaseErr != nil {
		return nil, fake.releaseErr
	}
	if request.Owner.token != fake.currentOwner ||
		request.Owner.epoch != fake.metadata.OwnerEpoch ||
		request.ExpectedVersion != fake.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionOwnerConflict
	}
	fake.metadata.Version++
	fake.metadata.OwnerEpoch = request.Owner.epoch
	fake.currentOwner = domainappdev.ProviderExecutionOwnerToken{}
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeRecoveryLedgerFake) RenewOwner(_ context.Context, request RenewProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.renewCount++
	if request.Owner.token != fake.currentOwner || request.Owner.epoch != fake.metadata.OwnerEpoch {
		return nil, domainappdev.ErrProviderExecutionOwnerConflict
	}
	if request.ExpectedVersion != fake.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	fake.metadata.Version++
	*fake.events = append(*fake.events, "owner-renewed")
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeRecoveryLedgerFake) ClaimRecovery(_ context.Context, request ClaimProviderExecutionRecoveryRequest) (*ClaimedProviderExecution, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.claimCount++
	if fake.claimErr != nil {
		return nil, fake.claimErr
	}
	if request.ExpectedVersion != fake.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	candidate := request.ProposedOwner
	if candidate.IsZero() {
		candidate = request.ExistingOwner
	}
	if candidate.IsZero() {
		return nil, domainappdev.ErrProviderExecutionOwnerConflict
	}
	if fake.metadata.OwnerEpoch == 0 {
		fake.metadata.OwnerEpoch = 1
	} else if candidate != fake.currentOwner {
		if !fake.leaseExpired {
			return nil, domainappdev.ErrProviderExecutionOwnerConflict
		}
		fake.metadata.OwnerEpoch++
	} else if request.ExistingOwnerEpoch != 0 && request.ExistingOwnerEpoch != fake.metadata.OwnerEpoch {
		return nil, domainappdev.ErrProviderExecutionOwnerConflict
	}
	fake.currentOwner = candidate
	fake.leaseExpired = false
	fake.metadata.Version++
	fake.owner = ProviderExecutionOwner{
		spaceID: fake.metadata.SpaceID, projectID: fake.metadata.ProjectID, generation: fake.metadata.Generation,
		providerKey: fake.metadata.ProviderKey, providerScope: fake.metadata.ProviderScope,
		token: candidate, epoch: fake.metadata.OwnerEpoch,
	}
	*fake.events = append(*fake.events, "claim")
	return &ClaimedProviderExecution{Metadata: fake.metadata, owner: fake.owner}, nil
}

func (fake *runtimeRecoveryLedgerFake) LoadRecovery(_ context.Context, request LoadProviderExecutionRecoveryRequest) (*ProviderExecutionRecovery, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.loadCount++
	if fake.loadErr != nil {
		return nil, fake.loadErr
	}
	if request.Owner.token != fake.currentOwner || request.Owner.epoch != fake.metadata.OwnerEpoch || request.ExpectedVersion != fake.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionOwnerConflict
	}
	*fake.events = append(*fake.events, "recovery-loaded")
	return &ProviderExecutionRecovery{
		Metadata: fake.metadata, providerExecutionID: fake.recoveryID,
		checkpoint: applicationsandbox.ExecutionCheckpoint{}, hasCheckpoint: fake.hasCheckpoint,
		owner: request.Owner, startOperationID: fake.startOperationID,
		launchProviderOperationID: fake.launchProviderOperationID,
		launchRequestDigest:       fake.launchRequestDigest,
	}, nil
}

func (fake *runtimeRecoveryLedgerFake) CompleteReconciledProviderLaunch(
	_ context.Context,
	request CompleteReconciledProviderLaunchRequest,
) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.completeReconciledCount++
	fake.completeReconciledRequest = request
	fake.recoveryID = request.ProviderExecutionID
	fake.hasCheckpoint = true
	fake.metadata.HasProviderExecution = true
	fake.metadata.HasCheckpoint = true
	fake.metadata.ObservedState = domainappdev.ProviderExecutionObservedRunning
	fake.metadata.LaunchState = domainappdev.ProviderExecutionLaunchComplete
	fake.metadata.Version++
	copy := fake.metadata
	return &copy, nil
}

type runtimeRecoveryRouterFake struct {
	*runtimeStatusRouterFake
	lookupCount   int
	lookupRequest ProviderRuntimeLaunchLookupRequest
	lookupResult  ProviderRuntimeLaunchLookupResult
	lookupErr     error
}

func (fake *runtimeRecoveryRouterFake) Resolve(_ context.Context, _ applicationsandbox.ResolveProviderRequest) (ProviderRuntimeSelection, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.resolveCount++
	if fake.resumeErr != nil {
		return nil, fake.resumeErr
	}
	*fake.events = append(*fake.events, "resolved")
	selection, ok := fake.selection.(ProviderRuntimeSelection)
	if !ok {
		return nil, errors.New("selection cannot execute")
	}
	return selection, nil
}

func (fake *runtimeRecoveryRouterFake) LookupProviderExecution(
	_ context.Context,
	request ProviderRuntimeLaunchLookupRequest,
) (ProviderRuntimeLaunchLookupResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.lookupCount++
	fake.lookupRequest = request
	return fake.lookupResult, fake.lookupErr
}

func runtimeRecoveryFixture(t *testing.T) (*ProviderRuntimeOrchestrator, *ProviderRuntimeOrchestrator, *runtimeRecoveryLedgerFake, *runtimeRecoveryRouterFake, *runtimeStopSelectionFake) {
	t.Helper()
	template, stopLedger, _, selection := runtimeStopFixture(t)
	ledgerEvents := make([]string, 0, 32)
	routerEvents := make([]string, 0, 32)
	selectionEvents := make([]string, 0, 32)
	checkpointEvents := make([]string, 0, 32)
	stopLedger.events = &ledgerEvents
	selection.runtimeStartSelectionFake.events = &selectionEvents
	checkpoints := template.checkpoints.(*runtimeStartCheckpointFake)
	checkpoints.events = &checkpointEvents
	ledger := &runtimeRecoveryLedgerFake{
		runtimeStopLedgerFake:     stopLedger,
		currentOwner:              template.ownerToken,
		leaseExpired:              true,
		startOperationID:          "start-operation-original",
		launchProviderOperationID: "appdev-start-provider-operation",
		launchRequestDigest:       infrasandbox.ExecutionRequestDigest(sha256.Sum256([]byte("canonical launch"))),
	}
	router := &runtimeRecoveryRouterFake{runtimeStatusRouterFake: &runtimeStatusRouterFake{
		events: &routerEvents, selection: selection,
	}}
	instance1, err := NewProviderRuntimeOrchestrator(
		ledger, checkpoints, router, template.sources, template.grants, template.endpoints,
		runtimeStartOwnerGenerator{token: template.ownerToken}, template.config,
	)
	require.NoError(t, err)
	newOwner, err := domainappdev.NewProviderExecutionOwnerToken(stringsReaderOf("r", 32))
	require.NoError(t, err)
	instance2, err := NewProviderRuntimeOrchestrator(
		ledger, checkpoints, router, template.sources, template.grants, template.endpoints,
		runtimeStartOwnerGenerator{token: newOwner}, template.config,
	)
	require.NoError(t, err)
	return instance1, instance2, ledger, router, selection
}

func runtimeRecoveryInput() ProviderRuntimeRecoverProjectInput {
	return ProviderRuntimeRecoverProjectInput{SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject}
}

func TestProviderRuntimeRecoveryRestartClaimsNewEpochAndResumesExistingExecution(t *testing.T) {
	instance1, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedPending
	ledger.metadata.OwnerEpoch = 0
	ledger.metadata.Version = 1
	ledger.currentOwner = domainappdev.ProviderExecutionOwnerToken{}
	ledger.leaseExpired = false
	ledger.startOperationID = "restart-start-operation"
	selection.executeResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning}}
	selection.statusResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning}}

	started, err := instance1.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: ledger.startOperationID, ActorID: "runtime-start",
	})
	require.NoError(t, err)
	require.False(t, started.CanStart)
	oldEpoch := ledger.metadata.OwnerEpoch
	oldOwner := ProviderExecutionOwner{
		spaceID: runtimeStartSpace, projectID: runtimeStartProject, generation: ledger.metadata.Generation,
		providerKey: runtimeStartProvider, providerScope: ledger.metadata.ProviderScope,
		token: instance1.ownerToken, epoch: oldEpoch,
	}
	ledger.recoveryID = "provider-execution-real"
	ledger.hasCheckpoint = true
	ledger.leaseExpired = true

	projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, oldEpoch+1, ledger.metadata.OwnerEpoch)
	require.Equal(t, 1, router.resumeCount)
	require.Equal(t, 1, router.resolveCount, "only instance1 Start may resolve")
	require.Len(t, selection.executeRequests, 1, "recovery must not execute an active provider again")
	_, err = ledger.RenewOwner(context.Background(), RenewProviderExecutionOwnerRequest{Owner: oldOwner, ExpectedVersion: ledger.metadata.Version})
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionOwnerConflict)
}

func TestProviderRuntimeRecoveryOwnerBusyDoesNotCallProvider(t *testing.T) {
	_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.leaseExpired = false
	projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
	require.ErrorIs(t, err, ErrProviderRuntimeConflict)
	require.True(t, projection.Recovering)
	require.False(t, projection.CanStart)
	require.Zero(t, router.resolveCount)
	require.Zero(t, router.resumeCount)
	require.Empty(t, selection.executeRequests)
}

func TestProviderRuntimeRecoveryClaimConflictDoesNotCallProvider(t *testing.T) {
	_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.claimErr = domainappdev.ErrProviderExecutionVersionConflict
	projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
	require.ErrorIs(t, err, ErrProviderRuntimeConflict)
	require.True(t, projection.Recovering)
	require.Zero(t, router.resolveCount)
	require.Zero(t, router.resumeCount)
	require.Empty(t, selection.executeRequests)
}

func TestProviderRuntimeRecoveryDesiredStopAndTerminalContinueCleanup(t *testing.T) {
	tests := []struct {
		name     string
		desired  domainappdev.ProviderExecutionDesiredState
		observed domainappdev.ProviderExecutionObservedState
		cancel   int
	}{
		{name: "cleanup pending", desired: domainappdev.ProviderExecutionDesiredStop, observed: domainappdev.ProviderExecutionObservedCleanupPending, cancel: 1},
		{name: "terminal", desired: domainappdev.ProviderExecutionDesiredRun, observed: domainappdev.ProviderExecutionObservedSucceeded, cancel: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
			ledger.metadata.DesiredState = test.desired
			ledger.metadata.ObservedState = test.observed
			projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
			require.NoError(t, err)
			require.Equal(t, ProviderRuntimeStateStopped, projection.State)
			require.True(t, projection.CanStart)
			require.Equal(t, test.cancel, selection.cancelCount)
			require.Zero(t, selection.keepAliveCount)
			require.Equal(t, 1, selection.releaseSuccesses)
			require.Equal(t, 1, ledger.completeCount)
			require.Zero(t, router.resolveCount)
		})
	}
}

func TestProviderRuntimeRecoveryActiveUsesResumeStatusAndKeepAlive(t *testing.T) {
	_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
	selection.statusResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning}}
	projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, 1, router.resumeCount)
	require.Zero(t, router.resolveCount)
	require.Equal(t, 1, selection.keepAliveCount)
	require.Equal(t, 1, selection.statusCount)
	require.Zero(t, selection.releaseAttempts)
	require.False(t, projection.CanStart)
	require.Equal(t, domainappdev.ProviderExecutionObservedRunning, ledger.metadata.ObservedState)
}

func TestProviderRuntimeRecoverySubmittingReplaysOriginalOperationWithoutNewGeneration(t *testing.T) {
	_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
	ledger.startOperationID = "original-start-operation"
	selection.executeResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning}}
	generation := ledger.metadata.Generation
	projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, generation, projection.Generation)
	require.Equal(t, generation, ledger.metadata.Generation)
	require.Equal(t, 1, router.resolveCount)
	require.Zero(t, router.resumeCount)
	require.Len(t, selection.executeRequests, 1)
	require.Equal(t, providerRuntimeStableID("appdev_start", runtimeStartSpace, runtimeStartProject, ledger.startOperationID), selection.executeRequests[0].IdempotencyKey)
	require.Zero(t, ledger.startCount, "submitting recovery must not reserve a second submission")
}

func TestProviderRuntimeRecoveryPendingContinuesOriginalStart(t *testing.T) {
	_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedPending
	ledger.startOperationID = "original-pending-operation"
	selection.executeResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning}}
	generation := ledger.metadata.Generation
	projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, generation, ledger.metadata.Generation)
	require.Equal(t, 1, ledger.startCount)
	require.Equal(t, 1, router.resolveCount)
	require.Len(t, selection.executeRequests, 1)
}

func TestProviderRuntimeRecoveryInvalidRecoveryDataFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*runtimeRecoveryLedgerFake, *runtimeRecoveryRouterFake)
	}{
		{name: "missing checkpoint", configure: func(ledger *runtimeRecoveryLedgerFake, _ *runtimeRecoveryRouterFake) { ledger.hasCheckpoint = false }},
		{name: "corrupt checkpoint", configure: func(ledger *runtimeRecoveryLedgerFake, _ *runtimeRecoveryRouterFake) {
			ledger.loadErr = errors.New("ciphertext token-secret object/key")
		}},
		{name: "provider mismatch", configure: func(ledger *runtimeRecoveryLedgerFake, _ *runtimeRecoveryRouterFake) {
			ledger.metadata.ProviderKey = "spoof-provider"
		}},
		{name: "resume", configure: func(_ *runtimeRecoveryLedgerFake, router *runtimeRecoveryRouterFake) {
			router.resumeErr = errors.New("provider-execution-real raw response")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
			test.configure(ledger, router)
			projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
			require.Error(t, err)
			require.True(t, projection == nil || projection.Recovering)
			require.Zero(t, router.resolveCount)
			require.Empty(t, selection.executeRequests)
			safe := err.Error() + fmt.Sprintf("%v|%+v|%#v", projection, projection, projection)
			for _, secret := range []string{"token-secret", "object/key", "provider-execution-real"} {
				require.NotContains(t, safe, secret)
			}
		})
	}
}

func TestProviderRuntimeRecoveryConcurrentDoesNotAcquireExecuteOrRelease(t *testing.T) {
	_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
	selection.statusResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning}}
	const workers = 64
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			_, _ = instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
		}()
	}
	for i := 0; i < workers; i++ {
		<-ready
	}
	close(start)
	wait.Wait()
	require.Zero(t, router.resolveCount)
	require.Empty(t, selection.executeRequests)
	require.Zero(t, selection.releaseSuccesses)
	require.Equal(t, domainappdev.ProviderExecutionObservedRunning, ledger.metadata.ObservedState)
}

func TestProviderRuntimeRecoveryWithoutRecordIsStopped(t *testing.T) {
	_, instance2, ledger, router, selection := runtimeRecoveryFixture(t)
	ledger.completed = true
	projection, err := instance2.RecoverProject(context.Background(), runtimeRecoveryInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateStopped, projection.State)
	require.True(t, projection.CanStart)
	require.Zero(t, router.resolveCount)
	require.Zero(t, router.resumeCount)
	require.Empty(t, selection.executeRequests)
}

func TestProviderRuntimeRecoveryRejectsInvalidInputAndCanceledContext(t *testing.T) {
	_, instance2, _, _, _ := runtimeRecoveryFixture(t)
	_, err := instance2.RecoverProject(context.Background(), ProviderRuntimeRecoverProjectInput{})
	require.ErrorIs(t, err, ErrProviderRuntimeInvalid)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = instance2.RecoverProject(ctx, runtimeRecoveryInput())
	require.ErrorIs(t, err, context.Canceled)
}

var _ ProjectRecoveryScanner = (*ProviderRuntimeOrchestrator)(nil)
