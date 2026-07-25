// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockListAdminAnnouncements = vi.hoisted(() => vi.fn());
const mockCreateAdminAnnouncement = vi.hoisted(() => vi.fn());
const mockUpdateAdminAnnouncement = vi.hoisted(() => vi.fn());
const mockScheduleAdminAnnouncement = vi.hoisted(() => vi.fn());
const mockPublishAdminAnnouncement = vi.hoisted(() => vi.fn());
const mockCancelAdminAnnouncement = vi.hoisted(() => vi.fn());
const mockReplayAdminAnnouncements = vi.hoisted(() => vi.fn());
const mockListAdminAnnouncementAuditEvents = vi.hoisted(() => vi.fn());
const mockListAdminUsers = vi.hoisted(() => vi.fn());
const mockListAdminWorkspaces = vi.hoisted(() => vi.fn());

vi.mock('../../service', () => ({
  AdminAnnouncementRouteType: {
    None: 0,
    SystemAnnouncements: 2,
    WorkspaceHome: 1,
  },
  cancelAdminAnnouncement: mockCancelAdminAnnouncement,
  createAdminAnnouncement: mockCreateAdminAnnouncement,
  listAdminAnnouncementAuditEvents: mockListAdminAnnouncementAuditEvents,
  listAdminAnnouncements: mockListAdminAnnouncements,
  listAdminUsers: mockListAdminUsers,
  listAdminWorkspaces: mockListAdminWorkspaces,
  publishAdminAnnouncement: mockPublishAdminAnnouncement,
  replayAdminAnnouncements: mockReplayAdminAnnouncements,
  scheduleAdminAnnouncement: mockScheduleAdminAnnouncement,
  updateAdminAnnouncement: mockUpdateAdminAnnouncement,
}));

import { AnnouncementsSection } from '../index';

const draftAnnouncement = {
  audience: {
    target_ids: [],
    type: 'all' as const,
  },
  body: '系统将在今晚进行维护。',
  created_at: '2026-07-25T01:00:00Z',
  created_by: '100',
  id: 'announcement-1',
  route: { type: 2 as const },
  projected_count: 0,
  projection_status: 'idle' as const,
  recipient_count: 0,
  severity: 'info' as const,
  status: 'draft' as const,
  title: '系统维护公告',
  updated_at: '2026-07-25T01:00:00Z',
  updated_by: '100',
  version: 1,
};

describe('AnnouncementsSection', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);

    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [],
      total: 0,
    });
    mockCreateAdminAnnouncement.mockResolvedValue({
      announcement: draftAnnouncement,
      replayed: false,
    });
    mockUpdateAdminAnnouncement.mockResolvedValue({
      announcement: {
        ...draftAnnouncement,
        version: 2,
      },
    });
    mockScheduleAdminAnnouncement.mockResolvedValue({
      announcement: {
        ...draftAnnouncement,
        scheduled_at: '2099-01-01T02:00:00Z',
        status: 'scheduled',
        version: 2,
      },
    });
    mockPublishAdminAnnouncement.mockResolvedValue({
      announcement: {
        ...draftAnnouncement,
        projection_status: 'projecting',
        published_at: '2026-07-25T02:00:00Z',
        status: 'published',
        version: 2,
      },
      deferred: true,
      error_code: 'projection_pending',
      replayed: false,
    });
    mockCancelAdminAnnouncement.mockResolvedValue({
      announcement: {
        ...draftAnnouncement,
        status: 'cancelled',
        version: 2,
      },
    });
    mockReplayAdminAnnouncements.mockResolvedValue({
      completed: 1,
      deferred: 0,
      failed: 0,
      processed: 1,
    });
    mockListAdminAnnouncementAuditEvents.mockResolvedValue({
      audit_events: [],
      total: 0,
    });
    mockListAdminUsers.mockResolvedValue({ total: 0, users: [] });
    mockListAdminWorkspaces.mockResolvedValue({
      total: 0,
      workspaces: [],
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.clearAllMocks();
  });

  const flush = async () => {
    await act(async () => {
      for (let index = 0; index < 8; index += 1) {
        await Promise.resolve();
      }
    });
  };

  const renderSection = async () => {
    act(() => root.render(<AnnouncementsSection />));
    await flush();
  };

  const button = (name: string) => {
    const target = Array.from(container.querySelectorAll('button')).find(item =>
      item.textContent?.includes(name),
    );
    expect(target).toBeTruthy();
    return target as HTMLButtonElement;
  };

  const click = async (target: HTMLElement) => {
    await act(async () => {
      target.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      await Promise.resolve();
      await Promise.resolve();
    });
  };

  const change = async (
    target: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement,
    value: string,
  ) => {
    await act(async () => {
      const prototype =
        target instanceof HTMLInputElement
          ? HTMLInputElement.prototype
          : target instanceof HTMLSelectElement
            ? HTMLSelectElement.prototype
            : HTMLTextAreaElement.prototype;
      const valueSetter = Object.getOwnPropertyDescriptor(
        prototype,
        'value',
      )?.set;
      if (!valueSetter) {
        throw new Error('native value setter is unavailable');
      }
      valueSetter.call(target, value);
      target.dispatchEvent(new Event('input', { bubbles: true }));
      target.dispatchEvent(new Event('change', { bubbles: true }));
      await Promise.resolve();
    });
  };

  it('shows loading and then the empty state', async () => {
    let resolveList:
      | ((value: { announcements: never[]; total: number }) => void)
      | undefined;
    mockListAdminAnnouncements.mockReturnValueOnce(
      new Promise(resolve => {
        resolveList = resolve;
      }),
    );

    act(() => root.render(<AnnouncementsSection />));
    expect(container.querySelector('[role="status"]')?.textContent).toContain(
      '正在加载公告',
    );

    resolveList?.({ announcements: [], total: 0 });
    await flush();
    expect(container.textContent).toContain('暂无公告');
  });

  it('shows infrastructure and permission failures without inventing details', async () => {
    mockListAdminAnnouncements.mockRejectedValueOnce({});
    await renderSection();
    expect(container.textContent).toContain('加载公告失败，请稍后重试');

    act(() => root.unmount());
    container.remove();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    mockListAdminAnnouncements.mockRejectedValueOnce(
      new Error('system administrator permission is required'),
    );

    await renderSection();
    expect(container.textContent).toContain(
      'system administrator permission is required',
    );
  });

  it('filters the durable announcement list by lifecycle status', async () => {
    await renderSection();

    const filter = container.querySelector(
      '[aria-label="公告状态筛选"]',
    ) as HTMLSelectElement;
    await change(filter, 'published');

    expect(mockListAdminAnnouncements).toHaveBeenLastCalledWith({
      limit: 50,
      status: 'published',
    });
  });

  it('ignores an older list success that resolves after the latest filter request', async () => {
    const latestAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-latest',
      status: 'published' as const,
      title: '最新筛选结果',
    };
    let resolveOlder:
      | ((value: {
          announcements: Array<typeof draftAnnouncement>;
          total: number;
        }) => void)
      | undefined;
    let resolveLatest:
      | ((value: {
          announcements: Array<typeof latestAnnouncement>;
          total: number;
        }) => void)
      | undefined;
    mockListAdminAnnouncements
      .mockReturnValueOnce(
        new Promise(resolve => {
          resolveOlder = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise(resolve => {
          resolveLatest = resolve;
        }),
      );

    act(() => root.render(<AnnouncementsSection />));
    await flush();
    const filter = container.querySelector(
      '[aria-label="公告状态筛选"]',
    ) as HTMLSelectElement;
    await change(filter, 'published');
    resolveLatest?.({
      announcements: [latestAnnouncement],
      total: 1,
    });
    await flush();
    expect(container.textContent).toContain('最新筛选结果');

    resolveOlder?.({
      announcements: [draftAnnouncement],
      total: 1,
    });
    await flush();
    expect(container.textContent).toContain('最新筛选结果');
    expect(container.textContent).not.toContain('系统维护公告');
  });

  it('keeps the latest list loading state when an older request rejects first', async () => {
    const latestAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-latest',
      status: 'published' as const,
      title: '最新请求完成',
    };
    let rejectOlder: ((reason?: unknown) => void) | undefined;
    let resolveLatest:
      | ((value: {
          announcements: Array<typeof latestAnnouncement>;
          total: number;
        }) => void)
      | undefined;
    mockListAdminAnnouncements
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectOlder = reject;
        }),
      )
      .mockReturnValueOnce(
        new Promise(resolve => {
          resolveLatest = resolve;
        }),
      );

    act(() => root.render(<AnnouncementsSection />));
    await flush();
    const filter = container.querySelector(
      '[aria-label="公告状态筛选"]',
    ) as HTMLSelectElement;
    await change(filter, 'published');
    rejectOlder?.(new Error('旧请求失败'));
    await flush();
    expect(container.textContent).toContain('正在加载公告');
    expect(container.textContent).not.toContain('旧请求失败');

    resolveLatest?.({
      announcements: [latestAnnouncement],
      total: 1,
    });
    await flush();
    expect(container.textContent).not.toContain('正在加载公告');
    expect(container.textContent).toContain('最新请求完成');
  });

  it('creates a validated draft without client-supplied actor or recipients', async () => {
    await renderSection();

    await change(
      container.querySelector('[aria-label="公告标题"]') as HTMLInputElement,
      '发布窗口提醒',
    );
    await change(
      container.querySelector('[aria-label="公告正文"]') as HTMLTextAreaElement,
      '请在发布窗口结束后刷新页面。',
    );
    await change(
      container.querySelector(
        '[aria-label="公告跳转类型"]',
      ) as HTMLSelectElement,
      '2',
    );
    await click(button('保存草稿'));

    expect(mockCreateAdminAnnouncement).toHaveBeenCalledWith(
      {
        audience: { target_ids: [], type: 'all' },
        body: '请在发布窗口结束后刷新页面。',
        route: {
          space_id: undefined,
          type: 2,
        },
        severity: 'info',
        title: '发布窗口提醒',
      },
      expect.any(String),
    );
    const payload = mockCreateAdminAnnouncement.mock.calls[0][0];
    expect(payload).not.toHaveProperty('actor_id');
    expect(payload).not.toHaveProperty('recipient_ids');
  });

  it('saves dirty form values before scheduling with the returned version', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    await renderSection();
    await click(button('查看'));
    await change(
      container.querySelector('[aria-label="公告标题"]') as HTMLInputElement,
      '计划发布前保存的标题',
    );
    await change(
      container.querySelector(
        '[aria-label="公告计划发布时间"]',
      ) as HTMLInputElement,
      '2099-01-01T10:00',
    );
    await click(button('计划发布'));

    expect(mockUpdateAdminAnnouncement).toHaveBeenCalledWith(
      'announcement-1',
      1,
      expect.objectContaining({
        title: '计划发布前保存的标题',
      }),
    );
    expect(mockScheduleAdminAnnouncement).toHaveBeenCalledWith(
      'announcement-1',
      2,
      expect.stringMatching(/^2099-01-01T/),
    );
    expect(
      mockUpdateAdminAnnouncement.mock.invocationCallOrder[0],
    ).toBeLessThan(mockScheduleAdminAnnouncement.mock.invocationCallOrder[0]);
  });

  it('saves dirty form values before publishing with the returned version', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    await renderSection();
    await click(button('查看'));
    await change(
      container.querySelector('[aria-label="公告正文"]') as HTMLTextAreaElement,
      '发布前必须持久化的正文。',
    );
    await click(button('立即发布'));

    expect(mockUpdateAdminAnnouncement).toHaveBeenCalledWith(
      'announcement-1',
      1,
      expect.objectContaining({
        body: '发布前必须持久化的正文。',
      }),
    );
    expect(mockPublishAdminAnnouncement).toHaveBeenCalledWith(
      'announcement-1',
      2,
      expect.any(String),
    );
    expect(
      mockUpdateAdminAnnouncement.mock.invocationCallOrder[0],
    ).toBeLessThan(mockPublishAdminAnnouncement.mock.invocationCallOrder[0]);
  });

  it('stops publish when saving the dirty form fails', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    mockUpdateAdminAnnouncement.mockRejectedValueOnce(
      new Error('草稿保存失败'),
    );
    await renderSection();
    await click(button('查看'));
    await change(
      container.querySelector('[aria-label="公告正文"]') as HTMLTextAreaElement,
      '尚未保存的正文。',
    );
    await click(button('立即发布'));

    expect(mockPublishAdminAnnouncement).not.toHaveBeenCalled();
    expect(container.textContent).toContain('草稿保存失败');
  });

  it('keeps editor switching disabled until an in-flight save settles', async () => {
    const secondAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-2',
      title: '第二条公告',
    };
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement, secondAnnouncement],
      total: 2,
    });
    let resolveSave:
      | ((value: { announcement: typeof draftAnnouncement }) => void)
      | undefined;
    mockUpdateAdminAnnouncement.mockReturnValueOnce(
      new Promise(resolve => {
        resolveSave = resolve;
      }),
    );
    await renderSection();
    const viewButtons = Array.from(container.querySelectorAll('button')).filter(
      item => item.textContent?.includes('查看'),
    ) as HTMLButtonElement[];
    await click(viewButtons[0]);
    await change(
      container.querySelector('[aria-label="公告标题"]') as HTMLInputElement,
      'A 的本地修改',
    );
    await click(button('保存修改'));

    expect(button('新建草稿').disabled).toBe(true);
    expect(viewButtons[1].disabled).toBe(true);
    await click(button('新建草稿'));
    await click(viewButtons[1]);
    expect(container.textContent).toContain('公告 ID announcement-1');

    resolveSave?.({
      announcement: {
        ...draftAnnouncement,
        title: 'A 的已保存结果',
        version: 2,
      },
    });
    await flush();

    expect(container.textContent).toContain('公告 ID announcement-1');
    expect(button('新建草稿').disabled).toBe(false);
    expect(viewButtons[1].disabled).toBe(false);
  });

  it('waits for the preceding save before publishing', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    let resolveSave:
      | ((value: { announcement: typeof draftAnnouncement }) => void)
      | undefined;
    mockUpdateAdminAnnouncement.mockReturnValueOnce(
      new Promise(resolve => {
        resolveSave = resolve;
      }),
    );
    await renderSection();
    await click(button('查看'));
    await change(
      container.querySelector('[aria-label="公告正文"]') as HTMLTextAreaElement,
      '发布前等待保存的正文。',
    );
    await click(button('立即发布'));

    expect(mockPublishAdminAnnouncement).not.toHaveBeenCalled();
    resolveSave?.({
      announcement: {
        ...draftAnnouncement,
        body: '发布前等待保存的正文。',
        version: 2,
      },
    });
    await flush();

    expect(mockPublishAdminAnnouncement).toHaveBeenCalledWith(
      draftAnnouncement.id,
      2,
      expect.any(String),
    );
  });

  it('keeps editor switching disabled until a successful publish settles', async () => {
    const secondAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-2',
      title: '第二条公告',
    };
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement, secondAnnouncement],
      total: 2,
    });
    let resolvePublish:
      | ((value: {
          announcement: Record<string, unknown>;
          deferred: boolean;
          replayed: boolean;
        }) => void)
      | undefined;
    mockPublishAdminAnnouncement.mockReturnValueOnce(
      new Promise(resolve => {
        resolvePublish = resolve;
      }),
    );
    await renderSection();
    const viewButtons = Array.from(container.querySelectorAll('button')).filter(
      item => item.textContent?.includes('查看'),
    ) as HTMLButtonElement[];
    await click(viewButtons[0]);
    await change(
      container.querySelector('[aria-label="公告正文"]') as HTMLTextAreaElement,
      '发布请求前已保存的正文。',
    );
    await click(button('立即发布'));
    await flush();

    expect(mockPublishAdminAnnouncement).toHaveBeenCalledWith(
      draftAnnouncement.id,
      2,
      expect.any(String),
    );
    expect(viewButtons[1].disabled).toBe(true);
    await click(viewButtons[1]);
    expect(container.textContent).toContain('公告 ID announcement-1');

    resolvePublish?.({
      announcement: {
        ...draftAnnouncement,
        projection_status: 'projecting',
        status: 'published',
        title: 'A 的迟到发布结果',
        version: 3,
      },
      deferred: true,
      replayed: false,
    });
    await flush();

    expect(container.textContent).toContain('公告 ID announcement-1');
    expect(container.textContent).toContain('公告已发布');
    const currentViewButtons = Array.from(
      container.querySelectorAll('button'),
    ).filter(item => item.textContent?.includes('查看')) as HTMLButtonElement[];
    expect(currentViewButtons[1].disabled).toBe(false);
  });

  it('keeps editor switching disabled until a rejected publish settles', async () => {
    const secondAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-2',
      title: '第二条公告',
    };
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement, secondAnnouncement],
      total: 2,
    });
    let rejectPublish: ((reason?: unknown) => void) | undefined;
    mockPublishAdminAnnouncement.mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectPublish = reject;
      }),
    );
    await renderSection();
    const viewButtons = Array.from(container.querySelectorAll('button')).filter(
      item => item.textContent?.includes('查看'),
    ) as HTMLButtonElement[];
    await click(viewButtons[0]);
    await change(
      container.querySelector('[aria-label="公告正文"]') as HTMLTextAreaElement,
      '发布请求前已保存的正文。',
    );
    await click(button('立即发布'));
    await flush();

    expect(mockPublishAdminAnnouncement).toHaveBeenCalledWith(
      draftAnnouncement.id,
      2,
      expect.any(String),
    );
    expect(viewButtons[1].disabled).toBe(true);
    await click(viewButtons[1]);
    expect(container.textContent).toContain('公告 ID announcement-1');

    rejectPublish?.(new Error('A 发布失败'));
    await flush();

    expect(container.textContent).toContain('公告 ID announcement-1');
    expect(
      container.querySelector('[data-feedback-tone="error"]'),
    ).not.toBeNull();
    const currentViewButtons = Array.from(
      container.querySelectorAll('button'),
    ).filter(item => item.textContent?.includes('查看')) as HTMLButtonElement[];
    expect(currentViewButtons[1].disabled).toBe(false);
  });

  it('does not let a slow refresh for announcement A replace selected announcement B', async () => {
    const secondAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-2',
      title: '第二条公告',
    };
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement, secondAnnouncement],
      total: 2,
    });
    await renderSection();
    const viewButtons = Array.from(container.querySelectorAll('button')).filter(
      item => item.textContent?.includes('查看'),
    );
    await click(viewButtons[0] as HTMLButtonElement);

    let resolveRefresh:
      | ((value: {
          announcements: Array<typeof draftAnnouncement>;
          total: number;
        }) => void)
      | undefined;
    mockListAdminAnnouncements.mockReturnValueOnce(
      new Promise(resolve => {
        resolveRefresh = resolve;
      }),
    );
    await click(button('刷新'));
    await click(viewButtons[1] as HTMLButtonElement);

    resolveRefresh?.({
      announcements: [
        {
          ...draftAnnouncement,
          title: '服务端更新后的第一条公告',
          version: 2,
        },
        secondAnnouncement,
      ],
      total: 2,
    });
    await flush();

    expect(
      (container.querySelector('[aria-label="公告标题"]') as HTMLInputElement)
        .value,
    ).toBe('第二条公告');
    expect(container.textContent).toContain('公告 ID announcement-2');
    expect(container.textContent).not.toContain('公告 ID announcement-1，');
  });

  it('keeps the dirty editor base version when the list refreshes', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    await renderSection();
    await click(button('查看'));
    await change(
      container.querySelector('[aria-label="公告标题"]') as HTMLInputElement,
      '本地尚未保存的标题',
    );
    mockListAdminAnnouncements.mockResolvedValueOnce({
      announcements: [
        {
          ...draftAnnouncement,
          title: '其他管理员保存的标题',
          version: 2,
        },
      ],
      total: 1,
    });

    await click(button('刷新'));
    expect(
      (container.querySelector('[aria-label="公告标题"]') as HTMLInputElement)
        .value,
    ).toBe('本地尚未保存的标题');
    await click(button('保存修改'));

    expect(mockUpdateAdminAnnouncement).toHaveBeenCalledWith(
      draftAnnouncement.id,
      1,
      expect.objectContaining({ title: '本地尚未保存的标题' }),
    );
  });

  it('atomically hydrates every editor field from a clean refresh', async () => {
    const refreshedAnnouncement = {
      ...draftAnnouncement,
      audience: {
        target_ids: ['200'],
        type: 'users' as const,
      },
      body: '服务端刷新的正文。',
      route: {
        space_id: '200',
        type: 1 as const,
      },
      scheduled_at: '2099-01-02T03:04:00Z',
      severity: 'warning' as const,
      title: '服务端刷新的标题',
      version: 2,
    };
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    await renderSection();
    await click(button('查看'));
    mockListAdminAnnouncements.mockResolvedValueOnce({
      announcements: [refreshedAnnouncement],
      total: 1,
    });

    await click(button('刷新'));

    expect(
      (container.querySelector('[aria-label="公告标题"]') as HTMLInputElement)
        .value,
    ).toBe(refreshedAnnouncement.title);
    expect(
      (
        container.querySelector(
          '[aria-label="公告正文"]',
        ) as HTMLTextAreaElement
      ).value,
    ).toBe(refreshedAnnouncement.body);
    expect(
      (container.querySelector('[aria-label="公告级别"]') as HTMLSelectElement)
        .value,
    ).toBe('warning');
    expect(
      (
        container.querySelector(
          '[aria-label="公告跳转类型"]',
        ) as HTMLSelectElement
      ).value,
    ).toBe('1');
    expect(
      (
        container.querySelector(
          '[aria-label="公告跳转工作空间"]',
        ) as HTMLInputElement
      ).value,
    ).toBe(refreshedAnnouncement.route.space_id);
    expect(
      (
        container.querySelector(
          '[aria-label="公告目标受众"]',
        ) as HTMLSelectElement
      ).value,
    ).toBe('users');
    expect(container.textContent).toContain('已选择 1 项');
    const scheduleDate = new Date(refreshedAnnouncement.scheduled_at);
    const expectedSchedule = new Date(
      scheduleDate.getTime() - scheduleDate.getTimezoneOffset() * 60_000,
    )
      .toISOString()
      .slice(0, 16);
    expect(
      (
        container.querySelector(
          '[aria-label="公告计划发布时间"]',
        ) as HTMLInputElement
      ).value,
    ).toBe(expectedSchedule);

    await click(button('保存修改'));
    expect(mockUpdateAdminAnnouncement).toHaveBeenCalledWith(
      draftAnnouncement.id,
      2,
      expect.objectContaining({
        audience: {
          target_ids: ['200'],
          type: 'users',
        },
        body: refreshedAnnouncement.body,
        title: refreshedAnnouncement.title,
      }),
    );
  });

  it('surfaces a 409 when a dirty save uses its unchanged base version', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    await renderSection();
    await click(button('查看'));
    await change(
      container.querySelector('[aria-label="公告正文"]') as HTMLTextAreaElement,
      '基于版本一编辑的本地正文。',
    );
    mockListAdminAnnouncements.mockResolvedValueOnce({
      announcements: [
        {
          ...draftAnnouncement,
          body: '其他管理员已经保存的正文。',
          version: 2,
        },
      ],
      total: 1,
    });
    await click(button('刷新'));
    mockUpdateAdminAnnouncement.mockRejectedValueOnce(
      Object.assign(new Error('公告版本冲突，请刷新后重试'), {
        status: 409,
      }),
    );

    await click(button('保存修改'));

    expect(mockUpdateAdminAnnouncement).toHaveBeenCalledWith(
      draftAnnouncement.id,
      1,
      expect.objectContaining({ body: '基于版本一编辑的本地正文。' }),
    );
    expect(container.textContent).toContain('公告版本冲突，请刷新后重试');
  });

  it('clears the previous audit immediately while the next audit is pending or fails', async () => {
    const secondAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-2',
      title: '第二条公告',
    };
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement, secondAnnouncement],
      total: 2,
    });
    let rejectSecond: ((reason?: unknown) => void) | undefined;
    mockListAdminAnnouncementAuditEvents.mockImplementation(
      (announcementID: string) => {
        if (announcementID === draftAnnouncement.id) {
          return Promise.resolve({
            audit_events: [
              {
                action: 'updated',
                actor_id: '111',
                announcement_id: draftAnnouncement.id,
                created_at: '2026-07-25T01:00:00Z',
                id: 'audit-1',
                projected_count: 0,
                projection_status: 'idle',
                recipient_count: 0,
                result: 'succeeded',
              },
            ],
            total: 1,
          });
        }
        return new Promise((_resolve, reject) => {
          rejectSecond = reject;
        });
      },
    );
    await renderSection();
    const viewButtons = Array.from(container.querySelectorAll('button')).filter(
      item => item.textContent?.includes('查看'),
    );
    await click(viewButtons[0] as HTMLButtonElement);
    expect(container.textContent).toContain('actor 111');

    await click(viewButtons[1] as HTMLButtonElement);
    expect(container.textContent).not.toContain('actor 111');
    expect(container.textContent).toContain('正在加载审计记录');

    rejectSecond?.(new Error('audit unavailable'));
    await flush();
    expect(container.textContent).not.toContain('actor 111');
    expect(container.textContent).toContain('加载公告审计记录失败');
    expect(
      Array.from(container.querySelectorAll('[role="status"]')).some(item =>
        item.textContent?.includes('正在加载审计记录'),
      ),
    ).toBe(false);

    await click(viewButtons[0] as HTMLButtonElement);
    expect(container.textContent).toContain('actor 111');
    expect(container.textContent).not.toContain('加载公告审计记录失败');
    expect(container.textContent).not.toContain('正在加载审计记录');
  });

  it('keeps the selected announcement audit when an older request resolves late', async () => {
    const secondAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-2',
      title: '第二条公告',
    };
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement, secondAnnouncement],
      total: 2,
    });
    let resolveFirst:
      | ((value: {
          audit_events: Array<Record<string, unknown>>;
          total: number;
        }) => void)
      | undefined;
    let resolveSecond:
      | ((value: {
          audit_events: Array<Record<string, unknown>>;
          total: number;
        }) => void)
      | undefined;
    mockListAdminAnnouncementAuditEvents.mockImplementation(
      (announcementID: string) =>
        new Promise(resolve => {
          if (announcementID === draftAnnouncement.id) {
            resolveFirst = resolve;
          } else {
            resolveSecond = resolve;
          }
        }),
    );
    await renderSection();
    const viewButtons = Array.from(container.querySelectorAll('button')).filter(
      item => item.textContent?.includes('查看'),
    );
    expect(viewButtons).toHaveLength(2);
    await click(viewButtons[0] as HTMLButtonElement);
    await click(viewButtons[1] as HTMLButtonElement);

    resolveSecond?.({
      audit_events: [
        {
          action: 'updated',
          actor_id: '222',
          announcement_id: secondAnnouncement.id,
          created_at: '2026-07-25T02:00:00Z',
          id: 'audit-2',
          projected_count: 0,
          projection_status: 'idle',
          recipient_count: 0,
          result: 'succeeded',
        },
      ],
      total: 1,
    });
    await flush();
    expect(container.textContent).toContain('actor 222');

    resolveFirst?.({
      audit_events: [
        {
          action: 'updated',
          actor_id: '111',
          announcement_id: draftAnnouncement.id,
          created_at: '2026-07-25T01:00:00Z',
          id: 'audit-1',
          projected_count: 0,
          projection_status: 'idle',
          recipient_count: 0,
          result: 'succeeded',
        },
      ],
      total: 1,
    });
    await flush();
    expect(container.textContent).toContain('actor 222');
    expect(container.textContent).not.toContain('actor 111');
  });

  it('keeps editor switching disabled until replay settles', async () => {
    const secondAnnouncement = {
      ...draftAnnouncement,
      id: 'announcement-2',
      title: '第二条公告',
    };
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement, secondAnnouncement],
      total: 2,
    });
    let resolveReplay:
      | ((value: {
          completed: number;
          deferred: number;
          failed: number;
          processed: number;
        }) => void)
      | undefined;
    mockReplayAdminAnnouncements.mockReturnValueOnce(
      new Promise(resolve => {
        resolveReplay = resolve;
      }),
    );
    mockListAdminAnnouncementAuditEvents.mockImplementation(
      (announcementID: string) => {
        if (announcementID === draftAnnouncement.id) {
          return Promise.resolve({
            audit_events: [
              {
                action: 'updated',
                actor_id: '111',
                announcement_id: draftAnnouncement.id,
                created_at: '2026-07-25T01:00:00Z',
                id: 'audit-1',
                projected_count: 0,
                projection_status: 'idle',
                recipient_count: 0,
                result: 'succeeded',
              },
            ],
            total: 1,
          });
        }
        return Promise.resolve({ audit_events: [], total: 0 });
      },
    );
    await renderSection();
    const viewButtons = Array.from(container.querySelectorAll('button')).filter(
      item => item.textContent?.includes('查看'),
    ) as HTMLButtonElement[];
    await click(viewButtons[0]);
    await click(button('重放待处理'));

    expect(viewButtons[1].disabled).toBe(true);
    await click(viewButtons[1]);
    expect(container.textContent).toContain('公告 ID announcement-1');
    expect(container.textContent).toContain('actor 111');

    resolveReplay?.({
      completed: 1,
      deferred: 0,
      failed: 0,
      processed: 1,
    });
    await flush();
    expect(mockListAdminAnnouncementAuditEvents).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('公告 ID announcement-1');
    expect(container.textContent).toContain('actor 111');
    const currentViewButtons = Array.from(
      container.querySelectorAll('button'),
    ).filter(item => item.textContent?.includes('查看')) as HTMLButtonElement[];
    expect(currentViewButtons[1].disabled).toBe(false);
  });

  it('does not continue a pending publish after the page is unmounted', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    let resolvePublish:
      | ((value: {
          announcement: Record<string, unknown>;
          deferred: boolean;
          replayed: boolean;
        }) => void)
      | undefined;
    mockPublishAdminAnnouncement.mockReturnValueOnce(
      new Promise(resolve => {
        resolvePublish = resolve;
      }),
    );
    await renderSection();
    await click(button('查看'));
    await change(
      container.querySelector('[aria-label="公告正文"]') as HTMLTextAreaElement,
      '卸载前等待发布的正文。',
    );
    await click(button('立即发布'));
    expect(mockPublishAdminAnnouncement).toHaveBeenCalledTimes(1);
    const listCallsBeforeUnmount = mockListAdminAnnouncements.mock.calls.length;
    const auditCallsBeforeUnmount =
      mockListAdminAnnouncementAuditEvents.mock.calls.length;
    act(() => root.unmount());

    resolvePublish?.({
      announcement: {
        ...draftAnnouncement,
        projection_status: 'projecting',
        status: 'published',
        version: 3,
      },
      deferred: true,
      replayed: false,
    });
    await flush();

    expect(mockListAdminAnnouncements).toHaveBeenCalledTimes(
      listCallsBeforeUnmount,
    );
    expect(mockListAdminAnnouncementAuditEvents).toHaveBeenCalledTimes(
      auditCallsBeforeUnmount,
    );
    expect(mockReplayAdminAnnouncements).not.toHaveBeenCalled();
  });

  it('replays durable pending work through the admin endpoint', async () => {
    await renderSection();
    await click(button('重放待处理'));

    expect(mockReplayAdminAnnouncements).toHaveBeenCalledWith(undefined);
  });

  it('keeps only the latest audience candidate request result and loading state', async () => {
    let resolveOld:
      | ((value: {
          total: number;
          users: Array<{ name: string; user_id: string }>;
        }) => void)
      | undefined;
    let resolveLatest:
      | ((value: {
          total: number;
          users: Array<{ name: string; user_id: string }>;
        }) => void)
      | undefined;
    mockListAdminUsers
      .mockReturnValueOnce(
        new Promise(resolve => {
          resolveOld = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise(resolve => {
          resolveLatest = resolve;
        }),
      );
    await renderSection();
    await change(
      container.querySelector(
        '[aria-label="公告目标受众"]',
      ) as HTMLSelectElement,
      'users',
    );
    const keyword = container.querySelector(
      '[aria-label="搜索公告受众"]',
    ) as HTMLInputElement;
    await change(keyword, '旧候选');
    await click(button('查询'));
    await change(keyword, '新候选');
    await click(button('查询'));

    resolveLatest?.({
      total: 1,
      users: [{ name: '新候选用户', user_id: '202' }],
    });
    await flush();
    expect(container.textContent).toContain('新候选用户');
    expect(container.textContent).not.toContain('加载中...');

    resolveOld?.({
      total: 1,
      users: [{ name: '旧候选用户', user_id: '101' }],
    });
    await flush();
    expect(container.textContent).toContain('新候选用户');
    expect(container.textContent).not.toContain('旧候选用户');
    expect(container.textContent).not.toContain('加载中...');
  });

  it('drops an audience candidate continuation after unmount', async () => {
    let resolveCandidates:
      | ((value: {
          total: number;
          users: Array<{ name: string; user_id: string }>;
        }) => void)
      | undefined;
    mockListAdminUsers.mockReturnValueOnce(
      new Promise(resolve => {
        resolveCandidates = resolve;
      }),
    );
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => {});
    await renderSection();
    await change(
      container.querySelector(
        '[aria-label="公告目标受众"]',
      ) as HTMLSelectElement,
      'users',
    );
    await click(button('查询'));
    consoleError.mockClear();
    act(() => root.unmount());
    resolveCandidates?.({
      total: 1,
      users: [{ name: '卸载后的候选', user_id: '303' }],
    });
    await flush();

    expect(mockListAdminUsers).toHaveBeenCalledTimes(1);
    expect(mockListAdminWorkspaces).not.toHaveBeenCalled();
    expect(consoleError).not.toHaveBeenCalled();
    consoleError.mockRestore();
  });

  it('shows normal background delivery as informational publish feedback', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    mockPublishAdminAnnouncement.mockResolvedValueOnce({
      announcement: {
        ...draftAnnouncement,
        projection_status: 'projecting',
        status: 'published',
        version: 3,
      },
      deferred: true,
      error_code: 'projection_pending',
      replayed: false,
    });
    await renderSection();
    await click(button('查看'));
    await click(button('立即发布'));
    await flush();

    const feedback = container.querySelector('[data-feedback-tone="info"]');
    expect(feedback?.textContent).toContain('公告已发布，通知正在后台投递');
    expect(container.textContent).not.toContain('投影失败');
  });

  it('shows failed projection as safe error feedback with a stable code', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    mockPublishAdminAnnouncement.mockResolvedValueOnce({
      announcement: {
        ...draftAnnouncement,
        last_error: 'password=hunter2',
        last_error_code: 'storage_timeout',
        projection_status: 'failed',
        status: 'published',
        version: 3,
      },
      deferred: true,
      replayed: false,
    });
    await renderSection();
    await click(button('查看'));
    await click(button('立即发布'));
    await flush();

    const feedback = container.querySelector('[data-feedback-tone="error"]');
    expect(feedback?.textContent).toContain(
      '公告已发布，但通知投影失败，等待后台重试',
    );
    expect(feedback?.textContent).toContain('storage_timeout');
    expect(container.textContent).not.toContain('password=hunter2');
  });

  it('uses top-level retry error code while projection status is still pending', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [draftAnnouncement],
      total: 1,
    });
    mockPublishAdminAnnouncement.mockResolvedValueOnce({
      announcement: {
        ...draftAnnouncement,
        last_error: 'client_secret=raw-value',
        projection_status: 'projecting',
        status: 'published',
        version: 3,
      },
      deferred: true,
      error_code: 'projection_retry_pending',
      replayed: false,
    });
    await renderSection();
    await click(button('查看'));
    await click(button('立即发布'));
    await flush();

    const feedback = container.querySelector('[data-feedback-tone="error"]');
    expect(feedback?.textContent).toContain(
      '公告已发布，但通知投影失败，等待后台重试',
    );
    expect(feedback?.textContent).toContain('projection_retry_pending');
    expect(container.textContent).not.toContain('client_secret=raw-value');
    expect(container.querySelector('[data-feedback-tone="info"]')).toBeNull();
  });

  it('distinguishes normal projection from retry and shows only safe codes', async () => {
    mockListAdminAnnouncements.mockResolvedValue({
      announcements: [
        {
          ...draftAnnouncement,
          id: 'pending',
          projection_status: 'projecting',
          status: 'published',
        },
        {
          ...draftAnnouncement,
          id: 'retry',
          last_error_code: 'storage',
          projection_status: 'failed',
          status: 'published',
        },
      ],
      total: 2,
    });
    await renderSection();

    expect(container.textContent).toContain('正常投递中');
    expect(container.textContent).toContain('投影失败，等待重试');
    expect(container.textContent).toContain('storage');
    expect(
      container.querySelector('[aria-label="投影状态 projection_pending"]'),
    ).toBeTruthy();
    expect(
      container.querySelector(
        '[aria-label="投影状态 projection_retry_pending"]',
      ),
    ).toBeTruthy();
  });

  it('shows stable replay error codes without raw error data', async () => {
    mockReplayAdminAnnouncements.mockResolvedValue({
      completed: 0,
      deferred: 1,
      error_codes: {
        'password=secret': 1,
        storage: 2,
      },
      failed: 2,
      processed: 2,
    });
    await renderSection();
    await click(button('重放待处理'));

    expect(container.textContent).toContain('storage(2)');
    expect(container.textContent).toContain('请检查失败公告后再次重放');
    expect(container.textContent).not.toContain('password=secret');
  });
});
