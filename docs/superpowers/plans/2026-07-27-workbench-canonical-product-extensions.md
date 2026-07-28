# Workbench Canonical Product Extensions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变 `/api/workbench/task_threads`、`/api/threads`、当前默认 UI 和 Agent 执行链的前提下，补齐 `/api/workbench/threads` 的 26 个产品扩展，并完成当前仍为 `501` 的两条 canonical SSE 核心路由。

**Architecture:** `thread.thrift` 继续是单一 canonical service，新增 `thread_product.thrift` 承载产品 DTO，二者均不依赖 `task.thrift`。所有持久化操作直接调用现有 `agentthread.ApplicationService`；建议问题是唯一的只读旁路，完成 Thread 授权后调用现有 `application/workbench.GenerateSuggestions`。Handler 只做门控、认证、严格校验、安全投影、SSE 和结构化日志，不调用旧 Hertz handler，不新增表、状态机、worker 分支或双写。

**Tech Stack:** Go 1.24、CloudWeGo Hertz、Thrift/Hertz codegen、Eino ADK 既有运行链、React api-schema、Vitest、Go `testing`/Testify、SSE

---

## Baseline And Safety Boundary

- 基线固定为 `dev@50afc8beff2273cef5a076478e4fcf0df2004425`，执行分支为 `codex/workbench-canonical-product-client`。
- 当前 `StreamCanonicalRun` 与 `ReconnectCanonicalRunStream` 位于 `workbench_canonical_entrypoints.go`，打开 gate 后返回 `501 canonical_not_implemented`。前端 canonical 模式开始前必须先补齐这两条路由。
- `/api/workbench/task_threads` 的 36 条来源路由和 `/api/threads` 的 23 条来源路由必须保持 method/path、请求、响应、SSE 和日志语义不变。
- `/api/workbench/tasks*` 与 `/api/workbench/chat` 必须继续 `404`，且不能进入 handler chain。
- `COZE_WORKBENCH_CANONICAL_API_ENABLED` 仍默认关闭；关闭时新增路由统一 `404`。
- 不修改数据库 migration、Eino ADK、Run/Thread 状态机、worker、checkpoint bytes、对象存储布局或来源接口。
- 不开放 API key、Bearer scope、外部限流或生产网关；本计划仍只使用现有 Web session principal。
- canonical body 不接受 `space_id`、`user_id`、owner；目标空间只从 `X-Coze-Space-ID` header 读取并由服务端授权。
- 成功响应无 `{code,msg,data}` envelope；纯删除返回 `204`；所有 ID 输出十进制字符串；时间输出 RFC 3339。
- 日志不得包含正文、Memory 内容、signed URL、credential、tool 参数/结果、provider body、原始 usage 或对象存储路径。

## Product Route Matrix

| Method | Route | Handler |
| --- | --- | --- |
| `POST` | `/api/workbench/threads/:thread_id/messages` | `AppendCanonicalThreadMessage` |
| `POST` | `/api/workbench/threads/:thread_id/suggestions` | `GenerateCanonicalThreadSuggestions` |
| `GET` | `/api/workbench/threads/:thread_id/uploads` | `ListCanonicalThreadUploads` |
| `POST` | `/api/workbench/threads/:thread_id/uploads` | `UploadCanonicalThreadFiles` |
| `DELETE` | `/api/workbench/threads/:thread_id/uploads/:file_id` | `DeleteCanonicalThreadUpload` |
| `GET` | `/api/workbench/threads/:thread_id/artifacts` | `ListCanonicalThreadArtifacts` |
| `GET` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id/content` | `GetCanonicalThreadArtifactContent` |
| `GET` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url` | `GetCanonicalThreadArtifactSignedURL` |
| `DELETE` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id` | `DeleteCanonicalThreadArtifact` |
| `POST` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore` | `RestoreCanonicalThreadArtifact` |
| `POST` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review` | `ReviewCanonicalThreadArtifactScan` |
| `GET` | `/api/workbench/threads/:thread_id/artifact_scan_jobs` | `ListCanonicalThreadArtifactScanJobs` |
| `POST` | `/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry` | `RetryCanonicalThreadArtifactScanJob` |
| `GET` | `/api/workbench/threads/:thread_id/token_usage` | `GetCanonicalThreadTokenUsage` |
| `GET` | `/api/workbench/threads/:thread_id/memories` | `ListCanonicalThreadMemories` |
| `PUT` | `/api/workbench/threads/:thread_id/memories/:memory_id` | `UpdateCanonicalThreadMemory` |
| `DELETE` | `/api/workbench/threads/:thread_id/memories/:memory_id` | `DeleteCanonicalThreadMemory` |
| `POST` | `/api/workbench/threads/:thread_id/memories/:memory_id/restore` | `RestoreCanonicalThreadMemory` |
| `POST` | `/api/workbench/threads/:thread_id/memories/clear` | `ClearCanonicalThreadMemories` |
| `GET` | `/api/workbench/threads/:thread_id/memories/export` | `ExportCanonicalThreadMemories` |
| `POST` | `/api/workbench/threads/:thread_id/memories/import` | `ImportCanonicalThreadMemories` |
| `GET` | `/api/workbench/threads/:thread_id/memories/audit_events` | `ListCanonicalThreadMemoryAuditEvents` |
| `GET` | `/api/workbench/threads/:thread_id/guardrail_audit_events` | `ListCanonicalThreadGuardrailAuditEvents` |
| `GET` | `/api/workbench/threads/:thread_id/guardrail_audit_events/export` | `ExportCanonicalThreadGuardrailAuditEvents` |
| `GET` | `/api/workbench/threads/:thread_id/mcp_runtime_audit_events` | `ListCanonicalThreadMCPRuntimeAuditEvents` |
| `POST` | `/api/workbench/threads/:thread_id/runs/:run_id/retry` | `RetryCanonicalSubagentRun` |

## File Structure

| File | Responsibility |
| --- | --- |
| `idl/workbench/thread_product.thrift` | Product request/response DTOs only; no service and no `task.thrift` include |
| `idl/workbench/thread.thrift` | One canonical service, 21 core methods plus 26 product methods, explicit space header mapping |
| `backend/api/handler/coze/workbench_canonical_run_stream.go` | Create-stream/reconnect-stream implementation and canonical disconnect policy |
| `backend/api/handler/coze/workbench_canonical_product_contract.go` | Product pagination, content type, resource IDs, size bounds and safe log fields |
| `backend/api/handler/coze/workbench_canonical_product_projection.go` | Upload, Artifact, scan, usage, Memory and Audit public projections |
| `backend/api/handler/coze/workbench_canonical_message_product_service.go` | Internal append and best-effort suggestions |
| `backend/api/handler/coze/workbench_canonical_upload_service.go` | List/upload/delete by stable file ID |
| `backend/api/handler/coze/workbench_canonical_artifact_service.go` | Artifact, content, signed URL and scan operations |
| `backend/api/handler/coze/workbench_canonical_memory_audit_service.go` | Memory CRUD/import/export and audit reads |
| `backend/api/handler/coze/workbench_canonical_usage_retry_service.go` | Token usage and subagent retry |
| `backend/application/agentthread/upload_file.go` | Additive delete-by-file-ID application orchestration only |
| `backend/api/router/coze/workbench_canonical_thread_route_test.go` | Exact canonical/current/retired route snapshots |
| `frontend/packages/arch/api-schema/src/idl/workbench/thread_product.ts` | Generated product DTOs |
| `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts` | Generated single canonical client |
| `docs/superpowers/runbooks/workbench-canonical-product-client-validation.md` | Gate, route, safe curl, SSE and local UI validation |

### Task 1: Freeze The 47-Route Canonical Surface

**Files:**
- Modify: `backend/api/router/coze/workbench_canonical_thread_route_test.go`
- Modify: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`

- [ ] **Step 1: Write the failing product route snapshot**

Append the exact 26-route matrix to a separate slice, then assert the entire canonical prefix exactly equals core plus product routes:

```go
var canonicalProductRoutes = []routeExpectation{
	{http.MethodPost, "/api/workbench/threads/:thread_id/messages"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/suggestions"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/uploads"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/uploads"},
	{http.MethodDelete, "/api/workbench/threads/:thread_id/uploads/:file_id"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/artifacts"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/artifacts/:artifact_id/content"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url"},
	{http.MethodDelete, "/api/workbench/threads/:thread_id/artifacts/:artifact_id"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/artifact_scan_jobs"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/token_usage"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/memories"},
	{http.MethodPut, "/api/workbench/threads/:thread_id/memories/:memory_id"},
	{http.MethodDelete, "/api/workbench/threads/:thread_id/memories/:memory_id"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/memories/:memory_id/restore"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/memories/clear"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/memories/export"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/memories/import"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/memories/audit_events"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/guardrail_audit_events"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/guardrail_audit_events/export"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/mcp_runtime_audit_events"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/:run_id/retry"},
}

func canonicalRouteSnapshot() []routeExpectation {
	result := append([]routeExpectation{}, canonicalThreadRoutes...)
	return append(result, canonicalProductRoutes...)
}
```

Replace the positive canonical assertion with:

```go
requireExactRouteSnapshot(t, h, "/api/workbench/threads", canonicalRouteSnapshot())
```

Keep both source snapshots and all retired ChatTask probes byte-for-byte unchanged.

- [ ] **Step 2: Extend the generated TypeScript contract test**

Freeze the 26 exported method names, the stable-ID upload route, and the absence of a `task` import:

```ts
const productMethods = [
  'AppendCanonicalThreadMessage',
  'GenerateCanonicalThreadSuggestions',
  'ListCanonicalThreadUploads',
  'UploadCanonicalThreadFiles',
  'DeleteCanonicalThreadUpload',
  'ListCanonicalThreadArtifacts',
  'GetCanonicalThreadArtifactContent',
  'GetCanonicalThreadArtifactSignedURL',
  'DeleteCanonicalThreadArtifact',
  'RestoreCanonicalThreadArtifact',
  'ReviewCanonicalThreadArtifactScan',
  'ListCanonicalThreadArtifactScanJobs',
  'RetryCanonicalThreadArtifactScanJob',
  'GetCanonicalThreadTokenUsage',
  'ListCanonicalThreadMemories',
  'UpdateCanonicalThreadMemory',
  'DeleteCanonicalThreadMemory',
  'RestoreCanonicalThreadMemory',
  'ClearCanonicalThreadMemories',
  'ExportCanonicalThreadMemories',
  'ImportCanonicalThreadMemories',
  'ListCanonicalThreadMemoryAuditEvents',
  'ListCanonicalThreadGuardrailAuditEvents',
  'ExportCanonicalThreadGuardrailAuditEvents',
  'ListCanonicalThreadMCPRuntimeAuditEvents',
  'RetryCanonicalSubagentRun',
] as const;

for (const method of productMethods) {
  expect(api[method]).toBeTypeOf('function');
}
expect(generatedSource).toContain('/uploads/:file_id');
expect(generatedSource).not.toContain('workbench/task');
```

- [ ] **Step 3: Run both tests and verify RED**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/router/coze -run '^TestWorkbenchCanonicalThreadRoutes$' -count=1
```

Expected: FAIL because the 26 product routes are absent; source snapshots and ChatTask probes pass.

Run:

```bash
cd frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
```

Expected: FAIL because product methods and `thread_product.ts` do not exist.

- [ ] **Step 4: Commit the failing contract tests**

```bash
git add backend/api/router/coze/workbench_canonical_thread_route_test.go frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts
git commit -m "test: freeze canonical workbench product routes"
```

### Task 2: Add Product IDL And Regenerate One Canonical Client

**Files:**
- Create: `idl/workbench/thread_product.thrift`
- Modify: `idl/workbench/thread.thrift`
- Generated: `backend/api/model/workbench/thread_product_contract/thread_product.go`
- Generated: `backend/api/model/workbench/thread_contract/thread.go`
- Generated: `backend/api/model/workbench/chat/workbench.go`
- Generated: `backend/api/model/coze/api.go`
- Generated: `backend/api/router/coze/api.go`
- Generated: `backend/api/router/coze/middleware.go`
- Modify: `backend/api/handler/coze/workbench_canonical_entrypoints.go`
- Generated: `frontend/packages/arch/api-schema/src/idl/workbench/thread_product.ts`
- Generated: `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts`
- Modify: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`
- Modify: `.github/scripts/check-file-size.sh`

- [ ] **Step 1: Define transport-independent product DTOs**

Create `thread_product.thrift` with this namespace and common request convention:

```thrift
namespace go workbench.thread_product_contract

include "../base.thrift"

struct CanonicalProductEmptyResponse {}

struct CanonicalProductThreadRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalProductPageRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i32 limit (api.query="limit")
    4: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}
```

Define the resource structs with the exact public fields below. ID fields are `string`; all `*_at` fields are RFC 3339 `string`; JSON metadata fields use `(api.value_type="any")`:

| Struct | Fields |
| --- | --- |
| `CanonicalUploadFile` | `file_id,file_name,virtual_path,content_type,size_bytes,created_at` |
| `CanonicalArtifact` | `artifact_id,thread_id,run_id,file_id,title,artifact_type,virtual_path,content_type,size_bytes,preview_mode,metadata,created_at,updated_at,deleted_at?` |
| `CanonicalArtifactScanJob` | `job_id,thread_id,run_id,artifact_id,file_id,scanner,status,worker_ref,attempt_count,error_code,available_at?,started_at?,ended_at?,created_at,updated_at` |
| `CanonicalTokenUsage` | `usage_id,thread_id,run_id,source,step_id,step_index,step_name,model_name,provider,input_tokens,output_tokens,total_tokens,cost_micros,currency,estimated,created_at` |
| `CanonicalTokenUsageAggregate` | `input_tokens,output_tokens,total_tokens,cost_micros,call_count,lead_agent_tokens,subagent_tokens,middleware_tokens,tool_tokens` |
| `CanonicalRunTokenUsageAggregate` | `run_id,aggregate` |
| `CanonicalMemory` | `memory_id,thread_id,run_id?,scope,content,metadata,score,confidence,source_type,source_id,correction_of_memory_id?,corrected_at?,expires_at?,created_at,updated_at,deleted_at?` |
| `CanonicalMemoryAuditEvent` | `event_id,thread_id,run_id?,memory_id?,actor_id?,event_type,scope,source_type,source_id,affected_count,created_at` |
| `CanonicalGuardrailAuditEvent` | `event_id,thread_id,run_id?,actor_id?,event_type,target_type,target_id,operation,source,action,fail_mode,provider,reason_code,rule_ids,created_at` |
| `CanonicalMCPRuntimeAuditEvent` | `event_id,thread_id,run_id?,server_id?,runtime_tool_name,event_type,error_code,elapsed_millis,output_bytes,created_at` |

Use route-specific list responses. Each list response has its named array, `total`, `has_more`, and optional `next_cursor`; do not introduce a generic JSON envelope:

```thrift
struct CanonicalUploadListResponse {
    1: required list<CanonicalUploadFile> uploads
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
}

struct CanonicalArtifactListResponse {
    1: required list<CanonicalArtifact> artifacts
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
}
```

Apply that shape to scan jobs, memories, memory audit events, guardrail audit events and MCP runtime audit events. Token usage returns `usage`, `total`, `has_more`, optional `next_cursor`, `aggregate` and `run_aggregates`.

Define request fields exactly as follows:

| Request | Path/header plus body/query fields |
| --- | --- |
| `AppendCanonicalThreadMessageRequest` | `run_id`, `role`, `content`, `metadata`, `append_mode` body |
| `GenerateCanonicalThreadSuggestionsRequest` | `n`, `model_name`, `model_type` body; no client message body |
| `UploadCanonicalThreadFilesRequest` | Thread path and space header only; multipart is read manually |
| `DeleteCanonicalThreadUploadRequest` | `file_id` path |
| `ListCanonicalThreadArtifactsRequest` | `run_id`, `deleted_only`, `limit`, `offset` query |
| `CanonicalArtifactRouteRequest` | `artifact_id` path |
| `GetCanonicalThreadArtifactContentRequest` | `artifact_id` path, `mode` query |
| `GetCanonicalThreadArtifactSignedURLRequest` | `artifact_id` path, `mode`, `ttl_seconds` query |
| `ReviewCanonicalThreadArtifactScanRequest` | `artifact_id` path, `decision`, `reason` body |
| `ListCanonicalThreadArtifactScanJobsRequest` | `run_id`, `artifact_id`, `status`, `scanner`, `limit`, `offset` query |
| `RetryCanonicalThreadArtifactScanJobRequest` | `job_id` path |
| `GetCanonicalThreadTokenUsageRequest` | `run_id`, `include_child_runs`, `source`, `limit`, `offset` query |
| `ListCanonicalThreadMemoriesRequest` | `run_id`, `scope`, `scopes`, `q`, `include_expired`, `include_deleted`, `limit`, `offset` query |
| `UpdateCanonicalThreadMemoryRequest` | `memory_id` path; current editable Memory fields in body |
| `CanonicalMemoryRouteRequest` | `memory_id` path |
| `ClearCanonicalThreadMemoriesRequest` | `run_id`, `scopes` body |
| `ExportCanonicalThreadMemoriesRequest` | current list filters plus `limit` query |
| `ImportCanonicalThreadMemoriesRequest` | bounded `memories` body |
| `ListCanonicalThreadMemoryAuditEventsRequest` | `memory_id`, `limit`, `offset` query |
| `ListCanonicalThreadGuardrailAuditEventsRequest` | `run_id`, `limit`, `offset` query |
| `ExportCanonicalThreadGuardrailAuditEventsRequest` | `run_id`, `limit`, `offset` query |
| `ListCanonicalThreadMCPRuntimeAuditEventsRequest` | `run_id`, `limit`, `offset` query |
| `RetryCanonicalSubagentRunRequest` | `run_id` path and `Idempotency-Key` header |

The upload response contains `uploads` and `skipped_files`; suggestion response contains only `suggestions`; signed URL response contains `artifact_id,url,expires_in_seconds,content_type,preview_mode`; scan review contains `artifact_id,decision,scan_status,reviewed`; clear/import/export responses expose their current operation summary without an envelope.

- [ ] **Step 2: Add explicit workspace header mappings to core requests**

Before changing the IDL, extend `workbench-thread-contract.test.ts` so every core and
product request config expects `X-Coze-Space-ID` in `reqMapping.header`; preserve
`Idempotency-Key`, `Last-Event-ID` and `Prefer` alongside it. Run the focused Vitest and
observe a RED failure because the generated core requests do not yet expose the workspace
header.

In `thread.thrift`, add this field to every core request struct, including `CanonicalRouteRequest` and `CanonicalRunRouteRequest`:

```thrift
required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
```

Allocate the next unused field number in each struct. This is an IDL/client correction only; handlers continue to read and authorize the same header, and no body gains `space_id`.

Define the core `CanonicalMessage` DTO from the existing public projection with required
`message_id,thread_id,run_id,role,content,metadata,created_at` and optional `seq`. IDs and time
are strings; metadata uses `(api.value_type="any")`. Change `CanonicalMessagePage.data` from
untyped JSON to `list<CanonicalMessage>`. This is a generated type correction only: the existing
handler already returns this exact JSON shape. Extend the TypeScript contract test to freeze these
fields and the typed page.

- [ ] **Step 3: Extend the existing canonical service**

Add `include "./thread_product.thrift"` and all 26 methods to `WorkbenchCanonicalThreadService`. The service declarations must use the exact method and route names from the Product Route Matrix. Representative declarations:

```thrift
thread_product.CanonicalSuggestionResponse GenerateCanonicalThreadSuggestions(
    1: thread_product.GenerateCanonicalThreadSuggestionsRequest req
) (api.post="/api/workbench/threads/:thread_id/suggestions")

thread_product.CanonicalUploadListResponse ListCanonicalThreadUploads(
    1: thread_product.CanonicalProductThreadRequest req
) (api.get="/api/workbench/threads/:thread_id/uploads")

thread_product.CanonicalProductEmptyResponse DeleteCanonicalThreadUpload(
    1: thread_product.DeleteCanonicalThreadUploadRequest req
) (api.delete="/api/workbench/threads/:thread_id/uploads/:file_id")

CanonicalRun RetryCanonicalSubagentRun(
    1: thread_product.RetryCanonicalSubagentRunRequest req
) (api.post="/api/workbench/threads/:thread_id/runs/:run_id/retry")
```

`thread.thrift` must not include `task.thrift`, and no product response may reuse a TaskThread DTO.

Add one exported gate-only entrypoint for each new product method to
`workbench_canonical_entrypoints.go`. Each entrypoint must call
`serveCanonicalEntrypoint` and do nothing else. This keeps the generated router compilable and
preserves the established default-off behavior: gate disabled returns `404`; gate enabled returns
`501 canonical_not_implemented` until Tasks 5-9 replace each stub with its real handler. Do not
accept the generic Hertz generated handler body, return `200`, or call a TaskThread handler.

- [ ] **Step 4: Run the fixed generators**

Run the repository's exact backend generator command from the prior core plan:

```bash
cd backend
hz --version
thriftgo --version
hz update -idl ../idl/api.thrift -enable_extends --exclude_file api/handler/coze/agent_run_service.go --exclude_file api/handler/coze/announcement_service.go --exclude_file api/handler/coze/app_dev_service.go --exclude_file api/handler/coze/bot_open_api_service.go --exclude_file api/handler/coze/config_service.go --exclude_file api/handler/coze/conversation_service.go --exclude_file api/handler/coze/database_service.go --exclude_file api/handler/coze/developer_api_service.go --exclude_file api/handler/coze/intelligence_service.go --exclude_file api/handler/coze/knowledge_service.go --exclude_file api/handler/coze/memory_service.go --exclude_file api/handler/coze/message_service.go --exclude_file api/handler/coze/open_apiauth_service.go --exclude_file api/handler/coze/passport_service.go --exclude_file api/handler/coze/playground_service.go --exclude_file api/handler/coze/plugin_develop_service.go --exclude_file api/handler/coze/public_product_service.go --exclude_file api/handler/coze/resource_service.go --exclude_file api/handler/coze/upload_service.go --exclude_file api/handler/coze/workbench_skill_service.go --exclude_file api/handler/coze/workflow_service.go
```

Expected versions: `hz version v0.9.7` and `thriftgo 0.4.5`. Restore `_adminMw()` in generated `backend/api/router/coze/middleware.go` to `adminAuthMiddlewareFactory()`, add license headers to newly generated files, run `gofmt`, and add only `backend/api/model/workbench/thread_product_contract/thread_product.go` to the generated-file allowlist in `.github/scripts/check-file-size.sh`.

Run:

```bash
cd backend
bash scripts/verify_api_codegen.sh
```

Expected: `WORKSPACE_READ_ONLY_SHA256=PASS` and `CODEGEN_DETERMINISTIC_SHA256=PASS`.

Run:

```bash
cd frontend/packages/arch/api-schema
rushx update
rushx test src/__tests__/workbench-thread-contract.test.ts
```

Expected: generated `thread_product.ts` exists, all 47 methods are exported from `thread.ts`, every request exposes the `X-Coze-Space-ID` header mapping, and the contract test passes.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/router/coze -run '^TestWorkbenchCanonicalThreadRoutes$' -count=1
```

Expected: PASS with exactly 47 canonical routes; both source snapshots and retired ChatTask probes remain unchanged.

- [ ] **Step 5: Commit IDL and generated code**

```bash
git add idl/workbench/thread.thrift idl/workbench/thread_product.thrift backend/api/model backend/api/router/coze backend/api/handler/coze/workbench_canonical_entrypoints.go frontend/packages/arch/api-schema .github/scripts/check-file-size.sh
git commit -m "feat: define canonical workbench product contract"
```

### Task 3: Complete Canonical Create And Reconnect SSE

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_run_stream.go`
- Modify: `backend/api/handler/coze/workbench_canonical_entrypoints.go`
- Create: `backend/api/handler/coze/workbench_canonical_run_stream_test.go`

- [ ] **Step 1: Write failing SSE tests**

Cover these named cases with the existing authenticated canonical test server and a recording `runEventStreamWriter`:

```go
func TestStreamCanonicalRunCreatesOneRunAndStreamsPersistedEvents(t *testing.T)
func TestStreamCanonicalRunReplaysIdempotentRunWithoutSecondMessage(t *testing.T)
func TestReconnectCanonicalRunStreamReplaysAfterEventIDBeforeLiveEvents(t *testing.T)
func TestReconnectCanonicalRunStreamUsesLastEventIDHeader(t *testing.T)
func TestCanonicalRunStreamContinueDoesNotCancelOnDisconnect(t *testing.T)
func TestCanonicalRunStreamCancelCancelsOnlyAfterConfirmedDisconnect(t *testing.T)
func TestCanonicalRunStreamWritesMetadataEventAndOneTerminalEnd(t *testing.T)
func TestCanonicalRunStreamProjectsErrorsWithoutInternalDetails(t *testing.T)
func TestCanonicalRunStreamMessagesTupleUsesMessagesEvents(t *testing.T)
func TestReconnectCanonicalRunStreamCancelOnDisconnectOverridesPersistedContinue(t *testing.T)
func TestReconnectCanonicalRunStreamFalseCancelEncodingOverridesPersistedCancel(t *testing.T)
```

Each test must assert the Run belongs to the path Thread, persisted events are ordered by `event_id`, `Content-Location` points to the canonical Run, and no provider/tool payload appears in emitted data.
Create and reconnect success responses must also expose the exact API-base-relative `Location` for the existing Run stream.

- [ ] **Step 2: Verify the current `501` failure**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'Test(Stream|Reconnect)CanonicalRun' -count=1
```

Expected: FAIL because both exported handlers still call `serveCanonicalEntrypoint`.

- [ ] **Step 3: Implement create-stream without handler-to-handler calls**

Move the two exported functions out of `workbench_canonical_entrypoints.go`. `StreamCanonicalRun` must perform this sequence directly:

```go
requestLog := beginCanonicalRequestLog("run.stream.create", "/api/workbench/threads/:thread_id/runs/stream")
defer completeCanonicalRequestLog(ctx, c, requestLog)

// gate -> initialized service -> path IDs -> space authorization -> strict body
// -> parseCanonicalRunSubmission -> createCanonicalRunBundle -> public projections
// -> Content-Location -> SSE headers -> metadata/replay/live/end.
```

Use `parseCanonicalRunSubmission(c, false)` and `createCanonicalRunBundle`; do not call `CreateCanonicalRun`. Project the returned Message and Run before upgrading the response. Set:

```go
c.Header("Content-Location", canonicalRunPath(threadID, response.Run.RunID))
c.Header("Location", canonicalRunStreamPath(threadID, response.Run.RunID))
setLangGraphRunStreamHeaders(c)
```

Then call the new lower-level `streamCanonicalRunEvents` with the persisted Run, requested modes, `after_event_id=0`, and `cancelOnDisconnect` derived from `submission.Options.OnDisconnect == "cancel"`.

- [ ] **Step 4: Implement reconnect replay and disconnect policy**

`ReconnectCanonicalRunStream` must independently parse `after_event_id` and `Last-Event-ID`, use the larger valid cursor when both are present, validate `cancel_on_disconnect` as exact SDK-compatible `true|1` or `false|0`, authorize the path Run, set `Content-Location`, the exact reconnect `Location`, and SSE headers, then call the same lower-level streamer. A reconnect request with `cancel_on_disconnect=true|1` is an explicit per-connection override and must not be suppressed by the Run's persisted `on_disconnect=continue` default. Case variants, surrounding whitespace and other values return `422`. The lower-level loop must:

1. write one `metadata` event;
2. page `ApplicationService.ListRunEvents` after the cursor;
3. write only `ProjectPublicRunEvent` output through existing LangGraph-compatible mode projection helpers;
4. poll for new persisted events;
5. write one `end` event after terminal state;
6. cancel only when the writer confirms a client disconnect and the request opted into cancellation;
7. stop without cancellation on timeout, terminal close or `on_disconnect=continue`.

For canonical output, `messages-tuple` remains a request mode only: publicly projected
`message.*`, `llm.*` and equivalent message events use `event: messages` with tuple data.
Do not change the legacy LangGraph or TaskThread route behavior while adding this adapter.

The implementation may reuse package-local SSE writer interfaces, constants and public event projection helpers from `langgraph_run_service.go`; it must not call a LangGraph or TaskThread HTTP handler.

- [ ] **Step 5: Run SSE and source-stream regressions**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'Test(Stream|Reconnect)CanonicalRun|Test.*LangGraph.*Stream|TestTaskThreadRunEventStream' -count=1
```

Expected: PASS; canonical no longer returns `501`; source SSE tests remain unchanged.

- [ ] **Step 6: Commit SSE completion**

```bash
git add backend/api/handler/coze/workbench_canonical_entrypoints.go backend/api/handler/coze/workbench_canonical_run_stream.go backend/api/handler/coze/workbench_canonical_run_stream_test.go
git commit -m "feat: complete canonical workbench run streams"
```

### Task 4: Add Shared Product Validation, Projection And Logs

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_product_contract.go`
- Create: `backend/api/handler/coze/workbench_canonical_product_contract_test.go`
- Create: `backend/api/handler/coze/workbench_canonical_product_projection.go`
- Create: `backend/api/handler/coze/workbench_canonical_product_projection_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_contract.go`
- Modify: `backend/api/handler/coze/workbench_canonical_projection.go`
- Modify: `backend/api/handler/coze/workbench_canonical_projection_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service_test.go`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `scripts/workbench-execution-graph/contract.mjs`

- [x] **Step 1: Write failing validation and redaction tests**

Add table tests for:

```go
func TestCanonicalProductPaginationAcceptsIntegralOffsetPages(t *testing.T)
func TestCanonicalProductPaginationRejectsNegativeOrMisalignedOffset(t *testing.T)
func TestCanonicalProductRouteIDsRejectMalformedAndZeroIDs(t *testing.T)
func TestCanonicalProductJSONRejectsUnknownFieldsAndOversizedBodies(t *testing.T)
func TestCanonicalProductProjectionUsesStringIDsAndRFC3339Times(t *testing.T)
func TestCanonicalProductProjectionRemovesRawUsageSecretsAndWorkerIdentity(t *testing.T)
func TestCanonicalProductCompletionLogContainsResourceFieldsWithoutPayload(t *testing.T)
```

Use fixtures containing `authorization`, `api_key`, `tool_arguments`, `provider_body`, `worker_id`, `lease_token`, `raw_usage` and a signed URL. Assert none appears in response JSON or captured logs.

- [x] **Step 2: Implement one pagination contract**

`canonicalProductPagination` accepts `limit` and `offset`, defaults to `50/0`, caps limit at `200`, rejects non-integral page offsets, and returns both application paging and public paging:

```go
type canonicalProductPage struct {
	Limit  int32
	Offset int32
	Page   int32
}

func (p canonicalProductPage) hasMore(total int64) bool {
	return int64(p.Offset)+int64(p.Limit) < total
}

func (p canonicalProductPage) nextCursor(total int64) *string {
	if !p.hasMore(total) {
		return nil
	}
	next := strconv.FormatInt(int64(p.Offset)+int64(p.Limit), 10)
	return &next
}
```

The handler passes `Page` and `Limit` to current application DTOs. It never silently rounds an arbitrary offset.

- [x] **Step 3: Implement public resource projections**

Use `ProjectPublicArtifact`, `ProjectPublicTokenUsage`, `canonicalEntityMetadataFromJSON`, `canonicalSanitizeMap`, `canonicalCleanString` and `canonicalTime`. Do not duplicate their redaction rules. Scan jobs expose a hashed `worker_ref` and stable `error_code`, not raw worker IDs or raw `LastError`. Token usage never defines `raw_usage` or raw metadata fields in its canonical struct.

Extend the existing additive `coze` projections required by the current UI adapter:

```go
type canonicalThreadCoze struct {
	ProductStatus     string `json:"product_status"`
	InitialSubmission any    `json:"initial_submission"`
	Source            string `json:"source"`
	Progress          int32  `json:"progress"`
	LastUserMessage   string `json:"last_user_message"`
	LastAgentMessage  string `json:"last_agent_message"`
}

type canonicalRunCoze struct {
	MessageID         *string           `json:"message_id"`
	SubmissionMessage *canonicalMessage `json:"submission_message,omitempty"`
	AttemptKind       string            `json:"attempt_kind"`
	SourceRunID       *string           `json:"source_run_id"`
	ParentRunID       *string           `json:"parent_run_id"`
	RunKind           string            `json:"run_kind"`
	StreamModes       []string          `json:"stream_modes"`
	OnDisconnect      string            `json:"on_disconnect"`
	Durability        string            `json:"durability"`
	TerminalReason    *string           `json:"terminal_reason"`
	StartedAt         *string           `json:"started_at"`
	EndedAt           *string           `json:"ended_at"`
}
```

Populate these only from existing public application projections. In create-run responses attach the already committed User Message; list/get responses leave `submission_message` absent. This is additive JSON inside the existing `coze` extension and does not change core SDK fields.

- [x] **Step 4: Extend structured completion logging safely**

Add `ResourceType`, `ResourceID`, `Limit`, `Offset` and `LifecycleStage` to `canonicalRequestLog`. Normalize enums and hash arbitrary resource IDs before logging unless they are already positive numeric IDs. Keep all existing fields and event name unchanged. Product handlers set only identifiers relevant to their operation; signed URL handlers log the artifact ID and result category, never the URL.

- [x] **Step 5: Run shared contract tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze -run 'TestCanonical(Product|Thread|Run).*Projection|TestCanonicalProductPagination|TestCanonicalProductCompletionLog' -count=1
```

Expected: PASS with no sensitive fixture values in output or logs.

- [x] **Step 5a: Harden IDL wire shape and public identifier boundaries**

Product wire structs now match `thread_product.thrift` required/optional presence exactly. Required entity IDs fail closed, scan jobs omit lease and worker internals while always emitting safe `worker_ref`/`error_code`, and bounded nonnumeric public identifiers remain available only when they are not sensitive values or URLs.

- [x] **Step 6: Commit shared helpers**

```bash
git add backend/api/handler/coze/workbench_canonical_contract.go backend/api/handler/coze/workbench_canonical_projection.go backend/api/handler/coze/workbench_canonical_product_contract.go backend/api/handler/coze/workbench_canonical_product_projection.go backend/api/handler/coze/*canonical*test.go
git commit -m "feat: add canonical product projections"
```

### Task 5: Implement Stable-ID Upload Operations

**Files:**
- Modify: `backend/application/agentthread/upload_file.go`
- Modify: `backend/application/agentthread/upload_file_test.go`
- Create: `backend/api/handler/coze/workbench_canonical_upload_service.go`
- Create: `backend/api/handler/coze/workbench_canonical_upload_service_test.go`

- [ ] **Step 1: Write failing application tests for file-ID deletion**

Add `DeleteTaskThreadUploadFileByIDRequest` and tests proving that the new use case:

- authorizes `SpaceID/UserID/ThreadID` through `ListTaskThreadUploadFiles`;
- matches exactly one positive `FileID`;
- delegates deletion to the current filename-based method;
- returns `Deleted=false` for a missing/already deleted ID;
- never deletes a same-name file from another Thread or space;
- leaves `DeleteTaskThreadUploadFile` behavior unchanged.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./application/agentthread -run 'TestDeleteTaskThreadUploadFile(ByID|ByName)' -count=1
```

Expected: FAIL because the ID-based application method does not exist.

- [ ] **Step 2: Implement the narrow application orchestration**

Add these DTOs and method in `upload_file.go`:

```go
type DeleteTaskThreadUploadFileByIDRequest struct {
	SpaceID  int64
	UserID   int64
	ThreadID int64
	FileID   int64
}

func (s *ApplicationService) DeleteTaskThreadUploadFileByID(
	ctx context.Context,
	req *DeleteTaskThreadUploadFileByIDRequest,
) (*DeleteTaskThreadUploadFileResponse, error)
```

Validate positive scope and ID, call `ListTaskThreadUploadFiles`, find the matching summary, then call the existing `DeleteTaskThreadUploadFile` with that summary's filename. Do not add a repository method, migration, second delete policy or object-storage branch.

- [ ] **Step 3: Write failing upload handler tests**

Cover gate-off `404`, unauthenticated, wrong workspace, malformed IDs, empty multipart, file-count/size bounds, partial skipped filenames, list response, stable-ID delete, delete idempotency and safe completion logs. Assert canonical never accepts filename in the path.

- [ ] **Step 4: Implement the three upload handlers**

Every handler starts with `beginCanonicalRequestLog`, `requireCanonicalAPI`, `requireCanonicalAgentThreadService`, `canonicalPathID`, `canonicalSpaceID` and `workbenchThreadAccessContext`. Upload accepts `files` or singular `file`, applies the existing 10-file/50 MiB each/100 MiB total bounds before calling `UploadTaskThreadFiles`, and returns:

```json
{"uploads":[],"skipped_files":[]}
```

List returns named array/total/has_more. Delete calls `DeleteTaskThreadUploadFileByID`; success returns `204`, while an absent or unauthorized resource uses the same canonical not-found response.

- [ ] **Step 5: Run upload and source regressions**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./application/agentthread ./api/handler/coze -run 'Test.*(CanonicalThreadUpload|TaskThreadUploadFile)' -count=1
```

Expected: PASS; old filename route tests remain unchanged.

- [ ] **Step 6: Commit upload support**

```bash
git add backend/application/agentthread/upload_file.go backend/application/agentthread/upload_file_test.go backend/api/handler/coze/workbench_canonical_upload_service.go backend/api/handler/coze/workbench_canonical_upload_service_test.go
git commit -m "feat: add canonical workbench uploads"
```

### Task 6: Implement Message Compatibility And Suggestions

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_message_product_service.go`
- Create: `backend/api/handler/coze/workbench_canonical_message_product_service_test.go`

- [ ] **Step 1: Write failing message and suggestion tests**

Cover:

```go
func TestAppendCanonicalThreadMessageRejectsOrdinaryUserTurnBeforeMutation(t *testing.T)
func TestAppendCanonicalThreadMessageRequiresInternalCompatModeAndOwnedRun(t *testing.T)
func TestAppendCanonicalThreadMessageReturnsPublicMessage(t *testing.T)
func TestGenerateCanonicalThreadSuggestionsUsesPersistedPublicMessages(t *testing.T)
func TestGenerateCanonicalThreadSuggestionsReturnsEmptyListWhenProviderFails(t *testing.T)
func TestGenerateCanonicalThreadSuggestionsNeverLogsMessageContent(t *testing.T)
```

- [ ] **Step 2: Implement internal append safeguards**

Require `append_mode="internal_compat"`, a positive owned `run_id`, and role `assistant` or `tool`. Reject `user`/`human` with `409 atomic_run_submission_required` before `AppendMessage`. This route is not used for ordinary UI turns; `POST /runs` remains the only User Message + Run write boundary.

- [ ] **Step 3: Implement best-effort suggestions from persisted messages**

Authorize the Thread through `agentthread.ApplicationService`, load at most the newest 40 public messages, map only role/content into `application/workbench.SuggestionMessage`, and call `appworkbench.SVC.GenerateSuggestions`. A provider error returns HTTP `200` with an empty array and a bounded failure category in logs. Do not include the error text or message content in logs.

- [ ] **Step 4: Run focused and source tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./application/workbench -run 'Test(Append|Generate)CanonicalThread|TestGenerateTaskThreadSuggestions' -count=1
```

Expected: PASS; source suggestion behavior remains unchanged.

- [ ] **Step 5: Commit message product support**

```bash
git add backend/api/handler/coze/workbench_canonical_message_product_service.go backend/api/handler/coze/workbench_canonical_message_product_service_test.go
git commit -m "feat: add canonical workbench suggestions"
```

### Task 7: Implement Artifact And Scan Operations

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_artifact_service.go`
- Create: `backend/api/handler/coze/workbench_canonical_artifact_service_test.go`

- [ ] **Step 1: Write failing table-driven handler tests**

For list/content/signed URL/delete/restore/review/list jobs/retry jobs, test success, empty list, pagination, malformed IDs, wrong Thread, wrong space, role denial, scan-blocked content, unsupported preview mode, retry conflict and application dependency failure. Signed URL tests must assert the URL is present only in the response and absent from logs and SSE fixtures.

- [ ] **Step 2: Implement list and content reads**

Map `limit/offset` through `canonicalProductPagination`; call `ListArtifacts`, `ReadArtifactContent` and `CreateArtifactSignedURL` with server-derived space/viewer identity. Raw content keeps `Content-Disposition`, `Content-Type`, `X-Content-Type-Options: nosniff` and current scan/size/type checks. Do not send content through a JSON envelope.

- [ ] **Step 3: Implement mutations and scan operations**

Call `DeleteArtifact`, `RestoreArtifact`, `ReviewArtifactScan`, `ListArtifactScanJobs` and `RetryArtifactScanJob` directly. Delete returns `204`; restore/review/retry return the projected resource or operation result. Preserve current conflict and authorization mapping; do not copy retention or scanner state logic into handlers.

- [ ] **Step 4: Run artifact, security and source regressions**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./application/agentthread -run 'Test.*(CanonicalThreadArtifact|TaskThreadArtifact|ArtifactScan)' -count=1
```

Expected: PASS with current source content/download/review behavior unchanged.

- [ ] **Step 5: Commit Artifact support**

```bash
git add backend/api/handler/coze/workbench_canonical_artifact_service.go backend/api/handler/coze/workbench_canonical_artifact_service_test.go
git commit -m "feat: add canonical workbench artifacts"
```

### Task 8: Implement Memory And Audit Operations

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_memory_audit_service.go`
- Create: `backend/api/handler/coze/workbench_canonical_memory_audit_service_test.go`

- [ ] **Step 1: Write failing read contract tests**

Cover Memory list/export/audit, Guardrail list/export and MCP list with empty/paged results, filter validation, cross-Thread resource IDs, export permission denial and sensitive metadata fixtures. Assert IDs are strings, times are RFC 3339 and no tool/provider body is exposed.

- [ ] **Step 2: Implement read and export handlers**

Map filters directly to `ListMemories`, `ExportMemories`, `ListMemoryAuditEvents`, `ListGuardrailAuditEvents`, `ExportGuardrailAuditEvents` and `ListMCPRuntimeAuditEvents`. Use viewer ID from session only. Export objects retain their schema name and counts, convert exported time to RFC 3339, and use the same safe projections as list responses.

- [ ] **Step 3: Write failing Memory mutation tests**

Cover update/delete/restore/clear/import success, invalid scope, invalid JSON metadata, import limit, duplicate import behavior, audit failure, transaction failure, unauthorized resource and no second write after an error. Delete returns `204`; each other operation returns a direct resource or summary.

- [ ] **Step 4: Implement Memory mutations through existing use cases**

Call `UpdateMemory`, `DeleteMemory`, `RestoreMemory`, `ClearMemories` and `ImportMemories` with server-derived actor/viewer IDs. Parse canonical JSON metadata into the current validated JSON string expected by the application service. Keep current import count and item-size limits; no handler-owned transaction or audit write is allowed.

- [ ] **Step 5: Run Memory/Audit and source regressions**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./application/agentthread -run 'Test.*(CanonicalThreadMemor|CanonicalThreadGuardrail|CanonicalThreadMCP|TaskThreadMemor|TaskThreadGuardrail|TaskThreadMCP)' -count=1
```

Expected: PASS; source route requests, envelopes and audit behavior remain unchanged.

- [ ] **Step 6: Commit Memory and Audit support**

```bash
git add backend/api/handler/coze/workbench_canonical_memory_audit_service.go backend/api/handler/coze/workbench_canonical_memory_audit_service_test.go
git commit -m "feat: add canonical workbench memory audits"
```

### Task 9: Implement Token Usage And Subagent Retry

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_usage_retry_service.go`
- Create: `backend/api/handler/coze/workbench_canonical_usage_retry_service_test.go`

- [ ] **Step 1: Write failing usage tests**

Cover Thread aggregate, one Run, child Run inclusion, source filter, pagination, wrong Thread/Run pair, empty usage and provider-raw fixture redaction. The JSON scan must prove `raw_usage` and raw metadata keys do not exist.

- [ ] **Step 2: Implement usage reads**

Validate an optional `run_id` belongs to the path Thread. Call `GetRunTokenUsage` when present and `GetThreadTokenUsage` otherwise. Return projected rows, aggregate, run aggregates, total, `has_more` and cursor. Do not add provider usage data to metadata or logs.

- [ ] **Step 3: Write failing retry tests**

Cover positive owned subagent Run, top-level Run rejection, non-failed Run conflict, cross-Thread denial, idempotent replay with the same key, conflicting payload/key and no mutation on failed authorization.

- [ ] **Step 4: Implement retry through `RetrySubagentRun`**

Parse `Idempotency-Key` with the same canonical validation and principal scoping used by Run creation. Call `RetrySubagentRun` exactly once and return `projectCanonicalRun`. Never reinterpret a top-level retry as a child retry; top-level retry continues to use `POST /runs`.

- [ ] **Step 5: Run usage/retry and source regressions**

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./application/agentthread -run 'Test.*(CanonicalThreadTokenUsage|CanonicalSubagentRun|TaskThreadTokenUsage|RetryTaskThreadSubagentRun)' -count=1
```

Expected: PASS with one persisted retry attempt per idempotency key.

- [ ] **Step 6: Commit usage and retry support**

```bash
git add backend/api/handler/coze/workbench_canonical_usage_retry_service.go backend/api/handler/coze/workbench_canonical_usage_retry_service_test.go
git commit -m "feat: add canonical workbench usage retry"
```

### Task 10: Document And Verify Backend Checkpoint A

**Files:**
- Create: `docs/superpowers/runbooks/workbench-canonical-product-client-validation.md`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/specs/2026-07-27-workbench-canonical-product-client-migration-design.md`

- [ ] **Step 1: Write the runbook**

Document:

- default-off gate and local enable command;
- session and `X-Coze-Space-ID` requirements;
- all 47 routes and the 26 product operations;
- safe curl examples for list, create Run, upload, retry and SSE replay;
- `Last-Event-ID`, `after_event_id`, `cancel_on_disconnect` and proxy buffering checks;
- request completion log fields and prohibited log content;
- V1 route preservation and ChatTask `404` probes;
- stop conditions and gate-off rollback;
- an explicit warning that API key/Bearer scope, distributed rate limiting and production gateway support remain outside this checkpoint.

- [ ] **Step 2: Update long-lived facts**

In `project-context.md`, record that canonical core plus product routes exist behind the same default-off gate, source routes remain active, and UI remains V1 until Checkpoint B passes. In the design spec, change document state from “等待书面规格复核” to “设计已确认，实施计划已冻结”; do not mark implementation complete.

- [ ] **Step 3: Run deterministic generation and full focused Go tests**

```bash
cd backend
bash scripts/verify_api_codegen.sh
GOCACHE=/private/tmp/coze-workbench-product-go-cache go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository -count=1
```

Expected: all packages PASS and both codegen hash checks PASS.

- [ ] **Step 4: Run generated TypeScript contract tests**

```bash
cd frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts src/__tests__/workbench-task-contract.test.ts src/__tests__/workbench-task-memory.test.ts
```

Expected: canonical 47-method contract and unchanged TaskThread source contracts PASS.

- [ ] **Step 5: Run retirement and dependency scans**

```bash
rg -n 'workbench/(chat|tasks)|ChatTask|sendWorkbenchChat|legacy_task_id|source_task_id' backend idl frontend/apps/coze-studio/src frontend/packages/arch/api-schema/src --glob '!**/__tests__/**' --glob '!**/*_test.go'
rg -n 'include "\./task.thrift"|workbenchTask|workbench\.task' idl/workbench/thread.thrift idl/workbench/thread_product.thrift backend/api/handler/coze/workbench_canonical_*.go
```

Expected: first command finds no live ChatTask production symbol; second finds no canonical dependency on TaskThread IDL/model/handler.

- [ ] **Step 6: Inspect diff scope and generated ownership**

```bash
git diff --check
git status --short
git diff --stat dev...HEAD
```

Expected: only planned IDL/generated files, canonical handler/tests, one additive upload application method and documentation changed; no migration, runtime, worker or source handler change.

- [ ] **Step 7: Commit Checkpoint A documentation**

```bash
git add docs/superpowers/runbooks/workbench-canonical-product-client-validation.md docs/superpowers/context/project-context.md docs/superpowers/specs/2026-07-27-workbench-canonical-product-client-migration-design.md
git commit -m "docs: add canonical product validation runbook"
```

## Checkpoint A Acceptance

Checkpoint A is ready for review only when:

1. all 47 canonical routes are generated, default-off and session/space authorized;
2. both canonical SSE routes return real replay/live streams rather than `501`;
3. 26 product routes call existing application use cases and use direct canonical responses;
4. upload deletion uses stable `file_id` without changing the source filename route;
5. no database/runtime/worker/state-machine change exists;
6. source route snapshots and ChatTask retirement probes pass;
7. sensitive projection and log tests pass;
8. deterministic backend/frontend codegen passes;
9. Checkpoint B has not yet changed the production UI client.
