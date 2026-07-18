// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	ProviderBuildStateIdle     ProviderBuildState = "idle"
	ProviderBuildStateBuilding ProviderBuildState = "building"
	ProviderBuildStateReady    ProviderBuildState = "ready"
	ProviderBuildStateFailed   ProviderBuildState = "failed"

	providerBuildListLimit = 32
)

var (
	ErrProviderBuildInvalid     = errors.New("appdev provider build input is invalid")
	ErrProviderBuildUnavailable = errors.New("appdev provider build is unavailable")
	ErrProviderBuildConflict    = errors.New("appdev provider build conflicts with current state")
	ErrProviderBuildUnsupported = errors.New("appdev provider build is unsupported")
)

type ProviderBuildState string

type ProviderBuildBeginInput struct {
	SpaceID     string
	ProjectID   string
	OperationID string
}

type ProviderBuildPollInput struct {
	SpaceID     string
	ProjectID   string
	OperationID string
}

type ProviderBuildRecoverInput struct {
	SpaceID   string
	ProjectID string
}

type ProviderBuildProjection struct {
	Generation       uint64             `json:"generation"`
	State            ProviderBuildState `json:"state"`
	ReleaseAvailable bool               `json:"release_available"`
	Size             int64              `json:"size,omitempty"`
	UpdatedAt        time.Time          `json:"updated_at,omitempty"`
	Stale            bool               `json:"stale"`
	SafeErrorCode    string             `json:"safe_error_code,omitempty"`
	SafeMessage      string             `json:"message,omitempty"`
}

func (projection ProviderBuildProjection) String() string {
	return fmt.Sprintf("ProviderBuildProjection{generation:%d,state:%s,release_available:%t,size:%d}",
		projection.Generation, projection.State, projection.ReleaseAvailable, projection.Size)
}

func (projection ProviderBuildProjection) GoString() string { return projection.String() }

func (projection ProviderBuildProjection) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, projection.String())
}

type ProviderExecutionBuildTarget struct {
	Metadata            ProviderExecutionMetadata
	providerExecutionID string
	buildOperationID    string
	artifactStatus      domainappdev.ProviderExecutionArtifactStatus
	artifactObjectKey   string
}

func (*ProviderExecutionBuildTarget) String() string {
	return "ProviderExecutionBuildTarget{internal:<redacted>}"
}

func (*ProviderExecutionBuildTarget) GoString() string {
	return "ProviderExecutionBuildTarget{internal:<redacted>}"
}

func (*ProviderExecutionBuildTarget) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderExecutionBuildTarget{internal:<redacted>}")
}

func (*ProviderExecutionBuildTarget) MarshalJSON() ([]byte, error) {
	return nil, domainappdev.ErrProviderExecutionSecret
}

type LoadProviderExecutionBuildTargetRequest struct {
	Owner           ProviderExecutionOwner
	ExpectedVersion uint64
	ProviderKey     string
	ProviderScope   domainsandbox.Scope
}

// LoadBuildTarget performs the owner-fenced internal read needed before the
// checkpoint is opened. Provider execution and build operation identities stay
// out of public metadata and generic formatting.
func (s *ProviderExecutionService) LoadBuildTarget(
	ctx context.Context,
	request LoadProviderExecutionBuildTargetRequest,
) (*ProviderExecutionBuildTarget, error) {
	if err := s.available(ctx); err != nil || request.ProviderKey != request.Owner.providerKey ||
		request.ProviderScope != request.Owner.providerScope {
		return nil, ErrProviderExecutionServiceInvalid
	}
	cas, err := providerExecutionOwnerCAS(request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	record, err := s.repository.LoadOwnedRecovery(ctx, cas)
	if err != nil {
		return nil, err
	}
	if record == nil || record.ProviderExecutionID == "" || record.ProviderKey != request.ProviderKey ||
		record.ProviderScope != request.ProviderScope {
		return nil, ErrProviderExecutionServiceInvalid
	}
	if record.ArtifactStatus != "" && record.ArtifactStatus != domainappdev.ProviderExecutionArtifactNone {
		hash, hashErr := domainappdev.HashProviderExecutionBuildOperationID(record.BuildOperationID, cas, record.ProviderExecutionID)
		if hashErr != nil || !hash.Equal(record.BuildOperationHash) {
			return nil, ErrProviderExecutionServiceInvalid
		}
	}
	return &ProviderExecutionBuildTarget{
		Metadata: providerExecutionMetadata(record), providerExecutionID: record.ProviderExecutionID,
		buildOperationID: record.BuildOperationID, artifactStatus: record.ArtifactStatus,
		artifactObjectKey: record.ArtifactObjectKey,
	}, nil
}

type ProviderBuildExecutionLedger interface {
	LoadCurrent(context.Context, LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error)
	RenewOwner(context.Context, RenewProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error)
	ClaimRecovery(context.Context, ClaimProviderExecutionRecoveryRequest) (*ClaimedProviderExecution, error)
	LoadBuildTarget(context.Context, LoadProviderExecutionBuildTargetRequest) (*ProviderExecutionBuildTarget, error)
	ReserveBuild(context.Context, ReserveProviderExecutionBuildRequest) (*ProviderExecutionMetadata, error)
	LoadRecovery(context.Context, LoadProviderExecutionRecoveryRequest) (*ProviderExecutionRecovery, error)
	AdvanceBuildObservation(context.Context, AdvanceProviderExecutionBuildObservationRequest) (*ProviderExecutionMetadata, error)
	BeginArtifactPublish(context.Context, BeginProviderArtifactPublishRequest) (*ProviderExecutionMetadata, error)
	CompleteArtifactPublish(context.Context, CompleteProviderArtifactPublishRequest) (*ProviderExecutionMetadata, error)
	FailBuild(context.Context, FailProviderExecutionBuildRequest) (*ProviderExecutionMetadata, error)
}

type ProviderBuildRouter interface {
	Resume(context.Context, applicationsandbox.ExecutionCheckpoint, string) (ProviderRuntimeStatusSelection, error)
}

type ProviderBuildArtifactPublisher interface {
	PublishWithPublisher(context.Context, infrasandbox.ArtifactPublisher, PublishBuildArtifactCommand) (*BuildArtifactReceipt, error)
}

type ProviderBuildOrchestratorConfig struct {
	ProviderKey   string
	ProviderScope domainsandbox.Scope
}

type ProviderBuildOrchestrator struct {
	ledger    ProviderBuildExecutionLedger
	router    ProviderBuildRouter
	publisher ProviderBuildArtifactPublisher
	owner     domainappdev.ProviderExecutionOwnerToken
	config    ProviderBuildOrchestratorConfig
}

func NewProviderBuildOrchestrator(
	ledger ProviderBuildExecutionLedger,
	router ProviderBuildRouter,
	publisher ProviderBuildArtifactPublisher,
	ownerGenerator ProviderRuntimeOwnerCapabilityGenerator,
	config ProviderBuildOrchestratorConfig,
) (*ProviderBuildOrchestrator, error) {
	if ledger == nil || router == nil || publisher == nil || strings.TrimSpace(config.ProviderKey) == "" ||
		config.ProviderScope != domainsandbox.ScopeAppDev {
		return nil, ErrProviderBuildInvalid
	}
	if ownerGenerator == nil {
		ownerGenerator = providerBuildCryptoOwnerGenerator{}
	}
	owner, err := ownerGenerator.GenerateProviderRuntimeOwnerCapability()
	if err != nil || owner.IsZero() {
		return nil, ErrProviderBuildUnavailable
	}
	return &ProviderBuildOrchestrator{ledger: ledger, router: router, publisher: publisher, owner: owner, config: config}, nil
}

func (*ProviderBuildOrchestrator) String() string {
	return "ProviderBuildOrchestrator{capabilities:<redacted>}"
}

func (*ProviderBuildOrchestrator) GoString() string {
	return "ProviderBuildOrchestrator{capabilities:<redacted>}"
}

func (*ProviderBuildOrchestrator) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderBuildOrchestrator{capabilities:<redacted>}")
}

func (*ProviderBuildOrchestrator) MarshalJSON() ([]byte, error) {
	return nil, domainappdev.ErrProviderExecutionSecret
}

type providerBuildCryptoOwnerGenerator struct{}

func (providerBuildCryptoOwnerGenerator) GenerateProviderRuntimeOwnerCapability() (domainappdev.ProviderExecutionOwnerToken, error) {
	return domainappdev.NewProviderExecutionOwnerToken(rand.Reader)
}

type providerBuildMode uint8

const (
	providerBuildModeBegin providerBuildMode = iota + 1
	providerBuildModePoll
	providerBuildModeRecover
)

type providerBuildCommand struct {
	spaceID     string
	projectID   string
	operationID string
	mode        providerBuildMode
}

func (orchestrator *ProviderBuildOrchestrator) BeginBuild(ctx context.Context, input ProviderBuildBeginInput) (*ProviderBuildProjection, error) {
	if !validProviderBuildInput(ctx, input.SpaceID, input.ProjectID, input.OperationID, true) {
		return nil, providerBuildInputError(ctx)
	}
	return orchestrator.run(ctx, providerBuildCommand{spaceID: input.SpaceID, projectID: input.ProjectID, operationID: input.OperationID, mode: providerBuildModeBegin})
}

func (orchestrator *ProviderBuildOrchestrator) PollBuild(ctx context.Context, input ProviderBuildPollInput) (*ProviderBuildProjection, error) {
	if !validProviderBuildInput(ctx, input.SpaceID, input.ProjectID, input.OperationID, true) {
		return nil, providerBuildInputError(ctx)
	}
	return orchestrator.run(ctx, providerBuildCommand{spaceID: input.SpaceID, projectID: input.ProjectID, operationID: input.OperationID, mode: providerBuildModePoll})
}

func (orchestrator *ProviderBuildOrchestrator) RecoverBuild(ctx context.Context, input ProviderBuildRecoverInput) (*ProviderBuildProjection, error) {
	if !validProviderBuildInput(ctx, input.SpaceID, input.ProjectID, "", false) {
		return nil, providerBuildInputError(ctx)
	}
	return orchestrator.run(ctx, providerBuildCommand{spaceID: input.SpaceID, projectID: input.ProjectID, mode: providerBuildModeRecover})
}

func (orchestrator *ProviderBuildOrchestrator) run(ctx context.Context, command providerBuildCommand) (*ProviderBuildProjection, error) {
	metadata, err := orchestrator.loadCurrent(ctx, command.spaceID, command.projectID)
	if err != nil {
		if command.mode == providerBuildModeRecover && errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
			return &ProviderBuildProjection{State: ProviderBuildStateIdle}, nil
		}
		return nil, normalizeProviderBuildError(ctx, err)
	}
	if metadata == nil {
		if command.mode == providerBuildModeRecover {
			return &ProviderBuildProjection{State: ProviderBuildStateIdle}, nil
		}
		return nil, ErrProviderBuildConflict
	}
	finishingPublish := providerBuildRuntimeAllowsPublishFinish(*metadata)
	if !providerBuildRuntimeAllowsWork(*metadata) && !finishingPublish {
		return nil, ErrProviderBuildConflict
	}
	owner, owned, err := orchestrator.acquireOwner(ctx, *metadata)
	if err != nil {
		return nil, err
	}
	target, err := orchestrator.ledger.LoadBuildTarget(ctx, LoadProviderExecutionBuildTargetRequest{
		Owner: owner, ExpectedVersion: owned.Version, ProviderKey: orchestrator.config.ProviderKey,
		ProviderScope: orchestrator.config.ProviderScope,
	})
	if err != nil || target == nil || target.providerExecutionID == "" || target.Metadata.Generation != owned.Generation {
		return nil, normalizeProviderBuildError(ctx, err)
	}
	metadata = &target.Metadata
	status := target.artifactStatus
	if status == "" {
		status = metadata.ArtifactStatus
	}
	if finishingPublish {
		operationID := command.operationID
		if command.mode == providerBuildModeRecover {
			operationID = target.buildOperationID
		}
		if status != domainappdev.ProviderExecutionArtifactPublishing ||
			!validProviderBuildOperationID(operationID) || target.buildOperationID != operationID {
			return nil, ErrProviderBuildConflict
		}
		return orchestrator.publish(ctx, owner, *metadata, target.providerExecutionID, operationID, nil)
	}
	operationID := command.operationID
	if command.mode == providerBuildModeRecover {
		operationID = target.buildOperationID
		if status == domainappdev.ProviderExecutionArtifactNone {
			return providerBuildProjection(*metadata), nil
		}
		if !validProviderBuildOperationID(operationID) {
			return nil, ErrProviderBuildUnavailable
		}
	}
	if status != domainappdev.ProviderExecutionArtifactNone && target.buildOperationID != operationID {
		return nil, ErrProviderBuildConflict
	}
	if status == domainappdev.ProviderExecutionArtifactReady || status == domainappdev.ProviderExecutionArtifactFailed {
		if target.buildOperationID == operationID {
			return providerBuildProjection(*metadata), nil
		}
	}

	if command.mode == providerBuildModeBegin &&
		(status == domainappdev.ProviderExecutionArtifactNone || status == domainappdev.ProviderExecutionArtifactReady || status == domainappdev.ProviderExecutionArtifactFailed) {
		metadata, err = orchestrator.ledger.ReserveBuild(ctx, ReserveProviderExecutionBuildRequest{
			Owner: owner, ExpectedVersion: metadata.Version, ProviderExecutionID: target.providerExecutionID, OperationID: operationID,
		})
		if err != nil || metadata == nil {
			return nil, normalizeProviderBuildError(ctx, err)
		}
		status = metadata.ArtifactStatus
	} else if command.mode == providerBuildModePoll && status == domainappdev.ProviderExecutionArtifactNone {
		return nil, ErrProviderBuildConflict
	}

	if status == domainappdev.ProviderExecutionArtifactReady || status == domainappdev.ProviderExecutionArtifactFailed {
		return providerBuildProjection(*metadata), nil
	}
	if status == domainappdev.ProviderExecutionArtifactDescriptorReady || status == domainappdev.ProviderExecutionArtifactPublishing {
		return orchestrator.publish(ctx, owner, *metadata, target.providerExecutionID, operationID, nil)
	}
	if status != domainappdev.ProviderExecutionArtifactBeginPending && status != domainappdev.ProviderExecutionArtifactBuilding {
		return nil, ErrProviderBuildUnavailable
	}

	recovery, selection, err := orchestrator.resume(ctx, owner, *metadata, target.providerExecutionID)
	if err != nil {
		return nil, err
	}
	executor, ok := selection.(infrasandbox.AppDevBuildExecutor)
	if !ok {
		orchestrator.failDeterministic(ctx, owner, *metadata, target.providerExecutionID, operationID,
			"provider_capability_missing", "build provider capability is unavailable")
		return nil, ErrProviderBuildUnsupported
	}
	var observation infrasandbox.BuildObservation
	if status == domainappdev.ProviderExecutionArtifactBeginPending {
		observation, err = executor.BeginBuild(ctx, recovery.ProviderExecutionID(), operationID)
	} else {
		observation, err = executor.BuildStatus(ctx, recovery.ProviderExecutionID(), operationID)
	}
	if err != nil {
		return nil, normalizeProviderBuildError(ctx, err)
	}
	return orchestrator.applyObservation(ctx, owner, *metadata, target.providerExecutionID, operationID, selection, observation)
}

func (orchestrator *ProviderBuildOrchestrator) loadCurrent(ctx context.Context, spaceID, projectID string) (*ProviderExecutionMetadata, error) {
	current, err := orchestrator.ledger.LoadCurrent(ctx, LoadCurrentProviderExecutionRequest{SpaceID: spaceID, ProjectID: projectID})
	if err != nil {
		if errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
			return nil, err
		}
		return nil, normalizeProviderBuildError(ctx, err)
	}
	if current != nil && (current.SpaceID != spaceID || current.ProjectID != projectID) {
		return nil, ErrProviderBuildUnavailable
	}
	return current, nil
}

func (orchestrator *ProviderBuildOrchestrator) acquireOwner(
	ctx context.Context,
	metadata ProviderExecutionMetadata,
) (ProviderExecutionOwner, ProviderExecutionMetadata, error) {
	if metadata.OwnerEpoch > 0 {
		owner := ProviderExecutionOwner{
			spaceID: metadata.SpaceID, projectID: metadata.ProjectID, generation: metadata.Generation,
			providerKey: metadata.ProviderKey, providerScope: metadata.ProviderScope,
			token: orchestrator.owner, epoch: metadata.OwnerEpoch,
		}
		renewed, err := orchestrator.ledger.RenewOwner(ctx, RenewProviderExecutionOwnerRequest{Owner: owner, ExpectedVersion: metadata.Version})
		if err == nil && renewed != nil {
			return owner, *renewed, nil
		}
		if err == nil {
			return ProviderExecutionOwner{}, ProviderExecutionMetadata{}, ErrProviderBuildUnavailable
		}
		if !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
			return ProviderExecutionOwner{}, ProviderExecutionMetadata{}, normalizeProviderBuildError(ctx, err)
		}
	}
	claimed, err := orchestrator.ledger.ClaimRecovery(ctx, ClaimProviderExecutionRecoveryRequest{
		SpaceID: metadata.SpaceID, ProjectID: metadata.ProjectID, Generation: metadata.Generation,
		ExpectedVersion: metadata.Version, ProposedOwner: orchestrator.owner,
	})
	if err != nil || claimed == nil {
		return ProviderExecutionOwner{}, ProviderExecutionMetadata{}, normalizeProviderBuildError(ctx, err)
	}
	return claimed.Owner(), claimed.Metadata, nil
}

func (orchestrator *ProviderBuildOrchestrator) resume(
	ctx context.Context,
	owner ProviderExecutionOwner,
	metadata ProviderExecutionMetadata,
	providerExecutionID string,
) (*ProviderExecutionRecovery, ProviderRuntimeStatusSelection, error) {
	recovery, err := orchestrator.ledger.LoadRecovery(ctx, LoadProviderExecutionRecoveryRequest{
		Owner: owner, ExpectedVersion: metadata.Version, ProviderKey: orchestrator.config.ProviderKey,
		ProviderScope: orchestrator.config.ProviderScope,
	})
	if err != nil || recovery == nil || recovery.ProviderExecutionID() != providerExecutionID {
		return nil, nil, normalizeProviderBuildError(ctx, err)
	}
	checkpoint, ok := recovery.Checkpoint()
	if !ok {
		return nil, nil, ErrProviderBuildUnavailable
	}
	selection, err := orchestrator.router.Resume(ctx, checkpoint, providerExecutionID)
	if err != nil || selection == nil {
		return nil, nil, normalizeProviderBuildError(ctx, err)
	}
	return recovery, selection, nil
}

func (orchestrator *ProviderBuildOrchestrator) applyObservation(
	ctx context.Context,
	owner ProviderExecutionOwner,
	metadata ProviderExecutionMetadata,
	providerExecutionID string,
	operationID string,
	selection ProviderRuntimeStatusSelection,
	observation infrasandbox.BuildObservation,
) (*ProviderBuildProjection, error) {
	normalized, err := infrasandbox.NormalizeBuildObservation(observation)
	if err != nil {
		orchestrator.failDeterministic(ctx, owner, metadata, providerExecutionID, operationID,
			"provider_contract_violation", "build provider returned an invalid observation")
		return nil, ErrProviderBuildUnavailable
	}
	switch normalized.Status {
	case infrasandbox.BuildStatusAccepted, infrasandbox.BuildStatusRunning:
		updated, updateErr := orchestrator.ledger.AdvanceBuildObservation(ctx, AdvanceProviderExecutionBuildObservationRequest{
			Owner: owner, ExpectedVersion: metadata.Version, ProviderExecutionID: providerExecutionID,
			OperationID: operationID, Status: domainappdev.ProviderExecutionArtifactBuilding,
		})
		if updateErr != nil || updated == nil {
			return nil, normalizeProviderBuildError(ctx, updateErr)
		}
		return providerBuildProjection(*updated), nil
	case infrasandbox.BuildStatusDescriptorReady:
		descriptor := domainDescriptorFromInfra(normalized.Descriptor)
		updated, updateErr := orchestrator.ledger.AdvanceBuildObservation(ctx, AdvanceProviderExecutionBuildObservationRequest{
			Owner: owner, ExpectedVersion: metadata.Version, ProviderExecutionID: providerExecutionID,
			OperationID: operationID, Status: domainappdev.ProviderExecutionArtifactDescriptorReady, Descriptor: descriptor,
		})
		if updateErr != nil || updated == nil {
			return nil, normalizeProviderBuildError(ctx, updateErr)
		}
		return orchestrator.publish(ctx, owner, *updated, providerExecutionID, operationID, selection)
	case infrasandbox.BuildStatusFailed:
		updated, updateErr := orchestrator.ledger.FailBuild(ctx, FailProviderExecutionBuildRequest{
			Owner: owner, ExpectedVersion: metadata.Version, ProviderExecutionID: providerExecutionID,
			OperationID: operationID, SafeErrorCode: normalized.SafeErrorCode, SafeErrorMessage: normalized.SafeErrorMessage,
		})
		if updateErr != nil || updated == nil {
			return nil, normalizeProviderBuildError(ctx, updateErr)
		}
		return providerBuildProjection(*updated), nil
	default:
		return nil, ErrProviderBuildUnavailable
	}
}

func (orchestrator *ProviderBuildOrchestrator) publish(
	ctx context.Context,
	owner ProviderExecutionOwner,
	metadata ProviderExecutionMetadata,
	providerExecutionID string,
	operationID string,
	selection ProviderRuntimeStatusSelection,
) (*ProviderBuildProjection, error) {
	descriptor, err := descriptorFromMetadata(metadata)
	if err != nil {
		return nil, ErrProviderBuildUnavailable
	}
	if metadata.ArtifactStatus == domainappdev.ProviderExecutionArtifactDescriptorReady {
		updated, updateErr := orchestrator.ledger.BeginArtifactPublish(ctx, BeginProviderArtifactPublishRequest{
			Owner: owner, ExpectedVersion: metadata.Version, ProviderExecutionID: providerExecutionID, OperationID: operationID,
		})
		if updateErr != nil || updated == nil {
			return nil, normalizeProviderBuildError(ctx, updateErr)
		}
		metadata = *updated
	}
	if metadata.ArtifactStatus != domainappdev.ProviderExecutionArtifactPublishing {
		return nil, ErrProviderBuildUnavailable
	}
	if selection == nil {
		_, selection, err = orchestrator.resume(ctx, owner, metadata, providerExecutionID)
		if err != nil {
			return nil, err
		}
	}
	provider, ok := selection.(infrasandbox.ArtifactPublisher)
	if !ok {
		orchestrator.failDeterministic(ctx, owner, metadata, providerExecutionID, operationID,
			"provider_capability_missing", "artifact publisher capability is unavailable")
		return nil, ErrProviderBuildUnsupported
	}
	receipt, err := orchestrator.publisher.PublishWithPublisher(ctx, provider, PublishBuildArtifactCommand{
		Audience: domainappdev.ArtifactGrantAudience{
			SpaceID: metadata.SpaceID, ProjectID: metadata.ProjectID, ProviderKey: metadata.ProviderKey,
			ProviderScope: metadata.ProviderScope, Operation: BuildArtifactPublishOperation,
		},
		Generation: metadata.Generation, ProviderExecutionID: providerExecutionID, Descriptor: descriptor,
	})
	if err != nil || receipt == nil || receipt.InternalObjectKey() == "" || receipt.Size() != descriptor.Size ||
		receipt.Digest().String() != descriptor.Digest {
		return nil, normalizeProviderBuildError(ctx, err)
	}
	updated, err := orchestrator.ledger.CompleteArtifactPublish(ctx, CompleteProviderArtifactPublishRequest{
		Owner: owner, ExpectedVersion: metadata.Version, ProviderExecutionID: providerExecutionID,
		OperationID: operationID, ObjectKey: receipt.InternalObjectKey(), Digest: receipt.Digest().String(), Size: receipt.Size(),
	})
	if err != nil || updated == nil {
		if reconciled := orchestrator.reconcileCompletedPublish(ctx, owner, metadata, providerExecutionID, operationID, receipt); reconciled != nil {
			return providerBuildProjection(*reconciled), nil
		}
		if err == nil {
			err = ErrProviderBuildUnavailable
		}
		return nil, normalizeProviderBuildError(ctx, err)
	}
	return providerBuildProjection(*updated), nil
}

func (orchestrator *ProviderBuildOrchestrator) reconcileCompletedPublish(
	ctx context.Context,
	owner ProviderExecutionOwner,
	before ProviderExecutionMetadata,
	providerExecutionID string,
	operationID string,
	receipt *BuildArtifactReceipt,
) *ProviderExecutionMetadata {
	if receipt == nil {
		return nil
	}
	current, err := orchestrator.ledger.LoadCurrent(ctx, LoadCurrentProviderExecutionRequest{SpaceID: before.SpaceID, ProjectID: before.ProjectID})
	if err != nil || current == nil || current.Generation != before.Generation ||
		current.ProviderKey != before.ProviderKey || current.ProviderScope != before.ProviderScope ||
		current.ArtifactStatus != domainappdev.ProviderExecutionArtifactReady ||
		current.ArtifactDigest != receipt.Digest().String() || current.ArtifactSize != receipt.Size() {
		return nil
	}
	target, err := orchestrator.ledger.LoadBuildTarget(ctx, LoadProviderExecutionBuildTargetRequest{
		Owner: owner, ExpectedVersion: current.Version, ProviderKey: current.ProviderKey, ProviderScope: current.ProviderScope,
	})
	if err != nil || target == nil || target.Metadata.Generation != current.Generation ||
		target.providerExecutionID != providerExecutionID || target.buildOperationID != operationID ||
		target.artifactStatus != domainappdev.ProviderExecutionArtifactReady ||
		target.artifactObjectKey != receipt.InternalObjectKey() {
		return nil
	}
	return current
}

func (orchestrator *ProviderBuildOrchestrator) failDeterministic(
	ctx context.Context,
	owner ProviderExecutionOwner,
	metadata ProviderExecutionMetadata,
	providerExecutionID string,
	operationID string,
	code string,
	message string,
) {
	_, _ = orchestrator.ledger.FailBuild(ctx, FailProviderExecutionBuildRequest{
		Owner: owner, ExpectedVersion: metadata.Version, ProviderExecutionID: providerExecutionID,
		OperationID: operationID, SafeErrorCode: code, SafeErrorMessage: message,
	})
}

func providerBuildProjection(metadata ProviderExecutionMetadata) *ProviderBuildProjection {
	projection := &ProviderBuildProjection{Generation: metadata.Generation}
	if metadata.ArtifactUpdatedAt != nil {
		projection.UpdatedAt = metadata.ArtifactUpdatedAt.UTC()
	}
	switch metadata.ArtifactStatus {
	case "", domainappdev.ProviderExecutionArtifactNone:
		projection.State = ProviderBuildStateIdle
	case domainappdev.ProviderExecutionArtifactBuilding,
		domainappdev.ProviderExecutionArtifactBeginPending,
		domainappdev.ProviderExecutionArtifactDescriptorReady,
		domainappdev.ProviderExecutionArtifactPublishing:
		projection.State = ProviderBuildStateBuilding
	case domainappdev.ProviderExecutionArtifactReady:
		projection.State = ProviderBuildStateReady
		projection.ReleaseAvailable = true
		projection.Size = metadata.ArtifactSize
	case domainappdev.ProviderExecutionArtifactFailed:
		projection.State = ProviderBuildStateFailed
		projection.SafeErrorCode, projection.SafeMessage = NormalizeProviderBuildPublicError(metadata.ArtifactSafeErrorCode)
	default:
		projection.State = ProviderBuildStateFailed
		projection.SafeErrorCode, projection.SafeMessage = NormalizeProviderBuildPublicError(infrasandbox.BuildSafeErrorCodeProviderContractViolation)
	}
	return projection
}

func providerBuildRuntimeAllowsWork(metadata ProviderExecutionMetadata) bool {
	return metadata.Generation > 0 && metadata.SpaceID != "" && metadata.ProjectID != "" &&
		metadata.ProviderKey != "" && metadata.ProviderScope == domainsandbox.ScopeAppDev &&
		metadata.DesiredState == domainappdev.ProviderExecutionDesiredRun &&
		metadata.ObservedState == domainappdev.ProviderExecutionObservedRunning
}

func providerBuildRuntimeAllowsPublishFinish(metadata ProviderExecutionMetadata) bool {
	return metadata.Generation > 0 && metadata.SpaceID != "" && metadata.ProjectID != "" &&
		metadata.ProviderKey != "" && metadata.ProviderScope == domainsandbox.ScopeAppDev &&
		metadata.DesiredState == domainappdev.ProviderExecutionDesiredStop &&
		metadata.ObservedState != domainappdev.ProviderExecutionObservedCleanupComplete &&
		metadata.ArtifactStatus == domainappdev.ProviderExecutionArtifactPublishing
}

func descriptorFromMetadata(metadata ProviderExecutionMetadata) (infrasandbox.ArtifactDescriptor, error) {
	domainDescriptor, err := domainappdev.NormalizeProviderBuildArtifactDescriptor(domainappdev.ProviderBuildArtifactDescriptor{
		Kind: metadata.ArtifactKind, Digest: metadata.ArtifactDigest, Size: metadata.ArtifactSize,
	})
	if err != nil {
		return infrasandbox.ArtifactDescriptor{}, err
	}
	return infrasandbox.NormalizeArtifactDescriptor(infrasandbox.ArtifactDescriptor{
		Kind: infrasandbox.ArtifactKind(domainDescriptor.Kind), Digest: domainDescriptor.Digest, Size: domainDescriptor.Size,
	})
}

func domainDescriptorFromInfra(descriptor infrasandbox.ArtifactDescriptor) domainappdev.ProviderBuildArtifactDescriptor {
	return domainappdev.ProviderBuildArtifactDescriptor{
		Kind: domainappdev.ProviderExecutionArtifactKind(descriptor.Kind), Digest: descriptor.Digest, Size: descriptor.Size,
	}
}

func validProviderBuildInput(ctx context.Context, spaceID, projectID, operationID string, requireOperation bool) bool {
	if ctx == nil || ctx.Err() != nil || strings.TrimSpace(spaceID) == "" || strings.TrimSpace(projectID) == "" ||
		len(spaceID) > infrasandbox.MaxIdentifierBytes || len(projectID) > infrasandbox.MaxIdentifierBytes ||
		!utf8.ValidString(spaceID) || !utf8.ValidString(projectID) {
		return false
	}
	if requireOperation && !validProviderBuildOperationID(operationID) {
		return false
	}
	return !requireOperation || operationID != ""
}

func validProviderBuildOperationID(value string) bool {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || index > 0 && strings.ContainsRune("._:-", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func providerBuildInputError(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrProviderBuildInvalid
}

func normalizeProviderBuildError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		return ErrProviderBuildConflict
	}
	if errors.Is(err, ErrBuildArtifactPublisherUnsupported) {
		return ErrProviderBuildUnsupported
	}
	return ErrProviderBuildUnavailable
}

var _ json.Marshaler = (*ProviderBuildOrchestrator)(nil)
