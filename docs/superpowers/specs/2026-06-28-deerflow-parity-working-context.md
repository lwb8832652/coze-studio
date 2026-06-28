# DeerFlow Parity Working Context

This document is the compact task context for DeerFlow parity work. It keeps
the root `AGENTS.md` small while preserving the decisions that should steer
future implementation.

## Active Scope

The active delivery ledger is:

- `docs/superpowers/plans/2026-06-27-deerflow-p0-launch-tracker.md`

The full parity reference is:

- `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

Before starting implementation, update the relevant tracker row to `进行中`
and record the intended verification. Before committing a completed mainline
slice, update the same tracker with evidence, blocked follow-ups, and P1/P2
deferrals.

## Vocabulary

Keep Coze Studio task-oriented:

- DeerFlow `New Chat` maps to `新建任务`
- DeerFlow `Chats` maps to `全部任务`
- DeerFlow recent chats map to `我的任务`
- DeerFlow chat detail maps to `任务详情`
- Memory is user-facing as `任务记忆`

Do not rename the primary product surface to `对话`.

## P0 Mainlines

P0 must produce one launchable DeerFlow-style task experience:

1. Task lifecycle: create, list, recent list, detail, stream, reconnect,
   follow-up, cancel, retry, loading, empty, and error states.
2. Go-native Eino ADK runtime: model execution, streaming, resumable turns,
   cancellation, checkpoints, runtime settings, and lightweight Runtime
   Doctor.
3. Agent and subagent configuration backed by durable Coze records.
4. Skills: list/create/edit/enable/version/history/rollback/export and
   progressive activation from the task composer/settings.
5. MCP and tools: durable catalog, safe auth handling, health, Eino runtime
   invocation, and task-detail tool events.
6. Memory: retrieval, async update, management panel, edit/delete/restore,
   import/export, and bounded runtime prompt injection.
7. Token usage: run/subagent/middleware/tool attribution and task-detail
   display.
8. Settings and acceptance: DeerFlow-equivalent runtime/model/memory/Skill/MCP
   settings plus unit, API, and browser smoke evidence.

## Explicit Exclusions

IM Channels are excluded from this project. Do not add Telegram, Slack,
Discord, Feishu/Lark, DingTalk, WeChat, channel credentials, channel workers,
channel settings, channel filters, or channel-specific task policy.

Phase 2 hardening is deferred unless it fixes a direct P0 regression:

- complex security scanning and quarantine;
- policy administration UI;
- Prometheus/OpenTelemetry export;
- deep sandbox diagnostics;
- storage lifecycle/reconciliation;
- load/chaos gates;
- full browser CI.

When an enhancement is not needed to match a DeerFlow-visible workflow, record
it as P1/P2 instead of implementing it inline.

## DeerFlow Detail Parity Checks

Use the same prompt in both systems when validating task detail parity:

```text
你可以绘制时序图或架构图吗？请直接给出一个 Mermaid sequenceDiagram 和一个 flowchart 架构图，并说明如何继续修改。
```

The DeerFlow task-detail reference exposes these visible behaviors:

- chat-like thread detail surface with user and assistant turns inline;
- thread title in the header;
- global token usage button/indicator;
- export action for the thread;
- artifact trigger and side panel;
- collapsible inline thinking/reasoning block; keep it visible as a DeerFlow
  parity target instead of removing it as redundant UI;
- rendered Mermaid diagrams, not raw fenced text;
- per-message token usage;
- bottom composer with mode/model controls, upload, submit/stop;
- recent conversation sidebar.

For Coze P0, task naming may stay different, but the task detail behavior
should be visibly equivalent unless a gap is explicitly marked P1/P2 in the
tracker.
