# MCP Health And Registry Boundary Design

## Goal

M6.3 adds safe MCP health metadata and a metadata-only MCP Tool Registry index
boundary. This gives Workbench and future runtime policy code a stable list of
model-safe MCP tool names without exposing MCP transport config, auth, schemas,
tool arguments, results, or provider payloads.

## Health Metadata

`MCPToolServer` now carries:

- `health_status`: `unknown`, `healthy`, or a future bounded status.
- `health_checked_at`: Unix milliseconds.
- `health_latency_ms`: bounded latency metadata.
- `health_error`: sanitized bounded error text.

New servers default to `unknown`. Existing updates preserve the current health
snapshot. A successful Workbench `TestCall` updates the server health to
`healthy` with checked time and latency. The health snapshot must never include
test arguments, tool output, config, auth, URLs, object keys, provider raw
responses, stack traces, prompts, model text, or checkpoint bytes.

## Registry Boundary

`GET /api/workbench/mcp_tools/registry_entries?space_id=...` returns safe MCP
tool index rows for enabled, non-deleted MCP servers only. Each row includes:

- stable registry/tool grant name;
- source/category/visibility metadata;
- server ID and server name;
- raw MCP tool name as configured on the server;
- description;
- enabled flag;
- health metadata.

The registry name uses the same naming adapter as Skill candidate grants:
`mcp_{server_id}_{sanitized_tool_name}`. Future runtime tool adapters must use
the same name so UI grants, model-visible names, executable names, audit
records, and policy checks cannot drift.

The registry response must not include:

- MCP `config`;
- MCP `auth`;
- tool `input_schema`;
- endpoint URLs, object keys, or filenames;
- OAuth tokens or secret material;
- tool arguments/results;
- health-check raw payloads;
- provider request/response bodies;
- prompt, model, checkpoint, or transcript content.

## Non-Goals

- No real MCP network health probe in this slice.
- No runtime Eino MCP adapter invocation.
- No unified Tool Registry table yet.
- No provider-native schema exposure.
- No OAuth/session lifecycle or stdio sandboxing.
- No authorization/audit expansion.

## Future Work

M6 should next connect the registry boundary to the unified Coze Tool Registry,
add durable health jobs and health history, enforce tenant/agent policy at
registry read time, and wrap real MCP transports through Eino MCP tools behind
Coze-owned session, sandbox, audit, timeout, and output-budget layers.
