# M4.19 Artifact Scan Worker Env Design

## Scope

M4.19 adds a disabled-by-default background worker wrapper for artifact scan
jobs. It uses the M4.18 `ProcessArtifactScanJobs` application shell and makes
that shell reachable from production startup when explicitly enabled by
environment variables.

This slice does not add an antivirus engine, scanner adapter, retry/backoff,
quarantine review, fail-closed outage policy, public API, or database
migration.

## Worker Contract

`ArtifactScanWorker` owns only polling configuration:

- scanner name;
- worker ID;
- batch size;
- lease TTL;
- polling interval.

`RunOnce` calls `ProcessArtifactScanJobs` and maps the response to worker
counts: claimed, succeeded, failed, skipped, and errored. It logs only counts,
scanner name, worker ID, and bounded error text. It must not log artifact
content, object URI, virtual path, filename, scanner reason, checkpoint bytes,
credentials, or provider raw bodies.

## Env Startup Contract

`StartArtifactScanWorkerFromEnv` is disabled unless
`AGENT_ARTIFACT_SCAN_WORKER_ENABLED=true`.

When enabled, it refuses to start unless the application service has:

- `ArtifactSVC`;
- `ArtifactObjectStorage`;
- `ArtifactScanner`.

This prevents pending artifacts from being marked clean by a fake default
scanner. The startup path may warn when configuration is incomplete, but it
must not silently install a permissive scanner or fall back to client-provided
scan state.

Supported env keys:

- `AGENT_ARTIFACT_SCAN_WORKER_ENABLED`
- `AGENT_ARTIFACT_SCAN_WORKER_ID`
- `AGENT_ARTIFACT_SCAN_WORKER_SCANNER`
- `AGENT_ARTIFACT_SCAN_WORKER_BATCH_SIZE`
- `AGENT_ARTIFACT_SCAN_WORKER_INTERVAL_MS`
- `AGENT_ARTIFACT_SCAN_WORKER_LEASE_TTL_MS`

## Application Startup

The main application startup calls `StartArtifactScanWorkerFromEnv` alongside
the existing run and resume workers. Because the scan worker is disabled by
default, existing deployments do not start it until an operator provides a real
scanner adapter and enables the env flag.

## Tests

- `RunOnce` delegates to `ProcessArtifactScanJobs` with scanner, worker ID,
  batch size, and lease TTL.
- Env startup is disabled by default.
- Env startup refuses to start when the scanner is missing.
- Env startup applies configured worker ID, scanner, batch size, interval, and
  lease TTL.
