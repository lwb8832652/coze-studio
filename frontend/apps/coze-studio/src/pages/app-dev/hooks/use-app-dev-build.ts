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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, max-params -- Async build fencing is intentionally colocated. */

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  APP_DEV_OPERATION_STORAGE_WARNING,
  clearAppDevPendingOperation,
  clearAppDevTerminalBuild,
  createAppDevOperationID,
  readAppDevPendingOperationResult,
  readAppDevTerminalBuildResult,
  writeAppDevPendingOperation,
  writeAppDevTerminalBuild,
} from '../utils/provider-operation';
import type { AppDevBuildInfo } from '../types';
import {
  AppDevSafeError,
  buildAppDevProject,
  getAppDevBuildStatus,
  isAppDevDeterministicError,
  normalizeAppDevError,
} from '../service';

const idleBuild: AppDevBuildInfo = {
  generation: 0,
  state: 'idle',
  releaseAvailable: false,
  size: 0,
  stale: false,
};

const pollDelays = [500, 1000, 2000, 4000, 8000, 8000, 8000, 8000];
const BUILD_REQUEST_TIMEOUT_MS = 15_000;
const BUILD_RECONCILE_TIMEOUT_MS = 60_000;

const requestWithDeadline = <T>(
  request: (signal: AbortSignal) => Promise<T>,
  parentSignal: AbortSignal,
  deadlineAt: number,
): Promise<T> => {
  const remaining = deadlineAt - Date.now();
  if (remaining <= 0 || parentSignal.aborted) {
    return Promise.reject(new AppDevSafeError('unavailable'));
  }
  const controller = new AbortController();
  const timeout = Math.min(BUILD_REQUEST_TIMEOUT_MS, remaining);

  return new Promise<T>((resolve, reject) => {
    let settled = false;
    const finish = (callback: () => void) => {
      if (settled) {
        return;
      }
      settled = true;
      window.clearTimeout(timer);
      parentSignal.removeEventListener('abort', onParentAbort);
      callback();
    };
    const onParentAbort = () => {
      controller.abort();
      finish(() => reject(new AppDevSafeError('unavailable')));
    };
    const timer = window.setTimeout(() => {
      controller.abort();
      finish(() => reject(new AppDevSafeError('unavailable')));
    }, timeout);
    parentSignal.addEventListener('abort', onParentAbort, { once: true });
    if (parentSignal.aborted) {
      controller.abort();
      finish(() => reject(new AppDevSafeError('unavailable')));
      return;
    }
    request(controller.signal).then(
      value => finish(() => resolve(value)),
      error =>
        finish(() =>
          reject(
            error instanceof AppDevSafeError
              ? error
              : new AppDevSafeError('unavailable'),
          ),
        ),
    );
  });
};

export const useAppDevBuild = (
  spaceId?: string,
  projectId?: string,
  principalId?: string,
) => {
  const [projection, setProjection] = useState<AppDevBuildInfo>(idleBuild);
  const [active, setActive] = useState(false);
  const [error, setError] = useState('');
  const identityEpochRef = useRef(0);
  const requestSequenceRef = useRef(0);
  const inFlightRef = useRef<Promise<AppDevBuildInfo | undefined>>();
  const controllerRef = useRef<AbortController>();
  const timerRef = useRef<number>();
  const identity =
    spaceId && projectId ? { spaceId, projectId, principalId } : undefined;
  const identityKey = identity
    ? `${identity.spaceId}\u0000${identity.projectId}\u0000${principalId || ''}`
    : '';

  const clearTimer = () => {
    if (timerRef.current !== undefined) {
      window.clearTimeout(timerRef.current);
      timerRef.current = undefined;
    }
  };

  const reportStorageWarning = (warning?: string) => {
    if (warning) {
      setError(
        warning === APP_DEV_OPERATION_STORAGE_WARNING
          ? warning
          : '构建恢复状态已失效，请重新构建',
      );
    }
  };

  const beginRequest = () => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    return {
      controller,
      epoch: identityEpochRef.current,
      sequence: ++requestSequenceRef.current,
    };
  };

  const currentRequest = (epoch: number, sequence: number) =>
    identityEpochRef.current === epoch &&
    requestSequenceRef.current === sequence;

  const readPending = () => {
    if (!identity) {
      return undefined;
    }
    const result = readAppDevPendingOperationResult(identity, 'build');
    reportStorageWarning(result.warning);
    return result.value;
  };

  const readTerminal = () => {
    if (!identity) {
      return undefined;
    }
    const result = readAppDevTerminalBuildResult(identity);
    reportStorageWarning(result.warning);
    return result.value;
  };

  const applyProjection = useCallback(
    (
      next: AppDevBuildInfo,
      operationId: string,
      epoch: number,
      sequence: number,
    ) => {
      if (!currentRequest(epoch, sequence) || !identity) {
        return false;
      }
      setProjection(next);
      setError('');
      if (next.state === 'building') {
        const pending = readPending();
        reportStorageWarning(
          writeAppDevPendingOperation(identity, 'build', {
            operationId,
            generation: next.generation,
            phase: 'observed',
            createdAt:
              pending?.operationId === operationId
                ? pending.createdAt
                : undefined,
          }).warning,
        );
        reportStorageWarning(clearAppDevTerminalBuild(identity).warning);
        setActive(true);
      } else if (next.state === 'ready' || next.state === 'failed') {
        reportStorageWarning(
          clearAppDevPendingOperation(identity, 'build', operationId).warning,
        );
        reportStorageWarning(
          writeAppDevTerminalBuild(identity, {
            operationId,
            generation: next.generation,
            projection: next,
          }).warning,
        );
        setActive(false);
      }
      return true;
    },
    [identityKey],
  );

  const handleRequestError = useCallback(
    (requestError: unknown, operationId: string, epoch: number) => {
      if (!identity || identityEpochRef.current !== epoch) {
        return;
      }
      setActive(false);
      if (isAppDevDeterministicError(requestError)) {
        reportStorageWarning(
          clearAppDevPendingOperation(identity, 'build', operationId).warning,
        );
        reportStorageWarning(
          clearAppDevTerminalBuild(identity, operationId).warning,
        );
        setProjection({
          ...idleBuild,
          stale: true,
          safeMessage: '构建状态已失效，请重新构建',
        });
      }
      setError(normalizeAppDevError(requestError));
    },
    [identityKey],
  );

  const poll = useCallback(
    (
      operationId: string,
      generation: number,
      attempt = 0,
      deadlineAt = Date.now() + BUILD_RECONCILE_TIMEOUT_MS,
    ): Promise<AppDevBuildInfo | undefined> => {
      if (!identity) {
        return Promise.resolve(undefined);
      }
      if (attempt >= pollDelays.length || Date.now() >= deadlineAt) {
        setActive(false);
        setError('构建状态确认超时，请点击刷新继续确认');
        return Promise.resolve(undefined);
      }
      const { controller, epoch, sequence } = beginRequest();
      setActive(true);
      return requestWithDeadline(
        signal =>
          getAppDevBuildStatus({
            ...identity,
            operationId,
            expectedGeneration: generation,
            signal,
          }),
        controller.signal,
        deadlineAt,
      )
        .then(result => {
          if (!applyProjection(result, operationId, epoch, sequence)) {
            return undefined;
          }
          if (result.state === 'building') {
            clearTimer();
            const delay = Math.min(
              pollDelays[attempt],
              Math.max(0, deadlineAt - Date.now()),
            );
            timerRef.current = window.setTimeout(() => {
              void poll(operationId, generation, attempt + 1, deadlineAt);
            }, delay);
          }
          return result;
        })
        .catch(requestError => {
          if (!controller.signal.aborted) {
            handleRequestError(requestError, operationId, epoch);
          }
          return undefined;
        });
    },
    [applyProjection, handleRequestError, identityKey],
  );

  const runBegin = useCallback(
    (
      operationId: string,
      recovering = false,
      deadlineAt = Date.now() + BUILD_RECONCILE_TIMEOUT_MS,
    ): Promise<AppDevBuildInfo | undefined> => {
      if (!identity) {
        setError('项目地址不完整，请返回项目列表重新进入');
        return Promise.resolve(undefined);
      }
      if (inFlightRef.current) {
        return inFlightRef.current;
      }
      const { controller, epoch, sequence } = beginRequest();
      const persisted = readPending();
      setActive(true);
      setError('');
      if (!recovering) {
        reportStorageWarning(
          clearAppDevPendingOperation(identity, 'build').warning,
        );
        reportStorageWarning(clearAppDevTerminalBuild(identity).warning);
      }
      reportStorageWarning(
        writeAppDevPendingOperation(identity, 'build', {
          operationId,
          phase: 'requesting',
          createdAt:
            persisted?.operationId === operationId
              ? persisted.createdAt
              : undefined,
        }).warning,
      );
      const promise = requestWithDeadline(
        signal =>
          buildAppDevProject({
            ...identity,
            operationId,
            signal,
          }),
        controller.signal,
        deadlineAt,
      )
        .then(result => {
          if (!applyProjection(result, operationId, epoch, sequence)) {
            return undefined;
          }
          if (result.state === 'building') {
            clearTimer();
            timerRef.current = window.setTimeout(
              () => {
                void poll(operationId, result.generation, 0, deadlineAt);
              },
              Math.min(pollDelays[0], deadlineAt - Date.now()),
            );
          }
          return result;
        })
        .catch(requestError => {
          if (!controller.signal.aborted) {
            handleRequestError(requestError, operationId, epoch);
          }
          return undefined;
        })
        .finally(() => {
          if (inFlightRef.current === promise) {
            inFlightRef.current = undefined;
          }
        });
      inFlightRef.current = promise;
      return promise;
    },
    [applyProjection, handleRequestError, identityKey, poll],
  );

  const reconcile = useCallback(() => {
    if (!identity) {
      return Promise.resolve(undefined);
    }
    clearTimer();
    const deadlineAt = Date.now() + BUILD_RECONCILE_TIMEOUT_MS;
    const pending = readPending();
    if (pending) {
      return pending.generation === undefined
        ? runBegin(pending.operationId, true, deadlineAt)
        : poll(pending.operationId, pending.generation, 0, deadlineAt);
    }
    const terminal = readTerminal();
    if (terminal) {
      setProjection(terminal.projection);
      return poll(terminal.operationId, terminal.generation, 0, deadlineAt);
    }
    return Promise.resolve(undefined);
  }, [identityKey, poll, runBegin]);

  const beginBuild = useCallback(() => {
    if (!identity) {
      return Promise.resolve(undefined);
    }
    const pending = readPending();
    if (pending) {
      return pending.generation === undefined
        ? runBegin(pending.operationId, true)
        : poll(pending.operationId, pending.generation);
    }
    return runBegin(createAppDevOperationID());
  }, [identityKey, poll, runBegin]);

  const markStale = useCallback(() => {
    if (!identity) {
      return;
    }
    setProjection(previous => {
      const next = {
        ...previous,
        stale: true,
        releaseAvailable: false,
        safeMessage: '发布产物已过期，请重新构建',
      };
      const terminal = readTerminal();
      if (terminal && ['ready', 'failed'].includes(next.state)) {
        reportStorageWarning(
          writeAppDevTerminalBuild(identity, {
            operationId: terminal.operationId,
            generation: next.generation,
            projection: next,
          }).warning,
        );
      }
      return next;
    });
  }, [identityKey]);

  useEffect(() => {
    identityEpochRef.current += 1;
    requestSequenceRef.current += 1;
    controllerRef.current?.abort();
    controllerRef.current = undefined;
    inFlightRef.current = undefined;
    clearTimer();
    setProjection(idleBuild);
    setActive(false);
    setError('');
    if (identity) {
      const pending = readPending();
      const terminal = readTerminal();
      if (terminal) {
        setProjection(terminal.projection);
      }
      if (pending?.generation !== undefined) {
        void poll(pending.operationId, pending.generation);
      } else if (pending) {
        void runBegin(pending.operationId, true);
      } else if (terminal) {
        void poll(terminal.operationId, terminal.generation);
      }
    }
    return () => {
      identityEpochRef.current += 1;
      requestSequenceRef.current += 1;
      controllerRef.current?.abort();
      controllerRef.current = undefined;
      inFlightRef.current = undefined;
      clearTimer();
    };
  }, [identityKey]);

  return {
    projection,
    building: active,
    error,
    beginBuild,
    reconcile,
    markStale,
  };
};
