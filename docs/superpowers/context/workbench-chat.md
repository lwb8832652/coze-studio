# WorkbenchChat 当前事实

## 权威边界

本文只描述已经落地的 WorkbenchChat 现状。发生冲突时，以当前源码、IDL、迁移、
生成代码和测试为准。历史 ChatTask plans/specs 只用于追溯，K2 仍是设计语料，
两者都不能用于推断当前实现。

WorkbenchChat 是任务交互业务域名称，不再代表旧的 `/api/workbench/chat` 方法。
产品界面和局部页面适配可以继续使用“任务”命名，公共 API 和数据合同必须映射到
canonical Thread 主模型。

## 唯一事实模型

```text
Thread
  -> Message
  -> Run
       -> RunEvent
       -> Checkpoint
       -> TokenUsage
       -> Artifact
  -> Memory
  -> GuardrailAudit / MCPRuntimeAudit
```

- 首次提交调用唯一单例 `canonicalThreadClient.createThread`；带文件提交先创建
  deferred Thread，再上传文件并调用 `createRun`。
- 详情、列表、最近任务、消息和附属数据只读取 `/api/workbench/threads/**`。
- 追问先上传附件，再通过 `createRun` 原子持久化当前轮用户消息和顶层 Run；服务端
  从 Thread 历史重建权威输入。
- 取消、人工恢复和子智能体重试分别调用 `cancelRun`、`resumeRun` 和
  `retrySubagentRun`；重试创建新 Run，不改写历史 Run。
- 事件订阅调用 `subscribeRunEvents`，通过 `@coze-arch/fetch-stream` 读取 canonical
  SSE；页面源码不得绕过统一 client 直接创建浏览器流连接。
- 执行内核为 Eino ADK，公共 API 只返回经过审核的 bounded projection。

## 代码所有权

- 前端入口：`frontend/apps/coze-studio/src/pages/workbench`；
- 任务列表与详情：`frontend/apps/coze-studio/src/pages/tasks`；
- 前端合同与唯一实例：`thread-client/workbench-thread-client.ts`、
  `canonical-thread-client.ts`、`canonical-thread-client-singleton.ts`；
- IDL：`idl/workbench/thread.thrift` 与 `idl/workbench/thread_product.thrift`；
- HTTP：`backend/api/handler/coze/workbench_canonical_*.go`；
- 路由事实：`backend/api/router/coze/api.go` 与
  `workbench_canonical_thread_route_test.go`；
- 应用层：`backend/application/agentthread`；
- 领域与持久化：`backend/domain/agentthread`；
- Runtime Doctor 与建议生成：`backend/application/workbench`。

`backend/application/workbench` 不拥有任务状态机或任务持久化。它只保留与当前
Workbench 辅助能力相关的 Runtime Doctor 和建议生成。

前端 `service.ts` 中仍存在 `createTaskThread` 等页面适配函数和
`LegacyPageResponse` 命名，用于保持既有 React 组件的数据形状；它们全部直接委托
唯一 canonical client，不是旧 transport、双 client 或 fallback。后端内部
`ApplicationService.CreateTaskThread` 仍用于原子首次提交、Scheduled Task 和飞书
入口，也不代表旧公共合同仍存在。

## 已退役边界

以下内容不属于当前实现，禁止重新作为 fallback、兼容层或新功能依赖引入：

- `/api/workbench/chat`；
- `/api/workbench/tasks` 及其详情、事件、取消和重试子路由；
- `/api/workbench/task_threads/**` 的 36 条 V1 路由；
- `/api/threads/**` 的 23 条本地 LangGraph Thread 路由；
- `/api/runs/**` 的 10 条本地 stateless LangGraph Run 路由；
- `WorkbenchChatRequest`、`WorkbenchChatResponse`、`ChatTask`、`TaskEvent`、
  `TaskStatus`；
- `backend/application/task`、`backend/domain/task` 和旧 Workbench runner/gateway；
- 前端 `GetTask`、`ListTaskEvents`、`sendWorkbenchChat` 回退；
- `agent_threads.legacy_task_id` 及 metadata 中的同名旧键；
- `chat_tasks`、`chat_task_attempts`、`chat_task_events`。

`/api/workbench/scheduled_tasks/**` 等 11 条 Scheduled Task 路由仍是当前产品合同，
不属于上述退役范围。canonical Run SSE 保留经审核的 SDK-compatible
event shape 不等于保留旧 `/api/runs/**` 合同或 fallback。

最终合同退役与页面回归证据见
`docs/superpowers/evidence/2026-07-30-workbench-final-contract-retirement.md`。历史 Gate A、
Gate B 文档只描述各自提交时点，不再代表当前 route surface。

历史 Atlas migration 保留为不可改写的演进记录，不代表表仍属于最新 schema。

## 数据库过渡

删除迁移为
`docker/atlas/migrations/20260726000100_drop_legacy_workbench_chat.sql`。迁移会在旧表
仍有行、`agent_threads.legacy_task_id` 仍非零或 metadata 仍含同名旧键时主动失败；
每个 DDL 步骤都按对象存在性执行，可在中途故障修复后重试。

各环境必须分别执行：只读统计、受控备份、明确确认、清空旧数据、再次确认、
应用迁移。代码合入不等于数据库迁移已执行；未取得当次确认时不得写目标数据库。
具体步骤见 `docs/superpowers/runbooks/workbench-chat-legacy-cleanup.md`。

## 检索与同步

- 结构与调用链先用 codebase-memory，结论必须回到真实源码核实；
- 长期业务关系使用 Graphify 查询本文件和 `project-context.md`；
- WorkbenchChat 公共 API、持久化、主流程或安全边界变化时，同一需求分支同步本文；
- Graphify 输出只保存在 ignored `graphify-out/`，不提交派生图；
- codebase-memory 或 Graphify 不可用时记录降级，使用 `rg`、源码、IDL 和测试完成
  审计，不得猜测补全。
