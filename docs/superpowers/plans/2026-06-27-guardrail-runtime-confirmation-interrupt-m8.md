# Guardrail Runtime Confirmation Interrupt M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert runtime-tool Guardrail `confirm` decisions into existing Eino
human-interaction confirmation interrupts so task runs can pause with a safe
approval prompt instead of failing as a plain tool error.

**Architecture:** Reuse the existing `HumanInteractionPrompt` schema and
`tool.StatefulInterrupt`. The ADK runtime tool Guardrail wrapper builds a
metadata-only confirmation prompt when the enforcer returns confirmation
required. The wrapper still does not execute the original tool until a future
resume slice handles approval data.

**Tech Stack:** Go, Eino `tool.StatefulInterrupt`, existing human interaction
prompt schema, focused unit tests.

---

### Task 1: RED Confirmation Interrupt Tests

**Files:**
- Modify: `backend/application/agentthread/adk_guardrail_runtime_tool_catalog_test.go`

- [x] Write a failing test that a Guardrail confirm decision does not invoke
  the original runtime tool and returns an Eino interrupt with a
  `HumanInteractionPrompt` of kind `confirmation`.
- [x] Write a failing test that the prompt is metadata-only and does not include
  tool arguments, prompts, model text, URLs, filenames, object URIs,
  credentials, checkpoint bytes, scanner raw payloads, or provider raw bodies.
- [x] Run focused tests and verify they fail because confirm currently returns
  a typed error instead of a stateful interrupt.

### Task 2: Runtime Confirmation Implementation

**Files:**
- Modify: `backend/application/agentthread/adk_guardrail_runtime_tool_catalog.go`

- [x] Add a helper that converts a `GuardrailRequest` plus
  `GuardrailDecision` into a bounded `HumanInteractionPrompt`.
- [x] Use `tool.StatefulInterrupt` with `humanInteractionToolState` when the
  enforcer returns `GuardrailConfirmationRequiredError` or a confirmation
  result.
- [x] Keep denied and audit-failed decisions as sanitized typed errors.
- [x] Ensure prompt fields expose only safe target identity, action, provider,
  reason code, and rule IDs.
- [x] Run focused tests until green.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M8.6 as runtime-tool confirmation interrupt only; full resume
  approval execution, Skill confirmation interrupts, subagent tools, scanner
  adapters, UI review, metrics, and OpenTelemetry linkage remain later slices.
- [x] Run focused backend tests.
- [x] Run `gofmt`.
- [x] Run `git diff --check`.
