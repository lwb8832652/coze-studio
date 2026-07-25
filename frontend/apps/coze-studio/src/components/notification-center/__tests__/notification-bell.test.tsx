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

import { type ReactNode, useState } from 'react';

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

const mockGetNoticeList = vi.hoisted(() => vi.fn());
const mockGetNoticeUnreadCount = vi.hoisted(() => vi.fn());
const mockNoticeMarkRead = vi.hoisted(() => vi.fn());
const mockNavigate = vi.hoisted(() => vi.fn());
const mockToastError = vi.hoisted(() => vi.fn());
const mockSetSpace = vi.hoisted(() => vi.fn());
const mockFetchSpaces = vi.hoisted(() => vi.fn());
const mockSpaceState = vi.hoisted(() => ({
  space: { id: '202' },
  spaceList: [{ id: '202' }, { id: '303' }],
}));

vi.mock('@coze-studio/api-schema/playground', () => ({
  GetNoticeList: mockGetNoticeList,
  GetNoticeUnreadCount: mockGetNoticeUnreadCount,
  NoticeCategory: {
    AppDev: 3,
    Billing: 8,
    IM: 7,
    MCP: 4,
    Resource: 5,
    ScheduledTask: 2,
    System: 9,
    Task: 1,
    Workspace: 6,
  },
  NoticeMarkRead: mockNoticeMarkRead,
  NoticeRankType: { All: 0, Unread: 1 },
  NoticeReadMode: { NoticeIDs: 1, Snapshot: 2 },
  NoticeRoute: {
    AppDev: 3,
    Billing: 6,
    None: 0,
    ScheduledTaskCenter: 2,
    Skill: 4,
    SystemAnnouncements: 7,
    TaskThread: 1,
    Workspace: 5,
  },
  NoticeSeverity: {
    Error: 4,
    Info: 1,
    Success: 2,
    Warning: 3,
  },
  ReadStatus: { Read: 2, Unread: 1 },
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => ({ user_id_str: 'notification-test-user' }),
}));

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: (
    selector: (state: {
      space: { id: string };
      spaceList: Array<{ id: string }>;
      setSpace: typeof mockSetSpace;
      fetchSpaces: typeof mockFetchSpaces;
    }) => unknown,
  ) =>
    selector({
      ...mockSpaceState,
      setSpace: mockSetSpace,
      fetchSpaces: mockFetchSpaces,
    }),
}));

vi.mock('@coze-arch/coze-design/icons', () => {
  const MockBellIcon = () => <span aria-hidden="true">bell</span>;
  const MockBotIcon = () => <span aria-hidden="true">bot</span>;
  const MockRefreshIcon = () => <span aria-hidden="true">refresh</span>;

  return {
    IconCozBell: MockBellIcon,
    IconCozBot: MockBotIcon,
    IconCozRefresh: MockRefreshIcon,
  };
});

vi.mock('@coze-arch/coze-design', () => {
  const MockBadge = ({
    children,
    count,
  }: {
    children?: ReactNode;
    count?: ReactNode;
  }) => (
    <span>
      {children}
      {count ? <span aria-hidden="true">{count}</span> : null}
    </span>
  );
  const MockPopover = ({
    children,
    content,
    onVisibleChange,
    visible,
  }: {
    children?: ReactNode;
    content?: ReactNode;
    onVisibleChange?: (visible: boolean) => void;
    visible?: boolean;
  }) => {
    const [, setRevision] = useState(0);
    return (
      <>
        <span
          onClick={() => {
            setRevision(current => {
              onVisibleChange?.(!visible);
              return current + 1;
            });
          }}
        >
          {children}
        </span>
        {visible ? <div role="dialog">{content}</div> : null}
      </>
    );
  };

  return {
    Badge: MockBadge,
    Popover: MockPopover,
    Toast: {
      error: mockToastError,
    },
  };
});

vi.mock('../index.less', () => ({}));

import {
  NotificationCategory,
  NotificationRoute,
  NotificationSeverity,
} from '../service';
import { getNotificationTarget, NotificationBell } from '../notification-bell';

const originalActEnvironment = globalThis.IS_REACT_ACT_ENVIRONMENT;
Object.assign(globalThis, {
  IS_REACT_ACT_ENVIRONMENT: true,
});

interface MountedRoot {
  container: HTMLDivElement;
  root: Root;
}

const mountedRoots = new Set<MountedRoot>();

const flush = async () => {
  for (let index = 0; index < 8; index += 1) {
    await Promise.resolve();
  }
};

const renderBell = async () => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.add({ container, root });

  await act(async () => {
    root.render(<NotificationBell compact />);
    await flush();
  });

  return container;
};

afterEach(() => {
  for (const mountedRoot of mountedRoots) {
    act(() => mountedRoot.root.unmount());
    mountedRoot.container.remove();
  }
  mountedRoots.clear();
  document.body.replaceChildren();
  vi.useRealTimers();
});

afterAll(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment;
});

describe('NotificationBell', () => {
  beforeEach(() => {
    mockGetNoticeList.mockReset();
    mockGetNoticeUnreadCount.mockReset();
    mockNoticeMarkRead.mockReset();
    mockNavigate.mockReset();
    mockToastError.mockReset();
    mockSetSpace.mockReset();
    mockFetchSpaces.mockReset();
    mockSpaceState.space = { id: '202' };
    mockSpaceState.spaceList = [{ id: '202' }, { id: '303' }];
    mockGetNoticeUnreadCount.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: { unread_count: 2 },
    });
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-1',
            content: 'AI Agent 学习路线已经生成',
            create_time: '1784678400000',
            read_status: 1,
            category: NotificationCategory.Task,
            severity: NotificationSeverity.Success,
            route: NotificationRoute.TaskThread,
            route_space_id: '202',
            route_target_id: 'thread-100',
            sender: {
              sender_name: 'NewX AI',
            },
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    mockNoticeMarkRead.mockResolvedValue({
      code: 0,
      msg: 'success',
    });
    mockFetchSpaces.mockResolvedValue({
      bot_space_list: mockSpaceState.spaceList,
    });
  });

  it('opens the shared notification center and keeps unread state until an explicit action', async () => {
    const container = await renderBell();
    const trigger = container.querySelector(
      'button[aria-label="通知，2 条未读"]',
    ) as HTMLButtonElement | null;

    expect(trigger).toBeTruthy();
    expect(trigger?.classList.contains('notification-bell__trigger')).toBe(
      true,
    );

    await act(async () => {
      trigger?.click();
      await flush();
    });

    expect(container.querySelector('[aria-label="通知中心"]')).toBeTruthy();
    expect(container.textContent).toContain('AI Agent 学习路线已经生成');
    expect(mockNoticeMarkRead).not.toHaveBeenCalled();

    const notice = container.querySelector(
      'button[data-notification-id="notice-1"]',
    ) as HTMLButtonElement | null;
    await act(async () => {
      notice?.click();
      await flush();
    });

    expect(mockNoticeMarkRead.mock.calls[0][0]).toEqual({
      notice_ids: ['notice-1'],
      read_mode: 1,
    });
  });

  it('closes the notification center when the bell is clicked again', async () => {
    const container = await renderBell();
    const trigger = container.querySelector(
      'button[aria-label="通知，2 条未读"]',
    ) as HTMLButtonElement | null;

    await act(async () => {
      trigger?.click();
      await flush();
    });
    expect(container.querySelector('[aria-label="通知中心"]')).toBeTruthy();

    await act(async () => {
      trigger?.click();
      await flush();
    });
    expect(container.querySelector('[aria-label="通知中心"]')).toBeNull();
  });

  it('defers popover visibility callbacks outside the popover state update', async () => {
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined);
    try {
      const container = await renderBell();
      const trigger = container.querySelector(
        'button[aria-label="通知，2 条未读"]',
      ) as HTMLButtonElement | null;

      await act(async () => {
        trigger?.click();
        await flush();
      });

      expect(
        consoleError.mock.calls
          .flat()
          .map(value => String(value))
          .join(' '),
      ).not.toContain('scheduled from inside an update function');
      expect(container.querySelector('[aria-label="通知中心"]')).toBeTruthy();
    } finally {
      consoleError.mockRestore();
    }
  });

  it('supports retry after the notification list fails', async () => {
    mockGetNoticeList
      .mockRejectedValueOnce(new Error('network error'))
      .mockResolvedValueOnce({
        code: 0,
        msg: 'success',
        data: { notice_list: [], next_cursor: '', has_more: false },
      });
    const container = await renderBell();

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(container.textContent).toContain('通知加载失败');
    const retry = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('重试'),
    );
    await act(async () => {
      retry?.click();
      await flush();
    });

    expect(mockGetNoticeList).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('暂无通知');
  });

  it('keeps a missing route retryable instead of latching the service off', async () => {
    const notFoundError = Object.assign(new Error('Request failed'), {
      response: { status: 404 },
    });
    mockGetNoticeUnreadCount.mockRejectedValueOnce(notFoundError);
    mockGetNoticeList.mockRejectedValueOnce(notFoundError);
    const container = await renderBell();
    const trigger = container.querySelector(
      'button[aria-label="通知"]',
    ) as HTMLButtonElement | null;

    await act(async () => {
      trigger?.click();
      await flush();
    });
    expect(container.textContent).toContain('通知加载失败');

    const retry = Array.from(container.querySelectorAll('button')).find(item =>
      item.textContent?.includes('重试'),
    );
    await act(async () => {
      retry?.click();
      await flush();
    });
    expect(mockGetNoticeList).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('AI Agent 学习路线已经生成');
  });

  it('constructs only server-generated structured targets', () => {
    expect(
      getNotificationTarget(
        {
          route: NotificationRoute.TaskThread,
          route_space_id: '202',
          route_target_id: 'thread-100',
        },
        '202',
      ),
    ).toBe('/space/202/tasks/thread-100');
    expect(
      getNotificationTarget(
        {
          route: NotificationRoute.TaskThread,
          route_space_id: '202',
          route_target_id: 'thread:7666445789865967616',
        },
        '202',
      ),
    ).toBe('/space/202/tasks/7666445789865967616');
    expect(
      getNotificationTarget(
        {
          route: NotificationRoute.AppDev,
          route_space_id: '202',
          route_target_id: 'app-100',
        },
        '202',
      ),
    ).toBe('/space/202/app-dev/app-100');
    expect(
      getNotificationTarget({
        route: NotificationRoute.SystemAnnouncements,
      }),
    ).toBe('/system/announcements');
    expect(
      getNotificationTarget(
        {
          route: NotificationRoute.Workspace,
          route_space_id: '202',
        },
        '202',
      ),
    ).toBe('/space/202/workspace');
    expect(
      getNotificationTarget(
        {
          route: NotificationRoute.Billing,
          route_space_id: '../system',
          route_target_id: 'attacker-path',
        },
        '202',
      ),
    ).toBe('/billing/subscriptions');
    expect(
      getNotificationTarget(
        {
          route: 999 as NotificationRoute,
          route_space_id: '202',
          route_target_id: 'thread-100',
          jump_link: 'https://attacker.example',
        },
        '202',
      ),
    ).toBe('');
    expect(
      getNotificationTarget(
        {
          route: NotificationRoute.TaskThread,
          route_space_id: '../system',
          route_target_id: 'models',
        },
        '202',
      ),
    ).toBe('');
    expect(
      getNotificationTarget(
        {
          route: NotificationRoute.TaskThread,
          route_space_id: '303',
          route_target_id: 'thread-100',
        },
        '202',
      ),
    ).toBe('');
  });

  it('verifies and switches workspace for a structured workspace route', async () => {
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-internal-space',
            content: '工作空间公告',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.Workspace,
            route_space_id: '303',
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-internal-space"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockFetchSpaces).toHaveBeenCalledWith(true);
    expect(mockSetSpace).toHaveBeenCalledWith('303');
    expect(mockNoticeMarkRead).toHaveBeenCalledTimes(1);
    expect(mockNavigate).toHaveBeenCalledWith('/space/303/workspace');
  });

  it('marks read before rejecting a workspace route after membership was revoked', async () => {
    mockFetchSpaces.mockResolvedValue({
      bot_space_list: [{ id: '202' }],
    });
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-internal-revoked',
            content: '已撤销空间公告',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.Workspace,
            route_space_id: '404',
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-internal-revoked"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockFetchSpaces).toHaveBeenCalledWith(true);
    expect(mockSetSpace).not.toHaveBeenCalled();
    expect(mockNoticeMarkRead).toHaveBeenCalledTimes(1);
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(mockToastError).toHaveBeenCalledWith({
      content: '你已不在该通知对应的工作空间',
    });
  });

  it('switches only to a server-listed workspace before navigating', async () => {
    const callOrder: string[] = [];
    let resolveMarkRead:
      | ((value: { code: number; msg: string }) => void)
      | undefined;
    const markReadResponse = new Promise<{
      code: number;
      msg: string;
    }>(resolve => {
      resolveMarkRead = resolve;
    });
    mockNoticeMarkRead.mockImplementation(() => {
      callOrder.push('mark-read');
      return markReadResponse;
    });
    mockSetSpace.mockImplementation(() => {
      callOrder.push('set-space');
    });
    mockNavigate.mockImplementation(() => {
      callOrder.push('navigate');
    });
    mockFetchSpaces.mockResolvedValue({
      bot_space_list: [{ id: '202' }, { id: '303' }],
    });
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-cross-space',
            content: '跨空间任务已完成',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.TaskThread,
            route_space_id: '303',
            route_target_id: 'thread-303',
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-cross-space"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockFetchSpaces).toHaveBeenCalledWith(true);
    expect(mockSetSpace).toHaveBeenCalledWith('303');
    expect(mockNavigate).toHaveBeenCalledWith('/space/303/tasks/thread-303');
    expect(mockNoticeMarkRead).toHaveBeenCalledTimes(1);
    expect(callOrder).toEqual(['mark-read', 'set-space', 'navigate']);

    await act(async () => {
      resolveMarkRead?.({ code: 0, msg: 'success' });
      await flush();
    });
  });

  it('refreshes membership before navigating inside the active workspace', async () => {
    const callOrder: string[] = [];
    mockNoticeMarkRead.mockImplementation(() => {
      callOrder.push('mark-read');
      return { code: 0, msg: 'success' };
    });
    mockNavigate.mockImplementation(() => {
      callOrder.push('navigate');
    });
    mockFetchSpaces.mockResolvedValue({
      bot_space_list: [{ id: '202' }, { id: '303' }],
    });
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-same-space',
            content: '当前空间任务已完成',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.TaskThread,
            route_space_id: '202',
            route_target_id: 'thread-202',
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-same-space"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockFetchSpaces).toHaveBeenCalledWith(true);
    expect(mockSetSpace).not.toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith('/space/202/tasks/thread-202');
    expect(callOrder).toEqual(['mark-read', 'navigate']);
  });

  it('marks read even when switching the verified workspace fails', async () => {
    mockFetchSpaces.mockResolvedValue({
      bot_space_list: [{ id: '202' }, { id: '303' }],
    });
    mockSetSpace.mockImplementation(() => {
      throw new Error('switch failed');
    });
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-switch-failed',
            content: '跨空间任务已完成',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.TaskThread,
            route_space_id: '303',
            route_target_id: 'thread-303',
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-switch-failed"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockFetchSpaces).toHaveBeenCalledWith(true);
    expect(mockSetSpace).toHaveBeenCalledWith('303');
    expect(mockNoticeMarkRead).toHaveBeenCalledTimes(1);
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(mockToastError).toHaveBeenCalledWith({
      content: '切换目标工作空间失败，请稍后重试',
    });
  });

  it('does not navigate when workspace membership cannot be verified', async () => {
    mockSpaceState.spaceList = [{ id: '202' }, { id: '404' }];
    mockFetchSpaces.mockResolvedValue({
      bot_space_list: [{ id: '202' }],
    });
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-forbidden-space',
            content: '不可访问的空间任务',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.TaskThread,
            route_space_id: '404',
            route_target_id: 'thread-404',
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-forbidden-space"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockFetchSpaces).toHaveBeenCalledWith(true);
    expect(mockSetSpace).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(mockNoticeMarkRead).toHaveBeenCalledTimes(1);
    expect(mockToastError).toHaveBeenCalledWith({
      content: '你已不在该通知对应的工作空间',
    });
  });

  it('marks a notification without a target and does not navigate', async () => {
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-no-target',
            content: '系统提示',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.None,
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-no-target"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockNoticeMarkRead).toHaveBeenCalledTimes(1);
    expect(mockFetchSpaces).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('reports mark-read failure without bypassing safe navigation', async () => {
    mockNoticeMarkRead.mockRejectedValue(new Error('storage unavailable'));
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-mark-failed',
            content: '系统公告',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.SystemAnnouncements,
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-mark-failed"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockNavigate).toHaveBeenCalledWith('/system/announcements');
    expect(mockToastError).toHaveBeenCalledWith({
      content: '标记通知已读失败，请稍后重试',
    });
  });

  it('marks a validated non-space route before navigating', async () => {
    const callOrder: string[] = [];
    mockNoticeMarkRead.mockImplementation(() => {
      callOrder.push('mark-read');
      return { code: 0, msg: 'success' };
    });
    mockNavigate.mockImplementation(() => {
      callOrder.push('navigate');
    });
    mockGetNoticeList.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        notice_list: [
          {
            id: 'notice-billing',
            content: '订阅状态已更新',
            create_time: '1784678400000',
            read_status: 1,
            route: NotificationRoute.Billing,
          },
        ],
        next_cursor: '',
        has_more: false,
      },
    });
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });
    await act(async () => {
      (
        container.querySelector(
          'button[data-notification-id="notice-billing"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(mockFetchSpaces).not.toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith('/billing/subscriptions');
    expect(callOrder).toEqual(['mark-read', 'navigate']);
  });

  it('shows restrained severity styling and unread polling errors', async () => {
    mockGetNoticeUnreadCount.mockRejectedValue(new Error('temporary'));
    const container = await renderBell();
    await act(async () => {
      (
        container.querySelector(
          'button[aria-label^="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(container.textContent).toContain('未读通知数量加载失败');
    expect(container.querySelector('[data-severity="success"]')).toBeTruthy();
  });
});
