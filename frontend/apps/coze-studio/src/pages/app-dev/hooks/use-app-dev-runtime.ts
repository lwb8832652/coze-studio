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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function -- Cohesive orchestrator. */

import { useCallback, useEffect, useRef, useState } from 'react';

import type { AppDevRuntimeInfo, AppDevRuntimeLog } from '../types';
import {
  getAppDevRuntimeStatus,
  keepAliveAppDevRuntime,
  listAppDevRuntimeLogs,
  normalizeAppDevError,
  restartAppDevRuntime,
  startAppDevRuntime,
  stopAppDevRuntime,
} from '../service';
import { useLatestRequest } from './use-latest-request';

const stoppedRuntime: AppDevRuntimeInfo = {
  status: 'stopped',
  message: '开发环境未启动',
};

export const useAppDevRuntime = (spaceId?: string, projectId?: string) => {
  const [runtime, setRuntime] = useState<AppDevRuntimeInfo>(stoppedRuntime);
  const [logs, setLogs] = useState<AppDevRuntimeLog[]>([]);
  const [loading, setLoading] = useState(false);
  const [logsLoading, setLogsLoading] = useState(false);
  const [error, setError] = useState('');
  const [statusLoaded, setStatusLoaded] = useState(false);
  const autoStartKeyRef = useRef('');
  const {
    beginRequest: beginStatusRequest,
    invalidateRequests: invalidateStatusRequests,
  } = useLatestRequest();
  const {
    beginRequest: beginLogsRequest,
    invalidateRequests: invalidateLogsRequests,
  } = useLatestRequest();
  const {
    beginRequest: beginMutationRequest,
    invalidateRequests: invalidateMutationRequests,
  } = useLatestRequest();
  const {
    beginRequest: beginKeepAliveRequest,
    invalidateRequests: invalidateKeepAliveRequests,
  } = useLatestRequest();

  const canRequest = Boolean(spaceId && projectId);

  const refreshStatus = useCallback(async () => {
    const isLatest = beginStatusRequest();
    if (!spaceId || !projectId) {
      if (isLatest()) {
        setRuntime(stoppedRuntime);
        setStatusLoaded(false);
      }
      return;
    }

    try {
      const result = await getAppDevRuntimeStatus({ spaceId, projectId });
      if (isLatest()) {
        setRuntime(result);
        setError('');
      }
    } catch (requestError) {
      if (isLatest()) {
        setError(normalizeAppDevError(requestError));
      }
    } finally {
      if (isLatest()) {
        setStatusLoaded(true);
      }
    }
  }, [beginStatusRequest, projectId, spaceId]);

  const refreshLogs = useCallback(async () => {
    const isLatest = beginLogsRequest();
    if (!spaceId || !projectId) {
      if (isLatest()) {
        setLogs([]);
        setLogsLoading(false);
      }
      return;
    }

    setLogsLoading(true);
    try {
      const result = await listAppDevRuntimeLogs({ spaceId, projectId });
      if (isLatest()) {
        setLogs(result.items);
      }
    } catch (requestError) {
      if (isLatest()) {
        setError(normalizeAppDevError(requestError));
      }
    } finally {
      if (isLatest()) {
        setLogsLoading(false);
      }
    }
  }, [beginLogsRequest, projectId, spaceId]);

  const mutateRuntime = useCallback(
    async (
      mutation: (identity: {
        spaceId: string;
        projectId: string;
      }) => Promise<AppDevRuntimeInfo>,
    ) => {
      if (!spaceId || !projectId) {
        setError('项目地址不完整，请返回项目列表重新进入');
        return;
      }

      const isLatest = beginMutationRequest();

      setLoading(true);
      setError('');

      try {
        const result = await mutation({ spaceId, projectId });
        if (!isLatest()) {
          return;
        }
        setRuntime(result);
        await refreshLogs();
      } catch (requestError) {
        if (isLatest()) {
          setError(normalizeAppDevError(requestError));
        }
      } finally {
        if (isLatest()) {
          setLoading(false);
        }
      }
    },
    [beginMutationRequest, projectId, refreshLogs, spaceId],
  );

  const start = useCallback(
    () => mutateRuntime(startAppDevRuntime),
    [mutateRuntime],
  );
  const restart = useCallback(
    () => mutateRuntime(restartAppDevRuntime),
    [mutateRuntime],
  );
  const stop = useCallback(
    () => mutateRuntime(stopAppDevRuntime),
    [mutateRuntime],
  );

  useEffect(() => {
    invalidateStatusRequests();
    invalidateLogsRequests();
    invalidateMutationRequests();
    invalidateKeepAliveRequests();
    autoStartKeyRef.current = '';
    setRuntime(stoppedRuntime);
    setLogs([]);
    setStatusLoaded(false);
    setLoading(false);
    setLogsLoading(false);
    setError('');
  }, [
    invalidateKeepAliveRequests,
    invalidateLogsRequests,
    invalidateMutationRequests,
    invalidateStatusRequests,
    projectId,
    spaceId,
  ]);

  useEffect(() => {
    void refreshStatus();
    void refreshLogs();
  }, [refreshLogs, refreshStatus]);

  useEffect(() => {
    if (!spaceId || !projectId || !statusLoaded || loading) {
      return;
    }
    if (runtime.status !== 'stopped' && runtime.status !== 'error') {
      return;
    }

    const autoStartKey = `${spaceId}:${projectId}`;
    if (autoStartKeyRef.current === autoStartKey) {
      return;
    }
    autoStartKeyRef.current = autoStartKey;
    void mutateRuntime(startAppDevRuntime);
  }, [
    loading,
    mutateRuntime,
    projectId,
    runtime.status,
    spaceId,
    statusLoaded,
  ]);

  useEffect(() => {
    if (!canRequest) {
      return;
    }

    const timer = window.setInterval(() => {
      void refreshStatus();
    }, 5000);

    return () => window.clearInterval(timer);
  }, [canRequest, refreshStatus]);

  useEffect(() => {
    if (!spaceId || !projectId || runtime.status !== 'running') {
      return;
    }

    const timer = window.setInterval(() => {
      const isLatest = beginKeepAliveRequest();
      void keepAliveAppDevRuntime({ spaceId, projectId })
        .then(result => {
          if (isLatest()) {
            setRuntime(result);
          }
        })
        .catch(requestError => {
          if (isLatest()) {
            setError(normalizeAppDevError(requestError));
          }
        });
    }, 30000);

    return () => window.clearInterval(timer);
  }, [beginKeepAliveRequest, projectId, runtime.status, spaceId]);

  useEffect(() => {
    if (
      !canRequest ||
      !['starting', 'running', 'restarting'].includes(runtime.status)
    ) {
      return;
    }

    const timer = window.setInterval(() => {
      void refreshLogs();
    }, 5000);

    return () => window.clearInterval(timer);
  }, [canRequest, refreshLogs, runtime.status]);

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
