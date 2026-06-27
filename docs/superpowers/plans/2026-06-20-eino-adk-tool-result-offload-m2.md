# Eino ADK Tool Result Offload M2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist oversized Eino tool results in a tenant-scoped Coze runtime
workspace and let the Agent recover bounded ranges through a read-only
`read_file` tool.

**Architecture:** Eino reduction remains the truncation and historical-clear
engine. A Coze backend writes content to object storage, upserts an
`agent_files` audit record, and exposes deterministic virtual paths containing
the source run ID. Eino filesystem middleware registers only a custom,
byte-paginated `read_file`; list, write, edit, glob, grep, multimodal read, and
shell execution remain disabled until M4.

**Tech Stack:** Go 1.24, Eino `v0.9.9` reduction/filesystem middleware, GORM,
MySQL/Atlas, Coze object storage, Go testing, Testify.

---

## File Structure

- Create `backend/application/agentthread/adk_offload_backend.go` for virtual
  path validation, deterministic object keys, object-storage reads/writes,
  bounded byte ranges, registry calls, and safe events.
- Create `backend/application/agentthread/adk_offload_backend_test.go` for path,
  tenancy, idempotency, limits, cleanup, range-read, and event contracts.
- Modify `backend/application/agentthread/adk_middleware.go` to build one
  per-run backend, enable Eino reduction, and register only the custom
  `read_file`.
- Modify `backend/application/agentthread/adk_middleware_test.go` for fail-closed
  defaults, configured offload, tool exposure, and middleware ordering.
- Create `backend/domain/agentthread/entity/file.go` for runtime file metadata.
- Create `backend/domain/agentthread/repository/runtime_file.go` for the
  dedicated repository contract.
- Modify `backend/domain/agentthread/repository/mysql.go` for the GORM model and
  idempotent upsert.
- Create `backend/domain/agentthread/service/runtime_file.go` for validation and
  ownership derivation from the run.
- Create focused repository and service tests.
- Modify `backend/application/agentthread/init.go` and
  `backend/application/agentthread/service.go` to expose the dedicated runtime
  file service without widening the existing `ThreadService` interface.
- Modify `backend/application/application.go` to inject object storage and the
  runtime file registry into the ADK middleware assembler.
- Add `docker/atlas/migrations/20260620000300_agent_files.sql` and update
  `docker/atlas/opencoze_latest_schema.hcl`.
- Update `AGENTS.md` and the DeerFlow parity roadmap after verification.

## Task 1: Durable Runtime File Registration

- [x] Add failing domain tests for `RegisterRuntimeFile`:
  - run, thread, space, and owner are loaded from the persisted run;
  - only `workspace` files under the reserved offload prefix are accepted;
  - size and SHA-256 digest must be positive and canonical;
  - object URI, virtual path, and metadata are validated without exposing
    content;
  - repeated registration of the same run/path updates digest, size, object
    URI, and metadata while retaining one row.
- [x] Add `AgentFile`, `AgentFileKind`, and `AgentFileStatus` entities.
- [x] Add a separate `RuntimeFileRepository` with:

```go
UpsertRuntimeFile(
    context.Context,
    *entity.AgentFile,
) (*entity.AgentFile, bool, error)
```

- [x] Implement `RuntimeFileService.RegisterRuntimeFile`. Generate an ID for a
  new row, derive ownership from `GetRun`, and delegate the upsert.
- [x] Add a GORM `agentFilePO` and an atomic MySQL upsert keyed by
  `(run_id, virtual_path)`. The returned boolean reports whether a row was
  created.
- [x] Add repository tests for insert, update, cross-run isolation, and JSON
  metadata persistence.
- [x] Add the migration and schema definition:

```sql
CREATE TABLE agent_files (
  id BIGINT PRIMARY KEY,
  space_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  thread_id BIGINT NOT NULL,
  run_id BIGINT NOT NULL,
  file_name VARCHAR(255) NOT NULL,
  original_file_name VARCHAR(255) NOT NULL DEFAULT '',
  file_kind VARCHAR(32) NOT NULL,
  virtual_path VARCHAR(1024) NOT NULL,
  object_uri VARCHAR(1024) NOT NULL,
  content_type VARCHAR(255) NOT NULL DEFAULT '',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  digest VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(32) NOT NULL DEFAULT 'active',
  metadata JSON NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  UNIQUE KEY uk_agent_files_run_path (run_id, virtual_path),
  KEY idx_agent_files_thread_kind (thread_id, file_kind),
  KEY idx_agent_files_space_created (space_id, created_at)
);
```

- [x] Run focused domain and repository tests.

## Task 2: Tenant-Scoped Offload Backend

- [x] Add failing tests for virtual paths:
  - accepted form is
    `/mnt/user-data/workspace/.coze/tool-results/runs/{run_id}/{trunc|clear}/{digest}.txt`;
  - writes require the embedded run ID to equal the active run;
  - reads may target historical runs but always derive space/thread from the
    active run;
  - traversal, backslash, control characters, malformed IDs, unsupported
    phases, extra segments, and non-hex names are rejected.
- [x] Define narrow dependencies:

```go
type ADKOffloadObjectStorage interface {
    PutObject(context.Context, string, []byte, ...storage.PutOptFn) error
    GetObject(context.Context, string) ([]byte, error)
}

type ADKRuntimeFileRegistry interface {
    RegisterRuntimeFile(
        context.Context,
        *RegisterRuntimeFileRequest,
    ) (*RuntimeFileSummary, bool, error)
}
```

- [x] Implement deterministic object keys:

```text
agent-runtime/{space_id}/{thread_id}/runs/{run_id}/tool-results/{phase}/{digest}.txt
```

  Never accept an object URI from Eino or model input.
- [x] Enforce default limits:
  - maximum offload file: 16 MiB;
  - default read range: 64 KiB;
  - maximum read range: 256 KiB.
- [x] On write:
  - reject empty or oversized content;
  - compute SHA-256 for registration metadata;
  - write `text/plain; charset=utf-8` to object storage;
  - register the file;
  - preserve a deterministic object if registration fails because an earlier
    successful registration may already reference the same key;
  - emit `context.tool_result_offloaded` only after both operations succeed,
    with IDs, phase, sizes, and digest but no content or object URI.
- [x] On range read:
  - load only the deterministic current-thread object key;
  - return a UTF-8-safe byte range, total size, and next offset;
  - reject negative offsets and excessive limits;
  - never return object URI or storage errors containing credentials.
- [x] Implement the unused Eino filesystem methods as explicit fail-closed
  errors. Do not emulate a general workspace in this milestone.
- [x] Run focused backend tests, including 20 idempotent concurrent writes to
  the same virtual path.

## Task 3: Read-Only Eino Filesystem Tool

- [x] Add a failing test that the configured filesystem middleware exposes
  exactly `read_file`; `ls`, `write_file`, `edit_file`, `glob`, `grep`, and
  `execute` must be absent.
- [x] Create a custom Eino tool with input:

```go
type adkReadOffloadInput struct {
    FilePath   string `json:"file_path"`
    OffsetByte int64  `json:"offset_byte,omitempty"`
    LimitBytes int    `json:"limit_bytes,omitempty"`
}
```

- [x] Return numbered metadata plus content:

```text
path: <virtual path>
offset_byte: <start>
next_offset_byte: <next or 0>
total_bytes: <total>
content:
<bounded content>
```

- [x] Configure Eino filesystem middleware with:
  - the custom `read_file`;
  - every other filesystem tool disabled;
  - `WithoutLargeToolResultOffloading: true` so only reduction owns offload;
  - no shell or streaming shell.
- [x] Add tool-call tests for the default range, explicit pagination, EOF,
  invalid path, and oversized limit.

## Task 4: Reduction And Runtime Wiring

- [x] Add failing middleware tests proving:
  - without an offload backend factory reduction remains fully disabled;
  - with a backend, oversized invokable and streaming tool results are written
    and replaced with Eino notices containing only the virtual path;
  - historical clear uses the same backend and read tool;
  - backend failure fails the tool call without returning a false success;
  - reduction and filesystem middleware share one per-run backend instance.
- [x] Add run config:

```json
{
  "tool_result_reduction": {
    "max_length_for_trunc": 50000,
    "max_tokens_for_clear": 100000,
    "clear_retention_suffix_limit": 1,
    "clear_at_least_tokens": 1000,
    "max_offload_bytes": 16777216,
    "default_read_bytes": 65536,
    "max_read_bytes": 262144
  }
}
```

  Reject non-positive values and ensure clear tokens remain below the context
  window.
- [x] Build the backend once in `ADKMiddlewareAssembler.Build` and pass it to
  both reduction and filesystem builders.
- [x] Enable Eino reduction only when the factory returns a backend:
  - `SkipTruncation: false`;
  - `SkipClear: false`;
  - deterministic trunc/clear path generators;
  - the existing Coze token estimator;
  - `ReadFileToolName: "read_file"`.
- [x] Preserve middleware order: reduction before policy/audit/usage and
  filesystem before the final multimodal provider projection.
- [x] Wire production storage and runtime file registration in
  `application.Init`.
- [x] Add ADK integration coverage where a tool emits an oversized result and
  the next model call invokes `read_file`. Cover fresh-run access to the source
  run's offload with the backend tenancy test.

## Task 5: Verification And Roadmap

- [x] Run focused offload, middleware, repository, and replay tests 20 times.
- [x] Run the `agentthread` race suite.
- [x] Run:

```bash
cd backend
go mod verify
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./...
```

- [x] Run `git diff --check` and Atlas migration validation/hash generation.
- [x] Update `AGENTS.md` with the read-only offload boundary and resume path
  rule.
- [x] Mark M2.9a complete in the master roadmap. Keep general workspace tools,
  uploads, output promotion, `agent_artifacts`, preview/download APIs, MIME
  scanning, retention cleanup, shell, and sandbox providers open in M4.
