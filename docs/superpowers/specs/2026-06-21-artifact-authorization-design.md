# M4.13 Artifact Authorization Design

## Scope

M4.13 adds an explicit authorization boundary for task-thread artifact list,
content, and delete operations. Earlier artifact slices already scoped every
lookup by `thread_id + artifact_id`, but they did not carry the current viewer
identity through the application layer.

The first production policy is intentionally narrow: only the canonical thread
creator can read or delete that thread's artifacts. Space-member, admin, share
link, and service-account expansion will plug into the same interface later.

## Contract

- Workbench handlers extract the viewer from request context:
  - web session user ID first;
  - OpenAPI key user ID second;
  - zero when neither is available.
- `ListArtifacts`, `ReadArtifactContent`, and `DeleteArtifact` requests carry
  `ViewerID`.
- `ApplicationService` calls an injected `ArtifactAuthorizer` before list,
  object-storage read, or soft delete.
- A nil authorizer preserves legacy tests and local compatibility, but
  production initialization wires the thread-owner authorizer.
- Denied access returns `ErrArtifactAccessDenied` from the application layer.
- The Workbench handler maps that error to HTTP 403 with a generic
  `artifact access denied` message.

## Thread Owner Authorizer

`ThreadOwnerArtifactAuthorizer` depends only on `domainservice.ThreadService`.
It loads the thread by ID and allows access when:

- `viewer_id > 0`;
- the thread exists;
- `thread.creator_id == viewer_id`.

It does not inspect artifact object URIs, virtual paths, storage URLs, file
names, prompt text, model output, checkpoint bytes, or provider payloads.

## Safety Rules

- Authorization runs before artifact repository listing, artifact metadata
  lookup, object-storage reads, or soft-delete writes.
- Denial errors must be content-free and must not disclose whether an artifact
  ID exists inside an inaccessible thread.
- Audit events remain unchanged and are emitted only after successful
  operations.
- The frontend continues to send only scoped IDs; authorization decisions stay
  server-owned.

## Tests

- Application service denies list/content/delete before artifact storage or
  repository calls when the authorizer rejects the viewer.
- Application service passes operation, thread ID, artifact ID when relevant,
  and viewer ID to the authorizer.
- Handler passes viewer ID from session/OpenAPI context into artifact
  requests.
- Handler maps `ErrArtifactAccessDenied` to HTTP 403.
