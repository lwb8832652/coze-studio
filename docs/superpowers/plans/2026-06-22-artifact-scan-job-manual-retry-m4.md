# M4.25 Artifact Scan Job Manual Retry Plan

## Checklist

- [x] Add red repository test for failed-job requeue CAS.
- [x] Add red domain service test for metadata-safe manual retry.
- [x] Add red application tests for authorization, retry, conflict, and audit.
- [x] Add red Workbench handler tests for safe success and conflict responses.
- [x] Add red frontend tests for retry service and drawer interaction.
- [x] Implement repository, domain, application, handler, route, and frontend
  retry paths.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run focused backend/frontend tests and final verification.

## Notes

- Manual retry is intentionally not a scanner verdict override.
- Quarantine review and manual release remain separate future gates.
