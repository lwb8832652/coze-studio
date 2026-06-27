# M4.26 Artifact Scan Review Design

## Scope

M4.26 adds a narrow human review boundary for artifact scan status. It lets an
authorized task owner release, quarantine, or block an artifact by updating the
existing scan metadata. It does not read artifact bytes, deploy a scanner,
restore deleted artifacts, create signed links, perform physical cleanup, or
add fail-open behavior.

## Backend Contract

Endpoint:

- `POST /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/scan_review`

Request body:

- `decision`: one of `release`, `quarantine`, or `block`.
- `reason`: optional bounded operator note. It is stored in artifact metadata
  through the existing scan-result path but is not emitted in audit events.

Decision mapping:

- `release` writes `scan_status=clean`.
- `quarantine` writes `scan_status=quarantined`.
- `block` writes `scan_status=blocked`.
- `scan_scanner` is `manual_review`.

Behavior:

- Authorize through `ArtifactAccessOperationReview`.
- Update only the scoped `thread_id + artifact_id` artifact metadata.
- Return only the review receipt: artifact ID, decision, final scan status,
  and reviewed flag.
- Return invalid-param errors for unsupported decisions and missing artifacts.
- Keep content access fail-closed: bytes are still readable only through the
  existing content endpoint after the final metadata state is `clean`.

## Audit

Successful review emits `artifact.scan.reviewed` with
`schema=coze.artifact_scan_review.v1`. Payload may include thread, run,
artifact, and file IDs, artifact type, content type, size, decision, final scan
status, scanner, scanner version, scanned time, and reviewed time. It must not
include the human reason, artifact title, virtual path, object URI, storage URL,
filename, file bytes, scanner raw response, prompt text, model output, tool
arguments, credentials, checkpoint bytes, or provider raw bodies.

## Frontend

The task-detail artifact drawer parses safe scan metadata from artifact
metadata and shows the scan status tag. For non-clean artifacts it exposes
row-level review actions:

- `放行` sends `release`;
- `隔离` sends `quarantine`;
- `阻断` sends `block`, except when the artifact is already blocked.

Each action uses a row-level loading state, keeps errors inside the drawer, and
refreshes the artifact list after success.

## Tests

- Application service authorizes review, updates scan metadata, and emits
  content-free audit.
- Application service rejects unsupported decisions before mutation.
- Workbench handler releases a blocked artifact and only then allows the
  existing content endpoint to read bytes.
- Route registration includes the review endpoint.
- Frontend service encodes route params and sends JSON.
- Task-detail drawer calls review and refreshes artifact metadata.
