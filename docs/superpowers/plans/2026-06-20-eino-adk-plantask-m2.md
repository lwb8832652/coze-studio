# Eino ADK Durable Plan Task M2 Implementation Plan

**Goal:** Expose Eino `plantask` tools through the Go-native Agent Harness while
keeping Coze as the durable, tenant-owned system of record for execution plans.

**Architecture:** A normalized plan store owns one high-watermark and a set of
plan items for each plan-scope run. A Coze adapter translates Eino's
`.highwatermark` and `{id}.json` backend protocol into transactional repository
operations. Fresh runs use their own plan scope; resume runs reuse the source
run's plan scope while emitting new events against the active run. Completed
items are archived from Eino after its automatic cleanup but remain durable for
history and task-detail rendering.

**Tech Stack:** Go 1.24, Eino `v0.9.9` ADK `plantask`, GORM, MySQL/Atlas,
React 18, TypeScript, Vitest.

---

## File Structure

- Create `backend/domain/agentthread/entity/plan.go` for plan and plan-item
  entities and statuses.
- Create `backend/domain/agentthread/repository/plan.go` for the dedicated
  repository contract.
- Modify `backend/domain/agentthread/repository/mysql.go` for plan persistence,
  high-watermark compare-and-swap, item upsert, archive, and list/read methods.
- Create `backend/domain/agentthread/service/plan.go` for run ownership,
  validation, and transactional mutations.
- Create focused repository and service tests.
- Create `backend/application/agentthread/adk_plan_backend.go` for the Eino
  backend adapter and safe plan events.
- Create `backend/application/agentthread/adk_plan_backend_test.go` for backend
  protocol, concurrency, archival, event, and tenancy contracts.
- Modify `backend/application/agentthread/adk_middleware.go` to register Eino
  `plantask` at a fixed middleware position.
- Modify `backend/application/agentthread/adk_executor.go` and `dto.go` to carry
  an internal plan-scope run ID across resume.
- Modify `backend/application/agentthread/init.go`, `service.go`, and
  `backend/application/application.go` for production wiring.
- Add an Atlas migration and schema entries for plan headers and items.
- Modify task-detail event projection and tests for `plan.task.*` events.
- Update `AGENTS.md` and the DeerFlow parity roadmap after verification.

## Task 1: Durable Plan Domain

- [x] Add failing service tests proving:
  - persisted run ownership supplies space, thread, and user IDs;
  - a missing plan header behaves as high-watermark zero;
  - high-watermark writes are compare-and-swap reservations;
  - duplicate reservations from independent workers cannot allocate one ID;
  - task IDs, statuses, JSON arrays, metadata, and text sizes are bounded;
  - archived items remain queryable for history but disappear from active
    Eino listings.
- [x] Define:

```go
type AgentRunPlan struct {
    RunID         int64
    ThreadID      int64
    SpaceID       int64
    UserID        int64
    HighWatermark int64
    Revision      int64
}

type AgentRunPlanItem struct {
    ID          int64
    RunID       int64
    TaskID      int64
    Subject     string
    Description string
    Status      AgentRunPlanItemStatus
    ActiveForm  string
    Owner       string
    Blocks      string
    BlockedBy   string
    Metadata    string
    Active      bool
    Version     int64
}
```

- [x] Add a separate `PlanRepository`; do not widen `ThreadRepository`.
- [x] Implement transactional high-watermark compare-and-swap and item
  mutations. Repository methods must work correctly across API/worker
  replicas, not only under an in-process mutex.
- [x] Add migration tables:
  - `agent_run_plans`, unique by `run_id`;
  - `agent_run_plan_items`, unique by `(run_id, task_id)`;
  - active-list and thread-history indexes.
- [x] Run focused domain and repository tests.

## Task 2: Coze-Backed Eino Backend

- [x] Add failing adapter tests for the only accepted paths:
  - `/plans/.highwatermark`;
  - `/plans/{positive-decimal-id}.json`.
- [x] Reject traversal, backslashes, control characters, extra segments,
  malformed IDs, unsupported offsets, and oversized content.
- [x] Implement `LsInfo`, `Read`, `Write`, and `Delete`:
  - `LsInfo` returns the watermark plus active task files;
  - `Read` returns canonical Eino task JSON;
  - watermark `Write` performs compare-and-swap from `next - 1` to `next`;
  - task `Write` validates that path ID equals JSON ID and upserts the item;
  - `Delete` archives the item instead of physically deleting it.
- [x] Preserve completed status when Eino automatically deletes all task files
  after the final completion. Explicit deletion of a non-completed item stores
  `deleted`.
- [x] Emit only after successful mutations:
  - `plan.task.created`;
  - `plan.task.updated`;
  - `plan.task.completed`;
  - `plan.task.deleted`.
- [x] Keep event payload bounded and safe: plan scope ID, plan task ID, subject,
  status, active form, owner, dependencies, revision, and aggregate counts.
  Do not include arbitrary metadata or full descriptions.
- [x] Run adapter tests including 20 concurrent task-create attempts across
  independent backend instances.

## Task 3: Eino Middleware And Resume Semantics

- [x] Add `ADKMiddlewarePlanTask` after skill/tool discovery and before context
  budgeting, policy, audit, and usage wrappers.
- [x] Add an optional `ADKPlanBackendFactory`. When absent, return a no-op
  middleware and expose no plan tools. When configured, fail closed on an empty
  backend or Eino middleware construction error.
- [x] Configure `plantask.New` with base directory `/plans` and expose exactly:
  - `TaskCreate`;
  - `TaskGet`;
  - `TaskUpdate`;
  - `TaskList`.
- [x] Do not enable DeepAgent `write_todos` as a second plan source of truth.
- [x] Add internal-only `RunSummary.PlanScopeRunID`. Default it to `RunID`.
- [x] On resume, build the Agent with `PlanScopeRunID = SourceRunID` while
  retaining the current run for checkpoint writes, usage, cancellation, and
  emitted event attribution.
- [x] Add executor contract tests for fresh runs, same-run resume, source-run
  resume, and invalid source scope.
- [x] Wire the plan service and backend factory in production.

## Task 4: Task Detail Projection

- [x] Add helper tests for `plan.task.created`, `updated`, `completed`, and
  `deleted`.
- [x] Project plan events as structured execution steps with stable Chinese
  labels while preserving the product term “任务”.
- [x] Deduplicate plan events by `plan_task_id` before computing visible plan
  progress so repeated updates do not inflate completion totals.
- [x] Keep generic run/tool/step events in the existing execution flow. Do not
  add a second frontend source of truth or a new menu named “对话”.
- [x] Run the focused Vitest package.

## Task 5: Verification And Documentation

- [x] Run focused Go tests for plan domain, repository, middleware, executor,
  and event contracts.
- [x] Run race coverage for the new plan packages.
- [x] Run:

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./...
```

- [x] Run focused frontend tests.
- [x] Update `AGENTS.md` with the durable plan ownership and resume rules.
- [x] Mark M2 item 10 complete in the DeerFlow parity roadmap with evidence.

## Deferred Follow-Up

- A dedicated plan-history/list API and richer collapsible plan panel may be
  added when task-detail APIs are consolidated. Durable plan rows and run
  events are the source contracts for that UI; this milestone does not create
  an overlapping public CRUD surface.
- Eino `planexecute` remains opt-in for specialized agents. It is not installed
  as the universal lead-agent runtime.
