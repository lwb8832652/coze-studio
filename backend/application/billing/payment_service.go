// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"strings"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

type CreateCheckoutInput struct {
	UserID  int64
	OrderNo string
}

func (s *Service) ConfigurePaymentGateway(gateway PaymentGateway) {
	if s == nil {
		return
	}
	if gateway == nil {
		gateway = DisabledPaymentGateway{}
	}
	s.gateway = gateway
}

func (s *Service) CreateCheckout(ctx context.Context, input CreateCheckoutInput) (*CreatePaymentResponse, error) {
	if s == nil || input.UserID <= 0 || strings.TrimSpace(input.OrderNo) == "" {
		return nil, domainbilling.ErrInvalidInput
	}
	order, err := s.user.GetOrderForUser(ctx, input.UserID, strings.TrimSpace(input.OrderNo))
	if err != nil {
		return nil, err
	}
	if order.TotalMicros == 0 {
		paid := order
		var paymentErr error
		switch {
		case order.Status == domainbilling.OrderStatusPending && order.PaymentStatus == domainbilling.PaymentStatusPending:
			digest := sha256.Sum256([]byte("free:" + order.OrderNo))
			paid, paymentErr = s.commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
				OrderNo: order.OrderNo, Gateway: "internal_free", ProviderTransactionID: "free:" + order.OrderNo,
				ProviderEventID: "free:" + order.OrderNo,
				EventDigest: hex.EncodeToString(digest[:]), AmountMicros: 0, Currency: order.Currency,
			})
			if paymentErr != nil {
				return nil, paymentErr
			}
		case order.PaymentStatus == domainbilling.PaymentStatusSucceeded &&
			order.FulfillmentStatus == domainbilling.FulfillmentStatusPending:
		case order.PaymentStatus == domainbilling.PaymentStatusSucceeded &&
			order.FulfillmentStatus == domainbilling.FulfillmentStatusSucceeded:
			return &CreatePaymentResponse{Gateway: "internal_free", ProviderTransactionID: "free:" + order.OrderNo}, nil
		default:
			return nil, domainbilling.ErrReservationNotActive
		}
		if _, paymentErr = s.commerce.FulfillOrder(ctx, paid.OrderNo); paymentErr != nil {
			return nil, paymentErr
		}
		return &CreatePaymentResponse{Gateway: "internal_free", ProviderTransactionID: "free:" + order.OrderNo}, nil
	}
	if order.Status != domainbilling.OrderStatusPending || order.PaymentStatus != domainbilling.PaymentStatusPending {
		return nil, domainbilling.ErrReservationNotActive
	}
	config, err := s.admin.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !config.PaymentEnabled {
		return nil, ErrPaymentGatewayUnavailable
	}
	returnURL, err := requiredSecurePaymentURL("BILLING_PAYMENT_RETURN_URL")
	if err != nil {
		return nil, err
	}
	notifyURL, err := requiredSecurePaymentURL("BILLING_PAYMENT_NOTIFY_URL")
	if err != nil {
		return nil, err
	}
	if s.gateway == nil {
		return nil, ErrPaymentGatewayUnavailable
	}
	return s.gateway.CreatePayment(ctx, CreatePaymentRequest{
		OrderNo: order.OrderNo, AmountMicros: order.TotalMicros, Currency: order.Currency,
		ReturnURL: returnURL, NotifyURL: notifyURL,
	})
}

func (s *Service) HandlePaymentCallback(
	ctx context.Context,
	gatewayName string,
	headers map[string]string,
	body []byte,
) (*domainbilling.Order, error) {
	if s == nil || s.gateway == nil || strings.TrimSpace(gatewayName) == "" || len(body) == 0 {
		if s != nil && s.gateway != nil {
			return nil, ErrPaymentCallbackInvalid
		}
		return nil, ErrPaymentGatewayUnavailable
	}
	config, err := s.admin.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !config.PaymentEnabled {
		return nil, ErrPaymentGatewayUnavailable
	}
	event, err := s.gateway.VerifyCallback(ctx, headers, body)
	if err != nil {
		return nil, err
	}
	if event == nil || !strings.EqualFold(strings.TrimSpace(event.Status), string(domainbilling.PaymentStatusSucceeded)) {
		return nil, ErrPaymentCallbackInvalid
	}
	if !strings.EqualFold(strings.TrimSpace(gatewayName), event.Gateway) {
		return nil, ErrPaymentCallbackInvalid
	}
	paid, err := s.commerce.RecordPaymentSucceeded(ctx, domainbilling.PaymentSucceededInput{
		OrderNo: event.OrderNo, Gateway: event.Gateway, ProviderTransactionID: event.ProviderTransactionID,
		ProviderEventID: event.ProviderEventID,
		EventDigest: event.EventDigest, AmountMicros: event.AmountMicros, Currency: event.Currency,
	})
	if err != nil {
		return nil, err
	}
	return s.commerce.FulfillOrder(ctx, paid.OrderNo)
}

func requiredSecurePaymentURL(envName string) (string, error) {
	raw := strings.TrimSpace(os.Getenv(envName))
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !paymentDebugLoopbackURL(parsed)) {
		return "", ErrPaymentGatewayUnavailable
	}
	return parsed.String(), nil
}
