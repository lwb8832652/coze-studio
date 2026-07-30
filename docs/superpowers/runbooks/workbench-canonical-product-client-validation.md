# Workbench Canonical Product Client Validation

更新时间：2026-07-30
状态：Gate B 后唯一有效验证手册

## 目的

本手册验证 Workbench、任务列表和任务详情已经完整使用
`/api/workbench/threads/**`，且切换没有改变现有产品行为。它同时验证旧 Thread
路由不可达、Scheduled Task 未被误删、stateless `/api/runs/**` 保持当前受控状态，
以及认证、租户隔离、SSE、错误映射和日志脱敏满足生产边界。

本手册不提供旧 client 回退步骤。出现失败时停止发布并修复 canonical 链路，不得
恢复 `/api/workbench/task_threads/**`、`/api/threads/**` 或 ChatTask。

## 固定事实

| 路由面 | 数量 | 预期状态 | 所有者 |
| --- | ---: | --- | --- |
| `/api/workbench/threads/**` | 47 | 注册且 always-on | `idl/workbench/thread.thrift` |
| `/api/workbench/task_threads/**` | 36 | 全部不可达 | 已退役 |
| `/api/threads/**` | 23 | 全部不可达 | 已退役 |
| Scheduled Task | 11 | 保留 | `idl/workbench/task.thrift` |
| stateless `/api/runs/**` | 10 | 暂时保留 | zero-use gate blocked |

路由数量和 method/path 对由
`backend/api/router/coze/workbench_canonical_thread_route_test.go` 固定。任何数量变化
都必须先解释公共合同变化，不能只修改表格或测试快照。

## 前置条件

1. 使用待发布分支的最新提交，工作区没有来源不明的改动。
2. 后端使用目标线上 MySQL 配置；只核对 host 分类和连通性，不打印 DSN、密码或
   session key。不得为通过验证而启动或改写本地 MySQL。
3. Redis、对象存储、Milvus 等依赖按目标环境配置；关键安全依赖缺失时必须 fail
   closed。
4. 按 `docs/superpowers/runbooks/local-debug-and-test.md` 使用测试账号和有效工作空间。
5. 后端监听 `http://localhost:8888`，前端监听 `http://localhost:8080`；端口冲突时
   选择空闲端口并在证据中记录。
6. 浏览器验收默认使用 Codex in-app browser，并保留同一登录会话。

推荐启动顺序：

```bash
cd bin
APP_ENV=debug ./opencoze -start
```

```bash
cd frontend/apps/coze-studio
rushx dev
```

启动后先确认：

- 未认证访问受保护资源返回 `401`；
- 前端首页返回 `200`；
- 后端日志显示目标数据库连接成功，但不包含任何 credential；
- 必需依赖不可用时页面和 API 显式报错，没有内存或未授权执行回退。

## 路由门禁

运行精确路由快照：

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-route-cache \
  go test -p 1 -gcflags='all=-l -N' ./api/router/coze \
  -run '^TestWorkbench(CanonicalThreadRoutes|StatelessRunRouteResolution)$' \
  -count=1
```

必须同时满足：

- 47 条 canonical method/path 对精确匹配；
- 36 条 TaskThread V1 method/path 对都不能进入 handler chain；
- 23 条本地 LangGraph Thread method/path 对都不能进入 handler chain；
- 11 条 Scheduled Task method/path 对完整保留；
- 10 条 stateless Run method/path 对完整保留，静态路径不会误匹配动态 `:run_id`；
- `/api/workbench/chat` 和 `/api/workbench/tasks` 仍不可达；
- 未定义的 POST stream/join 变体不能被动态路由误接收。

生成 schema 还必须与 IDL 保持一致：

```bash
cd frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
```

## 认证探针

使用权限为 `0600` 的临时 cookie jar。账号和密码只从本地调试手册或安全环境变量
读取，不进入命令历史、证据文档和日志。

所有受保护的 canonical 请求必须带：

- 登录后 session cookie；
- 当前工作空间的 `X-Coze-Space-ID`；
- JSON 写请求的正确 `Content-Type`；
- POST 重试场景的唯一 `Idempotency-Key`。

至少验证以下结果：

| 场景 | 预期 |
| --- | --- |
| 无 session | `401` |
| 无效或无权限 workspace | `403` 或资源隔离错误 |
| 不属于当前 workspace 的 Thread/Run | `403` 或 `404`，不得泄露归属 |
| 不存在的资源 | 稳定 `404` |
| 同幂等键、同 payload | 回放原 Run/Message，不重复写入 |
| 同幂等键、不同 payload/operation | 稳定 `409` |
| 无效参数、非法状态、超限 body | 对应 `400/413/422` |
| 服务端错误 | 稳定 `500`，响应和日志不含内部载荷 |
| 限流 | `429`，前端显示可恢复错误且 telemetry 不含正文 |

探针只使用测试数据。不得将线上 credential、真实用户正文、工具参数或对象存储
地址写入验证材料。

## 浏览器功能矩阵

使用有效 workspace 完成下列操作，每项记录 URL、Thread ID、Run ID、可见状态和
控制台结果；不得记录消息全文或附件内容。

| 区域 | 必测行为 |
| --- | --- |
| 任务列表 | 首次加载、状态筛选、搜索、刷新、空态、错误态 |
| 无附件创建 | 提交、流式进度、终态、标题更新、刷新后保留 |
| 单附件创建 | 选择、上传、移除、重新上传、执行 |
| 多附件创建 | 一次选择多个文件、上传完成后只创建一个 Run |
| 任务详情 | 消息、步骤、建议、Token Usage、详情面板 |
| Follow-up | 当前轮提交、附件先上传、只创建一个顶层 Run |
| Cancel | 运行中停止，刷新后仍显示取消状态 |
| Resume/Retry | 有资格的数据上创建新 Run，不改写历史 Run |
| Artifact | 列表、内容/签名 URL、删除、恢复、扫描审核/重试 |
| Memory | 列表、更新、删除、恢复、清空、导入导出、审计 |
| 诊断 | Guardrail、MCP Runtime、依赖正常/未知/不可用状态 |
| 隔离 | 不存在资源与无权限 workspace 都不能展示数据 |

没有可操作记录的 destructive 场景，可用确定性 handler/component 测试补充，但必须
在证据中明确“页面未构造该数据”，不能伪称完成页面操作。

## 网络与 SSE

正常 UI 工作流只允许访问 `/api/workbench/threads/**` 以及与任务无关的既有产品
接口。不得出现：

- `/api/workbench/task_threads/**`；
- `/api/threads/**`；
- `/api/workbench/tasks/**` 或 `/api/workbench/chat`；
- 同一次用户提交产生两个 Thread、两个顶层 Run 或两条流连接；
- 页面 service 绕过 `canonicalThreadClient` 自行创建流连接。

SSE 必须验证：

1. create-stream 只创建一个 Run 和一条 User Message；
2. reconnect 使用 `Last-Event-ID` 或 `after_event_id` 续传，不重复投影事件；
3. 终态前完成最后一次持久化事件 flush；
4. 普通完成、超时或 context 结束不触发取消；
5. 只有明确选择且 writer 确认断连时才执行 cancel-on-disconnect；
6. `messages-tuple` 在 wire 上使用 `messages` 事件和二元数组；
7. 浏览器刷新后从持久化 Message/RunEvent 恢复，不依赖内存状态。

## 日志门禁

canonical completion log 应包含足够排障的 bounded metadata：

- `operation`、`route_template`、HTTP method/status、duration、outcome；
- Thread/Run/资源 ID；
- 哈希化 principal 或经审核的 workspace 标识；
- 分页、重放、取消、恢复、扫描等有限状态字段。

日志、响应和 telemetry 禁止出现：

- session key、Cookie、Authorization、credential、token、secret；
- 用户消息正文、prompt、completion、文件内容或文件名原文；
- tool arguments/results、provider body、对象 URI；
- checkpoint bytes、原始 config/context/metadata；
- 原始 `Idempotency-Key`。

登录 handler 也适用同一规则。执行：

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-passport-cache \
  go test -p 1 -gcflags='all=-l -N' ./api/handler/coze \
  -run '^TestPassportWebEmailLoginPostDoesNotLogSessionKey$' -count=1
```

## 确定性验证

前端 canonical source gate：

```bash
cd frontend/apps/coze-studio
rushx test \
  src/pages/tasks/__tests__/canonical-frontend-contract.test.ts \
  src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts \
  src/pages/workbench/thread-client/__tests__/page-service-parity.test.ts
```

canonical client、SSE、错误和 telemetry：

```bash
cd frontend/apps/coze-studio
rushx test src/pages/workbench/thread-client/__tests__
```

后端至少运行 router、canonical handler、应用层和领域层相关包；Mockey 测试统一加
`-gcflags='all=-l -N'`。随后运行 IDL/codegen 校验、`go vet`、后端构建，以及前端
typecheck、lint、build。全量测试若存在基线失败，必须在同一提交的独立基线上复跑，
只有错误集合完全一致才可判定为非本次回归。

执行图与长期事实必须同步：

```bash
node scripts/workbench-execution-graph.mjs verify
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
node --test scripts/workbench-execution-graph.test.mjs
```

## 验收证据

Gate B 证据至少包含：

- 分支、提交 SHA、基线 SHA 和数据库环境分类；
- `47 present / 36 absent / 23 absent / 11 present / 10 retained-blocked`；
- 浏览器矩阵、正常工作流控制台和故意隔离探针结果；
- canonical 网络路径、唯一写入/唯一 SSE 证据；
- request/error/log 脱敏结果；
- 所有执行命令和退出状态；
- 页面无法构造而由确定性测试替代的场景；
- 基线失败对照和未消除的外部依赖风险。

## 停止条件

出现以下任一情况立即停止合并或发布：

- UI 命中任何已退役路由；
- 路由数量或 method/path 快照不一致；
- 同一提交产生重复 Thread、Run、Message 或 SSE；
- workspace 隔离、权限或资源归属校验失败；
- 日志、响应或 telemetry 泄露敏感内容；
- canonical 错误被静默转换为成功或旧链路回退；
- 关键依赖缺失时仍进入未授权执行路径；
- 页面核心流程回归，且无法由同一基线证明为既有问题；
- 执行图、上下文文档或派生图校验失败。
