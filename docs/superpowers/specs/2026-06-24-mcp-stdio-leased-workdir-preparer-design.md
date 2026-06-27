# MCP Stdio Leased Workdir Preparer Design

## Goal

M6.15 wires the stdio workdir lifecycle to a lease-store interface without
starting real MCP processes. The existing sandbox still depends only on
`ADKMCPRuntimeStdioWorkdirPreparer`, so lease support is introduced as a
wrapper around the filesystem preparer.

## Wrapper Boundary

`ADKMCPRuntimeStdioLeasedWorkdirPreparer` wraps an inner
`ADKMCPRuntimeStdioWorkdirPreparer` and an
`ADKMCPRuntimeStdioWorkdirLeaseStore`.

During prepare:

1. delegate to the inner preparer;
2. create an active lease through the lease store;
3. attach lease ID and worker ID to `ADKMCPRuntimeStdioPreparedWorkdir`;
4. clean up the prepared directory if lease creation fails.

During cleanup:

1. delegate to the inner preparer cleanup;
2. finish the lease as `released` when cleanup succeeds;
3. finish the lease as `failed` with sanitized `cleanup failed` when cleanup
   fails;
4. return only fixed sanitized cleanup or lease-finish errors.

## Redaction

Wrapper errors must not include model arguments, command names/args, env
keys/values, workdir/root paths, raw MCP tool names, server names, raw
config/auth, URLs, object keys, provider diagnostics, prompt/model text,
transcripts, checkpoint bytes, or secret-adjacent data.

## Non-Goals

- No concrete MySQL lease-store adapter in production bootstrap.
- No cleanup worker.
- No stale lease claim/recovery loop.
- No real stdio process execution.
- No Eino MCP adapter invocation.
- No public API or frontend exposure.

## Future Work

M6.16 should add the concrete application lease-store adapter that uses the
M6.14 repository plus an ID generator and configured worker identity, then
production bootstrap can compose manager, policy, filesystem preparer, leased
preparer, sandbox, and the dry-run runner for a safe smoke path.
