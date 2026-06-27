# Artifact Clamd Scanner Design

## Goal

Provide a production-shaped, opt-in artifact scanner deployment path without
adding a Python sidecar or a permissive fallback. Coze remains the control
plane and scan decision owner; clamd is only a concrete scanning backend.

## Scope

- Add a Go `clamd` artifact scanner adapter that speaks clamd `INSTREAM`.
- Support env selection through `AGENT_ARTIFACT_SCANNER_TYPE=clamd` or
  `clamav`.
- Add optional Docker compose scanner services behind a `scanner` profile.
- Keep artifact scanning disabled by default.

## Runtime Behavior

The clamd adapter connects to `AGENT_ARTIFACT_SCANNER_CLAMD_ADDR`, streams
artifact bytes in bounded chunks, and applies timeout and max-byte limits from
env. `OK` maps to `clean`; `FOUND` maps to `infected` and stores only the
signature as the bounded scan reason. Connection failures, clamd `ERROR`
responses, malformed responses, and oversized content are scanner errors and
preserve fail-closed content access.

## Deployment

Docker compose files define `artifact-scanner` under the `scanner` profile.
Normal Docker env points Coze to `artifact-scanner:3310`; debug env points a
local Go server to `127.0.0.1:3310`. Operators must explicitly enable both
`AGENT_ARTIFACT_SCANNER_TYPE=clamd` and
`AGENT_ARTIFACT_SCAN_WORKER_ENABLED=true`.

## Safety

The adapter and deployment status must not expose object URIs, virtual paths,
storage URLs, filenames, file bytes, raw scanner responses, credentials,
tokens, prompt text, model output, tool arguments, checkpoint bytes, or
provider raw bodies in logs, events, APIs, or UI. There is no clean fallback.
