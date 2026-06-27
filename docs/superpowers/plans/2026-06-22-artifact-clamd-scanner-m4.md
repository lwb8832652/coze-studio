# Artifact Clamd Scanner M4 Plan

## Result

M4.28 adds a concrete opt-in clamd scanner path for artifact scanning.

## Completed

- Added red tests for clamd scan clean/infected parsing and env construction.
- Implemented `NewClamdArtifactContentScanner` using clamd `INSTREAM`.
- Extended env scanner selection to `clamd` and `clamav`.
- Added clamd env keys to Docker env examples with scanning disabled by
  default.
- Added optional `artifact-scanner` services to MySQL and OceanBase compose
  variants, including debug variants.
- Updated AGENTS and the master DeerFlow parity roadmap.

## Verification

- `go test ./application/agentthread -run 'TestClamdArtifactContentScanner|TestArtifactContentScannerFromEnvBuildsClamdScanner|TestArtifactContentScannerFromEnvReportsUnknownType' -count=1 -gcflags="all=-l -N"`
- `go test ./application/agentthread -count=1 -gcflags="all=-l -N"`
- `docker compose ... --profile scanner config` for MySQL, MySQL debug,
  OceanBase, and OceanBase debug compose variants.
- `docker compose -f docker-compose.yml config` with temporary `.env` confirms
  the default profile does not include the `artifact-scanner` service.
- `atlas migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

Signed links, restore, physical cleanup, richer preview transforms,
MIME-specific renderer hardening, browser E2E coverage, and fail-open policy
remain separate M4 or production-acceptance tasks.
