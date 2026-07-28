# Workbench Canonical UI Cutover And Source Contract Retirement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development` (recommended) or
> `superpowers:executing-plans` to execute this plan task by task. Keep the
> checkboxes current and stop at every hard gate.

**Goal:** 将 Workbench/Tasks UI 和仓库内 NewX parity client 完整切换到
`/api/workbench/threads/**`，在功能等价和线上页面回归通过后，物理删除
`/api/workbench/task_threads/**`、`/api/threads/**`，并在零使用审计通过后删除
`/api/runs/**`。最终只保留一个 canonical 前端 client 和一套现有
`agentthread.ApplicationService -> domain -> repository -> MySQL -> Eino ADK` 主链。

**Architecture:** React 页面只依赖 app-owned request/view model；唯一生产 transport
是 `CanonicalThreadClient`。它负责 canonical JSON、multipart、Blob、SSE、workspace
header、幂等键、错误和时间转换。后端 canonical handler 继续调用现有应用层，不新增
状态机、表、执行器或数据迁移。TaskThread V1 与 LangGraph HTTP adapter 只在 Gate A
验证期间保留，Gate B 从 IDL、路由、handler、生成 client 和测试中物理删除。

**Tech Stack:** React 18、TypeScript 5.8、Rsbuild、Vitest、Rush/pnpm、Go、Hertz、
Thrift/Hz、Eino ADK、MySQL、`@coze-arch/fetch-stream`、Codex in-app browser

---

## Fixed Baseline And Hard Boundaries

- Worktree：`/private/tmp/coze-studio-workbench-canonical-ui-cutover-retirement`
- Branch：`codex/workbench-canonical-ui-cutover-retirement`
- Baseline：`dev@1b663df3449f3bd2849b37b33ae2755c109269cd`
- Approved design：
  `docs/superpowers/specs/2026-07-28-workbench-canonical-ui-cutover-retirement-design.md`
- Canonical contract：47 个 `/api/workbench/threads/**` 方法与路径组合，保持请求、
  响应、权限、公开投影和错误语义不变。
- Source contracts：36 个 `/api/workbench/task_threads/**`、23 个
  `/api/threads/**`，Gate B 全量删除。
- Stateless compatibility：10 个 `/api/runs/**`，只有五项零使用证据齐全后删除。
- `idl/workbench/task.thrift` 同时拥有 Scheduled Task 合同。只删除 TaskThread DTO 和
  service 方法；`/api/workbench/scheduled_tasks/**`、Task Center 页面及其生成 client
  必须原样保留。
- `backend/application/skill/builtin_deerflow/**` 和
  `backend/internal/deerflowparity/deerflow_client.go` 中的 `/api/threads/**` 指向外部
  DeerFlow，不是本地来源路由，不做机械替换。
- 不修改数据库 schema、迁移、领域 entity、repository、Eino ADK、Worker、
  checkpoint bytes、Artifact/Memory 业务规则或页面布局。
- 最终不保留 `TaskThreadV1Client`、client selector、
  `WORKBENCH_THREAD_CLIENT_MODE`、fallback、shadow request、双写或双 SSE。
- canonical 失败直接返回 canonical 失败。写请求结果不明确时使用同一幂等键查询
  canonical 资源，禁止调用旧来源合同重放。
- 日志不得包含消息正文、Memory 内容、附件名、signed URL、credential、provider
  body、tool 参数/结果或 checkpoint bytes。
- 删除、合并本地 `dev` 和推送 `origin/dev` 分别遵循仓库确认门禁；本计划执行阶段
  不自动合并或推送。

## Route Outcome Matrix

| Route family | Gate A | Gate B final state |
| --- | --- | --- |
| `/api/workbench/threads/**` | 47 routes available | 47 routes available and primary |
| `/api/workbench/task_threads/**` | 36 routes retained for comparison | 36 routes absent/404 |
| `/api/threads/**` | 23 routes retained for comparison | 23 routes absent/404 |
| `/api/runs/**` | 10 routes retained while audit runs | absent/404 only after five-part audit |
| `/api/workbench/tasks*`, `/api/workbench/chat` | absent/404 | absent/404 |
| `/api/workbench/scheduled_tasks/**` | unchanged | unchanged |

## Target File Ownership

| Area | Files | Final responsibility |
| --- | --- | --- |
| Frontend app contract | `frontend/apps/coze-studio/src/pages/workbench/thread-client/types.ts`, `workbench-thread-client.ts` | App-owned requests, resources, pages, commands, errors, SSE events |
| Canonical transport | `canonical-fetch.ts`, `canonical-thread-client.ts`, `run-event-cursor.ts` | The only production Workbench Thread HTTP/SSE owner |
| Canonical projection | `adapters/canonical-thread-adapter.ts`, `legacy-page-response.ts` | Transport-to-app and app-to-current-page structures |
| Safe telemetry | `client-telemetry.ts` | Operation-level logs without content-bearing values |
| Page delegation | `pages/workbench/service.ts`, `pages/tasks/service.ts`, `task-memory-service.ts`, `task-usage-service.ts` | Preserve current exports while delegating to canonical client |
| Run stream | `pages/tasks/task-run-event-stream.ts`, `task-detail-hooks.ts`, `task-run-actions-hook.ts` | One stream for one committed active Run |
| Backend primary API | `backend/api/handler/coze/workbench_canonical_*.go` | Strict canonical HTTP adapter and public projection |
| Shared HTTP access | `backend/api/handler/coze/workbench_thread_access.go` | Authentication-derived viewer, Thread access context, terminal-state helper |
| Source retirement | `idl/workbench/task.thrift`, generated model/router/schema, `workbench_thread_service.go`, `langgraph_*_service.go` | Remove old HTTP contracts without touching application/domain code |
| Internal consumer | `backend/internal/deerflowparity/newx_client.go` | Use local canonical routes and workspace header |
| Evidence and runbook | `docs/superpowers/evidence`, `docs/superpowers/context`, `docs/superpowers/runbooks` | Auditable Gate A/Gate B results and current project truth |

---

### Task 1: Freeze The Baseline And Complete The Usage-Audit Inputs

**Files:**
- Create: `docs/superpowers/evidence/2026-07-28-workbench-source-contract-usage-audit.md`
- Create: `docs/superpowers/evidence/2026-07-28-workbench-canonical-cutover-gate-a.md`
- Modify: `backend/api/router/coze/workbench_canonical_thread_route_test.go`
- Modify: `backend/api/router/coze/workbench_thread_route_test.go`

- [x] **Step 1: Record the immutable route inventory**

Keep the existing exact 47-route canonical snapshot, 36-route TaskThread V1 snapshot and
23-route LangGraph Thread snapshot. Add an exact 10-route stateless Run snapshot so all four
families use method-plus-template comparisons rather than substring counts.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./api/router/coze \
  -run '^(TestWorkbenchCanonicalThreadRoutes|TestRegisterIncludesWorkbenchTaskThreadRoutes|TestRegisterIncludesLangGraphThreadRoutes|TestRegisterIncludesLangGraphRunRoutes)$' \
  -count=1
```

Expected: PASS while all source routes are still present. Save the exact route counts and current
HEAD in the Gate A evidence file.

- [x] **Step 2: Freeze current page-visible V1 behavior**

Run the current Workbench, Tasks, service, usage, Memory, Artifact and stream tests before any
transport edit:

```bash
cd frontend/apps/coze-studio
rushx test -- \
  src/pages/workbench/__tests__/workbench.test.tsx \
  src/pages/tasks/__tests__/tasks.test.tsx \
  src/pages/tasks/__tests__/tasks-service.test.ts \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/tasks/__tests__/task-run-actions-hook.test.tsx \
  src/pages/tasks/__tests__/task-memory-section.test.tsx \
  src/pages/tasks/__tests__/task-usage-service.test.ts
```

Expected: PASS. Record any pre-existing skipped or substituted scenario; do not silently convert a
baseline failure into a migration failure.

- [x] **Step 3: Write the five-part zero-use audit record**

The audit document must have separate evidence rows for:

1. production source and graph callers;
2. frontend, generated client, IM, scheduled task, internal tool and operations-script callers;
3. gateway/service access logs for `^/api/runs(?:/|$)` with authenticated business requests
   separated from 404/security probes;
4. registered external SDK consumers, public documentation promises and named owner;
5. release/acceptance matrices that require stateless Run behavior.

Use a fixed window from `2026-06-28T00:00:00+08:00` through the final Gate A validation time and
record log system, query, environment, result count and reviewer. Never paste credentials, cookies,
request bodies or user content into the document.

If access logs or consumer registration cannot be queried, mark item 3 or 4 `BLOCKED` and record
the exact missing source. Task 14 is then forbidden; Tasks 2-13 may continue.

- [x] **Step 4: Verify local caller classification**

Use codebase-memory first for `CreateLangGraphStatelessRun`,
`registerLangGraphCustomRoutes`, `CreateTaskThread` and NewX client call paths, then verify source
strings:

```bash
rg -n '/api/runs(?:/|\")|/api/threads(?:/|\")|/api/workbench/task_threads' \
  backend frontend scripts idl \
  --glob '!backend/application/skill/builtin_deerflow/**'
```

Classify every production hit in the audit. Expected current local callers:

- Workbench/Tasks UI calls `/api/workbench/task_threads/**`;
- NewX parity calls local `/api/threads/**` and two TaskThread product paths;
- route/handler/tests implement all three source families;
- no production caller invokes local `/api/runs/**`.

- [x] **Step 5: Capture the old-page browser control**

Using the in-app browser and the configured online workspace, record the current URL, account role,
workspace ID, visible task list/detail state, a newly created clearly named validation Thread, one
completed Run, one refresh, console status and request paths. Do not delete or bulk-update online
records.

Expected: the control uses `/api/workbench/task_threads/**`; no conclusion about canonical UI is
made from this run.

- [x] **Step 6: Commit the baseline evidence and route snapshot**

```bash
git add \
  backend/api/router/coze/workbench_canonical_thread_route_test.go \
  backend/api/router/coze/workbench_thread_route_test.go \
  docs/superpowers/evidence/2026-07-28-workbench-source-contract-usage-audit.md \
  docs/superpowers/evidence/2026-07-28-workbench-canonical-cutover-gate-a.md
git commit -m "test: freeze workbench source contract baseline"
```

### Task 2: Define The App-Owned Canonical Client Boundary

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/types.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/workbench-thread-client.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/index.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/fixtures.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/legacy-task-thread-reference.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts`

- [x] **Step 1: Write the failing transport-isolation test**

The test must reject `@coze-studio/api-schema`, `workbenchTask`, `workbenchThread`, route strings and
browser transport construction from `types.ts`, `workbench-thread-client.ts` and `index.ts`.
It must also assert the public client contract literal is only `canonical_v1`.

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts
```

Expected: FAIL because the app-owned boundary does not exist.

- [x] **Step 2: Define normalized resources**

Use string IDs, snake_case visible field names and epoch-millisecond times to preserve current
rendering. Define `WorkbenchThread`, `WorkbenchTodo`, `WorkbenchMessage`, `WorkbenchRun`,
`WorkbenchRunEvent`, `WorkbenchUpload`, `WorkbenchArtifact`, `WorkbenchArtifactScanJob`,
`WorkbenchTokenUsage`, `WorkbenchTokenUsageAggregate`, `WorkbenchMemory`, Memory/Guardrail/MCP
audit events and `HumanInteractionResponse`.

Do not expose provider payload, raw usage, tool arguments/results, checkpoint bytes or credentials.
Keep `worker_id` only on the page-facing Artifact scan job; canonical maps its safe `worker_ref`
into that field. Do not put worker identity on `WorkbenchRun`. Keep JSON-valued page fields as
strings where existing render helpers expect strings.

- [x] **Step 3: Define one workspace-scoped request convention**

Every client call requires `space_id` at the app boundary. Thread calls add `thread_id`; Run calls
add `run_id`. Define page, cursor, abort and idempotency options once:

```ts
export interface WorkbenchScopedRequest { space_id: string }
export interface WorkbenchThreadRequest extends WorkbenchScopedRequest { thread_id: string }
export interface WorkbenchRunRequest extends WorkbenchThreadRequest { run_id: string }
export interface WorkbenchPage<T> {
  items: T[];
  total: number;
  has_more: boolean;
  next_cursor?: string;
}
```

`space_id` is never copied into canonical JSON/query. The transport uses it only in
`X-Coze-Space-ID` and injects it into app-owned view models after a successful response.

- [x] **Step 4: Define the complete production interface**

`WorkbenchThreadClient` must expose the current page operations: search/create/get Thread,
list/append Message compatibility, suggestions, list/create/get/cancel/resume/retry Run, list and
subscribe Run events, Upload list/create/delete, Artifact list/content/signed URL/delete/restore/
scan/retry, token usage, Memory list/update/delete/clear/restore/import/export and all three audit
families. `readonly contract` is exactly `'canonical_v1'`.

`RunEventSubscription` is `{ close(): void; closed: Promise<void> }`. Subscription input requires
space, Thread and Run IDs, optional cursor, an `AbortSignal`, and callbacks for event/end/error.

- [x] **Step 5: Add paired transport fixtures without a production V1 client**

For each resource family, create one frozen V1 response fixture and one canonical response fixture
representing the same visible data. `legacy-task-thread-reference.ts` is test-only: it may unwrap
old envelopes and freeze old wire inputs, but it cannot be exported from production `index.ts` or
issue network requests.

- [x] **Step 6: Make the boundary test pass and commit**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts
```

Expected: PASS.

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: define canonical workbench client boundary"
```

### Task 2A: Add The Server-Reviewed Canonical Edit Capability

**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_projection.go`
- Modify: `backend/api/handler/coze/workbench_canonical_projection_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service_test.go`

- [x] **Step 1: Write failing public-projection tests**

Assert canonical Thread JSON contains `coze.can_edit=true` only when the authenticated viewer is
the current Thread owner. Missing or mismatched viewer facts must produce `false`; `creator_id`,
`owner_id` and other identity fields remain absent. Cover create/get/search projection paths so the
capability cannot be present on only one handler.

- [x] **Step 2: Implement the additive capability**

Derive `can_edit` from the server authentication context and current Thread summary after existing
authorization. Do not accept it from request metadata, query, body or headers. Do not expose the
creator ID and do not change application/domain/repository contracts. Existing unauthorized paths
continue to fail closed before returning a Thread.

- [x] **Step 3: Verify and commit**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./api/handler/coze \
  -run '^(TestCanonicalThread.*CanEdit|TestCreateCanonicalThread|TestGetCanonicalThread|TestSearchCanonicalThreads)$' \
  -count=1
```

Use the exact discovered test names when existing handlers use more specific names. Expected:
PASS, with `coze.can_edit` present and identity fields still redacted.

```bash
git add \
  backend/api/handler/coze/workbench_canonical_projection.go \
  backend/api/handler/coze/workbench_canonical_projection_test.go \
  backend/api/handler/coze/workbench_canonical_thread_service_test.go
git commit -m "feat: expose canonical thread edit capability"
```

### Task 3: Implement Canonical Fetch, Errors And Core Operations

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-fetch.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/adapters/canonical-thread-adapter.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/index.ts`

- [ ] **Step 1: Write failing request snapshots**

Freeze method, path, query, body and headers for:

| App action | Canonical request |
| --- | --- |
| Search | `POST /api/workbench/threads/search`, `limit`, integral `offset` |
| Create without files | `POST /api/workbench/threads`, `coze.initial_run` |
| Create before file upload | `POST /api/workbench/threads`, `coze.deferred_initial_run` |
| Get | `GET /api/workbench/threads/:thread_id` |
| Messages | `GET .../messages?limit&before_seq|after_seq` |
| Runs | `GET .../runs?parent_run_id&status&limit&offset` |
| Follow-up/new Run | `POST .../runs`, atomic User Message plus Run |
| Get Run | `GET .../runs/:run_id` |
| Cancel | `POST .../runs/:run_id/cancel` |
| Resume | `POST .../runs/:run_id/resume` |
| Events | `GET .../runs/:run_id/events` |

Every request sets `X-Coze-Space-ID`, `x-requested-with: XMLHttpRequest` and
`credentials: same-origin`. JSON writes set `content-type: application/json`. Idempotent writes
put the original key in `Idempotency-Key`, never in two places.

- [ ] **Step 2: Implement the single fetch boundary**

`fetchCanonicalJSON` handles direct success bodies, `204`, pagination headers and canonical
`detail/code/retryable/trace_id` errors. Define `WorkbenchClientError` with HTTP status, stable code,
trace ID, retryable flag and outcome `rejected | failed | unknown`.

Network failure before an HTTP response is `unknown`. No catch block may call a source contract.
Reject malformed JSON, unknown response shape, unsafe integer conversion and non-finite time.

- [ ] **Step 3: Implement strict core adapters**

Adapters must:

- parse RFC 3339 through finite `Date.parse` checks;
- preserve all IDs as decimal strings;
- stringify reviewed metadata/payload objects only where current page helpers expect strings;
- map Thread title/source/progress/last-message metadata to the current view model;
- map server-reviewed `coze.can_edit` and never reconstruct `creator_id`;
- map only the public Run fields: parent/source Run, attempt/run kind, stream modes,
  disconnect/durability, optional start/end and terminal reason metadata;
- map `coze.initial_submission` and `coze.submission_message` to current create results;
- reject malformed resources instead of returning partial objects.

- [ ] **Step 4: Implement atomic create semantics**

No-attachment create sends one User Message in `coze.initial_run`. Attachment create sends
`coze.deferred_initial_run`; after canonical upload succeeds, `createRun` sends exactly one User
Message plus uploaded `file_id` references. Follow-up uses the same `createRun` atomic boundary.

Parse current JSON-string config/context/metadata locally before a write. Invalid JSON performs no
HTTP request. Preserve `multitask_strategy`, `on_disconnect`, `durability`, stream modes and the
original idempotency key.

- [ ] **Step 5: Prove there is no fallback path**

For 400, 401, 403, 404, 409, 413, 422, 429, 500, network reset and abort, assert exactly one
canonical request and no request whose URL starts with `/api/workbench/task_threads`,
`/api/threads` or `/api/runs`.

- [ ] **Step 6: Run tests and commit**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts
```

Expected: PASS.

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: implement canonical workbench core client"
```

### Task 3A: Close Canonical Turn-Metadata And Top-Level Retry Gaps

This task is a hard prerequisite for completing Task 3 and starting Task 4. The current UI has two
reviewed Run-creation branches that the canonical server cannot yet represent: a normal turn stores
`message_metadata` on the atomically-created User Message, while a failed top-level retry creates a
new Run without appending a duplicate User Message. Neither branch may be silently weakened during
the client cutover.

**Files:**
- Modify: `idl/workbench/thread.thrift`
- Modify generated IDL outputs under `backend/api/model/workbench/thread_contract` and
  `frontend/packages/arch/api-schema/src/idl/workbench`
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_stream.go`
- Modify: `backend/api/handler/coze/workbench_canonical_projection.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_stream_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_projection_test.go`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/workbench-thread-client.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/fixtures.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts`
- Modify: `docs/superpowers/specs/2026-07-26-workbench-thread-api-contract-design.md`
- Modify: `docs/superpowers/specs/2026-07-26-agent-execution-kernel-v2-production-spec.md`

- [x] **Step 1: Write failing server and app-owned contract tests**

Freeze the canonical Run request extension as:

```json
{
  "coze": {
    "message_metadata": {"source": "workbench_detail_followup"}
  }
}
```

for an ordinary turn, or:

```json
{
  "coze": {
    "attempt_kind": "retry",
    "source_run_id": "3001"
  }
}
```

for a failed top-level retry. The two forms are mutually exclusive. `message_metadata` must be a
bounded JSON object and is stored only on the new User Message. A retry requires one normalized User
Message in `input` as Run input, but it creates no Message row and returns no
`coze.submission_message`.

Add tests proving:

- an ordinary turn persists the reviewed message metadata in the same atomic Message + Run bundle;
- a retry binds a failed top-level source Run in the same path Thread, preserves input/config/
  metadata/options/idempotency, returns `coze.attempt_kind=retry` and `coze.source_run_id`, and does
  not increase the Message count;
- retry rejects a missing, cross-Thread, child, non-failed or malformed source Run;
- retry rejects `message_metadata`, a turn rejects retry-only fields, and client-owned protected Run
  metadata remains rejected;
- idempotent replay returns the same retry Run without looking for or creating a User Message;
- the app-owned request has explicit `attempt_kind?: 'turn' | 'retry'` and `source_run_id?: string`;
- the IDL and both generated outputs expose optional `coze` on create/stream/wait Run requests.

- [x] **Step 2: Implement the minimal application contract**

Add an explicit top-level retry source field to `ApplicationService.CreateRun`; zero keeps every
existing caller unchanged. When set, the application layer authorizes and loads the source Run,
requires the same Thread, a top-level task Run and failed terminal status, then writes a
server-owned `attempt_kind=retry` marker and `source_run_id` relation into Run metadata. Canonical
projection reads those protected fields into `coze` and removes them from public metadata. It must
call the existing message-less CreateRun path, not `RetrySubagentRun`, and must not create a Message.

Ordinary turns continue through the existing atomic `CreateRunBundle`; pass the reviewed
`MessageMetadata` to its Message spec unchanged after JSON validation. Do not add a second
persistence path or modify legacy TaskThread request semantics.

- [x] **Step 3: Implement the canonical `coze` request extension**

Parse `coze.message_metadata` for turns and `coze.attempt_kind/source_run_id` for retries. The
server, not caller metadata, owns retry identity. Include message metadata and retry source/kind in
the idempotency fingerprint. For retry, preserve the normalized single User Message as Run input,
set Application `MessageContent` empty, allow `CreateRunResponse.Message=nil`, and omit
`coze.message_id/submission_message`.

Set safe request logs to `submission_kind=run_retry` and the decimal source Run ID. Never log input,
message metadata, content, config or caller metadata. Apply the same submission validation to
create, stream and wait; all source routes and current UI remain unchanged.

- [x] **Step 4: Regenerate and verify the public contract**

Regenerate backend and frontend IDL outputs with the repository-pinned `hz`/`thriftgo` and
`idl2ts` toolchains:

```bash
cd backend
hz update -idl ../idl/api.thrift -enable_extends

cd ../frontend/packages/arch/api-schema
rushx update
```

Use the repository's existing handler exclusion arguments when invoking `hz update`, then run
`backend/scripts/verify_api_codegen.sh` to prove deterministic output and handwritten-handler
preservation. Verify that only the intended optional `coze` field and generated accessors change;
do not hand-edit generated files.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./application/agentthread ./api/handler/coze \
  -run 'Canonical.*(MessageMetadata|TopLevelRetry)|ApplicationCreateRun.*(MessageMetadata|TopLevelRetry)' \
  -count=1
```

Expected: PASS with a persisted turn Message and a message-less retry Run.

```bash
cd frontend/apps/coze-studio
rushx test src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts
```

Expected: PASS with the explicit app-owned retry fields and no transport-owned DTOs.

- [x] **Step 5: Commit the prerequisite contract**

```bash
git add \
  idl/workbench/thread.thrift \
  backend/api/model/workbench/thread_contract \
  backend/application/agentthread \
  backend/api/handler/coze \
  frontend/packages/arch/api-schema/src/idl/workbench \
  frontend/apps/coze-studio/src/pages/workbench/thread-client \
  docs/superpowers/specs/2026-07-26-workbench-thread-api-contract-design.md \
  docs/superpowers/specs/2026-07-26-agent-execution-kernel-v2-production-spec.md
git commit -m "feat: preserve canonical run submission semantics"
```

After this prerequisite passes its own spec and quality reviews, return to the Task 3 implementer.
The client must send the new extension, allow message-less retry creation results, and then resolve
all remaining Task 3 review findings before Task 3 can be marked complete.

### Task 4: Implement All Canonical Product Operations

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/adapters/canonical-thread-adapter.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-thread-product-client.test.ts`

- [ ] **Step 1: Write failing snapshots for all 26 product routes**

Cover suggestions, compatibility append, three Upload operations, eight Artifact/scan operations,
token usage, eight Memory operations, and Memory/Guardrail/MCP audit operations. Assert all resource
requests include the workspace header and never place `space_id`, owner or user ID in body/query.

- [ ] **Step 2: Implement Upload and Artifact transport**

Use `FormData` for upload and omit JSON content type. Delete Upload by canonical `file_id`, not old
filename. Return Artifact content as `Blob` plus content headers. Parse direct signed URL/scan
responses, but never pass the URL or filename to telemetry. Pure deletes accept only the documented
empty success response.

- [ ] **Step 3: Implement usage, Memory and audit transport**

Map page/page_size to non-negative integral limit/offset. Preserve abort behavior for usage.
Convert Memory metadata string to a JSON object on write and back to the current page string on
read. Never synthesize `raw_usage` from internal provider data. Preserve only reviewed public audit
fields and epoch-millisecond times.

- [ ] **Step 4: Enforce append and suggestion semantics**

Suggestions send model/count inputs and let the server load persisted messages. Compatibility
append sends `append_mode='internal_compat'`; reject user/human roles locally with
`atomic_run_submission_required`. Ordinary Workbench create and follow-up code must not call this
method.

- [ ] **Step 5: Run tests and commit**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/canonical-thread-product-client.test.ts
```

Expected: PASS for every path/header/body and paired visible-model fixture.

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: implement canonical workbench product client"
```

### Task 5: Add Run-Bound Canonical SSE And Safe Client Telemetry

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/run-event-cursor.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/client-telemetry.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/workbench-run-stream.test.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/client-telemetry.test.ts`
- Modify: `frontend/apps/coze-studio/package.json`
- Modify: `common/config/subspaces/default/pnpm-lock.yaml`

- [ ] **Step 1: Add the existing workspace SSE dependency**

Add `"@coze-arch/fetch-stream": "workspace:*"` to the app and run:

```bash
cd frontend
rush update
```

Expected: only the app importer/workspace edge changes in
`common/config/subspaces/default/pnpm-lock.yaml`; no new external package version is introduced.

- [ ] **Step 2: Write failing stream lifecycle tests**

Cover `GET /api/workbench/threads/:thread_id/runs/:run_id/stream`, workspace header,
`after_event_id`, `cancel_on_disconnect=false`, `stream_mode=events`, ignored metadata frames,
normalized events, cursor advancement after successful parse, reconnect, terminal end, server error,
abort and explicit close. Assert one subscription owns exactly one fetch stream and never creates
`EventSource` or a source-contract request.

- [ ] **Step 3: Implement per-Run cursor storage**

Key by contract, space, Thread and Run. Store only a positive decimal event ID in session storage,
with an in-memory fallback when storage is unavailable. Clear the key after terminal end; never
store response content or a mode flag.

- [ ] **Step 4: Implement fetch-based SSE**

Use `@coze-arch/fetch-stream` so the browser can send `X-Coze-Space-ID`. Parse event type, ID and
data; advance the cursor only after a valid public Run event. Resolve `closed` once. Close/abort one
source only. An error reports canonical failure and cannot open another URL.

- [ ] **Step 5: Add redaction-tested operation telemetry**

Emit `workbench_thread_client_operation` with only contract, operation, Thread/Run/resource IDs,
duration, outcome, stable error code and trace ID. For unexpected errors record error class/name,
not message. Tests must feed message content, Memory content, attachment filename, signed URL,
credential-looking text and tool result through operations and prove none appears in serialized
logger metadata.

- [ ] **Step 6: Run tests and commit**

```bash
cd frontend/apps/coze-studio
rushx test -- \
  src/pages/workbench/thread-client/__tests__/workbench-run-stream.test.ts \
  src/pages/workbench/thread-client/__tests__/client-telemetry.test.ts
```

Expected: PASS with one source per Run and redacted logs.

```bash
git add \
  frontend/apps/coze-studio/package.json \
  common/config/subspaces/default/pnpm-lock.yaml \
  frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: add canonical run stream client"
```

### Task 6: Delegate Existing Page Services To The Canonical Client

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/legacy-page-response.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-usage-service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-usage-service.test.ts`

- [ ] **Step 1: Write failing page-facing parity tests**

Inject a recording canonical client and call every existing Workbench/Tasks service export. Assert
callers still receive current `{code: 0, msg: 'success', data}` structures, suggestion's current
shape, Artifact Blob/header result and abort naming. Assert Runtime Doctor, Skill install and model/
Knowledge/Database/Workflow services still use their existing non-Thread owners.

- [ ] **Step 2: Implement named response presenters**

`legacy-page-response.ts` is page-shape compatibility only and cannot issue HTTP. Add named
presenters for Thread list/create/get, Message, Run/Event, Upload, Artifact/scan, usage, Memory and
audit families. Do not assemble anonymous envelopes throughout components.

- [ ] **Step 3: Replace service transport ownership**

Replace generated `workbenchTask` Thread calls and manual TaskThread fetches with the singleton
`CanonicalThreadClient`. Keep current export names to avoid page workflow changes. Every call must
receive current route/store `space_id`; presenter output must keep existing optional-field behavior.

`task-memory-service.ts` and `task-usage-service.ts` become thin canonical delegates. Preserve
`AbortError` behavior and current user-facing safe error messages.

- [ ] **Step 4: Prove Task Center remains separate**

Do not modify `frontend/apps/coze-studio/src/pages/task-center/service.ts` transport ownership.
Run its tests to prove Scheduled Task remains on the generated `workbenchTask` namespace.

- [ ] **Step 5: Run tests and commit**

```bash
cd frontend/apps/coze-studio
rushx test -- \
  src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts \
  src/pages/tasks/__tests__/tasks-service.test.ts \
  src/pages/tasks/__tests__/task-usage-service.test.ts \
  src/pages/task-center/__tests__/task-center-page.test.tsx \
  src/pages/task-center/__tests__/task-center-utils.test.ts
```

Expected: PASS; page exports are structurally unchanged and Task Center remains functional.

```bash
git add \
  frontend/apps/coze-studio/src/pages/workbench/service.ts \
  frontend/apps/coze-studio/src/pages/tasks/service.ts \
  frontend/apps/coze-studio/src/pages/tasks/task-memory-service.ts \
  frontend/apps/coze-studio/src/pages/tasks/task-usage-service.ts \
  frontend/apps/coze-studio/src/pages/tasks/__tests__ \
  frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "refactor: delegate workbench pages to canonical client"
```

### Task 7: Remove Thread Transport Types From The Component Tree

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/canonical-frontend-contract.test.ts`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifact-actions.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifact-list-item.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifact-message-list.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-helpers.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-deleted-artifacts-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-journal-events.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-subagents.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-token-usage.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-display-title.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-execution-todo-dock.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-export-action.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-guardrail-audit-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-human-interrupt-card.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-mcp-runtime-audit-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-import-export-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-list-actions.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-message-token-usage.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-thread-events.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-title-sync.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-usage-loader.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifact-scan-jobs-section.tsx`

- [ ] **Step 1: Make the production boundary scan fail**

Extend `canonical-frontend-contract.test.ts` to reject Thread-related `workbenchTask`/
`workbenchThread` imports, source route strings, direct `fetch` and direct `EventSource` under
Workbench, Tasks and workspace task list production files. The only transport allowlist is
`thread-client/canonical-fetch.ts`, `canonical-thread-client.ts` and the canonical adapter. Explicitly
allow Task Center's Scheduled Task use and unrelated generated APIs.

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/tasks/__tests__/canonical-frontend-contract.test.ts
```

Expected: FAIL and identify current transport-bound production files.

- [ ] **Step 2: Replace transport types only**

Import app-owned Thread, Run, Message, Event, Artifact, Todo, TokenUsage, Memory and audit types from
`pages/workbench/thread-client`. Preserve JSX, labels, sort order, state ownership and user action
ordering.

- [ ] **Step 3: Thread workspace scope through calls**

Supply `space_id`, `thread_id` and `run_id` from the authenticated route/store scope. Do not infer
workspace from response metadata. Add focused assertions for Memory, audit, scan, usage,
suggestions, upload and follow-up calls.

- [ ] **Step 4: Run boundary and component tests**

```bash
cd frontend/apps/coze-studio
rushx test -- \
  src/pages/tasks/__tests__/canonical-frontend-contract.test.ts \
  src/pages/workbench/__tests__/workbench.test.tsx \
  src/pages/tasks/__tests__/tasks.test.tsx \
  src/pages/tasks/__tests__/task-detail-loader.test.ts \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/tasks/__tests__/task-memory-section.test.tsx
```

Expected: PASS and no production page component imports Thread transport DTOs.

- [ ] **Step 5: Commit the component boundary**

```bash
git add \
  frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx \
  frontend/apps/coze-studio/src/pages/tasks \
  frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "refactor: isolate workbench page models"
```

### Task 8: Bind The Detail Stream To Stable Run Identity

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-run-event-stream.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail-data-scope.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-run-actions-hook.test.tsx`

- [ ] **Step 1: Write failing one-source lifecycle tests**

Cover initial active Run, terminal close, Thread change, Run change after resume, top-level retry,
subagent retry result isolation, unmount, late source-Run event and duplicate terminal event. Assert
there is never more than one open subscription and late events cannot update a newer attempt.

- [ ] **Step 2: Remove direct EventSource ownership**

`useTaskThreadRunEventStream` receives space, Thread and committed Run IDs and calls
`canonicalThreadClient.subscribeRunEvents`. It keeps current title, token and event mapping callbacks
but does not construct a URL or inspect a client mode.

- [ ] **Step 3: Commit stream changes only after Run state changes**

Resume/retry first accepts the returned Run, updates scoped detail state, then hook dependencies
close the previous source and open the new Run source. Guard callbacks with captured
`{spaceId, threadId, runId, generation}` before mutation.

- [ ] **Step 4: Run tests and commit**

```bash
cd frontend/apps/coze-studio
rushx test -- \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/tasks/__tests__/task-detail-data-scope.test.tsx \
  src/pages/tasks/__tests__/task-run-actions-hook.test.tsx \
  src/pages/workbench/thread-client/__tests__/workbench-run-stream.test.ts
```

Expected: PASS with one active source and one terminal transition.

```bash
git add \
  frontend/apps/coze-studio/src/pages/tasks/task-run-event-stream.ts \
  frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts \
  frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts \
  frontend/apps/coze-studio/src/pages/tasks/__tests__
git commit -m "refactor: bind task stream to canonical run"
```

### Task 9: Pass Gate A With Source Contracts Still Present

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/client-equivalence.test.ts`
- Modify: `docs/superpowers/evidence/2026-07-28-workbench-canonical-cutover-gate-a.md`
- Modify: `docs/superpowers/evidence/2026-07-28-workbench-source-contract-usage-audit.md`

- [ ] **Step 1: Compare paired visible outcomes**

For every resource family, run the test-only V1 reference fixture and canonical adapter fixture into
the same presenter. Compare page-visible fields, IDs, ordering, pagination, terminal statuses,
errors, abort behavior and Stream events. Transport-only differences are allowed; visible behavior
differences fail.

- [ ] **Step 2: Prove the canonical branch has one request source**

Mock browser transport at the page level for create without files, deferred create/upload/run,
follow-up, cancel, resume, retry, Artifact, Memory and SSE. Assert all requests begin with
`/api/workbench/threads` and each user action has the expected write count.

- [ ] **Step 3: Run Gate A automation**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client src/pages/workbench src/pages/tasks
rushx lint
rushx build
```

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" \
  ./api/handler/coze ./api/router/coze ./application/agentthread ./application/workbench \
  -count=1
```

Expected: PASS while source routes still exist.

- [ ] **Step 4: Run canonical page regression against the same online workspace**

Start the feature-branch backend with `COZE_WORKBENCH_CANONICAL_API_ENABLED=true` and the frontend
on unused local ports. This is the final use of the migration gate before Task 11 removes it. Use the
same account/workspace as the Task 1 control. Validate list/detail, no-file create, file create,
stream, refresh, follow-up, cancel/resume/retry where constructible, Artifact, usage, Memory and
audit views. Record network and console evidence.

Expected in the feature branch:

- no `/api/workbench/task_threads/**`, local `/api/threads/**` or `/api/runs/**` request;
- no duplicate write;
- one SSE bound to one Run;
- current visible behavior matches the control;
- no new console error and no sensitive client log value.

- [ ] **Step 5: Apply the Gate A stop rule**

Gate A is PASS only if Steps 1-4 pass and the `/api/runs/**` audit document has explicit status for
all five items. A blocked external-log item does not block UI migration, but it blocks Task 14.
Any product parity failure blocks Tasks 10-14.

- [ ] **Step 6: Commit Gate A evidence**

```bash
git add \
  frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/client-equivalence.test.ts \
  docs/superpowers/evidence/2026-07-28-workbench-canonical-cutover-gate-a.md \
  docs/superpowers/evidence/2026-07-28-workbench-source-contract-usage-audit.md
git commit -m "test: verify canonical workbench cutover parity"
```

### Task 10: Migrate The Internal NewX Parity Client

**Files:**
- Modify: `backend/internal/deerflowparity/newx_client.go`
- Modify: `backend/internal/deerflowparity/client_test.go`
- Preserve: `backend/internal/deerflowparity/deerflow_client.go`
- Preserve: `backend/application/skill/builtin_deerflow/**`

- [ ] **Step 1: Write failing canonical NewX snapshots**

For NewX only, require `/api/workbench/threads/**`, `X-Coze-Space-ID`, direct canonical response
shapes, `events` SSE mode and no fallback. Cover create, start, get, cancel, stream/reconnect,
state/history, Run messages/events and human-interaction resume.

Keep existing DeerFlow tests asserting external `/api/threads/**` and CSRF behavior. This proves the
reference client was not rewritten.

- [ ] **Step 2: Give NewX an explicit workspace source**

On successful Thread creation, store the validated `ThreadOptions.SpaceID` in a concurrency-safe
Thread-to-workspace map owned by `NewXClient`. Every subsequent NewX request resolves that map and fails
closed if scope is unknown; it never reads workspace from response metadata or accepts a caller
owner/user ID.

- [ ] **Step 3: Implement NewX-specific canonical helpers**

Do not change the shared DeerFlow route helpers to canonical. Add NewX-specific request/projection
functions for the canonical direct response, pagination headers and SSE frames. Map canonical
status/event shapes into the existing parity `RunHandle`, `MessagePage`, state/history and
`StreamResult` contracts.

Resume must use `POST /api/workbench/threads/:thread_id/runs/:run_id/resume`; pending interaction
lookup must use canonical Run events. No call may use TaskThread `run_events` or old resume.

- [ ] **Step 4: Run tests and commit**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./internal/deerflowparity -count=1
```

Expected: PASS; NewX uses canonical routes and the DeerFlow reference still uses external
`/api/threads/**`.

```bash
git add backend/internal/deerflowparity
git commit -m "refactor: migrate newx parity client to canonical api"
```

### Task 11: Make Canonical Routes The Always-On Primary Contract

**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_contract.go`
- Modify: `backend/api/handler/coze/workbench_canonical_contract_test.go`
- Delete: `backend/api/handler/coze/workbench_canonical_entrypoints.go`
- Modify: `backend/api/handler/coze/workbench_canonical_entrypoints_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_artifact_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_memory_audit_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_message_product_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_stream.go`
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_upload_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_usage_retry_service.go`
- Modify: corresponding `workbench_canonical_*_test.go` files

- [ ] **Step 1: Change gate tests first**

Replace default-off/404 assertions with assertions that canonical handlers enter normal auth,
validation or dependency checks without `COZE_WORKBENCH_CANONICAL_API_ENABLED`. Add a source scan
test that rejects the environment variable and `requireCanonicalAPI` from production backend code.

Run the focused tests and confirm they fail while the gate remains.

- [ ] **Step 2: Remove only the migration gate**

Delete `canonicalAPIEnabledEnv`, `canonicalAPIEnabled`, `requireCanonicalAPI`, request-level guard
branches, test `Setenv` calls and the now-unreferenced `serveCanonicalEntrypoint` production file.
Keep the 47-entry handler inventory test, session principal, workspace authorization, body limits,
strict JSON, public errors, rate limiting, dependency fail-closed checks and structured request logs
unchanged.

- [ ] **Step 3: Review comments and logs**

Update handler comments that still say default-off or migration-only. Every public handler must name
its route, authorization source, application service call and response type. Existing canonical
request logs remain one begin/complete pair per operation; do not add content-bearing debug logs.

- [ ] **Step 4: Run tests and commit**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze \
  -run 'Canonical|WorkbenchCanonical' -count=1
```

Expected: PASS with no feature-gate environment requirement.

```bash
git add backend/api/handler/coze
git commit -m "refactor: make canonical workbench api primary"
```

### Task 12: Retire The TaskThread V1 HTTP Contract

**Files:**
- Modify: `idl/workbench/task.thrift`
- Modify/regenerate: `backend/api/model/workbench/task/task.go`
- Modify/regenerate: `backend/api/model/workbench/chat/workbench.go`
- Modify/regenerate: `backend/api/model/coze/api.go`
- Delete: `backend/api/model/workbench/thread/thread.go`
- Modify/regenerate: `backend/api/router/coze/api.go`
- Modify: `backend/api/router/coze/custom_routes.go`
- Modify: `backend/api/router/coze/workbench_canonical_thread_route_test.go`
- Delete: `backend/api/router/coze/workbench_thread_route_test.go` after moving negative assertions
- Create: `backend/api/handler/coze/workbench_thread_access.go`
- Create: `backend/api/handler/coze/workbench_thread_access_test.go`
- Create: `backend/api/handler/coze/langgraph_http_contract.go`
- Modify: `backend/api/handler/coze/langgraph_thread_service.go`
- Modify: `backend/api/handler/coze/langgraph_run_service.go`
- Delete: `backend/api/handler/coze/workbench_thread_service.go`
- Delete: `backend/api/handler/coze/workbench_thread_service_test.go`
- Modify: `backend/scripts/verify_api_codegen.sh`
- Modify/regenerate: `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- Modify: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`
- Delete: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/legacy-task-thread-reference.ts`
- Remove V1-only fixtures from: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/fixtures.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/client-equivalence.test.ts`

- [ ] **Step 1: Make route-absence and Scheduled Task preservation tests fail**

Change the router contract to require all 36 TaskThread method/path pairs absent while all 47
canonical routes and all Scheduled Task routes remain exactly registered. Extend API schema tests to
require Scheduled Task exports and reject TaskThread DTO/method exports.

Run focused tests and confirm failure before deletion.

- [ ] **Step 2: Remove only TaskThread IDL ownership**

From `idl/workbench/task.thrift`, delete TaskThread DTOs from the current TaskThread section and
delete the TaskThread service methods after `ListScheduledTaskExecutions`. Preserve Scheduled Task
enums, DTOs, methods, namespace and route annotations byte-for-byte except generator formatting.

- [ ] **Step 3: Regenerate backend and frontend contracts**

Run the pinned backend generator:

```bash
cd backend
hz --version
thriftgo --version
hz update -idl ../idl/api.thrift -enable_extends \
  --exclude_file api/handler/coze/agent_run_service.go \
  --exclude_file api/handler/coze/announcement_service.go \
  --exclude_file api/handler/coze/app_dev_service.go \
  --exclude_file api/handler/coze/bot_open_api_service.go \
  --exclude_file api/handler/coze/config_service.go \
  --exclude_file api/handler/coze/conversation_service.go \
  --exclude_file api/handler/coze/database_service.go \
  --exclude_file api/handler/coze/developer_api_service.go \
  --exclude_file api/handler/coze/intelligence_service.go \
  --exclude_file api/handler/coze/knowledge_service.go \
  --exclude_file api/handler/coze/memory_service.go \
  --exclude_file api/handler/coze/message_service.go \
  --exclude_file api/handler/coze/open_apiauth_service.go \
  --exclude_file api/handler/coze/passport_service.go \
  --exclude_file api/handler/coze/playground_service.go \
  --exclude_file api/handler/coze/plugin_develop_service.go \
  --exclude_file api/handler/coze/public_product_service.go \
  --exclude_file api/handler/coze/resource_service.go \
  --exclude_file api/handler/coze/upload_service.go \
  --exclude_file api/handler/coze/workbench_skill_service.go \
  --exclude_file api/handler/coze/workflow_service.go
```

Expected versions: `hz version v0.9.7`, `thriftgo 0.4.5`. Restore repository-required generated
middleware postprocessing and license headers exactly as documented by
`backend/scripts/verify_api_codegen.sh`, then run `gofmt`.

```bash
cd frontend/packages/arch/api-schema
rushx update
```

- [ ] **Step 4: Remove custom routes and extract live shared helpers**

Delete only the `/workbench/task_threads/:thread_id` custom group from
`registerWorkbenchCustomRoutes`; keep IM and MCP routes.

Before deleting `workbench_thread_service.go`, move these still-live helpers with focused tests:

- viewer ID and authenticated Thread access context, including authorization, to
  `workbench_thread_access.go`;
- terminal Run status predicate to the same shared file or existing run lifecycle owner;
- Artifact content-disposition builder to `workbench_canonical_artifact_service.go`.

The still-present LangGraph adapter must stop borrowing V1 helpers before the file is deleted. Move
its error response into `langgraph_http_contract.go`, add a LangGraph-owned public Run-event
projection there, and update both LangGraph handlers to call those names. This compatibility file is
deleted in Task 14 when stateless routes pass their audit; if stateless routes remain, it remains
owned only by that explicit adapter.

Delete the handwritten V1-only `backend/api/model/workbench/thread/thread.go` after its caller scan
is empty, and remove that path from `handwritten_generated_excludes` in
`backend/scripts/verify_api_codegen.sh`. Do not move any other V1 projection or envelope helper.

- [ ] **Step 5: Delete V1 handler and frontend reference artifacts**

Delete the old handler/test after graph/source caller checks show only V1 routes/tests. Remove the
test-only V1 reference and paired V1 fixtures after Gate A evidence is committed. Production
canonical tests become the regression source.

- [ ] **Step 6: Verify generated and route contracts**

```bash
cd backend
bash scripts/verify_api_codegen.sh
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze \
  -run 'Workbench|Canonical|ScheduledTask' -count=1
```

```bash
cd frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
```

Expected: 36 V1 routes absent, 47 canonical routes present, Scheduled Task routes/types present,
codegen deterministic.

- [ ] **Step 7: Commit the V1 contract retirement**

```bash
git add -A \
  idl/workbench/task.thrift \
  backend/api/model \
  backend/api/router/coze \
  backend/api/handler/coze \
  backend/scripts/verify_api_codegen.sh \
  frontend/packages/arch/api-schema \
  frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "refactor: retire task thread v1 api"
```

### Task 13: Retire The Local LangGraph Thread Contract

**Files:**
- Modify: `backend/api/router/coze/custom_routes.go`
- Modify: `backend/api/router/coze/workbench_canonical_thread_route_test.go`
- Delete: `backend/api/handler/coze/langgraph_thread_service.go`
- Delete: `backend/api/handler/coze/langgraph_thread_service_test.go`
- Modify: `backend/api/handler/coze/langgraph_run_service.go`
- Modify: `backend/api/handler/coze/langgraph_run_service_test.go`
- Preserve until Task 14: `backend/api/model/agent/langgraph/run.go`
- Preserve until Task 14: `backend/api/model/agent/langgraph/thread.go`

- [ ] **Step 1: Make all 23 negative route assertions fail**

Replace preservation assertions with exact method/path absence checks for `/api/threads/**`. Keep
the 47 canonical positive snapshot and the external DeerFlow client path tests.

- [ ] **Step 2: Remove local route registration**

Delete the `/api/threads` registration block from `registerLangGraphCustomRoutes` and rename the
remaining function to `registerLangGraphStatelessRunRoutes` so its ownership is explicit until the
zero-use decision is applied.

- [ ] **Step 3: Delete Thread-only adapters**

Delete `langgraph_thread_service.go/test`. Remove Thread-bound Run handlers and tests from
`langgraph_run_service.go/test`, retaining only the ten stateless routes and their required helper
code. Keep both handwritten LangGraph model files until Task 14 resolves the stateless owner.

Do not delete application, domain, repository or checkpoint operations merely because an HTTP
projection disappeared.

- [ ] **Step 4: Prove internal consumers are migrated**

```bash
rg -n '/api/threads(?:/|\")|/api/workbench/task_threads' \
  backend/internal/deerflowparity/newx_client.go frontend/apps/coze-studio/src
```

Expected: no output. Then run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze ./internal/deerflowparity \
  -count=1
```

Expected: PASS; all 23 local routes absent. External DeerFlow reference tests remain green.

- [ ] **Step 5: Commit the Thread compatibility retirement**

```bash
git add -A backend/api/router/coze backend/api/handler/coze backend/internal/deerflowparity
git commit -m "refactor: retire local langgraph thread api"
```

### Task 14: Apply The Stateless Run Zero-Use Gate And Retire It

**Files:**
- Modify: `docs/superpowers/evidence/2026-07-28-workbench-source-contract-usage-audit.md`
- Modify: `backend/api/router/coze/custom_routes.go`
- Modify: `backend/api/router/coze/workbench_canonical_thread_route_test.go`
- Delete when audit PASS: `backend/api/handler/coze/langgraph_run_service.go`
- Delete when audit PASS: `backend/api/handler/coze/langgraph_run_service_test.go`
- Delete when audit PASS: `backend/api/handler/coze/langgraph_http_contract.go`
- Delete when audit PASS and no caller remains: `backend/api/model/agent/langgraph/run.go`
- Delete when audit PASS and no caller remains: `backend/api/model/agent/langgraph/thread.go`

- [ ] **Step 1: Enforce the five-item decision**

Read the signed audit result. Continue only when all five items are `PASS`, including zero valid
business traffic and no registered owner/consumer. Record reviewer and timestamp before editing
routes.

If any item is `BLOCKED` or has a caller, stop this task. Keep `/api/runs/**`, retain only its
required implementation, and add the owner/migration decision to the final residual-risk report.
Do not label it unused.

- [ ] **Step 2: Make all 10 negative route assertions fail**

Add exact absence checks for POST create/stream/wait and GET/POST resource, messages, feedback,
cancel, stream and join combinations. Run the router test to prove routes still exist before edit.

- [ ] **Step 3: Delete stateless route and adapter ownership**

Delete registrations, handlers, bindings, tests and helpers used only by stateless Run. Once both
local LangGraph families are gone, delete the empty registration function and both handwritten
`backend/api/model/agent/langgraph` files. Re-run caller scans before each file deletion.

- [ ] **Step 4: Run route and package tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze \
  -count=1
```

Expected when audit PASS: all 10 routes absent, canonical routes pass, and no local LangGraph API
adapter package/file remains.

- [ ] **Step 5: Commit the stateless retirement**

```bash
git add -A \
  backend/api/router/coze \
  backend/api/handler/coze \
  backend/api/model/agent/langgraph \
  docs/superpowers/evidence/2026-07-28-workbench-source-contract-usage-audit.md
git commit -m "refactor: retire unused stateless run api"
```

Skip this commit when the audit is not PASS.

### Task 15: Run The Canonical-Only Automated Verification

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/canonical-frontend-contract.test.ts`
- Modify: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`
- Modify: `backend/api/router/coze/workbench_canonical_thread_route_test.go`

- [ ] **Step 1: Add final source-contract assertions**

Require:

- one production `CanonicalThreadClient` and no selector/V1 client;
- no source route string in Workbench/Tasks or NewX production code;
- no `COZE_WORKBENCH_CANONICAL_API_ENABLED`;
- no TaskThread DTO/method in generated schema, while Scheduled Task remains;
- no local `/api/threads/**` registration;
- no local `/api/runs/**` registration when Task 14 passed;
- ChatTask routes and symbols remain retired;
- external DeerFlow route strings remain only in their explicit external owner.

- [ ] **Step 2: Run frontend verification from fresh output**

```bash
cd frontend/packages/arch/api-schema
rushx update
rushx test
```

```bash
cd frontend/apps/coze-studio
rushx test
rushx lint
rushx build
```

Expected: PASS with no mode environment variable and one canonical bundle path.

- [ ] **Step 3: Run backend codegen and focused verification**

```bash
cd backend
bash scripts/verify_api_codegen.sh
gofmt -l \
  api/handler/coze/workbench_canonical_*.go \
  api/handler/coze/workbench_thread_access*.go \
  api/router/coze/*.go \
  internal/deerflowparity/*.go
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" \
  ./api/handler/coze ./api/router/coze ./internal/deerflowparity \
  ./application/agentthread ./application/workbench \
  -count=1
go vet ./api/handler/coze ./api/router/coze ./internal/deerflowparity
```

Expected: codegen deterministic, `gofmt -l` has no output, tests/vet PASS.

- [ ] **Step 4: Run the repository-wide backend safety pass**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-full-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./... -count=1
```

```bash
cd ..
APP_ENV=debug make build_server
```

Expected: PASS. If an unrelated pre-existing failure appears, record exact package/test and prove it
also fails on the unchanged baseline before classifying it as external.

- [ ] **Step 5: Run deterministic source scans**

```bash
rg -n '/api/workbench/task_threads|COZE_WORKBENCH_CANONICAL_API_ENABLED|WORKBENCH_THREAD_CLIENT_MODE|TaskThreadV1Client' \
  backend frontend idl scripts \
  --glob '!**/*test*'
```

Expected: no production output.

```bash
rg -n '/api/threads(?:/|\")|/api/runs(?:/|\")' backend/api frontend/apps/coze-studio/src
```

Expected: no output when Task 14 passed; otherwise only the explicitly retained stateless Run
owner is allowed and listed in the audit.

- [ ] **Step 6: Commit final contract tests**

```bash
git add \
  frontend/apps/coze-studio/src/pages/tasks/__tests__/canonical-frontend-contract.test.ts \
  frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts \
  backend/api/router/coze/workbench_canonical_thread_route_test.go
git commit -m "test: enforce canonical-only workbench contract"
```

### Task 16: Run Gate B Browser Regression And Update Current Project Truth

**Files:**
- Create: `docs/superpowers/evidence/2026-07-28-workbench-canonical-cutover-gate-b.md`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-chat.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/runbooks/workbench-canonical-product-client-validation.md`
- Modify: `scripts/workbench-execution-graph.mjs`
- Modify: `scripts/workbench-execution-graph.test.mjs`

- [ ] **Step 1: Start the final feature branch against the configured online database**

Use ignored local environment files/symlinks; do not print or commit secrets. Start backend and
frontend on unused ports and record process IDs/URLs in local task notes. Verify backend health and
authenticated workspace access before page tests.

- [ ] **Step 2: Execute the canonical-only page matrix**

Using the in-app browser, verify:

- login/session and workspace isolation;
- task list, pagination, detail, title and status;
- no-file, single-file and multi-file first submission;
- upload failure/retry/delete where constructible;
- streaming, refresh persistence, Thread switch, reconnect and terminal completion;
- follow-up, suggestions, cancel, resume, top-level retry and subagent retry;
- Artifact preview/download/delete/restore/scan/retry;
- token usage, Memory import/export/edit/delete/restore/clear and audits;
- unauthorized, missing resource, rate limit, dependency unavailable and unknown-result behaviors
  through browser or named automated substitute.

Record which scenarios were page-observed and which used deterministic tests. Capture console and
network summaries without cookies, bodies, signed URLs or content.

- [ ] **Step 3: Probe final route registration**

Through the authenticated local backend, verify all 47 canonical method/path templates are
registered. Probe representative methods from all 36 TaskThread and 23 LangGraph Thread templates
and assert 404/route absence. When Task 14 passed, probe all 10 stateless templates likewise.

Do not treat a handler validation error as route absence; the router snapshot is the authoritative
all-method proof.

- [ ] **Step 4: Verify request and log ownership**

The browser network log must contain only `/api/workbench/threads/**` for Thread workflows. Server
logs must show canonical operation names, authenticated IDs and outcomes without content-bearing
values. Confirm no duplicate write and no second stream.

- [ ] **Step 5: Rewrite current context and runbook**

Update current facts to state canonical is primary/always-on, UI is canonical-only, source contracts
are retired, Scheduled Task is preserved and stateless Run status follows the audit result. Remove
instructions to set the canonical feature gate or preserve source route snapshots.

Update execution-chain and machine graph nodes/queries to point at the canonical client and routes;
remove deleted source paths. Keep historical specs/plans as historical records rather than rewriting
their original decisions.

- [ ] **Step 6: Verify the execution graph**

```bash
node scripts/workbench-execution-graph.mjs verify
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
node --test scripts/workbench-execution-graph.test.mjs
```

Expected: PASS with no deleted source path required by the current machine contract.

- [ ] **Step 7: Perform final scope and diff audit**

```bash
git diff --check dev...HEAD
git diff --stat dev...HEAD
git diff --name-status dev...HEAD
```

Expected scope: frontend canonical client/page delegation, canonical gate removal, NewX migration,
old HTTP contract deletion, generated code/tests and current documentation. No DB migration,
application/domain/repository/Eino behavior rewrite, Task Center regression or unrelated refactor.

- [ ] **Step 8: Commit Gate B evidence and current facts**

```bash
git add \
  docs/superpowers/evidence/2026-07-28-workbench-canonical-cutover-gate-b.md \
  docs/superpowers/context/project-context.md \
  docs/superpowers/context/workbench-chat.md \
  docs/superpowers/context/workbench-execution-chain.md \
  docs/superpowers/context/workbench-execution-graph.json \
  docs/superpowers/runbooks/workbench-canonical-product-client-validation.md \
  scripts/workbench-execution-graph.mjs \
  scripts/workbench-execution-graph.test.mjs
git commit -m "docs: record canonical workbench cutover"
```

### Task 17: Run The Required `dev` Integration Gates

**Files:**
- Read and follow: `docs/superpowers/runbooks/dev-integration-audit.md`
- No implementation file changes unless the audit finds a real defect

- [ ] **Step 1: Refresh and audit the requirement branch**

Fetch `origin/dev`, verify the branch is based on the latest required `dev`, rebase or merge only
according to the runbook, then rerun the affected verification matrix on the resulting SHA. Record
branch SHA, base SHA, file scope, test outputs, Gate A/Gate B evidence and residual risk.

- [ ] **Step 2: Request the first explicit confirmation**

Present the first audit report. Do not merge local `dev` until the user confirms that exact branch,
SHA, scope and verification result.

- [ ] **Step 3: Merge local `dev` and run the second audit**

After confirmation, merge without force operations. From merged local `dev`, rerun the stricter
verification and confirm `origin/dev` did not change during the audit.

- [ ] **Step 4: Request the second explicit confirmation**

Present the post-merge audit report. Do not push until the user separately confirms that exact local
`dev` SHA and verification result.

- [ ] **Step 5: Push without force and verify remote SHA**

After the second confirmation, push `dev`, verify `origin/dev` matches local `dev`, and report the
final SHA. If remote `dev` moved at any point, restart the first integration audit.

---

## Gate A Acceptance

- Canonical client and page delegation pass paired model/request tests.
- Same online workspace control and canonical page runs have equivalent visible behavior.
- Canonical page network has no source-contract request, fallback, duplicate write or second SSE.
- Source routes remain physically present during comparison.
- `/api/runs/**` audit has explicit evidence or an explicit external-evidence block.

## Gate B Acceptance

- Final production frontend contains one canonical client and no runtime selector/V1 adapter.
- 47 canonical routes remain registered and always-on behind normal auth/authorization.
- 36 TaskThread V1 and 23 local LangGraph Thread routes are absent.
- Ten stateless Run routes are absent only when the five-item audit passed; otherwise the retained
  owner and risk are explicit.
- Scheduled Task routes, generated types and Task Center tests remain intact.
- NewX parity uses canonical local routes; external DeerFlow reference keeps its own route dialect.
- TaskThread application/domain/repository/MySQL/Eino ADK behavior is unchanged.
- Frontend tests/lint/build, backend codegen/tests/vet/build, browser regression, route probes,
  execution graph and diff audit pass from fresh output.
- ChatTask routes remain absent and no source fallback is reintroduced.
- Local `dev` merge and remote push each require their own confirmed audit report.
