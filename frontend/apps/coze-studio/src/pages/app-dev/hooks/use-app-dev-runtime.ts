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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, complexity, max-lines, max-params -- Runtime lifecycle fencing is intentionally colocated. */

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  clearAppDevPendingOperation,
  createAppDevOperationID,
  APP_DEV_OPERATION_STORAGE_WARNING,
  type AppDevPendingOperationKind,
  readAppDevPendingOperationResult,
  writeAppDevPendingOperation,
} from '../utils/provider-operation';
import type { AppDevRuntimeInfo, AppDevRuntimeLog } from '../types';
import {
  getAppDevRuntimeStatus,
  isAppDevDeterministicError,
  keepAliveAppDevRuntime,
  listAppDevRuntimeLogs,
  normalizeAppDevError,
  restartAppDevRuntime,
  startAppDevRuntime,
  stopAppDevRuntime,
} from '../service';

const initialRuntime: AppDevRuntimeInfo = {
  generation: 0,
  status: 'stopped',
  canStart: false,
  recovering: false,
  stopping: false,
  message: '正在确认开发环境状态',
};

type RuntimeMutation = typeof startAppDevRuntime;
type RuntimeIntent = Extract<
  AppDevPendingOperationKind,
  | 'runtime-auto-start'
  | 'runtime-manual-start'
  | 'runtime-stop'
  | 'runtime-restart'
>;

const mutationForIntent: Record<RuntimeIntent, RuntimeMutation> = {
  'runtime-auto-start': startAppDevRuntime,
  'runtime-manual-start': startAppDevRuntime,
  'runtime-stop': stopAppDevRuntime,
  'runtime-restart': restartAppDevRuntime,
};

const runtimeIntents: RuntimeIntent[] = [
  'runtime-auto-start',
  'runtime-manual-start',
  'runtime-stop',
  'runtime-restart',
];

export const useAppDevRuntime = (
  spaceId?: string,
  projectId?: string,
  principalId?: string,
) => {
  const [runtime, setRuntime] = useState<AppDevRuntimeInfo>(initialRuntime);
  const [logs, setLogs] = useState<AppDevRuntimeLog[]>([]);
  const [loading, setLoading] = useState(false);
  const [logsLoading, setLogsLoading] = useState(false);
  const [error, setError] = useState('');
  const identityEpochRef = useRef(0);
  const observationSequenceRef = useRef(0);
  const latestGenerationRef = useRef(0);
  const logSequenceRef = useRef(0);
  const autoDecisionRef = useRef(false);
  const suppressAutoStartRef = useRef(false);
  const statusRequestRef = useRef<{
    key: string;
    sequence: number;
    promise: Promise<AppDevRuntimeInfo | undefined>;
    controller: AbortController;
  }>();
  const logsRequestRef = useRef<{
    key: string;
    sequence: number;
    generation: number;
    promise: Promise<void>;
    controller: AbortController;
  }>();
  const mutationRef = useRef<{
    intent: RuntimeIntent;
    operationId: string;
    promise: Promise<void>;
    controller: AbortController;
  }>();
  const keepAliveRef = useRef<{
    promise: Promise<void>;
    controller: AbortController;
  }>();
  const identity =
    spaceId && projectId ? { spaceId, projectId, principalId } : undefined;
  const identityKey = identity
    ? `${identity.spaceId}\u0000${identity.projectId}\u0000${principalId || ''}`
    : '';

  const currentIdentity = (epoch: number, key: string) =>
    identityEpochRef.current === epoch && identityKey === key;

  const invalidateObservations = () => {
    statusRequestRef.current?.controller.abort();
    keepAliveRef.current?.controller.abort();
    statusRequestRef.current = undefined;
    keepAliveRef.current = undefined;
    return ++observationSequenceRef.current;
  };

  const invalidateLogs = () => {
    logSequenceRef.current += 1;
    logsRequestRef.current?.controller.abort();
    logsRequestRef.current = undefined;
    setLogsLoading(false);
  };

  const reportStorageWarning = (warning?: string) => {
    if (warning) {
      setError(
        warning === APP_DEV_OPERATION_STORAGE_WARNING
          ? warning
          : '操作恢复状态已失效，请重新发起操作',
      );
    }
  };

  const readPending = (intent: AppDevPendingOperationKind) => {
    if (!identity) {
      return undefined;
    }
    const result = readAppDevPendingOperationResult(identity, intent);
    reportStorageWarning(result.warning);
    return result.value;
  };

  const commitObservation = (
    next: AppDevRuntimeInfo,
    epoch: number,
    key: string,
    sequence: number,
  ) => {
    if (
      !currentIdentity(epoch, key) ||
      observationSequenceRef.current !== sequence ||
      next.generation < latestGenerationRef.current
    ) {
      return false;
    }
    if (next.generation !== latestGenerationRef.current) {
      invalidateLogs();
      setLogs([]);
    }
    latestGenerationRef.current = next.generation;
    setRuntime(previous => ({
      ...next,
      lastKeepAliveAt:
        previous.generation === next.generation
          ? previous.lastKeepAliveAt
          : undefined,
    }));
    return true;
  };

  const refreshStatusInternal = useCallback(
    (force = false): Promise<AppDevRuntimeInfo | undefined> => {
      if (!identity) {
        setRuntime(initialRuntime);
        return Promise.resolve(undefined);
      }
      if (!force && statusRequestRef.current?.key === identityKey) {
        return statusRequestRef.current.promise;
      }
      statusRequestRef.current?.controller.abort();
      const controller = new AbortController();
      const epoch = identityEpochRef.current;
      const key = identityKey;
      const sequence = ++observationSequenceRef.current;
      const promise = getAppDevRuntimeStatus({
        ...identity,
        signal: controller.signal,
      })
        .then(result => {
          if (!commitObservation(result, epoch, key, sequence)) {
            return undefined;
          }
          setError('');
          return result;
        })
        .catch(requestError => {
          if (!controller.signal.aborted && currentIdentity(epoch, key)) {
            setError(normalizeAppDevError(requestError));
          }
          return undefined;
        })
        .finally(() => {
          if (statusRequestRef.current?.promise === promise) {
            statusRequestRef.current = undefined;
          }
        });
      statusRequestRef.current = { key, sequence, promise, controller };
      return promise;
    },
    [identityKey, projectId, spaceId],
  );

  const refreshStatus = useCallback(
    () => refreshStatusInternal(true),
    [refreshStatusInternal],
  );

  const refreshLogs = useCallback((): Promise<void> => {
    if (!identity) {
      setLogs([]);
      setLogsLoading(false);
      return Promise.resolve();
    }
    if (
      logsRequestRef.current?.key === identityKey &&
      logsRequestRef.current.generation === latestGenerationRef.current
    ) {
      return logsRequestRef.current.promise;
    }
    invalidateLogs();
    const controller = new AbortController();
    const epoch = identityEpochRef.current;
    const key = identityKey;
    const sequence = ++logSequenceRef.current;
    const generation = latestGenerationRef.current;
    setLogsLoading(true);
    const promise = listAppDevRuntimeLogs({
      ...identity,
      signal: controller.signal,
    })
      .then(result => {
        if (
          currentIdentity(epoch, key) &&
          logSequenceRef.current === sequence &&
          latestGenerationRef.current === generation
        ) {
          setLogs(result.items);
        }
      })
      .catch(requestError => {
        if (
          !controller.signal.aborted &&
          currentIdentity(epoch, key) &&
          logSequenceRef.current === sequence &&
          latestGenerationRef.current === generation
        ) {
          setError(normalizeAppDevError(requestError));
        }
      })
      .finally(() => {
        if (
          currentIdentity(epoch, key) &&
          logSequenceRef.current === sequence
        ) {
          setLogsLoading(false);
        }
        if (logsRequestRef.current?.promise === promise) {
          logsRequestRef.current = undefined;
        }
      });
    logsRequestRef.current = { key, sequence, generation, promise, controller };
    return promise;
  }, [identityKey, projectId, spaceId]);

  const executeMutation = useCallback(
    (intent: RuntimeIntent, requestedOperationId?: string): Promise<void> => {
      if (!identity) {
        setError('项目地址不完整，请返回项目列表重新进入');
        return Promise.resolve();
      }
      const active = mutationRef.current;
      if (active?.intent === intent) {
        return active.promise;
      }
      if (active) {
        active.controller.abort();
        reportStorageWarning(
          clearAppDevPendingOperation(
            identity,
            active.intent,
            active.operationId,
          ).warning,
        );
      }
      for (const otherIntent of runtimeIntents) {
        if (otherIntent === intent) {
          continue;
        }
        const superseded = readPending(otherIntent);
        if (superseded) {
          reportStorageWarning(
            clearAppDevPendingOperation(
              identity,
              otherIntent,
              superseded.operationId,
            ).warning,
          );
        }
      }
      const persisted = readPending(intent);
      const operationId =
        requestedOperationId ||
        persisted?.operationId ||
        createAppDevOperationID();
      reportStorageWarning(
        writeAppDevPendingOperation(identity, intent, {
          operationId,
          generation: persisted?.generation,
          phase: 'requesting',
          createdAt: persisted?.createdAt,
        }).warning,
      );
      const controller = new AbortController();
      const epoch = identityEpochRef.current;
      const key = identityKey;
      const sequence = invalidateObservations();
      invalidateLogs();
      setLoading(true);
      setError('');
      setRuntime(previous => ({
        ...previous,
        status: intent === 'runtime-stop' ? 'stopping' : 'starting',
        canStart: false,
        stopping: intent === 'runtime-stop',
      }));
      const promise = mutationForIntent[intent]({
        ...identity,
        operationId,
        signal: controller.signal,
      })
        .then(async result => {
          if (!commitObservation(result, epoch, key, sequence)) {
            return;
          }
          reportStorageWarning(
            writeAppDevPendingOperation(identity, intent, {
              operationId,
              generation: result.generation,
              phase: 'observed',
              createdAt: persisted?.createdAt,
            }).warning,
          );
          const authoritative = await refreshStatusInternal(true);
          if (!currentIdentity(epoch, key)) {
            return;
          }
          const complete =
            intent === 'runtime-stop'
              ? authoritative?.status === 'stopped'
              : Boolean(
                  authoritative &&
                    ['starting', 'running', 'recovering', 'error'].includes(
                      authoritative.status,
                    ),
                );
          if (complete) {
            reportStorageWarning(
              clearAppDevPendingOperation(identity, intent, operationId)
                .warning,
            );
          }
          void refreshLogs();
        })
        .catch(requestError => {
          if (isAppDevDeterministicError(requestError)) {
            reportStorageWarning(
              clearAppDevPendingOperation(identity, intent, operationId)
                .warning,
            );
          }
          if (!controller.signal.aborted && currentIdentity(epoch, key)) {
            setError(normalizeAppDevError(requestError));
          }
        })
        .finally(() => {
          if (mutationRef.current?.promise === promise) {
            mutationRef.current = undefined;
            if (currentIdentity(epoch, key)) {
              setLoading(false);
            }
          }
        });
      mutationRef.current = { intent, operationId, promise, controller };
      return promise;
    },
    [identityKey, projectId, refreshLogs, refreshStatusInternal, spaceId],
  );

  const start = useCallback(() => {
    if (identity) {
      reportStorageWarning(
        clearAppDevPendingOperation(identity, 'runtime-auto-start').warning,
      );
    }
    suppressAutoStartRef.current = true;
    return executeMutation('runtime-manual-start');
  }, [executeMutation, identityKey]);
  const restart = useCallback(() => {
    suppressAutoStartRef.current = true;
    return executeMutation('runtime-restart');
  }, [executeMutation]);
  const stop = useCallback(() => {
    suppressAutoStartRef.current = true;
    if (identity) {
      reportStorageWarning(
        clearAppDevPendingOperation(identity, 'runtime-auto-start').warning,
      );
    }
    return executeMutation('runtime-stop');
  }, [executeMutation, identityKey]);

  useEffect(() => {
    identityEpochRef.current += 1;
    observationSequenceRef.current += 1;
    latestGenerationRef.current = 0;
    logSequenceRef.current += 1;
    autoDecisionRef.current = false;
    suppressAutoStartRef.current = false;
    statusRequestRef.current?.controller.abort();
    logsRequestRef.current?.controller.abort();
    mutationRef.current?.controller.abort();
    keepAliveRef.current?.controller.abort();
    statusRequestRef.current = undefined;
    logsRequestRef.current = undefined;
    mutationRef.current = undefined;
    keepAliveRef.current = undefined;
    setRuntime(initialRuntime);
    setLogs([]);
    setLoading(false);
    setLogsLoading(false);
    setError('');
    const epoch = identityEpochRef.current;
    const key = identityKey;
    void refreshStatusInternal(true).then(authoritative => {
      if (
        !authoritative ||
        !currentIdentity(epoch, key) ||
        autoDecisionRef.current
      ) {
        return;
      }
      autoDecisionRef.current = true;
      if (!identity) {
        return;
      }
      const stopIntent = readPending('runtime-stop');
      const restartIntent = readPending('runtime-restart');
      const startIntent = readPending('runtime-manual-start');
      if (stopIntent) {
        suppressAutoStartRef.current = true;
        if (authoritative.status === 'stopped') {
          clearAppDevPendingOperation(
            identity,
            'runtime-stop',
            stopIntent.operationId,
          );
        } else {
          void executeMutation('runtime-stop', stopIntent.operationId);
        }
        return;
      }
      if (restartIntent) {
        suppressAutoStartRef.current = true;
        void executeMutation('runtime-restart', restartIntent.operationId);
        return;
      }
      if (startIntent) {
        suppressAutoStartRef.current = true;
        if (
          ['starting', 'running', 'recovering'].includes(authoritative.status)
        ) {
          clearAppDevPendingOperation(
            identity,
            'runtime-manual-start',
            startIntent.operationId,
          );
        } else if (authoritative.canStart) {
          void executeMutation('runtime-manual-start', startIntent.operationId);
        }
        return;
      }
      const autoIntent = readPending('runtime-auto-start');
      if (
        ['starting', 'running', 'recovering'].includes(authoritative.status)
      ) {
        if (autoIntent) {
          clearAppDevPendingOperation(
            identity,
            'runtime-auto-start',
            autoIntent.operationId,
          );
        }
        return;
      }
      if (authoritative.canStart && !suppressAutoStartRef.current) {
        const operationId =
          autoIntent?.operationId || createAppDevOperationID();
        void executeMutation('runtime-auto-start', operationId);
      }
    });
    return () => {
      identityEpochRef.current += 1;
      observationSequenceRef.current += 1;
      logSequenceRef.current += 1;
      statusRequestRef.current?.controller.abort();
      logsRequestRef.current?.controller.abort();
      mutationRef.current?.controller.abort();
      keepAliveRef.current?.controller.abort();
    };
  }, [identityKey]);

  useEffect(() => {
    if (runtime.status === 'stopped') {
      invalidateLogs();
      return;
    }
    void refreshLogs();
  }, [refreshLogs, runtime.generation, runtime.status]);

  useEffect(() => {
    if (
      !identity ||
      ![
        'starting',
        'running',
        'recovering',
        'stopping',
        'cleanup_pending',
      ].includes(runtime.status)
    ) {
      return;
    }
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') {
        void refreshStatusInternal(false);
      }
    }, 5000);
    return () => window.clearInterval(timer);
  }, [identityKey, refreshStatusInternal, runtime.status]);

  useEffect(() => {
    if (!identity || runtime.status !== 'running') {
      keepAliveRef.current?.controller.abort();
      keepAliveRef.current = undefined;
      return;
    }
    const sendKeepAlive = () => {
      if (document.visibilityState !== 'visible' || keepAliveRef.current) {
        return;
      }
      const controller = new AbortController();
      const epoch = identityEpochRef.current;
      const key = identityKey;
      const sequence = ++observationSequenceRef.current;
      const promise = keepAliveAppDevRuntime({
        ...identity,
        signal: controller.signal,
      })
        .then(result => {
          if (commitObservation(result, epoch, key, sequence)) {
            setRuntime(previous => ({
              ...previous,
              lastKeepAliveAt: new Date().toISOString(),
            }));
          }
        })
        .catch(requestError => {
          if (!controller.signal.aborted && currentIdentity(epoch, key)) {
            setError(normalizeAppDevError(requestError));
          }
        })
        .finally(() => {
          if (keepAliveRef.current?.promise === promise) {
            keepAliveRef.current = undefined;
          }
        });
      keepAliveRef.current = { promise, controller };
    };
    const timer = window.setInterval(sendKeepAlive, 30000);
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        sendKeepAlive();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisibility);
      keepAliveRef.current?.controller.abort();
      keepAliveRef.current = undefined;
    };
  }, [identityKey, projectId, runtime.generation, runtime.status, spaceId]);

  return {
    runtime,
    logs,
    loading,
    logsLoading,
    error,
    refreshStatus,
    refreshLogs,
    start,
    restart,
    stop,
  };
};
