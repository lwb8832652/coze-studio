# M4.14 Artifact Scan Result Plan

## Checklist

- [x] Add red repository tests for active-only scan metadata updates.
- [x] Add red domain service tests for scan status validation and metadata
  preservation.
- [x] Add red application tests for scan-result audit events.
- [x] Extend artifact repository and domain service contracts.
- [x] Implement metadata merge and active-row update.
- [x] Add application `RecordArtifactScanResult` and content-free audit event.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests and diff checks.

## Notes

- No database migration is expected in this slice.
- Do not add a scanner worker, queue, external antivirus dependency, or
  quarantine review UI.
- Keep audit payloads content-free and omit object URI, virtual path, title,
  filename, file bytes, prompt text, model output, tool arguments, credentials,
  and checkpoint data.
