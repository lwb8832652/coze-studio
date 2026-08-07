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

## Journal 执行体验

- 公开前端不再暴露 Flash、Thinking、Pro、Ultra 模式选择，新任务、追问和
  重试统一提交 `requested_policy=auto`。后端只保留 `auto`、`pro`、`ultra`
  策略；`pro` 和 `ultra` 是内部显式覆盖，不是公开页面交互。
- `auto` 在同一次 Agent 执行中按任务事实决定直答、Todo 规划或 Subagent
  协作，不增加独立意图识别模型调用。简单问题和单步操作不得为了 Journal
  强制创建计划或子代理。
- Journal 只为通过 feature gate 的顶层 Task Run 建立投影；子任务和
  Subagent Run 不建立公共 Journal。简单直答只保留生命周期事实，前端不展示
  空步骤或 Journal 外壳。
- `journal.intro` 是任务执行开场语的权威事件，位于第一个可见大步骤之前。后端可接收
  经过脱敏和长度限制的 `metadata.execution_intro`，但不得要求模型为了 Journal 改变
  原执行提示词；通常由完整计划标题生成安全兜底。它不是额外的聊天消息，也不得包含
  隐藏推理、原始工具参数、凭证或内部绝对路径。
- 左侧执行流只展示已经到达的公共事件。大步骤由 `milestone.*` 表示，小步骤由实际
  执行过的 `action.*`、`artifact.*`、`verification.*` 或 `confirmation.*` 表示；
  没有子动作的大步骤保持原子步骤，不显示展开入口。
- 工具调用开始时可绑定当时唯一的 active plan task；父子 Agent 的未结束调用可并存，
  terminal 事件成功持久化后才释放对应绑定。绑定只存在于当前进程的 Journal 投影内存，
  并按 Agent 作用域隔离，不进入模型上下文、Eino checkpoint、公共 checkpoint 或业务
  revision。恢复后绑定缺失时，Journal 仓储为同一 action 的后续阶段继承已经持久化的
  milestone；没有既有阶段、当前步骤不唯一或容量不足时才降级为原子步骤。任何情况都
  不能改写、阻塞或重试工具实际执行。
- Skill 使用事件与快照属于异步、限时的 Journal 观察，技能内容返回不得等待事件仓储、
  快照仓储或对象存储。观察超时、异常或运行结束竞态只允许缺失该条展示，不能改变技能
  加载结果、执行上下文、取消状态或任务终态。
- 同一工具动作的事件和内容快照必须复用稳定的 action identity、operation、target
  与 milestone；实际文件名和路径保留在经过审核的文档或代码快照内容中，不能通过
  改写动作字段破坏幂等和重放。
- 前端以规范 `journal.intro` 为准；历史 assistant 开场消息只作为没有规范事件时的
  兼容兜底，二者不得重复显示。生命周期、技能目录和内部运行元数据不能伪装成可见
  执行步骤。
- canonical Message 对每个顶层 Run 只投影一条公开 assistant 回复：存在持久化回复时
  以最新持久化内容为准，不再把中间 `message.completed` 事件或异常重复持久化内容当成
  第二条对话；尚无持久化回复时，只保留该 Run 最新一条非空事件内容作为运行中或
  中断态兜底。原始 RunEvent 继续完整保留给 Journal、审计和恢复使用。
- TokenUsage 以真实模型调用为计量单位。计费、安全等 ChatModel 包装器与其嵌套的
  provider 回调若属于同一父子调用且用量完全一致，只记录 provider 一次；互相独立的
  调用即使用量相同也必须分别记录，包装器去重不得改变额度预留、模型调用或工具执行。

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
