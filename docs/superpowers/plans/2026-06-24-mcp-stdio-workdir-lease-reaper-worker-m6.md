# MCP Stdio Workdir Lease Reaper Worker M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Start stale stdio workdir lease cleanup from production bootstrap
only when explicitly enabled by environment variables.

**Architecture:** Add a small scheduled worker around
`ADKMCPRuntimeStdioWorkdirLeaseReaper`, then wire `application.Init` to call
the worker startup function with the durable workdir lease repository.

**Tech Stack:** Go, Coze-owned worker pattern, durable
`MCPRuntimeWorkdirLeaseRepository`, stdio workdir lease reaper.

---

## Completed

- [x] Added `ADKMCPRuntimeStdioWorkdirLeaseCleaner`.
- [x] Added `ADKMCPRuntimeStdioWorkdirLeaseReaperWorker`.
- [x] Added `RunOnce` for deterministic tests and future maintenance hooks.
- [x] Added env-controlled startup with
  `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ENABLED`,
  `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ROOT`,
  `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_BATCH_SIZE`, and
  `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_INTERVAL_MS`.
- [x] Kept default-off behavior.
- [x] Added startup status for disabled/misconfigured states.
- [x] Wired `application.Init` to reuse the durable stdio workdir lease
  repository and start the worker only when env-enabled.
- [x] Added tests for `RunOnce`, sanitized worker error handling, disabled
  env, missing repository, invalid root, and configured worker construction.

## Verification

- `go test ./application/agentthread ./application -run 'TestADKMCPRuntimeStdioWorkdirLeaseReaperWorker' -count=1`

## Remaining

Metrics/exporter integration, health classification, operator-facing
diagnostics, real Eino MCP stdio adapter invocation, output offload, frontend
policy controls, and browser E2E remain open M6 or production acceptance work.
