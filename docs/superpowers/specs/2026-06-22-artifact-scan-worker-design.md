# M4.18 Artifact Scan Worker Shell Design

## Scope

M4.18 adds the application-level execution shell for artifact scan jobs. It
connects the existing M4.15 queue, M4.16 claim lease, M4.17 terminal update,
object storage, and an injectable scanner interface.

This slice does not add a long-running worker process, cron wiring, external
antivirus integration, retry/backoff scheduling, quarantine review,
fail-closed outage policy, public APIs, or a database migration.

## Execution Contract

`ProcessArtifactScanJobs` accepts scanner name, worker ID, limit, and lease
TTL. The application service:

1. validates artifact service, object storage, scanner, request, and worker ID;
2. claims due jobs through `ClaimArtifactScanJobs`;
3. loads each claimed artifact by scoped `thread_id + artifact_id`;
4. reads content from the server-side object URI;
5. calls the injected scanner with scoped IDs, content type, size, and bytes;
6. completes the job through `CompleteArtifactScanJob` when the scanner returns
   a valid scan status;
7. emits the existing content-free `artifact.scan.completed` audit event after
   a successful terminal update;
8. fails the claimed job with a generic bounded error when the artifact is
   missing, the object URI is absent, object storage cannot read, the scanner
   errors, or the scanner result is invalid.

The response reports claimed, succeeded, failed, and skipped counts. A skipped
job means the row was missing, the lease was no longer terminally writable, or
another worker won the terminal update race.

## Scanner Boundary

`ArtifactContentScanner` is an injected application interface. The request
contains only the data a content scanner needs:

- space, thread, run, user, artifact, and file IDs;
- scanner name;
- content type and byte size;
- artifact bytes.

It must not receive object URI, virtual path, storage URL, title, filename,
prompt text, model output, tool arguments, credentials, checkpoint bytes, or
provider raw bodies through this contract. Real engines can be adapted behind
this interface later.

## Safety Contract

Worker failure text is intentionally generic. Scanner errors, object storage
errors, object keys, virtual paths, filenames, and file bytes must not be
stored in scan job errors, surfaced in events, or returned to clients.

Successful scan audit events reuse `coze.artifact_scan.v1` and include scoped
IDs, artifact type, content type, size, scan status, scanner, scanner version,
and scanned time only. They must not include scanner reason or raw scanner
errors.

Infrastructure failures while claiming jobs or writing terminal state return an
application error instead of turning a job into a false scan decision.

## Tests

- Processing a claimed job reads the server-side object, calls the scanner, and
  completes the job with the scanner result while emitting a content-free audit
  event.
- Scanner failure marks the job failed with generic error text and does not
  leak object key, virtual path, filename, or file bytes.
- Processing rejects a missing scanner before claiming jobs.
