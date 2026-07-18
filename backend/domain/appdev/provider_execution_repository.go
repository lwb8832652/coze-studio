// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type EnsureProviderExecutionStartInput struct {
	ID              string
	SpaceID         string
	ProjectID       string
	IdempotencyKey  string
	ProviderKey     string
	ProviderScope   domainsandbox.Scope
	RequireNoActive bool
}

type ClaimProviderExecutionInput struct {
	SpaceID            string
	ProjectID          string
	Generation         uint64
	ExpectedVersion    uint64
	OwnerHash          ProviderExecutionOwnerHash
	ExpectedOwnerEpoch uint64
	LeaseDuration      time.Duration
}

type ProviderExecutionOwnerCAS struct {
	SpaceID         string
	ProjectID       string
	Generation      uint64
	ExpectedVersion uint64
	OwnerHash       ProviderExecutionOwnerHash
	OwnerEpoch      uint64
	ProviderKey     string
	ProviderScope   domainsandbox.Scope
}

type RenewProviderExecutionOwnerInput struct {
	OwnerCAS      ProviderExecutionOwnerCAS
	LeaseDuration time.Duration
}

type ReleaseProviderExecutionOwnerInput struct {
	OwnerCAS      ProviderExecutionOwnerCAS
	OperationHash ProviderExecutionOperationHash
}

type StartProviderSubmissionInput struct {
	OwnerCAS            ProviderExecutionOwnerCAS
	OperationHash       ProviderExecutionOperationHash
	ProviderOperationID string
	RequestDigest       ProviderExecutionLaunchRequestDigest
	LeaseDuration       time.Duration
}

type MarkProviderLaunchSubmittedInput struct {
	OwnerCAS              ProviderExecutionOwnerCAS
	OperationHash         ProviderExecutionOperationHash
	DispatchLeaseDuration time.Duration
}

type ClaimProviderLaunchReconciliationInput struct {
	SpaceID            string
	ProjectID          string
	Generation         uint64
	ExpectedVersion    uint64
	OwnerHash          ProviderExecutionOwnerHash
	ExpectedOwnerEpoch uint64
	ExpectedState      ProviderExecutionLaunchState
	LeaseDuration      time.Duration
	SetDesiredStop     bool
}

type AbortProviderLaunchInput struct {
	OwnerCAS      ProviderExecutionOwnerCAS
	OperationHash ProviderExecutionOperationHash
	ExpectedState ProviderExecutionLaunchState
}

type SaveProviderSubmissionInput struct {
	OwnerCAS              ProviderExecutionOwnerCAS
	OperationHash         ProviderExecutionOperationHash
	ProviderExecutionID   string
	ObservedState         ProviderExecutionObservedState
	ProviderLeaseDuration time.Duration
	PreviewRoute          string
}

type CompleteReconciledProviderLaunchInput struct {
	OwnerCAS                ProviderExecutionOwnerCAS
	LaunchOperationHash     ProviderExecutionOperationHash
	CheckpointOperationHash ProviderExecutionOperationHash
	CheckpointWriteRevision uint64
	CheckpointEnvelope      string
	ProviderExecutionID     string
	ProviderLeaseDuration   time.Duration
	PreviewRoute            string
}

type ProviderExecutionLaunchRepository interface {
	MarkSubmissionSubmitted(context.Context, MarkProviderLaunchSubmittedInput) (*ProviderExecution, error)
	ClaimLaunchReconciliation(context.Context, ClaimProviderLaunchReconciliationInput) (*ProviderExecution, error)
	AbortLaunch(context.Context, AbortProviderLaunchInput) (*ProviderExecution, error)
	CompleteReconciledLaunch(context.Context, CompleteReconciledProviderLaunchInput) (*ProviderExecution, error)
}

type ObserveProviderExecutionActiveInput struct {
	OwnerCAS              ProviderExecutionOwnerCAS
	ProviderExecutionID   string
	ProviderLeaseDuration time.Duration
	PreviewRoute          string
	SafeErrorCode         string
	SafeErrorMessage      string
}

type ReserveProviderBuildInput struct {
	OwnerCAS            ProviderExecutionOwnerCAS
	ProviderExecutionID string
	OperationID         string
	OperationHash       ProviderExecutionOperationHash
}

type AdvanceProviderBuildObservationInput struct {
	OwnerCAS            ProviderExecutionOwnerCAS
	ProviderExecutionID string
	OperationHash       ProviderExecutionOperationHash
	Status              ProviderExecutionArtifactStatus
	Descriptor          ProviderBuildArtifactDescriptor
}

type BeginProviderArtifactPublishInput struct {
	OwnerCAS            ProviderExecutionOwnerCAS
	ProviderExecutionID string
	OperationHash       ProviderExecutionOperationHash
}

type CompleteProviderArtifactPublishInput struct {
	OwnerCAS            ProviderExecutionOwnerCAS
	ProviderExecutionID string
	OperationHash       ProviderExecutionOperationHash
	ObjectKey           string
	Digest              string
	Size                int64
}

type FailProviderBuildInput struct {
	OwnerCAS            ProviderExecutionOwnerCAS
	ProviderExecutionID string
	OperationHash       ProviderExecutionOperationHash
	SafeErrorCode       string
	SafeErrorMessage    string
}

type ProviderExecutionBuildRepository interface {
	ReserveBuild(context.Context, ReserveProviderBuildInput) (*ProviderExecution, error)
	AdvanceBuildObservation(context.Context, AdvanceProviderBuildObservationInput) (*ProviderExecution, error)
	BeginArtifactPublish(context.Context, BeginProviderArtifactPublishInput) (*ProviderExecution, error)
	CompleteArtifactPublish(context.Context, CompleteProviderArtifactPublishInput) (*ProviderExecution, error)
	FailBuild(context.Context, FailProviderBuildInput) (*ProviderExecution, error)
}

type SaveProviderCheckpointInput struct {
	OwnerCAS                ProviderExecutionOwnerCAS
	OperationHash           ProviderExecutionOperationHash
	LaunchOperationHash     ProviderExecutionOperationHash
	CheckpointWriteRevision uint64
	CheckpointEnvelope      string
}

type ReserveProviderCheckpointWriteInput struct {
	OwnerCAS            ProviderExecutionOwnerCAS
	OperationHash       ProviderExecutionOperationHash
	ReservationDuration time.Duration
}

type ProviderExecutionCheckpointReservation struct {
	Revision         uint64
	ReservedVersion  uint64
	AlreadyCommitted bool
	Record           *ProviderExecution
}

type AbortProviderCheckpointWriteInput struct {
	OwnerCAS                ProviderExecutionOwnerCAS
	OperationHash           ProviderExecutionOperationHash
	CheckpointWriteRevision uint64
}

type SetProviderExecutionDesiredStopInput struct{ OwnerCAS ProviderExecutionOwnerCAS }

type BeginProviderExecutionCleanupInput struct{ OwnerCAS ProviderExecutionOwnerCAS }

type AdvanceProviderExecutionTerminalInput struct {
	OwnerCAS          ProviderExecutionOwnerCAS
	OperationHash     ProviderExecutionOperationHash
	ObservedState     ProviderExecutionObservedState
	PreviewRoute      string
	ArtifactObjectKey string
	SafeErrorCode     string
	SafeErrorMessage  string
}

type CompleteProviderExecutionCleanupInput struct {
	OwnerCAS    ProviderExecutionOwnerCAS
	OperationID string
}

type ListRecoverableProviderExecutionsInput struct {
	SpaceID   string
	ProjectID string
	Limit     int
}

type ProviderExecutionRepository interface {
	EnsureStart(context.Context, EnsureProviderExecutionStartInput) (*ProviderExecution, error)
	Claim(context.Context, ClaimProviderExecutionInput) (*ProviderExecution, error)
	RenewOwner(context.Context, RenewProviderExecutionOwnerInput) (*ProviderExecution, error)
	ReleaseOwner(context.Context, ReleaseProviderExecutionOwnerInput) (*ProviderExecution, error)
	StartSubmission(context.Context, StartProviderSubmissionInput) (*ProviderExecution, error)
	SaveSubmission(context.Context, SaveProviderSubmissionInput) (*ProviderExecution, error)
	ReserveCheckpointWrite(context.Context, ReserveProviderCheckpointWriteInput) (*ProviderExecutionCheckpointReservation, error)
	AbortCheckpointWrite(context.Context, AbortProviderCheckpointWriteInput) (*ProviderExecution, error)
	SaveCheckpoint(context.Context, SaveProviderCheckpointInput) (*ProviderExecution, error)
	SetDesiredStop(context.Context, SetProviderExecutionDesiredStopInput) (*ProviderExecution, error)
	BeginCleanup(context.Context, BeginProviderExecutionCleanupInput) (*ProviderExecution, error)
	AdvanceTerminal(context.Context, AdvanceProviderExecutionTerminalInput) (*ProviderExecution, error)
	CompleteCleanup(context.Context, CompleteProviderExecutionCleanupInput) (*ProviderExecution, error)
	LoadOwnedRecovery(context.Context, ProviderExecutionOwnerCAS) (*ProviderExecution, error)
	ListRecoverable(context.Context, ListRecoverableProviderExecutionsInput) ([]*RecoverableProviderExecution, error)
}

// LoadCurrentProviderExecutionInput identifies the latest unfinished provider
// execution for one tenant-scoped project. Unlike recovery enumeration this
// lookup is deliberately independent of owner lease expiry.
type LoadCurrentProviderExecutionInput struct {
	SpaceID   string
	ProjectID string
}

// ProviderExecutionCurrentRepository is kept separate from recovery scanning:
// request-path orchestration must see an execution owned by another live owner,
// while scanners only enumerate records eligible for takeover.
type ProviderExecutionCurrentRepository interface {
	LoadCurrent(context.Context, LoadCurrentProviderExecutionInput) (*ProviderExecution, error)
}

type LoadReadyProviderArtifactInput struct {
	SpaceID   string
	ProjectID string
}

type ProviderExecutionReadyArtifactRepository interface {
	LoadLatestReadyArtifact(context.Context, LoadReadyProviderArtifactInput) (*ProviderExecution, error)
}
