# M3.5 Eino ADK Subagent Policy Limits Plan

## Checklist

- [x] Add red tests for `max_subagents` and recursive `max_depth`.
- [x] Add red test coverage for child run subagent metadata.
- [x] Implement `ADKSubagentPolicy` parsing from run config.
- [x] Enforce limits before building child `AgentTool` instances.
- [x] Persist standardized child subagent identity metadata.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKSubagentToolProviderEnforces|TestADKSingleAgentSubagentRunSummaryMapsStableSnapshotFields' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
