# M3.20 Eino ADK Subagent Timeline Plan

## Checklist

- [x] Add task detail red test for child lifecycle timeline rendering.
- [x] Group parent-run `subagent.run.*` events by `child_run_id`.
- [x] Attach ordered timeline rows to durable subagent child runs.
- [x] Render an expandable `执行明细` section under each matching child card.
- [x] Keep timeline fields content-free and metadata-only.
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
