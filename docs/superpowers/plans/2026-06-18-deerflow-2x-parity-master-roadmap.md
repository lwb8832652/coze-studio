# DeerFlow 2.x Full Capability Parity Master Roadmap

## Baseline

This roadmap defines the production target for the Coze Studio Go-native Agent
Harness integration.

- DeerFlow reference repository: `bytedance/deer-flow`
- Reference branch: `main`
- Reference commit: `6044e5c5535d5f7a9aadb471ee0f1a6db279c48a`
- Reference date: 2026-06-18
- Product vocabulary remains task-oriented:
  - New Chat becomes `新建任务`
  - Chats becomes `全部任务`
  - Recent Chats becomes `我的任务`
  - Chat Detail becomes `任务详情`
- The implementation target is Go-native. A Python sidecar is not part of the
  migration strategy.
- Eino `v0.9.9` is the Agent execution baseline. Eino ADK is the target
  execution kernel; Eino `v0.10.0` alpha releases are outside the production
  baseline.

This is a capability-parity target, not a source-code port. Coze Studio keeps
its existing account, API authorization, resource configuration, development
configuration, plugin, workflow, knowledge, and database investments where
they already satisfy the required contract.

## Active Priority And Phase Split

Phase 1 is the only active delivery track until DeerFlow 2.x parity is
complete. Its scope is to fully reproduce DeerFlow-style task conversations,
task detail execution flow, Agent runtime behavior, Skill configuration,
MCP/tool configuration and invocation, memory, token usage, runtime settings,
and required frontend/backend contracts inside Coze Studio.

Phase 2 contains enhancements and production-hardening work that is useful but
not required to match DeerFlow behavior. This includes Guardrail audit
archive/retention, Prometheus and OpenTelemetry exporters, complex security
scanning, policy administration UI, storage lifecycle and reconciliation,
load/chaos gates, expanded operator runbooks, and other platform optimization
items. Do not start or continue Phase 2 work while Phase 1 parity gaps remain,
except to fix regressions caused by already-written code or to unblock a Phase
1 feature test.

When a Phase 1 task uncovers a desirable enhancement that is not necessary for
DeerFlow parity, add it to the Phase 2 backlog instead of implementing it
inline. The default decision rule is: if a user cannot observe the difference
in the DeerFlow-equivalent task, Agent, Skill, MCP/tool, memory, or token usage
workflow, it is Phase 2.

### Phase 1 Mainline Queue

Use this queue when selecting the next task. Complete the earliest unfinished
DeerFlow-visible capability before moving to later or Phase 2 work.

1. Task conversation and task-detail execution flow: `新建任务`, `全部任务`,
   `我的任务`, `任务详情`, event timeline, interruption, resume, retry,
   cancel, reconnect, empty/error/loading states.
2. Go-native Eino ADK runtime: streaming, checkpoints, resume, cancellation,
   TurnLoop follow-up input, normalized events, LangGraph-compatible
   thread/run/SSE API behavior required by DeerFlow workflows.
3. Agent and subagent configuration: durable Agent records, model/prompt/mode
   settings, subagent delegation, child run cards, timeout/cancel, usage
   attribution, and task-oriented UI labels.
4. Skills: create, edit, enable/disable, version/history, rollback/export,
   slash/progressive activation, runtime injection, and DeerFlow-equivalent
   frontend/backend controls.
5. MCP and tools: durable server/tool catalog, stdio/SSE/HTTP config, secret
   handling, health checks, runtime invocation through Eino, deferred tool
   search, and task-detail tool events.
6. Memory: retrieval, async update, task memory management UI, edit/delete/
   restore/import/export, recall budgets, and runtime prompt injection.
7. Token usage: model/tool/subagent/middleware attribution, grouped task
   detail display, pricing-ready metadata, and settings/debug display where
   DeerFlow exposes equivalent cost or usage visibility.
8. Settings and acceptance: DeerFlow-equivalent model, mode, memory, Skill,
   tool, MCP, runtime settings, generated API contracts, unit tests, black-box
   API tests, and browser checks for canonical task workflows.

Phase 2 work must not be selected from this roadmap while any Phase 1 item
above is incomplete. If Phase 2 code already exists in the worktree, finish
only the minimal verification needed to avoid leaving a broken build, then
return to the earliest unfinished Phase 1 item.

### Phase 2 Backlog Intake

Record, but do not implement, optimization or hardening ideas that do not
directly change a DeerFlow-visible Phase 1 workflow. Phase 2 backlog items
include:

- Guardrail policy administration, audit archive, retention, legal hold,
  archive-before-delete, and audit export hardening.
- Prometheus metrics, OpenTelemetry trace export, dashboards, alerts, and
  operator runbooks beyond the event and token usage views needed for Phase 1.
- Complex skill/tool/file/command/network/security scanners, quarantine,
  manual review, and security operations UI.
- Storage lifecycle/reconciliation, backup/rollback rehearsal, load/chaos
  testing, and multi-instance production acceptance gates.
- UI polish or platform optimization that is not visible in DeerFlow-equivalent
  task, Agent, Skill, MCP/tool, memory, token usage, or settings workflows.

## Explicit Exclusion

IM Channels are not part of this project:

- No Telegram, Slack, Discord, Feishu/Lark, DingTalk, WeChat, or other channel
  adapters.
- No external account binding, connect codes, channel credentials, inbound
  dispatch, outbound delivery, channel attachment delivery, or channel worker.
- No Channels settings page, channel badge, channel filter, or channel-specific
  task policy.
- Existing neutral fields such as `source` may remain, but no new channel
  behavior should be built.

The existing IM design document is retained as historical analysis only and is
not an implementation dependency or delivery milestone.

## Delivery Principles

1. Production correctness is part of parity. A UI shell or in-memory stub does
   not count as completed capability.
2. Identity and authorization come from authenticated server context. Client
   metadata is never an authority for `user_id`, `space_id`, or ownership.
3. Runs, events, checkpoints, token usage, files, memories, skills, tools, and
   agents must be durable and tenant-isolated.
4. Every executable extension passes through one Tool Registry, policy, and
   sandbox boundary. Full Guardrail audit, archive, retention, complex
   scanners, and observability hardening are Phase 2 unless required to unblock
   a DeerFlow-visible Phase 1 workflow.
5. Agent middleware is a first-class ordered pipeline with explicit state and
   replay semantics.
6. Frontend parity includes loading, empty, error, interrupted, reconnecting,
   canceled, permission-denied, and offline states.
7. Compatibility APIs are verified with black-box clients, not only handler
   unit tests.
8. Eino is an internal execution dependency, not the platform system of
   record. Public task, event, checkpoint, authorization, and LangGraph
   contracts remain Coze-owned.

## Eino ADK Integration Boundary

Eino ADK supplies the reusable Agent runtime primitives:

- `ChatModelAgent`, `Runner`, streaming `AgentEvent`, tool calling, model
  retry/failover, interrupt, resume, and checkpoint hooks.
- `TurnLoop` for queued input, push, preemption, safe-point cancellation,
  checkpointed stop, and resumable multi-turn execution.
- `AgentTool` and `DeepAgent` for delegation, task decomposition, subagents,
  filesystem tools, shell backends, and todo state.
- Middleware for summarization, tool-result reduction and offloading, dangling
  tool-call repair, filesystem access, dynamic tool search, plan tasks,
  `AGENTS.md`, and progressive `SKILL.md` loading.
- Skill execution modes `inline`, `fork`, and `fork_with_context`.
- Provider token-usage metadata, reasoning/media schema, and callback surfaces.
- Eino-ext adapters for model providers, MCP tools, web search/fetch, HTTP, and
  other external tool providers where they satisfy Coze security requirements.

Coze Studio remains responsible for:

- durable task/thread/run/message/event records and tenant authorization;
- stable checkpoint schemas, worker leases, cancellation, retry, recovery, and
  idempotency;
- LangGraph-compatible API and SSE event translation;
- Agent, Skill, MCP, tool, model, memory, sandbox, and settings control planes;
- Skill lifecycle, versions, permissions, allowed tools, scanning, quarantine,
  audit, and frontend management;
- token attribution, pricing snapshots, cost accounting, metrics, and
  observability.

Migration rules:

1. Upgrade the dependency while retaining the existing
   `flow/agent/react` behavior.
2. Add a Coze-owned ADK adapter for events, checkpoints, tools, skills, usage,
   and cancellation.
3. Migrate Harness capabilities incrementally behind feature flags.
4. Prefer `ChatModelAgent` with `AgentTool` or `DeepAgent`; use sequential,
   parallel, or loop workflow agents only when their fixed workflow semantics
   are specifically required.
5. Remove the legacy ReAct path only after checkpoint replay, resume,
   cancellation, streaming, token attribution, security, and LangGraph
   compatibility gates pass.
6. Use `*schema.Message` as the production baseline. Keep
   `*schema.AgenticMessage` experimental because Eino `v0.9.9` does not yet
   provide equivalent stream cancellation and retry behavior for it.
7. Before adding a new execution algorithm, verify whether stable Eino ADK
   already provides it. Coze should implement an adapter or policy wrapper
   instead of a competing Agent loop, retry, Todo, compression, tool-search,
   tool-repair, filesystem, Skill, or subagent implementation.

### Reuse Classification

| Capability | Eino responsibility | Coze responsibility |
| --- | --- | --- |
| Agent execution | ChatModelAgent, Runner, streaming events | runtime adapter, durable event translation |
| Turns and live input | TurnLoop, push, preempt, safe points | API, authorization, message/event persistence |
| Interrupt/cancel | address-based resume and recursive cancel | stable interrupt IDs, run state, worker acknowledgement |
| Model reliability | retry and failover | provider policy, credentials, audit, model catalog |
| Context control | summarization and reduction | budgets, transcript/artifact persistence, memory flush |
| Tool robustness | ToolCall repair, structured wrappers | error taxonomy, policy, audit, retry eligibility |
| Todo and plan | DeepAgent Todo, plantask, planexecute | durable backend, task-detail event/UI contract |
| Dynamic tools | toolsearch and deferred tools | durable registry, permissions, policy records |
| Subagents | AgentTool and DeepAgent task tool | Agent definitions, quotas, durable parent-child records |
| Skills | Skill middleware and execution modes | lifecycle, versions, permissions, scanning and UI |
| Files and shell | filesystem middleware interfaces | sandbox implementation, mounts, quotas and security |
| MCP invocation | eino-ext MCP tool adapter | transports, OAuth, secrets, sessions, sandbox and health |
| Token/tracing | response metadata and callbacks | attribution, price/cost, durable traces and metrics |

## Capability Matrix

| Domain | DeerFlow 2.x baseline | Current state | Required result |
| --- | --- | --- | --- |
| Thread/run runtime | durable threads, runs, events, state, history, resume | main persistence and API skeleton exists | production identity, lifecycle, HA, cleanup, migration |
| Streaming | model/tool/reasoning SSE with reconnect | persisted-event polling; model uses non-stream `Generate` | native model streaming, event bridge, replay, backpressure |
| Cancellation | active run cancellation | status update only | propagated cancel signal, worker acknowledgement, terminal consistency |
| Worker HA | durable run manager and recovery | DB claim with `SKIP LOCKED` | lease expiry, heartbeat, stale-run recovery, retry/DLQ |
| LangGraph API | thread state update/history, run stream/wait/join, messages | large subset exists | SDK black-box parity, authz, error/event shape parity |
| Agent middleware | ordered runtime middleware chain | monolithic Harness loop | Eino ADK handler pipeline plus Coze policy adapters |
| Dynamic context | date, memory, task context injection | basic memory and skill injection | deterministic context builder with budgets |
| Summarization | context threshold, summary preservation, memory flush | absent | Eino summarization plus Coze transcript and memory hooks |
| Todo/Plan | plan mode and live todo state | empty checkpoint field only | Coze-backed Eino plantask tools, events and task-detail UI |
| Clarification | return-direct clarification interrupt | absent | interrupted state, user response, resume flow, UI |
| Tool robustness | dangling call repair and structured tool errors | ADK call repair and normalized `tool.failed` events exist behind feature gate | policy, retry eligibility, audit hardening |
| Loop/safety control | loop detection and provider safety finish handling | absent | configurable loop detector and safety termination |
| Tool output budget | truncate/externalize oversized output | absent | Eino reduction/offload plus Coze artifact registration |
| Deferred tool search | tool catalog search and runtime promotion | absent | Coze catalog feeding Eino `tool_search` and deferred tools |
| Thinking/Vision | model capability flags and reasoning/media state | ADK reasoning/media events, callback metadata, and provider capability gate exist behind feature flag | Provider-specific option projection and catalog-backed capability resolution |
| Lead Agent modes | flash/thinking/pro/ultra context presets | Auto/Ask/Agent legacy modes | task-oriented equivalent with persisted per-thread settings |
| Subagents | registry, parallel task tool, timeout/cancel, token attribution | ADK `AgentTool` provider boundary exists; durable AgentHub wiring remains open | concurrent subagent runtime, state cards, tracing and usage |
| Custom agents | gallery, create/update/delete, SOUL/config, bootstrap | absent | Agent management backend and frontend |
| ACP agents | configured external agent invocation | absent | optional Go ACP client and permission boundary |
| Skills | parser, install, enable, edit, history, slash activation | versioning and prompt injection mostly present | create/enable/delete, slash/progressive activation, policy, evolution |
| Tool Registry | builtin, Coze, MCP, deferred, skill policy | in-memory registry skeleton | durable unified resolver and policy decision records |
| MCP | stdio/SSE/HTTP, OAuth, cache, persistent sessions | in-memory config and stub call | durable config, real transports, secret handling, sandbox execution |
| Sandbox | local/container/Kubernetes provider, virtual paths | existing CodeRunner only | provider API, thread mounts, lifecycle, quotas, audit |
| Files/uploads | thread upload directory and conversion | generic upload service only | thread-scoped files, conversion, limits, cleanup |
| Artifacts | outputs browsing, safe preview/download/present | absent | artifact registry, preview policy, drawer, citations |
| Memory | retrieval, async update queue, user editing | basic thread memory injection | facts model, updater, import/export/edit/clear, budgets |
| Token usage | per model/tool/subagent/middleware and UI modes | basic persistence and aggregate indicator | provider extraction, attribution, pricing, debug/per-turn UI |
| Guardrails | pre-tool deterministic policy and fail mode | absent | Phase 2 enhancement: allow/deny/confirm policy, fail-closed option, audit. Phase 1 uses only the minimal policy boundary needed for DeerFlow parity. |
| Security scanning | skill/config/call/file/command/network/secret scanning | archive structural checks only | Phase 2 enhancement: layered scanners, quarantine, manual review, audit UI. Phase 1 avoids complex scanners unless required for the parity feature to run safely. |
| Settings | model/mode/token/memory/skills/tools/runtime settings | scattered controls | consolidated Agent settings while preserving account/API auth |
| Feedback | run feedback and statistics | absent | run-level feedback API and task-detail interaction |
| Suggestions | follow-up suggestions | absent | post-run suggestion generation and UI |
| Web tools | search, fetch and HTTP tools | ADK Web Tool Catalog exposes policy-controlled `web_fetch` and backend-driven/env-backed HTTP `web_search` | Durable provider settings/UI, scanner and domain policy |
| Runtime doctor | provider and extension diagnostics | scattered configuration errors | model/MCP/sandbox/Skill connectivity and capability checks |
| Observability | tracing, run journal, structured metrics | logs and local counters | Phase 2 enhancement: OpenTelemetry traces, metrics, dashboards, runbook. Phase 1 keeps only the events and usage data visible in DeerFlow-equivalent workflows. |

## Milestone Order

### M0 - Current Slice Closure

1. Finish and verify the current Skills frontend/version-management changes.
2. Regenerate API artifacts instead of relying on handwritten generated-model
   extensions.
3. Add frontend unit coverage and a real browser acceptance pass.
4. Keep the worktree free of unrelated generated churn.

Exit gate:

- Skill import, list, edit, resource edit, rollback, export, selection, and
  Harness injection pass backend and frontend tests.

### M1 - Runtime Authority And High Availability

1. Derive identity and space access from authenticated context.
2. Add thread/run/message/event/checkpoint authorization at service boundaries.
3. Implement native model streaming and normalized model events.
4. Implement active cancellation propagation.
5. Add worker lease expiry, heartbeat, stale-run recovery, retry, and DLQ.
6. Add per-space concurrency, queue limits, timeout, and idempotency controls.
7. Complete thread patch/delete/state update and cleanup APIs.
8. Add JS and Python LangGraph SDK black-box suites.

Exit gate:

- Two API replicas and two workers can process, cancel, reconnect, and recover
  runs without sticky sessions or cross-tenant access.

### M2 - Go Agent Middleware And Execution Semantics

1. Introduce the Coze-owned Eino ADK adapter and feature flag.
2. Translate `AgentEvent` into durable Coze events and LangGraph SSE events.
3. Wrap ADK checkpoint storage with a versioned Coze checkpoint envelope.
4. Bridge active cancellation, interrupt targets, and resume data.
5. Integrate `TurnLoop` for follow-up input, preemption, safe-point stop, and
   resumable multi-turn execution.
6. Assemble and contract-test the Eino handler order instead of creating a
   competing middleware framework.
7. Dynamic context and deterministic token budgets.
8. Eino summarization with transcript persistence and memory flush hooks.
9. Eino reduction and filesystem offloading with Coze artifact registration.
10. Coze-backed Eino `plantask` tools and persisted Todo/Plan events.
11. Clarification and human-confirmation tools using Eino tool interrupts.
12. Eino dangling tool-call repair plus Coze tool error normalization.
13. Eino model retry/failover plus Coze LLM error and safety-finish
    classification.
14. Coze semantic loop detection and execution budgets.
15. Deferred tool catalog search and promotion using Eino dynamic tool search.
16. Eino Callback bridge for tracing, model metadata, and raw token usage.
17. Thinking, reasoning effort, image/PDF, and provider capability support.
18. Add Web search/fetch/HTTP tools through Eino-ext where policy and provider
    contracts pass review.

Progress evidence as of 2026-06-21:

- [x] The ADK adapter foundation, event/checkpoint contracts, cancellation,
  TurnLoop, middleware ordering, replay ordering, and usage idempotency gates
  are implemented behind the production feature gate.
- [x] Run-configured context limits, deterministic local token estimates,
  token-bounded thread-memory injection, observable Eino summarization events,
  middleware usage attribution, and summarized checkpoint restart coverage are
  implemented.
- [x] Coze-backed Eino Skill progressive discovery and inline execution are
  implemented with immutable run snapshots, frontmatter compatibility,
  bounded metadata/content, and production ADK wiring.
- [x] Eino dynamic tool search now runs before summarization and is guarded by
  deterministic budgets across visible and provider-native deferred tool
  definitions.
- [x] M2.7 complete: complete Eino messages are preserved in Coze checkpoints
  and transcripts while independent file and multimodal history allocations
  produce non-mutating, newest-first provider projections for normal and
  summarization calls. The latest user attachment turn fails closed when it
  exceeds its allocation; interrupt/resume replay preserves the original
  source state. Provider-aware accounting, file fetch, filesystem offload,
  artifact registration, scanning, and retention remain in M2.9/M4. Forked
  Skill execution remains tied to the M3 AgentHub,
  allowed-tool intersection, and durable child-run contracts.
- [x] M2.8 complete: immutable pre-summary and successful-terminal transcript
  snapshots, replay-stable digesting, tenant-safe snapshot references, durable
  idempotent memory flush jobs, and fail-open queue observability are wired to
  Eino ADK. Memory extraction, debounce, retry/DLQ processing, structured fact
  mutation, and user-facing memory management remain in M7.
- [x] M2.9a complete: Eino reduction now offloads oversized and historical
  tool results through a tenant-scoped Coze object-storage backend, records
  durable `agent_files` metadata, and exposes only a bounded read-only
  `read_file`. Source-run paths remain readable after fresh-run resume within
  the same thread, while writes are restricted to the active run.
- [x] M2.10 complete: Eino `TaskCreate`, `TaskGet`, `TaskUpdate`, and
  `TaskList` run through a Coze-backed `plantask.Backend`. Normalized
  `agent_run_plans` and `agent_run_plan_items` rows provide database CAS task
  ID reservation, durable history, soft archival after Eino cleanup, and
  source-run plan continuity on resume. Bounded `plan.task.*` events are
  projected into the existing “执行流程” UI with per-plan-task deduplication.
- [x] M2.11 complete: Eino ADK clarification and human-confirmation tools are
  exposed through Coze-owned DTOs, stable interrupt IDs, `command.resume.targets`,
  workbench resume APIs, LangGraph checkpoint resume target validation, and a
  Semi/Coze Design task-detail card. Resolved interactions create queued resume
  runs with deterministic idempotency and durable `human.interaction.resolved`
  events.
- [x] M2.12 complete: Eino `patchtoolcalls` now uses Coze
  `coze.tool_repair.v1` payloads and emits content-free `tool.repaired`
  events. ADK invokable, streamable, enhanced invokable, and enhanced
  streamable tool endpoint errors are normalized to `coze.tool_error.v1`; the
  event mapper projects them as `tool.failed`, including enhanced
  `UserInputMultiContent` text parts.
- [x] M2.13a complete: explicit run-configured model retry is wired through
  Eino `ModelRetryConfig` with bounded retry/backoff settings, pass-through for
  cancel/interrupt/control-flow errors, empty-output and selected finish-reason
  retry decisions, and `model.safety_finish` classification for safety/content
  filter finish reasons. Real failover candidate selection remains open until
  the Coze model catalog/provider policy boundary is available.
- [x] M2.14 complete: Coze-owned `semantic_loop` ADK middleware detects
  repeated Assistant tool-call plans and repeated Assistant text responses
  after model calls while keeping Eino `MaxIterations` as the hard execution
  budget. Loop termination maps to content-free
  `run.semantic_loop_detected` events with only kind, hash signature, count,
  limit, and bounded error text.
- [x] M2.15 complete: `ADKToolSetProvider` now lets ADK resolve static and
  deferred tools from a single Coze policy snapshot, and
  `ADKRuntimeToolCatalogProvider` adapts policy-filtered runtime catalog
  entries into Eino tools for existing `dynamictool/toolsearch` discovery and
  promotion. Human-interaction tool wrapping preserves deferred tools from
  the underlying provider.
- [x] M2.16 complete: Eino ADK callback usage records now carry Coze-owned
  deterministic `trace_id`, `span_id`, `parent_span_id`, idempotency metadata,
  raw token counts, and bounded model metadata snapshots. ADK execution result
  metadata includes the same trace ID when callback usage collection is active.
- [x] M2.17 complete: Coze-owned provider capability declarations now cover
  thinking, reasoning, vision, PDF, file, audio, and video support. The
  `provider_capability` middleware runs immediately before multimodal
  projection, rejects unsupported reasoning/thinking requests and unsupported
  media parts before provider invocation, and maps failures to content-free
  `model.unsupported_capability` events.
- [x] M2.17a complete: per-run reasoning/thinking settings now project into
  provider-specific Eino model options before ADK model invocation.
  `ApplicationADKAgentFactory` consumes explicit
  `ADKReasoningOptionProjector` adapters and includes built-in projections for
  OpenAI reasoning effort, Ark reasoning/thinking, and
  Qwen/Gemini/Claude/DeepSeek thinking. Supported requests fail closed when no
  projector exists instead of being silently ignored. Current built-in
  provider-family declarations are intentionally coarse; durable model catalog
  model/version capability narrowing remains future settings work.
- [x] M2.18 complete: A disabled-by-default `ADKWebToolCatalog` now exposes
  policy-controlled `web_fetch` and backend-driven `web_search` through the
  existing Eino runtime tool catalog path. `web_fetch` requires explicit host
  allowlists, rejects private IP literals by default, bounds timeout and
  response bytes, sanitizes redirect-policy errors, and returns structured
  `coze.web_fetch.v1` output. Direct Eino-ext `httprequest` exposure remains
  rejected because Coze must own SSRF, redirect, budget, audit, and output
  policy.
- [x] M2.18a complete: `web_search` now has an explicit production bootstrap
  path through `ADKWebSearchBackendFromEnv` and `NewADKHTTPWebSearchBackend`.
  The backend is disabled by default, requires
  `AGENT_THREAD_WEB_SEARCH_ENABLED=true`, posts bounded JSON to a fixed
  endpoint, supports secret-backed auth headers, clamps response budgets, and
  rejects private IP literals, HTTP, query/fragment endpoints, userinfo, and
  cross-host redirects unless explicitly configured for local/internal
  deployments. Application bootstrap injects the backend through
  `WithDefaultADKToolProviderWebSearchBackend`; sanitized errors avoid query
  text, endpoint paths, provider response bodies, and API keys.
- [ ] M2.9b and later middleware capabilities remain open. General workspace
  tools, upload mounting, output promotion, user-visible `agent_artifacts`,
  preview/download APIs, MIME/security scanning, retention cleanup, shell, and
  sandbox providers remain in M4. Failover candidate selection and
  durable Web provider settings/UI also remain open.

Exit gate:

- Legacy ReAct and ADK paths pass the same contract suite; middleware state
  survives checkpoint/resume; each middleware has focused unit, integration,
  and replay tests. `AgenticMessage` remains disabled unless it passes the same
  retry, cancellation, checkpoint, and streaming suite.

### M3 - Subagents And Custom Agents

1. Persistent Agent definitions and versioned SOUL/config.
2. Agent gallery, create, update, delete, and bootstrap flow.
3. Build Agent definitions into Eino `AgentTool` or DeepAgent subagents.
4. Parallel task execution using Eino Agent primitives with Coze concurrency
   and depth limits.
5. Parent cancellation, timeout, terminal race protection, and result merge.
6. Per-subagent tools, skills, model, sandbox, tracing, and token attribution.
7. Subtask cards and execution detail UI.
8. Optional ACP agent integration behind an explicit feature flag.

Exit gate:

- Parallel subagents are durable, cancelable, tenant-isolated, observable, and
  correctly attributed to parent runs.

Progress evidence as of 2026-06-21:

- [x] M3.1 started: `ADKSubagentToolProvider` now adapts Coze-owned subagent
  definitions into Eino `adk.NewAgentTool` instances while preserving base
  static and deferred tool sets. `ADKRunConfigSubagentDefinitionProvider`
  exists only as a temporary contract-test source; durable SingleAgent/AgentHub
  resolution, child-run records, policy intersections, state cards, tracing,
  and token attribution remain open.
- [x] M3.2 complete: `ADKSingleAgentSubagentDefinitionProvider` resolves
  temporary `subagent_refs` into `ADKSubagentDefinition` values by loading
  existing SingleAgent draft/version snapshots through a narrow service
  interface. The existing SingleAgent draft/version/publish tables remain the
  source of truth; full child ADK agent construction, AgentHub UI settings,
  parent-child run records, policy intersections, tracing, and token
  attribution remain open.
- [x] M3.3 complete: `ADKSingleAgentSubagentAgentFactory` now builds runnable
  child Eino ADK agents from SingleAgent draft/version snapshots. The first
  mapping covers resolved child name/description, `PromptInfo.Prompt`, and
  stable `ModelInfo` generation parameters (`ModelId`, `Temperature`,
  `MaxTokens`, `TopP`) while intentionally avoiding inheritance of parent run
  web/tool/model config. Durable child run records, per-child policy/tool/skill
  intersections, concurrency quotas, tracing, and token attribution remain
  open.
- [x] M3.4 complete: nested Eino ADK events now expose a standardized
  `subagent` identity payload with name, root, parent, step ID, run path, and
  depth. Event-side token usage metadata uses the same content-free identity
  fields, giving future child run records and frontend state cards a stable
  grouping contract. Durable child run records, lifecycle transitions,
  cancellation propagation, cost rollups, and UI rendering remain open.
- [x] M3.5 complete: `ADKSubagentToolProvider` now enforces provider-level
  explosion guards before constructing child `AgentTool` instances. Defaults
  are `max_subagents=16` and `max_depth=2`, with run-config overrides through
  `subagent_policy` / `subagentPolicy`. Child SingleAgent runs now persist the
  standardized `subagent` identity metadata needed for recursive depth checks.
  Durable child scheduling, cancellation propagation, lifecycle rows, cost
  rollups, and per-child policy intersections remain open.
- [x] M3.6 complete: child `AgentTool` invocations are now wrapped by a
  Coze-owned timeout guard. `timeout_ms` defaults to 300000 and can be
  overridden through `subagent_policy` / `subagentPolicy`. The wrapper preserves
  Eino tool metadata, propagates context cancellation/deadline errors, and does
  not inspect arguments or results. Durable child lifecycle cancellation,
  timeout terminal events, concurrency semaphores, retries, and resume semantics
  remain open.
- [x] M3.7 complete: `ADKToolPolicyProvider` now provides a shared allow-list
  boundary for static and dynamic Eino ADK tools. Lead runs without
  `tool_policy` keep existing behavior, while child SingleAgent runs emit
  explicit empty allow-lists by default and only receive tools granted by
  `ADKSubagentDefinition` / `ADKSubagentReference`. Future Skill, MCP, web,
  filesystem, and human-interaction grants should feed this same policy layer.
  Durable Skill/MCP authorization snapshots, denied-tool audit events, UI
  grants, and parent/tenant/model policy intersections remain open.
- [x] M3.8 complete: `ApplicationADKAgentFactory` now has contract coverage
  proving model-visible static tools, executable tools, and middleware tool
  inputs all come from the same policy-filtered provider result. Dynamic tool
  policy is likewise asserted at the middleware boundary. Native model tool
  search and durable Skill/MCP grant wiring still need end-to-end tests.
- [x] M3.9 complete: `ADKSubagentToolGrantProvider` now defines the durable
  grant entry point for SingleAgent-backed subagents. It returns allowed static
  and dynamic tool names, and `ADKSingleAgentSubagentDefinitionProvider`
  intersects temporary requested tools with durable grants when the provider is
  configured. This prevents run-config references from broadening permissions.
  Production AgentHub/SingleAgent settings, Skill/MCP grant snapshots,
  denied-tool audit events, and tenant/user/model policy intersections remain
  open.
- [x] M3.10 complete: `ADKSingleAgentSnapshotToolGrantProvider` now maps
  SingleAgent snapshot plugin and workflow configuration into stable child
  tool grant names. Valid `PluginInfo.ApiName` and
  `WorkflowInfo.WorkflowName` values are preserved; unsafe names use
  deterministic fallbacks `plugin_{plugin_id}_{api_id}` and
  `workflow_{workflow_id}`. Future Tool Registry and MCP adapters must reuse
  the same naming helpers so grant, executable, audit, and UI identifiers do
  not drift. Tool construction, executable registry wiring, denied-grant audit
  events, and Skill/MCP grant snapshots remain open.
- [x] M3.11 complete: production ADK executor wiring now uses
  `NewDefaultADKToolProviderWithSingleAgentSubagents` with
  `singleAgentSVC.DomainSVC`, so lead runs resolve runtime tools,
  human-interaction tools, and SingleAgent-backed `AgentTool` subagents through
  one outer `ADKToolPolicyProvider`. The helper keeps the old default provider
  shape when no SingleAgent source exists, and child SingleAgent agents use the
  default non-recursive provider filtered by their generated child
  `tool_policy`. Durable AgentHub references, child lifecycle rows,
  denied-grant audit events, and Tool Registry executable adapters remain
  open.
- [x] M3.12 complete: `ADKSubagentToolProvider` can now wrap child `AgentTool`
  invocations with content-free lifecycle events:
  `subagent.run.started`, `subagent.run.completed`, and
  `subagent.run.failed`. Production default provider wiring passes the ADK
  event sink into this wrapper, so lifecycle events persist through the parent
  run's `agent_run_events` stream. Payloads include standardized `subagent`
  identity, agent/version IDs, status, elapsed time, and sanitized error
  summaries while excluding tool arguments and model output. Durable child run
  rows, status cards, cancellation terminal classification, and audit
  enrichment remain open.
- [x] M3.13 complete: `agent_runs` now has `parent_run_id` and `run_kind` so
  durable child rows can be represented without polluting top-level task run
  lists. Repository `ListRuns` defaults to `parent_run_id=0`, explicit
  `ParentRunID` queries can list children, and worker claim paths skip child
  rows. Domain/application mappings preserve `RunKind` and `ParentRunID`, and
  service validation rejects invalid parent/thread combinations. Creating
  child rows from `AgentTool` lifecycle events, linking child row IDs to
  events, and UI child-run cards remain open.
- [x] M3.14 complete: `ADKSubagentRunRecorder` and
  `ApplicationADKSubagentRunRecorder` now create durable `run_kind='subagent'`
  child rows when `AgentTool` execution starts and transition them to
  succeeded/failed on terminal outcome. Production default provider wiring
  passes the recorder alongside the event sink, lifecycle events include
  `child_run_id`, and recorder-only operation still works without an event
  sink. Child-run UI cards, retry/resume semantics, and usage rollups remain
  open.
- [x] M3.15 complete: subagent child invocation errors are now classified
  before lifecycle events and durable child-run terminal updates. Cancellation
  maps to `RunStatusCanceled`, `subagent.run.canceled`, and
  `subagent_canceled`; timeout maps to `RunStatusFailed`,
  `subagent.run.failed`, and `subagent_timeout`; generic failures remain
  failed with `subagent_failed`. Parent-to-child cancellation propagation,
  terminal race protection, child retry/resume semantics, UI cards, and usage
  rollups remain open.
- [x] M3.16 complete: workbench task-thread run listing now accepts
  `parent_run_id` and returns `parent_run_id` plus `run_kind` for each run.
  The default list remains top-level-only, while task detail UI can explicitly
  fetch durable `run_kind='subagent'` child rows under a parent run. Frontend
  API schema types were synced for the new query and response fields. UI state
  cards, child event joins, retry/resume semantics, and usage rollups remain
  open.
- [x] M3.17 complete: canonical task detail now loads top-level runs, fetches
  child `run_kind='subagent'` rows with `parent_run_id`, and renders a
  content-free `子智能体执行` section with subagent name, durable status, sanitized
  error summary, assistant identity fallback, and update time. Child event
  joins, per-child token/cost rollups, retry/resume controls, and browser
  visual QA remain open.
- [x] M3.18 complete: subagent state cards now join latest
  `subagent.run.*` lifecycle events by `child_run_id`, showing content-free
  duration (`elapsed_ms`) and terminal classification while keeping the child
  run row as the primary status source. Expandable child timelines, per-child
  token/cost rollups, retry/resume controls, and browser visual QA remain
  open.
- [x] M3.19 complete: subagent state cards now fetch run-scoped token usage
  through the existing task-thread token usage API for each child run and
  render total tokens plus call count. Backend parent/child usage rollup APIs,
  cost/pricing snapshots, per-child provider/model attribution, retry/resume
  controls, and browser visual QA remain open.
- [x] M3.20 complete: subagent state cards now expose an expandable child
  lifecycle timeline grouped from parent-run `subagent.run.*` events by
  `child_run_id`. Timeline rows display content-free event titles, status,
  timestamps, elapsed time, terminal classification, and sanitized lifecycle
  errors only. Backend parent/child usage rollup APIs, cost/pricing snapshots,
  retry/resume controls, and browser visual QA remain open.
- [x] M3.21 complete: task-thread token usage now supports explicit
  `include_child_runs=true` on a parent `run_id`, returning rows and aggregate
  usage for that parent run plus direct subagent child runs while keeping
  historical run-scoped queries unchanged. Cost/pricing snapshots,
  per-child provider/model attribution, retry/resume controls, recursive
  descendant rollups, and browser visual QA remain open.
- [x] M3.22 complete: token usage responses now include optional
  `run_aggregates` per-run aggregate metadata, and canonical task detail uses
  the parent `include_child_runs=true` response to populate child subagent
  token cards without one request per child run. Cost/pricing snapshots,
  provider/model attribution display, retry/resume controls, recursive
  descendant rollups, and browser visual QA remain open.
- [x] M3.23 complete: canonical task detail now derives per-child subagent
  provider/model attribution from grouped token usage rows and renders a
  bounded `provider / model_name` label on child cards. Cost/pricing
  snapshots, retry/resume controls, recursive descendant rollups, and browser
  visual QA remain open.
- [x] M3.24 complete: child subagent cards now de-duplicate distinct
  provider/model attribution labels per child run and display the first label
  with a bounded `+N` suffix when additional labels exist. The card remains a
  metadata-only summary and does not expose raw usage rows, provider payloads,
  prompts, completions, tool data, or object identifiers. Cost/pricing
  breakdowns, retry/resume controls, recursive descendant rollups, and browser
  visual QA remain open.
- [x] M3.25 complete: child subagent cards now display a bounded cost snapshot
  from backend-provided `cost_micros` only when the child run has a positive
  aggregate cost and exactly one non-empty currency across its usage rows.
  Missing or mixed currency omits cost display, and pricing calculation remains
  backend-owned. Rich pricing breakdowns, retry/resume controls, recursive
  descendant rollups, and browser visual QA remain open.
- [x] M3.26 complete: Workbench task-thread API now has a backend subagent
  retry contract. Failed or canceled child `run_kind='subagent'` rows can be
  retried through
  `POST /api/workbench/task_threads/:thread_id/runs/:run_id/retry`, which
  creates a new top-level queued task run with a content-free
  `subagent_retry` command and a `subagent.retry.requested` event while leaving
  the historical child row immutable. Executor replay of that command,
  frontend retry controls, retry-result joins, recursive descendant rollups,
  and browser visual QA remain open.
- [x] M3.27 complete: Workbench run processing now recognizes queued
  `command.subagent_retry` runs and fails them closed with
  `error_code='subagent_retry_not_supported'` before invoking the normal
  executor. This prevents retry placeholder runs from silently executing as
  ordinary empty-message tasks while the real replay executor is still open.
  Frontend retry controls, retry-result joins, recursive descendant rollups,
  real replay execution, and browser visual QA remain open.
- [x] M3.28 complete: child subagent recording now captures the internal
  `AgentTool` invocation arguments in versioned `coze.subagent_tool_call.v1`
  run input so future retry replay has the original child call payload.
  Workbench API mapping redacts `command`, `input`, `config`, and `context`
  for `run_kind='subagent'` rows so those internal replay fields are not
  exposed to task detail clients. Frontend retry controls, retry-result joins,
  recursive descendant rollups, real replay execution, and browser visual QA
  remain open.
- [x] M3.29 complete: the run processor now treats `subagent_retry` as an
  executor capability boundary. Executors that implement
  `SubagentRetryRunExecutor` receive `ExecuteSubagentRetry` and reuse the
  normal append-message, completion, cancellation, interrupt, and failure
  mapping path. Executors without that capability still fail closed with
  `subagent_retry_not_supported`, so retry placeholders cannot fall through to
  ordinary empty-message execution. ADKExecutor source-child replay, frontend
  retry controls, retry-result joins, recursive descendant rollups, and browser
  visual QA remain open.
- [x] M3.30 complete: child subagent run config now preserves the replay
  definition snapshot required by future retry execution: runtime, agent name
  and description, SingleAgent reference, `full_chat_history`, and
  `tool_policy.allowed_tools` / `tool_policy.allowed_dynamic_tools`. Combined
  with M3.28 replay input, future ADKExecutor replay can rebuild the original
  child call without broadening tool grants. ADKExecutor source-child replay,
  frontend retry controls, retry-result joins, recursive descendant rollups,
  and browser visual QA remain open.
- [x] M3.31 complete: ADKExecutor now implements source-child subagent retry
  replay behind an injected `ADKSubagentRetrySourceResolver`. It validates the
  retry command against the source child run, parses
  `coze.subagent_tool_call.v1`, rebuilds the child agent from the source config,
  and invokes it through Eino `adk.NewAgentTool` so Eino owns the
  argument-to-message conversion. RuntimeSelector retry routing, recursive
  descendant rollups, and browser visual QA remain open.
- [x] M3.32 complete: RuntimeSelector now implements
  `SubagentRetryRunExecutor`, routes `subagent_retry` execution through the
  same runtime policy as ordinary runs, and sends allowed `eino_adk` retries to
  the ADK executor. Legacy or unsupported selected runtimes return
  `SubagentRetryUnsupportedError`, which RunProcessor maps back to
  `subagent_retry_not_supported` instead of generic `executor_error`.
  Recursive descendant rollups and browser visual QA remain open.
- [x] M3.33 complete: canonical task detail now exposes a `重试` action for
  failed or canceled child subagent cards. The frontend calls the Workbench
  retry API with only the canonical `thread_id` and child `run_id`, holds
  per-row loading state, refreshes detail after success, and shows bounded
  inline errors without exposing child command/input/config/context payloads.
  Recursive descendant rollups and browser visual QA remain open.
- [x] M3.34 complete: canonical task detail now joins top-level
  `subagent_retry` runs back to their source child subagent cards through safe
  retry metadata only. Retry top-level runs are excluded from ordinary child
  run and grouped token-usage parent queries, and cards display only compact
  retry count plus latest retry status. Recursive descendant rollups and
  browser visual QA remain open.

### M4 - Sandbox, Files, Uploads, And Artifacts

1. Sandbox provider interface.
2. Local development provider.
3. Container provider using the existing infrastructure boundary.
4. Kubernetes/provisioner provider.
5. Per-thread uploads/workspace/outputs virtual paths.
6. Implement Coze sandbox adapters for Eino filesystem `Backend`, `Shell`, and
   `StreamingShell`; reuse Eino list, glob, grep, read, write, edit, multimodal
   read, and execute tools.
7. Path, symlink, archive, command, network, quota, and lifecycle controls.
8. Upload conversion and historical-file context.
9. Artifact registration, list, preview, download, present, and citation.
10. TTL cleanup and orphan reconciliation.

Exit gate:

- Executable tools cannot access host paths directly in production, and active
  artifacts cannot execute inline.

Progress:

- [x] M4.1 complete: runtime file registration now enforces the same
  tool-result offload path policy as the ADK offload backend before persisting
  `agent_files`. Registered workspace offload paths must target the active run,
  use only `trunc` or `clear` phase directories, and use a lowercase SHA-256
  digest filename with `.txt`, so crafted callers cannot register arbitrary
  workspace paths or another run's offload file.
- [x] M4.2 complete: runtime file registration now enforces deterministic
  object-key scope before persisting `agent_files`. Object URIs must use
  `agent-runtime/{space_id}/{thread_id}/runs/{run_id}/tool-results/{trunc|clear}/{sha256}.txt`,
  match the validated virtual path's phase and filename, and match the
  authoritative run row's space, thread, and run IDs. The content digest field
  remains separately validated as lowercase SHA-256.
- [x] M4.3 complete: runtime offload reads now resolve an active `agent_files`
  record before reading object storage. The resolve gate revalidates
  workspace kind, active status, space/thread/source-run scope, virtual path,
  object URI, size, digest, and metadata, and the ADK `read_file` path uses
  the resolved object URI. Orphan objects or stale invalid rows are no longer
  readable just because a virtual path can be guessed.
- [x] M4.4 complete: the first durable artifact registry foundation is in
  place. `agent_artifacts` now has migration/HCL coverage plus Go entity and
  repository support for idempotent file-backed upsert and thread/run listing.
  Preview/download APIs, MIME sniffing, active-content disposition, deletion,
  audit, output scanning, and frontend drawer wiring remain open M4 work.
- [x] M4.5 complete: artifact registration now has a domain service that
  loads the backing `agent_files` row, requires active workspace/output files
  in the requested space/thread/run, copies stable file metadata into
  `agent_artifacts`, and assigns a conservative preview mode. HTML, XHTML,
  SVG, and unknown binary content default to download. Public artifact APIs,
  MIME sniffing at read time, response headers, deletion, audit, output
  scanning, and frontend drawer wiring remain open M4 work.
- [x] M4.6 complete: the workbench now exposes a read-only artifact list API
  at `GET /api/workbench/task_threads/:thread_id/artifacts`, with optional
  `run_id` filtering. The endpoint returns task-detail presentation metadata
  and intentionally omits object URIs, storage URLs, file bytes, inline
  previews, and download links. Preview/download content APIs, MIME sniffing,
  active-content response headers, deletion, audit, output scanning, and
  frontend drawer wiring remain open M4 work.
- [x] M4.7 complete: the workbench now exposes a scoped artifact content API
  at
  `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content`.
  The handler accepts `mode=preview|download`, resolves the artifact by
  `thread_id + artifact_id`, reads the server-side object URI from the
  registered row, and returns bytes with `Content-Type`,
  `Content-Disposition`, and `X-Content-Type-Options: nosniff`. Download mode
  always uses attachment; preview mode only uses inline for safe text, raster
  image, and PDF artifacts, while HTML, XHTML, SVG, unknown content, and
  download-only preview modes remain attachment. MIME sniffing, authorization
  expansion, deletion, audit, output scanning, signed links, and frontend
  drawer wiring remain open M4 work.
- [x] M4.8 complete: canonical task-thread detail pages now show a read-only
  `产物` entry backed by `ListTaskThreadArtifacts` and the artifact content
  endpoint. The frontend loads artifact metadata with the thread detail,
  opens safe text/image/PDF previews from blob URLs, and downloads any artifact
  through `mode=download`. The UI does not render object URIs, object storage
  URLs, or inline active content. MIME sniffing, space/admin/share authorization expansion,
  deletion, audit, output scanning, signed links, richer preview transforms,
  and browser E2E coverage remain open M4 work.
- [x] M4.9 complete: artifact content reads now enforce a metadata-backed scan
  gate before object storage access. Explicit unsafe scan statuses
  `pending`, `failed`, `blocked`, `infected`, and `quarantined` deny both
  preview and download reads, while missing historical metadata remains
  compatible and is audited as `unknown`. Successful reads emit a content-free
  `artifact.content.accessed` run event when a thread event sink is configured,
  excluding object URIs, virtual paths, storage URLs, filenames, file bytes,
  prompt text, model output, tool arguments, and checkpoint data. MIME
  sniffing, authorization expansion, scanner workers, quarantine review,
  signed links, richer preview transforms, and browser E2E coverage remain
  open M4 work.
- [x] M4.10 complete: artifact content reads now perform read-time MIME
  sniffing after object storage returns bytes. Response `Content-Type` uses
  the sniffed type for non-empty content, and preview disposition is
  conservative across registered metadata, stored preview mode, and sniffed
  bytes: if any side is download-only, the endpoint returns attachment even
  when `mode=preview`. This prevents safely labeled metadata from rendering
  HTML/SVG-like active bytes inline, and prevents unsafe registered metadata
  from becoming inline merely because bytes sniff as plain text.
  Space/admin/share authorization expansion, scanner workers, quarantine
  review, signed links, richer preview transforms, MIME-specific renderer
  hardening, and browser E2E coverage remain open M4 work.
- [x] M4.11 complete: artifacts now have a soft-delete lifecycle through
  `agent_artifacts.deleted_at` and
  `DELETE /api/workbench/task_threads/:thread_id/artifacts/:artifact_id`.
  Normal list, metadata get, preview, and download reads resolve active rows
  only, while deleted registry rows remain durable for audit, retention,
  replay, and future orphan reconciliation. Successful deletes emit a
  content-free `artifact.deleted` event with metadata only and never remove
  `agent_files` rows or object storage keys. Space/admin/share authorization
  expansion, scanner workers, quarantine review, signed links, restore,
  physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, and browser E2E coverage remain open M4 work.
- [x] M4.12 complete: the task-detail artifact drawer now exposes a
  Popconfirm-backed `删除` action that calls the scoped artifact DELETE
  endpoint with `thread_id + artifact_id` only, disables concurrent artifact
  actions, refreshes `ListTaskThreadArtifacts` after success, and keeps
  failures inside the drawer. The UI continues to avoid object URIs, storage
  URLs, inline active content, and physical cleanup decisions. Authorization
  expansion, scanner workers, quarantine review, signed links, restore,
  physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, and browser E2E coverage remain open M4 work.
- [x] M4.13 complete: artifact list, content, and delete paths now carry the
  viewer ID from Workbench request context into application requests and pass
  through an injectable `ArtifactAuthorizer` before repository listing,
  metadata lookup, object-storage reads, or soft-delete writes. Production
  initialization wires the first owner-only policy with
  `ThreadOwnerArtifactAuthorizer`, and authorization failures map to HTTP 403
  with content-free errors. Space-member/admin/share expansion, scanner
  workers, quarantine review, signed links, restore, physical cleanup, richer
  preview transforms, MIME-specific renderer hardening, and browser E2E
  coverage remain open M4 work.
- [x] M4.14 complete: artifact scan decisions now have a durable write
  boundary. The domain artifact service exposes `UpdateArtifactScanResult`,
  preserves unrelated metadata, normalizes `scan_status`, scanner, scanner
  version, reason, and scanned time, rejects `unknown` as a new write value,
  and updates only active artifact rows. The application layer exposes
  `RecordArtifactScanResult` for future scanner worker or review paths and
  emits content-free `artifact.scan.completed` audit events with
  `schema=coze.artifact_scan.v1`. Scanner worker scheduling/execution,
  quarantine review, fail-closed outage policy, signed links, restore,
  physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, and browser E2E coverage remain open M4 work.
- [x] M4.15 complete: new artifact registration now starts in a durable
  pending scan state. `RegisterArtifact` forces `scan_status=pending`, records
  the bounded scanner name and `scan_requested_at`, ignores caller-provided
  `clean` scan claims, and creates or gets an `agent_artifact_scan_jobs` row
  with idempotency key `artifact_scan:{artifact_id}:{scanner}`. The scan job
  table has scoped IDs, status, attempt/error, availability, and created/updated
  timestamps without storing object URI, virtual path, filename, file bytes, or
  model/tool content. Scanner worker claiming/execution, retry/backoff,
  quarantine review, fail-closed outage policy, signed links, restore,
  physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, and browser E2E coverage remain open M4 work.
- [x] M4.16 complete: artifact scan jobs can now be claimed with durable
  worker leases. `agent_artifact_scan_jobs` has `worker_id`,
  `lease_expires_at`, `started_at`, and `ended_at` fields plus scanner-aware
  pending and lease indexes. `ClaimArtifactScanJobs` defaults scanner, limit,
  and lease TTL, rejects empty worker IDs, claims due pending jobs, reclaims
  expired processing leases, marks rows `processing`, increments attempt count,
  and keeps lease payloads free of object URI, virtual path, filename, file
  bytes, and model/tool content. Scanner execution, succeeded/failed terminal
  updates, retry/backoff, quarantine review, fail-closed outage policy, signed
  links, restore, physical cleanup, richer preview transforms, MIME-specific
  renderer hardening, and browser E2E coverage remain open M4 work.
- [x] M4.17 complete: claimed artifact scan jobs now have terminal update
  boundaries. `CompleteArtifactScanJob` requires a processing job with a
  matching worker and active lease, writes the artifact scan decision through
  `UpdateArtifactScanResult`, then marks the job `succeeded`, clears the lease,
  clears `last_error`, and sets `ended_at`. `FailArtifactScanJob` marks only
  the job `failed`, stores bounded error text, clears the lease, and leaves
  artifact scan metadata unchanged so pending artifacts remain blocked until a
  retry or review path records a real decision. Scanner execution,
  retry/backoff scheduling, quarantine review, fail-closed outage policy,
  signed links, restore, physical cleanup, richer preview transforms,
  MIME-specific renderer hardening, and browser E2E coverage remain open M4
  work.
- [x] M4.18 complete: artifact scan jobs now have an application-level worker
  execution shell. `ProcessArtifactScanJobs` validates configured object
  storage and an injected `ArtifactContentScanner`, claims jobs through the
  domain lease boundary, loads each artifact by scoped IDs, reads the
  server-side object, sends scoped metadata plus bytes to the scanner, and
  completes or fails jobs through the M4.17 terminal service methods.
  Successful completion emits the existing content-free
  `artifact.scan.completed` event. Scanner and storage failures store only
  generic content-free error text and never persist object URI, virtual path,
  filename, file bytes, prompt text, model output, tool arguments,
  credentials, checkpoint bytes, or provider raw bodies. External antivirus
  adapters, retry/backoff, quarantine review, fail-closed outage policy, signed
  links, restore, physical cleanup, richer preview transforms, MIME-specific
  renderer hardening, and browser E2E coverage remain open M4 work.
- [x] M4.19 complete: artifact scan processing is now reachable through a
  disabled-by-default background worker. `ArtifactScanWorker` calls
  `ProcessArtifactScanJobs` with configured scanner, worker ID, batch size,
  lease TTL, and interval, and reports claimed/succeeded/failed/skipped/error
  counts without logging artifact content or storage locations.
  `StartArtifactScanWorkerFromEnv` is wired into application startup, but
  refuses to start unless `ArtifactSVC`, object storage, and an injected
  scanner are configured. No fake clean scanner or permissive fallback is
  installed. External antivirus adapters, retry/backoff, quarantine review,
  fail-closed outage policy, signed links, restore, physical cleanup, richer
  preview transforms, MIME-specific renderer hardening, and browser E2E
  coverage remain open M4 work.
- [x] M4.20 complete: artifact scan now has a concrete opt-in HTTP scanner
  adapter. `AGENT_ARTIFACT_SCANNER_TYPE=http` with
  `AGENT_ARTIFACT_SCANNER_HTTP_URL` configures `InitService` to inject an
  `ArtifactContentScanner`; missing or invalid scanner config leaves the
  scanner nil so the M4.19 worker still refuses to start. The adapter sends
  `coze.artifact_scan_request.v1` with scanner name, scoped IDs, content type,
  size, and base64 artifact content only, rejects object/path/name fields,
  bounds outbound bytes, supports bearer-token auth, rejects non-2xx or invalid
  responses, and accepts only terminal statuses (`clean`, `blocked`,
  `infected`, `quarantined`). Bundled antivirus service deployment,
  retry/backoff, quarantine review, fail-closed outage policy, signed links,
  restore, physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, and browser E2E coverage remain open M4 work.
- [x] M4.21 complete: artifact scan jobs now support retry/backoff without
  changing artifact scan metadata. `RetryArtifactScanJob` requeues a claimed
  processing job only when the worker ID matches and the lease is active,
  setting `status=pending`, clearing worker/lease fields, recording bounded
  generic error text, setting `available_at`, and preserving `attempt_count`
  until the next claim increments it. `ProcessArtifactScanJobs` now accepts
  max attempts and retry backoff; scanner/storage/result errors retry while
  the current attempt is below the max and final-fail at or above the limit.
  The scan worker can read `AGENT_ARTIFACT_SCAN_WORKER_MAX_ATTEMPTS` and
  `AGENT_ARTIFACT_SCAN_WORKER_RETRY_BACKOFF_MS`, defaulting to one attempt.
  Artifacts stay fail-closed because retry and final job failure leave
  `scan_status=pending` unless a real terminal scan decision is recorded.
  Quarantine review, dead-letter UI, scanner service deployment, signed links,
  restore, physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, browser E2E coverage, and the broader fail-closed outage policy
  matrix remain open M4 work.
- [x] M4.22 complete: artifact scan jobs now have a read-only Workbench
  observability API at
  `GET /api/workbench/task_threads/:thread_id/artifact_scan_jobs`. Repository,
  domain, application, handler, and router layers support filtering by run,
  artifact, status, scanner, and pagination. The response exposes only scan job
  metadata and bounded error text, reuses artifact list authorization, and
  omits artifact title, virtual path, object URI, filename, file bytes, raw
  scanner response, prompt/model/tool content, credentials, checkpoint bytes,
  and provider raw bodies. Frontend dead-letter UI, quarantine review actions,
  scanner service deployment, signed links, restore, physical cleanup, richer
  preview transforms, MIME-specific renderer hardening, browser E2E coverage,
  and the broader fail-closed outage policy matrix remain open M4 work.
- [x] M4.23 complete: artifact content reads now use an explicit fail-closed
  scan policy matrix. Only `scan_status=clean` may read bytes; missing,
  invalid, `pending`, `failed`, `blocked`, `infected`, and `quarantined`
  statuses block before object storage access with safe reason codes. Blocked
  reads emit content-free `artifact.content.blocked` events using
  `schema=coze.artifact_access_blocked.v1`, and the Workbench content endpoint
  maps the typed policy error to HTTP `409` with a bounded reason. Manual
  release, quarantine review actions, scanner service deployment, signed
  links, restore, physical cleanup, richer preview transforms, MIME-specific
  renderer hardening, browser E2E coverage, and frontend dead-letter UI remain
  open M4 work.
- [x] M4.24 complete: the task-detail artifact drawer now includes a read-only
  scan-job observability section backed by
  `GET /api/workbench/task_threads/:thread_id/artifact_scan_jobs`. The frontend
  service encodes filters and pagination, the drawer fetches page 1 with 20
  jobs on open, and the UI renders only safe metadata: job ID, status, scanner,
  worker ID, attempt count, run ID, and bounded last error. It keeps loading,
  error, empty, and manual refresh states local to the drawer. Retry actions,
  manual release, quarantine review, scanner service deployment, signed links,
  restore, physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, and browser E2E coverage remain open M4 work.
- [x] M4.25 complete: failed artifact scan jobs can now be manually retried
  from the Workbench API and task-detail artifact drawer. The backend exposes
  `POST /api/workbench/task_threads/:thread_id/artifact_scan_jobs/:job_id/retry`,
  authorizes through the artifact access boundary, and requeues only matching
  `failed` jobs with a CAS update on `id + thread_id + status=failed`. Retry
  preserves attempt count, clears worker/lease/start/end state, records the
  bounded generic retry marker, does not mutate artifact scan metadata, and
  emits content-free `artifact.scan_job.retry_requested`. The frontend shows a
  row-level retry action only for failed scan jobs and refreshes the queue
  after success. Manual release, quarantine review, scanner service
  deployment, signed links, restore, physical cleanup, richer preview
  transforms, MIME-specific renderer hardening, and browser E2E coverage
  remain open M4 work.
- [x] M4.26 complete: artifact scan status now has a narrow human-review
  override through
  `POST /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/scan_review`
  and the task-detail artifact drawer. The backend authorizes with
  `ArtifactAccessOperationReview`, accepts only `release`, `quarantine`, and
  `block`, maps them to `clean`, `quarantined`, and `blocked`, and writes
  metadata through the existing scan-result path with
  `scan_scanner=manual_review`. Review responses return only a receipt, and
  successful reviews emit content-free `artifact.scan.reviewed` without human
  reason text, artifact title, virtual path, object URI, filename, or bytes.
  The frontend shows scan status tags and row-level `放行` / `隔离` / `阻断`
  actions for non-clean artifacts, then refreshes the artifact list. Review
  does not bypass the content endpoint; artifact bytes remain readable only
  when the final scan status is `clean`. Scanner service deployment, signed
  links, restore, physical cleanup, richer preview transforms, MIME-specific
  renderer hardening, browser E2E coverage, and fail-open policy remain open
  M4 work.
- [x] M4.27 complete: artifact scanner deployment now has sanitized config and
  worker startup diagnostics. `NewArtifactContentScannerFromEnvWithStatus`
  reports enabled/type/configured/error without secrets; `InitService` stores
  the status on `ApplicationService.ArtifactScannerStatus`; and
  `StartArtifactScanWorkerFromEnvWithStatus` reports whether the worker
  started or why it refused to start. Invalid scanner env still leaves
  `ArtifactScanner` nil and prevents the worker from starting, preserving the
  existing fail-closed behavior. Docker env examples now document the scanner
  and artifact scan worker keys with scanning disabled by default. A bundled
  antivirus/scanner service, signed links, restore, physical cleanup, richer
  preview transforms, MIME-specific renderer hardening, browser E2E coverage,
  and fail-open policy remain open M4 work.
- [x] M4.28 complete: artifact scanning now has a concrete opt-in clamd path.
  The Go scanner factory accepts `AGENT_ARTIFACT_SCANNER_TYPE=clamd` or
  `clamav`, builds a clamd `INSTREAM` adapter from
  `AGENT_ARTIFACT_SCANNER_CLAMD_ADDR`, timeout, and max-byte env, maps clamd
  `OK` to `clean`, maps `FOUND` to `infected` with a bounded signature reason,
  and treats connection failures, clamd errors, malformed responses, and
  oversized content as scanner errors. Docker compose files now include an
  optional `artifact-scanner` service under the `scanner` profile, while env
  examples still keep scanning and the scan worker disabled by default. Signed
  links, restore, physical cleanup, richer preview transforms,
  MIME-specific renderer hardening, browser E2E coverage, and fail-open policy
  remain open M4 work.
- [x] M4.29 complete: artifact content reads now have an explicit scanner
  outage fail-mode. `AGENT_ARTIFACT_SCAN_OUTAGE_FAIL_MODE` defaults to
  `closed`, preserving the M4.23 rule that only `scan_status=clean` may read
  bytes. The only non-closed mode is `open_non_executable`; it may override
  only `unknown`, `pending`, and `failed` for artifacts whose stored preview
  mode and stored content type both classify as text or raster image.
  `blocked`, `infected`, `quarantined`, PDF, HTML, XHTML, SVG, octet-stream,
  download-only, and unsupported artifacts remain fail-closed. Override access
  events may include `scan_policy_mode`, `scan_policy_reason`, and
  `scan_policy_override`, and remain content-free. Signed links, restore,
  physical cleanup, richer preview transforms, MIME-specific renderer
  hardening, and browser E2E coverage remain open M4 work.
- [x] M4.30 complete: artifact restore now closes the soft-delete lifecycle.
  The repository restores only rows scoped by `thread_id + artifact_id` with
  `deleted_at > 0`, clears `deleted_at`, updates `updated_at`, and does not
  mutate runtime files, object storage, scan jobs, or artifact bytes. The
  Workbench exposes
  `POST /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/restore`
  with restore authorization and a receipt-only response. Successful restores
  emit content-free `artifact.restored` audit events, and the task artifact
  drawer shows an immediate `撤销移除` action after delete. Physical cleanup,
  signed links, richer preview transforms, MIME-specific renderer hardening,
  and browser E2E coverage remain open M4 work.
- [x] M4.31 complete: deleted artifact browsing now closes the older
  soft-delete restore gap. The Workbench artifact list endpoint keeps default
  active-only behavior and accepts explicit `deleted_only=true` for
  `deleted_at > 0` rows. Artifact summaries expose `deleted_at` lifecycle
  metadata while still omitting object URIs and storage URLs. The task artifact
  drawer now has a compact `当前 / 已移除` switch; the removed view loads the
  deleted-only list and exposes only row-level restore, then refreshes both
  active and removed lists after success. Physical cleanup, signed links,
  richer preview transforms, MIME-specific renderer hardening, and browser E2E
  coverage remain open M4 work.
- [x] M4.32 complete: artifact preview affordances now require MIME/mode
  agreement before the task drawer offers `预览`. The frontend preview helper
  allows text only for safe text/JSON MIME types, image only for raster image
  MIME types, and PDF only for `application/pdf`; HTML, XHTML, SVG,
  octet-stream, missing types, and mismatches are download-only in the drawer.
  Backend scan policy, sniffing, `nosniff`, and attachment disposition remain
  the final content-read authority. Physical cleanup, signed links, richer
  preview transforms, and browser E2E coverage remain open M4 work.
- [x] M4.33 complete: artifact signed URLs now exist for safe previews without
  bypassing Coze's artifact gates. The Workbench exposes
  `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url`
  for `mode=preview` with server-clamped TTL (`60-3600`, default `300`).
  Application signing reuses read authorization, scan read policy, active
  artifact lookup, object storage server-side sniffing, and MIME/mode matching
  before calling the storage `GetObjectUrl` adapter. The task drawer opens
  safe previews through signed URLs. Richer preview transforms and browser E2E
  coverage remain open M4 work.
- [x] M4.34 complete: artifact signed URLs now support forced-attachment
  downloads. `storage.GetOption` carries response-header overrides, and the
  MinIO, S3, and TOS signers map those to provider-supported
  `response-content-disposition` and `response-content-type` parameters.
  `mode=download` still runs read authorization, scan policy, active artifact
  lookup, and server-side MIME sniffing before signing; it forces
  `attachment; filename*=UTF-8''...` from server-side artifact metadata and
  returns only the receipt payload. The task drawer now uses signed URLs for
  both safe `预览` and `下载`, while the backend content endpoint remains as a
  compatibility path with `nosniff` and server-side `Content-Disposition`.
  Richer preview transforms and browser E2E coverage remain open M4 work.
- [x] M4.35 complete: artifact physical cleanup now has a backend-only,
  retention-based cleanup pass. The repository lists only soft-deleted
  artifacts whose `deleted_at` is at or before the cleanup cutoff and whose
  backing `agent_files.status` remains `active`; already-cleaned files and
  active artifacts are skipped. The application deletes the server-side object
  first, then marks the backing file `deleted` by expected `file_id` plus
  `object_uri`; delete failures leave the file active for retry, while
  `storage.ErrObjectNotFound` is treated as idempotent success. Cleanup audit
  events remain content-free and omit object URI, virtual path, object keys,
  filenames, file bytes, scanner raw responses, prompt/model text, tool data,
  credentials, checkpoint bytes, and provider raw bodies. Scheduling,
  retention policy UI, prefix-sweep reconciliation, and browser E2E coverage
  remain open M4 or production acceptance work.
- [x] M4.36 complete: safe text-like artifact previews now render inline in
  the task drawer through the existing Workbench content endpoint with
  `mode=preview`. The frontend detects only `text/plain`, `text/markdown`,
  `application/json`, `text/csv`, and `text/tab-separated-values` as inline
  preview candidates, then applies bounded local transforms: JSON
  pretty-printing, Markdown/source text as escaped React text, and CSV/TSV as
  a Coze Design `Table` capped at 50 data rows and 12 columns. Text rendering
  is capped at 128 KiB and never uses `dangerouslySetInnerHTML`. HTML, XHTML,
  SVG, octet-stream, missing content types, and MIME/mode mismatches remain
  download-only. Downloads and non-text safe preview families continue to use
  signed URLs. Browser E2E coverage, image/PDF drawer embedding,
  MIME-specific renderer hardening, renderer sandboxing, and object-store
  reconciliation remain open production acceptance work.
- [x] M4.37 complete: the task artifact drawer now exposes a stable,
  content-free selector contract for future browser E2E automation. The entry
  button uses `task-artifacts-open`, visible artifact rows use
  `task-artifact-item` plus safe `data-artifact-id`, and inline preview
  surfaces use `task-artifact-inline-preview`,
  `task-artifact-inline-preview-text`, and
  `task-artifact-inline-preview-table`. The existing task detail test asserts
  these selectors. This does not add a runnable Playwright/Cypress harness,
  browser visual snapshots, mobile viewport checks, image/PDF renderer checks,
  or CI browser execution; those remain production acceptance work.
- [x] M4.38 complete: frontend artifact preview family checks are now
  centralized through `artifactPreviewFamily`. `canPreviewArtifact` and
  `artifactInlinePreviewKind` both delegate to that positive allow-list, so
  preview button visibility and inline renderer routing cannot drift silently.
  Tests cover parameterized and case-insensitive safe MIME types plus active
  content rejections for HTML, XHTML, SVG, octet-stream, empty content types,
  and SVG/image mismatches. Backend content and signed URL endpoints remain
  the authoritative security boundary. Browser E2E, image/PDF drawer
  embedding, renderer sandboxing, and CI browser execution remain production
  acceptance work.
- [x] M4.39 complete: raster image artifact previews now render inside the
  task drawer. Image-family previews request the existing signed URL endpoint
  with `mode=preview` and place the returned short-lived URL only in an
  `<img src>` with lazy loading, no referrer, bounded height, and
  `object-fit: contain`. Text previews continue to use the content endpoint,
  downloads continue to use signed attachment URLs, and PDF previews keep the
  signed URL new-tab behavior. SVG, HTML, XHTML, octet-stream, and MIME/mode
  mismatches remain download-only. PDF embedding, browser E2E, mobile visual
  checks, and renderer sandboxing remain production acceptance work.
- [x] M4.40 complete: PDF artifact preview behavior is locked to signed URL
  new-tab previews. PDF preview actions request the existing signed URL
  endpoint with `mode=preview`, clear any stale inline drawer preview, and
  open with `window.open(url, '_blank', 'noopener,noreferrer')`. PDFs are not
  rendered inside the drawer, in iframes, as visible signed URLs, or in
  selector attributes. PDF embedding, sandboxing, browser E2E, mobile viewport
  checks, and CI browser execution remain production acceptance work.
- [x] M4.41 complete: preview actions now clear stale inline preview state
  before routing. A failed text preview after an image preview leaves the
  preview area empty and shows the bounded drawer error instead of keeping the
  old image visible. Download actions do not clear the current preview. Retry
  UI, browser E2E, signed URL refresh, PDF embedding, and mobile visual checks
  remain production acceptance work.

### M5 - Skills Complete Parity

1. Create, enable/disable, delete, edit, history, rollback, import, and export.
2. Public/custom/bootstrap category behavior.
3. Slash activation and deterministic prompt cleanup.
4. Implement an Eino Skill backend backed by Coze's durable Skill/version
   storage.
5. Use Eino Skill middleware for progressive metadata-only discovery followed
   by full `SKILL.md` loading.
6. Map Coze execution policy to Eino `inline`, `fork`, and
   `fork_with_context` modes.
7. Skill resource virtual mounts.
8. Allowed-tool policy.
9. Skill management/evolution with confirmation and audit.
10. Skill scanner and quarantine workflow.

Exit gate:

- Skills are durable, sandbox-readable, policy-constrained, checkpoint-safe,
  executable through the Eino Skill runtime, and manageable entirely from the
  UI.

- [x] M5.1 complete: the skill configuration list now exposes row-level
  `启用` / `停用` actions backed by the existing `UpdateSkill` contract. The
  frontend sends the current skill snapshot with only `enabled` inverted,
  refreshes the list after success, and keeps errors in the page-level error
  surface.
- [x] M5.2 complete: the toolbar now separates `创建技能` from `导入技能`.
  `创建技能` opens a basic `新建技能` panel and calls the existing
  `CreateSkill` contract with a complete enabled `CustomSkill` payload:
  trimmed name and description, version `1.0.0`, empty schema/executor JSON,
  and default closed permissions. The list reloads after success.
- [x] M5.3 complete: the version management drawer now exposes a `基础信息`
  editor for name, description, and version. Saving uses the existing
  `UpdateSkill` contract with a complete current Skill snapshot, preserves
  type, enabled state, schemas, executor, and permissions, reuses the drawer
  notice/error surface, and triggers parent list refresh. Delete, category
  management, runtime Skill middleware integration, resource mounts,
  allowed-tool policy, scanner/quarantine, and browser E2E remain open M5
  work.
- [x] M5.4 complete: the skill list toolbar now exposes explicit category
  filters: `全部技能`, `自定义`, `公共`, `内置`, `脚本`, and `工作流`.
  Filtering remains client-side against loaded `ListSkills` rows and maps
  `内置` to `DeerSkill` bootstrap records. Delete, server-side category
  queries, marketplace behavior, permissions and allowed-tool editing,
  runtime Skill middleware integration, resource mounts, scanner/quarantine,
  and browser E2E remain open M5 work.
- [x] M5.5 complete: Skill deletion now uses production soft-delete semantics.
  `skills.deleted_at` is the lifecycle marker, default repository reads and
  updates are active-only, `DeleteSkill` disables the row while preserving
  `skill_versions` and `skill_resources`, and normal version/resource/export
  paths gate on an active Skill so deleted content is not exposed through
  default APIs. The frontend exposes a confirmed row-level `删除` action,
  calls `DELETE /api/workbench/skills/:skill_id` with only `skill_id`, clears
  stale test-run state, and reloads the list. Restore, deleted-only browsing,
  hard-delete retention cleanup, marketplace uninstall semantics, audit,
  runtime Skill middleware integration, resource mounts, scanner/quarantine,
  and browser E2E remain open M5 or production acceptance work.
- [x] M5.6 complete: the Skill version drawer now has a `权限配置` editor
  backed by the existing full-snapshot `UpdateSkill` contract. It edits
  `permissions.network` through a controlled checkbox and
  `permissions.allowed_tools` through newline/comma-separated input, preserves
  unknown permission keys, deduplicates tool names, validates Eino-safe names
  before save, refreshes the parent list, and keeps errors in the drawer
  banner. Tool Registry pickers, MCP grants, backend policy audit, runtime
  Skill middleware enforcement, resource mounts, scanner/quarantine, and
  browser E2E remain open M5 or production acceptance work.
- [x] M5.7 complete: Skill permission editing now consumes a backend-owned
  metadata-only tool candidate API at
  `GET /api/workbench/skills/tool_candidates?space_id=...`. The static seed
  list exposes current ADK built-in grant names (`web_fetch`, `web_search`,
  `ask_user_clarification`, `request_human_confirmation`) with display name,
  description, category, and visibility only. The version drawer loads these
  candidates and renders compact buttons that toggle names into
  `permissions.allowed_tools` through the same validation/serialization path
  as manual text editing. Registry-backed candidates, MCP grants,
  per-space authorization, health/status metadata, runtime Skill middleware
  enforcement, resource mounts, scanner/quarantine, and browser E2E remain
  open M5/M6 or production acceptance work.
- [x] M5.8 complete: Skill tool candidates now have a provider boundary and
  optional source metadata so configured MCP tools can appear in the same
  permission editor without exposing executable configuration. The Skill
  application normalizes every candidate, requires Eino-safe grant names and
  non-empty descriptions, bounds metadata, and drops duplicate names with
  built-ins winning. The Workbench MCP service maps enabled server tools to
  stable grant names such as `mcp_100_search_docs`, skips disabled or
  incomplete definitions, and is injected into the Skill service at
  application bootstrap. The frontend shows MCP server source metadata on
  candidate buttons and still saves only `permissions.allowed_tools`. Durable
  MCP persistence, encrypted/masked secrets, health/status badges, Tool
  Registry authorization, runtime Eino MCP adapter execution, audit,
  scanner/quarantine, resource mounts, and browser E2E remain open M6 or
  production acceptance work.

### M6 - MCP And Unified Tools

1. Durable MCP server/tool models.
2. Secret encryption and masked round trips.
3. stdio, SSE, and streamable HTTP clients, wrapped as Eino MCP tools.
4. OAuth lifecycle for remote MCP.
5. Tool discovery/cache reset and health status.
6. Thread-scoped persistent session pool and eviction.
7. stdio command allowlist and sandbox execution.
8. Unified Tool Registry for builtin, Coze plugin/workflow, knowledge,
   database, skill, MCP, and deferred tools.
9. Tool policy UI and runtime selection.
10. Preserve extension points for MCP resources, prompts, sampling, and
    elicitation without making them initial parity gates.

- [x] M6.1 complete: Workbench MCP tool server configuration now has a
  MySQL-backed catalog. The production application bootstrap uses
  `mcptool.NewMySQLCatalog(DB)` instead of the in-memory catalog, while tests
  can still install `NewInMemoryCatalog()`. The durable table
  `mcp_tool_servers` stores server identity, space scope, display metadata,
  enabled state, JSON `config`, JSON `auth`, JSON `tools`, timestamps, and a
  `deleted_at` future soft-delete marker. The existing Workbench MCP APIs and
  the M5.8 Skill candidate provider continue to read through the same catalog
  interface. Secret encryption/masking, delete/restore lifecycle, normalized
  Tool Registry indexing, health checks, OAuth/session lifecycle, stdio
  sandboxing, runtime Eino MCP adapter invocation, authorization, audit,
  timeout/output budgets, and browser E2E remain open M6 or production
  acceptance work.
- [x] M6.2 complete: Workbench MCP auth is now masked at the application
  response boundary for create, list, get, and delete responses. Sensitive keys
  such as token, api key, secret, password, private key, and credential fields
  are replaced with `********`, while raw auth remains inside the catalog
  storage boundary. Updating an existing server with masked auth preserves the
  stored secret fields, and blank auth on existing updates preserves stored
  auth as well. MCP server deletion is exposed through
  `DELETE /api/workbench/mcp_tools/:server_id`; MySQL rows are soft-deleted by
  setting `mcp_tool_servers.deleted_at`, default list/detail/test-call paths no
  longer return deleted rows, and the tools page has a confirmed row-level
  delete action. Encrypted-at-rest secrets, restore/deleted browsing,
  authorization/audit, health status, normalized Tool Registry indexing,
  OAuth/session lifecycle, stdio sandboxing, runtime Eino MCP adapter
  invocation, and browser E2E remain open M6 or production acceptance work.
- [x] M6.3 complete: Workbench MCP server responses now include safe health
  metadata (`health_status`, `health_checked_at`, `health_latency_ms`, and
  bounded `health_error`), persisted in `mcp_tool_servers` and updated to
  `healthy` after successful test calls. Health metadata is content-free and
  must not include test arguments, tool output, config, auth, URLs, object
  keys, provider raw responses, prompts, model text, or checkpoint data. MCP
  registry indexing now has a metadata-only endpoint at
  `GET /api/workbench/mcp_tools/registry_entries?space_id=...`, returning
  enabled non-deleted tools with stable `mcp_{server_id}_{sanitized_tool_name}`
  names, server/tool identity, descriptions, enabled state, and health
  metadata only. Real health probes, health history, unified Tool Registry
  tables, authorization, audit, OAuth/session lifecycle, stdio sandboxing,
  runtime Eino MCP adapter invocation, output budgets, and browser E2E remain
  open M6 or production acceptance work.
- [x] M6.4 complete: MCP registry entries now feed a non-executable ADK
  runtime catalog boundary through `ADKMCPRuntimeToolCatalog` and
  `ADKMCPToolRegistry`. Runs must explicitly set `mcp_tools.enabled` /
  `mcpTools.enabled` before registry entries are resolved; generated tools
  default to deferred visibility, may be narrowed by adapter-local
  `allowed_tools`, and are still filtered by the outer `tool_policy` provider.
  Runtime definitions contain only safe names and descriptions, never MCP
  config, auth, input schema, endpoint URLs, object keys, arguments/results,
  prompts, model text, transcripts, checkpoints, or provider raw payloads.
  Invocation currently fails closed with a bounded unsupported-execution error.
  Production ADK bootstrap injects the MySQL-backed Workbench MCP service via
  `WithDefaultADKToolProviderMCPRegistry`, while child SingleAgent factories do
  not inherit parent MCP registry access implicitly. Real MCP transport/session
  execution, Eino MCP schema conversion, OAuth/secrets, stdio sandboxing,
  authorization, audit, output budgets, health probes, frontend policy UI, and
  browser E2E remain open M6 or production acceptance work.
- [x] M6.5 complete: the MCP runtime catalog now has an injectable
  `ADKMCPRuntimeToolExecutor` contract. Runtime tool invocations carry only the
  active run, safe registry name, durable server ID, raw configured MCP tool
  name, and model arguments JSON into the internal executor. Entries missing
  server ID, raw tool name, Eino-safe registry name, or description are not
  exposed as runtime tools. Missing executor behavior remains fail-closed, and
  executor errors are sanitized before reaching the model so arguments, server
  names, config, auth, URLs, object keys, provider diagnostics, prompt/model
  text, transcripts, and checkpoint data are not leaked. Default ADK provider
  wiring now accepts `WithDefaultADKToolProviderMCPExecutor`, but production
  bootstrap intentionally injects only the registry until real Coze-owned MCP
  transport/session, authorization, OAuth/secret, stdio sandbox, audit,
  timeout/output-budget, and health gates are implemented.
- [x] M6.6 complete: `ADKMCPRuntimeExecutor` is now the first Coze-owned MCP
  runtime service boundary behind the executor contract. It resolves durable
  MCP server rows through `ADKMCPRuntimeServerResolver`, validates active run
  space, server ID, raw configured tool name, JSON object arguments, server
  ownership, enabled state, and configured tool presence before transport, and
  rejects unsafe calls without invoking transport. It wraps transport calls
  with timeout and output-byte limits, rejects oversized output without
  exposing output content, and emits content-free `mcp.tool.started`,
  `mcp.tool.completed`, and `mcp.tool.failed` lifecycle events. Workbench MCP
  service now exposes internal `ResolveADKMCPRuntimeServer` with raw
  config/auth for future transport use while public Workbench responses remain
  masked. Real stdio/SSE/HTTP MCP transport, Eino MCP adapter conversion,
  OAuth/secret retrieval, stdio sandboxing, audit persistence, health
  probes/history, output offload, frontend policy controls, and browser E2E
  remain open M6 or production acceptance work.
- [x] M6.7 complete: `ADKMCPRuntimeTransportRouter` now implements the
  fail-closed transport selection layer behind `ADKMCPRuntimeExecutor`. It
  normalizes durable MCP server types for `stdio`, `sse`, `streamable_http`,
  `streamable-http`, and `http`, dispatches only to explicitly installed
  handlers, and returns fixed sanitized errors when a transport is disabled or
  unsupported. The router does not parse config/auth, inspect or transform tool
  arguments, spawn commands, open network clients, emit events, or own timeout
  and output budgets; those stay with Coze-owned executor and future concrete
  transport handlers. Tests cover no-handler fail-closed behavior, handler
  dispatch, unsupported types, and executor-composed sanitized failures. Real
  stdio/SSE/streamable HTTP MCP transport, Eino MCP adapter conversion,
  OAuth/secret retrieval, stdio sandboxing, network policy, audit persistence,
  health probes/history, output offload, production executor wiring, frontend
  policy controls, and browser E2E remain open M6 or production acceptance
  work.
- [x] M6.8 complete: `ADKMCPRuntimeStdioTransport` now provides the first
  stdio-specific handler for the router `Stdio` slot without enabling host
  command execution. It parses bounded stdio config (`command`, string-array
  `args`, string-map `env`, and optional working directory aliases), rejects
  invalid or oversized config with fixed sanitized errors, and requires both
  an injected `ADKMCPRuntimeStdioPolicy` and `ADKMCPRuntimeStdioSandbox` before
  delegation. Policy and sandbox failures are normalized so command paths,
  arguments, env values, workdirs, server names, config/auth, model arguments,
  provider diagnostics, prompt/model text, transcripts, and checkpoints do not
  leak through transport errors. The sandbox call carries only internal run,
  safe tool name, durable server ID, raw configured MCP tool name, model
  arguments JSON, and parsed command config; raw server auth and display name
  are intentionally left out until OAuth/secret projection is designed.
  Concrete stdio sandbox execution, command allow-listing, isolated workdirs,
  environment/secret projection, process/session limits, Eino MCP adapter
  invocation, audit persistence, health classification, output offload,
  production bootstrap wiring, frontend policy controls, and browser E2E remain
  open M6 or production acceptance work.
- [x] M6.9 complete: `ADKMCPRuntimeStdioStaticPolicy` now implements the first
  reusable stdio policy gate behind `ADKMCPRuntimeStdioTransport`. Empty
  options deny all commands; execution requires exact command allow-lists,
  absolute working-directory prefixes, env key allow-lists, and explicit
  args/env budgets. Workdir containment uses cleaned paths plus relative-prefix
  checks so sibling prefixes cannot escape isolation. Env key checks run before
  env count checks for stable policy classification. Policy errors are fixed
  and sanitized, covering command deny, workdir required/not allowed, args
  budget, env not allowed, and env budget without leaking command names, args,
  env keys/values, workdirs, server names, raw config/auth, model arguments,
  provider diagnostics, prompt/model text, transcripts, or checkpoints. Real
  sandbox execution, workdir creation/cleanup, secret projection, process and
  session limits, Eino MCP stdio adapter invocation, audit persistence, health
  classification, output offload, production bootstrap wiring, frontend policy
  controls, and browser E2E remain open M6 or production acceptance work.
- [x] M6.10 complete: `ADKMCPRuntimeStdioSandboxAdapter` now implements the
  stdio sandbox runner contract without enabling host command execution. It
  validates run/thread/space identity, durable server ID, Eino-safe runtime
  tool name, raw MCP tool name, JSON-object arguments, non-empty command, and
  absolute working directory before runner delegation. It projects calls to
  `ADKMCPRuntimeStdioSandboxExecution`, cloning args/env and cleaning workdir,
  then delegates only to an injected `ADKMCPRuntimeStdioSandboxRunner`. Missing
  runner, invalid call, and runner failures return fixed sanitized errors and
  do not leak model arguments, command names/args, env keys/values, workdirs,
  server names, raw config/auth, provider diagnostics, prompt/model text,
  transcripts, or checkpoints. Real process runner implementation, workdir
  creation/cleanup, secret projection, process/session limits, Eino MCP stdio
  adapter invocation, audit persistence, health classification, output offload,
  production bootstrap wiring, frontend policy controls, and browser E2E remain
  open M6 or production acceptance work.
- [x] M6.11 complete: `ADKMCPRuntimeStdioWorkdirManager` now implements a
  deterministic isolated workdir projection boundary without touching the
  filesystem. It requires an absolute root and valid run/thread/space, durable
  server ID, Eino-safe runtime tool name, and raw MCP tool name, then projects
  `{root}/spaces/{space_id}/threads/{thread_id}/runs/{run_id}/servers/{server_id}/tools/{safe_runtime_tool_name}`.
  It re-checks root containment after path cleaning and returns fixed sanitized
  errors for invalid input. `ADKMCPRuntimeStdioTransport` can optionally use
  this manager to overwrite untrusted MCP server `cwd` before static policy
  validation. Directory creation, permissions, cleanup, leases, secret
  projection, process/session limits, Eino MCP stdio adapter invocation, audit
  persistence, health classification, output offload, production bootstrap
  wiring, frontend policy controls, and browser E2E remain open M6 or
  production acceptance work.
- [x] M6.12 complete: `ADKMCPRuntimeStdioFilesystemWorkdirPreparer` now
  implements filesystem-backed prepare/cleanup for projected stdio workdirs
  without command execution. It validates absolute root/workdir containment,
  rejects the root itself as a workdir, creates directories with guarded
  `MkdirAll` and `Chmod`, verifies them with `Stat`, and cleans them with
  guarded `RemoveAll`. `ADKMCPRuntimeStdioSandboxAdapter` can optionally call
  the preparer before runner delegation and attempts cleanup after runner
  success or failure. Errors are fixed and sanitized so workdir paths, root
  paths, command/env/model content, server names, raw config/auth, provider
  diagnostics, transcripts, and checkpoints do not leak. Durable leases,
  retention/recovery, audit persistence, secret projection, process/session
  limits, Eino MCP stdio adapter invocation, health classification, output
  offload, production bootstrap wiring, frontend policy controls, and browser
  E2E remain open M6 or production acceptance work.
- [x] M6.13 complete: `ADKMCPRuntimeStdioDryRunRunner` now implements a
  non-executing stdio sandbox runner for full-chain smoke verification. It
  validates projected execution identity and shape, then returns bounded
  `coze.mcp_stdio_dry_run.v1` JSON containing only safe metadata: schema,
  dry-run status, Eino-safe runtime tool name, durable server ID,
  command/workdir presence booleans, args count, and env count. Direct tests
  cover safe metadata output and invalid execution rejection; composed tests
  cover transport parsing, workdir projection, static policy validation,
  filesystem workdir prepare/cleanup, sandbox delegation, and dry-run output.
  The runner does not start host commands, invoke MCP sessions, call Eino MCP
  adapters, project secrets, write audit records, update health, or offload
  output. Durable workdir leases, stale-run recovery, audit persistence,
  secret projection, process/session limits, Eino MCP stdio adapter invocation,
  health classification, output offload, production bootstrap wiring, frontend
  policy controls, and browser E2E remain open M6 or production acceptance
  work.
- [x] M6.14 complete: `agent_mcp_stdio_workdir_leases` now provides the
  durable internal data boundary for stdio workdir leases. The new
  `MCPRuntimeWorkdirLease` entity and `MCPRuntimeWorkdirLeaseRepository`
  support create, get, finish active lease as `released` or `failed`, worker
  ownership compare-and-set, and expired active lease listing for future
  recovery. Atlas migration `20260624000100_agent_mcp_stdio_workdir_leases.sql`
  and the latest schema table were added, and `atlas.sum` was regenerated with
  the pinned `arigaio/atlas:0.35.0-community-alpine` image. Lease rows may
  store internal workdir paths for cleanup, but these remain internal and must
  not be surfaced through public APIs or content-bearing events. Lease-aware
  preparer integration, stale cleanup worker, audit persistence, secret
  projection, process/session limits, Eino MCP stdio adapter invocation,
  health classification, output offload, production bootstrap wiring, frontend
  policy controls, and browser E2E remain open M6 or production acceptance
  work.
- [x] M6.15 complete: `ADKMCPRuntimeStdioLeasedWorkdirPreparer` now wraps an
  inner stdio workdir preparer and a lease-store interface while preserving the
  sandbox's existing `ADKMCPRuntimeStdioWorkdirPreparer` contract. Prepare
  delegates to the inner preparer, creates an active lease, and attaches lease
  ID/worker ID to the prepared workdir; cleanup releases the lease on success
  or marks it failed with sanitized `cleanup failed` when cleanup fails. Tests
  cover prepare-time lease creation, successful release, and failed cleanup
  marking. Concrete MySQL lease-store adapter, production bootstrap wiring,
  stale cleanup worker, audit persistence, secret projection, process/session
  limits, Eino MCP stdio adapter invocation, health classification, output
  offload, frontend policy controls, and browser E2E remain open M6 or
  production acceptance work.
- [x] M6.16 complete: `ApplicationADKMCPRuntimeStdioWorkdirLeaseStore` now
  implements the stdio workdir lease-store interface using the durable
  `MCPRuntimeWorkdirLeaseRepository`, `idgen.IDGenerator`, configured worker
  ID, and lease TTL. Create validates execution identity and absolute prepared
  workdir, generates a lease ID, and writes an active domain lease with
  `lease_expires_at = now + ttl`; finish maps application release/failure
  states to domain statuses and requires repository CAS success. Repository
  and idgen failures are normalized to fixed sanitized errors. Tests cover
  create mapping, finish mapping, and error redaction. Production dry-run
  bootstrap wiring, stale cleanup worker, audit persistence, secret projection,
  process/session limits, Eino MCP stdio adapter invocation, health
  classification, output offload, frontend policy controls, and browser E2E
  remain open M6 or production acceptance work.
- [x] M6.17 complete: `NewADKMCPRuntimeStdioDryRunTransport` now provides an
  explicit safe-smoke composition point for stdio MCP runtime handling. It
  wires workdir projection, static policy, filesystem prepare/cleanup, durable
  lease-store adapter, leased preparer, dry-run runner, sandbox, and stdio
  transport. Tests cover the full chain from stdio config parsing through
  durable lease creation/release, workdir cleanup, and safe dry-run metadata,
  plus fail-closed behavior when lease persistence is unavailable. This does
  not auto-enable stdio in production bootstrap and still does not execute
  commands or invoke Eino MCP adapters. Production runtime-policy/env wiring,
  stale cleanup worker, audit persistence, secret projection, process/session
  limits, Eino MCP stdio adapter invocation, health classification, output
  offload, frontend policy controls, and browser E2E remain open M6 or
  production acceptance work.
- [x] M6.18 complete: MCP runtime production bootstrap now has an explicit
  env opt-in contract. `ADKMCPRuntimeBootstrapConfigFromEnv` keeps MCP runtime
  execution disabled by default, validates dry-run stdio workdir root, worker
  ID, command allow-list, timeouts, output budgets, and stdio policy budgets,
  and `application.Init` injects the optional executor through
  `WithDefaultADKToolProviderMCPExecutor`. The executor uses
  `primaryServices.mcpToolSVC`, durable stdio workdir leases, `infra.IDGenSVC`,
  content-free run events, and `NewADKMCPRuntimeStdioDryRunTransport`. This
  still mounts only the dry-run runner, so no host command execution or real
  Eino MCP adapter invocation is enabled. Tests cover env parsing,
  incomplete-config rejection, default-off nil executor, and dry-run invocation
  through the lease path. Stale cleanup worker, audit persistence, secret
  projection, process/session limits, real Eino MCP stdio adapter invocation,
  SSE/streamable HTTP transports, health classification, output offload,
  frontend policy controls, and browser E2E remain open M6 or production
  acceptance work.
- [x] M6.19 complete: `ADKMCPRuntimeStdioWorkdirLeaseReaper` now provides a
  safe one-shot cleanup boundary for expired active stdio workdir leases. It
  lists expired rows through the durable repository, validates every lease path
  against a configured absolute root, refuses root/sibling/relative paths and
  empty worker IDs, delegates cleanup to the stdio filesystem workdir
  preparer, and only then finishes the lease as `failed` with sanitized
  `stale lease expired` using the original lease worker ID for CAS. Tests cover
  successful cleanup/finish, unsafe path rejection, cleanup failure without
  finish, and repository list error sanitization. Audit persistence, metrics,
  real Eino MCP stdio adapter invocation, health classification, output
  offload, frontend policy controls, and browser E2E remain open M6 or
  production acceptance work.
- [x] M6.20 complete: stale stdio workdir lease cleanup is now mounted behind
  an explicit env-controlled scheduled worker.
  `ADKMCPRuntimeStdioWorkdirLeaseReaperWorker` wraps the M6.19 one-shot reaper,
  exposes `RunOnce` for deterministic tests and future maintenance hooks, and
  starts from `application.Init` only when
  `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ENABLED=true`. The worker requires an
  absolute `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ROOT`, supports batch and
  interval env knobs, and deliberately does not inherit the dry-run runtime
  root automatically. Tests cover worker delegation, sanitized error handling,
  default-disabled startup, missing repository, invalid root, and configured
  worker construction. Metrics/exporter integration, health classification,
  operator-facing diagnostics, real Eino MCP stdio adapter invocation, output
  offload, frontend policy controls, and browser E2E remain open M6 or
  production acceptance work.
- [x] M6.21 complete: MCP runtime lifecycle audit records are now persisted in
  a durable metadata-only table. `agent_mcp_runtime_audit_events` stores only
  audit ID, space/thread/run/server IDs, Eino-safe runtime tool name, lifecycle
  event type, sanitized error code, elapsed milliseconds, output byte count,
  and creation timestamp. `MCPRuntimeAuditRepository` provides create/list
  access, `ApplicationADKMCPRuntimeAuditRecorder` owns ID generation and
  sanitized error mapping, and `ADKMCPRuntimeExecutor` records content-free
  `mcp.tool.started`, `mcp.tool.completed`, and `mcp.tool.failed` audit
  records through an optional recorder hook. Production bootstrap wires the
  durable recorder into the optional MCP runtime executor. Tests cover
  repository persistence/listing, bounded metadata fields, recorder mapping and
  sanitized errors, executor success/failure audit lifecycle, and bootstrap
  audit injection. Admin-only audit browsing, retention policy, metrics,
  OpenTelemetry span linkage, fail-closed audit policy for real MCP execution,
  real Eino MCP stdio adapter invocation, output offload, frontend policy
  controls, and browser E2E remain open M6 or production acceptance work.
- [x] M6.22 complete: MCP runtime health classification now updates the
  existing Workbench MCP server health fields from post-transport terminal
  runtime outcomes. `ADKMCPRuntimeExecutor` reports metadata-only health
  records through an optional reporter hook after successful transport,
  transport failure, or output-budget failure. Pre-transport validation,
  policy, tenant, disabled-server, invalid-argument, missing-tool, missing
  resolver, and missing-transport failures do not mutate server health.
  `mcptool.ApplicationService.RecordRuntimeHealth` maps success to `healthy`,
  maps runtime failure to `unhealthy`, clamps latency, fills checked time, and
  sanitizes failure errors to short codes such as `transport_failed`,
  `output_budget_exceeded`, or `runtime_failed`. Production bootstrap wires the
  reporter into the optional MCP runtime executor. Tests cover executor
  healthy/unhealthy reporting, validation skip behavior, bootstrap injection,
  service health persistence, and unsafe error redaction. SSE/streamable HTTP
  transports, output offload, operator diagnostics and metrics, frontend
  policy controls, and browser E2E remain open M6 or production acceptance
  work.
- [x] M6.23 complete: stdio MCP can now execute through Eino's MCP adapter
  behind the existing Coze-owned runtime shell. `ADKMCPRuntimeStdioEinoRunner`
  implements `ADKMCPRuntimeStdioSandboxRunner`, receives only projected and
  policy-checked sandbox execution, creates an initialized `mcp-go` stdio
  client through an injectable factory, loads the selected target tool through
  `github.com/cloudwego/eino-ext/components/tool/mcp v0.0.8`, invokes it,
  bounds output, and closes the client. The reusable
  `NewADKMCPRuntimeStdioRuntimeTransport` builder keeps dry-run and Eino modes
  on the same workdir projection, static policy, leased workdir, sandbox,
  audit, health, timeout, and output-budget path. Bootstrap remains default-off
  and installs real Eino stdio only when
  `AGENT_THREAD_MCP_STDIO_EINO_ENABLED=true`; dry-run and Eino mode flags are
  mutually exclusive. Tests cover runner success, all sanitized failure
  classes, output budget, bootstrap env parsing, mutual exclusion, and Eino
  transport construction. Session pooling, OAuth lifecycle, encrypted
  credential retrieval, SSE/streamable HTTP transports, metrics/exporter,
  frontend policy controls, and browser E2E remain open M6 or production
  acceptance work.
- [x] M6.24 complete: MCP runtime output offload now handles oversized
  post-transport MCP tool results through the existing Coze-owned runtime
  file backend. `ADKMCPRuntimeExecutor` accepts an optional
  `ADKMCPRuntimeOutputOffloader`; when a result exceeds the inline budget,
  missing offloader preserves fail-closed `output_budget_exceeded`, while a
  configured offloader writes the result through `ADKOffloadBackend`, returns
  a bounded `coze.mcp_runtime_output_offload.v1` notice with virtual path and
  byte-count metadata only, records completed audit output bytes as the
  original result size, and reports runtime health as success. Offload write
  failure maps to fixed `output_offload_failed` and unhealthy health status.
  `application.Init` wires MCP output offload through the same
  `ADKOffloadBackendFactory` used by ADK reduction, so MCP and normal Eino
  tool-result offload share object storage, runtime file registration,
  `read_file` retrieval, and content-free offload events. Session pooling,
  OAuth lifecycle, encrypted credential retrieval, SSE/streamable HTTP
  transports, metrics/exporter, frontend policy controls, and browser E2E
  remain open M6 or production acceptance work.
- [x] M6.25 complete: stdio MCP runtime now supports explicit secret
  projection from internal server auth through config `auth_env`. The stdio
  transport parses `auth_env` as `env_name -> auth_path`, resolves string
  values from raw internal `MCPToolServer.Auth`, lets projected auth values
  override same-name public config env values, and then sends the final env
  map through the existing static policy allow-list and byte/count budgets.
  Invalid projection shape, invalid auth JSON, missing auth field, non-string
  auth value, and invalid env name all fail closed with fixed sanitized errors
  before policy or sandbox execution. Tests cover successful projection,
  collision override, and sanitized failures. OAuth refresh, real KMS/secret
  manager integration, encrypted-row migration/rotation, remote transport
  credentials, session pooling, SSE/streamable HTTP transports,
  metrics/exporter, frontend policy controls, and browser E2E remain open M6
  or production acceptance work.
- [x] M6.26 complete: MySQL-backed MCP auth storage now has an injectable
  at-rest codec boundary. `MySQLCatalog` accepts `MCPAuthCodec` through
  `WithMySQLCatalogAuthCodec`; `Upsert` encodes raw internal auth before
  writing `mcp_tool_servers.auth`, while `Get` and `List` decode stored auth
  before returning internal server records for Workbench masking and runtime
  `auth_env` projection. The default codec is passthrough for compatibility
  and existing rows, and codec output must remain valid JSON for the current
  column type. Encode/decode failures return fixed sanitized errors without
  raw auth, ciphertext, provider diagnostics, KMS IDs, stack traces, or
  secrets. Tests cover encoded storage, decoded internal reads, list decode,
  and sanitized codec failures. Real KMS/secret-manager provider wiring,
  encrypted-row migration/rotation, OAuth refresh, remote transport
  credentials, session pooling, SSE/streamable HTTP transports,
  metrics/exporter, frontend policy controls, and browser E2E remain open M6
  or production acceptance work.

Exit gate:

- Enabled MCP tools can be discovered and called from a real run, and disabled
  or unauthorized tools cannot be reached by crafted requests.

### M7 - Memory, Token Usage, And Settings

1. Long-term fact model with confidence, source, scope, and correction.
2. Async memory update queue, retries, and dead-letter handling.
3. Context-aware retrieval and token budget.
4. Memory list/search/edit/delete/import/export/clear UI.
5. Provider usage extraction from Eino response metadata and callbacks.
6. Lead, subagent, middleware, title, summary, and tool attribution.
7. Pricing/version metadata and estimated-cost markers.
8. Header, per-turn, and debug token views.
9. Consolidated model, mode, reasoning, memory, token, skill, tool, sandbox, and
   runtime settings.
10. Runtime doctor for model, Skill, MCP, sandbox, and provider capability
    diagnostics.

- [x] M7.1 complete: `agent_thread_memories` now has the long-term fact
  metadata foundation: `confidence`, `source_type`, `source_id`,
  `correction_of_memory_id`, and `corrected_at`. Domain memory creation
  validates confidence, trims source fields, keeps non-run scopes thread-level,
  and application/runtime mapping preserves the metadata through summaries,
  checkpoint memory values, and resume parsing. Prompt memory injection remains
  limited to scope/content, and memory extraction, retry/DLQ processing,
  retrieval ranking, user-facing memory management, import/export/clear,
  audit, and browser E2E remain open M7 work.
- [x] M7.2 complete: `agent_memory_flush_jobs` now has durable worker lease
  metadata and a queue lifecycle boundary. Repository and domain service
  methods can claim due pending jobs, reclaim expired processing jobs, complete
  active jobs, retry processing jobs back to pending with bounded sanitized
  errors, and fail jobs into a terminal failed/dead-letter state. Transitions
  use worker ID plus active lease compare-and-set semantics, and application
  summaries expose only lifecycle metadata. Actual memory extraction, debounce
  policy, structured fact upserts/corrections, user-facing memory management,
  retry workers, audit views, and browser E2E remain open M7 work.
- [x] M7.2b complete: memory flush jobs now have an application processing
  boundary and background worker entrypoint. `ProcessMemoryFlushJobs` claims
  leased jobs, loads the durable transcript snapshot through the domain
  service, invokes a configured `MemoryExtractor`, writes structured facts
  through `RememberMemory` with stable transcript-derived source IDs, applies
  a bounded app-layer idempotency check, and completes/retries/fails the job
  through existing lease-aware transitions. `MemoryFlushWorker` is controlled
  by `AGENT_MEMORY_FLUSH_WORKER_*` env vars and refuses to start without a
  configured extractor. Production Eino/model-backed extraction, DB-level
  concurrent upsert uniqueness, debounce policy, memory UI, audit views, and
  browser E2E remain open M7 work.
- [x] M7.2c complete: a model-backed `MemoryExtractor` now reuses the existing
  Eino `model.BaseChatModel` / `ChatModelProvider` path. It builds a bounded
  extraction prompt from transcript snapshot metadata, requires JSON `facts`
  output, parses fenced or plain JSON, normalizes scope/metadata, clamps
  score/confidence, and returns `MemoryExtractionFact` values for the worker
  persistence boundary. Deployment wiring, extractor model usage attribution,
  prompt evaluation fixtures, DB-level concurrent upsert uniqueness, memory UI,
  audit views, and browser E2E remain open M7 work.
- [x] M7.2d complete: model-backed memory extraction is now deployment-wired
  and usage-attributed. `AGENT_MEMORY_EXTRACTOR_*` env vars configure a
  disabled-by-default `ModelMemoryExtractor`, `InitService` attaches it with a
  `ThreadUsageCollector` when enabled, and extractor model calls record
  `TokenUsageSourceMiddleware` usage from `schema.ResponseMeta.Usage`.
  Metadata includes only safe identifiers such as snapshot/run/thread/model
  IDs, usage kind, and idempotency key; transcript text, model output,
  extracted fact content, prompts, provider raw payloads, tool data, URLs, and
  object keys remain excluded. Prompt evaluation fixtures, DB-level concurrent
  upsert uniqueness, memory UI, audit views, and browser E2E remain open M7
  work.
- [x] M7.2e complete: sourced memory facts now have durable source idempotency.
  `RememberMemory` routes only memories with both `source_type` and
  `source_id` through `CreateOrGetMemoryBySource`; ordinary empty-source
  memories still use plain create. The database adds nullable generated
  `source_key` and unique key `(thread_id, source_key)`, so retries and
  concurrent workers cannot duplicate extracted facts while multiple manual or
  unsourced memories remain allowed. Memory UI, prompt evaluation fixtures,
  audit views, and browser E2E remain open M7 work.
- [x] M7.3 complete: `ThreadMemoryProvider` now supports config-driven
  context-aware retrieval through `memory_retrieval` (`limit`,
  `candidate_limit`, `scopes`, `min_confidence`, optional `query`). When no
  explicit query is provided it derives a deterministic query from the latest
  user input, asks the backend for a bounded candidate set, removes corrected
  and low-confidence facts, ranks remaining memories by query overlap and
  confidence-weighted score, and keeps ADK memory middleware as the final
  token-budget injection boundary. Vector retrieval, embeddings, semantic
  ranking, UI memory management, extraction/upsert workers, audit views, and
  browser E2E remain open M7 work.
- [x] M7.4 backend foundation complete: Workbench task-thread memory
  management now has metadata-safe backend APIs under
  `/api/workbench/task_threads/:thread_id/memories` for list/search, full
  snapshot update, soft-delete, and scoped clear. `agent_thread_memories`
  gained `deleted_at`; runtime recall and default management lists exclude
  deleted rows, while management queries use explicit pagination and can opt
  into expired rows. Responses expose only memory content plus bounded
  metadata fields already owned by the memory row. Later M7 slices completed
  restore, audit history, frontend memory management UI, and frontend
  permission affordances; browser E2E remains open M7 work.
- [x] M7.5 frontend management panel complete: canonical task-thread detail
  pages now render `任务记忆` with safe list/search, scope filters, full-snapshot
  edit, soft-delete, and scoped clear wired to the Workbench memory APIs.
  The list view does not render raw metadata JSON or execution-private
  payloads; browser E2E remains open M7 work.
- [x] M7.6 backend restore and audit history complete: Workbench task-thread
  memory management now supports explicit deleted-row listing,
  `POST /api/workbench/task_threads/:thread_id/memories/:memory_id/restore`,
  and metadata-only audit listing at
  `GET /api/workbench/task_threads/:thread_id/memories/audit_events`.
  Successful update, delete, clear, and restore operations write
  `agent_memory_audit_events`; audit payloads contain only event identity,
  thread/run/space/memory/actor IDs, scope, source metadata, affected count,
  and timestamps. Later M7 slices completed frontend restore controls, audit
  timeline UI, and permission controls; browser E2E remains open M7 work.
- [x] M7.7 frontend restore and audit viewing complete: the task-thread detail
  `任务记忆` panel now has an explicit `已删除` toggle, restores deleted rows
  through the Workbench restore API, and opens a metadata-only `记忆审计`
  SideSheet per memory. The frontend keeps deleted rows out of default lists
  and does not render raw metadata JSON or execution-private payloads in audit
  history. Browser E2E remains open M7 work.
- [x] M7.8 generated API schema wiring complete: Workbench task-thread memory
  management is now represented in `idl/workbench/task.thrift` and
  `@coze-studio/api-schema` `workbenchTask` clients for list/search,
  full-snapshot update, soft-delete, scoped clear, restore, and metadata-only
  audit listing. The task page service re-exports those generated clients and
  types instead of extending the old hand-written memory fetch layer. Remaining
  open M7 work is browser E2E.
- [x] M7.9 memory import/export API complete: Workbench task-thread memory
  export now returns bounded schema `coze.task_thread_memories.export.v1`
  payloads through generated `workbenchTask` clients, and import writes through
  the domain `ImportMemories` boundary with source-based idempotency plus a
  metadata-only aggregate `memory.imported` audit event. Remaining M7 work is
  browser E2E.
- [x] M7.10 frontend memory import/export controls complete: canonical
  task-thread detail pages now expose `导出记忆` and `导入记忆` actions in the
  `任务记忆` panel. Export calls the generated `ExportTaskThreadMemories`
  client with a bounded limit and downloads the safe
  `coze.task_thread_memories.export.v1` JSON payload. Import opens a
  SideSheet, validates that schema, strips unknown fields before calling the
  generated `ImportTaskThreadMemories` client, refreshes the list, and shows
  imported/skipped counts without rendering imported raw JSON. Remaining M7
  work is browser E2E.
- [x] M7.11 backend memory authorization complete: Workbench task-thread
  memory list, export, import, update, delete, clear, restore, and audit
  requests now pass session `ViewerID` through the handler/application DTOs and
  authorize through the `MemoryAuthorizer` boundary before touching
  `ThreadSVC`. The default production wiring uses thread-owner authorization,
  keeps `ActorID` as audit identity only, and maps denied memory access to a
  bounded HTTP 403 response.
- [x] M7.12 frontend memory permission affordances complete: canonical
  task-thread detail derives a read-only `任务记忆` panel from current user ID
  and thread creator ID. Read-only viewers can still list/search, export,
  view deleted rows, and open metadata-only audit history, while import,
  edit, delete, restore, and scoped clear are disabled or blocked by handler
  guards. Backend `MemoryAuthorizer` remains the authority; the UI state is a
  UX affordance only. Remaining M7 work is browser E2E.
- [x] M7.13 task-memory E2E selector contract complete: the canonical
  `任务记忆` panel now exposes stable, content-free selectors for the panel,
  toolbar actions, rows, read-only tag, import sheet, import controls, audit
  sheet, and audit rows. Selector attributes carry only stable test IDs plus
  safe memory row identity (`data-memory-id`) and do not expose memory content,
  metadata JSON, source IDs, run IDs, prompts, model text, tool payloads,
  object URIs, or provider raw data. The app still has no runnable
  Playwright/Cypress harness; true browser execution, mobile viewport checks,
  and CI browser jobs remain production acceptance work.

Exit gate:

- Memory and usage remain correct after restart, retry, resume, and subagent
  execution.

### M8 - Phase 2 Security, Observability, And Production Acceptance

M8 is parked as Phase 2 backlog while Phase 1 DeerFlow parity remains
incomplete. Do not continue M8 slices during Phase 1, including Guardrail
archive/retention, Prometheus/OpenTelemetry exporters, complex scanners,
policy UI, storage lifecycle, or load/chaos gates, unless the work is needed
to fix an already-written regression or unblock a DeerFlow parity test.
Completed notes below are historical implementation context and should not be
used as the next active task queue until the Phase 1 exit gate has passed.

1. Guardrail provider contract with allow, deny, confirm, fail-open, and
   fail-closed modes.
2. Static and runtime scanning for skills, MCP, tool calls, files, commands,
   network targets, and secrets.
3. Security decision and audit event persistence.
4. Human confirmation interrupt integrated with run resume.
5. OpenTelemetry tracing across API, worker, model, tool, sandbox, and subagent.
6. Metrics for queues, streams, leases, checkpoints, retries, token/cost, and
   security decisions.
7. Feedback and suggestion APIs and frontend.
8. Migration, reconciliation, retention, backup, rollback, and operator
   runbooks.
9. Load, chaos, security, browser E2E, and SDK compatibility gates.

- [x] M8.1 guardrail provider contract complete: the agentthread application
  layer now defines metadata-only guardrail requests, decisions, providers,
  fail modes, and a chain evaluator. Decisions merge with deny > confirm >
  warn > allow precedence; provider errors fail open or fail closed based on
  request policy, with bounded safe reason codes. Decision metadata is trimmed,
  bounded, and filtered so prompts, model text, tool arguments/results, object
  URIs, URLs, filenames, credentials, checkpoint bytes, raw scanner payloads,
  and provider raw bodies are not echoed. Runtime enforcement, human
  confirmation interrupts, durable security audit tables, scanner adapters,
  UI review, metrics, and OpenTelemetry linkage remain follow-up M8 work.
- [x] M8.2 guardrail audit persistence complete: `agent_guardrail_audit_events`
  now provides the durable metadata-only security decision record. The
  application recorder stores run/thread/space/actor identity, event type,
  target type, redacted safe target ID, operation, source, action, fail mode,
  provider, reason code, sanitized rule IDs, and timestamp, with generic
  sanitized recorder errors. The domain repository supports create/list and
  bounded string persistence, and Atlas migration
  `20260627000100_agent_guardrail_audit_events.sql` plus latest schema were
  added. Runtime enforcement wrappers, scanner adapters, human confirmation
  interrupts, UI review, metrics, OpenTelemetry linkage, retention policy, and
  operator runbooks remain follow-up M8 work.
- [x] M8.3 guardrail enforcement gate complete: `GuardrailEnforcer` now wraps
  the M8.1 provider and M8.2 audit recorder as the runtime-facing decision
  boundary. It normalizes provider decisions, maps provider failures through
  request fail mode, records content-free audit events before returning an
  outcome, allows `allow`, allows `warn` with a warning flag, returns typed
  `GuardrailDeniedError` for `deny`, returns typed
  `GuardrailConfirmationRequiredError` for `confirm`, and fails closed with
  `GuardrailAuditFailedError` when audit recording is missing or fails. Eino
  tool wrapping, scanner adapters, human confirmation checkpoint integration,
  UI review, metrics, OpenTelemetry linkage, and operator runbooks remain
  follow-up M8 work.
- [x] M8.4 guardrail runtime tool wrapper complete:
  `ADKGuardrailRuntimeToolCatalog` now decorates ADK runtime catalog tool
  invokers, and `WithDefaultADKToolProviderGuardrailEnforcer` wires the wrapper
  into the default ADK tool provider. The wrapper preserves tool metadata,
  builds metadata-only Guardrail requests from run identity and runtime tool
  name, excludes tool arguments/results and other content-bearing payloads,
  invokes original tools only for `allow` and `warn`, short-circuits `deny`
  and audit failure with sanitized typed Guardrail errors, and routes
  `confirm` through the M8.6 human-interaction interrupt path.
  This slice covers runtime catalog tools such as MCP and web tools only;
  Skill middleware, subagent tools, scanner adapters, UI review, metrics,
  OpenTelemetry linkage, retention, and operator runbooks remain follow-up M8
  work.
- [x] M8.5 guardrail Skill load boundary complete: `adkSkillBackend.Get` can
  now run metadata-only Guardrail checks before returning full Skill
  instructions, and `ADKMiddlewareAssemblerOptions.GuardrailEnforcer` wires
  the same boundary into the default Eino Skill middleware path. Skill load
  requests include only run/thread/space/creator identity, target type
  `skill`, skill name, operation `load`, source `adk_skill_backend`, and
  fail-closed policy; Skill bodies, catalog JSON, prompts, model text, tool
  payloads, object URIs, URLs, filenames, credentials, checkpoint bytes,
  scanner raw payloads, and provider raw bodies stay out of Guardrail metadata
  and errors. Static Skill scanning, subagent tools, human confirmation
  checkpoint integration, UI review, metrics, OpenTelemetry linkage, retention,
  and operator runbooks remain follow-up M8 work.
- [x] M8.6 guardrail runtime confirmation interrupt complete:
  `ADKGuardrailRuntimeToolCatalog` now converts runtime-tool Guardrail
  `confirm` decisions into Eino `tool.StatefulInterrupt` responses carrying a
  metadata-only `HumanInteractionPrompt` of kind `confirmation` and
  `humanInteractionToolState`. The prompt exposes only safe runtime tool
  identity, action `invoke`, provider, reason code, sanitized rule IDs, a safe
  summary, and default reject guidance. Tool arguments/results, prompts, model
  text, URLs, filenames, object URIs, credentials, checkpoint bytes, scanner
  raw payloads, provider raw bodies, and hidden run config stay out of the
  prompt and error string. Original runtime tools are not invoked while
  confirmation is pending. Full resume approval execution, Skill confirmation
  interrupts, scanner adapters, UI review, metrics, OpenTelemetry linkage,
  retention, and operator runbooks remain follow-up M8 work.
- [x] M8.7 guardrail subagent tool boundary complete:
  `ADKSubagentToolProvider` now accepts
  `WithADKSubagentToolProviderGuardrailEnforcer`, and the default SingleAgent
  subagent provider passes through the shared Guardrail enforcer. The subagent
  wrapper sits outside timeout and lifecycle wrappers, so denied,
  confirmation-required, and audit-failed decisions block before child run rows
  or `subagent.run.*` lifecycle events are created. Requests are metadata-only:
  parent run/thread/space/creator identity, target type `tool_call`, subagent
  tool name, operation `invoke`, source `adk_subagent_tool`, and fail-closed
  policy. Subagent arguments, child prompts, child model input/output, tool
  results, URLs, filenames, object URIs, credentials, checkpoint bytes,
  scanner raw payloads, provider raw bodies, and hidden run config stay out of
  Guardrail metadata, prompts, and errors. `allow` and `warn` proceed through
  the original subagent path, while `confirm` returns a metadata-only Eino
  human-interaction confirmation interrupt. Skill confirmation interrupts,
  scanner adapters, UI review, metrics, OpenTelemetry linkage, retention, and
  operator runbooks remain follow-up M8 work.
- [x] M8.8 guardrail confirmation resume complete for runtime and subagent
  tools: Guardrail wrappers now consume saved `humanInteractionToolState`
  before provider evaluation. Approved `HumanInteractionResponse` data resumes
  and invokes the original guarded runtime or subagent tool once, while
  bypassing a second provider/audit evaluation for the same interrupted call.
  Rejected responses return sanitized `GuardrailConfirmationRejectedError`
  without invoking the guarded tool. Missing resume data or a resume targeted
  at another interrupt re-interrupts with the saved metadata-only prompt.
  Subagent approved resumes enter the inner Eino `AgentTool` through a child
  tool address segment so the inner AgentTool does not consume the outer
  Guardrail state. Resume errors, prompts, and metadata still exclude tool
  arguments, child prompts, model input/output, tool results, URLs, filenames,
  object URIs, credentials, checkpoint bytes, scanner raw payloads, provider
  raw bodies, and hidden run config. Skill confirmation interrupts, scanner
  adapters, UI review, metrics, OpenTelemetry linkage, retention, and operator
  runbooks remain follow-up M8 work.
- [x] M8.9 guardrail audit Workbench query boundary complete:
  `GET /api/workbench/task_threads/:thread_id/guardrail_audit_events` lists
  metadata-only Guardrail audit records with optional `run_id`, `page`, and
  `page_size`, plus a filtered total count. `ApplicationService` owns the
  read boundary through `GuardrailAuditRepository` and
  `ThreadOwnerGuardrailAuditAuthorizer`, so only the task thread creator can
  read that thread's Guardrail audit rows by default. API payloads expose only
  safe identity, event type, target type, redacted safe target ID, operation,
  source, action, fail mode, provider, reason code, sanitized rule IDs, and
  created timestamp. Decision messages, prompts, model input/output, tool
  arguments/results, object URIs, raw URLs, filenames, credentials, checkpoint
  bytes, scanner raw payloads, provider raw bodies, hidden run config, and
  client-controlled identity stay out of the contract. Guardrail audit UI,
  exports, retention policy, metrics, OpenTelemetry linkage, scanner adapters,
  and operator runbooks remain follow-up M8 work.
- [x] M8.10 guardrail pattern scanner provider complete:
  `GuardrailPatternProvider` is the first runtime Guardrail scanner adapter.
  It is disabled by default and can be enabled with
  `AGENT_GUARDRAIL_PROVIDER_TYPE=pattern` plus
  `AGENT_GUARDRAIL_PATTERN_RULES_JSON`. Rules match only metadata-safe
  request fields: target type, target ID, operation, source, and explicitly
  supplied metadata strings. Matches may return `warn`, `confirm`, or `deny`;
  no match returns allow. Invalid explicit env config fails closed through a
  sanitized `guardrail_config_invalid` deny decision. Production ADK assembly
  now passes the env-built enforcer to runtime catalog tools, SingleAgent
  subagent tools, and Skill content loading, all backed by the same
  metadata-only audit recorder. Pattern matching and decisions still exclude
  prompts, model input/output, tool arguments/results, object URIs, raw URLs,
  filenames, credentials, checkpoint bytes, scanner raw payloads, provider raw
  bodies, hidden run config, and client-controlled identity. External scanner
  adapters, policy UI, audit UI, exports, retention policy, metrics,
  OpenTelemetry linkage, and operator runbooks remain follow-up M8 work.
- [x] M8.11 guardrail HTTP scanner provider complete:
  `HTTPGuardrailProvider` adds the external enterprise scanner adapter while
  keeping Guardrail disabled by default. `AGENT_GUARDRAIL_PROVIDER_TYPE=http`
  plus `AGENT_GUARDRAIL_HTTP_URL` wires the provider through the same
  `NewGuardrailProviderFromEnvWithStatus` and `NewGuardrailEnforcerFromEnv`
  path as pattern scanning; optional `AGENT_GUARDRAIL_HTTP_TOKEN` sends a
  Bearer token and `AGENT_GUARDRAIL_HTTP_TIMEOUT_MS` controls request timeout.
  Endpoint config must be `http` or `https` and must not include userinfo.
  The outbound request uses schema `coze.guardrail_scan_request.v1` and sends
  only metadata-safe space/thread/run/user IDs, target type, sanitized target
  ID, operation, source, fail mode, and sanitized metadata. The adapter rejects
  invalid response actions without leaking response bodies, and accepts only
  `warn`, `confirm`, or `deny`; Coze retains no-match/default allow ownership.
  HTTP scanner decisions pass through the shared Guardrail sanitizer before
  audit or interrupt use. Policy UI, audit UI, exports, retention policy,
  metrics, OpenTelemetry linkage, and operator runbooks remain follow-up M8
  work.
- [x] M8.12 guardrail audit frontend panel complete:
  canonical task-thread detail pages now render a `安全审计` panel backed by
  the generated `ListTaskThreadGuardrailAuditEvents` client re-exported from
  the task page service. The panel loads the latest 20 metadata-only audit
  rows, supports manual refresh, renders empty/error/loading states, and shows
  only event/action/target/operation/source/fail-mode/provider/reason/rule
  IDs/run/timestamp metadata. It does not display decision messages, prompts,
  model input/output, tool arguments/results, object URIs, raw URLs,
  filenames, credentials, checkpoint bytes, scanner raw payloads, provider raw
  bodies, hidden run config, or raw metadata JSON. Policy UI, exports,
  retention policy, metrics, OpenTelemetry linkage, and operator runbooks
  remain follow-up M8 work.
- [x] M8.13 guardrail audit current-list export complete:
  the task-detail `安全审计` panel now offers a frontend-only JSON export of
  the currently loaded audit rows with schema
  `coze.task_thread_guardrail_audit.export.v1`. Export rows are rebuilt from a
  safe allow-list containing event/run/thread/space/actor IDs, event type,
  target type, safe target ID, operation, source, action, fail mode, provider,
  reason code, sanitized rule IDs, and created timestamp. The export does not
  include decision messages, prompts, model input/output, tool
  arguments/results, object URIs, raw URLs, filenames, credentials, checkpoint
  bytes, scanner raw payloads, provider raw bodies, hidden run config, raw
  metadata JSON, or unreviewed future API fields. This is not a full-history
  audit export; backend full export, retention policy, metrics, OpenTelemetry
  linkage, and operator runbooks remain follow-up M8 work.
- [x] M8.14 guardrail audit backend paginated export boundary complete:
  `GET /api/workbench/task_threads/:thread_id/guardrail_audit_events/export`
  returns schema `coze.task_thread_guardrail_audit.export.v1` with
  `thread_id`, `exported_at`, normalized `page` / `page_size`, `total`, and
  metadata-only Guardrail audit events. The application service uses distinct
  `GuardrailAuditAccessOperationExport` authorization, optional `run_id`
  filtering, and an export page-size cap of 1000 while reusing the same safe
  event mapper as the list API. IDL, backend API models, route registration,
  generated frontend schema, and task page service re-export were updated.
  The backend export payload still excludes decision messages, prompts, model
  input/output, tool arguments/results, object URIs, raw URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies,
  hidden run config, raw metadata JSON, and client-controlled identity.
  Retention policy, metrics, OpenTelemetry linkage, policy UI, and operator
  runbooks remained follow-up M8 work after this backend slice.
- [x] M8.15 guardrail audit frontend backend-pagination export complete:
  the canonical task-detail `安全审计` export action now calls the generated
  `ExportTaskThreadGuardrailAuditEvents` client instead of serializing only
  the currently loaded rows. The UI keeps the latest 20-row list for display,
  uses Semi/Coze Design Button `loading` for export progress, disables export
  when backend total is zero, and fetches export pages with `page_size=1000`
  until accumulated safe events reach backend `total` or an empty page is
  returned. The downloaded JSON preserves schema
  `coze.task_thread_guardrail_audit.export.v1`, adds normalized `page` /
  `page_size` metadata, parses `rule_ids` into a string array for existing
  consumers, and still excludes decision messages, prompts, model
  input/output, tool arguments/results, object URIs, raw URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies,
  hidden run config, raw metadata JSON, client-controlled identity, and
  unreviewed future API fields. Scheduled archive/export jobs, metrics,
  OpenTelemetry linkage, policy UI, and operator runbooks remain follow-up M8
  work.
- [x] M8.16 guardrail audit retention cleanup complete: the Guardrail audit
  repository now supports bounded batch deletion by `created_at` cutoff, and
  `GuardrailAuditRetentionReaper` computes retention cutoffs before deleting
  one ordered batch at a time. `GuardrailAuditRetentionWorker` is wired into
  application startup but disabled by default through
  `AGENT_GUARDRAIL_AUDIT_RETENTION_WORKER_ENABLED`; example environments
  document a 90-day retention window, 1000-row batches, and a 1-hour interval
  via `AGENT_GUARDRAIL_AUDIT_RETENTION_DAYS`,
  `AGENT_GUARDRAIL_AUDIT_RETENTION_BATCH_SIZE`, and
  `AGENT_GUARDRAIL_AUDIT_RETENTION_INTERVAL_MS`. Cleanup hard-deletes only
  metadata-only audit rows older than the cutoff, returns/logs sanitized
  errors, and still excludes decision messages, prompts, model input/output,
  tool arguments/results, object URIs, raw URLs, filenames, credentials,
  checkpoint bytes, scanner raw payloads, provider raw bodies, hidden run
  config, raw metadata JSON, client-controlled identity, and raw repository
  error details. Scheduled archive/export jobs, legal-hold policy,
  OpenTelemetry exporter wiring, policy UI, and operator runbooks remain
  follow-up M8 work.
- [x] M8.17 guardrail security decision metrics boundary complete:
  `GuardrailEnforcer` now accepts a `GuardrailMetricsCollector` and emits one
  `GuardrailEvaluationMetricsObservation` per runtime decision, including
  audit failures. Observations are content-free and limited to target type,
  operation, source, fail mode, action, provider, sanitized error code,
  allowed/warning/confirmation flags, `audit_recorded`, and elapsed
  milliseconds. `AGENT_GUARDRAIL_METRICS_LOG_ENABLED=false` keeps metric logs
  disabled by default; enabling it wires a logging collector through
  `NewGuardrailEnforcerFromEnv` for deployments prepared to ingest
  per-decision observations. The metrics boundary still excludes
  space/thread/run/user IDs, target IDs, reason codes, rule IDs, decision
  messages, prompts, model input/output, tool arguments/results, object URIs,
  raw URLs, filenames, credentials, checkpoint bytes, scanner raw payloads,
  provider raw bodies, hidden run config, raw metadata JSON, client-controlled
  identity, and raw repository/provider errors. Prometheus and OpenTelemetry
  exporters, scheduled archive/export jobs, legal-hold policy, policy UI, and
  operator runbooks remain follow-up M8 work.
- [x] M8.18 guardrail audit retention metrics and operator runbook boundary
  complete: `GuardrailAuditRetentionWorker` now accepts a
  `GuardrailAuditRetentionMetricsCollector` and emits one content-free
  cleanup observation per run. Observations include only success/failure,
  sanitized error code, cutoff timestamp, deleted row count, configured
  retention days, batch size, and elapsed milliseconds.
  `AGENT_GUARDRAIL_AUDIT_RETENTION_METRICS_LOG_ENABLED=false` keeps retention
  metric logs disabled by default; enabling it wires a logging collector for
  deployments prepared to ingest periodic cleanup observations. The
  `docs/superpowers/runbooks/guardrail-audit-operations.md` runbook documents
  safe enablement, verification, rollback, and incident triage. The metrics
  and runbook boundary still excludes space/thread/run/user IDs, target IDs,
  audit row IDs, reason codes, rule IDs, decision messages, prompts, model
  input/output, tool arguments/results, object URIs, raw URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies,
  hidden run config, raw metadata JSON, raw SQL, raw repository errors,
  client-controlled identity, and archive object locations. Scheduled
  archive/export jobs, Prometheus/OpenTelemetry exporters, and policy UI remain
  follow-up M8 work.
- [x] M8.19 guardrail audit retention legal-hold policy complete:
  `AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED=false` keeps the
  default retention behavior unchanged. When enabled, the retention worker
  skips cleanup before calling the reaper or repository, emits a content-free
  skipped cleanup observation with `skip_reason='legal_hold'`, and returns an
  empty result. Legal hold does not disable the worker or metrics collector,
  so operators can verify the hold is active without deleting rows. The
  legal-hold path still excludes space/thread/run/user IDs, target IDs, audit
  row IDs, reason codes, rule IDs, decision messages, prompts, model
  input/output, tool arguments/results, object URIs, raw URLs, filenames,
  credentials, checkpoint bytes, scanner raw payloads, provider raw bodies,
  hidden run config, raw metadata JSON, raw SQL, raw repository errors,
  client-controlled identity, and archive object locations. Future
  archive/export jobs must honor this policy before destructive cleanup.
- [x] M8.20 guardrail audit archive/export boundary complete:
  `GuardrailAuditRepository.ListGuardrailAuditEventsBefore` now reads
  metadata-only audit rows by `created_at` cutoff with stable
  `created_at ASC, id ASC` pagination and cutoff totals. The new
  `GuardrailAuditArchiveExporter` builds schema
  `coze.guardrail_audit.archive.v1` payloads from one bounded expired batch
  and delegates persistence to a `GuardrailAuditArchiveWriter`. This is a
  non-destructive boundary only: it does not delete rows, start a scheduler,
  bypass legal hold, or bind the archive path to a specific object-storage
  implementation. Archive payloads use the existing safe audit row allow-list
  and exclude decision messages, prompts, model input/output, tool
  arguments/results, object URIs, raw URLs, filenames, credentials,
  checkpoint bytes, scanner raw payloads, provider raw bodies, hidden run
  config, raw metadata JSON, raw SQL, raw repository/provider errors,
  client-controlled identity, and unreviewed future fields. Scheduled archive
  workers, concrete object-storage writers, legal-hold-aware delete
  orchestration, Prometheus/OpenTelemetry exporters, and policy UI remain
  follow-up M8 work.
- [x] M8.21 guardrail audit object-storage archive writer complete:
  `GuardrailAuditObjectStorageArchiveWriter` now writes bounded
  `coze.guardrail_audit.archive.v1` archive payloads as snake_case JSON
  through injected Coze object storage. It returns only a stable `ArchiveID`
  and keeps the internal object key out of public results. Validation and
  storage failures collapse to `guardrail audit archive write failed`, so raw
  object locations, storage provider errors, credentials, prompts, model
  input/output, tool arguments/results, checkpoint bytes, scanner raw
  payloads, provider raw bodies, raw metadata JSON, raw SQL, and hidden run
  config cannot leak through this boundary. Scheduled archive workers,
  legal-hold-aware delete orchestration, storage lifecycle policy,
  Prometheus/OpenTelemetry exporters, and policy UI remain follow-up M8 work.
- [x] M8.22 guardrail audit scheduled archive worker complete:
  `GuardrailAuditArchiveWorker` is now available behind disabled-by-default
  env switch `AGENT_GUARDRAIL_AUDIT_ARCHIVE_WORKER_ENABLED`. It computes a
  cutoff from `AGENT_GUARDRAIL_AUDIT_ARCHIVE_RETENTION_DAYS`, calls
  `GuardrailAuditArchiveExporter` with bounded
  `AGENT_GUARDRAIL_AUDIT_ARCHIVE_BATCH_SIZE`, writes through the M8.21 object
  storage writer, and can emit content-free archive metrics behind
  `AGENT_GUARDRAIL_AUDIT_ARCHIVE_METRICS_LOG_ENABLED`. Application startup now
  wires the archive worker before retention cleanup, but the worker remains
  non-destructive and disabled unless explicitly enabled. The worker honors
  shared `AGENT_GUARDRAIL_AUDIT_RETENTION_LEGAL_HOLD_ENABLED` by skipping
  archive attempts and reporting only `skip_reason='legal_hold'`. Logs and
  metrics are restricted to success, sanitized error code, skip reason, cutoff
  timestamp, archived count, total, stable archive ID, retention days, batch
  size, and elapsed milliseconds. They must not expose object keys, object
  URIs, raw URLs, filenames, credentials, prompts, model input/output, tool
  arguments/results, checkpoint bytes, scanner raw payloads, provider raw
  bodies, hidden run config, raw metadata JSON, raw SQL, raw repository,
  provider, or storage errors. Legal-hold-aware delete-after-archive
  orchestration, storage lifecycle policy, Prometheus/OpenTelemetry exporters,
  and policy UI remain follow-up M8 work.
- [x] M8.23 guardrail audit archive-before-delete orchestration complete:
  retention cleanup can now require a successful archive first via
  `AGENT_GUARDRAIL_AUDIT_RETENTION_REQUIRE_ARCHIVE_ENABLED=true`. When enabled,
  startup requires object storage and builds
  `GuardrailAuditArchiveBeforeDeleteCleaner`, which computes one cutoff,
  archives one bounded batch, validates the returned cutoff and archived event
  IDs, and deletes only those exact archived IDs through
  `DeleteGuardrailAuditEventsByIDs`. This closes the race where deleting by a
  fresh cutoff query after archive could remove rows that were inserted between
  archive and cleanup. Archive failures, missing archived IDs, cutoff
  mismatches, and delete failures fail closed as
  `guardrail audit retention cleanup failed`; raw storage/repository/provider
  errors, archive object locations, prompts, model input/output, tool
  arguments/results, checkpoint bytes, scanner raw payloads, raw metadata JSON,
  raw SQL, hidden run config, and audit row payload content remain excluded.
  The existing legal-hold switch still skips retention cleanup before the
  archive-before-delete cleaner runs. Retention metrics may include archived
  row count and stable archive ID, but must not expose archived event IDs or
  object locations. Storage lifecycle policy, Prometheus/OpenTelemetry
  exporters, and policy UI remain follow-up M8 work.
- [x] M8.24 guardrail Prometheus metrics collector/exporter complete:
  `GuardrailPrometheusMetricsCollector` now implements the decision, archive,
  and retention metrics collector interfaces and can be enabled through
  `AGENT_GUARDRAIL_PROMETHEUS_METRICS_ENABLED`. Existing logging collectors may
  be muxed with Prometheus collectors, so operators can enable logs,
  Prometheus, or both independently. The collector registers aggregate counters
  and latency histograms for decision evaluations, archive attempts/rows, and
  retention attempts/rows, using only low-cardinality metadata labels such as
  action, provider, error code, success, skipped, skip reason, and row kind.
  It deliberately excludes archive IDs, archived event IDs, thread/run/space
  IDs, target IDs, object keys, object URIs, raw URLs, filenames, credentials,
  prompts, model input/output, tool arguments/results, checkpoint bytes,
  scanner raw payloads, provider raw bodies, hidden run config, raw metadata
  JSON, raw SQL, raw repository/provider/storage errors, and audit row payload
  content. This slice registers collectors only; safe HTTP scrape endpoint
  exposure remains platform infra work. OpenTelemetry linkage, storage
  lifecycle/reconciliation, and policy UI remain follow-up M8 work.

Exit gate:

- All production gates below pass in a multi-instance staging environment.

## Production Gates

1. No client-controlled identity or tenant authority.
2. No host execution for production stdio MCP, skill scripts, or bash tools.
3. No run can remain permanently `running` after worker loss.
4. Cancel reaches active model, tool, sandbox, and subagent work.
5. SSE reconnect produces no missing or duplicate logical events.
6. Checkpoint resume produces a single terminal outcome.
7. Every tool call has source, policy decision, audit identity, and trace link.
8. Scanner outage follows the configured fail mode and defaults to fail closed
   for executable content.
9. Files and artifacts pass path, symlink, MIME, size, and active-content
   controls.
10. Token usage is durable and attributed across lead agent, middleware,
    subagent, and tools.
11. JS and Python LangGraph compatibility suites pass.
12. Desktop and mobile task workflows pass browser E2E.
13. Database migration, rollback, retention, and reconciliation are rehearsed.
14. IM Channels are absent from routes, menus, settings, deployment
    dependencies, and acceptance criteria.

## Remaining Work Estimate

With IM Channels excluded and M8 parked into Phase 2, the remaining scope is
approximately:

- Phase 1: finish the DeerFlow parity milestone groups M0 through M7,
  including task conversations/detail, runtime execution semantics, Agents,
  Skills, MCP/tools, sandbox/files/artifacts where required for parity,
  memory, token usage, and settings.
- Phase 2: resume M8 production-hardening only after Phase 1 parity exits, or
  earlier only for regression fixes that block Phase 1.
- Recalculate task count after the next Phase 1 slice is selected, because the
  active backlog should now be measured against DeerFlow-visible workflows
  rather than all production-hardening ideas.

These numbers should be recalculated after M2, M4, and M7 because ADK runtime
semantics, sandbox/provisioner integration, MCP stdio isolation, and memory
retrieval are the largest Phase 1 technical uncertainties.
