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

/* eslint-disable @coze-arch/max-line-per-function, max-lines -- Usage loading keeps polling, retry and bounded merge semantics together. */

import { useCallback, useEffect, useRef, useState } from 'react';

import type {
  WorkbenchTokenUsage,
  WorkbenchTokenUsageAggregate,
} from '../workbench/thread-client';

import {
  mapTaskThreadTokenUsageAggregate,
  mapTaskThreadTokenUsageRowToSnapshot,
  mapTaskTokenUsageSnapshotsByRunID,
  type TaskDetailTokenUsage,
  type TaskTokenUsageSnapshot,
} from './task-detail-token-usage';
import { getTaskThreadTokenUsage } from './service';

type TaskThreadTokenUsage = WorkbenchTokenUsage;
type TaskThreadTokenUsageAggregate = WorkbenchTokenUsageAggregate;

const TASK_USAGE_PAGE_SIZE = 50;
const TASK_USAGE_MAX_PAGES = 20;
const TASK_USAGE_MAX_ROWS = TASK_USAGE_PAGE_SIZE * TASK_USAGE_MAX_PAGES;
const TASK_USAGE_ERROR = 'Token 用量明细加载失败，请重试';
const scheduleTaskUsageMicrotask = (callback: () => void) => {
  if (typeof queueMicrotask === 'function') {
    queueMicrotask(callback);
    return;
  }
  void Promise.resolve().then(callback);
};

export interface TaskUsageLoadResult {
  error: string;
  isPartial: boolean;
  loadedCount: number;
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageByRunID: Record<string, TaskDetailTokenUsage>;
  totalCount: number;
}

interface InternalTaskUsageLoadResult extends TaskUsageLoadResult {
  summaryLoaded: boolean;
  usageSnapshotsByID: Map<string, TaskTokenUsageSnapshot>;
}

interface TaskUsageState extends TaskUsageLoadResult {
  loading: boolean;
  scopeKey: string;
}

interface TaskUsageLoadOptions {
  isCurrentScope?: () => boolean;
  signal?: AbortSignal;
  spaceID: string;
}

const getTrustedPositiveCount = (value: unknown, loadedCount: number) =>
  typeof value === 'number' &&
  Number.isFinite(value) &&
  Number.isInteger(value) &&
  value > 0 &&
  value >= loadedCount
    ? value
    : undefined;

const normalizeUsageID = (value: unknown) => String(value ?? '').trim();

const createTaskUsageAbortError = () => {
  if (typeof DOMException !== 'undefined') {
    return new DOMException('Task usage request aborted', 'AbortError');
  }

  const error = new Error('Task usage request aborted');
  error.name = 'AbortError';
  return error;
};

export const isTaskUsageAbortError = (error: unknown) =>
  Boolean(
    error &&
      typeof error === 'object' &&
      'name' in error &&
      error.name === 'AbortError',
  );

const assertTaskUsageRequestCurrent = ({
  isCurrentScope,
  signal,
}: TaskUsageLoadOptions) => {
  if (signal?.aborted || isCurrentScope?.() === false) {
    throw createTaskUsageAbortError();
  }
};

const createUsageSnapshotsByID = (rows: TaskThreadTokenUsage[]) => {
  const snapshots = new Map<string, TaskTokenUsageSnapshot>();

  rows.forEach((row, index) => {
    const snapshot = mapTaskThreadTokenUsageRowToSnapshot(row);
    if (!snapshot) {
      return;
    }
    const snapshotID =
      normalizeUsageID(snapshot.usageID) ||
      `rest:${index}:${snapshot.runID}:${snapshot.createdAt}`;
    snapshots.set(snapshotID, {
      ...snapshot,
      usageID: snapshotID,
    });
  });

  return snapshots;
};

const createUsageLoadResult = ({
  aggregate,
  error = '',
  forcePartial = false,
  reportedTotal,
  rows,
  summaryLoaded,
}: {
  aggregate?: TaskThreadTokenUsageAggregate;
  error?: string;
  forcePartial?: boolean;
  reportedTotal?: number;
  rows: TaskThreadTokenUsage[];
  summaryLoaded: boolean;
}): InternalTaskUsageLoadResult => {
  const totalCount = Math.max(reportedTotal ?? 0, rows.length);
  const usageSnapshotsByID = createUsageSnapshotsByID(rows);

  return {
    error,
    isPartial:
      forcePartial ||
      (reportedTotal !== undefined && rows.length < reportedTotal) ||
      (reportedTotal === undefined && rows.length > 0),
    loadedCount: rows.length,
    summaryLoaded,
    tokenUsage: mapTaskThreadTokenUsageAggregate(aggregate, rows, totalCount),
    tokenUsageByRunID: mapTaskTokenUsageSnapshotsByRunID(
      usageSnapshotsByID.values(),
    ),
    totalCount,
    usageSnapshotsByID,
  };
};

export const loadTaskThreadUsage = async (
  threadID: string,
  options: TaskUsageLoadOptions,
): Promise<InternalTaskUsageLoadResult> => {
  const rowsByUsageID = new Map<string, TaskThreadTokenUsage>();
  const anonymousRows = new Map<string, TaskThreadTokenUsage>();
  let aggregate: TaskThreadTokenUsageAggregate | undefined;
  let error = '';
  let forcePartial = false;
  let reportedTotal: number | undefined;
  let summaryLoaded = false;

  const getRows = () => [...rowsByUsageID.values(), ...anonymousRows.values()];
  const upsertRows = (pageRows: TaskThreadTokenUsage[], page: number) => {
    let newRows = 0;

    pageRows.forEach((row, index) => {
      const usageID = normalizeUsageID(row.usage_id);
      const key = usageID || `anonymous:${page}:${index}`;
      const target = usageID ? rowsByUsageID : anonymousRows;
      if (!target.has(key)) {
        if (getRows().length >= TASK_USAGE_MAX_ROWS) {
          return;
        }
        newRows += 1;
      }
      target.set(key, row);
    });

    return newRows;
  };
  const requestPage = async (page: number) => {
    assertTaskUsageRequestCurrent(options);
    const response = await getTaskThreadTokenUsage(
      {
        thread_id: threadID,
        space_id: options.spaceID,
        page,
        page_size: TASK_USAGE_PAGE_SIZE,
      },
      { signal: options.signal },
    );
    assertTaskUsageRequestCurrent(options);
    return response;
  };

  for (let page = 1; page <= TASK_USAGE_MAX_PAGES; page += 1) {
    let response: Awaited<ReturnType<typeof getTaskThreadTokenUsage>>;
    try {
      response = await requestPage(page);
    } catch (requestError) {
      if (isTaskUsageAbortError(requestError)) {
        throw requestError;
      }
      error = TASK_USAGE_ERROR;
      forcePartial = true;
      break;
    }

    if (response.code !== 0 || !response.data) {
      error = TASK_USAGE_ERROR;
      forcePartial = true;
      break;
    }

    const pageRows = response.data.usage ?? [];
    if (page === 1) {
      aggregate = response.data.aggregate;
      summaryLoaded = true;
    }
    const newRows = upsertRows(pageRows, page);
    const loadedCount = getRows().length;
    const trustedTotal = getTrustedPositiveCount(
      response.data.total,
      loadedCount,
    );
    if (trustedTotal !== undefined) {
      reportedTotal = Math.max(reportedTotal ?? 0, trustedTotal);
    } else if (response.data.total !== undefined) {
      reportedTotal = undefined;
      if (loadedCount > 0 || response.data.total !== 0) {
        forcePartial = true;
      }
    }
    if (reportedTotal !== undefined && loadedCount >= reportedTotal) {
      break;
    }
    if (pageRows.length < TASK_USAGE_PAGE_SIZE) {
      forcePartial =
        forcePartial ||
        (reportedTotal !== undefined && loadedCount < reportedTotal);
      break;
    }
    if (newRows === 0) {
      forcePartial = true;
      break;
    }
    if (loadedCount >= TASK_USAGE_MAX_ROWS || page === TASK_USAGE_MAX_PAGES) {
      forcePartial = true;
      break;
    }
  }

  if (summaryLoaded) {
    try {
      const confirmation = await requestPage(1);
      if (confirmation.code !== 0 || !confirmation.data) {
        error = TASK_USAGE_ERROR;
        forcePartial = true;
      } else {
        aggregate = confirmation.data.aggregate;
        upsertRows(confirmation.data.usage ?? [], 1);
        const trustedTotal = getTrustedPositiveCount(
          confirmation.data.total,
          getRows().length,
        );
        if (trustedTotal !== undefined) {
          reportedTotal = Math.max(reportedTotal ?? 0, trustedTotal);
        } else if (confirmation.data.total !== undefined) {
          reportedTotal = undefined;
          if (getRows().length > 0 || confirmation.data.total !== 0) {
            forcePartial = true;
          }
        }
      }
    } catch (requestError) {
      if (isTaskUsageAbortError(requestError)) {
        throw requestError;
      }
      error = TASK_USAGE_ERROR;
      forcePartial = true;
    }
  }

  return {
    ...createUsageLoadResult({
      aggregate,
      error,
      forcePartial,
      reportedTotal,
      rows: getRows(),
      summaryLoaded,
    }),
  };
};

const createUsageState = (
  scopeKey: string,
  loading = false,
): TaskUsageState => ({
  error: '',
  isPartial: false,
  loadedCount: 0,
  loading,
  scopeKey,
  tokenUsage: undefined,
  tokenUsageByRunID: {},
  totalCount: 0,
});

const mergeUsageSnapshotMonotonic = (
  current: TaskTokenUsageSnapshot | undefined,
  candidate: TaskTokenUsageSnapshot,
): TaskTokenUsageSnapshot => {
  if (!current) {
    return candidate;
  }

  return {
    ...current,
    ...candidate,
    source: candidate.source || current.source,
    stepID: candidate.stepID || current.stepID,
    stepName: candidate.stepName || current.stepName,
    modelName: candidate.modelName || current.modelName,
    provider: candidate.provider || current.provider,
    currency: candidate.currency || current.currency,
    createdAt: candidate.createdAt || current.createdAt,
    inputTokens: Math.max(current.inputTokens, candidate.inputTokens),
    outputTokens: Math.max(current.outputTokens, candidate.outputTokens),
    totalTokens: Math.max(current.totalTokens, candidate.totalTokens),
    costMicros: Math.max(current.costMicros, candidate.costMicros),
  };
};

const mergeUsageSnapshotMaps = (
  target: Map<string, TaskTokenUsageSnapshot>,
  incoming: Map<string, TaskTokenUsageSnapshot>,
) => {
  for (const [snapshotID, snapshot] of incoming) {
    target.set(
      snapshotID,
      mergeUsageSnapshotMonotonic(target.get(snapshotID), snapshot),
    );
  }
};

const mergeTaskUsageSummaryMonotonic = (
  current: TaskDetailTokenUsage | undefined,
  candidate: TaskDetailTokenUsage | undefined,
) => {
  if (!candidate) {
    return current;
  }
  if (!current) {
    return candidate;
  }

  const candidateIsMonotonic =
    candidate.inputTokens >= current.inputTokens &&
    candidate.outputTokens >= current.outputTokens &&
    candidate.totalTokens >= current.totalTokens &&
    candidate.costMicros >= current.costMicros &&
    candidate.callCount >= current.callCount &&
    candidate.leadAgentTokens >= current.leadAgentTokens &&
    candidate.subagentTokens >= current.subagentTokens &&
    candidate.middlewareTokens >= current.middlewareTokens &&
    candidate.toolTokens >= current.toolTokens;
  if (!candidateIsMonotonic) {
    return current;
  }

  return {
    ...candidate,
    currency: candidate.currency || current.currency,
    modelAttributions: Array.from(
      new Set([...current.modelAttributions, ...candidate.modelAttributions]),
    ),
  };
};

export const useTaskUsageData = ({
  enabled,
  refreshKey = 0,
  spaceID,
  threadID,
}: {
  enabled: boolean;
  refreshKey?: string | number;
  spaceID?: string;
  threadID: string;
}) => {
  const scopeKey =
    enabled && spaceID && threadID ? JSON.stringify([spaceID, threadID]) : '';
  const abortControllerRef = useRef<AbortController>();
  const requestInFlightRef = useRef(false);
  const pendingRefreshRef = useRef(false);
  const refreshScheduledRef = useRef(false);
  const invalidationGenerationRef = useRef(0);
  const loadUsageRef =
    useRef<(mode?: 'initial' | 'background' | 'retry') => Promise<void>>();
  const mountedRef = useRef(true);
  const scopeKeyRef = useRef(scopeKey);
  const scopeGenerationRef = useRef(0);
  const requestGenerationRef = useRef(0);
  const runServerSnapshotsByIDRef = useRef<Map<string, TaskTokenUsageSnapshot>>(
    new Map(),
  );
  const summaryVisibleRef = useRef<TaskDetailTokenUsage>();

  if (scopeKeyRef.current !== scopeKey) {
    scopeKeyRef.current = scopeKey;
    scopeGenerationRef.current += 1;
    requestGenerationRef.current += 1;
  }
  const scopeGeneration = scopeGenerationRef.current;
  const [state, setState] = useState<TaskUsageState>(() =>
    createUsageState(scopeKey, Boolean(scopeKey)),
  );

  const isCurrentScope = useCallback(
    (submittedScopeKey: string, generation: number, request?: number) =>
      mountedRef.current &&
      scopeKeyRef.current === submittedScopeKey &&
      scopeGenerationRef.current === generation &&
      (request === undefined || requestGenerationRef.current === request),
    [],
  );

  const scheduleUsageRefresh = useCallback(() => {
    const submittedScopeKey = scopeKey;
    const submittedGeneration = scopeGeneration;
    if (
      !submittedScopeKey ||
      !isCurrentScope(submittedScopeKey, submittedGeneration)
    ) {
      return;
    }
    if (requestInFlightRef.current) {
      pendingRefreshRef.current = true;
      return;
    }
    if (refreshScheduledRef.current) {
      return;
    }

    refreshScheduledRef.current = true;
    scheduleTaskUsageMicrotask(() => {
      refreshScheduledRef.current = false;
      if (!isCurrentScope(submittedScopeKey, submittedGeneration)) {
        return;
      }
      if (requestInFlightRef.current) {
        pendingRefreshRef.current = true;
        return;
      }
      void loadUsageRef.current?.('background');
    });
  }, [isCurrentScope, scopeGeneration, scopeKey]);

  const loadUsage = useCallback(
    async (_mode: 'initial' | 'background' | 'retry' = 'retry') => {
      if (!scopeKey || !spaceID) {
        return;
      }
      if (requestInFlightRef.current) {
        pendingRefreshRef.current = true;
        return;
      }

      const controller = new AbortController();
      abortControllerRef.current = controller;
      requestInFlightRef.current = true;
      const submittedScopeKey = scopeKey;
      const submittedGeneration = scopeGeneration;
      const requestGeneration = requestGenerationRef.current + 1;
      const invalidationAtStart = invalidationGenerationRef.current;
      requestGenerationRef.current = requestGeneration;
      setState(current =>
        current.scopeKey === submittedScopeKey
          ? { ...current, error: '', loading: true }
          : createUsageState(submittedScopeKey, true),
      );

      try {
        const result = await loadTaskThreadUsage(threadID, {
          isCurrentScope: () =>
            isCurrentScope(
              submittedScopeKey,
              submittedGeneration,
              requestGeneration,
            ),
          signal: controller.signal,
          spaceID,
        });
        if (
          !isCurrentScope(
            submittedScopeKey,
            submittedGeneration,
            requestGeneration,
          )
        ) {
          return;
        }

        mergeUsageSnapshotMaps(
          runServerSnapshotsByIDRef.current,
          result.usageSnapshotsByID,
        );
        const serverSnapshots = runServerSnapshotsByIDRef.current;
        const tokenUsage = result.summaryLoaded
          ? mergeTaskUsageSummaryMonotonic(
              summaryVisibleRef.current,
              result.tokenUsage,
            )
          : summaryVisibleRef.current;
        summaryVisibleRef.current = tokenUsage;

        setState(current => {
          if (current.scopeKey !== submittedScopeKey) {
            return current;
          }
          const loadedCount = Math.max(
            current.loadedCount,
            result.loadedCount,
            serverSnapshots.size,
          );
          const completeCoversKnownRows =
            !result.error &&
            !result.isPartial &&
            result.loadedCount >= current.loadedCount;

          return {
            error: result.error,
            isPartial: completeCoversKnownRows
              ? false
              : current.isPartial || result.isPartial,
            loadedCount,
            loading: false,
            scopeKey: submittedScopeKey,
            tokenUsage,
            tokenUsageByRunID: mapTaskTokenUsageSnapshotsByRunID(
              serverSnapshots.values(),
            ),
            totalCount: Math.max(
              current.totalCount,
              result.totalCount,
              loadedCount,
            ),
          };
        });
      } catch (loadError) {
        if (
          isTaskUsageAbortError(loadError) ||
          !isCurrentScope(
            submittedScopeKey,
            submittedGeneration,
            requestGeneration,
          )
        ) {
          return;
        }
        setState(current =>
          current.scopeKey === submittedScopeKey
            ? {
                ...current,
                error: TASK_USAGE_ERROR,
                isPartial: true,
                loading: false,
              }
            : current,
        );
      } finally {
        if (abortControllerRef.current === controller) {
          abortControllerRef.current = undefined;
          requestInFlightRef.current = false;
          const needsFollowUp =
            pendingRefreshRef.current ||
            invalidationGenerationRef.current > invalidationAtStart;
          pendingRefreshRef.current = false;
          if (needsFollowUp) {
            scheduleUsageRefresh();
          }
        }
      }
    },
    [
      isCurrentScope,
      scheduleUsageRefresh,
      scopeGeneration,
      scopeKey,
      spaceID,
      threadID,
    ],
  );
  loadUsageRef.current = loadUsage;

  useEffect(() => {
    mountedRef.current = true;

    return () => {
      mountedRef.current = false;
      abortControllerRef.current?.abort();
      scopeGenerationRef.current += 1;
      requestGenerationRef.current += 1;
    };
  }, []);

  useEffect(() => {
    abortControllerRef.current?.abort();
    requestInFlightRef.current = false;
    pendingRefreshRef.current = false;
    refreshScheduledRef.current = false;
    invalidationGenerationRef.current = 0;
    runServerSnapshotsByIDRef.current = new Map();
    summaryVisibleRef.current = undefined;
    setState(createUsageState(scopeKey, Boolean(scopeKey)));
    if (scopeKey) {
      void loadUsage('initial');
    }

    return () => {
      abortControllerRef.current?.abort();
    };
  }, [loadUsage, scopeKey]);

  const refreshContractRef = useRef({ refreshKey, scopeKey });
  useEffect(() => {
    const previous = refreshContractRef.current;
    refreshContractRef.current = { refreshKey, scopeKey };
    if (
      !scopeKey ||
      previous.scopeKey !== scopeKey ||
      previous.refreshKey === refreshKey
    ) {
      return;
    }
    scheduleUsageRefresh();
  }, [refreshKey, scheduleUsageRefresh, scopeKey]);

  const handleTokenUsageSnapshot = useCallback(
    (_snapshot: TaskTokenUsageSnapshot) => {
      const submittedScopeKey = scopeKey;
      const submittedGeneration = scopeGeneration;
      if (
        !submittedScopeKey ||
        !isCurrentScope(submittedScopeKey, submittedGeneration)
      ) {
        return;
      }
      invalidationGenerationRef.current += 1;
      scheduleUsageRefresh();
    },
    [isCurrentScope, scheduleUsageRefresh, scopeGeneration, scopeKey],
  );

  const scopedState =
    state.scopeKey === scopeKey
      ? state
      : createUsageState(scopeKey, Boolean(scopeKey));

  return {
    error: scopedState.error,
    handleTokenUsageSnapshot,
    isPartial: scopedState.isPartial,
    loadedCount: scopedState.loadedCount,
    loading: scopedState.loading,
    retry: () => loadUsage('retry'),
    tokenUsage: scopedState.tokenUsage,
    tokenUsageByRunID: scopedState.tokenUsageByRunID,
    totalCount: scopedState.totalCount,
  };
};
