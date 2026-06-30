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

import { useState } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';

import {
  createDefaultWorkbenchResourceSelection,
  createDefaultWorkbenchRuntimeSettings,
  DEFAULT_WORKBENCH_MODE,
  stringifyWorkbenchRunConfig,
  type WorkbenchComposerSubmitPayload,
} from '../workbench/components/types';
import type { TaskRunActionLoading } from './task-run-action-bar';
import {
  fetchTaskDetail,
  type LoadedTaskDetailSource,
  type TaskDetail,
} from './task-detail-loader';
import {
  cancelTaskThreadRun,
  createTaskThreadRun,
  retryTaskThreadSubagentRun,
} from './service';
import { getTaskInputText } from './helpers';

type ChatTask = workbenchTask.ChatTask;

const getTaskRetryPayload = (
  message: string,
): WorkbenchComposerSubmitPayload => {
  const resourceSelection = createDefaultWorkbenchResourceSelection();

  return {
    message,
    mode: DEFAULT_WORKBENCH_MODE,
    runtimeSettings: createDefaultWorkbenchRuntimeSettings(resourceSelection),
    ...resourceSelection,
  };
};

const getTaskRetryMetadata = ({
  sourceRunId,
  taskId,
}: {
  sourceRunId: string;
  taskId: string;
}) =>
  JSON.stringify({
    source: 'task_retry',
    source_run_id: sourceRunId,
    source_task_id: taskId,
    requested_at: Date.now(),
  });

export const useTaskRunActions = ({
  applyTaskDetail,
  spaceID,
  task,
  taskDetailId,
  taskDetailSource,
}: {
  applyTaskDetail: (detail: TaskDetail) => void;
  spaceID?: string;
  task?: ChatTask;
  taskDetailId?: string;
  taskDetailSource: LoadedTaskDetailSource;
}) => {
  const [taskRunActionLoading, setTaskRunActionLoading] =
    useState<TaskRunActionLoading>('');
  const [taskRunActionError, setTaskRunActionError] = useState('');
  const [retryingSubagentRunId, setRetryingSubagentRunId] = useState('');
  const [subagentRetryError, setSubagentRetryError] = useState('');

  const handleCancelTaskRun = async (runId: string) => {
    if (!runId || taskRunActionLoading) {
      return;
    }
    if (!taskDetailId || taskDetailSource !== 'thread') {
      setTaskRunActionError('缺少任务运行上下文，请刷新后重试');
      return;
    }

    setTaskRunActionLoading('cancel');
    setTaskRunActionError('');
    try {
      await cancelTaskThreadRun({
        thread_id: taskDetailId,
        run_id: runId,
      });
      const detail = await fetchTaskDetail({
        id: taskDetailId,
        spaceId: spaceID,
        source: taskDetailSource,
      });
      applyTaskDetail(detail);
    } catch (err) {
      setTaskRunActionError(
        err instanceof Error ? err.message : '取消任务失败，请稍后再试',
      );
    } finally {
      setTaskRunActionLoading('');
    }
  };

  const handleRetryTaskRun = async (sourceRunId: string) => {
    if (!sourceRunId || taskRunActionLoading) {
      return;
    }
    if (!taskDetailId || taskDetailSource !== 'thread' || !task) {
      setTaskRunActionError('缺少任务重试上下文，请刷新后重试');
      return;
    }

    const message = getTaskInputText(task.input) || task.title;
    const retryPayload = getTaskRetryPayload(message);

    setTaskRunActionLoading('retry');
    setTaskRunActionError('');
    try {
      await createTaskThreadRun({
        thread_id: taskDetailId,
        input: JSON.stringify({
          messages: [
            {
              role: 'user',
              content: message,
            },
          ],
        }),
        config: stringifyWorkbenchRunConfig(retryPayload),
        metadata: getTaskRetryMetadata({
          sourceRunId,
          taskId: task.id,
        }),
        idempotency_key: `${taskDetailId}:${sourceRunId}:task_retry`,
      });
      const detail = await fetchTaskDetail({
        id: taskDetailId,
        spaceId: spaceID,
        source: taskDetailSource,
      });
      applyTaskDetail(detail);
    } catch (err) {
      setTaskRunActionError(
        err instanceof Error ? err.message : '重试任务失败，请稍后再试',
      );
    } finally {
      setTaskRunActionLoading('');
    }
  };

  const handleRetrySubagentRun = async (runId: string) => {
    if (!runId || retryingSubagentRunId) {
      return;
    }
    if (!taskDetailId || taskDetailSource !== 'thread') {
      setSubagentRetryError('缺少任务恢复上下文，请刷新后重试');
      return;
    }

    setRetryingSubagentRunId(runId);
    setSubagentRetryError('');
    try {
      await retryTaskThreadSubagentRun({
        thread_id: taskDetailId,
        run_id: runId,
      });
      const detail = await fetchTaskDetail({
        id: taskDetailId,
        spaceId: spaceID,
        source: taskDetailSource,
      });
      applyTaskDetail(detail);
    } catch (err) {
      setSubagentRetryError(
        err instanceof Error ? err.message : '重试子智能体失败，请稍后再试',
      );
    } finally {
      setRetryingSubagentRunId('');
    }
  };

  return {
    handleCancelTaskRun,
    handleRetryTaskRun,
    handleRetrySubagentRun,
    retryingSubagentRunId,
    subagentRetryError,
    taskRunActionError,
    taskRunActionLoading,
  };
};
