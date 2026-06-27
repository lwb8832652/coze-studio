# Agent Memory Management API M7 Plan

**Goal:** Add the backend foundation for DeerFlow-style memory management while
keeping Coze as the system of record and preserving the existing task naming.

**Scope:** This slice adds Workbench task-thread memory APIs for list/search,
full-snapshot edit, soft-delete, and scoped clear. It does not add frontend UI,
import/export, restore, audit history, or browser E2E.

## Design

- Keep using `agent_thread_memories` and `entity.Memory`; do not introduce a
  parallel memory store.
- Add `deleted_at` to `agent_thread_memories`. Runtime recall and default
  management list paths exclude deleted rows.
- Expose Workbench endpoints under
  `/api/workbench/task_threads/:thread_id/memories`.
- Return memory row content plus bounded metadata only. Do not expose prompts,
  model output, tool arguments/results, checkpoint bytes, object URIs, raw
  provider payloads, credentials, URLs, filenames, or hidden run config.

## Tasks

1. Add failing tests for repository search/soft-delete, domain normalization,
   application DTO mapping, and Workbench route registration.
2. Implement repository list/search/update/delete/clear with soft-delete.
3. Add domain and application service methods.
4. Add Workbench API model structs, handlers, and routes.
5. Add Atlas migration and latest schema update.
6. Update AGENTS and the master roadmap.

## Verification

```bash
cd backend
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./api/router/coze -run 'TestThreadRepositoryManagesMemoriesWithSearchAndSoftDelete|TestManageMemories|TestApplicationMemoryMethodsMapDomainMemories|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1
```

Broader affected-package and Atlas validation should be run before closing this
slice.
