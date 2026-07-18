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
	infraappdev "github.com/coze-dev/coze-studio/backend/infra/appdev"
)

type cursorAwareRecoverySource struct {
	mu       sync.Mutex
	projects []infraAppDevRecoveryProject
	cursors  []*infraAppDevRecoveryCursor
}

func (source *cursorAwareRecoverySource) ListProviderExecutionRecoveryProjects(
	_ context.Context,
	cursor *infraAppDevRecoveryCursor,
	limit int,
) ([]infraAppDevRecoveryProject, *infraAppDevRecoveryCursor, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.cursors = append(source.cursors, cloneRecoveryCursor(cursor))
	start := 0
	if cursor != nil {
		for index, project := range source.projects {
			if project.SpaceID == cursor.SpaceID && project.ProjectID == cursor.ProjectID {
				start = index + 1
				break
			}
		}
	}
	end := start + limit
	if end > len(source.projects) {
		end = len(source.projects)
	}
	items := append([]infraAppDevRecoveryProject(nil), source.projects[start:end]...)
	if end == len(source.projects) || len(items) == 0 {
		return items, nil, nil
	}
	last := items[len(items)-1]
	return items, &infraAppDevRecoveryCursor{SpaceID: last.SpaceID, ProjectID: last.ProjectID}, nil
}

func cloneRecoveryCursor(cursor *infraAppDevRecoveryCursor) *infraAppDevRecoveryCursor {
	if cursor == nil {
		return nil
	}
	copy := *cursor
	return &copy
}

type recordingRuntimeRecoverer struct {
	mu         sync.Mutex
	projects   []string
	projection appdevapp.ProviderRuntimeProjection
	errByID    map[string]error
	onRecover  func()
}

func (recoverer *recordingRuntimeRecoverer) RecoverProjectResumeOnly(
	_ context.Context,
	input appdevapp.ProviderRuntimeRecoverProjectInput,
) (*appdevapp.ProviderRuntimeProjection, error) {
	recoverer.mu.Lock()
	recoverer.projects = append(recoverer.projects, input.ProjectID)
	if err := recoverer.errByID[input.ProjectID]; err != nil {
		recoverer.mu.Unlock()
		return nil, err
	}
	projection := recoverer.projection
	if projection.State == "" {
		projection = appdevapp.ProviderRuntimeProjection{
			Generation: 1,
			State:      appdevapp.ProviderRuntimeStateRunning,
		}
	}
	onRecover := recoverer.onRecover
	recoverer.mu.Unlock()
	if onRecover != nil {
		onRecover()
	}
	return &projection, nil
}

type recordingBuildRecoverer struct {
	mu       sync.Mutex
	projects []string
}

func (recoverer *recordingBuildRecoverer) RecoverBuild(
	_ context.Context,
	input appdevapp.ProviderBuildRecoverInput,
) (*appdevapp.ProviderBuildProjection, error) {
	recoverer.mu.Lock()
	recoverer.projects = append(recoverer.projects, input.ProjectID)
	recoverer.mu.Unlock()
	return &appdevapp.ProviderBuildProjection{
		Generation: 1,
		State:      appdevapp.ProviderBuildStateIdle,
	}, nil
}

type noOpSnapshotCleanup struct{}

func (noOpSnapshotCleanup) CleanupDeferredProviderSnapshotRestoreObjects(context.Context, int) (int, error) {
	return 0, nil
}

type ownerAwareRecoverySource struct {
	mu          sync.Mutex
	project     infraAppDevRecoveryProject
	ownerActive bool
	calls       int
}

func (source *ownerAwareRecoverySource) ListProviderExecutionRecoveryProjects(
	_ context.Context,
	_ *infraAppDevRecoveryCursor,
	_ int,
) ([]infraAppDevRecoveryProject, *infraAppDevRecoveryCursor, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.calls++
	if source.ownerActive {
		return nil, nil, nil
	}
	return []infraAppDevRecoveryProject{source.project}, nil, nil
}

func (source *ownerAwareRecoverySource) markOwnerActive() {
	source.mu.Lock()
	source.ownerActive = true
	source.mu.Unlock()
}

func TestAppDevRecoverySchedulerBuildConsumesRuntimeClaimWithoutOwnerEligibleRequery(t *testing.T) {
	source := &ownerAwareRecoverySource{project: infraAppDevRecoveryProject{
		SpaceID: "1001", ProjectID: "owner-aware-project",
	}}
	runtime := &recordingRuntimeRecoverer{onRecover: source.markOwnerActive}
	build := &recordingBuildRecoverer{}
	scheduler, err := newAppDevRecoveryScheduler(
		source,
		runtime,
		build,
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		noOpSnapshotCleanup{},
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Millisecond,
			AttemptTimeout: time.Second, ProjectTimeout: 100 * time.Millisecond,
			ClaimedLeaseDuration: time.Minute, BatchSize: 1,
		},
	)
	require.NoError(t, err)

	require.True(t, scheduler.reconcile(context.Background()))
	require.Equal(t, []string{"owner-aware-project"}, runtime.projects)
	require.Equal(t, []string{"owner-aware-project"}, build.projects)
	require.Equal(t, 1, source.calls, "build must consume the runtime claim snapshot instead of querying an owner-filtered pager")
}

type blockingSnapshotCleanup struct {
	called chan struct{}
}

func (cleanup *blockingSnapshotCleanup) CleanupDeferredProviderSnapshotRestoreObjects(ctx context.Context, _ int) (int, error) {
	close(cleanup.called)
	<-ctx.Done()
	return 0, ctx.Err()
}

type recordingSnapshotCleanupPager struct {
	mu      sync.Mutex
	cursors []*infraappdev.ProviderSnapshotCleanupCursor
	pages   []*infraappdev.ProviderSnapshotCleanupCursor
}

func (pager *recordingSnapshotCleanupPager) CleanupDeferredProviderSnapshotRestoreObjects(
	context.Context,
	int,
) (int, error) {
	return 0, errors.New("non-paged cleanup path must not be used")
}

func (pager *recordingSnapshotCleanupPager) CleanupDeferredProviderSnapshotRestoreObjectsPage(
	_ context.Context,
	cursor *infraappdev.ProviderSnapshotCleanupCursor,
	_ int,
) (int, *infraappdev.ProviderSnapshotCleanupCursor, error) {
	pager.mu.Lock()
	defer pager.mu.Unlock()
	pager.cursors = append(pager.cursors, cloneAppDevSnapshotCleanupCursor(cursor))
	index := len(pager.cursors) - 1
	if index >= len(pager.pages) {
		return 0, nil, nil
	}
	return 1, cloneAppDevSnapshotCleanupCursor(pager.pages[index]), nil
}

func TestAppDevRecoverySchedulerFakeClockFencesClaimsAndShutdownRelease(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	project := infraAppDevRecoveryProject{SpaceID: "1001", ProjectID: "clocked"}
	releaser := &appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 2)}
	scheduler, err := newAppDevRecoveryScheduler(
		&cursorAwareRecoverySource{},
		&recordingRuntimeRecoverer{},
		&recordingBuildRecoverer{},
		releaser,
		noOpSnapshotCleanup{},
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Millisecond,
			AttemptTimeout: time.Second, ProjectTimeout: 100 * time.Millisecond,
			ClaimedLeaseDuration: time.Minute, BatchSize: 1,
			Now: func() time.Time { return now },
		},
	)
	require.NoError(t, err)

	scheduler.commitRuntimeRecovery(project, &appdevapp.ProviderRuntimeProjection{
		Generation: 1, State: appdevapp.ProviderRuntimeStateRunning,
	}, nil)
	require.True(t, scheduler.hasCurrentRecoveryClaim(project))
	require.Equal(t, now.Add(time.Minute), scheduler.claimUntil["1001\x00clocked"])

	now = now.Add(time.Minute + time.Microsecond)
	require.False(t, scheduler.hasCurrentRecoveryClaim(project))
	require.Empty(t, scheduler.claimed)

	now = now.Add(time.Second)
	scheduler.commitRuntimeRecovery(project, &appdevapp.ProviderRuntimeProjection{
		Generation: 1, State: appdevapp.ProviderRuntimeStateRunning,
	}, nil)
	require.NoError(t, scheduler.Shutdown(context.Background()))
	select {
	case released := <-releaser.calls:
		require.Equal(t, project.SpaceID, released.SpaceID)
		require.Equal(t, project.ProjectID, released.ProjectID)
	default:
		t.Fatal("shutdown did not release the fake-clock owner claim")
	}
}

func TestAppDevRecoverySchedulerRetainsClaimWhenResumeOnlyRetainsOwner(t *testing.T) {
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	project := infraAppDevRecoveryProject{SpaceID: "1001", ProjectID: "retained-owner"}
	scheduler, err := newAppDevRecoveryScheduler(
		&cursorAwareRecoverySource{},
		&recordingRuntimeRecoverer{},
		&recordingBuildRecoverer{},
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		noOpSnapshotCleanup{},
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Millisecond,
			AttemptTimeout: time.Second, ProjectTimeout: 100 * time.Millisecond,
			ClaimedLeaseDuration: time.Minute, BatchSize: 1,
			Now: func() time.Time { return now },
		},
	)
	require.NoError(t, err)
	key := project.SpaceID + "\x00" + project.ProjectID
	scheduler.claimed[key] = project
	scheduler.claimUntil[key] = now.Add(time.Minute)

	projection := &appdevapp.ProviderRuntimeProjection{
		Generation: 9, State: appdevapp.ProviderRuntimeStateStarting,
		Recovering: true, CanStart: false,
	}
	scheduler.commitRuntimeRecovery(
		project,
		projection,
		appdevapp.MarkProviderRuntimeRecoveryOwnerRetained(appdevapp.ErrProviderRuntimeUnavailable),
	)

	require.True(t, scheduler.hasCurrentRecoveryClaim(project))
	require.Equal(t, project, scheduler.claimed[key])
}

func TestAppDevRecoverySchedulerCleanupPagerKeepsIndependentCursor(t *testing.T) {
	first := &infraappdev.ProviderSnapshotCleanupCursor{
		DueAt: time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC), ObjectKey: "cleanup-a",
	}
	pager := &recordingSnapshotCleanupPager{pages: []*infraappdev.ProviderSnapshotCleanupCursor{first, nil}}
	scheduler, err := newAppDevRecoveryScheduler(
		&cursorAwareRecoverySource{},
		&recordingRuntimeRecoverer{},
		&recordingBuildRecoverer{},
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		pager,
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Second,
			AttemptTimeout: time.Second, ProjectTimeout: 100 * time.Millisecond, BatchSize: 1,
		},
	)
	require.NoError(t, err)
	scheduler.runtimeCursor = &infraAppDevRecoveryCursor{SpaceID: "1001", ProjectID: "runtime"}
	scheduler.buildCursor = &infraAppDevRecoveryCursor{SpaceID: "1001", ProjectID: "build"}

	require.True(t, scheduler.reconcileCleanup(context.Background()))
	require.Equal(t, first, scheduler.cleanupCursor)
	require.Equal(t, "runtime", scheduler.runtimeCursor.ProjectID)
	require.Equal(t, "build", scheduler.buildCursor.ProjectID)

	require.True(t, scheduler.reconcileCleanup(context.Background()))
	require.Nil(t, scheduler.cleanupCursor)
	require.Len(t, pager.cursors, 2)
	require.Nil(t, pager.cursors[0])
	require.Equal(t, first, pager.cursors[1])
}

func TestAppDevRecoverySchedulerPersistsIndependentPhaseCursors(t *testing.T) {
	projects := make([]infraAppDevRecoveryProject, 0, 5)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		projects = append(projects, infraAppDevRecoveryProject{SpaceID: "1001", ProjectID: id})
	}
	source := &cursorAwareRecoverySource{projects: projects}
	runtime := &recordingRuntimeRecoverer{}
	build := &recordingBuildRecoverer{}
	scheduler, err := newAppDevRecoveryScheduler(
		source,
		runtime,
		build,
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 8)},
		noOpSnapshotCleanup{},
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Second,
			AttemptTimeout: time.Second, BatchSize: 2,
		},
	)
	require.NoError(t, err)

	require.True(t, scheduler.reconcile(context.Background()))
	require.Equal(t, []string{"a", "b"}, runtime.projects)
	require.Equal(t, []string{"a", "b"}, build.projects)

	require.True(t, scheduler.reconcile(context.Background()))
	require.Equal(t, []string{"a", "b", "c", "d"}, runtime.projects)
	require.Equal(t, []string{"a", "b", "c", "d"}, build.projects)

	require.True(t, scheduler.reconcile(context.Background()))
	require.Equal(t, []string{"a", "b", "c", "d", "e"}, runtime.projects)
	require.Equal(t, []string{"a", "b", "c", "d", "e"}, build.projects)
	require.Len(t, source.cursors, 3)
	require.Nil(t, source.cursors[0])
	require.Equal(t, "b", source.cursors[1].ProjectID)
	require.Equal(t, "d", source.cursors[2].ProjectID)
}

func TestAppDevRecoverySchedulerCleanupDeadlineDoesNotCancelRuntimeOrBuild(t *testing.T) {
	source := &cursorAwareRecoverySource{projects: []infraAppDevRecoveryProject{{
		SpaceID: "1001", ProjectID: "fair",
	}}}
	runtime := &recordingRuntimeRecoverer{}
	build := &recordingBuildRecoverer{}
	cleanup := &blockingSnapshotCleanup{called: make(chan struct{})}
	scheduler, err := newAppDevRecoveryScheduler(
		source,
		runtime,
		build,
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		cleanup,
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Second,
			AttemptTimeout: 200 * time.Millisecond, ProjectTimeout: 20 * time.Millisecond,
			BatchSize: 1,
		},
	)
	require.NoError(t, err)

	require.False(t, scheduler.reconcile(context.Background()), "cleanup timeout is reported without starving later phases")
	require.Equal(t, []string{"fair"}, runtime.projects)
	require.Equal(t, []string{"fair"}, build.projects)
}

func TestAppDevRecoverySchedulerRejectsProjectTimeoutEqualToAttemptTimeout(t *testing.T) {
	_, err := newAppDevRecoveryScheduler(
		&cursorAwareRecoverySource{},
		&recordingRuntimeRecoverer{},
		&recordingBuildRecoverer{},
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		noOpSnapshotCleanup{},
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Second,
			AttemptTimeout: time.Second, ProjectTimeout: time.Second, BatchSize: 1,
		},
	)
	require.Error(t, err)
}

type stubbornRuntimeRecoverer struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (recoverer *stubbornRuntimeRecoverer) RecoverProjectResumeOnly(
	ctx context.Context,
	_ appdevapp.ProviderRuntimeRecoverProjectInput,
) (*appdevapp.ProviderRuntimeProjection, error) {
	recoverer.once.Do(func() { close(recoverer.entered) })
	<-recoverer.release
	return nil, ctx.Err()
}

func TestAppDevRecoverySchedulerShutdownTimeoutRetainsSingleRunAttempt(t *testing.T) {
	source := &cursorAwareRecoverySource{projects: []infraAppDevRecoveryProject{{
		SpaceID: "1001", ProjectID: "blocked",
	}}}
	runtime := &stubbornRuntimeRecoverer{entered: make(chan struct{}), release: make(chan struct{})}
	scheduler, err := newAppDevRecoveryScheduler(
		source,
		runtime,
		&recordingBuildRecoverer{},
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		noOpSnapshotCleanup{},
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Second,
			AttemptTimeout: time.Second, BatchSize: 1,
		},
	)
	require.NoError(t, err)
	scheduler.Start(context.Background())
	<-runtime.entered

	expired, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, scheduler.Shutdown(expired), context.Canceled)

	scheduler.mu.Lock()
	firstDone := scheduler.done
	firstCancel := scheduler.cancel
	scheduler.mu.Unlock()
	require.NotNil(t, firstDone)
	require.NotNil(t, firstCancel)

	retryDone := make(chan error, 1)
	go func() {
		retryDone <- scheduler.Shutdown(context.Background())
	}()
	scheduler.Start(context.Background())
	scheduler.mu.Lock()
	require.Equal(t, firstDone, scheduler.done)
	require.NotNil(t, scheduler.cancel)
	scheduler.mu.Unlock()

	close(runtime.release)
	require.NoError(t, <-retryDone)
	scheduler.mu.Lock()
	require.Nil(t, scheduler.done)
	require.Nil(t, scheduler.cancel)
	scheduler.mu.Unlock()
}

func TestAppDevRecoverySchedulerDropsTerminalAndConflictClaims(t *testing.T) {
	project := infraAppDevRecoveryProject{SpaceID: "1001", ProjectID: "terminal"}
	source := &cursorAwareRecoverySource{projects: []infraAppDevRecoveryProject{project}}
	runtime := &recordingRuntimeRecoverer{projection: appdevapp.ProviderRuntimeProjection{
		Generation: 1, State: appdevapp.ProviderRuntimeStateStopped, CanStart: true,
	}}
	scheduler, err := newAppDevRecoveryScheduler(
		source, runtime, &recordingBuildRecoverer{},
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		noOpSnapshotCleanup{},
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Second,
			AttemptTimeout: time.Second, BatchSize: 1,
		},
	)
	require.NoError(t, err)
	scheduler.claimed["1001\x00terminal"] = project
	require.True(t, scheduler.reconcile(context.Background()))
	require.Empty(t, scheduler.claimed)

	runtime.errByID = map[string]error{"terminal": domainappdev.ErrProviderExecutionOwnerConflict}
	scheduler.claimed["1001\x00terminal"] = project
	require.True(t, scheduler.reconcile(context.Background()))
	require.Empty(t, scheduler.claimed)
	require.True(t, errors.Is(runtime.errByID["terminal"], domainappdev.ErrProviderExecutionOwnerConflict))
}
