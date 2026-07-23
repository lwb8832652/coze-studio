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

/* eslint-disable @typescript-eslint/no-extraneous-class, @typescript-eslint/consistent-type-imports -- Test doubles mirror browser globals and external component contracts. */

import type { ReactNode } from 'react';

import { afterAll, afterEach, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot as createReactRoot, type Root } from 'react-dom/client';
import { workbenchTask } from '@coze-studio/api-schema';

const originalActEnvironment = globalThis.IS_REACT_ACT_ENVIRONMENT;
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
const originalIntersectionObserver = globalThis.IntersectionObserver;
const testRoots = new Set<Root>();
const testListenerCleanups = new Set<() => void>();

const createRoot = (...args: Parameters<typeof createReactRoot>): Root => {
  const reactRoot = createReactRoot(...args);
  let mounted = true;
  const trackedRoot: Root = {
    render: children => {
      reactRoot.render(children);
    },
    unmount: () => {
      if (!mounted) {
        return;
      }
      mounted = false;
      testRoots.delete(trackedRoot);
      reactRoot.unmount();
    },
  };
  testRoots.add(trackedRoot);

  return trackedRoot;
};

const addTestWindowListener = (type: string, listener: EventListener) => {
  window.addEventListener(type, listener);
  testListenerCleanups.add(() => {
    window.removeEventListener(type, listener);
  });
};

const mockNavigate = vi.hoisted(() => vi.fn());
const mockListTaskThreads = vi.hoisted(() => vi.fn());
const mockUseRouteConfig = vi.hoisted(() =>
  vi.fn(() => ({ subMenuKey: 'chats/new' })),
);
const mockSetSpace = vi.hoisted(() => vi.fn());
const mockCreateSpace = vi.hoisted(() => vi.fn());
const mockFetchSpaces = vi.hoisted(() => vi.fn());
const mockGetSystemAdminStatus = vi.hoisted(() => vi.fn());
const mockUserState = vi.hoisted(() => ({
  screenName: 'wb',
  userId: 'user-1',
}));
let mockUserSequence = 0;
const mockUseSpaceStore = vi.hoisted(() =>
  vi.fn((selector: (state: unknown) => unknown) =>
    selector({
      space: {
        id: 'space-1',
        name: '畅享AI',
        role_type: 1,
        space_type: 2,
      },
      spaceList: [
        {
          id: 'space-1',
          name: '畅享AI',
          role_type: 1,
          space_type: 2,
        },
        {
          id: 'space-2',
          name: 'Personal Space',
          role_type: 1,
          space_type: 1,
        },
      ],
      setSpace: mockSetSpace,
      createSpace: mockCreateSpace,
      fetchSpaces: mockFetchSpaces,
    }),
  ),
);
const capturedWorkspaceSubMenuProps = vi.hoisted(() => ({
  current: undefined as
    | {
        menus: Array<{ label: string; path: string }>;
        header?: ReactNode;
        footer?: ReactNode;
        collapsed?: boolean;
      }
    | undefined,
}));
const capturedAccountDropdownProps = vi.hoisted(() => ({
  current: undefined as
    | {
        extraSettingsTabs?: Array<{ id: string; tabName: string } | 'divider'>;
        extraMenuItems?: Array<{
          key: string;
          title: string;
          onClick: () => void;
        }>;
      }
    | undefined,
}));

let latestIntersectionCallback:
  | ((entries: Array<Partial<IntersectionObserverEntry>>) => void)
  | undefined;

class MockIntersectionObserver {
  observe = vi.fn();
  disconnect = vi.fn();

  constructor(callback: IntersectionObserverCallback) {
    latestIntersectionCallback = entries =>
      callback(entries as IntersectionObserverEntry[], this as never);
  }
}

vi.mock('react-router-dom', () => ({
  useLocation: () => ({ pathname: '/space/space-1/chats/new', search: '' }),
  useNavigate: () => mockNavigate,
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror package component names. */
vi.mock('@coze-foundation/space-ui-base', () => ({
  WorkspaceSubMenu: (props: {
    menus: Array<{ label: string; path: string }>;
    header?: ReactNode;
    footer?: ReactNode;
    collapsed?: boolean;
  }) => {
    capturedWorkspaceSubMenuProps.current = props;

    return (
      <aside
        data-testid="workspace-sub-menu"
        data-collapsed={String(Boolean(props.collapsed))}
      >
        {props.header}
        <nav>
          {props.menus.map(item => (
            <button key={item.path} type="button">
              {item.label}
            </button>
          ))}
        </nav>
        {props.footer}
      </aside>
    );
  },
}));

vi.mock('@coze-foundation/global-adapter', () => ({
  AccountDropdown: (props: {
    extraSettingsTabs?: Array<{ id: string; tabName: string } | 'divider'>;
    extraMenuItems?: Array<{
      key: string;
      title: string;
      onClick: () => void;
    }>;
  }) => {
    capturedAccountDropdownProps.current = props;
    return <span data-testid="account-dropdown" />;
  },
}));

vi.mock('@coze-foundation/global-adapter/account-dropdown', () => ({
  AccountDropdown: (props: {
    extraSettingsTabs?: Array<{ id: string; tabName: string } | 'divider'>;
    extraMenuItems?: Array<{
      key: string;
      title: string;
      onClick: () => void;
    }>;
  }) => {
    capturedAccountDropdownProps.current = props;
    return <span data-testid="account-dropdown" />;
  },
}));

vi.mock('@coze-foundation/account-ui-adapter', () => ({
  useLogout: () => ({
    node: <span data-testid="logout-modal" />,
    open: vi.fn(),
  }),
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after package mocks. */

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: mockUseSpaceStore,
}));

vi.mock('@coze-arch/bot-hooks', () => ({
  useRouteConfig: mockUseRouteConfig,
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  getIsLogined: () => true,
  getIsSettled: () => true,
  getUserAuthInfos: () => [],
  getUserInfo: () => ({
    name: '刘文波',
    screen_name: mockUserState.screenName,
    user_id_str: mockUserState.userId,
  }),
  subscribeUserAuthInfos: () => () => undefined,
  useIsLogined: () => true,
  useIsSettled: () => true,
  useUserAuthInfo: () => null,
  useUserLabel: () => null,
  useUserInfo: () => ({
    name: '刘文波',
    screen_name: mockUserState.screenName,
    user_id_str: mockUserState.userId,
  }),
}));

vi.mock('@coze-arch/bot-api/developer_api', () => ({
  default: class {},
  SpaceType: {
    Personal: 1,
    Team: 2,
  },
}));

vi.mock('../../../pages/system/service', () => ({
  getSystemAdminStatus: mockGetSystemAdminStatus,
}));

vi.mock('../../../pages/tools/mcp-settings-panel', () => ({
  MCP_TOOL_SETTINGS_TAB_ID: 'mcp-tools',
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Mock export mirrors the package component name.
  MCPToolSettingsPanel: ({ spaceId }: { spaceId?: string }) => (
    <span data-testid="mcp-settings-panel">{spaceId}</span>
  ),
}));

vi.mock('../../../pages/tools/feishu-im-settings-panel', () => ({
  FEISHU_IM_SETTINGS_TAB_ID: 'feishu-im',
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Mock export mirrors the package component name.
  FeishuIMSettingsPanel: ({ spaceId }: { spaceId?: string }) => (
    <span data-testid="feishu-im-settings-panel">{spaceId}</span>
  ),
}));

vi.mock('../../../pages/tasks/service', () => ({
  listTaskThreads: mockListTaskThreads,
}));

vi.mock('@coze-arch/coze-design', () => {
  const loadingComponent = ({ loading }: { loading: boolean }) =>
    loading ? <span data-testid="loading" /> : null;
  const typographyText = ({
    children,
    className,
  }: {
    children?: ReactNode;
    className?: string;
  }) => <span className={className}>{children}</span>;
  const inputComponent = ({
    value,
    placeholder,
    maxLength,
    onChange,
  }: {
    value?: string;
    placeholder?: string;
    maxLength?: number;
    onChange?: (value: string) => void;
  }) => (
    <input
      value={value}
      placeholder={placeholder}
      maxLength={maxLength}
      onChange={event => onChange?.(event.target.value)}
    />
  );
  const modalComponent = ({
    visible,
    title,
    children,
    onOk,
    onCancel,
    okText,
    cancelText,
  }: {
    visible: boolean;
    title: string;
    children?: ReactNode;
    onOk?: () => void;
    onCancel?: () => void;
    okText?: string;
    cancelText?: string;
  }) =>
    visible ? (
      <div role="dialog">
        <h2>{title}</h2>
        {children}
        <button type="button" onClick={onCancel}>
          {cancelText}
        </button>
        <button type="button" onClick={onOk}>
          {okText}
        </button>
      </div>
    ) : null;

  return {
    ['Input']: inputComponent,
    ['Loading']: loadingComponent,
    ['Modal']: modalComponent,
    ['Toast']: {
      error: vi.fn(),
      success: vi.fn(),
    },
    ['Typography']: {
      Text: typographyText,
    },
  };
});

vi.mock('@coze-arch/coze-design/icons', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@coze-arch/coze-design/icons')>();
  const icon = ({ className }: { className?: string }) => (
    <span className={className} data-testid="coze-icon" />
  );

  return {
    ...actual,
    ['IconCozArrowDown']: icon,
    ['IconCozAsynchronousTask']: icon,
    ['IconCozAsynchronousTaskFill']: icon,
    ['IconCozBot']: icon,
    ['IconCozBotFill']: icon,
    ['IconCozClock']: icon,
    ['IconCozClockFill']: icon,
    ['IconCozCode']: icon,
    ['IconCozCodeFill']: icon,
    ['IconCozExit']: icon,
    ['IconCozKnowledge']: icon,
    ['IconCozKnowledgeFill']: icon,
    ['IconCozMore']: icon,
    ['IconCozPlus']: icon,
    ['IconCozSetting']: icon,
    ['IconCozSettingFill']: icon,
    ['IconCozSideExpand']: icon,
    ['IconCozSkill']: icon,
    ['IconCozWorkspace']: icon,
    ['IconCozWorkspaceFill']: icon,
  };
});

import { getWorkspaceTaskStatusMeta } from '../workspace-task-status';
import { WorkspaceTaskList } from '../workspace-task-list';
import {
  ASSISTANT_BADGE,
  ASSISTANT_LABEL,
  ACCOUNT_ACTION_ENTRIES,
  ACCOUNT_SETTINGS_ENTRY,
  PERSONAL_CENTER_ENTRY,
  SIGN_OUT_ENTRY,
  SYSTEM_MANAGEMENT_ENTRY,
  getVisibleWorkspaceMenuMeta,
  shouldShowSystemManagementEntry,
  WORKSPACE_MENU_META,
} from '../menu';
import { WorkspaceSubMenu } from '../index';

afterEach(() => {
  for (const root of [...testRoots]) {
    act(() => {
      root.unmount();
    });
  }
  testRoots.clear();
  for (const removeListener of testListenerCleanups) {
    removeListener();
  }
  testListenerCleanups.clear();
  document.body.replaceChildren();
  capturedAccountDropdownProps.current = undefined;
  capturedWorkspaceSubMenuProps.current = undefined;
  if (originalIntersectionObserver) {
    Object.defineProperty(globalThis, 'IntersectionObserver', {
      configurable: true,
      writable: true,
      value: originalIntersectionObserver,
    });
  } else {
    Reflect.deleteProperty(globalThis, 'IntersectionObserver');
  }
  vi.restoreAllMocks();
});

afterAll(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment;
});

describe('NewX AI WorkspaceSubMenu', () => {
  beforeEach(() => {
    mockUserSequence += 1;
    mockUserState.screenName = `wb-${mockUserSequence}`;
    mockUserState.userId = `user-${mockUserSequence}`;
    latestIntersectionCallback = undefined;
    mockGetSystemAdminStatus.mockResolvedValue({ is_admin: false });
    Object.defineProperty(globalThis, 'IntersectionObserver', {
      configurable: true,
      writable: true,
      value: MockIntersectionObserver,
    });
  });

  it('defines the workspace navigation structure', () => {
    const labels = WORKSPACE_MENU_META.map(item => item.label);
    const paths = WORKSPACE_MENU_META.map(item => item.path);

    expect(ASSISTANT_LABEL).toBe('专属助理');
    expect(ASSISTANT_BADGE).toBe('Beta');
    expect(labels).toEqual([
      '新建任务',
      '资源配置',
      '网页应用开发',
      '技能配置',
      '开发配置',
      '工作空间',
      '任务中心',
      '全部任务',
    ]);
    expect(WORKSPACE_MENU_META[0]).toMatchObject({
      label: '新建任务',
      path: 'chats/new',
      variant: 'primary',
    });
    expect(WORKSPACE_MENU_META.at(-1)).toMatchObject({
      label: '全部任务',
      path: 'chats',
    });
    expect(WORKSPACE_MENU_META[2]).toMatchObject({
      label: '网页应用开发',
      path: 'app-dev',
    });
    expect(WORKSPACE_MENU_META[5]).toMatchObject({
      label: '工作空间',
      path: 'workspace',
    });
    expect(paths).not.toContain('tools');
    expect(paths).toContain('task-center');
  });

  it('hides developer feature menus for every role when workspace development is disabled', () => {
    const memberLabels = getVisibleWorkspaceMenuMeta({
      allow_develop: false,
      role_type: 3,
    }).map(item => item.label);
    const adminLabels = getVisibleWorkspaceMenuMeta({
      allow_develop: false,
      role_type: 2,
    }).map(item => item.label);
    const ownerLabels = getVisibleWorkspaceMenuMeta({
      allow_develop: false,
      role_type: 1,
    }).map(item => item.label);

    expect(memberLabels).toEqual([
      '新建任务',
      '工作空间',
      '任务中心',
      '全部任务',
    ]);
    expect(adminLabels).toEqual([
      '新建任务',
      '工作空间',
      '任务中心',
      '全部任务',
    ]);
    expect(ownerLabels).toEqual([
      '新建任务',
      '工作空间',
      '任务中心',
      '全部任务',
    ]);
  });

  it('keeps workspace settings visible for personal spaces and regular team members', () => {
    const personalLabels = getVisibleWorkspaceMenuMeta({
      space_type: 1,
      role_type: 1,
    }).map(item => item.label);
    const teamMemberLabels = getVisibleWorkspaceMenuMeta({
      space_type: 2,
      role_type: 3,
      allow_develop: true,
    }).map(item => item.label);

    expect(personalLabels).toContain('工作空间');
    expect(teamMemberLabels).toContain('工作空间');
    expect(teamMemberLabels).toContain('资源配置');
    expect(teamMemberLabels).toContain('网页应用开发');
  });

  it('defines the system management quick entry outside workspace navigation', () => {
    expect(SYSTEM_MANAGEMENT_ENTRY).toMatchObject({
      label: '系统管理',
      path: '/system/overview',
    });
    expect(
      shouldShowSystemManagementEntry({
        hasUser: false,
        isSystemAdmin: true,
      }),
    ).toBe(false);
    expect(
      shouldShowSystemManagementEntry({
        hasUser: true,
        isSystemAdmin: false,
      }),
    ).toBe(false);
    expect(
      shouldShowSystemManagementEntry({
        hasUser: true,
        isSystemAdmin: true,
      }),
    ).toBe(true);
  });

  it('keeps account action metadata for the bottom-left account menu', () => {
    expect(ACCOUNT_SETTINGS_ENTRY).toMatchObject({
      label: '账号设置',
      path: '/profile',
    });
    expect(PERSONAL_CENTER_ENTRY).toMatchObject({
      label: '个人中心',
      path: '/profile',
    });
    expect(SIGN_OUT_ENTRY).toMatchObject({
      label: '退出登录',
      action: 'logout',
    });
    expect(ACCOUNT_ACTION_ENTRIES.map(item => item.label)).toEqual([
      '账号设置',
      '退出登录',
    ]);
  });

  it('keeps account actions out of the workspace switcher dropdown', async () => {
    mockNavigate.mockReset();

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceSubMenu />);
      await Promise.resolve();
    });

    const switcherButton = container.querySelector(
      '[aria-haspopup="menu"]',
    ) as HTMLButtonElement | null;

    act(() => {
      switcherButton?.click();
    });

    expect(container.textContent).toContain('创建团队空间');
    expect(container.textContent).toContain('个人空间');
    expect(container.textContent).not.toContain('Personal Space');
    expect(container.textContent).not.toContain('账号设置');
    expect(container.textContent).not.toContain('退出登录');
    expect(container.textContent).not.toContain('系统管理');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('places system management in the bottom-left account dropdown for admins', async () => {
    mockNavigate.mockReset();
    mockGetSystemAdminStatus.mockResolvedValueOnce({ is_admin: true });
    capturedAccountDropdownProps.current = undefined;

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceSubMenu />);
      await Promise.resolve();
      await Promise.resolve();
    });

    const systemManagementEntry =
      capturedAccountDropdownProps.current?.extraMenuItems?.find(
        item => item.key === 'system-management',
      );

    expect(systemManagementEntry).toMatchObject({
      title: '系统管理',
    });

    act(() => {
      systemManagementEntry?.onClick();
    });

    expect(mockNavigate).toHaveBeenCalledWith('/system/overview');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('moves MCP configuration out of primary navigation into account settings dropdown', async () => {
    mockNavigate.mockReset();
    capturedWorkspaceSubMenuProps.current = undefined;
    capturedAccountDropdownProps.current = undefined;

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceSubMenu />);
      await Promise.resolve();
    });

    expect(
      capturedWorkspaceSubMenuProps.current?.menus.map(item => item.label),
    ).toEqual([
      '新建任务',
      '资源配置',
      '网页应用开发',
      '技能配置',
      '开发配置',
      '工作空间',
      '任务中心',
      '全部任务',
    ]);
    expect(container.textContent).not.toContain('工具');
    expect(
      container.querySelector('[data-testid="workspace_settings_button"]'),
    ).toBe(null);
    expect(container.textContent).not.toContain('功能菜单');
    expect(capturedAccountDropdownProps.current?.extraSettingsTabs).toEqual([
      expect.objectContaining({
        id: 'workspace-models',
        tabName: '模型管理',
      }),
      expect.objectContaining({
        id: 'mcp-tools',
        tabName: 'MCP 配置',
      }),
      expect.objectContaining({
        id: 'feishu-im',
        tabName: 'IM 机器人',
      }),
    ]);
    expect(mockNavigate).not.toHaveBeenCalled();

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('toggles the compact sidebar from the header trigger', async () => {
    const collapseEvents: Array<CustomEvent> = [];
    const handleCollapseEvent = (event: Event) => {
      collapseEvents.push(event as CustomEvent);
    };
    addTestWindowListener(
      'coze-workspace-submenu-collapse-change',
      handleCollapseEvent,
    );

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceSubMenu />);
      await Promise.resolve();
    });

    const sidebar = container.querySelector(
      '[data-testid="workspace-sub-menu"]',
    );
    const collapseButton = container.querySelector(
      '[data-testid="workspace_sidebar_collapse_button"]',
    ) as HTMLButtonElement | null;

    expect(collapseButton).not.toBeNull();
    expect(collapseButton?.getAttribute('aria-expanded')).toBe('true');
    expect(sidebar?.getAttribute('data-collapsed')).toBe('false');
    expect(capturedWorkspaceSubMenuProps.current?.collapsed).toBe(false);

    act(() => {
      collapseButton?.click();
    });

    expect(collapseButton?.getAttribute('aria-expanded')).toBe('false');
    expect(sidebar?.getAttribute('data-collapsed')).toBe('true');
    expect(capturedWorkspaceSubMenuProps.current?.collapsed).toBe(true);
    expect(collapseEvents.at(-1)?.detail).toEqual({
      storageKey: 'workspace-submenu-width',
      collapsed: true,
    });

    act(() => {
      root?.unmount();
    });
    window.removeEventListener(
      'coze-workspace-submenu-collapse-change',
      handleCollapseEvent,
    );
    container.remove();
  });

  it('uses distinct sidebar status indicators and keeps green for completed tasks only', () => {
    const completedMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Succeeded,
    );
    const runningMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Running,
    );
    const failedMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Failed,
    );

    expect(completedMeta).toMatchObject({
      tone: 'success',
      color: '#2a9e06',
      ariaLabel: '已完成状态',
    });
    expect(runningMeta).toMatchObject({
      tone: 'running',
      color: '#2a6df4',
      ariaLabel: '运行中状态',
    });
    expect(failedMeta).toMatchObject({
      tone: 'danger',
      color: '#f54a45',
      ariaLabel: '异常状态',
    });
    expect(runningMeta.color).not.toBe(completedMeta.color);
  });

  it('maps canonical task thread statuses to sidebar status indicators', () => {
    expect(getWorkspaceTaskStatusMeta('idle')).toMatchObject({
      tone: 'waiting',
      ariaLabel: '等待状态',
    });
    expect(getWorkspaceTaskStatusMeta('completed')).toMatchObject({
      tone: 'success',
      color: '#2a9e06',
      ariaLabel: '已完成状态',
    });
    expect(getWorkspaceTaskStatusMeta('failed')).toMatchObject({
      tone: 'danger',
      color: '#f54a45',
      ariaLabel: '异常状态',
    });
    expect(getWorkspaceTaskStatusMeta('interrupted')).toMatchObject({
      tone: 'waiting',
      ariaLabel: '等待状态',
    });
  });

  it('renders recent task threads from the canonical task thread source', async () => {
    mockNavigate.mockReset();
    mockListTaskThreads.mockResolvedValue({
      data: {
        threads: [
          {
            thread_id: 'thread-1',
            legacy_task_id: 'task-legacy-1',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: '整理周报',
            status: 'completed',
            source: 'task',
            progress: 100,
            last_user_message: '汇总本周项目进展',
            last_agent_message: '已生成周报',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    expect(mockListTaskThreads).toHaveBeenCalledWith({
      space_id: 'space-1',
      page: 1,
      page_size: 20,
    });
    expect(container.textContent).toContain('整理周报');

    const taskButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('整理周报'),
    ) as HTMLButtonElement;

    act(() => {
      taskButton.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/tasks/thread-1');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('loads more recent tasks when the sidebar reaches the bottom', async () => {
    const firstPageThreads = Array.from({ length: 20 }, (_, index) => ({
      thread_id: `thread-${index + 1}`,
      legacy_task_id: '0',
      space_id: 'space-1',
      creator_id: 'user-1',
      title: `任务 ${index + 1}`,
      status: 'completed',
      source: 'task',
      progress: 100,
      last_user_message: '',
      last_agent_message: '',
      created_at: 1717000000000 - index,
      updated_at: 1717000300000 - index,
    }));
    let resolveNextPage: (
      value: Awaited<ReturnType<typeof mockListTaskThreads>>,
    ) => void = () => undefined;
    const nextPageRequest = new Promise<
      Awaited<ReturnType<typeof mockListTaskThreads>>
    >(resolve => {
      resolveNextPage = resolve;
    });
    mockListTaskThreads
      .mockResolvedValueOnce({
        data: {
          threads: firstPageThreads,
          total: 21,
        },
        code: 0,
        msg: '',
      })
      .mockReturnValueOnce(nextPageRequest);

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    expect(mockListTaskThreads).toHaveBeenCalledWith({
      space_id: 'space-1',
      page: 1,
      page_size: 20,
    });

    await act(async () => {
      latestIntersectionCallback?.([{ isIntersecting: true }]);
      await Promise.resolve();
    });

    expect(mockListTaskThreads).toHaveBeenLastCalledWith({
      space_id: 'space-1',
      page: 2,
      page_size: 20,
    });
    expect(container.textContent).toContain('加载更多任务');

    await act(async () => {
      resolveNextPage({
        data: {
          threads: [
            {
              thread_id: 'thread-21',
              legacy_task_id: '0',
              space_id: 'space-1',
              creator_id: 'user-1',
              title: '任务 21',
              status: 'completed',
              source: 'task',
              progress: 100,
              last_user_message: '',
              last_agent_message: '',
              created_at: 1716999999999,
              updated_at: 1717000299999,
            },
          ],
          total: 21,
        },
        code: 0,
        msg: '',
      });
      await nextPageRequest;
      await Promise.resolve();
    });

    expect(container.textContent).toContain('任务 21');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('uses the same compact display title for sidebar task history', async () => {
    mockListTaskThreads.mockResolvedValue({
      data: {
        threads: [
          {
            thread_id: 'thread-travel',
            legacy_task_id: '0',
            space_id: 'space-1',
            creator_id: 'user-1',
            title:
              '请生成一份《武汉3日游攻略》正式文档，包含行程概览、每日安排、预算表、注意事项，并生成一个可在产物面板预览和下载的 Markdown 或 PDF 文档。',
            status: 'completed',
            source: 'task',
            progress: 100,
            last_user_message: '请生成一份《武汉3日游攻略》正式文档',
            last_agent_message: '',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    expect(
      container.querySelector('.coze-prototype-sidebar-task-name')?.textContent,
    ).toBe('武汉3日游攻略');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('prepends a newly created task thread without waiting for a page refresh', async () => {
    mockListTaskThreads.mockResolvedValue({
      data: {
        threads: [
          {
            thread_id: 'thread-old',
            legacy_task_id: '0',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: '旧任务',
            status: 'completed',
            source: 'task',
            progress: 100,
            last_user_message: '',
            last_agent_message: '',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    act(() => {
      window.dispatchEvent(
        new CustomEvent('coze:workspace-task-thread-upsert', {
          detail: {
            space_id: 'space-1',
            thread: {
              thread_id: 'thread-new',
              legacy_task_id: '0',
              space_id: 'space-1',
              creator_id: 'user-1',
              title: '新任务',
              status: 'running',
              source: 'task',
              progress: 0,
              last_user_message: '',
              last_agent_message: '',
              created_at: 1717000400000,
              updated_at: 1717000400000,
            },
          },
        }),
      );
    });

    const taskNames = Array.from(
      container.querySelectorAll('.coze-prototype-sidebar-task-name'),
    ).map(item => item.textContent);

    expect(taskNames).toEqual(['新任务', '旧任务']);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('patches an existing recent task title without inserting a partial task', async () => {
    mockListTaskThreads.mockResolvedValue({
      data: {
        threads: [
          {
            thread_id: 'thread-travel',
            legacy_task_id: '0',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: '请生成一份《武汉3日游攻略》正式文档',
            status: 'running',
            source: 'task',
            progress: 50,
            last_user_message: '',
            last_agent_message: '',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    act(() => {
      window.dispatchEvent(
        new CustomEvent('coze:workspace-task-thread-upsert', {
          detail: {
            mode: 'patch',
            space_id: 'space-1',
            thread: {
              thread_id: 'thread-travel',
              title: '武汉3日游攻略',
            },
          },
        }),
      );
      window.dispatchEvent(
        new CustomEvent('coze:workspace-task-thread-upsert', {
          detail: {
            mode: 'patch',
            space_id: 'space-1',
            thread: {
              thread_id: 'missing-thread',
              title: '不应插入',
            },
          },
        }),
      );
    });

    const taskNames = Array.from(
      container.querySelectorAll('.coze-prototype-sidebar-task-name'),
    ).map(item => item.textContent);

    expect(taskNames).toEqual(['武汉3日游攻略']);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('keeps a generated title patch that arrives before the task list response', async () => {
    let releaseListRequest = () => undefined;
    const listRequestBarrier = new Promise<void>(resolve => {
      releaseListRequest = resolve;
    });
    mockListTaskThreads.mockImplementation(async () => {
      await listRequestBarrier;
      return {
        data: {
          threads: [
            {
              thread_id: 'thread-delayed-title',
              legacy_task_id: '0',
              space_id: 'space-1',
              creator_id: 'user-1',
              title:
                '请为一款面向中小企业的智能协作平台设计完整的季度产品发布计划',
              status: 'completed',
              source: 'task',
              progress: 100,
              last_user_message: '',
              last_agent_message: '',
              created_at: 1717000000000,
              updated_at: 1717000300000,
            },
          ],
          total: 1,
        },
        code: 0,
        msg: '',
      };
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    act(() => {
      window.dispatchEvent(
        new CustomEvent('coze:workspace-task-thread-upsert', {
          detail: {
            mode: 'patch',
            space_id: 'space-1',
            thread: {
              thread_id: 'thread-delayed-title',
              title: '中小企业智能协作平台发布计划',
              updated_at: 1717000400000,
            },
          },
        }),
      );
    });

    await act(async () => {
      releaseListRequest();
      await listRequestBarrier;
      await Promise.resolve();
    });

    expect(
      container.querySelector('.coze-prototype-sidebar-task-name')?.textContent,
    ).toBe('中小企业智能协作平台发布计划');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('opens canonical recent task thread when legacy task id is zero string', async () => {
    mockNavigate.mockReset();
    mockListTaskThreads.mockResolvedValue({
      data: {
        threads: [
          {
            thread_id: 'thread-zero-legacy',
            legacy_task_id: '0',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: 'Canonical 新建任务',
            status: 'idle',
            source: 'web',
            progress: 0,
            last_user_message: '请用一句话回复 smoke OK',
            last_agent_message: '',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    const taskButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('Canonical 新建任务'),
    ) as HTMLButtonElement;

    act(() => {
      taskButton.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/tasks/thread-zero-legacy',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
