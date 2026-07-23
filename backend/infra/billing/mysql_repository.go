// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type MySQLRepository struct {
	db    *gorm.DB
	idGen idgen.IDGenerator
}

func NewMySQLRepository(db *gorm.DB, idGenerator idgen.IDGenerator) *MySQLRepository {
	return &MySQLRepository{db: db, idGen: idGenerator}
}

func (r *MySQLRepository) RunInTransaction(ctx context.Context, fn func(domainbilling.Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&MySQLRepository{db: tx, idGen: r.idGen})
	})
}

func (r *MySQLRepository) GetOrCreateAccountForUpdate(ctx context.Context, subject domainbilling.Subject, now time.Time) (*domainbilling.Account, error) {
	var po accountPO
	lookup := r.db.WithContext(ctx).
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		Limit(1).
		Find(&po)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected == 0 {
		id, err := r.idGen.GenID(ctx)
		if err != nil {
			return nil, err
		}
		candidate := accountPO{
			ID:              id,
			SubjectType:     string(subject.Type),
			SubjectID:       subject.ID,
			AvailableMicros: 0,
			ReservedMicros:  0,
			Version:         domainbilling.InitialVersion,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "subject_type"}, {Name: "subject_id"}},
			DoNothing: true,
		}).Create(&candidate).Error; err != nil {
			return nil, err
		}
	}
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		First(&po).Error
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) GetAccountByIDForUpdate(ctx context.Context, accountID int64) (*domainbilling.Account, error) {
	var po accountPO
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&po, "id = ?", accountID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) UpdateAccount(ctx context.Context, account *domainbilling.Account, expectedVersion int64) error {
	result := r.db.WithContext(ctx).Model(&accountPO{}).
		Where("id = ? AND version = ?", account.ID, expectedVersion).
		Updates(map[string]any{
			"available_micros": account.AvailableMicros,
			"reserved_micros":  account.ReservedMicros,
			"version":          expectedVersion + 1,
			"updated_at":       account.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainbilling.ErrVersionConflict
	}
	account.Version = expectedVersion + 1
	return nil
}

func (r *MySQLRepository) CreateBatch(ctx context.Context, batch *domainbilling.CreditBatch) error {
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	batch.ID = id
	return r.db.WithContext(ctx).Create(batchPOFromDomain(batch)).Error
}

func (r *MySQLRepository) GetBatchByIDForUpdate(ctx context.Context, batchID int64) (*domainbilling.CreditBatch, error) {
	var po batchPO
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&po, "id = ?", batchID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) ListSpendableBatchesForUpdate(ctx context.Context, accountID int64, now time.Time) ([]*domainbilling.CreditBatch, error) {
	var pos []batchPO
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ? AND remaining_micros > 0 AND (expires_at IS NULL OR expires_at > ?)", accountID, now).
		Order("expires_at IS NULL ASC, expires_at ASC, id ASC").Find(&pos).Error
	return batchesToDomain(pos, err)
}

func (r *MySQLRepository) ListExpiredBatchesForUpdate(ctx context.Context, accountID int64, now time.Time) ([]*domainbilling.CreditBatch, error) {
	var pos []batchPO
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ? AND remaining_micros > 0 AND expires_at IS NOT NULL AND expires_at <= ?", accountID, now).
		Order("expires_at ASC, id ASC").Find(&pos).Error
	return batchesToDomain(pos, err)
}

func (r *MySQLRepository) UpdateBatch(ctx context.Context, batch *domainbilling.CreditBatch, expectedVersion int64) error {
	result := r.db.WithContext(ctx).Model(&batchPO{}).
		Where("id = ? AND version = ?", batch.ID, expectedVersion).
		Updates(map[string]any{
			"remaining_micros": batch.RemainingMicros,
			"version":          expectedVersion + 1,
			"updated_at":       batch.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainbilling.ErrVersionConflict
	}
	batch.Version = expectedVersion + 1
	return nil
}

func (r *MySQLRepository) FindLedgerByBusinessNo(ctx context.Context, businessNo string) (*domainbilling.LedgerEntry, error) {
	var po ledgerPO
	err := r.db.WithContext(ctx).Where("business_no = ?", businessNo).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) CreateLedger(ctx context.Context, entry *domainbilling.LedgerEntry) error {
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	entry.ID = id
	return r.db.WithContext(ctx).Create(ledgerPOFromDomain(entry)).Error
}

func (r *MySQLRepository) FindReservationByReserveBusinessNoForUpdate(ctx context.Context, businessNo string) (*domainbilling.Reservation, error) {
	var po reservationPO
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("reserve_business_no = ?", businessNo).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain()
}

func (r *MySQLRepository) CreateReservation(ctx context.Context, reservation *domainbilling.Reservation) error {
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	reservation.ID = id
	po, err := reservationPOFromDomain(reservation)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(po).Error
}

func (r *MySQLRepository) UpdateReservation(ctx context.Context, reservation *domainbilling.Reservation, expectedVersion int64) error {
	allocationsJSON, err := json.Marshal(reservation.Allocations)
	if err != nil {
		return err
	}
	result := r.db.WithContext(ctx).Model(&reservationPO{}).
		Where("id = ? AND version = ?", reservation.ID, expectedVersion).
		Updates(map[string]any{
			"settlement_business_no": nullableString(reservation.SettlementBusinessNo),
			"release_business_no":    nullableString(reservation.ReleaseBusinessNo),
			"settled_micros":         reservation.SettledMicros,
			"released_micros":        reservation.ReleasedMicros,
			"status":                 string(reservation.Status),
			"allocations_json":       string(allocationsJSON),
			"expires_at":             reservation.ExpiresAt,
			"version":                expectedVersion + 1,
			"updated_at":             reservation.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domainbilling.ErrVersionConflict
	}
	reservation.Version = expectedVersion + 1
	return nil
}

type accountPO struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	SubjectType     string    `gorm:"column:subject_type;uniqueIndex:uk_billing_account_subject,priority:1"`
	SubjectID       int64     `gorm:"column:subject_id;uniqueIndex:uk_billing_account_subject,priority:2"`
	AvailableMicros int64     `gorm:"column:available_micros"`
	ReservedMicros  int64     `gorm:"column:reserved_micros"`
	Version         int64     `gorm:"column:version"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (accountPO) TableName() string { return "billing_accounts" }

func (p accountPO) toDomain() *domainbilling.Account {
	return &domainbilling.Account{
		ID:              p.ID,
		Subject:         domainbilling.Subject{Type: domainbilling.SubjectType(p.SubjectType), ID: p.SubjectID},
		AvailableMicros: p.AvailableMicros,
		ReservedMicros:  p.ReservedMicros,
		Version:         p.Version,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

type batchPO struct {
	ID              int64      `gorm:"column:id;primaryKey"`
	AccountID       int64      `gorm:"column:account_id"`
	SourceType      string     `gorm:"column:source_type"`
	SourceID        string     `gorm:"column:source_id"`
	GrantBusinessNo string     `gorm:"column:grant_business_no"`
	GrantedMicros   int64      `gorm:"column:granted_micros"`
	RemainingMicros int64      `gorm:"column:remaining_micros"`
	ExpiresAt       *time.Time `gorm:"column:expires_at"`
	Version         int64      `gorm:"column:version"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (batchPO) TableName() string { return "credit_batches" }

func batchPOFromDomain(batch *domainbilling.CreditBatch) *batchPO {
	return &batchPO{
		ID: batch.ID, AccountID: batch.AccountID, SourceType: batch.SourceType, SourceID: batch.SourceID,
		GrantBusinessNo: batch.GrantBusinessNo, GrantedMicros: batch.GrantedMicros,
		RemainingMicros: batch.RemainingMicros, ExpiresAt: batch.ExpiresAt, Version: batch.Version,
		CreatedAt: batch.CreatedAt, UpdatedAt: batch.UpdatedAt,
	}
}

func (p batchPO) toDomain() *domainbilling.CreditBatch {
	return &domainbilling.CreditBatch{
		ID: p.ID, AccountID: p.AccountID, SourceType: p.SourceType, SourceID: p.SourceID,
		GrantBusinessNo: p.GrantBusinessNo, GrantedMicros: p.GrantedMicros,
		RemainingMicros: p.RemainingMicros, ExpiresAt: p.ExpiresAt, Version: p.Version,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func batchesToDomain(pos []batchPO, err error) ([]*domainbilling.CreditBatch, error) {
	if err != nil {
		return nil, err
	}
	result := make([]*domainbilling.CreditBatch, 0, len(pos))
	for index := range pos {
		result = append(result, pos[index].toDomain())
	}
	return result, nil
}

type ledgerPO struct {
	ID                   int64     `gorm:"column:id;primaryKey"`
	AccountID            int64     `gorm:"column:account_id"`
	BatchID              *int64    `gorm:"column:batch_id"`
	Direction            string    `gorm:"column:direction"`
	EntryType            string    `gorm:"column:entry_type"`
	AmountMicros         int64     `gorm:"column:amount_micros"`
	AvailableAfterMicros int64     `gorm:"column:available_after_micros"`
	ReservedAfterMicros  int64     `gorm:"column:reserved_after_micros"`
	BusinessNo           string    `gorm:"column:business_no"`
	ActorUserID          int64     `gorm:"column:actor_user_id"`
	MetadataJSON         string    `gorm:"column:metadata_json"`
	CreatedAt            time.Time `gorm:"column:created_at"`
}

func (ledgerPO) TableName() string { return "credit_ledger_entries" }

func ledgerPOFromDomain(entry *domainbilling.LedgerEntry) *ledgerPO {
	return &ledgerPO{
		ID: entry.ID, AccountID: entry.AccountID, BatchID: entry.BatchID,
		Direction: string(entry.Direction), EntryType: string(entry.Type), AmountMicros: entry.AmountMicros,
		AvailableAfterMicros: entry.AvailableAfterMicros, ReservedAfterMicros: entry.ReservedAfterMicros,
		BusinessNo: entry.BusinessNo, ActorUserID: entry.ActorUserID, MetadataJSON: entry.MetadataJSON,
		CreatedAt: entry.CreatedAt,
	}
}

func (p ledgerPO) toDomain() *domainbilling.LedgerEntry {
	return &domainbilling.LedgerEntry{
		ID: p.ID, AccountID: p.AccountID, BatchID: p.BatchID,
		Direction: domainbilling.LedgerDirection(p.Direction), Type: domainbilling.LedgerEntryType(p.EntryType),
		AmountMicros: p.AmountMicros, AvailableAfterMicros: p.AvailableAfterMicros,
		ReservedAfterMicros: p.ReservedAfterMicros, BusinessNo: p.BusinessNo,
		ActorUserID: p.ActorUserID, MetadataJSON: p.MetadataJSON, CreatedAt: p.CreatedAt,
	}
}

type reservationPO struct {
	ID                   int64     `gorm:"column:id;primaryKey"`
	AccountID            int64     `gorm:"column:account_id"`
	ReserveBusinessNo    string    `gorm:"column:reserve_business_no"`
	SettlementBusinessNo *string   `gorm:"column:settlement_business_no"`
	ReleaseBusinessNo    *string   `gorm:"column:release_business_no"`
	ReservedMicros       int64     `gorm:"column:reserved_micros"`
	SettledMicros        int64     `gorm:"column:settled_micros"`
	ReleasedMicros       int64     `gorm:"column:released_micros"`
	Status               string    `gorm:"column:status"`
	AllocationsJSON      string    `gorm:"column:allocations_json"`
	ExpiresAt            time.Time `gorm:"column:expires_at"`
	Version              int64     `gorm:"column:version"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
}

func (reservationPO) TableName() string { return "credit_reservations" }

func reservationPOFromDomain(reservation *domainbilling.Reservation) (*reservationPO, error) {
	allocationsJSON, err := json.Marshal(reservation.Allocations)
	if err != nil {
		return nil, err
	}
	return &reservationPO{
		ID: reservation.ID, AccountID: reservation.AccountID, ReserveBusinessNo: reservation.ReserveBusinessNo,
		SettlementBusinessNo: nullableString(reservation.SettlementBusinessNo), ReleaseBusinessNo: nullableString(reservation.ReleaseBusinessNo),
		ReservedMicros: reservation.ReservedMicros, SettledMicros: reservation.SettledMicros,
		ReleasedMicros: reservation.ReleasedMicros, Status: string(reservation.Status),
		AllocationsJSON: string(allocationsJSON), ExpiresAt: reservation.ExpiresAt,
		Version: reservation.Version, CreatedAt: reservation.CreatedAt, UpdatedAt: reservation.UpdatedAt,
	}, nil
}

func (p reservationPO) toDomain() (*domainbilling.Reservation, error) {
	var allocations []domainbilling.Allocation
	if err := json.Unmarshal([]byte(p.AllocationsJSON), &allocations); err != nil {
		return nil, err
	}
	return &domainbilling.Reservation{
		ID: p.ID, AccountID: p.AccountID, ReserveBusinessNo: p.ReserveBusinessNo,
		SettlementBusinessNo: dereferenceString(p.SettlementBusinessNo), ReleaseBusinessNo: dereferenceString(p.ReleaseBusinessNo),
		ReservedMicros: p.ReservedMicros, SettledMicros: p.SettledMicros, ReleasedMicros: p.ReleasedMicros,
		Status: domainbilling.ReservationStatus(p.Status), Allocations: allocations, ExpiresAt: p.ExpiresAt,
		Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}, nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
