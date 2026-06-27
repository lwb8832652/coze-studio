# MCP Stdio Workdir Lease Design

## Goal

M6.14 adds the durable data boundary for stdio MCP workdir leases. The goal is
to persist enough internal metadata for future cleanup, stale-run recovery, and
audit correlation without enabling real MCP process execution.

## Data Model

The new `agent_mcp_stdio_workdir_leases` table stores one row per prepared
workdir lease:

- lease ID;
- space, thread, run, and server IDs;
- Eino-safe runtime tool name;
- internal workdir path;
- status: `active`, `released`, or `failed`;
- worker ID;
- lease expiration, release, created, and updated timestamps;
- bounded sanitized last error.

The table intentionally does not store model arguments, raw MCP tool names,
command names/args, env keys/values, raw MCP config/auth, provider payloads,
prompt/model text, transcripts, checkpoint bytes, object URIs, URLs, or
filenames.

## Repository Contract

`MCPRuntimeWorkdirLeaseRepository` owns the first durable contract:

- create a lease row when a future lease-aware preparer prepares a workdir;
- read a lease by ID;
- finish an active lease as `released` or `failed`;
- list expired active leases for future recovery workers.

Finishing is compare-and-set by lease ID, active status, and worker ID when a
worker ID is supplied. This prevents stale cleanup owners from closing a lease
that another owner already recovered.

## Non-Goals

- No integration into `ADKMCPRuntimeStdioSandboxAdapter` yet.
- No cleanup worker.
- No stale lease claim/recovery loop.
- No real stdio process execution.
- No Eino MCP adapter invocation.
- No public API or frontend exposure.
- No audit event emission in this slice.

## Future Work

M6.15 should wrap the existing filesystem workdir preparer with a lease-aware
adapter. Later slices should add stale lease recovery, cleanup retry policy,
audit persistence, and production bootstrap wiring before replacing the dry-run
runner with a real Eino MCP stdio runner.
