# M4.21 Artifact Scan Retry Backoff Plan

## Checklist

- [x] Add red repository test for active-lease retry and delayed re-claim.
- [x] Add red domain test for retry without artifact metadata mutation.
- [x] Add red application tests for retry before max attempts and final failure
  at max attempts.
- [x] Add worker env assertions for max attempts and retry backoff.
- [x] Implement repository `RetryArtifactScanJob`.
- [x] Implement domain `RetryArtifactScanJob`.
- [x] Extend `ProcessArtifactScanJobs` with max attempts, retry backoff, and
  retried counts.
- [x] Extend scan worker options/env/result counts.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests, Atlas validation, docs scan, and diff checks.

## Notes

- No database migration is expected in this slice.
- Retry keeps artifact metadata unchanged, so reads stay fail-closed while scan
  status is pending.
- Do not store raw scanner errors, object URI, virtual path, filename, artifact
  bytes, prompt text, model output, tool arguments, credentials, checkpoint
  bytes, or provider raw bodies in retry/failure text.
