# M4.5 Artifact Registration Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Register active workspace/output files as durable task artifacts.

**Architecture:** Add a domain service that reads backing file metadata
server-side, validates scope and status, computes a conservative preview mode,
and writes `agent_artifacts` through the repository added in M4.4.

**Tech Stack:** Go domain service tests.

---

### Task 1: Add Red Tests

**Files:**
- Create: `backend/domain/agentthread/service/artifact_test.go`

- [x] **Step 1: Cover successful registration**

Verify an active output file becomes an artifact with copied file metadata,
default title, preview mode, generated ID, and timestamps.

- [x] **Step 2: Cover preview policy**

Verify text, JSON, raster image, PDF, SVG, HTML, and unknown binary content
types map to conservative preview modes.

- [x] **Step 3: Cover invalid inputs**

Verify cross-scope, deleted file, upload file, and invalid metadata inputs are
rejected without writing an artifact.

- [x] **Step 4: Verify red**

Run:

```bash
cd backend && go test ./domain/agentthread/service -run 'TestArtifactService|TestArtifactPreviewMode' -count=1 -gcflags="all=-l -N"
```

Expected red result: artifact service and preview classifier do not exist.

### Task 2: Implement Service

**Files:**
- Create: `backend/domain/agentthread/service/artifact.go`
- Modify: `backend/domain/agentthread/repository/runtime_file.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`
- Modify: `backend/domain/agentthread/service/runtime_file_test.go`

- [x] **Step 1: Add backing file lookup**

Add `GetFileByID` to the file repository so artifact registration loads the
backing file row server-side.

- [x] **Step 2: Add artifact service**

Add `RegisterArtifact` with scope, status, kind, size, metadata, title, and
artifact type validation.

- [x] **Step 3: Add conservative preview classifier**

Map plain text/JSON/CSV to `text`, raster images to `image`, PDF to `pdf`,
and active or unknown content to `download`.

- [x] **Step 4: Preserve repository tests**

Update fakes and repository coverage for `GetFileByID`.

### Task 3: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-artifact-registration-service-design.md`
- Create: `docs/superpowers/plans/2026-06-21-artifact-registration-service-m4.md`

- [x] **Step 1: Document registration boundary**

State that artifact registration must load server-side file metadata and that
preview mode is conservative.

- [x] **Step 2: Update roadmap**

Add M4.5 completion status and reduce the remaining estimate by one artifact
registration service slice.

### Task 4: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused service tests**

Run:

```bash
cd backend && go test ./domain/agentthread/service -run 'TestArtifactService|TestArtifactPreviewMode' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run affected tests**

Run:

```bash
cd backend && go test ./domain/agentthread/service ./domain/agentthread/repository -run 'TestArtifact|TestRuntimeFileRepository|TestRuntimeFileService' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 3: Check formatting and doc placeholders**

Run:

```bash
gofmt -w backend/domain/agentthread/service/artifact.go backend/domain/agentthread/service/artifact_test.go backend/domain/agentthread/service/runtime_file_test.go backend/domain/agentthread/repository/runtime_file.go backend/domain/agentthread/repository/mysql.go backend/domain/agentthread/repository/mysql_test.go
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-artifact-registration-service-design.md docs/superpowers/plans/2026-06-21-artifact-registration-service-m4.md
```
