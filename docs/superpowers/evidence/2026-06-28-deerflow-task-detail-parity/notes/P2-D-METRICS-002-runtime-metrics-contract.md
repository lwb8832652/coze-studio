# P2-D-METRICS-002 Runtime Metrics Contract

## Scope

Define the safe metrics contract for DeerFlow-parity runtime observability
before implementing generic Prometheus or OpenTelemetry exporters.

This slice does not add metrics code. It prevents future implementation from
leaking prompts, model text, tool payloads, object locations, checkpoint bytes,
credentials, or high-cardinality IDs into labels.

## DeerFlow Reference Checked

Same reference set as `P2-D-OPS-001`:

- tracing callbacks for LangSmith/Langfuse;
- Langfuse trace metadata;
- run-event persistence and trace truncation;
- run-event config;
- subagent/background task trace-id logs.

The checked DeerFlow tree favors tracing callbacks plus run-event persistence,
not a broad built-in Prometheus/OTel contract. Coze therefore needs an explicit
safe-label contract before adding generic runtime metrics.

## Implemented Documentation

Added:

- `docs/superpowers/specs/2026-07-02-deerflow-parity-runtime-metrics-contract.md`

The contract defines:

- global metric rules;
- allowed common labels;
- conditionally allowed labels;
- forbidden labels and values;
- task runtime, worker, model/token, web search, MCP, artifact, memory, and
  Guardrail metric families;
- alert candidates;
- rollout order;
- implementation notes for future collector work.

## Verification

Passed:

```bash
git diff --check
git diff --no-index --check /dev/null docs/superpowers/specs/2026-07-02-deerflow-parity-runtime-metrics-contract.md
git diff --no-index --check /dev/null docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P2-D-METRICS-002-runtime-metrics-contract.md
```

## Residual Work

- Implement generic task-runtime collectors behind disabled-by-default flags.
- Decide whether OpenTelemetry should be added directly or kept as a later
  exporter after Prometheus/log metrics prove safe.
