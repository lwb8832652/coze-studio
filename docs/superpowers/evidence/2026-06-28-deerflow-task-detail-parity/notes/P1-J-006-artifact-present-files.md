# P1-J-006 Artifact / present_files Parity Evidence

Date: 2026-07-01

## Scope

This note closes the P1-J-006 acceptance row for document artifacts created
through `present_files`: message card placement, side preview flow,
download/copy controls, safe metadata, MIME coverage, and final-answer ordering.

The earlier split-width-only evidence remains in
`P1-J-006-artifact-split-width.md`.

## DeerFlow Reference

Source anchors verified from local DeerFlow:

- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/messages/message-list.tsx:331`
  renders `assistant:present-files` groups by collecting present files, optional
  group Markdown content, `ArtifactFileList`, and token usage.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/artifacts/artifact-file-list.tsx:25`
  renders a file card list; clicking a card selects the artifact and opens the
  right panel, `.skill` files show install, and every file shows download.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/artifacts/artifact-file-detail.tsx:58`
  renders the right artifact detail panel with code/preview switch, copy,
  download/open, skill install when applicable, and close.
- `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/artifacts/artifact-file-detail.tsx:400`
  renders Markdown preview with the same artifact panel flow.

Runtime reference task:

- `http://localhost:2026/workspace/chats/fb3b753d-7c88-44e9-949e-5e0a7f860286`

Prompt:

```text
请生成一份《P1-J-006 产物验证》Markdown 简短文档，包含标题、要点列表和一个小表格。请保存为 Markdown 文件，并把文件作为右侧 Artifacts 产物展示，不要只在聊天里输出正文。
```

Visible DeerFlow behavior:

- Chat turn shows a single `P1-J-006-产物验证.md` card with `Markdown file`
  and `下载`.
- Right Artifacts panel opens with `P1-J-006-产物验证.md`, copy-to-clipboard,
  close, and Markdown preview content.
- No unsafe raw fields are visible: `tool_result`, `arguments`,
  `credentials`, `Authorization`, `checkpoint`, `object_uri`, `raw_usage`.

Screenshot:

- `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/deerflow/P1-J-006-deerflow-artifact-document-preview.png`

## Coze Current Behavior

Source anchors verified:

- `backend/application/agentthread/artifact_output.go:302` resolves
  `present_files`, validates output files, registers artifacts with
  `{"source":"present_files"}`, and emits `artifact.presented`.
- `backend/application/agentthread/artifact_output.go:531` emits bounded
  `coze.artifact_presented.v1` event metadata; `safeArtifactEventItems` omits
  object URI and file contents.
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx:530` renders thread
  artifact cards through `TaskArtifactMessageList` with
  `renderReviewActions={false}`.
- `frontend/apps/coze-studio/src/pages/tasks/detail.tsx:970` applies the same
  no-review-card behavior for fallback thread detail rendering.
- `frontend/apps/coze-studio/src/pages/tasks/task-artifact-message-list.tsx:89`
  renders the DeerFlow-style file card, install only for `.skill`, and download
  for every artifact.
- `frontend/apps/coze-studio/src/pages/tasks/task-artifact-message-list.tsx:257`
  auto-previews the first previewable file and opens the side preview through
  the same action path as a click.
- `frontend/apps/coze-studio/src/pages/tasks/task-artifact-inline-preview.tsx:51`
  renders title, code/preview switch, copy, close, and Markdown/table/image/text
  previews.

Runtime reference task:

- `http://localhost:8080/space/7656275718757679104/tasks/7657061782099329024`

Visible Coze behavior:

- Message card shows `java-learning-roadmap.md`, `Markdown file`, and `下载`.
- No visible `放行` / `隔离` / `阻断` buttons are present on the message card.
- Clicking the card opens the right-side Markdown preview panel.
- Right panel includes the artifact title, code/preview switch, copy, close,
  and rendered Markdown content.
- No unsafe raw fields are visible: `tool_result`, `arguments`,
  `credentials`, `Authorization`, `checkpoint`, `object_uri`, `raw_usage`.

Screenshot:

- `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-J-006-coze-artifact-document-preview.png`

## DOM Evidence Summary

DeerFlow:

```json
{
  "hasMarkdownFile": true,
  "hasDownload": true,
  "hasArtifactsPanel": true,
  "fileCard": "P1-J-006-产物验证.md Markdown file 下载",
  "panelActions": ["复制到剪贴板", "关闭"],
  "unsafeHits": []
}
```

Coze:

```json
{
  "hasMarkdownFile": true,
  "hasDownload": true,
  "visibleReviewButtons": [],
  "visiblePreviewButtons": [],
  "visibleDownloadButtons": ["下载"],
  "artifactCard": "java-learning-roadmap.md Markdown file 下载",
  "rightPreview": "java-learning-roadmap.md ... Markdown preview",
  "unsafeHits": []
}
```

## Automated Verification

Commands:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx -t "generated document artifacts|document artifact cards|presented document cards"

cd backend
go test ./application/agentthread -run 'TestApplication(WriteOutputFileStoresObjectAndRegistersOutputFile|CreateSkillPackageWritesInstallableSkillArchive|PresentOutputFilesRegistersArtifactsAndEmitsSafeEvent)' -count=1
```

Result:

- Frontend targeted suite passed: 3 tests, 46 skipped.
- Backend targeted suite passed.

Covered assertions:

- Generated document cards render in a DeerFlow-style side preview and do not
  mix with thread export.
- Non-skill generated document cards do not show review actions.
- Copy button is icon-only in the preview header.
- Markdown preview renders as Markdown, not raw text, and long previews show a
  bounded truncation notice.
- Document artifact cards remain attached to their owning assistant turn.
- If `artifact.presented` happens before the final assistant answer, the card is
  rendered before the final answer.
- `present_files` registers artifacts with bounded `source=present_files`
  metadata and emits a safe `artifact.presented` event without object URI or
  file content.

## Remaining P1 Notes

P1-J-006 is complete for Markdown/document artifacts and `.skill` action
semantics. Broader visual polish for the reasoning/steps block remains in
P1-J-008; streaming stop/retry/follow-up behavior remains in P1-J-007.
