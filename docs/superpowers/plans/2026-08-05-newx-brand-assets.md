# NewX AI Brand Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create production-ready Open Portal Logo and favicon assets, activate them through the existing site configuration, and verify the repaired upload flow end to end.

**Architecture:** Keep the brand geometry in two small SVG source files and export deterministic 512×512 PNG files for the current upload contract. Do not replace the compiled default favicon; activate the new images only through the existing admin configuration and verify both public consumers.

**Tech Stack:** SVG, PNG, macOS Quick Look renderer, shell image metadata checks, React site configuration, Go site-brand API, Codex in-app browser.

---

## File map

- `frontend/apps/coze-studio/assets/brand/newx-site-logo.svg`: scalable Open Portal Logo source.
- `frontend/apps/coze-studio/assets/brand/newx-site-logo.png`: 512×512 Logo upload artifact.
- `frontend/apps/coze-studio/assets/brand/newx-favicon.svg`: scalable simplified favicon source.
- `frontend/apps/coze-studio/assets/brand/newx-favicon.png`: 512×512 favicon upload artifact.

### Task 1: Create and validate the vector sources

**Files:**
- Create: `frontend/apps/coze-studio/assets/brand/newx-site-logo.svg`
- Create: `frontend/apps/coze-studio/assets/brand/newx-favicon.svg`

- [ ] **Step 1: Add the Logo SVG**

Create the file with the approved colors and geometry:

```svg
<svg width="512" height="512" viewBox="0 0 512 512" fill="none" xmlns="http://www.w3.org/2000/svg">
  <rect x="8" y="8" width="496" height="496" rx="136" fill="#F5F8F6" stroke="#D7E4DB" stroke-width="16"/>
  <path d="M112 384 224 104h80L192 384h-80Z" fill="#176E43"/>
  <path d="M400 104 320 304l80 80h-96l-80-80 80-200h96Z" fill="#2AA767"/>
  <circle cx="400" cy="104" r="30" fill="#C9E94D"/>
</svg>
```

- [ ] **Step 2: Add the favicon SVG**

Create the high-contrast version without the Logo border, color split, or node:

```svg
<svg width="512" height="512" viewBox="0 0 512 512" fill="none" xmlns="http://www.w3.org/2000/svg">
  <rect width="512" height="512" rx="128" fill="#218C55"/>
  <path d="M120 392 216 120h80L200 392h-80Z" fill="#FFFFFF"/>
  <path d="M392 120 312 320l72 72h-96l-72-72 80-200h96Z" fill="#FFFFFF"/>
</svg>
```

- [ ] **Step 3: Validate both XML documents**

Run from the repository root:

```bash
xmllint --noout \
  frontend/apps/coze-studio/assets/brand/newx-site-logo.svg \
  frontend/apps/coze-studio/assets/brand/newx-favicon.svg
```

Expected: exit status `0` with no output.

- [ ] **Step 4: Inspect the source files visually**

Open both SVG files with the local image viewer. Confirm that the Logo has a light rounded tile,
two green portal planes and one lime node; confirm that the favicon has a green rounded tile and
only the white portal silhouette.

- [ ] **Step 5: Commit the vector sources**

```bash
git add frontend/apps/coze-studio/assets/brand/newx-site-logo.svg \
  frontend/apps/coze-studio/assets/brand/newx-favicon.svg
git commit -m "feat: add NewX brand vectors"
```

### Task 2: Export and validate the PNG upload artifacts

**Files:**
- Create: `frontend/apps/coze-studio/assets/brand/newx-site-logo.png`
- Create: `frontend/apps/coze-studio/assets/brand/newx-favicon.png`

- [ ] **Step 1: Render both SVG files at 512px**

Use the local Quick Look renderer with explicit paths:

```bash
qlmanage -t -s 512 -o /private/tmp \
  frontend/apps/coze-studio/assets/brand/newx-site-logo.svg \
  frontend/apps/coze-studio/assets/brand/newx-favicon.svg
mv /private/tmp/newx-site-logo.svg.png \
  frontend/apps/coze-studio/assets/brand/newx-site-logo.png
mv /private/tmp/newx-favicon.svg.png \
  frontend/apps/coze-studio/assets/brand/newx-favicon.png
```

Expected: both destination PNG files exist.

- [ ] **Step 2: Verify file type and dimensions**

```bash
file frontend/apps/coze-studio/assets/brand/newx-site-logo.png \
  frontend/apps/coze-studio/assets/brand/newx-favicon.png
sips -g pixelWidth -g pixelHeight \
  frontend/apps/coze-studio/assets/brand/newx-site-logo.png \
  frontend/apps/coze-studio/assets/brand/newx-favicon.png
```

Expected: both files are RGBA PNG images with width and height exactly `512`.

- [ ] **Step 3: Verify upload size limits**

```bash
test "$(stat -f %z frontend/apps/coze-studio/assets/brand/newx-site-logo.png)" -le 2097152
test "$(stat -f %z frontend/apps/coze-studio/assets/brand/newx-favicon.png)" -le 524288
```

Expected: both commands exit `0`.

- [ ] **Step 4: Inspect the 16px favicon result**

```bash
sips -z 16 16 frontend/apps/coze-studio/assets/brand/newx-favicon.png \
  --out /private/tmp/newx-favicon-16.png
```

Open `/private/tmp/newx-favicon-16.png` with pixel-preserving detail. Confirm that the white portal
remains separated from the green background without isolated accent pixels.

- [ ] **Step 5: Commit the PNG files**

```bash
git add frontend/apps/coze-studio/assets/brand/newx-site-logo.png \
  frontend/apps/coze-studio/assets/brand/newx-favicon.png
git commit -m "feat: add NewX brand upload images"
```

### Task 3: Activate the assets through system configuration

**Files:**
- Runtime configuration only; no source files change.

- [ ] **Step 1: Open the existing admin settings session**

Use the in-app browser at `http://localhost:8888/system/settings`. Confirm the authenticated account
is the documented `840582614` system administrator before changing configuration.

- [ ] **Step 2: Replace the Logo draft**

Choose `frontend/apps/coze-studio/assets/brand/newx-site-logo.png` in the Logo replacement control.
Expected: `POST /api/admin/config/site/assets` returns `200`, the preview image completes with a
non-zero natural width, and no upload error alert appears.

- [ ] **Step 3: Replace the favicon draft**

Choose `frontend/apps/coze-studio/assets/brand/newx-favicon.png` in the favicon replacement control.
Expected: `POST /api/admin/config/site/assets` returns `200`, the square-image validation passes,
and the preview image completes with a non-zero natural width.

- [ ] **Step 4: Save the configuration**

Click `保存配置` once both previews are valid. Expected:
`POST /api/admin/config/basic/save` returns `200`, the revision advances, and the page reports a
successful save without a conflict or unavailable-resource message.

- [ ] **Step 5: Reload and verify persisted previews**

Reload `/system/settings`. Expected: both preview images load, no broken-image placeholder is shown,
and neither card reports `资源不可用` or an upload failure.

### Task 4: Run browser and branch verification

**Files:**
- No source files change.

- [ ] **Step 1: Verify favicon ownership**

On `/system/settings`, inspect `link[rel~="icon"]`. Expected: exactly one favicon link exists, it is
the managed dynamic site-config link, and its image request succeeds.

- [ ] **Step 2: Verify the workspace Logo**

Open the current workspace page. Expected: the Open Portal Logo loads in the sidebar with non-zero
natural dimensions, while `网页应用开发` remains absent from the visible menu.

- [ ] **Step 3: Check fresh browser diagnostics**

Review page errors, console entries and failed network requests produced after the save. Expected:
no new Logo, favicon, site-config or object-storage errors.

- [ ] **Step 4: Re-run focused regression tests**

From `backend`:

```bash
GOCACHE=/private/tmp/coze-go-build go test -count=1 -gcflags="all=-l -N" \
  ./api/handler/coze -run 'TestUploadSiteAsset|TestGetPublicSiteConfig|TestBasicConfigurationHandler'
```

From `frontend/apps/coze-studio`:

```bash
npm run test -- \
  src/pages/system/__tests__/system-settings-section.test.tsx \
  src/components/__tests__/workspace-mark.test.tsx \
  src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

From `frontend/packages/foundation/global-adapter`:

```bash
npm run test -- src/site-config.test.ts
```

Expected: all selected tests pass.

- [ ] **Step 5: Confirm scope and cleanliness**

```bash
git status --short
git diff --check origin/dev...HEAD
git log --oneline origin/dev..HEAD
```

Expected: no uncommitted source changes, no whitespace errors, and only the current defect and
approved brand-asset commits are present. Continue with the repository's first `dev` integration
audit; do not merge or push without its required user confirmation.
