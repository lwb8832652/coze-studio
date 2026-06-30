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
- file name, file type label, card-click preview, and download action.
- For canonical thread detail, artifact cards are grouped by owning `run_id`
  and rendered under the matching assistant turn, matching DeerFlow's
  `assistant:present-files` placement instead of appending the whole thread
  artifact list at the end of the transcript.
- Preview and download use shared `useTaskArtifactActions`, the same action
  boundary used by the task artifact panel. Task detail now shares one artifact
  action state across all message-card groups so only one side preview is open.
- Auto-preview selects only the latest previewable generated artifact. Older
  artifacts remain available in their original assistant turn without stealing
  focus after a follow-up creates a newer same-name document.
- Active-content artifacts such as HTML still use the artifact download path
  rather than inline execution.

## Verification

- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "places document artifact cards in their owning assistant turns and previews the latest generated file"`
- `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx`
- `npx tsc --noEmit --project tsconfig.json`
- The full task-detail test suite passed with 40 tests.
- 2026-06-30 follow-up verification:
  `npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx` passed
  with 49 tests, `npx eslint src/pages/tasks/detail.tsx
  src/pages/tasks/task-artifact-message-list.tsx
  src/pages/tasks/task-execution-todo-dock.tsx
  src/pages/tasks/__tests__/task-detail.test.tsx
  src/components/workspace-prototype.less --quiet` passed, and
  `git diff --check` passed.

## Browser Status

- Local frontend
  `http://localhost:8080/space/7656275718757679104/tasks/7657061782099329024`
  returned HTTP 200 and was verified with in-app browser DOM reads.
- Screenshot evidence:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/TD-DOC-012-artifact-side-preview-layout-7657061782099329024.png`
  and
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/TD-DOC-012-artifact-message-card-7657061782099329024.png`.
- DOM verification on the generated Java roadmap task: artifact message card
  text is `java-learning-roadmap.mdMarkdown file下载`, visible card buttons are
  only `下载`, execution-feed count is `2`, and To-dos dock count is `0` for
  this non-todo document scene.
- Wide viewport verification at 1625x875: task page width is 1325px, chat
  detail width is 723px, and artifact side preview width is 530px, matching the
  DeerFlow 60/40 side-panel direction rather than a fixed narrow preview.
- Direct unauthenticated curl to
  `/api/workbench/task_threads/7657061782099329024/artifacts?space_id=7656275718757679104`
  returns `401 missing session_key in cookie`; keep artifact API response
  capture as browser-network or authenticated-session evidence, not as a raw
  cookie reuse step.
