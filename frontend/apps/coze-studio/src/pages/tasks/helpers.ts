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

import {
  getTaskReasoningContent,
  stripTaskThinkingTags,
} from './task-reasoning';
export { getTaskEventDisplay, getTaskEventText } from './task-event-display';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;
type TaskThread = workbenchTask.TaskThread;

export type TaskStatusFilter = 'all' | 'running' | 'succeeded' | 'failed';
export type TaskExecutionType = 'Ark' | 'Agent';
export type TaskResultType = 'answer' | 'agent_trace' | 'report';
export type TaskExecutionStatus =
  | 'completed'
  | 'running'
  | 'failed'
  | 'pending'
  | 'neutral';

export interface TaskEventDisplay {
  title: string;
  detail?: string;
  thought?: string;
  status: TaskExecutionStatus;
  runtime?: TaskExecutionType;
  progress?: number;
  structured: boolean;
  kind: 'step' | 'thought' | 'event';
}

export interface TaskResultPayload {
  message: string;
  reasoning?: string;
  resultType: TaskResultType;
  executionType?: TaskExecutionType;
  retrievalSources: string[];
}

export const getTaskThreadDetailId = (
  task: Pick<TaskThread, 'legacy_task_id' | 'thread_id'>,
) => {
  const legacyTaskID = task.legacy_task_id?.trim();

  return legacyTaskID && legacyTaskID !== '0' ? legacyTaskID : task.thread_id;
};

const parseJSONObject = (
  value?: string,
): Record<string, unknown> | undefined => {
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

const getString = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
};

const normalizeExecutionType = (
  value?: string,
): TaskExecutionType | undefined => {
  if (value === 'Ark' || value === 'Agent') {
    return value;
  }

  return undefined;
};

const normalizeResultType = (value?: string): TaskResultType => {
  switch (value) {
    case 'agent_trace':
      return 'agent_trace';
    case 'report':
      return 'report';
    case 'answer':
    default:
      return 'answer';
  }
};

const getStringArray = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  if (!Array.isArray(value)) {
    return [];
  }

  return value.filter(
    (item): item is string => typeof item === 'string' && Boolean(item.trim()),
  );
};

export const getTaskInputText = (input?: string) => getPayloadText(input);

export const parseTaskResultPayload = (result?: string): TaskResultPayload => {
  const parsed = parseJSONObject(result);
  const rawMessage = getPayloadText(result);
  const reasoning = getTaskReasoningContent(parsed, rawMessage);

  return {
    message: stripTaskThinkingTags(rawMessage),
    reasoning: reasoning || undefined,
    resultType: normalizeResultType(getString(parsed, 'result_type')),
    executionType: normalizeExecutionType(getString(parsed, 'execution_type')),
    retrievalSources: getStringArray(parsed, 'retrieval_sources'),
  };
};

export const getTaskResultText = (result?: string) =>
  parseTaskResultPayload(result).message;

export const getTaskExecutionType = (input?: string): TaskExecutionType => {
  const parsed = parseJSONObject(input);

  return (
    normalizeExecutionType(getString(parsed, 'execution_type')) ??
    normalizeExecutionType(getString(parsed, 'executionType')) ??
    normalizeExecutionType(getString(parsed, 'runtime')) ??
    'Agent'
  );
};

export const getLatestAnswerEventMessage = (events: TaskEvent[]) => {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];

    if (
      event.event_type !== 'answer.delta' &&
      event.event_type !== 'answer.completed' &&
      event.event_type !== 'message.completed'
    ) {
      continue;
    }

    const parsed = parseJSONObject(event.payload);
    const role = getString(parsed, 'role');

    if (
      event.event_type === 'message.completed' &&
      role &&
      role !== 'assistant'
    ) {
      continue;
    }

    const message =
      getString(parsed, 'message') ?? getString(parsed, 'content');

    if (message) {
      return stripTaskThinkingTags(message);
    }
  }

  return '';
};

export const getLatestAnswerEventReasoning = (events: TaskEvent[]) => {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];

    if (
      event.event_type !== 'answer.delta' &&
      event.event_type !== 'answer.completed' &&
      event.event_type !== 'message.completed'
    ) {
      continue;
    }

    const parsed = parseJSONObject(event.payload);
    const role = getString(parsed, 'role');

    if (
      event.event_type === 'message.completed' &&
      role &&
      role !== 'assistant'
    ) {
      continue;
    }

    const reasoning = getTaskReasoningContent(
      parsed,
      getString(parsed, 'message') ?? getString(parsed, 'content') ?? '',
    );

    if (reasoning) {
      return reasoning;
    }
  }

  return '';
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

export const isTaskTerminalStatus = (status: workbenchTask.TaskStatus) =>
  status === workbenchTask.TaskStatus.Succeeded ||
  status === workbenchTask.TaskStatus.Failed ||
  status === workbenchTask.TaskStatus.Canceled;

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
