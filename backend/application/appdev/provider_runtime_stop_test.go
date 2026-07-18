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
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type runtimeStopLedgerFake struct {
	*runtimeStatusLedgerFake

	setStopErr              error
	beginErr                error
	completeErr             error
	completeCommitThenError bool
	setStopCount            int
	beginCount              int
	completeCount           int
	completed               bool
	setStopOperation        string
	completeOperation       string
	strictRenewCAS          bool
}

func (fake *runtimeStopLedgerFake) ClaimLaunchReconciliation(
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
	token := request.ExistingOwner
	if token.IsZero() {
		token = request.ProposedOwner
	}
	if token.IsZero() {
		return nil, domainappdev.ErrProviderExecutionOwnerConflict
	}
	if fake.metadata.OwnerEpoch == 0 {
		fake.metadata.OwnerEpoch = 1
	}
	fake.metadata.Version++
	if request.SetDesiredStop {
		fake.metadata.DesiredState = domainappdev.ProviderExecutionDesiredStop
		*fake.events = append(*fake.events, "desired-stop")
	}
	fake.owner = ProviderExecutionOwner{
		spaceID: fake.metadata.SpaceID, projectID: fake.metadata.ProjectID,
		generation: fake.metadata.Generation, providerKey: fake.metadata.ProviderKey,
		providerScope: fake.metadata.ProviderScope, token: token, epoch: fake.metadata.OwnerEpoch,
	}
	copy := fake.metadata
	return &ClaimedProviderExecution{Metadata: copy, owner: fake.owner}, nil
}

func (fake *runtimeStopLedgerFake) ListRecoverable(_ context.Context, request ListRecoverableProviderExecutionsRequest) ([]ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.listCount++
	if request.SpaceID != fake.metadata.SpaceID || request.ProjectID != fake.metadata.ProjectID {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if fake.completed {
		return nil, nil
	}
	if fake.recoveryHidden {
		return nil, nil
	}
	return []ProviderExecutionMetadata{fake.metadata}, nil
}

func (fake *runtimeStopLedgerFake) RenewOwner(_ context.Context, request RenewProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.renewCount++
	if fake.renewErr != nil {
		return nil, fake.renewErr
	}
	if fake.strictRenewCAS && request.ExpectedVersion != fake.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	fake.metadata.Version++
	*fake.events = append(*fake.events, "owner-renewed")
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStopLedgerFake) SetDesiredStop(_ context.Context, request SetProviderExecutionDesiredStopRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.setStopCount++
	fake.setStopOperation = request.OperationID
	if fake.setStopErr != nil {
		return nil, fake.setStopErr
	}
	if fake.metadata.DesiredState != domainappdev.ProviderExecutionDesiredStop {
		fake.metadata.DesiredState = domainappdev.ProviderExecutionDesiredStop
		fake.metadata.Version++
		*fake.events = append(*fake.events, "desired-stop")
	}
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStopLedgerFake) BeginCleanup(_ context.Context, request BeginProviderExecutionCleanupRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.beginCount++
	if fake.beginErr != nil {
		return nil, fake.beginErr
	}
	if fake.metadata.ObservedState != domainappdev.ProviderExecutionObservedCleanupPending {
		fake.metadata.ObservedState = domainappdev.ProviderExecutionObservedCleanupPending
		fake.metadata.Version++
		*fake.events = append(*fake.events, "cleanup-begun")
	}
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStopLedgerFake) CompleteCleanup(_ context.Context, request CompleteProviderExecutionCleanupRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.completeCount++
	fake.completeOperation = request.OperationID
	if fake.completeCommitThenError {
		fake.metadata.ObservedState = domainappdev.ProviderExecutionObservedCleanupComplete
		fake.metadata.Version++
		fake.completed = true
		*fake.events = append(*fake.events, "cleanup-complete")
		fake.completeCommitThenError = false
		return nil, errors.New("cleanup response lost token-secret")
	}
	if fake.completeErr != nil {
		return nil, fake.completeErr
	}
	if !fake.completed {
		fake.metadata.ObservedState = domainappdev.ProviderExecutionObservedCleanupComplete
		fake.metadata.Version++
		fake.completed = true
		*fake.events = append(*fake.events, "cleanup-complete")
	}
	copy := fake.metadata
	return &copy, nil
}

type runtimeStopSelectionFake struct {
	*runtimeStatusSelectionFake

	cancelCount            int
	cancelOperation        string
	cancelErr              error
	releaseAttempts        int
	releaseSuccesses       int
	released               bool
	releaseErr             error
	releaseCommitThenError bool
}

func (fake *runtimeStopSelectionFake) Cancel(_ context.Context, operationID string) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.cancelCount++
	fake.cancelOperation = operationID
	*fake.events = append(*fake.events, "canceled")
	return fake.cancelErr
}

func (fake *runtimeStopSelectionFake) Status(ctx context.Context) (infrasandbox.ExecuteResult, error) {
	result, err := fake.runtimeStatusSelectionFake.Status(ctx)
	if err != nil || result.Status != "" {
		return result, err
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.statusResults) == 0 {
		return result, nil
	}
	return fake.statusResults[len(fake.statusResults)-1], nil
}

func (fake *runtimeStopSelectionFake) Release(context.Context) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.releaseAttempts++
	if fake.releaseCommitThenError {
		if !fake.released {
			fake.released = true
			fake.releaseSuccesses++
		}
		fake.releaseCommitThenError = false
		return errors.New("release response lost provider-execution-real")
	}
	if fake.releaseErr != nil {
		return fake.releaseErr
	}
	if !fake.released {
		fake.released = true
		fake.releaseSuccesses++
		*fake.events = append(*fake.events, "released")
	}
	return nil
}

func runtimeStopFixture(t *testing.T) (*ProviderRuntimeOrchestrator, *runtimeStopLedgerFake, *runtimeStatusRouterFake, *runtimeStopSelectionFake) {
	t.Helper()
	events := make([]string, 0, 20)
	baseLedger := newRuntimeStartLedger(&events)
	baseLedger.metadata.ObservedState = domainappdev.ProviderExecutionObservedRunning
	baseLedger.metadata.OwnerEpoch = 3
	baseLedger.metadata.Version = 20
	statusLedger := &runtimeStatusLedgerFake{
		runtimeStartLedgerFake: baseLedger, recoveryID: "provider-execution-real", hasCheckpoint: true,
	}
	ledger := &runtimeStopLedgerFake{runtimeStatusLedgerFake: statusLedger}
	checkpoints := &runtimeStartCheckpointFake{events: &events}
	baseSelection := &runtimeStartSelectionFake{events: &events}
	statusSelection := &runtimeStatusSelectionFake{
		runtimeStartSelectionFake: baseSelection,
		statusResults: []infrasandbox.ExecuteResult{{
			ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusCanceled,
		}},
	}
	selection := &runtimeStopSelectionFake{runtimeStatusSelectionFake: statusSelection}
	router := &runtimeStatusRouterFake{events: &events, selection: selection}
	artifact, err := NewProviderRuntimeSourceArtifact(
		"internal/source/stop.zip",
		"sha256:da9c13d074f95a58b619d1474c70b930b934eeb0cc875b45a263f15f3dfcdba2",
		14,
	)
	require.NoError(t, err)
	ownerToken, err := domainappdev.NewProviderExecutionOwnerToken(stringsReaderOf("q", 32))
	require.NoError(t, err)
	orchestrator, err := NewProviderRuntimeOrchestrator(
		ledger, checkpoints, router, runtimeStartSourceFake{artifact: artifact}, &runtimeStartGrantFake{}, runtimeStartEndpointFake{},
		runtimeStartOwnerGenerator{token: ownerToken},
		ProviderRuntimeOrchestratorConfig{
			ProviderKey: runtimeStartProvider, ProviderScope: domainsandbox.ScopeAppDev,
			Policy:     domainsandbox.RuntimePolicy{TimeoutSeconds: 120, MemoryLimitMB: 512, CPULimit: 1, MaxOutputBytes: 1024, MaxConcurrency: 1},
			Entrypoint: "npm", ExecutionTimeout: time.Minute, GrantTTL: time.Minute, ProviderLeaseDuration: time.Minute,
		},
	)
	require.NoError(t, err)
	return orchestrator, ledger, router, selection
}

func runtimeStopInput() ProviderRuntimeStopInput {
	return ProviderRuntimeStopInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject,
		OperationID: "stop-operation-001", ActorID: runtimeStartActor,
	}
}

func TestProviderRuntimeStopHappyPathOrdersTerminalCleanup(t *testing.T) {
	orchestrator, ledger, router, selection := runtimeStopFixture(t)
	projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateStopped, projection.State)
	require.True(t, projection.CanStart)
	require.False(t, projection.Stopping)
	require.Equal(t, 1, selection.cancelCount)
	require.NotEmpty(t, selection.cancelOperation)
	require.Equal(t, 1, selection.releaseSuccesses)
	require.Equal(t, 1, ledger.completeCount)
	require.Equal(t, runtimeStopInput().OperationID, ledger.setStopOperation)
	require.Equal(t, runtimeStopInput().OperationID, ledger.completeOperation)
	require.Zero(t, router.resolveCount)
	require.Equal(t, []string{
		"owner-renewed", "desired-stop", "cleanup-begun", "recovery-loaded", "resumed",
		"canceled", "provider-status", "released", "cleanup-complete",
	}, *ledger.events)
}

func TestProviderRuntimeStopActiveAfterCancelStaysCleanupPending(t *testing.T) {
	orchestrator, ledger, _, selection := runtimeStopFixture(t)
	selection.statusResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning}}
	projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateCleanupPending, projection.State)
	require.True(t, projection.Stopping)
	require.False(t, projection.CanStart)
	require.Zero(t, selection.releaseAttempts)
	require.Zero(t, ledger.completeCount)
}

func TestProviderRuntimeStopPublishingPersistsIntentWithoutCleanup(t *testing.T) {
	orchestrator, ledger, router, selection := runtimeStopFixture(t)
	ledger.metadata.ArtifactStatus = domainappdev.ProviderExecutionArtifactPublishing

	projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
	require.NoError(t, err)
	require.True(t, projection.Stopping)
	require.False(t, projection.CanStart)
	require.Equal(t, domainappdev.ProviderExecutionDesiredStop, ledger.metadata.DesiredState)
	require.Equal(t, 1, ledger.setStopCount)
	require.Zero(t, ledger.beginCount)
	require.Zero(t, router.resumeCount)
	require.Zero(t, selection.cancelCount)
	require.Zero(t, selection.releaseAttempts)
	require.Zero(t, ledger.completeCount)
}

func TestProviderRuntimeStopRetryContinuesSameOperationAndCompletes(t *testing.T) {
	orchestrator, ledger, _, selection := runtimeStopFixture(t)
	selection.statusResults = []infrasandbox.ExecuteResult{
		{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning},
		{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusCanceled},
	}
	first, err := orchestrator.Stop(context.Background(), runtimeStopInput())
	require.NoError(t, err)
	require.True(t, first.Stopping)
	second, err := orchestrator.ContinueCleanup(context.Background(), runtimeStopInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateStopped, second.State)
	require.True(t, second.CanStart)
	require.Equal(t, 2, selection.cancelCount)
	require.Equal(t, 1, selection.releaseSuccesses)
	require.Equal(t, 1, ledger.completeCount)
}

func TestProviderRuntimeStopAlreadyTerminalSkipsCancelButReleasesAndCompletes(t *testing.T) {
	orchestrator, ledger, _, selection := runtimeStopFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSucceeded
	projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateStopped, projection.State)
	require.Zero(t, selection.cancelCount)
	require.Zero(t, selection.statusCount)
	require.Equal(t, 1, selection.releaseSuccesses)
	require.Equal(t, 1, ledger.completeCount)
}

func TestProviderRuntimeStopSubmittedLaunchConvergesByAuthoritativeLookup(t *testing.T) {
	tests := []struct {
		name            string
		lookup          ProviderRuntimeLaunchLookupResult
		lookupErr       error
		wantErr         bool
		wantLookup      int
		wantAbort       bool
		wantComplete    bool
		wantRelease     int
		wantLaunchState domainappdev.ProviderExecutionLaunchState
	}{
		{
			name: "found handle is persisted before provider cleanup",
			lookup: ProviderRuntimeLaunchLookupResult{
				Status: infrasandbox.ExecutionLookupFound,
				Execution: infrasandbox.ExecuteResult{
					ExecutionID: "provider-execution-reconciled",
					Status:      infrasandbox.ExecutionStatusRunning,
				},
			},
			wantLookup: 1, wantComplete: true, wantRelease: 1,
			wantLaunchState: domainappdev.ProviderExecutionLaunchComplete,
		},
		{
			name: "authoritative not found aborts then completes no-provider cleanup",
			lookup: ProviderRuntimeLaunchLookupResult{
				Status: infrasandbox.ExecutionLookupNotFound,
			},
			wantLookup: 1, wantAbort: true, wantComplete: true,
			wantLaunchState: domainappdev.ProviderExecutionLaunchAborted,
		},
		{
			name: "unknown keeps submitted for retry",
			lookup: ProviderRuntimeLaunchLookupResult{
				Status: infrasandbox.ExecutionLookupUnknown,
			},
			lookupErr: errors.New("lookup temporarily unavailable"),
			wantErr:   true, wantLookup: 1,
			wantLaunchState: domainappdev.ProviderExecutionLaunchSubmitted,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, orchestrator, ledger, router, selection := runtimeRecoveryFixture(t)
			ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
			ledger.metadata.LaunchState = domainappdev.ProviderExecutionLaunchSubmitted
			ledger.metadata.HasProviderExecution = false
			ledger.metadata.HasCheckpoint = false
			ledger.recoveryID = ""
			ledger.hasCheckpoint = false
			ledger.leaseExpired = true
			router.lookupResult = test.lookup
			router.lookupResult.Selection = selection
			router.lookupErr = test.lookupErr
			selection.statusResults = []infrasandbox.ExecuteResult{{
				ExecutionID: "provider-execution-reconciled",
				Status:      infrasandbox.ExecutionStatusCanceled,
			}}

			projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
			if test.wantErr {
				require.ErrorIs(t, err, ErrProviderRuntimeUnavailable)
				require.NotNil(t, projection)
				require.True(t, projection.Stopping)
				require.False(t, projection.CanStart)
			} else {
				require.NoError(t, err)
				require.NotNil(t, projection)
				require.Equal(t, ProviderRuntimeStateStopped, projection.State)
				require.True(t, projection.CanStart)
			}
			require.Equal(t, test.wantLookup, router.lookupCount)
			require.Equal(t, test.wantAbort, ledger.abortRequest.OperationID != "")
			require.Equal(t, test.wantComplete, ledger.completed)
			require.Equal(t, test.wantRelease, selection.releaseSuccesses)
			require.Equal(t, test.wantLaunchState, ledger.metadata.LaunchState)
			require.Zero(t, router.resolveCount)
			if test.wantErr {
				require.Zero(t, ledger.beginCount)
				require.Zero(t, ledger.completeCount)
			}
		})
	}
}

func TestProviderRuntimeStopWithoutActiveExecutionIsIdempotent(t *testing.T) {
	orchestrator, ledger, router, selection := runtimeStopFixture(t)
	ledger.completed = true
	projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateStopped, projection.State)
	require.True(t, projection.CanStart)
	require.Zero(t, ledger.renewCount)
	require.Zero(t, router.resumeCount)
	require.Zero(t, selection.releaseAttempts)
}

func TestProviderRuntimeStopUsesCurrentWhenLiveOwnerIsNotRecoveryEligible(t *testing.T) {
	orchestrator, ledger, router, selection := runtimeStopFixture(t)
	ledger.recoveryHidden = true

	projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
	require.NoError(t, err)
	require.True(t, projection.CanStart)
	require.Equal(t, 1, ledger.currentCount)
	require.Zero(t, ledger.listCount)
	require.Equal(t, 1, ledger.renewCount)
	require.Equal(t, 1, router.resumeCount)
	require.Equal(t, 1, selection.releaseSuccesses)
}

func TestProviderRuntimeStopOwnerBusyNeverCallsProvider(t *testing.T) {
	orchestrator, ledger, router, selection := runtimeStopFixture(t)
	ledger.renewErr = domainappdev.ErrProviderExecutionOwnerConflict
	ledger.claimErr = domainappdev.ErrProviderExecutionOwnerConflict
	projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
	require.ErrorIs(t, err, ErrProviderRuntimeConflict)
	require.True(t, projection.Recovering)
	require.Zero(t, ledger.setStopCount)
	require.Zero(t, router.resumeCount)
	require.Zero(t, selection.cancelCount)
}

func TestProviderRuntimeStopFailuresNeverMistakenlyComplete(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*runtimeStopLedgerFake, *runtimeStatusRouterFake, *runtimeStopSelectionFake)
	}{
		{name: "checkpoint", configure: func(ledger *runtimeStopLedgerFake, _ *runtimeStatusRouterFake, _ *runtimeStopSelectionFake) {
			ledger.hasCheckpoint = false
		}},
		{name: "open checkpoint", configure: func(ledger *runtimeStopLedgerFake, _ *runtimeStatusRouterFake, _ *runtimeStopSelectionFake) {
			ledger.loadErr = errors.New("ciphertext token-secret")
		}},
		{name: "resume", configure: func(_ *runtimeStopLedgerFake, router *runtimeStatusRouterFake, _ *runtimeStopSelectionFake) {
			router.resumeErr = errors.New("resume provider-execution-real")
		}},
		{name: "cancel", configure: func(_ *runtimeStopLedgerFake, _ *runtimeStatusRouterFake, selection *runtimeStopSelectionFake) {
			selection.cancelErr = errors.New("cancel token-secret")
		}},
		{name: "status", configure: func(_ *runtimeStopLedgerFake, _ *runtimeStatusRouterFake, selection *runtimeStopSelectionFake) {
			selection.statusErrors = []error{errors.New("status raw body")}
		}},
		{name: "release", configure: func(_ *runtimeStopLedgerFake, _ *runtimeStatusRouterFake, selection *runtimeStopSelectionFake) {
			selection.releaseErr = errors.New("release provider-execution-real")
		}},
		{name: "complete", configure: func(ledger *runtimeStopLedgerFake, _ *runtimeStatusRouterFake, _ *runtimeStopSelectionFake) {
			ledger.completeErr = errors.New("complete owner-token")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			orchestrator, ledger, router, selection := runtimeStopFixture(t)
			test.configure(ledger, router, selection)
			projection, err := orchestrator.Stop(context.Background(), runtimeStopInput())
			require.Error(t, err)
			require.False(t, projection.CanStart)
			require.True(t, projection.Stopping || projection.Recovering)
			require.False(t, ledger.completed)
			require.NotEqual(t, domainappdev.ProviderExecutionObservedCleanupComplete, ledger.metadata.ObservedState)
			safe := err.Error() + fmt.Sprintf("%v|%+v|%#v", projection, projection, projection)
			for _, secret := range []string{"token-secret", "provider-execution-real", "owner-token", "raw body"} {
				require.NotContains(t, safe, secret)
			}
		})
	}
}

func TestProviderRuntimeStopReleaseAndCompleteResponseLossAreIdempotent(t *testing.T) {
	t.Run("release", func(t *testing.T) {
		orchestrator, ledger, _, selection := runtimeStopFixture(t)
		selection.releaseCommitThenError = true
		first, err := orchestrator.Stop(context.Background(), runtimeStopInput())
		require.Error(t, err)
		require.False(t, first.CanStart)
		second, err := orchestrator.ContinueCleanup(context.Background(), runtimeStopInput())
		require.NoError(t, err)
		require.True(t, second.CanStart)
		require.Equal(t, 1, selection.releaseSuccesses)
		require.Equal(t, 1, ledger.completeCount)
	})
	t.Run("complete", func(t *testing.T) {
		orchestrator, ledger, _, selection := runtimeStopFixture(t)
		ledger.completeCommitThenError = true
		first, err := orchestrator.Stop(context.Background(), runtimeStopInput())
		require.Error(t, err)
		require.False(t, first.CanStart)
		second, err := orchestrator.ContinueCleanup(context.Background(), runtimeStopInput())
		require.NoError(t, err)
		require.True(t, second.CanStart)
		require.Equal(t, 1, selection.releaseSuccesses)
		require.Equal(t, 1, ledger.completeCount)
	})
}

func TestProviderRuntimeStopConcurrentSameOperationHasOneEffectiveRelease(t *testing.T) {
	orchestrator, ledger, router, selection := runtimeStopFixture(t)
	ledgerEvents := make([]string, 0, 16)
	routerEvents := make([]string, 0, 16)
	selectionEvents := make([]string, 0, 16)
	ledger.events = &ledgerEvents
	router.events = &routerEvents
	selection.runtimeStartSelectionFake.events = &selectionEvents
	ledger.strictRenewCAS = true
	const workers = 64
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			_, _ = orchestrator.Stop(context.Background(), runtimeStopInput())
		}()
	}
	for i := 0; i < workers; i++ {
		<-ready
	}
	close(start)
	wg.Wait()
	require.Equal(t, 1, selection.releaseSuccesses)
	require.True(t, ledger.completed)
}

func TestProviderRuntimeStopStatusDesiredStopDoesNotKeepAlive(t *testing.T) {
	orchestrator, ledger, _, selection := runtimeStopFixture(t)
	ledger.metadata.DesiredState = domainappdev.ProviderExecutionDesiredStop
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedCleanupPending
	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.True(t, projection.Stopping)
	require.Equal(t, ProviderRuntimeStateCleanupPending, projection.State)
	require.Zero(t, selection.keepAliveCount)
	require.Zero(t, selection.statusCount)
}

var _ ProviderRuntimeStatusSelection = (*runtimeStopSelectionFake)(nil)
var _ ProviderRuntimeCleanupSelection = (*runtimeStopSelectionFake)(nil)
var _ ProviderRuntimeResumeRouter = (*runtimeStatusRouterFake)(nil)
var _ applicationsandbox.ExecutionCheckpoint
