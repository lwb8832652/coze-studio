# P2-D-METRICS-008 MCP Transport Split

## Scope

- Preserve the existing metadata-only MCP runtime audit contract while adding
  bounded transport metadata for metrics.
- Keep MCP invocation metrics content-free: no server ID, tool name, arguments,
  results, URLs, headers, credentials, or raw provider payloads.
- Avoid fake health gauges. Current runtime has only per-invocation health
  callbacks, not a queryable current-status aggregate.

## Source Verification

- `ADKMCPRuntimeExecutor` resolves `toolapi.MCPToolServer` before invoking a
  transport.
- `ADKMCPRuntimeTransportRouter` derives the canonical transport with
  `normalizeADKMCPRuntimeTransportType` from `MCPToolServer.ServerType`.
- Existing `ADKMCPRuntimeHealthReporter` receives one callback per transport
  attempt; no state store or server-count aggregation exists in the inspected
  application/domain code.

## Implementation

- Added `ADKMCPRuntimeAuditRecord.Transport`.
- Terminal audit records now receive the canonical transport after server
  resolution. Pre-resolution `mcp.tool.started` records may remain empty and
  are intentionally not used for invocation metrics.
- `recordRuntimeMCPInvocation` now ignores non-terminal lifecycle events and
  records only terminal `completed`/`failed` style events.
- Prometheus transport labels are bounded to `stdio`, `sse`,
  `streamable_http`, or `unknown`.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestApplicationADKMCPRuntimeAuditRecorder(RecordsRuntimeMetrics|SkipsNonTerminalRuntimeMetrics)|TestADKMCPRuntimeExecutorRecordsContentFreeAuditLifecycle' -count=1
go test ./application/agentthread -count=1
```

Both commands passed locally.

## Residual Work

- `P2-D-METRICS-010`: implement a real MCP health state aggregation surface
  before adding `coze_agent_thread_mcp_health`.
- `P2-D-METRICS-010`: continue run backlog status aggregation, model
  latency/failure, web search, artifact-domain, and memory-domain collectors.
