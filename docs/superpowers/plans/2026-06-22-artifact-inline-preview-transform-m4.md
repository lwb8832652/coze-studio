# Artifact Inline Preview Transform M4 Plan

## Result

M4.36 adds in-drawer previews for safe text-like task artifacts while keeping
active content download-only.

## Completed

- Added artifact preview helper APIs for detecting inline-safe text artifact
  families.
- Added bounded transforms for plain text, Markdown source, JSON
  pretty-printing, and CSV/TSV table previews.
- Kept Markdown as escaped source text and avoided HTML rendering.
- Switched safe text preview actions from signed URL new-tab opening to the
  existing Workbench content endpoint with `mode=preview`.
- Kept signed URLs for downloads and for non-text safe preview families such
  as image/PDF.
- Rendered CSV/TSV through Coze Design `Table`; rendered other text as React
  text in a `<pre>`.
- Added UI and helper tests covering inline text preview and safe transforms.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/tasks/__tests__/task-artifacts-helpers.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/tasks-service.test.ts`
- `npx eslint src/pages/tasks/task-artifact-inline-preview.tsx src/pages/tasks/task-artifact-undo-notice.tsx src/pages/tasks/task-artifacts-panel.tsx src/pages/tasks/task-artifacts-helpers.ts src/pages/tasks/__tests__/task-artifacts-helpers.test.ts src/pages/tasks/__tests__/task-detail.test.tsx --quiet`
- `go test ./application/agentthread ./api/handler/coze ./api/router/coze -count=1 -gcflags="all=-l -N"`
- `/opt/homebrew/bin/atlas migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

Browser E2E coverage, image/PDF drawer embedding, MIME-specific renderer
hardening, renderer sandboxing, and object-store reconciliation remain open
production acceptance work.
