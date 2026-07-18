// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const sandboxPrometheusMetricsEnabledEnv = "SANDBOX_PROMETHEUS_METRICS_ENABLED"

type ProviderSelectionMetricsObservation struct {
	Scope        domainsandbox.Scope
	ProviderType domainsandbox.ProviderType
	Outcome      string
	ResultCode   string
}

type ProviderHealthMetricsObservation struct {
	ProviderType domainsandbox.ProviderType
	Status       domainsandbox.HealthStatus
	Outcome      string
	ResultCode   string
	Elapsed      time.Duration
}

type SandboxExecutionMetricsObservation struct {
	Scope        domainsandbox.Scope
	ProviderType domainsandbox.ProviderType
	Outcome      string
	ResultCode   string
	Elapsed      time.Duration
}

type CapacityRejectionMetricsObservation struct {
	Scope        domainsandbox.Scope
	ProviderType domainsandbox.ProviderType
	ResultCode   string
}

type CredentialDecryptFailureMetricsObservation struct {
	ProviderType domainsandbox.ProviderType
	Field        string
	ResultCode   string
}

// SandboxMetricsRecorder deliberately has no provider name/key, user,
// execution, endpoint, command, or payload fields, keeping metric labels low
// cardinality and content-free by construction.
type SandboxMetricsRecorder interface {
	RecordProviderSelection(context.Context, ProviderSelectionMetricsObservation)
	RecordProviderHealth(context.Context, ProviderHealthMetricsObservation)
	RecordExecution(context.Context, SandboxExecutionMetricsObservation)
	RecordCapacityRejection(context.Context, CapacityRejectionMetricsObservation)
	RecordCredentialDecryptFailure(context.Context, CredentialDecryptFailureMetricsObservation)
}

type SandboxPrometheusMetricsCollector struct {
	providerSelectionsTotal        *prometheus.CounterVec
	providerHealthChecksTotal      *prometheus.CounterVec
	providerHealthDurationMs       *prometheus.HistogramVec
	executionsTotal                *prometheus.CounterVec
	executionDurationMs            *prometheus.HistogramVec
	capacityRejectionsTotal        *prometheus.CounterVec
	credentialDecryptFailuresTotal *prometheus.CounterVec
}

var defaultSandboxPrometheusMetrics struct {
	once      sync.Once
	collector *SandboxPrometheusMetricsCollector
	err       error
}

func NewSandboxPrometheusMetricsCollector(
	registerer prometheus.Registerer,
) (*SandboxPrometheusMetricsCollector, error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	collector := &SandboxPrometheusMetricsCollector{}
	var err error
	collector.providerSelectionsTotal, err = registerSandboxCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "sandbox_provider_selections_total",
				Help:      "Total Sandbox provider selection outcomes.",
			},
			[]string{"provider_type", "scope", "outcome", "result_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.providerHealthChecksTotal, err = registerSandboxCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "sandbox_provider_health_checks_total",
				Help:      "Total Sandbox provider health-check outcomes.",
			},
			[]string{"provider_type", "status", "outcome", "result_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.providerHealthDurationMs, err = registerSandboxHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "sandbox_provider_health_check_duration_milliseconds",
				Help:      "Sandbox provider health-check duration in milliseconds.",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 30000, 60000},
			},
			[]string{"provider_type", "outcome", "result_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.executionsTotal, err = registerSandboxCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "sandbox_executions_total",
				Help:      "Total Sandbox execution outcomes.",
			},
			[]string{"provider_type", "scope", "outcome", "result_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.executionDurationMs, err = registerSandboxHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "sandbox_execution_duration_milliseconds",
				Help:      "Sandbox execution submission duration in milliseconds.",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 30000, 60000, 300000},
			},
			[]string{"provider_type", "scope", "outcome", "result_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.capacityRejectionsTotal, err = registerSandboxCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "sandbox_capacity_rejections_total",
				Help:      "Total Sandbox capacity acquisition rejections.",
			},
			[]string{"provider_type", "scope", "result_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.credentialDecryptFailuresTotal, err = registerSandboxCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "sandbox_credential_decrypt_failures_total",
				Help:      "Total Sandbox provider credential decryption failures.",
			},
			[]string{"provider_type", "field", "result_code"},
		),
	)
	if err != nil {
		return nil, err
	}
	return collector, nil
}

func NewSandboxPrometheusMetricsCollectorFromEnv() *SandboxPrometheusMetricsCollector {
	if !envkey.GetBoolD(sandboxPrometheusMetricsEnabledEnv, false) {
		return nil
	}
	defaultSandboxPrometheusMetrics.once.Do(func() {
		defaultSandboxPrometheusMetrics.collector, defaultSandboxPrometheusMetrics.err =
			NewSandboxPrometheusMetricsCollector(prometheus.DefaultRegisterer)
	})
	if defaultSandboxPrometheusMetrics.err != nil {
		logs.CtxWarnf(
			context.Background(),
			"[sandbox-prometheus-metrics] collector init failed",
		)
		return nil
	}
	return defaultSandboxPrometheusMetrics.collector
}

func (c *SandboxPrometheusMetricsCollector) RecordProviderSelection(
	_ context.Context,
	observation ProviderSelectionMetricsObservation,
) {
	if c == nil {
		return
	}
	c.providerSelectionsTotal.WithLabelValues(
		sandboxMetricLabel(string(observation.ProviderType), "unknown", 32),
		sandboxMetricLabel(string(observation.Scope), "unknown", 32),
		sandboxMetricLabel(observation.Outcome, "failure", 16),
		sandboxMetricLabel(observation.ResultCode, "unknown", 64),
	).Inc()
}

func (c *SandboxPrometheusMetricsCollector) RecordProviderHealth(
	_ context.Context,
	observation ProviderHealthMetricsObservation,
) {
	if c == nil {
		return
	}
	providerType := sandboxMetricLabel(string(observation.ProviderType), "unknown", 32)
	outcome := sandboxMetricLabel(observation.Outcome, "failure", 16)
	resultCode := sandboxMetricLabel(observation.ResultCode, "unknown", 64)
	c.providerHealthChecksTotal.WithLabelValues(
		providerType,
		sandboxMetricLabel(string(observation.Status), "unknown", 32),
		outcome,
		resultCode,
	).Inc()
	c.providerHealthDurationMs.WithLabelValues(providerType, outcome, resultCode).
		Observe(sandboxMetricMilliseconds(observation.Elapsed))
}

func (c *SandboxPrometheusMetricsCollector) RecordExecution(
	_ context.Context,
	observation SandboxExecutionMetricsObservation,
) {
	if c == nil {
		return
	}
	providerType := sandboxMetricLabel(string(observation.ProviderType), "unknown", 32)
	scope := sandboxMetricLabel(string(observation.Scope), "unknown", 32)
	outcome := sandboxMetricLabel(observation.Outcome, "failure", 16)
	resultCode := sandboxMetricLabel(observation.ResultCode, "unknown", 64)
	c.executionsTotal.WithLabelValues(providerType, scope, outcome, resultCode).Inc()
	c.executionDurationMs.WithLabelValues(providerType, scope, outcome, resultCode).
		Observe(sandboxMetricMilliseconds(observation.Elapsed))
}

func (c *SandboxPrometheusMetricsCollector) RecordCapacityRejection(
	_ context.Context,
	observation CapacityRejectionMetricsObservation,
) {
	if c == nil {
		return
	}
	c.capacityRejectionsTotal.WithLabelValues(
		sandboxMetricLabel(string(observation.ProviderType), "unknown", 32),
		sandboxMetricLabel(string(observation.Scope), "unknown", 32),
		sandboxMetricLabel(observation.ResultCode, "unknown", 64),
	).Inc()
}

func (c *SandboxPrometheusMetricsCollector) RecordCredentialDecryptFailure(
	_ context.Context,
	observation CredentialDecryptFailureMetricsObservation,
) {
	if c == nil {
		return
	}
	c.credentialDecryptFailuresTotal.WithLabelValues(
		sandboxMetricLabel(string(observation.ProviderType), "unknown", 32),
		sandboxMetricLabel(observation.Field, "unknown", 32),
		sandboxMetricLabel(observation.ResultCode, "unknown", 64),
	).Inc()
}

func sandboxMetricsOutcome(err error) string {
	if err == nil {
		return "success"
	}
	return "failure"
}

func sandboxMetricsResultCode(err error) string {
	if err == nil {
		return "none"
	}
	if code := domainsandbox.ErrorCodeOf(err); code != "" {
		return code
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	default:
		return "internal"
	}
}

func sandboxMetricMilliseconds(elapsed time.Duration) float64 {
	if elapsed < 0 {
		return 0
	}
	return float64(elapsed.Microseconds()) / 1000
}

func sandboxMetricLabel(value, fallback string, maxLength int) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxLength {
		return fallback
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character < 'A' || character > 'Z') &&
			(character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' &&
			character != '-' &&
			character != '.' {
			return fallback
		}
	}
	return value
}

func registerSandboxCounterVec(
	registerer prometheus.Registerer,
	collector *prometheus.CounterVec,
) (*prometheus.CounterVec, error) {
	if err := registerer.Register(collector); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing, nil
			}
		}
		return nil, err
	}
	return collector, nil
}

func registerSandboxHistogramVec(
	registerer prometheus.Registerer,
	collector *prometheus.HistogramVec,
) (*prometheus.HistogramVec, error) {
	if err := registerer.Register(collector); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing, nil
			}
		}
		return nil, err
	}
	return collector, nil
}
