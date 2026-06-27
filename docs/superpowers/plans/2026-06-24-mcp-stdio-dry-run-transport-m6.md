# MCP Stdio Dry-Run Transport M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an explicit builder for the full stdio MCP dry-run smoke
transport without enabling real process execution.

**Architecture:** `NewADKMCPRuntimeStdioDryRunTransport` composes the existing
workdir manager, static policy, filesystem preparer, durable lease-store
adapter, leased preparer, dry-run runner, sandbox, and stdio transport.

**Tech Stack:** Go, Coze-owned MCP runtime transport router, stdio sandbox
boundary, durable lease repository adapter.

---

## Completed

- [x] Added `ADKMCPRuntimeStdioDryRunTransportOptions`.
- [x] Added `NewADKMCPRuntimeStdioDryRunTransport`.
- [x] Composed workdir projection, static policy, filesystem prepare/cleanup,
  durable lease store, leased preparer, dry-run runner, sandbox, and stdio
  transport.
- [x] Added full-chain smoke test that creates and releases a durable lease,
  cleans the projected workdir, and returns safe dry-run metadata.
- [x] Added fail-closed test for missing lease store.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioDryRunTransport' -count=1`

## Remaining

Production runtime-policy/env wiring, stale lease cleanup worker, audit
persistence, secret projection, process/session limits, Eino MCP stdio adapter
invocation, health classification, output offload, frontend policy controls,
and browser E2E remain open M6 or production acceptance work.
