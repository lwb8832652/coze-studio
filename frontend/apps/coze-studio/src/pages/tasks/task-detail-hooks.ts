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

import { useCallback, useEffect, useState } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';

import type {
  WorkbenchComposerSubmitPayload,
  WorkbenchMode,
} from '../workbench/components/types';
import { useTaskThreadRunEventStream } from './task-run-event-stream';
import { useTaskRunActions } from './task-run-actions-hook';
import type { PendingHumanInteraction } from './task-human-interaction';
import { sendFollowUpMessage } from './task-follow-up';
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

export const useTaskDetailData = ({
  taskDetailId,
  taskDetailSource,
}: {
  taskDetailId?: string;
  taskDetailSource: TaskDetailSource;
}) => {
  const [task, setTask] = useState<ChatTask | undefined>();
  const [events, setEvents] = useState<TaskEvent[]>([]);
  const [messages, setMessages] = useState<TaskThreadMessage[]>([]);
  const [artifacts, setArtifacts] = useState<
    workbenchTask.TaskThreadArtifact[]
  >([]);
  const [latestTaskRunID, setLatestTaskRunID] = useState('');
  const [subagentRuns, setSubagentRuns] = useState<TaskDetailSubagentRun[]>([]);
  const [tokenUsage, setTokenUsage] = useState<TaskDetailTokenUsage>();
  const [loadedTaskDetailSource, setLoadedTaskDetailSource] =
    useState<LoadedTaskDetailSource>(getInitialLoadedSource(taskDetailSource));
  const [loadedThreadId, setLoadedThreadId] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [pollingVersion, setPollingVersion] = useState(0);
  const applyTaskDetail = useCallback((detail: TaskDetail) => {
    setLoadedTaskDetailSource(detail.source);
    setLoadedThreadId(detail.threadId ?? '');
    setTask(detail.task);
    setEvents(detail.events);
    setMessages(detail.messages ?? []);
    setArtifacts(detail.artifacts ?? []);
    setLatestTaskRunID(detail.latestTaskRunID ?? '');
    setSubagentRuns(detail.subagentRuns ?? []);
    setTokenUsage(detail.tokenUsage);
    if (shouldPollTaskDetail(detail)) {
      setPollingVersion(version => version + 1);
    }
  }, []);
  const refreshArtifacts = useCallback(async () => {
    if (!loadedThreadId || loadedTaskDetailSource !== 'thread') {
      return;
    }

    const response = await listTaskThreadArtifacts({
      thread_id: loadedThreadId,
      page: 1,
      page_size: 50,
    });
    setArtifacts(response.data?.artifacts ?? []);
  }, [loadedTaskDetailSource, loadedThreadId]);

  useEffect(() => {
    if (!taskDetailId) {
      return;
    }
    let canceled = false;
    const loadTaskDetail = async (showLoading = false) => {
      if (showLoading) {
        setLoading(true);
        setLoadedTaskDetailSource(getInitialLoadedSource(taskDetailSource));
        setLoadedThreadId(taskDetailSource === 'thread' ? taskDetailId : '');
      }
      setError('');
      try {
        const detail = await fetchTaskDetail({
          id: taskDetailId,
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
    void loadTaskDetail(true);
    return () => {
      canceled = true;
    };
  }, [applyTaskDetail, taskDetailId, taskDetailSource]);

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
  }, [applyTaskDetail, pollingVersion, taskDetailId, taskDetailSource]);

  useTaskThreadRunEventStream({
    enabled: loadedTaskDetailSource === 'thread' && Boolean(loadedThreadId),
    setEvents,
    threadId: loadedThreadId,
  });

  return {
    applyTaskDetail,
    artifacts,
    error,
    events,
    messages,
    latestTaskRunID,
    loadedTaskDetailSource,
    loadedThreadId,
    loading,
    refreshArtifacts,
    subagentRuns,
    task,
    tokenUsage,
  };
};

export const useTaskDetailActions = ({
  applyTaskDetail,
  pendingHumanInteraction,
  spaceID,
  task,
  taskDetailId,
  taskDetailSource,
}: {
  applyTaskDetail: (detail: TaskDetail) => void;
  pendingHumanInteraction?: PendingHumanInteraction;
  spaceID?: string;
  task?: ChatTask;
  taskDetailId?: string;
  taskDetailSource: LoadedTaskDetailSource;
}) => {
  const [followUpValue, setFollowUpValue] = useState('');
  const [followUpMode, setFollowUpMode] = useState<WorkbenchMode>('Auto');
  const [followUpLoading, setFollowUpLoading] = useState(false);
  const [followUpError, setFollowUpError] = useState('');
  const [humanInteractionLoading, setHumanInteractionLoading] = useState(false);
  const [humanInteractionError, setHumanInteractionError] = useState('');
  const taskRunActions = useTaskRunActions({
    applyTaskDetail,
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
      await sendFollowUpMessage({
        activeTaskId,
        isCanonicalThreadDetail,
        payload,
        spaceId: spaceID,
        threadId: taskDetailId,
      });

      setFollowUpValue('');
      const detail = await fetchTaskDetail({
        id: taskDetailId,
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
