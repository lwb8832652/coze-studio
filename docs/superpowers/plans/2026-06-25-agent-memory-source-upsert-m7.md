# Agent Memory Source Upsert Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for each behavior change.

**Goal:** Add durable source-based idempotency for extracted memory facts so
worker retries and concurrent workers cannot create duplicate sourced facts.

**Architecture:** Keep ordinary user/manual memories unchanged. Only memories
with both `source_type` and `source_id` use source idempotency. The database
adds a nullable generated `source_key` that is populated only for non-empty
source fields, with a unique key on `(thread_id, source_key)`. Multiple memories
with empty source fields continue to be allowed because their generated key is
NULL.

**Tech Stack:** Go, GORM, Atlas migration, existing AgentThread memory service.

---

### Task 1: Repository And Domain Contract

**Files:**
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [x] **Step 1: Write failing tests**

Add tests proving `CreateOrGetMemoryBySource` returns an existing memory for
the same thread/source pair, and `RememberMemory` uses that idempotent path when
source metadata is present.

- [x] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./domain/agentthread/repository ./domain/agentthread/service -run 'TestThreadRepositoryCreateOrGetMemoryBySourceIsIdempotent|TestRememberMemoryUsesSourceIdempotency' -count=1
```

Expected: repository build failure and service behavior failure because the
source-idempotent method does not exist and `RememberMemory` still uses plain
create.

- [x] **Step 3: Implement repository/domain path**

Add `CreateOrGetMemoryBySource`, query-before-create behavior, database
`ON CONFLICT DO NOTHING` fallback, and `RememberMemory` routing for sourced
facts only.

### Task 2: Schema

**Files:**
- Add: `docker/atlas/migrations/20260625000300_agent_memory_source_dedupe.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Add generated source key**

Add nullable generated `source_key` for non-empty `source_type` and `source_id`
only, then add unique key `(thread_id, source_key)`.

- [x] **Step 2: Refresh Atlas checksum**

Run:

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

Expected: local Atlas validation exits 0.
