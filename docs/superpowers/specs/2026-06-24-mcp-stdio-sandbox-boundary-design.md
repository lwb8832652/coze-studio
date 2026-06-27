# MCP Stdio Sandbox Boundary Design

## Goal

M6.8 adds the first stdio-specific MCP transport handler behind
`ADKMCPRuntimeTransportRouter`. It still does not execute local commands or
start real MCP sessions. Instead, it parses a bounded stdio server config,
requires an explicit policy decision, and delegates only to an injected
Coze-owned sandbox interface.

## Stdio Transport Boundary

`ADKMCPRuntimeStdioTransport` implements `ADKMCPRuntimeTransportInvoker` and is
intended to be mounted in the router `Stdio` slot.

It accepts `ADKMCPRuntimeStdioTransportOptions`:

- `Policy`: validates the parsed sandbox call before execution.
- `Sandbox`: the only interface allowed to perform future stdio process and
  MCP session work.
- `MaxConfigBytes`: optional bounded config size. The default is 16 KiB.

Both `Policy` and `Sandbox` are required for execution. Missing policy returns
`mcp runtime stdio policy is not configured`. Missing sandbox returns
`mcp runtime stdio sandbox is not configured`. This keeps production bootstrap
safe even if the transport is accidentally mounted without its security
dependencies.

## Config Parsing

The handler accepts only durable MCP servers with `server_type="stdio"`.
Server config must be a JSON object containing:

- `command`: non-empty string;
- `args`: optional array of strings;
- `env`: optional object with string values;
- `cwd`, `working_dir`, or `workingDir`: optional working directory string.

Invalid JSON, missing command, non-string args, non-string env values, empty
config, missing server, wrong server type, or config above the byte budget all
return `mcp runtime stdio config is invalid`. Raw JSON parser errors are never
returned.

## Sandbox Call

After parsing, the handler creates `ADKMCPRuntimeStdioSandboxCall` with:

- active run summary;
- safe runtime tool name;
- durable server ID;
- raw configured MCP tool name;
- model arguments JSON;
- parsed stdio command config.

The call intentionally does not include server display name or raw auth in
this slice. OAuth/secret resolution and encrypted secret projection remain a
future runtime boundary.

## Errors And Data Safety

Policy errors are normalized to `mcp runtime stdio policy denied`. Sandbox
errors are normalized to `mcp runtime stdio transport failed`.

Returned errors must not include:

- tool arguments;
- server name;
- raw config or auth;
- command path or args;
- env values or secret names;
- working directory;
- endpoint URLs, filenames, object keys, provider diagnostics, prompt/model
  text, transcripts, checkpoints, or secret-adjacent data.

Lifecycle events, timeout, output byte budget, server ownership, enabled
state, and tool membership validation remain owned by `ADKMCPRuntimeExecutor`.

## Non-Goals

- No `os/exec` or host command execution.
- No container, Kubernetes, or sandbox provider implementation.
- No Eino MCP stdio adapter conversion.
- No OAuth or encrypted secret retrieval.
- No session cache, health probe, audit persistence, or output offload.
- No frontend transport policy controls.

## Future Work

Next slices should add a concrete sandbox implementation with command
allow-listing, isolated working directories, environment projection, process
limits, session lifecycle, Eino MCP adapter invocation, audit records, health
classification, output offload, and production bootstrap wiring.
