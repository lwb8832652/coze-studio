# Sandbox Runner 2C4G Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development`
> (recommended) or `executing-plans` to implement this plan task by task. Track
> progress with the checkboxes in this document and stop at every phase gate.

**Goal:** Add a NewX-native remote Sandbox Runner that executes Agent, AppDev,
MCP stdio, and Plugin workloads with tenant isolation, durable fair queuing, and
safe resource limits on a shared 2C4G host, while preserving the existing remote
Provider v1 contract and third-party Provider behavior.

**Architecture:** Keep NewX as the control plane and durable business record.
Extend the current remote Provider contract only with optional, versioned
features, signed tenant context, queue status, and signed scheduler configuration.
Run a separate `sandbox-runner` process against a rootless Docker-compatible
socket. Redis owns encrypted short-lived queue/runtime state; MySQL owns the
versioned scheduler configuration. Existing non-queue Providers continue using
`RuntimePolicy.MaxConcurrency`; only Providers advertising `queue_status_v1`
use `max_outstanding` admission capacity before the Runner applies its own
weighted execution slots.

**Tech stack:** Go 1.24, Hertz/net/http, GORM, Redis, Docker-compatible Engine
API over a rootless Unix socket, React 18, TypeScript, Vitest, Atlas Community
1.2.3, Docker Compose, GitHub Actions.

**Approved design:**
`docs/superpowers/specs/2026-08-11-sandbox-runner-2c4g-design.md`

---

## Non-negotiable compatibility rules

- `coze.sandbox.execute.v1` request/response JSON remains byte-compatible.
- Existing third-party Providers do not need to implement queue status,
  configuration push, or signed tenant context.
- `RuntimePolicy.MaxConcurrency` keeps its current meaning for Providers that
  do not advertise `queue_status_v1`.
- Native Runner requests must carry server-derived signed tenant context;
  missing or inconsistent identity fails closed.
- NewX, not Runner, remains the source of truth for users, spaces, tasks, and
  final business state.
- Neither NewX nor Runner may fall back to host command execution.
- NewX never mounts the host/root Docker socket. Runner may access only its
  dedicated rootless Docker-compatible socket.
- No credential, command, raw provider body, object key, host path, or tenant
  identifier may enter public errors, low-cardinality metrics, or admin audit
  metadata.
- Any change under `backend/application/agentthread/**` must update the two
  Workbench execution-chain authority files and pass the graph verification
  commands in Task 15.

## Default 2C4G profile

The first persisted scheduler row must normalize to this complete snapshot:

```json
{
  "total_weight": 2,
  "max_outstanding": 32,
  "global_queue_depth": 32,
  "per_space_queue_depth": 8,
  "per_user_queue_depth": 4,
  "host_memory_reserve_mb": 1536,
  "cancel_grace_seconds": 5,
  "health_failure_threshold": 3,
  "health_recovery_threshold": 2,
  "workloads": {
    "agent": {"weight": 2, "cpu_limit": 1.25, "memory_limit_mb": 1536, "pid_limit": 128, "queue_timeout_seconds": 600, "idle_ttl_seconds": 300},
    "appdev": {"weight": 2, "cpu_limit": 1.25, "memory_limit_mb": 1536, "pid_limit": 192, "queue_timeout_seconds": 1200, "idle_ttl_seconds": 600},
    "mcp_stdio": {"weight": 1, "cpu_limit": 0.4, "memory_limit_mb": 384, "pid_limit": 64, "queue_timeout_seconds": 300, "idle_ttl_seconds": 180},
    "plugin": {"weight": 1, "cpu_limit": 0.4, "memory_limit_mb": 384, "pid_limit": 64, "queue_timeout_seconds": 300, "idle_ttl_seconds": 0}
  }
}
```

The domain validator must reject partial snapshots, total weight outside
`1..64`, queue depths outside `1..4096`, non-positive CPU/memory/PID limits,
workload weight greater than total weight, and any profile that leaves less
than 1536 MiB host reserve in the declared 2C4G compatibility mode.

## File map

### Shared control plane

- `backend/domain/sandbox/entity.go`, `policy.go`, `repository.go`: Provider
  features, scheduler settings, validation, and repository interfaces.
- `backend/infra/sandbox/mysql_models.go`, `mysql_convert.go`,
  `mysql_repository.go`: feature and scheduler persistence.
- `backend/infra/sandbox/provider.go`, `remote_provider.go`: signed execution
  identity, queue status, configuration push, and strict remote client.
- `backend/application/sandbox/types.go`, `service.go`, `router.go`,
  `metrics.go`: admin DTOs, settings mutation, feature-aware admission, and
  safe runtime projection.
- `backend/application/sandbox_wiring.go`: repository, signer, and control-plane
  wiring.
- `backend/api/handler/coze/admin_sandbox.go` and
  `backend/api/router/coze/custom_routes.go`: admin settings/runtime routes.
- `docker/atlas/migrations/20260811000100_sandbox_runner_scheduler.sql`,
  `docker/atlas/migrations/atlas.sum`, `docker/atlas/opencoze_latest_schema.hcl`:
  additive schema.

### Native Runner

- `backend/cmd/sandbox-runner/main.go`: standalone entrypoint.
- `backend/internal/sandboxrunner/config.go`, `server.go`, `auth.go`,
  `protocol.go`: startup, TLS, authentication, strict v1 endpoints.
- `backend/internal/sandboxrunner/store.go`, `redis_store.go`: encrypted
  idempotency, queue, execution, and runtime records.
- `backend/internal/sandboxrunner/scheduler.go`: workspace round-robin,
  space FIFO, user-heavy-task gate, deadlines, and resource watermark.
- `backend/internal/sandboxrunner/lifecycle.go`: reuse keys, generations,
  drain, destroy, and quarantine.
- `backend/internal/sandboxrunner/runtime/driver.go`, `docker_driver.go`,
  `security.go`: rootless container API and enforced sandbox profile.
- `backend/internal/sandboxrunner/workload.go`: four reviewed entrypoint
  adapters; no arbitrary command endpoint.
- `backend/internal/sandboxrunner/metrics.go`: bounded Prometheus metrics.

### Identity propagation

- `backend/pkg/sandboxidentity/context.go`: typed, server-owned identity context.
- `backend/infra/coderunner/code.go` and
  `backend/infra/coderunner/impl/controlplane/runner.go`: identity transfer into
  Provider requests.
- `backend/application/agentthread/runner.go` and
  `backend/application/agentthread/adk_mcp_stdio_provider_transport.go`: attach
  authoritative Agent/MCP identity.
- `backend/application/appdev/provider_runtime_orchestrator.go` and
  `backend/infra/appdev/sandbox_runtime_manager.go`: attach space/project/user
  identity.
- `backend/application/plugin/code_plugin.go`: attach authorized plugin debug
  identity and one-shot execution identity.
- `backend/domain/workflow/internal/nodes/code/code.go`: attach workflow
  execution identity from the authoritative workflow execution context.

### System management UI

- `frontend/apps/coze-studio/src/pages/system/sandbox-service.ts`: settings and
  runtime-status contracts.
- `frontend/apps/coze-studio/src/pages/system/sandbox-scheduler-card.tsx` and
  `.module.less`: versioned 2C4G controls and active/desired state.
- `frontend/apps/coze-studio/src/pages/system/sandbox-management-section.tsx`:
  integrate the card without changing Provider CRUD behavior.
- `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-scheduler-card.test.tsx`
  and `sandbox-service.test.ts`: validation, stale version, and safe projection.

### Packaging and operations

- `backend/Dockerfile.sandbox-runner`: minimal Runner binary image.
- `deploy/sandbox-runner/Dockerfile.runtime`: digest-pinned execution image.
- `deploy/dev/docker-compose.yml`,
  `deploy/dev/docker-compose.runner-2c4g.yml`, `.env.example`, `deploy.sh`:
  preserve the existing local-data compose mode and add a remote-data 2C4G
  profile with the third service, rootless socket/TLS mounts, health gate,
  rollback, and revision tracking.
- `.github/workflows/deploy-dev.yml`: build, verify, promote, and deploy
  `coze-sandbox-runner` together with server/web.
- `deploy/dev/tests/*.sh`: release contract regression tests.
- `docs/superpowers/runbooks/sandbox-control-plane-operations.md`: rollout,
  drain, quarantine, key rotation, and rollback.

---

## Phase 1 — Contracts, storage, and control plane

### Task 1: Lock optional Provider features and signed tenant identity

**Files:**

- Modify: `backend/domain/sandbox/entity.go`
- Modify: `backend/domain/sandbox/policy.go`
- Modify: `backend/infra/sandbox/provider.go`
- Modify: `backend/infra/sandbox/remote_provider.go`
- Modify: `backend/infra/sandbox/remote_provider_test.go`
- Create: `backend/infra/sandbox/remote_provider_queue_test.go`
- Create: `backend/pkg/sandboxidentity/context.go`
- Create: `backend/pkg/sandboxidentity/context_test.go`

- [ ] **Step 1: Add failing normalization and wire tests**

Cover these cases before implementation:

- health accepts omitted `features` and preserves existing capabilities;
- health accepts exactly `queue_status_v1` and rejects unknown/duplicate features;
- strict health decoding still rejects every other unknown field;
- signed identity contains only numeric/string IDs, issue/expiry times, nonce,
  request digest, and key ID;
- signature verification rejects tampering, expiry, wrong key, wrong body digest,
  and missing scope-required fields;
- `ExecuteRequest` JSON remains unchanged when identity is attached;
- queue status response accepts only the approved safe fields.

Use these public types:

```go
type ProviderFeature string

const ProviderFeatureQueueStatusV1 ProviderFeature = "queue_status_v1"

type ExecutionIdentity struct {
    SpaceID     int64
    UserID      int64
    ProjectID   string
    SessionID   string
    ExecutionID string
}

type QueueStatus struct {
    Schema               string `json:"schema"`
    Waiting              bool   `json:"waiting"`
    ApproximatePosition  int    `json:"approximate_position"`
    EstimatedWaitSeconds int    `json:"estimated_wait_seconds"`
    DeadlineUnixMilli    int64  `json:"deadline_unix_milli"`
    Cancelable           bool   `json:"cancelable"`
    ReasonCode           string `json:"reason_code"`
}
```

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/sandbox ./pkg/sandboxidentity ./infra/sandbox -run 'Test(ProviderFeature|ExecutionIdentity|RemoteProviderQueue|RemoteProviderHealth)' -count=1
```

Expected: FAIL because feature normalization, identity signing, and queue status
do not exist.

- [ ] **Step 2: Implement additive contracts**

Add `Features []ProviderFeature` to `HealthSnapshot`/`HealthResult`, but do not
mix features into `Capabilities []Scope`. Add `Identity ExecutionIdentity` to
`ExecuteRequest` with `json:"-"`. Preserve `canonicalExecuteRequest` and
`DigestExecuteRequest` body bytes; bind the identity envelope to that digest.

Add optional interface:

```go
type QueueStatusProvider interface {
    QueueStatus(context.Context, string) (QueueStatus, error)
}
```

Use `GET /v1/executions/{execution_id}/queue-status` with schema
`coze.sandbox.queue_status.v1`. Send signed identity in
`X-Coze-Sandbox-Context` and `X-Coze-Sandbox-Context-Signature`; never put it in
the execute JSON body.

- [ ] **Step 3: Re-run focused tests and compatibility tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/sandbox ./pkg/sandboxidentity ./infra/sandbox -count=1
```

Expected: PASS, including all existing remote Provider strict-protocol tests.

- [ ] **Step 4: Commit**

```bash
git add backend/domain/sandbox backend/pkg/sandboxidentity backend/infra/sandbox/provider.go backend/infra/sandbox/remote_provider.go backend/infra/sandbox/*remote_provider*test.go
git commit -m "feat: add sandbox runner optional protocol contracts"
```

### Task 2: Persist Provider features and versioned scheduler settings

**Files:**

- Create: `docker/atlas/migrations/20260811000100_sandbox_runner_scheduler.sql`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `backend/domain/sandbox/entity.go`
- Create: `backend/domain/sandbox/scheduler.go`
- Create: `backend/domain/sandbox/scheduler_test.go`
- Modify: `backend/domain/sandbox/repository.go`
- Modify: `backend/infra/sandbox/mysql_models.go`
- Modify: `backend/infra/sandbox/mysql_convert.go`
- Modify: `backend/infra/sandbox/mysql_repository.go`
- Create: `backend/infra/sandbox/mysql_scheduler_repository.go`
- Create: `backend/infra/sandbox/mysql_scheduler_repository_test.go`
- Modify: `backend/infra/sandbox/mysql_integration_test.go`

- [ ] **Step 1: Write failing domain and repository tests**

Test the exact default profile above, complete-snapshot validation, detached
map/slice values, singleton row creation, CAS update, stale-version conflict,
and feature round-trip. Assert credentials never appear in `settings_json`.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/sandbox ./infra/sandbox -run 'Test(SchedulerSettings|MySQLScheduler|ProviderFeatures)' -count=1
```

Expected: FAIL because the entity, repository, table, and feature column are
missing.

- [ ] **Step 2: Add the additive migration**

The migration must:

```sql
ALTER TABLE `sandbox_providers`
  ADD COLUMN `last_health_features_json` JSON NULL AFTER `last_health_capabilities_json`;

UPDATE `sandbox_providers`
SET `last_health_features_json` = JSON_ARRAY()
WHERE `last_health_features_json` IS NULL;

ALTER TABLE `sandbox_providers`
  MODIFY COLUMN `last_health_features_json` JSON NOT NULL;

CREATE TABLE `sandbox_scheduler_settings` (
  `id` TINYINT UNSIGNED NOT NULL,
  `settings_json` JSON NOT NULL,
  `version` BIGINT UNSIGNED NOT NULL,
  `updated_by` BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `chk_sandbox_scheduler_settings_singleton` CHECK (`id` = 1)
);
```

Insert the normalized default row with `id=1`, `version=1`, and
`updated_by=0`. Do not alter or reinterpret existing `max_concurrency` rows.

- [ ] **Step 3: Implement domain and MySQL persistence**

Add `SchedulerSettingsRepository` with `GetSchedulerSettings` and
`UpdateSchedulerSettingsCAS`. The update must lock the singleton row, require
the expected version, validate the entire snapshot, increment once, and return
a detached value.

- [ ] **Step 4: Regenerate and validate Atlas state**

```bash
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e migrate hash --dir file:///migrations
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e migrate validate --dir file:///migrations
```

Expected: Atlas reports a valid migration directory and rewrites only
`atlas.sum` in addition to the new migration/schema snapshot.

- [ ] **Step 5: Run persistence tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/sandbox ./infra/sandbox -run 'Test(SchedulerSettings|MySQLScheduler|ProviderFeatures|MySQLIntegration)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/domain/sandbox backend/infra/sandbox/mysql_* docker/atlas/migrations/20260811000100_sandbox_runner_scheduler.sql docker/atlas/migrations/atlas.sum docker/atlas/opencoze_latest_schema.hcl
git commit -m "feat: persist sandbox scheduler settings"
```

### Task 3: Add admin settings and safe runtime-status APIs

**Files:**

- Modify: `backend/application/sandbox/types.go`
- Modify: `backend/application/sandbox/service.go`
- Create: `backend/application/sandbox/scheduler_service.go`
- Create: `backend/application/sandbox/scheduler_service_test.go`
- Modify: `backend/application/sandbox_wiring.go`
- Modify: `backend/api/handler/coze/admin_sandbox.go`
- Modify: `backend/api/handler/coze/admin_sandbox_test.go`
- Modify: `backend/api/router/coze/custom_routes.go`
- Modify: `backend/api/router/coze/admin_sandbox.go`
- Modify: `backend/api/router/coze/admin_sandbox_test.go`

- [ ] **Step 1: Add failing service, handler, and route tests**

Require system-admin auth for:

```text
GET /api/admin/sandboxes/scheduler-settings
PUT /api/admin/sandboxes/scheduler-settings
GET /api/admin/sandboxes/runtime-status
```

PUT accepts `{expected_version, settings}` only. Unknown, duplicate,
case-variant, secret-like, and partial fields return 400. A stale version returns
the existing `SANDBOX_VERSION_CONFLICT`. Runtime status contains only config
versions, aggregate queue/slot/watermark counts, drain/quarantine counts, and
stable reason codes.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./application/sandbox ./api/handler/coze ./api/router/coze -run 'Test(AdminSandboxScheduler|SchedulerService|AdminSandboxRuntimeStatus)' -count=1
```

Expected: FAIL because the service and routes are absent.

- [ ] **Step 2: Implement CAS mutation and audit**

Use actions `scheduler_settings.update` and
`scheduler_settings.update_failed`. Safe metadata is limited to
`previous_version`, `new_version`, and `changed_fields`; never include the JSON
snapshot itself. Commit DB configuration before attempting remote application;
if Runner application fails, return the saved desired version plus
`applied=false` and a stable `PROVIDER_UNAVAILABLE` state rather than rolling
back the database.

- [ ] **Step 3: Implement safe runtime-status aggregation**

Add an application interface consumed by the handler so tests can inject a
fake. If no native Runner is healthy, return a successful admin envelope with
`available=false`; do not turn the whole Sandbox management page into 500.

- [ ] **Step 4: Run focused backend tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./application/sandbox ./api/handler/coze ./api/router/coze -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/application/sandbox backend/application/sandbox_wiring.go backend/api/handler/coze/admin_sandbox* backend/api/router/coze/admin_sandbox* backend/api/router/coze/custom_routes.go
git commit -m "feat: manage sandbox scheduler settings"
```

### Task 4: Add the scheduler card to system Sandbox management

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-service.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-scheduler-card.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-scheduler-card.module.less`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-management-section.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-scheduler-card.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-service.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-system-page.test.tsx`

- [ ] **Step 1: Add failing API and component tests**

Test initial load, default 2C4G values, complete snapshot submission, stale
version refresh, disabled save during mutation, abort on unmount, desired vs
applied version, unavailable Runner, and field-specific validation. Assert no
credential or endpoint is rendered by the runtime-status card.

Run:

```bash
cd frontend/apps/coze-studio
npx vitest run src/pages/system/__tests__/sandbox-service.test.ts src/pages/system/__tests__/sandbox-scheduler-card.test.tsx src/pages/system/__tests__/sandbox-system-page.test.tsx
```

Expected: FAIL because settings clients and card are absent.

- [ ] **Step 2: Implement the typed API client**

Add `getSandboxSchedulerSettings`, `updateSandboxSchedulerSettings`, and
`getSandboxRuntimeStatus` using the existing strict envelope/error path. Keep
the existing Provider DTO and CRUD methods unchanged.

- [ ] **Step 3: Implement the card with existing design-system components**

Render a collapsed “2C4G 调度配置” card above the Provider table. Group
global limits and four workloads, show units, keep keyboard/focus behavior,
and show “配置版本 / Runner 已应用版本”. Do not add a new system navigation
item.

- [ ] **Step 4: Run frontend tests**

```bash
cd frontend/apps/coze-studio
npx vitest run src/pages/system/__tests__/sandbox-*.test.tsx src/pages/system/__tests__/sandbox-service.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/apps/coze-studio/src/pages/system/sandbox-*
git commit -m "feat: expose sandbox scheduler controls"
```

## Phase 1 gate

- [ ] Existing Provider CRUD/health tests pass unchanged.
- [ ] Existing non-queue Provider capacity tests still assert
  `MaxConcurrency`.
- [ ] Atlas validation passes; no migration is applied as part of this gate.
- [ ] System UI can save the complete 2C4G snapshot but reports Runner as
  unavailable until Phase 2.

---

## Phase 2 — Native Runner API, durable queue, and scheduling

### Task 5: Separate NewX admission from Runner execution slots

**Files:**

- Modify: `backend/application/sandbox/router.go`
- Modify: `backend/application/sandbox/router_test.go`
- Modify: `backend/application/sandbox/task9_async_router_test.go`
- Modify: `backend/application/sandbox/types.go`
- Modify: `backend/infra/sandbox/provider.go`
- Modify: `backend/infra/sandbox/remote_provider.go`
- Modify: `backend/infra/sandbox/remote_provider_queue_test.go`

- [ ] **Step 1: Add failing feature-aware capacity tests**

Prove that a Provider without `queue_status_v1` acquires
`Policy.MaxConcurrency`, while a queue-capable Provider acquires the persisted
`max_outstanding`. Cover Resolve, resume/rebuild, lease renewal, cancellation,
and cleanup. A missing settings repository or invalid settings must fail closed.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./application/sandbox -run 'TestProviderRouter.*(Capacity|Queue|Resume)' -count=1
```

Expected: FAIL because all Providers still use `MaxConcurrency`.

- [ ] **Step 2: Add a single admission-limit helper**

```go
func providerAdmissionLimit(
    descriptor ProviderDescriptor,
    settings domainsandbox.SchedulerSettings,
) (int, error) {
    if descriptor.HasFeature(domainsandbox.ProviderFeatureQueueStatusV1) {
        return settings.MaxOutstanding, nil
    }
    return descriptor.Policy.MaxConcurrency, nil
}
```

Use this helper at every capacity-acquire path. Persist the selected feature
and admission limit in the existing execution checkpoint so a restart does not
reinterpret an in-flight lease after a health/config change.

- [ ] **Step 3: Add queue-status polling without changing terminal status**

Expose queue status from `SelectedProvider` only when the runtime implements
`QueueStatusProvider`. Existing status polling continues to treat `accepted`
as non-terminal. Unsupported Providers return a typed “feature unavailable”
result, not an execution failure.

- [ ] **Step 4: Run router and remote tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./application/sandbox ./infra/sandbox -run 'Test(ProviderRouter|SelectedProvider|RemoteProviderQueue)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/application/sandbox backend/infra/sandbox/provider.go backend/infra/sandbox/remote_provider.go backend/infra/sandbox/remote_provider_queue_test.go
git commit -m "feat: split sandbox admission and execution capacity"
```

### Task 6: Build the strict authenticated Runner HTTP surface

**Files:**

- Create: `backend/cmd/sandbox-runner/main.go`
- Create: `backend/internal/sandboxrunner/config.go`
- Create: `backend/internal/sandboxrunner/config_test.go`
- Create: `backend/internal/sandboxrunner/auth.go`
- Create: `backend/internal/sandboxrunner/auth_test.go`
- Create: `backend/internal/sandboxrunner/protocol.go`
- Create: `backend/internal/sandboxrunner/server.go`
- Create: `backend/internal/sandboxrunner/server_test.go`

- [ ] **Step 1: Add failing HTTP contract tests**

Drive the server through `httptest` and the existing `RemoteProvider`. Cover:

- `GET /v1/health` returns `protocol_version=v1`, four capabilities, and
  `features=["queue_status_v1"]`;
- execute validates bearer credential, signed context, strict JSON, size,
  deadline, scope/entrypoint, and idempotency key;
- status, keep-alive, cancel, lookup, build, artifact publish, queue status,
  and configuration endpoints reject trailing paths and unknown fields;
- every public error is a fixed code/message without request/provider detail;
- TLS is mandatory unless `SANDBOX_RUNNER_DEBUG_LOOPBACK_HTTP=true` and the
  listener is loopback.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner -run 'Test(Server|Auth|Config)' -count=1
```

Expected: FAIL because the package does not exist.

- [ ] **Step 2: Implement startup and fail-closed configuration**

Require:

```text
SANDBOX_RUNNER_ENABLED=true
SANDBOX_RUNNER_LISTEN_ADDR=:9443
SANDBOX_RUNNER_TLS_CERT_FILE=/run/secrets/sandbox-runner.crt
SANDBOX_RUNNER_TLS_KEY_FILE=/run/secrets/sandbox-runner.key
SANDBOX_RUNNER_AUTH_TOKEN=<deployment secret>
SANDBOX_RUNNER_CONTEXT_VERIFY_KEYS_JSON=<versioned keyring>
SANDBOX_RUNNER_QUEUE_KEYS_JSON=<versioned keyring>
SANDBOX_RUNNER_ACTIVE_QUEUE_KEY_ID=<key id>
SANDBOX_RUNNER_ROOTLESS_ENDPOINT=unix:///run/user/10001/podman/podman.sock
SANDBOX_RUNNER_EXECUTION_IMAGE=<registry>/coze-sandbox-runtime@sha256:<digest>
SANDBOX_RUNNER_EMERGENCY_STOP=false
```

The NewX server loads the matching signing side from
`SANDBOX_EXECUTION_CONTEXT_SIGNING_KEYS_JSON` and
`SANDBOX_EXECUTION_CONTEXT_ACTIVE_KEY_ID`. Both loaders require a versioned
keyring and redact values from string formatting, errors, logs, and audits.

Reject missing secrets, permissive secret-file modes, non-Unix runtime
endpoints, mutable image tags, and emergency stop. Never log values.

- [ ] **Step 3: Implement strict protocol handlers against interfaces**

Define a `Store`, `Scheduler`, `LifecycleManager`, and `ArtifactPublisher`
interface. The server validates/authenticates and delegates only; it does not
contain container logic.

- [ ] **Step 4: Run Runner server tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner -count=1
```

Expected: PASS with fakes.

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/sandbox-runner backend/internal/sandboxrunner
git commit -m "feat: add authenticated sandbox runner api"
```

### Task 7: Add encrypted Redis execution and queue state

**Files:**

- Create: `backend/internal/sandboxrunner/store.go`
- Create: `backend/internal/sandboxrunner/redis_store.go`
- Create: `backend/internal/sandboxrunner/redis_store_test.go`
- Create: `backend/internal/sandboxrunner/redis_scripts.go`

- [ ] **Step 1: Add failing Redis state-machine tests**

Use `miniredis`. Cover atomic accept/idempotent replay, same key/different digest
conflict, queue-depth caps, monotonic state transitions, terminal immutability,
cancel races, lookup after ambiguous submission, TTL, active key rotation, old
key read/new key write, and ciphertext-only Redis values.

State keys must be namespaced by Runner deployment ID and contain only hashes
of tenant IDs in key names. Plain identity and execution envelopes exist only
inside authenticated encryption.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner -run 'TestRedis(Store|Queue|Idempotency|KeyRotation)' -count=1
```

Expected: FAIL because Redis persistence is absent.

- [ ] **Step 2: Implement the atomic scripts and encrypted envelope**

Accepted records include request digest, encrypted request, identity digest,
scope, weight class, deadline, state, timestamps, and terminal projection.
Encrypt with AES-GCM and authenticated metadata `{schema,key_id,deployment_id}`.
Never persist artifact bearer tokens past their expiry; consume download grants
exactly once.

- [ ] **Step 3: Implement recovery scan**

On startup, requeue durable `accepted` records, reconcile `running` records
against labeled containers, quarantine ambiguous containers, and never change a
terminal record. Redis unavailable rejects new accepts but preserves best-effort
status/cancel for already loaded records.

- [ ] **Step 4: Run store tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner -run 'TestRedis' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/sandboxrunner/store.go backend/internal/sandboxrunner/redis_*
git commit -m "feat: persist sandbox runner queue state"
```

### Task 8: Implement 2C4G fair scheduling and resource watermarks

**Files:**

- Create: `backend/internal/sandboxrunner/scheduler.go`
- Create: `backend/internal/sandboxrunner/scheduler_test.go`
- Create: `backend/internal/sandboxrunner/resources.go`
- Create: `backend/internal/sandboxrunner/resources_test.go`

- [ ] **Step 1: Add deterministic failing scheduler tests**

Use a fake clock and resource sampler. Cover workspace round-robin, space FIFO,
same-user heavy limit one, AppDev project serialization, one heavy vs two light
slots, no heavy/light mixing, queue deadlines, cancellation, approximate
position privacy, moving wait estimate, config-version snapshots, and pause at
the 1536 MiB reserve.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner -run 'Test(Scheduler|ResourceWatermark)' -count=1
```

Expected: FAIL because no scheduler exists.

- [ ] **Step 2: Implement the state machine**

The scheduler may dispatch only after one atomic Redis transition reserves the
weight and records the selected runtime key. Release weight once on every
terminal path. `QUEUE_TIMEOUT` is terminal; low memory produces
`CAPACITY_UNAVAILABLE` while retaining the queued item until its deadline.

- [ ] **Step 3: Add race and randomized invariant tests**

Run at least 1,000 generated enqueue/cancel/finish operations and assert:

```text
0 <= used_weight <= total_weight
heavy_active implies light_active == 0
each user heavy_active <= 1
each appdev project active <= 1
terminal execution never returns to accepted/running
```

- [ ] **Step 4: Run normal and race tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner -run 'Test(Scheduler|ResourceWatermark)' -count=1
GOCACHE=/private/tmp/coze-go-build go test -race ./internal/sandboxrunner -run 'TestScheduler' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/sandboxrunner/scheduler.go backend/internal/sandboxrunner/scheduler_test.go backend/internal/sandboxrunner/resources*
git commit -m "feat: schedule sandbox work fairly on 2c4g"
```

## Phase 2 gate

- [ ] Existing RemoteProvider tests pass against the in-memory Runner server.
- [ ] Durable accept returns before execution capacity is available.
- [ ] Queue depth caps and deadlines are deterministic.
- [ ] No container runtime dependency is required yet; scheduler tests use a
  fake dispatcher.

---

## Phase 3 — Rootless execution, lifecycle, and workload integration

### Task 9: Implement lifecycle keys, reuse, drain, and quarantine

**Files:**

- Create: `backend/internal/sandboxrunner/lifecycle.go`
- Create: `backend/internal/sandboxrunner/lifecycle_test.go`
- Create: `backend/internal/sandboxrunner/runtime/driver.go`
- Create: `backend/internal/sandboxrunner/runtime/fake_driver_test.go`

- [ ] **Step 1: Add failing lifecycle tests**

Assert exact keys and TTLs:

```text
agent     = space_id + user_id       (300s)
appdev    = space_id + project_id    (600s)
mcp_stdio = space_id + session_id    (180s)
plugin    = space_id + execution_id  (destroy immediately)
```

Also test image digest, policy version, scheduler version, and credential
generation mismatch; residual process cleanup; failed health check; drain;
cancel grace/kill; restart recovery; cleanup retry; quarantine; and no reuse
after incomplete cleanup.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner -run 'TestLifecycle' -count=1
```

Expected: FAIL because lifecycle management is absent.

- [ ] **Step 2: Implement lifecycle management behind `runtime.Driver`**

Container labels may include only hashed reuse key, scope, image digest,
configuration version, credential generation, and Runner deployment ID. Do not
label raw user/space/project/session IDs. Before reuse, stop residual processes,
clear task tmpfs/credential paths, and execute a fixed health probe.

- [ ] **Step 3: Run lifecycle tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner -run 'TestLifecycle' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/sandboxrunner/lifecycle* backend/internal/sandboxrunner/runtime
git commit -m "feat: manage isolated sandbox lifecycles"
```

### Task 10: Add the rootless Docker-compatible runtime driver

**Files:**

- Modify: `backend/go.mod`
- Modify: `backend/go.sum`
- Create: `backend/internal/sandboxrunner/runtime/docker_driver.go`
- Create: `backend/internal/sandboxrunner/runtime/docker_driver_test.go`
- Create: `backend/internal/sandboxrunner/runtime/security.go`
- Create: `backend/internal/sandboxrunner/runtime/security_test.go`
- Create: `deploy/sandbox-runner/Dockerfile.runtime`

- [ ] **Step 1: Add failing driver request tests**

Use a fake Docker-compatible API server; do not require a local daemon. Assert
the create request always has:

```text
non-root user; read-only rootfs; no-new-privileges; cap-drop=ALL;
private user/pid/ipc/network namespaces; bounded PIDs/CPU/memory;
tmpfs /tmp and /run/newx-secrets; no privileged mode; no devices;
no host/rootless socket mount; no arbitrary bind mounts; digest image only
```

Network-disabled workloads receive `NetworkMode=none`. Network-enabled
workloads attach only to a Runner-managed egress network and receive the
validated proxy endpoint; raw destination access remains blocked outside the
container.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner/runtime -count=1
```

Expected: FAIL because the driver does not exist.

- [ ] **Step 2: Add the Docker client as a direct dependency**

Use the already resolved `github.com/docker/docker` major version from
`backend/go.sum`. Configure it only from the validated Unix endpoint; do not
read `DOCKER_HOST` implicitly.

- [ ] **Step 3: Implement policy enforcement and egress checks**

Resolve allowlisted DNS through the Runner, reject loopback/link-local/private/
metadata/host-gateway/control-plane addresses after every resolution, and pass
only an opaque proxy grant to the container. A DNS rebinding or unavailable
policy enforcer fails closed with `POLICY_DENIED` or `PROVIDER_UNAVAILABLE`.

- [ ] **Step 4: Build a minimal immutable runtime image**

The image contains fixed Python/Node launchers and the reviewed Agent, Plugin,
MCP, and AppDev adapters. It runs as UID 10001 and has no Docker CLI, SSH
server, package-manager credentials, or NewX configuration. Task 14 resolves
the revision tag to a digest before Runner startup.

- [ ] **Step 5: Run tests and build locally**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner/runtime -count=1
cd ..
docker build -f deploy/sandbox-runner/Dockerfile.runtime -t coze-sandbox-runtime:test .
```

Expected: tests PASS and image build succeeds.

- [ ] **Step 6: Commit**

```bash
git add backend/go.mod backend/go.sum backend/internal/sandboxrunner/runtime deploy/sandbox-runner/Dockerfile.runtime
git commit -m "feat: execute sandboxes through rootless containers"
```

### Task 11: Add reviewed workload adapters and authoritative identity propagation

**Files:**

- Create: `backend/internal/sandboxrunner/workload.go`
- Create: `backend/internal/sandboxrunner/workload_test.go`
- Modify: `backend/infra/coderunner/code.go`
- Modify: `backend/infra/coderunner/impl/controlplane/runner.go`
- Modify: `backend/infra/coderunner/impl/controlplane/runner_test.go`
- Modify: `backend/application/agentthread/runner.go`
- Modify: `backend/application/agentthread/adk_mcp_stdio_provider_transport.go`
- Modify: `backend/application/agentthread/adk_mcp_stdio_provider_transport_test.go`
- Modify: `backend/application/appdev/provider_runtime_orchestrator.go`
- Modify: `backend/application/appdev/provider_runtime_start_test.go`
- Modify: `backend/application/appdev/provider_runtime_status_test.go`
- Modify: `backend/infra/appdev/sandbox_runtime_manager.go`
- Modify: `backend/application/plugin/code_plugin.go`
- Modify: `backend/application/plugin/code_plugin_test.go`
- Modify: `backend/domain/workflow/internal/nodes/code/code.go`
- Modify: `backend/domain/workflow/internal/nodes/code/code_test.go`

- [ ] **Step 1: Add failing identity-source tests**

Prove each identity comes from an already-authorized server record:

- Agent: `RunSummary.SpaceID`, `CreatorID`, and `RunID`;
- MCP: Agent Run space/creator plus thread/run-derived session ID;
- AppDev: authorized project owner input plus project ID;
- Plugin: validated plugin space, authenticated user, and generated execution ID;
- Workflow code: root workflow basic space, operator, and root execution ID.

Client body IDs and arbitrary context values must not override these fields.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./infra/coderunner/impl/controlplane ./application/agentthread ./application/appdev ./infra/appdev ./application/plugin ./domain/workflow/internal/nodes/code -run 'Test.*SandboxIdentity' -count=1
```

Expected: FAIL because identity is not propagated.

- [ ] **Step 2: Attach one typed identity context at each authorized boundary**

`RunProcessor.processRun` wraps the executor context with Agent identity before
calling `p.executor.Execute`. MCP derives its session from the same authoritative
Run. AppDev and Plugin pass explicit verified identity through `RunRequest`.
Workflow execution context supplies its root execution facts. The control-plane
code runner copies only a normalized identity into `ExecuteRequest.Identity`.

- [ ] **Step 3: Implement only four fixed entrypoints**

```text
agent/code/run
plugin/code/run
mcp/stdio/invoke
appdev/runtime/start
```

Dispatch uses exact scope/workload/entrypoint tuples. Reject shell
entrypoints, absolute paths, unapproved executables, and arbitrary image or
command overrides. Plugin always sets one-shot lifecycle.

- [ ] **Step 4: Run integration package tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./infra/coderunner/impl/controlplane ./application/agentthread ./application/appdev ./infra/appdev ./application/plugin ./domain/workflow/internal/nodes/code -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/sandboxrunner/workload* backend/infra/coderunner backend/application/agentthread backend/application/appdev backend/infra/appdev backend/application/plugin backend/domain/workflow/internal/nodes/code
git commit -m "feat: route sandbox workloads with tenant identity"
```

### Task 12: Apply signed scheduler configuration atomically

**Files:**

- Create: `backend/infra/sandbox/config_signer.go`
- Create: `backend/infra/sandbox/config_signer_test.go`
- Modify: `backend/infra/sandbox/remote_provider.go`
- Create: `backend/infra/sandbox/remote_provider_config_test.go`
- Modify: `backend/application/sandbox/scheduler_service.go`
- Modify: `backend/application/sandbox/health.go`
- Create: `backend/internal/sandboxrunner/configuration.go`
- Create: `backend/internal/sandboxrunner/configuration_test.go`

- [ ] **Step 1: Add failing signing, push, and generation tests**

Use schema `coze.sandbox.scheduler_config.v1` and endpoint
`PUT /v1/configuration`. Cover canonical JSON, key ID, issued/expiry time,
version monotonicity, replay, tamper, unknown key, partial snapshot, stale
version, active-task snapshot retention, and credential/image generation drain.

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./infra/sandbox ./application/sandbox ./internal/sandboxrunner -run 'Test.*(ConfigSigner|Configuration|SchedulerApply)' -count=1
```

Expected: FAIL because configuration signing/application is absent.

- [ ] **Step 2: Implement NewX signing and push**

Load `SANDBOX_RUNNER_CONFIG_SIGNING_KEYS_JSON` and
`SANDBOX_RUNNER_ACTIVE_CONFIG_KEY_ID` through a redacting keyring loader.
After a successful admin CAS update, push the full snapshot to every healthy
queue-capable default Runner. The health monitor also repairs an applied
version behind the desired version.

- [ ] **Step 3: Implement Runner verification and atomic activation**

Verify first, persist the encrypted snapshot to Redis, then atomically swap the
in-memory pointer. New dispatches capture the new version; running tasks retain
their old snapshot; idle runtimes with an old version enter drain.

- [ ] **Step 4: Run configuration tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./infra/sandbox ./application/sandbox ./internal/sandboxrunner -run 'Test.*(Config|SchedulerApply)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/infra/sandbox/config_signer* backend/infra/sandbox/remote_provider*config* backend/infra/sandbox/remote_provider.go backend/application/sandbox/scheduler_service.go backend/application/sandbox/health.go backend/internal/sandboxrunner/configuration*
git commit -m "feat: distribute signed sandbox scheduler config"
```

## Phase 3 gate

- [ ] Fake-driver end-to-end tests execute all four scopes.
- [ ] A missing identity, Redis, rootless runtime, policy, or key fails closed.
- [ ] Plugin containers are destroyed immediately; reusable workloads never
  cross their exact reuse keys or configuration generations.
- [ ] Existing Agent, MCP, AppDev, Plugin, and Workflow package tests pass.

---

## Phase 4 — Observability, deployment, and acceptance

### Task 13: Add bounded Runner metrics and admin runtime projection

**Files:**

- Create: `backend/internal/sandboxrunner/metrics.go`
- Create: `backend/internal/sandboxrunner/metrics_test.go`
- Modify: `backend/internal/sandboxrunner/server.go`
- Modify: `backend/infra/sandbox/provider.go`
- Modify: `backend/infra/sandbox/remote_provider.go`
- Create: `backend/infra/sandbox/remote_provider_runtime_status_test.go`
- Modify: `backend/application/sandbox/metrics.go`
- Modify: `backend/application/sandbox/scheduler_service.go`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-scheduler-card.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-scheduler-card.test.tsx`

- [ ] **Step 1: Add failing metric allowlist tests**

Allow labels only from `scope`, `workload`, `state`, `outcome`, and stable
`reason_code`. Explicitly reject user, space, project, session, execution,
endpoint, image, command, and credential labels.

- [ ] **Step 2: Implement `/metrics` and `/v1/runtime-status`**

Runtime status returns desired/applied config version, aggregate queue depth by
scope, used/total weight, memory reserve state, draining count, quarantine
count, and latest stable reason code. It returns no container IDs or tenant
keys.

- [ ] **Step 3: Complete the admin card projection**

Poll only while the Sandbox page is visible, stop on unmount, and retain the
last successful projection with an explicit stale badge on transient errors.

- [ ] **Step 4: Run tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./internal/sandboxrunner ./infra/sandbox ./application/sandbox -run 'Test.*(Metrics|RuntimeStatus)' -count=1
cd ../frontend/apps/coze-studio
npx vitest run src/pages/system/__tests__/sandbox-scheduler-card.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/sandboxrunner/metrics* backend/internal/sandboxrunner/server.go backend/infra/sandbox backend/application/sandbox frontend/apps/coze-studio/src/pages/system/sandbox-scheduler-card*
git commit -m "feat: observe sandbox runner safely"
```

### Task 14: Package Runner and extend the dev release transaction

**Files:**

- Create: `backend/Dockerfile.sandbox-runner`
- Modify: `.github/workflows/deploy-dev.yml`
- Modify: `deploy/dev/docker-compose.yml`
- Create: `deploy/dev/docker-compose.runner-2c4g.yml`
- Modify: `deploy/dev/.env.example`
- Modify: `deploy/dev/deploy.sh`
- Modify: `deploy/dev/tests/compose_contract_test.sh`
- Modify: `deploy/dev/tests/workflow_contract_test.sh`
- Modify: `deploy/dev/tests/deploy_test.sh`
- Modify: `deploy/dev/tests/image_contract_test.sh`

- [ ] **Step 1: Extend failing deployment contract tests first**

Require three application images plus the runtime image:

```text
coze-server
coze-web
coze-sandbox-runner
coze-sandbox-runtime
```

The workflow must build revision tags, inspect revision labels/digests, promote
all images only after verification, and send all immutable references to the
deploy hook. The deploy script must pull, start, health-check, record, and
rollback server/web/Runner as one revision. The execution runtime must be
passed to Runner by digest.

The existing `deploy/dev/docker-compose.yml` local-data mode remains
source-compatible. The new `docker-compose.runner-2c4g.yml` is a complete
remote-data deployment file containing only `nsqd`, `coze-server`,
`coze-sandbox-runner`, and `coze-web`; it must not start OceanBase, MySQL,
Redis, Elasticsearch, or object-storage containers. `deploy.sh` selects it only
when `DEPLOY_PROFILE=runner-2c4g`, so rollout does not silently change existing
servers.

Run:

```bash
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/image_contract_test.sh
```

Expected: FAIL because Runner images/services are not present.

- [ ] **Step 2: Build the Runner image**

Use a multi-stage Go build for `./cmd/sandbox-runner`; final image runs as UID
10001, contains CA certificates only, exposes 9443, and has revision/source
labels. It does not contain the NewX server binary or execution toolchains.

- [ ] **Step 3: Add the co-located 2C4G compose service**

Mount only:

```text
read-only Runner app.env/secret files
read-only TLS certificate and key
the dedicated rootless Podman/Docker-compatible Unix socket
```

Set Runner container memory to 192 MiB, CPU to 0.20, PIDs to 64, and
`no-new-privileges`. Do not mount `/var/run/docker.sock`. NewX reaches Runner
over the private compose network using HTTPS. The Provider endpoint/credential
remain database-managed rather than hardcoded in Compose.

- [ ] **Step 4: Extend CI promotion and deployment rollback**

Promotion must be all-or-nothing across server, web, Runner, and runtime image.
Rollback restores the previous Runner and its previous runtime digest before
declaring success. Health checks require `/v1/health` plus a rootless runtime
probe; they must not create a user execution.

- [ ] **Step 5: Re-run deployment tests**

```bash
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/image_contract_test.sh
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/Dockerfile.sandbox-runner .github/workflows/deploy-dev.yml deploy/dev
git commit -m "feat: deploy sandbox runner with dev"
```

### Task 15: Update long-term execution facts and operations runbook

**Files:**

- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Modify: `docs/superpowers/specs/2026-08-11-sandbox-runner-2c4g-design.md`

- [ ] **Step 1: Update the current facts, not implementation history**

Document that Agent execution remains Eino ADK and only code/MCP/AppDev/Plugin
sandbox calls cross the Provider boundary. Add the authoritative identity
propagation edge from `RunProcessor.processRun` to the Sandbox execution
context without representing Redis Runner queue as the Workbench Run queue.
Workbench Run queue remains MySQL.

- [ ] **Step 2: Add operational procedures**

Cover key rotation, configuration push, desired/applied mismatch, drain,
quarantine disposal, Redis loss, Runner restart, runtime image change, 2C4G
watermark diagnosis, emergency stop, separate-node migration, and rollback.
Use placeholders only for operator-supplied secret values in command examples;
never include real credentials.

- [ ] **Step 3: Verify the managed Workbench graph**

```bash
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Expected: all three commands pass and every current contract node retains an
`anchored_in` path to real source.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/context/project-context.md docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json docs/superpowers/runbooks/sandbox-control-plane-operations.md docs/superpowers/specs/2026-08-11-sandbox-runner-2c4g-design.md
git commit -m "docs: operationalize sandbox runner"
```

### Task 16: Run full acceptance and staged rollout

**Files:**

- Create: `backend/internal/sandboxrunner/e2e_test.go`
- Create: `backend/internal/sandboxrunner/security_e2e_test.go`
- Create: `backend/internal/sandboxrunner/recovery_e2e_test.go`
- Create: `deploy/sandbox-runner/tests/acceptance.sh`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`

- [ ] **Step 1: Run full automated tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./domain/sandbox ./infra/sandbox ./application/sandbox ./internal/sandboxrunner/... ./infra/coderunner/impl/controlplane ./application/agentthread ./application/appdev ./infra/appdev ./application/plugin ./domain/workflow/internal/nodes/code ./api/handler/coze ./api/router/coze -count=1
cd ../frontend/apps/coze-studio
npx vitest run src/pages/system/__tests__/sandbox-*.test.tsx src/pages/system/__tests__/sandbox-service.test.ts
cd ../../..
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/image_contract_test.sh
git diff --check
```

Expected: every command passes.

- [ ] **Step 2: Run rootless 2C4G acceptance**

Run `deploy/sandbox-runner/tests/acceptance.sh` on a Linux cgroup-v2 host
limited to 2 CPU/4 GiB. It must verify:

- one heavy task runs while heavy/light peers wait;
- two light tasks run concurrently and a third waits;
- workspace round-robin and user-heavy limit;
- queue cancellation and all default deadlines;
- Agent/AppDev/MCP reuse and Plugin immediate destruction;
- no cross-tenant files/processes/env/network;
- metadata/private/host-gateway/rootless-socket access denied;
- Redis restart, Runner restart, ambiguous submit lookup, key rotation, drain,
  quarantine, and object-storage publish-only retry;
- host available memory never falls below the configured reserve because of a
  new dispatch; no OOM kill is used as normal admission control.

Expected: the script prints one PASS line for each invariant and exits 0.

- [ ] **Step 3: Perform browser acceptance in rollout order**

Using the in-app browser and an authorized dev account, record URL, account,
space, visible state, interaction, and console/network errors for:

1. Plugin debug and MCP stdio;
2. Agent code/tool execution;
3. AppDev runtime/build;
4. `/system/sandboxes` settings, queue status, desired/applied version, drain,
   and quarantine aggregates.

Do not re-enable the hidden AppDev workspace menu; use the existing authorized
direct/test entrypoint.

- [ ] **Step 4: Stop on any rollout-slice failure**

Disable routing for the failing scope/default Provider, keep the healthy prior
slices, and do not enable the next slice. Never switch to host runtime.

- [ ] **Step 5: Commit acceptance assets**

```bash
git add backend/internal/sandboxrunner/*e2e_test.go deploy/sandbox-runner/tests/acceptance.sh docs/superpowers/runbooks/sandbox-control-plane-operations.md
git commit -m "test: accept sandbox runner on 2c4g"
```

---

## Final integration gate

Before requesting merge approval:

```bash
git fetch origin dev
git diff --stat origin/dev...HEAD
git diff --check origin/dev...HEAD
git log --oneline origin/dev..HEAD
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
```

Then run the Task 16 test matrix against the fetched `origin/dev` baseline. If
the baseline SHA, migration interval, changed-file scope, or release workflow
changes, repeat the affected integration checks and present the new exact SHAs
to the user. Do not merge, apply migrations, publish, push `dev`, or operate the
remote Runner without separate explicit approval for that exact range.

## Expected commit sequence

```text
feat: add sandbox runner optional protocol contracts
feat: persist sandbox scheduler settings
feat: manage sandbox scheduler settings
feat: expose sandbox scheduler controls
feat: split sandbox admission and execution capacity
feat: add authenticated sandbox runner api
feat: persist sandbox runner queue state
feat: schedule sandbox work fairly on 2c4g
feat: manage isolated sandbox lifecycles
feat: execute sandboxes through rootless containers
feat: route sandbox workloads with tenant identity
feat: distribute signed sandbox scheduler config
feat: observe sandbox runner safely
feat: deploy sandbox runner with dev
docs: operationalize sandbox runner
test: accept sandbox runner on 2c4g
```

Each commit must pass its focused tests. Phase gates must pass before starting
the next phase so protocol/storage, scheduling, container isolation, and release
changes remain independently reviewable and revertible.
