# MCP Stdio Workdir Lease Store Design

## Goal

M6.16 adds the concrete application adapter that connects the stdio workdir
lease wrapper to the durable repository created in M6.14. It remains an
internal runtime boundary and does not expose workdir lease data through
Workbench or LangGraph APIs.

## Adapter Boundary

`ApplicationADKMCPRuntimeStdioWorkdirLeaseStore` implements
`ADKMCPRuntimeStdioWorkdirLeaseStore` and depends on:

- `MCPRuntimeWorkdirLeaseRepository`;
- `idgen.IDGenerator`;
- configured worker ID;
- configured lease TTL, defaulting to 300000 ms;
- optional test clock.

On create, it validates run identity, server ID, Eino-safe runtime tool name,
absolute prepared workdir, repository, ID generator, worker ID, and positive
TTL. It then generates a lease ID and writes an active
`MCPRuntimeWorkdirLease` row with `lease_expires_at = now + ttl`.

On finish, it maps application statuses to domain statuses, uses the lease
worker ID when present, falls back to the configured worker ID, and requires
the repository compare-and-set to update an active lease. Repository errors or
zero-row updates return fixed sanitized failure.

## Redaction

Create and finish errors are fixed:

- `mcp runtime stdio workdir lease create failed`;
- `mcp runtime stdio workdir lease finish failed`.

They must not include generated IDs, workdir paths, raw repository errors,
model arguments, raw MCP tool names, command names/args, env keys/values, raw
config/auth, URLs, object keys, provider diagnostics, prompt/model text,
transcripts, checkpoint bytes, or secret-adjacent data.

## Non-Goals

- No production bootstrap wiring yet.
- No cleanup worker.
- No stale lease claim/recovery loop.
- No real stdio process execution.
- No Eino MCP adapter invocation.
- No public API or frontend exposure.

## Future Work

M6.17 should compose the workdir manager, static policy, filesystem preparer,
lease store, leased preparer, sandbox, and dry-run runner in a production
smoke wiring path that remains disabled unless explicit runtime policy enables
it. Later slices should add stale lease recovery and then real Eino MCP stdio
adapter invocation.
