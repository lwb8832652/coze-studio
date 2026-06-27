# Eino ADK Tool Repair Error Normalization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Eino ADK tool repair and tool errors production-safe by using Eino `patchtoolcalls`, Coze-owned schemas, and stable Coze run events.

**Architecture:** Keep Eino as the execution primitive and add a thin Coze middleware adapter for normalization. ADK event mapping owns public event semantics and converts normalized tool result JSON into `tool.failed`.

**Tech Stack:** Go, Eino ADK v0.9.9, Coze `RunEventSink`, existing `agentthread` test helpers.

---

### Task 1: Tool Result Schema Helpers

**Files:**
- Create: `backend/application/agentthread/adk_tool_result_protocol.go`
- Test: `backend/application/agentthread/adk_tool_result_protocol_test.go`

- [x] Write tests for `coze.tool_error.v1` JSON encoding, decoding, empty errors, and truncation.
- [x] Run `cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKToolResultProtocol'`.
- [x] Implement minimal helpers and constants.
- [x] Re-run the focused protocol tests.

### Task 2: Patch Tool Calls With Coze Repair Payloads

**Files:**
- Modify: `backend/application/agentthread/adk_middleware.go`
- Test: `backend/application/agentthread/adk_middleware_test.go`

- [x] Write a failing test proving dangling tool calls are patched with `coze.tool_repair.v1` and emit `tool.repaired`.
- [x] Run the focused test and confirm it fails with the current default Eino message.
- [x] Configure `patchtoolcalls.New` with a Coze generator that emits content-free repair events.
- [x] Re-run the focused test.

### Task 3: Tool Error Normalization Middleware

**Files:**
- Create: `backend/application/agentthread/adk_tool_error_normalization.go`
- Modify: `backend/application/agentthread/adk_middleware.go`
- Test: `backend/application/agentthread/adk_tool_error_normalization_test.go`

- [x] Write failing tests for invokable, streamable, enhanced invokable, and enhanced streamable endpoint errors.
- [x] Run the focused tests and confirm each missing behavior fails.
- [x] Implement the ADK middleware wrapper methods.
- [x] Add `tool_error_normalization` to middleware order after `reduction` and before policy/audit/usage.
- [x] Re-run the focused tests and middleware order tests.

### Task 4: Event Mapping For Failed Tool Results

**Files:**
- Modify: `backend/application/agentthread/adk_event_mapper.go`
- Test: `backend/application/agentthread/adk_event_mapper_test.go`

- [x] Write failing tests for normalized string tool failures and enhanced text-part failures.
- [x] Run the focused mapper tests and confirm they fail.
- [x] Extract text from `UserInputMultiContent` and map `coze.tool_error.v1` to `tool.failed`.
- [x] Re-run the focused mapper tests.

### Task 5: Documentation And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document the Coze tool repair/error contracts in `AGENTS.md`.
- [x] Update the DeerFlow parity roadmap evidence and remove this item from the open gap list.
- [x] Run `cd backend && go test -gcflags="all=-l -N" ./application/agentthread`.
- [x] Run `git diff --check`.
