# Agent Memory Import Export Frontend M7 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Add production-shaped task-detail `任务记忆` import/export controls
on top of the M7.9 Workbench memory import/export API.

**Architecture:** The task detail memory panel remains the canonical UI. Export
calls the generated `ExportTaskThreadMemories` client, downloads the bounded
`coze.task_thread_memories.export.v1` JSON payload, and shows only aggregate
success/error text. Import uses a SideSheet with a JSON TextArea, parses only
the approved schema, strips unknown fields before calling the generated
`ImportTaskThreadMemories` client, then refreshes the memory list.

**Tech Stack:** React, TypeScript, Coze Design/Semi components, generated
`workbenchTask` clients, Vitest, ESLint.

---

### Task 1: Failing UI Coverage

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-memory-section.test.tsx`

- [x] Add a failing export test that expects a `导出记忆` action, generated
  client call with `limit=100`, JSON `Blob` download, safe file name, and a
  bounded success status.
- [x] Add a failing import test that opens a `导入任务记忆` SideSheet, submits a
  schema payload, forwards only allowed memory fields, drops unknown hidden
  fields, refreshes the list, and shows imported/skipped counts.

### Task 2: Import/Export State Boundary

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/tasks/task-memory-import-export-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section-utils.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/service.ts`

- [x] Re-export generated import/export response and item types from the task
  service boundary.
- [x] Add `MEMORY_EXPORT_SCHEMA`, `MEMORY_EXPORT_LIMIT`, and
  `parseMemoryImportPayload`.
- [x] Validate import JSON shape, schema, non-empty memories, batch limit, and
  non-empty memory content.
- [x] Build import requests from an explicit allow-list only:
  `content`, `metadata`, `run_id`, `scope`, `score`, `confidence`,
  `source_type`, `source_id`, `correction_of_memory_id`, `corrected_at`, and
  `expires_at`.
- [x] Keep export/download and import/refresh state in a focused
  `useTaskMemoryImportExport` hook.

### Task 3: Semi/Coze Design UI

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/tasks/task-memory-import-sheet.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-memory-section.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-prototype.less`

- [x] Add `导出记忆` and `导入记忆` toolbar buttons with Coze Design icons.
- [x] Add a `导入任务记忆` SideSheet using `TextArea` and Button loading states.
- [x] Add a bounded `role=status` notice for import/export success.
- [x] Split the toolbar and import sheet so component and hook files stay under
  repo lint limits.

### Task 4: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M7.10 frontend import/export controls and remaining M7 work.
- [x] Run focused task memory panel tests.
- [x] Run focused task service and api-schema tests.
- [x] Run focused frontend ESLint.
- [x] Run `git diff --check`.
