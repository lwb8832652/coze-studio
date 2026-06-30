# TD-DOC-003 / TD-DOC-008 Markdown Preview And Download

## Scope

- Align generated Markdown document preview with DeerFlow artifact preview.
- Keep document download separate from top-bar task export.
- Preserve safe fallback behavior for active-content files.

## DeerFlow Reference

- DeerFlow renders Markdown artifacts through the artifact preview panel rather
  than as raw `<pre>` text.
- Artifact file cards open preview on click and keep a dedicated download
  action for the file.

## Coze Implementation

- `TaskArtifactInlinePreview` now treats `ArtifactInlinePreview.kind ===
  'markdown'` as Markdown preview content.
- Markdown artifacts keep the code/preview toggle. Preview mode renders through
  `TaskMarkdownContent`; code mode still exposes the raw source view.
- Long text previews display a bounded truncation notice so users know the
  panel is showing only part of the file and can download the full document.
- Generated document message cards keep per-file download buttons. Markdown and
  HTML document downloads both call the artifact signed URL API with
  `mode='download'` and do not use the top-bar task export path.

## Verification

- Red test first: `task-artifact-inline-preview-markdown` was missing for a
  Markdown generated document.
- Red test first: truncation notice was missing for an oversized Markdown
  preview.
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders generated document artifacts"`
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-artifacts-helpers.test.ts`
- `npx tsc --noEmit --project tsconfig.json`
- The focused document artifact case passed, and the combined task detail +
  artifact helper suites passed with 42 tests.

## Browser Status

- Real browser screenshots and network samples are still required for final
  acceptance:
- generated Markdown side preview screenshot,
- truncation notice screenshot with a large document,
- Markdown download request summary,
- HTML or active-content download request summary.
