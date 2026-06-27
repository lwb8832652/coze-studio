# M3.22 Eino ADK Subagent Token Run Aggregates Plan

## Checklist

- [x] Add red handler assertion for `run_aggregates`.
- [x] Add per-run aggregate DTOs to backend API and frontend schema.
- [x] Add repository `AggregateTokenUsageByRun`.
- [x] Thread grouped aggregates through domain and application services.
- [x] Update task detail loader to use parent `include_child_runs=true`.
- [x] Keep missing child usage omitted rather than rendered as zero.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.

## Verification

```bash
cd backend && go test ./api/handler/coze ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository -count=1 -gcflags="all=-l -N"
```

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__
```

```bash
cd frontend/apps/coze-studio && npx eslint \
  src/pages/tasks/task-detail-loader.ts \
  src/pages/tasks/task-detail-subagents.ts \
  src/pages/tasks/task-detail-token-usage.ts \
  src/pages/tasks/task-subagent-runs-section.tsx \
  src/pages/tasks/detail.tsx \
  src/pages/tasks/task-detail-hooks.ts \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  --cache --quiet
```
