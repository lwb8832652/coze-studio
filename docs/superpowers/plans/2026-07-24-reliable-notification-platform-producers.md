# Reliable Notification Platform Producers Implementation Plan

> **For Codex:** REQUIRED SUB-SKILL: Use subagent-driven-development to implement this plan task-by-task.

**Goal:** Publish stable, non-noisy notifications for AppDev, MCP, plugin/resource operations, and Feishu IM.

**Architecture:** Terminal state writes publish outbox intents transactionally. Health-style producers maintain durable episode state and notify only after a configured stability threshold and once on recovery.

**Tech Stack:** Go, MySQL, AppDev provider orchestration, MCP catalog, plugin lifecycle, Feishu official Go SDK.

### Task 1: AppDev build and runtime events

**Files:**
- Modify: `backend/infra/appdev/provider_execution_repository.go`
- Modify: `backend/infra/appdev/persistent_store.go`
- Modify: `backend/application/appdev/build.go`
- Modify: `backend/application/appdev/provider_build_orchestrator.go`
- Modify: `backend/application/appdev/provider_runtime_orchestrator.go`
- Test: `backend/infra/appdev/provider_execution_repository_test.go`
- Test: `backend/application/appdev/provider_build_orchestrator_test.go`
- Test: `backend/application/appdev/provider_runtime_orchestrator_test.go`

**Steps:**
1. Append build-completed and final build-failed events inside `CompleteArtifactPublish` and `FailBuild`.
2. Append runtime-failed and runtime-recovered events inside authoritative terminal/healthy transitions.
3. Notify the project creator from persisted project data.
4. Use provider execution ID plus generation and transition for idempotency.
5. Exclude polling, start, keepalive, recovery attempts, and user-requested stop.
6. Add fencing, duplicate callback, rollback, and recovery tests.

### Task 2: MCP stable health episodes

**Files:**
- Modify: `backend/application/mcptool/catalog_mysql.go`
- Modify: `backend/application/mcptool/service.go`
- Create: `backend/domain/mcptool/health_episode.go`
- Test: `backend/application/mcptool/catalog_mysql_test.go`
- Test: `backend/application/mcptool/service_test.go`

**Steps:**
1. Persist consecutive failure count, active incident ID, incident opened time, and last recovery time with the MCP server record or a dedicated episode table.
2. Extend `UpdateHealth` to lock the server, compare prior health, update episode state, and append outbox in one transaction.
3. Open an incident only after three consecutive failed health checks.
4. Publish one unhealthy notification per incident and one recovery notification when a confirmed healthy check closes it.
5. Resolve custom-server recipients from persisted creator and workspace owner/admin policy; resolve official-server incidents only for affected workspace administrators.
6. Keep disabled MCP services out of health checking and notification production.
7. Add tests proving disabled services and transient failures cannot fail AgentThread execution or generate notifications.

### Task 3: Plugin publishing and resource copy

**Files:**
- Modify: `backend/application/plugin/lifecycle.go`
- Modify: `backend/domain/plugin/repository/plugin_impl.go`
- Create: `backend/domain/resourcecopy/entity.go`
- Create: `backend/domain/resourcecopy/repository.go`
- Create: `backend/infra/resourcecopy/mysql_repository.go`
- Modify: `backend/application/resourcecopy/service.go`
- Test: `backend/application/plugin/lifecycle_test.go`
- Test: `backend/application/resourcecopy/service_test.go`

**Steps:**
1. Publish ordinary plugin success/failure in the authoritative plugin transaction.
2. Add a durable saga journal for code-plugin publication before claiming reliable final failure.
3. Replace Redis-only resource-copy result state with a persistent task row containing actor, space, source, target, status, attempts, and terminal error code.
4. Append copy completed/failed events in the resource-copy terminal transaction.
5. Use plugin version ID or copy task ID plus terminal state for idempotency.
6. Exclude synchronous resource CRUD and intermediate retries.
7. Add replay, crash recovery, and tenant isolation tests.

### Task 4: Feishu IM stable failure and recovery

**Files:**
- Modify: `backend/domain/imchannel/entity.go`
- Modify: `backend/infra/imchannel/mysql_repository.go`
- Modify: `backend/application/imchannel/runtime.go`
- Modify: `backend/application/imchannel/agent_runner.go`
- Test: `backend/application/imchannel/runtime_test.go`
- Test: `backend/application/imchannel/agent_runner_test.go`

**Steps:**
1. Persist consecutive runtime failures, incident ID, incident notification state, and recovery state.
2. Append channel-unhealthy only after the configured stable failure threshold and channel-recovered once after reconnect.
3. Append inbound-event-dead-lettered when event processing reaches its final retry.
4. Resolve recipients from workspace owner/admin membership and channel creator/last updater.
5. Keep App Secret and provider payloads out of notification metadata and logs.
6. Add tests for transient reconnect, stable failure, duplicate callbacks, recovery, final event dead-letter, and secret redaction.

### Task 5: Keep synchronous Skill flows quiet

**Files:**
- Test: `backend/application/skill/skill_lifecycle_test.go`

**Steps:**
1. Document that current skill create, update, import, enable, disable, and delete operations are synchronous.
2. Add regression tests proving these synchronous CRUD calls do not append notification outbox rows.
3. Reserve stable event names for a future durable asynchronous skill build/import operation without exposing them in the current API.
