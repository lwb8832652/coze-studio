# M4.24 Artifact Scan Jobs Frontend Observability Plan

## Checklist

- [x] Add red service test for `artifact_scan_jobs` URL encoding and filters.
- [x] Add red task-detail test for rendering scan jobs in the artifact drawer.
- [x] Implement `listTaskThreadArtifactScanJobs` with native fetch.
- [x] Load scan jobs when the task artifact drawer opens.
- [x] Render scan job metadata, loading, error, empty, and refresh states.
- [x] Keep the scan-job UI read-only and content-free.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run focused frontend tests and final verification.

## Notes

- The UI intentionally displays recent scan jobs rather than adding mutation
  actions.
- Future retry, quarantine review, and manual release APIs must keep their own
  authorization and audit gates.
