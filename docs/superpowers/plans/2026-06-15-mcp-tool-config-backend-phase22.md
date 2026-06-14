# MCP Tool Config Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the first backend contract for workbench MCP tool configuration so the task tool menu can later manage MCP servers and test calls.

**Architecture:** Introduce a small workbench MCP application service with a replaceable in-memory catalog for this phase, exposed through `/api/workbench/mcp_tools` APIs. The service stores server metadata and tool schemas, validates JSON configuration, and provides a deterministic test-call response that proves the API contract without starting a real MCP client process yet.

**Tech Stack:** Go, Hertz handlers, workbench API model package, existing router registration tests, Go unit tests.

---

## Scope

- Add workbench MCP tool API models for server configs, tool schemas, list/upsert/get/test-call responses.
- Add an application-layer MCP tool catalog service with in-memory repository semantics.
- Support:
  - `GET /api/workbench/mcp_tools`
  - `POST /api/workbench/mcp_tools`
  - `GET /api/workbench/mcp_tools/:server_id`
  - `POST /api/workbench/mcp_tools/:server_id/test_call`
- Validate required fields and JSON strings for `config`, `auth`, `input_schema`, and test-call `arguments`.
- Return deterministic test-call output that includes server/tool identity and arguments.
- Register the new routes under the existing workbench group.

## Non-Goals

- No real MCP transport or subprocess lifecycle.
- No secrets encryption yet.
- No database table/migration yet.
- No frontend MCP tool page.
- No agent harness automatic tool loading from these configs.
- No LangGraph compatibility API.
- No security scanner.

## API Contract

- `MCPToolServer` fields: `server_id`, `space_id`, `name`, `description`, `server_type`, `enabled`, `config`, `auth`, `tools`, `created_at`, `updated_at`.
- `MCPToolDefinition` fields: `name`, `description`, `input_schema`.
- Upsert accepts optional `server_id`; if absent, the service generates a new ID.
- Test-call accepts `tool_name` and `arguments` JSON, then returns `status`, `output`, and `latency_ms`.

## Testing

- Application tests cover create/list/get/test-call and invalid JSON validation.
- Handler tests cover upsert/list/get/test-call response payloads.
- Router tests verify all four routes are registered.
- Existing agentthread/tool registry tests remain unchanged.
