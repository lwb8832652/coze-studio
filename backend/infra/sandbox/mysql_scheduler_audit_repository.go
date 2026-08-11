// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm"
)

func (r *MySQLRepository) AppendSchedulerAuditEvent(ctx context.Context, input domainsandbox.AppendSchedulerAuditEventInput) (*domainsandbox.SchedulerAuditEvent, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeAppendSchedulerAuditEventInput(input)
	if err != nil {
		return nil, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	return appendSchedulerAuditEvent(db, normalized)
}

func appendSchedulerAuditEvent(db *gorm.DB, input domainsandbox.AppendSchedulerAuditEventInput) (*domainsandbox.SchedulerAuditEvent, error) {
	metadataJSON, err := marshalSchedulerAuditMetadata(input.Metadata)
	if err != nil {
		return nil, err
	}
	actorUserID, err := positiveDomainInt64ToUint64(input.ActorUserID)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	po := &schedulerAuditEventPO{ActorUserID: actorUserID, RequestID: input.RequestID, Action: string(input.Action), MetadataJSON: metadataJSON, CreatedAt: persistenceNow()}
	if err := db.Create(po).Error; err != nil {
		return nil, err
	}
	return po.toDomain()
}
