// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"strconv"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	ProviderExecutionQuarantineDisposedCode    = "legacy_launch_disposed"
	ProviderExecutionQuarantineDisposedMessage = "provider launch disposition recorded"

	ProviderExecutionQuarantineReasonProviderAbsent  = "provider_absence_confirmed"
	ProviderExecutionQuarantineReasonProviderCleaned = "provider_cleanup_confirmed"
)

type ProviderExecutionQuarantineAcknowledgement string

const (
	ProviderExecutionQuarantineProviderAbsent  ProviderExecutionQuarantineAcknowledgement = "provider_absent"
	ProviderExecutionQuarantineProviderCleaned ProviderExecutionQuarantineAcknowledgement = "provider_cleaned"
)

func ValidProviderExecutionQuarantineDisposition(
	acknowledgement ProviderExecutionQuarantineAcknowledgement,
	reason string,
) bool {
	switch acknowledgement {
	case ProviderExecutionQuarantineProviderAbsent:
		return reason == ProviderExecutionQuarantineReasonProviderAbsent
	case ProviderExecutionQuarantineProviderCleaned:
		return reason == ProviderExecutionQuarantineReasonProviderCleaned
	default:
		return false
	}
}

func HashProviderExecutionQuarantineDispositionOperationID(
	operationID string,
	actorID int64,
	spaceID string,
	projectID string,
	generation uint64,
	providerKey string,
	providerScope domainsandbox.Scope,
) (ProviderExecutionOperationHash, error) {
	if !validProviderExecutionOpaque(operationID, 128) || actorID <= 0 ||
		!ValidProviderExecutionSpaceID(spaceID) || !ValidProviderExecutionProjectID(projectID) ||
		generation == 0 || domainsandbox.ValidateProviderKey(providerKey) != nil ||
		providerScope != domainsandbox.ScopeAppDev {
		return ProviderExecutionOperationHash{}, ErrProviderExecutionInvalid
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("appdev_provider_execution_quarantine_disposition:v1\x00"))
	var actor [8]byte
	binary.BigEndian.PutUint64(actor[:], uint64(actorID))
	_, _ = hasher.Write(actor[:])
	_, _ = hasher.Write([]byte(spaceID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(projectID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(strconv.FormatUint(generation, 10)))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(providerKey))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(providerScope))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(operationID))
	var result ProviderExecutionOperationHash
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

type DisposeProviderExecutionQuarantineInput struct {
	SpaceID         string
	ProjectID       string
	Generation      uint64
	ExpectedVersion uint64
	ExpectedState   ProviderExecutionLaunchState
	OwnerHash       ProviderExecutionOwnerHash
	ActorID         int64
	OperationID     string
	Acknowledgement ProviderExecutionQuarantineAcknowledgement
	Reason          string
	EvidenceHash    ProviderExecutionOperationHash
}

type ProviderExecutionQuarantineAudit struct {
	ID              string
	ExecutionID     string
	SpaceID         string
	ProjectID       string
	Generation      uint64
	ExpectedVersion uint64
	OwnerEpoch      uint64
	ActorID         int64
	OperationHash   ProviderExecutionOperationHash
	Acknowledgement ProviderExecutionQuarantineAcknowledgement
	Reason          string
	EvidenceHash    ProviderExecutionOperationHash
	CreatedAt       time.Time
}

func (ProviderExecutionQuarantineAudit) String() string {
	return "ProviderExecutionQuarantineAudit{metadata:<redacted>}"
}

func (ProviderExecutionQuarantineAudit) GoString() string {
	return "ProviderExecutionQuarantineAudit{metadata:<redacted>}"
}

type ProviderExecutionQuarantineDisposition struct {
	Execution *ProviderExecution
	Audit     *ProviderExecutionQuarantineAudit
}

type ProviderExecutionQuarantineRepository interface {
	DisposeQuarantine(
		context.Context,
		DisposeProviderExecutionQuarantineInput,
	) (*ProviderExecutionQuarantineDisposition, error)
}
