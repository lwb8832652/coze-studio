# P2-D-METRICS-007 MCP Invocation Collector

## Scope

Implement MCP invocation metrics for the DeerFlow-parity runtime using the
existing metadata-only MCP runtime audit path.

This slice does not expose MCP tool arguments, tool results, JSON-RPC payloads,
stdio output, remote headers, server IDs, run IDs, thread IDs, URLs, or raw
errors as metric labels.

## Implemented

- Extended `RuntimeMetricsCollector` with MCP invocation observations.
- Added Prometheus metrics:
  - `coze_agent_thread_mcp_invocations_total`
  - `coze_agent_thread_mcp_latency_ms`
  - `coze_agent_thread_mcp_output_bytes`
- Recorded metrics after `ApplicationADKMCPRuntimeAuditRecorder` successfully
  persists the metadata-only audit event.
- Derived `result` from bounded audit event type:
  - completed/success event -> `success`
  - failed/error event -> `failed`
- Empty error code is normalized to `none`.

## Safety Boundary

MCP invocation metrics emit only bounded metadata labels:

- `transport`
- `result`
- `error_code`

`transport` is currently emitted as `unknown`, because the existing audit record
does not carry stdio/sse/http transport type. Exact transport split is deferred
to a follow-up task so the executor contract can be extended deliberately.

Invalid or path/secret-like label values are normalized to `unknown`.

## Verification

Passed:

```bash
cd backend
go test ./application/agentthread -run 'TestApplicationADKMCPRuntimeAuditRecorderRecordsRuntimeMetrics|TestRuntimePrometheusMetricsCollectorRecordsMCPInvocationSafely' -count=1
go test ./application/agentthread -run 'TestApplicationADKMCPRuntimeAuditRecorder|TestThreadUsageCollector|TestADKUsage|TestRuntimePrometheusMetricsCollector|TestRunProcessor|TestResumeRunProcessor|Test.*Worker.*' -count=1
go test ./application/agentthread -count=1
```

## Residual Work

- `P2-D-METRICS-008`: exact MCP transport split is tracked separately.
- `P2-D-METRICS-009`: MCP health state aggregation, run backlog/queue delay,
  web search, artifact-domain, and memory-domain collectors.
