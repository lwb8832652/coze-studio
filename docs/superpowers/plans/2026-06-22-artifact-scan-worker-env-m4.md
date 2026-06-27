# M4.19 Artifact Scan Worker Env Plan

## Checklist

- [x] Add red worker tests for `RunOnce` delegation and result counts.
- [x] Add red env startup tests for disabled default and missing scanner.
- [x] Add red env startup test for configured worker fields.
- [x] Implement `ArtifactScanWorker`, options, result counts, `Start`, and
  `RunOnce`.
- [x] Add env-gated `StartArtifactScanWorkerFromEnv`.
- [x] Wire the scan worker startup into application initialization.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests, Atlas validation, docs scan, and diff checks.

## Notes

- The worker is disabled by default.
- The worker must not start without a configured `ArtifactContentScanner`.
- Do not add a fake clean scanner, public API, external antivirus dependency,
  retry/backoff, quarantine review, or fail-closed outage policy in this slice.
