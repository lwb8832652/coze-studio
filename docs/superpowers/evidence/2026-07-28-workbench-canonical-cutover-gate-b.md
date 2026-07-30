# Workbench Canonical Cutover Gate B

## Decision

Gate B 的 Workbench 产品切换结论为 **PASS**，但只覆盖已经满足证据的边界：

- Workbench/Tasks UI 只使用 `/api/workbench/threads/**`；
- 47 个 canonical method/path 组合保持注册并由常规鉴权保护；
- 36 个 TaskThread V1 和 23 个本地 LangGraph Thread method/path 组合全部不可达；
- ChatTask 的 `/api/workbench/tasks*` 与 `/api/workbench/chat` 继续不可达；
- 11 个 Scheduled Task method/path 组合保持注册；
- 10 个 `/api/runs/**` stateless adapter 因五项零使用审计仍为 `BLOCKED` 而明确保留，
  不把“本地无调用方”误写成可删除结论。

该结论不授权合入 `dev` 或推送远程。两项动作继续执行
`docs/superpowers/runbooks/dev-integration-audit.md` 的两次独立确认门禁。

## Provenance And Environment

- 分支：`codex/workbench-canonical-ui-cutover-retirement`
- Task 16 开始 SHA：`04b428f2bc671016e78a340cc045dd4e7b6fe9ba`
- 登录日志安全修复提交：`2c713b4a`；只删除敏感日志并增加回归测试。
- 验证日期：`2026-07-30`
- 前端：feature worktree，`http://localhost:8080`
- 后端：同一 feature worktree，`http://localhost:8888`
- 数据库：只读取 ignored debug 配置并输出分类，结果为
  `mysql_host_class=remote`；证据不包含 host、账号、密码或 DSN。
- 页面账号与空间：复用本地调试 runbook 的测试登录和已配置在线空间；本文不记录
  cookie、session、完整动态 ID、请求正文或用户内容。
- 数据边界：没有数据库迁移、批量更新、删除在线记录或领域状态机改造。
- 本 evidence 文件不写自引用最终 SHA。提交后使用以下命令机械获取：

```bash
git log -1 --format=%H -- docs/superpowers/evidence/2026-07-28-workbench-canonical-cutover-gate-b.md
```

## Final Route Surface

权威证据是 Hertz router 的完整 method 加 template 精确集合和逐路由
`FullPath()` 解析测试，不以 handler 的参数校验错误代替路由存在性判断。

| Route family | 最终数量 | Gate B 状态 |
| --- | ---: | --- |
| `/api/workbench/threads/**` | 47 | 注册、always-on、主产品合同 |
| `/api/workbench/task_threads/**` | 36 | 全部不可达 |
| `/api/threads/**` | 23 | 全部不可达 |
| `/api/workbench/scheduled_tasks/**` 及相邻资源 | 11 | 保留 |
| `/api/runs/**` | 10 | 审计阻塞，明确保留 |

最终定向命令：

```bash
cd backend
GOCACHE=/private/tmp/coze-task16-final-go-cache go test -p 1 -gcflags="all=-l -N" ./api/router/coze ./api/handler/coze -run '^(TestWorkbenchCanonicalThreadRoutes|TestWorkbenchStatelessRunRouteResolution|TestPassportWebEmailLoginPostDoesNotLogSessionKey)$' -count=1
```

结果：退出码 `0`，`api/router/coze` 与 `api/handler/coze` 均 PASS。路由测试逐一
证明 47/36/23/11/10 的最终状态，并验证静态 `/api/runs/stream`、`/api/runs/wait`
不会被动态 `:run_id` 路由吞掉。

已登录本地后端的代表性只读探测与 router snapshot 一致：

| Probe | HTTP 结果 | 解释 |
| --- | ---: | --- |
| 现有 canonical Thread 详情 | 200 | canonical 业务链可访问 |
| 旧 TaskThread V1 详情 | 404 | 来源产品合同已移除 |
| 旧本地 LangGraph Thread 详情 | 404 | 来源兼容合同已移除 |
| 旧 ChatTask 详情 | 404 | ChatTask 继续不存在 |
| 已知 stateless Run 详情 | 200 | 按阻塞审计结论保留 |
| Scheduled Task 列表 | 403 | 路由和权限中间件存在；当前测试账号无该资源权限 |

`403` 只用于确认 Scheduled Task 没有被误删；11 个方法的完整存在性由 router
snapshot 提供。

## Browser Regression Matrix

使用 Codex in-app browser 对同一在线空间执行 Gate B。动态标识在本文中统一替换为
`{workspace_id}`、`{thread_id}` 和 `{run_id}`。

| 场景 | 结论 | 证据类型 |
| --- | --- | --- |
| 登录、session、有效空间 | PASS | 页面实测；登录 200，重启后重新登录可用 |
| 空间隔离 | PASS | 页面实测；无权限空间显示权限拒绝，不展示业务数据 |
| 列表、详情、标题、状态、Thread 切换 | PASS | 页面实测；列表进入既有与新建详情均正常 |
| 分页与 cursor | AUTOMATED SUBSTITUTE | 页面实测默认列表；翻页/cursor 边界由 client/schema 定向测试覆盖 |
| 无附件首次提交 | PASS | 页面实测；Run 完成且刷新后持久化 |
| 单附件 | PARTIAL | 页面实测选择与移除；完整提交、失败和重试由 deterministic tests 覆盖 |
| 多附件首次提交 | PASS | 页面实测；一次选择两个文件并完成一个 Run，页面显示三步 |
| streaming、终态和刷新 | PASS | 页面实测；回复只出现一次，刷新后仍为完成态 |
| reconnect 与 unknown-result | AUTOMATED SUBSTITUTE | 正常 stream 页面实测；断线和未知结果由 run-stream/client tests 覆盖 |
| follow-up 与 suggestions | PASS | 页面实测；追问完成，建议入口与返回正常 |
| cancel | PASS | 页面实测；既有长 Run 最终显示 canceled |
| resume、顶层 retry、subagent retry | AUTOMATED SUBSTITUTE | 当前在线数据没有可安全复用的 interrupted/failed/subagent Run |
| Artifact | PASS EMPTY + SUBSTITUTE | 页面显示 0 个产物；预览、下载、删除、恢复、扫描与 retry 用定向测试 |
| Token usage | PASS | 页面实测；usage 弹层和刷新后的聚合数据可见 |
| Memory 与审计 | PASS EMPTY + SUBSTITUTE | 页面显示 Memory/Guardrail/MCP audit 空态；破坏性动作使用定向测试 |
| missing resource | PASS | 页面实测；显示 `Resource not found` |
| 401/403/404/409/413/422/429/500 | PASS | 401/403/404 有页面或 route probe；其余使用 canonical client/handler tests |
| dependency unavailable | PASS WITH ENV LIMIT | Runtime doctor 显示 Sandbox disabled、MCP unknown；Artifact scanner Docker unhealthy |

Sandbox、MCP 和 Artifact scanner 的状态来自当前环境，不是 canonical transport
回归。没有为了让页面“变绿”而伪造依赖或静默 fallback。

最后一次有效详情页 reload 后，控制台最近 20 条记录全部为 `log`，没有 warning 或
error。较早的无权限空间测试按预期触发 error boundary；该有意负向测试不计为正常
业务流错误。

## Request And Log Ownership

页面 Thread workflow 实际请求只出现 canonical 路径，包含：

```text
POST /api/workbench/threads/search
POST /api/workbench/threads
GET  /api/workbench/threads/{thread_id}
GET  /api/workbench/threads/{thread_id}/messages
GET  /api/workbench/threads/{thread_id}/runs
POST /api/workbench/threads/{thread_id}/runs
GET  /api/workbench/threads/{thread_id}/runs/{run_id}/events
GET  /api/workbench/threads/{thread_id}/runs/{run_id}/stream
POST /api/workbench/threads/{thread_id}/runs/{run_id}/cancel
GET  /api/workbench/threads/{thread_id}/artifacts
GET  /api/workbench/threads/{thread_id}/token_usage
GET  /api/workbench/threads/{thread_id}/memories
GET  /api/workbench/threads/{thread_id}/guardrail_audit_events
GET  /api/workbench/threads/{thread_id}/mcp_runtime_audit_events
POST /api/workbench/threads/{thread_id}/suggestions
```

浏览器网络和后端 operation log 均未出现
`/api/workbench/task_threads/**`、本地 `/api/threads/**` 或 stateless
`/api/runs/**` 参与 Thread 页面流程。页面提交和 Run lifecycle 只有一次写入及一条
active Run stream；client equivalence、stream 和 action tests 同时约束 duplicate
write、fallback、shadow request 与第二条 SSE。

服务端 canonical log 只记录 operation、route template、status、duration、脱敏
principal、Thread/Run ID 和 outcome，不记录消息正文、Memory 内容、附件名、signed
URL、credential、provider body、tool 参数/结果或 checkpoint bytes。

### Login Session Log Defect

Gate B 运行时检查发现登录 handler 会把原始 session key 写入日志。该问题不属于
Workbench 请求合同，但会破坏生产日志安全边界，因此在进入集成审计前关闭：

1. 先增加 `TestPassportWebEmailLoginPostDoesNotLogSessionKey`，解析 `Set-Cookie` 并锁定
   session cookie 的名称、值、Path、Max-Age、SameSite、Secure 和 HttpOnly 语义，
   同时断言日志中不存在测试 session key；
2. RED 阶段测试明确失败于 session key 被写入日志；
3. 删除 `PassportWebEmailLoginPost` 的单条敏感 `Infof`，不修改登录响应、cookie、
   session 生命周期或认证应用服务；
4. GREEN 阶段定向测试通过；加强 cookie policy 断言后再次执行仍通过；
5. 重新构建并重启 feature backend，实际重新登录返回 200，运行时日志不再出现原始
   session key。

`APP_ENV=debug make build_server` 退出码为 `0`。构建脚本在二进制与配置复制完成后
输出一条既有的 `backend/static` 不存在提示，但该步骤不是失败条件，运行中的新二进制
已完成登录与 canonical 页面复验。

## Automated Verification

| Verification | Fresh result |
| --- | --- |
| Workbench/Tasks frontend | `42/42` files，`429/429` tests PASS |
| Workbench generated schema | `3/3` files，`13/13` tests PASS |
| Backend route and login security | `2/2` packages PASS |
| Execution graph source contract | PASS，115 nodes / 132 edges / 28 chains |
| Execution graph tests | `53/53` PASS |
| Graphify build | PASS，派生图写入 ignored graphify-out |
| Derived graph verification | PASS |
| `git diff --check` | PASS |

前端命令：

```bash
cd frontend/apps/coze-studio
rushx test src/pages/workbench/thread-client src/pages/workbench src/pages/tasks

cd ../../packages/arch/api-schema
rushx test __tests__/workbench-model-contract.test.ts __tests__/workbench-task-contract.test.ts src/__tests__/workbench-thread-contract.test.ts
```

执行图命令：

```bash
node scripts/workbench-execution-graph.mjs verify
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
node --test scripts/workbench-execution-graph.test.mjs
```

Task 15 已在 `04b428f2b` 记录更宽回归：前端全量 `886/899` 的 13 个 system 测试
失败与 detached baseline `8f2471cc4` 完全一致；后端全量的 6 个既有失败也与该
baseline 完全一致。Gate B 本轮又重新执行受影响范围并全部通过。既有前端测试输出仍
包含 React mock warning、Browserslist 数据过期提示和 notification polling 的 sandbox
`EPERM localhost:3000` 噪声，均未导致测试失败，也没有来源合同请求。

## Execution Graph And Current Truth

current context、runbook 与 machine contract 现统一表达：

- React 页面通过 app-owned service 访问唯一
  `CanonicalThreadClient` singleton；
- JSON、multipart、Blob、workspace header、幂等键与错误投影由 canonical client
  统一拥有；
- Run SSE 使用 `@coze-arch/fetch-stream`，不是 browser `EventSource`；
- Hertz canonical handler 继续复用
  `agentthread.ApplicationService -> domain -> repository -> MySQL -> Eino ADK`；
- 内部 `CreateTaskThread` use case 仍服务 canonical initial submit、Scheduled Task
  和飞书入口，它不是已删除的 TaskThread V1 HTTP 合同；
- Scheduled Task 合同仍由 `idl/workbench/task.thrift` 拥有；
- stateless Run adapter 标记为 `retained_zero_use_gate_blocked`；
- 现行图不再要求 TaskThread V1、本地 LangGraph Thread、运行时 selector、fallback、
  browser EventSource 或 feature gate。

最终 machine graph 为 115 nodes、132 edges、28 chains、9 queries、9 exclusions，
profile digest 为
`2dabe0e07a562c2cc18755d27a1a9ef6e1eba292f51221c7ddb7686e6123b50c`。

## Residual Risk And Next Gate

- `/api/runs/**` 仍是显式兼容面；缺少网关访问日志和外部 consumer registry 证据前
  不删除。后续如补齐五项零使用审计，应单独立项并重新执行 10-route 删除门禁。
- 在线环境没有自然构造所有 destructive Artifact/Memory、resume/retry、429 和依赖
  故障场景；本文逐项标明 deterministic substitute，不把未观察场景写成页面实测。
- 当前服务继续运行在 `http://localhost:8080` 和 `http://localhost:8888`，方便用户
  复核页面。
- 下一步仅进入 Task 17 的第一次 `dev` 集成审计。审计报告形成后必须停在第一次
  明确确认点，不能把此前的“继续自动完成”解释为合并授权。
