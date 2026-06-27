# MCP Runtime Transport Router M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a server-type transport router behind the MCP runtime executor,
while keeping all real MCP transports disabled by default.

**Architecture:** `ADKMCPRuntimeTransportRouter` implements
`ADKMCPRuntimeTransportInvoker`, normalizes durable MCP `server_type`, and
dispatches only to explicitly configured handlers. Missing or unsupported
transports return sanitized fail-closed errors.

**Tech Stack:** Go, Eino ADK runtime tool boundary, Workbench MCP runtime
service.

---

## Result

M6.7 adds the transport selection seam needed before real stdio/SSE/HTTP MCP
session work, without enabling any command execution or network access by
default.

## Completed

- Added `ADKMCPRuntimeTransportRouterOptions` with explicit `Stdio`, `SSE`,
  and `StreamableHTTP` handler slots.
- Added `ADKMCPRuntimeTransportRouter` implementing
  `ADKMCPRuntimeTransportInvoker`.
- Added `ADKMCPRuntimeTransportInvokerFunc` for tests and future adapter
  wiring.
- Normalized `server_type` values for `stdio`, `sse`, `streamable_http`,
  `streamable-http`, and `http`.
- Kept unsupported and unconfigured transports fail-closed with fixed
  sanitized errors.
- Verified router errors and executor-composed transport failures do not leak
  MCP arguments, server names, config, auth, endpoint URLs, or secret-adjacent
  data.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeTransportRouter|TestADKMCPRuntimeExecutorWithTransportRouter' -count=1`

## Remaining

Concrete stdio/SSE/streamable HTTP handlers, Eino MCP adapter conversion,
OAuth/session lifecycle, encrypted secret retrieval, stdio sandboxing, network
policy, audit persistence, health probes/history, output offload, production
bootstrap executor wiring, frontend policy controls, and browser E2E remain
open M6 or production acceptance work.
