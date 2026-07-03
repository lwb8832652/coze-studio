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
/* eslint-disable max-lines -- Task detail orchestration remains grouped during DeerFlow parity stabilization. */

import { useCallback, useEffect, useRef, useState } from 'react';

import { workbenchTask } from '@coze-studio/api-schema';

import {
  DEFAULT_WORKBENCH_MODE,
  type WorkbenchComposerSubmitPayload,
  type WorkbenchMode,
} from '../workbench/components/types';
import { useTaskThreadTitleSync } from './task-title-sync';
import { useTaskThreadRunEventStream } from './task-run-event-stream';
import { useTaskRunActions } from './task-run-actions-hook';
import type { PendingHumanInteraction } from './task-human-interaction';
import {
  sendFollowUpMessage,
  type CanonicalThreadFollowUpResult,
} from './task-follow-up';
import {
  mergeTaskTokenUsageSnapshot,
  mergeTaskTokenUsageSnapshotByRunID,
  type TaskTokenUsageSnapshot,
} from './task-detail-token-usage';
import {
  fetchTaskDetail,
  type LoadedTaskDetailSource,
  type TaskDetail,
  type TaskDetailSource,
  type TaskDetailSubagentRun,
  type TaskDetailTokenUsage,
} from './task-detail-loader';
import { listTaskThreadArtifacts, resumeTaskThreadRun } from './service';
import { isTaskTerminalStatus } from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;
type TaskThreadMessage = workbenchTask.TaskThreadMessage;
type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;
type TaskThreadTodo = workbenchTask.TaskThreadTodo;

const TASK_DETAIL_POLLING_DELAY_MS = 2000;
const RUN_TERMINAL_STATUSES = new Set([
  'succeeded',
  'failed',
  'canceled',
  'interrupted',
]);

const getInitialLoadedSource = (
  source: TaskDetailSource,
): LoadedTaskDetailSource => (source === 'thread' ? 'thread' : 'task');

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
    case 'pending':
    case 'queued':
      return workbenchTask.TaskStatus.Queued;
    case 'running':
    default:
      return workbenchTask.TaskStatus.Running;
  }
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

const useLoadTaskDetailEffect = ({
  applyTaskDetail,
  setError,
  setLoadedTaskDetailSource,
  setLoadedThreadId,
  setLoading,
  spaceID,
  taskDetailId,
  taskDetailSource,
}: {
  applyTaskDetail: (detail: TaskDetail) => void;
  setError: (value: string) => void;
  setLoadedTaskDetailSource: (value: LoadedTaskDetailSource) => void;
  setLoadedThreadId: (value: string) => void;
  setLoading: (value: boolean) => void;
  spaceID?: string;
  taskDetailId?: string;
  taskDetailSource: TaskDetailSource;
}) => {
  useEffect(() => {
    if (!taskDetailId) {
      return;
    }
    let canceled = false;
    const loadTaskDetail = async () => {
      setLoading(true);
      setLoadedTaskDetailSource(getInitialLoadedSource(taskDetailSource));
      setLoadedThreadId(taskDetailSource === 'thread' ? taskDetailId : '');
      setError('');
      try {
        const detail = await fetchTaskDetail({
          id: taskDetailId,
          spaceId: spaceID,
          source: taskDetailSource,
        });
        if (!canceled) {
          applyTaskDetail(detail);
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
    setError,
    setLoadedTaskDetailSource,
    setLoadedThreadId,
    setLoading,
    spaceID,
    taskDetailId,
    taskDetailSource,
  ]);
};

const usePollTaskDetailEffect = ({
  applyTaskDetail,
  pollingVersion,
  setError,
  spaceID,
  taskDetailId,
  taskDetailSource,
}: {
  applyTaskDetail: (detail: TaskDetail) => void;
  pollingVersion: number;
  setError: (value: string) => void;
  spaceID?: string;
  taskDetailId?: string;
  taskDetailSource: TaskDetailSource;
}) => {
  useEffect(() => {
    if (!taskDetailId || pollingVersion <= 0) {
      return;
    }

    let canceled = false;
    const timer = setTimeout(() => {
      const refreshTaskDetail = async () => {
        setError('');
        try {
          const detail = await fetchTaskDetail({
            id: taskDetailId,
            spaceId: spaceID,
            source: taskDetailSource,
          });
          if (!canceled) {
            applyTaskDetail(detail);
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
    pollingVersion,
    setError,
    spaceID,
    taskDetailId,
    taskDetailSource,
  ]);
};

const buildOptimisticFollowUpDetail = ({
  artifacts,
  events,
  followUpResult,
  messages,
  payload,
  subagentRuns,
  task,
  threadId,
  todos,
  tokenUsage,
  tokenUsageByRunID,
}: {
  artifacts: TaskThreadArtifact[];
  events: TaskEvent[];
  followUpResult?: CanonicalThreadFollowUpResult;
  messages: TaskThreadMessage[];
  payload: WorkbenchComposerSubmitPayload;
  subagentRuns: TaskDetailSubagentRun[];
  task?: ChatTask;
  threadId: string;
  todos: TaskThreadTodo[];
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageByRunID: Record<string, TaskDetailTokenUsage>;
}): TaskDetail | undefined => {
  const runID = followUpResult?.run?.run_id;
  if (!task || !runID) {
    return undefined;
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

  return {
    source: 'thread',
    threadId,
    task: {
      ...task,
      status: mapRunStatusToOptimisticTaskStatus(followUpResult.run?.status),
      progress: Math.max(task.progress || 0, 1),
      last_user_message: optimisticUserMessage.content,
      last_agent_message: '',
      updated_at: now,
    },
    events,
    artifacts,
    latestTaskRunID: runID,
    latestTaskRunStatus: followUpResult.run?.status || 'running',
    messages: appendOptimisticMessage(messages, optimisticUserMessage),
    subagentRuns,
    todos,
    tokenUsage,
    tokenUsageByRunID,
  };
};

export const useTaskDetailData = ({
  spaceID,
  taskDetailId,
  taskDetailSource,
}: {
  spaceID?: string;
  taskDetailId?: string;
  taskDetailSource: TaskDetailSource;
}) => {
  const [task, setTask] = useState<ChatTask | undefined>();
  const [events, setEvents] = useState<TaskEvent[]>([]);
  const [messages, setMessages] = useState<TaskThreadMessage[]>([]);
  const [todos, setTodos] = useState<TaskThreadTodo[]>([]);
  const [artifacts, setArtifacts] = useState<
    workbenchTask.TaskThreadArtifact[]
  >([]);
  const [latestTaskRunID, setLatestTaskRunID] = useState('');
  const [suggestionModelName, setSuggestionModelName] = useState('');
  const [suggestionModelType, setSuggestionModelType] = useState('');
  const [subagentRuns, setSubagentRuns] = useState<TaskDetailSubagentRun[]>([]);
  const [tokenUsage, setTokenUsage] = useState<TaskDetailTokenUsage>();
  const [tokenUsageByRunID, setTokenUsageByRunID] = useState<
    Record<string, TaskDetailTokenUsage>
  >({});
  const [loadedTaskDetailSource, setLoadedTaskDetailSource] =
    useState<LoadedTaskDetailSource>(getInitialLoadedSource(taskDetailSource));
  const [loadedThreadId, setLoadedThreadId] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [pollingVersion, setPollingVersion] = useState(0);
  const seenTokenUsageSnapshotIDs = useRef<Set<string>>(new Set());
  const { handleThreadTitleUpdated, setCurrentTask } = useTaskThreadTitleSync({
    setTask,
    spaceID,
  });
  const applyTaskDetail = useCallback(
    (detail: TaskDetail) => {
      setLoadedTaskDetailSource(detail.source);
      setLoadedThreadId(detail.threadId ?? '');
      setCurrentTask(detail.task);
      setEvents(detail.events);
      setMessages(detail.messages ?? []);
      setTodos(detail.todos ?? []);
      setArtifacts(detail.artifacts ?? []);
      setLatestTaskRunID(detail.latestTaskRunID ?? '');
      setSuggestionModelName(detail.suggestionModelName ?? '');
      setSuggestionModelType(detail.suggestionModelType ?? '');
      setSubagentRuns(detail.subagentRuns ?? []);
      setTokenUsage(detail.tokenUsage);
      setTokenUsageByRunID(detail.tokenUsageByRunID ?? {});
      if (shouldPollTaskDetail(detail)) {
        setPollingVersion(version => version + 1);
      }
    },
    [setCurrentTask],
  );
  const handleTokenUsageSnapshot = useCallback(
    (snapshot: TaskTokenUsageSnapshot) => {
      const snapshotID =
        snapshot.usageID || `${snapshot.runID}:${snapshot.createdAt}`;
      if (seenTokenUsageSnapshotIDs.current.has(snapshotID)) {
        return;
      }
      seenTokenUsageSnapshotIDs.current.add(snapshotID);
      setTokenUsage(current => mergeTaskTokenUsageSnapshot(current, snapshot));
      setTokenUsageByRunID(current =>
        mergeTaskTokenUsageSnapshotByRunID(current, snapshot),
      );
    },
    [],
  );
  const refreshArtifacts = useCallback(async () => {
    if (!loadedThreadId || loadedTaskDetailSource !== 'thread') {
      return;
    }

    const response = await listTaskThreadArtifacts({
      thread_id: loadedThreadId,
      space_id: spaceID,
      page: 1,
      page_size: 50,
    });
    setArtifacts(response.data?.artifacts ?? []);
  }, [loadedTaskDetailSource, loadedThreadId, spaceID]);

  useLoadTaskDetailEffect({
    applyTaskDetail,
    setError,
    setLoadedTaskDetailSource,
    setLoadedThreadId,
    setLoading,
    spaceID,
    taskDetailId,
    taskDetailSource,
  });
  usePollTaskDetailEffect({
    applyTaskDetail,
    pollingVersion,
    setError,
    spaceID,
    taskDetailId,
    taskDetailSource,
  });

  useTaskThreadRunEventStream({
    enabled: loadedTaskDetailSource === 'thread' && Boolean(loadedThreadId),
    onTokenUsageSnapshot: handleTokenUsageSnapshot,
    onThreadTitleUpdated: handleThreadTitleUpdated,
    setEvents,
    threadId: loadedThreadId,
  });

  useEffect(() => {
    seenTokenUsageSnapshotIDs.current.clear();
  }, [loadedThreadId]);

  return {
    applyTaskDetail,
    artifacts,
    error,
    events,
    messages,
    latestTaskRunID,
    suggestionModelName,
    suggestionModelType,
    loadedTaskDetailSource,
    loadedThreadId,
    loading,
    refreshArtifacts,
    subagentRuns,
    task,
    todos,
    tokenUsage,
    tokenUsageByRunID,
  };
};

interface TaskDetailActionsOptions {
  applyTaskDetail: (detail: TaskDetail) => void;
  artifacts: TaskThreadArtifact[];
  events: TaskEvent[];
  messages: TaskThreadMessage[];
  pendingHumanInteraction?: PendingHumanInteraction;
  spaceID?: string;
  subagentRuns: TaskDetailSubagentRun[];
  task?: ChatTask;
  taskDetailId?: string;
  taskDetailSource: LoadedTaskDetailSource;
  todos: TaskThreadTodo[];
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageByRunID: Record<string, TaskDetailTokenUsage>;
}

export const useTaskDetailActions = ({
  applyTaskDetail,
  artifacts,
  events,
  messages,
  pendingHumanInteraction,
  spaceID,
  subagentRuns,
  task,
  taskDetailId,
  taskDetailSource,
  todos,
  tokenUsage,
  tokenUsageByRunID,
}: TaskDetailActionsOptions) => {
  const [followUpValue, setFollowUpValue] = useState('');
  const [followUpMode, setFollowUpMode] = useState<WorkbenchMode>(
    DEFAULT_WORKBENCH_MODE,
  );
  const [followUpLoading, setFollowUpLoading] = useState(false);
  const [followUpError, setFollowUpError] = useState('');
  const [humanInteractionLoading, setHumanInteractionLoading] = useState(false);
  const [humanInteractionError, setHumanInteractionError] = useState('');
  const taskRunActions = useTaskRunActions({
    applyTaskDetail,
    spaceID,
    task,
    taskDetailId,
    taskDetailSource,
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
    const isCanonicalThreadDetail = taskDetailSource === 'thread';
    const activeTaskId = task?.id ?? taskDetailId;
    setFollowUpLoading(true);
    setFollowUpError('');
    try {
      const followUpResult = await sendFollowUpMessage({
        activeTaskId,
        isCanonicalThreadDetail,
        payload,
        spaceId: spaceID,
        threadId: taskDetailId,
      });
      const optimisticDetail = isCanonicalThreadDetail
        ? buildOptimisticFollowUpDetail({
            artifacts,
            events,
            followUpResult,
            messages,
            payload,
            subagentRuns,
            task,
            threadId: taskDetailId,
            todos,
            tokenUsage,
            tokenUsageByRunID,
          })
        : undefined;

      if (optimisticDetail) {
        applyTaskDetail(optimisticDetail);
      }

      setFollowUpValue('');
      const detail = await fetchTaskDetail({
        id: taskDetailId,
        spaceId: spaceID,
        source: taskDetailSource,
      });
      applyTaskDetail(detail);
    } catch (err) {
      setFollowUpError(
        err instanceof Error ? err.message : '继续追问失败，请稍后重试',
      );
    } finally {
      setFollowUpLoading(false);
    }
  };

  const handleHumanInteractionSubmit = async (
    response: workbenchTask.HumanInteractionResponse,
  ) => {
    if (!pendingHumanInteraction || humanInteractionLoading) {
      return;
    }
    if (
      !taskDetailId ||
      taskDetailSource !== 'thread' ||
      !pendingHumanInteraction.sourceRunId
    ) {
      setHumanInteractionError('缺少任务恢复上下文，请刷新后重试');
      return;
    }

    setHumanInteractionLoading(true);
    setHumanInteractionError('');
    try {
      await resumeTaskThreadRun({
        thread_id: taskDetailId,
        run_id: pendingHumanInteraction.sourceRunId,
        interrupt_id: pendingHumanInteraction.interruptId,
        response,
      });
      const detail = await fetchTaskDetail({
        id: taskDetailId,
        spaceId: spaceID,
        source: taskDetailSource,
      });
      applyTaskDetail(detail);
    } catch (err) {
      setHumanInteractionError(
        err instanceof Error ? err.message : '提交失败，请稍后重试',
      );
    } finally {
      setHumanInteractionLoading(false);
    }
  };

  return {
    followUpError,
    followUpLoading,
    followUpMode,
    followUpValue,
    handleFollowUpSubmit,
    handleHumanInteractionSubmit,
    humanInteractionError,
    humanInteractionLoading,
    setFollowUpMode,
    setFollowUpValue,
    ...taskRunActions,
  };
};
