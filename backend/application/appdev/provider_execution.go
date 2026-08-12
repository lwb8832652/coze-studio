// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	defaultProviderExecutionOwnerLease             = 30 * time.Second
	defaultProviderExecutionCheckpointReservation  = 30 * time.Second
	defaultProviderExecutionCheckpointAbortTimeout = 2 * time.Second
)

var ErrProviderExecutionServiceInvalid = errors.New("appdev provider execution service input is invalid")

type ProviderExecutionCheckpointCodec interface {
	Seal(context.Context, applicationsandbox.ExecutionCheckpointBinding, applicationsandbox.ExecutionCheckpoint) (string, error)
	Open(context.Context, applicationsandbox.ExecutionCheckpointBinding, string) (applicationsandbox.ExecutionCheckpoint, error)
}

type ProviderExecutionService struct {
	repository       domainappdev.ProviderExecutionRepository
	codec            ProviderExecutionCheckpointCodec
	nodeClock        func() time.Time
	random           io.Reader
	ownerLease       time.Duration
	reservationLease time.Duration
}

type ProviderExecutionServiceOption func(*ProviderExecutionService) error

// WithProviderExecutionClock remains a test hook for node-local behavior. Lease
// ownership never consumes this clock; the repository uses its database clock.
func WithProviderExecutionClock(clock func() time.Time) ProviderExecutionServiceOption {
	return func(service *ProviderExecutionService) error {
		if clock == nil {
			return ErrProviderExecutionServiceInvalid
		}
		service.nodeClock = clock
		return nil
	}
}

func WithProviderExecutionRandom(random io.Reader) ProviderExecutionServiceOption {
	return func(service *ProviderExecutionService) error {
		if random == nil {
			return ErrProviderExecutionServiceInvalid
		}
		service.random = random
		return nil
	}
}

func WithProviderExecutionOwnerLease(duration time.Duration) ProviderExecutionServiceOption {
	return func(service *ProviderExecutionService) error {
		if duration <= 0 || duration > time.Hour {
			return ErrProviderExecutionServiceInvalid
		}
		service.ownerLease = duration
		return nil
	}
}

func WithProviderExecutionCheckpointReservation(duration time.Duration) ProviderExecutionServiceOption {
	return func(service *ProviderExecutionService) error {
		if duration <= 0 || duration > time.Hour {
			return ErrProviderExecutionServiceInvalid
		}
		service.reservationLease = duration
		return nil
	}
}

func NewProviderExecutionService(repository domainappdev.ProviderExecutionRepository, codec ProviderExecutionCheckpointCodec, options ...ProviderExecutionServiceOption) (*ProviderExecutionService, error) {
	if repository == nil || codec == nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	service := &ProviderExecutionService{
		repository: repository, codec: codec, nodeClock: func() time.Time { return time.Now().UTC() }, random: rand.Reader,
		ownerLease: defaultProviderExecutionOwnerLease, reservationLease: defaultProviderExecutionCheckpointReservation,
	}
	for _, option := range options {
		if option == nil || option(service) != nil {
			return nil, ErrProviderExecutionServiceInvalid
		}
	}
	return service, nil
}

type EnsureProviderExecutionStartRequest struct {
	SpaceID         string
	ProjectID       string
	ActorUserID     int64
	IdempotencyKey  string
	ProviderKey     string
	ProviderScope   domainsandbox.Scope
	RequireNoActive bool
}

type ClaimProviderExecutionRecoveryRequest struct {
	SpaceID            string
	ProjectID          string
	Generation         uint64
	ExpectedVersion    uint64
	ExistingOwner      domainappdev.ProviderExecutionOwnerToken
	ExistingOwnerEpoch uint64
	ProposedOwner      domainappdev.ProviderExecutionOwnerToken
}

type ProviderExecutionMetadata struct {
	ID                       string
	SpaceID                  string
	ProjectID                string
	ActorUserID              int64
	Generation               uint64
	HasProviderExecution     bool
	HasCheckpoint            bool
	DesiredState             domainappdev.ProviderExecutionDesiredState
	ObservedState            domainappdev.ProviderExecutionObservedState
	ProviderKey              string
	ProviderScope            domainsandbox.Scope
	LaunchState              domainappdev.ProviderExecutionLaunchState
	ProviderLeaseExpiresAt   *time.Time
	OwnerEpoch               uint64
	OwnerExpiresAt           *time.Time
	PreviewRoute             string
	ArtifactStatus           domainappdev.ProviderExecutionArtifactStatus
	ArtifactKind             domainappdev.ProviderExecutionArtifactKind
	ArtifactDigest           string
	ArtifactSize             int64
	ArtifactVersion          uint64
	ArtifactUpdatedAt        *time.Time
	ArtifactSafeErrorCode    string
	ArtifactSafeErrorMessage string
	SafeErrorCode            string
	SafeErrorMessage         string
	Version                  uint64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type ProviderExecutionOwner struct {
	spaceID       string
	projectID     string
	generation    uint64
	providerKey   string
	providerScope domainsandbox.Scope
	token         domainappdev.ProviderExecutionOwnerToken
	epoch         uint64
}

func (ProviderExecutionOwner) String() string   { return "[REDACTED provider execution owner]" }
func (ProviderExecutionOwner) GoString() string { return "[REDACTED provider execution owner]" }
func (ProviderExecutionOwner) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED provider execution owner]")
}
func (ProviderExecutionOwner) MarshalJSON() ([]byte, error) {
	return nil, domainappdev.ErrProviderExecutionSecret
}

type ClaimedProviderExecution struct {
	Metadata ProviderExecutionMetadata
	owner    ProviderExecutionOwner
}

func (c *ClaimedProviderExecution) Owner() ProviderExecutionOwner {
	if c == nil {
		return ProviderExecutionOwner{}
	}
	return c.owner
}

type RenewProviderExecutionOwnerRequest struct {
	Owner           ProviderExecutionOwner
	ExpectedVersion uint64
}

type ReleaseProviderExecutionOwnerRequest struct {
	Owner           ProviderExecutionOwner
	ExpectedVersion uint64
	OperationID     string
}

type StartProviderExecutionSubmissionRequest struct {
	Owner               ProviderExecutionOwner
	ExpectedVersion     uint64
	OperationID         string
	ProviderOperationID string
	RequestDigest       domainappdev.ProviderExecutionLaunchRequestDigest
}

type MarkProviderExecutionLaunchSubmittedRequest struct {
	Owner                 ProviderExecutionOwner
	ExpectedVersion       uint64
	OperationID           string
	DispatchLeaseDuration time.Duration
}

type ClaimProviderExecutionLaunchReconciliationRequest struct {
	SpaceID            string
	ProjectID          string
	Generation         uint64
	ExpectedVersion    uint64
	ProposedOwner      domainappdev.ProviderExecutionOwnerToken
	ExistingOwner      domainappdev.ProviderExecutionOwnerToken
	ExistingOwnerEpoch uint64
	ExpectedState      domainappdev.ProviderExecutionLaunchState
	SetDesiredStop     bool
}

type AbortProviderExecutionLaunchRequest struct {
	Owner           ProviderExecutionOwner
	ExpectedVersion uint64
	OperationID     string
	ExpectedState   domainappdev.ProviderExecutionLaunchState
}

type SaveProviderExecutionSubmissionRequest struct {
	Owner                 ProviderExecutionOwner
	ExpectedVersion       uint64
	LaunchOperationID     string
	ProviderExecutionID   string
	ObservedState         domainappdev.ProviderExecutionObservedState
	ProviderLeaseDuration time.Duration
	PreviewRoute          string
}

type ObserveProviderExecutionActiveRequest struct {
	Owner                 ProviderExecutionOwner
	ExpectedVersion       uint64
	ProviderExecutionID   string
	ProviderLeaseDuration time.Duration
	PreviewRoute          string
	SafeErrorCode         string
	SafeErrorMessage      string
}

type ReserveProviderExecutionBuildRequest struct {
	Owner               ProviderExecutionOwner
	ExpectedVersion     uint64
	ProviderExecutionID string
	OperationID         string
}

type AdvanceProviderExecutionBuildObservationRequest struct {
	Owner               ProviderExecutionOwner
	ExpectedVersion     uint64
	ProviderExecutionID string
	OperationID         string
	Status              domainappdev.ProviderExecutionArtifactStatus
	Descriptor          domainappdev.ProviderBuildArtifactDescriptor
}

type BeginProviderArtifactPublishRequest struct {
	Owner               ProviderExecutionOwner
	ExpectedVersion     uint64
	ProviderExecutionID string
	OperationID         string
}

type CompleteProviderArtifactPublishRequest struct {
	Owner               ProviderExecutionOwner
	ExpectedVersion     uint64
	ProviderExecutionID string
	OperationID         string
	ObjectKey           string
	Digest              string
	Size                int64
}

type FailProviderExecutionBuildRequest struct {
	Owner               ProviderExecutionOwner
	ExpectedVersion     uint64
	ProviderExecutionID string
	OperationID         string
	SafeErrorCode       string
	SafeErrorMessage    string
}

type SaveProviderExecutionCheckpointRequest struct {
	Owner             ProviderExecutionOwner
	ExpectedVersion   uint64
	OperationID       string
	LaunchOperationID string
	ProviderKey       string
	ProviderScope     domainsandbox.Scope
	Checkpoint        applicationsandbox.ExecutionCheckpoint
}

type SetProviderExecutionDesiredStopRequest struct {
	Owner           ProviderExecutionOwner
	ExpectedVersion uint64
	OperationID     string
}

type BeginProviderExecutionCleanupRequest struct {
	Owner           ProviderExecutionOwner
	ExpectedVersion uint64
}

type AdvanceProviderExecutionTerminalRequest struct {
	Owner             ProviderExecutionOwner
	ExpectedVersion   uint64
	OperationID       string
	ObservedState     domainappdev.ProviderExecutionObservedState
	PreviewRoute      string
	ArtifactObjectKey string
	SafeErrorCode     string
	SafeErrorMessage  string
}

type CompleteProviderExecutionCleanupRequest struct {
	Owner           ProviderExecutionOwner
	ExpectedVersion uint64
	OperationID     string
}

type LoadProviderExecutionRecoveryRequest struct {
	Owner           ProviderExecutionOwner
	ExpectedVersion uint64
	ProviderKey     string
	ProviderScope   domainsandbox.Scope
}

type ListRecoverableProviderExecutionsRequest struct {
	SpaceID   string
	ProjectID string
	Limit     int
}

type ProviderExecutionRecovery struct {
	Metadata                  ProviderExecutionMetadata
	providerExecutionID       string
	startOperationID          string
	launchProviderOperationID string
	launchRequestDigest       infrasandbox.ExecutionRequestDigest
	buildOperationID          string
	buildOperationHash        domainappdev.ProviderExecutionOperationHash
	artifactStatus            domainappdev.ProviderExecutionArtifactStatus
	artifactObjectKey         string
	checkpoint                applicationsandbox.ExecutionCheckpoint
	hasCheckpoint             bool
	owner                     ProviderExecutionOwner
}

func (r *ProviderExecutionRecovery) launchIdentity() (string, infrasandbox.ExecutionRequestDigest, bool) {
	if r == nil || r.launchProviderOperationID == "" || r.launchRequestDigest.IsZero() {
		return "", infrasandbox.ExecutionRequestDigest{}, false
	}
	return r.launchProviderOperationID, r.launchRequestDigest, true
}

func (r *ProviderExecutionRecovery) launchLookupIdentity() (string, infrasandbox.ExecutionRequestDigest, bool, bool) {
	if r == nil || r.launchProviderOperationID == "" {
		return "", infrasandbox.ExecutionRequestDigest{}, false, false
	}
	if r.Metadata.LaunchState == domainappdev.ProviderExecutionLaunchLegacySubmitted &&
		r.launchRequestDigest.IsZero() {
		return r.launchProviderOperationID, infrasandbox.ExecutionRequestDigest{}, true, true
	}
	if r.launchRequestDigest.IsZero() {
		return "", infrasandbox.ExecutionRequestDigest{}, false, false
	}
	return r.launchProviderOperationID, r.launchRequestDigest, false, true
}

func (r *ProviderExecutionRecovery) ProviderExecutionID() string {
	if r == nil {
		return ""
	}
	return r.providerExecutionID
}

func (r *ProviderExecutionRecovery) startOperation() (string, bool) {
	if r == nil {
		return "", false
	}
	if _, err := domainappdev.HashProviderExecutionOperationID(r.startOperationID); err != nil {
		return "", false
	}
	return r.startOperationID, true
}

func (r *ProviderExecutionRecovery) buildOperation() (string, domainappdev.ProviderExecutionArtifactStatus, bool) {
	if r == nil || r.artifactStatus == "" || r.artifactStatus == domainappdev.ProviderExecutionArtifactNone ||
		r.buildOperationHash.IsZero() {
		return "", "", false
	}
	cas, err := providerExecutionOwnerCAS(r.owner, r.Metadata.Version)
	if err != nil {
		return "", "", false
	}
	hash, err := domainappdev.HashProviderExecutionBuildOperationID(r.buildOperationID, cas, r.providerExecutionID)
	if err != nil || !hash.Equal(r.buildOperationHash) {
		return "", "", false
	}
	return r.buildOperationID, r.artifactStatus, true
}

func (r *ProviderExecutionRecovery) HasCheckpoint() bool { return r != nil && r.hasCheckpoint }

func (r *ProviderExecutionRecovery) Checkpoint() (applicationsandbox.ExecutionCheckpoint, bool) {
	if r == nil || !r.hasCheckpoint {
		return applicationsandbox.ExecutionCheckpoint{}, false
	}
	return r.checkpoint, true
}

func (r *ProviderExecutionRecovery) Owner() ProviderExecutionOwner {
	if r == nil {
		return ProviderExecutionOwner{}
	}
	return r.owner
}

func (*ProviderExecutionRecovery) String() string   { return "[REDACTED provider execution recovery]" }
func (*ProviderExecutionRecovery) GoString() string { return "[REDACTED provider execution recovery]" }
func (*ProviderExecutionRecovery) MarshalJSON() ([]byte, error) {
	return nil, domainappdev.ErrProviderExecutionSecret
}

func (s *ProviderExecutionService) EnsureStart(ctx context.Context, request EnsureProviderExecutionStartRequest) (*ProviderExecutionMetadata, error) {
	if err := s.available(ctx); err != nil {
		return nil, err
	}
	id, err := newProviderExecutionLedgerID(s.random)
	if err != nil {
		return nil, err
	}
	record, err := s.repository.EnsureStart(ctx, domainappdev.EnsureProviderExecutionStartInput{
		ID: id, SpaceID: request.SpaceID, ProjectID: request.ProjectID, ActorUserID: request.ActorUserID, IdempotencyKey: request.IdempotencyKey,
		ProviderKey: request.ProviderKey, ProviderScope: request.ProviderScope, RequireNoActive: request.RequireNoActive,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) ClaimRecovery(ctx context.Context, request ClaimProviderExecutionRecoveryRequest) (*ClaimedProviderExecution, error) {
	if err := s.available(ctx); err != nil {
		return nil, err
	}
	token := request.ExistingOwner
	if !request.ProposedOwner.IsZero() {
		if !token.IsZero() || request.ExistingOwnerEpoch != 0 {
			return nil, ErrProviderExecutionServiceInvalid
		}
		token = request.ProposedOwner
	} else if token.IsZero() {
		if request.ExistingOwnerEpoch != 0 {
			return nil, ErrProviderExecutionServiceInvalid
		}
		var err error
		token, err = domainappdev.NewProviderExecutionOwnerToken(s.random)
		if err != nil {
			return nil, err
		}
	} else if request.ExistingOwnerEpoch == 0 {
		return nil, ErrProviderExecutionServiceInvalid
	}
	record, err := s.repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, Generation: request.Generation,
		ExpectedVersion: request.ExpectedVersion, OwnerHash: token.OwnerIdentityHash(),
		ExpectedOwnerEpoch: request.ExistingOwnerEpoch, LeaseDuration: s.ownerLease,
	})
	if err != nil {
		return nil, err
	}
	return claimedProviderExecution(record, token), nil
}

func (s *ProviderExecutionService) RenewOwner(ctx context.Context, request RenewProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	record, err := s.repository.RenewOwner(ctx, domainappdev.RenewProviderExecutionOwnerInput{OwnerCAS: cas, LeaseDuration: s.ownerLease})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) ReleaseOwner(ctx context.Context, request ReleaseProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	operationHash, err := domainappdev.HashProviderExecutionReleaseOperationID(request.OperationID, cas.OwnerHash)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	record, err := s.repository.ReleaseOwner(ctx, domainappdev.ReleaseProviderExecutionOwnerInput{OwnerCAS: cas, OperationHash: operationHash})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) StartProviderSubmission(ctx context.Context, request StartProviderExecutionSubmissionRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	operationHash, err := domainappdev.HashProviderExecutionLaunchOperationID(request.OperationID, cas)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	if request.RequestDigest.IsZero() {
		return nil, ErrProviderExecutionServiceInvalid
	}
	if _, err := domainappdev.HashProviderExecutionOperationID(request.ProviderOperationID); err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	record, err := s.repository.StartSubmission(ctx, domainappdev.StartProviderSubmissionInput{
		OwnerCAS: cas, OperationHash: operationHash,
		ProviderOperationID: request.ProviderOperationID, RequestDigest: request.RequestDigest,
		LeaseDuration: s.ownerLease,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) MarkProviderLaunchSubmitted(ctx context.Context, request MarkProviderExecutionLaunchSubmittedRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if request.DispatchLeaseDuration <= 0 {
		return nil, ErrProviderExecutionServiceInvalid
	}
	operationHash, err := domainappdev.HashProviderExecutionLaunchOperationID(request.OperationID, cas)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	repository, ok := s.repository.(domainappdev.ProviderExecutionLaunchRepository)
	if !ok {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	record, err := repository.MarkSubmissionSubmitted(ctx, domainappdev.MarkProviderLaunchSubmittedInput{
		OwnerCAS: cas, OperationHash: operationHash,
		DispatchLeaseDuration: request.DispatchLeaseDuration,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) ClaimLaunchReconciliation(
	ctx context.Context,
	request ClaimProviderExecutionLaunchReconciliationRequest,
) (*ClaimedProviderExecution, error) {
	if err := s.available(ctx); err != nil {
		return nil, err
	}
	token := request.ExistingOwner
	if !request.ProposedOwner.IsZero() {
		if !token.IsZero() || request.ExistingOwnerEpoch != 0 {
			return nil, ErrProviderExecutionServiceInvalid
		}
		token = request.ProposedOwner
	} else if token.IsZero() {
		if request.ExistingOwnerEpoch != 0 {
			return nil, ErrProviderExecutionServiceInvalid
		}
		var err error
		token, err = domainappdev.NewProviderExecutionOwnerToken(s.random)
		if err != nil {
			return nil, err
		}
	} else if request.ExistingOwnerEpoch == 0 {
		return nil, ErrProviderExecutionServiceInvalid
	}
	repository, ok := s.repository.(domainappdev.ProviderExecutionLaunchRepository)
	if !ok {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	record, err := repository.ClaimLaunchReconciliation(
		ctx,
		domainappdev.ClaimProviderLaunchReconciliationInput{
			SpaceID: request.SpaceID, ProjectID: request.ProjectID,
			Generation: request.Generation, ExpectedVersion: request.ExpectedVersion,
			OwnerHash: token.OwnerIdentityHash(), ExpectedOwnerEpoch: request.ExistingOwnerEpoch,
			ExpectedState: request.ExpectedState, LeaseDuration: s.ownerLease,
			SetDesiredStop: request.SetDesiredStop,
		},
	)
	if err != nil {
		return nil, err
	}
	return claimedProviderExecution(record, token), nil
}

func (s *ProviderExecutionService) AbortProviderLaunch(ctx context.Context, request AbortProviderExecutionLaunchRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	operationHash, err := domainappdev.HashProviderExecutionLaunchOperationID(request.OperationID, cas)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	repository, ok := s.repository.(domainappdev.ProviderExecutionLaunchRepository)
	if !ok {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	record, err := repository.AbortLaunch(ctx, domainappdev.AbortProviderLaunchInput{
		OwnerCAS: cas, OperationHash: operationHash, ExpectedState: request.ExpectedState,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) SaveProviderSubmission(ctx context.Context, request SaveProviderExecutionSubmissionRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	operationHash, err := domainappdev.HashProviderExecutionLaunchOperationID(request.LaunchOperationID, cas)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	record, err := s.repository.SaveSubmission(ctx, domainappdev.SaveProviderSubmissionInput{
		OperationHash: operationHash,
		OwnerCAS:      cas, ProviderExecutionID: request.ProviderExecutionID, ObservedState: request.ObservedState,
		ProviderLeaseDuration: request.ProviderLeaseDuration, PreviewRoute: request.PreviewRoute,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

type CompleteReconciledProviderLaunchRequest struct {
	Owner                 ProviderExecutionOwner
	ExpectedVersion       uint64
	LaunchOperationID     string
	CheckpointOperationID string
	ProviderExecutionID   string
	ProviderLeaseDuration time.Duration
	PreviewRoute          string
	Checkpoint            applicationsandbox.ExecutionCheckpoint
}

func (s *ProviderExecutionService) CompleteReconciledProviderLaunch(
	ctx context.Context,
	request CompleteReconciledProviderLaunchRequest,
) (*ProviderExecutionMetadata, error) {
	if err := s.available(ctx); err != nil {
		return nil, err
	}
	cas, err := providerExecutionOwnerCAS(request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	launchHash, err := domainappdev.HashProviderExecutionLaunchOperationID(request.LaunchOperationID, cas)
	if err != nil || !domainappdev.ValidProviderExecutionProviderID(request.ProviderExecutionID) ||
		request.ProviderLeaseDuration <= 0 {
		return nil, ErrProviderExecutionServiceInvalid
	}
	checkpointHash, err := domainappdev.HashProviderExecutionOperationID(request.CheckpointOperationID)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	repository, ok := s.repository.(domainappdev.ProviderExecutionLaunchRepository)
	if !ok {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	reservation, err := s.repository.ReserveCheckpointWrite(ctx, domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: checkpointHash, ReservationDuration: s.reservationLease,
	})
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	if reservation.AlreadyCommitted {
		if reservation.Record == nil || reservation.Record.LaunchState != domainappdev.ProviderExecutionLaunchComplete ||
			reservation.Record.ProviderExecutionID != request.ProviderExecutionID ||
			!reservation.Record.LaunchOperationHash.Equal(launchHash) {
			return nil, domainappdev.ErrProviderExecutionOperationConflict
		}
		metadata := providerExecutionMetadata(reservation.Record)
		return &metadata, nil
	}
	cas.ExpectedVersion = reservation.ReservedVersion
	envelope, err := s.codec.Seal(ctx, providerExecutionCheckpointBinding(request.Owner), request.Checkpoint)
	if err != nil {
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultProviderExecutionCheckpointAbortTimeout)
		_, _ = s.repository.AbortCheckpointWrite(abortCtx, domainappdev.AbortProviderCheckpointWriteInput{
			OwnerCAS: cas, OperationHash: checkpointHash, CheckpointWriteRevision: reservation.Revision,
		})
		cancel()
		return nil, ErrProviderExecutionServiceInvalid
	}
	record, err := repository.CompleteReconciledLaunch(ctx, domainappdev.CompleteReconciledProviderLaunchInput{
		OwnerCAS: cas, LaunchOperationHash: launchHash, CheckpointOperationHash: checkpointHash,
		CheckpointWriteRevision: reservation.Revision, CheckpointEnvelope: envelope,
		ProviderExecutionID: request.ProviderExecutionID, ProviderLeaseDuration: request.ProviderLeaseDuration,
		PreviewRoute: request.PreviewRoute,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) ObserveActive(ctx context.Context, request ObserveProviderExecutionActiveRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	repository, ok := s.repository.(interface {
		ObserveActive(context.Context, domainappdev.ObserveProviderExecutionActiveInput) (*domainappdev.ProviderExecution, error)
	})
	if !ok {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	record, err := repository.ObserveActive(ctx, domainappdev.ObserveProviderExecutionActiveInput{
		OwnerCAS: cas, ProviderExecutionID: request.ProviderExecutionID,
		ProviderLeaseDuration: request.ProviderLeaseDuration, PreviewRoute: request.PreviewRoute,
		SafeErrorCode: request.SafeErrorCode, SafeErrorMessage: request.SafeErrorMessage,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) ReserveBuild(ctx context.Context, request ReserveProviderExecutionBuildRequest) (*ProviderExecutionMetadata, error) {
	cas, repository, operationHash, err := s.providerBuildRequest(ctx, request.Owner, request.ExpectedVersion, request.ProviderExecutionID, request.OperationID)
	if err != nil {
		return nil, err
	}
	record, err := repository.ReserveBuild(ctx, domainappdev.ReserveProviderBuildInput{
		OwnerCAS: cas, ProviderExecutionID: request.ProviderExecutionID, OperationID: request.OperationID, OperationHash: operationHash,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) AdvanceBuildObservation(ctx context.Context, request AdvanceProviderExecutionBuildObservationRequest) (*ProviderExecutionMetadata, error) {
	cas, repository, operationHash, err := s.providerBuildRequest(ctx, request.Owner, request.ExpectedVersion, request.ProviderExecutionID, request.OperationID)
	if err != nil {
		return nil, err
	}
	record, err := repository.AdvanceBuildObservation(ctx, domainappdev.AdvanceProviderBuildObservationInput{
		OwnerCAS: cas, ProviderExecutionID: request.ProviderExecutionID, OperationHash: operationHash,
		Status: request.Status, Descriptor: request.Descriptor,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) BeginArtifactPublish(ctx context.Context, request BeginProviderArtifactPublishRequest) (*ProviderExecutionMetadata, error) {
	cas, repository, operationHash, err := s.providerBuildRequest(ctx, request.Owner, request.ExpectedVersion, request.ProviderExecutionID, request.OperationID)
	if err != nil {
		return nil, err
	}
	record, err := repository.BeginArtifactPublish(ctx, domainappdev.BeginProviderArtifactPublishInput{
		OwnerCAS: cas, ProviderExecutionID: request.ProviderExecutionID, OperationHash: operationHash,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) CompleteArtifactPublish(ctx context.Context, request CompleteProviderArtifactPublishRequest) (*ProviderExecutionMetadata, error) {
	cas, repository, operationHash, err := s.providerBuildRequest(ctx, request.Owner, request.ExpectedVersion, request.ProviderExecutionID, request.OperationID)
	if err != nil {
		return nil, err
	}
	record, err := repository.CompleteArtifactPublish(ctx, domainappdev.CompleteProviderArtifactPublishInput{
		OwnerCAS: cas, ProviderExecutionID: request.ProviderExecutionID, OperationHash: operationHash,
		ObjectKey: request.ObjectKey, Digest: request.Digest, Size: request.Size,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) FailBuild(ctx context.Context, request FailProviderExecutionBuildRequest) (*ProviderExecutionMetadata, error) {
	cas, repository, operationHash, err := s.providerBuildRequest(ctx, request.Owner, request.ExpectedVersion, request.ProviderExecutionID, request.OperationID)
	if err != nil {
		return nil, err
	}
	record, err := repository.FailBuild(ctx, domainappdev.FailProviderBuildInput{
		OwnerCAS: cas, ProviderExecutionID: request.ProviderExecutionID, OperationHash: operationHash,
		SafeErrorCode: request.SafeErrorCode, SafeErrorMessage: request.SafeErrorMessage,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) providerBuildRequest(ctx context.Context, owner ProviderExecutionOwner, expectedVersion uint64, providerExecutionID, operationID string) (domainappdev.ProviderExecutionOwnerCAS, domainappdev.ProviderExecutionBuildRepository, domainappdev.ProviderExecutionOperationHash, error) {
	cas, err := s.ownerCASForRequest(ctx, owner, expectedVersion)
	if err != nil {
		return domainappdev.ProviderExecutionOwnerCAS{}, nil, domainappdev.ProviderExecutionOperationHash{}, err
	}
	operationHash, err := domainappdev.HashProviderExecutionBuildOperationID(operationID, cas, providerExecutionID)
	if err != nil {
		return domainappdev.ProviderExecutionOwnerCAS{}, nil, domainappdev.ProviderExecutionOperationHash{}, ErrProviderExecutionServiceInvalid
	}
	repository, ok := s.repository.(domainappdev.ProviderExecutionBuildRepository)
	if !ok {
		return domainappdev.ProviderExecutionOwnerCAS{}, nil, domainappdev.ProviderExecutionOperationHash{}, domainappdev.ErrProviderExecutionUnavailable
	}
	return cas, repository, operationHash, nil
}

func (s *ProviderExecutionService) BeginCleanup(ctx context.Context, request BeginProviderExecutionCleanupRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	record, err := s.repository.BeginCleanup(ctx, domainappdev.BeginProviderExecutionCleanupInput{OwnerCAS: cas})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) SaveCheckpoint(ctx context.Context, request SaveProviderExecutionCheckpointRequest) (*ProviderExecutionMetadata, error) {
	if err := s.available(ctx); err != nil || request.ProviderKey != request.Owner.providerKey || request.ProviderScope != request.Owner.providerScope {
		return nil, ErrProviderExecutionServiceInvalid
	}
	operationHash, err := domainappdev.HashProviderExecutionOperationID(request.OperationID)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	cas, err := providerExecutionOwnerCAS(request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	var launchOperationHash domainappdev.ProviderExecutionOperationHash
	if request.LaunchOperationID != "" {
		launchOperationHash, err = domainappdev.HashProviderExecutionLaunchOperationID(request.LaunchOperationID, cas)
		if err != nil {
			return nil, ErrProviderExecutionServiceInvalid
		}
	}
	reservation, err := s.repository.ReserveCheckpointWrite(ctx, domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: operationHash, ReservationDuration: s.reservationLease,
	})
	if err != nil {
		return nil, err
	}
	if reservation == nil {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	if reservation.AlreadyCommitted {
		if reservation.Record == nil {
			return nil, domainappdev.ErrProviderExecutionUnavailable
		}
		metadata := providerExecutionMetadata(reservation.Record)
		return &metadata, nil
	}
	cas.ExpectedVersion = reservation.ReservedVersion
	envelope, err := s.codec.Seal(ctx, providerExecutionCheckpointBinding(request.Owner), request.Checkpoint)
	if err != nil {
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultProviderExecutionCheckpointAbortTimeout)
		_, _ = s.repository.AbortCheckpointWrite(abortCtx, domainappdev.AbortProviderCheckpointWriteInput{
			OwnerCAS: cas, OperationHash: operationHash, CheckpointWriteRevision: reservation.Revision,
		})
		cancel()
		return nil, ErrProviderExecutionServiceInvalid
	}
	record, err := s.repository.SaveCheckpoint(ctx, domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: cas, OperationHash: operationHash, LaunchOperationHash: launchOperationHash,
		CheckpointWriteRevision: reservation.Revision, CheckpointEnvelope: envelope,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) SetDesiredStop(ctx context.Context, request SetProviderExecutionDesiredStopRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	record, err := s.repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: cas})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) AdvanceTerminal(ctx context.Context, request AdvanceProviderExecutionTerminalRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	operationHash, err := domainappdev.HashProviderExecutionOperationID(request.OperationID)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	record, err := s.repository.AdvanceTerminal(ctx, domainappdev.AdvanceProviderExecutionTerminalInput{
		OwnerCAS: cas, OperationHash: operationHash, ObservedState: request.ObservedState, PreviewRoute: request.PreviewRoute,
		ArtifactObjectKey: request.ArtifactObjectKey, SafeErrorCode: request.SafeErrorCode, SafeErrorMessage: request.SafeErrorMessage,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) CompleteCleanup(ctx context.Context, request CompleteProviderExecutionCleanupRequest) (*ProviderExecutionMetadata, error) {
	cas, err := s.ownerCASForRequest(ctx, request.Owner, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	_, err = domainappdev.HashProviderExecutionCleanupOperationID(request.OperationID, cas)
	if err != nil {
		return nil, ErrProviderExecutionServiceInvalid
	}
	record, err := s.repository.CompleteCleanup(ctx, domainappdev.CompleteProviderExecutionCleanupInput{OwnerCAS: cas, OperationID: request.OperationID})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func (s *ProviderExecutionService) LoadRecovery(ctx context.Context, request LoadProviderExecutionRecoveryRequest) (*ProviderExecutionRecovery, error) {
	if err := s.available(ctx); err != nil || request.ProviderKey != request.Owner.providerKey || request.ProviderScope != request.Owner.providerScope {
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
	recovery := &ProviderExecutionRecovery{
		Metadata: providerExecutionMetadata(record), providerExecutionID: record.ProviderExecutionID,
		startOperationID: record.IdempotencyKey, buildOperationID: record.BuildOperationID,
		buildOperationHash: record.BuildOperationHash, artifactStatus: record.ArtifactStatus,
		artifactObjectKey: record.ArtifactObjectKey, owner: request.Owner,
	}
	recovery.launchProviderOperationID = record.LaunchProviderOperationID
	if digest, digestErr := infrasandbox.ExecutionRequestDigestFromBytes(record.LaunchRequestDigest.Bytes()); digestErr == nil {
		recovery.launchRequestDigest = digest
	}
	if record.CheckpointEnvelope != "" {
		checkpoint, err := s.codec.Open(ctx, providerExecutionCheckpointBinding(request.Owner), record.CheckpointEnvelope)
		if err != nil {
			return nil, ErrProviderExecutionServiceInvalid
		}
		recovery.checkpoint = checkpoint
		recovery.hasCheckpoint = true
	}
	return recovery, nil
}

func (s *ProviderExecutionService) ListRecoverable(ctx context.Context, request ListRecoverableProviderExecutionsRequest) ([]ProviderExecutionMetadata, error) {
	if err := s.available(ctx); err != nil {
		return nil, err
	}
	if !domainappdev.ValidProviderExecutionSpaceID(request.SpaceID) || !domainappdev.ValidProviderExecutionProjectID(request.ProjectID) {
		return nil, ErrProviderExecutionServiceInvalid
	}
	records, err := s.repository.ListRecoverable(ctx, domainappdev.ListRecoverableProviderExecutionsInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID, Limit: request.Limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]ProviderExecutionMetadata, 0, len(records))
	for _, record := range records {
		items = append(items, recoverableProviderExecutionMetadata(record))
	}
	return items, nil
}

func (s *ProviderExecutionService) ownerCASForRequest(ctx context.Context, owner ProviderExecutionOwner, expectedVersion uint64) (domainappdev.ProviderExecutionOwnerCAS, error) {
	if err := s.available(ctx); err != nil {
		return domainappdev.ProviderExecutionOwnerCAS{}, err
	}
	return providerExecutionOwnerCAS(owner, expectedVersion)
}

func (s *ProviderExecutionService) available(ctx context.Context) error {
	if s == nil || s.repository == nil || s.codec == nil || s.nodeClock == nil || s.random == nil ||
		s.ownerLease <= 0 || s.reservationLease <= 0 || ctx == nil {
		return ErrProviderExecutionServiceInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func providerExecutionOwnerCAS(owner ProviderExecutionOwner, expectedVersion uint64) (domainappdev.ProviderExecutionOwnerCAS, error) {
	cas := domainappdev.ProviderExecutionOwnerCAS{
		SpaceID: owner.spaceID, ProjectID: owner.projectID, Generation: owner.generation,
		ExpectedVersion: expectedVersion, OwnerHash: owner.token.OwnerIdentityHash(), OwnerEpoch: owner.epoch,
		ProviderKey: owner.providerKey, ProviderScope: owner.providerScope,
	}
	if owner.token.IsZero() || domainappdev.ValidateProviderExecutionOwnerCAS(cas) != nil {
		return domainappdev.ProviderExecutionOwnerCAS{}, ErrProviderExecutionServiceInvalid
	}
	return cas, nil
}

func providerExecutionCheckpointBinding(owner ProviderExecutionOwner) applicationsandbox.ExecutionCheckpointBinding {
	return applicationsandbox.ExecutionCheckpointBinding{
		SpaceID: owner.spaceID, ProjectID: owner.projectID, Generation: owner.generation,
		ProviderKey: owner.providerKey, Scope: owner.providerScope,
	}
}

func claimedProviderExecution(record *domainappdev.ProviderExecution, token domainappdev.ProviderExecutionOwnerToken) *ClaimedProviderExecution {
	return &ClaimedProviderExecution{
		Metadata: providerExecutionMetadata(record),
		owner: ProviderExecutionOwner{
			spaceID: record.SpaceID, projectID: record.ProjectID, generation: record.Generation,
			providerKey: record.ProviderKey, providerScope: record.ProviderScope, token: token, epoch: record.OwnerEpoch,
		},
	}
}

// LoadCurrent returns the highest unfinished generation for one tenant-scoped
// project regardless of owner lease expiry. Request-path orchestrators use this
// to distinguish an absent execution from one owned by a live peer.
type LoadCurrentProviderExecutionRequest struct {
	SpaceID   string
	ProjectID string
}

type MatchCurrentProviderExecutionStartRequest struct {
	SpaceID     string
	ProjectID   string
	OperationID string
}

// MatchCurrentStartOperation compares the supplied capability inside the
// application service and never returns the persisted raw operation.
func (s *ProviderExecutionService) MatchCurrentStartOperation(ctx context.Context, request MatchCurrentProviderExecutionStartRequest) (*ProviderExecutionMetadata, bool, error) {
	if err := s.available(ctx); err != nil {
		return nil, false, err
	}
	if !domainappdev.ValidProviderExecutionSpaceID(request.SpaceID) || !domainappdev.ValidProviderExecutionProjectID(request.ProjectID) ||
		!domainappdev.ValidProviderExecutionIdempotencyKey(request.OperationID) {
		return nil, false, ErrProviderExecutionServiceInvalid
	}
	repository, ok := s.repository.(domainappdev.ProviderExecutionCurrentRepository)
	if !ok {
		return nil, false, domainappdev.ErrProviderExecutionUnavailable
	}
	record, err := repository.LoadCurrent(ctx, domainappdev.LoadCurrentProviderExecutionInput{SpaceID: request.SpaceID, ProjectID: request.ProjectID})
	if errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		return nil, false, nil
	}
	if err != nil || record == nil {
		return nil, false, err
	}
	metadata := providerExecutionMetadata(record)
	if len(record.IdempotencyKey) != len(request.OperationID) {
		return &metadata, false, nil
	}
	return &metadata, subtle.ConstantTimeCompare([]byte(record.IdempotencyKey), []byte(request.OperationID)) == 1, nil
}

func (s *ProviderExecutionService) LoadCurrent(ctx context.Context, request LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error) {
	if s == nil || strings.TrimSpace(request.SpaceID) == "" || strings.TrimSpace(request.ProjectID) == "" {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	repository, ok := s.repository.(domainappdev.ProviderExecutionCurrentRepository)
	if !ok {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	record, err := repository.LoadCurrent(ctx, domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID,
	})
	if err != nil {
		return nil, err
	}
	metadata := providerExecutionMetadata(record)
	return &metadata, nil
}

func providerExecutionMetadata(record *domainappdev.ProviderExecution) ProviderExecutionMetadata {
	if record == nil {
		return ProviderExecutionMetadata{}
	}
	return ProviderExecutionMetadata{
		ID: record.ID, SpaceID: record.SpaceID, ProjectID: record.ProjectID, ActorUserID: record.ActorUserID, Generation: record.Generation,
		DesiredState: record.DesiredState, ObservedState: record.ObservedState, ProviderKey: record.ProviderKey,
		ProviderScope: record.ProviderScope, LaunchState: record.LaunchState,
		ProviderLeaseExpiresAt: cloneProviderExecutionTime(record.ProviderLeaseExpiresAt),
		OwnerEpoch:             record.OwnerEpoch, OwnerExpiresAt: cloneProviderExecutionTime(record.OwnerExpiresAt),
		PreviewRoute:   record.PreviewRoute,
		ArtifactStatus: record.ArtifactStatus, ArtifactKind: record.ArtifactKind, ArtifactDigest: record.ArtifactDigest,
		ArtifactSize: record.ArtifactSize, ArtifactVersion: record.ArtifactVersion, ArtifactUpdatedAt: cloneProviderExecutionTime(record.ArtifactUpdatedAt),
		ArtifactSafeErrorCode: record.ArtifactSafeErrorCode, ArtifactSafeErrorMessage: record.ArtifactSafeErrorMessage,
		SafeErrorCode: record.SafeErrorCode, SafeErrorMessage: record.SafeErrorMessage,
		Version: record.Version, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func recoverableProviderExecutionMetadata(record *domainappdev.RecoverableProviderExecution) ProviderExecutionMetadata {
	if record == nil {
		return ProviderExecutionMetadata{}
	}
	return ProviderExecutionMetadata{
		ID: record.ID, SpaceID: record.SpaceID, ProjectID: record.ProjectID, ActorUserID: record.ActorUserID, Generation: record.Generation,
		HasProviderExecution: record.HasProviderExecution, HasCheckpoint: record.HasCheckpoint,
		DesiredState: record.DesiredState, ObservedState: record.ObservedState, ProviderKey: record.ProviderKey,
		ProviderScope: record.ProviderScope, LaunchState: record.LaunchState,
		ProviderLeaseExpiresAt: cloneProviderExecutionTime(record.ProviderLeaseExpiresAt),
		OwnerEpoch:             record.OwnerEpoch, OwnerExpiresAt: cloneProviderExecutionTime(record.OwnerExpiresAt),
		PreviewRoute: record.PreviewRoute, SafeErrorCode: record.SafeErrorCode, SafeErrorMessage: record.SafeErrorMessage,
		Version: record.Version, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func newProviderExecutionLedgerID(random io.Reader) (string, error) {
	if random == nil {
		return "", ErrProviderExecutionServiceInvalid
	}
	buffer := make([]byte, 16)
	if _, err := io.ReadFull(random, buffer); err != nil {
		return "", domainappdev.ErrProviderExecutionUnavailable
	}
	return "apx_" + hex.EncodeToString(buffer), nil
}

func cloneProviderExecutionTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

var _ json.Marshaler = ProviderExecutionOwner{}
var _ json.Marshaler = (*ProviderExecutionRecovery)(nil)
