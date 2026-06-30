# TD-DOC-011 Artifact Redaction

## Scope

- Prevent artifact list and preview UI from rendering internal storage
  metadata.
- Preserve safe user-space file path display for DeerFlow-style workspace and
  output paths.

## Finding

- `TaskArtifactListItem` rendered `artifact.virtual_path` directly.
- If an artifact row contained an internal object URI such as
  `agent-runtime://...?...token=...`, the task artifact drawer could expose it
  to the UI.

## Coze Implementation

- Added `artifactDisplayPath`.
- Only paths under `/mnt/user-data/outputs/` and `/mnt/user-data/workspace/`
  are displayed.
- Internal object URIs, tokenized paths, and raw metadata remain hidden from
  artifact list UI.
- Artifact preview/download action errors remain bounded by the TD-DOC-009
  error handling path.

## Verification

- Red test first: task detail artifact drawer rendered
  `agent-runtime://objects/private/budget.csv?token=object-secret`.
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders canonical thread artifacts"`
- `npm run test -- src/pages/tasks/__tests__/task-artifacts-helpers.test.ts`
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-artifacts-helpers.test.ts`
- `npx tsc --noEmit --project tsconfig.json`
- The combined task detail + artifact helper suites passed with 43 tests.

## Browser Status

- Real browser screenshots and backend response samples are still required for
  final acceptance:
- safe `/mnt/user-data/outputs/...` path display,
- hidden object URI / tokenized path,
- artifact list response with metadata-safe fields.
