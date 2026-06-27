# M4.17 Artifact Scan Job Terminal Design

## Scope

M4.17 adds terminal update boundaries for claimed artifact scan jobs. It does
not execute scanners, read object storage, retry with backoff, quarantine
artifacts, expose public APIs, or add a new table.

The slice connects the existing pieces:

- M4.15 creates pending scan jobs and pending artifact metadata;
- M4.16 lets a worker claim a job with a lease;
- M4.14 records artifact scan decisions.

M4.17 lets a worker finish a claimed job as `succeeded` or `failed`.

## Success Contract

`CompleteArtifactScanJob` accepts `job_id`, `worker_id`, scan status, optional
scanner version, optional reason, and optional scanned time.

The domain service:

1. validates the worker ID and scan status;
2. loads the claimed job;
3. requires `status=processing`, matching worker, and an active lease;
4. writes the artifact scan result through `UpdateArtifactScanResult`;
5. marks the job `succeeded`, clears the lease, clears `last_error`, and sets
   `ended_at`.

This order avoids a job reaching `succeeded` while the artifact still has stale
or pending metadata. If the final terminal update loses the race, the scan
result remains idempotent and the worker can retry completion.

## Failure Contract

`FailArtifactScanJob` accepts `job_id`, `worker_id`, optional error text, and
optional ended time.

Failure marks only the job as `failed`; it does not set artifact metadata to
`clean`, `blocked`, or any other scan result. The artifact remains gated by its
current scan metadata, normally `pending`, until a future retry or review path
records a terminal scan decision.

## Repository Contract

The repository exposes:

- `GetArtifactScanJob`
- `CompleteArtifactScanJob`
- `FailArtifactScanJob`

Terminal updates require `id`, matching `worker_id`, `status=processing`, and
`lease_expires_at > now`. If those checks fail, the method returns
`updated=false`.

Terminal job rows may include scoped IDs, scanner, worker ID, status, attempt
count, bounded error text, and timestamps. They must not include object URI,
virtual path, storage URL, title, filename, file bytes, prompt text, model
output, tool arguments, credentials, checkpoint bytes, or provider raw bodies.

## Tests

- Repository completes only a claimed job with a matching worker and active
  lease.
- Repository fails only a claimed job with a matching worker and active lease,
  bounding stored error text.
- Domain service completion writes artifact scan metadata before marking the
  job succeeded.
- Domain service failure does not mutate artifact scan metadata.
