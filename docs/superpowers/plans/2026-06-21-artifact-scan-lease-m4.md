# M4.16 Artifact Scan Job Lease Plan

## Checklist

- [x] Add red repository tests for due pending claim and expired lease reclaim.
- [x] Add red domain service tests for worker/scanner/limit/lease validation.
- [x] Extend scan job entity, repository request, PO, and mapper with lease
  fields.
- [x] Implement repository claim logic with row locking where available.
- [x] Add domain `ClaimArtifactScanJobs` validation and defaults.
- [x] Add Atlas migration and HCL schema updates for lease fields/indexes.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests, Atlas hash/validation, docs scan, and diff
  checks.

## Notes

- Do not execute scanners, read object storage, write scan results, or add
  quarantine UI in this slice.
- Claim rows must not contain object URI, virtual path, title, filename, file
  bytes, prompt text, model output, tool arguments, credentials, or checkpoint
  bytes.
