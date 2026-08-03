# Home And Skill Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the authenticated site entrance open a new task and make every skill-management action visible and correctly named.

**Architecture:** Keep workspace selection in the shared space initializer, but let the Coze Studio adapter opt out of restoring the last submenu and choose `chats/new` as its landing route. Keep skill-page changes scoped to the existing management component and stylesheet.

**Tech Stack:** React, TypeScript, React Router, Less, Vitest

---

### Task 1: Lock The Expected Behavior

**Files:**
- Create: `frontend/packages/foundation/space-ui-base/src/hooks/__tests__/use-init-space.test.ts`
- Create: `frontend/packages/foundation/space-ui-base/src/hooks/space-landing.ts`
- Modify: `frontend/apps/coze-studio/src/pages/skill/__tests__/management-page.test.tsx`

- [x] Add a test proving the product landing configuration keeps the selected workspace but uses `chats/new` instead of the stored submenu.
- [x] Change the skill-page assertions to require `自动创建`, `导入技能`, and no `刷新` action.
- [x] Run both focused suites and confirm they fail against the current implementation.

### Task 2: Implement The Landing Contract

**Files:**
- Create: `frontend/packages/foundation/space-ui-base/src/hooks/space-landing.ts`
- Modify: `frontend/packages/foundation/space-ui-base/src/hooks/use-init-space.ts`
- Modify: `frontend/packages/foundation/space-ui-adapter/src/hooks/use-init-space.ts`

- [x] Add optional `fallbackSpaceMenu` and `restoreLastSubMenu` inputs to the shared initializer while preserving its existing defaults.
- [x] Configure the Coze Studio adapter with `fallbackSpaceMenu: 'chats/new'` and `restoreLastSubMenu: false`.
- [x] Run the focused initializer test and confirm it passes.

### Task 3: Correct Skill Actions

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/skill/management-page.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill/skill-management.less`

- [x] Rename `使用 AI 创建` to `自动创建` without changing its workbench intent or navigation target.
- [x] Remove the manual refresh action from the filter toolbar.
- [x] Give secondary action buttons an explicit design-system color, border, and stable text-layout style within the existing page theme.
- [x] Run the skill-management tests and confirm they pass.

### Task 4: Verify The Delivered Experience

**Files:**
- Verify only; no additional product files expected.

- [x] Run focused Vitest suites and the Coze Studio production build.
- [x] Rebuild and restart the single local Web service at `http://localhost:8888`.
- [x] Verify `/` lands on `/space/:space_id/chats/new`.
- [x] Verify the skill page visibly shows `导入技能`, `自动创建`, and `手动创建`, with no `刷新` action.
- [x] Leave changes uncommitted until the repository's integration audit and user confirmation flow is requested.
