# P2-D-METRICS-017 MCP Health Collector

## Scope

- Implement the safe MCP health metric family from the runtime metrics
  contract:
  - `coze_agent_thread_mcp_health`
- Keep labels bounded to `transport` and `status`.
- Do not expose MCP server IDs, tool names, target URLs, headers, arguments,
  outputs, JSON-RPC payloads, credentials, raw errors, thread IDs, run IDs,
  user IDs, or space IDs.

## Source Verification

- `ADKMCPRuntimeExecutor` already reports runtime health after transport
  success or terminal transport/output failure through
  `ADKMCPRuntimeHealthReporter`.
- The executor resolves the MCP server before tool invocation, so it can attach
  the normalized transport (`stdio`, `sse`, or `streamable_http`) to the health
  report without exposing server details.
- Previous MCP invocation metrics use terminal audit records for counts,
  latency, and output bytes; health state needs a current-status gauge, not
  another terminal counter.

## Implementation

- Added `RuntimeMCPHealthMetricsObservation` and `RecordRuntimeMCPHealth` to
  the runtime metrics collector contract.
- Added disabled-by-default Prometheus gauge
  `coze_agent_thread_mcp_health` with bounded `transport/status` labels.
- The collector keeps an internal `serverID -> transport/status` map only to
  compute current health counts. Server IDs are not emitted as labels or metric
  values.
- Health status is normalized to `healthy` or `unhealthy`; invalid labels fall
  back to `unhealthy`/`unknown`.
- `ADKMCPRuntimeHealthReport` now carries the resolved transport from the MCP
  executor.
- `newADKMCPRuntimeHealthReporterFromEnv` fans out existing health reporters to
  the runtime Prometheus collector when
  `AGENT_THREAD_RUNTIME_PROMETHEUS_METRICS_ENABLED` is enabled.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestADKMCPRuntimeHealthReporterWithMetricsFansOutSafely|TestRuntimePrometheusMetricsCollectorRecordsMCPHealthSafely|TestRuntimePrometheusMetricsCollectorSnapshotsMCPHealthByCurrentState|TestADKMCPRuntimeExecutorReportsRuntimeHealthAfterTransport|TestADKMCPRuntimeExecutorReportsUnhealthyAfterTransportFailure' -count=1
go test ./application/agentthread -count=1
```

Both commands passed locally after the implementation.

## Residual Work

- `P2-D-METRICS-018` was completed in the following slice.
