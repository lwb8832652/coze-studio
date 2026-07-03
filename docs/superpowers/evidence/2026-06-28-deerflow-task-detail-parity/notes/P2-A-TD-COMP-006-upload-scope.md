# P2-A TD-COMP-006 Upload Scope

## DeerFlow Reference

- Frontend upload API:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/uploads/api.ts`
  posts `FormData(files)` to `POST /api/threads/:thread_id/uploads`, lists
  uploads through `/uploads/list`, and deletes through `/uploads/:filename`.
- Upload submit integration:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/uploads/hooks.ts`
  provides `useUploadFilesOnSubmit`.
- Composer attachment UI:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/ai-elements/prompt-input.tsx`
  owns file input state, attachment cards, remove actions, and file-dialog
  control.
- Runtime context:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/middlewares/uploads_middleware.py`
  injects uploaded file metadata into `<uploaded_files>` so the agent can read
  `/mnt/user-data/uploads/*`.

## Coze Current State

- Coze task detail/new task composer has a DeerFlow-positioned attachment
  affordance (`button[aria-label="添加附件"]`) and the existing DOM test keeps it
  to one visible button.
- File:
  `frontend/apps/coze-studio/src/pages/workbench/components/workbench-composer-controls.tsx`.
- Test helper:
  `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
  `expectDeerFlowTaskComposer`.

## Gap

The actual upload contract is not implemented in Coze yet:

- no Workbench upload API/client for thread uploads;
- no hidden file input / attachment card state in the Coze composer;
- no submit-time upload step;
- no `uploaded_files` context binding into Go-native Eino ADK runs;
- no upload list/delete API evidence.

This is bigger than a visible P2-A styling/detail debt and should not be
silently marked as parity-complete.

## Scoped Follow-Up

Track full upload parity as P2-F:

1. Add Workbench upload IDL/API: upload/list/delete files for a task thread.
2. Store uploads in a tenant-scoped user-data bucket with filename/path
   validation and size/MIME limits.
3. Add composer file input, attachment cards, remove/retry states, and
   submit-time upload.
4. Inject bounded `<uploaded_files>` metadata into Eino ADK run context.
5. Add frontend, backend, and browser evidence for upload/list/delete and agent
   consumption.
