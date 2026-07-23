// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"

	"gorm.io/gorm"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	infrabilling "github.com/coze-dev/coze-studio/backend/infra/billing"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

// Service is the application boundary used by HTTP handlers, runtime metering,
// subscription grants, and administrative adjustments.
type Service struct {
	db          *gorm.DB
	idGenerator idgen.IDGenerator
	ledger      *domainbilling.Service
	commerce    *domainbilling.CommerceService
	metering    *domainbilling.MeteringService
	admin       *infrabilling.AdminRepository
	user        *infrabilling.UserRepository
	maintenance *infrabilling.MaintenanceRepository
	gateway     PaymentGateway
}

var defaultService *Service

func NewService(db *gorm.DB, idGenerator idgen.IDGenerator) *Service {
	repository := infrabilling.NewMySQLRepository(db, idGenerator)
	ledger := domainbilling.NewService(repository)
	return &Service{
		db:          db,
		idGenerator: idGenerator,
		ledger:      ledger,
		commerce:    domainbilling.NewCommerceService(repository, ledger),
		metering:    domainbilling.NewMeteringService(repository, ledger),
		admin:       infrabilling.NewAdminRepository(db, idGenerator),
		user:        infrabilling.NewUserRepository(db),
		maintenance: infrabilling.NewMaintenanceRepository(db, idGenerator),
		gateway:     NewPaymentGatewayFromEnv(),
	}
}

func (s *Service) AdminRepository() *infrabilling.AdminRepository { return s.admin }
func (s *Service) UserRepository() *infrabilling.UserRepository   { return s.user }

func InitService(db *gorm.DB, idGenerator idgen.IDGenerator) *Service {
	defaultService = NewService(db, idGenerator)
	return defaultService
}

func DefaultService() *Service {
	return defaultService
}

func (s *Service) Grant(ctx context.Context, input domainbilling.GrantInput) (*domainbilling.GrantResult, error) {
	return s.ledger.Grant(ctx, input)
}

func (s *Service) Reserve(ctx context.Context, input domainbilling.ReserveInput) (*domainbilling.ReservationResult, error) {
	return s.ledger.Reserve(ctx, input)
}

func (s *Service) Settle(ctx context.Context, input domainbilling.SettleInput) (*domainbilling.ReservationResult, error) {
	return s.ledger.Settle(ctx, input)
}

func (s *Service) Release(ctx context.Context, input domainbilling.ReleaseInput) (*domainbilling.ReservationResult, error) {
	return s.ledger.Release(ctx, input)
}

func (s *Service) GetBalance(ctx context.Context, subject domainbilling.Subject) (*domainbilling.Balance, error) {
	return s.ledger.GetBalance(ctx, subject)
}

func (s *Service) CreateOrder(ctx context.Context, input domainbilling.CreateOrderInput) (*domainbilling.Order, error) {
	return s.commerce.CreateOrder(ctx, input)
}

func (s *Service) RecordPaymentSucceeded(ctx context.Context, input domainbilling.PaymentSucceededInput) (*domainbilling.Order, error) {
	return s.commerce.RecordPaymentSucceeded(ctx, input)
}

func (s *Service) FulfillOrder(ctx context.Context, orderNo string) (*domainbilling.Order, error) {
	return s.commerce.FulfillOrder(ctx, orderNo)
}

func (s *Service) SettleUsage(ctx context.Context, input domainbilling.SettleUsageInput) (*domainbilling.UsageRecord, error) {
	return s.metering.SettleUsage(ctx, input)
}

func (s *Service) ReserveUsage(ctx context.Context, input domainbilling.ReserveUsageInput) (string, int64, error) {
	return s.metering.ReserveUsage(ctx, input)
}
func (s *Service) SettlementEnabled(ctx context.Context) (bool, error) {
	config, err := s.admin.GetConfig(ctx)
	if err != nil {
		return false, err
	}
	return config.SettlementEnabled, nil
}
