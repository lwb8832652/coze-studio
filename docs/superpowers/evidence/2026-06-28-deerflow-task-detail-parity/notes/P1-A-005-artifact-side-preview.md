# P1-A-005 Artifact Side Preview

## Scope

This P1-A evidence records the paired DeerFlow/Coze Markdown artifact card and
right-side preview baseline. It reuses the already verified P1-J-006
`present_files` artifact parity run, because that run directly covers this
acceptance point without needing another model invocation.

## DeerFlow Evidence

- Source anchors:
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-list.tsx`
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/artifacts/artifact-file-list.tsx`
  - `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/artifacts/artifact-file-detail.tsx`
- Runtime task:
  `http://localhost:2026/workspace/chats/fb3b753d-7c88-44e9-949e-5e0a7f860286`
- P1-A screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-A-005-deerflow-artifact-side-preview.png`

Visible behavior:

- The assistant turn renders a Markdown file card with a download action.
- Opening/selecting the card shows the right artifact panel.
- The right panel includes the file title, preview/code switch, copy, close,
  and rendered Markdown preview.

## Coze Evidence

- Source anchors:
  - `backend/application/agentthread/artifact_output.go`
  - `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
  - `frontend/apps/coze-studio/src/pages/tasks/task-artifact-message-list.tsx`
  - `frontend/apps/coze-studio/src/pages/tasks/task-artifact-inline-preview.tsx`
- Runtime task:
  `http://localhost:8080/space/7656275718757679104/tasks/7657061782099329024`
- P1-A screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-005-coze-artifact-side-preview.png`

Visible behavior:

- The assistant turn renders a `java-learning-roadmap.md` Markdown file card
  with a download action.
- The generated document card does not show scan-review actions such as
  `放行` / `隔离` / `阻断`.
- Opening/selecting the card shows the right-side Markdown preview panel.
- The right panel includes the file title, preview/code switch, copy, close,
  and rendered Markdown content.
- The side panel uses the DeerFlow 60/40 split baseline recorded in
  `P1-J-006-artifact-split-width.md`.

## Automated Verification

Existing P1-J-006 commands remain the acceptance test coverage for this P1-A
baseline:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "generated document artifacts|document artifact cards|presented document cards"

cd backend
go test ./application/agentthread -run 'TestApplication(WriteOutputFileStoresObjectAndRegistersOutputFile|CreateSkillPackageWritesInstallableSkillArchive|PresentOutputFilesRegistersArtifactsAndEmitsSafeEvent)' -count=1
```

Covered behavior:

- Generated document cards render in their owning assistant turn.
- The latest generated artifact auto-opens the side preview.
- Non-skill document artifacts show download only, not install or scan-review
  actions.
- Markdown previews render as Markdown.
- `present_files` emits bounded `artifact.presented` metadata without object
  URI or file contents.

## Security Redaction

No screenshot or summary intentionally records raw object URI, signed URL,
provider payload, tool arguments, tool results, credentials, checkpoint bytes,
or raw storage paths. Artifact content visible in the preview is user-facing
document content.

## Remaining Gap

No open P1-A-005 gap remains for Markdown artifact card and side preview.
Broader MIME sample coverage remains under P1-C.
