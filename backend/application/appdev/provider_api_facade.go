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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	providerAPIMinOperationIDLength = 8
	providerAPIMaxOperationIDLength = 128
	providerAPIMaxSafeMessageLength = 256
)

var (
	ErrProviderControlInvalid     = errors.New("appdev provider control input is invalid")
	ErrProviderControlNotFound    = errors.New("appdev provider control resource is not found")
	ErrProviderControlUnavailable = errors.New("appdev provider control is unavailable")
	ErrProviderControlConflict    = errors.New("appdev provider control conflicts with current state")
)

// ProviderRuntimeControl is the only runtime authority exposed to the provider
// API facade. The legacy RuntimeManager is intentionally not part of it.
type ProviderRuntimeControl interface {
	Start(context.Context, ProviderRuntimeStartInput) (*ProviderRuntimeProjection, error)
	Status(context.Context, ProviderRuntimeStatusInput) (*ProviderRuntimeProjection, error)
	Stop(context.Context, ProviderRuntimeStopInput) (*ProviderRuntimeProjection, error)
}

type ProviderRuntimeStartOperationMatcher interface {
	MatchCurrentStartOperation(context.Context, ProviderRuntimeStartInput) (*ProviderRuntimeProjection, bool, error)
}

type ProviderBuildControl interface {
	BeginBuild(context.Context, ProviderBuildBeginInput) (*ProviderBuildProjection, error)
	PollBuild(context.Context, ProviderBuildPollInput) (*ProviderBuildProjection, error)
	RecoverBuild(context.Context, ProviderBuildRecoverInput) (*ProviderBuildProjection, error)
}

type ProviderReleaseOpener interface {
	OpenRelease(context.Context, ProviderReleaseRequest) (*ProviderRelease, error)
}

type ProviderSnapshotRestoreStore interface {
	domainappdev.ProviderSnapshotRestoreRepository
}

type ProviderBuildProjectSource interface {
	GetProject(context.Context, string, string) (*domainappdev.Project, error)
}

type ProviderAPIFacade struct {
	runtime     ProviderRuntimeControl
	build       ProviderBuildControl
	releases    ProviderReleaseOpener
	snapshots   ProviderSnapshotRestoreStore
	buildSource ProviderBuildProjectSource
}

func NewProviderAPIFacade(runtime ProviderRuntimeControl, build ProviderBuildControl, releases ProviderReleaseOpener, snapshots ProviderSnapshotRestoreStore) *ProviderAPIFacade {
	facade := &ProviderAPIFacade{runtime: runtime, build: build, releases: releases, snapshots: snapshots}
	if source, ok := snapshots.(ProviderBuildProjectSource); ok {
		facade.buildSource = source
	}
	return facade
}

func (*ProviderAPIFacade) String() string   { return "ProviderAPIFacade{controls:<redacted>}" }
func (*ProviderAPIFacade) GoString() string { return "ProviderAPIFacade{controls:<redacted>}" }
func (*ProviderAPIFacade) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderAPIFacade{controls:<redacted>}")
}
func (*ProviderAPIFacade) MarshalJSON() ([]byte, error) { return nil, ErrProviderControlUnavailable }

type ProviderRuntimeMutationRequest struct {
	SpaceID     string
	ProjectID   string
	ActorUserID int64
	OperationID string
	ActorID     string
}

type ProviderRuntimeStatusRequest struct {
	SpaceID   string
	ProjectID string
	ActorID   string
}

type ProviderBuildOperationRequest struct {
	SpaceID     string
	ProjectID   string
	OperationID string
}

type ProviderBuildRecoverRequest struct {
	SpaceID   string
	ProjectID string
}

type ProviderSnapshotRestoreRequest struct {
	SpaceID     string
	ProjectID   string
	ActorUserID int64
	SnapshotID  string
	OperationID string
	ActorID     string
}

type ProviderRuntimeAPIProjection struct {
	Generation      uint64               `json:"generation"`
	State           ProviderRuntimeState `json:"state"`
	CanStart        bool                 `json:"canStart"`
	Recovering      bool                 `json:"recovering"`
	Stopping        bool                 `json:"stopping"`
	RelativePreview string               `json:"relativePreview,omitempty"`
	SafeMessage     string               `json:"safeMessage,omitempty"`
}

func (projection ProviderRuntimeAPIProjection) String() string {
	return fmt.Sprintf("ProviderRuntimeAPIProjection{generation:%d,state:%s,canStart:%t,recovering:%t,stopping:%t}", projection.Generation, projection.State, projection.CanStart, projection.Recovering, projection.Stopping)
}
func (projection ProviderRuntimeAPIProjection) GoString() string { return projection.String() }
func (projection ProviderRuntimeAPIProjection) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, projection.String())
}

type ProviderBuildAPIProjection struct {
	Generation       uint64             `json:"generation"`
	State            ProviderBuildState `json:"state"`
	ReleaseAvailable bool               `json:"releaseAvailable"`
	Size             int64              `json:"size,omitempty"`
	UpdatedAt        time.Time          `json:"updatedAt,omitempty"`
	Stale            bool               `json:"stale"`
	SafeErrorCode    string             `json:"safeErrorCode,omitempty"`
	SafeMessage      string             `json:"safeMessage,omitempty"`
}

// NormalizeProviderBuildPublicError is the application boundary for provider
// build errors. The provider controls neither the public code set nor message.
func NormalizeProviderBuildPublicError(code string) (string, string) {
	return infrasandbox.NormalizeBuildSafeError(code)
}

func (projection ProviderBuildAPIProjection) String() string {
	return fmt.Sprintf("ProviderBuildAPIProjection{generation:%d,state:%s,releaseAvailable:%t,size:%d}", projection.Generation, projection.State, projection.ReleaseAvailable, projection.Size)
}
func (projection ProviderBuildAPIProjection) GoString() string { return projection.String() }
func (projection ProviderBuildAPIProjection) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, projection.String())
}

func (facade *ProviderAPIFacade) StartRuntime(ctx context.Context, request ProviderRuntimeMutationRequest) (*ProviderRuntimeAPIProjection, error) {
	if facade == nil || facade.runtime == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderRuntimeMutationRequest(request) {
		return nil, providerControlInputError(ctx)
	}
	projection, err := facade.runtime.Start(ctx, ProviderRuntimeStartInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, ActorUserID: request.ActorUserID, OperationID: request.OperationID, ActorID: request.ActorID,
	})
	return providerRuntimeAPIResult(ctx, projection, err)
}

func (facade *ProviderAPIFacade) RuntimeStatus(ctx context.Context, request ProviderRuntimeStatusRequest) (*ProviderRuntimeAPIProjection, error) {
	if facade == nil || facade.runtime == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderAPIScope(request.SpaceID, request.ProjectID) {
		return nil, providerControlInputError(ctx)
	}
	projection, err := facade.runtime.Status(ctx, ProviderRuntimeStatusInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID,
		OperationID: providerRuntimeStableID("appdev_api_status", request.SpaceID, request.ProjectID), ActorID: request.ActorID,
	})
	return providerRuntimeAPIResult(ctx, projection, err)
}

func (facade *ProviderAPIFacade) StopRuntime(ctx context.Context, request ProviderRuntimeMutationRequest) (*ProviderRuntimeAPIProjection, error) {
	if facade == nil || facade.runtime == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderRuntimeMutationRequest(request) {
		return nil, providerControlInputError(ctx)
	}
	observed, err := facade.runtime.Status(ctx, ProviderRuntimeStatusInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID,
		OperationID: providerRuntimeStableID("appdev_api_stop_status", request.OperationID, request.SpaceID, request.ProjectID), ActorID: request.ActorID,
	})
	if err != nil || observed == nil {
		return providerRuntimeAPIResult(ctx, observed, err)
	}
	expectedGeneration := observed.Generation
	projection, err := facade.runtime.Stop(ctx, ProviderRuntimeStopInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, OperationID: request.OperationID, ActorID: request.ActorID,
		ExpectedGeneration: &expectedGeneration,
	})
	return providerRuntimeAPIResult(ctx, projection, err)
}

func (facade *ProviderAPIFacade) RestartRuntime(ctx context.Context, request ProviderRuntimeMutationRequest) (*ProviderRuntimeAPIProjection, error) {
	if facade == nil || facade.runtime == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderRuntimeMutationRequest(request) {
		return nil, providerControlInputError(ctx)
	}
	startInput := ProviderRuntimeStartInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, ActorUserID: request.ActorUserID,
		OperationID: providerRuntimeStableID("appdev_api_restart_start", request.OperationID, request.SpaceID, request.ProjectID), ActorID: request.ActorID,
	}
	observed, statusErr := facade.runtime.Status(ctx, ProviderRuntimeStatusInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID,
		OperationID: providerRuntimeStableID("appdev_api_restart_status", request.OperationID, request.SpaceID, request.ProjectID), ActorID: request.ActorID,
	})
	if statusErr != nil || observed == nil {
		return providerRuntimeAPIResult(ctx, observed, statusErr)
	}
	current, matched, err := facade.matchCurrentStartOperation(ctx, startInput)
	if err != nil {
		return providerRuntimeAPIResult(ctx, current, err)
	}
	if matched {
		return providerRuntimeAPIResult(ctx, current, nil)
	}
	stopOperation := providerRuntimeStableID("appdev_api_restart_stop", request.OperationID, request.SpaceID, request.ProjectID)
	expectedGeneration := observed.Generation
	stopped, err := facade.runtime.Stop(ctx, ProviderRuntimeStopInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, OperationID: stopOperation, ActorID: request.ActorID,
		ExpectedGeneration: &expectedGeneration,
	})
	stopProjection, safeErr := providerRuntimeAPIResult(ctx, stopped, err)
	if safeErr != nil || !providerRuntimeAPIAllowsStart(stopped) {
		return stopProjection, safeErr
	}
	started, err := facade.runtime.Start(ctx, startInput)
	return providerRuntimeAPIResult(ctx, started, err)
}

func (facade *ProviderAPIFacade) BeginBuild(ctx context.Context, request ProviderBuildOperationRequest) (*ProviderBuildAPIProjection, error) {
	if facade == nil || facade.build == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderBuildAPIRequest(request) {
		return nil, providerControlInputError(ctx)
	}
	projection, err := facade.build.BeginBuild(ctx, ProviderBuildBeginInput{SpaceID: request.SpaceID, ProjectID: request.ProjectID, OperationID: request.OperationID})
	return facade.providerBuildAPIResult(ctx, request.SpaceID, request.ProjectID, projection, err)
}

func (facade *ProviderAPIFacade) PollBuild(ctx context.Context, request ProviderBuildOperationRequest) (*ProviderBuildAPIProjection, error) {
	if facade == nil || facade.build == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderBuildAPIRequest(request) {
		return nil, providerControlInputError(ctx)
	}
	projection, err := facade.build.PollBuild(ctx, ProviderBuildPollInput{SpaceID: request.SpaceID, ProjectID: request.ProjectID, OperationID: request.OperationID})
	return facade.providerBuildAPIResult(ctx, request.SpaceID, request.ProjectID, projection, err)
}

func (facade *ProviderAPIFacade) RecoverBuild(ctx context.Context, request ProviderBuildRecoverRequest) (*ProviderBuildAPIProjection, error) {
	if facade == nil || facade.build == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderAPIScope(request.SpaceID, request.ProjectID) {
		return nil, providerControlInputError(ctx)
	}
	projection, err := facade.build.RecoverBuild(ctx, ProviderBuildRecoverInput{SpaceID: request.SpaceID, ProjectID: request.ProjectID})
	return facade.providerBuildAPIResult(ctx, request.SpaceID, request.ProjectID, projection, err)
}

func (facade *ProviderAPIFacade) OpenRelease(ctx context.Context, request ProviderReleaseRequest) (*ProviderRelease, error) {
	if facade == nil || facade.releases == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderAPIScope(request.SpaceID, request.ProjectID) {
		return nil, providerControlInputError(ctx)
	}
	release, err := facade.releases.OpenRelease(ctx, request)
	if err != nil || release == nil {
		return nil, normalizeProviderControlError(ctx, err)
	}
	return release, nil
}

func (facade *ProviderAPIFacade) RestoreSnapshot(ctx context.Context, request ProviderSnapshotRestoreRequest) (*ProviderRuntimeAPIProjection, error) {
	if facade == nil || facade.runtime == nil || facade.snapshots == nil {
		return nil, ErrProviderControlUnavailable
	}
	if !validProviderRuntimeMutationRequest(ProviderRuntimeMutationRequest{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, ActorUserID: request.ActorUserID, OperationID: request.OperationID, ActorID: request.ActorID,
	}) || !validProviderAPISafeID(request.SnapshotID) {
		return nil, providerControlInputError(ctx)
	}
	operationHash, err := domainappdev.HashProviderSnapshotRestoreOperation(request.SpaceID, request.ProjectID, request.SnapshotID, request.OperationID)
	if err != nil {
		return nil, providerControlInputError(ctx)
	}
	parentOperationHash, err := domainappdev.HashProviderSnapshotRestoreParentOperation(request.SpaceID, request.ProjectID, request.OperationID)
	if err != nil {
		return nil, providerControlInputError(ctx)
	}
	startInput := ProviderRuntimeStartInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, ActorUserID: request.ActorUserID,
		OperationID: providerSnapshotRestoreStableID("start", request), ActorID: request.ActorID,
	}
	var status *ProviderRuntimeProjection
	var statusProjection *ProviderRuntimeAPIProjection
	loadStatus := func() error {
		if status != nil {
			return nil
		}
		observed, statusErr := facade.runtime.Status(ctx, ProviderRuntimeStatusInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID,
			OperationID: providerSnapshotRestoreStableID("status", request), ActorID: request.ActorID,
		})
		projected, safeErr := providerRuntimeAPIResult(ctx, observed, statusErr)
		if safeErr != nil {
			return safeErr
		}
		status = observed
		statusProjection = projected
		return nil
	}
	journal, err := facade.snapshots.LoadProviderSnapshotRestore(ctx, domainappdev.LoadProviderSnapshotRestoreInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID,
		OperationHash: operationHash, ParentOperationHash: parentOperationHash,
	})
	if err != nil && !errors.Is(err, domainappdev.ErrNotFound) {
		return nil, normalizeProviderControlError(ctx, err)
	}
	if journal == nil {
		if err := loadStatus(); err != nil {
			return statusProjection, err
		}
		wasActive := !providerRuntimeAPIAllowsStart(status)
		var runtimeGeneration uint64
		if wasActive {
			runtimeGeneration = status.Generation
			if runtimeGeneration == 0 {
				return nil, ErrProviderControlConflict
			}
		}
		journal, err = facade.snapshots.ReserveProviderSnapshotRestore(ctx, domainappdev.ReserveProviderSnapshotRestoreInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID,
			OperationHash: operationHash, ParentOperationHash: parentOperationHash,
			RuntimeGeneration: runtimeGeneration, RestartRequired: wasActive,
		})
		if err != nil {
			return nil, normalizeProviderControlError(ctx, err)
		}
	}
	if !providerSnapshotRestoreJournalMatches(journal, request, operationHash, parentOperationHash) {
		return nil, ErrProviderControlConflict
	}

	current, matched, matchErr := facade.matchCurrentStartOperation(ctx, startInput)
	if matchErr != nil {
		return providerRuntimeAPIResult(ctx, current, matchErr)
	}
	if journal.Phase == domainappdev.ProviderSnapshotRestorePhasePending && matched {
		return nil, ErrProviderControlConflict
	}
	if (journal.Phase == domainappdev.ProviderSnapshotRestorePhaseStarted || journal.Phase == domainappdev.ProviderSnapshotRestorePhaseCompleted) && journal.RestartRequired {
		if !matched || current == nil || current.Generation != journal.StartedGeneration {
			return nil, ErrProviderControlConflict
		}
		completed, completeErr := facade.snapshots.CompleteProviderSnapshotRestore(ctx, domainappdev.CompleteProviderSnapshotRestoreInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID,
			OperationHash: operationHash, ParentOperationHash: parentOperationHash,
			ExpectedSourceVersion: journal.ResultSourceVersion, StartedGeneration: journal.StartedGeneration,
		})
		if completeErr != nil {
			return providerRuntimeAPIResult(ctx, current, normalizeProviderControlError(ctx, completeErr))
		}
		if !providerSnapshotRestoreJournalMatches(completed, request, operationHash, parentOperationHash) || completed.Phase != domainappdev.ProviderSnapshotRestorePhaseCompleted || completed.StartedGeneration != current.Generation {
			return nil, ErrProviderControlConflict
		}
		return providerRuntimeAPIResult(ctx, current, nil)
	}
	if journal.Phase == domainappdev.ProviderSnapshotRestorePhaseCompleted {
		if matched || journal.RestartRequired || journal.StartedGeneration != 0 {
			return nil, ErrProviderControlConflict
		}
		if err := loadStatus(); err != nil {
			return statusProjection, err
		}
		completed, completeErr := facade.snapshots.CompleteProviderSnapshotRestore(ctx, domainappdev.CompleteProviderSnapshotRestoreInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID,
			OperationHash: operationHash, ParentOperationHash: parentOperationHash, ExpectedSourceVersion: journal.ResultSourceVersion,
		})
		if completeErr != nil || !providerSnapshotRestoreJournalMatches(completed, request, operationHash, parentOperationHash) || completed.Phase != domainappdev.ProviderSnapshotRestorePhaseCompleted {
			if completeErr == nil {
				completeErr = ErrProviderControlConflict
			}
			return nil, normalizeProviderControlError(ctx, completeErr)
		}
		return statusProjection, nil
	}
	if journal.Phase == domainappdev.ProviderSnapshotRestorePhaseRestored && matched {
		if !journal.RestartRequired || current == nil || current.Generation == 0 {
			return nil, ErrProviderControlConflict
		}
		journal, err = facade.snapshots.MarkProviderSnapshotRestoreStarted(ctx, domainappdev.MarkProviderSnapshotRestoreStartedInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID,
			OperationHash: operationHash, ParentOperationHash: parentOperationHash,
			ExpectedSourceVersion: journal.ResultSourceVersion, StartedGeneration: current.Generation,
		})
		if err != nil {
			return providerRuntimeAPIResult(ctx, current, normalizeProviderControlError(ctx, err))
		}
	}
	if journal.Phase == domainappdev.ProviderSnapshotRestorePhasePending && journal.RestartRequired {
		if err := loadStatus(); err != nil {
			return statusProjection, err
		}
		if status == nil || status.Generation != journal.RuntimeGeneration {
			facade.failProviderSnapshotRestoreDeterministic(ctx, journal, "runtime_generation_changed", "runtime generation changed before snapshot restore")
			return nil, ErrProviderControlConflict
		}
		expectedGeneration := journal.RuntimeGeneration
		stopped, stopErr := facade.runtime.Stop(ctx, ProviderRuntimeStopInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID,
			OperationID: providerSnapshotRestoreStableID("stop", request), ActorID: request.ActorID, ExpectedGeneration: &expectedGeneration,
		})
		stopProjection, normalized := providerRuntimeAPIResult(ctx, stopped, stopErr)
		if normalized != nil || !providerRuntimeAPIAllowsStart(stopped) {
			return stopProjection, normalized
		}
		status = stopped
		statusProjection = stopProjection
	} else if journal.Phase == domainappdev.ProviderSnapshotRestorePhasePending {
		if err := loadStatus(); err != nil {
			return statusProjection, err
		}
		if !providerRuntimeAPIAllowsStart(status) {
			return nil, ErrProviderControlConflict
		}
	}
	if journal.Phase == domainappdev.ProviderSnapshotRestorePhasePending {
		applied, applyErr := facade.snapshots.ApplyProviderSnapshotRestore(ctx, domainappdev.ApplyProviderSnapshotRestoreInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID, OperationHash: operationHash,
			ParentOperationHash: parentOperationHash, ExpectedSourceVersion: journal.SourceVersion,
		})
		if applyErr != nil {
			if providerSnapshotRestoreErrorIsDeterministic(applyErr) {
				if failErr := facade.failProviderSnapshotRestoreDeterministic(ctx, journal, "snapshot_restore_rejected", "snapshot restore cannot continue"); failErr != nil && !errors.Is(failErr, domainappdev.ErrProviderSnapshotRestoreConflict) {
					return nil, normalizeProviderControlError(ctx, failErr)
				}
			}
			return nil, normalizeProviderControlError(ctx, applyErr)
		}
		journal = applied
		if !providerSnapshotRestoreJournalMatches(journal, request, operationHash, parentOperationHash) || journal.Phase != domainappdev.ProviderSnapshotRestorePhaseRestored {
			return nil, ErrProviderControlConflict
		}
	}
	if !journal.RestartRequired {
		if matched || journal.Phase != domainappdev.ProviderSnapshotRestorePhaseRestored {
			return nil, ErrProviderControlConflict
		}
		journal, err = facade.snapshots.CompleteProviderSnapshotRestore(ctx, domainappdev.CompleteProviderSnapshotRestoreInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID,
			OperationHash: operationHash, ParentOperationHash: parentOperationHash, ExpectedSourceVersion: journal.ResultSourceVersion,
		})
		if err != nil {
			return nil, normalizeProviderControlError(ctx, err)
		}
		if !providerSnapshotRestoreJournalMatches(journal, request, operationHash, parentOperationHash) || journal.Phase != domainappdev.ProviderSnapshotRestorePhaseCompleted {
			return nil, ErrProviderControlConflict
		}
		return statusProjection, nil
	}
	if journal.Phase == domainappdev.ProviderSnapshotRestorePhaseRestored {
		started, startErr := facade.runtime.Start(ctx, startInput)
		if startErr != nil {
			return providerRuntimeAPIResult(ctx, started, startErr)
		}
		current, matched, err = facade.matchCurrentStartOperation(ctx, startInput)
		if err != nil || !matched || current == nil {
			if err == nil {
				err = ErrProviderControlConflict
			}
			return providerRuntimeAPIResult(ctx, current, err)
		}
		journal, err = facade.snapshots.MarkProviderSnapshotRestoreStarted(ctx, domainappdev.MarkProviderSnapshotRestoreStartedInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID,
			OperationHash: operationHash, ParentOperationHash: parentOperationHash,
			ExpectedSourceVersion: journal.ResultSourceVersion, StartedGeneration: current.Generation,
		})
		if err != nil {
			return providerRuntimeAPIResult(ctx, current, normalizeProviderControlError(ctx, err))
		}
	}
	if journal.Phase != domainappdev.ProviderSnapshotRestorePhaseStarted {
		return nil, ErrProviderControlConflict
	}
	current, matched, err = facade.matchCurrentStartOperation(ctx, startInput)
	if err != nil || !matched || current == nil || current.Generation != journal.StartedGeneration {
		if err == nil {
			err = ErrProviderControlConflict
		}
		return providerRuntimeAPIResult(ctx, current, err)
	}
	_, err = facade.snapshots.CompleteProviderSnapshotRestore(ctx, domainappdev.CompleteProviderSnapshotRestoreInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, SnapshotID: request.SnapshotID,
		OperationHash: operationHash, ParentOperationHash: parentOperationHash,
		ExpectedSourceVersion: journal.ResultSourceVersion, StartedGeneration: journal.StartedGeneration,
	})
	return providerRuntimeAPIResult(ctx, current, err)
}

func providerSnapshotRestoreStableID(stage string, request ProviderSnapshotRestoreRequest) string {
	return providerRuntimeStableID("appdev_api_snapshot_"+stage, request.OperationID, request.SpaceID, request.ProjectID, request.SnapshotID)
}

func providerSnapshotRestoreJournalMatches(journal *domainappdev.ProviderSnapshotRestoreJournal, request ProviderSnapshotRestoreRequest, operationHash domainappdev.ProviderSnapshotRestoreOperationHash, parentOperationHash domainappdev.ProviderSnapshotRestoreParentOperationHash) bool {
	return journal != nil && journal.SpaceID == request.SpaceID && journal.ProjectID == request.ProjectID && journal.SnapshotID == request.SnapshotID &&
		journal.OperationHash.Equal(operationHash) && journal.ParentOperationHash.Equal(parentOperationHash)
}

func (facade *ProviderAPIFacade) failProviderSnapshotRestoreDeterministic(ctx context.Context, journal *domainappdev.ProviderSnapshotRestoreJournal, code, message string) error {
	if facade == nil || facade.snapshots == nil || journal == nil || journal.Phase != domainappdev.ProviderSnapshotRestorePhasePending {
		return domainappdev.ErrProviderSnapshotRestoreConflict
	}
	_, err := facade.snapshots.FailProviderSnapshotRestore(ctx, domainappdev.FailProviderSnapshotRestoreInput{
		SpaceID: journal.SpaceID, ProjectID: journal.ProjectID, SnapshotID: journal.SnapshotID,
		OperationHash: journal.OperationHash, ParentOperationHash: journal.ParentOperationHash,
		ExpectedSourceVersion: journal.SourceVersion, SafeErrorCode: code, SafeErrorMessage: message,
	})
	return err
}

func providerSnapshotRestoreErrorIsDeterministic(err error) bool {
	return errors.Is(err, domainappdev.ErrProviderSnapshotRestoreInvalid) || errors.Is(err, domainappdev.ErrProviderSnapshotRestoreConflict) || errors.Is(err, domainappdev.ErrNotFound)
}

func (facade *ProviderAPIFacade) matchCurrentStartOperation(ctx context.Context, input ProviderRuntimeStartInput) (*ProviderRuntimeProjection, bool, error) {
	matcher, ok := facade.runtime.(ProviderRuntimeStartOperationMatcher)
	if !ok {
		return nil, false, ErrProviderControlUnavailable
	}
	return matcher.MatchCurrentStartOperation(ctx, input)
}

func providerRuntimeAPIResult(ctx context.Context, projection *ProviderRuntimeProjection, err error) (*ProviderRuntimeAPIProjection, error) {
	result := providerRuntimeAPIProjection(projection)
	if err == nil && projection != nil {
		return result, nil
	}
	safeErr := normalizeProviderControlError(ctx, err)
	if result != nil {
		result.SafeMessage = providerControlSafeMessage(safeErr)
	}
	return result, safeErr
}

func providerRuntimeAPIProjection(projection *ProviderRuntimeProjection) *ProviderRuntimeAPIProjection {
	if projection == nil {
		return nil
	}
	preview := ""
	if validProviderAPIRelativePreview(projection.PreviewRoute) {
		preview = projection.PreviewRoute
	}
	return &ProviderRuntimeAPIProjection{
		Generation: projection.Generation, State: projection.State, CanStart: projection.CanStart,
		Recovering: projection.Recovering, Stopping: projection.Stopping, RelativePreview: preview,
	}
}

func providerBuildAPIResult(ctx context.Context, projection *ProviderBuildProjection, err error) (*ProviderBuildAPIProjection, error) {
	var result *ProviderBuildAPIProjection
	if projection != nil {
		result = &ProviderBuildAPIProjection{
			Generation: projection.Generation, State: projection.State, ReleaseAvailable: projection.ReleaseAvailable,
			Size: projection.Size, UpdatedAt: projection.UpdatedAt, Stale: projection.Stale,
			SafeErrorCode: providerBuildPublicErrorCode(projection.State, projection.SafeErrorCode),
			SafeMessage:   providerBuildPublicErrorMessage(projection.State, projection.SafeErrorCode, projection.SafeMessage),
		}
	}
	if err == nil && projection != nil {
		return result, nil
	}
	safeErr := normalizeProviderControlError(ctx, err)
	if result != nil && result.SafeMessage == "" {
		result.SafeMessage = providerControlSafeMessage(safeErr)
	}
	return result, safeErr
}

func providerBuildPublicErrorCode(state ProviderBuildState, code string) string {
	if state != ProviderBuildStateFailed {
		return ""
	}
	normalizedCode, _ := NormalizeProviderBuildPublicError(code)
	return normalizedCode
}

func providerBuildPublicErrorMessage(state ProviderBuildState, code, nonProviderMessage string) string {
	if state != ProviderBuildStateFailed {
		return safeProviderAPIMessage(nonProviderMessage)
	}
	_, message := NormalizeProviderBuildPublicError(code)
	return message
}

func (facade *ProviderAPIFacade) providerBuildAPIResult(ctx context.Context, spaceID, projectID string, projection *ProviderBuildProjection, err error) (*ProviderBuildAPIProjection, error) {
	result, safeErr := providerBuildAPIResult(ctx, projection, err)
	if safeErr != nil || result == nil || result.State != ProviderBuildStateReady || !result.ReleaseAvailable {
		return result, safeErr
	}
	if facade == nil || facade.buildSource == nil || result.UpdatedAt.IsZero() {
		result.SafeMessage = providerControlSafeMessage(ErrProviderControlUnavailable)
		return result, ErrProviderControlUnavailable
	}
	project, projectErr := facade.buildSource.GetProject(ctx, spaceID, projectID)
	if projectErr != nil || project == nil {
		result.SafeMessage = providerControlSafeMessage(normalizeProviderControlError(ctx, projectErr))
		return result, normalizeProviderControlError(ctx, projectErr)
	}
	if project.SpaceID != spaceID || project.ID != projectID || project.SourceUpdatedAt.IsZero() {
		result.SafeMessage = providerControlSafeMessage(ErrProviderControlUnavailable)
		return result, ErrProviderControlUnavailable
	}
	result.Stale = project.SourceUpdatedAt.After(result.UpdatedAt)
	return result, nil
}

func providerRuntimeAPIAllowsStart(projection *ProviderRuntimeProjection) bool {
	return projection != nil && projection.CanStart &&
		(projection.State == ProviderRuntimeStateStopped || projection.State == ProviderRuntimeStateCleanupComplete)
}

func validProviderRuntimeMutationRequest(request ProviderRuntimeMutationRequest) bool {
	return validProviderAPIScope(request.SpaceID, request.ProjectID) && validProviderAPIOperationID(request.OperationID) && validProviderAPIActor(request.ActorID)
}

func validProviderBuildAPIRequest(request ProviderBuildOperationRequest) bool {
	return validProviderAPIScope(request.SpaceID, request.ProjectID) && validProviderAPIOperationID(request.OperationID)
}

func validProviderAPIScope(spaceID, projectID string) bool {
	return domainappdev.ValidProviderExecutionSpaceID(spaceID) && domainappdev.ValidProviderExecutionProjectID(projectID)
}

func validProviderAPIOperationID(value string) bool {
	if len(value) < providerAPIMinOperationIDLength || len(value) > providerAPIMaxOperationIDLength || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return domainappdev.ValidProviderExecutionIdempotencyKey(value)
}

func validProviderAPIActor(value string) bool {
	return value == "" || (len(value) <= 128 && value == strings.TrimSpace(value) && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n\t"))
}

func validProviderAPISafeID(value string) bool {
	return len(value) > 0 && len(value) <= 128 && value == strings.TrimSpace(value) && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n\t/\\")
}

func validProviderAPIRelativePreview(value string) bool {
	return value != "" && len(value) <= 512 && strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") &&
		!strings.ContainsAny(value, "?#\\\x00\r\n\t") && !strings.Contains(value, "..") && utf8.ValidString(value)
}

func providerControlInputError(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrProviderControlInvalid
}

func normalizeProviderControlError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, ErrProviderRuntimeInvalid) || errors.Is(err, ErrProviderBuildInvalid) ||
		errors.Is(err, domainappdev.ErrProviderExecutionInvalid) || errors.Is(err, domainappdev.ErrArtifactGrantInvalid) ||
		errors.Is(err, domainappdev.ErrProviderSnapshotRestoreInvalid) {
		return ErrProviderControlInvalid
	}
	if errors.Is(err, domainappdev.ErrProviderExecutionNotFound) || errors.Is(err, domainappdev.ErrNotFound) {
		return ErrProviderControlNotFound
	}
	if errors.Is(err, ErrProviderRuntimeConflict) || errors.Is(err, ErrProviderBuildConflict) || errors.Is(err, domainappdev.ErrProviderExecutionConflict) ||
		errors.Is(err, domainappdev.ErrProviderSnapshotRestoreConflict) {
		return ErrProviderControlConflict
	}
	return ErrProviderControlUnavailable
}

func providerControlSafeMessage(err error) string {
	switch {
	case errors.Is(err, ErrProviderControlConflict):
		return "provider operation is already in progress"
	case errors.Is(err, ErrProviderControlNotFound):
		return "provider resource is not available"
	case errors.Is(err, ErrProviderControlInvalid):
		return "provider operation input is invalid"
	case err != nil:
		return "provider operation is temporarily unavailable"
	default:
		return ""
	}
}

func safeProviderAPIMessage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > providerAPIMaxSafeMessageLength || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n\t") {
		return ""
	}
	return value
}

type ProviderReleaseRequest struct {
	SpaceID   string
	ProjectID string
}

type LoadReadyProviderArtifactRequest struct {
	SpaceID   string
	ProjectID string
}

type ProviderReadyArtifact struct {
	Generation uint64
	Size       int64
	UpdatedAt  time.Time
	objectKey  string
	digest     string
}

func (*ProviderReadyArtifact) String() string   { return "ProviderReadyArtifact{internal:<redacted>}" }
func (*ProviderReadyArtifact) GoString() string { return "ProviderReadyArtifact{internal:<redacted>}" }
func (*ProviderReadyArtifact) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderReadyArtifact{internal:<redacted>}")
}
func (*ProviderReadyArtifact) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderControlUnavailable
}

type ProviderReadyArtifactSource interface {
	LoadReadyArtifact(context.Context, LoadReadyProviderArtifactRequest) (*ProviderReadyArtifact, error)
}

type ProviderReleaseProjectSource interface {
	GetProject(context.Context, string, string) (*domainappdev.Project, error)
}

type ProviderReadyReleaseReader struct {
	artifacts ProviderReadyArtifactSource
	projects  ProviderReleaseProjectSource
	store     ArtifactObjectStore
	fileOps   ArtifactGatewayTempFileOps
	tempDir   string
}

func NewProviderReadyReleaseReader(artifacts ProviderReadyArtifactSource, projects ProviderReleaseProjectSource, store ArtifactObjectStore) (*ProviderReadyReleaseReader, error) {
	if artifacts == nil || projects == nil || store == nil {
		return nil, ErrProviderControlInvalid
	}
	return &ProviderReadyReleaseReader{artifacts: artifacts, projects: projects, store: store, fileOps: osArtifactGatewayTempFileOps{}}, nil
}

func (reader *ProviderReadyReleaseReader) OpenRelease(ctx context.Context, request ProviderReleaseRequest) (*ProviderRelease, error) {
	if reader == nil || reader.artifacts == nil || reader.projects == nil || reader.store == nil || !validProviderAPIScope(request.SpaceID, request.ProjectID) {
		return nil, providerControlInputError(ctx)
	}
	artifact, err := reader.artifacts.LoadReadyArtifact(ctx, LoadReadyProviderArtifactRequest{SpaceID: request.SpaceID, ProjectID: request.ProjectID})
	if err != nil || artifact == nil || artifact.Generation == 0 || artifact.Size <= 0 || artifact.UpdatedAt.IsZero() || artifact.objectKey == "" {
		return nil, normalizeProviderControlError(ctx, err)
	}
	digest, err := domainappdev.ParseArtifactGrantDigest(artifact.digest)
	if err != nil || digest.IsZero() {
		return nil, ErrProviderControlUnavailable
	}
	project, err := reader.projects.GetProject(ctx, request.SpaceID, request.ProjectID)
	if err != nil || project == nil || project.SpaceID != request.SpaceID || project.ID != request.ProjectID {
		return nil, normalizeProviderControlError(ctx, err)
	}
	object, err := reader.store.OpenArtifact(ctx, artifact.objectKey, artifact.Size)
	if err != nil || object == nil || object.Body == nil {
		return nil, normalizeProviderControlError(ctx, err)
	}
	objectBody := object.Body
	closedObject := false
	defer func() {
		if !closedObject {
			_ = objectBody.Close()
		}
	}()
	if object.Size != artifact.Size || (object.Digest != nil && *object.Digest != digest) {
		return nil, ErrProviderControlUnavailable
	}
	staged, err := createArtifactGatewayTemp(reader.fileOps, reader.tempDir)
	if err != nil {
		return nil, ErrProviderControlUnavailable
	}
	keepStaged := false
	defer func() {
		if !keepStaged {
			closeArtifactGatewayTemp(staged)
		}
	}()
	size, actualDigest, err := stageArtifactGatewayStream(ctx, staged, objectBody, artifact.Size)
	closeErr := objectBody.Close()
	closedObject = true
	if err != nil || size != artifact.Size || actualDigest != digest {
		return nil, normalizeProviderControlError(ctx, err)
	}
	if closeErr != nil {
		return nil, ErrProviderControlUnavailable
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return nil, ErrProviderControlUnavailable
	}
	keepStaged = true
	return &ProviderRelease{
		Size: artifact.Size, FileName: strings.TrimSuffix(safeBuildArchiveName(project.Name), ".zip") + "-release.zip",
		ContentType: "application/zip", Stale: !project.SourceUpdatedAt.IsZero() && project.SourceUpdatedAt.After(artifact.UpdatedAt),
		UpdatedAt: artifact.UpdatedAt, body: &ArtifactDownload{Size: size, Digest: actualDigest, file: staged},
	}, nil
}

type ProviderRelease struct {
	Size        int64     `json:"size"`
	FileName    string    `json:"fileName"`
	ContentType string    `json:"contentType"`
	Stale       bool      `json:"stale"`
	UpdatedAt   time.Time `json:"updatedAt"`

	mu     sync.Mutex
	body   io.ReadCloser
	closed bool
}

func (release *ProviderRelease) Read(buffer []byte) (int, error) {
	if release == nil {
		return 0, io.ErrClosedPipe
	}
	release.mu.Lock()
	defer release.mu.Unlock()
	if release.closed || release.body == nil {
		return 0, io.ErrClosedPipe
	}
	return release.body.Read(buffer)
}

func (release *ProviderRelease) Close() error {
	if release == nil {
		return nil
	}
	release.mu.Lock()
	defer release.mu.Unlock()
	if release.closed {
		return nil
	}
	release.closed = true
	if release.body == nil {
		return nil
	}
	err := release.body.Close()
	release.body = nil
	return err
}

func (*ProviderRelease) String() string   { return "ProviderRelease{stream:<redacted>}" }
func (*ProviderRelease) GoString() string { return "ProviderRelease{stream:<redacted>}" }
func (*ProviderRelease) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderRelease{stream:<redacted>}")
}
func (*ProviderRelease) MarshalJSON() ([]byte, error) { return nil, ErrProviderControlUnavailable }

// LoadReadyArtifact is an internal, tenant-scoped read. It never exposes the
// object identity through generic formatting or public projections.
func (s *ProviderExecutionService) LoadReadyArtifact(ctx context.Context, request LoadReadyProviderArtifactRequest) (*ProviderReadyArtifact, error) {
	if err := s.available(ctx); err != nil || !validProviderAPIScope(request.SpaceID, request.ProjectID) {
		return nil, ErrProviderExecutionServiceInvalid
	}
	repository, ok := s.repository.(domainappdev.ProviderExecutionReadyArtifactRepository)
	if !ok {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	record, err := repository.LoadLatestReadyArtifact(ctx, domainappdev.LoadReadyProviderArtifactInput{SpaceID: request.SpaceID, ProjectID: request.ProjectID})
	if err != nil {
		return nil, err
	}
	descriptor, err := domainappdev.NormalizeProviderBuildArtifactDescriptor(domainappdev.ProviderBuildArtifactDescriptor{
		Kind: record.ArtifactKind, Digest: record.ArtifactDigest, Size: record.ArtifactSize,
	})
	objectKey, keyErr := domainappdev.NormalizeProviderExecutionArtifactObjectKey(record.ArtifactObjectKey)
	if err != nil || keyErr != nil || record.ArtifactStatus != domainappdev.ProviderExecutionArtifactReady || objectKey == "" || record.Generation == 0 || record.ArtifactUpdatedAt == nil || record.ArtifactUpdatedAt.IsZero() {
		return nil, ErrProviderControlUnavailable
	}
	return &ProviderReadyArtifact{
		Generation: record.Generation, Size: descriptor.Size, UpdatedAt: record.ArtifactUpdatedAt.UTC(),
		objectKey: objectKey, digest: descriptor.Digest,
	}, nil
}

// SetProviderAPI injects the provider-only facade during wiring. It never
// derives a facade from the legacy RuntimeManager or host build path.
func (s *Service) SetProviderAPI(facade *ProviderAPIFacade) error {
	if s == nil || facade == nil {
		return ErrProviderControlInvalid
	}
	s.providerArchiveMu.Lock()
	defer s.providerArchiveMu.Unlock()
	s.providerGeneration++
	s.providerAPI = facade
	return nil
}

func (s *Service) ClearProviderAPIIfCurrent(facade *ProviderAPIFacade) {
	if s == nil || facade == nil {
		return
	}
	s.providerArchiveMu.Lock()
	defer s.providerArchiveMu.Unlock()
	if s.providerAPI == facade {
		s.providerGeneration++
		s.providerAPI = nil
	}
}

func (s *Service) ProviderAPI() (*ProviderAPIFacade, error) {
	if s == nil {
		return nil, ErrProviderControlUnavailable
	}
	s.providerArchiveMu.RLock()
	defer s.providerArchiveMu.RUnlock()
	if s.providerAPI == nil {
		return nil, ErrProviderControlUnavailable
	}
	return s.providerAPI, nil
}

var _ json.Marshaler = (*ProviderAPIFacade)(nil)
var _ json.Marshaler = (*ProviderRelease)(nil)
