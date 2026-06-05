# Coze Studio Figma UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the Coze Studio workspace sidebar and task/skill pages to match the approved Figma prototypes while preserving existing resource library and project development pages.

**Architecture:** Keep generic workspace shell behavior in `@coze-foundation/space-ui-base`, but move Coze Studio-specific sidebar composition into the app package so task APIs do not leak into foundation packages. Add a task detail route under `/space/:space_id/tasks/:task_id` and reuse the existing workbench task service layer.

**Tech Stack:** React 18, TypeScript, react-router-dom v6, Vitest, `@coze-arch/coze-design`, existing Semi/Coze token classes, Tailwind utilities, `@coze-studio/api-schema`.

---

## File Structure

- Modify `frontend/packages/foundation/space-ui-base/src/components/workspace-sub-menu/index.tsx`
  - Add a generic `bottomPanel?: ReactNode` prop.
  - Default to the current `FavoritesList` when no slot is provided.
- Modify `frontend/packages/foundation/space-ui-base/src/components/workspace-sub-menu/components/workspace-list-item.tsx`
  - Add optional visual variants for the primary "新建任务" item.
- Modify `frontend/packages/foundation/space-ui-base/src/components/workspace-sub-menu/components/workspace-list.tsx`
  - Forward the new item variant without changing default adapter behavior.
- Create `frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`
  - App-specific sidebar composition with Figma labels and "我的任务".
- Create `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx`
  - Recent task list backed by `listTasks`.
- Modify `frontend/apps/coze-studio/src/routes/async-components.tsx`
  - Load the local app sidebar and task detail page.
- Modify `frontend/apps/coze-studio/src/routes/index.tsx`
  - Add `/space/:space_id/tasks/:task_id` with `TASKS` active.
- Create `frontend/apps/coze-studio/src/pages/tasks/helpers.ts`
  - Shared task status text, action visibility, time, tone, filtering helpers.
- Modify `frontend/apps/coze-studio/src/pages/tasks/index.tsx`
  - Figma-style all tasks list with search/filter/tabs and row navigation.
- Create `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
  - Task execution/detail page.
- Modify `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`
  - Move helper imports and add list rendering tests.
- Create `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
  - Detail page loading and data rendering tests.
- Modify `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
  - Figma-style new task composer, extension selector, and template cards.
- Modify `frontend/apps/coze-studio/src/pages/workbench/index.less`
  - Updated Figma-aligned layout and responsive behavior.
- Modify `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`
  - Update expected first screen and preserve send behavior assertions.
- Modify `frontend/apps/coze-studio/src/pages/skill/index.tsx`
  - Figma-style skill configuration view with tabs/search/create/import behavior.
- Modify `frontend/apps/coze-studio/src/pages/skill/__tests__/skill.test.tsx`
  - Add search/create expectations while preserving stale-output behavior.

## Task 1: Base Sidebar Slot And Item Variant

**Files:**
- Modify: `frontend/packages/foundation/space-ui-base/src/components/workspace-sub-menu/index.tsx`
- Modify: `frontend/packages/foundation/space-ui-base/src/components/workspace-sub-menu/components/workspace-list-item.tsx`
- Modify: `frontend/packages/foundation/space-ui-base/src/components/workspace-sub-menu/components/workspace-list.tsx`

- [ ] **Step 1: Write the failing test by adding a local render smoke test**

Create `frontend/packages/foundation/space-ui-base/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`:

```tsx
/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { renderToStaticMarkup } from 'react-dom/server';
import { vi } from 'vitest';

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: (selector: (state: unknown) => unknown) =>
    selector({
      inited: true,
      loading: false,
      spaceList: [{ id: 'space-1' }],
    }),
}));

vi.mock('@coze-arch/coze-design', () => ({
  Skeleton: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  Space: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock('./components/favorites-list', () => ({
  FavoritesList: () => <div>Favourites fallback</div>,
}));

import { WorkspaceSubMenu } from '../index';

describe('WorkspaceSubMenu', () => {
  it('renders a custom bottom panel when provided', () => {
    const markup = renderToStaticMarkup(
      <WorkspaceSubMenu
        header={<div>Header</div>}
        menus={[]}
        bottomPanel={<div>我的任务</div>}
      />,
    );

    expect(markup).toContain('我的任务');
    expect(markup).not.toContain('Favourites fallback');
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
cd frontend/packages/foundation/space-ui-base
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: fail because `WorkspaceSubMenu` does not accept or render `bottomPanel`.

- [ ] **Step 3: Implement the minimal base slot and primary variant**

Update `IWorkspaceListItem`:

```ts
export interface IWorkspaceListItem {
  icon?: ReactNode;
  activeIcon?: ReactNode;
  title?: () => string;
  path?: string;
  dataTestId?: string;
  variant?: 'default' | 'primary';
}
```

Update `WorkspaceSubMenu` props and bottom content:

```tsx
interface IWorkspaceSubMenuProps {
  header: ReactNode;
  menus: Array<IWorkspaceListItem>;
  currentSubMenu?: string;
  bottomPanel?: ReactNode;
}

export const WorkspaceSubMenu = ({
  header,
  menus,
  currentSubMenu,
  bottomPanel,
}: IWorkspaceSubMenuProps) => {
  // existing store code remains
  const lowerPanel = bottomPanel ?? <FavoritesList />;

  return (
    <Skeleton loading={loading} active placeholder={<Skeleton.Paragraph />}>
      <Space spacing={4} vertical className="w-full h-full">
        <div className="flex-none w-full">{header}</div>
        {hasSpace ? (
          <>
            <div className="flex-none w-full">
              <WorkspaceList menus={menus} currentSubMenu={currentSubMenu} />
            </div>
            <div className="flex-grow max-h-full overflow-y-auto w-full mt-[24px]">
              {lowerPanel}
            </div>
          </>
        ) : null}
      </Space>
    </Skeleton>
  );
};
```

Add primary item classes in `WorkspaceListItem`:

```tsx
const isActive = path === currentSubMenu;
const isPrimary = variant === 'primary';

className={classNames(
  'flex items-center gap-[8px] transition-colors rounded-[8px] h-[32px] w-full px-[8px] cursor-pointer group',
  {
    'coz-bg-primary coz-fg-plus': isActive && !isPrimary,
    'coz-fg-primary hover:coz-mg-secondary-hovered': !isActive && !isPrimary,
    'bg-[#1f1f26] text-white hover:bg-[#34343d]': isPrimary,
  },
)}
```

- [ ] **Step 4: Run the test and verify it passes**

Run:

```bash
cd frontend/packages/foundation/space-ui-base
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: pass.

## Task 2: App Workspace Sidebar And Recent Tasks

**Files:**
- Create: `frontend/apps/coze-studio/src/components/workspace-sub-menu/index.tsx`
- Create: `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`

- [ ] **Step 1: Write the failing test**

Create `frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`:

```tsx
/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

import { renderToStaticMarkup } from 'react-dom/server';
import { vi } from 'vitest';

vi.mock('@coze-arch/bot-hooks', () => ({
  useRouteConfig: () => ({ subMenuKey: 'workbench' }),
}));

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: (selector: (state: unknown) => unknown) =>
    selector({
      space: { id: 'space-1', name: '刘文波 的工作空间', icon_url: '' },
    }),
}));

vi.mock('@coze-foundation/space-ui-base', () => ({
  WorkspaceSubMenu: ({ header, menus, bottomPanel }: any) => (
    <aside>
      {header}
      {menus.map((item: any) => (
        <div key={item.path} data-variant={item.variant}>
          {item.title()}
        </div>
      ))}
      {bottomPanel}
    </aside>
  ),
}));

vi.mock('../workspace-task-list', () => ({
  WorkspaceTaskList: () => <section>我的任务</section>,
}));

vi.mock('@coze-arch/coze-design', () => ({
  Avatar: () => <span />,
  Space: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Typography: {
    Text: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
  },
}));

vi.mock('@coze-arch/coze-design/icons', () => new Proxy({}, { get: () => () => <span /> }));

import { WorkspaceSubMenu } from '../index';

describe('Coze Studio WorkspaceSubMenu', () => {
  it('renders the Figma workspace navigation structure', () => {
    const markup = renderToStaticMarkup(<WorkspaceSubMenu />);

    expect(markup).toContain('专属助理');
    expect(markup).toContain('Beta');
    expect(markup).toContain('新建任务');
    expect(markup).toContain('资源配置');
    expect(markup).toContain('技能配置');
    expect(markup).toContain('开发配置');
    expect(markup).toContain('任务触发器');
    expect(markup).toContain('全部任务');
    expect(markup).toContain('我的任务');
    expect(markup).toContain('data-variant="primary"');
  });
});
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: fail because the local sidebar component does not exist.

- [ ] **Step 3: Implement the app sidebar**

Create `index.tsx` with:

```tsx
import { WorkspaceSubMenu as BaseWorkspaceSubMenu } from '@coze-foundation/space-ui-base';
import { useSpaceStore } from '@coze-foundation/space-store';
import { useRouteConfig } from '@coze-arch/bot-hooks';
import { Avatar, Space, Typography } from '@coze-arch/coze-design';
import {
  IconCozAsynchronousTask,
  IconCozAsynchronousTaskFill,
  IconCozBot,
  IconCozBotFill,
  IconCozCode,
  IconCozCodeFill,
  IconCozKnowledge,
  IconCozKnowledgeFill,
  IconCozSetting,
  IconCozSettingFill,
  IconCozTrigger,
} from '@coze-arch/coze-design/icons';
import { SpaceSubModuleEnum } from '@coze-foundation/space-ui-adapter';

import { WorkspaceTaskList } from './workspace-task-list';

export const WorkspaceSubMenu = () => {
  const { subMenuKey } = useRouteConfig();
  const currentSpace = useSpaceStore(state => state.space);

  const menus = [
    {
      icon: <IconCozBot />,
      activeIcon: <IconCozBotFill />,
      title: () => '新建任务',
      path: SpaceSubModuleEnum.WORKBENCH,
      dataTestId: 'navigation_workspace_new_task',
      variant: 'primary' as const,
    },
    {
      icon: <IconCozKnowledge />,
      activeIcon: <IconCozKnowledgeFill />,
      title: () => '资源配置',
      path: SpaceSubModuleEnum.LIBRARY,
      dataTestId: 'navigation_workspace_library',
    },
    {
      icon: <IconCozSetting />,
      activeIcon: <IconCozSettingFill />,
      title: () => '技能配置',
      path: SpaceSubModuleEnum.SKILL,
      dataTestId: 'navigation_workspace_skill',
    },
    {
      icon: <IconCozCode />,
      activeIcon: <IconCozCodeFill />,
      title: () => '开发配置',
      path: SpaceSubModuleEnum.DEVELOP,
      dataTestId: 'navigation_workspace_develop',
    },
    {
      icon: <IconCozTrigger />,
      activeIcon: <IconCozTrigger />,
      title: () => '任务触发器',
      path: SpaceSubModuleEnum.TASK_TRIGGER,
      dataTestId: 'navigation_workspace_task_trigger',
    },
    {
      icon: <IconCozAsynchronousTask />,
      activeIcon: <IconCozAsynchronousTaskFill />,
      title: () => '全部任务',
      path: SpaceSubModuleEnum.TASKS,
      dataTestId: 'navigation_workspace_tasks',
    },
  ];

  const header = (
    <div className="w-full">
      <Space className="h-[48px] px-[8px] w-full rounded-[8px] hover:coz-mg-secondary-hovered" spacing={8}>
        <Avatar className="w-[24px] h-[24px] rounded-[6px] shrink-0" src={currentSpace?.icon_url} />
        <Typography.Text ellipsis={{ showTooltip: true, rows: 1 }} className="flex-1 coz-fg-primary text-[14px] font-[500]">
          {currentSpace?.name || ''}
        </Typography.Text>
      </Space>
      <div className="mt-[8px] flex items-center justify-between rounded-[8px] border border-solid coz-stroke-primary px-[10px] py-[8px] text-[13px] leading-[20px] coz-bg-plus">
        <span className="font-[500] coz-fg-primary">专属助理</span>
        <span className="rounded-[6px] bg-[#e8fff4] px-[6px] text-[12px] font-[600] text-[#0a8f5a]">Beta</span>
      </div>
    </div>
  );

  return (
    <BaseWorkspaceSubMenu
      header={header}
      menus={menus}
      currentSubMenu={subMenuKey}
      bottomPanel={<WorkspaceTaskList />}
    />
  );
};
```

- [ ] **Step 4: Implement recent task list**

Create `workspace-task-list.tsx`:

```tsx
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useSpaceStore } from '@coze-foundation/space-store';
import { Loading } from '@coze-arch/coze-design';
import { workbenchTask } from '@coze-studio/api-schema';

import { listTasks } from '../../pages/tasks/service';
import { formatUpdatedTime, getTaskStatusText } from '../../pages/tasks/helpers';

type ChatTask = workbenchTask.ChatTask;

export const WorkspaceTaskList = () => {
  const navigate = useNavigate();
  const spaceId = useSpaceStore(state => state.space.id);
  const [tasks, setTasks] = useState<ChatTask[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!spaceId) {
      return;
    }

    let canceled = false;

    const load = async () => {
      setLoading(true);
      try {
        const response = await listTasks({ space_id: spaceId, page_size: 8 });
        if (!canceled) {
          setTasks(response.data?.tasks ?? []);
        }
      } catch {
        if (!canceled) {
          setTasks([]);
        }
      } finally {
        if (!canceled) {
          setLoading(false);
        }
      }
    };

    void load();

    return () => {
      canceled = true;
    };
  }, [spaceId]);

  return (
    <section className="w-full h-full flex flex-col" aria-label="我的任务">
      <div className="flex h-[24px] items-center justify-between pl-[8px] pr-[4px] mb-[4px]">
        <h2 className="m-0 text-[14px] leading-[20px] font-[600] coz-fg-secondary">我的任务</h2>
        <button
          type="button"
          className="border-0 bg-transparent text-[12px] leading-[18px] coz-fg-secondary cursor-pointer"
          onClick={() => spaceId && navigate(`/space/${spaceId}/tasks`)}
        >
          查看全部
        </button>
      </div>
      {loading ? (
        <div className="flex h-[120px] items-center justify-center">
          <Loading loading={true} size="mini" />
        </div>
      ) : null}
      {!loading && tasks.length === 0 ? (
        <div className="px-[8px] py-[12px] text-[13px] leading-[20px] coz-fg-dim">暂无任务</div>
      ) : null}
      <div className="grid gap-[4px]">
        {tasks.map(task => (
          <button
            key={task.id}
            type="button"
            className="min-w-0 rounded-[8px] border-0 bg-transparent px-[8px] py-[8px] text-left cursor-pointer hover:coz-mg-secondary-hovered"
            onClick={() => spaceId && navigate(`/space/${spaceId}/tasks/${task.id}`)}
          >
            <div className="truncate text-[13px] leading-[20px] font-[500] coz-fg-primary">{task.title}</div>
            <div className="mt-[2px] flex items-center justify-between gap-[8px] text-[12px] leading-[18px] coz-fg-secondary">
              <span>{getTaskStatusText(task.status)}</span>
              <span className="truncate">{formatUpdatedTime(task.updated_at)}</span>
            </div>
          </button>
        ))}
      </div>
    </section>
  );
};
```

- [ ] **Step 5: Point the app sidebar lazy import at the local component**

In `frontend/apps/coze-studio/src/routes/async-components.tsx`:

```ts
export const spaceSubMenu = lazy(() => import('../components/workspace-sub-menu'));
```

- [ ] **Step 6: Run tests and verify green**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: pass.

## Task 3: Shared Task Helpers And Detail Route

**Files:**
- Create: `frontend/apps/coze-studio/src/pages/tasks/helpers.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tasks/detail.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/async-components.tsx`
- Modify: `frontend/apps/coze-studio/src/routes/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`

- [ ] **Step 1: Write the failing helper and detail tests**

Update `tasks.test.tsx` imports:

```ts
import {
  canCancelTask,
  filterTasks,
  formatUpdatedTime,
  getTaskStatusText,
} from '../helpers';
```

Add:

```ts
it('filters tasks by title and status', () => {
  const tasks = [
    { id: '1', title: '生成周报', status: workbenchTask.TaskStatus.Running },
    { id: '2', title: '排查错误', status: workbenchTask.TaskStatus.Failed },
  ] as workbenchTask.ChatTask[];

  expect(filterTasks(tasks, '周报', 'all')).toHaveLength(1);
  expect(filterTasks(tasks, '', 'failed')).toHaveLength(1);
  expect(getTaskStatusText(workbenchTask.TaskStatus.Succeeded)).toBe('已完成');
});
```

Create `task-detail.test.tsx` with a mocked `getTask` and `listTaskEvents`, then assert the detail page renders title, input, result, event payload, and progress.

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: fail because `helpers.ts` and `detail.tsx` do not exist.

- [ ] **Step 3: Create task helpers**

Create `helpers.ts`:

```ts
import { workbenchTask } from '@coze-studio/api-schema';

type ChatTask = workbenchTask.ChatTask;

export type TaskStatusFilter = 'all' | 'running' | 'succeeded' | 'failed';

export const getTaskStatusText = (status: workbenchTask.TaskStatus) => {
  const statusMap: Record<workbenchTask.TaskStatus, string> = {
    [workbenchTask.TaskStatus.Created]: '已创建',
    [workbenchTask.TaskStatus.Queued]: '排队中',
    [workbenchTask.TaskStatus.Running]: '运行中',
    [workbenchTask.TaskStatus.Succeeded]: '已完成',
    [workbenchTask.TaskStatus.Failed]: '失败',
    [workbenchTask.TaskStatus.Canceling]: '取消中',
    [workbenchTask.TaskStatus.Canceled]: '已取消',
  };

  return statusMap[status] ?? '未知';
};

export const getTaskStatusTone = (status: workbenchTask.TaskStatus) => {
  if (status === workbenchTask.TaskStatus.Running || status === workbenchTask.TaskStatus.Queued) {
    return 'running';
  }
  if (status === workbenchTask.TaskStatus.Succeeded) {
    return 'success';
  }
  if (status === workbenchTask.TaskStatus.Failed || status === workbenchTask.TaskStatus.Canceled) {
    return 'danger';
  }
  return 'neutral';
};

export const canCancelTask = (status: workbenchTask.TaskStatus) =>
  status === workbenchTask.TaskStatus.Created ||
  status === workbenchTask.TaskStatus.Queued ||
  status === workbenchTask.TaskStatus.Running;

export const canRetryTask = (status: workbenchTask.TaskStatus) =>
  status === workbenchTask.TaskStatus.Failed;

export const formatUpdatedTime = (timestamp: number) => {
  if (!timestamp) {
    return '-';
  }

  return new Date(timestamp).toLocaleString();
};

export const filterTasks = (
  tasks: ChatTask[],
  keyword: string,
  statusFilter: TaskStatusFilter,
) => {
  const normalizedKeyword = keyword.trim().toLowerCase();

  return tasks.filter(task => {
    const matchesKeyword = normalizedKeyword
      ? task.title.toLowerCase().includes(normalizedKeyword) ||
        task.input?.toLowerCase().includes(normalizedKeyword)
      : true;
    const matchesStatus =
      statusFilter === 'all' ||
      (statusFilter === 'running' &&
        [workbenchTask.TaskStatus.Created, workbenchTask.TaskStatus.Queued, workbenchTask.TaskStatus.Running].includes(task.status)) ||
      (statusFilter === 'succeeded' && task.status === workbenchTask.TaskStatus.Succeeded) ||
      (statusFilter === 'failed' &&
        [workbenchTask.TaskStatus.Failed, workbenchTask.TaskStatus.Canceled].includes(task.status));

    return matchesKeyword && matchesStatus;
  });
};
```

- [ ] **Step 4: Add task detail page and route**

Add a lazy export:

```ts
export const TaskDetailPage = lazy(() => import('../pages/tasks/detail'));
```

Add route before the plain `tasks` route:

```tsx
{
  path: 'tasks/:task_id',
  Component: TaskDetailPage,
  loader: () => ({
    subMenuKey: SpaceSubModuleEnum.TASKS,
  }),
},
```

Implement `detail.tsx` using `getTask({ task_id })`, `listTaskEvents({ task_id })`, `getTaskStatusText`, `formatUpdatedTime`, and existing loading/error state patterns.

- [ ] **Step 5: Run tests and verify green**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx
```

Expected: pass.

## Task 4: Figma-Style All Tasks Page

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tasks/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/tasks.test.tsx`

- [ ] **Step 1: Write the failing page-rendering test**

Add a test that mocks `listTasks`, renders `TasksPage`, and asserts:

```ts
expect(container.textContent).toContain('全部任务');
expect(container.textContent).toContain('跨任务追踪执行状态、产物和历史记录');
expect(container.textContent).toContain('搜索任务');
expect(container.textContent).toContain('已收藏');
expect(container.textContent).toContain('生成周报');
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx
```

Expected: fail because the subtitle/search/favorite UI does not exist.

- [ ] **Step 3: Implement the redesigned all tasks list**

Use existing services and helpers. Add local state:

```ts
const [keyword, setKeyword] = useState('');
const [statusFilter, setStatusFilter] = useState<TaskStatusFilter>('all');
const [view, setView] = useState<'all' | 'favorite'>('all');
const [favoriteTaskIds, setFavoriteTaskIds] = useState<string[]>([]);
const visibleTasks = filterTasks(tasks, keyword, statusFilter).filter(task =>
  view === 'favorite' ? favoriteTaskIds.includes(task.id) : true,
);
```

Render toolbar controls, task rows, local favorite toggles, and navigate rows to `/space/${space_id}/tasks/${task.id}`.

- [ ] **Step 4: Run tests and verify green**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/tasks.test.tsx
```

Expected: pass.

## Task 5: Figma-Style Workbench

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.less`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`

- [ ] **Step 1: Write the failing first-screen test update**

Update the first-screen test to assert these Figma controls:

```ts
expect(markup).toContain('选择扩展');
expect(markup).toContain('研究分析');
expect(markup).toContain('生成报告');
expect(markup).toContain('整理知识库');
expect(markup).toContain('aria-label="发送任务"');
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/workbench/__tests__/workbench.test.tsx
```

Expected: fail because the extension and card copy does not exist.

- [ ] **Step 3: Implement redesigned composer**

Keep `mapModeToChatMode` and `sendWorkbenchChat` untouched. Replace simple tags with template cards, add extension selector state, add compact icon buttons with accessible labels, and keep the send button disabled until a message exists.

- [ ] **Step 4: Run tests and verify green**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/workbench/__tests__/workbench.test.tsx
```

Expected: pass.

## Task 6: Figma-Style Skill Configuration

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/skill/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/skill/__tests__/skill.test.tsx`

- [ ] **Step 1: Write failing search/create UI test**

Add a test that renders `SkillPage` and asserts:

```ts
expect(container.textContent).toContain('技能配置');
expect(container.textContent).toContain('管理任务执行时可调用的工具、脚本和流程');
expect(container.textContent).toContain('创建技能');
expect(container.querySelector('input[aria-label="搜索技能"]')).toBeTruthy();
```

- [ ] **Step 2: Run the test and verify it fails**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/skill/__tests__/skill.test.tsx
```

Expected: fail because search/create UI does not exist.

- [ ] **Step 3: Implement redesigned skill page**

Keep `listSkills`, `importSkill`, and `testRunSkill`. Add:

```ts
const [keyword, setKeyword] = useState('');
const [activeType, setActiveType] = useState<'all' | 'script' | 'workflow'>('all');
const [showImportPanel, setShowImportPanel] = useState(false);
const visibleSkills = skills.filter(skill => {
  const matchesKeyword = keyword.trim()
    ? skill.name.toLowerCase().includes(keyword.trim().toLowerCase()) ||
      skill.description?.toLowerCase().includes(keyword.trim().toLowerCase())
    : true;
  const matchesType =
    activeType === 'all' ||
    (activeType === 'script' && skill.type === workbenchSkill.SkillType.Script) ||
    (activeType === 'workflow' && skill.type === workbenchSkill.SkillType.Workflow);

  return matchesKeyword && matchesType;
});
```

Render Figma-style header, tabs, search, create/import toggle, and redesigned rows.

- [ ] **Step 4: Run tests and verify green**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/skill/__tests__/skill.test.tsx
```

Expected: pass.

## Task 7: Full Verification

**Files:**
- All touched files from Tasks 1-6.

- [ ] **Step 1: Run focused package tests**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/workbench/__tests__/workbench.test.tsx src/pages/tasks/__tests__/tasks.test.tsx src/pages/tasks/__tests__/task-detail.test.tsx src/pages/skill/__tests__/skill.test.tsx src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: pass.

- [ ] **Step 2: Run base sidebar test**

Run:

```bash
cd frontend/packages/foundation/space-ui-base
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx
```

Expected: pass.

- [ ] **Step 3: Run app lint or note blockers**

Run:

```bash
cd frontend/apps/coze-studio
npm run lint
```

Expected: pass, or document pre-existing/tooling blockers exactly.

- [ ] **Step 4: Inspect final diff**

Run:

```bash
git diff --stat
git diff -- frontend/apps/coze-studio frontend/packages/foundation/space-ui-base
```

Expected: diff is limited to the approved UI redesign scope.

## Self-Review

- Spec coverage: sidebar structure, resource/develop reuse, new task, all tasks, task detail, skill configuration, Semi guidance, and testing are covered by Tasks 1-7.
- Placeholder scan: no unresolved placeholder steps remain.
- Type consistency: task helpers use `workbenchTask.ChatTask`, `TaskStatus`, and `TaskEvent` from the generated API schema.
- Boundary check: Coze Studio task fetching lives under `frontend/apps/coze-studio`, while `space-ui-base` only receives a generic slot.
