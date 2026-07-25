// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	appbilling "github.com/coze-dev/coze-studio/backend/application/billing"
	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestBillingPaymentCallbackErrorResponsePreservesRetryAndSanitizesMessage(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		statusCode int
		message    string
	}{
		{
			name: "invalid signature",
			err: appbilling.ErrPaymentCallbackInvalid,
			statusCode: http.StatusBadRequest,
			message: "invalid payment callback",
		},
		{
			name: "business conflict",
			err: domainbilling.ErrIdempotencyConflict,
			statusCode: http.StatusConflict,
			message: "payment callback state conflict",
		},
		{
			name: "storage retry",
			err: fmt.Errorf("%w: deadlock secret-provider-token", domainnotification.ErrStorage),
			statusCode: http.StatusServiceUnavailable,
			message: "payment callback retry later",
		},
		{
			name: "gateway unavailable retry",
			err: appbilling.ErrPaymentGatewayUnavailable,
			statusCode: http.StatusServiceUnavailable,
			message: "payment callback retry later",
		},
		{
			name: "unknown storage-like failure",
			err: errors.New("driver deadlock secret-provider-token"),
			statusCode: http.StatusServiceUnavailable,
			message: "payment callback retry later",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, message := billingPaymentCallbackErrorResponse(tc.err)
			if statusCode != tc.statusCode || message != tc.message {
				t.Fatalf("response = %d %q, want %d %q", statusCode, message, tc.statusCode, tc.message)
			}
			if strings.Contains(message, "secret") || strings.Contains(message, "deadlock") || strings.Contains(message, "token") {
				t.Fatalf("callback error response leaks internal detail: %q", message)
			}
		})
	}
}
