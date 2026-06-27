# M4.8 Artifact Drawer Frontend Design

## Scope

M4.8 adds the first task-detail UI for registered artifacts. It targets canonical task-thread detail pages and uses the existing Workbench artifact list/content APIs. The feature is intentionally read-only: users can see artifact metadata, preview safe content through a browser-controlled blob URL, and download artifacts.

## UI Contract

- Task detail top bar shows a compact `产物` action when the current detail source is a canonical thread.
- Opening the action displays a right-side `SideSheet` titled `任务产物`.
- The panel lists artifacts with title, type, preview mode, size, and virtual path.
- Empty state says there are no artifacts for the task.
- Each artifact has:
  - `预览` for preview-safe modes: `text`, `image`, and `pdf`.
  - `下载` for every artifact.
- Preview and download both call the content endpoint. Preview opens a blob URL in a new tab. Download creates an object URL and clicks a temporary anchor with `download`.

## Data Flow

1. `fetchTaskDetail` loads artifacts only for canonical thread detail pages.
2. `TaskDetailPage` passes artifacts and thread ID into `TaskArtifactsPanel`.
3. `TaskArtifactsPanel` keeps SideSheet open/close and action loading state locally.
4. `service.ts` provides:
   - generated JSON client export for list.
   - manual `fetchTaskThreadArtifactContent` for raw bytes.
5. Blob URLs are revoked after use.

## Safety Rules

- Frontend never receives or renders object storage keys.
- Preview action is hidden for `download` and `unsupported` preview modes.
- HTML, XHTML, SVG, and unknown content stay download-only because the backend reports them as `download`.
- Preview uses `window.open` on a blob URL rather than inline HTML injection or iframe insertion.
- Download filename comes from artifact title first, then virtual path basename, then `artifact`.

## Semi Components

Use `SideSheet`, `List`, `Button`, `Tag`, and lightweight text layout from Coze Design/Semi. Semi MCP for version `2.72.3` confirms `SideSheet` is the drawer-equivalent component in this workspace; `Drawer` is not available in this version.

## Tests

- Service exports artifact list/content helpers and constructs the content endpoint with mode.
- Detail loader includes artifacts for canonical thread detail and does not load them for legacy task detail.
- Task detail renders the artifact action, opens the panel, lists artifact metadata, previews safe artifacts, and downloads any artifact.
