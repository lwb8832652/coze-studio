# Artifact Preview Family Hardening M4 Plan

## Result

M4.38 centralizes task artifact frontend preview family decisions and adds
active-content mismatch tests.

## Completed

- Added `artifactPreviewFamily` as the shared frontend preview family helper.
- Changed `canPreviewArtifact` to delegate to the shared helper.
- Changed `artifactInlinePreviewKind` to require the shared text preview
  family before selecting JSON, CSV, TSV, Markdown, or plain text renderers.
- Added tests for parameterized/case-insensitive safe MIME types.
- Added tests rejecting HTML, XHTML, SVG, octet-stream, empty content types,
  and SVG/image mismatches.
- Updated AGENTS and the master roadmap.

## Verification

- `npx vitest run src/pages/tasks/__tests__/task-artifacts-helpers.test.ts`

## Remaining

Backend authority, signed URL safety, browser E2E, image/PDF drawer embedding,
and renderer sandboxing remain separate acceptance work.
