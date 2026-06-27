# M4.15 Artifact Scan Queue Design

## Scope

M4.15 adds the durable queue foundation for artifact scanning. It does not add
a scanner worker loop, external antivirus dependency, public API, quarantine
review UI, or fail-closed outage policy. The goal is narrower:

- new artifact registration stores scan metadata as `pending`;
- registration creates an idempotent pending scan job;
- M4.9 read gates block new artifacts until M4.14 records a terminal scan
  result.

Existing artifacts without scan metadata remain read-compatible through the
M4.9 `unknown` path. New artifact registration must not accept caller-provided
`clean` metadata as proof of safety.

## Data Model

Add `agent_artifact_scan_jobs`:

- `id`
- `thread_id`, `run_id`, `space_id`, `user_id`
- `artifact_id`, `file_id`
- `scanner`
- `idempotency_key`
- `status`
- `attempt_count`
- `last_error`
- `available_at`, `created_at`, `updated_at`

The unique key is `(artifact_id, idempotency_key)` so repeated registration or
retry-safe enqueue calls return the first job. The pending worker index is
`(status, available_at, created_at)`.

Job statuses are `pending`, `processing`, `succeeded`, and `failed`. M4.15 only
creates `pending` rows.

## Registration Flow

`RegisterArtifact` continues to validate the backing runtime file and upsert the
artifact registry row. Before upsert, it merges metadata with these scan fields:

- `scan_status=pending`
- `scan_scanner=<scanner name>`
- `scan_requested_at=<now>`

The default scanner name is `default`. Future runtime configuration may choose a
tenant-specific scanner, but this slice keeps it static and deterministic.

After upsert, the service creates or gets a scan job using idempotency key
`artifact_scan:{artifact_id}:{scanner}`. If the artifact row already exists and
the job already exists, registration is idempotent.

## Security Notes

The scan job row may store scoped IDs, scanner name, status, attempt counts,
timestamps, and bounded error text in the future. It must not store object URI,
virtual path, title, filename, file bytes, prompt text, model output, tool
arguments, checkpoint bytes, credentials, or provider raw payloads.

## Tests

- Repository creates or gets scan jobs idempotently.
- Domain service registration writes `pending` scan metadata even if caller
  metadata claims `clean`.
- Domain service registration creates a pending scan job with scoped IDs and no
  object URI or path data.
- Existing artifact registration remains idempotent.
