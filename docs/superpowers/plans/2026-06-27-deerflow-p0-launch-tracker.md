# DeerFlow P0 Launch Tracker

> **For agentic workers:** This is the active task ledger for the first
> launchable DeerFlow-parity release. Keep this document updated before and
> after each implementation slice. Do not select Phase 2 work while any P0
> item is unfinished unless it fixes a direct P0 regression.

**Goal:** Ship one usable production-shaped DeerFlow-parity version quickly by
freezing the scope around task-oriented Agent execution, runtime settings,
Skills/MCP/tools, memory, token usage, and a lightweight Runtime Doctor.

**Architecture:** Coze Studio remains the control plane and durable system of
record. Eino ADK remains the Go-native execution kernel. P0 favors the
smallest production-compatible adapter that completes DeerFlow-visible task
workflows; broader platform hardening is recorded as P1/P2.

**Tech Stack:** Go, Hertz, Eino ADK, MySQL, Atlas migrations, React,
TypeScript, Semi/Coze Design, generated Workbench API clients.

---

## Scope Freeze

P0 is the only active delivery scope until this tracker reaches the P0 exit
gate.

- Keep product vocabulary task-oriented: `新建任务`, `全部任务`, `我的任务`,
  `任务详情`, `任务记忆`.
- Keep the runtime Go-native. Do not introduce a Python sidecar.
- Use Eino-first execution. Before adding custom Agent, Skill, tool, memory,
  summarization, retry, or tool-result handling logic, check whether Eino ADK
  already provides the primitive and wrap it through a Coze adapter.
- Do not add IM Channels. They are excluded from routes, menus, settings,
  dependencies, tests, and acceptance criteria.
- Do not start Phase 2 hardening while P0 is open. Complex scanners,
  Prometheus/OpenTelemetry exporters, policy admin UI, load/chaos gates,
  storage lifecycle, and full operator runbooks are deferred unless needed to
  fix a P0 regression.

## Status Rules

Use these status values only:

- `已完成`: implementation exists and this tracker does not require more P0
  development, except ordinary regression checks.
- `待验收`: implementation appears present from the current roadmap/codebase,
  but the P0 acceptance command or browser/API smoke test has not been rerun
  in this cutline.
- `进行中`: actively being implemented in the current slice.
- `待开发`: required for P0 and not implemented yet.
- `阻塞`: cannot proceed without user input, external service availability, or
  a prerequisite fix. Include the blocker in the notes.
- `延后(P1)` / `延后(P2)`: explicitly out of the first launchable version.

Every implementation slice must update this document:

1. Before starting: set the selected subtask to `进行中` and add the planned
   verification command or browser check.
2. Before commit: move completed subtasks to `已完成` or `待验收`, record any
   blocked or deferred follow-up, and keep unrelated P1/P2 ideas out of P0.
3. After verification: record the command or browser path in the acceptance
   notes for the touched mainline.

## P0 Mainlines

### 1. 任务主流程闭环

| Subtask | Status | P0 Acceptance | Notes |
| --- | --- | --- | --- |
| 任务主流程闭环-新建任务入口 | 待验收 | A user can create a task from the task-oriented entry without exposing chat naming. | Keep label `新建任务`. |
| 任务主流程闭环-新建任务后端创建 | 待验收 | Create API persists thread/run identity with server-side owner/space authority. | No client-controlled identity. |
| 任务主流程闭环-全部任务列表 | 待验收 | `全部任务` lists top-level task runs only by default. | Child subagent runs are not mixed into the default list. |
| 任务主流程闭环-我的任务/最近任务 | 待验收 | `我的任务` shows recent user-owned tasks with loading, empty, and error states. | Preserve task vocabulary. |
| 任务主流程闭环-任务详情基础信息 | 待验收 | Detail page shows task title, status, timestamps, safe assistant identity, and actions. | No raw hidden run config. |
| 任务主流程闭环-用户输入追加 | 待验收 | User can append input/follow-up to a resumable task without losing history. | Must keep durable message ordering. |
| 任务主流程闭环-SSE流式输出 | 待验收 | Running task streams model/events to the detail page and reaches one terminal state. | Reconnect must avoid duplicate logical events. |
| 任务主流程闭环-执行步骤/事件时间线 | 待验收 | Detail page shows safe step/event cards for model, tool, subagent, memory, and terminal events. | Do not render prompt text, tool args/results, checkpoint bytes, object URIs, or raw provider payloads. |
| 任务主流程闭环-取消运行 | 待验收 | Cancel reaches active runtime work and the UI reflects canceled terminal state. | Include model/tool/subagent cancellation where supported. |
| 任务主流程闭环-失败重试 | 待验收 | Failed task can create a new retry run with safe retry metadata. | Historical failed run rows must not mutate. |
| 任务主流程闭环-空/加载/错误状态 | 待验收 | Lists and detail render bounded loading, empty, and error states. | Use Semi/Coze Design components. |
| 任务主流程闭环-桌面浏览器冒烟 | 待开发 | Browser smoke covers create, stream, cancel/retry, reopen detail, and refresh. | Full CI browser suite is P1 if harness is not ready. |

### 2. Eino ADK 运行设置和 Runtime Doctor

| Subtask | Status | P0 Acceptance | Notes |
| --- | --- | --- | --- |
| 运行设置-ADK runtime 选择与策略 | 已完成 | Backend can select `eino_adk` according to runtime policy and fail closed when unsupported. | Keep legacy fallback only where policy allows it. |
| 运行设置-模型 reasoning 参数 | 已完成 | Backend projects reasoning options into ADK run config without leaking hidden config. | Recent backend slice completed this. |
| 运行设置-模型可靠性/候选模型 | 已完成 | Backend supports failover candidates and bounded reliability config. | Frontend consolidation remains below. |
| 运行设置-Web Fetch/Search 配置 | 已完成 | Backend exposes durable web fetch/search settings for Workbench runtime config. | Do not expose secrets or raw endpoint details. |
| 运行设置-前端聚合面板 | 已完成 | Settings UI can view/edit P0 runtime, model, reasoning, web, memory, Skill, and MCP toggles from one task-oriented surface. | Extended existing `WorkbenchRuntimeSettingsControl` with reasoning effort, Skill invocation, and editable MCP toggles. Submit payload now trims `enable_skills` / `enable_mcp` according to runtime switches and serializes safe `reasoning_effort` plus `skills` runtime settings. Verified with `rushx test -- src/pages/workbench/__tests__/workbench.test.tsx`. |
| Runtime Doctor-后端基础接口 | 已完成 | `GET /api/workbench/runtime_doctor` returns ADK runtime, Web, and MCP health summaries safely. | Recent backend slice completed this. |
| Runtime Doctor-前端状态面板 | 已完成 | UI displays runtime, model config, Web, MCP, Skill, and memory status with refresh/error states. | Added task-detail panel with refresh/error states and safe pending cards for model/Skill/memory deep checks. Verified with `rushx test -- src/pages/tasks/__tests__/task-runtime-doctor-section.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx` and `rushx lint` in `frontend/apps/coze-studio`. |
| Runtime Doctor-模型连通性基础检查 | 已完成 | P0 shows whether configured model settings are present and usable enough to start a run. | Added backend `model.default` check that resolves the configured default chat model without invoking generation/streaming, converts provider errors/panics to bounded safe diagnostics, and renders through the existing task-detail Runtime Doctor panel. Verified with `go test ./application/workbench -run 'TestRuntimeDoctor(Model|Skill)' -count=1`, `go test ./api/handler/coze -run 'TestWorkbenchRuntimeDoctor' -count=1`, and `rushx test -- src/pages/tasks/__tests__/task-runtime-doctor-section.test.tsx`. Deep live provider capability matrix stays P1. |
| Runtime Doctor-Skill/MCP基础健康 | 已完成 | P0 shows enabled/disabled/error counts and safe names where already available. | Added backend `skills.runtime_catalog` check with enabled/total counts and bounded sanitized enabled Skill names; uninitialized Skill service reports disabled, real list failures report error. Existing MCP health summary remains metadata-only. Verified with the Runtime Doctor backend, handler, and frontend tests above. |
| Runtime Doctor-sandbox/shell深度诊断 | 延后(P1) | N/A | Keep shell/sandbox disabled or development-only until the boundary is closed. |

### 3. Skills / MCP / Tools 最小可用闭环

| Subtask | Status | P0 Acceptance | Notes |
| --- | --- | --- | --- |
| Skills-列表/创建/编辑/启停 | 待验收 | User can manage DeerFlow-style Skills through existing task-oriented Workbench controls. | Keep generated API contracts where available. |
| Skills-版本/历史/回滚/导出 | 待验收 | Existing history/export paths work without exposing hidden runtime payloads. | If a gap is discovered, fix only the P0 visible path. |
| Skills-Eino runtime 注入 | 待验收 | Enabled Skills can be loaded through Eino ADK middleware with safe policy boundaries. | Eino-first, no parallel custom executor. |
| Skills-slash/渐进激活 | 待验收 | User-visible activation behavior matches DeerFlow enough for task workflows. | Browser smoke can be manual for P0. |
| MCP-服务/工具目录配置 | 待验收 | User can configure and list MCP servers/tools with durable metadata and safe auth handling. | No raw secrets in responses. |
| MCP-健康状态 | 已完成 | Health status is visible in tool settings or Runtime Doctor. | Runtime Doctor displays total/enabled/healthy/unhealthy/unknown MCP counts without config/auth details. |
| MCP-Eino 工具调用 | 待验收 | Enabled MCP/tool entries can be invoked through Eino ADK tool path. | Tool policy allowlist must apply. |
| Tools-基础 allowlist 策略 | 待验收 | Empty explicit allowlist denies that tool class; configured grants do not broaden child agent permissions. | Reuse `ADKToolPolicyProvider`. |
| Tools-任务详情工具事件 | 已完成 | Tool events appear as safe task-detail cards. | Hardened task-detail tool event projection so tool arguments/results, raw provider payloads, URLs, credentials, object identifiers, and unsafe tool names are hidden from cards. Verified with `rushx test -- src/pages/tasks/__tests__/tasks.test.tsx` and `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`. |
| Tools-复杂策略管理 UI | 延后(P1) | N/A | Record policy UI needs as P1, not P0. |
| MCP-多 transport 深度管理 | 延后(P1) | N/A | P0 only needs the DeerFlow-visible minimum. |

### 4. 任务记忆和 Token 用量

| Subtask | Status | P0 Acceptance | Notes |
| --- | --- | --- | --- |
| 任务记忆-后端 CRUD/search/clear | 已完成 | Workbench task-thread memory APIs support list/search/update/delete/clear with authorization. | Existing M7 slices completed backend foundation. |
| 任务记忆-restore/audit | 已完成 | Deleted rows can be restored and metadata-only audit can be listed. | Audit must not expose memory content. |
| 任务记忆-import/export | 已完成 | Safe schema import/export works through generated clients. | Unknown fields are stripped on import. |
| 任务记忆-前端管理面板 | 已完成 | `任务记忆` panel supports list/search/filter/edit/delete/clear/restore/audit/import/export. | Browser execution still needs P0 smoke. |
| 任务记忆-权限只读态 | 已完成 | Non-owner read-only affordance disables write actions while backend remains authoritative. | UI is not the authority. |
| 任务记忆-浏览器冒烟 | 待开发 | Browser smoke covers search, edit, delete, restore, import, export, and read-only display. | Full CI browser suite can be P1. |
| 记忆抽取-worker 默认关闭 | 已完成 | Model-backed extractor can be configured but does not start accidentally without explicit env. | P0 may ship with extraction disabled. |
| Token-运行级用量聚合 | 待验收 | Task detail can show run-level total token count and call count. | Missing usage means omit display, not zero cost. |
| Token-子智能体用量聚合 | 待验收 | Child subagent cards can use grouped run aggregates without one request per child. | Keep provider/model label bounded. |
| Token-成本快照 | 延后(P1) | N/A | Only display cost when backend already provides safe positive cost and one currency. |

### 5. 上线验收和回归

| Subtask | Status | P0 Acceptance | Notes |
| --- | --- | --- | --- |
| 上线验收-Atlas migration 状态 | 待验收 | Local Atlas can inspect/apply pending migrations without drift. | Use local Atlas; do not rely on a remote binary. |
| 上线验收-生成 API 合约 | 待验收 | IDL/generated Workbench clients are in sync after touched API changes. | Avoid hand-written fetch clients for generated endpoints. |
| 上线验收-后端单测 | 待验收 | Targeted backend package tests pass for touched code. | Record known unrelated failures separately. |
| 上线验收-前端类型/单测 | 待验收 | Targeted app/package typecheck or unit tests pass for touched UI. | Use package-local commands when full Rush is too slow. |
| 上线验收-桌面端任务冒烟 | 待开发 | Manual or automated browser path covers P0 task lifecycle. | Required before first P0 release candidate. |
| 上线验收-部署配置清单 | 待开发 | Document required env vars, disabled defaults, middleware services, and rollback note. | Keep secrets out of docs. |
| 上线验收-已知问题清单 | 待开发 | Known unrelated failures and deferred P1/P2 items are documented before release. | Prevent P0 from silently absorbing Phase 2 work. |

## P1 After P0

P1 starts only after the P0 exit gate passes.

- Full browser E2E harness and CI jobs for task, memory, Skills, MCP, and
  Runtime Doctor workflows.
- Complete LangGraph compatibility suite beyond the DeerFlow-visible P0 API
  behavior.
- Runtime Doctor live model connectivity, provider capability matrix, sandbox
  diagnostics, and richer Skill/MCP probes.
- Shell/sandbox production providers, output promotion polish, richer artifact
  previews, MIME-specific renderer hardening, and mobile viewport checks.
- Skills/MCP/Tools policy controls, invocation history, output budgets, and
  deeper transport management.
- Pricing/cost snapshots, provider/model breakdowns, and usage debug views.

## P2 Backlog

P2 remains parked while P0 or P1 parity work is active.

- Complex skill/tool/file/command/network/security scanners and quarantine.
- Guardrail policy administration, audit archive/retention/legal hold, and
  scheduled archive operations not needed for P0.
- Prometheus/OpenTelemetry exporters, dashboards, alerts, load/chaos gates,
  storage lifecycle/reconciliation, backup/rollback rehearsal, and expanded
  operator runbooks.

## P0 Exit Gate

P0 can be considered launchable only when all are true:

1. All P0 subtasks are `已完成` or explicitly accepted as `待验收` with a
   recorded non-blocking release note.
2. A desktop task smoke covers create, stream, detail refresh, cancel or retry,
   memory panel open, tool/Skill/MCP status visibility, and token display.
3. No P0 API response exposes prompts, model completions, tool arguments,
   tool results, checkpoint bytes, credentials, object URIs, raw provider
   bodies, or hidden run config outside already-reviewed safe contracts.
4. IM Channels are absent from routes, menus, settings, dependencies, and
   acceptance criteria.
5. Phase 2 items discovered during P0 are recorded here or in the master
   roadmap instead of being implemented inline.
