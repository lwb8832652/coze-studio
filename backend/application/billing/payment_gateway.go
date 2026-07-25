// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

var (
	ErrPaymentGatewayUnavailable = errors.New("billing: payment gateway unavailable")
	ErrPaymentCallbackInvalid    = errors.New("billing: invalid payment callback")
)

type CreatePaymentRequest struct {
	OrderNo      string `json:"order_no"`
	AmountMicros int64  `json:"amount_micros"`
	Currency     string `json:"currency"`
	ReturnURL    string `json:"return_url"`
	NotifyURL    string `json:"notify_url"`
}
type CreatePaymentResponse struct {
	Gateway               string `json:"gateway"`
	ProviderTransactionID string `json:"provider_transaction_id"`
	CheckoutURL           string `json:"checkout_url"`
}
type VerifiedPaymentEvent struct {
	OrderNo               string `json:"order_no"`
	Gateway               string `json:"gateway"`
	ProviderTransactionID string `json:"provider_transaction_id"`
	ProviderEventID       string `json:"provider_event_id,omitempty"`
	EventDigest           string `json:"event_digest"`
	Currency              string `json:"currency"`
	AmountMicros          int64  `json:"amount_micros"`
	Status                string `json:"status"`
}

// PaymentGateway implementations own provider-specific signing and callback verification.
type PaymentGateway interface {
	CreatePayment(ctx context.Context, request CreatePaymentRequest) (*CreatePaymentResponse, error)
	VerifyCallback(ctx context.Context, headers map[string]string, body []byte) (*VerifiedPaymentEvent, error)
	QueryPayment(ctx context.Context, providerTransactionID string) (*VerifiedPaymentEvent, error)
}

type DisabledPaymentGateway struct{}

func (DisabledPaymentGateway) CreatePayment(context.Context, CreatePaymentRequest) (*CreatePaymentResponse, error) {
	return nil, ErrPaymentGatewayUnavailable
}
func (DisabledPaymentGateway) VerifyCallback(context.Context, map[string]string, []byte) (*VerifiedPaymentEvent, error) {
	return nil, ErrPaymentGatewayUnavailable
}
func (DisabledPaymentGateway) QueryPayment(context.Context, string) (*VerifiedPaymentEvent, error) {
	return nil, ErrPaymentGatewayUnavailable
}

type SignedHTTPPaymentGateway struct {
	name          string
	baseURL       *url.URL
	apiKey        string
	signingSecret []byte
	client        *http.Client
}

func NewPaymentGatewayFromEnv() PaymentGateway {
	baseURL := strings.TrimSpace(os.Getenv("BILLING_PAYMENT_GATEWAY_URL"))
	apiKey := strings.TrimSpace(os.Getenv("BILLING_PAYMENT_GATEWAY_KEY"))
	secret := strings.TrimSpace(os.Getenv("BILLING_PAYMENT_GATEWAY_SIGNING_SECRET"))
	name := strings.TrimSpace(os.Getenv("BILLING_PAYMENT_GATEWAY_NAME"))
	if baseURL == "" || apiKey == "" || secret == "" || name == "" {
		return DisabledPaymentGateway{}
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !paymentDebugLoopbackURL(parsed)) {
		return DisabledPaymentGateway{}
	}
	return &SignedHTTPPaymentGateway{
		name: name, baseURL: parsed, apiKey: apiKey, signingSecret: []byte(secret),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *SignedHTTPPaymentGateway) CreatePayment(ctx context.Context, request CreatePaymentRequest) (*CreatePaymentResponse, error) {
	if g == nil || strings.TrimSpace(request.OrderNo) == "" || request.AmountMicros < 0 || strings.TrimSpace(request.Currency) == "" {
		return nil, ErrPaymentGatewayUnavailable
	}
	var response CreatePaymentResponse
	if err := g.doJSON(ctx, http.MethodPost, "/payments", request, &response); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.ProviderTransactionID) == "" || strings.TrimSpace(response.CheckoutURL) == "" {
		return nil, fmt.Errorf("billing: payment gateway returned an incomplete checkout")
	}
	checkoutURL, err := url.Parse(response.CheckoutURL)
	if err != nil || checkoutURL.Host == "" || (checkoutURL.Scheme != "https" && !paymentDebugLoopbackURL(checkoutURL)) {
		return nil, fmt.Errorf("billing: payment gateway returned an insecure checkout URL")
	}
	response.Gateway = g.name
	return &response, nil
}

func (g *SignedHTTPPaymentGateway) VerifyCallback(_ context.Context, headers map[string]string, body []byte) (*VerifiedPaymentEvent, error) {
	if g == nil {
		return nil, ErrPaymentGatewayUnavailable
	}
	if len(body) == 0 || len(body) > 1<<20 {
		return nil, fmt.Errorf("%w: payment callback body is invalid", ErrPaymentCallbackInvalid)
	}
	provided := strings.TrimSpace(headers["x-billing-signature"])
	expected := signPaymentPayload(g.signingSecret, body)
	if provided == "" || !hmac.Equal([]byte(strings.ToLower(provided)), []byte(expected)) {
		return nil, fmt.Errorf("%w: signature mismatch", ErrPaymentCallbackInvalid)
	}
	var event VerifiedPaymentEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("%w: malformed payment callback payload", ErrPaymentCallbackInvalid)
	}
	if strings.TrimSpace(event.OrderNo) == "" || strings.TrimSpace(event.ProviderTransactionID) == "" || event.AmountMicros < 0 || strings.TrimSpace(event.Currency) == "" || !strings.EqualFold(event.Status, string(domainbilling.PaymentStatusSucceeded)) {
		return nil, fmt.Errorf("%w: payment callback fields are invalid", ErrPaymentCallbackInvalid)
	}
	event.Gateway = g.name
	digest := sha256.Sum256(body)
	event.EventDigest = hex.EncodeToString(digest[:])
	return &event, nil
}

func (g *SignedHTTPPaymentGateway) QueryPayment(ctx context.Context, providerTransactionID string) (*VerifiedPaymentEvent, error) {
	if g == nil || strings.TrimSpace(providerTransactionID) == "" {
		return nil, ErrPaymentGatewayUnavailable
	}
	var event VerifiedPaymentEvent
	path := "/payments/" + url.PathEscape(providerTransactionID)
	if err := g.doJSON(ctx, http.MethodGet, path, nil, &event); err != nil {
		return nil, err
	}
	event.Gateway = g.name
	return &event, nil
}

func (g *SignedHTTPPaymentGateway) doJSON(ctx context.Context, method, path string, payload, output any) error {
	endpoint := g.baseURL.ResolveReference(&url.URL{Path: strings.TrimRight(g.baseURL.Path, "/") + path})
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+g.apiKey)
	request.Header.Set("X-Billing-Signature", signPaymentPayload(g.signingSecret, body))
	response, err := g.client.Do(request)
	if err != nil {
		return fmt.Errorf("billing: payment gateway request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("billing: payment gateway rejected request with status %d", response.StatusCode)
	}
	if output != nil && len(responseBody) > 0 {
		if err = json.Unmarshal(responseBody, output); err != nil {
			return fmt.Errorf("billing: invalid payment gateway response")
		}
	}
	return nil
}

func signPaymentPayload(secret, payload []byte) string {
	signer := hmac.New(sha256.New, secret)
	_, _ = signer.Write(payload)
	return hex.EncodeToString(signer.Sum(nil))
}

func paymentDebugLoopbackURL(value *url.URL) bool {
	if value == nil || value.Scheme != "http" || os.Getenv("APP_ENV") != "debug" {
		return false
	}
	host := strings.ToLower(value.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
