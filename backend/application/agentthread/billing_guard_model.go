/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentthread

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	appbilling "github.com/coze-dev/coze-studio/backend/application/billing"
	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

const defaultBillingEstimateOutputTokens int64 = 8192

type billingGuardChatModel struct {
	inner           model.BaseChatModel
	run             *RunSummary
	maxOutputTokens int64
}

type billingGuardService interface {
	SettlementEnabled(ctx context.Context) (bool, error)
	ReserveUsage(ctx context.Context, input domainbilling.ReserveUsageInput) (string, int64, error)
}

var billingGuardServiceProvider = func() billingGuardService {
	service := appbilling.DefaultService()
	if service == nil {
		return nil
	}
	return service
}

type billingGuardToolChatModel struct {
	*billingGuardChatModel
	innerTool model.ToolCallingChatModel
}

type billingGuardLegacyChatModel struct {
	*billingGuardChatModel
	innerLegacy model.ChatModel
}

func wrapBillingGuardChatModel(
	inner model.BaseChatModel,
	run *RunSummary,
	cfg modelExecutorConfig,
) model.BaseChatModel {
	if inner == nil {
		return nil
	}
	maxOutputTokens := defaultBillingEstimateOutputTokens
	if cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
		maxOutputTokens = int64(*cfg.MaxTokens)
	}
	base := &billingGuardChatModel{
		inner:           inner,
		run:             run,
		maxOutputTokens: maxOutputTokens,
	}
	if toolModel, ok := inner.(model.ToolCallingChatModel); ok {
		return &billingGuardToolChatModel{billingGuardChatModel: base, innerTool: toolModel}
	}
	if legacyModel, ok := inner.(model.ChatModel); ok {
		return &billingGuardLegacyChatModel{billingGuardChatModel: base, innerLegacy: legacyModel}
	}
	return base
}

func (m *billingGuardChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.Message, error) {
	if err := m.reserve(ctx, input); err != nil {
		return nil, err
	}
	return m.inner.Generate(ctx, input, opts...)
}

func (m *billingGuardChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	if err := m.reserve(ctx, input); err != nil {
		return nil, err
	}
	return m.inner.Stream(ctx, input, opts...)
}

func (m *billingGuardToolChatModel) WithTools(
	tools []*schema.ToolInfo,
) (model.ToolCallingChatModel, error) {
	bound, err := m.innerTool.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &billingGuardToolChatModel{
		billingGuardChatModel: &billingGuardChatModel{
			inner:           bound,
			run:             m.run,
			maxOutputTokens: m.maxOutputTokens,
		},
		innerTool: bound,
	}, nil
}

func (m *billingGuardLegacyChatModel) BindTools(tools []*schema.ToolInfo) error {
	return m.innerLegacy.BindTools(tools)
}

func (m *billingGuardChatModel) reserve(ctx context.Context, input []*schema.Message) error {
	service := billingGuardServiceProvider()
	if service == nil {
		return nil
	}
	enabled, err := service.SettlementEnabled(ctx)
	if err != nil {
		return fmt.Errorf("read billing settlement policy before model call: %w", err)
	}
	if !enabled {
		return nil
	}
	if m == nil || m.run == nil || m.run.CreatorID <= 0 {
		return fmt.Errorf("billing subject is unavailable before model call")
	}
	call, ok := ctx.Value(adkUsageCallContextKey).(adkUsageCall)
	if !ok || strings.TrimSpace(call.callID) == "" || strings.TrimSpace(call.modelName) == "" || strings.TrimSpace(call.provider) == "" {
		return fmt.Errorf("billing model identity is unavailable before model call")
	}

	_, _, err = service.ReserveUsage(ctx, domainbilling.ReserveUsageInput{
		Subject:               domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: m.run.CreatorID},
		RunID:                 fmt.Sprintf("%d", m.run.RunID),
		UsageSequence:         1,
		Provider:              call.provider,
		ModelID:               call.modelName,
		ReservationBusinessNo: billingUsageReservationBusinessNo(m.run.RunID, adkUsageIdempotencyKey(m.run.RunID, call)),
		Usage: domainbilling.TokenUsage{
			Input:  estimateBillingInputTokens(input),
			Output: m.maxOutputTokens,
		},
	})
	if err != nil {
		return fmt.Errorf("reserve credits before model call: %w", err)
	}
	return nil
}

func billingUsageReservationBusinessNo(runID int64, idempotencyKey string) string {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("run-%d", runID)
	}
	return "usage-reserve:" + idempotencyKey
}

func estimateBillingInputTokens(messages []*schema.Message) int64 {
	var estimate int64
	for _, message := range messages {
		if message == nil {
			continue
		}
		runes := utf8.RuneCountInString(message.Content)
		estimate += int64((runes+1)/2 + 16)
	}
	if estimate < 128 {
		return 128
	}
	return estimate
}

var _ model.BaseChatModel = (*billingGuardChatModel)(nil)
var _ model.ToolCallingChatModel = (*billingGuardToolChatModel)(nil)
var _ model.ChatModel = (*billingGuardLegacyChatModel)(nil)
