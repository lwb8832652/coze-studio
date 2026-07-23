// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appbilling "github.com/coze-dev/coze-studio/backend/application/billing"
	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

func currentBillingUserID(ctx context.Context) (int64, error) {
	value := ctxutil.GetUIDFromCtx(ctx)
	if value == nil || *value <= 0 {
		return 0, errors.New("unauthorized")
	}
	return *value, nil
}
func userBillingContext(ctx context.Context, c *app.RequestContext) (int64, *domainbilling.Balance, bool) {
	userID, err := currentBillingUserID(ctx)
	if err != nil {
		c.JSON(http.StatusUnauthorized, map[string]any{"code": 401, "msg": "unauthorized"})
		return 0, nil, false
	}
	service := billingService(c)
	if service == nil {
		return 0, nil, false
	}
	balance, err := service.GetBalance(ctx, domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: userID})
	if err != nil {
		billingError(c, err)
		return 0, nil, false
	}
	return userID, balance, true
}

func GetBillingAccount(ctx context.Context, c *app.RequestContext) {
	_, balance, ok := userBillingContext(ctx, c)
	if ok {
		billingOK(c, balance)
	}
}

func GetPublicBillingConfig(ctx context.Context, c *app.RequestContext) {
	if _, _, ok := userBillingContext(ctx, c); !ok {
		return
	}
	config, err := billingService(c).AdminRepository().GetConfig(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, map[string]any{
		"credit_name":      config.CreditName,
		"display_scale":    config.DisplayScale,
		"payment_enabled":  config.PaymentEnabled,
		"default_currency": config.DefaultCurrency,
	})
}
func GetMySubscription(ctx context.Context, c *app.RequestContext) {
	_, balance, ok := userBillingContext(ctx, c)
	if !ok {
		return
	}
	data, err := billingService(c).UserRepository().GetSubscription(ctx, balance.AccountID)
	if errors.Is(err, domainbilling.ErrNotFound) {
		billingOK(c, nil)
		return
	}
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func ListAvailableBillingPlans(ctx context.Context, c *app.RequestContext) {
	if _, _, ok := userBillingContext(ctx, c); !ok {
		return
	}
	data, err := billingService(c).UserRepository().ListPlans(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func ListAvailableCreditPackages(ctx context.Context, c *app.RequestContext) {
	if _, _, ok := userBillingContext(ctx, c); !ok {
		return
	}
	data, err := billingService(c).UserRepository().ListPackages(ctx)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func ListMyCreditLedger(ctx context.Context, c *app.RequestContext) {
	_, balance, ok := userBillingContext(ctx, c)
	if !ok {
		return
	}
	data, err := billingService(c).UserRepository().ListLedger(ctx, balance.AccountID)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func ListMyModelUsage(ctx context.Context, c *app.RequestContext) {
	_, balance, ok := userBillingContext(ctx, c)
	if !ok {
		return
	}
	data, err := billingService(c).UserRepository().ListUsage(ctx, balance.AccountID)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func ListMyBillingOrders(ctx context.Context, c *app.RequestContext) {
	userID, _, ok := userBillingContext(ctx, c)
	if !ok {
		return
	}
	data, err := billingService(c).UserRepository().ListOrders(ctx, userID)
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}
func CreateMyBillingOrder(ctx context.Context, c *app.RequestContext) {
	userID, _, ok := userBillingContext(ctx, c)
	if !ok {
		return
	}
	var request struct {
		OrderType domainbilling.OrderType `json:"order_type"`
		TargetID  int64                   `json:"target_id,string"`
		OrderNo   string                  `json:"order_no"`
	}
	if err := c.BindAndValidate(&request); err != nil {
		billingError(c, domainbilling.ErrInvalidInput)
		return
	}
	data, err := billingService(c).CreateOrder(ctx, domainbilling.CreateOrderInput{UserID: userID, Subject: domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: userID}, Type: request.OrderType, TargetID: request.TargetID, OrderNo: request.OrderNo})
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}

func CreateMyBillingCheckout(ctx context.Context, c *app.RequestContext) {
	userID, err := currentBillingUserID(ctx)
	if err != nil {
		c.JSON(http.StatusUnauthorized, map[string]any{"code": 401, "msg": "unauthorized"})
		return
	}
	service := billingService(c)
	if service == nil {
		return
	}
	data, err := service.CreateCheckout(ctx, appbilling.CreateCheckoutInput{UserID: userID, OrderNo: c.Param("order_no")})
	if err != nil {
		billingError(c, err)
		return
	}
	billingOK(c, data)
}

func HandleBillingPaymentCallback(ctx context.Context, c *app.RequestContext) {
	service := billingService(c)
	if service == nil {
		return
	}
	headers := map[string]string{}
	c.Request.Header.VisitAll(func(key, value []byte) { headers[strings.ToLower(string(key))] = string(value) })
	data, err := service.HandlePaymentCallback(ctx, c.Param("gateway"), headers, c.Request.Body())
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]any{"code": 400, "msg": "invalid payment callback"})
		return
	}
	billingOK(c, data)
}
