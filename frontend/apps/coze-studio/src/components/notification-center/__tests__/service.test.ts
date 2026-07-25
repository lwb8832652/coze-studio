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

const playgroundApi = vi.hoisted(() => ({
  GetNoticeList: vi.fn(),
  GetNoticeUnreadCount: vi.fn(),
  NoticeMarkRead: vi.fn(),
}));

vi.mock('@coze-studio/api-schema/playground', () => ({
  GetNoticeList: playgroundApi.GetNoticeList,
  GetNoticeUnreadCount: playgroundApi.GetNoticeUnreadCount,
  NoticeCategory: {},
  NoticeMarkRead: playgroundApi.NoticeMarkRead,
  NoticeRankType: { All: 0, Unread: 1 },
  NoticeReadMode: { NoticeIDs: 1, Snapshot: 2 },
  NoticeRoute: {},
  NoticeSeverity: {},
}));

import {
  getNotificationUnreadCount,
  listNotifications,
  markAllNotificationsRead,
  markNotificationsRead,
  NotificationErrorCode,
  NotificationServiceError,
  NotificationReadMode,
} from '../service';

describe('notification service', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    playgroundApi.NoticeMarkRead.mockResolvedValue({
      code: 0,
      msg: 'success',
    });
  });

  it('does not permanently disable the API after a transient 404', async () => {
    playgroundApi.GetNoticeUnreadCount.mockRejectedValueOnce({
      response: { status: 404 },
    }).mockResolvedValueOnce({
      code: 0,
      data: { unread_count: 3 },
    });

    await expect(getNotificationUnreadCount()).rejects.toBeDefined();
    await expect(getNotificationUnreadCount()).resolves.toBe(3);
    expect(playgroundApi.GetNoticeUnreadCount).toHaveBeenCalledTimes(2);
  });

  it('maps the current Playground notification page contract', async () => {
    playgroundApi.GetNoticeList.mockResolvedValue({
      code: 0,
      data: {
        notice_list: [{ id: '1', content: '完成' }],
        next_cursor: 'opaque-cursor',
        has_more: true,
        snapshot_cutoff: '3003',
      },
    });

    await expect(listNotifications()).resolves.toEqual({
      notifications: [{ id: '1', content: '完成' }],
      nextCursor: 'opaque-cursor',
      hasMore: true,
      snapshotCutoff: '3003',
    });
  });

  it('keeps single and explicit mark-all operations separate', async () => {
    playgroundApi.NoticeMarkRead.mockResolvedValue({ code: 0, msg: 'success' });

    await markNotificationsRead(['1']);
    await markAllNotificationsRead('3003');

    expect(playgroundApi.NoticeMarkRead).toHaveBeenCalledTimes(2);
    expect(playgroundApi.NoticeMarkRead.mock.calls[0][0]).toEqual({
      notice_ids: ['1'],
      read_mode: NotificationReadMode.NoticeIDs,
    });
    expect(playgroundApi.NoticeMarkRead.mock.calls[1][0]).toEqual({
      read_mode: NotificationReadMode.Snapshot,
      snapshot_cutoff: '3003',
    });
  });

  it('requires a stable cutoff for mark-all', async () => {
    await expect(markAllNotificationsRead('')).rejects.toThrow('缺少通知快照');
    expect(playgroundApi.NoticeMarkRead).not.toHaveBeenCalled();
  });

  it('surfaces stable backend error codes to notification consumers', async () => {
    playgroundApi.GetNoticeList.mockRejectedValue({
      response: {
        status: 400,
        data: {
          code: 1,
          error_code: NotificationErrorCode.InvalidCursor,
          msg: '通知游标无效',
        },
      },
    });

    const error = await listNotifications().catch(cause => cause);
    expect(error).toBeInstanceOf(NotificationServiceError);
    expect(error).toMatchObject({
      code: NotificationErrorCode.InvalidCursor,
      status: 400,
    });
  });

  it('recognizes the dedicated invalid notification ID error', async () => {
    playgroundApi.NoticeMarkRead.mockRejectedValue({
      response: {
        status: 400,
        data: {
          code: 1,
          error_code: NotificationErrorCode.InvalidNotificationID,
          msg: '通知 ID 无效',
        },
      },
    });

    const error = await markNotificationsRead(['not-an-id']).catch(
      cause => cause,
    );
    expect(error).toBeInstanceOf(NotificationServiceError);
    expect(error).toMatchObject({
      code: NotificationErrorCode.InvalidNotificationID,
      status: 400,
    });
  });

  it('keeps compatibility with the pre-generation string code response', async () => {
    playgroundApi.GetNoticeUnreadCount.mockRejectedValue({
      response: {
        status: 403,
        data: {
          code: NotificationErrorCode.Forbidden,
          msg: '无权读取通知',
        },
      },
    });

    const error = await getNotificationUnreadCount().catch(cause => cause);
    expect(error).toBeInstanceOf(NotificationServiceError);
    expect(error).toMatchObject({
      code: NotificationErrorCode.Forbidden,
      status: 403,
    });
  });
});
