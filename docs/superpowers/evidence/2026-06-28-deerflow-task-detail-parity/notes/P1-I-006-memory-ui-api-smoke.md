# P1-I-006 Memory UI/API Smoke Evidence

Date: 2026-07-01

## Scope

Validate that task-memory management agrees with runtime-written memory:

- task detail memory panel lists the runtime-created long-term memory;
- edit, delete, deleted-view, and restore actions work from the UI;
- backend state and audit events match the visible UI state;
- memory metadata remains bounded.

## Browser UI Evidence

Task:

`http://localhost:8080/space/7656275718757679104/tasks/7657481311426183168`

Screenshots:

- `screenshots/coze/P1-I-006-memory-panel-list.png`
- `screenshots/coze/P1-I-006-memory-deleted.png`
- `screenshots/coze/P1-I-006-memory-restored.png`

Observed flow:

1. Opened task `详情` panel and verified `任务记忆` listed 1 item.
2. The listed item was the runtime-created `long_term` memory for `银杏计划`.
3. Edited the test memory content to append `P1-I-006 UI smoke 已编辑`.
4. Deleted the memory; default list changed to `0 条` and displayed empty state.
5. Opened `已删除`; the deleted record was available with `恢复`.
6. Confirmed restore; default list returned to `1 条` and the deleted marker was
   gone.

## Backend State Evidence

Safe database summary after restore:

- `active_memory_count`: 1
- `deleted_memory_count`: 0
- active memory:
  - `scope`: `long_term`
  - `source_type`: `transcript_summary`
  - `metadata_keys`: `category`
  - `confidence`: `1.0`
- audit events:
  - `memory.updated`
  - `memory.deleted`
  - `memory.restored`
- `unsafe_memory_key_hits`: 0 for prompt, completion, tool result, checkpoint,
  object URI, signed URL, and related unsafe metadata markers.

## API Evidence Boundary

The UI actions above use the same Workbench memory APIs as the generated client.
A direct temporary-browser navigation to
`/api/workbench/task_threads/:thread_id/memories` was blocked by the browser
security layer with `ERR_BLOCKED_BY_CLIENT`, so this evidence records API
effects through the real UI calls plus database state/audit verification rather
than embedding a raw JSON API payload.

## Verification

```bash
cd backend
go test ./api/handler/coze -run 'TestExportTaskThreadMemoriesHandlerReturnsSchemaPayload|TestImportTaskThreadMemoriesHandlerCreatesMemoriesAndAuditsActor|TestListTaskThreadMemoriesHandlerPassesSessionViewerID|TestListTaskThreadMemoriesHandlerMapsAuthorizationDeniedToForbidden' -count=1 -gcflags="all=-N -l"
```

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/tasks/__tests__/task-memory-section-icons.test.tsx
```

Result: both passed.
