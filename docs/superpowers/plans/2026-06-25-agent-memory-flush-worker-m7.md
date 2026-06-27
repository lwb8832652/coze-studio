# Agent Memory Flush Worker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for each behavior change.

**Goal:** Add the M7.2b worker processing boundary for queued memory flush jobs:
claim jobs, load the transcript snapshot, invoke a configured extractor, write
safe structured facts through existing memory APIs, and complete/retry/fail the
job through lease-aware transitions.

**Architecture:** Keep Coze as the system of record. `ProcessMemoryFlushJobs`
is an application-layer orchestrator modelled after artifact scan processing.
It depends on a configured `MemoryExtractor` interface; the worker must not
invent natural-language facts with heuristics. Extracted facts are persisted
through `RememberMemory`, use stable transcript-derived source IDs, and apply a
bounded app-layer idempotency check before write.

**Tech Stack:** Go, existing AgentThread service/repository, existing memory
flush job lease lifecycle.

---

### Task 1: Application Processing Contract

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Add: `backend/application/agentthread/memory_flush_processor.go`
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/application/workbench/test_fakes_test.go`

- [x] **Step 1: Write failing tests**

Add tests proving claimed memory flush jobs load transcript snapshots, pass safe
snapshot metadata to a configured extractor, write returned facts with stable
`transcript_summary` source IDs, complete the job, and retry extractor failures
with bounded sanitized errors.

- [x] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestApplicationProcessMemoryFlushJobs' -count=1
```

Expected: build failure because the processing API, extractor contract, and
test fakes do not exist yet.

- [x] **Step 3: Implement processor**

Add `MemoryExtractor`, `MemoryExtractionRequest`, `MemoryExtractionFact`,
`ProcessMemoryFlushJobs`, snapshot loading through domain service, app-layer
source ID idempotency, fact writes through `RememberMemory`, lease-aware
complete/retry/fail handling, and content-free lifecycle events.

- [x] **Step 4: Verify GREEN**

Run the focused command again and expect PASS.

### Task 2: Worker Entrypoint

**Files:**
- Modify: `backend/application/agentthread/worker.go`
- Modify: `backend/application/agentthread/worker_test.go`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] **Step 1: Add failing worker tests**

Prove `MemoryFlushWorker.RunOnce` delegates to the application processor, and
env startup stays disabled unless the worker is enabled and a memory extractor
is configured.

- [x] **Step 2: Implement worker**

Add `AGENT_MEMORY_FLUSH_WORKER_*` env configuration, default batch/lease/retry
settings, `RunOnce`, `Start`, and `StartMemoryFlushWorkerFromEnv`.

- [x] **Step 3: Verify worker tests**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestApplicationProcessMemoryFlushJobs|TestMemoryFlushWorker' -count=1
```

Expected: all focused processor and worker tests pass.

### Remaining Work

- Plug an Eino/model-backed production `MemoryExtractor` into the interface.
- Add DB-level unique upsert semantics for high-concurrency source IDs if
  multiple workers or retries can race on the same fact.
- Add memory management UI, audit views, and browser E2E in later M7 slices.
