// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appbilling "github.com/coze-dev/coze-studio/backend/application/billing"
	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	infrabilling "github.com/coze-dev/coze-studio/backend/infra/billing"
)

func billingService(c *app.RequestContext) *appbilling.Service {
	service := appbilling.DefaultService()
	if service == nil {
		c.JSON(http.StatusServiceUnavailable, map[string]any{"code": 503, "msg": "billing service unavailable"})
	}
	return service
}

func billingOK(c *app.RequestContext, data any) {
	c.JSON(http.StatusOK, map[string]any{"code": 0, "msg": "", "data": data})
}
func billingError(c *app.RequestContext, err error) {
	status := http.StatusInternalServerError
	message := "billing request failed"
	switch {
	case errors.Is(err, domainbilling.ErrInvalidInput):
		status, message = http.StatusBadRequest, "invalid billing request"
	case errors.Is(err, domainbilling.ErrNotFound):
		status, message = http.StatusNotFound, "billing resource not found"
	case errors.Is(err, domainbilling.ErrInsufficientCredits):
		status, message = http.StatusConflict, "insufficient credits"
	case errors.Is(err, domainbilling.ErrIdempotencyConflict),
		errors.Is(err, domainbilling.ErrVersionConflict),
		errors.Is(err, domainbilling.ErrReservationNotActive):
		status, message = http.StatusConflict, "billing state conflict"
	case errors.Is(err, appbilling.ErrPaymentGatewayUnavailable):
		status, message = http.StatusServiceUnavailable, "payment service unavailable"
	}
	c.JSON(status, map[string]any{"code": status, "msg": message})
}
func billingActorID(ctx context.Context) int64 {
	value := ctxutil.GetUIDFromCtx(ctx)
	if value == nil {
		return 0
	}
	return *value
}

func GetAdminBillingOverview(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.AdminRepository().Overview(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func GetAdminBillingConfig(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.AdminRepository().GetConfig(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func SaveAdminBillingConfig(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	var request infrabilling.BillingConfig
	if err := c.BindAndValidate(&request); err != nil {
		billingError(c, domainbilling.ErrInvalidInput)
		return
	}
	data, err := service.AdminRepository().SaveConfig(ctx, request, billingActorID(ctx))
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}

func ListAdminBillingPlans(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.AdminRepository().ListPlans(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func CreateAdminBillingPlan(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	var request infrabilling.CreatePlanInput
	if err := c.BindAndValidate(&request); err != nil {
		billingError(c, domainbilling.ErrInvalidInput)
		return
	}
	request.ActorUserID = billingActorID(ctx)
	data, err := service.AdminRepository().CreatePlan(ctx, request)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}

func ListAdminCreditPackages(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.AdminRepository().ListPackages(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func CreateAdminCreditPackage(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	var request infrabilling.CreatePackageInput
	if err := c.BindAndValidate(&request); err != nil {
		billingError(c, domainbilling.ErrInvalidInput)
		return
	}
	request.ActorUserID = billingActorID(ctx)
	data, err := service.AdminRepository().CreatePackage(ctx, request)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}

func ListAdminBillingAccounts(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.AdminRepository().ListAccounts(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func ListAdminCreditLedger(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	accountID, _ := strconv.ParseInt(c.Query("account_id"), 10, 64)
	data, err := service.AdminRepository().ListLedger(ctx, accountID)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func ListAdminBillingOrders(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.AdminRepository().ListOrders(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func ListAdminModelPrices(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.AdminRepository().ListPrices(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func CreateAdminModelPrice(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	var request infrabilling.CreatePriceInput
	if err := c.BindAndValidate(&request); err != nil {
		billingError(c, domainbilling.ErrInvalidInput)
		return
	}
	request.ActorUserID = billingActorID(ctx)
	data, err := service.AdminRepository().CreatePrice(ctx, request)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}

func ListAdminBillingUsageMonitoring(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.AdminRepository().ListUsageMonitoring(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}

type adminCreditAdjustmentRequest struct {
	SubjectType  string `json:"subject_type"`
	SubjectID    int64  `json:"subject_id,string"`
	AmountMicros int64  `json:"amount_micros"`
	BusinessNo   string `json:"business_no"`
	Reason       string `json:"reason"`
}

func AdjustAdminBillingCredits(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	var request adminCreditAdjustmentRequest
	if err := c.BindAndValidate(&request); err != nil {
		billingError(c, domainbilling.ErrInvalidInput)
		return
	}
	data, err := service.AdjustCredits(ctx, appbilling.CreditAdjustmentInput{
		Subject:      domainbilling.Subject{Type: domainbilling.SubjectType(request.SubjectType), ID: request.SubjectID},
		AmountMicros: request.AmountMicros, BusinessNo: request.BusinessNo, Reason: request.Reason, ActorUserID: billingActorID(ctx),
	})
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}

func RunAdminBillingMaintenance(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.RunMaintenance(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{"code": 500, "msg": err.Error(), "data": data})
		return
	}
	billingOK(c, data)
}
