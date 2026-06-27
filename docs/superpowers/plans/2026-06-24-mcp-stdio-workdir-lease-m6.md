# MCP Stdio Workdir Lease M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add durable stdio MCP workdir lease persistence without wiring it
into runtime execution yet.

**Architecture:** A new domain entity and repository contract persist
`agent_mcp_stdio_workdir_leases` rows. The MySQL/GORM implementation reuses
the existing `threadRepository` style and sqlite-backed repository tests.

**Tech Stack:** Go, GORM, SQLite unit tests, Atlas migrations, Coze-owned MCP
runtime boundaries.

---

## Completed

- [x] Add `MCPRuntimeWorkdirLease` entity and status constants.
- [x] Add `MCPRuntimeWorkdirLeaseRepository` interface and request DTOs.
- [x] Add sqlite-backed repository tests for create/get, finish, worker
  compare-and-set, and expired active lease listing.
- [x] Implement GORM PO mapping and repository methods.
- [x] Add Atlas migration and latest schema table.
- [x] Regenerate Atlas migration checksums and validate migrations with the
  pinned Atlas Community v0.35.0 container image.
- [x] Update AGENTS and the master roadmap with M6.14 context.

## Verification

- `go test ./domain/agentthread/repository -run 'TestMCPRuntimeWorkdirLeaseRepository' -count=1`
- `go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./application/mcptool ./application -count=1`
- `docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

Lease-aware stdio workdir preparer integration, stale lease cleanup worker,
audit persistence, secret projection, process/session limits, Eino MCP stdio
adapter invocation, health classification, output offload, production
bootstrap wiring, frontend policy controls, and browser E2E remain open M6 or
production acceptance work.
