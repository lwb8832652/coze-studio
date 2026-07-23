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

/* eslint-disable @typescript-eslint/naming-convention, @typescript-eslint/consistent-type-imports -- Test doubles mirror external component exports and lazy package types. */

import {
  isValidElement,
  type ComponentProps,
  type ReactNode,
  useState,
} from 'react';

import {
  afterAll,
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

const originalActEnvironment = globalThis.IS_REACT_ACT_ENVIRONMENT;
Object.assign(globalThis, {
  IS_REACT_ACT_ENVIRONMENT: true,
});

const mockNavigate = vi.hoisted(() => vi.fn());
const mockOpenAccountSettings = vi.hoisted(() => vi.fn());
const mockOpenLogoutModal = vi.hoisted(() => vi.fn());
const mockGetSystemAdminStatus = vi.hoisted(() => vi.fn());
const mockSystemAdminState = vi.hoisted(() => ({
  isAdmin: true,
}));
const mockUserState = vi.hoisted(() => ({
  email: 'codex@example.com',
  userId: 'user-1',
}));
let mockUserSequence = 0;

vi.mock('../workspace-prototype.less', () => ({}));
vi.mock('../../pages/workbench/index.less', () => ({}));

vi.mock('react-router-dom', () => ({
  useLocation: () => ({
    pathname: '/space/space-1/chats',
    search: '',
    state: null,
  }),
  useNavigate: () => mockNavigate,
  useParams: () => ({
    space_id: 'space-1',
  }),
}));

vi.mock('@coze-arch/bot-api', async importOriginal => {
  const actual = await importOriginal<typeof import('@coze-arch/bot-api')>();
  return {
    ...actual,
    PlaygroundApi: {
      ...actual.PlaygroundApi,
      GetNoticeList: vi.fn().mockResolvedValue({
        code: 0,
        msg: 'success',
        data: { notice_list: [], next_cursor: '', has_more: false },
      }),
      GetNoticeUnreadCount: vi.fn().mockResolvedValue({
        code: 0,
        msg: 'success',
        data: { unread_count: 0 },
      }),
      NoticeMarkRead: vi.fn().mockResolvedValue({ code: 0, msg: 'success' }),
    },
  };
});

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: (
    selector: (state: {
      createSpace: ReturnType<typeof vi.fn>;
      fetchSpaces: ReturnType<typeof vi.fn>;
      setSpace: ReturnType<typeof vi.fn>;
      space: {
        id: string;
        name: string;
        role_type: number;
        space_type: number;
      };
      spaceList: unknown[];
    }) => unknown,
  ) =>
    selector({
      createSpace: vi.fn(),
      fetchSpaces: vi.fn(),
      setSpace: vi.fn(),
      space: {
        id: 'space-1',
        name: 'Codex Workspace',
        role_type: 1,
        space_type: 2,
      },
      spaceList: [],
    }),
}));

vi.mock('@coze-foundation/space-ui-base', () => ({
  WorkspaceSubMenu: ({ footer }: { footer?: ReactNode }) => (
    <aside data-testid="real-workspace-submenu-footer">{footer}</aside>
  ),
}));

vi.mock('@coze-arch/bot-hooks', () => ({
  useRouteConfig: () => ({
    subMenuKey: 'chats',
  }),
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => ({
    avatar_url: 'https://example.com/avatar.png',
    email: mockUserState.email,
    name: 'Codex 用户',
    screen_name: 'codex-user',
    user_id_str: mockUserState.userId,
  }),
  useUserLabel: () => null,
}));

vi.mock('@coze-foundation/account-adapter', () => ({
  useUserInfo: () => ({
    avatar_url: 'https://example.com/avatar.png',
    email: mockUserState.email,
    name: 'Codex 用户',
    screen_name: 'codex-user',
    user_id_str: mockUserState.userId,
  }),
}));

vi.mock('@coze-foundation/account-ui-adapter', () => ({
  useLogout: () => ({
    node: null,
    open: mockOpenLogoutModal,
  }),
}));

vi.mock('@coze-foundation/account-ui-base', () => ({
  UserInfoPanel: () => null,
  useAccountSettings: () => ({
    node: null,
    open: mockOpenAccountSettings,
  }),
}));

vi.mock('@coze-studio/open-auth', () => ({
  PatBody: () => null,
}));

vi.mock('@coze-arch/i18n', () => ({
  I18n: {
    t: (key: string) =>
      ({
        basic_log_out: '退出登录',
        menu_profile_account: '账号',
        navi_bar_account_settings: '账号设置',
        settings_api_authorization: 'API 授权',
      })[key] || key,
  },
}));

vi.mock('@coze-foundation/layout', () => ({
  GlobalLayoutAccountDropdown: ({
    children,
    menus = [],
  }: {
    children?: ReactNode;
    menus?: ReactNode[];
  }) => {
    const [visible, setVisible] = useState(false);

    return (
      <div>
        <button
          type="button"
          aria-label="账号菜单"
          aria-expanded={visible}
          onClick={() => setVisible(current => !current)}
        >
          账号
        </button>
        {visible ? (
          <div role="menu" aria-label="账号菜单选项">
            {menus.map((item, index) => {
              if (
                isValidElement(item) ||
                !item ||
                typeof item !== 'object' ||
                !('title' in item)
              ) {
                return null;
              }

              const menuItem = item as {
                dataTestId?: string;
                onClick: () => void;
                title: string;
              };

              return (
                <button
                  key={`${menuItem.title}-${index}`}
                  type="button"
                  data-testid={menuItem.dataTestId}
                  onClick={() => {
                    setVisible(false);
                    menuItem.onClick();
                  }}
                >
                  {menuItem.title}
                </button>
              );
            })}
          </div>
        ) : null}
        {children}
      </div>
    );
  },
}));

vi.mock(
  '../../../../../packages/foundation/global-adapter/src/components/account-dropdown/account-settings/index.tsx',
  () => ({
    useAccountSettings: () => ({
      node: null,
      open: mockOpenAccountSettings,
    }),
  }),
);

vi.mock(
  '../../../../../packages/foundation/global-adapter/src/components/account-dropdown/user-info-menu.tsx',
  () => ({
    UserInfoMenu: () => null,
  }),
);

vi.mock(
  '@coze-foundation/global-adapter/account-dropdown',
  async importOriginal => {
    const { AccountDropdown } =
      await importOriginal<
        typeof import('@coze-foundation/global-adapter/account-dropdown')
      >();

    return {
      AccountDropdown,
    };
  },
);

vi.mock('@coze-arch/coze-design', () => ({
  Avatar: ({ children, src }: { children?: ReactNode; src?: string }) => (
    <span data-testid="workspace-user-avatar" data-src={src}>
      {children}
    </span>
  ),
  Badge: ({ children, count }: { children?: ReactNode; count?: ReactNode }) => (
    <span>
      {children}
      {count}
    </span>
  ),
  Button: ({ children }: { children?: ReactNode }) => (
    <button type="button">{children}</button>
  ),
  Dropdown: {
    Divider: () => null,
  },
  List: () => null,
  Popover: ({ children }: { children?: ReactNode }) => <>{children}</>,
  SideSheet: ({
    children,
    visible,
  }: {
    children?: ReactNode;
    visible?: boolean;
  }) => (visible ? <aside>{children}</aside> : null),
  Toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
  Typography: {
    Text: ({ children }: { children?: ReactNode }) => <span>{children}</span>,
  },
}));

vi.mock('../../../../../packages/components/bot-icons/src/index.tsx', () => {
  const Icon = () => <span aria-hidden="true" />;

  return new Proxy(
    {},
    {
      get: (_target, property) => (property === 'then' ? undefined : Icon),
    },
  );
});

vi.mock('@coze-arch/coze-design/icons', () => {
  const Icon = () => <span aria-hidden="true" />;

  return {
    IconCozArrowDown: Icon,
    IconCozArrowLeft: Icon,
    IconCozAsynchronousTask: Icon,
    IconCozAsynchronousTaskFill: Icon,
    IconCozBell: Icon,
    IconCozBot: Icon,
    IconCozBotFill: Icon,
    IconCozClock: Icon,
    IconCozClockFill: Icon,
    IconCozCode: Icon,
    IconCozCodeFill: Icon,
    IconCozDocument: Icon,
    IconCozDownload: Icon,
    IconCozExit: Icon,
    IconCozImage: Icon,
    IconCozInfoCircle: Icon,
    IconCozKnowledge: Icon,
    IconCozKnowledgeFill: Icon,
    IconCozMore: Icon,
    IconCozLink: Icon,
    IconCozPlugin: Icon,
    IconCozPlus: Icon,
    IconCozSetting: Icon,
    IconCozSideExpand: Icon,
    IconCozSkill: Icon,
    IconCozStar: Icon,
    IconCozUpload: Icon,
    IconCozWorkflow: Icon,
    IconCozWorkspace: Icon,
    IconCozWorkspaceFill: Icon,
  };
});

vi.mock('../../pages/tools/mcp-settings-panel', () => ({
  MCP_TOOL_SETTINGS_TAB_ID: 'mcp-tools',
  MCPToolSettingsPanel: () => null,
}));

vi.mock('../../pages/tools/feishu-im-settings-panel', () => ({
  FEISHU_IM_SETTINGS_TAB_ID: 'feishu-im',
  FeishuIMSettingsPanel: () => null,
}));

vi.mock('../../pages/system/service', () => ({
  getSystemAdminStatus: mockGetSystemAdminStatus,
}));

vi.mock('../workspace-sub-menu/workspace-task-list', () => ({
  WorkspaceTaskList: () => null,
}));

vi.mock('../../pages/workbench/components/workbench-composer', () => ({
  WorkbenchComposer: () => null,
}));

vi.mock('../../pages/tasks/task-artifacts-panel', () => ({
  TaskArtifactsPanel: () => null,
}));

vi.mock('../../pages/tasks/task-runtime-doctor-section', () => ({
  TaskRuntimeDoctorSection: () => null,
}));

vi.mock('../../pages/tasks/task-memory-section', () => ({
  TaskMemorySection: () => null,
}));

vi.mock('../../pages/tasks/task-mcp-runtime-audit-section', () => ({
  TaskMCPRuntimeAuditSection: () => null,
}));

vi.mock('../../pages/tasks/task-guardrail-audit-section', () => ({
  TaskGuardrailAuditSection: () => null,
}));

import { WorkspaceSubMenu } from '../workspace-sub-menu';
import { WorkspacePageTopBar } from '../workspace-page-top-bar';
import { WorkspaceHeaderActions } from '../workspace-header-actions';
import { TaskDetailHeader } from '../../pages/tasks/task-detail-header';

interface MountedRoot {
  container: HTMLDivElement;
  root: Root;
}

const mountedRoots = new Set<MountedRoot>();

const cleanup = () => {
  for (const mountedRoot of mountedRoots) {
    act(() => {
      mountedRoot.root.unmount();
    });
    mountedRoot.container.remove();
  }
  mountedRoots.clear();
  document.body.replaceChildren();
};

afterEach(cleanup);

afterAll(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment;
});

const renderNode = async (node: ReactNode) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  const mountedRoot = { container, root };
  mountedRoots.add(mountedRoot);

  await act(async () => {
    root.render(node);
    await Promise.resolve();
    await Promise.resolve();
  });

  return {
    container,
    rerender: async (nextNode: ReactNode) => {
      await act(async () => {
        root.render(nextNode);
        await Promise.resolve();
        await Promise.resolve();
      });
    },
    unmount: () => {
      if (!mountedRoots.delete(mountedRoot)) {
        return;
      }
      act(() => {
        root.unmount();
      });
      container.remove();
    },
  };
};

const renderWorkspaceHeaderActions = (
  props: ComponentProps<typeof WorkspaceHeaderActions> = {},
) => renderNode(<WorkspaceHeaderActions {...props} />);

const findButtonByText = (container: ParentNode, text: string) =>
  Array.from(container.querySelectorAll('button')).find(
    button => button.textContent === text,
  );

const createDeferred = <T,>() => {
  let resolve: (value: T) => void = () => undefined;
  const promise = new Promise<T>(resolvePromise => {
    resolve = resolvePromise;
  });

  return {
    promise,
    resolve,
  };
};

const openHeaderAccountMenu = async (container: HTMLElement) => {
  await act(async () => {
    (
      container.querySelector(
        'button[aria-label="账号菜单"]',
      ) as HTMLButtonElement | null
    )?.click();
    await Promise.resolve();
  });
};

describe('WorkspaceHeaderActions', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    mockOpenAccountSettings.mockReset();
    mockOpenLogoutModal.mockReset();
    mockGetSystemAdminStatus.mockReset();
    mockGetSystemAdminStatus.mockImplementation(() =>
      Promise.resolve({
        is_admin: mockSystemAdminState.isAdmin,
      }),
    );
    mockSystemAdminState.isAdmin = true;
    mockUserSequence += 1;
    mockUserState.email = `codex+${mockUserSequence}@example.com`;
    mockUserState.userId = `user-${mockUserSequence}`;
  });

  it('keeps notification state controlled without creating a panel', async () => {
    const onNotificationClick = vi.fn();
    const { container, unmount } = await renderWorkspaceHeaderActions({
      onNotificationClick,
      unreadCount: 3,
    });
    const notificationButton = container.querySelector(
      'button[aria-label="通知，3 条未读"]',
    ) as HTMLButtonElement | null;

    expect(notificationButton).toBeTruthy();
    expect(container.querySelectorAll('[aria-label*="通知"]')).toHaveLength(1);
    expect(
      Array.from(container.querySelectorAll('[aria-hidden="true"]')).find(
        element => element.textContent?.trim() === '3',
      ),
    ).toBeTruthy();

    act(() => {
      notificationButton?.click();
    });

    expect(onNotificationClick).toHaveBeenCalledTimes(1);
    expect(container.querySelector('[role="dialog"]')).toBeNull();

    unmount();
  });

  it('opens the real admin workspace account menu and preserves every action', async () => {
    mockSystemAdminState.isAdmin = true;
    const { container } = await renderWorkspaceHeaderActions();

    const clickAccountItem = async (label: string) => {
      await openHeaderAccountMenu(container);
      const button = findButtonByText(container, label);
      expect(button).toBeTruthy();
      act(() => {
        button?.click();
      });
    };

    await clickAccountItem('账号设置');
    await clickAccountItem('API 授权');
    await clickAccountItem('MCP 配置');
    await clickAccountItem('IM 机器人');
    await clickAccountItem('退出登录');
    await clickAccountItem('系统管理');

    expect(mockOpenAccountSettings).toHaveBeenCalledWith('account');
    expect(mockOpenAccountSettings).toHaveBeenCalledWith('api-auth');
    expect(mockOpenAccountSettings).toHaveBeenCalledWith('mcp-tools');
    expect(mockOpenAccountSettings).toHaveBeenCalledWith('feishu-im');
    expect(mockOpenLogoutModal).toHaveBeenCalledTimes(1);
    expect(mockNavigate).toHaveBeenCalledWith('/system/overview');
  });

  it('keeps system management out of the real non-admin account menu', async () => {
    mockSystemAdminState.isAdmin = false;
    const { container } = await renderWorkspaceHeaderActions();

    await openHeaderAccountMenu(container);

    expect(findButtonByText(container, '账号设置')).toBeTruthy();
    expect(findButtonByText(container, 'API 授权')).toBeTruthy();
    expect(findButtonByText(container, 'MCP 配置')).toBeTruthy();
    expect(findButtonByText(container, 'IM 机器人')).toBeTruthy();
    expect(findButtonByText(container, '退出登录')).toBeTruthy();
    expect(findButtonByText(container, '系统管理')).toBeUndefined();
  });

  it('gives sidebar and header account menus independent triggers', async () => {
    const { container } = await renderNode(
      <>
        <WorkspaceSubMenu />
        <WorkspaceHeaderActions />
      </>,
    );
    const accountTriggers = Array.from(
      container.querySelectorAll('button[aria-label="账号菜单"]'),
    );

    expect(accountTriggers).toHaveLength(2);
    expect(mockGetSystemAdminStatus).toHaveBeenCalledTimes(1);

    await act(async () => {
      accountTriggers[1]?.click();
      await Promise.resolve();
    });

    expect(accountTriggers[0]?.getAttribute('aria-expanded')).toBe('false');
    expect(accountTriggers[1]?.getAttribute('aria-expanded')).toBe('true');
  });

  it('revalidates admin status when account menus mount sequentially', async () => {
    await renderNode(<WorkspaceSubMenu />);

    expect(mockGetSystemAdminStatus).toHaveBeenCalledTimes(1);

    await renderWorkspaceHeaderActions();

    expect(mockGetSystemAdminStatus).toHaveBeenCalledTimes(2);
  });

  it('hides stale admin state on user switch and ignores late responses', async () => {
    const staleUserRequest = createDeferred<{ is_admin: boolean }>();
    mockGetSystemAdminStatus.mockReset();
    mockGetSystemAdminStatus
      .mockResolvedValueOnce({ is_admin: true })
      .mockReturnValueOnce(staleUserRequest.promise)
      .mockResolvedValueOnce({ is_admin: false });
    mockUserState.userId = 'admin-user';

    const view = await renderWorkspaceHeaderActions();
    await openHeaderAccountMenu(view.container);

    expect(findButtonByText(view.container, '系统管理')).toBeTruthy();

    mockUserState.userId = 'stale-user';
    await view.rerender(<WorkspaceHeaderActions />);

    expect(findButtonByText(view.container, '系统管理')).toBeUndefined();

    mockUserState.userId = 'member-user';
    await view.rerender(<WorkspaceHeaderActions />);

    await act(async () => {
      staleUserRequest.resolve({ is_admin: true });
      await Promise.resolve();
    });

    expect(mockGetSystemAdminStatus).toHaveBeenCalledTimes(3);
    expect(findButtonByText(view.container, '系统管理')).toBeUndefined();
  });

  it('uses the shared actions in the workspace top bar', async () => {
    const workspaceTopBar = await renderNode(<WorkspacePageTopBar />);

    expect(
      workspaceTopBar.container.querySelector('button[aria-label^="通知"]'),
    ).toBeTruthy();
    expect(
      workspaceTopBar.container.querySelector('button[aria-label="账号菜单"]'),
    ).toBeTruthy();
    workspaceTopBar.unmount();
  });

  it('keeps the real task actions ordered and returns to the task list', async () => {
    const task = {
      id: 'task-1',
      input: '{"messages":[{"role":"user","content":"整理周报"}]}',
      result: '',
      title: '整理周报',
    } as ComponentProps<typeof TaskDetailHeader>['task'];
    const { container, unmount } = await renderNode(
      <TaskDetailHeader
        artifacts={[]}
        memoryReadOnly={false}
        messages={[]}
        spaceId="space-1"
        task={task}
        threadId="thread-1"
        tokenUsage={{
          callCount: 1,
          costMicros: 0,
          currency: 'USD',
          inputTokens: 60,
          leadAgentTokens: 100,
          middlewareTokens: 0,
          modelAttributions: [],
          outputTokens: 40,
          subagentTokens: 0,
          toolTokens: 0,
          totalTokens: 100,
        }}
        tokenUsageViewMode="summary"
      />,
    );
    const backButton = container.querySelector(
      'button[aria-label="返回全部任务"]',
    ) as HTMLButtonElement | null;
    const exportButton = container.querySelector(
      'button[aria-label="打开任务导出菜单"]',
    );
    const tokenButton = container.querySelector(
      'button[aria-label^="查看 Token 用量"]',
    );
    const actions = container.querySelector(
      '.coze-prototype-task-topbar-actions',
    );
    const detailButton = findButtonByText(container, '详情');
    const favoriteButton = findButtonByText(container, '收藏');
    const shareButton = findButtonByText(container, '分享');
    const notificationButton = actions?.querySelector(
      'button[aria-label^="通知"]',
    );
    const accountButton = actions?.querySelector(
      'button[aria-label="账号菜单"]',
    );

    expect(backButton).toBeTruthy();
    expect(tokenButton).toBeNull();
    expect(exportButton).toBeTruthy();
    expect(detailButton).toBeTruthy();
    expect(favoriteButton).toBeTruthy();
    expect(shareButton).toBeTruthy();
    expect(notificationButton).toBeTruthy();
    expect(accountButton).toBeTruthy();
    expect(
      (shareButton?.compareDocumentPosition(notificationButton as Node) ?? 0) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(
      (notificationButton?.compareDocumentPosition(accountButton as Node) ??
        0) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBe(Node.DOCUMENT_POSITION_FOLLOWING);

    act(() => {
      backButton?.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/chats');
    expect(mockNavigate).not.toHaveBeenCalledWith(-1);

    unmount();
  });
});
