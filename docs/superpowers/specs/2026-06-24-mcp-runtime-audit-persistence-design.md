# MCP Runtime Audit Persistence Design

## Goal

M6.21 adds a durable, metadata-only audit record path for MCP runtime tool
execution. This gives the future real MCP adapter a content-safe audit trail
before host process execution or remote MCP sessions are enabled.

## Data Model

`agent_mcp_runtime_audit_events` stores only bounded execution metadata:

- audit event ID;
- space, thread, run, and server IDs;
- Eino-safe runtime tool name;
- lifecycle event type;
- sanitized error code;
- elapsed milliseconds;
- output byte count;
- creation timestamp.

The table intentionally has no JSON payload, no arguments column, no output
column, no config/auth column, no URL/object-key/file-name column, and no raw
provider body column.

## Runtime Integration

`ADKMCPRuntimeExecutor` accepts an optional `ADKMCPRuntimeAuditRecorder`.
Lifecycle events are recorded alongside existing content-free run events:

- `mcp.tool.started`;
- `mcp.tool.completed`;
- `mcp.tool.failed`.

`ApplicationADKMCPRuntimeAuditRecorder` owns ID generation and maps executor
metadata to `MCPRuntimeAuditRepository`. `application.Init` wires this recorder
through `NewADKMCPRuntimeToolExecutorFromConfig` when the MCP runtime executor
is explicitly enabled.

## Safety Rules

- Audit recorder input must be metadata-only.
- Audit fields are trimmed and bounded before persistence.
- Audit write failures return only `mcp runtime audit record failed` from the
  recorder.
- Executor audit recording must not put tool arguments, model input/output,
  tool output, MCP config/auth, command args, env values, URLs, filenames,
  object keys, provider diagnostics, transcripts, checkpoints, or secrets into
  audit rows, run events, or model-facing errors.
- The current executor treats audit recording as an auxiliary content-free
  signal; future real MCP execution may tighten this into fail-closed audit
  policy once the production audit SLO is defined.

## Non-Goals

- No public Workbench audit browsing API.
- No frontend audit UI.
- No raw provider payload capture.
- No pricing or token attribution.
- No real MCP process/session execution.

## Future Work

Future slices can add admin-only audit browsing, retention policy, metrics,
OpenTelemetry span linkage, and fail-closed audit policy for real MCP
transport execution.
