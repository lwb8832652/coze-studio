# MCP Runtime Transport Router Design

## Goal

M6.7 adds a Coze-owned MCP runtime transport router behind
`ADKMCPRuntimeExecutor`. It still does not implement real stdio, SSE, or
streamable HTTP MCP sessions. The router classifies the durable server
`server_type`, dispatches to an explicitly configured transport handler, and
fails closed when the selected transport is not installed.

## Router Boundary

`ADKMCPRuntimeTransportRouter` implements `ADKMCPRuntimeTransportInvoker`.
It accepts optional handlers through `ADKMCPRuntimeTransportRouterOptions`:

- `Stdio` for `server_type="stdio"`;
- `SSE` for `server_type="sse"`;
- `StreamableHTTP` for `server_type="streamable_http"`, plus `http` and
  `streamable-http` aliases.

The router passes the existing `ADKMCPRuntimeTransportCall` to the selected
handler. It does not inspect or transform tool arguments, does not parse MCP
server config/auth, does not create network clients, and does not spawn local
processes.

## Fail-Closed Behavior

If a transport handler is missing, the router returns a fixed sanitized error
for that transport type, such as `mcp runtime stdio transport is disabled`.
If the server type is missing or unsupported, it returns
`unsupported mcp runtime transport`.

Router errors must not include:

- MCP tool arguments;
- MCP server name;
- raw `config` or `auth`;
- endpoint URLs;
- command paths or arguments;
- filenames, object keys, provider diagnostics, prompt/model text,
  transcripts, checkpoints, or secret-adjacent data.

`ADKMCPRuntimeExecutor` remains responsible for lifecycle events, timeout,
output byte budget, server ownership, enabled state, and tool membership
validation. Router errors are still treated as transport failures by the
executor, preserving the existing content-free `mcp.tool.failed` event path.

## Production Wiring Rule

Production bootstrap must not install a concrete stdio/SSE/HTTP handler until
the corresponding transport passes its own authorization, OAuth/secret,
session lifecycle, stdio sandbox, network policy, audit, output budget, and
health gates. Installing only the router keeps runtime MCP invocation safely
disabled while allowing follow-up slices to land one transport at a time.

## Non-Goals

- No real MCP stdio process execution.
- No SSE or streamable HTTP client implementation.
- No Eino MCP adapter conversion.
- No OAuth or encrypted secret retrieval.
- No stdio sandbox or network allow-list implementation.
- No health probes/history persistence.
- No frontend transport policy controls.

## Future Work

Follow-up slices should add concrete handlers behind this router in transport
order: stdio sandbox/session manager, streamable HTTP/SSE session manager,
Eino MCP adapter conversion, per-transport policy and audit records, health
classification, and output offload for large tool results.
