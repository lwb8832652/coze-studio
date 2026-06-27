# MCP Stdio Workdir Manager M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add deterministic isolated workdir projection for stdio MCP runtime
calls without touching the filesystem.

**Architecture:** `ADKMCPRuntimeStdioWorkdirManager` validates run/server/tool
identity and projects a canonical path under a configured absolute root.
`ADKMCPRuntimeStdioTransport` can optionally use the manager to overwrite
server-provided `cwd` before policy validation and sandbox delegation.

**Tech Stack:** Go, Coze-owned ADK MCP runtime executor, MCP stdio transport
boundary.

---

## Result

M6.11 provides a deterministic workdir projection boundary that later sandbox
implementations can prepare and execute inside.

## Completed

- Added `ADKMCPRuntimeStdioWorkdirProjector` and function adapter.
- Added `ADKMCPRuntimeStdioWorkdirManagerOptions`.
- Added `ADKMCPRuntimeStdioWorkdirManager`.
- Added `ADKMCPRuntimeStdioWorkdirRequest` and
  `ADKMCPRuntimeStdioWorkdirProjection`.
- Generated deterministic paths under:
  `{root}/spaces/{space_id}/threads/{thread_id}/runs/{run_id}/servers/{server_id}/tools/{safe_runtime_tool_name}`.
- Rejected missing/relative roots and invalid run/server/tool identity with a
  fixed sanitized error.
- Added optional `WorkdirManager` to `ADKMCPRuntimeStdioTransportOptions`.
- Ensured configured manager overwrites untrusted server `cwd` before static
  policy validation.
- Added focused tests for deterministic projection, invalid input rejection,
  and transport-before-policy workdir projection.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioWorkdirManager|TestADKMCPRuntimeStdioTransportProjectsWorkdir' -count=1`

## Remaining

Filesystem-backed workdir creation, permissions, cleanup, leases, audit,
secret projection, process/session limits, Eino MCP stdio adapter invocation,
health classification, output offload, production bootstrap wiring, frontend
policy controls, and browser E2E remain open M6 or production acceptance work.
