# M3.19 Eino ADK Subagent Token Rollup Plan

## Checklist

- [x] Add task detail red test for per-child run token usage calls and card
      display.
- [x] Fetch run-scoped token usage for each durable subagent child run.
- [x] Reuse the existing token usage aggregate mapping.
- [x] Render child total token count and call count in subagent state cards.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run task page tests and targeted lint.

## Verification

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
```

```bash
cd frontend/apps/coze-studio && npx eslint \
  src/pages/tasks/task-detail-loader.ts \
  src/pages/tasks/detail.tsx \
  src/pages/tasks/task-detail-hooks.ts \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  --cache --quiet
```
