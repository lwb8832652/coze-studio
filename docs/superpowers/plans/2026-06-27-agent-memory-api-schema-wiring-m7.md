# Agent Memory API Schema Wiring M7 Plan

**Goal:** Move task-thread memory management frontend calls onto the generated
Workbench task API schema so the UI contract matches the IDL instead of a
hand-written fetch layer.

**Scope:** This slice covers `idl/workbench/task.thrift`,
`@coze-studio/api-schema` `workbenchTask` clients, the task page service
exports, focused tests, and project guidance. It does not add memory
import/export, permission controls, browser E2E, or new runtime memory
behavior.

## Design

- Treat `idl/workbench/task.thrift` as the durable contract for task-thread
  memory list/search, full-snapshot update, soft-delete, scoped clear,
  restore, and audit listing.
- Mirror the same contracts in
  `frontend/packages/arch/api-schema/src/idl/workbench/task.ts` so generated
  client metadata covers URL, method, path fields, query fields, and body
  fields.
- Re-export memory clients and types from
  `frontend/apps/coze-studio/src/pages/tasks/service.ts` through
  `workbenchTask`. Do not add new hand-written fetch clients for these memory
  endpoints unless the generated client has a documented blocker.
- Keep audit/listing schemas metadata-safe. Contracts must not expose prompt
  text, model input/output, tool arguments/results, checkpoint bytes, object
  URIs, raw provider payloads, credentials, URLs, filenames, hidden run config,
  or restored memory content in audit payloads.

## Tasks

1. Add failing api-schema tests for task memory generated client exports and
   metadata mappings.
2. Add failing task service tests that require memory exports to be identical
   to `workbenchTask` generated clients.
3. Add memory request/response structs and service methods to
   `idl/workbench/task.thrift`.
4. Add matching TypeScript interfaces and `createAPI` clients to
   `frontend/packages/arch/api-schema/src/idl/workbench/task.ts`.
5. Switch task page service exports to generated memory clients and types.
6. Update the task service tests from hand-written fetch behavior to generated
   metadata contract checks.
7. Update AGENTS and the master roadmap.

## Verification

```bash
cd frontend/packages/arch/api-schema
npm run test -- __tests__/workbench-task-memory.test.ts
npx eslint __tests__/workbench-task-memory.test.ts src/idl/workbench/task.ts --cache --quiet

cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
npx eslint src/pages/tasks/service.ts src/pages/tasks/task-memory-section-hooks.ts src/pages/tasks/task-memory-section.tsx src/pages/tasks/task-memory-section-utils.ts src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx --cache --quiet
```
