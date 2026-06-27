# Artifact Deleted Browsing M4 Plan

## Result

M4.31 adds explicit deleted-only artifact browsing and restores older removed
artifacts from the task artifact drawer.

## Completed

- Added `deleted_only` propagation through Workbench API, application service,
  domain service, and repository list requests.
- Kept default artifact listing active-only and added deleted-only repository
  filtering for `deleted_at > 0`.
- Added `deleted_at` to artifact summary/API payloads for lifecycle display.
- Added a task drawer `当前 / 已移除` switch.
- Added a removed-artifact section that loads `deleted_only=true` and exposes
  only row-level restore.
- Restoring from the removed list refreshes both active and deleted lists.
- Added backend and frontend regression tests.
- Updated AGENTS and the master roadmap.

## Verification

- `go test ./domain/agentthread/repository ./application/agentthread ./api/handler/coze -run 'TestArtifactRepositoryUpsertsByFileAndListsByThread|TestApplicationListArtifactsMapsDomainArtifacts|TestListTaskThreadArtifactsHandlerReturnsDeletedArtifactsWhenRequested' -count=1 -gcflags="all=-l -N"`
- `npx vitest run src/pages/tasks/__tests__/task-detail.test.tsx --testNamePattern="lists deleted thread artifacts"`
- `npx eslint src/pages/tasks/task-artifacts-panel.tsx src/pages/tasks/task-deleted-artifacts-section.tsx src/pages/tasks/task-artifact-scan-jobs-section.tsx src/pages/tasks/__tests__/task-detail.test.tsx --quiet`

## Remaining

Physical cleanup, signed links, richer preview transforms, MIME-specific
renderer hardening, and browser E2E coverage remain separate M4 or production
acceptance tasks.
