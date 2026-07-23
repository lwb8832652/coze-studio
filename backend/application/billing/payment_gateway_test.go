// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSignedHTTPPaymentGatewayVerifiesSucceededCallback(t *testing.T) {
	gateway := &SignedHTTPPaymentGateway{name: "test", signingSecret: []byte("secret")}
	body, err := json.Marshal(VerifiedPaymentEvent{
		OrderNo: "ord_1", ProviderTransactionID: "pay_1", Currency: "CNY",
		AmountMicros: 12_000_000, Status: "succeeded",
	})
	if err != nil {
		t.Fatal(err)
	}
	event, err := gateway.VerifyCallback(context.Background(), map[string]string{
		"x-billing-signature": signPaymentPayload(gateway.signingSecret, body),
	}, body)
	if err != nil {
		t.Fatalf("VerifyCallback() error = %v", err)
	}
	if event.Gateway != "test" || event.EventDigest == "" || event.OrderNo != "ord_1" {
		t.Fatalf("VerifyCallback() event = %#v", event)
	}
}

func TestSignedHTTPPaymentGatewayRejectsInvalidOrNonSucceededCallback(t *testing.T) {
	gateway := &SignedHTTPPaymentGateway{name: "test", signingSecret: []byte("secret")}
	body := []byte(`{"order_no":"ord_1","provider_transaction_id":"pay_1","currency":"CNY","amount_micros":1,"status":"failed"}`)
	if _, err := gateway.VerifyCallback(context.Background(), map[string]string{
		"x-billing-signature": signPaymentPayload(gateway.signingSecret, body),
	}, body); err == nil {
		t.Fatal("VerifyCallback() accepted a failed payment event")
	}
	if _, err := gateway.VerifyCallback(context.Background(), map[string]string{
		"x-billing-signature": "invalid",
	}, body); err == nil {
		t.Fatal("VerifyCallback() accepted an invalid signature")
	}
}

func TestSignedHTTPPaymentGatewayCreatesSecureCheckout(t *testing.T) {
	t.Setenv("APP_ENV", "debug")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/payments" {
			http.Error(response, "unexpected request", http.StatusBadRequest)
			return
		}
		if request.Header.Get("Authorization") != "Bearer api-key" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"provider_transaction_id":"pay_1","checkout_url":"` + serverURLForTest(request) + `/checkout"}`))
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	gateway := &SignedHTTPPaymentGateway{
		name: "test", baseURL: parsed, apiKey: "api-key", signingSecret: []byte("secret"), client: server.Client(),
	}
	result, err := gateway.CreatePayment(context.Background(), CreatePaymentRequest{
		OrderNo: "ord_1", AmountMicros: 10, Currency: "CNY",
		ReturnURL: "http://127.0.0.1/return", NotifyURL: "http://127.0.0.1/notify",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	if result.Gateway != "test" || result.ProviderTransactionID != "pay_1" {
		t.Fatalf("CreatePayment() result = %#v", result)
	}
}

func serverURLForTest(request *http.Request) string {
	return "http://" + request.Host
}
