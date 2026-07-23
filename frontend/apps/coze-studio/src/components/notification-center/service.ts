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

import { NoticeRankType, type Notice } from '@coze-arch/bot-api/playground_api';
import { PlaygroundApi } from '@coze-arch/bot-api';

export type Notification = Notice;

export interface NotificationPage {
  hasMore: boolean;
  nextCursor: string;
  notifications: Notification[];
}

const HTTP_NOT_FOUND = 404;
let notificationApiUnavailable = false;

const isMissingNotificationApi = (error: unknown) =>
  (
    error as
      | {
          response?: {
            status?: number;
          };
        }
      | undefined
  )?.response?.status === HTTP_NOT_FOUND;

const emptyNotificationPage = (): NotificationPage => ({
  notifications: [],
  nextCursor: '',
  hasMore: false,
});

const assertSuccessfulResponse = (response: {
  code?: number | string;
  msg?: string;
}) => {
  if (response.code !== undefined && Number(response.code) !== 0) {
    throw new Error(response.msg || '通知服务请求失败');
  }
};

export const getNotificationUnreadCount = async () => {
  if (notificationApiUnavailable) {
    return 0;
  }
  try {
    const response = await PlaygroundApi.GetNoticeUnreadCount({});
    assertSuccessfulResponse(response);
    return Math.max(0, Number(response.data?.unread_count ?? 0));
  } catch (error) {
    if (isMissingNotificationApi(error)) {
      notificationApiUnavailable = true;
      return 0;
    }
    throw error;
  }
};

export const listNotifications = async (
  cursor = '0',
): Promise<NotificationPage> => {
  if (notificationApiUnavailable) {
    return emptyNotificationPage();
  }
  try {
    const response = await PlaygroundApi.GetNoticeList({
      cursor,
      count: 20,
      notice_rank_type: NoticeRankType.All,
    });
    assertSuccessfulResponse(response);

    return {
      notifications: response.data?.notice_list ?? [],
      nextCursor: response.data?.next_cursor ?? '',
      hasMore: Boolean(response.data?.has_more),
    };
  } catch (error) {
    if (isMissingNotificationApi(error)) {
      notificationApiUnavailable = true;
      return emptyNotificationPage();
    }
    throw error;
  }
};

export const markNotificationsRead = async (noticeIds: string[]) => {
  if (noticeIds.length === 0 || notificationApiUnavailable) {
    return;
  }
  try {
    const response = await PlaygroundApi.NoticeMarkRead({
      notice_ids: noticeIds,
      mark_all: false,
    });
    assertSuccessfulResponse(response);
  } catch (error) {
    if (isMissingNotificationApi(error)) {
      notificationApiUnavailable = true;
      return;
    }
    throw error;
  }
};

export const markAllNotificationsRead = async () => {
  if (notificationApiUnavailable) {
    return;
  }
  try {
    const response = await PlaygroundApi.NoticeMarkRead({
      notice_ids: [],
      mark_all: true,
    });
    assertSuccessfulResponse(response);
  } catch (error) {
    if (isMissingNotificationApi(error)) {
      notificationApiUnavailable = true;
      return;
    }
    throw error;
  }
};
