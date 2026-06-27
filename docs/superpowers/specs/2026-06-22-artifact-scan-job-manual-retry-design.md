# M4.25 Artifact Scan Job Manual Retry Design

## Scope

M4.25 adds a narrow manual retry action for failed artifact scan jobs. It does
not add manual release, quarantine review, restore, scanner deployment, signed
links, physical cleanup, or fail-open behavior.

## Backend Contract

Endpoint:

- `POST /api/workbench/task_threads/:thread_id/artifact_scan_jobs/:job_id/retry`

Behavior:

- Authorize with the same thread-level artifact access boundary used by scan
  job observability.
- Requeue only jobs matching `id + thread_id + status=failed`.
- Set `status=pending`.
- Clear worker ID, lease expiry, started time, and ended time.
- Preserve `attempt_count`.
- Set `available_at` and `updated_at` to the retry time.
- Set bounded `last_error` to `manual retry requested`.
- Do not mutate artifact metadata or record a clean scan status.
- Return HTTP `409` with a generic message when the job is not retryable.

## Audit

Successful retry emits `artifact.scan_job.retry_requested` with
`schema=coze.artifact_scan_job_retry_requested.v1`. Payload may include job,
thread, run, artifact, and file IDs, scanner, status, attempt count, and
availability time. It must not include artifact title, virtual path, object
URI, storage URL, filename, file bytes, scanner raw response, prompt text,
model output, tool arguments, credentials, checkpoint bytes, or provider raw
bodies.

## Frontend

The task-detail artifact drawer shows a `重试` action only for failed scan jobs.
Clicking it calls the retry endpoint, uses a row-level loading state, keeps
errors inside the drawer, and refreshes the scan queue after success.

## Tests

- Repository CAS requeues failed jobs only.
- Domain service does not mutate artifact metadata.
- Application service authorizes, requeues, maps non-retryable jobs to
  conflict, and emits content-free audit.
- Workbench handler returns safe `200` and `409` responses.
- Frontend service encodes route params and the drawer refreshes after retry.
