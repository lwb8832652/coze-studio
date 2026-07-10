# nuwax-ai AppDev Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Coze Studio 中生产级完整迁移 nuwax-ai 的网页应用开发 AppDev 能力，包含菜单、项目入口、IDE、文件编辑、实时预览、开发环境、AI SSE 对话、导入导出和完整验收。

**Architecture:** 前端在 `frontend/apps/coze-studio/src/pages/app-dev/` 新建独立 AppDev 页面域，按项目入口、IDE 布局、对话、文件、预览、运行状态拆分模块，并通过懒加载接入空间路由。后端在 `backend/domain/appdev/`、`backend/application/appdev/`、`backend/infra/appdev/` 和 `backend/api/handler/coze/` 增加 Go-native AppDev 领域能力，所有接口以服务端认证上下文做空间隔离和权限校验。

**Tech Stack:** React、TypeScript、Coze Design/Semi、Monaco Editor、Server-Sent Events、Go、Hertz、DDD 分层、Vitest、Go targeted tests。

**Execution status (2026-07-10):** 二期 AppDev 范围已完成生产验收。下文复选框
保留最初的 TDD 实施骨架，不再作为剩余任务判断依据；真实完成状态和证据以文末
`Final acceptance record` 为准，避免把历史步骤误报为未完成工作项。

---

## Scope

本计划覆盖二期完整 AppDev 迁移，不覆盖三期系统管理完全对齐。系统管理、用户管理、模型配置页面的 nuwax-ai 完整视觉和功能 parity 不在本计划内，但 AppDev 依赖的编码模型查询和未配置引导必须在二期完成。

## File Structure

### Frontend files

- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
  - 增加 `网页应用开发` 一级菜单，路径 `app-dev`，位置在 `资源配置` 与 `技能配置` 之间。
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`
  - 给 `app-dev` 菜单配置图标和 active icon。
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`
  - 覆盖菜单顺序、可见性、路由 key。
- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
  - 懒加载 AppDev 项目入口页和 IDE 页。
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
  - 注册 `/space/:spaceID/app-dev` 和 `/space/:spaceID/app-dev/:projectID`。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/types.ts`
  - 定义 AppDev 项目、文件、运行环境、对话、SSE 事件、模型、数据源类型。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/service.ts`
  - 封装 AppDev API client 和 response normalization。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-projects.ts`
  - 项目列表、创建、删除、导入、刷新。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-project-info.ts`
  - IDE 项目详情加载、权限错误、项目不存在。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-files.ts`
  - 文件树、内容加载、保存、新建、删除、重命名、上传。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-runtime.ts`
  - 开发环境启动、状态、keepAlive、重启、停止、日志。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-chat.ts`
  - AI 发送、取消、SSE 消息归并、历史加载。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-model-selector.ts`
  - 查询 PageApp 编码模型，处理未配置引导。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/utils/file-utils.ts`
  - 文件扩展名、语言、路径、大小、人类可读时间工具。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/utils/sse-utils.ts`
  - SSE 事件解析、消息投影、脱敏。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/project-list.tsx`
  - 项目入口页列表、搜索、刷新、空态、错误态。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/create-project-modal.tsx`
  - 新建网页应用弹窗。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/import-project-modal.tsx`
  - 导入项目弹窗。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/ide-layout.tsx`
  - 三栏 IDE 布局容器。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/chat-panel.tsx`
  - AI 对话区。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/chat-message.tsx`
  - 用户消息、AI 消息、思考、工具调用、错误消息渲染。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/file-tree.tsx`
  - 文件树、右键操作、空态。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/code-editor.tsx`
  - Monaco 编辑器懒加载、保存状态。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/preview-panel.tsx`
  - iframe 预览、刷新、打开新窗口、错误态。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/runtime-toolbar.tsx`
  - 启动、重启、停止、导出、运行状态。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/dev-logs-panel.tsx`
  - 开发日志展示和刷新。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/index.tsx`
  - 项目入口页。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/ide.tsx`
  - IDE 详情页。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/index.less`
  - 迁移并适配 nuwax-ai AppDev 样式。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-service.test.ts`
  - API client 测试。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-menu.test.tsx`
  - 菜单和路由测试可以合并到现有侧栏测试，必要时单独覆盖。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-project-list.test.tsx`
  - 项目入口页测试。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-ide.test.tsx`
  - IDE 布局和核心交互测试。
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-sse-utils.test.ts`
  - SSE 事件投影测试。

### Backend files

- Create: `backend/domain/appdev/entity.go`
  - AppDev 领域实体。
- Create: `backend/domain/appdev/repository.go`
  - Repository 接口。
- Create: `backend/domain/appdev/service.go`
  - 权限和路径校验领域服务。
- Create: `backend/application/appdev/project.go`
  - 项目用例。
- Create: `backend/application/appdev/file.go`
  - 文件用例。
- Create: `backend/application/appdev/runtime.go`
  - 运行环境用例。
- Create: `backend/application/appdev/chat.go`
  - AI 会话和 SSE 用例。
- Create: `backend/application/appdev/model.go`
  - PageApp 编码模型查询用例。
- Create: `backend/infra/appdev/local_store.go`
  - 本地开发环境文件存储封装。
- Create: `backend/infra/appdev/runtime_manager.go`
  - 开发服务器生命周期管理。
- Create: `backend/infra/appdev/sse_broker.go`
  - SSE 事件 broker。
- Create: `backend/api/handler/coze/app_dev_service.go`
  - HTTP handlers。
- Modify: `backend/api/router/coze/api.go`
  - 注册 AppDev 路由。
- Create: `backend/application/appdev/appdev_test.go`
  - 应用层 targeted tests。
- Create: `backend/api/handler/coze/app_dev_service_test.go`
  - handler targeted tests。

### Docs

- Modify: `docs/superpowers/specs/2026-07-05-nuwax-appdev-parity-design.md`
  - 实施中发现的已确认差异和验收证据写回。
- Create: `docs/superpowers/runbooks/appdev-local-debug.md`
  - AppDev 本地调试、端口、运行目录、验证账号、常见错误。

---

## Task 1: Add AppDev menu and routes

**Files:**
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`
- Modify: `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/ide.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/index.less`

- [ ] **Step 1: Write menu expectation**

Add the following expectation to the existing workspace submenu structure test:

```tsx
expect(labels).toEqual([
  '新建任务',
  '资源配置',
  '网页应用开发',
  '技能配置',
  '开发配置',
  '工作空间',
  '全部任务',
]);
expect(WORKSPACE_MENU_META[2]).toMatchObject({
  label: '网页应用开发',
  path: 'app-dev',
});
```

- [ ] **Step 2: Verify the menu test fails**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: FAIL because `网页应用开发` is not in `WORKSPACE_MENU_META`.

- [ ] **Step 3: Add AppDev menu metadata**

Add `APP_DEV: 'app-dev'` to `SPACE_SUB_MODULE`, insert this menu item after `资源配置`, and include it in `DEVELOPER_FEATURE_MENU_PATHS`:

```ts
{
  label: '网页应用开发',
  path: SPACE_SUB_MODULE.APP_DEV,
  dataTestId: 'navigation_workspace_app_dev',
}
```

- [ ] **Step 4: Add AppDev sidebar icon mapping**

In `MENU_ICONS`, map `SPACE_SUB_MODULE.APP_DEV` to code-style icons:

```tsx
[SPACE_SUB_MODULE.APP_DEV]: {
  icon: <IconCozCode />,
  activeIcon: <IconCozCodeFill />,
},
```

- [ ] **Step 5: Add lazy route exports**

In `async-components.tsx`, add:

```tsx
export const AppDev = lazy(() => import('../pages/app-dev'));
export const AppDevIDE = lazy(() => import('../pages/app-dev/ide'));
```

- [ ] **Step 6: Add route definitions**

In the space child routes, insert:

```tsx
{
  path: 'app-dev',
  Component: AppDev,
  loader: () => ({
    subMenuKey: SPACE_SUB_MODULE.APP_DEV,
  }),
},
{
  path: 'app-dev/:project_id',
  Component: AppDevIDE,
  loader: () => ({
    subMenuKey: SPACE_SUB_MODULE.APP_DEV,
  }),
},
```

- [ ] **Step 7: Create temporary production-shaped pages**

Create `index.tsx`:

```tsx
import './index.less';

export default function AppDevPage() {
  return (
    <main className="app-dev-page">
      <section className="app-dev-page__header">
        <div>
          <p className="app-dev-page__eyebrow">网页应用开发</p>
          <h1>网页应用项目</h1>
          <p>创建、导入并进入网页应用开发工作台。</p>
        </div>
      </section>
    </main>
  );
}
```

Create `ide.tsx`:

```tsx
import './index.less';

export default function AppDevIDEPage() {
  return (
    <main className="app-dev-ide">
      <section className="app-dev-ide__pane app-dev-ide__chat">AI 对话</section>
      <section className="app-dev-ide__pane app-dev-ide__editor">文件与编辑器</section>
      <section className="app-dev-ide__pane app-dev-ide__preview">预览与日志</section>
    </main>
  );
}
```

Create `index.less`:

```less
.app-dev-page {
  min-height: 100%;
  padding: 32px;
  background: #f7f8fb;
}

.app-dev-page__header {
  display: flex;
  justify-content: space-between;
  gap: 24px;
}

.app-dev-page__eyebrow {
  margin: 0 0 8px;
  color: #4f46e5;
  font-size: 13px;
  font-weight: 600;
}

.app-dev-page h1 {
  margin: 0;
  color: #111827;
  font-size: 28px;
  line-height: 36px;
}

.app-dev-page p {
  margin: 8px 0 0;
  color: #5f6675;
}

.app-dev-ide {
  display: grid;
  grid-template-columns: 360px minmax(420px, 1fr) 420px;
  height: 100%;
  min-height: 720px;
  background: #eef1f6;
}

.app-dev-ide__pane {
  min-width: 0;
  border-right: 1px solid #dde2ec;
  background: #fff;
}
```

- [ ] **Step 8: Run targeted frontend test**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: PASS.

---

## Task 2: Define AppDev frontend contracts and API client

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/app-dev/types.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-service.test.ts`

- [ ] **Step 1: Write service normalization tests**

Create tests for project list, project create, file tree, runtime status, SSE URL creation, and error normalization. Use mocked `fetch`.

```ts
import {
  createAppDevProject,
  getAppDevProject,
  listAppDevProjects,
  normalizeAppDevError,
} from '../service';

describe('app-dev service', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('normalizes project list response', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: {
            items: [{ id: 'p1', name: '商城首页', status: 'ready' }],
            total: 1,
          },
        }),
      ),
    );

    await expect(listAppDevProjects({ spaceId: 's1' })).resolves.toEqual({
      items: [{ id: 'p1', name: '商城首页', status: 'ready' }],
      total: 1,
    });
  });

  it('sends project creation payload without user controlled owner fields', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: { id: 'p2', name: '活动页', status: 'creating' },
        }),
      ),
    );

    await createAppDevProject({
      spaceId: 's1',
      name: '活动页',
      prompt: '做一个活动落地页',
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/projects',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          name: '活动页',
          prompt: '做一个活动落地页',
        }),
      }),
    );
  });

  it('turns http errors into readable messages', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ message: '无权限访问项目' }), {
        status: 403,
      }),
    );

    await expect(getAppDevProject({ spaceId: 's1', projectId: 'p1' })).rejects.toThrow(
      '无权限访问项目',
    );
  });

  it('normalizes unknown errors', () => {
    expect(normalizeAppDevError(new Error('network down'))).toBe('network down');
    expect(normalizeAppDevError('bad')).toBe('bad');
    expect(normalizeAppDevError(null)).toBe('请求失败，请稍后重试');
  });
});
```

- [ ] **Step 2: Run service tests to verify failure**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-service.test.ts
```

Expected: FAIL because `service.ts` and `types.ts` do not exist.

- [ ] **Step 3: Define TypeScript contracts**

Create `types.ts` with project, file, runtime, chat, SSE, model, and API response types. Include literal unions for statuses:

```ts
export type AppDevProjectStatus = 'creating' | 'ready' | 'archived' | 'error';
export type AppDevRuntimeStatus = 'stopped' | 'starting' | 'running' | 'restarting' | 'error';
export type AppDevChatRole = 'user' | 'assistant' | 'system';
export type AppDevMessageType =
  | 'user'
  | 'assistant'
  | 'thinking'
  | 'tool_call'
  | 'tool_call_update'
  | 'error'
  | 'section';

export interface AppDevProject {
  id: string;
  name: string;
  description?: string;
  prompt?: string;
  status: AppDevProjectStatus;
  runtimeStatus?: AppDevRuntimeStatus;
  previewUrl?: string;
  creatorName?: string;
  updatedAt?: string;
  createdAt?: string;
}

export interface AppDevFileNode {
  id: string;
  path: string;
  name: string;
  type: 'file' | 'directory';
  size?: number;
  children?: AppDevFileNode[];
  updatedAt?: string;
}

export interface AppDevFileContent {
  path: string;
  content: string;
  version: string;
  language: string;
}

export interface AppDevRuntimeInfo {
  status: AppDevRuntimeStatus;
  previewUrl?: string;
  message?: string;
  lastKeepAliveAt?: string;
}

export interface AppDevChatMessage {
  id: string;
  type: AppDevMessageType;
  role?: AppDevChatRole;
  content: string;
  title?: string;
  isStreaming?: boolean;
  createdAt?: string;
}

export interface AppDevSSEEvent {
  event: string;
  data: Record<string, unknown>;
}

export interface AppDevModel {
  id: string;
  name: string;
  provider?: string;
  protocol?: string;
}

export interface AppDevPageResult<T> {
  items: T[];
  total: number;
}

export interface AppDevApiResponse<T> {
  code?: number;
  message?: string;
  data?: T;
}
```

- [ ] **Step 4: Implement fetch wrapper and project APIs**

Create `service.ts` with a shared `requestAppDev` helper and project APIs:

```ts
import type {
  AppDevApiResponse,
  AppDevPageResult,
  AppDevProject,
} from './types';

const jsonHeaders = {
  'content-type': 'application/json',
};

export const normalizeAppDevError = (error: unknown) => {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  if (typeof error === 'string' && error) {
    return error;
  }
  return '请求失败，请稍后重试';
};

async function readJson<T>(response: Response): Promise<AppDevApiResponse<T>> {
  if (response.headers.get('content-type')?.includes('application/json')) {
    return (await response.json()) as AppDevApiResponse<T>;
  }
  return {} as AppDevApiResponse<T>;
}

async function requestAppDev<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  const payload = await readJson<T>(response);

  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.message || `请求失败：${response.status}`);
  }

  return payload.data as T;
}

export const listAppDevProjects = async ({
  spaceId,
  keyword,
}: {
  spaceId: string;
  keyword?: string;
}) => {
  const params = new URLSearchParams();
  if (keyword?.trim()) {
    params.set('keyword', keyword.trim());
  }
  const query = params.toString();

  return requestAppDev<AppDevPageResult<AppDevProject>>(
    `/api/app-dev/spaces/${spaceId}/projects${query ? `?${query}` : ''}`,
  );
};

export const createAppDevProject = async ({
  spaceId,
  name,
  prompt,
}: {
  spaceId: string;
  name: string;
  prompt: string;
}) =>
  requestAppDev<AppDevProject>(`/api/app-dev/spaces/${spaceId}/projects`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ name, prompt }),
  });

export const getAppDevProject = async ({
  spaceId,
  projectId,
}: {
  spaceId: string;
  projectId: string;
}) =>
  requestAppDev<AppDevProject>(
    `/api/app-dev/spaces/${spaceId}/projects/${projectId}`,
  );
```

- [ ] **Step 5: Run service tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-service.test.ts
```

Expected: PASS.

---

## Task 3: Build AppDev project entry page

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-projects.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/project-list.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/create-project-modal.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/import-project-modal.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/index.less`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-project-list.test.tsx`

- [ ] **Step 1: Write project page tests**

Cover loading, empty, error, list, create modal, navigation to IDE. Mock `listAppDevProjects` and `createAppDevProject`.

- [ ] **Step 2: Verify project page tests fail**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-project-list.test.tsx
```

Expected: FAIL because the project list hook and components are not implemented.

- [ ] **Step 3: Implement `useAppDevProjects`**

The hook must expose:

```ts
{
  projects,
  total,
  loading,
  error,
  keyword,
  setKeyword,
  refresh,
  createProject,
}
```

Behavior:

- Fetch on mount and when `spaceId` changes.
- Debounce keyword search by 300ms.
- Return readable errors through `normalizeAppDevError`.
- Disable create while request is in flight.

- [ ] **Step 4: Implement project list UI**

Required states:

- Loading skeleton.
- Empty state with `创建网页应用` primary button.
- Error state with retry button.
- List cards with project name, status, creator, update time, and `进入开发` button.
- Search input and refresh button.

- [ ] **Step 5: Implement create and import modals**

Create modal fields:

- 应用名称：required, max 50.
- 初始需求：required, max 2000.

Import modal fields:

- zip file upload.
- name override optional.
- submit disabled until file selected.

- [ ] **Step 6: Wire page navigation**

After project creation, navigate to:

```ts
`/space/${spaceId}/app-dev/${project.id}`
```

- [ ] **Step 7: Run project page tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-project-list.test.tsx
```

Expected: PASS.

---

## Task 4: Implement AppDev backend project and file foundations

**Files:**
- Create: `backend/domain/appdev/entity.go`
- Create: `backend/domain/appdev/repository.go`
- Create: `backend/domain/appdev/service.go`
- Create: `backend/application/appdev/project.go`
- Create: `backend/application/appdev/file.go`
- Create: `backend/infra/appdev/local_store.go`
- Create: `backend/api/handler/coze/app_dev_service.go`
- Modify: `backend/api/router/coze/api.go`
- Create: `backend/application/appdev/appdev_test.go`
- Create: `backend/api/handler/coze/app_dev_service_test.go`

- [ ] **Step 1: Write application tests first**

Test cases:

- Create project uses authenticated user and server-side space ID.
- List projects filters by space ID.
- Get project denies cross-space access.
- File path `../secret` is rejected.
- Save file creates parent directories only under project root.

- [ ] **Step 2: Run backend tests to verify failure**

Run:

```bash
cd backend
go test ./application/appdev ./api/handler/coze -run AppDev -count=1
```

Expected: FAIL because packages do not exist.

- [ ] **Step 3: Define domain entities**

Core structs:

```go
type ProjectStatus string
type RuntimeStatus string

const (
    ProjectStatusCreating ProjectStatus = "creating"
    ProjectStatusReady    ProjectStatus = "ready"
    ProjectStatusArchived ProjectStatus = "archived"
    ProjectStatusError    ProjectStatus = "error"

    RuntimeStatusStopped    RuntimeStatus = "stopped"
    RuntimeStatusStarting   RuntimeStatus = "starting"
    RuntimeStatusRunning    RuntimeStatus = "running"
    RuntimeStatusRestarting RuntimeStatus = "restarting"
    RuntimeStatusError      RuntimeStatus = "error"
)

type Project struct {
    ID            string
    SpaceID       string
    Name          string
    Description   string
    Prompt        string
    Status        ProjectStatus
    RuntimeStatus RuntimeStatus
    PreviewURL    string
    CreatorID     string
    CreatorName   string
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type FileNode struct {
    ID        string
    Path      string
    Name      string
    Type      string
    Size      int64
    Children  []*FileNode
    UpdatedAt time.Time
}
```

- [ ] **Step 4: Implement repository interfaces**

Define interfaces for project metadata and file store separately so the first implementation can use local storage while preserving boundaries.

- [ ] **Step 5: Implement path sanitizer**

Rules:

- Clean slash paths.
- Reject empty path for file operations.
- Reject absolute paths.
- Reject paths containing `..`.
- Reject null bytes.
- Limit path length to 512.

- [ ] **Step 6: Implement local store**

Use an AppDev root under an ignored runtime directory. The infra API should only accept sanitized project IDs and sanitized relative paths.

- [ ] **Step 7: Implement HTTP handlers**

Handlers:

- `GET /api/app-dev/spaces/:space_id/projects`
- `POST /api/app-dev/spaces/:space_id/projects`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/files`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/files/content`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/files/content`
- `DELETE /api/app-dev/spaces/:space_id/projects/:project_id/files`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/files/rename`

- [ ] **Step 8: Run backend tests**

Run:

```bash
cd backend
go test ./application/appdev ./api/handler/coze -run AppDev -count=1
```

Expected: PASS.

---

## Task 5: Build file tree and Monaco editor

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-project-info.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-files.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/utils/file-utils.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/file-tree.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/code-editor.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/ide.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-ide.test.tsx`

- [ ] **Step 1: Write IDE file interaction tests**

Cover:

- Loads project info and file tree.
- Opens file content.
- Edits file and shows unsaved state.
- Saves file and clears unsaved state.
- Rejects save when no file selected.
- Shows no-permission and not-found states.

- [ ] **Step 2: Verify IDE tests fail**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-ide.test.tsx
```

Expected: FAIL.

- [ ] **Step 3: Add file service APIs**

Add:

```ts
export const listAppDevFiles = async (...);
export const getAppDevFileContent = async (...);
export const saveAppDevFileContent = async (...);
export const deleteAppDevFile = async (...);
export const renameAppDevFile = async (...);
```

- [ ] **Step 4: Implement file utilities**

Functions:

- `getLanguageFromPath(path: string): string`
- `formatFileSize(size?: number): string`
- `getFileName(path: string): string`
- `isEditableFile(path: string, size?: number): boolean`

- [ ] **Step 5: Implement file hook**

State:

- `tree`
- `selectedPath`
- `content`
- `draft`
- `version`
- `dirty`
- `loadingTree`
- `loadingContent`
- `saving`
- `error`

Actions:

- `refreshTree`
- `openFile`
- `setDraft`
- `saveCurrentFile`
- `deletePath`
- `renamePath`

- [ ] **Step 6: Implement FileTree component**

Requirements:

- Directory expand/collapse.
- Selected file highlight.
- Empty state.
- Context actions: 新建文件, 新建目录, 重命名, 删除, 刷新.
- Confirmation before delete.

- [ ] **Step 7: Implement Monaco editor wrapper**

Requirements:

- Lazy import Monaco.
- Readonly state while loading or no selected file.
- Save shortcut `Cmd/Ctrl+S`.
- Language mode from path.
- Unsaved indicator.
- Large file warning.

- [ ] **Step 8: Run IDE tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-ide.test.tsx
```

Expected: PASS.

---

## Task 6: Add runtime manager, preview, and logs

**Files:**
- Create: `backend/application/appdev/runtime.go`
- Create: `backend/infra/appdev/runtime_manager.go`
- Modify: `backend/api/handler/coze/app_dev_service.go`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-runtime.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/preview-panel.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/runtime-toolbar.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/dev-logs-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/ide.tsx`

- [ ] **Step 1: Write runtime backend tests**

Cover:

- Start runtime for an existing project.
- KeepAlive returns preview URL when running.
- Restart transitions through restarting.
- Stop clears running status.
- Logs are scoped to project and space.

- [ ] **Step 2: Implement backend runtime API**

Routes:

- `POST /api/app-dev/spaces/:space_id/projects/:project_id/runtime/start`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/runtime/status`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/runtime/keep-alive`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/runtime/restart`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/runtime/stop`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/runtime/logs`

- [ ] **Step 3: Implement frontend runtime service APIs**

Add service functions for all runtime routes.

- [ ] **Step 4: Implement runtime hook**

Behavior:

- Poll status every 5 seconds while IDE is open.
- KeepAlive every 30 seconds when running.
- Stop polling on unmount.
- Show last error message without leaking raw logs.

- [ ] **Step 5: Implement preview panel**

States:

- No URL.
- Starting.
- Running with iframe.
- Preview load failed.
- Runtime error.
- Refreshing.

- [ ] **Step 6: Implement toolbar and logs**

Toolbar actions:

- 启动环境
- 重启
- 停止
- 刷新预览
- 导出项目

Logs:

- Timestamp.
- Level.
- Message.
- Refresh button.
- Empty state.

- [ ] **Step 7: Run targeted tests**

Run:

```bash
cd backend
go test ./application/appdev ./api/handler/coze -run AppDevRuntime -count=1

cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-ide.test.tsx
```

Expected: PASS.

---

## Task 7: Add AI chat and SSE event parity

**Files:**
- Create: `backend/application/appdev/chat.go`
- Create: `backend/infra/appdev/sse_broker.go`
- Modify: `backend/api/handler/coze/app_dev_service.go`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/utils/sse-utils.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-chat.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/chat-panel.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/components/chat-message.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-sse-utils.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/ide.tsx`

- [ ] **Step 1: Write SSE projection tests**

Cover these nuwax-ai message events:

- `prompt_start`
- `agent_thought_chunk`
- `agent_message_chunk`
- `tool_call`
- `tool_call_update`
- `prompt_end`
- `heartbeat`
- `error`

Expected projection:

- raw payload is not exposed to UI.
- assistant chunks merge into a streaming assistant message.
- thinking chunks merge into a thinking message.
- tool calls render with title and safe summary.
- prompt_end clears streaming state.

- [ ] **Step 2: Implement SSE projection utilities**

Functions:

- `parseSSEEvent(raw: string): AppDevSSEEvent | null`
- `projectAppDevEvent(messages, event): AppDevChatMessage[]`
- `sanitizeToolPayload(payload): string`

Never expose:

- credentials.
- raw prompt.
- raw provider body.
- checkpoint bytes.
- object storage URIs.

- [ ] **Step 3: Implement backend chat routes**

Routes:

- `POST /api/app-dev/spaces/:space_id/projects/:project_id/chat`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/chat/events`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/chat/cancel`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/chat/history`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/chat/status`

- [ ] **Step 4: Implement chat hook**

Behavior:

- Send message with selected model and attachments.
- Open EventSource to events endpoint.
- Append projected messages.
- Cancel running task.
- Load history on project open.
- Reconnect only for transient disconnects.

- [ ] **Step 5: Implement chat UI**

Required UI:

- Message list.
- Prompt input.
- Send button.
- Cancel button while running.
- Thinking block.
- Tool call block.
- Error block with retry guidance.
- Empty conversation state.

- [ ] **Step 6: Run SSE and IDE tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-sse-utils.test.ts src/pages/app-dev/__tests__/app-dev-ide.test.tsx
```

Expected: PASS.

---

## Task 8: Add coding model selector and data source binding

**Files:**
- Create: `backend/application/appdev/model.go`
- Modify: `backend/api/handler/coze/app_dev_service.go`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-model-selector.ts`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/chat-panel.tsx`

- [ ] **Step 1: Write model selector tests**

Cover:

- Loads PageApp models.
- Filters unsupported usage scenarios.
- Shows configuration guidance when no model exists.
- Blocks send until a model is selected when required.

- [ ] **Step 2: Implement backend model endpoint**

Route:

```text
GET /api/app-dev/spaces/:space_id/models?scenario=PageApp
```

Return only safe metadata:

- id.
- name.
- provider.
- protocol.

- [ ] **Step 3: Implement frontend selector hook**

Expose:

```ts
{
  models,
  selectedModelId,
  setSelectedModelId,
  loading,
  error,
  hasUsableModel,
  refresh,
}
```

- [ ] **Step 4: Add selector to chat panel**

UX:

- Compact selector near input.
- No model state shows a warning banner.
- Banner links to current model configuration page if available.

- [ ] **Step 5: Run targeted tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-ide.test.tsx
```

Expected: PASS.

---

## Task 9: Add import, export, and project archive

**Files:**
- Modify: `backend/application/appdev/project.go`
- Modify: `backend/application/appdev/file.go`
- Modify: `backend/api/handler/coze/app_dev_service.go`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/import-project-modal.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/runtime-toolbar.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/project-list.tsx`

- [ ] **Step 1: Write import/export tests**

Cover:

- Upload accepts `.zip` only.
- Rejects path traversal entries inside zip.
- Export returns archive for current project only.
- Archive project removes it from active list but keeps audit metadata.

- [ ] **Step 2: Implement backend import/export**

Routes:

- `POST /api/app-dev/spaces/:space_id/projects/import`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/export`
- `DELETE /api/app-dev/spaces/:space_id/projects/:project_id`

- [ ] **Step 3: Implement frontend import flow**

Flow:

- Select zip.
- Show file name and size.
- Submit disabled during upload.
- Navigate to imported project on success.
- Show backend validation errors.

- [ ] **Step 4: Implement export flow**

Flow:

- Click export.
- Download `project-name.zip`.
- Show success or error toast.

- [ ] **Step 5: Run targeted tests**

Run:

```bash
cd backend
go test ./application/appdev ./api/handler/coze -run AppDev -count=1

cd frontend/apps/coze-studio
npm run test -- src/pages/app-dev/__tests__/app-dev-project-list.test.tsx src/pages/app-dev/__tests__/app-dev-ide.test.tsx
```

Expected: PASS.

---

## Task 10: Visual parity pass against nuwax-ai

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/index.less`
- Modify focused AppDev components only when visual or interaction gaps are found.
- Modify: `docs/superpowers/specs/2026-07-05-nuwax-appdev-parity-design.md`

- [ ] **Step 1: Capture nuwax-ai reference**

Open local nuwax-ai:

```text
http://localhost/
```

Login:

```text
admin@nuwax.com
123456
```

Capture screenshots for:

- AppDev project entry.
- Create project flow.
- IDE empty/loading state.
- IDE with file selected.
- AI task running.
- Tool call display.
- Runtime starting/running/error.
- Preview loaded/error.
- Import/export states.

- [ ] **Step 2: Capture Coze AppDev**

Open:

```text
http://localhost:8080/space/<spaceID>/app-dev
http://localhost:8080/space/<spaceID>/app-dev/<projectID>
```

Capture the same screens.

- [ ] **Step 3: Fix each visible difference**

For every mismatch, classify and fix:

- Layout mismatch.
- Missing action.
- Wrong text.
- Wrong spacing.
- Wrong state color.
- Missing hover/selected/disabled.
- Missing loading/empty/error.
- Incorrect modal behavior.
- Incorrect stream/tool rendering.

- [ ] **Step 4: Record parity evidence**

Update the spec with:

- Reference page.
- Coze page.
- Result.
- Remaining gap count.

Expected final remaining gap count: `0`.

---

## Task 11: Full production verification

**Files:**
- Modify only files required to fix verification failures.
- Modify: `docs/superpowers/specs/2026-07-05-nuwax-appdev-parity-design.md`
- Create: `docs/superpowers/runbooks/appdev-local-debug.md`

- [ ] **Step 1: Run frontend targeted tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/app-dev/__tests__/app-dev-service.test.ts src/pages/app-dev/__tests__/app-dev-project-list.test.tsx src/pages/app-dev/__tests__/app-dev-ide.test.tsx src/pages/app-dev/__tests__/app-dev-sse-utils.test.ts
```

Expected: PASS.

- [ ] **Step 2: Run frontend typecheck**

Run:

```bash
cd frontend/apps/coze-studio
npx tsc --noEmit --project tsconfig.json
```

Expected: PASS.

- [ ] **Step 3: Run backend targeted tests**

Run:

```bash
cd backend
go test ./application/appdev ./api/handler/coze -run AppDev -count=1
```

Expected: PASS.

- [ ] **Step 4: Browser acceptance**

Verify in browser:

- Menu `网页应用开发` appears under `资源配置`.
- Project list loads.
- Create project works.
- IDE opens.
- File tree loads.
- File open/edit/save works.
- Runtime start/restart/stop works.
- Preview loads.
- Logs load.
- AI chat streams messages.
- Tool calls render safely.
- Cancel works.
- Import works.
- Export works.
- Refresh preserves project state.
- Cross-space project access is denied.

- [ ] **Step 5: Write runbook**

Create `docs/superpowers/runbooks/appdev-local-debug.md` with:

- Local frontend URL.
- Local backend URL.
- nuwax reference URL and test account.
- AppDev route examples.
- Runtime directory location.
- Common failure modes and recovery steps.
- Verification commands.

- [ ] **Step 6: Final no-gap review**

Before calling二期完成, confirm:

- No visible nuwax-ai AppDev parity gaps remain.
- No known AppDev P0/P1/P2 defects remain.
- No AppDev API relies on frontend-supplied owner/user/space identity.
- No raw prompt, credentials, checkpoint bytes, provider body, or object URI leaks to UI.
- AppDev is lazy-loaded.
- Existing task, workspace, system pages still open.

---

## Execution Notes

- Do not merge to `dev` during implementation unless the user explicitly asks.
- Do not mark二期完成 until Task 11 passes.
- Keep system management full parity out of this plan; it belongs to三期.
- If a nuwax-ai AppDev behavior is unclear, verify it in `http://localhost/` before implementing.
- If Coze existing architecture conflicts with direct nuwax-ai code copying, preserve Coze architecture and reproduce the user-visible behavior.

## Final acceptance record (2026-07-10)

### Scope result

- AppDev 一级菜单、懒加载路由、项目入口、创建/导入/重命名/复制/归档、IDE、
  文件编辑、开发运行时、实时预览、日志、AI 对话、原型图、模型和数据源、快照、
  构建发布、发布产物下载和源码导出均已形成真实前后端闭环。
- AppDev API 只从服务端会话获取用户身份，并在每个入口执行工作空间成员校验；
  项目存储按 `space_id/project_id` 隔离，文件与压缩包路径执行规范化和越界拦截。
- SSE 只投影审核后的展示字段；前端在运行期间每 3 秒对账服务端状态，丢失
  `prompt_end` 时会自动恢复历史、文件树、预览和待命状态，完成后停止轮询。
- AppDev 页面通过 `React.lazy` 加载；任务、工作空间、系统用户、模型配置和系统
  配置页面已做回归，不受二期路由和依赖影响。
- 三期系统管理完整视觉与功能 parity 仍明确排除在本计划之外。

### Automated verification

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/app-dev
# 6 files, 39 tests passed

npx tsc --noEmit --project tsconfig.json
# passed

cd backend
go test ./application/appdev ./infra/appdev -count=1
# passed

go test -gcflags='all=-l -N' ./api/handler/coze -run AppDev -count=1
# passed
```

### In-app browser acceptance

- Coze project list: `http://localhost:8080/space/7645565700475453440/app-dev`
- Coze IDE: `http://localhost:8080/space/7645565700475453440/app-dev/appdev_0f00f828a29419fa`
- nuwax reference: `http://localhost/space/1/page-develop` and
  `http://localhost/space/1/app-dev/3684773775216640`
- Verified create/list/open, preview/code, unsaved-file guard, save/refresh, prototype image
  upload and fallback messaging, AI execution/cancel/history, missed-SSE reconciliation,
  publish, release download, source export, and refresh recovery.
- The accepted AI change `状态对账浏览器验收通过` became visible in the iframe without a
  manual page reload; browser application-console error count was `0`.
- Regression URLs `/chats`, `/workspace`, `/system/users`, `/system/settings`, and the
  system `模型配置` page loaded real data without application-console errors.

### Remaining gaps

- Phase 2 AppDev scope: `0` known P0/P1/P2 gaps at acceptance time.
- Phase 3: full nuwax parity for system management remains a separate, explicitly deferred
  project and is not an AppDev completion gap.
