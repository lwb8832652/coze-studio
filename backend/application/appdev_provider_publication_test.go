// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
)

func TestAppDevProviderShutdownCannotUnpublishNewerServiceBinding(t *testing.T) {
	service := appdevapp.NewService(nil)
	lifecycle := appdevapp.ProviderProjectArchiveLifecycleFunc{
		Current: func(context.Context, string, string) (uint64, error) { return 0, nil },
		Prepare: func(context.Context, appdevapp.ProviderProjectArchiveInput) error { return nil },
	}
	firstFacade := &appdevapp.ProviderAPIFacade{}
	secondFacade := &appdevapp.ProviderAPIFacade{}
	first, err := service.PublishProviderRuntimeBindings(firstFacade, lifecycle)
	require.NoError(t, err)
	second, err := service.PublishProviderRuntimeBindings(secondFacade, lifecycle)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.UnpublishProviderRuntimeBindings(second) })

	runtime := &appDevProviderRuntime{service: service, serviceOwner: first}
	err = runtime.Shutdown(context.Background())
	require.NoError(t, err)
	current, currentErr := service.ProviderAPI()
	require.NoError(t, currentErr)
	require.Same(t, secondFacade, current)
}

func TestAppDevProviderShutdownDoesNotUnpublishBeforeSchedulerStopsAndRetries(t *testing.T) {
	service := appdevapp.NewService(nil)
	lifecycle := appdevapp.ProviderProjectArchiveLifecycleFunc{
		Current: func(context.Context, string, string) (uint64, error) { return 0, nil },
		Prepare: func(context.Context, appdevapp.ProviderProjectArchiveInput) error { return nil },
	}
	facade := &appdevapp.ProviderAPIFacade{}
	publication, err := service.PublishProviderRuntimeBindings(facade, lifecycle)
	require.NoError(t, err)
	runtimeRecoverer := &stubbornRuntimeRecoverer{
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	scheduler, err := newAppDevRecoveryScheduler(
		&cursorAwareRecoverySource{projects: []infraAppDevRecoveryProject{{
			SpaceID: "1001", ProjectID: "shutdown-retry",
		}}},
		runtimeRecoverer,
		&recordingBuildRecoverer{},
		&appDevRecoveryOwnerReleaserFake{calls: make(chan appdevapp.ProviderRuntimeRecoverProjectInput, 1)},
		noOpSnapshotCleanup{},
		appDevRecoverySchedulerConfig{
			Interval: time.Hour, ErrorBackoff: time.Millisecond,
			AttemptTimeout: time.Second, ProjectTimeout: 100 * time.Millisecond,
			ClaimedLeaseDuration: time.Minute, BatchSize: 1,
		},
	)
	require.NoError(t, err)
	runtime := &appDevProviderRuntime{
		scheduler: scheduler, service: service, serviceOwner: publication,
	}
	scheduler.Start(context.Background())
	<-runtimeRecoverer.entered

	expired, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, runtime.Shutdown(expired), context.Canceled)
	current, currentErr := service.ProviderAPI()
	require.NoError(t, currentErr)
	require.Same(t, facade, current, "publication must remain while scheduler still owns recovery work")

	close(runtimeRecoverer.release)
	require.NoError(t, runtime.Shutdown(context.Background()))
	_, currentErr = service.ProviderAPI()
	require.ErrorIs(t, currentErr, appdevapp.ErrProviderControlUnavailable)
}
