# M4.21 Artifact Scan Retry Backoff Design

## Scope

M4.21 adds retry/backoff scheduling for failed artifact scan attempts. It builds
on M4.16 leases, M4.17 terminal updates, M4.18 worker execution, M4.19 worker
startup, and M4.20 HTTP scanner adapters.

This slice does not add quarantine review, dead-letter UI, scanner service
deployment, fail-open behavior, public APIs, or a database migration.

## Retry Contract

`RetryArtifactScanJob` is a domain/repository boundary for a claimed processing
job whose scanner attempt failed but still has retry budget.

The retry update requires:

- matching job ID;
- matching worker ID;
- `status=processing`;
- active lease.

On success it sets the job back to `pending`, clears worker and lease fields,
stores bounded error text, sets `available_at` to the next retry time, clears
`ended_at`, and leaves `attempt_count` unchanged. The next successful claim
increments `attempt_count`.

## Application Policy

`ProcessArtifactScanJobs` accepts:

- `max_attempts`;
- `retry_backoff_millis`.

The default `max_attempts` is `1`, so existing behavior remains one attempt and
then final job failure. When `max_attempts > 1`, scanner/storage/result errors
retry while `job.attempt_count < max_attempts`; once the current attempt reaches
the limit, the worker uses the existing final `FailArtifactScanJob` path.

Failure and retry error text is generic and content-free. It must not include
object URI, virtual path, filename, file bytes, raw scanner errors, prompt text,
model output, tool arguments, credentials, checkpoint bytes, or provider raw
bodies.

## Fail-Closed Behavior

Retrying or finally failing a scan job does not mutate artifact scan metadata.
Artifacts normally remain `scan_status=pending`, so content reads continue to
be blocked until a real terminal scan decision is recorded through
`CompleteArtifactScanJob` or review tooling in a future slice.

## Worker Env

The background scan worker reads:

- `AGENT_ARTIFACT_SCAN_WORKER_MAX_ATTEMPTS`
- `AGENT_ARTIFACT_SCAN_WORKER_RETRY_BACKOFF_MS`

Defaults are one attempt and a 60 second backoff. With the default one attempt,
the backoff value has no effect.

## Tests

- Repository retry requeues a claimed job and makes it claimable only after
  `available_at`.
- Domain retry does not mutate artifact metadata.
- Application retry requeues scanner failures before max attempts.
- Application final-fails scanner failures at max attempts.
- Worker env wiring applies max attempts and retry backoff.
