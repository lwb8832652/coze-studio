# MCP Runtime Output Offload M6 Plan

**Goal:** Offload oversized MCP runtime results through the existing
Coze-owned runtime file backend instead of failing every large but otherwise
successful MCP call.

**Architecture:** `ADKMCPRuntimeExecutor` remains the post-transport budget
owner. When output exceeds the inline budget, it calls an injected
`ADKMCPRuntimeOutputOffloader`. The production adapter builds an
`ADKOffloadBackend` for the active run, writes the raw output as a runtime
offload file, and returns a bounded `coze.mcp_runtime_output_offload.v1`
notice.

## Implementation

- [x] Added `ADKMCPRuntimeOutputOffloadRequest`,
  `ADKMCPRuntimeOutputOffloadResult`, and `ADKMCPRuntimeOutputOffloader`.
- [x] Added `WithADKMCPRuntimeExecutorOutputOffloader`.
- [x] Changed oversized post-transport MCP output handling:
  - no offloader keeps fail-closed `output_budget_exceeded`;
  - configured offloader success returns a small notice and marks health
    successful;
  - configured offloader failure returns fixed `output_offload_failed` and
    marks health unhealthy.
- [x] Added `ADKMCPRuntimeOutputOffloadBackendAdapter` on top of
  `ADKOffloadBackend`.
- [x] Wired bootstrap dependencies and `application.Init` to reuse the same
  `ADKOffloadBackendFactory` as ADK reduction.
- [x] Updated AGENTS and master roadmap context.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeExecutorOffloadsOversizedOutput|TestADKMCPRuntimeExecutorFailsClosedWhenOutputOffloadFails|TestADKMCPRuntimeOutputOffloadBackendAdapterWritesSafeNotice|TestNewADKMCPRuntimeToolExecutorFromConfigInjectsOutputOffloader' -count=1`
- `go test ./application/agentthread -run 'TestADKMCPRuntimeExecutor|TestNewADKMCPRuntimeToolExecutorFromConfig' -count=1`
- `go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./application/mcptool ./application -count=1`
- `docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

M6 still needs session pooling and lifecycle policy, secret projection and
encrypted credential retrieval, SSE/streamable HTTP transport support,
metrics/exporter and operator diagnostics, frontend policy controls, and
browser E2E coverage.
