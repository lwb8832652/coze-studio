# Artifact Restore M4 Plan

## Result

M4.30 adds registry-only artifact restore and an immediate drawer undo action.

## Completed

- Added repository, domain, application, handler, route, frontend service, and
  drawer tests.
- Implemented scoped restore CAS for deleted `agent_artifacts` rows.
- Added `RestoreArtifact` application flow with restore authorization and
  content-free `artifact.restored` audit events.
- Added Workbench restore endpoint and route registration.
- Added frontend restore service and `撤销移除` drawer action after delete.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./api/handler/coze ./api/router/coze -run 'Test.*Restore|TestDeleteTaskThreadArtifactHandlerHidesArtifactFromList|TestRegisterIncludesWorkbenchTaskThreadRoutes' -count=1 -gcflags="all=-l -N"`
- `npx vitest run src/pages/tasks/__tests__/tasks-service.test.ts --testNamePattern='restore|exports'`
- `npx vitest run src/pages/tasks/__tests__/task-detail.test.tsx --testNamePattern='restores the last removed thread artifact'`

## Remaining

Physical cleanup, signed links, richer preview transforms,
MIME-specific renderer hardening, and browser E2E coverage remain separate M4
or production-acceptance tasks.
