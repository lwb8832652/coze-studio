# TD-DOC-004 / TD-DOC-006 / TD-DOC-010 Table Image Lifecycle

## Scope

- Verify CSV/table artifact preview in task detail.
- Verify image artifact preview stays on signed URL image preview.
- Confirm artifact delete/restore flows keep list and preview state coherent.

## Coze Implementation

- CSV artifacts with `preview_mode='text'` and `content_type='text/csv'` are
  fetched through `/content?mode=preview`, parsed into columns/rows, and
  rendered through the table preview branch.
- Image artifacts such as PNG use artifact signed URL `mode='preview'` and
  render an `<img>` preview. SVG remains rejected by MIME-family safety rules.
- Deleting the currently previewed artifact clears the preview and refreshes
  the active artifact list. Undo restore and deleted-list restore both reload
  artifact state through the existing Workbench task artifact APIs.

## Verification

- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders canonical thread artifacts"`
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-artifacts-helpers.test.ts`
- `npx tsc --noEmit --project tsconfig.json`
- The combined task detail + artifact helper suites passed with 42 tests.

## Browser Status

- Real browser screenshots and network samples are still required for final
  acceptance:
- CSV table artifact preview,
- PNG/image artifact preview,
- delete current artifact and preview close,
- restore artifact from undo and deleted list.
