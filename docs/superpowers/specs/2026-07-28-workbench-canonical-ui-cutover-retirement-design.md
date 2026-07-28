# Workbench Canonical UI Cutover And Source Contract Retirement Design

## 1. 文档状态

- 日期：2026-07-28
- 状态：设计已确认，待实施
- 目标分支：`codex/workbench-canonical-ui-cutover-retirement`
- 基线：`dev@1b663df3449f3bd2849b37b33ae2755c109269cd`
- 上位合同：
  `docs/superpowers/specs/2026-07-26-workbench-thread-api-contract-design.md`
- 前置实现：
  `docs/superpowers/specs/2026-07-27-workbench-canonical-product-client-migration-design.md`

本文覆盖 Workbench UI 切换和来源合同退役。它取代前置设计中“本期默认 UI 保持
V1、来源路由不删除”的阶段性约束，但不改变已经冻结的 canonical 请求、响应、
权限、状态机和公开投影语义。

用户已确认本期采用同周期迁移和退役：先完成 canonical 全链路验证，再删除两套
Thread 来源合同，随后在旧路由确实不可达的状态下重新验证。无状态
`/api/runs/**` 没有仓库内业务调用方，也进入零使用审计；确认没有外部流量后同步
删除。

## 2. 目标

本期结束后，Thread-bound Workbench 能力只通过
`/api/workbench/threads/**` 提供。Workbench/Tasks UI、仓库内 NewX parity client、
测试和当前文档都改用 canonical 合同。旧路由、旧前端 transport、IDL 方法和
不再使用的 handler 一并删除。最终不保留没有明确调用方、所有者和产品用途的
LangGraph 兼容 route。

底层仍只有一套 `TaskThread -> Message -> Run -> RunEvent` 数据和执行链。此次工作
不改数据库表名、领域模型、Eino ADK、Worker、checkpoint 存储或 Artifact/Memory
业务规则。

## 3. 已核验现状

### 3.1 路由面

| 路由族 | 当前规模 | 当前职责 | 本期处置 |
| --- | ---: | --- | --- |
| `/api/workbench/task_threads/**` | 36 个方法与路径组合 | 当前 Workbench UI 产品合同 | 完整删除 |
| `/api/threads/**` | 23 个方法与路径组合 | 旧 LangGraph 形状的 Thread-bound 合同 | 完整删除 |
| `/api/workbench/threads/**` | 21 个 core + 26 个 product 路由 | canonical 主合同 | 成为唯一 Thread-bound 合同 |
| `/api/runs/**` | 10 个 stateless Run 路由 | 无仓库内业务调用方的一次性执行兼容能力 | 零使用审计后删除 |
| `/api/workbench/tasks*`、`/api/workbench/chat` | 已退役 | ChatTask 旧合同 | 继续保持 `404` |

`/api/workbench/task_threads/**` 包含 Thread 列表、创建、详情、Message、Run、
RunEvent/SSE、取消、恢复、重试、Upload、Artifact、扫描、Token usage、Memory 和
Audit。删除前必须证明这些产品能力已逐项由 canonical 页面链路使用。

`/api/threads/**` 包含 Thread CRUD、state/history、checkpoint readiness、Thread-bound
Run、wait、stream、join、cancel、Message 和 Event。它与 `/api/runs/**` 在同一手写
LangGraph handler 区域中注册。两组路由都通过删除门禁后，可以清理整个旧
LangGraph HTTP adapter；被其他模块使用的通用 helper 仍需按调用图保留。

### 3.2 当前调用方

- Workbench 和 Tasks 页面直接使用 `workbenchTask` 生成 client，并在部分服务中手写
  `/api/workbench/task_threads/**` fetch、multipart 和 EventSource。
- `backend/internal/deerflowparity/newx_client.go` 仍调用本项目的 `/api/threads/**`，
  是仓库内必须迁移的真实调用方。
- `backend/application/skill/builtin_deerflow/**` 中的 `/api/threads/**` 指向外部
  DeerFlow gateway，不是本项目来源路由，不能因本次退役机械替换。
- 当前生产源码未发现其他本项目 `/api/threads/**` 调用方。历史设计文档中的示例
  不构成运行时调用，但需要更新现状说明或保留明确的历史标记。
- 当前生产源码也未发现 `/api/runs/**` 调用方。搜索结果只有路由注册、handler、
  测试和历史文档，因此仓库内零调用条件已经满足；仓库外流量仍需访问日志证明。

### 3.3 共同主链

三套 HTTP adapter 最终都调用现有 `agentthread.ApplicationService`，并共用领域服务、
repository、MySQL、对象存储和 Eino ADK。canonical 已经具备当前 UI 所需的 core 与
product 路由，因此本次切换不需要复制数据，也不需要建立第二套执行器。

## 4. 最终架构

```text
Workbench / Tasks UI
  -> app-owned page requests and view models
  -> CanonicalThreadClient
  -> /api/workbench/threads/**
  -> canonical Hertz handlers
  -> agentthread.ApplicationService
  -> existing domain / repository / MySQL / Eino ADK

NewX parity client
  -> /api/workbench/threads/**
```

最终生产代码不保留 `TaskThreadV1Client`、client selector、读写 fallback、shadow
request 或双 SSE。canonical 请求失败时直接向页面返回原始失败语义；结果不明确时
依靠幂等键和同一 canonical 资源确认结果，不能改走旧路由重放写请求。

## 5. 合同迁移规则

### 5.1 Workbench V1 产品合同

页面动作映射到 canonical 的既有原子边界：

| 页面动作 | 最终 canonical 操作 |
| --- | --- |
| 首次提交，无附件 | `POST /api/workbench/threads` 的 `coze.initial_run` |
| 首次提交，有附件 | 创建 deferred Thread，上传后 `POST .../runs` |
| 追问 | `POST .../runs`，服务端原子写 User Message 与 Run |
| 详情与历史 | Thread、Message、Run、Run Event 查询 |
| 实时事件 | 绑定具体 Run 的 `GET .../runs/:run_id/stream` |
| 取消与恢复 | `POST .../cancel`、`POST .../resume` |
| 顶层失败重试 | 新建顶层 Run，保留来源 Run |
| 子智能体重试 | `POST .../runs/:run_id/retry` |
| 文件与产物 | canonical Upload、Artifact 和扫描路由 |
| 用量、记忆与审计 | canonical product 路由 |

页面操作顺序、文案、状态机、列表排序和展示模型保持不变。transport 的 RFC 3339
时间、直接响应、字符串 ID 和 canonical error 只在 client/adapter 边界转换。

### 5.2 旧 LangGraph Thread 合同

`/api/threads/**` 与 canonical 不是字节兼容关系，迁移按功能语义执行：

- Thread CRUD、search、state、history、Message、Run、wait、cancel 和事件使用
  canonical 对应路由；
- 旧 `GET .../checkpoints/:checkpoint_id/resume` 不在新合同中复刻，恢复统一使用
  `POST .../runs/:run_id/resume`；
- 旧 Run-specific `POST .../stream` 和 `POST .../join` 不迁移，canonical 固定使用
  `GET .../stream` 和 `GET .../join`；
- 旧 busy Thread 删除行为不保留，canonical 继续返回 `409 thread_busy`；
- 旧请求中由客户端提交的 `space_id`、owner 或 user identity 不再作为授权事实，
  workspace 来自 `X-Coze-Space-ID` 和服务端认证上下文。

仓库内 NewX parity client 必须改用 canonical path、header、SSE mode 和直接响应，
并继续通过现有登录 session 鉴权。它不能新增旧路由 fallback。

### 5.3 Stateless Run 零使用审计与退役

`/api/runs/**` 是 2026-06-17 引入的 LangGraph 一次性执行兼容入口。所谓 stateless
只表示客户端不预先管理 Thread，服务端仍会创建 backing Thread 和持久化 Run。它
不属于当前 Workbench 页面链路，canonical 也没有承诺同形状的 stateless API。

删除前必须同时满足：

1. 生产源码除 route/handler 外没有调用方，当前审计已满足；
2. 前端、内部工具、IM、计划任务、生成 client 和运维脚本没有调用方；
3. 网关或服务访问日志中 10 个 `/api/runs/**` 路由没有有效业务流量；
4. 没有登记的外部 SDK 客户、公开文档承诺或具名业务所有者；
5. 相关能力不在当前发布验收矩阵中，也没有 canonical 迁移需求。

五项满足后，删除全部 10 个路由、stateless handler、专用 binding、测试和只被它们
使用的 helper。如果访问日志发现调用方，应先识别所有者并决定迁移到
Thread-bound canonical Run 或明确保留；不能把“源码没有引用”当作外部零流量。

## 6. 前端设计

### 6.1 App-owned 边界

在 `frontend/apps/coze-studio/src/pages/workbench/thread-client` 建立应用层请求、资源、
错误、分页和 SSE 类型。React 组件不导入 `workbenchTask` 或 `workbenchThread`
transport DTO，也不读取合同模式。

`CanonicalThreadClient` 负责：

- canonical JSON、multipart、Blob 和 SSE transport；
- `X-Coze-Space-ID`、`Idempotency-Key`、same-origin credential 和请求中止；
- direct response、RFC 3339 时间、分页 header 和 canonical error 转换；
- 每个 Run 独立的 SSE cursor 与生命周期；
- 只记录合同名、操作、资源 ID、耗时、结果和 trace reference 的脱敏日志。

现有 `pages/workbench/service.ts`、`pages/tasks/service.ts` 和 Memory/usage helper 保留
页面需要的导出名称，但内部只委托 canonical client。兼容 presenter 只负责把
app-owned model 变成现有页面结构，不能发 HTTP 请求。

### 6.2 临时等价验证与最终清理

功能分支前半段允许实现测试专用的 V1 adapter，用它冻结旧 wire contract，并与
canonical adapter 比较页面模型和用户结果。通过删除门禁后，该 adapter、selector、
V1 fixture 和运行时模式一并删除。最终 release build 只有 canonical client。

`WORKBENCH_THREAD_CLIENT_MODE` 不进入最终产品配置。原计划中的 release canonical
拒绝逻辑也应删除，因为 canonical 已成为唯一生产合同。

## 7. 后端与 IDL 设计

### 7.1 Canonical 成为主路由

最终 `/api/workbench/threads/**` 不再依赖默认关闭的迁移 gate。删除
`COZE_WORKBENCH_CANONICAL_API_ENABLED` 的请求级开关及相关 404 分支，保留认证、
workspace authorization、严格 JSON、大小限制、公开投影、错误映射和结构化日志。

此处不增加运行时回退开关。若发布后需要回退，重新部署删除前的版本。

### 7.2 删除 TaskThread V1 HTTP 合同

- 从 `idl/workbench/task.thrift` 删除 TaskThread 来源 service 方法和只被这些方法使用
  的 HTTP DTO；Scheduled Task 等同文件其他能力保留；
- 重新生成 Hertz router/model 和前端 API schema；
- 删除 `/api/workbench/task_threads/**` 的生成路由、自定义 Upload/Artifact/SSE 路由、
  middleware 和 handler；
- 只删除无调用的 HTTP projection/helper。canonical 共用的 application、domain、
  repository、entity 和业务校验继续保留。

### 7.3 删除 LangGraph Thread 与无状态 Run 合同

- 从 `registerLangGraphCustomRoutes` 删除 `/api/threads/**` 注册；
- 在零使用审计通过后删除 `/api/runs/**` 注册；
- 删除只服务这两套旧合同的 handler、binding、projection、测试和路由注册函数；
- 用调用图核对 `langgraph_run_service.go`、`langgraph_thread_service.go` 和
  `backend/api/model/agent/langgraph`，整文件无调用时删除，仍被 canonical 或其他业务使用
  的通用代码按现有所有权移动或保留；
- 把两套路由快照测试改为负向不可达测试。

## 8. 数据、安全与日志

- 不执行数据库迁移、数据复制、ID 转换或历史 Message/Run 重放；
- UI 与 parity client 使用同一存量 Thread、Run、Message、Artifact 和 Memory；
- 线上回归创建的记录使用明确测试标题，不批量清理线上数据；
- session principal 和 `X-Coze-Space-ID` 是 Workbench canonical 的授权边界；
- client 和 server 日志不得记录正文、Memory 内容、附件名、signed URL、credential、
  provider body、tool 参数/结果或 checkpoint bytes；
- `/api/workbench/tasks*` 和 `/api/workbench/chat` 的防复活测试继续保留。

## 9. 双门禁实施顺序

### 9.1 Gate A：迁移验证，旧路由暂存

1. 建立 app-owned client、canonical transport、adapter、SSE 和页面 service 委托。
2. 用当前 V1 请求快照和成对 fixture 验证 transport 差异不会改变页面结果。
3. 运行前端完整相关测试、lint、build 和后端 canonical Checkpoint A。
4. 使用同一线上 workspace 分别验证旧页面链路和 canonical 页面链路。
5. 核对 canonical 模式没有 `/api/workbench/task_threads`、`/api/threads`、fallback、
   双写或第二条 SSE。
6. 完成 `/api/runs/**` 的源码、调用图、生成 client、运维脚本和访问日志审计。

Gate A 任一项失败，停止删除并修复。失败期间旧来源路由保持原样。

### 9.2 Gate B：物理删除后验证

Gate A 全部通过后再执行删除：

1. 切为 canonical-only 前端并迁移 NewX parity client。
2. 删除两套 Thread 来源 route、IDL 方法、生成 client、handler、fixture 和兼容测试。
3. `/api/runs/**` 零使用审计通过时，删除其 10 个路由及专用实现。
4. 验证 36 个 TaskThread V1 路由、23 个 LangGraph Thread 路由，以及已确认无用的
   10 个 stateless Run 路由全部 `404`。
5. 验证 47 个 canonical 路由仍注册。
6. 重跑 codegen、Go 测试、前端合同/页面测试、lint 和 release build。
7. 在 canonical-only 最终产物上重复线上页面回归，检查请求日志和控制台。

Gate B 通过前不得合入 `dev`。删除后发现任何页面、权限、Run 状态、SSE、Upload、
Artifact、Memory 或 Audit 回归，应在功能分支恢复到 Gate A 的已验证提交并修复，
不能临时加 URL fallback。

## 10. 页面回归矩阵

最终 canonical-only 页面至少验证：

- 登录、workspace 隔离、任务列表、详情、标题和状态；
- 无附件、单附件、多附件首提，上传失败、重试和删除；
- 流式回复、刷新持久化、Thread 切换、断线重连和终态；
- 追问、建议、取消、恢复、顶层重试和子智能体重试；
- Artifact 预览、下载、删除、恢复、扫描与扫描重试；
- Token usage、Memory 导入导出、编辑、删除、恢复、清理与 Audit；
- 未授权、资源不存在、限流、服务不可用和结果不明确状态；
- 页面控制台无新增错误，服务端日志只出现 canonical Thread 路由。

不能稳定构造的 human interrupt、扫描失败或网关故障场景由自动化合同测试补足，
并在最终报告中明确哪些是页面实测、哪些是测试替代证据。

## 11. 回滚

本期按用户确认取消来源合同的长期并行观察期，因此最终版本没有运行时 V1 回滚。
发布回滚方式是重新部署删除前的已验证版本。三套 adapter 共用同一持久化事实，
回滚不需要数据库回滚，也不得删除 canonical 创建的 Thread 或 Run。

如果请求结果不明确，客户端使用同一幂等键查询 canonical 结果。禁止为了“确认是否
成功”调用旧来源再次创建 Message、Run、Upload 或 Memory 写入。

仓库内审计能证明当前源码调用方已迁移，但不能证明仓库外未知客户端不存在。若无法
取得生产网关或访问日志，最终审计必须把这一点列为残余风险，不能把页面回归等同于
外部调用方流量证明。

## 12. 验收标准

1. Workbench/Tasks UI 只拥有一个 canonical client，页面无 transport DTO。
2. 首提、追问、取消、恢复和重试保持当前持久化与状态语义，无双写。
3. 一个活动 Run 只拥有一个 SSE source，旧 Run 的迟到事件不能更新新 attempt。
4. 线上 canonical-only 页面矩阵通过，页面和服务端没有新增错误。
5. `/api/workbench/task_threads/**` 的 36 个旧路由全部不可达。
6. `/api/threads/**` 的 23 个旧路由全部不可达。
7. `/api/workbench/threads/**` 的 47 个路由保持可用。
8. `/api/runs/**` 通过零使用审计后，其 10 个 stateless 路由全部不可达。
9. 旧 LangGraph HTTP adapter 没有无调用残留；保留代码都有当前调用方和明确所有者。
10. TaskThread application/domain/repository、数据库和 Eino ADK 主链未被删除或复制。
11. ChatTask 旧路由继续为 `404`，没有重新引入 fallback。
12. codegen、相关 Go/TypeScript 测试、lint、build、合同扫描和 diff 审计全部通过。
13. 按仓库 `dev` 双重集成审计执行，合入本地 `dev` 和推送远程分别取得确认。
