# Artifact PDF Preview Contract M4 Plan

## Result

M4.40 locks PDF artifact preview behavior to signed URL new-tab previews and
clears stale inline drawer previews before opening non-inline previews.

## Completed

- Added task detail coverage for PDF preview after an inline image preview.
- Kept PDF preview on the existing signed URL endpoint with `mode=preview`.
- Cleared existing inline preview state before opening non-inline previews in
  a new tab.
- Confirmed raster image previews still render inside the drawer and text
  previews still use the content endpoint.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/tasks/__tests__/task-detail.test.tsx --testNamePattern "previews safe text inline"`

## Remaining

PDF embedding, PDF renderer sandboxing, browser E2E, mobile viewport checks,
and CI browser execution remain separate production acceptance work.
