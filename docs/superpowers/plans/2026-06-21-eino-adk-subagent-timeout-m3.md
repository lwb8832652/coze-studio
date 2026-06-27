# M3.6 Eino ADK Subagent Timeout Plan

## Checklist

- [x] Inspect Eino `AgentTool` and `tool.InvokableTool` interfaces.
- [x] Add red test coverage for a blocking child model with `timeout_ms`.
- [x] Add `timeout_ms` / `timeoutMs` to `ADKSubagentPolicy`.
- [x] Wrap child `AgentTool` invocation with `context.WithTimeout`.
- [x] Preserve Eino tool metadata and cancellation error propagation.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKSubagentToolProviderAppliesTimeoutPolicy|TestADKSubagentToolProviderEnforces' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
