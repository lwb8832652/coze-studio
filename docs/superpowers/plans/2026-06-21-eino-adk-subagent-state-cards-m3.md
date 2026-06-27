# M3.17 Eino ADK Subagent State Cards Plan

## Checklist

- [x] Add task detail red test for child run loading and state-card rendering.
- [x] Load top-level thread runs and child runs through `parent_run_id`.
- [x] Map child run rows to a content-free frontend `TaskDetailSubagentRun`
      view model.
- [x] Store subagent rows in `useTaskDetailData` and render them in the task
      detail page.
- [x] Add scoped prototype styles for subagent rows and status chips.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run task-detail frontend tests.

## Verification

```bash
cd frontend/apps/coze-studio && npm run test -- \
  src/pages/tasks/__tests__/task-detail.test.tsx
```

Final wider verification is tracked from the active implementation session
before handoff.
