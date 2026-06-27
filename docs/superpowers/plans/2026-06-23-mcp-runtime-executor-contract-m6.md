# MCP Runtime Executor Contract M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an injectable MCP runtime executor contract behind the ADK MCP
runtime catalog while keeping production execution disabled.

**Architecture:** Keep MCP registry discovery, ADK tool wrapping, and actual
MCP execution as separate layers. The runtime catalog maps safe registry rows
to `ADKMCPRuntimeToolCall`, then delegates to an optional executor. Missing
executor and executor failures are fail-closed and sanitized.

**Tech Stack:** Go, Eino ADK runtime tool catalog.

---

## Result

M6.5 adds the executor contract needed for future real MCP transport execution.
The default runtime remains non-executable until an executor is explicitly
injected.

## Completed

- Added `ADKMCPRuntimeToolCall` and `ADKMCPRuntimeToolExecutor`.
- Added `WithADKMCPRuntimeToolExecutor` for `ADKMCPRuntimeToolCatalog`.
- Added `WithDefaultADKToolProviderMCPExecutor` for default ADK provider
  wiring.
- Updated MCP runtime tool definitions to carry safe registry name, server ID,
  and raw configured MCP tool name into the executor call.
- Kept missing executor behavior fail-closed.
- Sanitized executor errors so tool-facing errors include only the safe tool
  name, not MCP arguments, server names, config, auth, URLs, object keys, or
  provider diagnostics.
- Added focused tests for executor invocation identity, sanitized executor
  errors, default fail-closed behavior, and default provider executor wiring.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeToolCatalog|TestDefaultADKToolProviderCanWireMCP' -count=1`

## Remaining

Real MCP stdio/SSE/HTTP transport, Eino MCP adapter conversion, OAuth/session
lifecycle, encrypted secret retrieval, authorization, audit records,
timeout/output budgets, health probes, frontend policy controls, and browser
E2E remain open M6 or production acceptance work.
