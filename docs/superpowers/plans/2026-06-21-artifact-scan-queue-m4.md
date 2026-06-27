# M4.15 Artifact Scan Queue Plan

## Checklist

- [x] Add red repository tests for idempotent scan job creation.
- [x] Add red domain service tests for registration pending metadata and scan
  job enqueue.
- [x] Add artifact scan job entity, repository contract, PO, mapper, and table
  name.
- [x] Add `agent_artifact_scan_jobs` Atlas migration and HCL schema.
- [x] Wire `RegisterArtifact` to force pending scan metadata and enqueue a
  pending scan job.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests, Atlas hash/validation, docs scan, and diff
  checks.

## Notes

- Do not add scanner worker execution, external antivirus dependencies,
  quarantine review UI, or public APIs in this slice.
- New artifact rows must not trust caller-provided `clean` scan metadata.
- Scan jobs must not persist object URI, virtual path, title, filename, file
  bytes, prompt text, model output, tool arguments, credentials, or checkpoint
  bytes.
