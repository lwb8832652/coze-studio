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

import {
  TaskThreadDetailStatus,
  type TaskThreadDetailEvent,
  type TaskThreadDetailModel,
} from './task-thread-detail-model';
import {
  getTaskReasoningContent,
  stripTaskThinkingTags,
} from './task-reasoning';
export {
  getTaskThreadEventDisplay,
  getTaskThreadEventText,
} from './task-event-display';

export type TaskStatusFilter = 'all' | 'running' | 'succeeded' | 'failed';
export type TaskExecutionType = 'Ark' | 'Agent';
export type TaskResultType = 'answer' | 'agent_trace' | 'report';
export type TaskExecutionStatus =
  | 'completed'
  | 'running'
  | 'failed'
  | 'pending'
  | 'neutral';

export interface TaskThreadEventDisplay {
  title: string;
  detail?: string;
  thought?: string;
  status: TaskExecutionStatus;
  runtime?: TaskExecutionType;
  progress?: number;
  structured: boolean;
  visibleInFlow?: boolean;
  kind: 'step' | 'thought' | 'event';
}

export interface TaskResultPayload {
  message: string;
  reasoning?: string;
  resultType: TaskResultType;
  executionType?: TaskExecutionType;
  retrievalSources: string[];
}

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

export const getLatestAnswerEventMessage = (
  events: TaskThreadDetailEvent[],
) => {
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

export const getLatestAnswerEventReasoning = (
  events: TaskThreadDetailEvent[],
) => {
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

export const getTaskStatusText = (status: TaskThreadDetailStatus) => {
  const statusMap: Record<TaskThreadDetailStatus, string> = {
    [TaskThreadDetailStatus.Created]: '已创建',
    [TaskThreadDetailStatus.Queued]: '排队中',
    [TaskThreadDetailStatus.Running]: '运行中',
    [TaskThreadDetailStatus.Succeeded]: '已完成',
    [TaskThreadDetailStatus.Failed]: '失败',
    [TaskThreadDetailStatus.Canceling]: '取消中',
    [TaskThreadDetailStatus.Canceled]: '已取消',
  };

  return statusMap[status] ?? '未知';
};

export const getTaskStatusTone = (status: TaskThreadDetailStatus) => {
  if (
    status === TaskThreadDetailStatus.Running ||
    status === TaskThreadDetailStatus.Queued
  ) {
    return 'running';
  }

  if (status === TaskThreadDetailStatus.Succeeded) {
    return 'success';
  }

  if (
    status === TaskThreadDetailStatus.Failed ||
    status === TaskThreadDetailStatus.Canceled
  ) {
    return 'danger';
  }

  return 'neutral';
};

export const canCancelTask = (status: TaskThreadDetailStatus) =>
  status === TaskThreadDetailStatus.Created ||
  status === TaskThreadDetailStatus.Queued ||
  status === TaskThreadDetailStatus.Running;

export const canRetryTask = (status: TaskThreadDetailStatus) =>
  status === TaskThreadDetailStatus.Failed;

export const isTaskTerminalStatus = (status: TaskThreadDetailStatus) =>
  status === TaskThreadDetailStatus.Succeeded ||
  status === TaskThreadDetailStatus.Failed ||
  status === TaskThreadDetailStatus.Canceled;

export const formatUpdatedTime = (timestamp: number) => {
  if (!timestamp) {
    return '-';
  }

  return new Date(timestamp).toLocaleString();
};

const MILLISECONDS_PER_MINUTE = 60_000;
const MINUTES_PER_HOUR = 60;
const HOURS_PER_DAY = 24;
const DAYS_PER_MONTH = 30;
const DAYS_PER_YEAR = 365;

export const formatTaskListTime = (
  timestamp: number,
  now: number = Date.now(),
) => {
  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    return '-';
  }

  const minute = MILLISECONDS_PER_MINUTE;
  const hour = MINUTES_PER_HOUR * minute;
  const day = HOURS_PER_DAY * hour;
  const elapsed = Math.max(0, now - timestamp);

  if (elapsed < minute) {
    return '刚刚';
  }

  if (elapsed < hour) {
    return `${Math.floor(elapsed / minute)}分钟前`;
  }

  if (elapsed < day) {
    return `${Math.floor(elapsed / hour)}小时前`;
  }

  const elapsedDays = Math.floor(elapsed / day);

  if (elapsedDays < DAYS_PER_MONTH) {
    return `${elapsedDays}天前`;
  }

  if (elapsedDays < DAYS_PER_YEAR) {
    return `${Math.max(1, Math.floor(elapsedDays / DAYS_PER_MONTH))}个月前`;
  }

  return `${Math.max(1, Math.floor(elapsedDays / DAYS_PER_YEAR))}年前`;
};

export const filterTasks = (
  tasks: TaskThreadDetailModel[],
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
          TaskThreadDetailStatus.Created,
          TaskThreadDetailStatus.Queued,
          TaskThreadDetailStatus.Running,
        ].includes(task.status)) ||
      (statusFilter === 'succeeded' &&
        task.status === TaskThreadDetailStatus.Succeeded) ||
      (statusFilter === 'failed' &&
        [
          TaskThreadDetailStatus.Failed,
          TaskThreadDetailStatus.Canceled,
        ].includes(task.status));

    return matchesKeyword && matchesStatus;
  });
};
