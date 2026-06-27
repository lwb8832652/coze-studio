# M3.10 Eino ADK SingleAgent Snapshot Tool Grant Plan

## Checklist

- [x] Add red tests for plugin and workflow snapshot grant names.
- [x] Add red tests for deterministic fallback names.
- [x] Add red tests for incomplete snapshot entries being skipped.
- [x] Implement `ADKSingleAgentSnapshotToolGrantProvider`.
- [x] Reuse the existing tool-policy name validator and config normalization.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Run focused and package-level backend tests.

## Verification

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread \
  -run 'TestADKSingleAgentSnapshotToolGrantProvider' \
  -count=1
```

```bash
cd backend && go test -gcflags="all=-l -N" ./application/agentthread
```
