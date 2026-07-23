# Site Brand Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add production-grade site branding configuration to the existing Coze system settings, persist it through the current revisioned basic configuration contract, and apply it consistently to browser metadata, authentication, workspace navigation, top bars, and task conversations.

**Architecture:** Extend the existing `BasicConfiguration` source of truth with four optional branding fields. Administrators save text through the existing compare-and-swap basic configuration API and upload image assets through a dedicated, validated multipart endpoint. A public sanitized endpoint resolves stored object URIs into browser-safe URLs. The frontend bootstraps this projection before rendering authenticated or sign-in layouts and stores it in the existing common configuration store.

**Tech Stack:** Go, Hertz, Thrift/Hz, object storage, React, TypeScript, Zustand, Semi/Coze Design, Vitest, Go testing.

---

## File map

- `idl/admin/config.thrift`: extends the persisted basic configuration contract.
- `backend/bizpkg/config/base/base.go`: defaults, patch semantics, revisioned persistence.
- `backend/api/handler/coze/config_service.go`: admin save validation and public projection.
- `backend/api/handler/coze/site_config_service.go`: dedicated site asset upload and public configuration handlers.
- `backend/application/upload/icon.go`: object upload and signed URL resolution.
- `backend/api/router/coze/custom_routes.go`: public site configuration route.
- `backend/api/router/coze/api.go`: generated/admin asset upload route wiring.
- `frontend/packages/foundation/global-store/src/stores/common-config-store.ts`: site configuration state and defaults.
- `frontend/packages/foundation/global-adapter/src/site-config.ts`: public fetch and browser metadata application.
- `frontend/packages/foundation/global-adapter/src/hooks/use-app-init/use-init-common-config.ts`: global bootstrap.
- `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/index.tsx`: sign-in/register branding.
- `frontend/packages/studio/components/src/coze-brand/index.tsx`: configurable logo rendering.
- `frontend/apps/coze-studio/src/components/workspace-mark.tsx`: configurable global brand mark with existing fallback.
- `frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx`: configured product name.
- `frontend/apps/coze-studio/src/pages/workbench/index.tsx`: configured product name.
- `frontend/apps/coze-studio/src/pages/tasks/conversation-turn.tsx`: configured assistant label.
- `frontend/apps/coze-studio/src/pages/system/site-brand-settings-form.tsx`: system settings form and asset upload UX.
- `frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx`: embeds site configuration without changing existing settings behavior.
- `frontend/apps/coze-studio/src/pages/system/service.ts`: typed admin/public site APIs.

### Task 1: Lock the site configuration contract with failing tests

**Files:**
- Modify: `backend/bizpkg/config/base/base_test.go`
- Modify: `backend/api/handler/coze/config_service_test.go`
- Create: `backend/api/handler/coze/site_config_service_test.go`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/system-settings-section.test.tsx`
- Create: `frontend/packages/foundation/global-adapter/src/hooks/use-app-init/__tests__/use-init-common-config.test.tsx`
- Modify: `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/__tests__/index.test.tsx`

- [x] Add backend tests for defaults, partial patch persistence, unchanged-field preservation, and revision conflicts with the four new branding fields.
- [x] Add handler tests proving public responses expose only sanitized branding fields and never admin emails, registration policy, storage URIs, or credentials.
- [x] Add upload tests for admin-only access, supported MIME types, byte limits, dimension limits, transparent logo acceptance, favicon constraints, and stable object-key prefixes.
- [x] Add frontend bootstrap tests for success, fallback defaults, metadata application, and retry-safe initialization.
- [x] Add settings tests for dirty-field patching, upload preview, remove/reset, validation, save conflict, loading, error, and disabled states.
- [x] Add login tests proving configured site name, description, and logo replace hard-coded product branding.
- [x] Run targeted suites and record the expected failures before production code exists.

### Task 2: Extend and regenerate the basic configuration contract

**Files:**
- Modify: `idl/admin/config.thrift`
- Generate: `backend/api/model/admin/config/config.go`
- Generate: affected Hertz router/model files
- Generate: affected frontend API schema files

- [x] Add optional fields with stable IDs:
  - `site_name`
  - `site_description`
  - `site_logo_uri`
  - `favicon_uri`
- [x] Regenerate frontend API schema:

```bash
cd frontend/packages/arch/api-schema
npm run update
```

- [x] Regenerate backend models and router:

```bash
cd backend
PATH=/Users/liuwenbo/go/bin:$PATH hz update -idl ../idl/api.thrift -enable_extends
```

- [x] Keep generated files generated; do not hand-edit generated structs or serializers.

### Task 3: Persist branding and validate admin updates

**Files:**
- Modify: `backend/bizpkg/config/base/base.go`
- Modify: `backend/api/handler/coze/config_service.go`

- [x] Extend `BasicConfigurationPatch`, `IsEmpty`, save merging, and legacy/default projection.
- [x] Use stable defaults:
  - site name: `NewX AI`
  - description: current NewX sign-in product description
  - logo/favicon: empty URI, allowing frontend bundled fallbacks
- [x] Normalize text by trimming surrounding whitespace.
- [x] Enforce bounded lengths and reject control characters.
- [x] Accept image URI removal with an explicit empty string.
- [x] Preserve all fields absent from a partial patch and retain existing revision conflict behavior.
- [x] Restrict accepted site asset URIs to server-owned branding prefixes.

### Task 4: Add validated site asset upload and public projection

**Files:**
- Create: `backend/api/handler/coze/site_config_service.go`
- Modify: `backend/application/upload/icon.go`
- Modify: `backend/api/router/coze/custom_routes.go`
- Modify/Generate: `backend/api/router/coze/api.go`

- [x] Add `POST /api/admin/config/site/assets` behind the existing server-side admin middleware.
- [x] Accept one multipart image plus an explicit asset kind (`logo` or `favicon`).
- [x] Decode the image before storage and validate actual content, MIME type, byte size, width, height, and aspect constraints.
- [x] Store assets under server-controlled immutable keys:
  - `site-brand/logo/<content-hash>.<ext>`
  - `site-brand/favicon/<content-hash>.<ext>`
- [x] Return only `{ uri, url, width, height, mime_type }`.
- [x] Add `GET /api/site/config` outside authentication and return only:
  - `site_name`
  - `site_description`
  - `site_logo_url`
  - `favicon_url`
  - `revision`
- [x] Resolve object URIs to signed URLs at read time and degrade a single broken asset to its fallback without failing the full public response.
- [x] Never expose storage URIs through the public endpoint.

### Task 5: Bootstrap and apply site branding globally

**Files:**
- Modify: `frontend/packages/foundation/global-store/src/stores/common-config-store.ts`
- Create: `frontend/packages/foundation/global-adapter/src/site-config.ts`
- Modify: `frontend/packages/foundation/global-adapter/src/index.tsx`
- Modify: `frontend/packages/foundation/global-adapter/src/hooks/use-app-init/use-init-common-config.ts`
- Modify: `frontend/packages/foundation/global-adapter/src/components/global-layout/index.tsx`

- [x] Add a typed site configuration slice and fallback defaults to the existing common configuration store.
- [x] Fetch `/api/site/config` with same-origin credentials and a bounded timeout.
- [x] Treat network/JSON failures as fallback defaults while marking initialization complete.
- [x] Apply:
  - `document.title`
  - `<meta name="description">`
  - `<link rel="icon">`
- [x] Replace existing managed metadata nodes rather than appending duplicates.
- [x] Allow an authenticated admin save to refresh the store and metadata without a page reload.
- [x] Keep the existing global loading boundary until the first site configuration attempt completes.

### Task 6: Replace hard-coded user-visible product branding

**Files:**
- Modify: `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/index.tsx`
- Modify: `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/favicon.tsx`
- Modify: `frontend/packages/studio/components/src/coze-brand/index.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-mark.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/conversation-turn.tsx`

- [x] Render the configured logo, site name, and description on sign-in and registration pages.
- [x] Render a real `<img>` only when a configured logo URL exists; preserve the current bundled mark as the fallback.
- [x] Use the configured site name for top-bar readiness copy and task assistant identity.
- [x] Keep workspace names, user avatars, skills, MCP service icons, and Agent Runtime internal identity unchanged.
- [x] Preserve current layout dimensions, keyboard behavior, ARIA labels, error states, and mobile behavior.

### Task 7: Add the system site settings form

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/system/site-brand-settings-form.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/site-brand-settings-form.module.less`
- Modify: `frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/index.tsx`

- [x] Add a `站点配置` section before the existing system settings fields.
- [x] Include site name, service address, description, site logo, and browser icon.
- [x] Reuse `server_host` for service address rather than introducing a duplicate field.
- [x] Use Coze/Semi form, upload, tooltip, skeleton, empty, error, retry, and confirmation patterns.
- [x] Show current images, upload progress, validation errors, replacement preview, and remove/reset actions.
- [x] Keep text save revisioned through the existing basic configuration API.
- [x] Refresh public branding immediately after successful save.
- [x] Do not render marketplace, banner, default plaza agent, theme, or unrelated system configuration.

### Task 8: Verify the production flow

**Files:**
- Update: `docs/superpowers/specs/2026-07-23-site-brand-configuration-design.md`
- Update: this plan with completed checkboxes and evidence

- [x] Run targeted Go tests for base configuration, handlers, routing, MIME validation, and access control.
- [x] Run targeted Vitest suites for global bootstrap, account branding, settings form, and affected workspace/task components.
- [x] Run the IDL/code generation verification script for generated drift.
- [x] Use the Codex in-app browser to verify:
  - `/system/settings`
  - `/sign`
  - `/space/<space_id>/chats/new`
  - `/space/<space_id>/tasks/<task_id>`
- [x] Confirm title, meta description, favicon, logo, site name, immediate update, reload persistence, fallback behavior, non-admin denial, keyboard focus, and console cleanliness.
- [x] Record any external-object-storage or expired-session blocker explicitly; do not claim acceptance without visible evidence.


## Completion evidence (2026-07-23)

- Frontend production regression suite: `88` test files and `674` tests passed.
- TypeScript: `npx tsc --noEmit --project tsconfig.json` passed.
- Backend targeted production packages passed with Mockey-compatible compiler flags.
- API generation drift verification passed for all `56` hashed files.
- Atlas migration chain validated after the final formatting cleanup.
- Codex in-app browser acceptance covered system settings, model management, billing, MCP catalog, workbench, account settings, and task-detail usage behavior.
- The final implementation keeps site configuration inside the existing system settings section rather than introducing a second independent settings surface.
