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
export enum TaskStatus {
  Created = 1,
  Queued = 2,
  Running = 3,
  Succeeded = 4,
  Failed = 5,
  Canceling = 6,
  Canceled = 7,
}
export interface TaskEvent {
  id: string,
  task_id: string,
  event_type: string,
  payload?: string,
  created_at: number,
}
export interface ChatTask {
  id: string,
  space_id: string,
  creator_id: string,
  conversation_id?: string,
  message_id?: string,
  skill_id?: string,
  title: string,
  status: TaskStatus,
  progress: number,
  input?: string,
  result?: string,
  error?: string,
  created_at: number,
  updated_at: number,
}
export interface CreateTaskRequest {
  space_id: string,
  title: string,
  conversation_id?: string,
  message_id?: string,
  skill_id?: string,
  input?: string,
}
export interface CreateTaskResponse {
  data?: ChatTask,
  code: number,
  msg: string,
}
export interface ListTasksRequest {
  space_id: string,
  status?: TaskStatus,
  page?: number,
  page_size?: number,
}
export interface ListTasksData {
  tasks: ChatTask[],
  total: number,
}
export interface ListTasksResponse {
  data?: ListTasksData,
  code: number,
  msg: string,
}
export interface GetTaskRequest {
  task_id: string
}
export interface GetTaskResponse {
  data?: ChatTask,
  code: number,
  msg: string,
}
export interface TaskEventsData {
  events: TaskEvent[]
}
export interface TaskEventsResponse {
  data?: TaskEventsData,
  code: number,
  msg: string,
}
export const CreateTask = /*#__PURE__*/createAPI<CreateTaskRequest, CreateTaskResponse>({
  "url": "/api/workbench/tasks",
  "method": "POST",
  "name": "CreateTask",
  "reqType": "CreateTaskRequest",
  "reqMapping": {
    "body": ["space_id", "title", "conversation_id", "message_id", "skill_id", "input"]
  },
  "resType": "CreateTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListTasks = /*#__PURE__*/createAPI<ListTasksRequest, ListTasksResponse>({
  "url": "/api/workbench/tasks",
  "method": "GET",
  "name": "ListTasks",
  "reqType": "ListTasksRequest",
  "reqMapping": {
    "query": ["space_id", "status", "page", "page_size"]
  },
  "resType": "ListTasksResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const GetTask = /*#__PURE__*/createAPI<GetTaskRequest, GetTaskResponse>({
  "url": "/api/workbench/tasks/:task_id",
  "method": "GET",
  "name": "GetTask",
  "reqType": "GetTaskRequest",
  "reqMapping": {
    "path": ["task_id"]
  },
  "resType": "GetTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const CancelTask = /*#__PURE__*/createAPI<GetTaskRequest, GetTaskResponse>({
  "url": "/api/workbench/tasks/:task_id/cancel",
  "method": "POST",
  "name": "CancelTask",
  "reqType": "GetTaskRequest",
  "reqMapping": {
    "path": ["task_id"]
  },
  "resType": "GetTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const RetryTask = /*#__PURE__*/createAPI<GetTaskRequest, GetTaskResponse>({
  "url": "/api/workbench/tasks/:task_id/retry",
  "method": "POST",
  "name": "RetryTask",
  "reqType": "GetTaskRequest",
  "reqMapping": {
    "path": ["task_id"]
  },
  "resType": "GetTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListTaskEvents = /*#__PURE__*/createAPI<GetTaskRequest, TaskEventsResponse>({
  "url": "/api/workbench/tasks/:task_id/events",
  "method": "GET",
  "name": "ListTaskEvents",
  "reqType": "GetTaskRequest",
  "reqMapping": {
    "path": ["task_id"]
  },
  "resType": "TaskEventsResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
