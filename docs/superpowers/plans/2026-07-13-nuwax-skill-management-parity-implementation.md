# Nuwax Skill Management Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-grade Coze-native skill management workflow whose visible behavior and functional closure match Nuwax for list management, two creation paths, project editing, versioning, testing, publishing, copying, import/export, and conversation-linked development.

**Architecture:** Extend the existing Workbench Skill Thrift contract and the current `skills`, `skill_versions`, and `skill_resources` persistence model instead of creating a parallel subsystem. Every file mutation creates a new immutable version snapshot with optimistic concurrency; publication points to an immutable version. The React frontend adds a Nuwax-aligned management page and dedicated detail workbench while retaining React Router, Coze Design, Semi, and the existing `mode=skill` task flow.

**Tech Stack:** Go, Hertz, GORM, MySQL, Thrift/hz, Atlas v0.35.0, React 18, TypeScript, React Router, Coze Design/Semi UI, Vitest.

**Execution constraint:** Do not run Git commands, create commits, merge, or push unless the user explicitly authorizes those operations in the active turn.

---

## File responsibility map

### Contract and persistence

- Modify `idl/workbench/skill.thrift`: source-of-truth public skill contracts and routes.
- Regenerate `backend/api/model/workbench/skill/skill.go`: Go API models.
- Extend `backend/api/model/workbench/skill/skill_extension.go` only when `hz update` cannot represent a required compatibility helper.
- Regenerate `frontend/packages/arch/api-schema/src/idl/workbench/skill.ts`: typed frontend client.
- Create `docker/atlas/migrations/20260713000100_skill_management_parity.sql`: metadata, optimistic revision, and immutable publication records.

### Backend domain and API

- Modify `backend/domain/skill/entity/skill.go`: metadata and publication entities.
- Modify `backend/domain/skill/repository/repository.go`: filtered paging, publication, and snapshot operations.
- Modify `backend/domain/skill/repository/mysql.go`: tenant-scoped persistence.
- Modify `backend/domain/skill/repository/mysql_test.go`: repository behavior.
- Modify `backend/domain/skill/service/service.go`: domain service interface.
- Modify `backend/domain/skill/service/service_impl.go`: copy, publish, and snapshot mutation orchestration.
- Create `backend/domain/skill/service/file_mutation.go`: safe path normalization and immutable file-tree mutations.
- Create `backend/domain/skill/service/file_mutation_test.go`: path and mutation coverage.
- Modify `backend/application/skill/skill.go`: request mapping, authorization-compatible use cases, paging, files, copy, and publish.
- Modify `backend/application/skill/skill_test.go`: application coverage.
- Modify `backend/api/handler/coze/workbench_skill_service.go`: Hertz handlers.
- Modify `backend/api/handler/coze/workbench_skill_service_test.go`: handler security and response tests.
- Modify `backend/api/router/coze/workbench_skill_route_test.go`: generated route coverage.

### Frontend management page

- Modify `frontend/apps/coze-studio/src/pages/skill/service.ts`: generated client exports.
- Modify `frontend/apps/coze-studio/src/pages/skill/index.tsx`: management orchestration.
- Replace responsibilities in `frontend/apps/coze-studio/src/pages/skill/skill-page-hooks.ts`: query state and mutations.
- Modify `frontend/apps/coze-studio/src/pages/skill/skill-page-components.tsx`: searchable paged card surface.
- Create `frontend/apps/coze-studio/src/pages/skill/skill-create-menu.tsx`: AI/manual method selection.
- Create `frontend/apps/coze-studio/src/pages/skill/skill-create-modal.tsx`: Nuwax-aligned manual creation.
- Create `frontend/apps/coze-studio/src/pages/skill/skill-import-modal.tsx`: validated project import.
- Create `frontend/apps/coze-studio/src/pages/skill/skill-copy-modal.tsx`: target workspace selection.
- Create `frontend/apps/coze-studio/src/pages/skill/skill-page.less`: page-specific styles without growing the shared prototype stylesheet.
- Modify `frontend/apps/coze-studio/src/pages/skill/__tests__/skill.test.tsx`: management tests.

### Frontend detail workbench

- Create `frontend/apps/coze-studio/src/pages/skill-detail/index.tsx`: detail orchestration.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/skill-detail-header.tsx`: metadata, status, publish, and actions.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/skill-file-tree.tsx`: accessible file tree and entry operations.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/skill-file-editor.tsx`: text/binary editor state.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/skill-run-panel.tsx`: test-run input and result.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/skill-version-drawer.tsx`: version browse, export, and rollback.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/skill-detail-hooks.ts`: detail query and mutations.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/skill-file-model.ts`: pure tree construction and dirty-state helpers.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/index.less`: responsive three-pane layout.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/__tests__/skill-detail.test.tsx`: detail workflow tests.
- Create `frontend/apps/coze-studio/src/pages/skill-detail/__tests__/skill-file-model.test.ts`: pure file model tests.
- Modify `frontend/apps/coze-studio/src/routes/async-components.tsx`: lazy detail component.
- Modify `frontend/apps/coze-studio/src/routes/index.tsx`: standard and conversation detail routes.

## Task 1: Lock the extended Thrift contract

**Files:**

- Modify: `idl/workbench/skill.thrift`
- Create: `frontend/packages/arch/api-schema/__tests__/workbench-skill-management-contract.test.ts`
- Regenerate: `backend/api/model/workbench/skill/skill.go`
- Regenerate: `frontend/packages/arch/api-schema/src/idl/workbench/skill.ts`

- [ ] **Step 1: Write the failing source-contract test**

Add assertions that the IDL and generated client expose the new metadata, paging, draft, copy, and publish methods:

```ts
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

describe('workbench skill management contract', () => {
  const thrift = readFileSync(
    resolve(process.cwd(), '../../../../idl/workbench/skill.thrift'),
    'utf8',
  );

  it.each([
    'enum SkillPublishStatus',
    'struct SkillDraft',
    'struct CopySkillRequest',
    'struct PublishSkillRequest',
    'GetSkillDraft',
    'CreateSkillDraftEntry',
    'UpdateSkillDraftFile',
    'MoveSkillDraftEntry',
    'DeleteSkillDraftEntry',
    'UploadSkillDraftFiles',
    'CopySkill',
    'PublishSkill',
  ])('contains %s', token => {
    expect(thrift).toContain(token);
  });
});
```

- [ ] **Step 2: Run the contract test and confirm red state**

Run:

```bash
cd frontend/packages/arch/api-schema
rushx test -- __tests__/workbench-skill-management-contract.test.ts
```

Expected: FAIL because the listed contracts do not yet exist.

- [ ] **Step 3: Extend the source IDL**

Add the following public shapes while retaining all existing field IDs:

```thrift
enum SkillPublishStatus {
    Draft = 1,
    Published = 2,
    Changed = 3,
}

struct SkillDraftEntry {
    1: required string path
    2: required bool is_directory
    3: required i64 size
    4: optional string sha256
    5: optional string content_base64
}

struct SkillDraft {
    1: required i64 skill_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 version_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 revision
    4: required list<SkillDraftEntry> entries
}
```

Extend `Skill` with optional `icon_uri`, `usage_scenarios`,
`development_thread_id`, `published_version_id`, `published_at`, `revision`, and
`publish_status`. Extend `ListSkillsRequest` with `keyword`, `publish_status`,
`page`, and `page_size`; extend the response data with `total`, `page`, and
`page_size`.

Define explicit request/response structs for draft retrieval, file create,
file update, move, delete, batch upload, copy, and publish. All write requests
must carry `skill_id` and `expected_revision`; copy must carry
`target_space_id`; upload items must carry `path` and `content_base64`.

- [ ] **Step 4: Regenerate Go and TypeScript clients**

Run:

```bash
cd backend
hz update -idl ../idl/api.thrift -enable_extends

cd ../frontend/packages/arch/api-schema
rushx update
```

Expected: both commands exit `0`; generated models expose every new service
method. If generator-wide unrelated formatting changes occur, keep only files
derived from `idl/workbench/skill.thrift`.

- [ ] **Step 5: Run contract tests**

Run:

```bash
cd frontend/packages/arch/api-schema
rushx test -- __tests__/workbench-skill-management-contract.test.ts
```

Expected: PASS.

## Task 2: Add compatible persistence metadata and publications

**Files:**

- Create: `docker/atlas/migrations/20260713000100_skill_management_parity.sql`
- Modify: `backend/domain/skill/entity/skill.go`
- Modify: `backend/domain/skill/repository/mysql_test.go`

- [ ] **Step 1: Write failing repository migration expectations**

Add a sqlite-backed repository test that migrates the expanded persistence
models, creates a skill with metadata, and reads it back:

```go
func TestSkillRepositoryPersistsManagementMetadata(t *testing.T) {
    repo, db := newSkillRepositoryTestHarness(t)
    require.NoError(t, db.AutoMigrate(&skillPO{}, &skillVersionPO{}, &skillResourcePO{}, &skillPublicationPO{}))

    created, err := repo.Create(context.Background(), &entity.Skill{
        ID: 101, SpaceID: 7, Name: "research", Description: "Research helper",
        Type: entity.TypeCustomSkill, Version: "1.0.0", Enabled: true,
        IconURI: "asset://skill/101", UsageScenarios: []string{"task_agent"}, Revision: 1,
    })
    require.NoError(t, err)
    require.Equal(t, int64(1), created.Revision)
    require.Equal(t, []string{"task_agent"}, created.UsageScenarios)
}
```

- [ ] **Step 2: Run the focused repository test**

Run:

```bash
cd backend
go test ./domain/skill/repository -run TestSkillRepositoryPersistsManagementMetadata -count=1
```

Expected: FAIL because fields and publication persistence are absent.

- [ ] **Step 3: Add the Atlas migration**

Create a migration equivalent to:

```sql
ALTER TABLE `skills`
  ADD COLUMN `icon_uri` varchar(1024) NOT NULL DEFAULT '' AFTER `description`,
  ADD COLUMN `usage_scenarios` json NULL AFTER `icon_uri`,
  ADD COLUMN `development_thread_id` bigint NOT NULL DEFAULT 0 AFTER `usage_scenarios`,
  ADD COLUMN `published_version_id` bigint NOT NULL DEFAULT 0 AFTER `development_thread_id`,
  ADD COLUMN `published_at` bigint NOT NULL DEFAULT 0 AFTER `published_version_id`,
  ADD COLUMN `revision` bigint NOT NULL DEFAULT 1 AFTER `published_at`,
  ADD KEY `idx_skills_space_thread` (`space_id`, `development_thread_id`),
  ADD KEY `idx_skills_space_published` (`space_id`, `published_version_id`);

CREATE TABLE IF NOT EXISTS `skill_publications` (
  `id` bigint NOT NULL,
  `space_id` bigint NOT NULL,
  `skill_id` bigint NOT NULL,
  `version_id` bigint NOT NULL,
  `publisher_id` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_skill_publications_skill_created` (`skill_id`, `created_at`),
  UNIQUE KEY `uk_skill_publications_skill_version` (`skill_id`, `version_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

- [ ] **Step 4: Extend domain entities and repository POs**

Add explicit fields and an immutable publication entity:

```go
type Skill struct {
    ID, SpaceID int64
    Name, Description, IconURI string
    UsageScenarios []string
    DevelopmentThreadID, PublishedVersionID, PublishedAt, Revision int64
    // retain existing fields
}

type SkillPublication struct {
    ID, SpaceID, SkillID, VersionID, PublisherID, CreatedAt int64
}
```

Serialize `UsageScenarios` as JSON and default missing legacy values to an
empty slice and revision `1`.

- [ ] **Step 5: Validate migration and persistence**

Run:

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
cd backend && go test ./domain/skill/repository -run 'TestSkillRepository(PersistsManagementMetadata|CreateAndListVersions)' -count=1
```

Expected: Atlas reports a valid migration directory and tests PASS.

## Task 3: Implement paged and tenant-scoped skill listing

**Files:**

- Modify: `backend/domain/skill/repository/repository.go`
- Modify: `backend/domain/skill/repository/mysql.go`
- Modify: `backend/domain/skill/repository/mysql_test.go`
- Modify: `backend/domain/skill/service/service.go`
- Modify: `backend/domain/skill/service/service_impl.go`

- [ ] **Step 1: Write failing filter and paging tests**

Cover keyword, type, enabled, publish status, soft-delete exclusion, ordering,
and cross-space isolation:

```go
func TestSkillRepositorySearchSkillsIsTenantScoped(t *testing.T) {
    repo := seedSearchSkills(t)
    result, err := repo.Search(context.Background(), entity.SkillQuery{
        SpaceID: 7, Keyword: "research", Page: 1, PageSize: 20,
    })
    require.NoError(t, err)
    require.Equal(t, int64(1), result.Total)
    require.Equal(t, int64(7), result.Items[0].SpaceID)
}
```

- [ ] **Step 2: Run the focused repository tests**

Run:

```bash
cd backend
go test ./domain/skill/repository -run TestSkillRepositorySearchSkills -count=1
```

Expected: FAIL because `Search` and query types do not exist.

- [ ] **Step 3: Add query and page domain types**

```go
type PublishStatus string

const (
    PublishStatusDraft PublishStatus = "draft"
    PublishStatusPublished PublishStatus = "published"
    PublishStatusChanged PublishStatus = "changed"
)

type SkillQuery struct {
    SpaceID int64
    Keyword string
    Type *Type
    Enabled *bool
    PublishStatus *PublishStatus
    Page, PageSize int
}

type SkillPage struct {
    Items []*Skill
    Total int64
    Page, PageSize int
}
```

- [ ] **Step 4: Implement repository filtering**

Build one GORM query rooted at `space_id = ? AND deleted_at = 0`. Escape `%`,
`_`, and the escape character before a `LIKE` search. Clamp page size to
`1..100`, calculate total before `Limit/Offset`, and order by
`updated_at DESC, id DESC`. Compute publication filters from
`published_version_id` and the latest version ID without accepting a client
space override.

- [ ] **Step 5: Run repository tests**

Run:

```bash
cd backend
go test ./domain/skill/repository -run 'TestSkillRepository(SearchSkills|SoftDelete)' -count=1
```

Expected: PASS.

## Task 4: Add immutable draft file mutations

**Files:**

- Create: `backend/domain/skill/service/file_mutation.go`
- Create: `backend/domain/skill/service/file_mutation_test.go`
- Modify: `backend/domain/skill/service/service.go`
- Modify: `backend/domain/skill/service/service_impl.go`

- [ ] **Step 1: Write failing path and mutation tests**

```go
func TestNormalizeSkillPathRejectsTraversal(t *testing.T) {
    for _, value := range []string{"../secret", "/absolute", "a/../../b", "a\\..\\b"} {
        _, err := normalizeSkillPath(value)
        require.Error(t, err, value)
    }
}

func TestApplyFileMutationCreatesNewSnapshot(t *testing.T) {
    svc, repo := newSkillServiceHarness(t)
    result, err := svc.UpdateDraftFile(context.Background(), UpdateDraftFileCommand{
        SkillID: 9, ExpectedRevision: 3, Path: "references/prompt.md", Content: []byte("new"),
    })
    require.NoError(t, err)
    require.NotEqual(t, repo.BaseVersionID(), result.Version.ID)
    require.Equal(t, int64(4), result.Skill.Revision)
}
```

- [ ] **Step 2: Run draft mutation tests**

Run:

```bash
cd backend
go test ./domain/skill/service -run 'Test(NormalizeSkillPath|ApplyFileMutation)' -count=1
```

Expected: FAIL because the mutation service is absent.

- [ ] **Step 3: Implement safe immutable mutations**

Define commands for create, update, move, delete, and batch upload. Normalize
slashes, trim leading `./`, reject unsafe paths, reserve case-sensitive
`SKILL.md`, and enforce unique normalized paths. Load the latest version and
resources, compare `ExpectedRevision`, apply the mutation in memory, then write
a new `SkillVersion` and complete resource set in one repository transaction.

Use typed conflict errors:

```go
type RevisionConflictError struct {
    Expected int64
    Actual int64
}

func (e *RevisionConflictError) Error() string {
    return fmt.Sprintf("skill revision conflict: expected %d, actual %d", e.Expected, e.Actual)
}
```

- [ ] **Step 4: Enforce project limits**

Apply these server constants before writing:

```go
const (
    maxSkillFileBytes = 20 << 20
    maxSkillProjectBytes = 80 << 20
    maxSkillFileCount = 1000
    maxSkillPathBytes = 512
)
```

Reject symlink archive entries, duplicate paths, missing `SKILL.md`, binary
content sent to the text update endpoint, and deletion or renaming of
`SKILL.md`.

- [ ] **Step 5: Run domain tests**

Run:

```bash
cd backend
go test ./domain/skill/service -run 'Test(NormalizeSkillPath|ApplyFileMutation|SkillDraft)' -count=1
```

Expected: PASS.

## Task 5: Add copy and publish domain operations

**Files:**

- Modify: `backend/domain/skill/repository/repository.go`
- Modify: `backend/domain/skill/repository/mysql.go`
- Modify: `backend/domain/skill/service/service.go`
- Modify: `backend/domain/skill/service/service_impl.go`
- Modify: `backend/domain/skill/service/service_impl_test.go`

- [ ] **Step 1: Write failing copy and publish tests**

```go
func TestCopySkillCreatesIndependentSnapshot(t *testing.T) {
    svc := seededSkillService(t)
    copied, err := svc.Copy(context.Background(), CopyCommand{
        SkillID: 10, SourceSpaceID: 7, TargetSpaceID: 8,
    })
    require.NoError(t, err)
    require.NotEqual(t, int64(10), copied.ID)
    require.Equal(t, int64(8), copied.SpaceID)
    require.Zero(t, copied.PublishedVersionID)
}

func TestPublishSkillRecordsImmutableVersion(t *testing.T) {
    svc := seededSkillService(t)
    published, err := svc.Publish(context.Background(), PublishCommand{
        SkillID: 10, SpaceID: 7, PublisherID: 99, ExpectedRevision: 4,
    })
    require.NoError(t, err)
    require.Equal(t, published.VersionID, published.Skill.PublishedVersionID)
}
```

- [ ] **Step 2: Run focused domain tests**

Run:

```bash
cd backend
go test ./domain/skill/service -run 'Test(CopySkill|PublishSkill)' -count=1
```

Expected: FAIL.

- [ ] **Step 3: Implement transactional copy**

Within one transaction, allocate new skill/version/resource IDs, copy the
latest snapshot, reset publication and source-thread fields, and suffix the
name only when the target space already contains the same active name. Never
reuse mutable rows between spaces.

- [ ] **Step 4: Implement publication**

Validate the latest `SKILL.md`, resource paths, schemas, executor, permissions,
and revision. Insert one `SkillPublication` and update
`published_version_id/published_at` atomically. Re-publishing the same version
must be idempotent and return the existing publication.

- [ ] **Step 5: Run copy and publish tests**

Run:

```bash
cd backend
go test ./domain/skill/service ./domain/skill/repository -run 'Test(CopySkill|PublishSkill|SkillPublication)' -count=1
```

Expected: PASS.

## Task 6: Expose application use cases and secure HTTP handlers

**Files:**

- Modify: `backend/application/skill/skill.go`
- Modify: `backend/application/skill/skill_test.go`
- Modify: `backend/api/handler/coze/workbench_skill_service.go`
- Modify: `backend/api/handler/coze/workbench_skill_service_test.go`
- Modify: `backend/api/router/coze/workbench_skill_route_test.go`

- [ ] **Step 1: Write failing authorization-first tests**

Add handler tests proving unauthorized requests do not call mutation methods:

```go
func TestCopySkillHandlerRejectsTargetSpaceBeforeMutation(t *testing.T) {
    app := &recordingSkillApplication{}
    req := newAuthenticatedRequest(`{"skill_id":"10","target_space_id":"8"}`)
    app.TargetSpaceWritable = false
    response := invokeCopySkill(t, app, req)
    require.Equal(t, http.StatusForbidden, response.StatusCode())
    require.Zero(t, app.CopyCalls)
}
```

Cover missing viewer, foreign skill, foreign target space, stale revision,
invalid path, oversized upload, and redacted internal errors.

- [ ] **Step 2: Run focused handler tests**

Run:

```bash
cd backend
MOCKEY_CHECK_GCFLAGS=false go test ./api/handler/coze -run 'Test(GetSkillDraft|CreateSkillDraftEntry|UpdateSkillDraftFile|MoveSkillDraftEntry|DeleteSkillDraftEntry|UploadSkillDraftFiles|CopySkill|PublishSkill)Handler' -count=1
```

Expected: FAIL because handlers are absent.

- [ ] **Step 3: Map API requests to domain commands**

Application methods must resolve the authenticated actor, authorize the source
space before reading, authorize the target space before copy writes, decode
base64 with bounded readers, and map domain conflicts to a public conflict
error. Derive publish status as:

```go
func publishStatus(skill *entity.Skill, latestVersionID int64) skillapi.SkillPublishStatus {
    if skill.PublishedVersionID == 0 {
        return skillapi.SkillPublishStatus_Draft
    }
    if skill.PublishedVersionID == latestVersionID {
        return skillapi.SkillPublishStatus_Published
    }
    return skillapi.SkillPublishStatus_Changed
}
```

- [ ] **Step 4: Add handlers and route assertions**

Handlers bind generated request structs, call application methods, return
`400` for validation, `403` for authorization, `404` for foreign or missing
skills, `409` for revision conflicts, and `500` with a generic message for
internal failures. Route tests assert every new IDL method is registered under
`/api/workbench/skills`.

- [ ] **Step 5: Run application, handler, and route tests**

Run:

```bash
cd backend
go test ./application/skill -run 'TestApplication(ListSkills|GetSkillDraft|CreateSkillDraftEntry|UpdateSkillDraftFile|MoveSkillDraftEntry|DeleteSkillDraftEntry|UploadSkillDraftFiles|CopySkill|PublishSkill)' -count=1
MOCKEY_CHECK_GCFLAGS=false go test ./api/handler/coze -run 'Test(GetSkillDraft|CreateSkillDraftEntry|UpdateSkillDraftFile|MoveSkillDraftEntry|DeleteSkillDraftEntry|UploadSkillDraftFiles|CopySkill|PublishSkill)Handler' -count=1
go test ./api/router/coze -run TestRegisterIncludesWorkbenchSkill -count=1
```

Expected: PASS.

## Task 7: Preserve AI creation source linkage

**Files:**

- Modify: `backend/application/skill/skill.go`
- Modify: `backend/application/skill/skill_test.go`
- Modify: `backend/api/handler/coze/workbench_skill_service_test.go`

- [ ] **Step 1: Write the failing installation-link test**

```go
func TestApplicationInstallSkillRecordsDevelopmentThread(t *testing.T) {
    app, repo := newInstallSkillApplication(t)
    result, err := app.InstallSkillFromArtifact(context.Background(), &skillapi.InstallSkillFromArtifactRequest{
        SpaceID: 7, ThreadID: 88, ArtifactID: 99,
    })
    require.NoError(t, err)
    require.Equal(t, int64(88), repo.Skill(result.Data.Skill.ID).DevelopmentThreadID)
}
```

- [ ] **Step 2: Run the focused test**

Run:

```bash
cd backend
go test ./application/skill -run TestApplicationInstallSkillRecordsDevelopmentThread -count=1
```

Expected: FAIL.

- [ ] **Step 3: Persist the authorized source thread**

After existing thread/artifact/space authorization and successful import,
update only the imported skill's `DevelopmentThreadID`. The application must
not accept a viewer or creator ID from the request body.

- [ ] **Step 4: Keep duplicate install behavior unchanged**

Duplicate installation continues to return the existing domain duplicate
error. It must not overwrite the existing skill's source thread.

- [ ] **Step 5: Run install regressions**

Run:

```bash
cd backend
go test ./application/skill -run 'TestApplication(InstallSkillRecordsDevelopmentThread|ImportSkillUsesCustomDefaultType)' -count=1
MOCKEY_CHECK_GCFLAGS=false go test ./api/handler/coze -run TestInstallSkillFromArtifactHandler -count=1
```

Expected: PASS.

## Task 8: Build the management-page data layer

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/skill/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/skill/skill-page-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/skill/__tests__/skill.test.tsx`

- [ ] **Step 1: Write failing query-state tests**

Test that URL query parameters drive requests and a filter change resets the
page:

```tsx
it('loads paged skills from URL filters and resets page after search', async () => {
  renderSkillPage('/space/7/skill?keyword=report&page=3&status=changed');
  await waitFor(() =>
    expect(mockListSkills).toHaveBeenCalledWith(
      expect.objectContaining({ space_id: '7', keyword: 'report', page: 3 }),
    ),
  );
  await userEvent.clear(screen.getByRole('searchbox'));
  await userEvent.type(screen.getByRole('searchbox'), 'research');
  expect(currentLocation().search).toContain('page=1');
});
```

- [ ] **Step 2: Run the management test**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill/__tests__/skill.test.tsx -t 'loads paged skills'
```

Expected: FAIL.

- [ ] **Step 3: Export generated services**

Re-export `GetSkillDraft`, draft mutations, `CopySkill`, and `PublishSkill` from
`service.ts`; do not add hand-written fetch clients.

- [ ] **Step 4: Implement URL-backed list state**

Create a hook whose public shape is:

```ts
interface SkillListState {
  keyword: string;
  type?: workbenchSkill.SkillType;
  enabled?: boolean;
  publishStatus?: workbenchSkill.SkillPublishStatus;
  page: number;
  pageSize: number;
  total: number;
  loading: boolean;
  error: string;
  skills: workbenchSkill.Skill[];
  refresh: () => Promise<void>;
  setKeyword: (value: string) => void;
  setPublishStatus: (value?: workbenchSkill.SkillPublishStatus) => void;
  setPage: (value: number) => void;
}
```

Debounce keyword requests, cancel stale effects, and preserve current results
while refreshing.

- [ ] **Step 5: Run list-state tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill/__tests__/skill.test.tsx
```

Expected: PASS for existing and new list data tests.

## Task 9: Align list UI and both creation paths

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/skill/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill/skill-page-components.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill/skill-create-menu.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill/skill-create-modal.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill/skill-import-modal.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill/skill-copy-modal.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill/skill-page.less`
- Modify: `frontend/apps/coze-studio/src/pages/skill/__tests__/skill.test.tsx`

- [ ] **Step 1: Write failing interaction tests**

Cover the creation menu, AI route, manual modal, successful detail navigation,
import validation, copy refresh, and delete confirmation:

```tsx
it('offers AI and manual skill creation without an intermediate page', async () => {
  renderSkillPage('/space/7/skill');
  await userEvent.click(screen.getByRole('button', { name: '新建技能' }));
  expect(screen.getByRole('menuitem', { name: 'AI 辅助创建' })).toBeVisible();
  expect(screen.getByRole('menuitem', { name: '手动创建' })).toBeVisible();
  await userEvent.click(screen.getByRole('menuitem', { name: 'AI 辅助创建' }));
  expect(currentLocation().pathname).toBe('/space/7/chats/new');
  expect(currentLocation().search).toBe('?mode=skill');
});
```

- [ ] **Step 2: Run focused interaction tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill/__tests__/skill.test.tsx -t 'offers AI and manual skill creation'
```

Expected: FAIL.

- [ ] **Step 3: Implement creation and import surfaces**

Use Coze Design `Button`, `Dropdown`, `Modal`, `Form`, `Input`, `Select`,
`TextArea`, and `Upload`. Manual fields are icon, name, usage scenarios, and
description. Create success navigates to `/space/${spaceId}/skill/${skill.id}`.
Import accepts one `.skill`, `.zip`, or exact `SKILL.md`, applies the 20 MiB UI
limit, and navigates to the returned skill.

- [ ] **Step 4: Implement list cards and actions**

Render search, type/status filters, server pagination, loading skeletons, empty
state, refresh error, and cards containing icon, usage scenarios, enable state,
publish state, and update time. Copy opens a workspace selector; export uses the
existing bounded download helper; delete requires confirmation.

- [ ] **Step 5: Run management tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill/__tests__/skill.test.tsx
```

Expected: PASS.

## Task 10: Add detail routes and detail header

**Files:**

- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/skill-detail-header.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/skill-detail-hooks.ts`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/index.less`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/__tests__/skill-detail.test.tsx`

- [ ] **Step 1: Write failing route and header tests**

```tsx
it('loads a tenant-scoped skill detail and shows publish state', async () => {
  mockGetSkill.mockResolvedValue(skillResponse({ publish_status: 'Changed' }));
  renderSkillDetail('/space/7/skill/10');
  expect(await screen.findByRole('heading', { name: 'research' })).toBeVisible();
  expect(screen.getByText('有更新未发布')).toBeVisible();
  expect(mockGetSkill).toHaveBeenCalledWith({ skill_id: '10' });
});
```

- [ ] **Step 2: Run the detail test**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill-detail/__tests__/skill-detail.test.tsx
```

Expected: FAIL because the route and page do not exist.

- [ ] **Step 3: Add lazy routes**

Register exact routes for `/space/:space_id/skill/:skill_id` and
`/space/:space_id/skill/:skill_id/conversation`. Ensure the static `/skill`
route is declared before the parameterized route only if the current router
ranking requires it.

- [ ] **Step 4: Build the detail shell and header**

Load detail and draft in parallel, show bounded loading/error/not-found states,
and render back, metadata edit, enabled, publish state, version history,
test-run, publish, import, export, and delete actions. Read-only skills disable
mutations with a reason tooltip.

- [ ] **Step 5: Run detail route tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill-detail/__tests__/skill-detail.test.tsx
```

Expected: PASS for route and header cases.

## Task 11: Build the file tree and editor with dirty protection

**Files:**

- Create: `frontend/apps/coze-studio/src/pages/skill-detail/skill-file-model.ts`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/skill-file-tree.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/skill-file-editor.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/skill-detail-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/__tests__/skill-file-model.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/__tests__/skill-detail.test.tsx`

- [ ] **Step 1: Write failing pure-model tests**

```ts
it('builds deterministic nested file nodes', () => {
  expect(buildSkillFileTree([
    entry('SKILL.md'),
    entry('references/prompt.md'),
    directory('references'),
  ])).toMatchObject([
    { path: 'references', children: [{ path: 'references/prompt.md' }] },
    { path: 'SKILL.md' },
  ]);
});

it('protects SKILL.md from destructive actions', () => {
  expect(canDeleteSkillEntry('SKILL.md')).toBe(false);
  expect(canRenameSkillEntry('SKILL.md')).toBe(false);
});
```

- [ ] **Step 2: Run pure-model tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill-detail/__tests__/skill-file-model.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement the pure file model**

Export deterministic tree construction, parent-path expansion, text/binary
detection, language mapping, dirty comparison, and reserved-file checks. Do
not place API calls in this module.

- [ ] **Step 4: Implement tree, editor, and leave guard**

The tree supports selection, create file/folder, rename, delete, upload, and
refresh with keyboard focus preserved. The editor keeps server content and
draft content separately. On file switch or navigation while dirty, show
“保存并继续 / 放弃修改 / 取消”. Save sends the current expected revision; a
`409` keeps the draft and offers refresh instead of overwriting.

- [ ] **Step 5: Run file-workbench tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill-detail/__tests__/skill-file-model.test.ts src/pages/skill-detail/__tests__/skill-detail.test.tsx
```

Expected: PASS.

## Task 12: Integrate versions, test run, and publishing

**Files:**

- Create: `frontend/apps/coze-studio/src/pages/skill-detail/skill-run-panel.tsx`
- Create: `frontend/apps/coze-studio/src/pages/skill-detail/skill-version-drawer.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/skill-detail-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/__tests__/skill-detail.test.tsx`
- Retire page use of: `frontend/apps/coze-studio/src/pages/skill/skill-version-panel.tsx`

- [ ] **Step 1: Write failing delivery-flow tests**

```tsx
it('publishes the saved revision and refreshes status', async () => {
  renderSkillDetail('/space/7/skill/10');
  await userEvent.click(await screen.findByRole('button', { name: '发布' }));
  await userEvent.click(screen.getByRole('button', { name: '确认发布' }));
  expect(mockPublishSkill).toHaveBeenCalledWith({
    skill_id: '10',
    expected_revision: 4,
  });
  expect(await screen.findByText('已发布')).toBeVisible();
});
```

Cover unsaved-run warning, run loading/result/error, version selection,
read-only historical content, export, and rollback creating a new latest
snapshot.

- [ ] **Step 2: Run delivery-flow tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill-detail/__tests__/skill-detail.test.tsx -t 'publishes the saved revision'
```

Expected: FAIL.

- [ ] **Step 3: Build the run panel**

Submit only the server-saved draft, disable duplicate runs, display safe output
or task ID, and preserve input after failure. If the editor is dirty, disable
run and explain that the file must be saved first.

- [ ] **Step 4: Build version and publish flows**

Reuse generated version APIs in a dedicated drawer. Historical content is
read-only; export downloads the selected snapshot; rollback confirms and then
refreshes detail/draft/revision. Publish confirms the saved revision and
refreshes list-compatible status.

- [ ] **Step 5: Run detail delivery tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill-detail/__tests__/skill-detail.test.tsx src/pages/skill/__tests__/skill-version-panel.test.tsx
```

Expected: PASS. Existing version-panel regression tests remain green until the
old component is safely removed in a later cleanup.

## Task 13: Add conversation-linked skill development

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/skill/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/skill-detail-hooks.ts`
- Modify: `frontend/apps/coze-studio/src/pages/skill-detail/__tests__/skill-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill/__tests__/skill.test.tsx`

- [ ] **Step 1: Write failing navigation and fallback tests**

```tsx
it('opens AI-created skills in conversation detail', async () => {
  renderSkillPageWith(skill({ id: '10', development_thread_id: '88' }));
  await userEvent.click(screen.getByRole('button', { name: /research/ }));
  expect(currentLocation().pathname).toBe('/space/7/skill/10/conversation');
  expect(currentLocation().search).toBe('?thread_id=88');
});
```

Also verify a missing, foreign, or inaccessible thread falls back to standard
detail without exposing thread data.

- [ ] **Step 2: Run focused navigation tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill/__tests__/skill.test.tsx src/pages/skill-detail/__tests__/skill-detail.test.tsx -t 'conversation'
```

Expected: FAIL.

- [ ] **Step 3: Implement conversation detail composition**

Reuse the existing task-detail/workbench components through their public props;
do not duplicate task streaming or event rendering. Compose the conversation
surface beside the same skill file workbench and expose a “文件开发 / AI 开发”
mode switch.

- [ ] **Step 4: Implement safe fallback**

Treat thread `403` and `404` as unavailable source context, remove the invalid
query from the visible route, and retain skill detail access if the skill itself
is authorized. Other errors use the standard bounded error state.

- [ ] **Step 5: Run conversation tests**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill/__tests__/skill.test.tsx src/pages/skill-detail/__tests__/skill-detail.test.tsx
```

Expected: PASS.

## Task 14: Complete production verification and browser acceptance

**Files:**

- Modify: `docs/superpowers/plans/2026-07-13-nuwax-skill-management-parity-implementation.md` with checked steps and evidence only after each command actually passes.
- Create: `docs/superpowers/evidence/2026-07-13-nuwax-skill-management-parity/notes/acceptance.md`
- Create screenshots under: `docs/superpowers/evidence/2026-07-13-nuwax-skill-management-parity/screenshots/`

- [ ] **Step 1: Run backend focused tests**

Run:

```bash
cd backend
go test ./domain/skill/... ./application/skill -count=1
MOCKEY_CHECK_GCFLAGS=false go test ./api/handler/coze -run 'Test.*Skill' -count=1
go test ./api/router/coze -run 'TestRegisterIncludesWorkbenchSkill' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run frontend focused tests and typecheck**

Run:

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/skill/__tests__ src/pages/skill-detail/__tests__
npx tsc --noEmit --project tsconfig.json
```

Expected: PASS with no new type errors.

- [ ] **Step 3: Validate Atlas**

Run:

```bash
atlas version
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

Expected: Atlas `v0.35.0`; hash and validate succeed.

- [ ] **Step 4: Compare Nuwax and Coze in the in-app browser**

Use Nuwax `http://localhost/` with `admin@nuwax.com` and Coze
`http://localhost:8080` with the repository test account. Record matching
results for list filters, both creation paths, import, file CRUD, upload,
unsaved protection, version history, test run, publish, rollback, export, copy,
delete, read-only state, and permission failures. Record console errors and
exact URLs.

- [ ] **Step 5: Record final evidence**

The acceptance note must state every command result, tested space and skill IDs,
successful and rejected permission paths, screenshots, remaining differences,
and whether any scope item is blocked. Do not mark the plan complete while any
in-scope flow is unverified.

## Plan self-review

- Spec coverage: tasks cover management list, both creation paths, import,
  metadata, project files, optimistic concurrency, versions, test run,
  publication, copy, export, delete, AI source linkage, permissions, migration,
  tests, and in-app browser acceptance.
- Scope boundary: public marketplace discovery, independent admin review,
  billing, subscriptions, and unrelated resource systems remain excluded.
- Type consistency: public contracts use `expected_revision`,
  `development_thread_id`, `published_version_id`, `publish_status`, and
  `SkillDraftEntry` consistently across backend and frontend tasks.
- Generated code: `idl/workbench/skill.thrift` remains the source of truth;
  generated Go and TypeScript clients are regenerated rather than hand-edited.
- Git: no plan step performs Git operations without a new explicit user
  authorization.
