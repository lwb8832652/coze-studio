// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	infranotification "github.com/coze-dev/coze-studio/backend/infra/notification"
)

const (
	defaultMaintenanceCandidateLimit = 200
	maxMaintenanceCandidateLimit     = 500
)

var _ domainbilling.Repository = (*MySQLRepository)(nil)

const subscriptionMaintenanceSafeRoutePredicate = `
	accounts.subject_type IN ('user', 'workspace')
	AND accounts.subject_id > 0
	AND orders.user_id > 0
	AND (accounts.subject_type <> 'user' OR accounts.subject_id = orders.user_id)`

const orderMaintenanceSafeRoutePredicate = `
	accounts.subject_type IN ('user', 'workspace')
	AND accounts.subject_id > 0
	AND orders.user_id > 0
	AND (accounts.subject_type <> 'user' OR accounts.subject_id = orders.user_id)`

type MaintenanceRepository struct {
	db    *gorm.DB
	idGen idgen.IDGenerator
}

type ExpiredReservationCandidate struct {
	ID                int64
	ReserveBusinessNo string
}

// DueSubscriptionCandidate and ExpiredOrderCandidate remain as aliases for
// callers of the previous infra API. The reusable contract lives in domain.
type DueSubscriptionCandidate = domainbilling.MaintenanceSubscriptionCandidate
type ExpiredOrderCandidate = domainbilling.MaintenanceOrderCandidate

type subscriptionMaintenanceRow struct {
	ID               int64
	Version          int64
	AccountID        int64
	SourceOrderID    int64
	Status           string
	CurrentPeriodEnd time.Time
	SubjectType      string
	SubjectID        int64
	ActorUserID      int64
	SubjectAccountID int64
	JoinedOrderID    int64
	OrderAccountID   int64
}

type expiredOrderRow struct {
	ID                int64
	Version           int64
	AccountID         int64
	Status            string
	PaymentStatus     string
	FulfillmentStatus string
	ExpiresAt         time.Time
	SubjectType       string
	SubjectID         int64
	ActorUserID       int64
	SubjectAccountID  int64
}

const subscriptionMaintenanceSelect = `
	subscriptions.id,
	subscriptions.version,
	subscriptions.account_id,
	subscriptions.source_order_id,
	subscriptions.status,
	subscriptions.current_period_end,
	accounts.subject_type,
	accounts.subject_id,
	accounts.id AS subject_account_id,
	orders.id AS joined_order_id,
	orders.account_id AS order_account_id,
	orders.user_id AS actor_user_id`

const expiredOrderSelect = `
	orders.id,
	orders.version,
	orders.account_id,
	orders.status,
	orders.payment_status,
	orders.fulfillment_status,
	orders.expires_at,
	accounts.subject_type,
	accounts.subject_id,
	accounts.id AS subject_account_id,
	orders.user_id AS actor_user_id`

type AccountSubjectCandidate struct {
	AccountID   int64
	SubjectType string
	SubjectID   int64
}

type creditThresholdConfigPO struct {
	SubjectType          string    `gorm:"column:subject_type;primaryKey"`
	SubjectID            int64     `gorm:"column:subject_id;primaryKey"`
	Enabled              bool      `gorm:"column:enabled"`
	ThresholdMicros      int64     `gorm:"column:threshold_micros"`
	RecoveryMarginMicros int64     `gorm:"column:recovery_margin_micros"`
	Version              int64     `gorm:"column:version"`
	UpdatedBy            int64     `gorm:"column:updated_by"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
}

func (creditThresholdConfigPO) TableName() string {
	return "billing_credit_threshold_configs"
}

type creditThresholdEpisodePO struct {
	AccountID            int64      `gorm:"column:account_id;primaryKey"`
	EpisodeNo            int64      `gorm:"column:episode_no"`
	Active               bool       `gorm:"column:active"`
	ThresholdMicros      int64      `gorm:"column:threshold_micros"`
	RecoveryMarginMicros int64      `gorm:"column:recovery_margin_micros"`
	Version              int64      `gorm:"column:version"`
	OpenedAt             time.Time  `gorm:"column:opened_at"`
	RecoveredAt          *time.Time `gorm:"column:recovered_at"`
	CreatedAt            time.Time  `gorm:"column:created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at"`
}

func (creditThresholdEpisodePO) TableName() string {
	return "billing_credit_threshold_episodes"
}

type ReconciliationResult struct {
	RunID         int64          `json:"run_id,string"`
	CheckedCount  int64          `json:"checked_count"`
	MismatchCount int64          `json:"mismatch_count"`
	Mismatches    []AccountDrift `json:"mismatches"`
}

type AccountDrift struct {
	AccountID         int64 `json:"account_id,string"`
	AvailableMicros   int64 `json:"available_micros"`
	ExpectedAvailable int64 `json:"expected_available_micros"`
	ReservedMicros    int64 `json:"reserved_micros"`
	ExpectedReserved  int64 `json:"expected_reserved_micros"`
}

func NewMaintenanceRepository(db *gorm.DB, idGenerator idgen.IDGenerator) *MaintenanceRepository {
	return &MaintenanceRepository{db: db, idGen: idGenerator}
}

func (r *MaintenanceRepository) ListExpiredReservations(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]ExpiredReservationCandidate, error) {
	if limit <= 0 {
		limit = 200
	}
	var rows []ExpiredReservationCandidate
	err := r.db.WithContext(ctx).Table("credit_reservations").
		Select("id, reserve_business_no").
		Where("status = ? AND expires_at <= ?", string(domainbilling.ReservationStatusReserved), now).
		Order("expires_at ASC, id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

func (r *MaintenanceRepository) ListDueSubscriptions(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]DueSubscriptionCandidate, error) {
	if r == nil || r.db == nil || now.IsZero() {
		return nil, domainbilling.ErrInvalidInput
	}
	now = now.UTC()
	query := r.db.WithContext(ctx).Table("user_subscriptions AS subscriptions").
		Select(subscriptionMaintenanceSelect).
		Joins("JOIN billing_accounts AS accounts ON accounts.id = subscriptions.account_id").
		Joins("JOIN billing_orders AS orders ON orders.id = subscriptions.source_order_id AND orders.account_id = subscriptions.account_id").
		Where(subscriptionMaintenanceSafeRoutePredicate).
		Where("subscriptions.status = ? AND subscriptions.current_period_end <= ?", string(domainbilling.SubscriptionStatusActive), now).
		Order("subscriptions.current_period_end ASC, subscriptions.id ASC").
		Limit(boundedMaintenanceCandidateLimit(limit))
	return scanSubscriptionMaintenanceCandidates(query)
}

func (r *MaintenanceRepository) ListExpiringSubscriptions(
	ctx context.Context,
	now time.Time,
	window time.Duration,
	limit int,
) ([]DueSubscriptionCandidate, error) {
	if r == nil || r.db == nil || now.IsZero() || window <= 0 {
		return nil, domainbilling.ErrInvalidInput
	}
	now = now.UTC()
	query := r.db.WithContext(ctx).Table("user_subscriptions AS subscriptions").
		Select(subscriptionMaintenanceSelect).
		Joins("JOIN billing_accounts AS accounts ON accounts.id = subscriptions.account_id").
		Joins("JOIN billing_orders AS orders ON orders.id = subscriptions.source_order_id AND orders.account_id = subscriptions.account_id").
		Where(subscriptionMaintenanceSafeRoutePredicate).
		Where(
			"subscriptions.status = ? AND subscriptions.current_period_end > ? AND subscriptions.current_period_end <= ?",
			string(domainbilling.SubscriptionStatusActive),
			now,
			now.Add(window),
		).
		Where(
			unprojectedExpiringSubscriptionPredicate(r.db),
			string(domainnotification.EventBillingSubscriptionExpiring),
			"billing_subscription_period",
		).
		Order("subscriptions.current_period_end ASC, subscriptions.id ASC").
		Limit(boundedMaintenanceCandidateLimit(limit))
	return scanSubscriptionMaintenanceCandidates(query)
}

func (r *MaintenanceRepository) NotifyExpiringSubscription(
	ctx context.Context,
	candidate DueSubscriptionCandidate,
	now time.Time,
	window time.Duration,
) (bool, error) {
	outcome, err := r.ProjectExpiringSubscription(ctx, candidate, now, window)
	return outcome.NotificationInserted, err
}

func (r *MaintenanceRepository) ExpireDueSubscription(
	ctx context.Context,
	candidate DueSubscriptionCandidate,
	now time.Time,
) (bool, error) {
	outcome, err := r.ExpireDueSubscriptionProjection(ctx, candidate, now)
	return outcome.Transitioned, err
}

// ExpireSubscription preserves the previous repository API while routing the
// transition through the transactional notification projection.
func (r *MaintenanceRepository) ExpireSubscription(ctx context.Context, candidate DueSubscriptionCandidate) error {
	changed, err := r.ExpireDueSubscription(ctx, candidate, time.Now().UTC())
	if err != nil {
		return err
	}
	if !changed {
		return domainbilling.ErrVersionConflict
	}
	return nil
}

func (r *MaintenanceRepository) ListExpiredOrders(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]ExpiredOrderCandidate, error) {
	if r == nil || r.db == nil || now.IsZero() {
		return nil, domainbilling.ErrInvalidInput
	}
	now = now.UTC()
	var rows []expiredOrderRow
	err := r.db.WithContext(ctx).Table("billing_orders AS orders").
		Select(expiredOrderSelect).
		Joins("JOIN billing_accounts AS accounts ON accounts.id = orders.account_id").
		Where(orderMaintenanceSafeRoutePredicate).
		Where(
			"orders.status = ? AND orders.payment_status = ? AND orders.fulfillment_status = ? AND orders.expires_at <= ?",
			string(domainbilling.OrderStatusPending),
			string(domainbilling.PaymentStatusPending),
			string(domainbilling.FulfillmentStatusPending),
			now,
		).
		Order("orders.expires_at ASC, orders.id ASC").
		Limit(boundedMaintenanceCandidateLimit(limit)).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	candidates := make([]ExpiredOrderCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, expiredOrderCandidateFromRow(row))
	}
	return candidates, nil
}

func (r *MaintenanceRepository) CloseExpiredOrder(
	ctx context.Context,
	candidate ExpiredOrderCandidate,
	now time.Time,
) (bool, error) {
	outcome, err := r.CloseExpiredOrderProjection(ctx, candidate, now)
	return outcome.Transitioned, err
}

// CloseExpiredOrders preserves the previous API without returning to a bulk
// update. Every candidate still commits in its own transaction.
func (r *MaintenanceRepository) CloseExpiredOrders(ctx context.Context, now time.Time) (int64, error) {
	var closed int64
	for {
		candidates, err := r.ListExpiredOrders(ctx, now, maxMaintenanceCandidateLimit)
		if err != nil {
			return closed, err
		}
		for _, candidate := range candidates {
			changed, closeErr := r.CloseExpiredOrder(ctx, candidate, now)
			if closeErr != nil {
				return closed, closeErr
			}
			if changed {
				closed++
			}
		}
		if len(candidates) < maxMaintenanceCandidateLimit {
			return closed, nil
		}
	}
}

const subscriptionMaintenanceIssueReasonExpression = `CASE
	WHEN accounts.id IS NULL THEN 'missing_account'
	WHEN source_orders.id IS NULL THEN 'missing_source_order'
	WHEN source_orders.account_id <> subscriptions.account_id THEN 'source_order_account_mismatch'
	WHEN accounts.subject_type NOT IN ('user', 'workspace') OR accounts.subject_id <= 0 THEN 'invalid_subject'
	WHEN source_orders.user_id <= 0 THEN 'invalid_actor'
	WHEN accounts.subject_type = 'user' AND accounts.subject_id <> source_orders.user_id THEN 'user_subject_order_mismatch'
	ELSE ''
END`

const orderMaintenanceIssueReasonExpression = `CASE
	WHEN accounts.id IS NULL THEN 'missing_account'
	WHEN accounts.subject_type NOT IN ('user', 'workspace') OR accounts.subject_id <= 0 THEN 'invalid_subject'
	WHEN orders.user_id <= 0 THEN 'invalid_actor'
	WHEN accounts.subject_type = 'user' AND accounts.subject_id <> orders.user_id THEN 'user_subject_order_mismatch'
	ELSE ''
END`

type maintenanceIssueCountRow struct {
	Reason string `gorm:"column:reason"`
	Count  int64  `gorm:"column:count"`
}

// ScanExpiringSubscriptions returns only candidates with a complete persisted
// route. Corrupt rows are aggregated separately so they cannot consume LIMIT.
func (r *MaintenanceRepository) ScanExpiringSubscriptions(
	ctx context.Context,
	now time.Time,
	window time.Duration,
	limit int,
) (domainbilling.MaintenanceSubscriptionBatch, error) {
	candidates, err := r.ListExpiringSubscriptions(ctx, now, window, limit)
	if err != nil {
		return domainbilling.MaintenanceSubscriptionBatch{}, err
	}
	issues, err := r.scanSubscriptionMaintenanceIssues(
		ctx,
		"subscriptions.status = ? AND subscriptions.current_period_end > ? AND subscriptions.current_period_end <= ?",
		[]any{string(domainbilling.SubscriptionStatusActive), now.UTC(), now.UTC().Add(window)},
	)
	if err != nil {
		return domainbilling.MaintenanceSubscriptionBatch{}, err
	}
	return domainbilling.MaintenanceSubscriptionBatch{Candidates: candidates, Issues: issues}, nil
}

func (r *MaintenanceRepository) ScanDueSubscriptions(
	ctx context.Context,
	now time.Time,
	limit int,
) (domainbilling.MaintenanceSubscriptionBatch, error) {
	candidates, err := r.ListDueSubscriptions(ctx, now, limit)
	if err != nil {
		return domainbilling.MaintenanceSubscriptionBatch{}, err
	}
	issues, err := r.scanSubscriptionMaintenanceIssues(
		ctx,
		"subscriptions.status = ? AND subscriptions.current_period_end <= ?",
		[]any{string(domainbilling.SubscriptionStatusActive), now.UTC()},
	)
	if err != nil {
		return domainbilling.MaintenanceSubscriptionBatch{}, err
	}
	return domainbilling.MaintenanceSubscriptionBatch{Candidates: candidates, Issues: issues}, nil
}

func (r *MaintenanceRepository) ScanExpiredOrders(
	ctx context.Context,
	now time.Time,
	limit int,
) (domainbilling.MaintenanceOrderBatch, error) {
	candidates, err := r.ListExpiredOrders(ctx, now, limit)
	if err != nil {
		return domainbilling.MaintenanceOrderBatch{}, err
	}
	issues, err := r.scanOrderMaintenanceIssues(ctx, now.UTC())
	if err != nil {
		return domainbilling.MaintenanceOrderBatch{}, err
	}
	return domainbilling.MaintenanceOrderBatch{Candidates: candidates, Issues: issues}, nil
}

func (r *MaintenanceRepository) scanSubscriptionMaintenanceIssues(
	ctx context.Context,
	scope string,
	args []any,
) ([]domainbilling.MaintenanceIssueSummary, error) {
	rows := make([]maintenanceIssueCountRow, 0, domainbilling.MaxMaintenanceIssueSummaries)
	err := r.db.WithContext(ctx).
		Table("user_subscriptions AS subscriptions").
		Joins("LEFT JOIN billing_accounts AS accounts ON accounts.id = subscriptions.account_id").
		Joins("LEFT JOIN billing_orders AS source_orders ON source_orders.id = subscriptions.source_order_id").
		Select(subscriptionMaintenanceIssueReasonExpression+" AS reason, COUNT(*) AS count").
		Where(scope, args...).
		Where("("+subscriptionMaintenanceIssueReasonExpression+") <> ''").
		Group("reason").
		Order("reason ASC").
		Limit(domainbilling.MaxMaintenanceIssueSummaries).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("aggregate invalid subscription maintenance routes: %w", err)
	}
	return maintenanceIssueSummaries(rows), nil
}

func (r *MaintenanceRepository) scanOrderMaintenanceIssues(
	ctx context.Context,
	now time.Time,
) ([]domainbilling.MaintenanceIssueSummary, error) {
	rows := make([]maintenanceIssueCountRow, 0, domainbilling.MaxMaintenanceIssueSummaries)
	err := r.db.WithContext(ctx).
		Table("billing_orders AS orders").
		Joins("LEFT JOIN billing_accounts AS accounts ON accounts.id = orders.account_id").
		Select(orderMaintenanceIssueReasonExpression+" AS reason, COUNT(*) AS count").
		Where(
			"orders.status = ? AND orders.payment_status = ? AND orders.fulfillment_status = ? AND orders.expires_at <= ?",
			string(domainbilling.OrderStatusPending),
			string(domainbilling.PaymentStatusPending),
			string(domainbilling.FulfillmentStatusPending),
			now,
		).
		Where("("+orderMaintenanceIssueReasonExpression+") <> ''").
		Group("reason").
		Order("reason ASC").
		Limit(domainbilling.MaxMaintenanceIssueSummaries).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("aggregate invalid order maintenance routes: %w", err)
	}
	return maintenanceIssueSummaries(rows), nil
}

func maintenanceIssueSummaries(rows []maintenanceIssueCountRow) []domainbilling.MaintenanceIssueSummary {
	result := make([]domainbilling.MaintenanceIssueSummary, 0, len(rows))
	for _, row := range rows {
		result = append(result, domainbilling.MaintenanceIssueSummary{
			Reason: domainbilling.MaintenanceIssueReason(row.Reason),
			Count:  row.Count,
		})
	}
	return result
}

func (r *MaintenanceRepository) ProjectExpiringSubscription(
	ctx context.Context,
	candidate domainbilling.MaintenanceSubscriptionCandidate,
	now time.Time,
	window time.Duration,
) (domainbilling.MaintenanceProjectionOutcome, error) {
	outcome := domainbilling.MaintenanceProjectionOutcome{}
	if r == nil || r.db == nil || r.idGen == nil || candidate.ID <= 0 || now.IsZero() || window <= 0 {
		return outcome, domainbilling.ErrInvalidInput
	}
	now = now.UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, found, err := lockSubscriptionMaintenanceRow(
			ctx,
			tx,
			candidate.ID,
			"subscriptions.status = ? AND subscriptions.current_period_end > ? AND subscriptions.current_period_end <= ?",
			string(domainbilling.SubscriptionStatusActive),
			now,
			now.Add(window),
		)
		if err != nil {
			return err
		}
		if !found {
			outcome.Stale = true
			return nil
		}
		route, err := billingNotificationRouteFromSubscriptionRow(row)
		if err != nil {
			return err
		}
		if !subscriptionMaintenanceCandidateMatches(row, candidate) {
			outcome.Stale = true
			return nil
		}
		event, err := domainbilling.SubscriptionExpiringNotificationEvent(
			row.ID, row.CurrentPeriodEnd, now, route,
		)
		if err != nil {
			return err
		}
		outcome.NotificationInserted, err =
			r.appendMaintenanceNotificationOutbox(ctx, tx, event)
		return err
	})
	return outcome, err
}

func (r *MaintenanceRepository) ExpireDueSubscriptionProjection(
	ctx context.Context,
	candidate domainbilling.MaintenanceSubscriptionCandidate,
	now time.Time,
) (domainbilling.MaintenanceProjectionOutcome, error) {
	outcome := domainbilling.MaintenanceProjectionOutcome{}
	if r == nil || r.db == nil || r.idGen == nil || candidate.ID <= 0 || now.IsZero() {
		return outcome, domainbilling.ErrInvalidInput
	}
	now = now.UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, found, err := lockSubscriptionMaintenanceRow(
			ctx,
			tx,
			candidate.ID,
			"subscriptions.status = ? AND subscriptions.current_period_end <= ?",
			string(domainbilling.SubscriptionStatusActive),
			now,
		)
		if err != nil {
			return err
		}
		if !found {
			outcome.Stale = true
			return nil
		}
		route, err := billingNotificationRouteFromSubscriptionRow(row)
		if err != nil {
			return err
		}
		if !subscriptionMaintenanceCandidateMatches(row, candidate) {
			outcome.Stale = true
			return nil
		}
		result := tx.WithContext(ctx).Table("user_subscriptions").
			Where(
				"id = ? AND status = ? AND version = ? AND current_period_end = ? AND current_period_end <= ?",
				row.ID,
				string(domainbilling.SubscriptionStatusActive),
				row.Version,
				row.CurrentPeriodEnd,
				now,
			).
			Updates(map[string]any{
				"status":     string(domainbilling.SubscriptionStatusExpired),
				"auto_renew": false,
				"version":    row.Version + 1,
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			outcome.Stale = true
			return nil
		}
		event, err := domainbilling.SubscriptionExpiredNotificationEvent(
			row.ID, row.CurrentPeriodEnd, now, route,
		)
		if err != nil {
			return err
		}
		outcome.NotificationInserted, err =
			r.appendMaintenanceNotificationOutbox(ctx, tx, event)
		if err != nil {
			return err
		}
		outcome.Transitioned = true
		return nil
	})
	return outcome, err
}

func (r *MaintenanceRepository) CloseExpiredOrderProjection(
	ctx context.Context,
	candidate domainbilling.MaintenanceOrderCandidate,
	now time.Time,
) (domainbilling.MaintenanceProjectionOutcome, error) {
	outcome := domainbilling.MaintenanceProjectionOutcome{}
	if r == nil || r.db == nil || r.idGen == nil || candidate.ID <= 0 || now.IsZero() {
		return outcome, domainbilling.ErrInvalidInput
	}
	now = now.UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, found, err := lockExpiredOrderRow(ctx, tx, candidate.ID, now)
		if err != nil {
			return err
		}
		if !found {
			outcome.Stale = true
			return nil
		}
		route, err := billingNotificationRouteFromExpiredOrderRow(row)
		if err != nil {
			return err
		}
		if !expiredOrderCandidateMatches(row, candidate) {
			outcome.Stale = true
			return nil
		}
		result := tx.WithContext(ctx).Table("billing_orders").
			Where(
				"id = ? AND status = ? AND payment_status = ? AND fulfillment_status = ? AND version = ? AND expires_at = ? AND expires_at <= ?",
				row.ID,
				string(domainbilling.OrderStatusPending),
				string(domainbilling.PaymentStatusPending),
				string(domainbilling.FulfillmentStatusPending),
				row.Version,
				row.ExpiresAt,
				now,
			).
			Updates(map[string]any{
				"status":         string(domainbilling.OrderStatusClosed),
				"payment_status": string(domainbilling.PaymentStatusFailed),
				"closed_at":      now,
				"updated_at":     now,
				"version":        row.Version + 1,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			outcome.Stale = true
			return nil
		}
		event, err := domainbilling.OrderTimedOutNotificationEvent(
			row.ID, row.ExpiresAt, now, route,
		)
		if err != nil {
			return err
		}
		outcome.NotificationInserted, err =
			r.appendMaintenanceNotificationOutbox(ctx, tx, event)
		if err != nil {
			return err
		}
		outcome.Transitioned = true
		return nil
	})
	return outcome, err
}

func scanSubscriptionMaintenanceCandidates(query *gorm.DB) ([]DueSubscriptionCandidate, error) {
	var rows []subscriptionMaintenanceRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	candidates := make([]DueSubscriptionCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, subscriptionMaintenanceCandidateFromRow(row))
	}
	return candidates, nil
}

func subscriptionMaintenanceCandidateFromRow(row subscriptionMaintenanceRow) DueSubscriptionCandidate {
	return DueSubscriptionCandidate{
		ID: row.ID, Version: row.Version, AccountID: row.AccountID,
		SourceOrderID: row.SourceOrderID, CurrentPeriodEnd: row.CurrentPeriodEnd,
		Route: domainbilling.BillingNotificationRoute{
			Subject: domainbilling.Subject{
				Type: domainbilling.SubjectType(row.SubjectType), ID: row.SubjectID,
			},
			ActorUserID: row.ActorUserID,
		},
	}
}

func expiredOrderCandidateFromRow(row expiredOrderRow) ExpiredOrderCandidate {
	return ExpiredOrderCandidate{
		ID: row.ID, Version: row.Version, AccountID: row.AccountID,
		ExpiresAt: row.ExpiresAt,
		Route: domainbilling.BillingNotificationRoute{
			Subject: domainbilling.Subject{
				Type: domainbilling.SubjectType(row.SubjectType), ID: row.SubjectID,
			},
			ActorUserID: row.ActorUserID,
		},
	}
}

func billingNotificationRouteFromSubscriptionRow(row subscriptionMaintenanceRow) (domainbilling.BillingNotificationRoute, error) {
	if row.AccountID <= 0 || row.SourceOrderID <= 0 ||
		row.SubjectAccountID != row.AccountID ||
		row.JoinedOrderID != row.SourceOrderID ||
		row.OrderAccountID != row.AccountID {
		return domainbilling.BillingNotificationRoute{}, fmt.Errorf(
			"%w: subscription, account, and source order association is invalid",
			domainnotification.ErrRecipientResolution,
		)
	}
	return domainbilling.NewBillingNotificationRoute(
		domainbilling.SubjectType(row.SubjectType), row.SubjectID, row.ActorUserID,
	)
}

func billingNotificationRouteFromExpiredOrderRow(row expiredOrderRow) (domainbilling.BillingNotificationRoute, error) {
	if row.AccountID <= 0 || row.SubjectAccountID != row.AccountID {
		return domainbilling.BillingNotificationRoute{}, fmt.Errorf(
			"%w: order and billing account association is invalid",
			domainnotification.ErrRecipientResolution,
		)
	}
	return domainbilling.NewBillingNotificationRoute(
		domainbilling.SubjectType(row.SubjectType), row.SubjectID, row.ActorUserID,
	)
}

func lockSubscriptionMaintenanceRow(
	ctx context.Context,
	tx *gorm.DB,
	id int64,
	eligibilityPredicate string,
	eligibilityArgs ...any,
) (subscriptionMaintenanceRow, bool, error) {
	var row subscriptionMaintenanceRow
	err := tx.WithContext(ctx).Table("user_subscriptions AS subscriptions").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select(subscriptionMaintenanceSelect).
		Joins("LEFT JOIN billing_accounts AS accounts ON accounts.id = subscriptions.account_id").
		Joins("LEFT JOIN billing_orders AS orders ON orders.id = subscriptions.source_order_id AND orders.account_id = subscriptions.account_id").
		Where("subscriptions.id = ?", id).
		Where(eligibilityPredicate, eligibilityArgs...).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return subscriptionMaintenanceRow{}, false, nil
	}
	return row, err == nil, err
}

func lockExpiredOrderRow(
	ctx context.Context,
	tx *gorm.DB,
	id int64,
	now time.Time,
) (expiredOrderRow, bool, error) {
	var row expiredOrderRow
	err := tx.WithContext(ctx).Table("billing_orders AS orders").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select(expiredOrderSelect).
		Joins("LEFT JOIN billing_accounts AS accounts ON accounts.id = orders.account_id").
		Where("orders.id = ?", id).
		Where(
			"orders.status = ? AND orders.payment_status = ? AND orders.fulfillment_status = ? AND orders.expires_at <= ?",
			string(domainbilling.OrderStatusPending),
			string(domainbilling.PaymentStatusPending),
			string(domainbilling.FulfillmentStatusPending),
			now,
		).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return expiredOrderRow{}, false, nil
	}
	return row, err == nil, err
}

func subscriptionMaintenanceCandidateMatches(row subscriptionMaintenanceRow, candidate DueSubscriptionCandidate) bool {
	if row.ID != candidate.ID || row.Status != string(domainbilling.SubscriptionStatusActive) ||
		(candidate.AccountID > 0 && row.AccountID != candidate.AccountID) ||
		(candidate.SourceOrderID > 0 && row.SourceOrderID != candidate.SourceOrderID) ||
		candidate.CurrentPeriodEnd.IsZero() || !row.CurrentPeriodEnd.Equal(candidate.CurrentPeriodEnd) {
		return false
	}
	return candidate.Version <= 0 || row.Version == candidate.Version
}

func expiredOrderCandidateMatches(row expiredOrderRow, candidate ExpiredOrderCandidate) bool {
	if row.ID != candidate.ID || row.Status != string(domainbilling.OrderStatusPending) ||
		(candidate.AccountID > 0 && row.AccountID != candidate.AccountID) ||
		row.PaymentStatus != string(domainbilling.PaymentStatusPending) ||
		row.FulfillmentStatus != string(domainbilling.FulfillmentStatusPending) ||
		candidate.ExpiresAt.IsZero() || !row.ExpiresAt.Equal(candidate.ExpiresAt) {
		return false
	}
	return candidate.Version <= 0 || row.Version == candidate.Version
}

func boundedMaintenanceCandidateLimit(limit int) int {
	if limit <= 0 {
		return defaultMaintenanceCandidateLimit
	}
	if limit > maxMaintenanceCandidateLimit {
		return maxMaintenanceCandidateLimit
	}
	return limit
}

func unprojectedExpiringSubscriptionPredicate(db *gorm.DB) string {
	aggregateIDExpression := "CAST(subscriptions.id AS CHAR)"
	projectionVersionExpression := "(TIMESTAMPDIFF(MICROSECOND, '1970-01-01 00:00:00', subscriptions.current_period_end) DIV 1000)"
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "sqlite" {
		aggregateIDExpression = "CAST(subscriptions.id AS TEXT)"
		projectionVersionExpression = "CAST(strftime('%s', subscriptions.current_period_end) AS INTEGER) * 1000 + CAST(substr(strftime('%f', subscriptions.current_period_end), 4, 3) AS INTEGER)"
	}
	return fmt.Sprintf(`NOT EXISTS (
		SELECT 1
		FROM notification_outbox AS projections
		WHERE projections.event_type = ?
		  AND projections.aggregate_type = ?
		  AND projections.aggregate_id = %s
		  AND projections.aggregate_version = %s
	)`, aggregateIDExpression, projectionVersionExpression)
}

func (r *MaintenanceRepository) appendMaintenanceNotificationOutbox(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) (bool, error) {
	return infranotification.NewMySQLRepository(tx, r.idGen).
		AppendInTransactionWithResult(ctx, tx, event)
}

func (r *MaintenanceRepository) AppendCreditAdjustmentNotification(
	ctx context.Context,
	accountID int64,
	businessNo string,
	actorUserID int64,
	occurredAt time.Time,
) error {
	if r == nil ||
		r.db == nil ||
		r.idGen == nil ||
		accountID <= 0 ||
		actorUserID <= 0 ||
		occurredAt.IsZero() {
		return domainbilling.ErrInvalidInput
	}
	var persisted accountPO
	if err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", accountID).
		First(&persisted).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainbilling.ErrNotFound
		}
		return err
	}
	account := persisted.toDomain()
	eventID, err := domainbilling.CreditAdjustmentNotificationEventID(
		account.ID,
		businessNo,
	)
	if err != nil {
		return err
	}
	recipients, found, err := creditAdjustmentRecipientSnapshot(
		ctx,
		r.db,
		eventID,
	)
	if err != nil {
		return err
	}
	if !found {
		recipients, err = billingCreditNotificationRecipients(ctx, r.db, account.Subject)
		if err != nil {
			return err
		}
	}
	event, err := domainbilling.CreditAdjustmentNotificationEvent(
		*account,
		businessNo,
		recipients,
		actorUserID,
		occurredAt,
	)
	if err != nil {
		return err
	}
	return appendCreditNotificationOutbox(ctx, r.db, r.idGen, event)
}

func creditAdjustmentRecipientSnapshot(
	ctx context.Context,
	db *gorm.DB,
	eventID string,
) ([]int64, bool, error) {
	if db == nil || eventID == "" {
		return nil, false, domainbilling.ErrInvalidInput
	}
	var persisted struct {
		PayloadJSON []byte `gorm:"column:payload_json"`
	}
	err := db.WithContext(ctx).
		Table("notification_outbox").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("payload_json").
		Where("event_id = ?", eventID).
		Take(&persisted).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var payload domainnotification.EventPayload
	if err = json.Unmarshal(persisted.PayloadJSON, &payload); err != nil {
		return nil, true, fmt.Errorf(
			"%w: decode persisted credit adjustment recipients",
			domainnotification.ErrStorage,
		)
	}
	recipients, err := domainnotification.NormalizeRecipientIDs(
		payload.ExplicitRecipientIDs,
	)
	if err != nil {
		return nil, true, err
	}
	return recipients, true, nil
}

func (r *MySQLRepository) ApplyCreditThreshold(
	ctx context.Context,
	evaluation domainbilling.CreditThresholdEvaluation,
) error {
	if r == nil || r.db == nil || r.idGen == nil {
		return domainnotification.ErrStorage
	}
	if err := evaluation.Validate(); err != nil {
		return err
	}
	config, err := r.resolveCreditThresholdConfig(ctx, evaluation.Account.Subject)
	if err != nil {
		return err
	}
	current, found, err := r.lockCreditThresholdEpisode(ctx, evaluation.Account.ID)
	if err != nil {
		return err
	}
	decision, err := domainbilling.EvaluateCreditThreshold(
		config,
		evaluation.Account.ID,
		evaluation.SettledBalanceBeforeMicros,
		evaluation.SettledBalanceAfterMicros,
		current,
		evaluation.OccurredAt,
	)
	if err != nil {
		return err
	}
	if !decision.Changed {
		return nil
	}
	if decision.Episode == nil {
		return domainbilling.ErrVersionConflict
	}
	if !found {
		if err = r.db.WithContext(ctx).
			Create(creditThresholdEpisodePOFromDomain(decision.Episode)).Error; err != nil {
			return err
		}
	} else {
		result := r.db.WithContext(ctx).
			Model(&creditThresholdEpisodePO{}).
			Where(
				"account_id = ? AND version = ?",
				current.AccountID,
				current.Version,
			).
			Updates(map[string]any{
				"episode_no":             decision.Episode.EpisodeNo,
				"active":                 decision.Episode.Active,
				"threshold_micros":       decision.Episode.ThresholdMicros,
				"recovery_margin_micros": decision.Episode.RecoveryMarginMicros,
				"version":                decision.Episode.Version,
				"opened_at":              decision.Episode.OpenedAt,
				"recovered_at":           decision.Episode.RecoveredAt,
				"updated_at":             decision.Episode.UpdatedAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainbilling.ErrVersionConflict
		}
	}
	if !decision.Notify {
		return nil
	}
	recipients, err := billingCreditNotificationRecipients(
		ctx,
		r.db,
		evaluation.Account.Subject,
	)
	if err != nil {
		return err
	}
	event, err := domainbilling.CreditLowNotificationEvent(
		evaluation.Account,
		*decision.Episode,
		recipients,
		evaluation.ActorUserID,
	)
	if err != nil {
		return err
	}
	return appendCreditNotificationOutbox(ctx, r.db, r.idGen, event)
}

func (r *MySQLRepository) resolveCreditThresholdConfig(
	ctx context.Context,
	subject domainbilling.Subject,
) (domainbilling.CreditThresholdConfig, error) {
	if subject.Validate() != nil {
		return domainbilling.CreditThresholdConfig{}, domainbilling.ErrInvalidInput
	}
	for _, subjectID := range []int64{subject.ID, 0} {
		var persisted creditThresholdConfigPO
		err := r.db.WithContext(ctx).
			Where(
				"subject_type = ? AND subject_id = ?",
				string(subject.Type),
				subjectID,
			).
			First(&persisted).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return domainbilling.CreditThresholdConfig{}, err
		}
		config := domainbilling.CreditThresholdConfig{
			Enabled:              persisted.Enabled,
			ThresholdMicros:      persisted.ThresholdMicros,
			RecoveryMarginMicros: persisted.RecoveryMarginMicros,
		}
		if err = config.Validate(); err != nil {
			return domainbilling.CreditThresholdConfig{}, err
		}
		return config, nil
	}
	return domainbilling.DefaultCreditThresholdConfig(), nil
}

func (r *MySQLRepository) lockCreditThresholdEpisode(
	ctx context.Context,
	accountID int64,
) (*domainbilling.CreditThresholdEpisode, bool, error) {
	var persisted creditThresholdEpisodePO
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ?", accountID).
		First(&persisted).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	episode := persisted.toDomain()
	if err = episode.Validate(); err != nil {
		return nil, false, err
	}
	return episode, true, nil
}

func billingCreditNotificationRecipients(
	ctx context.Context,
	db *gorm.DB,
	subject domainbilling.Subject,
) ([]int64, error) {
	if db == nil || subject.Validate() != nil {
		return nil, domainbilling.ErrInvalidInput
	}
	if subject.Type == domainbilling.SubjectTypeUser {
		return domainnotification.NormalizeRecipientIDs([]int64{subject.ID})
	}
	var recipients []int64
	if err := db.WithContext(ctx).
		Table("space_user").
		Distinct("user_id").
		Where("space_id = ? AND role_type IN ?", subject.ID, []int32{1, 2}).
		Order("user_id ASC").
		Pluck("user_id", &recipients).Error; err != nil {
		return nil, err
	}
	normalized, err := domainnotification.NormalizeRecipientIDs(recipients)
	if err != nil {
		return nil, domainBillingCreditRouteError(
			"workspace billing account has no persisted owner or administrator",
		)
	}
	return normalized, nil
}

func appendCreditNotificationOutbox(
	ctx context.Context,
	tx *gorm.DB,
	idGenerator idgen.IDGenerator,
	event domainnotification.Event,
) error {
	event, err := domainnotification.CanonicalizeEvent(event)
	if err != nil {
		return err
	}
	if err = domainnotification.DefaultTemplateRegistry().ValidateAppendable(event); err != nil {
		return err
	}
	return infranotification.NewMySQLRepository(tx, idGenerator).
		AppendInTransaction(ctx, tx, event)
}

func domainBillingCreditRouteError(message string) error {
	return fmt.Errorf(
		"%w: %s",
		domainnotification.ErrRecipientResolution,
		message,
	)
}

func creditThresholdEpisodePOFromDomain(
	episode *domainbilling.CreditThresholdEpisode,
) *creditThresholdEpisodePO {
	return &creditThresholdEpisodePO{
		AccountID:            episode.AccountID,
		EpisodeNo:            episode.EpisodeNo,
		Active:               episode.Active,
		ThresholdMicros:      episode.ThresholdMicros,
		RecoveryMarginMicros: episode.RecoveryMarginMicros,
		Version:              episode.Version,
		OpenedAt:             episode.OpenedAt,
		RecoveredAt:          episode.RecoveredAt,
		CreatedAt:            episode.CreatedAt,
		UpdatedAt:            episode.UpdatedAt,
	}
}

func (p creditThresholdEpisodePO) toDomain() *domainbilling.CreditThresholdEpisode {
	return &domainbilling.CreditThresholdEpisode{
		AccountID:            p.AccountID,
		EpisodeNo:            p.EpisodeNo,
		Active:               p.Active,
		ThresholdMicros:      p.ThresholdMicros,
		RecoveryMarginMicros: p.RecoveryMarginMicros,
		Version:              p.Version,
		OpenedAt:             p.OpenedAt,
		RecoveredAt:          p.RecoveredAt,
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
	}
}

func (r *MaintenanceRepository) ListAccountSubjectsAfter(
	ctx context.Context,
	afterAccountID int64,
	limit int,
) ([]AccountSubjectCandidate, error) {
	if limit <= 0 {
		limit = 200
	}
	var rows []AccountSubjectCandidate
	err := r.db.WithContext(ctx).Table("billing_accounts").
		Select("id AS account_id, subject_type, subject_id").
		Where("id > ?", afterAccountID).
		Order("id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

func (r *MaintenanceRepository) ReconcileAccounts(ctx context.Context, now time.Time) (*ReconciliationResult, error) {
	var checked int64
	if err := r.db.WithContext(ctx).Table("billing_accounts").Count(&checked).Error; err != nil {
		return nil, err
	}
	var drifts []AccountDrift
	err := r.db.WithContext(ctx).Raw(`
		SELECT accounts.id AS account_id,
		       accounts.available_micros,
		       COALESCE(batches.expected_available, 0) AS expected_available,
		       accounts.reserved_micros,
		       COALESCE(reservations.expected_reserved, 0) AS expected_reserved
		FROM billing_accounts AS accounts
		LEFT JOIN (
			SELECT account_id, SUM(remaining_micros) AS expected_available
			FROM credit_batches
			WHERE expires_at IS NULL OR expires_at > ?
			GROUP BY account_id
		) AS batches ON batches.account_id = accounts.id
		LEFT JOIN (
			SELECT account_id, SUM(reserved_micros - settled_micros - released_micros) AS expected_reserved
			FROM credit_reservations
			WHERE status = ? AND expires_at > ?
			GROUP BY account_id
		) AS reservations ON reservations.account_id = accounts.id
		WHERE accounts.available_micros <> COALESCE(batches.expected_available, 0)
		   OR accounts.reserved_micros <> COALESCE(reservations.expected_reserved, 0)
		ORDER BY accounts.id ASC
		LIMIT 500`, now, string(domainbilling.ReservationStatusReserved), now).Scan(&drifts).Error
	if err != nil {
		return nil, err
	}
	runID, err := r.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	result := &ReconciliationResult{RunID: runID, CheckedCount: checked, MismatchCount: int64(len(drifts)), Mismatches: drifts}
	summary, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	row := map[string]any{
		"id": runID, "scope": "accounts", "status": "completed", "started_at": now,
		"completed_at": now, "checked_count": checked, "mismatch_count": len(drifts),
		"summary_json": string(summary),
	}
	if err = r.db.WithContext(ctx).Table("billing_reconciliation_runs").Create(row).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func (r *AdminRepository) CreateAuditLog(
	ctx context.Context,
	accountID, actorUserID int64,
	action, businessNo, summaryJSON string,
) error {
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	row := auditLogPO{
		ID: id, AccountID: accountID, ActorUserID: actorUserID,
		Action: action, BusinessNo: businessNo, SummaryJSON: summaryJSON,
		CreatedAt: time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

type auditLogPO struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	AccountID   int64     `gorm:"column:account_id"`
	ActorUserID int64     `gorm:"column:actor_user_id"`
	Action      string    `gorm:"column:action;uniqueIndex:uk_billing_audit_business_action,priority:2"`
	BusinessNo  string    `gorm:"column:business_no;uniqueIndex:uk_billing_audit_business_action,priority:1"`
	SummaryJSON string    `gorm:"column:summary_json"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (auditLogPO) TableName() string { return "billing_audit_logs" }
