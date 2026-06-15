# Agent Thread Memory Persistence Phase 25 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist and recall thread/run memory for the Go-native Agent Harness through the `agentthread` domain.

**Architecture:** Extend the existing `agentthread` domain repository and service with a small `agent_thread_memories` table and Remember/Recall methods. Recall remains deterministic and keyword-free in this phase: it returns unexpired memories for the same thread, including global thread memories and the current run memory, ordered by score and recency. The application layer adds a `ThreadMemoryProvider` adapter so `HarnessExecutorOptions.MemoryProvider` can use persisted memories without coupling the harness to database code.

**Tech Stack:** Go, Gorm, Atlas migrations, existing `agentthread` domain/application packages, Go unit tests with `testify/require`.

---

## Scope

- Add `entity.Memory` with thread/run scope metadata.
- Add `CreateMemory` and `ListMemories` to `repository.ThreadRepository`.
- Add `RememberMemory` and `RecallMemories` to `service.ThreadService`.
- Add MySQL/Gorm persistence for `agent_thread_memories`.
- Add Atlas migration and update latest schema snapshot.
- Add application-layer memory summaries and `ThreadMemoryProvider`.
- Wire the default Agent Harness executor to use persisted thread memory.

## Non-Goals

- No vector embeddings or semantic ranking.
- No memory summarization/write-back from completed runs.
- No frontend memory management UI.
- No public HTTP API for memory CRUD.
- No LangGraph API, IM channel, token accounting, or safety scanning.

## Tasks

### Task 1: RED repository tests

**Files:**
- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`

- [ ] Add `TestThreadRepositoryCreateAndListMemories`.
- [ ] Assert list returns unexpired memories for the same thread and current run.
- [ ] Assert ordering is score desc, updated_at desc, id desc.
- [ ] Assert unrelated thread memories, unrelated run memories, and expired memories are excluded.
- [ ] Run `cd backend && go test ./domain/agentthread/repository -run TestThreadRepositoryCreateAndListMemories -count=1`.
- [ ] Expected RED: compile fails because memory entity and repository methods do not exist.

### Task 2: GREEN repository persistence

**Files:**
- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`

- [ ] Add `MemoryScope` constants for `thread`, `run`, and `long_term`.
- [ ] Add `Memory` entity fields: `ID`, `ThreadID`, `RunID`, `SpaceID`, `Scope`, `Content`, `Metadata`, `Score`, `ExpiresAt`, `CreatedAt`, `UpdatedAt`.
- [ ] Add `CreateMemory` and `ListMemories` repository methods.
- [ ] Add `memoryPO`, table name `agent_thread_memories`, conversion helpers, and JSON metadata validation.
- [ ] Run repository test and keep it green.

### Task 3: RED/GREEN domain service

**Files:**
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [ ] Add `TestRememberMemoryCreatesThreadMemory`.
- [ ] Add `TestRecallMemoriesNormalizesLimitAndUsesRunContext`.
- [ ] Implement `RememberMemory` validation: existing thread required, non-empty content required, default scope `thread`, generated id, timestamps.
- [ ] Implement `RecallMemories` validation: thread id required, default limit 8.
- [ ] Run `cd backend && go test ./domain/agentthread/service -run 'Test(RememberMemory|RecallMemories)' -count=1`.

### Task 4: RED/GREEN application provider

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/service_test.go`
- Create: `backend/application/agentthread/memory_provider.go`
- Create: `backend/application/agentthread/memory_provider_test.go`

- [ ] Add `MemorySummary`, `RememberMemoryRequest`, `RecallMemoriesRequest`, and response DTOs.
- [ ] Add app methods that map domain memory to app memory summaries.
- [ ] Add `ThreadMemoryProvider` implementing `MemoryProvider`.
- [ ] Verify provider maps domain memories into `AgentMemory`.
- [ ] Run `cd backend && go test ./application/agentthread -run 'Test(Application.*Memory|ThreadMemoryProvider)' -count=1`.

### Task 5: Migration and wiring

**Files:**
- Create: `docker/atlas/migrations/20260615000100_agent_thread_memories.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `backend/application/application.go`

- [ ] Add `agent_thread_memories` migration.
- [ ] Update latest schema snapshot with the new table.
- [ ] Run `make atlas-hash` if Atlas is available.
- [ ] Wire `NewThreadMemoryProvider(primaryServices.agentThreadSVC, 8)` into `NewHarnessExecutor`.

### Task 6: Verification

**Commands:**
- `cd backend && go test ./domain/agentthread/... ./application/agentthread -count=1`
- `git diff --check`
- `rg "agent_thread_memories|ThreadMemoryProvider|RecallMemories" backend docker/atlas -n`

## Self-Review

- Persistence is thread/run scoped and deterministic.
- Harness still works without a memory provider.
- No memory API or UI is added prematurely.
- No vector ranking is implied by this phase.
