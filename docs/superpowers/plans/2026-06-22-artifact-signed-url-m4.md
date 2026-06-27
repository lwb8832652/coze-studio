# Artifact Signed URL M4 Plan

## Result

M4.33 added preview artifact signed URLs behind Coze authorization, scan
policy, and MIME sniffing. M4.34 extends the same endpoint to forced-attachment
downloads by adding signed response-header overrides at the storage adapter
boundary.

## Completed

- Added `CreateArtifactSignedURL` application flow with TTL clamp
  (`60-3600`, default `300`).
- Added an object-storage signing adapter boundary using the existing
  `GetObjectUrl` implementation on MinIO, S3, and TOS storage.
- Reused artifact read authorization and scan read policy before signing.
- Added server-side content sniffing before signing so misclassified active
  content cannot bypass the backend content endpoint.
- Added Workbench API route
  `GET /api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url`.
- Added receipt-only API payloads and frontend service.
- Switched safe task artifact previews to signed URL open-in-new-tab behavior.
- Added `storage.GetOption` response-header override fields and mapped them to
  MinIO query parameters, S3 `GetObjectInput` fields, and TOS pre-signed query
  parameters.
- Allowed `mode=download` signed URLs only after authorization, scan policy,
  active artifact lookup, and server-side MIME sniffing. Download signing
  forces `Content-Disposition: attachment` from server-side artifact metadata
  and the sniffed content type.
- Switched the task artifact drawer download action to `mode=download` signed
  URLs instead of blob content downloads.
- Superseded text-like preview usage in M4.36: safe text/JSON/CSV/TSV/Markdown
  previews now use the content endpoint for inline transforms, while
  downloads and non-text safe preview families continue to use signed URLs.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./infra/storage -count=1 -gcflags="all=-l -N"`
- `go test ./application/agentthread -run 'TestApplicationCreateArtifactSignedURL' -count=1 -gcflags="all=-l -N"`
- `go test ./api/handler/coze -run 'TestGetTaskThreadArtifactSignedURLHandler' -count=1 -gcflags="all=-l -N"`
- `go test ./api/router/coze -run 'TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"`
- `npx vitest run src/pages/tasks/__tests__/tasks-service.test.ts src/pages/tasks/__tests__/task-detail.test.tsx`

## Remaining

Browser E2E coverage, MIME-specific renderer hardening, and image/PDF drawer
embedding remain separate production acceptance tasks.
