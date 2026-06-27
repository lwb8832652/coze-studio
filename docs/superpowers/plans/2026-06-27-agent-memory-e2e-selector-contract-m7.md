# Agent Memory E2E Selector Contract M7 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add stable, content-free selector coverage for the task-detail
`任务记忆` panel so future browser E2E automation can target memory workflows.

**Architecture:** The repository does not currently include a runnable
Playwright or Cypress harness for `@coze-studio/app`; the existing
`@coze-data/e2e` package is a selector constant package. This slice follows
the M4 artifact drawer pattern: add stable DOM selectors to the memory panel
and assert them through existing Vitest coverage, without introducing a new
browser framework.

**Tech Stack:** React, TypeScript, Coze Design/Semi components, Vitest,
ESLint.

---

### Task 1: Failing Selector Coverage

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-memory-section.test.tsx`

- [x] Add a failing selector-contract test for `task-memory-panel`,
  `task-memory-row`, safe `data-memory-id`, toolbar buttons, import sheet, and
  read-only tag selectors.
- [x] Extend restore/audit coverage to assert `task-memory-audit-sheet` and
  `task-memory-audit-row`.
- [x] Assert selectors do not carry memory content, metadata JSON, source ID,
  run ID, object URI, prompt text, model text, tool payloads, credentials, or
  raw provider payloads.

### Task 2: Stable Memory Selectors

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-row.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-import-sheet.tsx`

- [x] Add `data-testid="task-memory-panel"` to the panel root.
- [x] Add toolbar selectors for search, refresh, export, import, deleted
  toggle, and clear controls.
- [x] Add row selectors with only safe identity attributes:
  `data-testid="task-memory-row"` and `data-memory-id`.
- [x] Add `task-memory-readonly`, `task-memory-import-sheet`,
  `task-memory-import-input`, `task-memory-import-submit`,
  `task-memory-audit-sheet`, and `task-memory-audit-row` selectors.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M7.13 selector contract and keep true browser runner work in
  production acceptance until a Playwright/Cypress harness exists.
- [x] Run focused task memory panel and task detail tests.
- [x] Run focused frontend ESLint.
- [x] Run `git diff --check`.
