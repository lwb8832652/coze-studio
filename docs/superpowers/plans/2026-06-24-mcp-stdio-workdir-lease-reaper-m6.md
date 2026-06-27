# MCP Stdio Workdir Lease Reaper M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a safe one-shot cleanup boundary for expired active stdio MCP
workdir leases.

**Architecture:** `ADKMCPRuntimeStdioWorkdirLeaseReaper` lists expired active
leases from the durable repository, validates workdir paths against a
configured root, delegates cleanup to the filesystem workdir preparer, and
finishes cleaned leases through repository CAS using the original worker ID.

**Tech Stack:** Go, Coze-owned MCP stdio workdir preparer, durable
`MCPRuntimeWorkdirLeaseRepository`.

---

## Completed

- [x] Added `ADKMCPRuntimeStdioWorkdirLeaseReaperOptions`.
- [x] Added `ADKMCPRuntimeStdioWorkdirLeaseReaperResult`.
- [x] Added `ADKMCPRuntimeStdioWorkdirLeaseReaper`.
- [x] Implemented expired active lease listing with bounded batch size and
  fixed sanitized fatal error.
- [x] Implemented per-lease path validation that rejects nil rows, non-active
  rows, empty worker IDs, invalid IDs, relative workdirs, root workdir, and
  paths outside the configured root.
- [x] Implemented cleanup-before-finish behavior.
- [x] Implemented stale lease finish as `failed` with sanitized
  `stale lease expired`.
- [x] Added tests for successful cleanup/finish, unsafe path rejection,
  cleanup failure without finish, and repository list error sanitization.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioWorkdirLeaseReaper' -count=1`

## Remaining

Env-controlled scheduled reaper worker, production bootstrap wiring, audit
persistence, metrics, real Eino MCP stdio adapter invocation, health
classification, output offload, frontend policy controls, and browser E2E
remain open M6 or production acceptance work.
