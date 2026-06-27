# MCP Stdio Dry-Run Runner M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a non-executing stdio runner that verifies the full MCP runtime
stdio chain without starting host commands.

**Architecture:** `ADKMCPRuntimeStdioDryRunRunner` implements the existing
`ADKMCPRuntimeStdioSandboxRunner` interface. It validates the projected
execution and returns safe bounded JSON metadata, allowing tests to compose
transport, static policy, workdir projection, filesystem preparation, runner
delegation, and cleanup.

**Tech Stack:** Go, Coze-owned ADK MCP runtime executor, MCP stdio transport,
static stdio policy, sandbox runner contract.

---

## Result

M6.13 provides an end-to-end stdio runtime chain smoke path while keeping real
process execution disabled.

## Completed

- Added `ADKMCPRuntimeStdioDryRunRunnerOptions`.
- Added `ADKMCPRuntimeStdioDryRunRunner`.
- Added defensive dry-run execution validation.
- Added bounded `coze.mcp_stdio_dry_run.v1` JSON output.
- Returned only safe metadata: schema, status, runtime tool name, server ID,
  command/workdir presence, args count, and env count.
- Excluded command names/args, env keys/values, raw MCP tool names, model
  arguments, workdir/root paths, server names, raw config/auth, provider
  diagnostics, transcripts, and checkpoints from dry-run output.
- Added focused tests for direct safe metadata output, invalid execution
  rejection, and the full transport-policy-workdir-sandbox-cleanup chain.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioDryRunRunner' -count=1`

## Remaining

Durable workdir leases, stale-run recovery, audit persistence, secret
projection, process/session limits, Eino MCP stdio adapter invocation, health
classification, output offload, production bootstrap wiring, frontend policy
controls, and browser E2E remain open M6 or production acceptance work.
