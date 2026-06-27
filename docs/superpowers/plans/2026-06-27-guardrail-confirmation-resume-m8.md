# Guardrail Confirmation Resume M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Guardrail confirmation interrupts resume safely: approved
responses execute the original guarded tool once, rejected responses fail
closed without invoking it, and resumed calls do not re-run the Guardrail
provider.

**Architecture:** Reuse Eino checkpoint/resume semantics and the existing
`HumanInteractionResponse` schema. Guardrail wrappers inspect
`humanInteractionToolState` before evaluating the provider. A matching approved
response proceeds to the wrapped runtime/subagent tool; a rejected response
returns a sanitized typed error; missing resume data re-interrupts with the
saved prompt.

**Tech Stack:** Go, Eino compose checkpoint/resume, `tool.GetInterruptState`,
`tool.GetResumeContext`, existing human-interaction schema.

---

### Task 1: RED Resume Tests

**Files:**
- Add: `backend/application/agentthread/adk_guardrail_confirmation_resume_test.go`

- [x] Add a runtime-tool graph resume test proving approved confirmation
  executes the original tool once and does not evaluate Guardrail again.
- [x] Add a runtime-tool graph resume test proving rejected confirmation does
  not execute the original tool and returns a sanitized typed rejection error.
- [x] Add a subagent graph resume test proving approved confirmation executes
  the original subagent tool once and does not evaluate Guardrail again.
- [x] Add a subagent graph resume test proving rejected confirmation does not
  execute the original subagent tool and returns a sanitized typed rejection
  error.

### Task 2: Resume Implementation

**Files:**
- Modify: `backend/application/agentthread/adk_guardrail_confirmation.go`
- Modify: `backend/application/agentthread/adk_guardrail_runtime_tool_catalog.go`
- Modify: `backend/application/agentthread/adk_subagent_tool_provider.go`
- Modify: `backend/application/agentthread/guardrail_enforcer.go`

- [x] Add a typed sanitized `GuardrailConfirmationRejectedError`.
- [x] Add a shared helper that handles interrupted Guardrail confirmation
  state, resume data, validation, approved/rejected decisions, and re-interrupt
  when the current tool is not the resume target.
- [x] Call the helper before evaluating Guardrail in runtime tool wrappers.
- [x] Call the helper before evaluating Guardrail in subagent wrappers.
- [x] Ensure approved resumes bypass a second provider/audit evaluation but
  still invoke the original wrapped tool normally.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M8.8 as Guardrail confirmation resume for runtime/subagent
  tools only; Skill confirmation interrupts, scanner adapters, UI review,
  metrics, and OpenTelemetry linkage remain later slices.
- [x] Run focused backend tests.
- [x] Run `gofmt`.
- [x] Run `git diff --check`.
