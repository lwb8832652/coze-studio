// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type MaintenanceRepository struct {
	db    *gorm.DB
	idGen idgen.IDGenerator
}

type ExpiredReservationCandidate struct {
	ID                int64
	ReserveBusinessNo string
}

type DueSubscriptionCandidate struct {
	ID               int64
	CurrentPeriodEnd time.Time
}

type AccountSubjectCandidate struct {
	AccountID   int64
	SubjectType string
	SubjectID   int64
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
	if limit <= 0 {
		limit = 200
	}
	var rows []DueSubscriptionCandidate
	err := r.db.WithContext(ctx).Table("user_subscriptions AS subscriptions").
		Select("subscriptions.id, subscriptions.current_period_end").
		Where("subscriptions.status = ? AND subscriptions.current_period_end <= ?", string(domainbilling.SubscriptionStatusActive), now).
		Order("subscriptions.current_period_end ASC, subscriptions.id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}

func (r *MaintenanceRepository) ExpireSubscription(
	ctx context.Context,
	candidate DueSubscriptionCandidate,
) error {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Table("user_subscriptions").
		Where("id = ? AND status = ? AND current_period_end = ?", candidate.ID, string(domainbilling.SubscriptionStatusActive), candidate.CurrentPeriodEnd).
		Updates(map[string]any{
			"status":     string(domainbilling.SubscriptionStatusExpired),
			"auto_renew": false,
			"version":    gorm.Expr("version + 1"),
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainbilling.ErrVersionConflict
	}
	return nil
}

func (r *MaintenanceRepository) CloseExpiredOrders(ctx context.Context, now time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Table("billing_orders").
		Where("status = ? AND expires_at <= ?", string(domainbilling.OrderStatusPending), now).
		Updates(map[string]any{
			"status":         string(domainbilling.OrderStatusClosed),
			"payment_status": string(domainbilling.PaymentStatusFailed),
			"closed_at":      now, "updated_at": now,
			"version": gorm.Expr("version + 1"),
		})
	return result.RowsAffected, result.Error
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
