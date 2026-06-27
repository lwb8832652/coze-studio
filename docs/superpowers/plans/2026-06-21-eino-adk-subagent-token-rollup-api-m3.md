# M3.21 Eino ADK Subagent Token Rollup API Plan

## Checklist

- [x] Add red handler test for `include_child_runs=true`.
- [x] Add `include_child_runs` to API and frontend schema request types.
- [x] Add repository `RunIDs` filters for usage rows and aggregate totals.
- [x] Resolve direct child run IDs in the domain service.
- [x] Preserve existing run-scoped token usage behavior by default.
- [x] Add domain, repository, application, and handler coverage.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.

## Verification

```bash
cd backend && go test ./api/handler/coze -run TestGetTaskThreadTokenUsageHandlerCanIncludeChildRuns -count=1 -gcflags="all=-l -N"
```

```bash
cd backend && go test ./domain/agentthread/service -run 'TestGetRunTokenUsage' -count=1 -gcflags="all=-l -N"
```

```bash
cd backend && go test ./domain/agentthread/repository -run TestThreadRepositoryCreateAndAggregateTokenUsage -count=1 -gcflags="all=-l -N"
```

```bash
cd backend && go test ./application/agentthread -run TestApplicationTokenUsageMethodsMapDomainUsage -count=1 -gcflags="all=-l -N"
```
