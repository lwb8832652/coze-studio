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
	"strconv"

	appbilling "github.com/coze-dev/coze-studio/backend/application/billing"
	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
)

type AgentTokenUsage struct {
	Source         TokenUsageSource
	IdempotencyKey string
	StepID         string
	StepIndex      int32
	StepName       string
	ToolName       string
	ModelName      string
	Provider       string
	InputTokens    int64
	OutputTokens   int64
	TotalTokens    int64
	CostMicros     int64
	Currency       string
	Estimated      bool
	RawUsage       string
	Metadata       string
}

type UsageCollector interface {
	Record(ctx context.Context, run *RunSummary, usage AgentTokenUsage) error
}

type ThreadUsageCollectorOptions struct {
	MetricsCollector RuntimeMetricsCollector
	EventSink        RunEventSink
}

type ThreadUsageCollector struct {
	app              *ApplicationService
	metricsCollector RuntimeMetricsCollector
	eventSink        RunEventSink
}

func NewThreadUsageCollector(app *ApplicationService) *ThreadUsageCollector {
	return NewThreadUsageCollectorWithOptions(app, ThreadUsageCollectorOptions{})
}

func NewThreadUsageCollectorWithOptions(
	app *ApplicationService,
	opts ThreadUsageCollectorOptions,
) *ThreadUsageCollector {
	return &ThreadUsageCollector{
		app:              app,
		metricsCollector: opts.MetricsCollector,
		eventSink:        opts.EventSink,
	}
}

func (c *ThreadUsageCollector) Record(ctx context.Context, run *RunSummary, usage AgentTokenUsage) error {
	if c == nil || c.app == nil {
		return fmt.Errorf("agent thread usage collector application service is required")
	}
	if run == nil {
		return fmt.Errorf("run is required")
	}
	if run.RunID <= 0 {
		return fmt.Errorf("run id is required")
	}

	source := usage.Source
	if source == "" {
		source = TokenUsageSourceLeadAgent
	}
	stepName := usage.StepName
	if stepName == "" {
		stepName = usage.ToolName
	}

	resp, err := c.app.RecordTokenUsage(ctx, &RecordTokenUsageRequest{
		RunID:        run.RunID,
		Source:       source,
		StepID:       usage.StepID,
		StepIndex:    usage.StepIndex,
		StepName:     stepName,
		ModelName:    usage.ModelName,
		Provider:     usage.Provider,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
		CostMicros:   usage.CostMicros,
		Currency:     usage.Currency,
		Estimated:    usage.Estimated,
		RawUsage:     usage.RawUsage,
		Metadata:     usage.Metadata,
	})
	if err != nil {
		return err
	}
	recorded := recordTokenUsageResponseUsage(resp)
	recordRuntimeTokenUsage(
		ctx,
		c.metricsCollector,
		run,
		recorded.Source,
		recorded.ModelName,
		recorded.InputTokens,
		recorded.OutputTokens,
	)
	c.emitTokenUsageSnapshot(ctx, run, recorded)
	if err := recordBillableTokenUsage(ctx, run, recorded, usage.IdempotencyKey); err != nil {
		return err
	}

	return nil
}

func recordBillableTokenUsage(
	ctx context.Context,
	run *RunSummary,
	usage TokenUsageSummary,
	idempotencyKey string,
) error {
	service := appbilling.DefaultService()
	if service == nil {
		return nil
	}
	enabled, err := service.SettlementEnabled(ctx)
	if err != nil {
		return fmt.Errorf("read billing settlement policy: %w", err)
	}
	if !enabled {
		return nil
	}
	if run == nil || run.CreatorID <= 0 || usage.UsageID <= 0 || usage.ModelName == "" || usage.Provider == "" {
		return fmt.Errorf("billable token usage identity is incomplete")
	}

	runID := strconv.FormatInt(run.RunID, 10)
	sequence := usage.UsageID
	subject := domainbilling.Subject{Type: domainbilling.SubjectTypeUser, ID: run.CreatorID}
	tokens := domainbilling.TokenUsage{Input: usage.InputTokens, Output: usage.OutputTokens}
	reservationBusinessNo, _, err := service.ReserveUsage(ctx, domainbilling.ReserveUsageInput{
		Subject:               subject,
		RunID:                 runID,
		UsageSequence:         sequence,
		Provider:              usage.Provider,
		ModelID:               usage.ModelName,
		ReservationBusinessNo: billingUsageReservationBusinessNo(run.RunID, idempotencyKey),
		Usage:                 tokens,
	})
	if err != nil {
		return fmt.Errorf("reserve usage credits: %w", err)
	}
	_, err = service.SettleUsage(ctx, domainbilling.SettleUsageInput{
		Subject:               subject,
		UserID:                run.CreatorID,
		SpaceID:               run.SpaceID,
		RunID:                 runID,
		UsageSequence:         sequence,
		Provider:              usage.Provider,
		ModelID:               usage.ModelName,
		ReservationBusinessNo: reservationBusinessNo,
		Usage:                 tokens,
	})
	if err != nil {
		return fmt.Errorf("settle usage credits: %w", err)
	}
	return nil
}

func (c *ThreadUsageCollector) emitTokenUsageSnapshot(
	ctx context.Context,
	run *RunSummary,
	usage TokenUsageSummary,
) {
	if c == nil || c.eventSink == nil || run == nil || usage.TotalTokens <= 0 {
		return
	}

	threadID := usage.ThreadID
	if threadID <= 0 {
		threadID = run.ThreadID
	}
	runID := usage.RunID
	if runID <= 0 {
		runID = run.RunID
	}

	emitRunEvent(ctx, c.eventSink, RunEvent{
		ThreadID:  threadID,
		RunID:     runID,
		EventType: "token_usage.snapshot",
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"usage_id":      usage.UsageID,
			"run_id":        runID,
			"source":        string(usage.Source),
			"step_id":       usage.StepID,
			"step_index":    usage.StepIndex,
			"step_name":     usage.StepName,
			"model_name":    usage.ModelName,
			"provider":      usage.Provider,
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
			"total_tokens":  usage.TotalTokens,
			"cost_micros":   usage.CostMicros,
			"currency":      usage.Currency,
			"estimated":     usage.Estimated,
			"created_at":    usage.CreatedAt,
		}),
	})
}

func recordTokenUsageResponseUsage(resp *RecordTokenUsageResponse) TokenUsageSummary {
	if resp == nil || resp.Usage == nil {
		return TokenUsageSummary{}
	}

	return *resp.Usage
}
