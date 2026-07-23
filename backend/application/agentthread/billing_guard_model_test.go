// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

type billingGuardServiceStub struct {
	enabled       bool
	settlementErr error
	reserveErr    error
	reserveCalls  int
	input         domainbilling.ReserveUsageInput
}

func (s *billingGuardServiceStub) SettlementEnabled(context.Context) (bool, error) {
	return s.enabled, s.settlementErr
}

func (s *billingGuardServiceStub) ReserveUsage(_ context.Context, input domainbilling.ReserveUsageInput) (string, int64, error) {
	s.reserveCalls++
	s.input = input
	return input.ReservationBusinessNo, 1, s.reserveErr
}

type billingGuardModelStub struct {
	calls int
}

func (m *billingGuardModelStub) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	m.calls++
	return schema.AssistantMessage("ok", nil), nil
}

func (m *billingGuardModelStub) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.calls++
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("ok", nil)}), nil
}

func TestBillingGuardBypassesWhenDefaultServiceIsUnavailable(t *testing.T) {
	if service := billingGuardServiceProvider(); service != nil {
		t.Fatalf("billingGuardServiceProvider() = %#v, want nil", service)
	}
	inner := &billingGuardModelStub{}
	guard := wrapBillingGuardChatModel(inner, &RunSummary{RunID: 10, CreatorID: 20}, modelExecutorConfig{})

	if _, err := guard.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner calls = %d, want 1", inner.calls)
	}
}

func TestBillingGuardBypassesReservationWhenSettlementDisabled(t *testing.T) {
	service := &billingGuardServiceStub{}
	restoreBillingGuardService(t, service)
	inner := &billingGuardModelStub{}
	guard := wrapBillingGuardChatModel(inner, &RunSummary{RunID: 10, CreatorID: 20}, modelExecutorConfig{})

	if _, err := guard.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if inner.calls != 1 || service.reserveCalls != 0 {
		t.Fatalf("calls inner=%d reserve=%d", inner.calls, service.reserveCalls)
	}
}

func TestBillingGuardFailsClosedWithoutModelIdentity(t *testing.T) {
	service := &billingGuardServiceStub{enabled: true}
	restoreBillingGuardService(t, service)
	inner := &billingGuardModelStub{}
	guard := wrapBillingGuardChatModel(inner, &RunSummary{RunID: 10, CreatorID: 20}, modelExecutorConfig{})

	if _, err := guard.Generate(context.Background(), []*schema.Message{schema.UserMessage("hello")}); err == nil {
		t.Fatal("Generate() accepted missing billing model identity")
	}
	if inner.calls != 0 || service.reserveCalls != 0 {
		t.Fatalf("calls inner=%d reserve=%d", inner.calls, service.reserveCalls)
	}
}

func TestBillingGuardReservesBeforeCallingModel(t *testing.T) {
	service := &billingGuardServiceStub{enabled: true}
	restoreBillingGuardService(t, service)
	inner := &billingGuardModelStub{}
	maxTokens := 256
	guard := wrapBillingGuardChatModel(inner, &RunSummary{RunID: 10, CreatorID: 20}, modelExecutorConfig{MaxTokens: &maxTokens})
	ctx := context.WithValue(context.Background(), adkUsageCallContextKey, adkUsageCall{
		callID: "call-1", modelName: "model-1", provider: "provider-1",
	})

	if _, err := guard.Generate(ctx, []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if inner.calls != 1 || service.reserveCalls != 1 {
		t.Fatalf("calls inner=%d reserve=%d", inner.calls, service.reserveCalls)
	}
	if service.input.Subject.ID != 20 || service.input.Provider != "provider-1" || service.input.ModelID != "model-1" {
		t.Fatalf("ReserveUsage() input = %#v", service.input)
	}
	if service.input.Usage.Input != 128 || service.input.Usage.Output != 256 || service.input.ReservationBusinessNo == "" {
		t.Fatalf("ReserveUsage() estimate = %#v", service.input)
	}
}

func TestBillingGuardDoesNotCallModelWhenReservationFails(t *testing.T) {
	service := &billingGuardServiceStub{enabled: true, reserveErr: domainbilling.ErrInsufficientCredits}
	restoreBillingGuardService(t, service)
	inner := &billingGuardModelStub{}
	guard := wrapBillingGuardChatModel(inner, &RunSummary{RunID: 10, CreatorID: 20}, modelExecutorConfig{})
	ctx := context.WithValue(context.Background(), adkUsageCallContextKey, adkUsageCall{
		callID: "call-1", modelName: "model-1", provider: "provider-1",
	})

	_, err := guard.Generate(ctx, []*schema.Message{schema.UserMessage("hello")})
	if !errors.Is(err, domainbilling.ErrInsufficientCredits) {
		t.Fatalf("Generate() error = %v", err)
	}
	if inner.calls != 0 || service.reserveCalls != 1 {
		t.Fatalf("calls inner=%d reserve=%d", inner.calls, service.reserveCalls)
	}
}

func restoreBillingGuardService(t *testing.T, service billingGuardService) {
	t.Helper()
	original := billingGuardServiceProvider
	billingGuardServiceProvider = func() billingGuardService { return service }
	t.Cleanup(func() { billingGuardServiceProvider = original })
}
