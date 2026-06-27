# M4.7 Artifact Content API Design

## Scope

M4.7 adds a server-side content read path for registered task artifacts. The API accepts only `thread_id`, `artifact_id`, and an optional read mode. It never accepts or returns object storage keys. The service resolves artifact metadata from `agent_artifacts`, reads the object through the configured storage backend, and returns bytes with conservative response headers.

## Contract

- Endpoint: `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content`
- Query:
  - `mode=preview` returns inline content only for preview-safe artifact types.
  - `mode=download` always returns an attachment.
  - Empty or unknown mode defaults to preview behavior.
- Response:
  - Body is the stored artifact bytes.
  - `Content-Type` uses the artifact content type, or `application/octet-stream` if empty.
  - `Content-Disposition` is `inline` for preview-safe text, image, and PDF artifacts, otherwise `attachment`.
  - `X-Content-Type-Options` is always `nosniff`.

## Safety Rules

- The handler must resolve artifact scope through `thread_id + artifact_id`.
- Object URI is read only from the artifact row and is never returned in JSON, headers, or errors.
- HTML, XHTML, SVG, unknown content types, and `preview_mode=download` are forced to attachment even when `mode=preview`.
- Missing storage configuration returns an application error instead of falling back to any host filesystem path.
- The read path is intentionally separate from future signed download links, preview transforms, or access-control expansion.

## Data Flow

1. Handler binds route and query parameters.
2. Application service calls domain artifact service to fetch the scoped artifact.
3. Application service reads the object via a narrow storage interface.
4. Application service maps metadata to a content response and computes disposition.
5. Handler writes bytes and safe headers.

## Tests

- Repository: fetching by `thread_id + artifact_id` returns the artifact and rejects cross-thread reads.
- Application: reading content returns bytes, metadata, safe inline disposition, and never exposes object URI in the DTO.
- Application: active content such as HTML is attachment even in preview mode.
- Handler: endpoint returns bytes with content type, content disposition, and `nosniff`.
- Router: Workbench task thread artifact content route is registered.
