# M4.4 Artifact Registry Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for implementation and superpowers:verification-before-completion before reporting completion. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first durable artifact registry layer for task detail output
presentation.

**Architecture:** Keep low-level file metadata in `agent_files`. Add
`agent_artifacts` as a presentation registry with repository-level upsert and
list operations. API, frontend, preview, download, scanning, and audit remain
future M4 slices.

**Tech Stack:** Go, GORM, Atlas migrations.

---

### Task 1: Add Red Tests

**Files:**
- Modify: `backend/domain/agentthread/repository/mysql_test.go`

- [x] **Step 1: Cover artifact upsert/list**

Add repository tests for idempotent `file_id` upsert, thread listing, run
filtering, ordering, and row count stability.

- [x] **Step 2: Verify red**

Run:

```bash
cd backend && go test ./domain/agentthread/repository -run 'TestArtifactRepositoryUpsertsByFileAndListsByThread' -count=1 -gcflags="all=-l -N"
```

Expected red result: artifact entity, PO, repository factory, and request types
do not exist yet.

### Task 2: Implement Artifact Repository

**Files:**
- Create: `backend/domain/agentthread/entity/artifact.go`
- Create: `backend/domain/agentthread/repository/artifact.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`

- [x] **Step 1: Add entity**

Add `AgentArtifact` and preview mode constants: `text`, `image`, `pdf`,
`download`, and `unsupported`.

- [x] **Step 2: Add repository contract**

Add `ArtifactRepository` with `UpsertArtifact` and `ListArtifacts`.

- [x] **Step 3: Add MySQL mapping**

Add `agentArtifactPO`, conversion helpers, idempotent upsert by `file_id`, and
thread/run listing.

### Task 3: Add Schema

**Files:**
- Create: `docker/atlas/migrations/20260621000200_agent_artifacts.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`

- [x] **Step 1: Add migration**

Create `agent_artifacts` with the planned columns, thread/run indexes, and
unique `file_id`.

- [x] **Step 2: Update schema HCL**

Mirror the new table in `opencoze_latest_schema.hcl`.

- [x] **Step 3: Refresh Atlas checksum**

Run:

```bash
(cd docker/atlas && /tmp/atlas-v0.35.0 migrate hash)
/tmp/atlas-v0.35.0 migrate validate --dir file://docker/atlas/migrations
```

### Task 4: Update Context

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Create: `docs/superpowers/specs/2026-06-21-artifact-registry-foundation-design.md`
- Create: `docs/superpowers/plans/2026-06-21-artifact-registry-foundation-m4.md`

- [x] **Step 1: Document artifact boundary**

State that `agent_artifacts` is a presentation registry and does not yet imply
preview/download API completion.

- [x] **Step 2: Update roadmap**

Add M4.4 completion status and reduce the remaining estimate by one artifact
registry foundation slice.

### Task 5: Verify

**Files:**
- All files touched above

- [x] **Step 1: Run focused repository test**

Run:

```bash
cd backend && go test ./domain/agentthread/repository -run 'TestArtifactRepositoryUpsertsByFileAndListsByThread' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 2: Run affected repository tests**

Run:

```bash
cd backend && go test ./domain/agentthread/repository -run 'TestRuntimeFileRepository|TestArtifactRepository|TestPlanRepository' -count=1 -gcflags="all=-l -N"
```

- [x] **Step 3: Validate migrations**

Run:

```bash
/tmp/atlas-v0.35.0 migrate validate --dir file://docker/atlas/migrations
```

- [x] **Step 4: Check formatting and doc placeholders**

Run:

```bash
gofmt -w backend/domain/agentthread/entity/artifact.go backend/domain/agentthread/repository/artifact.go backend/domain/agentthread/repository/mysql.go backend/domain/agentthread/repository/mysql_test.go
git diff --check
placeholder_pattern="$(printf 'T%sD|TO%sDO|implement %s|fill in %s|Similar to %s' B '' 'later' 'details' 'Task')"
rg -n "$placeholder_pattern" docs/superpowers/specs/2026-06-21-artifact-registry-foundation-design.md docs/superpowers/plans/2026-06-21-artifact-registry-foundation-m4.md
```
