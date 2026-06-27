# Agent Memory Restore And Audit M7 Plan

**Goal:** Add production-grade backend restore and metadata-only audit history
for Workbench task-thread memories while keeping `任务记忆` as a task-detail
management surface.

**Scope:** This slice adds explicit deleted-row listing, restore, audit event
persistence, audit listing, Atlas schema updates, and backend tests. It does
not add frontend restore controls, audit timeline UI, import/export,
permission controls, generated api-schema wiring, or browser E2E.

## Design

- Keep `agent_thread_memories` as the memory system of record. Restore may only
  clear `deleted_at` for a row that matches the requested `thread_id` and
  `memory_id`.
- Keep default memory lists unchanged. Deleted rows are returned only when a
  management caller explicitly sends `include_deleted=true`.
- Add `agent_memory_audit_events` as a content-free audit table. It stores only
  event identity, thread/run/space/memory/actor IDs, event type, scope, source
  type/id, affected count, and timestamp.
- Record audit events only after successful update, delete, clear, or restore.
  No audit row is written for validation failures or not-found mutations.
- Expose audit history through Workbench as safe metadata only. Do not expose
  memory content, metadata JSON, prompts, model input/output, tool
  arguments/results, checkpoint bytes, object URIs, raw provider payloads,
  credentials, URLs, filenames, or hidden run config.

## Tasks

1. Add failing repository, domain service, and router tests for restore and
   audit routes.
2. Implement repository restore and memory audit persistence/listing.
3. Add domain and application service methods plus safe DTO mappers.
4. Add Workbench API models, handlers, and routes.
5. Add Atlas migration and latest schema table.
6. Update AGENTS and the master roadmap.

## Verification

```bash
cd backend
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./api/model/workbench/thread ./api/handler/coze ./api/router/coze -run 'TestThreadRepositoryManagesMemoriesWithSearchAndSoftDelete|TestManageMemoriesNormalizesAndDelegates|TestApplicationMemoryMethodsMapDomainMemories|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./api/router/coze -count=1

cd ../docker/atlas
atlas migrate hash
atlas migrate validate --dir file://migrations
```
