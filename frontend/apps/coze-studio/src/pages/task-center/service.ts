// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import { workbenchTask } from '@coze-studio/api-schema';

export const createScheduledTask = workbenchTask.CreateScheduledTask;
export const updateScheduledTask = workbenchTask.UpdateScheduledTask;
export const getScheduledTask = workbenchTask.GetScheduledTask;
export const listScheduledTasks = workbenchTask.ListScheduledTasks;
export const deleteScheduledTask = workbenchTask.DeleteScheduledTask;
export const enableScheduledTask = workbenchTask.EnableScheduledTask;
export const disableScheduledTask = workbenchTask.DisableScheduledTask;
export const executeScheduledTask = workbenchTask.ExecuteScheduledTask;
export const listScheduledTaskExecutions =
  workbenchTask.ListScheduledTaskExecutions;
export const listScheduledTaskTargets = workbenchTask.ListScheduledTaskTargets;
export const listScheduledTaskCronPresets =
  workbenchTask.ListScheduledTaskCronPresets;

export type ScheduledTask = workbenchTask.ScheduledTask;
export type ScheduledTaskExecution = workbenchTask.ScheduledTaskExecution;
export type ScheduledTaskTarget = workbenchTask.ScheduledTaskTarget;
export type ScheduledTaskCronPreset = workbenchTask.ScheduledTaskCronPreset;
export type CreateScheduledTaskRequest =
  workbenchTask.CreateScheduledTaskRequest;
export type UpdateScheduledTaskRequest =
  workbenchTask.UpdateScheduledTaskRequest;
