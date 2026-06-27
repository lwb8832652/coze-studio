# MCP Runtime Output Offload Design

M6.24 adds the MCP runtime output-offload boundary. The goal is to keep real
MCP tool calls usable when their result is larger than the inline response
budget, without placing raw tool output into run events, audit rows, health
records, errors, or model-visible metadata beyond a small file reference.

## Scope

This slice introduces:

- `ADKMCPRuntimeOutputOffloader`, an executor-level hook for oversized MCP
  results.
- `ADKMCPRuntimeOutputOffloadBackendAdapter`, which reuses the existing
  `ADKOffloadBackend` object-storage and runtime-file registry path.
- A model-visible `coze.mcp_runtime_output_offload.v1` JSON notice.
- Bootstrap wiring from `application.Init` through the existing
  `ADKOffloadBackendFactory`.

It does not add a new storage table, public API, object-URI exposure, session
pooling, secret projection, SSE/HTTP transports, metrics/exporter, or frontend
controls.

## Execution Flow

1. `ADKMCPRuntimeExecutor` validates the MCP call and invokes the selected
   transport.
2. If the transport result is within `ExecutorMaxOutputBytes`, the result is
   returned inline as before.
3. If the result is oversized and no output offloader is configured, the
   executor preserves fail-closed `output_budget_exceeded` behavior.
4. If an output offloader is configured, the executor calls it with the active
   run, Eino-safe runtime tool name, server ID, raw configured MCP tool name,
   raw result content, and start timestamp.
5. The backend adapter writes the content through `ADKOffloadBackend.Write`
   using the existing `trunc` phase path:
   `/mnt/user-data/workspace/.coze/tool-results/runs/{run_id}/trunc/{sha}.txt`.
6. The adapter returns a bounded JSON notice containing only safe metadata:
   schema, `offloaded=true`, runtime tool name, server ID, output byte count,
   virtual path, and `read_tool="read_file"`.
7. The executor returns the notice to the model, emits normal completed
   lifecycle metadata, records audit output bytes as the original output size,
   and reports runtime health as success.

## Safety Contract

The notice must not contain raw output bytes, model input/output, tool
arguments, MCP config/auth, command args, env values, object URI, storage URL,
provider payloads, transcripts, checkpoint bytes, stack traces, provider file
names, or secrets.

Errors are fixed and sanitized:

- Missing offloader: `output_budget_exceeded`.
- Configured offloader failure: `output_offload_failed`.

Offloader errors must not include the raw output or object storage details.
Health status is updated only after transport. Successful offload is healthy;
offload failure is unhealthy with `output_offload_failed`.

## Reuse Boundary

MCP output offload intentionally reuses the ADK tool-result offload backend
instead of adding a second persistence path. This keeps object keys, runtime
file registration, read authorization, and future cleanup under the same
Coze-owned boundary already used by Eino reduction and filesystem middleware.
