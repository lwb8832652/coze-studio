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

import {
  GetNoticeList as getNoticeListGenerated,
  GetNoticeUnreadCount as getNoticeUnreadCountGenerated,
  NoticeCategory,
  NoticeMarkRead as noticeMarkReadGenerated,
  NoticeRankType,
  NoticeReadMode,
  NoticeRoute,
  NoticeSeverity,
  type Notice,
  type NoticeMarkReadRequest,
} from '@coze-studio/api-schema/playground';

export {
  NoticeCategory as NotificationCategory,
  NoticeReadMode as NotificationReadMode,
  NoticeRoute as NotificationRoute,
  NoticeSeverity as NotificationSeverity,
};

export enum NotificationErrorCode {
  InvalidCursor = 'NOTIFICATION_INVALID_CURSOR',
  InvalidReadMode = 'NOTIFICATION_INVALID_READ_MODE',
  InvalidPageSize = 'NOTIFICATION_INVALID_PAGE_SIZE',
  InvalidRequest = 'NOTIFICATION_INVALID_REQUEST',
  InvalidNotificationID = 'NOTIFICATION_INVALID_NOTIFICATION_ID',
  Forbidden = 'NOTIFICATION_FORBIDDEN',
  Internal = 'NOTIFICATION_INTERNAL',
}

export class NotificationServiceError extends Error {
  constructor(
    public readonly code: NotificationErrorCode,
    message: string,
    public readonly status?: number,
  ) {
    super(message);
    this.name = 'NotificationServiceError';
  }
}

export type Notification = Notice;

export interface NotificationPage {
  hasMore: boolean;
  nextCursor: string;
  notifications: Notification[];
  snapshotCutoff: string;
}

const notificationRequestOptions = {
  __disableErrorToast: true,
};

const notificationErrorCodes = new Set<string>(
  Object.values(NotificationErrorCode),
);

const isNotificationErrorCode = (
  value: unknown,
): value is NotificationErrorCode =>
  typeof value === 'string' && notificationErrorCodes.has(value);

const normalizeNotificationError = (cause: unknown): never => {
  if (cause instanceof NotificationServiceError) {
    throw cause;
  }
  const response = (
    cause as {
      response?: {
        data?: {
          code?: unknown;
          error_code?: unknown;
          msg?: unknown;
        };
        status?: number;
      };
    }
  )?.response;
  const errorCode = response?.data?.error_code;
  const code = isNotificationErrorCode(errorCode)
    ? errorCode
    : response?.data?.code;
  if (isNotificationErrorCode(code)) {
    throw new NotificationServiceError(
      code,
      typeof response?.data?.msg === 'string'
        ? response.data.msg
        : '通知服务请求失败',
      response?.status,
    );
  }
  if (cause instanceof Error) {
    throw cause;
  }
  throw new Error('通知服务请求失败');
};

const assertSuccessfulResponse = (response: {
  code?: number | string;
  error_code?: unknown;
  msg?: string;
}) => {
  if (response.code !== undefined && Number(response.code) !== 0) {
    const code = isNotificationErrorCode(response.error_code)
      ? response.error_code
      : response.code;
    if (isNotificationErrorCode(code)) {
      throw new NotificationServiceError(
        code,
        response.msg || '通知服务请求失败',
      );
    }
    throw new Error(response.msg || '通知服务请求失败');
  }
};

const postNotificationMarkRead = async (body: NoticeMarkReadRequest) => {
  try {
    const response = await noticeMarkReadGenerated(
      body,
      notificationRequestOptions,
    );
    assertSuccessfulResponse(response);
  } catch (cause) {
    normalizeNotificationError(cause);
  }
};

export const getNotificationUnreadCount = async () => {
  try {
    const response = await getNoticeUnreadCountGenerated(
      {},
      notificationRequestOptions,
    );
    assertSuccessfulResponse(response);
    return Math.max(0, Number(response.data?.unread_count ?? 0));
  } catch (cause) {
    normalizeNotificationError(cause);
  }
};

export const listNotifications = async (
  cursor = '0',
): Promise<NotificationPage> => {
  try {
    const response = await getNoticeListGenerated(
      {
        cursor,
        count: 20,
        notice_rank_type: NoticeRankType.All,
      },
      notificationRequestOptions,
    );
    assertSuccessfulResponse(response);
    const data = response.data as typeof response.data & {
      notice_list?: Notification[];
      snapshot_cutoff?: string;
    };

    return {
      notifications: data?.notice_list ?? [],
      nextCursor: data?.next_cursor ?? '',
      hasMore: Boolean(data?.has_more),
      snapshotCutoff: data?.snapshot_cutoff ?? '0',
    };
  } catch (cause) {
    normalizeNotificationError(cause);
  }
};

export const markNotificationsRead = async (noticeIds: string[]) => {
  if (noticeIds.length === 0) {
    return;
  }
  await postNotificationMarkRead({
    notice_ids: noticeIds,
    read_mode: NoticeReadMode.NoticeIDs,
  });
};

export const markAllNotificationsRead = async (snapshotCutoff: string) => {
  if (!/^[1-9]\d*$/.test(snapshotCutoff)) {
    throw new Error('缺少通知快照，无法执行全部已读');
  }
  await postNotificationMarkRead({
    read_mode: NoticeReadMode.Snapshot,
    snapshot_cutoff: snapshotCutoff,
  });
};
