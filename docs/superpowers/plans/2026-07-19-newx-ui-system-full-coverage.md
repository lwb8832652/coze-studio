# NewX 全功能 UI 统一 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变现有 API、权限和业务语义的前提下，让 Coze Studio 当前全部可访问功能页面统一到已确认的 NewX 方案 1，并消除已知死链和视觉回归。

**Architecture:** 先建立应用级语义令牌和共享页面骨架，再按页面族迁移局部样式；Coze Design/Semi 组件继续负责交互和可访问性，业务页面只消费令牌和共享模式。路由完整性与全量页面矩阵作为前置门禁，IDE 采用“统一外壳、保留专业工作区”的策略，避免全局 CSS 粗暴覆盖编辑器、画布和第三方组件。

**Tech Stack:** React、TypeScript、React Router、Less、Tailwind、`@coze-arch/coze-design`、Semi Design、Vitest、Codex in-app browser。

---

## 执行边界

- 本计划分成 9 个可独立验收的批次，必须按顺序执行；共享令牌和路由由前两个批次锁定，后续页面不得另建局部色板。
- 不改变后端 API、请求字段、权限判断、空间角色、任务 Runtime、MCP 凭据和 Sandbox 安全策略。
- 不新增生态市场、分组、抖音 Bot、社交场景、Widget、团队旧页或独立模型详情等未启用产品能力。
- 不使用手写 SVG、Emoji 或文本符号代替图标；统一使用仓库已有品牌资源和 `@coze-arch/coze-design/icons`。
- 每批实现后跑对应 targeted tests；全部批次完成后才进行全路由浏览器验收和提交评审。

### Task 1: 建立路由完整性门禁

**Files:**

- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/plugin/tool/plugin-mock-set/page.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/plugin/tool/plugin-mock-set/detail/page.tsx`
- Modify: `frontend/packages/studio/workspace/entry-base/src/pages/develop/components/bot-card/menu-actions.tsx`
- Modify: `frontend/packages/studio/workspace/project-publish/src/publish-button/index.tsx`
- Modify: `frontend/packages/studio/workspace/project-publish/src/publish-main/publish-record.tsx`
- Modify: `frontend/packages/agent-ide/layout/src/components/header/more-menu-button/index.tsx`
- Create: `frontend/apps/coze-studio/src/routes/__tests__/route-integrity.test.tsx`

- [ ] **Step 1: 写主路由匹配失败测试**

```tsx
import { describe, expect, it } from 'vitest';
import { matchRoutes } from 'react-router-dom';

import { router } from '../index';

const reachablePaths = [
  '/sign',
  '/oauth/confirm',
  '/profile',
  '/system/overview',
  '/space/1/chats/new',
  '/space/1/develop',
  '/space/1/bot/2',
  '/space/1/bot/2/publish',
  '/space/1/project-ide/2/session',
  '/space/1/project-ide/2/publish',
  '/space/1/library',
  '/space/1/app-dev',
  '/space/1/app-dev/2',
  '/space/1/skill',
  '/space/1/skill/2',
  '/space/1/tools',
  '/space/1/workspace',
  '/space/1/task-center',
  '/space/1/tasks/2',
  '/space/1/chats',
  '/space/1/knowledge/2',
  '/space/1/knowledge/2/upload',
  '/space/1/database/2',
  '/space/1/plugin/2',
  '/space/1/plugin/2/tool/3',
  '/space/1/plugin/2/tool/3/plugin-mock-set',
  '/space/1/plugin/2/tool/3/plugin-mock-set/4',
  '/work_flow',
  '/search/demo',
  '/explore/plugin',
  '/explore/template',
];

describe('production route integrity', () => {
  it.each(reachablePaths)('matches %s', path => {
    expect(matchRoutes(router.routes, path)).not.toBeNull();
  });
});
```

- [ ] **Step 2: 运行测试并确认 MockSet 两条路径失败**

Run: `cd frontend/apps/coze-studio && npm run test -- src/routes/__tests__/route-integrity.test.tsx`

Expected: 只有 MockSet 列表和详情路径匹配失败，其余当前注册路径通过。

- [ ] **Step 3: 注册现有 MockSet 页面**

在 `async-components.tsx` 暴露现有列表和详情组件；在 `plugin/:plugin_id/tool/:tool_id` 子路由下注册 `plugin-mock-set` 与 `plugin-mock-set/:mockset_id`。保持 `space_id`、`plugin_id`、`tool_id`、`mockset_id` 参数名与页面现有 `useParams` 一致。

- [ ] **Step 4: 统一旧发布与分析链接**

将 Bot 分析入口改为 `/space/${spaceID}/bot/${id}/publish?tab=analysis`；将 Agent 发布记录链接改为 `/space/${spaceId}/bot/${botId}/publish`；将 Project 发布记录链接改为 `/space/${spaceId}/project-ide/${projectId}/publish`。查询参数仅保留当前发布页真实读取的参数，不创建第二套发布页面。

- [ ] **Step 5: 保持未启用能力不可见**

检查当前功能开关下不渲染 `douyin-bot`、`social-scene`、`widget`、`model/:id`、`team` 和 `cloud-tool` 入口。共享跳转工具可以保留兼容字符串，但当前应用不得生成可点击入口。

- [ ] **Step 6: 重跑路由测试**

Run: `cd frontend/apps/coze-studio && npm run test -- src/routes/__tests__/route-integrity.test.tsx`

Expected: 全部 `reachablePaths` 通过，无未处理的可见死链。

### Task 2: 建立全局语义令牌与可访问性基线

**Files:**

- Create: `frontend/apps/coze-studio/src/styles/newx-tokens.less`
- Modify: `frontend/apps/coze-studio/src/global.less`
- Modify: `frontend/apps/coze-studio/src/components/workspace-prototype.less`
- Modify: `frontend/apps/coze-studio/src/index.less`

- [ ] **Step 1: 新增方案 1 的语义令牌**

```less
:root {
  --newx-color-bg-app: #f6f7f4;
  --newx-color-bg-canvas: #ffffff;
  --newx-color-bg-subtle: #f3f5f1;
  --newx-color-text-primary: #171a18;
  --newx-color-text-secondary: #606760;
  --newx-color-text-tertiary: #8a918a;
  --newx-color-border: #e2e6df;
  --newx-color-border-strong: #cfd6cd;
  --newx-color-accent: #168a52;
  --newx-color-accent-hover: #117343;
  --newx-color-accent-soft: #e9f6ee;
  --newx-color-danger: #c83e3e;
  --newx-color-warning: #b36a16;
  --newx-color-focus: #168a52;
  --newx-radius-control: 8px;
  --newx-radius-panel: 12px;
  --newx-radius-dialog: 16px;
  --newx-control-height: 40px;
  --newx-shadow-float: 0 14px 38px rgb(23 26 24 / 12%);
  --newx-shadow-panel: 0 4px 18px rgb(23 26 24 / 6%);
}
```

- [ ] **Step 2: 引入令牌并移除全局视觉冲突**

在 `global.less` 顶部引入令牌；将应用背景、文字和滚动条改为语义变量。删除对 `:focus-visible` 的全局 `outline: none` 抑制，只保留鼠标点击时不需要的浏览器装饰。

- [ ] **Step 3: 建立统一焦点环**

```less
:where(button, a, input, textarea, select, [tabindex]):focus-visible {
  outline: 2px solid var(--newx-color-focus);
  outline-offset: 2px;
}
```

- [ ] **Step 4: 替换共享原型样式中的紫色和重复硬编码**

将 `#5147ff`、紫色透明背景、旧渐变头像、重复的灰色边框和卡片阴影替换为语义令牌；保留业务状态色，不把警告、失败和运行态全部染成绿色。

- [ ] **Step 5: 样式静态校验**

Run: `cd frontend/apps/coze-studio && npx stylelint 'src/{global.less,index.less,styles/*.less,components/workspace-prototype.less}'`

Expected: 无 stylelint 错误，应用级文件不再出现新的紫色主操作硬编码。

### Task 3: 统一全局壳层、导航、图标与头像

**Files:**

- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-prototype.less`
- Modify: `frontend/packages/foundation/global-adapter/src/components/account-dropdown/index.tsx`
- Modify: `frontend/packages/foundation/global-adapter/src/components/account-dropdown/account-settings/index.tsx`
- Test: `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`

- [ ] **Step 1: 扩展现有侧栏测试**

断言 8 个菜单顺序、任务中心使用非空 `prefixIcon`、工作空间在任务中心和全部任务之前、系统管理只对管理员显示、团队空间创建入口仍可用。

- [ ] **Step 2: 统一品牌、导航与任务状态图标**

使用现有 NewX 三角品牌资源；所有导航、搜索、模型、附件、状态、任务中心和更多操作使用 Coze Design 线性图标，图标框统一 `16px`、描边视觉重量一致。

- [ ] **Step 3: 统一顶部和底部头像**

两个位置都从同一个 `userInfo` 头像/首字母计算函数取值；保留右上身份交互，账号设置、退出登录和系统管理仍只放在左下账号菜单。

- [ ] **Step 4: 统一侧栏尺寸与折叠态**

菜单控件高度使用 `40px`，列表行使用统一节奏；折叠态保留 Tooltip、选中态和键盘可达性，不用文本符号补图标。

- [ ] **Step 5: 运行侧栏测试**

Run: `cd frontend/apps/coze-studio && npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`

Expected: 菜单、权限、创建空间和头像行为全部通过。

### Task 4: 统一登录、个人中心与账号设置

**Files:**

- Modify: `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/index.tsx`
- Modify: `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/index.less`
- Modify: `frontend/packages/foundation/account-ui-base/src/hooks/use-account-settings/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/profile/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/profile/index.less`
- Modify: `frontend/apps/coze-studio/src/pages/tools/mcp-settings-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/mcp-capability-sheet.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/mcp-server-form-sheet.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/mcp-test-modal.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/feishu-im-settings-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/feishu-im-settings-panel.module.less`

- [ ] **Step 1: 对齐认证页全部实际状态**

统一登录、注册、验证码、找回密码、提交中和错误状态的品牌、标题、表单宽度、控件高度、错误反馈和返回入口；只改适配器当前真实渲染的状态。

- [ ] **Step 2: 统一个人中心**

将资料头、头像、用户名、昵称、简介、邮箱、保存状态和错误反馈映射到语义令牌；保持现有资料 API 和只读字段边界。

- [ ] **Step 3: 统一账号设置弹窗**

账号、API 授权、MCP 配置和 IM 机器人使用同一左侧导航、内容边距、标题层级、滚动策略和关闭按钮；Tab 切换不得丢失未提交表单状态。

- [ ] **Step 4: 覆盖 MCP 与飞书全部子面板**

统一列表、空态、服务表单、能力与日志 SideSheet、试运行 Modal、导出/删除确认、飞书机器人表单、连接状态、回复模式和群聊策略；敏感字段继续脱敏。

- [ ] **Step 5: targeted tests**

Run: `cd frontend/apps/coze-studio && npm run test -- src/pages/tools src/pages/profile`

Expected: 账号资料、MCP 和飞书已有测试全部通过；缺失的关键弹层交互补测试后通过。

### Task 5: 统一工作台、任务列表、任务详情与任务中心

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.less`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/extensions-popover.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-inspector.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-artifacts-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-runtime-doctor-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-guardrail-audit-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-mcp-runtime-audit-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/task-center/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/task-center/index.less`
- Modify: `frontend/apps/coze-studio/src/pages/task-center/task-form-modal.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/task-center/execution-records-modal.tsx`
- Test: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] **Step 1: 统一首页与输入器**

保持模板、技能预填、模型选择、附件、Runtime 设置、`@` 资源和发送逻辑；只调整层级、留白、控件尺寸、图标、聚焦态和错误提示。

- [ ] **Step 2: 统一任务列表与侧栏历史**

搜索、筛选、收藏、批量操作、状态图标、长标题、加载更多、个人/团队切换、空态和失败态采用同一列表模式。

- [ ] **Step 3: 统一任务详情主区域**

消息、推理、计划事件、人机交互、子 Agent、待办、产物、Token 和输入器使用明确层级；读取产物失败必须显示可操作的重试，不得出现裸错误文本。

- [ ] **Step 4: 统一任务详情 SideSheet**

Runtime Doctor、Guardrail、MCP 审计和记忆区使用统一 Section 模式；只展示现有安全合同允许的 bounded metadata。

- [ ] **Step 5: 统一任务中心**

任务中心图标、任务表格/卡片、创建编辑 Modal、执行记录、启停、运行、删除和错误回滚全部接入共享控件。

- [ ] **Step 6: 运行任务相关测试**

Run: `cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/task-center`

Expected: 任务主流程、产物错误、权限和定时任务交互通过。

### Task 6: 统一资源、技能、插件、MCP 与工作空间页面

**Files:**

- Modify: `frontend/packages/studio/workspace/entry-base/src/pages/library/index.tsx`
- Modify: `frontend/packages/studio/workspace/entry-base/src/pages/library/index.module.less`
- Modify: `frontend/packages/studio/workspace/entry-base/src/pages/library/components/library-header.tsx`
- Modify: `frontend/packages/studio/workspace/entry-base/src/pages/library/components/base-library-item.tsx`
- Modify: `frontend/packages/studio/workspace/entry-base/src/pages/knowledge-preview/index.tsx`
- Modify: `frontend/packages/studio/workspace/entry-base/src/pages/knowledge-upload/index.tsx`
- Modify: `frontend/packages/studio/workspace/entry-base/src/pages/database/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill/management-page.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill/skill-management.less`
- Modify: `frontend/apps/coze-studio/src/pages/skill/skill-version-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill/skill-version-panel-sections.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/index.less`
- Modify: `frontend/apps/coze-studio/src/pages/workspace/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/plugin/layout.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/plugin/page.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/plugin/tool/page.tsx`

- [ ] **Step 1: 统一资源库与资源详情**

覆盖 Agent/项目、插件、工作流、知识库和数据库的真实资源类型；统一 Header、筛选、创建菜单、列表/卡片、操作菜单、上传步骤、详情导航、空态和错误态。

- [ ] **Step 2: 统一技能管理与详情**

覆盖列表、手动创建、AI 创建、导入、复制、删除、详情编辑、版本、权限、资源文件、回滚和开发会话返回；不恢复旧 `mode=skill` 页面。

- [ ] **Step 3: 统一插件工具与 MockSet**

插件、工具、调试、MockSet 列表和详情沿用 Workspace Base 功能，外层面包屑、标题、表单、表格、空态和错误态接入统一令牌。

- [ ] **Step 4: 统一工作空间管理**

基础资料、成员、邀请、角色、移除、转让、删除和所有危险操作 Modal 使用统一模式；继续以后端真实角色决定可编辑性。

- [ ] **Step 5: 运行资源与权限测试**

Run: `cd frontend/apps/coze-studio && npm run test -- src/pages/skill src/pages/workspace src/pages/plugin`

Expected: CRUD、角色权限、版本操作和路由参数测试通过。

### Task 7: 统一 AppDev、Agent、Project 与 Workflow IDE

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/app-dev/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/ide.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/index.less`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/project-list.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/file-tree.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/code-editor.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/chat-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/preview-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/dev-logs-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/runtime-toolbar.tsx`
- Modify: `frontend/packages/agent-ide/layout/src/components/header/index.tsx`
- Modify: `frontend/packages/project-ide/main/src/components/top-bar/operators/index.tsx`
- Modify: `frontend/packages/workflow/playground/src/components/workflow-header/components/history-button/components/history-drawer/index.tsx`

- [ ] **Step 1: 统一 AppDev 项目页**

列表、创建、导入、复制、重命名、描述、归档、空态和失败态使用共享页面模式。

- [ ] **Step 2: 统一 AppDev IDE 外壳**

顶部栏、文件树、Chat、预览、日志、Runtime Toolbar、项目设置、快照和版本历史接入令牌；Monaco 编辑区保留专业深色主题和键盘行为。

- [ ] **Step 3: 统一 Agent/Project IDE 外壳**

只调整 Header、面板边界、控件、弹层、状态反馈和发布页；不改变资源树、画布、Dock、拖拽和编辑器内部交互。

- [ ] **Step 4: 统一 Workflow 工作区**

Header、节点面板、属性表单、问题面板、历史抽屉、调试日志和发布反馈使用语义令牌；不对画布节点坐标或连线做全局 CSS 覆盖。

- [ ] **Step 5: 运行 AppDev 与 IDE targeted tests**

Run: `cd frontend/apps/coze-studio && npm run test -- src/pages/app-dev`

Run: `rush test --to @coze-project-ide/main --to @coze-workflow/playground-adapter --to @coze-agent-ide/layout-adapter`

Expected: 文件操作、保存、预览、发布导航和 IDE 关键交互通过。

### Task 8: 统一系统管理、搜索与现有探索页

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/system/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/overview-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/user-management-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/workspace-management-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/model-config-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-management-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-management-section.module.less`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-provider-form.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-provider-detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-audit-drawer.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx`
- Modify: `frontend/packages/community/explore/src/components/search/header/index.tsx`
- Modify: `frontend/packages/community/explore/src/components/sub-menu/index.tsx`

- [ ] **Step 1: 统一系统管理壳和 6 个分区**

二级导航、标题、统计卡、表格、表单、Provider 管理、审计抽屉和保存反馈使用同一管理后台模式；所有管理员入口和 API 继续服务端强校验。

- [ ] **Step 2: 统一错误与权限状态**

普通用户访问 `/system/*` 显示设计一致的 `403`；会话过期、Provider 失败、模型连接失败和配置保存失败不能被空白页或成功 Toast 掩盖。

- [ ] **Step 3: 统一搜索与现有探索页**

只统一当前 `/search/:word`、`/explore/plugin`、`/explore/template` 的壳、筛选、结果和状态；不新增生态市场业务。

- [ ] **Step 4: 运行管理后台测试**

Run: `cd frontend/apps/coze-studio && npm run test -- src/pages/system`

Expected: 6 个 section、管理员权限、Sandbox 安全状态和配置错误路径通过。

### Task 9: 全量路由与视觉回归验收

**Files:**

- Create: `docs/superpowers/verification/2026-07-19-newx-ui-full-route-matrix.md`
- Update: `docs/superpowers/specs/2026-07-19-newx-ui-system-design.md`

- [ ] **Step 1: 运行前端静态与单元校验**

Run: `cd frontend/apps/coze-studio && npx tsc --noEmit --project tsconfig.json`

Run: `cd frontend/apps/coze-studio && npm run test -- src/routes src/components/workspace-sub-menu src/pages`

Expected: TypeScript 无错误，相关测试全部通过。

- [ ] **Step 2: 用内置浏览器逐路由验收**

按设计规范“完整功能页面覆盖基线”的顺序访问每个直接页面；记录 URL、账号、空间、权限、关键交互、空/错/加载状态和控制台错误。重定向路由记录最终 URL，IDE 通配路由至少覆盖入口、内部资源、调试和发布。

- [ ] **Step 3: 做同视口截图对比**

桌面使用同一视口分别截取选定方案 1 基准和实现页；逐项检查图标、头像、字体、颜色、间距、圆角、边框、焦点、弹层和滚动区域。发现差异先修复，再重新截图比较。

- [ ] **Step 4: 验收响应式和可访问性**

检查常见笔记本宽度、窄屏、键盘 Tab 顺序、焦点可见、Escape 关闭、Modal 焦点回收、长文本和中文/现有语言切换。

- [ ] **Step 5: 记录已知边界**

验证矩阵只允许记录外部服务不可用、测试数据缺失或明确延期能力；不允许把可修复的视觉问题、死链、裸错误、缺图标或权限错误列为“已知限制”后交付。

- [ ] **Step 6: 提交前评审**

向用户提供变更摘要、完整验证证据和残余风险；得到明确确认后再执行提交、合并或推送流程。
