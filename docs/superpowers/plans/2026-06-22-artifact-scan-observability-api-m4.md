# M4.22 Artifact Scan Observability API Plan

## Checklist

- [x] Add red repository test for filtered scan job listing.
- [x] Add red domain tests for filter mapping and unknown status rejection.
- [x] Add red application tests for summary mapping and authorization.
- [x] Add red HTTP handler and router tests for the Workbench API.
- [x] Implement repository/domain/application/API read-only listing.
- [x] Keep response payload content-free and path-free.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests and final verification.

## Notes

- No database migration is expected in this slice; it reads the existing
  `agent_artifact_scan_jobs` table.
- The endpoint is observability-only. Do not add retry, release, quarantine, or
  artifact scan metadata mutation here.
