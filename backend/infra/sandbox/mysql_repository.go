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
	"time"

	"gorm.io/gorm"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func (r *MySQLRepository) CreateProvider(
	ctx context.Context,
	input domainsandbox.CreateProviderInput,
) (*domainsandbox.Provider, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeCreateProviderInput(input)
	if err != nil {
		return nil, err
	}
	if err := validateProviderPersistenceFields(
		normalized.Type,
		normalized.EndpointSecret,
		normalized.EndpointHint,
		normalized.CredentialSecret,
		normalized.CredentialFingerprint,
	); err != nil {
		return nil, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	po, err := newProviderPO(normalized, persistenceNow())
	if err != nil {
		return nil, err
	}
	if err := secretWriteDB(db).Create(po).Error; err != nil {
		if isDuplicateDatabaseError(err) {
			return nil, domainsandbox.ErrProviderAlreadyExists
		}
		return nil, err
	}
	return po.toDomain()
}

func (r *MySQLRepository) GetProvider(ctx context.Context, providerID int64) (*domainsandbox.Provider, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	po, err := findLiveProvider(db, providerID, false)
	if err != nil {
		return nil, err
	}
	return po.toDomain()
}

func (r *MySQLRepository) GetProviderForUpdate(ctx context.Context, providerID int64) (*domainsandbox.Provider, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	if r.txState == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	po, err := findLiveProvider(db, providerID, true)
	if err != nil {
		return nil, err
	}
	return po.toDomain()
}

func (r *MySQLRepository) GetProviderByKey(ctx context.Context, providerKey string) (*domainsandbox.Provider, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	var po providerPO
	err = db.Where("provider_key = ? AND deleted_at IS NULL", providerKey).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainsandbox.ErrProviderNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain()
}

func (r *MySQLRepository) ListProviders(
	ctx context.Context,
	request domainsandbox.ProviderListRequest,
) ([]*domainsandbox.Provider, int64, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, 0, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeProviderListRequest(request)
	if err != nil {
		return nil, 0, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	pos := make([]providerPO, 0, normalized.Limit)
	// Count and page rows share one database snapshot. Offset pages from
	// separate requests remain eventually consistent when intervening writes occur.
	err = r.withConsistentRead(db, func(snapshot *gorm.DB) error {
		query := snapshot.Model(&providerPO{}).Where("deleted_at IS NULL")
		if normalized.Keyword != "" {
			keyword := "%" + escapeLikeKeyword(normalized.Keyword) + "%"
			query = query.Where("(provider_key LIKE ? ESCAPE '!' OR name LIKE ? ESCAPE '!')", keyword, keyword)
		}
		if normalized.Type != "" {
			query = query.Where("provider_type = ?", string(normalized.Type))
		}
		if normalized.Status != "" {
			query = query.Where("status = ?", string(normalized.Status))
		}
		if normalized.Health != "" {
			query = query.Where("health_status = ?", string(normalized.Health))
		}
		if normalized.Scope != "" {
			condition, arguments, scopeErr := providerRequestedScopeSQL(snapshot.Dialector.Name(), normalized.Scope)
			if scopeErr != nil {
				return scopeErr
			}
			query = query.Where(condition, arguments...)
		}
		if len(normalized.AuthorizedScopes) != 0 {
			condition, arguments, scopeErr := providerScopeSubsetSQL(snapshot.Dialector.Name(), normalized.AuthorizedScopes)
			if scopeErr != nil {
				return scopeErr
			}
			query = query.Where(condition, arguments...)
		}
		if err := query.Count(&total).Error; err != nil {
			return err
		}
		return query.Order("created_at DESC, id DESC").Offset(normalized.Offset).Limit(normalized.Limit).Find(&pos).Error
	})
	if err != nil {
		return nil, 0, err
	}
	providers := make([]*domainsandbox.Provider, 0, len(pos))
	for index := range pos {
		provider, err := pos[index].toDomain()
		if err != nil {
			return nil, 0, err
		}
		providers = append(providers, provider)
	}
	return providers, total, nil
}

func providerRequestedScopeSQL(
	dialect string,
	requestedScope domainsandbox.Scope,
) (string, []any, error) {
	normalized, err := domainsandbox.NormalizeScopes([]domainsandbox.Scope{requestedScope})
	if err != nil {
		return "", nil, err
	}
	value := string(normalized[0])
	if dialect == "mysql" {
		return "JSON_CONTAINS(scopes_json, JSON_QUOTE(?))", []any{value}, nil
	}
	return "EXISTS (SELECT 1 FROM json_each(sandbox_providers.scopes_json) WHERE json_each.value = ?)", []any{value}, nil
}

func providerScopeSubsetSQL(
	dialect string,
	allowedScopes []domainsandbox.Scope,
) (string, []any, error) {
	normalized, err := domainsandbox.NormalizeScopes(allowedScopes)
	if err != nil {
		return "", nil, err
	}
	if dialect == "mysql" {
		allowedScopesJSON, err := marshalScopes(normalized)
		if err != nil {
			return "", nil, err
		}
		return "JSON_CONTAINS(?, scopes_json)", []any{allowedScopesJSON}, nil
	}
	values := make([]string, len(normalized))
	for index, scope := range normalized {
		values[index] = string(scope)
	}
	return "NOT EXISTS (SELECT 1 FROM json_each(sandbox_providers.scopes_json) WHERE json_each.value NOT IN ?)", []any{values}, nil
}

func (r *MySQLRepository) UpdateProvider(
	ctx context.Context,
	input domainsandbox.UpdateProviderInput,
) (*domainsandbox.Provider, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return nil, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeUpdateProviderInput(input)
	if err != nil {
		return nil, err
	}
	if err := validateProviderPersistenceFields(
		normalized.Type,
		normalized.EndpointSecret,
		normalized.EndpointHint,
		normalized.CredentialSecret,
		normalized.CredentialFingerprint,
	); err != nil {
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
	maxConcurrency, err := providerMaxConcurrencyToPO(normalized.Policy.MaxConcurrency)
	if err != nil {
		return nil, err
	}
	scopesJSON, err := marshalScopes(normalized.Scopes)
	if err != nil {
		return nil, err
	}
	policyJSON, err := marshalRuntimePolicy(normalized.Policy)
	if err != nil {
		return nil, err
	}
	nextVersion, err := domainsandbox.NextVersion(normalized.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return nil, err
	}
	var updated *domainsandbox.Provider
	err = db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"name":                   normalized.Name,
			"provider_type":          string(normalized.Type),
			"endpoint_secret":        nullableString(normalized.EndpointSecret),
			"endpoint_hint":          normalized.EndpointHint,
			"credential_secret":      nullableString(normalized.CredentialSecret),
			"credential_fingerprint": normalized.CredentialFingerprint,
			"scopes_json":            scopesJSON,
			"policy_json":            policyJSON,
			"max_concurrency":        maxConcurrency,
			"updated_by":             actorUserID,
			"updated_at":             persistenceNow(),
			"version":                gorm.Expr("version + 1"),
		}
		if normalized.ResetHealth {
			updates["health_status"] = string(domainsandbox.HealthStatusUnknown)
			updates["last_health_capabilities_json"] = "[]"
			updates["last_health_code"] = ""
			updates["last_health_message"] = ""
			updates["last_health_latency_ms"] = 0
			updates["last_health_at"] = nil
		}
		result := secretWriteDB(tx).Model(&providerPO{}).
			Where("id = ? AND version = ? AND deleted_at IS NULL", providerID, normalized.ExpectedVersion).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return providerCASFailure(tx, normalized.ProviderID)
		}
		po, err := findLiveProvider(tx, normalized.ProviderID, false)
		if err != nil {
			return err
		}
		updated, err = po.toDomain()
		return err
	})
	if err != nil {
		return nil, err
	}
	if updated.Version != nextVersion {
		return nil, domainsandbox.ErrVersionConflict
	}
	return updated, nil
}

func (r *MySQLRepository) UpdateProviderStatus(
	ctx context.Context,
	input domainsandbox.UpdateProviderStatusInput,
) (uint64, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return 0, err
	}
	defer release()
	normalized, err := domainsandbox.NormalizeUpdateProviderStatusInput(input)
	if err != nil {
		return 0, err
	}
	nextVersion, err := domainsandbox.NextVersion(normalized.ExpectedVersion)
	if err != nil {
		return 0, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return 0, err
	}
	providerID, err := positiveDomainInt64ToUint64(normalized.ProviderID)
	if err != nil {
		return 0, err
	}
	actorUserID, err := positiveDomainInt64ToUint64(normalized.ActorUserID)
	if err != nil {
		return 0, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		provider, err := findLiveProvider(tx, normalized.ProviderID, true)
		if err != nil {
			return err
		}
		if provider.Version != normalized.ExpectedVersion {
			return domainsandbox.ErrVersionConflict
		}
		result := tx.Model(&providerPO{}).
			Where("id = ? AND version = ? AND deleted_at IS NULL", providerID, normalized.ExpectedVersion).
			Updates(map[string]any{
				"status":     string(normalized.Status),
				"updated_by": actorUserID,
				"updated_at": persistenceNow(),
				"version":    gorm.Expr("version + 1"),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainsandbox.ErrVersionConflict
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return nextVersion, nil
}

func (r *MySQLRepository) UpdateProviderHealth(
	ctx context.Context,
	input domainsandbox.UpdateProviderHealthInput,
) (uint64, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return 0, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return 0, err
	}
	var nextVersion uint64
	err = db.Transaction(func(tx *gorm.DB) error {
		po, err := findLiveProvider(tx, input.ProviderID, true)
		if err != nil {
			return err
		}
		provider, err := po.toDomain()
		if err != nil {
			return err
		}
		normalized, err := domainsandbox.NormalizeUpdateProviderHealthInput(input, provider.Scopes)
		if err != nil {
			return err
		}
		providerID, err := positiveDomainInt64ToUint64(normalized.ProviderID)
		if err != nil {
			return err
		}
		actorUserID, err := positiveDomainInt64ToUint64(normalized.ActorUserID)
		if err != nil {
			return err
		}
		latencyMillis, err := nonNegativeDomainInt64ToUint32(normalized.Health.LatencyMillis)
		if err != nil {
			return domainsandbox.ErrInvalidInput
		}
		nextVersion, err = domainsandbox.NextVersion(normalized.ExpectedVersion)
		if err != nil {
			return err
		}
		capabilitiesJSON, err := marshalScopes(normalized.Health.Capabilities)
		if err != nil {
			return err
		}
		featuresJSON, err := marshalProviderFeatures(normalized.Health.Features)
		if err != nil {
			return err
		}
		var checkedAt *time.Time
		if !normalized.Health.CheckedAt.IsZero() {
			value := normalized.Health.CheckedAt.UTC().Truncate(time.Millisecond)
			checkedAt = &value
		}
		result := tx.Model(&providerPO{}).
			Where("id = ? AND version = ? AND deleted_at IS NULL", providerID, normalized.ExpectedVersion).
			Updates(map[string]any{
				"health_status":                 string(normalized.Health.Status),
				"last_health_capabilities_json": capabilitiesJSON,
				"last_health_features_json":     featuresJSON,
				"last_health_code":              normalized.Health.ReasonCode,
				"last_health_message":           normalized.Health.Message,
				"last_health_latency_ms":        latencyMillis,
				"last_health_at":                checkedAt,
				"updated_by":                    actorUserID,
				"updated_at":                    persistenceNow(),
				"version":                       gorm.Expr("version + 1"),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return providerCASFailure(tx, normalized.ProviderID)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return nextVersion, nil
}

func (r *MySQLRepository) DeleteProvider(
	ctx context.Context,
	input domainsandbox.DeleteProviderInput,
) (uint64, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return 0, err
	}
	defer release()
	if err := domainsandbox.ValidateDeleteProviderInput(input); err != nil {
		return 0, err
	}
	nextVersion, err := domainsandbox.NextVersion(input.ExpectedVersion)
	if err != nil {
		return 0, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return 0, err
	}
	providerID, err := positiveDomainInt64ToUint64(input.ProviderID)
	if err != nil {
		return 0, err
	}
	actorUserID, err := positiveDomainInt64ToUint64(input.ActorUserID)
	if err != nil {
		return 0, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		provider, err := findLiveProvider(tx, input.ProviderID, true)
		if err != nil {
			return err
		}
		if provider.Version != input.ExpectedVersion {
			return domainsandbox.ErrVersionConflict
		}
		var references []providerDefaultPO
		query := tx.Select("scope").Where("provider_id = ?", providerID).Limit(1)
		query = withUpdateLock(query)
		if err := query.Find(&references).Error; err != nil {
			return err
		}
		if len(references) != 0 {
			return domainsandbox.ErrProviderInUse
		}
		now := persistenceNow()
		result := tx.Model(&providerPO{}).
			Where("id = ? AND version = ? AND deleted_at IS NULL", providerID, input.ExpectedVersion).
			Updates(map[string]any{
				"deleted_at": now,
				"updated_at": now,
				"updated_by": actorUserID,
				"version":    gorm.Expr("version + 1"),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return providerCASFailure(tx, input.ProviderID)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return nextVersion, nil
}
