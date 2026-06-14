# Agent Run Claim Phase 13 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the durable run lifecycle primitives needed for a Go-native Agent worker to claim pending task-thread runs and mark them terminal.

**Architecture:** Keep the lifecycle inside the existing `agentthread` bounded context. Add a run state machine, repository methods for claiming pending runs and compare-and-set status updates, service validation, and application DTOs. This phase intentionally stops before starting background goroutines, streaming events, LLM calls, or tool execution.

**Tech Stack:** Go, GORM, SQLite-backed unit tests, existing `agentthread` domain/application packages.

---

## Scope

This phase implements:

- FIFO claiming of `pending` runs into `running` with `worker_id`, `started_at`, and `updated_at`.
- Compare-and-set terminal updates from `running` to `succeeded`, `failed`, or `canceled`.
- Domain state-machine validation so invalid transitions fail as client errors before touching persistence.
- Application-layer DTOs and mapping for future worker code.

This phase does not implement:

- The Go Agent Harness loop.
- SSE/WebSocket streaming.
- Tool/MCP execution.
- Background worker startup from `application.Init`.
- Retry/backoff and stale lease recovery.

## File Structure

- Modify `backend/domain/agentthread/repository/repository.go`: add claim/update request structs and interface methods.
- Modify `backend/domain/agentthread/repository/mysql.go`: implement transactional claim and conditional status update.
- Modify `backend/domain/agentthread/repository/mysql_test.go`: add repository RED/GREEN tests.
- Create `backend/domain/agentthread/service/state_machine.go`: run transition rules.
- Create `backend/domain/agentthread/service/state_machine_test.go`: state-machine tests.
- Modify `backend/domain/agentthread/service/service.go`: add service request structs and interface methods.
- Modify `backend/domain/agentthread/service/service_impl.go`: add claim/complete/fail/cancel methods.
- Modify `backend/domain/agentthread/service/service_impl_test.go`: add service RED/GREEN tests and memory repo methods.
- Modify `backend/application/agentthread/dto.go`: add claim/update request/response DTOs.
- Modify `backend/application/agentthread/service.go`: expose application methods.
- Modify `backend/application/agentthread/service_test.go`: add mapping tests and mock methods.

## Task 1: Repository Lifecycle Operations

**Files:**
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Test: `backend/domain/agentthread/repository/mysql_test.go`

- [ ] **Step 1: Write failing repository tests**

Add tests that:

```go
func TestThreadRepositoryClaimPendingRunsMarksOldestRunsRunning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	require.NoError(t, repo.CreateRun(context.Background(), newRepositoryTestRun(1, 10, entity.RunStatusPending, 100)))
	require.NoError(t, repo.CreateRun(context.Background(), newRepositoryTestRun(2, 10, entity.RunStatusPending, 101)))
	require.NoError(t, repo.CreateRun(context.Background(), newRepositoryTestRun(3, 10, entity.RunStatusRunning, 99)))

	claimed, err := repo.ClaimPendingRuns(context.Background(), ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, int64(1), claimed[0].ID)
	require.Equal(t, entity.RunStatusRunning, claimed[0].Status)
	require.Equal(t, "worker-a", claimed[0].WorkerID)
	require.NotZero(t, claimed[0].StartedAt)
	got, err := repo.GetRun(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusRunning, got.Status)
	require.Equal(t, "worker-a", got.WorkerID)
}

func TestThreadRepositoryUpdateRunStatusUsesExpectedStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runPO{}))

	repo := NewThreadRepository(db)
	run := newRepositoryTestRun(1, 10, entity.RunStatusRunning, 100)
	run.WorkerID = "worker-a"
	require.NoError(t, repo.CreateRun(context.Background(), run))

	require.NoError(t, repo.UpdateRunStatus(context.Background(), UpdateRunStatusRequest{
		RunID:   1,
		From:    entity.RunStatusRunning,
		To:      entity.RunStatusSucceeded,
		WorkerID:"worker-a",
	}))
	got, err := repo.GetRun(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, entity.RunStatusSucceeded, got.Status)
	require.NotZero(t, got.EndedAt)

	err = repo.UpdateRunStatus(context.Background(), UpdateRunStatusRequest{
		RunID:   1,
		From:    entity.RunStatusRunning,
		To:      entity.RunStatusFailed,
		WorkerID:"worker-a",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not in status")
}
```

- [ ] **Step 2: Run repository tests to verify RED**

Run:

```bash
cd backend
go test ./domain/agentthread/repository -run 'TestThreadRepository(ClaimPendingRuns|UpdateRunStatus)'
```

Expected: FAIL because `ClaimPendingRunsRequest`, `UpdateRunStatusRequest`, `ClaimPendingRuns`, and `UpdateRunStatus` do not exist.

- [ ] **Step 3: Implement repository API**

Add to `repository.go`:

```go
ClaimPendingRuns(ctx context.Context, req ClaimPendingRunsRequest) ([]*entity.Run, error)
UpdateRunStatus(ctx context.Context, req UpdateRunStatusRequest) error
```

Implement in `mysql.go`:

- default `Limit` to `10`;
- select `pending` runs ordered by `created_at ASC, id ASC`;
- for non-SQLite dialects use `FOR UPDATE SKIP LOCKED`;
- update each selected row with `status=running`, `worker_id`, `started_at`, `updated_at`;
- update terminal statuses with `WHERE id = ? AND status = ?` and optional `worker_id` guard;
- set `ended_at` when the target status is terminal.

- [ ] **Step 4: Run repository tests to verify GREEN**

Run:

```bash
cd backend
go test ./domain/agentthread/repository -run 'TestThreadRepository(ClaimPendingRuns|UpdateRunStatus)'
```

Expected: PASS.

## Task 2: Domain Service Run State Machine

**Files:**
- Create: `backend/domain/agentthread/service/state_machine.go`
- Create: `backend/domain/agentthread/service/state_machine_test.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Test: `backend/domain/agentthread/service/service_impl_test.go`

- [ ] **Step 1: Write failing state-machine tests**

Create tests that assert:

```go
func TestCanTransitionRunAllowsWorkerLifecycle(t *testing.T) {
	assert.True(t, CanTransitionRun(entity.RunStatusPending, entity.RunStatusRunning))
	assert.True(t, CanTransitionRun(entity.RunStatusRunning, entity.RunStatusSucceeded))
	assert.True(t, CanTransitionRun(entity.RunStatusRunning, entity.RunStatusFailed))
	assert.True(t, CanTransitionRun(entity.RunStatusRunning, entity.RunStatusCanceled))
}

func TestCanTransitionRunBlocksTerminalToRunning(t *testing.T) {
	assert.False(t, CanTransitionRun(entity.RunStatusSucceeded, entity.RunStatusRunning))
	assert.False(t, CanTransitionRun(entity.RunStatusFailed, entity.RunStatusRunning))
}

func TestEnsureRunTransitionReturnsClientError(t *testing.T) {
	err := EnsureRunTransition(entity.RunStatusSucceeded, entity.RunStatusRunning)
	require.Error(t, err)
	assert.True(t, IsClientError(err))
}
```

- [ ] **Step 2: Write failing service tests**

Add tests that:

- `ClaimPendingRuns` requires non-empty `worker_id`;
- service normalizes `Limit` and delegates to repository;
- `CompleteRun` transitions running to succeeded;
- `FailRun` stores `error_code` and `error_message`;
- invalid terminal-to-running transitions return client errors.

- [ ] **Step 3: Run service tests to verify RED**

Run:

```bash
cd backend
go test ./domain/agentthread/service -run 'Test(CanTransitionRun|EnsureRunTransition|ClaimPendingRuns|CompleteRun|FailRun)'
```

Expected: FAIL because the new functions and methods do not exist.

- [ ] **Step 4: Implement service lifecycle**

Add request structs:

```go
type ClaimPendingRunsRequest struct {
	WorkerID string
	Limit    int32
}

type UpdateRunStatusRequest struct {
	RunID        int64
	From         entity.RunStatus
	To           entity.RunStatus
	WorkerID     string
	ErrorCode    string
	ErrorMessage string
}
```

Add service methods:

```go
ClaimPendingRuns(ctx context.Context, req *ClaimPendingRunsRequest) ([]*entity.Run, error)
CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*entity.Run, error)
```

Each status method should validate `RunID`, expected transition, and call repository `UpdateRunStatus`, then return `GetRun`.

- [ ] **Step 5: Run service tests to verify GREEN**

Run:

```bash
cd backend
go test ./domain/agentthread/service -run 'Test(CanTransitionRun|EnsureRunTransition|ClaimPendingRuns|CompleteRun|FailRun)'
```

Expected: PASS.

## Task 3: Application DTOs and Mapping

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/service.go`
- Test: `backend/application/agentthread/service_test.go`

- [ ] **Step 1: Write failing application tests**

Add tests that:

- `ClaimPendingRuns` maps `WorkerID` and `Limit` to the domain service;
- returned domain runs map to `RunSummary` with `running` status and `worker_id`;
- `CompleteRun` maps `running -> succeeded`;
- `FailRun` maps `error_code` and `error_message`.

- [ ] **Step 2: Run application tests to verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestApplication(ClaimPendingRuns|CompleteRun|FailRun)'
```

Expected: FAIL because application DTOs and methods do not exist.

- [ ] **Step 3: Implement application methods**

Add DTOs:

```go
type ClaimPendingRunsRequest struct {
	WorkerID string
	Limit    int32
}

type ClaimPendingRunsResponse struct {
	Runs []*RunSummary
}

type UpdateRunStatusRequest struct {
	RunID        int64
	From         RunStatus
	To           RunStatus
	WorkerID     string
	ErrorCode    string
	ErrorMessage string
}

type UpdateRunStatusResponse struct {
	Run *RunSummary
}
```

Expose:

```go
ClaimPendingRuns(ctx context.Context, req *ClaimPendingRunsRequest) (*ClaimPendingRunsResponse, error)
CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error)
FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error)
CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error)
```

- [ ] **Step 4: Run application tests to verify GREEN**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestApplication(ClaimPendingRuns|CompleteRun|FailRun)'
```

Expected: PASS.

## Task 4: Final Verification and Commit

**Files:**
- All files above.

- [ ] **Step 1: Run focused verification**

Run:

```bash
cd backend
go test -count=1 ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread
```

Expected: PASS.

- [ ] **Step 2: Run whitespace check**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Commit**

Run:

```bash
git add docs/superpowers/plans/2026-06-14-agent-run-claim-phase13.md \
  backend/domain/agentthread/repository/repository.go \
  backend/domain/agentthread/repository/mysql.go \
  backend/domain/agentthread/repository/mysql_test.go \
  backend/domain/agentthread/service/state_machine.go \
  backend/domain/agentthread/service/state_machine_test.go \
  backend/domain/agentthread/service/service.go \
  backend/domain/agentthread/service/service_impl.go \
  backend/domain/agentthread/service/service_impl_test.go \
  backend/application/agentthread/dto.go \
  backend/application/agentthread/service.go \
  backend/application/agentthread/service_test.go
git commit -m "feat: add agent run claiming lifecycle"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: This plan covers the Phase 13 target of durable pending run claiming and status transitions.
- Placeholder scan: No placeholders remain.
- Type consistency: `ClaimPendingRunsRequest`, `UpdateRunStatusRequest`, `RunStatus`, and `RunSummary` names are consistent across repository, domain service, and application layers.
