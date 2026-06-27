# MCP Stdio Dry-Run Transport Design

## Goal

M6.17 adds a single composition point for the stdio MCP dry-run smoke path. It
lets production bootstrap code explicitly assemble the safe stdio handler
without enabling real MCP process execution.

## Composition

`NewADKMCPRuntimeStdioDryRunTransport` returns an
`ADKMCPRuntimeStdioTransport` composed from:

- `ADKMCPRuntimeStdioWorkdirManager`;
- `ADKMCPRuntimeStdioStaticPolicy`;
- `ADKMCPRuntimeStdioFilesystemWorkdirPreparer`;
- `ApplicationADKMCPRuntimeStdioWorkdirLeaseStore`;
- `ADKMCPRuntimeStdioLeasedWorkdirPreparer`;
- `ADKMCPRuntimeStdioDryRunRunner`;
- `ADKMCPRuntimeStdioSandboxAdapter`.

The caller must explicitly provide workdir root, lease repository, ID
generator, worker ID, command allow-list, env allow-list, and policy budgets.
Missing or unusable dependencies fail closed at the relevant boundary.

## Runtime Behavior

When mounted as the router's stdio handler, the dry-run transport:

1. parses bounded stdio config;
2. projects the workdir under the configured root;
3. enforces static command/env/args/workdir policy;
4. prepares the filesystem workdir;
5. creates a durable active lease;
6. runs only the dry-run runner;
7. cleans the workdir;
8. marks the lease released or failed.

The result is still `coze.mcp_stdio_dry_run.v1` safe metadata only.

## Non-Goals

- No direct production `application.Init` enablement.
- No real stdio process execution.
- No Eino MCP adapter invocation.
- No secret projection.
- No OAuth/session lifecycle.
- No stale lease cleanup worker.
- No public API or frontend exposure.

## Future Work

M6.18 adds explicit runtime-policy/env wiring so the dry-run stdio transport
can be mounted in production only when configured. Later slices can add stale
lease recovery and then replace the dry-run runner with a real Eino MCP stdio
runner behind the same policy, lease, audit, and redaction boundaries.
