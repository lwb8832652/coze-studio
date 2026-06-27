# M4.9 Artifact Scan Gate And Audit Plan

## Checklist

- [x] Add red application tests for successful content-free audit events.
- [x] Add red application tests proving unsafe scan status blocks before
      storage reads.
- [x] Add metadata scan-status normalization helpers.
- [x] Add scan gate to `ReadArtifactContent`.
- [x] Add optional artifact access auditor wiring through the existing run
      event stream.
- [x] Update AGENTS.md and the DeerFlow parity roadmap.
- [x] Run targeted backend tests and diff checks.

## Notes

- Do not add a database migration in this slice.
- Do not add a scanner worker or external antivirus dependency here.
- Keep missing scan status compatible for existing artifacts.
- Keep event payloads content-free and omit object URI, virtual path, filename,
  file bytes, prompt text, model output, tool arguments, and checkpoint data.
