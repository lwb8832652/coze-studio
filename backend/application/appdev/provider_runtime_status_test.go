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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type runtimeStatusLedgerFake struct {
	*runtimeStartLedgerFake

	renewErr       error
	loadErr        error
	observeErr     error
	recoveryID     string
	hasCheckpoint  bool
	observeRequest ObserveProviderExecutionActiveRequest
	observeCount   int
	listCount      int
	currentCount   int
	recoveryHidden bool
	currentErr     error
	renewCount     int
	loadCount      int
	concurrentCAS  bool
}

func (fake *runtimeStatusLedgerFake) ListRecoverable(_ context.Context, request ListRecoverableProviderExecutionsRequest) ([]ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.listCount++
	if request.SpaceID != fake.metadata.SpaceID || request.ProjectID != fake.metadata.ProjectID {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if fake.recoveryHidden {
		return nil, nil
	}
	return []ProviderExecutionMetadata{fake.metadata}, nil
}

func (fake *runtimeStatusLedgerFake) RenewOwner(_ context.Context, request RenewProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.renewCount++
	if fake.renewErr != nil {
		return nil, fake.renewErr
	}
	fake.metadata.Version++
	*fake.events = append(*fake.events, "owner-renewed")
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStatusLedgerFake) LoadRecovery(_ context.Context, request LoadProviderExecutionRecoveryRequest) (*ProviderExecutionRecovery, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.loadCount++
	if fake.loadErr != nil {
		return nil, fake.loadErr
	}
	*fake.events = append(*fake.events, "recovery-loaded")
	return &ProviderExecutionRecovery{
		Metadata: fake.metadata, providerExecutionID: fake.recoveryID,
		checkpoint: applicationsandbox.ExecutionCheckpoint{}, hasCheckpoint: fake.hasCheckpoint,
		owner: request.Owner,
	}, nil
}

func (fake *runtimeStatusLedgerFake) ObserveActive(_ context.Context, request ObserveProviderExecutionActiveRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.observeCount++
	if fake.observeErr != nil {
		return nil, fake.observeErr
	}
	if fake.concurrentCAS && fake.observeCount > 1 {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	fake.observeRequest = request
	fake.metadata.Version++
	fake.metadata.ObservedState = domainappdev.ProviderExecutionObservedRunning
	fake.metadata.PreviewRoute = request.PreviewRoute
	*fake.events = append(*fake.events, "active-observed")
	copy := fake.metadata
	return &copy, nil
}

type runtimeStatusSelectionFake struct {
	*runtimeStartSelectionFake

	statusResults  []infrasandbox.ExecuteResult
	statusErrors   []error
	statusCount    int
	keepAliveCount int
	keepAliveErr   error
}

func (fake *runtimeStatusSelectionFake) KeepAlive(context.Context) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.keepAliveCount++
	*fake.events = append(*fake.events, "keep-alive")
	return fake.keepAliveErr
}

func (fake *runtimeStatusSelectionFake) Status(context.Context) (infrasandbox.ExecuteResult, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	index := fake.statusCount
	fake.statusCount++
	*fake.events = append(*fake.events, "provider-status")
	var result infrasandbox.ExecuteResult
	if index < len(fake.statusResults) {
		result = fake.statusResults[index]
	}
	if index < len(fake.statusErrors) {
		return result, fake.statusErrors[index]
	}
	return result, nil
}

type runtimeStatusRouterFake struct {
	mu sync.Mutex

	events       *[]string
	selection    ProviderRuntimeStatusSelection
	resolveCount int
	resumeCount  int
	resumeErr    error
	resumeID     string
}

func (fake *runtimeStatusRouterFake) Resolve(context.Context, applicationsandbox.ResolveProviderRequest) (ProviderRuntimeSelection, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.resolveCount++
	return nil, errors.New("status must not resolve")
}

func (fake *runtimeStatusRouterFake) Resume(_ context.Context, _ applicationsandbox.ExecutionCheckpoint, executionID string) (ProviderRuntimeStatusSelection, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.resumeCount++
	fake.resumeID = executionID
	if fake.resumeErr != nil {
		return nil, fake.resumeErr
	}
	*fake.events = append(*fake.events, "resumed")
	return fake.selection, nil
}

func runtimeStatusFixture(t *testing.T) (*ProviderRuntimeOrchestrator, *runtimeStatusLedgerFake, *runtimeStartCheckpointFake, *runtimeStatusRouterFake, *runtimeStatusSelectionFake) {
	t.Helper()
	events := make([]string, 0, 16)
	baseLedger := newRuntimeStartLedger(&events)
	baseLedger.metadata.ObservedState = domainappdev.ProviderExecutionObservedRunning
	baseLedger.metadata.OwnerEpoch = 2
	baseLedger.metadata.Version = 10
	ledger := &runtimeStatusLedgerFake{
		runtimeStartLedgerFake: baseLedger,
		recoveryID:             "provider-execution-real",
		hasCheckpoint:          true,
	}
	checkpoints := &runtimeStartCheckpointFake{events: &events}
	baseSelection := &runtimeStartSelectionFake{events: &events}
	selection := &runtimeStatusSelectionFake{
		runtimeStartSelectionFake: baseSelection,
		statusResults: []infrasandbox.ExecuteResult{{
			ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning,
			PreviewRoute: "/preview/status",
		}},
	}
	router := &runtimeStatusRouterFake{events: &events, selection: selection}
	digest := "sha256:da9c13d074f95a58b619d1474c70b930b934eeb0cc875b45a263f15f3dfcdba2"
	artifact, err := NewProviderRuntimeSourceArtifact("internal/source/status.zip", digest, 14)
	require.NoError(t, err)
	ownerToken, err := domainappdev.NewProviderExecutionOwnerToken(stringsReaderOf("s", 32))
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
	return orchestrator, ledger, checkpoints, router, selection
}

func stringsReaderOf(value string, count int) *strings.Reader {
	return strings.NewReader(strings.Repeat(value, count))
}

func runtimeStatusInput() ProviderRuntimeStatusInput {
	return ProviderRuntimeStatusInput{SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: "status-operation-001", ActorID: runtimeStartActor}
}

func TestProviderRuntimeStatusWithoutProviderIDOnlyReturnsRecoveringProjection(t *testing.T) {
	orchestrator, ledger, _, router, selection := runtimeStatusFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
	ledger.recoveryID = ""

	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateStarting, projection.State)
	require.True(t, projection.Recovering)
	require.False(t, projection.CanStart)
	require.Zero(t, router.resumeCount)
	require.Zero(t, router.resolveCount)
	require.Zero(t, selection.statusCount)
	require.Zero(t, ledger.renewCount)
}

func TestProviderRuntimeStatusNoRecordReturnsStoppedProjection(t *testing.T) {
	orchestrator, ledger, _, router, selection := runtimeStatusFixture(t)
	ledger.currentErr = domainappdev.ErrProviderExecutionNotFound

	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateStopped, projection.State)
	require.True(t, projection.CanStart)
	require.Zero(t, router.resumeCount)
	require.Zero(t, selection.statusCount)
}

func TestProviderRuntimeStatusResumesWithoutResolveOrAcquire(t *testing.T) {
	orchestrator, _, _, router, selection := runtimeStatusFixture(t)
	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, 1, router.resumeCount)
	require.Equal(t, "provider-execution-real", router.resumeID)
	require.Zero(t, router.resolveCount)
	require.Equal(t, 1, selection.statusCount)
	require.Zero(t, selection.releaseCount)
}

func TestProviderRuntimeStatusUsesCurrentWhenLiveOwnerIsNotRecoveryEligible(t *testing.T) {
	orchestrator, ledger, _, router, selection := runtimeStatusFixture(t)
	ledger.recoveryHidden = true

	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, 1, ledger.currentCount)
	require.Zero(t, ledger.listCount)
	require.Equal(t, 1, ledger.renewCount)
	require.Equal(t, 1, router.resumeCount)
	require.Equal(t, 1, selection.statusCount)
}

func TestProviderRuntimeStatusOrdersKeepAliveStatusCASAndCheckpoint(t *testing.T) {
	orchestrator, ledger, checkpoints, _, _ := runtimeStatusFixture(t)
	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, "/preview/status", ledger.observeRequest.PreviewRoute)
	require.Equal(t, "provider-execution-real", ledger.observeRequest.ProviderExecutionID)
	require.Equal(t, 1, checkpoints.count)
	require.Equal(t, []string{
		"owner-renewed", "recovery-loaded", "resumed", "keep-alive", "provider-status",
		"active-observed", "checkpoint-created", "checkpoint-saved",
	}, *ledger.events)
}

func TestProviderRuntimeStatusAcceptedCannotRegressRunningAndNeverReleases(t *testing.T) {
	orchestrator, ledger, _, _, selection := runtimeStatusFixture(t)
	selection.statusResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusAccepted}}
	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, domainappdev.ProviderExecutionObservedRunning, ledger.metadata.ObservedState)
	require.Equal(t, 1, ledger.observeCount)
	require.Zero(t, selection.releaseCount)
}

func TestProviderRuntimeStatusTerminalAdvancesWithoutCleanupOrRelease(t *testing.T) {
	orchestrator, ledger, checkpoints, _, selection := runtimeStatusFixture(t)
	exitCode := 0
	selection.statusResults = []infrasandbox.ExecuteResult{{
		ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusSucceeded,
		ExitCode: &exitCode, PreviewRoute: "/preview/final",
	}}
	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateSucceeded, projection.State)
	require.Equal(t, domainappdev.ProviderExecutionObservedSucceeded, ledger.terminalRequest.ObservedState)
	require.False(t, projection.CanStart)
	require.Zero(t, checkpoints.count)
	require.Zero(t, selection.releaseCount)
	require.Equal(t, 1, ledger.terminalCount)
}

func TestProviderRuntimeStatusOwnerBusyNeverCallsProvider(t *testing.T) {
	orchestrator, ledger, _, router, selection := runtimeStatusFixture(t)
	ledger.renewErr = domainappdev.ErrProviderExecutionOwnerConflict
	ledger.claimErr = domainappdev.ErrProviderExecutionOwnerConflict
	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.ErrorIs(t, err, ErrProviderRuntimeConflict)
	require.True(t, projection.Recovering)
	require.Zero(t, router.resumeCount)
	require.Zero(t, selection.keepAliveCount)
	require.Zero(t, selection.statusCount)
}

func TestProviderRuntimeStatusExpiredOwnerClaimsThenResumes(t *testing.T) {
	orchestrator, ledger, _, router, _ := runtimeStatusFixture(t)
	ledger.renewErr = domainappdev.ErrProviderExecutionOwnerConflict
	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, 1, ledger.claimCount)
	require.Equal(t, 1, router.resumeCount)
}

func TestProviderRuntimeStatusCheckpointResumeAndProviderErrorsAreSafe(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*runtimeStatusLedgerFake, *runtimeStatusRouterFake, *runtimeStatusSelectionFake)
	}{
		{name: "corrupt checkpoint", configure: func(ledger *runtimeStatusLedgerFake, _ *runtimeStatusRouterFake, _ *runtimeStatusSelectionFake) {
			ledger.loadErr = errors.New("corrupt checkpoint token-secret provider-execution-real")
		}},
		{name: "resume", configure: func(_ *runtimeStatusLedgerFake, router *runtimeStatusRouterFake, _ *runtimeStatusSelectionFake) {
			router.resumeErr = errors.New("resume raw provider-execution-real")
		}},
		{name: "status", configure: func(_ *runtimeStatusLedgerFake, _ *runtimeStatusRouterFake, selection *runtimeStatusSelectionFake) {
			selection.statusErrors = []error{errors.New("raw provider body token-secret")}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			orchestrator, ledger, _, router, selection := runtimeStatusFixture(t)
			test.configure(ledger, router, selection)
			projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
			require.Error(t, err)
			require.True(t, projection.Recovering)
			require.NotContains(t, err.Error()+fmt.Sprintf("%#v", projection), "token-secret")
			require.NotContains(t, err.Error()+fmt.Sprintf("%#v", projection), "provider-execution-real")
			require.Zero(t, selection.releaseCount)
		})
	}
}

func TestProviderRuntimeStatusPreviewValidation(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		orchestrator, ledger, checkpoints, _, selection := runtimeStatusFixture(t)
		selection.statusResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning, PreviewRoute: "https://evil.example/?token=x"}}
		projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
		require.Error(t, err)
		require.True(t, projection.Recovering)
		require.Zero(t, ledger.observeCount)
		require.Zero(t, checkpoints.count)
	})
	t.Run("relative", func(t *testing.T) {
		orchestrator, ledger, _, _, selection := runtimeStatusFixture(t)
		selection.statusResults = []infrasandbox.ExecuteResult{{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning, PreviewRoute: "/preview/safe"}}
		projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
		require.NoError(t, err)
		require.Equal(t, "/preview/safe", projection.PreviewRoute)
		require.Equal(t, "/preview/safe", ledger.observeRequest.PreviewRoute)
	})
}

func TestProviderRuntimeStatusStaleCASDoesNotRegressOrLeakSecrets(t *testing.T) {
	orchestrator, ledger, checkpoints, _, selection := runtimeStatusFixture(t)
	ledger.observeErr = errors.New("stale CAS provider-execution-real token-secret")
	projection, err := orchestrator.Status(context.Background(), runtimeStatusInput())
	require.Error(t, err)
	require.True(t, projection.Recovering)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, domainappdev.ProviderExecutionObservedRunning, ledger.metadata.ObservedState)
	require.Zero(t, checkpoints.count)
	require.Zero(t, selection.releaseCount)
	require.NotContains(t, err.Error()+fmt.Sprintf("%#v", projection), "provider-execution-real")
	require.NotContains(t, err.Error()+fmt.Sprintf("%#v", projection), "token-secret")
}

func TestProviderRuntimeStatusConcurrentStaleCASIsRaceSafe(t *testing.T) {
	orchestrator, ledger, checkpoints, router, selection := runtimeStatusFixture(t)
	ledgerEvents := make([]string, 0, 8)
	checkpointEvents := make([]string, 0, 4)
	routerEvents := make([]string, 0, 4)
	selectionEvents := make([]string, 0, 8)
	ledger.events = &ledgerEvents
	checkpoints.events = &checkpointEvents
	router.events = &routerEvents
	selection.events = &selectionEvents
	ledger.concurrentCAS = true
	selection.statusResults = []infrasandbox.ExecuteResult{
		{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning},
		{ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusAccepted},
	}
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready <- struct{}{}
			<-start
			_, _ = orchestrator.Status(context.Background(), runtimeStatusInput())
		}()
	}
	<-ready
	<-ready
	close(start)
	wg.Wait()
	require.Equal(t, 2, selection.statusCount)
	require.Equal(t, domainappdev.ProviderExecutionObservedRunning, ledger.metadata.ObservedState)
	require.Zero(t, selection.releaseCount)
}
