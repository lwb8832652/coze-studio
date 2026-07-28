# Workbench Canonical Product Client Validation

本手册用于 Checkpoint A 的后端 canonical 产品扩展验收。它不代表生产 UI 已切到
canonical，也不授权删除 `/api/workbench/task_threads` 或 `/api/threads`。

## Scope

- 验证默认关闭的 `/api/workbench/threads` canonical 合同；
- 验证 21 个 core 路由和 26 个 product 路由；
- 验证旧 `/api/workbench/task_threads`、`/api/threads` 与 ChatTask 退役边界；
- 验证 session、workspace header、SSE cursor、公开投影和日志脱敏；
- 不验证 API key、Bearer scope、分布式限流、生产网关 SSE 或外部容量隔离。

生产级外部接入仍属于后续阶段。Checkpoint A 只证明服务端合同和旧接口兼容性。

## Local Gate

canonical API 默认关闭。关闭时所有 canonical 路由应返回 `404`，且不能进入业务
handler。

本地开启：

```bash
export APP_ENV=debug
export COZE_WORKBENCH_CANONICAL_API_ENABLED=true
```

使用本地后端时把上述变量放入 ignored `bin/.env.debug` 或当前 shell，不提交到仓库。
关闭或回滚只需删除变量或改为非 `true`，例如：

```bash
unset COZE_WORKBENCH_CANONICAL_API_ENABLED
```

## Auth Contract

所有 canonical Workbench 请求必须同时满足：

- 已登录 session cookie；
- `X-Coze-Space-ID` 为正整数；
- session principal 是该 workspace 成员；
- path `thread_id`、`run_id`、resource id 均由服务端校验 ownership；
- body 中的 `space_id`、`user_id`、`owner` 不作为授权事实。

占位变量：

```bash
BASE_URL=http://localhost:8888
COOKIE='session_key=<local-session-cookie>'
SPACE_ID=<workspace-id>
THREAD_ID=<thread-id>
RUN_ID=<run-id>
```

通用 header：

```bash
-H "Cookie: ${COOKIE}" \
-H "X-Coze-Space-ID: ${SPACE_ID}"
```

## Route Surface

### Core Routes

| Method | Path |
| --- | --- |
| `POST` | `/api/workbench/threads` |
| `POST` | `/api/workbench/threads/search` |
| `GET` | `/api/workbench/threads/:thread_id` |
| `PATCH` | `/api/workbench/threads/:thread_id` |
| `DELETE` | `/api/workbench/threads/:thread_id` |
| `GET` | `/api/workbench/threads/:thread_id/state` |
| `POST` | `/api/workbench/threads/:thread_id/state` |
| `GET` | `/api/workbench/threads/:thread_id/history` |
| `POST` | `/api/workbench/threads/:thread_id/history` |
| `GET` | `/api/workbench/threads/:thread_id/messages` |
| `GET` | `/api/workbench/threads/:thread_id/runs` |
| `POST` | `/api/workbench/threads/:thread_id/runs` |
| `POST` | `/api/workbench/threads/:thread_id/runs/stream` |
| `POST` | `/api/workbench/threads/:thread_id/runs/wait` |
| `GET` | `/api/workbench/threads/:thread_id/runs/:run_id` |
| `GET` | `/api/workbench/threads/:thread_id/runs/:run_id/stream` |
| `GET` | `/api/workbench/threads/:thread_id/runs/:run_id/join` |
| `POST` | `/api/workbench/threads/:thread_id/runs/:run_id/cancel` |
| `POST` | `/api/workbench/threads/:thread_id/runs/:run_id/resume` |
| `GET` | `/api/workbench/threads/:thread_id/runs/:run_id/events` |
| `GET` | `/api/workbench/threads/:thread_id/runs/:run_id/messages` |

Forbidden variants must remain unreachable:

- `POST /api/workbench/threads/:thread_id/runs/:run_id/stream`
- `POST /api/workbench/threads/:thread_id/runs/:run_id/join`

### Product Routes

| Method | Path | Operation |
| --- | --- | --- |
| `POST` | `/api/workbench/threads/:thread_id/messages` | internal message append |
| `POST` | `/api/workbench/threads/:thread_id/suggestions` | suggestions |
| `GET` | `/api/workbench/threads/:thread_id/uploads` | list uploads |
| `POST` | `/api/workbench/threads/:thread_id/uploads` | multipart upload |
| `DELETE` | `/api/workbench/threads/:thread_id/uploads/:file_id` | delete upload by file id |
| `GET` | `/api/workbench/threads/:thread_id/artifacts` | list artifacts |
| `GET` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id/content` | artifact content |
| `GET` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url` | artifact signed URL |
| `DELETE` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id` | delete artifact |
| `POST` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore` | restore artifact |
| `POST` | `/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review` | scan review |
| `GET` | `/api/workbench/threads/:thread_id/artifact_scan_jobs` | list scan jobs |
| `POST` | `/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry` | retry scan job |
| `GET` | `/api/workbench/threads/:thread_id/token_usage` | token usage |
| `GET` | `/api/workbench/threads/:thread_id/memories` | list memories |
| `PUT` | `/api/workbench/threads/:thread_id/memories/:memory_id` | update memory |
| `DELETE` | `/api/workbench/threads/:thread_id/memories/:memory_id` | delete memory |
| `POST` | `/api/workbench/threads/:thread_id/memories/:memory_id/restore` | restore memory |
| `POST` | `/api/workbench/threads/:thread_id/memories/clear` | clear memories |
| `GET` | `/api/workbench/threads/:thread_id/memories/export` | export memories |
| `POST` | `/api/workbench/threads/:thread_id/memories/import` | import memories |
| `GET` | `/api/workbench/threads/:thread_id/memories/audit_events` | memory audit |
| `GET` | `/api/workbench/threads/:thread_id/guardrail_audit_events` | guardrail audit |
| `GET` | `/api/workbench/threads/:thread_id/guardrail_audit_events/export` | guardrail export |
| `GET` | `/api/workbench/threads/:thread_id/mcp_runtime_audit_events` | MCP runtime audit |
| `POST` | `/api/workbench/threads/:thread_id/runs/:run_id/retry` | subagent retry |

## Safe Curl Probes

### Gate-Off Probe

```bash
curl -i -X POST "${BASE_URL}/api/workbench/threads" \
  -H "Cookie: ${COOKIE}" \
  -H "X-Coze-Space-ID: ${SPACE_ID}" \
  -H "Content-Type: application/json" \
  --data '{"metadata":{"title":"gate-off probe"}}'
```

Expected with gate off: `404`.

### Create Thread With Initial Run

```bash
curl -i "${BASE_URL}/api/workbench/threads" \
  -H "Cookie: ${COOKIE}" \
  -H "X-Coze-Space-ID: ${SPACE_ID}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: local-create-thread-001" \
  --data '{
    "metadata": {"title": "Local canonical validation"},
    "coze": {
      "initial_run": {
        "assistant_id": "agent",
        "input": {
          "messages": [
            {"role": "user", "content": "请生成一个三点排查计划"}
          ]
        },
        "config": {"runtime": "eino_adk"},
        "metadata": {"source": "runbook"}
      }
    }
  }'
```

Expected:

- `200`;
- response has decimal string `thread_id`;
- response does not contain `input`, `command`, `config`, `context`, provider body or idempotency key.

### Create Follow-Up Run

```bash
curl -i "${BASE_URL}/api/workbench/threads/${THREAD_ID}/runs" \
  -H "Cookie: ${COOKIE}" \
  -H "X-Coze-Space-ID: ${SPACE_ID}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: local-run-001" \
  --data '{
    "assistant_id": "agent",
    "input": {
      "messages": [
        {"role": "user", "content": "继续补充风险项"}
      ]
    },
    "metadata": {"source": "runbook_followup"},
    "stream_mode": ["messages-tuple", "updates"],
    "on_disconnect": "continue"
  }'
```

Expected:

- `200`;
- `Content-Location` points to `/threads/{thread_id}/runs/{run_id}`;
- the persisted User Message and Run are created atomically;
- response does not expose request body internals.

### Multipart Upload

```bash
curl -i "${BASE_URL}/api/workbench/threads/${THREAD_ID}/uploads" \
  -H "Cookie: ${COOKIE}" \
  -H "X-Coze-Space-ID: ${SPACE_ID}" \
  -F "files=@/path/to/local-test.txt;type=text/plain"
```

Expected:

- `200`;
- response has `uploads[]` with decimal string `file_id`;
- deletion uses `DELETE /uploads/{file_id}`, never filename.

### Subagent Retry

```bash
curl -i -X POST \
  "${BASE_URL}/api/workbench/threads/${THREAD_ID}/runs/${RUN_ID}/retry" \
  -H "Cookie: ${COOKIE}" \
  -H "X-Coze-Space-ID: ${SPACE_ID}" \
  -H "Idempotency-Key: local-subagent-retry-001"
```

Expected:

- retry is accepted only for failed or canceled subagent runs;
- top-level runs return `422 invalid_request`;
- running subagent runs return `409 run_conflict`;
- non-empty body such as `{"idempotency_key":"x"}` returns `422 invalid_request`;
- same header key replays the same retry run;
- same key with another source run returns `409 idempotency_conflict`.

### SSE Create And Reconnect

Create stream:

```bash
curl -N -i "${BASE_URL}/api/workbench/threads/${THREAD_ID}/runs/stream" \
  -H "Cookie: ${COOKIE}" \
  -H "X-Coze-Space-ID: ${SPACE_ID}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: local-stream-001" \
  --data '{
    "assistant_id": "agent",
    "input": {
      "messages": [
        {"role": "user", "content": "流式输出一个检查清单"}
      ]
    },
    "stream_mode": ["messages-tuple", "updates"],
    "on_disconnect": "continue"
  }'
```

Reconnect by query cursor:

```bash
curl -N -i \
  "${BASE_URL}/api/workbench/threads/${THREAD_ID}/runs/${RUN_ID}/stream?after_event_id=${AFTER_EVENT_ID}" \
  -H "Cookie: ${COOKIE}" \
  -H "X-Coze-Space-ID: ${SPACE_ID}"
```

Reconnect by header cursor:

```bash
curl -N -i \
  "${BASE_URL}/api/workbench/threads/${THREAD_ID}/runs/${RUN_ID}/stream" \
  -H "Cookie: ${COOKIE}" \
  -H "X-Coze-Space-ID: ${SPACE_ID}" \
  -H "Last-Event-ID: ${AFTER_EVENT_ID}"
```

When both query `after_event_id` and `Last-Event-ID` are present, the server uses the
larger cursor. `cancel_on_disconnect=true` may cancel only when the writer confirms
disconnect and the run is still active; confirmed reconnect, timeout and terminal drains
must not cancel the run.

For any reverse proxy check, ensure buffering is disabled for SSE. The public API should
stream frames progressively; a proxy that waits for completion is not acceptable for
production, but production gateway SSE support is outside Checkpoint A.

## Logs And Redaction

Canonical completion logs should include safe operational fields only:

- `event_name=workbench.api.request.completed`;
- `client_contract=canonical_v1`;
- `operation`;
- `route_template`;
- `http_method`;
- `http_status`;
- `duration_ms`;
- `outcome`;
- `principal_id_hash`;
- `thread_id`;
- `run_id`;
- `source_run_id`;
- `resource_type`;
- `resource_id`;
- `limit`;
- `offset`;
- `lifecycle_stage`;
- `response_body_kind`;
- `location_kind`;
- `response_projection_version`;
- `stream_modes`;
- `raise_error_mode`;
- `failure_projection`;
- `idempotency_key_hash`.

Logs and responses must not contain:

- request body;
- message content;
- memory content;
- signed URL;
- credential, token or API key;
- tool arguments or results;
- provider body or raw usage;
- checkpoint bytes;
- object storage URI;
- hidden config or context.

## Legacy Preservation

Run these probes with the same server binary:

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-product-go-cache \
go test -p 1 -gcflags="all=-l -N" ./api/router/coze \
  -run '^TestWorkbenchCanonicalThreadRoutes$' -count=1
```

Expected:

- canonical route snapshot equals 47 routes;
- `/api/workbench/task_threads` snapshot is unchanged;
- `/api/threads` snapshot is unchanged;
- `/api/workbench/tasks*` and `/api/workbench/chat` return `404`;
- ChatTask retired paths do not enter a handler chain.

Additional source scan:

```bash
rg -n 'ChatTask|sendWorkbenchChat|source_task_id' \
  backend idl frontend/apps/coze-studio/src frontend/packages/arch/api-schema/src \
  --glob '!**/__tests__/**' --glob '!**/*_test.go'

rg -n 'legacy_task_id' \
  backend idl frontend/apps/coze-studio/src frontend/packages/arch/api-schema/src \
  --glob '!**/__tests__/**' --glob '!**/*_test.go'

rg -n 'include "\./task.thrift"|workbenchTask|workbench\.task' \
  idl/workbench/thread.thrift idl/workbench/thread_product.thrift \
  backend/api/handler/coze/workbench_canonical_*.go
```

Expected:

- the first and third commands return exit code `1` with no output;
- `legacy_task_id` appears only in metadata protection, projection filtering and
  retired LangGraph metadata cleanup;
- no live production ChatTask symbol;
- canonical IDL and canonical handlers do not depend on TaskThread IDL/model/handler.

## Deterministic Verification

Backend:

```bash
cd backend
bash scripts/verify_api_codegen.sh
GOCACHE=/private/tmp/coze-workbench-product-go-cache \
go test -p 1 -gcflags="all=-l -N" \
  ./api/handler/coze \
  ./api/router/coze \
  ./application/agentthread \
  ./domain/agentthread/service \
  ./domain/agentthread/repository \
  -count=1
```

Atlas migration hash and validate, from the repository root:

```bash
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" \
  arigaio/atlas:0.35.0-community-alpine \
  migrate hash --dir file:///migrations

docker run --rm -v "$PWD":/work -w /work \
  arigaio/atlas:0.35.0-community-alpine \
  migrate validate --dir file://docker/atlas/migrations
```

Frontend generated contract:

```bash
cd frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
```

The consolidated contract test freezes the 47 generated canonical API functions and
asserts that generated canonical thread sources do not import or reference
`TaskThread` contracts.

Diff and ownership:

```bash
git diff --check
git status --short
git diff --stat dev...HEAD
```

Expected diff scope:

- IDL and generated schema/client for canonical contract;
- canonical handler and tests;
- one additive upload delete-by-file-ID application use case;
- one additive idempotent message index for suggestions recent public message lookup;
- documentation;
- no table semantic, runtime, worker, state-machine or old source handler rewrite.

## Stop Conditions

Stop Checkpoint A validation if any of these occurs:

- canonical gate off does not return `404`;
- unauthenticated request is accepted;
- missing or wrong `X-Coze-Space-ID` is accepted;
- cross-workspace or cross-thread access returns data;
- any canonical response includes raw provider, credential, tool payload or checkpoint bytes;
- canonical write falls back to `/api/workbench/task_threads`;
- a failed retry/resume/upload/artifact/memory write leaves partial records;
- SSE reconnect duplicates or skips logical events;
- old `/api/workbench/task_threads` or `/api/threads` route snapshot changes;
- retired ChatTask route enters a handler chain;
- generated code changes after verification.

When a stop condition triggers, leave production UI on V1, turn off
`COZE_WORKBENCH_CANONICAL_API_ENABLED`, keep evidence, and debug on the feature branch.
