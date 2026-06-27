# MCP Stdio Workdir Lease Reaper Design

## Goal

M6.19 adds a one-shot cleanup boundary for expired active stdio MCP workdir
leases. It lets a future worker safely clean stale workdirs left behind after
process crashes, timeouts, or interrupted runs.

## Behavior

`ADKMCPRuntimeStdioWorkdirLeaseReaper`:

1. lists expired active leases through `MCPRuntimeWorkdirLeaseRepository`;
2. validates each lease before filesystem cleanup;
3. removes only workdirs under the configured absolute root;
4. refuses to remove the root itself or paths outside root;
5. uses the original lease `worker_id` when finishing the row;
6. marks successfully cleaned stale leases as `failed` with sanitized
   `last_error = "stale lease expired"`;
7. returns aggregate counters for listed, cleaned, finished, invalid, and
   failed rows.

Repository list failures are fatal and return the fixed sanitized error
`mcp runtime stdio workdir lease cleanup failed`. Per-lease cleanup or finish
failures are counted and the reaper continues.

## Safety Rules

- The reaper must never clean a relative path, root path, or sibling path.
- Empty lease worker IDs are invalid; the reaper must not close such rows.
- Cleanup happens before lease finish. If cleanup fails, the active lease stays
  open for a future retry.
- Finishing uses active-status compare-and-set plus the original worker ID, so
  a row changed by another worker is not closed by stale cleanup.
- Errors and result counters must not include workdir paths, root paths, model
  arguments, raw MCP tool names, command names/args, env keys/values,
  config/auth, URLs, object keys, provider diagnostics, prompt/model text,
  transcripts, checkpoint bytes, or secrets.

## Non-Goals

- No production scheduled worker.
- No application bootstrap wiring.
- No audit persistence.
- No real stdio process execution.
- No Eino MCP adapter invocation.
- No public API or frontend exposure.

## Future Work

M6.20 adds the explicit env-controlled scheduled worker that invokes this
reaper with bounded interval, batch size, and root configuration. Later work
can add audit persistence, metrics, and health classification.
