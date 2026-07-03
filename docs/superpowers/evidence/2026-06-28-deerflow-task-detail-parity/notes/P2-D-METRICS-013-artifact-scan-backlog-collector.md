# P2-D-METRICS-013 Artifact Scan Backlog Collector

## Scope

- Implement the safe `coze_agent_thread_artifact_scan_backlog` gauge from the
  runtime metrics contract.
- Use a real global artifact scan job aggregation, not current worker claimed
  counts.
- Keep labels bounded to `scanner` and `status`; do not expose artifact IDs,
  thread IDs, run IDs, file IDs, object URI/path, title, metadata, scanner
  payloads, raw errors, prompts, or content.

## Source Verification

- `ArtifactScanJob` active backlog states are `pending` and `processing`.
- Terminal statuses (`succeeded`, `failed`) are not backlog.
- `ArtifactScanWorker.RunOnce` already owns the runtime tick cadence, so it can
  record a safe global snapshot after each processing attempt.
- Repository aggregation is required because per-tick claimed jobs cannot
  represent total backlog across scanners and workers.

## Implementation

- Added `entity.ArtifactScanBacklogAggregate`.
- Added repository/domain service `AggregateArtifactScanBacklog` contracts.
- MySQL repository groups `agent_artifact_scan_jobs` by `scanner, status` for
  active statuses.
- Domain service normalizes the default active-status list to `pending` and
  `processing`.
- Runtime metrics collector now exposes disabled-by-default Prometheus gauge
  `coze_agent_thread_artifact_scan_backlog` with bounded `scanner/status`
  labels.
- Gauge recording resets the collector snapshot before writing current values,
  preventing stale labels after backlog state changes.
- `ArtifactScanWorker` records the backlog snapshot after per-job metrics and
  before worker tick metrics. Metrics are fail-open and skipped when the
  collector is disabled.

## Verification

```bash
cd backend
go test ./domain/agentthread/repository ./application/agentthread -run 'TestArtifactRepositoryAggregateArtifactScanBacklogGroupsActiveStatusesByScanner|TestRuntimePrometheusMetricsCollectorRecordsArtifactScanBacklogSafely|TestArtifactScanWorkerRecordsRuntimeArtifactScanBacklogMetrics' -count=1
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread -count=1
```

Both commands passed locally.

## Residual Work

- `P2-D-METRICS-014` added memory flush backlog aggregation.
- `P2-D-METRICS-015`: MCP health state aggregation, memory facts counters, model
  call latency/failure, and web search metrics remain open.
