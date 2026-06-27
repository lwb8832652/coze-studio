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
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGuardrailEnforcerAllowsAndWarnsAfterAudit(t *testing.T) {
	tests := []struct {
		name     string
		action   GuardrailAction
		wantWarn bool
	}{
		{name: "allow", action: GuardrailActionAllow},
		{name: "warn", action: GuardrailActionWarn, wantWarn: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			audit := &recordingGuardrailDecisionAuditRecorder{}
			enforcer := NewGuardrailEnforcer(GuardrailEnforcerOptions{
				Provider: GuardrailProviderFunc(
					func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
						return GuardrailDecision{
							Action:     testCase.action,
							Provider:   "policy",
							ReasonCode: "safe",
						}, nil
					},
				),
				AuditRecorder: audit,
			})

			result, err := enforcer.Evaluate(
				context.Background(),
				sampleGuardrailEnforcementRequest(),
			)

			require.NoError(t, err)
			require.True(t, result.Allowed)
			require.Equal(t, testCase.wantWarn, result.Warning)
			require.False(t, result.RequiresConfirmation)
			require.Equal(t, testCase.action, result.Decision.Action)
			require.Len(t, audit.records, 1)
			require.Equal(t, testCase.action, audit.records[0].decision.Action)
		})
	}
}

func TestGuardrailEnforcerDenyAndConfirmReturnTypedErrors(t *testing.T) {
	tests := []struct {
		name       string
		action     GuardrailAction
		wantCode   string
		assertType func(t *testing.T, err error)
	}{
		{
			name:     "deny",
			action:   GuardrailActionDeny,
			wantCode: "guardrail_denied",
			assertType: func(t *testing.T, err error) {
				t.Helper()
				var denied *GuardrailDeniedError
				require.ErrorAs(t, err, &denied)
			},
		},
		{
			name:     "confirm",
			action:   GuardrailActionConfirm,
			wantCode: "guardrail_confirmation_required",
			assertType: func(t *testing.T, err error) {
				t.Helper()
				var confirmation *GuardrailConfirmationRequiredError
				require.ErrorAs(t, err, &confirmation)
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			audit := &recordingGuardrailDecisionAuditRecorder{}
			enforcer := NewGuardrailEnforcer(GuardrailEnforcerOptions{
				Provider: GuardrailProviderFunc(
					func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
						return GuardrailDecision{
							Action:     testCase.action,
							Provider:   "scanner /mnt/raw",
							ReasonCode: "high risk",
							Message:    "secret sk-secret prompt",
						}, nil
					},
				),
				AuditRecorder: audit,
			})

			result, err := enforcer.Evaluate(
				context.Background(),
				sampleGuardrailEnforcementRequest(),
			)

			require.Error(t, err)
			testCase.assertType(t, err)
			require.Contains(t, err.Error(), testCase.wantCode)
			require.NotContains(t, err.Error(), "sk-secret")
			require.NotContains(t, err.Error(), "/mnt/raw")
			require.False(t, result.Allowed)
			require.Equal(t, testCase.action == GuardrailActionConfirm, result.RequiresConfirmation)
			require.Equal(t, testCase.wantCode, result.ErrorCode)
			require.Len(t, audit.records, 1)
		})
	}
}

func TestGuardrailEnforcerAuditFailureFailsClosed(t *testing.T) {
	enforcer := NewGuardrailEnforcer(GuardrailEnforcerOptions{
		Provider: GuardrailProviderFunc(
			func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
				return GuardrailDecision{
					Action:     GuardrailActionAllow,
					Provider:   "policy",
					ReasonCode: "safe",
				}, nil
			},
		),
		AuditRecorder: &recordingGuardrailDecisionAuditRecorder{
			err: errors.New("audit database leaked sk-secret /mnt/raw"),
		},
	})

	result, err := enforcer.Evaluate(
		context.Background(),
		sampleGuardrailEnforcementRequest(),
	)

	require.Error(t, err)
	var auditErr *GuardrailAuditFailedError
	require.ErrorAs(t, err, &auditErr)
	require.False(t, result.Allowed)
	require.Equal(t, "guardrail_audit_failed", result.ErrorCode)
	require.NotContains(t, err.Error(), "sk-secret")
	require.NotContains(t, err.Error(), "/mnt/raw")
}

func TestGuardrailEnforcerRecordsContentFreeMetrics(t *testing.T) {
	metrics := &recordingGuardrailMetricsCollector{}
	enforcer := NewGuardrailEnforcer(GuardrailEnforcerOptions{
		Provider: GuardrailProviderFunc(
			func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
				return GuardrailDecision{
					Action:     GuardrailActionDeny,
					Provider:   "scanner /mnt/raw",
					ReasonCode: "high risk",
					Message:    "secret sk-secret prompt",
				}, nil
			},
		),
		AuditRecorder:    &recordingGuardrailDecisionAuditRecorder{},
		MetricsCollector: metrics,
		NowMillis:        newSequenceMillis(1000, 1017),
	})

	result, err := enforcer.Evaluate(
		context.Background(),
		sampleGuardrailEnforcementRequest(),
	)

	require.Error(t, err)
	require.False(t, result.Allowed)
	require.Len(t, metrics.evaluations, 1)
	got := metrics.evaluations[0]
	require.Equal(t, "tool_call", got.TargetType)
	require.Equal(t, "invoke", got.Operation)
	require.Equal(t, "adk_tool_wrapper", got.Source)
	require.Equal(t, "fail_closed", got.FailMode)
	require.Equal(t, "deny", got.Action)
	require.Equal(t, "scanner_mnt_raw", got.Provider)
	require.Equal(t, "guardrail_denied", got.ErrorCode)
	require.False(t, got.Allowed)
	require.False(t, got.Warning)
	require.False(t, got.RequiresConfirmation)
	require.True(t, got.AuditRecorded)
	require.Equal(t, int64(17), got.ElapsedMs)

	serialized := strings.Join([]string{
		got.TargetType,
		got.Operation,
		got.Source,
		got.FailMode,
		got.Action,
		got.Provider,
		got.ErrorCode,
	}, " ")
	require.NotContains(t, serialized, "runtime_tool:search_docs")
	require.NotContains(t, serialized, "sk-secret")
	require.NotContains(t, serialized, "secret prompt")
	require.NotContains(t, serialized, "/mnt/raw")
}

func TestGuardrailEnforcerMetricsRecordAuditFailures(t *testing.T) {
	metrics := &recordingGuardrailMetricsCollector{}
	enforcer := NewGuardrailEnforcer(GuardrailEnforcerOptions{
		Provider: GuardrailProviderFunc(
			func(context.Context, GuardrailRequest) (GuardrailDecision, error) {
				return GuardrailDecision{
					Action:     GuardrailActionAllow,
					Provider:   "policy",
					ReasonCode: "safe",
				}, nil
			},
		),
		AuditRecorder: &recordingGuardrailDecisionAuditRecorder{
			err: errors.New("audit database leaked sk-secret /mnt/raw"),
		},
		MetricsCollector: metrics,
	})

	result, err := enforcer.Evaluate(
		context.Background(),
		sampleGuardrailEnforcementRequest(),
	)

	require.Error(t, err)
	require.False(t, result.Allowed)
	require.Len(t, metrics.evaluations, 1)
	got := metrics.evaluations[0]
	require.Equal(t, "allow", got.Action)
	require.Equal(t, "guardrail_audit_failed", got.ErrorCode)
	require.False(t, got.Allowed)
	require.False(t, got.AuditRecorded)
}

func TestGuardrailMetricsCollectorFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentGuardrailMetricsLogEnabledEnv, "")

	collector := NewGuardrailMetricsCollectorFromEnv()

	require.Nil(t, collector)
}

func TestGuardrailMetricsCollectorFromEnvBuildsLoggingCollector(t *testing.T) {
	t.Setenv(agentGuardrailMetricsLogEnabledEnv, "true")

	collector := NewGuardrailMetricsCollectorFromEnv()

	require.NotNil(t, collector)
	_, ok := collector.(*GuardrailLoggingMetricsCollector)
	require.True(t, ok)
}

func sampleGuardrailEnforcementRequest() GuardrailRequest {
	return GuardrailRequest{
		SpaceID:    30,
		ThreadID:   10,
		RunID:      20,
		UserID:     40,
		TargetType: GuardrailTargetToolCall,
		TargetID:   "runtime_tool:search_docs",
		Operation:  "invoke",
		Source:     "adk_tool_wrapper",
		FailMode:   GuardrailFailClosed,
	}
}

func newSequenceMillis(values ...int64) func() int64 {
	index := 0
	return func() int64 {
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

type recordingGuardrailDecisionAuditRecorder struct {
	records []recordedGuardrailDecision
	err     error
}

type recordedGuardrailDecision struct {
	request  GuardrailRequest
	decision GuardrailDecision
}

func (r *recordingGuardrailDecisionAuditRecorder) RecordGuardrailDecision(
	ctx context.Context,
	request GuardrailRequest,
	decision GuardrailDecision,
) error {
	r.records = append(r.records, recordedGuardrailDecision{
		request:  request,
		decision: decision,
	})
	return r.err
}

type recordingGuardrailMetricsCollector struct {
	evaluations []GuardrailEvaluationMetricsObservation
}

func (c *recordingGuardrailMetricsCollector) RecordGuardrailEvaluation(
	ctx context.Context,
	observation GuardrailEvaluationMetricsObservation,
) {
	c.evaluations = append(c.evaluations, observation)
}
