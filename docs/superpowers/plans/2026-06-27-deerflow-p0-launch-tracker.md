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
| 任务主流程闭环-新建任务入口 | 已完成 | A user can create a task from the task-oriented entry without exposing chat naming. | Workspace menu keeps `新建任务` at `chats/new`; Workbench submit now calls generated `CreateTaskThread`, sends the bounded Eino ADK runtime config, and navigates to canonical task-thread detail by `thread_id`. Verified with `rushx test -- src/pages/workbench/__tests__/workbench.test.tsx src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx` and desktop smoke at `http://127.0.0.1:8888/space/7656103552997130240/chats/new`. |
| 任务主流程闭环-新建任务后端创建 | 已完成 | Create API persists thread/run identity with server-side owner/space authority. | `POST /api/workbench/task_threads` creates a canonical thread, queued top-level task run, and initial user message without going through legacy `chat_tasks`; runtime policy is validated before thread persistence so disabled ADK configs cannot leave orphan task threads. Verified with `go test ./application/agentthread -run 'TestApplicationCreate(TaskThread|Run)' -count=1`, `go test ./api/handler/coze -run 'TestCreateTaskThreadHandlerCreatesThreadRunAndInitialMessage' -count=1`, and `go test ./api/router/coze -run 'TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1`. |
| 任务主流程闭环-全部任务列表 | 已完成 | `全部任务` lists top-level task runs only by default. | Existing task page reads canonical task threads, treats `legacy_task_id="0"` as absent, and opens task-thread detail paths without exposing raw JSON. Verified with `rushx test -- src/pages/tasks/__tests__/tasks.test.tsx` and `go test ./api/handler/coze -run 'Test(ListTaskThreadsHandlerReturnsAgentThreads|ListTaskThreadRunsHandlerReturnsChildRunsForParentRun)' -count=1`. |
| 任务主流程闭环-我的任务/最近任务 | 已完成 | `我的任务` shows recent user-owned tasks with loading, empty, and error states. | Workspace sidebar `我的任务` uses canonical task threads with `page_size=8`, preserves task vocabulary, treats `legacy_task_id="0"` as absent, and opens task-thread detail. General list loading/empty/error states are covered by task-list tests; sidebar browser smoke remains under the desktop smoke item. Verified with `rushx test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/tasks/__tests__/tasks.test.tsx`. |
| 任务主流程闭环-任务详情基础信息 | 已完成 | Detail page shows task title, status, timestamps, safe assistant identity, and actions. | Existing task detail tests cover canonical thread detail rendering, compatibility fallback, safe summary fields, hidden raw input/result JSON, and `legacy_task_id="0"` canonical fallback so detail does not call `/api/workbench/tasks/0`. Detail now also derives terminal UI state from the latest canonical top-level run when the thread aggregate is stale, so completed runs close the cancel action and show `2/2 已完成 · 100%`. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "legacy task id is zero string"`, `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`, and browser smoke on `http://localhost:8080/space/7656275718757679104/tasks/7656293553953308672`. |
| 任务主流程闭环-用户输入追加 | 已完成 | User can append input/follow-up to a resumable task without losing history. | Canonical thread detail appends a durable user message, creates a queued follow-up run with bounded runtime config and stable idempotency key, then refreshes messages in place. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`, `go test ./api/handler/coze -run 'Test(AppendTaskThreadMessageHandlerCreatesMessage|CreateTaskThreadRunHandlerCreatesPendingRun|ResumeTaskThreadRunHandlerCreatesQueuedResumeRun)' -count=1`, and `go test ./domain/agentthread/service -run 'Test(AppendMessageCreatesMessageWithGeneratedID|CreateRunDefaultsStatusAndRuntimeOptions)' -count=1`. |
| 任务主流程闭环-SSE流式输出 | 已完成 | Running task streams model/events to the detail page and reaches one terminal state. | Existing detail hook opens the Workbench run-event EventSource, projects incoming events into the execution flow, and closes the stream on unmount; backend stream tests cover event and interrupted-run completion. Workbench run event REST and SSE share the same API mapper, so ADK raw message/tool/model payload redaction applies to both channels. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx` and `go test -gcflags="all=-N -l" ./api/handler/coze -run 'Test(ListTaskThreadRunEventsHandler(RedactsUnsafePayload|ReturnsEvents)|StreamTaskThreadRunEventsWritesEventsAndDone|TaskThreadRunEventStreamStopsForInterruptedRun)' -count=1`. |
| 任务主流程闭环-执行步骤/事件时间线 | 已完成 | Detail page shows safe step/event cards for model, tool, subagent, memory, and terminal events. | Event projection tests cover step, plan, tool, human-interaction, subagent lifecycle, and token-aware subagent cards while hiding unsafe tool details. Workbench `run_events` API now summarizes unsafe ADK `message.completed`, `tool.completed`, `tool.failed`, `model.safety_finish`, and `agent.output` payloads to bounded metadata before returning them, while preserving safe `run.*`, `step.*`, `plan.task.*`, and `subagent.run.*` UI metadata. Verified with `rushx test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx` and the backend redaction command under SSE output. |
| 任务主流程闭环-取消运行 | 已完成 | Cancel reaches active runtime work and the UI reflects canceled terminal state. | Canonical task-thread detail now shows `取消任务` for active thread runs, binds the action to the latest top-level task run from `ListTaskThreadRuns(parent_run_id=0)`, calls `POST /api/workbench/task_threads/:thread_id/runs/:run_id/cancel`, validates run/thread ownership, passes the current run status into the cancel transition, then refreshes detail. Pending and running cancellation paths are covered so newly queued task runs no longer fail with a running-only CAS. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestCancelTaskThreadRunHandler(CancelsPendingRun|TransitionsRun)' -count=1`, and live API smoke on `127.0.0.1:8888` showing `canceled_from=pending` and `status=canceled`. Runtime-level model/tool interruption remains bounded by existing worker cancellation support. |
| 任务主流程闭环-失败重试 | 已完成 | Failed task can create a new retry run with safe retry metadata. | Canonical task-thread detail now shows `重试任务` for failed thread runs, binds the action to the latest top-level task run rather than mixed child-run events, and creates a new top-level run with bounded input/config plus metadata-only `source=task_retry` and `source_run_id`; historical failed run rows are not mutated. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx` and live API smoke on `127.0.0.1:8888` creating a top-level retry run with `metadata.source=task_retry`. |
| 任务主流程闭环-空/加载/错误状态 | 已完成 | Lists and detail render bounded loading, empty, and error states. | Added frontend characterization coverage for task-list loading, empty, and error states; detail page already renders bounded loading/error/not-found states. Verified with `rushx test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx`. |
| 任务主流程闭环-桌面浏览器冒烟 | 进行中 | Browser smoke covers create, stream, cancel/retry, reopen detail, and refresh. | Partial desktop smoke passed for canonical create -> ADK run -> detail refresh on `127.0.0.1:8888`: `POST /api/workbench/task_threads` returned 200, detail used only task-thread APIs, worker completed the Eino ADK run as `succeeded`, messages included the assistant answer, and token usage returned aggregate rows. Follow-up API smoke covered canonical task creation, pending cancel, task-level retry metadata, and task-memory search/edit/delete/restore/import/export on the live server. Additional browser smoke on `localhost:8080` confirmed a completed Mermaid prompt task shows `已完成`, hides `取消任务`, renders execution progress as `2/2 已完成 · 100%`, and renders two Mermaid diagrams as ready SVGs without raw fenced Mermaid text in the answer area. Current DeerFlow comparison smoke also confirms task detail keeps `运行诊断`、`安全审计`、`任务记忆` behind header `详情` inspector, the follow-up composer is docked outside the scrollable chat transcript, task detail header no longer shows `任务详情 › 已完成` or `产物 0`, header actions show compact clickable `Tokens` plus `导出` and `详情`, clicking `Tokens` opens Input/Output/Total usage details, assistant answers no longer show `普通回答 · Agent/Ark` result labels, `执行流程` remains visible as a lightweight step feed with each step's status/title/runtime/detail/time, and each Mermaid diagram exposes SVG download plus source copy actions. TD-COMP-001/003 are code-complete for the DeerFlow composer shell: task detail removes `Auto/Ask/Agent` and the visible run settings button, keeps `拓展` and `@`, adds the DeerFlow mode trigger defaulting to `Ultra`, keeps attachment/link entries, and uses the green round send icon; real browser visual confirmation remains manual验收. TD-EXP-002 export is code-complete against the user-provided DeerFlow Markdown/JSON samples: header `导出` opens Markdown and JSON items, Markdown/JSON both include only visible user/assistant transcript and filter thinking/tool/internal markers; real browser download sample remains manual验收. TD-MSG-005 inline thinking is code-complete and covered by unit tests for ADK `message.completed` reasoning extraction, `<think>` stripping, and provider signature hiding; it still needs a real reasoning sample browser screenshot before marking fully complete. TD-TOKEN-002 is code-complete for DeerFlow-style display modes and assistant per-turn token summaries; real browser screenshot remains manual验收. The active DeerFlow comparison ledger is `docs/superpowers/plans/2026-06-28-deerflow-task-detail-parity-validation.md`; future task-detail work must reference a case id from that file and store screenshots/API samples under `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/`. DeerFlow-visible gaps still open for document generation/preview validation and full chat-like detail layout details. Full CI browser suite is P1 if harness is not ready. |

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
| Skills-列表/创建/编辑/启停 | 已完成 | User can manage DeerFlow-style Skills through existing task-oriented Workbench controls. | Frontend Skill page tests cover list/create/import/test-run/toggle/delete states; backend handler/application/domain tests cover create/update/list/delete and safe mapping. Verified with `rushx test -- src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx src/pages/tools/__tests__/tools.test.tsx src/pages/tools/__tests__/tools-service.test.ts src/pages/workbench/__tests__/workbench.test.tsx`, `go test -gcflags="all=-l -N" ./api/handler/coze -run 'Test(WorkbenchMCPToolHandlers|ListSkillVersionsHandler|ListSkillVersionResourcesHandler|ExportSkillVersionHandler|RollbackSkillVersionHandler|DeleteSkillHandler|ListSkillToolCandidatesHandler|UpdateSkillVersion(Resource|Content)Handler)' -count=1`, and `go test ./application/skill ./domain/skill/service ./domain/skill/repository ./application/mcptool -run 'Test' -count=1`. |
| Skills-版本/历史/回滚/导出 | 已完成 | Existing history/export paths work without exposing hidden runtime payloads. | Skill version panel and backend tests cover version list, resource list/update, content update, export, and rollback. Verified with the same Skill frontend/backend commands above. |
| Skills-Eino runtime 注入 | 已完成 | Enabled Skills can be loaded through Eino ADK middleware with safe policy boundaries. | `RuntimeSkillProvider` selects enabled prompt Skills by explicit `enable_skills` selectors, loads latest version snapshots, applies body budgets, and ADK Skill backend tests cover runtime loading and sanitized Skill backend descriptions. Verified with `go test -gcflags="all=-l -N" ./application/agentthread -run 'Test(ADKSkill|ADKMCP|ADKTool|DefaultADKTool|ADKSingleAgentToolGrant|ADKSubagentToolProvider)' -count=1`. |
| Skills-slash/渐进激活 | 已完成 | User-visible activation behavior matches DeerFlow enough for task workflows. | Workbench extension popover selects Skills/MCP into `enable_skills` / `enable_mcp`; runtime settings can disable selected classes by sending empty enabled arrays while preserving UI selections. Verified through Workbench tests in the frontend command above. Full browser smoke remains under the P0 desktop smoke items. |
| MCP-服务/工具目录配置 | 已完成 | User can configure and list MCP servers/tools with durable metadata and safe auth handling. | Frontend Tools page/service tests cover list/search/create/test/delete states; backend MCP handler/application/catalog tests cover upsert/list/get/delete/registry/test-call, health recording, secret masking, and auth-at-rest codec boundaries. Verified with the frontend command above plus `go test ./application/mcptool -run 'Test' -count=1`. |
| MCP-健康状态 | 已完成 | Health status is visible in tool settings or Runtime Doctor. | Runtime Doctor displays total/enabled/healthy/unhealthy/unknown MCP counts without config/auth details. |
| MCP-Eino 工具调用 | 已完成 | Enabled MCP/tool entries can be invoked through Eino ADK tool path. | `ADKMCPRuntimeToolCatalog` loads enabled MCP registry entries from run config `mcp_tools`, filters by allowlist when present, and invokes through the MCP runtime executor. Verified with `go test -gcflags="all=-l -N" ./application/agentthread -run 'Test(ADKSkill|ADKMCP|ADKTool|DefaultADKTool|ADKSingleAgentToolGrant|ADKSubagentToolProvider)' -count=1`. |
| Tools-基础 allowlist 策略 | 已完成 | Empty explicit allowlist denies that tool class; configured grants do not broaden child agent permissions. | Existing `ADKToolPolicyProvider` and default provider tests cover explicit empty allowlist denial, filtered static/dynamic tools, and SingleAgent grant intersection. Verified with the ADK agentthread command above. |
| Tools-任务详情工具事件 | 已完成 | Tool events appear as safe task-detail cards. | Hardened task-detail tool event projection so tool arguments/results, raw provider payloads, URLs, credentials, object identifiers, and unsafe tool names are hidden from cards. Verified with `rushx test -- src/pages/tasks/__tests__/tasks.test.tsx` and `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`. |
| Tools-复杂策略管理 UI | 延后(P1) | N/A | Record policy UI needs as P1, not P0. |
| MCP-多 transport 深度管理 | 延后(P1) | N/A | P0 only needs the DeerFlow-visible minimum. |

### 4. 任务记忆和 Token 用量

| Subtask | Status | P0 Acceptance | Notes |
| --- | --- | --- | --- |
| 任务记忆-后端 CRUD/search/clear | 已完成 | Workbench task-thread memory APIs support list/search/update/delete/clear with authorization. | Existing M7 slices completed backend foundation. |
| 任务记忆-restore/audit | 已完成 | Deleted rows can be restored and metadata-only audit can be listed. | Audit must not expose memory content. |
| 任务记忆-import/export | 已完成 | Safe schema import/export works through generated clients. | Unknown fields are stripped on import. |
| 任务记忆-前端管理面板 | 已完成 | `任务记忆` panel supports list/search/filter/edit/delete/clear/restore/audit/import/export. | Verified with `rushx test -- src/pages/tasks/__tests__/task-memory-section.test.tsx`, which covers safe list/search controls, edit, delete, clear, deleted toggle, restore, metadata-only audit, import, export, and read-only affordances. |
| 任务记忆-权限只读态 | 已完成 | Non-owner read-only affordance disables write actions while backend remains authoritative. | UI is not the authority. |
| 任务记忆-浏览器冒烟 | 待验收 | Browser smoke covers search, edit, delete, restore, import, export, and read-only display. | Live API smoke passed on `127.0.0.1:8888` after fixing MySQL LIKE escaping: register -> space -> `POST /api/workbench/task_threads` -> import memory -> `GET /memories?q=searchable` -> update -> export -> delete -> `include_deleted=true` list -> restore -> audit events. Frontend component smoke passed with the `task-memory-section` Vitest command above. Manual visual browser confirmation remains because in-app browser automation timed out on DOM/screenshot capture. |
| 记忆抽取-worker 默认关闭 | 已完成 | Model-backed extractor can be configured but does not start accidentally without explicit env. | P0 may ship with extraction disabled. |
| Token-运行级用量聚合 | 已完成 | Task detail can show run-level total token count and call count. | Existing task-detail loader maps `aggregate` into the top-bar `TaskTokenUsageIndicator` and omits missing/zero usage. Workbench token-usage API rows now redact `raw_usage` and metadata before returning so provider raw usage, prompts, tool arguments, object URIs, and credentials do not leave the backend boundary. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestGetTaskThreadTokenUsageHandler(ReturnsRowsAndAggregate|CanIncludeChildRuns)' -count=1`, and the broader Workbench handler command under上线验收. |
| Token-每轮用量展示 | 待验收 | DeerFlow-style token display modes and assistant per-turn usage are visible on task detail. | Code-complete: top-bar `Tokens` popover now shows Chinese `输入`、`输出`、`总计` plus display modes `关闭`、`总览`、`每轮`、`调试`; default `每轮` maps safe `/token_usage` rows by assistant `run_id` and displays per-turn `Tokens / 输入 / 输出 / 总计`, while omitting provider/raw_usage/step_name. Verified with `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "token usage"`, full `task-detail.test.tsx`, and `npx tsc --noEmit --project tsconfig.json`; real browser screenshot remains manual验收. |
| Token-子智能体用量聚合 | 已完成 | Child subagent cards can use grouped run aggregates without one request per child. | Existing subagent loader calls token usage once per parent with `include_child_runs=true`, maps `run_aggregates` by child `run_id`, displays bounded provider/model attribution and token/call metadata, and omits missing child usage. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`. |
| Token-成本快照 | 延后(P1) | N/A | Only display cost when backend already provides safe positive cost and one currency. |

### 5. 上线验收和回归

| Subtask | Status | P0 Acceptance | Notes |
| --- | --- | --- | --- |
| 上线验收-Atlas migration 状态 | 已完成 | Local Atlas can inspect/apply pending migrations without drift. | Local Atlas v0.35.0 `migrate validate` passes. External MySQL schema apply completed through the ignored debug env target, and follow-up dry-run reports `Schema is synced, no changes to be made`. Fixed `agent_files` MySQL key-length compatibility by indexing a 64-char `virtual_path_hash` while preserving full `virtual_path`. |
| 上线验收-生成 API 合约 | 已完成 | IDL/generated Workbench clients are in sync after touched API changes. | Backfilled existing task-thread, run, token, artifact, resume, cancel, retry, and create-task-thread contracts into `idl/workbench/task.thrift`; added api-schema contract coverage so task-thread generated clients cannot exist without thrift service methods, and `ListTaskThreadRuns` must map `parent_run_id` as a query parameter for explicit child-run listing. Verified with `rushx test -- __tests__/workbench-task-contract.test.ts __tests__/workbench-task-memory.test.ts` and `npm run test -- __tests__/workbench-task-contract.test.ts`. `rushx update` succeeds but currently rewrites unrelated generated files' formatting/license headers, so that generator-wide churn is excluded from P0 commits. |
| 上线验收-后端单测 | 已完成 | Targeted backend package tests pass for touched code. | P0 canonical-create backend validation passed with `go test ./application/agentthread -run 'TestApplicationCreate(TaskThread|Run)' -count=1`, `go test ./api/handler/coze -run 'TestCreateTaskThreadHandlerCreatesThreadRunAndInitialMessage' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'Test(CreateTaskThread|CreateTaskThreadRun|AppendTaskThreadMessage|GetTaskThread|ListTaskThread)' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestCancelTaskThreadRunHandler(CancelsPendingRun|TransitionsRun)' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestGetTaskThreadTokenUsageHandler(ReturnsRowsAndAggregate|CanIncludeChildRuns)' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'Test(ListTaskThreadRunEventsHandler(RedactsUnsafePayload|ReturnsEvents)|StreamTaskThreadRunEventsWritesEventsAndDone|TaskThreadRunEventStreamStopsForInterruptedRun)' -count=1`, `go test ./api/router/coze -run 'TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1`, plus existing P0 Skills/MCP/Tools targeted commands. A too-broad handler test command still hits known non-P0 Workflow/Conversation paths and Mockey gcflags requirements; use targeted package commands for P0 cutline evidence. |
| 上线验收-前端类型/单测 | 已完成 | Targeted app/package typecheck or unit tests pass for touched UI. | Frontend validation passed with `rushx test -- src/pages/workbench/__tests__/workbench.test.tsx`, `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "legacy task id is zero string"`, `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`, `rushx test -- __tests__/workbench-task-contract.test.ts __tests__/workbench-task-memory.test.ts`, and `make fe`. Build warnings are limited to the existing Browserslist/caniuse-lite and typeless package warnings. |
| 上线验收-桌面端任务冒烟 | 进行中 | Manual or automated browser path covers P0 task lifecycle. | Partial browser smoke passed for create, ADK execution, detail refresh, message rendering, and token usage on local `127.0.0.1:8888` with `AGENT_THREAD_RUNTIME_DEFAULT=eino_adk`, `AGENT_THREAD_EINO_ADK_ENABLED=true`, and `AGENT_THREAD_WORKER_ENABLED=true`. Live API smoke passed for pending cancel, task-level retry metadata, and task-memory search/edit/delete/restore/import/export after the MySQL search fix. Remaining before release candidate: manual browser confirmation for visible cancel/retry button path and visual task-memory panel because the in-app browser automation timed out on DOM/screenshot capture. |
| 上线验收-部署配置清单 | 已完成 | Document required env vars, disabled defaults, middleware services, and rollback note. | Added `docs/superpowers/plans/2026-06-27-deerflow-p0-release-readiness.md` with required services, env switches, disabled defaults, verification commands, browser smoke checklist, and rollback notes. Secrets stay out of tracked files. |
| 上线验收-已知问题清单 | 已完成 | Known unrelated failures and deferred P1/P2 items are documented before release. | Known P0 issues and explicit P1/P2 deferrals are listed in the release readiness doc so P0 does not absorb Phase 2 work. |

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
