# DeerFlow Upload Parity Plan

## Scope

This plan implements P2-F: Composer upload/attachment full loop.

Non-goals for this slice:

- no IM channel attachments;
- no production malware scanning UI beyond existing artifact/guardrail
  metadata-only surfaces;
- no broad redesign of the DeerFlow-style composer;
- no local filesystem path exposure to Workbench API/UI.

## Work Items

### P2-F-UPLOAD-001 Reference And Contract

Status: 已完成

- DeerFlow source chain verified for frontend API, submit hook, composer UI,
  backend upload router, and runtime uploads middleware.
- Coze gap documented in
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P2-A-TD-COMP-006-upload-scope.md`.
- Design documented in
  `docs/superpowers/specs/2026-07-03-deerflow-upload-parity-design.md`.

### P2-F-UPLOAD-002 Backend Upload Storage And API

Status: 已完成

- Added upload service using existing `agent_files` table and object storage.
- Added task-thread upload/list/delete API handlers and routes.
- Enforced thread owner, space, filename, size, total-size, and upload count
  checks.
- Returned bounded metadata only; storage object URI remains server-side.

### P2-F-UPLOAD-003 Deferred New-Task Submit

Status: 已完成

- Added optional `defer_start` create-thread path.
- Kept no-attachment behavior unchanged.
- For attachment path, create thread first, upload files, append message, then
  create run.

### P2-F-UPLOAD-004 Follow-Up Attachments

Status: 已完成

- Uploads files before canonical follow-up run creation.
- Preserves fetched prior history in the run input.
- Stores bounded file metadata on run input as `uploaded_files`.

### P2-F-UPLOAD-005 ADK Uploaded-Files Runtime

Status: 已完成

- Added uploaded-files middleware that injects a DeerFlow-like
  `<uploaded_files>` block into the latest user model message.
- Added `/mnt/user-data/uploads/*` resolve support for runtime `read_file`.
- Keeps object URI and raw storage metadata hidden from UI/API.

### P2-F-UPLOAD-006 Frontend Composer Attachments

Status: 已完成

- Wired the paperclip to a hidden multi-file input.
- Renders selected attachment chips with remove action.
- Passes selected files through new-task and canonical follow-up submit
  payloads.
- Keeps current @ resource selection behavior intact.
- Upload failures surface through the existing composer error area; richer
  per-file scanner/failure badges are deferred unless product decides to add a
  visible safety review state.

### P2-F-UPLOAD-007 Verification And Evidence

Status: 已完成

- Targeted Go and frontend tests passed.
- P2 task/workbench/skill/tools Vitest matrix passed.
- Evidence note added:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P2-F-UPLOAD-002-007-attachment-runtime-loop.md`.

Verification commands:

```bash
cd backend && go test ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread ./api/handler/coze -run 'TestUploadFileService|TestRuntimeFileServiceResolvesOwnedUploadFile|TestRuntimeFileRepository(UpsertsByRunAndVirtualPath|ManagesThreadUploads)|TestApplication.*TaskThreadUploadFiles|TestApplicationCreateTaskThreadCanDeferRunStartForUploads|TestADKUploadedFilesMiddleware|TestADKOffloadBackendReadsUploadedThreadFile|Test.*TaskThread.*Upload|TestCreateTaskThread' -count=1 -gcflags='all=-N -l'
cd frontend/apps/coze-studio && npm run test -- src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/skill/__tests__/skill.test.tsx src/pages/tools/__tests__/tools.test.tsx src/pages/workbench/__tests__/workbench.test.tsx
git diff --check
```

## Implementation Order

1. Backend service/repository/API tests.
2. Backend upload implementation.
3. Deferred submit and run input metadata tests.
4. ADK upload middleware and `read_file` upload resolve tests.
5. Frontend service/composer submit tests.
6. Frontend implementation.
7. Matrix tests and tracker/evidence update.

This order keeps the visible UI dependent on a real data contract, matching the
DeerFlow parity rule.
