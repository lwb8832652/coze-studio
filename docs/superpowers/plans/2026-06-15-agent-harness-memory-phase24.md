# Agent Harness Memory Injection Phase 24 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first production-facing memory hook to the Go-native Agent Harness by recalling memory before planning and exposing it to planners and step runners.

**Architecture:** Keep this phase backend-only and local to `backend/application/agentthread`. Introduce a small `MemoryProvider` contract, normalize recalled records into `AgentMemoryContext`, attach that context to `AgentHarnessState`, and emit a `memory.recalled` run event before the first planning call. This creates a stable integration point for later persistent short-term/long-term memory stores without coupling the harness to existing single-agent memory internals yet.

**Tech Stack:** Go, existing `agentthread` application package, existing run event sink, Go unit tests with `testify/require`.

---

## Scope

- Add an `AgentMemory` value type with `id`, `scope`, `content`, `metadata`, and `score`.
- Add a `MemoryProvider` interface with `Recall(ctx, run)` for run/thread memory retrieval.
- Add `MemoryProvider` to `HarnessExecutorOptions`.
- Add `MemoryContext` to `AgentHarnessState`.
- Recall memory once at the start of `HarnessExecutor.Execute`.
- Emit `memory.recalled` when memory exists, including count and scopes.
- Ensure cloned harness state preserves memory context for planners and step runners.

## Non-Goals

- No database schema or persistent memory repository.
- No vector retrieval or embedding pipeline.
- No frontend memory settings page.
- No memory write-back/summarization.
- No token usage accounting.
- No IM channel or LangGraph API work.

## Tasks

### Task 1: RED test for recall before planning

**Files:**
- Modify: `backend/application/agentthread/harness_test.go`
- Modify: `backend/application/agentthread/harness.go`

- [ ] Add a failing test named `TestHarnessExecutorRecallsMemoryBeforePlanning`.
- [ ] The test should create a fake memory provider returning two records.
- [ ] The planner should assert it receives `state.Memory.Items`.
- [ ] The runner should assert it receives the same memory context.
- [ ] Expected RED: compile fails because `MemoryProvider`, `AgentMemory`, and `AgentHarnessState.Memory` do not exist.

### Task 2: GREEN memory provider contract

**Files:**
- Modify: `backend/application/agentthread/harness.go`

- [ ] Add `AgentMemory`, `AgentMemoryContext`, and `MemoryProvider`.
- [ ] Add `MemoryProvider` to `HarnessExecutorOptions` and `HarnessExecutor`.
- [ ] Call `Recall` once before the planner loop.
- [ ] Trim empty memory content and ignore empty records.
- [ ] Preserve memory in `cloneHarnessState`.

### Task 3: RED/GREEN event coverage

**Files:**
- Modify: `backend/application/agentthread/harness_test.go`
- Modify: `backend/application/agentthread/harness.go`

- [ ] Assert `memory.recalled` is emitted before `step.started`.
- [ ] Assert event payload includes `memory_count` and `scopes`.
- [ ] Implement event emission using the existing `RunEventSink`.

### Task 4: Verification

**Commands:**
- `cd backend && go test ./application/agentthread`
- `cd backend && go test ./application/agentthread ./domain/agentthread/...`
- `git diff --check`

## Self-Review

- Scope stays on harness memory injection only.
- No persistent memory storage is introduced in this phase.
- The harness remains usable without a memory provider.
- Memory context is immutable from caller perspective through cloned state slices.
