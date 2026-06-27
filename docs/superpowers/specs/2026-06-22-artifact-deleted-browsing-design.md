# Artifact Deleted Browsing Design

## Goal

Complete the user-facing soft-delete lifecycle for task artifacts. Users can
open the task artifact drawer, switch from current artifacts to removed
artifacts, and restore an older soft-deleted artifact without relying only on
the immediate undo notice.

## API

`GET /api/workbench/task_threads/:thread_id/artifacts` keeps its default
behavior: it returns only active artifacts where `deleted_at = 0`.

The same endpoint accepts `deleted_only=true`. In that mode it returns only
soft-deleted artifacts where `deleted_at > 0`. The flag is explicit so existing
task detail loads, content reads, and active artifact pagination are not
silently broadened.

Responses include `deleted_at` as safe lifecycle metadata. They still do not
include object URIs or storage URLs.

## Backend Behavior

The repository applies one visibility predicate per request:

- default: `thread_id = ? AND deleted_at = 0`
- deleted-only: `thread_id = ? AND deleted_at > 0`

Run filters, pagination, and ordering continue to apply inside that selected
visibility. Restore remains a separate `POST .../restore` action and remains
registry-only.

## Frontend Behavior

The task artifact drawer shows a compact `当前 / 已移除` switch. `当前` keeps the
existing scan queue plus active artifact list. `已移除` loads
`deleted_only=true`, shows removed artifact metadata, and exposes only a
restore action per row.

Restoring from the removed list refreshes both the active artifact list and the
removed artifact list. Removed rows do not expose preview, download, delete, or
scan-review actions.

## Security

Deleted browsing is authorized through the existing artifact list operation and
does not add a content-read path. It exposes lifecycle metadata needed by the
drawer, but not object URIs, storage URLs, file bytes, scanner raw responses,
prompt text, model output, tool arguments, credentials, checkpoint bytes, or
provider raw bodies.
