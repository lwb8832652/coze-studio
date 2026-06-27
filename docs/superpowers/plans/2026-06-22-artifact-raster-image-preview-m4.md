# Artifact Raster Image Preview M4 Plan

## Result

M4.39 renders safe raster image artifact previews inside the task drawer using
backend signed preview URLs.

## Completed

- Extended the task artifact preview state to support content and raster image
  renderers.
- Kept safe text previews on the content endpoint.
- Routed image-family previews through signed URL creation and rendered the
  returned URL as `<img src>`.
- Kept PDF previews on the existing signed URL new-tab behavior.
- Added bounded image preview styling with `object-fit: contain`.
- Added task detail coverage proving image preview does not call
  `window.open` and renders the signed URL only in the image element.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/tasks/__tests__/task-detail.test.tsx --testNamePattern "previews safe text inline"`
- `npx vitest run src/pages/tasks/__tests__/task-artifacts-helpers.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts`
- `npx eslint src/pages/tasks/task-artifact-inline-preview.tsx src/pages/tasks/task-artifact-undo-notice.tsx src/pages/tasks/task-artifacts-panel.tsx src/pages/tasks/task-artifacts-helpers.ts src/pages/tasks/__tests__/task-artifacts-helpers.test.ts src/pages/tasks/__tests__/task-detail.test.tsx --quiet`
- `go test ./application/agentthread ./api/handler/coze ./api/router/coze -count=1 -gcflags="all=-l -N"`
- `/opt/homebrew/bin/atlas migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

PDF embedding, SVG rejection browser coverage, mobile visual checks, browser
E2E, and renderer sandboxing remain separate production acceptance work.
