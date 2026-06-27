# Artifact Preview Stale State M4 Plan

## Result

M4.41 clears stale inline preview state whenever a new preview action starts.

## Completed

- Added task detail coverage for an image preview followed by a failed text
  preview.
- Updated preview action handling to clear inline preview state before running
  preview routing.
- Kept download actions from clearing the current preview.
- Confirmed PDF new-tab previews still clear stale inline preview through the
  shared preview-start behavior.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/tasks/__tests__/task-detail.test.tsx --testNamePattern "previews safe text inline"`
- `npx vitest run src/pages/tasks/__tests__/task-artifacts-helpers.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts`
- `npx eslint src/pages/tasks/task-artifact-inline-preview.tsx src/pages/tasks/task-artifact-undo-notice.tsx src/pages/tasks/task-artifacts-panel.tsx src/pages/tasks/task-artifacts-helpers.ts src/pages/tasks/__tests__/task-artifacts-helpers.test.ts src/pages/tasks/__tests__/task-detail.test.tsx --quiet`
- `go test ./application/agentthread ./api/handler/coze ./api/router/coze -count=1 -gcflags="all=-l -N"`
- `/opt/homebrew/bin/atlas migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

Retry UI, browser E2E, PDF embedding, mobile viewport checks, and signed URL
refresh remain separate production acceptance work.
