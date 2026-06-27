# Guardrail Audit Persistence M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the metadata-only durable audit boundary for Guardrail decisions
so later runtime enforcement, scanner adapters, and human confirmation can
share one safe security record path.

**Architecture:** `backend/application/agentthread` owns the recorder that
normalizes a `GuardrailRequest` plus `GuardrailDecision` into a content-free
audit record. `backend/domain/agentthread` owns the entity and GORM repository.
Atlas adds `agent_guardrail_audit_events` with indexed run, thread, space,
actor, target, and event fields.

**Tech Stack:** Go, GORM, SQLite-backed repository tests, Atlas v0.35.0
migrations.

---

### Task 1: Application Recorder RED Tests

**Files:**
- Create: `backend/application/agentthread/guardrail_audit_test.go`

- [x] Write a failing test that records a confirm decision and asserts the
  persisted entity contains only safe metadata: identity fields, target type,
  safe target ID, operation, source, action, fail mode, provider, reason code,
  sanitized rule IDs, event type, and created time.
- [x] Write a failing test that unsafe target IDs are redacted and recorder
  failures return a generic error that does not echo tool names, prompts,
  object URIs, credentials, checkpoint bytes, URLs, filenames, or provider raw
  data.
- [x] Run focused recorder tests and verify they fail because the audit
  recorder contract does not exist yet.

### Task 2: Domain Repository RED Tests

**Files:**
- Create: `backend/domain/agentthread/repository/guardrail_audit_test.go`

- [x] Write a failing repository test for create/list ordering by
  `created_at ASC, id ASC` and run-scoped filtering.
- [x] Write a failing repository test for bounded persisted string fields.
- [x] Run focused repository tests and verify they fail because the domain
  entity/repository contract does not exist yet.

### Task 3: Implementation

**Files:**
- Create: `backend/domain/agentthread/entity/guardrail_audit.go`
- Create: `backend/domain/agentthread/repository/guardrail_audit.go`
- Create: `backend/application/agentthread/guardrail_audit.go`

- [x] Add `GuardrailAuditEvent` with metadata-only fields.
- [x] Add `GuardrailAuditRepository`, PO mapping, create/list methods, and
  bounded field persistence.
- [x] Add `GuardrailAuditRecorder` and application recorder that uses the
  existing Guardrail normalizers, generates IDs, stamps time, redacts unsafe
  targets, and returns generic sanitized errors.
- [x] Run focused application and repository tests until green.

### Task 4: Atlas And Guidance

**Files:**
- Create: `docker/atlas/migrations/20260627000100_agent_guardrail_audit_events.sql`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Add the MySQL migration and latest schema table.
- [x] Regenerate Atlas checksums with local Atlas v0.35.0.
- [x] Validate the migration directory with local Atlas v0.35.0.
- [x] Document M8.2 as audit persistence only and leave runtime enforcement,
  scanner adapters, human confirmation interrupts, UI review, metrics, and
  OpenTelemetry linkage for later M8 slices.
- [x] Run focused backend tests.
- [x] Run `gofmt`.
- [x] Run `git diff --check`.
