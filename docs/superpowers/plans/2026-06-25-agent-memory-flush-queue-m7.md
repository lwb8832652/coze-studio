# Agent Memory Flush Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for each behavior change.

**Goal:** Turn memory flush jobs from idempotent enqueue records into a durable
worker-claimable async queue with retry and failed terminal handling.

**Architecture:** Reuse `agent_memory_flush_jobs` as the queue. Add worker lease
metadata and repository/service methods modelled after artifact scan jobs:
pending jobs can be claimed, expired processing jobs can be reclaimed, and
terminal/retry transitions require worker ID plus an active lease.

**Tech Stack:** Go, GORM, Atlas migration, existing AgentThread memory flush
job service.

---

### Task 1: Queue Contract

**Files:**
- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [x] **Step 1: Write failing tests**

Add tests for due pending claim, expired processing reclaim, active worker
completion, retry-to-pending, failed terminal handling, worker mismatch denial,
worker ID trimming, default limits, and lease TTL projection.

- [x] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./domain/agentthread/service ./domain/agentthread/repository -run 'TestThreadRepositoryClaimMemoryFlushJobsMarksDuePendingProcessing|TestThreadRepositoryFinishMemoryFlushJobUsesWorkerLease|TestClaimMemoryFlushJobsNormalizesWorkerLeaseAndLimit|TestFinishMemoryFlushJobsValidateWorkerAndDelegate' -count=1
```

Expected: build failure because queue worker fields and methods do not exist.

- [x] **Step 3: Implement queue lifecycle**

Add worker lease fields, repository claim/complete/retry/fail methods, bounded
error text, and domain service validation/defaults.

- [x] **Step 4: Verify GREEN**

Run the focused command again and expect PASS.

### Task 2: Schema And Context

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/thread_app.go`
- Modify: `backend/application/agentthread/adk_transcript_test.go`
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `backend/application/workbench/test_fakes_test.go`
- Add: `docker/atlas/migrations/20260625000200_agent_memory_flush_worker_leases.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Preserve safe job metadata through summaries**

Expose worker lease timestamps and worker ID only as job lifecycle metadata.

- [x] **Step 2: Add Atlas migration**

Add worker lease columns and a worker ID index with backward-compatible
defaults.

- [x] **Step 3: Refresh Atlas checksum with local Atlas**

Run:

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

Expected: local `atlas v0.35.0` exits 0.

- [x] **Step 4: Verify packages**

Run:

```bash
cd backend
go test ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread ./application/workbench -count=1
```

Expected: all listed packages pass.
