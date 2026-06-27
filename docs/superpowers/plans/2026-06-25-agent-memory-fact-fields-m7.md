# Agent Memory Fact Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for each behavior change.

**Goal:** Add the M7.1 long-term memory fact metadata foundation without changing
memory extraction, retrieval ranking, or UI management semantics.

**Architecture:** Extend the existing `agent_thread_memories` table and
`entity.Memory` shape instead of creating a parallel memory store. Keep `score`
as the current recall ordering signal; add confidence, source, and correction
metadata as durable fields for later extraction, editing, and audit workflows.

**Tech Stack:** Go, GORM, Atlas migration, existing AgentThread memory service.

---

### Task 1: Fact Field Contract

**Files:**
- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`

- [x] **Step 1: Write failing tests**

Add tests proving long-term memories can persist confidence, source type,
source ID, correction target, and corrected timestamp, and invalid confidence
is rejected.

- [x] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread -run 'TestRememberLongTermMemoryPersistsFactFields|TestRememberMemoryRejectsInvalidConfidence|TestThreadRepositoryCreateAndListMemories|TestApplicationMemoryMethodsMapDomainMemories|TestThreadMemoryProviderRecallsApplicationMemories' -count=1
```

Expected: build failure because the fact fields do not exist yet.

- [x] **Step 3: Implement fields and validation**

Add the fields to domain requests/entities, GORM PO mapping, and service
validation. Keep `long_term` memories thread-level by clearing `run_id` unless
the memory scope is `run`.

- [x] **Step 4: Verify GREEN**

Run the focused command again and expect PASS.

### Task 2: Runtime Mapping And Schema

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/thread_app.go`
- Modify: `backend/application/agentthread/harness.go`
- Modify: `backend/application/agentthread/memory_provider.go`
- Modify: `backend/application/agentthread/resume_runner.go`
- Add: `docker/atlas/migrations/20260625000100_agent_memory_fact_fields.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Preserve metadata through application/runtime summaries**

Map fact fields through application summaries, `ThreadMemoryProvider`,
checkpoint memory values, and resume parsing. Keep prompt injection limited to
scope/content for now.

- [x] **Step 2: Add Atlas migration**

Add columns with backwards-compatible defaults and indexes for source and
correction lookups.

- [x] **Step 3: Refresh Atlas checksum**

Run:

```bash
docker run --rm -v "$PWD":/work -w /work/docker/atlas arigaio/atlas:0.35.0-community-alpine migrate hash
docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations
```

Expected: checksum refreshes and validation exits 0.
