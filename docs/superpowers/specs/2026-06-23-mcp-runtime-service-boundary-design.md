# MCP Runtime Service Boundary Design

## Goal

M6.6 adds a Coze-owned MCP runtime service boundary behind the ADK MCP executor
contract. It still does not start real MCP stdio, SSE, or HTTP transport.
Instead, it validates a runtime call, resolves the durable MCP server row,
applies tenant and tool checks, bounds transport execution, and emits
content-free lifecycle events.

## Runtime Service

`ADKMCPRuntimeExecutor` implements `ADKMCPRuntimeToolExecutor`. It depends on:

- `ADKMCPRuntimeServerResolver`: resolves a durable MCP server by ID.
- `ADKMCPRuntimeTransportInvoker`: future real transport/session invocation.

The executor validates before transport:

- run and run `space_id` are present;
- MCP server ID is positive;
- raw configured MCP tool name is present;
- arguments are a valid JSON object;
- resolved server belongs to the active run space;
- server is enabled;
- tool is configured on the server.

Calls failing these checks never reach transport.

## Resolver Boundary

`mcptool.ApplicationService.ResolveADKMCPRuntimeServer` exposes the raw server
row for internal runtime use. It intentionally returns raw `config`, `auth`,
and tool definitions because a future transport/session layer needs those
fields to connect to MCP. This method is not a Workbench response mapper and
must not be exposed directly through HTTP.

Workbench create/list/get/delete responses remain masked through
`cloneServerForResponse`.

## Timeout And Output Budget

The runtime executor wraps transport calls with a timeout. Default timeout is
30 seconds. It also rejects transport output larger than the configured byte
budget. Default output budget is 64 KiB.

Oversized output is not truncated in this slice because durable offload,
artifact registration, and output budgeting need a separate policy decision.
The model-facing error only says the output exceeded budget and includes the
safe tool name.

## Events And Errors

The executor may emit:

- `mcp.tool.started`;
- `mcp.tool.completed`;
- `mcp.tool.failed`.

Payloads contain only metadata such as schema, safe tool name, server ID,
elapsed time, error code, and output byte count. They must not include MCP
arguments, server names, config, auth, endpoint URLs, filenames, object keys,
tool output, provider diagnostics, prompt/model text, transcripts, or
checkpoint bytes.

Returned errors are classified and sanitized. Raw resolver or transport errors
are not propagated to model-facing tool errors.

## Non-Goals

- No real MCP transport/session implementation.
- No Eino MCP adapter conversion.
- No OAuth or encrypted secret retrieval.
- No stdio sandboxing.
- No audit table or health-history persistence.
- No frontend policy controls.

## Future Work

Next slices should implement the transport invoker behind this service:
server-type-specific stdio/SSE/HTTP session management, OAuth/secret access,
stdio sandbox policy, Eino MCP adapter conversion, output offload, audit
records, health updates, and provider-specific failure classification.
