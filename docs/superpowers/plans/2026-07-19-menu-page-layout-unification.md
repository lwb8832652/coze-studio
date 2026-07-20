# C 端菜单页面统一布局 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为七个 C 端一级菜单页面建立一致的顶部状态栏、页面宽度、标题区、筛选区和内容起始线。

**Architecture:** 使用应用层 `newx-menu-page` 语义类和集中式 NewX 主题规则作为页面布局合同。页面组件仅增加稳定类名和统一顶部状态栏，不移动业务状态或 API；跨包的开发列表布局通过基础 `Layout` 组件暴露语义类。

**Tech Stack:** React 18、TypeScript、Less、Coze Design、Vitest、Codex in-app browser。

---

### Task 1: 建立布局防回归合同

**Files:**
- Create: `frontend/apps/coze-studio/src/styles/__tests__/newx-menu-page-layout.test.ts`

- [ ] **Step 1: 写入失败的布局合同测试**

```ts
it.each(pageSources)('%s 使用统一菜单页语义类', (_, source, className) => {
  expect(source).toContain(className);
});
```

- [ ] **Step 2: 运行测试并确认 RED**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/styles/__tests__/newx-menu-page-layout.test.ts
```

Expected: FAIL，缺少 `newx-menu-page`、`newx-workspace-topbar` 和共享布局类。

### Task 2: 统一页面结构语义

**Files:**
- Modify: `src/components/workspace-page-top-bar.tsx`
- Modify: `src/pages/library.tsx`
- Modify: `src/pages/develop.tsx`
- Modify: `src/pages/app-dev/index.tsx`
- Modify: `src/pages/app-dev/components/project-list.tsx`
- Modify: `src/pages/skill/management-page.tsx`
- Modify: `src/pages/task-center/index.tsx`
- Modify: `src/pages/workspace/index.tsx`
- Modify: `src/pages/tasks/index.tsx`
- Modify: `../../packages/studio/workspace/entry-base/src/components/layout/list.tsx`
- Modify: `../../packages/studio/workspace/entry-base/src/pages/library/components/library-header.tsx`

- [ ] **Step 1: 为顶部状态栏增加 `newx-workspace-topbar`**
- [ ] **Step 2: 为七个菜单入口增加 `newx-menu-page` 和页面修饰类**
- [ ] **Step 3: 为跨包列表布局增加稳定的 Header、Toolbar、Content 语义类**
- [ ] **Step 4: 为资源库和网页应用列表补充简短页面说明**

### Task 3: 实现统一 NewX 页面骨架

**Files:**
- Modify: `frontend/apps/coze-studio/src/styles/newx-components.less`

- [ ] **Step 1: 定义 `--newx-page-*` 尺寸令牌**
- [ ] **Step 2: 统一顶部状态栏、页面画布和内容最大宽度**
- [ ] **Step 3: 统一标题、说明、操作区、筛选区和内容面板**
- [ ] **Step 4: 为 900px 以下视口补充换行和 16px 内边距**

### Task 4: 验证

**Files:**
- Test: `frontend/apps/coze-studio/src/styles/__tests__/newx-menu-page-layout.test.ts`

- [ ] **Step 1: 运行布局合同测试并确认 GREEN**

```bash
cd frontend/apps/coze-studio
npm run test -- src/styles/__tests__/newx-menu-page-layout.test.ts
```

- [ ] **Step 2: 确认开发服务 HMR 编译成功**
- [ ] **Step 3: 使用内置浏览器逐页截图并测量标题坐标、字号和横向溢出**
- [ ] **Step 4: 将改造前后页面制作成同视口对比图并进行视觉复核**

