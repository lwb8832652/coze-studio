# Workbench Canonical UI Client Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在生产仍默认使用 TaskThread V1、旧接口 wire contract 完全不变的前提下，为 Workbench/Tasks UI 建立 app-owned `WorkbenchThreadClient`，实现 V1 与 canonical 两个无 fallback adapter，并完成 canonical 全页面本地回归。

**Architecture:** React 组件只依赖 app-owned request/view model 和既有 page service 导出，不再依赖 transport DTO。`TaskThreadV1Client` 保留当前请求路径、参数、envelope、EventSource 和错误语义；`CanonicalThreadClient` 使用同一应用请求模型，将空间写入 `X-Coze-Space-ID`、将写入映射到 canonical 原子边界，并通过带 header 的 fetch SSE 连接具体 Run。构建期 selector 只选择一个 client；本期 release build 固定拒绝 canonical，任何请求失败都不得切换 client 或双写。

**Tech Stack:** React 18、TypeScript、Rsbuild、Vitest、`@coze-studio/api-schema`、`@coze-arch/fetch-stream`、`@coze-arch/logger`、Hertz canonical API

---

## Dependency And Safety Boundary

- 本计划只能在 `2026-07-27-workbench-canonical-product-extensions.md` 的 Checkpoint A 全部通过后执行。
- `StreamCanonicalRun` 与 `ReconnectCanonicalRunStream` 必须已经不再返回 `501`。
- 生产和常规本地启动省略配置时使用 `v1`；`canonical` 只允许显式本地 dev/test。
- 不修改 Workbench/Tasks 页面布局、文案、状态机、store、路由或用户操作顺序。
- 页面内部请求统一携带 `space_id`，用于 canonical header。V1 adapter 必须按当前每个来源方法的原参数映射，不能因为 app request 新增 `space_id` 而改变旧 wire body/query。
- 一次动作只调用一个 client；无 read fallback、write fallback、shadow read、shadow write 或双 SSE。
- canonical 错误保留结果不明确状态和 trace reference，不因失败调用 V1。
- Thread/Run/File/Artifact/Memory/Event ID 在 app model 中保持 `string`；RFC 3339 transport 时间在 adapter 中转换成当前 UI 使用的 epoch milliseconds。
- Transport DTO 只能出现在 `thread-client/task-thread-v1-client.ts`、`thread-client/canonical-thread-client.ts` 及各自 adapter 内。
- 日志只允许 client、operation、ID、duration、outcome、trace reference；不记录正文、Memory、signed URL、附件名、provider body 或 tool payload。

## File Structure

| File | Responsibility |
| --- | --- |
| `pages/workbench/thread-client/types.ts` | App-owned requests, resources, pages, commands, errors and SSE events |
| `pages/workbench/thread-client/workbench-thread-client.ts` | Complete client interface and operation observer wrapper |
| `pages/workbench/thread-client/task-thread-v1-client.ts` | Only TaskThread generated client/manual V1 fetch owner |
| `pages/workbench/thread-client/canonical-thread-client.ts` | Only canonical JSON/multipart/binary/SSE transport owner |
| `pages/workbench/thread-client/client-selector.ts` | Build-time singleton selection; default V1 |
| `pages/workbench/thread-client/canonical-fetch.ts` | Same-origin fetch, headers, direct response and canonical error parsing |
| `pages/workbench/thread-client/client-telemetry.ts` | Safe structured client operation logs |
| `pages/workbench/thread-client/run-event-cursor.ts` | Per-Run session cursor store |
| `pages/workbench/thread-client/adapters/task-thread-v1-adapter.ts` | V1 envelope/JSON string to app model |
| `pages/workbench/thread-client/adapters/canonical-thread-adapter.ts` | Canonical direct DTO/RFC time to app model |
| `pages/workbench/thread-client/index.ts` | App-owned public exports only |
| `pages/workbench/service.ts` | Existing Workbench exports delegate to selected client; unrelated model/resource APIs unchanged |
| `pages/tasks/service.ts` | Existing Tasks exports delegate to selected client; Runtime Doctor/Skill APIs unchanged |
| `pages/tasks/task-run-event-stream.ts` | One selected-client subscription bound to one active Run |
| `pages/tasks/task-detail-hooks.ts` | Supplies space/run scope and switches stream only after Run identity changes |
| `frontend/apps/coze-studio/rsbuild.config.ts` | Validate and define `WORKBENCH_THREAD_CLIENT_MODE` |
| `frontend/apps/coze-studio/src/global.d.ts` | Build constant type |

### Task 1: Freeze The App-Owned Client Boundary

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/types.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/workbench-thread-client.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/index.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/fixtures.ts`

- [ ] **Step 1: Define normalized resources without transport imports**

Use snake_case field names and epoch-millisecond times so existing rendering logic remains structural. The core model declarations are:

```ts
export interface WorkbenchThread {
  thread_id: string;
  space_id: string;
  creator_id: string;
  title: string;
  status: string;
  source: string;
  progress: number;
  last_user_message: string;
  last_agent_message: string;
  created_at: number;
  updated_at: number;
  values?: { todos?: WorkbenchTodo[] };
}

export interface WorkbenchMessage {
  message_id: string;
  thread_id: string;
  run_id: string;
  role: string;
  content: string;
  metadata: string;
  created_at: number;
}

export interface WorkbenchRun {
  run_id: string;
  thread_id: string;
  parent_run_id: string;
  space_id: string;
  creator_id: string;
  assistant_id: string;
  run_kind: string;
  status: string;
  command: string;
  input: string;
  config: string;
  context: string;
  metadata: string;
  stream_mode: string;
  multitask_strategy: string;
  on_disconnect: string;
  durability: string;
  idempotency_key: string;
  worker_id: string;
  error_code: string;
  error_message: string;
  started_at: number;
  ended_at: number;
  created_at: number;
  updated_at: number;
}

export interface WorkbenchRunEvent {
  event_id: string;
  thread_id: string;
  run_id: string;
  event_type: string;
  payload: string;
  created_at: number;
}
```

Define the current UI fields for Upload, Artifact, ArtifactScanJob, TokenUsage, TokenUsageAggregate, Memory, MemoryAuditEvent, GuardrailAuditEvent, MCPRuntimeAuditEvent and HumanInteractionResponse. Do not define `raw_usage`, provider body, tool arguments/results, signed URL metadata or credential fields. Keep `worker_id` in the app model for V1 compatibility; canonical maps the safe `worker_ref` into it.

- [ ] **Step 2: Define one workspace-scoped request convention**

Every client request carries `space_id` at the app boundary, even when V1 did not previously send it:

```ts
export interface WorkbenchScopedRequest {
  space_id: string;
}

export interface WorkbenchThreadRequest extends WorkbenchScopedRequest {
  thread_id: string;
}

export interface WorkbenchRunRequest extends WorkbenchThreadRequest {
  run_id: string;
}

export interface WorkbenchPageRequest {
  page?: number;
  page_size?: number;
}

export interface WorkbenchPage<T> {
  items: T[];
  total: number;
  has_more: boolean;
  next_cursor?: string;
}
```

Define method-specific inputs using the fields current components already submit. Keep `space_id` app-only; each transport decides header/body/query placement.

- [ ] **Step 3: Define the complete interface**

```ts
export interface WorkbenchThreadClient {
  readonly contract: 'task_threads_v1' | 'canonical_v1';
  searchThreads(input: SearchThreadsInput): Promise<WorkbenchPage<WorkbenchThread>>;
  createThread(input: CreateThreadInput): Promise<CreateThreadResult>;
  getThread(input: WorkbenchThreadRequest): Promise<WorkbenchThread>;
  listMessages(input: ListMessagesInput): Promise<WorkbenchPage<WorkbenchMessage>>;
  appendMessageForCompatibility(input: AppendMessageInput): Promise<WorkbenchMessage>;
  generateSuggestions(input: GenerateSuggestionsInput): Promise<string[]>;
  listRuns(input: ListRunsInput): Promise<WorkbenchPage<WorkbenchRun>>;
  createRun(input: CreateRunInput): Promise<CreateRunResult>;
  cancelRun(input: WorkbenchRunRequest): Promise<WorkbenchRun>;
  resumeRun(input: ResumeRunInput): Promise<WorkbenchRun>;
  retrySubagentRun(input: RetrySubagentRunInput): Promise<WorkbenchRun>;
  listRunEvents(input: ListRunEventsInput): Promise<WorkbenchPage<WorkbenchRunEvent>>;
  subscribeRunEvents(input: SubscribeRunEventsInput): RunEventSubscription;
  listUploads(input: WorkbenchThreadRequest): Promise<WorkbenchPage<WorkbenchUpload>>;
  uploadFiles(input: UploadFilesInput): Promise<UploadFilesResult>;
  deleteUpload(input: DeleteUploadInput): Promise<void>;
  listArtifacts(input: ListArtifactsInput): Promise<WorkbenchPage<WorkbenchArtifact>>;
  getArtifactContent(input: GetArtifactContentInput): Promise<ArtifactContentResult>;
  getArtifactSignedURL(input: GetArtifactSignedURLInput): Promise<ArtifactSignedURLResult>;
  deleteArtifact(input: ArtifactActionInput): Promise<void>;
  restoreArtifact(input: ArtifactActionInput): Promise<RestoreArtifactResult>;
  reviewArtifactScan(input: ReviewArtifactScanInput): Promise<ArtifactScanReviewResult>;
  listArtifactScanJobs(input: ListArtifactScanJobsInput): Promise<WorkbenchPage<WorkbenchArtifactScanJob>>;
  retryArtifactScanJob(input: RetryArtifactScanJobInput): Promise<RetryArtifactScanJobResult>;
  getTokenUsage(input: GetTokenUsageInput, options?: RequestOptions): Promise<TokenUsageResult>;
  listMemories(input: ListMemoriesInput): Promise<WorkbenchPage<WorkbenchMemory>>;
  updateMemory(input: UpdateMemoryInput): Promise<WorkbenchMemory>;
  deleteMemory(input: MemoryActionInput): Promise<void>;
  clearMemories(input: ClearMemoriesInput): Promise<{ deleted: number }>;
  restoreMemory(input: MemoryActionInput): Promise<WorkbenchMemory>;
  importMemories(input: ImportMemoriesInput): Promise<ImportMemoriesResult>;
  exportMemories(input: ExportMemoriesInput): Promise<ExportMemoriesResult>;
  listMemoryAuditEvents(input: ListMemoryAuditEventsInput): Promise<WorkbenchPage<WorkbenchMemoryAuditEvent>>;
  listGuardrailAuditEvents(input: ListGuardrailAuditEventsInput): Promise<WorkbenchPage<WorkbenchGuardrailAuditEvent>>;
  exportGuardrailAuditEvents(input: ExportGuardrailAuditEventsInput): Promise<ExportGuardrailAuditEventsResult>;
  listMCPRuntimeAuditEvents(input: ListMCPRuntimeAuditEventsInput): Promise<WorkbenchPage<WorkbenchMCPRuntimeAuditEvent>>;
}
```

`RunEventSubscription` contains `{ close(): void; closed: Promise<void> }`. `SubscribeRunEventsInput` requires `space_id`, `thread_id`, `run_id`, optional cursor, callbacks for event/done/error, and an `AbortSignal`.

- [ ] **Step 4: Add frozen fixtures and boundary scans**

Create one V1 fixture and one canonical fixture for each resource family. They represent the same Thread/Run IDs and visible data but use their respective transport shapes. The contract test must assert `types.ts`, `workbench-thread-client.ts` and `index.ts` contain no `@coze-studio/api-schema`, `workbenchTask` or `workbenchThread` import.

- [ ] **Step 5: Run and verify the interface test**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts
```

Expected: PASS for pure type/runtime fixture checks; no transport implementation is required yet.

- [ ] **Step 6: Commit the client boundary**

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: define workbench thread client boundary"
```

### Task 2: Implement The Behavior-Preserving V1 Client

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/task-thread-v1-client.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/adapters/task-thread-v1-adapter.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/task-thread-v1-client.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/index.ts`

- [ ] **Step 1: Write failing V1 request snapshots**

Mock generated `workbenchTask` functions and `fetch`. Freeze the current path/body/query for create/list/get/messages/runs/resume/cancel/retry/memory/audit. Freeze the six manual Artifact/scan routes and upload multipart route. For app requests that now include `space_id`, assert the V1 wire request is exactly the previous request:

```ts
expect(mockListMessages).toHaveBeenCalledWith({
  thread_id: 'thread-1',
  page: 2,
  page_size: 20,
});
expect(mockListMessages).not.toHaveBeenCalledWith(
  expect.objectContaining({ space_id: expect.anything() }),
);
```

For source methods that already required or accepted `space_id` (`ListTaskThreads`, `CreateTaskThread`, Artifact actions), preserve their existing mapping.

- [ ] **Step 2: Write failing normalized fixture tests**

Assert V1 envelopes unwrap into app-owned resources, JSON strings remain strings, IDs remain strings, epoch times remain numbers, `raw_usage` is discarded, and `code !== 0` becomes a typed `WorkbenchClientError` without invoking any other client.

- [ ] **Step 3: Implement V1 JSON methods**

`TaskThreadV1Client` is the only production module allowed to import `workbenchTask`. Each method calls one current generated function and passes the result to `task-thread-v1-adapter.ts`. Do not wrap a call in a canonical fallback. Keep generated `.withAbort()` behavior for token usage and preserve `AbortError` names.

- [ ] **Step 4: Move current manual V1 fetches unchanged**

Move upload, Artifact content/signed URL/delete/restore, scan list/retry/review and V1 stream URL logic from `pages/workbench/service.ts`, `pages/tasks/service.ts` and `task-usage-service.ts` into this client. Preserve:

- `x-requested-with: XMLHttpRequest`;
- existing Chinese error messages;
- current query parameter names/order;
- source `{code,msg,data}` validation;
- current `EventSource` route `/api/workbench/task_threads/:thread_id/run_events/stream` and optional `run_id/after_event_id` query.

- [ ] **Step 5: Run V1 client tests**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/task-thread-v1-client.test.ts
```

Expected: PASS; every old wire snapshot is unchanged.

- [ ] **Step 6: Commit V1 implementation**

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: add task thread v1 adapter"
```

### Task 3: Implement Canonical Fetch, Errors And Core Mapping

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-fetch.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/adapters/canonical-thread-adapter.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/index.ts`

- [ ] **Step 1: Write failing canonical request tests**

Freeze these mappings:

| App action | Canonical request |
| --- | --- |
| search | `POST /api/workbench/threads/search`, `limit=page_size`, `offset=(page-1)*page_size`, no body `space_id` |
| create without files | `POST /api/workbench/threads`, `coze.initial_run`, one User Message |
| deferred create | `POST /api/workbench/threads`, `coze.deferred_initial_run` |
| get | `GET /api/workbench/threads/:thread_id` |
| list messages | `GET .../messages?limit&before_seq|after_seq` |
| list runs | `GET .../runs?parent_run_id&status&limit&offset` |
| create/follow-up Run | `POST .../runs`, one User Message plus uploaded `file_id` references |
| cancel | `POST .../runs/:run_id/cancel` |
| resume | `POST .../runs/:run_id/resume`, canonical interrupt response |
| list events | `GET .../runs/:run_id/events` |

Every request must set `X-Coze-Space-ID`, `x-requested-with`, `credentials: same-origin`; JSON requests set `content-type`. Idempotent writes put the app key in `Idempotency-Key`, not in body.

- [ ] **Step 2: Implement one canonical fetch boundary**

```ts
export class WorkbenchClientError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly traceId: string,
    readonly retryable: boolean,
    readonly outcome: 'rejected' | 'failed' | 'unknown',
  ) {
    super(message);
    this.name = 'WorkbenchClientError';
  }
}

export const canonicalHeaders = (spaceId: string, json = true) => ({
  ...(json ? { 'content-type': 'application/json' } : {}),
  'x-coze-space-id': spaceId,
  'x-requested-with': 'XMLHttpRequest',
});
```

`fetchCanonicalJSON` parses direct success JSON and canonical `code/detail/trace_id/retryable` errors. Network failure before an HTTP response yields `outcome='unknown'`; no caller catches it to invoke V1. `204` returns `undefined` without parsing JSON.

- [ ] **Step 3: Implement canonical core adapter functions**

`canonical-thread-adapter.ts` must:

- parse RFC 3339 with a strict `Date.parse` finite check;
- stringify safe metadata/payload objects for current UI helpers;
- inject app request `space_id` into the app model without trusting a response body space;
- map `metadata.title` and `coze.source/progress/last_*` into `WorkbenchThread`;
- map `coze.parent_run_id/run_kind/started_at/ended_at/terminal_reason` into `WorkbenchRun`;
- map `coze.initial_submission` into the current create result;
- map `coze.submission_message` into the current create-run result;
- preserve event IDs/cursors and safe event payloads;
- reject malformed IDs/times instead of returning partial resources.

- [ ] **Step 4: Implement create workflows exactly once**

For no-attachment create, send:

```ts
{
  metadata: { title, source: 'web' },
  coze: {
    initial_run: {
      assistant_id: 'agent',
      input: { messages: [{ role: 'user', content: message }] },
      config,
      context,
      metadata,
    },
  },
}
```

For `defer_start`, use `deferred_initial_run` with the same validated message/config. A later `createRun` sends:

```ts
{
  assistant_id: 'agent',
  input: {
    messages: [{ role: 'user', content: messageContent }],
    uploaded_files: uploads.map(file => ({ file_id: file.file_id })),
  },
  config,
  context,
  metadata,
  multitask_strategy: input.multitask_strategy || 'reject',
  on_disconnect: 'continue',
  durability: input.durability || 'async',
  stream_mode: ['events'],
}
```

Parse current JSON-string config/context/metadata before transport. Invalid JSON fails locally before a write and does not invoke V1.

- [ ] **Step 5: Run canonical core client tests**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts
```

Expected: PASS for path/header/body/idempotency/error and fixture mapping; no V1 mock call occurs on any canonical error.

- [ ] **Step 6: Commit canonical core client**

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: add canonical workbench core client"
```

### Task 4: Implement Canonical Product Methods

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/adapters/canonical-thread-adapter.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/canonical-thread-product-client.test.ts`

- [ ] **Step 1: Write failing route/header/body snapshots**

Cover all 26 product methods. Assert upload delete uses `file_id`, multipart carries the workspace header but no JSON content type, Artifact content returns `Blob`, signed URL is returned but never included in telemetry, Memory metadata converts between app JSON string and canonical object, and pure deletes accept only `204`.

- [ ] **Step 2: Implement Upload and Artifact methods**

Use `FormData` for `/uploads`; list and delete use canonical product routes. Use reviewed `fetch` for Artifact binary content and direct JSON for signed URL/scan actions. Preserve `AbortSignal` where the interface accepts it. Do not put `space_id` in query/body.

- [ ] **Step 3: Implement usage, Memory and Audit methods**

Map page/page_size to integral limit/offset. Token usage adapter supplies empty `raw_usage` and metadata only if current app types still require those keys; it never receives raw values from canonical. Memory write metadata must parse as a JSON object. Audit adapters preserve visible safe fields and convert canonical times.

- [ ] **Step 4: Implement suggestion and compatibility append**

Suggestions send only model/count inputs because canonical loads persisted messages. `appendMessageForCompatibility` sends `append_mode='internal_compat'`; a user/human role is rejected locally with `atomic_run_submission_required`. No ordinary Workbench or follow-up workflow may call this method.

- [ ] **Step 5: Run product client tests**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/canonical-thread-product-client.test.ts
```

Expected: PASS for all product route snapshots and normalized fixture equivalence.

- [ ] **Step 6: Commit product methods**

```bash
git add frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: add canonical workbench product client"
```

### Task 5: Add Run-Bound Canonical SSE Without A Second Source

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/run-event-cursor.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/task-thread-v1-client.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/workbench-run-stream.test.ts`
- Modify: `frontend/apps/coze-studio/package.json`
- Modify: `common/config/rush/pnpm-lock.yaml`

- [ ] **Step 1: Add the existing workspace stream package**

Add:

```json
"@coze-arch/fetch-stream": "workspace:*"
```

to the app dependencies and run:

```bash
cd frontend
rush update
```

Expected: only the app dependency and Rush lockfile workspace edge change; no new external version is introduced.

- [ ] **Step 2: Write failing stream lifecycle tests**

Using fake `EventSource` for V1 and fake `fetchStream` for canonical, cover:

- V1 keeps its current thread stream URL and event names;
- canonical requests `GET /api/workbench/threads/:thread_id/runs/:run_id/stream`;
- canonical sends space header, `after_event_id`, `cancel_on_disconnect=false`, `stream_mode=events`;
- `metadata` does not enter the task timeline;
- `events` becomes one normalized `WorkbenchRunEvent`;
- unknown modes are ignored safely;
- cursor advances only after a parsed event;
- reconnect uses the stored cursor;
- `end` resolves `closed` once;
- `error` reports once and never starts V1;
- close/abort terminates exactly one source.

- [ ] **Step 3: Implement a per-Run session cursor store**

Use a key containing contract, space, Thread and Run IDs. Store only a positive decimal event ID in `sessionStorage`, with an in-memory fallback when storage is unavailable. Clear the key after terminal `end`; do not store response bodies or feature mode.

- [ ] **Step 4: Implement canonical fetch SSE**

Use `@coze-arch/fetch-stream` because browser `EventSource` cannot attach `X-Coze-Space-ID`. Pass `credentials`, headers and abort signal. Parse `ParseEvent.type/event/id/data`; persist `id` only after successfully adapting an `events` payload. Do not create a timer-based V1 fallback.

- [ ] **Step 5: Keep the V1 source unchanged behind the interface**

The V1 implementation continues to instantiate one `EventSource` with the existing URL and listeners for `run.event` and `done`. It adapts the same payload into `WorkbenchRunEvent`; no canonical header or route is added to V1.

- [ ] **Step 6: Run stream tests**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/workbench-run-stream.test.ts
```

Expected: PASS with one source per subscription and no fallback call.

- [ ] **Step 7: Commit stream support**

```bash
git add frontend/apps/coze-studio/package.json common/config/rush/pnpm-lock.yaml frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: add canonical workbench run stream client"
```

### Task 6: Add Safe Client Telemetry And A Release-Safe Selector

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/client-telemetry.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/client-selector.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/client-selector.test.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/client-telemetry.test.ts`
- Modify: `frontend/apps/coze-studio/rsbuild.config.ts`
- Modify: `frontend/apps/coze-studio/src/global.d.ts`
- Modify: `frontend/apps/coze-studio/package.json`

- [ ] **Step 1: Write failing selector tests**

Assert `undefined` selects V1, `v1` selects V1, `canonical` selects canonical in a non-release test, invalid values throw, and a release build with canonical throws before bundling. Assert the returned singleton does not change during a page session.

- [ ] **Step 2: Define and validate the build value**

At Rsbuild config evaluation:

```ts
const threadClientMode = process.env.WORKBENCH_THREAD_CLIENT_MODE ?? 'v1';
const releaseBuild =
  process.env.WORKBENCH_THREAD_CLIENT_RELEASE_BUILD === 'true';

if (threadClientMode !== 'v1' && threadClientMode !== 'canonical') {
  throw new Error('WORKBENCH_THREAD_CLIENT_MODE must be v1 or canonical');
}
if (releaseBuild && threadClientMode === 'canonical') {
  throw new Error('canonical Workbench client is not enabled for release builds');
}
```

Add `WORKBENCH_THREAD_CLIENT_MODE: JSON.stringify(threadClientMode)` to `source.define`, declare the global as `'v1' | 'canonical'`, and change the app build script to set `WORKBENCH_THREAD_CLIENT_RELEASE_BUILD=true`. The dev script does not set the release marker.

- [ ] **Step 3: Implement safe operation telemetry**

Wrap each public method once with `@coze-arch/logger`. Emit `eventName='workbench_thread_client_operation'` and only:

```ts
{
  client_contract,
  operation,
  thread_id,
  run_id,
  resource_id,
  duration_ms,
  outcome,
  trace_id,
}
```

Never pass request/response objects or errors as metadata. Convert canonical error to stable code/outcome; for unexpected errors log only error class/name, not message.

- [ ] **Step 4: Prove sensitive values never enter telemetry**

Feed message content, Memory content, attachment filename, signed URL, API key and tool result through mocked operations. Assert none appears in serialized logger calls.

- [ ] **Step 5: Run selector/telemetry and release-gate tests**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/client-selector.test.ts src/pages/workbench/thread-client/__tests__/client-telemetry.test.ts
WORKBENCH_THREAD_CLIENT_MODE=canonical rushx build
```

Expected: unit tests PASS; build exits non-zero with the explicit release-build rejection before compilation.

- [ ] **Step 6: Commit selector and telemetry**

```bash
git add frontend/apps/coze-studio/rsbuild.config.ts frontend/apps/coze-studio/src/global.d.ts frontend/apps/coze-studio/package.json frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "feat: select workbench thread client safely"
```

### Task 7: Delegate Existing Page Services Without Changing UI Results

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-usage-service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/legacy-page-response.ts`
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts`

- [ ] **Step 1: Write failing page service parity tests**

Inject a recording normalized client and call every existing service export. Assert existing callers still receive their current structural result:

```ts
{
  code: 0,
  msg: 'success',
  data: expectedData,
}
```

Keep suggestion's current direct `{ suggestions }` shape and Artifact content's current Blob/header shape. Verify Runtime Doctor and Skill install exports still reference their generated non-Thread clients.

- [ ] **Step 2: Add one compatibility presenter**

`legacy-page-response.ts` converts normalized results into the current page-facing structures only. It is not a transport adapter and cannot issue HTTP. Define named presenters for thread list/create/get, messages, runs/events, uploads, artifacts/scan, usage, Memory and Audit so no page service manually assembles ad hoc envelopes.

- [ ] **Step 3: Delegate Workbench service exports**

Replace direct `workbenchTask.CreateTaskThread`, `AppendTaskThreadMessage`, `CreateTaskThreadRun` and upload fetch with selected-client calls plus presenters. Leave LLM models, Knowledge, Database, Workflow and Runtime Doctor functions unchanged.

- [ ] **Step 4: Delegate Tasks service exports**

Replace direct generated Thread methods and manual Artifact/scan fetches. Preserve all exported function and type names currently imported by components/tests. Replace generated type aliases with aliases to app-owned types. `task-usage-service.ts` delegates abort handling to `client.getTokenUsage` and preserves `AbortError` behavior.

- [ ] **Step 5: Run service and parity tests**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-usage-service.test.ts
```

Expected: PASS; callers see the same success structures and V1 wire snapshots remain unchanged.

- [ ] **Step 6: Commit page service delegation**

```bash
git add frontend/apps/coze-studio/src/pages/workbench/service.ts frontend/apps/coze-studio/src/pages/tasks/service.ts frontend/apps/coze-studio/src/pages/tasks/task-usage-service.ts frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks-service.test.ts frontend/apps/coze-studio/src/pages/workbench/thread-client
git commit -m "refactor: delegate workbench page services"
```

### Task 8: Remove Transport Types From The Component Tree

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/canonical-frontend-contract.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-deleted-artifacts-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-usage-loader.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifact-message-list.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-subagents.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-token-usage.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-message-token-usage.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-thread-events.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifact-actions.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-human-interrupt-card.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifact-list-item.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-title-sync.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-display-title.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-execution-todo-dock.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-export-action.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-journal-events.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-helpers.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-list-actions.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-import-export-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-guardrail-audit-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-mcp-runtime-audit-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifact-scan-jobs-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-deleted-artifacts-section.tsx`

- [ ] **Step 1: Make the transport-boundary scan fail**

Extend `canonical-frontend-contract.test.ts` to reject `workbenchTask` and `workbenchThread` in production Workbench/Tasks files except the two client implementations and their adapters. Keep all current ChatTask retirement patterns.

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/tasks/__tests__/canonical-frontend-contract.test.ts
```

Expected: FAIL and list the current type-only `workbenchTask` imports.

- [ ] **Step 2: Replace type-only transport imports mechanically**

Import `WorkbenchThread`, `WorkbenchRun`, `WorkbenchMessage`, `WorkbenchRunEvent`, `WorkbenchArtifact`, `WorkbenchTodo`, `WorkbenchTokenUsage`, `WorkbenchMemory` and audit types from `../workbench/thread-client` or the correct relative path. Do not change JSX, labels, sorting, state or rendering logic.

- [ ] **Step 3: Thread `space_id` through every client call**

All page service calls must receive the route/prop space explicitly. Update hook/component props where required, including Memory sections, Audit sections, scan jobs, usage, suggestions and stream. V1 adapter strips newly supplied app-only space fields for routes that never received them before; canonical uses them only as `X-Coze-Space-ID`.

Use this invariant in tests:

```ts
expect(clientMethod).toHaveBeenCalledWith(
  expect.objectContaining({
    space_id: 'space-1',
    thread_id: 'thread-1',
  }),
);
```

- [ ] **Step 4: Run the boundary scan and focused component tests**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/tasks/__tests__/canonical-frontend-contract.test.ts src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail-loader.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-memory-section.test.tsx
```

Expected: PASS; no production component imports transport Thread DTOs.

- [ ] **Step 5: Commit component type isolation**

```bash
git add frontend/apps/coze-studio/src/pages/workbench frontend/apps/coze-studio/src/pages/tasks
git commit -m "refactor: isolate workbench transport types"
```

### Task 9: Switch Detail SSE By Stable Run Identity

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-run-event-stream.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail-data-scope.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-run-actions-hook.test.tsx`

- [ ] **Step 1: Write failing one-source lifecycle tests**

Cover initial active Run subscription, terminal close, Thread change, Run change after resume, top-level retry, subagent retry result isolation, unmount, late source-Run event and duplicate terminal event. Assert there is never more than one open subscription and a late event from the previous Run cannot update the new attempt.

- [ ] **Step 2: Replace direct `EventSource` ownership**

`useTaskThreadRunEventStream` receives `spaceId`, `threadId` and `runId`, calls the selected client's `subscribeRunEvents`, and closes it in cleanup. It keeps current event mapping/title/token callbacks. It does not inspect the selected mode or construct URLs.

- [ ] **Step 3: Bind stream changes to committed Run state**

Pass `latestTaskRunID` after detail load. Resume/retry actions first accept the returned new Run, update the scoped detail state, then allow the hook dependency to close the old subscription and open the new one. Guard every callback with captured `{threadId,runId,generation}` before mutating state.

- [ ] **Step 4: Run lifecycle tests**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-detail-data-scope.test.tsx src/pages/tasks/__tests__/task-run-actions-hook.test.tsx
```

Expected: PASS; source replacement and terminal completion occur once.

- [ ] **Step 5: Commit run-bound stream integration**

```bash
git add frontend/apps/coze-studio/src/pages/tasks/task-run-event-stream.ts frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts frontend/apps/coze-studio/src/pages/tasks/__tests__
git commit -m "refactor: bind task stream to active run"
```

### Task 10: Verify Both Client Modes At Unit And Build Level

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/client-equivalence.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] **Step 1: Compare V1 and canonical fixtures through real adapters**

For Thread create/search/get, messages, Runs/events, uploads, Artifacts/scan, usage, Memory, audit, resume/cancel/retry and errors, feed the paired fixtures through both real adapters and assert equal app view models. Explicitly allow canonical safe `worker_ref` to occupy the app `worker_id` display field; do not compare transport-only envelopes or raw timestamps.

- [ ] **Step 2: Run the same page workflow suite with injected clients**

Parameterize Workbench submit, Tasks list and Task detail tests over `TaskThreadV1Client` and `CanonicalThreadClient` transports. Cover no attachment, one/multiple attachments, upload retry/delete, follow-up, suggestions, cancel, resume, top-level retry, subagent retry, Artifact actions, usage, Memory and Audit. Assert final rendered and stored app state, not byte-identical requests.

- [ ] **Step 3: Prove no fallback or dual writes**

For every write family, make the selected transport reject after request dispatch. Assert exactly one transport call, no call to the other client, no second Message/Run/File/Memory mutation, and an error state with the original trace/outcome.

- [ ] **Step 4: Run the complete focused frontend suite**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__ src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__
rushx lint
rushx build
```

Expected: all tests/lint/default-V1 release build PASS.

- [ ] **Step 5: Verify canonical release rejection again**

```bash
cd frontend/apps/coze-studio
WORKBENCH_THREAD_CLIENT_MODE=canonical rushx build
```

Expected: non-zero exit with `canonical Workbench client is not enabled for release builds`; no bundle is emitted.

- [ ] **Step 6: Commit equivalence coverage**

```bash
git add frontend/apps/coze-studio/src/pages/workbench frontend/apps/coze-studio/src/pages/tasks
git commit -m "test: verify workbench client equivalence"
```

### Task 11: Run Real V1 And Canonical Page Regression

**Files:**
- Modify: `docs/superpowers/runbooks/workbench-canonical-product-client-validation.md`
- Modify: `docs/superpowers/context/project-context.md`

- [ ] **Step 1: Start a feature backend on alternate ports with the gate enabled**

Follow `docs/superpowers/runbooks/local-debug-and-test.md`, use the ignored debug environment, and set:

```bash
LISTEN_ADDR=:18888 APP_ENV=debug COZE_WORKBENCH_CANONICAL_API_ENABLED=true ./opencoze -start
```

Reserve backend port `18888` for this matrix and leave the existing `8888` dev process untouched. If `18888` is already occupied, stop and record one replacement port in the validation runbook before starting any process; use that same recorded value for both frontend builds. Confirm logs show `.env.debug`, the canonical gate is enabled, and no production-only runtime fallback is active.

- [ ] **Step 2: Run the V1 dev build first**

```bash
cd frontend/apps/coze-studio
WORKBENCH_THREAD_CLIENT_MODE=v1 WEB_SERVER_PORT=18888 rushx dev --port 18080
```

In the Codex in-app browser, use the runbook account and one fixed workspace. Record URLs and verify:

- no/single/multiple attachment submit;
- upload failure/retry/delete;
- list/detail/title/status;
- follow-up/suggestions/refresh/Thread switch;
- stream/reconnect/terminal/cancel/resume/retries;
- Artifact preview/download/delete/restore/scan;
- usage, Memory import/export/edit/delete/restore/clear and Audit;
- unauthorized/unavailable/rate-limited/unknown-result states;
- no new console errors.

- [ ] **Step 3: Run the canonical dev build against the same backend/data**

```bash
cd frontend/apps/coze-studio
WORKBENCH_THREAD_CLIENT_MODE=canonical WEB_SERVER_PORT=18888 rushx dev --port 18081
```

Repeat the same workflow in the same workspace. Compare final page state and persisted Thread/Run/Message/File/Artifact/Memory records. Do not require byte-identical HTTP payloads. Confirm Network shows only `/api/workbench/threads` for Thread product work and no write to `/api/workbench/task_threads` or `/api/threads`.

- [ ] **Step 4: Test explicit stop conditions**

Stop and fix before proceeding if any cross-space result differs, one action creates duplicate records, Run cancellation/resume/retry changes, late SSE updates the wrong attempt, upload/Artifact policy is weaker, Memory audit is missing, or canonical failure triggers a V1 request.

- [ ] **Step 5: Update the runbook and long-lived context**

Add exact V1/canonical dev commands, ports, tested workspace, visible results, console status and rollback instructions to the runbook. Update `project-context.md` to say both clients exist but release/default remains V1; do not state production canonical rollout has started.

- [ ] **Step 6: Commit browser evidence documentation**

```bash
git add docs/superpowers/runbooks/workbench-canonical-product-client-validation.md docs/superpowers/context/project-context.md
git commit -m "docs: record workbench client parity validation"
```

### Task 12: Final Scope, Contract And Retirement Audit

**Files:**
- Verify only; no implementation file is introduced in this task

- [ ] **Step 1: Re-run backend Checkpoint A**

```bash
cd backend
bash scripts/verify_api_codegen.sh
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository -count=1
```

Expected: PASS from fresh output.

- [ ] **Step 2: Re-run frontend contracts, tests, lint and build**

```bash
cd frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts src/__tests__/workbench-task-contract.test.ts src/__tests__/workbench-task-memory.test.ts
cd ../../../apps/coze-studio
rushx test -- src/pages/workbench/thread-client/__tests__ src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__
rushx lint
rushx build
```

Expected: PASS with default V1 release output.

- [ ] **Step 3: Scan transport ownership and fallback absence**

```bash
rg -n 'workbenchTask|workbenchThread' frontend/apps/coze-studio/src/pages/workbench frontend/apps/coze-studio/src/pages/tasks --glob '!**/__tests__/**'
rg -n 'fallback|shadow|TaskThreadV1Client.*CanonicalThreadClient|CanonicalThreadClient.*TaskThreadV1Client' frontend/apps/coze-studio/src/pages/workbench/thread-client
rg -n 'workbench/(chat|tasks)|ChatTask|sendWorkbenchChat|legacy_task_id|source_task_id' frontend/apps/coze-studio/src backend idl --glob '!**/__tests__/**' --glob '!**/*_test.go'
```

Expected: transport imports appear only in their owning clients/adapters; fallback scan finds only explicit prohibition tests/docs, not runtime switching; ChatTask production symbols remain absent.

- [ ] **Step 4: Confirm source routes remain present and unused by canonical mode**

Use backend route tests for presence, then use canonical browser Network evidence for absence of UI calls. Do not delete `/api/workbench/task_threads`, `/api/threads`, `task.thrift`, generated V1 schema or `TaskThreadV1Client` in this phase.

- [ ] **Step 5: Inspect final diff and dependency scope**

```bash
git diff --check
git status --short
git diff --stat dev...HEAD
git diff --name-only dev...HEAD
```

Expected: no migration, Eino ADK, worker, domain state-machine, page layout or retired ChatTask code; changes are limited to canonical contract/handlers, one upload application method, dual client boundary, service/type/SSE plumbing, tests and docs.

## Checkpoint B Acceptance

Checkpoint B is ready for the repository's first `dev` integration audit only when:

1. default and release builds select V1;
2. canonical is available only by explicit local dev/test mode;
3. both adapters produce equivalent app-owned models and user outcomes;
4. V1 wire snapshots are unchanged despite explicit app-level `space_id` plumbing;
5. canonical writes use only canonical routes and never fallback;
6. one active Run owns one SSE source and one persistent cursor;
7. the full page matrix passes in both modes with no new console error;
8. current source routes remain installed and ChatTask remains retired;
9. no old interface is deleted until a later rollout, observation and traffic-audit phase;
10. the dual `dev` audit and two separate user confirmations still remain mandatory before merge/push.
