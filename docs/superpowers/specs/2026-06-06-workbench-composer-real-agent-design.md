# Workbench Composer 与真实 AgentRun 设计

日期：2026-06-06
状态：已获方向确认，等待实现计划

## 背景

当前 Chat 工作台已经有首页输入区、任务详情页、任务事件和本地任务执行原型，但实现里仍存在两类临时逻辑：

1. 首页 `WorkbenchComposer` 只存在于 `workbench/index.tsx` 内部，任务详情页还有独立的 `FollowUpComposer`。
2. 后端 task 执行会写固定的模拟步骤，并生成“本地占位执行结果”，这会让 Agent 与报告能力看起来像真实执行，实际并未进入真实 AgentRun 或真实模型链路。

本轮目标是在不一次性做完复杂 Skills、KB、MCP 配置面板的前提下，先把统一输入组件、真实 Ark 模型回答、Ask 检索增强问答、最小真实 AgentRun 闭环落地。

## 目标

1. 首页和任务详情页复用同一个 `WorkbenchComposer` 组件。
2. 所有发送动作统一走 `sendWorkbenchChat`。
3. `task_id` 为空时创建新任务，完成后跳转任务详情页。
4. `task_id` 存在时在当前任务追加本轮提问，页面原地刷新。
5. `Auto` 走真实 builtin chat model，结果标记为 `answer` 和 `Ark`。
6. `Ask` 走真实 builtin chat model，可检索默认知识库，结果标记为 `answer`。
7. `Agent` 先接真实 `AgentRunDomainSVC.AgentRun` 最小闭环，结果标记为 `agent_trace`。
8. 清理或停用固定模拟 Agent 步骤、固定占位结果、默认报告模板拼接。
9. 前端按 `answer`、`agent_trace`、`report` 三类结果做差异化渲染。

## 非目标

1. 不在本轮完成完整 Skills、知识库、MCP 勾选 UI。
2. 不在本轮实现多智能体选择器或 `agent_id` 下拉。
3. 不在本轮重构完整 conversation/message 存储模型。
4. 不在本轮实现复杂资源权限过滤；AgentRun 先使用系统默认资源配置。
5. 不把普通 Ark/Ask 回答渲染成报告样式。

## 现有上下文

相关前端文件：

- `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- `frontend/apps/coze-studio/src/pages/workbench/extensions-popover.tsx`
- `frontend/apps/coze-studio/src/pages/workbench/service.ts`
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/helpers.ts`
- `frontend/apps/coze-studio/src/components/workspace-prototype.less`

相关后端文件：

- `idl/workbench/workbench.thrift`
- `backend/api/model/workbench/chat/workbench.go`
- `frontend/packages/arch/api-schema/src/idl/workbench/workbench.ts`
- `backend/application/workbench/chat_gateway.go`
- `backend/application/task/task.go`
- `backend/domain/task/service/service.go`
- `backend/domain/task/service/service_impl.go`
- `backend/application/conversation/conversation.go`
- `backend/domain/conversation/agentrun/service/agent_run.go`
- `backend/bizpkg/llm/modelbuilder/builtin.go`

## 接口设计

`WorkbenchChatRequest` 增加最小字段：

```thrift
struct WorkbenchChatRequest {
    1: required i64 space_id
    2: optional i64 conversation_id
    3: required string message
    4: required ChatMode mode
    5: optional i64 selected_skill_id
    6: optional i64 task_id
    7: optional list<string> enable_skills
    8: optional list<string> enable_mcp
    9: optional list<string> enable_kbs
    255: optional base.Base Base
}
```

`enable_skills`、`enable_mcp`、`enable_kbs` 是一期预留字段。前端在用户没有改动拓展配置时不强制传值，后端按默认全量资源处理。

`WorkbenchChatData` 保持返回 `task`，并增加结果元信息：

```thrift
struct WorkbenchChatData {
    1: required RouteTarget route_target
    2: optional string answer
    3: optional task.ChatTask task
    4: optional i64 conversation_id
    5: optional string reason
    6: optional string result_type
    7: optional string execution_type
}
```

## 统一 Composer

新组件建议放在：

`frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`

组件职责：

1. 管理输入框、模式切换、`@` 资源唤起、附件按钮、拓展面板、发送按钮。
2. 接收 `taskId?: string`，但不直接决定业务跳转；跳转和刷新由页面容器处理。
3. 接收 `onSubmit(payload)`，payload 包含 `message`、`mode`、`enableSkills`、`enableMcp`、`enableKbs`。
4. 统一 loading、disabled、error 展示。
5. 详情页复用同一组件，样式可以通过 `variant="home" | "detail"` 做轻量差异。

首页容器负责：

1. 读取 `space_id`。
2. 调用 `sendWorkbenchChat({ space_id, message, mode, ...reservedFields })`。
3. 成功拿到 `data.task.id` 后跳转 `/space/:space_id/tasks/:task_id`。

详情页容器负责：

1. 读取 `space_id` 和 `task_id`。
2. 调用 `sendWorkbenchChat({ space_id, task_id, message, mode, ...reservedFields })`。
3. 成功后清空输入并重新拉取 `getTask` 与 `listTaskEvents`，不跳转。

## 后端模式行为

### Auto

`Auto` 是 Ark 极速应答模式。后端使用 `modelbuilder.GetBuiltinChatModel(ctx, "WKB_")` 初始化真实模型并调用 `Generate`。

落库约定：

```json
{
  "message": "模型回答内容",
  "result_type": "answer",
  "execution_type": "Ark"
}
```

前端按普通对话内容渲染，不拼接报告模板。

### Ask

`Ask` 是普通问答模式。后端使用 builtin chat model 生成回答，可先检索默认知识库并把相关片段作为参考上下文注入模型提示。

Ask 的边界：

1. 可以检索知识库。
2. 后续可以接入联网搜索，把搜索结果作为参考上下文。
3. 不主动执行 Skills、MCP、SQL 或其他有副作用工具。
4. 即使使用知识库或联网搜索，结果仍按普通问答渲染，不展示 Agent 全流程执行面板。

落库约定：

```json
{
  "message": "模型回答内容",
  "result_type": "answer",
  "retrieval_sources": ["knowledge"]
}
```

前端按普通对话内容渲染。

### Agent

`Agent` 是真实智能执行模式。本轮先做最小闭环：

1. 不要求前端选择 `agent_id`。
2. 不要求本轮完成 Skills、KB、MCP 勾选控件。
3. 后端调用 `ConversationSVC.AgentRunDomainSVC.AgentRun`。
4. `enable_skills`、`enable_mcp`、`enable_kbs` 为空时使用系统默认全量配置。
5. 将 AgentRun 流式事件转换为 task events。
6. 最终结果写入 task result，标记为 `agent_trace`。

落库约定：

```json
{
  "message": "Agent 最终回答",
  "result_type": "agent_trace",
  "execution_type": "Agent"
}
```

事件流建议使用这些 event type：

- `user.message`：详情页追加提问。
- `agent.run_started`：AgentRun 已启动。
- `agent.thought`：模型思考或规划摘要。
- `agent.tool_call`：工具调用。
- `agent.tool_result`：工具结果。
- `agent.knowledge_retrieval`：知识库检索。
- `agent.answer_delta`：流式回答片段。
- `agent.run_completed`：最终结果。
- `agent.run_failed`：失败原因。

如果 AgentRun 事件暂时无法完整映射，先保留原始 payload 中的可展示字段，并保证前端可以用 `title`、`detail`、`thought`、`status` 渲染。

## 任务追加提问

当前 `chat_tasks` 只有单个 `input` 和 `result` 字段，不适合在本轮直接承载完整多轮会话。因此一期使用 task events 记录追加提问和执行轨迹：

1. 新建任务时，`task.input` 保存第一轮用户输入与模式。
2. 详情页追加提问时，后端 append `user.message` 事件。
3. 本轮执行过程 append Agent 或 answer 事件。
4. `task.result` 保存最新一轮结果摘要，便于列表和详情首屏展示。
5. 后续如果要完整多轮历史，再新增 `chat_task_turns` 或接入 conversation/message 表。

## 结果类型渲染

前端 helper 统一解析 task result：

1. `answer`：渲染普通助手回答气泡。
2. `agent_trace`：渲染执行流程和最终回答。
3. `report`：只在后端明确返回 `result_type: "report"` 时使用报告样式。

没有 `result_type` 的旧数据使用兼容规则：

1. 有结构化 Agent 事件时视为 `agent_trace`。
2. 有普通 `message` 时视为 `answer`。
3. 不再因为标题或任务描述包含“报告”就自动套报告样式。

## 错误与加载状态

1. Composer 发送中禁用按钮并展示 loading。
2. 后端模型调用失败时，任务标记 failed，`task.error` 写入可展示错误。
3. AgentRun 中途失败时，append `agent.run_failed`，并把任务状态置为 failed。
4. 详情页继续保留轮询刷新；提交追加提问成功后立即刷新一次。
5. 首页如果 `sendWorkbenchChat` 失败，不再绕过 chat gateway 调 `createWorkbenchTask` 兜底。

## 测试策略

前端测试：

1. `WorkbenchComposer` 在首页和详情页都渲染 Auto/Ask/Agent、拓展、附件、发送控件。
2. 首页发送只调用 `sendWorkbenchChat`，成功后跳转返回的 task detail。
3. 详情页发送带 `task_id`，成功后重新拉取 task 与 events，不跳转。
4. `answer`、`agent_trace`、`report` 三类结果走不同渲染分支。
5. 旧数据无 `result_type` 时保持可读展示。

后端测试：

1. `Auto` 创建或更新任务时写入 `result_type: answer`、`execution_type: Ark`。
2. `Ask` 写入 `result_type: answer`，可检索知识库，但不走 Skills、MCP、SQL 等执行型工具。
3. `Agent` 调用 `AgentRunDomainSVC.AgentRun`，并把流式事件 append 到 task events。
4. 带 `task_id` 的请求 append `user.message`，不创建新任务。
5. 空消息、非法 mode、缺失 task/domain service 返回 client error 或明确服务错误。

## 后续扩展

二期在本设计之上补齐：

1. 拓展面板 Skills、KB、MCP 勾选控件。
2. 勾选资源完整透传后端，并注入 AgentRun 运行环境。
3. SQL 查询、MCP 第三方接口调用、知识库检索的完整 trace 展示。
4. Ask 模式接入联网搜索，并把搜索来源、引用片段和失败降级策略纳入普通问答渲染。
5. 多智能体选择器，新增 `agent_id` 字段和对应校验逻辑。
6. 更完整的多轮会话表或 conversation/message 接入。

## 自检

1. 设计覆盖了首页和详情页统一输入组件。
2. 设计覆盖了 `task_id` 新建与追加两种路径。
3. 设计覆盖了 Auto、Ask、Agent 三种模式。
4. 设计明确 Ask 可检索知识库，并为后续联网搜索预留普通问答路径。
5. 设计明确真实 AgentRun 先做最小闭环，复杂资源配置后续补齐。
6. 设计明确了 `answer`、`agent_trace`、`report` 的渲染边界。
7. 设计没有要求本轮重构完整 conversation 存储模型。
8. 设计明确停止首页绕过 `sendWorkbenchChat` 的兜底创建逻辑。
