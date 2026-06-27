# M4.12 Artifact Delete Frontend Plan

## Checklist

- [x] Add red service test for the artifact DELETE helper.
- [x] Add red task-detail UI test for delete confirmation and artifact refresh.
- [x] Implement `deleteTaskThreadArtifact` in the task service module.
- [x] Add `refreshArtifacts` to canonical thread detail state.
- [x] Wire `TaskArtifactsPanel` delete action with Semi `Popconfirm`.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted frontend tests, lint, and diff checks.

## Notes

- Keep product wording as `任务` and `产物`.
- Delete from the UI is a soft-delete request only.
- Do not expose object URI, storage URL, file bytes, or raw storage metadata in
  frontend requests, DOM, or errors.
