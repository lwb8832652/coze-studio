# DeerFlow P0 Release Readiness

This document is the deployment and known-issue checklist for the first
launchable DeerFlow-parity cutline. Keep secrets out of this file; use ignored
environment files or the production secret manager.

## Scope

- Product vocabulary stays task-oriented: `新建任务`, `全部任务`, `我的任务`,
  `任务详情`, and `任务记忆`.
- Runtime target is Go-native Eino ADK. No Python sidecar is part of P0.
- IM Channels, complex security scanners, Prometheus/OpenTelemetry exporter
  work, deep model/provider matrices, and full browser CI suites remain P1/P2.

## Required Services

| Service | P0 Requirement | Notes |
| --- | --- | --- |
| MySQL-compatible DB | Required | Configure `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_DATABASE`, `MYSQL_DSN`, and `ATLAS_URL` in the ignored env file or secret manager. Do not commit endpoint values or credentials. |
| Atlas | Required for schema gate | Use Atlas Community `v0.35.0`. Validate with `atlas migrate validate --dir file://docker/atlas/migrations`; inspect/apply only against an approved target. |
| Redis | Required | Configure `REDIS_ADDR` and `REDIS_PASSWORD`. |
| MinIO/S3/TOS storage | Required | P0 local/debug uses MinIO. Production may use S3/TOS if `STORAGE_TYPE` and provider credentials are configured. |
| NSQ or configured MQ | Required | P0 default is `COZE_MQ_TYPE=nsq` and `MQ_NAME_SERVER`. |
| Elasticsearch | Required for existing search paths | Configure `ES_ADDR`, `ES_VERSION`, and optional auth. |
| Milvus/vector store | Required for existing knowledge/vector paths | Configure `VECTOR_STORE_TYPE` and matching provider settings. |
| etcd | Required by the existing middleware stack | Keep the compose/service health gate enabled. |

Debug compose notes:

- `make middleware` should not start the local `mysql:8.4.5` service or the
  MySQL-image init client by default.
- The local MySQL container is a manual fallback only via `local-mysql` or
  `mysql` profile.
- `make sql_init` uses a local `mysql` or `mariadb` client and the ignored
  debug env file.
- Avoid pasting full `docker compose config --env-file docker/.env.debug`
  output into chat or logs because Compose expands secrets.

## Runtime Env

### Agent Runtime

For P0 launch, set the runtime policy explicitly:

```bash
export AGENT_THREAD_RUNTIME_DEFAULT=eino_adk
export AGENT_THREAD_EINO_ADK_ENABLED=true
```

Run workers are disabled unless explicitly enabled:

```bash
export AGENT_THREAD_WORKER_ENABLED=true
export AGENT_THREAD_WORKER_ID=agent-run-worker
export AGENT_THREAD_WORKER_BATCH_SIZE=10
export AGENT_THREAD_WORKER_INTERVAL_MS=2000

export AGENT_THREAD_RESUME_WORKER_ENABLED=true
export AGENT_THREAD_RESUME_WORKER_ID=agent-resume-worker
export AGENT_THREAD_RESUME_WORKER_BATCH_SIZE=10
export AGENT_THREAD_RESUME_WORKER_INTERVAL_MS=2000
```

Use one worker identity per deployment unit. Do not run multiple workers with
the same ID in the same environment.

### Model Configuration

Configure either run-selected model IDs through Workbench runtime settings or
the agent-thread built-in model prefix used by the runtime:

```bash
export AGENT_THREAD_BUILTIN_CM_TYPE=ark
export AGENT_THREAD_BUILTIN_CM_ARK_API_KEY=...
export AGENT_THREAD_BUILTIN_CM_ARK_MODEL=...
export AGENT_THREAD_BUILTIN_CM_ARK_BASE_URL=...
```

The general Coze model envs (`MODEL_*`, `BUILTIN_CM_*`) still power existing
agent/workflow paths. Runtime Doctor P0 checks configuration presence and safe
model resolution; live provider capability matrices remain P1.

### Skills

Skill management and runtime loading are P0-ready through the existing
Workbench Skill APIs and Eino ADK Skill middleware. No extra process is
required. Runtime selection is controlled by task run config fields:

- `enable_skills`
- `skills.enabled`
- `skills.allowed_skills`

### MCP Tools

MCP catalog and health metadata are P0-ready. Runtime execution is disabled
unless the MCP runtime is explicitly enabled:

```bash
export AGENT_THREAD_MCP_RUNTIME_ENABLED=true
export AGENT_THREAD_MCP_RUNTIME_TIMEOUT_MS=300000
export AGENT_THREAD_MCP_RUNTIME_MAX_OUTPUT_BYTES=65536
```

For stdio MCP tools, enable exactly one execution mode:

```bash
# Dry-run smoke mode.
export AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED=true
export AGENT_THREAD_MCP_STDIO_EINO_ENABLED=false

# Or real Eino stdio execution.
export AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED=false
export AGENT_THREAD_MCP_STDIO_EINO_ENABLED=true
```

Required stdio policy values:

```bash
export AGENT_THREAD_MCP_STDIO_WORKDIR_ROOT=/var/lib/coze/mcp
export AGENT_THREAD_MCP_STDIO_WORKER_ID=mcp-stdio-worker
export AGENT_THREAD_MCP_STDIO_ALLOWED_COMMANDS=npx,node,python
```

Optional bounded policy values include
`AGENT_THREAD_MCP_STDIO_ALLOWED_ENV_KEYS`,
`AGENT_THREAD_MCP_STDIO_MAX_ARGS`, `AGENT_THREAD_MCP_STDIO_MAX_ARG_BYTES`,
`AGENT_THREAD_MCP_STDIO_MAX_ENV_VARS`,
`AGENT_THREAD_MCP_STDIO_MAX_ENV_VALUE_BYTES`,
`AGENT_THREAD_MCP_STDIO_LEASE_TTL_MS`,
`AGENT_THREAD_MCP_STDIO_MAX_CONFIG_BYTES`, and
`AGENT_THREAD_MCP_STDIO_DRY_RUN_OUTPUT_BYTES`.

Enable stale workdir cleanup only after the stdio workdir root is stable:

```bash
export AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ENABLED=true
export AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ROOT=/var/lib/coze/mcp
```

### Web Search

Web search is disabled by default. Enable only with a trusted endpoint:

```bash
export AGENT_THREAD_WEB_SEARCH_ENABLED=true
export AGENT_THREAD_WEB_SEARCH_ENDPOINT=https://search.example.internal/query
export AGENT_THREAD_WEB_SEARCH_API_KEY=...
export AGENT_THREAD_WEB_SEARCH_HEADER=Authorization
export AGENT_THREAD_WEB_SEARCH_TIMEOUT_MS=10000
export AGENT_THREAD_WEB_SEARCH_MAX_RESPONSE_BYTES=65536
export AGENT_THREAD_WEB_SEARCH_ALLOW_HTTP=false
export AGENT_THREAD_WEB_SEARCH_ALLOW_PRIVATE_IPS=false
```

### Memory

Task memory management is P0-ready. Model-backed extraction can remain disabled
for the first launch:

```bash
export AGENT_MEMORY_EXTRACTOR_ENABLED=false
export AGENT_MEMORY_FLUSH_WORKER_ENABLED=false
```

Enable extraction only after model settings and worker ownership are decided:

```bash
export AGENT_MEMORY_EXTRACTOR_ENABLED=true
export AGENT_MEMORY_EXTRACTOR_MODEL_ID=...
export AGENT_MEMORY_EXTRACTOR_MAX_TOKENS=512
export AGENT_MEMORY_EXTRACTOR_MAX_FACTS=16
export AGENT_MEMORY_FLUSH_WORKER_ENABLED=true
export AGENT_MEMORY_FLUSH_WORKER_ID=memory-flush-worker
```

### Guardrails And Artifact Scanning

P0 may ship with these disabled:

```bash
export AGENT_GUARDRAIL_PROVIDER_TYPE=""
export AGENT_GUARDRAIL_AUDIT_RETENTION_WORKER_ENABLED=false
export AGENT_ARTIFACT_SCANNER_TYPE=""
export AGENT_ARTIFACT_SCAN_WORKER_ENABLED=false
```

Enable guardrails or scanners only after the external service, policy, and
failure mode are validated in a non-production environment.

## Release Verification

Run these commands before declaring a P0 release candidate:

```bash
atlas migrate validate --dir file://docker/atlas/migrations

# Inspect approved DB target without printing secrets.
. docker/.env.debug >/dev/null 2>&1
atlas schema inspect -u "$ATLAS_URL" \
  --format '{{ sql . }}' \
  --exclude 'atlas_schema_revisions,table_*' >/tmp/coze_atlas_schema.sql

cd frontend/apps/coze-studio
rushx test -- \
  src/pages/skill/__tests__/skill.test.tsx \
  src/pages/skill/__tests__/skill-version-panel.test.tsx \
  src/pages/tools/__tests__/tools.test.tsx \
  src/pages/tools/__tests__/tools-service.test.ts \
  src/pages/workbench/__tests__/workbench.test.tsx

cd ../../../backend
go test -gcflags="all=-l -N" ./api/handler/coze \
  -run 'Test(WorkbenchMCPToolHandlers|ListSkillVersionsHandler|ListSkillVersionResourcesHandler|ExportSkillVersionHandler|RollbackSkillVersionHandler|DeleteSkillHandler|ListSkillToolCandidatesHandler|UpdateSkillVersion(Resource|Content)Handler)' \
  -count=1
go test ./api/router/coze \
  -run 'TestRegisterIncludesWorkbench(SkillVersion|MCPTool)Routes' \
  -count=1
go test ./application/skill ./domain/skill/service ./domain/skill/repository ./application/mcptool \
  -run 'Test' \
  -count=1
go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'Test(ADKSkill|ADKMCP|ADKTool|DefaultADKTool|ADKSingleAgentToolGrant|ADKSubagentToolProvider)' \
  -count=1
go test -gcflags="all=-l -N" ./api/handler/coze \
  -run 'TestGetTaskThreadTokenUsageHandler(ReturnsRowsAndAggregate|CanIncludeChildRuns)' \
  -count=1
go test -gcflags="all=-l -N" ./api/handler/coze \
  -run 'Test(ListTaskThreadRunEventsHandler(RedactsUnsafePayload|ReturnsEvents)|StreamTaskThreadRunEventsWritesEventsAndDone|TaskThreadRunEventStreamStopsForInterruptedRun)' \
  -count=1
```

Browser smoke remains required before RC:

- create a new task;
- observe stream/event timeline to one terminal state;
- reopen and refresh task detail;
- click the visible cancel/retry controls and confirm the detail refreshes;
- open `任务记忆`, search/edit/delete/restore/import/export;
- confirm Runtime Doctor shows runtime, model, Web, MCP, Skill, and memory
  status without secrets;
- confirm token usage renders when usage exists and is omitted when missing.

## Known P0 Issues

- Full desktop browser smoke is still pending. API-level smoke has covered
  canonical task creation, pending cancel, task-level retry metadata, and
  task-memory search, edit, delete, restore, import, and export on the live
  local server, but the in-app browser automation timed out while capturing
  DOM/screenshot state, so the visible cancel/retry button path and visual
  `任务记忆` panel confirmation still need a manual browser pass before RC.
- Atlas apply against the approved external debug DB target completed, and the
  follow-up dry-run reports `Schema is synced, no changes to be made`. The
  debug flow intentionally no longer pulls a MySQL dev image.
- A broad `go test ./api/handler/coze ... -run Test` command runs unrelated
  legacy handler tests and currently fails on non-P0 Workflow/Conversation
  paths plus Mockey `-gcflags` requirements. Use the targeted P0 commands
  above for this cutline.
- `.codex/config.toml` is an untracked local Codex file in this worktree and
  should not be committed as part of P0.
- Runtime Doctor P0 does not perform deep live model/provider capability
  matrices; this remains P1.
- Cost/pricing snapshots, complex policy UI, full browser CI, complex
  scanners, Prometheus/OpenTelemetry exporters, load/chaos gates, and operator
  runbooks remain P1/P2.

## Rollback

- Prefer redeploying the previous application image/commit for application
  rollback.
- To fail closed without redeploying, disable optional runtime workers and
  external tool paths:
  `AGENT_THREAD_WORKER_ENABLED=false`,
  `AGENT_THREAD_RESUME_WORKER_ENABLED=false`,
  `AGENT_THREAD_MCP_RUNTIME_ENABLED=false`,
  `AGENT_THREAD_WEB_SEARCH_ENABLED=false`,
  `AGENT_MEMORY_EXTRACTOR_ENABLED=false`,
  and `AGENT_MEMORY_FLUSH_WORKER_ENABLED=false`.
- Do not switch new tasks to `legacy`; after `AR-PARITY-002.1` it is a
  historical checkpoint compatibility path only. Redeploy the previous
  approved image for execution-runtime rollback.
- Treat DB migrations as forward-only unless a rollback migration has been
  reviewed. Take a DB backup before release-gate apply.
- Do not rotate or expose secrets through rollback logs. Keep env files and
  Compose rendered output out of issue reports and chat logs.
