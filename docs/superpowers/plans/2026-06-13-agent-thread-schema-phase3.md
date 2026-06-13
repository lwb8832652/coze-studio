# Agent Thread Schema Phase 3 Implementation Plan

**Goal:** Add deployable database schema for `agent_threads` before workbench writes use the new thread persistence path.

**Architecture:** Keep this phase limited to Atlas assets. The Go repository already defines the table contract and sqlite tests cover repository behavior. This phase makes that contract available to MySQL environments through the existing Atlas migration workflow.

**Scope**

This phase implements:

1. A new Atlas migration file that creates `agent_threads`.
2. A matching `opencoze_latest_schema.hcl` table definition.
3. Atlas migration hash refresh.

This phase does not implement:

1. Workbench write-through from `chat_tasks` to `agent_threads`.
2. HTTP/IDL endpoints.
3. run/message/event persistence tables.

## Task 1: Add `agent_threads` Migration

- [ ] Create `docker/atlas/migrations/20260613000100_agent_threads.sql`.
- [ ] Define columns matching `backend/domain/agentthread/repository/threadPO`.
- [ ] Add indexes for space+updated, space+status, creator+updated, and legacy task lookup.

## Task 2: Update Latest Schema

- [ ] Add table `agent_threads` to `docker/atlas/opencoze_latest_schema.hcl`.
- [ ] Keep nullability/defaults aligned with the SQL migration.

## Task 3: Verify Atlas Assets

- [ ] Refresh `docker/atlas/migrations/atlas.sum`.
- [ ] Run a migration hash/check command through Atlas.
- [ ] Run existing Go repository/application tests to confirm schema-facing code still passes.

## Acceptance Checklist

- [ ] `agent_threads` exists in migrations.
- [ ] `agent_threads` exists in latest schema HCL.
- [ ] Migration hash is refreshed.
- [ ] Focused backend tests still pass.
