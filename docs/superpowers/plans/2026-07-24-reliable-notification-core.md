# Reliable Notification Core Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use subagent-driven-development to implement this plan task-by-task.

**Goal:** Provide a production-grade notification persistence, outbox worker, authenticated API, and frontend bell implementation without changing existing user-visible entry points.

**Architecture:** The source Thrift contract remains under Playground API because the current frontend already consumes `GetNoticeList`, `GetNoticeUnreadCount`, and `NoticeMarkRead`. Backend domain and infrastructure code are new and independent of Workbench task internals.

**Tech Stack:** Go, Hertz, MySQL, Atlas, Thrift, React, TypeScript, Vitest.

### Task 1: Restore the source API contract

**Files:**
- Modify: `idl/playground/playground.thrift`
- Regenerate: `backend/api/model/playground/playground.go`
- Regenerate: `backend/api/router/playground/playground.go`
- Regenerate: `frontend/packages/arch/idl/src/auto-generated/playground_api/index.ts`
- Regenerate: `frontend/packages/arch/idl/src/auto-generated/playground_api/namespaces/playground.ts`
- Test: `backend/api/handler/coze/notification_service_test.go`

**Steps:**
1. Add `NotificationSeverity`, `NotificationCategory`, `NotificationRouteType`, `Notification`, cursor paging requests/responses, unread-count response, and explicit read operations to the source IDL.
2. Preserve the existing RPC names and HTTP paths used by `PlaygroundApi`.
3. Define mark-read as one request with exactly one mode: `notification_ids` or `mark_all_before`.
4. Use opaque cursor strings and bounded page size with a default of 20 and maximum of 100.
5. Regenerate backend and frontend outputs using the repository’s existing IDL generation command.
6. Add handler contract tests for invalid mixed mark-read modes and page-size bounds.

### Task 2: Add durable schema and domain model

**Files:**
- Create: `docker/atlas/migrations/20260724000100_reliable_notifications.sql`
- Modify: `docker/atlas/migrations/atlas.sum`
- Create: `backend/domain/notification/entity.go`
- Create: `backend/domain/notification/repository.go`
- Create: `backend/domain/notification/service.go`
- Create: `backend/domain/notification/service_test.go`

**Steps:**
1. Create `notification_outbox` with event ID, idempotency key, event type, actor/space/resource fields, bounded payload JSON, status, attempts, availability time, lease owner/time, last error, and timestamps.
2. Create immutable `notification_message` rows keyed by event and display contract.
3. Create `notification_recipient` rows keyed by message and user with `read_at`, delivery state, and timestamps.
4. Add unique constraints for producer idempotency and per-recipient materialization.
5. Add keyset indexes on `(recipient_user_id, created_at, id)` and worker indexes on `(status, available_at, lease_expires_at)`.
6. Implement constructors that reject unsupported event types, unsafe route targets, oversized text/metadata, missing recipients, and credential-like metadata keys.
7. Add unit tests for validation, deterministic idempotency, route allowlisting, and payload redaction.

### Task 3: Implement transactional repository and worker leasing

**Files:**
- Create: `backend/infra/notification/mysql_repository.go`
- Create: `backend/infra/notification/mysql_repository_test.go`
- Create: `backend/application/notification/service.go`
- Create: `backend/application/notification/worker.go`
- Create: `backend/application/notification/worker_test.go`

**Steps:**
1. Implement outbox append on a supplied transaction/query handle so producers can share their business transaction.
2. Implement `ClaimOutboxBatch` with `FOR UPDATE SKIP LOCKED`, lease owner, and lease expiry.
3. Materialize message and recipients transactionally using unique-key idempotency.
4. Implement exponential retry with bounded attempts and terminal dead-letter state.
5. Implement lease-expiry recovery without duplicating user-visible rows.
6. Implement user-scoped keyset list, unread count, mark selected read, and mark all read through a server cutoff.
7. Add tests for duplicate append, concurrent claim, lease recovery, poisoned rows, cursor stability, and concurrent mark-all snapshots.

### Task 4: Wire lifecycle and authenticated handlers

**Files:**
- Create: `backend/api/handler/coze/notification_service.go`
- Create: `backend/api/handler/coze/notification_service_test.go`
- Modify: `backend/api/router/coze/custom_routes.go`
- Modify: `backend/application/application.go`
- Modify: `backend/application/shutdown.go`
- Modify: `backend/main.go`

**Steps:**
1. Resolve the viewer exclusively from authenticated request context.
2. Map domain notifications to generated API types without exposing raw metadata.
3. Register the three Playground notification endpoints.
4. Construct the notification service and worker during application startup.
5. Start the worker with a cancellable context and register a bounded shutdown hook.
6. Return stable application error codes for invalid cursor, invalid read mode, and unauthorized access.
7. Add handler tests proving cross-user IDs cannot be read or marked.

### Task 5: Harden the existing notification bell

**Files:**
- Modify: `frontend/apps/coze-studio/src/components/notification-center/service.ts`
- Modify: `frontend/apps/coze-studio/src/components/notification-center/use-notifications.ts`
- Modify: `frontend/apps/coze-studio/src/components/notification-center/notification-bell.tsx`
- Modify: `frontend/apps/coze-studio/src/components/notification-center/notification-center.module.less`
- Modify: `frontend/apps/coze-studio/src/components/workspace-header-actions.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-header.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Test: `frontend/apps/coze-studio/src/components/notification-center/__tests__/notification-bell.test.tsx`
- Test: `frontend/apps/coze-studio/src/components/notification-center/__tests__/use-notifications.test.tsx`

**Steps:**
1. Remove the module-global permanent 404 disable switch and replace it with retryable request state.
2. Keep unread polling at 30 seconds only while the document is visible and trigger an immediate refresh on visibility restore.
3. Fetch pages only when the popover is open and preserve existing rows during next-page loading.
4. Keep opening the popover read-neutral.
5. Implement optimistic single-read with rollback on failure and explicit mark-all using the server cutoff returned with the list.
6. Render severity, timestamp, unread indicator, bounded summary, loading, empty, error, retry, and pagination states.
7. Navigate only through the server route enum and current workspace context.
8. Preserve the shared top-right notification component across home, workspace, workbench, and task detail headers.
9. Add Vitest coverage for polling visibility, retry after backend recovery, single-read rollback, explicit mark-all, pagination, and unsafe route rejection.
