# Guardrail Enforcement Gate M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the runtime-facing Guardrail enforcement gate that evaluates a
provider decision, records the metadata-only audit event, and maps decisions
into allow, warn, confirmation-required, or denied outcomes.

**Architecture:** `backend/application/agentthread` owns a small gate around
the M8.1 provider and M8.2 audit recorder. The gate is independent of Eino tool
wrapping for now; later slices can call it from Skill, MCP, file, network,
command, and tool-call execution paths.

**Tech Stack:** Go, application/agentthread package, focused unit tests.

---

### Task 1: RED Enforcement Tests

**Files:**
- Create: `backend/application/agentthread/guardrail_enforcer_test.go`

- [x] Write a failing test that `allow` and `warn` decisions record audit and
  return allowed outcomes, with `warn` preserving a warning flag.
- [x] Write a failing test that `deny` returns a typed denied error, records
  audit, and does not echo target IDs, prompts, object URIs, credentials,
  checkpoint bytes, URLs, filenames, or provider raw payloads in errors.
- [x] Write a failing test that `confirm` returns a typed confirmation-required
  error and records audit without creating a human-interaction checkpoint yet.
- [x] Write a failing test that audit recorder failures fail closed with a
  generic sanitized error.
- [x] Run focused tests and verify they fail because the enforcement gate does
  not exist yet.

### Task 2: Enforcement Gate Implementation

**Files:**
- Create: `backend/application/agentthread/guardrail_enforcer.go`

- [x] Add `GuardrailEnforcer`, options, and result type.
- [x] Add typed `GuardrailDeniedError`,
  `GuardrailConfirmationRequiredError`, and `GuardrailAuditFailedError`.
- [x] Evaluate the provider, map provider errors through request fail mode,
  normalize the decision, record audit, and then map action to result.
- [x] Treat audit recorder failure or missing recorder as fail-closed.
- [x] Run focused tests until green.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M8.3 as an enforcement gate only; Eino tool wrapping,
  scanner adapters, human confirmation checkpoint integration, UI review,
  metrics, and OpenTelemetry linkage remain later slices.
- [x] Run focused backend tests.
- [x] Run `gofmt`.
- [x] Run `git diff --check`.
