# Agent Tool Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the Go-native tool calling foundation that lets the agent harness execute registered tools and persist tool step events.

**Architecture:** Keep the first tool layer inside `backend/application/agentthread` so it can be used by the current Go Agent Harness without introducing MCP configuration or UI scope. A small registry resolves tool names to handlers, the harness adds a `tool` step type, and existing run events become the durable tool execution timeline for later frontend/MCP work.

**Tech Stack:** Go, existing agentthread application package, existing `agent_run_events` event sink, existing Go test stack.

---

## Scope

- Add a `ToolRegistry` abstraction with in-memory registration and lookup.
- Add a `ToolCall` / `ToolResult` protocol with JSON argument/result strings.
- Add `AgentStepTypeTool` and tool-specific fields on `AgentStep`.
- Add a `ToolStepRunner` that invokes registered tools and returns structured metadata.
- Emit `tool.started`, `tool.completed`, and `tool.failed` events through the existing run event sink.
- Keep `ModelStepRunner` behavior unchanged for current production runs.
- Add unit tests for registry behavior, successful tool execution, missing-tool failures, and harness event emission.

## Non-Goals

- No MCP server configuration or remote tool invocation.
- No frontend tool configuration page.
- No persisted tool catalog table.
- No LangGraph compatibility API.
- No security scanner or sandbox policy.
- No planner changes that automatically choose tools from model output.

## Event Contract

- `tool.started` payload:
  - `step_id`, `step_type`, `step_name`, `step_index`
  - `tool_name`
  - `arguments_present`
- `tool.completed` payload:
  - `step_id`, `step_type`, `step_name`, `step_index`
  - `tool_name`
  - `result_present`
- `tool.failed` payload:
  - `step_id`, `step_type`, `step_name`, `step_index`
  - `tool_name`
  - `error_message`

## Testing

- `ToolRegistry` tests verify registration rejects empty names and lookup returns the registered handler.
- `ToolStepRunner` tests verify successful calls pass name/arguments/run/state to the handler.
- `ToolStepRunner` tests verify missing tools return a clear unsupported-tool error.
- `HarnessExecutor` tests verify tool steps emit started/completed/failed events and include tool metadata in the final run result.
