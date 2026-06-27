# MCP Stdio Workdir Preparer M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add filesystem-backed working-directory prepare/cleanup for stdio MCP
runtime calls without command execution.

**Architecture:** `ADKMCPRuntimeStdioFilesystemWorkdirPreparer` implements a
new `ADKMCPRuntimeStdioWorkdirPreparer` contract. The sandbox adapter can
optionally prepare a validated projected workdir before runner delegation and
clean it afterward.

**Tech Stack:** Go, Coze-owned ADK MCP runtime executor, MCP stdio sandbox
boundary.

---

## Result

M6.12 provides the first filesystem-backed lifecycle boundary for projected
stdio workdirs while keeping process execution disabled.

## Completed

- Added `ADKMCPRuntimeStdioWorkdirPreparer`.
- Added `ADKMCPRuntimeStdioPreparedWorkdir`.
- Added `ADKMCPRuntimeStdioFilesystemWorkdirPreparer`.
- Added root containment validation for prepare and cleanup.
- Added guarded `MkdirAll`, `Chmod`, `Stat`, and `RemoveAll`.
- Rejected workdirs outside root and the root itself.
- Added optional `WorkdirPreparer` to `ADKMCPRuntimeStdioSandboxOptions`.
- Ensured sandbox adapter prepares before runner and attempts cleanup after
  runner success or failure.
- Added focused tests for directory creation/mode, cleanup, unsafe path
  rejection, prepare-run-cleanup ordering, and runner-failure cleanup.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioFilesystemWorkdirPreparer|TestADKMCPRuntimeStdioSandboxPrepares|TestADKMCPRuntimeStdioSandboxCleans' -count=1`

## Remaining

Durable leases, retention/recovery, audit persistence, secret projection,
process/session limits, Eino MCP stdio adapter invocation, health
classification, output offload, production bootstrap wiring, frontend policy
controls, and browser E2E remain open M6 or production acceptance work.
