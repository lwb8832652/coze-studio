# MCP Stdio Leased Workdir Preparer M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a lease-aware stdio workdir preparer wrapper without enabling
real MCP process execution.

**Architecture:** `ADKMCPRuntimeStdioLeasedWorkdirPreparer` implements the
existing `ADKMCPRuntimeStdioWorkdirPreparer` interface by composing an inner
preparer and a lease-store interface. It creates a lease after prepare, stores
lease identity in `ADKMCPRuntimeStdioPreparedWorkdir`, and finishes the lease
after cleanup.

**Tech Stack:** Go, Coze-owned MCP stdio sandbox boundary, application-level
adapter interfaces.

---

## Completed

- [x] Added `ADKMCPRuntimeStdioWorkdirLease` and status constants.
- [x] Added `ADKMCPRuntimeStdioWorkdirLeaseStore`.
- [x] Added lease ID and worker ID fields to
  `ADKMCPRuntimeStdioPreparedWorkdir`.
- [x] Added `ADKMCPRuntimeStdioLeasedWorkdirPreparer`.
- [x] Added tests for lease creation after prepare.
- [x] Added tests for release after successful cleanup.
- [x] Added tests for failed lease marking after cleanup failure.
- [x] Kept errors fixed and sanitized.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioLeasedWorkdirPreparer' -count=1`

## Remaining

Concrete MySQL lease-store adapter, production bootstrap wiring, stale lease
cleanup worker, audit persistence, secret projection, process/session limits,
Eino MCP stdio adapter invocation, health classification, output offload,
frontend policy controls, and browser E2E remain open M6 or production
acceptance work.
