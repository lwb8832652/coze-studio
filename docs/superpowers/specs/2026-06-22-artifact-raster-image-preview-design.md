# Artifact Raster Image Preview Design

## Goal

Render safe raster image artifacts inside the task drawer without changing the
backend artifact security boundary.

M4.39 supports only the image preview family already allowed by
`artifactPreviewFamily`:

- `image/png`
- `image/jpeg`
- `image/gif`
- `image/webp`
- `image/bmp`
- `image/tiff`

SVG is not a raster image preview and remains download-only. PDF remains a
signed URL new-tab preview until a separate sandboxed renderer decision is
made.

## Data Flow

When a user clicks `预览` on a raster image artifact, the frontend requests:

`GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url?mode=preview&ttl_seconds=300`

The backend remains responsible for authorization, active artifact lookup,
scan read policy, server-side MIME sniffing, preview-mode/content-type
agreement, and short-lived signing.

The frontend renders the signed URL only as an `<img src>`. It must not show
the URL as text, persist it, send it to analytics, copy it into selector
attributes, or include object URI / object key / virtual path fetch authority
in the renderer state.

## UI Bounds

The drawer image renderer:

- uses `loading="lazy"`;
- uses `referrerPolicy="no-referrer"`;
- caps preview height and uses `object-fit: contain`;
- keeps a close action with the same preview shell as text previews.

## Non-Goals

M4.39 does not embed PDFs, render SVG, add Markdown HTML rendering, add
browser E2E, add image pixel-diff snapshots, or change backend storage signing
behavior.
