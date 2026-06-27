# M4.22 Artifact Scan Observability API Design

## Scope

M4.22 adds a read-only Workbench API for inspecting artifact scan jobs. It is
intended for dead-letter and worker troubleshooting after retries are exhausted.
It does not add a frontend UI, manual approval, retry action, quarantine
release, scanner deployment, or artifact metadata mutation.

## API

`GET /api/workbench/task_threads/:thread_id/artifact_scan_jobs`

Supported query filters:

- `run_id`
- `artifact_id`
- `status`
- `scanner`
- `page`
- `page_size`

The API reuses artifact list authorization. If a future authorization policy
needs artifact-level checks, the optional `artifact_id` is passed through the
application authorization request.

## Response Boundary

The response may include only scan job metadata:

- job, thread, run, space, user, artifact, and file IDs;
- scanner, status, worker ID, attempt count, and bounded last error;
- availability, lease, start, end, create, and update timestamps;
- total count.

The response must not include artifact title, virtual path, object URI, storage
URL, filename, file bytes, scanner raw response, prompt text, model output, tool
arguments, credentials, checkpoint bytes, or provider raw bodies.

## Implementation Notes

The repository lists `agent_artifact_scan_jobs` by thread and optional filters,
ordered by `updated_at DESC, id DESC`. The domain layer validates scan job
statuses against `pending`, `processing`, `succeeded`, and `failed`, trims
scanner names, and clamps pagination. The application layer maps domain rows to
safe summaries and the API layer exposes those summaries without joining
artifact rows.

## Tests

- Repository filtering by thread, run, status, scanner, and artifact.
- Domain status validation and pagination normalization.
- Application mapping and authorization-before-repository behavior.
- HTTP handler safe response shape with no object URI, virtual path, or
  filename.
- Router registration.
