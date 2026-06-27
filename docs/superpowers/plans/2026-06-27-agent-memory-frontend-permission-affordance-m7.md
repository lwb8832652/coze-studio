# Agent Memory Frontend Permission Affordance M7 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add task-detail frontend permission affordances for `任务记忆`
after M7.11 backend authorization, without treating UI state as a security
boundary.

**Architecture:** The task detail page derives a read-only memory panel from
the current user ID and the task-thread creator ID. The `任务记忆` panel keeps
safe read actions available, including list/search, export, deleted-row
viewing, and audit viewing. Write actions are disabled or blocked in handler
state for non-owners: import, edit, delete, restore, and clear. Backend
`MemoryAuthorizer` remains the production authority.

**Tech Stack:** React, TypeScript, Coze Design/Semi components, generated
`workbenchTask` clients, Vitest, ESLint.

---

### Task 1: Read-Only UI Coverage

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-memory-section.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [x] Add task-memory panel coverage that passes `readOnly` and expects
  `导入记忆`, `清空`, `编辑`, `删除`, and `恢复` write controls to be disabled.
- [x] Assert safe read controls such as `导出记忆` and `审计` remain enabled.
- [x] Mock the current user hook in task-detail tests so owner versus
  non-owner checks can be evaluated without pulling real account state.
- [x] Add a task-detail regression test that renders a non-owner thread viewer
  and expects the memory panel to switch to read-only controls.

### Task 2: Permission State And Safe UI

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-import-export-hooks.ts`
- Create: `frontend/apps/coze-studio/src/pages/tasks/task-memory-list-actions.ts`
- Create: `frontend/apps/coze-studio/src/pages/tasks/task-memory-row.tsx`

- [x] Derive `readOnly` only for task-thread detail pages when the current user
  ID is known and differs from `task.creator_id`.
- [x] Show a bounded `只读` tag in the memory panel for read-only viewers.
- [x] Keep read/list/export/audit behavior available in read-only mode.
- [x] Disable write controls and keep handler-level write guards for import,
  edit, delete, restore, and clear.
- [x] Split row rendering and list write actions into focused files so lint
  limits stay satisfied.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M7.12 frontend permission affordances and remaining browser E2E.
- [x] Run focused task memory panel and task detail tests.
- [x] Run focused backend memory authorization tests.
- [x] Run focused frontend ESLint.
- [x] Run `git diff --check`.
