# M4.18 Artifact Scan Worker Shell Plan

## Checklist

- [x] Add red application tests for successful claimed-job scanning.
- [x] Add red application tests for scanner failure without content leakage.
- [x] Add red application test for missing scanner configuration.
- [x] Add `ArtifactContentScanner`, scan request/result DTOs, and scanner
  injection on `ApplicationService`.
- [x] Add `ProcessArtifactScanJobs` to claim jobs, read artifact content,
  invoke the scanner, and complete or fail jobs through the domain boundary.
- [x] Emit the existing content-free scan-completed audit event after a
  successful terminal update.
- [x] Keep scan failure text generic and content-free.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests, Atlas validation, docs scan, and diff checks.

## Notes

- No database migration is expected in this slice.
- Do not add a daemon, scheduler, public API, external antivirus dependency,
  quarantine review flow, retry/backoff policy, or fail-closed outage policy
  here.
- Scanner adapters must stay behind `ArtifactContentScanner` and must not
  expose object URI, virtual path, filename, file bytes, prompt text, model
  output, tool arguments, credentials, checkpoint bytes, or provider raw bodies
  in job errors or run events.
