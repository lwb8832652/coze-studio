// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

func TestMaintenanceRepositoryLegacyAndProjectionAPIsHaveOneWayDelegation(t *testing.T) {
	t.Parallel()

	repository := &MaintenanceRepository{}
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	subscription := domainbilling.MaintenanceSubscriptionCandidate{ID: 1}
	order := domainbilling.MaintenanceOrderCandidate{ID: 2}

	_, err := repository.NotifyExpiringSubscription(
		context.Background(), subscription, now, time.Hour,
	)
	if !errors.Is(err, domainbilling.ErrInvalidInput) {
		t.Fatalf("legacy expiring API error = %v, want ErrInvalidInput", err)
	}
	_, err = repository.ProjectExpiringSubscription(
		context.Background(), subscription, now, time.Hour,
	)
	if !errors.Is(err, domainbilling.ErrInvalidInput) {
		t.Fatalf("expiring projection API error = %v, want ErrInvalidInput", err)
	}

	_, err = repository.ExpireDueSubscription(context.Background(), subscription, now)
	if !errors.Is(err, domainbilling.ErrInvalidInput) {
		t.Fatalf("legacy subscription API error = %v, want ErrInvalidInput", err)
	}
	_, err = repository.ExpireDueSubscriptionProjection(
		context.Background(), subscription, now,
	)
	if !errors.Is(err, domainbilling.ErrInvalidInput) {
		t.Fatalf("subscription projection API error = %v, want ErrInvalidInput", err)
	}

	_, err = repository.CloseExpiredOrder(context.Background(), order, now)
	if !errors.Is(err, domainbilling.ErrInvalidInput) {
		t.Fatalf("legacy order API error = %v, want ErrInvalidInput", err)
	}
	_, err = repository.CloseExpiredOrderProjection(context.Background(), order, now)
	if !errors.Is(err, domainbilling.ErrInvalidInput) {
		t.Fatalf("order projection API error = %v, want ErrInvalidInput", err)
	}
}
