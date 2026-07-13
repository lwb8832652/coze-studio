# DeerFlow Backend Agent Runtime Parity Design

**Status:** Approved by the standing mainline rule to follow the recommended
approach without per-step confirmation.

**Baseline:** DeerFlow `5851f8250eb150ca23134c79b11ebc5073ac2789`
(`rollback/pre-cryptography`) and NewX AI
`9da1c9c00b56dc4ce5adec402f7b75280af589dd`.

**Goal:** Make the NewX AI Go-native Agent runtime externally equivalent to
the locked DeerFlow backend Agent behavior while preserving or improving
durability, tenant isolation, safety, and operability.

## 1. Parity Definition

Parity is measured by observable contracts, not by matching Python classes or
copying implementation defects.

- The same task input, mode, model capability, enabled Skills, MCP servers,
  uploads, memory and thread history must expose the same tools, context and
  Agent behavior.
- Run, message, checkpoint, Todo, artifact, token and event projections must
  carry the same user-visible meaning and ordering.
- Interrupt, clarification, cancel, retry, resume and follow-up must reach one
  unambiguous terminal or resumable state.
- DeerFlow defects identified in the locked baseline are not parity targets.
  NewX AI must keep a stronger contract for worker ownership, stale-run
  recovery, authorization, event replay and transactional creation.
- Raw prompts, model output, tool arguments/results, credentials, checkpoint
  bytes and provider payloads remain internal even where DeerFlow exposes more.

## 2. Verified Reference Architecture

The production DeerFlow Gateway does not use
`agents/factory.py::create_deerflow_agent`. Its real path is:

```text
POST /runs or /threads/{thread_id}/runs
  -> gateway.services.start_run
  -> asyncio task run_agent
  -> runtime worker installs thread/run/journal context
  -> make_lead_agent / _make_lead_agent
  -> tool and Skill policy filtering
  -> deferred MCP assembly
  -> langchain.create_agent(ThreadState)
  -> graph astream with checkpointer/store
  -> RunJournal + run/event persistence + SSE bridge
```

Primary references:

- DeerFlow Agent construction:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/lead_agent/agent.py`
- DeerFlow state and reducers:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/thread_state.py`
- DeerFlow run lifecycle:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/runtime/runs/worker.py`
- NewX AI production wiring: `backend/application/application.go`
- NewX AI ADK factory: `backend/application/agentthread/adk_agent_factory.go`
- NewX AI middleware assembly:
  `backend/application/agentthread/adk_middleware.go`

## 3. Current Alignment Assessment

The existing Go runtime is real and uses Eino ADK, but it is not yet fully
aligned. The broad capability audit found one fully aligned area, seven
partially aligned areas, and three areas where NewX AI is stronger. Runtime
and security review also found production-blocking defects outside the feature
matrix.

| Capability | Status | Verified gap |
| --- | --- | --- |
| Model Agent loop | Partial | ADK is opt-in; the server default remains legacy and the legacy planner is only a one-model-step harness. |
| DeerFlow system prompt | Partial | ADK uses `run.config.system_prompt`; there is no authoritative DeerFlow-equivalent lead prompt composition when it is absent. |
| Mode semantics | Partial | Thinking is projected, but `is_plan_mode` is not consumed to gate Plan/Todo and `subagent_enabled` does not produce DeerFlow Ultra semantics. |
| Middleware pipeline | Partial | Several declared entries (`agentsmd`, `policy`, `audit`, `usage`) are reserved no-ops; ordering does not represent the verified DeerFlow hook phases. |
| Tool search | Aligned | Keyword, `+required`, `select:` and max-five behavior are available through Eino; promoted catalog state is not durable across summarization. |
| Skill activation | Partial | Explicit and model-selected Skill loading exists, but Skill package files and in-run Skill evolution do not. |
| MCP | Stronger | stdio/SSE/HTTP plus durable tenant registry, health, audit and offload are present. |
| Filesystem/sandbox | Partial | Generic Agent only has bounded `read_file` plus output artifact writes; no thread workspace `ls/glob/grep/write/edit/bash` lifecycle. |
| Subagents | Partial | Durable SingleAgent child runs exist, but no DeerFlow `task` abstraction, general-purpose/bash built-ins, background polling, or shared workspace. |
| Memory/summarization | Partial | Recall, transcript and extraction pipeline exist, but extractor/flush defaults do not produce DeerFlow's default-on memory evolution. |
| Todo/plan | Partial | Durable plan backend is stronger, but is mounted for every ADK run instead of only Pro/Ultra plan mode. |
| Uploads/multimodal | Partial | Native multimodal support is stronger, but document conversion/outline and `view_image` workspace bridge are absent. |
| Token usage | Stronger | Durable idempotent attribution and child aggregation exceed DeerFlow. |
| Artifacts | Stronger | Durable authorization, scanning, deletion/restore and signed access exceed DeerFlow. |
| Guardrail/repair/loop | Partial | Confirmation and audit are stronger; raw/invalid dangling calls, tool-frequency thresholds and argument-aware policy are not equivalent. |
| Run durability | Partial | Durable rows/checkpoints exist, but there is no run lease/heartbeat/stale recovery and batch claim can strand rows in `running`. |
| Cancel/retry/resume | Partial | ADK safe-point cancel and checkpoint resume exist; cancel races, unscheduled subagent retry and non-transactional resume remain. |
| SSE/history | Partial | LangGraph stream pagination is stronger; Workbench SSE cannot reach events after the first 200. |
| Authorization/redaction | Blocking | Core thread/run endpoints lack object/space authorization and several paths expose raw config/events/errors. |

## 4. Target Architecture

### 4.1 One Production Runtime

`eino_adk` becomes the only production task runtime. The legacy harness remains
temporarily readable for historical checkpoints and explicit migration tests,
but it is not selectable by new Workbench requests.

```text
Workbench / LangGraph adapter
  -> authenticated ThreadCommandService
  -> transactional thread/message/run write
  -> durable run queue + lease
  -> EinoParityExecutor
       -> DeerFlowRuntimeConfig projection
       -> LeadPromptComposer
       -> RuntimeStateStore
       -> ordered middleware phases
       -> Eino ChatModelAgent / TurnLoop
       -> tool, Skill, MCP, sandbox and subagent adapters
  -> durable journal/checkpoint/token/artifact records
  -> redacted Workbench and LangGraph projections
```

### 4.2 Runtime Configuration Contract

Create one parsed `DeerFlowRuntimeConfig` and stop letting individual
middlewares reinterpret raw JSON. It includes:

- `mode`: `flash`, `thinking`, `pro`, `ultra`;
- derived `thinking_enabled`, `reasoning_effort`, `is_plan_mode`,
  `subagent_enabled`, `max_concurrent_subagents`;
- model selection and provider capabilities;
- enabled Agent, Skill, MCP, web, memory and upload grants;
- context, tool-output, summarization and multimodal budgets;
- explicit safety, cancellation, retry and durability policies.

Mode projection is deterministic:

| Mode | Thinking | Plan/Todo | Subagent | Default effort |
| --- | --- | --- | --- | --- |
| flash | off | off | off | none |
| thinking | on | off | off | low |
| pro | on | on | off | medium |
| ultra | on | on | on | high |

Explicit bounded fields override mode defaults. Unsupported thinking or
reasoning is downgraded exactly once and emits a bounded capability event.

### 4.3 Lead Prompt Composition

Add a versioned, testable prompt composer rather than accepting a client
system prompt as the default Agent identity. The composer builds:

- role and response style;
- clarification-before-action policy;
- output/workspace path contract;
- Skill catalog metadata and explicit activation rules;
- deferred tool catalog metadata;
- Pro Todo behavior;
- Ultra decomposition, concurrency and synthesis behavior;
- citation requirements for web tools;
- custom Agent SOUL/config overlays from durable SingleAgent versions.

Client-supplied system text is a bounded overlay, never a replacement for
security, workspace, tool or output rules.

### 4.4 Durable Runtime State

Persist an explicit parity state envelope alongside Eino checkpoint bytes:

- messages and summary boundary;
- title;
- Todo list with replace-on-write semantics;
- sandbox/workspace identity with fail-closed conflict semantics;
- thread upload projection;
- presented artifacts with ordered deduplication;
- viewed image references;
- promoted deferred tools keyed by catalog hash;
- active Skill references and version snapshots;
- interrupt targets and completion reason.

The envelope is the API projection source. Eino checkpoint bytes remain an
internal resume implementation detail.

### 4.5 Middleware Phases

Exact Python middleware classes are not copied. Equivalent behavior is
assembled into ordered phases whose request and unwind ordering is tested:

1. tool-output budget and offload;
2. thread paths, uploads and lazy sandbox;
3. dangling tool-call repair and model reliability;
4. guardrail and sandbox audit;
5. tool error normalization;
6. dynamic date and memory context;
7. explicit Skill activation;
8. summarization and pre-summary memory flush;
9. conditional Todo/plan;
10. token attribution, title and memory queue;
11. vision/view-image injection;
12. deferred tool filtering and promoted state;
13. conditional subagent concurrency limits;
14. loop detection and safety finish;
15. clarification termination.

No declared middleware may silently resolve to a reserved no-op. Disabled
capabilities use explicit predicates and do not enter the chain.

### 4.6 Thread Workspace And Sandbox

Introduce a `ThreadWorkspace` interface with one workspace per tenant/thread:

- `/mnt/user-data/uploads`: read-only uploaded files and converted views;
- `/mnt/user-data/workspace`: temporary read/write Agent work;
- `/mnt/user-data/outputs`: final deliverables;
- `/mnt/skills`: versioned read-only Skill package tree.

Eino filesystem tools expose bounded `ls`, `glob`, `grep`, `read_file`,
`write_file` and `edit_file`. `bash` is a separate tool available only through
an isolated sandbox provider and explicit policy. Host shell execution remains
disabled by default. Every operation enforces tenant/thread paths, size,
result count, encoding and timeout limits.

### 4.7 Skills And Subagents

- Mount full Skill packages, not just `SKILL.md` text.
- Add an audited `skill_manage` adapter for create/edit/patch/delete/history,
  backed by existing durable Skill records and security scanning.
- Keep SingleAgent child runs, but expose a DeerFlow-compatible `task` tool
  with `general-purpose`, `bash` and configured Agent types.
- Ultra mode permits at most three task calls by default, bounded to two to
  four. Child Agents disable thinking by default, use isolated checkpoints,
  share only explicitly granted workspace state, and report bounded status,
  token and result summaries to the parent.
- Background child execution and polling are durable; retry rows must have an
  executable queue type and worker path.

### 4.8 Memory, Todo And Title

- Memory recall happens before the first model call with bounded thread and
  long-term scopes.
- Terminal and pre-summary transcript fragments enqueue durable extraction.
- Production defaults enable extraction and its worker when memory is enabled.
- Todo is mounted only in Pro/Ultra. Incomplete Todo completion may re-enter
  the model at most twice before a bounded terminal warning.
- Title generation occurs after the first completed exchange, is idempotent,
  and updates the same thread title used by all task lists.

### 4.9 Run Lifecycle And Reliability

Add lease fields and fencing to claimed runs. A worker must heartbeat while
executing. Stale ownership is reconciled by policy:

- resumable checkpoint available: enqueue a recovery resume run;
- no resumable checkpoint: fail the abandoned run with a bounded code;
- never leave a row indefinitely `running`.

Process every claimed row independently; one malformed run cannot strand the
rest of the batch. Thread/message/run creation and human resume use database
transactions or compensating rollback. Cancel writes durable intent first,
registers a generation/fence, and prevents any post-cancel assistant message
or success transition.

### 4.10 API, SSE And Security

- All thread/run/message/checkpoint/event/artifact/token/memory operations use
  authenticated viewer and workspace authority.
- Client `space_id`, owner and user fields are filters only after server-side
  authorization.
- Workbench and LangGraph adapters read the same application contracts.
- SSE uses event-id cursor pagination until exhaustion; reconnect resumes after
  `Last-Event-ID` without a fixed 200-event ceiling.
- Public projections expose bounded status, safe labels, token counts and
  redacted errors only.
- Raw config, command, context, metadata, model content, reasoning, tool
  arguments/results, object keys and checkpoint bytes never cross the API
  boundary.

## 5. DeerFlow Defects We Will Not Replicate

The locked baseline has verified defects. They are recorded so later audits do
not mistake a stronger NewX AI implementation for drift:

- no durable multi-worker lease or cross-worker stream bridge;
- PostgreSQL stale Run recovery is absent;
- graph interrupt/resume request fields are not fully wired;
- `/wait` may hide terminal failure when a checkpoint exists;
- rollback can report success after checkpoint restore failure;
- late join can attach to an empty stream indefinitely;
- some LangGraph-compatible request fields are accepted but ignored.

NewX AI must match successful user-visible behavior and API shapes while
returning clearer errors and stronger recovery for these cases.

## 6. Verification Strategy

### 6.1 Contract Tests

For every capability, add fixture-driven parity tests with the same normalized
input and expected observable output for DeerFlow and NewX AI:

- mode-derived runtime config and tool set;
- middleware phase order and reverse unwind behavior;
- state reducer semantics;
- prompt section presence and precedence;
- tool/Skill/MCP promotion and policy;
- Todo, title, memory and artifact state;
- interrupt, clarification, cancel, resume, retry and recovery;
- SSE sequence, reconnect and terminal behavior;
- LangGraph-compatible endpoint schemas.

### 6.2 Production Safety Tests

- cross-user and cross-space IDOR tests for every core endpoint;
- batch claim failure isolation;
- lease expiry and process-crash recovery;
- cancel-before-register and cancel-during-finalize races;
- transaction rollback for thread/run/message and resume creation;
- more than 200 run events with reconnect;
- raw payload and secret redaction scans;
- sandbox traversal, symlink, command, size and timeout boundaries.

### 6.3 Live Acceptance Matrix

Run the same prompts on locked DeerFlow and NewX AI for each mode:

1. direct answer without tools;
2. web search with citations;
3. Pro Todo plan and completion;
4. Ultra parallel subagents;
5. explicit and automatic Skill activation;
6. MCP invocation;
7. uploaded document and image processing;
8. Markdown artifact generation and presentation;
9. clarification, cancel, retry and resume;
10. long conversation summarization and later memory recall.

Capture request/response, normalized event sequence, checkpoint/state summary,
tool list, token attribution and terminal status. Manual visual review remains
separate from this backend acceptance gate.

## 7. Delivery Slices

1. **Security and lifecycle gate:** authorization, redaction, claim isolation,
   lease/recovery, cancel fencing, transactional creation and SSE pagination.
2. **Agent semantic core:** one ADK runtime, parsed mode contract, prompt
   composer, conditional pipeline and durable parity state.
3. **Workspace and context:** thread sandbox, file tools, uploads conversion,
   view-image, Skill package mounts and dynamic context.
4. **Agent orchestration:** DeerFlow-compatible task tool, built-in subagents,
   durable background execution and retry scheduling.
5. **Memory and state:** default-on memory evolution, summary flush, Todo/title
   semantics and promoted tool state.
6. **Compatibility and acceptance:** LangGraph endpoint closure, fixture
   contract suite and paired live DeerFlow/NewX AI runs.

Each slice is independently testable and committed only after targeted Go
tests, migration validation where applicable, API contract tests, redaction
checks and code review pass.

## 8. Exit Gate

Backend Agent parity is complete only when:

- all matrix rows are `aligned` or documented `stronger` with no user-visible
  regression;
- no production runtime can silently select the legacy one-step harness;
- no middleware capability is represented by a reserved no-op;
- all core Agent APIs pass tenant/object authorization and redaction tests;
- run leases, recovery, cancellation and event replay pass race and crash
  tests;
- all ten paired live acceptance cases produce equivalent normalized behavior;
- the detailed evidence document records commands, outputs and remaining
  upstream DeerFlow defects.
