# Eino Agent Runtime Guidance

Coze Studio remains the control plane and durable system of record. Eino ADK
is the Go-native execution kernel, not a public platform contract.

## Eino-First Rule

Before adding custom Agent runtime behavior, check whether Eino ADK already
provides the primitive. Prefer a Coze adapter or policy wrapper around Eino
instead of a parallel implementation.

Use Eino ADK for:

- `ChatModelAgent`, `Runner`, `AgentEvent`, streaming, tool calling,
  interrupt, resume, and checkpoint hooks;
- `TurnLoop` for queued input, push, preemption, safe-point cancellation, and
  resumable turns;
- `AgentTool` and `DeepAgent` for delegation and subagents;
- middleware for summarization, tool-result reduction, dangling tool-call
  repair, filesystem access, dynamic tool search, plan tasks, `AGENTS.md`,
  and `SKILL.md` loading.

Use `*schema.Message` as the production baseline. Keep
`*schema.AgenticMessage` experimental until cancellation, retry, checkpoint,
streaming, and provider parity are covered by contract tests.

## Coze-Owned Boundaries

Coze owns:

- task/thread/run/message/event records and tenant authorization;
- durable checkpoints, worker leases, cancellation, retries, and recovery;
- LangGraph-compatible HTTP and SSE contracts;
- Agent, Skill, MCP, model, memory, sandbox, tool, and settings control
  planes;
- policy, guardrails, audit, token attribution, pricing snapshots, cost, and
  observability.

Treat Eino `AgentEvent`, checkpoint bytes, Skill runtime state, and provider
metadata as internal execution contracts. Translate them through Coze-owned
adapters before exposing or persisting public contracts.

## Runtime Selection

Production task execution uses Eino ADK only:

- `AGENT_THREAD_RUNTIME_DEFAULT=eino_adk` (optional; this is also the default)
- `AGENT_THREAD_EINO_ADK_ENABLED=true` (optional; this is also the default)

New public requests are normalized to `runtime=eino_adk` before persistence.
An explicit `legacy`, unknown runtime, disabled Eino runtime, or legacy
production default fails closed. The legacy harness remains wired only to read
or resume historical rows/checkpoints without an ADK marker and for explicit
migration tests; it is not a production rollback selector.

## Tools And Policy

Use `ADKToolPolicyProvider` as the common allow-list boundary for runtime
tools. Absence of `tool_policy` keeps existing tools unchanged; an explicit
empty allow-list denies that tool class.

`ApplicationADKAgentFactory` must use the provider-returned filtered tool set
as the single source for:

- model-visible tools;
- executable `ToolsNodeConfig.Tools`;
- middleware `StaticTools` and `DynamicTools`.

Do not rebuild a separate model-visible tool list outside that path.

Skill, MCP, web, filesystem, human-interaction, and subagent grants should
feed the same policy layer instead of adding parallel filters.

## Subagents

Use `ADKSubagentToolProvider` plus Eino `adk.NewAgentTool` for Coze-owned
child Agent definitions. Do not hand-roll a custom delegation loop when
`AgentTool` is sufficient.

Durable SingleAgent draft/version/publish tables remain the Agent system of
record. Child SingleAgent runs must not inherit parent runtime config such as
web tools, tool catalogs, or model overrides unless explicit durable grants
allow them.

Subagent tool names must be Eino-safe (`[A-Za-z_][A-Za-z0-9_]{0,63}`), unique
within the parent run, and backed by non-empty descriptions.

Default subagent limits:

- `max_subagents=16`
- `max_depth=2`
- `timeout_ms=300000`

Run config `subagent_policy` / `subagentPolicy` may override bounded values.

Parent runs use `parent_run_id=0` and `run_kind='task'`. Child rows use
`agent_runs.parent_run_id` plus `run_kind='subagent'`. Default run listing and
worker claim paths must stay top-level-only unless a caller explicitly asks
for a parent run's children or an admin mixed view.

Subagent lifecycle events must stay content-free:

- allowed: subagent identity, child run ID, status, elapsed time, sanitized
  terminal classification/error metadata;
- forbidden: tool arguments, model input/output, final child text,
  checkpoint bytes, object URIs, URLs, filenames, and raw provider payloads.

Subagent retry creates a new top-level queued task run with safe
`subagent_retry` metadata. Never mutate or requeue the historical child row.

## Memory

Use the existing `agent_thread_memories` table and `entity.Memory` model. Do
not introduce a parallel memory store for DeerFlow parity.

Model-visible memory remains bounded to scope/content. Management APIs may
expose safe row metadata only and must not expose prompts, model output, tool
arguments/results, checkpoint bytes, object URIs, raw provider bodies,
credentials, URLs, filenames, hidden run config, or audit-derived memory
content.

Memory extraction and flush workers are disabled unless explicitly configured.
The worker must use a configured extractor and must not add heuristic natural
language extraction in the worker itself.

## Token Usage

Use Eino response metadata and callbacks as raw usage inputs only. Coze owns
idempotent attribution, pricing snapshots, cost accounting, and durable audit.

Usage metadata may include safe correlation fields such as trace/span IDs,
idempotency key, source, usage kind, provider, model, retry attempt, and
bounded token counts. It must not include prompt text, completion text, tool
arguments/results, object keys, checkpoint bytes, credentials, or raw provider
responses.

Task-thread token usage may explicitly include direct child runs through
`include_child_runs=true`. Do not silently broaden historical run-scoped
queries.

## Artifacts And Guardrails

Artifacts are user-visible presentation records in `agent_artifacts` backed by
Coze-managed files/offloads. Read, delete, restore, scan, signed URL, and
preview paths must remain tenant-authorized and metadata-safe.

Guardrail/security work exists in the codebase but is Phase 2 unless it fixes
a P0 regression. Do not expand complex scanners, archive/retention, policy UI,
or observability exporters while DeerFlow-visible P0 gaps remain.
