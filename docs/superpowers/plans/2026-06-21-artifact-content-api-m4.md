# M4.7 Artifact Content API Plan

## Checklist

- [x] Add red repository tests for scoped artifact lookup.
- [x] Add red application tests for content read and active-content attachment.
- [x] Add red handler and router tests for the content endpoint.
- [x] Implement artifact repository and domain service lookup.
- [x] Implement application content service with storage injection.
- [x] Implement Workbench content endpoint and response headers.
- [x] Update AGENTS.md and master roadmap.
- [x] Run targeted backend tests and diff checks.

## Notes

- Keep object storage keys server-side only.
- Prefer a narrow application storage interface with `GetObject` to avoid spreading the full storage contract.
- No frontend UI is included in this slice; frontend can consume this endpoint in a later artifact drawer task.
