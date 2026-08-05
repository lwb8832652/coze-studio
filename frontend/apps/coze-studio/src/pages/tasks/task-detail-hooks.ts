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
/* eslint-disable max-params, @coze-arch/max-line-per-function -- Task detail hooks coordinate one bounded task identity and its async projections. */

/* eslint-disable max-lines -- Task detail orchestration remains grouped during DeerFlow parity stabilization. */

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type SetStateAction,
} from 'react';

import type {
  HumanInteractionResponse,
  WorkbenchArtifact,
  WorkbenchMessage,
  WorkbenchRun,
  WorkbenchRunEvent,
  WorkbenchTodo,
} from '../workbench/thread-client';
import type { WorkbenchComposerSubmitPayload } from '../workbench/components/types';
import { useTaskUsageData } from './task-usage-loader';
import { useTaskThreadTitleSync } from './task-title-sync';
import {
  TaskThreadDetailStatus,
  type TaskThreadDetailEvent,
  type TaskThreadDetailModel,
} from './task-thread-detail-model';
import { useTaskThreadRunEventStream } from './task-run-event-stream';
import { useTaskRunActions } from './task-run-actions-hook';
import type { PendingHumanInteraction } from './task-human-interaction';
import {
  createFollowUpIdempotencyKey,
  sendFollowUpMessage,
  type CanonicalThreadFollowUpResult,
} from './task-follow-up';
import type { TaskTokenUsageSnapshot } from './task-detail-token-usage';
import {
  fetchTaskDetail,
  type TaskDetail,
  type TaskDetailSubagentRun,
  type TaskDetailTokenUsage,
} from './task-detail-loader';
import { listTaskThreadArtifacts, resumeTaskThreadRun } from './service';
import { isTaskTerminalStatus } from './helpers';

type TaskThreadMessage = WorkbenchMessage;
type TaskThreadArtifact = WorkbenchArtifact;
type TaskThreadTodo = WorkbenchTodo;

const EMPTY_TASK_ARTIFACTS: TaskThreadArtifact[] = [];
const EMPTY_TASK_EVENTS: TaskThreadDetailEvent[] = [];
const EMPTY_TASK_MESSAGES: TaskThreadMessage[] = [];
const EMPTY_TASK_SUBAGENT_RUNS: TaskDetailSubagentRun[] = [];
const EMPTY_TASK_TODOS: TaskThreadTodo[] = [];
const TASK_DETAIL_POLLING_DELAY_MS = 2000;
const createTaskScopeKey = (spaceID?: string, taskDetailId?: string) =>
  JSON.stringify([spaceID ?? '', taskDetailId ?? '']);
const RUN_TERMINAL_STATUSES = new Set([
  'success',
  'succeeded',
  'error',
  'failed',
  'canceled',
  'interrupted',
]);

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') {
    return false;
  }

  const abortCandidate = error as { code?: unknown; name?: unknown };

  return (
    abortCandidate.name === 'AbortError' || abortCandidate.code === 'ABORT_ERR'
  );
};

const isAmbiguousFollowUpError = (error: unknown) => {
  if (!error || typeof error !== 'object') {
    return false;
  }

  const candidate = error as { message?: unknown; name?: unknown };
  const message =
    typeof candidate.message === 'string' ? candidate.message : '';

  return (
    candidate.name === 'TimeoutError' ||
    candidate.name === 'NetworkError' ||
    /timeout|timed out|network|fetch|超时|网络/i.test(message)
  );
};

const shouldPollTaskDetail = (detail: TaskDetail) => {
  const latestRunStatus = String(detail.latestTaskRunStatus ?? '')
    .trim()
    .toLowerCase();
  if (latestRunStatus) {
    return !RUN_TERMINAL_STATUSES.has(latestRunStatus);
  }

  return Boolean(detail.task && !isTaskTerminalStatus(detail.task.status));
};

const mapRunStatusToOptimisticTaskStatus = (status?: string) => {
  switch (
    String(status ?? '')
      .trim()
      .toLowerCase()
  ) {
    case 'created':
      return TaskThreadDetailStatus.Created;
    case 'pending':
    case 'queued':
      return TaskThreadDetailStatus.Queued;
    case 'completed':
    case 'success':
    case 'succeeded':
      return TaskThreadDetailStatus.Succeeded;
    case 'error':
    case 'failed':
      return TaskThreadDetailStatus.Failed;
    case 'canceling':
      return TaskThreadDetailStatus.Canceling;
    case 'canceled':
    case 'cancelled':
      return TaskThreadDetailStatus.Canceled;
    case 'running':
    default:
      return TaskThreadDetailStatus.Running;
  }
};

const compareNumericRunIDs = (left: string, right: string) => {
  const normalize = (value: string) => {
    const trimmed = value.trim();
    if (!/^\d+$/.test(trimmed)) {
      return undefined;
    }

    return trimmed.replace(/^0+(?=\d)/, '');
  };
  const normalizedLeft = normalize(left);
  const normalizedRight = normalize(right);
  if (!normalizedLeft || !normalizedRight) {
    return undefined;
  }
  if (normalizedLeft.length !== normalizedRight.length) {
    return normalizedLeft.length - normalizedRight.length;
  }

  return normalizedLeft.localeCompare(normalizedRight);
};

const isRunCurrentOrNewer = ({
  currentCreatedAt,
  currentRunID,
  incomingCreatedAt,
  incomingRunID,
}: {
  currentCreatedAt: number;
  currentRunID: string;
  incomingCreatedAt: number;
  incomingRunID: string;
}) => {
  if (!currentRunID) {
    return true;
  }
  if (incomingRunID === currentRunID) {
    return true;
  }
  if (!incomingRunID || incomingCreatedAt !== currentCreatedAt) {
    return incomingCreatedAt > currentCreatedAt;
  }

  const idComparison = compareNumericRunIDs(incomingRunID, currentRunID);
  return idComparison !== undefined && idComparison > 0;
};

const applyRunStateToTask = (
  task: TaskThreadDetailModel | undefined,
  run: WorkbenchRun,
) => {
  if (!task) {
    return task;
  }

  const status = mapRunStatusToOptimisticTaskStatus(run.status);
  const active = !isTaskTerminalStatus(status);

  return {
    ...task,
    status,
    progress:
      status === TaskThreadDetailStatus.Succeeded
        ? 100
        : active
          ? Math.max(task.progress || 0, 1)
          : task.progress,
    error:
      status === TaskThreadDetailStatus.Failed
        ? run.terminal_reason || task.error || ''
        : '',
    updated_at: Math.max(
      task.updated_at || 0,
      run.updated_at || 0,
      run.created_at || 0,
    ),
  };
};

const appendOptimisticMessage = (
  messages: TaskThreadMessage[],
  message: TaskThreadMessage,
) => {
  if (
    message.message_id &&
    messages.some(item => item.message_id === message.message_id)
  ) {
    return messages;
  }

  return [...messages, message];
};

interface TaskDetailCollectionSnapshot {
  artifacts: TaskThreadArtifact[];
  events: TaskThreadDetailEvent[];
  messages: TaskThreadMessage[];
}

export interface TaskDetailRequestToken {
  generation: number;
  taskScopeKey: string;
}

interface TaskDetailRequestSnapshot {
  baseRevision: number;
  collections: TaskDetailCollectionSnapshot;
  runRevision: number;
}

const mergeCollectionWithLocalDelta = <T>(
  baseline: T[],
  current: T[],
  incoming: T[],
  getID: (item: T) => string,
): T[] => {
  const baselineByID = new Map(baseline.map(item => [getID(item), item]));
  const currentByID = new Map(current.map(item => [getID(item), item]));
  const resultByID = new Map(incoming.map(item => [getID(item), item]));

  baselineByID.forEach((_item, id) => {
    if (!currentByID.has(id)) {
      resultByID.delete(id);
    }
  });
  currentByID.forEach((item, id) => {
    if (!baselineByID.has(id) || baselineByID.get(id) !== item) {
      resultByID.set(id, item);
    }
  });

  return Array.from(resultByID.values());
};

const useLoadTaskDetailEffect = ({
  applyTaskDetail,
  captureTaskDetailRequestToken,
  setError,
  setLoadedThreadId,
  setLoading,
  spaceID,
  taskDetailId,
}: {
  applyTaskDetail: (
    detail: TaskDetail,
    taskDetailId?: string,
    requestToken?: TaskDetailRequestToken,
  ) => void;
  captureTaskDetailRequestToken: () => TaskDetailRequestToken;
  setError: (value: string) => void;
  setLoadedThreadId: (value: string) => void;
  setLoading: (value: boolean) => void;
  spaceID?: string;
  taskDetailId?: string;
}) => {
  useEffect(() => {
    if (!spaceID || !taskDetailId) {
      return;
    }
    const submittedSpaceID = spaceID;
    let canceled = false;
    const loadTaskDetail = async () => {
      setLoading(true);
      setLoadedThreadId('');
      setError('');
      const requestToken = captureTaskDetailRequestToken();
      try {
        const detail = await fetchTaskDetail({
          id: taskDetailId,
          spaceId: submittedSpaceID,
        });
        if (!canceled) {
          applyTaskDetail(detail, taskDetailId, requestToken);
        }
      } catch (err) {
        if (!canceled) {
          setError(err instanceof Error ? err.message : '加载任务详情失败');
        }
      } finally {
        if (!canceled) {
          setLoading(false);
        }
      }
    };
    void loadTaskDetail();
    return () => {
      canceled = true;
    };
  }, [
    applyTaskDetail,
    captureTaskDetailRequestToken,
    setError,
    setLoadedThreadId,
    setLoading,
    spaceID,
    taskDetailId,
  ]);
};

const usePollTaskDetailEffect = ({
  applyTaskDetail,
  captureTaskDetailRequestToken,
  pollingVersion,
  setError,
  spaceID,
  taskDetailId,
}: {
  applyTaskDetail: (
    detail: TaskDetail,
    taskDetailId?: string,
    requestToken?: TaskDetailRequestToken,
  ) => void;
  captureTaskDetailRequestToken: () => TaskDetailRequestToken;
  pollingVersion: number;
  setError: (value: string) => void;
  spaceID?: string;
  taskDetailId?: string;
}) => {
  useEffect(() => {
    if (!spaceID || !taskDetailId || pollingVersion <= 0) {
      return;
    }
    const submittedSpaceID = spaceID;

    let canceled = false;
    const timer = setTimeout(() => {
      const refreshTaskDetail = async () => {
        setError('');
        const requestToken = captureTaskDetailRequestToken();
        try {
          const detail = await fetchTaskDetail({
            id: taskDetailId,
            spaceId: submittedSpaceID,
          });
          if (!canceled) {
            applyTaskDetail(detail, taskDetailId, requestToken);
          }
        } catch (err) {
          if (!canceled) {
            setError(err instanceof Error ? err.message : '加载任务详情失败');
          }
        }
      };
      void refreshTaskDetail();
    }, TASK_DETAIL_POLLING_DELAY_MS);

    return () => {
      canceled = true;
      clearTimeout(timer);
    };
  }, [
    applyTaskDetail,
    captureTaskDetailRequestToken,
    pollingVersion,
    setError,
    spaceID,
    taskDetailId,
  ]);
};

export const useTaskDetailData = ({
  spaceID,
  taskDetailId,
}: {
  spaceID?: string;
  taskDetailId?: string;
}) => {
  const [task, setTask] = useState<TaskThreadDetailModel | undefined>();
  const [events, setEvents] = useState<TaskThreadDetailEvent[]>([]);
  const [messages, setMessages] = useState<TaskThreadMessage[]>([]);
  const [todos, setTodos] = useState<TaskThreadTodo[]>([]);
  const [artifacts, setArtifacts] = useState<WorkbenchArtifact[]>([]);
  const [latestTaskRunID, setLatestTaskRunID] = useState('');
  const latestTaskRunIDRef = useRef('');
  const latestTaskRunCreatedAtRef = useRef(0);
  const runRevisionRef = useRef(0);
  const [suggestionModelName, setSuggestionModelName] = useState('');
  const [suggestionModelType, setSuggestionModelType] = useState('');
  const [subagentRuns, setSubagentRuns] = useState<TaskDetailSubagentRun[]>([]);
  const [loadedTaskDetailId, setLoadedTaskDetailId] = useState('');
  const [loadedSpaceID, setLoadedSpaceID] = useState('');
  const [loadedThreadId, setLoadedThreadId] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [errorTaskScopeKey, setErrorTaskScopeKey] = useState('');
  const [pollingVersion, setPollingVersion] = useState(0);
  const taskScopeKey = createTaskScopeKey(spaceID, taskDetailId);
  const mountedRef = useRef(true);
  const scopedTaskScopeKeyRef = useRef(taskScopeKey);
  const scopedTaskDetailIdRef = useRef(taskDetailId);
  const scopedSpaceIDRef = useRef(spaceID);
  const taskDetailGenerationRef = useRef(0);
  const taskDetailGeneration =
    scopedTaskScopeKeyRef.current === taskScopeKey
      ? taskDetailGenerationRef.current
      : taskDetailGenerationRef.current + 1;
  useLayoutEffect(() => {
    if (scopedTaskScopeKeyRef.current === taskScopeKey) {
      return;
    }

    scopedTaskScopeKeyRef.current = taskScopeKey;
    scopedTaskDetailIdRef.current = taskDetailId;
    scopedSpaceIDRef.current = spaceID;
    taskDetailGenerationRef.current = taskDetailGeneration;
  }, [spaceID, taskDetailGeneration, taskDetailId, taskScopeKey]);
  const collectionStateRef = useRef<TaskDetailCollectionSnapshot>({
    artifacts: [],
    events: [],
    messages: [],
  });
  const authoritativeCollectionRef = useRef<TaskDetailCollectionSnapshot>({
    artifacts: [],
    events: [],
    messages: [],
  });
  const collectionRevisionRef = useRef(0);
  const requestSnapshotsRef = useRef<
    WeakMap<TaskDetailRequestToken, TaskDetailRequestSnapshot>
  >(new WeakMap());
  const isCurrentTaskScope = useCallback(
    (submittedTaskScopeKey: string, generation: number) =>
      mountedRef.current &&
      scopedTaskScopeKeyRef.current === submittedTaskScopeKey &&
      taskDetailGenerationRef.current === generation,
    [],
  );
  const setCurrentScopeError = useCallback(
    (value: string) => {
      setErrorTaskScopeKey(taskScopeKey);
      setError(value);
    },
    [taskScopeKey],
  );
  const { handleThreadTitleUpdated, setCurrentTask } = useTaskThreadTitleSync({
    setTask,
    spaceID,
  });
  const captureTaskDetailRequestToken = useCallback(() => {
    const requestToken: TaskDetailRequestToken = {
      generation: taskDetailGenerationRef.current,
      taskScopeKey: scopedTaskScopeKeyRef.current,
    };
    requestSnapshotsRef.current.set(requestToken, {
      baseRevision: collectionRevisionRef.current,
      collections: collectionStateRef.current,
      runRevision: runRevisionRef.current,
    });

    return requestToken;
  }, []);
  const applyTaskDetail = useCallback(
    (
      detail: TaskDetail,
      appliedTaskDetailId?: string,
      requestToken?: TaskDetailRequestToken,
    ) => {
      if (
        requestToken &&
        !isCurrentTaskScope(requestToken.taskScopeKey, requestToken.generation)
      ) {
        requestSnapshotsRef.current.delete(requestToken);
        return;
      }
      const detailIdentity =
        appliedTaskDetailId ??
        detail.threadId ??
        detail.task?.id ??
        scopedTaskDetailIdRef.current;
      const detailSpaceID = detail.task?.space_id;
      if (
        (detailIdentity && scopedTaskDetailIdRef.current !== detailIdentity) ||
        (detailSpaceID &&
          scopedSpaceIDRef.current &&
          detailSpaceID !== scopedSpaceIDRef.current)
      ) {
        return;
      }
      const incomingCollections: TaskDetailCollectionSnapshot = {
        artifacts: detail.artifacts ?? [],
        events: detail.events,
        messages: detail.messages ?? [],
      };
      const requestSnapshot = requestToken
        ? requestSnapshotsRef.current.get(requestToken)
        : undefined;
      if (requestToken) {
        requestSnapshotsRef.current.delete(requestToken);
      }
      const incomingRunID = detail.latestTaskRunID ?? '';
      const incomingRunCreatedAt = detail.latestTaskRunCreatedAt ?? 0;
      const runRevisionUnchanged =
        !requestSnapshot ||
        requestSnapshot.runRevision === runRevisionRef.current;
      const incomingRunIsCurrentOrNewer = isRunCurrentOrNewer({
        currentCreatedAt: latestTaskRunCreatedAtRef.current,
        currentRunID: latestTaskRunIDRef.current,
        incomingCreatedAt: incomingRunCreatedAt,
        incomingRunID,
      });
      if (!runRevisionUnchanged || !incomingRunIsCurrentOrNewer) {
        return;
      }
      const baselineCollections =
        requestSnapshot?.collections ?? authoritativeCollectionRef.current;
      const hasConcurrentCollectionChanges = requestSnapshot
        ? collectionRevisionRef.current !== requestSnapshot.baseRevision
        : true;
      const nextCollections = hasConcurrentCollectionChanges
        ? {
            artifacts: mergeCollectionWithLocalDelta(
              baselineCollections.artifacts,
              collectionStateRef.current.artifacts,
              incomingCollections.artifacts,
              artifact => artifact.artifact_id,
            ),
            events: mergeCollectionWithLocalDelta(
              baselineCollections.events,
              collectionStateRef.current.events,
              incomingCollections.events,
              event => event.id,
            ),
            messages: mergeCollectionWithLocalDelta(
              baselineCollections.messages,
              collectionStateRef.current.messages,
              incomingCollections.messages,
              message => message.message_id,
            ),
          }
        : incomingCollections;
      collectionStateRef.current = nextCollections;
      authoritativeCollectionRef.current = nextCollections;
      collectionRevisionRef.current += 1;
      setLoadedTaskDetailId(detailIdentity ?? '');
      setLoadedSpaceID(scopedSpaceIDRef.current ?? '');
      setLoadedThreadId(detail.threadId ?? '');
      setCurrentTask(detail.task);
      setEvents(nextCollections.events);
      setMessages(nextCollections.messages);
      setTodos(detail.todos ?? []);
      setArtifacts(nextCollections.artifacts);
      const currentRunID = latestTaskRunIDRef.current;
      if (incomingRunID !== currentRunID) {
        runRevisionRef.current += 1;
      }
      latestTaskRunIDRef.current = incomingRunID;
      latestTaskRunCreatedAtRef.current =
        incomingRunID === currentRunID
          ? Math.max(latestTaskRunCreatedAtRef.current, incomingRunCreatedAt)
          : incomingRunCreatedAt;
      setLatestTaskRunID(incomingRunID);
      setSuggestionModelName(detail.suggestionModelName ?? '');
      setSuggestionModelType(detail.suggestionModelType ?? '');
      setSubagentRuns(detail.subagentRuns ?? []);
      if (shouldPollTaskDetail(detail)) {
        setPollingVersion(version => version + 1);
      }
    },
    [isCurrentTaskScope, setCurrentTask],
  );
  const setScopedEvents = useCallback(
    (nextEvents: SetStateAction<TaskThreadDetailEvent[]>) => {
      if (isCurrentTaskScope(taskScopeKey, taskDetailGeneration)) {
        setEvents(currentEvents => {
          const resolvedEvents =
            typeof nextEvents === 'function'
              ? nextEvents(currentEvents)
              : nextEvents;
          collectionStateRef.current = {
            ...collectionStateRef.current,
            events: resolvedEvents,
          };
          collectionRevisionRef.current += 1;
          return resolvedEvents;
        });
      }
    },
    [isCurrentTaskScope, taskDetailGeneration, taskScopeKey],
  );
  const handleScopedThreadTitleUpdated = useCallback(
    (...args: Parameters<typeof handleThreadTitleUpdated>) => {
      if (isCurrentTaskScope(taskScopeKey, taskDetailGeneration)) {
        handleThreadTitleUpdated(...args);
      }
    },
    [
      handleThreadTitleUpdated,
      isCurrentTaskScope,
      taskDetailGeneration,
      taskScopeKey,
    ],
  );
  const scopedThreadTitleUpdatedRef = useRef(handleScopedThreadTitleUpdated);
  useLayoutEffect(() => {
    scopedThreadTitleUpdatedRef.current = handleScopedThreadTitleUpdated;
  }, [handleScopedThreadTitleUpdated]);
  const handleStableScopedThreadTitleUpdated = useCallback(
    (...args: Parameters<typeof handleScopedThreadTitleUpdated>) => {
      const [update] = args;
      if (update.threadId !== scopedTaskDetailIdRef.current) {
        return;
      }
      scopedThreadTitleUpdatedRef.current(...args);
    },
    [],
  );
  const applyOptimisticFollowUp = useCallback(
    ({
      followUpResult,
      payload,
      threadId,
    }: {
      followUpResult?: CanonicalThreadFollowUpResult;
      payload: WorkbenchComposerSubmitPayload;
      threadId: string;
    }) => {
      const runID = followUpResult?.run?.run_id;
      if (
        !runID ||
        threadId !== scopedTaskDetailIdRef.current ||
        followUpResult.run?.space_id !== scopedSpaceIDRef.current ||
        !mountedRef.current
      ) {
        return;
      }

      const appendedMessage = followUpResult.message;
      const now = Date.now();
      const optimisticUserMessage: TaskThreadMessage = {
        message_id: appendedMessage?.message_id || `pending-${runID}`,
        thread_id: threadId,
        run_id: runID,
        role: 'user',
        content: appendedMessage?.content || payload.message,
        metadata: appendedMessage?.metadata || '',
        created_at: appendedMessage?.created_at || now,
      };

      setTask(currentTask =>
        currentTask
          ? {
              ...currentTask,
              status: mapRunStatusToOptimisticTaskStatus(
                followUpResult.run?.status,
              ),
              progress: Math.max(currentTask.progress || 0, 1),
              last_user_message: optimisticUserMessage.content,
              last_agent_message: '',
              updated_at: now,
            }
          : currentTask,
      );
      setMessages(currentMessages => {
        const nextMessages = appendOptimisticMessage(
          currentMessages,
          optimisticUserMessage,
        );
        collectionStateRef.current = {
          ...collectionStateRef.current,
          messages: nextMessages,
        };
        collectionRevisionRef.current += 1;
        return nextMessages;
      });
      const run = followUpResult?.run;
      if (
        run &&
        (!latestTaskRunIDRef.current ||
          run.run_id === latestTaskRunIDRef.current ||
          run.created_at >= latestTaskRunCreatedAtRef.current)
      ) {
        if (run.run_id !== latestTaskRunIDRef.current) {
          runRevisionRef.current += 1;
        }
        latestTaskRunIDRef.current = run.run_id;
        latestTaskRunCreatedAtRef.current = run.created_at;
        setLatestTaskRunID(run.run_id);
      }
    },
    [],
  );
  const commitTopLevelRun = useCallback(
    (run: WorkbenchRun) => {
      if (
        !mountedRef.current ||
        run.thread_id !== scopedTaskDetailIdRef.current ||
        run.space_id !== scopedSpaceIDRef.current ||
        (run.parent_run_id && run.parent_run_id !== '0')
      ) {
        return;
      }

      const currentRunID = latestTaskRunIDRef.current;
      if (
        currentRunID &&
        !isRunCurrentOrNewer({
          currentCreatedAt: latestTaskRunCreatedAtRef.current,
          currentRunID,
          incomingCreatedAt: run.created_at,
          incomingRunID: run.run_id,
        })
      ) {
        return;
      }
      if (run.run_id !== currentRunID) {
        runRevisionRef.current += 1;
      }
      latestTaskRunIDRef.current = run.run_id;
      latestTaskRunCreatedAtRef.current =
        run.run_id === currentRunID
          ? Math.max(latestTaskRunCreatedAtRef.current, run.created_at)
          : run.created_at;
      setLatestTaskRunID(run.run_id);
      setCurrentTask(currentTask => applyRunStateToTask(currentTask, run));
    },
    [setCurrentTask],
  );
  const loadedTaskDetailCurrent =
    Boolean(taskDetailId) &&
    loadedTaskDetailId === taskDetailId &&
    loadedSpaceID === (spaceID ?? '');
  const taskUsage = useTaskUsageData({
    enabled: loadedTaskDetailCurrent && Boolean(task && loadedThreadId),
    refreshKey:
      task && isTaskTerminalStatus(task.status) ? 'terminal' : 'active',
    spaceID,
    threadID: loadedThreadId,
  });
  const tokenUsageSnapshotHandlerRef = useRef(
    taskUsage.handleTokenUsageSnapshot,
  );
  const tokenUsageStreamThreadIDRef = useRef('');
  useLayoutEffect(() => {
    tokenUsageSnapshotHandlerRef.current = taskUsage.handleTokenUsageSnapshot;
    tokenUsageStreamThreadIDRef.current =
      loadedTaskDetailCurrent && task ? loadedThreadId : '';
  }, [loadedTaskDetailCurrent, loadedThreadId, task, taskUsage]);
  const handleScopedTokenUsageSnapshot = useCallback(
    (snapshot: TaskTokenUsageSnapshot, event: WorkbenchRunEvent) => {
      const currentThreadID = tokenUsageStreamThreadIDRef.current;
      if (
        !currentThreadID ||
        String(event.thread_id ?? '') !== currentThreadID
      ) {
        return;
      }
      tokenUsageSnapshotHandlerRef.current(snapshot);
    },
    [],
  );
  const refreshArtifacts = useCallback(async () => {
    if (!loadedTaskDetailCurrent || !loadedThreadId || !task) {
      return;
    }
    const submittedTaskScopeKey = taskScopeKey;
    const requestGeneration = taskDetailGenerationRef.current;

    const response = await listTaskThreadArtifacts({
      thread_id: loadedThreadId,
      space_id: spaceID,
      page: 1,
      page_size: 50,
    });
    if (isCurrentTaskScope(submittedTaskScopeKey, requestGeneration)) {
      const nextArtifacts = response.data?.artifacts ?? [];
      collectionStateRef.current = {
        ...collectionStateRef.current,
        artifacts: nextArtifacts,
      };
      collectionRevisionRef.current += 1;
      setArtifacts(nextArtifacts);
    }
  }, [
    isCurrentTaskScope,
    loadedTaskDetailCurrent,
    loadedThreadId,
    spaceID,
    task,
    taskDetailId,
    taskScopeKey,
  ]);

  useEffect(() => {
    mountedRef.current = true;

    return () => {
      mountedRef.current = false;
      taskDetailGenerationRef.current += 1;
    };
  }, []);

  useEffect(() => {
    const emptyCollections: TaskDetailCollectionSnapshot = {
      artifacts: [],
      events: [],
      messages: [],
    };
    collectionStateRef.current = emptyCollections;
    authoritativeCollectionRef.current = emptyCollections;
    collectionRevisionRef.current = 0;
    requestSnapshotsRef.current = new WeakMap();
    setTask(undefined);
    setEvents([]);
    setMessages([]);
    setTodos([]);
    setArtifacts([]);
    latestTaskRunIDRef.current = '';
    latestTaskRunCreatedAtRef.current = 0;
    runRevisionRef.current += 1;
    setLatestTaskRunID('');
    setSuggestionModelName('');
    setSuggestionModelType('');
    setSubagentRuns([]);
    setLoadedTaskDetailId('');
    setLoadedSpaceID('');
    setLoadedThreadId('');
    setErrorTaskScopeKey(taskScopeKey);
    setError('');
  }, [taskScopeKey]);

  useLoadTaskDetailEffect({
    applyTaskDetail,
    captureTaskDetailRequestToken,
    setError: setCurrentScopeError,
    setLoadedThreadId,
    setLoading,
    spaceID,
    taskDetailId,
  });
  usePollTaskDetailEffect({
    applyTaskDetail,
    captureTaskDetailRequestToken,
    pollingVersion,
    setError: setCurrentScopeError,
    spaceID,
    taskDetailId,
  });

  useTaskThreadRunEventStream({
    enabled:
      loadedTaskDetailCurrent &&
      Boolean(task && loadedThreadId && latestTaskRunID && spaceID),
    onTokenUsageSnapshot: handleScopedTokenUsageSnapshot,
    onThreadTitleUpdated: handleStableScopedThreadTitleUpdated,
    runId: latestTaskRunID,
    setEvents: setScopedEvents,
    spaceId: spaceID,
    threadId: loadedThreadId,
  });

  return {
    applyTaskDetail,
    applyOptimisticFollowUp,
    artifacts: loadedTaskDetailCurrent ? artifacts : EMPTY_TASK_ARTIFACTS,
    captureTaskDetailRequestToken,
    commitTopLevelRun,
    error: errorTaskScopeKey === taskScopeKey ? error : '',
    events: loadedTaskDetailCurrent ? events : EMPTY_TASK_EVENTS,
    messages: loadedTaskDetailCurrent ? messages : EMPTY_TASK_MESSAGES,
    latestTaskRunID: loadedTaskDetailCurrent ? latestTaskRunID : '',
    suggestionModelName: loadedTaskDetailCurrent ? suggestionModelName : '',
    suggestionModelType: loadedTaskDetailCurrent ? suggestionModelType : '',
    loadedTaskDetailCurrent,
    loadedThreadId: loadedTaskDetailCurrent ? loadedThreadId : '',
    loading:
      Boolean(taskDetailId) &&
      (loading ||
        (!loadedTaskDetailCurrent &&
          !(errorTaskScopeKey === taskScopeKey && error))),
    refreshArtifacts,
    subagentRuns: loadedTaskDetailCurrent
      ? subagentRuns
      : EMPTY_TASK_SUBAGENT_RUNS,
    task: loadedTaskDetailCurrent ? task : undefined,
    todos: loadedTaskDetailCurrent ? todos : EMPTY_TASK_TODOS,
    retryTokenUsage: taskUsage.retry,
    tokenUsage: loadedTaskDetailCurrent ? taskUsage.tokenUsage : undefined,
    tokenUsageByRunID: loadedTaskDetailCurrent
      ? taskUsage.tokenUsageByRunID
      : {},
    tokenUsageError: loadedTaskDetailCurrent ? taskUsage.error : '',
    tokenUsageIsPartial: loadedTaskDetailCurrent && taskUsage.isPartial,
    tokenUsageLoadedCount: loadedTaskDetailCurrent ? taskUsage.loadedCount : 0,
    tokenUsageLoading: loadedTaskDetailCurrent && taskUsage.loading,
    tokenUsageTotalCount: loadedTaskDetailCurrent ? taskUsage.totalCount : 0,
  };
};

interface TaskDetailActionsOptions {
  applyTaskDetail: (
    detail: TaskDetail,
    taskDetailId?: string,
    requestToken?: TaskDetailRequestToken,
  ) => void;
  applyOptimisticFollowUp?: (input: {
    followUpResult?: CanonicalThreadFollowUpResult;
    payload: WorkbenchComposerSubmitPayload;
    threadId: string;
  }) => void;
  captureTaskDetailRequestToken?: () => TaskDetailRequestToken;
  commitTopLevelRun?: (run: WorkbenchRun) => void;
  artifacts: TaskThreadArtifact[];
  events: TaskThreadDetailEvent[];
  messages: TaskThreadMessage[];
  pendingHumanInteraction?: PendingHumanInteraction;
  spaceID?: string;
  subagentRuns: TaskDetailSubagentRun[];
  task?: TaskThreadDetailModel;
  taskDetailId?: string;
  todos: TaskThreadTodo[];
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageByRunID: Record<string, TaskDetailTokenUsage>;
}

export const useTaskDetailActions = ({
  applyTaskDetail,
  applyOptimisticFollowUp,
  captureTaskDetailRequestToken,
  commitTopLevelRun,
  pendingHumanInteraction,
  spaceID,
  task,
  taskDetailId,
}: TaskDetailActionsOptions) => {
  const [followUpValue, setFollowUpValue] = useState('');
  const [followUpLoading, setFollowUpLoading] = useState(false);
  const [followUpResetKey, setFollowUpResetKey] = useState(0);
  const [followUpError, setFollowUpError] = useState('');
  const [humanInteractionLoading, setHumanInteractionLoading] = useState(false);
  const [humanInteractionError, setHumanInteractionError] = useState('');
  const taskScopeKey = createTaskScopeKey(spaceID, taskDetailId);
  const scopedTaskScopeKeyRef = useRef(taskScopeKey);
  const followUpRequestGenerationRef = useRef(0);
  const humanInteractionRequestGenerationRef = useRef(0);
  const successfulHumanInteractionKeysRef = useRef(new Set<string>());
  const mountedRef = useRef(true);
  const followUpAttemptRef = useRef<{
    fingerprint: string;
    idempotencyKey: string;
  }>();
  const fileIdentityRef = useRef(new WeakMap<File, number>());
  const nextFileIdentityRef = useRef(1);
  useLayoutEffect(() => {
    if (scopedTaskScopeKeyRef.current === taskScopeKey) {
      return;
    }

    scopedTaskScopeKeyRef.current = taskScopeKey;
    followUpRequestGenerationRef.current += 1;
    humanInteractionRequestGenerationRef.current += 1;
    successfulHumanInteractionKeysRef.current.clear();
  }, [taskScopeKey]);
  const taskRunActions = useTaskRunActions({
    applyTaskDetail,
    captureTaskDetailRequestToken,
    commitTopLevelRun,
    spaceID,
    task,
    taskDetailId,
  });

  useEffect(() => {
    mountedRef.current = true;

    return () => {
      mountedRef.current = false;
      followUpRequestGenerationRef.current += 1;
      humanInteractionRequestGenerationRef.current += 1;
    };
  }, []);

  useEffect(() => {
    setFollowUpValue('');
    setFollowUpLoading(false);
    setFollowUpResetKey(0);
    setFollowUpError('');
    setHumanInteractionLoading(false);
    setHumanInteractionError('');
    followUpAttemptRef.current = undefined;
  }, [taskScopeKey]);

  const getFollowUpFingerprint = (payload: WorkbenchComposerSubmitPayload) =>
    JSON.stringify({
      message: payload.message,
      modelType: payload.modelType,
      modelName: payload.modelName,
      taskId: payload.taskId,
      enableSkills: [...(payload.enable_skills ?? [])].sort(),
      enableMCP: [...(payload.enable_mcp ?? [])].sort(),
      enableKnowledge: [...(payload.enable_kbs ?? [])].sort(),
      enableDatabases: [...(payload.enable_databases ?? [])].sort(),
      runtimeSettings: payload.runtimeSettings,
      files: (payload.files ?? []).map(file => {
        let identity = fileIdentityRef.current.get(file);
        if (!identity) {
          identity = nextFileIdentityRef.current++;
          fileIdentityRef.current.set(file, identity);
        }

        return {
          identity,
          name: file.name,
          size: file.size,
          type: file.type,
          lastModified: file.lastModified,
        };
      }),
    });

  const handleFollowUpSubmit = async (
    payload: WorkbenchComposerSubmitPayload,
  ) => {
    if (!payload.message || followUpLoading) {
      return;
    }
    if (!spaceID || !taskDetailId) {
      setFollowUpError('缺少任务上下文，无法继续追问');
      return;
    }
    const submittedTaskDetailId = taskDetailId;
    const submittedSpaceID = spaceID;
    const submittedTaskScopeKey = taskScopeKey;
    const fingerprint = getFollowUpFingerprint(payload);
    if (followUpAttemptRef.current?.fingerprint !== fingerprint) {
      followUpAttemptRef.current = {
        fingerprint,
        idempotencyKey: createFollowUpIdempotencyKey(
          `${submittedSpaceID}:${submittedTaskDetailId}`,
        ),
      };
    }
    const { idempotencyKey } = followUpAttemptRef.current;
    const requestGeneration = ++followUpRequestGenerationRef.current;
    const isCurrentRequest = () =>
      mountedRef.current &&
      followUpRequestGenerationRef.current === requestGeneration &&
      scopedTaskScopeKeyRef.current === submittedTaskScopeKey;
    setFollowUpLoading(true);
    setFollowUpError('');
    let followUpResult: Awaited<ReturnType<typeof sendFollowUpMessage>>;
    try {
      followUpResult = await sendFollowUpMessage({
        payload,
        spaceId: submittedSpaceID,
        threadId: submittedTaskDetailId,
        idempotencyKey,
      });
    } catch (err) {
      if (!isCurrentRequest()) {
        return;
      }
      setFollowUpError(
        isAbortError(err)
          ? '发送已取消'
          : err instanceof Error
            ? err.message
            : '继续追问失败，请稍后重试',
      );
      if (isAbortError(err) || !isAmbiguousFollowUpError(err)) {
        followUpAttemptRef.current = undefined;
      }
      setFollowUpLoading(false);
      return;
    }

    if (!isCurrentRequest()) {
      return;
    }

    followUpAttemptRef.current = undefined;
    if (followUpResult.run) {
      commitTopLevelRun?.(followUpResult.run);
    }
    applyOptimisticFollowUp?.({
      followUpResult,
      payload,
      threadId: submittedTaskDetailId,
    });

    setFollowUpValue('');
    setFollowUpResetKey(resetKey => resetKey + 1);
    const refreshRequestToken = captureTaskDetailRequestToken?.();
    try {
      const detail = await fetchTaskDetail({
        id: submittedTaskDetailId,
        spaceId: submittedSpaceID,
      });
      if (isCurrentRequest()) {
        applyTaskDetail(detail, submittedTaskDetailId, refreshRequestToken);
      }
    } catch (err) {
      if (isCurrentRequest()) {
        setFollowUpError('消息已发送但刷新失败，可重试刷新');
      }
    } finally {
      if (isCurrentRequest()) {
        setFollowUpLoading(false);
      }
    }
  };

  const handleHumanInteractionSubmit = async (
    response: HumanInteractionResponse,
  ) => {
    if (!pendingHumanInteraction || humanInteractionLoading) {
      return;
    }
    if (!spaceID || !taskDetailId || !pendingHumanInteraction.sourceRunId) {
      setHumanInteractionError('缺少任务恢复上下文，请刷新后重试');
      return;
    }

    const submittedTaskDetailId = taskDetailId;
    const submittedSpaceID = spaceID;
    const submittedTaskScopeKey = taskScopeKey;
    const mutationKey = `${submittedTaskScopeKey}:${pendingHumanInteraction.sourceRunId}:${pendingHumanInteraction.interruptId}`;
    const requestGeneration = ++humanInteractionRequestGenerationRef.current;
    const isCurrentRequest = () =>
      mountedRef.current &&
      humanInteractionRequestGenerationRef.current === requestGeneration &&
      scopedTaskScopeKeyRef.current === submittedTaskScopeKey;
    setHumanInteractionLoading(true);
    setHumanInteractionError('');
    try {
      if (!successfulHumanInteractionKeysRef.current.has(mutationKey)) {
        const resumeResult = await resumeTaskThreadRun({
          thread_id: submittedTaskDetailId,
          run_id: pendingHumanInteraction.sourceRunId,
          interrupt_id: pendingHumanInteraction.interruptId,
          response,
          space_id: submittedSpaceID,
        });
        if (!isCurrentRequest()) {
          return;
        }
        successfulHumanInteractionKeysRef.current.add(mutationKey);
        if (resumeResult.data) {
          commitTopLevelRun?.(resumeResult.data);
        }
      }
      const refreshRequestToken = captureTaskDetailRequestToken?.();
      const detail = await fetchTaskDetail({
        id: submittedTaskDetailId,
        spaceId: submittedSpaceID,
      });
      if (isCurrentRequest()) {
        applyTaskDetail(detail, submittedTaskDetailId, refreshRequestToken);
      }
    } catch (err) {
      if (isCurrentRequest()) {
        setHumanInteractionError(
          err instanceof Error ? err.message : '提交失败，请稍后重试',
        );
      }
    } finally {
      if (isCurrentRequest()) {
        setHumanInteractionLoading(false);
      }
    }
  };

  return {
    followUpError,
    followUpLoading,
    followUpResetKey,
    followUpValue,
    handleFollowUpSubmit,
    handleHumanInteractionSubmit,
    humanInteractionError,
    humanInteractionLoading,
    setFollowUpValue,
    ...taskRunActions,
  };
};
