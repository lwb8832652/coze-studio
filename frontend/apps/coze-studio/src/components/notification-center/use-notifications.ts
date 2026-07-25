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

/* eslint-disable @coze-arch/max-line-per-function -- State-machine refs share one hook lifecycle. */
/* eslint-disable max-lines, max-lines-per-function -- Read recovery is one cohesive state machine. */

import {
  type SetStateAction,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';

import { ReadStatus } from '@coze-studio/api-schema/playground';
import { useUserInfo } from '@coze-arch/foundation-sdk';

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

interface PendingReadRecovery {
  accountGeneration: number;
  notificationIDs: Set<string>;
  unreadFloor: number;
  waiters: Array<() => void>;
}

const useNotificationUnreadPolling = (
  accountGeneration: number,
  authenticated: boolean,
  isAccountCurrent: (generation: number) => boolean,
) => {
  const [unreadCount, setUnreadCount] = useState(0);
  const [unreadError, setUnreadError] = useState('');
  const mountedRef = useRef(false);
  const requestSequenceRef = useRef(0);
  const unreadRevisionRef = useRef(0);

  const invalidateUnreadRequests = useCallback(() => {
    requestSequenceRef.current += 1;
  }, []);

  const refreshUnreadCountWithResult = useCallback(async () => {
    if (!authenticated || !isAccountCurrent(accountGeneration)) {
      return false;
    }
    const sequence = ++requestSequenceRef.current;
    const requestGeneration = accountGeneration;
    try {
      const count = await getNotificationUnreadCount();
      if (
        mountedRef.current &&
        sequence === requestSequenceRef.current &&
        isAccountCurrent(requestGeneration)
      ) {
        unreadRevisionRef.current += 1;
        setUnreadCount(count);
        setUnreadError('');
        return true;
      }
      return false;
    } catch (error) {
      if (
        mountedRef.current &&
        sequence === requestSequenceRef.current &&
        isAccountCurrent(requestGeneration)
      ) {
        setUnreadError(
          error instanceof Error ? '未读通知数量加载失败' : '通知服务暂不可用',
        );
      }
      return false;
    }
  }, [accountGeneration, authenticated, isAccountCurrent]);

  const refreshUnreadCount = useCallback(async () => {
    await refreshUnreadCountWithResult();
  }, [refreshUnreadCountWithResult]);

  useEffect(() => {
    mountedRef.current = true;
    if (!authenticated) {
      requestSequenceRef.current += 1;
      setUnreadCount(0);
      setUnreadError('');
      return () => {
        mountedRef.current = false;
        requestSequenceRef.current += 1;
      };
    }
    void refreshUnreadCount();

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        void refreshUnreadCount();
      }
    };
    const handleWindowFocus = () => {
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
    window.addEventListener('focus', handleWindowFocus);

    return () => {
      mountedRef.current = false;
      requestSequenceRef.current += 1;
      window.clearInterval(interval);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      window.removeEventListener('focus', handleWindowFocus);
    };
  }, [accountGeneration, authenticated, refreshUnreadCount]);

  return {
    invalidateUnreadRequests,
    refreshUnreadCount,
    refreshUnreadCountWithResult,
    setUnreadCount,
    setUnreadError,
    unreadCount,
    unreadError,
    unreadRevisionRef,
  };
};

export const useNotifications = () => {
  const userInfo = useUserInfo();
  const sessionKey = String(userInfo?.user_id_str ?? '');
  const authenticated = sessionKey !== '';
  const accountRef = useRef({
    generation: 0,
    sessionKey,
  });
  if (accountRef.current.sessionKey !== sessionKey) {
    accountRef.current = {
      generation: accountRef.current.generation + 1,
      sessionKey,
    };
  }
  const accountGeneration = accountRef.current.generation;
  const [stateAccountKey, setStateAccountKey] = useState(sessionKey);
  const [open, setOpen] = useState(false);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [paginationError, setPaginationError] = useState('');
  const [hasMore, setHasMore] = useState(false);
  const [nextCursor, setNextCursor] = useState('');
  const [snapshotCutoff, setSnapshotCutoff] = useState('0');
  const mountedRef = useRef(false);
  const listRequestSequenceRef = useRef(0);
  const readMutationSequenceRef = useRef(0);
  const readMutationByIDRef = useRef(new Map<string, number>());
  const markAllMutationRef = useRef(0);
  const readMutationGenerationRef = useRef(0);
  const activeReadMutationCountRef = useRef(0);
  const readRecoverySequenceRef = useRef(0);
  const pendingReadRecoveryRef = useRef<PendingReadRecovery | null>(null);
  const isAccountCurrent = useCallback(
    (generation: number) => accountRef.current.generation === generation,
    [],
  );
  const {
    invalidateUnreadRequests,
    refreshUnreadCount,
    refreshUnreadCountWithResult,
    setUnreadCount,
    setUnreadError,
    unreadCount,
    unreadError,
    unreadRevisionRef,
  } = useNotificationUnreadPolling(
    accountGeneration,
    authenticated,
    isAccountCurrent,
  );

  const loadFirstPage = useCallback(async () => {
    if (!authenticated || !isAccountCurrent(accountGeneration)) {
      return false;
    }
    const sequence = ++listRequestSequenceRef.current;
    const requestGeneration = accountGeneration;
    setLoading(true);
    setLoadingMore(false);
    setError('');
    setPaginationError('');
    try {
      const page = await listNotifications();
      if (
        !mountedRef.current ||
        sequence !== listRequestSequenceRef.current ||
        !isAccountCurrent(requestGeneration)
      ) {
        return false;
      }
      setNotifications(page.notifications);
      setNextCursor(page.nextCursor);
      setHasMore(page.hasMore);
      setSnapshotCutoff(page.snapshotCutoff);
      return true;
    } catch (cause) {
      if (
        mountedRef.current &&
        sequence === listRequestSequenceRef.current &&
        isAccountCurrent(requestGeneration)
      ) {
        setError(
          cause instanceof Error
            ? '通知加载失败，请稍后重试'
            : '通知服务暂不可用，请稍后重试',
        );
      }
      return false;
    } finally {
      if (
        mountedRef.current &&
        sequence === listRequestSequenceRef.current &&
        isAccountCurrent(requestGeneration)
      ) {
        setLoading(false);
      }
    }
  }, [accountGeneration, authenticated, isAccountCurrent]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      listRequestSequenceRef.current += 1;
      readMutationByIDRef.current.clear();
      markAllMutationRef.current += 1;
      readMutationGenerationRef.current += 1;
      activeReadMutationCountRef.current = 0;
      readRecoverySequenceRef.current += 1;
      const pendingRecovery = pendingReadRecoveryRef.current;
      pendingReadRecoveryRef.current = null;
      pendingRecovery?.waiters.forEach(resolve => resolve());
    };
  }, []);

  useEffect(() => {
    listRequestSequenceRef.current += 1;
    readMutationByIDRef.current.clear();
    markAllMutationRef.current += 1;
    readMutationGenerationRef.current += 1;
    activeReadMutationCountRef.current = 0;
    readRecoverySequenceRef.current += 1;
    const pendingRecovery = pendingReadRecoveryRef.current;
    pendingReadRecoveryRef.current = null;
    pendingRecovery?.waiters.forEach(resolve => resolve());
    unreadRevisionRef.current += 1;
    setStateAccountKey(sessionKey);
    setOpen(false);
    setNotifications([]);
    setLoading(false);
    setLoadingMore(false);
    setError('');
    setPaginationError('');
    setHasMore(false);
    setNextCursor('');
    setSnapshotCutoff('0');
    setUnreadCount(0);
  }, [accountGeneration, sessionKey, setUnreadCount]);

  useEffect(() => {
    if (authenticated && open) {
      void loadFirstPage();
      void refreshUnreadCount();
    }
  }, [authenticated, loadFirstPage, open, refreshUnreadCount]);

  const loadMore = useCallback(async () => {
    if (
      !isAccountCurrent(accountGeneration) ||
      !authenticated ||
      !hasMore ||
      loadingMore ||
      !nextCursor
    ) {
      return;
    }
    const sequence = ++listRequestSequenceRef.current;
    const requestGeneration = accountGeneration;
    const requestedCursor = nextCursor;
    const requestedCutoff = snapshotCutoff;
    setLoadingMore(true);
    setPaginationError('');
    try {
      const page = await listNotifications(requestedCursor);
      if (
        !mountedRef.current ||
        sequence !== listRequestSequenceRef.current ||
        !isAccountCurrent(requestGeneration)
      ) {
        return;
      }
      if (page.snapshotCutoff !== requestedCutoff) {
        setPaginationError('通知快照已更新，请重新打开通知中心');
        return;
      }
      setNotifications(current =>
        mergeNotifications(current, page.notifications),
      );
      setNextCursor(page.nextCursor);
      setHasMore(page.hasMore);
    } catch (cause) {
      if (
        mountedRef.current &&
        sequence === listRequestSequenceRef.current &&
        isAccountCurrent(requestGeneration)
      ) {
        setPaginationError(
          cause instanceof Error
            ? '更多通知加载失败，请重试'
            : '通知服务暂不可用',
        );
      }
    } finally {
      if (
        mountedRef.current &&
        sequence === listRequestSequenceRef.current &&
        isAccountCurrent(requestGeneration)
      ) {
        setLoadingMore(false);
      }
    }
  }, [
    accountGeneration,
    authenticated,
    hasMore,
    isAccountCurrent,
    loadingMore,
    nextCursor,
    snapshotCutoff,
  ]);

  const beginReadMutation = useCallback(() => {
    const overlappedAtStart = activeReadMutationCountRef.current > 0;
    activeReadMutationCountRef.current += 1;
    return {
      generation: ++readMutationGenerationRef.current,
      overlappedAtStart,
    };
  }, []);

  const hasConcurrentReadMutation = useCallback(
    (ticket: { generation: number; overlappedAtStart: boolean }) =>
      ticket.overlappedAtStart ||
      ticket.generation !== readMutationGenerationRef.current ||
      activeReadMutationCountRef.current > 1,
    [],
  );

  const queueAuthoritativeReadRecovery = useCallback(
    (
      requestGeneration: number,
      notificationIDs: Iterable<string>,
      unreadFloor: number,
    ) =>
      new Promise<void>(resolve => {
        let pending = pendingReadRecoveryRef.current;
        if (!pending || pending.accountGeneration !== requestGeneration) {
          pending?.waiters.forEach(settle => settle());
          pending = {
            accountGeneration: requestGeneration,
            notificationIDs: new Set<string>(),
            unreadFloor: 0,
            waiters: [],
          };
          pendingReadRecoveryRef.current = pending;
        }
        for (const notificationID of notificationIDs) {
          pending.notificationIDs.add(notificationID);
        }
        pending.unreadFloor = Math.max(pending.unreadFloor, unreadFloor);
        pending.waiters.push(() => resolve());
      }),
    [],
  );

  const restorePendingReadRecovery = useCallback(
    (pending: PendingReadRecovery) => {
      const { current } = pendingReadRecoveryRef;
      if (!current || current.accountGeneration !== pending.accountGeneration) {
        current?.waiters.forEach(resolve => resolve());
        pendingReadRecoveryRef.current = pending;
        return;
      }
      for (const notificationID of pending.notificationIDs) {
        current.notificationIDs.add(notificationID);
      }
      current.unreadFloor = Math.max(current.unreadFloor, pending.unreadFloor);
      current.waiters.push(...pending.waiters);
    },
    [],
  );

  const runPendingReadRecovery = useCallback(() => {
    if (activeReadMutationCountRef.current !== 0) {
      return;
    }
    const pending = pendingReadRecoveryRef.current;
    if (!pending) {
      return;
    }
    pendingReadRecoveryRef.current = null;
    const recoveryCancelSequence = readRecoverySequenceRef.current;
    const recoveryMutationGeneration = readMutationGenerationRef.current;
    void (async () => {
      let listRecovered = false;
      let unreadRecovered = false;
      let recoveredPage: Awaited<ReturnType<typeof listNotifications>> | null =
        null;
      let recoveredUnreadCount = 0;
      let pendingRestored = false;
      try {
        if (mountedRef.current && isAccountCurrent(pending.accountGeneration)) {
          [listRecovered, unreadRecovered] = await Promise.all([
            listNotifications()
              .then(page => {
                recoveredPage = page;
                return true;
              })
              .catch(() => false),
            getNotificationUnreadCount()
              .then(count => {
                recoveredUnreadCount = count;
                return true;
              })
              .catch(() => false),
          ]);
        }
        if (
          !mountedRef.current ||
          recoveryCancelSequence !== readRecoverySequenceRef.current ||
          !isAccountCurrent(pending.accountGeneration)
        ) {
          return;
        }
        if (recoveryMutationGeneration !== readMutationGenerationRef.current) {
          restorePendingReadRecovery(pending);
          pendingRestored = true;
          if (activeReadMutationCountRef.current === 0) {
            runPendingReadRecovery();
          }
          return;
        }
        if (listRecovered && recoveredPage) {
          listRequestSequenceRef.current += 1;
          setNotifications(recoveredPage.notifications);
          setNextCursor(recoveredPage.nextCursor);
          setHasMore(recoveredPage.hasMore);
          setSnapshotCutoff(recoveredPage.snapshotCutoff);
          setLoading(false);
          setLoadingMore(false);
          setPaginationError('');
        }
        if (unreadRecovered) {
          invalidateUnreadRequests();
          unreadRevisionRef.current += 1;
          setUnreadCount(recoveredUnreadCount);
          setUnreadError('');
        }
        if (listRecovered && unreadRecovered) {
          setError('');
          return;
        }
        if (!listRecovered) {
          setNotifications(current =>
            current.map(notification =>
              notification.id && pending.notificationIDs.has(notification.id)
                ? { ...notification, read_status: ReadStatus.Unread }
                : notification,
            ),
          );
        }
        if (!unreadRecovered) {
          unreadRevisionRef.current += 1;
          setUnreadCount(current =>
            Math.max(
              current,
              pending.unreadFloor,
              pending.notificationIDs.size,
            ),
          );
        }
        setError('通知状态同步失败，请重试');
      } finally {
        if (!pendingRestored) {
          pending.waiters.forEach(resolve => resolve());
        }
      }
    })();
  }, [
    invalidateUnreadRequests,
    isAccountCurrent,
    restorePendingReadRecovery,
    setUnreadCount,
    setUnreadError,
    unreadRevisionRef,
  ]);

  const finishReadMutation = useCallback(
    (requestGeneration: number) => {
      if (!isAccountCurrent(requestGeneration)) {
        return;
      }
      activeReadMutationCountRef.current = Math.max(
        0,
        activeReadMutationCountRef.current - 1,
      );
      if (activeReadMutationCountRef.current === 0) {
        runPendingReadRecovery();
      }
    },
    [isAccountCurrent, runPendingReadRecovery],
  );

  const markRead = useCallback(
    async (notification: Notification) => {
      if (!notification.id || notification.read_status === ReadStatus.Read) {
        return;
      }
      if (!authenticated || !isAccountCurrent(accountGeneration)) {
        return;
      }
      invalidateUnreadRequests();
      const requestGeneration = accountGeneration;
      const mutation = ++readMutationSequenceRef.current;
      const markAllMutation = markAllMutationRef.current;
      const readMutationTicket = beginReadMutation();
      const conservativeUnreadFloor = unreadCount;
      const unreadRevision = ++unreadRevisionRef.current;
      readMutationByIDRef.current.set(notification.id, mutation);
      let mutationFinished = false;
      const finishCurrentMutation = () => {
        if (mutationFinished) {
          return;
        }
        mutationFinished = true;
        if (readMutationByIDRef.current.get(notification.id) === mutation) {
          readMutationByIDRef.current.delete(notification.id);
        }
        finishReadMutation(requestGeneration);
      };
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
        if (
          mountedRef.current &&
          isAccountCurrent(requestGeneration) &&
          readMutationByIDRef.current.get(notification.id) === mutation &&
          markAllMutationRef.current === markAllMutation
        ) {
          if (!hasConcurrentReadMutation(readMutationTicket)) {
            await refreshUnreadCount();
          }
        }
      } catch (cause) {
        let recovery: Promise<void> | undefined;
        if (mountedRef.current && isAccountCurrent(requestGeneration)) {
          if (hasConcurrentReadMutation(readMutationTicket)) {
            recovery = queueAuthoritativeReadRecovery(
              requestGeneration,
              [notification.id],
              conservativeUnreadFloor,
            );
          } else if (
            readMutationByIDRef.current.get(notification.id) === mutation &&
            markAllMutationRef.current === markAllMutation
          ) {
            setNotifications(current =>
              current.map(item =>
                item.id === notification.id
                  ? { ...item, read_status: ReadStatus.Unread }
                  : item,
              ),
            );
            setUnreadCount(current =>
              unreadRevisionRef.current === unreadRevision
                ? (() => {
                    unreadRevisionRef.current += 1;
                    return current + 1;
                  })()
                : current,
            );
          }
        }
        if (recovery) {
          finishCurrentMutation();
          await recovery;
        }
        throw cause;
      } finally {
        finishCurrentMutation();
      }
    },
    [
      accountGeneration,
      authenticated,
      beginReadMutation,
      finishReadMutation,
      hasConcurrentReadMutation,
      invalidateUnreadRequests,
      isAccountCurrent,
      queueAuthoritativeReadRecovery,
      refreshUnreadCount,
      setUnreadCount,
      unreadCount,
      unreadRevisionRef,
    ],
  );

  const markAllRead = useCallback(async () => {
    if (
      !isAccountCurrent(accountGeneration) ||
      !authenticated ||
      unreadCount === 0
    ) {
      return;
    }
    if (!/^[1-9]\d*$/.test(snapshotCutoff)) {
      throw new Error('通知快照尚未加载完成');
    }
    invalidateUnreadRequests();
    const requestGeneration = accountGeneration;
    const mutation = ++markAllMutationRef.current;
    const readMutationTicket = beginReadMutation();
    const conservativeUnreadFloor = unreadCount;
    const optimisticIDs = new Set(
      notifications
        .filter(
          notification =>
            notification.id && notification.read_status !== ReadStatus.Read,
        )
        .map(notification => notification.id as string),
    );
    const unreadRevision = ++unreadRevisionRef.current;
    let mutationFinished = false;
    const finishCurrentMutation = () => {
      if (mutationFinished) {
        return;
      }
      mutationFinished = true;
      finishReadMutation(requestGeneration);
    };
    setNotifications(current =>
      current.map(notification =>
        notification.id && optimisticIDs.has(notification.id)
          ? { ...notification, read_status: ReadStatus.Read }
          : notification,
      ),
    );
    setUnreadCount(current => Math.max(0, current - optimisticIDs.size));
    try {
      await markAllNotificationsRead(snapshotCutoff);
      if (!hasConcurrentReadMutation(readMutationTicket)) {
        await refreshUnreadCount();
      }
    } catch (cause) {
      let recovery: Promise<void> | undefined;
      if (mountedRef.current && isAccountCurrent(requestGeneration)) {
        if (hasConcurrentReadMutation(readMutationTicket)) {
          recovery = queueAuthoritativeReadRecovery(
            requestGeneration,
            optimisticIDs,
            conservativeUnreadFloor,
          );
        } else if (markAllMutationRef.current === mutation) {
          setNotifications(current =>
            current.map(notification =>
              notification.id &&
              optimisticIDs.has(notification.id) &&
              notification.read_status === ReadStatus.Read
                ? { ...notification, read_status: ReadStatus.Unread }
                : notification,
            ),
          );
          setUnreadCount(current =>
            unreadRevisionRef.current === unreadRevision
              ? (() => {
                  unreadRevisionRef.current += 1;
                  return current + optimisticIDs.size;
                })()
              : current,
          );
        }
      }
      if (recovery) {
        finishCurrentMutation();
        await recovery;
      }
      throw cause;
    } finally {
      finishCurrentMutation();
    }
  }, [
    accountGeneration,
    authenticated,
    beginReadMutation,
    finishReadMutation,
    hasConcurrentReadMutation,
    invalidateUnreadRequests,
    isAccountCurrent,
    notifications,
    queueAuthoritativeReadRecovery,
    refreshUnreadCount,
    setUnreadCount,
    snapshotCutoff,
    unreadCount,
    unreadRevisionRef,
  ]);

  const retryNotificationState = useCallback(async () => {
    const [listRecovered, unreadRecovered] = await Promise.all([
      loadFirstPage(),
      refreshUnreadCountWithResult(),
    ]);
    if (!mountedRef.current || !isAccountCurrent(accountGeneration)) {
      return;
    }
    if (listRecovered && unreadRecovered) {
      setError('');
    } else if (listRecovered) {
      setError('通知状态同步失败，请重试');
    }
  }, [
    accountGeneration,
    isAccountCurrent,
    loadFirstPage,
    refreshUnreadCountWithResult,
  ]);

  const stateIsCurrent = authenticated && stateAccountKey === sessionKey;
  const setNotificationOpen = useCallback(
    (value: SetStateAction<boolean>) => {
      if (!authenticated) {
        setOpen(false);
        return;
      }
      setOpen(value);
    },
    [authenticated],
  );

  return {
    canMarkAllRead: stateIsCurrent && /^[1-9]\d*$/.test(snapshotCutoff),
    error: stateIsCurrent ? error : '',
    hasMore: stateIsCurrent && hasMore,
    loadMore,
    loading: stateIsCurrent && loading,
    loadingMore: stateIsCurrent && loadingMore,
    markAllRead,
    markRead,
    notifications: stateIsCurrent ? notifications : [],
    open: stateIsCurrent && open,
    paginationError: stateIsCurrent ? paginationError : '',
    retry: retryNotificationState,
    retryLoadMore: loadMore,
    refreshUnreadCount,
    setOpen: setNotificationOpen,
    unreadCount: stateIsCurrent ? unreadCount : 0,
    unreadError: stateIsCurrent ? unreadError : '',
  };
};
