# M4.27 Artifact Scanner Deployment Diagnostics Design

## Scope

M4.27 makes artifact scanner deployment safer to operate by exposing sanitized
configuration diagnostics and documented env defaults. It does not add a
bundled antivirus service, a scanner container, signed links, restore, physical
cleanup, or fail-open behavior.

## Backend Contract

Scanner env construction now has two entry points:

- `NewArtifactContentScannerFromEnv()` keeps the existing scanner-only API.
- `NewArtifactContentScannerFromEnvWithStatus()` returns both the scanner and
  `ArtifactScannerEnvStatus`.

`ArtifactScannerEnvStatus` records only:

- whether scanner env is enabled;
- normalized scanner type;
- whether a scanner was successfully configured;
- a sanitized configuration error.

The status must not include scanner tokens, artifact object URIs, virtual
paths, storage URLs, filenames, file bytes, prompt text, model output, tool
arguments, credentials, checkpoint bytes, or provider raw bodies.

`InitService` stores the status on `ApplicationService.ArtifactScannerStatus`.
Invalid scanner config still leaves `ArtifactScanner` nil so the artifact scan
worker refuses to start.

## Worker Startup

`StartArtifactScanWorkerFromEnvWithStatus` returns both the worker and an
`ArtifactScanWorkerEnvStatus`. The existing `StartArtifactScanWorkerFromEnv`
remains as a compatibility wrapper.

Startup status records whether the worker env was enabled, whether a worker
started, a bounded reason when it did not start, and the scanner config status.
When scanner config failed, startup logs may include the sanitized scanner
error, but not secrets or artifact content.

## Deployment Defaults

`docker/.env.example` and `docker/.env.debug.example` document the scanner env
keys with scanning disabled by default:

- `AGENT_ARTIFACT_SCANNER_TYPE`
- `AGENT_ARTIFACT_SCANNER_HTTP_URL`
- `AGENT_ARTIFACT_SCANNER_HTTP_TOKEN`
- `AGENT_ARTIFACT_SCANNER_HTTP_TIMEOUT_MS`
- `AGENT_ARTIFACT_SCANNER_HTTP_MAX_BYTES`
- `AGENT_ARTIFACT_SCAN_WORKER_*`

Operators must deploy and authorize a trusted scanner service separately before
turning on `AGENT_ARTIFACT_SCANNER_TYPE=http` and
`AGENT_ARTIFACT_SCAN_WORKER_ENABLED=true`.

## Tests

- Invalid HTTP scanner env reports a sanitized status and returns nil scanner.
- Unknown scanner type reports a sanitized status and returns nil scanner.
- `InitService` records scanner status.
- Worker startup status reports scanner config errors without starting.
