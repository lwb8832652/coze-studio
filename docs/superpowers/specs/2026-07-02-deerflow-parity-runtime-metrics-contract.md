# DeerFlow Parity Runtime Metrics Contract

## Goal

Define safe, low-cardinality metrics for the Coze DeerFlow-parity task
runtime before adding generic Prometheus or OpenTelemetry exporters.

This contract complements `docs/superpowers/runbooks/deerflow-parity-runtime-operations.md`.
It is not an implementation plan by itself. Any exporter implementation must
first pass this label/content review.

## Reference Shape

DeerFlow keeps operational visibility through:

- tracing callbacks for LangSmith and Langfuse;
- run events split into message, trace, and lifecycle categories;
- run-event trace truncation budgets;
- checkpoint/store health assumptions;
- trace IDs on subagent and background task logs.

Coze should mirror the same principle: task runtime metrics are aggregate
signals, while detailed inspection remains in bounded task events, runtime
doctor, MCP audit, Guardrail audit, token usage, and artifact/memory job APIs.

## Global Rules

- Metrics must be aggregate, bounded, and metadata-only.
- Labels must be low-cardinality and reviewed before rollout.
- Use counters for attempts, completions, and failures.
- Use histograms for latency, queue delay, output bytes, token totals, and
  backlog age.
- Use gauges only for current backlog or enabled/health counts.
- Do not emit user-controlled text as labels.
- Do not emit IDs as labels unless explicitly listed as safe. The default is
  no IDs in labels.
- Metric names use the `coze_agent_thread_` prefix except existing Guardrail
  metrics, which keep their current `coze_guardrail_` prefix.

## Allowed Labels

Common labels:

- `runtime`: `legacy`, `eino_adk`, `unknown`.
- `result`: `success`, `failed`, `canceled`, `interrupted`, `skipped`.
- `status`: bounded run/job status enum.
- `worker_type`: `run`, `resume`, `memory_flush`, `artifact_scan`,
  `mcp_workdir_reaper`, `guardrail_archive`, `guardrail_retention`.
- `source`: bounded source enum, such as `task`, `resume`, `followup`,
  `system`.
- `error_code`: sanitized enum, or `none`.
- `provider`: bounded provider enum where applicable.
- `transport`: `stdio`, `sse`, `streamable_http`, `http`, `unknown`.
- `content_family`: `text`, `image`, `pdf`, `archive`, `binary`, `unknown`.
- `scanner`: `http`, `clamd`, `clamav`, `none`, `unknown`.

Conditionally allowed labels:

- `model_family`: normalized model family, not raw provider request IDs or
  arbitrary model display names.
- `tool_group`: bounded tool category such as `mcp`, `web`, `skill`,
  `subagent`, `artifact`.

## Forbidden Labels And Values

Never emit these as labels or metric values:

- prompt, message text, model completion text, chain-of-thought, hidden system
  prompt, raw provider body;
- web search query, HTTP URL, redirect URL, host, page title, snippet, raw
  response body;
- tool arguments, tool results, MCP JSON-RPC body, stdio stdout/stderr,
  remote headers;
- artifact object URI, object key, filename, filesystem path, file content,
  raw scanner payload;
- checkpoint bytes, hidden run config, raw audit payload, SQL, memory fact
  text;
- credential, bearer token, auth env key/value, API key, cookie, session ID;
- user ID, space ID, thread ID, run ID, artifact ID, event ID, server ID, or
  other high-cardinality IDs.

## Metric Families

### Task Runtime

- `coze_agent_thread_runs_total`
  - Type: counter.
  - Labels: `runtime`, `source`, `result`, `error_code`.
  - Increment when a run reaches terminal or interrupted status.
- `coze_agent_thread_run_latency_ms`
  - Type: histogram.
  - Labels: `runtime`, `source`, `result`.
  - Observe wall-clock runtime from claim/start to terminal/interrupted status.
- `coze_agent_thread_run_queue_delay_ms`
  - Type: histogram.
  - Labels: `runtime`, `source`.
  - Observe created-to-claimed delay.
- `coze_agent_thread_runs_backlog`
  - Type: gauge.
  - Labels: `runtime`, `status`.
  - Current pending/queued/running/interrupted backlog.

### Workers

- `coze_agent_thread_worker_ticks_total`
  - Type: counter.
  - Labels: `worker_type`, `result`, `error_code`.
  - Increment per worker tick.
- `coze_agent_thread_worker_claimed_total`
  - Type: counter.
  - Labels: `worker_type`, `result`.
  - Increment by claimed rows/jobs per tick.
- `coze_agent_thread_worker_tick_latency_ms`
  - Type: histogram.
  - Labels: `worker_type`, `result`.
  - Observe full tick duration.

### Model And Token Usage

- `coze_agent_thread_model_calls_total`
  - Type: counter.
  - Labels: `runtime`, `model_family`, `result`, `error_code`.
  - Increment once per provider call.
- `coze_agent_thread_model_latency_ms`
  - Type: histogram.
  - Labels: `runtime`, `model_family`, `result`.
  - Observe provider call duration.
- `coze_agent_thread_tokens_total`
  - Type: counter.
  - Labels: `runtime`, `model_family`, `source`, `direction`.
  - `direction`: `input`, `output`, `cached`, `reasoning`.
  - Values come from persisted token usage aggregation only.

### Web Search

- `coze_agent_thread_web_search_calls_total`
  - Type: counter.
  - Labels: `provider`, `result`, `error_code`.
  - Do not label with query, host, URL, or result title.
- `coze_agent_thread_web_search_latency_ms`
  - Type: histogram.
  - Labels: `provider`, `result`.
- `coze_agent_thread_web_search_results`
  - Type: histogram.
  - Labels: `provider`, `result`.

### MCP Runtime

- `coze_agent_thread_mcp_invocations_total`
  - Type: counter.
  - Labels: `transport`, `result`, `error_code`.
  - Do not label with server ID, tool name, request body, or target URL.
- `coze_agent_thread_mcp_latency_ms`
  - Type: histogram.
  - Labels: `transport`, `result`.
- `coze_agent_thread_mcp_output_bytes`
  - Type: histogram.
  - Labels: `transport`, `result`.
- `coze_agent_thread_mcp_health`
  - Type: gauge.
  - Labels: `transport`, `status`.
  - Value is server count by status bucket.

### Artifacts

- `coze_agent_thread_artifact_scan_jobs_total`
  - Type: counter.
  - Labels: `scanner`, `content_family`, `result`, `error_code`.
- `coze_agent_thread_artifact_scan_latency_ms`
  - Type: histogram.
  - Labels: `scanner`, `content_family`, `result`.
- `coze_agent_thread_artifact_scan_queue_delay_ms`
  - Type: histogram.
  - Labels: `scanner`, `content_family`.
- `coze_agent_thread_artifact_scan_backlog`
  - Type: gauge.
  - Labels: `scanner`, `status`.

### Memory

- `coze_agent_thread_memory_flush_jobs_total`
  - Type: counter.
  - Labels: `result`, `error_code`.
- `coze_agent_thread_memory_flush_latency_ms`
  - Type: histogram.
  - Labels: `result`.
- `coze_agent_thread_memory_facts_total`
  - Type: counter.
  - Labels: `operation`, `result`.
  - `operation`: `upsert`, `remove`, `skip`.
  - Do not emit fact text, source text, or scopes as labels.
- `coze_agent_thread_memory_flush_backlog`
  - Type: gauge.
  - Labels: `status`.

### Guardrail

Guardrail metric families already exist and keep their current prefix:

- `coze_guardrail_evaluations_total`
- `coze_guardrail_evaluation_latency_ms`
- `coze_guardrail_audit_archive_attempts_total`
- `coze_guardrail_audit_archive_rows_total`
- `coze_guardrail_audit_archive_latency_ms`
- `coze_guardrail_audit_retention_attempts_total`
- `coze_guardrail_audit_retention_rows_total`
- `coze_guardrail_audit_retention_latency_ms`

Any Guardrail label changes must be reviewed in
`docs/superpowers/runbooks/guardrail-audit-operations.md`.

## Alert Candidates

Alerts must be tuned per environment after baseline collection:

- Run queue p95 delay above 60 seconds for 10 minutes.
- Run failure ratio above 10 percent for 10 minutes.
- Resume failure ratio above 5 percent for 10 minutes.
- Worker tick failure repeated for 5 consecutive intervals.
- MCP invocation failure ratio above 20 percent by transport for 10 minutes.
- MCP output budget exceeded spike above baseline.
- Artifact scan backlog older than 15 minutes.
- Artifact scan final failure ratio above 5 percent.
- Memory flush backlog older than 30 minutes.
- Memory extractor failure repeated for 5 consecutive intervals.
- Web search timeout/failure ratio above 30 percent for 10 minutes.
- Token output/input ratio or total tokens above reviewed budget.
- Guardrail deny or provider-error spikes above reviewed baseline.

## Rollout Order

1. Add unit tests for label sets before exporter implementation.
2. Add collectors behind disabled-by-default env flags.
3. Enable in non-production with synthetic traffic.
4. Compare logs, runtime doctor, audit APIs, and metric counters for the same
   smoke runs.
5. Confirm forbidden labels are absent from scrape output.
6. Enable dashboard panels with no alerting.
7. Enable alerts after baseline review.
8. Document rollback switches in the runtime operations runbook.

## Implementation Status

- `P2-D-METRICS-004` implemented the Workers metric family for worker ticks,
  claimed rows/jobs, and tick latency.
- `P2-D-METRICS-005` implemented terminal run counters and latency histograms
  for run and resume processors.
- `P2-D-METRICS-006` implemented input/output token usage counters based on
  persisted token usage rows.
- `P2-D-METRICS-007` implemented MCP invocation count, latency, and output
  byte metrics through the metadata-only MCP runtime audit recorder.
- `P2-D-METRICS-008` implemented exact MCP transport labels for terminal
  audit-backed invocation metrics and filters non-terminal lifecycle events
  such as `mcp.tool.started`.
- `P2-D-METRICS-009` implemented run queue delay histograms from claimed run
  metadata (`StartedAt - CreatedAt`).
- `P2-D-METRICS-010` implemented run backlog status aggregation through a real
  domain/repository aggregate and Prometheus snapshot gauge
  `coze_agent_thread_runs_backlog`.
- `P2-D-METRICS-011` implemented memory flush job counters and terminal latency
  histograms from bounded per-job worker summaries.
- `P2-D-METRICS-012` implemented artifact scan job counters, terminal latency,
  and queue delay histograms from bounded per-job worker summaries.
- `P2-D-METRICS-013` implemented artifact scan backlog status aggregation
  through a real repository/domain aggregate and Prometheus snapshot gauge
  `coze_agent_thread_artifact_scan_backlog`.
- `P2-D-METRICS-014` implemented memory flush backlog status aggregation
  through a real repository/domain aggregate and Prometheus snapshot gauge
  `coze_agent_thread_memory_flush_backlog`.
- `P2-D-METRICS-015` implemented memory fact counters from metadata-only flush
  summaries and Prometheus counter `coze_agent_thread_memory_facts_total`.
- `P2-D-METRICS-016` implemented web search call counters, latency histograms,
  and result-count histograms through a metrics-enabled ADK WebSearchBackend
  wrapper.
- `P2-D-METRICS-017` implemented MCP health state aggregation through
  `coze_agent_thread_mcp_health`, using bounded `transport/status` labels and
  internal-only server ID state for current-count snapshots.
- `P2-D-METRICS-018` implemented model call counters and latency histograms for
  ADK chat model `Generate` and `Stream` calls through a metrics-enabled model
  wrapper.
- The collector is disabled by default through
  `AGENT_THREAD_RUNTIME_PROMETHEUS_METRICS_ENABLED`.
- It currently instruments run, resume, memory flush, and artifact scan worker
  ticks, run/resume processor terminal statuses, and persisted input/output
  token usage, model call count/latency, run queue delay, run backlog
  snapshots, memory flush job count/latency, memory facts counters, memory
  flush backlog snapshots, artifact scan job count/latency/queue delay,
  artifact scan backlog snapshots, web search calls/latency/result counts, MCP
  health snapshots, plus MCP audit-backed invocations with bounded transport
  labels.

## Implementation Notes

- Prefer one collector package for task-runtime metrics with narrow interfaces
  injected into worker/processors.
- Do not make hot-path model/tool execution depend on a remote metrics sink.
- Collector failures must never fail a user task.
- Prometheus collection should deduplicate already-registered collectors the
  way Guardrail metrics do.
- OTel export, if added, must use the same safe labels and no raw span
  attributes containing user/task content.
