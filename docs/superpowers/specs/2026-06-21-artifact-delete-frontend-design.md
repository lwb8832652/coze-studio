# M4.12 Artifact Delete Frontend Design

## Scope

M4.12 wires the M4.11 artifact soft-delete endpoint into canonical task-thread
detail pages. The task object and menu wording stay as `任务`; this slice only
adds a user action inside the existing `任务产物` drawer.

The backend remains the source of truth. The frontend never decides physical
cleanup, restore, retention, or object-storage deletion.

## UI Contract

- Each artifact row shows a `删除` action next to the existing `预览` and
  `下载` actions.
- The delete action is wrapped in a Semi `Popconfirm` because the row will
  disappear from task detail after confirmation.
- The confirm copy states that this removes the artifact from the task detail
  view and does not delete the underlying file.
- The cancel button receives initial focus for the destructive confirmation.
- While one artifact action is running, other artifact actions are disabled.
- Delete errors are shown inside the drawer through the existing artifact error
  surface.

## Data Flow

1. `TaskArtifactsPanel` receives `threadId`, current artifacts, and an optional
   refresh callback from `TaskDetailPage`.
2. On confirm, it calls
   `DELETE /api/workbench/task_threads/:thread_id/artifacts/:artifact_id`
   through a small `deleteTaskThreadArtifact` service helper.
3. On success, it calls the refresh callback.
4. `useTaskDetailData.refreshArtifacts` reloads
   `ListTaskThreadArtifacts({ page: 1, page_size: 50 })` for canonical thread
   detail pages and updates only the artifact list state.
5. The drawer stays open, so the user can see the updated empty/list state
   without losing detail-page context.

## Safety Rules

- The client sends only path-scoped `thread_id` and `artifact_id`.
- The client never sends or renders object URI, storage URL, file ID, or raw
  artifact bytes for deletion.
- Deletion is a soft delete of the artifact presentation registry. Physical
  cleanup, restore, retention, and orphan reconciliation remain backend worker
  responsibilities.
- Frontend events and errors must not include object URIs, virtual paths,
  storage URLs, filenames from storage, file bytes, prompts, model output, tool
  arguments, checkpoint bytes, credentials, or provider raw bodies.

## Semi Components

Use `Popconfirm` from the Coze Design/Semi wrapper with `okType="danger"` and
`cancelButtonProps={{ autoFocus: true }}`. Semi `2.72.3` supports async
`onConfirm`, loading states for promise-returning confirm handlers, and
keyboard/focus return behavior for click triggers.

Use the existing `Button` pattern with `size="small"`, `theme="borderless"`,
`type="danger"`, `loading`, `disabled`, an `aria-label`, and
`IconCozTrashCan`.

## Tests

- Service test covers the DELETE helper, encoded route params, and
  `x-requested-with` header.
- Task detail test covers opening the artifact drawer, confirming delete,
  calling the DELETE helper with scoped IDs, and refreshing the artifact list
  to the empty state.
