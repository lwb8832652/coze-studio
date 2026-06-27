# Artifact Restore Design

## Goal

Complete the soft-delete lifecycle for task artifacts with a safe restore path.
Restore is a registry-only action: it makes a hidden artifact visible again and
does not mutate backing runtime files, object storage, scan jobs, or bytes.

## API

`POST /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/restore`
restores a previously deleted artifact row for the same thread. The response is
a receipt with `artifact_id` and `restored`; it does not return artifact
content, paths, object keys, storage URLs, or raw metadata.

## Domain Behavior

The repository performs a compare-and-swap update where
`thread_id + artifact_id + deleted_at > 0` clears `deleted_at` and updates
`updated_at`. Restoring an already-active or missing artifact returns
`restored=false`.

## Frontend

The task artifact drawer shows an immediate `撤销移除` action after a
successful delete. The action calls the restore endpoint with only thread and
artifact IDs, refreshes the artifact list, and keeps errors local to the
drawer.

## Audit

Successful restores emit `artifact.restored` with
`schema=coze.artifact_restored.v1`. Payloads may include scoped IDs, artifact
type, content type, size, and restored time. They must not include title,
virtual path, object URI, storage URL, filename, file bytes, scanner raw
response, prompt text, model output, tool arguments, credentials, checkpoint
bytes, or provider raw bodies.
