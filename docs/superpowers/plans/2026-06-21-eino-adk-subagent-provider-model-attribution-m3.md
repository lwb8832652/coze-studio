# M3.23 Eino ADK Subagent Provider Model Attribution Plan

## Checklist

- [x] Add red task-detail test for provider/model labels on subagent cards.
- [x] Derive per-child attribution from parent rollup usage rows.
- [x] Render only bounded `provider / model_name` metadata.
- [x] Keep missing attribution omitted.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run task page tests and targeted lint.

## Verification

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
```

```bash
cd frontend/apps/coze-studio && npx eslint \
  src/pages/tasks/task-detail-subagents.ts \
  src/pages/tasks/task-subagent-runs-section.tsx \
  src/pages/tasks/task-detail-loader.ts \
  src/pages/tasks/task-detail-hooks.ts \
  src/pages/tasks/detail.tsx \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  --cache --quiet
```
