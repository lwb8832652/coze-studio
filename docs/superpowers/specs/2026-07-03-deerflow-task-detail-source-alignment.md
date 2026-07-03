# DeerFlow Task Detail Source Alignment Audit

日期：2026-07-03

范围：任务详情页、消息/步骤渲染、Artifacts、Token、Todo、追问、取消、导出、
历史加载、标题同步和相关后端接口。

结论：2026-07-03 P1-K 已完成用户确认的 5 个强制对齐点：Todo 持久态、
Artifacts 60/40 可拖拽双栏、follow-up suggestions、思考/步骤视觉体系、多轮
追问全量上下文。Coze 仍不是直接复用 DeerFlow 的 React/shadcn/Python 源码，
因为本项目保留 Go/Eino + Coze Design/Semi + 任务命名与生产安全边界；但任务详情
这 5 个可见能力已经按 DeerFlow 源码链路完成等价实现和 targeted verification。
后续所有任务详情改动仍必须以本文的源码链路为准，不能靠截图或推断补丁。

## 1. 总体结论

### 1.1 已经对齐的主链路

- 任务详情从 thread 维度加载：Coze 使用
  `/api/workbench/task_threads/:thread_id`、messages、runs、run_events、
  token_usage、artifacts；DeerFlow 使用 `/api/threads/:thread_id` 相关
  LangGraph 兼容接口。
- 执行步骤不再以 checkpoint history 为主数据源：Coze 通过 run events 和
  `journal_messages` 投影出 DeerFlow 风格消息；DeerFlow 通过
  `RunJournal(category=message)` 暴露 run messages。
- run lifecycle、标题同步、部分系统事件不渲染到可见步骤里：Coze 已有对应
  前端测试覆盖。
- Token 顶部入口、每轮 token、per-turn/debug 模式已具备可见功能。
- Markdown、Mermaid、Artifact 卡片、右侧预览、下载、`.skill` 安装能力均已
  有对应实现。
- 追问会附带历史消息构造下一轮 run input，避免丢失前文。
- 运行中停止按钮、SSE run event stream、轮询刷新、标题同步和我的任务列表
  upsert 已有实现。
- Todo dock 优先读取 `TaskThread.values.todos`，事件推导只作为老数据兜底。
- Artifacts 打开时内容区为 60/40 可拖拽双栏，关闭时恢复 chat 100%。
- 单轮终态 assistant turn 后自动调用 Workbench suggestions API，展示推荐追问。
- canonical follow-up run input 通过分页读取完整 thread messages，不再固定截断
  在最近 50 条。

### 1.2 保留差异和原因

- DeerFlow 任务详情的历史消息加载是按 run 分页：
  `GET /api/threads/:thread_id/runs/:run_id/messages?before_seq=...`，顶部
  IntersectionObserver 触发继续加载。Coze 当前详情页初始只取 thread messages
  的分页聚合结果；follow-up 已全量分页读取，但详情页顶部“继续加载更老消息”
  仍是 P2 级体验增强。
- DeerFlow visible steps 是 `thread.messages -> getMessageGroups ->
  MessageGroup.convertToSteps`。Coze 当前是 `run_events + persisted messages ->
  journal_messages -> task-event-projection -> TaskEventsSection`，行为靠近，但
  不是组件/数据模型同构。
- DeerFlow Artifact file list 对 `.skill` 始终显示 install + download。Coze
  继续保留生产级安全扫描能力，但主消息卡默认不渲染 release/quarantine/block，
  避免破坏 DeerFlow 文件卡主视觉；相关治理能力留在次级/安全入口。
- DeerFlow 输入框使用 leading slash skill suggestion。Coze 当前增加了 `@` 资源
  引用，这是 Coze 独有能力，不应当强行作为 DeerFlow parity 项。
- DeerFlow 使用 shadcn/lucide 的 `ChainOfThought`、`LightbulbIcon`、
  `ListTodoIcon` 等组件。Coze 使用 `@coze-arch/coze-design` icon 和
  `coze-prototype-*` 样式实现等价结构，不直接引入 shadcn 组件。

## 2. DeerFlow 源码链路

### 2.1 页面入口和状态流

| 功能 | DeerFlow 源码 | 证据 |
| --- | --- | --- |
| 页面入口 | `frontend/src/app/workspace/chats/[thread_id]/page.tsx` | `ChatPage` 使用 `useThreadStream`、`useThreadTokenUsage`、`ThreadTitle`、`MessageList`、`TodoList`、`InputBox`。 |
| 消息流 | `frontend/src/core/threads/hooks.ts` | `useStream<AgentThreadState>` 使用 `assistantId: "lead_agent"`、`reconnectOnMount: true`、`fetchStateHistory: { limit: 1 }`。 |
| 历史加载 | `frontend/src/core/threads/hooks.ts` | `useThreadHistory` 先 `useThreadRuns`，再按 run 调 `/api/threads/:thread_id/runs/:run_id/messages`，使用 `before_seq` 继续加载。 |
| 页面渲染 | `frontend/src/components/workspace/messages/message-list.tsx` | `MessageList` 对 `thread.messages` 调 `getMessageGroups`，然后分 human、assistant、present-files、subagent、processing 渲染。 |
| 底部输入 | `frontend/src/components/workspace/input-box.tsx` | streaming 状态下 submit 触发 `onStop`；完成后请求 suggestions。 |

关键事实：

- `POST /api/langgraph/threads/:thread_id/history` 或 DeerFlow 对应 checkpoint
  history 不是可见步骤唯一数据源。它用于 checkpoint/state，不应该替代 run
  messages。
- 可见步骤来自 run journal message，而不是 raw provider event 或 checkpoint
  bytes。
- DeerFlow 运行态和历史态是分层的：`useStream` 管实时状态，`useThreadHistory`
  管历史消息回填。

### 2.2 DeerFlow 消息分组规则

源码：`frontend/src/core/messages/utils.ts`

规则：

- `human` 直接形成一个 human group。
- `tool` 优先挂到上一个可接收 tool 的 processing group。
- clarification tool 会同时进入上一组并开启独立 clarification group。
- AI 如果有 present files，形成 `assistant:present-files`。
- AI 如果有 subagent tool，形成 `assistant:subagent`。
- AI 如果有 reasoning 或 tool calls，形成 `assistant:processing`。
- AI 有 content 且没有 tool calls，会额外形成 assistant bubble。
- hidden control messages 包括 `summary`、`loop_warning`、`todo_reminder`、
  `todo_completion_reminder`，不进入 UI。

这意味着 DeerFlow 的步骤展示不是简单的“事件列表”，而是一个基于 message 类型、
tool call、tool result 和 reasoning 的分组过程。

### 2.3 DeerFlow 步骤展示规则

源码：`frontend/src/components/workspace/messages/message-group.tsx`

规则：

- `MessageGroup` 先把 messages 转成 steps。
- AI reasoning 转成 reasoning step。
- AI tool calls 转成 toolCall step，tool name 为 `task` 的子智能体调用被排除在
  普通步骤外。
- tool result 通过 tool_call_id 找回并挂到对应 toolCall step。
- 默认只突出最后一个 tool call 和最后一个 reasoning step。
- 前置步骤用“查看其他 N 个步骤 / 隐藏步骤”折叠。
- 思考步骤使用 `LightbulbIcon`，内容展开/收起。
- 容器是 `ChainOfThought`，样式为 `w-full gap-2 rounded-lg border p-0.5`。

### 2.4 DeerFlow Artifacts

源码：

- `frontend/src/components/workspace/chats/chat-box.tsx`
- `frontend/src/components/workspace/artifacts/artifact-file-list.tsx`
- `frontend/src/components/workspace/artifacts/artifact-file-detail.tsx`
- `backend/app/gateway/routers/artifacts.py`

规则：

- `ChatBox` 使用 resizable panel，关闭时 chat 100/artifacts 0，打开时 chat
  60/artifacts 40。
- `thread.values.artifacts` 同步到 artifacts provider。
- present-files 消息内的文件会渲染 `ArtifactFileList`。
- 点击文件卡打开右侧 artifact panel。
- `.skill` 文件有 install 按钮；所有文件有 download。
- 右侧详情支持 code/preview toggle、copy、download、open in new window、close。

### 2.5 DeerFlow Token

源码：

- `frontend/src/components/workspace/token-usage-indicator.tsx`
- `frontend/src/core/threads/api.ts`
- `backend/app/gateway/routers/thread_runs.py`

规则：

- 顶部显示 `Tokens <total>`，可点开下拉。
- 下拉显示输入、输出、总计。
- 展示模式：关闭、总览、每轮、调试。
- 说明文案强调顶部总量优先用后端持久化线程用量，当前回复仍在流式时不会强行
  累加当前回合。

### 2.6 DeerFlow Todo

源码：`frontend/src/components/workspace/todo-list.tsx`

规则：

- Todo 来源是 `thread.values.todos`。
- 输入框上方展示一个 dock，默认 collapsed。
- header 使用 `ListTodoIcon`，右侧 chevron。
- body 固定高度展开，显示 todo 队列。

### 2.7 DeerFlow 追问、停止和 suggestions

源码：

- `frontend/src/components/workspace/input-box.tsx`
- `backend/app/gateway/routers/suggestions.py`

规则：

- streaming 状态下提交按钮变成停止行为，调用 `onStop`。
- 停止后由 LangGraph SDK 的 `thread.stop()` 处理。
- 完成一次回复后，前端取最近 6 条 human/assistant 消息，请求
  `POST /api/threads/:thread_id/suggestions`。
- suggestions 失败时静默返回空数组。

## 3. Coze 当前源码链路

### 3.1 后端接口和数据合同

IDL：`idl/workbench/task.thrift`

关键结构：

- `TaskThread`：任务线程标题、状态、最近用户/助手消息。
- `TaskThreadMessage`：持久化用户/助手消息。
- `TaskThreadRun`：run 元信息、status、config、context、metadata。
- `TaskThreadRunEvent`：run event 原始投影，包含 event_type/payload。
- `TaskThreadRunJournalMessage`：为前端任务详情提供的 DeerFlow 风格消息投影。
- `TaskThreadTokenUsage` / aggregate / run aggregate：token 记录和聚合。
- `TaskThreadArtifact`：文件产物 metadata。

接口：

| Coze Workbench API | 用途 |
| --- | --- |
| `GET /api/workbench/task_threads/:thread_id` | 任务详情基本信息 |
| `GET /api/workbench/task_threads/:thread_id/messages` | 持久化消息，当前详情页取 `page=1&page_size=50` |
| `GET /api/workbench/task_threads/:thread_id/runs` | run 列表，当前详情页取最新 top-level run |
| `GET /api/workbench/task_threads/:thread_id/run_events` | run events，并返回 `journal_messages` |
| `GET /api/workbench/task_threads/:thread_id/run_events/stream` | SSE run event stream |
| `GET /api/workbench/task_threads/:thread_id/token_usage` | token 用量 |
| `GET /api/workbench/task_threads/:thread_id/artifacts` | artifacts |
| `POST /api/workbench/task_threads/:thread_id/messages` | 追问追加用户消息 |
| `POST /api/workbench/task_threads/:thread_id/runs` | 创建追问 run |
| `POST /api/workbench/task_threads/:thread_id/runs/:run_id/cancel` | 停止 run |

后端入口：

- `backend/api/handler/coze/workbench_thread_service.go`
- `backend/application/agentthread/service.go`
- `backend/application/agentthread/run_journal_messages.go`
- `backend/domain/agentthread/service/service_impl.go`
- `backend/domain/agentthread/repository/mysql.go`

重要实现细节：

- `ListTaskThreadRunEvents` 先取 run events，再调用
  `taskThreadRunJournalMessagesForAPIEvents`。
- `taskThreadRunJournalMessagesForAPIEvents` 会额外拉取 thread messages 和 runs，
  再调用 `ProjectThreadRunJournalMessages`。
- `ProjectRunJournalMessages` 明确说明目标是归一化为 DeerFlow 的
  `getMessageGroups/convertToSteps` 前置消息形态，并且避免使用 checkpoint history。
- messages 仓储按 `created_at ASC, id ASC` 排序。
- run events 仓储按 `id ASC` 排序。
- runs 仓储默认 top-level，按 `created_at DESC, id DESC` 排序。

### 3.2 前端详情加载

源码：

- `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-run-event-stream.ts`

加载流程：

1. `TaskDetailPage` 从路由读取 `task_id` 或 `thread_id`。
2. `useTaskDetailData` 调 `fetchTaskDetail`。
3. `fetchTaskThreadDetail` 并发拉取 thread、messages、latest run、run events、
   token usage、artifacts。
4. `mergeJournalTaskEvents` 优先使用 `journal_messages`，移除 raw
   `message.completed` 和 raw `tool.*` 的重复渲染。
5. `TaskThreadConversation` 根据 messages/events/artifacts 分组渲染。
6. `useTaskThreadRunEventStream` 订阅 SSE，将新 run event 合并到 `events`，并处理
   `context.thread_title_updated`。
7. 非 terminal 状态会 2 秒 polling 一次 `fetchTaskDetail`。

### 3.3 Coze 步骤投影

源码：

- `frontend/apps/coze-studio/src/pages/tasks/task-event-projection.ts`
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`

当前逻辑：

- `projectTaskExecutionEvents` 先过滤不可见 flow。
- structured events 存在时优先展示 structured projection。
- terminal run 会把仍是 running 的可见步骤修正成 completed 显示。
- 只展示最后一个 actionable step，其余折叠到“查看其他 N 个步骤”。
- `message.completed` 中的 reasoning 会转成 thought step。
- tool calls 会解析为 tool step，支持 web_search、skill、path 等特殊 label。
- 测试已经覆盖：
  - title sync event 不渲染成步骤。
  - run.started/run.completed 不渲染成 DeerFlow 步骤。
  - write_file/web_search/write_todos 等 tool label 按 DeerFlow 风格展示。

### 3.4 Coze Artifacts

源码：

- `frontend/apps/coze-studio/src/pages/tasks/task-artifact-message-list.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-panel.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-markdown-content.tsx`

当前逻辑：

- Artifact 卡片跟随消息/run 渲染。
- previewable 文件点击打开右侧预览。
- Markdown 中 Mermaid code block 会被拆出来独立渲染，并提供下载 SVG、复制源码。
- `.skill` artifact 可安装。
- 安全扫描/审核状态会影响可见操作，Coze 额外支持 release/quarantine/block、
  deleted artifacts、scan jobs。

### 3.5 Coze 追问

源码：

- `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`

当前逻辑：

- canonical thread detail 下，追问先拉最近 50 条 thread messages。
- 上传附件后追加 user message。
- 创建新 run 时把 history messages 和当前用户消息打包到 input。
- 先构造 optimistic detail，再刷新 `fetchTaskDetail`。
- 非 canonical legacy task detail 仍走 `sendWorkbenchChat`。

## 4. 逐项对齐矩阵

| 模块 | DeerFlow 标准 | Coze 当前状态 | 判定 | 后续动作 |
| --- | --- | --- | --- | --- |
| 路由 | `/workspace/chats/:thread_id` | `/space/:space_id/tasks/:thread_id` 和 `/chats/new` | 保留差异 | 菜单命名按 Coze 原型保留“任务”，不改叫对话。 |
| 顶部标题 | `ThreadTitle` 使用 thread state/title | `TaskTopBar` 使用 artifact/title/message 规则，支持 title sync event | 部分对齐 | 继续用 Coze 顶栏，但要验证长标题不回退。 |
| Token 顶部入口 | `TokenUsageIndicator` | `TaskTokenUsageIndicator` | 部分对齐 | 文案/布局可继续贴近；Coze 额外 cost/tool 字段需决策是否隐藏。 |
| 导出 | `ExportTrigger` 导出 Markdown/JSON | `TaskExportAction` | 部分对齐 | 需要保持 DeerFlow 导出结构，特别是 JSON/MD 字段。 |
| 消息来源 | `thread.messages` + `useThreadHistory` | Workbench messages/events/journal_messages | 部分对齐 | 数据契约不同，不能声称源码级一致。 |
| 历史加载 | per-run `before_seq` + 顶部 load more | follow-up run input 已分页读取完整 thread messages；详情页顶部继续加载旧消息仍未做 | 部分对齐 | P1-K 已消除追问 50 条截断；顶部旧消息加载保留为 P2 体验增强。 |
| 消息分组 | `getMessageGroups` | `TaskThreadConversation` 自定义分组 | 部分对齐 | 如要 100% 行为，应补前端等价 group adapter。 |
| 执行步骤 | `MessageGroup.convertToSteps` | `projectTaskExecutionEvents` + `TaskEventsSection` | 部分对齐 | 继续按 DeerFlow step 算法核对 last tool / last reasoning / folding。 |
| 思考块 | `ChainOfThought` + `LightbulbIcon` | `coze-prototype-chain-of-thought` + Coze icons | 已完成 | 已按 DeerFlow `ChainOfThought` 源码收敛 header/content/step/rail/status/motion；保留 Coze icon 实现。 |
| Loading | `MessageListSkeleton` 和 `StreamingIndicator` | 自定义 skeleton + `TaskStreamingIndicator` | 部分对齐 | 对齐 DeerFlow 初始加载、等待回复的点状动画。 |
| Todo | `thread.values.todos` | `TaskThread.values.todos` 持久态优先，run events fallback | 已完成 | 后端只暴露 id/title/status bounded metadata；前端 Todo dock 刷新后仍从 thread values 恢复。 |
| 输入框停靠 | `InputBox` 固定在底部，Todo 在上方 | `TaskFollowUpComposer` 固定底部，Todo stack 在上方 | 基本对齐 | 输入框视觉按用户另行确认，本文不作为完成判定。 |
| 停止按钮 | streaming submit 触发 `onStop` | `stopMode` / cancel run | 基本对齐 | 需用真实运行中任务验证按钮状态。 |
| 追问历史 | SDK state + persisted history | canonical follow-up 分页读取完整 thread messages | 已完成 | `task-follow-up.ts` 循环分页，run input 按时间顺序包含全量历史后追加当前消息。 |
| Follow-up suggestions | `/api/threads/:id/suggestions` | `POST /api/workbench/task_threads/:thread_id/suggestions` | 已完成 | 响应直接 `{suggestions:[...]}`，失败静默空数组；前端终态 assistant turn 后自动展示 chips。 |
| Artifact list | present-files group 中 card | artifact groups 按 run/message 渲染 | 部分对齐 | 卡片位置、按钮、安装态仍需逐例对比。 |
| Artifact 右侧面板 | ChatBox 60/40 resizable | 内容区 60/40 可拖拽 split + Coze side preview | 已完成 | 打开 artifact 时 chat/artifacts 默认 60/40，可拖拽 30%-55%；关闭恢复 chat 100%。 |
| `.skill` 安装 | install + download | install + download，已安装状态仍需运行态确认 | 部分对齐 | 安装成功后按钮态需和 DeerFlow 一致。 |
| 安全扫描按钮 | DeerFlow 无 | 主消息卡隐藏 review actions；安全能力保留 | 已完成 | 默认文件卡只保留 DeerFlow 主动作；扫描治理能力不进入标准卡片主视觉。 |
| Mermaid | Markdown preview 渲染 Mermaid | 独立 Mermaid split/render，支持 copy/download | 基本对齐 | 样式、图表操作位置继续按 DeerFlow 微调。 |
| 子智能体 | subagent group + SubtaskCard | subagent runs/重试/详情 | 部分对齐 | 需用 subagent 测试例核对。 |
| MCP/Skill 工具步骤 | tool call + result | tool.completed/message.completed 投影 | 部分对齐 | 技能/MCP 当前用户确认可二期优化，保留回归用例即可。 |
| 任务记忆 | DeerFlow 有 thread/context 相关能力 | Coze 有 memory API/UI inspector | 部分对齐 | 记忆可见体验和 DeerFlow 仍需单独验收。 |

## 5. 必补差异清单

### P1-D1 历史消息分页和顶部加载（P2 体验增强）

问题：DeerFlow 在消息列表顶部有 load more，滚到顶部自动加载旧 run messages。
Coze P1-K 已解决 follow-up run input 的完整历史读取，但详情页顶部继续加载更早
可见消息仍不是 DeerFlow 同构实现。

建议实现：

1. 后端保留现有 thread messages 分页，前端增加 detail history state。
2. 优先按 Coze 当前 API 实现 page-based 顶部加载；如果需要更接近 DeerFlow，再补
   run-scoped journal message cursor API。
3. UI 使用 DeerFlow 的顶部加载样式：loading spinner + “加载更多”。
4. 验收：构造超过 50 条消息的 thread，详情页顶部滚动可继续加载，加载中有感知。

### P1-D2 步骤算法更贴近 DeerFlow

问题：Coze 当前步骤由 event projection 生成，不完全等价
`getMessageGroups/convertToSteps`。

建议实现：

1. 在前端新增 DeerFlow message group adapter，输入 Coze `journal_messages`，
   输出 group/step。
2. 和 DeerFlow 一样：
   - reasoning + content 且无 tool calls 时既展示 thinking，也展示 assistant bubble。
   - tool result 必须按 `tool_call_id` 挂回 tool call。
   - 默认突出最后 tool call 和最后 reasoning。
   - `task` 子智能体 tool 不进入普通工具步骤。
3. 保留 Coze security/lifecycle event 过滤。
4. 验收：同一文档生成、联网搜索、skill、MCP、subagent case 中步骤数量、折叠、
   label、icon、路径展示与 DeerFlow 一致。

### P1-K1 Todo 待办模块持久态（已完成）

原问题：DeerFlow Todo 来源是 `thread.values.todos`，Coze 从 run event 推导。

完成状态：

- 后端 `TaskThread.values.todos` 从最新 checkpoint/channel values 或 thread
  metadata 安全投影，只暴露 todo `id/title/status`，不暴露 raw checkpoint、
  prompt、tool args 或 tool result。
- 前端 `TaskExecutionTodoDock` 优先使用线程持久态 todos；没有持久态时才使用
  run event 推导作为老数据 fallback。
- 验证：`TestGetTaskThreadHandlerReturnsThreadValuesTodos`、`renders persisted
  thread todos`、完整 `task-detail.test.tsx`。

强制实现：

1. 后端定义安全的 Workbench thread values/todos 投影，只暴露 todo id/title/status
   等 bounded metadata，不暴露 raw checkpoint、prompt、tool args 或 tool result。
2. 从 Eino/Coze thread checkpoint/channel values 或 thread metadata 中持久化并读取
   todos，语义对齐 DeerFlow `thread.values.todos`。
3. Workbench `TaskThread` 或等价 detail response 暴露 `values.todos`。
4. 前端 Todo dock 优先使用线程持久态 todos；仅当没有持久态时，才用 run event
   推导作为兼容兜底。
5. 验收：创建/更新 todo 后刷新详情页，Todo dock 仍显示同样状态；无持久态老数据
   仍能从事件 fallback 显示。

### P1-K2 Artifacts 产物面板（已完成）

原问题：DeerFlow 是 resizable 60/40 ChatBox，Coze 右侧区域宽度和行为不同。

完成状态：

- 任务详情内容区增加 `coze-prototype-detail-split`；artifact 打开时 chat 与
  artifacts 默认 60/40，关闭时 chat 恢复 100%。
- 拖拽 handle 可在 30%-55% 之间调整 artifact 面板宽度，移动端仍使用 overlay
  以避免小屏内容挤压。
- 文件卡 preview 统一打开右侧 panel；主消息卡传入 `renderReviewActions=false`，
  release/quarantine/block 不再破坏 DeerFlow 文件卡主视觉。
- 验证：`keeps task detail responsive overrides`、`renders generated document
  artifacts`、完整 `task-detail.test.tsx`。

强制实现：

1. 保留 Coze 顶栏和任务路由差异，但内容区必须改为 DeerFlow 60/40 可拖拽双栏：
   artifacts 关闭时 chat 100%，打开时 chat 60% / artifacts 40%。
2. 文件卡统一使用 DeerFlow 的触发逻辑：点击/预览打开右侧 panel，关闭后恢复 chat
   100%。
3. Artifact header 对齐 code/preview toggle、copy、download、close。
4. 文档预览不要总出现在聊天最下方，应跟随产生它的 assistant turn，并同步打开右侧
   Artifacts。
5. Coze release/quarantine/block 等安全扫描控件默认弱化到次级入口或更多菜单，不在
   文件卡主视觉上破坏 DeerFlow 标准动作。

### P1-D5 Loading 和 streaming 细节

问题：用户多次反馈等待加载动画、停止按钮、消息下方加载点和 DeerFlow 不一致。

建议实现：

1. 初始详情 loading 对齐 `MessageListSkeleton`。
2. 运行中 assistant turn 下方对齐 DeerFlow `StreamingIndicator`。
3. 输入框 submit 按钮 running 时展示停止方块，取消文字按钮不出现在主视觉。
4. 验收：发送 “青岛最佳旅游时间” 等简单问题时，等待阶段可见动画，完成后自动消失。

### P1-K3 Follow-up suggestions（已完成）

原问题：DeerFlow 完成回复后会生成建议追问。Coze 当前任务详情没有等价 Workbench
suggestions API。

完成状态：

- 新增 Workbench API：`POST /api/workbench/task_threads/:thread_id/suggestions`。
- 请求体对齐 DeerFlow：`messages`、`n`、`model_name`、`model_type`；响应直接返回
  `{ "suggestions": [...] }`，不使用 Coze `code/data` 包裹。
- 服务端复用 Workbench chat model provider，解析 JSON array，剥离 `<think>` 与
  markdown code fence；模型不可用或异常时返回空 suggestions，不阻断主对话。
- 2026-07-03 代码审核补齐模型字段贯通：IDL/API schema/handler 支持
  `model_type`，前端从 latest top-level run config 白名单提取
  `model_name/model_type` 传入 suggestions 请求；模型字段也纳入前端生成 key，
  避免 suggestions effect 在模型状态尚未补齐时先发空模型请求后不再重试。
- 前端在终态 assistant turn 后取最近 6 条 user/assistant 消息自动请求，展示
  DeerFlow-style suggestion chips，点击填入追问输入。
- 验证：`TestGenerateSuggestions*`、`TestGenerateTaskThreadSuggestionsHandlerReturnsDeerFlowShape`、
  `generates DeerFlow-style follow-up suggestions`。

强制实现：

1. 后端新增 Workbench task thread suggestions endpoint，语义对齐 DeerFlow
   `POST /api/threads/:thread_id/suggestions`。
2. 请求体接收 bounded human/assistant 最近消息和 `n/model_name` 等安全字段；服务端
   不返回 raw prompt、completion、provider body。
3. 前端在最新 assistant 完成后请求最近 6 条 human/assistant 消息。
4. suggestions 失败返回空数组或静默不展示，不阻断主对话。
5. 建议问题显示在 composer 上方，点击后填入/发送行为与 DeerFlow 保持一致。
6. 验收：完成一轮后显示 2-3 个建议问题，可点击继续追问。

### P1-K4 视觉、图标、思考组件体系（已完成）

原问题：DeerFlow 使用 shadcn `ChainOfThought`、`LightbulbIcon`、`ListTodoIcon`
和成体系的折叠动效；Coze 当前是仿制样式，间距、图标和状态翻转仍有差异。

完成状态：

- 已重新核对 DeerFlow
  `frontend/src/components/ai-elements/chain-of-thought.tsx`，并在 Coze 样式中对齐：
  step `flex gap-2 text-sm`、timeline rail、current-color icon、running/pending/failed
  状态色、path pill、chevron rotate、fade/slide entry motion。
- Coze 没有引入 shadcn/lucide 直接依赖，继续使用现有 Coze Design icon 与
  `coze-prototype-*` class，但可见交互和结构按 DeerFlow 收敛。
- 验证：`renders DeerFlow-style reasoning`、`renders ADK reasoning`、完整
  `task-detail.test.tsx`。

强制实现：

1. 以 DeerFlow `MessageGroup.convertToSteps` 和 `ChainOfThought` 源码为参照，逐项
   对齐 step row、rail、icon、path pill、collapsed/expanded 状态。
2. 思考步骤使用灯泡语义图标，Todo dock 使用待办语义图标，不再用临时字符图标。
3. 折叠/展开的文案、chevron 状态、上下间距、边框和动效要与 DeerFlow 运行态对齐。
4. 保留 Coze 安全过滤和 raw payload 隐藏规则；视觉对齐不能暴露 tool args/result。
5. 验收：同一 DeerFlow/Coze 文档生成任务中，步骤卡、思考块、Todo dock 的视觉结构
   和交互节奏一致；不一致项必须记录原因。

### P1-K5 多轮追问全量上下文（已完成）

原问题：DeerFlow follow-up 基于完整 run/thread history；Coze 当前
`task-follow-up.ts` 只读取 `page_size: 50`，长对话会截断前文。

完成状态：

- `task-follow-up.ts` 将历史读取改为 `page_size=200` 分页循环，直到累计数量达到
  API 返回的 total。
- canonical follow-up run input 按时间顺序包含完整 user/assistant 历史消息，再追加
  当前用户消息；不再静默丢弃最早消息。
- suggestions 仍按 DeerFlow 源码只取最近 6 条消息生成推荐问题，这和 follow-up
  run input 的全量上下文不是同一链路。
- 验证：`task-follow-up.test.ts` 覆盖超过一页的历史；`sends canonical thread
  follow-up messages` 验证详情页调用。

强制实现：

1. 前端或后端提供全量历史消息读取逻辑，不能再固定只取第一页 50 条。
2. 若沿用现有 paged API，则循环分页直到无更多消息或达到服务端安全上限；如需要
   后端专用接口，必须保持返回内容仅限 user/assistant safe message。
3. follow-up run input 按时间顺序包含完整 user/assistant 历史，再追加当前用户消息。
4. 对超长输入的 token 压缩策略只能作为显式 runtime 能力，不得静默丢弃最早消息。
5. 验收：构造超过 50 条历史消息的任务，追问 run input 包含第 1 条和最后 1 条历史
   消息，前端前一轮步骤仍保留。

### P1-D7 Token 下拉内容

问题：Coze 增加 cost/tool/lead/subagent 等字段，功能更强但不完全等于 DeerFlow。

建议决策：

- 若以 DeerFlow 视觉为准：默认下拉只显示输入、输出、总计、显示方式、说明。
- 若保留 Coze 生产观测：把额外字段放到“调试/详情”折叠区，不影响默认视图。

## 6. 验收用例

### 6.1 必跑 DeerFlow/Coze 双系统对照

| 用例 | 输入 | 需要核对 |
| --- | --- | --- |
| 普通问答 | `青岛最佳旅游时间` | loading、streaming、停止按钮、最终 assistant bubble、Token。 |
| 文档生成 | 生成武汉 3 日游 Markdown 文档并作为 Artifacts 展示 | present-files 位置、文件卡、右侧预览、下载、导出、Token。 |
| Mermaid | 绘制时序图或架构图 | Mermaid 渲染、复制、下载、正文位置、右侧预览。 |
| 追问 | 在已有任务里继续问“导出 PDF 怎么做” | 历史上下文、前一轮步骤是否保留、新一轮步骤位置、标题不回退。 |
| 停止 | 运行中点击发送/停止按钮 | 按钮形态、停止语义、run 状态、loading 消失。 |
| 长历史 | 超过 50 条消息的任务 | 顶部加载旧消息、加载动画、滚动位置保持。 |
| Todo | 生成 todo 的任务 | Todo dock 来源、展开/收起、状态更新、刷新后是否保留。 |
| Artifact 安装 | 生成 `.skill` 文件并安装 | 安装按钮、安装成功后的状态、技能列表是否出现。 |
| MCP/工具 | 天气或搜索工具 | 工具步骤 label、tool result、是否自动选择/使用。 |
| 记忆 | 需要记忆/检索的多轮任务 | 记忆写入、检索提示、UI 是否只展示安全 metadata。 |

### 6.2 代码侧验收

- P1-K 本轮已执行：
  - `cd backend && go test ./application/workbench -run TestGenerateSuggestions -count=1`
  - `cd backend && go test ./api/handler/coze -run 'Test(GetTaskThreadHandlerReturnsThreadValuesTodos|GenerateTaskThreadSuggestionsHandlerReturnsDeerFlowShape)' -count=1 -gcflags="all=-N -l"`
  - `cd backend && go test ./api/router/coze -run TestRegisterIncludesWorkbenchTaskThreadRoutes -count=1`
  - `cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-follow-up.test.ts`
  - `cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`
  - `cd frontend/apps/coze-studio && npx tsc --noEmit --project tsconfig.json`
- 后续改任务详情前至少重跑：
  `cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`
- 如果改迁移：
  `atlas migrate validate --dir file://docker/atlas/migrations`

## 7. 源码证据索引

### DeerFlow

- `frontend/src/app/workspace/chats/[thread_id]/page.tsx`
  - `ChatPage` 汇总 `useThreadStream`、`MessageList`、`TodoList`、`InputBox`。
- `frontend/src/core/threads/hooks.ts`
  - `buildRunMessagesUrl`
  - `useThreadStream`
  - `useThreadHistory`
- `frontend/src/core/messages/utils.ts`
  - `getMessageGroups`
  - `getAssistantTurnUsageMessages`
- `frontend/src/components/workspace/messages/message-list.tsx`
  - `LoadMoreHistoryIndicator`
  - `MessageList`
- `frontend/src/components/workspace/messages/message-group.tsx`
  - `MessageGroup`
  - `convertToSteps`
- `frontend/src/components/workspace/input-box.tsx`
  - `handleSubmit`
  - follow-up suggestions
- `frontend/src/components/workspace/artifacts/artifact-file-list.tsx`
- `frontend/src/components/workspace/artifacts/artifact-file-detail.tsx`
- `frontend/src/components/workspace/token-usage-indicator.tsx`
- `frontend/src/components/workspace/todo-list.tsx`
- `backend/app/gateway/routers/thread_runs.py`
  - runs create/stream/list/cancel
  - thread/run messages
  - token usage
- `backend/app/gateway/routers/suggestions.py`
- `backend/app/gateway/routers/artifacts.py`
- `backend/packages/harness/deerflow/runtime/journal.py`
- `backend/packages/harness/deerflow/runtime/events/store/base.py`

### Coze

- `idl/workbench/task.thrift`
- `backend/api/handler/coze/workbench_thread_service.go`
- `backend/api/model/workbench/thread/thread.go`
- `backend/application/workbench/suggestions.go`
- `backend/application/agentthread/service.go`
- `backend/application/agentthread/run_journal_messages.go`
- `backend/domain/agentthread/service/service_impl.go`
- `backend/domain/agentthread/repository/mysql.go`
- `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-run-event-stream.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-follow-up-composer.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-event-projection.ts`
- `frontend/apps/coze-studio/src/pages/tasks/task-artifact-message-list.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-panel.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-markdown-content.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-token-usage-indicator.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-message-token-usage.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-execution-todo-dock.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/task-display-title.ts`
- `frontend/apps/coze-studio/src/components/workspace-prototype.less`
- `backend/application/workbench/suggestions_test.go`
- `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-follow-up.test.ts`
- `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

## 8. P1-K 收口后建议

P1-K 已完成本文用户确认的 5 个必须项。后续不应再把 Todo 持久态、Artifact
60/40 双栏、suggestions、思考组件或 follow-up 全量历史当作未开始任务。

推荐后续顺序：

1. 浏览器人工复验：用同一文档生成、Mermaid、长历史追问、Todo、suggestions case
   在 Coze 与 DeerFlow 中做最后一轮视觉确认。
2. 详情页顶部旧消息加载：如果用户仍要求长历史可视加载同构，按 P2 单独实现
   DeerFlow-style top load-more。
3. 步骤算法同构增强：如需进一步减少 projection 偏差，再把 Coze `journal_messages`
   适配为更接近 DeerFlow `getMessageGroups/convertToSteps` 的 adapter。
4. Token 默认视图：决定是否把 Coze 扩展字段继续折叠到调试区，默认视图只保留
   DeerFlow 输入/输出/总计。
5. Skills/MCP policy history：用户已确认当前功能可用，后续问题转 P2 细化。

## 9. 风险和边界

- 不能为了“像 DeerFlow”而暴露 Coze 当前明确禁止展示的 raw prompt、tool args、
  tool results、checkpoint bytes、provider raw body 或凭据。
- Coze 的安全扫描、artifact review、memory audit、MCP runtime audit 属于生产级
  加固能力；可在视觉上弱化，但不建议删除。
- 如果要达到真正源码复用级 100%，需要引入 DeerFlow 前端 message group/component
  体系或重写 Coze detail 的渲染骨架；这会影响 Coze Design、现有测试和生产安全
  面板，需要作为单独大任务评估。当前 P1-K 的“对齐”定义为源码核实后的等价能力
  和可见体验对齐，不是直接复制 DeerFlow 技术栈。
- 本文记录的 P1-K 代码验证已经通过；最终像素级视觉仍建议由用户用本地浏览器
  按 6.1 case 复验。
