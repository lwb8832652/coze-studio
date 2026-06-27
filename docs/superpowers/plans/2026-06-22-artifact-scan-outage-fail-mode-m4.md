# Artifact Scan Outage Fail-Mode M4 Plan

## Result

M4.29 adds an explicit scanner outage fail-mode while preserving fail-closed as
the default.

## Completed

- Added red tests for default closed policy, `open_non_executable` scope,
  env parsing, InitService wiring, and content-free override audit metadata.
- Added `ArtifactScanReadPolicyConfig` and env parsing for
  `AGENT_ARTIFACT_SCAN_OUTAGE_FAIL_MODE`.
- Updated artifact content read policy to allow only text/image outage
  overrides for `unknown`, `pending`, and `failed`.
- Kept `blocked`, `infected`, `quarantined`, PDF, HTML, SVG, octet-stream,
  download-only, and unsupported artifacts fail-closed.
- Updated Docker env examples, AGENTS, and the master roadmap.

## Verification

- `go test ./application/agentthread -run 'TestArtifactScanReadPolicy|TestApplicationReadArtifactContentAllowsUnscannedTextWithExplicitOutageFailOpen|TestInitServiceRecordsArtifactScannerStatus' -count=1 -gcflags="all=-l -N"`

## Remaining

Signed links, restore, physical cleanup, richer preview transforms,
MIME-specific renderer hardening, and browser E2E coverage remain separate M4
or production-acceptance tasks.
