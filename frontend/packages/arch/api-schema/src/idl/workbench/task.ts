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

import * as base from './../base';
export { base };
import { createAPI } from './../../api/config';
export enum ScheduledTaskTargetType {
  Agent = 1,
  Workflow = 2,
}
export enum ScheduledTaskScheduleType {
  Once = 1,
  Hourly = 2,
  Daily = 3,
  Weekly = 4,
  Cron = 5,
}
export enum ScheduledTaskStatus {
  Enabled = 1,
  Disabled = 2,
  Completed = 3,
}
export enum ScheduledTaskExecutionStatus {
  Queued = 1,
  Running = 2,
  Succeeded = 3,
  Failed = 4,
  Canceled = 5,
}
export interface ScheduledTask {
  id: string,
  space_id: string,
  creator_id: string,
  name: string,
  target_type: ScheduledTaskTargetType,
  target_id: string,
  target_name: string,
  target_icon_uri?: string,
  schedule_type: ScheduledTaskScheduleType,
  cron_expr?: string,
  timezone: string,
  run_once_at?: number,
  minute?: number,
  hour?: number,
  weekday?: number,
  payload: string,
  keep_conversation: boolean,
  status: ScheduledTaskStatus,
  execution_count: number,
  max_executions: number,
  latest_execution_at: number,
  next_execution_at: number,
  created_at: number,
  updated_at: number,
  version: number,
  latest_execution_status?: ScheduledTaskExecutionStatus,
  creator_name?: string,
}
export interface ScheduledTaskExecution {
  id: string,
  task_id: string,
  trigger_type: string,
  scheduled_at: number,
  status: ScheduledTaskExecutionStatus,
  attempt: number,
  thread_id?: string,
  run_id?: string,
  workflow_execution_id?: string,
  error_code?: string,
  error_message?: string,
  started_at: number,
  finished_at: number,
  created_at: number,
}
export interface ScheduledTaskTarget {
  id: string,
  type: ScheduledTaskTargetType,
  name: string,
  icon_uri?: string,
  published: boolean,
  input_schema?: string,
}
export interface ScheduledTaskCronPreset {
  id: string,
  label: string,
  schedule_type: ScheduledTaskScheduleType,
  cron_expr?: string,
}
export interface CreateScheduledTaskRequest {
  space_id: string,
  name: string,
  target_type: ScheduledTaskTargetType,
  target_id: string,
  schedule_type: ScheduledTaskScheduleType,
  cron_expr?: string,
  timezone: string,
  run_once_at?: number,
  minute?: number,
  hour?: number,
  weekday?: number,
  payload: string,
  keep_conversation?: boolean,
  max_executions?: number,
}
export interface UpdateScheduledTaskRequest {
  task_id: string,
  space_id: string,
  name: string,
  target_type: ScheduledTaskTargetType,
  target_id: string,
  schedule_type: ScheduledTaskScheduleType,
  cron_expr?: string,
  timezone: string,
  run_once_at?: number,
  minute?: number,
  hour?: number,
  weekday?: number,
  payload: string,
  keep_conversation?: boolean,
  max_executions?: number,
  version: number,
}
export interface GetScheduledTaskRequest {
  task_id: string,
  space_id: string,
}
export interface ScheduledTaskActionRequest {
  task_id: string,
  space_id: string,
}
export interface ListScheduledTasksRequest {
  space_id: string,
  target_type?: ScheduledTaskTargetType,
  keyword?: string,
  status?: ScheduledTaskStatus,
  page?: number,
  page_size?: number,
}
export interface ListScheduledTaskExecutionsRequest {
  task_id: string,
  space_id: string,
  page?: number,
  page_size?: number,
}
export interface ListScheduledTaskTargetsRequest {
  space_id: string,
  target_type: ScheduledTaskTargetType,
  keyword?: string,
  page?: number,
  page_size?: number,
}
export interface ListScheduledTaskCronPresetsRequest {
  timezone?: string
}
export interface ScheduledTaskResponse {
  data?: ScheduledTask,
  code: number,
  msg: string,
}
export interface ScheduledTaskExecutionResponse {
  data?: ScheduledTaskExecution,
  code: number,
  msg: string,
}
export interface ListScheduledTasksData {
  tasks: ScheduledTask[],
  total: number,
}
export interface ListScheduledTasksResponse {
  data?: ListScheduledTasksData,
  code: number,
  msg: string,
}
export interface ListScheduledTaskExecutionsData {
  executions: ScheduledTaskExecution[],
  total: number,
}
export interface ListScheduledTaskExecutionsResponse {
  data?: ListScheduledTaskExecutionsData,
  code: number,
  msg: string,
}
export interface ListScheduledTaskTargetsData {
  targets: ScheduledTaskTarget[],
  total: number,
}
export interface ListScheduledTaskTargetsResponse {
  data?: ListScheduledTaskTargetsData,
  code: number,
  msg: string,
}
export interface ListScheduledTaskCronPresetsResponse {
  data: ScheduledTaskCronPreset[],
  code: number,
  msg: string,
}
export const CreateScheduledTask = /*#__PURE__*/createAPI<CreateScheduledTaskRequest, ScheduledTaskResponse>({
  "url": "/api/workbench/scheduled_tasks",
  "method": "POST",
  "name": "CreateScheduledTask",
  "reqType": "CreateScheduledTaskRequest",
  "reqMapping": {
    "body": ["space_id", "name", "target_type", "target_id", "schedule_type", "cron_expr", "timezone", "run_once_at", "minute", "hour", "weekday", "payload", "keep_conversation", "max_executions"]
  },
  "resType": "ScheduledTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListScheduledTasks = /*#__PURE__*/createAPI<ListScheduledTasksRequest, ListScheduledTasksResponse>({
  "url": "/api/workbench/scheduled_tasks",
  "method": "GET",
  "name": "ListScheduledTasks",
  "reqType": "ListScheduledTasksRequest",
  "reqMapping": {
    "query": ["space_id", "target_type", "keyword", "status", "page", "page_size"]
  },
  "resType": "ListScheduledTasksResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListScheduledTaskTargets = /*#__PURE__*/createAPI<ListScheduledTaskTargetsRequest, ListScheduledTaskTargetsResponse>({
  "url": "/api/workbench/scheduled_task_targets",
  "method": "GET",
  "name": "ListScheduledTaskTargets",
  "reqType": "ListScheduledTaskTargetsRequest",
  "reqMapping": {
    "query": ["space_id", "target_type", "keyword", "page", "page_size"]
  },
  "resType": "ListScheduledTaskTargetsResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListScheduledTaskCronPresets = /*#__PURE__*/createAPI<ListScheduledTaskCronPresetsRequest, ListScheduledTaskCronPresetsResponse>({
  "url": "/api/workbench/scheduled_task_cron_presets",
  "method": "GET",
  "name": "ListScheduledTaskCronPresets",
  "reqType": "ListScheduledTaskCronPresetsRequest",
  "reqMapping": {
    "query": ["timezone"]
  },
  "resType": "ListScheduledTaskCronPresetsResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const GetScheduledTask = /*#__PURE__*/createAPI<GetScheduledTaskRequest, ScheduledTaskResponse>({
  "url": "/api/workbench/scheduled_tasks/:task_id",
  "method": "GET",
  "name": "GetScheduledTask",
  "reqType": "GetScheduledTaskRequest",
  "reqMapping": {
    "path": ["task_id"],
    "query": ["space_id"]
  },
  "resType": "ScheduledTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const UpdateScheduledTask = /*#__PURE__*/createAPI<UpdateScheduledTaskRequest, ScheduledTaskResponse>({
  "url": "/api/workbench/scheduled_tasks/:task_id",
  "method": "PUT",
  "name": "UpdateScheduledTask",
  "reqType": "UpdateScheduledTaskRequest",
  "reqMapping": {
    "path": ["task_id"],
    "body": ["space_id", "name", "target_type", "target_id", "schedule_type", "cron_expr", "timezone", "run_once_at", "minute", "hour", "weekday", "payload", "keep_conversation", "max_executions", "version"]
  },
  "resType": "ScheduledTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const DeleteScheduledTask = /*#__PURE__*/createAPI<ScheduledTaskActionRequest, ScheduledTaskResponse>({
  "url": "/api/workbench/scheduled_tasks/:task_id",
  "method": "DELETE",
  "name": "DeleteScheduledTask",
  "reqType": "ScheduledTaskActionRequest",
  "reqMapping": {
    "path": ["task_id"],
    "body": ["space_id"]
  },
  "resType": "ScheduledTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const EnableScheduledTask = /*#__PURE__*/createAPI<ScheduledTaskActionRequest, ScheduledTaskResponse>({
  "url": "/api/workbench/scheduled_tasks/:task_id/enable",
  "method": "POST",
  "name": "EnableScheduledTask",
  "reqType": "ScheduledTaskActionRequest",
  "reqMapping": {
    "path": ["task_id"],
    "body": ["space_id"]
  },
  "resType": "ScheduledTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const DisableScheduledTask = /*#__PURE__*/createAPI<ScheduledTaskActionRequest, ScheduledTaskResponse>({
  "url": "/api/workbench/scheduled_tasks/:task_id/disable",
  "method": "POST",
  "name": "DisableScheduledTask",
  "reqType": "ScheduledTaskActionRequest",
  "reqMapping": {
    "path": ["task_id"],
    "body": ["space_id"]
  },
  "resType": "ScheduledTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ExecuteScheduledTask = /*#__PURE__*/createAPI<ScheduledTaskActionRequest, ScheduledTaskExecutionResponse>({
  "url": "/api/workbench/scheduled_tasks/:task_id/execute",
  "method": "POST",
  "name": "ExecuteScheduledTask",
  "reqType": "ScheduledTaskActionRequest",
  "reqMapping": {
    "path": ["task_id"],
    "body": ["space_id"]
  },
  "resType": "ScheduledTaskExecutionResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListScheduledTaskExecutions = /*#__PURE__*/createAPI<ListScheduledTaskExecutionsRequest, ListScheduledTaskExecutionsResponse>({
  "url": "/api/workbench/scheduled_tasks/:task_id/executions",
  "method": "GET",
  "name": "ListScheduledTaskExecutions",
  "reqType": "ListScheduledTaskExecutionsRequest",
  "reqMapping": {
    "path": ["task_id"],
    "query": ["space_id", "page", "page_size"]
  },
  "resType": "ListScheduledTaskExecutionsResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});