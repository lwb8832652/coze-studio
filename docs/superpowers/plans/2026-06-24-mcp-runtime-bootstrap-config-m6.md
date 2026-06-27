# MCP Runtime Bootstrap Config M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the safe MCP stdio dry-run transport into production bootstrap
behind explicit env configuration while preserving default-off behavior.

**Architecture:** Parse env into `ADKMCPRuntimeBootstrapConfig`, build a
dry-run-only `ADKMCPRuntimeToolExecutor` when explicitly enabled, and inject it
through the existing default ADK tool provider MCP executor option.

**Tech Stack:** Go, Coze-owned ADK MCP runtime executor, durable stdio workdir
lease repository, stdio dry-run transport.

---

## Completed

- [x] Added MCP runtime bootstrap env constants and
  `ADKMCPRuntimeBootstrapConfigFromEnv`.
- [x] Added validation for default-off behavior, explicit dry-run stdio
  enablement, absolute workdir root, worker ID, command allow-list, and
  positive timeout/output/policy budgets.
- [x] Added `NewADKMCPRuntimeToolExecutorFromConfig` to compose the dry-run
  stdio transport, router, executor, resolver, durable lease repository, ID
  generator, and event sink.
- [x] Wired `application.Init` to inject the optional MCP runtime executor via
  `WithDefaultADKToolProviderMCPExecutor`.
- [x] Kept real stdio execution disabled; the mounted stdio handler still uses
  `ADKMCPRuntimeStdioDryRunRunner`.
- [x] Added tests for env parsing, incomplete config rejection, default-off
  nil executor, and dry-run executor invocation through the durable lease path.

## Verification

- `go test ./application/agentthread ./application -run 'TestADKMCPRuntimeBootstrapConfigFromEnv|TestNewADKMCPRuntimeToolExecutorFromConfig|TestDefaultADKToolProviderCanWireMCPExecutor' -count=1`

## Remaining

Stale active-lease cleanup worker, audit persistence, secret projection,
process/session limits, Eino MCP stdio adapter invocation, SSE/streamable HTTP
transports, health classification, output offload, frontend policy controls,
and browser E2E remain open M6 or production acceptance work.
