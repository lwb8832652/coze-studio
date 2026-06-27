# M4.27 Artifact Scanner Deployment Diagnostics Plan

## Checklist

- [x] Add red tests for scanner env diagnostic status.
- [x] Add red test for `InitService` recording scanner status.
- [x] Add red test for worker startup status when scanner config is invalid.
- [x] Implement scanner env status, service storage, and worker startup status.
- [x] Document scanner env defaults in Docker env examples.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run focused backend tests and final verification.

## Notes

- This slice improves scanner deployment visibility only.
- It deliberately keeps fail-closed behavior: invalid scanner config yields no
  scanner and the worker does not start.
- Bundled antivirus/scanner service deployment remains a separate production
  integration task.
