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
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestProviderRuntimeStopRejectsExpectedGenerationBeforeSideEffects(t *testing.T) {
	orchestrator, ledger, router, selection := runtimeStopFixture(t)
	expected := ledger.metadata.Generation + 1
	input := runtimeStopInput()
	input.ExpectedGeneration = &expected

	projection, err := orchestrator.Stop(context.Background(), input)
	if projection != nil || !errors.Is(err, ErrProviderRuntimeConflict) {
		t.Fatalf("generation-fenced stop = %#v, %v", projection, err)
	}
	if ledger.renewCount != 0 || ledger.setStopCount != 0 || router.resumeCount != 0 || selection.cancelCount != 0 || selection.releaseAttempts != 0 {
		t.Fatalf("stale stop reached side effects renew/set/resume/cancel/release=%d/%d/%d/%d/%d", ledger.renewCount, ledger.setStopCount, router.resumeCount, selection.cancelCount, selection.releaseAttempts)
	}
}

func TestProviderAPIFacadeCompositeStopsFenceObservedGeneration(t *testing.T) {
	t.Run("ordinary stop", func(t *testing.T) {
		runtime := &providerAPIGenerationFenceRuntime{observedGeneration: 7, currentGeneration: 7}
		facade := NewProviderAPIFacade(runtime, nil, nil, nil)
		_, err := facade.StopRuntime(context.Background(), ProviderRuntimeMutationRequest{
			SpaceID: "1001", ProjectID: "project-a", OperationID: "generation-stop-001", ActorID: "actor-1",
		})
		if err != nil || runtime.stopSideEffects != 1 || len(runtime.stopInputs) != 1 || runtime.stopInputs[0].ExpectedGeneration == nil || *runtime.stopInputs[0].ExpectedGeneration != 7 {
			t.Fatalf("ordinary stop fence inputs=%#v effects=%d err=%v", runtime.stopInputs, runtime.stopSideEffects, err)
		}
	})

	t.Run("restart cannot stop a replacement generation", func(t *testing.T) {
		runtime := &providerAPIGenerationFenceRuntime{observedGeneration: 7, currentGeneration: 7, advanceBeforeStop: true}
		facade := NewProviderAPIFacade(runtime, nil, nil, nil)
		projection, err := facade.RestartRuntime(context.Background(), ProviderRuntimeMutationRequest{
			SpaceID: "1001", ProjectID: "project-a", OperationID: "generation-restart-001", ActorID: "actor-1",
		})
		if projection != nil || !errors.Is(err, ErrProviderControlConflict) || runtime.stopSideEffects != 0 || runtime.startCalls != 0 {
			t.Fatalf("restart generation race = %#v, %v stop/start=%d/%d", projection, err, runtime.stopSideEffects, runtime.startCalls)
		}
		if len(runtime.stopInputs) != 1 || runtime.stopInputs[0].ExpectedGeneration == nil || *runtime.stopInputs[0].ExpectedGeneration != 7 {
			t.Fatalf("restart expected generation = %#v", runtime.stopInputs)
		}
	})

	t.Run("snapshot restore cannot stop a replacement generation", func(t *testing.T) {
		runtime := &providerAPIGenerationFenceRuntime{observedGeneration: 11, currentGeneration: 11, advanceBeforeStop: true}
		snapshots := &providerAPISnapshotFake{}
		facade := NewProviderAPIFacade(runtime, nil, nil, snapshots)
		projection, err := facade.RestoreSnapshot(context.Background(), ProviderSnapshotRestoreRequest{
			SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-generation", OperationID: "generation-restore-001", ActorID: "actor-1",
		})
		if projection != nil || !errors.Is(err, ErrProviderControlConflict) || runtime.stopSideEffects != 0 || snapshots.calls != 0 {
			t.Fatalf("restore generation race = %#v, %v stop/apply=%d/%d", projection, err, runtime.stopSideEffects, snapshots.calls)
		}
		if len(runtime.stopInputs) != 1 || runtime.stopInputs[0].ExpectedGeneration == nil || *runtime.stopInputs[0].ExpectedGeneration != 11 {
			t.Fatalf("restore expected generation = %#v", runtime.stopInputs)
		}
	})
}

func TestProviderAPIFacadeSnapshotFailureClassification(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		shouldFail bool
	}{
		{name: "invalid", err: domainappdev.ErrProviderSnapshotRestoreInvalid, shouldFail: true},
		{name: "conflict", err: domainappdev.ErrProviderSnapshotRestoreConflict, shouldFail: true},
		{name: "not found", err: domainappdev.ErrNotFound, shouldFail: true},
		{name: "unavailable", err: domainappdev.ErrProviderSnapshotRestoreUnavailable},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "canceled", err: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &providerAPIRuntimeFake{statusResult: &ProviderRuntimeProjection{State: ProviderRuntimeStateStopped, CanStart: true}}
			store := &providerSnapshotFailureStore{providerAPISnapshotFake: &providerAPISnapshotFake{}, applyErr: test.err}
			facade := NewProviderAPIFacade(runtime, nil, nil, store)
			_, err := facade.RestoreSnapshot(context.Background(), ProviderSnapshotRestoreRequest{
				SpaceID: "1001", ProjectID: "project-a", SnapshotID: "snapshot-failure", OperationID: "snapshot-failure-001", ActorID: "actor-1",
			})
			if err == nil {
				t.Fatal("restore failure was hidden")
			}
			expectedCalls := 0
			if test.shouldFail {
				expectedCalls = 1
			}
			if store.failCalls != expectedCalls {
				t.Fatalf("FailProviderSnapshotRestore calls=%d want=%d err=%v", store.failCalls, expectedCalls, err)
			}
		})
	}
}

func TestProviderControlNotFoundMapsTenantScopedDomainNotFound(t *testing.T) {
	if err := normalizeProviderControlError(context.Background(), domainappdev.ErrNotFound); !errors.Is(err, ErrProviderControlNotFound) {
		t.Fatalf("domain not found mapped to %v", err)
	}
}

func TestProviderBuildAPIProjectionComputesStaleFromAuthoritativeProject(t *testing.T) {
	artifactTime := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	build := &providerAPIBuildFake{beginResult: &ProviderBuildProjection{
		Generation: 9, State: ProviderBuildStateReady, ReleaseAvailable: true, Size: 64, UpdatedAt: artifactTime,
	}, pollResult: &ProviderBuildProjection{
		Generation: 9, State: ProviderBuildStateReady, ReleaseAvailable: true, Size: 64, UpdatedAt: artifactTime,
	}}
	source := &providerBuildSourceSnapshotStore{
		providerAPISnapshotFake: &providerAPISnapshotFake{},
		project:                 &domainappdev.Project{ID: "project-a", SpaceID: "1001", SourceUpdatedAt: artifactTime.Add(-time.Second)},
	}
	facade := NewProviderAPIFacade(nil, build, nil, source)
	fresh, err := facade.BeginBuild(context.Background(), ProviderBuildOperationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "build-stale-check-001"})
	if err != nil || fresh.Stale {
		t.Fatalf("fresh build = %#v, %v", fresh, err)
	}
	source.project.SourceUpdatedAt = artifactTime.Add(time.Second)
	stale, err := facade.PollBuild(context.Background(), ProviderBuildOperationRequest{SpaceID: "1001", ProjectID: "project-a", OperationID: "build-stale-check-001"})
	if err != nil || !stale.Stale {
		t.Fatalf("stale build = %#v, %v", stale, err)
	}
}

type providerAPIGenerationFenceRuntime struct {
	observedGeneration uint64
	currentGeneration  uint64
	advanceBeforeStop  bool
	stopSideEffects    int
	startCalls         int
	stopInputs         []ProviderRuntimeStopInput
}

func (runtime *providerAPIGenerationFenceRuntime) Start(context.Context, ProviderRuntimeStartInput) (*ProviderRuntimeProjection, error) {
	runtime.startCalls++
	return &ProviderRuntimeProjection{Generation: runtime.currentGeneration + 1, State: ProviderRuntimeStateRunning}, nil
}

func (runtime *providerAPIGenerationFenceRuntime) Status(context.Context, ProviderRuntimeStatusInput) (*ProviderRuntimeProjection, error) {
	return &ProviderRuntimeProjection{Generation: runtime.observedGeneration, State: ProviderRuntimeStateRunning}, nil
}

func (runtime *providerAPIGenerationFenceRuntime) Stop(_ context.Context, input ProviderRuntimeStopInput) (*ProviderRuntimeProjection, error) {
	runtime.stopInputs = append(runtime.stopInputs, input)
	if runtime.advanceBeforeStop {
		runtime.currentGeneration++
		runtime.advanceBeforeStop = false
	}
	if input.ExpectedGeneration == nil || *input.ExpectedGeneration != runtime.currentGeneration {
		return nil, ErrProviderRuntimeConflict
	}
	runtime.stopSideEffects++
	return &ProviderRuntimeProjection{Generation: runtime.currentGeneration, State: ProviderRuntimeStateCleanupComplete, CanStart: true}, nil
}

func (*providerAPIGenerationFenceRuntime) MatchCurrentStartOperation(context.Context, ProviderRuntimeStartInput) (*ProviderRuntimeProjection, bool, error) {
	return nil, false, nil
}

type providerSnapshotFailureStore struct {
	*providerAPISnapshotFake
	applyErr  error
	failCalls int
}

func (store *providerSnapshotFailureStore) ApplyProviderSnapshotRestore(context.Context, domainappdev.ApplyProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	return nil, store.applyErr
}

func (store *providerSnapshotFailureStore) FailProviderSnapshotRestore(_ context.Context, input domainappdev.FailProviderSnapshotRestoreInput) (*domainappdev.ProviderSnapshotRestoreJournal, error) {
	store.failCalls++
	if store.journal == nil || !store.journal.OperationHash.Equal(input.OperationHash) || !store.journal.ParentOperationHash.Equal(input.ParentOperationHash) {
		return nil, domainappdev.ErrProviderSnapshotRestoreConflict
	}
	copy := *store.journal
	copy.Phase = domainappdev.ProviderSnapshotRestorePhaseFailed
	copy.SafeErrorCode = input.SafeErrorCode
	copy.SafeErrorMessage = input.SafeErrorMessage
	store.journal = &copy
	return &copy, nil
}

type providerBuildSourceSnapshotStore struct {
	*providerAPISnapshotFake
	project *domainappdev.Project
}

func (store *providerBuildSourceSnapshotStore) GetProject(_ context.Context, spaceID, projectID string) (*domainappdev.Project, error) {
	if store.project == nil || store.project.SpaceID != spaceID || store.project.ID != projectID {
		return nil, domainappdev.ErrNotFound
	}
	copy := *store.project
	return &copy, nil
}
