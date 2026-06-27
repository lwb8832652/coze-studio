# Guardrail Runtime Tool Wrapper M8 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the M8 Guardrail enforcement gate into ADK runtime tool
invocation so MCP, web, and other runtime catalog tools are checked before
execution.

**Architecture:** Add a catalog wrapper in `backend/application/agentthread`
that decorates `ADKRuntimeToolDefinition.Invoker`. The wrapper builds a
metadata-only `GuardrailRequest`, calls `GuardrailEnforcer`, and invokes the
original tool only for allow/warn outcomes. Default ADK tool provider wiring can
opt into the wrapper through a provider option.

**Tech Stack:** Go, Eino ADK runtime tool catalog boundary, focused unit tests.

---

### Task 1: RED Runtime Wrapper Tests

**Files:**
- Create: `backend/application/agentthread/adk_guardrail_runtime_tool_catalog_test.go`

- [x] Write a failing test that an allow decision invokes the original runtime
  tool and passes a metadata-only request to the enforcer.
- [x] Write a failing test that a deny decision does not invoke the original
  runtime tool and returns a sanitized typed error.
- [x] Write a failing test that a confirm decision does not invoke the original
  runtime tool and returns a confirmation-required typed error.
- [x] Write a failing default-provider wiring test showing MCP runtime tools
  can be wrapped by the Guardrail enforcer.
- [x] Run focused tests and verify they fail because the wrapper does not exist.

### Task 2: Runtime Wrapper Implementation

**Files:**
- Create: `backend/application/agentthread/adk_guardrail_runtime_tool_catalog.go`
- Modify: `backend/application/agentthread/adk_human_interaction.go`

- [x] Add a small `ADKGuardrailEnforcer` interface implemented by
  `GuardrailEnforcer`.
- [x] Add `ADKGuardrailRuntimeToolCatalog` that wraps each
  `ADKRuntimeToolDefinition.Invoker` without changing tool name, description,
  schema, or visibility.
- [x] Build `GuardrailRequest` using only run/thread/space/creator identity,
  target type `tool_call`, target ID as the tool name, operation `invoke`,
  source `adk_runtime_tool`, and fail mode `fail_closed`.
- [x] Short-circuit denied, confirmation-required, and audit-failed decisions
  before original invocation. Do not pass tool arguments to Guardrail metadata.
- [x] Add `WithDefaultADKToolProviderGuardrailEnforcer` and wrap the composite
  runtime catalog only when an enforcer is provided.
- [x] Run focused tests until green.

### Task 3: Guidance And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document M8.4 as runtime catalog tool wrapping only; Skill middleware,
  subagent tools, human confirmation checkpoint integration, scanner adapters,
  UI review, metrics, and OpenTelemetry linkage remain later slices.
- [x] Run focused backend tests.
- [x] Run `gofmt`.
- [x] Run `git diff --check`.
