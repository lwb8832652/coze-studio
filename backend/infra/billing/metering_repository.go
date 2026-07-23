// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

func (r *MySQLRepository) GetEffectiveModelPrice(ctx context.Context, provider, modelID string, at time.Time) (*domainbilling.ModelPrice, error) {
	var po modelPricePO
	err := r.db.WithContext(ctx).Where("provider = ? AND model_id = ? AND status = ? AND effective_at <= ?", provider, modelID, string(domainbilling.CatalogStatusPublished), at).
		Order("effective_at DESC, version DESC").First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) FindUsageRecord(ctx context.Context, runID string, sequence int64) (*domainbilling.UsageRecord, error) {
	var po usageRecordPO
	err := r.db.WithContext(ctx).Where("run_id = ? AND usage_sequence = ?", runID, sequence).First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domainbilling.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return po.toDomain(), nil
}

func (r *MySQLRepository) CreateUsageRecord(ctx context.Context, record *domainbilling.UsageRecord) error {
	id, err := r.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	record.ID = id
	return r.db.WithContext(ctx).Create(usageRecordPOFromDomain(record)).Error
}

type modelPricePO struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	Provider        string    `gorm:"column:provider"`
	ModelID         string    `gorm:"column:model_id"`
	Version         int       `gorm:"column:version"`
	Currency        string    `gorm:"column:currency"`
	InputPrice      int64     `gorm:"column:input_per_million_micros"`
	OutputPrice     int64     `gorm:"column:output_per_million_micros"`
	CacheWritePrice int64     `gorm:"column:cache_write_per_million_micros"`
	CacheHitPrice   int64     `gorm:"column:cache_hit_per_million_micros"`
	Status          string    `gorm:"column:status"`
	EffectiveAt     time.Time `gorm:"column:effective_at"`
	CreatedBy       int64     `gorm:"column:created_by"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (modelPricePO) TableName() string { return "model_price_versions" }
func (p modelPricePO) toDomain() *domainbilling.ModelPrice {
	return &domainbilling.ModelPrice{ID: p.ID, Provider: p.Provider, ModelID: p.ModelID, Version: p.Version, Currency: p.Currency, InputPerMillionMicros: p.InputPrice, OutputPerMillionMicros: p.OutputPrice, CacheWritePerMillionMicros: p.CacheWritePrice, CacheHitPerMillionMicros: p.CacheHitPrice, Status: domainbilling.CatalogStatus(p.Status), EffectiveAt: p.EffectiveAt, CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt}
}

type usageRecordPO struct {
	ID                int64     `gorm:"column:id;primaryKey"`
	AccountID         int64     `gorm:"column:account_id"`
	UserID            int64     `gorm:"column:user_id"`
	SpaceID           int64     `gorm:"column:space_id"`
	TaskID            int64     `gorm:"column:task_id"`
	RunID             string    `gorm:"column:run_id"`
	UsageSequence     int64     `gorm:"column:usage_sequence"`
	Provider          string    `gorm:"column:provider"`
	ModelID           string    `gorm:"column:model_id"`
	PriceVersionID    int64     `gorm:"column:price_version_id"`
	InputTokens       int64     `gorm:"column:input_tokens"`
	OutputTokens      int64     `gorm:"column:output_tokens"`
	CacheWriteTokens  int64     `gorm:"column:cache_write_tokens"`
	CacheHitTokens    int64     `gorm:"column:cache_hit_tokens"`
	ChargeMicros      int64     `gorm:"column:charge_micros"`
	PriceSnapshotJSON string    `gorm:"column:price_snapshot_json"`
	Status            string    `gorm:"column:status"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (usageRecordPO) TableName() string { return "model_usage_records" }
func usageRecordPOFromDomain(r *domainbilling.UsageRecord) *usageRecordPO {
	return &usageRecordPO{ID: r.ID, AccountID: r.AccountID, UserID: r.UserID, SpaceID: r.SpaceID, TaskID: r.TaskID, RunID: r.RunID, UsageSequence: r.UsageSequence, Provider: r.Provider, ModelID: r.ModelID, PriceVersionID: r.PriceVersionID, InputTokens: r.Usage.Input, OutputTokens: r.Usage.Output, CacheWriteTokens: r.Usage.CacheWrite, CacheHitTokens: r.Usage.CacheHit, ChargeMicros: r.ChargeMicros, PriceSnapshotJSON: r.PriceSnapshotJSON, Status: r.Status, CreatedAt: r.CreatedAt}
}
func (p usageRecordPO) toDomain() *domainbilling.UsageRecord {
	return &domainbilling.UsageRecord{ID: p.ID, AccountID: p.AccountID, UserID: p.UserID, SpaceID: p.SpaceID, TaskID: p.TaskID, RunID: p.RunID, UsageSequence: p.UsageSequence, Provider: p.Provider, ModelID: p.ModelID, PriceVersionID: p.PriceVersionID, Usage: domainbilling.TokenUsage{Input: p.InputTokens, Output: p.OutputTokens, CacheWrite: p.CacheWriteTokens, CacheHit: p.CacheHitTokens}, ChargeMicros: p.ChargeMicros, PriceSnapshotJSON: p.PriceSnapshotJSON, Status: p.Status, CreatedAt: p.CreatedAt}
}
