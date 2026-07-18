// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func (r *MySQLRepository) SummarizeProviders(
	ctx context.Context,
	authorizedScopes []domainsandbox.Scope,
) (domainsandbox.ProviderSummary, error) {
	release, err := r.acquireOperation()
	if err != nil {
		return domainsandbox.ProviderSummary{}, err
	}
	defer release()
	normalizedScopes, err := domainsandbox.NormalizeScopes(authorizedScopes)
	if err != nil {
		return domainsandbox.ProviderSummary{}, err
	}
	db, err := r.dbFor(ctx)
	if err != nil {
		return domainsandbox.ProviderSummary{}, err
	}
	query := db.Model(&providerPO{}).Where("deleted_at IS NULL")
	condition, arguments, err := providerScopeSubsetSQL(db.Dialector.Name(), normalizedScopes)
	if err != nil {
		return domainsandbox.ProviderSummary{}, err
	}
	query = query.Where(condition, arguments...)
	var row struct {
		Total     int64 `gorm:"column:total"`
		Enabled   int64 `gorm:"column:enabled"`
		Unhealthy int64 `gorm:"column:unhealthy"`
	}
	err = query.Select(
		"COUNT(*) AS total, "+
			"COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS enabled, "+
			"COALESCE(SUM(CASE WHEN health_status = ? THEN 1 ELSE 0 END), 0) AS unhealthy",
		string(domainsandbox.ProviderStatusEnabled),
		string(domainsandbox.HealthStatusUnhealthy),
	).Scan(&row).Error
	if err != nil {
		return domainsandbox.ProviderSummary{}, err
	}
	return domainsandbox.ProviderSummary{
		Total: row.Total, Enabled: row.Enabled, Unhealthy: row.Unhealthy,
	}, nil
}
