# MCP Runtime Adapter Boundary M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect MCP registry entries to the ADK runtime tool catalog as a
safe, non-executable adapter boundary.

**Architecture:** Add an `ADKMCPRuntimeToolCatalog` that reads metadata-only
registry entries through an injected provider, converts safe entries to
deferred runtime tool definitions, and fails closed on invocation until real
Coze-owned MCP transport/session execution is implemented.

**Tech Stack:** Go, Eino ADK runtime tool catalog, Workbench MCP application
service.

---

## Result

M6.4 connects the MySQL-backed Workbench MCP registry to ADK runtime tool
catalog resolution. Runs can explicitly enable MCP metadata exposure with
`mcp_tools.enabled=true`; real MCP execution remains disabled.

## Completed

- Added `ADKMCPToolRegistry` and `ADKMCPRuntimeToolCatalog`.
- Added `ADKCompositeRuntimeToolCatalog` so web and MCP runtime catalogs share
  the existing `ADKRuntimeToolCatalogProvider`.
- Added `WithDefaultADKToolProviderMCPRegistry` and production bootstrap wiring
  through `primaryServices.mcpToolSVC`.
- Added `mcptool.ApplicationService.ListMCPToolRegistryEntries` as the safe
  registry-provider interface for runtime use.
- Kept MCP runtime tools disabled by default and deferred by default when
  enabled.
- Ensured generated runtime tool definitions omit input schema, config, auth,
  arguments, results, URLs, object keys, prompt/model text, transcripts, and
  checkpoint bytes.
- Added fail-closed unsupported invocation behavior until real MCP transport is
  wired.
- Added focused backend tests for disabled-by-default behavior, metadata-only
  deferred conversion, fail-closed invocation, default provider wiring,
  composite runtime catalogs, and direct MCP registry provider output.

## Verification

- `go test ./application/agentthread -run 'TestADKRuntimeToolCatalogProvider|TestADKCompositeRuntimeToolCatalog|TestADKMCPRuntimeToolCatalog|TestDefaultADKToolProviderCanWireMCPRegistry' -count=1`
- `go test ./application/mcptool -run 'TestApplicationServiceListsMCPToolRegistryEntries|TestApplicationServiceUpsertsListsGetsAndTestsMCPServer' -count=1`
- `go test ./application/mcptool -count=1`
- `go test ./application -run TestNonExistent -count=1`

## Remaining

Real MCP invocation, Eino MCP tool conversion with schemas, stdio/SSE/HTTP
transport, OAuth/session lifecycle, encrypted secret storage, authorization,
audit, timeout/output budgets, health probes, unified Tool Registry tables,
frontend policy controls, and browser E2E remain open M6 or production
acceptance work.
