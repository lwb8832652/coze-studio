# M3.16 Eino ADK Subagent Child Run API Plan

## Checklist

- [x] Add handler test for `parent_run_id` child run listing.
- [x] Update workbench task-thread test schema for child run columns.
- [x] Add `parent_run_id` query support to `ListTaskThreadRuns`.
- [x] Add `parent_run_id` and `run_kind` fields to task-thread run API
      responses.
- [x] Sync frontend workbench task schema types.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused handler tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./api/handler/coze \
  -run 'TestListTaskThreadRunsHandlerReturnsRuns|TestListTaskThreadRunsHandlerReturnsChildRunsForParentRun' \
  -count=1
```

Final package-level verification is tracked from the active implementation
session before handoff.
