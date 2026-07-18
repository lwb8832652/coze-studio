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

package sandbox

import (
	"context"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm"
)

func (r *MySQLRepository) AppendProviderAuditEvent(
	ctx context.Context,
	input domainsandbox.AppendProviderAuditEventInput,
) (*domainsandbox.ProviderAuditEvent, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeAppendProviderAuditEventInput(input)
	if err != nil {
		return nil, err
	}
	metadataJSON, err := marshalAuditMetadata(normalized.Metadata)
	if err != nil {
		return nil, err
	}
	providerID, err := positiveDomainInt64ToUint64(normalized.ProviderID)
	if err != nil {
		return nil, err
	}
	actorUserID, err := positiveDomainInt64ToUint64(normalized.ActorUserID)
	if err != nil {
		return nil, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	po := &providerAuditEventPO{
		ProviderID:   &providerID,
		ActorUserID:  actorUserID,
		Action:       normalized.Action,
		Result:       normalized.Result,
		RequestID:    normalized.RequestID,
		MetadataJSON: metadataJSON,
		CreatedAt:    persistenceNow(),
	}
	if err := db.Create(po).Error; err != nil {
		return nil, err
	}
	return po.toDomain()
}

func (r *MySQLRepository) ListProviderAuditEvents(
	ctx context.Context,
	request domainsandbox.ProviderAuditListRequest,
) ([]*domainsandbox.ProviderAuditEvent, int64, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, 0, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeProviderAuditListRequest(request)
	if err != nil {
		return nil, 0, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	pos := make([]providerAuditEventPO, 0, normalized.Limit)
	// Count and page rows share one database snapshot. Offset pages from
	// separate requests remain eventually consistent when intervening writes occur.
	err = r.withConsistentRead(db, func(snapshot *gorm.DB) error {
		query := snapshot.Model(&providerAuditEventPO{})
		if normalized.ProviderID > 0 {
			providerID, err := positiveDomainInt64ToUint64(normalized.ProviderID)
			if err != nil {
				return err
			}
			query = query.Where("provider_id = ?", providerID)
		}
		if normalized.Action != "" {
			query = query.Where("action = ?", normalized.Action)
		}
		if normalized.Result != "" {
			query = query.Where("result = ?", normalized.Result)
		}
		if err := query.Count(&total).Error; err != nil {
			return err
		}
		return query.Order("created_at DESC, event_id DESC").Offset(normalized.Offset).Limit(normalized.Limit).Find(&pos).Error
	})
	if err != nil {
		return nil, 0, err
	}
	events := make([]*domainsandbox.ProviderAuditEvent, 0, len(pos))
	for index := range pos {
		event, err := pos[index].toDomain()
		if err != nil {
			return nil, 0, err
		}
		events = append(events, event)
	}
	return events, total, nil
}
