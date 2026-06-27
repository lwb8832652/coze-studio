# M4.23 Artifact Scan Fail-Closed Policy Plan

## Checklist

- [x] Add red application test for scan read policy matrix.
- [x] Add red application test for unknown scan status blocking and audit.
- [x] Add red handler test for HTTP `409` on scan-policy block.
- [x] Implement explicit scan read policy helper.
- [x] Emit content-free `artifact.content.blocked` events.
- [x] Add typed scan-policy error and Workbench `409` mapping.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run focused backend tests and final verification.

## Notes

- Missing or malformed artifact scan metadata is now fail-closed.
- This slice does not add manual release, quarantine review, or retry action
  APIs.
