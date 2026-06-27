# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Project Overview

Coze Studio is an all-in-one AI agent development platform with both frontend (React + TypeScript) and backend (Go) components. The project uses a sophisticated monorepo architecture managed by Rush.js with 135+ frontend packages organized in a hierarchical dependency system.

## Development Commands

### Environment Setup
```bash
# Clone and setup
git clone https://github.com/coze-dev/coze-studio.git
cd coze-studio

# Install frontend dependencies
rush update

# For Docker-based development
cd docker
cp .env.example .env
# Configure model settings in backend/conf/model/
docker compose up -d
# Access at http://localhost:8888
```

### Development Workflow
```bash
# Start middleware services (MySQL, Redis, Elasticsearch, etc.)
make middleware

# Start Go backend in development mode
make server

# Start frontend development server
cd frontend/apps/coze-studio
npm run dev

# Full development environment
make debug
```

### Build Commands
```bash
# Build frontend only
make fe

# Build Go server
make build_server

# Build everything with Docker
make web

# Rush monorepo commands
rush build                    # Build all packages
rush rebuild -o @coze-studio/app  # Build specific package
rush test                     # Run all tests
rush lint                     # Lint all packages
```

### Testing
```bash
# Run tests (Vitest-based)
rush test
npm run test                  # In specific package
npm run test:cov             # With coverage

# Backend tests
cd backend && go test ./...
```

## Architecture Overview

### Frontend Architecture
- **Monorepo**: Rush.js with 135+ packages across 4 dependency levels
- **Build System**: Rsbuild (Rspack-based) for fast builds
- **UI Framework**: React 18 + TypeScript + Semi Design + Tailwind CSS
- **State Management**: Zustand for global state
- **Package Organization**:
  - `arch/`: Core infrastructure (level-1)
  - `common/`: Shared components and utilities (level-2)
  - `agent-ide/`, `workflow/`, `studio/`: Feature domains (level-3)
  - `apps/coze-studio`: Main application (level-4)

### Semi UI and Frontend Styling

- Use the repository's Semi-based design system for product UI. In
  `frontend/apps/coze-studio`, prefer components from
  `@coze-arch/coze-design` and icons from
  `@coze-arch/coze-design/icons`.
- Do not introduce another component library or hand-roll a control when an
  equivalent Semi/Coze Design component exists.
- Before using a component API or adding custom component CSS, query the Semi
  MCP when it is available:
  - Use `get_semi_document` for component props, examples, accessibility,
    content guidance, theme variables, and design tokens.
  - Use `get_component_file_list`, then `get_file_code` or
    `get_function_code`, when the public documentation is insufficient or the
    implementation and class structure must be inspected.
- Query the version used by the workspace instead of assuming the MCP default.
  Determine it from the target package and Rush lockfile first. At the time of
  writing, `@coze-arch/coze-design` uses `@douyinfe/semi-ui` `2.72.3`.
- Prefer component props such as `theme`, `type`, `size`, `loading`,
  `disabled`, `icon`, and `block`, plus existing Semi/Coze theme CSS variables,
  before adding bespoke selectors or hard-coded colors.
- Reuse existing project tokens, Less utilities, and nearby component patterns.
  Keep custom styles scoped to layout or product-specific presentation that the
  component API and theme tokens cannot express.
- Import directly from `@douyinfe/semi-ui` only when the Coze Design wrapper
  does not expose the required component and the surrounding package already
  follows that direct-import pattern.
- Preserve keyboard behavior, focus states, ARIA labels, loading states,
  disabled states, validation, empty states, and error states supplied by Semi.
- When visual behavior is unclear, treat Semi MCP documentation and the
  installed component source as the source of truth, then verify the result in
  the browser at desktop and mobile sizes.

### Backend Architecture (Go)
- **Framework**: Hertz HTTP framework
- **Architecture**: Domain-Driven Design (DDD) with microservices
- **Agent Runtime Baseline**: Eino `v0.9.9` with Eino ADK as the target
  execution kernel
- **Structure**:
  - `domain/`: Business logic and entities
  - `application/`: Application services and use cases
  - `api/`: HTTP handlers and routing
  - `infra/`: Infrastructure implementations
  - `crossdomain/`: Cross-cutting concerns

### Agent Runtime Direction

- Active delivery priority is Phase 1 DeerFlow parity. First completely
  replicate DeerFlow 2.x task/conversation, execution flow, Agent, Skill, MCP
  tool, memory, token-usage, and settings capabilities in Coze Studio while
  keeping the product vocabulary task-oriented.
- Do not start or extend non-parity platform hardening while Phase 1 remains
  incomplete. Guardrail audit archive/retention, Prometheus exporters,
  OpenTelemetry exporters, complex security scanning, policy admin UI, storage
  lifecycle/reconciliation, load/chaos gates, and similar enhancements belong
  to Phase 2 unless they directly block a DeerFlow parity feature or a required
  regression fix.
- When a Phase 1 implementation reveals an enhancement or optimization that is
  not necessary to match DeerFlow behavior, record it in the Phase 2 backlog
  instead of implementing it inline. For ambiguous work, choose the smallest
  production-compatible adapter needed for DeerFlow parity and defer broader
  platform optimization.
- Phase 1 mainline task order is:
  1. task conversation and task-detail execution flow;
  2. Go-native Eino ADK runtime, streaming, resume, cancellation, and
     LangGraph-compatible API behavior required by DeerFlow workflows;
  3. Agent and subagent configuration backed by existing SingleAgent records;
  4. Skill create/edit/enable/version/history/slash/progressive activation;
  5. MCP and tool configuration, durable catalog, runtime invocation, and
     DeerFlow-visible tool health;
  6. memory retrieval/update/edit/import/export and task-detail memory UI;
  7. token usage attribution and task-detail/settings display;
  8. DeerFlow-equivalent settings and acceptance tests.
- Do not choose the next task from M8 or other Phase 2 hardening while any
  Phase 1 mainline item above still has parity gaps.
- The production target is a Go-native Agent Harness. Do not introduce a
  Python sidecar as a runtime dependency or migration bridge.
- Use Eino ADK for the execution kernel:
  - `ChatModelAgent`, `Runner`, `AgentEvent`, tool calling, streaming, and
    interrupt/resume.
  - `AgentTool` and `DeepAgent` for delegated or multi-agent work.
  - `TurnLoop` for push, preemption, safe-point cancellation, and resumable
    multi-turn execution.
  - ADK middleware for summarization, tool-result reduction, dangling
    tool-call repair, filesystem access, dynamic tool search, plan tasks,
    `AGENTS.md`, and `SKILL.md` loading.
- Follow an Eino-first rule for new Harness behavior. Before implementing an
  Agent loop, retry/failover, Todo/Plan, context summarization, tool-result
  offloading, tool-call repair, deferred tool search, filesystem tool, Skill
  executor, or subagent algorithm, verify whether Eino ADK already provides it.
  Prefer a Coze adapter around the Eino primitive instead of a second
  implementation.
- Prefer `ChatModelAgent` with `AgentTool` or `DeepAgent` for new multi-agent
  work. Eino's sequential, parallel, and loop workflow agents are available,
  but upstream marks them as not recommended for most scenarios.
- For subagents, use `ADKSubagentToolProvider` to adapt Coze-owned child Agent
  definitions into Eino `adk.NewAgentTool` instances. Do not build a custom
  delegation loop when `AgentTool` is sufficient.
- The existing SingleAgent draft/version/publish tables remain the durable
  Agent system of record for M3. `ADKRunConfigSubagentDefinitionProvider` is a
  temporary contract-test source, not a durable product API.
- Use `ADKSingleAgentSubagentDefinitionProvider` when subagent definitions must
  come from existing SingleAgent records. It may resolve temporary
  `subagent_refs` / `subagentRefs` through `ADKRunConfigSubagentReferenceProvider`,
  but production AgentHub settings should replace that run-config reference
  source.
- Use `ADKSingleAgentSubagentAgentFactory` to build runnable child ADK agents
  from SingleAgent draft/version snapshots. It maps only stable snapshot fields:
  `PromptInfo.Prompt`, `ModelInfo.ModelId`, `Temperature`, `MaxTokens`, and
  `TopP`, plus the resolved child name and description. The generated child
  `RunSummary` preserves parent run, thread, space, and creator identity for
  current attribution, sets `AssistantID` to `singleagent:{agent_id}`, and must
  not inherit parent run config such as web tools, tool catalogs, or model
  overrides.
- Use `ADKToolPolicyProvider` as the common allow-list boundary for ADK tools.
  Absence of `tool_policy` / `toolPolicy` keeps existing tools unchanged;
  presence of an explicit empty allow-list denies that tool class. Child
  SingleAgent runs must emit a `tool_policy` in generated config and default to
  empty `allowed_tools` and `allowed_dynamic_tools`, so child agents do not
  inherit parent tools unless their `ADKSubagentDefinition` or
  `ADKSubagentReference` explicitly grants tool names. Future Skill, MCP, web,
  filesystem, and human-interaction grants should feed this same policy layer
  instead of adding parallel filtering logic. Use
  `ADKSubagentToolGrantProvider` as the durable grant entry point for
  SingleAgent-backed subagents: it returns allowed static/dynamic tool names
  only and must not construct tools, invoke MCP, load Skill content, or decide
  model visibility directly. When a grant provider is configured, requested
  tool names are intersected with durable grants so temporary run config cannot
  broaden permissions. Use `ADKSingleAgentSnapshotToolGrantProvider` as the
  first snapshot-backed grant adapter: valid `PluginInfo.ApiName` and
  `WorkflowInfo.WorkflowName` values are used directly; unsafe names fall back
  to `plugin_{plugin_id}_{api_id}` and `workflow_{workflow_id}`. Future Tool
  Registry and MCP adapters must reuse the same naming helpers so grant names,
  model-visible names, executable names, audit records, and UI grant displays
  cannot drift. Production ADK executor wiring should use
  `NewDefaultADKToolProviderWithSingleAgentSubagents(singleAgentDomainSVC)` so
  the lead run resolves runtime tools, human-interaction tools, and
  SingleAgent subagent tools through one outer `ADKToolPolicyProvider`. Child
  SingleAgent agents are built with the default non-recursive provider and are
  filtered by their generated child `tool_policy`; do not expose recursive
  subagent settings until durable AgentHub configuration exists. Pass the ADK
  event sink with `WithDefaultADKToolProviderEventSink` in production so
  `AgentTool` invocations emit content-free `subagent.run.started`,
  `subagent.run.completed`, and `subagent.run.failed` events on the parent run.
  These lifecycle payloads may include standardized `subagent` identity,
  agent/version IDs, status, elapsed time, and sanitized error messages, but
  must never include tool arguments, model input/output, final text, checkpoint
  bytes, or object URIs. Durable child run rows use
  `agent_runs.parent_run_id` plus `run_kind='subagent'`; top-level task runs
  use `parent_run_id=0` and `run_kind='task'`. Default run listing and worker
  claim paths must stay top-level-only unless a caller explicitly requests a
  parent run's children or an admin mixed view. Production subagent tools
  should pass `WithDefaultADKToolProviderSubagentRunRecorder` with
  `NewApplicationADKSubagentRunRecorder(agentThreadSVC)` so child rows are
  created at `AgentTool` start and moved to succeeded/failed/canceled on
  terminal outcome. Timeout remains `RunStatusFailed` with
  `subagent.run.failed`, `error_code='subagent_timeout'`, and
  `terminal_classification='timeout'`; cancellation becomes
  `RunStatusCanceled` with `subagent.run.canceled`,
  `error_code='subagent_canceled'`, and
  `terminal_classification='canceled'`; generic child errors remain failed
  with `error_code='subagent_failed'`. Lifecycle event payloads should include
  `child_run_id` when a child row exists and must stay content-free.
  Child subagent retry requests must not mutate or requeue the historical child
  run row. The Workbench retry API creates a new top-level queued task run
  (`parent_run_id=0`, `run_kind='task'`) with a metadata-only
  `subagent_retry` command that references the failed or canceled child
  `source_run_id` and parent run. This keeps worker claim semantics
  top-level-only while preserving immutable child-run history. Future executor
  support should consume that command to replay the child/subagent work and
  must keep retry events content-free. Until replay execution is implemented,
  the run processor must fail queued `subagent_retry` commands closed with
  `error_code='subagent_retry_not_supported'` unless the selected executor
  explicitly implements `SubagentRetryRunExecutor`. Replay-capable executors
  must handle `ExecuteSubagentRetry` through the same append-message,
  terminal-status, cancellation, interrupt, and failure mapping path as normal
  execution. Unsupported executors must never execute those commands as
  ordinary empty-message task runs.
  `ADKExecutor.ExecuteSubagentRetry` must load source child rows only through
  an injected `ADKSubagentRetrySourceResolver`, validate thread/source/parent
  identity, parse source `agent_runs.input` schema `coze.subagent_tool_call.v1`,
  rebuild the child agent from the source child config, and invoke it through
  Eino `adk.NewAgentTool`. Do not hand-roll child argument-to-message
  conversion when Eino AgentTool can perform it. Retry result metadata may
  include source and parent run IDs, but must not include tool arguments,
  model input/output, prompts, checkpoint bytes, object URIs, URLs, filenames,
  or provider payloads.
  `RuntimeSelector` must implement `SubagentRetryRunExecutor` and route retry
  execution through the same runtime policy as ordinary runs. `eino_adk`
  retries route to ADK only when the server policy allows ADK and the ADK
  executor implements retry; legacy or unsupported selected runtimes return
  `SubagentRetryUnsupportedError`, which `RunProcessor` maps to
  `error_code='subagent_retry_not_supported'` instead of generic
  `executor_error`.
  Child subagent run rows may persist the internal `AgentTool` invocation input
  needed for future replay as `agent_runs.input` schema
  `coze.subagent_tool_call.v1`. Child run `config` must also preserve the
  replay definition snapshot: runtime, agent name/description, SingleAgent
  reference, `full_chat_history`, and `tool_policy.allowed_tools` /
  `tool_policy.allowed_dynamic_tools`. Those payloads must stay internal
  execution contracts. Workbench API mappers must redact `command`, `input`,
  `config`, and `context` for `run_kind='subagent'` responses, while
  preserving safe identity, status, metadata, lifecycle, usage, and timestamp
  fields needed by task detail cards.
  Workbench task-thread run listing accepts `parent_run_id` to fetch a parent
  run's child rows explicitly, and `TaskThreadRun` API payloads expose
  `parent_run_id` plus `run_kind`; the default list must remain top-level-only.
  Canonical task detail UI may render child `run_kind='subagent'` rows as
  `子智能体执行` state cards using only durable run identity, status, sanitized
  error text, assistant identity, and timestamps. It must not render tool
  arguments, model input/output, final child text, checkpoint bytes, object
  URIs, or raw provider payloads in those cards.
  The task detail frontend may join latest `subagent.run.*` lifecycle events
  to those cards by `child_run_id` and display only content-free
  `elapsed_ms`, `terminal_classification`, and sanitized lifecycle error
  fallback metadata.
  It may also fetch run-scoped token usage for each durable child run through
  the existing task-thread token usage API and display total token count plus
  call count. Missing child usage must be omitted rather than treated as
  confirmed zero cost. When backend usage data includes positive `cost_micros`
  and the child run has exactly one non-empty currency across its usage rows,
  task detail may display a bounded formatted cost snapshot such as
  `Cost USD 0.000250`; omit cost display for missing or mixed currency. Pricing
  calculation, snapshots, cost accounting, and provider/model attribution
  remain Coze-owned backend responsibilities.
  The task-thread token usage API accepts `include_child_runs=true` with a
  parent `run_id` to list and aggregate usage for that parent run plus direct
  child `run_kind='subagent'` rows. The parameter is explicit to preserve
  existing run-scoped semantics; do not silently broaden historical `run_id`
  queries. Recursive descendant rollups should remain a separate product/API
  decision.
  Responses may include `run_aggregates`, an array of per-run aggregate
  metadata keyed by `run_id`. Task detail should use this grouped response to
  populate child subagent token cards without issuing one token-usage request
  per child run. Missing per-run aggregates still mean "omit usage display",
  not confirmed zero cost. Keep grouped usage payloads metadata-only: no
  prompt text, completion text, tool arguments/results, object URIs, raw
  provider bodies, or checkpoint bytes.
  Task detail may derive child subagent provider/model attribution from
  grouped token usage rows and display only a bounded `provider / model_name`
  label. If a child run has multiple distinct provider/model attribution
  labels, display the first observed label plus a bounded count suffix such as
  `provider / model_name +N`; do not expand a full provider/model breakdown in
  the card. This is observability metadata, not a pricing snapshot. Do not
  expose raw usage JSON, provider response bodies, prompt text, completion
  text, tool arguments/results, credentials, or object identifiers through that
  label.
  The same detail UI may render an expandable child lifecycle timeline from
  parent-run `subagent.run.*` events grouped by `child_run_id`. Timeline rows
  may show event title, status, timestamp, `elapsed_ms`,
  `terminal_classification`, and sanitized lifecycle error text only; do not
  include child final output, model input/output, tool arguments, tool results,
  checkpoint bytes, object URIs, URLs, filenames, or provider raw payloads.
  Task detail may expose a subagent `重试` action only for failed or canceled
  child `run_kind='subagent'` rows on canonical thread detail. The frontend
  action must call the Workbench retry API with only `thread_id` and child
  `run_id`, keep per-row loading state to prevent duplicate clicks, refresh
  task detail after success, and surface bounded user-facing errors. It must
  not send or render child command, input, config, context, tool arguments,
  model text, checkpoint bytes, object URIs, URLs, filenames, or provider raw
  payloads.
  Task detail may join top-level retry runs back to the source child subagent
  card only through safe retry metadata (`source='subagent_retry'`,
  `source_run_id`, status, sanitized error fields, and timestamps). Retry
  top-level runs must not be treated as ordinary parent runs for child-run or
  grouped token-usage queries, and the UI may display only compact retry count
  and latest retry status metadata.
  `ApplicationADKAgentFactory` must use the
  provider-returned filtered tool set as the single source for model
  `WithTools`, executable `ToolsNodeConfig.Tools`, and middleware
  `StaticTools` / `DynamicTools`; do not rebuild a separate model-visible tool
  list outside this path.
- Subagent tool names must be Eino-safe (`[A-Za-z_][A-Za-z0-9_]{0,63}`),
  unique across the parent run tool set, and backed by non-empty descriptions.
  ADK events with nested `RunPath` must expose the standardized `subagent`
  payload (`name`, `root_name`, `parent_name`, `step_id`, `run_path`, `depth`),
  and event-side token usage metadata must use the same identity fields without
  persisting message, tool-result, URL, file-name, or provider raw content.
  Parent/child authorization, allowed-tool intersection, quotas, tracing,
  token attribution, cancellation, and durable parent-child run records remain
  Coze-owned responsibilities.
- Enforce subagent explosion guards at `ADKSubagentToolProvider` before
  building child `AgentTool` instances. Defaults are `max_subagents=16` and
  `max_depth=2`, and child `AgentTool` invocation is wrapped with
  `timeout_ms=300000` by default. These values are overridable through run
  config `subagent_policy` / `subagentPolicy`. Child run metadata must carry
  the standardized `subagent` identity so recursive depth checks can be
  applied before nested child agents are constructed. The timeout wrapper
  preserves Eino tool metadata and propagates context cancellation/deadline
  errors without inspecting tool arguments or results.
- Use `*schema.Message` as the production message baseline. Eino `v0.9.9`
  documents incomplete stream cancellation and retry behavior for
  `*schema.AgenticMessage`; keep that path experimental until contract tests
  cover cancellation, retry, checkpoint, streaming, and provider parity.
- Keep Coze Studio as the control plane and system of record for:
  - tasks, threads, runs, messages, events, ownership, and tenant isolation;
  - durable checkpoints, leases, cancellation, retries, and recovery;
  - LangGraph-compatible HTTP and SSE contracts;
  - Agent, Skill, MCP, model, memory, sandbox, and tool configuration;
  - policy, guardrails, security scanning, audit, token attribution, and cost.
- Treat Eino `AgentEvent`, checkpoint data, token usage, and Skill runtime as
  internal execution contracts. Translate them through Coze-owned adapters
  before exposing or persisting public platform contracts.
- Use Eino response metadata and callbacks as raw usage and tracing inputs.
  Coze still owns idempotent attribution, pricing snapshots, cost accounting,
  OpenTelemetry span relationships, and durable audit records.
- ADK callback usage metadata must include Coze-owned `trace_id`, `span_id`,
  `parent_span_id`, `idempotency_key`, source, usage kind, provider, model
  name, retry attempt, and a bounded `model_metadata` snapshot. These fields
  are stable correlation inputs for future OpenTelemetry export; they are not
  a substitute for Coze-owned pricing, cost, or audit records.
- Callback usage and execution metadata must not include prompt text, model
  completion text, tool arguments, object keys, checkpoint bytes, credentials,
  or raw provider response bodies. Keep raw token usage limited to normalized
  count fields such as prompt, completion, total, cached, and reasoning tokens.
- Long-term memory facts use the existing `agent_thread_memories` table and
  `entity.Memory` model, not a parallel store. M7.1 adds durable
  `confidence`, `source_type`, `source_id`, `correction_of_memory_id`, and
  `corrected_at` fields. `score` remains the current recall ordering signal;
  `confidence` is fact quality metadata for future extraction/editing/audit.
  Domain memory creation must validate `confidence` in `[0,1]`, trim source
  fields, reject negative correction IDs/timestamps, and keep non-run scopes
  (`thread` and `long_term`) thread-level by clearing `run_id`. Application
  summaries, `ThreadMemoryProvider`, checkpoint memory values, and resume
  parsing should preserve these fields, but prompt injection should continue
  to expose only bounded memory scope/content until M7 retrieval and policy
  work explicitly broadens the model-visible payload.
- Memory flush jobs use `agent_memory_flush_jobs` as the durable async update
  queue. M7.2 adds worker lease metadata (`worker_id`, `lease_expires_at`,
  `started_at`, `ended_at`) and repository/service methods to claim due
  pending jobs, reclaim expired processing jobs, complete succeeded jobs,
  retry processing jobs back to pending with backoff, and fail jobs into the
  dead-letter/terminal failed state. Claim and terminal updates must use
  worker ID plus active lease compare-and-set semantics so multiple workers
  cannot process the same job. `last_error` is bounded/sanitized metadata
  only; do not persist transcript text, model output, prompts, provider raw
  bodies, tool arguments/results, object URIs, credentials, or checkpoint
  bytes in memory flush lifecycle fields or events. M7.2 is queue
  infrastructure only; actual memory extraction, debounce policy, structured
  fact mutation, UI management, and audit views remain later M7 work.
- M7.2b adds the memory flush processing boundary. `ProcessMemoryFlushJobs`
  must claim leased jobs through `ThreadSVC`, load transcript snapshots through
  the domain service, invoke only a configured `MemoryExtractor`, and persist
  returned `MemoryExtractionFact` values through `RememberMemory`. Do not add
  heuristic natural-language memory extraction in this worker. Fact source IDs
  should stay stable and transcript-derived (`snapshot:<id>:<key-or-hash>`) so
  retries can skip already-written facts through the app-layer idempotency
  check. Worker lifecycle events must remain content-free: no transcript text,
  extracted fact content, prompts, model output, tool arguments/results,
  object URIs, credentials, checkpoint bytes, filenames, URLs, or provider raw
  payloads. `MemoryFlushWorker` is controlled by
  `AGENT_MEMORY_FLUSH_WORKER_*` env vars and must not start without a
  configured extractor. Production Eino/model-backed extraction, DB-level
  concurrent upsert uniqueness, debounce policy, memory UI, and audit views
  remain later M7 work.
- M7.2c adds `ModelMemoryExtractor` as the first production-shaped extractor
  implementation. It must reuse the existing Eino `model.BaseChatModel`
  provider path through `ChatModelProvider`, ask for JSON-only `facts`, accept
  only the bounded structured schema, normalize scopes/metadata, and clamp
  score/confidence before returning `MemoryExtractionFact` values. Keep model
  output parsing strict: invalid JSON should fail the extraction so the worker
  can retry/fail through the lease lifecycle. The extractor must not persist
  memory directly, emit events, decide worker retries, expose transcript text
  in errors/events, or bypass the worker's idempotency and policy boundary.
  Prompt evaluation fixtures, DB-level upsert uniqueness, and admin
  observability remain later M7 work.
- M7.2d wires `ModelMemoryExtractor` through disabled-by-default
  `AGENT_MEMORY_EXTRACTOR_*` env vars and `InitService`. Supported vars are
  `AGENT_MEMORY_EXTRACTOR_ENABLED`,
  `AGENT_MEMORY_EXTRACTOR_MODEL_ID`,
  `AGENT_MEMORY_EXTRACTOR_MODEL_NAME`,
  `AGENT_MEMORY_EXTRACTOR_TEMPERATURE`,
  `AGENT_MEMORY_EXTRACTOR_TOP_P`,
  `AGENT_MEMORY_EXTRACTOR_MAX_TOKENS`, and
  `AGENT_MEMORY_EXTRACTOR_MAX_FACTS`. When enabled, `InitService` attaches
  the extractor with `ThreadUsageCollector`, so extractor model calls record
  `TokenUsageSourceMiddleware` usage from `schema.ResponseMeta.Usage`.
  Extractor usage metadata may include only safe identifiers such as schema,
  source, usage kind, thread/run/snapshot IDs, model ID/name, finish reason,
  and idempotency key. It must not include transcript text, extracted fact
  content, prompts, model output, provider raw payloads, tool
  arguments/results, URLs, filenames, object keys, credentials, or checkpoint
  bytes.
- M7.2e adds source-based memory idempotency. `RememberMemory` must route only
  memories with both non-empty `source_type` and `source_id` through
  `CreateOrGetMemoryBySource`; empty-source manual/user memories must continue
  to allow multiple rows. The DB uses generated nullable `source_key` and
  unique `(thread_id, source_key)` so MySQL permits multiple NULL source keys
  but rejects duplicate extracted facts for the same thread/source pair.
  Future memory import, rollback, correction, and UI edit flows should either
  preserve source IDs intentionally or clear them when a new independent memory
  row is desired.
- M7.3 memory retrieval is config-driven at `ThreadMemoryProvider`. Run config
  `memory_retrieval` may set `limit`, `candidate_limit`, `scopes`,
  `min_confidence`, and optional `query`; absent query is derived from the
  latest user input. The provider asks the backend with `candidate_limit`,
  filters corrected memories and low-confidence facts, ranks deterministically
  by lexical query overlap and confidence-weighted score, and returns `limit`.
  ADK memory middleware remains the final token-budget boundary and
  model-visible memory remains bounded scope/content only. Do not introduce
  vector/embedding/semantic ranking, UI behavior, or another retrieval store
  under this slice.
- M7.4 starts the Workbench memory management backend boundary under
  `/api/workbench/task_threads/:thread_id/memories`. The API supports
  metadata-safe list/search (`q` or `query`), full-snapshot update, row
  soft-delete, and scoped clear through the existing `agent_thread_memories`
  table. `agent_thread_memories.deleted_at` is the soft-delete marker;
  runtime recall and default management list paths must exclude deleted rows.
  Management list uses explicit page/page_size and may include expired rows
  only when requested; it must not expose prompt text, model completion text,
  tool arguments/results, checkpoint bytes, object URIs, raw provider bodies,
  credentials, URLs, filenames, or hidden run config. Browser E2E remains
  later M7 work.
- M7.5 adds the canonical task-detail frontend memory management panel. Keep
  the user-facing object named `任务记忆` and mount it only for task-thread
  detail pages. The panel may list/search by safe row metadata, filter by
  `thread` / `run` / `long_term`, perform full-snapshot edits through the
  existing Workbench memory API, soft-delete rows, and scoped-clear the current
  range. List rows must not render raw metadata JSON, prompts, model
  input/output, tool arguments/results, checkpoint bytes, object URIs, raw
  provider payloads, credentials, URLs, filenames, or hidden run config.
  Browser E2E remains separate M7 work.
- M7.6 adds backend restore and metadata-only audit history for Workbench
  task-thread memories. Deleted memories remain hidden by default and are
  visible only through explicit `include_deleted=true` management queries.
  `POST /api/workbench/task_threads/:thread_id/memories/:memory_id/restore`
  may restore only a memory row that belongs to the same thread and has a
  non-zero `deleted_at`. Successful update, delete, clear, and restore
  operations write `agent_memory_audit_events` rows with only event identity,
  thread/run/space/memory/actor IDs, scope, source type/id, affected count, and
  timestamp. Audit APIs must not expose memory content, metadata JSON, prompts,
  model input/output, tool arguments/results, checkpoint bytes, object URIs,
  raw provider payloads, credentials, URLs, filenames, or hidden run config.
  Future frontend restore controls and audit-history views must use these
  metadata-only contracts and keep ordinary task detail memory lists unchanged.
- M7.7 adds frontend restore controls and audit-history viewing to the
  canonical task-detail `任务记忆` panel. Deleted memories remain hidden unless
  the user explicitly enables the `已删除` list toggle, which maps to
  `include_deleted=true`; default memory list behavior must not change.
  Restore actions may send only `thread_id` and `memory_id` through the
  Workbench restore API and should reload the current memory list afterward.
  The audit SideSheet may call the metadata-only audit API with `thread_id`,
  `memory_id`, `page`, and `page_size`, and may display only event type,
  scope, source type/id, affected count, timestamps, and safe identity fields.
  It must not render raw memory metadata JSON, prompts, model input/output,
  tool arguments/results, checkpoint bytes, object URIs, raw provider
  payloads, credentials, URLs, filenames, hidden run config, or restored
  memory content from audit payloads.
- M7.8 wires Workbench task-thread memory management into generated frontend
  API contracts. `idl/workbench/task.thrift` and
  `frontend/packages/arch/api-schema/src/idl/workbench/task.ts` are the
  contract sources for list/search, full-snapshot update, soft-delete,
  scoped clear, restore, and audit listing. Task-page code should import these
  endpoints through `frontend/apps/coze-studio/src/pages/tasks/service.ts`,
  which re-exports the generated `workbenchTask` clients and types. Do not add
  new hand-written fetch clients for these memory endpoints unless the
  generated client has a documented blocker. Generated memory and audit
  payloads must remain metadata-safe: no prompt text, model input/output, tool
  arguments/results, checkpoint bytes, object URIs, raw provider payloads,
  credentials, URLs, filenames, hidden run config, or restored memory content
  may be added to audit contracts.
- M7.9 adds backend and generated-client support for task memory
  import/export. `GET
  /api/workbench/task_threads/:thread_id/memories/export` returns bounded
  schema `coze.task_thread_memories.export.v1` data using the same safe memory
  row shape as the management list. `POST
  /api/workbench/task_threads/:thread_id/memories/import` accepts memory-row
  fields only, writes through the domain `ImportMemories` boundary, preserves
  source-based idempotency, and records a metadata-only aggregate
  `memory.imported` audit event with actor ID and affected count. Import/export
  contracts must not expose prompts, model input/output, tool
  arguments/results, checkpoint bytes, object URIs, raw provider payloads,
  credentials, URLs, filenames, hidden run config, or audit-derived memory
  content. Frontend buttons/dialogs should consume the generated
  `workbenchTask` clients re-exported by the task page service.
- M7.10 adds frontend controls for task memory import/export to the canonical
  task-detail `任务记忆` panel. The toolbar may expose `导出记忆` and
  `导入记忆` actions. Export should call the generated
  `ExportTaskThreadMemories` client with a bounded limit, download only the
  backend `coze.task_thread_memories.export.v1` JSON payload, and show bounded
  aggregate status text. Import should use a SideSheet/TextArea, accept only
  that schema, build requests from an explicit memory-field allow-list, drop
  unknown fields before calling `ImportTaskThreadMemories`, refresh the list
  after success, and display imported/skipped counts. The frontend must not
  render or forward prompts, model input/output, tool arguments/results,
  checkpoint bytes, object URIs, raw provider payloads, credentials, URLs,
  filenames, hidden run config, or audit-derived memory content from imported
  JSON. Browser E2E remains later M7 work.
- M7.11 adds backend authorization for Workbench task-thread memory
  management. `ApplicationService` owns a `MemoryAuthorizer` boundary and
  default `ThreadOwnerMemoryAuthorizer` wiring in `InitService`; all memory
  list, export, import, update, delete, clear, restore, and audit application
  methods must authorize before touching `ThreadSVC`. Handler requests must
  pass `workbenchViewerIDFromCtx(ctx)` as `ViewerID`; mutating methods may
  still pass `ActorID` for metadata-only audit identity, but `ActorID` must not
  be treated as an authorization substitute. `ErrMemoryAccessDenied` maps to
  HTTP 403 with bounded `memory access denied` text. Authorization failures
  must not expose memory content, metadata JSON, prompts, model input/output,
  tool arguments/results, checkpoint bytes, object URIs, raw provider
  payloads, credentials, URLs, filenames, hidden run config, or audit-derived
  memory content. Browser E2E remains later M7 work.
- M7.12 adds frontend permission affordances for task-detail `任务记忆`.
  Canonical task-thread detail may derive read-only memory state only when the
  current user ID is known and differs from the task-thread `creator_id`.
  Read-only mode may show a bounded `只读` tag, keep list/search, deleted-row
  viewing, export, and metadata-only audit actions available, and disable or
  handler-block write actions including import, edit, delete, restore, and
  clear. The frontend state is a UX affordance only and must never replace
  backend `MemoryAuthorizer` checks. Read-only UI and errors must not render
  memory metadata JSON, prompts, model input/output, tool arguments/results,
  checkpoint bytes, object URIs, raw provider payloads, credentials, URLs,
  filenames, hidden run config, or audit-derived memory content. Browser E2E
  remains later M7 work.
- M7.13 adds a task-memory E2E selector contract without introducing a new
  browser runner. The `任务记忆` panel may expose stable, content-free
  selectors such as `task-memory-panel`, `task-memory-row`,
  `task-memory-search`, `task-memory-refresh`, `task-memory-export`,
  `task-memory-import`, `task-memory-toggle-deleted`, `task-memory-clear`,
  `task-memory-readonly`, `task-memory-import-sheet`,
  `task-memory-import-input`, `task-memory-import-submit`,
  `task-memory-audit-sheet`, and `task-memory-audit-row`. Row identity may use
  `data-memory-id`; do not add memory content, metadata JSON, source IDs, run
  IDs, object URIs, prompts, model input/output, tool arguments/results,
  checkpoint bytes, raw provider payloads, credentials, URLs, filenames, or
  hidden run config to selector attributes. The repository still has no
  runnable Playwright/Cypress harness for `@coze-studio/app`; true browser
  execution, mobile viewport checks, and CI browser jobs remain production
  acceptance work.
- The following M8 notes are Phase 2 reference material. Do not start new M8
  work during Phase 1 DeerFlow parity execution except to fix regressions from
  already-written code or unblock a Phase 1 test.
- M8.1 starts the Guardrail provider contract in
  `backend/application/agentthread`. `GuardrailRequest`,
  `GuardrailDecision`, and `GuardrailProvider` are metadata-only contracts for
  future Skill, MCP, file, network, command, artifact, and tool-call security
  checks. `ChainGuardrailProvider` merges decisions with deterministic
  precedence: `deny` > `confirm` > `warn` > `allow`. Provider failures use
  request fail mode: `fail_open` returns an allow decision with a bounded
  reason code, while `fail_closed` returns a deny decision. Decision provider,
  reason, rule IDs, user-facing messages, and metadata must be sanitized and
  bounded; do not include prompt text, model input/output, tool
  arguments/results, object URIs, URLs, filenames, credentials, checkpoint
  bytes, raw scanner payloads, or provider raw bodies. M8.1 is contract-only:
  runtime enforcement, human confirmation interrupts, durable security audit
  tables, scanner adapters, UI review, metrics, and OpenTelemetry linkage
  remain follow-up M8 work.
- M8.2 adds durable Guardrail audit persistence. Application code should use
  `ApplicationGuardrailAuditRecorder` to translate a `GuardrailRequest` and
  normalized `GuardrailDecision` into an `agent_guardrail_audit_events` row.
  The record is metadata-only: run/thread/space/actor identity, event type,
  target type, redacted safe target ID, operation, source, action, fail mode,
  provider, reason code, sanitized rule IDs, and timestamp. Do not persist
  decision messages, prompts, model input/output, tool arguments/results,
  object URIs, raw URLs, filenames, credentials, checkpoint bytes, scanner raw
  payloads, or provider raw bodies. Unsafe target IDs must be stored as
  `redacted`, and recorder failures must return generic sanitized errors.
  M8.2 still does not wire runtime enforcement, scanner adapters, human
  confirmation interrupts, UI review, metrics, or OpenTelemetry linkage.
- M8.3 adds `GuardrailEnforcer`, the runtime-facing decision gate around the
  M8.1 provider and M8.2 audit recorder. Callers pass a metadata-only
  `GuardrailRequest`; the gate evaluates the provider, normalizes the
  decision, records the audit event, and maps actions as follows: `allow`
  proceeds, `warn` proceeds with `Warning=true`, `deny` returns
  `GuardrailDeniedError`, and `confirm` returns
  `GuardrailConfirmationRequiredError`. Missing or failing audit recorders
  fail closed with `GuardrailAuditFailedError`. Gate errors must remain
  sanitized and include only bounded error codes/reason codes. M8.3 is still
  a gate contract; Eino tool wrapping, scanner adapters, human confirmation
  checkpoint integration, UI review, metrics, and OpenTelemetry linkage remain
  later M8 slices.
- M8.4 wires Guardrail into ADK runtime catalog tool invocation through
  `ADKGuardrailRuntimeToolCatalog`. Wrap runtime catalogs with
  `NewADKGuardrailRuntimeToolCatalog` or use
  `WithDefaultADKToolProviderGuardrailEnforcer` when building the default ADK
  tool provider. The wrapper must preserve tool name, description, schema, and
  visibility, and must build a metadata-only `GuardrailRequest` with run,
  thread, space, creator identity, target type `tool_call`, target ID as the
  runtime tool name, operation `invoke`, source `adk_runtime_tool`, and
  fail mode `fail_closed`. It must not pass tool arguments, tool results,
  prompts, model text, URLs, filenames, object URIs, credentials, checkpoint
  bytes, scanner raw payloads, or provider raw bodies into Guardrail metadata.
  Only `allow` and `warn` may invoke the original runtime tool. `deny` and
  audit failure must short-circuit before invocation with sanitized typed
  Guardrail errors; `confirm` uses the M8.6 human-interaction interrupt path.
  M8.4 covers runtime catalog tools such as MCP and web tools only; Skill
  middleware, subagent tools, scanner adapters, UI review, metrics, and
  OpenTelemetry linkage remain later M8 slices.
- M8.5 wires Guardrail into ADK Skill content loading. `adkSkillBackend.Get`
  may be configured with `WithADKSkillBackendGuardrail`, and the default Skill
  middleware path passes `ADKMiddlewareAssemblerOptions.GuardrailEnforcer` to
  that backend. The Guardrail request must be metadata-only: run/thread/space
  identity, creator identity, target type `skill`, target ID as the skill
  name, operation `load`, source `adk_skill_backend`, and fail mode
  `fail_closed`. Do not pass skill body, catalog JSON, prompts, model
  input/output, tool arguments/results, object URIs, URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, or provider raw bodies
  to Guardrail metadata or errors. Deny, confirmation-required, and audit
  failure must block before skill content is returned. M8.5 covers content-load
  enforcement only; static Skill scanning, subagent tools, human confirmation
  checkpoint integration, UI review, metrics, and OpenTelemetry linkage remain
  later M8 slices.
- M8.6 converts ADK runtime catalog tool Guardrail `confirm` decisions into an
  Eino `tool.StatefulInterrupt` carrying the existing
  `HumanInteractionPrompt` confirmation schema and `humanInteractionToolState`.
  The prompt must stay metadata-only and may expose only safe runtime tool
  identity, action `invoke`, provider, reason code, sanitized rule IDs, a safe
  summary, and default reject guidance. It must not include tool arguments,
  tool results, prompts, model input/output, URLs, filenames, object URIs,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies, or
  hidden run config. The original runtime tool must not run when confirmation
  interrupts. Full resume approval execution, Skill confirmation interrupts,
  scanner adapters, UI review, metrics, OpenTelemetry linkage, retention, and
  operator runbooks remain later M8 slices.
- M8.7 wires Guardrail into ADK subagent `AgentTool` invocation. Use
  `WithADKSubagentToolProviderGuardrailEnforcer` directly, or rely on
  `NewDefaultADKToolProviderWithSingleAgentSubagents` passing the same default
  Guardrail enforcer to both runtime catalog tools and subagent tools. The
  subagent Guardrail wrapper must sit outside timeout and lifecycle wrappers,
  so `deny`, `confirm`, and audit failures occur before child run rows are
  created or `subagent.run.*` lifecycle events are emitted. Requests must be
  metadata-only: parent run/thread/space/creator identity, target type
  `tool_call`, target ID as the subagent tool name, operation `invoke`, source
  `adk_subagent_tool`, and fail mode `fail_closed`. Do not include subagent
  arguments, child prompts, child model input/output, tool results, URLs,
  filenames, object URIs, credentials, checkpoint bytes, scanner raw payloads,
  provider raw bodies, or hidden run config in Guardrail metadata, prompts, or
  errors. `allow` and `warn` proceed into the original subagent tool path;
  `confirm` returns a metadata-only Eino human-interaction confirmation
  interrupt. Skill confirmation interrupts, scanner adapters, UI review,
  metrics, OpenTelemetry linkage, retention, and operator runbooks remain
  later M8 slices.
- M8.8 completes the Guardrail confirmation resume loop for runtime catalog
  tools and subagent tools. Guardrail wrappers must check saved
  `humanInteractionToolState` before evaluating the provider. Approved
  `HumanInteractionResponse` data invokes the original guarded tool once and
  must bypass a second Guardrail provider/audit evaluation for the same
  interrupted call; rejected responses return sanitized
  `GuardrailConfirmationRejectedError` without invoking the tool. Missing
  resume data or a resume targeted at another interrupt must re-interrupt with
  the saved metadata-only prompt. Subagent approved resumes must enter the
  inner Eino `AgentTool` through a child tool address segment so the inner
  AgentTool does not consume the outer Guardrail interrupt state. Resume
  handling must not include tool arguments, child prompts, model input/output,
  tool results, URLs, filenames, object URIs, credentials, checkpoint bytes,
  scanner raw payloads, provider raw bodies, or hidden run config in errors,
  prompts, or audit metadata. Skill confirmation interrupts, scanner adapters,
  UI review, metrics, OpenTelemetry linkage, retention, and operator runbooks
  remain later M8 slices.
- M8.9 adds the Workbench Guardrail audit query backend boundary at
  `GET /api/workbench/task_threads/:thread_id/guardrail_audit_events`.
  The endpoint accepts optional `run_id`, `page`, and `page_size`, and returns
  only metadata-safe `agent_guardrail_audit_events` fields plus `total`.
  Default authorization must use `ThreadOwnerGuardrailAuditAuthorizer`, so only
  the task thread creator can list that thread's Guardrail audit records.
  The response may include event/run/thread/space/actor identity, event type,
  target type, redacted safe target ID, operation, source, action, fail mode,
  provider, reason code, sanitized rule IDs, and created timestamp. It must not
  expose decision messages, prompts, model input/output, tool
  arguments/results, object URIs, raw URLs, filenames, credentials, checkpoint
  bytes, scanner raw payloads, provider raw bodies, hidden run config, or
  client-controlled identity. Future Guardrail UI and audit exports should
  consume this same metadata-only contract instead of adding parallel read
  paths.
- M8.10 adds the first runtime Guardrail scanner provider:
  `GuardrailPatternProvider`. It is disabled by default and can be enabled
  with `AGENT_GUARDRAIL_PROVIDER_TYPE=pattern` plus
  `AGENT_GUARDRAIL_PATTERN_RULES_JSON`. Rules may match only metadata-safe
  request fields: target type, target ID, operation, source, and explicitly
  supplied metadata string values. They may return only `warn`, `confirm`, or
  `deny`; `allow` remains the default no-match result. Invalid explicit env
  configuration must fail closed through a sanitized
  `guardrail_config_invalid` deny decision. Production ADK wiring should use
  `NewGuardrailEnforcerFromEnv` and pass the resulting enforcer to both
  `WithDefaultADKToolProviderGuardrailEnforcer` and
  `ADKMiddlewareAssemblerOptions.GuardrailEnforcer`, so runtime tools,
  subagent tools, and Skill content loading share the same provider and audit
  path. Pattern rules and decision metadata must not expose prompts, model
  input/output, tool arguments/results, object URIs, raw URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies,
  hidden run config, or client-controlled identity. `GuardrailProviderStatus`
  is diagnostic metadata only.
- M8.11 adds the external HTTP Guardrail scanner provider. It is disabled by
  default and is enabled only through `AGENT_GUARDRAIL_PROVIDER_TYPE=http`,
  `AGENT_GUARDRAIL_HTTP_URL`, optional `AGENT_GUARDRAIL_HTTP_TOKEN`, and
  `AGENT_GUARDRAIL_HTTP_TIMEOUT_MS`. The endpoint must be `http` or `https`
  and must not include URL userinfo; invalid explicit config still fails
  closed through the env-built enforcer. The request schema is
  `coze.guardrail_scan_request.v1` and may include only safe identity
  metadata: space/thread/run/user IDs, target type, sanitized target ID,
  operation, source, fail mode, and sanitized string metadata. It must not send
  prompts, model input/output, tool arguments/results, object URIs, raw URLs,
  filenames, credentials, checkpoint bytes, scanner raw payloads, provider raw
  bodies, hidden run config, or client-controlled authority. HTTP scanner
  responses may return only `warn`, `confirm`, or `deny`; `allow` is not an
  accepted external response action because no-match/default allow remains a
  Coze-owned decision. Response provider, reason, message, rule IDs, and
  metadata must pass through the same Guardrail decision sanitizer before
  audit, interrupts, or user-facing errors.
- M8.12 adds the canonical task-detail Guardrail audit frontend panel. Keep
  the user-facing object named `安全审计` and mount it only on task-thread
  detail pages. The panel may call the generated
  `ListTaskThreadGuardrailAuditEvents` client through
  `frontend/apps/coze-studio/src/pages/tasks/service.ts`, show the latest
  metadata-only events, and refresh manually. Rows may display event type,
  action, target type, safe target ID, operation, source, fail mode, provider,
  reason code, sanitized rule IDs, run ID, and timestamp. It must not render
  decision messages, prompts, model input/output, tool arguments/results,
  object URIs, raw URLs, filenames, credentials, checkpoint bytes, scanner raw
  payloads, provider raw bodies, hidden run config, or raw metadata JSON.
  Missing audit rows should be rendered as an empty state, not as proof that a
  run had no policy evaluation.
- M8.13 adds a frontend-only `安全审计` current-list export. The export button
  may serialize only the audit rows already loaded in the task-detail panel
  using schema `coze.task_thread_guardrail_audit.export.v1`. The exported JSON
  must rebuild each event from the explicit safe field allow-list: event/run/
  thread/space/actor IDs, event type, target type, safe target ID, operation,
  source, action, fail mode, provider, reason code, sanitized rule IDs, and
  created timestamp. It must not include decision messages, prompts, model
  input/output, tool arguments/results, object URIs, raw URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies,
  hidden run config, raw metadata JSON, or fields added by future API versions
  unless those fields are explicitly reviewed and added to the allow-list.
  This is not a full audit export contract; full history export must be a
  separate backend endpoint with authorization, pagination, and retention
  policy.
- M8.14 adds the backend Guardrail audit export boundary at `GET
  /api/workbench/task_threads/:thread_id/guardrail_audit_events/export`.
  The endpoint reuses `ThreadOwnerGuardrailAuditAuthorizer` through
  `GuardrailAuditAccessOperationExport`, accepts optional `run_id`, `page`,
  and `page_size`, caps export page size to 1000, and returns schema
  `coze.task_thread_guardrail_audit.export.v1` with `thread_id`,
  `exported_at`, normalized page metadata, `total`, and metadata-only audit
  events. It must use the same explicit safe event fields as the list/current
  export paths and must not expose decision messages, prompts, model
  input/output, tool arguments/results, object URIs, raw URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies,
  hidden run config, raw metadata JSON, or client-controlled identity.
  Frontend generated clients are available through the task page service.
- M8.15 switches the canonical task-detail `安全审计` export action to the
  backend Guardrail audit export client. The panel still lists the latest 20
  rows for display, but export must call
  `ExportTaskThreadGuardrailAuditEvents` with `page_size=1000`, continue
  requesting pages until the accumulated safe event count reaches backend
  `total` or an empty page is returned, and download a single JSON payload
  using schema `coze.task_thread_guardrail_audit.export.v1`. The downloaded
  payload may keep the existing frontend export shape by parsing `rule_ids`
  into a string array and adding normalized `page` / `page_size` metadata.
  Export loading should use the Semi/Coze Design `Button.loading` prop, and
  empty audit totals should keep the export action disabled. The frontend must
  not send run/thread ownership identity, and must not render or export
  decision messages, prompts, model input/output, tool arguments/results,
  object URIs, raw URLs, filenames, credentials, checkpoint bytes, scanner raw
  payloads, provider raw bodies, hidden run config, raw metadata JSON, or
  unreviewed future API fields. Scheduled archive/export jobs, OpenTelemetry
  exporter wiring, policy UI, and operator runbooks remain later M8 work.
- M8.16 adds backend Guardrail audit retention cleanup. The repository exposes
  bounded batch deletion for `agent_guardrail_audit_events` by `created_at`
  cutoff, and `GuardrailAuditRetentionReaper` computes the cutoff from a
  configured retention duration before deleting one ordered batch at a time.
  The worker is disabled by default and is controlled by
  `AGENT_GUARDRAIL_AUDIT_RETENTION_WORKER_ENABLED`,
  `AGENT_GUARDRAIL_AUDIT_RETENTION_DAYS`,
  `AGENT_GUARDRAIL_AUDIT_RETENTION_BATCH_SIZE`, and
  `AGENT_GUARDRAIL_AUDIT_RETENTION_INTERVAL_MS`; example defaults keep a
  90-day retention window, 1000-row batches, and a 1-hour interval. Cleanup
  hard-deletes metadata-only audit rows older than the cutoff and must not log
  or expose decision messages, prompts, model input/output, tool
  arguments/results, object URIs, raw URLs, filenames, credentials, checkpoint
  bytes, scanner raw payloads, provider raw bodies, hidden run config, raw
  metadata JSON, or raw repository errors. Scheduled archive/export jobs,
  legal-hold policy, OpenTelemetry exporter wiring, policy UI, and operator
  runbooks remain later M8 work.
- M8.17 adds the Guardrail security decision metrics boundary. New runtime
  checks should report through `GuardrailMetricsCollector` rather than logging
  ad hoc values. `GuardrailEvaluationMetricsObservation` may include only
  bounded content-free labels and counters: target type, operation, source,
  fail mode, action, provider, sanitized error code, allowed/warning/
  confirmation flags, `audit_recorded`, and elapsed milliseconds. It must not
  include space/thread/run/user IDs, target IDs, reason codes, rule IDs,
  decision messages, prompts, model input/output, tool arguments/results,
  object URIs, raw URLs, filenames, credentials, checkpoint bytes, scanner raw
  payloads, provider raw bodies, hidden run config, raw metadata JSON, or raw
  repository/provider errors. `AGENT_GUARDRAIL_METRICS_LOG_ENABLED=false` by
  default; enabling it emits content-free per-decision metric logs for
  deployments whose log pipeline can absorb the volume. Prometheus and
  OpenTelemetry exporters should adapt this collector boundary instead of
  adding parallel Guardrail instrumentation.
- M8.18 adds the Guardrail audit retention cleanup metrics and operator
  runbook boundary. `GuardrailAuditRetentionWorker` emits one
  `GuardrailAuditRetentionMetricsObservation` per cleanup attempt when a
  collector is configured. Observations may include only success/failure,
  sanitized error code, cutoff timestamp, deleted row count, configured
  retention days, batch size, and elapsed milliseconds. They must not include
  space/thread/run/user IDs, target IDs, audit row IDs, reason codes, rule
  IDs, decision messages, prompts, model input/output, tool arguments/results,
  object URIs, raw URLs, filenames, credentials, checkpoint bytes, scanner raw
  payloads, provider raw bodies, hidden run config, raw metadata JSON, raw SQL,
  or raw repository errors. `AGENT_GUARDRAIL_AUDIT_RETENTION_METRICS_LOG_ENABLED`
  stays disabled by default; enabling it emits periodic content-free cleanup
  metric logs. Operators should follow
  `docs/superpowers/runbooks/guardrail-audit-operations.md` before enabling
  retention cleanup or metrics logging. Scheduled archive/export jobs,
  legal-hold policy, Prometheus/OpenTelemetry exporters, and policy UI remain
  later M8 work.
- M8.19 adds the Guardrail audit retention legal-hold policy switch.
  `AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED=false` by default.
  When enabled, `GuardrailAuditRetentionWorker.RunOnce` must skip cleanup
  before calling the reaper or repository, emit a content-free skipped
  retention metrics observation with `skip_reason='legal_hold'`, and return an
  empty result. Legal hold must not disable the worker or retention metrics
  collector; this lets operators verify the hold is active without deleting
  rows. The legal-hold path must not log or expose space/thread/run/user IDs,
  target IDs, audit row IDs, reason codes, rule IDs, decision messages,
  prompts, model input/output, tool arguments/results, object URIs, raw URLs,
  filenames, credentials, checkpoint bytes, scanner raw payloads, provider raw
  bodies, hidden run config, raw metadata JSON, raw SQL, raw repository
  errors, or archive object locations. Future archive/export jobs must honor
  this policy before any destructive cleanup.
- M8.20 adds the non-destructive Guardrail audit archive/export boundary.
  `GuardrailAuditRepository.ListGuardrailAuditEventsBefore` pages
  `agent_guardrail_audit_events` rows by `created_at` cutoff with stable
  `created_at ASC, id ASC` ordering and returns the cutoff total. The
  application-level `GuardrailAuditArchiveExporter` reads one bounded batch,
  builds schema `coze.guardrail_audit.archive.v1`, and delegates persistence
  to a `GuardrailAuditArchiveWriter`. This slice must not delete rows, start a
  scheduler, bypass legal hold, or know object-storage implementation details.
  Archive payloads may include only the same metadata-safe audit row fields as
  Workbench export: event/run/thread/space/actor IDs, event type, target type,
  safe target ID, operation, source, action, fail mode, provider, reason code,
  sanitized rule IDs, and created timestamp. They must not include decision
  messages, prompts, model input/output, tool arguments/results, object URIs,
  raw URLs, filenames, credentials, checkpoint bytes, scanner raw payloads,
  provider raw bodies, hidden run config, raw metadata JSON, raw SQL, raw
  repository/provider errors, client-controlled identity, or unreviewed future
  fields. Future scheduled archive jobs should call this boundary before
  destructive cleanup and must honor legal hold.
- M8.21 adds the concrete Guardrail audit object-storage archive writer.
  `GuardrailAuditObjectStorageArchiveWriter` serializes only the
  `coze.guardrail_audit.archive.v1` allow-list into snake_case JSON and writes
  it through the injected Coze object storage interface under an internal
  archive prefix. The writer returns only a stable identifier `ArchiveID`; it
  must not return, log, persist through public contracts, or surface object
  keys, object URIs, raw URLs, filenames, credentials, storage provider
  errors, or scanner/provider payloads. Storage write and validation failures
  must collapse to `guardrail audit archive write failed`. This slice still
  must not start a scheduler, delete rows, bypass legal hold, introduce a
  storage lifecycle policy, or expose archive objects through Workbench APIs.
- M8.22 adds the scheduled Guardrail audit archive worker. It is disabled by
  default behind `AGENT_GUARDRAIL_AUDIT_ARCHIVE_WORKER_ENABLED` and computes
  the archive cutoff from `AGENT_GUARDRAIL_AUDIT_ARCHIVE_RETENTION_DAYS`.
  The worker calls `GuardrailAuditArchiveExporter` with a bounded
  `AGENT_GUARDRAIL_AUDIT_ARCHIVE_BATCH_SIZE`, writes through the M8.21
  object-storage writer, and records optional content-free archive metrics
  behind `AGENT_GUARDRAIL_AUDIT_ARCHIVE_METRICS_LOG_ENABLED`. It must honor
  the shared `AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED` switch by
  skipping archive attempts and emitting only `skip_reason='legal_hold'`.
  Archive worker logs and metrics may include success, sanitized error code,
  skip reason, cutoff timestamp, archived row count, total, stable archive ID,
  retention days, batch size, and elapsed milliseconds only. They must not
  include object keys, object URIs, raw URLs, filenames, credentials, prompts,
  model input/output, tool arguments/results, checkpoint bytes, scanner raw
  payloads, provider raw bodies, hidden run config, raw metadata JSON, raw SQL,
  raw repository/provider/storage errors, or audit row payload content. This
  slice is still non-destructive: it must not delete audit rows, coordinate
  deletion-after-archive, expose archive objects, or configure storage
  lifecycle policy.
- M8.23 adds opt-in Guardrail audit archive-before-delete orchestration for
  retention cleanup. Set
  `AGENT_GUARDRAIL_AUDIT_RETENTION_REQUIRE_ARCHIVE_ENABLED=true` together
  with the retention worker to require a successful archive before deletion.
  When enabled, retention startup must require object storage, build a
  `GuardrailAuditArchiveBeforeDeleteCleaner`, archive one bounded batch through
  `GuardrailAuditArchiveExporter`, validate the archive cutoff and archived
  event IDs, and delete only those exact archived event IDs through
  `DeleteGuardrailAuditEventsByIDs`. Do not delete by a fresh cutoff query
  after archiving, because concurrent inserts could otherwise cause
  unarchived rows to be deleted. Archive failures, mismatched cutoff values,
  missing/invalid archived event IDs, and delete errors must fail closed as
  `guardrail audit retention cleanup failed` and must not expose archive IDs,
  object keys, object URIs, raw URLs, filenames, credentials, prompts, model
  input/output, tool arguments/results, checkpoint bytes, scanner raw
  payloads, provider raw bodies, hidden run config, raw metadata JSON, raw SQL,
  raw repository/provider/storage errors, or audit row payload content. The
  existing legal-hold switch still skips the retention worker before this
  cleaner runs. Retention metrics may include stable archive IDs and archived
  row counts, but not archived event IDs or object locations.
- M8.24 adds the Guardrail Prometheus metrics collector/exporter boundary.
  `GuardrailPrometheusMetricsCollector` implements the decision, archive, and
  retention metrics collector interfaces and is enabled by
  `AGENT_GUARDRAIL_PROMETHEUS_METRICS_ENABLED`. Existing logging metrics env
  flags may be combined with this Prometheus collector through mux collectors.
  Prometheus metrics may expose only low-cardinality metadata labels such as
  action, provider, error code, success, skipped, skip reason, and row kind,
  plus aggregate counters and latency histograms. They must not label or emit
  archive IDs, archived event IDs, thread/run/space/user IDs, target IDs,
  object keys, object URIs, raw URLs, filenames, credentials, prompts, model
  input/output, tool arguments/results, checkpoint bytes, scanner raw payloads,
  provider raw bodies, hidden run config, raw metadata JSON, raw SQL, raw
  repository/provider/storage errors, or audit row payload content. This slice
  registers collectors only; exposing an HTTP scrape endpoint remains platform
  infra work and must be protected separately.
- Eino middleware ordering is part of runtime behavior. Add contract tests for
  the configured handler order, especially summarization, reduction,
  `AGENTS.md`, memory, Skill, dynamic tool search, ToolCall repair, policy,
  audit, and usage handlers.
- Use Eino `dynamictool/toolsearch` for large tool catalogs. The Coze Tool
  Registry remains responsible for durable discovery, authorization, policy,
  and producing the initial `tool.BaseTool` set. Model-visible tools and
  executable tools must come from the same policy result.
- Prefer `ADKToolSetProvider` when wiring runtime tools into ADK. It resolves
  static and deferred tools in one call so the visible tool list,
  provider-native deferred list, client-side `tool_search` catalog, and
  executable tools all share one Coze policy snapshot.
- Use `ADKRuntimeToolCatalogProvider` only as an adapter from a Coze-owned,
  policy-filtered runtime catalog to Eino `tool.BaseTool` values. It may
  partition tools as `static` or `deferred`, parse JSON Schema input schemas,
  and delegate invocation to a Coze invoker; it must not own MCP transport,
  OAuth, encrypted secrets, session lifecycle, sandboxing, health checks,
  policy, audit, timeout, or output-budget decisions.
- Web tools must use the Coze-owned `ADKWebToolCatalog` boundary before they
  become Eino tools. Keep them disabled by default and enable them only through
  explicit `web_tools` / `webTools` run config or future durable Tool Registry
  policy.
- Do not expose Eino-ext `httprequest` directly to the model. The available
  module accepts arbitrary URLs and reads full response bodies; Coze must own
  allowed-host policy, private-IP rejection, redirect checks, response-byte
  budgets, timeout bounds, audit, and sanitized errors.
- `web_fetch` is GET-only in M2.18. It requires `allowed_hosts`, permits HTTPS
  by default, permits HTTP only with `allow_http`, rejects private IP literals
  unless `allow_private_ips` is explicitly enabled, and returns bounded
  `coze.web_fetch.v1` JSON. Do not put raw URLs, request bodies, credentials,
  object keys, or response bodies in run events or errors.
- `web_search` must call an injected Coze-owned `ADKWebSearchBackend` and
  clamp result counts to `1-10`. DuckDuckGo and Google Eino-ext adapters are
  backend candidates only after network policy, retry behavior, secret
  storage, masked settings, and provider audit pass review.
- The production-shaped HTTP search adapter is
  `NewADKHTTPWebSearchBackend`, wired only when
  `AGENT_THREAD_WEB_SEARCH_ENABLED=true`. It posts bounded JSON to a fixed
  endpoint, defaults to HTTPS, rejects private IP literals and cross-host
  redirects unless explicitly configured for local/internal deployments, and
  must not expose queries, endpoint paths, API keys, provider response bodies,
  raw URLs, object keys, or credentials in errors or events. Durable Web
  provider settings and UI remain future work.
- Use a Coze-backed `plantask.Backend` for durable task-plan state. Do not make
  DeepAgent `write_todos` and `plantask` independent systems of record; disable
  one when both would otherwise be exposed.
- Persist Eino plan tasks in `agent_run_plans` and
  `agent_run_plan_items`; the Eino `.highwatermark` and `{id}.json` files are
  adapter protocol only. Reserve task IDs with database compare-and-swap, and
  archive completed/deleted task files instead of physically deleting history.
- Fresh runs own a new plan scope. A resumed run sets the internal
  `RunSummary.PlanScopeRunID` to the source run so the Agent continues the same
  plan, while checkpoints, usage, cancellation, and `plan.task.*` events remain
  attributed to the active run.
- Keep the product navigation and business object named “任务”. `plantask`
  items are execution-plan steps inside a run, not top-level task records.
- Eino's `adk/middlewares/skill` is the preferred `SKILL.md` runtime. It
  supports progressive discovery plus `inline`, `fork`, and
  `fork_with_context` execution. It does not replace Coze-owned Skill CRUD,
  versioning, publication, permissions, allowed-tool policy, scanning,
  quarantine, sandboxing, audit, or UI.
- The skill configuration list uses the existing `UpdateSkill` Workbench API
  for M5.1 enable/disable management. The frontend sends a complete current
  skill snapshot with only `enabled` inverted and reloads the list after
  success; do not add a temporary partial-update API unless the backend
  contract is intentionally introduced with validation, authorization, audit,
  and tests.
- The skill configuration page uses the existing `CreateSkill` Workbench API
  for M5.2 basic custom Skill creation. `创建技能` must remain a create action,
  while `导入技能` owns file/content import. The frontend sends an enabled
  `CustomSkill` with trimmed name/description, version `1.0.0`, `{}` input
  and output schemas, `{}` executor, and
  `{"network":false,"allowed_tools":[]}` permissions, then reloads the list.
  Use a runtime `workbenchSkill` import when constructing this payload; a
  type-only import will erase `SkillType.CustomSkill` at runtime. Advanced
  SKILL.md editing, metadata editing, tool grants, resource mounts, scanner,
  quarantine, and execution policy remain separate Coze-owned M5 work.
- The Skill version management drawer uses the existing `UpdateSkill`
  Workbench API for M5.3 base metadata edits. The `基础信息` editor may update
  name, description, and version only; it must send a complete current Skill
  snapshot, preserve type, enabled state, schemas, executor, and permissions,
  reuse the drawer notice/error state, and refresh the parent list after
  success. Do not mutate historical SkillVersion rows from this form, and do
  not introduce a partial metadata endpoint without backend validation,
  authorization, audit, and tests.
- The skill list category filters use the existing `workbenchSkill.SkillType`
  enum for M5.4. Keep `全部技能`, `自定义`, `公共`, `内置`, `脚本`, and `工作流`
  mapped to all, `CustomSkill`, `PublicSkill`, `DeerSkill`, `Script`, and
  `Workflow` respectively. `内置` means bootstrap DeerFlow-style Skills in the
  current schema. This is a client-side filter over loaded `ListSkills` rows;
  do not add a second category model or server-side query behavior unless the
  API and persistence contract are intentionally expanded.
- Skill deletion is a soft-delete lifecycle in M5.5. `DeleteSkill` must update
  `skills.deleted_at`, disable the row, and preserve `skill_versions` and
  `skill_resources`; it must not hard-delete rows or storage content. Normal
  `List`, `Get`, `Update`, `TestRun`, version, resource, export, edit, and
  rollback paths should treat deleted Skills as not found unless a future
  explicit restore/audit API is introduced with authorization, retention, and
  audit semantics. The frontend delete action must confirm with the user and
  send only `skill_id` to DELETE.
- Skill permission editing in M5.6 reuses the full-snapshot `UpdateSkill`
  Workbench API. The version drawer may edit only `permissions.network` and
  `permissions.allowed_tools`, must preserve unknown permission keys, and must
  validate allowed tool names as Eino-safe
  `[A-Za-z_][A-Za-z0-9_]{0,63}` before saving. Do not add a parallel
  permission shape, partial permission endpoint, or Tool Registry picker
  without backend policy, authorization, audit, and tests.
- Skill tool candidates in M5.7 are exposed through
  `GET /api/workbench/skills/tool_candidates?space_id=...` as safe metadata
  only: name, display name, description, category, and visibility. The static
  seed list may include current ADK built-ins such as `web_fetch`,
  `web_search`, `ask_user_clarification`, and
  `request_human_confirmation`, but the API must not expose input schemas,
  endpoint URLs, OAuth/secrets, raw MCP configuration, health payloads,
  prompts, model text, tool arguments/results, object keys, or provider raw
  bodies. Future Tool Registry or MCP-backed candidates must preserve this
  metadata-only, policy-filtered contract.
- Skill tool candidates in M5.8 may also include source metadata:
  `source`, `source_id`, and `source_name`. The Skill application remains the
  final normalization boundary: candidate names must be Eino-safe
  `[A-Za-z_][A-Za-z0-9_]{0,63}`, descriptions must be non-empty, metadata is
  trimmed/bounded, and duplicate names are dropped with earlier sources
  winning. Built-in candidates should stay first. The current Workbench MCP
  provider maps enabled MCP server tools to stable grant names
  `mcp_{server_id}_{sanitized_tool_name}` and skips disabled servers or tools
  without descriptions. Do not expose raw MCP tool names as saved grants unless
  the Coze-owned naming adapter intentionally produces that exact value. Do
  not expose MCP config, auth, input schema, endpoints, URLs, object keys,
  health raw payloads, tool arguments/results, secrets, prompt/model text, or
  provider raw bodies through the candidate API or Skill permission UI.
- Use the Eino MCP tool adapter for MCP tool conversion and invocation. It does
  not replace Coze-owned durable MCP configuration, OAuth, encrypted secrets,
  transport/session lifecycle, health checks, stdio sandboxing, authorization,
  policy, audit, timeout, or output-budget enforcement.
- Workbench MCP tool configuration is durable starting in M6.1. The production
  application bootstrap must use `mcptool.NewMySQLCatalog(DB)`, while
  `NewInMemoryCatalog()` is for tests, fixtures, and temporary harnesses only.
  MCP server rows live in `mcp_tool_servers` with JSON `config`, `auth`, and
  `tools`, plus `deleted_at` for a future soft-delete lifecycle. Do not treat
  this table as a complete Tool Registry: encrypted secret storage, masked
  round trips, normalized tool indexing, health checks, OAuth/session
  lifecycle, stdio sandboxing, authorization, audit, timeouts, output budgets,
  and Eino MCP invocation remain separate Coze-owned layers. `UpsertServer`
  must reject updates where an existing `server_id` belongs to a different
  `space_id`; storage catalogs persist rows but do not own tenant policy.
- Workbench MCP auth responses are masked starting in M6.2. Keep raw auth JSON
  inside the catalog/storage boundary only; create, list, get, and delete
  responses must replace sensitive keys with `********`. If an update submits
  masked auth for an existing server, preserve those fields from the stored
  auth JSON instead of overwriting credentials with the mask. Blank auth on an
  existing update also preserves stored auth. This masking is not encryption:
  encrypted-at-rest storage, KMS/secret manager integration, masked edit
  workflows, audit, and authorization remain Coze-owned follow-up work.
- Workbench MCP deletion is a soft-delete lifecycle in M6.2. The public API is
  `DELETE /api/workbench/mcp_tools/:server_id`; default list/detail/test-call
  and Skill candidate paths must not return deleted rows. `MySQLCatalog`
  updates `mcp_tool_servers.deleted_at`, while `InMemoryCatalog` may remove
  rows because it is only a test/harness implementation. Do not add restore,
  hard-delete cleanup, or deleted-only browsing without explicit authorization,
  retention, and audit design.
- Workbench MCP health metadata starts in M6.3. MCP server responses may expose
  only `health_status`, `health_checked_at`, `health_latency_ms`, and bounded
  sanitized `health_error`. Successful Workbench test calls may set
  `health_status='healthy'` with checked time and latency. Health metadata
  must never include test arguments, tool output, config, auth, URLs, filenames,
  object keys, provider raw responses, stack traces, prompts, model text, or
  checkpoint bytes.
- MCP Tool Registry indexing starts as a metadata-only boundary at
  `GET /api/workbench/mcp_tools/registry_entries?space_id=...`. It returns
  enabled, non-deleted MCP tools only, with stable registry names generated by
  the same `mcp_{server_id}_{sanitized_tool_name}` adapter used for Skill
  grants. Registry entries may include source/category/visibility, server ID
  and name, raw configured MCP tool name, description, enabled state, and health
  metadata. They must not expose MCP config, auth, input schema, endpoint URLs,
  object keys, tool arguments/results, health raw payloads, provider request or
  response bodies, prompt/model/checkpoint/transcript content, or credentials.
  Future runtime Eino MCP adapters, audit records, model-visible tools, UI
  grants, and executable tool names must reuse this naming boundary so policy
  checks cannot drift.
- MCP runtime adapter wiring starts in M6.4 as a non-executable ADK runtime
  catalog boundary. `ADKMCPRuntimeToolCatalog` reads only safe registry entries
  through `ADKMCPToolRegistry` and is disabled unless run config explicitly
  sets `mcp_tools.enabled` / `mcpTools.enabled`. Generated runtime tool
  definitions default to deferred visibility, may be narrowed by
  `allowed_tools` / `allowedTools`, and are still filtered by the outer
  `tool_policy` provider. They must not include MCP config, auth, input schema,
  URLs, filenames, object keys, tool arguments/results, prompt/model text,
  transcripts, checkpoints, or provider raw payloads. Invocation must fail
  closed until a Coze-owned MCP transport/session executor is wired behind
  authorization, OAuth/secret handling, stdio sandboxing, audit, timeout, and
  output-budget enforcement. Production ADK bootstrap may inject
  `primaryServices.mcpToolSVC` through
  `WithDefaultADKToolProviderMCPRegistry`; child SingleAgent factories must not
  inherit parent MCP registry access implicitly.
- MCP runtime executor contract starts in M6.5. `ADKMCPRuntimeToolExecutor`
  receives only internal execution metadata: active run summary, safe
  model-visible MCP tool name, durable MCP server ID, raw configured MCP tool
  name, and arguments JSON. Registry entries without server ID, raw tool name,
  Eino-safe registry name, or non-empty description must not become runtime
  tools. No executor is installed by default; invocation without one must fail
  closed. If an executor returns an error, the model-facing error may include
  only the safe tool name and must not include tool arguments, server names,
  config, auth, URLs, filenames, object keys, provider diagnostics, prompt or
  model text, transcripts, checkpoints, or secret-adjacent data. Production
  bootstrap may inject only the M6.18 dry-run MCP executor through explicit
  environment configuration; do not inject a real MCP executor until Coze-owned
  transport/session, authorization, OAuth/secret, stdio sandbox, audit,
  timeout, output-budget, and health gates are in place.
- MCP runtime service boundary starts in M6.6. `ADKMCPRuntimeExecutor` is the
  Coze-owned service behind `ADKMCPRuntimeToolExecutor`; it resolves durable MCP
  server rows, validates active run space, server ID, raw configured tool name,
  JSON object arguments, server ownership, enabled state, and configured tool
  presence before any transport call. `mcptool.ApplicationService.
  ResolveADKMCPRuntimeServer` is an internal resolver and may return raw
  server `config`, `auth`, and tool definitions to the runtime transport layer;
  never expose that method directly through Workbench HTTP responses. Workbench
  create/list/get/delete responses must remain masked. The runtime executor
  wraps transport with timeout and output-byte limits, rejects oversized output
  without truncating or offloading in this slice, and emits only content-free
  `mcp.tool.started`, `mcp.tool.completed`, and `mcp.tool.failed` lifecycle
  events. Runtime errors and events must not include MCP arguments, server
  names, config, auth, endpoint URLs, filenames, object keys, tool output,
  provider diagnostics, prompt/model text, transcripts, checkpoint bytes, or
  secret-adjacent data. Real stdio/SSE/HTTP transport, Eino MCP conversion,
  OAuth/secrets, sandboxing, audit persistence, and health history remain
  separate follow-up work. Oversized runtime output is handled by the M6.24
  executor-level offload hook, not by resolver or transport adapters.
- MCP runtime transport routing starts in M6.7. `ADKMCPRuntimeTransportRouter`
  implements `ADKMCPRuntimeTransportInvoker` and selects only explicit handler
  slots for `stdio`, `sse`, and `streamable_http` MCP server types; `http` and
  `streamable-http` normalize to the streamable HTTP slot. With no handler
  installed, every transport fails closed with a fixed sanitized error such as
  `mcp runtime stdio transport is disabled`; unsupported or missing server
  types fail with `unsupported mcp runtime transport`. The router must not
  parse config/auth, spawn local commands, open network clients, inspect or
  transform tool arguments, emit events, or own timeout/output budgets.
  `ADKMCPRuntimeExecutor` continues to own validation, timeout, byte budget,
  and content-free lifecycle events. Do not install concrete stdio/SSE/HTTP
  handlers in production until authorization, OAuth/secret retrieval, session
  lifecycle, stdio sandboxing, network policy, audit, M6.22 health reporter
  semantics, Eino MCP adapter conversion, and M6.24 output-offload behavior
  pass review.
  Router errors and composed executor errors must not leak arguments, server
  names, config, auth, endpoint URLs, command paths, filenames, object keys,
  provider diagnostics, prompt/model text, transcripts, checkpoints, or
  secret-adjacent data.
- MCP stdio sandbox boundary starts in M6.8. `ADKMCPRuntimeStdioTransport`
  implements `ADKMCPRuntimeTransportInvoker` for the router `Stdio` slot, but
  it still must not execute host commands directly. It parses only bounded
  stdio config JSON with `command`, optional string-array `args`, optional
  string-map `env`, and optional `cwd` / `working_dir` / `workingDir`; default
  config budget is 16 KiB. Missing server, wrong server type, invalid JSON,
  empty command, non-string args/env values, or oversized config fail with the
  fixed error `mcp runtime stdio config is invalid`. Execution requires both
  `ADKMCPRuntimeStdioPolicy` and `ADKMCPRuntimeStdioSandbox`; missing policy or
  sandbox fails closed, policy denial becomes `mcp runtime stdio policy
  denied`, and sandbox failure becomes `mcp runtime stdio transport failed`.
  The sandbox call may carry active run, safe runtime tool name, durable server
  ID, raw configured MCP tool name, model arguments JSON, and parsed command
  config, but this slice intentionally does not pass server display name or raw
  auth. Stdio `auth_env` projection is added later in M6.25; OAuth lifecycle,
  encrypted credential retrieval, process limits, session lifecycle, Eino MCP
  adapter invocation, audit, and production bootstrap wiring remain future
  work. Output offload is owned by the outer M6.24 executor hook, not by the
  stdio transport. Health updates are owned by the outer M6.22 executor
  reporter, not by the stdio transport.
  Stdio transport errors must not leak tool arguments, server
  names, config/auth, command paths or args, env values, working directories,
  URLs, filenames, object keys, provider diagnostics, prompt/model text,
  transcripts, checkpoint bytes, or secret-adjacent data.
- MCP stdio static policy starts in M6.9. `ADKMCPRuntimeStdioStaticPolicy`
  implements `ADKMCPRuntimeStdioPolicy` and is the first reusable stdio gate
  before any sandbox execution. Empty options are intentionally unusable and
  deny all commands. Configure exact `AllowedCommands`, absolute
  `AllowedWorkingDirPrefixes`, `AllowedEnvKeys`, and explicit `MaxArgs`,
  `MaxArgBytes`, `MaxEnvVars`, and `MaxEnvValueBytes` budgets before mounting
  it. Zero budgets mean zero allowed items or bytes, so production bootstrap
  must set deliberate values. `RequireWorkingDir` forces a non-empty absolute
  workdir under an allowed prefix. Workdir containment uses cleaned paths and
  relative-prefix checks, so sibling prefixes such as `/mnt/coze/mcp-secret`
  must not match `/mnt/coze/mcp`. Env key allow-list checks run before env
  count checks for stable policy classification. Static policy errors are
  fixed and sanitized: command not allowed, workdir required/not allowed, args
  budget, env not allowed, and env budget. They must not include model
  arguments, command names/args, working directories, env keys/values, server
  names, raw config/auth, URLs, object keys, provider diagnostics, prompt/model
  text, transcripts, checkpoint bytes, or secret-adjacent data. This policy
  does not create directories, project secrets, execute commands, invoke Eino
  MCP adapters, write audit records, or update health.
- MCP stdio sandbox runner contract starts in M6.10.
  `ADKMCPRuntimeStdioSandboxAdapter` implements `ADKMCPRuntimeStdioSandbox`,
  but still must not execute local commands. It validates sandbox calls
  defensively before runner delegation: run/run ID/thread ID/space ID must be
  present, server ID must be positive, runtime tool name must be Eino-safe,
  raw MCP tool name must be non-empty, arguments must be a JSON object, command
  must be non-empty, and working directory must be absolute. Invalid calls fail
  with `mcp runtime stdio sandbox call is invalid`. Missing runner fails with
  `mcp runtime stdio runner is not configured`; runner errors become
  `mcp runtime stdio runner failed`. The adapter projects to
  `ADKMCPRuntimeStdioSandboxExecution`, cloning args/env and cleaning workdir,
  then delegates to injected `ADKMCPRuntimeStdioSandboxRunner`. It does not
  create directories, project secrets, invoke Eino MCP adapters, execute
  commands, write audit records, update health, or offload output. Sandbox
  adapter errors must not leak model arguments, command names/args, env
  keys/values, workdirs, server names, raw config/auth, URLs, object keys,
  provider diagnostics, prompt/model text, transcripts, checkpoint bytes, or
  secret-adjacent data.
- MCP stdio workdir projection starts in M6.11.
  `ADKMCPRuntimeStdioWorkdirManager` implements
  `ADKMCPRuntimeStdioWorkdirProjector` and derives deterministic Coze-owned
  absolute paths only; it must not create directories, touch filesystem state,
  resolve symlinks, mount filesystems, project secrets, execute commands,
  invoke Eino MCP adapters, write audit records, or update health. Root must
  be absolute. A valid request requires run/run ID/thread ID/space ID, positive
  server ID, Eino-safe runtime tool name, and non-empty raw MCP tool name. The
  generated path is
  `{root}/spaces/{space_id}/threads/{thread_id}/runs/{run_id}/servers/{server_id}/tools/{safe_runtime_tool_name}`.
  The manager must re-check that the cleaned path remains under root and fail
  closed with `mcp runtime stdio workdir is invalid` otherwise.
  `ADKMCPRuntimeStdioTransportOptions.WorkdirManager` is optional; when set,
  transport overwrites parsed server `cwd` with the projected Coze-owned path
  before static policy validation. Workdir errors must not leak model
  arguments, command names/args, env keys/values, configured cwd, projected
  paths, root paths, server names, raw config/auth, URLs, object keys, provider
  diagnostics, prompt/model text, transcripts, checkpoint bytes, or
  secret-adjacent data.
- MCP stdio filesystem workdir preparation starts in M6.12.
  `ADKMCPRuntimeStdioWorkdirPreparer` owns prepare/cleanup for projected
  workdirs. `ADKMCPRuntimeStdioFilesystemWorkdirPreparer` may create, chmod,
  stat, and remove directories, but only after validating an absolute root,
  an absolute workdir under that root, and that the workdir is not the root
  itself. It must return fixed sanitized errors:
  `mcp runtime stdio workdir prepare failed` and
  `mcp runtime stdio workdir cleanup failed`. `ADKMCPRuntimeStdioSandboxOptions.
  WorkdirPreparer` is optional. When configured, the sandbox adapter prepares
  before runner delegation and attempts cleanup after runner success or runner
  failure. Runner failure remains `mcp runtime stdio runner failed`; successful
  runner plus cleanup failure becomes `mcp runtime stdio workdir cleanup
  failed`. This boundary still must not execute commands, invoke Eino MCP
  adapters, project secrets, enforce process/session limits, write audit
  records, persist leases, update health, or offload output. Workdir preparer
  errors must not leak model arguments, command names/args, env keys/values,
  workdir paths, root paths, server names, raw config/auth, URLs, object keys,
  provider diagnostics, prompt/model text, transcripts, checkpoint bytes, or
  secret-adjacent data.
- MCP stdio dry-run runner starts in M6.13.
  `ADKMCPRuntimeStdioDryRunRunner` implements
  `ADKMCPRuntimeStdioSandboxRunner` only as a non-executing smoke boundary.
  It validates projected sandbox execution, then returns bounded
  `coze.mcp_stdio_dry_run.v1` JSON with safe metadata only: schema, dry-run
  status, Eino-safe runtime tool name, durable server ID, command/workdir
  presence booleans, args count, and env count. Invalid input fails with
  `mcp runtime stdio dry-run execution is invalid`; oversized dry-run output
  fails with `mcp runtime stdio dry-run output exceeds budget`. This runner
  must not call `os/exec`, open MCP sessions, invoke Eino MCP adapters,
  project secrets, enforce process/session limits, write audit records, update
  health, or offload output. It is suitable for tests and temporary smoke
  wiring only, not as production MCP execution. Dry-run output and errors must
  not leak raw MCP tool names, model arguments, command names/args, env
  keys/values, workdir/root paths, server names, raw config/auth, URLs, object
  keys, provider diagnostics, prompt/model text, transcripts, checkpoint
  bytes, or secret-adjacent data.
- MCP stdio workdir lease persistence starts in M6.14.
  `agent_mcp_stdio_workdir_leases` is the durable internal data boundary for
  prepared stdio workdirs. `MCPRuntimeWorkdirLeaseRepository` can create a
  lease, read it by ID, finish an active lease as `released` or `failed`, and
  list expired active leases for future recovery. Finishing uses active-status
  compare-and-set and, when a worker ID is provided, worker ownership matching
  so stale cleanup owners cannot close another owner's lease. Lease rows may
  persist internal workdir paths for future cleanup, but these paths must not
  be exposed through public Workbench/LangGraph APIs or content-bearing run
  events. The repository bounds `last_error` length only; callers must pass
  sanitized error text. This slice does not wire leases into
  `ADKMCPRuntimeStdioSandboxAdapter`, does not run cleanup workers, does not
  execute commands, and does not invoke Eino MCP adapters. Future lease-aware
  preparers and recovery workers must not persist or expose model arguments,
  raw MCP tool names, command names/args, env keys/values, raw config/auth,
  provider payloads, prompt/model text, transcripts, checkpoint bytes, object
  URIs, URLs, or filenames.
- MCP stdio leased workdir preparation starts in M6.15.
  `ADKMCPRuntimeStdioLeasedWorkdirPreparer` wraps an inner
  `ADKMCPRuntimeStdioWorkdirPreparer` plus an
  `ADKMCPRuntimeStdioWorkdirLeaseStore` while preserving the existing sandbox
  `WorkdirPreparer` interface. Prepare delegates to the inner preparer,
  creates an active lease, attaches lease ID and worker ID to
  `ADKMCPRuntimeStdioPreparedWorkdir`, and attempts inner cleanup if lease
  creation fails. Cleanup delegates to the inner cleanup and then finishes the
  lease as `released` on success or `failed` with sanitized `cleanup failed`
  on cleanup failure. Missing inner preparer, missing lease store, invalid
  lease identity, lease create failure, cleanup failure, and lease finish
  failure must return fixed sanitized errors. This wrapper still does not
  provide the concrete MySQL lease-store adapter, production bootstrap wiring,
  stale cleanup worker, command execution, or Eino MCP adapter invocation.
  Wrapper errors and lease error text must not include model arguments,
  command names/args, env keys/values, workdir/root paths, raw MCP tool names,
  server names, raw config/auth, URLs, object keys, provider diagnostics,
  prompt/model text, transcripts, checkpoint bytes, or secret-adjacent data.
- MCP stdio workdir lease store adapter starts in M6.16.
  `ApplicationADKMCPRuntimeStdioWorkdirLeaseStore` implements
  `ADKMCPRuntimeStdioWorkdirLeaseStore` using the durable
  `MCPRuntimeWorkdirLeaseRepository`, `idgen.IDGenerator`, configured worker
  ID, and configured lease TTL. The default lease TTL is 300000 ms. Create
  validates repository, ID generator, worker ID, positive TTL, run/thread/space
  identity, server ID, Eino-safe runtime tool name, and absolute prepared
  workdir before creating an active lease row with `lease_expires_at = now +
  ttl`. Finish maps application `released` / `failed` to domain statuses,
  uses the lease worker ID with configured worker fallback, and requires the
  repository active-row compare-and-set to succeed. Repository or ID generator
  errors, invalid input, and zero-row finish updates must return fixed
  sanitized errors: `mcp runtime stdio workdir lease create failed` or
  `mcp runtime stdio workdir lease finish failed`. This adapter still does not
  wire production bootstrap, run cleanup workers, execute commands, invoke
  Eino MCP adapters, or expose lease rows through public APIs. Adapter errors
  must not include generated IDs, workdir paths, raw repository errors, model
  arguments, raw MCP tool names, command names/args, env keys/values, raw
  config/auth, URLs, object keys, provider diagnostics, prompt/model text,
  transcripts, checkpoint bytes, or secret-adjacent data.
- MCP stdio dry-run transport composition starts in M6.17.
  `NewADKMCPRuntimeStdioDryRunTransport` is the explicit safe-smoke builder
  for stdio MCP runtime handling. It composes workdir projection, static
  policy, filesystem workdir preparation, durable lease-store adapter, leased
  preparer, dry-run runner, sandbox, and stdio transport. Callers must provide
  the workdir root, lease repository, ID generator, worker ID, command
  allow-list, env allow-list, and policy budgets deliberately; missing or
  unusable dependencies fail closed at the existing boundaries. The builder
  must still use `ADKMCPRuntimeStdioDryRunRunner`, not `os/exec` or a real
  Eino MCP adapter. It does not auto-enable stdio in production bootstrap,
  project secrets, own OAuth/session lifecycle, run cleanup workers, emit new
  public API data, or expose workdir lease rows to users. Dry-run transport
  output and errors must preserve all prior redaction rules for model
  arguments, command names/args, env keys/values, workdir/root paths, raw MCP
  tool names, server names, raw config/auth, URLs, object keys, provider
  diagnostics, prompt/model text, transcripts, checkpoint bytes, and
  secret-adjacent data.
- MCP runtime bootstrap env wiring starts in M6.18.
  `ADKMCPRuntimeBootstrapConfigFromEnv` parses the production opt-in contract
  for MCP runtime execution. Runtime tools still fail closed by default:
  `AGENT_THREAD_MCP_RUNTIME_ENABLED` must be true, and stdio must explicitly
  select exactly one concrete mode: dry-run through
  `AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED=true` or real Eino execution through
  `AGENT_THREAD_MCP_STDIO_EINO_ENABLED=true`. These two mode flags are
  mutually exclusive. Enabled stdio requires an absolute
  `AGENT_THREAD_MCP_STDIO_WORKDIR_ROOT`, non-empty
  `AGENT_THREAD_MCP_STDIO_WORKER_ID`, and non-empty
  `AGENT_THREAD_MCP_STDIO_ALLOWED_COMMANDS`; optional policy and runtime
  budgets are bounded by `AGENT_THREAD_MCP_STDIO_MAX_ARGS`,
  `AGENT_THREAD_MCP_STDIO_MAX_ARG_BYTES`,
  `AGENT_THREAD_MCP_STDIO_MAX_ENV_VARS`,
  `AGENT_THREAD_MCP_STDIO_MAX_ENV_VALUE_BYTES`,
  `AGENT_THREAD_MCP_STDIO_LEASE_TTL_MS`,
  `AGENT_THREAD_MCP_STDIO_MAX_CONFIG_BYTES`,
  `AGENT_THREAD_MCP_STDIO_DRY_RUN_OUTPUT_BYTES`,
  `AGENT_THREAD_MCP_RUNTIME_TIMEOUT_MS`, and
  `AGENT_THREAD_MCP_RUNTIME_MAX_OUTPUT_BYTES`. `application.Init` wires the
  optional executor through `WithDefaultADKToolProviderMCPExecutor`, using
  `primaryServices.mcpToolSVC` as resolver, the durable
  `MCPRuntimeWorkdirLeaseRepository`, `infra.IDGenSVC`, the content-free run
  event sink, and the configured stdio runtime transport. Dry-run mode must
  use `ADKMCPRuntimeStdioDryRunRunner`; real Eino mode must use
  `ADKMCPRuntimeStdioEinoRunner` inside the same Coze-owned policy, workdir,
  lease, audit, health, timeout, and output-budget shell. Bootstrap must not
  project secrets, own OAuth/session lifecycle, run stale lease cleanup, or
  expose lease rows/public API data. Env parse and validation
  errors may name only environment variables, not configured values. Dry-run
  output, real MCP output, lifecycle events, and errors must not leak model
  arguments, command names/args, env keys/values, workdir/root paths, raw MCP
  tool names, server names, raw config/auth, URLs, object keys, provider
  diagnostics, prompt/model text, transcripts, checkpoint bytes, or
  secret-adjacent data.
- MCP stdio workdir lease reaping starts in M6.19.
  `ADKMCPRuntimeStdioWorkdirLeaseReaper` is a one-shot cleanup boundary for
  expired active stdio workdir leases. It lists expired leases through
  `MCPRuntimeWorkdirLeaseRepository`, validates every row before filesystem
  cleanup, delegates cleanup to `ADKMCPRuntimeStdioWorkdirPreparer`, and only
  then finishes the lease as `failed` with sanitized
  `last_error="stale lease expired"`. It must use the original lease
  `worker_id` when finishing so repository active-status and worker compare
  checks prevent closing a row that changed ownership. Nil rows, non-active
  rows, empty worker IDs, invalid IDs, relative workdirs, root workdirs, and
  paths outside the configured absolute root are invalid and must not be
  cleaned or finished. Cleanup failure must leave the active lease open for a
  future retry. Reaper errors and result counters must not include
  workdir/root paths, model arguments, raw MCP tool names, command names/args,
  env keys/values, config/auth, URLs, object keys, provider diagnostics,
  prompt/model text, transcripts, checkpoint bytes, or secret-adjacent data.
- MCP stdio workdir lease reaper worker starts in M6.20.
  `ADKMCPRuntimeStdioWorkdirLeaseReaperWorker` is the scheduled wrapper around
  the M6.19 one-shot reaper. It is default-off and starts from
  `application.Init` only when
  `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ENABLED=true`. It requires an
  absolute `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ROOT` and supports bounded
  `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_BATCH_SIZE` and
  `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_INTERVAL_MS`. The worker root is a
  deliberate separate cleanup opt-in and must not automatically inherit
  `AGENT_THREAD_MCP_STDIO_WORKDIR_ROOT`. `RunOnce` may be used for tests or
  future maintenance hooks, while `Start` owns ticker scheduling. Startup
  status reasons may name missing/misconfigured components but must not echo
  configured root values. Worker logs may include aggregate counters and fixed
  sanitized errors only. The worker must not expose lease rows, workdir paths,
  command details, env values, config/auth, prompts, model text, transcripts,
  checkpoint bytes, provider payloads, URLs, object keys, or secrets.
- MCP runtime audit persistence starts in M6.21.
  `agent_mcp_runtime_audit_events` is a durable metadata-only audit table for
  MCP runtime lifecycle records. It may store audit ID, space/thread/run/server
  IDs, Eino-safe runtime tool name, lifecycle event type, sanitized error code,
  elapsed milliseconds, output byte count, and creation timestamp. It must not
  store tool arguments, model input/output, tool output, MCP config/auth,
  command args, env values, URLs, filenames, object keys, provider diagnostics,
  transcripts, checkpoint bytes, raw provider bodies, or secrets.
  `ApplicationADKMCPRuntimeAuditRecorder` owns ID generation and maps
  `ADKMCPRuntimeAuditRecord` to `MCPRuntimeAuditRepository`; recorder failures
  return only `mcp runtime audit record failed`. `ADKMCPRuntimeExecutor` may
  record content-free `mcp.tool.started`, `mcp.tool.completed`, and
  `mcp.tool.failed` audit records through
  `WithADKMCPRuntimeExecutorAuditRecorder`. `application.Init` wires the
  durable recorder into the optional MCP runtime executor. Audit writing is
  currently auxiliary for the dry-run runtime path; a future real MCP adapter
  may make audit persistence fail-closed once the production audit SLO and
  retry semantics are defined.
- MCP runtime health classification starts in M6.22. Runtime health updates use
  the existing Workbench MCP server health fields and must remain an internal
  metadata-only application-service path. `ADKMCPRuntimeExecutor` may report
  only post-transport terminal outcomes through
  `ADKMCPRuntimeHealthReporter`: success becomes healthy, transport failure
  becomes unhealthy with `transport_failed`, and inline output-budget failure
  becomes unhealthy with `output_budget_exceeded` only when the output cannot
  be offloaded. Successful output offload is a successful runtime outcome;
  output offload failure becomes unhealthy with `output_offload_failed`.
  Pre-transport validation, policy,
  tenant, disabled-server, missing-tool, invalid-argument, missing-resolver,
  or missing-transport failures must not mutate server health. The mcptool
  service owns mapping reports to `Catalog.UpdateHealth`, timestamp fallback,
  latency clamping, and health-error sanitization. Health errors may contain
  only short ASCII error codes; unsafe, long, path-like, URL-like, or
  secret-adjacent input must collapse to `runtime_failed`. Health status,
  checked timestamp, latency, and sanitized error code must not expose tool
  arguments/results, model input/output, MCP config/auth, command details, env
  values, workdir paths, URLs, filenames, object keys, provider payloads,
  transcripts, checkpoint bytes, raw transport errors, stack traces, or
  secrets.
- MCP stdio Eino runner starts in M6.23.
  `ADKMCPRuntimeStdioEinoRunner` is the first real stdio MCP execution runner.
  It implements `ADKMCPRuntimeStdioSandboxRunner`, receives only an already
  projected and policy-checked `ADKMCPRuntimeStdioSandboxExecution`, creates
  an initialized MCP client through `ADKMCPRuntimeStdioEinoClientFactory`,
  loads the selected tool through Eino's MCP adapter
  `github.com/cloudwego/eino-ext/components/tool/mcp v0.0.8`, invokes only the
  configured target tool, bounds output, and closes the client. The underlying
  MCP client dependency is `github.com/mark3labs/mcp-go v0.43.0`.
  Production bootstrap may install this runner only when
  `AGENT_THREAD_MCP_STDIO_EINO_ENABLED=true` and
  `AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED` is false. Do not bypass the existing
  stdio static policy, run-scoped workdir projection, leased workdir preparer,
  outer timeout/output budget, audit recorder, or M6.22 health reporter when
  adding real MCP stdio behavior. Runner errors must stay fixed and sanitized:
  invalid execution, missing factory/provider, client failure, discovery
  failure, missing tool, non-invokable tool, tool call failure, or output
  budget exceeded. They must not include command names/args, env values,
  workdir paths, model arguments, tool results, raw MCP config/auth, raw MCP
  tool names, URLs, filenames, object keys, provider diagnostics, stack
  traces, prompts, completions, transcripts, checkpoint bytes, or secrets.
  Session pooling, OAuth lifecycle, encrypted credential retrieval,
  SSE/streamable HTTP transports, metrics/exporter, frontend policy controls,
  and browser E2E remain separate follow-up work.
- MCP runtime output offload starts in M6.24.
  `ADKMCPRuntimeExecutor` may use `ADKMCPRuntimeOutputOffloader` when a
  post-transport MCP tool result exceeds the inline output budget. The
  production adapter is `ADKMCPRuntimeOutputOffloadBackendAdapter`, which
  reuses the existing `ADKOffloadBackend`, object storage, runtime file
  registry, and `read_file` tool-result retrieval path. Oversized MCP output
  must be written under the existing runtime offload virtual path protocol
  using the `trunc` phase, and the model-visible result must be a small
  `coze.mcp_runtime_output_offload.v1` JSON notice containing only schema,
  offload status, Eino-safe runtime tool name, server ID, output byte count,
  virtual path, and read tool name. The notice, run events, audit rows, health
  records, and errors must not contain raw tool output, model input/output,
  tool arguments, MCP config/auth, command args, env values, object URIs,
  storage URLs, provider payloads, transcripts, checkpoint bytes, stack
  traces, filenames from providers, or secrets. Missing offloader keeps the
  previous fail-closed `output_budget_exceeded` behavior; configured offloader
  failure maps to fixed `output_offload_failed`. `application.Init` must wire
  MCP output offload through the same `ADKOffloadBackendFactory` used by ADK
  reduction so MCP runtime and normal Eino tool-result offload share one
  Coze-owned storage and registry boundary.
- MCP stdio secret projection starts in M6.25.
  `ADKMCPRuntimeStdioTransport` may parse an optional config `auth_env` map
  whose keys are process environment variable names and whose values are
  dot-separated paths inside the internal raw `MCPToolServer.Auth` JSON. The
  transport resolves those values after parsing normal `env`, and
  auth-projected values override same-name public config env values. The
  resulting env map must still pass `ADKMCPRuntimeStdioStaticPolicy`
  `AllowedEnvKeys`, `MaxEnvVars`, and `MaxEnvValueBytes`; `auth_env` must not
  bypass policy. Projection errors are fixed and sanitized: invalid
  `auth_env`, invalid auth JSON, missing auth field, non-string auth field, or
  invalid env name. Errors, run events, audit rows, health records, and model
  output must not expose auth paths, env values, command names/args, workdir
  paths, raw MCP config/auth, model arguments, tool output, object keys,
  provider payloads, transcripts, checkpoint bytes, or secrets. This is not a
  full secret manager: OAuth refresh, real KMS/secret-manager provider,
  encrypted-row migration/rotation, masked edit UX, and remote transport
  credentials remain separate Coze-owned work.
- MCP auth catalog codec starts in M6.26. `MySQLCatalog` accepts an optional
  `MCPAuthCodec` through `WithMySQLCatalogAuthCodec`. `Upsert` must encode
  internal raw `MCPToolServer.Auth` before writing `mcp_tool_servers.auth`;
  `Get` and `List` must decode the stored value before returning internal
  server records to the application and runtime resolver. The default codec is
  passthrough for compatibility, tests, local development, and existing rows.
  Codec outputs must remain valid JSON because the current column type is
  JSON. Encode and decode failures must return only fixed sanitized errors:
  `mcp tool auth encode failed` or `mcp tool auth decode failed`; they must not
  include raw auth JSON, ciphertext, provider diagnostics, KMS IDs, object
  keys, stack traces, or secrets. Workbench public responses still mask auth
  after catalog reads; runtime `ResolveADKMCPRuntimeServer` receives decoded
  raw auth for M6.25 `auth_env` projection. This is only the storage boundary:
  real KMS/secret-manager integration, encrypted-row migration, key rotation,
  OAuth refresh, masked edit UX, and remote transport credential flows remain
  separate Coze-owned work.
- Use Eino filesystem middleware only through a Coze-owned filesystem Backend
  and Shell implementation. Production code must not bind Eino filesystem or
  shell tools directly to host paths or host command execution.
- Reuse Eino summarization, reduction, large-result offloading, and
  `patchtoolcalls` middleware. Coze remains responsible for transcript and
  artifact persistence, access control, retention, memory flush, and frontend
  presentation.
- Configure Eino `patchtoolcalls` through the Coze adapter. Patched dangling
  tool calls use the model-visible `coze.tool_repair.v1` JSON payload and may
  emit a content-free `tool.repaired` event; do not expose raw repaired tool
  output content in that event.
- Normalize ADK tool endpoint errors through the Coze
  `tool_error_normalization` middleware before they reach the model. The
  normalized tool result schema is `coze.tool_error.v1`; it applies to
  invokable, streamable, enhanced invokable, and enhanced streamable tools.
  The ADK event mapper must translate those tool results to `tool.failed`,
  including enhanced text parts from `UserInputMultiContent`.
- Tool error payloads are recovery hints, not raw diagnostics. Trim and bound
  `error_message`, avoid stack traces or internal object keys, and keep retry
  eligibility, policy decisions, audit, and provider classification Coze-owned.
- Use Eino `ModelRetryConfig` for model retry. Retry is disabled by default
  and may be enabled only through explicit run config (`model_retry` or
  `modelRetry`). Keep `max_retries` bounded to 1-5 and `backoff_ms` bounded to
  0-60000. Do not retry context cancellation, ADK cancellation, interrupts,
  stream cancellation, or safety-classified finish reasons.
- Do not wire model failover to a fake or implicit fallback. Real failover
  requires a Coze-owned model-candidate provider that enforces model catalog,
  credentials, tenant authorization, policy, quota, and audit.
- Safety/content-filter finish reasons are Coze event semantics. Map them to
  `model.safety_finish` with a bounded `finish_classification` payload while
  preserving the assistant text for transcript/checkpoint continuity.
- Keep Eino `MaxIterations` as the hard execution budget for ADK model-call
  loops. Use the Coze `semantic_loop` middleware only as a softer production
  guard for repeated behavior; do not replace Eino's Agent loop with a custom
  loop.
- Semantic-loop detection is disabled unless run config contains
  `semantic_loop` or `semanticLoop`. Bound
  `max_repeated_tool_calls` / `maxRepeatedToolCalls` and
  `max_repeated_assistant_messages` / `maxRepeatedAssistantMessages` to
  `0-20`; zero disables that detector.
- Semantic-loop events must be content-free. Map `ADKSemanticLoopError` to
  `run.semantic_loop_detected` with kind, hash signature, count, limit, and
  bounded error text only. Never include raw model text, tool arguments,
  transcript content, object keys, or checkpoint bytes in this event.
- Provider capabilities are a Coze-owned gate before provider invocation.
  Extend `ADKModelCapabilities` and the optional `ADKProviderCapabilityModel`
  interface when a model adapter can safely declare support for thinking,
  reasoning, vision, PDF, file, audio, or video input. Run config overrides may
  use `provider_capabilities` / `providerCapabilities` or
  `model_capabilities` / `modelCapabilities`.
- The `provider_capability` middleware must run immediately before
  `multimodalbudget`. It validates normalized `reasoning_effort` values
  (`minimal`, `low`, `medium`, `high`), `thinking_enabled`, and media parts
  against declared capabilities while preserving complete source messages for
  transcript and checkpoint state.
- Per-run reasoning and thinking requests must be projected through
  Coze-owned model option adapters before provider invocation. Prefer an
  explicit `ADKReasoningOptionProjector` implementation on custom model
  adapters; otherwise `ApplicationADKAgentFactory` may use the built-in
  Eino-ext projections for OpenAI reasoning effort, Ark reasoning/thinking,
  and Qwen/Gemini/Claude/DeepSeek thinking. Do not silently ignore a supported
  reasoning request when no projector exists. Current built-in declarations
  are provider-family coarse; future durable model catalog capabilities should
  narrow support per model/version.
- Map `ADKProviderCapabilityError` to `model.unsupported_capability` with only
  capability, part type, count, and bounded error text. Do not include raw
  attachment URLs, filenames, base64 data, prompt text, provider responses,
  object keys, or credentials in this event.
- Provider capability checks do not authorize file fetching, PDF parsing, OCR,
  transcoding, artifact registration, or security scanning. Those remain behind
  Coze-owned filesystem, sandbox, policy, scanner, retention, and audit
  boundaries.
- Treat complete `*schema.Message` values, including multimodal parts, as
  Coze-owned source state. File and multimodal history limits must be enforced
  through a transient, non-mutating model-input projection; never replace the
  checkpoint or transcript source message with a pruned provider view.
- Protect every file and multimodal part in the latest user turn. If that turn
  alone exceeds its configured allocation, fail before provider invocation.
  Allocate remaining history newest-first and emit only content-free budget
  metadata in events and omission markers.
- Until provider-aware accounting is implemented, estimate image, audio,
  video, and file parts with deterministic provider-neutral units. Do not
  count base64 payload length or RFC 2397 data payload length as text tokens.
- Multimodal budgeting does not authorize file fetching, host filesystem
  access, artifact registration, or payload offloading. Those capabilities
  require the Coze-owned filesystem backend, policy, scanning, retention, and
  audit boundaries.
- Eino tool-result reduction may write only through the Coze runtime offload
  backend. Its virtual paths are restricted to
  `/mnt/user-data/workspace/.coze/tool-results/runs/{run_id}/{trunc|clear}/`,
  writes must target the active run, and reads may resolve a source run only
  inside the active thread and space.
- Runtime file registration must independently enforce the same tool-result
  offload path policy before persisting `agent_files`: the path must be
  `/mnt/user-data/workspace/.coze/tool-results/runs/{run_id}/{trunc|clear}/{sha256}.txt`,
  the path run ID must equal the registered run, phase must be `trunc` or
  `clear`, and the file name must be a lowercase SHA-256 hex digest with
  `.txt`. Do not rely only on ADK backend path generation for this boundary.
- Runtime file registration must also enforce deterministic object-key scope:
  `agent-runtime/{space_id}/{thread_id}/runs/{run_id}/tool-results/{trunc|clear}/{sha256}.txt`.
  The object key's space, thread, and run IDs must match the authoritative run
  row, and its phase and filename must match the validated virtual path. The
  path/object filename is the stable offload file identity; the persisted
  `Digest` field remains the content SHA-256 and must stay a separate
  lowercase SHA-256 validation.
- Runtime offload reads must resolve an active `agent_files` row through the
  Coze runtime file service before touching object storage. The resolved row
  must be a workspace file in the active run's space/thread, may point to a
  source run only inside that same space/thread, and must re-pass virtual path,
  object-key, status, size, digest, and metadata validation. Read code should
  use the service-returned object URI instead of recomputing a key from
  untrusted model input alone.
- `agent_artifacts` is the durable presentation registry for user-visible
  outputs. It is separate from `agent_files`: `agent_files` stores file
  metadata, while `agent_artifacts` records which file is presented in the
  task UI, with title, artifact type, preview mode, and listing order.
- Artifact registration must load the backing `agent_files` row server-side
  and require active `workspace` or `output` files in the requested
  space/thread/run. The initial preview mode is conservative and based on the
  stored content type: plain text/JSON/CSV may be text, safe raster images may
  be image, PDF may be pdf, and HTML/XHTML/SVG/unknown binary must be
  download. This registration-time classification is not a replacement for
  future preview/download API MIME sniffing, active-content response headers,
  audit, or authorization.
- The initial artifact workbench APIs are read-only:
  `GET /api/workbench/task_threads/:thread_id/artifacts` and
  `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content`.
  List may filter by `run_id` and returns presentation metadata only. It must
  not return `object_uri`, storage URLs, file bytes, inline previews, or
  download links. Content reads must resolve artifact metadata by
  `thread_id + artifact_id` server-side, read the service-owned object URI
  from that row, and never expose the object URI in JSON, headers, or errors.
  Content responses must set `X-Content-Type-Options: nosniff`; `mode=download`
  always uses attachment, and `mode=preview` may use inline only for
  preview-safe text, raster image, and PDF artifacts. HTML, XHTML, SVG,
  unknown content, and `preview_mode=download` must remain attachment. MIME
  sniffing, authorization expansion, deletion, audit, output scanning, and
  signed links remain separate M4 work.
- Artifact content reads must normalize scan status from artifact metadata
  keys `scan_status` or `scanStatus` before reading object storage. Explicit
  unsafe statuses `pending`, `failed`, `blocked`, `infected`, and
  `quarantined` block both preview and download reads before storage access.
  Missing or unrecognized status is currently compatible and audited as
  `unknown` until a full scanner worker and outage policy are available.
  Successful reads with a configured thread event sink emit
  `artifact.content.accessed` with `schema=coze.artifact_access.v1`; payloads
  must stay content-free and omit object URIs, virtual paths, storage URLs,
  filenames, file bytes, prompt text, model output, tool arguments,
  checkpoint bytes, credentials, and provider raw bodies.
- Artifact content responses must perform read-time MIME sniffing after object
  storage returns bytes. The response `Content-Type` should use the sniffed
  type for non-empty content, while inline preview remains conservative:
  `mode=download` is always attachment, and `mode=preview` may be inline only
  when the registered artifact content type, the stored preview mode, and the
  sniffed response type are all preview-safe. If any of those classify as
  download-only, return attachment. HTML, XHTML, SVG, XML, JavaScript,
  octet-stream, unknown binary, and mismatched active content must never be
  served inline. Sniffing is a presentation/header control only; it does not
  replace scan status, antivirus scanning, sandbox execution, quarantine, or
  preview conversion.
- Artifact deletion is a soft-delete of the `agent_artifacts` registry row via
  `deleted_at`; it must not delete `agent_files`, object storage keys,
  runtime offload files, or sandbox files. Normal artifact list, metadata get,
  preview, and download paths must resolve only `deleted_at=0` rows. The
  Workbench endpoint is
  `DELETE /api/workbench/task_threads/:thread_id/artifacts/:artifact_id` and
  accepts only the scoped path IDs, never object URI, virtual path, file ID,
  storage URL, or filename from the client. Successful deletes with a
  configured thread event sink emit content-free `artifact.deleted` with
  `schema=coze.artifact_deleted.v1`; payloads may include thread/run/artifact
  IDs, file ID, artifact type, content type, size, and deleted time, but must
  omit object URIs, virtual paths, storage URLs, filenames, file bytes, prompt
  text, model output, tool arguments, checkpoint bytes, credentials, and
  provider raw bodies. Physical cleanup, retention, restore, and orphan
  reconciliation remain separate lifecycle worker work.
- Artifact list, content, and delete operations must pass the current viewer
  identity from the Workbench handler into the application layer and run
  through `ArtifactAuthorizer` before repository listing, metadata lookup,
  object storage reads, or soft-delete writes. Production initialization uses
  `ThreadOwnerArtifactAuthorizer` as the first policy: only the canonical
  thread creator can read or delete that thread's artifacts. A nil authorizer
  is compatibility-only for tests/local transitional wiring. Denied access
  maps to HTTP 403 with a generic `artifact access denied` message and must
  not reveal whether an artifact exists, object URIs, virtual paths, storage
  URLs, filenames, file bytes, prompts, model output, tool arguments,
  checkpoint bytes, credentials, or provider raw bodies. Future space-member,
  admin, share-link, and service-account access must replace or extend this
  same authorizer boundary, not bypass it in handlers.
- Artifact scan decisions are written through the domain/application scan
  result boundary, not by ad hoc metadata mutation. Use
  `UpdateArtifactScanResult` in the domain artifact service to merge
  normalized `scan_status`, `scan_scanner`, `scan_scanner_version`,
  `scan_reason`, and `scan_scanned_at` into active artifact metadata while
  preserving unrelated keys. New writes may use only `clean`, `pending`,
  `failed`, `blocked`, `infected`, or `quarantined`; `unknown` remains a
  read-time compatibility value for historical rows. Application-level scan
  recording uses `RecordArtifactScanResult` and emits content-free
  `artifact.scan.completed` with `schema=coze.artifact_scan.v1` when a run is
  present. Scan audit events may include scoped IDs, artifact type, content
  type, size, scan status, scanner, scanner version, and scanned time, but
  must omit object URIs, virtual paths, storage URLs, titles, filenames, file
  bytes, scan reasons, prompt text, model output, tool arguments, checkpoint
  bytes, credentials, and provider raw bodies. Scanner workers, queues,
  external antivirus engines, quarantine review, and fail-closed outage policy
  remain separate work and must reuse this write boundary.
- New artifact registration must create a durable pending scan state before the
  artifact can be read. `RegisterArtifact` must force `scan_status=pending`,
  set the bounded scanner name and `scan_requested_at`, ignore any
  caller-provided `clean` claim, and create or get an
  `agent_artifact_scan_jobs` row through `CreateOrGetArtifactScanJob` using an
  idempotency key of `artifact_scan:{artifact_id}:{scanner}`. The initial
  scanner name is `default`. Scan job rows may persist scoped thread/run/space,
  user, artifact, file IDs, scanner, status, attempts, errors, and timestamps,
  but must not persist object URI, virtual path, storage URL, title, filename,
  file bytes, prompt text, model output, tool arguments, credentials,
  checkpoint bytes, or provider raw bodies. M4.15 only enqueues pending scan
  jobs; worker claiming, scanner execution, retry/backoff, quarantine review,
  and fail-closed outage policy remain follow-up work.
- Artifact scan jobs are claimed through the domain artifact service, not by
  ad hoc SQL from worker code. Use `ClaimArtifactScanJobs` with scanner,
  worker ID, limit, and lease TTL. The service defaults scanner to `default`,
  limit to `10`, and lease TTL to `300000` milliseconds, and rejects empty
  worker IDs. The repository may claim due `pending` jobs and expired
  `processing` jobs for the same scanner, marks them `processing`, sets
  `worker_id`, `lease_expires_at`, `started_at` on first claim, increments
  `attempt_count`, and updates `updated_at`. Scan job lease rows and claim
  results must not contain object URI, virtual path, storage URL, title,
  filename, file bytes, prompt text, model output, tool arguments,
  credentials, checkpoint bytes, or provider raw bodies. M4.16 does not mark
  jobs `succeeded` or `failed`, execute scanners, read object storage, or apply
  quarantine; those follow-on paths must reuse this lease boundary.
- Claimed artifact scan jobs must be terminally updated through the domain
  artifact service, not by direct worker SQL. `CompleteArtifactScanJob`
  requires a processing job with matching worker ID and active lease, writes
  the artifact scan result through `UpdateArtifactScanResult`, then marks the
  job `succeeded`, clears the lease, clears `last_error`, and sets
  `ended_at`. `FailArtifactScanJob` marks only the job `failed`, clears the
  lease, stores bounded error text, and must not mutate artifact scan metadata;
  this keeps pending artifacts blocked until a retry or review path records a
  real scan decision. Terminal job updates must not include object URI,
  virtual path, storage URL, title, filename, file bytes, prompt text, model
  output, tool arguments, credentials, checkpoint bytes, or provider raw
  bodies. M4.17 still does not execute scanners, read object storage, perform
  retry/backoff scheduling, or apply quarantine.
- Artifact scan job execution goes through the application-level
  `ProcessArtifactScanJobs` shell. It must claim jobs through
  `ClaimArtifactScanJobs`, load artifacts by scoped IDs, read bytes only
  through the configured `ArtifactObjectStorage`, call an injected
  `ArtifactContentScanner`, and finish jobs through
  `CompleteArtifactScanJob` or `FailArtifactScanJob`. Scanner requests may
  include scoped IDs, scanner name, content type, size, and artifact bytes, but
  must not include object URI, virtual path, storage URL, title, filename,
  prompt text, model output, tool arguments, credentials, checkpoint bytes, or
  provider raw bodies. Scanner and storage failures must store generic
  content-free error text such as `artifact scan failed`; never persist raw
  scanner errors, object keys, paths, filenames, or bytes. Successful scanner
  completion must emit the existing `artifact.scan.completed` event with
  scoped IDs and scan metadata only; do not include scanner reason, raw errors,
  object keys, paths, filenames, or bytes. M4.18 still does not add a daemon,
  scheduler, external antivirus dependency, retry/backoff,
  quarantine review, or fail-closed outage policy.
- Artifact scan worker startup is controlled by
  `AGENT_ARTIFACT_SCAN_WORKER_ENABLED=true|false` and is disabled by default.
  `StartArtifactScanWorkerFromEnv` must refuse to start unless
  `ArtifactSVC`, `ArtifactObjectStorage`, and an injected
  `ArtifactContentScanner` are configured. It may read worker ID, scanner
  name, batch size, interval, and lease TTL from
  `AGENT_ARTIFACT_SCAN_WORKER_ID`,
  `AGENT_ARTIFACT_SCAN_WORKER_SCANNER`,
  `AGENT_ARTIFACT_SCAN_WORKER_BATCH_SIZE`,
  `AGENT_ARTIFACT_SCAN_WORKER_INTERVAL_MS`, and
  `AGENT_ARTIFACT_SCAN_WORKER_LEASE_TTL_MS`. Do not install a fake clean
  scanner, silently fall back to permissive scan results, or start the worker
  when scanner configuration is absent. Worker logs and result counts may
  include scanner name, worker ID, counts, and bounded errors only; never log
  object URI, virtual path, filenames, artifact bytes, scanner reason,
  credentials, checkpoint bytes, or provider raw bodies. M4.19 still does not
  add an external antivirus adapter, retry/backoff, quarantine review, public
  API, or fail-closed outage policy.
- The first concrete artifact scanner adapter is HTTP and is opt-in through
  `AGENT_ARTIFACT_SCANNER_TYPE=http`. `InitService` may inject it only through
  `NewArtifactContentScannerFromEnv`, using
  `AGENT_ARTIFACT_SCANNER_HTTP_URL`,
  `AGENT_ARTIFACT_SCANNER_HTTP_TOKEN`,
  `AGENT_ARTIFACT_SCANNER_HTTP_TIMEOUT_MS`, and
  `AGENT_ARTIFACT_SCANNER_HTTP_MAX_BYTES`. Missing, unknown, or invalid scanner
  configuration must leave `ArtifactScanner` nil so the M4.19 worker refuses
  to start; do not install a fake clean scanner or permissive fallback. The
  HTTP request schema is `coze.artifact_scan_request.v1` and may include only
  scanner name, scoped IDs, content type, size, and base64 artifact content.
  It must not include object URI, virtual path, storage URL, title, filename,
  prompt text, model output, tool arguments, credentials, checkpoint bytes, or
  provider raw bodies. HTTP scanner responses may return only terminal
  statuses: `clean`, `blocked`, `infected`, or `quarantined`; reject `pending`,
  `failed`, unknown statuses, non-2xx responses, oversized content, and invalid
  payloads as scanner errors. Scanner version and reason flow through the
  bounded domain scan-result path, while audit events still omit scanner reason
  and raw scanner errors. M4.20 still does not add a bundled antivirus engine,
  scanner-service deployment, retry/backoff, quarantine review, or
  fail-closed outage policy.
- Artifact scanner deployment diagnostics must stay fail-closed and sanitized.
  `NewArtifactContentScannerFromEnvWithStatus` may return
  `ArtifactScannerEnvStatus` with enabled/type/configured/error fields only;
  `InitService` stores it on `ApplicationService.ArtifactScannerStatus`.
  `StartArtifactScanWorkerFromEnvWithStatus` may return
  `ArtifactScanWorkerEnvStatus` with worker enabled/started/reason and scanner
  status, while `StartArtifactScanWorkerFromEnv` remains a compatibility
  wrapper. Invalid, missing, or unsupported scanner config must still leave
  `ArtifactScanner` nil and prevent the worker from starting. Status fields,
  logs, env examples, events, API responses, and errors must not include
  scanner tokens, artifact object URIs, virtual paths, storage URLs, filenames,
  file bytes, scanner raw responses, prompt text, model output, tool
  arguments, credentials, checkpoint bytes, or provider raw bodies.
  `docker/.env.example` and `docker/.env.debug.example` document scanner env
  keys with scanning disabled by default; operators must deploy a trusted
  scanner service separately before enabling `AGENT_ARTIFACT_SCANNER_TYPE=http`
  or `AGENT_ARTIFACT_SCANNER_TYPE=clamd` with
  `AGENT_ARTIFACT_SCAN_WORKER_ENABLED=true`. M4.27 still does not add a
  bundled antivirus engine, scanner container, signed links, restore, physical
  cleanup, richer preview transforms, browser E2E coverage, or fail-open
  behavior.
- Clamd artifact scanning is opt-in through
  `AGENT_ARTIFACT_SCANNER_TYPE=clamd` or `clamav`. The Go adapter uses clamd's
  `INSTREAM` protocol against `AGENT_ARTIFACT_SCANNER_CLAMD_ADDR`, applies
  `AGENT_ARTIFACT_SCANNER_CLAMD_TIMEOUT_MS` and
  `AGENT_ARTIFACT_SCANNER_CLAMD_MAX_BYTES`, maps `OK` to `clean`, maps
  `FOUND` to `infected` with a bounded signature reason, and treats clamd
  errors, malformed responses, connection failures, and oversized content as
  scanner errors. Docker compose files include an optional `artifact-scanner`
  service under the `scanner` profile using
  `AGENT_ARTIFACT_SCANNER_CLAMD_IMAGE` with scanning still disabled by default.
  Main Docker env examples point clamd to `artifact-scanner:3310`; debug env
  examples point local Go servers to `127.0.0.1:3310`. Do not make Coze server
  depend on the scanner profile, do not silently fall back to clean results,
  and do not expose clamd raw responses, object URIs, virtual paths, storage
  URLs, filenames, file bytes, scanner tokens, prompt text, model output, tool
  arguments, credentials, checkpoint bytes, or provider raw bodies through
  logs, events, API responses, or UI. M4.28 still does not add signed links,
  restore, physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, browser E2E coverage, or fail-open policy.
- Artifact scan outage fail-mode is controlled by
  `AGENT_ARTIFACT_SCAN_OUTAGE_FAIL_MODE`. Empty, invalid, or `closed` keeps
  the M4.23 behavior: only `scan_status=clean` may read bytes. The only
  supported non-closed mode is `open_non_executable`, and it may override only
  outage-like states `unknown`, `pending`, and `failed` for artifacts whose
  stored preview mode and content type both classify as non-executable text or
  raster image. It must not allow `blocked`, `infected`, or `quarantined`, and
  it must not allow PDF, HTML, XHTML, SVG, octet-stream, download-only, or
  unsupported artifacts. Override access events may include
  `scan_policy_mode`, `scan_policy_reason`, and `scan_policy_override`; those
  events must remain content-free and must not include artifact title, virtual
  path, object URI, storage URL, filename, file bytes, scanner raw response,
  prompt text, model output, tool arguments, credentials, checkpoint bytes, or
  provider raw bodies. M4.29 still does not add signed links, restore,
  physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, or browser E2E coverage.
- Artifact restore is a registry-only soft-delete reversal. Repository restore
  clears `agent_artifacts.deleted_at` with a CAS update scoped by
  `thread_id + artifact_id` and only for rows where `deleted_at > 0`; it must
  not mutate `agent_files`, runtime offload rows, object storage, scan jobs, or
  artifact bytes. Workbench exposes
  `POST /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/restore`
  and the task-detail drawer may show an immediate `撤销移除` action after a
  successful delete. Restore must authorize with
  `ArtifactAccessOperationRestore`, return only a receipt plus safe artifact
  ID metadata, refresh the active artifact list after success, and emit
  content-free `artifact.restored` with `schema=coze.artifact_restored.v1`.
  Restore responses, events, and logs must not include artifact title, virtual
  path, object URI, storage URL, filename, file bytes, scanner raw response,
  prompt text, model output, tool arguments, credentials, checkpoint bytes, or
  provider raw bodies. The task-detail UI may reuse the artifact name that was
  already visible before delete for the local undo notice, but it must not
  fetch or render hidden metadata from the restore response. M4.30 still does
  not add physical cleanup, signed links, richer preview transforms,
  MIME-specific renderer hardening, or browser E2E coverage.
- Artifact deleted browsing uses the existing Workbench artifact list endpoint
  with explicit `deleted_only=true`. The default list must remain active-only
  (`deleted_at=0`); deleted-only mode must return only `deleted_at>0` rows and
  must not silently broaden active task detail loads or content-read paths.
  Artifact list payloads may include `deleted_at` lifecycle metadata. The
  task-detail drawer may expose a `当前 / 已移除` switch, and the `已移除` view
  may show safe artifact summary metadata plus row-level restore only. It must
  not expose preview, download, delete, or scan-review actions for removed
  rows, and it must not include object URI, storage URL, file bytes, scanner
  raw response, prompt text, model output, tool arguments, credentials,
  checkpoint bytes, or provider raw bodies. Restoring from the removed list
  must call the restore endpoint and refresh both active and deleted lists.
  M4.31 still does not add physical cleanup, signed links, richer preview
  transforms, MIME-specific renderer hardening, or browser E2E coverage.
- Artifact scan retry/backoff uses `RetryArtifactScanJob`, not direct SQL and
  not artifact metadata mutation. A retry requires a processing job with
  matching worker ID and active lease, then sets the scan job back to
  `pending`, clears worker and lease fields, records bounded generic error
  text, sets `available_at`, clears `ended_at`, and preserves
  `attempt_count` until the next claim increments it. `ProcessArtifactScanJobs`
  accepts `MaxAttempts` and `RetryBackoffMillis`; defaults are one attempt and
  60000 ms. Retry only when `max_attempts > 1` and the current
  `attempt_count` is below the max. At or above the max, use the existing final
  `FailArtifactScanJob` path. Both retry and final failure must keep artifact
  scan metadata unchanged so pending artifacts remain blocked until a real
  terminal scan decision or future review path records one. Worker env may set
  `AGENT_ARTIFACT_SCAN_WORKER_MAX_ATTEMPTS` and
  `AGENT_ARTIFACT_SCAN_WORKER_RETRY_BACKOFF_MS`. Retry/failure text and logs
  must not include object URI, virtual path, storage URL, title, filename,
  artifact bytes, raw scanner errors, prompt text, model output, tool
  arguments, credentials, checkpoint bytes, or provider raw bodies. M4.21 still
  does not add quarantine review, dead-letter UI, scanner service deployment,
  fail-open behavior, or a fail-closed outage policy matrix.
- Artifact scan job observability is exposed through the read-only Workbench
  endpoint `GET /api/workbench/task_threads/:thread_id/artifact_scan_jobs`.
  It may filter by run, artifact, status, scanner, and pagination, and may
  return only scan job metadata: job/thread/run/space/user/artifact/file IDs,
  scanner, job status, worker ID, attempt count, bounded last error,
  availability/lease/start/end/create/update timestamps, and total count.
  The endpoint reuses artifact list authorization and must not return artifact
  title, virtual path, object URI, storage URL, filename, content bytes,
  scanner raw response, prompt text, model output, tool arguments, credentials,
  checkpoint bytes, or provider raw bodies. It is for dead-letter inspection
  only; it must not mutate artifact metadata, retry jobs, mark scans clean,
  quarantine, restore, or perform manual release. M4.22 still does not add a
  frontend dead-letter UI, quarantine review action, scanner service
  deployment, fail-open behavior, or a fail-closed outage policy matrix.
- Artifact content reads follow an explicit fail-closed scan policy. Only
  `scan_status=clean` may read bytes from `ArtifactObjectStorage`; `unknown`,
  missing, invalid, `pending`, `failed`, `blocked`, `infected`, and
  `quarantined` must all block before object storage access. Blocked reads
  emit content-free `artifact.content.blocked` with
  `schema=coze.artifact_access_blocked.v1`, scoped IDs, mode, preview mode,
  artifact type, normalized scan status, and a reason code such as
  `scan_unknown`, `scan_pending`, `scan_failed`, `scan_blocked`,
  `scan_infected`, or `scan_quarantined`. The Workbench content endpoint maps
  this typed policy error to HTTP `409` with a bounded reason. The event,
  error, and response must not include artifact title, virtual path, object
  URI, storage URL, filename, file bytes, scanner raw response, prompt text,
  model output, tool arguments, credentials, checkpoint bytes, or provider raw
  bodies. M4.23 still does not add manual release, quarantine review actions,
  fail-open behavior, scanner service deployment, or frontend dead-letter UI.
- The task-detail artifact drawer is the canonical thread frontend surface for
  `agent_artifacts`. It should load artifacts through
  `ListTaskThreadArtifacts`, render safe text-like previews inline from the
  content endpoint with `mode=preview`, use signed URLs for downloads and
  non-text safe previews, and soft-delete visible artifacts through
  `DELETE /api/workbench/task_threads/:thread_id/artifacts/:artifact_id`.
  Delete actions must use a Semi/Coze `Popconfirm`, send only scoped
  `thread_id + artifact_id`, refresh the artifact list after success, and keep
  errors inside the drawer. Do not render artifact bytes with
  `dangerouslySetInnerHTML`, inline HTML/SVG iframes, object storage URLs, or
  object URIs in the DOM. Continue to hide preview actions for `download` and
  `unsupported` preview modes.
  Preview buttons must also require a matching safe MIME family: text previews
  only for `text/plain`, `text/markdown`, `text/csv`,
  `text/tab-separated-values`, or `application/json`; image previews only for
  raster image types; PDF previews only for `application/pdf`. HTML, XHTML,
  SVG, `application/octet-stream`, missing types, and preview-mode/content-type
  mismatches must render as download-only in the drawer. M4.32 hardens
  frontend preview affordances only; it does not add physical cleanup, richer
  preview transforms, or browser E2E coverage.
- Artifact signed URLs support safe previews and forced-attachment downloads
  as of M4.34. The Workbench endpoint is
  `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url`
  with `mode=preview` or `mode=download` and optional `ttl_seconds`; the
  backend clamps TTL to 60-3600 seconds and defaults to 300 seconds. Signed
  URL creation must reuse artifact read authorization, scan read policy,
  active artifact lookup, and server-side MIME sniffing before calling object
  storage `GetObjectUrl`. Preview mode must still require stored preview mode,
  stored content type, and sniffed content type to agree on a safe preview
  family. Download mode may sign scan-allowed active artifacts only when the
  storage adapter honors signed response-header overrides: it must pass
  `response-content-disposition=attachment; filename*=UTF-8''...` from
  server-side artifact metadata plus the sniffed `response-content-type` to
  the underlying MinIO, S3, or TOS presigner. Custom storage adapters must
  implement the same `storage.WithResponseContentDisposition` and
  `storage.WithResponseContentType` semantics before being used for signed
  downloads. Scan-blocked, missing, deleted, or unregistered artifacts must
  not receive signed URLs. The signed URL response may include only artifact
  ID, signed URL, expiry seconds, effective content type, and preview mode; it
  must not include object URI, virtual path, object key outside the signed URL
  itself, file bytes, scanner raw response, prompt text, model output, tool
  arguments, credentials, checkpoint bytes, or provider raw bodies. The
  task-detail drawer uses signed URLs for `下载` and non-text safe `预览`; the
  backend content endpoint remains available for safe text inline previews and
  must keep `nosniff` plus server-side `Content-Disposition`. Browser E2E
  coverage remains separate production acceptance work.
- Artifact physical cleanup is backend-only in M4.35. Cleanup may process only
  artifacts with `deleted_at > 0` and `deleted_at <= cutoff` whose backing
  `agent_files.status` is still `active`; active artifacts and already
  cleaned files must be skipped. The application service owns the object-store
  side effect: it deletes the server-side `ObjectURI` first, then marks the
  backing file as `deleted` by matching both `file_id` and expected
  `object_uri`. If object deletion fails, the file row must remain active so a
  later pass can retry. `storage.ErrObjectNotFound` is idempotent success and
  may mark the file deleted. Cleanup must not physically remove
  `agent_artifacts` rows, must not expose a user-facing permanent delete
  action, and must not include object URI, virtual path, object key, filenames,
  file bytes, scanner raw response, prompt text, model output, tool arguments,
  credentials, checkpoint bytes, or provider raw bodies in audit events or
  responses. Cleanup audit may include only schema, thread/run/artifact/file
  IDs, artifact type, content type, size, `deleted_at`, `cleaned_at`, and a
  bounded `not_found` marker. M4.35 still does not add lifecycle scheduling,
  retention policy UI, object-store prefix sweep reconciliation, or browser
  E2E coverage.
- Artifact inline preview transforms exist as of M4.36 for safe text-like
  artifact families only: `text/plain`, `text/markdown`, `application/json`,
  `text/csv`, and `text/tab-separated-values`. The frontend must fetch these
  through the existing Workbench content endpoint with `mode=preview`, then
  render bounded React text or a Coze Design `Table`. Markdown remains escaped
  source text, not rendered HTML. JSON may be pretty-printed locally. CSV/TSV
  preview is capped to 50 data rows and 12 columns, and source text is capped
  to 128 KiB before rendering. Inline preview state may contain only artifact
  ID, display name, effective content type, bounded transformed text/table
  rows, and a truncation marker. It must not contain object URI, virtual path
  as a fetch authority, object key, signed URL, scanner raw response, prompt
  text, model output, tool arguments, credentials, checkpoint bytes, or
  provider raw bodies. HTML, XHTML, SVG, octet-stream, missing content types,
  and MIME/mode mismatches remain download-only.
- Artifact frontend preview affordances use `artifactPreviewFamily` as the
  shared positive allow-list as of M4.38. `canPreviewArtifact` and
  `artifactInlinePreviewKind` must delegate to that helper instead of adding a
  second MIME/mode switch. Adding a new preview MIME type requires updating the
  allow-list and helper tests together. Frontend family checks are only an
  affordance boundary; backend content and signed URL endpoints remain the
  authority for authorization, scan policy, server-side MIME sniffing,
  `nosniff`, attachment disposition, and storage signing.
- Raster image artifact previews render inside the task drawer as of M4.39.
  Only image-family values allowed by `artifactPreviewFamily` may use this
  path: PNG, JPEG, GIF, WebP, BMP, and TIFF. The frontend must request the
  existing signed URL endpoint with `mode=preview` and render the returned URL
  only as an `<img src>` with `loading="lazy"` and
  `referrerPolicy="no-referrer"`. Do not render signed URLs as visible text,
  copy them into selectors, persist them, send them to analytics, or use this
  renderer for SVG, PDF, HTML, XHTML, octet-stream, or MIME/mode mismatches.
  PDF previews remain signed URL new-tab behavior until a separate sandboxed
  renderer design is accepted.
- PDF artifact previews are explicitly non-inline as of M4.40. A PDF preview
  action must request the existing signed URL endpoint with `mode=preview`,
  clear any stale drawer inline preview, and open the URL with
  `window.open(url, '_blank', 'noopener,noreferrer')`. Do not render PDF signed
  URLs inside the drawer, in an iframe, as visible text, or in selector
  attributes. Future PDF embedding requires a separate sandboxed renderer
  design and browser E2E coverage.
- Artifact preview actions clear stale inline preview state at preview start
  as of M4.41. This applies to text, raster image, PDF, and any future preview
  route so a failed or non-inline preview cannot leave an old drawer renderer
  visible. Download actions must not clear the current preview.
- Artifact drawer E2E selector contract starts in M4.37. Keep stable
  selectors on the task artifact entry button (`task-artifacts-open`), each
  visible artifact row (`task-artifact-item` plus safe `data-artifact-id`),
  and inline preview surfaces (`task-artifact-inline-preview`,
  `task-artifact-inline-preview-text`, and
  `task-artifact-inline-preview-table`). These selectors are for browser E2E
  automation only and must not encode object URI, virtual path, object key,
  signed URL, filenames from untrusted storage paths, prompt/model content,
  tool arguments, credentials, checkpoint bytes, or provider raw bodies.
- The task-detail artifact drawer also exposes read-only artifact scan job
  observability. On drawer open, the frontend calls
  `GET /api/workbench/task_threads/:thread_id/artifact_scan_jobs` through
  `listTaskThreadArtifactScanJobs` with `page=1&page_size=20`, shows only
  safe scan metadata such as job ID, status, scanner, worker ID, attempt
  count, run ID, and bounded last error, and allows manual refresh. This UI
  must not show artifact title, virtual path, object URI, storage URL,
  filename, file bytes, scanner raw response, prompt text, model output, tool
  arguments, credentials, checkpoint bytes, or provider raw bodies. M4.24 is
  observability-only; it must not retry jobs, mutate artifact metadata, mark
  scans clean, quarantine, restore, or perform manual release.
- Artifact scan job manual retry is exposed through
  `POST /api/workbench/task_threads/:thread_id/artifact_scan_jobs/:job_id/retry`
  and the task-detail artifact drawer. It may only requeue a scan job in the
  same thread when the current job status is `failed`; repository updates must
  be compare-and-swap on `id + thread_id + status=failed`. A retry sets
  `status=pending`, clears worker, lease, start, and end fields, records only
  bounded generic `last_error=manual retry requested`, preserves
  `attempt_count`, and does not mutate artifact scan metadata or mark an
  artifact clean. Successful retry emits content-free
  `artifact.scan_job.retry_requested` with
  `schema=coze.artifact_scan_job_retry_requested.v1`, scoped IDs, scanner,
  status, attempt count, and availability time only. UI retry buttons are
  shown only for `failed` scan jobs and refresh the queue after success. The
  API, event, UI, and errors must not include artifact title, virtual path,
  object URI, storage URL, filename, file bytes, scanner raw response, prompt
  text, model output, tool arguments, credentials, checkpoint bytes, or
  provider raw bodies. M4.25 still does not add manual release, quarantine
  review, restore, scanner service deployment, signed links, physical cleanup,
  or fail-open behavior.
- Artifact scan human review is exposed through
  `POST /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/scan_review`
  and the task-detail artifact drawer. It accepts only `release`,
  `quarantine`, and `block`: `release` writes `scan_status=clean`,
  `quarantine` writes `scan_status=quarantined`, and `block` writes
  `scan_status=blocked`, with `scan_scanner=manual_review`. The application
  layer must authorize through `ArtifactAccessOperationReview`, update only
  the scoped `thread_id + artifact_id` artifact metadata, and return only a
  review receipt rather than full artifact content or storage metadata.
  Successful review emits content-free `artifact.scan.reviewed` with
  `schema=coze.artifact_scan_review.v1`, scoped IDs, artifact type, content
  type, size, decision, final scan status, scanner, and review timestamps. The
  optional human reason may be stored in artifact metadata but must not be
  emitted in audit events. The UI may show scan status tags and row-level
  `放行` / `隔离` / `阻断` actions for non-clean artifacts, then refresh the
  artifact list after success. Review does not bypass the content endpoint:
  bytes remain readable only when the final metadata state is `clean`. The API,
  event, UI review response, and errors must not include artifact title,
  virtual path, object URI, storage URL, filename, file bytes, scanner raw
  response, prompt text, model output, tool arguments, credentials, checkpoint
  bytes, or provider raw bodies. M4.26 still does not add restore, scanner
  service deployment, signed links, physical cleanup, richer preview
  transforms, MIME-specific renderer hardening, browser E2E coverage, or
  fail-open behavior.
- The M2 offload filesystem surface is read-only to the model: expose only the
  bounded, byte-paginated `read_file` tool. Keep `ls`, `write_file`,
  `edit_file`, `glob`, `grep`, shell, and streaming shell disabled until the M4
  sandbox and workspace policy is implemented.
- Runtime offload object keys are deterministic. If database registration
  fails after object storage succeeds, do not delete the key because an older
  successful registration may reference it. Emit no content or object URI in
  events and leave orphan reconciliation to the lifecycle worker.
- The legacy `github.com/cloudwego/eino/flow/agent/react` integration remains a
  migration compatibility path only. Do not add new platform capabilities
  directly to it; place new runtime behavior behind the Agent Harness adapter
  boundary so it can move to ADK.
- Use stable Eino releases for the production baseline. Do not adopt alpha,
  beta, or release-candidate versions without an explicit architecture review.
- Production runtime selection is controlled by
  `AGENT_THREAD_RUNTIME_DEFAULT=legacy|eino_adk` and
  `AGENT_THREAD_EINO_ADK_ENABLED=true|false`. The defaults are `legacy` and
  `false`. An `eino_adk` default or per-run override is invalid unless ADK is
  enabled. Validate this policy before persisting a queued run and again at
  executor selection; never silently fall back from an invalid runtime.
- Treat Eino core and `eino-ext` adapters as one compatibility set. When the
  Eino version changes, review and upgrade every directly referenced model,
  embedding, and ACL adapter together; do not upgrade only
  `github.com/cloudwego/eino`. At the `v0.9.9` baseline this includes the Ark,
  Claude, DeepSeek, Gemini, Ollama, OpenAI, and Qwen model adapters plus the
  configured embedding adapters.
- Eino `v0.9.x` tool schemas use JSON Schema. For inferred tools, use
  `utils.WithSchemaModifier` with `github.com/eino-contrib/jsonschema`; do not
  add new code using the removed OpenAPI-v3 `WithSchemaCustomizer` or
  `ToOpenAPIV3` APIs. Add contract tests for dynamic descriptions, enums,
  required fields, arrays, and nested objects when migrating custom schemas.
- Eino `v0.9.x` interrupt/resume is address-based:
  - Persist and expose stable Coze-owned interrupt IDs while retaining the Eino
    address and `InterruptContexts` inside the runtime checkpoint envelope.
  - New runtime code should use `Interrupt`, `StatefulInterrupt`,
    `CompositeInterrupt`, `Resume`, and `ResumeWithData`.
  - Interrupt info, state, resume data, and composite payloads must be
    checkpoint-serializable. Use stable strings or registered DTOs; do not put
    runtime-only values such as `error`, functions, channels, open streams, or
    unregistered interface implementations into checkpoint state.
  - `InterruptAndRerun` and `NewInterruptAndRerunErr` are deprecated
    compatibility APIs. Existing Workflow code may retain them until its
    checkpoint format is migrated, but new Harness/ADK code must not depend on
    them.
  - Tests for nested graphs, tools, loops, batches, and subagents must verify
    targeted resume, partial resume, replay after restart, and a single
    terminal outcome.
- Keep public checkpoint payloads independent of Eino's gob-serialized runtime
  structs. Store Eino checkpoint bytes in a versioned internal envelope with
  runtime version, envelope/schema version, message type, interrupt mapping,
  run revision, and migration metadata.
- When upgrading Eino:
  1. Upgrade and compile the existing runtime without behavior changes.
  2. Run the repository's backend test mode with
     `-gcflags="all=-l -N"` because Mockey-based tests require it.
  3. Introduce ADK adapters and event/checkpoint contract tests.
  4. Migrate runtime slices behind feature flags.
  5. Remove the legacy ReAct path only after replay, resume, cancellation,
     token, and LangGraph compatibility gates pass.

### Key Architectural Patterns
- **Adapter Pattern**: Extensive use for loose coupling between layers
- **Interface Segregation**: Clear contracts between domains
- **Event-Driven**: NSQ message queue for async communication
- **API-First**: Comprehensive OpenAPI specifications

## Database & Infrastructure

### Docker Services Stack
- **Database**: MySQL 8.4.5
- **Cache**: Redis 8.0
- **Search**: Elasticsearch 8.18.0 with SmartCN analyzer
- **Vector DB**: Milvus v2.5.10 for embeddings
- **Storage**: MinIO for object storage
- **Message Queue**: NSQ (nsqlookupd, nsqd, nsqadmin)
- **Configuration**: etcd 3.5

### Database Management
```bash
# Sync database schema
make sync_db

# Dump database schema
make dump_db

# Initialize SQL data
make sql_init

# Atlas migration management
make atlas-hash
```

### Atlas CLI In Codex Worktrees

- The repository's migration tooling is pinned to Atlas Community
  `v0.35.0`, matching the `arigaio/atlas:0.35.0-community-alpine` development
  image. Do not generate `atlas.sum` with an arbitrary newer CLI.
- If `atlas` is already available on `PATH`, confirm `atlas version` reports
  `v0.35.0`, then use `make atlas-hash`.
- In this worktree, local `atlas version` has been confirmed as `v0.35.0`.
  Prefer local Atlas commands for migration work:
  `(cd docker/atlas && atlas migrate hash)` and
  `atlas migrate validate --dir file://docker/atlas/migrations`. Fall back to
  `arigaio/atlas:0.35.0-community-alpine` only if local Atlas is missing or
  the version no longer matches.
- On macOS Codex worktrees, Homebrew installation may stall during formula API
  refresh or require system-level write permissions. Use Atlas's official
  installer in download-only mode instead; this requires no `sudo` and does
  not modify the system environment:

```bash
curl -sSf https://atlasgo.sh -o /tmp/atlas-install.sh
ATLAS_VERSION=v0.35.0 CI=true sh /tmp/atlas-install.sh \
  --community --no-install -o /tmp/atlas-v0.35.0
chmod +x /tmp/atlas-v0.35.0

# Regenerate checksums after adding or editing migration SQL.
(cd docker/atlas && /tmp/atlas-v0.35.0 migrate hash)

# Validate the complete migration directory from the repository root.
/tmp/atlas-v0.35.0 migrate validate \
  --dir file://docker/atlas/migrations
```

- Never hand-edit `docker/atlas/migrations/atlas.sum`. Keep the temporary Atlas
  binary outside the repository and do not commit it.

## Key Development Patterns

### Frontend Package Development
- Each package follows consistent structure with `README.md`, `package.json`, `tsconfig.json`, `eslint.config.js`
- Adapter pattern extensively used for decoupling (e.g., `-adapter` suffix packages)
- Base/Core pattern for shared functionality (e.g., `-base` suffix packages)
- Use workspace references (`workspace:*`) for internal dependencies

### Backend Development
- Follow DDD principles with clear domain boundaries
- Use dependency injection via interfaces
- Implement proper error handling with custom error types
- Write comprehensive tests for domain logic

### Model Configuration
Before deployment, configure AI models in `backend/conf/model/`:
1. Copy template from `backend/conf/model/template/`
2. Set `id`, `meta.conn_config.api_key`, and `meta.conn_config.model`
3. Supported providers: OpenAI, Volcengine Ark, Codex, Gemini, Qwen, DeepSeek, Ollama

## Testing Strategy

### Coverage Requirements by Package Level
- **Level 1**: 80% coverage, 90% increment
- **Level 2**: 30% coverage, 60% increment
- **Level 3-4**: 0% coverage (flexible)

### Testing Framework
- **Frontend**: Vitest for unit/integration tests
- **Backend**: Go's built-in testing framework
- **E2E**: Separate e2e subspace configuration

## Common Issues & Solutions

### Frontend Development
- Use `rush update` instead of `npm install` at root level
- Build packages in dependency order using `rush build`
- For hot reload issues, check Rsbuild configuration in specific package

### Backend Development
- Ensure middleware services are running (`make middleware`)
- Check database connectivity and schema sync
- Verify model configurations are properly set

### Docker Issues
- Ensure sufficient resources (minimum 2 Core, 4GB RAM)
- Check port conflicts (8888 for frontend, various for services)
- Use `make clean` to reset Docker volumes if needed

## IDL and Code Generation

The project uses Interface Definition Language (IDL) for API contract management:
- IDL files in `idl/` directory (Thrift format)
- Frontend code generation via `@coze-arch/idl2ts-*` packages
- Backend uses generated Go structs

## Plugin Development

For custom plugin development:
- Reference templates in `backend/conf/plugin/pluginproduct/`
- Follow OAuth schema in `backend/conf/plugin/common/oauth_schema.json`
- Configure authentication keys for third-party services

## Contributing

- Use conventional commits via `rush commit`
- Run linting with `rush lint-staged` (pre-commit hook)
- Ensure tests pass before submitting PRs
- Follow team-based package organization and tagging conventions
