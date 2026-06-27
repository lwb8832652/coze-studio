# Artifact Preview MIME Hardening M4 Plan

## Result

M4.32 hardens task artifact drawer preview actions with a frontend MIME
allow-list that must agree with `preview_mode`.

## Completed

- Added a regression test for a misclassified HTML artifact with
  `preview_mode=text`.
- Updated `canPreviewArtifact` so preview buttons require both safe
  `preview_mode` and matching safe `content_type`.
- Kept HTML, XHTML, SVG, octet-stream, missing content types, and MIME/mode
  mismatches download-only in the drawer.
- Preserved the backend content endpoint as the final authority for scanning,
  sniffing, attachment disposition, and byte reads.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/tasks/__tests__/task-detail.test.tsx --testNamePattern="renders canonical thread artifacts"`

## Remaining

Physical cleanup, richer preview transforms, signed download response-header
overrides, and browser E2E coverage remain separate M4 or production
acceptance tasks.
