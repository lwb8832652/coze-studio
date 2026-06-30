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

import { getTaskToolEventDisplay } from './task-event-tool-display';
import type {
  TaskEventDisplay,
  TaskExecutionStatus,
  TaskExecutionType,
} from './helpers';

const STATUS_TEXT_BY_KEY: Record<string, string> = {
  created: '任务已创建',
  queued: '任务已进入队列',
  running: '任务运行中',
  succeeded: '任务已完成',
  failed: '任务失败',
  canceling: '任务取消中',
  canceled: '任务已取消',
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

const getString = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
};

const getNumber = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return typeof value === 'number' && Number.isFinite(value)
    ? value
    : undefined;
};

const getStringArray = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return Array.isArray(value)
    ? value
        .map(item => (typeof item === 'string' ? item.trim() : ''))
        .filter(Boolean)
    : [];
};

const getBoolean = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return typeof value === 'boolean' ? value : undefined;
};

const normalizeExecutionType = (
  value?: string,
): TaskExecutionType | undefined => {
  if (value === 'Ark' || value === 'Agent') {
    return value;
  }

  return undefined;
};

const normalizeExecutionStatus = (value?: string): TaskExecutionStatus => {
  switch (value) {
    case 'completed':
    case 'succeeded':
    case 'success':
    case 'done':
      return 'completed';
    case 'running':
    case 'processing':
      return 'running';
    case 'failed':
    case 'error':
      return 'failed';
    case 'pending':
    case 'queued':
    case 'created':
      return 'pending';
    default:
      return 'neutral';
  }
};

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

interface EventDisplayContext {
  eventType?: string;
  payload?: string;
  parsed?: Record<string, unknown>;
  title?: string;
  detail?: string;
  thought?: string;
  runtime?: TaskExecutionType;
  progress?: number;
  status: TaskExecutionStatus;
  structured: boolean;
}

const getPlanEventDisplay = ({
  eventType,
  parsed,
}: EventDisplayContext): TaskEventDisplay | undefined => {
  if (!eventType?.startsWith('plan.task.')) {
    return undefined;
  }

  const subject = getString(parsed, 'subject') ?? '未命名计划项';
  const status = normalizeExecutionStatus(getString(parsed, 'status'));
  const activeForm = getString(parsed, 'active_form');
  const owner = getString(parsed, 'owner');
  const detail = [activeForm, owner ? `负责人：${owner}` : undefined]
    .filter(Boolean)
    .join(' · ');
  const titleMap: Record<string, string> = {
    'plan.task.created': `计划：${subject}`,
    'plan.task.updated':
      status === 'running' ? `执行计划：${subject}` : `更新计划：${subject}`,
    'plan.task.completed': `完成计划：${subject}`,
    'plan.task.deleted': `移除计划：${subject}`,
  };

  return {
    title: titleMap[eventType] ?? `计划：${subject}`,
    detail,
    status: eventType === 'plan.task.deleted' ? 'neutral' : status,
    runtime: 'Agent',
    structured: true,
    kind: 'step',
  };
};

const getMessageEventDisplay = ({
  detail,
  eventType,
  parsed,
  runtime,
  title,
}: EventDisplayContext): TaskEventDisplay | undefined => {
  if (eventType !== 'message.completed') {
    return undefined;
  }

  const role = getString(parsed, 'role');
  const reasoning =
    getString(parsed, 'reasoning_content') ?? getString(parsed, 'reasoning');

  if (role === 'assistant' && reasoning) {
    return {
      title: title ?? '思考',
      thought: reasoning,
      status: 'completed',
      runtime: runtime ?? 'Agent',
      structured: true,
      kind: 'thought',
    };
  }

  return {
    title: title ?? (role === 'assistant' ? '回答生成完成' : '消息生成完成'),
    detail: detail ?? (role === 'assistant' ? '已产生最终回答' : ''),
    status: 'completed',
    runtime: runtime ?? 'Agent',
    structured: true,
    kind: 'step',
    visibleInFlow: false,
  };
};

const getRunEventDisplay = ({
  detail,
  eventType,
  parsed,
  payload,
  runtime,
  status,
}: EventDisplayContext): TaskEventDisplay | undefined => {
  if (!eventType?.startsWith('run.')) {
    return undefined;
  }

  const workerID = getString(parsed, 'worker_id');
  const errorMessage = getString(parsed, 'error_message');
  const runDisplayMap: Record<
    string,
    Pick<TaskEventDisplay, 'title' | 'status'>
  > = {
    'run.started': { title: '任务开始执行', status: 'running' },
    'run.completed': { title: '任务执行完成', status: 'completed' },
    'run.failed': { title: '任务执行失败', status: 'failed' },
  };
  const display = runDisplayMap[eventType] ?? {
    title: getTaskEventText(eventType, payload),
    status,
  };

  return {
    ...display,
    detail: detail ?? errorMessage ?? (workerID ? `Worker: ${workerID}` : ''),
    runtime: runtime ?? 'Agent',
    structured: true,
    kind: 'step',
    visibleInFlow: false,
  };
};

const getSkillLoadedEventDisplay = ({
  eventType,
  parsed,
}: EventDisplayContext): TaskEventDisplay | undefined => {
  if (eventType !== 'skills.loaded') {
    return undefined;
  }

  const skillNames = getStringArray(parsed, 'skill_names');
  const singleSkillName = getString(parsed, 'skill_name');
  const names = [...skillNames, singleSkillName].filter(Boolean);
  const skillCount = getNumber(parsed, 'skill_count') ?? names.length;
  const title =
    names.length === 1
      ? `可用技能 “${names[0]}”`
      : skillCount > 0
        ? `可用技能目录 ${skillCount} 个`
        : '可用技能目录';

  return {
    title,
    detail: names.length > 1 ? names.slice(0, 3).join('、') : undefined,
    status: 'completed',
    runtime: 'Agent',
    structured: true,
    kind: 'step',
  };
};

const getStepEventDisplay = ({
  detail,
  eventType,
  parsed,
  runtime,
}: EventDisplayContext): TaskEventDisplay | undefined => {
  if (!eventType?.startsWith('step.')) {
    return undefined;
  }

  const stepName =
    getString(parsed, 'step_name') ||
    getString(parsed, 'step_id') ||
    `步骤 ${(getNumber(parsed, 'step_index') ?? 0) + 1}`;
  const stepType = getString(parsed, 'step_type');
  const errorMessage = getString(parsed, 'error_message');
  const final = getBoolean(parsed, 'final');
  const baseDisplay = {
    runtime: runtime ?? 'Agent',
    structured: true,
    kind: 'step' as const,
  };

  if (eventType === 'step.started') {
    return {
      ...baseDisplay,
      title: `开始执行 ${stepName}`,
      detail: detail ?? stepType,
      status: 'running',
    };
  }

  if (eventType === 'step.completed') {
    return {
      ...baseDisplay,
      title: `完成 ${stepName}`,
      detail: detail ?? (final ? '已产生最终回答' : stepType),
      status: 'completed',
    };
  }

  if (eventType === 'step.failed') {
    return {
      ...baseDisplay,
      title: `${stepName} 执行失败`,
      detail: detail ?? errorMessage ?? stepType,
      status: 'failed',
    };
  }

  return undefined;
};

const getDatabaseEventDisplay = ({
  detail,
  eventType,
  parsed,
  status,
  title,
}: EventDisplayContext): TaskEventDisplay | undefined => {
  if (eventType !== 'agent.database_query') {
    return undefined;
  }

  const databaseID = getString(parsed, 'database_id');
  const sql = getString(parsed, 'sql');
  const rowCount = getNumber(parsed, 'row_count');
  const error = getString(parsed, 'error');
  const queryDetail = [
    databaseID ? `数据库: ${databaseID}` : undefined,
    sql ? `SQL: ${sql}` : undefined,
    typeof rowCount === 'number' ? `返回 ${rowCount} 行` : undefined,
    error,
  ]
    .filter(Boolean)
    .join(' · ');

  return {
    title: title ?? '数据库查询',
    detail: detail ?? queryDetail,
    status,
    runtime: 'Agent',
    structured: true,
    kind: 'step',
  };
};

const getInternalLifecycleEventDisplay = ({
  eventType,
  runtime,
}: EventDisplayContext): TaskEventDisplay | undefined => {
  if (
    !eventType?.startsWith('context.') &&
    !eventType?.startsWith('memory.update_') &&
    !eventType?.startsWith('guardrail.')
  ) {
    return undefined;
  }

  return {
    title: '内部状态已更新',
    status: 'completed',
    runtime: runtime ?? 'Agent',
    structured: true,
    kind: 'event',
    visibleInFlow: false,
  };
};

const getStructuredEventDisplay = ({
  detail,
  eventType,
  payload,
  progress,
  runtime,
  status,
  structured,
  thought,
  title,
}: EventDisplayContext): TaskEventDisplay | undefined => {
  if (!structured) {
    return undefined;
  }

  return {
    title: title ?? getTaskEventText(eventType, payload),
    detail,
    thought,
    status,
    runtime,
    progress,
    structured,
    kind: thought ? 'thought' : 'step',
  };
};

export const getTaskEventDisplay = (
  eventType?: string,
  payload?: string,
): TaskEventDisplay => {
  const parsed = parseJSONObject(payload);
  const title = getString(parsed, 'title');
  const detail = getString(parsed, 'detail');
  const thought =
    getString(parsed, 'thought') ?? getString(parsed, 'reasoning_content');
  const runtime = normalizeExecutionType(getString(parsed, 'runtime'));
  const progress = getNumber(parsed, 'progress');
  const structured = Boolean(title || detail || thought || runtime || progress);
  const status = normalizeExecutionStatus(getString(parsed, 'status'));
  const context: EventDisplayContext = {
    detail,
    eventType,
    parsed,
    payload,
    progress,
    runtime,
    status,
    structured,
    thought,
    title,
  };

  return (
    getTaskToolEventDisplay(context) ??
    getPlanEventDisplay(context) ??
    getMessageEventDisplay(context) ??
    getRunEventDisplay(context) ??
    getSkillLoadedEventDisplay(context) ??
    getStepEventDisplay(context) ??
    getDatabaseEventDisplay(context) ??
    getInternalLifecycleEventDisplay(context) ??
    getStructuredEventDisplay(context) ?? {
      title: getTaskEventText(eventType, payload),
      status: 'completed',
      structured: false,
      kind: 'event',
    }
  );
};
