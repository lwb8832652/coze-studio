# Workbench Canonical Thread API Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变 `/api/workbench/task_threads`、`/api/threads` 和既有 UI 行为的前提下，新增默认关闭的 `/api/workbench/threads` canonical Thread/Run 核心合同，并用固定版本 JavaScript/Python LangGraph SDK 验证兼容性。

**Architecture:** 新路由只做 HTTP/SDK 合同适配，并且只能调用 `agentthread.ApplicationService`。当前应用合同无法原子保证的 metadata search、title+metadata patch、busy delete 和 public state update，使用新增的 additive application/domain/repository use case 补齐；旧方法、旧调用点和默认查询分支保持不变，不新增数据库表、执行器或双写。手写 handler 负责严格校验、公开投影、SSE 和可观测性；Thrift 只作为路由及生成客户端的合同来源。首批能力由 `COZE_WORKBENCH_CANONICAL_API_ENABLED=false` 默认门控，仅 session principal 可用；生产 API key/Bearer、分布式限流、网关 SSE 和 UI 切换分别在后续批次完成。

**Tech Stack:** Go 1.24、CloudWeGo Hertz、Thrift/Hertz codegen、React/TypeScript api-schema、Vitest、`@langchain/langgraph-sdk==1.6.0`、`langgraph-sdk==0.4.2`、pnpm 8.15.8、uv、GitHub Actions

---

## Scope And Safety Boundary

本计划交付 `canonical_v1` 的核心 Thread/Run 合同，且必须同时满足以下边界：

- 新路由默认关闭；关闭时返回 `404`，不暴露半成品合同。
- 不修改 `/api/workbench/task_threads` 与 `/api/threads` 的 route、method、DTO、默认值、响应、SSE 或日志语义。
- `/api/workbench/tasks*` 与 `/api/workbench/chat` 继续为 `404`，不得恢复 ChatTask handler、IDL、client、application/domain 包或 fallback。
- 所有 canonical handler 只调用 `agentthread.ApplicationService`。允许在该 service、domain service 和 repository 增加 canonical 所需的新方法或 optional request 字段，但当前 handler 不改调用参数，旧方法的零值分支和运行结果必须由回归测试冻结。
- 不新增数据库迁移、持久化表、执行器、worker 分支、数据复制或双写；public state update 复用现有 checkpoint 表并使用独立 `canonical_public_state` runtime type，不能修改 Eino ADK checkpoint bytes 或 runtime key。
- 本批次只支持现有 Web session principal；不得把 x-api-key、外部 Bearer、scope、分布式限流或网关生产配置写成已交付能力。
- `Idempotency-Key` 透传给现有 Run-backed 幂等边界。空 Thread 和 deferred Thread 的通用 exactly-once registry 不在本批次，不得宣称已经具备生产级 exactly-once。
- Workbench UI 不切换 client；原页面、请求参数和业务流程必须保持不变。
- 固定 SDK 版本为 JavaScript `1.6.0`、Python `0.4.2`；Python 仅用于黑盒合同测试，不进入服务运行时。

## File Structure

| 文件 | 职责 |
| --- | --- |
| `idl/workbench/thread.thrift` | canonical route、请求和公开响应的唯一 IDL 输入 |
| `backend/api/handler/coze/workbench_canonical_contract.go` | feature gate、严格 JSON、路径/查询/header 校验、统一错误、trace、分页与安全日志 |
| `backend/api/handler/coze/workbench_canonical_projection.go` | Thread/Run/Message/Event/State 的集中公开投影与状态映射 |
| `backend/api/handler/coze/workbench_canonical_entrypoints.go` | 21 个 codegen route 对应的 exported handler，只做门控和分发 |
| `backend/api/handler/coze/workbench_canonical_thread_service.go` | Thread create/search/get/patch/delete/state/history/messages 的内部实现 |
| `backend/api/handler/coze/workbench_canonical_run_service.go` | Run list/create/get/wait/join/cancel/resume/events/messages |
| `backend/api/handler/coze/workbench_canonical_stream.go` | SSE mode、metadata、回放、实时订阅、cursor、终态和响应 header |
| `backend/api/handler/coze/workbench_canonical_*_test.go` | handler、投影、事务副作用、错误和 SSE 合同测试 |
| `backend/api/router/coze/workbench_canonical_thread_route_test.go` | 新旧 route/method 快照和 ChatTask 防复活 |
| `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts` | 由 IDL 生成的 canonical TypeScript schema |
| `backend/api/handler/coze/testdata/workbench_sdk_compat/` | 固定 JS/Python SDK 黑盒脚本及锁文件 |
| `backend/scripts/verify_workbench_sdk_compat.sh` | 安装锁定依赖并运行真实 SDK 兼容测试 |
| `.github/workflows/ci@backend.yml` | 独立 canonical SDK compatibility job |
| `backend/application/agentthread/canonical_contract.go` | canonical 所需的原子 submission/patch/delete/public-state 与查询用例 |
| `backend/domain/agentthread/service/canonical_contract.go` | canonical domain 校验和 additive service methods |
| `backend/domain/agentthread/repository/canonical_contract.go` | search/patch/delete/query request 与 repository 扩展合同 |
| `backend/domain/agentthread/repository/mysql_canonical.go` | 同一数据库事实上的事务、过滤、排序和 cursor 实现 |

### Task 1: Freeze Source Contracts And Canonical Route Surface

**Files:**
- Create: `backend/api/router/coze/workbench_canonical_thread_route_test.go`
- Modify: none
- Test: `backend/api/router/coze/workbench_canonical_thread_route_test.go`

- [ ] **Step 1: Write the failing canonical route snapshot test**

新增 table-driven test，使用现有 `Register` 测试方式枚举以下 method/path，并断言 canonical 路由必须已注册；由于实现尚未注册这些路由，本步骤应精确产生 RED：

```go
var canonicalThreadRoutes = []routeExpectation{
	{http.MethodPost, "/api/workbench/threads"},
	{http.MethodPost, "/api/workbench/threads/search"},
	{http.MethodGet, "/api/workbench/threads/:thread_id"},
	{http.MethodPatch, "/api/workbench/threads/:thread_id"},
	{http.MethodDelete, "/api/workbench/threads/:thread_id"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/state"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/state"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/history"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/history"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/messages"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/stream"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/wait"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id/stream"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id/join"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/:run_id/cancel"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/:run_id/resume"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id/events"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id/messages"},
}
```

同一测试必须断言 canonical **不存在** `POST .../{run_id}/stream` 与 `POST .../{run_id}/join`。

- [ ] **Step 2: Add immutable source-route and retired-route snapshots**

在同一文件保存当前来源路由的 method/path 集合，并调用测试 helper 比较注册结果；同时发起以下负向探测：

```go
var retiredChatTaskPaths = []string{
	"/api/workbench/tasks",
	"/api/workbench/tasks/1",
	"/api/workbench/chat",
}
```

每个 retired path 必须返回 `404`，且 recording application service 的调用计数保持 `0`。

- [ ] **Step 3: Run the route test and verify RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/router/coze -run 'TestWorkbenchCanonicalThreadRoutes' -count=1
```

Expected: FAIL，仅因 canonical route 未注册；来源 route snapshot 和 ChatTask `404` 断言通过。

- [ ] **Step 4: Commit the failing contract test**

```bash
git add backend/api/router/coze/workbench_canonical_thread_route_test.go
git commit -m "test: freeze canonical workbench thread routes"
```

### Task 2: Add Canonical Thread IDL And Generated Schemas

**Files:**
- Create: `idl/workbench/thread.thrift`
- Modify: `idl/workbench/workbench.thrift`
- Modify: `idl/api.thrift`
- Modify: `frontend/packages/arch/api-schema/api.config.js`
- Modify: `frontend/packages/arch/api-schema/package.json`
- Modify: `frontend/packages/arch/api-schema/src/index.ts`
- Create/Generated: `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts`
- Generated: `backend/api/model/workbench/thread_contract/thread.go`
- Generated: `backend/api/model/workbench/chat/workbench.go`
- Generated: `backend/api/model/coze/api.go`
- Generated: `backend/api/router/coze/api.go`
- Generated: `backend/api/router/coze/middleware.go`
- Create: `backend/api/handler/coze/workbench_canonical_entrypoints.go`
- Test: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`

- [ ] **Step 1: Write a failing TypeScript schema contract test**

测试生成的 API 函数、route/method 元数据和 ID 字符串类型。`idl2ts` 不生成 service interface，
因此测试直接读取生成源文件冻结 21 个 `createAPI` 定义：

```ts
import { readFileSync } from 'node:fs';

import * as api from '../idl/workbench/thread';
import type { CanonicalRun, CanonicalThread } from '../idl/workbench/thread';

const generatedSource = readFileSync(
  new URL('../idl/workbench/thread.ts', import.meta.url),
  'utf8',
);

describe('workbench canonical thread IDL', () => {
  it('keeps public identifiers as strings', () => {
    const thread: Pick<CanonicalThread, 'thread_id'> = { thread_id: '2001' };
    const run: Pick<CanonicalRun, 'thread_id' | 'run_id'> = {
      thread_id: thread.thread_id,
      run_id: '3001',
    };
    expect(run).toEqual({ thread_id: '2001', run_id: '3001' });
  });

  it('exports the complete canonical service', () => {
    const methods = [
      'CreateCanonicalThread', 'SearchCanonicalThreads', 'GetCanonicalThread',
      'PatchCanonicalThread', 'DeleteCanonicalThread', 'GetCanonicalThreadState',
      'UpdateCanonicalThreadState', 'GetCanonicalThreadHistory',
      'PostCanonicalThreadHistory', 'ListCanonicalThreadMessages',
      'ListCanonicalRuns', 'CreateCanonicalRun', 'StreamCanonicalRun',
      'WaitCanonicalRun', 'GetCanonicalRun', 'ReconnectCanonicalRunStream',
      'JoinCanonicalRun', 'CancelCanonicalRun', 'ResumeCanonicalRun',
      'ListCanonicalRunEvents', 'ListCanonicalRunMessages',
    ] as const;
    expect(methods).toHaveLength(21);
    for (const method of methods) {
      expect(api[method]).toBeTypeOf('function');
    }
    expect(generatedSource).toContain('"url": "/api/workbench/threads"');
    expect(generatedSource).toContain('"method": "PATCH"');
    expect(generatedSource).not.toContain('/:run_id/stream",\n  "method": "POST"');
    expect(generatedSource).not.toContain('/:run_id/join",\n  "method": "POST"');
  });
});
```

- [ ] **Step 2: Run the schema test and verify RED**

Run:

```bash
cd frontend
node common/scripts/install-run-rush.js test --to @coze-studio/api-schema -- --run workbench-thread-contract
```

Expected: FAIL because `idl/workbench/thread` does not exist.

- [ ] **Step 3: Define the canonical IDL route source**

创建 `idl/workbench/thread.thrift`，使用独立 namespace 和 service name，避免与手写
`workbench/thread` model 及现有 handler 文件冲突。IDL 中的 body 字段保持顶层 JSON 名称；
动态 JSON 使用 `api.value_type="any"`，不能引入 `payload` 包装：

```thrift
namespace go workbench.thread_contract

include "../base.thrift"

struct CanonicalRouteRequest {
    1: optional i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalRunRouteRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalThread {
    1: required string thread_id
    2: required string created_at
    3: required string updated_at
    4: required string metadata (api.value_type="any")
    5: required string status
    6: required string values (api.value_type="any")
    7: required string interrupts (api.value_type="any")
    8: required string coze (api.value_type="any")
}

struct CanonicalRun {
    1: required string run_id
    2: required string thread_id
    3: required string assistant_id
    4: required string status
    5: required string created_at
    6: required string updated_at
    7: required string metadata (api.value_type="any")
    8: required string multitask_strategy
    9: required string coze (api.value_type="any")
}

struct CanonicalCheckpoint {
    1: required string thread_id
    2: required string checkpoint_ns
    3: required string checkpoint_id
    4: required string checkpoint_map (api.value_type="any")
}

struct CanonicalThreadState {
    1: required string values (api.value_type="any")
    2: required list<string> next
    3: required CanonicalCheckpoint checkpoint
    4: required string metadata (api.value_type="any")
    5: required string created_at
    6: optional CanonicalCheckpoint parent_checkpoint
    7: required string tasks (api.value_type="any")
    8: required string interrupts (api.value_type="any")
}

struct CanonicalThreadUpdateStateResult {
    1: required CanonicalCheckpoint checkpoint
    2: required CanonicalCheckpoint configurable
}

struct CanonicalMessagePage {
    1: required string data (api.value_type="any")
    2: required bool has_more
    3: optional string next_before_seq
    4: optional string next_after_seq
}

struct CanonicalRunEventPage {
    1: required string data (api.value_type="any")
    2: required bool has_more
    3: optional string next_after_event_id
}

struct CreateCanonicalThreadRequest {
    1: optional string thread_id (api.body="thread_id")
    2: optional string metadata (api.body="metadata", api.value_type="any")
    3: optional string if_exists (api.body="if_exists")
    4: optional string ttl (api.body="ttl", api.value_type="any")
    5: optional string supersteps (api.body="supersteps", api.value_type="any")
    6: optional string coze (api.body="coze", api.value_type="any")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalThreadListResponse {
    1: required list<CanonicalThread> body (api.body=".")
}

struct CanonicalThreadStateListResponse {
    1: required list<CanonicalThreadState> body (api.body=".")
}

struct CanonicalRunListResponse {
    1: required list<CanonicalRun> body (api.body=".")
}

struct CanonicalValuesResponse {
    1: required string body (api.body=".", api.value_type="any")
}

struct CanonicalStreamResponse {
    1: required string body (api.body=".")
}

struct CanonicalEmptyResponse {
}

service WorkbenchCanonicalThreadService {
    CanonicalThread CreateCanonicalThread(1: CreateCanonicalThreadRequest req) (api.post="/api/workbench/threads")
    CanonicalThreadListResponse SearchCanonicalThreads(1: SearchCanonicalThreadsRequest req) (api.post="/api/workbench/threads/search")
    CanonicalThread GetCanonicalThread(1: GetCanonicalThreadRequest req) (api.get="/api/workbench/threads/:thread_id")
    CanonicalThread PatchCanonicalThread(1: PatchCanonicalThreadRequest req) (api.patch="/api/workbench/threads/:thread_id")
    CanonicalEmptyResponse DeleteCanonicalThread(1: CanonicalRouteRequest req) (api.delete="/api/workbench/threads/:thread_id")
    CanonicalThreadState GetCanonicalThreadState(1: GetCanonicalThreadStateRequest req) (api.get="/api/workbench/threads/:thread_id/state")
    CanonicalThreadUpdateStateResult UpdateCanonicalThreadState(1: UpdateCanonicalThreadStateRequest req) (api.post="/api/workbench/threads/:thread_id/state")
    CanonicalThreadStateListResponse GetCanonicalThreadHistory(1: GetCanonicalThreadHistoryRequest req) (api.get="/api/workbench/threads/:thread_id/history")
    CanonicalThreadStateListResponse PostCanonicalThreadHistory(1: PostCanonicalThreadHistoryRequest req) (api.post="/api/workbench/threads/:thread_id/history")
    CanonicalMessagePage ListCanonicalThreadMessages(1: ListCanonicalThreadMessagesRequest req) (api.get="/api/workbench/threads/:thread_id/messages")
    CanonicalRunListResponse ListCanonicalRuns(1: ListCanonicalRunsRequest req) (api.get="/api/workbench/threads/:thread_id/runs")
    CanonicalRun CreateCanonicalRun(1: CreateCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs")
    CanonicalStreamResponse StreamCanonicalRun(1: CreateCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs/stream")
    CanonicalValuesResponse WaitCanonicalRun(1: WaitCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs/wait")
    CanonicalRun GetCanonicalRun(1: CanonicalRunRouteRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id")
    CanonicalStreamResponse ReconnectCanonicalRunStream(1: CanonicalRunRouteRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/stream")
    CanonicalValuesResponse JoinCanonicalRun(1: CanonicalRunRouteRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/join")
    CanonicalEmptyResponse CancelCanonicalRun(1: CanonicalRunRouteRequest req) (api.post="/api/workbench/threads/:thread_id/runs/:run_id/cancel")
    CanonicalRun ResumeCanonicalRun(1: ResumeCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs/:run_id/resume")
    CanonicalRunEventPage ListCanonicalRunEvents(1: ListCanonicalRunEventsRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/events")
    CanonicalMessagePage ListCanonicalRunMessages(1: ListCanonicalRunMessagesRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/messages")
}
```

在上述同一 IDL 中完整定义引用到的 request/response 类型。operation-specific request
字段必须与规格逐一一致：Thread search 为 `metadata,status,ids,limit,offset,sort_by,sort_order,values,select,extract`；
patch 为 `metadata,ttl`；state/history 为 `values,as_node,checkpoint,checkpoint_id,subgraphs,limit,before`；
Run create/wait 为 Task 6 列出的全部固定 SDK 字段；list/get/stream/join/cancel/events/messages 的 query
字段分别按第 6-8 节定义。raw values、metadata、state、interrupt、event data 等动态 JSON
字段使用 `api.value_type="any"`；所有公共 ID 在 TypeScript 中必须是 `string`。

request struct 与字段集合固定如下，字段位置必须使用 `api.path`、`api.query`、`api.header`
或 `api.body` 明确标注，不能依赖 method 默认推断：

| Request struct | 字段 |
| --- | --- |
| `SearchCanonicalThreadsRequest` | body: `metadata,status,ids,limit,offset,sort_by,sort_order,values,select,extract` |
| `GetCanonicalThreadRequest` | path: `thread_id`; query: `include` |
| `PatchCanonicalThreadRequest` | path: `thread_id`; header: `Prefer`; body: `metadata,ttl` |
| `GetCanonicalThreadStateRequest` | path: `thread_id`; query: `checkpoint,checkpoint_id,subgraphs` |
| `UpdateCanonicalThreadStateRequest` | path: `thread_id`; body: `values,as_node,checkpoint,checkpoint_id` |
| `GetCanonicalThreadHistoryRequest` | path: `thread_id`; query: `limit,before,checkpoint,checkpoint_id` |
| `PostCanonicalThreadHistoryRequest` | path: `thread_id`; body: `limit,before,checkpoint,checkpoint_id` |
| `ListCanonicalThreadMessagesRequest` | path: `thread_id`; query: `before_seq,after_seq,limit` |
| `ListCanonicalRunsRequest` | path: `thread_id`; query: `status,limit,offset,parent_run_id,select` |
| `CreateCanonicalRunRequest` | path: `thread_id`; body: Task 6 `canonicalCreateRunRequest` 的完整字段集 |
| `WaitCanonicalRunRequest` | path: `thread_id`; body: create fields 加 `raise_error` |
| `CanonicalRunRouteRequest` | path: `thread_id,run_id`; stream/join/cancel 的 query 由各自专用 struct 扩展 |
| `ResumeCanonicalRunRequest` | path: `thread_id,run_id`; body: `interrupt_id,response` |
| `ListCanonicalRunEventsRequest` | path: `thread_id,run_id`; query: `after_event_id,event_types,limit` |
| `ListCanonicalRunMessagesRequest` | path: `thread_id,run_id`; query: `before_seq,after_seq,limit` |

每个 request 还包含 `255: optional base.Base Base (api.none="true")`。共享字段使用相同
Thrift type、JSON 名和 optional/required 规则；`api.value_type="any"` 只用于动态 JSON，
不得用于 ID、状态、分页或 mode。

数组和 raw values 响应必须使用上面的 `api.body="."` wrapper，使生成 TypeScript client 的
返回类型仍是 `Thread[]`、`Run[]`、`ThreadState[]` 或 `any`，而不是 `{body: ...}`。
IDL 只负责 codegen 元数据和生成 client 类型。手写 handler 必须直接严格解析原始 JSON body，
不能给 SDK 请求增加 `payload` 包装，也不能给成功响应增加 envelope。

- [ ] **Step 4: Re-export the service from both IDL roots**

在 `idl/workbench/workbench.thrift` 增加 `include "./thread.thrift"` 和：

```thrift
service WorkbenchCanonicalThreadService extends thread.WorkbenchCanonicalThreadService {}
```

在 `idl/api.thrift` 增加：

```thrift
service WorkbenchCanonicalThreadService extends workbench.WorkbenchCanonicalThreadService {}
```

- [ ] **Step 5: Register the frontend IDL entry and exports**

在 `api.config.js` 的 `entries` 中增加：

```js
workbenchThread: './idl/workbench/thread.thrift',
```

在 package exports、`typesVersions` 和 `src/index.ts` 中导出 `./idl/workbench/thread`，命名规则与现有 `workbenchTask` 保持一致。

- [ ] **Step 6: Run exact code generators**

先核对并执行仓库固定版本与完整生成命令：

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

Expected versions: `hz version v0.9.7` and `thriftgo 0.4.5`。生成后用 `apply_patch`
把 `backend/api/router/coze/middleware.go` 中 `_adminMw()` 恢复为：

```go
func _adminMw() []app.HandlerFunc {
	return []app.HandlerFunc{adminAuthMiddlewareFactory()}
}
```

对新增 generator-owned Go 文件补仓库 license header 并运行 `gofmt`。随后运行只读验证器：

```bash
cd backend
bash scripts/verify_api_codegen.sh
```

Expected: generated baseline 与 clean generation 完全一致，routes 含全部 21 methods，
`WORKSPACE_READ_ONLY_SHA256=PASS` 和 `CODEGEN_DETERMINISTIC_SHA256=PASS`。

Run:

```bash
cd frontend/packages/arch/api-schema
pnpm run update
```

Expected: `src/idl/workbench/thread.ts` is generated and contains the exported service/types.

- [ ] **Step 7: Make generated handlers compile without product logic**

把 codegen 新增的 stub 收敛到 `workbench_canonical_entrypoints.go`：21 个 exported handler
全部先调用同一 fail-closed helper；默认关闭返回 `404`，测试显式打开时返回
`501 canonical_not_implemented`。这一步只用于让 route snapshot 从“路由不存在”推进为
“路由存在且受门控”，不得调用 application service。

- [ ] **Step 8: Run route and schema tests**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/router/coze ./api/handler/coze -run 'TestWorkbenchCanonicalThreadRoutes' -count=1
```

Run:

```bash
cd frontend
node common/scripts/install-run-rush.js test --to @coze-studio/api-schema -- --run workbench-thread-contract
```

Expected: PASS.

- [ ] **Step 9: Commit IDL and generated contract**

```bash
git add idl/workbench/thread.thrift idl/workbench/workbench.thrift idl/api.thrift backend/api/model backend/api/router/coze backend/api/handler/coze frontend/packages/arch/api-schema
git commit -m "feat: add canonical workbench thread contract"
```

### Task 3: Implement Strict Contract Foundation And Safe Observability

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_contract.go`
- Create: `backend/api/handler/coze/workbench_canonical_contract_test.go`

- [ ] **Step 1: Write failing tests for feature gate, strict JSON and public errors**

覆盖以下行为：

```go
func TestCanonicalGateDefaultsToNotFound(t *testing.T)
func TestCanonicalGateAcceptsOnlyExplicitTrue(t *testing.T)
func TestCanonicalDecodeRejectsUnknownField(t *testing.T)
func TestCanonicalDecodeRejectsTrailingJSON(t *testing.T)
func TestCanonicalParseIDRejectsZeroNegativeAndNonDecimal(t *testing.T)
func TestCanonicalSpaceIDRequiresAuthorizedHeader(t *testing.T)
func TestCanonicalErrorNeverLeaksCauseOrPayload(t *testing.T)
func TestCanonicalTraceIDUsesRequestLogID(t *testing.T)
func TestCanonicalHeadersAreAPIBaseRelative(t *testing.T)
func TestCanonicalPaginationHeadersAreStable(t *testing.T)
```

测试 error body 必须精确匹配：

```json
{"detail":"Unsupported field: checkpoint_during","code":"unsupported_sdk_field","retryable":false,"trace_id":"trace-test"}
```

- [ ] **Step 2: Run the contract tests and verify RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestCanonical(Gate|Decode|Parse|Space|Error|Trace|Headers|Pagination)' -count=1
```

Expected: FAIL because helpers/types are undefined.

- [ ] **Step 3: Implement the contract helpers**

定义并使用以下核心接口：

```go
const canonicalAPIEnabledEnv = "COZE_WORKBENCH_CANONICAL_API_ENABLED"
const canonicalContractVersion = "canonical_v1"

type canonicalError struct {
	Detail    string `json:"detail"`
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
	TraceID   string `json:"trace_id"`
}

type canonicalRequestLog struct {
	Operation     string
	RouteTemplate string
	ThreadID      int64
	RunID         int64
	StartedAt     time.Time
}

func canonicalAPIEnabled(getenv func(string) string) bool
func requireCanonicalAPI(ctx context.Context, c *app.RequestContext) bool
func decodeCanonicalJSON(c *app.RequestContext, dst any) *canonicalError
func canonicalPathID(c *app.RequestContext, name string) (int64, *canonicalError)
func canonicalQueryInt64(c *app.RequestContext, name string) (*int64, *canonicalError)
func canonicalSpaceID(ctx context.Context, c *app.RequestContext) (int64, *canonicalError)
func writeCanonicalError(ctx context.Context, c *app.RequestContext, status int, public canonicalError)
func canonicalTraceID(ctx context.Context) string
func canonicalRunPath(threadID, runID int64) string
func canonicalRunStreamPath(threadID, runID int64) string
func canonicalRunJoinPath(threadID, runID int64) string
func setCanonicalPaginationHeaders(c *app.RequestContext, total int64, offset, limit int)
func logCanonicalRequestCompleted(ctx context.Context, c *app.RequestContext, info canonicalRequestLog, outcome string)
```

`decodeCanonicalJSON` 必须使用 `json.Decoder.DisallowUnknownFields()`，拒绝空 body、多个 JSON 值和非 object body。`canonicalTraceID` 优先读取 `consts.CtxLogIDKey`，不生成第二个不可关联 ID。日志使用稳定 event name，如：

本批次没有可复用的“当前空间”服务端 context，因此 canonical create/search 明确要求
`X-Coze-Space-ID`。`canonicalSpaceID` 只把该 header 当空间声明，随后调用现有
`ApplicationService.AuthorizeWorkspaceAccess` 验证 session viewer 的成员关系；body metadata、
owner、creator 或 user ID 不能提供/覆盖空间。缺失或非法 header 返回 `400`，无权空间按
防枚举规则返回 `404`。这不会改变当前 UI，因为 UI 尚未切换 canonical client。

```go
logs.CtxInfof(ctx,
	"event_name=workbench.api.request.completed client_contract=%s operation=%s route_template=%s http_method=%s http_status=%d duration_ms=%d outcome=%s thread_id=%d run_id=%d",
	canonicalContractVersion, info.Operation, info.RouteTemplate, string(c.Method()), c.Response.StatusCode(),
	time.Since(info.StartedAt).Milliseconds(), outcome, info.ThreadID, info.RunID,
)
```

不得记录 request/response body、message content、metadata 值、token、Cookie、header 原值、文件名、tool/provider payload 或错误链。

- [ ] **Step 4: Map application/domain errors to canonical HTTP semantics**

建立集中 mapper，至少覆盖：格式 `400`、未认证 `401`、access denied `404`（防枚举）、状态/幂等冲突 `409`、不支持字段/值 `422`、依赖不可用 `503`、未知内部错误 `500`。公开 `detail` 使用稳定安全文案；内部 `err` 仅以受控 `error_class` 记录，不直接写 body 或日志。

- [ ] **Step 5: Run tests and commit**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestCanonical(Gate|Decode|Parse|Space|Error|Trace|Headers|Pagination)' -count=1
git add api/handler/coze/workbench_canonical_contract.go api/handler/coze/workbench_canonical_contract_test.go
git commit -m "feat: add canonical API contract guards"
```

Expected: PASS.

### Task 4: Implement Central Public Projections

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_projection.go`
- Create: `backend/api/handler/coze/workbench_canonical_projection_test.go`

- [ ] **Step 1: Write failing status and redaction tests**

用 table tests 冻结以下映射：

```go
var canonicalRunStatusCases = []struct {
	internal       string
	sdk            string
	terminalReason *string
}{
	{"pending", "pending", nil},
	{"queued", "pending", nil},
	{"running", "running", nil},
	{"interrupted", "interrupted", nil},
	{"succeeded", "success", nil},
	{"failed", "error", nil},
	{"canceled", "interrupted", ptr("canceled")},
}
```

Thread 投影覆盖 `idle|busy|interrupted|error` 和 `coze.product_status`。Run JSON 必须断言不包含 `input`、`command`、`config`、`context`、`worker_id`、`idempotency_key`、内部错误链。Message/Event/State 必须断言不包含 checkpoint bytes、工具原始参数/结果、provider body、隐藏配置或 `legacy_task_id`。

- [ ] **Step 2: Run projection tests and verify RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestCanonical.*Projection' -count=1
```

Expected: FAIL because projection types/functions do not exist.

- [ ] **Step 3: Implement one projection table and public DTO family**

定义：

```go
type canonicalThread struct {
	ThreadID  string                 `json:"thread_id"`
	CreatedAt string                 `json:"created_at"`
	UpdatedAt string                 `json:"updated_at"`
	Metadata  map[string]any         `json:"metadata"`
	Status    string                 `json:"status"`
	Values    map[string]any         `json:"values"`
	Interrupts map[string]any        `json:"interrupts"`
	Coze      canonicalThreadCoze    `json:"coze"`
}

type canonicalRun struct {
	RunID             string           `json:"run_id"`
	ThreadID          string           `json:"thread_id"`
	AssistantID       string           `json:"assistant_id"`
	Status            string           `json:"status"`
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
	Metadata          map[string]any   `json:"metadata"`
	MultitaskStrategy string           `json:"multitask_strategy"`
	Coze              canonicalRunCoze `json:"coze"`
}

func projectCanonicalThread(ctx context.Context, summary *appagentthread.ThreadSummary) (*canonicalThread, error)
func projectCanonicalRun(summary *appagentthread.RunSummary) (*canonicalRun, error)
func projectCanonicalMessage(summary *appagentthread.MessageSummary) (*canonicalMessage, error)
func projectCanonicalRunEvent(summary *appagentthread.RunEventSummary) (*canonicalRunEvent, error)
func projectCanonicalThreadState(source canonicalStateSource) (*canonicalThreadState, error)
```

所有 ID 使用 `strconv.FormatInt(..., 10)`；时间统一为 `time.UnixMilli(...).UTC().Format(time.RFC3339Nano)` 或与实体单位相符的唯一 helper。metadata 只保留 title 和审核后的普通业务 key，拒绝/清洗 owner、creator、space、runtime、credential、secret、token、`legacy_task_id`。

- [ ] **Step 4: Implement stable state/update-state shapes**

`canonicalThreadState` 必须含 `values,next,checkpoint,metadata,created_at,parent_checkpoint,tasks,interrupts`。同一次 state update 生成一个 checkpoint projection，并同时返回：

```go
type canonicalThreadUpdateStateResult struct {
	Checkpoint   canonicalCheckpoint `json:"checkpoint"`
	Configurable canonicalCheckpoint `json:"configurable"`
}
```

两个字段由同一个局部变量赋值，测试使用 `assert.Equal` 验证完全一致。

- [ ] **Step 5: Run tests and commit**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestCanonical.*Projection' -count=1
git add api/handler/coze/workbench_canonical_projection.go api/handler/coze/workbench_canonical_projection_test.go
git commit -m "feat: add canonical thread public projections"
```

Expected: PASS.

### Task 4A: Add Missing Application And Transaction Capabilities

**Files:**
- Create: `backend/application/agentthread/canonical_contract.go`
- Create: `backend/application/agentthread/canonical_contract_test.go`
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Create: `backend/domain/agentthread/service/canonical_contract.go`
- Create: `backend/domain/agentthread/service/canonical_contract_test.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Create: `backend/domain/agentthread/repository/canonical_contract.go`
- Create: `backend/domain/agentthread/repository/mysql_canonical.go`
- Create: `backend/domain/agentthread/repository/mysql_canonical_test.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Test: existing `backend/application/agentthread/service_test.go`
- Test: existing `backend/domain/agentthread/service/service_impl_test.go`
- Test: existing `backend/domain/agentthread/repository/mysql_test.go`

这一任务不是重写业务流程，而是补足真实源码中已确认缺失的事务/查询能力。禁止在 handler
中用“先读再写”“先查 busy 再 delete”或整表拉回内存模拟生产语义。

- [ ] **Step 1: Write failing source-compatibility tests before shared-layer edits**

新增测试冻结当前零值行为：

```go
func TestCanonicalExtensionsKeepLegacyCreateTaskThreadDefaults(t *testing.T)
func TestCanonicalExtensionsKeepLegacyListThreadsPagingAndOrder(t *testing.T)
func TestCanonicalExtensionsKeepLegacyListRunsPagingAndOrder(t *testing.T)
func TestCanonicalExtensionsKeepLegacyListMessagesPagingAndOrder(t *testing.T)
func TestCanonicalExtensionsKeepLegacyListRunEventsPagingAndOrder(t *testing.T)
func TestCanonicalExtensionsKeepLegacyDeleteThreadCascade(t *testing.T)
```

这些测试使用当前 handler/application 传入的零值 request，精确断言 SQL 过滤、排序、分页、
Thread source/metadata 和删除级联与修改前相同。

- [ ] **Step 2: Run compatibility tests and establish GREEN baseline**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" \
  ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository \
  -run 'TestCanonicalExtensionsKeepLegacy' -count=1
```

Expected: PASS before production changes; these are characterization tests, not RED tests.

- [ ] **Step 3: Write failing atomic initial/deferred submission tests**

给 `CreateTaskThreadRequest` 增加 canonical-only optional fields的测试先行定义：

```go
type CreateTaskThreadRequest struct {
	// Existing fields remain unchanged.
	ThreadMetadata string
	ThreadSource   ThreadSource
}
```

测试必须断言：

- 两个字段为空时仍持久化 `source=web` 和 `{"source":"workbench_new_task"}`；
- canonical initial 使用同一 `CreateThreadRunMessage` 事务持久化审核后的 Thread metadata、
  User Message 和 Run；
- canonical deferred 先执行现有 `normalizeNewRunRuntimeConfig` 与标题推导，再只创建 Thread；
- 运行配置非法时 Thread/Message/Run/Event 均为零；
- Run metadata 与 Thread metadata 分开传递，不能互相覆盖。

- [ ] **Step 4: Implement additive submission fields and re-run tests**

在 `CreateTaskThread` 中只对非空新字段选择新值：

```go
threadSource := req.ThreadSource
if threadSource == "" {
	threadSource = ThreadSourceWeb
}
threadMetadata := strings.TrimSpace(req.ThreadMetadata)
if threadMetadata == "" {
	threadMetadata = `{"source":"workbench_new_task"}`
}
```

deferred 的 `CreateThreadRequest` 和 initial 的 `CreateThreadRunMessage.Thread` 共用这两个局部值。
当前 handler 不设置新字段，因此原流程保持原值。

- [ ] **Step 5: Write failing repository search and exact-offset tests**

在 application/domain/repository 的 request 中增加以下 optional 字段，全部先写 MySQL/SQLite
repository tests：

```go
type CanonicalPage struct {
	Offset int32
	Limit  int32
}

type SearchThreadsRequest struct {
	SpaceID      int64
	UserID       int64
	IDs          []int64
	Status       *entity.ThreadStatus
	Metadata     map[string]any
	SortBy       string
	SortOrder    string
	Page         CanonicalPage
}

type SearchRunsRequest struct {
	ThreadID    int64
	ParentRunID *int64
	Status      *entity.RunStatus
	Page        CanonicalPage
}

type ListRunEventsByCursorRequest struct {
	ThreadID     int64
	RunID        int64
	AfterEventID int64
	EventTypes   []string
	Limit        int32
}

type ListCheckpointsBeforeRequest struct {
	ThreadID          int64
	BeforeCheckpointID int64
	Limit             int32
}
```

测试 arbitrary `offset=7,limit=3`、ids、top-level scalar metadata equality、每种 sort、同方向
`thread_id` tie-breaker、权限过滤后的 total、event type+cursor、checkpoint before cursor。
metadata key 必须匹配 `^[A-Za-z][A-Za-z0-9_.-]{0,63}$`，最多 16 个；value 只允许
string/bool/finite number/null，object/array 返回 domain invalid argument。

- [ ] **Step 6: Implement separate canonical query methods**

不要改变现有 `ListThreads/ListRuns/ListRunEvents/ListCheckpoints` 的 SQL 分支；新增：

```go
SearchThreads(ctx context.Context, req SearchThreadsRequest) ([]*entity.Thread, int64, error)
SearchRuns(ctx context.Context, req SearchRunsRequest) ([]*entity.Run, int64, error)
ListRunEventsByCursor(ctx context.Context, req ListRunEventsByCursorRequest) ([]*entity.RunEvent, bool, error)
ListCheckpointsBefore(ctx context.Context, req ListCheckpointsBeforeRequest) ([]*entity.Checkpoint, bool, error)
```

repository 使用 GORM 参数绑定和 `datatypes.JSONQuery("metadata").Equals(value, key)`；sort column
由 switch 映射固定 SQL 字符串，绝不拼接客户端原值。查询 `limit+1` 计算 `has_more`，total 和
cursor 都在 space/user/thread 过滤后计算。ApplicationService 为这些方法补授权与 DTO projection。

- [ ] **Step 7: Write failing atomic patch tests**

定义：

```go
type PatchThreadRequest struct {
	ThreadID     int64
	Title        *string
	MetadataPatch map[string]any
	UpdatedAt    int64
}
```

repository test 在 transaction 内锁定 Thread row、读取并 merge 当前 metadata、一次 update 同时写
title/metadata/updated_at，然后返回同一 committed snapshot。并发 patch 测试必须证明不同 key
不会因 read-modify-write 丢失；无字段、空 title、保护 key 返回 invalid argument 且零写入。

- [ ] **Step 8: Implement PatchThread through all three layers**

新增 `PatchThread`，保护 metadata keys 固定为：`space_id,user_id,owner_id,creator_id,status,runtime,
credential,credentials,secret,token,api_key,legacy_task_id`。ApplicationService 先执行 Thread 授权；
domain service 校验 title/metadata；repository 完成 row-lock transaction。现有
`UpdateThreadTitle/UpdateThreadMetadata` 保留，旧 handler 不改调用点。

- [ ] **Step 9: Write failing race-safe busy-delete tests**

新增 `DeleteThreadIfIdle`。测试同时启动 `CreateRunBundle` 与 delete：两者都锁同一 Thread row，
只允许以下线性化结果：Run 先提交则 delete 返回 `ErrActiveRunExists` 且数据完整；delete 先提交则
Run 创建失败且不存在孤立 Message/Event。pending/queued/running 顶层 Run 阻止删除；
interrupted/succeeded/failed/canceled 与 child-only Run 不定义为 busy。

- [ ] **Step 10: Implement DeleteThreadIfIdle without changing legacy delete**

把现有 cascade statements 提取为 transaction-local unexported helper。`DeleteThread` 继续直接调用
该 helper；`DeleteThreadIfIdle` 先 row lock、查询 active top-level Run，再调用同一 helper。
不得通过 delete 隐式 cancel。ApplicationService 新方法做授权并把 `ErrActiveRunExists` 原样上抛。

- [ ] **Step 11: Write failing isolated public-state tests**

定义 canonical public state use case：

```go
type UpdatePublicThreadStateRequest struct {
	ThreadID         int64
	BaseCheckpointID int64
	Values           map[string]any
	AsNode           string
}

type UpdatePublicThreadStateResponse struct {
	Checkpoint *CheckpointSummary
}
```

首期唯一可写 channel 为 `custom`，值必须是最大 64 KiB 的 JSON object；messages、Run status、
artifacts、usage、audit、interrupts、runtime、uploaded_files 等返回 `ErrUnsupportedPublicStateChannel`。
测试断言新 checkpoint 使用：

```text
runtime_type = canonical_public_state
runtime_key  = thread:{thread_id}
checkpoint_ns = canonical.public
```

它引用 Thread 最新顶层 Run 和选定 parent checkpoint，只保存审核后的 public JSON；Eino ADK
`checkpoint_bytes`、envelope、runtime key 和现有 checkpoint row 不修改。无 Run 的 draft Thread
返回稳定 conflict，base checkpoint 不属于 Thread 返回 not found/denied。

- [ ] **Step 12: Implement UpdatePublicThreadState and query support**

ApplicationService 授权并读取 Thread、latest top-level Run、base/latest checkpoint，merge 上一个
`canonical_public_state` 的 `custom` map，再调用现有 `CreateCheckpoint`。history 查询同时返回 Eino
公开投影和 canonical public checkpoints；runtime resume 仍只使用
`GetLatestRuntimeCheckpoint(runtime=eino_adk, runtime_key=...)`。

- [ ] **Step 13: Run shared-layer tests and legacy regressions**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" \
  ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository \
  -run 'Test(Canonical|ApplicationCreateTaskThread|ListThreads|ListRuns|ListRunEvents|ListCheckpoints|ThreadRepositoryDeleteThread)' \
  -count=1
```

Expected: PASS, including all Step 1 characterization tests.

- [ ] **Step 14: Commit additive shared capabilities**

```bash
git add application/agentthread domain/agentthread/service domain/agentthread/repository
git commit -m "feat: add canonical agent thread use cases"
```

### Task 5: Implement Thread Core Endpoints

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_thread_service.go`
- Create: `backend/api/handler/coze/workbench_canonical_thread_service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_contract.go`

- [ ] **Step 1: Write failing create-thread tests**

覆盖三类互斥请求；每个请求都带 `X-Coze-Space-ID: 1001`，fixture 必须证明该空间已由
当前 session viewer 授权：

```json
{"metadata":{"title":"新建任务"}}
```

```json
{"metadata":{"title":"新建任务"},"coze":{"initial_run":{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"请生成产品发布方案"}]},"config":{"runtime":"eino_adk","mode":"pro"},"metadata":{"source":"workbench_home"}}}}
```

```json
{"metadata":{},"coze":{"deferred_initial_run":{"assistant_id":"agent","input":{"messages":[{"role":"user","content":"分析附件中的销售数据"}]},"config":{"runtime":"eino_adk","mode":"pro"},"metadata":{"source":"workbench_home_with_uploads"}}}}
```

断言：空 Thread 只创建 Thread；initial 在一个 application bundle 中创建 Thread+User Message+Run；deferred 先校验运行配置并复用当前标题推导，只创建 Thread。两种 initial 同时提交、客户端 `thread_id`、`ttl`、`supersteps`、`metadata.graph_id`、`if_exists!=raise` 返回 `422`，且零副作用。

- [ ] **Step 2: Run create tests and verify RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestCreateCanonicalThread' -count=1
```

Expected: FAIL because handler still returns `501`.

- [ ] **Step 3: Implement CreateCanonicalThread**

严格解析以下私有 DTO：

```go
type canonicalCreateThreadRequest struct {
	ThreadID  *string                    `json:"thread_id,omitempty"`
	Metadata  map[string]any             `json:"metadata,omitempty"`
	IfExists *string                    `json:"if_exists,omitempty"`
	TTL       any                        `json:"ttl,omitempty"`
	Supersteps []json.RawMessage         `json:"supersteps,omitempty"`
	Coze      *canonicalCreateThreadCoze `json:"coze,omitempty"`
}
```

使用 `workbenchThreadAccessContext` 与 `workbenchViewerIDFromCtx` 建立现有授权上下文；空 Thread
调 `CreateThread`，initial/deferred 调 Task 4A 扩展后的 `CreateTaskThread`，分别传入审核后的
`ThreadMetadata/ThreadSource` 和 Run metadata。只在 adapter 中映射 body/header，不复制
application 事务、运行配置校验或标题算法。响应为 raw `canonicalThread`，initial 摘要放入
`coze.initial_submission`。

- [ ] **Step 4: Write failing search/get/patch/delete tests**

覆盖：授权空间 search、默认 `updated_at DESC, thread_id DESC`、`status/ids/metadata/limit/offset`、分页 headers、拒绝 `values/select/extract`；get 拒绝非空 `include`；patch 只允许 title/安全 metadata，精确支持 `Prefer: return=minimal`；busy delete `409 thread_busy` 且不调用 DeleteThread，non-busy 调现有 DeleteThread 并返回 `204`。

- [ ] **Step 5: Implement search/get/patch/delete**

每个 exported handler 加 GoDoc，说明 canonical 身份、认证前提、application use case、响应体和
副作用。search 调 `ApplicationService.SearchThreads`，patch 调 `PatchThread`，delete 调
`DeleteThreadIfIdle`；handler 不做 read-modify-write 或 busy pre-check。metadata/status/sort
基础校验在调用 application 之前完成，domain 再校验一次；Thread/space/owner 只能来自 session
context 与已授权资源，不从 body 推导。

- [ ] **Step 6: Write failing state/history/messages tests**

覆盖：GET state 的 `subgraphs` 省略/false；true 返回 `422`；POST state 仅允许 Task 4A 的
`custom` channel，返回一致的 `checkpoint/configurable`；对 Eino runtime bytes 做 before/after
hash 断言；GET/POST history 返回同序同形；messages 使用 `before_seq/after_seq/limit` 并返回
`{data,has_more,next_before_seq,next_after_seq}`。

- [ ] **Step 7: Implement state/history/messages adapters**

GET state 复用现有公开 checkpoint projection；POST state 调
`ApplicationService.UpdatePublicThreadState`；history 使用 `ListCheckpointsBefore`。messages 复用
`ProjectThreadRunJournalMessages`，按 application 分页循环读取而不是固定截断 200/500 条；
canonical projection 再按 `created_at ASC, source kind ASC, numeric message/event ID ASC` 排序，
以完整 append-only 序列中的 `index+1` 作为 seq 后应用 cursor。公开 state/history 只能使用 TaskThread state、公开
checkpoint opaque ID 和审核后的 interrupts；不得回显内部 checkpoint bytes/channel versions/
task result/error。

- [ ] **Step 8: Run Thread tests and source-route regressions**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze -run 'Test(Create|Search|Get|Patch|Delete|Update|List).*CanonicalThread|TestWorkbenchCanonicalThreadRoutes|Test.*TaskThread' -count=1
```

Expected: PASS; current TaskThread snapshots unchanged.

- [ ] **Step 9: Commit Thread endpoints**

```bash
git add api/handler/coze/workbench_canonical_contract.go api/handler/coze/workbench_canonical_thread_service.go api/handler/coze/workbench_canonical_thread_service_test.go
git commit -m "feat: implement canonical thread endpoints"
```

### Task 6: Implement Run Validation And Non-Streaming Endpoints

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Create: `backend/api/handler/coze/workbench_canonical_run_service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_contract.go`

- [ ] **Step 1: Write failing run request validation tests**

冻结默认值与 allowlist：

```go
var canonicalRunDefaults = canonicalRunOptions{
	StreamModes:       []string{"values"},
	MultitaskStrategy: "reject",
	OnDisconnect:      "cancel",
	Durability:        "async",
}
```

接受 `stream_mode` string 或 array；allowlist 为 `values,updates,messages,messages-tuple,custom,events`。`stream_subgraphs=false`、`if_not_exists=reject`、`stream_resumable=false` 和已声明 nullable SDK 字段可接受；任何未知字段、未知 mode、非空 webhook/on_completion/after_seconds/feedback_keys/interrupt/checkpoint/langsmith_tracer、`checkpoint_during`、`multitask_strategy!=reject`、`durability!=async` 返回 `422` 且零副作用。

- [ ] **Step 2: Run validation tests and verify RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestCanonicalRunRequest' -count=1
```

Expected: FAIL because run request parser does not exist.

- [ ] **Step 3: Implement strict run DTO normalization**

定义 private DTO，并把 accepted fields 显式列全：

```go
type canonicalCreateRunRequest struct {
	AssistantID       string          `json:"assistant_id"`
	Input             json.RawMessage `json:"input"`
	Command           json.RawMessage `json:"command,omitempty"`
	Metadata          map[string]any  `json:"metadata,omitempty"`
	Config            json.RawMessage `json:"config,omitempty"`
	Context           json.RawMessage `json:"context,omitempty"`
	StreamMode        json.RawMessage `json:"stream_mode,omitempty"`
	MultitaskStrategy string          `json:"multitask_strategy,omitempty"`
	OnDisconnect      string          `json:"on_disconnect,omitempty"`
	Durability        string          `json:"durability,omitempty"`
	StreamResumable   *bool           `json:"stream_resumable,omitempty"`
	StreamSubgraphs   *bool           `json:"stream_subgraphs,omitempty"`
	IfNotExists       string          `json:"if_not_exists,omitempty"`
	RaiseError        *bool           `json:"raise_error,omitempty"`
	Webhook           json.RawMessage `json:"webhook,omitempty"`
	OnCompletion      json.RawMessage `json:"on_completion,omitempty"`
	AfterSeconds      json.RawMessage `json:"after_seconds,omitempty"`
	FeedbackKeys      json.RawMessage `json:"feedback_keys,omitempty"`
	InterruptBefore   json.RawMessage `json:"interrupt_before,omitempty"`
	InterruptAfter    json.RawMessage `json:"interrupt_after,omitempty"`
	Checkpoint        json.RawMessage `json:"checkpoint,omitempty"`
	CheckpointID      json.RawMessage `json:"checkpoint_id,omitempty"`
	LangsmithTracer   json.RawMessage `json:"langsmith_tracer,omitempty"`
}
```

只接受本次 turn 的一个 User Message；拒绝历史消息数组、伪造 Assistant Message 或覆盖权威历史。`Idempotency-Key` 只从 header 读取并透传现有 application request。

- [ ] **Step 4: Write failing create/list/get tests**

断言 CreateCanonicalRun 调用 `ApplicationService.CreateRun` 的 MessageContent bundle 路径，原子创建 User Message+Run；response 不回显敏感请求字段；`Content-Location` 精确为 `/threads/{thread_id}/runs/{run_id}`。list/get 校验 Run 属于 path Thread，分页 header 正确，非空 `select` 返回 `422`。

- [ ] **Step 5: Implement create/list/get**

使用 `workbenchThreadAccessContext(ctx, threadID, runID)` 后调用
`CreateRun/SearchRuns/GetRun`。所有 thread/run mismatch 在 projection 前拒绝；日志记录 ID、
operation、accepted mode、header kind 和稳定 error code，不记录消息或配置。

- [ ] **Step 6: Write failing wait/join/cancel/resume tests**

覆盖：

- wait/join 成功返回 raw values，不是 Run/ThreadState/envelope；
- failed wait `raise_error=true` 返回非 2xx canonical error；省略/false 返回 `200` 和安全 `__error__` values；
- join failed 固定返回 `200` 和同一 `__error__` values；
- join `cancel_on_disconnect` 只接受省略/false/0；
- cancel 只接受 `action=interrupt` 和 `wait=0|1`，幂等返回 `204`，rollback 为 `422`；
- resume 来源必须是 interrupted Run，成功创建新 attempt，`coze.source_run_id` 指向来源 Run；
- `POST runs` 的 `command.resume` 进入同一个 resume use case；
- 所有等待测试有明确 timeout，超时不隐式取消 Run。

- [ ] **Step 7: Implement wait/join/cancel/resume**

复用现有 wait/poll helper 或提取 current LangGraph handler 的无协议业务 helper；不得重用其错误 envelope、POST join 或 SSE join 语义。resume 调用 `ResumeHumanInteraction`，cancel 调用 `CancelRun`；handler 不伪造 application 状态迁移日志。

- [ ] **Step 8: Write failing events/messages page tests and implement them**

events 使用 Task 4A 的 `ListRunEventsByCursor`，接受 `after_event_id,event_types,limit`，按
event ID 升序，返回 `{data,has_more,next_after_event_id}`。Run messages 与 Thread messages
使用同一完整 journal loader、排序和 `before_seq,after_seq,limit` cursor；同一 Message 的
`message_id/role/content/created_at/run_id/seq` 完全一致。

- [ ] **Step 9: Run non-stream Run tests and regressions**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze -run 'TestCanonical(Run|CreateRun|ListRun|GetRun|Wait|Join|Cancel|Resume|Events|Messages)|Test.*LangGraph(Thread|Run)|Test.*TaskThread' -count=1
```

Expected: PASS; `/api/threads` and `/api/workbench/task_threads` snapshots unchanged.

- [ ] **Step 10: Commit non-stream Run endpoints**

```bash
git add api/handler/coze/workbench_canonical_contract.go api/handler/coze/workbench_canonical_run_service.go api/handler/coze/workbench_canonical_run_service_test.go
git commit -m "feat: implement canonical run endpoints"
```

### Task 7: Implement Canonical SSE Create And Reconnect

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_stream.go`
- Create: `backend/api/handler/coze/workbench_canonical_stream_test.go`

- [ ] **Step 1: Write failing SSE frame and mode tests**

覆盖单 mode、多 mode、`messages-tuple -> event: messages`、metadata 首帧、持久化 event ID、error/end、UTF-8、分片/合并和 heartbeat 不分配业务 ID。metadata data 至少含 `thread_id,run_id,trace_id,modes`。

- [ ] **Step 2: Run SSE tests and verify RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestCanonicalSSE' -count=1
```

Expected: FAIL because canonical stream writer does not exist.

- [ ] **Step 3: Implement SSE writer and public event mapping**

定义集中 frame writer：

```go
type canonicalSSEFrame struct {
	ID    string
	Event string
	Data  any
}

func writeCanonicalSSEFrame(w io.Writer, frame canonicalSSEFrame) error
func canonicalSSEEventName(requestedMode string, event *appagentthread.RunEventSummary) (string, bool)
func canonicalSSEData(requestedMode string, event *appagentthread.RunEventSummary) (any, error)
```

业务 event 使用公开 projection；不得把 raw payload、tool args/results、provider body 或 checkpoint bytes直接写入 `data`。高频 chunk 不逐条 INFO。

- [ ] **Step 4: Write failing cursor/replay/live transition tests**

测试 `Last-Event-ID` 与 `after_event_id` 都为合法 64 位十进制时取较大值；非法/负数返回 `400`；GET stream 的 `cancel_on_disconnect` 只接受省略/0；回放 cursor 后事件，再 attach live，边界事件不丢不重；终态只发一次 end。

- [ ] **Step 5: Implement POST create-stream and GET reconnect-stream**

POST 先复用 Task 6 的 run parser/create helper，再写：

```text
Content-Location: /threads/{thread_id}/runs/{run_id}
Location: /threads/{thread_id}/runs/{run_id}/stream
Content-Type: text/event-stream
```

GET 只连接既有 Run，不创建新 Run，并返回同一 Location。记录 `workbench.stream.opened/replayed/live_attached/closed`，使用同一 `trace_id + stream_connection_id`，只记录 modes、cursor、回放数量、close reason 和安全错误码。

- [ ] **Step 6: Add disconnect semantics tests**

验证 `on_disconnect=continue` 不取消 Run；`cancel` 仅在确认 client disconnect 后调用现有 `CancelRunOnDisconnect`；正常 terminal close 不重复取消。来源 route 的 disconnect 行为保持原样。

- [ ] **Step 7: Run SSE and full handler tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze -run 'TestCanonicalSSE|TestCanonical.*Stream|Test.*LangGraph.*Stream|Test.*TaskThread.*Stream' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit SSE endpoints**

```bash
git add api/handler/coze/workbench_canonical_stream.go api/handler/coze/workbench_canonical_stream_test.go
git commit -m "feat: add canonical run streaming"
```

### Task 8: Add In-Process Compatibility Server Fixture

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_sdk_compat_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_*_test.go` only if extracting shared fixtures

- [ ] **Step 1: Write a skipped-by-default SDK harness test**

```go
func TestWorkbenchCanonicalSDKCompatibility(t *testing.T) {
	if os.Getenv("WORKBENCH_SDK_COMPAT") != "1" {
		t.Skip("set WORKBENCH_SDK_COMPAT=1 to run locked LangGraph SDK clients")
	}
	// Start an in-process Hertz server with canonical gate enabled,
	// deterministic session auth, recording AgentThread service and worker fixture.
}
```

普通 package test 必须只 skip 该测试，不探测 Node/Python、不联网、不安装依赖。

- [ ] **Step 2: Implement deterministic auth and worker fixture**

fixture 使用现有 `installAgentThreadTestService` 方式注入同一 application service；测试 auth 中间件固定 viewer/space。worker 循环只 claim fixture 创建的 pending Run，追加经过公开 mapper 可识别的 messages/updates/events，最后提交 success/error/interrupted。测试结束必须关闭 server、worker、subscriptions 和临时资源，不残留 goroutine。

- [ ] **Step 3: Expose fixture metadata to child SDK processes**

设置：

```go
env := append(os.Environ(),
	"WORKBENCH_SDK_API_URL="+serverURL+"/api/workbench",
	"WORKBENCH_SDK_SPACE_ID=1001",
	"WORKBENCH_SDK_TEST_SESSION=canonical-test-session",
)
```

子进程 stdout 只允许输出一行 JSON summary；stderr 捕获后只在测试失败时输出，且 fixture 内容不得含 secret 或用户数据。

- [ ] **Step 4: Run the ordinary Go test and verify safe skip**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestWorkbenchCanonicalSDKCompatibility' -count=1 -v
```

Expected: PASS with one explicit skip.

- [ ] **Step 5: Commit fixture**

```bash
git add api/handler/coze/workbench_canonical_sdk_compat_test.go
git commit -m "test: add canonical SDK server fixture"
```

### Task 9: Add Locked JavaScript SDK 1.6.0 Compatibility Client

**Files:**
- Create: `backend/api/handler/coze/testdata/workbench_sdk_compat/js/package.json`
- Create: `backend/api/handler/coze/testdata/workbench_sdk_compat/js/pnpm-lock.yaml`
- Create: `backend/api/handler/coze/testdata/workbench_sdk_compat/js/compat.mjs`
- Modify: `backend/api/handler/coze/workbench_canonical_sdk_compat_test.go`

- [ ] **Step 1: Pin the package and generate a frozen lock**

`package.json`：

```json
{
  "name": "workbench-canonical-sdk-compat-js",
  "private": true,
  "type": "module",
  "engines": {"node": ">=21"},
  "dependencies": {
    "@langchain/langgraph-sdk": "1.6.0"
  }
}
```

Run:

```bash
cd backend/api/handler/coze/testdata/workbench_sdk_compat/js
pnpm install --lockfile-only
pnpm install --frozen-lockfile
```

Expected: lock resolves exactly `@langchain/langgraph-sdk@1.6.0` with integrity `sha512-J/B1SkCG0U+eXEXH/X89dDHxP8I0eULjLtXYvZ39uk2TxEKjLsrW4LY5J7Qwrf0GCDA+IM/agjKSLXALnctWTw==`.

- [ ] **Step 2: Write the JS black-box contract script**

使用 SDK `Client` 和 `onRequest` hook 注入测试 session、space 和唯一 `Idempotency-Key`。脚本必须执行并断言：create empty Thread、create Run、get/list Run、stream `messages-tuple+updates`、断线后根据 Location/Last-Event-ID 重连、join 返回 values、wait 返回 values、cancel 返回 void/无 body、updateState 读取 `configurable`、unsupported field 返回 `422`。

成功只输出：

```json
{"language":"javascript","sdk_version":"1.6.0","status":"passed"}
```

- [ ] **Step 3: Wire the JS child process into the Go harness**

Go test 使用 `exec.CommandContext`，设置固定 timeout，工作目录为 JS fixture，执行 `node compat.mjs`，解析唯一 summary 并精确断言 language/version/status。

- [ ] **Step 4: Run the JS compatibility test and verify GREEN**

```bash
cd backend
WORKBENCH_SDK_COMPAT=1 WORKBENCH_SDK_COMPAT_LANGUAGE=javascript GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run '^TestWorkbenchCanonicalSDKCompatibility$' -count=1 -v
```

Expected: PASS and summary reports JavaScript SDK `1.6.0`.

- [ ] **Step 5: Commit JS harness**

```bash
git add api/handler/coze/testdata/workbench_sdk_compat/js api/handler/coze/workbench_canonical_sdk_compat_test.go
git commit -m "test: verify JavaScript LangGraph SDK compatibility"
```

### Task 10: Add Locked Python SDK 0.4.2 Compatibility Clients

**Files:**
- Create: `backend/api/handler/coze/testdata/workbench_sdk_compat/python/pyproject.toml`
- Create: `backend/api/handler/coze/testdata/workbench_sdk_compat/python/uv.lock`
- Create: `backend/api/handler/coze/testdata/workbench_sdk_compat/python/compat.py`
- Modify: `backend/api/handler/coze/workbench_canonical_sdk_compat_test.go`

- [ ] **Step 1: Pin the Python package and generate a frozen lock**

```toml
[project]
name = "workbench-canonical-sdk-compat-python"
version = "0.0.0"
requires-python = ">=3.11"
dependencies = ["langgraph-sdk==0.4.2"]
```

Run:

```bash
cd backend/api/handler/coze/testdata/workbench_sdk_compat/python
uv lock
uv sync --frozen
```

Expected: lock resolves exactly `langgraph-sdk==0.4.2`; wheel hash includes `sha256:75fa5096c1177ce39c847096a8fe3745ffd480ddb412995f836e9f5f884c43dd`.

- [ ] **Step 2: Write sync and async Python black-box tests**

`compat.py` 同时使用固定 SDK 的 sync 与 async clients，执行 create/get/search、create/get/list Run、stream/reconnect、join、wait、cancel、get/update state。必须覆盖：sync wait 显式发送 `raise_error`；async wait 省略该字段；failed wait 的 HTTP-error/values-error 两种投影；Python `update_state` 读取 `checkpoint`。

成功只输出：

```json
{"language":"python","sdk_version":"0.4.2","status":"passed","clients":["sync","async"]}
```

- [ ] **Step 3: Wire Python into the Go harness**

Go test 按 `WORKBENCH_SDK_COMPAT_LANGUAGE=python|all` 选择执行 `uv run --frozen python compat.py`，解析 summary，并对 version 与 clients 做精确断言。

- [ ] **Step 4: Run Python and combined SDK compatibility tests**

```bash
cd backend
WORKBENCH_SDK_COMPAT=1 WORKBENCH_SDK_COMPAT_LANGUAGE=python GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run '^TestWorkbenchCanonicalSDKCompatibility$' -count=1 -v
```

```bash
cd backend
WORKBENCH_SDK_COMPAT=1 WORKBENCH_SDK_COMPAT_LANGUAGE=all GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run '^TestWorkbenchCanonicalSDKCompatibility$' -count=1 -v
```

Expected: both PASS; summaries report JS `1.6.0` and Python `0.4.2` sync+async.

- [ ] **Step 5: Commit Python harness**

```bash
git add api/handler/coze/testdata/workbench_sdk_compat/python api/handler/coze/workbench_canonical_sdk_compat_test.go
git commit -m "test: verify Python LangGraph SDK compatibility"
```

### Task 11: Add Reproducible Verification Script And CI Job

**Files:**
- Create: `backend/scripts/verify_workbench_sdk_compat.sh`
- Modify: `.github/workflows/ci@backend.yml`

- [ ] **Step 1: Write a failing shell verification contract**

脚本必须 `set -euo pipefail`，校验 repo root、Node `>=21`、pnpm、uv、Go；分别执行 `pnpm install --frozen-lockfile` 和 `uv sync --frozen`；读取安装后的 package metadata 精确校验版本；执行 combined Go compatibility test。版本或 lock 漂移必须非零退出。

- [ ] **Step 2: Implement the script**

核心命令固定为：

```bash
pnpm --dir "$js_dir" install --frozen-lockfile
uv sync --project "$python_dir" --frozen
WORKBENCH_SDK_COMPAT=1 WORKBENCH_SDK_COMPAT_LANGUAGE=all \
  GOCACHE="${GOCACHE:-/private/tmp/coze-workbench-canonical-go-cache}" \
  go test -p 1 -gcflags="all=-l -N" ./api/handler/coze \
  -run '^TestWorkbenchCanonicalSDKCompatibility$' -count=1 -v
```

不得使用本机 `/private/tmp` 审计解包目录作为依赖来源；CI 只信任仓库 lockfile 与 registry integrity/hash。

- [ ] **Step 3: Add a dedicated CI job**

在 `.github/workflows/ci@backend.yml` 增加独立 `workbench-sdk-compat` job，checkout 后安装 Go、Node >=21、pnpm 8.15.8、Python >=3.11、uv，运行脚本。path filter 至少包含：

```yaml
- 'idl/**'
- 'backend/api/handler/coze/workbench_canonical_*'
- 'backend/api/router/coze/**'
- 'backend/scripts/verify_workbench_sdk_compat.sh'
- 'frontend/packages/arch/api-schema/**'
```

该 job 不依赖生产 API key、Redis 或网关；使用 in-process server 和 test-only session fixture。

- [ ] **Step 4: Run local reproducibility checks**

```bash
cd backend
bash scripts/verify_workbench_sdk_compat.sh
```

Expected: JS `1.6.0` and Python `0.4.2` compatibility PASS.

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Commit CI verification**

```bash
git add backend/scripts/verify_workbench_sdk_compat.sh .github/workflows/ci@backend.yml
git commit -m "ci: verify canonical Workbench SDK contract"
```

### Task 12: Document Phase Status And Run The No-Impact Gate

**Files:**
- Modify: `docs/superpowers/context/project-context.md`
- Create: `docs/superpowers/runbooks/workbench-canonical-thread-api.md`
- Modify: `docs/superpowers/specs/2026-07-26-workbench-thread-api-contract-design.md`
- Modify: `docs/superpowers/plans/2026-07-26-workbench-canonical-thread-api-core.md`

- [ ] **Step 1: Update long-term facts without overstating readiness**

记录：canonical core 已实现但默认关闭；当前 UI 仍使用 `TaskThreadV1Client`；外部 production allowlist 尚未开放；当前 `/api/threads` 与 `/api/workbench/task_threads` 仍是来源合同；ChatTask 仍退役。明确后续顺序：生产 principal/scope/key -> 分布式限流与容量 -> 网关 SSE -> UI 双 client 联调 -> 灰度 -> 观察期 -> 分别删除旧接口。

- [ ] **Step 2: Add an operator/developer runbook**

runbook 必须包含：本地显式启用方式、session 前提、route 表、curl 安全示例、SDK test 命令、trace/log event 查询、SSE cursor/header 排障、禁记字段、关闭开关回滚方式和“禁止在生产开放”的醒目标记。不得提供尚未实现的 x-api-key 示例。

- [ ] **Step 3: Mark implementation status in the contract spec**

只更新“实施状态/阶段”区块：Phase 0A core implementation 的真实完成项与未完成项分别列出。不得把 API key、scope、rate limit、gateway、UI migration 或旧接口删除标为完成。

- [ ] **Step 4: Run generated-code and focused backend verification**

```bash
cd backend
bash scripts/verify_api_codegen.sh
```

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze -count=1
```

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-canonical-go-cache go test -p 1 -gcflags="all=-l -N" \
  ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository -count=1
```

```bash
cd frontend
node common/scripts/install-run-rush.js test --to @coze-studio/api-schema -- --run workbench-thread-contract
```

```bash
cd backend
bash scripts/verify_workbench_sdk_compat.sh
```

Expected: all PASS.

- [ ] **Step 5: Run source-contract and retirement scans**

```bash
rg -n 'api/workbench/(tasks|chat)|WorkbenchChat\(|ChatTask|backend/(application|domain)/task' backend idl frontend/packages/arch/api-schema
```

Expected: no revived ChatTask route/IDL/client/application/domain symbol; Runtime Doctor service shell and denylist-only historical metadata cleanup are reviewed as allowed evidence, not treated as fallback.

```bash
git diff --name-only eae28c1c04edb4edc9697b28f25b809af5967215...HEAD
git diff --check eae28c1c04edb4edc9697b28f25b809af5967215...HEAD
```

Expected: only planned IDL/generated schema、canonical handler/tests、additive
application/domain/repository contracts、CI script/workflow 和当前 docs 发生变化；没有 migration/schema、
worker/runtime、当前 handler 或 UI runtime 文件改动。shared-layer diff 中现有方法的变化仅限复用
helper 或 canonical optional fields，Step 1 characterization tests 必须证明旧零值分支不变。

- [ ] **Step 6: Use codebase-memory to verify impact**

运行 `detect_changes`（若当前 MCP 版本未提供，则以 `search_graph`/`trace_path` 检查新 handlers）并核对：所有 canonical handler 只进入 `agentthread.ApplicationService`；没有第二套 storage/runtime；旧 handler 的 inbound/outbound calls 未改变。图谱结论必须回到真实 diff 和源码复核。

- [ ] **Step 7: Commit docs and final verification evidence**

```bash
git add docs/superpowers/context/project-context.md docs/superpowers/runbooks/workbench-canonical-thread-api.md docs/superpowers/specs/2026-07-26-workbench-thread-api-contract-design.md docs/superpowers/plans/2026-07-26-workbench-canonical-thread-api-core.md
git commit -m "docs: record canonical Workbench API phase"
```

### Task 13: First Integration Audit Only

**Files:**
- Read: `docs/superpowers/runbooks/dev-integration-audit.md`
- Modify: none unless audit reveals an in-scope defect

- [ ] **Step 1: Confirm branch and upstream facts**

```bash
git status --short
git branch --show-current
git rev-parse HEAD
git rev-parse dev
git rev-parse origin/dev
git merge-base HEAD origin/dev
```

Expected: clean `codex/workbench-canonical-thread-api`; branch contains local `dev@eae28c1...`; no remote push occurs.

- [ ] **Step 2: Review the full diff for scope and sensitive data**

```bash
git diff --stat eae28c1c04edb4edc9697b28f25b809af5967215...HEAD
git diff --check eae28c1c04edb4edc9697b28f25b809af5967215...HEAD
git log --oneline --decorate eae28c1c04edb4edc9697b28f25b809af5967215..HEAD
```

逐文件确认无 credential/token/Cookie、无 request/response payload dump、无旧 route 语义变化、
无数据库 migration/schema、worker/runtime 或 UI runtime 改动；repository 代码只包含同表上的
additive query/transaction methods。

- [ ] **Step 3: Re-run the fresh verification matrix**

重新执行 Task 12 的 Go、TypeScript、codegen、real SDK 和 retirement tests；审计报告只引用本轮新鲜输出。

- [ ] **Step 4: Present the first audit report and stop before merge**

报告包含 branch、HEAD SHA、base SHA、文件范围、验证命令/结果、未完成生产能力和剩余风险。未经用户第一次明确确认，不合入本地 `dev`；未经第二次审计与第二次明确确认，不推送 `origin/dev`。

## Deferred Production Batches

以下内容不在本计划中，必须各自形成独立设计复核与实施计划：

1. **Production principal and authorization:** session/Bearer/x-api-key 单一 principal 解析、ambiguous credential 拒绝、API key schema/rotation/revocation、scope、空间授权、CSRF 边界。
2. **Rate limit and capacity isolation:** principal/space/IP/active Run/SSE/token buckets、Redis/fail-closed、Workbench 保留容量、外部 admission 与压测。
3. **Gateway SSE:** 独立 location、禁缓冲/缓存/压缩、header 透传、首帧/长连接/断连/重连代理测试。
4. **Workbench UI migration:** `CanonicalThreadClient`、TaskThread view-model adapter、功能等价 fixture、单写、读 shadow、feature flag、真实页面联调和回滚。
5. **Product extensions:** uploads、artifacts、token usage、memories、audits、suggestions、top-level retry 与各自权限/日志/限流。
6. **Generic idempotency registry:** Thread/deferred/non-Run write 的 principal+space+operation+fingerprint exactly-once 记录与冲突合同。
7. **Source route retirement:** 在 UI/外部调用方迁移、观察期和流量证据满足后，分别删除 `/api/workbench/task_threads` 与 `/api/threads`；ChatTask 保持已退役，不参与迁移。
