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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestProviderAPIFacadeOperationValidationAndPassthrough(t *testing.T) {
	runtime := &providerAPIRuntimeFake{startResult: &ProviderRuntimeProjection{Generation: 7, State: ProviderRuntimeStateRunning}}
	build := &providerAPIBuildFake{beginResult: &ProviderBuildProjection{Generation: 7, State: ProviderBuildStateBuilding}}
	facade := NewProviderAPIFacade(runtime, build, nil, nil)

	if _, err := facade.StartRuntime(context.Background(), ProviderRuntimeMutationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "bad id"}); !errors.Is(err, ErrProviderControlInvalid) {
		t.Fatalf("invalid operation error = %v", err)
	}
	if runtime.startCalls != 0 {
		t.Fatal("invalid operation reached runtime control")
	}
	request := ProviderRuntimeMutationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "runtime-start-001", ActorID: "actor-1"}
	projection, err := facade.StartRuntime(context.Background(), request)
	if err != nil || projection.Generation != 7 || runtime.startInput.OperationID != request.OperationID {
		t.Fatalf("StartRuntime() = %#v, %v, input=%#v", projection, err, runtime.startInput)
	}
	if _, err := facade.BeginBuild(context.Background(), ProviderBuildOperationRequest{
		SpaceID: "1001", ProjectID: "project-a", OperationID: "build-operation-001",
	}); err != nil || build.beginInput.OperationID != "build-operation-001" {
		t.Fatalf("BeginBuild passthrough = %v, input=%#v", err, build.beginInput)
	}
}

func TestProviderAPIFacadeMissingControlsFailClosed(t *testing.T) {
	facade := NewProviderAPIFacade(nil, nil, nil, nil)
	if _, err := facade.StartRuntime(context.Background(), ProviderRuntimeMutationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "runtime-start-001"}); !errors.Is(err, ErrProviderControlUnavailable) {
		t.Fatalf("missing runtime error = %v", err)
	}
	if _, err := facade.BeginBuild(context.Background(), ProviderBuildOperationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "build-operation-001"}); !errors.Is(err, ErrProviderControlUnavailable) {
		t.Fatalf("missing build error = %v", err)
	}
	if _, err := facade.OpenRelease(context.Background(), ProviderReleaseRequest{SpaceID: "1001", ProjectID: "project-a"}); !errors.Is(err, ErrProviderControlUnavailable) {
		t.Fatalf("missing release error = %v", err)
	}
	if _, err := (&Service{}).ProviderAPI(); !errors.Is(err, ErrProviderControlUnavailable) {
		t.Fatalf("missing service facade error = %v", err)
	}
}

func TestProviderAPIFacadeInjectionNeverCallsLegacyRuntimeManager(t *testing.T) {
	legacy := &providerAPILegacyRuntimeFake{}
	service := NewService(nil, legacy)
	providerRuntime := &providerAPIRuntimeFake{startResult: &ProviderRuntimeProjection{Generation: 1, State: ProviderRuntimeStateStarting}}
	facade := NewProviderAPIFacade(providerRuntime, nil, nil, nil)
	if err := service.SetProviderAPI(facade); err != nil {
		t.Fatal(err)
	}
	injected, err := service.ProviderAPI()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := injected.StartRuntime(context.Background(), ProviderRuntimeMutationRequest{
		SpaceID: "1001", ProjectID: "project-a", OperationID: "runtime-start-001",
	}); err != nil {
		t.Fatal(err)
	}
	if legacy.calls != 0 || providerRuntime.startCalls != 1 {
		t.Fatalf("legacy/provider calls = %d/%d", legacy.calls, providerRuntime.startCalls)
	}
}

func TestProviderAPIFacadeRestartWaitsForCleanupComplete(t *testing.T) {
	runtime := &providerAPIRuntimeFake{statusResult: &ProviderRuntimeProjection{Generation: 4, State: ProviderRuntimeStateRunning}, stopResults: []*ProviderRuntimeProjection{
		{Generation: 4, State: ProviderRuntimeStateCleanupPending, Stopping: true, CanStart: false},
		{Generation: 4, State: ProviderRuntimeStateCleanupComplete, CanStart: true},
	}, startResult: &ProviderRuntimeProjection{Generation: 5, State: ProviderRuntimeStateStarting}}
	facade := NewProviderAPIFacade(runtime, nil, nil, nil)
	request := ProviderRuntimeMutationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "runtime-restart-001", ActorID: "actor-1"}

	first, err := facade.RestartRuntime(context.Background(), request)
	if err != nil || !first.Stopping || first.CanStart || runtime.startCalls != 0 {
		t.Fatalf("first restart = %#v, %v, starts=%d", first, err, runtime.startCalls)
	}
	second, err := facade.RestartRuntime(context.Background(), request)
	if err != nil || second.Generation != 5 || runtime.startCalls != 1 {
		t.Fatalf("second restart = %#v, %v, starts=%d", second, err, runtime.startCalls)
	}
	if len(runtime.stopInputs) != 2 || runtime.stopInputs[0].OperationID != runtime.stopInputs[1].OperationID ||
		runtime.stopInputs[0].OperationID == runtime.startInput.OperationID {
		t.Fatalf("restart suboperations = stop=%#v start=%#v", runtime.stopInputs, runtime.startInput)
	}
}

func TestProviderAPIFacadeRestartResponseLossMatchesCurrentStartBeforeStop(t *testing.T) {
	runtime := &providerAPIRuntimeFake{
		statusResult: &ProviderRuntimeProjection{Generation: 4, State: ProviderRuntimeStateRunning},
		stopResults:  []*ProviderRuntimeProjection{{Generation: 4, State: ProviderRuntimeStateCleanupComplete, CanStart: true}},
		startResult:  &ProviderRuntimeProjection{Generation: 5, State: ProviderRuntimeStateRunning},
	}
	facade := NewProviderAPIFacade(runtime, nil, nil, nil)
	request := ProviderRuntimeMutationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "runtime-restart-loss-001", ActorID: "actor-1"}
	first, err := facade.RestartRuntime(context.Background(), request)
	if err != nil || first.Generation != 5 || runtime.startCalls != 1 || len(runtime.stopInputs) != 1 {
		t.Fatalf("first restart = %#v, %v, stop/start=%d/%d", first, err, len(runtime.stopInputs), runtime.startCalls)
	}
	runtime.matchFound = true
	runtime.matchResult = &ProviderRuntimeProjection{Generation: 5, State: ProviderRuntimeStateRunning}
	second, err := facade.RestartRuntime(context.Background(), request)
	if err != nil || second.Generation != 5 || runtime.startCalls != 1 || len(runtime.stopInputs) != 1 {
		t.Fatalf("response-loss retry = %#v, %v, stop/start=%d/%d", second, err, len(runtime.stopInputs), runtime.startCalls)
	}
	if len(runtime.matchInputs) != 2 || runtime.matchInputs[0].OperationID != runtime.startInput.OperationID || runtime.matchInputs[1].OperationID != runtime.startInput.OperationID {
		t.Fatalf("restart match/start operations = %#v / %#v", runtime.matchInputs, runtime.startInput)
	}
}

func TestProviderAPIFacadeRestartMatchedTerminalNeverStopsNewGeneration(t *testing.T) {
	runtime := &providerAPIRuntimeFake{
		matchFound:   true,
		matchResult:  &ProviderRuntimeProjection{Generation: 8, State: ProviderRuntimeStateSucceeded, CanStart: false},
		statusResult: &ProviderRuntimeProjection{Generation: 8, State: ProviderRuntimeStateSucceeded, CanStart: false},
	}
	facade := NewProviderAPIFacade(runtime, nil, nil, nil)
	projection, err := facade.RestartRuntime(context.Background(), ProviderRuntimeMutationRequest{
		SpaceID: "1001", ProjectID: "project-a", OperationID: "runtime-restart-terminal", ActorID: "actor-1",
	})
	if err != nil || projection.Generation != 8 || projection.State != ProviderRuntimeStateSucceeded || len(runtime.stopInputs) != 0 || runtime.startCalls != 0 {
		t.Fatalf("matched terminal restart = %#v, %v, stop/start=%d/%d", projection, err, len(runtime.stopInputs), runtime.startCalls)
	}
}

func TestProviderAPIFacadeProjectionsAndErrorsDoNotLeak(t *testing.T) {
	runtime := &providerAPIRuntimeFake{
		statusResult: &ProviderRuntimeProjection{Generation: 3, State: ProviderRuntimeStateRunning, PreviewRoute: "/preview/safe"},
		statusErr:    errors.New("provider-execution-secret checkpoint-token object/key raw body"),
	}
	build := &providerAPIBuildFake{pollResult: &ProviderBuildProjection{
		Generation: 3, State: ProviderBuildStateReady, ReleaseAvailable: true, Size: 42, UpdatedAt: time.Now().UTC(),
	}}
	source := &providerBuildSourceSnapshotStore{
		providerAPISnapshotFake: &providerAPISnapshotFake{},
		project: &domainappdev.Project{
			ID: "project-a", SpaceID: "1001", SourceUpdatedAt: build.pollResult.UpdatedAt.Add(-time.Second),
		},
	}
	facade := NewProviderAPIFacade(runtime, build, nil, source)
	runtimeProjection, runtimeErr := facade.RuntimeStatus(context.Background(), ProviderRuntimeStatusRequest{SpaceID: "1001", ProjectID: "project-a"})
	if !errors.Is(runtimeErr, ErrProviderControlUnavailable) || runtimeProjection == nil || runtimeProjection.RelativePreview != "/preview/safe" {
		t.Fatalf("RuntimeStatus() = %#v, %v", runtimeProjection, runtimeErr)
	}
	buildProjection, err := facade.PollBuild(context.Background(), ProviderBuildOperationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "build-operation-001"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(struct {
		Runtime *ProviderRuntimeAPIProjection `json:"runtime"`
		Build   *ProviderBuildAPIProjection   `json:"build"`
	}{runtimeProjection, buildProjection})
	if err != nil {
		t.Fatal(err)
	}
	formatted := string(encoded) + fmt.Sprintf("%v|%+v|%#v|%v", runtimeProjection, runtimeProjection, runtimeProjection, runtimeErr)
	for _, secret := range []string{"provider-execution-secret", "checkpoint-token", "object/key", "raw body", "artifactPath", "object_key", "digest"} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("projection leaked %q: %s", secret, formatted)
		}
	}
}

func TestProviderAPIFacadeSnapshotRestoreStopBarrierAndStableSuboperations(t *testing.T) {
	runtime := &providerAPIRuntimeFake{
		statusResult: &ProviderRuntimeProjection{Generation: 8, State: ProviderRuntimeStateRunning},
		stopResults: []*ProviderRuntimeProjection{
			{Generation: 8, State: ProviderRuntimeStateCleanupPending, Stopping: true},
			{Generation: 8, State: ProviderRuntimeStateCleanupComplete, CanStart: true},
		},
		startResult: &ProviderRuntimeProjection{Generation: 9, State: ProviderRuntimeStateStarting},
	}
	snapshots := &providerAPISnapshotFake{}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	request := ProviderSnapshotRestoreRequest{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-1", OperationID: "snapshot-restore-001", ActorID: "actor-1",
	}
	first, err := facade.RestoreSnapshot(context.Background(), request)
	if err != nil || !first.Stopping || snapshots.calls != 0 || runtime.startCalls != 0 {
		t.Fatalf("first restore = %#v, %v restore/start=%d/%d", first, err, snapshots.calls, runtime.startCalls)
	}
	second, err := facade.RestoreSnapshot(context.Background(), request)
	if err != nil || second.Generation != 9 || snapshots.calls != 1 || runtime.startCalls != 1 {
		t.Fatalf("second restore = %#v, %v restore/start=%d/%d", second, err, snapshots.calls, runtime.startCalls)
	}
	if len(runtime.stopInputs) != 2 || runtime.stopInputs[0].OperationID != runtime.stopInputs[1].OperationID || runtime.stopInputs[0].OperationID == runtime.startInput.OperationID {
		t.Fatalf("snapshot suboperations = stop=%#v start=%#v", runtime.stopInputs, runtime.startInput)
	}
}

func TestProviderAPIFacadeSnapshotRestoreResponseLossDoesNotRepeatCompositeOperation(t *testing.T) {
	runtime := &providerAPIRuntimeFake{
		statusResult: &ProviderRuntimeProjection{Generation: 12, State: ProviderRuntimeStateRunning},
		stopResults:  []*ProviderRuntimeProjection{{Generation: 12, State: ProviderRuntimeStateCleanupComplete, CanStart: true}},
		startResult:  &ProviderRuntimeProjection{Generation: 13, State: ProviderRuntimeStateRunning},
	}
	snapshots := &providerAPISnapshotFake{}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	request := ProviderSnapshotRestoreRequest{SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-1", OperationID: "snapshot-response-loss", ActorID: "actor-1"}
	first, err := facade.RestoreSnapshot(context.Background(), request)
	if err != nil || first.Generation != 13 || snapshots.calls != 1 || runtime.startCalls != 1 || len(runtime.stopInputs) != 1 {
		t.Fatalf("first snapshot restore = %#v, %v restore/stop/start=%d/%d/%d", first, err, snapshots.calls, len(runtime.stopInputs), runtime.startCalls)
	}
	runtime.matchFound = true
	runtime.matchResult = &ProviderRuntimeProjection{Generation: 13, State: ProviderRuntimeStateRunning}
	second, err := facade.RestoreSnapshot(context.Background(), request)
	if err != nil || second.Generation != 13 || snapshots.calls != 1 || runtime.startCalls != 1 || len(runtime.stopInputs) != 1 {
		t.Fatalf("snapshot response-loss retry = %#v, %v restore/stop/start=%d/%d/%d", second, err, snapshots.calls, len(runtime.stopInputs), runtime.startCalls)
	}
}

func TestProviderAPIFacadeSnapshotRestoreWithoutRuntimeIsPersistentlyIdempotent(t *testing.T) {
	runtime := &providerAPIRuntimeFake{statusResult: &ProviderRuntimeProjection{State: ProviderRuntimeStateStopped, CanStart: true}}
	snapshots := &providerAPISnapshotFake{}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	request := ProviderSnapshotRestoreRequest{SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-1", OperationID: "snapshot-no-runtime", ActorID: "actor-1"}
	if _, err := facade.RestoreSnapshot(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.RestoreSnapshot(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if snapshots.calls != 1 || len(runtime.stopInputs) != 0 || runtime.startCalls != 0 {
		t.Fatalf("no-runtime retry restore/stop/start=%d/%d/%d", snapshots.calls, len(runtime.stopInputs), runtime.startCalls)
	}
}

func TestProviderAPIFacadeSnapshotRestoreResumesAfterStartFailureWithoutReapplyingFiles(t *testing.T) {
	runtime := &providerAPIRuntimeFake{
		statusResult: &ProviderRuntimeProjection{Generation: 20, State: ProviderRuntimeStateRunning},
		stopResults: []*ProviderRuntimeProjection{
			{Generation: 20, State: ProviderRuntimeStateCleanupComplete, CanStart: true},
			{Generation: 20, State: ProviderRuntimeStateCleanupComplete, CanStart: true},
		},
		startResult: &ProviderRuntimeProjection{Generation: 21, State: ProviderRuntimeStateStarting},
		startErr:    errors.New("provider response lost after start"),
	}
	snapshots := &providerAPISnapshotFake{}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	request := ProviderSnapshotRestoreRequest{SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-1", OperationID: "snapshot-start-loss", ActorID: "actor-1"}
	if _, err := facade.RestoreSnapshot(context.Background(), request); !errors.Is(err, ErrProviderControlUnavailable) {
		t.Fatalf("first start failure = %v", err)
	}
	runtime.startErr = nil
	projection, err := facade.RestoreSnapshot(context.Background(), request)
	if err != nil || projection.Generation != 21 || snapshots.calls != 1 || runtime.startCalls != 2 {
		t.Fatalf("restored-phase retry = %#v, %v restore/start=%d/%d", projection, err, snapshots.calls, runtime.startCalls)
	}
}

func TestProviderAPIFacadeSnapshotRestoreLoadsJournalBeforeStartMatcher(t *testing.T) {
	runtime := &providerAPIRuntimeFake{
		matchFound:  true,
		matchResult: &ProviderRuntimeProjection{Generation: 31, State: ProviderRuntimeStateRunning},
	}
	hash, err := domainappdev.HashProviderSnapshotRestoreOperation("1001", "project-a", "snapshot-1", "snapshot-shared-parent")
	if err != nil {
		t.Fatal(err)
	}
	parentHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation("1001", "project-a", "snapshot-shared-parent")
	if err != nil {
		t.Fatal(err)
	}
	snapshots := &providerAPISnapshotFake{journal: &domainappdev.ProviderSnapshotRestoreJournal{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-1", OperationHash: hash, ParentOperationHash: parentHash,
		Phase: domainappdev.ProviderSnapshotRestorePhaseCompleted, RestartRequired: true,
		RuntimeGeneration: 30, SourceVersion: 1, ResultSourceVersion: 2, StartedGeneration: 31,
	}}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	projection, restoreErr := facade.RestoreSnapshot(context.Background(), ProviderSnapshotRestoreRequest{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-2", OperationID: "snapshot-shared-parent", ActorID: "actor-1",
	})
	if projection != nil || !errors.Is(restoreErr, ErrProviderControlConflict) {
		t.Fatalf("cross-snapshot restore = %#v, %v", projection, restoreErr)
	}
	if len(runtime.matchInputs) != 0 || runtime.startCalls != 0 || len(runtime.stopInputs) != 0 {
		t.Fatalf("provider was called before journal identity validation: match/start/stop=%d/%d/%d", len(runtime.matchInputs), runtime.startCalls, len(runtime.stopInputs))
	}
}

func TestProviderAPIFacadeSnapshotRestoreConvergesRestoredJournalAfterStartResponseLoss(t *testing.T) {
	request := ProviderSnapshotRestoreRequest{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-response-loss", OperationID: "snapshot-response-loss-parent", ActorID: "actor-1",
	}
	hash, err := domainappdev.HashProviderSnapshotRestoreOperation(request.SpaceID, request.ProjectID, request.SnapshotID, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	parentHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation(request.SpaceID, request.ProjectID, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &providerAPIRuntimeFake{
		matchFound:  true,
		matchResult: &ProviderRuntimeProjection{Generation: 42, State: ProviderRuntimeStateRunning},
	}
	snapshots := &providerAPISnapshotFake{sourceVersion: 2, journal: &domainappdev.ProviderSnapshotRestoreJournal{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID, OperationHash: hash, ParentOperationHash: parentHash,
		Phase: domainappdev.ProviderSnapshotRestorePhaseRestored, RestartRequired: true,
		RuntimeGeneration: 41, SourceVersion: 1, ResultSourceVersion: 2,
	}}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	projection, restoreErr := facade.RestoreSnapshot(context.Background(), request)
	if restoreErr != nil || projection == nil || projection.Generation != 42 {
		t.Fatalf("response-loss convergence = %#v, %v", projection, restoreErr)
	}
	if runtime.startCalls != 0 || snapshots.markStartedCalls != 1 || snapshots.completeCalls != 1 || snapshots.journal.Phase != domainappdev.ProviderSnapshotRestorePhaseCompleted {
		t.Fatalf("response-loss convergence start/mark/complete/phase=%d/%d/%d/%s", runtime.startCalls, snapshots.markStartedCalls, snapshots.completeCalls, snapshots.journal.Phase)
	}
}

func TestProviderAPIFacadeSnapshotRestoreRejectsPendingJournalWithMatchedStart(t *testing.T) {
	request := ProviderSnapshotRestoreRequest{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-pending", OperationID: "snapshot-pending-parent", ActorID: "actor-1",
	}
	hash, err := domainappdev.HashProviderSnapshotRestoreOperation(request.SpaceID, request.ProjectID, request.SnapshotID, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	parentHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation(request.SpaceID, request.ProjectID, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &providerAPIRuntimeFake{matchFound: true, matchResult: &ProviderRuntimeProjection{Generation: 50, State: ProviderRuntimeStateRunning}}
	snapshots := &providerAPISnapshotFake{sourceVersion: 1, journal: &domainappdev.ProviderSnapshotRestoreJournal{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID, OperationHash: hash, ParentOperationHash: parentHash,
		Phase: domainappdev.ProviderSnapshotRestorePhasePending, RestartRequired: true, RuntimeGeneration: 49, SourceVersion: 1,
	}}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	projection, restoreErr := facade.RestoreSnapshot(context.Background(), request)
	if projection != nil || !errors.Is(restoreErr, ErrProviderControlConflict) || snapshots.calls != 0 || runtime.startCalls != 0 {
		t.Fatalf("pending matched restore = %#v, %v apply/start=%d/%d", projection, restoreErr, snapshots.calls, runtime.startCalls)
	}
}

func TestProviderAPIFacadeSnapshotRestoreCompletedRetryFencesStartedGeneration(t *testing.T) {
	request := ProviderSnapshotRestoreRequest{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-completed", OperationID: "snapshot-completed-parent", ActorID: "actor-1",
	}
	hash, err := domainappdev.HashProviderSnapshotRestoreOperation(request.SpaceID, request.ProjectID, request.SnapshotID, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	parentHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation(request.SpaceID, request.ProjectID, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &providerAPIRuntimeFake{matchFound: true, matchResult: &ProviderRuntimeProjection{Generation: 62, State: ProviderRuntimeStateRunning}}
	snapshots := &providerAPISnapshotFake{sourceVersion: 2, journal: &domainappdev.ProviderSnapshotRestoreJournal{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID, OperationHash: hash, ParentOperationHash: parentHash,
		Phase: domainappdev.ProviderSnapshotRestorePhaseCompleted, RestartRequired: true,
		RuntimeGeneration: 60, SourceVersion: 1, ResultSourceVersion: 2, StartedGeneration: 61,
	}}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	projection, restoreErr := facade.RestoreSnapshot(context.Background(), request)
	if projection != nil || !errors.Is(restoreErr, ErrProviderControlConflict) || snapshots.completeCalls != 0 {
		t.Fatalf("completed generation mismatch = %#v, %v complete=%d", projection, restoreErr, snapshots.completeCalls)
	}
}

func TestProviderAPIFacadeSnapshotRestoreCompletedAllowsNewParentAndSnapshot(t *testing.T) {
	oldParent, err := domainappdev.HashProviderSnapshotRestoreParentOperation("1001", "project-a", "snapshot-old-parent")
	if err != nil {
		t.Fatal(err)
	}
	oldFull, err := domainappdev.HashProviderSnapshotRestoreOperation("1001", "project-a", "snapshot-old", "snapshot-old-parent")
	if err != nil {
		t.Fatal(err)
	}
	runtime := &providerAPIRuntimeFake{statusResult: &ProviderRuntimeProjection{State: ProviderRuntimeStateStopped, CanStart: true}}
	snapshots := &providerAPISnapshotFake{sourceVersion: 2, journal: &domainappdev.ProviderSnapshotRestoreJournal{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-old", OperationHash: oldFull, ParentOperationHash: oldParent,
		Phase: domainappdev.ProviderSnapshotRestorePhaseCompleted, SourceVersion: 1, ResultSourceVersion: 2,
	}}
	facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
	projection, restoreErr := facade.RestoreSnapshot(context.Background(), ProviderSnapshotRestoreRequest{
		SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-new", OperationID: "snapshot-new-parent", ActorID: "actor-1",
	})
	if restoreErr != nil || projection == nil || !projection.CanStart || snapshots.calls != 1 || snapshots.journal.Phase != domainappdev.ProviderSnapshotRestorePhaseCompleted {
		t.Fatalf("new restore after completed = %#v, %v apply=%d phase=%s", projection, restoreErr, snapshots.calls, snapshots.journal.Phase)
	}
}

func TestProviderSnapshotRestoreSuboperationsBindSnapshotIdentity(t *testing.T) {
	request := ProviderSnapshotRestoreRequest{SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-a", OperationID: "snapshot-parent-operation", ActorID: "actor-1"}
	first := providerSnapshotRestoreStableID("start", request)
	request.SnapshotID = "snapshot-b"
	second := providerSnapshotRestoreStableID("start", request)
	if first == second {
		t.Fatalf("snapshot-bound suboperation IDs matched: %q", first)
	}
	for _, value := range []string{first, second} {
		if !strings.HasPrefix(value, "appdev_api_snapshot_start_") || len(value) != len("appdev_api_snapshot_start_")+sha256.Size*2 {
			t.Fatalf("unsafe snapshot suboperation ID %q", value)
		}
	}
}

func TestProviderExecutionReadyArtifactUsesDedicatedArtifactTimestamp(t *testing.T) {
	artifactAt := time.Date(2026, 7, 17, 5, 0, 0, 123000000, time.UTC)
	rowUpdatedAt := artifactAt.Add(3 * time.Hour)
	record := &domainappdev.ProviderExecution{
		Generation: 4, ArtifactStatus: domainappdev.ProviderExecutionArtifactReady,
		ArtifactKind:   domainappdev.ProviderExecutionArtifactKindAppDevBuildArchive,
		ArtifactDigest: "sha256:" + strings.Repeat("a", 64), ArtifactSize: 32,
		ArtifactObjectKey: "appdev/builds/internal/release.zip", ArtifactUpdatedAt: &artifactAt, UpdatedAt: rowUpdatedAt,
	}
	repository := &providerAPIArtifactTimeRepository{record: record}
	service, err := NewProviderExecutionService(repository, providerAPIArtifactTimeCodec{})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := service.LoadReadyArtifact(context.Background(), LoadReadyProviderArtifactRequest{SpaceID: "1001", ProjectID: "project-a"})
	if err != nil || ready == nil || !ready.UpdatedAt.Equal(artifactAt) {
		t.Fatalf("ready artifact timestamp = %#v, %v; row updated=%s", ready, err, rowUpdatedAt)
	}
	record.ArtifactUpdatedAt = nil
	if ready, err := service.LoadReadyArtifact(context.Background(), LoadReadyProviderArtifactRequest{SpaceID: "1001", ProjectID: "project-a"}); ready != nil || !errors.Is(err, ErrProviderControlUnavailable) {
		t.Fatalf("NULL artifact timestamp = %#v, %v", ready, err)
	}
}

func TestProviderReadyReleaseReaderValidatesStreamAndMarksStale(t *testing.T) {
	payload := []byte("validated provider release")
	digest := sha256.Sum256(payload)
	updatedAt := time.Now().UTC().Add(-time.Minute)
	source := &providerAPIReadyArtifactFake{artifact: &ProviderReadyArtifact{
		Generation: 5, Size: int64(len(payload)), UpdatedAt: updatedAt,
		objectKey: "tenant/internal/secret-object.zip", digest: "sha256:" + hex.EncodeToString(digest[:]),
	}}
	projects := &providerAPIReleaseProjectFake{project: &domainappdev.Project{
		ID: "project-a", SpaceID: "1001", Name: "Safe Project", SourceUpdatedAt: updatedAt.Add(time.Second),
	}}
	stream := &providerAPITrackingReadCloser{Reader: bytes.NewReader(payload)}
	store := &providerAPIArtifactStoreFake{read: &ArtifactObjectRead{Body: stream, Size: int64(len(payload))}}
	reader, err := NewProviderReadyReleaseReader(source, projects, store)
	if err != nil {
		t.Fatal(err)
	}
	release, err := reader.OpenRelease(context.Background(), ProviderReleaseRequest{SpaceID: "1001", ProjectID: "project-a"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(release)
	if err != nil || !bytes.Equal(content, payload) || !release.Stale || release.Size != int64(len(payload)) || release.ContentType != "application/zip" {
		t.Fatalf("release = %#v, content=%q, err=%v", release, content, err)
	}
	if !stream.closed {
		t.Fatal("storage stream was not closed after validation")
	}
	if err := release.Close(); err != nil {
		t.Fatal(err)
	}
	formatted := fmt.Sprintf("%v|%+v|%#v", release, release, release)
	if strings.Contains(formatted, source.artifact.objectKey) || strings.Contains(formatted, source.artifact.digest) {
		t.Fatalf("release formatting leaked internal facts: %s", formatted)
	}
}

func TestProviderReadyReleaseReaderTenantMismatchAndFailuresAreSafe(t *testing.T) {
	payload := []byte("release")
	digest := sha256.Sum256(payload)
	source := &providerAPIReadyArtifactFake{artifact: &ProviderReadyArtifact{
		Generation: 1, Size: int64(len(payload)), UpdatedAt: time.Now().UTC(),
		objectKey: "tenant/internal/secret-object.zip", digest: "sha256:" + hex.EncodeToString(digest[:]),
	}}
	projects := &providerAPIReleaseProjectFake{project: &domainappdev.Project{ID: "project-a", SpaceID: "1001", Name: "Safe"}}
	store := &providerAPIArtifactStoreFake{read: &ArtifactObjectRead{Body: &providerAPITrackingReadCloser{Reader: strings.NewReader("tampered")}, Size: 8}}
	reader, err := NewProviderReadyReleaseReader(source, projects, store)
	if err != nil {
		t.Fatal(err)
	}
	if release, err := reader.OpenRelease(context.Background(), ProviderReleaseRequest{SpaceID: "1002", ProjectID: "project-a"}); release != nil || !errors.Is(err, ErrProviderControlNotFound) {
		t.Fatalf("cross-tenant release = %#v, %v", release, err)
	}
	release, tamperErr := reader.OpenRelease(context.Background(), ProviderReleaseRequest{SpaceID: "1001", ProjectID: "project-a"})
	if release != nil || !errors.Is(tamperErr, ErrProviderControlUnavailable) {
		t.Fatalf("tampered release = %#v, %v", release, tamperErr)
	}
	if strings.Contains(tamperErr.Error(), source.artifact.objectKey) || strings.Contains(tamperErr.Error(), source.artifact.digest) {
		t.Fatalf("release error leaked internal facts: %v", tamperErr)
	}
}

func TestProviderReadyReleaseReaderStorageAndCloseErrorsAreSafe(t *testing.T) {
	payload := []byte("release")
	digest := sha256.Sum256(payload)
	source := &providerAPIReadyArtifactFake{artifact: &ProviderReadyArtifact{
		Generation: 1, Size: int64(len(payload)), UpdatedAt: time.Now().UTC(), objectKey: "internal/secret/key.zip",
		digest: "sha256:" + hex.EncodeToString(digest[:]),
	}}
	projects := &providerAPIReleaseProjectFake{project: &domainappdev.Project{ID: "project-a", SpaceID: "1001", Name: "Safe"}}

	t.Run("storage", func(t *testing.T) {
		reader, err := NewProviderReadyReleaseReader(source, projects, &providerAPIArtifactStoreFake{err: errors.New("raw storage URI internal/secret/key.zip")})
		if err != nil {
			t.Fatal(err)
		}
		release, openErr := reader.OpenRelease(context.Background(), ProviderReleaseRequest{SpaceID: "1001", ProjectID: "project-a"})
		if release != nil || !errors.Is(openErr, ErrProviderControlUnavailable) || strings.Contains(openErr.Error(), "internal/secret") {
			t.Fatalf("storage failure = %#v, %v", release, openErr)
		}
	})

	t.Run("source close", func(t *testing.T) {
		stream := &providerAPITrackingReadCloser{Reader: bytes.NewReader(payload), closeErr: errors.New("raw close internal/secret/key.zip")}
		reader, err := NewProviderReadyReleaseReader(source, projects, &providerAPIArtifactStoreFake{
			read: &ArtifactObjectRead{Body: stream, Size: int64(len(payload))},
		})
		if err != nil {
			t.Fatal(err)
		}
		release, openErr := reader.OpenRelease(context.Background(), ProviderReleaseRequest{SpaceID: "1001", ProjectID: "project-a"})
		if release != nil || !errors.Is(openErr, ErrProviderControlUnavailable) || !stream.closed || strings.Contains(openErr.Error(), "internal/secret") {
			t.Fatalf("close failure = %#v, %v, closed=%t", release, openErr, stream.closed)
		}
	})
}

func TestProviderPublicLegacyDTOsHideInternalArtifactAndPreview(t *testing.T) {
	encoded, err := json.Marshal(struct {
		Project       ProjectDTO               `json:"project"`
		Runtime       RuntimeInfoDTO           `json:"runtime"`
		Build         BuildProjectData         `json:"build"`
		DomainProject domainappdev.Project     `json:"domainProject"`
		DomainRuntime domainappdev.RuntimeInfo `json:"domainRuntime"`
	}{
		Project:       ProjectDTO{PreviewURL: "https://internal/preview", LastBuildArtifact: "internal/object/key"},
		Runtime:       RuntimeInfoDTO{PreviewURL: "https://internal/runtime"},
		Build:         BuildProjectData{ArtifactPath: "/private/release", DownloadURL: "https://internal/download"},
		DomainProject: domainappdev.Project{PreviewURL: "https://internal/domain-preview", LastBuildArtifact: "internal/domain-object"},
		DomainRuntime: domainappdev.RuntimeInfo{PreviewURL: "https://internal/domain-runtime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"previewUrl", "lastBuildArtifact", "artifactPath", "downloadUrl", "internal/object", "/private/release"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("legacy DTO leaked %q: %s", forbidden, encoded)
		}
	}
}

type providerAPIRuntimeFake struct {
	startResult  *ProviderRuntimeProjection
	statusResult *ProviderRuntimeProjection
	stopResults  []*ProviderRuntimeProjection
	startErr     error
	statusErr    error
	stopErr      error
	startInput   ProviderRuntimeStartInput
	statusInput  ProviderRuntimeStatusInput
	stopInputs   []ProviderRuntimeStopInput
	startCalls   int
	matchFound   bool
	matchResult  *ProviderRuntimeProjection
	matchErr     error
	matchInputs  []ProviderRuntimeStartInput
}

func (fake *providerAPIRuntimeFake) Start(_ context.Context, input ProviderRuntimeStartInput) (*ProviderRuntimeProjection, error) {
	fake.startCalls++
	fake.startInput = input
	if fake.startErr == nil && fake.startResult != nil {
		fake.matchFound = true
		fake.matchResult = fake.startResult
	}
	return fake.startResult, fake.startErr
}
func (fake *providerAPIRuntimeFake) Status(_ context.Context, input ProviderRuntimeStatusInput) (*ProviderRuntimeProjection, error) {
	fake.statusInput = input
	return fake.statusResult, fake.statusErr
}
func (fake *providerAPIRuntimeFake) Stop(_ context.Context, input ProviderRuntimeStopInput) (*ProviderRuntimeProjection, error) {
	fake.stopInputs = append(fake.stopInputs, input)
	if len(fake.stopResults) == 0 {
		return nil, fake.stopErr
	}
	result := fake.stopResults[0]
	if len(fake.stopResults) > 1 {
		fake.stopResults = fake.stopResults[1:]
	}
	return result, fake.stopErr
}

func (fake *providerAPIRuntimeFake) MatchCurrentStartOperation(_ context.Context, input ProviderRuntimeStartInput) (*ProviderRuntimeProjection, bool, error) {
	fake.matchInputs = append(fake.matchInputs, input)
	return fake.matchResult, fake.matchFound, fake.matchErr
}

type providerAPIBuildFake struct {
	beginResult   *ProviderBuildProjection
	pollResult    *ProviderBuildProjection
	recoverResult *ProviderBuildProjection
	beginInput    ProviderBuildBeginInput
	pollInput     ProviderBuildPollInput
}

func (fake *providerAPIBuildFake) BeginBuild(_ context.Context, input ProviderBuildBeginInput) (*ProviderBuildProjection, error) {
	fake.beginInput = input
	return fake.beginResult, nil
}
func (fake *providerAPIBuildFake) PollBuild(_ context.Context, input ProviderBuildPollInput) (*ProviderBuildProjection, error) {
	fake.pollInput = input
	return fake.pollResult, nil
}
func (fake *providerAPIBuildFake) RecoverBuild(context.Context, ProviderBuildRecoverInput) (*ProviderBuildProjection, error) {
	return fake.recoverResult, nil
}

type providerAPISnapshotFake struct {
	calls            int
	markStartedCalls int
	completeCalls    int
	sourceVersion    int64
	journal          *domainappdev.ProviderSnapshotRestoreJournal
}

func (fake *providerAPISnapshotFake) LoadProviderSnapshotRestore(_ context.Context, input domainappdev.LoadProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if fake.journal == nil {
		return nil, domainappdev.ErrNotFound
	}
	if fake.journal.SpaceID != input.SpaceID || fake.journal.ProjectID != input.ProjectID {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	if fake.journal.ParentOperationHash.Equal(input.ParentOperationHash) {
		if fake.journal.SnapshotID != input.SnapshotID || !fake.journal.OperationHash.Equal(input.OperationHash) {
			return nil, domainappdev.ErrProviderSnapshotRestoreConflict
		}
	} else if fake.journal.Phase == domainappdev.ProviderSnapshotRestorePhaseCompleted || fake.journal.Phase == domainappdev.ProviderSnapshotRestorePhaseFailed {
		return nil, domainappdev.ErrNotFound
	} else {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	copy := *fake.journal
	return &copy, nil
}

func (fake *providerAPISnapshotFake) ReserveProviderSnapshotRestore(_ context.Context, input domainappdev.ReserveProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if fake.journal != nil {
		journal, err := fake.LoadProviderSnapshotRestore(context.Background(), domainappdev.LoadProviderSnapshotRestoreInput{
			SpaceID: input.SpaceID, ProjectID: input.ProjectID, SnapshotID: input.SnapshotID,
			OperationHash: input.OperationHash, ParentOperationHash: input.ParentOperationHash,
		})
		if err == nil || !errors.Is(err, domainappdev.ErrNotFound) {
			return journal, err
		}
	}
	if fake.sourceVersion == 0 {
		fake.sourceVersion = 1
	}
	fake.journal = &domainappdev.ProviderSnapshotRestoreJournal{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID, SnapshotID: input.SnapshotID,
		OperationHash: input.OperationHash, ParentOperationHash: input.ParentOperationHash, Phase: domainappdev.ProviderSnapshotRestorePhasePending,
		RuntimeGeneration: input.RuntimeGeneration, RestartRequired: input.RestartRequired, SourceVersion: fake.sourceVersion,
	}
	copy := *fake.journal
	return &copy, nil
}

func (fake *providerAPISnapshotFake) ApplyProviderSnapshotRestore(_ context.Context, input domainappdev.ApplyProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if fake.journal == nil || !fake.journal.OperationHash.Equal(input.OperationHash) || !fake.journal.ParentOperationHash.Equal(input.ParentOperationHash) || fake.journal.Phase != domainappdev.ProviderSnapshotRestorePhasePending || input.ExpectedSourceVersion != fake.journal.SourceVersion {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	fake.calls++
	fake.sourceVersion++
	fake.journal.ResultSourceVersion = fake.sourceVersion
	fake.journal.Phase = domainappdev.ProviderSnapshotRestorePhaseRestored
	copy := *fake.journal
	return &copy, nil
}

func (fake *providerAPISnapshotFake) FailProviderSnapshotRestore(_ context.Context, input domainappdev.FailProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if fake.journal == nil || !fake.journal.OperationHash.Equal(input.OperationHash) || !fake.journal.ParentOperationHash.Equal(input.ParentOperationHash) ||
		fake.journal.SnapshotID != input.SnapshotID || fake.journal.SourceVersion != input.ExpectedSourceVersion || fake.journal.Phase != domainappdev.ProviderSnapshotRestorePhasePending {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	fake.journal.Phase = domainappdev.ProviderSnapshotRestorePhaseFailed
	fake.journal.SafeErrorCode = input.SafeErrorCode
	fake.journal.SafeErrorMessage = input.SafeErrorMessage
	copy := *fake.journal
	return &copy, nil
}

func (fake *providerAPISnapshotFake) MarkProviderSnapshotRestoreStarted(_ context.Context, input domainappdev.MarkProviderSnapshotRestoreStartedInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if fake.journal == nil || !fake.journal.OperationHash.Equal(input.OperationHash) || !fake.journal.ParentOperationHash.Equal(input.ParentOperationHash) || fake.journal.Phase != domainappdev.ProviderSnapshotRestorePhaseRestored || input.ExpectedSourceVersion != fake.journal.ResultSourceVersion {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	fake.markStartedCalls++
	fake.journal.Phase = domainappdev.ProviderSnapshotRestorePhaseStarted
	fake.journal.StartedGeneration = input.StartedGeneration
	copy := *fake.journal
	return &copy, nil
}

func (fake *providerAPISnapshotFake) CompleteProviderSnapshotRestore(_ context.Context, input domainappdev.CompleteProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	if fake.journal == nil || !fake.journal.OperationHash.Equal(input.OperationHash) || !fake.journal.ParentOperationHash.Equal(input.ParentOperationHash) || input.ExpectedSourceVersion != fake.journal.ResultSourceVersion {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	if fake.journal.Phase == domainappdev.ProviderSnapshotRestorePhaseCompleted {
		if fake.journal.RestartRequired && input.StartedGeneration != fake.journal.StartedGeneration {
			return nil, domainappdev.ErrProviderSnapshotRestoreConflict
		}
		fake.completeCalls++
		copy := *fake.journal
		return &copy, nil
	}
	if fake.journal.RestartRequired && (fake.journal.Phase != domainappdev.ProviderSnapshotRestorePhaseStarted || input.StartedGeneration != fake.journal.StartedGeneration) {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	if !fake.journal.RestartRequired && fake.journal.Phase != domainappdev.ProviderSnapshotRestorePhaseRestored && fake.journal.Phase != domainappdev.ProviderSnapshotRestorePhaseCompleted {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	fake.completeCalls++
	fake.journal.Phase = domainappdev.ProviderSnapshotRestorePhaseCompleted
	copy := *fake.journal
	return &copy, nil
}

type providerAPIArtifactTimeRepository struct {
	domainappdev.ProviderExecutionRepository
	record *domainappdev.ProviderExecution
}

func (repository *providerAPIArtifactTimeRepository) LoadLatestReadyArtifact(context.Context, domainappdev.LoadReadyProviderArtifactInput) (*domainappdev.ProviderExecution, error) {
	return repository.record, nil
}

type providerAPIArtifactTimeCodec struct{}

func (providerAPIArtifactTimeCodec) Seal(context.Context, applicationsandbox.ExecutionCheckpointBinding, applicationsandbox.ExecutionCheckpoint) (string, error) {
	return "", errors.New("unused")
}

func (providerAPIArtifactTimeCodec) Open(context.Context, applicationsandbox.ExecutionCheckpointBinding, string) (applicationsandbox.ExecutionCheckpoint, error) {
	return applicationsandbox.ExecutionCheckpoint{}, errors.New("unused")
}

type providerAPIReadyArtifactFake struct{ artifact *ProviderReadyArtifact }

func (fake *providerAPIReadyArtifactFake) LoadReadyArtifact(_ context.Context, request LoadReadyProviderArtifactRequest) (*ProviderReadyArtifact, error) {
	if request.SpaceID != "1001" || request.ProjectID != "project-a" {
		return nil, domainappdev.ErrProviderExecutionNotFound
	}
	return fake.artifact, nil
}

type providerAPIReleaseProjectFake struct{ project *domainappdev.Project }

func (fake *providerAPIReleaseProjectFake) GetProject(_ context.Context, spaceID, projectID string) (*domainappdev.Project, error) {
	if fake.project == nil || fake.project.SpaceID != spaceID || fake.project.ID != projectID {
		return nil, errors.New("project unavailable")
	}
	return fake.project, nil
}

type providerAPIArtifactStoreFake struct {
	read *ArtifactObjectRead
	err  error
}

func (*providerAPIArtifactStoreFake) PutArtifact(context.Context, string, io.Reader, int64) error {
	return errors.New("not supported")
}
func (fake *providerAPIArtifactStoreFake) OpenArtifact(context.Context, string, int64) (*ArtifactObjectRead, error) {
	return fake.read, fake.err
}

type providerAPITrackingReadCloser struct {
	io.Reader
	closed   bool
	closeErr error
}

func (reader *providerAPITrackingReadCloser) Close() error {
	reader.closed = true
	return reader.closeErr
}

type providerAPILegacyRuntimeFake struct{ calls int }

func (fake *providerAPILegacyRuntimeFake) Start(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	fake.calls++
	return nil, errors.New("legacy runtime must not be called")
}
func (fake *providerAPILegacyRuntimeFake) Status(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	fake.calls++
	return nil, errors.New("legacy runtime must not be called")
}
func (fake *providerAPILegacyRuntimeFake) KeepAlive(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	fake.calls++
	return nil, errors.New("legacy runtime must not be called")
}
func (fake *providerAPILegacyRuntimeFake) Restart(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	fake.calls++
	return nil, errors.New("legacy runtime must not be called")
}
func (fake *providerAPILegacyRuntimeFake) Stop(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	fake.calls++
	return nil, errors.New("legacy runtime must not be called")
}
func (fake *providerAPILegacyRuntimeFake) Logs(context.Context, *RuntimeManagerRequest) ([]*domainappdev.RuntimeLog, error) {
	fake.calls++
	return nil, errors.New("legacy runtime must not be called")
}
