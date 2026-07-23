// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const tokensPerMillion int64 = 1_000_000

type ModelPrice struct {
	ID                                                   int64
	Provider, ModelID                                    string
	Version                                              int
	Currency                                             string
	InputPerMillionMicros, OutputPerMillionMicros        int64
	CacheWritePerMillionMicros, CacheHitPerMillionMicros int64
	Status                                               CatalogStatus
	EffectiveAt                                          time.Time
	CreatedBy                                            int64
	CreatedAt                                            time.Time
}

type TokenUsage struct{ Input, Output, CacheWrite, CacheHit int64 }

type UsageRecord struct {
	ID, AccountID, UserID, SpaceID, TaskID int64
	RunID                                  string
	UsageSequence                          int64
	Provider, ModelID                      string
	PriceVersionID                         int64
	Usage                                  TokenUsage
	ChargeMicros                           int64
	PriceSnapshotJSON                      string
	Status                                 string
	CreatedAt                              time.Time
}

type MeteringRepository interface {
	GetEffectiveModelPrice(ctx context.Context, provider, modelID string, at time.Time) (*ModelPrice, error)
	FindUsageRecord(ctx context.Context, runID string, sequence int64) (*UsageRecord, error)
	CreateUsageRecord(ctx context.Context, record *UsageRecord) error
}

type SettleUsageInput struct {
	Subject                                  Subject
	UserID, SpaceID, TaskID                  int64
	RunID                                    string
	UsageSequence                            int64
	Provider, ModelID, ReservationBusinessNo string
	Usage                                    TokenUsage
}

type MeteringService struct {
	repository MeteringRepository
	ledger     *Service
	now        func() time.Time
}

func NewMeteringService(repository MeteringRepository, ledger *Service) *MeteringService {
	return &MeteringService{repository: repository, ledger: ledger, now: time.Now}
}

type ReserveUsageInput struct {
	Subject               Subject
	RunID                 string
	UsageSequence         int64
	Provider, ModelID     string
	ReservationBusinessNo string
	Usage                 TokenUsage
}

func (s *MeteringService) ReserveUsage(ctx context.Context, input ReserveUsageInput) (string, int64, error) {
	if input.Subject.Validate() != nil || strings.TrimSpace(input.RunID) == "" || input.UsageSequence <= 0 {
		return "", 0, ErrInvalidInput
	}
	price, err := s.repository.GetEffectiveModelPrice(ctx, input.Provider, input.ModelID, s.now().UTC())
	if err != nil {
		return "", 0, err
	}
	charge, err := CalculateCharge(*price, input.Usage)
	if err != nil {
		return "", 0, err
	}
	businessNo := strings.TrimSpace(input.ReservationBusinessNo)
	if businessNo == "" {
		businessNo = fmt.Sprintf("usage-reserve:%s:%d", input.RunID, input.UsageSequence)
	}
	businessNo, err = normalizeBusinessNo(businessNo)
	if err != nil {
		return "", 0, err
	}
	if charge == 0 {
		return businessNo, 0, nil
	}
	_, err = s.ledger.Reserve(ctx, ReserveInput{Subject: input.Subject, AmountMicros: charge, BusinessNo: businessNo})
	return businessNo, charge, err
}

func CalculateCharge(price ModelPrice, usage TokenUsage) (int64, error) {
	values := []int64{usage.Input, usage.Output, usage.CacheWrite, usage.CacheHit,
		price.InputPerMillionMicros, price.OutputPerMillionMicros, price.CacheWritePerMillionMicros, price.CacheHitPerMillionMicros}
	for _, value := range values {
		if value < 0 {
			return 0, ErrInvalidInput
		}
	}
	parts := [][2]int64{{usage.Input, price.InputPerMillionMicros}, {usage.Output, price.OutputPerMillionMicros},
		{usage.CacheWrite, price.CacheWritePerMillionMicros}, {usage.CacheHit, price.CacheHitPerMillionMicros}}
	total := int64(0)
	for _, part := range parts {
		if part[0] != 0 && part[1] > math.MaxInt64/part[0] {
			return 0, ErrInvalidInput
		}
		product := part[0] * part[1]
		charge := product / tokensPerMillion
		if product%tokensPerMillion != 0 {
			charge++
		}
		var err error
		total, err = checkedAdd(total, charge)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (s *MeteringService) SettleUsage(ctx context.Context, input SettleUsageInput) (*UsageRecord, error) {
	if input.Subject.Validate() != nil || input.UserID <= 0 || strings.TrimSpace(input.RunID) == "" || input.UsageSequence < 0 || strings.TrimSpace(input.ReservationBusinessNo) == "" {
		return nil, ErrInvalidInput
	}
	if existing, err := s.repository.FindUsageRecord(ctx, input.RunID, input.UsageSequence); err == nil {
		return existing, nil
	}
	now := s.now().UTC()
	price, err := s.repository.GetEffectiveModelPrice(ctx, input.Provider, input.ModelID, now)
	if err != nil {
		return nil, err
	}
	charge, err := CalculateCharge(*price, input.Usage)
	if err != nil {
		return nil, err
	}
	snapshot, err := json.Marshal(price)
	if err != nil {
		return nil, err
	}
	if charge == 0 {
		balance, balanceErr := s.ledger.GetBalance(ctx, input.Subject)
		if balanceErr != nil {
			return nil, balanceErr
		}
		record := &UsageRecord{AccountID: balance.AccountID, UserID: input.UserID, SpaceID: input.SpaceID, TaskID: input.TaskID, RunID: input.RunID, UsageSequence: input.UsageSequence, Provider: price.Provider, ModelID: price.ModelID, PriceVersionID: price.ID, Usage: input.Usage, ChargeMicros: 0, PriceSnapshotJSON: string(snapshot), Status: "settled", CreatedAt: now}
		if createErr := s.repository.CreateUsageRecord(ctx, record); createErr != nil {
			if existing, findErr := s.repository.FindUsageRecord(ctx, input.RunID, input.UsageSequence); findErr == nil {
				return existing, nil
			}
			return nil, createErr
		}
		return record, nil
	}
	settlementBusinessNo := fmt.Sprintf("usage:%s:%d", input.RunID, input.UsageSequence)
	result, err := s.ledger.Settle(ctx, SettleInput{ReservationBusinessNo: input.ReservationBusinessNo,
		BusinessNo: settlementBusinessNo, ActualMicros: charge, ActorUserID: input.UserID,
		MetadataJSON: fmt.Sprintf(`{"run_id":%q,"usage_sequence":%d}`, input.RunID, input.UsageSequence)})
	if err != nil {
		return nil, err
	}
	record := &UsageRecord{AccountID: result.Balance.AccountID, UserID: input.UserID, SpaceID: input.SpaceID, TaskID: input.TaskID,
		RunID: input.RunID, UsageSequence: input.UsageSequence, Provider: price.Provider, ModelID: price.ModelID,
		PriceVersionID: price.ID, Usage: input.Usage, ChargeMicros: charge, PriceSnapshotJSON: string(snapshot), Status: "settled", CreatedAt: now}
	if err = s.repository.CreateUsageRecord(ctx, record); err != nil {
		if existing, findErr := s.repository.FindUsageRecord(ctx, input.RunID, input.UsageSequence); findErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return record, nil
}
