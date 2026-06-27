# MCP Stdio Workdir Lease Reaper Worker Design

## Goal

M6.20 mounts the stale stdio workdir lease reaper behind an explicit
environment-controlled scheduled worker. It keeps stale cleanup disabled by
default and does not enable real MCP execution.

## Environment Contract

The worker starts only when
`AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ENABLED=true`.

Required setting when enabled:

- `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_ROOT`: absolute Coze-owned stdio
  workdir root to clean under.

Optional settings:

- `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_BATCH_SIZE`: expired lease batch size,
  default `10`.
- `AGENT_THREAD_MCP_STDIO_WORKDIR_REAPER_INTERVAL_MS`: worker interval, default
  `300000`.

The reaper worker uses its own explicit root env and does not automatically
inherit `AGENT_THREAD_MCP_STDIO_WORKDIR_ROOT`. Operators must opt in to cleanup
deliberately.

## Runtime Behavior

`StartADKMCPRuntimeStdioWorkdirLeaseReaperWorkerFromEnvWithStatus`:

1. returns nil when disabled;
2. refuses to start without a durable workdir lease repository;
3. refuses to start when the configured root is missing or not absolute;
4. constructs `ADKMCPRuntimeStdioWorkdirLeaseReaper`;
5. starts `ADKMCPRuntimeStdioWorkdirLeaseReaperWorker`.

`ADKMCPRuntimeStdioWorkdirLeaseReaperWorker` uses a ticker and calls
`CleanupExpiredADKMCPRuntimeStdioWorkdirLeases` on each tick. `RunOnce` is also
available for tests and future admin/maintenance hooks.

## Safety Rules

- Default-off behavior is required.
- Startup status reasons must not include raw root values.
- Worker logs may include only aggregate counters and fixed sanitized errors.
- The worker must not expose lease rows, workdir paths, command details, env
  values, config/auth, prompts, model text, transcripts, checkpoint bytes,
  provider payloads, URLs, object keys, or secrets.
- Cleanup path validation and CAS finishing remain owned by the M6.19 reaper.

## Non-Goals

- No public API or frontend controls.
- No metrics/exporter integration.
- No real stdio process execution.
- No Eino MCP adapter invocation.

## Future Work

M6.21 adds MCP runtime audit persistence. Later slices can add metrics, health
classification, and operator-facing diagnostics for stale cleanup counts.
