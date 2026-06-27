# Artifact Preview Family Hardening Design

## Goal

Keep task artifact preview affordances aligned across preview buttons and
inline renderer routing.

M4.38 centralizes frontend MIME family checks in one helper. The frontend
continues to use a positive allow-list:

- text family: `text/plain`, `text/markdown`, `text/csv`,
  `text/tab-separated-values`, `application/json`;
- image family: `image/png`, `image/jpeg`, `image/gif`, `image/webp`,
  `image/bmp`, `image/tiff`;
- PDF family: `application/pdf`.

Everything else returns no preview family, including HTML, XHTML, SVG,
octet-stream, empty content type, and preview-mode/content-type mismatches.

## Frontend Contract

`artifactPreviewFamily` is the canonical frontend decision for whether an
artifact may show a preview action. `artifactInlinePreviewKind` may only
return a text renderer kind when the family is `text`.

This keeps UI affordances and inline renderer routing from drifting when new
MIME types are added.

## Security Boundary

This is frontend hardening only. The backend content and signed URL endpoints
remain authoritative for authorization, active artifact lookup, scan read
policy, server-side MIME sniffing, `nosniff`, attachment disposition, and
storage signing.

The frontend helper must never turn unknown MIME types into previewable
content. Add new preview support by extending the allow-list and tests
together.
