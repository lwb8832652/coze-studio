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
	"errors"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm"
)

func (r *MySQLRepository) GetProviderDefault(
	ctx context.Context,
	scope domainsandbox.Scope,
) (*domainsandbox.ProviderDefault, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	if err := validateDefaultScope(scope); err != nil {
		return nil, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	po, err := findProviderDefault(db, scope, false)
	if err != nil {
		return nil, err
	}
	if po == nil {
		return nil, domainsandbox.ErrDefaultMissing
	}
	return po.toDomain()
}

func (r *MySQLRepository) ListProviderDefaults(ctx context.Context) ([]*domainsandbox.ProviderDefault, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	pos := make([]providerDefaultPO, 0)
	if err := db.Order("scope ASC").Find(&pos).Error; err != nil {
		return nil, err
	}
	defaults := make([]*domainsandbox.ProviderDefault, 0, len(pos))
	for index := range pos {
		item, err := pos[index].toDomain()
		if err != nil {
			return nil, err
		}
		defaults = append(defaults, item)
	}
	return defaults, nil
}

func (r *MySQLRepository) SetProviderDefault(
	ctx context.Context,
	input domainsandbox.SetProviderDefaultInput,
) (*domainsandbox.ProviderDefault, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	if _, _, err := domainsandbox.NormalizeSetProviderDefaultInput(input, nil); err != nil &&
		!errors.Is(err, domainsandbox.ErrDefaultMissing) {
		return nil, err
	}
	providerID, err := positiveDomainInt64ToUint64(input.ProviderID)
	if err != nil {
		return nil, err
	}
	actorUserID, err := positiveDomainInt64ToUint64(input.ActorUserID)
	if err != nil {
		return nil, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	var saved *domainsandbox.ProviderDefault
	err = db.Transaction(func(tx *gorm.DB) error {
		observed, err := findProviderDefault(tx, input.Scope, false)
		if err != nil {
			return err
		}
		observedDomain, err := domainDefault(observed)
		if err != nil {
			return err
		}
		if _, _, err := domainsandbox.NormalizeSetProviderDefaultInput(input, observedDomain); err != nil {
			return err
		}
		if _, err := findLiveProvider(tx, input.ProviderID, true); err != nil {
			return err
		}
		current, err := findProviderDefault(tx, input.Scope, true)
		if err != nil {
			return err
		}
		currentDomain, err := domainDefault(current)
		if err != nil {
			return err
		}
		normalized, nextVersion, err := domainsandbox.NormalizeSetProviderDefaultInput(input, currentDomain)
		if err != nil {
			return err
		}
		now := persistenceNow()
		if current == nil {
			created := &providerDefaultPO{
				Scope:      string(normalized.Scope),
				ProviderID: providerID,
				Version:    nextVersion,
				UpdatedBy:  actorUserID,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := tx.Create(created).Error; err != nil {
				if isDuplicateDatabaseError(err) {
					return domainsandbox.ErrVersionConflict
				}
				return err
			}
			saved, err = created.toDomain()
			return err
		}
		result := tx.Model(&providerDefaultPO{}).
			Where("scope = ? AND version = ?", string(normalized.Scope), normalized.ExpectedVersion).
			Updates(map[string]any{
				"provider_id": providerID,
				"version":     gorm.Expr("version + 1"),
				"updated_by":  actorUserID,
				"updated_at":  now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			latest, err := findProviderDefault(tx, normalized.Scope, false)
			if err != nil {
				return err
			}
			if latest == nil {
				return domainsandbox.ErrDefaultMissing
			}
			return domainsandbox.ErrVersionConflict
		}
		current.ProviderID = providerID
		current.Version = nextVersion
		current.UpdatedBy = actorUserID
		current.UpdatedAt = now
		saved, err = current.toDomain()
		return err
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}
