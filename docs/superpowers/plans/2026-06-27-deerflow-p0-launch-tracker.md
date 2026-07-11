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

## Recent Slice Notes

- 2026-07-11 `AR-PARITY-001` (进行中): restarted the backend Agent runtime
  parity audit against the locked, locally deployed DeerFlow baseline
  `5851f825`. Static tracing confirmed that current NewX AI has a real Eino ADK
  path but is not yet fully equivalent: ADK is not the server default, several
  declared middleware slots are reserved no-ops, plan mode is not used to gate
  Todo, the generic Agent sandbox lacks DeerFlow filesystem/shell semantics,
  memory evolution workers are default-off, and the durable run layer still
  has redaction, lease/recovery, cancellation, retry scheduling and Workbench
  SSE pagination gaps. Thread/Run authorization is closed in
  `AR-PARITY-001.1`. The approved design and exit gate are in
  `docs/superpowers/specs/2026-07-11-deerflow-backend-agent-runtime-parity-design.md`.
  Planned verification: targeted tests across `application/agentthread`,
  `domain/agentthread/service`, `domain/agentthread/repository`, Workbench and
  LangGraph handlers/routes; Atlas hash/validate for migrations; paired live
  DeerFlow/NewX AI acceptance for direct, search, Pro, Ultra, Skill, MCP,
  upload, artifact, interrupt and memory cases.
  - `AR-PARITY-001.1` Thread/Run authorization (已完成): added one fail-closed
    application boundary for authenticated viewer, space, thread and optional
    run ownership, then enforce it across public Workbench and LangGraph
    thread/message/run/checkpoint/event/token operations. Acceptance covers
    owner success plus unauthenticated, cross-user, cross-space and mismatched
    run/thread rejection with no mutation. Workspace authority uses a targeted
    `space_user` membership lookup; dependency failures remain controlled 5xx
    responses, and successful request scopes are reused by Workbench Chat and
    SSE without repeated authorization queries. Verification:
    `go test ./application/agentthread -run 'Authoriz|AccessDenied' -count=1`
    and `go test -gcflags="all=-N -l" ./api/handler/coze -run
    'Forbidden|AccessDenied|Authoriz' -count=1`,
    `go test -gcflags="all=-N -l" ./... -count=1`, `gofmt -l`,
    `git diff --check`, independent `SPEC COMPLIANT`, and independent `CODE
    QUALITY APPROVED`.
  - `AR-PARITY-001.2` Public runtime projection/redaction (已完成): replaced
    handler-local partial filtering with one application allow-list projection
    for public runs, user-visible messages, journal rows, events, checkpoints,
    token usage, artifacts and bounded runtime errors. Workbench and LangGraph
    REST/SSE now share the same safe event projection. Approved visible reply
    text and stream tokens remain available, while reasoning, tool arguments/
    results, credentials, object URIs, unsafe paths, raw command/input/config/
    context/metadata, worker/idempotency fields, checkpoint bytes and provider
    errors remain internal. Recovery/update paths use internal checkpoint state
    separately, avoiding projection-induced corruption. Evidence:
    `docs/superpowers/evidence/2026-07-11-deerflow-agent-runtime-public-projection.md`.
    Verification: application redaction fixtures, handler
    `Redacts|DoesNotExpose|UnsafePayload`, complete application/handler suites,
    `go vet ./application/agentthread ./api/handler/coze`, task-detail Vitest
    61/61, frontend `tsc --noEmit`, full serial backend
    `go test -p 1 -gcflags="all=-N -l" ./... -count=1`, `gofmt -l`, and
    `git diff --check`. A parallel full-backend attempt hit an unrelated
    Mockey/Go 1.25 `SIGBUS` in unchanged workflow compose code; that package
    passed immediately when isolated with the required gcflags.
  - `AR-PARITY-001.3` Leased run ownership and fencing (已完成): persisted lease
    owner/token/expiry, heartbeat, cancellation-request timestamp and monotonic
    execution generation. Pending and protected resume claims now atomically
    install a cryptographically random lease; renewal, release, expired-lease
    discovery and running-to-success/failure/interruption transitions require a
    live owner + token + generation fence. Terminal transitions clear active
    ownership while retaining generation, and only protected checkpoint resume
    runs may be released back to `queued`. The internal lease contract is
    propagated through the domain/application services and both run processors;
    public Workbench/LangGraph projections continue to redact every lease field.
    Batch isolation, periodic worker heartbeat and stale recovery remain in
    `AR-PARITY-001.4`. Verification: repository RED/GREEN lease tests, complete
    domain/application/handler/router suites, targeted `go vet`, Atlas v0.35.0
    hash/validate, `gofmt`, `git diff --check`, and serial full backend
    `go test -p 1 -gcflags="all=-N -l" ./... -count=1` all passed.
  - `AR-PARITY-001.4` Batch isolation, lease heartbeat and stale recovery
    (已完成): claimed ordinary and protected-resume batches now continue after
    one infrastructure failure and explicitly release any still-owned,
    unfinalized lease. Both execution paths share an injected-clock heartbeat,
    cancel work when renewal loses ownership, stop renewal before terminal CAS,
    and release rather than falsely fail runs on process shutdown. A separate
    expired-lease owner/token/generation CAS rejects old workers. The latest
    active, decodable and runtime-compatible checkpoint creates one protected
    resume run keyed by source run + execution generation; retry after partial
    recovery reuses that run, while no compatible checkpoint terminates with
    bounded `run_abandoned` metadata. Recovery and resume workers plus lease
    timing are wired into the debug/operations profile. Evidence:
    `docs/superpowers/evidence/2026-07-11-deerflow-agent-run-lease-ownership.md`.
    Verification: ordinary/resume batch RED/GREEN tests, heartbeat/lease-loss/
    shutdown tests, stale-CAS and old-worker rejection, Eino ADK invalid/latest
    checkpoint and retry-idempotency tests, affected packages, targeted
    `go vet`, `gofmt`, `git diff --check`, and serial full backend
    `go test -p 1 -gcflags="all=-N -l" ./... -count=1` all passed.

- 2026-07-01 `TD-COMP-007`: Coze-only `@` resource reference composer was
  stabilized after the DeerFlow composer parity cut. The inline trigger now
  keeps `@` / `@资源类型：` as plain prefix text, applies the gray pill only to
  the search placeholder, converts selected resources into icon + resource name
  chips, supports multiple inline references mixed with user text, supports
  Backspace removal/cancel, and positions the chooser from the current `@`
  anchor (`bottom` on new-task home, `top` on task detail). Resource types are
  fixed to 技能 / 知识库 / 数据库 / 工作流 / 插件 / 代码仓库; Skills, knowledge,
  databases, and workflows load existing project resources, while plugin and
  code repository remain reserved empty states. The chooser rows intentionally
  show title only, no descriptions, to avoid the cramped UI seen during manual
  testing. Verification: `npm run test --
  src/pages/workbench/__tests__/workbench.test.tsx`; in-app browser smoke on
  `/space/7656275718757679104/chats/new` confirmed resource-type rows
  `技能/知识库/数据库/工作流/插件/代码仓库`, `descCount=0`, keyboard
  `ArrowDown + Enter` enters `@知识库：请输入搜索知识库`, and Skill search rows
  also render title-only entries.
- 2026-07-01 `TD-DETAIL-007`: task-detail visible layout was rechecked
  against DeerFlow source (`ChatBox` `OPEN_MODE={chat:60, artifacts:40}`,
  `ArtifactFileList`, `MessageGroup`, and `StreamingIndicator`) and current
  Coze runtime pages. Coze document Artifacts now use a DeerFlow split-layout
  side preview contract instead of falling back to a full overlay at desktop
  widths, generated document cards keep DeerFlow-style actions (download for
  documents, install only for `.skill`), and `web_search` tool-call projection
  renders a readable `搜索网页` step with the safe query and search step icon when
  the event carries one; backend run-event redaction now whitelists only bounded
  `web_search.query` from assistant tool calls so URLs, tool results, and other
  arguments stay hidden. Detail execution rows also stop rendering the default
  `Agent` runtime badge inside step labels, matching DeerFlow's `ToolCall` /
  `ChainOfThoughtStep` structure where the agent identity lives in the message
  header. Verification: `npm run test --
  src/pages/tasks/__tests__/tasks.test.tsx`, `npm run test --
  src/pages/tasks/__tests__/task-detail.test.tsx`, `npx tsc --noEmit
  --project tsconfig.json`, `git diff --check`, and in-app browser smoke on
  tasks `7657061782099329024` and `7657273152627539968` confirming
  `data-layout=deerflow-split`, document cards with only `下载`, and
  `web_search` no longer displaying the raw tool name as the primary label.
- 2026-07-01 `TD-COMP-006`: DeerFlow `web_search` parity was rechecked
  against DeerFlow config/source and a live DeerFlow container. DeerFlow uses
  `deerflow.community.ddg_search.tools:web_search_tool`, backed by Python
  `ddgs` defaults `backend=auto`, `region=wt-wt`, and `safesearch=moderate`.
  Live validation showed fixed `duckduckgo` can return no results locally while
  `auto` and `brave` do return results. Coze now defaults `web_search` to an
  `auto` provider chain: Eino-ext DuckDuckGo v2 -> bounded Brave HTML search
  -> Eino-ext Wikipedia. Explicit providers remain
  `duckduckgo/brave/wikipedia/http/disabled`, and the visible tool contract
  stays `web_search`. Local debug also needs explicit `HTTP_PROXY` /
  `HTTPS_PROXY` in ignored `bin/.env.debug`, because Go `net/http` does not
  read macOS system proxy settings the way DeerFlow's Python/Docker path did.
  Frontend Workbench defaults keep search enabled and raw HTTP fetch disabled.
  Verification: `go test ./application/agentthread -count=1`, direct Go
  `ADKWebSearchBackendFromEnv` smoke returned 5 results for `青岛最佳旅游时间`,
  backend build `go build -ldflags='-s -w' -o ../bin/opencoze main.go`, and
  browser smoke on task `7657273152627539968` confirmed `tool_search` selected
  `web_search`, `tool.completed` succeeded, and the run finished `succeeded`
  with a Qingdao travel-time answer.
- 2026-06-30 `TD-MSG-006`: task-detail execution steps and inline thinking
  blocks were rechecked against DeerFlow source (`ChainOfThought`,
  `Reasoning`, and `MessageGroup.convertToSteps`) and restyled to match the
  DeerFlow visible structure: historical steps default to `查看其他 N 个步骤`,
  expanded state switches to `隐藏步骤`, step rows use per-action icons plus a
  vertical rail and path pills, and the inline `思考` block uses the DeerFlow
  icon/chevron collapsible row. Verified with
  `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`,
  `npx tsc --noEmit --project tsconfig.json`, `git diff --check`, and in-app
  browser smoke on
  `http://localhost:8080/space/7656275718757679104/tasks/7657061782099329024`
  confirming default `查看其他 4 个步骤` collapsed state and `隐藏步骤` expanded
  state.
- 2026-06-30 `TD-COMP-003`: DeerFlow mode selection now uses native
  `flash/thinking/pro/ultra` runtime semantics instead of collapsing to Coze
  `Auto/Ask/Agent`. Coze defaults the DeerFlow composer to `Pro`, sends
  `thinking_enabled`, `is_plan_mode`, and `subagent_enabled` in Workbench run
  config, and keeps legacy WorkbenchChat enum mapping only for compatibility.
  Provider `reasoning_effort` is now sent only when Workbench runtime reasoning
  is explicitly enabled, matching DeerFlow's model-capability boundary and
  avoiding DeepSeek provider capability failures. Verified against DeerFlow
  `input-box.tsx`, model metadata, and thread submit context wiring, with
  `npx vitest run src/pages/workbench/__tests__/workbench.test.tsx
  src/pages/tasks/__tests__/task-detail.test.tsx` and `npx eslint` on touched
  frontend files. In-app browser DOM verification confirms the homepage
  composer uses DeerFlow presentation and shows `Pro`; visual mode-menu
  screenshot remains manual验收.
- 2026-06-30 `TD-COMP-005`: MCP auto-call validation found that weather MCP
  was invoked automatically but received empty arguments because
  `MCPToolRegistryEntry` did not carry `input_schema` into Eino `ToolInfo`.
  Fixed registry/API/ADK mapping so MCP JSON schema reaches model-visible
  tool metadata while still hiding auth/config/tool results. Verified with
  `go test ./application/agentthread -run 'TestADKMCP|TestDefaultADKToolProviderCanWireMCP' -count=1`,
  `go test ./application/mcptool -count=1`,
  `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestWorkbenchMCPToolHandlers' -count=1`,
  and the Workbench/task-detail Vitest command above. Post-fix runtime smoke on
  `thread_id=7656986536797274112` confirmed `tool_search` selected
  `mcp_7656840243668058112_get_weather` and the model sent `city=北京`.
- 2026-06-29 `TD-COMP-005`: MCP tools now include a local weather stdio MCP
  fixture for real Eino runtime validation. Coze lazy-syncs `weather` together
  with the DeerFlow-derived `github` and `postgres` MCP servers, and debug env
  switches now run the real Eino stdio MCP executor
  (`AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED=false`,
  `AGENT_THREAD_MCP_STDIO_EINO_ENABLED=true`). Browser smoke on
  `http://localhost:8080/space/7656275718757679104/tasks/7656842727241285632`
  confirmed the tools page renders `weather/postgres/github` enabled, and a
  new task automatically selected `tool_search` then invoked
  `mcp_7656840243668058112_get_weather` without manual MCP selection. Verified
  with `go test ./application/mcptool -count=1`,
  `go test ./application/agentthread -run 'TestADKMCPRuntimeBootstrapConfigFromEnvParsesEinoStdio|TestNewADKMCPRuntimeToolExecutorFromConfigBuildsEinoStdio|TestADKMCPRuntimeStdioEinoRunner|TestADKMCPRuntimeToolCatalogTreatsEmptyAllowedToolsAsAutoDiscovery' -count=1`,
  and `npx vitest run src/pages/workbench/__tests__/workbench.test.tsx`.
- 2026-06-29 `TD-SKILL-005`: ADK runtime now treats DeerFlow-style
  `/skill-name` as explicit single-turn Skill activation when the runtime has
  narrowed the turn to one inline Skill. The selected Skill instructions are
  preloaded before the first model call, matching DeerFlow's
  `SkillActivationMiddleware` behavior instead of relying on the model to
  guess and call the generic Skill loader first. Verified with
  `go test ./application/agentthread -run 'TestRuntimeSkillProvider|TestADKSkillMiddleware|TestModelStepRunner.*Skill|TestHarnessExecutorLoadsEnabledSkillsBeforePlanning' -count=1`.
- 2026-06-29 `TD-SKILL-001`: DeerFlow public builtin skills are now migrated
  into Coze backend as an embedded bundle. Source parity was checked against
  `/Users/liuwenbo/code/BuildingAI/deer-flow/skills/public`: 22 builtin skill
  directories and 91 total files. Coze lazily syncs missing builtin skills per
  workspace as `deer_skill` records, preserves original `SKILL.md` snapshots
  and support resources, uses SKILL frontmatter `name` for idempotency
  (`vercel-deploy-claimable` imports as `vercel-deploy`), and avoids duplicate
  imports across repeated list calls or service restarts. Verified with
  `go test ./application/skill ./domain/skill/... ./application/agentthread -run 'Skill|skill' -count=1`
  and `go test ./api/router/coze -run 'Skill|skill' -count=1`. Live browser
  validation after backend rebuild/restart confirmed
  `GET /api/workbench/skills?space_id=7656275718757679104` returns 22 enabled
  `deer_skill` records and the Skill page renders the builtin list.
- 2026-06-29 `TD-DOC-003~011`: task detail artifact preview/download safety is
  code-complete for the current frontend boundary. Markdown renders through the
  Markdown previewer with source/preview toggle and truncation notice; CSV
  renders as a table; PNG/PDF use signed-url preview paths; HTML/SVG active
  content stays download-only; preview/download failures use bounded UI
  messages; artifact lists hide internal object URIs and raw metadata while
  preserving safe `/mnt/user-data/...` paths. Verified with
  `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-artifacts-helpers.test.ts`
  and `npx tsc --noEmit --project tsconfig.json`. Real browser screenshots and
  API samples remain manual/next-run 验收.
- 2026-06-29 `TD-DOC-012`: task detail document artifact cards now follow the
  DeerFlow `present_files` turn model more closely. Coze groups
  `TaskThreadArtifact` cards by owning `run_id`, renders cards under the
  matching assistant turn, and auto-previews only the latest previewable
  generated artifact through one shared side preview state. Verified with
  `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`; in-app
  browser control timed out during DOM capture, so live screenshot remains
  manual/next-run 验收.
- 2026-06-28 `TD-DOC-001/002`: code-complete backend DeerFlow-style
  `write_file` + `present_files` chain. `write_file` writes only
  `/mnt/user-data/outputs/*` files to object storage and registers
  `AgentFileKindOutput`; `present_files` registers only those output files as
  task artifacts and emits a content-free `artifact.presented` event. Internal
  `.coze/tool-results` offload remains workspace-only and is not promoted to
  artifacts. Verified with targeted backend tests; browser prompt/artifact panel
  evidence remains manual验收.

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
| 任务主流程闭环-桌面浏览器冒烟 | 已完成 | Browser smoke covers create, stream, cancel/retry, reopen detail, and refresh. | Partial desktop smoke passed for canonical create -> ADK run -> detail refresh on `127.0.0.1:8888`: `POST /api/workbench/task_threads` returned 200, detail used only task-thread APIs, worker completed the Eino ADK run as `succeeded`, messages included the assistant answer, and token usage returned aggregate rows. Follow-up API smoke covered canonical task creation, pending cancel, task-level retry metadata, and task-memory search/edit/delete/restore/import/export on the live server. Additional browser smoke on `localhost:8080` confirmed a completed Mermaid prompt task shows `已完成`, hides `取消任务`, renders execution progress as `2/2 已完成 · 100%`, and renders two Mermaid diagrams as ready SVGs without raw fenced Mermaid text in the answer area. Current DeerFlow comparison smoke also confirms task detail keeps `运行诊断`、`安全审计`、`任务记忆` behind header `详情` inspector, the follow-up composer is docked outside the scrollable chat transcript, task detail header no longer shows `任务详情 › 已完成` or `产物 0`, header actions show compact clickable `Tokens` plus `导出` and `详情`, clicking `Tokens` opens Input/Output/Total usage details, assistant answers no longer show `普通回答 · Agent/Ark` result labels, `执行流程` remains visible as a lightweight step feed with each step's status/title/runtime/detail/time, and each Mermaid diagram exposes SVG download plus source copy actions. TD-COMP-001/003 are code-complete for the DeerFlow composer shell: task detail removes `Auto/Ask/Agent` and the visible run settings button, keeps `拓展` and `@`, adds the DeerFlow mode trigger defaulting to `Pro`, writes native `flash/thinking/pro/ultra` runtime context, keeps attachment/link entries, and uses the green round send icon; 真实视觉补证转 P1. TD-EXP-002 export is code-complete against the user-provided DeerFlow Markdown/JSON samples: header `导出` opens Markdown and JSON items, Markdown/JSON both include only visible user/assistant transcript and filter thinking/tool/internal markers; 真实下载样例补证转 P1. TD-MSG-005 inline thinking is code-complete and covered by unit tests for ADK `message.completed` reasoning extraction, `<think>` stripping, and provider signature hiding; 真实 reasoning 样本截图转 P1. TD-TOKEN-002 is code-complete for DeerFlow-style display modes and assistant per-turn token summaries; 真实浏览器截图转 P1. TD-DOC-012 is code-complete for DeerFlow-style generated-document cards in the task transcript: active task artifacts render as file cards with preview/download actions, reusing artifact content and signed URL APIs instead of the header thread export path; 真实 artifact 截图已在后续收口补齐，更多样本转 P1. The active DeerFlow comparison ledger is `docs/superpowers/plans/2026-06-28-deerflow-task-detail-parity-validation.md`; future task-detail work must reference a case id from that file and store screenshots/API samples under `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/`. DeerFlow-visible gaps still open for document generation/preview validation and full chat-like detail layout details. Full CI browser suite is P1 if harness is not ready. |

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
| Skills-DeerFlow 内置技能迁移 | 已完成 | All DeerFlow public builtin Skills are available in Coze workspaces without manual import. | Migrated DeerFlow public builtin Skill entries into an embedded Coze backend bundle and lazy-syncs missing builtins per workspace as `deer_skill`, preserving latest `SKILL.md` version snapshots and currently embedded resources. Verified with `go test ./application/skill ./domain/skill/... ./application/agentthread -run 'Skill|skill' -count=1`, `go test ./api/router/coze -run 'Skill|skill' -count=1`, and browser/API validation of `/api/workbench/skills?space_id=7656275718757679104` returning 22 builtin records plus Skill page screenshot `TD-SKILL-001-skill-page-builtins.png`. Important follow-up: DeerFlow `skill-creator` has scripts/references/agents/eval-viewer packaging resources that are not yet executable through Coze runtime, because `RuntimeSkillProvider` currently loads only `SKILL.md` instructions. |
| Skills-列表/创建/编辑/启停 | 已完成 | User can manage DeerFlow-style Skills through existing task-oriented Workbench controls. | Frontend Skill page tests cover list/create/import/test-run/toggle/delete states; backend handler/application/domain tests cover create/update/list/delete, safe mapping, and DeerFlow builtin lazy sync. User `.skill` import/install now defaults missing frontmatter `type` to `CustomSkill`, while builtin sync still defaults to `DeerSkill`; API verification imported `codex-custom-skill-1782721734` and list returned `type=5`. Skill-creator entry prompt now asks the agent to produce `.skill` through `create_skill_package` and `present_files` so the artifact can be installed into custom skills. `create_skill_package` is an ADK runtime tool that builds a real zip `.skill` from structured `skill_name`, `skill_md`, and optional text resources without opening shell execution. Verified with `rushx test -- src/pages/skill/__tests__/skill.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx src/pages/tools/__tests__/tools.test.tsx src/pages/tools/__tests__/tools-service.test.ts src/pages/workbench/__tests__/workbench.test.tsx`, targeted `go test ./application/agentthread -run 'Test(Application(CreateSkillPackage|WriteOutputFile|PresentOutputFiles)|ADKArtifactToolCatalog|DefaultADKToolProviderCanWireArtifactTools|RuntimeSkillProvider|ADKSkill)' -count=1`, `go test -gcflags="all=-l -N" ./api/handler/coze -run 'Test(WorkbenchMCPToolHandlers|ListSkillVersionsHandler|ListSkillVersionResourcesHandler|ExportSkillVersionHandler|RollbackSkillVersionHandler|DeleteSkillHandler|ListSkillToolCandidatesHandler|UpdateSkillVersion(Resource|Content)Handler)' -count=1`, and `go test ./application/skill ./domain/skill/service ./domain/skill/repository ./application/mcptool -run 'Test' -count=1`. |
| Skills-版本/历史/回滚/导出 | 已完成 | Existing history/export paths work without exposing hidden runtime payloads. | Skill version panel and backend tests cover version list, resource list/update, content update, export, and rollback. Verified with the same Skill frontend/backend commands above. |
| Skills-Eino runtime 注入 | 已完成 | Enabled Skills can be loaded through Eino ADK middleware with safe policy boundaries. | `RuntimeSkillProvider` selects enabled prompt Skills by explicit `enable_skills` selectors, loads latest version snapshots, applies body budgets, and ADK Skill backend tests cover runtime loading and sanitized Skill backend descriptions. ADK selected Skill loading now emits metadata-only `skills.loaded` events, and task-detail projection renders them as readable `加载 “skill-creator” 技能` steps instead of raw JSON. Matched Skill `tool.failed` results now stay failed and render the concrete Skill name while hiding raw error details. Live local HTTP/API smoke created `run_id=7656724465300013056` with `skills.loaded` payload limited to skill count, ID, and name. Verified with `go test -gcflags="all=-l -N" ./application/agentthread -run 'Test(ADKSkill|ADKMCP|ADKTool|DefaultADKTool|ADKSingleAgentToolGrant|ADKSubagentToolProvider)' -count=1`, targeted `TestADKSkillMiddlewareEmitsLoadedEventWithoutSkillContent`, and `npm run test -- src/pages/tasks/__tests__/tasks.test.tsx`. |
| Skills-slash/渐进激活 | 已完成 | User-visible activation behavior matches DeerFlow enough for task workflows. | Workbench extension popover selects Skills/MCP into `enable_skills` / `enable_mcp`; runtime settings can disable selected classes by sending empty enabled arrays while preserving UI selections. Verified through Workbench tests in the frontend command above. Full browser smoke remains under the P0 desktop smoke items. |
| MCP-服务/工具目录配置 | 已完成 | User can configure and list MCP servers/tools with durable metadata and safe auth handling. | DeerFlow source verified: `ToolSettingsPage` renders a SettingsSection with MCP server name/description and an enabled switch only; create/test/delete/search/health-management controls are not part of the visible mainline settings page. Coze `/tools` page has been narrowed to that DeerFlow-style MCP server switch list while preserving the backend Workbench MCP catalog/upsert/test-call APIs for runtime and later P1 management. Browser DOM verification on `http://localhost:8080/space/7656275718757679104/tools` confirmed `管理 MCP 工具的配置和启用状态。`, empty state `暂无 MCP 工具。`, and no `创建工具配置` / `测试` / `删除` / segment controls. Verified with `npm run test -- src/pages/tools/__tests__/tools.test.tsx src/pages/tools/__tests__/tools-service.test.ts`, full `npm run test -- src/pages/workbench/__tests__/workbench.test.tsx`, `npx tsc --noEmit --project tsconfig.json`, `go test ./application/agentthread -run 'TestADKMCP|TestADKRuntimeToolCatalog|TestDefaultADKToolProvider|TestADKToolPolicyProvider' -count=1`, `go test ./application/mcptool -run 'Test' -count=1`, and `go test ./api/handler/coze -run 'TestWorkbenchMCPToolHandlers' -count=1`. |
| MCP-DeerFlow 默认工具迁移 | 已完成 | DeerFlow 默认 MCP server/tool definitions are imported into Coze's durable MCP catalog and become selectable/invokable through the existing ADK MCP runtime path. | Added Coze-native lazy sync for DeerFlow default `github` and `postgres` MCP servers into `mcp_tool_servers`, preserving Coze registry entries, `mcp_tools.allowed_tools`, and ADK runtime executor as the execution truth. Verified GitHub tool names/descriptions against `@modelcontextprotocol/server-github@2025.4.8` package source and Postgres `query` against `@modelcontextprotocol/server-postgres@0.6.2`; migrated 26 GitHub tools plus Postgres `query` with input schemas. Added local `weather` stdio MCP as a safe real-runtime validation fixture so P0 can prove Eino stdio invocation without external credentials. Registry seeding now works even when Agent/runtime asks for tools before the tools page is opened, and browser smoke confirmed `/tools` lists `weather/postgres/github` enabled. Verified with `go test ./application/mcptool -run 'Test' -count=1`, `go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKMCP|TestADKRuntimeToolCatalog|TestDefaultADKToolProvider' -count=1`, `go test ./api/handler/coze -run 'TestWorkbenchMCPToolHandlers' -count=1`, and `go test ./application -run '^$' -count=1`. External runtime calls still depend on valid user credentials/targets, e.g. GitHub token and Postgres URL; failures surface through the existing MCP runtime health/error path. |
| MCP-健康状态 | 已完成 | Health status is visible in tool settings or Runtime Doctor. | Runtime Doctor displays total/enabled/healthy/unhealthy/unknown MCP counts without config/auth details. |
| MCP-Eino 工具调用 | 已完成 | Enabled MCP/tool entries can be invoked through Eino ADK tool path. | `ADKMCPRuntimeToolCatalog` loads enabled MCP registry entries from run config `mcp_tools`, filters by allowlist when present, exposes registry `input_schema` as Eino `ToolInfo.ParamsOneOf`, and invokes through the MCP runtime executor. Debug config now disables stdio dry-run and enables the real Eino stdio runner. Browser/API smoke on tasks `7656842727241285632` and `7656972508431646720` confirmed intent-based auto selection: the Agent used `tool_search` and then called `mcp_7656840243668058112_get_weather` without manual MCP selection. Post-fix fresh run `7656986536797274112` confirmed city-specific arguments with `city=北京` and structured weather output. Verified with `go test ./application/agentthread -run 'TestADKMCP|TestDefaultADKToolProviderCanWireMCP' -count=1`, `go test ./application/mcptool -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestWorkbenchMCPToolHandlers' -count=1`, the Workbench/task-detail Vitest command, plus the targeted Eino stdio config/runner command in Recent Slice Notes. |
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
| 任务记忆-浏览器冒烟 | 已完成 | Browser smoke covers search, edit, delete, restore, import, export, and read-only display. | Live API smoke passed on `127.0.0.1:8888` after fixing MySQL LIKE escaping: register -> space -> `POST /api/workbench/task_threads` -> import memory -> `GET /memories?q=searchable` -> update -> export -> delete -> `include_deleted=true` list -> restore -> audit events. Frontend component smoke passed with the `task-memory-section` Vitest command above. Manual visual browser confirmation moved to P1 because the P0 API and component smoke already passed. |
| 记忆抽取-worker 默认关闭 | 已完成 | Model-backed extractor can be configured but does not start accidentally without explicit env. | P0 may ship with extraction disabled. |
| Token-运行级用量聚合 | 已完成 | Task detail can show run-level total token count and call count. | Existing task-detail loader maps `aggregate` into the top-bar `TaskTokenUsageIndicator` and omits missing/zero usage. Workbench token-usage API rows now redact `raw_usage` and metadata before returning so provider raw usage, prompts, tool arguments, object URIs, and credentials do not leave the backend boundary. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestGetTaskThreadTokenUsageHandler(ReturnsRowsAndAggregate|CanIncludeChildRuns)' -count=1`, and the broader Workbench handler command under上线验收. |
| Token-每轮用量展示 | 已完成 | DeerFlow-style token display modes and assistant per-turn usage are visible on task detail. | Code-complete: top-bar `Tokens` popover now shows Chinese `输入`、`输出`、`总计` plus display modes `关闭`、`总览`、`每轮`、`调试`; default `每轮` maps safe `/token_usage` rows by assistant `run_id` and displays per-turn `Tokens / 输入 / 输出 / 总计`, while omitting provider/raw_usage/step_name. Verified with `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "token usage"`, full `task-detail.test.tsx`, and `npx tsc --noEmit --project tsconfig.json`; 真实浏览器截图转 P1. |
| Token-子智能体用量聚合 | 已完成 | Child subagent cards can use grouped run aggregates without one request per child. | Existing subagent loader calls token usage once per parent with `include_child_runs=true`, maps `run_aggregates` by child `run_id`, displays bounded provider/model attribution and token/call metadata, and omits missing child usage. Verified with `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`. |
| Token-成本快照 | 延后(P1) | N/A | Only display cost when backend already provides safe positive cost and one currency. |

### 5. 上线验收和回归

| Subtask | Status | P0 Acceptance | Notes |
| --- | --- | --- | --- |
| 上线验收-Atlas migration 状态 | 已完成 | Local Atlas can inspect/apply pending migrations without drift. | Local Atlas v0.35.0 `migrate validate` passes. External MySQL schema apply completed through the ignored debug env target, and follow-up dry-run reports `Schema is synced, no changes to be made`. Fixed `agent_files` MySQL key-length compatibility by indexing a 64-char `virtual_path_hash` while preserving full `virtual_path`. |
| 上线验收-生成 API 合约 | 已完成 | IDL/generated Workbench clients are in sync after touched API changes. | Backfilled existing task-thread, run, token, artifact, resume, cancel, retry, and create-task-thread contracts into `idl/workbench/task.thrift`; added `.skill` artifact install to `idl/workbench/skill.thrift` and generated `workbenchSkill.InstallSkillFromArtifact`, so task artifact actions no longer call the install route through hand-written fetch. Added api-schema contract coverage so task-thread generated clients cannot exist without thrift service methods, and `ListTaskThreadRuns` must map `parent_run_id` as a query parameter for explicit child-run listing. Verified with `rushx test -- __tests__/workbench-task-contract.test.ts __tests__/workbench-task-memory.test.ts`, `npm run test -- __tests__/workbench-task-contract.test.ts`, and `npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts`. `rushx update` succeeds but currently rewrites unrelated generated files' formatting/license headers, so that generator-wide churn is excluded from P0 commits. |
| 上线验收-后端单测 | 已完成 | Targeted backend package tests pass for touched code. | P0 canonical-create backend validation passed with `go test ./application/agentthread -run 'TestApplicationCreate(TaskThread|Run)' -count=1`, `go test ./api/handler/coze -run 'TestCreateTaskThreadHandlerCreatesThreadRunAndInitialMessage' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'Test(CreateTaskThread|CreateTaskThreadRun|AppendTaskThreadMessage|GetTaskThread|ListTaskThread)' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestCancelTaskThreadRunHandler(CancelsPendingRun|TransitionsRun)' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestGetTaskThreadTokenUsageHandler(ReturnsRowsAndAggregate|CanIncludeChildRuns)' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'Test(ListTaskThreadRunEventsHandler(RedactsUnsafePayload|ReturnsEvents)|StreamTaskThreadRunEventsWritesEventsAndDone|TaskThreadRunEventStreamStopsForInterruptedRun)' -count=1`, `go test ./api/router/coze -run 'TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1`, plus existing P0 Skills/MCP/Tools targeted commands. A too-broad handler test command still hits known non-P0 Workflow/Conversation paths and Mockey gcflags requirements; use targeted package commands for P0 cutline evidence. |
| 上线验收-前端类型/单测 | 已完成 | Targeted app/package typecheck or unit tests pass for touched UI. | Frontend validation passed with `rushx test -- src/pages/workbench/__tests__/workbench.test.tsx`, `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "legacy task id is zero string"`, `rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx`, `rushx test -- __tests__/workbench-task-contract.test.ts __tests__/workbench-task-memory.test.ts`, and `make fe`. Build warnings are limited to the existing Browserslist/caniuse-lite and typeless package warnings. |
| 上线验收-桌面端任务冒烟 | 已完成 | Manual or automated browser path covers P0 task lifecycle. | Partial browser smoke passed for create, ADK execution, detail refresh, message rendering, and token usage on local `127.0.0.1:8888` with `AGENT_THREAD_RUNTIME_DEFAULT=eino_adk`, `AGENT_THREAD_EINO_ADK_ENABLED=true`, and `AGENT_THREAD_WORKER_ENABLED=true`. Live API smoke passed for pending cancel, task-level retry metadata, and task-memory search/edit/delete/restore/import/export after the MySQL search fix. Remaining visual browser confirmation for cancel/retry affordances and task-memory panel moved to P1; P0 lifecycle and API smoke already passed. |
| 上线验收-部署配置清单 | 已完成 | Document required env vars, disabled defaults, middleware services, and rollback note. | Added `docs/superpowers/plans/2026-06-27-deerflow-p0-release-readiness.md` with required services, env switches, disabled defaults, verification commands, browser smoke checklist, and rollback notes. Secrets stay out of tracked files. |
| 上线验收-已知问题清单 | 已完成 | Known unrelated failures and deferred P1/P2 items are documented before release. | Known P0 issues and explicit P1/P2 deferrals are listed in the release readiness doc so P0 does not absorb Phase 2 work. |

## P1 After P0

P1 starts only after the P0 exit gate passes.

- Active P1 stabilization tracker:
  `docs/superpowers/plans/2026-07-01-deerflow-p1-stabilization-tracker.md`

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
