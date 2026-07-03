# P2-D-OPS-001 Runtime Operations Runbook

## Scope

Create the first P2-D operations slice for DeerFlow parity runtime hardening:
a unified runbook for API/runtime worker observability, safe signals, forbidden
signals, smoke verification, and flag-based rollback.

This is a documentation and evidence slice only. It does not claim full P2-D
completion because generic OpenTelemetry export and non-Guardrail Prometheus
metric families are still open.

## DeerFlow Reference Checked

- `backend/packages/harness/deerflow/tracing/factory.py`
  - Builds tracing callbacks for explicitly enabled LangSmith/Langfuse
    providers.
- `backend/packages/harness/deerflow/tracing/metadata.py`
  - Adds Langfuse session/user/name/tags into runnable metadata.
- `backend/packages/harness/deerflow/runtime/events/store/db.py`
  - Persists message/trace/lifecycle run events and truncates trace content by
    byte budget.
- `backend/packages/harness/deerflow/config/run_events_config.py`
  - Defines run-event storage backends and token tracking.
- `backend/packages/harness/deerflow/subagents/executor.py`
  - Uses trace IDs in subagent logs and task execution metadata.
- `backend/packages/harness/deerflow/tools/builtins/task_tool.py`
  - Uses trace IDs for background subagent task logs, polling, timeout, and
    cleanup visibility.

Observation: the checked DeerFlow tree does not provide a single OTel runbook.
Its operational model combines tracing callbacks, run-event persistence,
checkpoint/store configuration, health checks, and trace-id logs.

## Coze Current Surfaces Checked

- Runtime doctor API:
  - `backend/api/handler/coze/workbench_runtime_doctor_service.go`
  - `backend/api/model/workbench/diagnostic/diagnostic.go`
- Runtime policy and worker flags:
  - `backend/application/agentthread/runtime_selector.go`
  - `backend/application/agentthread/worker.go`
- Token trace metadata:
  - `backend/application/agentthread/adk_usage.go`
- Artifact scanner flags:
  - `backend/application/agentthread/artifact_scanner.go`
- Web search flags:
  - `backend/application/agentthread/adk_web_search_backend.go`
- MCP runtime bootstrap and transport flags:
  - `backend/application/agentthread/adk_mcp_runtime_bootstrap.go`
- Guardrail runbook:
  - `docs/superpowers/runbooks/guardrail-audit-operations.md`
- Local debug runbook:
  - `docs/superpowers/runbooks/local-debug-and-test.md`

## Implemented Documentation

- Added `docs/superpowers/runbooks/deerflow-parity-runtime-operations.md`.
- The runbook covers:
  - operator surfaces;
  - enablement order;
  - runtime, worker, memory, artifact, web-search, MCP, and Guardrail flags;
  - safe signals;
  - forbidden signals;
  - backend/frontend smoke verification;
  - flag-first rollback order;
  - incident triage paths;
  - explicit open gaps for generic OTel and non-Guardrail metric families.

## Verification

Passed:

```bash
git diff --check
```

Targeted tests are not required for this documentation-only slice, but the
runbook records the smoke commands that should be run before enabling runtime
paths or changing visible diagnostics.

## Residual Work

- `P2-D-METRICS-002`: define safe generic task/runtime metric families and
  alert thresholds.
- `P2-D-OTEL-003`: decide whether Coze should add OpenTelemetry directly or
  continue with existing metadata/runtime doctor surfaces plus logs.
