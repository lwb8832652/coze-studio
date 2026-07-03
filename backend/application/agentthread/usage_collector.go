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
)

type AgentTokenUsage struct {
	Source       TokenUsageSource
	StepID       string
	StepIndex    int32
	StepName     string
	ToolName     string
	ModelName    string
	Provider     string
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CostMicros   int64
	Currency     string
	Estimated    bool
	RawUsage     string
	Metadata     string
}

type UsageCollector interface {
	Record(ctx context.Context, run *RunSummary, usage AgentTokenUsage) error
}

type ThreadUsageCollectorOptions struct {
	MetricsCollector RuntimeMetricsCollector
}

type ThreadUsageCollector struct {
	app              *ApplicationService
	metricsCollector RuntimeMetricsCollector
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

	return nil
}

func recordTokenUsageResponseUsage(resp *RecordTokenUsageResponse) TokenUsageSummary {
	if resp == nil || resp.Usage == nil {
		return TokenUsageSummary{}
	}

	return *resp.Usage
}
