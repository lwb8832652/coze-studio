# TD-DOC-012 Artifact Message Cards

## Scope

- Align the DeerFlow `present_files` experience for generated documents.
- Keep thread export (`导出`) separate from artifact/document download.
- Reuse the existing task artifact content and signed URL APIs for preview and
  download.

## DeerFlow Reference

- DeerFlow groups assistant `present_files` messages as
  `assistant:present-files`.
- The message group renders an `ArtifactFileList` in the chat transcript.
- File card click opens artifact preview, while the card download action uses
  the artifact download URL.

## Coze Implementation

- Added `TaskArtifactMessageList` under task detail transcript.
- The list renders active `TaskThreadArtifact` rows as document cards with:
  file name, file type label, size, preview action, and download action.
- Preview and download use shared `useTaskArtifactActions`, the same action
  boundary used by the task artifact panel.
- Active-content artifacts such as HTML still use the artifact download path
  rather than inline execution.

## Verification

- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "renders generated document artifacts in the conversation"`
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`
- `npx tsc --noEmit --project tsconfig.json`
- `TSESTREE_SINGLE_RUN=true npx eslint --fix --cache src/pages/tasks/task-artifact-actions.ts src/pages/tasks/task-artifact-message-list.tsx src/pages/tasks/task-artifacts-panel.tsx src/pages/tasks/detail.tsx src/pages/tasks/__tests__/task-detail.test.tsx`

## Browser Status

- Local frontend `http://localhost:8080` returned HTTP 200.
- Local backend `http://localhost:8888` was reachable and returned HTTP 401
  without browser session cookies.
- In-app browser automation timed out while reading the selected tab, so a live
  screenshot with real generated artifacts remains manual/next-run validation.
