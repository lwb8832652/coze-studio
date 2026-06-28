# TD-COMP-001 / TD-COMP-003 DeerFlow Composer Shell

## Scope

- Task detail follow-up composer uses a DeerFlow-specific presentation.
- Remove the visible `Auto` / `Ask` / `Agent` segmented controls from task detail.
- Hide the task-detail visible `运行设置` button in the composer.
- Keep `拓展`, `@`, attachment, and link entries.
- Show DeerFlow-style mode trigger, defaulting to `Ultra`.
- Use the DeerFlow-like green round send icon.

## Reference

- User-provided DeerFlow target screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/TD-COMP-001-deerflow-composer-target.png`
- DeerFlow source:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/input-box.tsx`

## Coze Code Evidence

- `frontend/apps/coze-studio/src/pages/tasks/task-follow-up-composer.tsx`
- `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer.tsx`
- `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer-controls.tsx`
- `frontend/apps/coze-studio/src/pages/workbench/index.less`
- `frontend/apps/coze-studio/src/components/workspace-prototype.less`
- `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

## Verification

```text
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "keeps the follow-up composer"
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npm run test -- src/pages/workbench/__tests__/workbench.test.tsx
npx tsc --noEmit --project tsconfig.json
TSESTREE_SINGLE_RUN=true npx eslint --fix --cache src/pages/workbench/components/workbench-composer.tsx src/pages/workbench/components/workbench-composer-controls.tsx src/pages/tasks/task-follow-up-composer.tsx src/pages/tasks/__tests__/task-detail.test.tsx
git diff --check
```

## Remaining Validation

- Manual browser visual confirmation on `http://localhost:8080/.../tasks/:thread_id`.
- Capture the opened DeerFlow-style mode menu and compare it with the DeerFlow reference.
- Validate whether the attachment icon is only a visible shell or must be promoted to full P0 upload behavior.
