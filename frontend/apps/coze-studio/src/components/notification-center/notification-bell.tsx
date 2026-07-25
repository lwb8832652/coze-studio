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
import { useCallback, useRef } from 'react';

import { ReadStatus } from '@coze-studio/api-schema/playground';
import { useSpaceStore } from '@coze-foundation/space-store';
import {
  IconCozBell,
  IconCozBot,
  IconCozRefresh,
} from '@coze-arch/coze-design/icons';
import { Badge, Popover, Toast } from '@coze-arch/coze-design';

import { useNotifications } from './use-notifications';
import {
  type Notification,
  NotificationCategory,
  NotificationRoute,
  NotificationSeverity,
} from './service';

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

const safeRouteValue = (value?: string) => {
  const normalized = value?.trim() ?? '';
  if (!normalized || !/^[A-Za-z0-9][A-Za-z0-9._:@-]*$/.test(normalized)) {
    return '';
  }
  return normalized;
};

const isSpaceScopedRoute = (route?: NotificationRoute) =>
  route === NotificationRoute.TaskThread ||
  route === NotificationRoute.ScheduledTaskCenter ||
  route === NotificationRoute.AppDev ||
  route === NotificationRoute.Skill ||
  route === NotificationRoute.Workspace;

export const getNotificationTarget = (
  notification: Partial<Notification>,
  activeSpaceID?: string,
) => {
  const routeSpaceID = safeRouteValue(notification.route_space_id);
  const activeSpace = safeRouteValue(activeSpaceID);
  if (
    isSpaceScopedRoute(notification.route) &&
    (!routeSpaceID || routeSpaceID !== activeSpace)
  ) {
    return '';
  }
  const spaceID = encodeURIComponent(routeSpaceID);
  const routeTargetID = safeRouteValue(notification.route_target_id);
  const normalizedTargetID =
    notification.route === NotificationRoute.TaskThread &&
    routeTargetID.startsWith('thread:')
      ? routeTargetID.slice('thread:'.length)
      : routeTargetID;
  const targetID = encodeURIComponent(safeRouteValue(normalizedTargetID));
  switch (notification.route) {
    case NotificationRoute.TaskThread:
      return spaceID && targetID ? `/space/${spaceID}/tasks/${targetID}` : '';
    case NotificationRoute.ScheduledTaskCenter:
      return spaceID ? `/space/${spaceID}/task-center` : '';
    case NotificationRoute.AppDev:
      return spaceID && targetID ? `/space/${spaceID}/app-dev/${targetID}` : '';
    case NotificationRoute.Skill:
      return spaceID ? `/space/${spaceID}/skill` : '';
    case NotificationRoute.Workspace:
      return spaceID ? `/space/${spaceID}/workspace` : '';
    case NotificationRoute.Billing:
      return '/billing/subscriptions';
    case NotificationRoute.SystemAnnouncements:
      return '/system/announcements';
    default:
      return '';
  }
};

const getNotificationTitle = (notification: Notification) =>
  notification.sender?.sender_name || '系统通知';

const getSeverityName = (severity?: NotificationSeverity) => {
  switch (severity) {
    case NotificationSeverity.Success:
      return 'success';
    case NotificationSeverity.Warning:
      return 'warning';
    case NotificationSeverity.Error:
      return 'error';
    default:
      return 'info';
  }
};

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
        disabled={unreadCount === 0 || !state.canMarkAllRead}
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
    {state.unreadError ? (
      <div className="notification-center__polling-error" role="status">
        <span>{state.unreadError}</span>
        <button
          type="button"
          onClick={() => {
            void state.refreshUnreadCount();
          }}
        >
          重新同步
        </button>
      </div>
    ) : null}
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
                data-category={
                  notification.category ?? NotificationCategory.Task
                }
                data-severity={getSeverityName(notification.severity)}
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
            <>
              {state.paginationError ? (
                <div className="notification-center__pagination-error">
                  <span>{state.paginationError}</span>
                  <button
                    type="button"
                    disabled={state.loadingMore}
                    onClick={() => void state.retryLoadMore()}
                  >
                    重试
                  </button>
                </div>
              ) : null}
              <button
                type="button"
                className="notification-center__load-more"
                disabled={state.loadingMore}
                onClick={() => void state.loadMore()}
              >
                {state.loadingMore ? '正在加载...' : '加载更多'}
              </button>
            </>
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
  const activeSpaceID = useSpaceStore(store => store.space.id);
  const setSpace = useSpaceStore(store => store.setSpace);
  const fetchSpaces = useSpaceStore(store => store.fetchSpaces);
  const unreadCount = unreadCountOverride ?? state.unreadCount;
  const displayUnreadCount =
    unreadCount > MAX_VISIBLE_UNREAD_COUNT ? '99+' : unreadCount;
  const ariaLabel = unreadCount > 0 ? `通知，${unreadCount} 条未读` : '通知';
  const visibilityChangeSequenceRef = useRef(0);

  const handlePopoverVisibleChange = useCallback(
    (nextOpen: boolean) => {
      const sequence = ++visibilityChangeSequenceRef.current;
      queueMicrotask(() => {
        if (visibilityChangeSequenceRef.current === sequence) {
          state.setOpen(nextOpen);
        }
      });
    },
    [state.setOpen],
  );

  const markReadBestEffort = (notification: Notification) => {
    void state.markRead(notification).catch(error => {
      Toast.error({
        content:
          error instanceof Error
            ? '标记通知已读失败，请稍后重试'
            : '通知服务暂不可用',
      });
    });
  };

  const handleNotificationClick = async (notification: Notification) => {
    markReadBestEffort(notification);
    const routeSpaceID = safeRouteValue(notification.route_space_id);
    const navigationSpaceID = routeSpaceID;
    const hasTarget =
      notification.route !== undefined &&
      notification.route !== NotificationRoute.None;
    if (!hasTarget) {
      return;
    }

    const target = getNotificationTarget(
      notification,
      routeSpaceID || activeSpaceID,
    );
    if (!target) {
      Toast.error({ content: '该通知的目标已失效' });
      return;
    }

    if (isSpaceScopedRoute(notification.route)) {
      let accessibleSpaces: Array<{ id?: string }> = [];
      try {
        const spaceInfo = await fetchSpaces(true);
        accessibleSpaces = spaceInfo?.bot_space_list ?? [];
      } catch {
        Toast.error({ content: '无法验证目标工作空间权限，请稍后重试' });
        return;
      }
      if (!accessibleSpaces.some(space => space.id === navigationSpaceID)) {
        Toast.error({ content: '你已不在该通知对应的工作空间' });
        return;
      }
      if (navigationSpaceID !== safeRouteValue(activeSpaceID)) {
        try {
          setSpace(navigationSpaceID);
        } catch {
          Toast.error({ content: '切换目标工作空间失败，请稍后重试' });
          return;
        }
      }
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
      onVisibleChange={handlePopoverVisibleChange}
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
