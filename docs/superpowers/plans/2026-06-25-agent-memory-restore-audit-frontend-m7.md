# Agent Memory Restore And Audit Frontend M7 Plan

**Goal:** Add task-detail frontend controls for deleted memory browsing,
restore, and metadata-only audit viewing while preserving the product naming
of `任务记忆`.

**Scope:** This slice updates the existing task-thread memory panel, its
manual Workbench API clients, and frontend tests. It does not add memory
import/export, permission controls, generated api-schema wiring, browser E2E,
or a separate settings page.

## Design

- Keep default memory lists unchanged. Deleted memories are requested only when
  the user explicitly enables the `已删除` toggle, which sends
  `include_deleted=true`.
- Render restore actions only for rows whose `deleted_at` is non-zero. Restore
  sends only `thread_id` and `memory_id`, then reloads the current list.
- Add a row-level `审计` action that opens a SideSheet and requests audit
  events for that memory with explicit pagination.
- Keep audit history metadata-only. The UI may show event type, scope, source
  type/id, affected count, timestamps, and bounded identity fields; it must not
  render raw metadata JSON, prompts, model input/output, tool arguments/results,
  checkpoint bytes, object URIs, raw provider payloads, credentials, URLs,
  filenames, hidden run config, or audit-derived memory content.

## Tasks

1. Add failing service tests for `include_deleted`, restore, and audit list
   API contracts.
2. Add failing component tests for deleted listing, restore, and audit
   SideSheet behavior.
3. Implement manual API clients and service re-exports.
4. Extend the task memory hook and component with restore/audit state.
5. Add focused styles for deleted rows and audit history.
6. Update AGENTS and the master roadmap.

## Verification

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__
npx eslint src/pages/tasks/task-memory-section.tsx src/pages/tasks/task-memory-section-hooks.ts src/pages/tasks/task-memory-section-utils.ts src/pages/tasks/task-memory-service.ts src/pages/tasks/service.ts src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts --cache --quiet
```
