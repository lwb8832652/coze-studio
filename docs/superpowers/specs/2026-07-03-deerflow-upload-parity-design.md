# DeerFlow Upload Parity Design

## Goal

P2-F closes the Composer upload loop against DeerFlow behavior:

1. users can attach files from the new-task and task-detail composers;
2. files are uploaded to the current task thread before the run starts;
3. user messages and run input carry bounded file metadata;
4. Eino ADK receives a DeerFlow-like `<uploaded_files>` context block;
5. runtime `read_file` can read `/mnt/user-data/uploads/*`;
6. list/delete APIs can show and remove thread uploads without exposing storage
   object URIs.

This is not a visual-only slice. A paperclip button without upload, persistence,
and runtime readability does not count as parity.

## DeerFlow Reference Chain

- Frontend upload API:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/uploads/api.ts`
  defines `POST /api/threads/:thread_id/uploads`, `GET /uploads/list`, and
  `DELETE /uploads/:filename`.
- Frontend submit integration:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/core/uploads/hooks.ts`
  uploads selected files before thread submit and maps them to
  `additional_kwargs.files`.
- Composer UI:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/input-box.tsx`
  opens the file dialog from the paperclip action and renders attachment chips.
- Runtime context:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/agents/middlewares/uploads_middleware.py`
  prepends `<uploaded_files>` to the latest human message and points tools at
  `/mnt/user-data/uploads/<filename>`.
- Backend upload API:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/uploads.py`
  applies owner checks, safe filename normalization, per-file and total size
  limits, list, delete, and bounded response metadata.

## Coze Current Contract

Coze already has the right storage foundation:

- `agent_files` table;
- `AgentFileKindUpload`;
- object storage interfaces used by artifact/runtime file paths;
- ADK offload `read_file` tool.

The missing pieces are upload-specific contracts:

- thread-scoped upload API and service;
- `run_id = 0` support for thread uploads in the existing `agent_files` table;
- `/mnt/user-data/uploads/*` virtual path validation and resolution;
- submit-time upload plumbing in both new-task and follow-up flows;
- ADK middleware that injects bounded uploaded file context.

## Data Contract

Upload virtual paths use:

```text
/mnt/user-data/uploads/<safe_unique_filename>
```

Storage object keys use:

```text
agent-runtime/<space_id>/<thread_id>/uploads/<safe_unique_filename>
```

Thread uploads are stored in `agent_files` with:

- `file_kind = upload`;
- `run_id = 0`;
- `status = active` or deleted/archived status supported by existing entity
  enums;
- `object_uri` kept server-side only;
- `virtual_path` returned to the client and runtime.

Public upload metadata may include:

- `file_id`;
- `filename`;
- `size`;
- `path` / `virtual_path`;
- `content_type`;
- `created_at`.

Public upload metadata must not include:

- `object_uri`;
- credentials;
- raw scanner output;
- filesystem host paths;
- prompt/tool/provider payloads.

## Submit Flow

### New Task With Attachments

Current one-call create starts the run immediately, so attachments would arrive
too late. P2-F adds a deferred path:

1. `POST /api/workbench/task_threads` with `defer_start = true` creates only the
   thread.
2. Frontend uploads selected files to the returned thread ID.
3. Frontend appends the user message with bounded file metadata.
4. Frontend creates the run with the same historical message input and uploaded
   file metadata.

The no-attachment path keeps the existing one-call behavior.

### Follow-Up With Attachments

1. Upload selected files to the existing thread.
2. Append the user message with bounded file metadata.
3. Create the follow-up run with full history plus the latest message metadata.

## Runtime Injection

Add an Eino ADK middleware after memory/skill preparation and before model call
that:

- reads uploaded file metadata from the active run input or message metadata;
- lists current thread uploads when needed;
- prepends a bounded `<uploaded_files>` block to the latest user message;
- hides the injected block from Workbench-visible transcript rendering;
- does not expose `object_uri`.

The block mirrors DeerFlow intent:

```xml
<uploaded_files>
The following files are available to read:
- filename: example.pdf
  path: /mnt/user-data/uploads/example.pdf
  size: 12345 bytes
</uploaded_files>
```

Runtime `read_file` must resolve upload virtual paths through the same tenant,
thread, and status checks as output/offload paths.

## Limits

Use DeerFlow-compatible defaults unless the project already has stricter limits:

- max files per upload request: 10;
- max file size: 50 MiB;
- max total upload size: 100 MiB;
- safe filename basename only;
- skip or reject path traversal and control characters;
- generated duplicate filenames use a deterministic suffix.

## Verification

Minimum P2-F verification:

- backend tests for upload/list/delete/limits/path validation;
- backend tests for deferred new-task create and follow-up run file metadata;
- backend tests for ADK upload injection and `read_file` upload resolution;
- frontend tests for file picker, attachment cards, remove, and submit upload
  sequencing;
- existing task-detail/workbench Vitest matrix;
- evidence note summarizing commands and remaining browser/manual checks.
