# M4.16 Artifact Scan Job Lease Design

## Scope

M4.16 makes artifact scan jobs claimable by background workers. It does not run
a scanner, read object storage, write scan results, retry with backoff,
quarantine artifacts, or expose a public API. The slice only adds durable lease
metadata and a claim boundary.

This follows the existing run-claim pattern while adding an explicit lease so a
worker crash does not leave a scan job permanently stuck in `processing`.

## Schema

Extend `agent_artifact_scan_jobs` with:

- `worker_id`
- `lease_expires_at`
- `started_at`
- `ended_at`

The pending index becomes:

- `(status, scanner, available_at, created_at)`

Add a lease index:

- `(status, scanner, lease_expires_at, created_at)`

M4.16 only writes `pending` to `processing`. `succeeded` and `failed` terminal
updates are follow-up work.

## Claim Contract

`ClaimArtifactScanJobs` accepts `scanner`, `worker_id`, `limit`, and
`lease_ttl_ms`.

The domain service:

- requires a non-empty worker ID;
- defaults scanner to `default`;
- defaults limit to `10`;
- defaults lease TTL to `300000` milliseconds;
- trims bounded text fields before passing to the repository.

The repository claims:

- pending jobs for the scanner whose `available_at <= now`;
- processing jobs for the scanner whose `lease_expires_at <= now`.

Claimed rows are marked `processing`, assigned to the worker, receive a new
`lease_expires_at`, set `started_at` when first claimed, increment
`attempt_count`, and update `updated_at`. Rows are selected oldest first by
availability, creation time, and ID.

## Security Notes

Claim payloads and job rows may include worker ID, scanner name, scoped IDs,
status, attempt counts, lease timestamps, and bounded errors. They must not
include object URI, virtual path, storage URL, title, filename, file bytes,
prompt text, model output, tool arguments, credentials, checkpoint bytes, or
provider raw bodies.

## Tests

- Repository claims the oldest due pending jobs for a scanner and marks them
  `processing`.
- Repository reclaims expired `processing` jobs but skips unexpired leases.
- Domain service trims worker/scanner, normalizes limit and lease TTL, and
  rejects missing worker IDs.
