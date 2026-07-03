# DeerFlow Parity Observability Exporter Decision

## Decision

Do not add a generic OpenTelemetry exporter in the current P2-D slice.

Instead:

1. Keep DeerFlow-style task visibility grounded in run events, runtime doctor,
   token usage, MCP audit, artifact scan jobs, memory jobs, Guardrail audit,
   and bounded logs.
2. Use the runtime metrics contract before adding any generic Prometheus
   collectors.
3. Add OpenTelemetry only after a reviewed exporter policy exists for
   attributes, sampling, retention, tenant isolation, and sink ownership.

## Why

The checked DeerFlow reference implements observability with LangSmith and
Langfuse callbacks, run-event persistence, trace metadata, and trace-id logs.
It does not expose a checked-in generic OpenTelemetry runbook or exporter.

The checked Coze tree currently has:

- Guardrail Prometheus collectors under
  `backend/application/agentthread/guardrail_prometheus_metrics.go`;
- no `go.opentelemetry` package usage in the inspected task-runtime paths;
- no inspected `promhttp` endpoint for a generic metrics scrape surface;
- existing runtime doctor and metadata-only audit APIs for operational triage.

Adding OTel directly now would introduce a new dependency and a new external
data path before label/attribute, retention, and tenant-isolation review. That
would be riskier than DeerFlow parity requires for this cut.

## Required Before Any OTel Implementation

- A reviewed safe-attribute contract matching
  `docs/superpowers/specs/2026-07-02-deerflow-parity-runtime-metrics-contract.md`.
- Exporter sink ownership and retention policy.
- Sampling policy for high-volume model/tool spans.
- Explicit ban on prompt text, model output, tool arguments/results,
  checkpoint bytes, artifact object locations, credentials, and raw provider
  bodies as span attributes.
- Tenant isolation review for trace/session identifiers.
- Rollback flags and smoke tests in the runtime operations runbook.

## Recommended Next Implementation Order

1. Implement generic Prometheus/log collectors behind disabled-by-default flags
   using the safe metrics contract.
2. Expose or document the approved scrape/log sink path.
3. Add dashboards without alerting.
4. Baseline and tune alerts.
5. Re-evaluate OTel with the same safe labels as span attributes.

## Non-Goals

- Do not add LangSmith or Langfuse credentials/settings to tracked files.
- Do not expose raw task events through metrics.
- Do not add a new public observability API without a separate API contract.
- Do not make task execution depend on exporter availability.
