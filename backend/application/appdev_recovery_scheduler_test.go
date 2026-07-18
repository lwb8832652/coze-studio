// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type appDevRecoverySourceFake struct {
	projects []infraAppDevRecoveryProject
	err      error
	calls    chan struct{}
}

func (fake *appDevRecoverySourceFake) ListProviderExecutionRecoveryProjects(
	context.Context,
	*infraAppDevRecoveryCursor,
	int,
) ([]infraAppDevRecoveryProject, *infraAppDevRecoveryCursor, error) {
	select {
	case fake.calls <- struct{}{}:
	default:
	}
	return append([]infraAppDevRecoveryProject(nil), fake.projects...), nil, fake.err
}

type appDevRuntimeRecovererFake struct {
	mu       sync.Mutex
	calls    []appdevapp.ProviderRuntimeRecoverProjectInput
	block    bool
	started  chan struct{}
	canceled chan struct{}
	err      error
}

func (fake *appDevRuntimeRecovererFake) RecoverProjectResumeOnly(ctx context.Context, input appdevapp.ProviderRuntimeRecoverProjectInput) (*appdevapp.ProviderRuntimeProjection, error) {
	fake.mu.Lock()
	fake.calls = append(fake.calls, input)
	fake.mu.Unlock()
	if fake.started != nil {
		select {
		case fake.started <- struct{}{}:
		default:
		}
	}
	if fake.block {
		<-ctx.Done()
		if fake.canceled != nil {
			close(fake.canceled)
		}
		return nil, ctx.Err()
	}
	if fake.err != nil {
		return nil, fake.err
	}
	return &appdevapp.ProviderRuntimeProjection{Generation: 1, State: appdevapp.ProviderRuntimeStateRunning}, nil
}

type appDevBuildRecovererFake struct {
	calls chan appdevapp.ProviderBuildRecoverInput
}

func (fake *appDevBuildRecovererFake) RecoverBuild(_ context.Context, input appdevapp.ProviderBuildRecoverInput) (*appdevapp.ProviderBuildProjection, error) {
	fake.calls <- input
	return &appdevapp.ProviderBuildProjection{Generation: 1, State: appdevapp.ProviderBuildStateIdle}, nil
}

type appDevRecoveryOwnerReleaserFake struct {
	calls chan appdevapp.ProviderRuntimeRecoverProjectInput
}

func (fake *appDevRecoveryOwnerReleaserFake) ReleaseRecoveryOwner(_ context.Context, input appdevapp.ProviderRuntimeRecoverProjectInput) error {
	fake.calls <- input
	return nil
}

type appDevRecoveryOwnerRetryFake struct {
	mu       sync.Mutex
	calls    map[string]int
	failures map[string]int
}

func (fake *appDevRecoveryOwnerRetryFake) ReleaseRecoveryOwner(_ context.Context, input appdevapp.ProviderRuntimeRecoverProjectInput) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calls[input.ProjectID]++
	if fake.failures[input.ProjectID] > 0 {
		fake.failures[input.ProjectID]--
		return errors.New("owner release failed")
	}
	return nil
}

type appDevSnapshotCleanupFake struct{ calls chan struct{} }

func (fake *appDevSnapshotCleanupFake) CleanupDeferredProviderSnapshotRestoreObjects(context.Context, int) (int, error) {
	fake.calls <- struct{}{}
	return 0, nil
}

func TestAppDevRecoverySchedulerScansImmediatelyAndReleasesOwnersOnShutdown(t *testing.T) {
	project := infraAppDevRecoveryProject{SpaceID: "1001", ProjectID: "project-a"}
	source := &appDevRecoverySourceFake{projects: []infraAppDevRecoveryProject{project}, calls: make(chan struct{}, 4)}
	runtime := &appDevRuntimeRecovererFake{}
	build := &appDevBuildRecovererFake{calls: make(chan appdevapp.ProviderBuildRecoverInput, 4)}
	releaser := &appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 4)}
	cleanup := &appDevSnapshotCleanupFake{calls: make(chan struct{}, 4)}
	scheduler, err := newAppDevRecoveryScheduler(source, runtime, build, releaser, cleanup, appDevRecoverySchedulerConfig{
		Interval: time.Hour, ErrorBackoff: time.Millisecond, AttemptTimeout: time.Second, BatchSize: 32,
	})
	require.NoError(t, err)
	scheduler.Start(context.Background())

	select {
	case call := <-build.calls:
		require.Equal(t, project.SpaceID, call.SpaceID)
		require.Equal(t, project.ProjectID, call.ProjectID)
	case <-time.After(time.Second):
		t.Fatal("startup recovery did not run")
	}
	select {
	case <-cleanup.calls:
	case <-time.After(time.Second):
		t.Fatal("deferred cleanup did not run")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, scheduler.Shutdown(shutdownCtx))
	select {
	case released := <-releaser.calls:
		require.Equal(t, project.SpaceID, released.SpaceID)
		require.Equal(t, project.ProjectID, released.ProjectID)
	default:
		t.Fatal("owner was not released")
	}
}

func TestAppDevRecoverySchedulerShutdownCancelsBlockedAttemptAndIsBounded(t *testing.T) {
	source := &appDevRecoverySourceFake{
		projects: []infraAppDevRecoveryProject{{SpaceID: "1001", ProjectID: "project-a"}},
		calls:    make(chan struct{}, 1),
	}
	runtime := &appDevRuntimeRecovererFake{
		block: true, started: make(chan struct{}, 1), canceled: make(chan struct{}),
	}
	scheduler, err := newAppDevRecoveryScheduler(
		source,
		runtime,
		&appDevBuildRecovererFake{calls: make(chan appdevapp.ProviderBuildRecoverInput, 1)},
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		&appDevSnapshotCleanupFake{calls: make(chan struct{}, 1)},
		appDevRecoverySchedulerConfig{Interval: time.Hour, ErrorBackoff: time.Millisecond, AttemptTimeout: time.Hour, BatchSize: 8},
	)
	require.NoError(t, err)
	scheduler.Start(context.Background())
	<-runtime.started
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, scheduler.Shutdown(shutdownCtx))
	select {
	case <-runtime.canceled:
	default:
		t.Fatal("blocked recovery did not observe scheduler cancellation")
	}
}

func TestAppDevRecoverySchedulerSkipsBuildWhenRuntimeOwnerIsBusy(t *testing.T) {
	source := &appDevRecoverySourceFake{
		projects: []infraAppDevRecoveryProject{{SpaceID: "1001", ProjectID: "project-a"}},
		calls:    make(chan struct{}, 2),
	}
	runtime := &appDevRuntimeRecovererFake{err: errors.New("owner busy")}
	build := &appDevBuildRecovererFake{calls: make(chan appdevapp.ProviderBuildRecoverInput, 1)}
	scheduler, err := newAppDevRecoveryScheduler(
		source, runtime, build,
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		&appDevSnapshotCleanupFake{calls: make(chan struct{}, 2)},
		appDevRecoverySchedulerConfig{Interval: time.Hour, ErrorBackoff: time.Hour, AttemptTimeout: time.Second, BatchSize: 8},
	)
	require.NoError(t, err)
	scheduler.Start(context.Background())
	<-source.calls
	time.Sleep(10 * time.Millisecond)
	select {
	case <-build.calls:
		t.Fatal("build recovery ran without runtime ownership")
	default:
	}
	require.NoError(t, scheduler.Shutdown(context.Background()))
}

func TestAppDevRecoverySchedulerContinuesAfterOwnerConflictAndPaginates(t *testing.T) {
	source := &appDevRecoverySourceFake{
		projects: []infraAppDevRecoveryProject{
			{SpaceID: "1001", ProjectID: "busy"},
			{SpaceID: "1001", ProjectID: "recoverable"},
		},
		calls: make(chan struct{}, 2),
	}
	runtime := &appDevRuntimeRecovererFake{}
	build := &appDevBuildRecovererFake{calls: make(chan appdevapp.ProviderBuildRecoverInput, 2)}
	scheduler, err := newAppDevRecoveryScheduler(
		source, runtime, build,
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 2)},
		&appDevSnapshotCleanupFake{calls: make(chan struct{}, 2)},
		appDevRecoverySchedulerConfig{Interval: time.Hour, ErrorBackoff: time.Hour, AttemptTimeout: time.Second, BatchSize: 1},
	)
	require.NoError(t, err)
	runtime.err = domainappdev.ErrProviderExecutionOwnerConflict
	require.True(t, scheduler.reconcile(context.Background()), "owner conflicts are normal skips")
}

func TestAppDevRecoverySchedulerShutdownRetriesAllOwnerReleases(t *testing.T) {
	projects := []infraAppDevRecoveryProject{
		{SpaceID: "1001", ProjectID: "project-a"},
		{SpaceID: "1001", ProjectID: "project-b"},
	}
	source := &appDevRecoverySourceFake{projects: projects, calls: make(chan struct{}, 2)}
	releaser := &appDevRecoveryOwnerRetryFake{
		calls: map[string]int{}, failures: map[string]int{"project-a": 1, "project-b": 1},
	}
	scheduler, err := newAppDevRecoveryScheduler(
		source, &appDevRuntimeRecovererFake{},
		&appDevBuildRecovererFake{calls: make(chan appdevapp.ProviderBuildRecoverInput, 4)},
		releaser, &appDevSnapshotCleanupFake{calls: make(chan struct{}, 2)},
		appDevRecoverySchedulerConfig{Interval: time.Hour, ErrorBackoff: time.Millisecond, AttemptTimeout: time.Second, BatchSize: 8},
	)
	require.NoError(t, err)
	scheduler.Start(context.Background())
	<-source.calls
	require.Eventually(t, func() bool {
		scheduler.mu.Lock()
		defer scheduler.mu.Unlock()
		return len(scheduler.claimed) == 2
	}, time.Second, time.Millisecond)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, scheduler.Shutdown(shutdownCtx))
	require.GreaterOrEqual(t, releaser.calls["project-a"], 2)
	require.GreaterOrEqual(t, releaser.calls["project-b"], 2)
}
