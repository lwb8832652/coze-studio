// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type providerExecutionQuarantineAuditRecord struct {
	ID                string                                                  `gorm:"column:id;type:varchar(64);primaryKey"`
	ExecutionID       string                                                  `gorm:"column:execution_id;type:varchar(64);not null"`
	SpaceID           int64                                                   `gorm:"column:space_id;type:bigint unsigned;not null;uniqueIndex:uk_appdev_provider_quarantine_audit_operation,priority:1"`
	ProjectID         string                                                  `gorm:"column:project_id;type:varchar(64);not null;uniqueIndex:uk_appdev_provider_quarantine_audit_operation,priority:2"`
	Generation        uint64                                                  `gorm:"column:generation;type:bigint unsigned;not null;uniqueIndex:uk_appdev_provider_quarantine_audit_operation,priority:3"`
	ExpectedVersion   uint64                                                  `gorm:"column:expected_version;type:bigint unsigned;not null"`
	OwnerIdentityHash []byte                                                  `gorm:"column:owner_identity_hash;type:binary(32);not null"`
	OwnerEpoch        uint64                                                  `gorm:"column:owner_epoch;type:bigint unsigned;not null"`
	ActorID           int64                                                   `gorm:"column:actor_id;type:bigint unsigned;not null"`
	OperationHash     []byte                                                  `gorm:"column:operation_hash;type:binary(32);not null;uniqueIndex:uk_appdev_provider_quarantine_audit_operation,priority:4"`
	Acknowledgement   domainappdev.ProviderExecutionQuarantineAcknowledgement `gorm:"column:acknowledgement;type:varchar(32);not null"`
	Reason            string                                                  `gorm:"column:reason;type:varchar(64);not null"`
	EvidenceHash      []byte                                                  `gorm:"column:evidence_hash;type:binary(32);not null"`
	CreatedAt         time.Time                                               `gorm:"column:created_at;type:datetime(6);not null"`
}

func (providerExecutionQuarantineAuditRecord) TableName() string {
	return "appdev_provider_execution_quarantine_audits"
}

func (repository *ProviderExecutionRepository) DisposeQuarantine(
	ctx context.Context,
	input domainappdev.DisposeProviderExecutionQuarantineInput,
) (*domainappdev.ProviderExecutionQuarantineDisposition, error) {
	spaceID, err := parseProviderExecutionSpaceID(input.SpaceID)
	if err != nil || repository == nil || repository.db == nil || repository.clock == nil ||
		!domainappdev.ValidProviderExecutionProjectID(input.ProjectID) ||
		input.Generation == 0 || input.ExpectedVersion == 0 ||
		input.ExpectedState != domainappdev.ProviderExecutionLaunchQuarantined ||
		input.OwnerHash.IsZero() || input.ActorID <= 0 || input.EvidenceHash.IsZero() ||
		!domainappdev.ValidProviderExecutionQuarantineDisposition(input.Acknowledgement, input.Reason) {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}
	if _, hashErr := domainappdev.HashProviderExecutionOperationID(input.OperationID); hashErr != nil {
		return nil, domainappdev.ErrProviderExecutionInvalid
	}

	var disposition *domainappdev.ProviderExecutionQuarantineDisposition
	err = repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record providerExecutionRecord
		if takeErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("space_id = ? AND project_id = ? AND generation = ?", spaceID, input.ProjectID, input.Generation).
			Take(&record).Error; takeErr != nil {
			return normalizeProviderExecutionLookupError(takeErr)
		}
		if _, hydrateErr := providerExecutionRecordToDomain(&record); hydrateErr != nil {
			return hydrateErr
		}
		operationHash, hashErr := domainappdev.HashProviderExecutionQuarantineDispositionOperationID(
			input.OperationID, input.ActorID, input.SpaceID, input.ProjectID, input.Generation,
			record.ProviderKey, record.ProviderScope,
		)
		if hashErr != nil {
			return hashErr
		}
		existing, existingErr := loadProviderExecutionQuarantineAudit(tx, spaceID, input, operationHash)
		if existingErr == nil {
			if idempotencyErr := validateProviderExecutionQuarantineAudit(existing, &record, input, operationHash); idempotencyErr != nil {
				return idempotencyErr
			}
			execution, convertErr := providerExecutionRecordToDomain(&record)
			if convertErr != nil {
				return convertErr
			}
			disposition = &domainappdev.ProviderExecutionQuarantineDisposition{
				Execution: execution,
				Audit:     providerExecutionQuarantineAuditToDomain(existing),
			}
			return nil
		}
		if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		if record.Version != input.ExpectedVersion {
			return domainappdev.ErrProviderExecutionVersionConflict
		}
		if record.LaunchState != input.ExpectedState {
			return domainappdev.ErrProviderExecutionStateConflict
		}
		if record.ProviderExecutionID != "" &&
			input.Acknowledgement != domainappdev.ProviderExecutionQuarantineProviderCleaned {
			return domainappdev.ErrProviderExecutionStateConflict
		}
		if record.ArtifactStatus == domainappdev.ProviderExecutionArtifactPublishing {
			return domainappdev.ErrProviderExecutionStateConflict
		}
		dbNow, clockErr := repository.clock.read(ctx, tx)
		if clockErr != nil {
			return clockErr
		}
		if len(record.OwnerIdentityHash) != 0 && record.OwnerExpiresAt != nil && record.OwnerExpiresAt.After(dbNow) {
			return domainappdev.ErrProviderExecutionOwnerConflict
		}
		if record.OwnerEpoch == math.MaxUint64 {
			return domainappdev.ErrProviderExecutionConflict
		}
		ownerEpoch := record.OwnerEpoch + 1
		audit := &providerExecutionQuarantineAuditRecord{
			ID: hex.EncodeToString(operationHash.Bytes()), ExecutionID: record.ID,
			SpaceID: spaceID, ProjectID: input.ProjectID, Generation: input.Generation,
			ExpectedVersion: input.ExpectedVersion, OwnerIdentityHash: input.OwnerHash.Bytes(),
			OwnerEpoch: ownerEpoch, ActorID: input.ActorID,
			OperationHash: operationHash.Bytes(), Acknowledgement: input.Acknowledgement,
			Reason: input.Reason, EvidenceHash: input.EvidenceHash.Bytes(), CreatedAt: dbNow,
		}
		if createErr := tx.Create(audit).Error; createErr != nil {
			return createErr
		}
		update := tx.Model(&providerExecutionRecord{}).
			Where("space_id = ? AND project_id = ? AND generation = ? AND version = ? AND launch_state = ?",
				spaceID, input.ProjectID, input.Generation, input.ExpectedVersion, input.ExpectedState).
			Where("(owner_identity_hash IS NULL OR owner_expires_at IS NULL OR owner_expires_at <= ?)",
				repository.clock.nowExpression()).
			Updates(map[string]any{
				"desired_state":                   domainappdev.ProviderExecutionDesiredStop,
				"observed_state":                  domainappdev.ProviderExecutionObservedPending,
				"provider_execution_id":           "",
				"submission_started_at":           nil,
				"launch_state":                    domainappdev.ProviderExecutionLaunchAborted,
				"launch_operation_hash":           nil,
				"launch_provider_operation_id":    "",
				"launch_request_digest":           nil,
				"launch_expires_at":               nil,
				"checkpoint_envelope":             "",
				"checkpoint_write_revision":       0,
				"checkpoint_write_pending":        false,
				"checkpoint_write_operation_hash": nil,
				"checkpoint_write_expires_at":     nil,
				"checkpoint_last_operation_hash":  nil,
				"provider_lease_expires_at":       nil,
				"owner_identity_hash":             nil,
				"owner_epoch":                     ownerEpoch,
				"owner_expires_at":                nil,
				"release_owner_operation_hash":    nil,
				"safe_error_code":                 domainappdev.ProviderExecutionQuarantineDisposedCode,
				"safe_error_message":              domainappdev.ProviderExecutionQuarantineDisposedMessage,
				"version":                         gorm.Expr("version + 1"),
				"updated_at":                      repository.clock.nowExpression(),
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return domainappdev.ErrProviderExecutionConflict
		}
		var after providerExecutionRecord
		if takeErr := tx.Where("space_id = ? AND project_id = ? AND generation = ?",
			spaceID, input.ProjectID, input.Generation).Take(&after).Error; takeErr != nil {
			return takeErr
		}
		execution, convertErr := providerExecutionRecordToDomain(&after)
		if convertErr != nil {
			return convertErr
		}
		disposition = &domainappdev.ProviderExecutionQuarantineDisposition{
			Execution: execution,
			Audit:     providerExecutionQuarantineAuditToDomain(audit),
		}
		return nil
	})
	if err != nil {
		return nil, normalizeProviderExecutionDatabaseError(err)
	}
	return disposition, nil
}

func loadProviderExecutionQuarantineAudit(
	tx *gorm.DB,
	spaceID int64,
	input domainappdev.DisposeProviderExecutionQuarantineInput,
	operationHash domainappdev.ProviderExecutionOperationHash,
) (*providerExecutionQuarantineAuditRecord, error) {
	var audit providerExecutionQuarantineAuditRecord
	err := tx.Where(
		"space_id = ? AND project_id = ? AND generation = ? AND operation_hash = ?",
		spaceID, input.ProjectID, input.Generation, operationHash.Bytes(),
	).Take(&audit).Error
	return &audit, err
}

func validateProviderExecutionQuarantineAudit(
	audit *providerExecutionQuarantineAuditRecord,
	record *providerExecutionRecord,
	input domainappdev.DisposeProviderExecutionQuarantineInput,
	operationHash domainappdev.ProviderExecutionOperationHash,
) error {
	if audit == nil || record == nil || audit.ExecutionID != record.ID ||
		audit.ExpectedVersion != input.ExpectedVersion ||
		audit.ActorID != input.ActorID || audit.Acknowledgement != input.Acknowledgement ||
		audit.Reason != input.Reason || !operationHash.EqualBytes(audit.OperationHash) ||
		!input.EvidenceHash.EqualBytes(audit.EvidenceHash) ||
		record.DesiredState != domainappdev.ProviderExecutionDesiredStop ||
		(record.LaunchState != domainappdev.ProviderExecutionLaunchAborted &&
			record.ObservedState != domainappdev.ProviderExecutionObservedCleanupPending &&
			record.ObservedState != domainappdev.ProviderExecutionObservedCleanupComplete) {
		return domainappdev.ErrProviderExecutionOperationConflict
	}
	return nil
}

func providerExecutionQuarantineAuditToDomain(
	record *providerExecutionQuarantineAuditRecord,
) *domainappdev.ProviderExecutionQuarantineAudit {
	if record == nil {
		return nil
	}
	operationHash, _ := providerExecutionOperationHashFromBytes(record.OperationHash)
	evidenceHash, _ := providerExecutionOperationHashFromBytes(record.EvidenceHash)
	return &domainappdev.ProviderExecutionQuarantineAudit{
		ID: record.ID, ExecutionID: record.ExecutionID,
		SpaceID: strconv.FormatInt(record.SpaceID, 10), ProjectID: record.ProjectID,
		Generation: record.Generation, ExpectedVersion: record.ExpectedVersion,
		OwnerEpoch: record.OwnerEpoch, ActorID: record.ActorID,
		OperationHash: operationHash, Acknowledgement: record.Acknowledgement,
		Reason: record.Reason, EvidenceHash: evidenceHash, CreatedAt: record.CreatedAt.UTC(),
	}
}

var _ domainappdev.ProviderExecutionQuarantineRepository = (*ProviderExecutionRepository)(nil)
