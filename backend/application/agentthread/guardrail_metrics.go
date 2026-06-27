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

	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const agentGuardrailMetricsLogEnabledEnv = "AGENT_GUARDRAIL_METRICS_LOG_ENABLED"

type GuardrailMetricsCollector interface {
	RecordGuardrailEvaluation(
		ctx context.Context,
		observation GuardrailEvaluationMetricsObservation,
	)
}

type GuardrailEvaluationMetricsObservation struct {
	TargetType           string
	Operation            string
	Source               string
	FailMode             string
	Action               string
	Provider             string
	ErrorCode            string
	Allowed              bool
	Warning              bool
	RequiresConfirmation bool
	AuditRecorded        bool
	ElapsedMs            int64
}

type GuardrailLoggingMetricsCollector struct{}

func NewGuardrailMetricsCollectorFromEnv() GuardrailMetricsCollector {
	var collectors []GuardrailMetricsCollector
	if envkey.GetBoolD(agentGuardrailMetricsLogEnabledEnv, false) {
		collectors = append(collectors, &GuardrailLoggingMetricsCollector{})
	}
	if prometheusCollector := NewGuardrailPrometheusMetricsCollectorFromEnv(); prometheusCollector != nil {
		collectors = append(collectors, prometheusCollector)
	}

	return newGuardrailMetricsCollectorMux(collectors...)
}

func (c *GuardrailLoggingMetricsCollector) RecordGuardrailEvaluation(
	ctx context.Context,
	observation GuardrailEvaluationMetricsObservation,
) {
	if c == nil {
		return
	}

	logs.CtxInfof(
		ctx,
		"[guardrail-metrics] evaluation target_type=%s operation=%s source=%s fail_mode=%s action=%s provider=%s error_code=%s allowed=%t warning=%t requires_confirmation=%t audit_recorded=%t elapsed_ms=%d",
		observation.TargetType,
		observation.Operation,
		observation.Source,
		observation.FailMode,
		observation.Action,
		observation.Provider,
		observation.ErrorCode,
		observation.Allowed,
		observation.Warning,
		observation.RequiresConfirmation,
		observation.AuditRecorded,
		observation.ElapsedMs,
	)
}

func newGuardrailEvaluationMetricsObservation(
	request GuardrailRequest,
	result GuardrailEnforcementResult,
	auditRecorded bool,
	elapsedMs int64,
) GuardrailEvaluationMetricsObservation {
	if elapsedMs < 0 {
		elapsedMs = 0
	}
	decision := normalizeGuardrailDecision(result.Decision)

	return GuardrailEvaluationMetricsObservation{
		TargetType:           guardrailMetricsLabel(string(request.TargetType), "unknown", 32),
		Operation:            guardrailMetricsLabel(request.Operation, "unknown", 64),
		Source:               guardrailMetricsLabel(request.Source, "unknown", 64),
		FailMode:             guardrailMetricsLabel(string(request.FailMode), string(GuardrailFailClosed), 16),
		Action:               guardrailMetricsLabel(string(decision.Action), string(GuardrailActionDeny), 16),
		Provider:             guardrailMetricsLabel(decision.Provider, "guardrail", 64),
		ErrorCode:            sanitizeGuardrailIdentifier(result.ErrorCode, 64),
		Allowed:              result.Allowed,
		Warning:              result.Warning,
		RequiresConfirmation: result.RequiresConfirmation,
		AuditRecorded:        auditRecorded,
		ElapsedMs:            elapsedMs,
	}
}

func guardrailMetricsLabel(value string, fallback string, limit int) string {
	label := sanitizeGuardrailIdentifier(value, limit)
	if label == "" {
		return fallback
	}

	return label
}
