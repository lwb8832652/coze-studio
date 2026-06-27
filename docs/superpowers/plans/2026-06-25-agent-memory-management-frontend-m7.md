# Agent Memory Management Frontend M7 Plan

**Goal:** Add the canonical task-detail memory management UI while preserving
the product vocabulary of `任务` and keeping backend memory APIs as the source
of truth.

**Scope:** This slice adds the `任务记忆` panel on task-thread detail pages for
safe list/search, scope filters, full-snapshot edit, soft-delete, and scoped
clear. It does not add import/export, restore, audit history, permission
controls, generated api-schema wiring, or browser E2E.

## Design

- Mount only on task-thread detail pages; legacy task details remain unchanged.
- Use Coze Design/Semi controls through `@coze-arch/coze-design`.
- Keep the list view metadata-safe: show content, scope, source label,
  confidence, and updated time only.
- Do not render raw metadata JSON, prompts, model input/output, tool
  arguments/results, checkpoint bytes, object URIs, raw provider payloads,
  credentials, URLs, filenames, or hidden run config in the list.
- Use the Workbench memory API as the only mutation path. Edits send a
  complete current memory snapshot; deletes remain soft-delete; clear is
  scoped to the selected range.

## Tasks

1. Add failing frontend service and component tests for memory list/search,
   edit, delete, and clear.
2. Add memory API clients while preserving the existing `tasks/service.ts`
   import surface.
3. Implement `TaskMemorySection` with a small hook and utility split to satisfy
   frontend lint limits.
4. Mount the section in `TaskDetailPage` for task-thread details.
5. Update AGENTS and the master roadmap.

## Verification

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__
npx eslint src/pages/tasks/task-memory-section.tsx src/pages/tasks/task-memory-section-hooks.ts src/pages/tasks/task-memory-section-utils.ts src/pages/tasks/task-memory-service.ts src/pages/tasks/detail.tsx src/pages/tasks/service.ts src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx --cache --quiet
```
