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

const noticeMarkRead = vi.hoisted(() => vi.fn());

vi.mock('@coze-studio/api-schema/playground', () => ({
  GetNoticeList: vi.fn(),
  GetNoticeUnreadCount: vi.fn(),
  NoticeCategory: {},
  NoticeMarkRead: noticeMarkRead,
  NoticeRankType: { All: 0, Unread: 1 },
  NoticeReadMode: { NoticeIDs: 1, Snapshot: 2 },
  NoticeRoute: {},
  NoticeSeverity: {},
}));

import { markAllNotificationsRead, markNotificationsRead } from '../service';

describe('notification mark-read generated client', () => {
  beforeEach(() => {
    noticeMarkRead.mockReset();
    noticeMarkRead.mockResolvedValue({ code: 0, msg: 'success' });
  });

  it('passes the single-read mode and notice ids to the generated client', async () => {
    await markNotificationsRead(['notice-1', 'notice-2']);

    expect(noticeMarkRead).toHaveBeenCalledTimes(1);
    expect(noticeMarkRead.mock.calls[0][0]).toEqual({
      notice_ids: ['notice-1', 'notice-2'],
      read_mode: 1,
    });
  });

  it('posts the stable snapshot cutoff for mark-all', async () => {
    await markAllNotificationsRead('3003');

    expect(noticeMarkRead.mock.calls[0][0]).toEqual({
      read_mode: 2,
      snapshot_cutoff: '3003',
    });
  });
});
