# Reliable Notification Core Producers Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use subagent-driven-development to implement this plan task-by-task.

**Goal:** Publish reliable notifications from AgentThread, scheduled-task, and workspace lifecycle transactions.

**Architecture:** Each domain repository accepts a bounded notification outbox intent and appends it on the same transaction as the authoritative state transition. Application services calculate recipient policy and display metadata before entering the repository.

**Tech Stack:** Go, GORM/Gen Query, MySQL, Mockey, existing domain services.

### Task 1: AgentThread terminal events

**Files:**
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service_impl.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/runner.go`
- Test: `backend/domain/agentthread/repository/mysql_test.go`
- Test: `backend/domain/agentthread/service_impl_test.go`
- Test: `backend/application/agentthread/runner_test.go`

**Steps:**
1. Extend `FinalizeRunSuccess`, `UpdateRunStatus`, `RequestRunCancellation`, and `ReconcileExpiredRunLease` inputs with an optional validated outbox intent.
2. Append outbox rows in the same transaction as run status, messages, run events, checkpoint, and title persistence.
3. Derive event idempotency from the durable run event ID plus terminal transition.
4. Publish success, final failure, and explicit cancellation only when the state transition actually changed.
5. Suppress AgentThread terminal notifications for runs carrying scheduled-task origin metadata.
6. Add transaction rollback and replay tests proving no duplicate notifications.

### Task 2: Exact awaiting-input classification

**Files:**
- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/application/agentthread/runner.go`
- Modify: `backend/application/agentthread/service.go`
- Test: `backend/application/agentthread/runner_test.go`
- Test: `backend/domain/agentthread/state_machine_test.go`

**Steps:**
1. Add a durable, bounded interaction reference for user-input interruptions.
2. Publish `task.awaiting_input` only when the interrupt contains that interaction reference.
3. Use the interaction event ID for idempotency.
4. Do not publish for lease recovery, multitask rollback, cancellation, or generic interrupted status.
5. Add tests for each excluded interruption source.

### Task 3: Scheduled-task execution finalization

**Files:**
- Modify: `backend/domain/scheduledtask/repository/repository.go`
- Modify: `backend/domain/scheduledtask/repository/mysql.go`
- Modify: `backend/application/scheduledtask/dispatcher.go`
- Modify: `backend/application/scheduledtask/worker.go`
- Test: `backend/domain/scheduledtask/repository/mysql_test.go`
- Test: `backend/application/scheduledtask/worker_test.go`

**Steps:**
1. Replace split `FinishExecution` and `RecordTaskExecution` writes with one `FinalizeExecution` transaction.
2. Compare-and-set the execution terminal state, update task latest status/count/time, and append outbox atomically.
3. Add explicit cancellation finalization if the scheduler already exposes cancellation; otherwise keep cancellation out of the public event taxonomy until the state exists.
4. Notify the schedule owner on completed and final failed execution.
5. Use execution ID plus terminal state for idempotency.
6. Add tests for duplicate callbacks, stale workers, rollback, and AgentThread duplicate suppression.

### Task 4: Workspace membership transactions

**Files:**
- Modify: `backend/domain/user/repository/repository.go`
- Modify: `backend/domain/user/internal/dal/space_user.go`
- Modify: `backend/domain/user/service/user_impl.go`
- Modify: `backend/application/workspace/workspace.go`
- Test: `backend/domain/user/service/space_test.go`
- Test: `backend/application/workspace/workspace_test.go`

**Steps:**
1. Add transactional repository operations for member add, role change, member removal, and ownership transfer.
2. Append workspace outbox events in the same transaction as membership mutation.
3. Derive recipients from the affected member plus current owner/admin policy, never from client-submitted IDs.
4. Do not expose personal-space membership events as team invitation notifications.
5. Use the durable membership row/event epoch for idempotency so role transitions can repeat safely over time.
6. Add permission, tenant isolation, replay, and rollback tests.
