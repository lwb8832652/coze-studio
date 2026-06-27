# M4.13 Artifact Authorization Plan

## Checklist

- [x] Add red application tests for artifact authorization denial and request
  metadata.
- [x] Add red handler tests for viewer propagation and 403 mapping.
- [x] Add `ViewerID` to artifact application requests.
- [x] Add `ArtifactAuthorizer`, access request, operations, and
  `ErrArtifactAccessDenied`.
- [x] Implement and wire `ThreadOwnerArtifactAuthorizer` in production init.
- [x] Pass viewer ID from Workbench handlers into list/content/delete.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests and diff checks.

## Notes

- Keep product wording as `任务`.
- Do not expose object URI, virtual path, storage URL, filename, file bytes, or
  raw provider content in authorization errors.
- No database migration is expected in this slice.
