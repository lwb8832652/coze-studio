# Reliable Notification Commerce and System Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use subagent-driven-development to implement this plan task-by-task.

**Goal:** Add durable billing, credit, subscription, sandbox-health, and administrator-announcement notifications without inventing unsupported business states.

**Architecture:** Existing terminal billing transactions emit outbox rows directly. Missing lifecycle concepts such as final payment failure, subscription expiry scanning, low-credit thresholds, and persistent sandbox incidents are implemented as explicit durable modules before notifications are enabled.

**Tech Stack:** Go, MySQL, billing domain services, sandbox control plane, administrator authorization.

### Task 1: Payment and fulfillment notifications

**Files:**
- Modify: `backend/infra/billing/commerce_repository.go`
- Modify: `backend/domain/billing/commerce_service.go`
- Modify: `backend/application/billing/payment_service.go`
- Test: `backend/infra/billing/commerce_repository_test.go`
- Test: `backend/application/billing/payment_service_test.go`

**Steps:**
1. Append payment-succeeded in the same transaction as `RecordPaymentSucceeded`.
2. Append subscription-activated in the final successful fulfillment transaction.
3. Add a durable final-failure state only where the payment provider exposes an authoritative terminal callback; do not notify transient callback or network failures.
4. Resolve user/workspace recipients from the persisted order subject.
5. Use provider payment event ID and fulfillment event ID for idempotency.
6. Add duplicate webhook, rollback, delayed callback, and subject-isolation tests.

### Task 2: Subscription expiry and order timeout projections

**Files:**
- Modify: `backend/infra/billing/maintenance_repository.go`
- Modify: `backend/application/billing/worker.go`
- Create: `backend/domain/billing/subscription_notification.go`
- Test: `backend/infra/billing/maintenance_repository_test.go`
- Test: `backend/application/billing/worker_test.go`

**Steps:**
1. Replace bulk timeout count updates with candidate selection plus per-row compare-and-set transaction.
2. Add user/workspace subject projection to expiring subscription candidates.
3. Publish expiring notifications once at configured windows and expired once after terminal transition.
4. Persist the last-notified window on the subscription or notification projection to prevent daily duplicates.
5. Keep auto-renew failure out of scope until an authoritative provider state exists.
6. Add clock-boundary, replay, stale-worker, and timezone tests.

### Task 3: Credit adjustments and low-balance episodes

**Files:**
- Modify: `backend/application/billing/maintenance.go`
- Modify: `backend/domain/billing/service.go`
- Create: `backend/domain/billing/credit_threshold.go`
- Modify: `backend/infra/billing/maintenance_repository.go`
- Test: `backend/application/billing/maintenance_test.go`
- Test: `backend/domain/billing/credit_threshold_test.go`

**Steps:**
1. Append administrator credit-adjusted events inside the existing outer adjustment transaction.
2. Add configurable user/workspace low-balance thresholds with durable active-episode state.
3. Evaluate thresholds after settled balance changes, not after reserve/release bookkeeping.
4. Notify once when crossing below the threshold and reset the episode only after crossing above a recovery margin.
5. Never expose internal ledger entries or provider cost payloads.
6. Add threshold crossing, hysteresis, concurrent settlement, and adjustment rollback tests.

### Task 4: Persistent sandbox incidents

**Files:**
- Modify: `backend/domain/sandbox/entity.go`
- Modify: `backend/domain/sandbox/repository.go`
- Modify: `backend/application/sandbox/health.go`
- Create: `backend/application/sandbox/health_monitor.go`
- Test: `backend/application/sandbox/health_test.go`
- Test: `backend/application/sandbox/health_monitor_test.go`

**Steps:**
1. Persist provider health episode, consecutive failures, incident ID, notification state, and recovery state.
2. Add a bounded monitor worker using the existing sandbox control-plane enablement rules.
3. Publish one system-admin unhealthy notification after the stable threshold and one recovery notification.
4. Keep runtime routing fail-closed and independent from notification success.
5. Resolve recipients through server-side system-administrator authorization.
6. Add disabled-control-plane, transient failure, stable failure, cooldown, recovery, and worker restart tests.

### Task 5: Administrator announcements

**Files:**
- Create: `backend/domain/announcement/entity.go`
- Create: `backend/domain/announcement/repository.go`
- Create: `backend/infra/announcement/mysql_repository.go`
- Create: `backend/application/announcement/service.go`
- Create: `backend/api/handler/coze/announcement_service.go`
- Modify: `backend/api/router/coze/custom_routes.go`
- Modify: `idl/playground/playground.thrift`
- Create: `frontend/apps/coze-studio/src/pages/system/announcements/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/routes.tsx`
- Test: `backend/application/announcement/service_test.go`
- Test: `backend/api/handler/coze/announcement_service_test.go`
- Test: `frontend/apps/coze-studio/src/pages/system/announcements/__tests__/index.test.tsx`

**Steps:**
1. Add draft, scheduled, published, and cancelled announcement states with bounded title, body, audience, severity, and optional internal route target.
2. Require server-side system-administrator authorization for all write operations.
3. Snapshot the audience at publish time and append one outbox event per bounded recipient batch.
4. Support all users, selected workspaces, and selected users without accepting arbitrary recipient ownership claims.
5. Add a system-management page for create, preview, schedule, publish, cancel, and audit history.
6. Add authorization, schedule replay, audience isolation, payload bound, and frontend state tests.
