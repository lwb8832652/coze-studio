# M3.18 Eino ADK Subagent Event Join Plan

## Checklist

- [x] Add task detail red test for lifecycle event `child_run_id` join.
- [x] Parse latest `subagent.run.*` event metadata by child run ID.
- [x] Join `elapsed_ms`, `terminal_classification`, and sanitized lifecycle
      error fallback into the subagent card view model.
- [x] Render lifecycle duration and classification in the subagent state card.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run task-detail frontend tests.

## Verification

```bash
cd frontend/apps/coze-studio && npm run test -- \
  src/pages/tasks/__tests__/task-detail.test.tsx
```

Final wider verification is tracked from the active implementation session
before handoff.
