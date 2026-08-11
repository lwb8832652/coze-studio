# Workbench Adaptive Execution P1M-B1：Application Admission Freeze 设计

**日期：** 2026-08-12
**状态：** 已批准，待实施
**范围：** 仅冻结新提交的 application Config/Context；不退休历史 mode consumer

## 1. 目标

P1M-B1 在 P1M-A 的 canonical HTTP 拒绝之外增加 application 层防御：任何通过公开
`ApplicationService.CreateTaskThread` 或 `ApplicationService.CreateRun` 提交的新配置，
都不能携带以下七个客户端执行控制：

- `requested_policy`
- `mode`
- `thinking_enabled`
- `reasoning_effort`
- `is_plan_mode`
- `subagent_enabled`
- `max_concurrent_subagents`

校验必须发生在 runtime normalization、来源 Run 查询和任何持久化之前。canonical、IM、
Scheduled Task 以及未来直接调用公开 application 方法的调用方都受此约束。

## 2. 信任边界

公开 `CreateRun` 固定视为新提交输入，不接受调用者提供的绕过标记。现有
`ApplicationADKSubagentRunRecorder` 仍由服务端生成包含旧执行字段的 child Config；它改走
同 package 私有的 server-owned child 入口。该入口必须同时校验 `ParentRunID > 0` 与
`RunKindSubagent`，不能通过 exported bool、DTO 字段或 context marker 暴露给包外调用者。

Human resume、subagent retry、journal recovery 和 lease recovery 没有新的 Config/Context
输入，它们继续继承数据库中的历史 Config，不经过 admission。top-level retry 属于公开
`CreateRun` 的新请求，因此仍需拒绝七字段。

## 3. 结构化校验

新增 application package 私有 validator，分别审核 `Config` 与 `Context` JSON object：

1. 先按固定七字段顺序检查当前 object；
2. 再按 `configurable`、`context` 顺序递归，最多四次保留容器跳转；
3. key 使用 ASCII 大小写不敏感匹配，错误 path 输出小写 canonical key；
4. 不扫描字符串、数组或其它业务 object，例如 `resource.mode` 与
   `scheduled_task.variables.mode` 继续合法；
5. `runtime=eino_adk`、model、Skill、MCP、知识库、数据库与可靠性配置继续合法。

命中时返回 typed `ErrUnsupportedExecutionControl`，携带稳定 path，例如
`config.configurable.mode`。canonical application error mapper 将其映射为
`422 unsupported_execution_control`、`retryable=false`；P1M-A handler 仍保留更早的 raw-body
拒绝与 duplicate-key/budget 防护。

## 4. 数据与兼容合同

- 被拒绝的 CreateTaskThread immediate/deferred 均不得创建 Thread、Run 或 Message。
- 被拒绝的 CreateRun 与 top-level retry 不得读取来源 Run或创建 Run/Message。
- server-owned ADK child 继续生成并持久化当前兼容字段。
- historical resume/retry/recovery 继续原样继承旧 Config；不重写历史行。
- `normalizeNewDeerFlowRunConfig`、`ParseDeerFlowRuntimeConfig`、ADK consumers 与 Journal gate
  保持现状；它们的退休属于后续 P1M-B2/P1M-C。

## 5. 验收

- 七字段 × `config`/`context` root 与四层保留容器均 typed reject；大小写稳定。
- 普通字符串、数组、资源 object 和合法 runtime/model/resource 配置不误拒。
- CreateTaskThread、CreateRun、top-level retry 均在 persistence 前拒绝。
- ADK server-owned child、Human resume、subagent retry、journal recovery、lease recovery 的旧
  Config 兼容回归通过。
- canonical typed error 精确映射为现有 P1M-A 422 wire contract。
- 相关 Go tests、compile-only、gofmt、diff-check 和 Workbench execution graph 门禁通过。

## 6. 非目标

不改前端、IDL、generated client、repository、migration 或 P1L；不删除 DeerFlow mode/parser；
不改变 ADK reasoning/Plan/Subagent 行为；不宣称完整 P1M 或 mode retirement 完成。
