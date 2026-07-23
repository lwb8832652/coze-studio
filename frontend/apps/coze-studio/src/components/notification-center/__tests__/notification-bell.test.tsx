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

import { type ReactNode } from 'react';

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

vi.mock('@coze-arch/bot-api', () => ({
  PlaygroundApi: {
    GetNoticeList: mockGetNoticeList,
    GetNoticeUnreadCount: mockGetNoticeUnreadCount,
    NoticeMarkRead: mockNoticeMarkRead,
  },
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
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
  }) => (
    <>
      <span onClick={() => onVisibleChange?.(!visible)}>{children}</span>
      {visible ? <div role="dialog">{content}</div> : null}
    </>
  );

  return {
    Badge: MockBadge,
    Popover: MockPopover,
    Toast: {
      error: mockToastError,
    },
  };
});

vi.mock('../index.less', () => ({}));

import { NotificationBell } from '../notification-bell';

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
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
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

    expect(mockNoticeMarkRead).toHaveBeenCalledWith({
      notice_ids: ['notice-1'],
      mark_all: false,
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

  it('shows a stable empty state when the community backend has no notice route', async () => {
    const notFoundError = Object.assign(new Error('Request failed'), {
      response: { status: 404 },
    });
    mockGetNoticeUnreadCount.mockRejectedValueOnce(notFoundError);
    mockGetNoticeList.mockRejectedValueOnce(notFoundError);
    const container = await renderBell();

    await act(async () => {
      (
        container.querySelector(
          'button[aria-label="通知"]',
        ) as HTMLButtonElement | null
      )?.click();
      await flush();
    });

    expect(container.textContent).toContain('暂无通知');
    expect(container.textContent).not.toContain('通知加载失败');
  });
});
