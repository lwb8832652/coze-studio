# MCP Runtime Executor Contract Design

## Goal

M6.5 adds the first executable contract behind the MCP runtime catalog without
starting real MCP stdio, SSE, or HTTP transport. The purpose is to split ADK
tool wrapping from MCP execution so future transport/session code can be added
behind a Coze-owned boundary instead of inside the model-visible tool adapter.

## Contract

`ADKMCPRuntimeToolExecutor` receives an `ADKMCPRuntimeToolCall`:

- active `RunSummary`;
- safe model-visible tool name;
- durable MCP server ID;
- raw configured MCP tool name;
- model arguments JSON.

The runtime catalog builds this call from the metadata-only registry row.
Registry entries without a positive server ID, a raw tool name, an Eino-safe
registry name, or a non-empty description are skipped. The executor is internal
runtime plumbing. It may see tool arguments because it must eventually call the
MCP server, but it must not place arguments, config, auth, URLs, object keys,
provider raw payloads, prompt/model text, transcripts, or checkpoint bytes in
events or errors.

## Default Behavior

No executor is installed by default. Without an executor, invocation fails
closed with:

`mcp runtime tool execution is not enabled: <safe_tool_name>`

This keeps M6.4 behavior intact for production while allowing tests and future
transport adapters to use the same ADK tool path.

## Error Boundary

If an executor returns an error, the ADK invoker returns only:

`mcp runtime tool execution failed: <safe_tool_name>`

The raw executor error is intentionally not propagated to the model-facing
error because it may contain MCP server names, arguments, URLs, command paths,
provider diagnostics, or secret-adjacent data. Future audit records may store
classified bounded metadata through a separate Coze-owned audit path, not this
tool error string.

## Wiring

`WithADKMCPRuntimeToolExecutor` installs an executor on
`ADKMCPRuntimeToolCatalog`. `WithDefaultADKToolProviderMCPExecutor` forwards it
through default ADK provider construction, alongside
`WithDefaultADKToolProviderMCPRegistry`.

The production bootstrap currently injects the MCP registry service only. A
real executor should be injected only after transport/session, OAuth/secret,
stdio sandbox, authorization, audit, timeout, output-budget, and health gates
are implemented.

## Non-Goals

- No real MCP transport execution.
- No Eino MCP adapter schema conversion yet.
- No input schema exposure to the model.
- No health probe or health-history update.
- No frontend policy UI.

## Future Work

The next slice should introduce a Coze-owned MCP transport executor that can
resolve a server row by ID, enforce tenant and runtime policy, invoke MCP via
Eino's MCP adapter or a session wrapper, bound outputs, classify failures, and
emit content-free audit/health metadata.
