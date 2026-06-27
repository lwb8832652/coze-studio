# Artifact Physical Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a safe backend cleanup pass for soft-deleted task artifacts after a retention window.

**Architecture:** The repository lists cleanup candidates by joining deleted artifacts to active backing files, then marks the file deleted by expected `file_id + object_uri`. The application service performs object storage deletion first and updates database state only after the object delete succeeds or reports not found.

**Tech Stack:** Go, GORM, existing `agent_artifacts` / `agent_files` tables, existing object storage `DeleteObject`.

---

### Task 1: Domain Cleanup Candidate Contract

**Files:**
- Modify: `backend/domain/agentthread/repository/artifact.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`
- Modify: `backend/domain/agentthread/service/artifact.go`
- Modify: `backend/domain/agentthread/service/artifact_test.go`

- [x] **Step 1: Write the failing repository test**

Add a test that inserts one active artifact, one recently deleted artifact, one expired deleted artifact with an active file, and one expired deleted artifact whose backing file is already deleted. Assert only the expired deleted artifact with an active file is listed.

- [x] **Step 2: Run repository red test**

Run: `go test ./domain/agentthread/repository -run TestArtifactRepositoryListDeletedCleanupCandidates -count=1 -gcflags="all=-l -N"`

Expected: compile or test failure because cleanup candidate methods do not exist.

- [x] **Step 3: Implement repository methods**

Add `ListDeletedArtifactCleanupCandidates` and `MarkArtifactFileDeleted` to the artifact repository interface and MySQL implementation. The list query must require `agent_artifacts.deleted_at > 0`, `agent_artifacts.deleted_at <= cutoff`, and `agent_files.status = active`.

- [x] **Step 4: Run repository green test**

Run: `go test ./domain/agentthread/repository -run TestArtifactRepositoryListDeletedCleanupCandidates -count=1 -gcflags="all=-l -N"`

Expected: pass.

- [x] **Step 5: Add domain service pass-through test**

Add a service test proving invalid cutoff/limit are bounded and that mark-file-deleted passes expected object URI through to the repository.

- [x] **Step 6: Run domain service tests**

Run: `go test ./domain/agentthread/service -run 'TestArtifactService.*Cleanup' -count=1 -gcflags="all=-l -N"`

Expected: pass.

### Task 2: Application Cleanup Pass

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/service_test.go`

- [x] **Step 1: Write the failing application test**

Add a test where an expired deleted artifact candidate points at an object. Assert `ProcessDeletedArtifactCleanup` calls object storage `DeleteObject`, marks the file deleted, returns a metadata-only result, and emits an audit event without object URI or virtual path.

- [x] **Step 2: Run application red test**

Run: `go test ./application/agentthread -run TestApplicationProcessDeletedArtifactCleanup -count=1 -gcflags="all=-l -N"`

Expected: compile or test failure because the application method does not exist.

- [x] **Step 3: Implement the application method**

Add `ProcessDeletedArtifactCleanupRequest` and response types. The method clamps limit, computes cutoff from `NowMillis - RetentionMillis`, lists candidates, deletes object storage keys through a `DeleteObject` capability, marks files deleted only after object deletion succeeds, treats `storage.ErrObjectNotFound` as success, and records bounded errors without raw object keys.

- [x] **Step 4: Run application green test**

Run: `go test ./application/agentthread -run TestApplicationProcessDeletedArtifactCleanup -count=1 -gcflags="all=-l -N"`

Expected: pass.

### Task 3: Docs And Roadmap

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- Modify: `docs/superpowers/plans/2026-06-22-artifact-physical-cleanup-m4.md`

- [x] **Step 1: Update AGENTS**

Document cleanup eligibility, idempotency, file-status marking, and no object URI leakage.

- [x] **Step 2: Update master roadmap**

Add M4.35 complete once verified and reduce the independently testable remaining task estimate by one.

- [x] **Step 3: Run verification**

Run:

```bash
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread -count=1 -gcflags="all=-l -N"
```

Run:

```bash
/opt/homebrew/bin/atlas migrate validate --dir file://docker/atlas/migrations
```

Run:

```bash
git diff --check
```

Expected: all commands exit 0.
