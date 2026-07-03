# P2-F-UPLOAD-002-007 Attachment Runtime Loop

## Scope

This note closes the P2-F upload implementation items after
`P2-F-UPLOAD-001` established the DeerFlow reference contract.

Covered items:

- backend upload storage and API;
- deferred new-task submit with attachments;
- canonical follow-up attachments;
- ADK uploaded-files runtime injection;
- frontend composer file picker and attachment chips;
- targeted verification.

## DeerFlow Reference

Previously verified DeerFlow sources:

- frontend upload API:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/uploads/api.ts`;
- frontend upload submit hook:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/uploads/hooks.ts`;
- composer attachment UI:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/input-box.tsx`;
- backend upload router:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/uploads.py`;
- runtime upload middleware:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/middlewares/uploads_middleware.py`.

The relevant behavior is: upload files to the thread before the run starts,
carry bounded uploaded-file metadata in message/run input, expose
`/mnt/user-data/uploads/<filename>` to runtime tools, and inject a
`<uploaded_files>` model context block without exposing storage object URIs.

## Coze Implementation

Backend:

- added task-thread upload/list/delete APIs under
  `/api/workbench/task_threads/:thread_id/uploads`;
- reused `agent_files` with `file_kind=upload` and `run_id=0`;
- enforced thread owner, space, status, safe filename, per-file size, total
  size, and file-count limits;
- added duplicate filename normalization;
- returned bounded file metadata only;
- extended runtime file resolution so `read_file` can read
  `/mnt/user-data/uploads/*` through the upload registry;
- added `ADKUploadedFilesMiddleware` to inject a DeerFlow-like
  `<uploaded_files>` block into the latest user model message;
- accepted both top-level `uploaded_files` and DeerFlow-compatible
  `messages[].additional_kwargs.files` metadata.

Frontend:

- the composer paperclip opens a hidden multi-file input;
- selected files render as removable chips;
- new-task attachment submit creates the thread with `defer_start`, uploads
  files, appends the message, then creates the run;
- canonical task-detail follow-up uploads files before appending the user
  message and creating the run, then preserves the fetched history in run
  input;
- no-attachment new-task behavior remains the existing one-call create path.

## Verification

Backend targeted verification:

```bash
cd backend && go test ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread ./api/handler/coze -run 'TestUploadFileService|TestRuntimeFileServiceResolvesOwnedUploadFile|TestRuntimeFileRepository(UpsertsByRunAndVirtualPath|ManagesThreadUploads)|TestApplication.*TaskThreadUploadFiles|TestApplicationCreateTaskThreadCanDeferRunStartForUploads|TestADKUploadedFilesMiddleware|TestADKOffloadBackendReadsUploadedThreadFile|Test.*TaskThread.*Upload|TestCreateTaskThread' -count=1 -gcflags='all=-N -l'
```

Result: passed across 4 packages.

Frontend targeted verification:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
```

Result: 2 files passed, 89 tests passed.

Frontend P2 matrix verification:

```bash
cd frontend/apps/coze-studio && npm run test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/task-memory-section.test.tsx src/pages/skill/__tests__/skill.test.tsx src/pages/tools/__tests__/tools.test.tsx src/pages/workbench/__tests__/workbench.test.tsx
```

Result: 6 files passed, 133 tests passed.

Whitespace verification:

```bash
git diff --check
```

Result: passed.

## Deferred Or Explicitly Out Of Scope

- No IM-channel attachments.
- No Workbench-visible object URI or raw storage metadata.
- No production malware scanning UI. Upload failures currently use the composer
  error surface; richer per-file scanner/failure badges should be a separate
  product decision.
- Manual browser verification was not run in this slice; automated DOM and
  backend contract coverage is recorded above.
