# MCP Runtime Audit Persistence M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist content-free MCP runtime lifecycle audit records before real
MCP execution is enabled.

**Architecture:** Add a durable audit table and repository, an application
recorder that owns ID generation, and an optional executor recorder hook wired
through production bootstrap.

**Tech Stack:** Go, GORM, Atlas migrations, Coze-owned ADK MCP runtime
executor.

---

## Completed

- [x] Added `agent_mcp_runtime_audit_events` migration and latest schema.
- [x] Added `MCPRuntimeAuditEvent` entity.
- [x] Added `MCPRuntimeAuditRepository`.
- [x] Added `ApplicationADKMCPRuntimeAuditRecorder`.
- [x] Added `ADKMCPRuntimeAuditRecorder` and `ADKMCPRuntimeAuditRecord`.
- [x] Added `WithADKMCPRuntimeExecutorAuditRecorder`.
- [x] Recorded `mcp.tool.started`, `mcp.tool.completed`, and
  `mcp.tool.failed` metadata from `ADKMCPRuntimeExecutor`.
- [x] Wired production bootstrap to create a durable audit recorder and pass it
  through `NewADKMCPRuntimeToolExecutorFromConfig`.
- [x] Added tests for repository persistence/listing, recorder ID mapping and
  sanitized errors, executor success/failure lifecycle audit, and bootstrap
  audit injection.

## Verification

- `go test ./domain/agentthread/repository ./application/agentthread ./application -run 'TestMCPRuntimeAudit|TestApplicationADKMCPRuntimeAudit|TestADKMCPRuntimeExecutorRecords.*Audit|TestNewADKMCPRuntimeToolExecutorFromConfigInvokesDryRunStdio' -count=1`
- `docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations`

## Remaining

Admin-only audit browsing, retention policy, metrics/exporter integration,
OpenTelemetry span linkage, fail-closed audit policy for real MCP execution,
health classification, real Eino MCP stdio adapter invocation, output offload,
frontend policy controls, and browser E2E remain open M6 or production
acceptance work.
