# MCP Stdio Sandbox Runner Contract M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a stdio sandbox runner contract without enabling real command
execution.

**Architecture:** `ADKMCPRuntimeStdioSandboxAdapter` implements the existing
`ADKMCPRuntimeStdioSandbox` interface, validates calls defensively, projects
them into `ADKMCPRuntimeStdioSandboxExecution`, and delegates to an injected
`ADKMCPRuntimeStdioSandboxRunner`. Missing runner and runner errors are fixed
sanitized failures.

**Tech Stack:** Go, Coze-owned ADK MCP runtime executor, MCP stdio sandbox
boundary.

---

## Result

M6.10 provides a concrete sandbox adapter and runner contract that future real
stdio execution can implement without changing transport or policy code.

## Completed

- Added `ADKMCPRuntimeStdioSandboxOptions`.
- Added `ADKMCPRuntimeStdioSandboxAdapter` implementing
  `ADKMCPRuntimeStdioSandbox`.
- Added `ADKMCPRuntimeStdioSandboxRunner` and function adapter.
- Added `ADKMCPRuntimeStdioSandboxExecution`.
- Added defensive validation for run identity, server ID, safe tool name, raw
  MCP tool name, JSON-object arguments, command, and absolute workdir.
- Added projection that clones args/env and cleans the working directory.
- Normalized missing runner, invalid call, and runner failures to fixed
  sanitized errors.
- Added focused tests for fail-closed missing runner, runner projection,
  invalid call rejection before runner, and runner error redaction.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioSandbox' -count=1`

## Remaining

Real process runner implementation, isolated workdir creation/cleanup, env
secret projection, process/session limits, Eino MCP stdio adapter invocation,
audit persistence, health classification, output offload, production bootstrap
wiring, frontend policy controls, and browser E2E remain open M6 or production
acceptance work.
