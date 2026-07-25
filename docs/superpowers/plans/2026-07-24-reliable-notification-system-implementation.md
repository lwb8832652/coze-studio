# Reliable Notification System Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use subagent-driven-development to implement this plan task-by-task.

**Goal:** Replace the current frontend-only notification shell with a durable, tenant-safe notification system that covers user-visible lifecycle changes across tasks, scheduled work, workspace administration, platform services, billing, and system operations.

**Architecture:** Business transactions append immutable domain-notification events to a database outbox. A background worker leases and materializes those events into immutable notification messages plus recipient read-state rows. Producers never call notification delivery synchronously and notification failure never changes the originating business result. All recipient, workspace, authorization, and jump-link data is derived server-side.

**Tech Stack:** Go, Hertz, GORM/Gen Query, MySQL 8, Atlas migrations, Thrift generated API clients, React, TypeScript, Semi/Coze Design, Vitest.

## Delivery order

1. Implement the core contract, schema, repository, worker, API, and existing bell integration from `2026-07-24-reliable-notification-core.md`.
2. Add AgentThread, scheduled-task, and workspace producers from `2026-07-24-reliable-notification-core-producers.md`.
3. Add AppDev, MCP, plugin/resource, and Feishu IM producers from `2026-07-24-reliable-notification-platform-producers.md`.
4. Add billing, credit, subscription, sandbox, and administrator announcement producers from `2026-07-24-reliable-notification-commerce-system.md`.
5. Run the acceptance matrix in this document only after implementation is complete and verification permission has been granted.

## Non-negotiable behavior

- Every notification-producing state transition and its outbox row commit in one database transaction.
- Producer idempotency keys use a durable business event identifier, never status text or wall-clock time.
- Notification delivery is at-least-once; materialization is idempotent and user-visible rows are exactly-once per recipient and event.
- Worker leasing uses bounded batches, lease expiry, exponential retry, terminal dead-letter state, and reconciliation.
- Opening the popover does not mark notifications read.
- Single-item read and explicit mark-all-read are separate APIs.
- Mark-all-read uses a server snapshot cutoff so concurrently created notifications stay unread.
- All list and read APIs bind the authenticated user on the server and reject cross-user access.
- Links are generated from an allowlisted internal route type plus resource ID; arbitrary URLs are never persisted or returned.
- Payloads contain bounded display metadata only. Prompts, completions, tool arguments/results, credentials, object URIs, raw provider responses, and hidden run configuration are forbidden.
- Queueing, running, heartbeat, retry, polling, internal tool/subagent steps, synchronous CRUD, and transient network errors do not generate notifications.
- Skill CRUD remains synchronous in the current product and therefore does not generate lifecycle notifications. Notification producers are added only when a durable asynchronous skill operation exists.

## Notification taxonomy

| Domain | User-visible events |
| --- | --- |
| AgentThread | completed, failed, cancelled, awaiting_input |
| Scheduled task | execution_completed, execution_failed, execution_cancelled |
| Workspace | member_added, member_removed, role_changed, ownership_transferred |
| AppDev | build_completed, build_failed, runtime_failed, runtime_recovered |
| MCP | service_unhealthy, service_recovered |
| Plugin/resource | publish_completed, publish_failed, copy_completed, copy_failed |
| Feishu IM | channel_unhealthy, channel_recovered, inbound_event_dead_lettered |
| Billing | payment_succeeded, payment_failed_final, subscription_activated, subscription_expiring, subscription_expired, credits_adjusted, credits_low |
| System | sandbox_unhealthy, sandbox_recovered, administrator_announcement |

## Shared data contract

Every materialized notification exposes:

- `id`: immutable notification ID.
- `type`: stable machine event type.
- `category`: task, workspace, appdev, mcp, plugin, im, billing, system.
- `severity`: info, success, warning, error.
- `title` and `summary`: bounded localized display copy.
- `space_id`: optional tenant scope derived by the producer.
- `resource_type` and `resource_id`: opaque internal target identity.
- `route_type`: allowlisted internal route enum.
- `created_at`, `read_at`.
- `metadata`: reviewed, size-bounded display-only fields.

## Acceptance matrix

1. A successful task creates one success notification for the task owner and no duplicate after worker retry.
2. A failed task creates one error notification even when all MCP services are disabled.
3. An awaiting-input task creates a warning notification once for the interaction event, not for generic interrupted recovery.
4. A scheduled execution generates only the scheduled-task notification and suppresses the duplicate AgentThread notification.
5. Workspace member and role changes notify only affected users and authorized owners/admins according to the event policy.
6. MCP health flapping below the threshold generates no notification; a stable failure and subsequent recovery each generate one.
7. Feishu transient reconnect attempts generate no notification; a stable channel failure and final event dead-letter do.
8. Payment and subscription terminal transitions generate durable notifications in the same transaction as the business state.
9. Opening the bell leaves unread counts unchanged; explicit read and mark-all update counts correctly.
10. A user cannot list, read, or navigate another user’s notification.
11. A poisoned outbox row retries, becomes dead-lettered, and does not block later rows.
12. Notification worker downtime does not affect business completion and catches up after restart.

## Commit boundaries

1. `feat: add durable notification core`
2. `feat: publish core workflow notifications`
3. `feat: publish platform lifecycle notifications`
4. `feat: publish billing and system notifications`
5. `test: cover reliable notification acceptance flows`
