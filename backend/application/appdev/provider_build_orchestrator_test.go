// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestProviderBuildOrchestratorBeginAcceptedKeepsRuntimeActive(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.selection.beginObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusAccepted}

	projection, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if err != nil {
		t.Fatalf("BeginBuild() error = %v", err)
	}
	if projection.State != ProviderBuildStateBuilding || projection.Generation != fixture.generation || projection.ReleaseAvailable {
		t.Fatalf("projection = %+v", projection)
	}
	fixture.assertEvents(t, "list", "renew", "target", "reserve", "recovery", "resume", "begin", "observe:building")
	if fixture.selection.beginExecutionID != fixture.providerExecutionID || fixture.selection.beginOperationID != fixture.operationID {
		t.Fatalf("BeginBuild identity = %q, %q", fixture.selection.beginExecutionID, fixture.selection.beginOperationID)
	}
	if fixture.router.resolveCalls.Load() != 0 || fixture.selection.releaseCalls.Load() != 0 {
		t.Fatalf("Resolve/Release calls = %d/%d", fixture.router.resolveCalls.Load(), fixture.selection.releaseCalls.Load())
	}
}

func TestProviderBuildOrchestratorPollRunningUsesStatusOnly(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.ledger.setBuild(fixture.operationID, domainappdev.ProviderExecutionArtifactBuilding, domainappdev.ProviderBuildArtifactDescriptor{})
	fixture.selection.statusObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusRunning}

	projection, err := fixture.orchestrator.PollBuild(context.Background(), ProviderBuildPollInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if err != nil {
		t.Fatalf("PollBuild() error = %v", err)
	}
	if projection.State != ProviderBuildStateBuilding || fixture.selection.beginCalls.Load() != 0 || fixture.selection.statusCalls.Load() != 1 {
		t.Fatalf("projection/calls = %+v, begin=%d status=%d", projection, fixture.selection.beginCalls.Load(), fixture.selection.statusCalls.Load())
	}
	fixture.assertEvents(t, "list", "renew", "target", "recovery", "resume", "status", "observe:building")
}

func TestProviderBuildOrchestratorUsesCurrentWhenLiveOwnerIsNotRecoveryEligible(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.ledger.recoveryHidden = true
	fixture.selection.beginObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusAccepted}

	projection, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if err != nil {
		t.Fatalf("BeginBuild() error = %v", err)
	}
	if projection.State != ProviderBuildStateBuilding || fixture.ledger.currentCalls != 1 || fixture.ledger.recoveryListCalls != 0 {
		t.Fatalf("projection/current/recovery = %+v/%d/%d", projection, fixture.ledger.currentCalls, fixture.ledger.recoveryListCalls)
	}
}

func TestProviderBuildOrchestratorDescriptorPublishesAndCompletes(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.selection.beginObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusDescriptorReady, Descriptor: fixture.descriptor}

	projection, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if err != nil {
		t.Fatalf("BeginBuild() error = %v", err)
	}
	if projection.State != ProviderBuildStateReady || !projection.ReleaseAvailable || projection.Size != fixture.descriptor.Size {
		t.Fatalf("projection = %+v", projection)
	}
	fixture.assertEvents(t, "list", "renew", "target", "reserve", "recovery", "resume", "begin", "observe:descriptor_ready", "publish-reserve", "publish", "publish-complete")
	command := fixture.publisher.lastCommand()
	if command.Generation != fixture.generation || command.ProviderExecutionID != fixture.providerExecutionID ||
		command.Audience.SpaceID != fixture.spaceID || command.Audience.ProjectID != fixture.projectID ||
		command.Audience.ProviderKey != fixture.providerKey || command.Audience.ProviderScope != domainsandbox.ScopeAppDev ||
		command.Audience.Operation != BuildArtifactPublishOperation || command.Descriptor != fixture.descriptor {
		t.Fatalf("publish command = %+v", command)
	}
	if fixture.ledger.objectKey != fixture.objectKey || fixture.ledger.metadata.ArtifactDigest != fixture.descriptor.Digest {
		t.Fatalf("persisted artifact = %q, %q", fixture.ledger.objectKey, fixture.ledger.metadata.ArtifactDigest)
	}
}

func TestProviderBuildOrchestratorPublishingRetryKeepsDestinationIdentity(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.ledger.setBuild(fixture.operationID, domainappdev.ProviderExecutionArtifactPublishing, domainDescriptor(fixture.descriptor))
	fixture.publisher.err = ErrBuildArtifactPublishUnavailable

	_, firstErr := fixture.orchestrator.PollBuild(context.Background(), ProviderBuildPollInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if !errors.Is(firstErr, ErrProviderBuildUnavailable) {
		t.Fatalf("first PollBuild() error = %v", firstErr)
	}
	first := fixture.publisher.lastCommand()
	fixture.publisher.err = nil
	projection, secondErr := fixture.orchestrator.PollBuild(context.Background(), ProviderBuildPollInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if secondErr != nil || projection.State != ProviderBuildStateReady {
		t.Fatalf("second PollBuild() = %+v, %v", projection, secondErr)
	}
	second := fixture.publisher.lastCommand()
	if first.Generation != second.Generation || first.ProviderExecutionID != second.ProviderExecutionID || first.Descriptor != second.Descriptor || first.Audience != second.Audience {
		t.Fatalf("retry destination identity changed: first=%+v second=%+v", first, second)
	}
	if fixture.publisher.calls.Load() != 2 || fixture.ledger.completeCalls != 1 {
		t.Fatalf("publish/complete calls = %d/%d", fixture.publisher.calls.Load(), fixture.ledger.completeCalls)
	}
}

func TestProviderBuildOrchestratorFinishesPublishingAfterStopIntent(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.ledger.setBuild(fixture.operationID, domainappdev.ProviderExecutionArtifactPublishing, domainDescriptor(fixture.descriptor))
	fixture.ledger.metadata.DesiredState = domainappdev.ProviderExecutionDesiredStop

	projection, err := fixture.orchestrator.PollBuild(context.Background(), ProviderBuildPollInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if err != nil {
		t.Fatalf("PollBuild() error = %v", err)
	}
	if projection.State != ProviderBuildStateReady || fixture.ledger.objectKey != fixture.objectKey {
		t.Fatalf("projection/object = %+v/%q", projection, fixture.ledger.objectKey)
	}
	if fixture.selection.beginCalls.Load() != 0 || fixture.selection.statusCalls.Load() != 0 || fixture.ledger.reserveCalls != 0 {
		t.Fatalf("forbidden begin/status/reserve = %d/%d/%d", fixture.selection.beginCalls.Load(), fixture.selection.statusCalls.Load(), fixture.ledger.reserveCalls)
	}
}

func TestProviderBuildOrchestratorCompleteResponseLossUsesExactLedgerPostcondition(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.ledger.setBuild(fixture.operationID, domainappdev.ProviderExecutionArtifactPublishing, domainDescriptor(fixture.descriptor))
	fixture.ledger.metadata.DesiredState = domainappdev.ProviderExecutionDesiredStop
	fixture.ledger.completeCommitThenError = true

	projection, err := fixture.orchestrator.RecoverBuild(context.Background(), ProviderBuildRecoverInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID,
	})
	if err != nil {
		t.Fatalf("RecoverBuild() response-loss error = %v", err)
	}
	if projection.State != ProviderBuildStateReady || fixture.ledger.completeCalls != 1 || fixture.ledger.objectKey != fixture.objectKey {
		t.Fatalf("projection/complete/object = %+v/%d/%q", projection, fixture.ledger.completeCalls, fixture.ledger.objectKey)
	}
}

func TestProviderBuildOrchestratorSameOperationIsIdempotentAndDifferentOperationConflicts(t *testing.T) {
	t.Run("ready same operation", func(t *testing.T) {
		fixture := newProviderBuildFixture(t)
		fixture.ledger.setBuild(fixture.operationID, domainappdev.ProviderExecutionArtifactReady, domainDescriptor(fixture.descriptor))
		projection, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
			SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
		})
		if err != nil || projection.State != ProviderBuildStateReady || fixture.router.resumeCalls.Load() != 0 || fixture.ledger.reserveCalls != 0 {
			t.Fatalf("BeginBuild() = %+v, %v; resume=%d reserve=%d", projection, err, fixture.router.resumeCalls.Load(), fixture.ledger.reserveCalls)
		}
	})
	t.Run("active different operation", func(t *testing.T) {
		fixture := newProviderBuildFixture(t)
		fixture.ledger.setBuild("other-operation", domainappdev.ProviderExecutionArtifactBuilding, domainappdev.ProviderBuildArtifactDescriptor{})
		_, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
			SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
		})
		if !errors.Is(err, ErrProviderBuildConflict) || fixture.router.resumeCalls.Load() != 0 || fixture.ledger.reserveCalls != 0 {
			t.Fatalf("BeginBuild() error = %v; resume=%d reserve=%d", err, fixture.router.resumeCalls.Load(), fixture.ledger.reserveCalls)
		}
	})
}

func TestProviderBuildOrchestratorRecoverUsesPersistedOperationAndStatus(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.ledger.setBuild(fixture.operationID, domainappdev.ProviderExecutionArtifactBuilding, domainappdev.ProviderBuildArtifactDescriptor{})
	fixture.ledger.renewErr = domainappdev.ErrProviderExecutionOwnerConflict
	fixture.selection.statusObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusRunning}

	projection, err := fixture.orchestrator.RecoverBuild(context.Background(), ProviderBuildRecoverInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID,
	})
	if err != nil || projection.State != ProviderBuildStateBuilding {
		t.Fatalf("RecoverBuild() = %+v, %v", projection, err)
	}
	if fixture.ledger.claimCalls != 1 || fixture.selection.beginCalls.Load() != 0 || fixture.selection.statusCalls.Load() != 1 || fixture.router.resolveCalls.Load() != 0 {
		t.Fatalf("claim/begin/status/resolve = %d/%d/%d/%d", fixture.ledger.claimCalls, fixture.selection.beginCalls.Load(), fixture.selection.statusCalls.Load(), fixture.router.resolveCalls.Load())
	}
}

func TestProviderBuildOrchestratorRecoveryReplaysBeginAfterReservationCrash(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.ledger.recoveryErr = errors.New("simulated process crash before provider begin")

	_, firstErr := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if !errors.Is(firstErr, ErrProviderBuildUnavailable) {
		t.Fatalf("first BeginBuild() error = %v", firstErr)
	}
	if fixture.ledger.metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactBeginPending ||
		fixture.selection.beginCalls.Load() != 0 || fixture.ledger.reserveCalls != 1 {
		t.Fatalf("crash boundary status/begin/reserve = %q/%d/%d", fixture.ledger.metadata.ArtifactStatus, fixture.selection.beginCalls.Load(), fixture.ledger.reserveCalls)
	}

	fixture.ledger.recoveryErr = nil
	fixture.ledger.renewErr = domainappdev.ErrProviderExecutionOwnerConflict
	second := fixture.newOrchestrator(t, 0x42)
	fixture.selection.beginObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusAccepted}
	projection, err := second.RecoverBuild(context.Background(), ProviderBuildRecoverInput{SpaceID: fixture.spaceID, ProjectID: fixture.projectID})
	if err != nil || projection.State != ProviderBuildStateBuilding {
		t.Fatalf("second RecoverBuild() = %+v, %v", projection, err)
	}
	if fixture.selection.beginCalls.Load() != 1 || fixture.selection.statusCalls.Load() != 0 ||
		fixture.ledger.reserveCalls != 1 || fixture.ledger.metadata.Generation != fixture.generation ||
		fixture.ledger.metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactBuilding {
		t.Fatalf("recovery begin/status/reserve/generation/state = %d/%d/%d/%d/%q",
			fixture.selection.beginCalls.Load(), fixture.selection.statusCalls.Load(), fixture.ledger.reserveCalls,
			fixture.ledger.metadata.Generation, fixture.ledger.metadata.ArtifactStatus)
	}
}

func TestProviderBuildOrchestratorRecoveryReplaysLostBeginResponseIdempotently(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.selection.beginFunc = func(call int64, _, _ string) (infrasandbox.BuildObservation, error) {
		if call == 1 {
			return infrasandbox.BuildObservation{}, context.DeadlineExceeded
		}
		return infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusRunning}, nil
	}

	_, firstErr := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if !errors.Is(firstErr, context.DeadlineExceeded) || fixture.ledger.metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactBeginPending {
		t.Fatalf("lost response error/state = %v/%q", firstErr, fixture.ledger.metadata.ArtifactStatus)
	}

	fixture.ledger.renewErr = domainappdev.ErrProviderExecutionOwnerConflict
	second := fixture.newOrchestrator(t, 0x43)
	projection, err := second.RecoverBuild(context.Background(), ProviderBuildRecoverInput{SpaceID: fixture.spaceID, ProjectID: fixture.projectID})
	if err != nil || projection.State != ProviderBuildStateBuilding {
		t.Fatalf("RecoverBuild() = %+v, %v", projection, err)
	}
	if fixture.selection.beginCalls.Load() != 2 || fixture.selection.statusCalls.Load() != 0 ||
		fixture.selection.createdBuilds.Load() != 1 || fixture.ledger.metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactBuilding {
		t.Fatalf("begin/status/created/state = %d/%d/%d/%q", fixture.selection.beginCalls.Load(),
			fixture.selection.statusCalls.Load(), fixture.selection.createdBuilds.Load(), fixture.ledger.metadata.ArtifactStatus)
	}
}

func TestProviderBuildOrchestratorPollUsesStatusOnlyAfterPositiveBeginObservation(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.selection.beginObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusAccepted}
	if _, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	}); err != nil {
		t.Fatal(err)
	}
	fixture.selection.statusObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusRunning}
	if _, err := fixture.orchestrator.PollBuild(context.Background(), ProviderBuildPollInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	}); err != nil {
		t.Fatal(err)
	}
	if fixture.selection.beginCalls.Load() != 1 || fixture.selection.statusCalls.Load() != 1 ||
		fixture.ledger.metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactBuilding {
		t.Fatalf("begin/status/state = %d/%d/%q", fixture.selection.beginCalls.Load(), fixture.selection.statusCalls.Load(), fixture.ledger.metadata.ArtifactStatus)
	}
}

func TestProviderBuildOrchestratorOwnerBusyDoesNotCallProvider(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.ledger.renewErr = domainappdev.ErrProviderExecutionOwnerConflict
	fixture.ledger.claimErr = domainappdev.ErrProviderExecutionOwnerConflict

	_, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
		SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
	})
	if !errors.Is(err, ErrProviderBuildConflict) || fixture.router.resumeCalls.Load() != 0 || fixture.selection.beginCalls.Load() != 0 {
		t.Fatalf("BeginBuild() error = %v; resume=%d begin=%d", err, fixture.router.resumeCalls.Load(), fixture.selection.beginCalls.Load())
	}
}

func TestProviderBuildOrchestratorMissingCapabilitiesFailsClosed(t *testing.T) {
	t.Run("build executor", func(t *testing.T) {
		fixture := newProviderBuildFixture(t)
		fixture.router.selection = &providerBuildStatusOnlySelection{}
		_, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
			SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
		})
		if !errors.Is(err, ErrProviderBuildUnsupported) || fixture.ledger.metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactFailed {
			t.Fatalf("BeginBuild() error/status = %v/%q", err, fixture.ledger.metadata.ArtifactStatus)
		}
	})
	t.Run("artifact publisher", func(t *testing.T) {
		fixture := newProviderBuildFixture(t)
		executorOnly := &providerBuildExecutorOnlySelection{
			providerBuildStatusOnlySelection: providerBuildStatusOnlySelection{},
			events:                           &fixture.ledger.events,
			observation:                      infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusDescriptorReady, Descriptor: fixture.descriptor},
		}
		fixture.router.selection = executorOnly
		_, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
			SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
		})
		if !errors.Is(err, ErrProviderBuildUnsupported) || fixture.ledger.completeCalls != 0 {
			t.Fatalf("BeginBuild() error/complete = %v/%d", err, fixture.ledger.completeCalls)
		}
	})
}

func TestProviderBuildOrchestratorRejectsMalformedOrMismatchedDescriptor(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		fixture := newProviderBuildFixture(t)
		fixture.selection.beginObservation = infrasandbox.BuildObservation{
			Status:     infrasandbox.BuildStatusDescriptorReady,
			Descriptor: infrasandbox.ArtifactDescriptor{Kind: infrasandbox.ArtifactKindAppDevBuildArchive, Digest: "bad", Size: 12},
		}
		_, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
			SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
		})
		if !errors.Is(err, ErrProviderBuildUnavailable) || fixture.publisher.calls.Load() != 0 || fixture.ledger.metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactFailed {
			t.Fatalf("BeginBuild() error/status/publish = %v/%q/%d", err, fixture.ledger.metadata.ArtifactStatus, fixture.publisher.calls.Load())
		}
	})
	t.Run("ledger descriptor mismatch", func(t *testing.T) {
		fixture := newProviderBuildFixture(t)
		fixture.ledger.setBuild(fixture.operationID, domainappdev.ProviderExecutionArtifactDescriptorReady, domainDescriptor(fixture.descriptor))
		fixture.ledger.metadata.ArtifactDigest = "bad"
		_, err := fixture.orchestrator.PollBuild(context.Background(), ProviderBuildPollInput{
			SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
		})
		if !errors.Is(err, ErrProviderBuildUnavailable) || fixture.publisher.calls.Load() != 0 {
			t.Fatalf("PollBuild() error/publish = %v/%d", err, fixture.publisher.calls.Load())
		}
	})
}

func TestProviderBuildOrchestratorBlocksStoppedCleanupAndTerminalRuntime(t *testing.T) {
	tests := []struct {
		name     string
		desired  domainappdev.ProviderExecutionDesiredState
		observed domainappdev.ProviderExecutionObservedState
	}{
		{name: "stopping", desired: domainappdev.ProviderExecutionDesiredStop, observed: domainappdev.ProviderExecutionObservedRunning},
		{name: "cleanup", desired: domainappdev.ProviderExecutionDesiredStop, observed: domainappdev.ProviderExecutionObservedCleanupPending},
		{name: "terminal", desired: domainappdev.ProviderExecutionDesiredRun, observed: domainappdev.ProviderExecutionObservedSucceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProviderBuildFixture(t)
			fixture.ledger.metadata.DesiredState = test.desired
			fixture.ledger.metadata.ObservedState = test.observed
			_, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
				SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
			})
			if !errors.Is(err, ErrProviderBuildConflict) || fixture.ledger.reserveCalls != 0 || fixture.router.resumeCalls.Load() != 0 {
				t.Fatalf("BeginBuild() error/reserve/resume = %v/%d/%d", err, fixture.ledger.reserveCalls, fixture.router.resumeCalls.Load())
			}
		})
	}
}

func TestProviderBuildOrchestratorFailuresNeverMarkReady(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*providerBuildFixture)
	}{
		{name: "resume", setup: func(f *providerBuildFixture) { f.router.resumeErr = errors.New("resume raw secret") }},
		{name: "provider", setup: func(f *providerBuildFixture) { f.selection.beginErr = errors.New("provider raw secret") }},
		{name: "publisher", setup: func(f *providerBuildFixture) {
			f.selection.beginObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusDescriptorReady, Descriptor: f.descriptor}
			f.publisher.err = errors.New("storage raw secret")
		}},
		{name: "ledger complete", setup: func(f *providerBuildFixture) {
			f.selection.beginObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusDescriptorReady, Descriptor: f.descriptor}
			f.ledger.completeErr = errors.New("db raw secret")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProviderBuildFixture(t)
			test.setup(fixture)
			projection, err := fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
				SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
			})
			if err == nil || projection != nil && projection.State == ProviderBuildStateReady || fixture.ledger.metadata.ArtifactStatus == domainappdev.ProviderExecutionArtifactReady {
				t.Fatalf("BeginBuild() = %+v, %v; ledger=%q", projection, err, fixture.ledger.metadata.ArtifactStatus)
			}
			for _, secret := range []string{"resume raw secret", "provider raw secret", "storage raw secret", "db raw secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked raw backend value: %v", err)
				}
			}
		})
	}
}

func TestProviderBuildOrchestratorConcurrentBeginDoesNotDuplicateProviderWork(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	fixture.selection.beginObservation = infrasandbox.BuildObservation{Status: infrasandbox.BuildStatusAccepted}
	const workers = 64
	ready := make(chan struct{}, workers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			_, _ = fixture.orchestrator.BeginBuild(context.Background(), ProviderBuildBeginInput{
				SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID,
			})
		}()
	}
	for index := 0; index < workers; index++ {
		<-ready
	}
	close(start)
	wait.Wait()
	if fixture.selection.createdBuilds.Load() != 1 || !fixture.selection.onlyOperation(fixture.operationID) ||
		fixture.publisher.calls.Load() > 1 || fixture.selection.releaseCalls.Load() != 0 {
		t.Fatalf("begin/created/publish/release calls = %d/%d/%d/%d", fixture.selection.beginCalls.Load(),
			fixture.selection.createdBuilds.Load(), fixture.publisher.calls.Load(), fixture.selection.releaseCalls.Load())
	}
}

func TestProviderBuildOrchestratorInputProjectionAndErrorsAreSafe(t *testing.T) {
	fixture := newProviderBuildFixture(t)
	for _, input := range []ProviderBuildBeginInput{
		{}, {SpaceID: fixture.spaceID, ProjectID: fixture.projectID},
		{SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: "../escape"},
	} {
		if _, err := fixture.orchestrator.BeginBuild(context.Background(), input); !errors.Is(err, ErrProviderBuildInvalid) {
			t.Fatalf("BeginBuild(%+v) error = %v", input, err)
		}
	}
	projection := ProviderBuildProjection{Generation: fixture.generation, State: ProviderBuildStateReady, ReleaseAvailable: true, Size: fixture.descriptor.Size, SafeMessage: "ready"}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	values := string(encoded) + fmt.Sprintf("%v %+v %#v", projection, projection, projection) + fmt.Sprintf("%v %+v %#v", fixture.orchestrator, fixture.orchestrator, fixture.orchestrator)
	for _, secret := range []string{fixture.operationID, fixture.providerExecutionID, fixture.objectKey, "bearer", "checkpoint"} {
		if strings.Contains(values, secret) {
			t.Fatalf("format leaked %q in %q", secret, values)
		}
	}
	if _, err := json.Marshal(fixture.orchestrator); !errors.Is(err, domainappdev.ErrProviderExecutionSecret) {
		t.Fatalf("orchestrator JSON error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.orchestrator.BeginBuild(canceled, ProviderBuildBeginInput{SpaceID: fixture.spaceID, ProjectID: fixture.projectID, OperationID: fixture.operationID}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled BeginBuild() error = %v", err)
	}
}

type providerBuildFixture struct {
	t                   *testing.T
	spaceID             string
	projectID           string
	providerKey         string
	providerExecutionID string
	operationID         string
	objectKey           string
	generation          uint64
	descriptor          infrasandbox.ArtifactDescriptor
	ledger              *providerBuildFakeLedger
	selection           *providerBuildFakeSelection
	router              *providerBuildFakeRouter
	publisher           *providerBuildFakePublisher
	orchestrator        *ProviderBuildOrchestrator
}

func newProviderBuildFixture(t *testing.T) *providerBuildFixture {
	t.Helper()
	ownerToken, err := domainappdev.NewProviderExecutionOwnerToken(bytes.NewReader(bytes.Repeat([]byte{0x41}, 128)))
	if err != nil {
		t.Fatal(err)
	}
	fixture := &providerBuildFixture{
		t: t, spaceID: "space-1", projectID: "project-1", providerKey: "provider-a",
		providerExecutionID: "execution-real-1", operationID: "build-operation-1",
		objectKey: "internal/object/key.zip", generation: 7,
		descriptor: infrasandbox.ArtifactDescriptor{Kind: infrasandbox.ArtifactKindAppDevBuildArchive, Digest: "sha256:" + strings.Repeat("a", 64), Size: 4096},
	}
	fixture.ledger = newProviderBuildFakeLedger(fixture, ownerToken)
	fixture.selection = &providerBuildFakeSelection{events: &fixture.ledger.events, createdOperations: make(map[string]struct{})}
	fixture.router = &providerBuildFakeRouter{selection: fixture.selection, events: &fixture.ledger.events}
	fixture.publisher = &providerBuildFakePublisher{events: &fixture.ledger.events, receipt: newBuildArtifactReceipt(fixture.objectKey, mustArtifactDigest(t, fixture.descriptor.Digest), fixture.descriptor.Size)}
	fixture.orchestrator, err = NewProviderBuildOrchestrator(
		fixture.ledger, fixture.router, fixture.publisher,
		providerBuildStaticOwnerGenerator{token: ownerToken},
		ProviderBuildOrchestratorConfig{ProviderKey: fixture.providerKey, ProviderScope: domainsandbox.ScopeAppDev},
	)
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *providerBuildFixture) newOrchestrator(t *testing.T, seed byte) *ProviderBuildOrchestrator {
	t.Helper()
	token, err := domainappdev.NewProviderExecutionOwnerToken(bytes.NewReader(bytes.Repeat([]byte{seed}, 128)))
	if err != nil {
		t.Fatal(err)
	}
	orchestrator, err := NewProviderBuildOrchestrator(
		fixture.ledger, fixture.router, fixture.publisher, providerBuildStaticOwnerGenerator{token: token},
		ProviderBuildOrchestratorConfig{ProviderKey: fixture.providerKey, ProviderScope: domainsandbox.ScopeAppDev},
	)
	if err != nil {
		t.Fatal(err)
	}
	return orchestrator
}

func (fixture *providerBuildFixture) assertEvents(t *testing.T, want ...string) {
	t.Helper()
	got := fixture.ledger.eventSnapshot()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

type providerBuildStaticOwnerGenerator struct {
	token domainappdev.ProviderExecutionOwnerToken
}

func (generator providerBuildStaticOwnerGenerator) GenerateProviderRuntimeOwnerCapability() (domainappdev.ProviderExecutionOwnerToken, error) {
	return generator.token, nil
}

type providerBuildEventLog struct {
	mu     sync.Mutex
	values []string
}

func (log *providerBuildEventLog) add(value string) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.values = append(log.values, value)
}

func (log *providerBuildEventLog) snapshot() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]string(nil), log.values...)
}

type providerBuildFakeLedger struct {
	mu                      sync.Mutex
	fixture                 *providerBuildFixture
	ownerToken              domainappdev.ProviderExecutionOwnerToken
	metadata                ProviderExecutionMetadata
	operationID             string
	providerExecutionID     string
	objectKey               string
	events                  providerBuildEventLog
	renewErr                error
	claimErr                error
	completeErr             error
	completeCommitThenError bool
	recoveryErr             error
	reserveCalls            int
	claimCalls              int
	completeCalls           int
	currentCalls            int
	recoveryListCalls       int
	recoveryHidden          bool
}

func newProviderBuildFakeLedger(fixture *providerBuildFixture, ownerToken domainappdev.ProviderExecutionOwnerToken) *providerBuildFakeLedger {
	return &providerBuildFakeLedger{
		fixture: fixture, ownerToken: ownerToken, providerExecutionID: fixture.providerExecutionID,
		metadata: ProviderExecutionMetadata{
			ID: "ledger-1", SpaceID: fixture.spaceID, ProjectID: fixture.projectID, Generation: fixture.generation,
			DesiredState: domainappdev.ProviderExecutionDesiredRun, ObservedState: domainappdev.ProviderExecutionObservedRunning,
			ProviderKey: fixture.providerKey, ProviderScope: domainsandbox.ScopeAppDev,
			OwnerEpoch: 3, Version: 11, ArtifactStatus: domainappdev.ProviderExecutionArtifactNone,
			UpdatedAt: time.Now().UTC(),
		},
	}
}

func (ledger *providerBuildFakeLedger) eventSnapshot() []string { return ledger.events.snapshot() }

func (ledger *providerBuildFakeLedger) ListRecoverable(context.Context, ListRecoverableProviderExecutionsRequest) ([]ProviderExecutionMetadata, error) {
	ledger.events.add("list")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.recoveryListCalls++
	if ledger.recoveryHidden {
		return nil, nil
	}
	return []ProviderExecutionMetadata{ledger.metadata}, nil
}

func (ledger *providerBuildFakeLedger) RenewOwner(context.Context, RenewProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error) {
	ledger.events.add("renew")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.renewErr != nil {
		return nil, ledger.renewErr
	}
	ledger.metadata.Version++
	copy := ledger.metadata
	return &copy, nil
}

func (ledger *providerBuildFakeLedger) ClaimRecovery(_ context.Context, request ClaimProviderExecutionRecoveryRequest) (*ClaimedProviderExecution, error) {
	ledger.events.add("claim")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.claimCalls++
	if ledger.claimErr != nil {
		return nil, ledger.claimErr
	}
	ledger.metadata.Version++
	ledger.metadata.OwnerEpoch++
	if !request.ProposedOwner.IsZero() {
		ledger.ownerToken = request.ProposedOwner
	}
	owner := ProviderExecutionOwner{
		spaceID: ledger.fixture.spaceID, projectID: ledger.fixture.projectID, generation: ledger.fixture.generation,
		providerKey: ledger.fixture.providerKey, providerScope: domainsandbox.ScopeAppDev,
		token: ledger.ownerToken, epoch: ledger.metadata.OwnerEpoch,
	}
	return &ClaimedProviderExecution{Metadata: ledger.metadata, owner: owner}, nil
}

func (ledger *providerBuildFakeLedger) LoadBuildTarget(context.Context, LoadProviderExecutionBuildTargetRequest) (*ProviderExecutionBuildTarget, error) {
	ledger.events.add("target")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return &ProviderExecutionBuildTarget{
		Metadata: ledger.metadata, providerExecutionID: ledger.providerExecutionID,
		buildOperationID: ledger.operationID, artifactStatus: ledger.metadata.ArtifactStatus,
		artifactObjectKey: ledger.objectKey,
	}, nil
}

func (ledger *providerBuildFakeLedger) ReserveBuild(_ context.Context, request ReserveProviderExecutionBuildRequest) (*ProviderExecutionMetadata, error) {
	ledger.events.add("reserve")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if request.ExpectedVersion != ledger.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	if ledger.metadata.ArtifactStatus == domainappdev.ProviderExecutionArtifactBeginPending ||
		ledger.metadata.ArtifactStatus == domainappdev.ProviderExecutionArtifactBuilding {
		if ledger.operationID == request.OperationID {
			copy := ledger.metadata
			return &copy, nil
		}
		return nil, domainappdev.ErrProviderExecutionOperationConflict
	}
	ledger.reserveCalls++
	ledger.operationID = request.OperationID
	ledger.metadata.ArtifactStatus = domainappdev.ProviderExecutionArtifactBeginPending
	ledger.metadata.ArtifactVersion++
	ledger.metadata.Version++
	copy := ledger.metadata
	return &copy, nil
}

func (ledger *providerBuildFakeLedger) LoadRecovery(_ context.Context, request LoadProviderExecutionRecoveryRequest) (*ProviderExecutionRecovery, error) {
	ledger.events.add("recovery")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.recoveryErr != nil {
		return nil, ledger.recoveryErr
	}
	owner := request.Owner
	return &ProviderExecutionRecovery{
		Metadata: ledger.metadata, providerExecutionID: ledger.providerExecutionID,
		buildOperationID: ledger.operationID, artifactStatus: ledger.metadata.ArtifactStatus,
		checkpoint: applicationsandbox.ExecutionCheckpoint{}, hasCheckpoint: true, owner: owner,
	}, nil
}

func (ledger *providerBuildFakeLedger) AdvanceBuildObservation(_ context.Context, request AdvanceProviderExecutionBuildObservationRequest) (*ProviderExecutionMetadata, error) {
	ledger.events.add("observe:" + string(request.Status))
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if request.ExpectedVersion != ledger.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	ledger.metadata.ArtifactStatus = request.Status
	if request.Status == domainappdev.ProviderExecutionArtifactDescriptorReady {
		ledger.metadata.ArtifactKind = request.Descriptor.Kind
		ledger.metadata.ArtifactDigest = request.Descriptor.Digest
		ledger.metadata.ArtifactSize = request.Descriptor.Size
	}
	ledger.metadata.Version++
	copy := ledger.metadata
	return &copy, nil
}

func (ledger *providerBuildFakeLedger) BeginArtifactPublish(_ context.Context, request BeginProviderArtifactPublishRequest) (*ProviderExecutionMetadata, error) {
	ledger.events.add("publish-reserve")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if request.ExpectedVersion != ledger.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	ledger.metadata.ArtifactStatus = domainappdev.ProviderExecutionArtifactPublishing
	ledger.metadata.Version++
	copy := ledger.metadata
	return &copy, nil
}

func (ledger *providerBuildFakeLedger) CompleteArtifactPublish(_ context.Context, request CompleteProviderArtifactPublishRequest) (*ProviderExecutionMetadata, error) {
	ledger.events.add("publish-complete")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.completeErr != nil && !ledger.completeCommitThenError {
		return nil, ledger.completeErr
	}
	if request.ExpectedVersion != ledger.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	ledger.completeCalls++
	ledger.objectKey = request.ObjectKey
	ledger.metadata.ArtifactStatus = domainappdev.ProviderExecutionArtifactReady
	ledger.metadata.ArtifactDigest = request.Digest
	ledger.metadata.ArtifactSize = request.Size
	ledger.metadata.Version++
	if ledger.completeCommitThenError {
		return nil, errors.New("publish completion response lost")
	}
	copy := ledger.metadata
	return &copy, nil
}

func (ledger *providerBuildFakeLedger) FailBuild(_ context.Context, request FailProviderExecutionBuildRequest) (*ProviderExecutionMetadata, error) {
	ledger.events.add("build-fail")
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if request.ExpectedVersion != ledger.metadata.Version {
		return nil, domainappdev.ErrProviderExecutionVersionConflict
	}
	ledger.metadata.ArtifactStatus = domainappdev.ProviderExecutionArtifactFailed
	ledger.metadata.ArtifactSafeErrorCode = request.SafeErrorCode
	ledger.metadata.ArtifactSafeErrorMessage = request.SafeErrorMessage
	ledger.metadata.Version++
	copy := ledger.metadata
	return &copy, nil
}

func (ledger *providerBuildFakeLedger) setBuild(operationID string, status domainappdev.ProviderExecutionArtifactStatus, descriptor domainappdev.ProviderBuildArtifactDescriptor) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.operationID = operationID
	ledger.metadata.ArtifactStatus = status
	ledger.metadata.ArtifactKind = descriptor.Kind
	ledger.metadata.ArtifactDigest = descriptor.Digest
	ledger.metadata.ArtifactSize = descriptor.Size
}

type providerBuildFakeRouter struct {
	selection    ProviderRuntimeStatusSelection
	events       *providerBuildEventLog
	resumeErr    error
	resumeCalls  atomic.Int64
	resolveCalls atomic.Int64
}

func (router *providerBuildFakeRouter) Resume(context.Context, applicationsandbox.ExecutionCheckpoint, string) (ProviderRuntimeStatusSelection, error) {
	router.resumeCalls.Add(1)
	router.events.add("resume")
	if router.resumeErr != nil {
		return nil, router.resumeErr
	}
	return router.selection, nil
}

type providerBuildFakeSelection struct {
	mu                sync.Mutex
	events            *providerBuildEventLog
	beginObservation  infrasandbox.BuildObservation
	statusObservation infrasandbox.BuildObservation
	beginFunc         func(int64, string, string) (infrasandbox.BuildObservation, error)
	beginErr          error
	statusErr         error
	beginExecutionID  string
	beginOperationID  string
	beginCalls        atomic.Int64
	createdBuilds     atomic.Int64
	createdOperations map[string]struct{}
	beginOperations   []string
	statusCalls       atomic.Int64
	releaseCalls      atomic.Int64
}

func (selection *providerBuildFakeSelection) BeginBuild(_ context.Context, executionID, operationID string) (infrasandbox.BuildObservation, error) {
	call := selection.beginCalls.Add(1)
	selection.events.add("begin")
	selection.mu.Lock()
	selection.beginExecutionID = executionID
	selection.beginOperationID = operationID
	selection.beginOperations = append(selection.beginOperations, operationID)
	if _, exists := selection.createdOperations[operationID]; !exists {
		selection.createdOperations[operationID] = struct{}{}
		selection.createdBuilds.Add(1)
	}
	beginFunc := selection.beginFunc
	selection.mu.Unlock()
	if beginFunc != nil {
		return beginFunc(call, executionID, operationID)
	}
	return selection.beginObservation, selection.beginErr
}

func (selection *providerBuildFakeSelection) onlyOperation(operationID string) bool {
	selection.mu.Lock()
	defer selection.mu.Unlock()
	if len(selection.beginOperations) == 0 {
		return false
	}
	for _, value := range selection.beginOperations {
		if value != operationID {
			return false
		}
	}
	return true
}

func (selection *providerBuildFakeSelection) BuildStatus(context.Context, string, string) (infrasandbox.BuildObservation, error) {
	selection.statusCalls.Add(1)
	selection.events.add("status")
	return selection.statusObservation, selection.statusErr
}

func (selection *providerBuildFakeSelection) PublishArtifact(context.Context, string, infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
	return infrasandbox.ArtifactPublishResult{Accepted: true}, nil
}

func (*providerBuildFakeSelection) KeepAlive(context.Context) error { return nil }
func (*providerBuildFakeSelection) Status(context.Context) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, nil
}
func (*providerBuildFakeSelection) Checkpoint() (applicationsandbox.ExecutionCheckpoint, error) {
	return applicationsandbox.ExecutionCheckpoint{}, nil
}

type providerBuildStatusOnlySelection struct{}

func (*providerBuildStatusOnlySelection) KeepAlive(context.Context) error { return nil }
func (*providerBuildStatusOnlySelection) Status(context.Context) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, nil
}
func (*providerBuildStatusOnlySelection) Checkpoint() (applicationsandbox.ExecutionCheckpoint, error) {
	return applicationsandbox.ExecutionCheckpoint{}, nil
}

type providerBuildExecutorOnlySelection struct {
	providerBuildStatusOnlySelection
	events      *providerBuildEventLog
	observation infrasandbox.BuildObservation
}

func (selection *providerBuildExecutorOnlySelection) BeginBuild(context.Context, string, string) (infrasandbox.BuildObservation, error) {
	selection.events.add("begin")
	return selection.observation, nil
}

func (selection *providerBuildExecutorOnlySelection) BuildStatus(context.Context, string, string) (infrasandbox.BuildObservation, error) {
	selection.events.add("status")
	return selection.observation, nil
}

type providerBuildFakePublisher struct {
	events   *providerBuildEventLog
	receipt  *BuildArtifactReceipt
	err      error
	calls    atomic.Int64
	mu       sync.Mutex
	commands []PublishBuildArtifactCommand
}

func (publisher *providerBuildFakePublisher) PublishWithPublisher(_ context.Context, _ infrasandbox.ArtifactPublisher, command PublishBuildArtifactCommand) (*BuildArtifactReceipt, error) {
	publisher.calls.Add(1)
	publisher.events.add("publish")
	publisher.mu.Lock()
	publisher.commands = append(publisher.commands, command)
	publisher.mu.Unlock()
	if publisher.err != nil {
		return nil, publisher.err
	}
	return publisher.receipt, nil
}

func (publisher *providerBuildFakePublisher) lastCommand() PublishBuildArtifactCommand {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if len(publisher.commands) == 0 {
		return PublishBuildArtifactCommand{}
	}
	return publisher.commands[len(publisher.commands)-1]
}

func domainDescriptor(descriptor infrasandbox.ArtifactDescriptor) domainappdev.ProviderBuildArtifactDescriptor {
	return domainappdev.ProviderBuildArtifactDescriptor{Kind: domainappdev.ProviderExecutionArtifactKind(descriptor.Kind), Digest: descriptor.Digest, Size: descriptor.Size}
}

func mustArtifactDigest(t *testing.T, value string) domainappdev.ArtifactGrantDigest {
	t.Helper()
	digest, err := domainappdev.ParseArtifactGrantDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
