# TD-DOC-005 / TD-DOC-007 / TD-DOC-009 Artifact Safety And Errors

## Scope

- Keep binary document preview/download on signed URL boundaries.
- Prevent HTML/SVG/active content from rendering inline in the app origin.
- Show bounded artifact action errors without leaking raw provider details.

## DeerFlow Reference

- DeerFlow treats generated files as artifacts with explicit preview and
  download actions.
- Active document content is not rendered as executable same-origin app UI.
- User-visible failures stay concise and do not expose internal payloads.

## Coze Implementation

- PDF preview uses artifact signed URL `mode='preview'` and opens in a new
  browser context instead of rendering raw bytes in the chat transcript.
- HTML and SVG-like active content are rejected by `artifactPreviewFamily` for
  text/image inline preview even if an artifact row is incorrectly marked with
  a text/image preview mode.
- Active-content files still expose download through artifact signed URL
  `mode='download'`.
- Artifact preview/download failures now show bounded user-facing messages:
  `读取任务产物失败，请稍后重试` or `下载任务产物失败，请稍后重试`.
- Raw error strings, long provider payloads, tokens, object URLs, and internal
  exception messages are not rendered into the task detail UI for these action
  failures.

## Verification

- Red test first: preview failure rendered a raw error containing
  `token=super-secret` and a long repeated payload.
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders canonical thread artifacts"`
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-artifacts-helpers.test.ts`
- `npx tsc --noEmit --project tsconfig.json`
- The combined task detail + artifact helper suites passed with 42 tests.

## Browser Status

- Real browser screenshots and network samples are still required for final
  acceptance:
- PDF preview/open action,
- HTML/SVG download-only behavior,
- failed preview bounded error state,
- signed URL request summaries for preview and download.
