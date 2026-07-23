# Workspace Model Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Nuwax-equivalent system and workspace model configuration to the existing account settings modal, with workspace authorization, encrypted credentials, runtime visibility, and complete management interactions.

**Architecture:** Keep `model_instance` as the model source of truth and use `model_instance_endpoint` plus `model_instance_grant` for secure endpoints and workspace scope. A generated Workbench Model API exposes sanitized DTOs; the application service performs membership and manager checks before delegating persistence. The existing task model selector continues consuming `GetTypeList`, which becomes workspace-aware and returns the union of system models and models granted to the requested workspace.

**Tech Stack:** Go, Hertz, GORM Gen, Thrift/Hz, React, TypeScript, Semi/Coze Design, Vitest, Atlas MySQL migrations.

---

## File map

- `idl/workbench/model.thrift`: generated client/server contract for model settings.
- `idl/api.thrift`: registers `WorkbenchModelService`.
- `backend/bizpkg/config/modelmgr/workspace_store.go`: atomic persistence, endpoint encryption, grants, and sanitized projections.
- `backend/bizpkg/config/modelmgr/modelmgr.go`: wires the credential codec into model configuration.
- `backend/bizpkg/config/modelmgr/model_get.go`: hydrates encrypted endpoint configuration for runtime execution.
- `backend/application/modelmgr/workspace_model.go`: authorization-aware workspace model use cases.
- `backend/application/modelmgr/modelmgr.go`: workspace-filtered model selection projection.
- `backend/api/handler/coze/workspace_model_service.go`: generated-contract HTTP adapter.
- `frontend/apps/coze-studio/src/pages/tools/model-settings-service.ts`: generated API aliases and DTO mapping.
- `frontend/apps/coze-studio/src/pages/tools/model-settings-panel.tsx`: system/workspace model UI.
- `frontend/apps/coze-studio/src/pages/tools/model-settings-panel.module.less`: modal-scoped styles.
- `frontend/apps/coze-studio/src/components/workspace-account-dropdown.tsx`: adds the settings tab.
- `frontend/apps/coze-studio/src/pages/workbench/service.ts`: continues passing `space_id` and consumes workspace-filtered models.

### Task 1: Define and generate the Workbench Model contract

**Files:**
- Create: `frontend/packages/arch/api-schema/__tests__/workbench-model-contract.test.ts`
- Modify: `frontend/packages/arch/api-schema/package.json`
- Create: `idl/workbench/model.thrift`
- Modify: `idl/api.thrift`
- Generate: `frontend/packages/arch/api-schema/src/idl/workbench/model.ts`
- Generate: `backend/api/model/workbench/model/model.go`
- Generate: `backend/api/router/coze/api.go`

- [ ] **Step 1: Write the failing schema contract test**

```ts
import * as model from '../src/idl/workbench/model';

it('exports workspace model management APIs', () => {
  expect(typeof model.ListWorkspaceModels).toBe('function');
  expect(typeof model.GetWorkspaceModel).toBe('function');
  expect(typeof model.UpsertWorkspaceModel).toBe('function');
  expect(typeof model.TestWorkspaceModel).toBe('function');
  expect(typeof model.SetWorkspaceModelStatus).toBe('function');
  expect(typeof model.DeleteWorkspaceModel).toBe('function');
});
```

- [ ] **Step 2: Run the contract test and verify RED**

Run: `cd frontend/packages/arch/api-schema && npm test -- __tests__/workbench-model-contract.test.ts`

Expected: FAIL because `src/idl/workbench/model.ts` does not exist.

- [ ] **Step 3: Add the Thrift contract**

Define these stable DTOs in `idl/workbench/model.thrift`:

```thrift
enum WorkspaceModelScope { System = 1, Space = 2 }

struct WorkspaceModelEndpointInput {
  1: optional i64 id (agw.js_conv="str", api.js_conv="true")
  2: required string base_url
  3: optional string api_key
  4: optional i32 weight = 1
  5: optional bool enabled = true
}

struct WorkspaceModel {
  1: required i64 id (agw.js_conv="str", api.js_conv="true")
  2: required WorkspaceModelScope scope
  3: required string provider_key
  4: required string display_name
  5: required string model_identifier
  6: required bool enabled
  7: required bool credential_configured
  8: required bool can_manage
  9: optional string description
  10: optional string protocol
  11: optional list<string> capabilities
  12: optional list<string> usage_scenarios
  13: optional i64 max_context_tokens
  14: optional i64 max_output_tokens
  15: optional string function_call_mode
  16: optional list<WorkspaceModelEndpointView> endpoints
}
```

Expose list/detail/upsert/test/status/delete methods under `/api/workbench/models`. Every request that accesses workspace data contains `space_id`; path IDs remain string-converted `i64`.

- [ ] **Step 4: Register and generate the clients**

Add `include "./workbench/model.thrift"` and:

```thrift
service WorkbenchModelService extends model.WorkbenchModelService {}
```

Run:

```bash
cd frontend/packages/arch/api-schema && npm run update
cd backend && PATH=/Users/liuwenbo/go/bin:$PATH hz update -idl ../idl/api.thrift -enable_extends
```

Expose the generated frontend module through the stable subpath:

```json
"./workbench-model": "./src/idl/workbench/model.ts"
```

and add the matching `typesVersions` entry for `workbench-model`.

- [ ] **Step 5: Re-run the schema test and verify GREEN**

Expected: `workbench-model-contract.test.ts` passes and package export `./workbench-model` resolves.

### Task 2: Add encrypted endpoint and workspace-grant persistence

**Files:**
- Create: `backend/bizpkg/config/modelmgr/workspace_store.go`
- Create: `backend/bizpkg/config/modelmgr/workspace_store_test.go`
- Modify: `backend/bizpkg/config/modelmgr/modelmgr.go`
- Modify: `backend/bizpkg/config/modelmgr/model_get.go`
- Modify: `docker/atlas/migrations/20260722000100_system_model_management.sql`
- Modify: `backend/bizpkg/config/modelmgr/persistence_contract_test.go`
- Generate: `backend/bizpkg/config/modelmgr/internal/model/model_instance_endpoint.gen.go`
- Generate: `backend/bizpkg/config/modelmgr/internal/model/model_instance_grant.gen.go`
- Generate: `backend/bizpkg/config/modelmgr/internal/query/model_instance_endpoint.gen.go`
- Generate: `backend/bizpkg/config/modelmgr/internal/query/model_instance_grant.gen.go`

- [ ] **Step 1: Write failing store tests**

Cover these behaviors with a transactional test database or repository fakes:

```go
func TestUpsertWorkspaceModelEncryptsCredentialAndCreatesGrant(t *testing.T)
func TestUpdateWorkspaceModelKeepsCredentialWhenAPIKeyIsBlank(t *testing.T)
func TestListWorkspaceModelsNeverReturnsCredentialEnvelope(t *testing.T)
func TestDeleteWorkspaceModelRejectsForeignWorkspace(t *testing.T)
func TestGetRuntimeModelDecryptsManagedEndpoint(t *testing.T)
```

Assert that the persisted `api_key_envelope` is non-empty, does not contain plaintext, and the returned DTO only contains `CredentialConfigured=true`.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd backend && go test ./bizpkg/config/modelmgr -run 'Test.*WorkspaceModel|TestGetRuntimeModel' -count=1`

Expected: FAIL because workspace persistence methods do not exist.

- [ ] **Step 3: Generate ORM models for the existing migration**

First make both endpoint and grant primary keys `AUTO_INCREMENT`; this lets the transaction obtain the endpoint ID before encrypting its credential without adding a second ID generator to `ModelConfig`. Use the repository's ORM generator so endpoint and grant query files match:

```sql
model_instance_endpoint(model_id, base_url, api_key_envelope, api_key_fingerprint, weight, enabled, sort_order)
model_instance_grant(model_id, subject_type, subject_id)
```

Do not hand-edit generated files.

- [ ] **Step 4: Wire the credential codec**

Extend `ModelConfig` with:

```go
credentialCodec ModelCredentialCodec
```

Load `MODEL_CREDENTIAL_KEYS_JSON` and `MODEL_CREDENTIAL_ACTIVE_KEY_ID` during application initialization. Managed model writes fail closed with `ErrModelCredentialCodecMissing`; legacy reads remain available.

- [ ] **Step 5: Implement atomic workspace persistence**

Add explicit inputs and sanitized outputs:

```go
type WorkspaceModelWrite struct {
    ID, SpaceID, CreatorID int64
    ProviderKey, DisplayName, ModelIdentifier string
    Description, Protocol string
    Enabled bool
    Endpoints []WorkspaceModelEndpointWrite
}

func (c *ModelConfig) UpsertWorkspaceModel(ctx context.Context, in WorkspaceModelWrite) (*WorkspaceModelView, error)
func (c *ModelConfig) ListWorkspaceModels(ctx context.Context, spaceID int64) ([]*WorkspaceModelView, error)
func (c *ModelConfig) GetWorkspaceModel(ctx context.Context, spaceID, modelID int64) (*WorkspaceModelView, error)
func (c *ModelConfig) SetWorkspaceModelStatus(ctx context.Context, spaceID, modelID int64, enabled bool) error
func (c *ModelConfig) DeleteWorkspaceModel(ctx context.Context, spaceID, modelID int64) error
```

Use one transaction for model, endpoints, and the `SPACE` grant. On blank API key during update, retain the existing envelope. Scope every mutation through the matching active workspace grant.

- [ ] **Step 6: Hydrate runtime connections**

When an enabled managed endpoint exists, decrypt its envelope and build the existing `config.Connection.BaseConnInfo`. Never place the plaintext into logs or management projections.

- [ ] **Step 7: Re-run store tests and verify GREEN**

Expected: all workspace persistence, isolation, and credential tests pass.

### Task 3: Add authorization-aware application use cases

**Files:**
- Create: `backend/application/modelmgr/workspace_model.go`
- Create: `backend/application/modelmgr/workspace_model_test.go`
- Modify: `backend/application/modelmgr/init.go`

- [ ] **Step 1: Write failing permission tests**

```go
func TestWorkspaceModelListAllowsMemberRead(t *testing.T)
func TestWorkspaceModelMutationAllowsOwnerAndAdmin(t *testing.T)
func TestWorkspaceModelMutationRejectsMember(t *testing.T)
func TestWorkspaceModelRequestRejectsNonMember(t *testing.T)
func TestWorkspaceModelTestDoesNotPersistDraftCredential(t *testing.T)
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd backend && go test ./application/modelmgr -run TestWorkspaceModel -count=1`

- [ ] **Step 3: Implement the service boundary**

```go
type WorkspaceAccessChecker interface {
    CheckWorkspaceMembership(context.Context, *workspace.CheckWorkspaceMembershipRequest) (*workspace.WorkspaceDetail, error)
    CheckWorkspaceAppDevAccess(context.Context, *workspace.CheckWorkspaceAppDevAccessRequest) (*workspace.WorkspaceDetail, error)
}

type WorkspaceModelService struct {
    Access WorkspaceAccessChecker
    Store  WorkspaceModelStore
}
```

Reads call membership checks. Writes and connection tests call manager access with `RequireManager=true`. The service accepts actor ID and space ID as separate values and never substitutes request-provided actor data.

- [ ] **Step 4: Implement connection testing**

Build the existing provider-specific model builder from draft input, execute a bounded `1+1=?` request with timeout, and return only duration plus normalized success/error code. Do not log request credentials or provider response bodies.

- [ ] **Step 5: Re-run permission tests and verify GREEN**

Expected: owner/admin/member/non-member paths pass.

### Task 4: Expose the Workbench Model HTTP API

**Files:**
- Create: `backend/api/handler/coze/workspace_model_service.go`
- Create: `backend/api/handler/coze/workspace_model_service_test.go`
- Modify/Generate: `backend/api/router/coze/api.go`

- [ ] **Step 1: Write failing handler tests**

Register the generated routes and assert:

```go
require.Equal(t, 401, unauthenticated.Code)
require.Equal(t, 403, memberMutation.Code)
require.NotContains(t, listBody, "api_key_envelope")
require.NotContains(t, detailBody, "api_key")
require.Contains(t, ownerCreateBody, `"credential_configured":true`)
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd backend && go test ./api/handler/coze -run TestWorkspaceModel -count=1`

- [ ] **Step 3: Implement handlers**

Each handler obtains `currentUserID := ctxutil.GetUIDFromCtx(ctx)`, binds generated requests, calls `WorkspaceModelApplicationSVC`, maps domain errors to `400/403/404/409/502`, and returns generated response objects.

- [ ] **Step 4: Re-run handler tests and verify GREEN**

Expected: HTTP authorization, validation, and redaction cases pass.

### Task 5: Make model selection workspace-aware

**Files:**
- Modify: `backend/application/modelmgr/modelmgr.go`
- Create: `backend/application/modelmgr/model_visibility_test.go`
- Modify: `backend/api/handler/coze/developer_api_service.go`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`

- [ ] **Step 1: Write failing visibility tests**

```go
func TestGetModelListReturnsSystemAndGrantedWorkspaceModels(t *testing.T)
func TestGetModelListExcludesOtherWorkspaceAndDisabledModels(t *testing.T)
func TestGetModelListRejectsWorkspaceForNonMember(t *testing.T)
```

Add a frontend assertion that `DeveloperApi.GetTypeList` receives the active `space_id` and renders a workspace model by its distinct model instance ID.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
cd backend && go test ./application/modelmgr -run TestGetModelList -count=1
cd frontend/apps/coze-studio && npm run test -- src/pages/workbench/__tests__/workbench.test.tsx
```

- [ ] **Step 3: Implement visibility filtering**

Use `req.SpaceID` and the authenticated user to return enabled system models plus enabled models with an active `SPACE` grant for that workspace. Preserve model instance IDs; do not deduplicate on model name.

- [ ] **Step 4: Re-run tests and verify GREEN**

Expected: cross-space models are absent and the selected workspace model ID reaches the task runtime unchanged.

### Task 6: Build the settings model panel

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/tools/model-settings-service.ts`
- Create: `frontend/apps/coze-studio/src/pages/tools/model-settings-panel.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tools/model-settings-panel.module.less`
- Create: `frontend/apps/coze-studio/src/pages/tools/__tests__/model-settings-panel.test.tsx`

- [ ] **Step 1: Write failing UI tests**

Cover:

```tsx
it('shows system and workspace model tabs');
it('keeps system models read only');
it('allows owner to test and save a workspace model');
it('keeps member controls read only');
it('does not render returned credential material');
it('clears old workspace data when spaceId changes');
it('keeps loaded rows visible when refresh fails');
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd frontend/apps/coze-studio && npm run test -- src/pages/tools/__tests__/model-settings-panel.test.tsx`

- [ ] **Step 3: Add generated API aliases**

Import from the stable package subpath to avoid root-export regressions:

```ts
import * as workbenchModel from '@coze-studio/api-schema/workbench-model';

export const listWorkspaceModels = workbenchModel.ListWorkspaceModels;
export const getWorkspaceModel = workbenchModel.GetWorkspaceModel;
export const upsertWorkspaceModel = workbenchModel.UpsertWorkspaceModel;
export const testWorkspaceModel = workbenchModel.TestWorkspaceModel;
export const setWorkspaceModelStatus = workbenchModel.SetWorkspaceModelStatus;
export const deleteWorkspaceModel = workbenchModel.DeleteWorkspaceModel;
```

- [ ] **Step 4: Implement the panel states**

Use Coze Design/Semi components for tabs, search, cards/table, form modal, switch, confirmation, skeleton, empty state, inline error, and retry. The system tab has no mutation controls. Workspace controls render only when `can_manage` is true.

- [ ] **Step 5: Implement the Nuwax-equivalent form**

Fields: provider, display name, model identifier, protocol, Base URL, API Key, capabilities, function-call mode, max context/output tokens, usage scenarios, and enabled status. API Key is never initialized from a response; edit helper text says leaving it blank keeps the existing key.

- [ ] **Step 6: Re-run UI tests and verify GREEN**

Expected: all loading, empty, error, read-only, edit, test, and delete states pass.

### Task 7: Integrate the settings tab

**Files:**
- Modify: `frontend/apps/coze-studio/src/components/workspace-account-dropdown.tsx`
- Modify: `frontend/apps/coze-studio/src/components/__tests__/workspace-header-actions.test.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`

- [ ] **Step 1: Add failing integration assertions**

Assert settings order is `账号 / API 授权 / 模型配置 / MCP 配置 / IM 机器人`, and the panel receives the current space ID.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/__tests__/workspace-header-actions.test.tsx src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

- [ ] **Step 3: Add the tab**

Insert:

```tsx
{
  id: MODEL_SETTINGS_TAB_ID,
  tabName: '模型配置',
  content: () => <ModelSettingsPanel spaceId={currentSpace?.id} />,
}
```

before MCP configuration.

- [ ] **Step 4: Re-run integration tests and verify GREEN**

Expected: menu ordering, opening behavior, and current-space propagation pass.

### Task 8: Production verification and documentation

**Files:**
- Modify: `docs/superpowers/plans/2026-07-22-workspace-model-configuration.md`
- Modify only if new operational variables are required: `docs/superpowers/runbooks/local-debug-and-test.md`

- [ ] **Step 1: Run targeted backend tests**

```bash
cd backend
go test ./bizpkg/config/modelmgr ./application/modelmgr ./api/handler/coze -run 'Test.*WorkspaceModel|TestGetModelList' -count=1
```

- [ ] **Step 2: Validate schema and generated contracts**

```bash
cd docker/atlas
atlas migrate hash
atlas migrate validate --dir file://migrations
cd ../../frontend/packages/arch/api-schema
npm test -- __tests__/workbench-model-contract.test.ts
```

- [ ] **Step 3: Run targeted frontend tests**

```bash
cd frontend/apps/coze-studio
npm run test -- \
  src/pages/tools/__tests__/model-settings-panel.test.tsx \
  src/components/__tests__/workspace-header-actions.test.tsx \
  src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx \
  src/pages/workbench/__tests__/workbench.test.tsx
```

- [ ] **Step 4: Validate with the Codex in-app browser**

Use `http://localhost:8080/space/7645565700475453440/chats` and local account `840582614@qq.com`:

1. Open account settings and confirm tab order and consistent modal layout.
2. Verify system models are read only.
3. Create a workspace model, test connectivity, save, edit without replacing the key, disable, enable, and delete.
4. Switch personal/team spaces and verify strict list isolation.
5. Open the task model selector and run a minimal message with the workspace model.
6. Verify member write requests return `403` and no browser console errors occur.

- [ ] **Step 5: Record evidence**

Mark completed checklist items in this plan and record tested URLs, workspace IDs, roles, commands, and any external-provider blocker. Do not claim production completion when the credential keyring or a real provider is unavailable.
