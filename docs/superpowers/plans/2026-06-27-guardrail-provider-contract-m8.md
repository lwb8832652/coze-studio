# Guardrail Provider Contract M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first Go-native guardrail provider contract for M8 security
work, covering allow, warn, confirm, deny, fail-open, and fail-closed
decisions without wiring runtime execution yet.

**Architecture:** `backend/application/agentthread` owns the application-layer
contract used by future Skill, MCP, file, network, command, and tool-call
scanners. Providers return metadata-only decisions. A chain evaluator merges
provider decisions with deny > confirm > warn > allow precedence and applies a
per-request fail mode when a provider errors.

**Tech Stack:** Go, application/agentthread package, focused unit tests.

---

### Task 1: Failing Guardrail Contract Coverage

**Files:**
- Create: `backend/application/agentthread/guardrail_provider_test.go`

- [x] Add a table test for decision precedence: deny overrides confirm, confirm
  overrides warn, warn overrides allow, and all-allow stays allow.
- [x] Add fail-open and fail-closed tests for provider errors.
- [x] Add metadata sanitization tests to ensure decision payloads bound provider
  name, reason code, message, rule IDs, and metadata values without echoing
  prompt text, tool arguments, object URIs, credentials, checkpoint bytes, or
  provider raw payloads.

### Task 2: Guardrail Contract Implementation

**Files:**
- Create: `backend/application/agentthread/guardrail_provider.go`

- [x] Define `GuardrailAction` constants: `allow`, `warn`, `confirm`,
  and `deny`.
- [x] Define `GuardrailFailMode` constants: `fail_open` and `fail_closed`.
- [x] Define metadata-only `GuardrailRequest`, `GuardrailDecision`, and
  `GuardrailProvider`.
- [x] Implement `GuardrailProviderFunc` for tests and adapters.
- [x] Implement `ChainGuardrailProvider` with deterministic decision
  precedence and fail mode behavior.
- [x] Implement decision normalization that trims and bounds reason code,
  provider, user-facing message, rule IDs, and metadata keys/values.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M8.1 as contract-only and note runtime enforcement, audit
  persistence, scanners, and human confirmation integration as follow-up.
- [x] Run focused guardrail Go tests.
- [x] Run `gofmt`.
- [x] Run `git diff --check`.
