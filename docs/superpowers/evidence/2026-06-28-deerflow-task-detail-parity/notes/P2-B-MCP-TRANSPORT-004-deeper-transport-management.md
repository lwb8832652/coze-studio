# P2-B-MCP-TRANSPORT-004 deeper MCP transport management

## Scope

Close the verified backend gap between DeerFlow and Coze for non-stdio MCP
servers. This is production hardening only; it does not add new visible
selection UI.

## DeerFlow reference checked

DeerFlow source was checked before implementation:

- `backend/packages/harness/deerflow/mcp/client.py`
- `backend/packages/harness/deerflow/mcp/session_pool.py`
- `backend/packages/harness/deerflow/mcp/tools.py`
- `backend/app/gateway/routers/mcp.py`

Observed behavior:

- MCP config supports `stdio`, `sse`, and `http`.
- Enabled MCP servers are loaded through `MultiServerMCPClient`.
- `stdio` sessions are pooled by server/thread context.
- HTTP/SSE servers are valid MCP transports and do not require the stdio
  workdir lease path.
- Config APIs mask secrets and keep command/transport policy server-side.

## Coze implementation

Coze already had strict stdio lifecycle controls and a transport router that
recognized `stdio`, `sse`, and `streamable_http`, but bootstrap only injected
the stdio handler. This slice adds:

- `ADKMCPRuntimeRemoteTransport`, a fail-closed remote transport adapter for
  `sse` and `streamable_http`.
- `ADKMCPRuntimeRemoteEinoRunner`, which follows the stdio Eino runner shape:
  create MCP client, initialize it, fetch the requested Eino MCP tool, invoke
  only the target tool, and enforce output byte limits.
- mcp-go based remote client factory using:
  - `transport.NewSSE`
  - `transport.NewStreamableHTTP`
- Remote bootstrap flags:
  - `AGENT_THREAD_MCP_REMOTE_EINO_ENABLED`
  - `AGENT_THREAD_MCP_REMOTE_ALLOWED_HOSTS`
  - `AGENT_THREAD_MCP_REMOTE_ALLOW_HTTP`
  - `AGENT_THREAD_MCP_REMOTE_MAX_CONFIG_BYTES`
  - `AGENT_THREAD_MCP_REMOTE_MAX_HEADERS`
  - `AGENT_THREAD_MCP_REMOTE_MAX_HEADER_BYTES`
- `NewADKMCPRuntimeToolExecutorFromConfig` now permits remote-only MCP runtime
  without stdio lease/idgen dependencies, while still requiring a server
  resolver and explicit remote policy.

## Safety boundary

Remote MCP is disabled by default. When enabled, the transport still rejects:

- unsupported schemes and unsupported MCP transport types
- userinfo/fragments in URLs
- non-allowlisted hosts
- insecure HTTP except localhost, unless explicitly enabled
- oversized config/header payloads
- unsafe header names or CR/LF header values
- invalid tool call JSON arguments

Errors returned to the caller stay sanitized and do not include remote URLs,
headers, auth values, tool arguments, provider raw payloads, or credentials.

## Verification

RED:

```bash
cd backend
go test ./application/agentthread -run 'TestADKMCPRuntimeRemoteTransport' -count=1
go test ./application/agentthread -run 'TestADKMCPRuntimeRemoteEinoRunner' -count=1
go test ./application/agentthread -run 'TestADKMCPRuntimeBootstrapConfigFromEnvParsesRemoteEino|TestADKMCPRuntimeBootstrapConfigFromEnvRejectsIncompleteRemote|TestNewADKMCPRuntimeToolExecutorFromConfigBuildsRemoteHTTP' -count=1
```

The tests failed first because the remote transport, remote Eino runner, and
bootstrap config did not exist yet.

GREEN:

```bash
cd backend
go test ./application/agentthread -run 'TestADKMCPRuntimeRemoteTransport' -count=1
go test ./application/agentthread -run 'TestADKMCPRuntimeRemoteEinoRunner' -count=1
go test ./application/agentthread -run 'TestADKMCPRuntimeBootstrapConfigFromEnvParsesRemoteEino|TestADKMCPRuntimeBootstrapConfigFromEnvRejectsIncompleteRemote|TestNewADKMCPRuntimeToolExecutorFromConfigBuildsRemoteHTTP' -count=1
```

All targeted tests passed after the transport, runner, and bootstrap wiring
were implemented.

Regression:

```bash
cd backend
go test ./application/agentthread -run 'TestADKMCPRuntimeRemote|TestADKMCPRuntimeBootstrapConfigFromEnv|TestNewADKMCPRuntimeToolExecutorFromConfig|TestADKMCPRuntimeTransportRouter|TestADKMCPRuntimeStdio|TestADKMCPRuntimeExecutor|TestADKMCPRuntimeToolCatalog' -count=1
```

The combined MCP runtime regression passed.
