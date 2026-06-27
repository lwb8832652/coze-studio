# M4.20 Artifact HTTP Scanner Adapter Plan

## Checklist

- [x] Add red tests for the HTTP scanner request payload and result mapping.
- [x] Add red tests for rejecting non-terminal scan statuses.
- [x] Add red tests for rejecting oversized content before outbound requests.
- [x] Add red tests for the env scanner factory.
- [x] Implement `HTTPArtifactContentScanner`.
- [x] Implement `NewArtifactContentScannerFromEnv`.
- [x] Wire scanner injection into `InitService`.
- [x] Update AGENTS.md and the master roadmap.
- [x] Run targeted backend tests, Atlas validation, docs scan, and diff checks.

## Notes

- The adapter is opt-in through `AGENT_ARTIFACT_SCANNER_TYPE=http`.
- Do not add a fake clean scanner or silently mark artifacts clean when the
  scanner is missing or misconfigured.
- Do not include object URI, virtual path, filename, prompt text, model output,
  tool arguments, credentials, checkpoint bytes, or provider raw bodies in the
  scanner request, job error text, or audit events.
