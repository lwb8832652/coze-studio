# Artifact Inline Preview Transform Design

## Goal

Improve task artifact readability inside the task drawer without opening a new
active-content execution surface.

M4.36 adds inline transforms only for already-safe text artifact families:

- `text/plain`
- `text/markdown`
- `application/json`
- `text/csv`
- `text/tab-separated-values`

HTML, XHTML, SVG, octet-stream, missing content types, and MIME/mode
mismatches remain download-only. Raster images and PDFs may keep using signed
preview URLs until browser E2E and MIME-specific renderer hardening are done.

## Data Flow

The drawer still lists artifacts through `ListTaskThreadArtifacts`. When the
user clicks `预览` on a safe text artifact, the frontend calls the existing
Workbench content endpoint:

`GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content?mode=preview`

This endpoint remains the authority for authorization, active artifact lookup,
scan read policy, MIME sniffing, `nosniff`, and server-side
`Content-Disposition`.

The frontend reads the returned Blob as text and applies a local presentation
transform:

- JSON is parsed and pretty-printed when valid, otherwise shown as source text.
- CSV and TSV are parsed into a bounded table.
- Markdown is shown as escaped source text, not rendered as HTML.
- Plain text is shown as escaped source text.

## Frontend Bounds

Inline previews are bounded before rendering:

- source text is capped at 128 KiB;
- CSV/TSV display is capped at 50 data rows and 12 columns;
- truncated previews show only a bounded marker;
- React text rendering is used for source text, with no
  `dangerouslySetInnerHTML`.

CSV/TSV parsing supports quoted fields, escaped quotes, LF, and CRLF for the
bounded preview use case. It is not a durable data import parser and must not
be reused for backend ingestion or security decisions.

## Security Contract

Inline preview state may include only:

- artifact ID;
- display name;
- effective content type;
- bounded transformed text;
- bounded CSV/TSV rows and column labels;
- a truncation marker.

It must not include object URI, virtual path as a fetch authority, object key,
signed URL, file bytes beyond the frontend cap, scanner raw response, prompt
text, model output, tool arguments, credentials, checkpoint bytes, or provider
raw bodies.

## Non-Goals

M4.36 does not add HTML/SVG rendering, Markdown-to-HTML rendering, image/PDF
embedding inside the drawer, browser E2E coverage, permanent renderer
sandboxing, object-store reconciliation, or new backend artifact endpoints.
