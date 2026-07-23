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

import { useCallback, useEffect, useRef, useState } from 'react';

import { ReadStatus } from '@coze-arch/bot-api/playground_api';

import {
  getNotificationUnreadCount,
  listNotifications,
  markAllNotificationsRead,
  markNotificationsRead,
  type Notification,
} from './service';

const NOTIFICATION_POLL_INTERVAL = 30_000;

const mergeNotifications = (
  current: Notification[],
  incoming: Notification[],
) => {
  const merged = new Map<string, Notification>();
  [...current, ...incoming].forEach(notification => {
    if (notification.id) {
      merged.set(notification.id, notification);
    }
  });
  return Array.from(merged.values()).sort(
    (left, right) =>
      Number(right.create_time ?? 0) - Number(left.create_time ?? 0),
  );
};

const useNotificationUnreadPolling = () => {
  const [unreadCount, setUnreadCount] = useState(0);
  const [unreadError, setUnreadError] = useState('');
  const mountedRef = useRef(false);

  const refreshUnreadCount = useCallback(async () => {
    try {
      const count = await getNotificationUnreadCount();
      if (mountedRef.current) {
        setUnreadCount(count);
        setUnreadError('');
      }
    } catch (error) {
      if (mountedRef.current) {
        setUnreadError(
          error instanceof Error ? '未读通知数量加载失败' : '通知服务暂不可用',
        );
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void refreshUnreadCount();

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        void refreshUnreadCount();
      }
    };
    const interval = window.setInterval(() => {
      if (document.visibilityState === 'visible') {
        void refreshUnreadCount();
      }
    }, NOTIFICATION_POLL_INTERVAL);
    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      mountedRef.current = false;
      window.clearInterval(interval);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [refreshUnreadCount]);

  return {
    refreshUnreadCount,
    setUnreadCount,
    unreadCount,
    unreadError,
  };
};

export const useNotifications = () => {
  const [open, setOpen] = useState(false);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [hasMore, setHasMore] = useState(false);
  const [nextCursor, setNextCursor] = useState('');
  const [loaded, setLoaded] = useState(false);
  const mountedRef = useRef(false);
  const requestSequenceRef = useRef(0);
  const { setUnreadCount, unreadCount, unreadError } =
    useNotificationUnreadPolling();

  const loadFirstPage = useCallback(async () => {
    const sequence = ++requestSequenceRef.current;
    setLoading(true);
    setError('');
    try {
      const page = await listNotifications();
      if (!mountedRef.current || sequence !== requestSequenceRef.current) {
        return;
      }
      setNotifications(page.notifications);
      setNextCursor(page.nextCursor);
      setHasMore(page.hasMore);
      setLoaded(true);
    } catch (cause) {
      if (mountedRef.current && sequence === requestSequenceRef.current) {
        setLoaded(true);
        setError(
          cause instanceof Error
            ? '通知加载失败，请稍后重试'
            : '通知服务暂不可用，请稍后重试',
        );
      }
    } finally {
      if (mountedRef.current && sequence === requestSequenceRef.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      requestSequenceRef.current += 1;
    };
  }, []);

  useEffect(() => {
    if (open && !loaded && !loading) {
      void loadFirstPage();
    }
  }, [loadFirstPage, loaded, loading, open]);

  const loadMore = useCallback(async () => {
    if (!hasMore || loadingMore || !nextCursor) {
      return;
    }
    setLoadingMore(true);
    try {
      const page = await listNotifications(nextCursor);
      if (!mountedRef.current) {
        return;
      }
      setNotifications(current =>
        mergeNotifications(current, page.notifications),
      );
      setNextCursor(page.nextCursor);
      setHasMore(page.hasMore);
    } finally {
      if (mountedRef.current) {
        setLoadingMore(false);
      }
    }
  }, [hasMore, loadingMore, nextCursor]);

  const markRead = useCallback(
    async (notification: Notification) => {
      if (!notification.id || notification.read_status === ReadStatus.Read) {
        return;
      }
      const previousUnreadCount = unreadCount;
      setNotifications(current =>
        current.map(item =>
          item.id === notification.id
            ? { ...item, read_status: ReadStatus.Read }
            : item,
        ),
      );
      setUnreadCount(current => Math.max(0, current - 1));
      try {
        await markNotificationsRead([notification.id]);
      } catch (cause) {
        if (mountedRef.current) {
          setNotifications(current =>
            current.map(item =>
              item.id === notification.id ? notification : item,
            ),
          );
          setUnreadCount(previousUnreadCount);
        }
        throw cause;
      }
    },
    [setUnreadCount, unreadCount],
  );

  const markAllRead = useCallback(async () => {
    if (unreadCount === 0) {
      return;
    }
    const previousNotifications = notifications;
    const previousUnreadCount = unreadCount;
    setNotifications(current =>
      current.map(notification => ({
        ...notification,
        read_status: ReadStatus.Read,
      })),
    );
    setUnreadCount(0);
    try {
      await markAllNotificationsRead();
    } catch (cause) {
      if (mountedRef.current) {
        setNotifications(previousNotifications);
        setUnreadCount(previousUnreadCount);
      }
      throw cause;
    }
  }, [notifications, setUnreadCount, unreadCount]);

  return {
    error,
    hasMore,
    loadMore,
    loading,
    loadingMore,
    markAllRead,
    markRead,
    notifications,
    open,
    retry: loadFirstPage,
    setOpen,
    unreadCount,
    unreadError,
  };
};
