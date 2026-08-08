# Model Connectivity and Legacy Admin Retirement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `/system/models` accurately test the configured model, preserve every runtime-relevant option from the legacy model editor, and then remove only the obsolete model-management UI from `backend/conf/admin/index.html`.

**Architecture:** Extend the admin model contract with a secret-free provider-options DTO, project those options to and from the existing `connection` JSON, and keep endpoint credentials in the existing encrypted endpoint table. OpenAI-compatible connectivity tests will send a one-token non-streaming chat-completions request. The legacy HTML model UI is removed only after backend and React compatibility tests pass; compatibility APIs used by knowledge configuration remain.

**Tech Stack:** Go 1.x, Hertz/Thrift, GORM, React 18, TypeScript, Vitest, `httptest`, Rush.js.

---

## File map

- `idl/admin/config.thrift`: public provider-options fields for model create/update/detail.
- `frontend/packages/arch/api-schema/__tests__/admin-model-management-contract.test.ts`: secret-safety and provider-options contract assertions.
- Generated Go/TypeScript API files: regenerated from the Thrift source; never hand-edited.
- `backend/bizpkg/config/modelmgr/system_store.go`: provider option projection/validation and real connectivity probe.
- `backend/bizpkg/config/modelmgr/system_store_test.go`: persisted provider-option compatibility tests.
- `backend/bizpkg/config/modelmgr/system_connectivity_test.go`: isolated HTTP probe behavior and error mapping tests.
- `frontend/apps/coze-studio/src/pages/system/service.ts`: local view types and request serialization.
- `frontend/apps/coze-studio/src/pages/system/model-config-dialog.tsx`: provider-driven fields and validation.
- `frontend/apps/coze-studio/src/pages/system/__tests__/model-config-section.test.tsx`: form rendering, edit hydration, and save preservation tests.
- `frontend/apps/coze-studio/src/pages/system/__tests__/system-service.test.ts`: request payload coverage.
- `backend/conf/admin/index.html`: remove only the obsolete model-management page.
- `backend/conf/admin/index_test.go`: static retirement guard that preserves knowledge model lookup.
- `README.md`, `README.zh_CN.md`: point model setup to `/system/models`.

### Task 1: Lock the approved contract and design

**Files:**

- Modify: `idl/admin/config.thrift`
- Modify: `frontend/packages/arch/api-schema/__tests__/admin-model-management-contract.test.ts`
- Modify: `docs/superpowers/specs/2026-08-08-model-connectivity-runtime-probe-design.md`
- Create: `docs/superpowers/plans/2026-08-08-model-connectivity-and-legacy-admin-retirement.md`

- [ ] **Step 1: Add the failing provider-options contract test**

Append this test to `admin-model-management-contract.test.ts`:

```ts
it('exposes only the legacy runtime provider options that are safe to round trip', () => {
  const source = readContract();
  const options = source.match(
    /struct\s+ModelProviderOptions\s*\{(?<body>[\s\S]*?)\n\s*\}/,
  )?.groups?.body;

  expect(options).toBeDefined();
  expect(options).toContain('ark_region');
  expect(options).toContain('openai_by_azure');
  expect(options).toContain('openai_api_version');
  expect(options).toContain('gemini_backend');
  expect(options).toContain('gemini_project');
  expect(options).toContain('gemini_location');
  expect(options).not.toMatch(/api_key|secret|access_key|header/i);
  expect(source).toMatch(
    /struct\s+ModelManagementInput[\s\S]*optional\s+ModelProviderOptions\s+provider_options/,
  );
  expect(source).toMatch(
    /struct\s+ModelDetail[\s\S]*optional\s+ModelProviderOptions\s+provider_options/,
  );
});
```

- [ ] **Step 2: Run the contract test and verify it fails**

Run:

```bash
cd frontend/packages/arch/api-schema
npm run test -- admin-model-management-contract.test.ts
```

Expected: FAIL because `ModelProviderOptions` and `provider_options` do not exist.

- [ ] **Step 3: Add the narrow secret-free Thrift contract**

Add before `ModelManagementInput` and append field `17`/`11` without renumbering existing fields:

```thrift
struct ModelProviderOptions {
    1: optional string ark_region
    2: optional bool openai_by_azure
    3: optional string openai_api_version
    4: optional i32 gemini_backend
    5: optional string gemini_project
    6: optional string gemini_location
}

struct ModelManagementInput {
    // existing fields 1-16 remain unchanged
    17: optional ModelProviderOptions provider_options
}

struct ModelDetail {
    // existing fields 1-10 remain unchanged
    11: optional ModelProviderOptions provider_options
}
```

- [ ] **Step 4: Regenerate and verify API code**

Run:

```bash
cd backend
hz update -idl ../idl/api.thrift -enable_extends
cd ../frontend/packages/arch/api-schema
npm run update
cd ../../../..
bash backend/scripts/verify_api_codegen.sh
```

Expected: generation succeeds, the verification script reports deterministic generated output, and no handwritten handler is replaced.

- [ ] **Step 5: Re-run the contract test**

Run:

```bash
cd frontend/packages/arch/api-schema
npm run test -- admin-model-management-contract.test.ts
```

Expected: PASS.

- [ ] **Step 6: Commit the approved contract and planning documents**

```bash
git add idl/admin/config.thrift backend/api/model/admin/config frontend/packages/arch/api-schema/src/idl/admin/config.ts frontend/packages/arch/api-schema/__tests__/admin-model-management-contract.test.ts docs/superpowers/specs/2026-08-08-model-connectivity-runtime-probe-design.md docs/superpowers/plans/2026-08-08-model-connectivity-and-legacy-admin-retirement.md
git commit -m "feat: preserve managed model provider options"
```

### Task 2: Preserve provider-specific runtime options

**Files:**

- Modify: `backend/bizpkg/config/modelmgr/system_store.go`
- Modify: `backend/bizpkg/config/modelmgr/system_store_test.go`

- [ ] **Step 1: Write failing persistence and legacy-round-trip tests**

Add table-driven cases covering Ark, OpenAI Azure, and Gemini Vertex. Each case must create a model, read detail, update the model using the returned options, and inspect the stored `connection` JSON:

```go
func TestSystemModelProviderOptionsRoundTrip(t *testing.T) {
    tests := []struct {
        name        string
        providerKey string
        modelClass  developer_api.ModelClass
        options     *config.ModelProviderOptions
        assertConn  func(*testing.T, *config.Connection)
    }{
        {
            name: "ark region", providerKey: "doubao", modelClass: developer_api.ModelClass_SEED,
            options: &config.ModelProviderOptions{ArkRegion: ptr.Of("cn-beijing")},
            assertConn: func(t *testing.T, conn *config.Connection) {
                require.Equal(t, "cn-beijing", conn.Ark.Region)
            },
        },
        {
            name: "openai azure", providerKey: "openai", modelClass: developer_api.ModelClass_GPT,
            options: &config.ModelProviderOptions{
                OpenaiByAzure: ptr.Of(true), OpenaiAPIVersion: ptr.Of("2024-06-01"),
            },
            assertConn: func(t *testing.T, conn *config.Connection) {
                require.True(t, conn.Openai.ByAzure)
                require.Equal(t, "2024-06-01", conn.Openai.APIVersion)
            },
        },
        {
            name: "gemini vertex", providerKey: "gemini", modelClass: developer_api.ModelClass_Gemini,
            options: &config.ModelProviderOptions{
                GeminiBackend: ptr.Of(int32(2)), GeminiProject: ptr.Of("project-a"),
                GeminiLocation: ptr.Of("us-central1"),
            },
            assertConn: func(t *testing.T, conn *config.Connection) {
                require.Equal(t, int32(2), conn.Gemini.Backend)
                require.Equal(t, "project-a", conn.Gemini.Project)
                require.Equal(t, "us-central1", conn.Gemini.Location)
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cfg := newWorkspaceModelTestConfig(t)
            cfg.ModelMetaConf.Provider2Models[tt.modelClass.String()] = map[string]ModelMeta{
                "default": {
                    DisplayInfo: &config.DisplayInfo{Name: tt.name},
                    Connection:  &config.Connection{BaseConnInfo: &config.BaseConnectionInfo{}},
                    Capability:  &developer_api.ModelAbility{},
                },
            }
            draft := newSystemModelTestInput("system-secret")
            draft.ProviderKey = tt.providerKey
            draft.ModelIdentifier = "provider-option-test"
            draft.ProviderOptions = tt.options

            modelID, err := cfg.UpsertSystemModel(context.Background(), 9, nil, draft)
            require.NoError(t, err)
            detail, err := cfg.GetSystemModelDetail(context.Background(), modelID)
            require.NoError(t, err)
            require.Equal(t, tt.options, detail.ProviderOptions)

            draft.ProviderOptions = detail.ProviderOptions
            draft.Endpoints[0].ID = ptr.Of(detail.Endpoints[0].ID)
            draft.Endpoints[0].APIKey = nil
            _, err = cfg.UpsertSystemModel(context.Background(), 9, &modelID, draft)
            require.NoError(t, err)

            var row systemModelRow
            require.NoError(t, cfg.db.Table(modelInstanceTable).First(&row, modelID).Error)
            var conn config.Connection
            require.NoError(t, json.Unmarshal([]byte(row.Connection), &conn))
            tt.assertConn(t, &conn)
        })
    }
}
```

Also add a test that inserts a legacy OpenAI connection with Azure fields and verifies `GetSystemModelDetail` projects them without exposing any API key.

- [ ] **Step 2: Run the backend tests and verify they fail**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./bizpkg/config/modelmgr -run 'TestSystemModelProviderOptions' -count=1
```

Expected: FAIL because detail and payload construction do not project provider options.

- [ ] **Step 3: Implement provider option validation and projection**

Add focused helpers in `system_store.go`:

```go
func validateSystemModelProviderOptions(providerKey string, options *config.ModelProviderOptions) error
func projectSystemModelProviderOptions(connectionJSON string, providerKey string) *config.ModelProviderOptions
func applySystemModelProviderOptions(connection *config.Connection, providerKey string, options *config.ModelProviderOptions)
```

Required behavior:

- `doubao` accepts only `ark_region` with a 128-byte limit.
- `openai` accepts only `openai_by_azure` and `openai_api_version`; API version is limited to 64 bytes.
- `gemini` accepts only backend `0`, `1`, or `2`; backend `2` requires non-empty Project and Location, each limited to 256 bytes.
- Any option belonging to another provider returns `ErrSystemModelInvalid` instead of being silently discarded.
- `GetSystemModelDetail` calls `projectSystemModelProviderOptions` and assigns `ProviderOptions`.
- `validateSystemModelInput` calls the validator.
- `buildSystemModelPayload` applies options after loading model metadata and before JSON encoding.

- [ ] **Step 4: Make provider protocol defaults match their runtime adapters**

Replace the current two-state protocol selection in `ListSystemModelProviders` with:

```go
func systemModelDefaultProtocol(providerKey string) string {
    switch normalizeProviderKey(providerKey) {
    case "claude":
        return "anthropic"
    case "gemini":
        return "gemini"
    case "ollama":
        return "ollama"
    default:
        return "openai-compatible"
    }
}
```

Add assertions for all seven provider keys to `system_store_test.go`.

- [ ] **Step 5: Re-run model manager tests**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./bizpkg/config/modelmgr -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit provider-option persistence**

```bash
git add backend/bizpkg/config/modelmgr/system_store.go backend/bizpkg/config/modelmgr/system_store_test.go
git commit -m "fix: retain model provider runtime options"
```

### Task 3: Replace the OpenAI-compatible false-positive probe

**Files:**

- Create: `backend/bizpkg/config/modelmgr/system_connectivity_test.go`
- Modify: `backend/bizpkg/config/modelmgr/system_store.go`

- [ ] **Step 1: Write the failing false-positive regression test**

Use `httptest.NewServer` with `/models` returning 200 and `/chat/completions` returning 404. Call `TestSystemModelEndpoint` with `Protocol: "openai-compatible"` and assert failure:

```go
func TestSystemModelEndpointUsesConfiguredModel(t *testing.T) {
    var calledPath string
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        calledPath = r.URL.Path
        if r.URL.Path == "/models" {
            w.WriteHeader(http.StatusOK)
            return
        }
        http.Error(w, `{"error":"Model not exist"}`, http.StatusNotFound)
    }))
    defer server.Close()

    cfg := newWorkspaceModelTestConfig(t)
    result, err := cfg.TestSystemModelEndpoint(context.Background(), &config.TestModelEndpointReq{
        ProviderKey: "qwen", ModelIdentifier: "qwen3.7-max", Protocol: "openai-compatible",
        Endpoint: &config.ModelEndpointInput{
            BaseURL: server.URL, APIKey: ptr.Of("secret"), Weight: 1, Enabled: true, SortOrder: 0,
        },
    })

    require.NoError(t, err)
    require.False(t, result.Success)
    require.Equal(t, "/chat/completions", calledPath)
    require.NotNil(t, result.ErrorCode)
    require.Equal(t, "model_not_found", *result.ErrorCode)
}
```

- [ ] **Step 2: Run the new test and verify it fails**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./bizpkg/config/modelmgr -run TestSystemModelEndpointUsesConfiguredModel -count=1
```

Expected: FAIL because the current implementation calls `/models` and returns success.

- [ ] **Step 3: Add success, credential, and status-mapping tests**

Add tests that decode the request JSON and assert:

```json
{
  "model": "qwen3.7-max",
  "messages": [{"role": "user", "content": "ping"}],
  "max_tokens": 1,
  "stream": false
}
```

Assert `Authorization: Bearer secret`, a maximum 12-second client timeout, stored encrypted credential resolution when `model_id` is supplied, and these stable mappings:

```text
401 -> invalid_credential
403 -> permission_denied
404 -> model_not_found
429 -> rate_limited
5xx -> provider_unavailable
network error -> network_error
```

Assert no provider response body is returned in `ErrorMessage`.

- [ ] **Step 4: Implement the minimal chat-completions probe**

Add a request DTO and URL helper:

```go
type systemModelChatProbeRequest struct {
    Model     string                        `json:"model"`
    Messages  []systemModelChatProbeMessage `json:"messages"`
    MaxTokens int                           `json:"max_tokens"`
    Stream    bool                          `json:"stream"`
}

func systemModelChatCompletionsURL(baseURL string) string {
    normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
    if strings.HasSuffix(strings.ToLower(normalized), "/chat/completions") {
        return normalized
    }
    return normalized + "/chat/completions"
}
```

For protocols containing `openai`, marshal the fixed one-token request, send `POST`, and set `Content-Type`, `Accept`, `Authorization`, and the existing User-Agent. Non-OpenAI protocols retain `workspaceModelProbeURL` and `GET`. Continue limiting response reads to 4 KiB and disallow redirects.

- [ ] **Step 5: Run connectivity and package tests**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./bizpkg/config/modelmgr -run TestSystemModelEndpoint -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./bizpkg/config/modelmgr -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the connectivity fix**

```bash
git add backend/bizpkg/config/modelmgr/system_store.go backend/bizpkg/config/modelmgr/system_connectivity_test.go
git commit -m "fix: test configured model with real inference"
```

### Task 4: Round-trip provider options through the system frontend

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/system/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/system-service.test.ts`

- [ ] **Step 1: Write the failing service serialization test**

Extend the managed-create test with:

```ts
provider_options: {
  openai_by_azure: true,
  openai_api_version: '2024-06-01',
},
```

Assert the exact JSON body sent to `/api/admin/config/model/create` contains the same object and does not contain any provider secret field.

- [ ] **Step 2: Run the service test and verify it fails type checking or assertion**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-service.test.ts
```

Expected: FAIL because `AdminModelManagementInput` has no provider options.

- [ ] **Step 3: Add the local view types**

Add to `service.ts`:

```ts
export interface AdminModelProviderOptions {
  ark_region?: string;
  openai_by_azure?: boolean;
  openai_api_version?: string;
  gemini_backend?: number;
  gemini_project?: string;
  gemini_location?: string;
}
```

Add `provider_options?: AdminModelProviderOptions` to both `AdminModelManagementInput` and `AdminModelDetail`. Existing request helpers already serialize the management object and need no separate secret handling.

- [ ] **Step 4: Re-run the service test**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-service.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit frontend contract wiring**

```bash
git add frontend/apps/coze-studio/src/pages/system/service.ts frontend/apps/coze-studio/src/pages/system/__tests__/system-service.test.ts
git commit -m "feat: round trip model provider options"
```

### Task 5: Add provider-driven fields to the model dialog

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/system/model-config-dialog.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/model-config-section.test.tsx`

- [ ] **Step 1: Write failing form hydration tests**

Add provider fixtures and test these visible states:

```ts
expect(container.querySelector('input[aria-label="Ark Region"]')).not.toBeNull();
expect(container.querySelector('input[aria-label="OpenAI Azure"]')).not.toBeNull();
expect(container.querySelector('input[aria-label="OpenAI API Version"]')).not.toBeNull();
expect(container.querySelector('select[aria-label="Gemini Backend"]')).not.toBeNull();
expect(container.querySelector('input[aria-label="Gemini Project"]')).not.toBeNull();
expect(container.querySelector('input[aria-label="Gemini Location"]')).not.toBeNull();
```

For edit mode, return `provider_options` from `getAdminManagedModelDetail`, save without changing the fields, and assert `updateAdminManagedModel` receives the same values.

- [ ] **Step 2: Run the component test and verify it fails**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/model-config-section.test.tsx
```

Expected: FAIL because the fields are absent.

- [ ] **Step 3: Hydrate and reset provider options safely**

Set `provider_options: {}` in `createInitialValue`, copy `detail.provider_options ?? {}` in `detailToValue`, and reset it when the provider select changes:

```ts
setValue(current => ({
  ...current,
  provider_key: event.target.value,
  protocol: provider?.protocol || current.protocol,
  provider_options: {},
  endpoints: nextEndpoints,
}));
```

- [ ] **Step 4: Render provider-specific controls**

Render controls in the model-parameters section using `value.provider_key`:

- `doubao`: text input `Ark Region`.
- `openai`: checkbox `OpenAI Azure` and text input `OpenAI API Version`.
- `gemini`: select `Gemini Backend` with values `0`, `1`, `2`; when value is `2`, render `Gemini Project` and `Gemini Location`.

Every change updates only `provider_options`; no credential value is stored there.

- [ ] **Step 5: Add Vertex validation**

Before endpoint validation, add:

```ts
if (
  value.provider_key === 'gemini' &&
  value.provider_options?.gemini_backend === 2 &&
  (!value.provider_options.gemini_project?.trim() ||
    !value.provider_options.gemini_location?.trim())
) {
  return '使用 Vertex AI 时请填写 Project 和 Location';
}
```

Trim string provider options in `normalizeValue` while preserving `false` and numeric zero.

- [ ] **Step 6: Run component and service tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/model-config-section.test.tsx src/pages/system/__tests__/system-service.test.ts
```

Expected: PASS.

- [ ] **Step 7: Commit provider-driven UI compatibility**

```bash
git add frontend/apps/coze-studio/src/pages/system/model-config-dialog.tsx frontend/apps/coze-studio/src/pages/system/__tests__/model-config-section.test.tsx
git commit -m "feat: expose legacy model provider options"
```

### Task 6: Retire only the legacy HTML model manager

**Files:**

- Modify: `backend/conf/admin/index.html`
- Create: `backend/conf/admin/index_test.go`
- Modify: `README.md`
- Modify: `README.zh_CN.md`

- [ ] **Step 1: Write the failing static retirement test**

Create `backend/conf/admin/index_test.go`:

```go
package admin_test

import (
    "os"
    "strings"
    "testing"
)

func TestLegacyAdminRetiresOnlyModelManagement(t *testing.T) {
    data, err := os.ReadFile("index.html")
    if err != nil {
        t.Fatal(err)
    }
    source := string(data)
    for _, removed := range []string{
        `data-page="model-management"`,
        "loadModelManagementPage",
        "renderModelManagement",
        "openAddModelModalWithProvider",
        "saveNewModel",
    } {
        if strings.Contains(source, removed) {
            t.Fatalf("legacy model UI still contains %q", removed)
        }
    }
    for _, preserved := range []string{
        `data-page="basic-config"`,
        `data-page="knowledge-config"`,
        "initBuiltinModelSelect",
        "/api/admin/config/model/list",
    } {
        if !strings.Contains(source, preserved) {
            t.Fatalf("required legacy admin capability %q was removed", preserved)
        }
    }
}
```

- [ ] **Step 2: Run the static test and verify it fails**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./conf/admin -count=1
```

Expected: FAIL because the legacy model UI is still present.

- [ ] **Step 3: Remove the exact obsolete HTML slices**

Use `apply_patch` to remove:

- the `model-management` submenu item;
- `nav.model`, `page.model.*`, `model.*`, and model-only help/card translations in both locales;
- all `model-management` entries from translation/page maps and the load branch;
- the model shortcut card in `showWorkspaceContent`;
- `loadModelManagementPage`, `sanitizeUrl`, `maskApiKey`, `getThinkingTypeText`, `renderModelManagement`, `deleteModel`, `openAddModelModalWithProvider`, `closeAddModelModal`, and `saveNewModel`.

Do not remove `initBuiltinModelSelect`, its `/api/admin/config/model/list` fetch, knowledge configuration, or the server model routes.

- [ ] **Step 4: Update setup documentation**

Replace only the model setup URL in both READMEs:

```text
http://localhost:8888/admin/#model-management
```

with:

```text
http://localhost:8888/system/models
```

- [ ] **Step 5: Run the static test and literal audit**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./conf/admin -count=1
cd ..
rg -n 'model-management|loadModelManagementPage|renderModelManagement|saveNewModel' backend/conf/admin/index.html README.md README.zh_CN.md
rg -n 'initBuiltinModelSelect|/api/admin/config/model/list' backend/conf/admin/index.html
```

Expected: Go test PASS; the first `rg` returns no matches; the second returns the preserved knowledge model selector and list call.

- [ ] **Step 6: Commit legacy UI retirement**

```bash
git add backend/conf/admin/index.html backend/conf/admin/index_test.go README.md README.zh_CN.md
git commit -m "refactor: retire legacy admin model manager"
```

### Task 7: Run focused and integrated verification

**Files:**

- Modify only if a test exposes a defect in the files already listed above.

- [ ] **Step 1: Verify generated contracts and formatting**

Run:

```bash
bash backend/scripts/verify_api_codegen.sh
gofmt -w backend/bizpkg/config/modelmgr/system_store.go backend/bizpkg/config/modelmgr/system_store_test.go backend/bizpkg/config/modelmgr/system_connectivity_test.go backend/conf/admin/index_test.go
git diff --check
```

Expected: code generation is deterministic and `git diff --check` prints nothing.

- [ ] **Step 2: Run backend regression**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./bizpkg/config/modelmgr ./api/handler/coze ./conf/admin -count=1
```

Expected: PASS.

- [ ] **Step 3: Run frontend contract and page tests**

Run:

```bash
cd frontend/packages/arch/api-schema
npm run test -- admin-model-management-contract.test.ts
cd ../../../apps/coze-studio
npm run test -- src/pages/system/__tests__/model-config-section.test.tsx src/pages/system/__tests__/system-service.test.ts src/pages/system/__tests__/system-page.test.tsx
npm run lint -- src/pages/system/service.ts src/pages/system/model-config-dialog.tsx src/pages/system/__tests__/model-config-section.test.tsx src/pages/system/__tests__/system-service.test.ts
```

Expected: all tests PASS and lint exits zero.

- [ ] **Step 4: Inspect the final dependency and secret surface**

Run:

```bash
rg -n 'api_key|secret|access_key|header' idl/admin/config.thrift frontend/apps/coze-studio/src/pages/system/service.ts frontend/apps/coze-studio/src/pages/system/model-config-dialog.tsx
rg -n 'model-management|loadModelManagementPage|renderModelManagement|saveNewModel' backend/conf/admin/index.html README.md README.zh_CN.md
git status --short
git diff origin/dev...HEAD --stat
```

Expected: secret literals exist only in write-only endpoint input declarations and password input handling; no retired HTML symbol remains; status contains only intentional changes.

- [ ] **Step 5: Run local browser acceptance before any dev integration**

Using the already-running project and the confirmed system administrator account:

- Open `http://localhost:8080/system/models`.
- Edit the Qwen model and verify the saved endpoint credential remains configured without being displayed.
- Run connectivity test: a nonexistent model identifier must show a safe failure; a valid model must show success.
- Open provider forms and verify Ark, OpenAI Azure, and Gemini Vertex fields appear only for their providers and survive edit/reopen.
- Confirm the browser console has no new errors and `/admin/#model-management` no longer renders a model-management page.

- [ ] **Step 6: Record the verified head without merging or pushing**

Run:

```bash
git status --short --branch
git log -8 --oneline --decorate
git rev-parse HEAD
git rev-parse origin/dev
```

Expected: clean feature branch with exact HEAD and current `origin/dev` recorded. Merging, publishing, or pushing still requires the user's confirmation against these exact SHAs.
