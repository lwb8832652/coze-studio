// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import type { workbenchTask } from '@coze-studio/api-schema';

export interface TaskFormValues {
  name: string;
  targetType: workbenchTask.ScheduledTaskTargetType;
  targetID: string;
  scheduleType: workbenchTask.ScheduledTaskScheduleType;
  timezone: string;
  runOnceAt: string;
  minute: number;
  hour: number;
  weekday: number;
  cronExpr: string;
  message: string;
  agentVariables?: string;
  workflowInputs: string;
  keepConversation: boolean;
  maxExecutions: number;
}
