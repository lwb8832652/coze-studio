# M4.10 Artifact MIME Sniffing Design

## Scope

M4.10 adds read-time MIME sniffing to the artifact content endpoint. Artifact
registration still stores the content type copied from `agent_files`, but the
content endpoint must not trust that metadata alone when deciding whether bytes
can be rendered inline.

This slice does not add a scanner worker, preview conversion service,
quarantine workflow, or signed URL flow. It only hardens response content type
and disposition for bytes already authorized by the artifact read path.

## Contract

- After object storage returns bytes, the application service runs standard
  byte sniffing on the first bytes of the artifact.
- Response `Content-Type` uses the sniffed type when it is available.
- Preview disposition is conservative:
  - `mode=download` always returns attachment.
  - `mode=preview` may return inline only when both the registered content type
    and sniffed content type map to preview-safe types.
  - If either side maps to download, the response is attachment.
- Empty content falls back to the registered content type, or
  `application/octet-stream` when metadata is empty.
- The audit event added in M4.9 records the effective response content type and
  attachment decision only. It still must not include bytes, object URI,
  virtual path, storage URL, filename, prompt text, model output, tool
  arguments, checkpoint bytes, or provider raw data.

## Safety Rules

- HTML, XHTML, SVG, XML, JavaScript, unknown binary, and octet-stream content
  are never served inline by this endpoint.
- A safe registered type cannot override unsafe sniffed bytes.
- A safe sniffed type cannot override an unsafe registered type, because the
  stored metadata may carry policy meaning from registration.
- MIME sniffing is a presentation and response-header control. It does not
  replace scan status, antivirus scanning, sandbox execution, or active-content
  preview transforms.

## Tests

- Application: artifact registered as text but containing HTML is served with a
  sniffed HTML content type and attachment disposition.
- Application: artifact registered as SVG but containing plain text remains
  attachment because registered type is unsafe.
- Handler: content endpoint uses the effective sniffed content type in the
  response header.
