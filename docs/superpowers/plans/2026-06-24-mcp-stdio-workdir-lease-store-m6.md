# MCP Stdio Workdir Lease Store M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect stdio workdir lease wrapper code to the durable lease
repository through an internal application adapter.

**Architecture:** `ApplicationADKMCPRuntimeStdioWorkdirLeaseStore` implements
`ADKMCPRuntimeStdioWorkdirLeaseStore`. It maps prepared workdir executions to
domain `MCPRuntimeWorkdirLease` rows, uses `idgen.IDGenerator` for lease IDs,
applies worker identity and TTL, and maps release/failure states back to the
repository finish contract.

**Tech Stack:** Go, Coze-owned MCP stdio runtime boundaries, domain
repository adapter, idgen.

---

## Completed

- [x] Added `ApplicationADKMCPRuntimeStdioWorkdirLeaseStoreOptions`.
- [x] Added `ApplicationADKMCPRuntimeStdioWorkdirLeaseStore`.
- [x] Added create-time validation and lease ID generation.
- [x] Added active domain lease mapping with TTL and worker ID.
- [x] Added release/failed status mapping for finish.
- [x] Required repository compare-and-set success for finish.
- [x] Sanitized repository/idgen errors.
- [x] Added tests for create mapping, finish mapping, and error redaction.

## Verification

- `go test ./application/agentthread -run 'TestApplicationADKMCPRuntimeStdioWorkdirLeaseStore' -count=1`

## Remaining

Production dry-run bootstrap wiring, stale lease cleanup worker, audit
persistence, secret projection, process/session limits, Eino MCP stdio adapter
invocation, health classification, output offload, frontend policy controls,
and browser E2E remain open M6 or production acceptance work.
