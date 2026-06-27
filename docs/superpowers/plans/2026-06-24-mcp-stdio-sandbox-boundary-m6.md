# MCP Stdio Sandbox Boundary M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the stdio transport boundary behind the MCP runtime router
without enabling host command execution.

**Architecture:** `ADKMCPRuntimeStdioTransport` parses bounded stdio config,
requires an injected policy and sandbox interface, and normalizes all
policy/sandbox errors before they can reach the model-facing tool path. The
actual process/session implementation remains outside this slice.

**Tech Stack:** Go, Coze-owned ADK MCP runtime executor, MCP transport router.

---

## Result

M6.8 provides the first stdio-specific handler that future MCP execution can
mount behind `ADKMCPRuntimeTransportRouter` while staying fail-closed by
default.

## Completed

- Added `ADKMCPRuntimeStdioTransport` implementing
  `ADKMCPRuntimeTransportInvoker`.
- Added `ADKMCPRuntimeStdioPolicy` and `ADKMCPRuntimeStdioSandbox` interfaces.
- Added function adapters for future wiring and tests:
  `ADKMCPRuntimeStdioPolicyFunc` and `ADKMCPRuntimeStdioSandboxFunc`.
- Added `ADKMCPRuntimeStdioSandboxCall` and `ADKMCPRuntimeStdioConfig`.
- Parsed bounded stdio config fields: `command`, `args`, `env`, and
  `cwd` / `working_dir` / `workingDir`.
- Required policy and sandbox before execution can proceed.
- Normalized config, policy, and sandbox failures to fixed sanitized errors.
- Added focused tests for missing policy, missing sandbox, config parsing and
  delegation, invalid config rejection, and policy/sandbox error redaction.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioTransport' -count=1`

## Remaining

Concrete stdio sandbox implementation, command allow-listing, isolated
working directory setup, environment and secret projection, process/session
limits, Eino MCP adapter invocation, session cache, audit persistence, health
classification, output offload, production bootstrap wiring, frontend policy
controls, and browser E2E remain open M6 or production acceptance work.
