// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestRunMaintenanceNotificationProjectionUsesUTCWindowAndCountsOnlyTransitions(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, time.July, 24, 16, 0, 0, 0, location)
	window := 96 * time.Hour
	repository := &maintenanceProjectionTestRepository{
		expiringBatch: domainbilling.MaintenanceSubscriptionBatch{Candidates: []domainbilling.MaintenanceSubscriptionCandidate{{ID: 1}, {ID: 2}}},
		dueBatch:      domainbilling.MaintenanceSubscriptionBatch{Candidates: []domainbilling.MaintenanceSubscriptionCandidate{{ID: 3}, {ID: 4}}},
		orderBatch:    domainbilling.MaintenanceOrderBatch{Candidates: []domainbilling.MaintenanceOrderCandidate{{ID: 5}, {ID: 6}}},
		notifyOutcomes: map[int64]domainbilling.MaintenanceProjectionOutcome{
			1: {NotificationInserted: true}, 2: {},
		},
		expireOutcomes: map[int64]domainbilling.MaintenanceProjectionOutcome{
			3: {Transitioned: true, NotificationInserted: true}, 4: {Stale: true},
		},
		closeOutcomes: map[int64]domainbilling.MaintenanceProjectionOutcome{
			5: {Transitioned: true, NotificationInserted: true}, 6: {Stale: true},
		},
	}

	result, failures := runMaintenanceNotificationProjection(context.Background(), repository, now, window)
	if len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
	if result.ExpiringSubscriptions != 1 || result.ExpiredSubscriptions != 1 || result.ClosedOrders != 1 {
		t.Fatalf("projection result = %#v", result)
	}
	if result.Expiring.NewProjections != 1 || result.Expiring.DuplicateProjections != 1 ||
		result.Expired.StaleCandidates != 1 || result.Orders.StaleCandidates != 1 {
		t.Fatalf("stage observability = %#v", result)
	}
	for name, observed := range map[string]time.Time{
		"expiring": repository.expiringNow,
		"due":      repository.dueNow,
		"orders":   repository.ordersNow,
	} {
		if !observed.Equal(now.UTC()) || observed.Location() != time.UTC {
			t.Fatalf("%s scan time = %v, want UTC %v", name, observed, now.UTC())
		}
	}
	if repository.expiringWindow != window {
		t.Fatalf("expiring window = %v, want %v", repository.expiringWindow, window)
	}
	for _, limit := range []int{repository.expiringLimit, repository.dueLimit, repository.orderLimit} {
		if limit != maintenanceProjectionBatchSize {
			t.Fatalf("candidate limit = %d, want %d", limit, maintenanceProjectionBatchSize)
		}
	}
}

func TestRunMaintenanceNotificationProjectionContinuesAfterCorruptCandidate(t *testing.T) {
	corruptRouteFailure := domainnotification.ErrRecipientResolution
	listFailure := errors.New("due scan failed")
	repository := &maintenanceProjectionTestRepository{
		expiringBatch: domainbilling.MaintenanceSubscriptionBatch{
			Candidates: []domainbilling.MaintenanceSubscriptionCandidate{{ID: 11}, {ID: 12}},
			Issues: []domainbilling.MaintenanceIssueSummary{{Reason: domainbilling.MaintenanceIssueMissingAccount, Count: 3}},
		},
		orderBatch: domainbilling.MaintenanceOrderBatch{Candidates: []domainbilling.MaintenanceOrderCandidate{{ID: 13}}},
		notifyOutcomes: map[int64]domainbilling.MaintenanceProjectionOutcome{12: {NotificationInserted: true}},
		closeOutcomes:  map[int64]domainbilling.MaintenanceProjectionOutcome{13: {Transitioned: true, NotificationInserted: true}},
		notifyErrors:   map[int64]error{11: corruptRouteFailure},
		scanDueError:   listFailure,
	}

	result, failures := runMaintenanceNotificationProjection(
		context.Background(), repository, time.Now(), 24*time.Hour,
	)
	if result.ExpiringSubscriptions != 1 || result.ExpiredSubscriptions != 0 || result.ClosedOrders != 1 {
		t.Fatalf("projection result = %#v", result)
	}
	if len(failures) != 2 || !errors.Is(failures[0], corruptRouteFailure) || !errors.Is(failures[1], listFailure) {
		t.Fatalf("failures = %v", failures)
	}
	if len(repository.notifiedIDs) != 2 || len(repository.closedIDs) != 1 {
		t.Fatalf("processing stopped early: notified=%v closed=%v", repository.notifiedIDs, repository.closedIDs)
	}
	if result.Expiring.InvalidCandidates != 4 || result.Expiring.Failed != 0 || result.Expired.Failed != 1 {
		t.Fatalf("bounded stage counts = %#v", result)
	}
}

func TestParseSubscriptionExpiringWindow(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "configured", value: "96h", want: 96 * time.Hour},
		{name: "trimmed", value: " 48h ", want: 48 * time.Hour},
		{name: "empty", value: "", want: defaultSubscriptionExpiringWindow},
		{name: "invalid", value: "seven days", want: defaultSubscriptionExpiringWindow},
		{name: "non-positive", value: "0s", want: defaultSubscriptionExpiringWindow},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := parseSubscriptionExpiringWindow(test.value); got != test.want {
				t.Fatalf("parseSubscriptionExpiringWindow(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

type maintenanceProjectionTestRepository struct {
	expiringBatch domainbilling.MaintenanceSubscriptionBatch
	dueBatch      domainbilling.MaintenanceSubscriptionBatch
	orderBatch    domainbilling.MaintenanceOrderBatch

	notifyOutcomes map[int64]domainbilling.MaintenanceProjectionOutcome
	expireOutcomes map[int64]domainbilling.MaintenanceProjectionOutcome
	closeOutcomes  map[int64]domainbilling.MaintenanceProjectionOutcome
	notifyErrors  map[int64]error
	expireErrors  map[int64]error
	closeErrors   map[int64]error

	scanExpiringError error
	scanDueError      error
	scanOrderError    error

	expiringNow    time.Time
	dueNow         time.Time
	ordersNow      time.Time
	expiringWindow time.Duration
	expiringLimit  int
	dueLimit       int
	orderLimit     int
	notifiedIDs    []int64
	expiredIDs     []int64
	closedIDs      []int64
}

func (r *maintenanceProjectionTestRepository) ScanExpiringSubscriptions(
	_ context.Context,
	now time.Time,
	window time.Duration,
	limit int,
) (domainbilling.MaintenanceSubscriptionBatch, error) {
	r.expiringNow, r.expiringWindow, r.expiringLimit = now, window, limit
	return r.expiringBatch, r.scanExpiringError
}

func (r *maintenanceProjectionTestRepository) ProjectExpiringSubscription(
	_ context.Context,
	candidate domainbilling.MaintenanceSubscriptionCandidate,
	_ time.Time,
	_ time.Duration,
) (domainbilling.MaintenanceProjectionOutcome, error) {
	r.notifiedIDs = append(r.notifiedIDs, candidate.ID)
	return r.notifyOutcomes[candidate.ID], r.notifyErrors[candidate.ID]
}

func (r *maintenanceProjectionTestRepository) ScanDueSubscriptions(
	_ context.Context,
	now time.Time,
	limit int,
) (domainbilling.MaintenanceSubscriptionBatch, error) {
	r.dueNow, r.dueLimit = now, limit
	return r.dueBatch, r.scanDueError
}

func (r *maintenanceProjectionTestRepository) ExpireDueSubscriptionProjection(
	_ context.Context,
	candidate domainbilling.MaintenanceSubscriptionCandidate,
	_ time.Time,
) (domainbilling.MaintenanceProjectionOutcome, error) {
	r.expiredIDs = append(r.expiredIDs, candidate.ID)
	return r.expireOutcomes[candidate.ID], r.expireErrors[candidate.ID]
}

func (r *maintenanceProjectionTestRepository) ScanExpiredOrders(
	_ context.Context,
	now time.Time,
	limit int,
) (domainbilling.MaintenanceOrderBatch, error) {
	r.ordersNow, r.orderLimit = now, limit
	return r.orderBatch, r.scanOrderError
}

func (r *maintenanceProjectionTestRepository) CloseExpiredOrderProjection(
	_ context.Context,
	candidate domainbilling.MaintenanceOrderCandidate,
	_ time.Time,
) (domainbilling.MaintenanceProjectionOutcome, error) {
	r.closedIDs = append(r.closedIDs, candidate.ID)
	return r.closeOutcomes[candidate.ID], r.closeErrors[candidate.ID]
}
