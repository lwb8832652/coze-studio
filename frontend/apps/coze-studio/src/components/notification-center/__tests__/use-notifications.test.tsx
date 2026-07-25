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

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

const service = vi.hoisted(() => ({
  getNotificationUnreadCount: vi.fn(),
  listNotifications: vi.fn(),
  markAllNotificationsRead: vi.fn(),
  markNotificationsRead: vi.fn(),
}));

const account = vi.hoisted(() => ({
  userID: 'user-a',
}));

vi.mock('../service', async importOriginal => ({
  ...(await importOriginal<Record<string, unknown>>()),
  ...service,
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => ({ user_id_str: account.userID }),
}));

import { ReadStatus } from '@coze-studio/api-schema/playground';

import { useNotifications } from '../use-notifications';
import { NotificationRoute } from '../service';

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
};

const notification = (id: string) => ({
  id,
  content: id,
  read_status: ReadStatus.Unread,
  route: NotificationRoute.TaskThread,
  route_space_id: '202',
  route_target_id: id,
});

describe('useNotifications', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    account.userID = 'user-a';
    service.getNotificationUnreadCount.mockResolvedValue(0);
    service.listNotifications.mockResolvedValue({
      notifications: [],
      nextCursor: '',
      hasMore: false,
      snapshotCutoff: '0',
    });
  });

  it('refreshes on every open without marking anything read', async () => {
    const { result } = renderHook(() => useNotifications());

    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(service.listNotifications).toHaveBeenCalledTimes(1);
    });

    act(() => result.current.setOpen(false));
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(service.listNotifications).toHaveBeenCalledTimes(2);
    });

    expect(service.markNotificationsRead).not.toHaveBeenCalled();
    expect(service.markAllNotificationsRead).not.toHaveBeenCalled();
  });

  it('keeps pagination failure retryable without replacing the first page', async () => {
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: [{ id: '1', content: '第一条' }],
        nextCursor: 'next',
        hasMore: true,
        snapshotCutoff: '3003',
      })
      .mockRejectedValueOnce(new Error('temporary'));
    const { result } = renderHook(() => useNotifications());

    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(1);
    });
    await act(async () => {
      await result.current.loadMore();
    });

    expect(result.current.notifications).toHaveLength(1);
    expect(result.current.paginationError).toBeTruthy();
  });

  it('ignores an old first-page result after account switch', async () => {
    const oldFirstPage = deferred<{
      notifications: ReturnType<typeof notification>[];
      nextCursor: string;
      hasMore: boolean;
      snapshotCutoff: string;
    }>();
    service.listNotifications
      .mockReturnValueOnce(oldFirstPage.promise)
      .mockResolvedValue({
        notifications: [notification('new-account')],
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '9000',
      });
    const { result, rerender } = renderHook(() => useNotifications());

    act(() => result.current.setOpen(true));
    account.userID = 'user-b';
    rerender();
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications[0]?.id).toBe('new-account');
    });
    await act(async () => {
      oldFirstPage.resolve({
        notifications: [notification('old-first')],
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
      await oldFirstPage.promise;
    });

    expect(result.current.notifications.map(item => item.id)).toEqual([
      'new-account',
    ]);
  });

  it('ignores an old pagination result after account switch', async () => {
    const oldNextPage = deferred<{
      notifications: ReturnType<typeof notification>[];
      nextCursor: string;
      hasMore: boolean;
      snapshotCutoff: string;
    }>();
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: [notification('old-first')],
        nextCursor: 'old-next',
        hasMore: true,
        snapshotCutoff: '3003',
      })
      .mockReturnValueOnce(oldNextPage.promise)
      .mockResolvedValue({
        notifications: [notification('new-account')],
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '9000',
      });
    const { result, rerender } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications[0]?.id).toBe('old-first');
    });
    act(() => {
      void result.current.loadMore();
    });

    account.userID = 'user-b';
    rerender();
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications[0]?.id).toBe('new-account');
    });
    await act(async () => {
      oldNextPage.resolve({
        notifications: [notification('old-next')],
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
      await oldNextPage.promise;
    });

    expect(result.current.notifications.map(item => item.id)).toEqual([
      'new-account',
    ]);
  });

  it('does not roll back old single-read or mark-all state into a new account', async () => {
    const singleRead = deferred<undefined>();
    const markAll = deferred<undefined>();
    service.getNotificationUnreadCount.mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: [notification('old-1'), notification('old-2')],
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockResolvedValue({
        notifications: [notification('new-1')],
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '9000',
      });
    service.markNotificationsRead.mockReturnValue(singleRead.promise);
    service.markAllNotificationsRead.mockReturnValue(markAll.promise);
    const { result, rerender } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(2);
      expect(result.current.unreadCount).toBe(2);
    });

    const singlePromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    const allPromise = result.current.markAllRead().catch(() => undefined);
    account.userID = 'user-b';
    rerender();
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications[0]?.id).toBe('new-1');
    });
    await act(async () => {
      singleRead.reject(new Error('old single read failed'));
      markAll.reject(new Error('old mark all failed'));
      await Promise.all([singlePromise, allPromise]);
    });

    expect(result.current.notifications.map(item => item.id)).toEqual([
      'new-1',
    ]);
    expect(service.markAllNotificationsRead).toHaveBeenCalledWith('3003');
  });

  it('rolls back a failed single read for the current account only', async () => {
    service.getNotificationUnreadCount.mockResolvedValue(1);
    service.listNotifications.mockResolvedValue({
      notifications: [notification('notice-1')],
      nextCursor: '',
      hasMore: false,
      snapshotCutoff: '3003',
    });
    service.markNotificationsRead.mockRejectedValue(new Error('failed'));
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(1);
    });

    await act(async () => {
      await expect(
        result.current.markRead(result.current.notifications[0]),
      ).rejects.toThrow('failed');
    });

    expect(result.current.notifications[0]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.unreadCount).toBe(1);
  });

  it('does not overwrite a newer polling count when single-read rollback fails', async () => {
    const singleRead = deferred<undefined>();
    service.getNotificationUnreadCount.mockResolvedValue(2);
    service.listNotifications.mockResolvedValue({
      notifications: [notification('notice-1')],
      nextCursor: '',
      hasMore: false,
      snapshotCutoff: '3003',
    });
    service.markNotificationsRead.mockReturnValue(singleRead.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.unreadCount).toBe(2);
    });

    const markPromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    service.getNotificationUnreadCount.mockResolvedValue(7);
    await act(async () => {
      await result.current.refreshUnreadCount();
    });
    await act(async () => {
      singleRead.reject(new Error('failed'));
      await markPromise;
    });

    expect(result.current.notifications[0]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.unreadCount).toBe(7);
  });

  it('invalidates an older unread poll when single-read starts', async () => {
    service.getNotificationUnreadCount.mockResolvedValue(1);
    service.listNotifications.mockResolvedValue({
      notifications: [notification('notice-1')],
      nextCursor: '',
      hasMore: false,
      snapshotCutoff: '3003',
    });
    service.markNotificationsRead.mockResolvedValue(undefined);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.unreadCount).toBe(1);
    });

    const olderUnread = deferred<number>();
    service.getNotificationUnreadCount
      .mockReturnValueOnce(olderUnread.promise)
      .mockResolvedValueOnce(0);
    const callsBeforePoll =
      service.getNotificationUnreadCount.mock.calls.length;
    let olderPoll!: Promise<void>;
    act(() => {
      olderPoll = result.current.refreshUnreadCount();
    });
    await waitFor(() => {
      expect(service.getNotificationUnreadCount).toHaveBeenCalledTimes(
        callsBeforePoll + 1,
      );
    });

    await act(async () => {
      await result.current.markRead(result.current.notifications[0]);
    });
    expect(result.current.unreadCount).toBe(0);

    await act(async () => {
      olderUnread.resolve(99);
      await olderPoll;
    });
    expect(result.current.unreadCount).toBe(0);
  });

  it('reloads unread count after mark-all so post-cutoff notices stay unread', async () => {
    service.getNotificationUnreadCount
      .mockResolvedValueOnce(2)
      .mockResolvedValueOnce(2)
      .mockResolvedValue(1);
    service.listNotifications.mockResolvedValue({
      notifications: [notification('before-cutoff')],
      nextCursor: '',
      hasMore: false,
      snapshotCutoff: '3003',
    });
    service.markAllNotificationsRead.mockResolvedValue(undefined);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.unreadCount).toBe(2);
    });

    await act(async () => {
      await result.current.markAllRead();
    });

    expect(service.markAllNotificationsRead).toHaveBeenCalledWith('3003');
    expect(service.getNotificationUnreadCount).toHaveBeenCalledTimes(3);
    expect(result.current.unreadCount).toBe(1);
    expect(result.current.notifications[0]?.read_status).toBe(ReadStatus.Read);
  });

  it('invalidates an older unread poll when mark-all starts', async () => {
    service.getNotificationUnreadCount
      .mockResolvedValueOnce(1)
      .mockResolvedValueOnce(1);
    service.listNotifications.mockResolvedValue({
      notifications: [notification('before-cutoff')],
      nextCursor: '',
      hasMore: false,
      snapshotCutoff: '3003',
    });
    service.markAllNotificationsRead.mockResolvedValue(undefined);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.unreadCount).toBe(1);
    });

    const olderUnread = deferred<number>();
    service.getNotificationUnreadCount
      .mockReturnValueOnce(olderUnread.promise)
      .mockResolvedValueOnce(0);
    const callsBeforePoll =
      service.getNotificationUnreadCount.mock.calls.length;
    let olderPoll!: Promise<void>;
    act(() => {
      olderPoll = result.current.refreshUnreadCount();
    });
    await waitFor(() => {
      expect(service.getNotificationUnreadCount).toHaveBeenCalledTimes(
        callsBeforePoll + 1,
      );
    });

    await act(async () => {
      await result.current.markAllRead();
    });
    expect(result.current.unreadCount).toBe(0);

    await act(async () => {
      olderUnread.resolve(99);
      await olderPoll;
    });
    expect(result.current.unreadCount).toBe(0);
  });

  it('rolls back only mark-all optimistic IDs and preserves newer unread revisions', async () => {
    const markAll = deferred<undefined>();
    service.getNotificationUnreadCount.mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: [notification('first-page')],
        nextCursor: 'next-page',
        hasMore: true,
        snapshotCutoff: '3003',
      })
      .mockResolvedValueOnce({
        notifications: [notification('concurrent-page')],
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
    service.markAllNotificationsRead.mockReturnValue(markAll.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications.map(item => item.id)).toEqual([
        'first-page',
      ]);
      expect(result.current.unreadCount).toBe(2);
    });

    const markPromise = result.current.markAllRead().catch(() => undefined);
    await act(async () => {
      await result.current.loadMore();
    });
    service.getNotificationUnreadCount.mockResolvedValue(7);
    await act(async () => {
      await result.current.refreshUnreadCount();
    });
    await act(async () => {
      markAll.reject(new Error('mark all failed'));
      await markPromise;
    });

    expect(result.current.notifications.map(item => item.id)).toEqual([
      'first-page',
      'concurrent-page',
    ]);
    expect(
      result.current.notifications.every(
        item => item.read_status === ReadStatus.Unread,
      ),
    ).toBe(true);
    expect(result.current.unreadCount).toBe(7);
  });

  it('authoritatively recovers when single and mark-all both fail with the single response first', async () => {
    const singleRead = deferred<undefined>();
    const markAll = deferred<undefined>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
    ];
    service.getNotificationUnreadCount.mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockResolvedValue({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
    service.markNotificationsRead.mockReturnValue(singleRead.promise);
    service.markAllNotificationsRead.mockReturnValue(markAll.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(2);
      expect(result.current.unreadCount).toBe(2);
    });

    const singlePromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    const markAllPromise = result.current.markAllRead().catch(() => undefined);
    await act(async () => {
      singleRead.reject(new Error('single failed'));
      await Promise.resolve();
    });
    expect(service.listNotifications).toHaveBeenCalledTimes(1);

    await act(async () => {
      markAll.reject(new Error('mark all failed'));
      await Promise.all([singlePromise, markAllPromise]);
    });

    expect(service.listNotifications).toHaveBeenCalledTimes(2);
    expect(
      result.current.notifications.every(
        item => item.read_status === ReadStatus.Unread,
      ),
    ).toBe(true);
    expect(result.current.unreadCount).toBe(2);
  });

  it('authoritatively recovers when single and mark-all both fail with the mark-all response first', async () => {
    const singleRead = deferred<undefined>();
    const markAll = deferred<undefined>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
    ];
    service.getNotificationUnreadCount.mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockResolvedValue({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
    service.markNotificationsRead.mockReturnValue(singleRead.promise);
    service.markAllNotificationsRead.mockReturnValue(markAll.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(2);
      expect(result.current.unreadCount).toBe(2);
    });

    const singlePromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    const markAllPromise = result.current.markAllRead().catch(() => undefined);
    await act(async () => {
      markAll.reject(new Error('mark all failed'));
      await Promise.resolve();
    });
    expect(service.listNotifications).toHaveBeenCalledTimes(1);

    await act(async () => {
      singleRead.reject(new Error('single failed'));
      await Promise.all([singlePromise, markAllPromise]);
    });

    expect(service.listNotifications).toHaveBeenCalledTimes(2);
    expect(
      result.current.notifications.every(
        item => item.read_status === ReadStatus.Unread,
      ),
    ).toBe(true);
    expect(result.current.unreadCount).toBe(2);
  });

  it('keeps the single read when it succeeds before a concurrent mark-all failure', async () => {
    const singleRead = deferred<undefined>();
    const markAll = deferred<undefined>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
    ];
    const authoritativeNotifications = [
      { ...notification('notice-1'), read_status: ReadStatus.Read },
      notification('notice-2'),
    ];
    service.getNotificationUnreadCount.mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockResolvedValue({
        notifications: authoritativeNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
    service.markNotificationsRead.mockReturnValue(singleRead.promise);
    service.markAllNotificationsRead.mockReturnValue(markAll.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(2);
      expect(result.current.unreadCount).toBe(2);
    });

    const singlePromise = result.current.markRead(
      result.current.notifications[0],
    );
    const markAllPromise = result.current.markAllRead().catch(() => undefined);
    service.getNotificationUnreadCount.mockResolvedValue(1);
    await act(async () => {
      singleRead.resolve();
      await singlePromise;
    });
    await act(async () => {
      markAll.reject(new Error('mark all failed'));
      await markAllPromise;
    });

    expect(result.current.notifications[0]?.read_status).toBe(ReadStatus.Read);
    expect(result.current.notifications[1]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.unreadCount).toBe(1);
  });

  it('keeps mark-all success when it resolves before a concurrent single failure', async () => {
    const singleRead = deferred<undefined>();
    const markAll = deferred<undefined>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
    ];
    const authoritativeNotifications = initialNotifications.map(item => ({
      ...item,
      read_status: ReadStatus.Read,
    }));
    service.getNotificationUnreadCount.mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockResolvedValue({
        notifications: authoritativeNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
    service.markNotificationsRead.mockReturnValue(singleRead.promise);
    service.markAllNotificationsRead.mockReturnValue(markAll.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(2);
    });

    const singlePromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    const markAllPromise = result.current.markAllRead();
    service.getNotificationUnreadCount.mockResolvedValue(0);
    await act(async () => {
      markAll.resolve();
      await markAllPromise;
    });
    await act(async () => {
      singleRead.reject(new Error('single failed'));
      await singlePromise;
    });

    expect(
      result.current.notifications.every(
        item => item.read_status === ReadStatus.Read,
      ),
    ).toBe(true);
    expect(result.current.unreadCount).toBe(0);
  });

  it('falls back to conservative unread state when concurrent recovery also fails', async () => {
    const singleRead = deferred<undefined>();
    const markAll = deferred<undefined>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
    ];
    service.getNotificationUnreadCount.mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockRejectedValue(new Error('authoritative list failed'));
    service.markNotificationsRead.mockReturnValue(singleRead.promise);
    service.markAllNotificationsRead.mockReturnValue(markAll.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(2);
      expect(result.current.unreadCount).toBe(2);
    });

    const singlePromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    const markAllPromise = result.current.markAllRead().catch(() => undefined);
    service.getNotificationUnreadCount.mockRejectedValue(
      new Error('authoritative unread failed'),
    );
    await act(async () => {
      singleRead.reject(new Error('single failed'));
      markAll.reject(new Error('mark all failed'));
      await Promise.all([singlePromise, markAllPromise]);
    });

    expect(
      result.current.notifications.every(
        item => item.read_status === ReadStatus.Unread,
      ),
    ).toBe(true);
    expect(result.current.unreadCount).toBeGreaterThanOrEqual(2);
    expect(result.current.error).toBe('通知状态同步失败，请重试');

    service.listNotifications.mockResolvedValue({
      notifications: initialNotifications,
      nextCursor: '',
      hasMore: false,
      snapshotCutoff: '3003',
    });
    service.getNotificationUnreadCount.mockResolvedValue(2);
    await act(async () => {
      await result.current.retry();
    });
    expect(result.current.error).toBe('');
  });

  it('reruns an interrupted read recovery so stale snapshots do not overwrite newer reads', async () => {
    const firstRead = deferred<undefined>();
    const secondRead = deferred<undefined>();
    const staleList = deferred<{
      notifications: ReturnType<typeof notification>[];
      nextCursor: string;
      hasMore: boolean;
      snapshotCutoff: string;
    }>();
    const staleUnread = deferred<number>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
      notification('notice-3'),
    ];
    const recoveredNotifications = [
      notification('notice-1'),
      notification('notice-2'),
      { ...notification('notice-3'), read_status: ReadStatus.Read },
    ];
    service.getNotificationUnreadCount
      .mockResolvedValueOnce(3)
      .mockResolvedValueOnce(3)
      .mockReturnValueOnce(staleUnread.promise)
      .mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockReturnValueOnce(staleList.promise)
      .mockResolvedValue({
        notifications: recoveredNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
    service.markNotificationsRead
      .mockReturnValueOnce(firstRead.promise)
      .mockReturnValueOnce(secondRead.promise)
      .mockResolvedValue(undefined);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(3);
      expect(result.current.unreadCount).toBe(3);
    });

    const firstPromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    const secondPromise = result.current
      .markRead(result.current.notifications[1])
      .catch(() => undefined);
    await act(async () => {
      firstRead.reject(new Error('first failed'));
      secondRead.reject(new Error('second failed'));
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(service.listNotifications).toHaveBeenCalledTimes(2);
    });

    await act(async () => {
      await result.current.markRead(result.current.notifications[2]);
    });
    await act(async () => {
      staleList.resolve({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
      staleUnread.resolve(3);
      await Promise.all([firstPromise, secondPromise]);
    });

    await waitFor(() => {
      expect(service.listNotifications).toHaveBeenCalledTimes(3);
    });
    expect(result.current.notifications[0]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.notifications[1]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.notifications[2]?.read_status).toBe(ReadStatus.Read);
    expect(result.current.unreadCount).toBe(2);
  });

  it('does not commit stale recovery lists when the rerun list recovery fails', async () => {
    const firstRead = deferred<undefined>();
    const secondRead = deferred<undefined>();
    const staleList = deferred<{
      notifications: ReturnType<typeof notification>[];
      nextCursor: string;
      hasMore: boolean;
      snapshotCutoff: string;
    }>();
    const staleUnread = deferred<number>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
      notification('notice-3'),
    ];
    service.getNotificationUnreadCount
      .mockResolvedValueOnce(3)
      .mockResolvedValueOnce(3)
      .mockReturnValueOnce(staleUnread.promise)
      .mockResolvedValue(2);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockReturnValueOnce(staleList.promise)
      .mockRejectedValue(new Error('rerun list failed'));
    service.markNotificationsRead
      .mockReturnValueOnce(firstRead.promise)
      .mockReturnValueOnce(secondRead.promise)
      .mockResolvedValue(undefined);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(3);
      expect(result.current.unreadCount).toBe(3);
    });

    const firstPromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    const secondPromise = result.current
      .markRead(result.current.notifications[1])
      .catch(() => undefined);
    await act(async () => {
      firstRead.reject(new Error('first failed'));
      secondRead.reject(new Error('second failed'));
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(service.listNotifications).toHaveBeenCalledTimes(2);
    });

    await act(async () => {
      await result.current.markRead(result.current.notifications[2]);
    });
    await act(async () => {
      staleList.resolve({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
      staleUnread.resolve(3);
      await Promise.all([firstPromise, secondPromise]);
    });

    await waitFor(() => {
      expect(service.listNotifications).toHaveBeenCalledTimes(3);
    });
    expect(result.current.notifications[0]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.notifications[1]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.notifications[2]?.read_status).toBe(ReadStatus.Read);
    expect(result.current.unreadCount).toBe(2);
    expect(result.current.error).toBe('通知状态同步失败，请重试');
  });

  it('merges an older in-flight recovery back after a newer recovery has started', async () => {
    const firstRead = deferred<undefined>();
    const secondRead = deferred<undefined>();
    const thirdRead = deferred<undefined>();
    const fourthRead = deferred<undefined>();
    const olderRecoveryList = deferred<{
      notifications: ReturnType<typeof notification>[];
      nextCursor: string;
      hasMore: boolean;
      snapshotCutoff: string;
    }>();
    const olderRecoveryUnread = deferred<number>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
      notification('notice-3'),
      notification('notice-4'),
    ];
    service.getNotificationUnreadCount
      .mockResolvedValueOnce(4)
      .mockResolvedValueOnce(4)
      .mockReturnValueOnce(olderRecoveryUnread.promise)
      .mockResolvedValue(4);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockReturnValueOnce(olderRecoveryList.promise)
      .mockRejectedValue(new Error('recovery list failed'));
    service.markNotificationsRead
      .mockReturnValueOnce(firstRead.promise)
      .mockReturnValueOnce(secondRead.promise)
      .mockReturnValueOnce(thirdRead.promise)
      .mockReturnValueOnce(fourthRead.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(4);
      expect(result.current.unreadCount).toBe(4);
    });

    const firstPromise = result.current
      .markRead(result.current.notifications[0])
      .catch(() => undefined);
    const secondPromise = result.current
      .markRead(result.current.notifications[1])
      .catch(() => undefined);
    await act(async () => {
      firstRead.reject(new Error('first failed'));
      secondRead.reject(new Error('second failed'));
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(service.listNotifications).toHaveBeenCalledTimes(2);
    });

    const thirdPromise = result.current
      .markRead(result.current.notifications[2])
      .catch(() => undefined);
    const fourthPromise = result.current
      .markRead(result.current.notifications[3])
      .catch(() => undefined);
    await act(async () => {
      thirdRead.reject(new Error('third failed'));
      fourthRead.reject(new Error('fourth failed'));
      await Promise.all([thirdPromise, fourthPromise]);
    });
    expect(result.current.notifications[0]?.read_status).toBe(ReadStatus.Read);
    expect(result.current.notifications[1]?.read_status).toBe(ReadStatus.Read);
    expect(result.current.notifications[2]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.notifications[3]?.read_status).toBe(
      ReadStatus.Unread,
    );

    await act(async () => {
      olderRecoveryList.resolve({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
      olderRecoveryUnread.resolve(4);
      await Promise.all([firstPromise, secondPromise]);
    });

    await waitFor(() => {
      expect(service.listNotifications).toHaveBeenCalledTimes(4);
    });
    expect(
      result.current.notifications.every(
        item => item.read_status === ReadStatus.Unread,
      ),
    ).toBe(true);
    expect(result.current.unreadCount).toBe(4);
  });

  it('keeps recovered list state when only unread recovery fails', async () => {
    const firstRead = deferred<undefined>();
    const secondRead = deferred<undefined>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
    ];
    const recoveredNotifications = [
      { ...notification('notice-1'), read_status: ReadStatus.Read },
      notification('notice-2'),
    ];
    service.getNotificationUnreadCount
      .mockResolvedValueOnce(2)
      .mockResolvedValueOnce(2)
      .mockRejectedValue(new Error('unread failed'));
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockResolvedValue({
        notifications: recoveredNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      });
    service.markNotificationsRead
      .mockReturnValueOnce(firstRead.promise)
      .mockReturnValueOnce(secondRead.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(2);
    });

    const firstPromise = result.current.markRead(
      result.current.notifications[0],
    );
    const secondPromise = result.current
      .markRead(result.current.notifications[1])
      .catch(() => undefined);
    await act(async () => {
      firstRead.resolve();
      secondRead.reject(new Error('second failed'));
      await Promise.all([firstPromise, secondPromise]);
    });

    expect(result.current.notifications[0]?.read_status).toBe(ReadStatus.Read);
    expect(result.current.notifications[1]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.error).toBe('通知状态同步失败，请重试');
  });

  it('keeps recovered unread count when only list recovery fails', async () => {
    const firstRead = deferred<undefined>();
    const secondRead = deferred<undefined>();
    const initialNotifications = [
      notification('notice-1'),
      notification('notice-2'),
    ];
    service.getNotificationUnreadCount
      .mockResolvedValueOnce(2)
      .mockResolvedValueOnce(2)
      .mockResolvedValue(1);
    service.listNotifications
      .mockResolvedValueOnce({
        notifications: initialNotifications,
        nextCursor: '',
        hasMore: false,
        snapshotCutoff: '3003',
      })
      .mockRejectedValue(new Error('list failed'));
    service.markNotificationsRead
      .mockReturnValueOnce(firstRead.promise)
      .mockReturnValueOnce(secondRead.promise);
    const { result } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(2);
      expect(result.current.unreadCount).toBe(2);
    });

    const firstPromise = result.current.markRead(
      result.current.notifications[0],
    );
    const secondPromise = result.current
      .markRead(result.current.notifications[1])
      .catch(() => undefined);
    await act(async () => {
      firstRead.resolve();
      secondRead.reject(new Error('second failed'));
      await Promise.all([firstPromise, secondPromise]);
    });

    expect(result.current.notifications[0]?.read_status).toBe(ReadStatus.Read);
    expect(result.current.notifications[1]?.read_status).toBe(
      ReadStatus.Unread,
    );
    expect(result.current.unreadCount).toBe(1);
    expect(result.current.error).toBe('通知状态同步失败，请重试');
  });

  it('masks old account state synchronously during account switch and logout', async () => {
    service.getNotificationUnreadCount.mockResolvedValue(3);
    service.listNotifications.mockResolvedValue({
      notifications: [notification('old-account')],
      nextCursor: '',
      hasMore: false,
      snapshotCutoff: '3003',
    });
    const { result, rerender } = renderHook(() => useNotifications());
    act(() => result.current.setOpen(true));
    await waitFor(() => {
      expect(result.current.notifications).toHaveLength(1);
      expect(result.current.unreadCount).toBe(3);
    });

    account.userID = 'user-b';
    rerender();
    expect(result.current.notifications).toEqual([]);
    expect(result.current.unreadCount).toBe(0);
    expect(result.current.error).toBe('');
    expect(result.current.unreadError).toBe('');

    account.userID = '';
    rerender();
    expect(result.current.notifications).toEqual([]);
    expect(result.current.unreadCount).toBe(0);
    expect(result.current.open).toBe(false);
  });

  it('does not poll or load notices without an authenticated identity', async () => {
    account.userID = '';
    const { result } = renderHook(() => useNotifications());
    await act(async () => {
      await Promise.resolve();
      result.current.setOpen(true);
      await Promise.resolve();
    });

    expect(service.getNotificationUnreadCount).not.toHaveBeenCalled();
    expect(service.listNotifications).not.toHaveBeenCalled();
    expect(result.current.notifications).toEqual([]);
    expect(result.current.unreadCount).toBe(0);
    expect(result.current.open).toBe(false);
  });

  it('refreshes unread state when the document becomes visible', async () => {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'hidden',
    });
    renderHook(() => useNotifications());
    await waitFor(() => {
      expect(service.getNotificationUnreadCount).toHaveBeenCalledTimes(1);
    });

    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible',
    });
    act(() => {
      document.dispatchEvent(new Event('visibilitychange'));
    });

    await waitFor(() => {
      expect(service.getNotificationUnreadCount).toHaveBeenCalledTimes(2);
    });
  });

  it('ignores an old account unread response', async () => {
    const oldUnread = deferred<number>();
    service.getNotificationUnreadCount
      .mockReturnValueOnce(oldUnread.promise)
      .mockResolvedValueOnce(2);
    const { result, rerender } = renderHook(() => useNotifications());
    account.userID = 'user-b';
    rerender();
    await waitFor(() => {
      expect(result.current.unreadCount).toBe(2);
    });

    await act(async () => {
      oldUnread.resolve(99);
      await oldUnread.promise;
    });

    expect(result.current.unreadCount).toBe(2);
  });
});
