# Guardrail Skill Load Boundary M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Guard ADK Skill middleware content loading so a selected task skill is
checked before its full instructions are returned to the agent context.

**Architecture:** Extend `adkSkillBackend` with optional Guardrail context and
call the same M8 `ADKGuardrailEnforcer` from `Get`. Keep existing backend
construction compatible through variadic options. Add middleware assembler
option wiring so future production service setup can inject the enforcer.

**Tech Stack:** Go, Eino ADK Skill middleware backend, focused unit tests.

---

### Task 1: RED Skill Guardrail Tests

**Files:**
- Modify: `backend/application/agentthread/adk_skill_backend_test.go`

- [x] Write a failing test that a skill load calls the enforcer with a
  metadata-only request and then returns content for allow/warn decisions.
- [x] Write a failing test that deny and confirm decisions return typed
  Guardrail errors before returning skill content.
- [x] Write a failing middleware assembler test proving
  `ADKMiddlewareAssemblerOptions.GuardrailEnforcer` reaches the skill backend.
- [x] Run focused tests and verify they fail because the skill guardrail
  boundary does not exist yet.

### Task 2: Skill Backend Implementation

**Files:**
- Modify: `backend/application/agentthread/adk_skill_backend.go`
- Modify: `backend/application/agentthread/adk_middleware.go`

- [x] Add variadic `ADKSkillBackendOption` support without breaking existing
  `newADKSkillBackend(skills, budget)` callers.
- [x] Add optional run/enforcer fields to `adkSkillBackend`.
- [x] In `Get`, evaluate Guardrail before returning skill content using target
  type `skill`, target ID as skill name, operation `load`, source
  `adk_skill_backend`, and fail mode `fail_closed`.
- [x] Do not pass skill body, catalog JSON, prompts, model text, tool payloads,
  object URIs, filenames, URLs, credentials, checkpoint bytes, or provider raw
  bodies to Guardrail metadata or errors.
- [x] Add `GuardrailEnforcer` to `ADKMiddlewareAssemblerOptions` and pass it
  to `newADKSkillBackend`.
- [x] Run focused tests until green.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M8.5 as Skill content-load guardrail only; Skill scanners,
  subagent tools, human confirmation checkpoint integration, UI review,
  metrics, and OpenTelemetry linkage remain later slices.
- [x] Run focused backend tests.
- [x] Run `gofmt`.
- [x] Run `git diff --check`.
