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

/* eslint-disable @coze-arch/max-line-per-function -- Run actions keep shared confirmation and cancellation state in one hook. */

import { useEffect, useLayoutEffect, useRef, useState } from 'react';

import type { WorkbenchRun } from '../workbench/thread-client';
import {
  createDefaultWorkbenchResourceSelection,
  createDefaultWorkbenchRuntimeSettings,
  DEFAULT_WORKBENCH_MODE,
  stringifyWorkbenchRunConfig,
  type WorkbenchComposerSubmitPayload,
} from '../workbench/components/types';
import type { TaskThreadDetailModel } from './task-thread-detail-model';
import type { TaskRunActionLoading } from './task-run-action-bar';
import { fetchTaskDetail, type TaskDetail } from './task-detail-loader';
import {
  cancelTaskThreadRun,
  createTaskThreadRun,
  retryTaskThreadSubagentRun,
} from './service';
import { getTaskInputText } from './helpers';

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
  threadId,
}: {
  sourceRunId: string;
  threadId: string;
}) =>
  JSON.stringify({
    source: 'task_retry',
    source_run_id: sourceRunId,
    source_thread_id: threadId,
    requested_at: Date.now(),
  });

const isTopLevelRun = (run: WorkbenchRun): boolean =>
  !run.parent_run_id || run.parent_run_id === '0';

const createTaskRunScopeKey = (spaceID?: string, taskDetailId?: string) =>
  JSON.stringify([spaceID ?? '', taskDetailId ?? '']);

export const useTaskRunActions = <TaskRequestToken>({
  applyTaskDetail,
  captureTaskDetailRequestToken,
  commitTopLevelRun,
  spaceID,
  task,
  taskDetailId,
}: {
  applyTaskDetail: (
    detail: TaskDetail,
    taskDetailId?: string,
    requestToken?: TaskRequestToken,
  ) => void;
  captureTaskDetailRequestToken?: () => TaskRequestToken;
  commitTopLevelRun?: (run: WorkbenchRun) => void;
  spaceID?: string;
  task?: TaskThreadDetailModel;
  taskDetailId?: string;
}) => {
  const [taskRunActionLoading, setTaskRunActionLoading] =
    useState<TaskRunActionLoading>('');
  const [taskRunActionError, setTaskRunActionError] = useState('');
  const [retryingSubagentRunId, setRetryingSubagentRunId] = useState('');
  const [subagentRetryError, setSubagentRetryError] = useState('');
  const mountedRef = useRef(true);
  const taskScopeKey = createTaskRunScopeKey(spaceID, taskDetailId);
  const scopedTaskScopeKeyRef = useRef(taskScopeKey);
  const taskRequestGenerationRef = useRef(0);
  const actionGenerationRef = useRef(0);
  const operationGenerationRef = useRef({
    cancel: 0,
    retry: 0,
    subagent: 0,
  });
  const activeOperationRef = useRef<'' | 'cancel' | 'retry' | 'subagent'>('');
  const successfulMutationKeysRef = useRef(new Set<string>());
  const [taskRunActionsDisabled, setTaskRunActionsDisabled] = useState(false);

  useLayoutEffect(() => {
    if (scopedTaskScopeKeyRef.current === taskScopeKey) {
      return;
    }

    scopedTaskScopeKeyRef.current = taskScopeKey;
    taskRequestGenerationRef.current += 1;
    actionGenerationRef.current += 1;
    operationGenerationRef.current.cancel += 1;
    operationGenerationRef.current.retry += 1;
    operationGenerationRef.current.subagent += 1;
    activeOperationRef.current = '';
    successfulMutationKeysRef.current.clear();
  }, [taskScopeKey]);

  useEffect(() => {
    mountedRef.current = true;

    return () => {
      mountedRef.current = false;
      taskRequestGenerationRef.current += 1;
      actionGenerationRef.current += 1;
    };
  }, []);

  useEffect(() => {
    setTaskRunActionLoading('');
    setTaskRunActionError('');
    setRetryingSubagentRunId('');
    setSubagentRetryError('');
    setTaskRunActionsDisabled(false);
    activeOperationRef.current = '';
    successfulMutationKeysRef.current.clear();
  }, [taskScopeKey]);

  const captureTaskRequest = (
    submittedTaskScopeKey: string,
    operation: 'cancel' | 'retry' | 'subagent',
  ) => ({
    actionGeneration: ++actionGenerationRef.current,
    generation: taskRequestGenerationRef.current,
    operation,
    operationGeneration: ++operationGenerationRef.current[operation],
    taskScopeKey: submittedTaskScopeKey,
  });
  const isCurrentTaskRequest = ({
    actionGeneration,
    generation,
    operation,
    operationGeneration,
    taskScopeKey: submittedTaskScopeKey,
  }: {
    actionGeneration: number;
    generation: number;
    operation: 'cancel' | 'retry' | 'subagent';
    operationGeneration: number;
    taskScopeKey: string;
  }) =>
    mountedRef.current &&
    actionGenerationRef.current === actionGeneration &&
    taskRequestGenerationRef.current === generation &&
    operationGenerationRef.current[operation] === operationGeneration &&
    scopedTaskScopeKeyRef.current === submittedTaskScopeKey;

  const handleCancelTaskRun = async (runId: string) => {
    if (!runId || activeOperationRef.current) {
      return;
    }
    if (!spaceID || !taskDetailId) {
      setTaskRunActionError('缺少任务运行上下文，请刷新后重试');
      return;
    }
    const submittedTaskDetailId = taskDetailId;
    const submittedSpaceID = spaceID;
    const submittedTaskScopeKey = taskScopeKey;
    const mutationKey = `cancel:${submittedTaskScopeKey}:${runId}`;
    const request = captureTaskRequest(submittedTaskScopeKey, 'cancel');

    activeOperationRef.current = 'cancel';
    setTaskRunActionsDisabled(true);
    setTaskRunActionLoading('cancel');
    setTaskRunActionError('');
    try {
      if (!successfulMutationKeysRef.current.has(mutationKey)) {
        try {
          await cancelTaskThreadRun({
            thread_id: submittedTaskDetailId,
            run_id: runId,
            space_id: submittedSpaceID,
          });
        } catch (err) {
          if (isCurrentTaskRequest(request)) {
            setTaskRunActionError(
              err instanceof Error ? err.message : '取消任务失败，请稍后再试',
            );
          }
          return;
        }
        if (!isCurrentTaskRequest(request)) {
          return;
        }
        successfulMutationKeysRef.current.add(mutationKey);
      }

      const refreshRequestToken = captureTaskDetailRequestToken?.();
      try {
        const detail = await fetchTaskDetail({
          id: submittedTaskDetailId,
          spaceId: submittedSpaceID,
        });
        if (isCurrentTaskRequest(request)) {
          applyTaskDetail(detail, submittedTaskDetailId, refreshRequestToken);
        }
      } catch {
        if (isCurrentTaskRequest(request)) {
          setTaskRunActionError('操作已成功但刷新失败，可重试刷新');
        }
      }
    } finally {
      if (isCurrentTaskRequest(request)) {
        activeOperationRef.current = '';
        setTaskRunActionsDisabled(false);
        setTaskRunActionLoading('');
      }
    }
  };

  const handleRetryTaskRun = async (sourceRunId: string) => {
    if (!sourceRunId || activeOperationRef.current) {
      return;
    }
    if (!spaceID || !taskDetailId || !task) {
      setTaskRunActionError('缺少任务重试上下文，请刷新后重试');
      return;
    }
    const submittedTaskDetailId = taskDetailId;
    const submittedSpaceID = spaceID;
    const submittedTaskScopeKey = taskScopeKey;
    const mutationKey = `retry:${submittedTaskScopeKey}:${sourceRunId}`;
    const request = captureTaskRequest(submittedTaskScopeKey, 'retry');

    const message = getTaskInputText(task.input) || task.title;
    const retryPayload = getTaskRetryPayload(message);

    activeOperationRef.current = 'retry';
    setTaskRunActionsDisabled(true);
    setTaskRunActionLoading('retry');
    setTaskRunActionError('');
    try {
      if (!successfulMutationKeysRef.current.has(mutationKey)) {
        try {
          const response = await createTaskThreadRun({
            thread_id: submittedTaskDetailId,
            space_id: submittedSpaceID,
            attempt_kind: 'retry',
            message_content: message,
            source_run_id: sourceRunId,
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
              threadId: task.id,
            }),
            idempotency_key: `${submittedSpaceID}:${submittedTaskDetailId}:${sourceRunId}:task_retry`,
          });
          if (!isCurrentTaskRequest(request)) {
            return;
          }
          if (response.data && isTopLevelRun(response.data)) {
            commitTopLevelRun?.(response.data);
          }
        } catch (err) {
          if (isCurrentTaskRequest(request)) {
            setTaskRunActionError(
              err instanceof Error ? err.message : '重试任务失败，请稍后再试',
            );
          }
          return;
        }
        if (!isCurrentTaskRequest(request)) {
          return;
        }
        successfulMutationKeysRef.current.add(mutationKey);
      }

      const refreshRequestToken = captureTaskDetailRequestToken?.();
      try {
        const detail = await fetchTaskDetail({
          id: submittedTaskDetailId,
          spaceId: submittedSpaceID,
        });
        if (isCurrentTaskRequest(request)) {
          applyTaskDetail(detail, submittedTaskDetailId, refreshRequestToken);
        }
      } catch {
        if (isCurrentTaskRequest(request)) {
          setTaskRunActionError('操作已成功但刷新失败，可重试刷新');
        }
      }
    } finally {
      if (isCurrentTaskRequest(request)) {
        activeOperationRef.current = '';
        setTaskRunActionsDisabled(false);
        setTaskRunActionLoading('');
      }
    }
  };

  const handleRetrySubagentRun = async (runId: string) => {
    if (!runId || activeOperationRef.current) {
      return;
    }
    if (!spaceID || !taskDetailId) {
      setSubagentRetryError('缺少任务恢复上下文，请刷新后重试');
      return;
    }
    const submittedTaskDetailId = taskDetailId;
    const submittedSpaceID = spaceID;
    const submittedTaskScopeKey = taskScopeKey;
    const mutationKey = `subagent:${submittedTaskScopeKey}:${runId}`;
    const request = captureTaskRequest(submittedTaskScopeKey, 'subagent');

    activeOperationRef.current = 'subagent';
    setTaskRunActionsDisabled(true);
    setRetryingSubagentRunId(runId);
    setSubagentRetryError('');
    try {
      if (!successfulMutationKeysRef.current.has(mutationKey)) {
        try {
          await retryTaskThreadSubagentRun({
            thread_id: submittedTaskDetailId,
            run_id: runId,
            space_id: submittedSpaceID,
          });
          if (!isCurrentTaskRequest(request)) {
            return;
          }
        } catch (err) {
          if (isCurrentTaskRequest(request)) {
            setSubagentRetryError(
              err instanceof Error
                ? err.message
                : '重试子智能体失败，请稍后再试',
            );
          }
          return;
        }
        if (!isCurrentTaskRequest(request)) {
          return;
        }
        successfulMutationKeysRef.current.add(mutationKey);
      }

      const refreshRequestToken = captureTaskDetailRequestToken?.();
      try {
        const detail = await fetchTaskDetail({
          id: submittedTaskDetailId,
          spaceId: submittedSpaceID,
        });
        if (isCurrentTaskRequest(request)) {
          applyTaskDetail(detail, submittedTaskDetailId, refreshRequestToken);
        }
      } catch {
        if (isCurrentTaskRequest(request)) {
          setSubagentRetryError('操作已成功但刷新失败，可重试刷新');
        }
      }
    } finally {
      if (isCurrentTaskRequest(request)) {
        activeOperationRef.current = '';
        setTaskRunActionsDisabled(false);
        setRetryingSubagentRunId('');
      }
    }
  };

  return {
    handleCancelTaskRun,
    handleRetryTaskRun,
    handleRetrySubagentRun,
    retryingSubagentRunId,
    subagentRetryError,
    taskRunActionsDisabled,
    taskRunActionError,
    taskRunActionLoading,
  };
};
