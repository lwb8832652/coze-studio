# MCP Stdio Eino Runner Design

M6.23 adds the first real stdio MCP execution runner behind the existing Coze
runtime boundaries. It uses Eino's MCP tool adapter, but only after Coze-owned
policy, workdir projection, leased workdir preparation, timeout, audit, and
health hooks have already been applied by the surrounding runtime chain.

## Scope

This slice introduces:

- `ADKMCPRuntimeStdioEinoRunner`, an `ADKMCPRuntimeStdioSandboxRunner`
  implementation.
- A small client factory boundary for creating and initializing an MCP stdio
  client.
- A tool provider boundary that calls
  `github.com/cloudwego/eino-ext/components/tool/mcp.GetTools`.
- A reusable stdio runtime transport builder shared by dry-run and Eino
  runner modes.
- `AGENT_THREAD_MCP_STDIO_EINO_ENABLED` as an explicit real-execution opt-in.

It does not add session pooling, OAuth, encrypted secret projection,
SSE/streamable HTTP transport, frontend policy controls, output offload, or
operator diagnostics.

## Execution Flow

The stdio runtime chain remains:

1. `ADKMCPRuntimeExecutor` validates run, tenant, server, enabled state, tool
   membership, timeout, output budget, events, audit, and health.
2. `ADKMCPRuntimeTransportRouter` selects the stdio handler.
3. `ADKMCPRuntimeStdioTransport` parses bounded stdio config.
4. `ADKMCPRuntimeStdioWorkdirManager` projects a run-scoped working directory.
5. `ADKMCPRuntimeStdioStaticPolicy` checks exact command allow-list, env
   allow-list, argument budget, env budget, and workdir prefix.
6. `ADKMCPRuntimeStdioSandboxAdapter` prepares a leased workdir.
7. `ADKMCPRuntimeStdioEinoRunner` creates an MCP client, initializes it, asks
   Eino MCP adapter for the selected tool, invokes that tool, bounds output,
   and closes the client.

## Bootstrap

The production bootstrap is still default-off. A real stdio runner is installed
only when:

- `AGENT_THREAD_MCP_RUNTIME_ENABLED=true`;
- `AGENT_THREAD_MCP_STDIO_EINO_ENABLED=true`;
- `AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED` is false;
- workdir root, worker ID, and allowed commands are valid.

Dry-run mode and Eino mode are mutually exclusive. If both env flags are true,
bootstrap validation fails closed.

## Security

The Eino runner returns fixed sanitized errors only:

- invalid execution;
- missing client factory;
- missing tool provider;
- client failure;
- tool discovery failure;
- tool not found;
- tool not invokable;
- tool call failure;
- output budget exceeded.

Runner errors must not include command names, args, env values, workdir paths,
tool arguments/results, MCP config/auth, raw MCP tool names, URLs, object keys,
provider diagnostics, stack traces, prompts, model text, transcripts,
checkpoint bytes, or secrets.

The runner may pass the projected working directory to the MCP stdio subprocess
through a Coze-owned command factory. It must not bypass the existing static
policy or leased workdir preparation.

## Dependencies

The Eino MCP adapter module is:

- `github.com/cloudwego/eino-ext/components/tool/mcp v0.0.8`

It uses `github.com/mark3labs/mcp-go v0.43.0` underneath. The adapter expects
an initialized MCP client, converts MCP tool schemas into Eino `tool.BaseTool`
values, and invokes tools through `tool.InvokableTool`.
