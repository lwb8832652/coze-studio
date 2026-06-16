# Agent Token Usage Phase 26 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist and query Agent Harness token usage by run, thread, step, source, and model.

**Architecture:** Extend the existing `agentthread` domain with an `agent_token_usage` table and small repository/service APIs for recording usage rows and aggregating totals. Add an application-layer `ThreadUsageCollector` adapter, then wire it into `HarnessExecutorOptions` so step metadata with usage can be persisted without coupling the harness to MySQL.

**Tech Stack:** Go, Hertz application services, GORM, Atlas migrations, SQLite-backed repository tests, existing `agentthread` harness tests.

---

## Scope

Implement:

1. `agent_token_usage` persistence table and Atlas schema entry.
2. Domain entity/repository/service for usage recording and run/thread aggregate queries.
3. Application DTO methods for `RecordTokenUsage`, `GetRunTokenUsage`, and `GetThreadTokenUsage`.
4. `UsageCollector` hook in `HarnessExecutor`.
5. `ThreadUsageCollector` adapter wired into default Agent Harness initialization.

Do not implement in this phase:

1. Frontend token indicator.
2. Public HTTP API routes.
3. Provider-specific token extraction for OpenAI/Ark/Ollama.
4. Pricing tables or automatic cost calculation.
5. Subagent runtime attribution beyond the storage/source fields.
6. LangGraph API response fields.

## Data Model

`agent_token_usage` stores one row per attributable usage event:

```sql
CREATE TABLE IF NOT EXISTS `agent_token_usage` (
  `id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `run_id` bigint NOT NULL,
  `space_id` bigint NOT NULL,
  `source` varchar(32) NOT NULL,
  `step_id` varchar(128) NOT NULL DEFAULT '',
  `step_index` int NOT NULL DEFAULT 0,
  `step_name` varchar(128) NOT NULL DEFAULT '',
  `model_name` varchar(128) NOT NULL DEFAULT '',
  `provider` varchar(64) NOT NULL DEFAULT '',
  `input_tokens` bigint NOT NULL DEFAULT 0,
  `output_tokens` bigint NOT NULL DEFAULT 0,
  `total_tokens` bigint NOT NULL DEFAULT 0,
  `cost_micros` bigint NOT NULL DEFAULT 0,
  `currency` varchar(16) NOT NULL DEFAULT '',
  `estimated` tinyint(1) NOT NULL DEFAULT 0,
  `raw_usage` json DEFAULT NULL,
  `metadata` json DEFAULT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_token_usage_thread_run` (`thread_id`, `run_id`),
  KEY `idx_agent_token_usage_space_source` (`space_id`, `source`),
  KEY `idx_agent_token_usage_run_source` (`run_id`, `source`),
  KEY `idx_agent_token_usage_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

`source` values:

1. `lead_agent`
2. `subagent`
3. `middleware`
4. `tool`

## Tasks

### Task 1: Repository RED/GREEN

**Files:**

- Modify: `backend/domain/agentthread/entity/thread.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`

- [ ] Write a failing repository test `TestThreadRepositoryCreateAndAggregateTokenUsage`.
- [ ] Verify RED with `cd backend && go test ./domain/agentthread/repository -run TestThreadRepositoryCreateAndAggregateTokenUsage -count=1`.
- [ ] Add `TokenUsageSource`, `TokenUsage`, and `TokenUsageAggregate` entities.
- [ ] Add repository methods:
  - `CreateTokenUsage(ctx, usage)`
  - `ListTokenUsage(ctx, ListTokenUsageRequest)`
  - `AggregateTokenUsage(ctx, AggregateTokenUsageRequest)`
- [ ] Add `tokenUsagePO`, JSON validation, conversion helpers, and aggregate query.
- [ ] Verify GREEN with the same repository test.

### Task 2: Domain Service RED/GREEN

**Files:**

- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [ ] Write failing service tests for `RecordTokenUsage`, `GetRunTokenUsage`, and `GetThreadTokenUsage`.
- [ ] Verify RED with `cd backend && go test ./domain/agentthread/service -run 'Test(RecordTokenUsage|GetRunTokenUsage|GetThreadTokenUsage)' -count=1`.
- [ ] Implement validation:
  - `thread_id`, `run_id`, and content source are required for record.
  - `input_tokens`, `output_tokens`, `total_tokens`, and `cost_micros` cannot be negative.
  - If `total_tokens` is zero, compute `input_tokens + output_tokens`.
  - `lead_agent`, `subagent`, `middleware`, and `tool` are the only valid sources.
  - Persisted usage uses the run's `space_id` and `thread_id`.
- [ ] Implement run/thread aggregate reads with default page size 100 for rows.
- [ ] Verify GREEN.

### Task 3: Application Usage APIs RED/GREEN

**Files:**

- Modify: `backend/application/agentthread/dto.go`
- Modify: `backend/application/agentthread/thread_app.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/service_test.go`

- [ ] Write failing application tests that map domain usage records and aggregates.
- [ ] Verify RED with `cd backend && go test ./application/agentthread -run 'TestApplicationTokenUsageMethodsMapDomainUsage' -count=1`.
- [ ] Add app DTOs:
  - `TokenUsageSource`
  - `TokenUsageSummary`
  - `TokenUsageAggregateSummary`
  - `RecordTokenUsageRequest/Response`
  - `GetTokenUsageRequest/Response`
- [ ] Implement app methods that call the domain service and map summaries.
- [ ] Verify GREEN.

### Task 4: Harness Collector RED/GREEN

**Files:**

- Modify: `backend/application/agentthread/harness.go`
- Modify: `backend/application/agentthread/harness_test.go`
- Create: `backend/application/agentthread/usage_collector.go`
- Create: `backend/application/agentthread/usage_collector_test.go`

- [ ] Write a failing harness test where a step result metadata payload contains:

```json
{
  "usage": {
    "input_tokens": 12,
    "output_tokens": 8,
    "total_tokens": 20,
    "model_name": "gpt-test",
    "provider": "openai-compatible",
    "estimated": false,
    "raw_usage": {"prompt_tokens": 12, "completion_tokens": 8}
  }
}
```

- [ ] Verify RED because no collector hook exists.
- [ ] Add `UsageCollector` and `AgentTokenUsage` application-level structs.
- [ ] Parse `usage` from step result metadata; ignore empty usage.
- [ ] Default source to `lead_agent` for model steps and `tool` for tool steps.
- [ ] Record step id, name, index, tool name, and raw usage JSON.
- [ ] Emit `usage.recorded` event with aggregate token count after successful collection.
- [ ] Verify GREEN.

### Task 5: Wiring, Migration, And Verification

**Files:**

- Modify: `backend/application/application.go`
- Create: `docker/atlas/migrations/20260616000100_agent_token_usage.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`

- [ ] Wire `agentthread.NewThreadUsageCollector(primaryServices.agentThreadSVC)` into `NewHarnessExecutor`.
- [ ] Add Atlas migration and schema table.
- [ ] Regenerate `atlas.sum` using Atlas CLI if available; otherwise use Atlas Go `migrate.Checksum` from a temp module and validate with `migrate.Validate`.
- [ ] Run:
  - `cd backend && go test ./domain/agentthread/... ./application/agentthread -count=1`
  - `cd backend && go test ./application/... -count=1`
  - Atlas `migrate.Validate`
  - `git diff --check`
- [ ] Stage only Phase 26 files and commit with `feat: persist agent token usage`.

## Self-Review

The plan covers the Phase 26 backend fact source and collector. It intentionally leaves frontend indicator, public HTTP routes, provider-specific adapters, automatic pricing, and LangGraph response fields for later phases so this commit remains testable and reversible. No placeholders or unresolved scope decisions remain.
