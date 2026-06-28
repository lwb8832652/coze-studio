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
| 菜单和路由 | `workspace/chats/new`、`workspace/chats/:id`、最近对话 | `新建任务`、`全部任务`、`我的任务`、`tasks/:id` | 待验收 | 命名保留任务方式，行为对齐。 |
| 任务详情布局 | Chat-like thread surface, header actions, bottom composer | Canonical task detail page | 进行中 | Mermaid 和终态已修复；整体 chat-like 视觉仍需对比。 |
| 消息/思考/Markdown | user/assistant turn, inline reasoning, Markdown/Mermaid | task messages + answer/result markdown | 进行中 | inline thinking 保留，需对齐 DeerFlow 展示方式。 |
| 执行流 | LangGraph run stream/events | Workbench run events + execution cards | 待验收 | Coze 可显示更多安全事件；P0 要先保证 DeerFlow 可见体验。 |
| 追问/停止/重试 | bottom composer send/stop, run cancel/retry | follow-up, cancel, retry task run | 待验收 | 需要页面手动/自动对比。 |
| Artifacts | header trigger + side panel + file links | Task artifacts panel | 待验收 | 需验证 UI、接口、文件预览。 |
| 文档生成/预览 | Agent 生成文件后进入 Artifacts，可内联预览或安全下载 | 任务运行生成文档产物，任务详情可预览/下载 | 待验收 | 独立 `TD-DOC-*` case，不能只看 Artifacts 按钮存在。 |
| 导出 | thread export action | 任务导出能力 | 待开发 | 当前任务详情尚未按 DeerFlow 导出入口完整对齐。 |
| Token 用量 | global token button, per-turn usage | run aggregate indicator, child aggregate | 待验收 | per-message/per-turn 展示需对齐。 |
| Skills | slash/progressive activation, Skills page/API | 技能配置 + Eino runtime loading | 待验收 | 重点验 task composer activation。 |
| MCP/Tools | MCP settings, tool catalog, tool calls | 工具配置 + Eino MCP runtime | 待验收 | 重点验可见配置和 task-detail safe event。 |
| 记忆 | runtime memory + settings | `任务记忆` panel + retrieval | 待验收 | Coze 管理面板更重，需确认 DeerFlow 可见等价。 |
| 设置 | model/mode/runtime settings | Workbench runtime settings + 设置 | 待验收 | 不改 API 授权。 |
| 后端 API | `/api/threads`, `/runs/stream`, `/messages`, `/token-usage` | `/api/workbench/task_threads` 系列 | 进行中 | 路径不同，语义要对齐。 |
| 代码方向 | Next.js + LangGraph SDK + Python gateway | React/Semi + Go Hertz + Eino ADK | 进行中 | Eino-first；不引 Python sidecar。 |

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
| Skills APIs | `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/skills.py` |
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
| Follow-up composer | `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts` |
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
| TD-NAV-001 | 详情路由打开 | `/workspace/chats/:thread_id` 直接进入详情 | `/space/:space_id/tasks/:thread_id` 直接进入任务详情 | 两边 URL 截图，刷新后仍可用 | detail 接口 200 | 两边 page/router 文件 | 待验收 |
| TD-NAV-002 | 最近列表进入 | 侧边栏最近对话点开同一 thread | `我的任务` 点开同一 thread，不跳 agent legacy page | 点击前后 URL/标题截图 | list/detail 接口 thread id 一致 | Coze sidebar/list service | 待验收 |
| TD-LAYOUT-001 | 整体布局 | 顶部 header、中间消息流、底部 composer、右侧/弹出 Artifacts | 保留任务命名，但布局信息密度和交互位置对齐 | desktop full-page screenshot | N/A | detail/top-bar/follow-up/artifacts files | 进行中 |
| TD-LAYOUT-002 | 响应式 | DeerFlow 窄屏不遮挡消息和 composer | Coze 窄屏不重叠、不溢出 | 390px/768px 截图 | N/A | LESS/CSS and component layout | 待验收 |
| TD-LAYOUT-003 | 对话记录纯净度 | 聊天记录区只承载用户消息、助手回复、inline 思考、工具/任务状态和产物引用 | 运行诊断、安全审计、任务记忆不得直接铺在聊天对话记录中；应进入 header action、侧栏 inspector、弹层或折叠二级面板 | main content DOM + inspector open DOM | N/A | `task-detail-inspector.tsx`, `detail.tsx`, `task-top-bar.tsx`, `task-detail.test.tsx` | 已完成 |
| TD-HDR-001 | 标题 | Header 显示 thread title | Header 显示任务 title，不能只显示 ID | header 截图 | detail response title | loader/top-bar | 待验收 |
| TD-HDR-002 | 状态和进度 | running/ready 状态清楚 | terminal run 显示 `已完成` 且无 stale running | screenshot/DOM | runs response latest top-level status | loader/detail tests | 已完成 |
| TD-HDR-003 | Header actions | Token、导出、Artifacts 在右上 | Token、导出、Artifacts 对齐；导出若缺失标 P0 缺口 | action 区截图 | token/artifact/export APIs | top-bar/artifacts/export files | 待开发 |

### B. 消息流、思考和 Markdown

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-MSG-001 | 用户消息 | 用户 turn 右侧气泡/块，纯文本安全显示 | Coze 显示用户输入，不当 Markdown 执行 | message screenshot | messages API role/order | DeerFlow message item, Coze messages mapping | 待验收 |
| TD-MSG-002 | 助手消息 | assistant turn inline，Markdown 渲染 | Coze 结果按消息流显示，不只像报告卡片 | screenshot | messages/answer payload | detail/markdown files | 进行中 |
| TD-MSG-003 | 历史分页 | DeerFlow 可加载更多 history | Coze 任务历史刷新后完整可恢复 | load-more/reopen screenshot | messages pagination params/response | service/loader | 待验收 |
| TD-MSG-004 | 流式中状态 | streaming indicator 和停止按钮同步 | Coze run_events 流式更新步骤和答案 | running screenshot/video if needed | SSE event samples | stream hook/event projection | 待验收 |
| TD-MSG-005 | inline 思考块 | Reasoning trigger + collapsible content 保留 | Coze 保留 inline 思考块，不移除 | expanded/collapsed screenshot | event/message reasoning fields | DeerFlow reasoning, Coze event projection/detail | 待开发 |
| TD-MSG-006 | 复制和反馈 | assistant turn 有复制/反馈操作 | Coze 至少有复制；反馈如不做则 P1 | hover/action screenshot | feedback API if present | message toolbar/action files | 差异待确认 |
| TD-MSG-007 | 代码块/表格/链接 | Markdown 组件渲染稳定 | Coze Markdown 不破坏布局，链接安全打开 | markdown matrix screenshot | message body sample | markdown components/tests | 待验收 |
| TD-MD-001 | Mermaid sequenceDiagram | DeerFlow 输出 SVG/图形，不显示 raw fence | Coze 渲染 sequenceDiagram SVG | screenshot + DOM svg count | assistant message content | markdown component test | 已完成 |
| TD-MD-002 | Mermaid flowchart | DeerFlow 输出 SVG/图形，不显示 raw fence | Coze 渲染 flowchart SVG | screenshot + DOM svg count | assistant message content | markdown component test | 已完成 |
| TD-MD-003 | Mermaid 错误态 | 错误图不炸页面，可显示源码/错误提示 | Coze Mermaid 失败时安全 fallback | invalid Mermaid screenshot | message body sample | component error test | 待验收 |

### C. 执行流和运行控制

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-RUN-001 | 创建并运行 | 发送后创建 thread/run 并进入 streaming | 新建任务后创建 thread/run，进入任务详情 | new-task flow screenshot | create/run request/response | workbench page + handler tests | 待验收 |
| TD-RUN-002 | 事件流 | LangGraph SSE 输出 messages/events | Coze SSE 输出 safe run_events，UI 实时更新 | network SSE sample | SSE raw event sample | run-event-stream/event projection | 待验收 |
| TD-RUN-003 | 刷新重连 | running/recent page refresh 后继续看状态 | Coze refresh 后状态和输出可恢复 | refresh screenshot | list events/runs/messages | loader/stream files | 待验收 |
| TD-RUN-004 | 停止 | stop 按钮取消 active run | `取消任务` 只在 active run 显示，成功终止 | running -> canceled screenshot | cancel request/response | run action hook + handler tests | 待验收 |
| TD-RUN-005 | 失败重试 | failed/canceled run 可 retry | `重试任务` 创建新 top-level retry run | failed/canceled screenshot | retry request/response metadata | retry hook + handler tests | 待验收 |
| TD-RUN-006 | 终态收敛 | 完成后页面不再显示 running | Coze 最新 top-level terminal run 驱动 header/progress | completed screenshot | runs latest status | loader/detail tests | 已完成 |
| TD-FLOW-001 | 步骤/时间线 | DeerFlow 可见思考/任务/工具进度 | Coze 显示安全执行流程，但不偏离主消息体验 | execution flow screenshot | events list | event projection tests | 待验收 |
| TD-FLOW-002 | 工具事件 | tool 调用状态可读，不泄露敏感参数 | Coze tool card 隐藏 args/results/raw provider | tool event screenshot | redacted event response | safety tests | 待验收 |
| TD-FLOW-003 | 子智能体事件 | subagent 状态清楚 | Coze 子智能体卡片只显示安全 metadata | subagent case screenshot | child run/events/token response | subagent files/tests | 待验收 |

### D. Composer、追问和附件

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-COMP-001 | 底部输入框 | 固定底部，未遮挡消息 | Coze task detail 底部追问输入体验对齐 | browser scroll metrics + DOM | N/A | `detail.tsx`, `workspace-prototype.less`, `task-detail.test.tsx` | 已完成 |
| TD-COMP-002 | 模型选择 | model selector 可见且和上下文绑定 | Coze 运行设置/模型选择可修改 run config | model dropdown screenshot | run config request | settings control/service | 待验收 |
| TD-COMP-003 | 模式选择 | flash/thinking/pro/ultra 或同等能力 | Coze Auto/模式和 reasoning 参数可用 | mode control screenshot | config payload | runtime settings | 待验收 |
| TD-COMP-004 | Skill slash/渐进激活 | 输入 `/` 可筛选 Skill | Coze 技能选择/启用进入 run config | slash/select screenshot | enable_skills payload | workbench/task settings | 待验收 |
| TD-COMP-005 | MCP/tool 激活 | 可选择 MCP/tool，运行时调用 | Coze 工具配置进入 run config | tool selection screenshot | enable_mcp payload | tools/service/runtime | 待验收 |
| TD-COMP-006 | 文件上传 | composer 支持附件并展示上传文件 | Coze 支持附件或明确 P1 差异 | upload screenshot | upload/artifact request | upload/artifact files | 差异待确认 |
| TD-COMP-007 | 追问追加 | 已完成任务可继续追问并保留历史 | Coze append message + queued run + refresh | before/after screenshot | append/create-run request/response | follow-up service/tests | 待验收 |

### E. Artifacts、导出、Token 和记忆

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-ART-001 | Artifacts 入口 | Header ArtifactTrigger 打开 side panel | Coze 顶部/侧边 Artifacts 入口等价 | panel screenshot | list artifacts response | artifacts panel/files | 待验收 |
| TD-ART-002 | Artifact 预览 | 文件列表、图片/Markdown/链接可预览 | Coze 预览安全、权限和失败态清楚 | preview screenshot | content/signed_url response | artifact preview files/tests | 待验收 |
| TD-ART-003 | Artifact 空态/错误态 | 无 artifact 时不干扰任务流 | Coze 空态/错误态边界清楚 | empty/error screenshot | 404/empty response | artifacts tests | 待验收 |
| TD-EXP-001 | 导出入口 | Header export action | Coze 任务详情提供导出入口 | export button screenshot | export request/response | export service/component | 待开发 |
| TD-EXP-002 | 导出内容 | 导出包含消息/图表/关键结果 | Coze 导出内容等价且脱敏 | exported file sample | export payload | export tests | 待开发 |
| TD-TOKEN-001 | 全局 token | Header TokenUsageIndicator | Coze header 显示 run/thread aggregate | token screenshot | token_usage aggregate | token indicator tests | 待验收 |
| TD-TOKEN-002 | 每轮 token | DeerFlow per-turn/per-step usage | Coze task detail 每条消息/步骤显示等价信息 | per-message usage screenshot | usage rows grouped by message/run | token usage files/tests | 待开发 |
| TD-TOKEN-003 | active token | running 时可显示 pending/active usage | Coze running 时不显示误导性 0 | streaming token screenshot | include_active or event metadata | token loader | 差异待确认 |
| TD-MEM-001 | 记忆入口 | DeerFlow 运行时记忆/设置可见 | Coze `任务记忆` panel 在详情页可用 | memory panel screenshot | list memories response | memory section/tests | 待验收 |
| TD-MEM-002 | 记忆管理 | 搜索/编辑/删除/恢复/导入导出 | Coze 管理能力可用且不泄露 raw metadata | CRUD screenshots | memory API request/response | memory tests | 待验收 |
| TD-MEM-003 | 记忆注入 | 运行时召回影响回答 | Coze memory_retrieval config 生效 | controlled prompt result | run config + memory provider evidence | agentthread memory files/tests | 待验收 |

### F. 文档生成和预览

标准文档生成验证提示词：

```text
请生成一份《武汉3日游攻略》正式文档，包含行程概览、每日安排、预算表、注意事项，并生成一个可在产物面板预览和下载的 Markdown 或 PDF 文档。
```

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-DOC-001 | 文档生成触发 | Agent 接受文档生成提示后产出可识别文档或 artifact | Coze 任务运行生成文档型产物，不只在正文里输出普通文本 | prompt、完成态、Artifacts 数量截图 | run events + artifacts list，含 artifact_type/content_type/preview_mode | ADK output/offload + artifact registration files/tests | 待验收 |
| TD-DOC-002 | 产物登记 | DeerFlow 生成文件能在 Artifacts 面板出现 | Coze 生成文件写入 `TaskThreadArtifact`，title/file_id/virtual_path/size/content_type 安全可读 | Artifacts 面板列表截图 | `GET /artifacts` 响应字段摘要 | `TaskThreadArtifact` IDL、handler、repository tests | 待验收 |
| TD-DOC-003 | Markdown/plain text 预览 | 文本类文档可内联预览 | Coze Markdown/TXT/CSV 类文档可在任务详情安全预览，长文本可截断提示 | 预览打开截图 | `/content?mode=preview` content-type、truncated 摘要 | inline preview helpers/tests | 待验收 |
| TD-DOC-004 | 表格预览 | CSV/表格内容有可读结构 | Coze CSV/表格预览显示列、行、截断状态，不撑破布局 | 表格预览截图 | content preview kind=table/columns/rows 摘要 | `task-artifact-inline-preview.tsx` tests | 待验收 |
| TD-DOC-005 | PDF/Office 文档 | DeerFlow 对二进制文档提供打开/下载路径 | Coze PDF/DOCX/XLSX/PPTX 至少提供安全下载或签名 URL；若支持内嵌预览需记录 renderer | PDF/Office artifact 操作截图 | `/signed_url?mode=preview|download` 或 `/content?mode=download` 响应摘要 | signed-url/content service + backend handler tests | 待验收 |
| TD-DOC-006 | 图片类文档/图表 | 图片 artifact 可预览 | Coze PNG/JPG/WebP 等图片 artifact 可内联预览并可下载 | 图片预览截图 | signed-url/content-type 摘要 | image preview branch tests | 待验收 |
| TD-DOC-007 | HTML/SVG 主动内容安全 | DeerFlow HTML/XHTML/SVG 强制下载，避免同源脚本执行 | Coze HTML/SVG/active content 不在应用 origin 内联执行，只允许下载或安全隔离预览 | 操作截图和浏览器行为说明 | content-disposition/preview_mode/safety response 摘要 | active-content safety tests | 待验收 |
| TD-DOC-008 | 下载 | DeerFlow artifact download 可用 | Coze 每个文档产物有下载动作，文件名和 MIME 合理 | download action screenshot | content-disposition 或 signed-url 摘要 | list item/service tests | 待验收 |
| TD-DOC-009 | 预览失败态 | DeerFlow 404/权限/过大有明确提示 | Coze 404、无权限、扫描阻断、过大/截断、格式不支持时显示 bounded error | error/unsupported screenshot | 404/403/413/scan blocked response 摘要 | panel/error tests | 待验收 |
| TD-DOC-010 | 删除/恢复对预览影响 | DeerFlow 删除/不可见后不再可读 | Coze 隐藏/删除产物后列表不展示，恢复后可重新预览；底层安全策略不绕过 | delete/restore 前后截图 | delete/restore/list/content response 摘要 | artifact lifecycle tests | 待验收 |
| TD-DOC-011 | 文档内容脱敏 | 预览不展示 secrets/raw tool/provider payload | Coze 预览和下载接口不得泄露 credentials、tool args/results、raw provider bodies、object keys | UI 检查截图 | redacted response sample | backend redaction/safety tests | 待验收 |
| TD-DOC-012 | 导出与生成文档区分 | DeerFlow thread 导出和 artifact 文档下载是两个入口 | Coze 任务导出、记忆导出、文档下载三类入口文案和内容边界清楚 | header/panel action 截图 | export/download request summaries | top-bar/artifact/memory export files | 待开发 |

### G. 设置、权限、安全和异常态

| ID | 功能点 | DeerFlow 基线 | Coze 期望 | 前端证据 | 后端证据 | 代码证据 | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TD-SET-001 | Runtime 设置 | settings/local + context controls | Coze 设置页/运行设置保留 DeerFlow 关键项 | settings screenshot | config payload | settings control/service | 待验收 |
| TD-SET-002 | API 授权 | DeerFlow auth 不影响任务详情 | Coze API 授权保持现状不调整 | settings screenshot | N/A | settings route | 待验收 |
| TD-ERR-001 | 加载态 | history/loading skeleton | Coze detail/list loading 清楚 | throttled screenshot | delayed response | tests | 待验收 |
| TD-ERR-002 | 空态 | new chat welcome | Coze 新建任务/空列表空态清楚 | empty screenshot | empty response | tests | 待验收 |
| TD-ERR-003 | 错误态 | 403/404/500 有可理解提示 | Coze 无权限/不存在/接口失败不跳 legacy agent page | error screenshot | failing response sample | router/detail tests | 待验收 |
| TD-SEC-001 | 输出脱敏 | 不暴露 secrets/raw provider/tool args | Coze 所有 task detail API 和 UI 脱敏 | UI inspection | redacted response samples | backend redaction tests | 待验收 |
| TD-SEC-002 | 多租户 | 只能看自己的 thread/task | Coze space/user 权限生效 | cross-account/API sample | 403/404 response | auth tests | 待验收 |

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
- 底部有追问 composer、Auto/Ask/Agent、拓展、运行设置、发送；修复后
  composer 作为底部 dock，不再属于聊天记录滚动流。
- `运行诊断`、`安全审计`、`任务记忆` 已移入 header `详情` inspector，不再
  直接铺在聊天对话记录中。

首轮差异：

- `TD-LAYOUT-001`: Coze 主内容仍需继续向 DeerFlow chat-like 详情收敛。
  `TD-LAYOUT-003` 已完成：运行诊断、安全审计、任务记忆移入 header
  `详情` inspector，聊天记录区保持纯净。
- `TD-COMP-001`: 已完成。DeerFlow 和 Coze 均为页面不滚、消息区域内部
  滚动、底部 composer 固定；后续只做视觉细节继续对齐。
- `TD-MSG-005`: DeerFlow 有 inline `思考`，Coze 目标页当前未检测到 reasoning
  展示信号，需要补齐或确认数据来源。
- `TD-HDR-003` / `TD-EXP-001` / `TD-EXP-002`: DeerFlow header 有 thread
  `导出`，Coze 当前可见的是安全审计导出和记忆导出，缺少任务/thread 导出。
- `TD-TOKEN-002`: DeerFlow 有 assistant turn per-token 摘要，Coze 当前主要是
  header aggregate，需要补 per-message/per-turn 展示或明确折叠入口。
- `TD-COMP-006`: DeerFlow composer 有文件上传入口；Coze 目标页本轮只看到
  `@` 和链接图标，需要单独验证附件上传是否存在和是否等价。
- `TD-DOC-*`: 文档生成、登记、预览、下载、安全 fallback 已补 case，但本轮
  尚未跑标准文档生成提示词，需要下一步专项验证。

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
- `TD-MD-001` 和 `TD-MD-002` 已通过最近修复：Coze 任务详情能把
  `sequenceDiagram` 和 `flowchart` Mermaid fenced block 渲染为 SVG。
- `TD-MSG-005` inline 思考块需要保留并对齐 DeerFlow。
- `TD-EXP-001` / `TD-EXP-002` 是当前可见 P0 缺口，需要后续补齐。
- `TD-TOKEN-002` 每轮/每消息 token 显示是 DeerFlow 可见能力，当前 Coze
  需要继续对齐。
- `TD-DOC-*` 文档生成和预览已经纳入 P0 验收矩阵，后续必须按文档生成
  标准提示词做专项对比。
- Runtime Doctor 是 Coze 运维增强，不作为 DeerFlow 视觉主线阻塞项；若影响
  页面信息密度，按 P1 或折叠展示处理。

## 执行纪律

- 每个后续开发任务必须引用本文件中的 case id。
- 如果新发现 DeerFlow 可见能力，本文件先补 case，再改代码。
- 如果新发现 Coze 增强但 DeerFlow 不可见，写入 P1/P2，不进入当前 P0。
- 浏览器验证先对比 DeerFlow，再对比 Coze，同一 case 的截图和网络样本成对
  落盘。
