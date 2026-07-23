# System Model Navigation Grouping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 将模型定价和模型监控移动到系统管理的“模型管理”分组，同时保持路由和业务功能不变。

**Architecture:** 在 `content.ts` 中声明稳定的导航分组和顺序，页面组件只负责按配置渲染。通过纯数据单测保证分组完整、顺序正确且没有重复入口。

**Tech Stack:** React 18、TypeScript、Vitest、现有 Coze/Semi 样式。

---

### Task 1: 固化并渲染系统管理导航分组

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/system/content.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/index.tsx`
- Test: `frontend/apps/coze-studio/src/pages/system/__tests__/system-navigation.test.ts`

- [x] **Step 1: 编写失败测试**

测试 `SYSTEM_NAV_GROUPS` 中模型分组顺序为 `models`、`billing-pricing`、`billing-monitoring`，订阅积分分组不包含模型入口，并验证所有 `SYSTEM_SECTIONS` 只出现一次。

- [x] **Step 2: 运行测试并确认失败**

Run: `npm run test -- src/pages/system/__tests__/system-navigation.test.ts`

Expected: FAIL，因为 `SYSTEM_NAV_GROUPS` 尚未导出。

- [x] **Step 3: 实现最小分组配置与渲染**

新增显式分组常量；有标题分组使用现有 `.coze-prototype-system-nav-group` 和 `.is-child`，无标题入口保持一级菜单样式。保留沙箱管理员过滤和原路径跳转。

- [x] **Step 4: 验证自动化和页面**

Run: `npm run test -- src/pages/system/__tests__/system-navigation.test.ts`

Run: `npx tsc --noEmit --project tsconfig.json`

Expected: 全部通过。随后使用 Codex 内置浏览器验收 `/system/models`、`/system/billing-pricing`、`/system/billing-monitoring`。

本任务未获得 Git 操作授权，不执行提交、合并或推送。

## 验收结果

- Vitest：2 个导航契约用例通过。
- TypeScript：`tsc --noEmit` 通过。
- 浏览器：模型管理分组为“模型配置、模型定价、模型监控”；订阅与积分分组为“积分基础配置、订阅套餐、积分包、用户积分、积分记录、订单管理”。
- 路由：`/system/billing-pricing` 与 `/system/billing-monitoring` 均正常加载，无页面请求错误。
