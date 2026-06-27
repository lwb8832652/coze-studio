# M4.20 Artifact HTTP Scanner Adapter Design

## Scope

M4.20 adds the first concrete `ArtifactContentScanner` adapter. The adapter
calls an operator-configured HTTP scanning service and returns the normalized
scan decision to the M4.18 worker shell.

This slice does not add a bundled antivirus engine, scanner service
deployment, retry/backoff, quarantine review, fail-closed outage policy, public
API, or database migration.

## Configuration

The adapter is disabled unless `AGENT_ARTIFACT_SCANNER_TYPE=http`.

Supported env keys:

- `AGENT_ARTIFACT_SCANNER_TYPE`
- `AGENT_ARTIFACT_SCANNER_HTTP_URL`
- `AGENT_ARTIFACT_SCANNER_HTTP_TOKEN`
- `AGENT_ARTIFACT_SCANNER_HTTP_TIMEOUT_MS`
- `AGENT_ARTIFACT_SCANNER_HTTP_MAX_BYTES`

`InitService` reads these env vars through `NewArtifactContentScannerFromEnv`.
If the scanner type is missing, unknown, or invalidly configured, no scanner is
injected. The M4.19 worker still refuses to start without an injected scanner.

## HTTP Protocol

The adapter sends a POST JSON payload with schema
`coze.artifact_scan_request.v1`.

Request fields:

- scanner name;
- space, thread, run, user, artifact, and file IDs;
- content type;
- size in bytes;
- base64 artifact content.

The request must not include object URI, virtual path, storage URL, title,
filename, prompt text, model output, tool arguments, credentials, checkpoint
bytes, or provider raw bodies.

If `AGENT_ARTIFACT_SCANNER_HTTP_TOKEN` is set, the adapter sends it as a bearer
token. Endpoint URLs must use `http` or `https` and must not contain userinfo.

## Response Contract

The scanner response may return only terminal scan statuses:

- `clean`
- `blocked`
- `infected`
- `quarantined`

`pending` and `failed` are rejected as scanner decisions. Scanner outages,
invalid responses, non-2xx responses, oversized content, and invalid statuses
return scanner errors so the worker marks the scan job failed without mutating
artifact scan metadata.

Scanner version and reason may be returned and are bounded by the domain scan
metadata path before persistence. Audit events still omit scanner reason and
raw scanner errors.

## Tests

- The HTTP adapter posts scoped IDs, content type, size, and base64 content,
  sends the bearer token, and does not include storage/path/name fields.
- The adapter rejects non-terminal statuses.
- The adapter rejects oversized content before issuing an HTTP request.
- The env factory builds an HTTP scanner only when explicitly configured.
