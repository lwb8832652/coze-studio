# Nuwax 任务中心完整迁移设计

## 目标

在 Coze Studio 工作空间侧边栏新增一级菜单 `任务中心`，完整迁移 Nuwax
任务中心的用户能力，并保持 Coze 当前 `全部任务` 作为会话和执行记录入口。
任务中心不是静态页面，也不复用旧的 ChatTask 壳，而是生产级、可持久化、
可恢复、可多实例运行的 Go 原生定时任务领域。

参考基线：

- Nuwax 页面与交互：`https://nuwax.com/job.html`
- Nuwax 本地参考源码：`/Users/liuwenbo/code/BuildingAI/nuwax-ai`
- Coze 现有 Agent 执行基座：`backend/application/agentthread`
- Coze 现有 Workflow 发布态执行基座：`backend/application/workflow`

## 用户体验

侧边栏在 `全部任务` 上方新增 `任务中心`。路由为：

```text
/space/:space_id/task-center
```

页面对齐 Nuwax 的主要信息架构：

- 顶部标题、任务说明和 `创建任务` 主按钮；
- 按任务类型和任务名称筛选；
- 表格展示任务类型、名称、执行对象、状态、执行次数、最近执行、下次执行、
  创建人、创建时间和操作；
- 行内支持立即执行和启用/停用；更多菜单支持查看执行记录、编辑、删除；
- 创建和编辑使用同一弹窗；
- 支持 Agent 与 Workflow 两种执行对象；
- 支持单次、每小时、每天、每周和合法 Cron 表达式；
- Agent 支持任务内容、变量和 `保持会话`；Workflow 支持发布态输入参数；
- 执行记录抽屉展示触发方式、状态、开始/结束时间、错误摘要以及关联的
  AgentThread 或 Workflow execution；
- 所有列表具备 loading、empty、error、refresh、disabled 和分页状态。

## 领域模型

### ScheduledTask

- `id`、`space_id`、`creator_id`
- `name`
- `target_type`: `agent` 或 `workflow`
- `target_id`、`target_name`、`target_icon_uri`
- `schedule_type`: `once`、`hourly`、`daily`、`weekly`、`cron`
- `cron_expr`、`timezone`、`run_once_at`
- `payload`: 经边界校验后的 JSON；Agent 为 message/variables，Workflow 为 inputs
- `keep_conversation`
- `status`: `enabled`、`disabled`、`completed`
- `execution_count`、`max_executions`
- `latest_execution_at`、`next_execution_at`
- `lease_owner`、`lease_expires_at`
- `version`，用于乐观并发控制
- `created_at`、`updated_at`、`deleted_at`

### ScheduledTaskExecution

- `id`、`task_id`、`space_id`
- `trigger_type`: `schedule` 或 `manual`
- `scheduled_at`
- `status`: `queued`、`running`、`succeeded`、`failed`、`canceled`
- `attempt`、`idempotency_key`
- `thread_id`、`run_id` 或 `workflow_execution_id`
- `error_code`、脱敏后的 `error_message`
- `started_at`、`finished_at`、`created_at`、`updated_at`

任务定义与执行记录分表，避免调度状态覆盖历史执行结果。执行记录使用唯一
`idempotency_key` 保证同一触发窗口只能创建一次。

## 调度模型

服务启动时注册 Task Center worker。Worker 使用短轮询批量领取到期任务，领取
动作必须是数据库条件更新，条件包含 `status=enabled`、`next_execution_at<=now`
及过期 lease。领取成功后：

1. 在事务中创建唯一执行记录；
2. 计算并持久化下一次执行时间；
3. 单次任务或达到最大次数的任务标记为 completed；
4. 提交事务后异步调用执行适配器；
5. 执行完成后更新记录和任务聚合字段；
6. 进程重启后，过期 lease 可重新领取，唯一键阻止重复执行。

时间计算统一使用 IANA timezone。服务端拒绝秒级高频、过去的单次时间、非法
Cron 和超出策略的任务数量。手动执行走同一执行管线，但不改变下次调度时间。

## 执行适配

### Agent

- 创建任务时验证 Agent 属于当前空间且调用者有使用权限；
- 不保持会话时，每次触发创建新 AgentThread 和 run；
- 保持会话时，为任务维护专用 thread，后续触发在同一 thread 创建 run；
- run 使用稳定的 idempotency key，并在 metadata 中写入 task/execution 标识；
- 页面执行记录可跳转到现有任务详情。

### Workflow

- 创建任务时验证 Workflow 属于当前空间且存在已发布版本；
- 使用发布态、后台、异步执行模式；
- 任务输入经过发布态 schema 校验；
- execution id 写入任务执行记录；
- 不通过内部 HTTP 回调自身 API，直接调用应用/领域接口，避免伪造 API auth。

## API 合同

在 Workbench Task IDL 增加生成式客户端合同：

- `CreateScheduledTask`
- `UpdateScheduledTask`
- `GetScheduledTask`
- `ListScheduledTasks`
- `DeleteScheduledTask`
- `EnableScheduledTask`
- `DisableScheduledTask`
- `ExecuteScheduledTask`
- `ListScheduledTaskExecutions`
- `ListScheduledTaskTargets`
- `ListScheduledTaskCronPresets`

所有 API 从认证上下文取得 user id，并以路径/请求中的 space id 做服务端成员与
资源权限校验。客户端提交的 creator、状态、执行次数和关联执行 id 一律忽略。

## 安全与可观测性

- payload 限制大小，错误信息脱敏，不返回 provider raw body、凭据和工具参数；
- 删除采用软删除，运行中的历史执行仍可审计；
- 创建、修改、启停、手动执行和删除写结构化审计日志；
- 暴露 bounded metrics：领取数、成功数、失败数、调度延迟和 lease 冲突；
- 任务对象删除、取消发布或权限变化时执行失败关闭，不绕过权限继续运行；
- Worker 关闭遵循 context cancellation，不遗留新的后台 goroutine。

## 验收标准

- 与 Nuwax 相同的核心页面结构和全部用户操作均可完成；
- Agent 与 Workflow 的创建、周期触发、单次触发和手动触发均形成真实执行；
- 启停、编辑、删除、保持会话、筛选、分页和执行记录行为正确；
- 多实例领取、幂等、重启恢复、权限失败和非法输入有自动化测试；
- Atlas hash/validate、相关 Go tests、Vitest 和 TypeScript 检查通过；
- 使用 Codex in-app browser 对创建、编辑、启停、立即执行、记录查看和删除做
  端到端验收，并记录 URL、空间、账号、关键状态和控制台错误。
