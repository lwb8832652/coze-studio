# Artifact Scan Outage Fail-Mode Design

## Goal

Make scanner outage behavior explicit without weakening the default artifact
read gate. The default remains fail-closed: only `scan_status=clean` may read
bytes.

## Configuration

`AGENT_ARTIFACT_SCAN_OUTAGE_FAIL_MODE` supports:

- `closed`: default behavior.
- `open_non_executable`: allow a narrow outage override.

Unset or invalid values normalize to `closed`.

## Override Rules

`open_non_executable` may override only outage-like scan states:
`unknown`, `pending`, and `failed`.

The artifact must be classified as non-executable by both stored preview mode
and stored content type. Only text and raster image artifacts qualify. PDF,
HTML, XHTML, SVG, octet-stream, download-only, unsupported, blocked,
infected, and quarantined artifacts remain blocked.

## Observability

Allowed override reads continue to emit `artifact.content.accessed`. The event
may include `scan_policy_mode`, `scan_policy_reason`, and
`scan_policy_override`. It must not include artifact title, virtual path,
object URI, storage URL, filename, file bytes, scanner raw response, prompt
text, model output, tool arguments, credentials, checkpoint bytes, or provider
raw bodies.
