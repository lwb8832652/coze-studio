# Artifact Preview MIME Hardening Design

## Goal

Prevent the task artifact drawer from offering browser preview actions for
active-content or mismatched MIME artifacts, even if an artifact row has an
overly permissive `preview_mode`.

## Existing Backend Boundary

The content endpoint already returns `X-Content-Type-Options: nosniff`.
Application reads force attachment for `download` and `unsupported` preview
modes, for stored content types that determine as download-only, and for
sniffed content that determines as download-only.

M4.32 does not replace those backend controls. It hardens the frontend
affordance so users are not offered a preview button for content that the UI
can classify as unsafe before requesting bytes.

## Frontend Rule

The drawer may show `预览` only when both `preview_mode` and `content_type`
agree with a safe preview family:

- `preview_mode=text`: `text/plain`, `text/markdown`, `text/csv`,
  `text/tab-separated-values`, or `application/json`
- `preview_mode=image`: raster image MIME types only
- `preview_mode=pdf`: `application/pdf`

HTML, XHTML, SVG, `application/octet-stream`, missing types, and
preview-mode/content-type mismatches are download-only in the drawer.

## Security

The UI must not render artifact bytes through `dangerouslySetInnerHTML`,
inline HTML/SVG iframes, object storage URLs, storage URLs, or object URIs.
The backend content endpoint remains the authority for final read policy,
scan status, attachment disposition, and content sniffing.
