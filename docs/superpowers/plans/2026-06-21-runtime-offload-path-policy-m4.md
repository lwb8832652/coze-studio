# M4.1 Runtime Offload Path Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce active-run, phase, and digest-file constraints when runtime
offload files are registered.

**Architecture:** Add strict virtual-path parsing inside
`backend/domain/agentthread/service/runtime_file.go`. Keep ADK offload backend
behavior unchanged; the domain service becomes the second defensive gate before
persisting `agent_files`.

**Tech Stack:** Go, domain service tests.

---

### Task 1: Add Red Tests

**Files:**
- Modify: `backend/domain/agentthread/service/runtime_file_test.go`

- [x] **Step 1: Use valid digest path fixture**

Change the happy-path and base invalid-case fixture to use a lowercase
64-character hex filename:

```go
name := strings.Repeat("a", 64) + ".txt"
path := "/mnt/user-data/workspace/.coze/tool-results/runs/20/trunc/" + name
```

- [x] **Step 2: Add invalid path cases**

Add invalid cases for:

```go
path run id does not match req.RunID
phase is "output"
filename is "short.txt"
filename has uppercase hex
```

Each case must keep `FileName` equal to the mutated path base so the failure
comes from path policy, not an existing base-name check.

- [x] **Step 3: Verify red**

Run:

```bash
cd backend && go test ./domain/agentthread/service -run 'TestRuntimeFileServiceRegistersOwnedWorkspaceOffload|TestRuntimeFileServiceRejectsInvalidWorkspaceOffload' -count=1 -gcflags="all=-l -N"
```

Expected red result: the new invalid path cases are accepted by the current
prefix-only path validation.

### Task 2: Implement Strict Path Parsing

**Files:**
- Modify: `backend/domain/agentthread/service/runtime_file.go`

- [x] **Step 1: Add parser**

Add a small parser that rejects empty paths, backslashes, control characters,
unclean paths, unsupported phases, non-positive run IDs, and non-lowercase
64-character hex file names.

- [x] **Step 2: Enforce request run ownership**

In `RegisterRuntimeFile`, parse `VirtualPath` and reject when parsed run ID
does not equal `req.RunID`.

- [x] **Step 3: Preserve existing checks**

Keep existing file kind, file name, object URI, size, digest, metadata, and run
lookup checks unchanged.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-runtime-offload-path-policy-design.md`
- Create: `docs/superpowers/plans/2026-06-21-runtime-offload-path-policy-m4.md`

- [x] **Step 1: Document registration gate**

State that runtime file registration must enforce the same active-run,
`trunc|clear`, and sha256 filename policy as ADK offload path generation.

- [x] **Step 2: Update roadmap**

Add M4.1 completion status and reduce the remaining estimate by one runtime
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
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-runtime-offload-path-policy-design.md docs/superpowers/plans/2026-06-21-runtime-offload-path-policy-m4.md
```
