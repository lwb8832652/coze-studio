// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import { workbenchTask } from '@coze-studio/api-schema';

import {
  buildTaskPayload,
  formatScheduleSummary,
  formatTaskStatus,
  validateTaskForm,
} from '../utils';
import type { TaskFormValues } from '../types';

const baseValues: TaskFormValues = {
  name: '每日简报',
  targetType: workbenchTask.ScheduledTaskTargetType.Agent,
  targetID: '100',
  scheduleType: workbenchTask.ScheduledTaskScheduleType.Daily,
  timezone: 'Asia/Shanghai',
  runOnceAt: '',
  minute: 30,
  hour: 9,
  weekday: 1,
  cronExpr: '',
  message: '总结今天的重要信息',
  workflowInputs: '{}',
  keepConversation: true,
  maxExecutions: 0,
};

describe('task center utilities', () => {
  it('describes every supported schedule without leaking cron internals', () => {
    expect(formatScheduleSummary(baseValues)).toBe('每天 09:30');
    expect(
      formatScheduleSummary({
        ...baseValues,
        scheduleType: workbenchTask.ScheduledTaskScheduleType.Weekly,
        weekday: 5,
        hour: 18,
        minute: 5,
      }),
    ).toBe('每周五 18:05');
    expect(
      formatScheduleSummary({
        ...baseValues,
        scheduleType: workbenchTask.ScheduledTaskScheduleType.Cron,
        cronExpr: '0 9 * * 1-5',
      }),
    ).toBe('Cron · 0 9 * * 1-5');
  });

  it('builds bounded payloads for agents and workflows', () => {
    expect(JSON.parse(buildTaskPayload(baseValues))).toEqual({
      message: '总结今天的重要信息',
      variables: {},
    });
    expect(
      JSON.parse(
        buildTaskPayload({
          ...baseValues,
          targetType: workbenchTask.ScheduledTaskTargetType.Workflow,
          workflowInputs: '{"city":"武汉"}',
        }),
      ),
    ).toEqual({ city: '武汉' });
  });

  it('validates target, payload, schedule, and one-time timestamps', () => {
    expect(validateTaskForm({ ...baseValues, targetID: '' })).toContain(
      '执行目标',
    );
    expect(
      validateTaskForm({
        ...baseValues,
        targetType: workbenchTask.ScheduledTaskTargetType.Workflow,
        workflowInputs: '{bad json}',
      }),
    ).toContain('JSON');
    expect(
      validateTaskForm({
        ...baseValues,
        scheduleType: workbenchTask.ScheduledTaskScheduleType.Cron,
        cronExpr: '* * *',
      }),
    ).toContain('5 段');
  });

  it('projects the latest execution state without hiding disabled tasks', () => {
    expect(
      formatTaskStatus({
        status: workbenchTask.ScheduledTaskStatus.Enabled,
        latest_execution_status:
          workbenchTask.ScheduledTaskExecutionStatus.Succeeded,
      }),
    ).toBe('执行成功，待下次执行');
    expect(
      formatTaskStatus({
        status: workbenchTask.ScheduledTaskStatus.Disabled,
        latest_execution_status:
          workbenchTask.ScheduledTaskExecutionStatus.Running,
      }),
    ).toBe('已停用');
  });
});
