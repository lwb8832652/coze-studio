// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type AdminRepository struct {
	db    *gorm.DB
	idGen idgen.IDGenerator
}

func NewAdminRepository(db *gorm.DB, idGenerator idgen.IDGenerator) *AdminRepository {
	return &AdminRepository{db: db, idGen: idGenerator}
}

type BillingConfig struct {
	CreditName        string `json:"credit_name"`
	DisplayScale      int    `json:"display_scale"`
	AllowNegative     bool   `json:"allow_negative"`
	SettlementEnabled bool   `json:"settlement_enabled"`
	PaymentEnabled    bool   `json:"payment_enabled"`
	DefaultCurrency   string `json:"default_currency"`
	Version           int64  `json:"version"`
}
type AdminOverview struct{ Accounts, Plans, Packages, PendingOrders, UsageRecords int64 }
type CreatePlanInput struct {
	Key, Name, Description         string
	Cycle                          domainbilling.BillingCycle
	PriceMicros, CreditGrantMicros int64
	Currency, FeaturesJSON         string
	SortOrder                      int
	ActorUserID                    int64
}
type CreatePackageInput struct {
	Key, Name, Description                 string
	PriceMicros, CreditMicros              int64
	Currency                               string
	ValidityDays, PurchaseLimit, SortOrder int
	ActorUserID                            int64
}
type AdminPlanView struct {
	ID                int64     `json:"id,string"`
	Key               string    `gorm:"column:plan_key" json:"key"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Status            string    `json:"status"`
	SortOrder         int       `json:"sort_order"`
	CurrentVersion    int       `json:"current_version"`
	Cycle             string    `json:"cycle"`
	PriceMicros       int64     `json:"price_micros"`
	Currency          string    `json:"currency"`
	CreditGrantMicros int64     `json:"credit_grant_micros"`
	FeaturesJSON      string    `json:"features_json"`
	EffectiveAt       time.Time `json:"effective_at"`
}
type CreatePriceInput struct {
	Provider, ModelID, Currency                             string
	InputPrice, OutputPrice, CacheWritePrice, CacheHitPrice int64
	EffectiveAt                                             time.Time
	ActorUserID                                             int64
}
type AccountView struct {
	ID              int64     `json:"id,string"`
	SubjectType     string    `json:"subject_type"`
	SubjectID       int64     `json:"subject_id,string"`
	AvailableMicros int64     `json:"available_micros"`
	ReservedMicros  int64     `json:"reserved_micros"`
	Version         int64     `json:"version"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type LedgerView struct {
	ID                                                      int64 `json:"id,string"`
	AccountID                                               int64 `json:"account_id,string"`
	Direction, EntryType                                    string
	AmountMicros, AvailableAfterMicros, ReservedAfterMicros int64
	BusinessNo                                              string
	ActorUserID                                             int64
	CreatedAt                                               time.Time
}
type OrderView struct {
	ID                                                  int64 `json:"id,string"`
	OrderNo                                             string
	UserID, AccountID                                   int64
	OrderType, Status, PaymentStatus, FulfillmentStatus string
	TotalMicros                                         int64
	Currency                                            string
	CreatedAt                                           time.Time
}
type PriceView struct {
	ID                                                      int64 `json:"id,string"`
	Provider, ModelID                                       string
	Version                                                 int
	Currency                                                string
	InputPrice, OutputPrice, CacheWritePrice, CacheHitPrice int64
	Status                                                  string
	EffectiveAt                                             time.Time
}
type UsageMonitorView struct {
	Provider         string    `json:"provider"`
	ModelID          string    `json:"model_id"`
	Calls            int64     `json:"calls"`
	InputTokens      int64     `json:"input_tokens"`
	OutputTokens     int64     `json:"output_tokens"`
	CacheWriteTokens int64     `json:"cache_write_tokens"`
	CacheHitTokens   int64     `json:"cache_hit_tokens"`
	ChargeMicros     int64     `json:"charge_micros"`
	LastUsedAt       time.Time `json:"last_used_at"`
}

type CreditThresholdConfigView struct {
	SubjectType          domainbilling.SubjectType `json:"subject_type"`
	SubjectID            int64                     `json:"subject_id,string"`
	SourceSubjectID      int64                     `json:"source_subject_id,string"`
	Inherited            bool                      `json:"inherited"`
	Enabled              bool                      `json:"enabled"`
	ThresholdMicros      int64                     `json:"threshold_micros"`
	RecoveryMarginMicros int64                     `json:"recovery_margin_micros"`
	Version              int64                     `json:"version"`
	SourceVersion        int64                     `json:"source_version"`
	UpdatedAt            time.Time                 `json:"updated_at"`
}

type SaveCreditThresholdConfigInput struct {
	Subject              domainbilling.Subject `json:"-"`
	Enabled              bool                  `json:"enabled"`
	ThresholdMicros      int64                 `json:"threshold_micros"`
	RecoveryMarginMicros int64                 `json:"recovery_margin_micros"`
	ExpectedVersion      int64                 `json:"expected_version"`
}

func (r *AdminRepository) Overview(ctx context.Context) (*AdminOverview, error) {
	result := &AdminOverview{}
	queries := []struct {
		table  string
		where  string
		target *int64
	}{{"billing_accounts", "", &result.Accounts}, {"subscription_plans", "deleted_at IS NULL", &result.Plans}, {"credit_packages", "deleted_at IS NULL", &result.Packages}, {"billing_orders", "status = 'pending_payment'", &result.PendingOrders}, {"model_usage_records", "", &result.UsageRecords}}
	for _, query := range queries {
		db := r.db.WithContext(ctx).Table(query.table)
		if query.where != "" {
			db = db.Where(query.where)
		}
		if err := db.Count(query.target).Error; err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *AdminRepository) GetConfig(ctx context.Context) (*BillingConfig, error) {
	var row struct {
		CreditName                                       string
		DisplayScale                                     int
		AllowNegative, SettlementEnabled, PaymentEnabled bool
		DefaultCurrency                                  string
		Version                                          int64
	}
	err := r.db.WithContext(ctx).Table("billing_system_config").Where("id = 1").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &BillingConfig{CreditName: "积分", DisplayScale: 2, DefaultCurrency: "CNY", Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	return &BillingConfig{CreditName: row.CreditName, DisplayScale: row.DisplayScale, AllowNegative: row.AllowNegative, SettlementEnabled: row.SettlementEnabled, PaymentEnabled: row.PaymentEnabled, DefaultCurrency: row.DefaultCurrency, Version: row.Version}, nil
}

func (r *AdminRepository) SaveConfig(ctx context.Context, config BillingConfig, actorUserID int64) (*BillingConfig, error) {
	if strings.TrimSpace(config.CreditName) == "" || config.DisplayScale < 0 || config.DisplayScale > 6 || actorUserID <= 0 {
		return nil, domainbilling.ErrInvalidInput
	}
	result := r.db.WithContext(ctx).Table("billing_system_config").Where("id = 1 AND version = ?", config.Version).Updates(map[string]any{"credit_name": strings.TrimSpace(config.CreditName), "display_scale": config.DisplayScale, "allow_negative": config.AllowNegative, "settlement_enabled": config.SettlementEnabled, "payment_enabled": config.PaymentEnabled, "default_currency": strings.ToUpper(config.DefaultCurrency), "updated_by": actorUserID, "version": config.Version + 1, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, domainbilling.ErrVersionConflict
	}
	return r.GetConfig(ctx)
}

func (r *AdminRepository) GetCreditThresholdConfig(
	ctx context.Context,
	subject domainbilling.Subject,
) (*CreditThresholdConfigView, error) {
	if r == nil || r.db == nil || validateAdminCreditThresholdSubject(subject) != nil {
		return nil, domainbilling.ErrInvalidInput
	}
	var row creditThresholdConfigPO
	err := r.db.WithContext(ctx).
		Where("subject_type = ? AND subject_id = ?", string(subject.Type), subject.ID).
		Take(&row).Error
	inherited := false
	if errors.Is(err, gorm.ErrRecordNotFound) && subject.ID > 0 {
		err = r.db.WithContext(ctx).
			Where("subject_type = ? AND subject_id = 0", string(subject.Type)).
			Take(&row).Error
		inherited = err == nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return creditThresholdConfigView(subject, row, inherited), nil
}

func (r *AdminRepository) SaveCreditThresholdConfig(
	ctx context.Context,
	input SaveCreditThresholdConfigInput,
	actorUserID int64,
) (*CreditThresholdConfigView, error) {
	config := domainbilling.CreditThresholdConfig{
		Enabled:              input.Enabled,
		ThresholdMicros:      input.ThresholdMicros,
		RecoveryMarginMicros: input.RecoveryMarginMicros,
	}
	if r == nil || r.db == nil ||
		actorUserID <= 0 ||
		input.ExpectedVersion < 0 ||
		validateAdminCreditThresholdSubject(input.Subject) != nil ||
		config.Validate() != nil {
		return nil, domainbilling.ErrInvalidInput
	}
	now := time.Now().UTC()
	var view *CreditThresholdConfigView
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.ExpectedVersion == 0 {
			row := creditThresholdConfigPO{
				SubjectType:          string(input.Subject.Type),
				SubjectID:            input.Subject.ID,
				Enabled:              config.Enabled,
				ThresholdMicros:      config.ThresholdMicros,
				RecoveryMarginMicros: config.RecoveryMarginMicros,
				Version:              domainbilling.InitialVersion,
				UpdatedBy:            actorUserID,
				CreatedAt:            now,
				UpdatedAt:            now,
			}
			if createErr := createInitialCreditThresholdConfigCAS(tx, &row); createErr != nil {
				return createErr
			}
			view = creditThresholdConfigView(input.Subject, row, false)
			return nil
		}

		var row creditThresholdConfigPO
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"subject_type = ? AND subject_id = ?",
				string(input.Subject.Type),
				input.Subject.ID,
			).
			Take(&row).Error
		switch {
		case errors.Is(findErr, gorm.ErrRecordNotFound):
			return domainbilling.ErrVersionConflict
		case findErr != nil:
			return findErr
		}
		if row.Version != input.ExpectedVersion {
			return domainbilling.ErrVersionConflict
		}
		nextVersion := row.Version + 1
		result := tx.Model(&creditThresholdConfigPO{}).
			Where(
				"subject_type = ? AND subject_id = ? AND version = ?",
				row.SubjectType,
				row.SubjectID,
				row.Version,
			).
			Updates(map[string]any{
				"enabled":                config.Enabled,
				"threshold_micros":       config.ThresholdMicros,
				"recovery_margin_micros": config.RecoveryMarginMicros,
				"version":                nextVersion,
				"updated_by":             actorUserID,
				"updated_at":             now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domainbilling.ErrVersionConflict
		}
		row.Enabled = config.Enabled
		row.ThresholdMicros = config.ThresholdMicros
		row.RecoveryMarginMicros = config.RecoveryMarginMicros
		row.Version = nextVersion
		row.UpdatedBy = actorUserID
		row.UpdatedAt = now
		view = creditThresholdConfigView(input.Subject, row, false)
		return nil
	})
	if err != nil {
		return nil, normalizeCreditThresholdConfigWriteError(err)
	}
	return view, nil
}

func createInitialCreditThresholdConfigCAS(
	tx *gorm.DB,
	row *creditThresholdConfigPO,
) error {
	if tx == nil || row == nil {
		return domainbilling.ErrInvalidInput
	}
	var result *gorm.DB
	if strings.EqualFold(strings.TrimSpace(tx.Dialector.Name()), "mysql") {
		result = tx.Create(row)
	} else {
		result = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	}
	if result.Error != nil {
		return normalizeCreditThresholdConfigWriteError(result.Error)
	}
	if result.RowsAffected != 1 {
		return domainbilling.ErrVersionConflict
	}
	return nil
}

func normalizeCreditThresholdConfigWriteError(err error) error {
	if err == nil {
		return nil
	}
	var mysqlError *mysqldriver.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1062, 1205, 1213:
			return fmt.Errorf(
				"%w: credit threshold configuration changed concurrently",
				domainbilling.ErrVersionConflict,
			)
		}
	}
	return err
}

func validateAdminCreditThresholdSubject(subject domainbilling.Subject) error {
	if subject.ID < 0 {
		return domainbilling.ErrInvalidInput
	}
	switch subject.Type {
	case domainbilling.SubjectTypeUser, domainbilling.SubjectTypeWorkspace:
	default:
		return domainbilling.ErrInvalidInput
	}
	if subject.ID == 0 {
		return nil
	}
	return subject.Validate()
}

func creditThresholdConfigView(
	requestedSubject domainbilling.Subject,
	row creditThresholdConfigPO,
	inherited bool,
) *CreditThresholdConfigView {
	targetVersion := row.Version
	if inherited {
		targetVersion = 0
	}
	return &CreditThresholdConfigView{
		SubjectType:          requestedSubject.Type,
		SubjectID:            requestedSubject.ID,
		SourceSubjectID:      row.SubjectID,
		Inherited:            inherited,
		Enabled:              row.Enabled,
		ThresholdMicros:      row.ThresholdMicros,
		RecoveryMarginMicros: row.RecoveryMarginMicros,
		Version:              targetVersion,
		SourceVersion:        row.Version,
		UpdatedAt:            row.UpdatedAt,
	}
}

func (r *AdminRepository) ListPlans(ctx context.Context) ([]AdminPlanView, error) {
	var rows []AdminPlanView
	err := r.db.WithContext(ctx).Table("subscription_plans AS plans").
		Select(`plans.id, plans.plan_key, plans.name, plans.description,
			plans.status, plans.sort_order, plans.current_version,
			versions.billing_cycle AS cycle, versions.price_micros, versions.currency,
			versions.credit_grant_micros, versions.features_json, versions.effective_at`).
		Joins("JOIN subscription_plan_versions AS versions ON versions.plan_id = plans.id AND versions.version = plans.current_version").
		Where("plans.deleted_at IS NULL").Order("plans.sort_order ASC, plans.id ASC").Scan(&rows).Error
	return rows, err
}
func (r *AdminRepository) CreatePlan(ctx context.Context, input CreatePlanInput) (*domainbilling.SubscriptionPlan, error) {
	if input.ActorUserID <= 0 || strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Name) == "" || input.PriceMicros < 0 || input.CreditGrantMicros < 0 || (input.Cycle != domainbilling.BillingCycleMonthly && input.Cycle != domainbilling.BillingCycleYearly) {
		return nil, domainbilling.ErrInvalidInput
	}
	if input.FeaturesJSON == "" {
		input.FeaturesJSON = "[]"
	}
	if !json.Valid([]byte(input.FeaturesJSON)) {
		return nil, domainbilling.ErrInvalidInput
	}
	now := time.Now().UTC()
	planID, err := r.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	versionID, err := r.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	plan := planPO{ID: planID, Key: strings.TrimSpace(input.Key), Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Status: string(domainbilling.CatalogStatusPublished), SortOrder: input.SortOrder, CurrentVersion: 1, CreatedBy: input.ActorUserID, UpdatedBy: input.ActorUserID, CreatedAt: now, UpdatedAt: now}
	version := planVersionPO{ID: versionID, PlanID: planID, Version: 1, Cycle: string(input.Cycle), PriceMicros: input.PriceMicros, Currency: strings.ToUpper(input.Currency), CreditGrantMicros: input.CreditGrantMicros, FeaturesJSON: input.FeaturesJSON, EffectiveAt: now, CreatedBy: input.ActorUserID, CreatedAt: now}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&plan).Error; e != nil {
			return e
		}
		return tx.Create(&version).Error
	})
	if err != nil {
		return nil, err
	}
	return plan.toDomain(), nil
}

func (r *AdminRepository) ListPackages(ctx context.Context) ([]*domainbilling.CreditPackage, error) {
	var rows []creditPackagePO
	if err := r.db.WithContext(ctx).Where("deleted_at IS NULL").Order("sort_order ASC,id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*domainbilling.CreditPackage, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toDomain())
	}
	return out, nil
}
func (r *AdminRepository) CreatePackage(ctx context.Context, input CreatePackageInput) (*domainbilling.CreditPackage, error) {
	if input.ActorUserID <= 0 || strings.TrimSpace(input.Key) == "" || strings.TrimSpace(input.Name) == "" || input.PriceMicros < 0 || input.CreditMicros <= 0 || input.ValidityDays < 0 {
		return nil, domainbilling.ErrInvalidInput
	}
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := creditPackagePO{ID: id, Key: strings.TrimSpace(input.Key), Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Status: string(domainbilling.CatalogStatusPublished), PriceMicros: input.PriceMicros, Currency: strings.ToUpper(input.Currency), CreditMicros: input.CreditMicros, ValidityDays: input.ValidityDays, PurchaseLimit: input.PurchaseLimit, SortOrder: input.SortOrder, Version: domainbilling.InitialVersion, CreatedBy: input.ActorUserID, UpdatedBy: input.ActorUserID, CreatedAt: now, UpdatedAt: now}
	if err = r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *AdminRepository) ListAccounts(ctx context.Context) ([]AccountView, error) {
	var rows []accountPO
	err := r.db.WithContext(ctx).Order("updated_at DESC,id DESC").Limit(500).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]AccountView, 0, len(rows))
	for _, v := range rows {
		out = append(out, AccountView{ID: v.ID, SubjectType: v.SubjectType, SubjectID: v.SubjectID, AvailableMicros: v.AvailableMicros, ReservedMicros: v.ReservedMicros, Version: v.Version, UpdatedAt: v.UpdatedAt})
	}
	return out, nil
}
func (r *AdminRepository) ListLedger(ctx context.Context, accountID int64) ([]LedgerView, error) {
	var rows []ledgerPO
	query := r.db.WithContext(ctx)
	if accountID > 0 {
		query = query.Where("account_id = ?", accountID)
	}
	if err := query.Order("created_at DESC,id DESC").Limit(500).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]LedgerView, 0, len(rows))
	for _, v := range rows {
		out = append(out, LedgerView{ID: v.ID, AccountID: v.AccountID, Direction: v.Direction, EntryType: v.EntryType, AmountMicros: v.AmountMicros, AvailableAfterMicros: v.AvailableAfterMicros, ReservedAfterMicros: v.ReservedAfterMicros, BusinessNo: v.BusinessNo, ActorUserID: v.ActorUserID, CreatedAt: v.CreatedAt})
	}
	return out, nil
}
func (r *AdminRepository) ListOrders(ctx context.Context) ([]OrderView, error) {
	var rows []orderPO
	if err := r.db.WithContext(ctx).Order("created_at DESC,id DESC").Limit(500).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OrderView, 0, len(rows))
	for _, v := range rows {
		out = append(out, OrderView{ID: v.ID, OrderNo: v.OrderNo, UserID: v.UserID, AccountID: v.AccountID, OrderType: v.OrderType, Status: v.Status, PaymentStatus: v.PaymentStatus, FulfillmentStatus: v.FulfillmentStatus, TotalMicros: v.TotalMicros, Currency: v.Currency, CreatedAt: v.CreatedAt})
	}
	return out, nil
}
func (r *AdminRepository) ListPrices(ctx context.Context) ([]PriceView, error) {
	var rows []modelPricePO
	if err := r.db.WithContext(ctx).Order("provider ASC,model_id ASC,version DESC").Limit(500).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PriceView, 0, len(rows))
	for _, v := range rows {
		out = append(out, PriceView{ID: v.ID, Provider: v.Provider, ModelID: v.ModelID, Version: v.Version, Currency: v.Currency, InputPrice: v.InputPrice, OutputPrice: v.OutputPrice, CacheWritePrice: v.CacheWritePrice, CacheHitPrice: v.CacheHitPrice, Status: v.Status, EffectiveAt: v.EffectiveAt})
	}
	return out, nil
}
func (r *AdminRepository) CreatePrice(ctx context.Context, input CreatePriceInput) (*domainbilling.ModelPrice, error) {
	if input.ActorUserID <= 0 || strings.TrimSpace(input.Provider) == "" || strings.TrimSpace(input.ModelID) == "" || input.InputPrice < 0 || input.OutputPrice < 0 || input.CacheWritePrice < 0 || input.CacheHitPrice < 0 {
		return nil, domainbilling.ErrInvalidInput
	}
	var version int
	r.db.WithContext(ctx).Model(&modelPricePO{}).Where("provider = ? AND model_id = ?", input.Provider, input.ModelID).Select("COALESCE(MAX(version),0)").Scan(&version)
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if input.EffectiveAt.IsZero() {
		input.EffectiveAt = now
	}
	row := modelPricePO{ID: id, Provider: strings.TrimSpace(input.Provider), ModelID: strings.TrimSpace(input.ModelID), Version: version + 1, Currency: strings.ToUpper(input.Currency), InputPrice: input.InputPrice, OutputPrice: input.OutputPrice, CacheWritePrice: input.CacheWritePrice, CacheHitPrice: input.CacheHitPrice, Status: string(domainbilling.CatalogStatusPublished), EffectiveAt: input.EffectiveAt, CreatedBy: input.ActorUserID, CreatedAt: now}
	if err = r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return row.toDomain(), nil
}

func (r *AdminRepository) ListUsageMonitoring(ctx context.Context) ([]UsageMonitorView, error) {
	var rows []UsageMonitorView
	err := r.db.WithContext(ctx).Table("model_usage_records").
		Select(`provider, model_id, COUNT(*) AS calls,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			COALESCE(SUM(cache_write_tokens), 0) AS cache_write_tokens,
			COALESCE(SUM(cache_hit_tokens), 0) AS cache_hit_tokens,
			COALESCE(SUM(charge_micros), 0) AS charge_micros,
			MAX(created_at) AS last_used_at`).
		Group("provider, model_id").Order("last_used_at DESC").Limit(500).Scan(&rows).Error
	return rows, err
}
