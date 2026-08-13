// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm"
)

var _ domainsandbox.SessionSettingsAuditRepository = (*MySQLRepository)(nil)

func (r *MySQLRepository) AppendSessionSettingsAuditEvent(ctx context.Context, input domainsandbox.AppendSchedulerAuditEventInput) (*domainsandbox.SchedulerAuditEvent, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeAppendSessionSettingsAuditEventInput(input)
	if err != nil {
		return nil, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	return appendSessionSettingsAuditEvent(db, normalized)
}

func (r *MySQLRepository) UpdateSessionSettingsCASWithAudit(ctx context.Context, input domainsandbox.UpdateSessionSettingsInput, audit domainsandbox.AppendSchedulerAuditEventInput) (domainsandbox.SessionRuntimeSettings, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	defer release()
	normalizedAudit, err := domainsandbox.NormalizeAppendSessionSettingsAuditEventInput(audit)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	var result domainsandbox.SessionRuntimeSettings
	err = db.Transaction(func(tx *gorm.DB) error {
		previous, updated, updateErr := updateSessionSettingsCASSnapshot(tx, input)
		if updateErr != nil {
			return updateErr
		}
		if !sessionSettingsAuditDescribesUpdate(input, previous, updated, normalizedAudit) {
			return domainsandbox.ErrInvalidInput
		}
		if _, auditErr := appendSessionSettingsAuditEvent(tx, normalizedAudit); auditErr != nil {
			return auditErr
		}
		result = updated
		return nil
	})
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}, err
	}
	return result, nil
}

func sessionSettingsAuditDescribesUpdate(input domainsandbox.UpdateSessionSettingsInput, previous, updated domainsandbox.SessionRuntimeSettings, audit domainsandbox.AppendSchedulerAuditEventInput) bool {
	changed := domainsandbox.SessionSettingsChangedFields(previous, updated)
	return audit.Action == domainsandbox.SchedulerAuditActionUpdate &&
		audit.ActorUserID == input.UpdatedBy &&
		len(changed) > 0 &&
		audit.Metadata[domainsandbox.SchedulerAuditMetadataPreviousVersion] == strconv.FormatUint(previous.Version, 10) &&
		audit.Metadata[domainsandbox.SchedulerAuditMetadataNewVersion] == strconv.FormatUint(updated.Version, 10) &&
		audit.Metadata[domainsandbox.SchedulerAuditMetadataChangedFields] == strings.Join(changed, ",")
}

func appendSessionSettingsAuditEvent(db *gorm.DB, input domainsandbox.AppendSchedulerAuditEventInput) (*domainsandbox.SchedulerAuditEvent, error) {
	metadataJSON, err := marshalCanonicalJSON(input.Metadata)
	if err != nil {
		return nil, err
	}
	actorUserID, err := positiveDomainInt64ToUint64(input.ActorUserID)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	po := &schedulerAuditEventPO{
		ActorUserID: actorUserID, RequestID: input.RequestID, Action: string(input.Action),
		MetadataJSON: metadataJSON, CreatedAt: persistenceNow(),
	}
	if err := db.Create(po).Error; err != nil {
		return nil, err
	}
	return sessionSettingsAuditEventDomain(po)
}

func sessionSettingsAuditEventDomain(po *schedulerAuditEventPO) (*domainsandbox.SchedulerAuditEvent, error) {
	if po == nil || po.EventID == 0 || po.EventID > math.MaxInt64 || po.ActorUserID == 0 || po.ActorUserID > math.MaxInt64 {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	metadata := make(map[string]string)
	if err := json.Unmarshal([]byte(po.MetadataJSON), &metadata); err != nil || metadata == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	normalized, err := domainsandbox.NormalizeAppendSessionSettingsAuditEventInput(domainsandbox.AppendSchedulerAuditEventInput{
		ActorUserID: int64(po.ActorUserID), RequestID: po.RequestID,
		Action: domainsandbox.SchedulerAuditAction(po.Action), Metadata: metadata,
	})
	if err != nil {
		return nil, errors.Join(domainsandbox.ErrConfigurationInvalid, err)
	}
	return &domainsandbox.SchedulerAuditEvent{
		ID: int64(po.EventID), ActorUserID: normalized.ActorUserID, RequestID: normalized.RequestID,
		Action: normalized.Action, Metadata: normalized.Metadata, CreatedAt: po.CreatedAt.UTC(),
	}, nil
}
