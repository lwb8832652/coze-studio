# M4.14 Artifact Scan Result Design

## Scope

M4.14 adds the durable write boundary for artifact scan decisions. It does not
introduce a scanner worker, antivirus dependency, queue, quarantine review UI,
new table, or public API. The goal is to let a future scanner or operator path
record a bounded scan decision on an existing active artifact, then emit a
content-free audit event through the existing run event stream.

This slice keeps product wording as `任务` and reuses the artifact metadata scan
contract consumed by the M4.9 read gate.

## Contract

The domain artifact service exposes `UpdateArtifactScanResult`. The request is
scoped by `thread_id + artifact_id` and accepts:

- `scan_status`: one of `clean`, `pending`, `failed`, `blocked`, `infected`,
  or `quarantined`;
- `scanner`: optional scanner name, trimmed and bounded;
- `scanner_version`: optional scanner version, trimmed and bounded;
- `reason`: optional human-readable reason, trimmed and bounded;
- `scanned_at`: optional millisecond timestamp, defaulted by the service.

The repository updates only active artifact rows. Deleted artifacts are left
unchanged and return a no-op result. Existing artifact metadata is preserved,
while scan fields are overwritten with normalized keys:

- `scan_status`
- `scan_scanner`
- `scan_scanner_version`
- `scan_reason`
- `scan_scanned_at`

Invalid existing metadata is treated as an empty object for the purpose of this
controlled update. Object URI, virtual path, title, filename, and file bytes are
never copied into scan metadata.

## Application Boundary

The application service exposes `RecordArtifactScanResult` as the scanner-facing
boundary. It delegates persistence to the domain service and emits
`artifact.scan.completed` when an active artifact is updated and has a run ID.

The audit payload schema is `coze.artifact_scan.v1`. It may include only:

- `thread_id`, `run_id`, `artifact_id`, `file_id`
- `artifact_type`, `content_type`, `size_bytes`
- `scan_status`, `scanner`, `scanner_version`, `scanned_at`

It must not include object URI, virtual path, filename/title, file bytes, prompt
text, model output, tool arguments, checkpoint bytes, credentials, or raw
provider payloads.

## Error Handling

- Missing service wiring, nil request, invalid scope, or invalid scan status
  returns an application/domain error.
- Missing or deleted artifacts return `Updated=false` and do not emit audit.
- Audit emission is required when the application has a thread service and a
  persisted artifact with a run ID. Audit failure returns an error so production
  wiring cannot silently bypass security audit.

## Tests

- Domain service preserves existing metadata while writing normalized scan
  fields.
- Repository updates only active rows and returns no-op for deleted or missing
  artifacts.
- Application emits a content-free `artifact.scan.completed` audit event after
  a successful update.
- Invalid scan status is rejected before repository writes.
