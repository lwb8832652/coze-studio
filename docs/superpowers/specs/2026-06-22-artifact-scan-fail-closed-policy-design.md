# M4.23 Artifact Scan Fail-Closed Policy Design

## Scope

M4.23 makes the artifact content-read policy explicit and observable. It does
not add manual release, quarantine review actions, scanner deployment,
fail-open behavior, or frontend dead-letter UI.

## Policy Matrix

Only `scan_status=clean` may read bytes from object storage.

All other states block before object storage access:

- missing or invalid status: `scan_unknown`
- `pending`: `scan_pending`
- `failed`: `scan_failed`
- `blocked`: `scan_blocked`
- `infected`: `scan_infected`
- `quarantined`: `scan_quarantined`

This means legacy or malformed artifact metadata is treated as unsafe until a
real terminal scan decision is recorded.

## Observability

Blocked reads emit `artifact.content.blocked` with
`schema=coze.artifact_access_blocked.v1`. Payload may include scoped IDs,
mode, preview mode, artifact type, normalized scan status, and reason code.
Must not include artifact title, virtual path, object URI, storage URL,
filename, file bytes, scanner raw response, prompt text, model output, tool
arguments, credentials, checkpoint bytes, or provider raw bodies.

## HTTP Behavior

Workbench artifact content reads map typed scan-policy error to HTTP `409`
with generic message and safe reason code. Response must not include object
paths, filenames, file content, or raw scanner diagnostics.

## Tests

- Application policy matrix allows only `clean`.
- Missing scan status blocks before storage access and emits content-free
  blocked event.
- Workbench content endpoint returns `409` and safe reason for blocked scan
  status.
