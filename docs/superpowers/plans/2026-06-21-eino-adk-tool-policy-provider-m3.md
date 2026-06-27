# M3.7 Eino ADK Tool Policy Provider Plan

## Checklist

- [x] Add red tests for static and dynamic tool allow-list filtering.
- [x] Add red tests for explicit empty allow-lists denying all tools.
- [x] Add red tests for child run default deny-all and explicit child grants.
- [x] Implement `ADKToolPolicyProvider`.
- [x] Add policy parsing to `subagents` and `subagent_refs` temporary config.
- [x] Map child SingleAgent allow-lists into generated child run config.
- [x] Wrap `NewDefaultADKToolProvider` with the policy provider.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKToolPolicyProvider|TestADKRunConfigSubagentDefinitionProviderParsesDefinitions|TestADKRunConfigSubagentReferenceProviderParsesRefs|TestADKSingleAgentSubagentRunSummary|TestDefaultADKToolProvider' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
