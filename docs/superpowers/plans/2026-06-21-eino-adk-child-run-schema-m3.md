# M3.13 Eino ADK Child Run Schema Plan

## Checklist

- [x] Add red repository tests for child run create/get mapping.
- [x] Add red repository tests for top-level default list filtering and
      parent child-run queries.
- [x] Add red worker-claim test proving pending child rows are skipped.
- [x] Add red service tests for subagent run defaults and parent-thread
      validation.
- [x] Add `RunKind`, `parent_run_id`, and `run_kind` to entity, DTO, service,
      repository, and application mappings.
- [x] Add Atlas migration and update latest schema HCL.
- [x] Regenerate `atlas.sum` with Atlas Community v0.35.0.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused backend tests and Atlas validation.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" \
  ./domain/agentthread/repository ./domain/agentthread/service \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestApplicationCreateRunMapsDomainRun|TestApplicationListRunsMapsDomainRuns|TestADKSingleAgentSubagentRunSummary' \
  -count=1
```

```bash
/tmp/atlas-v0.35.0 migrate validate --dir file://docker/atlas/migrations
```
