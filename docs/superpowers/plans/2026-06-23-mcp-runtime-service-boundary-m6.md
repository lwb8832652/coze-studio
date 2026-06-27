# MCP Runtime Service Boundary M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first Coze-owned MCP runtime service boundary behind the ADK
MCP executor contract, while keeping real MCP transport unimplemented.

**Architecture:** `ADKMCPRuntimeExecutor` resolves durable MCP server rows,
validates run/server/tool/arguments, wraps future transport calls with timeout
and output budget, and emits content-free lifecycle events. Workbench MCP
application service exposes an internal raw resolver method for runtime use.

**Tech Stack:** Go, Eino ADK runtime tool boundary, Workbench MCP service.

---

## Result

M6.6 adds a runtime service that can safely sit between ADK MCP tool calls and
future real MCP transport/session execution.

## Completed

- Added `ADKMCPRuntimeServerResolver`, `ADKMCPRuntimeTransportInvoker`, and
  `ADKMCPRuntimeTransportCall`.
- Added `ADKMCPRuntimeExecutor` implementing `ADKMCPRuntimeToolExecutor`.
- Added timeout and max-output-byte options.
- Added content-free lifecycle events: `mcp.tool.started`,
  `mcp.tool.completed`, and `mcp.tool.failed`.
- Added validation for active run space, server ID, raw tool name, JSON object
  arguments, server space ownership, enabled server state, and configured tool
  presence.
- Ensured failed validation does not call transport.
- Ensured oversized transport output fails closed without leaking output
  content.
- Added `mcptool.ApplicationService.ResolveADKMCPRuntimeServer` for internal
  runtime server resolution with raw config/auth preserved.
- Added focused backend tests for success, validation failures, deadline
  propagation, content-free events, output budget rejection, and raw runtime
  resolver behavior.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeExecutor|TestADKMCPRuntimeToolCatalog|TestDefaultADKToolProviderCanWireMCP' -count=1`
- `go test ./application/mcptool -run 'TestApplicationServiceResolvesRuntimeServerWithRawConfigAndAuth|TestApplicationServiceListsMCPToolRegistryEntries' -count=1`

## Remaining

Real MCP stdio/SSE/HTTP transport, Eino MCP adapter conversion, OAuth/session
lifecycle, encrypted secret retrieval, stdio sandboxing, audit persistence,
health probes/history, output offload, frontend policy controls, and browser
E2E remain open M6 or production acceptance work.
