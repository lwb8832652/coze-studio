# MCP Stdio Eino Runner M6 Plan

**Goal:** Add a real stdio MCP runner that invokes tools through Eino's MCP
adapter while staying inside Coze-owned runtime, policy, workdir, audit, and
health boundaries.

**Architecture:** `ADKMCPRuntimeStdioEinoRunner` implements
`ADKMCPRuntimeStdioSandboxRunner`. It receives a projected and policy-checked
`ADKMCPRuntimeStdioSandboxExecution`, creates an initialized MCP client through
an injectable factory, asks an injectable tool provider for the target Eino
tool, invokes the tool, bounds output, and closes the client. Bootstrap can
select dry-run or real Eino stdio mode through mutually exclusive env flags.

**Tech Stack:** Go, Eino MCP adapter
`github.com/cloudwego/eino-ext/components/tool/mcp v0.0.8`, mcp-go
`github.com/mark3labs/mcp-go v0.43.0`.

## Implementation

- [x] Added `ADKMCPRuntimeStdioEinoRunner`.
- [x] Added `ADKMCPRuntimeStdioEinoClientFactory` and
  `ADKMCPRuntimeStdioEinoToolProvider` seams for testing and future session
  management.
- [x] Added mcp-go client factory that starts and initializes a stdio MCP
  client with a Coze-owned working-directory command factory.
- [x] Added Eino MCP tool provider that calls `mcp.GetTools` for the selected
  tool only.
- [x] Added fixed sanitized errors for invalid execution, client failure,
  tool discovery failure, missing tool, non-invokable tool, tool call failure,
  and output-budget failure.
- [x] Refactored dry-run transport composition into
  `NewADKMCPRuntimeStdioRuntimeTransport`.
- [x] Added `AGENT_THREAD_MCP_STDIO_EINO_ENABLED` env parsing and mutually
  exclusive dry-run/Eino mode validation.
- [x] Wired real Eino stdio mode into `NewADKMCPRuntimeToolExecutorFromConfig`
  while preserving default-off behavior.
- [x] Updated AGENTS and the master DeerFlow parity roadmap.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioEinoRunner|TestADKMCPRuntimeBootstrapConfigFromEnvParsesEinoStdio|TestADKMCPRuntimeBootstrapConfigFromEnvRejectsMultipleStdioModes|TestNewADKMCPRuntimeToolExecutorFromConfigBuildsEinoStdio|TestNewADKMCPRuntimeToolExecutorFromConfigInvokesDryRunStdio' -count=1`
- `go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./application/mcptool ./application -count=1`
- `docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations`
- `git diff --check`

## Remaining

M6 still needs session pooling and lifecycle policy, secret projection and
encrypted credential retrieval, SSE/streamable HTTP transport support,
metrics/exporter and operator diagnostics, frontend policy controls, and
browser E2E coverage.
