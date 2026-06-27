# M4.17 Artifact Scan Job Terminal Plan

## Checklist

- [x] Add red repository tests for complete/fail active claimed scan jobs.
- [x] Add red domain service tests for completion scan-result consistency and
  failure no artifact mutation.
- [x] Extend repository contract with scan job get, complete, and fail methods.
- [x] Implement active-lease terminal updates.
- [x] Add domain `CompleteArtifactScanJob` and `FailArtifactScanJob` methods.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests, Atlas validation, docs scan, and diff checks.

## Notes

- No database migration is expected in this slice.
- Do not execute scanners, read object storage, add quarantine UI, or expose
  public APIs here.
- Terminal job errors must be bounded and must not include object URI, virtual
  path, filename, file bytes, prompt text, model output, tool arguments,
  credentials, or checkpoint bytes.
