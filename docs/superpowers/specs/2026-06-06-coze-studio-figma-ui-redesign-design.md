# Coze Studio Figma UI Redesign Design

Date: 2026-06-06
Status: Design approved for implementation planning

## Summary

Redesign the Coze Studio workspace UI to follow the provided Figma prototypes while preserving existing backend and high-value product surfaces. The redesign focuses on the workspace sidebar, new task workbench, all tasks list, task execution detail, and skill configuration page.

The resource configuration page continues to use the existing resource library route and implementation. The development configuration page continues to use the existing project development route and implementation.

## References

- Figma home/new task prototype: file `CTjZCYL6itxNLakd650nuJ`, frame `3:1889`
- Figma all tasks prototype: file `CTjZCYL6itxNLakd650nuJ`, frame `4:723`
- Figma task execution prototype: file `CTjZCYL6itxNLakd650nuJ`, frame `4:1268`
- Figma skill configuration prototype: file `CTjZCYL6itxNLakd650nuJ`, frame `4:1607`
- Semi skill: `.codex/skills/semi-ui-skills/SKILL.md`
- Semi MCP config: `@douyinfe/semi-mcp`

## Goals

- Match the Figma information architecture in the Coze Studio workspace sidebar.
- Replace the current lower sidebar favorites area with a Coze Studio "我的任务" task list.
- Keep "资源配置" fully backed by the current resource library page.
- Keep "开发配置" fully backed by the current project development page.
- Refresh the local task workbench, all tasks, task execution, and skill configuration pages to align with the Figma visual language.
- Use existing Coze Studio APIs and existing Semi/Coze Design dependencies.

## Non-Goals

- Do not rebuild the resource library page.
- Do not rebuild the project development page.
- Do not introduce a new design system package.
- Do not copy Figma-generated React code directly into the app.
- Do not move Coze Studio task APIs into generic foundation packages.

## Existing Context

The app routes workspace pages under `/space/:space_id`. Current relevant routes are:

- `/space/:space_id/workbench`
- `/space/:space_id/library`
- `/space/:space_id/skill`
- `/space/:space_id/develop`
- `/space/:space_id/task-trigger`
- `/space/:space_id/tasks`

The existing workspace sidebar is composed in `@coze-foundation/space-ui-adapter`, with generic rendering in `@coze-foundation/space-ui-base`. The base sidebar currently renders the workspace menu plus a generic favorites list. Task APIs live in the app layer through `@coze-studio/api-schema`, so task-specific UI should stay in the Coze Studio adapter/app layer.

The frontend already uses Semi through the repository's existing Semi and Coze Design setup. Implementation should prefer `@coze-arch/coze-design` and established local wrappers, and consult Semi MCP documentation for component usage when the component API is unclear.

## Sidebar Design

The workspace sidebar should follow the Figma structure:

- Workspace header with avatar/name.
- "专属助理 Beta" assistant affordance.
- Primary "新建任务" entry mapped to `/space/:space_id/workbench`.
- Configuration navigation:
  - "资源配置" mapped to `/space/:space_id/library`
  - "技能配置" mapped to `/space/:space_id/skill`
  - "开发配置" mapped to `/space/:space_id/develop`
  - "任务触发器" mapped to `/space/:space_id/task-trigger`
  - "全部任务" mapped to `/space/:space_id/tasks`
- Lower "我的任务" task list using the existing task list API.

"全部任务" is the full-page task list entry. "我的任务" is the sidebar task summary and should show recent tasks in the same workspace. Clicking a task should navigate to a task execution/detail route.

### Sidebar Component Boundary

`space-ui-base` should remain generic. It can accept an optional lower-panel slot so adapter packages can replace the current favorites area. If no custom slot is provided, it should keep rendering the existing `FavoritesList`.

`space-ui-adapter` should own the Coze Studio task panel because it is closer to workspace routing and can safely depend on app-specific task APIs if required by package boundaries.

## Page Design

### New Task Workbench

Route: `/space/:space_id/workbench`

The workbench should become the Figma home/new-task page:

- Centered greeting: "欢迎来到 刘文波 的工作空间" or the current workspace/user-derived equivalent.
- Large task composer with the existing send behavior.
- Mode selector for Auto, Ask, and Agent.
- Extension selector and compact composer action buttons.
- Prominent green send button.
- Template cards below the composer.

The existing `sendWorkbenchChat` behavior remains the source of truth for submitting a task. The redesign changes presentation and light interaction affordances, not backend behavior.

### All Tasks

Route: `/space/:space_id/tasks`

The all tasks page should become the Figma list view:

- Centered content area with a max width similar to the prototype.
- Page title and short subtitle.
- Search input, status filter, and segmented "全部 / 已收藏" control.
- Task rows with title, status, update time, progress, and compact actions.
- Refresh remains available, but visually integrated with the toolbar.

Existing `listTasks`, `cancelTask`, and `retryTask` actions remain in use.

### Task Execution Detail

Route: `/space/:space_id/tasks/:task_id`

Add a task detail page matching the Figma task execution prototype:

- Top task status summary.
- User task prompt or title block.
- Execution flow/events section.
- Report/output section based on task data and task events.
- Bottom follow-up composer as UI-only affordance unless an existing follow-up API is available.

Use existing `getTask` and `listTaskEvents`. If event payloads are sparse, render a graceful timeline with available status/title/time data rather than inventing backend fields.

### Skill Configuration

Route: `/space/:space_id/skill`

The skill page should retain current list/import/test behavior but adopt the Figma structure:

- Page title, subtitle, and docs link.
- Category tabs or segmented controls.
- Search field.
- Primary "创建技能" action.
- Skill rows/cards with tags, owner/source metadata, and status.

Existing `listSkills`, `importSkill`, and `testRunSkill` remain in use.

### Resource Configuration

Route: `/space/:space_id/library`

No redesign of the underlying resource library page. The sidebar label changes to "资源配置"; navigation continues to the existing `Library` adapter page.

### Development Configuration

Route: `/space/:space_id/develop`

No redesign of the underlying project development page. The sidebar label changes to "开发配置"; navigation continues to the existing `Develop` adapter page.

## Data Flow

- Workbench uses the existing chat/workbench service to create tasks.
- Sidebar "我的任务" uses the existing task list API scoped by `space_id`.
- All tasks uses existing task list and task action APIs.
- Task detail uses existing task detail and task events APIs.
- Skill configuration uses existing skill list/import/test APIs.

All API errors should surface in-page using concise messages. Loading states should preserve layout height where possible to avoid jumpy UI.

## Semi And Styling Guidance

- Prefer `@coze-arch/coze-design` components already used in the repo.
- Use Semi MCP when component API or examples are uncertain.
- Use direct Semi imports only where the repository already follows that pattern or no Coze Design wrapper exists.
- Preserve existing Tailwind utility usage and Coze token classes such as `coz-bg-*`, `coz-fg-*`, and `coz-stroke-*`.
- Keep the visual language quiet and operational: white/light gray surfaces, compact spacing, 8px or smaller radii for cards and controls, no decorative hero art or gradient backgrounds.

## Accessibility And Responsiveness

- Sidebar items need visible active and hover states.
- Icon-only controls need accessible labels or tooltips.
- Composer and search inputs should have meaningful placeholders.
- Long task and skill names should truncate with tooltip or wrap without overlapping adjacent actions.
- Pages should remain usable at desktop and narrow viewport widths.

## Testing

Add or update focused tests for:

- Sidebar menu labels and navigation targets.
- Workbench first-screen rendering and submit behavior.
- All tasks helper behavior and action visibility.
- Task detail loading, error, and empty event states.
- Skill page stale output and list/search rendering behavior where practical.

Run package-level tests for `frontend/apps/coze-studio` and any touched foundation package tests. If the full app server can run locally, verify the major routes visually.

## Self-Review

- No backend API changes are required.
- Existing resource and development pages remain intact.
- Task APIs remain outside generic foundation UI.
- The sidebar "全部任务" and "我的任务" meanings are distinct and explicit.
- The design is scoped to Coze Studio workspace UI and does not require unrelated refactors.
