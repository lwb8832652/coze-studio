// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type providerArchiveStoreFake struct {
	domainappdev.Store
	project       *domainappdev.Project
	archiveCalls  int
	intent        *domainappdev.ProjectArchiveIntent
	completeCalls int
}

func (store *providerArchiveStoreFake) GetProject(context.Context, string, string) (*domainappdev.Project, error) {
	if store.project == nil {
		return nil, domainappdev.ErrNotFound
	}
	if store.project.SourceVersion <= 0 {
		store.project.SourceVersion = 1
	}
	copy := *store.project
	return &copy, nil
}

func (store *providerArchiveStoreFake) ArchiveProject(context.Context, string, string) (*domainappdev.Project, error) {
	store.archiveCalls++
	copy := *store.project
	copy.Status = domainappdev.ProjectStatusArchived
	return &copy, nil
}

func (store *providerArchiveStoreFake) LoadProjectArchive(
	context.Context,
	domainappdev.LoadProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	if store.intent == nil {
		return nil, domainappdev.ErrProjectArchiveNotFound
	}
	copy := *store.intent
	return &copy, nil
}

func (store *providerArchiveStoreFake) ReserveProjectArchive(
	_ context.Context,
	input domainappdev.ReserveProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	if store.intent == nil {
		identity, err := domainappdev.NewProjectArchiveIntentIdentity(
			input.SpaceID,
			input.ProjectID,
			input.ExpectedSourceVersion,
			input.RuntimeGeneration,
			1,
		)
		if err != nil {
			return nil, err
		}
		store.intent = &domainappdev.ProjectArchiveIntent{
			ProjectArchiveIntentIdentity: identity,
			State:                        domainappdev.ProjectArchiveStateArchiving,
		}
	}
	copy := *store.intent
	return &copy, nil
}

func (store *providerArchiveStoreFake) CompleteProjectArchive(
	_ context.Context,
	input domainappdev.CompleteProjectArchiveInput,
) (*domainappdev.ProjectArchiveIntent, error) {
	store.completeCalls++
	if store.intent == nil || input.Intent.OperationHash != store.intent.OperationHash {
		return nil, domainappdev.ErrProjectArchiveConflict
	}
	sourceVersion := store.project.SourceVersion
	if sourceVersion <= 0 {
		sourceVersion = 1
	}
	if input.Intent.SourceVersion != sourceVersion {
		return nil, domainappdev.ErrProjectArchiveConflict
	}
	store.intent.State = domainappdev.ProjectArchiveStateArchived
	store.archiveCalls++
	store.project.Status = domainappdev.ProjectStatusArchived
	copy := *store.intent
	return &copy, nil
}

type providerArchiveLifecycleFake struct {
	generation uint64
	err        error
	currentErr error
	prepareErr error
	inputs     []ProviderProjectArchiveInput
}

func (fake *providerArchiveLifecycleFake) CurrentProjectArchiveGeneration(context.Context, string, string) (uint64, error) {
	if fake.currentErr != nil {
		return 0, fake.currentErr
	}
	return fake.generation, fake.err
}

func (fake *providerArchiveLifecycleFake) PrepareProjectArchive(_ context.Context, input ProviderProjectArchiveInput) error {
	fake.inputs = append(fake.inputs, input)
	if fake.prepareErr != nil {
		return fake.prepareErr
	}
	return fake.err
}

func TestArchiveProjectRequiresProviderLifecycleAndStableGenerationBoundOperation(t *testing.T) {
	sourceTime := time.Date(2026, 7, 17, 1, 2, 3, 0, time.UTC)
	store := &providerArchiveStoreFake{project: &domainappdev.Project{
		ID: "project-a", SpaceID: "1001", Status: domainappdev.ProjectStatusReady, SourceUpdatedAt: sourceTime,
	}}
	service := NewService(store)
	require.NoError(t, service.SetProjectRuntimeLifecycleMode(ProjectRuntimeLifecycleModeProviderRequired))
	_, err := service.ArchiveProject(context.Background(), &ArchiveProjectRequest{
		SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7,
	})
	require.ErrorIs(t, err, ErrProviderControlUnavailable)
	require.Zero(t, store.archiveCalls)

	lifecycle := &providerArchiveLifecycleFake{generation: 9}
	publication, publishErr := service.PublishProviderRuntimeBindings(&ProviderAPIFacade{}, lifecycle)
	require.NoError(t, publishErr)
	defer service.UnpublishProviderRuntimeBindings(publication)
	_, err = service.ArchiveProject(context.Background(), &ArchiveProjectRequest{
		SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7,
	})
	require.NoError(t, err)
	require.Equal(t, 1, store.archiveCalls)
	require.Len(t, lifecycle.inputs, 1)
	require.Equal(t, uint64(9), lifecycle.inputs[0].ExpectedGeneration)
	require.NotEmpty(t, lifecycle.inputs[0].OperationID)

	store.archiveCalls = 0
	lifecycle.err = errors.New("provider unavailable")
	_, err = service.ArchiveProject(context.Background(), &ArchiveProjectRequest{
		SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7,
	})
	require.NoError(t, err, "completed durable intent must be idempotent")
	require.Zero(t, store.archiveCalls)
}

func TestArchiveProjectPersistsIntentAcrossCleanupFailureAndRetry(t *testing.T) {
	store := &providerArchiveStoreFake{project: &domainappdev.Project{
		ID: "project-a", SpaceID: "1001", Status: domainappdev.ProjectStatusReady,
		SourceVersion: 4,
	}}
	lifecycle := &providerArchiveLifecycleFake{generation: 9, prepareErr: errors.New("cleanup unavailable")}
	service := NewService(store)
	require.NoError(t, service.SetProjectRuntimeLifecycleMode(ProjectRuntimeLifecycleModeProviderRequired))
	publication, publishErr := service.PublishProviderRuntimeBindings(&ProviderAPIFacade{}, lifecycle)
	require.NoError(t, publishErr)
	defer service.UnpublishProviderRuntimeBindings(publication)

	request := &ArchiveProjectRequest{SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7}
	_, err := service.ArchiveProject(context.Background(), request)
	require.Error(t, err)
	require.NotNil(t, store.intent)
	require.Equal(t, domainappdev.ProjectArchiveStateArchiving, store.intent.State)
	require.Zero(t, store.completeCalls)
	require.Zero(t, store.archiveCalls)
	require.Len(t, lifecycle.inputs, 1)
	firstOperation := lifecycle.inputs[0].OperationID

	lifecycle.prepareErr = nil
	_, err = service.ArchiveProject(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, lifecycle.inputs, 2)
	require.Equal(t, firstOperation, lifecycle.inputs[1].OperationID)
	require.Equal(t, 1, store.completeCalls)
	require.Equal(t, 1, store.archiveCalls)
	require.Equal(t, domainappdev.ProjectArchiveStateArchived, store.intent.State)
}

func TestArchiveProjectRejectsSourceMutationAcrossProviderBarrier(t *testing.T) {
	store := &providerArchiveStoreFake{project: &domainappdev.Project{
		ID: "project-a", SpaceID: "1001", Status: domainappdev.ProjectStatusReady,
		SourceUpdatedAt: time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC),
	}}
	lifecycle := &providerArchiveLifecycleFake{generation: 3}
	service := NewService(store)
	require.NoError(t, service.SetProjectRuntimeLifecycleMode(ProjectRuntimeLifecycleModeProviderRequired))
	publication, publishErr := service.PublishProviderRuntimeBindings(&ProviderAPIFacade{}, ProviderProjectArchiveLifecycleFunc{
		Current: lifecycle.CurrentProjectArchiveGeneration,
		Prepare: func(ctx context.Context, input ProviderProjectArchiveInput) error {
			store.project.SourceUpdatedAt = store.project.SourceUpdatedAt.Add(time.Second)
			store.project.SourceVersion++
			return lifecycle.PrepareProjectArchive(ctx, input)
		},
	})
	require.NoError(t, publishErr)
	defer service.UnpublishProviderRuntimeBindings(publication)
	_, err := service.ArchiveProject(context.Background(), &ArchiveProjectRequest{
		SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7,
	})
	require.ErrorIs(t, err, ErrProviderControlConflict)
	require.Zero(t, store.archiveCalls)
}

type providerArchiveLegacyRuntimeFake struct {
	stopCalls int
	stopErr   error
}

func (*providerArchiveLegacyRuntimeFake) Start(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return nil, nil
}

func (*providerArchiveLegacyRuntimeFake) Status(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return nil, nil
}

func (*providerArchiveLegacyRuntimeFake) KeepAlive(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return nil, nil
}

func (*providerArchiveLegacyRuntimeFake) Restart(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return nil, nil
}

func (fake *providerArchiveLegacyRuntimeFake) Stop(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	fake.stopCalls++
	return nil, fake.stopErr
}

func (*providerArchiveLegacyRuntimeFake) Logs(context.Context, *RuntimeManagerRequest) ([]*domainappdev.RuntimeLog, error) {
	return nil, nil
}

func TestArchiveProjectUsesOnlyExplicitRuntimeLifecycleMode(t *testing.T) {
	t.Run("unconfigured zero value fails closed", func(t *testing.T) {
		store := &providerArchiveStoreFake{project: &domainappdev.Project{
			ID: "project-a", SpaceID: "1001", Status: domainappdev.ProjectStatusReady,
		}}
		service := NewService(store)

		_, err := service.ArchiveProject(context.Background(), &ArchiveProjectRequest{
			SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7,
		})

		require.ErrorIs(t, err, ErrProviderControlUnavailable)
		require.Zero(t, store.archiveCalls)
	})

	t.Run("provider required never falls back to legacy", func(t *testing.T) {
		store := &providerArchiveStoreFake{project: &domainappdev.Project{
			ID: "project-a", SpaceID: "1001", Status: domainappdev.ProjectStatusReady,
		}}
		legacy := &providerArchiveLegacyRuntimeFake{}
		service := NewService(store, legacy)
		require.NoError(t, service.SetProjectRuntimeLifecycleMode(ProjectRuntimeLifecycleModeProviderRequired))

		_, err := service.ArchiveProject(context.Background(), &ArchiveProjectRequest{
			SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7,
		})

		require.ErrorIs(t, err, ErrProviderControlUnavailable)
		require.Zero(t, legacy.stopCalls)
		require.Zero(t, store.archiveCalls)
	})

	t.Run("legacy runtime requires explicit debug mode", func(t *testing.T) {
		store := &providerArchiveStoreFake{project: &domainappdev.Project{
			ID: "project-a", SpaceID: "1001", Status: domainappdev.ProjectStatusReady,
		}}
		legacy := &providerArchiveLegacyRuntimeFake{}
		service := NewService(store, legacy)
		require.NoError(t, service.SetProjectRuntimeLifecycleMode(ProjectRuntimeLifecycleModeLegacyDebug))

		_, err := service.ArchiveProject(context.Background(), &ArchiveProjectRequest{
			SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7,
		})

		require.NoError(t, err)
		require.Equal(t, 1, legacy.stopCalls)
		require.Equal(t, 1, store.archiveCalls)
	})

	t.Run("explicit non provider mode archives without runtime", func(t *testing.T) {
		store := &providerArchiveStoreFake{project: &domainappdev.Project{
			ID: "project-a", SpaceID: "1001", Status: domainappdev.ProjectStatusReady,
		}}
		service := NewService(store)
		require.NoError(t, service.SetProjectRuntimeLifecycleMode(ProjectRuntimeLifecycleModeNonProvider))

		_, err := service.ArchiveProject(context.Background(), &ArchiveProjectRequest{
			SpaceID: "1001", ProjectID: "project-a", CurrentUserID: 7,
		})

		require.NoError(t, err)
		require.Equal(t, 1, store.archiveCalls)
	})
}

func TestProviderServicePublicationOldOwnerCannotClearNewRuntime(t *testing.T) {
	service := NewService(&providerArchiveStoreFake{})
	firstLifecycle := &providerArchiveLifecycleFake{}
	secondLifecycle := &providerArchiveLifecycleFake{}
	firstFacade := &ProviderAPIFacade{}
	secondFacade := &ProviderAPIFacade{}

	firstOwner, err := service.PublishProviderRuntimeBindings(firstFacade, firstLifecycle)
	require.NoError(t, err)
	secondOwner, err := service.PublishProviderRuntimeBindings(secondFacade, secondLifecycle)
	require.NoError(t, err)

	require.False(t, service.UnpublishProviderRuntimeBindings(firstOwner))
	current, err := service.ProviderAPI()
	require.NoError(t, err)
	require.Same(t, secondFacade, current)
	lifecycle, _ := service.providerArchiveConfiguration()
	require.Same(t, secondLifecycle, lifecycle)
	require.True(t, service.UnpublishProviderRuntimeBindings(secondOwner))
	_, err = service.ProviderAPI()
	require.ErrorIs(t, err, ErrProviderControlUnavailable)
}

func TestProviderServiceExposesNoOwnerlessArchiveLifecycleMutation(t *testing.T) {
	serviceType := reflect.TypeOf(&Service{})
	for _, method := range []string{"SetProviderArchiveLifecycle", "ClearProviderArchiveLifecycle"} {
		_, exists := serviceType.MethodByName(method)
		require.False(t, exists, "ownerless provider binding mutation %s must not be exported", method)
	}
}
