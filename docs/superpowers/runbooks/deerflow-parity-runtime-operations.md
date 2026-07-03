# DeerFlow Parity Runtime Operations Runbook

## Purpose

Operate the DeerFlow-parity task runtime in Coze Studio with safe
observability, rollback switches, and bounded incident triage. This runbook
covers the production control plane for task API, run workers, resume workers,
model calls, web tools, MCP tools, artifacts, memory, token usage, and
Guardrail audit paths.

The runbook is intentionally metadata-only. It must not require operators to
inspect prompts, model completions, tool arguments, tool results, artifact
object locations, checkpoint bytes, credentials, or raw provider payloads.

## DeerFlow Reference

- DeerFlow does not expose a single OpenTelemetry runbook in the checked
  reference tree. Runtime observability is split across LangSmith/Langfuse
  tracing callbacks, run-event persistence, checkpoint/store configuration,
  health checks, and trace-id log metadata.
- Relevant reference files:
  - `backend/packages/harness/deerflow/tracing/factory.py`
  - `backend/packages/harness/deerflow/tracing/metadata.py`
  - `backend/packages/harness/deerflow/runtime/events/store/db.py`
  - `backend/packages/harness/deerflow/config/run_events_config.py`
  - `backend/packages/harness/deerflow/subagents/executor.py`
  - `backend/packages/harness/deerflow/tools/builtins/task_tool.py`
- Coze parity should therefore prioritize safe task-level metadata, run-event
  status, runtime doctor checks, MCP health/audit rows, token aggregates,
  Guardrail metrics, and worker logs before adding a broad OTel exporter.

## Operator Surfaces

- Runtime doctor:
  - `GET /api/workbench/runtime_doctor?space_id=<space_id>`
  - Safe summary only: runtime mode, model status, sandbox status, web tools,
    MCP totals, and check rows.
- Task detail inspector:
  - `详情`: metadata-only task/run state.
  - `工具调用`: MCP runtime audit rows.
  - `运行诊断`: runtime doctor summary.
  - `安全审计`: Guardrail audit rows and JSON export.
- Guardrail operations:
  - See `docs/superpowers/runbooks/guardrail-audit-operations.md`.
- Local debug:
  - See `docs/superpowers/runbooks/local-debug-and-test.md`.

## Enablement Order

1. Enable the Go-native runtime policy in a non-production environment.
2. Enable the run worker and confirm new tasks move from queued to running and
   then to terminal status.
3. Enable resume worker only after interruption/resume smoke tests pass.
4. Enable memory flush and memory extractor together.
5. Enable artifact scan worker with a scanner configured.
6. Enable web search only after provider/network policy has been validated.
7. Enable MCP runtime only with a strict transport policy:
   - stdio: command allow-list, env allow-list, leased workdir.
   - remote: allowed host list, HTTPS by default, bounded headers/config.
8. Enable Guardrail provider and audit export.
9. Enable Guardrail Prometheus or metrics logs only after label/content review.
10. Add broad OTel/exporter work only after safe-signal review and dashboard
    owners are defined.

## Environment Flags

### Runtime And Workers

- `AGENT_THREAD_RUNTIME_DEFAULT=legacy|eino_adk`
- `AGENT_THREAD_EINO_ADK_ENABLED=true|false`
- `AGENT_THREAD_WORKER_ENABLED=true|false`
- `AGENT_THREAD_WORKER_ID`
- `AGENT_THREAD_WORKER_BATCH_SIZE`
- `AGENT_THREAD_WORKER_INTERVAL_MS`
- `AGENT_THREAD_RESUME_WORKER_ENABLED=true|false`
- `AGENT_THREAD_RESUME_WORKER_ID`
- `AGENT_THREAD_RESUME_WORKER_BATCH_SIZE`
- `AGENT_THREAD_RESUME_WORKER_INTERVAL_MS`

### Memory

- `AGENT_MEMORY_FLUSH_WORKER_ENABLED=true|false`
- `AGENT_MEMORY_FLUSH_WORKER_ID`
- `AGENT_MEMORY_FLUSH_WORKER_BATCH_SIZE`
- `AGENT_MEMORY_FLUSH_WORKER_INTERVAL_MS`
- `AGENT_MEMORY_FLUSH_WORKER_LEASE_TTL_MS`
- `AGENT_MEMORY_FLUSH_WORKER_MAX_ATTEMPTS`
- `AGENT_MEMORY_FLUSH_WORKER_RETRY_BACKOFF_MS`
- `AGENT_MEMORY_EXTRACTOR_ENABLED=true|false`
- `AGENT_MEMORY_EXTRACTOR_MODEL_ID`
- `AGENT_MEMORY_EXTRACTOR_MODEL_NAME`
- `AGENT_MEMORY_EXTRACTOR_TEMPERATURE`
- `AGENT_MEMORY_EXTRACTOR_TOP_P`
- `AGENT_MEMORY_EXTRACTOR_MAX_TOKENS`
- `AGENT_MEMORY_EXTRACTOR_MAX_FACTS`

### Artifacts

- `AGENT_ARTIFACT_SCAN_WORKER_ENABLED=true|false`
- `AGENT_ARTIFACT_SCAN_WORKER_ID`
- `AGENT_ARTIFACT_SCAN_WORKER_SCANNER`
- `AGENT_ARTIFACT_SCAN_WORKER_BATCH_SIZE`
- `AGENT_ARTIFACT_SCAN_WORKER_INTERVAL_MS`
- `AGENT_ARTIFACT_SCAN_WORKER_LEASE_TTL_MS`
- `AGENT_ARTIFACT_SCAN_WORKER_MAX_ATTEMPTS`
- `AGENT_ARTIFACT_SCAN_WORKER_RETRY_BACKOFF_MS`
- `AGENT_ARTIFACT_SCANNER_TYPE=http|clamd|clamav`
- `AGENT_ARTIFACT_SCANNER_HTTP_URL`
- `AGENT_ARTIFACT_SCANNER_HTTP_TOKEN`
- `AGENT_ARTIFACT_SCANNER_HTTP_TIMEOUT_MS`
- `AGENT_ARTIFACT_SCANNER_HTTP_MAX_BYTES`
- `AGENT_ARTIFACT_SCANNER_CLAMD_ADDR`
- `AGENT_ARTIFACT_SCANNER_CLAMD_TIMEOUT_MS`
- `AGENT_ARTIFACT_SCANNER_CLAMD_MAX_BYTES`
- `AGENT_ARTIFACT_SCAN_OUTAGE_FAIL_MODE=closed|open_non_executable`

### Web Tools

- `AGENT_THREAD_WEB_SEARCH_ENABLED=true|false`
- `AGENT_THREAD_WEB_SEARCH_PROVIDER=auto|ddg|brave|wikipedia|http`
- `AGENT_THREAD_WEB_SEARCH_ENDPOINT`
- `AGENT_THREAD_WEB_SEARCH_API_KEY`
- `AGENT_THREAD_WEB_SEARCH_HEADER`
- `AGENT_THREAD_WEB_SEARCH_DDG_REGION`
- `AGENT_THREAD_WEB_SEARCH_DDG_SAFESEARCH`
- `AGENT_THREAD_WEB_SEARCH_BRAVE_BASE_URL`
- `AGENT_THREAD_WEB_SEARCH_WIKIPEDIA_BASE_URL`
- `AGENT_THREAD_WEB_SEARCH_WIKIPEDIA_LANGUAGE`
- `AGENT_THREAD_WEB_SEARCH_WIKIPEDIA_USER_AGENT`
- `AGENT_THREAD_WEB_SEARCH_WIKIPEDIA_DOC_MAX_CHARS`
- `AGENT_THREAD_WEB_SEARCH_TIMEOUT_MS`
- `AGENT_THREAD_WEB_SEARCH_MAX_RESPONSE_BYTES`
- `AGENT_THREAD_WEB_SEARCH_ALLOW_HTTP`
- `AGENT_THREAD_WEB_SEARCH_ALLOW_PRIVATE_IPS`

### MCP Runtime

- `AGENT_THREAD_MCP_RUNTIME_ENABLED=true|false`
- `AGENT_THREAD_MCP_RUNTIME_TIMEOUT_MS`
- `AGENT_THREAD_MCP_RUNTIME_MAX_OUTPUT_BYTES`
- `AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED=true|false`
- `AGENT_THREAD_MCP_STDIO_EINO_ENABLED=true|false`
- `AGENT_THREAD_MCP_STDIO_WORKDIR_ROOT`
- `AGENT_THREAD_MCP_STDIO_WORKER_ID`
- `AGENT_THREAD_MCP_STDIO_ALLOWED_COMMANDS`
- `AGENT_THREAD_MCP_STDIO_ALLOWED_ENV_KEYS`
- `AGENT_THREAD_MCP_STDIO_MAX_ARGS`
- `AGENT_THREAD_MCP_STDIO_MAX_ARG_BYTES`
- `AGENT_THREAD_MCP_STDIO_MAX_ENV_VARS`
- `AGENT_THREAD_MCP_STDIO_MAX_ENV_VALUE_BYTES`
- `AGENT_THREAD_MCP_STDIO_LEASE_TTL_MS`
- `AGENT_THREAD_MCP_STDIO_MAX_CONFIG_BYTES`
- `AGENT_THREAD_MCP_STDIO_DRY_RUN_OUTPUT_BYTES`
- `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ENABLED=true|false`
- `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ROOT`
- `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_BATCH_SIZE`
- `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_INTERVAL_MS`
- `AGENT_THREAD_MCP_REMOTE_EINO_ENABLED=true|false`
- `AGENT_THREAD_MCP_REMOTE_ALLOWED_HOSTS`
- `AGENT_THREAD_MCP_REMOTE_ALLOW_HTTP=true|false`
- `AGENT_THREAD_MCP_REMOTE_MAX_CONFIG_BYTES`
- `AGENT_THREAD_MCP_REMOTE_MAX_HEADERS`
- `AGENT_THREAD_MCP_REMOTE_MAX_HEADER_BYTES`

### Guardrail

Guardrail flags, metrics, archive, retention, and rollback steps are maintained
in `docs/superpowers/runbooks/guardrail-audit-operations.md`.

## Safe Signals

- Task/thread/run IDs, status, timestamps, runtime mode, worker ID, terminal
  error code, and bounded status messages.
- Runtime doctor status categories and counts.
- Worker processed/claimed/succeeded/failed counts.
- MCP server ID, tool name, event type, latency, output bytes, health status,
  and sanitized error code.
- Artifact scan job status, scanner type, size bytes, content type, retry count,
  and sanitized scan error code.
- Memory flush job status, attempt count, extracted fact count, and sanitized
  extractor error code.
- Token totals, model snapshot, model call IDs, trace ID, span ID, and
  parent-span ID.
- Web search provider, result count, elapsed time, and sanitized error code.
- Guardrail decision metadata and Prometheus labels approved in the Guardrail
  runbook.

## Forbidden Signals

- Prompt text, user private content, model completion text, hidden system
  prompts, chain-of-thought, raw model provider responses, and raw tool payloads.
- Tool arguments, tool results, MCP raw JSON-RPC bodies, stdio process stdout
  beyond bounded summaries, and remote MCP headers.
- Credentials, tokens, auth env values, object URIs, filesystem paths outside
  safe display allow-lists, checkpoint bytes, database SQL, and raw scanner
  bodies.
- High-cardinality or sensitive values as metric labels, including user IDs,
  object keys, filenames, prompts, model text, raw URLs, raw error bodies, and
  provider request IDs unless a reviewed contract explicitly allows them.

## Smoke Verification

Run the smallest matching set before enabling a new path:

```bash
cd backend
go test ./application/agentthread -run 'TestRuntimePolicyFromEnv|TestRunWorker|TestResumeRunWorker|TestMemoryFlushWorker|TestArtifactScanWorker' -count=1
go test ./application/agentthread -run 'TestADKUsage|TestADKWebSearch|TestADKMCPRuntime|TestGuardrailPrometheus' -count=1
go test ./api/handler/coze -run 'TestWorkbenchRuntimeDoctor|TestListTaskThreadMCPRuntimeAuditEvents|TestListTaskThreadGuardrailAuditEvents|TestListTaskThreadArtifactScanJobs' -count=1
```

For frontend task-detail regression after changing visible diagnostics:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json
```

For local browser smoke, use the same prompt in DeerFlow and Coze:

1. Create a task.
2. Confirm task appears in `我的任务` without refresh.
3. Confirm task reaches terminal status.
4. Confirm task detail shows answer, steps, token usage, artifacts if any,
   runtime doctor, MCP audit, and Guardrail audit without exposing forbidden
   signals.
5. Submit a follow-up and confirm previous history remains visible.

## Rollback

Runtime rollback should use feature flags before code rollback:

1. Stop new Eino ADK selection:
   - Set `AGENT_THREAD_RUNTIME_DEFAULT=legacy`.
   - Set `AGENT_THREAD_EINO_ADK_ENABLED=false` only if queued Eino runs should
     fail closed instead of executing.
2. Stop workers:
   - `AGENT_THREAD_WORKER_ENABLED=false`
   - `AGENT_THREAD_RESUME_WORKER_ENABLED=false`
3. Stop optional async paths:
   - `AGENT_MEMORY_FLUSH_WORKER_ENABLED=false`
   - `AGENT_ARTIFACT_SCAN_WORKER_ENABLED=false`
   - `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ENABLED=false`
4. Disable risky runtime tools:
   - `AGENT_THREAD_WEB_SEARCH_ENABLED=false`
   - `AGENT_THREAD_MCP_RUNTIME_ENABLED=false`
   - `AGENT_THREAD_MCP_STDIO_EINO_ENABLED=false`
   - `AGENT_THREAD_MCP_REMOTE_EINO_ENABLED=false`
5. For Guardrail metrics/archive/retention rollback, follow the Guardrail
   runbook and disable retention before archive cleanup.
6. Restart backend API/workers.
7. Verify runtime doctor reports the intended disabled or degraded state.
8. Verify no new run worker, memory flush, artifact scan, MCP runtime, or
   Guardrail cleanup observations are emitted after rollback.

## Incident Triage

- New tasks stay queued:
  - Check `AGENT_THREAD_WORKER_ENABLED`, worker logs, database connectivity,
    and run-claim counts.
- Follow-up or confirmation resume does not continue:
  - Check `AGENT_THREAD_RESUME_WORKER_ENABLED`, checkpoint presence, resume run
    status, and idempotency keys.
- Model calls hang or fail:
  - Check runtime doctor model section, model env prefix `AGENT_THREAD_`,
    provider timeout, and token usage rows.
- Web search is slow:
  - Check provider selection, local proxy settings, timeout, response budget,
    and whether provider fallback is expected.
- MCP tools fail:
  - Check tool config, transport policy, health status, audit event error code,
    output budget, and allowed command/host lists.
- Artifacts appear blocked or missing:
  - Check scan job status, scanner configuration, fail mode, and manual review
    decisions.
- Memory does not appear:
  - Check memory flush worker and extractor are both enabled, extractor model
    settings, retry count, and flush job errors.
- Guardrail decisions look too strict:
  - Check provider health, fail mode, audit decision distribution, and disable
    retention/metrics changes before changing policy.

## Open Gaps

- General OpenTelemetry exporter is intentionally not implemented in current
  P2-D; keep using the safe Prometheus families and metadata-only runtime
  records until exporter policy is reviewed.
- Dashboard definitions and alert routing remain operator enhancements on top of
  the implemented metrics contract.
