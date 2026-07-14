// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import { workbenchTask } from '@coze-studio/api-schema';

import type { TaskFormValues } from './types';
import type { ScheduledTask } from './service';

const WEEKDAY_LABELS = ['日', '一', '二', '三', '四', '五', '六'];

const padNumber = (value: number) => String(value).padStart(2, '0');

const scheduleParts = (value: TaskFormValues | ScheduledTask) =>
  'schedule_type' in value
    ? {
        type: value.schedule_type,
        runOnceAt: value.run_once_at || 0,
        minute: value.minute || 0,
        hour: value.hour || 0,
        weekday: value.weekday || 0,
        cronExpr: value.cron_expr || '',
      }
    : {
        type: value.scheduleType,
        runOnceAt: value.runOnceAt
          ? Math.floor(new Date(value.runOnceAt).getTime() / 1000)
          : 0,
        minute: value.minute,
        hour: value.hour,
        weekday: value.weekday,
        cronExpr: value.cronExpr,
      };

export const formatTimestamp = (timestamp?: number) => {
  if (!timestamp) {
    return '-';
  }
  const milliseconds =
    timestamp < 1_000_000_000_000 ? timestamp * 1000 : timestamp;
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(milliseconds));
};

export const formatScheduleSummary = (
  value: TaskFormValues | ScheduledTask,
) => {
  const schedule = scheduleParts(value);
  const time = `${padNumber(schedule.hour)}:${padNumber(schedule.minute)}`;
  switch (schedule.type) {
    case workbenchTask.ScheduledTaskScheduleType.Once:
      return schedule.runOnceAt
        ? `仅一次 · ${formatTimestamp(schedule.runOnceAt)}`
        : '仅执行一次';
    case workbenchTask.ScheduledTaskScheduleType.Hourly:
      return `每小时第 ${padNumber(schedule.minute)} 分`;
    case workbenchTask.ScheduledTaskScheduleType.Daily:
      return `每天 ${time}`;
    case workbenchTask.ScheduledTaskScheduleType.Weekly:
      return `每周${WEEKDAY_LABELS[schedule.weekday] || '一'} ${time}`;
    case workbenchTask.ScheduledTaskScheduleType.Cron:
      return schedule.cronExpr ? `Cron · ${schedule.cronExpr}` : '自定义 Cron';
    default:
      return '未配置';
  }
};

export const formatTaskStatus = (
  task: Pick<ScheduledTask, 'status' | 'latest_execution_status'>,
) => {
  if (task.status === workbenchTask.ScheduledTaskStatus.Disabled) {
    return '已停用';
  }
  if (task.status === workbenchTask.ScheduledTaskStatus.Completed) {
    return '已完成';
  }
  switch (task.latest_execution_status) {
    case workbenchTask.ScheduledTaskExecutionStatus.Queued:
    case workbenchTask.ScheduledTaskExecutionStatus.Running:
      return '执行中';
    case workbenchTask.ScheduledTaskExecutionStatus.Succeeded:
      return '执行成功，待下次执行';
    case workbenchTask.ScheduledTaskExecutionStatus.Failed:
    case workbenchTask.ScheduledTaskExecutionStatus.Canceled:
      return '执行失败，待下次执行';
    default:
      return '等待执行';
  }
};

export const formatExecutionStatus = (
  status: workbenchTask.ScheduledTaskExecutionStatus,
) => {
  switch (status) {
    case workbenchTask.ScheduledTaskExecutionStatus.Queued:
      return '排队中';
    case workbenchTask.ScheduledTaskExecutionStatus.Running:
      return '执行中';
    case workbenchTask.ScheduledTaskExecutionStatus.Succeeded:
      return '成功';
    case workbenchTask.ScheduledTaskExecutionStatus.Failed:
      return '失败';
    case workbenchTask.ScheduledTaskExecutionStatus.Canceled:
      return '已取消';
    default:
      return '未知';
  }
};

const parseJSONObject = (raw: string, label: string) => {
  const parsed = JSON.parse(raw || '{}') as unknown;
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(`${label}必须是 JSON 对象`);
  }
  return parsed as Record<string, unknown>;
};

export const buildTaskPayload = (values: TaskFormValues) => {
  if (values.targetType === workbenchTask.ScheduledTaskTargetType.Agent) {
    return JSON.stringify({
      message: values.message.trim(),
      variables: parseJSONObject(values.agentVariables || '{}', 'Agent 参数'),
    });
  }
  return JSON.stringify(parseJSONObject(values.workflowInputs, '工作流参数'));
};

export const validateTaskForm = (values: TaskFormValues) => {
  if (!values.name.trim()) {
    return '请输入任务名称';
  }
  if (values.name.trim().length > 100) {
    return '任务名称不能超过 100 个字符';
  }
  if (!values.targetID) {
    return '请选择执行目标';
  }
  if (!values.timezone) {
    return '请选择时区';
  }
  if (
    values.scheduleType === workbenchTask.ScheduledTaskScheduleType.Once &&
    (!values.runOnceAt || new Date(values.runOnceAt).getTime() <= Date.now())
  ) {
    return '单次执行时间必须晚于当前时间';
  }
  if (
    values.scheduleType === workbenchTask.ScheduledTaskScheduleType.Cron &&
    values.cronExpr.trim().split(/\s+/).length !== 5
  ) {
    return 'Cron 表达式必须包含 5 段（分 时 日 月 周）';
  }
  if (
    values.targetType === workbenchTask.ScheduledTaskTargetType.Agent &&
    !values.message.trim()
  ) {
    return '请输入 Agent 要执行的任务内容';
  }
  try {
    const payload = buildTaskPayload(values);
    if (payload.length > 10_000) {
      return '任务参数不能超过 10000 个字符';
    }
  } catch (error) {
    return error instanceof Error ? error.message : '任务参数不是有效 JSON';
  }
  if (values.maxExecutions < 0) {
    return '最大执行次数不能小于 0';
  }
  return '';
};

export const toLocalDateTimeInput = (timestamp?: number) => {
  if (!timestamp) {
    return '';
  }
  const date = new Date(timestamp * 1000);
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
};

export const parseTaskPayload = (task: ScheduledTask) => {
  try {
    const payload = JSON.parse(task.payload || '{}') as Record<string, unknown>;
    if (task.target_type === workbenchTask.ScheduledTaskTargetType.Agent) {
      return {
        message: typeof payload.message === 'string' ? payload.message : '',
        agentVariables: JSON.stringify(payload.variables || {}, null, 2),
        workflowInputs: '{}',
      };
    }
    return {
      message: '',
      agentVariables: '{}',
      workflowInputs: JSON.stringify(payload, null, 2),
    };
  } catch {
    return { message: '', agentVariables: '{}', workflowInputs: '{}' };
  }
};
