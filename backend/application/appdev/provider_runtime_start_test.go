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
	"encoding/json"
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

const (
	runtimeStartSpace     = "1001"
	runtimeStartProject   = "project-start"
	runtimeStartProvider  = "remote-default"
	runtimeStartOperation = "start-operation-001"
	runtimeStartActor     = "actor-42"
)

type runtimeStartLedgerFake struct {
	mu sync.Mutex

	metadata        ProviderExecutionMetadata
	events          *[]string
	ensureCount     int
	claimCount      int
	startCount      int
	submitCount     int
	saveCount       int
	terminalCount   int
	ensureErr       error
	claimErr        error
	startErr        error
	abortErr        error
	saveErr         error
	terminalErr     error
	startRequest    StartProviderExecutionSubmissionRequest
	submitRequest   MarkProviderExecutionLaunchSubmittedRequest
	abortRequest    AbortProviderExecutionLaunchRequest
	saveRequest     SaveProviderExecutionSubmissionRequest
	terminalRequest AdvanceProviderExecutionTerminalRequest
	owner           ProviderExecutionOwner
}

func newRuntimeStartLedger(events *[]string) *runtimeStartLedgerFake {
	return &runtimeStartLedgerFake{
		events: events,
		metadata: ProviderExecutionMetadata{
			ID: "ledger-1", SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, Generation: 7,
			DesiredState:  domainappdev.ProviderExecutionDesiredRun,
			ObservedState: domainappdev.ProviderExecutionObservedPending,
			ProviderKey:   runtimeStartProvider, ProviderScope: domainsandbox.ScopeAppDev, Version: 1,
		},
	}
}

func (fake *runtimeStartLedgerFake) EnsureStart(_ context.Context, request EnsureProviderExecutionStartRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.ensureCount++
	if fake.ensureErr != nil {
		return nil, fake.ensureErr
	}
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStartLedgerFake) ClaimRecovery(_ context.Context, request ClaimProviderExecutionRecoveryRequest) (*ClaimedProviderExecution, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.claimCount++
	if fake.claimErr != nil {
		return nil, fake.claimErr
	}
	fake.metadata.Version++
	if fake.metadata.OwnerEpoch == 0 {
		fake.metadata.OwnerEpoch = 1
	}
	fake.owner = ProviderExecutionOwner{
		spaceID: fake.metadata.SpaceID, projectID: fake.metadata.ProjectID, generation: fake.metadata.Generation,
		providerKey: fake.metadata.ProviderKey, providerScope: fake.metadata.ProviderScope,
		token: request.ProposedOwner, epoch: fake.metadata.OwnerEpoch,
	}
	if request.ProposedOwner.IsZero() {
		fake.owner.token = request.ExistingOwner
	}
	*fake.events = append(*fake.events, "claim")
	return &ClaimedProviderExecution{Metadata: fake.metadata, owner: fake.owner}, nil
}

func (fake *runtimeStartLedgerFake) StartProviderSubmission(_ context.Context, request StartProviderExecutionSubmissionRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.startCount++
	fake.startRequest = request
	if fake.startErr != nil {
		return nil, fake.startErr
	}
	fake.metadata.LaunchState = domainappdev.ProviderExecutionLaunchPrepared
	fake.metadata.Version++
	*fake.events = append(*fake.events, "launch-prepared")
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStartLedgerFake) MarkProviderLaunchSubmitted(_ context.Context, request MarkProviderExecutionLaunchSubmittedRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.submitCount++
	fake.submitRequest = request
	fake.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
	fake.metadata.LaunchState = domainappdev.ProviderExecutionLaunchSubmitted
	fake.metadata.Version++
	*fake.events = append(*fake.events, "launch-submitted")
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStartLedgerFake) AbortProviderLaunch(_ context.Context, request AbortProviderExecutionLaunchRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.abortRequest = request
	if fake.abortErr != nil {
		return nil, fake.abortErr
	}
	fake.metadata.ObservedState = domainappdev.ProviderExecutionObservedPending
	fake.metadata.LaunchState = domainappdev.ProviderExecutionLaunchAborted
	fake.metadata.Version++
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStartLedgerFake) SaveProviderSubmission(_ context.Context, request SaveProviderExecutionSubmissionRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.saveCount++
	fake.saveRequest = request
	if fake.saveErr != nil {
		return nil, fake.saveErr
	}
	fake.metadata.ObservedState = domainappdev.ProviderExecutionObservedRunning
	fake.metadata.PreviewRoute = request.PreviewRoute
	fake.metadata.Version++
	*fake.events = append(*fake.events, "submission-saved")
	copy := fake.metadata
	return &copy, nil
}

func (fake *runtimeStartLedgerFake) AdvanceTerminal(_ context.Context, request AdvanceProviderExecutionTerminalRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.terminalCount++
	fake.terminalRequest = request
	if fake.terminalErr != nil {
		return nil, fake.terminalErr
	}
	fake.metadata.ObservedState = request.ObservedState
	fake.metadata.PreviewRoute = request.PreviewRoute
	fake.metadata.Version++
	*fake.events = append(*fake.events, "terminal-observed")
	copy := fake.metadata
	return &copy, nil
}

type runtimeStartCheckpointFake struct {
	mu      sync.Mutex
	events  *[]string
	count   int
	err     error
	request SaveProviderExecutionCheckpointRequest
}

func (fake *runtimeStartCheckpointFake) SaveCheckpoint(_ context.Context, request SaveProviderExecutionCheckpointRequest) (*ProviderExecutionMetadata, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.count++
	fake.request = request
	if fake.err != nil {
		return nil, fake.err
	}
	*fake.events = append(*fake.events, "checkpoint-saved")
	return &ProviderExecutionMetadata{
		ID: "ledger-1", SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, Generation: 7,
		DesiredState:  domainappdev.ProviderExecutionDesiredRun,
		ObservedState: domainappdev.ProviderExecutionObservedRunning,
		ProviderKey:   runtimeStartProvider, ProviderScope: domainsandbox.ScopeAppDev,
		PreviewRoute: request.Owner.projectID, Version: request.ExpectedVersion + 1,
	}, nil
}

type runtimeStartSourceFake struct {
	artifact ProviderRuntimeSourceArtifact
	err      error
}

func (fake runtimeStartSourceFake) LoadProviderRuntimeSourceArtifact(context.Context, string, string) (ProviderRuntimeSourceArtifact, error) {
	return fake.artifact, fake.err
}

type runtimeStartGrantFake struct {
	mu      sync.Mutex
	issues  []IssueArtifactGrantRequest
	revokes []RevokeArtifactGrantRequest
	err     error
	token   domainappdev.ArtifactGrantToken
}

func (fake *runtimeStartGrantFake) Issue(_ context.Context, request IssueArtifactGrantRequest) (*ArtifactGrantCapability, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.err != nil {
		return nil, fake.err
	}
	fake.issues = append(fake.issues, request)
	id, _ := domainappdev.NewRandomArtifactGrantID(strings.NewReader(strings.Repeat("g", 32)))
	if fake.token.IsZero() {
		fake.token, _ = domainappdev.NewRandomArtifactGrantToken(strings.NewReader(strings.Repeat("t", 32)))
	}
	now := time.Now().UTC()
	return &ArtifactGrantCapability{
		grantID: id, token: fake.token, audience: request.Spec.Audience, direction: request.Spec.Direction,
		digest: request.Spec.Digest, size: request.Spec.Size, maxSize: request.Spec.MaxSize,
		issuedAt: now, expiresAt: now.Add(request.TTL),
	}, nil
}

func (fake *runtimeStartGrantFake) Revoke(_ context.Context, request RevokeArtifactGrantRequest) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.revokes = append(fake.revokes, request)
	return nil
}

type runtimeStartEndpointFake struct{}

func (runtimeStartEndpointFake) ArtifactDownloadEndpoint(_ context.Context, id domainappdev.ArtifactGrantID) (string, error) {
	return "https://gateway.example/internal/provider/artifacts/" + id.Encoded(), nil
}

type runtimeStartRouterFake struct {
	mu        sync.Mutex
	events    *[]string
	count     int
	err       error
	selection ProviderRuntimeSelection
}

func (fake *runtimeStartRouterFake) Resolve(context.Context, applicationsandbox.ResolveProviderRequest) (ProviderRuntimeSelection, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.count++
	if fake.err != nil {
		return nil, fake.err
	}
	*fake.events = append(*fake.events, "resolved")
	return fake.selection, nil
}

type runtimeStartSelectionFake struct {
	mu sync.Mutex

	events          *[]string
	executeResults  []infrasandbox.ExecuteResult
	executeErrors   []error
	executeRequests []infrasandbox.ExecuteRequest
	executeEntered  chan struct{}
	executeRelease  chan struct{}
	executeDeadline time.Time
	executeEnter    sync.Once
	releaseCount    int
	checkpointCount int
	checkpointErr   error
}

type runtimeStartSubmissionError struct {
	err       error
	uncertain bool
}

func (err runtimeStartSubmissionError) Error() string { return err.err.Error() }
func (err runtimeStartSubmissionError) Unwrap() error { return err.err }
func (err runtimeStartSubmissionError) SubmissionOutcomeUncertain() bool {
	return err.uncertain
}

func (fake *runtimeStartSelectionFake) Execute(ctx context.Context, request infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	fake.mu.Lock()
	index := len(fake.executeRequests)
	fake.executeRequests = append(fake.executeRequests, request)
	*fake.events = append(*fake.events, "executed")
	if deadline, ok := ctx.Deadline(); ok {
		fake.executeDeadline = deadline
	}
	var result infrasandbox.ExecuteResult
	if index < len(fake.executeResults) {
		result = fake.executeResults[index]
	}
	var executeErr error
	if index < len(fake.executeErrors) {
		executeErr = fake.executeErrors[index]
	}
	entered, release := fake.executeEntered, fake.executeRelease
	fake.mu.Unlock()
	if entered != nil {
		fake.executeEnter.Do(func() { close(entered) })
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return result, ctx.Err()
		}
	}
	return result, executeErr
}

func (fake *runtimeStartSelectionFake) Checkpoint() (applicationsandbox.ExecutionCheckpoint, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.checkpointCount++
	if fake.checkpointErr != nil {
		return applicationsandbox.ExecutionCheckpoint{}, fake.checkpointErr
	}
	*fake.events = append(*fake.events, "checkpoint-created")
	return applicationsandbox.ExecutionCheckpoint{}, nil
}

func (fake *runtimeStartSelectionFake) Release(context.Context) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.releaseCount++
	*fake.events = append(*fake.events, "released")
	return nil
}

type runtimeStartOwnerGenerator struct {
	token domainappdev.ProviderExecutionOwnerToken
}

func (generator runtimeStartOwnerGenerator) GenerateProviderRuntimeOwnerCapability() (domainappdev.ProviderExecutionOwnerToken, error) {
	return generator.token, nil
}

func runtimeStartFixture(t *testing.T) (*ProviderRuntimeOrchestrator, *runtimeStartLedgerFake, *runtimeStartCheckpointFake, *runtimeStartGrantFake, *runtimeStartRouterFake, *runtimeStartSelectionFake, ProviderRuntimeSourceArtifact) {
	t.Helper()
	events := make([]string, 0, 12)
	ledger := newRuntimeStartLedger(&events)
	checkpoints := &runtimeStartCheckpointFake{events: &events}
	grants := &runtimeStartGrantFake{}
	selection := &runtimeStartSelectionFake{
		events: &events,
		executeResults: []infrasandbox.ExecuteResult{{
			ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning,
			PreviewRoute: "/preview/runtime-7",
		}},
	}
	router := &runtimeStartRouterFake{events: &events, selection: selection}
	digest := sha256.Sum256([]byte("source archive"))
	artifact, err := NewProviderRuntimeSourceArtifact("internal/source/project-7.zip", fmt.Sprintf("sha256:%x", digest[:]), int64(len("source archive")))
	require.NoError(t, err)
	ownerToken, err := domainappdev.NewProviderExecutionOwnerToken(strings.NewReader(strings.Repeat("o", 32)))
	require.NoError(t, err)
	orchestrator, err := NewProviderRuntimeOrchestrator(
		ledger, checkpoints, router, runtimeStartSourceFake{artifact: artifact}, grants, runtimeStartEndpointFake{},
		runtimeStartOwnerGenerator{token: ownerToken},
		ProviderRuntimeOrchestratorConfig{
			ProviderKey: runtimeStartProvider, ProviderScope: domainsandbox.ScopeAppDev,
			Policy: domainsandbox.RuntimePolicy{
				TimeoutSeconds: 120, MemoryLimitMB: 512, CPULimit: 1, MaxOutputBytes: 1024 * 1024, MaxConcurrency: 1,
			},
			Entrypoint: "npm", Args: []string{"run", "preview"},
			ExecutionTimeout: time.Minute, GrantTTL: time.Minute, ProviderLeaseDuration: time.Minute,
		},
	)
	require.NoError(t, err)
	return orchestrator, ledger, checkpoints, grants, router, selection, artifact
}

func TestProviderRuntimeStartHappyPathPersistsRealProviderIDAndCheckpointInOrder(t *testing.T) {
	orchestrator, ledger, checkpoints, _, _, selection, _ := runtimeStartFixture(t)
	ledger.metadata.ActorUserID = 42

	projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, ActorUserID: 999, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, uint64(7), projection.Generation)
	require.False(t, projection.CanStart)
	require.Equal(t, "/preview/runtime-7", projection.PreviewRoute)
	require.Equal(t, "provider-execution-real", ledger.saveRequest.ProviderExecutionID)
	require.Equal(t, "/preview/runtime-7", ledger.saveRequest.PreviewRoute)
	require.Equal(t, 1, checkpoints.count)
	require.Equal(t, 1, selection.checkpointCount)
	require.Equal(t, []string{
		"claim", "resolved", "launch-prepared", "launch-submitted", "executed", "submission-saved", "checkpoint-created", "checkpoint-saved",
	}, *ledger.events)
	require.Equal(t, runtimeStartOperation, ledger.startRequest.OperationID)
	require.Equal(t, providerRuntimeStableID("appdev_start", runtimeStartSpace, runtimeStartProject, runtimeStartOperation), ledger.startRequest.ProviderOperationID)
	require.False(t, ledger.startRequest.RequestDigest.IsZero())
	require.Equal(t, runtimeStartOperation, ledger.submitRequest.OperationID)
	require.Equal(t, runtimeStartOperation, ledger.saveRequest.LaunchOperationID)
	require.Len(t, selection.executeRequests, 1)
	require.Equal(t, infrasandbox.ExecutionIdentity{
		SpaceID: 1001, UserID: 42, ProjectID: runtimeStartProject, ExecutionID: "ledger-1",
	}, selection.executeRequests[0].Identity)
}

func TestProviderRuntimeStartExecuteContextEndsBeforeDurableDispatchLease(t *testing.T) {
	orchestrator, ledger, _, _, _, selection, _ := runtimeStartFixture(t)
	selection.executeEntered = make(chan struct{})
	selection.executeRelease = make(chan struct{})
	result := make(chan error, 1)
	startedAt := time.Now()
	go func() {
		_, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
			SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject,
			OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
		})
		result <- err
	}()
	<-selection.executeEntered

	ledger.mu.Lock()
	require.Equal(t, domainappdev.ProviderExecutionLaunchSubmitted, ledger.metadata.LaunchState)
	dispatchLease := ledger.submitRequest.DispatchLeaseDuration
	ledger.mu.Unlock()
	selection.mu.Lock()
	executeDeadline := selection.executeDeadline
	selection.mu.Unlock()
	require.False(t, executeDeadline.IsZero(), "provider Execute must have a bounded transport context")
	require.LessOrEqual(t, executeDeadline.Sub(startedAt), orchestrator.config.ExecutionTimeout+time.Second)
	require.GreaterOrEqual(t, dispatchLease, orchestrator.config.ExecutionTimeout+providerRuntimeDispatchLeaseMargin)
	require.True(t, executeDeadline.Before(startedAt.Add(dispatchLease)),
		"provider Execute deadline must leave a durable dispatch-lease margin")

	close(selection.executeRelease)
	require.NoError(t, <-result)
}

func TestProviderRuntimeStartIssuesExactSourceGrantWithoutObjectKeyLeak(t *testing.T) {
	orchestrator, _, _, grants, _, selection, artifact := runtimeStartFixture(t)
	projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.NoError(t, err)
	require.Len(t, grants.issues, 1)
	issue := grants.issues[0]
	require.Equal(t, domainappdev.ArtifactGrantAudience{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject,
		ProviderKey: runtimeStartProvider, ProviderScope: domainsandbox.ScopeAppDev,
		Operation: ProviderRuntimeStartArtifactOperation,
	}, issue.Spec.Audience)
	require.Equal(t, domainappdev.ArtifactGrantDirectionDownload, issue.Spec.Direction)
	require.Equal(t, artifact.ObjectKey(), issue.Spec.ObjectKey)
	require.Equal(t, artifact.Size(), issue.Spec.Size)
	require.Equal(t, artifact.Size(), issue.Spec.MaxSize)
	require.LessOrEqual(t, issue.TTL, 120*time.Second)
	require.Len(t, selection.executeRequests, 1)
	reference := selection.executeRequests[0].ArtifactReferences[0]
	require.Equal(t, infrasandbox.ArtifactDirectionDownload, reference.Direction)
	require.Equal(t, artifact.Digest(), reference.Digest)
	require.Equal(t, artifact.Size(), reference.Size)
	require.NotContains(t, fmt.Sprintf("%v|%+v|%#v", reference, reference, reference), artifact.ObjectKey())
	require.NotContains(t, fmt.Sprintf("%v|%+v|%#v", projection, projection, projection), artifact.ObjectKey())
}

func TestProviderRuntimeStartSubmittedRetryDoesNotResolveOrExecute(t *testing.T) {
	orchestrator, ledger, checkpoints, grants, router, selection, _ := runtimeStartFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedRunning
	ledger.metadata.PreviewRoute = "/preview/existing"

	projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.NoError(t, err)
	require.Equal(t, ProviderRuntimeStateRunning, projection.State)
	require.Equal(t, "/preview/existing", projection.PreviewRoute)
	require.Zero(t, router.count)
	require.Empty(t, selection.executeRequests)
	require.Zero(t, ledger.claimCount)
	require.Zero(t, checkpoints.count)
	require.Empty(t, grants.issues)
}

func TestProviderRuntimeStartSubmittingRetryReplaysStableProviderIdempotency(t *testing.T) {
	orchestrator, ledger, _, _, _, selection, _ := runtimeStartFixture(t)
	ledger.metadata.ObservedState = domainappdev.ProviderExecutionObservedSubmitting
	ledger.metadata.OwnerEpoch = 1
	selection.executeErrors = []error{runtimeStartSubmissionError{
		err: errors.New("transport outcome unknown with token-secret"), uncertain: true,
	}, nil}
	selection.executeResults = []infrasandbox.ExecuteResult{
		{}, {ExecutionID: "provider-execution-real", Status: infrasandbox.ExecutionStatusRunning},
	}
	input := ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	}

	firstProjection, firstErr := orchestrator.Start(context.Background(), input)
	require.Error(t, firstErr)
	require.True(t, firstProjection.Recovering)
	secondProjection, secondErr := orchestrator.Start(context.Background(), input)
	require.NoError(t, secondErr)
	require.Equal(t, ProviderRuntimeStateRunning, secondProjection.State)
	require.Len(t, selection.executeRequests, 2)
	require.Equal(t, selection.executeRequests[0].IdempotencyKey, selection.executeRequests[1].IdempotencyKey)
	require.Equal(t, uint64(7), firstProjection.Generation)
	require.Equal(t, uint64(7), secondProjection.Generation)
	require.Zero(t, ledger.startCount)
}

func TestProviderRuntimeStartDefiniteExecuteFailureAbortsLaunchAndReleasesSelection(t *testing.T) {
	orchestrator, ledger, _, grants, _, selection, _ := runtimeStartFixture(t)
	selection.executeErrors = []error{runtimeStartSubmissionError{
		err: errors.New("definitely not submitted with token-secret"),
	}}

	projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject,
		OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.Error(t, err)
	require.True(t, projection.Recovering)
	require.Equal(t, runtimeStartOperation, ledger.startRequest.OperationID)
	require.Equal(t, runtimeStartOperation, ledger.abortRequest.OperationID)
	require.Equal(t, 1, selection.releaseCount)
	require.Len(t, grants.revokes, 1)
	require.NotContains(t, err.Error(), "token-secret")
}

func TestProviderRuntimeStartUncertainExecuteFailureRetainsLaunchInterlock(t *testing.T) {
	orchestrator, ledger, _, _, _, selection, _ := runtimeStartFixture(t)
	selection.executeErrors = []error{runtimeStartSubmissionError{
		err: errors.New("submission unknown with token-secret"), uncertain: true,
	}}

	projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject,
		OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.Error(t, err)
	require.True(t, projection.Recovering)
	require.Empty(t, ledger.abortRequest.OperationID)
	require.Zero(t, selection.releaseCount)
}

func TestProviderRuntimeStartPreExecuteFailureRevokesGrantAndReleasesSelection(t *testing.T) {
	orchestrator, ledger, _, grants, _, selection, _ := runtimeStartFixture(t)
	ledger.startErr = errors.New("db unavailable with token-secret")

	projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.Error(t, err)
	require.True(t, projection.Recovering)
	require.Len(t, grants.revokes, 1)
	require.Equal(t, 1, selection.releaseCount)
	require.Empty(t, selection.executeRequests)
	require.NotContains(t, err.Error(), "token-secret")
}

func TestProviderRuntimeStartPostExecuteFailuresNeverReleaseActiveSelection(t *testing.T) {
	t.Run("save submission", func(t *testing.T) {
		orchestrator, ledger, _, grants, _, selection, artifact := runtimeStartFixture(t)
		ledger.saveErr = errors.New("save failed with provider-execution-real and " + artifact.ObjectKey())
		projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
			SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
		})
		require.Error(t, err)
		require.True(t, projection.Recovering)
		require.Zero(t, selection.releaseCount)
		require.Len(t, grants.revokes, 1)
		require.NotContains(t, err.Error()+fmt.Sprintf("%#v", projection), "provider-execution-real")
		require.NotContains(t, err.Error()+fmt.Sprintf("%#v", projection), artifact.ObjectKey())
	})

	t.Run("checkpoint persistence", func(t *testing.T) {
		orchestrator, _, checkpoints, grants, _, selection, artifact := runtimeStartFixture(t)
		checkpoints.err = errors.New("ciphertext token-secret " + artifact.ObjectKey())
		projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
			SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
		})
		require.Error(t, err)
		require.True(t, projection.Recovering)
		require.Zero(t, selection.releaseCount)
		require.Len(t, grants.revokes, 1)
		require.NotContains(t, err.Error()+fmt.Sprintf("%#v", projection), "token-secret")
		require.NotContains(t, err.Error()+fmt.Sprintf("%#v", projection), artifact.ObjectKey())
	})
}

func TestProviderRuntimeStartInvalidInputAndEmptyProviderIDFailClosed(t *testing.T) {
	orchestrator, ledger, _, grants, router, selection, _ := runtimeStartFixture(t)
	_, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{})
	require.ErrorIs(t, err, ErrProviderRuntimeInvalid)
	require.Zero(t, ledger.ensureCount)
	require.Zero(t, router.count)

	selection.executeResults = []infrasandbox.ExecuteResult{{Status: infrasandbox.ExecutionStatusRunning}}
	projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.Error(t, err)
	require.True(t, projection.Recovering)
	require.Zero(t, ledger.saveCount)
	require.Zero(t, selection.releaseCount)
	require.Len(t, grants.revokes, 1)
}

func TestProviderRuntimeStartProjectionFormattingAndJSONContainOnlySafeFields(t *testing.T) {
	orchestrator, _, _, grants, _, _, artifact := runtimeStartFixture(t)
	projection, err := orchestrator.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject, OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.NoError(t, err)
	encoded, err := json.Marshal(projection)
	require.NoError(t, err)
	formatted := fmt.Sprintf("%v|%+v|%#v", projection, projection, projection)
	for _, secret := range []string{
		artifact.ObjectKey(), grants.token.Bearer(), "provider-execution-real", runtimeStartOperation,
	} {
		require.NotContains(t, string(encoded)+formatted, secret)
	}
	require.Contains(t, string(encoded), `"generation":7`)
	require.Contains(t, string(encoded), `"state":"running"`)
}
