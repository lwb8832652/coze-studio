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
	"strings"
	"time"
)

type GuardrailEnforcerOptions struct {
	Provider         GuardrailProvider
	AuditRecorder    GuardrailAuditRecorder
	MetricsCollector GuardrailMetricsCollector
	NowMillis        func() int64
}

type GuardrailEnforcer struct {
	provider         GuardrailProvider
	auditRecorder    GuardrailAuditRecorder
	metricsCollector GuardrailMetricsCollector
	nowMillis        func() int64
}

type GuardrailEnforcementResult struct {
	Decision             GuardrailDecision
	Allowed              bool
	Warning              bool
	RequiresConfirmation bool
	ErrorCode            string
}

type GuardrailDeniedError struct {
	ReasonCode string
}

func (e *GuardrailDeniedError) Error() string {
	return guardrailEnforcementErrorMessage(
		"guardrail_denied",
		e.ReasonCode,
	)
}

type GuardrailConfirmationRequiredError struct {
	ReasonCode string
}

func (e *GuardrailConfirmationRequiredError) Error() string {
	return guardrailEnforcementErrorMessage(
		"guardrail_confirmation_required",
		e.ReasonCode,
	)
}

type GuardrailConfirmationRejectedError struct {
	ReasonCode string
}

func (e *GuardrailConfirmationRejectedError) Error() string {
	return guardrailEnforcementErrorMessage(
		"guardrail_confirmation_rejected",
		e.ReasonCode,
	)
}

type GuardrailAuditFailedError struct{}

func (e *GuardrailAuditFailedError) Error() string {
	return "guardrail_audit_failed"
}

func NewGuardrailEnforcer(
	options GuardrailEnforcerOptions,
) *GuardrailEnforcer {
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &GuardrailEnforcer{
		provider:         options.Provider,
		auditRecorder:    options.AuditRecorder,
		metricsCollector: options.MetricsCollector,
		nowMillis:        nowMillis,
	}
}

func (e *GuardrailEnforcer) Evaluate(
	ctx context.Context,
	request GuardrailRequest,
) (result GuardrailEnforcementResult, err error) {
	startedAt := e.now()
	auditRecorded := false
	defer func() {
		e.recordEvaluationMetrics(
			ctx,
			request,
			result,
			auditRecorded,
			startedAt,
		)
	}()

	decision := e.evaluateDecision(ctx, request)
	result = GuardrailEnforcementResult{Decision: decision}
	if e == nil || e.auditRecorder == nil {
		result.ErrorCode = "guardrail_audit_failed"
		err = &GuardrailAuditFailedError{}
		return result, err
	}
	if recordErr := e.auditRecorder.RecordGuardrailDecision(
		ctx,
		request,
		decision,
	); recordErr != nil {
		result.ErrorCode = "guardrail_audit_failed"
		err = &GuardrailAuditFailedError{}
		return result, err
	}
	auditRecorded = true

	switch decision.Action {
	case GuardrailActionAllow:
		result.Allowed = true
		return result, nil
	case GuardrailActionWarn:
		result.Allowed = true
		result.Warning = true
		return result, nil
	case GuardrailActionConfirm:
		result.RequiresConfirmation = true
		result.ErrorCode = "guardrail_confirmation_required"
		err = &GuardrailConfirmationRequiredError{
			ReasonCode: decision.ReasonCode,
		}
		return result, err
	default:
		result.ErrorCode = "guardrail_denied"
		err = &GuardrailDeniedError{
			ReasonCode: decision.ReasonCode,
		}
		return result, err
	}
}

func (e *GuardrailEnforcer) recordEvaluationMetrics(
	ctx context.Context,
	request GuardrailRequest,
	result GuardrailEnforcementResult,
	auditRecorded bool,
	startedAt int64,
) {
	if e == nil || e.metricsCollector == nil {
		return
	}
	e.metricsCollector.RecordGuardrailEvaluation(
		ctx,
		newGuardrailEvaluationMetricsObservation(
			request,
			result,
			auditRecorded,
			e.now()-startedAt,
		),
	)
}

func (e *GuardrailEnforcer) evaluateDecision(
	ctx context.Context,
	request GuardrailRequest,
) GuardrailDecision {
	if e == nil || e.provider == nil {
		return normalizeGuardrailDecision(GuardrailDecision{
			Action:     GuardrailActionAllow,
			Provider:   "guardrail",
			ReasonCode: "no_provider",
		})
	}
	decision, err := e.provider.EvaluateGuardrail(ctx, request)
	if err != nil {
		return normalizeGuardrailDecision(
			guardrailProviderErrorDecision(request.FailMode),
		)
	}

	return normalizeGuardrailDecision(decision)
}

func (e *GuardrailEnforcer) now() int64 {
	if e == nil || e.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return e.nowMillis()
}

func guardrailEnforcementErrorMessage(code string, reasonCode string) string {
	reasonCode = sanitizeGuardrailIdentifier(reasonCode, 64)
	if strings.TrimSpace(reasonCode) == "" {
		return code
	}
	return code + ": " + reasonCode
}
