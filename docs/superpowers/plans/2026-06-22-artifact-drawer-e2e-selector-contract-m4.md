# Artifact Drawer E2E Selector Contract M4 Plan

## Result

M4.37 adds stable task artifact drawer selectors and tests them through the
existing Vitest coverage.

## Completed

- Added `data-testid="task-artifacts-open"` to the artifact drawer entry.
- Added `data-testid="task-artifact-item"` and `data-artifact-id` to visible
  artifact rows.
- Added stable inline preview selectors for the preview surface, source-text
  preview, and table preview.
- Extended the task detail artifact test to assert the selector contract.
- Documented that this is not full browser E2E coverage because the app does
  not currently include a runnable Playwright or Cypress harness.

## Verification

- `npx vitest run src/pages/tasks/__tests__/task-detail.test.tsx --testNamePattern "previews safe text inline"`
- `npx vitest run src/pages/tasks/__tests__/task-artifacts-helpers.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts`
- `npx eslint src/pages/tasks/task-artifact-inline-preview.tsx src/pages/tasks/task-artifact-undo-notice.tsx src/pages/tasks/task-artifacts-panel.tsx src/pages/tasks/task-artifacts-helpers.ts src/pages/tasks/__tests__/task-artifacts-helpers.test.ts src/pages/tasks/__tests__/task-detail.test.tsx --quiet`
- `go test ./application/agentthread ./api/handler/coze ./api/router/coze -count=1 -gcflags="all=-l -N"`
- `/opt/homebrew/bin/atlas migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

True browser E2E coverage, image/PDF renderer checks, mobile viewport checks,
and CI browser execution remain production acceptance work.
