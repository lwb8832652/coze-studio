# P2-D-METRICS-012 Artifact Scan Job Metrics

## Scope

- Implement safe artifact scan job counters, terminal latency, and queue delay
  from the runtime metrics contract.
- Keep labels bounded to `scanner`, `content_family`, `result`, and
  `error_code`.
- Do not expose artifact IDs, thread IDs, run IDs, file IDs, object URI/path,
  artifact title, metadata, scanner payloads, raw errors, prompts, or content.

## Source Verification

- `ArtifactScanWorker.RunOnce` delegates processing to
  `ApplicationService.ProcessArtifactScanJobs`.
- `ProcessArtifactScanJobs` owns the per-job lifecycle outcome: succeeded,
  retried, failed, or skipped.
- `ArtifactScanJob` carries `CreatedAt`, `StartedAt`, and terminal `EndedAt`;
  this is enough to record created-to-claim queue delay and terminal scan
  latency without exposing artifact identity.
- `AgentArtifact.ContentType` is reduced to a safe content-family enum:
  `text`, `image`, `audio`, `video`, `json`, `pdf`, `binary`, or `unknown`.

## Implementation

- Added `RuntimeArtifactScanJobMetricsObservation`.
- Added Prometheus counter `coze_agent_thread_artifact_scan_jobs_total` with
  `scanner/content_family/result/error_code` labels.
- Added Prometheus histogram `coze_agent_thread_artifact_scan_latency_ms` with
  bounded `scanner/content_family/result` labels.
- Added Prometheus histogram `coze_agent_thread_artifact_scan_queue_delay_ms`
  with bounded `scanner/content_family` labels.
- Extended the internal `ProcessArtifactScanJobsResponse` with bounded
  `ArtifactScanJobMetricsSummary` values.
- `ArtifactScanWorker` records artifact scan job metrics after processing and
  before worker tick metrics.
- Success and failed terminal jobs observe latency. Retried and skipped jobs are
  counted; they keep queue delay but do not write terminal latency samples.

## Verification

```bash
cd backend
go test ./application/agentthread -run 'TestRuntimePrometheusMetricsCollectorRecordsArtifactScanJobSafely|TestArtifactScanWorkerRecordsArtifactScanJobMetrics' -count=1
go test ./application/agentthread -count=1
```

Both commands passed locally.

## Residual Work

- `P2-D-METRICS-013` added artifact scan backlog aggregation through a global
  repository/domain aggregate by scanner and status.
- `P2-D-METRICS-014`: MCP health state aggregation, memory facts/backlog, model
  call latency/failure, and web search metrics remain open.
