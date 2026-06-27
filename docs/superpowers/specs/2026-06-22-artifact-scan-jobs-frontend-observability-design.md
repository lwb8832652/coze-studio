# M4.24 Artifact Scan Jobs Frontend Observability Design

## Scope

M4.24 connects the M4.22 read-only scan job API to the task-detail artifact
drawer. It gives operators a lightweight dead-letter view without adding job
mutation.

## Frontend Behavior

- Opening the artifact drawer requests
  `GET /api/workbench/task_threads/:thread_id/artifact_scan_jobs` with
  `page=1&page_size=20`.
- The drawer shows a scan queue count and recent scan jobs.
- Each item may show job ID, status, scanner, worker ID, attempt count, run ID,
  and bounded last error.
- The drawer includes a manual refresh action and local loading/error states.

## Safety Boundary

The frontend must not render artifact title, virtual path, object URI, storage
URL, filename, file bytes, scanner raw response, prompt text, model output,
tool arguments, credentials, checkpoint bytes, or provider raw bodies in the
scan-job section.

## Non-Goals

- Retry action.
- Manual release.
- Quarantine review or restore.
- Marking scans clean.
- Scanner service deployment.
- Signed links or physical cleanup.

## Tests

- Service test covers encoded route params, filters, pagination, and safe JSON
  response handling.
- Task detail test covers drawer scan-job loading and rendering without
  storage path leakage.
