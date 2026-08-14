# Workbench Adaptive Execution P1M-A：Canonical 入口冻结设计

**日期：** 2026-08-11
**状态：** 已实施（仅 P1M-A；P1M-B/C 与 P1L 仍待完成）
**预计实施时间：** 1～2 个工作日

## 1. 目标

P1M-A 是完整 P1M 之前的安全切片，只完成两件事：

1. 在 P1L 尚未完成时，临时关闭 canonical HTTP 的整 Thread 删除入口；
2. 冻结 canonical HTTP 对旧产品执行控制的外部输入，同时让第一方前端停止发送这些字段。

P1M-A 不等于 P1M PASS，也不宣称产品执行模式已经退休。后端仍可为兼容目的生成、持久化或读取旧 mode 事实，恢复链和 ADK consumer 保持原行为。

实施入口必须满足两个条件：P0D 已在选定的 exact base SHA 上 PASS；通过 codebase graph 与源码扫描冻结 `DeleteThread` / `DeleteThreadIfIdle` 的全部生产调用方。当前已知网络生产调用方只有 `DeleteCanonicalThread`，旧 `DeleteThread` 没有生产 caller。若实施基线出现其它生产 caller，本包立即停止，改为在 application 边界同时硬关闭两条删除 use case。

## 2. 当前事实

- `DELETE /api/workbench/threads/:thread_id` 仍注册，但 handler 已在 workspace 授权与 path 校验后固定返回 503，不调用 `DeleteThreadIfIdle`。
- idle cascade 的 repository 锁序仍未由 P1L 闭合；active Run/Attempt guard 不能证明 idle cascade 安全。
- canonical Run 的 `config`、`context` 是 opaque JSON；当前后端会从其中解析 `requested_policy`、产品 `mode`、reasoning、Plan 和 Subagent 控制。
- 第一方 Workbench 已停止发送七个外部执行控制字段，并暂时隐藏“模型推理”控件。
- `runtime=eino_adk`、模型选择、Skill、MCP、知识库、数据库、附件、重试和 failover 配置仍是合法公共输入。

## 3. 整 Thread 删除硬关闭

保留现有 route 和 IDL，不增加环境变量或运行时开关。在 `DeleteCanonicalThread` 中使用同文件私有编译期常量表达临时硬关闭。

处理顺序保持为：

1. canonical application dependency 检查；
2. session 与 workspace 鉴权；
3. `thread_id` 格式校验和 request log 上下文建立；
4. 返回临时禁用错误；
5. 不调用 `ApplicationService.DeleteThreadIfIdle`，不进入 domain/repository。

这里故意不读取具体 Thread。对于已通过 workspace 鉴权的合法正整数 ID，存在和不存在的 Thread 都返回同一 503，避免泄露资源存在性。正常 session/workspace 鉴权可以读取成员关系；鉴权后的 guard 不新增数据库 I/O，不调用删除 application/domain/repository，业务数据写入为零。

错误合同固定为：

| 字段 | 值 |
| --- | --- |
| HTTP | `503` |
| `code` | `thread_delete_temporarily_disabled` |
| `detail` | `Thread deletion is temporarily unavailable` |
| `retryable` | `false` |
| internal error class | `thread_delete_disabled` |

错误必须通过现有 `newCanonicalError` 与 `writeCanonicalError` 输出，沿用 canonical wire shape：`trace_id` 由现有 helper 注入，`error_code` 省略。不返回 `Retry-After`，不在公开错误或日志中暴露 P1L、锁序或 deadlock 原因。Artifact、Upload、Memory 等子资源删除不受影响。

该 guard 不能变成可由环境变量开启的长期 feature flag。恢复整 Thread 删除必须删除 guard，并在同一变更中满足第 9 节的解锁条件。

## 4. Canonical 外部执行控制冻结

### 4.1 拒绝字段

canonical HTTP 外部请求不得提交以下产品执行控制：

- `requested_policy`
- `mode`
- `thinking_enabled`
- `reasoning_effort`
- `is_plan_mode`
- `subagent_enabled`
- `max_concurrent_subagents`

以下字段继续合法：

- `runtime=eino_adk`
- 模型 ID/名称与模型 retry/failover
- Skill、MCP、知识库、数据库和 memory retrieval
- web tools、token usage、附件与普通业务 metadata

### 4.2 结构化校验

新增一个 canonical handler 私有 validator。它解析 JSON 结构，但不修改请求，且必须在 typed binder 和任何 application 写入之前执行。

校验只查看审核过的执行配置位置：

- Create Thread：request root，以及 `coze.initial_run` / `coze.deferred_initial_run` 下的 `config`、`context`；
- Create/Wait/Stream Run：request root、`config`、`context`；
- Resume 与 Subagent Retry：request root，原有严格 body allowlist 继续生效；
- 已进入 `config` 或 `context` 后，只沿保留容器名 `configurable`、`context` 继续检查。

validator 不扫描字符串内容，也不递归进入任意业务对象或数组。因此用户消息中出现 `mode`，或合法资源对象里存在其它领域的同名字段，不会被误拒。

当一个请求同时包含多个退休控制时，validator 按本节列出的审核路径顺序和第 4.1 节字段顺序返回第一个命中；不得依赖 Go map 遍历顺序。

结构规则与现有 binder 保持一致：request/container/control key 使用 ASCII 大小写不敏感匹配，错误路径统一输出小写 canonical key；大小写归一化后出现重复 key 时以 `400 invalid_json` 拒绝，不采用不透明的 last-wins。只允许最多 4 层保留容器跳转、64 个被检查对象和 256 字节路径；超过任一界限返回 `422 invalid_request`，且仍发生在 binder/application 之前。请求总体大小继续受现有 canonical body limit 约束。

命中后固定返回：

| 字段 | 值 |
| --- | --- |
| HTTP | `422` |
| `code` | `unsupported_execution_control` |
| `detail` | `Unsupported execution control: JSON_PATH` |
| `retryable` | `false` |
| internal error class | `unsupported_execution_control` |

`JSON_PATH` 使用稳定的 JSON 路径，例如 `config.requested_policy` 或 `coze.initial_run.context.configurable.reasoning_effort`。

### 4.3 覆盖入口

- root 无附件：Create Thread 初始提交；
- root 有附件：deferred initial Run，在 Thread 创建和 upload 之前拒绝；
- follow-up：Create Run；
- top-level retry：Create Run 的 retry form；
- streaming：与 Create Run 共用 submission parser；
- resume、subagent retry：保持现有窄 body 合同，不新增执行控制入口。

P1M-A 的声明严格限定为 **canonical HTTP ingress freeze**。Scheduled Task、IM 或其它直接调用 ApplicationService 的内部入口不在本包中；完整 P1M 后续必须在 typed admission 建立后补 application 层 defense-in-depth。

## 5. 第一方前端行为

Workbench 第一方请求构造器必须与服务端合同一致：

- 删除 `WORKBENCH_REQUESTED_POLICY` 及所有 `requested_policy` config/metadata 镜像；
- `createWorkbenchRunConfig` 不再输出 `reasoning_effort`；
- 隐藏当前 reasoning execution control，避免界面仍提供一个服务端必然拒绝的选择；
- root、附件、follow-up 和 retry 继续复用同一经过清理的 serializer；
- 保持 `runtime=eino_adk`、模型和全部审核后的资源选择不变。

可以暂时保留前端内部 runtime settings 的 reasoning 类型和默认值，前提是 UI 不展示、serializer 不输出；彻底删除这些兼容类型属于 P1M-C。

停止客户端 reasoning 选择是用户批准的临时产品能力收缩：P1M-A 期间 reasoning 由现有服务端默认/兼容逻辑决定。实施时必须同步修订 P1 主计划与当前事实文档，不能继续声称第一方客户端可选择 reasoning 或固定提交 `requested_policy=auto`。

## 6. 数据与兼容边界

- 外部拒绝必须发生在 Thread、Message、Run 写入之前。
- root 附件流程必须在 deferred Thread 创建之前完成校验，因此失败时不会留下 Thread 或上传事实。
- follow-up/retry 失败时，已有 Thread 和来源 Run 保持不变，不新增 Message/Run。
- 服务端 `normalizeNewDeerFlowRunConfig` 仍可基于缺省值写入 server-owned 兼容字段；这不是客户端控制，也不属于本包的退休声明。
- Human resume、lease recovery、journal recovery 和 subagent retry 继续继承旧 Config；P1M-A 不重写历史事实。
- 不改变幂等键、upload-before-run 顺序、Run/Message 原子创建或 SSE 行为。

部署顺序为前端清理先于后端拒绝；旧后端可以接受清理后的请求。后端拒绝上线后，仍缓存旧前端资源的客户端可能收到确定性的 422，这是 fail-closed 预期。若需回滚 ingress freeze，可回滚 validator 与前端清理，但删除 hard guard 必须保留。

## 7. 测试与验收

### 后端

- 删除入口：authorized workspace + 合法 ID 精确返回 503；application 删除调用为 0；原本可删除的 idle Thread 仍存在；响应包含 `trace_id` 且不含 `error_code`/`Retry-After`。
- 删除错误优先级：缺 dependency、缺 workspace header、workspace 拒绝和非法 ID 保持现有错误，不被 503 覆盖。
- 表驱动执行控制测试逐字段覆盖 request root、`config`、`context`、nested `configurable/context`。
- Create Thread immediate/deferred、Create/Wait/Stream Run 各至少一个 route 级拒绝用例。
- Resume Canonical Run 与 Subagent Retry 各有独立 route 用例，精确断言 422、`unsupported_execution_control`、原鉴权优先级和 application 调用为 0。
- 用户消息字符串包含 `requested_policy`/`mode` 仍允许，证明不是文本扫描。
- `runtime=eino_adk` 与合法模型/Skill/MCP/知识库/数据库配置继续通过。
- 每个拒绝用例检查精确 422/code/retryable，并检查相关 Thread/Message/Run 计数不变。
- 现有 lower-layer DeleteThread/DeleteThreadIfIdle 测试保留；它们只证明 dormant implementation，没有解锁生产入口。

### 前端

- serializer 精确断言七个退休字段均不存在；合法 runtime、模型和资源字段仍在。
- root、附件、follow-up、retry 的 config、metadata、message_metadata 均不含退休字段。
- reasoning 控件不再渲染。
- 现有上传、导航、follow-up、retry 和错误状态测试继续通过。

### 工程门禁

- 相关 Go package tests；
- 相关 Vitest；
- 前端 typecheck/lint（按改动范围）；
- `gofmt`、`git diff --check`；
- 因 handler 属 Workbench monitored path，实施时同步 execution chain/graph authority，并运行 graph verify/build/verify-derived。

## 8. 明确非目标

P1M-A 不做以下工作：

- 不定义 `AdaptiveAdmissionSnapshot` 或 `ExecutionDecision`；
- 不安装 baseline/adaptive producer，不接线 `AdaptiveGate`；
- 不修改 ADK、Plan、Journal、Subagent、reasoning inference 或 recovery consumer；
- 不删除后端 `DeerFlowMode` / `DeerFlowRequestedPolicy` 兼容实现；
- 不修改 IDL 或重新生成 Go/TypeScript client；
- 不修改 repository adaptive transaction、Attempt/Plan 锁序或 migration；
- 不实施 P1L，不宣称 repository deadlock 已闭合；
- 不交付 direct/multi-step 自适应判断、progress、verification 或 TaskDetail 新投影。

## 9. 后续解锁条件

- **P1M-B/C：** P1M-A 通过且删除 guard 保持，才开始 typed admission/decision、baseline producer、legacy recovery 和 mode consumer 退休。
- **P1D：** 完整 P1M PASS，且同 Attempt 第二个 Plan-bearing boundary 的 P1R blocker 已关闭；整 Thread 删除继续硬关闭，除非 P1L 已 PASS。
- **恢复整 Thread 删除：** 在移除 guard 的 exact candidate SHA 上重新冻结最终 ingress/writer inventory，覆盖 `DeleteThread` 与真正 idle 的 `DeleteThreadIfIdle` 两条 cascade，并完成真实 MySQL 删除竞态、post-delete cleanup、HTTP 204/409 回归和执行链/图合同更新；全部证据必须绑定同一候选 SHA，不能复用当前 P1L 草稿或旧实现的证据。
- **对外宣称旧字段从公共合同退休：** 完成 P1M-C 的 IDL/生成 client/consumer/recovery 闭环后才允许。

## 10. 预期文件面

设计允许的主要实现面为：

- canonical handler 的 deletion guard、execution-control validator 及现有 handler tests；
- Workbench serializer、reasoning control、new-task/follow-up metadata 及现有 frontend tests；
- Workbench execution chain/graph authority 更新。
- `workbench-chat.md`、`project-context.md` 与 MVP 主计划中的第一方请求事实、P1M-A/B/C 分包和临时 reasoning 收缩。

实施计划必须从真实 diff 反推精确文件列表，不为满足清单制造无关改动，也不修改当前未完成的 P1L 草稿。
