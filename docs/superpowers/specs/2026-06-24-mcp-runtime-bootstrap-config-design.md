# MCP Runtime Bootstrap Config Design

## Goal

M6.18 wires the safe MCP stdio dry-run transport into production bootstrap
behind explicit environment configuration. The default remains disabled and
MCP runtime invocation continues to fail closed when no executor is installed.

## Environment Contract

The parent switch is `AGENT_THREAD_MCP_RUNTIME_ENABLED`. The dry-run stdio
handler is mounted only when both this parent switch and
`AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED` are `true`.

Required dry-run stdio settings when enabled:

- `AGENT_THREAD_MCP_STDIO_WORKDIR_ROOT`: absolute Coze-owned workdir root.
- `AGENT_THREAD_MCP_STDIO_WORKER_ID`: lease owner identity.
- `AGENT_THREAD_MCP_STDIO_ALLOWED_COMMANDS`: comma-separated exact command
  allow-list.

Optional bounded settings:

- `AGENT_THREAD_MCP_STDIO_ALLOWED_ENV_KEYS`
- `AGENT_THREAD_MCP_STDIO_MAX_ARGS`
- `AGENT_THREAD_MCP_STDIO_MAX_ARG_BYTES`
- `AGENT_THREAD_MCP_STDIO_MAX_ENV_VARS`
- `AGENT_THREAD_MCP_STDIO_MAX_ENV_VALUE_BYTES`
- `AGENT_THREAD_MCP_STDIO_LEASE_TTL_MS`
- `AGENT_THREAD_MCP_STDIO_MAX_CONFIG_BYTES`
- `AGENT_THREAD_MCP_STDIO_DRY_RUN_OUTPUT_BYTES`
- `AGENT_THREAD_MCP_RUNTIME_TIMEOUT_MS`
- `AGENT_THREAD_MCP_RUNTIME_MAX_OUTPUT_BYTES`

Invalid booleans, invalid integers, relative workdir roots, missing worker ID,
or missing command allow-list fail server initialization with sanitized errors
that name only the environment variable.

## Bootstrap Wiring

`ADKMCPRuntimeBootstrapConfigFromEnv` parses and validates the env contract.
`NewADKMCPRuntimeToolExecutorFromConfig` returns nil unless the config is
explicitly enabled and the resolver, durable lease repository, and ID generator
are present.

When enabled, `application.Init` injects the resulting executor through
`WithDefaultADKToolProviderMCPExecutor`. The executor uses:

- `primaryServices.mcpToolSVC` as the MCP runtime server resolver;
- `MCPRuntimeWorkdirLeaseRepository` for durable stdio workdir leases;
- `infra.IDGenSVC` for lease IDs;
- `ApplicationRunEventSink` for content-free MCP tool lifecycle events;
- `NewADKMCPRuntimeStdioDryRunTransport` for the router's stdio slot.

The transport is still the dry-run runner. It does not execute commands or
open MCP sessions.

## Safety Rules

- Default-off behavior is part of the contract.
- Environment config must not include secrets.
- Workdir lease rows remain internal execution state.
- Runtime events remain content-free.
- Dry-run output may include only safe metadata from
  `coze.mcp_stdio_dry_run.v1`.
- Errors must not include tool arguments, command values, env values, workdir
  paths, server names, config/auth, URLs, filenames, object keys, provider
  diagnostics, prompt/model text, transcripts, checkpoints, or secrets.

## Non-Goals

- No real stdio process execution.
- No Eino MCP stdio adapter invocation.
- No SSE or streamable HTTP transport implementation.
- No secret projection or OAuth/session lifecycle.
- No stale lease cleanup worker.
- No public API or frontend controls for these env settings.

## Future Work

M6.19 should add stale active-lease recovery and cleanup. Later slices can add
audit persistence, secret projection, process/session limits, and then a real
Eino MCP stdio adapter behind the same policy, lease, timeout, audit, and
redaction boundaries.
