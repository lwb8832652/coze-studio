# Artifact PDF Preview Contract Design

## Goal

Keep PDF artifact previews safe and predictable after raster image previews
move into the task drawer.

M4.40 keeps PDF previews on the signed URL new-tab path. PDFs are not embedded
inside the drawer, not rendered in an iframe, and not mixed with the raster
image renderer.

## Behavior

When a PDF artifact with `preview_mode=pdf` and `content_type=application/pdf`
is previewed, the frontend requests:

`GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url?mode=preview&ttl_seconds=300`

If a signed URL is returned, the frontend:

1. clears any existing drawer inline preview;
2. opens the signed URL with `window.open(url, '_blank', 'noopener,noreferrer')`;
3. does not render the signed URL as visible text or copy it into selectors.

This prevents stale inline image/text previews from remaining visible after a
non-inline preview action.

## Non-Goals

M4.40 does not add PDF iframe rendering, PDF.js, browser pixel checks, mobile
visual checks, or renderer sandboxing. Those remain separate production
acceptance decisions.
