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
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const agentGuardrailPrometheusMetricsEnabledEnv = "AGENT_GUARDRAIL_PROMETHEUS_METRICS_ENABLED"

type GuardrailPrometheusMetricsCollector struct {
	evaluationsTotal       *prometheus.CounterVec
	evaluationLatencyMs    *prometheus.HistogramVec
	archiveAttemptsTotal   *prometheus.CounterVec
	archiveRowsTotal       *prometheus.CounterVec
	archiveLatencyMs       *prometheus.HistogramVec
	retentionAttemptsTotal *prometheus.CounterVec
	retentionRowsTotal     *prometheus.CounterVec
	retentionLatencyMs     *prometheus.HistogramVec
}

var defaultGuardrailPrometheusMetrics struct {
	once      sync.Once
	collector *GuardrailPrometheusMetricsCollector
	err       error
}

func NewGuardrailPrometheusMetricsCollector(
	registerer prometheus.Registerer,
) (*GuardrailPrometheusMetricsCollector, error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	collector := &GuardrailPrometheusMetricsCollector{}
	var err error
	collector.evaluationsTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "guardrail_evaluations_total",
				Help:      "Total number of Guardrail evaluations.",
			},
			[]string{
				"target_type",
				"operation",
				"source",
				"fail_mode",
				"action",
				"provider",
				"error_code",
				"allowed",
				"warning",
				"requires_confirmation",
				"audit_recorded",
			},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.evaluationLatencyMs, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "guardrail_evaluation_latency_ms",
				Help:      "Guardrail evaluation latency in milliseconds.",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000},
			},
			[]string{
				"target_type",
				"operation",
				"source",
				"fail_mode",
				"action",
				"provider",
				"error_code",
			},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.archiveAttemptsTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "guardrail_audit_archive_attempts_total",
				Help:      "Total number of Guardrail audit archive attempts.",
			},
			guardrailPrometheusOutcomeLabels(),
		),
	)
	if err != nil {
		return nil, err
	}
	collector.archiveRowsTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "guardrail_audit_archive_rows_total",
				Help:      "Total number of Guardrail audit rows archived.",
			},
			[]string{"row_kind"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.archiveLatencyMs, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "guardrail_audit_archive_latency_ms",
				Help:      "Guardrail audit archive latency in milliseconds.",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 30000},
			},
			guardrailPrometheusOutcomeLabels(),
		),
	)
	if err != nil {
		return nil, err
	}
	collector.retentionAttemptsTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "guardrail_audit_retention_attempts_total",
				Help:      "Total number of Guardrail audit retention cleanup attempts.",
			},
			guardrailPrometheusOutcomeLabels(),
		),
	)
	if err != nil {
		return nil, err
	}
	collector.retentionRowsTotal, err = registerCounterVec(
		registerer,
		prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "coze",
				Name:      "guardrail_audit_retention_rows_total",
				Help:      "Total number of Guardrail audit rows processed by retention cleanup.",
			},
			[]string{"row_kind"},
		),
	)
	if err != nil {
		return nil, err
	}
	collector.retentionLatencyMs, err = registerHistogramVec(
		registerer,
		prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "coze",
				Name:      "guardrail_audit_retention_latency_ms",
				Help:      "Guardrail audit retention cleanup latency in milliseconds.",
				Buckets:   []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000, 30000},
			},
			guardrailPrometheusOutcomeLabels(),
		),
	)
	if err != nil {
		return nil, err
	}

	return collector, nil
}

func NewGuardrailPrometheusMetricsCollectorFromEnv() *GuardrailPrometheusMetricsCollector {
	if !envkey.GetBoolD(agentGuardrailPrometheusMetricsEnabledEnv, false) {
		return nil
	}
	defaultGuardrailPrometheusMetrics.once.Do(func() {
		defaultGuardrailPrometheusMetrics.collector, defaultGuardrailPrometheusMetrics.err =
			NewGuardrailPrometheusMetricsCollector(prometheus.DefaultRegisterer)
	})
	if defaultGuardrailPrometheusMetrics.err != nil {
		logs.CtxWarnf(
			context.Background(),
			"[guardrail-prometheus-metrics] collector init failed: %v",
			defaultGuardrailPrometheusMetrics.err,
		)
		return nil
	}

	return defaultGuardrailPrometheusMetrics.collector
}

func (c *GuardrailPrometheusMetricsCollector) RecordGuardrailEvaluation(
	ctx context.Context,
	observation GuardrailEvaluationMetricsObservation,
) {
	if c == nil {
		return
	}
	labels := prometheus.Labels{
		"target_type":           guardrailMetricsLabel(observation.TargetType, "unknown", 32),
		"operation":             guardrailMetricsLabel(observation.Operation, "unknown", 64),
		"source":                guardrailMetricsLabel(observation.Source, "unknown", 64),
		"fail_mode":             guardrailMetricsLabel(observation.FailMode, string(GuardrailFailClosed), 16),
		"action":                guardrailMetricsLabel(observation.Action, string(GuardrailActionDeny), 16),
		"provider":              guardrailMetricsLabel(observation.Provider, "guardrail", 64),
		"error_code":            guardrailMetricsLabel(observation.ErrorCode, "none", 64),
		"allowed":               strconv.FormatBool(observation.Allowed),
		"warning":               strconv.FormatBool(observation.Warning),
		"requires_confirmation": strconv.FormatBool(observation.RequiresConfirmation),
		"audit_recorded":        strconv.FormatBool(observation.AuditRecorded),
	}
	c.evaluationsTotal.With(labels).Inc()
	c.evaluationLatencyMs.With(prometheus.Labels{
		"target_type": labels["target_type"],
		"operation":   labels["operation"],
		"source":      labels["source"],
		"fail_mode":   labels["fail_mode"],
		"action":      labels["action"],
		"provider":    labels["provider"],
		"error_code":  labels["error_code"],
	}).Observe(float64(nonNegativeGuardrailMetricMs(observation.ElapsedMs)))
}

func (c *GuardrailPrometheusMetricsCollector) RecordGuardrailAuditArchive(
	ctx context.Context,
	observation GuardrailAuditArchiveMetricsObservation,
) {
	if c == nil {
		return
	}
	labels := guardrailPrometheusOutcomeLabelValues(
		observation.Success,
		observation.ErrorCode,
		observation.Skipped,
		observation.SkipReason,
	)
	c.archiveAttemptsTotal.With(labels).Inc()
	if observation.Archived > 0 {
		c.archiveRowsTotal.WithLabelValues("archived").Add(float64(observation.Archived))
	}
	c.archiveLatencyMs.With(labels).Observe(float64(nonNegativeGuardrailMetricMs(observation.ElapsedMs)))
}

func (c *GuardrailPrometheusMetricsCollector) RecordGuardrailAuditRetention(
	ctx context.Context,
	observation GuardrailAuditRetentionMetricsObservation,
) {
	if c == nil {
		return
	}
	labels := guardrailPrometheusOutcomeLabelValues(
		observation.Success,
		observation.ErrorCode,
		observation.Skipped,
		observation.SkipReason,
	)
	c.retentionAttemptsTotal.With(labels).Inc()
	if observation.Archived > 0 {
		c.retentionRowsTotal.WithLabelValues("archived").Add(float64(observation.Archived))
	}
	if observation.Deleted > 0 {
		c.retentionRowsTotal.WithLabelValues("deleted").Add(float64(observation.Deleted))
	}
	c.retentionLatencyMs.With(labels).Observe(float64(nonNegativeGuardrailMetricMs(observation.ElapsedMs)))
}

type guardrailMetricsCollectorMux struct {
	collectors []GuardrailMetricsCollector
}

func newGuardrailMetricsCollectorMux(
	collectors ...GuardrailMetricsCollector,
) GuardrailMetricsCollector {
	filtered := make([]GuardrailMetricsCollector, 0, len(collectors))
	for _, collector := range collectors {
		if collector != nil {
			filtered = append(filtered, collector)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return &guardrailMetricsCollectorMux{collectors: filtered}
}

func (m *guardrailMetricsCollectorMux) RecordGuardrailEvaluation(
	ctx context.Context,
	observation GuardrailEvaluationMetricsObservation,
) {
	if m == nil {
		return
	}
	for _, collector := range m.collectors {
		collector.RecordGuardrailEvaluation(ctx, observation)
	}
}

type guardrailAuditArchiveMetricsCollectorMux struct {
	collectors []GuardrailAuditArchiveMetricsCollector
}

func newGuardrailAuditArchiveMetricsCollectorMux(
	collectors ...GuardrailAuditArchiveMetricsCollector,
) GuardrailAuditArchiveMetricsCollector {
	filtered := make([]GuardrailAuditArchiveMetricsCollector, 0, len(collectors))
	for _, collector := range collectors {
		if collector != nil {
			filtered = append(filtered, collector)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return &guardrailAuditArchiveMetricsCollectorMux{collectors: filtered}
}

func (m *guardrailAuditArchiveMetricsCollectorMux) RecordGuardrailAuditArchive(
	ctx context.Context,
	observation GuardrailAuditArchiveMetricsObservation,
) {
	if m == nil {
		return
	}
	for _, collector := range m.collectors {
		collector.RecordGuardrailAuditArchive(ctx, observation)
	}
}

type guardrailAuditRetentionMetricsCollectorMux struct {
	collectors []GuardrailAuditRetentionMetricsCollector
}

func newGuardrailAuditRetentionMetricsCollectorMux(
	collectors ...GuardrailAuditRetentionMetricsCollector,
) GuardrailAuditRetentionMetricsCollector {
	filtered := make([]GuardrailAuditRetentionMetricsCollector, 0, len(collectors))
	for _, collector := range collectors {
		if collector != nil {
			filtered = append(filtered, collector)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return &guardrailAuditRetentionMetricsCollectorMux{collectors: filtered}
}

func (m *guardrailAuditRetentionMetricsCollectorMux) RecordGuardrailAuditRetention(
	ctx context.Context,
	observation GuardrailAuditRetentionMetricsObservation,
) {
	if m == nil {
		return
	}
	for _, collector := range m.collectors {
		collector.RecordGuardrailAuditRetention(ctx, observation)
	}
}

func guardrailPrometheusOutcomeLabels() []string {
	return []string{"success", "error_code", "skipped", "skip_reason"}
}

func guardrailPrometheusOutcomeLabelValues(
	success bool,
	errorCode string,
	skipped bool,
	skipReason string,
) prometheus.Labels {
	return prometheus.Labels{
		"success":     strconv.FormatBool(success),
		"error_code":  guardrailMetricsLabel(errorCode, "none", 64),
		"skipped":     strconv.FormatBool(skipped),
		"skip_reason": guardrailMetricsLabel(skipReason, "none", 64),
	}
}

func nonNegativeGuardrailMetricMs(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func registerCounterVec(
	registerer prometheus.Registerer,
	collector *prometheus.CounterVec,
) (*prometheus.CounterVec, error) {
	if err := registerer.Register(collector); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := already.ExistingCollector.(*prometheus.CounterVec)
			if ok {
				return existing, nil
			}
		}
		return nil, err
	}
	return collector, nil
}

func registerHistogramVec(
	registerer prometheus.Registerer,
	collector *prometheus.HistogramVec,
) (*prometheus.HistogramVec, error) {
	if err := registerer.Register(collector); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := already.ExistingCollector.(*prometheus.HistogramVec)
			if ok {
				return existing, nil
			}
		}
		return nil, err
	}
	return collector, nil
}

func registerGaugeVec(
	registerer prometheus.Registerer,
	collector *prometheus.GaugeVec,
) (*prometheus.GaugeVec, error) {
	if err := registerer.Register(collector); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := already.ExistingCollector.(*prometheus.GaugeVec)
			if ok {
				return existing, nil
			}
		}
		return nil, err
	}
	return collector, nil
}
