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
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/prometheus/client_golang/prometheus"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	journalMetricDedupeTTL = time.Hour
	journalMetricDedupeMax = 100_000
)

var journalPrometheusLabelNames = []string{
	"version",
	"rollout_cohort",
	"task_type",
	"client_version",
	"result",
	"error_code",
}

const agentJournalPrometheusMetricsEnabledEnv = "AGENT_JOURNAL_PROMETHEUS_METRICS_ENABLED"

var defaultJournalPrometheusMetrics struct {
	once      sync.Once
	collector *JournalPrometheusMetricsCollector
	err       error
}

type JournalMetricLabels struct {
	Version       string
	RolloutCohort string
	TaskType      string
	ClientVersion string
	Result        string
	ErrorCode     string
}

func (l JournalMetricLabels) prometheusLabels() map[string]string {
	return map[string]string{
		"version":        journalVersionLabel(l.Version),
		"rollout_cohort": journalEnumLabel(l.RolloutCohort, "unknown", journalRolloutCohorts),
		"task_type":      journalEnumLabel(l.TaskType, "unknown", journalTaskTypes),
		"client_version": journalVersionLabel(l.ClientVersion),
		"result":         journalMetricToken(l.Result, "unknown", 48),
		"error_code":     journalMetricToken(l.ErrorCode, "none", 64),
	}
}

var (
	journalRolloutCohorts = map[string]struct{}{
		"control": {}, "treatment": {}, "disabled": {}, "legacy": {}, "unknown": {},
	}
	journalTaskTypes = map[string]struct{}{
		"atomic": {}, "simple": {}, "general": {}, "complex": {}, "unknown": {},
	}
)

type JournalEventVisibleMetricObservation struct {
	Labels    JournalMetricLabels
	EventID   int64
	RunID     int64
	TraceID   string
	SubmitAt  time.Time
	VisibleAt time.Time
}

type JournalCompletedTaskMetricObservation struct {
	Labels                  JournalMetricLabels
	Enrolled                bool
	Mode                    DeerFlowMode
	Completed               bool
	FinalSequenceContinuous bool
	ProjectionState         domainentity.JournalProjectionState
	DegradedControl         bool
}

type JournalSSEMetricObservation struct {
	Labels JournalMetricLabels
	Signal string
}

type JournalSnapshotMetricObservation struct {
	Labels  JournalMetricLabels
	Latency time.Duration
}

type JournalRecoveryMetricObservation struct {
	Labels JournalMetricLabels
}

type JournalPrometheusMetricsCollector struct {
	eventVisibleTotal     *prometheus.CounterVec
	eventVisibleLatencyMs *prometheus.HistogramVec
	completedTasksTotal   *prometheus.CounterVec
	sseActive             *prometheus.GaugeVec
	sseSignalsTotal       *prometheus.CounterVec
	snapshotRequestsTotal *prometheus.CounterVec
	snapshotLatencyMs     *prometheus.HistogramVec
	recoveryResultsTotal  *prometheus.CounterVec
	retentionBacklog      *prometheus.GaugeVec

	dedupeMu      sync.Mutex
	visibleDedupe map[string]time.Time
	lastDedupeGC  time.Time
}

func NewJournalPrometheusMetricsCollector(
	registerer prometheus.Registerer,
) (*JournalPrometheusMetricsCollector, error) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	collector := &JournalPrometheusMetricsCollector{
		visibleDedupe: make(map[string]time.Time),
	}
	var err error
	if collector.eventVisibleTotal, err = registerCounterVec(registerer, prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "coze", Name: "journal_event_visible_total", Help: "Total uniquely rendered Journal events."},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	if collector.eventVisibleLatencyMs, err = registerHistogramVec(registerer, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Namespace: "coze", Name: "journal_event_visible_latency_ms", Help: "Journal event submit-to-render latency in milliseconds.", Buckets: []float64{5, 10, 25, 50, 100, 200, 500, 1000, 3000, 10_000}},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	if collector.completedTasksTotal, err = registerCounterVec(registerer, prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "coze", Name: "journal_completed_tasks_total", Help: "Completed enrolled Journal tasks by completeness result."},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	if collector.sseActive, err = registerGaugeVec(registerer, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "coze", Name: "journal_sse_active", Help: "Active Journal SSE leases."},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	if collector.sseSignalsTotal, err = registerCounterVec(registerer, prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "coze", Name: "journal_sse_signals_total", Help: "Journal SSE slow-consumer and backfill signals."},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	if collector.snapshotRequestsTotal, err = registerCounterVec(registerer, prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "coze", Name: "journal_snapshot_requests_total", Help: "Journal snapshot requests by result."},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	if collector.snapshotLatencyMs, err = registerHistogramVec(registerer, prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Namespace: "coze", Name: "journal_snapshot_latency_ms", Help: "Journal snapshot response latency in milliseconds.", Buckets: []float64{5, 10, 25, 50, 100, 200, 300, 500, 1000, 3000, 10_000}},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	if collector.recoveryResultsTotal, err = registerCounterVec(registerer, prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "coze", Name: "journal_recovery_results_total", Help: "Journal recovery requests by result."},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	if collector.retentionBacklog, err = registerGaugeVec(registerer, prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "coze", Name: "journal_retention_backlog", Help: "Journal retention records awaiting cleanup."},
		journalPrometheusLabelNames,
	)); err != nil {
		return nil, err
	}
	return collector, nil
}

func NewJournalPrometheusMetricsCollectorFromEnv() *JournalPrometheusMetricsCollector {
	production := strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production")
	if !envkey.GetBoolD(agentJournalPrometheusMetricsEnabledEnv, production) {
		return nil
	}
	defaultJournalPrometheusMetrics.once.Do(func() {
		defaultJournalPrometheusMetrics.collector, defaultJournalPrometheusMetrics.err =
			NewJournalPrometheusMetricsCollector(prometheus.DefaultRegisterer)
	})
	if defaultJournalPrometheusMetrics.err != nil {
		logs.CtxWarnf(
			context.Background(),
			"[journal-prometheus-metrics] collector init failed: %v",
			defaultJournalPrometheusMetrics.err,
		)
		return nil
	}
	return defaultJournalPrometheusMetrics.collector
}

func (c *JournalPrometheusMetricsCollector) RecordEventVisible(
	_ context.Context,
	observation JournalEventVisibleMetricObservation,
) {
	if c == nil || observation.EventID <= 0 || observation.RunID <= 0 ||
		observation.SubmitAt.IsZero() || observation.VisibleAt.IsZero() ||
		c.seenVisibleEvent(observation) {
		return
	}
	labels := prometheus.Labels(observation.Labels.prometheusLabels())
	c.eventVisibleTotal.With(labels).Inc()
	latency := observation.VisibleAt.Sub(observation.SubmitAt)
	if latency < 0 {
		latency = 0
	}
	c.eventVisibleLatencyMs.With(labels).Observe(float64(latency.Milliseconds()))
}

func (c *JournalPrometheusMetricsCollector) RecordCompletedTask(
	_ context.Context,
	observation JournalCompletedTaskMetricObservation,
) {
	if c == nil || !observation.Enrolled || !observation.Completed ||
		(observation.Mode != DeerFlowModePro && observation.Mode != DeerFlowModeUltra) {
		return
	}
	complete := observation.FinalSequenceContinuous &&
		observation.ProjectionState == domainentity.JournalProjectionStateHealthy &&
		!observation.DegradedControl
	if complete {
		observation.Labels.Result = "complete"
	} else {
		observation.Labels.Result = "incomplete"
	}
	c.completedTasksTotal.With(prometheus.Labels(observation.Labels.prometheusLabels())).Inc()
}

func (c *JournalPrometheusMetricsCollector) SetSSEActive(
	_ context.Context,
	labels JournalMetricLabels,
	active int,
) {
	if c == nil {
		return
	}
	if active < 0 {
		active = 0
	}
	c.sseActive.With(prometheus.Labels(labels.prometheusLabels())).Set(float64(active))
}

func (c *JournalPrometheusMetricsCollector) RecordSSESignal(
	_ context.Context,
	observation JournalSSEMetricObservation,
) {
	if c == nil || (observation.Signal != "slow_consumer" && observation.Signal != "backfill") {
		return
	}
	observation.Labels.Result = observation.Signal
	c.sseSignalsTotal.With(prometheus.Labels(observation.Labels.prometheusLabels())).Inc()
}

func (c *JournalPrometheusMetricsCollector) RecordSnapshot(
	_ context.Context,
	observation JournalSnapshotMetricObservation,
) {
	if c == nil {
		return
	}
	labels := prometheus.Labels(observation.Labels.prometheusLabels())
	c.snapshotRequestsTotal.With(labels).Inc()
	latency := observation.Latency
	if latency < 0 {
		latency = 0
	}
	c.snapshotLatencyMs.With(labels).Observe(float64(latency.Milliseconds()))
}

func (c *JournalPrometheusMetricsCollector) RecordRecovery(
	_ context.Context,
	observation JournalRecoveryMetricObservation,
) {
	if c == nil {
		return
	}
	c.recoveryResultsTotal.With(prometheus.Labels(observation.Labels.prometheusLabels())).Inc()
}

func (c *JournalPrometheusMetricsCollector) SetRetentionBacklog(
	_ context.Context,
	labels JournalMetricLabels,
	backlog int64,
) {
	if c == nil {
		return
	}
	if backlog < 0 {
		backlog = 0
	}
	c.retentionBacklog.With(prometheus.Labels(labels.prometheusLabels())).Set(float64(backlog))
}

func (c *JournalPrometheusMetricsCollector) seenVisibleEvent(
	observation JournalEventVisibleMetricObservation,
) bool {
	key := fmt.Sprintf("%d:%d:%s", observation.RunID, observation.EventID, strings.TrimSpace(observation.TraceID))
	now := observation.VisibleAt
	c.dedupeMu.Lock()
	defer c.dedupeMu.Unlock()
	if _, exists := c.visibleDedupe[key]; exists {
		return true
	}
	if c.lastDedupeGC.IsZero() || now.Sub(c.lastDedupeGC) >= time.Minute || len(c.visibleDedupe) >= journalMetricDedupeMax {
		cutoff := now.Add(-journalMetricDedupeTTL)
		for candidate, seenAt := range c.visibleDedupe {
			if seenAt.Before(cutoff) || len(c.visibleDedupe) >= journalMetricDedupeMax {
				delete(c.visibleDedupe, candidate)
			}
		}
		c.lastDedupeGC = now
	}
	c.visibleDedupe[key] = now
	return false
}

type JournalAlertSeverity string

const (
	JournalAlertSeverityWarning  JournalAlertSeverity = "warning"
	JournalAlertSeverityCritical JournalAlertSeverity = "critical"
	JournalAlertSeverityP0       JournalAlertSeverity = "p0"
)

type JournalAlert struct {
	Signal      string
	Severity    JournalAlertSeverity
	KillFeature JournalFeature
}

type JournalSLISample struct {
	CompletedEnrolledTasks       int64
	CompleteJournalTasks         int64
	SnapshotEligibleRequests     int64
	SnapshotSuccessfulRequests   int64
	SSEActive                    int64
	SSECapacity                  int64
	DuplicateSideEffects         int64
	UnauthorizedOrSensitiveLeaks int64
}

func EvaluateJournalAlerts(sample JournalSLISample) []JournalAlert {
	alerts := make([]JournalAlert, 0, 5)
	if sample.DuplicateSideEffects > 0 {
		alerts = append(alerts, JournalAlert{
			Signal: "duplicate_side_effect", Severity: JournalAlertSeverityP0,
			KillFeature: JournalFeatureRecovery,
		})
	}
	if sample.UnauthorizedOrSensitiveLeaks > 0 {
		alerts = append(alerts, JournalAlert{
			Signal: "unauthorized_or_sensitive_leak", Severity: JournalAlertSeverityP0,
			KillFeature: JournalFeatureProjection,
		})
	}
	if sample.CompletedEnrolledTasks >= 100 {
		ratio := float64(sample.CompleteJournalTasks) / float64(sample.CompletedEnrolledTasks)
		if ratio < 0.9999 {
			alerts = append(alerts, JournalAlert{Signal: "event_completeness", Severity: JournalAlertSeverityCritical})
		} else if ratio < 0.99995 {
			alerts = append(alerts, JournalAlert{Signal: "event_completeness", Severity: JournalAlertSeverityWarning})
		}
	}
	if sample.SnapshotEligibleRequests >= 100 {
		ratio := float64(sample.SnapshotSuccessfulRequests) / float64(sample.SnapshotEligibleRequests)
		if ratio < 0.999 {
			alerts = append(alerts, JournalAlert{Signal: "snapshot_success", Severity: JournalAlertSeverityCritical})
		} else if ratio < 0.9995 {
			alerts = append(alerts, JournalAlert{Signal: "snapshot_success", Severity: JournalAlertSeverityWarning})
		}
	}
	if sample.SSECapacity > 0 {
		ratio := float64(sample.SSEActive) / float64(sample.SSECapacity)
		if ratio > 0.85 {
			alerts = append(alerts, JournalAlert{Signal: "sse_capacity", Severity: JournalAlertSeverityCritical})
		} else if ratio > 0.70 {
			alerts = append(alerts, JournalAlert{Signal: "sse_capacity", Severity: JournalAlertSeverityWarning})
		}
	}
	return alerts
}

type JournalSnapshotSLIResult string

const (
	JournalSnapshotSLISuccess      JournalSnapshotSLIResult = "success"
	JournalSnapshotSLIFailed       JournalSnapshotSLIResult = "failed"
	JournalSnapshotSLINoPermission JournalSnapshotSLIResult = "no_permission"
)

func JournalSnapshotSLI(results []JournalSnapshotSLIResult) (eligible int64, successful int64) {
	for _, result := range results {
		if result == JournalSnapshotSLINoPermission {
			continue
		}
		eligible++
		if result == JournalSnapshotSLISuccess {
			successful++
		}
	}
	return eligible, successful
}

func journalEnumLabel(value, fallback string, allowed map[string]struct{}) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if _, ok := allowed[value]; ok {
		return value
	}
	return fallback
}

func journalVersionLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 24 {
		return "unknown"
	}
	for _, r := range value {
		if !unicode.IsDigit(r) && r != '.' && r != '-' && r != '+' {
			return "unknown"
		}
	}
	return value
}

func journalMetricToken(value, fallback string, maxLength int) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	if len(value) > maxLength {
		return "other"
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return "other"
		}
	}
	return value
}
