# M4.11 Artifact Delete Lifecycle Plan

## Checklist

- [x] Add `deleted_at` migration and HCL schema update.
- [x] Add red repository tests for soft delete and active filtering.
- [x] Add red domain tests for scoped delete delegation.
- [x] Add red application tests for content-free delete audit.
- [x] Add red handler and router tests for the Workbench DELETE endpoint.
- [x] Implement repository/entity/domain/application delete path.
- [x] Implement Workbench handler, request/response models, and route.
- [x] Regenerate Atlas checksum with pinned Atlas v0.35.0.
- [x] Update AGENTS.md and the DeerFlow parity roadmap.
- [x] Run targeted backend tests and diff checks.

## Notes

- Do not remove `agent_files` rows or object storage keys in this slice.
- Keep normal list/get/content reads active-only.
- Keep audit event payloads content-free and do not include object URI,
  virtual path, storage URL, filename, file bytes, prompt/model/tool content,
  credentials, checkpoint bytes, or provider raw payloads.
