# Guardrail Prometheus Exporter M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Prometheus collector adapter for Guardrail decision, archive, and retention metrics without exposing sensitive audit content or object locations.

**Architecture:** Implement one `GuardrailPrometheusMetricsCollector` that satisfies the existing decision, archive, and retention metrics collector interfaces. Existing env factory functions can return a safe mux of logging and Prometheus collectors when enabled, while HTTP scrape endpoint wiring remains a separate platform concern.

**Tech Stack:** Go, `github.com/prometheus/client_golang/prometheus`, existing `agentthread` metrics observation structs.

---

### Task 1: Prometheus Collector Tests

**Files:**
- Create: `backend/application/agentthread/guardrail_prometheus_metrics_test.go`

- [x] **Step 1: Write decision metrics test**

```go
registry := prometheus.NewRegistry()
collector, err := NewGuardrailPrometheusMetricsCollector(registry)
require.NoError(t, err)
collector.RecordGuardrailEvaluation(context.Background(), GuardrailEvaluationMetricsObservation{
	TargetType: "tool_call",
	Operation: "invoke",
	Source: "adk_tool_wrapper",
	FailMode: "fail_closed",
	Action: "deny",
	Provider: "scanner /mnt/raw",
	ErrorCode: "guardrail denied",
	ElapsedMs: 17,
})
families, err := registry.Gather()
require.NoError(t, err)
require.Contains(t, metricFamiliesText(families), "coze_guardrail_evaluations_total")
require.NotContains(t, metricFamiliesText(families), "runtime_tool:search_docs")
```

- [x] **Step 2: Write archive/retention safety test**

Record archive and retention observations with archive IDs and sensitive-looking values. Assert gathered metric text contains aggregate counters/histograms but does not contain archive IDs, event IDs, object URIs, raw URLs, credentials, prompts, checkpoint bytes, scanner/provider raw payloads, or raw metadata.

### Task 2: Collector Implementation

**Files:**
- Create: `backend/application/agentthread/guardrail_prometheus_metrics.go`
- Modify: `backend/application/agentthread/guardrail_metrics.go`
- Modify: `backend/application/agentthread/guardrail_audit_archive_metrics.go`
- Modify: `backend/application/agentthread/guardrail_audit_retention_metrics.go`

- [x] **Step 1: Add Prometheus collector**

Define counters and histograms for:
- `coze_guardrail_evaluations_total`
- `coze_guardrail_evaluation_latency_ms`
- `coze_guardrail_audit_archive_attempts_total`
- `coze_guardrail_audit_archive_rows_total`
- `coze_guardrail_audit_archive_latency_ms`
- `coze_guardrail_audit_retention_attempts_total`
- `coze_guardrail_audit_retention_rows_total`
- `coze_guardrail_audit_retention_latency_ms`

- [x] **Step 2: Keep labels low-cardinality and metadata-only**

Use bounded labels such as action, provider, error code, success, skipped, skip reason, and row kind. Do not label by archive ID, event ID, thread ID, run ID, target ID, object URI, URL, filename, prompt text, tool arguments, raw metadata, or provider body.

- [x] **Step 3: Add env mux wiring**

Add `AGENT_GUARDRAIL_PROMETHEUS_METRICS_ENABLED`. Existing `NewGuardrail*MetricsCollectorFromEnv` functions should return nil, one collector, or a mux of logging + Prometheus collectors depending on env flags.

### Task 3: Documentation And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/runbooks/guardrail-audit-operations.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Document M8.24**

Record the Prometheus collector boundary, safe labels, env flag, and the fact that HTTP scrape endpoint wiring remains platform infra work unless a safe endpoint already exists.

- [ ] **Step 2: Run verification**

```bash
cd backend
go test -count=1 ./application/agentthread -run 'TestGuardrailPrometheus|TestGuardrailMetricsCollectorFromEnv|TestGuardrailAuditArchiveMetricsCollectorFromEnv|TestGuardrailAuditRetentionMetricsCollectorFromEnv'
go test -count=1 ./application/agentthread
go test -count=1 ./application
cd ..
git diff --check
```
