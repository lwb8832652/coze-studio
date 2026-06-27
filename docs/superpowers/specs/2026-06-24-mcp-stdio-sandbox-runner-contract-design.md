# MCP Stdio Sandbox Runner Contract Design

## Goal

M6.10 adds a stdio sandbox runner contract and projection adapter. It still
does not execute local commands. The adapter implements
`ADKMCPRuntimeStdioSandbox`, validates the sandbox call defensively, projects it
into a runner execution request, and delegates only to an explicitly injected
runner.

## Contract Boundary

`ADKMCPRuntimeStdioSandboxRunner` is the next execution boundary:

- `RunADKMCPRuntimeStdio(ctx, execution)` receives a fully projected
  `ADKMCPRuntimeStdioSandboxExecution`.
- The runner is the only future component allowed to own real process/session
  execution.
- Missing runner fails closed with `mcp runtime stdio runner is not configured`.
- Runner errors are normalized to `mcp runtime stdio runner failed`.

`ADKMCPRuntimeStdioSandboxAdapter` implements
`ADKMCPRuntimeStdioSandbox`. It does not create directories, execute commands,
project secrets, invoke Eino MCP adapters, write audit records, or update
health.

## Defensive Validation

The adapter rejects invalid calls before runner delegation:

- run, run ID, thread ID, and space ID must be present;
- durable server ID must be positive;
- runtime tool name must be Eino-safe;
- raw MCP tool name must be non-empty;
- arguments must be a valid JSON object;
- command must be non-empty;
- working directory must be an absolute path.

All invalid-call cases return `mcp runtime stdio sandbox call is invalid`
without exposing the invalid value.

## Projection

`ADKMCPRuntimeStdioSandboxExecution` carries:

- active run summary;
- safe runtime tool name;
- durable server ID;
- raw configured MCP tool name;
- arguments JSON;
- command;
- args;
- env;
- cleaned working directory.

The adapter clones args and env before handing them to the runner so later
call-site mutations cannot affect the execution request.

## Errors And Redaction

Adapter errors must not include model arguments, command names, command args,
env keys/values, working directories, server names, raw config/auth, URLs,
object keys, provider diagnostics, prompt/model text, transcripts, checkpoint
bytes, or secret-adjacent data.

`ADKMCPRuntimeStdioTransport` still wraps sandbox errors as
`mcp runtime stdio transport failed` before they reach the outer MCP runtime
executor.

## Non-Goals

- No host command execution.
- No process runner implementation.
- No Eino MCP stdio adapter invocation.
- No directory creation, mount setup, or cleanup.
- No secret lookup or env value projection.
- No audit persistence, health update, or output offload.
- No production bootstrap wiring.

## Future Work

Follow-up slices should implement a real runner behind this contract with
isolated working directories, approved env projection, process/session limits,
Eino MCP stdio adapter invocation, audit records, health classification, and
output offload.
