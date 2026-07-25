// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

// These SQLite tests cover candidate filtering, stable projection identity,
// rollback, and CAS behavior. Real MySQL FOR UPDATE and unique-key concurrency
// are covered by maintenance_repository_mysql_integration_test.go.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestMaintenanceRepositoryListsSubscriptionCandidatesAtClockBoundariesAndTimezone(t *testing.T) {
	db, repository := newMaintenanceRepositoryTestDatabase(t, true)
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, time.July, 24, 9, 30, 0, 0, shanghai)
	window := 72 * time.Hour

	seedMaintenanceRepositorySubscription(t, db, 101, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1101}, 1101, now.Add(-time.Millisecond), 3)
	seedMaintenanceRepositorySubscription(t, db, 102, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1102}, 1102, now, 3)
	seedMaintenanceRepositorySubscription(t, db, 103, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1103}, 1103, now.Add(24*time.Hour), 3)
	seedMaintenanceRepositorySubscription(t, db, 104, domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 2104}, 1104, now.Add(window), 3)
	seedMaintenanceRepositorySubscription(t, db, 105, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1105}, 1105, now.Add(window+time.Millisecond), 3)

	expiring, err := repository.ListExpiringSubscriptions(context.Background(), now, window, 10)
	if err != nil {
		t.Fatalf("ListExpiringSubscriptions() error = %v", err)
	}
	assertMaintenanceRepositoryCandidateIDs(t, expiring, []int64{103, 104})
	if expiring[1].Route.Subject.Type != domainbilling.SubjectTypeWorkspace ||
		expiring[1].Route.Subject.ID != 2104 || expiring[1].Route.ActorUserID != 1104 {
		t.Fatalf("workspace route = %#v, want persisted workspace/order facts", expiring[1].Route)
	}

	bounded, err := repository.ListExpiringSubscriptions(context.Background(), now, window, 1)
	if err != nil {
		t.Fatalf("bounded ListExpiringSubscriptions() error = %v", err)
	}
	assertMaintenanceRepositoryCandidateIDs(t, bounded, []int64{103})

	due, err := repository.ListDueSubscriptions(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("ListDueSubscriptions() error = %v", err)
	}
	assertMaintenanceRepositoryCandidateIDs(t, due, []int64{101, 102})
	if !due[1].CurrentPeriodEnd.Equal(now.UTC()) {
		t.Fatalf("due boundary = %v, want %v", due[1].CurrentPeriodEnd, now.UTC())
	}
}

func TestNotifyExpiringSubscriptionIsReplaySafeAndUsesPersistedRoutes(t *testing.T) {
	db, repository := newMaintenanceRepositoryTestDatabase(t, true)
	now := time.Date(2026, time.July, 24, 1, 0, 0, 0, time.UTC)
	window := 72 * time.Hour
	seedMaintenanceRepositorySubscription(t, db, 201, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1201}, 1201, now.Add(24*time.Hour), 4)
	seedMaintenanceRepositorySubscription(t, db, 202, domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9202}, 1202, now.Add(48*time.Hour), 5)

	candidates, err := repository.ListExpiringSubscriptions(context.Background(), now, window, 10)
	if err != nil {
		t.Fatalf("ListExpiringSubscriptions() error = %v", err)
	}
	for _, original := range candidates {
		candidate := original
		candidate.Route = domainbilling.BillingNotificationRoute{
			Subject:     domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 999999},
			ActorUserID: 999999,
		}
		for scanIndex, scanTime := range []time.Time{now, now.Add(time.Hour)} {
			projected, projectErr := repository.NotifyExpiringSubscription(
				context.Background(), candidate, scanTime, window,
			)
			if projectErr != nil {
				t.Fatalf("NotifyExpiringSubscription(%d) error = %v", candidate.ID, projectErr)
			}
			wantProjected := scanIndex == 0
			if projected != wantProjected {
				t.Fatalf("NotifyExpiringSubscription(%d) projected = %v, want %v", candidate.ID, projected, wantProjected)
			}
		}
	}

	rows := readMaintenanceRepositoryOutbox(t, db, domainnotification.EventBillingSubscriptionExpiring)
	if len(rows) != 2 {
		t.Fatalf("subscription_expiring outbox rows = %d, want 2", len(rows))
	}
	wantRoutes := map[string]struct {
		actorID int64
		spaceID int64
	}{
		"201": {actorID: 1201, spaceID: 0},
		"202": {actorID: 1202, spaceID: 9202},
	}
	for _, row := range rows {
		want, exists := wantRoutes[row.AggregateID]
		if !exists {
			t.Fatalf("unexpected aggregate ID %q", row.AggregateID)
		}
		if row.ActorID != want.actorID || row.SpaceID != want.spaceID {
			t.Fatalf("route for %s = actor %d space %d, want actor %d space %d", row.AggregateID, row.ActorID, row.SpaceID, want.actorID, want.spaceID)
		}
		assertMaintenanceRepositorySafePayload(t, row.PayloadJSON)
	}
}

func TestListExpiringSubscriptionsProgressesPastCommittedBatchProjection(t *testing.T) {
	db, repository := newMaintenanceRepositoryTestDatabase(t, true)
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, time.July, 24, 9, 30, 0, 0, shanghai)
	window := 72 * time.Hour
	seedMaintenanceRepositorySubscription(t, db, 211, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1211}, 1211, now.Add(24*time.Hour), 2)
	seedMaintenanceRepositorySubscription(t, db, 212, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1212}, 1212, now.Add(25*time.Hour), 2)
	seedMaintenanceRepositorySubscription(t, db, 213, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1213}, 1213, now.Add(26*time.Hour), 2)

	firstBatch, err := repository.ListExpiringSubscriptions(context.Background(), now, window, 2)
	if err != nil {
		t.Fatalf("first ListExpiringSubscriptions() error = %v", err)
	}
	assertMaintenanceRepositoryCandidateIDs(t, firstBatch, []int64{211, 212})
	for _, candidate := range firstBatch {
		projected, projectErr := repository.NotifyExpiringSubscription(context.Background(), candidate, now, window)
		if projectErr != nil || !projected {
			t.Fatalf("NotifyExpiringSubscription(%d) = %v, %v", candidate.ID, projected, projectErr)
		}
	}

	projected, err := repository.NotifyExpiringSubscription(context.Background(), firstBatch[0], now.Add(time.Hour), window)
	if err != nil || projected {
		t.Fatalf("replayed NotifyExpiringSubscription() = %v, %v, want false, nil", projected, err)
	}
	secondBatch, err := repository.ListExpiringSubscriptions(context.Background(), now.Add(time.Hour), window, 2)
	if err != nil {
		t.Fatalf("second ListExpiringSubscriptions() error = %v", err)
	}
	assertMaintenanceRepositoryCandidateIDs(t, secondBatch, []int64{213})
	projected, err = repository.NotifyExpiringSubscription(context.Background(), secondBatch[0], now.Add(time.Hour), window)
	if err != nil || !projected {
		t.Fatalf("NotifyExpiringSubscription(213) = %v, %v", projected, err)
	}

	finalBatch, err := repository.ListExpiringSubscriptions(context.Background(), now.Add(2*time.Hour), window, 2)
	if err != nil {
		t.Fatalf("final ListExpiringSubscriptions() error = %v", err)
	}
	if len(finalBatch) != 0 {
		t.Fatalf("final candidates = %#v, want none", finalBatch)
	}
	if rows := readMaintenanceRepositoryOutbox(t, db, domainnotification.EventBillingSubscriptionExpiring); len(rows) != 3 {
		t.Fatalf("subscription_expiring outbox rows = %d, want 3", len(rows))
	}
}

func TestMaintenanceRepositorySQLiteDomainConcurrentReplayIsIdempotent(t *testing.T) {
	db, repository := newMaintenanceRepositoryTestDatabase(t, true)
	now := time.Date(2026, time.July, 24, 2, 0, 0, 0, time.UTC)
	seedMaintenanceRepositorySubscription(t, db, 301, domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9301}, 1301, now, 7)
	candidates, err := repository.ListDueSubscriptions(context.Background(), now, 10)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("ListDueSubscriptions() = %#v, %v", candidates, err)
	}

	type transitionResult struct {
		changed bool
		err     error
	}
	results := make(chan transitionResult, 2)
	for worker := 0; worker < 2; worker++ {
		go func() {
			changed, transitionErr := repository.ExpireDueSubscription(context.Background(), candidates[0], now)
			results <- transitionResult{changed: changed, err: transitionErr}
		}()
	}
	changedCount := 0
	for worker := 0; worker < 2; worker++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("ExpireDueSubscription() error = %v", result.err)
		}
		if result.changed {
			changedCount++
		}
	}
	if changedCount != 1 {
		t.Fatalf("terminal transitions = %d, want 1", changedCount)
	}

	changed, err := repository.ExpireDueSubscription(context.Background(), candidates[0], now.Add(time.Hour))
	if err != nil || changed {
		t.Fatalf("replayed ExpireDueSubscription() = %v, %v, want false, nil", changed, err)
	}
	var stored maintenanceRepositoryTestSubscriptionPO
	if err = db.First(&stored, 301).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != string(domainbilling.SubscriptionStatusExpired) || stored.Version != 8 || stored.AutoRenew {
		t.Fatalf("stored subscription = %#v", stored)
	}
	rows := readMaintenanceRepositoryOutbox(t, db, domainnotification.EventBillingSubscriptionExpired)
	if len(rows) != 1 || rows[0].ActorID != 1301 || rows[0].SpaceID != 9301 {
		t.Fatalf("subscription_expired outbox = %#v", rows)
	}
}

func TestExpireDueSubscriptionRejectsStaleCandidate(t *testing.T) {
	db, repository := newMaintenanceRepositoryTestDatabase(t, true)
	now := time.Date(2026, time.July, 24, 3, 0, 0, 0, time.UTC)
	seedMaintenanceRepositorySubscription(t, db, 401, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1401}, 1401, now, 2)
	candidates, err := repository.ListDueSubscriptions(context.Background(), now, 10)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("ListDueSubscriptions() = %#v, %v", candidates, err)
	}
	newEnd := now.Add(30 * 24 * time.Hour)
	if err = db.Model(&maintenanceRepositoryTestSubscriptionPO{}).Where("id = ?", 401).Updates(map[string]any{
		"current_period_end": newEnd,
		"version":            3,
	}).Error; err != nil {
		t.Fatal(err)
	}

	changed, err := repository.ExpireDueSubscription(context.Background(), candidates[0], now)
	if err != nil || changed {
		t.Fatalf("stale ExpireDueSubscription() = %v, %v, want false, nil", changed, err)
	}
	if rows := readMaintenanceRepositoryOutbox(t, db, domainnotification.EventBillingSubscriptionExpired); len(rows) != 0 {
		t.Fatalf("stale candidate appended %d events", len(rows))
	}
}

func TestCloseExpiredOrderIsReplaySafeAndUsesPersistedRoutes(t *testing.T) {
	db, repository := newMaintenanceRepositoryTestDatabase(t, true)
	now := time.Date(2026, time.July, 24, 4, 0, 0, 0, time.UTC)
	seedMaintenanceRepositoryOrder(t, db, 501, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1501}, 1501, now)
	seedMaintenanceRepositoryOrder(t, db, 502, domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9502}, 1502, now.Add(-time.Millisecond))
	seedMaintenanceRepositoryOrder(t, db, 503, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1503}, 1503, now.Add(time.Millisecond))

	candidates, err := repository.ListExpiredOrders(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("ListExpiredOrders() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expired order candidates = %d, want 2", len(candidates))
	}
	for _, original := range candidates {
		candidate := original
		candidate.Route = domainbilling.BillingNotificationRoute{
			Subject:     domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 999999},
			ActorUserID: 999999,
		}
		changed, closeErr := repository.CloseExpiredOrder(context.Background(), candidate, now)
		if closeErr != nil || !changed {
			t.Fatalf("CloseExpiredOrder(%d) = %v, %v", candidate.ID, changed, closeErr)
		}
		changed, closeErr = repository.CloseExpiredOrder(context.Background(), candidate, now.Add(time.Hour))
		if closeErr != nil || changed {
			t.Fatalf("replayed CloseExpiredOrder(%d) = %v, %v", candidate.ID, changed, closeErr)
		}
	}

	rows := readMaintenanceRepositoryOutbox(t, db, domainnotification.EventBillingOrderTimedOut)
	if len(rows) != 2 {
		t.Fatalf("order_timed_out outbox rows = %d, want 2", len(rows))
	}
	wantRoutes := map[string]struct {
		actorID int64
		spaceID int64
	}{
		"501": {actorID: 1501, spaceID: 0},
		"502": {actorID: 1502, spaceID: 9502},
	}
	for _, row := range rows {
		want := wantRoutes[row.AggregateID]
		if row.ActorID != want.actorID || row.SpaceID != want.spaceID {
			t.Fatalf("route for %s = actor %d space %d", row.AggregateID, row.ActorID, row.SpaceID)
		}
		assertMaintenanceRepositorySafePayload(t, row.PayloadJSON)
	}
	for _, orderID := range []int64{501, 502} {
		var stored maintenanceRepositoryTestOrderPO
		if err = db.First(&stored, orderID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Status != string(domainbilling.OrderStatusClosed) ||
			stored.PaymentStatus != string(domainbilling.PaymentStatusFailed) || stored.Version != 5 {
			t.Fatalf("stored order %d = %#v", orderID, stored)
		}
	}
}

func TestMaintenanceRepositoryRollsBackWhenOutboxAppendFails(t *testing.T) {
	t.Run("expiring projection", func(t *testing.T) {
		db, repository := newMaintenanceRepositoryTestDatabase(t, true)
		now := time.Date(2026, time.July, 24, 5, 0, 0, 0, time.UTC)
		seedMaintenanceRepositorySubscription(t, db, 601, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1601}, 1601, now.Add(time.Hour), 2)
		candidates, err := repository.ListExpiringSubscriptions(context.Background(), now, 24*time.Hour, 10)
		if err != nil || len(candidates) != 1 {
			t.Fatalf("ListExpiringSubscriptions() = %#v, %v", candidates, err)
		}
		if err = db.Migrator().DropTable(&maintenanceRepositoryTestOutboxPO{}); err != nil {
			t.Fatal(err)
		}
		if _, err = repository.NotifyExpiringSubscription(context.Background(), candidates[0], now, 24*time.Hour); !errors.Is(err, domainnotification.ErrStorage) {
			t.Fatalf("NotifyExpiringSubscription() error = %v, want storage failure", err)
		}
		assertMaintenanceRepositorySubscriptionState(t, db, 601, domainbilling.SubscriptionStatusActive, 2)
	})

	t.Run("subscription terminal transition", func(t *testing.T) {
		db, repository := newMaintenanceRepositoryTestDatabase(t, false)
		now := time.Date(2026, time.July, 24, 6, 0, 0, 0, time.UTC)
		seedMaintenanceRepositorySubscription(t, db, 602, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1602}, 1602, now, 2)
		candidates, err := repository.ListDueSubscriptions(context.Background(), now, 10)
		if err != nil || len(candidates) != 1 {
			t.Fatalf("ListDueSubscriptions() = %#v, %v", candidates, err)
		}
		if _, err = repository.ExpireDueSubscription(context.Background(), candidates[0], now); !errors.Is(err, domainnotification.ErrStorage) {
			t.Fatalf("ExpireDueSubscription() error = %v, want storage failure", err)
		}
		assertMaintenanceRepositorySubscriptionState(t, db, 602, domainbilling.SubscriptionStatusActive, 2)
	})

	t.Run("order terminal transition", func(t *testing.T) {
		db, repository := newMaintenanceRepositoryTestDatabase(t, false)
		now := time.Date(2026, time.July, 24, 7, 0, 0, 0, time.UTC)
		seedMaintenanceRepositoryOrder(t, db, 603, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1603}, 1603, now)
		candidates, err := repository.ListExpiredOrders(context.Background(), now, 10)
		if err != nil || len(candidates) != 1 {
			t.Fatalf("ListExpiredOrders() = %#v, %v", candidates, err)
		}
		if _, err = repository.CloseExpiredOrder(context.Background(), candidates[0], now); !errors.Is(err, domainnotification.ErrStorage) {
			t.Fatalf("CloseExpiredOrder() error = %v, want storage failure", err)
		}
		var stored maintenanceRepositoryTestOrderPO
		if err = db.First(&stored, 603).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Status != string(domainbilling.OrderStatusPending) ||
			stored.PaymentStatus != string(domainbilling.PaymentStatusPending) || stored.Version != 4 {
			t.Fatalf("order transition was not rolled back: %#v", stored)
		}
	})
}

func TestMaintenanceRepositorySelectedCandidateBecomingCrossAccountIsIsolated(t *testing.T) {
	db, repository := newMaintenanceRepositoryTestDatabase(t, true)
	now := time.Date(2026, time.July, 24, 8, 0, 0, 0, time.UTC)
	seedMaintenanceRepositorySubscription(t, db, 701, domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9701}, 1701, now, 2)
	seedMaintenanceRepositorySubscription(t, db, 702, domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9702}, 1702, now, 2)
	crossAccountID := int64(7999)
	if err := db.Create(&maintenanceRepositoryTestAccountPO{
		ID: crossAccountID, SubjectType: string(domainbilling.SubjectTypeWorkspace), SubjectID: 9799,
	}).Error; err != nil {
		t.Fatal(err)
	}
	candidates, err := repository.ListDueSubscriptions(context.Background(), now, 10)
	if err != nil {
		t.Fatalf("ListDueSubscriptions() error = %v", err)
	}
	assertMaintenanceRepositoryCandidateIDs(t, candidates, []int64{701, 702})
	if err := db.Model(&maintenanceRepositoryTestOrderPO{}).
		Where("id = ?", int64(701*10+2)).
		Updates(map[string]any{"account_id": crossAccountID, "user_id": int64(1799)}).Error; err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		changed, transitionErr := repository.ExpireDueSubscription(context.Background(), candidate, now)
		if candidate.ID == 701 {
			if changed || !errors.Is(transitionErr, domainnotification.ErrRecipientResolution) {
				t.Fatalf("cross-account candidate = %v, %v, want false and recipient error", changed, transitionErr)
			}
			continue
		}
		if transitionErr != nil || !changed {
			t.Fatalf("valid candidate = %v, %v, want true, nil", changed, transitionErr)
		}
	}
	assertMaintenanceRepositorySubscriptionState(t, db, 701, domainbilling.SubscriptionStatusActive, 2)
	assertMaintenanceRepositorySubscriptionState(t, db, 702, domainbilling.SubscriptionStatusExpired, 3)
	rows := readMaintenanceRepositoryOutbox(t, db, domainnotification.EventBillingSubscriptionExpired)
	if len(rows) != 1 || rows[0].AggregateID != "702" || rows[0].ActorID != 1702 || rows[0].SpaceID != 9702 {
		t.Fatalf("isolated outbox rows = %#v", rows)
	}
}

func TestMaintenanceRepositoryBadRoutesBeyondLimitDoNotBlockValidCandidates(t *testing.T) {
	db, repository := newMaintenanceRepositoryTestDatabase(t, true)
	now := time.Date(2026, time.July, 24, 9, 0, 0, 0, time.UTC)
	for _, id := range []int64{711, 712, 713} {
		seedMaintenanceRepositorySubscription(t, db, id, domainbilling.Subject{Type: domainbilling.SubjectTypeWorkspace, ID: 9700 + id}, 1700+id, now, 2)
		crossAccountID := id*100 + 9
		if err := db.Create(&maintenanceRepositoryTestAccountPO{
			ID: crossAccountID, SubjectType: string(domainbilling.SubjectTypeWorkspace), SubjectID: 9900 + id,
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&maintenanceRepositoryTestOrderPO{}).
			Where("id = ?", id*10+2).
			Update("account_id", crossAccountID).Error; err != nil {
			t.Fatal(err)
		}
	}
	seedMaintenanceRepositorySubscription(t, db, 714, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1714}, 1714, now, 2)
	seedMaintenanceRepositorySubscription(t, db, 715, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: 1715}, 1715, now, 2)

	batch, err := repository.ScanDueSubscriptions(context.Background(), now, 2)
	if err != nil {
		t.Fatalf("ScanDueSubscriptions() error = %v", err)
	}
	assertMaintenanceRepositoryCandidateIDs(t, batch.Candidates, []int64{714, 715})
	if issueCountForMaintenanceTest(batch.Issues, domainbilling.MaintenanceIssueSourceOrderAccountMismatch) != 3 {
		t.Fatalf("issue summaries = %#v, want three account mismatches", batch.Issues)
	}
	for _, candidate := range batch.Candidates {
		outcome, transitionErr := repository.ExpireDueSubscriptionProjection(context.Background(), candidate, now)
		if transitionErr != nil || !outcome.Transitioned {
			t.Fatalf("valid candidate %d outcome = %#v, %v", candidate.ID, outcome, transitionErr)
		}
	}

	if err = db.Model(&maintenanceRepositoryTestOrderPO{}).
		Where("id = ?", int64(711*10+2)).
		Update("account_id", int64(711*10+1)).Error; err != nil {
		t.Fatal(err)
	}
	repaired, err := repository.ScanDueSubscriptions(context.Background(), now, 2)
	if err != nil {
		t.Fatalf("repaired ScanDueSubscriptions() error = %v", err)
	}
	assertMaintenanceRepositoryCandidateIDs(t, repaired.Candidates, []int64{711})
}

func issueCountForMaintenanceTest(
	issues []domainbilling.MaintenanceIssueSummary,
	reason domainbilling.MaintenanceIssueReason,
) int64 {
	for _, issue := range issues {
		if issue.Reason == reason {
			return issue.Count
		}
	}
	return 0
}

func newMaintenanceRepositoryTestDatabase(t *testing.T, withOutbox bool) (*gorm.DB, *MaintenanceRepository) {
	t.Helper()
	databaseName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+databaseName+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	tables := []any{
		&maintenanceRepositoryTestAccountPO{},
		&maintenanceRepositoryTestOrderPO{},
		&maintenanceRepositoryTestSubscriptionPO{},
	}
	if withOutbox {
		tables = append(tables, &maintenanceRepositoryTestOutboxPO{})
	}
	if err = db.AutoMigrate(tables...); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db, NewMaintenanceRepository(db, &maintenanceRepositoryTestIDGenerator{next: 10_000})
}

func seedMaintenanceRepositorySubscription(
	t *testing.T,
	db *gorm.DB,
	id int64,
	subject domainbilling.Subject,
	actorUserID int64,
	periodEnd time.Time,
	version int64,
) {
	t.Helper()
	accountID := id*10 + 1
	orderID := id*10 + 2
	now := periodEnd.Add(-30 * 24 * time.Hour).UTC()
	if err := db.Create(&maintenanceRepositoryTestAccountPO{
		ID: accountID, SubjectType: string(subject.Type), SubjectID: subject.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&maintenanceRepositoryTestOrderPO{
		ID: orderID, UserID: actorUserID, AccountID: accountID,
		Status: string(domainbilling.OrderStatusFulfilled), PaymentStatus: string(domainbilling.PaymentStatusSucceeded),
		FulfillmentStatus: string(domainbilling.FulfillmentStatusSucceeded), Version: 3,
		ExpiresAt: periodEnd.Add(-time.Hour).UTC(), UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&maintenanceRepositoryTestSubscriptionPO{
		ID: id, AccountID: accountID, SourceOrderID: orderID,
		Status: string(domainbilling.SubscriptionStatusActive), CurrentPeriodEnd: periodEnd.UTC(),
		AutoRenew: true, Version: version, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func seedMaintenanceRepositoryOrder(
	t *testing.T,
	db *gorm.DB,
	id int64,
	subject domainbilling.Subject,
	actorUserID int64,
	expiresAt time.Time,
) {
	t.Helper()
	accountID := id*10 + 1
	if err := db.Create(&maintenanceRepositoryTestAccountPO{
		ID: accountID, SubjectType: string(subject.Type), SubjectID: subject.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&maintenanceRepositoryTestOrderPO{
		ID: id, UserID: actorUserID, AccountID: accountID,
		Status: string(domainbilling.OrderStatusPending), PaymentStatus: string(domainbilling.PaymentStatusPending),
		FulfillmentStatus: string(domainbilling.FulfillmentStatusPending), Version: 4,
		ExpiresAt: expiresAt.UTC(), UpdatedAt: expiresAt.Add(-time.Hour).UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func assertMaintenanceRepositoryCandidateIDs(t *testing.T, candidates []DueSubscriptionCandidate, want []int64) {
	t.Helper()
	if len(candidates) != len(want) {
		t.Fatalf("candidate count = %d, want %d: %#v", len(candidates), len(want), candidates)
	}
	for index := range want {
		if candidates[index].ID != want[index] {
			t.Fatalf("candidate[%d].ID = %d, want %d", index, candidates[index].ID, want[index])
		}
	}
}

func assertMaintenanceRepositorySubscriptionState(t *testing.T, db *gorm.DB, id int64, status domainbilling.SubscriptionStatus, version int64) {
	t.Helper()
	var stored maintenanceRepositoryTestSubscriptionPO
	if err := db.First(&stored, id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != string(status) || stored.Version != version {
		t.Fatalf("subscription %d = %#v", id, stored)
	}
}

func readMaintenanceRepositoryOutbox(t *testing.T, db *gorm.DB, eventType domainnotification.EventType) []maintenanceRepositoryTestOutboxPO {
	t.Helper()
	if !db.Migrator().HasTable(&maintenanceRepositoryTestOutboxPO{}) {
		return nil
	}
	var rows []maintenanceRepositoryTestOutboxPO
	if err := db.Where("event_type = ?", string(eventType)).Order("aggregate_id ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func assertMaintenanceRepositorySafePayload(t *testing.T, raw []byte) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for key := range payload {
		if key != "resource_display_name" && key != "target_id" {
			t.Fatalf("unexpected payload field %q in %s", key, raw)
		}
	}
	normalized := strings.ToLower(string(raw))
	for _, forbidden := range []string{"ledger", "provider", "credential", "snapshot_json", "subject_id"} {
		if strings.Contains(normalized, forbidden) {
			t.Fatalf("payload exposes %q: %s", forbidden, raw)
		}
	}
}

type maintenanceRepositoryTestIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *maintenanceRepositoryTestIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

func (g *maintenanceRepositoryTestIDGenerator) GenMultiIDs(ctx context.Context, count int) ([]int64, error) {
	ids := make([]int64, count)
	for index := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	return ids, nil
}

type maintenanceRepositoryTestAccountPO struct {
	ID          int64  `gorm:"column:id;primaryKey"`
	SubjectType string `gorm:"column:subject_type"`
	SubjectID   int64  `gorm:"column:subject_id"`
}

func (maintenanceRepositoryTestAccountPO) TableName() string { return "billing_accounts" }

type maintenanceRepositoryTestOrderPO struct {
	ID                int64      `gorm:"column:id;primaryKey"`
	UserID            int64      `gorm:"column:user_id"`
	AccountID         int64      `gorm:"column:account_id"`
	Status            string     `gorm:"column:status"`
	PaymentStatus     string     `gorm:"column:payment_status"`
	FulfillmentStatus string     `gorm:"column:fulfillment_status"`
	Version           int64      `gorm:"column:version"`
	ExpiresAt         time.Time  `gorm:"column:expires_at"`
	ClosedAt          *time.Time `gorm:"column:closed_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (maintenanceRepositoryTestOrderPO) TableName() string { return "billing_orders" }

type maintenanceRepositoryTestSubscriptionPO struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	AccountID        int64     `gorm:"column:account_id"`
	SourceOrderID    int64     `gorm:"column:source_order_id"`
	Status           string    `gorm:"column:status"`
	CurrentPeriodEnd time.Time `gorm:"column:current_period_end"`
	AutoRenew        bool      `gorm:"column:auto_renew"`
	Version          int64     `gorm:"column:version"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (maintenanceRepositoryTestSubscriptionPO) TableName() string { return "user_subscriptions" }

type maintenanceRepositoryTestOutboxPO struct {
	ID               int64  `gorm:"column:id;primaryKey"`
	EventID          string `gorm:"column:event_id;size:128;uniqueIndex:uk_notification_outbox_event"`
	IdempotencyKey   string `gorm:"column:idempotency_key;size:64;uniqueIndex:uk_notification_outbox_idempotency"`
	EventType        string `gorm:"column:event_type;size:64"`
	AggregateType    string `gorm:"column:aggregate_type;size:64"`
	AggregateID      string `gorm:"column:aggregate_id;size:128"`
	AggregateVersion int64  `gorm:"column:aggregate_version"`
	OccurredAt       int64  `gorm:"column:occurred_at"`
	SpaceID          int64  `gorm:"column:space_id"`
	ActorID          int64  `gorm:"column:actor_id"`
	RecipientPolicy  string `gorm:"column:recipient_policy;size:64"`
	PayloadSchema    int32  `gorm:"column:payload_schema"`
	PayloadJSON      []byte `gorm:"column:payload_json;type:json"`
	Status           string `gorm:"column:status;size:16"`
	AttemptCount     int    `gorm:"column:attempt_count"`
	AvailableAt      int64  `gorm:"column:available_at"`
	LockedAt         int64  `gorm:"column:locked_at"`
	LockedBy         string `gorm:"column:locked_by;size:128"`
	LastErrorCode    string `gorm:"column:last_error_code;size:64"`
	CreatedAt        int64  `gorm:"column:created_at"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
	DeliveredAt      int64  `gorm:"column:delivered_at"`
}

func (maintenanceRepositoryTestOutboxPO) TableName() string { return "notification_outbox" }
