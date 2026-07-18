// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

var ErrProviderQuarantineDispositionPermissionDenied = errors.New("provider quarantine disposition permission denied")

type ProviderQuarantineDispositionActor struct {
	UserID      int64
	SystemAdmin bool
}

type ProviderQuarantineDispositionRequest struct {
	SpaceID         string
	ProjectID       string
	Generation      uint64
	ExpectedVersion uint64
	ExpectedState   domainappdev.ProviderExecutionLaunchState
	OperationID     string
	Acknowledgement domainappdev.ProviderExecutionQuarantineAcknowledgement
	Reason          string
	EvidenceHash    string
}

type ProviderQuarantineDispositionProjection struct {
	Generation      uint64                                                  `json:"generation"`
	Version         uint64                                                  `json:"version"`
	State           domainappdev.ProviderExecutionLaunchState               `json:"state"`
	Disposition     domainappdev.ProviderExecutionQuarantineAcknowledgement `json:"disposition"`
	CleanupRequired bool                                                    `json:"cleanup_required"`
}

func (projection ProviderQuarantineDispositionProjection) String() string {
	return fmt.Sprintf(
		"ProviderQuarantineDispositionProjection{Generation:%d Version:%d State:%q Disposition:%q CleanupRequired:%t}",
		projection.Generation,
		projection.Version,
		projection.State,
		projection.Disposition,
		projection.CleanupRequired,
	)
}

func (projection ProviderQuarantineDispositionProjection) GoString() string {
	return projection.String()
}

type ProviderQuarantineDispositionService struct {
	repository domainappdev.ProviderExecutionQuarantineRepository
	random     io.Reader
}

func NewProviderQuarantineDispositionService(
	repository domainappdev.ProviderExecutionQuarantineRepository,
	random io.Reader,
) (*ProviderQuarantineDispositionService, error) {
	if repository == nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if random == nil {
		random = rand.Reader
	}
	return &ProviderQuarantineDispositionService{repository: repository, random: random}, nil
}

func (service *ProviderQuarantineDispositionService) Dispose(
	ctx context.Context,
	actor ProviderQuarantineDispositionActor,
	request ProviderQuarantineDispositionRequest,
) (*ProviderQuarantineDispositionProjection, error) {
	if service == nil || service.repository == nil || service.random == nil || ctx == nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if actor.UserID <= 0 || !actor.SystemAdmin {
		return nil, ErrProviderQuarantineDispositionPermissionDenied
	}
	evidenceHash, err := parseProviderQuarantineEvidenceHash(request.EvidenceHash)
	if err != nil || !domainappdev.ValidProviderExecutionSpaceID(request.SpaceID) ||
		!domainappdev.ValidProviderExecutionProjectID(request.ProjectID) ||
		request.Generation == 0 || request.ExpectedVersion == 0 ||
		request.ExpectedState != domainappdev.ProviderExecutionLaunchQuarantined ||
		!domainappdev.ValidProviderExecutionQuarantineDisposition(request.Acknowledgement, request.Reason) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if _, err = domainappdev.HashProviderExecutionOperationID(request.OperationID); err != nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	ownerToken, err := domainappdev.NewProviderExecutionOwnerToken(service.random)
	if err != nil {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	result, err := service.repository.DisposeQuarantine(ctx, domainappdev.DisposeProviderExecutionQuarantineInput{
		SpaceID: request.SpaceID, ProjectID: request.ProjectID,
		Generation: request.Generation, ExpectedVersion: request.ExpectedVersion,
		ExpectedState: request.ExpectedState, OwnerHash: ownerToken.OwnerIdentityHash(),
		ActorID: actor.UserID, OperationID: request.OperationID,
		Acknowledgement: request.Acknowledgement, Reason: request.Reason, EvidenceHash: evidenceHash,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, domainappdev.ErrProviderExecutionInvalid),
			errors.Is(err, domainappdev.ErrProviderExecutionNotFound),
			errors.Is(err, domainappdev.ErrProviderExecutionConflict):
			return nil, err
		default:
			return nil, domainappdev.ErrProviderExecutionUnavailable
		}
	}
	if result == nil || result.Execution == nil || result.Audit == nil ||
		result.Execution.Generation != request.Generation ||
		result.Audit.ActorID != actor.UserID ||
		result.Audit.Acknowledgement != request.Acknowledgement {
		return nil, domainappdev.ErrProviderExecutionUnavailable
	}
	return &ProviderQuarantineDispositionProjection{
		Generation:      result.Execution.Generation,
		Version:         result.Execution.Version,
		State:           result.Execution.LaunchState,
		Disposition:     result.Audit.Acknowledgement,
		CleanupRequired: result.Execution.ObservedState != domainappdev.ProviderExecutionObservedCleanupComplete,
	}, nil
}

func parseProviderQuarantineEvidenceHash(value string) (domainappdev.ProviderExecutionOperationHash, error) {
	if len(value) != 64 {
		return domainappdev.ProviderExecutionOperationHash{}, domainappdev.ErrProviderExecutionInvalid
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return domainappdev.ProviderExecutionOperationHash{}, domainappdev.ErrProviderExecutionInvalid
	}
	var result domainappdev.ProviderExecutionOperationHash
	copy(result[:], decoded)
	if result.IsZero() {
		return domainappdev.ProviderExecutionOperationHash{}, domainappdev.ErrProviderExecutionInvalid
	}
	return result, nil
}
