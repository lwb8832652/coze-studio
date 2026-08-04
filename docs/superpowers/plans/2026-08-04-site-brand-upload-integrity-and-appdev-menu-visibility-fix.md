# Site Brand Upload Integrity and AppDev Menu Visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure site-brand uploads are verifiably persisted before configuration can reference them, prevent broken brand images, and temporarily hide the AppDev workspace menu without removing AppDev capability.

**Architecture:** Extend the existing site asset adapter with object metadata checks at upload, save, and public projection boundaries. Keep browser image loading as the final reachability check, add deterministic image fallbacks, and exclude AppDev only from the workspace-menu projection.

**Tech Stack:** Go, Hertz, object-storage abstraction, React, TypeScript, Vitest, jsdom, Rush.js.

---

## File map

- `backend/api/handler/coze/site_config_service.go`: upload and public URL projection.
- `backend/api/handler/coze/site_config_service_test.go`: upload/projection contracts.
- `backend/api/handler/coze/config_service.go`: save-time asset validation.
- `backend/api/handler/coze/config_service_test.go`: save validation tests.
- `frontend/apps/coze-studio/src/pages/system/site-asset-preview.ts`: browser reachability check.
- `frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx`: upload draft states.
- `frontend/apps/coze-studio/src/pages/system/__tests__/system-settings-section.test.tsx`: preview tests.
- `frontend/apps/coze-studio/src/components/workspace-mark.tsx`: Logo fallback.
- `frontend/apps/coze-studio/src/components/__tests__/workspace-mark.test.tsx`: Logo fallback test.
- `frontend/packages/foundation/global-adapter/src/site-config.ts`: favicon ownership.
- `frontend/packages/foundation/global-adapter/src/site-config.test.ts`: favicon tests.
- `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`: AppDev visibility policy.
- `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`: menu tests.
- `docs/superpowers/context/project-context.md`: retained AppDev fact.

### Task 1: Verify uploaded and publicly projected site assets

**Files:**
- Modify: `backend/api/handler/coze/site_config_service.go`
- Test: `backend/api/handler/coze/site_config_service_test.go`

- [ ] **Step 1: Write failing upload and projection tests**

Extend the test adapter with metadata lookup:

```go
assets := siteAssetService{
    upload: func(_ context.Context, body []byte, objectKey string) (string, string, error) {
        return objectKey, "https://assets.example.com/logo.png", nil
    },
    head: func(_ context.Context, objectKey string) error {
        headKeys = append(headKeys, objectKey)
        return nil
    },
}
```

Add a failure case using `storage.ErrObjectNotFound` and assert HTTP `500` with no returned URI.
Add a public configuration case where Logo metadata fails but favicon metadata and URL resolution
succeed; assert `site_logo_url` is empty and `favicon_url` remains populated.

- [ ] **Step 2: Run tests and verify RED**

Run from `backend`:

```bash
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze -run 'TestUploadSiteAsset|TestGetPublicSiteConfig'
```

Expected: compilation/assertion failure because `siteAssetService` has no `head` function.

- [ ] **Step 3: Implement metadata checks**

Add and wire the adapter field:

```go
type siteAssetService struct {
    upload func(context.Context, []byte, string) (string, string, error)
    url    func(context.Context, string) (string, error)
    head   func(context.Context, string) error
}
```

Production uses `upload.SVC.HeadObject`. After `assets.upload`, reject an empty URI, missing
metadata function, or metadata error before writing success. Resolve each public asset through a
helper that calls `head` before `url`; either failure produces an empty resource URL and a
credential-safe warning.

- [ ] **Step 4: Run tests and verify GREEN**

```bash
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze -run 'TestUploadSiteAsset|TestGetPublicSiteConfig'
```

Expected: selected tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/api/handler/coze/site_config_service.go backend/api/handler/coze/site_config_service_test.go
git commit -m "fix: verify persisted site brand uploads"
```

### Task 2: Reject missing brand objects during save

**Files:**
- Modify: `backend/api/handler/coze/config_service.go`
- Test: `backend/api/handler/coze/config_service_test.go`

- [ ] **Step 1: Write failing save tests**

Pass `siteAssetService` into the test server and add cases proving:

```go
// Non-empty site_logo_uri calls head and saves only after success.
// storage.ErrObjectNotFound returns 400 and leaves saveCalls at zero.
// Empty favicon_uri and text-only patches do not query storage.
```

- [ ] **Step 2: Run tests and verify RED**

```bash
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze -run 'TestBasicConfigurationHandler.*Brand'
```

Expected: failure because only URI syntax is validated.

- [ ] **Step 3: Implement save-time validation**

Pass `defaultSiteAssetService()` from the production handler and add:

```go
func validateSiteBrandObjects(
    ctx context.Context,
    patch baseconfig.BasicConfigurationPatch,
    assets siteAssetService,
) (string, error) {
    values := []struct {
        name string
        uri  *string
    }{
        {name: "site_logo_uri", uri: patch.SiteLogoURI},
        {name: "favicon_uri", uri: patch.FaviconURI},
    }
    for _, value := range values {
        if value.uri == nil || *value.uri == "" {
            continue
        }
        if assets.head == nil {
            return value.name, errors.New("site asset metadata service is unavailable")
        }
        if err := assets.head(ctx, *value.uri); err != nil {
            return value.name, err
        }
    }
    return "", nil
}
```

Map `storage.ErrObjectNotFound` to `400`; map other metadata failures to the existing internal
error response. Never call `SaveBaseConfig` after failure.

- [ ] **Step 4: Run focused and package tests**

```bash
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze -run 'TestBasicConfigurationHandler'
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze
```

Expected: both commands pass.

- [ ] **Step 5: Commit**

```bash
git add backend/api/handler/coze/config_service.go backend/api/handler/coze/config_service_test.go
git commit -m "fix: reject missing site brand objects"
```

### Task 3: Accept upload drafts only after preview succeeds

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/system/site-asset-preview.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx`
- Test: `frontend/apps/coze-studio/src/pages/system/__tests__/system-settings-section.test.tsx`

- [ ] **Step 1: Write failing preview tests**

Mock a new helper:

```ts
const verifySiteAssetPreview = vi.hoisted(() => vi.fn());
vi.mock('../site-asset-preview', () => ({ verifySiteAssetPreview }));
```

Assert upload success calls it before a URI is saved. When it rejects, assert the component shows
`上传资源不可访问`, does not show the image, and does not submit the returned URI. For a saved
non-empty URI with an empty public URL, assert `资源不可用，请重新上传`.

- [ ] **Step 2: Run test and verify RED**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-settings-section.test.tsx
```

Expected: assertions fail because preview reachability is not checked.

- [ ] **Step 3: Implement helper and guarded state update**

```ts
export const verifySiteAssetPreview = (url: string): Promise<void> =>
  new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => resolve();
    image.onerror = () => reject(new Error('site asset preview failed'));
    image.src = url;
  });
```

Await the helper after the API response and before updating URI/preview. Use a nested catch so
reachability and upload-validation messages stay distinct. Render an unavailable hint for
`configured && !preview`; image `onError` clears only the preview and reports reachability failure.

- [ ] **Step 4: Run test and verify GREEN**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-settings-section.test.tsx
```

Expected: all tests pass without React `act` warnings.

- [ ] **Step 5: Commit**

```bash
git add frontend/apps/coze-studio/src/pages/system/site-asset-preview.ts frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx frontend/apps/coze-studio/src/pages/system/__tests__/system-settings-section.test.tsx
git commit -m "fix: verify site brand previews before save"
```

### Task 4: Add workspace Logo fallback and single favicon ownership

**Files:**
- Modify: `frontend/apps/coze-studio/src/components/workspace-mark.tsx`
- Create: `frontend/apps/coze-studio/src/components/__tests__/workspace-mark.test.tsx`
- Modify: `frontend/packages/foundation/global-adapter/src/site-config.ts`
- Test: `frontend/packages/foundation/global-adapter/src/site-config.test.ts`

- [ ] **Step 1: Write failing tests**

Render `WorkspaceMark`, dispatch an image error, and assert it falls back to SVG. Add an unmanaged
static favicon before applying dynamic config and assert exactly one managed favicon remains. Then
apply config with no favicon and assert `/favicon.png` is restored as the only icon.

- [ ] **Step 2: Run tests and verify RED**

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/__tests__/workspace-mark.test.tsx
cd ../../packages/foundation/global-adapter
npm run test -- src/site-config.test.ts
```

Expected: broken image remains and duplicate favicons exist.

- [ ] **Step 3: Implement Logo fallback**

```tsx
const [failedLogoUrl, setFailedLogoUrl] = useState('');
const shouldRenderLogo = siteLogoUrl && failedLogoUrl !== siteLogoUrl;
```

Render the existing SVG when false; image `onError` stores the current URL. A later URL is retried.

- [ ] **Step 4: Implement favicon ownership**

When dynamic favicon exists, remove all unmanaged `link[rel~="icon"]` elements and upsert the
managed link. When absent, remove the managed link and ensure one default link exists:

```ts
const favicon = document.createElement('link');
favicon.rel = 'icon';
favicon.href = '/favicon.png';
document.head.appendChild(favicon);
```

- [ ] **Step 5: Run tests and verify GREEN**

Run the same two commands from Step 2. Expected: both pass.

- [ ] **Step 6: Commit**

```bash
git add frontend/apps/coze-studio/src/components/workspace-mark.tsx frontend/apps/coze-studio/src/components/__tests__/workspace-mark.test.tsx frontend/packages/foundation/global-adapter/src/site-config.ts frontend/packages/foundation/global-adapter/src/site-config.test.ts
git commit -m "fix: fall back from unavailable site branding"
```

### Task 5: Hide only the AppDev workspace menu entry

**Files:**
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
- Test: `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`
- Modify: `docs/superpowers/context/project-context.md`

- [ ] **Step 1: Write failing visibility assertions**

Keep the metadata assertion proving AppDev still exists, while visible menus exclude it:

```ts
expect(WORKSPACE_MENU_META).toContainEqual(
  expect.objectContaining({ path: SPACE_SUB_MODULE.APP_DEV }),
);
expect(getVisibleWorkspaceMenuMeta({ allow_develop: true })).not.toContainEqual(
  expect.objectContaining({ path: SPACE_SUB_MODULE.APP_DEV }),
);
```

Update rendered menu arrays to omit `网页应用开发` and preserve every other entry.

- [ ] **Step 2: Run test and verify RED**

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: AppDev is still visible.

- [ ] **Step 3: Implement temporary policy**

```ts
const TEMPORARILY_HIDDEN_WORKSPACE_MENU_PATHS = new Set<string>([
  SPACE_SUB_MODULE.APP_DEV,
]);
```

Filter this set in `getVisibleWorkspaceMenuMeta`; do not edit routes, notification mappings,
AppDev pages, APIs, or Sandbox scopes.

- [ ] **Step 4: Update long-term context**

Record under “AppDev 与 Sandbox” that AppDev capability/direct routes remain and only the sidebar
entry is temporarily hidden and not deprecated.

- [ ] **Step 5: Run menu and notification tests**

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/components/notification-center/__tests__/notification-bell.test.tsx
```

Expected: both files pass and notification direct links remain covered.

- [ ] **Step 6: Commit**

```bash
git add frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx docs/superpowers/context/project-context.md
git commit -m "fix: temporarily hide appdev workspace entry"
```

### Task 6: Full verification and browser regression

**Files:**
- Modify only if verification finds a defect in files listed above.

- [ ] **Step 1: Run backend verification**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./api/handler/coze
```

- [ ] **Step 2: Run frontend tests**

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-settings-section.test.tsx src/components/__tests__/workspace-mark.test.tsx src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/components/notification-center/__tests__/notification-bell.test.tsx
cd ../../packages/foundation/global-adapter
npm run test -- src/site-config.test.ts
```

- [ ] **Step 3: Run typecheck, lint, and diff checks**

```bash
cd frontend/apps/coze-studio
npx tsc --noEmit --project tsconfig.json
npm run lint
cd ../../../
git diff --check origin/dev...HEAD
```

Expected: commands exit `0`; any existing warning is reported separately.

- [ ] **Step 4: Run local browser matrix**

After rebuilding/restarting from this branch, use the `840582614` administrator account at
`http://localhost:8888/system/settings` to upload/save Logo and favicon, refresh, inspect workspace
Logo, confirm AppDev is absent from the sidebar, confirm exactly one dynamic favicon, and open the
existing AppDev direct route. No brand request may return `404` and no new console error may appear.

- [ ] **Step 5: Run first dev integration audit**

Follow `docs/superpowers/runbooks/dev-integration-audit.md`. Do not merge or push until the user
explicitly confirms the fresh report for the combined defect branch.
