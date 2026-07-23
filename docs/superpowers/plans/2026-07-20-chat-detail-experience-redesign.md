# Chat Detail Experience Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将任务对话详情页重构为面向 C 端用户的精致对话体验，同时复用首页输入框和顶部通知/头像能力，完整保留现有任务、消息、用量、导出、详情、收藏、分享、产物和错误恢复功能。

**Architecture:** 保留 `TaskDetailPage` 现有数据加载与任务运行编排，只拆分展示层。抽取共享 `ChatComposer` 和 `WorkspaceHeaderActions`，分别由首页与任务详情的薄适配器接入；任务详情采用聚焦阅读列、可折叠执行摘要和右下角用量入口。用量继续复用现有会话级与 run 级数据，只改变呈现位置，不改变统计语义。

**Tech Stack:** React 18、TypeScript、Semi UI / Coze Design、Less、Vitest、React Testing Library、Codex in-app browser。

---

## 设计基线与边界

- 设计规格：`docs/superpowers/specs/2026-07-20-chat-detail-experience-redesign-design.md`
- 当前详情截图：`.superpowers/audits/chat-detail-redesign-2026-07-20/01-current-chat-detail.png`
- 当前首页输入框截图：`.superpowers/audits/chat-detail-redesign-2026-07-20/02-current-home-composer.png`
- 已确认视觉目标：`.superpowers/audits/chat-detail-redesign-2026-07-20/03-approved-token-popover-target.png`
- 首页与详情页必须复用同一套输入框核心组件，页面只提供业务适配参数。
- 首页与详情页必须复用同一套通知、未读状态、头像和账号菜单组件。
- Token 用量从顶部移到输入框右下角，但保留会话总量、输入、输出和逐次回复明细。
- 不改任务 API、Agent Runtime、产物数据合同、路由合同和任务持久化逻辑。
- 不删除导出、详情、收藏、分享、通知、账号菜单、附件、模型选择、技能选择、发送、停止、重试和继续对话能力。

## Task 1: 固化共享头部操作区的行为合同

**Files:**
- Create: `frontend/apps/coze-studio/src/components/__tests__/workspace-header-actions.test.tsx`
- Create: `frontend/apps/coze-studio/src/components/workspace-header-actions.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx`

- [ ] 在新测试中覆盖通知按钮、未读标记、头像、账号菜单、设置、退出登录和系统管理员入口的可见性与回调。
- [ ] 先运行测试并确认因 `WorkspaceHeaderActions` 尚不存在而失败。

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/__tests__/workspace-header-actions.test.tsx
```

- [ ] 新建 `WorkspaceHeaderActions`，统一读取当前用户、通知状态和账号菜单能力；只通过 `className`、`compact` 等展示参数适配页面，不允许页面复制业务逻辑。
- [ ] 组件公开最小接口：

```ts
export interface WorkspaceHeaderActionsProps {
  className?: string;
  compact?: boolean;
}
```

- [ ] 将 `WorkspacePageTopBar`、`WorkbenchTopbar` 和 `TaskDetailHeader` 中各自的通知与头像实现替换为 `WorkspaceHeaderActions`。
- [ ] 确认任务详情页保留导出、详情、收藏、分享操作，并把共享头部操作区放在这些任务操作之后。
- [ ] 重新运行测试并确认通过。
- [ ] 提交本任务。

```bash
git add frontend/apps/coze-studio/src/components/workspace-header-actions.tsx \
  frontend/apps/coze-studio/src/components/__tests__/workspace-header-actions.test.tsx \
  frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx \
  frontend/apps/coze-studio/src/pages/workbench/index.tsx \
  frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx
git commit -m "feat: share workspace header actions"
```

## Task 2: 抽取首页与详情页共用的输入框核心

**Files:**
- Create: `frontend/apps/coze-studio/src/components/chat-composer/index.tsx`
- Create: `frontend/apps/coze-studio/src/components/chat-composer/index.less`
- Create: `frontend/apps/coze-studio/src/components/chat-composer/__tests__/chat-composer.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-follow-up-composer.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.less`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less`

- [ ] 为 `ChatComposer` 编写失败测试，覆盖 `hero`、`docked` 两种布局和下列共享行为：输入、附件入口、技能/工具入口、模型选择、发送、停止、loading、disabled、键盘提交和多行输入。
- [ ] 先运行测试并确认因共享组件不存在而失败。

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/chat-composer/__tests__/chat-composer.test.tsx
```

- [ ] 新建无业务数据请求的 `ChatComposer` 展示与交互核心，建议接口如下：

```ts
export interface ChatComposerProps {
  variant: 'hero' | 'docked';
  value: string;
  placeholder?: string;
  disabled?: boolean;
  submitting?: boolean;
  streaming?: boolean;
  modelLabel: string;
  skillLabel?: string;
  footerStart?: React.ReactNode;
  footerEnd?: React.ReactNode;
  onChange: (value: string) => void;
  onSubmit: () => void;
  onStop?: () => void;
  onAttach?: () => void;
  onSelectModel?: () => void;
  onSelectSkill?: () => void;
}
```

- [ ] 将富文本输入、键盘行为、发送/停止按钮、工具栏布局和无障碍标签集中到共享组件，避免 `WorkbenchComposer` 与 `TaskFollowUpComposer` 再维护两套 DOM。
- [ ] 保留 `WorkbenchComposer` 的模板首页业务状态，使其成为 `hero` 变体的薄适配器。
- [ ] 保留 `TaskFollowUpComposer` 的任务续聊、流式状态和错误恢复，使其成为 `docked` 变体的薄适配器。
- [ ] 将视觉变量统一为相同圆角、边框、焦点环、图标尺寸、按钮高度和禁用态；详情页仅通过变体调整容器高度与停靠方式。
- [ ] 运行共享组件测试并确认通过。
- [ ] 提交本任务。

```bash
git add frontend/apps/coze-studio/src/components/chat-composer \
  frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx \
  frontend/apps/coze-studio/src/pages/tasks/task-follow-up-composer.tsx \
  frontend/apps/coze-studio/src/pages/workbench/index.less \
  frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less
git commit -m "refactor: share chat composer across workbench"
```

## Task 3: 将 Token 用量迁移到输入框右下角

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/tasks/task-usage-popover.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tasks/task-usage-detail-drawer.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-usage-popover.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-usage-detail-drawer.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-follow-up-composer.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-message-token-usage.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less`

- [ ] 为用量入口与明细抽屉编写失败测试，覆盖会话总量、输入、输出、当前回复、run 级明细、空态、加载态、错误态、关闭和键盘焦点恢复。
- [ ] 先运行测试并确认新组件缺失导致失败。

```bash
cd frontend/apps/coze-studio
npm run test -- \
  src/pages/tasks/__tests__/task-usage-popover.test.tsx \
  src/pages/tasks/__tests__/task-usage-detail-drawer.test.tsx
```

- [ ] 新建 `TaskUsagePopover`，在详情输入框 `footerEnd` 渲染紧凑用量入口，浮层向上展开。
- [ ] 新建 `TaskUsageDetailDrawer`，复用 `tokenUsageByRunID` 展示每次 Agent 回复的输入、输出、合计和时间；不得向 UI 暴露 prompt、tool arguments、provider raw body 或其他内部运行数据。
- [ ] 从 `TaskDetailHeader` 移除顶部 Token 按钮，仅移除重复入口，不删除详情页已加载的统计数据。
- [ ] 从每条 Agent 消息下方移除常驻用量文本，将原 `TaskMessageTokenUsage` 的格式化和统计逻辑复用到浮层与抽屉。
- [ ] 在无统计数据时显示明确空态；接口失败时允许重试，不阻断对话阅读和继续发送。
- [ ] 重新运行用量测试并确认通过。
- [ ] 提交本任务。

```bash
git add frontend/apps/coze-studio/src/pages/tasks/task-usage-popover.tsx \
  frontend/apps/coze-studio/src/pages/tasks/task-usage-detail-drawer.tsx \
  frontend/apps/coze-studio/src/pages/tasks/__tests__/task-usage-popover.test.tsx \
  frontend/apps/coze-studio/src/pages/tasks/__tests__/task-usage-detail-drawer.test.tsx \
  frontend/apps/coze-studio/src/pages/tasks/detail.tsx \
  frontend/apps/coze-studio/src/pages/tasks/task-follow-up-composer.tsx \
  frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx \
  frontend/apps/coze-studio/src/pages/tasks/task-message-token-usage.tsx \
  frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less
git commit -m "feat: move task usage into composer"
```

## Task 4: 重构对话内容层级与执行过程

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/tasks/conversation-turn.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tasks/execution-summary.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tasks/__tests__/conversation-turn.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tasks/__tests__/execution-summary.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less`

- [ ] 为用户消息、Agent 消息、流式回复、执行中、执行成功、执行失败、重试、技能目录、Markdown、文件产物和长内容编写失败测试。
- [ ] 为执行摘要编写失败测试，覆盖默认折叠、步骤数、状态、展开、收起和失败步骤定位。
- [ ] 先运行测试并确认新组件缺失导致失败。

```bash
cd frontend/apps/coze-studio
npm run test -- \
  src/pages/tasks/__tests__/conversation-turn.test.tsx \
  src/pages/tasks/__tests__/execution-summary.test.tsx
```

- [ ] 新建 `ConversationTurn`，统一一轮对话的语义结构：用户消息右对齐气泡，Agent 回复使用阅读面而不是整块聊天气泡。
- [ ] 新建 `ExecutionSummary`，默认仅展示执行状态、步骤数和耗时；展开后继续复用现有步骤内容和错误信息。
- [ ] 将 `TaskThreadMessageList` 中分散的消息展示迁移到新组件，保留原消息顺序、状态映射、产物卡片和操作回调。
- [ ] 将内容主列限制在约 `720px` 至 `780px`，大屏居中、小屏占满；避免消息横跨整个视口。
- [ ] 移除当前详情背景中影响阅读的装饰性径向渐变，改为统一的浅色页面底和清晰层级。
- [ ] 保证长 Markdown、表格、代码块、链接、图片和文件卡片不溢出阅读列。
- [ ] 重新运行组件测试并确认通过。
- [ ] 提交本任务。

```bash
git add frontend/apps/coze-studio/src/pages/tasks/conversation-turn.tsx \
  frontend/apps/coze-studio/src/pages/tasks/execution-summary.tsx \
  frontend/apps/coze-studio/src/pages/tasks/__tests__/conversation-turn.test.tsx \
  frontend/apps/coze-studio/src/pages/tasks/__tests__/execution-summary.test.tsx \
  frontend/apps/coze-studio/src/pages/tasks/detail.tsx \
  frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less
git commit -m "feat: redesign task conversation turns"
```

## Task 5: 统一任务头部、弹窗、抽屉和产物分栏

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] 在任务详情集成测试中增加失败断言，覆盖标题、返回、导出、详情、收藏、分享、通知、头像、产物分栏开关和错误重试。
- [ ] 运行测试并确认新布局断言失败。

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
```

- [ ] 将头部整理为左侧任务上下文、右侧任务操作和共享用户操作区，统一按钮高度、图标、间距、hover、focus 和 disabled 状态。
- [ ] 保持详情、导出和分享的原业务组件与回调，只统一弹窗/抽屉的标题区、内容间距、操作区和关闭方式。
- [ ] 产物面板关闭时让对话阅读列居中；开启时保留原分栏、拖拽/宽度限制和错误重试能力。
- [ ] 详情页所有浮层使用一致的圆角、阴影、边框、遮罩和进入/退出动效，并支持 `Escape` 关闭与焦点返回。
- [ ] 重新运行任务详情测试并确认通过。
- [ ] 提交本任务。

```bash
git add frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx \
  frontend/apps/coze-studio/src/pages/tasks/detail.tsx \
  frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less \
  frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx
git commit -m "feat: align task detail chrome and overlays"
```

## Task 6: 补齐响应式、状态与无障碍回归

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less`
- Modify: `frontend/apps/coze-studio/src/components/chat-composer/index.less`
- Modify: `frontend/apps/coze-studio/src/components/chat-composer/__tests__/chat-composer.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`

- [ ] 增加窄屏测试：隐藏非关键文字、保留核心图标操作、对话列占满、输入框固定可用、用量浮层不越界、产物面板改为覆盖式抽屉。
- [ ] 增加键盘与 ARIA 测试：Tab 顺序、按钮名称、展开状态、弹窗标题关联、发送快捷键、`Escape` 关闭和焦点恢复。
- [ ] 增加首页回归测试，确认共享输入框和共享通知/头像组件接入后，模板、预填技能、附件、模型选择和发送流程没有缩减。
- [ ] 运行相关测试并先确认新增断言能够捕获缺失状态。

```bash
cd frontend/apps/coze-studio
npm run test -- \
  src/components/chat-composer/__tests__/chat-composer.test.tsx \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/workbench/__tests__/workbench.test.tsx
```

- [ ] 完成桌面、平板和手机断点样式，处理 reduced-motion、高对比焦点、长用户名、长标题和超长消息。
- [ ] 修复测试暴露的回归，但不新增规格外功能。
- [ ] 重新运行测试并确认通过。
- [ ] 提交本任务。

```bash
git add frontend/apps/coze-studio/src/pages/tasks/newx-task-ui.less \
  frontend/apps/coze-studio/src/components/chat-composer/index.less \
  frontend/apps/coze-studio/src/components/chat-composer/__tests__/chat-composer.test.tsx \
  frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx \
  frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx
git commit -m "test: cover responsive task conversation experience"
```

## Task 7: 完整回归与页面验收

**Files:**
- Modify if needed: `docs/superpowers/specs/2026-07-20-chat-detail-experience-redesign-design.md`
- Create: `.superpowers/audits/chat-detail-redesign-2026-07-20/04-implemented-desktop.png`
- Create: `.superpowers/audits/chat-detail-redesign-2026-07-20/05-implemented-usage-popover.png`
- Create: `.superpowers/audits/chat-detail-redesign-2026-07-20/06-implemented-mobile.png`

- [ ] 运行本次所有 targeted tests。

```bash
cd frontend/apps/coze-studio
npm run test -- \
  src/components/__tests__/workspace-header-actions.test.tsx \
  src/components/chat-composer/__tests__/chat-composer.test.tsx \
  src/pages/workbench/__tests__/workbench.test.tsx \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/tasks/__tests__/conversation-turn.test.tsx \
  src/pages/tasks/__tests__/execution-summary.test.tsx \
  src/pages/tasks/__tests__/task-usage-popover.test.tsx \
  src/pages/tasks/__tests__/task-usage-detail-drawer.test.tsx
```

- [ ] 运行 TypeScript 校验，修复本次改动引入的错误。

```bash
cd frontend/apps/coze-studio
npx tsc --noEmit --project tsconfig.json
```

- [ ] 使用 Codex in-app browser 验收首页：

```text
http://localhost:8080/space/7645565700475453440/chats/new
```

- [ ] 验证首页共享输入框、通知未读状态、通知面板、头像、账号菜单和创建任务流程。
- [ ] 使用 Codex in-app browser 验收任务详情：

```text
http://localhost:8080/space/7645565700475453440/tasks/7664082792375910400
```

- [ ] 验证消息阅读层级、执行摘要展开/收起、流式回复、停止、重试、继续对话、附件、模型、技能、产物、导出、详情、收藏、分享、通知和账号菜单。
- [ ] 验证右下角用量入口、向上浮层、会话总量、当前回复、逐次明细抽屉、关闭后的焦点恢复。
- [ ] 分别在桌面与移动视口截图，并与已确认视觉目标对比。
- [ ] 检查浏览器控制台，确保没有新增 error、未处理 Promise rejection、重复 key 或无障碍警告。
- [ ] 将最终验收结果、URL、账号/空间、关键状态、控制台结果和仍存在的非阻塞差异补充到设计规格。
- [ ] 提交验收记录。

```bash
git add docs/superpowers/specs/2026-07-20-chat-detail-experience-redesign-design.md \
  .superpowers/audits/chat-detail-redesign-2026-07-20
git commit -m "docs: record chat detail visual acceptance"
```

## 完成定义

- [ ] 首页与详情页使用同一 `ChatComposer` 核心组件，仅变体和业务适配不同。
- [ ] 首页与详情页使用同一 `WorkspaceHeaderActions`，通知与账号状态一致。
- [ ] 详情页顶部不再重复展示 Token 按钮，用量入口位于输入框右下角。
- [ ] 会话总用量和逐次回复用量均可访问，统计语义与改造前一致。
- [ ] 用户消息、Agent 回复、执行过程和产物形成清晰、稳定的阅读层级。
- [ ] 所有原有功能均有自动化回归或浏览器验收证据，没有功能缩减。
- [ ] 桌面、平板、移动端及 loading、empty、streaming、error、disabled 状态均通过验收。
- [ ] 页面视觉与已确认目标一致，且浏览器控制台无本次改动引入的错误。
