# M4.11 Artifact Delete Lifecycle Design

## Scope

M4.11 adds the first user-visible artifact lifecycle control: removing an
artifact from task detail. The operation is a soft delete of the artifact
registry row. It does not delete `agent_files`, object storage keys, runtime
offload files, or physical sandbox files.

The goal is to let the product hide an artifact from list, preview, and
download flows while preserving durable history for audit, retention, replay,
and orphan reconciliation.

## Data Model

`agent_artifacts` gains `deleted_at BIGINT NOT NULL DEFAULT 0`.

- `deleted_at=0`: active artifact, visible in task detail.
- `deleted_at>0`: removed artifact, hidden from normal list/read APIs.

The entity and repository mappings carry `DeletedAt` so future admin,
retention, and reconciliation jobs can reason about hidden rows without
looking inside JSON metadata.

## API Contract

- Endpoint:
  `DELETE /api/workbench/task_threads/:thread_id/artifacts/:artifact_id`
- Request authority:
  The path-scoped `thread_id + artifact_id` is the only accepted artifact
  locator in this slice. Object URI, virtual path, file ID, and storage URL are
  never accepted from the client.
- Response:
  `{ "code": 0, "msg": "success" }`
- Behavior:
  - Deletes are idempotent for active rows.
  - Already-deleted or cross-thread artifacts behave as not found.
  - List and content APIs only resolve active artifacts.

## Audit

If the artifact belongs to a run and a thread event sink is configured, a
successful delete emits `artifact.deleted` with
`schema=coze.artifact_deleted.v1`.

Allowed fields are metadata-only: `thread_id`, `run_id`, `artifact_id`,
`file_id`, `artifact_type`, `content_type`, `size_bytes`, and `deleted_at`.
The event must not include file bytes, object URI, virtual path, storage URL,
filename, prompt text, model output, tool arguments, checkpoint bytes,
credentials, or provider raw data.

## Non-Goals

- No physical object deletion.
- No retention worker or orphan reconciliation.
- No restore API.
- No frontend delete button in this slice.
- No broadened authorization model beyond the existing task-thread path scope.

## Tests

- Repository: soft delete sets `deleted_at`, hides the row from list/get, and
  is idempotent for already-deleted rows.
- Domain: delete validates scope and delegates to repository.
- Application: delete emits a content-free `artifact.deleted` event.
- Handler/router: Workbench DELETE endpoint is registered and returns success.
