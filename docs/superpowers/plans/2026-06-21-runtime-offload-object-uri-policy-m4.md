# M4.2 Runtime Offload Object URI Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce deterministic object-storage scope when runtime offload files
are registered.

**Architecture:** Add strict object URI parsing inside
`backend/domain/agentthread/service/runtime_file.go`. Keep the ADK offload
backend as the object-key generator, while the domain service becomes the
second defensive gate before persisting `agent_files`.

**Tech Stack:** Go, domain service tests.

---

### Task 1: Add Red Tests

**Files:**
- Modify: `backend/domain/agentthread/service/runtime_file_test.go`

- [x] **Step 1: Add object URI helper**

Add a fixture helper for object URIs using:

```text
agent-runtime/{space_id}/{thread_id}/runs/{run_id}/tool-results/{phase}/{file_name}
```

- [x] **Step 2: Add invalid object URI cases**

Add invalid cases for:

```text
object space mismatch
object thread mismatch
object run mismatch
object phase mismatch
object filename mismatch
object key with backslash
```

- [x] **Step 3: Verify red**

Run:

```bash
cd backend && go test ./domain/agentthread/service -run 'TestRuntimeFileServiceRegistersOwnedWorkspaceOffload|TestRuntimeFileServiceRejectsInvalidWorkspaceOffload' -count=1 -gcflags="all=-l -N"
```

Expected red result: the new object URI mismatch cases are accepted by the
current clean-relative-path validation.

### Task 2: Implement Scoped Object URI Parsing

**Files:**
- Modify: `backend/domain/agentthread/service/runtime_file.go`

- [x] **Step 1: Add parser**

Add a parser that rejects empty object URIs, URL schemes, absolute paths,
backslashes, control characters, unclean paths, unsupported phases,
non-positive IDs, and non-lowercase 64-character hex file names.

- [x] **Step 2: Enforce virtual path agreement**

In `RegisterRuntimeFile`, parse `ObjectURI` and reject when object run ID,
phase, or filename differs from the validated virtual path.

- [x] **Step 3: Enforce authoritative run scope**

After loading the run row, reject when object space ID, thread ID, or run ID
differs from the run row.

- [x] **Step 4: Preserve content digest semantics**

Keep `Digest` as the content SHA-256 validation. Do not compare it to the
path/object filename because the filename is the stable offload identity from
the tool-call context.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-runtime-offload-object-uri-policy-design.md`
- Create: `docs/superpowers/plans/2026-06-21-runtime-offload-object-uri-policy-m4.md`

- [x] **Step 1: Document registration gate**

State that runtime file registration must enforce object-key scope in addition
to virtual path policy.

- [x] **Step 2: Update roadmap**

Add M4.2 completion status and reduce the remaining estimate by one runtime
file policy slice.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused domain tests**

Run:

```bash
cd backend && go test ./domain/agentthread/service -run 'TestRuntimeFileServiceRegistersOwnedWorkspaceOffload|TestRuntimeFileServiceRejectsInvalidWorkspaceOffload' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run affected backend tests**

Run:

```bash
cd backend && go test ./domain/agentthread/service ./application/agentthread -run 'TestRuntimeFileService|TestApplicationADKRuntimeFileRegistry|TestADKOffload' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 3: Check formatting and doc placeholders**

Run:

```bash
gofmt -w backend/domain/agentthread/service/runtime_file.go backend/domain/agentthread/service/runtime_file_test.go
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-runtime-offload-object-uri-policy-design.md docs/superpowers/plans/2026-06-21-runtime-offload-object-uri-policy-m4.md
```
