# MCP Runtime Health Classification Design

M6.22 connects MCP runtime terminal outcomes to the existing Workbench MCP
server health fields. This slice does not add a new table or a new public API.
It makes runtime health updates an internal application-service concern so the
task runtime can report metadata while the MCP tool service owns Workbench
health normalization.

## Goals

- Mark an MCP server `healthy` after a runtime tool call reaches transport and
  completes successfully.
- Mark an MCP server `unhealthy` after a runtime tool call reaches transport
  and fails with a terminal runtime condition.
- Leave health unchanged for pre-transport validation and policy failures such
  as invalid run context, space mismatch, disabled server, invalid arguments,
  missing tool configuration, missing resolver, or missing transport.
- Keep health metadata bounded and content-free.

## Boundaries

`ADKMCPRuntimeExecutor` gets an optional
`ADKMCPRuntimeHealthReporter`. It emits `ADKMCPRuntimeHealthReport` only after
the transport boundary has been reached:

- success: `Success=true`, empty error code;
- transport failure: `Success=false`, `ErrorCode=transport_failed`;
- output budget failure: `Success=false`,
  `ErrorCode=output_budget_exceeded`.

The executor never sends MCP arguments, model input/output, transport errors,
server config/auth, command details, workdir paths, URLs, object keys, provider
payloads, checkpoint bytes, or tool output through this hook.

`mcptool.ApplicationService.RecordRuntimeHealth` maps the report to
`Catalog.UpdateHealth`:

- success becomes `health_status=healthy`;
- failure becomes `health_status=unhealthy`;
- missing checked time is filled with current wall-clock milliseconds;
- negative latency is clamped to zero;
- failure `health_error` is a short sanitized error code.

Invalid or secret-adjacent error text is not preserved. Runtime failure error
codes must be short ASCII identifiers using letters, digits, `_`, `-`, or `.`;
any other input becomes `runtime_failed`.

## Bootstrap

`ADKMCPRuntimeBootstrapDependencies` accepts the health reporter and passes it
to `NewADKMCPRuntimeExecutor`. `application.Init` adapts the agentthread report
to `primaryServices.mcpToolSVC.RecordRuntimeHealth`.

The production bootstrap remains default-off for MCP runtime execution. Health
classification only runs when the optional MCP runtime executor is actually
installed and a runtime tool reaches the transport layer.

## Security

Health responses may expose only status, checked timestamp, latency, and a
bounded sanitized error code. They must not expose:

- tool arguments or tool output;
- model prompts, completions, transcripts, or checkpoint bytes;
- MCP config/auth, credentials, command args, env values, workdir paths, URLs,
  filenames, object keys, or provider raw payloads;
- raw transport error strings or stack traces.

## Tests

The slice needs focused tests for:

- executor reports healthy after successful transport;
- executor reports unhealthy after transport failure;
- executor skips health updates for pre-transport validation failures;
- bootstrap passes the reporter into the configured executor;
- mcptool service records healthy/unhealthy states and sanitizes unsafe error
  text before persisting it.
