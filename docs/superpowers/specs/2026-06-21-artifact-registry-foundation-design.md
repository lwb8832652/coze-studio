# M4.4 Artifact Registry Foundation Design

## Goal

Add the durable backend storage foundation for user-visible task artifacts.

## Scope

This slice creates the `agent_artifacts` persistence layer and Go repository
contract. It does not expose artifact HTTP APIs, frontend drawer UI,
preview/download handlers, MIME sniffing, active-content response policy,
output directory scanning, deletion, retention cleanup, or audit events.

## Data Model

`agent_files` remains the low-level file metadata table. `agent_artifacts` is
the presentation registry that says a file should be shown as a task artifact.

The initial schema stores:

- tenant and ownership scope: `space_id`, `user_id`, `thread_id`, `run_id`;
- the backing file reference: `file_id`;
- presentation fields: `title`, `artifact_type`, `preview_mode`;
- access metadata copied from the file: `virtual_path`, `object_uri`,
  `content_type`, `size_bytes`;
- structured `metadata`;
- `created_at` and `updated_at`.

`file_id` is unique. Re-registering the same file updates the presentation
metadata without changing the existing artifact ID or original creation time.

## Listing Semantics

The repository lists artifacts by `thread_id`, optionally filtered by `run_id`,
ordered by newest `created_at` and `id`. This matches the future task-detail
drawer requirement to group and filter artifacts by run while keeping a stable
thread-level list.

## Security Boundary

This slice intentionally does not make artifacts readable by users. Future API
work must perform thread authorization, resolve virtual paths, avoid direct
object-storage URL exposure, sniff MIME types server-side, force active content
to attachment downloads, write audit events, and respect retention policy.

## Testing

Repository tests cover idempotent upsert by `file_id`, preservation of artifact
ID and creation time on update, thread-level listing, run-level filtering, and
row-count stability.
