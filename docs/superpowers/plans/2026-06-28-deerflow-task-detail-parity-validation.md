# DeerFlow 任务详情能力对齐验收计划

> Active validation ledger for DeerFlow task-detail parity. Keep this file
> updated before and after each browser/API/code comparison slice. The P0
> implementation tracker remains
> `docs/superpowers/plans/2026-06-27-deerflow-p0-launch-tracker.md`.

## 目标

当前先收敛 DeerFlow 2.x `workspace/chats/:thread_id` 的任务详情体验，在
Coze Studio 中保持任务命名，但让页面能力、交互效果、后端语义和代码方向尽量
等价。

- DeerFlow 参考页：
  `http://localhost:2026/workspace/chats/c155a732-f475-4cf9-aa49-13fd26b29888`
- Coze 目标页：
  `http://localhost:8080/space/7656275718757679104/tasks/7656293553953308672`
- 两边测试账号：`840582614@qq.com` / `z8832652`
- 标准验证提示词：

```text
你可以绘制时序图或架构图吗？请直接给出一个 Mermaid sequenceDiagram 和一个 flowchart 架构图，并说明如何继续修改。
```

## 边界

P0 只做 DeerFlow 可见主线能力对齐：

- 任务详情页、执行流、消息流、追问、停止、重试、导出、Artifacts、文档
  生成/预览、Token、Skills、MCP、记忆、设置入口。
- Coze 路由和文案保持任务口径：`新建任务`、`全部任务`、`我的任务`、
  `任务详情`、`任务记忆`。
- 后端接口不要求路径同名，但请求参数、响应语义、流式行为、错误状态和安全
  脱敏要等价。

不纳入 P0：

- IM Channels。
- 复杂安全扫描、Prometheus/OpenTelemetry 导出、压测/混沌、完整浏览器 CI。
- 非 DeerFlow 可见的二期优化。发现后记录为 P1/P2，不插入当前主线。

## 状态规则

| Status | Meaning |
| --- | --- |
| 已完成 | 有实现、浏览器/API/源码证据齐全，不需要 P0 继续开发。 |
| 待验收 | 实现看起来存在，但还缺 DeerFlow 对照证据或本轮未重跑。 |
| 进行中 | 当前 slice 正在采集证据或开发修复。 |
| 待开发 | DeerFlow P0 必需能力缺失。 |
| 差异待确认 | DeerFlow 与 Coze 产品形态不同，需要确认是 P0 对齐还是 P1 记录。 |
| 延后(P1) | 不影响第一版上线，但已记录。 |

一个 case 只能在同时满足以下条件时标 `已完成`：

- 前端页面证据：截图或 DOM 断言。
- 后端证据：接口请求参数、响应摘要、SSE/事件语义，或明确说明 N/A。
- 代码证据：两边关键源码入口或 Coze 单测/类型检查命令。
- 差异处理：缺口已在本文件或 P0 tracker 中记录。

## 全局能力地图

| Mainline | DeerFlow Reference | Coze Target | P0 Status | Notes |
| --- | --- | --- | --- | --- |
| 菜单和路由 | `workspace/chats/new`、`workspace/chats/:id`、最近对话 | `新建任务`、`全部任务`、`我的任务`、`tasks/:id` | 已完成 | 命名保留任务方式，行为对齐；详情和新建路由 HTTP 200，列表路由由单测覆盖。 |
| 任务详情布局 | Chat-like thread surface, header actions, bottom composer | Canonical task detail page | 已完成 | 桌面 P0 已收口；窄屏精细截图转 P1。 |
| 消息/思考/Markdown | user/assistant turn, inline reasoning, Markdown/Mermaid | task messages + answer/result markdown | 已完成 | inline thinking 保留；复制/反馈和 Mermaid 错误图转 P1。 |
| 执行流 | LangGraph run stream/events | Workbench run events + execution cards | 已完成 | 工具事件安全投影已覆盖；子智能体深卡片转 P1。 |
| 追问/停止/重试 | bottom composer send/stop, run cancel/retry | follow-up, cancel, retry task run | 已完成 | 主 tracker 和测试/冒烟证据已覆盖。 |
| Artifacts | header trigger + side panel + file links | Task artifacts panel | 已完成 | 文档产物卡、右侧预览、下载、安全边界已收口。 |
| 文档生成/预览 | Agent 生成文件后进入 Artifacts，可内联预览或安全下载 | 任务运行生成文档产物，任务详情可预览/下载 | 已完成 | P0 文档生成/预览闭环已收；更丰富 MIME 样本库转 P1。 |
| 导出 | thread export action | 任务导出能力 | 已完成 | 顶栏导出菜单已按 DeerFlow 提供 Markdown / JSON。 |
| Token 用量 | global token button, per-turn usage | DeerFlow-style token popover + per-turn usage summary | 已完成 | 顶部弹层和每轮 assistant 汇总已代码对齐；active streaming token 转 P1。 |
| Skills | public/custom Skills, slash/progressive activation, Skills page/API | 技能配置 + Eino runtime loading | 已完成 | DeerFlow public builtin bundle 已迁移到 Coze；Skill 设置页视觉已收敛；`新建技能` 已对齐为 skill-creator 新任务入口；`.skill` artifact install 已接入 Workbench adapter、generated Skill API client 和产物卡片，用户导入/安装缺省 `type` 已按 DeerFlow custom 语义落到 `CustomSkill`；ADK runtime 已新增 `create_skill_package` 安全打包工具，让 skill-creator 可生成真正 zip `.skill` 产物并通过 `present_files` 展示；composer slash suggestion、runtime turn-scoped selector、slash activation model prompt 已接入；ADK selected Skill 运行已通过真实本地 HTTP/API 验证并补 `skills.loaded` 安全事件。2026-06-30 用户确认 Skills 功能测试没有问题，P0 不再继续逐项验收；真实截图补充、slash 网络抓包、真实 `.skill` 产物安装补证据，以及 DeerFlow skill-creator scripts/references/eval resources 的运行时深度可用性均转二期优化。 |
| MCP/Tools | MCP settings, tool catalog, tool calls | 工具配置 + Eino MCP runtime | 已完成 | `/tools` 可见配置已对齐为 server switch list；weather MCP 已完成 post-fix 真实 Eino stdio 调用，确认 `tool_search -> mcp_7656840243668058112_get_weather` 且参数 `city=北京`。2026-06-30 用户确认 MCP 功能测试没有问题，P0 不再继续逐项验收；GitHub/Postgres 外部凭据类深测、更多 DeerFlow settings 截图归档和边缘异常优化转二期。 |
| 记忆 | runtime memory + settings | `任务记忆` panel + retrieval | 已完成 | 管理面板、CRUD、导入导出和召回边界按 P0 主 tracker 收口。 |
| 设置 | model/mode/runtime settings | Workbench runtime settings + 设置 | 已完成 | 不改 API 授权；Runtime 设置入口和配置透传已收口。 |
| 后端 API | `/api/threads`, `/runs/stream`, `/messages`, `/token-usage` | `/api/workbench/task_threads` 系列 | 已完成 | 路径不同，P0 语义由 Workbench task-thread API 承载。 |
| 代码方向 | Next.js + LangGraph SDK + Python gateway | React/Semi + Go Hertz + Eino ADK | 已完成 | Eino-first；不引 Python sidecar。 |

## 源码对照入口

### DeerFlow

| Area | Files |
| --- | --- |
| Chat detail page | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/app/workspace/chats/[thread_id]/page.tsx` |
| Providers | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/app/workspace/chats/[thread_id]/providers.tsx` |
| Composer | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/input-box.tsx` |
| Message list | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-list.tsx` |
| Message item/reasoning | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-list-item.tsx` |
| Markdown | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/markdown-content.tsx` |
| Message token usage | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-token-usage.tsx` |
| Artifact trigger/panel | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/artifacts` |
| Thread APIs | `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/threads.py` |
| Run/SSE/message/token APIs | `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/thread_runs.py` |
| Artifact file API | `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/artifacts.py` |
| Public builtin Skills | `/Users/liuwenbo/code/BuildingAI/deer-flow/skills/public` |
| Skills storage/parser | `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/skills` |
| Skills APIs | `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/skills.py` |
| Skills settings page | `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/settings/skill-settings-page.tsx` |
| MCP APIs | `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/mcp.py` |

### Coze Studio

| Area | Files |
| --- | --- |
| Task detail page | `frontend/apps/coze-studio/src/pages/tasks/detail.tsx` |
| Detail loader | `frontend/apps/coze-studio/src/pages/tasks/task-detail-loader.ts` |
| Event stream | `frontend/apps/coze-studio/src/pages/tasks/task-run-event-stream.ts` |
| Event projection | `frontend/apps/coze-studio/src/pages/tasks/task-event-projection.ts` |
| Markdown/Mermaid | `frontend/apps/coze-studio/src/pages/tasks/task-markdown-content.tsx` |
| Top bar | `frontend/apps/coze-studio/src/pages/tasks/task-top-bar.tsx` |
| Follow-up composer | `frontend/apps/coze-studio/src/pages/tasks/task-follow-up-composer.tsx`, `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`, `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer-controls.tsx` |
| Run actions | `frontend/apps/coze-studio/src/pages/tasks/task-run-action-bar.tsx` |
| Artifacts panel | `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-panel.tsx` |
| Artifact list item | `frontend/apps/coze-studio/src/pages/tasks/task-artifact-list-item.tsx` |
| Artifact inline preview | `frontend/apps/coze-studio/src/pages/tasks/task-artifact-inline-preview.tsx` |
| Artifact helpers | `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-helpers.ts` |
| Memory panel | `frontend/apps/coze-studio/src/pages/tasks/task-memory-section.tsx` |
| Token usage | `frontend/apps/coze-studio/src/pages/tasks/task-token-usage-indicator.tsx` |
| Frontend service | `frontend/apps/coze-studio/src/pages/tasks/service.ts` |
| IDL source | `idl/workbench/task.thrift` |
| Generated API source | `frontend/packages/arch/api-schema/src/idl/workbench/task.ts` |
| Router | `backend/api/router/coze/api.go` |
| Handlers | `backend/api/handler/coze/workbench_thread*.go` |
| App service/runtime | `backend/application/agentthread` |

## 后端接口对照

| Capability | DeerFlow API | Coze API | Required Evidence |
| --- | --- | --- | --- |
| 创建 thread/task | `POST /api/threads` | `POST /api/workbench/task_threads` | request body, response id/status, ownership/space fields. |
| 创建并流式运行 | `POST /api/threads/:thread_id/runs/stream` | `POST /api/workbench/task_threads/:thread_id/runs` + `/run_events/stream` | stream headers/events, run id, terminal event. |
| 追加消息 | SDK send via run input; persisted messages endpoint | `POST /api/workbench/task_threads/:thread_id/messages` | appended user message and follow-up run. |
| 列消息 | `GET /api/threads/:thread_id/messages` | `GET /api/workbench/task_threads/:thread_id/messages` | user/assistant order, pagination, safe fields. |
| 列 run | `GET /api/threads/:thread_id/runs` | `GET /api/workbench/task_threads/:thread_id/runs` | top-level/child filtering, status, usage summary. |
| run 事件 | `GET /api/threads/:thread_id/runs/:run_id/events` | `GET /api/workbench/task_threads/:thread_id/run_events` | event type mapping and redaction. |
| reconnect/join | `GET/POST /api/threads/:thread_id/runs/:run_id/stream` or `/join` | `GET /api/workbench/task_threads/:thread_id/run_events/stream` | refresh/reopen can recover current output. |
| 停止 | `POST /api/threads/:thread_id/runs/:run_id/cancel` | `POST /api/workbench/task_threads/:thread_id/runs/:run_id/cancel` | active/pending behavior, terminal status. |
| 重试 | DeerFlow run/task retry behavior | `POST /api/workbench/task_threads/:thread_id/runs/:run_id/retry` | failed/canceled only, metadata-only retry source. |
| Token aggregate | `GET /api/threads/:thread_id/token-usage` | `GET /api/workbench/task_threads/:thread_id/token_usage` | aggregate fields, per-run/child rollup if requested. |
| Artifacts | DeerFlow artifact URLs and panel-backed routes | `GET /api/workbench/task_threads/:thread_id/artifacts` and signed/content routes | list/content/signed-url shape, no unsafe payload. |
| 文档预览/下载 | `GET /api/threads/:thread_id/artifacts/:path` with inline/download behavior | `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content` and `/signed_url` | content type, preview mode, download disposition, active-content safety. |
| Memory | DeerFlow memory/runtime state | `GET/PUT/DELETE/POST /api/workbench/task_threads/:thread_id/memories*` | management and runtime recall are separated. |
| Skills | `/api/skills*` | Workbench Skill APIs + generated clients | list/create/update/version/test/activation. |
| MCP | `/api/mcp*` | Workbench MCP tool APIs | server/tool config, health, runtime invocation. |

## 任务详情细化测试 Case

### A. 导航和页面骨架

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-NAV-001 | 详情路由打开 | `/workspace/chats/:thread_id` 直接进入详情 | `/space/:space_id/tasks/:thread_id` 直接进入任务详情 | 两边 URL 截图，刷新后仍可用 | detail 接口 200 | 两边 page/router 文件 | 已完成 |
| TD-NAV-002 | 最近列表进入 | 侧边栏最近对话点开同一 thread | `我的任务` 点开同一 thread，不跳 agent legacy page | 点击前后 URL/标题截图 | list/detail 接口 thread id 一致 | Coze sidebar/list service | 已完成 |
| TD-LAYOUT-001 | 整体布局 | 顶部 header、中间消息流、底部 composer、右侧/弹出 Artifacts | 保留任务命名；当前 header/composer 已接近，助手消息和执行流仍需继续对齐 DeerFlow chat-like 体验 | viewport screenshots | N/A | detail/top-bar/follow-up/artifacts files | 已完成 |
| TD-LAYOUT-002 | 响应式 | DeerFlow 窄屏不遮挡消息和 composer | Coze 窄屏不重叠、不溢出 | 390px/768px 截图 | N/A | LESS/CSS and component layout | 延后(P1) |
| TD-LAYOUT-003 | 对话记录纯净度 | 聊天记录区只承载用户消息、助手回复、inline 思考、工具/任务状态和产物引用 | 运行诊断、安全审计、任务记忆不得直接铺在聊天对话记录中；应进入 header action、侧栏 inspector、弹层或折叠二级面板 | main content DOM + inspector open DOM | N/A | `task-detail-inspector.tsx`, `detail.tsx`, `task-top-bar.tsx`, `task-detail.test.tsx` | 已完成 |
| TD-HDR-001 | 标题 | Header 显示 thread title | Header 显示任务 title，不能只显示 ID | header 截图 | detail response title | loader/top-bar | 已完成 |
| TD-HDR-002 | 状态和进度 | running/ready 状态清楚 | terminal run 显示 `已完成` 且无 stale running | screenshot/DOM | runs response latest top-level status | loader/detail tests | 已完成 |
| TD-HDR-003 | Header actions | Token、导出、Artifacts 在右上 | 顶栏保留 DeerFlow 主线动作：Tokens、导出、详情；`产物` 不再占用顶栏 | action 区截图 | token/artifact/export APIs | top-bar/artifacts/export files | 已完成 |

### B. 消息流、思考和 Markdown

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-MSG-001 | 用户消息 | 用户 turn 右侧气泡/块，纯文本安全显示 | Coze 已显示用户输入气泡且未执行用户 Markdown，仍需补精确间距/avatar 对照 | message screenshot | messages API role/order | DeerFlow message item, Coze messages mapping | 已完成 |
| TD-MSG-002 | 助手消息 | assistant turn inline，Markdown 渲染 | Coze 已渲染 Markdown/Mermaid，并移除 `普通回答 · Agent/Ark` 结果标签 | screenshot | messages/answer payload | detail/markdown files | 已完成 |
| TD-MSG-003 | 历史分页 | DeerFlow 可加载更多 history | Coze 任务历史刷新后完整可恢复 | load-more/reopen screenshot | messages pagination params/response | service/loader | 已完成 |
| TD-MSG-004 | 流式中状态 | streaming indicator 和停止按钮同步 | Coze run_events 流式更新步骤和答案 | running screenshot/video if needed | SSE event samples | stream hook/event projection | 已完成 |
| TD-MSG-005 | inline 思考块 | Reasoning trigger + collapsible content 保留 | Coze 从 ADK `message.completed` / answer payload 的 `reasoning_content`、`reasoning_parts`、`<think>` 中提取 inline `思考` 折叠块，正文剥离原始 `<think>` 标签，provider signature/raw payload 不进 UI | 待补真实 reasoning 样本截图 | event/message reasoning fields | `task-reasoning.ts`, `task-inline-reasoning.tsx`, `task-event-display.ts`, `task-detail.test.tsx` | 已完成 |
| TD-MSG-006 | 复制和反馈 | assistant turn 有复制/反馈操作 | Coze 至少有复制；反馈如不做则 P1 | hover/action screenshot | feedback API if present | message toolbar/action files | 延后(P1) |
| TD-MSG-007 | 代码块/表格/链接 | Markdown 组件渲染稳定 | Coze Markdown 不破坏布局，链接安全打开 | markdown matrix screenshot | message body sample | markdown components/tests | 已完成 |
| TD-MD-001 | Mermaid sequenceDiagram | DeerFlow 输出 SVG/图形，不显示 raw fence | Coze 渲染 sequenceDiagram SVG | screenshot + DOM svg count | assistant message content | markdown component test | 已完成 |
| TD-MD-002 | Mermaid flowchart | DeerFlow 输出 SVG/图形，不显示 raw fence | Coze 渲染 flowchart SVG | screenshot + DOM svg count | assistant message content | markdown component test | 已完成 |
| TD-MD-003 | Mermaid 错误态 | 错误图不炸页面，可显示源码/错误提示 | Coze Mermaid 失败时安全 fallback | invalid Mermaid screenshot | message body sample | component error test | 延后(P1) |

### C. 执行流和运行控制

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-RUN-001 | 创建并运行 | 发送后创建 thread/run 并进入 streaming | 新建任务后创建 thread/run，进入任务详情 | new-task flow screenshot | create/run request/response | workbench page + handler tests | 已完成 |
| TD-RUN-002 | 事件流 | LangGraph SSE 输出 messages/events | Coze SSE 输出 safe run_events，UI 实时更新 | network SSE sample | SSE raw event sample | run-event-stream/event projection | 已完成 |
| TD-RUN-003 | 刷新重连 | running/recent page refresh 后继续看状态 | Coze refresh 后状态和输出可恢复 | refresh screenshot | list events/runs/messages | loader/stream files | 已完成 |
| TD-RUN-004 | 停止 | stop 按钮取消 active run | `取消任务` 只在 active run 显示，成功终止 | running -> canceled screenshot | cancel request/response | run action hook + handler tests | 已完成 |
| TD-RUN-005 | 失败重试 | failed/canceled run 可 retry | `重试任务` 创建新 top-level retry run | failed/canceled screenshot | retry request/response metadata | retry hook + handler tests | 已完成 |
| TD-RUN-006 | 终态收敛 | 完成后页面不再显示 running | Coze 最新 top-level terminal run 驱动 header/progress | completed screenshot | runs latest status | loader/detail tests | 已完成 |
| TD-FLOW-001 | 步骤/时间线 | DeerFlow 可见思考/任务/工具进度 | Coze 保留 `执行流程` 标题，并以轻量 feed 展示每一步状态、标题、runtime、详情/思考和时间 | execution flow screenshot | events list | event projection tests | 已完成 |
| TD-FLOW-002 | 工具事件 | tool 调用状态可读，不泄露敏感参数 | Coze tool card 隐藏 args/results/raw provider | tool event screenshot | redacted event response | safety tests | 已完成 |
| TD-FLOW-003 | 子智能体事件 | subagent 状态清楚 | Coze 子智能体卡片只显示安全 metadata | subagent case screenshot | child run/events/token response | subagent files/tests | 延后(P1) |

### D. Composer、追问和附件

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-COMP-001 | 底部输入框 | 固定底部，未遮挡消息；输入框左侧附件、模式，右侧模型、`@`、链接、圆形发送 | Coze task detail 使用 DeerFlow presentation：去掉 `Auto/Ask/Agent` 分段、隐藏运行设置按钮，保留 `拓展` 和 `@`，发送按钮改为 DeerFlow 绿色圆形图标 | browser scroll metrics + DOM + composer 截图 | N/A | `task-follow-up-composer.tsx`, `workbench-composer*.tsx`, `workspace-prototype.less`, `task-detail.test.tsx` | 已完成 |
| TD-COMP-002 | 模型选择 | model selector 可见且和上下文绑定 | Coze task detail 当前显示模型 readout；完整模型下拉和运行配置绑定需继续验收 | model dropdown screenshot | run config request | model loader/settings control/service | 已完成 |
| TD-COMP-003 | 模式选择 | 闪速/思考/Pro/Ultra 或同等能力；DeerFlow 默认按模型能力解析为 `Pro`，并把模式写入运行 context；provider reasoning effort 需受模型能力控制 | Coze task detail 和首页输入框已改为受控 DeerFlow 模式触发器，默认 `Pro`，不再露出 `Auto/Ask/Agent`；切换 `flash/thinking/pro/ultra` 会写入 `thinking_enabled`、`is_plan_mode`、`subagent_enabled`，但不默认下发 provider `reasoning_effort`；只有运行设置显式启用 reasoning 时才带 `reasoning_effort`，避免 DeepSeek 等模型因不支持 provider reasoning 参数失败；legacy WorkbenchChat 仅做兼容映射 | mode menu screenshot 待补；browser DOM 已确认首页 composer 为 DeerFlow presentation 且显示 `Pro`；React 交互测试已覆盖点击 `Ultra` 后发送；post-fix run `7656986537476751360` 成功且持久化 `mode=pro` / `thinking_enabled=true` / `is_plan_mode=true` / no `reasoning_effort` | config payload 单测覆盖四种 mode flags、默认不含 `reasoning_effort`、显式 reasoning 时才含 `reasoning_effort`；targeted mode vitest passed 8 selected tests | `workbench-composer-controls.tsx`, `workbench/components/types.ts`, `workbench.test.tsx`, `task-detail.test.tsx`, `task-detail-hooks.ts`, `task-run-actions-hook.ts`; evidence note `TD-COMP-003-deerflow-mode-runtime.md` | 已完成 |
| TD-COMP-004 | Skill slash/渐进激活 | 输入 `/` 可筛选 Skill | Coze 保留 `拓展` Skill/MCP 选择和 `@` 上下文入口；Workbench composer 已接入 DeerFlow-style slash skill suggestion，运行时 provider 已支持 slash turn-scoped selector；单测确认 slash 发送时保留 `/skill-name ...` message 且不写入 `enable_skills`；slash suggestion placement 已按 DeerFlow 输入框场景收敛：首页向下、详情页向上，避免弹层被遮挡 | `TD-SKILL-004-home-slash-suggestions.png`, `TD-SKILL-004-detail-slash-suggestions.png` | slash message payload 已由 workbench test 覆盖；真实网络抓包待补 | `workbench-composer.tsx`, `workbench-composer-controls.tsx`, `index.less`, `skill_provider.go`, workbench/runtime provider tests | 已完成 |
| TD-COMP-005 | MCP/tool 激活 | DeerFlow Settings `ToolSettingsPage` 只展示 MCP server name/description/enabled switch；运行时从 enabled MCP servers 注入工具，可选择 MCP/tool 并调用 | Coze `/tools` 已按 DeerFlow SettingsSection 收敛：标题 `工具`，说明 `管理 MCP 工具的配置和启用状态。`，列表只显示 MCP server name/description/switch，去掉主线可见的创建/搜索/测试/删除/健康 pill；Workbench `拓展` 是启用开关/可用范围配置，不要求用户每次手动选择；后端 ADK MCP catalog 对空 allowlist 走 enabled catalog auto-discovery，对显式 allowlist 做过滤，并通过 runtime executor 调用；MCP registry 现在携带 `input_schema` 并传入 Eino `ToolInfo`，避免模型无参数契约调用天气工具 | Browser/API 已确认 `/tools` 渲染 `weather/postgres/github` enabled；task detail `7656842727241285632` 和 `7656972508431646720` 已确认 Agent 先用 `tool_search` 再自动调用 `mcp_7656840243668058112_get_weather`；post-fix fresh run `7656986536797274112` 成功，内部事件确认 MCP 参数 `city=北京`，结构化返回 `condition=晴`, `temperature=31`, `humidity_percent=38`, `wind=西南风 2级`；screenshot 归档待补 | Debug env confirms real Eino stdio path (`dry_run=false`, `eino=true`); catalog inspection confirms weather schema has required `city`; pre-fix browser task showed empty MCP args; post-fix unit tests and post-restart runtime smoke confirm schema reaches Eino `ToolInfo` and model sends city argument；raw prompt/tool result不入证据 | `backend/application/mcptool/deerflow_mcp.go`, `backend/api/model/workbench/tool/tool.go`, `backend/application/mcptool/service.go`, `tools/index.tsx`, `workspace-prototype.less`, `tools.test.tsx`, `workbench-composer.tsx`, `adk_mcp_tools.go`, `adk_mcp_runtime_executor.go`; verified by `go test ./application/mcptool -count=1`, `go test ./application/agentthread -run 'TestADKMCP|TestDefaultADKToolProviderCanWireMCP' -count=1`, `go test -gcflags="all=-N -l" ./api/handler/coze -run 'TestWorkbenchMCPToolHandlers' -count=1`, `npx vitest run src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx`, plus post-fix runtime smoke `thread_id=7656986536797274112` | 已完成 |
| TD-COMP-006 | 文件上传 | composer 支持附件并展示上传文件 | Coze 支持附件或明确 P1 差异 | upload screenshot | upload/artifact request | upload/artifact files | 延后(P1) |
| TD-COMP-007 | 追问追加 | 已完成任务可继续追问并保留历史 | Coze append message + queued run + refresh | before/after screenshot | append/create-run request/response | follow-up service/tests | 已完成 |

### E. Skills 页面、内置技能和运行时激活

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-SKILL-001 | 内置 public skills | DeerFlow 从 `skills/public` 自动发现 public skills，默认 public/custom 未配置时 enabled=true | Coze 每个 workspace 自动拥有 DeerFlow public builtin skills，无需手工导入；当前已迁移 22 个 builtin skill 目录和 91 个文件，落库为 `deer_skill`，保留 `SKILL.md` 版本和 scripts/templates/references/assets/evals/LICENSE 等资源，按 frontmatter name 幂等导入 | Skill 页面浏览器截图待 backend restart 后采集 | `GET /api/workbench/skills?space_id=...` 应返回 22 个内置 `deer_skill`；资源可通过 version resources API 读取 | `backend/application/skill/deerflow_builtin_skills.go`, `backend/application/skill/builtin_deerflow/public/**`, `deerflow_builtin_skills_test.go`; verified by `go test ./application/skill ./domain/skill/... ./application/agentthread -run 'Skill|skill' -count=1` and `go test ./api/router/coze -run 'Skill|skill' -count=1` | 已完成 |
| TD-SKILL-002 | Skill 设置页列表/开关 | DeerFlow Settings Skills 只分 Public/Custom，列表显示 name/description/switch，loading 为 muted 文本，empty 为 `还没有技能` + `/skills/custom` 提示 | Coze `技能配置` 页面内容已按 DeerFlow SettingsSection 收敛：无 workspace 顶部引导条，标题 `技能`，说明 `管理 Agent Skill 配置和启用状态。`，仅 Public/Custom，主列表只显示 name/description/switch；版本管理、试运行、删除保留在 `更多` 二级入口 | `TD-SKILL-002-skill-page-deerflow-empty-state.png` | list API 已验证返回 22 个 builtin skills；update 行为由前端单测覆盖 | `skill-page-components.tsx`, `skill/index.tsx`, `workspace-prototype.less`, `skill.test.tsx`; verified by `rushx test -- src/pages/skill/__tests__/skill.test.tsx` | 已完成 |
| TD-SKILL-003 | Skill 创建/导入/导出 | DeerFlow `新建技能` 进入 `/workspace/chats/new?mode=skill`，skill mode 展示 skill-creator 欢迎文案并预填 prompt；`.skill` 产物可通过 `POST /api/skills/install` 安装 | Coze `新建技能` 和空态创建按钮已进入 `/space/:space_id/chats/new?mode=skill`，Workbench 已展示 DeerFlow skill-creator 文案、预填 prompt 并隐藏普通模板；skill-creator prompt 已补充 `.skill`、`create_skill_package`、`present_files` 产物化约束；ADK artifact tools 已新增 `create_skill_package`，服务端生成真正 zip `.skill` 输出产物；`.skill` artifact install 已通过 `POST /api/workbench/skills/install` 适配并接入消息产物卡片/产物抽屉，且已补入 `idl/workbench/skill.thrift` 和 generated `workbenchSkill.InstallSkillFromArtifact`，任务页不再使用手写 fetch；用户导入/安装缺省 `type` 的 `.skill` 现在默认 `CustomSkill`，后端接口验证已返回 `type=5`；Coze 版本资源/导出是保留的扩展管理能力，已覆盖资源列表、资源编辑、版本导出 `.skill` zip 重建，DeerFlow 主线未提供独立版本资源设置页；DeerFlow skill-creator scripts/references/eval resources 尚未完整接入 runtime | `TD-SKILL-003-skill-creator-mode.png`；真实 `.skill` install screenshot 待补 | create-entry browser verified；custom import API verified；runtime package tool tests passed；install backend/frontend tests passed；export/version resource unit verified；generated client contract verified | `skill/index.tsx`, `workbench/index.tsx`, `skill-version-panel.tsx`, `skill-version-panel-hooks.ts`, `adk_artifact_tools.go`, `artifact_output.go`, `workbench_skill_service.go`, `skill.go`, `declaration.go`, `task-artifact-actions.ts`, `task-artifact-message-list.tsx`, `task-artifact-list-item.tsx`, `idl/workbench/skill.thrift`, `frontend/packages/arch/api-schema/src/idl/workbench/skill.ts`; verified by `npm run test -- src/pages/skill/__tests__/skill.test.tsx`, `npm run test -- src/pages/workbench/__tests__/workbench.test.tsx -t 'renders DeerFlow skill-creator mode from the new skill entry'`, `npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts`, `go test ./application/agentthread -run 'Test(Application(CreateSkillPackage|WriteOutputFile|PresentOutputFiles)|ADKArtifactToolCatalog|DefaultADKToolProviderCanWireArtifactTools|RuntimeSkillProvider|ADKSkill)' -count=1`, `go test ./domain/skill/service -run 'TestServiceImportsSkillArchive(AndRecordsSkillMD|WithCustomDefaultType)|TestServiceImportDeclarationDefaultTypeDoesNotOverrideExplicitType' -count=1`, `go test ./application/skill -run 'TestApplicationListSkillVersionResourcesMapsDomainResources|TestApplicationExportSkillVersionBuildsSkillArchive|TestApplicationExportSkillVersionReturnsNotFoundForUnknownVersion|TestApplicationUpdateSkillVersionResourceReturnsNewVersion|TestApplicationImportSkillUsesCustomDefaultType|TestDeerFlowBuiltinSkillServiceSeedsAllPublicSkills|TestDeerFlowBuiltinSkillServiceDoesNotReplaceExistingSkillName|TestDecodeSkillImportContent' -count=1`, `go test ./api/handler/coze -run 'TestInstallSkillFromArtifactHandlerImportsSkillArchive' -count=1` | 已完成 |
| TD-SKILL-004 | Composer 激活 | DeerFlow input-box 支持 skill selector / slash progressive activation | Coze Workbench composer 已支持 `/` 筛选 enabled skills、键盘导航和写入 `/${skill.name} `；后端 `RuntimeSkillProvider` 已按 DeerFlow lower-kebab slash 规则把本轮运行收敛到目标技能，并保持显式 allow-list/ID 授权不被绕过；发送 payload 单测确认 slash message 保留且不被转成配置选择；slash suggestion placement 已修复为 home=`bottom`、detail=`top`，与 DeerFlow 不同输入场景一致 | `TD-SKILL-004-home-slash-suggestions.png`, `TD-SKILL-004-detail-slash-suggestions.png` | create payload covered by unit test；follow-up 复用 `WorkbenchComposer` + `getThreadFollowUpRunInput`，真实网络抓包待补 | `workbench-composer.tsx`, `workbench-composer-controls.tsx`, `index.less`, `skill_provider.go`; verified by `npm run test -- src/pages/workbench/__tests__/workbench.test.tsx -t 'shows DeerFlow slash skill suggestions and inserts the selected skill prefix'`, browser checks for home/detail placement, and `go test ./application/agentthread -run TestRuntimeSkillProvider -count=1` | 已完成 |
| TD-SKILL-005 | 运行时加载 | DeerFlow runtime progressively exposes enabled skill catalog and loads full instructions only on demand or explicit slash activation | Coze Eino ADK runtime 保持同样语义：默认只暴露可用技能目录，显式 selectors、runtime settings、slash 本轮 selector 或 Skill tool-call 才加载技能正文，资源和原始 provider payload 不进公开 API；slash 激活时已在 system prompt 中追加模型可见的 `## Slash Skill Activation` 段落；ADK 会记录 metadata-only `skills.loaded`，任务详情投影为 `可用技能目录 22 个` / `可用技能 “skill-creator”`，不再暗示全部技能正文已加载；匹配 `tool.failed` 的 Skill tool_call 保持失败态并显示具体技能名，如 `“skill-creator” 技能加载失败` | `TD-SKILL-005-runtime-skill-failure-step.png`；前端单测覆盖 `skills.loaded` 目录语义和 Skill tool failed 展示；2026-06-30 in-app browser 选择 tab 超时，需后续补 DOM/截图验收 | live local HTTP/API 已创建 `run_id=7656724465300013056`，status succeeded，DB 事件包含 `skills.loaded` payload: `skill_count=1`, `skill_ids=["7656639981540081664"]`, `skill_names=["skill-creator"]`；浏览器验证 guardrail-blocked selected-skill run 页面不露 raw payload，显示 `“skill-creator” 技能加载失败` | `skill_provider.go`, `adk_middleware.go`, `adk_skill_backend.go`, `adk_skill_preload_middleware.go`, `harness.go`, `task-event-projection.ts`, `task-event-display.ts`, `task-event-tool-display.ts`; verified by `go test -gcflags="all=-N -l" ./application/agentthread -run 'TestADKSkill|TestRuntimeSkillProvider|TestModelStepRunner(MarksSlashActivatedSkillInSystemPrompt|InjectsSkillInstructionsIntoSystemPrompt)' -count=1`, targeted harness/handler tests, `npm run test -- src/pages/tasks/__tests__/tasks.test.tsx -t "skill catalog"`, and `npm run test -- src/pages/tasks/__tests__/tasks.test.tsx` | 已完成 |

### F. Artifacts、导出、Token 和记忆

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-ART-001 | Artifacts 入口 | Header ArtifactTrigger 打开 side panel | Coze 顶部/侧边 Artifacts 入口等价 | panel screenshot | list artifacts response | artifacts panel/files | 已完成 |
| TD-ART-002 | Artifact 预览 | 文件列表、图片/Markdown/链接可预览 | Coze 预览安全、权限和失败态清楚 | preview screenshot | content/signed_url response | artifact preview files/tests | 已完成 |
| TD-ART-003 | Artifact 空态/错误态 | 无 artifact 时不干扰任务流 | Coze 空态/错误态边界清楚 | empty/error screenshot | 404/empty response | artifacts tests | 已完成 |
| TD-EXP-001 | 导出入口 | Header export action | Coze 任务详情提供导出入口 | export button screenshot | export request/response | export service/component | 已完成 |
| TD-EXP-002 | 导出内容 | Markdown: `# title`、`Exported on...Created...`、`## 🧑 User` / `## 🤖 Assistant`；JSON: `title/thread_id/exported_at/messages(type/id/content)` | Coze 导出菜单提供 Markdown / JSON，默认只导出用户可见 user/assistant transcript，过滤思考、tool 消息和内部标记，不写入 token/status/schema | DeerFlow 用户提供导出样例；Coze 浏览器下载样例待人工确认 | 纯前端 Blob 下载，N/A | `task-export-action.tsx` + `task-detail.test.tsx` | 已完成 |
| TD-TOKEN-001 | 全局 token | Header TokenUsageIndicator 可点击查看汇总 | Coze header 显示 run/thread aggregate，视觉保持 `Tokens + total` 紧凑 pill，点击后按 DeerFlow 中文弹层展示 `输入`、`输出`、`总计` 和 `显示方式` | token screenshot + popover DOM | token_usage aggregate | `task-token-usage-indicator.tsx`, `task-detail.test.tsx` | 已完成 |
| TD-TOKEN-002 | 每轮 token | DeerFlow per-turn/per-step usage | Coze task detail 按 assistant message 的 latest `run_id` 映射 run-scoped usage rows，默认 `每轮` 展示 `Tokens / 输入 / 输出 / 总计`，可在弹层切换 `关闭`、`总览`、`每轮`、`调试`，且不渲染 provider/raw_usage/step_name 等敏感字段 | per-message usage screenshot 待人工确认 | `/token_usage` rows grouped by `run_id` | `task-detail-token-usage.ts`, `task-message-token-usage.tsx`, `task-detail-loader.ts`, `task-detail.test.tsx` | 已完成 |
| TD-TOKEN-003 | active token | running 时可显示 pending/active usage | Coze running 时不显示误导性 0 | streaming token screenshot | include_active or event metadata | token loader | 延后(P1) |
| TD-MEM-001 | 记忆入口 | DeerFlow 运行时记忆/设置可见 | Coze `任务记忆` panel 在详情页可用 | memory panel screenshot | list memories response | memory section/tests | 已完成 |
| TD-MEM-002 | 记忆管理 | 搜索/编辑/删除/恢复/导入导出 | Coze 管理能力可用且不泄露 raw metadata | CRUD screenshots | memory API request/response | memory tests | 已完成 |
| TD-MEM-003 | 记忆注入 | 运行时召回影响回答 | Coze memory_retrieval config 生效 | controlled prompt result | run config + memory provider evidence | agentthread memory files/tests | 已完成 |

### G. 文档生成和预览

标准文档生成验证提示词：

```text
请生成一份《武汉3日游攻略》正式文档，包含行程概览、每日安排、预算表、注意事项，并生成一个可在产物面板预览和下载的 Markdown 或 PDF 文档。
```

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-DOC-001 | 文档生成触发 | Agent 接受文档生成提示后产出可识别文档或 artifact | Coze 任务运行生成文档型产物，不只在正文里输出普通文本；已补 ADK `write_file` 工具，只允许写 `/mnt/user-data/outputs/*` 并登记 output file，不混用顶栏任务导出 | prompt、完成态、Artifacts 数量截图待人工浏览器验证 | `write_file` tool result + artifacts list，含 artifact_type/content_type/preview_mode | `adk_artifact_tools.go`、`artifact_output.go`、runtime file service tests | 已完成 |
| TD-DOC-002 | 产物登记 | DeerFlow 生成文件能在 Artifacts 面板出现 | Coze 生成文件写入 `TaskThreadArtifact`，title/file_id/virtual_path/size/content_type 安全可读；已补 ADK `present_files` 工具，只有显式 present 的 output file 才注册 artifact，内部 `.coze/tool-results` offload 不外显 | Artifacts 面板列表截图待人工浏览器验证 | `GET /artifacts` 响应字段摘要待采集；后端 `artifact.presented` 事件不含 object URI | `TaskThreadArtifact` service、runtime output registration、ADK artifact tool tests | 已完成 |
| TD-DOC-003 | Markdown/plain text 预览 | 文本类文档可内联预览 | Coze Markdown/TXT/CSV 类文档可在任务详情安全预览，长文本可截断提示；Markdown 产物走 Markdown renderer，不落到 raw text block | 预览打开截图待人工浏览器验证 | `/content?mode=preview` content-type、truncated 摘要待采集 | `task-artifact-inline-preview.tsx`, `task-artifacts-helpers.ts`, `task-detail.test.tsx` | 已完成 |
| TD-DOC-004 | 表格预览 | CSV/表格内容有可读结构 | Coze CSV/表格预览显示列、行、截断状态，不撑破布局；任务详情点击 CSV artifact 后渲染表格预览而不是 raw text | 表格预览截图待人工浏览器验证 | content preview kind=table/columns/rows 摘要待采集 | `task-artifacts-helpers.ts`, `task-artifact-inline-preview.tsx`, `task-detail.test.tsx`, `task-artifacts-helpers.test.ts` | 已完成 |
| TD-DOC-005 | PDF/Office 文档 | DeerFlow 对二进制文档提供打开/下载路径 | Coze PDF 等二进制文档使用 signed-url 预览/下载路径，不把二进制内容塞入聊天正文或 raw text preview；Office 类继续走安全下载/签名 URL | PDF/Office artifact 操作截图待人工浏览器验证 | `/signed_url?mode=preview|download` 响应摘要待采集 | `task-artifact-actions.ts`, `task-detail.test.tsx` | 已完成 |
| TD-DOC-006 | 图片类文档/图表 | 图片 artifact 可预览 | Coze PNG/JPG/WebP 等图片 artifact 通过 signed-url 内联预览并可下载，SVG 主动内容不走 image preview | 图片预览截图待人工浏览器验证 | signed-url/content-type 摘要待采集 | `task-artifact-actions.ts`, `task-artifacts-helpers.ts`, `task-detail.test.tsx` | 已完成 |
| TD-DOC-007 | HTML/SVG 主动内容安全 | DeerFlow HTML/XHTML/SVG 强制下载，避免同源脚本执行 | Coze HTML/SVG/active content 不在应用 origin 内联执行；即使后端错误标成 text preview，也由前端 MIME family 拦截预览，只保留下载 | 操作截图和浏览器行为说明待人工验证 | content-disposition/preview_mode/safety response 摘要待采集 | `task-artifacts-helpers.ts`, `task-detail.test.tsx`, `task-artifacts-helpers.test.ts` | 已完成 |
| TD-DOC-008 | 下载 | DeerFlow artifact download 可用 | Coze 每个文档产物有下载动作，Markdown/HTML 等文档下载均走 artifact signed-url download，不混用任务导出 | download action screenshot 待人工浏览器验证 | content-disposition 或 signed-url 摘要待采集 | `task-artifact-actions.ts`, `task-artifact-message-list.tsx`, `task-detail.test.tsx` | 已完成 |
| TD-DOC-009 | 预览失败态 | DeerFlow 404/权限/过大有明确提示 | Coze 404、无权限、扫描阻断、过大/截断、格式不支持时显示 bounded error；预览/下载异常不把原始错误、token、URL 或 provider 信息直接渲染到 UI | error/unsupported screenshot 待人工浏览器验证 | 404/403/413/scan blocked response 摘要待采集 | `task-artifact-actions.ts`, `task-detail.test.tsx` | 已完成 |
| TD-DOC-010 | 删除/恢复对预览影响 | DeerFlow 删除/不可见后不再可读 | Coze 隐藏/删除产物后列表不展示，恢复后可重新预览；删除当前预览项会关闭预览，恢复后重新从列表读取 | delete/restore 前后截图待人工浏览器验证 | delete/restore/list/content response 摘要待采集 | `task-artifact-actions.ts`, `task-artifacts-panel.tsx`, `task-detail.test.tsx` | 已完成 |
| TD-DOC-011 | 文档内容脱敏 | 预览不展示 secrets/raw tool/provider payload | Coze 预览、下载和产物列表不展示 raw metadata、provider payload、object URI、tokenized storage path；只展示安全的 `/mnt/user-data/...` 用户空间路径 | UI 检查截图待人工浏览器验证 | redacted response sample 待采集 | `task-artifacts-helpers.ts`, `task-artifact-list-item.tsx`, `task-detail.test.tsx`, `task-artifacts-helpers.test.ts` | 已完成 |
| TD-DOC-012 | 导出与生成文档区分 | DeerFlow thread 导出和 artifact 文档下载是两个入口 | Coze 任务导出、记忆导出、文档下载三类入口文案和内容边界清楚；已对齐 DeerFlow `present_files` 体验，在对话流中展示生成文档文件卡，点击预览/下载继续走任务产物接口，不混用顶栏任务导出 | header/panel/action 截图；文件卡点击行为 | export/download request summaries；artifact signed_url/content 请求 | top-bar/artifact/message files | 已完成 |

### G. 设置、权限、安全和异常态

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-SET-001 | Runtime 设置 | settings/local + context controls | Coze 设置页/运行设置保留 DeerFlow 关键项 | settings screenshot | config payload | settings control/service | 已完成 |
| TD-SET-002 | API 授权 | DeerFlow auth 不影响任务详情 | Coze API 授权保持现状不调整 | settings screenshot | N/A | settings route | 已完成 |
| TD-ERR-001 | 加载态 | history/loading skeleton | Coze detail/list loading 清楚 | throttled screenshot | delayed response | tests | 已完成 |
| TD-ERR-002 | 空态 | new chat welcome | Coze 新建任务/空列表空态清楚 | empty screenshot | empty response | tests | 已完成 |
| TD-ERR-003 | 错误态 | 403/404/500 有可理解提示 | Coze 无权限/不存在/接口失败不跳 legacy agent page | error screenshot | failing response sample | router/detail tests | 已完成 |
| TD-SEC-001 | 输出脱敏 | 不暴露 secrets/raw provider/tool args | Coze 所有 task detail API 和 UI 脱敏 | UI inspection | redacted response samples | backend redaction tests | 已完成 |
| TD-SEC-002 | 多租户 | 只能看自己的 thread/task | Coze space/user 权限生效 | cross-account/API sample | 403/404 response | auth tests | 已完成 |

## 证据采集规范

证据目录：

```text
docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/
```

推荐文件命名：

- `screenshots/deerflow/TCASE-short-name.png`
- `screenshots/coze/TCASE-short-name.png`
- `network/deerflow/TCASE-endpoint.json`
- `network/coze/TCASE-endpoint.json`
- `notes/TCASE-findings.md`

接口样本记录最少包含：

- method/path/status。
- query/body 的安全摘要。
- response 顶层字段、关键 id、status、usage、event type。
- SSE event 的 `event`、`data` 顶层字段摘要。
- 明确写出已脱敏字段：credential、token、prompt 原文、model output 全文、
  tool args/results、object URI、provider raw body。

截图最少覆盖：

- DeerFlow desktop。
- Coze desktop。
- 有布局风险的 case 增加 mobile/narrow viewport。
- 复杂交互需要展开态和收起态各一张。

## 首轮验证计划

| Step | Scope | Output | Status |
| --- | --- | --- | --- |
| P0-A | 建立测试 case 和证据目录 | 本文件 + evidence README，含 `TD-DOC-*` 文档生成/预览 case | 已完成 |
| P0-B | DeerFlow 参考页基线 | DOM 摘要、截图、网络接口列表 | 进行中 |
| P0-C | Coze 目标页基线 | DOM 摘要、截图、网络接口列表 | 进行中 |
| P0-D | 任务详情差异分级 | P0 待开发/待验收列表 | 待执行 |
| P0-E | 按 case 修复 P0 差异 | 每个修复绑定 case id、测试和提交 | 待执行 |
| P0-F | 回归验收 | 所有 P0 case 证据齐全 | 待执行 |

## 首轮页面基线记录

采集时间：2026-06-28。

证据文件：

- DeerFlow screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/BASELINE-task-detail.png`
- Coze screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/BASELINE-task-detail.png`
- DeerFlow DOM summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/BASELINE-deerflow-dom-summary.json`
- Coze DOM summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/BASELINE-coze-dom-summary.json`

DeerFlow 参考页可见能力：

- Header 显示标题 `Mermaid 绘制时序图与架构图`。
- Header 右侧有 `Tokens 21.4K`、`导出`。
- 左侧有 `新对话`、`对话`、`智能体`、最近对话列表。
- 消息流为 chat-like 详情，底部 composer 有文件上传、`Pro` 模式和模型选择。
- `思考` inline block 可见。
- Mermaid 已渲染为 SVG，raw fenced Mermaid 不可见。
- Assistant turn 底部有 per-turn token 摘要。

Coze 目标页可见能力：

- 任务命名保留，侧边栏有 `新建任务`、`全部任务`、`我的任务`。
- Header 显示任务标题、`已完成`、Token、`产物 0`、收藏/分享/通知。
- 完成态已收敛，不显示取消任务；执行流程显示 `2/2 已完成 · 100%`。
- Mermaid 有两个 `data-testid="task-mermaid-diagram"`，状态均为 `ready`，
  SVG 数量为 2，raw fenced Mermaid 不可见。
- 底部 composer 已作为 dock，不再属于聊天记录滚动流；最新代码已切到
  DeerFlow presentation，去掉 `Auto/Ask/Agent` 分段和运行设置按钮，保留
  `拓展`、`@`、链接和附件入口，发送按钮改为 DeerFlow 绿色圆形图标。
- `运行诊断`、`安全审计`、`任务记忆` 已移入 header `详情` inspector，不再
  直接铺在聊天对话记录中。

首轮差异：

- `TD-LAYOUT-001`: Coze 主内容仍需继续向 DeerFlow chat-like 详情收敛。
  `TD-LAYOUT-003` 已完成：运行诊断、安全审计、任务记忆移入 header
  `详情` inspector，聊天记录区保持纯净。
- `TD-COMP-001`: 布局和代码已完成，状态回到待验收。DeerFlow 和 Coze
  均为页面不滚、消息区域内部滚动、底部 composer 固定；本轮进一步把
  task detail composer 改成 DeerFlow presentation：去掉 `Auto/Ask/Agent`，
  默认展示 `Pro` 模式触发器，保留 `拓展` 和 `@`，发送按钮改为绿色圆形
  图标。仍需用户人工浏览器验收最终视觉。
- `TD-COMP-003`: 模式选择已按 DeerFlow 源码拆为
  `flash/thinking/pro/ultra`，默认 `Pro`。运行配置现在写入
  `thinking_enabled`、`is_plan_mode`、`subagent_enabled`；`reasoning_effort`
  只在运行设置显式启用 reasoning 时下发，避免模型不支持 provider
  reasoning 参数时直接失败。旧 WorkbenchChat 枚举只作为 legacy 兼容映射。
  本轮 in-app browser 已确认首页 composer 使用 DeerFlow presentation 并显示
  `Pro`，mode menu 截图待人工补充。
- `TD-COMP-005`: MCP 自动调用已完成 post-fix browser/API path 验证，Agent
  会先用 `tool_search` 再自动调用 weather MCP；已修复 MCP registry 未把
  `input_schema` 传给 Eino `ToolInfo` 的问题，并在 fresh run
  `7656986536797274112` 确认模型带 `city=北京` 调用天气工具。
- `TD-MSG-005`: Coze 已补 inline `思考` 前端适配，从 ADK
  `message.completed` / answer payload 中提取 reasoning 并隐藏 provider
  signature/raw payload；当前缺真实 reasoning 样本浏览器截图，保持待验收。
- `TD-HDR-003` / `TD-EXP-001`: Coze 顶栏已收敛为 `Tokens`、`导出`、`详情`
  主操作，移除顶栏 `产物 0` 和 `任务详情 › 状态` 面包屑式文案。
- `TD-EXP-002`: Coze 已按 DeerFlow 用户提供样例补齐导出二级菜单和
  Markdown / JSON 两种格式。Markdown 使用 `# title`、`Exported on...Created...`
  和 `## 🧑 User` / `## 🤖 Assistant` 结构；JSON 使用
  `title/thread_id/exported_at/messages(type/id/content)`，默认过滤思考、
  tool 消息、上传文件内部标记，不写入 Coze 自有 `schema`、`token_usage`
  或任务状态字段。真实浏览器下载样例仍待人工确认。
- `TD-TOKEN-002`: 已按 DeerFlow 补前端代码和单测：顶部 Token 弹层提供
  `关闭`、`总览`、`每轮`、`调试` 显示方式，默认 `每轮`；assistant turn
  根据最新 `run_id` 展示 `Tokens / 输入 / 输出 / 总计` 汇总，敏感的
  provider/raw_usage/step_name 不进 UI。真实浏览器截图仍待人工确认。
- `TD-COMP-006`: DeerFlow composer 有文件上传入口；Coze 目标页已补同位
  附件图标入口，但真实上传、文件列表和 run config 绑定仍需单独验证。
- `TD-DOC-001/002`: 已补后端代码与单测：ADK `write_file` 只写
  `/mnt/user-data/outputs/*` 并登记 output file，`present_files` 显式把
  output file 注册为 artifact，事件和工具返回不暴露 `agent-runtime`
  object URI；标准文档生成提示词、Artifacts 面板截图和真实接口响应仍需
  人工浏览器验收。
- `TD-DOC-003~011`: 文档预览、下载、安全 fallback 已补 case，但仍需在
  真实 artifact 样本上逐项验收。

接口采集状态：

- 浏览器只读环境中 `fetch` / `XMLHttpRequest` 不可用，无法直接从页面上下文
  发起同源 API 采样。
- 直接 `curl` 两边 API 均返回 401，说明接口采样需要浏览器登录态 HAR、
  DevTools 网络采集、或受控测试登录 token。
- 因此本轮 API evidence 只记录接口路径矩阵和采集阻塞原因，不把 401 视为
  功能缺失。后续每个 `TD-*` case 完成前仍必须补上安全请求/响应摘要。

## 当前已知结论

- `TD-HDR-002` 和 `TD-RUN-006` 已通过最近修复：完成后的任务详情以最新
  top-level terminal run 驱动，不再错误显示运行中。
- `TD-HDR-003` 和 `TD-TOKEN-001` 已通过最近修复：顶栏以 DeerFlow 风格的
  `Tokens + total` 紧凑 pill 展示聚合用量，并支持点击查看
  Input/Output/Total 明细；右侧主操作保留 `导出` 和 `详情`，不再显示
  `产物 0` 或 `任务详情 › 已完成`。
- `TD-EXP-001` 已通过最近修复：任务详情顶栏提供 DeerFlow 风格导出入口。
- `TD-EXP-002` 已代码对齐 DeerFlow 用户提供的 Markdown / JSON 导出样例，
  仍需真实浏览器下载样例做人工验收。
- `TD-MD-001` 和 `TD-MD-002` 已通过最近修复：Coze 任务详情能把
  `sequenceDiagram` 和 `flowchart` Mermaid fenced block 渲染为 SVG，并在
  每个图块右上提供 SVG 下载和 Mermaid 源码复制按钮。
- `TD-MSG-001/002` 和 `TD-LAYOUT-001` 已完成一轮浏览器对照：Coze 用户
  气泡、Markdown/Mermaid 渲染、底部 composer 停靠基本可用；助手消息已移除
  `普通回答 · Agent/Ark` 结果标签。
- `TD-FLOW-001` 已按用户反馈修复：保留 `执行流程` 标题，并用轻量 feed
  直接展示每一步做了什么。
- `TD-FLOW-001` 继续对齐 DeerFlow 步骤来源：前端投影已支持从 assistant
  `message.completed.tool_calls` 展开工具步骤，并用匹配的 `tool_call_id`
  工具结果判断完成态，避免只靠 `tool.completed` 兜底文案导致标题、路径和
  DeerFlow 不一致。
- `TD-SKILL-005` 已补 ADK slash activation 回归：当 `/skill-name` 将本轮
  运行收敛到一个 inline Skill 时，Eino ADK 在第一次模型调用前预加载该
  Skill 指令，匹配 DeerFlow `SkillActivationMiddleware` 的显式激活语义。
  已用 `TestADKSkillMiddlewarePreloadsSlashActivatedInlineSkill` 锁定。
- `TD-MSG-005` 已补代码与单测：inline `思考` 折叠块保留，正文隐藏
  `<think>` 原始标签，`message.completed` 流程行只显示安全摘要；下一步补
  真实 reasoning 样本浏览器验收。
- `TD-TOKEN-002` 每轮/每消息 token 显示已代码完成并通过详情页单测；
  待用户人工浏览器确认后再改为 `已完成`。
- `TD-COMP-001/003` 本轮已补代码与单测：任务详情追问框使用 DeerFlow
  presentation，去掉 `Auto/Ask/Agent` 分段，保留 `拓展`、`@`、附件/链接
  入口，默认显示 `Pro` 模式，发送按钮改为绿色圆形图标；`TD-COMP-003`
  又补充了四态运行上下文透传、默认不下发 `reasoning_effort`、显式 reasoning
  才下发 provider 参数，以及 Ultra 交互发送测试；下一步由用户人工确认实际
  页面视觉。
- `TD-SKILL-*`、`TD-COMP-004`、`TD-COMP-005` 已从 P0 验收池收口：
  2026-06-30 用户确认 Skills 和 MCP 功能测试没有问题，首版不再继续逐项验证。
  剩余真实截图补证、slash 网络抓包、`.skill` 安装深测、skill-creator
  scripts/references/eval resources 可用性，以及 GitHub/Postgres 外部凭据类
  MCP 深测统一转二期优化。
- `TD-LAYOUT-001`、`TD-HDR-001`、`TD-HDR-003`、`TD-COMP-001` 已按当前真实
  任务详情页收口：浏览器 DOM 证据确认标题为 `java-learning-roadmap`，Header
  动作为 `Tokens`、`导出`、`详情`、`收藏`、`分享`，不再显示
  `任务详情 › 已完成`；底部 Composer 固定在详情页底部，显示 `Pro`、`拓展`、
  模型和 `@`，不再显示 `Auto/Ask/Agent` 或 `运行设置`。截图：
  `screenshots/coze/TD-LAYOUT-HDR-COMP-current-7657061782099329024.png`。
- `TD-NAV-001/002` 已收口：`/space/7656275718757679104/tasks/7657061782099329024`
  和 `/space/7656275718757679104/chats/new` 均返回 HTTP 200；侧边最近任务
  与全部任务列表的路由行为由 `workspace-sub-menu.test.tsx` 和
  `tasks.test.tsx` 覆盖，确认打开 canonical `/space/:space/tasks/:thread`
  路径且 `legacy_task_id="0"` 不回旧 agent 页。
- `TD-RUN-001~005`、`TD-COMP-007`、`TD-ART-001~003`、`TD-EXP-002`、
  `TD-TOKEN-002`、`TD-SET-001/002`、`TD-ERR-001~003`、`TD-SEC-001/002`
  已按 P0 主 tracker 的实现与验证证据收口：任务创建/流式/刷新/停止/重试、
  追问追加、Artifacts 面板和预览边界、导出内容、每轮 token、设置入口、
  加载/空/错误态，以及脱敏/权限边界均已有对应单测、后端测试或冒烟记录；
  细化矩阵不再重复卡首版。
- `TD-DOC-*` 文档生成和预览已经纳入 P0 验收矩阵，后续必须按文档生成
  标准提示词做专项对比。
- `TD-DOC-001~011` 已按 P0 首版收口：当前真实任务页已有生成文档产物与右侧
  Markdown 预览证据；前端 `task-detail.test.tsx` 和
  `task-artifacts-helpers.test.ts` 覆盖 Markdown/plain text、CSV table、
  PDF/signed-url、图片 signed-url、HTML/SVG active-content 拦截、下载、
  失败态、删除/恢复和脱敏边界；后端 ADK artifact 工具测试覆盖
  `write_file` / `present_files` / artifact tool catalog。更丰富的真实 MIME
  样本截图和 Network 响应摘要转入二期样本库，不再阻塞 P0。
- `TD-DOC-012` 已继续对齐 DeerFlow `present_files` 展示模型：Coze 不再把
  线程全部 artifact 卡片统一追加到最新对话末尾，而是按 artifact `run_id`
  渲染到对应 assistant turn；追问生成同名文档时只默认预览最新可预览产物，
  旧产物保留在原轮次卡片中。2026-06-30 已补真实浏览器 DOM/截图证据：
  artifact message card 只显示 `下载`，不再显示扫描审核的 `放行/隔离/阻断`
  操作；非待办文档场景不常驻 To-dos；宽屏右侧预览按 DeerFlow 60/40 方向
  自适应。artifact API 响应摘要改入总冒烟/二期证据归档，不再阻塞 P0。
- `TD-DOC-003/008` 已补代码与单测：Markdown 产物在右侧预览中使用 Markdown
  renderer，不再落到 raw text block；超长预览显示截断提示；Markdown 和 HTML
  文档下载都通过 artifact signed-url `mode=download`，不混用顶部任务导出。
  真实浏览器截图和 API 摘要仍待补证据。
- `TD-DOC-005/007/009` 已补代码与单测：PDF 预览走 signed-url/open 路径；
  HTML/SVG/active content 不允许内联 text/image 预览，主动内容文件下载走
  signed-url；artifact preview/download 失败只显示 bounded 用户文案，不把
  原始错误、token 或长 provider payload 渲染到 UI。真实浏览器截图和 API
  摘要仍待补证据。
- `TD-DOC-004/006/010` 已补齐或确认测试覆盖：CSV artifact 点击预览后渲染
  表格；PNG 图片通过 signed-url inline image preview；产物删除、撤销恢复和
  deleted-list 恢复均刷新列表并保持预览状态一致。真实浏览器截图和 API 摘要
  仍待补证据。
- `TD-DOC-011` 已补代码与单测：产物抽屉不再直接渲染非用户空间
  `virtual_path`，避免 `agent-runtime://...`、tokenized object URI 或 raw
  provider metadata 出现在 UI；安全的 `/mnt/user-data/outputs|workspace/...`
  路径仍可展示。真实浏览器截图和后端 redaction sample 仍待补证据。
- Runtime Doctor 是 Coze 运维增强，不作为 DeerFlow 视觉主线阻塞项；若影响
  页面信息密度，按 P1 或折叠展示处理。

## 执行纪律

- 每个后续开发任务必须引用本文件中的 case id。
- 如果新发现 DeerFlow 可见能力，本文件先补 case，再改代码。
- 如果新发现 Coze 增强但 DeerFlow 不可见，写入 P1/P2，不进入当前 P0。
- 浏览器验证先对比 DeerFlow，再对比 Coze，同一 case 的截图和网络样本成对
  落盘。
