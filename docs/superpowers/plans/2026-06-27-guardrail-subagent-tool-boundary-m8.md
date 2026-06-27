# Guardrail Subagent Tool Boundary M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run metadata-only Guardrail checks before ADK subagent `AgentTool`
invocations so denied or confirmation-required subagent work is blocked before
child run rows or lifecycle events are created.

**Architecture:** Add an optional Guardrail enforcer to `ADKSubagentToolProvider`
and wire it from the default ADK tool provider. The wrapper sits outside the
timeout and lifecycle wrappers so it evaluates first. Reuse the existing
human-interaction confirmation interrupt helper for `confirm` decisions.

**Tech Stack:** Go, Eino `AgentTool`, existing Guardrail gate, existing
`HumanInteractionPrompt` confirmation schema.

---

### Task 1: RED Subagent Guardrail Tests

**Files:**
- Modify: `backend/application/agentthread/adk_subagent_tool_provider_test.go`
- Modify: `backend/application/agentthread/adk_default_tool_provider_test.go`

- [x] Add a failing allow-path test proving subagent Guardrail receives only
  run identity, target type `tool_call`, subagent name, operation `invoke`,
  source `adk_subagent_tool`, and fail-closed policy.
- [x] Add a failing deny-path test proving denied subagent calls do not invoke
  the child agent, do not create child run rows, and do not emit lifecycle
  events.
- [x] Add a failing confirm-path test proving confirmation-required subagent
  calls return an Eino human-interaction interrupt and do not include subagent
  arguments in the prompt or error string.
- [x] Add a failing default-provider wiring assertion for the shared
  Guardrail enforcer.

### Task 2: Subagent Guardrail Implementation

**Files:**
- Modify: `backend/application/agentthread/adk_subagent_tool_provider.go`
- Modify: `backend/application/agentthread/adk_human_interaction.go`
- Modify: `backend/application/agentthread/adk_guardrail_runtime_tool_catalog.go`
- Add or modify shared helper if needed.

- [x] Add `WithADKSubagentToolProviderGuardrailEnforcer`.
- [x] Wrap subagent tools after timeout/lifecycle construction so Guardrail
  evaluates before lifecycle recording.
- [x] Build metadata-only Guardrail requests for subagent tool invocation.
- [x] Convert `confirm` decisions into stateful human-interaction interrupts.
- [x] Preserve `deny` and audit failures as sanitized typed errors.
- [x] Keep original subagent execution unchanged for `allow` and `warn`.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M8.7 as subagent tool invocation Guardrail only; full resume
  approval execution, Skill confirmation interrupts, scanner adapters, UI
  review, metrics, and OpenTelemetry linkage remain later slices.
- [x] Run focused backend tests.
- [x] Run `gofmt`.
- [x] Run `git diff --check`.
