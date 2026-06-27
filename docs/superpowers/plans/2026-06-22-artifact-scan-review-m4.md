# M4.26 Artifact Scan Review Plan

## Checklist

- [x] Add red application tests for authorization, decision mapping, metadata
  update, and content-free audit.
- [x] Add red Workbench handler and route tests for the review endpoint.
- [x] Add red frontend service and drawer tests for review actions.
- [x] Implement review DTOs, application service, audit event, handler, route,
  and frontend controls.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run focused backend/frontend tests and final verification.

## Notes

- Manual release is a metadata verdict override, not direct object access.
- Content bytes remain behind the existing `scan_status=clean` read policy.
- Human review reason is stored in artifact metadata but omitted from events.
- Scanner deployment, signed links, restore, physical cleanup, richer preview
  transforms, MIME-specific renderer hardening, browser E2E, and fail-open
  policy remain separate future gates.
