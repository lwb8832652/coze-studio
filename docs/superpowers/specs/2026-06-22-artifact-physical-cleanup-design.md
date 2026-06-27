# Artifact Physical Cleanup Design

## Goal

Add a safe physical cleanup path for task artifacts that have already been
soft-deleted and have passed a server-side retention window.

## Scope

Cleanup is backend-only in M4.35. It does not add a user-facing permanent
delete action, a frontend cleanup control, or recursive object-store garbage
collection.

## Rules

- Only artifacts with `deleted_at > 0` and `deleted_at <= cutoff` are eligible.
- The backing `agent_files` row must still be `active`; rows already marked
  `deleted` are skipped so cleanup is idempotent.
- Active artifacts are never physically cleaned.
- Cleanup deletes only the artifact's server-side `ObjectURI`; the client never
  supplies object keys.
- The object store is called before the database file row is marked deleted.
- If object deletion fails, the file row remains active so a later worker pass
  can retry.
- If the object is already absent and the storage adapter returns
  `storage.ErrObjectNotFound`, cleanup treats that as success and marks the
  backing file deleted.
- Database artifact rows remain soft-deleted history rows. The cleanup pass
  marks only `agent_files.status=deleted` with an updated timestamp.

## Backend Shape

The domain repository owns two operations:

- list deleted artifact cleanup candidates by cutoff and limit, joining
  `agent_artifacts` to active `agent_files`;
- mark a backing file deleted by `file_id` and expected `object_uri`.

The application service owns the object-storage side effect. It calculates the
cutoff from `now - retention_ms`, lists candidates, deletes objects, marks file
rows deleted only after successful object deletion, and emits content-free
cleanup audit events.

## Audit Payload

Cleanup audit events may include schema, thread ID, run ID, artifact ID, file
ID, artifact type, content type, size, `deleted_at`, `cleaned_at`, and status.
They must not include object URI, virtual path, object key, filenames, file
bytes, scanner raw response, prompt text, model output, tool arguments,
credentials, checkpoint bytes, or provider raw bodies.

## Remaining

M4.35 does not add lifecycle scheduling, retention policy UI, object-store
prefix sweep reconciliation, or browser E2E coverage. Those stay separate
production acceptance tasks.
