# Workbench Canonical 产品扩展与双 Client 迁移设计

## 1. 文档状态

- 日期：2026-07-27
- 状态：设计已确认，实施计划已冻结
- 目标分支：`codex/workbench-canonical-product-client`
- 基线：`dev@50afc8beff2273cef5a076478e4fcf0df2004425`
- 上位规范：
  `docs/superpowers/specs/2026-07-26-workbench-thread-api-contract-design.md`
- 已落地核心计划：
  `docs/superpowers/plans/2026-07-26-workbench-canonical-thread-api-core.md`

本文定义 canonical Thread/Run 核心合同之后的下一期工作。范围是补齐当前
Workbench UI 必需的产品扩展，并建立可验证的双 client 边界。旧来源接口在本期
保持不变，默认 UI client 也不切换。

## 2. 当前事实

当前源码已经具备以下边界：

- `/api/workbench/threads` 提供默认关闭的 canonical Thread/Run 核心合同；
- `/api/workbench/task_threads` 仍是当前 Workbench UI 的来源合同；
- `/api/threads` 仍是现有 LangGraph 形状的来源合同；
- `/api/workbench/tasks*` 与 `/api/workbench/chat` 已退役并保持不可达；
- 当前 UI 直接使用 `workbenchTask.CreateTaskThread`、
  `CreateTaskThreadRun`、`GetTaskThread` 等生成 client；
- 上传、Artifact、Token usage、Memory 和 Audit 等产品能力只在
  `/api/workbench/task_threads` 下完整可用；
- canonical 和 TaskThread 来源接口最终都委托同一个
  `agentthread.ApplicationService`、领域服务、MySQL 与 Eino ADK 执行链。

因此，只替换前端核心 URL 会形成混合 client。这样虽然代码改动少，但无法删除
旧来源接口，也不能证明请求、权限和失败语义已经对齐。

## 3. 目标与非目标

### 3.1 本期目标

1. 在 `/api/workbench/threads` 下补齐当前 UI 使用的产品扩展。
2. 产品扩展继续调用现有应用服务，不建立第二套状态机、持久化或执行器。
3. 前端增加稳定的 `WorkbenchThreadClient`，隔离 transport DTO 与页面模型。
4. 同时实现 `TaskThreadV1Client` 和 `CanonicalThreadClient`。
5. 旧 client 与 canonical client 经 adapter 后产生等价页面模型和用户结果。
6. canonical 写请求不双写，失败时不自动改走旧来源接口。
7. 用后端合同测试、前端 fixture 和真实页面回归证明迁移条件是否满足。

### 3.2 本期非目标

- 不删除或修改 `/api/workbench/task_threads`；
- 不删除或修改 `/api/threads`；
- 不恢复 ChatTask route、IDL、client、application/domain 或 fallback；
- 不默认切换生产 UI 到 canonical client；
- 不开放外部 API key、Bearer、scope 或生产 allowlist；
- 不实现分布式限流、网关 SSE 或外部容量隔离；
- 除 suggestions 最近公开消息查询的幂等索引迁移外，不新增数据库表、数据复制或双写；
- 不修改 Eino ADK、Worker、Run 状态机或 checkpoint bytes；
- 不重写 Workbench 和 Tasks 页面组件、store 或状态管理。

## 4. 总体架构

```text
Workbench / Tasks UI
  -> existing page service exports
  -> WorkbenchThreadClient
       -> TaskThreadV1Client
       -> CanonicalThreadClient
  -> generated client or reviewed fetch/SSE transport
  -> Hertz canonical handler
  -> agentthread.ApplicationService
  -> existing domain services and repositories
  -> MySQL / object storage / Eino ADK
```

`WorkbenchThreadClient` 是前端唯一切换点。React 组件不读取 feature flag，也不
判断当前合同类型。后端 canonical handler 是 HTTP 适配器，只负责认证上下文、
严格校验、公开投影、错误映射和结构化日志。

旧 handler 与 canonical handler 可以调用同一个应用用例，但 canonical handler
不得直接调用旧 HTTP handler。后者包含旧绑定类型、Coze envelope 和旧日志语义，
直接复用会把待删除合同带入新接口。

## 5. IDL 设计

### 5.1 文件边界

- `idl/workbench/thread.thrift`：保留 canonical 核心 DTO 和统一 service；
- `idl/workbench/thread_product.thrift`：新增产品扩展 DTO；
- `idl/workbench/task.thrift`：保持不变，不被 canonical IDL include；
- `idl/workbench/thread.thrift` include `thread_product.thrift`，并在现有
  `WorkbenchCanonicalThreadService` 中声明扩展方法。

这样既保留单一生成 client，又避免 `thread.thrift` 被产品 DTO 淹没。未来删除
`task.thrift` 中的 TaskThread 来源方法时，canonical 合同不会产生反向依赖。

### 5.2 类型规则

- HTTP 层的 Thread、Run、Message、File、Artifact、Memory 和 Event ID 都输出
  十进制字符串，Go 应用层继续使用 `int64`；
- canonical 时间输出 RFC 3339 字符串，adapter 转换为当前页面需要的时间值；
- metadata 在 IDL 中声明为 JSON value，并在 handler 投影前执行敏感字段过滤；
- list 响应统一包含资源数组、`total` 和 `has_more`，cursor 可用时额外返回
  `next_cursor`；
- command 成功返回目标资源或操作摘要，纯删除成功返回 `204`；
- canonical 响应不使用 `{code,msg,data}` 业务 envelope；
- 错误继续使用现有 canonical HTTP error 结构；
- Token usage 不返回 provider 原始 usage body；
- signed URL 只在当前响应中出现，不进入日志、Message、SSE 或持久化 metadata。

## 6. Canonical 产品路由

现有 core 路由不变。本期新增下列产品路由，全部位于
`/api/workbench/threads/:thread_id`。

### 6.1 Message 与建议

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/messages` | 内部兼容追加；普通 UI turn 禁止使用 |
| `POST` | `/suggestions` | 使用安全消息投影生成建议，失败不阻断主流程 |

普通追问继续调用 `POST /runs`，由服务端原子写入 User Message 和 Run。

### 6.2 Upload

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/uploads` | 列出 Thread 已登记上传 |
| `POST` | `/uploads` | multipart 上传并绑定 Thread |
| `DELETE` | `/uploads/:file_id` | 按稳定 ID 删除允许删除的上传 |

canonical URL 不接受 filename。现有 filename 删除路由保持原样；应用层需要增加按
`file_id` 读取并删除的窄用例时，应复用当前 ownership、引用检查和对象存储清理。

### 6.3 Artifact 与扫描

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/artifacts` | 按 Run、删除状态和分页筛选 |
| `GET` | `/artifacts/:artifact_id/content` | 返回审核后的预览或文本内容 |
| `GET` | `/artifacts/:artifact_id/signed_url` | 返回短时预览或下载地址 |
| `DELETE` | `/artifacts/:artifact_id` | 复用现有软删除和保留策略 |
| `POST` | `/artifacts/:artifact_id/restore` | 恢复允许恢复的 Artifact |
| `POST` | `/artifacts/:artifact_id/scan_review` | 授权人员执行扫描裁决 |
| `GET` | `/artifact_scan_jobs` | 列出扫描任务 |
| `POST` | `/artifact_scan_jobs/:job_id/retry` | 幂等重试扫描任务 |

content、signed URL 和扫描裁决继续执行现有 Thread/space ownership、扫描状态、大小、
类型和角色检查。

### 6.4 Token usage

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/token_usage` | 查询 Thread、Run 或子 Run 的安全用量投影 |

筛选项保留 `run_id`、`include_child_runs`、`source`、`limit` 和 cursor/offset 等价
能力。公开结果包含聚合和允许展示的明细，不输出 `raw_usage`。

### 6.5 Memory

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/memories` | 按 Run、scope、关键词和状态查询 |
| `PUT` | `/memories/:memory_id` | 更新允许字段并记录审计 |
| `DELETE` | `/memories/:memory_id` | 软删除 Memory |
| `POST` | `/memories/:memory_id/restore` | 恢复 Memory |
| `POST` | `/memories/clear` | 按 Run/scope 清理并审计 |
| `GET` | `/memories/export` | 返回脱敏导出对象 |
| `POST` | `/memories/import` | 限量校验并导入 |
| `GET` | `/memories/audit_events` | 查询 Memory 审计事件 |

Memory import、update、delete、restore 和 clear 都属于写操作，不能 shadow write，
也不能在 canonical 失败后改走来源接口。

### 6.6 Guardrail 与 MCP Audit

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/guardrail_audit_events` | 查询 Guardrail 审计事件 |
| `GET` | `/guardrail_audit_events/export` | 返回脱敏导出对象 |
| `GET` | `/mcp_runtime_audit_events` | 查询 MCP 运行审计事件 |

审计导出继续执行独立权限检查。credential、tool 参数/结果、provider body 和隐藏配置
不得进入响应。

### 6.7 子智能体 Retry

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/runs/:run_id/retry` | 只重试允许重试的子智能体 Run |

顶层失败重试仍通过 `POST /runs` 创建新顶层 Run。两种 retry 不得合并为一个含糊
动作。

## 7. 后端实现边界

### 7.1 Handler 分组

手写 handler 按职责拆分，避免单个文件承载全部扩展：

- `workbench_canonical_message_product_service.go`
- `workbench_canonical_upload_service.go`
- `workbench_canonical_artifact_service.go`
- `workbench_canonical_memory_audit_service.go`
- `workbench_canonical_usage_retry_service.go`

文件名可以在实施计划中按现有生成约束微调，但所有 handler 必须复用：

- `requireCanonicalAPI`；
- session principal 与 canonical space 解析；
- `workbenchThreadAccessContext`；
- canonical 请求体大小限制和严格 JSON；
- canonical error、trace、request completion log；
- 公开 metadata、错误和资源投影规则。

### 7.2 应用层与领域层

优先直接调用现有 `agentthread.ApplicationService` 能力。允许的增量仅限现有 HTTP
语义不能安全表达的窄用例，例如按稳定 `file_id` 删除上传。新增方法必须满足：

- 仍以服务端认证空间和用户为准；
- 在应用层完成 ownership 和操作编排；
- 复用现有 domain service、repository 和事务；
- 不为 canonical 建立独立表、状态字段或 worker 分支；
- 不改变旧方法的默认值和调用结果。

### 7.3 禁止的复用方式

- canonical handler 调用旧 Hertz handler；
- canonical IDL include `task.thrift` DTO；
- 用内部 HTTP 请求转发到 `/task_threads`；
- 将旧 `{code,msg,data}` envelope 原样作为 canonical 响应；
- 为少写代码而信任客户端 `space_id`、`user_id` 或 owner；
- 在 handler 中复制状态机、Artifact 保留策略或 Memory 审计规则。

## 8. 前端 Client 设计

### 8.1 文件与所有权

在 `frontend/apps/coze-studio/src/pages/workbench/thread-client` 建立共享边界：

```text
types.ts
workbench-thread-client.ts
task-thread-v1-client.ts
canonical-thread-client.ts
client-selector.ts
adapters/
```

`pages/workbench/service.ts` 和 `pages/tasks/service.ts` 保留现有导出名称，内部改为
委托 `WorkbenchThreadClient`。这样页面组件、hooks、store 和测试 mock 可以分批迁移，
不会在组件树中出现合同分支。

### 8.2 Client 接口

接口覆盖当前页面实际调用的能力：

```text
searchThreads / createThread / getThread
listMessages / generateSuggestions
listRuns / createRun / cancelRun / resumeRun / retrySubagentRun
listRunEvents / streamRun
listUploads / uploadFiles / deleteUpload
listArtifacts / getArtifactContent / getArtifactSignedURL
deleteArtifact / restoreArtifact / reviewArtifactScan
listArtifactScanJobs / retryArtifactScanJob
getTokenUsage
listMemories / updateMemory / deleteMemory / clearMemories / restoreMemory
importMemories / exportMemories / listMemoryAuditEvents
listGuardrailAuditEvents / exportGuardrailAuditEvents
listMCPRuntimeAuditEvents
```

接口返回 app-owned view model，不返回 `workbenchTask.*Response` 或
`thread_contract.*Response`。两种 transport 的 adapter 都在 client 目录内。

### 8.3 Client 选择

- 本期生产 selector 固定返回 `TaskThreadV1Client`；
- 单元测试通过依赖注入选择 client；
- 构建配置统一命名为 `WORKBENCH_THREAD_CLIENT_MODE`，只接受 `v1` 或
  `canonical`，省略时为 `v1`；
- 本地 canonical 页面回归显式设置
  `WORKBENCH_THREAD_CLIENT_MODE=canonical`，不使用 URL 参数或 localStorage 暗门；
- 本期生产构建和 CI 发布任务必须拒绝 `canonical` 值；
- selector 只在一次页面会话开始时确定，活动请求期间不切换；
- 下一期灰度接入真实环境/空间/用户稳定分桶时，只替换 selector policy；
- 任何前端选择都不能替代服务端鉴权和 canonical feature gate。

### 8.4 写入规则

- 一次用户动作只调用一个 client；
- 不做 write shadow；
- canonical 写失败后不调用 TaskThreadV1Client 重写；
- 结果不明确时使用同一个 `Idempotency-Key` 查询或重放 canonical 请求；
- 开关回滚只影响后续动作，已有 Thread/Run ID 不转换、不复制；
- 上传后的首个 Run、普通追问、resume 和 retry 都保留当前原子和 attempt 语义。

## 9. 关键数据流

### 9.1 无附件首次提交

UI 调用 `createThread`，canonical body 使用 `coze.initial_run`。服务端复用
`CreateTaskThread` 原子用例创建 Thread、User Message 和 Run。响应经 adapter 立刻
显示新任务，不等待列表轮询补齐。

### 9.2 有附件首次提交

1. `createThread` 使用 `coze.deferred_initial_run` 创建并校验 draft Thread；
2. `uploadFiles` 按 Thread 绑定上传，返回稳定 `file_id`；
3. `createRun` 引用上传描述符并原子创建首个 User Message 和 Run；
4. 上传失败只重试上传，Run 结果不明确只按同一幂等键处理。

前端不得通过旧“追加 Message + 创建 Run”组合模拟 canonical 写入。

### 9.3 普通追问

UI 只调用 `createRun`。服务端验证当前 Thread、活动 Run 策略和附件引用后，原子创建
User Message 与顶层 Run。

### 9.4 SSE、Resume 与 Retry

- SSE 明确绑定当前 `run_id`；
- 每个 Run 维护自己的持久化 cursor；
- resume 或 retry 返回新 Run 后，关闭来源 Run 的活动流；
- 来源 Run 的晚到事件不能更新新 attempt；
- 断线后先回放 cursor 之后的持久化事件，再接入 live stream；
- 终态由事件和终态读取共同校准，但页面只完成一次终态提交。

## 10. 错误、安全与日志

### 10.1 错误语义

canonical 产品扩展沿用现有 canonical error：HTTP 状态表达类别，body 只包含审核后的
`type`、`code`、`detail`、`request_id` 等公共字段。应用错误继续通过统一映射处理，
不把 SQL、对象存储路径、provider body 或 tool payload 返回前端。

前端 adapter 将 canonical error 转换成当前页面使用的错误对象。错误消息、重试按钮和
空状态由现有组件决定，client 不直接弹 Toast。

### 10.2 身份与权限

- `space_id` 和 `user_id` 不从 canonical body 读取；
- Thread、Run、File、Artifact、Memory 与 Audit 逐层验证同一空间 ownership；
- 不存在或无权访问统一使用防枚举响应；
- Memory 写入、审计导出、Artifact 下载和扫描裁决保留独立权限；
- canonical feature gate 关闭时所有新增路由保持不可用。

### 10.3 日志与遥测

服务端完成日志至少包含：

- `client_contract=canonical_v1`；
- operation、HTTP status、duration、request/trace reference；
- 经审核的 Thread/Run/resource ID；
- 幂等键 hash、分页范围和 SSE 生命周期阶段；
- 失败类别和稳定公共错误码。

不得记录请求正文、Message 内容、Memory 内容、signed URL、credential、tool 参数/结果、
provider body 或原始 usage。

前端遥测只记录 client 类型、operation、ID、耗时、结果类别和 trace reference，不记录
业务正文或下载地址。

## 11. 验证矩阵

### 11.1 IDL 与路由

- IDL lint/codegen 全部通过；
- Go model、Hertz router 和 TypeScript schema 无手工漂移；
- canonical core 与新增产品路由快照精确匹配；
- `/api/workbench/task_threads` 和 `/api/threads` 快照完全不变；
- ChatTask 路径保持未注册且不进入 handler chain。

### 11.2 后端合同与安全

每组产品扩展至少覆盖：

- 成功、空结果、分页和边界值；
- malformed ID、未知字段、超大 body 和错误 content type；
- 未登录、跨空间、资源不属于 Thread 和角色不足；
- application nil/错误、防枚举和安全错误投影；
- 敏感字段和日志脱敏；
- 旧 handler 参数、响应和日志回归。

写接口额外验证幂等、并发、事务失败和审计失败。任何失败都不能产生第二次写入或孤立
Message、Run、File、Artifact、Memory 记录。

### 11.3 前端合同

- 两种 client 的 fixture 经 adapter 后得到相同 view model；
- service 层不泄漏 transport DTO；
- canonical 请求路径、header、body 和 cursor 精确匹配；
- AbortSignal、超时、错误和结果不明确状态一致；
- 生产源码不存在 ChatTask client/fallback；
- 写失败不会触发另一个 client；
- 同一页面只有一个 SSE source 驱动状态。

### 11.4 页面回归

分别在 V1 和 canonical 模式下覆盖：

- Workbench 首页无附件、单附件和多附件提交；
- 上传失败、重试和删除；
- 任务列表、详情、标题和状态；
- 标准追问、建议问题、刷新和切换 Thread；
- SSE 首连、断线、cursor 恢复、未知事件和终态；
- interrupt/resume、cancel、顶层 retry 和子智能体 retry；
- Artifact 列表、预览、下载、删除、恢复和扫描审核；
- Token usage、Memory 管理、导入导出和 Audit；
- 无权限、服务不可用、限流和结果不明确状态；
- 浏览器控制台无新增 error。

页面回归使用同一测试账号、空间和后端数据，比较最终页面和持久化结果，不比较请求字段
是否逐字相同。

## 12. 发布顺序

本期拆成两个连续检查点。检查点 A 只交付后端 canonical 产品扩展；检查点 B 在 A 的
合同、安全和旧接口回归通过后，交付前端双 client 与页面等价验证。两个检查点分别提交，
前端不得在后端合同未冻结时自行猜测 DTO。

具体顺序如下：

1. 新增 IDL 和默认关闭的 canonical 产品路由；
2. 完成后端合同、安全和旧接口回归；
3. 增加双 client、adapter 与默认 V1 selector；
4. 运行双 client fixture 和 canonical 本地页面回归；
5. 保持生产默认 V1，提交本期审计。

后续独立阶段才执行：

1. 接入真实稳定分桶和生产安全依赖；
2. 小比例 canonical 灰度并观察；
3. UI 全量 canonical；
4. 完整观察期和来源流量审计；
5. 分别删除 `/api/workbench/task_threads` 与 `/api/threads`。

## 13. 回滚与停止条件

本期回滚只需关闭 canonical 后端 gate，生产 UI 仍走 V1，不涉及数据回滚。

出现以下任一情况必须停止迁移：

- 权限或防枚举结果不一致；
- 同一用户动作产生重复 Message、Run 或其他写记录；
- Run 状态、取消、resume、retry 或断线结果变化；
- 上传和 Artifact ownership、扫描或下载规则被放宽；
- Memory/Audit 结果缺失、越权或未记录审计；
- adapter 丢失页面使用字段或改变排序、分页、终态；
- canonical 失败后发生来源接口写 fallback；
- 旧来源接口合同或当前 UI 行为出现回归。

## 14. 预计改动范围

手工改动集中在：

- `idl/workbench/thread.thrift`
- `idl/workbench/thread_product.thrift`
- `backend/api/handler/coze/workbench_canonical_*`
- 必要时少量 `backend/application/agentthread` DTO/use case
- `frontend/apps/coze-studio/src/pages/workbench/thread-client`
- `pages/workbench/service.ts` 与 `pages/tasks/service.ts`
- 后端合同测试、前端 adapter/contract 测试和当前事实文档

生成改动包括 Go model、Hertz router/middleware 和前端 API schema。不得手工编辑生成文件。

本期预计不修改：

- 数据库表语义或状态 schema；唯一例外是 suggestions 最近公开消息查询的幂等索引迁移；
- Eino ADK、Worker 和 runtime；
- AgentThread 实体状态机；
- Workbench/Tasks 页面布局和组件行为；
- 已退役 ChatTask 边界。

## 15. 完成定义

只有同时满足以下条件，本期才算完成：

1. canonical 产品扩展覆盖当前 UI 的全部 Thread 资源能力；
2. canonical 与 V1 adapter 的功能等价测试通过；
3. canonical 模式真实页面全流程通过；
4. 旧来源路由、参数、响应和当前默认 UI 完整回归；
5. 除计划内增量索引外，无数据库表语义、执行器、双写或 ChatTask 复活；
6. 安全、幂等、日志、SSE 和失败语义满足本文约束；
7. `dev` 双阶段审计通过并获得对应用户确认。

完成本期不代表可以删除旧接口，也不代表生产 UI 已切到 canonical。删除和默认切换必须
分别满足后续阶段的灰度、观察和流量证据。
