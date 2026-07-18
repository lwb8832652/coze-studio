// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"errors"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type ProviderProjectArchiveInput struct {
	SpaceID            string
	ProjectID          string
	OperationID        string
	ExpectedGeneration uint64
}

type ProviderProjectArchiveLifecycle interface {
	CurrentProjectArchiveGeneration(context.Context, string, string) (uint64, error)
	PrepareProjectArchive(context.Context, ProviderProjectArchiveInput) error
}

type ProviderProjectArchiveLifecycleFunc struct {
	Current func(context.Context, string, string) (uint64, error)
	Prepare func(context.Context, ProviderProjectArchiveInput) error
}

func (lifecycle ProviderProjectArchiveLifecycleFunc) CurrentProjectArchiveGeneration(
	ctx context.Context,
	spaceID string,
	projectID string,
) (uint64, error) {
	if lifecycle.Current == nil {
		return 0, ErrProviderControlUnavailable
	}
	return lifecycle.Current(ctx, spaceID, projectID)
}

func (lifecycle ProviderProjectArchiveLifecycleFunc) PrepareProjectArchive(
	ctx context.Context,
	input ProviderProjectArchiveInput,
) error {
	if lifecycle.Prepare == nil {
		return ErrProviderControlUnavailable
	}
	return lifecycle.Prepare(ctx, input)
}

type ProviderProjectArchiveController struct {
	ledger interface {
		LoadCurrent(context.Context, LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error)
	}
	runtime interface {
		Stop(context.Context, ProviderRuntimeStopInput) (*ProviderRuntimeProjection, error)
	}
}

func NewProviderProjectArchiveController(
	ledger interface {
		LoadCurrent(context.Context, LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error)
	},
	runtime interface {
		Stop(context.Context, ProviderRuntimeStopInput) (*ProviderRuntimeProjection, error)
	},
) (*ProviderProjectArchiveController, error) {
	if ledger == nil || runtime == nil {
		return nil, ErrProviderControlInvalid
	}
	return &ProviderProjectArchiveController{ledger: ledger, runtime: runtime}, nil
}

func (controller *ProviderProjectArchiveController) CurrentProjectArchiveGeneration(
	ctx context.Context,
	spaceID string,
	projectID string,
) (uint64, error) {
	if controller == nil || ctx == nil || !validProviderAPIScope(spaceID, projectID) {
		return 0, providerControlInputError(ctx)
	}
	current, err := controller.ledger.LoadCurrent(ctx, LoadCurrentProviderExecutionRequest{
		SpaceID: spaceID, ProjectID: projectID,
	})
	if errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		return 0, nil
	}
	if err != nil || current == nil {
		return 0, normalizeProviderControlError(ctx, err)
	}
	return current.Generation, nil
}

func (controller *ProviderProjectArchiveController) PrepareProjectArchive(
	ctx context.Context,
	input ProviderProjectArchiveInput,
) error {
	if controller == nil || ctx == nil || !validProviderAPIScope(input.SpaceID, input.ProjectID) ||
		!validProviderAPIOperationID(input.OperationID) {
		return providerControlInputError(ctx)
	}
	if input.ExpectedGeneration == 0 {
		generation, err := controller.CurrentProjectArchiveGeneration(ctx, input.SpaceID, input.ProjectID)
		if err != nil {
			return err
		}
		if generation != 0 {
			return ErrProviderControlConflict
		}
		return nil
	}
	expected := input.ExpectedGeneration
	projection, err := controller.runtime.Stop(ctx, ProviderRuntimeStopInput{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: input.OperationID,
		ActorID: "project-archive", ExpectedGeneration: &expected,
	})
	if err != nil {
		return normalizeProviderControlError(ctx, err)
	}
	if projection == nil || !projection.CanStart ||
		(projection.State != ProviderRuntimeStateStopped && projection.State != ProviderRuntimeStateCleanupComplete) {
		return ErrProviderControlConflict
	}
	return nil
}
