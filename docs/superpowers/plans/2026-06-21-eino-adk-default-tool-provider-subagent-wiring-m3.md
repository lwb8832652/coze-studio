# M3.11 Eino ADK Default Tool Provider Subagent Wiring Plan

## Checklist

- [x] Add red tests for the subagent-aware default provider shape.
- [x] Split the default runtime tool provider from the policy wrapper.
- [x] Add `NewDefaultADKToolProviderWithSingleAgentSubagents`.
- [x] Wire SingleAgent snapshot grants into the default subagent provider.
- [x] Build child SingleAgent agents with the default non-recursive provider.
- [x] Move application initialization so SingleAgent service is available
      before the ADK executor is built.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused, application, and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestDefaultADKToolProviderWithSingleAgentSubagents|TestDefaultADKToolProviderIncludesHumanInteractionTools' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
