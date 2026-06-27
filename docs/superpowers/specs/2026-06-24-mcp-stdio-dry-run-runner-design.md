# MCP Stdio Dry-Run Runner Design

## Goal

M6.13 adds a non-executing stdio runner that can exercise the full runtime
chain from transport through policy, workdir projection, preparation, runner
delegation, and cleanup. It is a safety smoke boundary only; it does not start
a process or invoke MCP.

## Runner Boundary

`ADKMCPRuntimeStdioDryRunRunner` implements
`ADKMCPRuntimeStdioSandboxRunner`. It validates the projected sandbox
execution defensively, then returns bounded JSON with safe metadata only:

- schema and dry-run status;
- Eino-safe runtime tool name;
- durable server ID;
- booleans for command and workdir presence;
- args and env counts.

The result must not include raw MCP tool names, command names, command args,
env keys or values, model arguments, workdir paths, root paths, server names,
raw config/auth, URLs, object keys, provider diagnostics, prompt/model text,
transcripts, checkpoint bytes, or secret-adjacent data.

## Validation

The runner requires the same identity and execution shape expected by the
sandbox contract:

- run, run ID, thread ID, and space ID;
- positive server ID;
- Eino-safe runtime tool name;
- non-empty raw MCP tool name;
- JSON-object arguments;
- non-empty command;
- absolute working directory.

Invalid input returns `mcp runtime stdio dry-run execution is invalid`.
Oversized dry-run output returns
`mcp runtime stdio dry-run output exceeds budget`.

## Chain Verification

The runner is wired in tests behind:

1. `ADKMCPRuntimeStdioTransport`;
2. `ADKMCPRuntimeStdioWorkdirManager`;
3. `ADKMCPRuntimeStdioStaticPolicy`;
4. `ADKMCPRuntimeStdioSandboxAdapter`;
5. `ADKMCPRuntimeStdioFilesystemWorkdirPreparer`.

This proves the stdio chain can accept an enabled MCP server and produce a
safe bounded response while still cleaning the projected workdir and avoiding
host command execution.

## Non-Goals

- No `os/exec` usage.
- No MCP stdio client/session lifecycle.
- No Eino MCP adapter invocation.
- No secret projection.
- No process or cgroup limits.
- No lease persistence or recovery.
- No audit table or health update.
- No production bootstrap wiring.

## Future Work

Follow-up slices should add durable workdir leases and stale-run recovery,
then replace the dry-run runner in production wiring with a policy-guarded
Eino MCP stdio runner that preserves the same redaction and output-budget
contracts.
