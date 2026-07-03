# P2-D-OTEL-003 Observability Exporter Decision

## Scope

Decide whether to add OpenTelemetry directly in the current P2-D slice or keep
DeerFlow-style tracing metadata, run events, runtime doctor, logs, and
metadata-only APIs until a safe exporter policy exists.

## Evidence Checked

DeerFlow:

- `backend/packages/harness/deerflow/config/tracing_config.py`
  - Supports LangSmith and Langfuse flags and validates required credentials.
- `backend/packages/harness/deerflow/tracing/factory.py`
  - Builds callbacks for enabled tracing providers.
- `backend/packages/harness/deerflow/tracing/metadata.py`
  - Injects Langfuse session/user/name/tag metadata.

Coze:

- Exact search for `go.opentelemetry`, `opentelemetry`, `otel`, `promhttp`,
  `prometheus.DefaultRegisterer`, and `client_golang/prometheus` found only the
  existing Guardrail Prometheus collector in inspected task-runtime paths.
- Existing safe surfaces remain runtime doctor, task events, MCP audit,
  Guardrail audit, artifact scan jobs, memory jobs, token usage, and logs.

## Decision Recorded

Added:

- `docs/superpowers/specs/2026-07-02-deerflow-parity-observability-exporter-decision.md`

Decision:

- Do not add generic OpenTelemetry in this P2-D slice.
- Implement safe generic metrics collectors first in a later code slice, behind
  disabled-by-default flags.
- Re-evaluate OTel only after safe attribute, sampling, retention,
  tenant-isolation, sink ownership, rollback, and smoke-test policy are
  reviewed.

## Verification

Passed:

```bash
git diff --check
git diff --no-index --check /dev/null docs/superpowers/specs/2026-07-02-deerflow-parity-observability-exporter-decision.md
git diff --no-index --check /dev/null docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P2-D-OTEL-003-observability-exporter-decision.md
```

## Residual Work

- Implement generic runtime collectors behind disabled-by-default flags.
- Expose or document the approved scrape/log sink path after operator review.
