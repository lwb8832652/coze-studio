# M4.6 Artifact List API Design

## Goal

Expose a read-only task artifact listing endpoint for the workbench task detail
experience.

## Scope

This slice adds metadata listing only. It does not add artifact preview,
download, file-content reads, object-storage presigned URLs, deletion, output
directory scanning, retention cleanup, audit events, or frontend drawer UI.

## Endpoint

```text
GET /api/workbench/task_threads/{thread_id}/artifacts
```

Query parameters:

- `run_id`: optional source run filter;
- `page`: optional page number;
- `page_size`: optional page size.

Response data:

- `artifact_id`;
- `thread_id`;
- `run_id`;
- `file_id`;
- `title`;
- `artifact_type`;
- `virtual_path`;
- `content_type`;
- `size_bytes`;
- `preview_mode`;
- `metadata`;
- `created_at`;
- `updated_at`;
- `total`.

The response intentionally omits `object_uri`, raw storage URLs, file bytes,
inline preview content, and download links.

## Application Mapping

The application service maps domain `AgentArtifact` rows to workbench DTOs
through `ArtifactSummary`. Domain listing remains the source of pagination and
run filtering.

## Security Boundary

This endpoint is not an artifact access endpoint. It returns metadata needed
for task-detail presentation only. Future content endpoints must perform
thread authorization, server-side MIME sniffing, active-content disposition,
range/download controls, and audit logging before reading object storage.

## Testing

Tests cover application DTO mapping, handler JSON response shape, route
registration, run filtering, and the absence of object URI leakage.
