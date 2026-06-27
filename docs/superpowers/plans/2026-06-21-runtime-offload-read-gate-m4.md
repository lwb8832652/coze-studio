# M4.3 Runtime Offload Read Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ADK runtime offload reads require an active registered runtime
file record before object storage is read.

**Architecture:** Add a resolve path from ADK offload backend to application
registry, domain runtime file service, and repository lookup. The ADK backend
parses and paginates, while Coze domain code owns registration, scope, status,
object URI, and metadata validation.

**Tech Stack:** Go, domain service tests, application ADK backend tests,
repository tests.

---

### Task 1: Add Red Tests

**Files:**
- Modify: `backend/application/agentthread/adk_offload_backend_test.go`

- [x] **Step 1: Cover orphan object read**

Create an object under the deterministic storage key without registering an
`agent_files` row.

- [x] **Step 2: Verify red**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestADKOffloadBackendRejectsUnregisteredRuntimeFileRead' -count=1 -gcflags="all=-l -N"
```

Expected red result: current read code returns the orphan object content.

### Task 2: Add Resolve Boundary

**Files:**
- Modify: `backend/domain/agentthread/repository/runtime_file.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/service/runtime_file.go`
- Modify: `backend/application/agentthread/runtime_file_registry.go`
- Modify: `backend/application/agentthread/adk_offload_backend.go`

- [x] **Step 1: Add repository lookup**

Add `GetRuntimeFile(ctx, runID, virtualPath)` for exact `(run_id,
virtual_path)` lookup.

- [x] **Step 2: Add domain resolve request**

Add `ResolveRuntimeFile` to the runtime file service. It must verify request
scope, virtual path, stored row scope, workspace kind, active status, object
URI, size, digest, and metadata.

- [x] **Step 3: Add application registry resolve**

Expose the domain resolve method through `ApplicationADKRuntimeFileRegistry`
and return the resolved object URI.

- [x] **Step 4: Gate ADK reads**

In `ADKOffloadBackend.ReadRange`, resolve the runtime file before object
storage access and read using the service-returned object URI.

### Task 3: Add Contract Coverage

**Files:**
- Modify: `backend/domain/agentthread/service/runtime_file_test.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`
- Modify: `backend/application/agentthread/runtime_file_registry_test.go`
- Modify: `backend/application/agentthread/adk_offload_backend_test.go`

- [x] **Step 1: Domain resolve coverage**

Cover valid resolve plus missing file, scope mismatch, wrong kind, deleted
status, filename mismatch, object URI mismatch, invalid digest, and invalid
metadata.

- [x] **Step 2: Application registry coverage**

Cover mapping of resolve request and returned object URI.

- [x] **Step 3: Repository coverage**

Cover exact lookup after idempotent upsert.

- [x] **Step 4: ADK backend coverage**

Keep historical same-thread readback working after registration and reject
unregistered object reads.

### Task 4: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-runtime-offload-read-gate-design.md`
- Create: `docs/superpowers/plans/2026-06-21-runtime-offload-read-gate-m4.md`

- [x] **Step 1: Document read gate**

State that runtime offload reads must resolve an active registered runtime file
before object storage access.

- [x] **Step 2: Update roadmap**

Add M4.3 completion status and reduce the remaining estimate by one offload
read authorization slice.

### Task 5: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused tests**

Run:

```bash
cd backend && go test ./application/agentthread -run 'TestADKOffloadBackendRejectsUnregisteredRuntimeFileRead|TestADKOffloadBackendWritesRegistersAndReadsHistoricalRun|TestADKReadOffloadToolReturnsBoundedByteRanges' -count=1 -gcflags="all=-l -N"
cd backend && go test ./domain/agentthread/service -run 'TestRuntimeFileServiceResolvesOwnedWorkspaceOffload|TestRuntimeFileServiceRejectsInvalidRuntimeFileResolve|TestRuntimeFileServiceRegistersOwnedWorkspaceOffload|TestRuntimeFileServiceRejectsInvalidWorkspaceOffload' -count=1 -gcflags="all=-l -N"
cd backend && go test ./application/agentthread -run 'TestApplicationADKRuntimeFileRegistry' -count=1 -gcflags="all=-l -N"
cd backend && go test ./domain/agentthread/repository -run 'TestRuntimeFileRepositoryUpsertsByRunAndVirtualPath' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run affected backend tests**

Run:

```bash
cd backend && go test ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread -run 'TestRuntimeFileService|TestRuntimeFileRepository|TestApplicationADKRuntimeFileRegistry|TestADKOffload|TestADKReadOffloadTool|TestADKAgentOffloadsLargeToolResult' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 3: Check formatting and doc placeholders**

Run:

```bash
gofmt -w backend/domain/agentthread/repository/runtime_file.go backend/domain/agentthread/repository/mysql.go backend/domain/agentthread/repository/mysql_test.go backend/domain/agentthread/service/runtime_file.go backend/domain/agentthread/service/runtime_file_test.go backend/application/agentthread/adk_offload_backend.go backend/application/agentthread/adk_offload_backend_test.go backend/application/agentthread/runtime_file_registry.go backend/application/agentthread/runtime_file_registry_test.go
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-runtime-offload-read-gate-design.md docs/superpowers/plans/2026-06-21-runtime-offload-read-gate-m4.md
```
