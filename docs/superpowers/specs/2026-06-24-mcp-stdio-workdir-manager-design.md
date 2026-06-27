# MCP Stdio Workdir Manager Design

## Goal

M6.11 adds an isolated working-directory projection boundary for stdio MCP
runtime calls. It does not create directories, mount filesystems, execute
commands, or invoke MCP. It only derives a deterministic Coze-owned absolute
path and lets the existing stdio transport pass that path through policy and
sandbox boundaries.

## Manager Boundary

`ADKMCPRuntimeStdioWorkdirProjector` projects
`ADKMCPRuntimeStdioWorkdirRequest` into
`ADKMCPRuntimeStdioWorkdirProjection`.

`ADKMCPRuntimeStdioWorkdirManager` is the first concrete implementation. It
requires an absolute `Root` and produces:

```text
{root}/spaces/{space_id}/threads/{thread_id}/runs/{run_id}/servers/{server_id}/tools/{safe_runtime_tool_name}
```

The manager validates:

- root is absolute;
- run, run ID, thread ID, and space ID are present;
- server ID is positive;
- runtime tool name is Eino-safe;
- raw MCP tool name is non-empty;
- projected path remains under root after cleaning.

Any invalid input returns `mcp runtime stdio workdir is invalid`.

## Transport Integration

`ADKMCPRuntimeStdioTransportOptions` now accepts optional `WorkdirManager`.
When configured, transport parses stdio config first, then asks the manager for
the Coze-owned workdir and overwrites parsed `Config.WorkingDir` before policy
validation. This means static policy validates the Coze-owned isolated path
instead of trusting MCP server-provided `cwd`.

When no workdir manager is configured, the transport keeps the previous M6.8
behavior and passes parsed config workdir through unchanged.

## Safety Rules

The workdir manager must not:

- create directories;
- touch host filesystem state;
- resolve symlinks;
- mount filesystems;
- project secrets;
- execute commands;
- invoke Eino MCP adapters;
- write audit records or health updates.

Errors must not include model arguments, command names/args, env keys/values,
configured `cwd`, projected paths, root paths, server names, raw config/auth,
URLs, object keys, provider diagnostics, prompt/model text, transcripts,
checkpoint bytes, or secret-adjacent data.

## Future Work

Follow-up slices should add a filesystem-backed workdir preparer that can
create, permission, clean, lease, and audit these projected directories behind
the same validation boundary.
