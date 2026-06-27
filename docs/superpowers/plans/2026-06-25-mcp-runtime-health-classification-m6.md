# MCP Runtime Health Classification M6 Plan

**Goal:** Update existing Workbench MCP server health metadata from real MCP
runtime terminal outcomes without leaking runtime content.

**Architecture:** `ADKMCPRuntimeExecutor` owns the post-transport signal and
calls an optional `ADKMCPRuntimeHealthReporter`. `mcptool.ApplicationService`
owns the health-state mapping, checked timestamp fallback, latency clamping,
and sanitized error-code persistence through the existing catalog
`UpdateHealth` method.

**Tech Stack:** Go, Coze-owned ADK MCP runtime executor, Workbench MCP catalog.

## Implementation

- [x] Add `ADKMCPRuntimeHealthReport`,
  `ADKMCPRuntimeHealthReporter`, and function adapter.
- [x] Add `WithADKMCPRuntimeExecutorHealthReporter`.
- [x] Report healthy after successful post-transport execution.
- [x] Report unhealthy for `transport_failed` and
  `output_budget_exceeded`.
- [x] Keep pre-transport validation, authorization, and configuration failures
  from mutating health state.
- [x] Add `MCPRuntimeHealthReport` and
  `ApplicationService.RecordRuntimeHealth`.
- [x] Sanitize failure health errors to short metadata-only codes.
- [x] Wire production bootstrap through `application.Init`.
- [x] Update AGENTS and the master DeerFlow parity roadmap.

## Verification

- `go test ./application/mcptool ./application/agentthread -run 'TestApplicationServiceRecordsRuntimeHealth|TestADKMCPRuntimeExecutorReports.*Health|TestADKMCPRuntimeExecutorDoesNotReportHealthForPreTransportValidation|TestNewADKMCPRuntimeToolExecutorFromConfigInvokesDryRunStdio' -count=1`
- `go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./application/mcptool ./application -count=1`
- `docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

M6 still needs real Eino MCP stdio adapter invocation, SSE/streamable HTTP
transport support, output offload for large MCP results, operator-facing
diagnostics and metrics/exporter integration, frontend policy controls, and
browser E2E coverage. Health classification is now covered for the current
runtime transport boundary.
