# Artifact Signed URL Design

## Goal

Add a controlled signed URL path for task artifact previews and downloads
without weakening artifact authorization, scan policy, or active-content
protections.

## API

`GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url`
creates a short-lived signed URL. Supported modes are `preview` and
`download`. `ttl_seconds` is optional and clamped server-side to 60-3600
seconds; the default is 300 seconds.

The response contains only a receipt:

- artifact ID
- signed URL
- effective expiry seconds
- effective content type
- preview mode

It does not return object URI, virtual path, object key, file bytes, scanner
raw response, prompt text, model output, tool arguments, credentials,
checkpoint bytes, or provider raw bodies.

## Backend Guardrails

Signed URL creation runs the same authorization boundary as artifact content
reads. It then loads the active artifact row, checks the artifact scan read
policy, fetches object content server-side for MIME sniffing, and only then
calls the object storage signer.

Preview mode creates a URL only when stored preview mode, stored content type,
and sniffed content type all remain in the same safe preview family.

Download mode may sign scan-allowed active artifacts, including active-content
MIME types, only by forcing object storage response headers through signed
query parameters. The application passes:

- `response-content-disposition=attachment; filename*=UTF-8''...`
- `response-content-type=<sniffed content type>`

MinIO receives those as `PresignedGetObject` query parameters, S3 receives
them through `GetObjectInput.ResponseContentDisposition` and
`ResponseContentType`, and TOS receives them through `PreSignedURLInput.Query`.
Custom storage adapters must honor the same `storage.GetOption` fields before
download signing is considered safe.

## Frontend Behavior

The task artifact drawer uses signed URLs for downloads and for non-text safe
preview families such as image and PDF. M4.36 changed safe text-like previews
to use the backend content endpoint with `mode=preview` and render bounded
inline transforms inside the drawer. Download actions request `mode=download`
and trigger a hidden anchor against the signed URL. The backend content
endpoint must continue to set `X-Content-Type-Options: nosniff` plus
server-side `Content-Disposition`.

## Remaining

Browser E2E coverage, MIME-specific renderer hardening, and image/PDF drawer
embedding remain separate production acceptance work.
