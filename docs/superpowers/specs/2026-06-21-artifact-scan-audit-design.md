# M4.9 Artifact Scan Gate And Audit Design

## Scope

M4.9 hardens the artifact content read path added in M4.7. The first slice does
not add a scanner worker, a quarantine table, signed URLs, or a new audit API.
It adds two production boundaries where the current code already owns the
decision:

- a metadata-backed scan status gate before object storage is read;
- a content-free run event when artifact bytes are successfully served.

The contract stays task-oriented and keeps the existing artifact list and
content endpoints.

## Scan Status Contract

Artifact scan state is read from artifact metadata for this slice. Supported
metadata keys are `scan_status` and `scanStatus`. The normalized values are:

- `clean`: content read is allowed.
- `unknown`: content read is allowed for compatibility with existing artifacts.
- `pending`, `failed`, `blocked`, `infected`, `quarantined`: content read is
  denied before object storage is touched.

Unknown or missing metadata is intentionally compatible in M4.9 because
historical artifacts do not yet have scan records. Later scanner work can
tighten the default through a dedicated policy once every artifact has a scan
decision and scanner outage mode.

## Audit Contract

Successful content reads emit `artifact.content.accessed` through the existing
run event stream when an event sink is configured. The event is metadata-only.
It may include:

- `schema`: `coze.artifact_access.v1`
- `thread_id`, `run_id`, `artifact_id`, `file_id`
- `mode`, `preview_mode`, `artifact_type`
- `content_type`, `size_bytes`, `attachment`
- `scan_status`

It must not include file bytes, object URIs, virtual paths, storage URLs,
filenames, user prompt text, model output, tool arguments, checkpoint bytes, or
raw provider payloads.

## Data Flow

1. The handler binds `thread_id`, `artifact_id`, and `mode` as before.
2. The application service fetches the artifact by `thread_id + artifact_id`.
3. The application service normalizes metadata scan status.
4. Unsafe explicit scan statuses fail before object storage reads.
5. The application service reads the service-owned object URI.
6. The application service computes content type and attachment disposition.
7. If the artifact belongs to a run and an auditor is configured, the service
   emits a content-free `artifact.content.accessed` event.
8. The handler writes artifact bytes with the existing safe response headers.

## Error Handling

- Missing artifact service, object storage, request scope, artifact row, or
  object URI keeps the existing errors.
- Unsafe scan status returns an application error and does not call storage.
- Audit emission is best-effort when the auditor is not configured. When an
  auditor is configured and returns an error, the read returns that error so
  production wiring cannot silently bypass durable audit.

## Tests

- Application: clean artifact content read emits `artifact.content.accessed`
  with content-free payload fields and no object URI, virtual path, filename,
  or bytes.
- Application: explicit unsafe scan statuses block before object storage is
  read.
- Application: missing scan metadata remains compatible and audits
  `scan_status=unknown`.
