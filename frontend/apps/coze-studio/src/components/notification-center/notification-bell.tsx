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

import { useNavigate } from 'react-router-dom';

import {
  IconCozBell,
  IconCozBot,
  IconCozRefresh,
} from '@coze-arch/coze-design/icons';
import { Badge, Popover, Toast } from '@coze-arch/coze-design';
import { ReadStatus } from '@coze-arch/bot-api/playground_api';

import { useNotifications } from './use-notifications';
import { type Notification } from './service';

import './index.less';

const MAX_VISIBLE_UNREAD_COUNT = 99;

const formatNotificationTime = (value?: string) => {
  const timestamp = Number(value ?? 0);
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    return '';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(timestamp));
};

const getSafeNotificationTarget = (jumpLink?: string) => {
  if (!jumpLink) {
    return '';
  }
  const normalized = jumpLink.startsWith('/') ? jumpLink : '';
  return /^\/space\/[^/]+\/(?:tasks\/[^/?#]+|task-center|workspace)(?:[?#].*)?$/.test(
    normalized,
  )
    ? normalized
    : '';
};

const getNotificationTitle = (notification: Notification) =>
  notification.sender?.sender_name || '系统通知';

type NotificationState = ReturnType<typeof useNotifications>;

const NotificationPanel = ({
  onNotificationClick,
  state,
  unreadCount,
}: {
  onNotificationClick: (notification: Notification) => void;
  state: NotificationState;
  unreadCount: number;
}) => (
  <section className="notification-center" aria-label="通知中心">
    <header className="notification-center__header">
      <div>
        <strong>通知</strong>
        <span>{unreadCount > 0 ? `${unreadCount} 条未读` : '全部已读'}</span>
      </div>
      <button
        type="button"
        className="notification-center__read-all"
        disabled={unreadCount === 0}
        onClick={() => {
          void state.markAllRead().catch(error => {
            Toast.error({
              content:
                error instanceof Error
                  ? '全部已读操作失败，请稍后重试'
                  : '通知服务暂不可用',
            });
          });
        }}
      >
        全部已读
      </button>
    </header>
    <div className="notification-center__list" aria-live="polite">
      {state.loading ? (
        <div className="notification-center__state">正在加载通知...</div>
      ) : state.error ? (
        <div className="notification-center__state notification-center__state--error">
          <span>{state.error}</span>
          <button type="button" onClick={() => void state.retry()}>
            <IconCozRefresh />
            重试
          </button>
        </div>
      ) : state.notifications.length === 0 ? (
        <div className="notification-center__state">
          <IconCozBell className="notification-center__empty-icon" />
          <strong>暂无通知</strong>
          <span>任务和工作空间动态会在这里提醒你</span>
        </div>
      ) : (
        <>
          {state.notifications.map(notification => {
            const unread = notification.read_status !== ReadStatus.Read;
            return (
              <button
                key={notification.id}
                type="button"
                className="notification-center__item"
                data-unread={unread}
                data-notification-id={notification.id}
                onClick={() => onNotificationClick(notification)}
              >
                <span className="notification-center__avatar">
                  <IconCozBot />
                </span>
                <span className="notification-center__copy">
                  <span className="notification-center__item-heading">
                    <strong>{getNotificationTitle(notification)}</strong>
                    <time>
                      {formatNotificationTime(notification.create_time)}
                    </time>
                  </span>
                  <span className="notification-center__summary">
                    {notification.content || '你有一条新通知'}
                  </span>
                </span>
                {unread ? (
                  <span
                    className="notification-center__unread-dot"
                    aria-label="未读"
                  />
                ) : null}
              </button>
            );
          })}
          {state.hasMore ? (
            <button
              type="button"
              className="notification-center__load-more"
              disabled={state.loadingMore}
              onClick={() => void state.loadMore()}
            >
              {state.loadingMore ? '正在加载...' : '加载更多'}
            </button>
          ) : null}
        </>
      )}
    </div>
  </section>
);

export const NotificationBell = ({
  className,
  compact = false,
  unreadCount: unreadCountOverride,
}: {
  className?: string;
  compact?: boolean;
  unreadCount?: number;
}) => {
  const navigate = useNavigate();
  const state = useNotifications();
  const unreadCount = unreadCountOverride ?? state.unreadCount;
  const displayUnreadCount =
    unreadCount > MAX_VISIBLE_UNREAD_COUNT ? '99+' : unreadCount;
  const ariaLabel = unreadCount > 0 ? `通知，${unreadCount} 条未读` : '通知';

  const handleNotificationClick = async (notification: Notification) => {
    try {
      await state.markRead(notification);
    } catch (error) {
      Toast.error({
        content:
          error instanceof Error
            ? '标记通知已读失败，请稍后重试'
            : '通知服务暂不可用',
      });
      return;
    }

    if (!notification.jump_link) {
      return;
    }
    const target = getSafeNotificationTarget(notification.jump_link);
    if (!target) {
      Toast.error({ content: '该通知的目标已失效' });
      return;
    }
    state.setOpen(false);
    navigate(target);
  };

  const content = (
    <NotificationPanel
      state={state}
      unreadCount={unreadCount}
      onNotificationClick={notification => {
        void handleNotificationClick(notification);
      }}
    />
  );

  const trigger = (
    <button
      type="button"
      className={[
        'notification-bell__trigger',
        compact ? 'notification-bell__trigger--compact' : '',
        className ?? '',
      ]
        .filter(Boolean)
        .join(' ')}
      aria-label={ariaLabel}
      aria-haspopup="dialog"
      aria-expanded={state.open}
      onClick={() => state.setOpen(open => !open)}
    >
      <IconCozBell className={compact ? 'text-[14px]' : 'text-[16px]'} />
    </button>
  );

  return (
    <Popover
      content={content}
      position="bottomRight"
      trigger="custom"
      visible={state.open}
      onVisibleChange={state.setOpen}
      showArrow={false}
    >
      {unreadCount > 0 ? (
        <Badge count={displayUnreadCount} type="danger">
          {trigger}
        </Badge>
      ) : (
        trigger
      )}
    </Popover>
  );
};
