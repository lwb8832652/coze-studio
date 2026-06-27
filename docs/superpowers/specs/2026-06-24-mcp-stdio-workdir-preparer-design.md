# MCP Stdio Workdir Preparer Design

## Goal

M6.12 adds a filesystem-backed working-directory preparer for stdio MCP
runtime calls. This slice may create, chmod, and remove the projected workdir,
but it still does not execute commands or invoke MCP.

## Preparer Boundary

`ADKMCPRuntimeStdioWorkdirPreparer` owns the directory lifecycle contract:

- `PrepareADKMCPRuntimeStdioWorkdir(ctx, execution)` creates and permissions
  the projected directory.
- `CleanupADKMCPRuntimeStdioWorkdir(ctx, prepared)` removes the prepared
  working directory.

`ADKMCPRuntimeStdioPreparedWorkdir` carries only:

- root;
- working directory.

The filesystem implementation is
`ADKMCPRuntimeStdioFilesystemWorkdirPreparer`. It requires an absolute root,
rejects workdirs outside that root, rejects the root itself as a workdir, and
uses `MkdirAll`, `Chmod`, `Stat`, and guarded `RemoveAll`.

## Sandbox Integration

`ADKMCPRuntimeStdioSandboxOptions` now accepts optional `WorkdirPreparer`.
When configured, `ADKMCPRuntimeStdioSandboxAdapter`:

1. validates and projects the sandbox execution;
2. prepares the workdir;
3. delegates to the runner;
4. attempts cleanup after runner completion or runner failure.

If runner execution fails, the adapter still attempts cleanup and returns the
sanitized runner failure. If runner succeeds but cleanup fails, the adapter
returns `mcp runtime stdio workdir cleanup failed`.

## Errors And Redaction

The preparer returns fixed errors:

- `mcp runtime stdio workdir prepare failed`;
- `mcp runtime stdio workdir cleanup failed`.

Errors must not include model arguments, command names/args, env keys/values,
workdir paths, root paths, server names, raw config/auth, URLs, object keys,
provider diagnostics, prompt/model text, transcripts, checkpoint bytes, or
secret-adjacent data.

## Non-Goals

- No command execution.
- No Eino MCP adapter invocation.
- No secret projection.
- No process/session limits.
- No lease persistence.
- No audit table or health update.
- No production bootstrap wiring.

## Future Work

Follow-up slices should add durable lease records, stale workdir recovery,
audit events, optional retention policy, and then the real stdio MCP runner
behind the existing runner contract.
