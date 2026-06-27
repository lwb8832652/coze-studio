# M3.9 Eino ADK Subagent Tool Grant Provider Plan

## Checklist

- [x] Add red tests for durable grants populating child allowed tools.
- [x] Add red tests for requested tool names intersecting with durable grants.
- [x] Add red tests for grant provider error propagation.
- [x] Implement `ADKSubagentToolGrantProvider`.
- [x] Wire grants into `ADKSingleAgentSubagentDefinitionProvider`.
- [x] Keep no-grant-provider behavior compatible with temporary run-config
      allow-lists.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKSingleAgentSubagentDefinitionProvider.*ToolGrant|TestADKSingleAgentSubagentDefinitionProviderIntersects' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
