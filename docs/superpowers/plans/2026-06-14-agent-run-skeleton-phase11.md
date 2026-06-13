# Agent Run Skeleton Phase 11 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first production-shaped canonical run model behind task threads.

**Architecture:** Extend the existing `agentthread` domain instead of creating a parallel runtime package for this slice. Add `agent_runs` persistence, repository methods, domain service methods, and application DTOs for create/get/list. The run is durable and queryable, but it does not yet schedule workers, stream events, or execute tools.

**Tech Stack:** Go, GORM, sqlite tests, Atlas migrations, existing `agentthread` service/application patterns.

---

### Task 1: Run Repository Contract

**Files:**
- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`

- [ ] **Step 1: Write failing repository tests**

Add:
- `TestThreadRepositoryCreateAndGetRun`
- `TestThreadRepositoryListRunsFiltersByThreadAndOrdersNewestFirst`

The create/get test should create a run with production-shaped fields:

```go
run := &entity.Run{
  ID:                100,
  ThreadID:          10,
  SpaceID:           1,
  CreatorID:         2,
  AssistantID:       "default",
  Status:            entity.RunStatusPending,
  Command:           `{"resume":false}`,
  Input:             `{"messages":[{"role":"user","content":"hello"}]}`,
  Config:            `{"mode":"Auto"}`,
  Context:           `{"source":"web"}`,
  Metadata:          `{"trace":"abc"}`,
  StreamMode:        `["messages","updates"]`,
  MultitaskStrategy: "enqueue",
  OnDisconnect:      "continue",
  Durability:        "async",
  IdempotencyKey:    "idem-1",
  WorkerID:          "worker-1",
  ErrorCode:         "tool_failed",
  ErrorMessage:      "tool error",
  StartedAt:         11,
  EndedAt:           22,
  CreatedAt:         33,
  UpdatedAt:         44,
}
```

Assert all fields round-trip.

- [ ] **Step 2: Run repository tests to verify they fail**

```bash
cd backend
go test ./domain/agentthread/repository -run 'TestThreadRepository(CreateAndGetRun|ListRuns)'
```

Expected: FAIL because run fields, PO, and repository methods do not exist.

- [ ] **Step 3: Implement repository methods**

Add to repository:
- `CreateRun(ctx context.Context, run *entity.Run) error`
- `GetRun(ctx context.Context, id int64) (*entity.Run, error)`
- `ListRuns(ctx context.Context, req ListRunsRequest) ([]*entity.Run, int64, error)`

`ListRunsRequest` fields:

```go
type ListRunsRequest struct {
  ThreadID int64
  Status   *entity.RunStatus
  Page     int32
  PageSize int32
}
```

Default run ordering: `created_at DESC, id DESC`.

- [ ] **Step 4: Run repository tests to verify they pass**

```bash
cd backend
go test ./domain/agentthread/repository -run 'TestThreadRepository(CreateAndGetRun|ListRuns)'
```

Expected: PASS.

### Task 2: Run Domain Service

**Files:**
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [ ] **Step 1: Write failing domain service tests**

Add:
- `TestCreateRunRequiresExistingThreadAndInput`
- `TestCreateRunDefaultsStatusAndRuntimeOptions`
- `TestListRunsNormalizesPaging`

`CreateRun` should:
- require non-empty `ThreadID`
- require existing thread via `repo.GetThread`
- require non-empty JSON input string
- generate an ID
- default `Status` to `RunStatusPending`
- default `AssistantID` to `default`
- default `Command` to `{}`
- default `Config`, `Context`, `Metadata` to `{}`
- default `StreamMode` to `["messages","updates"]`
- default `MultitaskStrategy` to `enqueue`
- default `OnDisconnect` to `continue`
- default `Durability` to `async`
- copy `SpaceID` and `CreatorID` from the thread

- [ ] **Step 2: Run domain service tests to verify they fail**

```bash
cd backend
go test ./domain/agentthread/service -run 'Test(CreateRun|ListRuns)'
```

Expected: FAIL because service methods and memory repo methods do not exist.

- [ ] **Step 3: Implement domain service**

Add request structs:
- `CreateRunRequest`
- `GetRunRequest`
- `ListRunsRequest`

Add interface methods:
- `CreateRun`
- `GetRun`
- `ListRuns`

Add validation and defaults in `service_impl.go`.

- [ ] **Step 4: Run domain service tests to verify they pass**

```bash
cd backend
go test ./domain/agentthread/service -run 'Test(CreateRun|ListRuns)'
```

Expected: PASS.

### Task 3: Run Application Boundary

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/thread_app.go`
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `backend/application/agentthread/thread_app_test.go`

- [ ] **Step 1: Write failing application tests**

Add:
- `TestApplicationCreateRunMapsDomainRun`
- `TestApplicationListRunsMapsDomainRuns`

The app DTO should expose `RunSummary` and request/response structs with the same production-shaped fields as the domain entity, using application-level `RunStatus`.

- [ ] **Step 2: Run application tests to verify they fail**

```bash
cd backend
go test ./application/agentthread -run 'Test(Application(CreateRun|ListRuns)|InitServiceBuildsUsableThreadService)'
```

Expected: FAIL because application run DTOs and methods do not exist.

- [ ] **Step 3: Implement application DTO mapping**

Add:
- `RunStatus`
- `RunSummary`
- `CreateRunRequest/Response`
- `GetRunRequest/Response`
- `ListRunsRequest/Response`
- `DomainRunToSummary`
- `CreateRun`
- `GetRun`
- `ListRuns`

Extend `ApplicationService` with the same `ThreadSVC`; no new service dependency is needed in this slice.

- [ ] **Step 4: Run application tests to verify they pass**

```bash
cd backend
go test ./application/agentthread -run 'Test(Application(CreateRun|ListRuns)|InitServiceBuildsUsableThreadService)'
```

Expected: PASS.

### Task 4: Schema Migration

**Files:**
- Add: `docker/atlas/migrations/20260614000200_agent_runs.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`

- [ ] **Step 1: Add migration**

Create `agent_runs` with fields matching the entity and indexes:
- primary key `id`
- `idx_agent_runs_thread_created (thread_id, created_at)`
- `idx_agent_runs_space_status (space_id, status)`
- `uk_agent_runs_space_idempotency (space_id, idempotency_key)`

Use nullable `idempotency_key` so clients without idempotency keys are not deduped together.

- [ ] **Step 2: Update latest schema HCL**

Add the `agent_runs` table block to `docker/atlas/opencoze_latest_schema.hcl`.

- [ ] **Step 3: Regenerate Atlas hash**

```bash
docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate hash --dir file://docker/atlas/migrations
```

Expected: `docker/atlas/migrations/atlas.sum` updates with the new migration hash.

### Task 5: Verification and Commit

**Files:**
- Verify focused backend packages
- Verify diff whitespace

- [ ] **Step 1: Run focused backend tests**

```bash
cd backend
go test -count=1 ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread
```

Expected: PASS.

- [ ] **Step 2: Run whitespace check**

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/plans/2026-06-14-agent-run-skeleton-phase11.md backend/domain/agentthread/entity/thread.go backend/domain/agentthread/repository/repository.go backend/domain/agentthread/repository/mysql.go backend/domain/agentthread/repository/mysql_test.go backend/domain/agentthread/service/service.go backend/domain/agentthread/service/service_impl.go backend/domain/agentthread/service/service_impl_test.go backend/application/agentthread/dto.go backend/application/agentthread/service.go backend/application/agentthread/thread_app.go backend/application/agentthread/service_test.go backend/application/agentthread/thread_app_test.go docker/atlas/migrations/20260614000200_agent_runs.sql docker/atlas/opencoze_latest_schema.hcl docker/atlas/migrations/atlas.sum
git commit -m "feat: add agent run skeleton"
```
