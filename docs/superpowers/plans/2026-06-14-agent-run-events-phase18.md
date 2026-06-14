# Agent Run Events Phase 18 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the durable execution-event backend model for task runs so later phases can render execution flow and stream updates.

**Architecture:** Extend the existing `agentthread` domain instead of creating a parallel event package. Persist events in `agent_run_events`, expose repository/domain/application append and list methods, and keep HTTP/SSE wiring for a later phase.

**Tech Stack:** Go, GORM, Atlas migrations, existing agentthread repository/service/application tests.

---

## Scope

This phase implements:

- `agent_run_events` persistence.
- Repository methods:
  - `CreateRunEvent`
  - `ListRunEvents`
- Domain service methods:
  - `AppendRunEvent`
  - `ListRunEvents`
- Application DTO and mapping methods:
  - `RunEventSummary`
  - `AppendRunEvent`
  - `ListRunEvents`
- Atlas migration and latest schema update.

This phase does not implement:

- HTTP handlers for events.
- SSE/WebSocket streaming.
- Automatic event emission from `RunProcessor` or `HarnessExecutor`.
- Frontend execution-flow UI.
- MCP/tool/skill/memory event payloads.

## Files

- Modify `backend/domain/agentthread/repository/repository.go`: add event repository contract and list request.
- Modify `backend/domain/agentthread/repository/mysql.go`: add `runEventPO`, table mapping, create/list methods, conversion helpers.
- Modify `backend/domain/agentthread/repository/mysql_test.go`: add repository RED/GREEN tests.
- Modify `backend/domain/agentthread/service/service.go`: add event service request types and interface methods.
- Modify `backend/domain/agentthread/service/service_impl.go`: add append/list event service logic.
- Modify `backend/domain/agentthread/service/service_impl_test.go`: add service RED/GREEN tests and memory repo support.
- Modify `backend/application/agentthread/dto.go`: add event DTOs.
- Modify `backend/application/agentthread/thread_app.go`: add event summary mapper.
- Modify `backend/application/agentthread/service.go`: add application append/list methods.
- Modify `backend/application/agentthread/service_test.go`: add application RED/GREEN tests and fake service support.
- Add `docker/atlas/migrations/20260614000300_agent_run_events.sql`.
- Modify `docker/atlas/opencoze_latest_schema.hcl`.
- Modify `docker/atlas/migrations/atlas.sum` by running `make atlas-hash`.
- Add `docs/superpowers/plans/2026-06-14-agent-run-events-phase18.md`.

## Task 1: Repository Tests

**Files:**
- Modify: `backend/domain/agentthread/repository/mysql_test.go`

- [ ] **Step 1: Write failing create/list event test**

Add a test that migrates `runEventPO`, creates events for two runs, and asserts `ListRunEvents`:

- filters by `run_id`;
- returns events ordered by `created_at ASC, id ASC`;
- preserves `thread_id`, `event_type`, `payload`, and `created_at`.

- [ ] **Step 2: Write failing invalid payload test**

Assert that `CreateRunEvent` rejects invalid JSON payloads.

- [ ] **Step 3: Run tests to verify RED**

Run:

```bash
cd backend
go test ./domain/agentthread/repository -run 'TestThreadRepositoryCreateAndListRunEvents|TestThreadRepositoryRejectsInvalidRunEventPayload'
```

Expected: FAIL because event repository methods and PO do not exist.

## Task 2: Repository Implementation

**Files:**
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`

- [ ] **Step 1: Extend repository contract**

Add:

```go
CreateRunEvent(ctx context.Context, event *entity.RunEvent) error
ListRunEvents(ctx context.Context, req ListRunEventsRequest) ([]*entity.RunEvent, int64, error)

type ListRunEventsRequest struct {
	ThreadID int64
	RunID    int64
	Page     int32
	PageSize int32
}
```

- [ ] **Step 2: Add `runEventPO`**

Create a GORM PO for `agent_run_events` with:

- `id`
- `thread_id`
- `run_id`
- `event_type`
- `payload`
- `created_at`

Indexes:

- `idx_agent_run_events_run_created (run_id, created_at)`
- `idx_agent_run_events_thread_created (thread_id, created_at)`

- [ ] **Step 3: Add create/list methods**

`CreateRunEvent` validates payload JSON through the existing JSON helpers. `ListRunEvents` filters by `run_id` when present, otherwise by `thread_id`, normalizes page/page size to `1/100`, and sorts ascending by `created_at, id`.

- [ ] **Step 4: Run repository tests**

Run:

```bash
cd backend
go test ./domain/agentthread/repository -run 'TestThreadRepositoryCreateAndListRunEvents|TestThreadRepositoryRejectsInvalidRunEventPayload'
```

Expected: PASS.

## Task 3: Domain Service Tests and Implementation

**Files:**
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [ ] **Step 1: Write failing service tests**

Add tests for:

- `AppendRunEvent` derives `thread_id` from the existing run, generates an event ID, trims event type, defaults blank payload to `{}`, and stores the event.
- `AppendRunEvent` rejects missing run ID, missing event type, and non-existing run.
- `ListRunEvents` normalizes page/page size to `1/100`.

- [ ] **Step 2: Add service contracts**

Add:

```go
type AppendRunEventRequest struct {
	ThreadID  int64
	RunID     int64
	EventType string
	Payload   string
}

type ListRunEventsRequest struct {
	ThreadID int64
	RunID    int64
	Page     int32
	PageSize int32
}
```

and interface methods:

```go
AppendRunEvent(ctx context.Context, req *AppendRunEventRequest) (*entity.RunEvent, error)
ListRunEvents(ctx context.Context, req *ListRunEventsRequest) ([]*entity.RunEvent, int64, error)
```

- [ ] **Step 3: Implement service methods**

`AppendRunEvent` should call `GetRun` to ensure the run exists, derive thread ID from the run unless the request supplies a matching one, generate an ID, and persist.

`ListRunEvents` should require either `run_id` or `thread_id`, normalize paging, and call the repository.

- [ ] **Step 4: Run service tests**

Run:

```bash
cd backend
go test ./domain/agentthread/service -run 'TestAppendRunEvent|TestListRunEvents'
```

Expected: PASS.

## Task 4: Application Tests and Implementation

**Files:**
- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/thread_app.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/service_test.go`

- [ ] **Step 1: Write failing application tests**

Add tests for:

- `ApplicationService.AppendRunEvent` maps request fields to domain service and maps response to `RunEventSummary`.
- `ApplicationService.ListRunEvents` maps page request and total/events response.

- [ ] **Step 2: Add DTOs and mapper**

Add `RunEventSummary`, append/list request/response DTOs, and `DomainRunEventToSummary`.

- [ ] **Step 3: Add application methods**

Add `AppendRunEvent` and `ListRunEvents` methods, matching existing application service patterns.

- [ ] **Step 4: Run application tests**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestApplicationAppendRunEvent|TestApplicationListRunEvents'
```

Expected: PASS.

## Task 5: Schema

**Files:**
- Add: `docker/atlas/migrations/20260614000300_agent_run_events.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`

- [ ] **Step 1: Add migration**

Create `agent_run_events` with columns and indexes matching `runEventPO`.

- [ ] **Step 2: Update latest schema**

Add the `agent_run_events` table block to `opencoze_latest_schema.hcl`.

- [ ] **Step 3: Rehash Atlas**

Run:

```bash
make atlas-hash
```

Expected: `docker/atlas/migrations/atlas.sum` is updated with `20260614000300_agent_run_events.sql`.

## Task 6: Final Verification and Commit

**Files:**
- All files above.

- [ ] **Step 1: Run focused verification**

Run:

```bash
cd backend
go test -count=1 ./application/agentthread ./domain/agentthread/service ./domain/agentthread/repository
```

Expected: PASS.

- [ ] **Step 2: Run whitespace check**

Run:

```bash
git diff --check
git diff --cached --check
```

Expected: no output and exit code 0.

- [ ] **Step 3: Commit**

Run:

```bash
git add docs/superpowers/plans/2026-06-14-agent-run-events-phase18.md \
  backend/domain/agentthread/repository/repository.go \
  backend/domain/agentthread/repository/mysql.go \
  backend/domain/agentthread/repository/mysql_test.go \
  backend/domain/agentthread/service/service.go \
  backend/domain/agentthread/service/service_impl.go \
  backend/domain/agentthread/service/service_impl_test.go \
  backend/application/agentthread/dto.go \
  backend/application/agentthread/thread_app.go \
  backend/application/agentthread/service.go \
  backend/application/agentthread/service_test.go \
  docker/atlas/migrations/20260614000300_agent_run_events.sql \
  docker/atlas/opencoze_latest_schema.hcl \
  docker/atlas/migrations/atlas.sum
git commit -m "feat: add agent run events"
```

Expected: commit succeeds.

## Self-Review

- Spec coverage: This plan adds the execution-event fact source needed before stream/UI phases.
- Scope control: No HTTP route, frontend, SSE/WebSocket, MCP, skill, memory, or token usage work is included.
- Type consistency: `RunEvent` uses the existing domain entity and mirrors thread/message/run application mapping patterns.
