# MCP Tool Server Persistence M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist Workbench MCP tool server configuration in MySQL instead of
using the in-memory catalog in the application bootstrap path.

**Architecture:** Keep the existing `mcptool.Catalog` interface. Add
`MySQLCatalog` as the production implementation and keep `InMemoryCatalog` for
tests and local harnesses. Store the current API server shape in a durable
`mcp_tool_servers` table with JSON columns for config/auth/tools.

**Tech Stack:** Go, GORM, SQLite unit tests, Atlas migrations.

---

## Result

M6.1 adds durable MCP server storage and changes application initialization to
use `mcptool.NewMySQLCatalog(DB)`.

## Completed

- Added `mcpToolServerPO` and `MySQLCatalog`.
- Implemented `Upsert`, `Get`, and `List` through GORM.
- Preserved `ErrNotFound` behavior for missing rows.
- Persisted `config`, `auth`, and `tools` as JSON.
- Added SQLite-backed unit tests for round-trip persistence, ordering, and
  not-found classification.
- Switched application bootstrap from `NewInMemoryCatalog()` to
  `NewMySQLCatalog(basicServices.infra.DB)`.
- Added Atlas migration `20260623000100_mcp_tool_servers.sql`.
- Updated `opencoze_latest_schema.hcl` and regenerated `atlas.sum`.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./application/mcptool -count=1`
- `go test ./application/mcptool ./application/skill ./application -count=1`
- `atlas migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

Secret encryption and masking, MCP delete/restore lifecycle, normalized tool
registry indexing, health status, stdio sandboxing, OAuth/session lifecycle,
runtime Eino MCP adapter invocation, authorization policy, audit records, and
browser E2E remain separate M6 or production acceptance work.
