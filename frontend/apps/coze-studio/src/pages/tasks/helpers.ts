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

import { workbenchTask } from '@coze-studio/api-schema';

type ChatTask = workbenchTask.ChatTask;

export type TaskStatusFilter = 'all' | 'running' | 'succeeded' | 'failed';

const STATUS_TEXT_BY_KEY: Record<string, string> = {
  created: '任务已创建',
  queued: '任务已进入队列',
  running: '任务运行中',
  succeeded: '任务已完成',
  failed: '任务失败',
  canceling: '任务取消中',
  canceled: '任务已取消',
};

const parseJSONObject = (value?: string): Record<string, unknown> | undefined => {
  const trimmed = value?.trim();

  if (!trimmed) {
    return undefined;
  }

  try {
    const parsed: unknown = JSON.parse(trimmed);

    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return undefined;
  }

  return undefined;
};

const getPayloadText = (value?: string) => {
  const parsed = parseJSONObject(value);

  if (typeof parsed?.message === 'string') {
    return parsed.message;
  }

  if (typeof parsed?.text === 'string') {
    return parsed.text;
  }

  return value?.trim() ?? '';
};

export const getTaskInputText = (input?: string) => getPayloadText(input);

export const getTaskResultText = (result?: string) => getPayloadText(result);

export const getTaskEventText = (eventType?: string, payload?: string) => {
  const parsed = parseJSONObject(payload);

  if (typeof parsed?.message === 'string') {
    return parsed.message;
  }

  if (typeof parsed?.status === 'string') {
    return STATUS_TEXT_BY_KEY[parsed.status] ?? parsed.status;
  }

  if (typeof parsed?.to === 'string') {
    return STATUS_TEXT_BY_KEY[parsed.to] ?? `状态更新为 ${parsed.to}`;
  }

  return payload?.trim() || eventType || '任务事件';
};

export const getTaskStatusText = (status: workbenchTask.TaskStatus) => {
  const statusMap: Record<workbenchTask.TaskStatus, string> = {
    [workbenchTask.TaskStatus.Created]: '已创建',
    [workbenchTask.TaskStatus.Queued]: '排队中',
    [workbenchTask.TaskStatus.Running]: '运行中',
    [workbenchTask.TaskStatus.Succeeded]: '已完成',
    [workbenchTask.TaskStatus.Failed]: '失败',
    [workbenchTask.TaskStatus.Canceling]: '取消中',
    [workbenchTask.TaskStatus.Canceled]: '已取消',
  };

  return statusMap[status] ?? '未知';
};

export const getTaskStatusTone = (status: workbenchTask.TaskStatus) => {
  if (
    status === workbenchTask.TaskStatus.Running ||
    status === workbenchTask.TaskStatus.Queued
  ) {
    return 'running';
  }

  if (status === workbenchTask.TaskStatus.Succeeded) {
    return 'success';
  }

  if (
    status === workbenchTask.TaskStatus.Failed ||
    status === workbenchTask.TaskStatus.Canceled
  ) {
    return 'danger';
  }

  return 'neutral';
};

export const canCancelTask = (status: workbenchTask.TaskStatus) =>
  status === workbenchTask.TaskStatus.Created ||
  status === workbenchTask.TaskStatus.Queued ||
  status === workbenchTask.TaskStatus.Running;

export const canRetryTask = (status: workbenchTask.TaskStatus) =>
  status === workbenchTask.TaskStatus.Failed;

export const formatUpdatedTime = (timestamp: number) => {
  if (!timestamp) {
    return '-';
  }

  return new Date(timestamp).toLocaleString();
};

export const filterTasks = (
  tasks: ChatTask[],
  keyword: string,
  statusFilter: TaskStatusFilter,
) => {
  const normalizedKeyword = keyword.trim().toLowerCase();

  return tasks.filter(task => {
    const readableInput = getTaskInputText(task.input).toLowerCase();
    const matchesKeyword = normalizedKeyword
      ? task.title.toLowerCase().includes(normalizedKeyword) ||
        readableInput.includes(normalizedKeyword)
      : true;
    const matchesStatus =
      statusFilter === 'all' ||
      (statusFilter === 'running' &&
        [
          workbenchTask.TaskStatus.Created,
          workbenchTask.TaskStatus.Queued,
          workbenchTask.TaskStatus.Running,
        ].includes(task.status)) ||
      (statusFilter === 'succeeded' &&
        task.status === workbenchTask.TaskStatus.Succeeded) ||
      (statusFilter === 'failed' &&
        [
          workbenchTask.TaskStatus.Failed,
          workbenchTask.TaskStatus.Canceled,
        ].includes(task.status));

    return matchesKeyword && matchesStatus;
  });
};
